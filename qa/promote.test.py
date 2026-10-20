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
    def deployment(self, domains, routes):
        requested = []
        bindings = drift('SCRY_ENV', text='restore') + [
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
                        result = {'deployments': [{'id': 'previous', 'versions': [{'version_id': 'previous'}]}]}
                    elif path == script + '/versions/previous': result = {}
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
