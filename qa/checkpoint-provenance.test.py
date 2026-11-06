#!/usr/bin/env python3
"""Negative controls for the standalone release-evidence verifiers."""
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def git(root, *args):
    return subprocess.check_output(['git', '-C', str(root), *args]).decode().strip()


def sha(data):
    return hashlib.sha256(data).hexdigest()


def encoded(obj):
    return (json.dumps(obj, sort_keys=True, indent=2) + '\n').encode()


class Provenance(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / 'source'
        self.root.mkdir()
        git(self.root, 'init', '-q')
        (self.root / 'scripts').mkdir()
        (self.root / 'scripts/scry-ci').write_text('def excluded(path): return False\n')
        (self.root / 'one').write_text('one\n')
        (self.root / 'two').write_text('two\n')
        git(self.root, 'add', '.')
        git(self.root, '-c', 'user.name=Test', '-c', 'user.email=test@example.test', 'commit', '-qm', 'fixture')
        self.head = git(self.root, 'rev-parse', 'HEAD')
        self.outside = Path(self.temp.name)

    def call(self, script, *args):
        return subprocess.run(['python3', str(ROOT / 'scripts' / script), *map(str, args)], cwd=self.root, capture_output=True)

    def test_manifest_rejects_partial_forged_and_dirty(self):
        result = self.call('checkpoint-manifest', 'create')
        self.assertEqual(result.returncode, 0, result.stderr)
        manifest = json.loads(result.stdout)
        path = self.outside / 'manifest.json'
        path.write_text(json.dumps(manifest))
        self.assertEqual(self.call('checkpoint-manifest', 'check', path, self.head).returncode, 0)
        manifest['files'] = dict(list(manifest['files'].items())[:1])
        path.write_text(json.dumps(manifest))
        self.assertNotEqual(self.call('checkpoint-manifest', 'check', path, self.head).returncode, 0)
        self.assertNotEqual(self.call('checkpoint-manifest', 'check', path, '0' * 40).returncode, 0)
        (self.root / 'untracked').write_text('unexpected')
        self.assertNotEqual(self.call('checkpoint-manifest', 'create').returncode, 0)

    def artifact(self, alter_source=False, alter_worker=False):
        files = []
        source = io.BytesIO()
        with tarfile.open(fileobj=source, mode='w:gz') as tar:
            for path in ('one', 'scripts/scry-ci', 'two'):
                data = (self.root / path).read_bytes()
                archived = b'tampered\n' if alter_source and path == 'one' else data
                item = {'path': path, 'mode': '100644', 'git_blob': git(self.root, 'hash-object', path), 'sha256': sha(archived)}
                files.append(item)
                info = tarfile.TarInfo(path)
                info.size = len(archived)
                info.mode = 0o644
                tar.addfile(info, io.BytesIO(archived))
        metadata = {'format': 'scry-release-source-v1', 'source_state': 'committed', 'revision': self.head, 'git_revision': self.head, 'git_tree': git(self.root, 'rev-parse', 'HEAD^{tree}'), 'files': files, 'source_sha256': sha(encoded(files))}
        workers = {'index.js': b'js', 'index_bg.wasm': b'wasm', 'worker/shim.mjs': b'shim'}
        proof = {'format': 'scry-rust-worker-release-proof-v1', 'source_state': 'committed', 'revision': self.head, 'source_sha256': metadata['source_sha256'], 'worker': {k: sha(v) for k, v in workers.items()}}
        archive = self.outside / 'artifact.zip'
        with zipfile.ZipFile(archive, 'w') as z:
            z.writestr('source.json', encoded(metadata))
            z.writestr('proof.json', encoded(proof))
            z.writestr('source.tar.gz', source.getvalue())
            for name, data in workers.items():
                z.writestr('worker/' + name, b'tampered' if alter_worker and name == 'index.js' else data)
        return archive, sha(archive.read_bytes())

    def test_artifact_binds_complete_source_and_worker(self):
        archive, digest = self.artifact()
        dest = self.outside / 'worker'
        result = self.call('checkpoint-ci-artifact', archive, digest, self.head, '123', dest)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((dest / 'index.js').read_bytes(), b'js')
        self.assertNotEqual(self.call('checkpoint-ci-artifact', archive, '0' * 64, self.head, '123', self.outside / 'bad').returncode, 0)
        self.assertNotEqual(self.call('checkpoint-ci-artifact', archive, digest, '0' * 40, '123', self.outside / 'bad').returncode, 0)
        for kwargs in ({'alter_source': True}, {'alter_worker': True}):
            archive, digest = self.artifact(**kwargs)
            self.assertNotEqual(self.call('checkpoint-ci-artifact', archive, digest, self.head, '123', self.outside / 'bad').returncode, 0)


if __name__ == '__main__':
    unittest.main()
