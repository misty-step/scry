// Interactive synthetic journey for the collaborative preview. No provider access.
import {phoneFixture} from './phone-stream.mjs';
const f=await phoneFixture({port:Number(process.argv[2]||0)});
console.log(JSON.stringify({url:f.origin+'/recall',prompt:f.prompt}));
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,async()=>{await f.close();process.exit(0);});
