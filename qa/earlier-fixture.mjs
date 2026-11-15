// Loads one stored state of the earlier concept notebook into an unused
// Durable Object: the exact rows of tests/fixtures/earlier-notebook.json and
// the photo objects its archive carries. This is lower-level setup, not an
// application API. No HTTP route can create these states any more; the fixture
// is their only source (tests/fixtures/earlier-notebook.README.md).
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

const fixture=JSON.parse(readFileSync(new URL('../tests/fixtures/earlier-notebook.json',import.meta.url),'utf8'));
export const states=Object.keys(fixture);
// {archive, sha256, bytes, format, rows, now_ms} exactly as committed.
export const stored=name=>{assert.ok(fixture[name],`No earlier-notebook state named ${name}.`);return fixture[name];};
export const storedMeta=name=>JSON.parse(stored(name).rows.meta);
export const storedApp=name=>JSON.parse(stored(name).archive).app;

// The exact stored text of one top-level field of a JSON object, so a nested
// value is compared as bytes and never through a second serializer.
export function rawField(text,field){
 let depth=0,string=false;
 for(let i=0;i<text.length;i++){
  const ch=text[i];
  if(string){if(ch==='\\')i++;else if(ch==='"')string=false;continue;}
  if(ch==='"'){
   const end=text.indexOf('"',i+1);
   if(depth===1&&text.slice(i+1,end)===field&&text[end+1]===':'){
    let j=end+2,level=0,inner=false;
    for(;j<text.length;j++){
     const c=text[j];
     if(inner){if(c==='\\')j++;else if(c==='"')inner=false;continue;}
     if(c==='"')inner=true;else if(c==='{'||c==='[')level++;
     else if(c==='}'||c===']'){if(level===0)break;level--;}
     else if(c===','&&level===0)break;
    }
    return text.slice(end+2,j);
   }
   string=true;continue;
  }
  if(ch==='{'||ch==='[')depth++;else if(ch==='}'||ch===']')depth--;
 }
 return undefined;
}

// `storage` is Miniflare's inspection handle for the object; `assets` its R2
// bucket for photos. Refuses anything but a never-used object.
export async function seedEarlier(storage,name,assets){
 const state=stored(name);
 await storage.exec('CREATE TABLE IF NOT EXISTS records (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID');
 assert.deepEqual(await storage.exec('SELECT key FROM records'),[],'An earlier-notebook state is loaded only into an unused object.');
 for(const [key,value] of Object.entries(state.rows))await storage.exec('INSERT INTO records(key,value) VALUES(?,?)',key,value);
 const photos=JSON.parse(state.archive).photos;
 if(photos.length){
  assert.ok(assets,`${name} carries a photo: pass the SCRY_ASSETS bucket.`);
  for(const {photo,bytes} of photos)await assets.put(photo.key,Buffer.from(bytes,'base64'));
 }
 assert.deepEqual(await earlierRows(storage),state.rows);
 return state;
}
export const earlierRows=async storage=>Object.fromEntries((await storage.exec('SELECT key,value FROM records ORDER BY key')).map(row=>[row.key,row.value]));
