#!/usr/bin/env python3
"""Deliver the shared capture alarm between the stale fixture and health GET.

Requires the pinned Node/dependencies and built Worker in the current checkout.
Writes only under the supplied unused scratch directory. No credential output.
"""
import argparse
import json
from pathlib import Path
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--out', type=Path, required=True)
parser.add_argument('--revision', help='Read the runtime test at this Git revision; default: checkout')
args = parser.parse_args()
root = Path.cwd()
args.out.mkdir(parents=True, exist_ok=False)
source = (subprocess.check_output(['git', 'show', f'{args.revision}:qa/runtime.test.mjs']).decode()
          if args.revision else (root / 'qa/runtime.test.mjs').read_text())
source = source[:source.index('  const recoveryEgress=')] + '\n} finally {await preview.dispose();}\n'
source = source.replace("'./runtime-helper.mjs'", "'./helper.mjs'")
helper = (root / 'qa/runtime-helper.mjs').read_text()
helper = helper.replace("require('miniflare')", f'require({json.dumps(str(root / "node_modules/miniflare"))})')
helper = helper.replace('this.inner=new RustLearningSpace(ctx,env);',
                        'this.probeCtx=ctx;this.inner=new RustLearningSpace(ctx,env);')
needle = 'fetch(request){return this.inner.fetch(request);}alarm(){return this.inner.alarm();}'
assert needle in helper, 'Inspection wrapper changed; inspect before adapting the probe.'
helper = helper.replace(needle, '''async fetch(request){
  if(new URL(request.url).pathname==='/__probe')return Response.json({alarm_ms:await this.probeCtx.storage.getAlarm()});
  if(new URL(request.url).pathname==='/__probe/recover'){
    this.alarmDone=Promise.withResolvers();await this.probeCtx.storage.setAlarm(Date.now());
    await this.alarmDone.promise;return Response.json({alarm_ms:await this.probeCtx.storage.getAlarm()});
  }
  return this.inner.fetch(request);
}async alarm(){
  console.log('[DEBUG-344] '+JSON.stringify({label:'alarm-start',timestamp_ms:Date.now()}));
  try{return await this.inner.alarm();}finally{
    console.log('[DEBUG-344] '+JSON.stringify({label:'alarm-end',timestamp_ms:Date.now(),alarm_ms:await this.probeCtx.storage.getAlarm()}));
    this.alarmDone?.resolve();
  }
}''')
start = source.index("  await check('chief backup capability")
head, check = source[:start], source[start:]
# The only controlled interleaving: deliver the original capture notebook's
# native alarm after the UPDATE, before the exact original stale assertion.
needle = "const stale=await request('/healthz','');assert.equal(stale.status,503);"
assert needle in check
isolated = "name='scry-chief-backup-smoke'" in check
mf = 'fixture.mf' if isolated else 'preview'
observe = f'''const observe=async label=>{{
  const row=JSON.parse((await storage.exec("SELECT value FROM records WHERE key='meta'"))[0].value);
  const ns=await {mf}.getDurableObjectNamespace('LEARNING_SPACE');
  const alarm=await(await ns.get(ns.idFromName(baseBindings.SPACE_NAME)).fetch('https://internal.invalid/__probe')).json();
  console.log('[DEBUG-344] '+JSON.stringify({{label,timestamp_ms:Date.now(),revision:row.revision,backup:row.backup,...alarm}}));
}};
await observe('after-update');
const ns=await preview.getDurableObjectNamespace('LEARNING_SPACE');
await ns.get(ns.idFromName(baseBindings.SPACE_NAME)).fetch('https://internal.invalid/__probe/recover');
await observe('after-shared-alarm');
const stale=await request('/healthz','');
console.log('[DEBUG-344] '+JSON.stringify({{label:'health',timestamp_ms:Date.now(),status:stale.status,body:await stale.clone().text()}}));
await observe('after-health');assert.equal(stale.status,503);'''
check = check.replace(needle, observe)
(args.out / 'probe.mjs').write_text(head + check)
(args.out / 'helper.mjs').write_text(helper)
result = subprocess.run(['node', '--test', str(args.out / 'probe.mjs')], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=120)
(args.out / 'runtime.log').write_bytes(result.stdout)
for line in result.stdout.decode(errors='replace').splitlines():
    if '[DEBUG-344]' in line or line.startswith(('not ok', '# pass', '# fail')):
        print(line)
raise SystemExit(result.returncode)
