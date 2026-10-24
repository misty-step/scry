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


def version(etag=None, message=None, trigger=None):
    annotations = {k: v for k, v in [('workers/message', message), ('workers/triggered_by', trigger)] if v}
    return {'annotations': annotations, 'resources': {'script': {'etag': etag}} if etag else {}}


def drift(name, **change):
    return [dict(b, **change) if b['name'] == name else b for b in LIVE]


class Promotion(unittest.TestCase):
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
                with mock.patch.object(PROMOTE.urllib.request, 'urlopen', urlopen):
                    yield run, requested, receipt

    def test_deploy_refuses_serving_edge_drift(self):
        for domains, routes in [([dict(DOMAIN, hostname='changed.scry.study')], []),
                                ([DOMAIN], [{'pattern': 'scry.study/*'}])]:
            with self.subTest(domains=domains, routes=routes), self.deployment(domains, routes) as (run, requested, receipt):
                with self.assertRaisesRegex(AssertionError, '^Custom domains or routes changed during promotion$'):
                    PROMOTE.deploy()
                run.assert_called_once()
                self.assertEqual(json.loads(receipt.read_text()), {'source': 'a' * 40})

    def test_deploy_observes_serving_edges_before_wrangler(self):
        with self.deployment([DOMAIN], []) as (run, requested, receipt):
            def wrangler(*args, **kwargs):
                self.assertEqual(requested[-2:], ['/workers/domains?service=' + NAME,
                                                 '/workers/scripts/' + NAME + '/routes'])
                self.assertEqual(args[0][1], 'deploy')
                raise RuntimeError('wrangler intercepted')
            run.side_effect = wrangler
            with self.assertRaisesRegex(RuntimeError, '^wrangler intercepted$'):
                PROMOTE.deploy()
            run.assert_called_once()

    def test_deploy_promotes_over_a_secret_only_change_to_a_known_release(self):
        # 2026-10-20: rotating OWNER_SUBJECT made an unannotated version with the release's exact script.
        released = 'b' * 40
        history = [('secret-change', 'secret-only', version(etag='same', trigger='secret')),
                   ('release', 'released', version(etag='same', message='scry-rust ' + released))]
        with self.deployment([DOMAIN], [], env='production', history=history) as (run, requested, receipt):
            run.side_effect = lambda *args, **kwargs: (_ for _ in ()).throw(RuntimeError('stop at ' + args[0][1]))
            with self.assertRaisesRegex(RuntimeError, '^stop at merge-base$'):
                PROMOTE.deploy()
            self.assertEqual(run.call_args.args[0], ['git', 'merge-base', '--is-ancestor', released, 'a' * 40])

    def test_deploy_uploads_unless_the_live_version_itself_names_the_source(self):
        # Re-promoting an annotated release is idempotent; a secret-only version of the same
        # source still lacks the annotation the final check requires, so it uploads.
        source = 'a' * 40
        for label, history, uploads in [
                ('annotated', [('release', 'released', version(etag='same', message='scry-rust ' + source))], False),
                ('secret-only', [('secret-change', 'secret-only', version(etag='same', trigger='secret')),
                                 ('release', 'released', version(etag='same', message='scry-rust ' + source))], True)]:
            with self.subTest(label), self.deployment([DOMAIN], [], env='production', history=history) as (run, requested, receipt):
                def command(args, **kwargs):
                    if args[0] != 'git': raise RuntimeError('upload ' + args[1])
                run.side_effect = command
                if uploads:
                    with self.assertRaisesRegex(RuntimeError, '^upload deploy$'):
                        PROMOTE.deploy()
                else:
                    # Without an upload, promotion continues to its post-checks; this fixture stops there.
                    with self.assertRaisesRegex(AssertionError, 'Unexpected request: .*/subdomain$'):
                        PROMOTE.deploy()
                self.assertEqual(any(c.args[0][0] != 'git' for c in run.call_args_list), uploads)

    def test_deploy_refuses_an_untraceable_live_version(self):
        for history in [[('secret-change', 'secret-only', version(etag='changed', trigger='secret')),
                         ('release', 'released', version(etag='same', message='scry-rust ' + 'b' * 40))],
                        [('secret-change', 'secret-only', version(etag='same', trigger='secret'))],
                        [('manual', 'manual', version(etag='same', trigger='version_upload')),
                         ('release', 'released', version(etag='same', message='scry-rust ' + 'b' * 40))]]:
            with self.subTest(history=history), self.deployment([DOMAIN], [], env='production', history=history) as (run, requested, receipt):
                with self.assertRaisesRegex(AssertionError, '^Unrecognized live release$'):
                    PROMOTE.deploy()
                run.assert_not_called()

    def test_live_release_resolves_only_secret_changes_with_the_same_script(self):
        versions = {'released': version(etag='same', message='scry-rust ' + 'b' * 40),
                    'secret': version(etag='same', trigger='secret'),
                    'secret-again': version(etag='same', trigger='secret'),
                    'other-script': version(etag='other', trigger='secret'),
                    'upload': version(etag='same', trigger='version_upload'),
                    'no-etag': version(trigger='secret'),
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
    def fixture(self, history, source=None, *, recheck=None, final=None, page_size=100, page_change=None, uploaded_only=False):
        source = source or self.b
        current = deepcopy(history)
        versions = {'A': version(etag='old', message='scry-rust ' + self.a),
                    'B': version(etag='new', message='scry-rust ' + self.b),
                    'C': version(etag='candidate', message='scry-rust ' + self.c),
                    'secret-A': version(etag='old', trigger='secret'),
                    'secret-B': version(etag='new', trigger='secret')}
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
                    mock.patch('sys.stdout', new_callable=io.StringIO):
                yield uploads, requests, receipt, initial

    def test_full_forward_and_idempotent_receipts_preserve_secret_only_support(self):
        for name, history, uploads in [
            ('forward', [deployment('a', ('A', 100))], 1),
            ('idempotent', [deployment('b', ('B', 100)), deployment('a', ('A', 100))], 0),
            ('secret-only', [deployment('secret', ('secret-B', 100)), deployment('b', ('B', 100)), deployment('a', ('A', 100))], 1),
        ]:
            with self.subTest(name=name), self.fixture(history, uploaded_only=True) as (called, requests, receipt, _):
                PROMOTE.deploy()
                self.assertEqual(len(called), uploads)
                saved = json.loads(receipt.read_text())
                self.assertEqual(saved['stage'], 'exact_artifact_promoted')
                self.assertEqual(saved['deployment']['versions'][0]['percentage'], 100)
                self.assertFalse(any('not-deployed-C' in p for p in requests))

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
                with self.subTest(entries=entries, phase=phase), self.fixture([broken if phase == 'before' else pure], **options) as (uploads, _, receipt, initial):
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
            self.assertEqual(json.loads(receipt.read_text()), initial)

    def test_resolved_historical_split_does_not_block_forward_progress(self):
        history = [deployment('b', ('B', 100)), deployment('old-split', ('A', 90), ('B', 10)), deployment('a', ('A', 100))]
        with self.fixture(history, self.c) as (uploads, _, receipt, _):
            PROMOTE.deploy()
            self.assertEqual(len(uploads), 1)
            self.assertEqual(json.loads(receipt.read_text())['source'], self.c)

    def test_pagination_without_metadata_reads_until_empty_not_first_short_page(self):
        history = [deployment('b', ('B', 100)), deployment('a', ('A', 100))]
        with self.fixture(history, page_size=1, page_change=lambda page, result, info: (result, None)) as (uploads, requests, receipt, _):
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
