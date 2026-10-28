#!/usr/bin/env python3
"""Cloud-only launch backup gate. Never exports a learner archive or uploads production code."""
from datetime import datetime, timedelta, timezone
import email.parser
import email.policy
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tomllib
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[1]
MAX_ARCHIVE = 16 * 1024 * 1024
MAX_PAGES = 10
WINDOW = timedelta(minutes=55)
EVENT_FIELDS = {'cron', 'datetime', 'scheduledDatetime', 'scriptName', 'status'}
FILTER_FIELDS = {'scriptName', 'datetime_geq', 'datetime_leq'}
# Exact inspected v0.8.1 and NATO-master backup/retention/daily/scheduled code.
# A successful invocation has no such meaning for an arbitrary green release.
BACKUP_CONTRACT = '93c95a164ecf450f634f400531910ff0644a556d05876b2d0bb9c9f3d87c7eb0'
REVIEWED_BACKUP_SOURCES = {
    '62a7716e6f619c10fbda60630d86461ede33d8b7',
    'f254b7e2a6ddf2e4f080130cd8127781846fbe3b',
}
APPROVAL_PATH = 'deploy/cloud-recovery-approval.json'
RELEASE_METADATA = {'Cargo.toml', 'Cargo.lock', 'package.json', 'package-lock.json',
                    '.release-please-manifest.json', 'CHANGELOG.md'}
SCHEMA_QUERY = '''query {
 row:__type(name:"AccountWorkersInvocationsScheduled") {fields{name}}
 filt:__type(name:"AccountWorkersInvocationsScheduledFilter_InputObject") {inputFields{name}}
}'''


class GateError(Exception):
    """Fixed content-free diagnostics only; never include provider bodies or object keys."""


def require(condition, reason):
    if not condition:
        raise GateError(reason)


def stamp(value):
    try:
        result = datetime.fromisoformat(value.replace('Z', '+00:00'))
        require(result.tzinfo is not None, 'invalid provider timestamp')
        return result.astimezone(timezone.utc)
    except (ValueError, TypeError, AttributeError):
        raise GateError('invalid provider timestamp') from None


def iso(value):
    return value.astimezone(timezone.utc).isoformat().replace('+00:00', 'Z')


def bootstrap_slot(now):
    # Cron propagation can take 15 minutes. One dated slot, not a frequent cron.
    when = (now + timedelta(minutes=20)).replace(second=0, microsecond=0)
    return f'{when.minute} {when.hour} {when.day} {when.month} *', when


def no_paid_work(bindings):
    # Fail closed even if a provider's model/URL is currently absent: rearming
    # old-code alarms must not grant a paid capability through racing config.
    require(not {'OPENROUTER_API_KEY', 'JEV_API_KEY'} & {b['name'] for b in bindings},
            'provider-enabled production cannot bootstrap without separate authorization')


def validate_objects(objects, prefix, now):
    require(isinstance(objects, list) and len(objects) <= MAX_PAGES * 1000, 'invalid backup listing')
    seen = set()
    for item in objects:
        key = item.get('key')
        require(isinstance(key, str) and key.startswith(prefix) and key not in seen, 'invalid backup listing')
        seen.add(key)
        require(type(item.get('size')) is int and 0 < item['size'] <= MAX_ARCHIVE, 'backup size exceeds bounded recovery')
        require(isinstance(item.get('etag'), str) and bool(item['etag']), 'backup ETag missing')
        require(item.get('storage_class') == 'Standard', 'non-Standard backup storage is not admitted')
        uploaded = stamp(item.get('last_modified'))
        require(uploaded <= now, 'future backup timestamp')
        # Include the whole bootstrap window: no object may age into the
        # existing runtime's destructive >30-day retention path while waiting.
        require(uploaded > now + WINDOW - timedelta(days=30), 'unapproved retention deletion candidate')
    return objects


def list_objects(request, bucket, prefix, now):
    objects, cursors, cursor = [], set(), None
    for _ in range(MAX_PAGES):
        params = {'per_page': 1000, 'prefix': prefix}
        if cursor is not None:
            params['cursor'] = cursor
        body = request('/r2/buckets/' + bucket + '/objects?' + urllib.parse.urlencode(params), envelope=True)
        # REST List Objects uses the SDK's cursor pagination contract: both
        # result_info and is_truncated are optional, unlike Workers R2.list().
        # https://github.com/cloudflare/cloudflare-python/blob/main/src/cloudflare/pagination.py
        info = body.get('result_info')
        if info is None:
            info = {}
        require(isinstance(info, dict), 'invalid backup pagination metadata')
        next_cursor = info.get('cursor')
        require(next_cursor is None or isinstance(next_cursor, str), 'invalid backup pagination cursor')
        if 'is_truncated' in info:
            require(type(info['is_truncated']) is bool
                    and info['is_truncated'] == bool(next_cursor), 'contradictory backup pagination metadata')
        require(isinstance(body.get('result'), list), 'invalid backup listing')
        objects.extend(body['result'])
        if not next_cursor:
            return validate_objects(objects, prefix, now)
        cursor = next_cursor
        require(cursor not in cursors, 'ambiguous backup pagination')
        cursors.add(cursor)
    raise GateError('backup pagination exceeds bounded admission')


def require_schema(body):
    require(not body.get('errors'), 'Cron analytics permission or schema unavailable')
    data = body.get('data') or {}
    row, filt = data.get('row') or {}, data.get('filt') or {}
    require(EVENT_FIELDS <= {x.get('name') for x in row.get('fields', [])}
            and FILTER_FIELDS <= {x.get('name') for x in filt.get('inputFields', [])},
            'Cron analytics schema is unverified')


def cron_success(events, worker, cron, when):
    require(isinstance(events, list) and len(events) < 100, 'Cron analytics result truncated')
    matched = []
    for event in events:
        require(isinstance(event, dict) and EVENT_FIELDS <= event.keys(), 'invalid Cron analytics event')
        if event['scriptName'] == worker and event['cron'] == cron and stamp(event['scheduledDatetime']) == when:
            matched.append(event)
    require(len(matched) <= 1, 'ambiguous Cron invocation')
    return bool(matched and matched[0]['status'] == 'success'
                and when <= stamp(matched[0]['datetime']) <= when + WINDOW)


def events_query(account, worker, start, end):
    # JSON quoted literals prevent provider identifiers becoming GraphQL syntax.
    q = json.dumps
    return ('query {viewer{accounts(filter:{accountTag:' + q(account) + '}){'
            'workersInvocationsScheduled(limit:100,filter:{scriptName:' + q(worker)
            + ',datetime_geq:' + q(iso(start)) + ',datetime_leq:' + q(iso(end))
            + '}){cron datetime scheduledDatetime scriptName status}}}}')


def event_rows(body):
    require(not body.get('errors'), 'Cron analytics permission or query unavailable')
    accounts = (body.get('data') or {}).get('viewer', {}).get('accounts')
    require(isinstance(accounts, list) and len(accounts) == 1, 'Cron analytics account unavailable')
    events = accounts[0].get('workersInvocationsScheduled')
    require(isinstance(events, list) and len(events) < 100, 'Cron analytics result truncated')
    return events


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


class Cloudflare:
    """Bounded JSON/module control-plane transport; no learner-object GET API."""
    def __init__(self, account, token):
        self.account, self.token = account, token
        self.calls, self.limit = 0, 480
        self.claim_path = None
        self.open = urllib.request.build_opener(NoRedirect()).open

    def allow_claim(self, path):
        require(re.fullmatch(r'/r2/buckets/[a-z0-9-]+/objects/operations/recovery-claims/[a-f0-9]{32}\.json', path),
                'invalid content-free approval claim path')
        require(self.claim_path is None or self.claim_path == path, 'approval claim path changed')
        self.claim_path = path

    def request(self, path, *, method='GET', payload=None, body=None, headers=None,
                envelope=False, raw=False, graphql=False, absent=False, expected_status=None):
        self.calls += 1
        require(self.calls <= self.limit, 'cloud control-plane request budget exhausted')
        require(path.startswith('/'), 'invalid control-plane path')
        if '/objects/' in path:
            temporary_delete = method == 'DELETE' and re.fullmatch(
                r'/r2/buckets/scry-recovery-([a-f0-9]{24})-data/objects/(?:photos|snapshots)/recovery-\1/[a-zA-Z0-9.-]+', path)
            claim = path == self.claim_path and method in ('GET', 'PUT', 'DELETE')
            require(temporary_delete or claim, 'learner object download or unowned mutation is forbidden')
        url = 'https://api.cloudflare.com/client/v4' + ('/graphql' if graphql else '/accounts/' + self.account + path)
        require(payload is None or body is None, 'ambiguous control-plane body')
        data = body if body is not None else None if payload is None else json.dumps(payload).encode()
        extra = headers or {}
        require(not any(k.lower() in ('authorization', 'host') for k in extra), 'control-plane authority override refused')
        req = urllib.request.Request(url, data=data, method=method,
                                     headers={'Authorization': 'Bearer ' + self.token, 'Content-Type': 'application/json', **extra})
        try:
            with self.open(req, timeout=30) as response:
                require(expected_status is None or response.status == expected_status, 'cloud control-plane response status unverified')
                limit = 1024 if path == self.claim_path else 24 * 1024 * 1024 if raw else 4 * 1024 * 1024
                body = response.read(limit + 1)
                require(len(body) <= limit, 'provider response exceeded bounded read')
                if raw:
                    return body, response.headers['Content-Type']
                result = json.loads(body)
        except urllib.error.HTTPError as error:
            status = error.code
            error.close()
            if absent and status == 404:
                return None
            raise GateError('cloud control-plane request unavailable') from None
        except (urllib.error.URLError, OSError, ValueError):
            raise GateError('cloud control-plane request unavailable') from None
        if graphql:
            return result
        require(result.get('success') is True, 'cloud control-plane request refused')
        return result if envelope else result['result']

    def graphql(self, query):
        return self.request('/graphql', method='POST', payload={'query': query}, graphql=True)


def module_hashes(raw, kind):
    parts = email.parser.BytesParser(policy=email.policy.default).parsebytes(
        ('Content-Type: ' + kind + '\r\n\r\n').encode() + raw)
    result = {}
    for part in parts.iter_parts():
        name = part.get_filename() or part.get_param('name', header='Content-Disposition')
        require(isinstance(name, str) and Path(name).name == name and name not in result, 'ambiguous live module identity')
        result[name] = hashlib.sha256(part.get_payload(decode=True)).hexdigest()
    require(set(result) == {'index.js', 'index_bg.wasm'}, 'unexpected live module set')
    return result


def qualified_modules(promote, source):
    """Read only public exact-source CI bytes, never a learner archive."""
    runs = json.loads(promote.gh('actions/workflows/ci.yml/runs?' + urllib.parse.urlencode({
        'head_sha': source, 'branch': 'master', 'event': 'push', 'status': 'success'})))['workflow_runs']
    runs = [run for run in runs if run.get('head_sha') == source and run.get('event') == 'push'
            and run.get('head_branch') == 'master' and run.get('conclusion') == 'success'
            and run.get('path') == '.github/workflows/ci.yml']
    require(bool(runs), 'live source lacks successful exact master CI')
    artifacts = json.loads(promote.gh(f'actions/runs/{runs[0]["id"]}/artifacts'))['artifacts']
    matches = [a for a in artifacts if a.get('name') == 'scry-worker-' + source and not a.get('expired')]
    require(len(matches) == 1, 'live source artifact unavailable or ambiguous')
    artifact = matches[0]
    require(type(artifact.get('size_in_bytes')) is int and artifact['size_in_bytes'] <= 256 * 1024 * 1024,
            'live source artifact exceeds bounded read')
    raw = promote.gh(f'actions/artifacts/{artifact["id"]}/zip')
    require('sha256:' + hashlib.sha256(raw).hexdigest() == artifact.get('digest'), 'live source artifact digest mismatch')
    with zipfile.ZipFile(io.BytesIO(raw)) as archive:
        for name in ('proof.json', 'worker/index.js', 'worker/index_bg.wasm'):
            entries = [i for i in archive.infolist() if i.filename == name]
            require(len(entries) == 1 and entries[0].file_size <= 24 * 1024 * 1024, 'invalid live artifact member')
        proof = json.loads(archive.read('proof.json'))
        require(proof.get('source_state') == 'committed' and proof.get('revision') == source, 'live artifact provenance differs')
        result = {name: hashlib.sha256(archive.read('worker/' + name)).hexdigest() for name in ('index.js', 'index_bg.wasm')}
        require(result == {name: proof['worker'][name] for name in result}, 'live artifact module checksum differs')
        return result


def require_backup_contract(source):
    require(source in REVIEWED_BACKUP_SOURCES, 'live backup acknowledgment source is not reviewed')
    try:
        runtime = subprocess.check_output(['git', 'show', source + ':src/runtime.rs'], cwd=ROOT,
                                          stderr=subprocess.DEVNULL, text=True)
        daily = runtime.index('        if path == "/__internal/daily" {')
        pieces = [runtime[runtime.index('    async fn backup(&self, pre_release: bool)'):
                          runtime.index('    async fn restore(&self, req: Request)')],
                  runtime[runtime.index("// worker 0.8.7's scheduled macro"):],
                  runtime[daily:runtime.index('        let url = req.url()?;', daily)]]
        require(hashlib.sha256(''.join(pieces).encode()).hexdigest() == BACKUP_CONTRACT,
                'live backup acknowledgment contract is not reviewed')
    except (ValueError, subprocess.SubprocessError, OSError):
        raise GateError('live backup acknowledgment contract is not reviewed') from None


def cleanup_schedule(client, config, receipt_path):
    """Resume exact run-owned cleanup, including after the deploy process died."""
    path = Path(receipt_path)
    if not path.exists():
        return {'schedule_cleanup': 'not_created'}
    saved = json.loads(path.read_text())
    require(saved.get('format') == 'scry-bootstrap-schedule-v1'
            and saved.get('account_id') == config['account_id']
            and saved.get('worker') == config['name']
            and saved.get('native_run_id') == os.environ.get('GITHUB_RUN_ID')
            and saved.get('original_crons') == [], 'bootstrap cleanup receipt identity differs')
    when = stamp(saved.get('scheduled_at'))
    cron = f'{when.minute} {when.hour} {when.day} {when.month} *'
    require(saved.get('cron') == cron, 'bootstrap cleanup receipt identity differs')
    script = '/workers/scripts/' + config['name'] + '/schedules'
    try:
        current = client.request(script)['schedules']
        if current:
            require([s.get('cron') for s in current] == [cron],
                    'bootstrap schedule changed; cleanup requires operator inspection')
            # A preceding uncertain PUT is inspected above, never blindly replayed.
            client.request(script, method='PUT', payload=[])
            require(client.request(script)['schedules'] == [], 'bootstrap schedule cleanup unverified')
        saved['status'] = 'verified_absent'
        path.write_text(json.dumps(saved, sort_keys=True) + '\n')
        return {'schedule_cleanup': 'verified_absent'}
    except Exception as error:
        saved['status'] = 'cleanup_pending'
        path.write_text(json.dumps(saved, sort_keys=True) + '\n')
        error.schedule_cleanup = {'worker': config['name'], 'cron': cron,
                                  'original_crons': [], 'status': 'cleanup_pending',
                                  'expires_automatically': False,
                                  'storage_only_allowance_applies': False}
        raise


def bootstrap(client, config, baseline, verify_unchanged, *, now=None, sleep=time.sleep, clock=None,
              receipt_path=None):
    """One-slot schedule only; preserve the existing autonomous 24-hour DO cadence."""
    clock = clock or (lambda: datetime.now(timezone.utc))
    now = now or clock()
    script = '/workers/scripts/' + config['name']
    previous = client.request(script + '/schedules')['schedules']
    require(previous == [], 'unexpected production schedule; do not replace operator configuration')
    require_schema(client.graphql(SCHEMA_QUERY))
    # Validate the query and permission before the first mutation, not after it.
    event_rows(client.graphql(events_query(config['account_id'], config['name'], now - timedelta(minutes=1), now)))
    verify_unchanged()
    # The native production concurrency group is the single writer. Cloudflare
    # exposes no conditional schedule PUT; refuse observed outside drift as
    # close to the write as possible rather than overwriting a new schedule.
    require(client.request(script + '/schedules')['schedules'] == previous,
            'production schedule changed before bootstrap')
    cron, when = bootstrap_slot(now)
    desired = [{'cron': cron}]
    path = Path(receipt_path) if receipt_path else ROOT / 'target/recovery-schedule.json'
    require(os.environ.get('GITHUB_RUN_ID', '').isdigit(), 'native run identity unavailable')
    path.parent.mkdir(parents=True, exist_ok=True)
    # Persist before PUT: a killed deploy process must leave enough exact intent
    # for the independent always() cleanup step, without any learner identifiers.
    with path.open('x') as output:
        json.dump({'format': 'scry-bootstrap-schedule-v1', 'account_id': config['account_id'],
                   'worker': config['name'], 'native_run_id': os.environ['GITHUB_RUN_ID'],
                   'original_crons': [], 'cron': cron, 'scheduled_at': iso(when),
                   'status': 'cleanup_pending', 'expires_automatically': False,
                   'storage_only_allowance_applies': False}, output, sort_keys=True)
        output.flush()
        os.fsync(output.fileno())
    changed = False
    try:
        # An uncertain PUT is not retried: finally reads once and only removes
        # our exact one-slot schedule, never an operator's concurrent change.
        changed = True
        client.request(script + '/schedules', method='PUT', payload=desired)
        readback = client.request(script + '/schedules')['schedules']
        require([s.get('cron') for s in readback] == [cron], 'bootstrap schedule readback differs')
        while clock() < now + WINDOW:
            verify_unchanged()
            rows = event_rows(client.graphql(events_query(config['account_id'], config['name'], now, clock())))
            if cron_success(rows, config['name'], cron, when):
                verify_unchanged()
                return {'format': 'scry-cloud-backup-v1', 'scheduled_at': iso(when),
                        'provider_scheduled_success': True, 'live_version': baseline,
                        'full_readback_via_verified_runtime': True, 'learner_bodies_exported': False,
                        'existing_24h_alarm_rearmed': True, 'existing_30_day_retention_unchanged': True,
                        'natural_next_day_run_observed': False}
            sleep(60)
        raise GateError('no successful provider Cron event within bootstrap window')
    finally:
        if changed:
            cleanup_schedule(client, config, path)


def load_module(name):
    spec = importlib.util.spec_from_file_location(name.replace('-', '_'), ROOT / 'scripts' / (name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def metadata_change(path, before, after, version):
    """Only release-please's existing fields; dependencies and scripts stay exact."""
    if path in ('package.json', 'package-lock.json', '.release-please-manifest.json'):
        old, new = json.loads(before), json.loads(after)
        fields = [('.',)] if path == '.release-please-manifest.json' else [('version',)]
        if path == 'package-lock.json': fields.append(('packages', '', 'version'))
        for fields_path in fields:
            a, b = old, new
            for key in fields_path[:-1]: a, b = a[key], b[key]
            field = fields_path[-1]
            require(isinstance(a[field], str) and b[field] == version, 'release metadata version differs')
            a[field] = b[field] = 'approved-version-only'
        require(old == new, 'release metadata changed non-version content')
        return
    if path in ('Cargo.toml', 'Cargo.lock'):
        old, new = tomllib.loads(before), tomllib.loads(after)
        if path == 'Cargo.toml':
            require(old['package']['name'] == new['package']['name'] == 'scry', 'release package changed')
            a, b = old['package'], new['package']
        else:
            a = [p for p in old['package'] if p['name'] == 'scry']
            b = [p for p in new['package'] if p['name'] == 'scry']
            require(len(a) == len(b) == 1, 'release lock package ambiguous')
            a, b = a[0], b[0]
        require(isinstance(a['version'], str) and b['version'] == version, 'release metadata version differs')
        a['version'] = b['version'] = 'approved-version-only'
        require(old == new, 'release metadata changed non-version content')
        return
    header = '# Changelog\n\n'
    require(path == 'CHANGELOG.md' and before.startswith(header) and after.startswith(header),
            'release changelog header changed')
    before, after = before[len(header):], after[len(header):]
    require(after.endswith(before), 'release changelog rewrote prior history')
    prefix = after[:-len(before)] if before else after
    # Existing release notes are public release input; only a bounded prepend
    # for this precise version is allowed, never product/script file drift.
    require(len(prefix.encode()) <= 128 * 1024 and re.search(r'^## \[?' + re.escape(version) + r'\]?(?:\s|\()', prefix, re.M),
            'release changelog does not name approved version')


def verify_reviewed_source(approval, receipt):
    baseline, source, version = approval.get('reviewed_source'), receipt.get('source'), approval.get('release_version')
    require(isinstance(baseline, str) and re.fullmatch('[a-f0-9]{40}', baseline)
            and isinstance(source, str) and re.fullmatch('[a-f0-9]{40}', source)
            and isinstance(version, str) and re.fullmatch(r'\d+\.\d+\.\d+', version), 'approval source binding missing')
    def git(*args):
        return subprocess.check_output(['git', *args], cwd=ROOT, stderr=subprocess.DEVNULL)
    def tree(revision):
        entries = {}
        for line in git('ls-tree', '-rz', '--full-tree', revision).split(b'\0'):
            if not line: continue
            meta, path = line.split(b'\t', 1)
            entries[path.decode()] = meta.decode().split()
        return entries
    try:
        old, new = tree(baseline), tree(source)
        require(set(old) - {APPROVAL_PATH} == set(new) - {APPROVAL_PATH}, 'approved source file inventory changed')
        for path in new:
            if path == APPROVAL_PATH:
                require(new[path][:2] == ['100644', 'blob'], 'approval file must be ordinary data')
                continue
            require(old[path][:2] == new[path][:2], 'approved source mode changed')
            if path in RELEASE_METADATA - {'CHANGELOG.md'}:
                metadata_change(path, git('show', baseline + ':' + path).decode(), git('show', source + ':' + path).decode(), version)
            elif old[path] != new[path]:
                require(path in RELEASE_METADATA, 'approved product or operational source changed')
                metadata_change(path, git('show', baseline + ':' + path).decode(), git('show', source + ':' + path).decode(), version)
        package = json.loads(git('show', source + ':package.json'))
        manifest = json.loads(git('show', source + ':.release-please-manifest.json'))
        require(package['version'] == manifest['.'] == version, 'source is not the approved release version')
    except (subprocess.SubprocessError, OSError, ValueError, KeyError, TypeError):
        raise GateError('approval source binding could not be verified') from None


def observed_backup_window(approval, checked):
    """A fixed already-observed upload window, never permission for another backup."""
    values = [approval.get(key) for key in ('accounting_started_ms', 'backup_observed_after_ms',
                                           'backup_observed_before_ms')]
    require(all(type(value) is int and value > 0 for value in values), 'observed backup window is unverified')
    accounting, start, end = [datetime.fromtimestamp(value / 1000, timezone.utc) for value in values]
    require(accounting <= start < end <= checked and end - start <= timedelta(hours=1),
            'observed backup window is unverified')
    return start, end


def observed_backup(objects, window, checked):
    start, end = window
    matches = [item for item in objects if start <= stamp(item['last_modified']) <= end]
    require(len(matches) == 1, 'observed backup is absent or ambiguous')
    require(timedelta(0) <= checked - stamp(matches[0]['last_modified']) <= timedelta(hours=26),
            'observed backup is not fresh')
    return matches[0]


def gate(promote, receipt, config, before, deployments, prior, *, after_restore):
    """Restore old-code backup before upload; prove persisted recurrence after it."""
    recovery = load_module('scry-cloud-recovery')
    # Operational approval is absent by default. Cost admission must happen
    # before even the potentially billable R2 metadata listing.
    approval = json.loads((ROOT / APPROVAL_PATH).read_text())
    require(approval.get('approved') is True, 'one-operation cloud recovery approval is absent')
    require(callable(after_restore), 'native deployment callback is unavailable')
    require(approval.get('production_health_service') == config['name'],
            'temporary production health service access is not approved')
    verify_reviewed_source(approval, receipt)
    approval = recovery.admit(receipt, approval=approval)
    window = observed_backup_window(approval, datetime.now(timezone.utc))
    require(prior is not None, 'cloud bootstrap requires recognized live source')
    no_paid_work(before['bindings'])
    require_backup_contract(prior[0])
    live = promote.single_version(deployments[0])
    client = Cloudflare(config['account_id'], os.environ['CLOUDFLARE_API_TOKEN'])
    require(before.get('usage_model') == 'standard'
            and client.request('/workers/account-settings').get('default_usage_model') == 'standard',
            'existing Workers Standard pricing model is unverified')
    billing_period = recovery.verify_billing_period(client.request)
    bucket = next(b['bucket_name'] for b in before['bindings'] if b['name'] == 'SCRY_BACKUPS')
    script = '/workers/scripts/' + config['name']
    expected = qualified_modules(promote, prior[0])
    require(module_hashes(*client.request(script + '/content/v2', raw=True)) == expected,
            'live modules differ from qualified source; refusing schedule')
    # Source metadata means nothing if a different version is deployed. Pin
    # settings, native deployment, and serving edges across the whole window.
    edges = lambda: promote.edges(client.request('/workers/domains?service=' + config['name']),
                                  client.request(script + '/routes'), config['name'])
    serving = edges()
    def unchanged():
        current = client.request(script + '/deployments')['deployments'][0]
        require(current == deployments[0] and promote.single_version(current) == live, 'live deployment changed during backup gate')
        require(client.request(script + '/settings') == before, 'live settings changed during backup gate')
        require(edges() == serving, 'serving edges changed during backup gate')
    # Cron analytics returned no events despite an observed new upload. Do not
    # schedule again or treat metadata as success: the pinned archive must pass
    # the complete independent restore before the callback can deploy anything.
    unchanged()
    recovery.claim(approval, receipt, client.request, bucket, billing_period=billing_period)
    prefix = 'snapshots/' + config['vars']['SPACE_NAME'] + '/'
    started = datetime.now(timezone.utc)
    objects = list_objects(client.request, bucket, prefix, started)
    source = observed_backup(objects, window, started)
    unchanged()
    def deploy_verified():
        unchanged()
        after_restore()
    restored = recovery.run(config, promote.OUT, source, client.request, approval=approval, receipt=receipt,
                            billing_period=billing_period, after_restore=deploy_verified)
    require(restored.get('proof', {}).get('canonical_application_equal') is True
            and restored.get('recurrence_verified') is True and restored.get('cleanup_verified') is True,
            'independent recovery or temporary cleanup unverified')
    proof = {'format': 'scry-cloud-backup-v2', 'existing_observed_snapshot': True,
             'full_readback_via_independent_restore': True, 'persisted_alarm_verified': True,
             'learner_bodies_exported': False, 'new_production_backup_triggered': False,
             'existing_30_day_retention_unchanged': True, 'natural_next_day_run_observed': False}
    return {'backup': proof, 'recovery': restored}


def cleanup_claim():
    config = tomllib.loads((ROOT / 'deploy/rust-production.toml').read_text())
    client = Cloudflare(config['account_id'], os.environ['CLOUDFLARE_API_TOKEN'])
    pending = None
    # Production Cron cleanup has priority over waiting for claim expiry. Its
    # annual recurrence is NOT covered by the temporary storage-only allowance.
    try:
        print(json.dumps(cleanup_schedule(client, config, ROOT / 'target/recovery-schedule.json'), sort_keys=True))
    except Exception as error:
        pending = error
        print(json.dumps({'schedule_cleanup': 'cleanup_pending', 'expires_automatically': False,
                          'storage_only_allowance_applies': False}, sort_keys=True), flush=True)
    path = ROOT / 'target/recovery-claim.json'
    if path.exists():
        print(json.dumps(load_module('scry-cloud-recovery').cleanup_claim(path, client.request), sort_keys=True))
    if pending is not None:
        raise pending


if __name__ == '__main__':
    import sys
    if sys.argv[1:] != ['cleanup-claim']:
        raise SystemExit('Use the native promoter; the only standalone operation is cleanup-claim.')
    cleanup_claim()
