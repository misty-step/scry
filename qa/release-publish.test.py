#!/usr/bin/env python3
"""Exercise native exact-artifact shipping admission without GitHub/cloud access."""
from contextlib import contextmanager
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock
import zipfile

ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = (ROOT / '.github/workflows/release.yml').read_text()
CI = (ROOT / '.github/workflows/ci.yml').read_text()
spec = importlib.util.spec_from_file_location('release_promote', ROOT / 'scripts/scry-promote.py')
PROMOTE = importlib.util.module_from_spec(spec)
spec.loader.exec_module(PROMOTE)
SOURCE = 'a' * 40
SOURCE_FILES = (
    'deploy/rust-production.toml', 'scripts/scry-promote.py',
    'scripts/scry-cloud-backup.py', 'scripts/scry-cloud-recovery.py',
    'recovery/cloud-restore.mjs', 'recovery/target-adapter.mjs',
)
sha = lambda value: hashlib.sha256(value).hexdigest()


class ShippingAdmission(unittest.TestCase):
    @contextmanager
    def artifact(self, *, run_change=None, metadata_change=None, jobs=None,
                 proof_change=None, zip_extra=None, corrupt_member=False,
                 env_change=None, drift=None):
        """Tiny synthetic artifact; every effect other than temporary files is stubbed."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source_files = {name: ('synthetic committed ' + name + '\n').encode() for name in SOURCE_FILES}
            for name, content in source_files.items():
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(content + (b'local drift' if name == drift else b''))
            proof = {'source_state': 'committed', 'revision': SOURCE} | (proof_change or {})
            files = source_files | {'proof.json': json.dumps(proof).encode(),
                                    'worker/index.js': b'export default {};'}
            sums = ''.join(sha(content) + '  ' + name + '\n' for name, content in files.items())
            files['SHA256SUMS'] = sums.encode()
            if corrupt_member:
                files['worker/index.js'] = b'changed after checksums'
            files.update(zip_extra or {})
            buffer = io.BytesIO()
            with zipfile.ZipFile(buffer, 'w') as archive:
                for name, content in files.items():
                    archive.writestr(name, content)
            raw = buffer.getvalue()
            digest = sha(raw)
            run = {'id': 123, 'head_sha': SOURCE, 'head_branch': 'master',
                   'event': 'push', 'path': '.github/workflows/ci.yml',
                   'status': 'completed', 'conclusion': 'success'} | (run_change or {})
            metadata = {'id': 456, 'name': 'scry-worker-' + SOURCE, 'expired': False,
                        'workflow_run': {'id': 123}, 'digest': 'sha256:' + digest} | (metadata_change or {})
            jobs = [{'name': 'ci', 'status': 'completed', 'conclusion': 'success'}] if jobs is None else jobs
            calls = []
            def gh(path):
                calls.append(path)
                if path == 'actions/artifacts/456':
                    return json.dumps(metadata).encode()
                if path == 'actions/runs/123':
                    return json.dumps(run).encode()
                if path == 'actions/runs/123/jobs':
                    return json.dumps({'jobs': jobs}).encode()
                if path == 'actions/artifacts/456/zip':
                    return raw
                self.fail('Unexpected GitHub operation: ' + path)
            def git(args, **kwargs):
                self.assertEqual(kwargs, {'cwd': root})
                self.assertEqual(args[:2], ['git', 'show'])
                revision, name = args[2].split(':', 1)
                self.assertEqual(revision, SOURCE)
                self.assertIn(name, source_files)
                return source_files[name]
            out = root / 'target/ci-release'
            receipt = root / 'target/promotion.json'
            env = {'ARTIFACT_ID': '456', 'ARTIFACT_SHA256': digest,
                   'GITHUB_SHA': SOURCE, 'GITHUB_RUN_ID': '999',
                   'GITHUB_REPOSITORY': 'synthetic/scry'} | (env_change or {})
            with mock.patch.object(PROMOTE, 'ROOT', root), mock.patch.object(PROMOTE, 'OUT', out), \
                    mock.patch.object(PROMOTE, 'RECEIPT', receipt), mock.patch.object(PROMOTE, 'gh', side_effect=gh), \
                    mock.patch.object(PROMOTE.subprocess, 'check_output', side_effect=git), \
                    mock.patch.object(PROMOTE.subprocess, 'run', side_effect=AssertionError('No deployment/build command is allowed')), \
                    mock.patch.object(PROMOTE.urllib.request, 'urlopen', side_effect=AssertionError('No network is allowed')), \
                    mock.patch.dict(os.environ, env, clear=True):
                yield calls, out, receipt, digest

    def test_successful_completed_master_artifact_is_verified_without_rebuild(self):
        with self.artifact() as (calls, out, receipt, digest):
            PROMOTE.prepare()
            self.assertEqual(json.loads(receipt.read_text()), {
                'artifact_id': 456, 'artifact_zip_sha256': digest,
                'source': SOURCE, 'stage': 'artifact_verified'})
            self.assertEqual((out / 'worker/index.js').read_bytes(), b'export default {};')
            self.assertIn('actions/runs/123/jobs', calls)

    def test_same_running_workflow_accepts_only_its_completed_ci_job(self):
        with self.artifact(run_change={'status': 'in_progress', 'conclusion': None},
                           env_change={'GITHUB_RUN_ID': '123'}) as (calls, _, receipt, _):
            PROMOTE.prepare()
            self.assertIn('actions/runs/123/jobs', calls)
            self.assertEqual(json.loads(receipt.read_text())['source'], SOURCE)

    def test_failed_or_cancelled_deployment_does_not_erase_successful_build_proof(self):
        # Build and deploy now share one run. A failed later deployment must not
        # force a rebuild or prevent an explicitly authorized maintenance retry.
        for conclusion in ('failure', 'cancelled', 'timed_out'):
            with self.subTest(conclusion=conclusion), self.artifact(
                    run_change={'status': 'completed', 'conclusion': conclusion}) as (calls, _, receipt, _):
                PROMOTE.prepare()
                self.assertIn('actions/runs/123/jobs', calls)
                self.assertEqual(json.loads(receipt.read_text())['source'], SOURCE)

    def test_every_artifact_requires_one_completed_successful_ci_job(self):
        variants = [[], [{'name': 'other', 'status': 'completed', 'conclusion': 'success'}],
                    [{'name': 'ci', 'status': 'completed', 'conclusion': 'success'}] * 2]
        variants += [[{'name': 'ci', 'status': status, 'conclusion': conclusion}]
                     for status, conclusion in [('in_progress', None), ('queued', None),
                                                ('completed', 'failure'), ('completed', 'cancelled'),
                                                ('in_progress', 'success')]]
        for jobs in variants:
            for run_change in ({}, {'status': 'in_progress', 'conclusion': None}, {'conclusion': 'failure'}):
                with self.subTest(jobs=jobs, run=run_change), self.artifact(
                        run_change=run_change, env_change={'GITHUB_RUN_ID': '123'}, jobs=jobs) as (calls, _, receipt, _):
                    with self.assertRaisesRegex(AssertionError, 'CI job did not succeed'):
                        PROMOTE.prepare()
                    self.assertNotIn('actions/artifacts/456/zip', calls)
                    self.assertFalse(receipt.exists())

    def test_wrong_source_branch_event_or_workflow_refuses_before_download(self):
        variants = [{'head_sha': 'b' * 40}, {'head_branch': 'feature'},
                    {'event': 'pull_request'}, {'event': 'workflow_dispatch'}, {'path': '.github/workflows/other.yml'}]
        for change in variants:
            with self.subTest(change=change), self.artifact(run_change=change) as (calls, _, receipt, _):
                with self.assertRaisesRegex(AssertionError, 'master push CI|checked-out deployment source'):
                    PROMOTE.prepare()
                self.assertNotIn('actions/artifacts/456/zip', calls)
                self.assertFalse(receipt.exists())

    def test_expired_or_invalid_identifier_refuses_before_download(self):
        with self.artifact(metadata_change={'expired': True}) as (calls, _, receipt, _):
            with self.assertRaisesRegex(AssertionError, 'Artifact expired'):
                PROMOTE.prepare()
            self.assertNotIn('actions/artifacts/456/zip', calls)
            self.assertFalse(receipt.exists())
        for change in [{'ARTIFACT_ID': '../456'}, {'ARTIFACT_ID': ''},
                       {'ARTIFACT_SHA256': 'A' * 64}, {'ARTIFACT_SHA256': 'a' * 63}]:
            with self.subTest(change=change), self.artifact(env_change=change) as (calls, _, receipt, _):
                with self.assertRaisesRegex(AssertionError, 'Invalid artifact ID or SHA256'):
                    PROMOTE.prepare()
                self.assertEqual(calls, [])
                self.assertFalse(receipt.exists())

    def test_both_transport_and_github_digest_must_match(self):
        for change in [{'env_change': {'ARTIFACT_SHA256': 'b' * 64}},
                       {'metadata_change': {'digest': 'sha256:' + 'b' * 64}}]:
            with self.subTest(change=change), self.artifact(**change) as (_, out, receipt, _):
                with self.assertRaisesRegex(AssertionError, 'archive digest mismatch'):
                    PROMOTE.prepare()
                self.assertFalse(out.exists())
                self.assertFalse(receipt.exists())

    def test_zip_traversal_and_member_checksum_mismatch_cannot_create_receipt(self):
        for name in ('../outside', '/absolute'):
            with self.subTest(name=name), self.artifact(zip_extra={name: b'untrusted'}) as (_, out, receipt, _):
                with self.assertRaisesRegex(AssertionError, 'unsafe path'):
                    PROMOTE.prepare()
                self.assertFalse((out.parent / 'outside').exists())
                self.assertFalse(receipt.exists())
        with self.artifact(corrupt_member=True) as (_, _, receipt, _):
            with self.assertRaisesRegex(AssertionError, 'file checksum mismatch'):
                PROMOTE.prepare()
            self.assertFalse(receipt.exists())

    def test_archive_proof_must_name_exact_committed_source(self):
        for change in ({'source_state': 'worktree'}, {'revision': 'b' * 40}):
            with self.subTest(change=change), self.artifact(proof_change=change) as (_, _, receipt, _):
                with self.assertRaisesRegex(AssertionError, 'proof must match committed CI source'):
                    PROMOTE.prepare()
                self.assertFalse(receipt.exists())

    def test_local_deployment_source_drift_is_not_authorized_by_a_valid_artifact(self):
        for name in SOURCE_FILES:
            with self.subTest(name=name), self.artifact(drift=name) as (_, _, receipt, _):
                with self.assertRaisesRegex(AssertionError, 'configuration differs from tested source'):
                    PROMOTE.prepare()
                self.assertFalse(receipt.exists())

    def test_workflow_has_one_build_and_no_metadata_release_or_duplicate_walk(self):
        commands = '\n'.join(line for line in CI.splitlines() if not line.lstrip().startswith('#'))
        self.assertEqual(commands.count('bun run ci:full'), 1)
        self.assertNotIn('qa/walk --all', commands)
        self.assertNotIn('release-please', CI + WORKFLOW)
        self.assertNotIn('workflow_run:', WORKFLOW)
        self.assertIn('  story-walk:\n    needs: ci\n', CI)
        self.assertIn('  promote:\n    needs: ci\n', CI)
        self.assertIn("needs.ci.result == 'success'", CI)
        self.assertNotIn('deploy/cloud-recovery-approval.json', CI)
        self.assertFalse((ROOT / 'deploy/cloud-recovery-approval.json').exists())

    def test_story_walk_alias_executes_a_failure_for_every_unsuccessful_ci_result(self):
        alias = CI.split('  story-walk:', 1)[1].split('  promote:', 1)[0]
        self.assertIn("if: always() && github.event_name != 'workflow_dispatch'", alias)
        shell = next(line.split('run:', 1)[1].strip() for line in alias.splitlines() if 'run:' in line)
        for status in ('success', 'failure', 'cancelled', 'skipped', '', 'in_progress'):
            with self.subTest(status=status):
                result = subprocess.run(['bash', '-e', '-c', shell],
                    env={'PATH': os.environ['PATH'], 'CI_RESULT': status}, capture_output=True)
                self.assertEqual(result.returncode == 0, status == 'success')


    def test_manual_source_intent_cannot_fallback_to_github_sha(self):
        block = CI.split('      - name: Refuse a moved source or manual dispatch without exact intent', 1)[1].split('      - run:', 1)[0]
        self.assertIn('EXPECTED_HEAD: ${{ inputs.expected_head }}', block)
        shell = '\n'.join(line[10:] for line in block.split('        run: |\n', 1)[1].splitlines())
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            calls = root / 'calls'
            stub = root / 'git'
            stub.write_text("#!/usr/bin/env python3\nimport os,sys\nfrom pathlib import Path\nwith Path(os.environ['GIT_CALLS']).open('a') as output: print(' '.join(sys.argv[1:]),file=output)\nif sys.argv[1:]==['rev-parse','HEAD']: print(os.environ['CHECKOUT_SHA'])\nelif sys.argv[1:]==['ls-remote','origin','refs/heads/master']: print(os.environ['REMOTE_SHA']+'\\trefs/heads/master')\nelse: sys.exit(99)\n")
            stub.chmod(0o755)
            def run(expected, event='workflow_dispatch', checkout=SOURCE, remote=SOURCE):
                calls.write_text('')
                env = {'PATH': str(root) + os.pathsep + os.environ['PATH'], 'EXPECTED_HEAD': expected,
                       'EVENT_NAME': event, 'GITHUB_SHA': SOURCE, 'CHECKOUT_SHA': checkout,
                       'REMOTE_SHA': remote, 'GIT_CALLS': str(calls)}
                return subprocess.run(['bash', '-e', '-c', shell], env=env, capture_output=True)
            for invalid in ('', 'a' * 39, 'A' * 40, 'master', '$(exit 0)', SOURCE + '\n'):
                with self.subTest(invalid=invalid):
                    self.assertNotEqual(run(invalid).returncode, 0)
                    self.assertEqual(calls.read_text(), '')
            self.assertEqual(run(SOURCE).returncode, 0)
            self.assertEqual(run('', event='push').returncode, 0)
            self.assertNotEqual(run(SOURCE, checkout='b' * 40).returncode, 0)
            self.assertNotEqual(run(SOURCE, remote='b' * 40).returncode, 0)


    def test_title_is_data_not_shell_code(self):
        shell = next(line.strip() for line in WORKFLOW.splitlines()
                     if line.strip().startswith("python3 -c 'import os,re;"))
        for title, allowed in [('feat: Read useful release history', True), ('fix(auth)!: Require a new sign in', True),
                               ('chore: release master', True), ('fix: $(exit 7)', True), ('not conventional', False),
                               ('fix: first\nfeat: second', False), ('fix: ', False)]:
            with self.subTest(title=title):
                result = subprocess.run(['bash', '-e', '-c', shell],
                                        env={'PATH': os.environ['PATH'], 'PR_TITLE': title}, capture_output=True)
                self.assertEqual(result.returncode == 0, allowed)


if __name__ == '__main__':
    unittest.main()
