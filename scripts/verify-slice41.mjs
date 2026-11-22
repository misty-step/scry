#!/usr/bin/env node
// verify-slice41.mjs: check the ten slice-41 changes on a running Scry, in a
// real headless Chromium, the way a learner meets them.
//
//   SCRY_MODE=test SCRY_TEST_JUDGMENTS=right,missed,close PORT=18080 ./bin/scry &
//   node scripts/verify-slice41.mjs --url http://127.0.0.1:18080 [--evidence ~/chief-evidence/media]
//
// It needs a test-mode server (local sign-up, the test card processor and
// the scripted test judge handing out right, missed and close verdicts); it
// refuses a live one. For a fresh learner at desktop 1440x900 and phone
// 390x844, each in light and dark, it asserts:
//   1. no answer on the Study screen before it is given (NATO: no "Alpha",
//      "Alfa" or "India" in the rail, header, title or page);
//   2. a correct, an incorrect and an almost correct answer each stop on the
//      post-answer review (no auto-skip): both answers, the verdict in
//      words, Ask Why?, the stored schedule (next review, interval,
//      stability, difficulty, review count), an inline edit that is saved and
//      persisted, and Next card by button, Enter and Space;
//   3. Study with an empty library is its own screen with two ways forward;
//   4. Study and Create are told apart in the navigation and on the page;
//   5. adding a starter set far down the shelf confirms in place: no page
//      load, window.scrollY unchanged;
//   6. the first row of the shelf holds no Creeds;
//   7. the card step and Top up show a processing state while they submit;
//   8. Settings says "API Tokens" with no can_add checkbox; Inbox, Library,
//      Billing and Settings each say what they are for;
// and on every screen: 0 CSP violations, 0 console errors, 0 page errors,
// 0 failed requests, 0 px of sideways scroll, text contrast >= 4.5:1 and,
// on the phone, 44 px touch targets.
// With --evidence <dir> it also writes the screenshots and a recorded
// walkthrough video (slice41-walkthrough.webm) there. Prints one JSON
// summary on stdout; exits 0 only when every check passes.

import fs from "node:fs";
import path from "node:path";
import { chromium } from "playwright-core";

const args = process.argv.slice(2);
const opt = { url: process.env.SCRY_VERIFY_URL || "", evidence: "" };
for (let i = 0; i < args.length; i++) {
  if (args[i] === "--url") opt.url = args[++i];
  else if (args[i] === "--evidence") opt.evidence = path.resolve(args[++i]);
  else { console.error("usage: verify-slice41.mjs --url <base-url> [--evidence dir]"); process.exit(2); }
}
if (!opt.url) { console.error("--url is required"); process.exit(2); }
const base = opt.url.replace(/\/+$/, "");
if (opt.evidence) fs.mkdirSync(opt.evidence, { recursive: true });

const summary = { ok: false, url: base, startedAt: new Date().toISOString(), configs: [], checks: [], failures: [], minContrast: {}, screenshots: [] };
function check(name, ok, detail) {
  summary.checks.push(detail === undefined || ok ? { name, ok } : { name, ok, detail });
  if (!ok) summary.failures.push(detail === undefined ? name : name + ": " + (typeof detail === "string" ? detail : JSON.stringify(detail)));
  return ok;
}
const pause = ms => new Promise(r => setTimeout(r, ms));
const NATO = ["Alfa", "Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel", "India", "Juliett", "Kilo", "Lima", "Mike",
  "November", "Oscar", "Papa", "Quebec", "Romeo", "Sierra", "Tango", "Uniform", "Victor", "Whiskey", "Xray", "X-ray", "Yankee", "Zulu"];
const shows = (text, w) => new RegExp("(^|[^\\p{L}\\p{N}])" + w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + "($|[^\\p{L}\\p{N}])", "iu").test(text);
const CONFIGS = [
  { name: "desktop-light", width: 1440, height: 900, scheme: "light" },
  { name: "desktop-dark", width: 1440, height: 900, scheme: "dark" },
  { name: "mobile-light", width: 390, height: 844, scheme: "light", mobile: true },
  { name: "mobile-dark", width: 390, height: 844, scheme: "dark", mobile: true },
];

async function newContext(browser, c, extra = {}) {
  const context = await browser.newContext({
    viewport: { width: c.width, height: c.height }, colorScheme: c.scheme, isMobile: !!c.mobile, hasTouch: !!c.mobile,
    deviceScaleFactor: c.mobile ? 2 : 1, ...extra,
  });
  context.setDefaultTimeout(20000);
  await context.addInitScript(() => {
    window.__csp = [];
    document.addEventListener("securitypolicyviolation", e => window.__csp.push((e.effectiveDirective || e.violatedDirective) + " " + e.blockedURI), true);
  });
  return context;
}

// watch records every way a page can fail quietly.
function watch(page, problems) {
  page.on("console", m => { if (m.type() === "error") problems.push("console.error: " + m.text().slice(0, 200)); });
  page.on("pageerror", e => problems.push("pageerror: " + String(e.message).slice(0, 200)));
  page.on("requestfailed", r => {
    const err = r.failure()?.errorText || "failed";
    if (err.includes("ERR_ABORTED")) return; // a navigation the page itself replaced, or the held test request
    problems.push("request failed: " + r.method() + " " + new URL(r.url()).pathname + " " + err);
  });
  page.on("response", r => { if (r.status() >= 400) problems.push("http " + r.status() + " " + r.request().method() + " " + new URL(r.url()).pathname); });
}

// audit checks one screen: sideways scroll, CSP, contrast, and touch targets on the phone.
async function audit(page, c, label) {
  await page.evaluate(() => document.fonts.ready);
  const a = await page.evaluate(mobile => {
    const parse = s => { const m = (s.match(/[\d.]+%?/g) || []).map((x, i) => x.endsWith("%") ? parseFloat(x) / 100 * (i < 3 ? 255 : 1) : parseFloat(x)); return { r: m[0], g: m[1], b: m[2], a: m.length > 3 ? m[3] : 1 }; };
    const over = (fg, bg) => ({ r: fg.r * fg.a + bg.r * (1 - fg.a), g: fg.g * fg.a + bg.g * (1 - fg.a), b: fg.b * fg.a + bg.b * (1 - fg.a), a: 1 });
    const lum = c => { const f = x => { x /= 255; return x <= .04045 ? x / 12.92 : ((x + .055) / 1.055) ** 2.4; }; return .2126 * f(c.r) + .7152 * f(c.g) + .0722 * f(c.b); };
    const background = el => {
      const layers = [];
      for (let e = el; e; e = e.parentElement) { const b = parse(getComputedStyle(e).backgroundColor); if (b.a > 0) { layers.push(b); if (b.a >= 1) break; } }
      let bg = parse(getComputedStyle(document.documentElement).colorScheme.includes("dark") || matchMedia("(prefers-color-scheme: dark)").matches ? "rgb(18,16,26)" : "rgb(255,255,255)");
      for (const l of layers.reverse()) bg = over(l, bg);
      return bg;
    };
    let min = 99; const low = [];
    for (const el of document.querySelectorAll("header.top *, main *, footer *")) {
      if (!el.getClientRects().length || el.closest(".sr, .upcoming, [hidden], svg, .caret, template") || el.disabled) continue;
      const own = [...el.childNodes].some(n => n.nodeType === 3 && n.textContent.trim());
      if (!own) continue;
      const st = getComputedStyle(el);
      if (st.visibility === "hidden" || parseFloat(st.opacity) === 0) continue;
      const bg = background(el), fg = over(parse(st.color), bg);
      const a = lum(fg), b = lum(bg), ratio = (Math.max(a, b) + .05) / (Math.min(a, b) + .05);
      if (ratio < min) min = ratio;
      if (ratio < 4.5) low.push((el.className || el.tagName) + " " + ratio.toFixed(2) + " “" + el.textContent.trim().slice(0, 30) + "”");
    }
    const small = [];
    if (mobile) {
      for (const el of document.querySelectorAll("button, a.btn, .nav a, summary, a.link, .wordmark")) {
        const r = el.getBoundingClientRect();
        if (el.getClientRects().length && !el.closest("[hidden]") && r.height < 43.5) small.push((el.className || el.tagName) + " " + Math.round(r.height) + "px “" + el.textContent.trim().slice(0, 24) + "”");
      }
    }
    return { overflow: document.documentElement.scrollWidth - innerWidth, csp: window.__csp || [], min, low: low.slice(0, 5), small: small.slice(0, 5),
      fonts: [...new Set([...document.querySelectorAll("body, h1, .meta, button, input")].map(e => getComputedStyle(e).fontFamily.split(",")[0].replace(/"/g, "").trim()))] };
  }, !!c.mobile);
  const at = c.name + " " + label;
  check(at + ": 0 px sideways scroll", a.overflow <= 0, a.overflow + "px");
  check(at + ": 0 CSP violations", a.csp.length === 0, a.csp);
  check(at + ": text contrast >= 4.5:1", a.low.length === 0, a.low);
  check(at + ": Geist and Geist Mono only", a.fonts.every(f => f === "Geist" || f === "Geist Mono"), a.fonts);
  if (c.mobile) check(at + ": 44 px touch targets", a.small.length === 0, a.small);
  summary.minContrast[at] = Number(a.min.toFixed(2));
}

async function shot(page, name, full) {
  if (!opt.evidence) return;
  await page.waitForTimeout(350);
  const file = path.join(opt.evidence, name);
  await page.screenshot({ path: file, fullPage: !!full });
  summary.screenshots.push(file);
}

const text = (page, sel) => page.$eval(sel, e => e.textContent.replace(/\s+/g, " ").trim());
const signUp = async (page, tag) => {
  await page.goto(base + "/signup", { waitUntil: "load" });
  await page.fill("#email", "s41-" + tag + "-" + Date.now().toString(36) + Math.random().toString(36).slice(2, 6) + "@example.com");
  await page.fill("#password", "slice41-" + Math.random().toString(36).slice(2));
  await Promise.all([page.waitForURL(u => new URL(u).pathname !== "/signup"), page.click("main form button")]);
};
const readReview = page => page.evaluate(() => {
  const q = s => document.querySelector(s), t = s => (q(s)?.textContent || "").replace(/\s+/g, " ").trim();
  return { kind: q("[data-review]")?.dataset.kind || "", verdict: q("[data-verdict]")?.dataset.verdict || "", label: t("[data-verdict-label]"), icon: !!q("[data-verdict] svg[aria-hidden=true]"),
    why: t(".verdict .why"), prompt: t("[data-prompt]"), submitted: t("[data-submitted]"), canonical: t("[data-canonical]"), ask: t("[data-explain-label]"),
    next: t("[data-next]"), nextReview: t("[data-next-review]"), iso: q("[data-next-review] time")?.getAttribute("datetime") || "", interval: t("[data-interval]"),
    stability: t("[data-stability]"), difficulty: t("[data-difficulty]"), reviews: t("[data-reviews]"), total: q("[data-reviews]")?.dataset.reviewTotal || "",
    edit: t("[data-edit-toggle]"), asking: !!q("[data-answer-form]"), header: t("[data-study-header]"), struck: !!q(".struck-text"), body: q("main").innerText };
});
// spoilerFree checks the waiting card's screen for any NATO answer.
async function spoilerFree(page, c, label) {
  const seen = await page.evaluate(() => ({ rail: document.querySelector(".session-margin").innerText, header: document.querySelector("[data-study-header]").textContent.trim(),
    main: document.querySelector("main").innerText, title: document.title, top: document.querySelector("header.top").innerText, html: document.querySelector("main").innerHTML }));
  const found = NATO.filter(w => shows(seen.rail, w) || shows(seen.main, w) || shows(seen.title, w) || shows(seen.top, w));
  check(c.name + " " + label + ": no answer shown before answering (rail, header, title, page)", found.length === 0 && !/Alfa|India|Letters [A-Z]–/.test(seen.html), { found, rail: seen.rail.replace(/\s+/g, " ") });
  check(c.name + " " + label + ": the rail names the topic and a neutral set", /^NATO phonetic alphabet · Set [123]$/.test(seen.header), seen.header);
}
// answerFor answers cards until the scripted judge hands out the wanted verdict.
async function answerFor(page, c, wanted, answers) {
  for (let tries = 0; tries < 4; tries++) {
    await page.waitForSelector("[data-answer-form]");
    await spoilerFree(page, c, "card before a " + wanted + " answer");
    const typed = answers[wanted];
    await page.fill("#answer", typed);
    await page.keyboard.press("Enter");
    await page.waitForSelector("[data-review]");
    const r = await readReview(page);
    r.typed = typed;
    if (r.verdict === wanted) return r;
    await page.click("[data-next]");
  }
  check(c.name + ": the test judge handed out a " + wanted + " verdict", false, "start the server with SCRY_TEST_JUDGMENTS=right,missed,close");
  return null;
}
// busyState submits form with its request held back, and reads the state from inside the page.
async function busyState(page, formSel, postPath) {
  let posts = 0;
  const hold = async route => { if (route.request().method() === "POST") { posts++; await pause(900); } await route.continue(); };
  await page.route(base + postPath, hold);
  await page.$eval(formSel, f => f.addEventListener("submit", () => setTimeout(() => {
    const b = f.querySelector("button:not([type=button])"), box = f.querySelector("[data-busy]"), again = new Event("submit", { cancelable: true });
    f.dispatchEvent(again);
    sessionStorage.setItem("busy", JSON.stringify({ disabled: b.disabled, aria: b.getAttribute("aria-busy"), form: f.getAttribute("aria-busy"), shown: !box.hidden && box.getClientRects().length > 0,
      text: box.textContent.replace(/\s+/g, " ").trim(), spinner: !!box.querySelector("svg.icon-spinner"), bar: !!box.querySelector(".busy-bar"), status: !!box.querySelector("[role=status]"), second: again.defaultPrevented }));
  }, 200)));
  await Promise.all([page.waitForNavigation({ waitUntil: "load" }), page.click(formSel + " button:not([type=button])")]);
  await page.unroute(base + postPath, hold);
  const state = JSON.parse(await page.evaluate(() => sessionStorage.getItem("busy")) || "{}");
  state.posts = posts;
  return state;
}

async function run(browser, c) {
  const problems = [];
  const context = await newContext(browser, c);
  const page = await context.newPage();
  watch(page, problems);
  const n = c.name;
  try {
    await signUp(page, n);
    // 3. Empty library: pressing Study.
    await Promise.all([page.waitForNavigation({ waitUntil: "load" }), page.click(".nav a[href='/study']")]);
    const empty = await page.evaluate(() => ({ path: location.pathname, head: document.querySelector("[data-study-empty] h1")?.textContent || "", line: document.querySelector("[data-study-empty] .lede")?.textContent || "",
      ctas: [...document.querySelectorAll("[data-study-empty] .actions a")].map(a => [a.textContent.trim(), a.getAttribute("href")]) }));
    check(n + " (3) Study with an empty library stays on /study and says so", empty.path === "/study" && empty.head === "No cards to study yet." && empty.line.includes("Your library is empty"), empty);
    check(n + " (3) empty Study offers a starter set and Create", JSON.stringify(empty.ctas) === JSON.stringify([["Pick a starter set", "/shelf"], ["Create cards with AI", "/learn"]]), empty.ctas);
    // 4. Study and Create.
    const nav = await page.$$eval(".nav a", as => as.map(a => ({ label: a.querySelector("span")?.textContent || "", href: a.getAttribute("href"), icon: a.querySelector("svg[aria-hidden=true]")?.getAttribute("class") || "", sw: a.querySelector("svg")?.getAttribute("stroke-width"), stroke: a.querySelector("svg")?.getAttribute("stroke") })));
    check(n + " (4) navigation reads Study, Library, Inbox, Create, Billing, Settings", nav.map(x => x.label).join(",") === "Study,Library,Inbox,Create,Billing,Settings" && nav[3].href === "/learn", nav.map(x => x.label));
    check(n + " (4, 9) every destination has a line icon (currentColor, 1.75) and Study's differs from Create's", nav.every(x => x.icon.includes("icon-") && x.sw === "1.75" && x.stroke === "currentColor") && nav[0].icon !== nav[3].icon, nav);
    await audit(page, c, "empty Study");
    if (n === "desktop-light") await shot(page, "03-study-empty-desktop.png");

    // 6, 9. The shelf.
    await Promise.all([page.waitForNavigation({ waitUntil: "load" }), page.click("[data-cta=starter]")]);
    const shelf = await page.evaluate(() => {
      const tiles = [...document.querySelectorAll("[data-tile]")].map(t => ({ id: t.id, top: Math.round(t.getBoundingClientRect().top), icons: t.querySelectorAll("svg[aria-hidden=true]").length,
        subject: t.querySelector(".tile-title svg")?.getAttribute("class") || "", count: t.querySelector(".tile-count .num")?.textContent || "", tnum: getComputedStyle(t.querySelector(".tile-count .num")).fontVariantNumeric }));
      return { path: location.pathname, head: document.querySelector(".shelf-head h1").textContent.trim(), tiles, barHidden: document.querySelector("[data-shelf-bar]").hidden };
    });
    const row1 = shelf.tiles.filter(t => t.top === shelf.tiles[0].top).map(t => t.id);
    check(n + " (9) the shelf headline is concrete", shelf.path === "/shelf" && shelf.head === "Pick a deck and start recalling in seconds.", shelf.head);
    check(n + " (6) row 1 holds no Creeds and leads with the broad sets", !row1.some(id => /creed/.test(id)) && shelf.tiles.slice(0, 4).map(t => t.id).join(",") === "nato-alphabet,mental-models,kitchen-numbers,world-geography", { row1, first4: shelf.tiles.slice(0, 4).map(t => t.id) });
    check(n + " (6) the Creeds are kept, lower on the shelf", shelf.tiles.length === 30 && shelf.tiles.findIndex(t => t.id === "the-creeds") >= 20, shelf.tiles.findIndex(t => t.id === "the-creeds"));
    check(n + " (9) every tile has its subject icon and a card-stack count in tabular figures", shelf.tiles.every(t => t.icons >= 3 && /icon-[a-z]+/.test(t.subject) && /^\d+$/.test(t.count) && t.tnum.includes("tabular-nums")) &&
      shelf.tiles[0].subject.includes("icon-radio") && shelf.tiles[3].subject.includes("icon-globe"), shelf.tiles.filter(t => t.icons < 3).map(t => t.id));
    // 5. Adding in place: the first tile, then one far down the shelf.
    await page.evaluate(() => { window.__sameDocument = true; });
    await page.click("#nato-alphabet [data-add] button");
    await page.waitForSelector("#nato-alphabet [data-added]");
    await audit(page, c, "shelf");
    if (n === "desktop-light") await shot(page, "01-shelf-desktop-light.png");
    if (n === "desktop-dark") await shot(page, "02-shelf-desktop-dark.png");
    if (n === "mobile-light") await shot(page, "10-mobile-shelf-390.png");
    await page.$eval("#great-books", e => e.scrollIntoView({ block: "center" }));
    await page.waitForTimeout(150);
    const before = await page.evaluate(() => scrollY);
    await page.click("#great-books [data-add] button");
    await page.waitForSelector("#great-books [data-added]");
    await page.waitForTimeout(300);
    const added = await page.evaluate(() => ({ y: scrollY, same: window.__sameDocument === true, url: location.pathname + location.search + location.hash, added: document.querySelector("#great-books").classList.contains("is-added"),
      tag: document.querySelector("#great-books [data-added]").textContent.trim(), check: !!document.querySelector("#great-books [data-added] svg.icon-check"), form: !!document.querySelector("#great-books form"),
      outline: getComputedStyle(document.querySelector("#great-books")).outlineStyle, bar: !document.querySelector("[data-shelf-bar]").hidden, summary: document.querySelector("[data-shelf-summary]").textContent.trim(),
      cta: (() => { const a = document.querySelector("[data-shelf-bar] a"), r = a.getBoundingClientRect(); return { text: a.textContent.trim(), onScreen: r.top >= 0 && r.bottom <= innerHeight + 1 }; })() }));
    check(n + " (5) the tile is below the fold when added", before > 300, before);
    check(n + " (5) + Add confirms in place: same document, same URL, window.scrollY unchanged (" + before + " -> " + added.y + ")", added.same && added.url === "/shelf" && added.y === before && added.y !== 0, added);
    check(n + " (5) the tile reads Added with a check and an accent outline", added.added && added.tag === "Added" && added.check && !added.form && added.outline === "solid", added);
    check(n + " (5) the Start studying bar is on screen without scrolling", added.bar && added.summary === "2 sets added · 50 cards" && added.cta.text === "Start studying" && added.cta.onScreen, added);

    // 1, 2. Study.
    await Promise.all([page.waitForNavigation({ waitUntil: "load" }), page.click("[data-shelf-bar] a")]);
    check(n + " (4) Study says what it is for", (await text(page, ".margin-purpose")) === "Recall what's due today" && new URL(page.url()).pathname === "/study");
    await page.waitForSelector("[data-answer-form]");
    await spoilerFree(page, c, "first card");
    await audit(page, c, "Study, card waiting");
    if (n === "desktop-light") await shot(page, "04-study-active-spoilerfree.png");

    const answers = { right: "Alfa", missed: "Bananas", close: "Charly" };
    const labels = { right: "Correct", missed: "Incorrect", close: "Almost Correct" };
    for (const wanted of ["right", "missed", "close"]) {
      const r = await answerFor(page, c, wanted, answers);
      if (!r) continue;
      const at = n + " (2) " + wanted;
      check(at + ": the answer stops on the post-answer review", r.kind === wanted && !r.asking, { kind: r.kind, asking: r.asking });
      check(at + ": verdict badge \"" + labels[wanted] + "\" with an icon", r.label === labels[wanted] && r.icon, r.label);
      check(at + ": shows the submitted answer beside the correct one", r.submitted === r.typed && r.canonical.length > 0 && (wanted === "right" ? r.canonical === "Alfa" : r.canonical !== r.typed) && (wanted !== "missed" || r.struck), { submitted: r.submitted, canonical: r.canonical });
      check(at + ": the one-line why", r.why.length > 10, r.why);
      check(at + ": Ask Why?, Edit card and Next card are offered", r.ask === "Ask Why?" && r.edit === "Edit card" && r.next === "Next card", { ask: r.ask, edit: r.edit, next: r.next });
      const shown = r.iso ? new Date(r.iso + "T00:00:00Z").toLocaleDateString("en-US", { month: "short", day: "numeric", timeZone: "UTC" }) : "?";
      const days = Math.round((new Date(r.iso + "T00:00:00Z") - new Date(new Date().toISOString().slice(0, 10) + "T00:00:00Z")) / 864e5);
      check(at + ": next review date and relative time (" + r.nextReview + ")", r.nextReview === shown + " · " + (days === 1 ? "tomorrow" : "in " + days + " days") && days >= 1, { nextReview: r.nextReview, iso: r.iso });
      check(at + ": interval, stability and difficulty (" + [r.interval, r.stability, r.difficulty].join(", ") + ")", r.interval === (days === 1 ? "1 day" : days + " days") && /^\d+\.\d days$/.test(r.stability) && /^\d+\.\d \/ 10$/.test(r.difficulty));
      const split = { right: "1 · 1 correct · 0 almost · 0 incorrect", missed: "1 · 0 correct · 0 almost · 1 incorrect", close: "1 · 0 correct · 1 almost · 0 incorrect" }[wanted];
      check(at + ": review count and history (" + r.reviews + ")", r.reviews === split && r.total === "1", r.reviews);
      check(at + ": facts only, no game mechanics", !/streak|points|confetti|level up|🔥|🎉/i.test(r.body));
      check(at + ": the real place shows once answered", /^NATO phonetic alphabet · Letters [A-Z]–[A-Z]$/.test(r.header), r.header);
      await page.waitForTimeout(700);
      const still = await readReview(page);
      check(at + ": nothing auto-advances", still.kind === wanted && still.prompt === r.prompt);
      // Ask Why?: only when pressed; the verdict stands.
      await page.click("[data-explain]");
      await page.waitForFunction(() => { const e = document.querySelector("[data-explanation]"); return e && !e.hidden && e.textContent.length > 0; });
      const explained = await text(page, "[data-explanation]");
      const afterWhy = await readReview(page);
      check(at + ": Ask Why? explains in place and changes nothing", explained.startsWith(wanted === "right" ? "Why it holds:" : "What differed:") && afterWhy.label === r.label && afterWhy.reviews === r.reviews && afterWhy.nextReview === r.nextReview, explained);
      await audit(page, c, wanted + " review");
      if (wanted === "right") {
        if (n === "desktop-light") await shot(page, "05-study-review-correct.png", true);
        if (n === "mobile-light") await shot(page, "11-mobile-review-390.png");
        await page.click("[data-next]"); // by the button
      }
      if (wanted === "missed") {
        // Inline edit: saved in place, persisted, the grade and schedule stand.
        await page.click("[data-edit-toggle]");
        const edited = { prompt: r.prompt + " (code word)", answer: r.canonical + " (as in applause)" };
        await page.fill("#edit-prompt", edited.prompt);
        await page.fill("#edit-answer", edited.answer);
        const place = () => page.$eval("[data-edit-save]", b => Math.round(b.getBoundingClientRect().top));
        await page.$eval("[data-edit-save]", b => b.scrollIntoView({ block: "center" }));
        const y = await place();
        await page.click("[data-edit-save]");
        await page.waitForSelector("[data-edit-status]:not([hidden])");
        const saved = await readReview(page);
        const y2 = await place();
        check(at + ": inline edit saves in place with \"Saved\"", (await text(page, "[data-edit-status]")) === "Saved" && saved.canonical === edited.answer && saved.prompt === edited.prompt && new URL(page.url()).pathname === "/study", saved.canonical);
        check(at + ": the edit keeps the learner's place (" + y + " -> " + y2 + ")", Math.abs(y2 - y) <= 2);
        check(at + ": the edit changes neither the grade nor the schedule", saved.label === r.label && saved.reviews === r.reviews && saved.nextReview === r.nextReview && saved.interval === r.interval && saved.stability === r.stability);
        await audit(page, c, "missed review, edit open");
        if (n === "desktop-light") await shot(page, "06-study-review-missed-edit.png", true);
        await page.reload({ waitUntil: "load" });
        const kept = await readReview(page);
        check(at + ": the edit is persisted, and the review is still on screen after a reload", kept.canonical === edited.answer && kept.prompt === edited.prompt && kept.kind === "missed", kept.canonical);
        await page.goto(base + "/library?q=" + encodeURIComponent("as in applause"), { waitUntil: "load" });
        check(at + ": the edited card is in the library", (await page.$eval("main", m => m.innerText)).includes(edited.answer));
        await page.goto(base + "/study", { waitUntil: "load" });
        await page.evaluate(() => document.activeElement && document.activeElement.blur());
        await page.keyboard.press("Enter"); // by Enter
      }
      if (wanted === "close") {
        await page.evaluate(() => document.activeElement && document.activeElement.blur());
        await page.keyboard.press("Space"); // by Space
      }
      await page.waitForSelector("[data-answer-form]");
      check(at + ": Next card moves on to a new card", (await text(page, "#prompt")) !== r.prompt);
    }
    const scroll = await page.$$eval(".answered .tag", ts => ts.map(t => t.textContent.trim()));
    check(n + " (2) answered cards recede to one line each with a verdict tag", scroll.length >= 3 && scroll.every(t => ["correct", "almost correct", "incorrect"].includes(t)), scroll);

    // 4, 8. Create, Inbox, Library, Settings.
    await page.goto(base + "/learn", { waitUntil: "load" });
    check(n + " (4) Create: its name, icon and a four-word purpose", (await text(page, ".page-title")) === "Create" && !!(await page.$(".page-title svg.icon-create")) && (await text(page, ".page-head .purpose")) === "Generate cards with AI" && (await page.title()) === "Create · scry");
    await audit(page, c, "Create");
    if (n === "desktop-light") await shot(page, "07-create-desktop.png");
    await page.goto(base + "/inbox", { waitUntil: "load" });
    check(n + " (8) Inbox says what it is for", (await text(page, ".list-purpose")) === "Cards suggested by external scripts or tools wait here until you approve them.");
    await audit(page, c, "Inbox");
    await page.goto(base + "/library", { waitUntil: "load" });
    check(n + " (8) Library says what it is for, and its sections are neutral", (await text(page, ".page-head .purpose")) === "Every card you have, grouped by idea." && (await text(page, ".outline")).includes("Letters A–I") && !/Alfa to India/.test(await text(page, ".outline")));
    await audit(page, c, "Library");
    await page.goto(base + "/settings", { waitUntil: "load" });
    const set = await page.evaluate(() => ({ titles: [...document.querySelectorAll(".setting h2")].map(h => h.textContent.trim()), canAdd: document.querySelectorAll("[name=can_add]").length, boxes: document.querySelectorAll("#api-tokens input[type=checkbox]").length,
      fields: [...document.querySelectorAll("#api-tokens form[action='/settings/tokens'] input:not([type=hidden])")].map(i => i.name), body: document.querySelector("main").innerText }));
    check(n + " (8) Settings says \"API Tokens\", with no can_add checkbox", set.titles.includes("API Tokens") && !set.titles.includes("Agents") && set.canAdd === 0 && set.boxes === 0 && set.fields.join() === "name" && !/May add straight|without the inbox/i.test(set.body), set.titles);
    check(n + " (8) Settings: API access and the study profile in plain words", set.body.includes("Programmatic access to your account.") && set.titles.includes("Study profile") && !set.titles.includes("About you") && set.body.includes("Background and goals used to tailor AI-generated cards."));
    await audit(page, c, "Settings");
    if (n === "desktop-light") { await page.$eval("#api-tokens", e => e.scrollIntoView({ block: "start" })); await shot(page, "08-settings-api-tokens.png"); }

    // 7. The card step and Top up, while they submit.
    await page.goto(base + "/signup/card", { waitUntil: "load" });
    check(n + " (7) no processing state before the card is submitted", await page.$eval("[data-busy]", e => e.hidden));
    await audit(page, c, "card step");
    await page.fill("#number", "4242 4242 4242 4242"); await page.fill("#expiry", "12/30"); await page.fill("#cvc", "123");
    if (n === "desktop-light" && opt.evidence) {
      // For the picture only: the request is cancelled, so the page stays as it is while it waits.
      const cancel = route => route.request().method() === "POST" ? route.abort("aborted") : route.continue();
      await page.route(base + "/signup/card", cancel);
      await page.click("[data-card-form] button", { noWaitAfter: true });
      await page.waitForSelector("[data-busy]:not([hidden])");
      await shot(page, "09-card-loading-state.png");
      await page.unroute(base + "/signup/card", cancel);
      await page.reload({ waitUntil: "load" });
      await page.fill("#number", "4242 4242 4242 4242"); await page.fill("#expiry", "12/30"); await page.fill("#cvc", "123");
    }
    const card = await busyState(page, "[data-card-form]", "/signup/card");
    check(n + " (7) the card step shows it is working: disabled, busy, words, spinner, progress", card.disabled && card.aria === "true" && card.form === "true" && card.shown && /^Confirming your card( with Stripe)?…$/.test(card.text) && card.spinner && card.bar && card.status, card);
    check(n + " (7) a second submit while busy is refused: one request", card.second && card.posts === 1, card);
    check(n + " (7) the card is accepted", new URL(page.url()).pathname !== "/signup/card", page.url());
    await page.goto(base + "/billing", { waitUntil: "load" });
    check(n + " (8) Billing says what it is for", (await text(page, ".ledger .purpose")) === "Your plan, what you have used and what you have been charged.");
    await audit(page, c, "Billing");
    const topup = await busyState(page, "form.topup", "/billing/topup");
    check(n + " (7) Top up shows \"Updating billing…\" while it submits, once", topup.disabled && topup.aria === "true" && topup.shown && topup.text === "Updating billing…" && topup.spinner && topup.bar && topup.second && topup.posts === 1, topup);
    check(n + " (7) the top-up is recorded once", (await page.$$eval(".history td", tds => tds.filter(t => t.textContent.trim() === "Top-up").length)) === 1);
  } catch (err) {
    check(n + ": the walk completed", false, String(err.stack || err).split("\n").slice(0, 3).join(" | "));
  } finally {
    check(n + ": 0 console errors, page errors, failed requests or 4xx/5xx responses", problems.length === 0, problems);
    summary.configs.push({ name: n, viewport: c.width + "x" + c.height, scheme: c.scheme, problems });
    await context.close();
  }
}

// declined checks that a refused card gives the form back with the reason.
async function declined(browser) {
  const c = CONFIGS[0];
  const context = await newContext(browser, c);
  const page = await context.newPage();
  try {
    await signUp(page, "declined");
    await page.goto(base + "/signup/card", { waitUntil: "load" });
    await page.fill("#number", "4000 0000 0000 0002"); await page.fill("#expiry", "12/30"); await page.fill("#cvc", "123");
    await Promise.all([page.waitForNavigation({ waitUntil: "load" }), page.click("[data-card-form] button")]);
    const back = await page.evaluate(() => { const f = document.querySelector("[data-card-form]"); return f ? { path: location.pathname, busy: f.classList.contains("is-busy"), hidden: f.querySelector("[data-busy]").hidden, disabled: f.querySelector("button").disabled, error: f.querySelector("[role=alert]")?.textContent.trim() || "", field: !!f.querySelector("#number:not([disabled])") } : { path: location.pathname }; });
    check("(7) a declined card gives the form back, interactive, with the reason", back.path === "/signup/card" && back.busy === false && back.hidden && back.disabled === false && back.error.length > 5 && back.field, back);
  } catch (err) {
    check("(7) declined card", false, String(err.message || err).split("\n")[0]);
  } finally { await context.close(); }
}

// walkthrough records the video: Empty Study -> shelf in-place add -> spoiler-free Study ->
// post-answer review, Ask Why?, inline edit, Next card -> Create -> Settings API Tokens.
async function walkthrough(browser) {
  const c = { name: "walkthrough", width: 1280, height: 720, scheme: "light" };
  const dir = fs.mkdtempSync(path.join(opt.evidence, ".video-"));
  const context = await newContext(browser, c, { recordVideo: { dir, size: { width: 1280, height: 720 } } });
  const page = await context.newPage();
  const beat = (ms = 1100) => page.waitForTimeout(ms);
  const typeSlow = (sel, s) => page.locator(sel).pressSequentially(s, { delay: 70 });
  try {
    await signUp(page, "video"); await beat(600);
    await Promise.all([page.waitForNavigation(), page.click(".nav a[href='/study']")]); await beat(2200);        // empty Study
    await Promise.all([page.waitForNavigation(), page.click("[data-cta=starter]")]); await beat(1800);           // the shelf
    await page.click("#nato-alphabet [data-add] button"); await page.waitForSelector("#nato-alphabet [data-added]"); await beat(1500);
    await page.$eval("#great-books", e => e.scrollIntoView({ block: "center", behavior: "smooth" })); await beat(1700);
    await page.click("#great-books [data-add] button"); await page.waitForSelector("#great-books [data-added]"); await beat(2200); // in place
    await Promise.all([page.waitForNavigation(), page.click("[data-shelf-bar] a")]); await beat(2200);           // spoiler-free Study
    for (let i = 0, done = { right: false, missed: false }; i < 5 && !(done.right && done.missed); i++) {
      await page.waitForSelector("[data-answer-form]");
      await typeSlow("#answer", done.right ? "Bananas" : "Alfa"); await beat(500);
      await page.keyboard.press("Enter"); await page.waitForSelector("[data-review]"); await beat(2200);
      const verdict = await page.$eval("[data-verdict]", e => e.dataset.verdict);
      if (verdict === "right" && !done.right) {
        done.right = true;
        await page.click("[data-explain]"); await page.waitForSelector("[data-explanation]:not([hidden])"); await beat(2400);
      } else if (verdict === "missed" && !done.missed) {
        done.missed = true;
        await page.click("[data-explain]"); await page.waitForSelector("[data-explanation]:not([hidden])"); await beat(2000);
        await page.click("[data-edit-toggle]"); await beat(900);
        await page.fill("#edit-answer", ""); await typeSlow("#edit-answer", "Bravo (as in applause)"); await beat(500);
        await page.click("[data-edit-save]"); await page.waitForSelector("[data-edit-status]:not([hidden])"); await beat(2400);
      }
      await page.$eval("[data-next]", b => b.scrollIntoView({ block: "center", behavior: "smooth" })); await beat(800);
      await page.click("[data-next]"); await beat(1500);                                                         // Next card
    }
    await Promise.all([page.waitForNavigation(), page.click(".nav a[href='/learn']")]); await beat(2200);        // Create
    await Promise.all([page.waitForNavigation(), page.click(".nav a[href='/settings']")]); await beat(900);
    await page.$eval("#api-tokens", e => e.scrollIntoView({ block: "start", behavior: "smooth" })); await beat(2600); // API Tokens
  } catch (err) {
    check("walkthrough video recorded", false, String(err.message || err).split("\n")[0]);
  }
  const video = page.video();
  await context.close();
  const file = path.join(opt.evidence, "slice41-walkthrough.webm");
  if (video) {
    fs.copyFileSync(await video.path(), file);
    fs.rmSync(dir, { recursive: true, force: true });
    const size = fs.statSync(file).size;
    summary.video = { file, bytes: size };
    check("walkthrough video recorded, at most 8 MiB", size > 50000 && size <= 8 * 1024 * 1024, size + " bytes");
  }
}

const health = await fetch(base + "/healthz").then(r => r.json()).catch(() => ({}));
summary.healthz = { status: health.status, mode: health.mode, build_sha: health.build_sha, grading: health.grading, explainer: health.explainer, auth_mode: health.auth_mode, billing_mode: health.billing_mode };
if (health.mode !== "test" || String(health.auth_mode || "").startsWith("clerk-")) {
  console.error("verify-slice41: needs a test-mode server with local sign-up; /healthz says " + JSON.stringify(summary.healthz));
  process.exit(2);
}
const browser = await chromium.launch({ executablePath: [process.env.CHROME_BIN, "/usr/bin/google-chrome", "/usr/bin/chromium"].find(p => p && fs.existsSync(p)), headless: true, args: ["--no-sandbox"] });
try {
  for (const c of CONFIGS) await run(browser, c);
  await declined(browser);
  if (opt.evidence) await walkthrough(browser);
} finally {
  await browser.close();
}
summary.ok = summary.failures.length === 0;
summary.passed = summary.checks.filter(c => c.ok).length;
summary.total = summary.checks.length;
summary.finishedAt = new Date().toISOString();
process.stdout.write(JSON.stringify(summary, null, 2) + "\n");
if (!summary.ok) {
  process.stderr.write("verify-slice41: " + summary.failures.length + " failure(s):\n" + summary.failures.map(f => "  - " + f).join("\n") + "\n");
  process.exit(1);
}
