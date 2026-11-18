// Loads one stored state of the earlier concept notebook into an unused
// Durable Object: the exact rows of tests/fixtures/earlier-notebook.json and
// the photo objects its archive carries. This is lower-level setup, not an
// application API. No HTTP route can create these states any more; the fixture
// is their only source (tests/fixtures/earlier-notebook.README.md).
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

// Read on first use, so a harness that only imports this module (the walk's
// argument and binding checks) needs no fixture beside it.
let fixture;
const committed=()=>fixture??=JSON.parse(readFileSync(new URL('../tests/fixtures/earlier-notebook.json',import.meta.url),'utf8'));
export const states=()=>Object.keys(committed());
// {archive, sha256, bytes, format, rows, now_ms} exactly as committed.
export const stored=name=>{assert.ok(committed()[name],`No earlier-notebook state named ${name}.`);return committed()[name];};
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

// Reachability. Every stored thing of the earlier notebook, as the exact HTML
// fragment the page that holds it must contain: [path, fragment], where a null
// path means any page reached. The pages are reached only by following links
// from the Menu; tests/earlier_reachable.rs asserts the same natively.
export const escapeHtml=text=>text.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
export function expectedOnPages(app){
 const out=[],question=id=>`/questions/${id}/edit`;
 for(const g of Object.values(app.goals)){
  out.push([`/goals/${g.id}`,escapeHtml(g.title)],[`/goals/${g.id}`,escapeHtml(g.intent)]);
  if(g.transcript)out.push([`/goals/${g.id}`,escapeHtml(g.transcript)]);
  if(g.photo)out.push([`/goals/${g.id}`,`href="/photos/${g.id}"`]);
 }
 for(const c of Object.values(app.concepts)){
  const path=`/concepts/${c.id}`;out.push([path,escapeHtml(c.name)]);
  for(const note of c.notes){
   for(const p of note.text.split('\n\n').filter(p=>p.trim()))out.push([path,escapeHtml(p)]);
   for(const quote of note.quotes)out.push([path,`<blockquote>${escapeHtml(quote)}</blockquote>`]);
   if(note.basis==='general')out.push([path,'General knowledge']);
  }
  for(const p of c.prerequisites)out.push([path,`href="/concepts/${p}"`],[`/concepts/${p}`,`href="/concepts/${c.id}"`]);
 }
 for(const q of Object.values(app.questions)){
  out.push([`/concepts/${q.concept_id}`,`href="${question(q.id)}"`]);
  for(const v of [...q.versions,...(q.draft?[q.draft]:[])])for(const text of [v.prompt,v.answer,v.explanation,...v.choices,...v.quotes])out.push([question(q.id),escapeHtml(text)]);
  if(q.draft)out.push([question(q.id),'id="draft"']);
 }
 for(const e of app.events){
  out.push([question(e.question_id),`id="attempt-${e.id}"`]);
  for(const o of app.overrides.filter(o=>o.event_id===e.id))out.push([question(e.question_id),`id="correction-${o.id}">Your Correction: counted as ${o.correct?'recalled':'missed'}`]);
 }
 for(const r of app.resets)out.push([question(r.question_id),`id="reset-${r.id}"`]);
 for(const a of Object.values(app.assessments))out.push([question(a.question_id),`id="check-${a.id}" data-check="${a.status}"`],[question(a.question_id),escapeHtml(a.answer)]);
 for(const f of app.feedback)out.push([null,escapeHtml(f.text)]);
 // Retired generation records, on their Goal's page, exactly as saved.
 const raw=text=>`<pre class="raw-record">${escapeHtml(text)}</pre>`;
 for(const j of Object.values(app.jobs??{})){
  const path=`/goals/${j.goal_id}`;
  out.push([path,`id="job-${j.id}"`],[path,`Recorded status: ${escapeHtml(j.status)}`]);
  for(const text of [j.candidate_json,j.critic_json,j.critic_cache_json])if(text)out.push([path,raw(text)]);
  for(const reason of j.rejected)out.push([path,`<li>${escapeHtml(reason)}</li>`]);
  for(const model of [j.model,j.critic_model])if(model&&!model.startsWith('legacy-import'))out.push([path,escapeHtml(model)]);
  if(j.raw_response)out.push([path,escapeHtml(j.raw_response)]);
  for(const s of (app.spend??[]).filter(s=>s.work_id===j.id)){
   out.push([path,`id="usage-${s.id}"`]);
   if(s.raw_response)out.push([path,raw(s.raw_response)]);
  }
 }
 return out;
}
// The stored messages of saved checks, which no page shows.
export const storedCheckMessages=app=>Object.values(app.assessments).map(a=>a.error).filter(Boolean).map(escapeHtml);
export const hrefs=html=>[...html.matchAll(/href="([^"]*)"/g)].map(match=>match[1].replace(/&amp;/g,'&'));
// A link the crawl opens: a page of this app, not a file, export, API or query.
export const followable=href=>href.startsWith('/')&&!href.startsWith('//')&&!href.includes('?')&&!/^\/(assets|photos|api)\//.test(href)&&!['/export','/changelog'].includes(href.split('#')[0]);
// Follows every link from `start` breadth first; `get(path)` returns a page's
// HTML as served. Returns the pages by path and every link seen.
export async function crawl(get,start){
 const pages=new Map(),links=new Set(start),queue=[...start];
 while(queue.length){
  const path=queue.shift().split('#')[0];
  if(!followable(path)||pages.has(path))continue;
  const html=await get(path);pages.set(path,html);
  for(const link of hrefs(html))if(!links.has(link)){links.add(link);queue.push(link);}
 }
 return {pages,links};
}
export const menuLinks=html=>hrefs(/aria-label="Main navigation">([\s\S]*?)<\/nav>/.exec(html)[1]);
// Every expected fragment is on its page, and the crawl reached that page.
export function assertReachable({pages},app,where){
 const missing=[];
 for(const [path,fragment] of expectedOnPages(app)){
  const found=path===null?[...pages.values()].some(html=>html.includes(fragment)):pages.get(path)?.includes(fragment);
  if(!found)missing.push(`${path??'any page'}: ${pages.has(path)||path===null?fragment.slice(0,80):'page not reached'}`);
 }
 assert.deepEqual(missing,[],`${where}: not reachable from the Menu`);
 for(const message of storedCheckMessages(app))assert.ok(![...pages.values()].some(html=>html.includes(message)),`${where}: a check's stored message is shown`);
 // Every earlier page is reached, and none is a control or polls.
 const earlier=[...pages.keys()].filter(path=>path==='/notebook'||path==='/map'||/^\/(goals|concepts)\/[^/]+$|^\/questions\/[^/]+\/edit$/.test(path));
 assert.equal(earlier.length,2+Object.keys(app.goals).length+Object.keys(app.concepts).length+Object.keys(app.questions).length,`${where}: every earlier page is reached`);
 for(const path of earlier){const html=pages.get(path),main=html.slice(html.indexOf('<main'),html.indexOf('</main>'));assert.ok(!/<form|<input|<button|<textarea|data-poll/.test(main),`${where}: ${path} holds a control or a poll`);}
 return earlier;
}
