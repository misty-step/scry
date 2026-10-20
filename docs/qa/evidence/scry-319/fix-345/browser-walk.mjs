import {createRequire} from 'node:module';
import {mkdir, writeFile, readFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {mixedBrowserWalk} from './source/qa/mixed-notebook.mjs';
import {recoveryWalk} from './source/qa/browser-recovery.mjs';
const require=createRequire(new URL('./source/package.json',import.meta.url));
const {chromium}=require('playwright');
const browser=await chromium.launch({headless:true,args:['--no-sandbox']});
const screens=new URL('./screens/',import.meta.url);
await mkdir(screens,{recursive:true});
const capture=async(page,name)=>{await page.screenshot({path:new URL(name,screens).pathname,fullPage:true});return name;};
let results;
try {
 results={source:JSON.parse(await readFile(new URL('./source-meta.json',import.meta.url))),wasm_sha256:createHash('sha256').update(await readFile(new URL('./source/build/index_bg.wasm',import.meta.url))).digest('hex'),mixed:await mixedBrowserWalk(browser,capture),legacy:await recoveryWalk(browser,capture)};
} finally {await browser.close();}
await writeFile(new URL('./browser-results.json',import.meta.url),JSON.stringify(results,null,2)+'\n');
console.log(JSON.stringify(results));
if([...results.mixed,...results.legacy].some(x=>x.status!=='pass'))process.exit(1);
