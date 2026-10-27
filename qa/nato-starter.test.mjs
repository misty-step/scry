import test from 'node:test';
import {mkdir,writeFile} from 'node:fs/promises';
import {starterJourney} from './nato-starter.mjs';
await test('NATO starter: explicit native import, Learn and server-graded answer',async()=>{
 const report=await starterJourney();await mkdir('target/keyed-mapping',{recursive:true});await writeFile('target/keyed-mapping/starter-runtime.json',JSON.stringify(report,null,2)+'\n');
});
