#!/usr/bin/env python3
"""Small offline contract tests for direct deployment. No provider calls."""
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('deploy', ROOT / 'scripts/scry-deploy.py')
D = importlib.util.module_from_spec(spec)
spec.loader.exec_module(D)
SOURCE = 'a' * 40


class DeployTests(unittest.TestCase):
    def fixture(self, *, changed=None, corrupt=False, revision=SOURCE, config_change=None):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        root = Path(self.temporary.name)
        out = root / 'target/ci-release'
        (out / 'worker').mkdir(parents=True)
        files = {'index.js': b'export default {}', 'index_bg.wasm': b'wasm'}
        for name, data in files.items():
            (out / 'worker' / name).write_bytes(data)
        proof = {'revision': revision, 'source_state': 'committed',
                 'worker': {name: hashlib.sha256(data).hexdigest() for name, data in files.items()}}
        (out / 'proof.json').write_text(json.dumps(proof))
        if corrupt:
            (out / 'worker/index.js').write_bytes(b'changed')
        config = root / 'production.toml'
        config.write_text('''name="scry-existing"
account_id="synthetic"
[vars]
SPACE_NAME="existing-space"
[[durable_objects.bindings]]
name="LEARNING_SPACE"
class_name="LearningSpace"
script_name="scry-existing"
[[r2_buckets]]
binding="SCRY_ASSETS"
bucket_name="existing-assets"
[[r2_buckets]]
binding="SCRY_BACKUPS"
bucket_name="existing-backups"
''')
        before = {'bindings': [
            {'name': 'LEARNING_SPACE', 'type': 'durable_object_namespace', 'class_name': 'LearningSpace', 'namespace_id': 'original-id'},
            {'name': 'SCRY_ASSETS', 'type': 'r2_bucket', 'bucket_name': 'existing-assets'},
            {'name': 'SCRY_BACKUPS', 'type': 'r2_bucket', 'bucket_name': 'existing-backups'},
            {'name': 'SPACE_NAME', 'type': 'plain_text', 'text': 'existing-space'},
            {'name': 'INTERNAL_KEY', 'type': 'secret_text'}]}
        if config_change:
            config_change(before)
        deployed = False
        self.uploads = []
        self.requests = []
        def run(command, **kwargs):
            nonlocal deployed
            self.uploads.append(command)
            self.assertIn('--no-bundle', command)
            self.assertIn('scry-rust ' + SOURCE, command)
            deployed = True
        def request(path, raw=False):
            self.requests.append(path)
            if path.endswith('/settings'):
                result = copy.deepcopy(before)
                if deployed and changed == 'storage':
                    result['bindings'][0]['namespace_id'] = 'different-id'
                return result
            if path.endswith('/routes'):
                return [{'pattern': 'changed'}] if deployed and changed == 'routes' else []
            if path.startswith('/workers/domains'):
                return []
            if path.endswith('/subdomain'):
                return {'enabled': changed == 'public', 'previews_enabled': False}
            if path.endswith('/content/v2'):
                modules = dict(files)
                if changed == 'modules':
                    modules['index.js'] = b'wrong'
                body = b''
                for name, data in modules.items():
                    body += b'--boundary\r\nContent-Disposition: form-data; name="'+name.encode()+b'"\r\nContent-Type: application/octet-stream\r\n\r\n'+data+b'\r\n'
                return body + b'--boundary--\r\n', 'multipart/form-data; boundary=boundary'
            self.fail('Unexpected API request: ' + path)
        with patch.multiple(D, ROOT=root, OUT=out, CONFIG=config), patch.dict(os.environ, GITHUB_SHA=SOURCE):
            D.deploy(request, run)
        return json.loads((root / 'target/deployment.json').read_text())

    def test_deploys_exact_modules_without_data_operations(self):
        receipt = self.fixture()
        self.assertEqual(receipt['source'], SOURCE)
        self.assertFalse(receipt['learner_data_requested'])
        self.assertEqual(receipt['live_acceptance'], 'not_tested')
        self.assertEqual(len(self.uploads), 1)
        self.assertTrue(all(path.startswith('/workers/') for path in self.requests))

    def test_refuses_changed_artifact_before_upload(self):
        with self.assertRaisesRegex(RuntimeError, 'bytes changed'):
            self.fixture(corrupt=True)
        self.assertFalse(self.uploads)

    def test_refuses_different_source_before_upload(self):
        with self.assertRaisesRegex(RuntimeError, 'does not match'):
            self.fixture(revision='b' * 40)
        self.assertFalse(self.uploads)

    def test_refuses_different_storage_configuration_before_upload(self):
        with self.assertRaisesRegex(RuntimeError, 'bucket differs'):
            self.fixture(config_change=lambda b: b['bindings'][1].update(bucket_name='unexpected'))
        self.assertFalse(self.uploads)

    def test_refuses_different_learning_space_before_upload(self):
        with self.assertRaisesRegex(RuntimeError, 'learning space differs'):
            self.fixture(config_change=lambda b: b['bindings'][3].update(text='other-space'))
        self.assertFalse(self.uploads)

    def test_reports_wrong_live_modules(self):
        with self.assertRaisesRegex(RuntimeError, 'Live Worker bytes'):
            self.fixture(changed='modules')

    def test_reports_changed_storage(self):
        with self.assertRaisesRegex(RuntimeError, 'storage identity'):
            self.fixture(changed='storage')

    def test_reports_changed_routes(self):
        with self.assertRaisesRegex(RuntimeError, 'routes changed'):
            self.fixture(changed='routes')

    def test_reports_public_alternate_host(self):
        with self.assertRaisesRegex(RuntimeError, 'public Worker'):
            self.fixture(changed='public')

    def test_only_one_workflow_and_no_recovery_dispatch(self):
        workflows = list((ROOT / '.github/workflows').glob('*.yml'))
        self.assertEqual([p.name for p in workflows], ['ci.yml'])
        text = workflows[0].read_text()
        self.assertEqual(text.count('bun run ci:full'), 1)
        self.assertIn('python3 scripts/scry-deploy.py', text)
        self.assertNotIn('recovery_approval', text)
        self.assertNotIn('schedule:', text)
        self.assertNotIn('scry-promote', text)


if __name__ == '__main__':
    unittest.main()
