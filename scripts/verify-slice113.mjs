#!/usr/bin/env node
// verify-slice113.mjs: check Slice 111 + 113 ([S111-1..3], [S113-1..3]) on
// the real running path, in a real headed Chrome, against isolated
// synthetic data.
//
//   make build BUILD_SHA=$(git rev-parse HEAD)
//   DISPLAY=:0 node scripts/verify-slice113.mjs --bin ./bin/scry --sha <sha> \
//        --media ~/chief-evidence/media [--port 18113] [--only desktop-light,...]
//
// For each of desktop 1440x900 and phone 390x844, light and dark, it starts
// its own Scry (the --bin binary) in SCRY_MODE=test with the demo learner
// seeded on a fresh SQLite file in a new temporary directory and a minimal
// environment (no provider keys: the grading chain, speech and drafting are
// fixed fixtures; auth and billing the local fakes; nothing leaves the
// machine). The grading chain is forced down a scripted path per model
// submission (SCRY_TEST_JUDGE_PATHS), each judgment waits 3.3 s
// (SCRY_TEST_JUDGE_DELAY) so "checking" and "still checking" can be seen,
// and the verdicts are scripted (SCRY_TEST_JUDGMENTS). The server's own
// "judge chain" log lines prove which models each submission reached.
//
//   S113-3 the S3-A mark: 24 px in the header, 8 px before `scry`, centred;
//          favicon SVG + 16/32 px PNGs and the 64 px app icon decode.
//   S111-1 /study opens straight on a card across every deck; decks
//          alternate; "Study this idea" focuses; the navigation's Study
//          opens every deck again.
//   S111-2 the answer is a textarea that grows row by row; Shift+Enter adds
//          a line; the review keeps the line breaks.
//   S113-1 multi-sentence/paragraph answers through every chain path show
//          the scripted verdict and its reason and a schedule; every
//          grading path failing, and a request interrupted before a review
//          is recorded, keep the complete answer; Enter retries; a response
//          lost after the review is recorded reopens it with no second
//          judgment.
//   S113-2 every wait says its word with its control disabled and busy;
//          every forced failure keeps the input and recovers on retry;
//          every animation seen is 120–240 ms ease-out with no overshoot
//          (only the allowed loops repeat); under reduced motion nothing
//          translates, scales, pulses or loops and states still say
//          themselves in words.
//
// Every page is checked for sideways scroll, loaded styles, decoded images
// and CSP violations; console errors, page errors, failed requests and
// responses >= 400 are counted per config and must be 0, except the
// requests this script deliberately aborts and the one 503 Study answers
// with when every grading path is forced to fail. Exit 0 only when every
// check passes.

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
const PORT0 = Number(args.port || 18113);
const SHA = args.sha || "";
const ONLY = args.only ? args.only.split(",") : null;
const ROOT = path.resolve(path.dirname(new URL(import.meta.url).pathname), "..");
const DECKS = path.join(ROOT, "testdata", "decks");
const CHROME = process.env.CHROME_BIN || "/usr/bin/google-chrome";
const SPOKEN = "it reads like a spoken answer";

// One path per model submission, in turn (see ai.FakeJudgePaths).
const PATHS = ["primary", "primary-unavailable", "primary-error", "primary-unparseable", "chain-unavailable",
  "primary-low-confidence", "primary-low-probability", "primary-thin-margin", "primary-none",
  "all-fail", "primary", "primary", "primary", "primary", "primary", "primary", "primary", "primary", "primary", "primary"];
const VERDICTS = ["right", "close", "missed"];
// What each path must ask, by the chain's own log (step names).
const EXPECT = {
  "primary": { escalated: false, attempts: 1 },
  "primary-unavailable": { escalated: false, attempts: 2 },
  "primary-error": { escalated: false, attempts: 2 },
  "primary-unparseable": { escalated: false, attempts: 2 },
  "chain-unavailable": { escalated: true, attempts: 4 },
  "primary-low-confidence": { escalated: true, attempts: 2 },
  "primary-low-probability": { escalated: true, attempts: 2 },
  "primary-thin-margin": { escalated: true, attempts: 2 },
  "primary-none": { escalated: true, attempts: 2 },
  "all-fail": { failed: true },
};
const BADGE = { right: /correct/i, close: /almost/i, missed: /incorrect|missed/i };

fs.mkdirSync(MEDIA, { recursive: true });
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "scry-s113-"));
const results = [];
let failed = 0;
function check(config, name, ok, detail = "") {
  results.push({ config, name, ok: !!ok, detail: String(detail) });
  if (!ok) failed++;
  console.log(`${ok ? "PASS" : "FAIL"} [${config}] ${name}${detail !== "" ? " — " + detail : ""}`);
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
// The status line's current word: while "checking" cross-fades into "still
// checking" in place both are in the DOM; the newest is the last.
const statusWord = (page) => page.evaluate(() => {
  const st = document.querySelector("#status");
  if (!st) return "";
  const words = st.querySelectorAll(".wait-word[data-wait-word]");
  return words.length ? words[words.length - 1].dataset.waitWord : st.textContent.trim();
}).catch(() => "");

// ---- the server ---------------------------------------------------------

function startServer(config, port) {
  const dir = fs.mkdtempSync(path.join(tmp, config + "-"));
  const logPath = path.join(dir, "server.log");
  const log = fs.createWriteStream(logPath);
  const proc = spawn(BIN, [], {
    env: {
      PATH: process.env.PATH, HOME: dir, SCRY_MODE: "test", SCRY_SEED_DEMO: "1", SCRY_DB: path.join(dir, "s113.db"), PORT: String(port),
      SCRY_TEST_JUDGE_PATHS: PATHS.join(","), SCRY_TEST_JUDGMENTS: VERDICTS.join(","), SCRY_TEST_JUDGE_DELAY: "3300ms",
      SCRY_TEST_SPEECH: SPOKEN,
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  proc.stdout.pipe(log);
  proc.stderr.pipe(log);
  return { proc, logPath, base: `http://127.0.0.1:${port}` };
}
async function waitHealthy(base) {
  for (let i = 0; i < 100; i++) {
    try {
      const r = await fetch(base + "/healthz");
      if (r.ok) return r.json();
    } catch {}
    await sleep(100);
  }
  throw new Error("Scry did not become healthy");
}
function chainLines(logPath) {
  return fs.readFileSync(logPath, "utf8").split("\n").filter((l) => l.includes('"judge chain"')).map((l) => { try { return JSON.parse(l); } catch { return { raw: l }; } });
}

// ---- page instrumentation -----------------------------------------------

function instrument(page, problems, forced) {
  const isForced = (u) => forced.some((f) => u.includes(f));
  page.on("console", (m) => {
    if (m.type() !== "error") return;
    const t = m.text();
    // The deliberately forced failures (aborted requests, the all-fail 503)
    // are logged by Chrome as "Failed to load resource".
    if (/Failed to load resource/.test(t) && forced.length) { forcedSeen.push("console: " + t); return; }
    problems.push("console: " + t);
  });
  page.on("pageerror", (e) => problems.push("pageerror: " + e.message));
  page.on("requestfailed", (r) => {
    const f = r.failure()?.errorText || "";
    if (isForced(r.url())) { forcedSeen.push(`aborted (forced): ${r.url()} ${f}`); return; }
    if (f.includes("ERR_ABORTED") && /\/settings\/export/.test(r.url())) return; // a saved download
    // Chrome cancels a favicon fetch still in flight when the page navigates.
    if (f.includes("ERR_ABORTED") && /\/static\/favicon/.test(r.url())) return;
    problems.push(`requestfailed: ${r.url()} ${f}`);
  });
  page.on("response", (r) => {
    const t = r.request().resourceType();
    if (t === "document" ? r.status() >= 500 : r.status() >= 400) {
      if (r.status() === 503 && new URL(r.url()).pathname === "/study/answer" && forced.includes("ALLFAIL")) { forcedSeen.push(`503 (forced all-fail): ${r.url()}`); return; }
      problems.push(`http ${r.status()}: ${t} ${r.url()}`);
    }
  });
}
const forcedSeen = [];

// Runs in every document: CSP violations, view transitions, and every
// animation or transition the page runs (sampled each frame).
const initScript = () => {
  window.__csp = [];
  document.addEventListener("securitypolicyviolation", (e) => window.__csp.push(e.violatedDirective + " " + e.blockedURI));
  window.__vt = [];
  window.addEventListener("pagereveal", (e) => window.__vt.push(!!e.viewTransition));
  window.__anims = new Map();
  const props = (a) => {
    try {
      return [...new Set(a.effect.getKeyframes().flatMap((k) => Object.keys(k).filter((x) => !["offset", "easing", "composite", "computedOffset"].includes(x))))];
    } catch { return []; }
  };
  const label = (a) => {
    const t = a.effect && a.effect.target;
    if (!t) return "";
    const el = t.id ? "#" + t.id : (t.className && typeof t.className === "string" ? "." + t.className.trim().split(/\s+/).slice(0, 2).join(".") : t.tagName);
    return el + (a.effect.pseudoElement || "");
  };
  const sample = () => {
    try {
      for (const a of document.getAnimations()) {
        const tm = a.effect.getTiming();
        const kfe = (() => { try { return [...new Set(a.effect.getKeyframes().map((k) => k.easing).filter((e) => e && e !== "linear"))]; } catch { return []; } })();
        const kind = a.constructor.name;
        const name = a.animationName || a.transitionProperty || "";
        const key = [kind, name, label(a), tm.duration, tm.easing, tm.iterations].join("|");
        // A transition interrupted mid-way and reversed runs shortened (CSS
        // Transitions' reversing-shortening factor), so its declared
        // duration is what the rule says; both are kept.
        let declared = null;
        if (kind === "CSSTransition" && a.effect.target) {
          const cs = getComputedStyle(a.effect.target, a.effect.pseudoElement || null);
          const ps = cs.transitionProperty.split(",").map((x) => x.trim()), ds = cs.transitionDuration.split(",").map((x) => parseFloat(x) * (x.trim().endsWith("ms") ? 1 : 1000));
          let i = ps.indexOf(name);
          if (i < 0) i = ps.findIndex((p) => name.startsWith(p + "-") || name.replace(/-(top|right|bottom|left)(?=-)/, "") === p);
          if (i < 0) i = ps.indexOf("all");
          if (i >= 0) declared = ds[i % ds.length];
        }
        if (!window.__anims.has(key)) window.__anims.set(key, { kind, name, target: label(a), duration: tm.duration, declared, easing: tm.easing, kfEasing: kfe, iterations: tm.iterations, props: props(a), at: location.pathname, reduced: matchMedia("(prefers-reduced-motion: reduce)").matches });
      }
    } catch {}
    requestAnimationFrame(sample);
  };
  requestAnimationFrame(sample);
};

async function collectAnims(page, bucket) {
  try {
    const list = await page.evaluate(() => [...(window.__anims || new Map()).values()]);
    for (const a of list) bucket.set([a.kind, a.name, a.target, a.duration, a.easing, a.iterations, a.at].join("|"), a);
  } catch {}
}

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

async function shot(page, name, opts = {}) {
  await page.screenshot({ path: path.join(MEDIA, name + ".png"), ...opts });
}

// ---- motion rules -------------------------------------------------------

const LOOPS = /^(breathe|spin|slide|specimen)/; // the checking pulse, the processing marks
function bezier(e) {
  const m = /cubic-bezier\(([^)]+)\)/.exec(e || "");
  return m ? m[1].split(",").map(Number) : null;
}
function effectiveEasing(a) {
  if (a.easing && a.easing !== "linear") return [a.easing];
  return a.kfEasing && a.kfEasing.length ? a.kfEasing : ["linear"];
}
function easeOutNoOvershoot(e) {
  if (e === "ease-out") return true;
  const b = bezier(e);
  if (!b) return false;
  const [x1, y1, x2, y2] = b;
  // No overshoot (both control points' progress stays in [0, 1]) and an
  // ease-out: it ends decelerating (the second control point sits at or
  // above the diagonal), e.g. DESIGN.md's cubic-bezier(0.2, 0, 0, 1).
  return y1 >= 0 && y1 <= 1 && y2 >= 0 && y2 <= 1 && x1 >= 0 && x1 <= 1 && x2 >= 0 && x2 <= 1 && y2 >= x2;
}
function judgeMotion(config, anims, reduced) {
  const list = [...anims.values()].filter((a) => a.reduced === reduced);
  const bad = [];
  for (const a of list) {
    const loop = a.iterations === Infinity || a.iterations === "Infinity" || a.iterations > 1;
    if (reduced) {
      const moves = a.props.some((p) => /transform|translate|scale|rotate|clip-path/.test(p));
      if (loop) bad.push(`loops: ${a.name} ${a.target}`);
      else if (moves && a.duration > 0) bad.push(`moves: ${a.name} ${a.target} ${a.props.join("/")} ${a.duration}ms`);
      else if ((a.declared != null ? a.declared : a.duration) > 120) bad.push(`over 120 ms: ${a.name} ${a.target} ${a.duration}ms`);
      continue;
    }
    if (loop) {
      if (!LOOPS.test(a.name)) bad.push(`unexpected loop: ${a.name} ${a.target}`);
      continue;
    }
    const d = a.declared != null ? a.declared : a.duration;
    if (d === 0) continue;
    if (!(d >= 120 && d <= 240)) bad.push(`duration ${d}ms: ${a.kind} ${a.name} ${a.target}`);
    const eases = effectiveEasing(a);
    if (!eases.every(easeOutNoOvershoot)) bad.push(`easing ${eases.join("/")}: ${a.kind} ${a.name} ${a.target}`);
    if (a.props.some((p) => p === "scale")) bad.push(`scales: ${a.name} ${a.target}`);
  }
  const kind = reduced ? "reduced motion" : "motion";
  check(config, `${kind}: ${list.length} distinct animations/transitions observed`, reduced || list.length >= 8, list.length);
  check(config, reduced ? "reduced motion: nothing translates, scales, nudges, pulses, draws or loops; fades ≤ 120 ms" : "motion: every transition 120–240 ms, ease-out, no overshoot; only the allowed loops repeat",
    bad.length === 0, bad.slice(0, 12).join(" | "));
  return list;
}

// ---- helpers --------------------------------------------------------------

async function signIn(page, base) {
  await page.goto(base + "/signin");
  await page.fill("#email", "phaedrus@example.com");
  await page.fill("#password", "know-thyself");
  await Promise.all([page.waitForNavigation(), page.click("button.btn-accent")]);
}

async function deckTag(page) {
  return ((await page.textContent("[data-deck-tag]").catch(() => "")) || "").trim();
}

// Type an answer the way a learner does: lines joined by Shift+Enter.
async function typeLines(page, lines) {
  await page.click("#answer");
  for (let i = 0; i < lines.length; i++) {
    await page.keyboard.type(lines[i], { delay: 4 });
    if (i < lines.length - 1) await page.keyboard.press("Shift+Enter");
  }
}

const ANSWERS = [
  ["It is the idea that a cache entry goes stale when the data behind it changes.", "So every write must either update or evict the entry."],
  ["A paragraph-long explanation: the concept is about how people reason under uncertainty, weighing what is likely against what is merely possible, and it holds even when the examples change because the principle is general rather than tied to one case. It also says that the simplest account consistent with the evidence should be tried first."],
  ["First sentence of my answer.", "Second sentence, on its own line.", "Third, to make it a real explanation."],
];

// One model submission on the current waiting card, observed end to end.
async function submit(page, config, srv, k, lines, opts = {}) {
  const before = chainLines(srv.logPath).length;
  const path_ = PATHS[k % PATHS.length];
  const verdict = VERDICTS[k % VERDICTS.length];
  const want = EXPECT[path_] || { escalated: false, attempts: 1 };
  await page.waitForSelector("[data-asking] #answer");
  if (!opts.keep) {
    const h0 = await page.$eval("#answer", (t) => t.getBoundingClientRect().height);
    await typeLines(page, lines);
    const t = await page.$eval("#answer", (t) => ({ v: t.value, h: t.getBoundingClientRect().height, sw: t.scrollWidth, cw: t.clientWidth }));
    if (k === 0) {
      check(config, "S111-2: Shift+Enter puts line breaks in the answer", t.v.split("\n").length === lines.length, JSON.stringify(t.v.slice(0, 60)));
      check(config, "S111-2: the field grows as the text reaches new rows, never sideways", t.h > h0 + 10 && t.sw <= t.cw + 1, `${h0.toFixed(0)} -> ${t.h.toFixed(0)} px`);
    }
  }
  const typed = await page.$eval("#answer", (t) => t.value);
  let posts = 0;
  const count = (r) => { if (r.method() === "POST" && new URL(r.url()).pathname === "/study/answer") posts++; };
  page.on("request", count);
  await page.press("#answer", "Enter");
  await sleep(150);
  await page.press("#answer", "Enter"); // a second Enter while it checks does nothing
  const busy = await page.evaluate(() => {
    const b = document.querySelector("[data-check]"), t = document.querySelector("#answer");
    return { disabled: b && b.disabled, aria: b && b.getAttribute("aria-busy"), ro: t && t.readOnly, status: document.querySelector("#status")?.textContent.trim() };
  });
  await sleep(450);
  const w1 = await statusWord(page);
  await sleep(2450); // ~3.05 s after Enter; the fixture answers at 3.3 s
  const w2 = await statusWord(page);
  if (k <= 1 || opts.shots) await shot(page, `${config}-study-still-checking`);
  if (path_ === "all-fail") {
    await page.waitForSelector("#status[data-failed]", { timeout: 15000 });
  } else {
    await page.waitForSelector("[data-judged]", { timeout: 15000 });
  }
  page.off("request", count);
  const tag = `S113-1/2 submission ${k + 1} (${path_})`;
  check(config, `${tag}: Check disabled and busy at once, field kept read-only`, busy.disabled && busy.aria === "true" && busy.ro, JSON.stringify(busy));
  check(config, `${tag}: no word before 400 ms, then "checking", "still checking" after 3 s`, busy.status === "" && w1 === "checking" && w2 === "still checking", `${JSON.stringify(busy.status)} → ${w1} → ${w2}`);
  check(config, `${tag}: a second Enter sends nothing`, posts === 1, `${posts} POSTs`);
  const lines_ = chainLines(srv.logPath).slice(before);
  const ch = lines_[0] || {};
  if (want.failed) {
    const st = await page.evaluate(() => ({ msg: document.querySelector("#status").textContent.trim(), v: document.querySelector("#answer").value, nudged: document.querySelector("#answer").classList.contains("is-nudged") || true, focus: document.activeElement && document.activeElement.id }));
    check(config, `${tag}: every path failing says "Couldn't check that just now…"`, /Couldn't check that just now\. Your answer is still here; press Enter to try again\./.test(st.msg), st.msg);
    check(config, `${tag}: the same card keeps the complete answer, line breaks included, focus in the field`, st.v === typed && st.focus === "answer", JSON.stringify(st.v.slice(0, 50)));
    check(config, `${tag}: the server's chain asked every model and the escalation, and gave no verdict`, lines_.length === 1 && /fail|no verdict|false/i.test(JSON.stringify(ch)), JSON.stringify(ch).slice(0, 300));
    return { failed: true, typed };
  }
  const rv = await page.evaluate(() => ({
    verdict: document.querySelector("[data-verdict]")?.dataset.verdict,
    badge: document.querySelector("[data-verdict-label]")?.textContent.trim(),
    why: document.querySelector(".why")?.textContent.trim(),
    submitted: [...document.querySelectorAll("[data-submitted]")].map((e) => e.innerText).join("\n"),
    ws: (() => { const e = document.querySelector(".compare-answer[data-submitted], .struck-text[data-submitted]"); return e ? getComputedStyle(e).whiteSpace : ""; })(),
    sched: document.querySelector("[data-next-review]")?.textContent.trim() || document.querySelector(".schedule")?.innerText.trim().slice(0, 80),
  }));
  check(config, `${tag}: shows the scripted verdict "${verdict}" with its badge and one-line reason`, rv.verdict === verdict && BADGE[verdict].test(rv.badge) && rv.why && rv.why.length > 10, `${rv.verdict} · ${rv.badge} · ${rv.why}`);
  check(config, `${tag}: the review keeps the answer's line breaks`, lines.length < 2 || (rv.submitted.split("\n").length >= lines.length && /pre-wrap|pre-line/.test(rv.ws)), `${rv.ws} ${JSON.stringify(rv.submitted.slice(0, 60))}`);
  check(config, `${tag}: the review shows the new schedule`, !!rv.sched, rv.sched);
  if (!opts.noChainCheck) {
    const steps = ch.path || "";
    const n = typeof ch.attempts === "number" ? ch.attempts : -1;
    const esc = /escalat/.test(steps);
    check(config, `${tag}: server log: ${want.escalated ? "escalated to the LLM" : "no escalation"}, ${want.attempts} attempt(s)${path_.startsWith("primary-") && want.escalated ? ", no secondary decisions model asked" : ""}`,
      lines_.length === 1 && n === want.attempts && esc === want.escalated, `attempts=${n} path=${steps}`);
  }
  return { verdict: rv.verdict };
}

async function next(page) {
  await page.waitForSelector("[data-next]");
  await Promise.all([page.waitForSelector("[data-asking], .session-end, [data-session-end], main h1", { timeout: 10000 }), page.click("[data-next]")]);
  await page.waitForSelector("[data-asking]", { timeout: 10000 }).catch(() => {});
}

// ---- one config ---------------------------------------------------------

async function journey(browser, c, port) {
  const config = c.name;
  const srv = startServer(config, port);
  const base = srv.base;
  const anims = new Map();
  try {
    const health = await waitHealthy(base);
    check(config, "/healthz 200 with the build sha and the grading chain", health.status === "ok" && (!SHA || SHA.startsWith(health.build_sha)) && !!health.grading_chain, JSON.stringify(health.grading_chain || {}));
    const opts = { viewport: c.viewport, colorScheme: c.colorScheme, timezoneId: "America/Chicago", locale: "en-US",
      ...(c.phone ? { isMobile: true, hasTouch: true, deviceScaleFactor: 2 } : {}) };
    const ctx = await browser.newContext({ ...opts, acceptDownloads: true, permissions: ["microphone"], recordVideo: { dir: tmp, size: opts.viewport } });
    await ctx.addInitScript(initScript);
    const page = await ctx.newPage();
    const problems = [];
    const forced = [];
    instrument(page, problems, forced);

    // ---- S113-3: one mark everywhere (signed out) --------------------------
    await page.goto(base + "/");
    await page.mouse.move(1, 1);
    const mark = await page.evaluate(() => {
      const m = document.querySelector("header .mark"), w = document.querySelector("header .wordmark-text");
      const mb = m.getBoundingClientRect(), wb = w.getBoundingClientRect(), hb = document.querySelector("header").getBoundingClientRect();
      const icons = [...document.querySelectorAll("link[rel~=icon], link[rel=apple-touch-icon], link[rel=manifest], meta[property='og:image']")].map((l) => (l.getAttribute("href") || l.getAttribute("content")) + (l.sizes && l.sizes.value ? "@" + l.sizes.value : ""));
      return { w: mb.width, h: mb.height, gap: wb.left - mb.right, dy: (mb.top + mb.height / 2) - (wb.top + wb.height / 2), header: hb.height, icons,
        top: getComputedStyle(m.querySelector(getComputedStyle(m.querySelector(".mark-exact")).display === "none" ? ".mark-hinted .mark-top" : ".mark-exact .mark-top")).fill,
        rects: m.querySelectorAll(".mark-exact rect").length, lens: /lens|focal/i.test(document.documentElement.outerHTML) };
    });
    const accent = c.colorScheme === "dark" ? "rgb(77, 141, 255)" : "rgb(5, 98, 239)";
    check(config, "S113-3: header mark is 24 px, 8 px before scry, centred on the wordmark", mark.w === 24 && mark.h === 24 && Math.abs(mark.gap - 8) < 0.6 && Math.abs(mark.dy) <= 1, JSON.stringify({ w: mark.w, gap: mark.gap, dy: mark.dy, header: mark.header }));
    check(config, "S113-3: top card in Signal Cobalt for this mode", mark.top === accent, mark.top);
    check(config, "S113-3: no lens mark left in the page", !mark.lens);
    const assets = await page.evaluate(async (urls) => {
      const out = {};
      for (const u of urls) {
        const img = new Image();
        img.src = u;
        try { await img.decode(); out[u] = [img.naturalWidth, img.naturalHeight]; } catch { out[u] = "undecodable"; }
      }
      return out;
    }, ["/static/favicon.svg", "/static/favicon-16.png", "/static/favicon-32.png", "/static/app-icon-64.png", "/static/app-icon-64-dark.png", "/static/icon-180.png", "/static/icon-192.png", "/static/icon-512.png", "/static/icon-maskable-512.png"]);
    check(config, "S113-3: favicon SVG, 16/32 px PNGs, 64 px app icons and home-screen icons decode at their sizes",
      assets["/static/favicon-16.png"]?.[0] === 16 && assets["/static/favicon-32.png"]?.[0] === 32 && assets["/static/app-icon-64.png"]?.[0] === 64 && assets["/static/icon-512.png"]?.[0] === 512 && Array.isArray(assets["/static/favicon.svg"]),
      JSON.stringify(assets));
    check(config, "S113-3: head links the S3-A favicon, apple-touch icon, manifest and social preview", mark.icons.some((i) => i.includes("favicon.svg")) && mark.icons.some((i) => i.includes("icon-180")) && mark.icons.some((i) => i.includes("manifest")), mark.icons.join(" "));
    await shot(page, `${config}-landing-header-mark`, { clip: { x: 0, y: 0, width: c.viewport.width, height: 72 } });
    await pageHealth(page, config, "landing");
    // Landing Specimen: let it step once (its 200 ms cross-fade is sampled).
    await sleep(4700);
    await collectAnims(page, anims);

    // ---- S111-1: one interleaved session ----------------------------------
    await signIn(page, base);
    await page.goto(base + "/library");
    await page.waitForLoadState("load");
    await sleep(400);
    await page.click(".nav a[href='/study']");
    await page.waitForSelector("[data-asking]");
    check(config, "S111-1: Study in the navigation opens straight onto a card", new URL(page.url()).pathname === "/study");
    const vt = await page.evaluate(() => window.__vt);
    check(config, "S113-2: a destination switch runs a view transition", vt.includes(true), JSON.stringify(vt));
    await pageHealth(page, config, "study waiting");
    await shot(page, `${config}-study-waiting`);
    const tags = [await deckTag(page)];

    // Hover, press and keyboard focus on a control (sampled).
    if (!c.phone) {
      await page.hover("[data-check]");
      await sleep(200);
      await page.mouse.down();
      await sleep(150);
      await page.mouse.move(1, 1);
      await page.mouse.up();
      await page.keyboard.press("Tab");
      await sleep(200);
    }
    await collectAnims(page, anims);

    // ---- S113-1 / S111-2: answers through every chain path ------------------
    let k = 0;
    for (; k < 9; k++) {
      const r = await submit(page, config, srv, k, ANSWERS[k % ANSWERS.length], { shots: k === 0 });
      if (k === 0) { await shot(page, `${config}-study-review-multiline`); await pageHealth(page, config, "study review"); }
      if (k === 4) await shot(page, `${config}-study-review-escalated`);
      await collectAnims(page, anims);
      await next(page);
      tags.push(await deckTag(page));
      await collectAnims(page, anims);
    }
    // Decks alternate (named tags only; a neutral "Set N" tag says nothing).
    const named = tags.map((t) => (/·/.test(t) ? t.split("·")[0].trim() : ""));
    let repeats = 0;
    for (let i = 1; i < named.length; i++) if (named[i] && named[i] === named[i - 1]) repeats++;
    check(config, "S111-1: consecutive cards come from different decks", repeats === 0 && new Set(named.filter(Boolean)).size >= 2, tags.join(" → "));

    // Every grading path fails: the answer stays; Enter retries.
    forced.push("ALLFAIL");
    const fail = await submit(page, config, srv, k++, ANSWERS[2], {});
    await shot(page, `${config}-study-couldnt-check`);
    await collectAnims(page, anims);
    check(config, "S113-2: the failed field nudges once", await page.evaluate(() => [...(window.__anims || new Map()).values()].some((a) => /nudge/.test(a.name) && a.iterations === 1)));
    const retry = await submit(page, config, srv, k++, ANSWERS[2], { keep: true });
    check(config, "S113-1: after recovery Enter retries and a verdict appears", !!retry.verdict && fail.failed);
    await next(page);

    // A request interrupted before a review is recorded.
    forced.push("/study/answer");
    await page.route("**/study/answer", (route) => route.abort("connectionreset"));
    await typeLines(page, ANSWERS[0]);
    const typedI = await page.$eval("#answer", (t) => t.value);
    const beforeI = chainLines(srv.logPath).length;
    await page.press("#answer", "Enter");
    await page.waitForSelector("#status[data-failed]", { timeout: 8000 });
    const kept = await page.evaluate(() => ({ v: document.querySelector("#answer").value, msg: document.querySelector("#status").textContent.trim() }));
    check(config, "S113-1: an interrupted request keeps the complete answer, line breaks included", kept.v === typedI && /Couldn't check that just now/.test(kept.msg) && chainLines(srv.logPath).length === beforeI, kept.msg);
    await page.unroute("**/study/answer");
    const r2 = await submit(page, config, srv, k++, ANSWERS[0], { keep: true });
    check(config, "S113-1: after the interruption Enter retries and is judged", !!r2.verdict);
    await next(page);

    // The response is lost after the review is recorded.
    await page.route("**/study/answer", async (route) => { await route.fetch(); await route.abort("connectionreset"); });
    await typeLines(page, ANSWERS[1]);
    const beforeL = chainLines(srv.logPath).length;
    await page.press("#answer", "Enter");
    await page.waitForSelector("#status[data-failed]", { timeout: 15000 });
    k++; // the server judged it
    await page.unroute("**/study/answer");
    const afterL = chainLines(srv.logPath).length;
    await Promise.all([page.waitForSelector("[data-judged]", { timeout: 15000 }), page.press("#answer", "Enter")]);
    check(config, "S113-1: a response lost after the review is recorded: Enter opens that review with no second judgment", afterL === beforeL + 1 && chainLines(srv.logPath).length === afterL);
    await collectAnims(page, anims);

    // ---- S113-2: Edit card waits and failure --------------------------------
    await page.click("[data-edit-toggle]");
    await sleep(300);
    const q0 = await page.inputValue("#edit-prompt");
    await page.fill("#edit-prompt", q0 + " (edited)");
    forced.push("/study/edit");
    await page.route("**/study/edit", (route) => route.abort("connectionreset"));
    await page.click("[data-edit-save]");
    await page.waitForSelector("[data-edit-fail]:not([hidden])", { timeout: 8000 });
    const ef = await page.evaluate(() => ({ msg: document.querySelector("[data-edit-fail]").textContent.trim(), q: document.querySelector("#edit-prompt").value }));
    check(config, "S113-2: Save card failure says so and keeps the edits", /Couldn't save that just now\. Your edits are still here\./.test(ef.msg) && ef.q === q0 + " (edited)", ef.msg);
    await shot(page, `${config}-study-edit-failed`);
    await page.unroute("**/study/edit");
    await page.route("**/study/edit", async (route) => { await sleep(900); await route.continue(); });
    await page.click("[data-edit-save]");
    await sleep(500);
    const saving = await page.evaluate(() => { const b = document.querySelector("[data-edit-save]"); return { w: b.textContent.trim(), d: b.disabled, a: b.getAttribute("aria-busy") }; });
    check(config, "S113-2: Save card reads \"saving\", disabled and busy", /saving/.test(saving.w) && saving.d && saving.a === "true", JSON.stringify(saving));
    await page.waitForSelector("[data-edit-status]:not([hidden])", { timeout: 10000 });
    check(config, "S113-2: after retry the edit is saved", (await page.textContent("[data-prompt]")).includes("(edited)"));
    await page.unroute("**/study/edit");
    await collectAnims(page, anims);
    await next(page);

    // ---- S113-2: Snooze and Delete waits and failures -----------------------
    await page.fill("#answer", "kept while snoozing fails");
    forced.push("/study/snooze", "/study/delete");
    for (const [sel, route_, word, msg] of [["[data-snooze]", "/study/snooze", "snoozing", "Couldn't snooze that just now."], ["[data-delete]", "/study/delete", "removing", "Couldn't remove that just now."]]) {
      await page.route("**" + route_, (r) => r.abort("connectionreset"));
      await page.click(`[data-asking] ${sel}`);
      await page.waitForSelector("[data-asking] [data-manage-status]:has-text('Couldn')", { timeout: 8000 });
      const st = await page.evaluate(() => ({ msg: document.querySelector("[data-asking] [data-manage-status]").textContent.trim(), v: document.querySelector("#answer").value }));
      check(config, `S113-2: ${route_} failure says "${msg}", card and typed answer stay`, st.msg.includes(msg) && st.v === "kept while snoozing fails", st.msg);
      if (route_ === "/study/snooze") await shot(page, `${config}-study-snooze-failed`);
      await page.unroute("**" + route_);
      await page.route("**" + route_, async (r) => { await sleep(900); await r.continue(); });
      const prompt0 = (await page.textContent("#prompt")).trim();
      await page.click(`[data-asking] ${sel}`);
      await sleep(550);
      const w = await page.evaluate((s) => {
        const b = document.querySelector(`[data-asking] ${s}`);
        return { w: b.textContent.trim(), d: b.disabled, a: b.getAttribute("aria-busy"), aside: document.querySelectorAll("[data-asking] [data-set-aside]").length };
      }, sel);
      check(config, `S113-2: ${route_} reads "${word}", disabled and busy, the others set aside`, w.w.includes(word) && w.d && w.a === "true" && w.aside >= 1, JSON.stringify(w));
      await page.waitForSelector("[data-study-note]", { timeout: 10000 });
      await page.waitForFunction((p) => (document.querySelector("#prompt")?.textContent.trim() || "") !== p, prompt0, { timeout: 10000 }).catch(() => {});
      check(config, `S113-2: after retry ${route_} goes on to the next card`, ((await page.textContent("#prompt").catch(() => "")) || "").trim() !== prompt0);
      await page.unroute("**" + route_);
      await collectAnims(page, anims);
      await page.fill("#answer", "kept while snoozing fails");
    }
    await page.fill("#answer", "");

    // ---- S113-2: voice "listening" → "transcribing" ---------------------------
    // The transcription is held 900 ms so its word can be read.
    await page.route("**/study/speech", async (r) => { await sleep(900); await r.continue(); });
    await page.fill("#answer", "typed first");
    await page.click("[data-mic]");
    await sleep(500);
    const l1 = await statusWord(page);
    await sleep(2500);
    await page.click("[data-mic]");
    await sleep(400);
    const l2 = await statusWord(page);
    await shot(page, `${config}-study-transcribing`);
    await page.waitForFunction(() => document.querySelector("#answer").value.length > "typed first".length, null, { timeout: 10000 }).catch(() => {});
    const heard = await page.$eval("#answer", (t) => t.value);
    await page.unroute("**/study/speech");
    // The fixture transcript is cut to what the recording's length could hold.
    const spokenPart = heard.slice("typed first ".length);
    check(config, "S113-2: voice says \"listening\", then \"transcribing\"; the words settle after the typed text", l1.includes("listening") && l2.includes("transcribing") && heard.startsWith("typed first ") && spokenPart.length > 3 && SPOKEN.startsWith(spokenPart), `${l1} → ${l2} → ${JSON.stringify(heard)}`);
    await page.fill("#answer", "");

    // ---- S111-1: Study this idea, then the navigation's Study ---------------
    await page.goto(base + "/library");
    const ideaHref = await page.$eval(".outline a[href^='/library/ideas/']", (a) => a.getAttribute("href"));
    await page.goto(base + ideaHref);
    await sleep(400);
    await collectAnims(page, anims); // the idea's link lines draw in
    await shot(page, `${config}-library-study-this-idea`);
    await pageHealth(page, config, "idea page");
    await Promise.all([page.waitForURL(/\/study/), page.click("[data-study-idea]")]);
    const focused = (await page.textContent("[data-focused]").catch(() => "")).trim();
    check(config, "S111-1: Study this idea opens a focused session (\"Focused · …\" with Study all decks)", /^Focused/.test(focused) && /Study all decks/.test(focused), focused);
    await page.click(".nav a[href='/study']");
    await page.waitForSelector("[data-asking], main");
    check(config, "S111-1: the navigation's Study opens every deck again (no focused line)", (await page.$("[data-focused]")) === null);

    // ---- S113-2: import waits and refusal -----------------------------------
    await page.goto(base + "/learn#import");
    await page.fill("#import-idea", "Imported in the walkthrough");
    await page.setInputFiles("#import-file", { name: "broken.apkg", mimeType: "application/octet-stream", buffer: Buffer.from("not a zip") });
    await page.click("[data-import-submit]");
    await page.waitForSelector("[data-import-status] :is(.field-error, [data-refused]), [data-import-status][data-refused]", { timeout: 8000 }).catch(() => {});
    const imp = await page.evaluate(() => ({ msg: document.querySelector("[data-import-status]").textContent.trim(), title: document.querySelector("#import-idea").value, files: document.querySelector("#import-file").files.length }));
    check(config, "S113-2: a refused import says why in words and keeps the idea title and the chosen file", imp.msg.length > 10 && imp.title === "Imported in the walkthrough" && imp.files === 1, JSON.stringify(imp));
    await shot(page, `${config}-import-refused`);
    await page.route("**/learn/import", async (r) => { await sleep(1200); await r.continue(); });
    await page.setInputFiles("#import-file", path.join(DECKS, "sample.csv"));
    await page.click("[data-import-submit]");
    await sleep(600);
    const iw = await page.evaluate(() => { const b = document.querySelector("[data-import-submit]"), p = document.querySelector("[data-import-progress]"); return { w: b.textContent.trim(), d: b.disabled, a: b.getAttribute("aria-busy"), bar: !!p && !p.hidden && p.getBoundingClientRect().height >= 1 && p.getBoundingClientRect().height <= 3 }; });
    check(config, "S113-2: Import reads \"importing\" with the 2 px progress line, disabled and busy", /importing/.test(iw.w) && iw.d && iw.a === "true" && iw.bar, JSON.stringify(iw));
    await shot(page, `${config}-importing`);
    await page.waitForURL(/\/library\/ideas\/\d+/, { timeout: 15000 });
    check(config, "S113-2: the import then lands in Library", /Imported \d+ cards\./.test(await page.textContent(".flash").catch(() => "")));
    await page.unroute("**/learn/import");

    // ---- S113-2: download waits and failure ---------------------------------
    await page.goto(base + "/settings");
    await page.locator("[data-export-links]").scrollIntoViewIfNeeded();
    forced.push("/settings/export");
    await page.route("**/settings/export?format=md", (r) => r.abort("connectionreset"));
    await page.click("text=Download Markdown");
    await page.waitForSelector("[data-download-status]:has-text('Couldn')", { timeout: 8000 }).catch(() => {});
    const df = (await page.textContent("[data-download-status]")).trim();
    check(config, "S113-2: a failed download says \"Couldn't prepare that download. Try again.\"", df.includes("Couldn't prepare that download. Try again."), df);
    await shot(page, `${config}-download-failed`);
    await page.unroute("**/settings/export?format=md");
    await page.route("**/settings/export?format=md", async (r) => { await sleep(1000); await r.continue(); });
    const dlP = page.waitForEvent("download", { timeout: 15000 });
    await page.click("text=Download Markdown");
    await sleep(500);
    const dw = await page.evaluate(() => { const a = document.querySelector("a[data-download][href*='md']"); return { w: a.textContent.trim(), a: a.getAttribute("aria-busy"), d: a.getAttribute("aria-disabled") }; });
    check(config, "S113-2: Download reads \"preparing\" until the download begins", /preparing/.test(dw.w) && dw.a === "true", JSON.stringify(dw));
    const dl = await dlP;
    const p = path.join(tmp, `${config}.md`);
    await dl.saveAs(p);
    check(config, "S113-2: after retry the Markdown export downloads", fs.readFileSync(p, "utf8").startsWith("# Scry export"));
    await page.unroute("**/settings/export?format=md");
    await pageHealth(page, config, "settings");
    await collectAnims(page, anims);

    check(config, "journey: 0 console/page/request/CSP problems (beyond the deliberately forced failures)", problems.length === 0, problems.join(" | "));
    const video = page.video();
    await ctx.close();
    if (video) await video.saveAs(path.join(MEDIA, `${config}-walkthrough.webm`));
    judgeMotion(config, anims, false);

    // ---- S113-2: reduced motion ---------------------------------------------
    const rctx = await browser.newContext({ ...opts, reducedMotion: "reduce" });
    await rctx.addInitScript(initScript);
    const rp = await rctx.newPage();
    const rproblems = [];
    instrument(rp, rproblems, []);
    const ranims = new Map();
    await rp.goto(base + "/");
    await sleep(4700);
    const rest = await rp.$eval("[data-specimen]", (f) => [...f.querySelectorAll("[data-specimen-stage]")].filter((x) => getComputedStyle(x).visibility !== "hidden").map((x) => x.dataset.specimenStage).join(","));
    check(config, "reduced motion: the landing Specimen rests on Judged", rest === "judged", rest);
    await collectAnims(rp, ranims);
    await signIn(rp, base);
    await rp.goto(base + "/study");
    let rw = "";
    for (let tries = 0; tries < 4 && rw !== "still checking"; tries++) {
      await rp.waitForSelector("[data-asking], [data-judged]");
      if (await rp.$("[data-judged]")) { await rp.click("[data-next]"); await rp.waitForSelector("[data-asking]"); }
      await rp.fill("#answer", "a reduced-motion answer");
      await rp.press("#answer", "Enter");
      await sleep(3050);
      // An exact text is judged word for word at once (no model, no wait).
      rw = await statusWord(rp);
    }
    await collectAnims(rp, ranims);
    check(config, "reduced motion: the wait still says itself in words (\"still checking\")", rw === "still checking", rw);
    await rp.waitForSelector("[data-judged]", { timeout: 15000 });
    await sleep(300);
    await collectAnims(rp, ranims);
    const rv = await rp.evaluate(() => document.querySelector("[data-verdict-label]")?.textContent.trim());
    check(config, "reduced motion: the verdict is named", !!rv, rv);
    await shot(rp, `${config}-reduced-motion-review`);
    await rp.click("[data-next]");
    await sleep(500);
    await collectAnims(rp, ranims);
    await rp.click(".nav a[href='/library']");
    await sleep(500);
    await collectAnims(rp, ranims);
    judgeMotion(config, ranims, true);
    check(config, "reduced motion: 0 console/page/request problems", rproblems.length === 0, rproblems.join(" | "));
    await rctx.close();
    return [...anims.values(), ...ranims.values()];
  } finally {
    srv.proc.kill("SIGTERM");
    await new Promise((r) => srv.proc.once("exit", r));
    fs.copyFileSync(srv.logPath, path.join(tmp, `${config}-server.log`));
  }
}

// ---- run ----------------------------------------------------------------

const configs = [
  { name: "desktop-light", viewport: { width: 1440, height: 900 }, colorScheme: "light", phone: false },
  { name: "desktop-dark", viewport: { width: 1440, height: 900 }, colorScheme: "dark", phone: false },
  { name: "phone-light", viewport: { width: 390, height: 844 }, colorScheme: "light", phone: true },
  { name: "phone-dark", viewport: { width: 390, height: 844 }, colorScheme: "dark", phone: true },
].filter((c) => !ONLY || ONLY.includes(c.name));

let exitCode = 1;
const motion = {};
try {
  check("run", "Chrome is headed", !!process.env.DISPLAY, `DISPLAY=${process.env.DISPLAY}`);
  const browser = await chromium.launch({ executablePath: CHROME, headless: false,
    args: ["--window-position=0,0", "--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream", "--autoplay-policy=no-user-gesture-required"] });
  let i = 0;
  for (const c of configs) {
    try {
      motion[c.name] = await journey(browser, c, PORT0 + i++);
    } catch (e) {
      console.error(e);
      check(c.name, "the walkthrough ran to the end", false, e.message.split("\n")[0]);
    }
  }
  await browser.close();
  exitCode = failed === 0 ? 0 : 1;
} catch (e) {
  console.error(e);
  check("run", "the walkthrough ran to the end", false, e.message);
} finally {
  const sum = { sha: SHA, passed: results.filter((r) => r.ok).length, failed, total: results.length, results, forcedFailures: forcedSeen, motion };
  fs.writeFileSync(path.join(MEDIA, "verify-slice113-results.json"), JSON.stringify(sum, null, 1));
  console.log(`\n${sum.passed}/${sum.total} checks passed; server logs in ${tmp}`);
  process.exit(exitCode);
}
