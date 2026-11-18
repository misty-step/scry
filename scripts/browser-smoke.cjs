// Optional repeatable Chromium smoke; only the local Go test server is allowed.
const puppeteer = require("puppeteer-core");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const base = process.argv[2];
const out = process.env.SCRY_SMOKE_DIR || "/tmp/scry-remediation-smoke";
const pause = ms => new Promise(r => setTimeout(r,ms));
(async()=>{
 fs.mkdirSync(out,{recursive:true});
 const browser=await puppeteer.launch({executablePath:process.env.CHROME_BIN||"/usr/bin/google-chrome",headless:true,
 args:["--no-sandbox","--use-fake-ui-for-media-stream","--use-fake-device-for-media-stream","--autoplay-policy=no-user-gesture-required"]});
 try {
 const page=await browser.newPage(); const errors=[];let speech=0;
 page.on("pageerror",e=>errors.push(e.message));
 await page.setRequestInterception(true);
 page.on("request",r=>{
  if(r.url().startsWith(base+"/")){if(r.url().endsWith("/study/speech"))speech++;r.continue();}
  else if(r.url().startsWith("data:"))r.continue();
  else {errors.push("external request: "+r.url());r.abort();}
 });
 await page.setViewport({width:1280,height:1000});
 const go=async path=>{const r=await page.goto(base+path,{waitUntil:"networkidle0"});assert.equal(r.status(),200);};
 const submit=async selector=>Promise.all([page.waitForNavigation({waitUntil:"networkidle0"}),page.click(selector)]);
 await go("/signup");await page.type("#email","browser@example.com");await page.type("#password","browser-password");
 await submit("form button");
 await page.type("#number","4242424242424242");await page.type("#expiry","12/30");await page.type("#cvc","123");
 await submit("form button");
 await page.waitForSelector("#nato-alphabet");await submit("#nato-alphabet button");
 await go("/study");
 for(const width of [1280,390])for(const theme of ["light","dark"]){
  await page.setViewport({width,height:1000});
  await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);
  const metrics=await page.$eval("[data-check]",b=>{
   const s=getComputedStyle(b);
   const lum=color=>{const v=color.match(/[\d.]+/g).slice(0,3).map(Number).map(x=>{x/=255;return x<=.04045?x/12.92:((x+.055)/1.055)**2.4;});return .2126*v[0]+.7152*v[1]+.0722*v[2];};
   const a=lum(s.color),c=lum(s.backgroundColor);
   return {contrast:(Math.max(a,c)+.05)/(Math.min(a,c)+.05),height:b.getBoundingClientRect().height,overflow:document.documentElement.scrollWidth>innerWidth};
  });
  assert(metrics.contrast>=4.5,JSON.stringify(metrics));assert(metrics.height>=44);assert(!metrics.overflow);
  console.log("Check",width,theme,metrics);
  await page.screenshot({path:out+"/study-"+width+"-"+theme+".png",fullPage:true});
 }
 await page.setViewport({width:1280,height:1000});
 await page.click("[data-mic]");await page.waitForSelector(".is-listening");
 await pause(1700);assert.equal(speech,0,"must not upload while recording");
 await page.click("[data-mic]");
 await page.waitForFunction(()=>!document.querySelector(".is-listening"));
 console.log("speech result",await page.$eval("[data-status]",e=>e.textContent),await page.$eval("#answer",e=>e.value),errors);
 assert((await page.$eval("[data-status]",e=>e.textContent)).includes("heard"));
 assert.equal(speech,1);assert((await page.$eval("#answer",e=>e.value)).length>0);
 await page.click("[data-check]");await page.waitForSelector(".answered-why");
 const visible=await page.$eval(".answered-why",e=>{let s=getComputedStyle(e),r=e.getBoundingClientRect();return r.height>20&&r.width>40&&s.clipPath==="none";});
 assert(visible,"right reason must be visible body text");
 console.log("voice: one upload at stop; right reason visible");
 await page.type("#answer","This deliberately incorrect answer wraps across several lines on a small phone.");
 await page.click("[data-check]");await page.waitForSelector(".struck-text");
 for(const width of [1280,390])for(const theme of ["light","dark"]){
  await page.setViewport({width,height:1000});
  await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);
  assert.equal(await page.$eval(".struck-text",e=>getComputedStyle(e).textDecorationLine),"line-through");
  assert.equal(await page.$(".struck-line"),null);
  assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
  await page.screenshot({path:out+"/missed-"+width+"-"+theme+".png",fullPage:true});
 }
 for(const path of ["/library","/inbox","/billing","/settings"]){
  await go(path);
  const short=await page.$$eval("a.link",els=>els.filter(e=>e.getClientRects().length&&e.getBoundingClientRect().height<44).map(e=>e.textContent));
  assert.deepEqual(short,[],"short touch targets on "+path);
 }
 await page.click('input[name=theme][value=light]');await submit('form[action="/settings/theme"] button');
 assert.equal(await page.evaluate(()=>document.documentElement.dataset.theme),"light");
 assert.deepEqual(errors,[]);
 console.log("signup, trial, shelf, typed/spoken study, missed, library, inbox, billing, settings: PASS");
 }finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
