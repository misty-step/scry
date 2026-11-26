#!/usr/bin/env node
// verify-slice106.mjs: check Slice 106 (S106-1..S106-6) on the real running
// path, in a real headed Chrome, against isolated synthetic data.
//
//   make build BUILD_SHA=$(git rev-parse HEAD)
//   DISPLAY=:0 node scripts/verify-slice106.mjs --bin ./bin/scry --sha <sha> \
//        --media ~/chief-evidence/media [--port 18106] [--only desktop-light,...]
//
// It starts its own Scry (the --bin binary) in SCRY_MODE=test on a fresh
// SQLite file in a new temporary directory with a minimal environment (no
// provider keys: model and speech are the test fixtures, auth and billing
// the local fakes; nothing leaves the machine). Then, for desktop 1440x900
// and phone 390x844, each in light and dark, a new learner walks through:
//
//   S106-1 landing: the Study Specimen rests on Judged when served, steps on
//          its own, holds while hovered, a chosen step shows that stage;
//          the four schedule facts each sit on one line beside an icon, at
//          least 12 px clear of every rule; the four how-it-works rows have
//          icons; reduced motion stays on Judged; without script Judged.
//   S106-4 five destinations in navigation, no Inbox anywhere; /inbox ends
//          on Library; an API token from Settings adds cards through
//          /api/v1/items and /api/v1/suggestions; /api/v1/ideas lists them.
//   S106-2 Create: Markdown pasted into the textarea, Ctrl+Enter, a draft
//          outline from the fixture, Approve all, the cards in Library; no
//          hand-written card form in Library.
//   S106-3 Import a deck: .apkg, .csv and card-block .md straight into the
//          library ("Imported N cards."); unstructured .md to drafting; a
//          bad file refused in words. Downloads of Markdown, CSV and JSON;
//          (desktop-light) the Markdown and CSV exports re-imported by a
//          second learner give back the same export byte for byte.
//   S106-5 Study: Snooze a waiting card ("Snoozed until tomorrow."), answer
//          one, Edit card (question and a new idea) in the review, Delete in
//          the review, Restore from the quiet line, Delete a waiting card and
//          Restore it from Settings → Recently removed.
//
// Every page is checked for sideways scroll, loaded styles and decoded
// images; console errors, page errors, failed requests, subresource
// responses >= 400, document responses >= 500 and CSP violations are
// counted per config and must all be 0. Screenshots and one video per
// config go to --media. Exit 0 only when every check passes.

import { chromium } from "playwright-core";
import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const args = Object.fromEntries(process.argv.slice(2).reduce((a, v, i, all) => {
  if (v.startsWith("--")) a.push([v.slice(2), all[i + 1] && !all[i + 1].startsWith("--") ? all[i + 1] : "1"]);
  return a;
}, []));
const BIN = path.resolve(args.bin || "bin/scry");
const MEDIA = path.resolve(args.media || path.join(os.homedir(), "chief-evidence", "media"));
const PORT = Number(args.port || 18106);
const SHA = args.sha || "";
const ONLY = args.only ? args.only.split(",") : null;
const ROOT = path.resolve(path.dirname(new URL(import.meta.url).pathname), "..");
const DECKS = path.join(ROOT, "testdata", "decks");
const CHROME = process.env.CHROME_BIN || "/usr/bin/google-chrome";
const BASE = `http://127.0.0.1:${PORT}`;

fs.mkdirSync(MEDIA, { recursive: true });
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "scry-s106-"));
const results = [];
let failed = 0;
function check(config, name, ok, detail = "") {
  results.push({ config, name, ok: !!ok, detail: String(detail) });
  if (!ok) failed++;
  console.log(`${ok ? "PASS" : "FAIL"} [${config}] ${name}${detail !== "" ? " — " + detail : ""}`);
}

// ---- the server ---------------------------------------------------------

const serverLog = fs.createWriteStream(path.join(tmp, "server.log"));
const server = spawn(BIN, [], {
  env: { PATH: process.env.PATH, HOME: tmp, SCRY_MODE: "test", SCRY_DB: path.join(tmp, "s106.db"), PORT: String(PORT) },
  stdio: ["ignore", "pipe", "pipe"],
});
server.stdout.pipe(serverLog);
server.stderr.pipe(serverLog);
async function waitHealthy() {
  for (let i = 0; i < 100; i++) {
    try {
      const r = await fetch(BASE + "/healthz");
      if (r.ok) return r.json();
    } catch {}
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("Scry did not become healthy");
}

// ---- page instrumentation -----------------------------------------------

function instrument(page, problems) {
  page.on("console", (m) => { if (m.type() === "error") problems.push("console: " + m.text()); });
  page.on("pageerror", (e) => problems.push("pageerror: " + e.message));
  page.on("requestfailed", (r) => {
    const f = r.failure()?.errorText || "";
    // A download's navigation is aborted by the browser once it is saved.
    if (f.includes("ERR_ABORTED") && /\/settings\/export/.test(r.url())) return;
    // Chrome cancels a favicon fetch still in flight when the page navigates.
    if (f.includes("ERR_ABORTED") && /\/static\/favicon/.test(r.url())) return;
    problems.push(`requestfailed: ${r.url()} ${f}`);
  });
  page.on("response", (r) => {
    const t = r.request().resourceType();
    if (t === "document" ? r.status() >= 500 : r.status() >= 400) problems.push(`http ${r.status()}: ${t} ${r.url()}`);
  });
}

const cspInit = () => {
  window.__csp = [];
  document.addEventListener("securitypolicyviolation", (e) => window.__csp.push(e.violatedDirective + " " + e.blockedURI));
};

async function pageHealth(page, config, label) {
  const h = await page.evaluate(() => {
    const imgs = [...document.images].filter((i) => i.getBoundingClientRect().width > 0);
    return {
      overflow: document.documentElement.scrollWidth - window.innerWidth,
      sheets: document.styleSheets.length,
      font: getComputedStyle(document.body).fontFamily,
      undecoded: imgs.filter((i) => !(i.complete && i.naturalWidth > 0)).map((i) => i.src),
      csp: window.__csp || [],
    };
  });
  check(config, `${label}: 0 px sideways scroll`, h.overflow <= 0, `${h.overflow}px`);
  check(config, `${label}: styles loaded`, h.sheets > 0 && /Geist/.test(h.font), h.font.slice(0, 30));
  check(config, `${label}: visible images decoded`, h.undecoded.length === 0, h.undecoded.join(" "));
  check(config, `${label}: 0 CSP violations`, h.csp.length === 0, h.csp.join("; "));
}

async function shot(page, name) {
  await page.screenshot({ path: path.join(MEDIA, name + ".png") });
}

// ---- the journey --------------------------------------------------------

const MARKDOWN = `# Caching

## Cache invalidation

A cache entry is invalidated when the data behind it changes.
Write-through caches update the cache on every write.

## Thundering herd

When a popular entry expires, many requests miss at once and all hit the database.
Request coalescing lets one request refill the entry while the others wait.
`;

async function landing(browser, config, opts) {
  const ctx = await browser.newContext({ ...opts, recordVideo: undefined });
  await ctx.addInitScript(cspInit);
  const page = await ctx.newPage();
  const problems = [];
  instrument(page, problems);
  await page.goto(BASE + "/");
  const shown = async () => page.$eval("[data-specimen]", (f) => {
    const s = [...f.querySelectorAll("[data-specimen-stage]")].filter((x) => getComputedStyle(x).visibility !== "hidden" && Number(getComputedStyle(x).opacity) > 0.5);
    return s.map((x) => x.dataset.specimenStage).join(",");
  });
  check(config, "landing: served resting on Judged", (await shown()) === "judged", await shown());
  await shot(page, `${config}-landing`);
  await pageHealth(page, config, "landing");
  // Schedule facts: one line, beside an icon, >= 12 px clear of every rule.
  const facts = await page.$$eval(".specimen-schedule .specimen-fact", (cells) => cells.map((c) => {
    const cb = c.getBoundingClientRect(), dt = c.querySelector("dt"), dd = c.querySelector("dd");
    const db = dt.getBoundingClientRect(), vb = dd.getBoundingClientRect();
    const lh = parseFloat(getComputedStyle(dt).lineHeight) || parseFloat(getComputedStyle(dt).fontSize) * 1.4;
    return {
      label: dt.textContent.trim(), icon: !!dt.querySelector("svg"),
      oneLine: db.height <= lh * 1.3 && getComputedStyle(dt).whiteSpace === "nowrap",
      clear: Math.min(db.left - cb.left, cb.right - Math.max(db.right, vb.right), db.top - cb.top, cb.bottom - vb.bottom, db.left - cb.left),
      clipped: dt.scrollWidth > dt.clientWidth + 1 || dd.scrollWidth > dd.clientWidth + 1,
    };
  }));
  check(config, "landing: four schedule facts", facts.length === 4, facts.map((f) => f.label).join(" | "));
  for (const f of facts) {
    check(config, `landing: "${f.label}" on one line beside its icon, not clipped`, f.oneLine && f.icon && !f.clipped);
    check(config, `landing: "${f.label}" ≥ 12 px clear of every rule`, f.clear >= 12 - 0.5, `${f.clear.toFixed(1)}px`);
  }
  const rows = await page.$$eval(".plain-list .plain-row", (rs) => rs.map((r) => !!r.querySelector("h2 svg")));
  check(config, "landing: four how-it-works rows, each with its icon", rows.length === 4 && rows.every(Boolean));
  const body = await page.textContent("body");
  check(config, "landing: no Inbox, no approval queue in the copy", !/inbox|until you approve/i.test(body));
  // Stepping on its own (3.5–4.5 s), height constant.
  await page.mouse.move(1, 1);
  const h0 = await page.$eval("[data-specimen]", (f) => f.getBoundingClientRect().height);
  await page.waitForTimeout(4700);
  const s1 = await shown();
  check(config, "landing: the Specimen steps on its own", s1 !== "judged" && s1 !== "", s1);
  const h1 = await page.$eval("[data-specimen]", (f) => f.getBoundingClientRect().height);
  check(config, "landing: nothing shifts between stages", Math.abs(h1 - h0) < 0.5, `${h0} -> ${h1}`);
  // Hover holds it.
  await page.hover("[data-specimen]");
  const held = await shown();
  await page.waitForTimeout(4700);
  check(config, "landing: holds still while hovered", (await shown()) === held, held);
  // A chosen step shows that stage and stays.
  await page.click('[data-specimen-step="prompt"]');
  await page.mouse.move(1, 1);
  await page.waitForTimeout(4700);
  check(config, "landing: a chosen step (1 · Prompt) shows Prompt and stays", (await shown()) === "prompt");
  const pressed = await page.$eval('[data-specimen-step="prompt"]', (b) => [b.getAttribute("aria-pressed"), getComputedStyle(b).textDecorationLine, b.getBoundingClientRect().height]);
  check(config, "landing: the chosen step is pressed, underlined, 44 px", pressed[0] === "true" && /underline/.test(pressed[1]) && pressed[2] >= 44, pressed.join(" "));
  await shot(page, `${config}-landing-prompt-step`);
  await page.click('[data-specimen-step="answer"]');
  await page.waitForTimeout(300);
  check(config, "landing: 2 · Answer shows the typed answer", (await shown()) === "answer");
  await ctx.close();
  // Reduced motion: rests on Judged.
  const rctx = await browser.newContext({ ...opts, reducedMotion: "reduce" });
  const rp = await rctx.newPage();
  await rp.goto(BASE + "/");
  await rp.mouse.move(1, 1);
  await rp.waitForTimeout(9000);
  const rs = await rp.$eval("[data-specimen]", (f) => [...f.querySelectorAll("[data-specimen-stage]")].filter((x) => getComputedStyle(x).visibility !== "hidden").map((x) => x.dataset.specimenStage).join(","));
  check(config, "landing: under reduced motion it rests on Judged after 9 s", rs === "judged", rs);
  await rctx.close();
  // Without script: Judged, complete and still.
  const nctx = await browser.newContext({ ...opts, javaScriptEnabled: false });
  const np = await nctx.newPage();
  await np.goto(BASE + "/");
  const ns = await np.$eval("[data-specimen]", (f) => [...f.querySelectorAll("[data-specimen-stage]")].filter((x) => getComputedStyle(x).visibility !== "hidden").map((x) => x.dataset.specimenStage).join(","));
  check(config, "landing: without script Judged is shown", ns === "judged", ns);
  await nctx.close();
  check(config, "landing: 0 console/page/request problems", problems.length === 0, problems.join(" | "));
}

// Snooze, Delete and Restore post with fetch and swap the page in place;
// wait for the POST and the page it swaps in.
async function manage(page, selector, route, after) {
  await Promise.all([
    page.waitForResponse((r) => r.request().method() === "POST" && new URL(r.url()).pathname === route),
    page.click(selector),
  ]);
  await page.waitForSelector(after);
}

async function signUp(page, email) {
  await page.goto(BASE + "/signup");
  await page.fill("#email", email);
  await page.fill("#password", "correct horse battery");
  await Promise.all([page.waitForURL(/\/shelf/), page.click("button.btn-accent")]);
}

async function learner(browser, config, opts, phone) {
  const ctx = await browser.newContext({ ...opts, acceptDownloads: true, recordVideo: { dir: tmp, size: opts.viewport } });
  await ctx.addInitScript(cspInit);
  const page = await ctx.newPage();
  const problems = [];
  instrument(page, problems);
  const email = `${config}-${Date.now()}@example.com`;

  // Landing, then sign up, on camera.
  await page.goto(BASE + "/");
  await page.mouse.move(1, 1);
  await page.waitForTimeout(4500);
  await page.click('[data-specimen-step="judged"]');
  await page.waitForTimeout(600);
  await signUp(page, email);
  await pageHealth(page, config, "shelf");

  // S106-4: five destinations, all in view; no Inbox.
  const nav = await page.$$eval(".nav a", (as) => as.map((a) => {
    const b = a.getBoundingClientRect();
    return { t: a.textContent.trim(), inView: b.left >= 0 && b.right <= window.innerWidth + 0.5 && b.width > 0, icon: !!a.querySelector("svg") };
  }));
  const labels = nav.map((n) => n.t).join(",");
  check(config, "nav: Study, Library, Create, Billing, Settings", labels === "Study,Library,Create,Billing,Settings", labels);
  check(config, "nav: every destination in view with its icon", nav.every((n) => n.inView && n.icon));
  if (phone) {
    const rowsN = await page.$$eval(".nav a", (as) => new Set(as.map((a) => Math.round(a.getBoundingClientRect().top))).size);
    check(config, "phone nav: one row of five", rowsN === 1);
  }
  await shot(page, `${config}-shelf-nav`);

  // Add a starter set.
  await page.click('button[aria-label="Add Mental models"]').catch(async () => page.click(".tile-add >> nth=1"));
  await page.waitForSelector("[data-added]:visible");
  await page.waitForTimeout(400);

  // /inbox ends on Library.
  const r = await page.goto(BASE + "/inbox");
  check(config, "/inbox redirects to Library", new URL(page.url()).pathname.startsWith("/library") && r.status() === 200, page.url());
  const libBody = await page.textContent("main");
  check(config, "library: no hand-written card forms, no Inbox", !/Add an item|Add an exact text|inbox/i.test(libBody));
  check(config, "library: outline ends Starter shelf · Create or import cards", /Starter shelf · Create or import cards/.test(libBody));

  // S106-2: Create from pasted Markdown, to approval.
  await page.goto(BASE + "/learn");
  await page.fill("[data-ask-input]", MARKDOWN);
  await page.waitForTimeout(300);
  const ta = await page.$eval("[data-ask-input]", (t) => ({ tag: t.tagName, max: t.maxLength, h: t.getBoundingClientRect().height }));
  check(config, "create: a textarea up to 16,000 characters that grows", ta.tag === "TEXTAREA" && ta.max === 16000 && ta.h > 150, JSON.stringify(ta));
  await pageHealth(page, config, "create");
  await shot(page, `${config}-create-markdown`);
  await Promise.all([page.waitForURL(/\/learn\/\d+/), page.press("[data-ask-input]", "Control+Enter")]);
  await page.waitForSelector("text=Approve all", { timeout: 15000 });
  const draftIdeas = await page.$$eval(".draft-idea", (d) => d.length);
  check(config, "create: Ctrl+Enter drafts an outline grouped by idea", draftIdeas >= 2, `${draftIdeas} ideas`);
  await shot(page, `${config}-create-draft`);
  await pageHealth(page, config, "draft");
  await page.click("text=Approve all");
  await page.waitForLoadState("networkidle");
  await page.goto(BASE + "/library");
  check(config, "create: approved cards are in Library", /Caching/.test(await page.textContent(".outline")));

  // S106-3: import decks.
  for (const [file, idea, want] of [["sample.apkg", "", 4], ["sample.csv", "", 5], ["sample.md", "Memory science", 3]]) {
    await page.goto(BASE + "/learn#import");
    await page.setInputFiles("#import-file", path.join(DECKS, file));
    if (idea) await page.fill("#import-idea", idea);
    if (file === "sample.apkg") await shot(page, `${config}-import-form`);
    await Promise.all([page.waitForURL(/\/library\/ideas\/\d+/), page.click("[data-import-submit]")]);
    const flash = await page.textContent(".flash").catch(() => "");
    check(config, `import ${file}: Library opens on the idea, "Imported ${want} cards."`, flash.includes(`Imported ${want} cards.`), flash.trim());
    if (file === "sample.apkg") { await shot(page, `${config}-imported-apkg`); await pageHealth(page, config, "imported"); }
  }
  await page.goto(BASE + "/learn#import");
  await page.setInputFiles("#import-file", path.join(DECKS, "notes.md"));
  await Promise.all([page.waitForURL(/\/learn\/\d+/), page.click("[data-import-submit]")]);
  check(config, "import notes.md: unstructured Markdown goes to drafting", /Sent to drafting/.test(await page.textContent("main")));
  await page.goto(BASE + "/learn#import");
  await page.setInputFiles("#import-file", { name: "broken.apkg", mimeType: "application/octet-stream", buffer: Buffer.from("not a zip") });
  await page.click("[data-import-submit]");
  await page.waitForLoadState("domcontentloaded");
  // Import says "importing" while it waits ([S113-2]); wait for the refusal.
  await page.waitForSelector("[data-import-status] .field-error", { timeout: 10000 }).catch(() => {});
  const refusal = await page.textContent("[data-import-status]");
  check(config, "import: a bad file is refused in words", /Anki|read/i.test(refusal) && (await page.$("[data-import-status] .field-error")) !== null, refusal.trim());

  // S106-4: an API token from Settings adds straight to the library.
  await page.goto(BASE + "/settings");
  await page.fill("#name", "reading script");
  await Promise.all([page.waitForLoadState("domcontentloaded"), page.click("text=Create token")]);
  const token = (await page.textContent(".token-value")).trim();
  const curl = await page.textContent(".token-new pre");
  check(config, "settings: curl example for POST /api/v1/items", /\/api\/v1\/items/.test(curl));
  const api = async (method, p, body) => {
    const res = await page.request.fetch(BASE + p, { method, headers: { Authorization: "Bearer " + token, "Content-Type": "application/json" }, data: body });
    return [res.status(), await res.json()];
  };
  let [st, js] = await api("POST", "/api/v1/items", { source: "reading", items: [{ question: "What does TTL stand for?", answer: "Time to live", idea: "API cards", summary: "Cards from a script" }] });
  check(config, "api: POST /api/v1/items adds to the library", st === 201 && js.added === 1, `${st} ${JSON.stringify(js)}`);
  [st, js] = await api("POST", "/api/v1/suggestions", { items: [{ question: "What is a cache hit?", answer: "A lookup the cache can answer", idea: "API cards" }] });
  check(config, "api: POST /api/v1/suggestions adds straight to the library", st === 201 && js.added === 1, `${st} ${JSON.stringify(js)}`);
  [st, js] = await api("GET", "/api/v1/ideas");
  check(config, "api: GET /api/v1/ideas lists my ideas", st === 200 && js.ideas.some((i) => i.title === "API cards" && i.items === 2), `${st}`);
  [st] = await api("POST", "/api/v1/items", { items: [] });
  check(config, "api: 0 items refused (422)", st === 422);

  // S106-3: downloads.
  await page.goto(BASE + "/settings");
  await pageHealth(page, config, "settings");
  const downloads = {};
  for (const [label, ext] of [["Download Markdown", "md"], ["Download CSV", "csv"], ["Download JSON", "json"]]) {
    const [dl] = await Promise.all([page.waitForEvent("download"), page.click(`text=${label}`)]);
    const p = path.join(tmp, `${config}-export.${ext}`);
    await dl.saveAs(p);
    downloads[ext] = fs.readFileSync(p, "utf8");
  }
  check(config, "export: Markdown download", downloads.md.startsWith("# Scry export") && downloads.md.includes("Time to live"));
  check(config, "export: CSV download", downloads.csv.startsWith("idea,question,answer,exact") && downloads.csv.includes("Time to live"));
  check(config, "export: JSON download", JSON.parse(downloads.json).tables.items.length > 20);
  await page.locator("[data-export-links]").scrollIntoViewIfNeeded();
  await shot(page, `${config}-settings-your-data`);

  // S106-5: Study.
  await page.goto(BASE + "/study");
  await page.waitForSelector("[data-asking]");
  await pageHealth(page, config, "study waiting");
  const firstPrompt = (await page.textContent("#prompt")).trim();
  await shot(page, `${config}-study-waiting-actions`);
  await manage(page, "[data-asking] [data-snooze]", "/study/snooze", "[data-study-note]");
  check(config, "study: Snooze on a waiting card says \"Snoozed until tomorrow.\"", /Snoozed until tomorrow\./.test(await page.textContent("[data-study-note]")));
  const nextPrompt = (await page.textContent("#prompt").catch(() => "")).trim();
  check(config, "study: Snooze goes straight to the next card", nextPrompt !== "" && nextPrompt !== firstPrompt, `${firstPrompt} -> ${nextPrompt}`);
  // Answer one.
  await page.fill("#answer", "a guess");
  await Promise.all([page.waitForSelector("[data-judged]"), page.press("#answer", "Enter")]);
  await pageHealth(page, config, "study review");
  // Edit card: question and a new idea, in place.
  await page.click("[data-edit-toggle]");
  const newQ = nextPrompt + " (edited)";
  await page.fill("#edit-prompt", newQ);
  await page.fill("#edit-idea", "Edited in Study");
  await page.click("[data-edit-save]");
  await page.waitForSelector("[data-edit-status]:not([hidden])");
  const after = { q: (await page.textContent("[data-prompt]")).trim(), place: (await page.textContent("[data-study-header]")).trim(), verdict: await page.textContent("[data-verdict-label]") };
  check(config, "study: Edit card saves, the review shows the new question", after.q === newQ, after.q);
  check(config, "study: Edit card moves it, the review shows the new place", /Edited in Study/.test(after.place), after.place);
  await shot(page, `${config}-study-edit-saved`);
  // Delete in the review, then Restore from the quiet line.
  await page.reload();
  await page.waitForSelector("[data-judged]");
  await shot(page, `${config}-study-review-actions`);
  await manage(page, "[data-judged] [data-delete]", "/study/delete", "[data-study-note] [data-restore]");
  check(config, "study: Delete says \"Card removed · Restore\"", /Card removed/.test(await page.textContent("[data-study-note]")) && (await page.$("[data-restore]")) !== null);
  await shot(page, `${config}-study-card-removed`);
  await manage(page, "[data-restore]", "/study/restore", "[data-study-note]:has-text('Card restored')");
  await shot(page, `${config}-study-card-restored`);
  check(config, "study: Restore from the quiet line brings the card back", /Card restored/.test(await page.textContent("[data-study-note]").catch(() => "")));
  // Delete a waiting card, restore it from Settings → Recently removed.
  await page.goto(BASE + "/study");
  await page.waitForSelector("[data-asking]");
  const doomed = (await page.textContent("#prompt")).trim();
  await manage(page, "[data-asking] [data-delete]", "/study/delete", "[data-study-note]");
  await page.goto(BASE + "/settings");
  const removedRow = page.locator("li", { hasText: doomed.slice(0, 30) }).first();
  check(config, "settings: Recently removed lists the removed card", (await removedRow.count()) === 1, doomed);
  await removedRow.scrollIntoViewIfNeeded();
  await shot(page, `${config}-settings-recently-removed`);
  await Promise.all([page.waitForLoadState("domcontentloaded"), removedRow.locator("button", { hasText: "Restore" }).click()]);
  check(config, "settings: Restore puts the card back", /Restored/.test(await page.textContent(".flash").catch(() => "")));

  check(config, "journey: 0 console/page/request/CSP problems", problems.length === 0, problems.join(" | "));
  const video = page.video();
  await ctx.close();
  if (video) {
    const out = path.join(MEDIA, `${config}-walkthrough.webm`);
    await video.saveAs(out);
  }
  return downloads;
}

async function roundTrip(browser, config, opts, downloads) {
  const ctx = await browser.newContext({ ...opts, acceptDownloads: true });
  const page = await ctx.newPage();
  const problems = [];
  instrument(page, problems);
  for (const ext of ["md", "csv"]) {
    await page.waitForLoadState("load");
    await signUp(page, `rt-${ext}-${Date.now()}@example.com`);
    await page.goto(BASE + "/learn#import");
    await page.setInputFiles("#import-file", { name: `scry-export.${ext}`, mimeType: "text/plain", buffer: Buffer.from(downloads[ext]) });
    await Promise.all([page.waitForURL(/\/library\/ideas\/\d+/), page.click("[data-import-submit]")]);
    await page.goto(BASE + "/settings");
    const [dl] = await Promise.all([page.waitForEvent("download"), page.click(`text=${ext === "md" ? "Download Markdown" : "Download CSV"}`)]);
    const p = path.join(tmp, `rt.${ext}`);
    await dl.saveAs(p);
    const again = fs.readFileSync(p, "utf8");
    check(config, `round trip: the ${ext.toUpperCase()} export imports back identical (every idea, question, answer, exact flag)`, again === downloads[ext],
      `${downloads[ext].length} vs ${again.length} bytes`);
    await page.waitForLoadState("load");
    await page.context().clearCookies();
  }
  check(config, "round trip: 0 problems", problems.length === 0, problems.join(" | "));
  await ctx.close();
}

// ---- run ----------------------------------------------------------------

const configs = [
  { name: "desktop-light", viewport: { width: 1440, height: 900 }, colorScheme: "light", phone: false },
  { name: "desktop-dark", viewport: { width: 1440, height: 900 }, colorScheme: "dark", phone: false },
  { name: "phone-light", viewport: { width: 390, height: 844 }, colorScheme: "light", phone: true },
  { name: "phone-dark", viewport: { width: 390, height: 844 }, colorScheme: "dark", phone: true },
].filter((c) => !ONLY || ONLY.includes(c.name));

let exitCode = 1;
try {
  const health = await waitHealthy();
  check("server", "/healthz 200 with the build sha", health.status === "ok" && (!SHA || SHA.startsWith(health.build_sha)), JSON.stringify(health));
  const browser = await chromium.launch({ executablePath: CHROME, headless: false, args: ["--window-position=0,0"] });
  check("server", "Chrome is headed", !!process.env.DISPLAY, `DISPLAY=${process.env.DISPLAY}`);
  for (const c of configs) {
    const opts = { viewport: c.viewport, colorScheme: c.colorScheme, timezoneId: "America/Chicago", locale: "en-US",
      ...(c.phone ? { isMobile: true, hasTouch: true, deviceScaleFactor: 2 } : {}) };
    await landing(browser, c.name, opts);
    const downloads = await learner(browser, c.name, opts, c.phone);
    if (c.name === "desktop-light") await roundTrip(browser, c.name, opts, downloads);
  }
  await browser.close();
  exitCode = failed === 0 ? 0 : 1;
} catch (e) {
  console.error(e);
  check("run", "the walkthrough ran to the end", false, e.message);
} finally {
  server.kill("SIGTERM");
  await new Promise((r) => server.once("exit", r));
  const sum = { sha: SHA, passed: results.filter((r) => r.ok).length, failed, total: results.length, results };
  fs.writeFileSync(path.join(MEDIA, "verify-slice106-results.json"), JSON.stringify(sum, null, 1));
  fs.copyFileSync(path.join(tmp, "server.log"), path.join(tmp, "server.log.keep"));
  console.log(`\n${sum.passed}/${sum.total} checks passed; server log ${path.join(tmp, "server.log")}`);
  process.exit(exitCode);
}
