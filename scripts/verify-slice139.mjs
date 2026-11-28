#!/usr/bin/env node
// verify-slice139.mjs: check Slice 139 ([S139-1], [S139-2]) on the real
// running path, in a real headed Chrome.
//
//   make build BUILD_SHA=$(git rev-parse HEAD)
//   DISPLAY=:0 node scripts/verify-slice139.mjs --bin ./bin/scry --sha <sha> \
//        --media ~/chief-evidence/media [--port 18139] [--only desktop-light,...] \
//        [--expect-fail]
//
// For each of desktop 1440x900 (DPR 1) and phone 390x844 (touch, DPR 2),
// light and dark, it starts its own Scry (the --bin binary) in SCRY_MODE=test
// on a fresh SQLite file in a new temporary directory, with a minimal
// environment (no provider keys; nothing leaves the machine) and the test
// grader scripted to Incorrect (SCRY_TEST_JUDGMENTS=missed). It records a
// video and walks a learner's path with clicks, taps and keys:
//
//   sign up -> Settings: create an API token -> POST /api/v1/items with the
//   testdata/s139/fixture.json cards (resources, ideaResources, relatedTags,
//   relatedIdeas) -> Study: the long card waits (nothing of Related &
//   resources in its markup) -> a short wrong answer, graded Incorrect, with
//   other cards still to come -> the [S139-1] long-answer review (Title
//   size, 1.55, stacked 16 px apart, <= 68ch, no inner scroll, no clipping,
//   no sideways scroll, page heights, everything reached by scrolling the
//   page) -> Related & resources ([S139-2]): closed, counts, opened by click
//   or tap and by keyboard, rows, links, no shadow or fill, 200 ms ease-out,
//   at once under reduced motion -> Ask Why? ends in one "Rebuild:"
//   sentence -> Next card through the rest of the session (a short answer
//   with a line break stacks, a long learner answer stacks, two short
//   answers keep Judgment size side by side on desktop; the exact line has
//   no Ask Why?) -> the long card's own page and its idea's page.
//
// Every page is checked for sideways scroll, loaded styles, decoded images
// and CSP violations; console errors, page errors, failed requests and
// responses >= 400 must be 0, and no request may go to a resource's host.
// Exit 0 only when every check passes (with --expect-fail, the run is a
// negative control: it exits 0 only when at least one [S139-*] check fails).

import { chromium } from "playwright-core";
import { spawn, execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const args = Object.fromEntries(process.argv.slice(2).reduce((a, v, i, all) => {
  if (v.startsWith("--")) a.push([v.slice(2), all[i + 1] && !all[i + 1].startsWith("--") ? all[i + 1] : "1"]);
  return a;
}, []));
const ROOT = path.resolve(path.dirname(new URL(import.meta.url).pathname), "..");
const BIN = path.resolve(args.bin || "bin/scry");
const MEDIA = path.resolve(args.media || path.join(os.homedir(), "chief-evidence", "media"));
const PORT0 = Number(args.port || 18139);
const SHA = args.sha || "";
const ONLY = args.only ? args.only.split(",") : null;
const EXPECT_FAIL = !!args["expect-fail"];
const PREFIX = args.prefix || "s139";
const SHOTS = args.shots !== "0";
const CHROME = process.env.CHROME_BIN || "/usr/bin/google-chrome";
const FIXTURE = JSON.parse(fs.readFileSync(path.join(ROOT, "testdata", "s139", "fixture.json"), "utf8"));

const [LONG, SAME, TAGGED, LINKED, EXACT, ONEWORD] = FIXTURE.items;
const RESOURCE_TITLES = [...LONG.resources, ...LONG.ideaResources].map((r) => r.title);
const RESOURCE_HOSTS = ["en.wikipedia.org"];
const ANSWERS = new Map([
  [LONG.question, "the price"],                                  // short wrong answer, long correct one
  [SAME.question, "the best\noption"],                           // <= 80 characters with a line break
  [TAGGED.question, "a longer answer ".repeat(6).trim()],        // long learner answer, short correct one
  [LINKED.question, "no"],
  [ONEWORD.question, "Fixed"],                                    // two short answers
  [EXACT.question, "x"],
]);

fs.mkdirSync(MEDIA, { recursive: true });
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "scry-s139-"));
const results = [];
let failed = 0;
function check(config, name, ok, detail = "") {
  const d = typeof detail === "string" ? detail : JSON.stringify(detail);
  results.push({ config, name, ok: !!ok, detail: d });
  if (!ok) failed++;
  console.log(`${ok ? "PASS" : "FAIL"} [${config}] ${name}${d !== "" ? " — " + d : ""}`);
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ---- the server ---------------------------------------------------------

function startServer(bin, label, port) {
  const dir = fs.mkdtempSync(path.join(tmp, label + "-"));
  const log = fs.createWriteStream(path.join(dir, "server.log"));
  const proc = spawn(bin, [], {
    env: { PATH: process.env.PATH, HOME: dir, SCRY_MODE: "test", SCRY_TEST_JUDGMENTS: "missed", SCRY_DB: path.join(dir, "s139.db"), PORT: String(port) },
    stdio: ["ignore", "pipe", "pipe"],
  });
  proc.stdout.pipe(log);
  proc.stderr.pipe(log);
  return { proc, base: `http://127.0.0.1:${port}` };
}
async function waitHealthy(base) {
  for (let i = 0; i < 300; i++) {
    try {
      const r = await fetch(base + "/healthz");
      if (r.ok) return r.json();
    } catch {}
    await sleep(100);
  }
  throw new Error("Scry did not become healthy");
}

// ---- page instrumentation -----------------------------------------------

function instrument(page, problems, hosts) {
  page.on("console", (m) => { if (m.type() === "error") problems.push("console: " + m.text()); });
  page.on("pageerror", (e) => problems.push("pageerror: " + e.message));
  page.on("request", (r) => { try { hosts.add(new URL(r.url()).hostname); } catch {} });
  page.on("requestfailed", (r) => {
    const f = r.failure()?.errorText || "";
    if (f.includes("ERR_ABORTED") && /\/static\/favicon|\/favicon\.ico/.test(r.url())) return;
    problems.push(`requestfailed: ${r.url()} ${f}`);
  });
  page.on("response", (r) => { if (r.status() >= 400) problems.push(`http ${r.status()}: ${r.request().resourceType()} ${r.url()}`); });
}
const initScript = () => {
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
  check(config, `${label}: 0 CSP violations`, h.csp.length === 0, h.csp.join(" "));
}

async function shot(page, name, full = false) {
  if (!SHOTS) return;
  await page.screenshot({ path: path.join(MEDIA, `${PREFIX}-${name}.png`), fullPage: full });
}

function contextOptions(c) {
  return { viewport: c.viewport, colorScheme: c.colorScheme, timezoneId: "America/Chicago", locale: "en-US",
    ...(c.phone ? { isMobile: true, hasTouch: true, deviceScaleFactor: 2 } : {}) };
}

async function press(page, c, locator) {
  if (c.phone) await locator.tap(); else await locator.click();
}

// ---- in-page readers ----------------------------------------------------

// Every element of the review that scrolls inside itself, clips its text,
// or is wider than the page; and the page's own measures.
async function reviewBoxes(page) {
  return page.evaluate(() => {
    const review = document.querySelector("[data-review]");
    const scrollers = [], clipped = [];
    for (const el of review.querySelectorAll("*")) {
      // The review drawers are closed: what is inside a closed drawer is not
      // drawn (the Edit card form's fields only exist once it opens).
      if (el.closest("details:not([open])") && !el.matches("summary, summary *")) continue;
      const cs = getComputedStyle(el);
      if (/(auto|scroll)/.test(cs.overflowY) && el.scrollHeight > el.clientHeight + 1) scrollers.push(el.className || el.tagName);
      if (/(auto|scroll)/.test(cs.overflowX) && el.scrollWidth > el.clientWidth + 1) scrollers.push((el.className || el.tagName) + " (x)");
      if (/(hidden|clip)/.test(cs.overflowY + cs.overflowX) && !el.closest("details:not([open])") && !el.classList.contains("sr") &&
          (el.scrollHeight > el.clientHeight + 1 || el.scrollWidth > el.clientWidth + 1)) clipped.push(el.className || el.tagName);
    }
    return { scrollers, clipped, sideways: document.documentElement.scrollWidth - window.innerWidth, height: document.documentElement.scrollHeight };
  });
}

// The long pair, read from the live DOM.
async function longPair(page) {
  return page.evaluate(() => {
    const pair = document.querySelector("[data-long-pair]");
    if (!pair) return { missing: true };
    const blocks = [...pair.querySelectorAll(".long-answer")];
    const probe = document.createElement("span");
    const texts = blocks.map((b) => {
      const label = b.querySelector(".long-label"), t = b.querySelector(".long-text");
      const cs = getComputedStyle(t), r = t.getBoundingClientRect();
      probe.style.font = cs.font; probe.style.letterSpacing = cs.letterSpacing; probe.style.position = "absolute"; probe.textContent = "0";
      document.body.appendChild(probe);
      const ch = probe.getBoundingClientRect().width;
      probe.remove();
      const lcs = getComputedStyle(label);
      return {
        label: label.textContent.trim(), labelFont: lcs.fontFamily, labelColor: lcs.color, inkMuted: getComputedStyle(document.querySelector(".meta") || label).color,
        size: cs.fontSize, lh: parseFloat(cs.lineHeight) / parseFloat(cs.fontSize), weight: cs.fontWeight, family: cs.fontFamily,
        whiteSpace: cs.whiteSpace, width: r.width, max68: 68 * ch, column: t.parentElement.getBoundingClientRect().width,
        top: b.getBoundingClientRect().top, bottom: b.getBoundingClientRect().bottom, left: r.left,
        scrollH: t.scrollHeight, clientH: t.clientHeight, text: (t.querySelector("[data-canonical], [data-submitted]") || t).innerText,
      };
    });
    const struck = pair.querySelector(".long-struck-text");
    const scs = struck && getComputedStyle(struck);
    const prompt = document.querySelector("[data-prompt]").getBoundingClientRect();
    return { texts, promptLeft: prompt.left,
      strike: scs && { line: scs.textDecorationLine, thickness: scs.textDecorationThickness, color: scs.textDecorationColor, anim: scs.animationName },
      missed: getComputedStyle(document.querySelector(".verdict-missed") || document.body).color };
  });
}

// Words of a text node that break across lines (each word's line boxes).
async function brokenWords(page, selector) {
  return page.evaluate((sel) => {
    const el = document.querySelector(sel);
    const out = [];
    const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      if (n.parentElement.closest(".sr")) continue;
      const re = /\S+/g;
      let m;
      while ((m = re.exec(n.data))) {
        const range = document.createRange();
        range.setStart(n, m.index); range.setEnd(n, m.index + m[0].length);
        const tops = new Set([...range.getClientRects()].filter((r) => r.width > 0).map((r) => Math.round(r.top)));
        if (tops.size > 1 && !/-/.test(m[0])) out.push(m[0]);
      }
    }
    return out;
  }, selector);
}

// Is the element fully in the window and not cut by any clipping ancestor?
async function fullyVisible(page, selector, barClear) {
  return page.evaluate(([sel, clear]) => {
    const el = document.querySelector(sel);
    if (!el) return { ok: false, why: "missing" };
    const r = el.getBoundingClientRect();
    const fixed = (() => { for (let a = el; a; a = a.parentElement) if (getComputedStyle(a).position === "fixed") return true; return false; })();
    const bottomLimit = window.innerHeight - (fixed ? 0 : clear);
    if (r.top < 0 || r.bottom > bottomLimit + 0.5 || r.left < 0 || r.right > window.innerWidth + 0.5 || r.width === 0) {
      return { ok: false, why: `rect ${Math.round(r.top)}..${Math.round(r.bottom)} limit ${Math.round(bottomLimit)}` };
    }
    for (let a = el.parentElement; a && a !== document.body; a = a.parentElement) {
      const cs = getComputedStyle(a);
      if (/(hidden|clip|auto|scroll)/.test(cs.overflowX + cs.overflowY)) {
        const ar = a.getBoundingClientRect();
        if (r.top < ar.top - 0.5 || r.bottom > ar.bottom + 0.5 || r.left < ar.left - 0.5 || r.right > ar.right + 0.5) return { ok: false, why: "clipped by " + a.className };
      }
    }
    return { ok: true, fixed };
  }, [selector, barClear]);
}

// ---- one config ---------------------------------------------------------

async function journey(c, port) {
  const config = c.name;
  const srv = startServer(BIN, config, port);
  const base = srv.base;
  const browser = await chromium.launch({ headless: false, executablePath: CHROME, args: [`--window-size=${c.viewport.width},${c.viewport.height + 140}`, "--window-position=0,0"] });
  try {
    const health = await waitHealthy(base);
    check(config, "/healthz 200 with the build sha", health.status === "ok" && (!SHA || SHA.startsWith(health.build_sha)), `status=${health.status} build_sha=${health.build_sha} mode=${health.mode}`);
    const ctx = await browser.newContext({ ...contextOptions(c), recordVideo: { dir: tmp, size: c.viewport } });
    await ctx.addInitScript(initScript);
    const page = await ctx.newPage();
    const problems = [], hosts = new Set();
    instrument(page, problems, hosts);

    // ---- Sign up, then make an API token in Settings --------------------
    await page.goto(base + "/signup");
    await page.fill("#email", `${config}-${Date.now()}@example.com`);
    await page.fill("#password", "correct horse battery");
    await Promise.all([page.waitForURL(/\/shelf/), press(page, c, page.locator("main button.btn-accent"))]);
    await page.goto(base + "/settings");
    await page.fill("input[name=name]", "session miner");
    await Promise.all([page.waitForLoadState("domcontentloaded"), press(page, c, page.locator("text=Create token"))]);
    const token = (await page.textContent(".token-value")).trim();
    check(config, "settings: a new API token is shown once", /^scry_/.test(token));
    await pageHealth(page, config, "settings");

    // ---- The API: the fixture, the old shape, refusals ------------------
    const api = async (method, p, body, raw) => {
      const res = await ctx.request.fetch(base + p, { method, headers: { Authorization: "Bearer " + token, "Content-Type": "application/json" }, data: raw ?? JSON.stringify(body) });
      let js = {};
      try { js = await res.json(); } catch {}
      return [res.status(), js];
    };
    let [st, js] = await api("POST", "/api/v1/items", { items: [{ question: "What does TTL stand for?", answer: "Time to live", idea: "API cards" }] });
    check(config, "[S139-2] api: a request in the old shape gets the same 201 response", st === 201 && js.added === 1 && js.status === "in the library" && Object.keys(js).sort().join() === "added,ids,status", `${st} ${JSON.stringify(js)}`);
    [st, js] = await api("POST", "/api/v1/items", { items: [{ question: "Q", answer: "A", idea: "X" }, { question: "Q2", answer: "A2", idea: "X", resources: [{ title: "T", kind: "link", url: "javascript:alert(1)" }] }] });
    check(config, "[S139-2] api: a javascript: url is refused with 422 naming the item and field", st === 422 && /^item 2, resource 1: url must be an absolute/.test(js.error), `${st} ${js.error}`);
    [st, js] = await api("POST", "/api/v1/items", { items: [{ question: "Q", answer: "A", idea: "X", resources: [{ title: "T", kind: "podcast", body: "b" }] }] });
    check(config, "[S139-2] api: an unknown kind is refused with 422", st === 422 && /kind must be explainer, article, video, diagram or link/.test(js.error), `${st} ${js.error}`);
    [st] = await api("POST", "/api/v1/items", { items: [{ question: "Q", answer: "A", idea: "X", resources: [{ title: "T", kind: "link", url: "https://a.example", extra: 1 }] }] });
    check(config, "[S139-2] api: an unknown field still gets 400", st === 400, String(st));
    [st, js] = await api("POST", "/api/v1/items", FIXTURE);
    check(config, "[S139-2] api: the fixture with resources, ideaResources, relatedTags and relatedIdeas is added (201)", st === 201 && js.added === FIXTURE.items.length, `${st} ${JSON.stringify(js)}`);
    if (st === 400) {
      // A build from before [S139-2] refuses the new fields: add the same
      // cards without them, so the review itself is still checked (the
      // negative control's [S139-1] and [S139-2] review checks).
      const legacy = { ...FIXTURE, items: FIXTURE.items.map(({ resources, ideaResources, relatedTags, relatedIdeas, ...it }) => it) };
      [st, js] = await api("POST", "/api/v1/items", legacy);
      check(config, "fixture without the [S139-2] fields added instead (pre-change build)", st === 201, `${st} ${JSON.stringify(js)}`);
    }
    [st, js] = await api("GET", "/api/v1/ideas");
    check(config, "[S139-2] api: refused requests added nothing", st === 200 && !js.ideas.some((i) => i.title === "X"), JSON.stringify(js.ideas?.map((i) => i.title)));

    // ---- Study: work through the session --------------------------------
    await page.goto(base + "/study");
    await pageHealth(page, config, "study");
    const seen = new Set();
    for (let step = 0; step < 12; step++) {
      const asking = await page.$("[data-asking]");
      if (!asking) break;
      const prompt = (await page.textContent("#prompt")).trim();
      seen.add(prompt);
      const answer = ANSWERS.get(prompt) ?? "x";
      const isLong = prompt === LONG.question;

      if (isLong) {
        // Nothing of Related & resources while the card waits.
        const html = await page.content();
        const outside = html.replace(/<ol class="(scrollback|upcoming)"[\s\S]*?<\/ol>/g, "");
        const text = await page.evaluate(() => document.body.innerText);
        const related = [SAME, TAGGED, LINKED].map((i) => i.question);
        const leaks = ["Related &amp; resources", "Related & resources", "data-connected", ...RESOURCE_TITLES, ...RESOURCE_HOSTS].filter((s) => html.includes(s) || text.includes(s));
        const relLeaks = related.filter((q) => outside.includes(q));
        check(config, "[S139-2] while the card waits, its page text and markup hold no \"Related & resources\" and no resource title", leaks.length === 0, leaks.join(" | "));
        check(config, "[S139-2] while the card waits, no related question appears outside the session's own scrollback and coming-up cues", relLeaks.length === 0, relLeaks.join(" | "));
        await shot(page, `${config}-long-waiting`);
      }

      // Type the answer (Shift+Enter for a line break) and press Enter.
      const field = page.locator("[data-answer-input]");
      if (c.phone) await field.tap(); else await field.click();
      const lines = answer.split("\n");
      for (let i = 0; i < lines.length; i++) {
        if (i) await page.keyboard.press("Shift+Enter");
        await page.keyboard.type(lines[i], { delay: 8 });
      }
      await Promise.all([page.waitForSelector("[data-review]", { timeout: 15000 }), page.keyboard.press("Enter")]);
      await sleep(450);

      const kind = await page.getAttribute("[data-review]", "data-kind");
      const last = (await page.textContent("[data-next]")).includes("Finish");
      if (prompt === EXACT.question) {
        check(config, "[S139-2] an exact text still has no \"Ask Why?\"", kind === "exact" && !(await page.$("[data-explain]")), kind);
      } else if (isLong) {
        await longReview(page, c, config, last);
      } else if (prompt === SAME.question || prompt === TAGGED.question) {
        const p = await longPair(page);
        const why = prompt === SAME.question ? "a learner answer of 80 or fewer characters with a line break" : "a long learner answer with a short correct answer";
        check(config, `[S139-1] ${why} also stacks, "Your answer" over "Correct answer", at Title size`,
          !p.missing && p.texts.length === 2 && p.texts[0].label === "Your answer" && p.texts[1].label === "Correct answer" && p.texts.every((t) => t.size === "24px") && p.texts[1].top - p.texts[0].bottom >= 15.5 && p.texts[1].top - p.texts[0].bottom <= 16.5,
          p.missing ? "no long pair" : p.texts.map((t) => `${t.label} ${t.size} ${Math.round(t.top)}-${Math.round(t.bottom)}`).join(" / "));
        if (prompt === SAME.question) {
          check(config, "[S139-1] line breaks are kept in the struck answer", p.texts && /best\s*\n\s*option/.test(p.texts[0].text), JSON.stringify(p.texts?.[0]?.text));
          await shot(page, `${config}-linebreak-review`);
        }
      } else if (prompt === ONEWORD.question) {
        const j = await page.evaluate(() => {
          const s = document.querySelector(".judgment-struck"), r = document.querySelector(".judgment-right");
          return s && r && { long: !!document.querySelector("[data-long-pair]"), s: getComputedStyle(s).fontSize, r: getComputedStyle(r).fontSize,
            judgment: getComputedStyle(document.documentElement).getPropertyValue("--judgment"), st: s.getBoundingClientRect().top, rt: r.getBoundingClientRect().top,
            sl: s.getBoundingClientRect().left, rl: r.getBoundingClientRect().left };
        });
        check(config, "[S139-1] two short answers keep Judgment size (not stacked as a long pair)", j && !j.long && j.s === j.r && parseFloat(j.s) >= 72, JSON.stringify(j));
        if (!c.phone) check(config, "[S139-1] two short answers sit side by side on desktop", j && Math.abs(j.st - j.rt) < 2 && j.rl > j.sl, JSON.stringify(j));
        await shot(page, `${config}-short-review`);
      }
      const rb = await reviewBoxes(page);
      check(config, `review of "${prompt.slice(0, 32)}…": nothing scrolls inside the review or sideways`, rb.scrollers.length === 0 && rb.sideways <= 0, JSON.stringify(rb));

      // Next card (Enter on the review; the button has focus).
      await page.evaluate(() => window.scrollTo(0, 0));
      const nextBtn = page.locator("[data-next]");
      await Promise.all([page.waitForResponse((r) => new URL(r.url()).pathname === "/study/next" && r.request().method() === "POST"), press(page, c, nextBtn)]);
      await sleep(500);
    }
    check(config, "the whole fixture session was studied", FIXTURE.items.every((i) => seen.has(i.question)), [...seen].length + " cards");
    await pageHealth(page, config, "session end");

    // ---- The long card's own page and its idea's page -------------------
    await page.goto(base + "/library?q=" + encodeURIComponent("opportunity cost, and why"));
    const cardHref = await page.getAttribute(".item-row a.row-main", "href");
    await page.goto(base + cardHref);
    const card = await page.evaluate(() => {
      const d = document.querySelector("details.connected");
      return d && { open: d.open, summary: d.querySelector("summary").innerText.replace(/\s+/g, " ").trim(), rows: [...d.querySelectorAll("[data-connected-row]")].map((r) => r.dataset.connectedRow),
        afterAnswer: !!document.querySelector("#answer") && (document.querySelector("#answer").compareDocumentPosition(d) & Node.DOCUMENT_POSITION_FOLLOWING) > 0 };
    });
    check(config, "[S139-2] the card's own page shows the same closed disclosure under the card's answer", card && !card.open && /Related & resources · 3 cards · 3 resources/.test(card.summary) && card.afterAnswer && card.rows.join() === "related,related,related,resource,resource,resource", JSON.stringify(card));
    await pageHealth(page, config, "card page");
    await press(page, c, page.locator("details.connected summary"));
    await sleep(350);
    await shot(page, `${config}-card-page-open`);
    await page.goto(base + (await page.getAttribute("main .reading > p.meta a", "href")));
    const idea = await page.evaluate(() => {
      const d = document.querySelector("details.connected");
      return d && { open: d.open, summary: d.querySelector("summary").innerText.replace(/\s+/g, " ").trim(), rows: [...d.querySelectorAll(".connected-kind")].map((k) => k.textContent),
        afterSummary: (document.querySelector(".idea-summary").compareDocumentPosition(d) & Node.DOCUMENT_POSITION_FOLLOWING) > 0 };
    });
    check(config, "[S139-2] the idea's page shows its own resources only, closed, under its summary", idea && !idea.open && /Related & resources · 1 resource$/.test(idea.summary.replace(/ (Show|Hide)$/, "")) && idea.rows.join() === "DIAGRAM" && idea.afterSummary, JSON.stringify(idea));
    await pageHealth(page, config, "idea page");
    await shot(page, `${config}-idea-page`);

    const leakHosts = [...hosts].filter((h) => RESOURCE_HOSTS.includes(h));
    check(config, "[S139-2] the browser sent no request to a resource's host", leakHosts.length === 0, leakHosts.join(" "));
    check(config, "0 console errors, page errors, failed requests or responses >= 400", problems.length === 0, problems.slice(0, 8).join(" | "));
    const video = page.video();
    await ctx.close();
    if (video) {
      const webm = path.join(MEDIA, `${PREFIX}-${config}-walkthrough.webm`);
      await video.saveAs(webm);
      try {
        execFileSync("ffmpeg", ["-y", "-loglevel", "error", "-i", webm, "-c:v", "libx264", "-crf", "30", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-movflags", "+faststart", webm.replace(/\.webm$/, ".mp4")]);
        fs.rmSync(webm);
      } catch {}
    }
  } catch (e) {
    check(config, "journey ran to the end", false, e.stack || String(e));
  } finally {
    await browser.close().catch(() => {});
    srv.proc.kill("SIGTERM");
  }
}

// The long card's review: [S139-1], then [S139-2]'s disclosure and Ask Why?.
async function longReview(page, c, config, last) {
  check(config, "[S139-1] the long card is graded Incorrect with another card still to come", (await page.getAttribute("[data-verdict]", "data-verdict")) === "missed" && !last);
  const p = await longPair(page);
  check(config, "[S139-1] both answers stack, \"Your answer\" over \"Correct answer\"", !p.missing && p.texts.length === 2 && p.texts[0].label === "Your answer" && p.texts[1].label === "Correct answer" && p.texts[0].bottom <= p.texts[1].top,
    p.missing ? "no long pair" : p.texts.map((t) => t.label).join(" / "));
  if (p.missing) return;
  const gap = p.texts[1].top - p.texts[0].bottom;
  check(config, "[S139-1] 16 px apart", Math.abs(gap - 16) < 0.6, gap.toFixed(2) + "px");
  check(config, "[S139-1] each in Text at Title size (24 px, weight 500) with a 1.55 line height", p.texts.every((t) => t.size === "24px" && t.weight === "500" && Math.abs(t.lh - 1.55) < 0.01 && /Geist/.test(t.family) && !/Mono/.test(t.family)),
    p.texts.map((t) => `${t.size}/${t.lh.toFixed(3)} ${t.weight}`).join(" · "));
  check(config, "[S139-1] each no wider than 68ch or its column", p.texts.every((t) => t.width <= t.max68 + 0.5 && t.width <= t.column + 0.5), p.texts.map((t) => `${Math.round(t.width)} <= ${Math.round(t.max68)}, ${Math.round(t.column)}`).join(" · "));
  check(config, "[S139-1] each under its own Meta mono label in ink-muted", p.texts.every((t) => /Mono/.test(t.labelFont) && t.labelColor === t.inkMuted), p.texts.map((t) => t.labelFont.slice(0, 16) + " " + t.labelColor).join(" · "));
  check(config, "[S139-1] left-aligned with the question", p.texts.every((t) => Math.abs(t.left - p.promptLeft) < 1), p.texts.map((t) => Math.round(t.left)).join(",") + " vs " + Math.round(p.promptLeft));
  check(config, "[S139-1] line breaks kept (white-space: pre-wrap)", p.texts.every((t) => t.whiteSpace === "pre-wrap"));
  check(config, "[S139-1] the struck answer: a 2 px missed-red line through every line, fading in (no drawn clip)", p.strike && p.strike.line.includes("line-through") && p.strike.thickness === "2px" && p.strike.color === p.missed && p.strike.anim === "none", JSON.stringify(p.strike) + " missed=" + p.missed);
  check(config, "[S139-1] no review text is clipped (each answer's scrollHeight <= clientHeight)", p.texts.every((t) => t.scrollH <= t.clientH), p.texts.map((t) => `${t.scrollH}/${t.clientH}`).join(" · "));
  check(config, "[S139-1] the correct answer is whole: 250–400 characters, three or more sentences", p.texts[1].text.trim() === LONG.answer && LONG.answer.length >= 250 && LONG.answer.length <= 400 && LONG.answer.split(/(?<=\.)\s+/).length >= 3, LONG.answer.length + " chars");
  const broken = await brokenWords(page, "[data-long-correct]");
  check(config, "[S139-1] no word of the fixture breaks across lines except after a hyphen", broken.length === 0, broken.join(" "));
  const rb = await reviewBoxes(page);
  check(config, "[S139-1] no element in the review scrolls inside itself (overflow auto|scroll with more content than room)", rb.scrollers.length === 0, rb.scrollers.join(" "));
  check(config, "[S139-1] no review text is cut off by overflow hidden|clip", rb.clipped.length === 0, rb.clipped.join(" "));
  check(config, "[S139-1] nothing scrolls sideways (scrollWidth <= innerWidth)", rb.sideways <= 0, rb.sideways + "px");
  const doc = await page.evaluate(() => ({ minH: getComputedStyle(document.body).minHeight, scroller: document.scrollingElement === document.documentElement, y: window.scrollY }));
  check(config, "[S139-1] the page fills at least the window (min-height: 100dvh) and scrolls with the window", parseFloat(doc.minH) >= (c.phone ? 844 : 900) - 1 && doc.scroller, JSON.stringify(doc));
  const firstWindow = await page.evaluate(() => {
    window.scrollTo(0, 0);
    const sel = ["[data-prompt]", ".long-yours", ".long-correct", ".verdict-badge", ".why"];
    return sel.map((s) => { const r = document.querySelector(s).getBoundingClientRect(); return { s, bottom: Math.round(r.bottom), ok: r.top >= 0 && r.bottom <= window.innerHeight }; });
  });
  if (!c.phone) {
    check(config, "[S139-1] at 1440x900 the question, both answers, the badge and the why are inside the first window", firstWindow.every((f) => f.ok), JSON.stringify(firstWindow));
    check(config, "[S139-1] at 1440x900 the page is at most 1800 px tall", rb.height <= 1800, rb.height + "px");
  } else {
    check(config, "[S139-1] at 390x844 the page is at most 2532 px tall", rb.height <= 2532, rb.height + "px");
    const bar = await page.evaluate(() => { const f = document.querySelector(".judged-actions"), r = f.getBoundingClientRect(), b = document.querySelector("[data-next]").getBoundingClientRect(); return { pos: getComputedStyle(f).position, bottom: Math.round(r.bottom), h: window.innerHeight, inBar: b.top >= r.top && b.bottom <= r.bottom }; });
    check(config, "[S139-1] at 390x844 \"Next card\" stays in its pinned bar", bar.pos === "fixed" && Math.abs(bar.bottom - bar.h) <= 1 && bar.inBar, JSON.stringify(bar));
  }
  await shot(page, `${config}-long-review`);
  await shot(page, `${config}-long-review-full`, true);

  // Scrolling the page, not any inner box, reaches each control.
  const barH = c.phone ? await page.evaluate(() => document.querySelector(".judged-actions").getBoundingClientRect().height) : 0;
  for (const [label, sel] of [["Ask Why?", "[data-explain]"], ["the schedule table", "[data-telemetry]"], ["Edit card", "[data-edit-toggle]"], ["Snooze", ".review-foot [data-snooze]"], ["Delete", ".review-foot [data-delete]"], ["Next card", "[data-next]"]]) {
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.locator(sel).scrollIntoViewIfNeeded();
    await page.evaluate(([s, clear]) => {
      const r = document.querySelector(s).getBoundingClientRect();
      const fixed = getComputedStyle(document.querySelector(s).closest(".judged-actions") || document.body).position === "fixed";
      if (!fixed && r.bottom > window.innerHeight - clear) window.scrollBy(0, r.bottom - (window.innerHeight - clear) + 8);
    }, [sel, barH]);
    await sleep(120);
    const v = await fullyVisible(page, sel, barH);
    check(config, `[S139-1] scrolling the page reaches ${label}, fully visible and unclipped`, v.ok, v.why || (v.fixed ? "in the pinned bar" : ""));
  }
  await page.evaluate(() => window.scrollTo(0, 0));

  // ---- [S139-2] Related & resources -----------------------------------
  const d0 = await page.evaluate(() => {
    const d = document.querySelector("details.connected");
    if (!d) return null;
    const s = d.querySelector("summary"), cs = getComputedStyle(d), dc = getComputedStyle(d, "::details-content");
    const explain = document.querySelector("[data-explain-form]"), sched = document.querySelector("[data-telemetry]");
    return { open: d.hasAttribute("open"), text: s.innerText.replace(/\s+/g, " ").trim(), h: s.getBoundingClientRect().height,
      rule: cs.borderTopWidth + " " + cs.borderTopStyle, nameFont: getComputedStyle(s.querySelector(".connected-name")).fontFamily,
      show: s.querySelector(".connected-show").getBoundingClientRect().right, right: s.getBoundingClientRect().right,
      dur: dc.transitionDuration, ease: dc.transitionTimingFunction, prop: dc.transitionProperty,
      between: (explain.compareDocumentPosition(d) & Node.DOCUMENT_POSITION_FOLLOWING) > 0 && (d.compareDocumentPosition(sched) & Node.DOCUMENT_POSITION_FOLLOWING) > 0 };
  });
  check(config, "[S139-2] after a graded answer the disclosure is there, closed (no open)", d0 && !d0.open, JSON.stringify(d0));
  if (!d0) return;
  check(config, "[S139-2] its summary names the counts: \"Related & resources · 3 cards · 3 resources\" and \"Show\" at the right edge", d0.text === "Related & resources · 3 cards · 3 resources Show" && Math.abs(d0.show - d0.right) < 1, d0.text);
  check(config, "[S139-2] the closed row is at least 44 px tall under a 1 px line rule, in Meta mono", d0.h >= 44 && d0.rule === "1px solid" && /Mono/.test(d0.nameFont), `${d0.h}px ${d0.rule} ${d0.nameFont.slice(0, 16)}`);
  check(config, "[S139-2] between Ask Why? and the schedule", d0.between);
  check(config, "[S139-2] its computed transition is 200 ms ease-out (height and opacity)", /0\.2s/.test(d0.dur) && /cubic-bezier\(0\.2, 0, 0, 1\)/.test(d0.ease) && /block-size/.test(d0.prop) && /opacity/.test(d0.prop), `${d0.dur} ${d0.ease} ${d0.prop}`);
  check(config, "[S139-2] no related card's answer appears in the review", !(await page.evaluate((a) => a.some((x) => document.body.innerText.includes(x) || document.querySelector("details.connected").innerHTML.includes(x)), [SAME.answer, TAGGED.answer, LINKED.answer])));

  // Open by click (or tap).
  const summary = page.locator("details.connected summary");
  const y0 = await page.evaluate(() => window.scrollY);
  await summary.scrollIntoViewIfNeeded();
  const y1 = await page.evaluate(() => window.scrollY);
  await press(page, c, summary);
  await sleep(60);
  const mid = await page.evaluate(() => document.querySelector("details.connected").getBoundingClientRect().height);
  await sleep(400);
  const opened = await page.evaluate(() => {
    const d = document.querySelector("details.connected");
    const rows = [...d.querySelectorAll("[data-connected-row]")].map((r) => {
      const cs = getComputedStyle(r), k = r.querySelector(".connected-kind"), kcs = getComputedStyle(k);
      return { kind: k.textContent, row: r.dataset.connectedRow, rule: cs.borderTopWidth + " " + cs.borderTopStyle, shadow: cs.boxShadow, bg: cs.backgroundColor,
        kindFont: kcs.fontFamily, kindW: k.getBoundingClientRect().width, kindTop: k.getBoundingClientRect().top, textTop: r.querySelector(".connected-main").getBoundingClientRect().top,
        pad: cs.paddingTop + " " + cs.paddingBottom };
    });
    const links = [...d.querySelectorAll("a")].map((a) => ({ href: a.getAttribute("href"), target: a.getAttribute("target"), rel: a.getAttribute("rel"), related: a.hasAttribute("data-related-link"), name: a.getAttribute("aria-label") || a.textContent }));
    const list = d.querySelector(".connected-list");
    return { open: d.open, show: d.querySelector(".connected-show").innerText.trim(), rows, links, height: d.getBoundingClientRect().height, y: window.scrollY,
      listW: list.getBoundingClientRect().width, focus: document.activeElement === d.querySelector("summary") || !document.activeElement || document.activeElement === document.body };
  });
  const expectRows = "RELATED,RELATED,RELATED,ARTICLE,EXPLAINER,DIAGRAM";
  check(config, `[S139-2] opening it by ${c.phone ? "tap" : "click"} lists RELATED rows first, then resources, each with its mono kind label`,
    opened.open && opened.rows.map((r) => r.kind).join() === expectRows && opened.rows.every((r) => /Mono/.test(r.kindFont)), opened.rows.map((r) => r.kind).join());
  check(config, "[S139-2] \"Show\" reads \"Hide\" while open", opened.show === "Hide", opened.show);
  check(config, "[S139-2] rows divided by 1 px line rules, 12 px above and below, no box-shadow and no background fill",
    opened.rows.every((r) => r.rule === "1px solid" && r.shadow === "none" && /rgba\(0, 0, 0, 0\)|transparent/.test(r.bg) && r.pad === "12px 12px"), JSON.stringify(opened.rows[0]));
  check(config, c.phone ? "[S139-2] on phones the kind sits above the row's text" : "[S139-2] the kind sits in an 88 px column",
    c.phone ? opened.rows.every((r) => r.kindTop < r.textTop) : opened.rows.every((r) => Math.abs(r.textTop - r.kindTop) < 6 && r.kindW <= 88.5), JSON.stringify(opened.rows.map((r) => [Math.round(r.kindW), Math.round(r.kindTop - r.textTop)])));
  const rel = opened.links.filter((l) => l.related), res = opened.links.filter((l) => !l.related);
  check(config, "[S139-2] related links go to /library/items/{id}, named by the whole question", rel.length === 3 && rel.every((l) => /^\/library\/items\/\d+$/.test(l.href)) && rel.map((l) => l.name).join("|") === [SAME, TAGGED, LINKED].map((i) => i.question).join("|"), JSON.stringify(rel));
  check(config, "[S139-2] resource links have an http(s) URL, target=_blank and rel=\"noopener noreferrer nofollow\"", res.length === 1 && res.every((l) => /^https?:\/\//.test(l.href) && l.target === "_blank" && l.rel === "noopener noreferrer nofollow"), JSON.stringify(res));
  check(config, "[S139-2] opening it never scrolls the page", opened.y === y1, `${y1} -> ${opened.y}`);
  check(config, "[S139-2] it opens in height (mid-transition shorter than open)", mid < opened.height - 4, `${Math.round(mid)} -> ${Math.round(opened.height)}`);
  check(config, "[S139-2] the open list sits at the reading measure", opened.listW > 0 && opened.listW <= (c.phone ? 390 : 900), Math.round(opened.listW) + "px");
  const rbOpen = await reviewBoxes(page);
  check(config, "[S139-2] open, nothing scrolls inside the review or sideways", rbOpen.scrollers.length === 0 && rbOpen.sideways <= 0 && rbOpen.clipped.length === 0, JSON.stringify(rbOpen));
  await page.locator("details.connected").scrollIntoViewIfNeeded();
  await shot(page, `${config}-related-open`);
  await press(page, c, summary);
  await sleep(350);
  check(config, "[S139-2] pressing it again closes it", !(await page.evaluate(() => document.querySelector("details.connected").open)));

  // Keyboard: reached by Tab from Ask Why?, Space and Enter toggle it,
  // Space never goes to Next card.
  await page.focus("[data-explain]");
  await page.keyboard.press("Tab");
  const tabbed = await page.evaluate(() => document.activeElement?.matches("details.connected > summary"));
  check(config, "[S139-2] it is reached by Tab (right after Ask Why?)", tabbed);
  const url0 = page.url();
  await page.keyboard.press("Space");
  await sleep(350);
  const kb = await page.evaluate(() => ({ open: document.querySelector("details.connected")?.open, review: !!document.querySelector("[data-review]"), focus: document.activeElement?.matches("details.connected > summary") }));
  check(config, "[S139-2] Space opens it and never goes to Next card; focus stays on it", kb.open && kb.review && kb.focus && page.url() === url0, JSON.stringify(kb));
  await page.keyboard.press("Enter");
  await sleep(350);
  check(config, "[S139-2] Enter toggles it closed", !(await page.evaluate(() => document.querySelector("details.connected").open)));

  // Under emulated reduced motion it opens at once.
  await page.emulateMedia({ reducedMotion: "reduce" });
  const rm = await page.evaluate(async () => {
    const d = document.querySelector("details.connected");
    const dur = getComputedStyle(d, "::details-content").transitionDuration;
    d.querySelector("summary").click();
    await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
    const at = d.getBoundingClientRect().height;
    await new Promise((r) => setTimeout(r, 300));
    const end = d.getBoundingClientRect().height;
    d.querySelector("summary").click();
    return { dur, at, end };
  });
  check(config, "[S139-2] under emulated reduced motion it opens at once", /^0s/.test(rm.dur) && Math.abs(rm.at - rm.end) < 1, JSON.stringify(rm));
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await sleep(250);

  // Ask Why? names the gap to rebuild.
  await page.evaluate(() => window.scrollTo(0, 0));
  await press(page, c, page.locator("[data-explain]"));
  await page.waitForSelector("[data-explanation]:not([hidden])", { timeout: 10000 });
  await sleep(300);
  const ex = (await page.textContent("[data-explanation]")).trim();
  check(config, "[S139-2] Ask Why? on Incorrect ends in one \"Rebuild:\" sentence within 280 characters", ex.length <= 280 && (ex.match(/Rebuild:/g) || []).length === 1 && /Rebuild: [^.]*\([^)]*\)\.$|Rebuild: [^.]+\.$/.test(ex) && ex.startsWith("What differed:"), ex);
  const rbWhy = await reviewBoxes(page);
  check(config, "[S139-1] with the explanation shown, nothing scrolls inside the review or sideways", rbWhy.scrollers.length === 0 && rbWhy.sideways <= 0, JSON.stringify(rbWhy));
  await shot(page, `${config}-ask-why`);
}

const CONFIGS = [
  { name: "desktop-light", viewport: { width: 1440, height: 900 }, colorScheme: "light" },
  { name: "desktop-dark", viewport: { width: 1440, height: 900 }, colorScheme: "dark" },
  { name: "phone-light", viewport: { width: 390, height: 844 }, colorScheme: "light", phone: true },
  { name: "phone-dark", viewport: { width: 390, height: 844 }, colorScheme: "dark", phone: true },
].filter((c) => !ONLY || ONLY.includes(c.name));

let port = PORT0;
for (const c of CONFIGS) await journey(c, port++);
fs.writeFileSync(path.join(MEDIA, `${PREFIX}-results.json`), JSON.stringify({ sha: SHA, bin: path.basename(BIN), checks: results.length, failed, results }, null, 2));
console.log(`\n${results.length - failed}/${results.length} checks PASS`);
if (EXPECT_FAIL) {
  const n = results.filter((r) => /^\[S139-/.test(r.name) && !r.ok).length;
  console.log(`negative control: ${n} [S139-*] checks failed`);
  process.exit(n > 0 ? 0 : 1);
}
process.exit(failed ? 1 : 0);
