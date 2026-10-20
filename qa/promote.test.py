#!/usr/bin/env python3
"""Production promotion refuses binding drift before upload and records unchanged serving edges, offline."""
from contextlib import contextmanager
from copy import deepcopy
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import tomllib
import unittest
from unittest import mock

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
                    elif path == script + '/deployments':
                        result = {'deployments': [{'id': d, 'versions': [{'version_id': v}]} for d, v, _ in history]}
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
        live = lambda *ids: PROMOTE.live_release([{'versions': [{'version_id': i}]} for i in ids], versions.__getitem__)
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


if __name__ == '__main__':
    unittest.main()
