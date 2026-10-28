#!/usr/bin/env python3
"""Cloud backup gate negative controls; no network, credentials or learner data."""
from copy import deepcopy
from datetime import datetime, timedelta, timezone
import importlib.util
import io
import json
import os
from pathlib import Path
from types import SimpleNamespace
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
        for response in ({'result': [obj(1)], 'result_info': {'is_truncated': True}},
                         {'result': [obj(1)], 'result_info': {'is_truncated': 'false'}}):
            with self.subTest(response=response), self.assertRaises(BACKUP.GateError):
                BACKUP.list_objects(lambda *a, **k: response, 'bucket', 'snapshots/synthetic/', NOW)
        with self.assertRaisesRegex(BACKUP.GateError, 'pagination'):
            BACKUP.list_objects(lambda *a, **k: {'result': [], 'result_info': {'is_truncated': True, 'cursor': 'same'}},
                                'bucket', 'snapshots/synthetic/', NOW)

    def test_rest_listing_accepts_optional_metadata_and_follows_cursor_contract(self):
        obj = lambda n: {'key': f'snapshots/synthetic/{n}', 'size': 10, 'etag': 'a' * 32,
                         'last_modified': '2026-10-27T19:00:00Z', 'storage_class': 'Standard'}
        for info in (None, {}, {'cursor': None}, {'cursor': ''}, {'is_truncated': False}):
            response = {'result': [obj(1)], 'result_info': info}
            with self.subTest(info=info):
                self.assertEqual(BACKUP.list_objects(lambda *a, **k: response, 'bucket', 'snapshots/synthetic/', NOW), [obj(1)])
        # Actual observed shape: the response omits result_info entirely.
        self.assertEqual(BACKUP.list_objects(lambda *a, **k: {'result': [obj(1)]}, 'bucket', 'snapshots/synthetic/', NOW), [obj(1)])
        pages = iter([{'result': [obj(1)], 'result_info': {'cursor': 'synthetic-next'}}, {'result': [obj(2)]}])
        calls = []
        def request(path, **kwargs):
            calls.append(path)
            return next(pages)
        self.assertEqual(BACKUP.list_objects(request, 'bucket', 'snapshots/synthetic/', NOW), [obj(1), obj(2)])
        self.assertIn('cursor=synthetic-next', calls[1])
        for info in ([], 'invalid', {'cursor': 1}, {'is_truncated': None}, {'is_truncated': 'false'},
                     {'is_truncated': True}, {'is_truncated': True, 'cursor': ''},
                     {'is_truncated': False, 'cursor': 'contradiction'}):
            with self.subTest(info=info), self.assertRaises(BACKUP.GateError):
                BACKUP.list_objects(lambda *a, **k: {'result': [obj(1)], 'result_info': info}, 'bucket', 'snapshots/synthetic/', NOW)
        calls = []
        def endless(path, **kwargs):
            calls.append(path)
            return {'result': [obj(len(calls))], 'result_info': {'cursor': str(len(calls))}}
        with self.assertRaisesRegex(BACKUP.GateError, 'bounded admission'):
            BACKUP.list_objects(endless, 'bucket', 'snapshots/synthetic/', NOW)
        self.assertEqual(len(calls), BACKUP.MAX_PAGES)

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

    def test_transport_requires_exact_requested_http_status(self):
        client=BACKUP.Cloudflare('a'*32,'synthetic-only')
        class Response:
            status=201
            def __enter__(self):return self
            def __exit__(self,*args):pass
            def read(self,size):return b'{"success":true,"result":{"observability":null,"logpush":false}}'
        with mock.patch.object(client,'open',return_value=Response()):
            with self.assertRaises(BACKUP.GateError):client.request('/workers/scripts/synthetic/script-settings',expected_status=200)
            Response.status=200
            self.assertIsNone(client.request('/workers/scripts/synthetic/script-settings',expected_status=200)['observability'])

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


class ObservedSnapshot(unittest.TestCase):
    def setUp(self):
        self.approval = {'accounting_started_ms': int((NOW - timedelta(hours=1)).timestamp() * 1000),
                         'backup_observed_after_ms': int(NOW.timestamp() * 1000),
                         'backup_observed_before_ms': int((NOW + timedelta(minutes=55)).timestamp() * 1000)}
        self.checked = NOW + timedelta(hours=1)
        self.obj = {'key': 'snapshots/synthetic/1-daily-one.json', 'size': 10,
                    'etag': 'a' * 32, 'last_modified': BACKUP.iso(NOW + timedelta(minutes=20)),
                    'storage_class': 'Standard'}

    def test_selects_exact_unique_observed_upload_without_cron_or_health_success(self):
        earlier = self.obj | {'key': 'snapshots/synthetic/old', 'last_modified': BACKUP.iso(NOW - timedelta(minutes=1))}
        window = BACKUP.observed_backup_window(self.approval, self.checked)
        self.assertEqual(BACKUP.observed_backup([earlier, self.obj], window, self.checked), self.obj)
        for objects in ([], [earlier], [self.obj, self.obj | {'key': 'snapshots/synthetic/second'}]):
            with self.subTest(objects=len(objects)), self.assertRaises(BACKUP.GateError):
                BACKUP.observed_backup(objects, window, self.checked)
        with self.assertRaisesRegex(BACKUP.GateError, 'fresh'):
            BACKUP.observed_backup([self.obj], window, NOW + timedelta(hours=27))

    def test_observed_window_is_exact_bounded_and_inside_original_accounting(self):
        for field, value in [('backup_observed_after_ms', None), ('backup_observed_before_ms', True),
                             ('accounting_started_ms', 0), ('backup_observed_after_ms', 1),
                             ('backup_observed_before_ms', int((NOW + timedelta(hours=2)).timestamp() * 1000)),
                             ('backup_observed_before_ms', int((NOW - timedelta(seconds=1)).timestamp() * 1000))]:
            with self.subTest(field=field, value=value), self.assertRaises(BACKUP.GateError):
                BACKUP.observed_backup_window(self.approval | {field: value}, self.checked)
        for field in self.approval:
            changed = dict(self.approval); del changed[field]
            with self.subTest(missing=field), self.assertRaises(BACKUP.GateError):
                BACKUP.observed_backup_window(changed, self.checked)


class ObservedGateLifecycle(unittest.TestCase):
    def test_independent_restore_precedes_upload_and_recurrence_without_cron_requests(self):
        checked = NOW + timedelta(hours=1)
        approval = {'approved': True, 'production_health_service': 'synthetic-worker',
                    'accounting_started_ms': int((NOW - timedelta(hours=1)).timestamp() * 1000),
                    'backup_observed_after_ms': int(NOW.timestamp() * 1000),
                    'backup_observed_before_ms': int((NOW + timedelta(minutes=55)).timestamp() * 1000)}
        config = {'name': 'synthetic-worker', 'account_id': 'a' * 32, 'vars': {'SPACE_NAME': 'synthetic'}}
        before = {'usage_model': 'standard', 'bindings': [{'name': 'SCRY_BACKUPS', 'bucket_name': 'synthetic-backups'}]}
        deployment = {'id': 'old-live', 'versions': [{'version_id': 'old-live', 'percentage': 100}]}
        obj = {'key': 'snapshots/synthetic/1-daily-one.json', 'size': 10, 'etag': 'a' * 32,
               'last_modified': BACKUP.iso(NOW + timedelta(minutes=20)), 'storage_class': 'Standard'}
        events = []
        test = self
        class Client:
            def __init__(self, *args): pass
            def request(self, path, **kwargs):
                test.assertNotIn('upload', events, 'old deployment cannot be rechecked after exact upload')
                test.assertEqual(kwargs.get('method', 'GET'), 'GET')
                if path == '/workers/account-settings': return {'default_usage_model': 'standard'}
                if path.endswith('/content/v2'): return b'synthetic', 'synthetic'
                if path.endswith('/deployments'): return {'deployments': [deployment]}
                if path.endswith('/settings'): return before
                if path.endswith('/routes') or path.startswith('/workers/domains?'): return []
                if path.startswith('/r2/buckets/synthetic-backups/objects?'):
                    events.append('list'); return {'result': [obj]}
                test.fail('Unapproved request including Cron/GraphQL: ' + path)
            def graphql(self, *args): test.fail('Cron telemetry is not a gate')
        def restore(*args, after_restore, **kwargs):
            self.assertEqual(args[2], obj)
            events.append('restore_verified')
            after_restore()
            events.extend(['recurrence_verified', 'cleanup_verified'])
            return {'proof': {'canonical_application_equal': True}, 'recurrence_verified': True, 'cleanup_verified': True}
        recovery = SimpleNamespace(admit=lambda receipt, approval: approval,
            verify_billing_period=lambda request: {'synthetic': True},
            claim=lambda *args, **kwargs: events.append('claim'), run=restore)
        promote = SimpleNamespace(single_version=lambda value: 'old-live', edges=lambda *args: [], OUT=Path('synthetic'))
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory); (root / 'deploy').mkdir(); (root / BACKUP.APPROVAL_PATH).write_text(json.dumps(approval))
            with mock.patch.object(BACKUP, 'ROOT', root), mock.patch.object(BACKUP, 'Cloudflare', Client), \
                 mock.patch.object(BACKUP, 'load_module', return_value=recovery), mock.patch.object(BACKUP, 'datetime', wraps=datetime) as clock, \
                 mock.patch.object(BACKUP, 'verify_reviewed_source'), mock.patch.object(BACKUP, 'require_backup_contract'), \
                 mock.patch.object(BACKUP, 'qualified_modules', return_value={}), mock.patch.object(BACKUP, 'module_hashes', return_value={}), \
                 mock.patch.dict(os.environ, {'CLOUDFLARE_API_TOKEN': 'synthetic'}):
                clock.now.return_value = checked
                result = BACKUP.gate(promote, {'source': 'a' * 40}, config, before, [deployment], ('b' * 40, True),
                                     after_restore=lambda: events.append('upload'))
                self.assertEqual(events, ['claim', 'list', 'restore_verified', 'upload', 'recurrence_verified', 'cleanup_verified'])
                self.assertTrue(result['backup']['persisted_alarm_verified'])
                self.assertFalse(result['backup']['new_production_backup_triggered'])
                self.assertFalse(result['backup']['natural_next_day_run_observed'])
                events.clear(); approval.pop('production_health_service')
                (root / BACKUP.APPROVAL_PATH).write_text(json.dumps(approval))
                with self.assertRaisesRegex(BACKUP.GateError, 'service access is not approved'):
                    BACKUP.gate(promote, {'source': 'a' * 40}, config, before, [deployment], ('b' * 40, True),
                                after_restore=lambda: events.append('upload'))
                self.assertEqual(events, [])


class BootstrapLifecycle(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.receipt = Path(self.tmp.name) / 'schedule.json'
        self.run_env = mock.patch.dict(os.environ, {'GITHUB_RUN_ID': '123'})
        self.run_env.start()
        self.addCleanup(self.run_env.stop)

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
                                unchanged, now=NOW, clock=lambda: client.now, sleep=client.sleep,
                                receipt_path=self.receipt)

    def test_always_cleanup_recovers_persisted_schedule_after_process_interruption(self):
        client = self.fixture()
        saved = {'format': 'scry-bootstrap-schedule-v1', 'account_id': 'a' * 32,
                 'worker': 'synthetic-worker', 'native_run_id': '123', 'original_crons': [],
                 'cron': client.slot, 'scheduled_at': BACKUP.iso(client.when), 'status': 'cleanup_pending'}
        self.receipt.write_text(json.dumps(saved))
        client.schedules = [{'cron': client.slot}]
        config = {'name': 'synthetic-worker', 'account_id': 'a' * 32}
        result = BACKUP.cleanup_schedule(client, config, self.receipt)
        self.assertEqual(result['schedule_cleanup'], 'verified_absent')
        self.assertEqual(client.schedules, [])
        self.assertEqual(json.loads(self.receipt.read_text())['status'], 'verified_absent')
        client.schedules = [{'cron': 'operator-change'}]
        with self.assertRaisesRegex(BACKUP.GateError, 'operator inspection'):
            BACKUP.cleanup_schedule(client, config, self.receipt)
        self.assertEqual(client.schedules, [{'cron': 'operator-change'}])
        self.assertEqual(json.loads(self.receipt.read_text())['status'], 'cleanup_pending')

    def test_intent_is_durable_before_schedule_put_and_owned_by_this_run(self):
        client = self.fixture()
        original = client.request
        def request(path, **kwargs):
            if kwargs.get('method') == 'PUT' and kwargs.get('payload'):
                saved = json.loads(self.receipt.read_text())
                self.assertEqual(saved['cron'], client.slot)
                self.assertEqual(saved['native_run_id'], '123')
                self.assertEqual(saved['status'], 'cleanup_pending')
            return original(path, **kwargs)
        client.request = request
        self.run_bootstrap(client)
        self.assertEqual(json.loads(self.receipt.read_text())['status'], 'verified_absent')
        with mock.patch.dict(os.environ, {'GITHUB_RUN_ID': '456'}):
            with self.assertRaisesRegex(BACKUP.GateError, 'identity'):
                BACKUP.cleanup_schedule(client, {'name': 'synthetic-worker', 'account_id': 'a' * 32}, self.receipt)

    def test_native_always_path_cleans_schedule_even_without_claim_receipt(self):
        root = Path(self.tmp.name)
        (root / 'deploy').mkdir()
        (root / 'deploy/rust-production.toml').write_text('name="synthetic-worker"\naccount_id="' + 'a' * 32 + '"\n')
        (root / 'target').mkdir()
        with mock.patch.object(BACKUP, 'ROOT', root), mock.patch.object(BACKUP, 'Cloudflare'), \
                mock.patch.dict(os.environ, {'CLOUDFLARE_API_TOKEN': 'synthetic'}), \
                mock.patch.object(BACKUP, 'cleanup_schedule', return_value={'schedule_cleanup': 'verified_absent'}) as cleanup, \
                mock.patch.object(BACKUP, 'load_module') as recovery, mock.patch('sys.stdout', new_callable=io.StringIO):
            BACKUP.cleanup_claim()
            cleanup.assert_called_once()
            self.assertEqual(cleanup.call_args.args[2], root / 'target/recovery-schedule.json')
            recovery.assert_not_called()

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
