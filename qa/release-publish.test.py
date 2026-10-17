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
    def test_exact_ci_allows_success_and_refuses_failed_skipped_wrong_source_or_event(self):
        source = 'a' * 40
        good = dict(head_sha=source, head_branch='master', event='push', conclusion='success', path='.github/workflows/ci.yml')
        PUBLISH['gate'](good, source)
        for change in [dict(conclusion='failure'), dict(conclusion='skipped'), dict(conclusion=None),
                       dict(head_sha='b' * 40), dict(event='pull_request'), dict(head_branch='other'), dict(path='other.yml')]:
            with self.subTest(change=change), self.assertRaisesRegex(RuntimeError, 'successful exact-source'):
                PUBLISH['gate'](good | change, source)

    def test_real_preflight_refuses_missing_baseline_and_moved_head(self):
        block = WORKFLOW.split('      - name: Refuse a moved head or missing bootstrap\n', 1)[1].split('      - uses:', 1)[0]
        shell = '\n'.join(line[10:] for line in block.split('        run: |\n', 1)[1].splitlines())
        with tempfile.TemporaryDirectory() as directory:
            subprocess.run(['git', 'init', '--quiet', directory], check=True)
            subprocess.run(['git', '-C', directory, '-c', 'user.name=Synthetic release proof', '-c', 'user.email=release@example.invalid', 'commit', '--quiet', '--allow-empty', '-m', 'chore: isolated baseline'], check=True)
            head = subprocess.check_output(['git', '-C', directory, 'rev-parse', 'HEAD'], text=True).strip()
            config = json.loads((ROOT / 'release-please-config.json').read_text())
            config['bootstrap-sha'] = head
            path = Path(directory) / 'release-please-config.json'
            path.write_text(json.dumps(config))
            def run(expected):
                return subprocess.run(['bash', '-e', '-c', shell], cwd=directory, env={'PATH': os.environ['PATH'], 'EXPECTED_HEAD': expected}, capture_output=True)
            self.assertEqual(run(head).returncode, 0)
            self.assertNotEqual(run('f' * 40).returncode, 0)
            config['bootstrap-sha'] = 'f' * 40
            path.write_text(json.dumps(config))
            self.assertNotEqual(run(head).returncode, 0)

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
