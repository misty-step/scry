import {normalizedNatoJourney} from './nato-import-identity.mjs';
import {studyReviewJourney} from './study-review.mjs';
import {continuousLibraryJourney} from './continuous-library.mjs';
import {mixedReviewJourney,continuousStudyJourney} from './mixed-review.mjs';
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

await test('Default review fairly weaves practiced due subjects and advances same-day due work',async()=>{await writeFile(join(output,'mixed-review-runtime.json'),JSON.stringify(await mixedReviewJourney(),null,2)+'\n');});

await test('Default Study continues all canonical subjects through graded learning and paced completion',async()=>{await writeFile(join(output,'continuous-study-runtime.json'),JSON.stringify(await continuousStudyJourney(),null,2)+'\n');});

await test('Default Study admits external Library once and preserves paused and dropped subjects',async()=>{await writeFile(join(output,'continuous-library-runtime.json'),JSON.stringify(await continuousLibraryJourney(),null,2)+'\n');});

await test('Study shows sources without copy grading; Review hides answers and preserves assistance',async()=>{await writeFile(join(output,'study-review-runtime.json'),JSON.stringify(await studyReviewJourney(),null,2)+'\n');});

await test('Normalized Owner API NATO imports retain identity, history and strict grading through both starters',async()=>{await writeFile(join(output,'normalized-import-runtime.json'),JSON.stringify(await normalizedNatoJourney(),null,2)+'\n');});
