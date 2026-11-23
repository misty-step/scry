#!/usr/bin/env node
// verify-slice54.mjs: check the Slice #54 Folio (brand mark, landing
// specimen, craft polish) on a running test-mode Scry, in a real headless
// Chromium (system Chrome), the way a learner meets it.
//
//   SCRY_MODE=test SCRY_SEED_DEMO=1 SCRY_TEST_JUDGMENTS=missed,close,right PORT=18095 ./bin/scry &
//   node scripts/verify-slice54.mjs --url http://127.0.0.1:18095 [--evidence ~/chief-evidence/media] [--sha 69de183]
//
// It signs in as the seeded demo learner and, at desktop 1440x900 and phone
// 390x844 (touch), asserts: the brand mark beside the wordmark, favicon,
// apple-touch icon and manifest; the two-column landing with its Study
// Specimen on the 22 px dot grid and the public footer; Study's 196 px
// top-aligned margin, a waiting screen with no vertical scroll, a review
// framed at the question with Next card focused (preventScroll) and the
// Ask Why explanation's 2 px rule; Create's three example prompts that fill
// the field and draft rows that do not repeat the idea; the Library outline
// in tabular numbers and, on the phone, the collapsible idea picker above the
// open idea; Settings applying a mode at once and saving rapid switches one
// POST at a time with the last choice persisted; the phone's single-row
// header and navigation; and 0 console errors, page errors, failed requests,
// CSP violations and HTTP >= 500 everywhere. With --evidence it writes the
// screenshots and one walkthrough video per viewport. Prints a JSON summary;
// exits 0 only when every check passes.

import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { chromium } from "playwright-core";

const args = process.argv.slice(2);
const opt = { url: "", evidence: "", sha: "" };
for (let i = 0; i < args.length; i++) {
  if (args[i] === "--url") opt.url = args[++i];
  else if (args[i] === "--evidence") opt.evidence = path.resolve(args[++i]);
  else if (args[i] === "--sha") opt.sha = args[++i];
  else { console.error("usage: verify-slice54.mjs --url <base-url> [--evidence dir] [--sha short-sha]"); process.exit(2); }
}
if (!opt.url) { console.error("--url is required"); process.exit(2); }
const base = opt.url.replace(/\/+$/, "");
if (opt.evidence) fs.mkdirSync(opt.evidence, { recursive: true });

const summary = { ok: false, url: base, startedAt: new Date().toISOString(), checks: [], failures: [], problems: [], screenshots: [], videos: [] };
function check(name, ok, detail) {
  summary.checks.push(ok ? { name, ok } : { name, ok, detail });
  if (!ok) summary.failures.push(name + (detail === undefined ? "" : ": " + JSON.stringify(detail)));
  return ok;
}
const pause = ms => new Promise(r => setTimeout(r, ms));
const CONFIGS = [
  { name: "desktop", width: 1440, height: 900 },
  { name: "phone", width: 390, height: 844, mobile: true },
];
const DEMO = { email: "phaedrus@example.com", password: "know-thyself" };

function watch(page, label) {
  const push = s => summary.problems.push(label + " " + s);
  page.on("console", m => { if (m.type() === "error") push("console.error: " + m.text().slice(0, 200)); });
  page.on("pageerror", e => push("pageerror: " + String(e.message).slice(0, 200)));
  page.on("requestfailed", r => {
    const err = r.failure()?.errorText || "failed";
    if (err.includes("ERR_ABORTED")) return; // a navigation the page itself replaced
    push("request failed: " + r.method() + " " + new URL(r.url()).pathname + " " + err);
  });
  page.on("response", r => { if (r.status() >= 500) push("http " + r.status() + " " + r.request().method() + " " + new URL(r.url()).pathname); });
  page.on("request", r => { if (!r.url().startsWith(base) && !r.url().startsWith("data:") && !r.url().startsWith("blob:")) push("external request: " + r.url().slice(0, 120)); });
}

async function settle(page) {
  await page.evaluate(() => document.fonts.ready);
  await page.evaluate(() => Promise.race([
    Promise.all(document.getAnimations().filter(a => { const t = a.effect && a.effect.getComputedTiming(); return !t || Number.isFinite(t.endTime); }).map(a => a.finished.catch(() => {}))),
    new Promise(r => setTimeout(r, 2500)),
  ]));
  await page.waitForTimeout(250);
}

async function shot(page, name, full) {
  if (!opt.evidence) return;
  await settle(page);
  const file = path.join(opt.evidence, name);
  await page.screenshot({ path: file, fullPage: !!full });
  summary.screenshots.push(path.basename(file));
}

async function common(page, c, label) {
  const a = await page.evaluate(() => ({ overflow: document.documentElement.scrollWidth - innerWidth, csp: window.__csp || [] }));
  check(c.name + " " + label + ": 0 px sideways scroll", a.overflow <= 0, a.overflow);
  check(c.name + " " + label + ": 0 CSP violations", a.csp.length === 0, a.csp);
}

const press = (page, c, sel) => c.mobile ? page.locator(sel).first().tap() : page.locator(sel).first().click();

async function run(browser, c) {
  const videoDir = fs.mkdtempSync(path.join(os.tmpdir(), "scry54-video-"));
  const context = await browser.newContext({
    viewport: { width: c.width, height: c.height }, isMobile: !!c.mobile, hasTouch: !!c.mobile, deviceScaleFactor: c.mobile ? 2 : 1,
    colorScheme: "light", recordVideo: opt.evidence ? { dir: videoDir, size: { width: c.mobile ? 390 : 1280, height: c.mobile ? 844 : 800 } } : undefined,
  });
  context.setDefaultTimeout(20000);
  await context.addInitScript(() => {
    window.__csp = [];
    document.addEventListener("securitypolicyviolation", e => window.__csp.push((e.effectiveDirective || e.violatedDirective) + " " + e.blockedURI), true);
  });
  const page = await context.newPage();
  watch(page, c.name);

  // ---- Landing, brand, icons -------------------------------------------
  await page.goto(base + "/", { waitUntil: "load" });
  await settle(page);
  const land = await page.evaluate(() => {
    const q = s => document.querySelector(s), r = el => el && el.getBoundingClientRect();
    const mark = q(".wordmark svg"), text = q(".wordmark-text"), copy = q(".hero-copy"), spec = q(".specimen"), frame = q(".specimen-frame");
    const before = frame && getComputedStyle(frame, "::before");
    const top = q("header.top"), signin = [...document.querySelectorAll("header.top a")].find(a => /sign in/i.test(a.textContent));
    return {
      mark: !!mark, markW: r(mark)?.width, markH: r(mark)?.height, gap: mark && text ? Math.round(r(text).left - r(mark).right) : null,
      icon: q('link[rel=icon]')?.getAttribute("href"), apple: q('link[rel=apple-touch-icon]')?.getAttribute("href"), manifest: q('link[rel=manifest]')?.getAttribute("href"),
      headline: q(".hero h1")?.textContent.trim(), cta: q(".hero .btn-accent")?.textContent.trim(),
      side: copy && spec ? { copyRight: r(copy).right, specLeft: r(spec).left, copyBottom: r(copy).bottom, specTop: r(spec).top } : null,
      verdict: q(".specimen .verdict-badge")?.textContent.trim(), why: q(".specimen .why")?.textContent.trim(), schedule: !!q(".specimen-schedule"),
      grid: before ? (before.maskImage || before.webkitMaskImage || "") + " " + (before.maskSize || before.webkitMaskSize || "") : "",
      price: q(".trial-terms-line .nowrap") ? getComputedStyle(q(".trial-terms-line .nowrap")).whiteSpace + " " + q(".trial-terms-line .nowrap").textContent : "",
      foot: q("[data-public-foot]")?.textContent.trim() || "", topH: r(top)?.height,
      oneRow: signin && mark ? Math.abs((r(signin).top + r(signin).height / 2) - (r(mark).top + r(mark).height / 2)) < 12 : false,
    };
  });
  check(c.name + " landing: brand mark svg beside 'scry', 20px square, 8px gap", land.mark && Math.round(land.markW) === 20 && Math.round(land.markH) === 20 && land.gap === 8, land);
  check(c.name + " landing: favicon, apple-touch icon and manifest linked", land.icon === "/static/icon.svg" && land.apple === "/static/icon-180.png" && land.manifest === "/static/manifest.webmanifest", land);
  check(c.name + " landing: headline and one accent action", land.headline === "Keep what's worth knowing." && /Start the 7-day trial/.test(land.cta || ""), land);
  if (c.mobile) check("phone landing: hero stacks (specimen below copy)", land.side && land.side.specTop >= land.side.copyBottom - 1, land.side);
  else check("desktop landing: two columns, specimen right of copy", land.side && land.side.specLeft >= land.side.copyRight - 1 && land.side.specTop < land.side.copyBottom, land.side);
  check(c.name + " landing: Study Specimen shows a verdict in words, a reason and the schedule", /Correct|Almost Correct|Incorrect/.test(land.verdict || "") && !!land.why && land.schedule, land);
  check(c.name + " landing: specimen sits on the 22px dot grid", /grid\.svg/.test(land.grid) && /22px/.test(land.grid), land.grid);
  check(c.name + " landing: '$5 a month' unbroken", /^nowrap \$5 a month$/.test(land.price), land.price);
  check(c.name + " landing: public footer names Misty Step and privacy", /Misty Step/.test(land.foot) && /yours alone/.test(land.foot), land.foot);
  if (c.mobile) check("phone landing: signed-out header is one 56px row with Sign in", Math.round(land.topH) === 56 && land.oneRow, { h: land.topH, oneRow: land.oneRow });
  await common(page, c, "landing");
  await shot(page, c.name + "-landing.png", !c.mobile);
  if (c.mobile) {
    await page.locator(".specimen").scrollIntoViewIfNeeded();
    await shot(page, "phone-landing-specimen.png");
  }
  const assets = await page.evaluate(async () => {
    const out = {};
    for (const p of ["/static/icon.svg", "/static/icon-dark.svg", "/static/icon-180.png", "/static/icon-192.png", "/static/icon-512.png", "/static/manifest.webmanifest"]) {
      const r = await fetch(p); out[p] = r.status + " " + (r.headers.get("content-type") || "");
      if (p.endsWith("webmanifest")) { const m = await r.json(); out.manifest = { start_url: m.start_url, display: m.display, icons: (m.icons || []).map(i => i.src + " " + i.sizes) }; }
    }
    return out;
  });
  check(c.name + " icons and manifest are served", Object.entries(assets).filter(([k]) => k.startsWith("/")).every(([, v]) => v.startsWith("200")) && assets.manifest.start_url === "/study" && assets.manifest.display === "standalone", assets);

  // ---- Sign in (demo learner) ------------------------------------------
  await page.goto(base + "/signin", { waitUntil: "load" });
  await page.fill("#email", DEMO.email);
  await page.fill("#password", DEMO.password);
  const inputBorder = await page.$eval("#email", e => getComputedStyle(e).borderTopWidth + " " + getComputedStyle(e).borderTopStyle);
  check(c.name + " inputs: 1px hairline border at rest", inputBorder === "1px solid", inputBorder);
  await Promise.all([page.waitForURL(u => new URL(u).pathname !== "/signin"), press(page, c, "main form button.btn-accent")]);

  // ---- Header and nav --------------------------------------------------
  await page.goto(base + "/library", { waitUntil: "load" });
  const nav = await page.evaluate(() => {
    const links = [...document.querySelectorAll(".nav a")], n = document.querySelector(".nav");
    const tops = new Set(links.map(a => Math.round(a.getBoundingClientRect().top)));
    const cur = document.querySelector(".nav a[aria-current] .nav-label");
    return { labels: links.map(a => a.querySelector(".nav-label")?.textContent), rows: tops.size, h: n.getBoundingClientRect().height, scrolls: getComputedStyle(n).overflowX,
      underline: cur ? getComputedStyle(cur).textDecorationLine : "", linkUnderline: getComputedStyle(document.querySelector(".nav a[aria-current]")).textDecorationLine };
  });
  check(c.name + " nav: Study, Library, Inbox, Create, Billing, Settings in one row", nav.rows === 1 && nav.labels.join() === "Study,Library,Inbox,Create,Billing,Settings", nav);
  check(c.name + " nav: active underline on the label only", nav.underline === "underline" && nav.linkUnderline === "none", nav);
  if (c.mobile) check("phone nav: one 44px row that scrolls sideways", Math.round(nav.h) === 44 && nav.scrolls === "auto", nav);

  // ---- Library ---------------------------------------------------------
  const lib = await page.evaluate(() => ({ tabular: getComputedStyle(document.querySelector(".outline-num")).fontVariantNumeric, ideas: document.querySelectorAll(".pane-outline .outline-row").length }));
  check(c.name + " library: outline numbers are tabular", /tabular-nums/.test(lib.tabular) && lib.ideas > 1, lib);
  await common(page, c, "library");
  if (c.mobile) {
    const pick = await page.evaluate(() => {
      const p = document.querySelector("[data-idea-picker]"), idea = document.querySelector(".pane-idea"), out = document.querySelector(".pane-outline");
      return { visible: getComputedStyle(p).display !== "none", open: p.open, above: p.getBoundingClientRect().bottom <= idea.getBoundingClientRect().top + 1, outlineHidden: getComputedStyle(out).display === "none", ideaTop: idea.getBoundingClientRect().top };
    });
    check("phone library: collapsed idea picker above the open idea, full outline hidden", pick.visible && !pick.open && pick.above && pick.outlineHidden && pick.ideaTop < 844, pick);
    await shot(page, "phone-library.png");
    await press(page, c, ".picker-summary");
    await pause(300);
    await shot(page, "phone-library-picker-open.png");
    const target = page.locator("[data-idea-picker] .outline-row:not([aria-current])").nth(1);
    const title = (await target.locator(".outline-title").textContent()).trim();
    await Promise.all([page.waitForURL(/\/library\/ideas\//), target.tap()]);
    await settle(page);
    const landed = await page.evaluate(() => {
      const p = document.querySelector("[data-idea-picker]"), t = document.querySelector(".idea-title");
      return { open: p.open, title: t.textContent.trim(), titleTop: t.getBoundingClientRect().top, scrollY, current: document.querySelector(".picker-current").textContent.replace(/\s+/g, " ").trim() };
    });
    check("phone library: tapping an idea lands on it, title in view, picker collapsed above", landed.title === title && !landed.open && landed.titleTop > 0 && landed.titleTop < 400 && landed.current.includes(title), { title, landed });
    await shot(page, "phone-library-idea.png");
  } else {
    await shot(page, "desktop-library.png");
  }

  // ---- Create ----------------------------------------------------------
  await page.goto(base + "/learn", { waitUntil: "load" });
  const ex = await page.$$eval("[data-example]", as => as.map(a => ({ text: a.textContent.trim(), cls: a.className, border: getComputedStyle(a).borderTopWidth })));
  check(c.name + " create: three bordered example prompts under the field", ex.length === 3 && ex.every(e => e.border === "1px"), ex);
  const field = await page.$eval("#q", e => ({ fits: e.scrollWidth <= e.clientWidth + 1, size: getComputedStyle(e).fontSize }));
  if (c.mobile) {
    const ph = await page.$eval("#q", e => { const s = getComputedStyle(e), cv = document.createElement("canvas").getContext("2d"); cv.font = s.fontWeight + " " + s.fontSize + " " + s.fontFamily; return { w: cv.measureText(e.placeholder).width, box: e.clientWidth - parseFloat(s.paddingLeft) - parseFloat(s.paddingRight), size: s.fontSize }; });
    check("phone create: field scales so its placeholder fits", ph.w <= ph.box + 1, ph);
  }
  await common(page, c, "create");
  await shot(page, c.name + "-create.png");
  await press(page, c, "[data-example] >> nth=1");
  const filled = await page.$eval("#q", e => e.value);
  check(c.name + " create: tapping an example fills the field", filled === ex[1].text, { filled, want: ex[1].text });
  await shot(page, c.name + "-create-example-filled.png");
  await Promise.all([page.waitForURL(/\/learn\/\d+/), press(page, c, ".ask button.btn-accent")]);
  await page.waitForFunction(() => !document.querySelector("[data-drafting]") && document.querySelector(".draft-idea .sug"), null, { timeout: 30000 });
  await settle(page);
  const draft = await page.evaluate(() => ({ ideas: document.querySelectorAll(".draft-idea").length, rows: document.querySelectorAll(".draft-idea .sug").length, perRowIdea: document.querySelectorAll(".draft-idea .sug .sug-idea").length }));
  check(c.name + " create: draft outline rows do not repeat the idea heading", draft.ideas > 0 && draft.rows > 0 && draft.perRowIdea === 0, draft);
  await common(page, c, "draft");
  await shot(page, c.name + "-create-draft.png", !c.mobile);

  // ---- Study -----------------------------------------------------------
  await page.goto(base + "/study", { waitUntil: "load" });
  await page.waitForSelector("[data-answer-form]");
  await settle(page);
  const wait = await page.evaluate(() => {
    const m = document.querySelector(".session-margin"), f = document.querySelector(".session-flow"), main = document.querySelector("main.frame");
    const g = getComputedStyle(main, "::before");
    return { col: getComputedStyle(document.querySelector(".session")).gridTemplateColumns,
      topDelta: Math.round(Math.abs(m.getBoundingClientRect().top - f.getBoundingClientRect().top)), vscroll: document.documentElement.scrollHeight - innerHeight,
      grid: (g.maskImage || g.webkitMaskImage || "") + " " + (g.maskSize || g.webkitMaskSize || ""), header: document.querySelector("[data-study-header]")?.textContent.trim() };
  });
  if (!c.mobile) {
    check("desktop study: margin column is 196px", wait.col.startsWith("196px"), wait.col);
    check("desktop study: margin top-aligned with the flow", wait.topDelta <= 2, wait.topDelta);
    check("desktop study: waiting screen fits 1440x900 without vertical scroll", wait.vscroll <= 0, wait.vscroll);
  }
  check(c.name + " study: session sits on the 22px dot grid", /grid\.svg/.test(wait.grid) && /22px/.test(wait.grid), wait.grid);
  await common(page, c, "study waiting");
  await shot(page, c.name + "-study-waiting.png");
  await page.fill("#answer", "Able");
  await press(page, c, "[data-check]");
  await page.waitForSelector("[data-review]");
  await settle(page);
  const rev = await page.evaluate(() => {
    const p = document.querySelector("[data-prompt]").getBoundingClientRect(), v = document.querySelector("[data-verdict]")?.getBoundingClientRect();
    return { scrollY, promptTop: p.top, promptBottom: p.bottom, verdictTop: v?.top, focused: document.activeElement?.hasAttribute("data-next"), label: document.querySelector("[data-verdict-label]")?.textContent.trim(), topBar: document.querySelector("header.top").getBoundingClientRect().top };
  });
  check(c.name + " review: question and judgment in view, Next card focused without scrolling", rev.focused && rev.promptTop >= 0 && rev.promptBottom <= c.height && rev.verdictTop < c.height && rev.topBar >= 0 && rev.scrollY === 0, rev);
  summary.reviewVerdicts = Object.assign(summary.reviewVerdicts || {}, { [c.name]: rev.label });
  check(c.name + " review: verdict in words", /^(Correct|Almost Correct|Incorrect)$/.test(rev.label || ""), rev.label);
  await common(page, c, "review");
  await shot(page, c.name + "-study-review.png");
  if (await page.$("[data-explain]")) {
    await press(page, c, "[data-explain]");
    await page.waitForSelector(".explanation:not([hidden])");
    await settle(page);
    const why = await page.$eval(".explanation:not([hidden])", e => ({ rule: getComputedStyle(e).borderLeftWidth + " " + getComputedStyle(e).borderLeftStyle, text: e.textContent.trim().slice(0, 80) }));
    check(c.name + " review: Ask Why explanation set off by a 2px left rule", why.rule === "2px solid" && why.text.length > 0, why);
    await shot(page, c.name + "-study-review-explained.png", !c.mobile);
  } else check(c.name + " review: Ask Why offered", false, "no [data-explain]");
  // Opening the review afresh (a new navigation, so the browser's own scroll
  // restoration after clicking Ask Why further down does not apply).
  summary.scrollBeforeReopen = Object.assign(summary.scrollBeforeReopen || {}, { [c.name]: await page.evaluate(() => scrollY) });
  await page.goto(base + "/study", { waitUntil: "load" });
  await settle(page);
  const re = await page.evaluate(() => ({ scrollY, focused: document.activeElement?.hasAttribute("data-next"), promptTop: document.querySelector("[data-prompt]")?.getBoundingClientRect().top }));
  check(c.name + " review: a freshly opened review stays framed at the question, Next card focused", re.focused && re.scrollY === 0 && re.promptTop >= 0, re);
  await press(page, c, "[data-next]");
  await page.waitForSelector("[data-answer-form]");

  // ---- Settings: instant apply + serialized save -----------------------
  await page.goto(base + "/settings", { waitUntil: "load" });
  const btn = await page.evaluate(() => { const b = document.querySelector(".setting .form > .btn"), f = b.closest(".form"); return { b: Math.round(b.getBoundingClientRect().width), f: Math.round(f.getBoundingClientRect().width) }; });
  if (!c.mobile) check("desktop settings: section buttons content-sized", btn.b < btn.f, btn);
  let inFlight = 0, maxInFlight = 0; const bodies = [];
  await page.route("**/settings/theme", async route => {
    inFlight++; maxInFlight = Math.max(maxInFlight, inFlight);
    bodies.push(new URLSearchParams(route.request().postData() || "").get("theme"));
    await pause(600); // a slow network, so later switches land while a save is in flight
    try { await route.continue(); } finally { inFlight--; }
  });
  const radio = v => "input[name=theme][value=" + v + "]";
  await press(page, c, radio("dark"));
  const applied = await page.evaluate(() => document.documentElement.getAttribute("data-theme"));
  check(c.name + " settings: dark applies immediately", applied === "dark", applied);
  await shot(page, c.name + "-settings-dark-applied.png");
  // Real taps, as fast as a learner can switch: five changes in a row.
  for (const v of ["light", "system", "light", "dark"]) { await press(page, c, radio(v)); await pause(60); }
  const final = await page.evaluate(() => document.documentElement.getAttribute("data-theme"));
  const idle = async () => { for (let i = 0; i < 80 && inFlight > 0; i++) await pause(100); await pause(800); for (let i = 0; i < 80 && inFlight > 0; i++) await pause(100); };
  await idle();
  const tapped = bodies.splice(0);
  check(c.name + " settings: rapid tapped switches save one POST at a time (never two in flight)", maxInFlight === 1, { maxInFlight, tapped });
  check(c.name + " settings: tapped switches coalesce (fewer POSTs than changes) and the last POST carries the latest choice", tapped.length >= 2 && tapped.length < 5 && tapped[tapped.length - 1] === "dark" && final === "dark", { tapped, final });
  // A burst inside one save: every change lands while the first POST is in
  // flight, so exactly one follow-up goes out, carrying the last choice.
  maxInFlight = 0;
  await page.evaluate(() => { for (const v of ["light", "system", "dark", "light"]) document.querySelector("input[name=theme][value=" + v + "]").click(); });
  const burstApplied = await page.evaluate(() => document.documentElement.getAttribute("data-theme"));
  await idle();
  const burst = bodies.splice(0);
  check(c.name + " settings: a burst of 4 switches during one save sends exactly 2 POSTs, serialized, the last = latest", maxInFlight === 1 && burst.length === 2 && burst[0] === "light" && burst[1] === "light" && burstApplied === "light", { maxInFlight, burst, burstApplied });
  // Back to dark with one tap, for the persisted check and the dark picture.
  await press(page, c, radio("dark"));
  await idle();
  bodies.splice(0);
  await page.unroute("**/settings/theme");
  await page.reload({ waitUntil: "load" });
  const persisted = await page.evaluate(() => ({ checked: document.querySelector("input[name=theme]:checked")?.value, attr: document.documentElement.getAttribute("data-theme") }));
  check(c.name + " settings: latest mode persisted after reload", persisted.checked === "dark" && persisted.attr === "dark", persisted);
  await common(page, c, "settings");
  await shot(page, c.name + "-settings-dark.png", !c.mobile);
  // Restore "Follow my system" (one switch, saved) so later runs start clean.
  await press(page, c, radio("system"));
  for (let i = 0; i < 30; i++) { await pause(100); }
  await page.reload({ waitUntil: "load" });
  const restored = await page.evaluate(() => document.querySelector("input[name=theme]:checked")?.value);
  check(c.name + " settings: single switch back to system saved", restored === "system", restored);
  await shot(page, c.name + "-settings.png", !c.mobile);

  const video = page.video();
  await context.close();
  if (video && opt.evidence) {
    const dest = path.join(opt.evidence, "walkthrough-" + c.name + "-" + c.width + "x" + c.height + ".webm");
    await video.saveAs(dest);
    summary.videos.push(path.basename(dest));
  }
  fs.rmSync(videoDir, { recursive: true, force: true });
}

(async () => {
  const health = await fetch(base + "/healthz").then(async r => ({ status: r.status, body: await r.json() }));
  check("healthz 200, test mode" + (opt.sha ? ", build " + opt.sha : ""), health.status === 200 && health.body.mode === "test" && health.body.status === "ok" && (!opt.sha || health.body.build_sha === opt.sha), { status: health.status, build_sha: health.body.build_sha, mode: health.body.mode });
  summary.healthz = { status: health.status, build_sha: health.body.build_sha, mode: health.body.mode, database: health.body.database };
  if (health.body.mode !== "test") { console.log(JSON.stringify(summary, null, 2)); process.exit(1); }
  const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN || "/usr/bin/google-chrome", headless: true, args: ["--no-sandbox"] });
  try {
    for (const c of CONFIGS) {
      try { await run(browser, c); } catch (err) { check(c.name + " walkthrough ran to the end", false, String(err.stack || err).split("\n").slice(0, 3).join(" | ")); }
    }
  } finally { await browser.close(); }
  check("0 console errors, page errors, failed or external requests, HTTP >= 500", summary.problems.length === 0, summary.problems);
  summary.ok = summary.failures.length === 0;
  summary.passed = summary.checks.filter(c => c.ok).length;
  summary.total = summary.checks.length;
  summary.finishedAt = new Date().toISOString();
  console.log(JSON.stringify(summary, null, 2));
  process.exit(summary.ok ? 0 : 1);
})();
