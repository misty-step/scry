#!/usr/bin/env python3
"""Report native deployment failures; versions/tags do not gate shipping."""
import json
import os
import subprocess
import sys


def gh(*args, absent=False) -> bytes:
    result = subprocess.run(['gh', *args], capture_output=True, check=False)
    if absent and result.returncode and b'(HTTP 404)' in result.stderr:
        return b''
    if result.returncode:
        raise RuntimeError(result.stderr.decode())
    return result.stdout



def report_failure():
    repo = os.environ['GITHUB_REPOSITORY']
    title = 'Release automation failed'
    body = 'Deployment failed. Inspect ' + os.environ['RUN_URL'] + '. Inspect the exact-source CI and promotion receipt before retrying; never rebuild or rewind learner data.'
    issues = json.loads(gh('issue', 'list', '--repo', repo, '--state', 'open', '--search', '"' + title + '" in:title', '--json', 'number'))
    if issues:
        gh('issue', 'comment', str(issues[0]['number']), '--repo', repo, '--body', body)
    else:
        gh('issue', 'create', '--repo', repo, '--title', title, '--body', body)


if __name__ == '__main__':
    assert sys.argv[1:] == ['report-failure']
    report_failure()
