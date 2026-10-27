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
CONFIG = {'r2_buckets': [{'binding': 'SCRY_BACKUPS', 'bucket_name': 'source-private'}]}
ENV = {'GITHUB_RUN_ID': '321', 'GITHUB_RUN_ATTEMPT': '1', 'GITHUB_REPOSITORY': 'misty-step/scry', 'GITHUB_REF': 'refs/heads/master', 'GITHUB_EVENT_NAME': 'workflow_dispatch', 'GITHUB_ACTOR': 'github-actions[bot]'}
def approval():
    return dict(approved=True,scope='one-cloud-only-recovery-drill', approval_id='d'*32, reviewed_source=SOURCE,
                release_version='0.9.0', expires_ms=int(time.time()*1000)+7200000, one_time_cents=2000, failed_cleanup_storage_cents_per_month=25)

def artifact(root):
    modules = {'index.js': b'export default {};', 'index_bg.wasm': b'synthetic frozen wasm'}
    (root / 'worker').mkdir()
    for name, body in modules.items(): (root / 'worker' / name).write_bytes(body)
    (root / 'proof.json').write_text(json.dumps({'revision': SOURCE, 'source_state': 'committed', 'worker': {n: sha(b) for n, b in modules.items()}}))
    tooling = {'recovery/cloud-restore.mjs': b'export class RecoveryBridge {}',
               'recovery/target-adapter.mjs': b'export class LearningSpace {}'}
    (root / 'source.json').write_text(json.dumps({'files': [{'path': n, 'sha256': sha(b)} for n,b in tooling.items()]}))
    with tarfile.open(root / 'source.tar.gz', 'w:gz') as t:
        for n,b in tooling.items():
            m=tarfile.TarInfo(n);m.size=len(b);t.addfile(m,io.BytesIO(b))

class Cloud:
    def __init__(self, fail=None):
        self.scripts = {}; self.buckets = {}; self.namespaces = set(); self.calls = []; self.fail = fail
        self.external_calls = []
    def __call__(self, path, method='GET', **kw):
        self.calls.append((path, method, kw))
        if self.fail: self.fail(path, method, kw)
        if path.startswith('/workers/domains?'): return []
        if path == '/workers/subdomain': return {'subdomain': 'synthetic-existing'}
        if path.startswith('/workers/durable_objects/namespaces/'):
            identity = path.split('/')[4]
            return [] if identity in self.namespaces else None
        if path.startswith('/workers/scripts/'):
            name, _, tail = path[len('/workers/scripts/'):].partition('/')
            current = self.scripts.get(name)
            if tail == 'settings': return current['settings'] if current else None
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
                self.scripts[name] = {'modules': fields, 'settings': {'bindings': bindings, 'limits': metadata.get('limits'), 'usage_model': metadata.get('usage_model'), 'observability': metadata.get('observability'), 'logpush': metadata.get('logpush')}}
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
        assert url.startswith('https://scry-recovery-') and url.endswith('.synthetic-existing.workers.dev/restore')
        if kw['headers'].get('Authorization') in (None, 'Bearer synthetic-wrong-key'):
            assert not has_source, 'hosted refusal must precede learner binding'
            return 403, b'{}'
        assert has_source
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
            got=R.claim(approval(),RECEIPT,c.request,'synthetic-private',receipt_path=saved)
            self.assertEqual(got['github_run_id'],'321');self.assertTrue(c.assert_path)
            self.assertEqual([m for _,m in c.paths],['GET','PUT','GET'])
            with self.assertRaisesRegex(R.RecoveryBlocked,'ALREADY_CLAIMED'):R.claim(approval(),RECEIPT,c.request,'synthetic-private',receipt_path=saved)
            self.assertEqual([m for _,m in c.paths].count('PUT'),1)
            self.assertTrue(all('/operations/recovery-claims/' in p for p,_ in c.paths))

    def test_expiry_and_billing_month_guard_refuse_before_claim(self):
        stamp=int(R.datetime(2026,10,31,12,tzinfo=R.timezone.utc).timestamp()*1000)
        with mock.patch.dict(os.environ,ENV,clear=True):
            with self.assertRaisesRegex(R.RecoveryBlocked,'BILLING_PERIOD'):
                R.admit(RECEIPT,approval=approval()|{'expires_ms':stamp+7200000},now_ms=stamp)
            with self.assertRaises(R.RecoveryBlocked):R.admit(RECEIPT,approval=approval()|{'expires_ms':0})
            short=approval()|{'expires_ms':int(time.time()*1000)+60000}
            with self.assertRaises(R.RecoveryBlocked):R.admit(RECEIPT,approval=short)
            self.assertEqual(R.admit(RECEIPT,approval=short,starting=False),short)

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
            R.claim(a,RECEIPT,c.request,'synthetic-private',receipt_path=saved)
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

    def run_fixture(self, cloud, change=None):
        with tempfile.TemporaryDirectory() as d, mock.patch.dict(os.environ, ENV, clear=True):
            root=Path(d);artifact(root)
            if change: change(root)
            return R.run(CONFIG,root,PIN,cloud,approval=approval(),receipt=RECEIPT,external=cloud.external)

    def test_real_orchestration_uploads_frozen_bytes_and_deletes_only_created_data(self):
        cloud=Cloud(); receipt=self.run_fixture(cloud)
        self.assertEqual(receipt['status'],'verified');self.assertTrue(receipt['cleanup_verified'])
        self.assertEqual(cloud.scripts,{});self.assertEqual(cloud.namespaces,set());self.assertEqual(cloud.buckets,{})
        self.assertLessEqual(len(cloud.calls),128);self.assertEqual(len(cloud.external_calls),3)
        self.assertFalse(any('/source-private/objects/' in p for p,m,k in cloud.calls))
        self.assertFalse(any(m=='POST' and p.endswith('/subdomain') for p,m,k in cloud.calls))

    def test_oversize_or_invalid_page_proof_never_releases_and_still_cleans(self):
        for field, value in [('target_database_bytes', 32*1024*1024+1), ('target_page_size', 8192)]:
            cloud=Cloud(); original=cloud.external
            def external(url, _field=field, _value=value, **kw):
                status, body=original(url,**kw)
                if status == 200:
                    proof=json.loads(body); proof[_field]=_value; body=json.dumps(proof).encode()
                return status, body
            cloud.external=external
            with self.subTest(field=field), self.assertRaisesRegex(R.RecoveryBlocked, 'RECOVERY_NOT_VERIFIED'):
                self.run_fixture(cloud)
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
            with self.assertRaisesRegex(R.RecoveryBlocked, 'ARCHIVE_BYTES_EXCEED_LIMIT size=1048577 limit=1048576'):
                R.run(CONFIG,root,PIN|{'size':1024*1024+1},cloud,approval=approval(),receipt=RECEIPT,external=cloud.external)
            keys.assert_not_called();self.assertEqual(cloud.calls,[])

    def test_malformed_etag_refuses_before_cloud_or_credential_generation(self):
        cloud=Cloud()
        with tempfile.TemporaryDirectory() as d,mock.patch.dict(os.environ,ENV,clear=True),mock.patch.object(R.secrets,'token_hex') as keys:
            root=Path(d);artifact(root)
            for etag in ('"'+'1'*32,'1'*32+'"'):
                with self.assertRaisesRegex(R.RecoveryBlocked,'SOURCE_PIN'):
                    R.run(CONFIG,root,PIN|{'etag':etag},cloud,approval=approval(),receipt=RECEIPT,external=cloud.external)
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
