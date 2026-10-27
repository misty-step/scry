import {spawn} from 'node:child_process';

// Captured subprocess boundary for the Library journey; no retries.
export function cliProcess(binary,args,{env=process.env,input=''}={}) {
 return new Promise((done,reject)=>{
  // Ordinary commands do not read stdin. Even an empty write to a closed pipe
  // can emit EPIPE, so do not create that pipe when there is nothing to send.
  const proc=spawn(binary,args,{env,stdio:[input.length?'pipe':'ignore','pipe','pipe']});
  let out='',err='',stdinError=null;
  proc.stdout.setEncoding('utf8');proc.stderr.setEncoding('utf8');
  proc.stdout.on('data',data=>out+=data);proc.stderr.on('data',data=>err+=data);
  proc.on('error',reject);
  proc.stdin?.on('error',error=>{stdinError=error;});
  // exit can precede the final stdout/stderr bytes; close waits for both pipes.
  proc.on('close',(code,signal)=>{
   if(signal){reject(Error('CLI terminated before completion.'));return;}
   if(stdinError&&(stdinError.code!=='EPIPE'||code===0)){
    const error=Error('CLI stdin delivery failed.');error.code=stdinError.code;error.exitCode=code;reject(error);return;
   }
   // A refused command can exit before consuming input. Its actual nonzero
   // exit and complete output remain mandatory assertions in the journey.
   done({code,out,err,stdinError:stdinError?.code??null});
  });
  proc.stdin?.end(input);
 });
}
