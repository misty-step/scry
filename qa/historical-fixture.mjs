// Explicit lower-level setup of synthetic historical records. This is not an
// application API and never enables a retired HTTP operation. The native helper
// validates the complete input/output graph and bounded archive before any SQL.
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {resolve} from 'node:path';

export async function historicalMutation(storage,payload) {
 const before=await storage.exec('SELECT key,value FROM records ORDER BY key');
 assert.ok(before.some(row=>row.key==='meta'),'Initialize the isolated fixture first.');
 const rows=Object.fromEntries(before.map(row=>[row.key,row.value]));
 const input={now_ms:Date.now(),seed:'historical-synthetic-fixture',...payload,rows};
 const child=spawnSync(resolve(process.env.SCRY_HISTORICAL_FIXTURE||'target/debug/examples/qa-historical-fixture'),[],{
  input:JSON.stringify(input),encoding:'utf8',maxBuffer:40*1024*1024,
  env:{PATH:process.env.PATH,LANG:'C.UTF-8'},
 });
 if(child.error)throw child.error;
 let output;try{output=JSON.parse(child.stdout);}catch{throw Error('Historical fixture helper returned unreadable output.');}
 if(child.status!==0){const error=new Error(output.error||'Historical fixture refused.');error.status=output.status;throw error;}
 assert.equal(output.provenance,'synthetic-historical-engine-fixture; not a supported current HTTP write');
 assert.deepEqual(await storage.exec('SELECT key,value FROM records ORDER BY key'),before,'Fixture setup refuses concurrent runtime writes.');
 // Only changed rows are written. Old receipts and learning evidence survive;
 // deleting rows or overwriting a live namespace is not a helper capability.
 for(const row of before)assert.ok(Object.hasOwn(output.rows,row.key),'Historical setup may not delete existing records.');
 for(const [key,value]of Object.entries(output.rows))if(rows[key]!==value)
  await storage.exec('INSERT INTO records(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value',key,value);
 assert.deepEqual(Object.fromEntries((await storage.exec('SELECT key,value FROM records ORDER BY key')).map(row=>[row.key,row.value])),output.rows);
 const {rows:writtenRows,results,provenance,...receipt}=output;
 return receipt;
}
