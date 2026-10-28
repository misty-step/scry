#!/usr/bin/env python3
"""Production promotion refuses binding drift before upload and records unchanged serving edges, offline."""
from contextlib import contextmanager
from copy import deepcopy
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import tomllib
import unittest
from unittest import mock
from urllib.parse import parse_qs, urlsplit

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('scry_promote', ROOT / 'scripts/scry-promote.py')
PROMOTE = importlib.util.module_from_spec(spec)
spec.loader.exec_module(PROMOTE)
CONFIG = tomllib.loads((ROOT / 'deploy/rust-production.toml').read_text())
NAME = CONFIG['name']
# Shapes read back from the account settings endpoint, 2026-10-20 (values of secrets are never returned).
LIVE = [
    {'name': 'LEARNING_SPACE', 'type': 'durable_object_namespace', 'class_name': 'LearningSpace', 'namespace_id': 'a7b038df00f247dfbcf2d091ee29a187'},
    {'name': 'SCRY_ASSETS', 'type': 'r2_bucket', 'bucket_name': 'scry-rust-assets'},
    {'name': 'SCRY_BACKUPS', 'type': 'r2_bucket', 'bucket_name': 'scry-rust-backups'},
    {'name': 'SPACE_NAME', 'type': 'plain_text', 'text': CONFIG['vars']['SPACE_NAME']},
    {'name': 'SCRY_ENV', 'type': 'plain_text', 'text': 'production'},
    {'name': 'INTERNAL_KEY', 'type': 'secret_text'},
]
DOMAIN = {'zone_id': 'f7be06f7426d5532921cc4348746a2b8', 'zone_name': 'scry.study', 'hostname': 'scry.study',
          'service': NAME, 'environment': 'production', 'enabled': True}


def version(etag=None, message=None, trigger=None, number=1):
    annotations = {k: v for k, v in [('workers/message', message), ('workers/triggered_by', trigger)] if v}
    return {'number': number, 'annotations': annotations, 'resources': {'script': {'etag': etag}} if etag else {}}


def drift(name, **change):
    return [dict(b, **change) if b['name'] == name else b for b in LIVE]


class Promotion(unittest.TestCase):
    def test_cloud_gate_preserves_predeployment_primary_reason_and_cleanup_outcomes(self):
        name='scry-recovery-'+'a'*24+'-target'
        error=RuntimeError('RECOVERY_CLEANUP_FAILED')
        error.recovery_phase='restore';error.deployment_attempted=False
        error.recovery_operation='preview-exchange'
        error.cloud_request_attempts=75;error.preview_request_attempts=1
        error.primary_reason='RECOVERY_LIMITS_UNVERIFIED';error.retained_resources=[name]
        error.cleanup_outcomes=[{'resource':name,'step':'namespace_identity','status':'unverified','reason':'RECOVERY_MIGRATION_OUTCOME_UNCERTAIN'}]
        module=mock.Mock();module.gate.side_effect=error
        module.load_module.return_value.RECOVERY_OPERATIONS=frozenset({'preview-exchange'})
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'promotion.json';receipt={'source':'a'*40,'stage':'prepared'}
            with mock.patch.object(PROMOTE,'RECEIPT',path),mock.patch.object(PROMOTE.importlib.util,'spec_from_file_location') as spec, \
                    mock.patch.object(PROMOTE.importlib.util,'module_from_spec',return_value=module):
                spec.return_value.loader.exec_module.return_value=None
                with self.assertRaises(RuntimeError) as caught:PROMOTE.cloud_gate(receipt,{}, {}, [], None,after_restore=mock.Mock())
            self.assertIs(caught.exception,error)
            saved=json.loads(path.read_text());cleanup=json.loads((path.parent/'recovery-cleanup.json').read_text())
            self.assertEqual(saved['recovery_phase'],'restore');self.assertFalse(saved['deployment_attempted'])
            self.assertEqual(saved['recovery_primary_reason'],'RECOVERY_LIMITS_UNVERIFIED')
            self.assertEqual(saved['recovery_operation'],'preview-exchange')
            self.assertEqual(saved['recovery_cloud_request_attempts'],75);self.assertEqual(saved['recovery_preview_request_attempts'],1)
            self.assertEqual(cleanup['primary_reason'],'RECOVERY_LIMITS_UNVERIFIED')
            self.assertEqual(cleanup['recovery_phase'],'restore')
            self.assertEqual(cleanup['recovery_operation'],'preview-exchange')
            self.assertEqual(cleanup['recovery_cloud_request_attempts'],75);self.assertEqual(cleanup['recovery_preview_request_attempts'],1)
            self.assertEqual(cleanup['cleanup_outcomes'],error.cleanup_outcomes)

    def test_cloud_gate_flushes_sanitized_failure_receipt_before_reraising(self):
        name='scry-recovery-'+'b'*24+'-target'
        outcomes=[{'resource':name,'step':'namespace_identity','status':'unverified',
                   'reason':'RECOVERY_MIGRATION_OUTCOME_UNCERTAIN'}]
        for phase,attempted in [('restore',False),('recurrence',True)]:
            error=RuntimeError('PRIVATE_EXCEPTION_BODY')
            error.recovery_phase=phase;error.deployment_attempted=False
            error.primary_reason='RECOVERY_LOGGING_UNVERIFIED';error.retained_resources=[name]
            error.cleanup_outcomes=outcomes
            error.privacy_observations=[{'resource':name,'observability':None,'logpush':False,'tail_consumers':[],
                'disable_acknowledged':True,'assurance':'audited-no-learner-content-emissions'}]
            module=mock.Mock();module.gate.side_effect=error
            with self.subTest(phase=phase),tempfile.TemporaryDirectory() as directory:
                path=Path(directory)/'promotion.json'
                receipt={'source':'a'*40,'deployment_attempted':attempted,'private':'PRIVATE_RECEIPT_BODY'}
                observed=[]
                def emitted(body,*,flush):
                    self.assertTrue(path.exists())
                    observed.append((body,flush))
                with mock.patch.object(PROMOTE,'RECEIPT',path), \
                        mock.patch.object(PROMOTE.importlib.util,'spec_from_file_location') as spec, \
                        mock.patch.object(PROMOTE.importlib.util,'module_from_spec',return_value=module), \
                        mock.patch('builtins.print',side_effect=emitted) as output:
                    spec.return_value.loader.exec_module.return_value=None
                    with self.assertRaises(RuntimeError) as caught:
                        PROMOTE.cloud_gate(receipt,{}, {}, [], None,after_restore=mock.Mock())
                self.assertIs(caught.exception,error)
                self.assertEqual(json.loads(path.read_text())['recovery_privacy_observations'],error.privacy_observations)
                output.assert_called_once()
                body,flush=observed[0]
                self.assertIs(flush,True)
                self.assertEqual(json.loads(body),{'error':'CLOUD_RECOVERY_NOT_VERIFIED',
                    'deployment_attempted':attempted,'recovery_phase':phase,
                    'primary_reason':'RECOVERY_LOGGING_UNVERIFIED','recovery_operation':None,
                    'recovery_cloud_request_attempts':None,'recovery_preview_request_attempts':None,'cleanup_outcomes':outcomes})
                self.assertNotIn('PRIVATE',body)
                self.assertNotIn('source',json.loads(body))

    def test_cloud_gate_whitelists_operation_before_private_and_public_records(self):
        name='scry-recovery-'+'c'*24+'-bridge'
        for operation,expected in [('preview-exchange','preview-exchange'),
                                   ('PRIVATE_OPERATION_SECRET',None),
                                   ('PREVIEW_EXCHANGE',None),
                                   ({'private':'PRIVATE_OPERATION_SECRET'},None),
                                   (['PRIVATE_OPERATION_SECRET'],None)]:
            error=RuntimeError('PRIVATE_EXCEPTION_BODY')
            error.recovery_phase='restore';error.recovery_operation=operation
            error.primary_reason='RECOVERY_OPERATION_UNVERIFIED';error.retained_resources=[name]
            error.cleanup_outcomes=[]
            module=mock.Mock();module.gate.side_effect=error
            module.load_module.return_value.RECOVERY_OPERATIONS=frozenset({'preview-exchange'})
            with self.subTest(operation=operation),tempfile.TemporaryDirectory() as directory:
                path=Path(directory)/'promotion.json'
                with mock.patch.object(PROMOTE,'RECEIPT',path), \
                        mock.patch.object(PROMOTE.importlib.util,'spec_from_file_location') as spec, \
                        mock.patch.object(PROMOTE.importlib.util,'module_from_spec',return_value=module), \
                        mock.patch('builtins.print') as output:
                    spec.return_value.loader.exec_module.return_value=None
                    with self.assertRaises(RuntimeError) as caught:
                        PROMOTE.cloud_gate({'source':'a'*40},{}, {}, [], None,after_restore=mock.Mock())
                self.assertIs(caught.exception,error)
                public=output.call_args.args[0]
                saved=path.read_text();cleanup=(path.parent/'recovery-cleanup.json').read_text()
                for record in (public,saved,cleanup):
                    self.assertEqual(json.loads(record)['recovery_operation'],expected)
                    self.assertNotIn('PRIVATE',record)
                    self.assertNotIn('PREVIEW_EXCHANGE',record)
                output.assert_called_once()
                self.assertEqual(json.loads(saved)['recovery_primary_reason'],'RECOVERY_OPERATION_UNVERIFIED')

    def test_cloud_gate_sanitizes_request_counts_without_replacing_primary_failure(self):
        name='scry-recovery-'+'d'*24+'-bridge'
        cases=[(0,0,0,0),(131,6,131,6),(-1,-1,None,None),(132,7,None,None),
               (True,False,None,None),(1.0,2.0,None,None),
               ('PRIVATE_COUNT_SECRET',{'private':'PRIVATE_COUNT_SECRET'},None,None),
               (['PRIVATE_COUNT_SECRET'],None,None,None),(20,7,20,None),(132,2,None,2)]
        for cloud,preview,expected_cloud,expected_preview in cases:
            error=RuntimeError('PRIVATE_EXCEPTION_BODY');error.recovery_phase='restore'
            error.primary_reason='RECOVERY_OPERATION_UNVERIFIED';error.retained_resources=[name]
            error.cleanup_outcomes=[];error.cloud_request_attempts=cloud;error.preview_request_attempts=preview
            module=mock.Mock();module.gate.side_effect=error
            with self.subTest(cloud=cloud,preview=preview),tempfile.TemporaryDirectory() as directory:
                path=Path(directory)/'promotion.json'
                with mock.patch.object(PROMOTE,'RECEIPT',path), \
                        mock.patch.object(PROMOTE.importlib.util,'spec_from_file_location') as spec, \
                        mock.patch.object(PROMOTE.importlib.util,'module_from_spec',return_value=module), \
                        mock.patch('builtins.print') as output:
                    spec.return_value.loader.exec_module.return_value=None
                    with self.assertRaises(RuntimeError) as caught:
                        PROMOTE.cloud_gate({'source':'a'*40},{}, {}, [], None,after_restore=mock.Mock())
                self.assertIs(caught.exception,error)
                records=(output.call_args.args[0],path.read_text(),(path.parent/'recovery-cleanup.json').read_text())
                for record in records:
                    parsed=json.loads(record)
                    self.assertEqual(parsed['recovery_cloud_request_attempts'],expected_cloud)
                    self.assertEqual(parsed['recovery_preview_request_attempts'],expected_preview)
                    self.assertEqual(parsed.get('recovery_primary_reason',parsed.get('primary_reason')),
                                     'RECOVERY_OPERATION_UNVERIFIED')
                    self.assertNotIn('PRIVATE',record)
                output.assert_called_once()

    def test_cloud_gate_does_not_print_unvalidated_failure_fields(self):
        for phase,resource in [(None,'scry-recovery-'+'b'*24+'-target'),('restore','PRIVATE_RESOURCE')]:
            error=RuntimeError('PRIVATE_EXCEPTION_BODY');error.recovery_phase=phase
            error.primary_reason='RECOVERY_LOGGING_UNVERIFIED'
            error.cleanup_outcomes=[{'resource':resource,'step':'namespace_identity','status':'unverified',
                                    'reason':'RECOVERY_MIGRATION_OUTCOME_UNCERTAIN'}]
            module=mock.Mock();module.gate.side_effect=error
            with self.subTest(phase=phase),tempfile.TemporaryDirectory() as directory, \
                    mock.patch.object(PROMOTE,'RECEIPT',Path(directory)/'promotion.json'), \
                    mock.patch.object(PROMOTE.importlib.util,'spec_from_file_location') as spec, \
                    mock.patch.object(PROMOTE.importlib.util,'module_from_spec',return_value=module), \
                    mock.patch('builtins.print') as output:
                spec.return_value.loader.exec_module.return_value=None
                with self.assertRaises((RuntimeError,AssertionError)):
                    PROMOTE.cloud_gate({'source':'a'*40},{}, {}, [], None,after_restore=mock.Mock())
                output.assert_not_called()

    def test_declared_storage_matches_live_identity(self):
        self.assertEqual(PROMOTE.storage(LIVE), PROMOTE.declared(CONFIG))

    def test_storage_drift_is_detected(self):
        for live in [drift('SCRY_BACKUPS', bucket_name='other'), drift('SCRY_ASSETS', bucket_name='scry-rust-backups'),
                     drift('LEARNING_SPACE', class_name='Other'), drift('LEARNING_SPACE', script_name='foreign'),
                     [b for b in LIVE if b['name'] != 'SCRY_ASSETS'], LIVE + [{'name': 'EXTRA', 'type': 'r2_bucket', 'bucket_name': 'x'}]]:
            with self.subTest(live=live):
                self.assertNotEqual(PROMOTE.storage(live), PROMOTE.declared(CONFIG))

    def test_edges_filter_service_and_detect_any_change(self):
        www = dict(DOMAIN, hostname='www.scry.study', id='ignored', cert_id='ignored')
        seen = PROMOTE.edges([DOMAIN, www, dict(DOMAIN, service='other', hostname='x.scry.study')], [], NAME)
        self.assertEqual(seen, {'custom_domains': [['scry.study', 'scry.study', DOMAIN['zone_id'], 'production', True],
                                                   ['www.scry.study', 'scry.study', DOMAIN['zone_id'], 'production', True]], 'routes': []})
        self.assertEqual(PROMOTE.edges([www, DOMAIN], [], NAME), seen)
        for domains, routes in [([DOMAIN], []), ([DOMAIN, dict(www, enabled=False)], []), ([DOMAIN, dict(www, service='other')], []),
                                ([DOMAIN, dict(www, zone_id='z')], []), ([DOMAIN, www], [{'pattern': 'scry.study/*'}])]:
            with self.subTest(domains=domains, routes=routes):
                self.assertNotEqual(PROMOTE.edges(domains, routes, NAME), seen)

    def test_declared_refuses_foreign_worker_with_reason(self):
        config = deepcopy(CONFIG)
        config['durable_objects']['bindings'][0]['script_name'] = 'other-worker'
        with self.assertRaisesRegex(AssertionError, 'Durable Object binding points at another Worker'):
            PROMOTE.declared(config)

    @contextmanager
    def deployment(self, domains, routes, env='restore', history=None):
        requested = []
        # Newest first, as the deployments endpoint returns them: (deployment id, version id, version body).
        history = history or [('previous', 'previous', {})]
        bindings = drift('SCRY_ENV', text=env) + [
            {'name': name, 'type': 'secret_text'} for name in ('OWNER_SUBJECT', 'BACKUP_TRIGGER_KEY')]
        with tempfile.TemporaryDirectory() as directory:
            receipt = Path(directory) / 'promotion.json'
            receipt.write_text(json.dumps({'source': 'a' * 40}))
            with mock.patch.object(PROMOTE, 'RECEIPT', receipt), mock.patch.dict('os.environ', CLOUDFLARE_API_TOKEN='synthetic'), \
                    mock.patch.object(PROMOTE.subprocess, 'run') as run:
                def urlopen(request, timeout):
                    path = request.full_url.split('/accounts/' + CONFIG['account_id'], 1)[1]
                    requested.append(path)
                    script = '/workers/scripts/' + NAME
                    if path == script + '/settings': result = {'bindings': bindings}
                    elif path.endswith('/domains/managed'): result = {'enabled': False}
                    elif path.endswith('/domains/custom'): result = {'domains': []}
                    elif '/objects?' in path: result = [{'key': 'synthetic-snapshot'}]
                    elif path.split('?', 1)[0] == script + '/deployments':
                        result = {'deployments': [{'id': d, 'versions': [{'version_id': v, 'percentage': 100}]} for d, v, _ in history]}
                        if '?' in path and parse_qs(urlsplit(path).query)['page'] != ['1']:
                            result = {'deployments': []}
                    elif path.startswith(script + '/versions/') and path.rsplit('/', 1)[1] in {v for _, v, _ in history}:
                        result = next(body for _, v, body in history if v == path.rsplit('/', 1)[1])
                    elif path == '/workers/domains?service=' + NAME: result = domains if run.called else [DOMAIN]
                    elif path == script + '/routes': result = routes if run.called else []
                    else: self.fail('Unexpected request: ' + path)
                    return io.BytesIO(json.dumps({'success': True, 'result': result}).encode())
                with mock.patch.object(PROMOTE.urllib.request, 'urlopen', urlopen), \
                        mock.patch.object(PROMOTE, 'cloud_gate', side_effect=lambda *a, after_restore: (after_restore(), {'recovery': {'recurrence_verified': True, 'cleanup_verified': True}})[1]):
                    yield run, requested, receipt

    def test_deploy_refuses_serving_edge_drift(self):
        for domains, routes in [([dict(DOMAIN, hostname='changed.scry.study')], []),
                                ([DOMAIN], [{'pattern': 'scry.study/*'}])]:
            with self.subTest(domains=domains, routes=routes), self.deployment(domains, routes) as (run, requested, receipt):
                with self.assertRaisesRegex(AssertionError, '^Custom domains or routes changed during promotion$'):
                    PROMOTE.deploy()
                run.assert_called_once()
                self.assertEqual(json.loads(receipt.read_text())['stage'], 'upload_attempted_recurrence_pending')

    def test_deploy_observes_serving_edges_before_wrangler(self):
        with self.deployment([DOMAIN], []) as (run, requested, receipt):
            def wrangler(*args, **kwargs):
                self.assertEqual(requested[-3:-1], ['/workers/domains?service=' + NAME,
                                                   '/workers/scripts/' + NAME + '/routes'])
                self.assertIn('/objects?', requested[-1])
                self.assertEqual(args[0][1], 'deploy')
                raise RuntimeError('wrangler intercepted')
            run.side_effect = wrangler
            with self.assertRaisesRegex(RuntimeError, '^wrangler intercepted$'):
                PROMOTE.deploy()
            run.assert_called_once()

    def test_deploy_promotes_over_a_secret_only_change_to_a_known_release(self):
        # 2026-10-20: rotating OWNER_SUBJECT made an unannotated version with the release's exact script.
        released = 'b' * 40
        history = [('secret-change', 'secret-only', version(etag='same', trigger='secret', number=2)),
                   ('release', 'released', version(etag='same', message='scry-rust ' + released))]
        with self.deployment([DOMAIN], [], env='production', history=history) as (run, requested, receipt):
            run.side_effect = lambda *args, **kwargs: (_ for _ in ()).throw(RuntimeError('stop at ' + args[0][1]))
            with self.assertRaisesRegex(RuntimeError, '^stop at merge-base$'):
                PROMOTE.deploy()
            self.assertEqual(run.call_args.args[0], ['git', 'merge-base', '--is-ancestor', released, 'a' * 40])

    def test_deploy_uploads_unless_the_live_version_itself_names_the_source(self):
        # An annotated source without a prior full receipt must not skip recurrence.
        # A secret-only version still lacks the exact annotation and uploads.
        source = 'a' * 40
        for label, history, uploads in [
                ('annotated', [('release', 'released', version(etag='same', message='scry-rust ' + source))], False),
                ('secret-only', [('secret-change', 'secret-only', version(etag='same', trigger='secret', number=2)),
                                 ('release', 'released', version(etag='same', message='scry-rust ' + source))], True)]:
            with self.subTest(label), self.deployment([DOMAIN], [], env='production', history=history) as (run, requested, receipt):
                def command(args, **kwargs):
                    if args[0] != 'git': raise RuntimeError('upload ' + args[1])
                run.side_effect = command
                if uploads:
                    with self.assertRaisesRegex(RuntimeError, '^upload deploy$'):
                        PROMOTE.deploy()
                else:
                    # Freshly prepared receipts cannot turn an unverified upload into success.
                    with self.assertRaisesRegex(AssertionError, 'lacks verified recovery receipt'):
                        PROMOTE.deploy()
                self.assertEqual(any(c.args[0][0] != 'git' for c in run.call_args_list), uploads)

    def test_deploy_refuses_an_untraceable_live_version(self):
        for history in [[('secret-change', 'secret-only', version(etag='changed', trigger='secret', number=2)),
                         ('release', 'released', version(etag='same', message='scry-rust ' + 'b' * 40))],
                        [('secret-change', 'secret-only', version(etag='same', trigger='secret', number=2))],
                        [('manual', 'manual', version(etag='same', trigger='version_upload')),
                         ('release', 'released', version(etag='same', message='scry-rust ' + 'b' * 40))]]:
            with self.subTest(history=history), self.deployment([DOMAIN], [], env='production', history=history) as (run, requested, receipt):
                with self.assertRaisesRegex(AssertionError, '^Unrecognized live release$'):
                    PROMOTE.deploy()
                run.assert_not_called()

    def test_live_release_resolves_only_secret_changes_with_the_same_script(self):
        versions = {'released': version(etag='same', message='scry-rust ' + 'b' * 40),
                    'secret': version(etag='same', trigger='secret', number=2),
                    'secret-again': version(etag='same', trigger='secret', number=3),
                    'other-script': version(etag='other', trigger='secret', number=2),
                    'upload': version(etag='same', trigger='version_upload'),
                    'no-etag': version(trigger='secret', number=2),
                    'malformed': version(etag='same', message='scry-rust ' + 'b' * 39)}
        live = lambda *ids: PROMOTE.live_release([{'versions': [{'version_id': i, 'percentage': 100}]} for i in ids], versions.__getitem__)
        self.assertEqual(live('released'), ('b' * 40, True))
        self.assertEqual(live('secret', 'released'), ('b' * 40, False))
        self.assertEqual(live('secret-again', 'secret', 'released'), ('b' * 40, False))
        for ids in [('secret',), ('other-script', 'released'), ('upload', 'released'), ('no-etag', 'released'),
                    ('malformed',), ('secret', 'malformed'), ('secret', 'upload', 'released'), ()]:
            with self.subTest(ids=ids):
                self.assertIsNone(live(*ids))

    def test_deploy_refuses_config_drift_before_any_upload(self):
        def urlopen(request, timeout):
            assert request.full_url.endswith('/workers/scripts/' + NAME + '/settings'), request.full_url
            return io.BytesIO(json.dumps({'success': True, 'result': {'bindings': drift('SCRY_BACKUPS', bucket_name='other')}}).encode())
        with tempfile.TemporaryDirectory() as directory:
            receipt = Path(directory) / 'promotion.json'
            receipt.write_text(json.dumps({'source': 'a' * 40}))
            with mock.patch.object(PROMOTE, 'RECEIPT', receipt), mock.patch.dict('os.environ', CLOUDFLARE_API_TOKEN='synthetic'), \
                    mock.patch.object(PROMOTE.urllib.request, 'urlopen', urlopen), mock.patch.object(PROMOTE.subprocess, 'run') as run:
                with self.assertRaisesRegex(AssertionError, 'refusing before upload'):
                    PROMOTE.deploy()
                run.assert_not_called()


def deployment(identity, *versions):
    return {'id': identity, 'versions': [{'version_id': name, 'percentage': percent} for name, percent in versions]}


class DeploymentSafety(unittest.TestCase):
    """Real deploy(), receipt and Git ancestry; only HTTP and Wrangler effects are synthetic."""
    @classmethod
    def setUpClass(cls):
        cls.directory = tempfile.TemporaryDirectory()
        cls.root = Path(cls.directory.name)
        cls.real_run = staticmethod(subprocess.run)
        env = {'PATH': os.environ['PATH'], 'HOME': str(cls.root), 'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null'}
        def git(*args):
            return cls.real_run(['git', '-C', str(cls.root), '-c', 'user.name=Synthetic proof', '-c',
                                 'user.email=proof@example.invalid', *args], env=env, check=True, capture_output=True, text=True).stdout.strip()
        git('init', '--quiet')
        git('commit', '--quiet', '--allow-empty', '-m', 'release A')
        cls.a = git('rev-parse', 'HEAD')
        git('commit', '--quiet', '--allow-empty', '-m', 'release B')
        cls.b = git('rev-parse', 'HEAD')
        git('commit', '--quiet', '--allow-empty', '-m', 'candidate C')
        cls.c = git('rev-parse', 'HEAD')
        cls.divergent = git('commit-tree', git('rev-parse', 'HEAD^{tree}'), '-p', cls.a, '-m', 'divergent')

    @classmethod
    def tearDownClass(cls):
        cls.directory.cleanup()

    @contextmanager
    def fixture(self, history, source=None, *, recheck=None, final=None, page_size=100, page_change=None, uploaded_only=False, version_change=None, verified=False):
        source = source or self.b
        current = deepcopy(history)
        versions = {'A': version(etag='old', message='scry-rust ' + self.a),
                    'B': version(etag='new', message='scry-rust ' + self.b, number=3),
                    'C': version(etag='candidate', message='scry-rust ' + self.c, number=5),
                    'secret-A': version(etag='old', trigger='secret', number=2),
                    'secret-B': version(etag='new', trigger='secret', number=4)}
        if version_change: version_change(versions)
        if uploaded_only: versions['not-deployed-C'] = version(etag='candidate', message='scry-rust ' + self.c)
        bindings = deepcopy(LIVE) + [{'name': n, 'type': 'secret_text'} for n in ('OWNER_SUBJECT', 'BACKUP_TRIGGER_KEY')]
        modules = {'index.js': b'export default {};', 'index_bg.wasm': b'synthetic wasm'}
        raw = b''.join(b'--fixture\r\nContent-Disposition: form-data; name="' + n.encode() + b'"; filename="' + n.encode() + b'"\r\n\r\n' + b + b'\r\n' for n, b in modules.items()) + b'--fixture--\r\n'
        requests, uploads = [], []
        reads = 0
        with tempfile.TemporaryDirectory() as directory:
            out = Path(directory)
            receipt = out / 'promotion.json'
            initial = {'source': source, 'stage': 'artifact_verified'}
            if verified:
                initial.update(stage='exact_artifact_promoted', cloud_recovery={'recovery': {'recurrence_verified': True, 'cleanup_verified': True}})
            receipt.write_text(json.dumps(initial))
            (out / 'proof.json').write_text(json.dumps({'worker': {n: PROMOTE.sha(b) for n, b in modules.items()}}))
            def urlopen(request, timeout):
                nonlocal reads
                self.assertEqual(request.get_method(), 'GET')
                prefix = 'https://api.cloudflare.com/client/v4/accounts/' + CONFIG['account_id']
                self.assertTrue(request.full_url.startswith(prefix))
                path = request.full_url[len(prefix):]
                requests.append(path)
                script = '/workers/scripts/' + NAME
                info = None
                if path == script + '/settings': result = {'bindings': bindings}
                elif path.endswith('/domains/managed'): result = {'enabled': False}
                elif path.endswith('/domains/custom'): result = {'domains': []}
                elif '/objects?' in path: result = [{'key': 'synthetic-snapshot'}]
                elif path.startswith(script + '/deployments?'):
                    page = int(parse_qs(urlsplit(path).query)['page'][0])
                    batch = current[(page - 1) * page_size:page * page_size]
                    info = {'page': page, 'per_page': page_size, 'count': len(batch),
                            'total_count': len(current), 'total_pages': max(1, (len(current) + page_size - 1) // page_size)}
                    result = {'deployments': deepcopy(batch)}
                    if page_change: result, info = page_change(page, result, info)
                elif path == script + '/deployments':
                    reads += 1
                    observed = recheck if reads == 1 else final
                    result = {'deployments': deepcopy(current if observed is None else observed)}
                elif path.startswith(script + '/versions/'):
                    result = versions[path.rsplit('/', 1)[1]]
                elif path == '/workers/domains?service=' + NAME: result = [DOMAIN]
                elif path == script + '/routes': result = []
                elif path == script + '/subdomain': result = {'enabled': False, 'previews_enabled': False}
                elif path == script + '/content/v2':
                    response = io.BytesIO(raw)
                    response.headers = {'Content-Type': 'multipart/form-data; boundary=fixture'}
                    return response
                else: self.fail('Unexpected request: ' + path)
                return io.BytesIO(json.dumps({'success': True, 'result': result, **({'result_info': info} if info is not None else {})}).encode())
            def command(args, **kwargs):
                if args[:3] == ['git', 'merge-base', '--is-ancestor']:
                    self.assertEqual(kwargs['cwd'], self.root)
                    return self.real_run(args, **kwargs)
                self.assertEqual(args, [str(self.root / 'node_modules/.bin/wrangler'), 'deploy', '--config', str(PROMOTE.CONFIG), '--no-bundle', '--message', 'scry-rust ' + source])
                uploads.append(args)
                versions['uploaded'] = version(etag='new', message='scry-rust ' + source)
                current.insert(0, deployment('uploaded', ('uploaded', 100)))
            with mock.patch.object(PROMOTE, 'ROOT', self.root), mock.patch.object(PROMOTE, 'OUT', out), \
                    mock.patch.object(PROMOTE, 'RECEIPT', receipt), mock.patch.dict(os.environ, {'PATH': os.environ['PATH'], 'CLOUDFLARE_API_TOKEN': 'synthetic'}, clear=True), \
                    mock.patch.object(PROMOTE.urllib.request, 'urlopen', urlopen), mock.patch.object(PROMOTE.subprocess, 'run', side_effect=command), \
                    mock.patch.object(PROMOTE, 'cloud_gate', side_effect=lambda *a, after_restore: (after_restore(), {'recovery': {'recurrence_verified': True, 'cleanup_verified': True}})[1]), \
                    mock.patch('sys.stdout', new_callable=io.StringIO):
                yield uploads, requests, receipt, initial

    def test_full_forward_and_idempotent_receipts_preserve_secret_only_support(self):
        for name, history, uploads in [
            ('forward', [deployment('a', ('A', 100))], 1),
            ('idempotent', [deployment('b', ('B', 100)), deployment('a', ('A', 100))], 0),
            ('secret-only', [deployment('secret', ('secret-B', 100)), deployment('b', ('B', 100)), deployment('a', ('A', 100))], 1),
        ]:
            with self.subTest(name=name), self.fixture(history, uploaded_only=True, verified=uploads == 0) as (called, requests, receipt, _):
                PROMOTE.deploy()
                self.assertEqual(len(called), uploads)
                saved = json.loads(receipt.read_text())
                self.assertEqual(saved['stage'], 'exact_artifact_promoted')
                self.assertEqual(saved['deployment']['versions'][0]['percentage'], 100)
                self.assertFalse(any('not-deployed-C' in p for p in requests))

    def test_cloud_recovery_refusal_prevents_upload_and_success_receipt(self):
        with self.fixture([deployment('a', ('A', 100))]) as (uploads, _, receipt, initial):
            with mock.patch.object(PROMOTE, 'cloud_gate', side_effect=RuntimeError('synthetic cloud gate refused')) as gate:
                with self.assertRaisesRegex(RuntimeError, 'cloud gate refused'):
                    PROMOTE.deploy()
                gate.assert_called_once()
            self.assertEqual(uploads, [])
            self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_already_exact_promoted_release_does_not_spend_on_another_rehearsal(self):
        with self.fixture([deployment('b', ('B', 100))], verified=True) as (uploads, _, receipt, _):
            with mock.patch.object(PROMOTE, 'cloud_gate') as gate:
                PROMOTE.deploy()
                gate.assert_not_called()
            self.assertEqual(uploads, [])
            self.assertEqual(json.loads(receipt.read_text())['stage'], 'exact_artifact_promoted')

    def test_already_deployed_unverified_source_cannot_skip_recovery(self):
        with self.fixture([deployment('b', ('B', 100))]) as (uploads, _, receipt, initial):
            with mock.patch.object(PROMOTE, 'cloud_gate') as gate:
                with self.assertRaisesRegex(AssertionError, 'lacks verified recovery receipt'):
                    PROMOTE.deploy()
                gate.assert_not_called()
            self.assertEqual(uploads, [])
            self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_post_deploy_recurrence_failure_never_writes_success(self):
        with self.fixture([deployment('a', ('A', 100))]) as (uploads, _, receipt, _):
            def gate(*args, after_restore):
                self.assertEqual(uploads, [])
                after_restore()
                self.assertEqual(len(uploads), 1)
                self.assertEqual(json.loads(receipt.read_text())['stage'], 'exact_artifact_uploaded_recurrence_pending')
                raise RuntimeError('synthetic recurrence refused')
            with mock.patch.object(PROMOTE, 'cloud_gate', side_effect=gate):
                with self.assertRaisesRegex(RuntimeError, 'recurrence refused'):
                    PROMOTE.deploy()
            saved = json.loads(receipt.read_text())
            self.assertEqual(saved['stage'], 'exact_artifact_uploaded_recurrence_pending')
            self.assertFalse(saved['recovery_verified'])

    def test_rollback_refuses_duplicate_newer_and_same_source_candidates(self):
        history = [deployment('rollback', ('A', 100)), deployment('b', ('B', 100)), deployment('a', ('A', 100))]
        for source in (self.a, self.b, self.c):
            for secret in (False, True):
                live = ([deployment('secret', ('secret-A', 100))] if secret else []) + history
                with self.subTest(source=source, secret=secret), self.fixture(live, source, page_size=1) as (uploads, _, receipt, initial):
                    with self.assertRaisesRegex(AssertionError, 'Manual rollback'):
                        PROMOTE.deploy()
                    self.assertEqual(uploads, [])
                    self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_replayed_secret_version_cannot_inherit_a_later_release_annotation(self):
        history = [deployment('rollback', ('secret-A', 100)), deployment('b', ('B', 100)),
                   deployment('original-secret', ('secret-A', 100)), deployment('a', ('A', 100))]
        def equal_etags(versions):
            versions['B']['resources']['script']['etag'] = 'old'
        for source in (self.b, self.c):
            with self.subTest(source=source), self.fixture(history, source, page_size=1, version_change=equal_etags) as (uploads, _, receipt, initial):
                with self.assertRaisesRegex(AssertionError, 'Manual rollback'): PROMOTE.deploy()
                self.assertEqual(uploads, [])
                self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_secret_provenance_refuses_missing_conflicting_or_ambiguous_chronology(self):
        history = [deployment('secret', ('secret-B', 100)), deployment('b', ('B', 100)), deployment('a', ('A', 100))]
        changes = [
            lambda v: v['secret-B'].pop('number'),
            lambda v: v['B'].pop('number'),
            lambda v: v['A'].pop('number'),
            lambda v: v['secret-B'].update(number=True),
            lambda v: v['B'].update(number='3'),
            lambda v: v['B'].update(number=0),
            lambda v: v['B'].update(number=4),
            lambda v: v['B'].update(number=5),
            lambda v: v['A']['resources']['script'].update(etag='new'),
        ]
        for index, change in enumerate(changes):
            with self.subTest(change=index), self.fixture(history, version_change=change) as (uploads, _, receipt, initial):
                with self.assertRaisesRegex(AssertionError, 'Unrecognized live release'): PROMOTE.deploy()
                self.assertEqual(uploads, [])
                self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_secret_chain_refuses_reversed_creation_order(self):
        history = [deployment('new-secret', ('secret-B', 100)), deployment('old-secret', ('secret-A', 100)),
                   deployment('noncausal-secret', ('middle-secret', 100)), deployment('a', ('A', 100))]
        def reversed_chain(versions):
            versions['secret-B']['resources']['script']['etag'] = 'old'
            versions['middle-secret'] = version(etag='old', trigger='secret', number=3)
        with self.fixture(history, version_change=reversed_chain) as (uploads, _, receipt, initial):
            with self.assertRaisesRegex(AssertionError, 'Unrecognized live release'): PROMOTE.deploy()
            self.assertEqual(uploads, [])
            self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_unambiguous_multiple_secret_versions_and_redeployments_still_promote(self):
        history = [deployment('new-secret', ('secret-B', 100)), deployment('old-secret', ('secret-A', 100)),
                   deployment('repeat-release', ('A', 100)), deployment('a', ('A', 100))]
        def same_source(versions):
            versions['secret-B']['resources']['script']['etag'] = 'old'
        with self.fixture(history, version_change=same_source) as (uploads, _, receipt, _):
            PROMOTE.deploy()
            self.assertEqual(len(uploads), 1)
            self.assertEqual(json.loads(receipt.read_text())['stage'], 'exact_artifact_promoted')

    def test_stale_and_divergent_targets_still_refuse(self):
        for source in (self.a, self.divergent):
            with self.subTest(source=source), self.fixture([deployment('b', ('B', 100))], source) as (uploads, _, receipt, initial):
                with self.assertRaises(subprocess.CalledProcessError): PROMOTE.deploy()
                self.assertEqual(uploads, [])
                self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_split_and_malformed_traffic_refuse_at_every_read(self):
        invalid = [[('B', 10), ('A', 90)], [('A', 90), ('B', 10)], [('B', 100), ('A', 0)],
                   [('B', 50)], [('B', '100')], [('B', True)], [('B', None)], [], [('B', 100), ('B', 0)]]
        pure = deployment('live', ('B', 100))
        for entries in invalid:
            broken = deployment('live', *entries)
            for phase in ('before', 'recheck', 'final'):
                options = {} if phase == 'before' else {phase: [broken]}
                with self.subTest(entries=entries, phase=phase), self.fixture([broken if phase == 'before' else pure], verified=True, **options) as (uploads, _, receipt, initial):
                    with self.assertRaisesRegex(AssertionError, 'single version at 100%|deployment traffic'):
                        PROMOTE.deploy()
                    self.assertEqual(uploads, [])
                    self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_secret_chain_cannot_trace_through_a_split(self):
        history = [deployment('secret', ('secret-B', 100)), deployment('split', ('B', 50), ('A', 50))]
        with self.fixture(history) as (uploads, _, receipt, initial):
            with self.assertRaisesRegex(AssertionError, 'single version at 100%'): PROMOTE.deploy()
            self.assertEqual(uploads, [])
            self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_historical_split_inspects_every_version_that_served(self):
        history = [deployment('rollback', ('A', 100)), deployment('split', ('A', 90), ('B', 10))]
        with self.fixture(history) as (uploads, _, receipt, initial):
            with self.assertRaisesRegex(AssertionError, 'Manual rollback'): PROMOTE.deploy()
            self.assertEqual(uploads, [])
            self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_changed_deployment_id_still_refuses(self):
        with self.fixture([deployment('a', ('A', 100))], recheck=[deployment('moved', ('A', 100))]) as (uploads, _, receipt, initial):
            with self.assertRaisesRegex(AssertionError, 'Live deployment changed'): PROMOTE.deploy()
            self.assertEqual(uploads, [])
            self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_uploaded_candidate_split_readback_never_writes_success(self):
        with self.fixture([deployment('a', ('A', 100))], final=[deployment('late-split', ('B', 10), ('A', 90))]) as (uploads, _, receipt, initial):
            with self.assertRaisesRegex(AssertionError, 'single version at 100%'): PROMOTE.deploy()
            self.assertEqual(len(uploads), 1)
            self.assertEqual(json.loads(receipt.read_text())['stage'], 'upload_attempted_recurrence_pending')

    def test_resolved_historical_split_does_not_block_forward_progress(self):
        history = [deployment('b', ('B', 100)), deployment('old-split', ('A', 90), ('B', 10)), deployment('a', ('A', 100))]
        with self.fixture(history, self.c) as (uploads, _, receipt, _):
            PROMOTE.deploy()
            self.assertEqual(len(uploads), 1)
            self.assertEqual(json.loads(receipt.read_text())['source'], self.c)

    def test_pagination_without_metadata_reads_until_empty_not_first_short_page(self):
        history = [deployment('b', ('B', 100)), deployment('a', ('A', 100))]
        with self.fixture(history, page_size=1, verified=True, page_change=lambda page, result, info: (result, None)) as (uploads, requests, receipt, _):
            PROMOTE.deploy()
            self.assertTrue(any('/deployments?page=3&' in path for path in requests))
            self.assertEqual(uploads, [])
            self.assertEqual(json.loads(receipt.read_text())['stage'], 'exact_artifact_promoted')

    def test_incomplete_or_ambiguous_history_refuses_before_upload(self):
        history = [deployment('b', ('B', 100)), deployment('a', ('A', 100))]
        changes = [
            lambda p, r, i: (r, {}),
            lambda p, r, i: (r, dict(i, page=True)),
            lambda p, r, i: (r, dict(i, page=p + 1)),
            lambda p, r, i: (r, dict(i, total_count=i['total_count'] + 1)),
            lambda p, r, i: (r, dict(i, total_pages=i['total_pages'] + (p == 2))),
            lambda p, r, i: (r, None if p == 2 else i),
            lambda p, r, i: ({'deployments': [history[0]]}, i),
            lambda p, r, i: ({'deployments': [history[0]]}, None),
            lambda p, r, i: ({'deployments': []} if p == 1 else r, i),
            lambda p, r, i: ({'deployments': [dict(r['deployments'][0], id='')]}, i),
        ]
        for index, change in enumerate(changes):
            with self.subTest(change=index), self.fixture(history, page_size=1, page_change=change) as (uploads, _, receipt, initial):
                with self.assertRaisesRegex(AssertionError, 'deployment history|Deployment history'): PROMOTE.deploy()
                self.assertEqual(uploads, [])
                self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_missing_history_and_history_read_failure_refuse(self):
        with self.fixture([]) as (uploads, _, receipt, initial):
            with self.assertRaisesRegex(AssertionError, 'No live deployment history'): PROMOTE.deploy()
            self.assertEqual(uploads, [])
            self.assertEqual(json.loads(receipt.read_text()), initial)
        def failure(page, result, info): raise RuntimeError('synthetic history read failed')
        with self.fixture([deployment('a', ('A', 100))], page_change=failure) as (uploads, _, receipt, initial):
            with self.assertRaisesRegex(RuntimeError, 'synthetic history read failed'): PROMOTE.deploy()
            self.assertEqual(uploads, [])
            self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_unbounded_history_is_not_silently_truncated(self):
        def endless(page, result, info): return {'deployments': [deployment(str(page), ('A', 100))]}, None
        with self.fixture([deployment('a', ('A', 100))], page_change=endless) as (uploads, requests, receipt, initial):
            with self.assertRaisesRegex(AssertionError, 'exceeds the bounded read'): PROMOTE.deploy()
            self.assertEqual(sum('/deployments?' in path for path in requests), 100)
            self.assertEqual(uploads, [])
            self.assertEqual(json.loads(receipt.read_text()), initial)


if __name__ == '__main__':
    unittest.main()
