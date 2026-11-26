#!/usr/bin/env node
// verify-slice135.mjs: check Slice 135 ([S135-1], with the Specimen's
// subject header of [S131-1]) on the real running path, in a real headed
// Chrome: the landing page is clean on a phone.
//
//   make build BUILD_SHA=$(git rev-parse HEAD)
//   DISPLAY=:0 node scripts/verify-slice135.mjs --bin ./bin/scry --sha <sha> \
//        --media ~/chief-evidence/media [--port 18135] [--only phone-light,...]
//
// For each of desktop 1440x900 (DPR 1) and phone 390x844 (touch, DPR 2),
// light and dark, it starts its own Scry (the --bin binary) in SCRY_MODE=test
// with the demo learner seeded on a fresh SQLite file in a new temporary
// directory and a minimal environment (no provider keys; nothing leaves the
// machine), records a video and walks a newcomer's path:
//
//   landing (signed out) -> the Specimen steps on its own -> tap/click each
//   step -> "Start the 7-day trial" opens sign-up -> back -> header
//   "Sign in" -> typed demo sign-in -> Study opens on a card.
//
// Phone: the drafting band runs edge to edge with only top and bottom
// rules and no ruler ticks; headline, action, terms and the Specimen's
// border sit 16 px from both screen edges, aligned with the header; one
// hero action, full width and at least 48 px tall, the terms 16 px under
// it on one line; the Specimen is 358 px wide with 16 px inside (4 px above
// the steps); every stage keeps one Specimen height, its subject header
// does not move, and no stage leaves a blank run over 24 px between two of
// its parts or under its last part. Desktop: one hero action, no Sign in
// beside it, the terms on one line, ticks and side frame kept. A narrow
// phone (320 px) breaks the terms into two phrase-whole lines, and under
// reduced motion the Specimen rests on Judged and never moves. On every
// page: 0 px sideways scroll, styles loaded, images decoded, 0 CSP
// violations; console errors, page errors, failed requests and responses
// >= 400 must be 0. Exit 0 only when every check passes.

import { chromium } from "playwright-core";
import { spawn, execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const args = Object.fromEntries(process.argv.slice(2).reduce((a, v, i, all) => {
  if (v.startsWith("--")) a.push([v.slice(2), all[i + 1] && !all[i + 1].startsWith("--") ? all[i + 1] : "1"]);
  return a;
}, []));
const BIN = path.resolve(args.bin || "bin/scry");
const MEDIA = path.resolve(args.media || path.join(os.homedir(), "chief-evidence", "media"));
const PORT0 = Number(args.port || 18135);
const SHA = args.sha || "";
const ONLY = args.only ? args.only.split(",") : null;
const CHROME = process.env.CHROME_BIN || "/usr/bin/google-chrome";
const TERMS = "7 days free · no card needed · $5/month after";

fs.mkdirSync(MEDIA, { recursive: true });
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "scry-s135-"));
const results = [];
let failed = 0;
function check(config, name, ok, detail = "") {
  const d = typeof detail === "string" ? detail : JSON.stringify(detail);
  results.push({ config, name, ok: !!ok, detail: d });
  if (!ok) failed++;
  console.log(`${ok ? "PASS" : "FAIL"} [${config}] ${name}${d !== "" ? " — " + d : ""}`);
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const near = (a, b, tol = 0.6) => Math.abs(a - b) <= tol;

// ---- the server ---------------------------------------------------------

function startServer(config, port) {
  const dir = fs.mkdtempSync(path.join(tmp, config + "-"));
  const log = fs.createWriteStream(path.join(dir, "server.log"));
  const proc = spawn(BIN, [], {
    env: { PATH: process.env.PATH, HOME: dir, SCRY_MODE: "test", SCRY_SEED_DEMO: "1", SCRY_DB: path.join(dir, "s135.db"), PORT: String(port) },
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

function instrument(page, problems) {
  page.on("console", (m) => { if (m.type() === "error") problems.push("console: " + m.text()); });
  page.on("pageerror", (e) => problems.push("pageerror: " + e.message));
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
      overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
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

// ---- measurements -------------------------------------------------------

// The hero and its copy column, measured in the live page.
const heroMetrics = () => {
  const q = (s, r = document) => r.querySelector(s);
  const box = (e) => { const b = e.getBoundingClientRect(); return { l: b.left, r: b.right, t: b.top, b: b.bottom, w: b.width, h: b.height }; };
  const hero = q(".hero"), cs = getComputedStyle(hero), after = getComputedStyle(hero, "::after"), before = getComputedStyle(hero, "::before");
  const vw = document.documentElement.clientWidth;
  const visible = (e) => { const s = getComputedStyle(e); const b = e.getBoundingClientRect(); return s.display !== "none" && s.visibility !== "hidden" && b.width > 0 && b.height > 0; };
  const heroBtns = [...hero.querySelectorAll("a, button")].filter((e) => !e.closest("[data-specimen]") && visible(e));
  const btn = q(".hero .btn-accent"), terms = q("[data-trial-terms]"), h1 = q(".hero h1"), spec = q("[data-specimen]");
  const label = document.createRange(); label.selectNodeContents(btn);
  const lb = label.getBoundingClientRect(), bb = btn.getBoundingClientRect();
  const termTops = [...new Set([...terms.querySelectorAll(".nowrap")].flatMap((s) => [...s.getClientRects()].map((r) => Math.round(r.top))))];
  const phraseRects = [...terms.querySelectorAll(".nowrap")].map((s) => s.getClientRects().length);
  const signins = [...document.querySelectorAll("a, button")].filter((e) => /^\s*sign in\s*$/i.test(e.textContent) && visible(e));
  const wm = q("header .wordmark");
  const lineH = parseFloat(getComputedStyle(terms).lineHeight);
  const lines = [];
  for (const top of termTops) lines.push([...terms.querySelectorAll(".nowrap")].filter((s) => Math.round(s.getClientRects()[0].top) === top).map((s) => s.textContent).join(""));
  return {
    vw, hero: box(hero), border: [cs.borderTopWidth, cs.borderRightWidth, cs.borderBottomWidth, cs.borderLeftWidth],
    padding: [cs.paddingTop, cs.paddingRight, cs.paddingBottom, cs.paddingLeft], bg: cs.backgroundColor,
    ticks: after.display !== "none" && after.content !== "none" ? after.maskImage || after.webkitMaskImage || "" : "",
    dots: before.maskImage || before.webkitMaskImage || "",
    heroBtns: heroBtns.map((e) => e.textContent.trim()), btn: box(btn), btnBg: getComputedStyle(btn).backgroundColor,
    labelCentre: (lb.left + lb.width / 2) - (bb.left + bb.width / 2),
    terms: box(terms), termsText: terms.textContent.replace(/\s+/g, " ").trim(), termLines: termTops.length, lines, phraseRects, lineH,
    termsColor: getComputedStyle(terms).color, termsFont: getComputedStyle(terms).fontFamily, termsWrap: getComputedStyle(terms).textWrap || getComputedStyle(terms).textWrapStyle || "",
    h1: box(h1), spec: box(spec), lede: box(q(".hero .lede")),
    signins: signins.map((e) => ({ inHeader: !!e.closest("header"), ...box(e) })), wordmark: box(wm),
  };
};

// One Specimen stage: the visible parts, top-level and inside the phone's
// picture of Study, and every blank run between them.
const stageMetrics = () => {
  const fig = document.querySelector("[data-specimen]");
  const shown = [...fig.querySelectorAll("[data-specimen-stage]")].filter((s) => getComputedStyle(s).visibility !== "hidden" && Number(getComputedStyle(s).opacity) > 0.99);
  const s = shown[0];
  const vis = (e) => getComputedStyle(e).display !== "none" && e.getBoundingClientRect().height > 0;
  const parts = [...s.children].filter(vis);
  const runs = [];
  const gaps = (container, kids, name) => {
    const cb = container.getBoundingClientRect();
    let prev = cb.top;
    let first = true;
    for (const k of kids) {
      const b = k.getBoundingClientRect();
      if (!first) runs.push({ where: `${name}: before .${k.className.split(" ")[0]}`, px: Math.round(b.top - prev) });
      first = false;
      prev = b.bottom;
    }
    runs.push({ where: `${name}: under its last part`, px: Math.round(cb.bottom - prev) });
  };
  gaps(s, parts, "stage");
  const study = s.querySelector("[data-specimen-study]");
  if (study && vis(study)) gaps(study, [...study.children].filter(vis), "study picture");
  const fb = fig.getBoundingClientRect(), cs = getComputedStyle(fig);
  const head = fig.querySelector("[data-subject-header]").getBoundingClientRect();
  const field = s.querySelector(".specimen-field"), mic = s.querySelector(".specimen-mic"), chk = s.querySelector(".specimen-check"), next = s.querySelector(".specimen-next");
  const focusable = study ? study.querySelectorAll("a, button, input, textarea, select, [tabindex]").length : 0;
  const steps = fig.querySelector(".specimen-steps").getBoundingClientRect();
  return {
    stage: s.dataset.specimenStage, shown: shown.length, figH: fb.height, figW: fb.width, figL: fb.left, figR: fb.right,
    padding: [cs.paddingTop, cs.paddingRight, cs.paddingBottom, cs.paddingLeft], stepsTop: steps.top - fb.top, stepsH: steps.height,
    contentW: fb.width - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight) - 2,
    headTop: head.top - fb.top, headH: head.height, runs, maxRun: Math.max(...runs.map((r) => r.px)),
    parts: parts.map((p) => p.className.split(" ")[0]),
    field: field && vis(field) ? { h: field.getBoundingClientRect().height, bg: getComputedStyle(field).backgroundColor, radius: getComputedStyle(field).borderRadius, border: getComputedStyle(field).borderTopWidth, font: getComputedStyle(field).fontSize + "/" + getComputedStyle(field).lineHeight, pad: getComputedStyle(field).padding, text: field.textContent.trim(), hidden: field.getAttribute("aria-hidden") } : null,
    mic: mic && vis(mic) ? { w: mic.getBoundingClientRect().width, h: mic.getBoundingClientRect().height, radius: getComputedStyle(mic).borderRadius } : null,
    check: chk && vis(chk) ? { h: chk.getBoundingClientRect().height, bg: getComputedStyle(chk).backgroundColor, border: getComputedStyle(chk).borderTopWidth, text: chk.textContent.trim() } : null,
    next: next && vis(next) ? { text: next.textContent.trim(), lines: Math.round((next.getBoundingClientRect().height - parseFloat(getComputedStyle(next).paddingTop) - parseFloat(getComputedStyle(next).borderTopWidth)) / parseFloat(getComputedStyle(next).lineHeight)), ruleTop: getComputedStyle(next).borderTopWidth } : null,
    controlsHidden: study ? [...study.querySelectorAll(".specimen-controls, .specimen-next")].every((e) => e.getAttribute("aria-hidden") === "true") : null,
    focusable,
    desk: [...s.querySelectorAll(".specimen-desk")].filter(vis).length,
  };
};

const headerMetrics = () => {
  const h = document.querySelector("[data-subject-header]");
  const icon = h.querySelector("svg.icon");
  const name = h.querySelector(".specimen-deck-name"), sec = h.querySelector(".specimen-section"), count = h.querySelector(".specimen-count");
  const ib = icon.getBoundingClientRect(), nb = name.getBoundingClientRect(), cb = count.getBoundingClientRect();
  return {
    text: [...h.querySelectorAll(".specimen-deck-text, .specimen-count > [aria-hidden]")].map((e) => e.textContent).join(" "), icon: icon.getAttribute("class"), iconW: ib.width, iconGap: nb.left - ib.right,
    name: name.textContent, nameFont: getComputedStyle(name).fontSize + " " + getComputedStyle(name).fontWeight, nameColor: getComputedStyle(name).color,
    section: sec.textContent, sectionFont: getComputedStyle(sec).fontFamily, count: count.querySelector('[aria-hidden="true"]').textContent.trim(), countClear: cb.left - nb.right,
    sameLine: Math.abs((cb.top + cb.height / 2) - (nb.top + nb.height / 2)) <= 3, spoken: count.querySelector(".sr")?.textContent,
  };
};

// ---- one config ---------------------------------------------------------

async function journey(c, port) {
  const config = c.name, mode = c.colorScheme;
  const srv = startServer(config, port);
  const base = srv.base;
  const browser = await chromium.launch({ headless: false, executablePath: CHROME, args: [`--window-size=${c.viewport.width},${c.viewport.height + 140}`, "--window-position=0,0"] });
  try {
    const health = await waitHealthy(base);
    check(config, "/healthz 200 with the build sha", health.status === "ok" && (!SHA || SHA.startsWith(health.build_sha)), `status=${health.status} build_sha=${health.build_sha} mode=${health.mode}`);
    const opts = { viewport: c.viewport, colorScheme: mode, timezoneId: "America/Chicago", locale: "en-US",
      ...(c.phone ? { isMobile: true, hasTouch: true, deviceScaleFactor: 2 } : {}) };
    const ctx = await browser.newContext({ ...opts, recordVideo: { dir: tmp, size: c.viewport } });
    await ctx.addInitScript(initScript);
    const page = await ctx.newPage();
    const problems = [];
    instrument(page, problems);
    const press = (sel) => (c.phone ? page.tap(sel) : page.click(sel));

    // ---- landing, signed out --------------------------------------------
    await page.goto(base + "/");
    await page.mouse.move(1, 1).catch(() => {});
    await sleep(500);
    await page.screenshot({ path: path.join(MEDIA, `s135-${config}-landing.png`) });
    await page.locator(".hero").screenshot({ path: path.join(MEDIA, `s135-${config}-hero.png`) });
    await pageHealth(page, config, "landing");
    const m = await page.evaluate(heroMetrics);

    check(config, "hero: exactly one action, \"Start the 7-day trial\"", m.heroBtns.length === 1 && m.heroBtns[0] === "Start the 7-day trial", m.heroBtns);
    check(config, "\"Sign in\" shows once, in the header at the top right, never in the hero",
      m.signins.length === 1 && m.signins[0].inHeader && m.signins[0].r >= m.spec.r - 1 && m.signins[0].r > m.vw / 2, { signins: m.signins, specRight: m.spec.r });
    check(config, `trial terms read exactly "${TERMS}"`, m.termsText === TERMS, m.termsText);
    check(config, "trial terms sit on one line", m.termLines === 1 && m.terms.h <= m.lineH * 1.5, { lines: m.lines, h: m.terms.h });
    check(config, "trial terms: Meta mono, ink-muted, balanced, each phrase whole", /Geist Mono/.test(m.termsFont) && /balance/.test(m.termsWrap) && m.phraseRects.every((n) => n === 1),
      { font: m.termsFont.slice(0, 12), color: m.termsColor, wrap: m.termsWrap, rects: m.phraseRects });
    check(config, "hero keeps its dot grid on the sunken wash", /grid\.svg/.test(m.dots), m.dots.slice(0, 60));

    if (c.phone) {
      check(config, "phone: the band runs to both screen edges", near(m.hero.l, 0) && near(m.hero.r, m.vw), m.hero);
      check(config, "phone: only top and bottom 1 px rules, no side frame lines", m.border[0] === "1px" && m.border[2] === "1px" && m.border[1] === "0px" && m.border[3] === "0px", m.border);
      check(config, "phone: no ruler ticks on the hero", m.ticks === "", m.ticks);
      check(config, "phone: band padded 24 px top, 32 px bottom, 16 px sides", m.padding.join(" ") === "24px 16px 32px 16px", m.padding);
      const edges = { h1: m.h1.l, lede: m.lede.l, btn: [m.btn.l, m.vw - m.btn.r], terms: m.terms.l, spec: [m.spec.l, m.vw - m.spec.r], wordmark: m.wordmark.l };
      check(config, "phone: headline, lede, action, terms and the Specimen's border sit 16 px from the screen edges, aligned with the header",
        [m.h1.l, m.lede.l, m.btn.l, m.vw - m.btn.r, m.terms.l, m.spec.l, m.vw - m.spec.r, m.wordmark.l].every((v) => near(v, 16)), edges);
      check(config, "phone: the action spans the column, at least 48 px tall, its label centred", near(m.btn.w, m.vw - 32) && m.btn.h >= 48 && Math.abs(m.labelCentre) <= 1, { w: m.btn.w, h: m.btn.h, centre: m.labelCentre });
      check(config, "phone: the terms line is 16 px under the action", near(m.terms.t - m.btn.b, 16), m.terms.t - m.btn.b);
    } else {
      check(config, "desktop: hero keeps its 1 px frame on all four sides", m.border.every((b) => b === "1px"), m.border);
      check(config, "desktop: hero keeps its ruler ticks", /tick\.svg/.test(m.ticks) && /tick-major\.svg/.test(m.ticks), m.ticks.slice(0, 60));
      check(config, "desktop: the action keeps its size (not stretched)", m.btn.w < 300 && m.btn.h >= 44, { w: m.btn.w, h: m.btn.h });
    }

    // ---- the subject header ([S131-1]) ----------------------------------
    const hm = await page.evaluate(headerMetrics);
    check(config, "Specimen subject header: layers icon, \"Mental models · Set 1\", \"4 / 11\", no \"Study ·\"",
      /icon-layers/.test(hm.icon) && hm.name === "Mental models" && hm.section === "· Set 1" && hm.count === "4 / 11" && !/Study ·/.test(hm.text), hm);
    check(config, "subject header: 16 px icon 8 px before the name; name in ink, weight 500; section in mono; count on the name's line, 16 px clear",
      near(hm.iconW, 16) && near(hm.iconGap, 8) && /500/.test(hm.nameFont) && /Mono/.test(hm.sectionFont) && hm.sameLine && hm.countClear >= 16 && hm.spoken === "Card 4 of 11", hm);

    // ---- the Specimen steps on its own ----------------------------------
    const order = [];
    const t0 = Date.now();
    let last = "";
    while (Date.now() - t0 < 14000 && order.length < 4) {
      const st = await page.evaluate(() => [...document.querySelectorAll("[data-specimen-stage]")].filter((s) => s.classList.contains("is-shown")).map((s) => s.dataset.specimenStage).join(","));
      if (st !== last) { order.push(st); last = st; }
      await sleep(150);
    }
    check(config, "Specimen steps on its own: Judged -> Prompt -> Answer -> Judged", order.join(">") === "judged>prompt>answer>judged", order.join(">"));

    // ---- step through every stage by tap/click --------------------------
    const stages = {};
    for (const st of ["prompt", "answer", "judged"]) {
      await press(`[data-specimen-step="${st}"]`);
      await sleep(450);
      stages[st] = await page.evaluate(stageMetrics);
      await page.locator("[data-specimen]").screenshot({ path: path.join(MEDIA, `s135-${config}-specimen-${st}.png`) });
      const s = stages[st];
      check(config, `${st}: chosen by ${c.phone ? "tap" : "click"}, the only stage shown`, s.stage === st && s.shown === 1, s.stage);
      if (c.phone) {
        check(config, `${st}: no blank run over 24 px between parts or under the last part`, s.maxRun <= 24, s.runs);
        check(config, `${st}: phone parts are 12 px apart`, s.runs.filter((r) => !/under its last/.test(r.where) && !/before \.compare|before \.verdict|before \.specimen-schedule/.test(r.where)).every((r) => near(r.px, 12, 1)) || st === "judged", s.runs);
        if (st !== "judged") {
          check(config, `${st}: drawn like the phone's Study screen (prompt, field, Return to check + mic + Check, one card coming up)`,
            s.parts.join(",") === "specimen-prompt,specimen-study" && s.desk === 0 && s.field && s.mic && s.check && s.next, s.parts);
          check(config, `${st}: the field fills what the stage has left (>= 134 px), page ground, 1 px line, 4 px radius, 12/16 px padding, 18 px / 1.5`,
            s.field.h >= 134 && s.field.radius === "4px" && s.field.border === "1px" && s.field.pad === "12px 16px" && s.field.font === "18px/27px", s.field);
          check(config, `${st}: field holds ${st === "prompt" ? "\"Type your answer\" after the accent caret" : "the typed answer"}`,
            st === "prompt" ? s.field.text === "Type your answer" : /^don't assume malice/.test(s.field.text), s.field.text);
          check(config, `${st}: round 44 px mic; bordered (not filled) 44 px Check`, near(s.mic.w, 44) && near(s.mic.h, 44) && s.mic.radius === "50%" && s.check.h >= 44 && s.check.border === "1px" && /rgba\(0, 0, 0, 0\)|transparent/.test(s.check.bg), { mic: s.mic, check: s.check });
          check(config, `${st}: one upcoming card under a 1 px rule, on one line`, s.next.text === "NATO alphabet: K" && s.next.lines === 1 && s.next.ruleTop === "1px", s.next);
          check(config, `${st}: the drawn controls are a picture: aria-hidden, nothing focusable`, s.controlsHidden && s.focusable === 0, { hidden: s.controlsHidden, focusable: s.focusable });
        }
      } else {
        check(config, `${st}: desktop Specimen unchanged (answer line and hint, no phone picture)`, st === "judged" || (s.desk === 2 && !s.field), { desk: s.desk });
      }
    }
    const hs = Object.values(stages).map((s) => s.figH), heads = Object.values(stages).map((s) => s.headTop);
    check(config, "Specimen keeps one height in every stage (nothing below jumps)", hs.every((h) => near(h, hs[0], 0.5)), hs);
    check(config, "subject header does not move between stages", heads.every((h) => near(h, heads[0], 0.5)), heads);
    if (c.phone) {
      const s = stages.prompt;
      check(config, "phone: Specimen 358 px wide, 16 px inside (4 px above the 44 px steps), content column 324 px",
        near(s.figW, 358) && s.padding.join(" ") === "4px 16px 16px 16px" && near(s.stepsTop, 5) && near(s.stepsH, 45, 1) && near(s.contentW, 324), { w: s.figW, padding: s.padding, stepsTop: s.stepsTop, stepsH: s.stepsH, content: s.contentW });
      check(config, "phone: the field is about 268 px at 390x844", stages.prompt.field.h >= 240 && stages.prompt.field.h <= 300, stages.prompt.field.h);
      // Tapping the drawn controls does nothing more than tapping the Specimen.
      await press('[data-specimen-step="prompt"]');
      await sleep(300);
      const before = page.url();
      await page.tap('[data-specimen-stage="prompt"] .specimen-check');
      await page.tap('[data-specimen-stage="prompt"] .specimen-mic');
      await sleep(300);
      const after = await page.evaluate(() => [location.href, document.querySelector(".specimen-stage.is-shown").dataset.specimenStage]);
      check(config, "tapping the drawn Check and mic does nothing (no navigation, stage unchanged)", after[0] === before && after[1] === "prompt", after);
    }
    await pageHealth(page, config, "landing after stepping");

    // ---- the newcomer's path on ------------------------------------------
    await Promise.all([page.waitForURL(/\/signup/), press(".hero .btn-accent")]);
    await sleep(400);
    check(config, "\"Start the 7-day trial\" opens sign-up", /\/signup/.test(page.url()) && (await page.locator("h1").first().innerText()).length > 0, page.url());
    await pageHealth(page, config, "sign-up");
    await page.goBack();
    await page.waitForURL((u) => new URL(u).pathname === "/");
    await Promise.all([page.waitForURL(/\/signin/), press("header a[href='/signin']")]);
    check(config, "header \"Sign in\" opens sign-in", /\/signin/.test(page.url()), page.url());
    await page.fill("#email", "phaedrus@example.com");
    await page.fill("#password", "know-thyself");
    await Promise.all([page.waitForNavigation(), press("main form button.btn-accent")]);
    await sleep(500);
    const study = await page.evaluate(() => ({ path: location.pathname, prompt: !!document.querySelector("[data-asking], [data-judged], .empty-state, .shelf") }));
    check(config, "typed demo sign-in lands in Study", study.path.startsWith("/study") && study.prompt, study);
    await page.screenshot({ path: path.join(MEDIA, `s135-${config}-study-after-sign-in.png`) });
    await pageHealth(page, config, "Study after sign-in");

    check(config, "0 console errors, page errors, failed requests or responses >= 400", problems.length === 0, problems.slice(0, 8).join(" | "));
    const video = page.video();
    await ctx.close();
    if (video) {
      const webm = path.join(MEDIA, `s135-${config}-walkthrough.webm`);
      await video.saveAs(webm);
      try {
        execFileSync("ffmpeg", ["-y", "-loglevel", "error", "-i", webm, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", webm.replace(/\.webm$/, ".mp4")]);
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

// A narrow phone breaks the terms into two phrase-whole lines; under
// reduced motion the Specimen rests on Judged and never moves.
async function edges(port) {
  const config = "edge-cases";
  const srv = startServer(config, port);
  const browser = await chromium.launch({ headless: false, executablePath: CHROME });
  try {
    await waitHealthy(srv.base);
    for (const mode of ["light", "dark"]) {
      const ctx = await browser.newContext({ viewport: { width: 320, height: 700 }, colorScheme: mode, isMobile: true, hasTouch: true, deviceScaleFactor: 2 });
      await ctx.addInitScript(initScript);
      const page = await ctx.newPage();
      const problems = [];
      instrument(page, problems);
      await page.goto(srv.base + "/");
      await sleep(300);
      const m = await page.evaluate(heroMetrics);
      const lone = m.lines.some((l) => l.replace(/[·\s]/g, " ").trim().split(/\s+/).length < 2);
      check(`narrow-320-${mode}`, "trial terms break into two phrase-whole lines, only after a \" · \", never a lone word",
        m.termLines === 2 && m.phraseRects.every((n) => n === 1) && m.lines.every((l, i) => i === m.lines.length - 1 || /·$/.test(l.trim())) && !lone, m.lines);
      await page.locator(".hero-act").screenshot({ path: path.join(MEDIA, `s135-narrow-320-${mode}-terms.png`) });
      await pageHealth(page, `narrow-320-${mode}`, "landing");
      check(`narrow-320-${mode}`, "0 console errors, page errors, failed requests or responses >= 400", problems.length === 0, problems.join(" | "));
      await ctx.close();
    }
    for (const vp of [{ width: 390, height: 844, phone: true }, { width: 1440, height: 900 }]) {
      const name = `reduced-motion-${vp.phone ? "phone" : "desktop"}`;
      const ctx = await browser.newContext({ viewport: { width: vp.width, height: vp.height }, reducedMotion: "reduce", ...(vp.phone ? { isMobile: true, hasTouch: true, deviceScaleFactor: 2 } : {}) });
      const page = await ctx.newPage();
      await page.goto(srv.base + "/");
      const seen = new Set();
      for (let i = 0; i < 40; i++) {
        seen.add(await page.evaluate(() => document.querySelector(".specimen-stage.is-shown").dataset.specimenStage));
        await sleep(150);
      }
      check(name, "under reduced motion the Specimen rests on Judged and never moves (6 s)", seen.size === 1 && seen.has("judged"), [...seen]);
      await ctx.close();
    }
    // Without script the server's Judged stage is complete and still.
    const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, javaScriptEnabled: false, isMobile: true, hasTouch: true, deviceScaleFactor: 2 });
    const page = await ctx.newPage();
    await page.goto(srv.base + "/");
    const ns = await page.evaluate(() => [...document.querySelectorAll("[data-specimen-stage]")].filter((s) => getComputedStyle(s).visibility !== "hidden").map((s) => s.dataset.specimenStage));
    check("no-script-phone", "without script the Specimen shows Judged, complete", ns.join() === "judged", ns);
    await ctx.close();
  } finally {
    await browser.close().catch(() => {});
    srv.proc.kill("SIGTERM");
  }
}

const CONFIGS = [
  { name: "desktop-light", viewport: { width: 1440, height: 900 }, colorScheme: "light" },
  { name: "desktop-dark", viewport: { width: 1440, height: 900 }, colorScheme: "dark" },
  { name: "phone-light", viewport: { width: 390, height: 844 }, colorScheme: "light", phone: true },
  { name: "phone-dark", viewport: { width: 390, height: 844 }, colorScheme: "dark", phone: true },
].filter((c) => !ONLY || ONLY.includes(c.name));

let port = PORT0;
for (const c of CONFIGS) await journey(c, port++);
if (!ONLY) await edges(port++);
fs.writeFileSync(path.join(MEDIA, "s135-results.json"), JSON.stringify({ sha: SHA, checks: results.length, failed, results }, null, 2));
console.log(`\n${results.length - failed}/${results.length} checks PASS`);
process.exit(failed ? 1 : 0);
