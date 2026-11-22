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
 const page=await browser.newPage(); const errors=[];let speech=0,explains=0;
 page.on("console",m=>{if(m.type()==="error")errors.push("console: "+m.text());});
 page.on("response",r=>{if(r.status()>=400)errors.push("http "+r.status()+" "+r.url());});
 page.on("pageerror",e=>errors.push(e.message));
 await page.setRequestInterception(true);
 page.on("request",r=>{
  if(r.url().startsWith(base+"/")){if(r.url().endsWith("/study/speech"))speech++;if(r.url().endsWith("/study/explain"))explains++;r.continue();}
  else if(r.url().startsWith("data:"))r.continue();
  else {errors.push("external request: "+r.url());r.abort();}
 });
 await page.setViewport({width:1280,height:1000});
 const go=async path=>{const r=await page.goto(base+path,{waitUntil:"networkidle0"});assert.equal(r.status(),200);};
 const submit=async selector=>Promise.all([page.waitForNavigation({waitUntil:"networkidle0"}),page.click(selector)]);
 // Slice 36: sign-up starts the 7-day no-card trial and lands straight on
 // the starter shelf, with the trial chip in the chrome; no card step.
 const onTrialShelf=async(p,label)=>{
  assert.equal(new URL(p.url()).pathname,"/shelf",label+": sign-up must land on /shelf, not the card step");
  const chip=await p.$eval("[data-trial]",e=>({text:e.textContent.trim(),href:e.getAttribute("href"),shown:e.getClientRects().length>0}));
  assert(chip.shown&&chip.text.startsWith("Trial · 7 days left")&&chip.href==="/signup/card",label+": trial chip "+JSON.stringify(chip));
  assert.equal(await p.$("#number"),null,label+": no card fields on the shelf");
  await p.waitForSelector("#nato-alphabet");
 };
 await go("/signup");await page.type("#email","browser@example.com");await page.type("#password","browser-password");
 await submit("form button");
 await onTrialShelf(page,"signup");
 await page.screenshot({path:out+"/trial-shelf.png",fullPage:true});
 // Upgrading still works: the card step says nothing is charged today,
 // carries those terms in the form, and lands back on the shelf.
 await go("/signup/card");
 assert((await page.$eval(".trial-terms",e=>e.textContent)).includes("Nothing is charged today"));
 assert.equal(await page.$eval('input[name="billing_terms"]',e=>e.value),"trial");
 await page.type("#number","4242424242424242");await page.type("#expiry","12/30");await page.type("#cvc","123");
 await submit("form button");
 assert.equal(new URL(page.url()).pathname,"/shelf","card added: back to the shelf");
 assert.equal(await page.$("[data-trial]"),null,"no no-card trial chip once a card is on file");
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
 // D2: start and immediately stop: DESIGN's exact "couldn't hear" words.
 await page.keyboard.press("Escape");await page.keyboard.press("Space");await page.waitForSelector(".is-listening");
 await page.keyboard.press("Space");
 await page.waitForFunction(()=>!document.querySelector(".is-listening"));
 const uncaught=await page.$eval("[data-status]",e=>e.textContent);
 console.log("D2 immediate stop status:",JSON.stringify(uncaught));
 assert.equal(uncaught,"Didn't catch that. Try again or type it.");
 assert.equal(speech,1);assert.equal(await page.$eval("#answer",e=>e.value),"");
 await page.keyboard.press("Escape");await page.keyboard.press("Space");await page.waitForSelector(".is-listening");
 await pause(1700);assert.equal(speech,1,"must not upload while recording");
 assert.equal(await page.$eval("#answer",e=>e.value),"","D1: no live transcription");
 assert((await page.$eval("[data-status]",e=>e.textContent)).includes("listening"));
 await page.keyboard.press("Space");
 await page.waitForFunction(()=>!document.querySelector(".is-listening"));
 console.log("speech result",await page.$eval("[data-status]",e=>e.textContent),await page.$eval("#answer",e=>e.value),errors);
 assert((await page.$eval("[data-status]",e=>e.textContent)).includes("heard"));
 assert.equal(speech,2);assert((await page.$eval("#answer",e=>e.value)).length>0);
 await page.keyboard.press("Enter");await page.waitForSelector(".answered-why");
 const visible=await page.$eval(".answered-why",e=>{let s=getComputedStyle(e),r=e.getBoundingClientRect();return r.height>20&&r.width>40&&s.clipPath==="none";});
 assert(visible,"right reason must be visible body text");
 console.log("voice: one upload at stop; right reason visible");
 for(const width of [1280,390]){
 await page.setViewport({width,height:width===390?844:800});
 assert.equal(await page.$$eval(".answered-why",es=>es.length),1);
 await page.screenshot({path:out+"/d9-newest-right-"+width+".png",fullPage:true});
 }
 await page.setViewport({width:1280,height:800});
 await page.type("#answer","This deliberately incorrect answer wraps across several lines on a small phone.");
 await page.click("[data-check]");await page.waitForSelector(".struck-text");
 assert.equal(await page.$(".answered-why"),null,"D9: prior why clears when next answer arrives");
 for(const width of [1280,900,600,390])for(const theme of ["light","dark"]){
  await page.setViewport({width,height:width===390?844:800});
  await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);
  await page.evaluate(()=>document.fonts.ready);
  await pause(300);
  const pair=await page.$eval(".judgment-pair",e=>{const [a,b]=[...e.children].map(x=>x.getBoundingClientRect());return {left:a.left,otherLeft:b.left,bottom:a.bottom,top:b.top};});
  assert(Math.abs(pair.left-pair.otherLeft)<1 && pair.top>=pair.bottom+15,"D6: pair must stack at "+width+JSON.stringify(pair));
  assert.equal(await page.$eval(".struck-text",e=>getComputedStyle(e).textDecorationLine),"line-through");
  assert.equal(await page.$(".struck-line"),null);
  assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),JSON.stringify(await page.$$eval("*",es=>[innerWidth,document.documentElement.scrollWidth,...es.filter(e=>e.scrollWidth>e.clientWidth+1).map(e=>[e.className,e.clientWidth,e.scrollWidth])])));
  await page.screenshot({path:out+"/missed-"+width+"-"+theme+".png",fullPage:true});
 }
 // Ruling: the verdict is instant with the fixed criterion sentence and a
 // "Why?" control; nothing is explained until the learner asks.
 await page.setViewport({width:1280,height:800});
 assert.equal(explains,0,"no automatic explanation call");
 assert.equal(await page.$eval(".verdict .why",e=>e.textContent),"The expected answer is different.");
 const why=await page.$eval("[data-explain]",b=>({h:b.getBoundingClientRect().height,w:b.getBoundingClientRect().width,text:b.textContent}));
 assert(why.h>=44&&why.w>=44&&why.text.startsWith("Why?"),JSON.stringify(why));
 assert(await page.$eval("[data-explanation]",e=>e.hidden));
 await page.click("[data-explain]");
 await page.waitForFunction(()=>{const e=document.querySelector("[data-explanation]");return e&&!e.hidden&&e.textContent.length>0;});
 const explained=await page.$eval("[data-explanation]",e=>e.textContent);
 console.log("explanation:",explained);
 assert(explained.startsWith("What differed:"),explained);
 assert.equal(explains,1);
 assert.equal(await page.$eval(".verdict .tag",e=>e.textContent),"incorrect","verdict unchanged");
 assert.equal(await page.$eval(".verdict .why",e=>e.textContent),"The expected answer is different.");
 assert(await page.$eval("[data-explain-note]",e=>e.hidden),"not a fallback");
 for(const width of [1280,390])for(const theme of ["light","dark"]){
  await page.setViewport({width,height:width===390?844:800});
  await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);
  const m=await page.$eval("[data-explanation]",e=>{const s=getComputedStyle(e),r=e.getBoundingClientRect();return {w:r.width,right:r.right,vw:innerWidth,overflow:document.documentElement.scrollWidth>innerWidth,color:s.color};});
  assert(!m.overflow&&m.right<=m.vw+1,JSON.stringify(m));
  console.log("explanation",width,theme,m);
  await page.screenshot({path:out+"/explain-"+width+"-"+theme+".png",fullPage:true});
 }
 await page.setViewport({width:1280,height:800});
 // A short pair fits beside itself on desktop, but still stacks on phones.
 await page.$eval(".struck-text",e=>e.textContent="No");
 await page.$eval(".judgment-right",e=>e.textContent="Yes");
 for(const width of [1280,390]){
 await page.setViewport({width,height:width===390?844:800});
 const rects=await page.$eval(".judgment-pair",e=>[...e.children].map(x=>{const r=x.getBoundingClientRect();return {top:r.top,bottom:r.bottom,left:r.left};}));
 if(width===1280) assert(Math.abs(rects[0].top-rects[1].top)<1,"short pair should fit side by side");
 else assert(rects[1].top>=rects[0].bottom+15,"phones always stack");
 await page.screenshot({path:out+"/d6-short-"+width+".png",fullPage:true});
 }
 await page.evaluate(()=>document.activeElement.blur());
 await page.keyboard.press("Enter");
 await page.waitForSelector("[data-answer-form]");
 assert.equal(await page.$(".answered-why"),null,"D9: advancing a miss must not resurrect its why in scrollback");
 for(const width of [1280,390]){
 await page.setViewport({width,height:width===390?844:800});
 await page.screenshot({path:out+"/d9-receded-"+width+".png",fullPage:true});
 }
 for(const path of ["/library","/inbox","/billing","/settings"]){
  if(path==="/settings") { await go(path); } else {
 await page.keyboard.press("Escape");
 const key={"/library":"l","/inbox":"i","/billing":"b"}[path];
 await Promise.all([page.waitForNavigation({waitUntil:"networkidle0"}),(async()=>{await page.keyboard.press("g");await page.keyboard.press(key);})()]);
 assert.equal(new URL(page.url()).pathname,path);
 }
 if(path==="/library") {
 await page.keyboard.press("/");
 assert(await page.$eval("[data-search]",e=>e===document.activeElement));
 }
 if(path==="/billing"||path==="/settings") {
 const tooWide=await page.$$eval("main p",es=>es.filter(e=>e.getBoundingClientRect().width>parseFloat(getComputedStyle(e).maxWidth)+1).map(e=>e.className));
 assert.deepEqual(tooWide,[],"D8: reading paragraphs exceed measure");
 }
 const short=await page.$$eval("a.link",els=>els.filter(e=>e.getClientRects().length&&e.getBoundingClientRect().height<44).map(e=>e.textContent));
  assert.deepEqual(short,[],"short touch targets on "+path);
 }
 await page.click('input[name=theme][value=light]');await submit('form[action="/settings/theme"] button');
 assert.equal(await page.evaluate(()=>document.documentElement.dataset.theme),"light");
 // Complete the remaining primary navigation keys.
 for(const [key,path] of [["n","/learn"],["s","/study"]]){
 await page.keyboard.press("Escape");
 await Promise.all([page.waitForNavigation({waitUntil:"networkidle0"}),(async()=>{await page.keyboard.press("g");await page.keyboard.press(key);})()]);
 assert.equal(new URL(page.url()).pathname,path);
 }
 const context=await browser.createBrowserContext();
 const exact=await context.newPage();
 const exactSubmit=selector=>Promise.all([exact.waitForNavigation({waitUntil:"networkidle0"}),exact.click(selector)]);
 await exact.goto(base+"/signup",{waitUntil:"networkidle0"});
 await exact.type("#email","exact-browser@example.com");await exact.type("#password","browser-password");
 await exactSubmit("form button");
 await onTrialShelf(exact,"second signup"); // studies on the no-card trial
 await exact.goto(base+"/library/texts/new",{waitUntil:"networkidle0"});
 await exact.type("#title","Key exercise");await exact.type("#text","Remember this line.");
 await exactSubmit("form button");
 await exact.goto(base+"/study",{waitUntil:"networkidle0"});
 await exact.type("#answer","wrong");await exact.keyboard.press("Enter");await exact.waitForSelector("[data-again]");
 await exact.evaluate(()=>document.activeElement.blur());await exact.keyboard.press("a");
 await exact.waitForSelector("[data-answer-form]");
 assert((await exact.$eval(".exact-label",e=>e.textContent)).includes("again"));
 await exact.type("#answer","Remember this line.");await exact.keyboard.press("Enter");await exact.waitForSelector("[data-next]");
 await exact.evaluate(()=>document.activeElement.blur());await exact.keyboard.press("Enter");
 await exact.waitForSelector(".done");
 await context.close();
 assert.deepEqual(errors,[]);
 console.log("signup (no-card trial, shelf), card upgrade, trial study, typed/spoken study, missed, library, inbox, billing, settings: PASS");
 }finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
