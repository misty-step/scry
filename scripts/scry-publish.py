#!/usr/bin/env python3
"""Attach existing tested bytes and dispatch the existing promoter; never build or version."""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
from urllib.parse import urlencode


def gh(*args, absent=False) -> bytes:
    result = subprocess.run(['gh', *args], capture_output=True, check=False)
    if absent and result.returncode and b'(HTTP 404)' in result.stderr:
        return b''
    if result.returncode:
        raise RuntimeError(result.stderr.decode())
    return result.stdout


def gate(run, source):
    if not (run['head_sha'] == source and run['head_branch'] == 'master'
            and run['event'] == 'push' and run['conclusion'] == 'success'
            and run['path'] == '.github/workflows/ci.yml'):
        raise RuntimeError('Release requires successful exact-source master push CI')


def check_head():
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    bootstrap = json.loads(Path('release-please-config.json').read_text())['bootstrap-sha']
    subprocess.run(['git', 'merge-base', '--is-ancestor', bootstrap, head], check=True)
    expected = os.environ['EXPECTED_HEAD']
    skip = head != expected
    if skip:
        subprocess.run(['git', 'merge-base', '--is-ancestor', expected, head], check=True)
        print(f'::notice::Skipping release: master advanced from {expected} to {head}; the newer head\'s own CI completion will trigger its release run.')
    with Path(os.environ['GITHUB_OUTPUT']).open('a') as output:
        print('skip=' + str(skip).lower(), file=output)


def preflight():
    # A later green commit cannot authorize tagging an earlier failed merge.
    repo = os.environ['GITHUB_REPOSITORY']
    label = json.loads(Path('release-please-config.json').read_text())['label']
    query = urlencode({'state': 'closed', 'labels': label, 'per_page': 100})
    pages = json.loads(gh('api', '--paginate', '--slurp', f'repos/{repo}/issues?{query}'))
    for issue in (issue for page in pages for issue in page if 'pull_request' in issue):
        pr = json.loads(gh('api', f'repos/{repo}/pulls/{issue["number"]}'))
        if not pr['merged']:
            continue
        source = pr['merge_commit_sha']
        runs = json.loads(gh('api', f'repos/{repo}/actions/workflows/ci.yml/runs?head_sha={source}&branch=master&event=push&status=success'))['workflow_runs']
        assert runs, 'Refusing to tag a merged release PR without successful exact-source CI'
        gate(runs[0], source)


def main():
    repo = os.environ['GITHUB_REPOSITORY']
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    tag = os.environ.get('RELEASE_TAG') or 'v' + json.loads(Path('.release-please-manifest.json').read_text())['.']
    assert re.fullmatch(r'v\d+\.\d+\.\d+(?:-[\w.]+)?', tag)
    raw = gh('api', f'repos/{repo}/releases/tags/{tag}', absent=True)
    if not raw:
        assert not os.environ.get('RELEASE_TAG'), 'Generator reported a release that cannot be read back'
        print('No tagged release to finalize; source and deployment remain separate.')
        return
    release = json.loads(raw)
    assert not release['draft'] and release['tag_name'] == tag
    source = json.loads(gh('api', f'repos/{repo}/commits/{tag}'))['sha']
    if not os.environ.get('RELEASE_TAG') and source != head:
        print('No release at this head; existing release left unchanged.')
        return
    if os.environ.get('RELEASE_SHA'):
        assert source == os.environ['RELEASE_SHA'], 'Release tag moved from generator source'
    subprocess.run(['git', 'merge-base', '--is-ancestor', source, head], check=True)
    runs = json.loads(gh('api', f'repos/{repo}/actions/workflows/ci.yml/runs?head_sha={source}&branch=master&event=push&status=success'))['workflow_runs']
    assert runs, 'Tagged source has no successful CI artifact'
    run = runs[0]
    gate(run, source)
    artifacts = json.loads(gh('api', f'repos/{repo}/actions/runs/{run["id"]}/artifacts'))['artifacts']
    matches = [a for a in artifacts if a['name'] == 'scry-worker-' + source and not a['expired']]
    assert len(matches) == 1, 'Expected one exact tested Worker artifact'
    artifact = matches[0]
    assert re.fullmatch(r'sha256:[a-f0-9]{64}', artifact['digest'])
    digest = artifact['digest'].removeprefix('sha256:')
    raw = gh('api', f'repos/{repo}/actions/artifacts/{artifact["id"]}/zip')
    assert hashlib.sha256(raw).hexdigest() == digest, 'Artifact transport checksum differs'
    with tempfile.TemporaryDirectory() as directory:
        path = Path(directory) / 'scry-worker.zip'
        assets = [a for a in release['assets'] if a['name'] == path.name]
        assert len(assets) <= 1
        if assets:
            gh('release', 'download', tag, '--repo', repo, '--pattern', path.name, '--dir', directory)
            assert path.read_bytes() == raw, 'Refusing to replace a different published artifact'
        else:
            path.write_bytes(raw)
            gh('release', 'upload', tag, str(path), '--repo', repo)
        published = json.loads(gh('api', f'repos/{repo}/releases/tags/{tag}'))
        assert any(a['name'] == path.name and a['size'] == len(raw) for a in published['assets'])
    gh('workflow', 'run', 'ci.yml', '--repo', repo, '--ref', 'master', '-f', 'mode=promote',
       '-f', 'artifact_id=' + str(artifact['id']), '-f', 'artifact_sha256=' + digest)
    print(json.dumps({'tag': tag, 'source': source, 'ci_run': run['id'], 'artifact_id': artifact['id'],
                      'artifact_sha256': digest, 'promotion_dispatched': True}))


def report_failure():
    repo = os.environ['GITHUB_REPOSITORY']
    title = 'Release automation failed'
    body = 'Release or promotion failed. Inspect ' + os.environ['RUN_URL'] + '. A tag is not a production readback; repair and rerun the existing exact-artifact path.'
    issues = json.loads(gh('issue', 'list', '--repo', repo, '--state', 'open', '--search', '"' + title + '" in:title', '--json', 'number'))
    if issues:
        gh('issue', 'comment', str(issues[0]['number']), '--repo', repo, '--body', body)
    else:
        gh('issue', 'create', '--repo', repo, '--title', title, '--body', body)


if __name__ == '__main__':
    if sys.argv[1:] == ['report-failure']:
        report_failure()
    elif sys.argv[1:] == ['check-head']:
        check_head()
    elif sys.argv[1:] == ['preflight']:
        preflight()
    else:
        assert not sys.argv[1:]
        main()
