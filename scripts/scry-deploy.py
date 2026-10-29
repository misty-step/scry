#!/usr/bin/env python3
"""Deploy this CI job's tested Worker and read back its exact bytes. No data operations."""
import email.parser
import email.policy
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tomllib
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'target/ci-release'
CONFIG = ROOT / 'deploy/rust-production.toml'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def storage(settings):
    """Compare durable identities without reading any learner data or secret value."""
    return {b['name']: (b['type'], b.get('namespace_id'), b.get('bucket_name'))
            for b in settings['bindings']
            if b['type'] in ('durable_object_namespace', 'r2_bucket')}


def module_hashes(raw, content_type):
    message = email.parser.BytesParser(policy=email.policy.default).parsebytes(
        ('Content-Type: ' + content_type + '\r\n\r\n').encode() + raw)
    if not message.is_multipart():
        raise RuntimeError('Cloudflare did not return Worker modules')
    result = {}
    for part in message.iter_parts():
        name = part.get_filename() or part.get_param('name', header='Content-Disposition')
        if not name or Path(name).name in result:
            raise RuntimeError('Invalid or duplicate Worker module')
        result[Path(name).name] = sha(part.get_payload(decode=True))
    return result


def deploy(request, run=subprocess.run):
    config = tomllib.loads(CONFIG.read_text())
    source = os.environ['GITHUB_SHA']
    proof = json.loads((OUT / 'proof.json').read_text())
    if proof['revision'] != source or proof['source_state'] != 'committed':
        raise RuntimeError('Build does not match this source')
    expected = {name: proof['worker'][name] for name in ('index.js', 'index_bg.wasm')}
    if {name: sha((OUT / 'worker' / name).read_bytes()) for name in expected} != expected:
        raise RuntimeError('Tested Worker bytes changed')
    script = '/workers/scripts/' + config['name']
    before = request(script + '/settings')
    bindings = {b['name']: b for b in before['bindings']}
    if bindings['SPACE_NAME']['text'] != config['vars']['SPACE_NAME']:
        raise RuntimeError('Configured learning space differs from existing production')
    for bucket in config['r2_buckets']:
        if bindings[bucket['binding']]['bucket_name'] != bucket['bucket_name']:
            raise RuntimeError('Configured bucket differs from existing production')
    for binding in config['durable_objects']['bindings']:
        actual = bindings[binding['name']]
        if actual['class_name'] != binding['class_name'] or actual.get('script_name', config['name']) != binding.get('script_name', config['name']):
            raise RuntimeError('Configured Durable Object differs from existing production')
    routes = request(script + '/routes')
    domains = request('/workers/domains?service=' + config['name'])
    run([str(ROOT / 'node_modules/.bin/wrangler'), 'deploy', '--config', str(CONFIG),
         '--no-bundle', '--message', 'scry-rust ' + source], cwd=ROOT, check=True)
    after = request(script + '/settings')
    if storage(after) != storage(before):
        raise RuntimeError('Deployed storage identity changed')
    if request(script + '/routes') != routes or request('/workers/domains?service=' + config['name']) != domains:
        raise RuntimeError('Deployed routes changed')
    if request(script + '/subdomain') != {'enabled': False, 'previews_enabled': False}:
        raise RuntimeError('Unexpected public Worker hostname')
    raw, kind = request(script + '/content/v2', raw=True)
    if module_hashes(raw, kind) != expected:
        raise RuntimeError('Live Worker bytes differ from the tested build')
    receipt = {'source': source, 'worker': config['name'], 'module_sha256': expected,
               'storage_unchanged': True, 'routes_unchanged': True,
               'learner_data_requested': False, 'live_acceptance': 'not_tested'}
    (ROOT / 'target/deployment.json').write_text(json.dumps(receipt, indent=2) + '\n')
    print(json.dumps(receipt))


def main():
    config = tomllib.loads(CONFIG.read_text())
    base = 'https://api.cloudflare.com/client/v4/accounts/' + config['account_id']
    def request(path, raw=False):
        req = urllib.request.Request(base + path, headers={
            'Authorization': 'Bearer ' + os.environ['CLOUDFLARE_API_TOKEN']})
        with urllib.request.urlopen(req, timeout=60) as response:
            if raw:
                return response.read(), response.headers['Content-Type']
            body = json.load(response)
            if not body['success']:
                raise RuntimeError('Cloudflare API request failed')
            return body['result']
    deploy(request)


if __name__ == '__main__':
    try:
        main()
    except urllib.error.HTTPError as error:
        sys.exit(f'Deployment check failed: Cloudflare HTTP {error.code}')
    except RuntimeError as error:
        sys.exit("Deployment check failed: " + str(error))
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError) as error:
        # Never print provider bodies, request URLs, credentials or learner data.
        sys.exit('Deployment failed: ' + type(error).__name__ + '; inspect the named CI step')
