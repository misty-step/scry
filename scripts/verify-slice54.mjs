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
// CSP violations and HTTP >= 500 everywhere.
//
// Pass 3 adds: a theme change still waiting behind an in-flight save is kept
// when the learner navigates away at once (keepalive flush, sent once); the
// review without JavaScript stays at scrollY 0 for a sentence-length
// Incorrect answer; "/" on the phone Library opens the idea picker and
// focuses its search; a sentence typed on the phone Study screen never
// widens the page. Theme-save checks are gated on request events (a held
// response released on cue), not on fixed waits.
//
// Pass 4 adds: a loopback reverse proxy holds the Dark theme POST before it
// reaches Scry, the Light keepalive flush reaches Scry first, the held Dark
// arrives second, and Settings still shows Light (theme_seq); a plain form
// submit still applies after scripted saves; at 390x844 touch, a fresh
// learner's ~450-character question scrolls the waiting page when the answer
// is focused, and tapping Check opens its review at scrollY 0 with the
// question top on screen and Next/Finish focused, while Ask Why after a
// manual scroll keeps the learner's place. All are gated on request or DOM
// events.
//
// Pass 5 adds: theme_seq counts from the server (data-theme-seq), never from
// a clock. Context 1 saves Dark; a fresh context 2 whose browser clock is 3
// minutes behind picks Light, and Light persists. A JS Dark POST held before
// Scry is released after an explicit no-JS Light Save (which advances the
// sequence by 1000), and Light persists. With --evidence the verifier writes
// its own summary (walkthrough-checks.log.txt) into the bundle and checks the
// whole bundle is <= 1,750,000 bytes.
//
// Signing in happens in an unrecorded setup context; the recorded
// walkthrough reuses its storageState, so the test-mode demo hint on the
// sign-in page is never in a video. With --evidence it writes a compact set
// of screenshots and one walkthrough video per viewport (re-encoded with
// ffmpeg VP9 when available). Prints a JSON summary; exits 0 only when every
// check passes.

import fs from "node:fs";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { chromium } from "playwright-core";

const args = process.argv.slice(2);
const opt = { url: "", evidence: "", sha: "", serverLog: "", only: "" };
for (let i = 0; i < args.length; i++) {
  if (args[i] === "--url") opt.url = args[++i];
  else if (args[i] === "--server-log") opt.serverLog = path.resolve(args[++i]);
  else if (args[i] === "--evidence") opt.evidence = path.resolve(args[++i]);
  else if (args[i] === "--sha") opt.sha = args[++i];
  else if (args[i] === "--only") opt.only = args[++i]; // "pass4" / "pass5": only those checks (development, regression runs)
  else { console.error("usage: verify-slice54.mjs --url <base-url> [--evidence dir] [--sha short-sha] [--server-log scry.log] [--only pass4|pass5]"); process.exit(2); }
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
const CONFIGS = [
  { name: "desktop", width: 1440, height: 900 },
  { name: "phone", width: 390, height: 844, mobile: true },
];
// With --evidence the verifier writes its own JSON summary into the bundle
// and keeps the whole bundle within MEDIA_BUDGET bytes.
const CHECKS_LOG = "walkthrough-checks.log.txt";
const MEDIA_BUDGET = 1750000;
const DEMO = { email: "phaedrus@example.com", password: "know-thyself" };
const LONG_ANSWER = "I think it is the word that pilots use for this letter of the alphabet";

// origin: another origin that is Scry too (the Pass 4 loopback proxy).
function watch(page, label, origin) {
  const push = s => summary.problems.push(label + " " + s);
  page.on("console", m => { if (m.type() === "error") push("console.error: " + m.text().slice(0, 200)); });
  page.on("pageerror", e => push("pageerror: " + String(e.message).slice(0, 200)));
  page.on("requestfailed", r => {
    const err = r.failure()?.errorText || "failed";
    if (err.includes("ERR_ABORTED")) return; // a navigation the page itself replaced
    push("request failed: " + r.method() + " " + new URL(r.url()).pathname + " " + err);
  });
  page.on("response", r => { if (r.status() >= 500) push("http " + r.status() + " " + r.request().method() + " " + new URL(r.url()).pathname); });
  page.on("request", r => { if (!r.url().startsWith(base) && !(origin && r.url().startsWith(origin + "/")) && !r.url().startsWith("data:") && !r.url().startsWith("blob:")) push("external request: " + r.url().slice(0, 120)); });
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

const contextOptions = c => ({
  viewport: { width: c.width, height: c.height }, isMobile: !!c.mobile, hasTouch: !!c.mobile, deviceScaleFactor: c.mobile ? 2 : 1,
  colorScheme: "light",
});
async function instrument(context) {
  context.setDefaultTimeout(20000);
  await context.addInitScript(() => {
    window.__csp = [];
    document.addEventListener("securitypolicyviolation", e => window.__csp.push((e.effectiveDirective || e.violatedDirective) + " " + e.blockedURI), true);
  });
}

// Signed-out landing and sign-in, in a context that is never recorded.
// Returns the signed-in storageState for the recorded walkthrough.
async function setup(browser, c) {
  const context = await browser.newContext(contextOptions(c));
  await instrument(context);
  const page = await context.newPage();
  watch(page, c.name + " setup");

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
  if (!c.mobile) await shot(page, "desktop-landing.png");
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
  const state = await context.storageState();
  await context.close();
  return state;
}

// themePosts counts the POST /settings/theme requests in the server's
// structured request log (--server-log). A keepalive request sent while the
// page unloads is not seen by Playwright's routing, so the server's own log
// is the witness that it was sent, and sent once.
function themePosts() {
  if (!opt.serverLog) return -1;
  return fs.readFileSync(opt.serverLog, "utf8").split("\n").filter(l => l.includes('"method":"POST","path":"/settings/theme"')).length;
}
async function untilPolled(pred, ms = 8000) {
  const end = Date.now() + ms;
  for (;;) {
    if (await pred()) return true;
    if (Date.now() > end) return false;
    await new Promise(r => setTimeout(r, 50));
  }
}

// waiter resolves pred-gated promises whenever notify() is called.
function waiter() {
  const list = [];
  return {
    notify() { for (const w of list.splice(0)) { if (w.pred()) w.resolve(); else list.push(w); } },
    until(pred, label, ms = 10000) {
      if (pred()) return Promise.resolve();
      return new Promise((resolve, reject) => {
        const t = setTimeout(() => reject(new Error("timed out waiting for " + label)), ms);
        list.push({ pred, resolve: () => { clearTimeout(t); resolve(); } });
      });
    },
  };
}

async function run(browser, c, state) {
  const videoDir = fs.mkdtempSync(path.join(os.tmpdir(), "scry54-video-"));
  const context = await browser.newContext(Object.assign(contextOptions(c), {
    storageState: state,
    recordVideo: opt.evidence ? { dir: videoDir, size: { width: c.mobile ? 390 : 1280, height: c.mobile ? 844 : 800 } } : undefined,
  }));
  await instrument(context);
  const page = await context.newPage();
  watch(page, c.name);

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
  if (!c.mobile) {
    // "/" focuses the outline's search on desktop.
    await page.evaluate(() => document.activeElement && document.activeElement.blur());
    await page.keyboard.press("/");
    const slash = await page.evaluate(() => document.activeElement?.id || document.activeElement?.tagName);
    check("desktop library: '/' focuses the outline search", slash === "q", slash);
    await page.keyboard.press("Escape");
  }
  if (c.mobile) {
    const pick = await page.evaluate(() => {
      const p = document.querySelector("[data-idea-picker]"), idea = document.querySelector(".pane-idea"), out = document.querySelector(".pane-outline");
      return { visible: getComputedStyle(p).display !== "none", open: p.open, above: p.getBoundingClientRect().bottom <= idea.getBoundingClientRect().top + 1, outlineHidden: getComputedStyle(out).display === "none", ideaTop: idea.getBoundingClientRect().top };
    });
    check("phone library: collapsed idea picker above the open idea, full outline hidden", pick.visible && !pick.open && pick.above && pick.outlineHidden && pick.ideaTop < 844, pick);
    await press(page, c, ".picker-summary");
    await page.waitForFunction(() => document.querySelector("[data-idea-picker]").open);
    const target = page.locator("[data-idea-picker] .outline-row:not([aria-current])").nth(1);
    const title = (await target.locator(".outline-title").textContent()).trim();
    await Promise.all([page.waitForURL(/\/library\/ideas\//), target.tap()]);
    await settle(page);
    const landed = await page.evaluate(() => {
      const p = document.querySelector("[data-idea-picker]"), t = document.querySelector(".idea-title");
      return { open: p.open, title: t.textContent.trim(), titleTop: t.getBoundingClientRect().top, scrollY, current: document.querySelector(".picker-current").textContent.replace(/\s+/g, " ").trim() };
    });
    check("phone library: tapping an idea lands on it, title in view, picker collapsed above", landed.title === title && !landed.open && landed.titleTop > 0 && landed.titleTop < 400 && landed.current.includes(title), { title, landed });
    // "/" with nothing focused: the outline (and its search) is hidden on a
    // phone, so the picker opens and its own search takes focus.
    await page.evaluate(() => document.activeElement && document.activeElement.blur && document.activeElement.blur());
    const before = await page.evaluate(() => ({ active: document.activeElement?.tagName, desktopSearchHidden: document.querySelector("[data-search]").offsetParent === null }));
    await page.keyboard.press("/");
    await page.waitForFunction(() => document.querySelector("[data-idea-picker]").open, null, { timeout: 3000 }).catch(() => {});
    const slash = await page.evaluate(() => {
      const p = document.querySelector("[data-idea-picker]"), a = document.activeElement, r = a.getBoundingClientRect();
      return { open: p.open, active: a.id || a.tagName, inView: r.top >= 0 && r.bottom <= innerHeight && r.width > 0 };
    });
    check("phone library: '/' opens the idea picker and focuses its search", before.desktopSearchHidden && before.active === "BODY" && slash.open && slash.active === "q-picker" && slash.inView, { before, slash });
    await common(page, c, "library picker");
    await shot(page, "phone-library-slash-picker.png");
    await page.keyboard.press("Escape");
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
  await press(page, c, "[data-example] >> nth=1");
  const filled = await page.$eval("#q", e => e.value);
  check(c.name + " create: tapping an example fills the field", filled === ex[1].text, { filled, want: ex[1].text });
  if (!c.mobile) await shot(page, "desktop-create-example-filled.png");
  await Promise.all([page.waitForURL(/\/learn\/\d+/), press(page, c, ".ask button.btn-accent")]);
  await page.waitForFunction(() => !document.querySelector("[data-drafting]") && document.querySelector(".draft-idea .sug"), null, { timeout: 30000 });
  await settle(page);
  const draft = await page.evaluate(() => ({ ideas: document.querySelectorAll(".draft-idea").length, rows: document.querySelectorAll(".draft-idea .sug").length, perRowIdea: document.querySelectorAll(".draft-idea .sug .sug-idea").length }));
  check(c.name + " create: draft outline rows do not repeat the idea heading", draft.ideas > 0 && draft.rows > 0 && draft.perRowIdea === 0, draft);
  await common(page, c, "draft");

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
  if (c.mobile) {
    // A full sentence typed on a phone never widens the page or pushes
    // Check off screen.
    await page.locator("#answer").pressSequentially(LONG_ANSWER);
    const wide = await page.evaluate(() => {
      const b = document.querySelector("[data-check]").getBoundingClientRect();
      return { scrollWidth: document.documentElement.scrollWidth, innerWidth, checkLeft: Math.round(b.left), checkRight: Math.round(b.right) };
    });
    check("phone study: a sentence-length answer keeps the page 390px wide, Check on screen", wide.scrollWidth === 390 && wide.innerWidth === 390 && wide.checkRight <= 390 && wide.checkLeft >= 0, wide);
    await shot(page, "phone-study-long-answer.png");
    await page.fill("#answer", "");
  }
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
  if (!c.mobile) await shot(page, "desktop-study-review.png");
  if (await page.$("[data-explain]")) {
    await press(page, c, "[data-explain]");
    await page.waitForSelector(".explanation:not([hidden])");
    await settle(page);
    const why = await page.$eval(".explanation:not([hidden])", e => ({ rule: getComputedStyle(e).borderLeftWidth + " " + getComputedStyle(e).borderLeftStyle, text: e.textContent.trim().slice(0, 80) }));
    check(c.name + " review: Ask Why explanation set off by a 2px left rule", why.rule === "2px solid" && why.text.length > 0, why);
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
  // Every theme POST passes through here. The server handles it at once
  // (route.fetch); its response is handed back to the page only when the
  // test releases the gate, so "a save is in flight" is a fact, not a timing.
  const t = { sent: [], done: [], inFlight: 0, maxInFlight: 0, gate: null, onAccepted: null };
  const w = waiter();
  const hold = () => {
    let release, accepted;
    t.gate = new Promise(r => { release = r; });
    const acceptedP = new Promise(r => { accepted = r; });
    t.onAccepted = accepted;
    return { release, accepted: acceptedP };
  };
  await context.route("**/settings/theme", async route => {
    const v = new URLSearchParams(route.request().postData() || "").get("theme");
    t.sent.push(v); t.inFlight++; t.maxInFlight = Math.max(t.maxInFlight, t.inFlight); w.notify();
    const gate = t.gate, accepted = t.onAccepted;
    t.gate = null; t.onAccepted = null;
    try {
      const response = await route.fetch({ maxRedirects: 0 }); // the server has stored it
      if (accepted) accepted(v);
      if (gate) await gate;
      await route.fulfill({ response }).catch(() => {}); // the page may be gone
    } catch (err) {
      await route.abort().catch(() => {});
    } finally {
      t.inFlight--; t.done.push(v); w.notify();
    }
  });
  const reset = () => { t.sent.length = 0; t.done.length = 0; t.maxInFlight = 0; };
  // After the last expected save completes, give the page one task turn to
  // react to it (a follow-up, if any, is started from that response).
  const quiet = async n => {
    await w.until(() => t.done.length >= n && t.inFlight === 0, n + " theme saves");
    await page.evaluate(() => new Promise(r => requestAnimationFrame(() => setTimeout(r, 0)))).catch(() => {});
  };
  const radio = v => "input[name=theme][value=" + v + "]";
  const attr = () => page.evaluate(() => document.documentElement.getAttribute("data-theme"));

  // Real taps, as fast as a learner can switch: five changes while the
  // first save is held in flight.
  reset();
  let h = hold();
  await press(page, c, radio("dark"));
  check(c.name + " settings: dark applies immediately", (await attr()) === "dark", await attr());
  await h.accepted;
  for (const v of ["light", "system", "light", "dark"]) await press(page, c, radio(v));
  const final = await attr();
  const sentWhileHeld = t.sent.length;
  h.release();
  await quiet(2);
  const tapped = t.sent.slice();
  check(c.name + " settings: rapid tapped switches save one POST at a time (never two in flight)", t.maxInFlight === 1 && sentWhileHeld === 1, { maxInFlight: t.maxInFlight, sentWhileHeld, tapped });
  check(c.name + " settings: tapped switches coalesce (fewer POSTs than changes) and the last POST carries the latest choice", tapped.length === 2 && tapped[1] === "dark" && final === "dark", { tapped, final });

  // A burst inside one save: every change lands while the first POST is
  // held, so exactly one follow-up goes out, carrying the last choice.
  reset();
  h = hold();
  await page.evaluate(() => { for (const v of ["light", "system", "dark", "light"]) document.querySelector("input[name=theme][value=" + v + "]").click(); });
  const burstApplied = await attr();
  await h.accepted;
  h.release();
  await quiet(2);
  const burst = t.sent.slice();
  check(c.name + " settings: a burst of 4 switches during one save sends exactly 2 POSTs, serialized, the last = latest", t.maxInFlight === 1 && burst.length === 2 && burst[0] === "light" && burst[1] === "light" && burstApplied === "light", { maxInFlight: t.maxInFlight, burst, burstApplied });

  // Back to dark with one tap, for the persisted check and the dark picture.
  reset();
  await press(page, c, radio("dark"));
  await quiet(1);
  await page.reload({ waitUntil: "load" });
  const persisted = await page.evaluate(() => ({ checked: document.querySelector("input[name=theme]:checked")?.value, attr: document.documentElement.getAttribute("data-theme") }));
  check(c.name + " settings: latest mode persisted after reload", persisted.checked === "dark" && persisted.attr === "dark", persisted);
  await common(page, c, "settings");
  if (!c.mobile) await shot(page, "desktop-settings-dark.png", true);

  // Leaving at once: start from "Follow my system", select Dark and hold its
  // response after the server has stored it, select Light (applied at once,
  // its save waiting behind Dark), then go straight to Library. Light must
  // still be the saved mode, sent exactly once.
  reset();
  await press(page, c, radio("system"));
  await quiet(1);
  await page.reload({ waitUntil: "load" });
  reset();
  h = hold();
  const logBefore = themePosts();
  await press(page, c, radio("dark"));
  await h.accepted; // the server has stored Dark; its response is held
  await press(page, c, radio("light"));
  const leftWith = await attr();
  const pendingBeforeLeave = t.sent.slice();
  await Promise.all([page.waitForURL(u => new URL(u).pathname === "/library"), press(page, c, ".nav a[href='/library']")]);
  // The flushed save is a keepalive request from the page being left; wait
  // for the server to have it (its log, or else the stored mode).
  const savedLight = async () => /value="light" checked/.test(await (await context.request.get(base + "/settings")).text());
  const flushed = opt.serverLog ? await untilPolled(() => themePosts() - logBefore >= 2) : await untilPolled(savedLight);
  h.release();
  await w.until(() => t.inFlight === 0, "the held Dark save to finish").catch(() => {});
  await page.goto(base + "/settings", { waitUntil: "load" });
  const kept = await page.evaluate(() => ({ checked: document.querySelector("input[name=theme]:checked")?.value, attr: document.documentElement.getAttribute("data-theme") }));
  const serverPosts = opt.serverLog ? themePosts() - logBefore : null; // Dark + the one flushed Light
  check(c.name + " settings: leaving during an in-flight save keeps the last choice (Light), sent once",
    leftWith === "light" && pendingBeforeLeave.join() === "dark" && flushed && kept.checked === "light" && kept.attr === "light" && (serverPosts === null || serverPosts === 2),
    { leftWith, pendingBeforeLeave, flushed, serverPosts, kept });
  if (serverPosts === null) summary.notes = (summary.notes || []).concat("no --server-log: the flushed save was confirmed by the stored mode, not counted");

  // Restore "Follow my system" (one switch, saved) so later runs start clean.
  reset();
  await press(page, c, radio("system"));
  await quiet(1);
  await context.unroute("**/settings/theme");
  await page.reload({ waitUntil: "load" });
  const restored = await page.evaluate(() => document.querySelector("input[name=theme]:checked")?.value);
  check(c.name + " settings: single switch back to system saved", restored === "system", restored);

  const video = page.video();
  await context.close();
  if (video && opt.evidence) {
    const raw = path.join(videoDir, "raw.webm");
    await video.saveAs(raw);
    const dest = path.join(opt.evidence, "walkthrough-" + c.name + "-" + c.width + "x" + c.height + ".webm");
    compactVideo(raw, dest, c);
    summary.videos.push(path.basename(dest));
  }
  fs.rmSync(videoDir, { recursive: true, force: true });
}

// A sentence-length Incorrect answer (its struck answer set at Judgment
// size, so the review is far taller than the window) opens its review at
// the top with the question in view: with JavaScript, Next card is focused
// without scrolling; without it, nothing (no HTML autofocus) scrolls down.
async function longReview(browser, c, state, js) {
  const context = await browser.newContext(Object.assign(contextOptions(c), { storageState: state, javaScriptEnabled: js }));
  await instrument(context);
  const page = await context.newPage();
  const tag = js ? "JS" : "no-JS";
  watch(page, c.name + " " + tag);
  await page.goto(base + "/study", { waitUntil: "load" });
  // Without script every step is a page load; with it, <main> is swapped.
  const step = async sel => {
    if (js) {
      await page.locator(sel).click();
      await page.waitForSelector(sel === "[data-next]" ? "[data-answer-form]" : "[data-review]");
    } else {
      await Promise.all([page.waitForNavigation({ waitUntil: "load" }), page.locator(sel).click()]);
    }
  };
  let label = "";
  for (let tries = 0; tries < 6; tries++) {
    if (await page.$("[data-review]")) { await step("[data-next]"); continue; } // move off any open review
    await page.fill("#answer", LONG_ANSWER);
    await step("[data-check]");
    label = ((await page.textContent("[data-verdict-label]").catch(() => "")) || "").trim();
    if (label === "Incorrect") break;
  }
  await settle(page);
  const r = await page.evaluate(() => {
    const p = document.querySelector("[data-prompt]").getBoundingClientRect(), n = document.querySelector("[data-next]");
    return { scrollY, promptTop: Math.round(p.top), promptBottom: Math.round(p.bottom), docHeight: document.documentElement.scrollHeight,
      label: document.querySelector("[data-verdict-label]")?.textContent.trim(), htmlAutofocus: n ? n.hasAttribute("autofocus") : null,
      nextFocused: document.activeElement === n, typedShown: document.querySelector("[data-review]").textContent.includes("pilots use for this letter") };
  });
  const ok = r.label === "Incorrect" && r.typedShown && r.scrollY === 0 && r.promptTop >= 0 && r.promptBottom <= c.height && r.htmlAutofocus === false && r.docHeight > c.height && (js ? r.nextFocused : true);
  check(c.name + " " + tag + " review: sentence-length Incorrect answer opens at scrollY 0, question in view" + (js ? ", Next card focused" : ""), ok, r);
  if (!js && !c.mobile) await shot(page, "desktop-review-nojs-sentence.png");
  await common(page, c, tag + " review");
  await step("[data-next]"); // leave the session clean for the next run
  await context.close();
}

// ---- Pass 4: an older theme save that reaches Scry after a newer one ------
//
// A loopback reverse proxy sits between the page and Scry. It holds the Dark
// POST *before* forwarding it (the request itself is late, not just its
// response). Light, chosen next, waits behind Dark in the page and is sent as
// the keepalive flush when the learner goes straight to Library; it reaches
// Scry first. Only then is the held Dark request forwarded. Scry must keep
// Light: every scripted save carries theme_seq, and the older Dark is ignored.
function startHoldingProxy() {
  const target = new URL(base);
  const p = { events: [], armed: "", held: null, w: waiter() };
  const note = e => { p.events.push(Object.assign({ t: Date.now() }, e)); p.w.notify(); };
  const server = http.createServer((req, res) => {
    const chunks = [];
    req.on("data", ch => chunks.push(ch));
    req.on("end", () => {
      const body = Buffer.concat(chunks);
      const isTheme = req.method === "POST" && req.url.split("?")[0] === "/settings/theme";
      const form = isTheme ? new URLSearchParams(body.toString("utf8")) : null;
      const theme = form ? form.get("theme") : "", seq = form ? Number(form.get("theme_seq") || 0) : 0;
      const forward = () => {
        // A fresh upstream request from the buffered body: once the proxy has
        // the request, it reaches Scry even if the page that sent it is gone.
        const up = http.request({ hostname: target.hostname, port: target.port, method: req.method, path: req.url, headers: req.headers }, ur => {
          if (isTheme) note({ kind: "scry-answered", theme, seq, status: ur.statusCode });
          if (!res.destroyed && !res.headersSent) { res.writeHead(ur.statusCode, ur.headers); ur.pipe(res); } else ur.resume();
        });
        up.on("error", err => { if (isTheme) note({ kind: "upstream-error", theme, err: String(err) }); if (!res.headersSent) { res.writeHead(502); res.end(); } });
        if (isTheme) note({ kind: "forwarded", theme, seq });
        up.end(body);
      };
      if (isTheme) note({ kind: "received", theme, seq });
      if (isTheme && p.armed && theme === p.armed) {
        p.armed = "";
        p.held = forward;
        note({ kind: "held", theme, seq });
        return;
      }
      forward();
    });
  });
  return new Promise(resolve => server.listen(0, "127.0.0.1", () => {
    p.url = "http://127.0.0.1:" + server.address().port;
    p.close = () => new Promise(r => { server.closeAllConnections(); server.close(() => r()); });
    resolve(p);
  }));
}

async function heldBeforeScry(browser, c, state) {
  const p = await startHoldingProxy();
  const videoDir = fs.mkdtempSync(path.join(os.tmpdir(), "scry54-video-"));
  const context = await browser.newContext(Object.assign(contextOptions(c), {
    storageState: state, // its cookie is scoped to the host, so it holds on the proxy's port too
    recordVideo: opt.evidence ? { dir: videoDir, size: { width: 1280, height: 800 } } : undefined,
  }));
  await instrument(context);
  const page = await context.newPage();
  watch(page, c.name + " proxy", p.url);
  const ev = kind => p.events.filter(e => e.kind === kind);
  const radio = v => "input[name=theme][value=" + v + "]";
  const saved = async () => page.evaluate(() => ({ checked: document.querySelector("input[name=theme]:checked")?.value, attr: document.documentElement.getAttribute("data-theme") }));
  try {
    // Start from "Follow my system", saved.
    await page.goto(p.url + "/settings", { waitUntil: "load" });
    if ((await saved()).checked !== "system") {
      const n = ev("scry-answered").length;
      await press(page, c, radio("system"));
      await p.w.until(() => ev("scry-answered").length > n, "the system save");
      await page.reload({ waitUntil: "load" });
    }
    const start = await saved();
    p.events.length = 0;
    const logBefore = themePosts();

    p.armed = "dark";
    await press(page, c, radio("dark"));
    await p.w.until(() => ev("held").length === 1, "the Dark POST to be held before Scry");
    await press(page, c, radio("light"));
    const leftWith = await page.evaluate(() => document.documentElement.getAttribute("data-theme"));
    await Promise.all([page.waitForURL(u => new URL(u).pathname === "/library"), press(page, c, ".nav a[href='/library']")]);
    // The keepalive Light POST reaches Scry and is answered first ...
    await p.w.until(() => ev("scry-answered").some(e => e.theme === "light"), "the Light keepalive to reach Scry");
    const scryBeforeRelease = ev("forwarded").map(e => e.theme);
    // ... and only then does the held Dark request reach Scry.
    p.held();
    await p.w.until(() => ev("scry-answered").some(e => e.theme === "dark"), "the held Dark POST to reach Scry");
    await page.goto(p.url + "/settings", { waitUntil: "load" });
    const kept = await saved();
    const order = ev("forwarded").map(e => e.theme + "#" + e.seq);
    const answered = ev("scry-answered").map(e => e.theme + " " + e.status);
    const dark = ev("forwarded").find(e => e.theme === "dark"), light = ev("forwarded").find(e => e.theme === "light");
    const serverPosts = opt.serverLog ? themePosts() - logBefore : null;
    check(c.name + " settings: an older Dark POST held before Scry and arriving after the Light keepalive flush does not overwrite Light",
      start.checked === "system" && leftWith === "light" && scryBeforeRelease.join() === "light" &&
      ev("forwarded").map(e => e.theme).join() === "light,dark" && answered.join() === "light 303,dark 303" &&
      dark.seq > 0 && light.seq > dark.seq && kept.checked === "light" && kept.attr === "light" &&
      (serverPosts === null || serverPosts === 2),
      { start: start.checked, leftWith, scryBeforeRelease, order, answered, serverPosts, kept });
    summary.heldProxy = { order, answered, kept: kept.checked, serverPosts };
    await common(page, c, "settings after held proxy");
    await shot(page, "desktop-settings-light-kept-after-held-dark.png");

  } finally {
    const video = page.video();
    await context.close();
    if (video && opt.evidence) {
      const raw = path.join(videoDir, "raw.webm");
      await video.saveAs(raw);
      const dest = path.join(opt.evidence, "walkthrough-desktop-theme-held-before-scry.webm");
      compactVideo(raw, dest, c);
      summary.videos.push(path.basename(dest));
    }
    fs.rmSync(videoDir, { recursive: true, force: true });
    await p.close();
  }
  await noJsThemeSave(browser, c, state);
}

// Without JavaScript the Settings form posts the plain way, with no
// theme_seq: it always applies, even though the scripted saves before it
// stored a large sequence number. It restores "Follow my system" too.
async function noJsThemeSave(browser, c, state) {
  const context = await browser.newContext(Object.assign(contextOptions(c), { storageState: state, javaScriptEnabled: false }));
  await instrument(context);
  const page = await context.newPage();
  watch(page, c.name + " no-JS settings");
  try {
    await page.goto(base + "/settings", { waitUntil: "load" });
    const before = await page.evaluate(() => document.querySelector("input[name=theme]:checked")?.value);
    for (const v of ["dark", "system"]) {
      await page.check("[data-theme-form] input[name=theme][value=" + v + "]");
      await Promise.all([page.waitForNavigation({ waitUntil: "load" }), page.locator("[data-theme-form] button").first().click()]);
      const now = await page.evaluate(() => ({ checked: document.querySelector("input[name=theme]:checked")?.value, attr: document.documentElement.getAttribute("data-theme") }));
      check(c.name + " no-JS settings: Save applies " + v + " after scripted saves (no theme_seq)", now.checked === v && (v === "system" ? now.attr === null : now.attr === v), { before, now });
    }
    await common(page, c, "no-JS settings");
  } finally {
    await context.close();
  }
}

// ---- Pass 5: theme_seq counts from the server, never from a clock ---------
//
// Settings renders the stored theme save sequence (data-theme-seq); the page
// numbers each choice from it (+1 per choice), and a plain no-JS Save
// advances it by 1000. Two Pass 4b repros, both through the loopback proxy
// so every step is gated on Scry having answered:
//  1. Clock skew: context 1 (real clock) saves Dark; a fresh context 2 whose
//     browser clock is 3 minutes behind opens Settings, picks Light, reloads:
//     Light persists.
//  2. Held JS POST vs an explicit no-JS Save: JS context 1 picks Dark, held
//     by the proxy before Scry; no-JS context 2 picks Light and clicks Save
//     appearance (303); the held Dark is released (303); no-JS Settings
//     reloaded shows Light.
async function settingsNow(page) {
  return page.evaluate(() => {
    const f = document.querySelector("[data-theme-form]");
    return { checked: document.querySelector("input[name=theme]:checked")?.value, attr: document.documentElement.getAttribute("data-theme"),
      seq: f && f.hasAttribute("data-theme-seq") ? Number(f.getAttribute("data-theme-seq")) : null,
      seqField: !!document.querySelector("[data-theme-form] input[name=theme_seq]") };
  });
}

async function pass5Theme(browser, c, state) {
  const p = await startHoldingProxy();
  const ev = kind => p.events.filter(e => e.kind === kind);
  const radio = v => "input[name=theme][value=" + v + "]";
  const contexts = [];
  const open = async (label, extra, initScript) => {
    const context = await browser.newContext(Object.assign(contextOptions(c), { storageState: state }, extra || {}));
    contexts.push(context);
    await instrument(context);
    if (initScript) await initScript(context);
    const page = await context.newPage();
    watch(page, c.name + " " + label, p.url);
    return page;
  };
  // pick taps a mode on a JS page and waits for Scry to answer its save.
  const pick = async (page, v) => {
    const n = ev("scry-answered").length;
    await press(page, c, radio(v));
    await p.w.until(() => ev("scry-answered").length > n, "the " + v + " save to be answered");
    return ev("scry-answered").slice(-1)[0];
  };
  try {
    // ---- 1. Clock skew -------------------------------------------------
    const one = await open("pass5 clock ctx1");
    await one.goto(p.url + "/settings", { waitUntil: "load" });
    if ((await settingsNow(one)).checked !== "system") { await pick(one, "system"); await one.reload({ waitUntil: "load" }); }
    const one0 = await settingsNow(one);
    const darkSave = await pick(one, "dark");
    const SKEW = 3 * 60 * 1000;
    const skewedAt = Date.now() - SKEW;
    const two = await open("pass5 clock ctx2 (-3 min)", null, async ctx => { await ctx.clock.install({ time: new Date(skewedAt) }); });
    await two.goto(p.url + "/settings", { waitUntil: "load" });
    const skew = await two.evaluate(() => Date.now());
    const two0 = await settingsNow(two);
    const lightSave = await pick(two, "light");
    await two.reload({ waitUntil: "load" });
    const two1 = await settingsNow(two);
    const behindMs = Date.now() - skew;
    check(c.name + " pass5 clock skew: context 2 with its browser clock 3 min behind picks Light after context 1 saved Dark; reload shows Light",
      one0.checked === "system" && one0.seqField === false && darkSave.theme === "dark" && darkSave.status === 303 && darkSave.seq === one0.seq + 1 &&
      behindMs >= SKEW - 5000 && two0.checked === "dark" && two0.seq === darkSave.seq &&
      lightSave.theme === "light" && lightSave.status === 303 && lightSave.seq === two0.seq + 1 &&
      two1.checked === "light" && two1.attr === "light" && two1.seq === lightSave.seq,
      { ctx1Start: one0, darkSave, ctx2ClockBehindMs: behindMs, ctx2Loaded: two0, lightSave, ctx2Reloaded: two1 });
    summary.pass5ClockSkew = { ctx2ClockBehindMs: behindMs, dark: "dark#" + darkSave.seq + " " + darkSave.status, light: "light#" + lightSave.seq + " " + lightSave.status, reloaded: two1 };
    await common(two, c, "pass5 clock skew settings");
    await shot(two, "desktop-settings-light-after-clock-skew.png");

    // Reset to "Follow my system" for repro 2 with a plain no-JS Save, which
    // applies whatever the stored sequence is.
    const reset = await open("pass5 no-JS reset", { javaScriptEnabled: false });
    await reset.goto(p.url + "/settings", { waitUntil: "load" });
    await reset.check("[data-theme-form] " + radio("system"));
    await Promise.all([reset.waitForNavigation({ waitUntil: "load" }), reset.locator("[data-theme-form] button").first().click()]);
    check(c.name + " pass5 no-JS Save resets to system", (await settingsNow(reset)).checked === "system", await settingsNow(reset));

    // ---- 2. Held JS Dark vs explicit no-JS Light Save --------------------
    const js = await open("pass5 held JS ctx1");
    await js.goto(p.url + "/settings", { waitUntil: "load" });
    const js0 = await settingsNow(js);
    p.events.length = 0;
    const logBefore = themePosts();
    p.armed = "dark";
    await press(js, c, radio("dark"));
    await p.w.until(() => ev("held").length === 1, "the JS Dark POST to be held before Scry");
    const held = ev("held")[0];
    const nojs = await open("pass5 no-JS ctx2", { javaScriptEnabled: false });
    await nojs.goto(p.url + "/settings", { waitUntil: "load" });
    const nojs0 = await settingsNow(nojs);
    await nojs.check("[data-theme-form] " + radio("light"));
    await Promise.all([nojs.waitForNavigation({ waitUntil: "load" }), nojs.locator("[data-theme-form] button").first().click()]);
    const nojsSave = ev("scry-answered").find(e => e.theme === "light");
    const nojs1 = await settingsNow(nojs);
    const darkAnsweredBefore = ev("scry-answered").some(e => e.theme === "dark");
    p.held();
    await p.w.until(() => ev("scry-answered").some(e => e.theme === "dark"), "the held JS Dark POST to reach Scry");
    const heldAnswer = ev("scry-answered").find(e => e.theme === "dark");
    await nojs.reload({ waitUntil: "load" });
    const nojs2 = await settingsNow(nojs);
    const order = ev("forwarded").map(e => e.theme + "#" + e.seq);
    const serverPosts = opt.serverLog ? themePosts() - logBefore : null;
    check(c.name + " pass5 held JS Dark (before Scry) + explicit no-JS Light Save + release Dark: reload shows Light",
      js0.checked === "system" && held.seq === js0.seq + 1 && nojs0.seqField === false && nojs0.seq === js0.seq &&
      !!nojsSave && nojsSave.seq === 0 && nojsSave.status === 303 && nojs1.checked === "light" && nojs1.seq === js0.seq + 1000 &&
      !darkAnsweredBefore && heldAnswer.status === 303 && order.join() === "light#0,dark#" + held.seq &&
      nojs2.checked === "light" && nojs2.attr === "light" && nojs2.seq === js0.seq + 1000 && (serverPosts === null || serverPosts === 2),
      { jsLoaded: js0, held: "dark#" + held.seq, noJsLoaded: nojs0, noJsSave: nojsSave, afterNoJsSave: nojs1, heldAnswer, order, serverPosts, reloaded: nojs2 });
    summary.pass5HeldVsNoJs = { order, answered: ev("scry-answered").map(e => e.theme + "#" + e.seq + " " + e.status), reloaded: nojs2, serverPosts };
    await common(nojs, c, "pass5 no-JS settings after held Dark");
    await shot(nojs, "desktop-settings-nojs-light-kept-after-held-dark.png");

    // The JS page that sent the held Dark reads the new sequence on its next
    // load and its next choice applies; restore "Follow my system".
    await js.reload({ waitUntil: "load" });
    const js1 = await settingsNow(js);
    const restore = await pick(js, "system");
    await js.reload({ waitUntil: "load" });
    const js2 = await settingsNow(js);
    check(c.name + " pass5 a JS page loaded after the no-JS Save counts up from the new sequence and its save applies",
      js1.checked === "light" && js1.seq === nojs2.seq && restore.seq === js1.seq + 1 && restore.status === 303 && js2.checked === "system" && js2.seq === restore.seq,
      { loaded: js1, restore, reloaded: js2 });
  } finally {
    for (const ctx of contexts) await ctx.close().catch(() => {});
    await p.close();
  }
}

// ---- Pass 4: a long question's review on a phone opens framed at the top ---
//
// A fresh learner (signed up in an unrecorded context) with exactly one card,
// a ~450-character question. On a 390x844 touch phone with JavaScript, the
// answer field below the long prompt is focused, which scrolls the waiting
// page down; tapping Check must open the review at scrollY 0 with the
// question's top on screen and Next card focused. Scrolling down in the
// review afterwards and pressing Ask Why must not jump back up.
const LONG_QUESTION = "In the light-dependent reactions of photosynthesis, which take place across the thylakoid membranes inside the chloroplast, the energy of absorbed sunlight is used to split water molecules, releasing oxygen as a by-product and producing the energy carriers that later drive the Calvin cycle in the stroma, where carbon dioxide is fixed into sugar. What is the name of the green pigment, embedded in those membranes, that absorbs most of that light?";

async function longQuestionPhone(browser, c) {
  // Setup: a new learner and their single long card (never recorded).
  const setupCtx = await browser.newContext(contextOptions(c));
  await instrument(setupCtx);
  const s = await setupCtx.newPage();
  watch(s, c.name + " long-question setup");
  await s.goto(base + "/signup", { waitUntil: "load" });
  await s.fill("#email", "long-question-" + Date.now() + "@example.com");
  await s.fill("#password", "long-question-check");
  await Promise.all([s.waitForURL(u => new URL(u).pathname !== "/signup"), s.locator("main form button.btn-accent").first().tap()]);
  await s.goto(base + "/library/texts/new", { waitUntil: "load" });
  await s.fill("#title", "Plant biology");
  await s.fill("#text", "Placeholder line");
  await Promise.all([s.waitForURL(/\/library\/ideas\/\d+/), s.locator("main form button.btn-accent").first().tap()]);
  const ideaURL = s.url().split("?")[0];
  const placeholders = await s.$$eval("a[href^='/library/items/']", as => as.map(a => a.getAttribute("href")));
  await s.evaluate(() => { document.querySelector("#add").open = true; });
  await s.fill("#add #prompt", LONG_QUESTION);
  await s.fill("#add #answer", "Chlorophyll");
  await Promise.all([s.waitForNavigation({ waitUntil: "load" }), s.locator("#add form button").first().tap()]);
  for (const href of placeholders) {
    await s.goto(base + href, { waitUntil: "load" });
    await Promise.all([s.waitForNavigation({ waitUntil: "load" }), s.locator("form[action$='/remove'] button").first().tap()]);
  }
  await s.goto(ideaURL, { waitUntil: "load" });
  const cards = await s.$$eval(".pane-idea a[href^='/library/items/'] .item-q, main a[href^='/library/items/'] .item-q", els => [...new Set(els.map(e => e.textContent.trim()))]);
  const state = await setupCtx.storageState();
  await setupCtx.close();
  check(c.name + " long question: the fresh learner has exactly one card, a ~450-character question", cards.length === 1 && cards[0] === LONG_QUESTION && LONG_QUESTION.length >= 440 && LONG_QUESTION.length <= 460, { cards: cards.map(q => q.length), len: LONG_QUESTION.length });

  // The recorded part: study that card.
  const videoDir = fs.mkdtempSync(path.join(os.tmpdir(), "scry54-video-"));
  const context = await browser.newContext(Object.assign(contextOptions(c), {
    storageState: state, recordVideo: opt.evidence ? { dir: videoDir, size: { width: 390, height: 844 } } : undefined,
  }));
  await instrument(context);
  const page = await context.newPage();
  watch(page, c.name + " long-question");
  try {
    await page.goto(base + "/study", { waitUntil: "load" });
    await page.waitForSelector("[data-answer-form]");
    await settle(page);
    const prompt = (await page.textContent("main h1#prompt")).trim(); // the waiting prompt
    await page.locator("#answer").tap(); // focus the field below the long prompt
    await page.locator("#answer").pressSequentially("chlorophyll");
    await settle(page);
    const waiting = await page.evaluate(() => ({ scrollY, promptTop: Math.round(document.querySelector("main h1#prompt").getBoundingClientRect().top), focused: document.activeElement?.id }));
    check("phone long question: focusing the answer scrolls the waiting page down (scrollY > 0)", prompt === LONG_QUESTION && waiting.focused === "answer" && waiting.scrollY > 0, Object.assign({ promptLen: prompt.length }, waiting));
    await shot(page, "phone-long-question-answer-focused.png");
    await page.locator("[data-check]").tap();
    await page.waitForSelector("[data-review]");
    await settle(page);
    const rev = await page.evaluate(() => {
      const p = document.querySelector("[data-prompt]").getBoundingClientRect(), n = document.querySelector("[data-next]");
      return { scrollY, promptTop: Math.round(p.top), topBar: Math.round(document.querySelector("header.top").getBoundingClientRect().top),
        nextFocused: !!n && document.activeElement === n, nextLabel: n?.textContent.trim(), label: document.querySelector("[data-verdict-label]")?.textContent.trim(),
        docHeight: document.documentElement.scrollHeight };
    });
    check("phone long question: tapping Check opens the review at scrollY 0, question top on screen, Next/Finish focused",
      rev.scrollY === 0 && rev.promptTop >= 0 && rev.topBar >= 0 && rev.nextFocused && /^(Correct|Almost Correct|Incorrect)$/.test(rev.label || ""), Object.assign({ waitingScrollY: waiting.scrollY }, rev));
    summary.longQuestion = { waitingScrollY: waiting.scrollY, reviewScrollY: rev.scrollY, promptTop: rev.promptTop, verdict: rev.label, next: rev.nextLabel };
    await common(page, c, "long-question review");
    await shot(page, "phone-long-question-review-top.png");
    // Scrolled down by hand, Ask Why stays where the learner is.
    if (await page.$("[data-explain]")) {
      await page.locator("[data-explain]").scrollIntoViewIfNeeded();
      const before = await page.evaluate(() => scrollY);
      await page.locator("[data-explain]").tap();
      await page.waitForSelector(".explanation:not([hidden])");
      await settle(page);
      const after = await page.evaluate(() => scrollY);
      check("phone long question: after scrolling down, Ask Why does not jump back to the top", before > 0 && after > 0, { before, after, docHeight: rev.docHeight });
    } else check("phone long question: Ask Why offered", false, "no [data-explain]");
    await page.locator("[data-next]").tap();
    await page.waitForURL(u => new URL(u).pathname.startsWith("/study"));
  } finally {
    const video = page.video();
    await context.close();
    if (video && opt.evidence) {
      const raw = path.join(videoDir, "raw.webm");
      await video.saveAs(raw);
      const dest = path.join(opt.evidence, "walkthrough-phone-long-question-review.webm");
      compactVideo(raw, dest, c);
      summary.videos.push(path.basename(dest));
    }
    fs.rmSync(videoDir, { recursive: true, force: true });
  }
}

// compactVideo re-encodes Playwright's VP8 recording as a small VP9 file
// (no audio) played at half speed, since the gated walkthrough runs faster
// than a person can follow. Without ffmpeg the recording is kept as it is.
function compactVideo(raw, dest, c) {
  const scale = c.mobile ? "360:-2" : "960:-2";
  const r = spawnSync("ffmpeg", ["-y", "-loglevel", "error", "-i", raw, "-an", "-vf", "setpts=2.0*PTS,fps=10,scale=" + scale,
    "-c:v", "libvpx-vp9", "-b:v", "0", "-crf", c.mobile ? "46" : "44", "-deadline", "good", "-cpu-used", "4", "-row-mt", "1", dest], { encoding: "utf8" });
  if (r.status !== 0) { fs.copyFileSync(raw, dest); summary.videoNote = "ffmpeg unavailable or failed; raw recording kept"; }
}

(async () => {
  const health = await fetch(base + "/healthz").then(async r => ({ status: r.status, body: await r.json() }));
  check("healthz 200, test mode" + (opt.sha ? ", build " + opt.sha : ""), health.status === 200 && health.body.mode === "test" && health.body.status === "ok" && (!opt.sha || health.body.build_sha === opt.sha), { status: health.status, build_sha: health.body.build_sha, mode: health.body.mode });
  summary.healthz = { status: health.status, build_sha: health.body.build_sha, mode: health.body.mode, database: health.body.database };
  if (health.body.mode !== "test") { console.log(JSON.stringify(summary, null, 2)); process.exit(1); }
  const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN || "/usr/bin/google-chrome", headless: true, args: ["--no-sandbox"] });
  try {
    for (const c of CONFIGS) {
      try {
        const state = await setup(browser, c);
        if (opt.only === "pass5") { if (!c.mobile) await pass5Theme(browser, c, state); continue; }
        if (opt.only !== "pass4") await run(browser, c, state);
        if (!c.mobile) {
          if (opt.only !== "pass4") { await longReview(browser, c, state, true); await longReview(browser, c, state, false); }
          await heldBeforeScry(browser, c, state);
          await pass5Theme(browser, c, state);
        } else {
          await longQuestionPhone(browser, c);
        }
      } catch (err) { check(c.name + " walkthrough ran to the end", false, String(err.stack || err).split("\n").slice(0, 3).join(" | ")); }
    }
  } finally { await browser.close(); }
  check("0 console errors, page errors, failed or external requests, HTTP >= 500", summary.problems.length === 0, summary.problems);
  const finish = () => {
    summary.ok = summary.failures.length === 0;
    summary.passed = summary.checks.filter(c => c.ok).length;
    summary.total = summary.checks.length;
  };
  summary.finishedAt = new Date().toISOString();
  let text;
  if (opt.evidence) {
    // The verifier writes this summary into the bundle itself, and checks
    // the whole bundle (every file in the evidence dir, this log included)
    // against the media budget, so what it emits is what is delivered.
    const files = fs.readdirSync(opt.evidence).filter(f => f !== CHECKS_LOG && fs.statSync(path.join(opt.evidence, f)).isFile());
    const otherBytes = files.reduce((n, f) => n + fs.statSync(path.join(opt.evidence, f)).size, 0);
    summary.media = { dir: path.basename(opt.evidence), files: files.concat(CHECKS_LOG).sort(), budgetBytes: MEDIA_BUDGET, bytes: 0 };
    const name = "evidence bundle <= " + MEDIA_BUDGET + " bytes (all files, this log included)";
    const rec = { name, ok: true };
    summary.checks.push(rec);
    for (let i = 0; i < 4; i++) {
      finish();
      text = JSON.stringify(summary, null, 2) + "\n";
      const bytes = otherBytes + Buffer.byteLength(text);
      if (bytes === summary.media.bytes) break;
      summary.media.bytes = bytes;
      rec.ok = bytes <= MEDIA_BUDGET;
      summary.failures = summary.failures.filter(f => !f.startsWith(name));
      if (!rec.ok) { rec.detail = bytes; summary.failures.push(name + ": " + bytes); } else delete rec.detail;
    }
    fs.writeFileSync(path.join(opt.evidence, CHECKS_LOG), text);
  } else {
    finish();
    text = JSON.stringify(summary, null, 2) + "\n";
  }
  process.stdout.write(text);
  process.exit(summary.ok ? 0 : 1);
})();
