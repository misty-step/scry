#!/usr/bin/env python3
"""Exercise the daily production trigger offline, without pass or network access."""
import importlib.util
import io
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock
import urllib.error
import urllib.request
import urllib.response

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('scry_daily_backup', ROOT / 'scripts/scry-daily-backup.py')
BACKUP = importlib.util.module_from_spec(spec)
spec.loader.exec_module(BACKUP)
ENTRIES = [
    'misty-step/SCRY_ACCESS_CLIENT_ID',
    'misty-step/SCRY_ACCESS_CLIENT_SECRET',
    'operations/scry/production-20261016/backup-trigger',
]
VALUES = [b'synthetic-client-id-123456', b'synthetic-client-secret-123456', b'synthetic-backup-trigger-key-123456']


class DailyBackup(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.config = Path(directory.name) / 'production.toml'
        self.configure('https://scry.example')
        config = mock.patch.object(BACKUP, 'CONFIG', self.config)
        config.start()
        self.addCleanup(config.stop)
        credentials = mock.patch.object(BACKUP.subprocess, 'check_output', side_effect=[v + b'\n' for v in VALUES])
        self.credentials = credentials.start()
        self.addCleanup(credentials.stop)
        http = mock.patch.object(BACKUP, 'urlopen')
        self.http = http.start()
        self.addCleanup(http.stop)
        self.response = self.http.return_value.__enter__.return_value
        self.response.status = 200
        self.response.read.return_value = b'Snapshot verified.'

    def configure(self, origin):
        self.config.write_text('[vars]\nCANONICAL_ORIGIN = "' + origin + '"\n')

    def run_main(self):
        output = io.StringIO()
        with mock.patch('sys.stdout', output), mock.patch('sys.stderr', output):
            result = BACKUP.main()
        return result, output.getvalue()

    def test_success_posts_local_credentials_without_newlines(self):
        self.assertEqual(self.run_main(), (0, 'Scry production snapshot verified.\n'))
        self.assertEqual(self.credentials.call_args_list, [
            mock.call(['pass', 'show', entry], stderr=subprocess.DEVNULL, timeout=30) for entry in ENTRIES])
        request = self.http.call_args.args[0]
        self.assertEqual(request.full_url, 'https://scry.example/internal/backup')
        self.assertEqual(request.get_method(), 'POST')
        self.assertEqual(request.data, b'')
        headers = {k.lower(): v for k, v in request.header_items()}
        for name, value in zip(['cf-access-client-id', 'cf-access-client-secret', 'x-scry-backup-key'], VALUES):
            self.assertEqual(headers[name], value.decode())
        self.assertEqual(self.http.call_args.kwargs, {'timeout': 30})

    def test_success_accepts_verified_prefix(self):
        self.response.read.return_value = b'Snapshot verified.\n'
        self.assertEqual(self.run_main()[0], 0)

    def test_non_200_refuses_success_body(self):
        for status in [201, 204, 302, 403, 503]:
            with self.subTest(status=status):
                self.credentials.side_effect = [v + b'\n' for v in VALUES]
                self.response.status = status
                result, output = self.run_main()
                self.assertEqual(result, 1)
                self.assertIn('HTTP ' + str(status), output)
                self.assertNotIn('snapshot verified.', output)

    def test_wrong_body_is_failure_and_never_logged(self):
        self.response.read.return_value = b'private-upstream-body'
        self.assertEqual(self.run_main(), (1, 'Scry backup failed: verification response missing.\n'))

    def test_http_error_is_sanitized(self):
        body = io.BytesIO(b'private-upstream-body')
        self.http.side_effect = urllib.error.HTTPError('https://private-url', 403, 'private-reason', {}, body)
        self.assertEqual(self.run_main(), (1, 'Scry backup failed: HTTP 403.\n'))
        self.assertTrue(body.closed)

    def test_network_failures_are_sanitized(self):
        for error in [urllib.error.URLError('private-error'), TimeoutError('private-timeout')]:
            with self.subTest(error=type(error).__name__):
                self.credentials.side_effect = [v + b'\n' for v in VALUES]
                self.http.side_effect = error
                self.assertEqual(self.run_main(), (1, 'Scry backup failed: network request failed.\n'))

    def test_invalid_credentials_stop_before_network(self):
        for value in [b'', b'short', VALUES[0] + b'\nprivate-header', VALUES[0] + b'\x00',
                      VALUES[0] + b'\x7f', VALUES[0] + b' ', b'\xff' * 32]:
            with self.subTest(value=value):
                self.credentials.side_effect = [value, *VALUES[1:]]
                self.assertEqual(self.run_main(), (1, 'Scry backup failed: credential invalid.\n'))
                self.http.assert_not_called()

    def test_credential_lookup_errors_are_sanitized(self):
        for error in [FileNotFoundError('private-path'),
                      subprocess.CalledProcessError(1, ['pass'], output=b'private-output'),
                      subprocess.TimeoutExpired(['pass'], 30, output=b'private-output')]:
            with self.subTest(error=type(error).__name__):
                self.credentials.side_effect = error
                self.assertEqual(self.run_main(), (1, 'Scry backup failed: credential lookup failed.\n'))
                self.http.assert_not_called()

    def test_origin_changes_with_production_config(self):
        self.configure('https://moved.example/')
        self.assertEqual(self.run_main()[0], 0)
        self.assertEqual(self.http.call_args.args[0].full_url, 'https://moved.example/internal/backup')

    def test_invalid_origin_is_refused_before_credentials(self):
        for origin in ['http://scry.example', 'https://user:password@scry.example', 'https://scry.example/path',
                       'https://scry.example?query', 'https://scry.example#fragment', '', 'https://']:
            with self.subTest(origin=origin):
                self.configure(origin)
                self.assertEqual(self.run_main(), (1, 'Scry backup failed: canonical origin invalid.\n'))
                self.credentials.assert_not_called()
                self.http.assert_not_called()

    def test_unreadable_or_malformed_config_is_sanitized(self):
        self.config.unlink()
        self.assertEqual(self.run_main(), (1, 'Scry backup failed: production config unreadable.\n'))
        for text in ['not toml', '[vars]\n', '[vars]\nCANONICAL_ORIGIN = 42\n']:
            with self.subTest(text=text):
                self.config.write_text(text)
                self.assertEqual(self.run_main()[0], 1)
                self.credentials.assert_not_called()
                self.http.assert_not_called()

    def test_unexpected_error_never_exposes_details(self):
        self.http.side_effect = RuntimeError('private-error')
        self.assertEqual(self.run_main(), (1, 'Scry backup failed: unexpected failure.\n'))


class Redirects(unittest.TestCase):
    def test_redirects_are_refused_without_a_second_request(self):
        for status in [301, 302, 303, 307, 308]:
            with self.subTest(status=status):
                response = urllib.response.addinfourl(io.BytesIO(b'private-redirect-body'),
                    {'Location': 'https://elsewhere.example/internal/backup'}, 'https://scry.example/internal/backup', status)
                response.msg = 'private-redirect-reason'
                with mock.patch.object(urllib.request.HTTPSHandler, 'do_open', return_value=response) as transport:
                    with self.assertRaises(urllib.error.HTTPError) as caught:
                        BACKUP.urlopen(urllib.request.Request('https://scry.example/internal/backup',
                            data=b'', headers={'x-scry-backup-key': 'synthetic-secret'}, method='POST'), timeout=30)
                    self.assertEqual(caught.exception.code, status)
                    caught.exception.close()
                    transport.assert_called_once()


if __name__ == '__main__':
    unittest.main()
