#!/usr/bin/env python3
"""Production promotion refuses binding drift before upload and records unchanged serving edges, offline."""
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
