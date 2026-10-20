#!/usr/bin/env python3
"""Request and verify the daily production snapshot from Mirrodin."""
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
import time
import tomllib
import urllib.error
import urllib.parse
import urllib.request
import uuid

CONFIG = Path(__file__).resolve().parents[1] / 'deploy/rust-production.toml'
SENTRY_TRANSPORT = Path(__file__).resolve().parents[1] / 'deploy/cloudflare-hosting/telemetry.mjs'
PASS_HEADERS = {
    'CF-Access-Client-Id': 'misty-step/SCRY_ACCESS_CLIENT_ID',
    'CF-Access-Client-Secret': 'misty-step/SCRY_ACCESS_CLIENT_SECRET',
    'x-scry-backup-key': 'operations/scry/production-20261016/backup-trigger',
}


class BackupError(Exception):
    """A fixed, secret-free diagnostic suitable for the journal."""


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


urlopen = urllib.request.build_opener(NoRedirect()).open


def canonical_origin():
    try:
        with CONFIG.open('rb') as stream:
            origin = tomllib.load(stream)['vars']['CANONICAL_ORIGIN']
    except (OSError, ValueError, KeyError, TypeError):
        raise BackupError('production config unreadable') from None
    try:
        parsed = urllib.parse.urlsplit(origin)
        valid = (isinstance(origin, str) and origin.isascii()
                 and not any(ord(c) <= 32 or ord(c) == 127 for c in origin)
                 and parsed.scheme == 'https' and parsed.hostname
                 and not parsed.username and not parsed.password
                 and parsed.path in ('', '/') and not parsed.query and not parsed.fragment)
    except (ValueError, TypeError, AttributeError):
        valid = False
    if not valid:
        raise BackupError('canonical origin invalid')
    return origin.rstrip('/')


def run():
    origin = canonical_origin()
    headers = {'User-Agent': 'ScryDailyBackup/1.0'}
    for header, entry in PASS_HEADERS.items():
        try:
            value = subprocess.check_output(['pass', 'show', entry],
                stderr=subprocess.DEVNULL, timeout=30).decode().rstrip('\r\n')
        except (OSError, subprocess.SubprocessError):
            raise BackupError('credential lookup failed') from None
        except UnicodeError:
            raise BackupError('credential invalid') from None
        if len(value) < 20 or not value.isascii() or any(ord(c) <= 32 or ord(c) == 127 for c in value):
            raise BackupError('credential invalid')
        headers[header] = value
    request = urllib.request.Request(origin + '/internal/backup', data=b'', headers=headers, method='POST')
    with urlopen(request, timeout=30) as response:
        if response.status != 200:
            raise BackupError('HTTP ' + str(response.status))
        if not response.read(64).startswith(b'Snapshot verified.'):
            raise BackupError('verification response missing')


def send_checkin(checkin_id, status, *, duration=None):
    # Reuse the Worker's public ingest identifier without a second credential or copy.
    dsn = re.search(r'^const DSN = "([^"]+)";$', SENTRY_TRANSPORT.read_text(), re.MULTILINE)[1]
    parsed = urllib.parse.urlsplit(dsn)
    payload = {
        'check_in_id': checkin_id,
        'monitor_slug': 'scry-mirrodin-daily-backup',
        'status': status,
        'environment': 'production',
        'release': 'scry-daily-backup@' + hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
    }
    if status == 'in_progress':
        payload['monitor_config'] = {
            'schedule': {'type': 'crontab', 'value': '30 9 * * *'},
            'timezone': 'UTC', 'checkin_margin': 15, 'max_runtime': 3,
            'failure_issue_threshold': 1, 'recovery_threshold': 1,
        }
    else:
        payload['duration'] = duration
    body = '\n'.join(json.dumps(item) for item in ({'dsn': dsn}, {'type': 'check_in'}, payload)) + '\n'
    request = urllib.request.Request(f'https://{parsed.hostname}/api{parsed.path}/envelope/',
        data=body.encode(), method='POST',
        headers={'Content-Type': 'application/x-sentry-envelope', 'User-Agent': 'ScryDailyBackup/1.0'})
    with urlopen(request, timeout=2) as response:
        if response.status != 200:
            raise BackupError('monitoring check-in rejected')


def report_checkin(*args, **kwargs):
    try:
        send_checkin(*args, **kwargs)
        return True
    except Exception as error:
        if isinstance(error, urllib.error.HTTPError):
            error.close()
        print('Scry backup monitoring unavailable.', file=sys.stderr)
        return False


def main():
    checkin_id = uuid.uuid4().hex
    started = time.monotonic()
    monitoring_ok = report_checkin(checkin_id, 'in_progress')
    result = 0
    try:
        run()
    except Exception as error:
        if isinstance(error, urllib.error.HTTPError):
            reason = ('redirect refused: HTTP ' if 300 <= error.code < 400 else 'HTTP ') + str(error.code)
            error.close()
        elif isinstance(error, BackupError):
            reason = str(error)
        elif isinstance(error, (urllib.error.URLError, OSError)):
            reason = 'network request failed'
        else:
            reason = 'unexpected failure'
        print('Scry backup failed: ' + reason + '.', file=sys.stderr)
        result = 1
    else:
        print('Scry production snapshot verified.')
    monitoring_ok = report_checkin(checkin_id, 'error' if result else 'ok',
        duration=time.monotonic() - started) and monitoring_ok
    return result or int(not monitoring_ok)


if __name__ == '__main__':
    sys.exit(main())
