#!/usr/bin/env python3
"""Promote an existing successful CI artifact with native Wrangler, never rebuild."""
import email.parser
import email.policy
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tomllib
from typing import Any
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'target/ci-release'
CONFIG = ROOT / 'deploy/rust-production.toml'
RECEIPT = ROOT / 'target/promotion.json'
sha = lambda value: hashlib.sha256(value).hexdigest()

def gh(path):
    return subprocess.check_output(['gh', 'api', 'repos/' + os.environ['GITHUB_REPOSITORY'] + '/' + path])

def ancestor(old, new):
    assert re.fullmatch('[a-f0-9]{40}', old) and re.fullmatch('[a-f0-9]{40}', new), 'Invalid source revision for ancestry check'
    subprocess.run(['git', 'merge-base', '--is-ancestor', old, new], cwd=ROOT, check=True)

def traffic(deployment):
    versions = deployment.get('versions')
    assert isinstance(versions, list) and versions, 'Invalid deployment traffic; refusing promotion'
    assert all(isinstance(v, dict) and isinstance(v.get('version_id'), str) and v['version_id']
               and type(v.get('percentage')) in (int, float) and 0 <= v['percentage'] <= 100
               for v in versions), 'Invalid deployment traffic; refusing promotion'
    assert len({v['version_id'] for v in versions}) == len(versions) and sum(v['percentage'] for v in versions) == 100, 'Invalid deployment traffic; refusing promotion'
    return versions

def single_version(deployment):
    versions = traffic(deployment)
    assert len(versions) == 1 and versions[0]['percentage'] == 100, 'Live deployment must be a single version at 100%; refusing promotion'
    return versions[0]['version_id']

def deployment_history(request, script):
    """Read all retained native history, never assume a short first page is complete."""
    history, seen, bounds = [], set(), None
    for page in range(1, 101):
        body = request(script + f'/deployments?page={page}&per_page=100', envelope=True)
        batch = body['result']['deployments']
        assert isinstance(batch, list), 'Invalid deployment history'
        info = body.get('result_info')
        if info is not None:
            assert all(type(info.get(k)) is int for k in ('page', 'total_pages', 'total_count')), 'Incomplete deployment history pagination'
            assert info['page'] == page and info['total_pages'] >= page and info['total_count'] >= 0, 'Invalid deployment history pagination'
            current = (info['total_pages'], info['total_count'])
            assert bounds is None or bounds == current, 'Deployment history changed during pagination'
            bounds = current
        else:
            assert bounds is None, 'Deployment history pagination disappeared'
        for item in batch:
            identity = item.get('id')
            assert isinstance(identity, str) and identity and identity not in seen, 'Ambiguous or repeated deployment history page'
            seen.add(identity)
            history.append(item)
        if bounds is not None and page == bounds[0]:
            assert len(history) == bounds[1], 'Incomplete deployment history'
            break
        if not batch:
            assert bounds is None, 'Incomplete deployment history'
            break
    else:
        raise AssertionError('Deployment history exceeds the bounded read; refusing promotion')
    assert history, 'No live deployment history; refusing promotion'
    return history

def release_source(current):
    match = re.fullmatch('scry-rust ([a-f0-9]{40})', current.get('annotations', {}).get('workers/message', ''))
    return match[1] if match else None

def live_release(deployments, version):
    """Source of the live code and whether its own version names it. A secret-only change keeps the
    script, so it resolves to the release it re-deployed only when every step has the same script etag."""
    etag = None
    for deployment in deployments:
        current = version(single_version(deployment))
        script = current.get('resources', {}).get('script', {}).get('etag')
        if etag is not None and script != etag: return None
        annotations = current.get('annotations', {})
        prior = release_source(current)
        if prior: return prior, etag is None
        if annotations.get('workers/triggered_by') != 'secret' or not script: return None
        etag = script
    return None

def refuse_rollback(deployments, source, version):
    """Only actually deployed releases count; a newer upload alone never served traffic."""
    checked = {source}
    for deployment in deployments:
        for entry in traffic(deployment):
            if not entry['percentage']: continue
            prior = release_source(version(entry['version_id']))
            if prior is None or prior in checked: continue
            try:
                ancestor(prior, source)
            except subprocess.CalledProcessError as error:
                if error.returncode != 1: raise
                raise AssertionError('Manual rollback or divergent promoted history is in effect; operator resolution required before promotion') from error
            checked.add(prior)

def save(value):
    RECEIPT.write_text(json.dumps(value, indent=2) + '\n')

def prepare():
    identity, checksum = os.environ['ARTIFACT_ID'], os.environ['ARTIFACT_SHA256']
    assert re.fullmatch('[0-9]+', identity) and re.fullmatch('[a-f0-9]{64}', checksum), 'Invalid artifact ID or SHA256'
    metadata = json.loads(gh('actions/artifacts/' + identity))
    run = json.loads(gh('actions/runs/' + str(metadata['workflow_run']['id'])))
    assert not metadata['expired'] and run['conclusion'] == 'success', 'Artifact expired or CI run did not succeed'
    assert run['head_branch'] == 'master' and run['event'] == 'push' and run['path'] == '.github/workflows/ci.yml', 'Artifact must come from master push CI'
    ancestor(run['head_sha'], os.environ['GITHUB_SHA'])
    raw = gh('actions/artifacts/' + identity + '/zip')
    assert sha(raw) == checksum and metadata['digest'] == 'sha256:' + checksum, 'Artifact archive digest mismatch'
    OUT.mkdir(parents=True, exist_ok=False)
    archive = OUT.parent / 'promotion.zip'
    archive.write_bytes(raw)
    with zipfile.ZipFile(archive) as z:
        assert all(not Path(n).is_absolute() and '..' not in Path(n).parts for n in z.namelist()), 'Artifact archive contains an unsafe path'
        z.extractall(OUT)
    for line in (OUT / 'SHA256SUMS').read_text().splitlines():
        digest, name = line.split(None, 1)
        assert sha((OUT / name).read_bytes()) == digest, 'Artifact file checksum mismatch'
    proof = json.loads((OUT / 'proof.json').read_text())
    assert proof['source_state'] == 'committed' and proof['revision'] == run['head_sha'], 'Artifact proof must match committed CI source'
    for name in ('deploy/rust-production.toml', 'scripts/scry-promote.py'):
        expected = subprocess.check_output(['git', 'show', proof['revision'] + ':' + name], cwd=ROOT)
        assert (ROOT / name).read_bytes() == expected, 'Promotion configuration differs from tested source'
    save({'artifact_id': int(identity), 'artifact_zip_sha256': checksum, 'source': proof['revision'], 'stage': 'artifact_verified'})

def storage(bindings):
    """Durable identity Wrangler would rebind: DO class (and foreign script) or R2 bucket, by binding name."""
    return {b['name']: [b['type'], b.get('class_name') or b.get('bucket_name'), b.get('script_name')]
            for b in bindings if b['type'] in ('durable_object_namespace', 'r2_bucket')}

def declared(config):
    objects = config['durable_objects']['bindings']
    assert all(b.get('script_name', config['name']) == config['name'] for b in objects), 'Durable Object binding points at another Worker'
    return storage([{'name': b['name'], 'class_name': b['class_name'], 'type': 'durable_object_namespace'} for b in objects]
                   + [{'name': b['binding'], 'bucket_name': b['bucket_name'], 'type': 'r2_bucket'} for b in config['r2_buckets']])

def edges(domains, routes, service):
    """Everything that sends traffic to the Worker: Custom Domains and the script's own routes on any zone."""
    return {'custom_domains': sorted([d['hostname'], d['zone_name'], d['zone_id'], d['environment'], d['enabled']] for d in domains if d['service'] == service),
            'routes': sorted(r['pattern'] for r in routes)}

def deploy():
    config = tomllib.loads(CONFIG.read_text())
    base = 'https://api.cloudflare.com/client/v4/accounts/' + config['account_id']
    script = '/workers/scripts/' + config['name']
    def request(path, raw=False, envelope=False) -> Any:
        req = urllib.request.Request(base + path, headers={'Authorization': 'Bearer ' + os.environ['CLOUDFLARE_API_TOKEN']})
        with urllib.request.urlopen(req, timeout=60) as response:
            if raw: return response.read(), response.headers['Content-Type']
            body = json.load(response)
            assert body['success'], 'Cloudflare API request failed'
            return body if envelope else body['result']
    receipt = json.loads(RECEIPT.read_text())
    before = request(script + '/settings')
    bindings = {b['name']: b for b in before['bindings']}
    assert storage(before['bindings']) == declared(config), 'Configured storage bindings differ from live; refusing before upload'
    mutable = {k: bindings[k][field] for k, field in [('LEARNING_SPACE', 'namespace_id'), ('SCRY_ASSETS', 'bucket_name'), ('SCRY_BACKUPS', 'bucket_name')]}
    assert bindings['SPACE_NAME']['text'] == config['vars']['SPACE_NAME'], 'Live SPACE_NAME differs from configured space'
    assert {b['name'] for b in before['bindings'] if b['type'] == 'secret_text'} >= {'INTERNAL_KEY', 'OWNER_SUBJECT', 'BACKUP_TRIGGER_KEY'}, 'Required production secrets are missing'
    assert not ({'RESTORE_TOKEN', 'SYNTHETIC_ACCESS_CLIENT_ID'} & set(bindings)), 'Restore or synthetic access binding present before upload'
    for bucket in config['r2_buckets']:
        assert bindings[bucket['binding']]['bucket_name'] == bucket['bucket_name'], 'Live R2 bucket differs from configured bucket'
        assert not request('/r2/buckets/' + bucket['bucket_name'] + '/domains/managed')['enabled'], 'R2 managed domain is enabled'
        assert request('/r2/buckets/' + bucket['bucket_name'] + '/domains/custom')['domains'] == [], 'R2 custom domains are present'
    # An empty target cannot be activated accidentally. The operator bridge's
    # complete readback is the authority; presence alone is not restore proof.
    snapshots = request('/r2/buckets/' + mutable['SCRY_BACKUPS'] + '/objects?per_page=1000&prefix=snapshots%2F' + config['vars']['SPACE_NAME'] + '%2F')
    assert snapshots, 'Import and canonically verify the unused space before activation'
    deployments = deployment_history(request, script)
    previous = deployments[0]
    versions = {}
    def version(identity):
        if identity not in versions: versions[identity] = request(script + '/versions/' + identity)
        return versions[identity]
    prior = live_release(deployments, version)
    if prior:
        refuse_rollback(deployments, prior[0], version)
        ancestor(prior[0], receipt['source'])
    else: assert bindings['SCRY_ENV']['text'] == 'restore', 'Unrecognized live release'
    current = request(script + '/deployments')['deployments'][0]
    single_version(current)
    assert current['id'] == previous['id'], 'Live deployment changed before upload'
    observe = lambda: edges(request('/workers/domains?service=' + config['name']), request(script + '/routes'), config['name'])
    serving = observe()
    # A secret-only version of the same source still lacks the release annotation verified below.
    if prior != (receipt['source'], True):
        subprocess.run([str(ROOT / 'node_modules/.bin/wrangler'), 'deploy', '--config', str(CONFIG), '--no-bundle', '--message', 'scry-rust ' + receipt['source']], cwd=ROOT, check=True)
    after = request(script + '/settings')
    exact = {b['name']: b for b in after['bindings']}
    assert observe() == serving, 'Custom domains or routes changed during promotion'
    assert {k: exact[k][field] for k, field in [('LEARNING_SPACE', 'namespace_id'), ('SCRY_ASSETS', 'bucket_name'), ('SCRY_BACKUPS', 'bucket_name')]} == mutable, 'Durable storage identity changed during promotion'
    assert exact['SPACE_NAME']['text'] == config['vars']['SPACE_NAME'] and exact['SCRY_ENV']['text'] == 'production', 'Promoted space or environment differs from configuration'
    assert not ({'RESTORE_TOKEN', 'SYNTHETIC_ACCESS_CLIENT_ID'} & set(exact)), 'Restore or synthetic access binding present after upload'
    assert request(script + '/subdomain') == {'enabled': False, 'previews_enabled': False}, 'Worker subdomain or previews are enabled'
    raw, kind = request(script + '/content/v2', raw=True)
    parts = email.parser.BytesParser(policy=email.policy.default).parsebytes(('Content-Type: ' + kind + '\r\n\r\n').encode() + raw)
    actual = {}
    for part in parts.iter_parts():
        name = part.get_filename() or part.get_param('name', header='Content-Disposition')
        assert isinstance(name, str), 'Deployed module has no valid filename'
        actual[Path(name).name] = sha(part.get_payload(decode=True))
    # worker/shim.mjs is the artifact's backwards-compatibility re-export only;
    # native Wrangler uses the exact tested index.js directly.
    expected = {name: digest for name, digest in json.loads((OUT / 'proof.json').read_text())['worker'].items() if name in ('index.js', 'index_bg.wasm')}
    assert actual == expected, 'Deployed modules differ from verified artifact'
    deployed = request(script + '/deployments')['deployments'][0]
    final = version(single_version(deployed))
    assert release_source(final) == receipt['source'], 'Deployed release annotation differs from promoted source'
    receipt.update(stage='exact_artifact_promoted', worker=config['name'], deployment=deployed, mutable_bindings_unchanged=True,
                   module_sha256=actual, routes_changed=False, serving_edges=serving, learner_bodies_requested=False)
    save(receipt)
    print(json.dumps(receipt))

if __name__ == '__main__':
    {'prepare': prepare, 'deploy': deploy}[sys.argv[1]]()
