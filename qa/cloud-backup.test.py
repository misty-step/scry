#!/usr/bin/env python3
"""Cloud backup gate negative controls; no network, credentials or learner data."""
from copy import deepcopy
from datetime import datetime, timedelta, timezone
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('cloud_backup', ROOT / 'scripts/scry-cloud-backup.py')
BACKUP = importlib.util.module_from_spec(spec)
spec.loader.exec_module(BACKUP)
NOW = datetime(2026, 10, 27, 20, 0, tzinfo=timezone.utc)


class CloudBackup(unittest.TestCase):
    def test_bootstrap_slot_is_once_per_year_not_every_minute_and_has_propagation_margin(self):
        cron, when = BACKUP.bootstrap_slot(NOW)
        self.assertEqual(cron, '20 20 27 10 *')
        self.assertEqual(when, NOW + timedelta(minutes=20))

    def test_provider_capability_refuses_even_if_model_configuration_is_missing(self):
        BACKUP.no_paid_work([{'name': 'INTERNAL_KEY', 'type': 'secret_text'}])
        for name in ('OPENROUTER_API_KEY', 'JEV_API_KEY'):
            with self.subTest(name=name), self.assertRaisesRegex(BACKUP.GateError, 'provider'):
                BACKUP.no_paid_work([{'name': name, 'type': 'secret_text'}])

    def test_retention_refuses_every_old_object_and_future_boundary_before_schedule(self):
        good = {'key': 'snapshots/synthetic/1-daily-one.json', 'size': 10,
                'etag': 'a' * 32, 'last_modified': '2026-10-27T19:00:00Z', 'storage_class': 'Standard'}
        self.assertEqual(BACKUP.validate_objects([good], 'snapshots/synthetic/', NOW), [good])
        for change in ({'last_modified': '2026-09-27T20:00:00Z'},
                       {'last_modified': '2026-09-27T20:30:00Z'},
                       {'last_modified': 'not a date'}, {'last_modified': '2026-10-28T00:00:00Z'},
                       {'size': True}, {'size': 0}, {'etag': ''},
                       {'key': 'other/key'}, {'storage_class': 'InfrequentAccess'}):
            with self.subTest(change=change), self.assertRaises(BACKUP.GateError):
                BACKUP.validate_objects([good | change], 'snapshots/synthetic/', NOW)
        with self.assertRaises(BACKUP.GateError):
            BACKUP.validate_objects([good, good], 'snapshots/synthetic/', NOW)

    def test_listing_requires_complete_bounded_pagination(self):
        calls = []
        obj = lambda n: {'key': f'snapshots/synthetic/{n}', 'size': 10, 'etag': 'a' * 32,
                         'last_modified': '2026-10-27T19:00:00Z', 'storage_class': 'Standard'}
        def request(path, **kwargs):
            calls.append(path)
            if len(calls) == 1:
                return {'result': [obj(1)], 'result_info': {'is_truncated': True, 'cursor': 'next'}}
            return {'result': [obj(2)], 'result_info': {'is_truncated': False}}
        self.assertEqual(len(BACKUP.list_objects(request, 'bucket', 'snapshots/synthetic/', NOW)), 2)
        self.assertIn('cursor=next', calls[1])
        for response in ({'result': [obj(1)]},
                         {'result': [obj(1)], 'result_info': {'is_truncated': True}},
                         {'result': [obj(1)], 'result_info': {'is_truncated': 'false'}}):
            with self.subTest(response=response), self.assertRaises(BACKUP.GateError):
                BACKUP.list_objects(lambda *a, **k: response, 'bucket', 'snapshots/synthetic/', NOW)
        with self.assertRaisesRegex(BACKUP.GateError, 'pagination'):
            BACKUP.list_objects(lambda *a, **k: {'result': [], 'result_info': {'is_truncated': True, 'cursor': 'same'}},
                                'bucket', 'snapshots/synthetic/', NOW)

    def test_cron_event_is_actual_exact_slot_success_not_health_or_key_presence(self):
        cron, when = BACKUP.bootstrap_slot(NOW)
        event = {'cron': cron, 'datetime': '2026-10-27T20:20:01Z',
                 'scheduledDatetime': '2026-10-27T20:20:00Z', 'scriptName': 'synthetic-worker', 'status': 'success'}
        self.assertTrue(BACKUP.cron_success([event], 'synthetic-worker', cron, when))
        self.assertFalse(BACKUP.cron_success([], 'synthetic-worker', cron, when))
        for change in ({'cron': '* * * * *'}, {'status': 'ok'}, {'status': 'failure'},
                       {'scriptName': 'other'}, {'scheduledDatetime': '2026-10-26T20:20:00Z'},
                       {'datetime': '2026-10-27T20:19:59Z'}):
            with self.subTest(change=change):
                self.assertFalse(BACKUP.cron_success([event | change], 'synthetic-worker', cron, when))
        with self.assertRaisesRegex(BACKUP.GateError, 'ambiguous'):
            BACKUP.cron_success([event, event], 'synthetic-worker', cron, when)
        with self.assertRaisesRegex(BACKUP.GateError, 'truncated'):
            BACKUP.cron_success([event] * 100, 'synthetic-worker', cron, when)

    def test_graphql_schema_and_errors_fail_closed(self):
        schema = {'data': {'row': {'fields': [{'name': n} for n in BACKUP.EVENT_FIELDS]},
                           'filt': {'inputFields': [{'name': n} for n in BACKUP.FILTER_FIELDS]}}}
        BACKUP.require_schema(schema)
        for response in ({'data': {}}, dict(schema, errors=[{'message': 'PRIVATE'}]),
                         {'data': {'row': None, 'filt': None}}):
            with self.subTest(response=response), self.assertRaises(BACKUP.GateError):
                BACKUP.require_schema(response)
        bad = deepcopy(schema)
        bad['data']['row']['fields'].pop()
        with self.assertRaises(BACKUP.GateError): BACKUP.require_schema(bad)

    def test_full_readback_claim_requires_reviewed_live_runtime_not_any_green_build(self):
        runtime = (ROOT / 'src/runtime.rs').read_text()
        # Frozen CI snapshots intentionally have no .git history. The native
        # promoter transports exact-source Git bytes; exercise that seam here.
        with mock.patch.object(BACKUP.subprocess, 'check_output', return_value=runtime):
            BACKUP.require_backup_contract('62a7716e6f619c10fbda60630d86461ede33d8b7')
            BACKUP.require_backup_contract('f254b7e2a6ddf2e4f080130cd8127781846fbe3b')
        for changed in ('', runtime.replace('self.prune_backups(&prefix).await?;', 'return Ok(());'),
                        runtime.replace('Scheduled snapshot was not acknowledged.', 'Always resolve.')):
            with self.subTest(changed=bool(changed)), mock.patch.object(BACKUP.subprocess, 'check_output', return_value=changed):
                with self.assertRaisesRegex(BACKUP.GateError, 'contract is not reviewed'):
                    BACKUP.require_backup_contract('62a7716e6f619c10fbda60630d86461ede33d8b7')
        with self.assertRaisesRegex(BACKUP.GateError, 'source is not reviewed'):
            BACKUP.require_backup_contract('a' * 40)

    def test_transport_allows_only_content_free_claim_and_owned_temporary_object_deletion(self):
        client = BACKUP.Cloudflare('a' * 32, 'synthetic-only')
        claim = '/r2/buckets/private-backups/objects/operations/recovery-claims/' + 'a' * 32 + '.json'
        client.allow_claim(claim)
        class Response:
            headers = {'Content-Type': 'application/json'}
            def __enter__(self): return self
            def __exit__(self, *args): pass
            def read(self, size): return b'{"success":true,"result":{}}'
        with mock.patch.object(client, 'open', return_value=Response()) as opened:
            client.request(claim, raw=True)
            client.request(claim, method='PUT', body=b'{}')
            own = '/r2/buckets/scry-recovery-' + 'b' * 24 + '-data/objects/snapshots/recovery-' + 'b' * 24 + '/1-daily-one.json'
            client.request(own, method='DELETE')
            self.assertEqual(opened.call_count, 3)
            for path, method in [(own, 'GET'), (own.replace('recovery-' + 'b' * 24 + '/', 'recovery-' + 'c' * 24 + '/'), 'DELETE'),
                                 ('/r2/buckets/private-backups/objects/snapshots/owner/archive.json', 'GET'),
                                 ('/r2/buckets/private-backups/objects/snapshots/owner/archive.json', 'DELETE')]:
                with self.subTest(path=path, method=method), self.assertRaises(BACKUP.GateError):
                    client.request(path, method=method)
            self.assertEqual(opened.call_count, 3)


class BootstrapLifecycle(unittest.TestCase):
    def fixture(self, *, event=True, schema=True, put_error=False, drift=False, cleanup_error=False):
        testcase = self
        class Client:
            def __init__(self):
                self.schedules, self.puts, self.queries = [], [], 0
                self.now = NOW
                self.slot, self.when = BACKUP.bootstrap_slot(NOW)
            def request(self, path, *, method='GET', payload=None):
                testcase.assertEqual(path, '/workers/scripts/synthetic-worker/schedules')
                if method == 'PUT':
                    self.puts.append(payload)
                    if cleanup_error and not payload: raise BACKUP.GateError('cleanup failed')
                    self.schedules = payload
                    if put_error and payload: raise BACKUP.GateError('uncertain schedule PUT')
                return {'schedules': self.schedules}
            def graphql(self, query):
                self.queries += 1
                if query == BACKUP.SCHEMA_QUERY:
                    return {'data': {'row': {'fields': [{'name': n} for n in BACKUP.EVENT_FIELDS]},
                                     'filt': {'inputFields': [{'name': n} for n in BACKUP.FILTER_FIELDS]}}} if schema else {'errors': [{}]}
                rows = []
                if event and self.now >= self.when:
                    rows = [{'cron': self.slot, 'datetime': BACKUP.iso(self.when + timedelta(seconds=1)),
                             'scheduledDatetime': BACKUP.iso(self.when), 'scriptName': 'synthetic-worker', 'status': 'success'}]
                return {'data': {'viewer': {'accounts': [{'workersInvocationsScheduled': rows}]}}}
            def sleep(self, seconds):
                self.now += timedelta(seconds=seconds)
                if drift: self.schedules = [{'cron': 'operator-change'}]
        return Client()

    def run_bootstrap(self, client, unchanged=lambda: None):
        return BACKUP.bootstrap(client, {'name': 'synthetic-worker', 'account_id': 'a' * 32}, 'live-version',
                                unchanged, now=NOW, clock=lambda: client.now, sleep=client.sleep)

    def test_success_proves_event_restores_empty_crons_and_retains_alarm_truth(self):
        client = self.fixture()
        proof = self.run_bootstrap(client)
        self.assertTrue(proof['provider_scheduled_success'])
        self.assertTrue(proof['existing_24h_alarm_rearmed'])
        self.assertFalse(proof['natural_next_day_run_observed'])
        self.assertEqual(client.puts, [[{'cron': client.slot}], []])
        self.assertEqual(client.schedules, [])

    def test_no_event_times_out_and_removes_only_our_slot(self):
        client = self.fixture(event=False)
        with self.assertRaisesRegex(BACKUP.GateError, 'no successful provider Cron'):
            self.run_bootstrap(client)
        self.assertEqual(client.schedules, [])
        self.assertEqual(len(client.puts), 2)

    def test_schema_or_preflight_drift_refuses_before_mutation(self):
        client = self.fixture(schema=False)
        with self.assertRaises(BACKUP.GateError): self.run_bootstrap(client)
        self.assertEqual(client.puts, [])

        client = self.fixture()
        with self.assertRaisesRegex(BACKUP.GateError, 'synthetic drift'):
            self.run_bootstrap(client, lambda: BACKUP.require(False, 'synthetic drift'))
        self.assertEqual(client.puts, [])

    def test_uncertain_put_is_never_retried_and_owned_slot_is_cleaned(self):
        client = self.fixture(put_error=True)
        with self.assertRaisesRegex(BACKUP.GateError, 'uncertain schedule PUT'):
            self.run_bootstrap(client)
        self.assertEqual(client.puts, [[{'cron': client.slot}], []])

    def test_concurrent_operator_schedule_is_never_overwritten(self):
        client = self.fixture(drift=True)
        with self.assertRaisesRegex(BACKUP.GateError, 'cleanup requires operator inspection'):
            self.run_bootstrap(client)
        self.assertEqual(client.puts, [[{'cron': client.slot}]])
        self.assertEqual(client.schedules, [{'cron': 'operator-change'}])

    def test_cleanup_failure_refuses_success_even_after_real_event(self):
        client = self.fixture(cleanup_error=True)
        with self.assertRaisesRegex(BACKUP.GateError, 'cleanup failed'):
            self.run_bootstrap(client)

    def test_preexisting_operator_schedule_is_not_replaced(self):
        client = self.fixture()
        client.schedules = [{'cron': '30 9 * * *'}]
        with self.assertRaisesRegex(BACKUP.GateError, 'unexpected production schedule'):
            self.run_bootstrap(client)
        self.assertEqual(client.puts, [])

    def test_operator_schedule_added_during_preflight_is_not_overwritten(self):
        client = self.fixture()
        def changed(): client.schedules = [{'cron': 'operator-added'}]
        with self.assertRaisesRegex(BACKUP.GateError, 'schedule changed before bootstrap'):
            self.run_bootstrap(client, changed)
        self.assertEqual(client.puts, [])


class ApprovalSourceBinding(unittest.TestCase):
    def test_native_version_only_release_is_allowed_but_product_or_script_byte_is_not(self):
        with tempfile.TemporaryDirectory() as directory, mock.patch.object(BACKUP, 'ROOT', Path(directory)):
            root = Path(directory)
            def git(*args):
                return subprocess.check_output(['git', '-c', 'user.name=Synthetic proof', '-c',
                    'user.email=proof@example.invalid', *args], cwd=root, stderr=subprocess.DEVNULL).decode().strip()
            git('init', '--quiet')
            files = {'Cargo.toml': '[package]\nname="scry"\nversion="0.8.1"\n',
                     'Cargo.lock': 'version=4\n[[package]]\nname="scry"\nversion="0.8.1"\n',
                     'package.json': '{"name":"scry","version":"0.8.1","scripts":{"ci":"unchanged"}}',
                     'package-lock.json': '{"version":"0.8.1","packages":{"":{"version":"0.8.1"}}}',
                     '.release-please-manifest.json': '{".":"0.8.1"}',
                     'CHANGELOG.md': '# Changelog\n\n## [0.8.1]\n\nRetained history.\n',
                     'src/lib.rs': '// exact product bytes\n', 'scripts/guard.py': '# exact operational bytes\n',
                     BACKUP.APPROVAL_PATH: '{"approved":false}\n'}
            def write(path, text):
                target = root / path; target.parent.mkdir(parents=True, exist_ok=True); target.write_text(text)
            for path, contents in files.items(): write(path, contents)
            git('add', '.'); git('commit', '--quiet', '-m', 'reviewed implementation')
            baseline = git('rev-parse', 'HEAD')
            for path in BACKUP.RELEASE_METADATA - {'CHANGELOG.md'}: write(path, files[path].replace('0.8.1', '0.9.0'))
            write('CHANGELOG.md', files['CHANGELOG.md'].replace('# Changelog\n\n', '# Changelog\n\n## [0.9.0]\n\nNew release.\n\n'))
            write(BACKUP.APPROVAL_PATH, '{"approved":true}\n')
            git('add', '.'); git('commit', '--quiet', '-m', 'native version-only release')
            source = git('rev-parse', 'HEAD')
            approval = {'reviewed_source': baseline, 'release_version': '0.9.0'}
            BACKUP.verify_reviewed_source(approval, {'source': source})
            for path, text in [('src/lib.rs', files['src/lib.rs'] + 'x'),
                               ('scripts/guard.py', files['scripts/guard.py'] + 'x'),
                               ('package.json', '{"name":"scry","version":"0.9.0","scripts":{"ci":"bypass"}}'),
                               ('CHANGELOG.md', '# Changelog\n\n## [0.9.0]\nOld history removed.\n')]:
                git('reset', '--hard', source)
                write(path, text); git('add', '.'); git('commit', '--quiet', '-m', 'unauthorized one-byte drift')
                with self.subTest(path=path), self.assertRaises(BACKUP.GateError):
                    BACKUP.verify_reviewed_source(approval, {'source': git('rev-parse', 'HEAD')})

if __name__ == '__main__': unittest.main()
