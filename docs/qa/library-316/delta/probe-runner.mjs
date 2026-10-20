import {writeFile} from 'node:fs/promises';
const {libraryJourney}=await import(process.argv[2]);
const journey=await libraryJourney();
try {
 for(let i=0;i<5;i++){await journey.criteria[i]();console.log(`journey step ${i+1}: PASS`);}
} catch(error) {console.error(error);process.exitCode=1;}
finally {
 await writeFile(process.argv[3],JSON.stringify(journey.transcript,null,2)+'\n');
 await journey.close();
}
