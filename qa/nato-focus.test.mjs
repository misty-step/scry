import test from 'node:test';
import {mkdir,writeFile} from 'node:fs/promises';
import {join} from 'node:path';
import {focusedJourney,scheduledJourney,starterFixture,seedProgressFixture} from './nato-starter.mjs';
// The existing keyed-mapping browser proof runs starterWalk once and exports
// its phone/desktop screenshots; keep the atomic runtime proof alongside it.
const output='target/keyed-mapping';await mkdir(output,{recursive:true});
await test('NATO focus: one native action, protected recall, duplicate safety and honest progress',async()=>{
 const report=await focusedJourney();await writeFile(join(output,'focused-runtime.json'),JSON.stringify(report,null,2)+'\n');
});

await test('NATO scheduled selection skips future cards and stays caught up',async()=>{const report=await scheduledJourney();await writeFile(join(output,'scheduled-runtime.json'),JSON.stringify(report,null,2)+'\n');});

await test('Rich progress fixture deliberately requests extra practice after scheduler exhaustion',async()=>{const f=await starterFixture();try{await seedProgressFixture(f);}finally{await f.close();}});
