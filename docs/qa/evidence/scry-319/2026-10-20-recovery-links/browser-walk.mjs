import {createRequire} from 'node:module';
import {writeFile, readFile, mkdir} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {legacyRecoveryLinksWalk} from './source/qa/legacy-recovery-links.mjs';
import {recoveryWalk} from './source/qa/browser-recovery.mjs';
const require = createRequire(new URL('./source/package.json', import.meta.url));
const {chromium} = require('playwright');
const browser = await chromium.launch({headless:true, args:['--no-sandbox']});
const folder = new URL('./screens/', import.meta.url);
await mkdir(folder, {recursive:true});
const capture = async (page, name) => {await page.screenshot({path:new URL(name, folder).pathname, fullPage:true}); return name;};
let report;
try {
  report = {source:JSON.parse(await readFile(new URL('./source-meta.json', import.meta.url))), wasm_sha256:createHash('sha256').update(await readFile(new URL('./source/build/index_bg.wasm', import.meta.url))).digest('hex'), recovery_links:await legacyRecoveryLinksWalk(browser, capture), no_study:await recoveryWalk(browser, capture)};
} finally {await browser.close();}
await writeFile(new URL('./browser-results.json', import.meta.url), JSON.stringify(report, null, 2) + '\n');
console.log(JSON.stringify(report));
if ([...report.recovery_links, ...report.no_study].some(row => row.status !== 'pass')) process.exit(1);
