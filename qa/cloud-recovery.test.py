#!/usr/bin/env python3
"""Executable recovery orchestration with only cloud effects replaced by fixtures."""
import email.parser
import email.policy
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import time
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / 'scripts/scry-cloud-recovery.py'
spec = importlib.util.spec_from_file_location('scry_cloud_recovery', SCRIPT)
R = importlib.util.module_from_spec(spec)
spec.loader.exec_module(R)
sha = lambda b: hashlib.sha256(b).hexdigest()
SOURCE = 'a' * 40
RECEIPT = {'source': SOURCE, 'artifact_id': 123}
PIN = {'key': 'snapshots/synthetic-source/1000-daily-' + '1' * 48 + '.json', 'etag': '1' * 32, 'size': 100}
CONFIG = {'name':'synthetic-production', 'r2_buckets': [{'binding': 'SCRY_BACKUPS', 'bucket_name': 'source-private'}]}
ENV = {'GITHUB_RUN_ID': '321', 'GITHUB_RUN_ATTEMPT': '1', 'GITHUB_REPOSITORY': 'misty-step/scry', 'GITHUB_REF': 'refs/heads/master', 'GITHUB_EVENT_NAME': 'workflow_dispatch', 'GITHUB_ACTOR': 'github-actions[bot]'}
def approval():
    return dict(production_health_service=CONFIG['name'],approved=True,scope='one-cloud-only-recovery-drill', approval_id='d'*32, reviewed_source=SOURCE,
                release_version='0.9.0', expires_ms=int(time.time()*1000)+7200000, one_time_cents=2000, failed_cleanup_storage_cents_per_month=25)

def subscriptions(end='2026-11-10T12:00:00Z'):
    row=dict(id='e'*32, currency='USD', current_period_start='2026-10-10T12:00:00Z',
             current_period_end=end, frequency='monthly', state='Paid',
             rate_plan=dict(scope='account', currency='USD', externally_managed=False, is_contract=False))
    return dict(success=True, errors=[], result=[row], result_info=dict(count=1,page=1,per_page=20,total_count=1))

def billing_period():
    return R.verify_billing_period(lambda path, **kw: subscriptions())

def artifact(root):
    modules = {'index.js': b'export default {};', 'index_bg.wasm': b'synthetic frozen wasm'}
    (root / 'worker').mkdir()
    for name, body in modules.items(): (root / 'worker' / name).write_bytes(body)
    (root / 'proof.json').write_text(json.dumps({'revision': SOURCE, 'source_state': 'committed', 'worker': {n: sha(b) for n, b in modules.items()}}))
    tooling = {str(p.relative_to(ROOT)): p.read_bytes() for p in (ROOT / 'src').rglob('*.rs')}
    for name in ('recovery/cloud-restore.mjs', 'recovery/target-adapter.mjs',
                 'deploy/cloudflare-hosting/telemetry.mjs', 'Cargo.toml', 'Cargo.lock'):
        tooling[name] = (ROOT / name).read_bytes()
    (root / 'source.json').write_text(json.dumps({'files': [{'path': n, 'sha256': sha(b)} for n,b in tooling.items()]}))
    with tarfile.open(root / 'source.tar.gz', 'w:gz') as t:
        for n,b in tooling.items():
            m=tarfile.TarInfo(n);m.size=len(b);t.addfile(m,io.BytesIO(b))

class Cloud:
    def __init__(self, fail=None):
        self.scripts = {}; self.buckets = {}; self.namespaces = set(); self.calls = []; self.fail = fail
        self.external_calls = []; self.deployed=False
    def __call__(self, path, method='GET', **kw):
        self.calls.append((path, method, kw))
        if self.fail: self.fail(path, method, kw)
        if path.startswith('/workers/domains?'): return []
        if path == '/workers/subdomain': return {'subdomain': 'synthetic-existing'}
        if path.startswith('/workers/durable_objects/namespaces?'):
            rows={}
            for name,current in self.scripts.items():
                for binding in current['settings']['bindings']:
                    identity=binding.get('namespace_id')
                    if identity in self.namespaces:
                        rows[identity]={'id':identity,'script':binding.get('script_name',name),'class':binding['class_name']}
            page=int(R.urllib.parse.parse_qs(R.urllib.parse.urlsplit(path).query)['page'][0]);values=list(rows.values())
            part=values[(page-1)*1000:page*1000]
            return {'success':True,'result':part,'result_info':{'count':len(part),'page':page,'per_page':1000,'total_count':len(rows),'total_pages':max(1,(len(rows)+999)//1000)}}
        if path.startswith('/workers/durable_objects/namespaces/'):
            identity = path.split('/')[4]
            return [] if identity in self.namespaces else None
        if path.startswith('/workers/scripts/'):
            name, _, tail = path[len('/workers/scripts/'):].partition('/')
            current = self.scripts.get(name)
            if tail == 'settings': return current['settings'] if current else None
            if tail == 'script-settings':
                if method == 'PATCH':
                    assert kw['payload'] == {'observability': {'enabled': False}, 'logpush': False, 'tail_consumers': []}
                return current['script_settings'] if current else None
            if tail == 'subdomain': return {'enabled': False, 'previews_enabled': False}
            if tail == 'routes': return []
            if tail == 'schedules': return {'schedules': []}
            if tail == 'subdomain/edge-preview': return {'token': 'session-token-' + 'x' * 30}
            if tail == 'edge-preview':
                assert method == 'POST' and kw['headers']['cf-preview-upload-config-token'].startswith('session-')
                return {'preview_token': 'preview-token-' + 'x' * 30}
            if tail == 'content/v2':
                body, headers = R.multipart({}, current['modules'])
                # Native content/v2 contains modules, not upload metadata.
                boundary = 'fixture'
                body = b''.join(('--'+boundary+'\r\nContent-Disposition: form-data; name="'+n+'"; filename="'+n+'"\r\n\r\n').encode()+v+b'\r\n' for n,v in current['modules'].items())+('--'+boundary+'--\r\n').encode()
                return body, 'multipart/form-data; boundary=' + boundary
            if method == 'PUT':
                form = email.parser.BytesParser(policy=email.policy.default).parsebytes(('Content-Type: '+kw['headers']['Content-Type']+'\r\n\r\n').encode()+kw['body'])
                fields = {p.get_filename():p.get_payload(decode=True) for p in form.iter_parts()}
                metadata = json.loads(fields.pop('metadata'))
                bindings = []
                for b in metadata.get('bindings', []):
                    b = dict(b)
                    if b['type'] == 'durable_object_namespace':
                        b['namespace_id'] = sha((b.get('script_name', name)+b['class_name']).encode())[:32]
                        self.namespaces.add(b['namespace_id'])
                    if b['type'] == 'secret_text': b.pop('text')
                    bindings.append(b)
                for klass in metadata.get('migrations', {}).get('deleted_classes', []):
                    self.namespaces.discard(sha((name+klass).encode())[:32])
                # The observed /settings shape omits observability; script-level
                # logging has its own documented endpoint and distinct fixture.
                self.scripts[name] = {'modules': fields, 'settings': {'bindings': bindings, 'limits': metadata.get('limits'),
                    'usage_model': metadata.get('usage_model'), 'logpush': metadata.get('logpush')},
                    'script_settings': {'observability': metadata.get('observability'), 'logpush': metadata.get('logpush')}}
                return {'id': 'created'}
            if method == 'DELETE': self.scripts.pop(name); return {}
        if path == '/r2/buckets' and method == 'POST':
            value = dict(name=kw['payload']['name'], storage_class='Standard', creation_date='2026-10-27T21:00:00Z')
            self.buckets[value['name']] = value; return value
        if path.startswith('/r2/buckets/'):
            name, _, tail = path[len('/r2/buckets/'):].partition('/')
            if tail == 'domains/managed': return {'enabled': False}
            if tail == 'domains/custom': return {'domains': []}
            if tail == 'objects?per_page=1000': return {'result': [], 'result_info': {'is_truncated': False}}
            if method == 'DELETE': self.buckets.pop(name); return {}
            if not tail: return self.buckets.get(name)
        raise AssertionError((path, method, kw))
    def external(self, url, **kw):
        self.external_calls.append((url, kw))
        gateway=next(v for n,v in self.scripts.items() if n.endswith('-bridge'))
        has_source=any(b['name']=='SOURCE' for b in gateway['settings']['bindings'])
        assert url.startswith('https://scry-recovery-') and any(url.endswith('.synthetic-existing.workers.dev/'+p) for p in ('restore','recurrence'))
        if kw['headers'].get('Authorization') in (None, 'Bearer synthetic-wrong-key'):
            assert not has_source, 'hosted refusal must precede learner binding'
            return 403, b'{}'
        assert has_source
        if url.endswith('/recurrence'):
            assert self.deployed, 'recurrence must follow exact deployment callback'
            assert kw['body'] == b''
            return 200,b'{"backup_proof":true}'
        return 200, json.dumps(dict(format='scry-cloud-restore-proof-v1', canonical_application_equal=True,
            source_sha256='2'*64, snapshot_sha256='3'*64, source_bytes=100, snapshot_bytes=120,
            photos_full_bytes_verified=0, restore_attempts=1, backup_attempts=1, elapsed_ms=5, target_database_bytes=4096, target_page_size=4096)).encode()

class Recovery(unittest.TestCase):
    def setUp(self):
        # Billing/admission policy is clock-bound; fixtures must not become red
        # simply because CI runs near a different month's boundary.
        frozen=R.datetime(2026,10,27,12,tzinfo=R.timezone.utc).timestamp()
        patch=mock.patch.object(R.time,'time',return_value=frozen)
        patch.start();self.addCleanup(patch.stop)

    def test_admission_refuses_absent_wrong_run_replay_expired_and_under_budget(self):
        with mock.patch.dict(os.environ, ENV, clear=True):
            with mock.patch.object(R.Path,'exists',return_value=False), self.assertRaisesRegex(R.RecoveryBlocked, 'APPROVAL_REQUIRED'): R.admit(RECEIPT)
            a = approval()
            self.assertEqual(R.admit(RECEIPT, approval=a), a)
            for k,v in [('approved',False),('scope','other'), ('reviewed_source','broken'), ('approval_id',''), ('release_version','bad'), ('one_time_cents',True)]:
                with self.subTest(k=k), self.assertRaises(R.RecoveryBlocked): R.admit(RECEIPT,approval=a|{k:v})
            for k,v in [('one_time_cents',100),('failed_cleanup_storage_cents_per_month',24)]:
                with self.assertRaisesRegex(R.RecoveryBlocked,'COST_BOUND'): R.admit(RECEIPT,approval=a|{k:v})
            with mock.patch.dict(os.environ, GITHUB_RUN_ATTEMPT='2'), self.assertRaises(R.RecoveryBlocked): R.admit(RECEIPT,approval=a)

    def test_private_claim_is_readback_verified_and_never_retried_before_expiry(self):
        class Claims:
            def __init__(self): self.data=None;self.paths=[];self.allowed=None
            def allow_claim(self,path):self.allowed=path
            def request(self,path,method='GET',**kwargs):
                self.paths.append((path,method));self.assert_path=path==self.allowed
                if method=='PUT':self.data=kwargs['body'];return {}
                return (self.data,'application/json') if self.data is not None else None
        c=Claims()
        with tempfile.TemporaryDirectory() as d,mock.patch.dict(os.environ,ENV,clear=True):
            saved=Path(d)/'claim.json'
            got=R.claim(approval(),RECEIPT,c.request,'synthetic-private',receipt_path=saved,billing_period=billing_period())
            self.assertEqual(got['github_run_id'],'321');self.assertTrue(c.assert_path)
            self.assertEqual([m for _,m in c.paths],['GET','PUT','GET'])
            with self.assertRaisesRegex(R.RecoveryBlocked,'ALREADY_CLAIMED'):R.claim(approval(),RECEIPT,c.request,'synthetic-private',receipt_path=saved,billing_period=billing_period())
            self.assertEqual([m for _,m in c.paths].count('PUT'),1)
            self.assertTrue(all('/operations/recovery-claims/' in p for p,_ in c.paths))

    def test_known_zone_rows_do_not_invalidate_the_account_billing_cycle(self):
        now=int(R.datetime(2026,10,27,12,tzinfo=R.timezone.utc).timestamp()*1000)
        body=subscriptions()
        account=body['result'][0]
        other=json.loads(json.dumps(account));other['id']='f'*32
        zone=dict(id='1'*32, currency='USD', frequency='synthetic-unrelated', state='Paid',
                  rate_plan=dict(scope='zone', currency='USD', externally_managed=False, is_contract=False))
        second=json.loads(json.dumps(zone));second['id']='2'*32
        body['result']=[zone,account,second,other]
        body['result_info'].update(count=4,total_count=4)
        expected=R.verify_billing_period(lambda *args,**kwargs:subscriptions(),now_ms=now)
        self.assertEqual(R.verify_billing_period(lambda *args,**kwargs:body,now_ms=now),expected)

    def test_zone_selection_preserves_strict_relevant_account_and_inventory_checks(self):
        now=int(R.datetime(2026,10,27,12,tzinfo=R.timezone.utc).timestamp()*1000)
        account=subscriptions()['result'][0]
        zone=dict(id='1'*32, rate_plan=dict(scope='zone'))
        body=subscriptions();body['result']=[zone,account]
        body['result_info'].update(count=2,total_count=2)
        variants=[]
        def variant(change):
            value=json.loads(json.dumps(body));change(value);variants.append(value)
        variant(lambda b:b['result'][0]['rate_plan'].update(scope='unknown-scope'))
        variant(lambda b:b['result'][0]['rate_plan'].pop('scope'))
        variant(lambda b:b['result'][1]['rate_plan'].update(scope='unknown-account-scope'))
        variant(lambda b:b['result'][1]['rate_plan'].pop('scope'))
        variant(lambda b:b['result'][1].pop('current_period_end'))
        variant(lambda b:b['result'][1]['rate_plan'].update(scope='zone'))
        variant(lambda b:b['result'][1].update(frequency='yearly'))
        variant(lambda b:b['result'][0].update(id=b['result'][1]['id']))
        variant(lambda b:b['result'][0].update(id=''))
        variant(lambda b:b['result_info'].update(total_count=3))
        mixed=json.loads(json.dumps(body));other=json.loads(json.dumps(account))
        other.update(id='f'*32,current_period_end='2026-11-11T12:00:00Z')
        mixed['result'].append(other);mixed['result_info'].update(count=3,total_count=3);variants.append(mixed)
        for i,value in enumerate(variants):
            with self.subTest(case=i),self.assertRaisesRegex(R.RecoveryBlocked,'BILLING_PERIOD'):
                R.verify_billing_period(lambda *args,**kwargs:value,now_ms=now)

    def test_actual_provider_period_replaces_calendar_assumption(self):
        calls=[]
        def request(path, **kwargs):
            calls.append((path,kwargs)); return subscriptions()
        window=R.verify_billing_period(request)
        self.assertEqual(calls,[('/subscriptions',{'envelope':True})])
        self.assertEqual(window['period_end_ms'],int(R.datetime(2026,11,10,12,tzinfo=R.timezone.utc).timestamp()*1000))
        with mock.patch.dict(os.environ,ENV,clear=True):
            a=approval()
            R.require_billing_period(window,a)
            # A full drain must fit after the LAST allowed admission, not just now.
            with self.assertRaisesRegex(R.RecoveryBlocked,'BILLING_PERIOD'):
                R.require_billing_period(window|{'period_end_ms':int(time.time()*1000)+R.DRAIN_MS+1},a)
            with self.assertRaisesRegex(R.RecoveryBlocked,'BILLING_PERIOD'):
                R.require_billing_period(window|{'checked_ms':int(time.time()*1000)-300001},a)
            # Oct 27 is far from Nov 1, but an Oct 28 product renewal must HOLD.
            with self.assertRaisesRegex(R.RecoveryBlocked,'BILLING_PERIOD'):
                R.verify_billing_period(lambda *args,**kwargs: subscriptions('2026-10-28T12:00:00Z'))
            # Conversely, calendar month end is safe if provider renewal is later.
            at=int(R.datetime(2026,10,31,12,tzinfo=R.timezone.utc).timestamp()*1000)
            later=R.verify_billing_period(request,now_ms=at)
            R.require_billing_period(later,a|{'expires_ms':at+7200000},now_ms=at)
            with self.assertRaises(R.RecoveryBlocked):R.admit(RECEIPT,approval=a|{'expires_ms':0})
            short=a|{'expires_ms':int(time.time()*1000)+60000}
            with self.assertRaises(R.RecoveryBlocked):R.admit(RECEIPT,approval=short)
            self.assertEqual(R.admit(RECEIPT,approval=short,starting=False),short)

    def test_continuation_accounting_must_stay_in_the_verified_billing_period(self):
        window=billing_period();a=approval()
        # A first run remains compatible; a continuation charges prior usage
        # in the SAME actual provider period, never a fresh rounded allowance.
        R.require_billing_period(window,a)
        for at in (window['period_start_ms'],window['checked_ms']-3600000,window['checked_ms']):
            R.require_billing_period(window,a|{'accounting_started_ms':at})
        for at in (window['period_start_ms']-1,window['checked_ms']+1,0,-1,True,None,'123',1.5):
            with self.subTest(at=at),self.assertRaisesRegex(R.RecoveryBlocked,'BILLING_PERIOD'):
                R.require_billing_period(window,a|{'accounting_started_ms':at})
        # A renewal between the prior phase and this continuation must fail
        # even when the new period easily covers the new expiry and full drain.
        at=window['checked_ms']-3600000
        with self.assertRaisesRegex(R.RecoveryBlocked,'BILLING_PERIOD'):
            R.require_billing_period(window|{'period_start_ms':at+1},a|{'accounting_started_ms':at})

    def test_cross_period_continuation_refuses_before_any_claim_request(self):
        with mock.patch.dict(os.environ,ENV,clear=True):
            window=billing_period();call=mock.Mock()
            a=approval()|{'accounting_started_ms':window['period_start_ms']-1}
            with self.assertRaisesRegex(R.RecoveryBlocked,'BILLING_PERIOD'):
                R.claim(a,RECEIPT,call,'synthetic-private',billing_period=window)
            call.assert_not_called()

    def test_missing_denied_ambiguous_or_incomplete_billing_refuses_before_claim(self):
        variants=[]
        for field,value in [('frequency','yearly'),('state','Trial'),('currency','EUR'),
                            ('current_period_end','invalid'),('current_period_start','2026-11-01T00:00:00Z')]:
            body=subscriptions();body['result'][0][field]=value;variants.append(body)
        for field,value in [('scope','zone'),('externally_managed',True),('is_contract',True)]:
            body=subscriptions();body['result'][0]['rate_plan'][field]=value;variants.append(body)
        body=subscriptions();body['result_info']['total_count']=2;variants.append(body)
        body=subscriptions();body.pop('result_info');variants.append(body)
        body=subscriptions();body['result'].append(dict(body['result'][0],id='f'*32,current_period_end='2026-11-11T12:00:00Z'))
        body['result_info'].update(count=2,total_count=2);variants.append(body)
        for body in variants:
            with self.subTest(body=body),self.assertRaisesRegex(R.RecoveryBlocked,'BILLING_PERIOD'):
                R.verify_billing_period(lambda *args,**kwargs: body)
        with self.assertRaisesRegex(R.RecoveryBlocked,'BILLING_PERIOD_UNAVAILABLE'):
            R.verify_billing_period(mock.Mock(side_effect=PermissionError('PRIVATE_CANARY')))
        call=mock.Mock()
        with mock.patch.dict(os.environ,ENV,clear=True), self.assertRaisesRegex(R.RecoveryBlocked,'BILLING_PERIOD'):
            R.claim(approval(),RECEIPT,call,'synthetic-private')
        call.assert_not_called()

    def test_claim_cleanup_waits_expiry_and_only_deletes_exact_body(self):
        class Claims:
            def __init__(self):self.body=None;self.calls=[]
            def allow_claim(self,path):self.path=path
            def request(self,path,method='GET',**kw):
                self.calls.append((method,clock[0]))
                if method=='PUT':self.body=kw['body'];return {}
                if method=='DELETE':self.body=None;return {}
                return None if self.body is None else (self.body,'application/json')
        c=Claims();clock=[time.time()]
        with tempfile.TemporaryDirectory() as d,mock.patch.dict(os.environ,ENV,clear=True):
            saved=Path(d)/'claim.json';a=approval()
            R.claim(a,RECEIPT,c.request,'synthetic-private',receipt_path=saved,billing_period=billing_period())
            def sleep(t):clock[0]+=t
            self.assertEqual(R.cleanup_claim(saved,c.request,clock=lambda:clock[0],sleep=sleep),{'claim_cleanup':'verified_absent'})
            deletion=next(t for m,t in c.calls if m=='DELETE')
            self.assertGreater(deletion*1000,a['expires_ms'])
            # A changed body is never removed, even after expiry.
            c.body=b'changed'
            with self.assertRaisesRegex(R.RecoveryBlocked,'IDENTITY_CHANGED'):
                R.cleanup_claim(saved,c.request,clock=lambda:clock[0],sleep=sleep)
            self.assertEqual(c.body,b'changed')

    def test_rounding_without_free_tier_assumptions(self):
        usage={n:v[2] for n,v in R.METERS.items()}
        self.assertEqual(R.rounded_quote(usage),19066000)
        self.assertEqual(R.rounded_quote(usage|{'r2_class_a':1000001}),23566000)
        for bad in ({},usage|{'do_requests':True},usage|{'do_requests':-1}):
            with self.assertRaises(R.RecoveryBlocked): R.rounded_quote(bad)

    def run_fixture(self, cloud, change=None, after_restore=None):
        with tempfile.TemporaryDirectory() as d, mock.patch.dict(os.environ, ENV, clear=True):
            root=Path(d);artifact(root)
            if change: change(root)
            if after_restore is None: after_restore=lambda:setattr(cloud,'deployed',True)
            return R.run(CONFIG,root,PIN,cloud,approval=approval(),receipt=RECEIPT,external=cloud.external,billing_period=billing_period(),after_restore=after_restore)

    def test_preview_exchange_contract_and_bounded_complete_flow(self):
        class ExchangeCloud(Cloud):
            def __call__(self, path, method='GET', **kw):
                result = super().__call__(path, method=method, **kw)
                if path.endswith('/subdomain/edge-preview'):
                    result['exchange_url'] = 'https://session.synthetic-existing.workers.dev/exchange'
                return result
            def external(self, url, **kw):
                if url.endswith('/exchange'):
                    self.external_calls.append((url, kw))
                    self.assert_exchange = kw == {}
                    return 200, json.dumps({'token': 'session-exchanged-' + 'x'*30}).encode()
                return super().external(url, **kw)
        cloud = ExchangeCloud()
        receipt = self.run_fixture(cloud)
        self.assertTrue(cloud.assert_exchange)
        self.assertEqual(receipt['status'], 'verified')
        self.assertEqual(receipt['preview_requests'], 5)
        upload = next(kw for p,m,kw in cloud.calls if p.endswith('/edge-preview') and m == 'POST')
        self.assertEqual(upload['headers']['cf-preview-upload-config-token'], 'session-exchanged-' + 'x'*30)
        self.assertEqual((cloud.scripts, cloud.buckets, cloud.namespaces), ({}, {}, set()))

    def test_preview_exchange_malformed_response_is_fixed_and_cleanup_verified(self):
        for body in (b'not-json-private-sentinel', b'{}', b'[]', b'null', b'{"token":null}', b'{"token":"short"}'):
            class ExchangeCloud(Cloud):
                def __call__(self, path, method='GET', **kw):
                    result = super().__call__(path, method=method, **kw)
                    if path.endswith('/subdomain/edge-preview'):
                        result['exchange_url'] = 'https://session.synthetic-existing.workers.dev/exchange'
                    return result
                def external(self, url, **kw):
                    assert url.endswith('/exchange')
                    return 200, body
            with self.subTest(body=body):
                cloud = ExchangeCloud()
                with self.assertRaises(R.RecoveryBlocked) as caught:
                    self.run_fixture(cloud)
                error = caught.exception
                self.assertEqual(error.primary_reason, 'RECOVERY_PREVIEW_EXCHANGE_INVALID')
                self.assertEqual(error.recovery_operation, 'preview-exchange')
                self.assertFalse(error.deployment_attempted)
                self.assertEqual((cloud.scripts, cloud.buckets, cloud.namespaces), ({}, {}, set()))
                self.assertNotIn('private-sentinel', str(error))

    def test_preview_native_response_shapes_fail_at_identified_step(self):
        for tail, operation, reason in (
                ('subdomain/edge-preview', 'preview-session', 'RECOVERY_PREVIEW_SESSION_INVALID'),
                ('edge-preview', 'preview-upload', 'RECOVERY_PREVIEW_TOKEN_INVALID')):
            class ShapeCloud(Cloud):
                def __call__(self, path, method='GET', **kw):
                    result = super().__call__(path, method=method, **kw)
                    if path.endswith('/' + tail) and (tail != 'edge-preview' or method == 'POST'):
                        return None
                    return result
            cloud = ShapeCloud()
            with self.subTest(tail=tail), self.assertRaises(R.RecoveryBlocked) as caught:
                self.run_fixture(cloud)
            self.assertEqual(caught.exception.recovery_operation, operation)
            self.assertEqual(caught.exception.primary_reason, reason)
            self.assertFalse(caught.exception.deployment_attempted)
            self.assertEqual((cloud.scripts, cloud.buckets, cloud.namespaces), ({}, {}, set()))

    def test_preview_transport_failure_is_fixed_without_exception_details(self):
        for failure in (R.urllib.error.URLError('private-url-or-key'), TimeoutError('private-key')):
            with mock.patch.object(R.urllib.request, 'build_opener') as opener:
                opener.return_value.open.side_effect = failure
                with self.assertRaises(R.RecoveryBlocked) as caught:
                    R.private_request('https://session.synthetic-existing.workers.dev/exchange')
                self.assertEqual(R.fixed_reason(caught.exception), 'RECOVERY_PREVIEW_TRANSPORT_UNAVAILABLE')
                self.assertNotIn('private', str(caught.exception))

    def test_real_orchestration_uploads_frozen_bytes_and_deletes_only_created_data(self):
        cloud=Cloud(); receipt=self.run_fixture(cloud)
        self.assertEqual(receipt['status'],'verified');self.assertTrue(receipt['cleanup_verified'])
        self.assertEqual(cloud.scripts,{});self.assertEqual(cloud.namespaces,set());self.assertEqual(cloud.buckets,{})
        self.assertLessEqual(len(cloud.calls),128);self.assertEqual(len(cloud.external_calls),4);self.assertTrue(receipt['recurrence_verified'])
        self.assertFalse(any('/source-private/objects/' in p for p,m,k in cloud.calls))
        self.assertFalse(any(m=='POST' and p.endswith('/subdomain') for p,m,k in cloud.calls))

    def test_logging_uses_dedicated_script_settings_with_explicit_disabled_upload(self):
        class ObservedSettingsCloud(Cloud):
            def __call__(self, path, method='GET', **kw):
                value=super().__call__(path,method=method,**kw)
                if path.endswith('/settings') and value:
                    self_test.assertNotIn('observability',value)
                return value
        self_test=self
        cloud=ObservedSettingsCloud()
        self.assertEqual(self.run_fixture(cloud)['status'],'verified')
        reads=[(p,m) for p,m,_ in cloud.calls if p.endswith('/script-settings')]
        self.assertEqual(len(reads),6)  # target, refusal-only bridge, then learner-bound bridge
        self.assertEqual([m for _,m in reads], ['PATCH','GET']*3)
        self.assertTrue(all(k['expected_status']==200 for p,m,k in cloud.calls if p.endswith('/script-settings')))
        for _,method,kw in cloud.calls:
            if method!='PUT':continue
            form=email.parser.BytesParser(policy=email.policy.default).parsebytes(
                ('Content-Type: '+kw['headers']['Content-Type']+'\r\n\r\n').encode()+kw['body'])
            fields={p.get_filename():p.get_payload(decode=True) for p in form.iter_parts()}
            metadata=json.loads(fields['metadata'])
            if metadata.get('migrations',{}).get('deleted_classes'):continue
            self.assertEqual(metadata['observability'],{'enabled':False})
            self.assertIs(metadata['logpush'],False)

    def test_logging_missing_malformed_or_enabled_refuses_before_private_invocation(self):
        disabled={'observability':{'enabled':False},'logpush':False}
        variants=[None,[],{}, {'logpush':False}, {'observability':{'enabled':False}},
                  *[disabled|{'observability':v} for v in (False,[],{},'false',{'enabled':True},{'enabled':0},{'enabled':None})],
                  *[disabled|{'logpush':v} for v in (None,True,0,'false')]]
        for value in variants:
            class LoggingCloud(Cloud):
                def __call__(self,path,method='GET',**kw):
                    observed=super().__call__(path,method=method,**kw)
                    return value if path.endswith('/script-settings') else observed
            cloud=LoggingCloud();callback=mock.Mock()
            with self.subTest(value=value),self.assertRaises(R.RecoveryBlocked) as caught:
                self.run_fixture(cloud,after_restore=callback)
            self.assertEqual(caught.exception.primary_reason,'RECOVERY_LOGGING_UNVERIFIED')
            self.assertEqual(sum(p.endswith('/script-settings') for p,_,_ in cloud.calls),2)
            self.assertEqual(cloud.scripts,{})
            self.assertEqual(cloud.namespaces,set())
            self.assertEqual(cloud.external_calls,[])
            callback.assert_not_called()

    def test_observed_null_requires_audited_source_and_acknowledged_disable(self):
        class NullCloud(Cloud):
            def __call__(self,path,method='GET',**kw):
                value=super().__call__(path,method=method,**kw)
                if path.endswith('/script-settings'):
                    return {'observability':None,'logpush':False,'tail_consumers':[]}
                return value
        cloud=NullCloud();result=self.run_fixture(cloud)
        self.assertEqual(result['status'],'verified')
        self.assertEqual(len(result['privacy_observations']),3)
        for row in result['privacy_observations']:
            self.assertIsNone(row['observability'])
            self.assertEqual(row['assurance'],'audited-no-learner-content-emissions')
        with mock.patch.object(R,'audited_privacy_source',return_value=False):
            cloud=NullCloud()
            with self.assertRaises(R.RecoveryBlocked) as caught:self.run_fixture(cloud)
            self.assertEqual(caught.exception.primary_reason,'RECOVERY_LOGGING_UNVERIFIED')
            self.assertEqual(cloud.external_calls,[])
            self.assertEqual(cloud.scripts,{})
            self.assertEqual(cloud.namespaces,set())
        def refused(path,method,kw):
            if path.endswith('/script-settings') and method=='PATCH':raise RuntimeError('PRIVATE_CANARY')
        cloud=NullCloud(fail=refused)
        with self.assertRaises(R.RecoveryBlocked):self.run_fixture(cloud)
        self.assertEqual(cloud.external_calls,[])
        self.assertFalse(any(p.endswith('/script-settings') and m=='GET' for p,m,k in cloud.calls))
        self.assertEqual(cloud.scripts,{})
        self.assertEqual(cloud.namespaces,set())

    def test_tails_or_unknown_observability_refuse_and_cleanup_verified_namespace(self):
        for field,value in [('tail_consumers',[{'service':'unexpected'}]),('tail_consumers','unknown'),
                            ('observability',{'enabled':True}),('observability',{'enabled':False,'mystery':True})]:
            class TailCloud(Cloud):
                def __call__(self,path,method='GET',**kw):
                    result=super().__call__(path,method=method,**kw)
                    return {'observability':None,'logpush':False,'tail_consumers':[]}|{field:value} if path.endswith('/script-settings') else result
            cloud=TailCloud()
            with self.subTest(field=field,value=value),self.assertRaises(R.RecoveryBlocked):self.run_fixture(cloud)
            self.assertEqual(cloud.external_calls,[])
            self.assertEqual(cloud.scripts,{})
            self.assertEqual(cloud.namespaces,set())

    def test_audited_privacy_fingerprint_rejects_source_change(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);artifact(root)
            self.assertTrue(R.audited_privacy_source(root))
            with tarfile.open(root/'source.tar.gz','r:gz') as archive:
                entries={m.name:archive.extractfile(m).read() for m in archive.getmembers()}
            entries['src/runtime.rs']+=b'\n// unreviewed source change\n'
            with tarfile.open(root/'source.tar.gz','w:gz') as archive:
                for name,data in entries.items():
                    member=tarfile.TarInfo(name);member.size=len(data);archive.addfile(member,io.BytesIO(data))
            self.assertFalse(R.audited_privacy_source(root))

    def test_service_approval_absent_or_wrong_refuses_before_keys_or_cloud(self):
        cloud=Cloud()
        with tempfile.TemporaryDirectory() as d,mock.patch.dict(os.environ,ENV,clear=True),mock.patch.object(R.secrets,'token_hex') as keys:
            root=Path(d);artifact(root)
            for name in (None,'other-service'):
                with self.assertRaisesRegex(R.RecoveryBlocked,'PRODUCTION_HEALTH_SERVICE_APPROVAL_REQUIRED'):
                    R.run(CONFIG,root,PIN,cloud,approval=approval()|{'production_health_service':name},receipt=RECEIPT,
                          external=cloud.external,billing_period=billing_period(),after_restore=mock.Mock())
            keys.assert_not_called();self.assertEqual(cloud.calls,[])

    def test_callback_failure_is_deployment_attempted_and_never_probes_recurrence(self):
        cloud=Cloud();callback=mock.Mock(side_effect=RuntimeError('PRIVATE_CANARY'))
        with self.assertRaisesRegex(R.RecoveryBlocked,'DEPLOYMENT_CALLBACK_FAILED') as result:
            self.run_fixture(cloud,after_restore=callback)
        callback.assert_called_once_with()
        self.assertTrue(result.exception.deployment_attempted)
        self.assertEqual(result.exception.recovery_phase,'deployment')
        self.assertFalse(any(url.endswith('/recurrence') for url,_ in cloud.external_calls))
        self.assertFalse(cloud.scripts or cloud.buckets or cloud.namespaces)

    def test_recurrence_refusal_or_oversize_never_retries_and_cleans_deployed_attempt(self):
        for status,body in [(503,b'{"backup_proof":false}'),(200,b'{"backup_proof":true,"extra":1}'),
                            (200,b'{"backup_proof":true}'+b' '*65)]:
            cloud=Cloud();original=cloud.external
            def external(url,_status=status,_body=body,**kw):
                response=original(url,**kw)
                return (_status,_body) if url.endswith('/recurrence') else response
            cloud.external=external
            with self.assertRaisesRegex(R.RecoveryBlocked,'RECURRENCE_NOT_VERIFIED') as result:self.run_fixture(cloud)
            self.assertTrue(result.exception.deployment_attempted)
            self.assertEqual(result.exception.recovery_phase,'recurrence')
            self.assertEqual(sum(url.endswith('/recurrence') for url,_ in cloud.external_calls),1)
            self.assertFalse(cloud.scripts or cloud.buckets or cloud.namespaces)

    def test_callback_failure_with_cleanup_failure_preserves_deployment_phase(self):
        def fail(path,method,kw):
            if method=='DELETE' and path.startswith('/r2/buckets/'):raise OSError('PRIVATE_CANARY')
        cloud=Cloud(fail);callback=mock.Mock(side_effect=RuntimeError('PRIVATE_CANARY'))
        with self.assertRaisesRegex(R.RecoveryBlocked,'RECOVERY_CLEANUP_FAILED') as caught:
            self.run_fixture(cloud,after_restore=callback)
        self.assertEqual(caught.exception.recovery_phase,'deployment')
        self.assertTrue(caught.exception.deployment_attempted)
        self.assertEqual(caught.exception.retained_resources,list(cloud.buckets))
        self.assertFalse(any(url.endswith('/recurrence') for url,_ in cloud.external_calls))

    def test_wrong_service_readback_blocks_before_restore_and_deployment(self):
        class WrongServiceCloud(Cloud):
            def __call__(self,path,method='GET',**kw):
                value=super().__call__(path,method=method,**kw)
                if path.endswith('/settings') and value:
                    for b in value['bindings']:
                        if b['type']=='service':b['service']='wrong-target'
                return value
        cloud=WrongServiceCloud();callback=mock.Mock()
        with self.assertRaises(R.RecoveryBlocked):self.run_fixture(cloud,after_restore=callback)
        callback.assert_not_called()

    def test_cleanup_rest_listing_uses_optional_cursor_metadata_not_required_flag(self):
        for info in ('missing', None, {}, {'cursor': None}, {'cursor': ''}, {'is_truncated': False}):
            class CursorCloud(Cloud):
                def __call__(self, path, method='GET', **kw):
                    if path.endswith('/objects?per_page=1000'):
                        return {'result': []} if info == 'missing' else {'result': [], 'result_info': info}
                    return super().__call__(path, method=method, **kw)
            cloud = CursorCloud()
            with self.subTest(info=info):
                self.assertTrue(self.run_fixture(cloud)['cleanup_verified'])
                self.assertFalse(cloud.buckets or cloud.scripts or cloud.namespaces)
        for info in ([], 'invalid', {'cursor': 1}, {'cursor': 'more'}, {'is_truncated': None},
                     {'is_truncated': True}, {'is_truncated': 'false'},
                     {'is_truncated': False, 'cursor': 'contradiction'}):
            class UnboundedCloud(Cloud):
                def __call__(self, path, method='GET', **kw):
                    if path.endswith('/objects?per_page=1000'):
                        return {'result': [], 'result_info': info}
                    return super().__call__(path, method=method, **kw)
            cloud = UnboundedCloud()
            with self.subTest(info=info), self.assertRaisesRegex(R.RecoveryBlocked, 'RECOVERY_CLEANUP_FAILED'):
                self.run_fixture(cloud)
            self.assertEqual(len(cloud.buckets), 1)

    def test_oversize_or_invalid_page_proof_never_releases_and_still_cleans(self):
        for field, value in [('target_database_bytes', 32*1024*1024+1), ('target_page_size', 8192)]:
            cloud=Cloud(); original=cloud.external
            def external(url, _field=field, _value=value, **kw):
                status, body=original(url,**kw)
                if status == 200:
                    proof=json.loads(body); proof[_field]=_value; body=json.dumps(proof).encode()
                return status, body
            cloud.external=external;callback=mock.Mock()
            with self.subTest(field=field), self.assertRaisesRegex(R.RecoveryBlocked, 'RECOVERY_NOT_VERIFIED'):
                self.run_fixture(cloud,after_restore=callback)
            callback.assert_not_called()
            self.assertFalse(cloud.scripts or cloud.buckets or cloud.namespaces)

    def test_invalid_artifact_refuses_before_any_cloud_or_credentials(self):
        cloud=Cloud()
        with mock.patch.object(R.secrets,'token_hex') as keys:
            with self.assertRaisesRegex(R.RecoveryBlocked,'CHECKSUM'):
                self.run_fixture(cloud,lambda root:(root/'worker/index.js').write_bytes(b'changed'))
            keys.assert_not_called();self.assertEqual(cloud.calls,[])

    def test_metadata_archive_limit_refuses_before_resources_or_keys(self):
        cloud=Cloud()
        with tempfile.TemporaryDirectory() as d,mock.patch.dict(os.environ,ENV,clear=True),mock.patch.object(R.secrets,'token_hex') as keys:
            root=Path(d);artifact(root)
            with self.assertRaisesRegex(R.RecoveryBlocked, 'ARCHIVE_BYTES_EXCEED_LIMIT size=16777217 limit=16777216'):
                R.run(CONFIG,root,PIN|{'size':16*1024*1024+1},cloud,approval=approval(),receipt=RECEIPT,external=cloud.external,billing_period=billing_period(),after_restore=mock.Mock())
            keys.assert_not_called();self.assertEqual(cloud.calls,[])

    def test_malformed_etag_refuses_before_cloud_or_credential_generation(self):
        cloud=Cloud()
        with tempfile.TemporaryDirectory() as d,mock.patch.dict(os.environ,ENV,clear=True),mock.patch.object(R.secrets,'token_hex') as keys:
            root=Path(d);artifact(root)
            for etag in ('"'+'1'*32,'1'*32+'"'):
                with self.assertRaisesRegex(R.RecoveryBlocked,'SOURCE_PIN'):
                    R.run(CONFIG,root,PIN|{'etag':etag},cloud,approval=approval(),receipt=RECEIPT,external=cloud.external,billing_period=billing_period(),after_restore=mock.Mock())
            keys.assert_not_called();self.assertEqual(cloud.calls,[])

    def test_unknown_transfer_result_never_retries_and_cleans_up(self):
        cloud=Cloud()
        def external(url,**kw):
            if kw['headers'].get('Authorization','').startswith('Bearer ') and kw['headers']['Authorization']!='Bearer synthetic-wrong-key':
                raise TimeoutError('PRIVATE_CANARY')
            return 403,b'{}'
        cloud.external=external
        with self.assertRaisesRegex(R.RecoveryBlocked,'RECOVERY_NOT_VERIFIED'):self.run_fixture(cloud)
        self.assertFalse(cloud.scripts or cloud.buckets or cloud.namespaces)

    def test_cleanup_failure_blocks_success_and_retains_exact_private_resource_names(self):
        def fail(path,method,kw):
            if method=='DELETE' and path.startswith('/r2/buckets/'):raise OSError('PRIVATE_CANARY')
        cloud=Cloud(fail)
        with self.assertRaisesRegex(R.RecoveryBlocked,'RECOVERY_CLEANUP_FAILED') as caught:self.run_fixture(cloud)
        self.assertEqual(caught.exception.retained_resources,list(cloud.buckets))
        self.assertTrue(caught.exception.deployment_attempted)
        self.assertEqual(caught.exception.recovery_phase,'verified')
        self.assertNotIn('PRIVATE_CANARY',str(caught.exception))

    def test_ambiguous_namespace_creation_is_not_deleted_as_an_empty_shell(self):
        fired=False
        def fail(path,method,kw):
            nonlocal fired
            if method=='PUT' and b'new_sqlite_classes' in kw.get('body',b'') and not fired:
                fired=True;raise TimeoutError('ambiguous creation')
        cloud=Cloud(fail)
        with self.assertRaisesRegex(R.RecoveryBlocked,'CLEANUP_FAILED'):self.run_fixture(cloud)
        self.assertEqual(len(cloud.scripts),1)
        self.assertFalse(cloud.buckets)

    def test_cleanup_uses_namespace_inventory_not_invalid_limit_one(self):
        cloud=Cloud();self.run_fixture(cloud)
        self.assertTrue(any(p.startswith('/workers/durable_objects/namespaces?') for p,_,_ in cloud.calls))
        self.assertFalse(any('/objects?limit=' in p for p,_,_ in cloud.calls))

    def test_empty_object_list_does_not_prove_namespace_absence(self):
        class RetainedNamespace(Cloud):
            def __call__(self,path,method='GET',**kw):
                if path.startswith('/workers/durable_objects/namespaces/'):
                    self.calls.append((path,method,kw));return []
                value=super().__call__(path,method=method,**kw)
                if path.startswith('/workers/durable_objects/namespaces?'):
                    name=next(n for n in self.scripts if n.endswith('-bridge'))
                    page=int(R.urllib.parse.parse_qs(R.urllib.parse.urlsplit(path).query)['page'][0])
                    value['result']=[{'id':'f'*32,'script':name,'class':'RecoveryBridge'}] if page==1 else []
                    value['result_info'].update(count=len(value['result']),total_count=1,total_pages=1)
                return value
        cloud=RetainedNamespace()
        with self.assertRaisesRegex(R.RecoveryBlocked,'CLEANUP_FAILED') as caught:self.run_fixture(cloud)
        self.assertTrue(any(x['reason']=='RECOVERY_NAMESPACE_CLEANUP_UNVERIFIED' for x in caught.exception.cleanup_outcomes))
        self.assertFalse(any('/objects?limit=' in p for p,_,_ in cloud.calls))

    def test_namespace_inventory_requires_complete_consistent_pagination(self):
        rows=[{'id':str(i),'script':'unrelated','class':'Other'} for i in range(1001)]
        def response(page):
            part=rows[(page-1)*1000:page*1000]
            return {'success':True,'result':part,'result_info':{'count':len(part),'page':page,'per_page':1000,'total_count':1001,'total_pages':2}}
        calls=[]
        def request(path,method='GET',**kw):
            calls.append(path);return response(len(calls))
        self.assertTrue(R.Drill(request).namespace_absent('owned-id','owned-script'))
        self.assertEqual(len(calls),3)
        for change in [lambda r:r['result_info'].update(total_count=1002),
                       lambda r:r['result_info'].update(page=1),lambda r:r['result_info'].update(count=0),
                       lambda r:r['result_info'].update(total_pages=9),lambda r:r['result'].append(rows[0])]:
            calls=[]
            def bad(path,method='GET',**kw):
                calls.append(path);value=response(len(calls))
                if len(calls)==2:change(value)
                return value
            with self.subTest(change=change),self.assertRaisesRegex(R.RecoveryBlocked,'NAMESPACE_INVENTORY'):
                R.Drill(bad).namespace_absent('owned-id','owned-script')

    def test_namespace_sdk_uses_empty_terminal_page_not_short_page_or_required_metadata(self):
        rows=[{'id':'first','script':'unrelated','class':'Other'}, {'id':'second','script':'unrelated','class':'Other'}]
        for info in ('missing',None,{}, {'page':None,'per_page':None}):
            calls=[]
            def request(path,method='GET',**kw):
                calls.append(path);page=len(calls)
                response={'success':True,'result':rows[page-1:page]}
                if info!='missing':response['result_info']=info
                return response
            with self.subTest(info=info):
                self.assertTrue(R.Drill(request).namespace_absent('owned-id','owned-script'))
                self.assertEqual(len(calls),3)
        for info in ([],{'count':True},{'page':7},{'total_count':0},{'total_pages':0},{'per_page':0}):
            with self.subTest(info=info),self.assertRaisesRegex(R.RecoveryBlocked,'NAMESPACE_INVENTORY'):
                R.Drill(lambda *a,**k:{'success':True,'result':rows[:1],'result_info':info}).namespace_absent('owned-id','owned-script')
        calls=[]
        def repeat(*args,**kwargs):
            calls.append(1);return {'success':True,'result':rows[:1]}
        with self.assertRaisesRegex(R.RecoveryBlocked,'NAMESPACE_INVENTORY'):R.Drill(repeat).namespace_absent('owned-id','owned-script')
        self.assertEqual(len(calls),2)

    def test_fixed_reason_does_not_persist_arbitrary_identifier_shaped_messages(self):
        for error in (R.RecoveryBlocked('PRIVATE_SENTINEL'),RuntimeError('PRIVATE_SENTINEL')):
            self.assertEqual(R.fixed_reason(error),'RECOVERY_OPERATION_UNVERIFIED')
        self.assertEqual(R.fixed_reason(R.RecoveryBlocked('RECOVERY_LIMITS_UNVERIFIED')),'RECOVERY_LIMITS_UNVERIFIED')
        self.assertEqual(R.fixed_reason(R.RecoveryBlocked('RECOVERY_TAIL_EXPORT_PRESENT')),'RECOVERY_TAIL_EXPORT_PRESENT')

    def test_nullable_tail_consumers_through_actual_http_transport(self):
        # Exercise production Cloudflare.request, including status/envelope and
        # multipart decoding, with only the HTTP opener replaced. No FakeCloud.
        spec = importlib.util.spec_from_file_location('recovery_transport_test', ROOT/'scripts/scry-cloud-backup.py')
        transport = importlib.util.module_from_spec(spec); spec.loader.exec_module(transport)
        name = 'scry-recovery-' + 'a'*24 + '-target'
        base = '/workers/scripts/' + name
        modules = {'target-adapter.mjs': b'export default {}'}
        metadata = {'main_module': 'target-adapter.mjs', 'limits': {'cpu_ms': 30000, 'subrequests': 100},
                    'usage_model': 'standard', 'bindings': [{'name': 'LEARNING_SPACE',
                    'type': 'durable_object_namespace', 'class_name': 'LearningSpace'}]}
        missing = object()
        accepted = (missing, None, [])
        variants = [(a,b,200,True) for a in accepted for b in accepted]
        for bad in ([{'service':'synthetic-consumer'}], {}, False, 0, '', 'null'):
            variants.extend(((bad,[],200,False), ([],bad,200,False)))
        variants.append((None,None,202,False))
        for settings_tail, logging_tail, status, passes in variants:
            with self.subTest(settings=settings_tail, logging=logging_tail, status=status):
                settings = dict(metadata, bindings=[dict(metadata['bindings'][0], namespace_id='b'*32)])
                logging = {'observability': None, 'logpush': False}
                if settings_tail is not missing: settings['tail_consumers'] = settings_tail
                if logging_tail is not missing: logging['tail_consumers'] = logging_tail
                calls = []
                class Response(io.BytesIO):
                    def __init__(self, body, kind='application/json', code=200):
                        super().__init__(body); self.headers={'Content-Type':kind}; self.status=code
                def opened(request, timeout):
                    path = R.urllib.parse.urlsplit(request.full_url).path.removeprefix('/client/v4/accounts/synthetic')
                    calls.append((request.method,path))
                    if path == base+'/content/v2':
                        body = b'--fixture\r\nContent-Disposition: form-data; name="target-adapter.mjs"; filename="target-adapter.mjs"\r\n\r\n'+modules['target-adapter.mjs']+b'\r\n--fixture--\r\n'
                        return Response(body,'multipart/form-data; boundary=fixture')
                    if path == base and request.method == 'PUT': result = {}
                    elif path == base+'/settings': result = settings
                    elif path == base+'/script-settings':
                        if request.method == 'PATCH':
                            self.assertEqual(json.loads(request.data), {'observability':{'enabled':False},
                                             'logpush':False,'tail_consumers':[]})
                        result = logging
                    elif path == base+'/subdomain': result = {'enabled':False,'previews_enabled':False}
                    elif path == base+'/routes': result = []
                    elif path == base+'/schedules': result = {'schedules':[]}
                    elif path == '/workers/domains': result = []
                    else: self.fail('unexpected HTTP path')
                    return Response(json.dumps({'success':True,'errors':[],'result':result}).encode(),
                                    code=status if path.endswith('/script-settings') else 200)
                cloud = transport.Cloudflare('synthetic','synthetic'); cloud.open = opened
                drill = R.Drill(cloud.request, privacy_audited=True)
                if passes:
                    drill.bind(name,metadata,modules,'LearningSpace')
                    self.assertEqual(len(drill.privacy_observations),1)
                else:
                    with self.assertRaises((R.RecoveryBlocked,transport.GateError)) as caught:
                        drill.bind(name,metadata,modules,'LearningSpace')
                    if status == 200:
                        self.assertEqual(R.fixed_reason(caught.exception),'RECOVERY_TAIL_EXPORT_PRESENT')
                    self.assertEqual(drill.privacy_observations,[])
                self.assertEqual(drill.external_calls,0)
                self.assertLessEqual(len(calls),9)

    def test_primary_cause_is_not_replaced_by_cleanup_failure(self):
        def fail(path,method,kw):
            if method=='DELETE' and path.startswith('/r2/buckets/'):raise OSError('PRIVATE_CANARY')
        cloud=Cloud(fail)
        callback=mock.Mock(side_effect=R.RecoveryBlocked('RECOVERY_PROOF_INVALID'))
        with self.assertRaisesRegex(R.RecoveryBlocked,'CLEANUP_FAILED') as caught:self.run_fixture(cloud,after_restore=callback)
        error=caught.exception
        self.assertEqual(error.primary_reason,'RECOVERY_PROOF_INVALID')
        self.assertEqual(error.recovery_phase,'deployment');self.assertTrue(error.deployment_attempted)
        self.assertTrue(any(x['step']=='bucket_delete' and x['status']=='unverified' for x in error.cleanup_outcomes))
        self.assertNotIn('PRIVATE_CANARY',json.dumps(error.cleanup_outcomes))

    def test_request_limits_and_deadline_fail_without_effect(self):
        call=mock.Mock();drill=R.Drill(call,clock=lambda:0)
        drill.calls=75
        with self.assertRaisesRegex(R.RecoveryBlocked,'BUDGET'):drill.call('/anything')
        call.assert_not_called()
        drill=R.Drill(call,clock=lambda:0);drill.clock=lambda:901
        with self.assertRaisesRegex(R.RecoveryBlocked,'DEADLINE'):drill.call('/anything')
        call.assert_not_called()

    def test_private_transport_refuses_credentials_redirect_targets_and_non_cf_hosts(self):
        for url in ['http://example.workers.dev/','https://example.com/','https://secret@x.workers.dev/','https://x.workers.dev:8443/']:
            with self.subTest(url=url),self.assertRaises(R.RecoveryBlocked):R.private_request(url)

    def test_cli_is_offline_only(self):
        p=subprocess.run([sys.executable,str(SCRIPT)],capture_output=True,text=True,
            env={'CLOUDFLARE_API_TOKEN':'PRIVATE_CANARY'})
        self.assertEqual(p.returncode,3);self.assertNotIn('PRIVATE_CANARY',p.stdout+p.stderr)
        self.assertFalse(json.loads(p.stdout)['cloud_requests_made'])

if __name__=='__main__':unittest.main()
