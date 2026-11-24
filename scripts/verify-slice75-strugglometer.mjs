#!/usr/bin/env node
// verify-slice75-strugglometer.mjs: check Slice #75 (the Strugglometer:
// repeated-miss detection, bridge cards, holding and releasing a card) on
// the real running path, in real browsers, against isolated synthetic data.
//
//   make build        # or: go build -o bin/scry ./cmd/scry
//   node scripts/verify-slice75-strugglometer.mjs --bin ./bin/scry --label after \
//        [--shots ~/chief-evidence/media] [--traces DIR] [--only desktop-light,...] \
//        [--expect pass|observe] [--baseline] [--keep]
//
// For every config the script starts its own Scry (the --bin binary) in
// SCRY_MODE=test on a fresh SQLite file in a new temporary directory, with a
// minimal environment (no provider keys: model, speech and bridges are the
// test fixtures, auth and billing the local fakes, and nothing leaves the
// machine) and a fixed verdict script, SCRY_TEST_JUDGMENTS, so the journey
// is deterministic. It signs up a disposable learner through the UI, adds
// the NATO phonetic alphabet starter set, and seeds two cards' history in
// the same database file with node:sqlite (a synthetic past: card A has 3
// FSRS lapses; card B was missed once two days ago). Later "time travel"
// moves chosen cards' review times and due dates back together, as if days
// had passed, so the schedule's own invariants hold.
//
// Engines and viewports (each in light and dark):
//   iphone : WebKit, 390x844, touch, iPhone user agent, America/Chicago
//   desktop: Chromium (system Chrome), 1440x900, America/Chicago
// WebKit is Playwright's build (npx playwright@<playwright-core version>
// install webkit); Chromium is CHROME_BIN or /usr/bin/google-chrome.
//
// The journey (judged answers, in order: missed, right, missed, right,
// close, right, close):
//   1. Deck list and card detail: A (3 lapses) shows the cue "Repeated
//      miss — scaffold prerequisite bridges?" with a 16 px stroke=currentColor
//      icon; B (one miss) does not.
//   2. Study: B waits with no cue (no spoilers); B is missed again -> the
//      post-answer review shows the cue and "Build bridge cards".
//   3. Generation: the preview shows the 3 deterministic fixture cards;
//      one is edited, one removed in place; "Add bridges & hold parent"
//      adds 2 bridges and the review says "Held by 2 bridges".
//   4. Card A from its page: build, accept all 3, remove one bridge card
//      ("Held by 2 bridges"), then "Release hold" by hand.
//   5. The next session holds A (released, due) and B's and A's bridges,
//      never B (held). B's bridge 1 Incorrect -> B stays held by 2; bridge 2
//      Correct -> held by 1; later bridge 1 Almost Correct -> B is released
//      automatically with its schedule unchanged, and the next Study
//      session opens on B.
// Every page is checked for sideways scroll; on the phone each bridge panel
// and card is at least 366 px wide. Console errors, page errors, failed
// requests, HTTP >= 500 and CSP violations are counted per config (a
// WebKit screenshot's own injected <style> is set aside as tooling, as in
// verify-slice70.mjs). No Inter is loaded; only Scry's OFL Geist faces.
//
// --baseline runs only steps 1-2 and records whether the cue is there (for
// "before" screenshots against a build without the slice); with --expect
// observe the script always exits 0. --expect pass (default) exits 1 if any
// check fails.

import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { spawn } from "node:child_process";
import { DatabaseSync } from "node:sqlite";
import { chromium, webkit, devices } from "playwright-core";

const args = process.argv.slice(2);
const opt = { bin: "./bin/scry", label: "run", shots: "", traces: "", expect: "pass", baseline: false, only: "", keep: false, port: 18750 };
for (let i = 0; i < args.length; i++) {
  const a = args[i];
  const v = () => args[++i];
  if (a === "--bin") opt.bin = v();
  else if (a === "--label") opt.label = v();
  else if (a === "--shots") opt.shots = v();
  else if (a === "--traces") opt.traces = v();
  else if (a === "--expect") opt.expect = v();
  else if (a === "--baseline") opt.baseline = true;
  else if (a === "--only") opt.only = v();
  else if (a === "--keep") opt.keep = true;
  else if (a === "--port") opt.port = Number(v());
  else { console.error("unknown flag " + a); process.exit(2); }
}
opt.bin = path.resolve(opt.bin);
if (!fs.existsSync(opt.bin)) { console.error("no binary at " + opt.bin); process.exit(2); }
for (const d of [opt.shots, opt.traces]) if (d) fs.mkdirSync(d, { recursive: true });

const CONFIGS = [
  { name: "iphone-light", engine: "webkit", scheme: "light", mobile: true, journeyShots: true },
  { name: "iphone-dark", engine: "webkit", scheme: "dark", mobile: true, journeyShots: false },
  { name: "desktop-light", engine: "chromium", scheme: "light", mobile: false, journeyShots: true },
  { name: "desktop-dark", engine: "chromium", scheme: "dark", mobile: false, journeyShots: false },
].filter(c => !opt.only || opt.only.split(",").includes(c.name));

const JUDGMENTS = "missed,right,missed,right,close,right,close";
const CUE = "Repeated miss — scaffold prerequisite bridges?";
const GAME = /streak|points|level up|badge|confetti|🎉|🔥|achievement/i;

const results = [];
function check(name, ok, detail) {
  results.push({ name, ok: !!ok, detail });
  console.error((ok ? "PASS " : "FAIL ") + name + (ok ? "" : "  " + JSON.stringify(detail).slice(0, 600)));
}

// ---- the server ----------------------------------------------------------

async function startServer(c, port) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), `s75-${c.name}-`));
  const db = path.join(dir, "scry.db");
  const logPath = path.join(dir, "server.log");
  const log = fs.openSync(logPath, "w");
  // A minimal environment: no provider keys reach the app.
  const env = { PATH: process.env.PATH, HOME: dir, TMPDIR: dir, SCRY_MODE: "test", SCRY_DB: db, PORT: String(port),
    SCRY_TEST_JUDGMENTS: JUDGMENTS, BACKUP_DIR: path.join(dir, "backups") };
  const proc = spawn(opt.bin, [], { env, stdio: ["ignore", log, log], cwd: dir });
  const url = `http://127.0.0.1:${port}`;
  for (let i = 0; i < 100; i++) {
    try { const r = await fetch(url + "/healthz"); if (r.ok) { const h = await r.json(); return { proc, dir, db, url, logPath, health: h }; } } catch {}
    await new Promise(r => setTimeout(r, 100));
  }
  proc.kill("SIGTERM");
  throw new Error("server did not become healthy; see " + logPath);
}

async function stopServer(s) {
  if (!s) return;
  await new Promise(resolve => { s.proc.once("exit", resolve); s.proc.kill("SIGTERM"); setTimeout(resolve, 5000); });
}

// ---- the synthetic database ----------------------------------------------

function openDB(file) {
  const db = new DatabaseSync(file);
  db.exec("PRAGMA busy_timeout = 5000");
  return db;
}

const nowS = () => Math.floor(Date.now() / 1000);
const DAY = 86400;

function seed(db, email) {
  const u = db.prepare("SELECT id FROM users WHERE email = ?").get(email);
  const items = db.prepare("SELECT id, prompt, idea_id FROM items WHERE user_id = ? AND deleted_at = 0").all(u.id);
  const byPrompt = Object.fromEntries(items.map(i => [i.prompt, i]));
  const A = byPrompt["NATO alphabet: A"], B = byPrompt["NATO alphabet: B"];
  const now = nowS();
  // Everything else waits a month, so each session holds only what the
  // journey studies.
  db.prepare("UPDATE items SET due_at = ? WHERE user_id = ?").run(now + 30 * DAY, u.id);
  const review = db.prepare(`INSERT INTO reviews (user_id, item_id, session_id, at, answer, verdict, reason, detail, via, judge, practice, graded_prompt, graded_expected)
    VALUES (?, ?, 0, ?, ?, ?, ?, '{}', 'typed', 'test-judge', 0, ?, ?)`);
  const why = { right: "Same meaning as the correct answer.", close: "Close — part of it is missing or imprecise.", missed: "Not quite — compare it with the correct answer above." };
  // A: six reviews, three of them lapses; its latest two are not both
  // misses, so only the lapse count makes it struggle.
  const aHist = [["Alpha", "right"], ["Able", "missed"], ["Alfa", "right"], ["Apple", "missed"], ["Ack", "missed"], ["Alfa", "right"]];
  aHist.forEach(([ans, v], i) => review.run(u.id, A.id, now - (20 - 3 * i) * DAY, ans, v, why[v], A.prompt, "Alfa"));
  db.prepare("UPDATE items SET reps = 6, lapses = 3, stability = 1.4, difficulty = 7.8, last_review = ?, due_at = ? WHERE id = ?")
    .run(now - 5 * DAY, now + 30 * DAY, A.id);
  // B: missed once two days ago, due since yesterday.
  review.run(u.id, B.id, now - 2 * DAY, "Beta", "missed", why.missed, B.prompt, "Bravo");
  db.prepare("UPDATE items SET reps = 1, lapses = 0, stability = 0.4, difficulty = 7.2, last_review = ?, due_at = ? WHERE id = ?")
    .run(now - 2 * DAY, now - DAY, B.id);
  return { uid: u.id, A, B, ideaId: A.idea_id };
}

// Time travel: the item's last review, due date and review history move
// back together, as if `days` had passed.
function travel(db, itemId, days) {
  const s = days * DAY;
  db.prepare("UPDATE items SET due_at = due_at - ?, last_review = CASE WHEN last_review > 0 THEN last_review - ? ELSE 0 END WHERE id = ?").run(s, s, itemId);
  db.prepare("UPDATE reviews SET at = at - ? WHERE item_id = ?").run(s, itemId);
}

const sched = (db, id) => db.prepare("SELECT due_at, stability, difficulty, reps, lapses, last_review FROM items WHERE id = ?").get(id);
const heldBy = (db, id) => db.prepare(`SELECT COUNT(*) n FROM item_bridges b JOIN items i ON i.id = b.bridge_item_id
  JOIN ideas d ON d.id = i.idea_id WHERE b.parent_item_id = ? AND b.ended_at = 0 AND i.deleted_at = 0 AND d.deleted_at = 0`).get(id).n;
const bridgesOf = db => (id) => db.prepare(`SELECT b.bridge_item_id id, i.prompt, b.ended_at, b.end_reason FROM item_bridges b JOIN items i ON i.id = b.bridge_item_id
  WHERE b.parent_item_id = ? ORDER BY b.id`).all(id);
const sessionItems = (db, uid) => db.prepare(`SELECT si.item_id FROM session_items si JOIN study_sessions s ON s.id = si.session_id
  WHERE s.user_id = ? AND s.ended_at = 0 ORDER BY si.position`).all(uid).map(r => r.item_id);

// ---- the browser ---------------------------------------------------------

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
  return chromium.launch({ executablePath: process.env.CHROME_BIN || "/usr/bin/google-chrome" });
}

function watch(page, health) {
  page.__health = health;
  page.on("console", m => {
    if (m.text().startsWith("S75-CSP ")) health.csp.push(m.text().slice(8));
    else if (m.type() === "error") health.console.push(m.text().slice(0, 200));
  });
  page.on("pageerror", e => health.pageErrors.push(String(e).slice(0, 200)));
  page.on("requestfailed", r => {
    const why = (r.failure() || {}).errorText || "";
    if (/ERR_ABORTED|cancelled/i.test(why) && /\/static\/icon\.svg$|favicon/.test(r.url())) health.aborted.push(r.url() + " " + why);
    else health.failed.push(r.url() + " " + why);
  });
  page.on("response", r => {
    if (r.status() >= 500) health.http5xx.push(r.status() + " " + r.url());
    if (/\/static\/fonts\//.test(r.url())) health.fonts.add(new URL(r.url()).pathname);
  });
}

async function instrument(context) {
  await context.addInitScript(() => {
    document.addEventListener("securitypolicyviolation", e => {
      console.log("S75-CSP " + e.violatedDirective + " " + (e.blockedURI || "") + " " + location.pathname);
    }, true);
  });
}

let shotCount = 0;
async function shot(page, c, name, opts = {}) {
  if (!opt.shots) return;
  const file = path.join(opt.shots, `s75-${opt.label}-${c.name}-${name}.png`);
  const h = page.__health;
  const n = { console: h.console.length, csp: h.csp.length };
  await page.screenshot({ path: file, caret: "initial", fullPage: opts.fullPage !== false, scale: "css" });
  await page.waitForTimeout(150);
  h.tooling.push(...h.console.splice(n.console).map(x => "during screenshot " + name + ": " + x));
  h.tooling.push(...h.csp.splice(n.csp).map(x => "during screenshot " + name + ": csp " + x));
  shotCount++;
}

async function press(c, locator) {
  if (c.mobile) await locator.tap(); else await locator.click();
}

async function settle(page) {
  await page.waitForLoadState("load");
  await page.evaluate(() => document.fonts && document.fonts.ready);
  await page.waitForTimeout(350);
}

async function go(page, url) {
  await page.goto(url, { waitUntil: "load" });
  await settle(page);
}

// Layout and type facts measured in the page.
async function facts(page) {
  return page.evaluate(() => {
    const r = e => e.getBoundingClientRect();
    const q = s => document.querySelector(s), qa = s => [...document.querySelectorAll(s)];
    const iconFacts = qa(".struggle-line .icon, .row-cue .icon").map(svg => ({
      w: Math.round(r(svg).width), h: Math.round(r(svg).height), stroke: svg.getAttribute("stroke"), fill: svg.getAttribute("fill"),
      color: getComputedStyle(svg).color, textColor: getComputedStyle(svg.parentElement).color,
    }));
    const fam = getComputedStyle(document.body).fontFamily;
    const faces = [];
    if (document.fonts) document.fonts.forEach(f => { if (f.status === "loaded") faces.push(f.family); });
    return {
      vw: document.documentElement.clientWidth, scrollW: document.documentElement.scrollWidth,
      panels: qa(".struggle, .bridge-card, .bridge-problem").filter(e => r(e).height > 0).map(e => Math.round(r(e).width)),
      icons: iconFacts, fontFamily: fam, faces,
      text: q("main") ? q("main").innerText : "",
      cue: !!qa("[data-struggle-cue], [data-row-cue]").length,
    };
  });
}

function layoutChecks(tag, f, c, where) {
  check(`${tag}: ${where}: no sideways scroll (${f.scrollW} <= ${f.vw})`, f.scrollW <= f.vw, { scrollW: f.scrollW, vw: f.vw });
  if (c.mobile && f.panels.length) check(`${tag}: ${where}: every bridge panel/card is >= 366 px wide at 390 px`, f.panels.every(w => w >= 366), f.panels);
  if (f.icons.length) check(`${tag}: ${where}: cue icons are 16 px inline SVG, stroke=currentColor, no fill, in the color of their words`,
    f.icons.every(i => i.w === 16 && i.h === 16 && i.stroke === "currentColor" && i.fill === "none" && i.color === i.textColor), f.icons);
  check(`${tag}: ${where}: Geist (OFL) only, no Inter`, /Geist/.test(f.fontFamily) && !/\bInter\b/.test(f.fontFamily) && !f.faces.some(x => /\bInter\b/i.test(x)), { fam: f.fontFamily, faces: f.faces });
  check(`${tag}: ${where}: no game mechanics in the copy`, !GAME.test(f.text), f.text.match(GAME));
}

async function signUp(page, c) {
  const email = `s75-${opt.label}-${c.name}-${Date.now()}@example.com`;
  await go(page, page.__base + "/signup");
  await page.fill("#email", email);
  await page.fill("#password", "slice75-disposable-pass");
  await Promise.all([page.waitForURL(u => new URL(u).pathname !== "/signup"), press(c, page.locator("main form button.btn-accent").first())]);
  if (new URL(page.url()).pathname !== "/shelf") await go(page, page.__base + "/shelf");
  await press(c, page.locator("form[data-add]").first().locator("button"));
  await page.waitForSelector("[data-shelf-bar]:not([hidden])", { timeout: 10000 }).catch(() => {});
  return email;
}

async function answer(page, c, text) {
  await page.waitForSelector("[data-answer-form]");
  const input = page.locator("#answer");
  await press(c, input);
  if (text) await input.pressSequentially(text, { delay: 0 });
  await Promise.all([page.waitForSelector("[data-review]", { timeout: 15000 }), press(c, page.locator("[data-check]"))]);
  await settle(page);
}

async function next(page, c) {
  // Next card is fetched and swapped in place (app.js initNext).
  await press(c, page.locator("[data-next]"));
  await page.waitForFunction(() => !document.querySelector("[data-review]"), null, { timeout: 15000 });
  await settle(page);
}

async function currentPrompt(page) {
  return page.evaluate(() => (document.querySelector("[data-asking] .prompt, [data-review] [data-prompt]") || {}).textContent?.trim());
}

const verdictOf = page => page.evaluate(() => (document.querySelector("[data-verdict-label]") || {}).textContent?.trim());

async function clickNav(page, c, locator) {
  await Promise.all([page.waitForNavigation({ waitUntil: "load" }), press(c, locator)]);
  await settle(page);
}

// ---- one config ------------------------------------------------------------

async function runConfig(c, port) {
  const health = { console: [], pageErrors: [], failed: [], http5xx: [], csp: [], tooling: [], aborted: [], fonts: new Set() };
  const tag = `${opt.label} ${c.name}`;
  const out = { config: c.name };
  let server, db, browser, context;
  try {
    server = await startServer(c, port);
    out.healthz = { status: server.health.status, mode: server.health.mode, build_sha: server.health.build_sha };
    check(`${tag}: isolated test-mode server is healthy`, server.health.status === "ok", out.healthz);
    browser = await launch(c.engine);
    context = await browser.newContext(contextOptions(c));
    await instrument(context);
    if (opt.traces) await context.tracing.start({ screenshots: true, snapshots: true });
    const page = await context.newPage();
    page.__base = server.url;
    watch(page, health);
    const base = server.url;

    const email = await signUp(page, c);
    db = openDB(server.db);
    const s = seed(db, email);
    out.seed = { A: s.A.id, B: s.B.id, idea: s.ideaId };

    // 1. Deck list and card detail.
    await go(page, `${base}/library/ideas/${s.ideaId}`);
    let f = await facts(page);
    const rowA = await page.locator(`a.row-main[href="/library/items/${s.A.id}"]`).innerText();
    const rowB = await page.locator(`a.row-main[href="/library/items/${s.B.id}"]`).innerText();
    out.deck = { rowA, rowB };
    if (opt.baseline) check(`${tag}: [baseline] deck list cue present?`, true, { cueOnA: rowA.includes(CUE) });
    else {
      check(`${tag}: deck list: A (3 lapses) carries the cue`, rowA.includes(CUE), rowA);
      check(`${tag}: deck list: B (one miss) carries no cue`, !rowB.includes(CUE), rowB);
      layoutChecks(tag, f, c, "deck list");
    }
    await shot(page, c, "deck-list");
    await go(page, `${base}/library/items/${s.A.id}`);
    f = await facts(page);
    if (opt.baseline) check(`${tag}: [baseline] card detail cue present?`, true, { cue: f.text.includes(CUE) });
    else {
      check(`${tag}: card detail: cue and "Build bridge cards"`, f.text.includes(CUE) && (await page.locator("[data-build-bridges]").count()) === 1, f.text.slice(0, 300));
      layoutChecks(tag, f, c, "card detail");
    }
    await shot(page, c, "card-detail");

    // 2. Study: B waits with no cue, is missed again, and the review cues.
    await go(page, `${base}/study`);
    let p = await currentPrompt(page);
    f = await facts(page);
    check(`${tag}: B is the card waiting`, p === "NATO alphabet: B", p);
    check(`${tag}: waiting screen: no cue, no bridge control (no spoilers)`, !f.text.includes("Repeated miss") && !f.text.includes("Build bridge"), f.text.slice(0, 200));
    await answer(page, c, "Beta again");
    check(`${tag}: B judged Incorrect`, (await verdictOf(page)) === "Incorrect", await verdictOf(page));
    f = await facts(page);
    if (opt.baseline) {
      check(`${tag}: [baseline] post-answer cue present?`, true, { cue: f.text.includes(CUE) });
      await shot(page, c, "review-second-miss");
      return out;
    }
    check(`${tag}: post-answer review: cue "${CUE}" and "Build bridge cards"`, f.text.includes(CUE) && (await page.locator("[data-review] [data-build-bridges]").count()) === 1, f.text.slice(0, 400));
    layoutChecks(tag, f, c, "post-answer review");
    await page.locator("[data-struggle]").scrollIntoViewIfNeeded();
    await shot(page, c, "review-second-miss");

    // 3. Generation, edit, remove, accept.
    await clickNav(page, c, page.locator("[data-review] [data-build-bridges]"));
    f = await facts(page);
    const fixture = await page.locator("[data-bridge-q]").evaluateAll(es => es.map(e => e.value));
    check(`${tag}: preview: three deterministic fixture bridges, nothing added yet`, fixture.length === 3 && fixture[0].startsWith("Bridge fixture 1") && heldBy(db, s.B.id) === 0 &&
      db.prepare("SELECT COUNT(*) n FROM items WHERE source = 'bridge'").get().n === 0, fixture);
    check(`${tag}: preview names the card it leads to, never its answer`, f.text.includes("NATO alphabet: B") && !f.text.includes("Bravo"), f.text.slice(0, 300));
    layoutChecks(tag, f, c, "preview");
    await shot(page, c, "preview");
    const q1 = page.locator("[data-bridge-q]").first();
    await q1.fill("What is a phonetic alphabet for?");
    await page.locator("[data-bridge-a]").first().fill("Spelling letters clearly by voice");
    await press(c, page.locator("[data-bridge-card]").nth(2).locator("[data-bridge-remove]"));
    await page.waitForTimeout(200);
    const nums = await page.locator("[data-bridge-num]").allInnerTexts();
    check(`${tag}: Remove takes the third bridge out in place (no reload) and renumbers`, nums.join("|") === "Bridge 1|Bridge 2" && new URL(page.url()).pathname.endsWith("/bridges/draft"), nums);
    if (c.journeyShots) await shot(page, c, "preview-edited");
    await clickNav(page, c, page.locator("[data-bridge-accept]"));
    f = await facts(page);
    const bB = bridgesOf(db)(s.B.id);
    check(`${tag}: "Add bridges & hold parent" adds the 2 edited bridges and holds B`, bB.length === 2 && bB[0].prompt === "What is a phonetic alphabet for?" && heldBy(db, s.B.id) === 2, bB);
    check(`${tag}: back on the review: "Held by 2 bridges" and "Release hold"`, new URL(page.url()).pathname === "/study" && f.text.includes("Held by 2 bridges") && f.text.includes("Release hold"), f.text.slice(0, 400));
    layoutChecks(tag, f, c, "review after accepting");
    await page.locator("[data-hold]").scrollIntoViewIfNeeded();
    await shot(page, c, "review-held-by-2");
    await next(page, c); // finish the one-card session
    check(`${tag}: session finished`, /Done\./.test((await facts(page)).text), new URL(page.url()).pathname);

    // 4. Card A: build from its page, accept all 3, remove one, release.
    await go(page, `${base}/library/items/${s.A.id}`);
    await clickNav(page, c, page.locator("[data-build-bridges]"));
    await clickNav(page, c, page.locator("[data-bridge-accept]"));
    f = await facts(page);
    check(`${tag}: A held by 3 bridges after accepting all three`, heldBy(db, s.A.id) === 3 && f.text.includes("Held by 3 bridges"), f.text.slice(0, 300));
    layoutChecks(tag, f, c, "held card detail");
    if (c.journeyShots) await shot(page, c, "card-held-by-3");
    const aBridges = bridgesOf(db)(s.A.id);
    await go(page, `${base}/library/items/${aBridges[2].id}`);
    check(`${tag}: a bridge's page says it is a bridge card`, (await facts(page)).text.includes("Bridge card · leads up to"), null);
    await clickNav(page, c, page.locator("form[action$='/remove'] button"));
    await go(page, `${base}/library/items/${s.A.id}`);
    f = await facts(page);
    check(`${tag}: removing a bridge card leaves A held by 2`, heldBy(db, s.A.id) === 2 && f.text.includes("Held by 2 bridges") && bridgesOf(db)(s.A.id)[2].end_reason === "removed", f.text.slice(0, 200));
    travel(db, s.A.id, 31); // A is now overdue
    travel(db, s.B.id, 2); // so is B
    await go(page, `${base}/library/ideas/${s.ideaId}`);
    const heldRow = await page.locator(`li:has(a[href="/library/items/${s.A.id}"]) .due`).innerText();
    check(`${tag}: deck list says "held" for A, not "due today"`, heldRow === "held", heldRow);
    if (c.journeyShots) await shot(page, c, "deck-list-held");
    const aBefore = sched(db, s.A.id);
    await go(page, `${base}/library/items/${s.A.id}`);
    await clickNav(page, c, page.locator("[data-release-hold]"));
    f = await facts(page);
    check(`${tag}: Release hold: A no longer held, flash says so, schedule untouched`, heldBy(db, s.A.id) === 0 && f.text.includes("Hold released.") &&
      JSON.stringify(sched(db, s.A.id)) === JSON.stringify(aBefore), { text: f.text.slice(0, 200), before: aBefore, after: sched(db, s.A.id) });
    if (c.journeyShots) await shot(page, c, "card-released");

    // 5. The next session: A back, B held.
    const bBefore = sched(db, s.B.id);
    await go(page, `${base}/study`);
    const ids = sessionItems(db, s.uid);
    p = await currentPrompt(page);
    f = await facts(page);
    out.session2 = ids;
    check(`${tag}: next session: A (released) leads, B (held, overdue) is in no queue`, ids.length === 5 && ids[0] === s.A.id && !ids.includes(s.B.id) && p === "NATO alphabet: A", { ids, p });
    check(`${tag}: waiting screen shows no cue and no answer of a held card`, !f.text.includes("Repeated miss") && !f.text.includes("Bravo"), f.text.slice(0, 300));
    if (c.journeyShots) await shot(page, c, "session-held-queue");
    await answer(page, c, "Alfa"); // right
    await next(page, c);
    p = await currentPrompt(page);
    check(`${tag}: B's first bridge is up`, p === "What is a phonetic alphabet for?", p);
    await answer(page, c, "Talking"); // missed
    f = await facts(page);
    check(`${tag}: bridge Incorrect: B stays held by 2, the review says the card stays held`, (await verdictOf(page)) === "Incorrect" && heldBy(db, s.B.id) === 2 &&
      f.text.includes("stays held until this is answered Correct or Almost Correct"), { held: heldBy(db, s.B.id), text: f.text.slice(0, 300) });
    if (c.journeyShots) { await page.locator("[data-bridge-note]").scrollIntoViewIfNeeded(); await shot(page, c, "bridge-incorrect-still-held"); }
    await next(page, c);
    await answer(page, c, "A fixed script"); // right
    f = await facts(page);
    check(`${tag}: bridge Correct: B held by 1, the review says one bridge still holds it`, (await verdictOf(page)) === "Correct" && heldBy(db, s.B.id) === 1 &&
      f.text.includes("Bridge cleared · 1 bridge still holds the card it leads to."), { held: heldBy(db, s.B.id), text: f.text.slice(0, 300) });
    if (c.journeyShots) { await page.locator("[data-bridge-cleared]").scrollIntoViewIfNeeded(); await shot(page, c, "bridge-correct-held-by-1"); }
    await next(page, c);
    await answer(page, c, "x"); // close (A's bridge, an ordinary card now)
    await next(page, c);
    await answer(page, c, "y"); // right
    await next(page, c);
    check(`${tag}: second session finished`, /Done\./.test((await facts(page)).text), new URL(page.url()).pathname);
    await go(page, `${base}/library/items/${s.B.id}`);
    f = await facts(page);
    check(`${tag}: B's page: "Held by 1 bridge"`, f.text.includes("Held by 1 bridge"), f.text.slice(0, 300));
    if (c.journeyShots) await shot(page, c, "card-held-by-1");
    travel(db, bB[0].id, 2); // the missed bridge is due again
    await go(page, `${base}/study`);
    p = await currentPrompt(page);
    check(`${tag}: third session: only the missed bridge (B still held)`, JSON.stringify(sessionItems(db, s.uid)) === JSON.stringify([bB[0].id]) && p === "What is a phonetic alphabet for?", { ids: sessionItems(db, s.uid), p });
    await answer(page, c, "Spelling letters"); // close
    f = await facts(page);
    check(`${tag}: bridge Almost Correct: B released automatically, schedule untouched`, (await verdictOf(page)) === "Almost Correct" && heldBy(db, s.B.id) === 0 &&
      JSON.stringify(sched(db, s.B.id)) === JSON.stringify(bBefore) && f.text.includes("Bridge cleared · the card it leads to is back in your reviews, on its own schedule."),
      { before: bBefore, after: sched(db, s.B.id), text: f.text.slice(0, 300) });
    layoutChecks(tag, f, c, "bridge cleared review");
    await page.locator("[data-bridge-cleared]").scrollIntoViewIfNeeded();
    await shot(page, c, "bridge-cleared-parent-back");
    await next(page, c);
    await go(page, `${base}/study`);
    p = await currentPrompt(page);
    f = await facts(page);
    check(`${tag}: automatic reentry: the next session opens on B`, p === "NATO alphabet: B" && sessionItems(db, s.uid)[0] === s.B.id, { p, ids: sessionItems(db, s.uid) });
    layoutChecks(tag, f, c, "reentry");
    if (c.journeyShots) await shot(page, c, "reentry-parent-due");
    await page.waitForTimeout(300);
  } catch (err) {
    check(`${tag}: walkthrough completed`, false, String(err && err.stack || err).slice(0, 600));
  } finally {
    if (context && opt.traces) await context.tracing.stop({ path: path.join(opt.traces, `s75-${opt.label}-${c.name}-trace.zip`) }).catch(() => {});
    if (context) await context.close().catch(() => {});
    if (browser) await browser.close().catch(() => {});
    if (db) db.close();
    await stopServer(server);
    if (server && opt.traces) fs.copyFileSync(server.logPath, path.join(opt.traces, `s75-${opt.label}-${c.name}-server.log`));
    if (server && !opt.keep) fs.rmSync(server.dir, { recursive: true, force: true });
  }
  out.health = { ...health, fonts: [...health.fonts] };
  check(`${tag}: 0 console errors, page errors, failed requests, 5xx, CSP violations`,
    !health.console.length && !health.pageErrors.length && !health.failed.length && !health.http5xx.length && !health.csp.length, out.health);
  check(`${tag}: only Scry's own Geist font files were loaded`, [...health.fonts].every(f => /^\/static\/fonts\/Geist(Mono)?-Variable\.woff2$/.test(f)), [...health.fonts]);
  return out;
}

const runs = [];
let port = opt.port;
for (const c of CONFIGS) runs.push(await runConfig(c, port++));
const failed = results.filter(r => !r.ok);
const summary = { label: opt.label, bin: opt.bin, baseline: opt.baseline, passed: results.length - failed.length, failed: failed.length, screenshots: shotCount, checks: results, runs };
console.log(JSON.stringify(summary, null, 2));
process.exit(opt.expect === "pass" && failed.length ? 1 : 0);
