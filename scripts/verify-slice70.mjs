#!/usr/bin/env node
// verify-slice70.mjs: check Slice #70 (iPhone dogfooding polish on the Study
// review flow) on a running test-mode Scry, in real browsers, with
// disposable learners only.
//
//   SCRY_MODE=test SCRY_SEED_DEMO=1 SCRY_TEST_JUDGMENTS=missed,right,close PORT=18071 ./bin/scry &
//   node scripts/verify-slice70.mjs --url http://127.0.0.1:18071 --label candidate \
//        [--shots ~/chief-evidence/media] [--video] [--state-dir /tmp/s70] [--expect pass|observe]
//   # later, after restarting the same database with SCRY_TEST_CLOCK 20 hours on:
//   node scripts/verify-slice70.mjs --url ... --label candidate --stale --state-dir /tmp/s70
//
// Engines and viewports (each in light and dark):
//   iphone : WebKit, 390x844, touch, iPhone user agent, America/Chicago
//   desktop: Chromium (system Chrome), 1440x900, America/Chicago
// WebKit is Playwright's build (npx playwright@<playwright-core version>
// install webkit); Chromium is CHROME_BIN or /usr/bin/google-chrome.
//
// Each run signs up a fresh learner (s70-<label>-<config>-<time>@example.com),
// adds the NATO phonetic alphabet starter set and studies it:
//   1. the waiting screen: navigation (all six destinations in view, one row,
//      reachable by Tab, no sideways scroll) and, on the phone, no Study
//      title/purpose repeated under the navigation;
//   2. a skipped (blank) answer: "No answer given.", "Skipped — review the
//      answer above.", no grader rubric, Incorrect, no Ask Why?;
//   3. the review shows the current card only (no scrollback, no "coming
//      up", no next card's prompt), the schedule's relative time and
//      interval agree, Next card is filled in ink (not the accent) and is
//      focused without a ring after a tap/click, and gets exactly one ring
//      under keyboard navigation;
//   4. a graded answer: the why is one of the plain sentences;
//   5. a long answer: the end of the review scrolls clear of the pinned
//      Next card bar (phone);
//   6. progression: Next card moves to the next card (position + 1).
// It leaves the last review pending and saves the learner's storage state,
// so --stale can reopen that review after the server's clock has moved on
// and check that the event and the interval still agree.
//
// Console errors, page errors, failed requests, HTTP >= 500 and CSP
// violations are counted per page. Screenshots use caret: "initial": with
// Playwright's default caret hiding it injects an inline <style>, which
// WebKit reports as a style-src violation of Scry's (correct) CSP.
//
// --expect pass (default) exits 1 if any check fails; --expect observe
// (used on the baseline) records the same checks and always exits 0.

import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { chromium, webkit, devices } from "playwright-core";

const args = process.argv.slice(2);
const opt = { url: "", label: "run", shots: "", video: false, stateDir: os.tmpdir(), expect: "pass", stale: false, only: "" };
for (let i = 0; i < args.length; i++) {
  const a = args[i];
  const v = () => args[++i];
  if (a === "--url") opt.url = v();
  else if (a === "--label") opt.label = v();
  else if (a === "--shots") opt.shots = v();
  else if (a === "--video") opt.video = true;
  else if (a === "--state-dir") opt.stateDir = v();
  else if (a === "--expect") opt.expect = v();
  else if (a === "--stale") opt.stale = true;
  else if (a === "--only") opt.only = v();
  else { console.error("unknown flag " + a); process.exit(2); }
}
if (!opt.url) { console.error("--url is required"); process.exit(2); }
const base = opt.url.replace(/\/$/, "");
if (opt.shots) fs.mkdirSync(opt.shots, { recursive: true });

const CONFIGS = [
  { name: "iphone-light", engine: "webkit", scheme: "light", mobile: true },
  { name: "iphone-dark", engine: "webkit", scheme: "dark", mobile: true },
  { name: "desktop-light", engine: "chromium", scheme: "light", mobile: false },
  { name: "desktop-dark", engine: "chromium", scheme: "dark", mobile: false },
].filter(c => !opt.only || opt.only.split(",").includes(c.name));

const RUBRIC = /grader|contradictory|accepted answer|cannot reliably be classified|materially imprecise|irrelevant/i;
const PLAIN = ["Same meaning as the correct answer.", "Close — part of it is missing or imprecise.", "Not quite — compare it with the correct answer above."];
const SKIP = "Skipped — review the answer above.";
const LONG = "Alfa Bravo Charlie Delta Echo Foxtrot Golf Hotel India Juliett Kilo Lima Mike November Oscar Papa Quebec Romeo Sierra Tango Uniform Victor Whiskey Xray Yankee Zulu, which is the whole alphabet rather than one letter";

const results = [];
function check(name, ok, detail) {
  results.push({ name, ok: !!ok, detail });
  console.error((ok ? "PASS " : "FAIL ") + name + (ok ? "" : "  " + JSON.stringify(detail)));
}

function contextOptions(c) {
  const o = { colorScheme: c.scheme, timezoneId: "America/Chicago", locale: "en-US" };
  if (c.mobile) {
    const d = devices["iPhone 13"];
    Object.assign(o, { userAgent: d.userAgent, viewport: { width: 390, height: 844 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true });
  } else {
    Object.assign(o, { viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1 });
  }
  return o;
}

async function launch(engine) {
  if (engine === "webkit") return webkit.launch();
  const bin = process.env.CHROME_BIN || "/usr/bin/google-chrome";
  return chromium.launch({ executablePath: bin });
}

// Per-page health: console errors, page errors, failed requests, 5xx, CSP.
function watch(page, health) {
  page.__health = health;
  page.on("console", m => { if (m.type() === "error") health.console.push(m.text().slice(0, 200)); });
  page.on("pageerror", e => health.pageErrors.push(String(e).slice(0, 200)));
  // A favicon request Chrome cancels because the page navigated on is not a
  // failure of the app; it is listed under "aborted" and not counted.
  page.on("requestfailed", r => {
    const why = (r.failure() || {}).errorText || "";
    if (/ERR_ABORTED|cancelled/i.test(why) && /\/static\/icon\.svg$/.test(r.url())) health.aborted.push(r.url() + " " + why);
    else health.failed.push(r.url() + " " + why);
  });
  page.on("response", r => { if (r.status() >= 500) health.http5xx.push(r.status() + " " + r.url()); });
  page.on("console", m => { if (m.text().startsWith("S70-CSP ")) health.csp.push(m.text().slice(8)); });
}
async function instrument(context) {
  await context.addInitScript(() => {
    document.addEventListener("securitypolicyviolation", e => {
      console.log("S70-CSP " + e.violatedDirective + " " + (e.blockedURI || "") + " " + location.pathname);
    }, true);
  });
}

// Screenshots are taken inside a marked window: Playwright's WebKit
// screenshot injects an inline <style> into the page (even with caret:
// "initial"), which Scry's style-src 'self' policy correctly refuses. Any
// console error or CSP report raised inside that window is set aside as
// "tooling" (and still printed in the summary) rather than counted
// against the app.
async function shot(page, c, name, opts = {}) {
  if (!opt.shots) return;
  const file = path.join(opt.shots, `${opt.label}-${c.name}-${name}.png`);
  const h = page.__health;
  const n = h ? { console: h.console.length, csp: h.csp.length } : null;
  await page.screenshot({ path: file, caret: "initial", fullPage: !!opts.fullPage });
  await page.waitForTimeout(150);
  if (h) {
    h.tooling.push(...h.console.splice(n.console).map(x => "during screenshot " + name + ": " + x));
    h.tooling.push(...h.csp.splice(n.csp).map(x => "during screenshot " + name + ": csp " + x));
  }
}

async function press(page, c, locator) {
  if (c.mobile) await locator.tap(); else await locator.click();
}

async function settle(page) {
  await page.waitForLoadState("load");
  await page.evaluate(() => document.fonts && document.fonts.ready);
  await page.waitForTimeout(350); // past the 250 ms arrive/recede animations
}

// What the review looks like, measured in the page.
async function reviewFacts(page) {
  return page.evaluate(() => {
    const q = s => document.querySelector(s), qa = s => [...document.querySelectorAll(s)];
    const main = q("main");
    const next = q("[data-next]");
    const ns = next && getComputedStyle(next);
    const accentProbe = document.createElement("span");
    accentProbe.style.color = "var(--accent)"; document.body.appendChild(accentProbe);
    const accent = getComputedStyle(accentProbe).color; accentProbe.remove();
    const inkProbe = document.createElement("span");
    inkProbe.style.color = "var(--ink)"; document.body.appendChild(inkProbe);
    const ink = getComputedStyle(inkProbe).color; inkProbe.remove();
    const bar = q(".judged-actions"), barStyle = bar && getComputedStyle(bar);
    return {
      prompt: (q("[data-review] [data-prompt]") || {}).textContent?.trim(),
      verdict: (q("[data-verdict-label]") || {}).textContent?.trim(),
      why: (q("[data-review] .why") || {}).textContent?.trim(),
      noAnswer: !!qa("[data-skipped]").find(e => e.textContent.trim() === "No answer given."),
      askWhy: !!q("[data-explain]"),
      scrollback: qa("main .scrollback li").length,
      upcoming: qa("main .upcoming li").length,
      mainText: main.innerText,
      nextRel: (q("[data-next-rel]") || {}).textContent?.trim(),
      nextDate: (q("[data-next-review] time") || {}).textContent?.trim(),
      interval: (q("[data-interval]") || {}).textContent?.trim(),
      intervalFrom: (q("[data-interval-from]") || {}).textContent?.trim(),
      telemetry: (q("[data-telemetry]") || {}).innerText,
      trail: qa("[data-trail-verdict]").map(e => e.getAttribute("data-trail-verdict")),
      trailLabel: (q("[data-trail]") || { getAttribute: () => "" }).getAttribute("aria-label"),
      nextBg: ns && ns.backgroundColor, nextColor: ns && ns.color, accent, ink,
      nextFocused: document.activeElement === next,
      nextOutline: ns && (ns.outlineStyle + " " + ns.outlineWidth + " " + ns.outlineColor),
      nextBoxShadow: ns && ns.boxShadow,
      kbdNav: document.documentElement.classList.contains("kbd-nav"),
      barFixed: barStyle ? barStyle.position : "",
      position: (q(".margin-count") || {}).textContent?.trim(),
    };
  });
}

async function navFacts(page) {
  return page.evaluate(() => {
    const links = [...document.querySelectorAll("header nav[aria-label=Main] a")];
    const r = e => e.getBoundingClientRect();
    const vw = document.documentElement.clientWidth;
    const mode = document.querySelector(".margin-mode"), purpose = document.querySelector(".margin-purpose");
    const vis = e => !!e && getComputedStyle(e).display !== "none" && r(e).height > 0;
    return {
      labels: links.map(a => a.innerText.replace(/\s+/g, " ").trim()),
      rows: [...new Set(links.map(a => Math.round(r(a).top)))].length,
      allInView: links.every(a => r(a).left >= -0.5 && r(a).right <= vw + 0.5 && r(a).width > 0),
      minTarget: Math.min(...links.map(a => Math.min(r(a).width, r(a).height))),
      headerH: Math.round(r(document.querySelector("header.top")).height),
      scrollW: document.documentElement.scrollWidth, vw,
      navScrollable: (() => { const n = document.querySelector("header nav"); return n.scrollWidth > n.clientWidth + 1; })(),
      tabbable: links.every(a => a.tabIndex >= 0),
      studyTitleShown: vis(mode), studyPurposeShown: vis(purpose),
      current: (links.find(a => a.getAttribute("aria-current") === "page") || {}).innerText?.trim(),
    };
  });
}

async function signUp(page, c) {
  const email = `s70-${opt.label}-${c.name}-${Date.now()}@example.com`;
  await page.goto(base + "/signup", { waitUntil: "load" });
  await page.fill("#email", email);
  await page.fill("#password", "slice70-disposable-pass");
  await Promise.all([page.waitForURL(u => new URL(u).pathname !== "/signup"), press(page, c, page.locator("main form button.btn-accent").first())]);
  // Add the NATO phonetic alphabet (the first tile).
  if (new URL(page.url()).pathname !== "/shelf") await page.goto(base + "/shelf", { waitUntil: "load" });
  const add = page.locator("form[data-add]").first().locator("button");
  await press(page, c, add);
  await page.waitForSelector("[data-shelf-bar]:not([hidden])", { timeout: 10000 }).catch(() => {});
  return email;
}

async function submitAnswer(page, c, text) {
  await page.waitForSelector("[data-answer-form]");
  const input = page.locator("#answer");
  if (c.mobile) await input.tap(); else await input.click();
  if (text) await input.pressSequentially(text, { delay: 0 });
  // The learner taps/clicks Check (no keyboard navigation).
  await Promise.all([
    page.waitForSelector("[data-review]", { timeout: 15000 }),
    press(page, c, page.locator("[data-check]")),
  ]);
  await settle(page);
}

async function runConfig(c) {
  const health = { console: [], pageErrors: [], failed: [], http5xx: [], csp: [], tooling: [], aborted: [] };
  const browser = await launch(c.engine);
  const videoDir = fs.mkdtempSync(path.join(os.tmpdir(), "s70-video-"));
  const record = opt.video && c.name === "iphone-light";
  const context = await browser.newContext(Object.assign(contextOptions(c), record ? { recordVideo: { dir: videoDir, size: { width: 390, height: 844 } } } : {}));
  await instrument(context);
  const page = await context.newPage();
  watch(page, health);
  const tag = `${opt.label} ${c.name}`;
  const out = { config: c.name };
  try {
    out.email = await signUp(page, c);
    await page.goto(base + "/study", { waitUntil: "load" });
    await settle(page);

    // 1. Waiting screen and navigation.
    const nav = await navFacts(page);
    out.nav = nav;
    await shot(page, c, "1-study-waiting");
    check(`${tag}: nav lists the six destinations`, nav.labels.join(",").replace(/ \d+$/, "") .startsWith("Study,Library,Inbox,Create,Billing,Settings"), nav.labels);
    check(`${tag}: nav is one row with every destination in view, no sideways scroll`, nav.rows === 1 && nav.allInView && !nav.navScrollable && nav.scrollW <= nav.vw, nav);
    check(`${tag}: nav destinations have >= 44px targets and are tabbable`, nav.minTarget >= 44 && nav.tabbable, nav);
    if (c.mobile) {
      check(`${tag}: phone Study rail does not repeat "Study" + purpose under the nav`, !nav.studyTitleShown && !nav.studyPurposeShown && nav.current === "Study", nav);
      check(`${tag}: phone header is compact (<= 104px: 48px brand row + 52px nav)`, nav.headerH <= 104, nav.headerH);
    }
    // Keyboard: Tab from the top of the page reaches every nav destination.
    await page.evaluate(() => { document.activeElement && document.activeElement.blur(); window.scrollTo(0, 0); });
    const reached = [];
    for (let i = 0; i < 14; i++) {
      await page.keyboard.press("Tab");
      const r = await page.evaluate(() => {
        const a = document.activeElement;
        const s = getComputedStyle(a);
        return { nav: !!a.closest("header nav"), label: a.innerText.trim().split(/\s+/)[0], outline: s.outlineStyle + " " + s.outlineWidth };
      });
      if (r.nav) reached.push(r);
    }
    check(`${tag}: Tab reaches all six nav links with a visible 2px ring`, reached.length >= 6 && reached.every(r => r.outline.startsWith("solid 2px")), reached.map(r => r.label + ":" + r.outline));
    if (c.mobile) {
      await page.locator("header nav a[href='/settings']").focus();
      await shot(page, c, "1b-nav-keyboard-focus");
    }

    // 2-3. A skipped (blank) answer.
    const posBefore = await page.evaluate(() => (document.querySelector(".margin-count") || {}).textContent?.trim());
    await submitAnswer(page, c, "");
    const skip = await reviewFacts(page);
    out.skip = Object.assign({}, skip, { mainText: undefined });
    await shot(page, c, "2-review-skipped");
    check(`${tag}: skip says "No answer given."`, skip.noAnswer, skip.mainText.slice(0, 300));
    check(`${tag}: skip why is "${SKIP}"`, skip.why === SKIP, skip.why);
    check(`${tag}: skip is Incorrect with no Ask Why?`, skip.verdict === "Incorrect" && !skip.askWhy, { verdict: skip.verdict, askWhy: skip.askWhy });
    check(`${tag}: no grader rubric wording on the review`, !RUBRIC.test(skip.mainText), skip.why);
    check(`${tag}: review shows the current card only (no scrollback, no coming-up, no next prompt)`, skip.scrollback === 0 && skip.upcoming === 0 && !/NATO alphabet: B/.test(skip.mainText), { scrollback: skip.scrollback, upcoming: skip.upcoming });
    check(`${tag}: event and interval agree right after answering ("in 1 day" beside "1 day after this answer")`, skip.nextRel === "in 1 day" && skip.interval === "1 day" && skip.intervalFrom === "after this answer" && !/today/.test(skip.telemetry || ""), { rel: skip.nextRel, date: skip.nextDate, interval: skip.interval, from: skip.intervalFrom });
    check(`${tag}: Next card is filled in ink, not the accent`, skip.nextBg === skip.ink && skip.nextBg !== skip.accent, { bg: skip.nextBg, ink: skip.ink, accent: skip.accent });
    check(`${tag}: Next card is focused after Check without a ring (pointer)`, skip.nextFocused && /^none/.test(skip.nextOutline) && skip.nextBoxShadow === "none", { focused: skip.nextFocused, outline: skip.nextOutline, shadow: skip.nextBoxShadow });
    check(`${tag}: schedule shows a verdict trail with its words`, skip.trail.join(",") === "missed" && /Incorrect/.test(skip.trailLabel || ""), { trail: skip.trail, label: skip.trailLabel });

    // Keyboard ring on Next card: Shift+Tab then Tab lands back on it.
    await page.keyboard.press("Shift+Tab");
    await page.keyboard.press("Tab");
    const kb = await reviewFacts(page);
    check(`${tag}: under keyboard navigation Next card wears one 2px accent ring`, kb.nextFocused && kb.nextOutline.startsWith("solid 2px") && kb.nextBoxShadow === "none", { focused: kb.nextFocused, outline: kb.nextOutline });
    if (c.name === "iphone-light" || c.name === "desktop-light") {
      await page.locator("[data-next]").scrollIntoViewIfNeeded();
      await shot(page, c, "3-next-card-keyboard-ring");
    }

    // 6. Progression: Next card goes to card 2.
    await Promise.all([page.waitForSelector("[data-answer-form]"), press(page, c, page.locator("[data-next]"))]);
    await settle(page);
    const posAfter = await page.evaluate(() => (document.querySelector(".margin-count") || {}).textContent?.trim());
    const prompt2 = await page.evaluate(() => (document.querySelector("#prompt") || {}).textContent?.trim());
    check(`${tag}: Next card advances (position ${posBefore} -> ${posAfter}, prompt ${prompt2})`, posBefore && posAfter && parseInt(posAfter) === parseInt(posBefore) + 1 && /NATO alphabet: B/.test(prompt2 || ""), { posBefore, posAfter, prompt2 });

    // 4. A graded answer.
    await submitAnswer(page, c, "Bravo");
    const graded = await reviewFacts(page);
    out.graded = Object.assign({}, graded, { mainText: undefined });
    await shot(page, c, "4-review-graded");
    check(`${tag}: graded why is one plain sentence (${graded.verdict})`, PLAIN.includes(graded.why) && !RUBRIC.test(graded.mainText), graded.why);
    check(`${tag}: graded review shows the current card only`, graded.scrollback === 0 && graded.upcoming === 0 && !/NATO alphabet: (A|C)\b/.test(graded.mainText), { scrollback: graded.scrollback, upcoming: graded.upcoming });

    // 5. A long answer, then the end of the review vs the pinned bar.
    await Promise.all([page.waitForSelector("[data-answer-form]"), press(page, c, page.locator("[data-next]"))]);
    await settle(page);
    await submitAnswer(page, c, LONG);
    const long = await reviewFacts(page);
    await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
    await page.waitForTimeout(250);
    const clear = await page.evaluate(() => {
      const bar = document.querySelector(".judged-actions"), r = e => e.getBoundingClientRect();
      const fixed = getComputedStyle(bar).position === "fixed";
      const items = [...document.querySelectorAll(".session [data-review] > *, .session [data-review] .review-foot > *")]
        .filter(e => !e.classList.contains("judged-actions") && getComputedStyle(e).display !== "none" && r(e).height > 0 && !e.closest(".judged-actions"));
      const lastBottom = Math.max(...items.map(e => r(e).bottom));
      return { fixed, barTop: Math.round(r(bar).top), lastBottom: Math.round(lastBottom), scrollY: Math.round(scrollY), scrollH: document.documentElement.scrollHeight, vh: innerHeight };
    });
    out.long = clear;
    await shot(page, c, "5-review-long-scrolled-to-end");
    if (c.mobile) {
      check(`${tag}: long review scrolls and its end clears the pinned Next card bar`, clear.fixed && clear.scrollH > clear.vh && clear.lastBottom <= clear.barTop, clear);
    } else {
      check(`${tag}: desktop Next card sits in the flow (not pinned)`, !clear.fixed, clear);
    }
    check(`${tag}: long review still shows the current card only`, long.scrollback === 0 && long.upcoming === 0, { scrollback: long.scrollback, upcoming: long.upcoming });

    // Leave this review pending for --stale; keep the learner's state.
    fs.writeFileSync(path.join(opt.stateDir, `state-${opt.label}-${c.name}.json`), JSON.stringify(await context.storageState()));
  } catch (err) {
    check(`${tag}: walkthrough completed`, false, String(err).slice(0, 400));
  } finally {
    await context.close();
    if (record) {
      const vids = fs.readdirSync(videoDir).filter(f => f.endsWith(".webm"));
      if (vids.length && opt.shots) fs.copyFileSync(path.join(videoDir, vids[0]), path.join(opt.shots, `${opt.label}-iphone-light-walkthrough.webm`));
    }
    await browser.close();
  }
  out.health = health;
  check(`${tag}: 0 console errors, page errors, failed requests, 5xx, CSP violations`,
    !health.console.length && !health.pageErrors.length && !health.failed.length && !health.http5xx.length && !health.csp.length, health);
  return out;
}

async function runStale(c) {
  const health = { console: [], pageErrors: [], failed: [], http5xx: [], csp: [], tooling: [], aborted: [] };
  const file = path.join(opt.stateDir, `state-${opt.label}-${c.name}.json`);
  if (!fs.existsSync(file)) { check(`${opt.label} ${c.name}: stale state exists`, false, file); return {}; }
  const browser = await launch(c.engine);
  const context = await browser.newContext(Object.assign(contextOptions(c), { storageState: JSON.parse(fs.readFileSync(file, "utf8")) }));
  await instrument(context);
  const page = await context.newPage();
  watch(page, health);
  const tag = `${opt.label} ${c.name} (reopened later)`;
  await page.goto(base + "/study", { waitUntil: "load" });
  await settle(page);
  const f = await reviewFacts(page);
  await page.locator("[data-telemetry]").scrollIntoViewIfNeeded().catch(() => {});
  await shot(page, c, "6-review-reopened-20h-later");
  await context.close(); await browser.close();
  const out = { config: c.name, nextDate: f.nextDate, nextRel: f.nextRel, interval: f.interval, from: f.intervalFrom, telemetry: f.telemetry, health };
  check(`${tag}: the pending review is still on screen`, !!f.prompt, f.prompt);
  check(`${tag}: relative time counts down on the interval's clock (hours, not "today" beside "1 day")`, /^in \d+ hours?$/.test(f.nextRel || "") && f.interval === "1 day" && !/today/.test(f.telemetry || ""), out);
  check(`${tag}: 0 console errors, page errors, failed requests, 5xx, CSP violations`,
    !health.console.length && !health.pageErrors.length && !health.failed.length && !health.http5xx.length && !health.csp.length, health);
  return out;
}

const runs = [];
for (const c of CONFIGS) runs.push(opt.stale ? await runStale(c) : await runConfig(c));
const failed = results.filter(r => !r.ok);
const summary = { label: opt.label, url: base, stale: opt.stale, passed: results.length - failed.length, failed: failed.length, checks: results, runs };
console.log(JSON.stringify(summary, null, 2));
process.exit(opt.expect === "pass" && failed.length ? 1 : 0);
