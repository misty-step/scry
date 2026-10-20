#!/usr/bin/env python3
"""Run the real recovery assertion once with slow startup and a slow 429 bridge."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def replace_once(text, before, after):
    assert text.count(before) == 1, f"probe splice changed: {before!r}"
    return text.replace(before, after)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--artifact', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    parser.add_argument('--test-revision', help='Git revision; default is the worktree test')
    parser.add_argument('--expect', choices=['red', 'green'], required=True)
    args = parser.parse_args()
    source, artifact, out = args.source.resolve(), args.artifact.resolve(), args.out.resolve()
    out.mkdir(parents=True, exist_ok=False)
    test_path = 'qa/rust-telemetry.test.mjs'
    original = subprocess.check_output(['git', 'show', f'{args.test_revision}:{test_path}'], cwd=source).decode() if args.test_revision else (source / test_path).read_text()
    probe = replace_once(original, "'./runtime-helper.mjs'", json.dumps(str(source / 'qa/runtime-helper.mjs')))
    if 'async function instance(changes={})' in probe:
        probe = replace_once(probe, 'async function instance(changes={})', 'async function instance(changes={},onLog)')
    else:
        probe = replace_once(probe, 'async function instance(changes={},handleStructuredLogs)', 'async function instance(changes={},onLog)')
        probe = replace_once(probe, 'return createRuntime({bindings,handleStructuredLogs,', 'return createRuntime({bindings,')
    probe = replace_once(probe, 'const received=[];', "const received=[];\nconst timing=(event)=>console.log('[350] '+JSON.stringify({event,ms:Date.now()}));")
    probe = replace_once(probe, 'received.push({body,headers:request.headers});', "timing('sink_receipt');received.push({body,headers:request.headers});")
    # Observe the same post-processing warning as the candidate. Do not change
    # the compiled reporter, its clocks, its assertions, or its retry policy.
    hook = "handleStructuredLogs:log=>{if(log.message==='[scry-rust] telemetry unavailable; report dropped')timing('drop_observed');onLog?.(log);},"
    probe = replace_once(probe, 'return createRuntime({bindings,', 'return createRuntime({bindings,' + hook)
    probe = replace_once(probe, "return new Response(await response.text(),{status:response.status,headers:response.headers});", "const body=await response.text();if(response.status===429){timing('bridge_response_received');\n    await new Promise(resolve=>setTimeout(resolve,800));timing('bridge_response_returned');}\n    return new Response(body,{status:response.status,headers:response.headers});")
    # Delay after the old early anchor, immediately before the first request.
    marker = "      assert.equal((await local.worker.fetch(origin+'/healthz')).status,503);"
    start = probe.index("await test('same-isolate reporting recovers")
    position = probe.index(marker, start)
    probe = probe[:position] + "      timing('before_startup_delay');await new Promise(resolve=>setTimeout(resolve,2200));timing('first_request');\n" + probe[position:]
    if 'started+61500' in probe:
        probe = replace_once(probe, 'const started=Date.now();', "const started=Date.now();timing('legacy_anchor');")
    second = probe.index(marker, probe.index(marker, position) + len(marker))
    probe = probe[:second] + "      timing('second_request');\n" + probe[second:]
    probe = replace_once(probe, 'assert.equal(received.length,2);', "timing('second_assertion');assert.equal(received.length,2);")
    (out / 'probe.mjs').write_text(probe)
    artifacts = {str(p.relative_to(artifact)): sha(p) for p in sorted(artifact.rglob('*')) if p.is_file()}
    env = {key: os.environ[key] for key in ('PATH', 'HOME', 'TMPDIR') if key in os.environ}
    env.update(SCRY_WORKER_ARTIFACT=str(artifact), WRANGLER_SEND_METRICS='false', CLOUDFLARE_LOAD_DEV_VARS_FROM_DOT_ENV='false')
    command = ['node', '--test', '--test-name-pattern=same-isolate', str(out / 'probe.mjs')]
    result = subprocess.run(command, cwd=source, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=150)
    (out / 'runtime.log').write_bytes(result.stdout)
    events = [json.loads(line.split('[350] ', 1)[1]) for line in result.stdout.decode().splitlines() if '[350] ' in line]
    assert artifacts == {str(p.relative_to(artifact)): sha(p) for p in sorted(artifact.rglob('*')) if p.is_file()}, 'artifact mutated'
    times = {item['event']: item['ms'] for item in events if item['event'] != 'sink_receipt'}
    sends = [item['ms'] for item in events if item['event'] == 'sink_receipt']
    assert times['bridge_response_received'] < times['bridge_response_returned'] <= times['drop_observed'] < times['second_request']
    gap = times['second_request'] - times['drop_observed']
    if args.expect == 'red':
        assert result.returncode == 1 and '1 !== 2' in result.stdout.decode(), 'did not reproduce the original assertion'
        assert len(sends) == 1 and gap < 60_000, 'not the early-deadline mechanism'
    else:
        assert result.returncode == 0 and len(sends) == 2 and gap >= 60_000, 'recovery boundary failed'
    evidence = dict(test_revision=args.test_revision or 'worktree', test_sha256=hashlib.sha256(original.encode()).hexdigest(), helper_sha256=sha(source / 'qa/runtime-helper.mjs'), probe_sha256=sha(out / 'probe.mjs'), log_sha256=sha(out / 'runtime.log'), command=command, environment=env, exit_code=result.returncode, artifacts=artifacts, events=events, since_drop_ms=gap)
    (out / 'result.json').write_text(json.dumps(evidence, indent=2) + '\n')
    print(json.dumps(evidence, indent=2))


if __name__ == '__main__':
    main()
