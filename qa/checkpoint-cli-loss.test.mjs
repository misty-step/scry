import test from 'node:test';
import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {spawn} from 'node:child_process';
import {mkdtemp,rm,writeFile,chmod} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve} from 'node:path';

test('checkpoint CLI requires a retained ID and retries the exact lost-response payload',async t=>{
  const temp=await mkdtemp(join(tmpdir(),'scry-cli-loss-'));
  t.after(()=>rm(temp,{recursive:true,force:true}));
  const requests=[];
  const server=createServer(async(req,res)=>{
    try {
      const chunks=[];for await(const chunk of req)chunks.push(chunk);
      const value=JSON.parse(Buffer.concat(chunks).toString());requests.push(value);
      if(requests.length===1){req.socket.destroy();return;}
      const body=JSON.stringify({result:{data:{draft_generation:1,prompt_id:value.prompt_id},redirect:'/',entity_id:null,message:null},replayed:true});
      res.writeHead(200,{'Content-Type':'application/json','Content-Length':Buffer.byteLength(body)});res.end(body);
    } catch(error) {res.destroy(error);}
  });
  await new Promise((resolve,reject)=>{server.once('error',reject);server.listen(0,'127.0.0.1',resolve);});
  t.after(()=>new Promise(resolve=>server.close(resolve)));
  const config=join(temp,'login.json'),token='scry_ot_'+'a'.repeat(32)+'_'+'b'.repeat(48);
  await writeFile(config,JSON.stringify({origin:`http://127.0.0.1:${server.address().port}`,token}));await chmod(config,0o600);
  const run=(args,input)=>new Promise((resolveRun,reject)=>{
    const child=spawn(resolve('target/debug/scry'),[...args,'--json','--config',config],{stdio:['pipe','pipe','pipe']});let stdout='',stderr='';
    child.stdout.on('data',chunk=>stdout+=chunk);child.stderr.on('data',chunk=>stderr+=chunk);child.on('error',reject);
    child.on('close',code=>{try{assert.ok(!stdout.includes(token)&&!stderr.includes(token));resolveRun({code,value:JSON.parse(stdout)});}catch(error){reject(error);}});child.stdin.end(input);
  });
  const prompt='a'.repeat(32),text='synthetic answer',without=['save-draft',prompt,'--generation','0','--text-stdin'];
  const refusal=await run(without,text);assert.equal(refusal.code,1);assert.match(refusal.value.error,/operation-id/);assert.equal(requests.length,0);
  const args=[...without,'--operation-id','checkpoint-cli-retained-01'];
  const lost=await run(args,text);assert.equal(lost.code,1);assert.match(lost.value.error,/same operation ID/);assert.equal(requests.length,1);
  const retry=await run(args,text);assert.equal(retry.code,0);assert.equal(retry.value.replayed,true);assert.equal(requests.length,2);
  assert.deepEqual(requests[1],requests[0]);assert.equal(requests[0].operation_id,'checkpoint-cli-retained-01');
});
