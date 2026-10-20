#!/usr/bin/env python3
"""Exercise executable release refusals without any cloud or GitHub capability."""
import json
import os
from pathlib import Path
import runpy
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = (ROOT / '.github/workflows/release.yml').read_text()
PUBLISH = runpy.run_path(str(ROOT / 'scripts/scry-publish.py'))


class ReleaseRefusals(unittest.TestCase):
    def test_exact_source_preflight_runs_before_the_generator_can_publish(self):
        self.assertLess(WORKFLOW.index('run: python3 scripts/scry-publish.py preflight'), WORKFLOW.index('uses: googleapis/release-please-action@'))

    def test_head_skip_gates_all_release_effects_and_preserves_failure_reporting(self):
        blocks = WORKFLOW.split('\n      - ')
        head = next(i for i, block in enumerate(blocks) if '\n          EXPECTED_HEAD:' in block)
        self.assertIn('\n        id: head\n', blocks[head])
        self.assertIn('\n          GH_TOKEN: ${{ github.token }}\n', blocks[head])
        for block in blocks[head + 1:-1]:
            with self.subTest(step=block.splitlines()[0]):
                self.assertIn("\n        if: steps.head.outputs.skip == 'false'", block)
        self.assertIn('\n        if: failure()\n', blocks[-1])

    def test_exact_ci_allows_success_and_refuses_failed_skipped_wrong_source_or_event(self):
        source = 'a' * 40
        good = dict(head_sha=source, head_branch='master', event='push', conclusion='success', path='.github/workflows/ci.yml')
        PUBLISH['gate'](good, source)
        for change in [dict(conclusion='failure'), dict(conclusion='skipped'), dict(conclusion=None),
                       dict(head_sha='b' * 40), dict(event='pull_request'), dict(head_branch='other'), dict(path='other.yml')]:
            with self.subTest(change=change), self.assertRaisesRegex(RuntimeError, 'successful exact-source'):
                PUBLISH['gate'](good | change, source)

    def test_real_head_check_uses_current_ci_before_skipping_and_refuses_invalid_history(self):
        block = next(block for block in WORKFLOW.split('\n      - ') if '\n          EXPECTED_HEAD:' in block)
        shell = '\n'.join(line[10:] for line in block.split('        run: |\n', 1)[1].splitlines())
        with tempfile.TemporaryDirectory() as directory:
            subprocess.run(['git', 'init', '--quiet', directory], check=True)
            subprocess.run(['git', '-C', directory, '-c', 'user.name=Synthetic release proof', '-c', 'user.email=release@example.invalid', 'commit', '--quiet', '--allow-empty', '-m', 'chore: isolated baseline'], check=True)
            baseline = subprocess.check_output(['git', '-C', directory, 'rev-parse', 'HEAD'], text=True).strip()
            subprocess.run(['git', '-C', directory, '-c', 'user.name=Synthetic release proof', '-c', 'user.email=release@example.invalid', 'commit', '--quiet', '--allow-empty', '-m', 'chore: newer master'], check=True)
            head = subprocess.check_output(['git', '-C', directory, 'rev-parse', 'HEAD'], text=True).strip()
            tree = subprocess.check_output(['git', '-C', directory, 'rev-parse', 'HEAD^{tree}'], text=True).strip()
            divergent = subprocess.check_output(['git', '-C', directory, '-c', 'user.name=Synthetic release proof', '-c', 'user.email=release@example.invalid', 'commit-tree', tree, '-p', baseline, '-m', 'chore: divergent head'], text=True).strip()
            scripts = Path(directory) / 'scripts'
            scripts.mkdir()
            (scripts / 'scry-publish.py').write_bytes((ROOT / 'scripts/scry-publish.py').read_bytes())
            config = json.loads((ROOT / 'release-please-config.json').read_text())
            config['bootstrap-sha'] = baseline
            path = Path(directory) / 'release-please-config.json'
            path.write_text(json.dumps(config))
            output = Path(directory) / 'output'
            calls = Path(directory) / 'gh-calls'
            query = f'repos/synthetic/scry/actions/workflows/ci.yml/runs?head_sha={head}&branch=master&event=push&status=success'
            bin_path = Path(directory) / 'bin'
            bin_path.mkdir()
            stub = bin_path / 'gh'
            stub.write_text('''#!/usr/bin/env python3
import os, sys
from pathlib import Path
assert sys.argv[1:] == ['api', os.environ['GH_QUERY']], sys.argv
Path(os.environ['GH_CALLS']).write_text('called')
if os.environ['GH_API_ERROR'] == 'true':
    sys.exit('synthetic GitHub API failure')
print(os.environ['GH_RESPONSE'])
''')
            stub.chmod(0o755)
            good = dict(head_sha=head, head_branch='master', event='push', conclusion='success', path='.github/workflows/ci.yml')
            def run(expected, ci_runs=(), api_error=False):
                output.write_text('')
                calls.write_text('')
                env = {'PATH': str(bin_path) + os.pathsep + os.environ['PATH'], 'EXPECTED_HEAD': expected,
                       'GITHUB_OUTPUT': str(output), 'GITHUB_REPOSITORY': 'synthetic/scry', 'GH_TOKEN': 'synthetic-only',
                       'GH_QUERY': query, 'GH_CALLS': str(calls), 'GH_API_ERROR': str(api_error).lower(),
                       'GH_RESPONSE': json.dumps({'workflow_runs': ci_runs})}
                return subprocess.run(['bash', '-e', '-c', shell], cwd=directory, env=env, capture_output=True, text=True)
            allowed = run(baseline, ci_runs=[good])
            self.assertEqual(allowed.returncode, 0, allowed.stderr)
            self.assertEqual(output.read_text(), 'skip=false\n')
            self.assertNotIn('::notice::Skipping release', allowed.stdout)
            self.assertEqual(calls.read_text(), 'called')
            skipped = run(baseline)
            self.assertEqual(skipped.returncode, 0, skipped.stderr)
            self.assertIn('::notice::Skipping release', skipped.stdout)
            self.assertIn(baseline, skipped.stdout)
            self.assertIn(head, skipped.stdout)
            self.assertIn('if it succeeds', skipped.stdout)
            self.assertEqual(output.read_text(), 'skip=true\n')
            self.assertEqual(calls.read_text(), 'called')
            refused = run(baseline, api_error=True)
            self.assertNotEqual(refused.returncode, 0)
            self.assertIn('synthetic GitHub API failure', refused.stderr)
            self.assertEqual(output.read_text(), '')
            allowed = run(head)
            self.assertEqual(allowed.returncode, 0, allowed.stderr)
            self.assertEqual(output.read_text(), 'skip=false\n')
            self.assertEqual(calls.read_text(), '')
            for expected in [divergent, 'f' * 40]:
                with self.subTest(expected=expected):
                    refused = run(expected)
                    self.assertNotEqual(refused.returncode, 0)
                    self.assertTrue(refused.stderr)
                    self.assertEqual(output.read_text(), '')
                    self.assertEqual(calls.read_text(), '')
            for bootstrap in [divergent, 'f' * 40, None]:
                if bootstrap is None:
                    del config['bootstrap-sha']
                else:
                    config['bootstrap-sha'] = bootstrap
                path.write_text(json.dumps(config))
                for expected in [head, baseline]:
                    with self.subTest(bootstrap=bootstrap, expected=expected):
                        refused = run(expected)
                        self.assertNotEqual(refused.returncode, 0)
                        self.assertTrue(refused.stderr)
                        self.assertEqual(output.read_text(), '')
                        self.assertEqual(calls.read_text(), '')

    def test_title_is_data_not_shell_code(self):
        shell = next(line.strip() for line in WORKFLOW.splitlines() if line.strip().startswith("python3 -c 'import os,re;"))
        for title, allowed in [('feat: Read useful release history', True), ('fix(auth)!: Require a new sign in', True),
                               ('chore: release master', True), ('fix: $(exit 7)', True), ('not conventional', False),
                               ('fix: first\nfeat: second', False), ('fix: ', False)]:
            with self.subTest(title=title):
                result = subprocess.run(['bash', '-e', '-c', shell], env={'PATH': os.environ['PATH'], 'PR_TITLE': title}, capture_output=True)
                self.assertEqual(result.returncode == 0, allowed)


if __name__ == '__main__':
    unittest.main()
