#!/usr/bin/env node
// verify-slice70-folio.mjs: the Slice 70 Folio lines that verify-slice70.mjs
// does not measure, checked on a running test-mode Scry with disposable
// learners only (reconciliation of intent 8f3ce40f).
//
//   SCRY_MODE=test SCRY_SEED_DEMO=1 SCRY_TEST_JUDGMENTS=missed,right,close SCRY_DB=/tmp/f.db PORT=18071 ./bin/scry &
//   node scripts/verify-slice70-folio.mjs --url http://127.0.0.1:18071 --out /tmp/f-media --state /tmp/f-phone-state.json
//   # restart the same database with SCRY_TEST_CLOCK 3 days on, then:
//   node scripts/verify-slice70-folio.mjs --url http://127.0.0.1:18071 --out /tmp/f-media --state /tmp/f-phone-state.json --stale
//
// WebKit iPhone 390x844 (touch, light) and Chromium 1440x900 (dark):
//   - phone header: 48 px brand row over one 52 px nav row of six equal
//     columns, 16 px icon over an 11 px mono label; the Study rail hides
//     "Study"/its purpose on the phone and keeps them on desktop;
//   - the focused answer field wears no accent ring;
//   - Correct, Almost Correct and Incorrect reviews: the why is the plain
//     sentence of DESIGN.md's table, never the grader's rubric; the badge is
//     restrained (1 px border, 4 px radius, mono, 16 px icon, no fill except
//     the Almost Correct highlighter, no shadow);
//   - the schedule table: four across (two on the phone) with Reviews full
//     width below, 14 px icons, a 1 px hairline frame, radius 0, no fill;
//   - a card's history shows plain whys only;
//   - an exact text answered with nothing: "No answer given.", every word
//     highlighted as missing, Incorrect, "Skipped — review the answer above.";
//   - --stale: the phone's pending review, reopened after its due instant,
//     reads "due now" beside the same "1 day after this answer".
// Sign-up happens in an unrecorded context, so no credential is on video.
// Console errors raised inside a WebKit screenshot (its injected <style> is
// refused by Scry's CSP) are listed as tooling, not counted.
import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { chromium, webkit, devices } from "playwright-core";

const a = process.argv.slice(2);
const get = k => { const i = a.indexOf(k); return i >= 0 ? a[i + 1] : ""; };
const base = get("--url").replace(/\/$/, "");
const out = get("--out");
const stale = a.includes("--stale");
const statePath = get("--state") || path.join(out, "phone-state.json");
if (!base || !out) { console.error("--url and --out are required"); process.exit(2); }
fs.mkdirSync(out, { recursive: true });

const PLAIN = { right: "Same meaning as the correct answer.", close: "Close — part of it is missing or imprecise.", missed: "Not quite — compare it with the correct answer above." };
const SKIP = "Skipped — review the answer above.";
const RUBRIC = /grader|contradictory|accepted answer|cannot reliably be classified|materially imprecise|irrelevant/i;
const results = [];
const check = (name, ok, detail) => { results.push({ name, ok: !!ok, detail }); console.error((ok ? "PASS " : "FAIL ") + name + (ok ? "" : "  " + JSON.stringify(detail))); };

const CONFIGS = [
  { name: "phone", engine: "webkit", mobile: true, scheme: "light" },
  { name: "desktop", engine: "chromium", mobile: false, scheme: "dark" },
];
function ctxOpts(c, video) {
  const o = { colorScheme: c.scheme, timezoneId: "America/Chicago", locale: "en-US" };
  if (c.mobile) Object.assign(o, { userAgent: devices["iPhone 13"].userAgent, viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true });
  else Object.assign(o, { viewport: { width: 1440, height: 900 } });
  if (video) o.recordVideo = { dir: video, size: o.viewport };
  return o;
}
const launch = e => e === "webkit" ? webkit.launch() : chromium.launch({ executablePath: process.env.CHROME_BIN || "/usr/bin/google-chrome" });
const press = (c, l) => c.mobile ? l.tap() : l.click();
const settle = async p => { await p.waitForLoadState("load"); await p.waitForTimeout(400); };
// WebKit's screenshot injects an inline <style> that Scry's CSP refuses; errors raised inside a screenshot are tooling, not the app.
const tooling = [];
const shot = async (p, name) => { const h = p.__h, n = h ? h.console.length : 0; await p.screenshot({ path: path.join(out, name + ".png"), caret: "initial" }); await p.waitForTimeout(150); if (h) tooling.push(...h.console.splice(n).map(x => name + ": " + x)); };

async function answer(page, c, text) {
  await page.waitForSelector("[data-answer-form]");
  const input = page.locator("#answer");
  await press(c, input);
  if (text) await input.pressSequentially(text);
  await Promise.all([page.waitForSelector("[data-review]", { timeout: 15000 }), press(c, page.locator("[data-check]"))]);
  await settle(page);
}
async function next(page, c) {
  await Promise.all([page.waitForURL(/\/study/), press(c, page.locator("[data-next]"))]);
  await settle(page);
}
const review = page => page.evaluate(() => {
  const q = s => document.querySelector(s);
  const badge = q(".verdict-badge"), bs = badge && getComputedStyle(badge), icon = badge && badge.querySelector("svg");
  const cells = [...document.querySelectorAll("[data-telemetry] .fact")].map(f => { const r = f.getBoundingClientRect(); const ic = f.querySelector("dt svg"); return { label: f.querySelector("dt").textContent.trim(), top: Math.round(r.top), left: Math.round(r.left), w: Math.round(r.width), icon: ic ? Math.round(ic.getBoundingClientRect().width) : 0 }; });
  const tel = q("[data-telemetry]"), ts = tel && getComputedStyle(tel);
  return {
    kind: q("[data-review]").dataset.kind, verdict: badge && badge.dataset.verdict, label: q("[data-verdict-label]")?.textContent.trim(),
    why: q("[data-review] .why")?.textContent.trim(), skipped: !!q("[data-skipped]"),
    missingWords: document.querySelectorAll("[data-submitted] ins.x-missing").length, sameWords: document.querySelectorAll("[data-submitted] .x-same").length,
    canonicalWords: (q("[data-canonical]")?.textContent.trim().split(/\s+/) || []).length,
    badge: bs && { border: bs.borderTopWidth + " " + bs.borderTopStyle, radius: bs.borderTopLeftRadius, bg: bs.backgroundColor, font: bs.fontFamily, shadow: bs.boxShadow, icon: icon ? Math.round(icon.getBoundingClientRect().width) : 0 },
    cells, telBorder: ts && ts.borderTopWidth + " " + ts.borderTopStyle, telRadius: ts && ts.borderTopLeftRadius, telBg: ts && ts.backgroundColor, telShadow: ts && ts.boxShadow,
    text: q("main").innerText, itemId: q("[data-edit-form] input[name=item_id]")?.value,
    nextRel: q("[data-next-rel]")?.textContent.trim(), interval: q("[data-interval]")?.textContent.trim(), intervalFrom: q("[data-interval-from]")?.textContent.trim(),
  };
});

async function runStale() {
  // Reopen the pending review left by the phone run after the server clock moved past its due instant.
  const c = CONFIGS[0];
  const browser = await launch(c.engine);
  const vdir = fs.mkdtempSync(path.join(os.tmpdir(), "s70f-video-"));
  const ctx = await browser.newContext(Object.assign(ctxOpts(c, vdir), { storageState: statePath }));
  const page = await ctx.newPage();
  await page.goto(base + "/study"); await settle(page);
  const r = await review(page);
  await shot(page, "rc-phone-review-reopened-after-due");
  check(`stale: reopened past its due instant the event reads "due now" (interval unchanged: ${r.interval} ${r.intervalFrom})`, r.nextRel === "due now" && /after this answer/.test(r.intervalFrom || ""), r.nextRel);
  check("stale: 'today' never sits beside the interval", !/\btoday\b/i.test(r.text.split("Next review")[1] || ""), r.nextRel);
  await ctx.close(); await browser.close();
  const v = fs.readdirSync(vdir).find(f => f.endsWith(".webm")); if (v) fs.copyFileSync(path.join(vdir, v), path.join(out, "rc-phone-reopened-due-now.webm"));
}

// Sign up off camera so no credential is ever in a video.
async function signUp(browser, c) {
  const pre = await browser.newContext(ctxOpts(c));
  const p0 = await pre.newPage();
  const email = `rc-${c.name}-${Date.now()}@example.com`;
  await p0.goto(base + "/signup"); await p0.fill("#email", email); await p0.fill("#password", "reconcile-disposable-pass");
  await Promise.all([p0.waitForURL(u => new URL(u).pathname !== "/signup"), press(c, p0.locator("main form button.btn-accent").first())]);
  const state = await pre.storageState(); await pre.close();
  return state;
}

// A second learner with only The Creeds: the first card is an exact text.
async function exactSkip(browser, c) {
  const state = await signUp(browser, c);
  const vdir = fs.mkdtempSync(path.join(os.tmpdir(), "s70f-video-"));
  const ctx = await browser.newContext(Object.assign(ctxOpts(c, vdir), { storageState: state }));
  const page = await ctx.newPage();
  await page.goto(base + "/shelf"); await settle(page);
  await press(c, page.locator("#the-creeds form[data-add] button"));
  await page.waitForTimeout(800);
  await page.goto(base + "/study"); await settle(page);
  await answer(page, c, "");
  const r = await review(page);
  await shot(page, `rc-${c.name}-exact-skip-review`);
  check(`${c.name}: exact text, empty answer: "No answer given.", every word highlighted as missing, Incorrect, "${SKIP}"`,
    r.kind === "exact" && r.skipped && r.missingWords > 0 && r.sameWords === 0 && r.missingWords === r.canonicalWords && r.label === "Incorrect" && r.why === SKIP, { kind: r.kind, skipped: r.skipped, missing: r.missingWords, same: r.sameWords, words: r.canonicalWords, label: r.label, why: r.why });
  await ctx.close();
  const v = fs.readdirSync(vdir).find(x => x.endsWith(".webm"));
  if (v) fs.copyFileSync(path.join(vdir, v), path.join(out, `rc-${c.name}-exact-skip.webm`));
}

async function run(c) {
  const browser = await launch(c.engine);
  const vdir = fs.mkdtempSync(path.join(os.tmpdir(), "s70f-video-"));
  const state = await signUp(browser, c);
  const ctx = await browser.newContext(Object.assign(ctxOpts(c, vdir), { storageState: state }));
  const page = await ctx.newPage();
  const health = { console: [], pageErrors: [], http5xx: [] }; page.__h = health;
  page.on("console", m => { if (m.type() === "error") health.console.push(m.text().slice(0, 160)); });
  page.on("pageerror", e => health.pageErrors.push(String(e).slice(0, 160)));
  page.on("response", r => { if (r.status() >= 500) health.http5xx.push(r.url()); });
  const tag = c.name;

  // Add NATO (first tile) and The Creeds (exact texts).
  await page.goto(base + "/shelf"); await settle(page);
  await press(c, page.locator("form[data-add]").first().locator("button"));
  await page.waitForTimeout(600);
  await page.goto(base + "/study"); await settle(page);

  // Waiting screen: header geometry and the rail.
  const g = await page.evaluate(() => {
    const r = e => e.getBoundingClientRect();
    const nav = document.querySelector("header nav[aria-label=Main]"), links = [...nav.querySelectorAll("a")];
    const brand = document.querySelector("header.top .brand") || document.querySelector("header.top a");
    const vis = e => !!e && getComputedStyle(e).display !== "none" && r(e).height > 0;
    const lbl = links[0].querySelector(".nav-label") || links[0];
    return { headerH: Math.round(r(document.querySelector("header.top")).height), navH: Math.round(r(nav).height), navTop: Math.round(r(nav).top),
      linkH: Math.round(r(links[0]).height), widths: links.map(l => Math.round(r(l).width)), icon: Math.round(r(links[0].querySelector("svg")).width),
      labelPx: getComputedStyle(lbl).fontSize, labelFont: getComputedStyle(lbl).fontFamily,
      railStudy: vis(document.querySelector(".margin-mode")), railPurpose: vis(document.querySelector(".margin-purpose")),
      place: vis(document.querySelector(".margin-idea")), count: vis(document.querySelector(".margin-count")) };
  });
  if (c.mobile) {
    check(`${tag}: brand row 48px above one 52px nav row (header ${g.headerH}px, nav top ${g.navTop}, nav ${g.navH}px)`, g.navTop === 48 && g.navH === 52 && g.headerH <= 101, g);
    check(`${tag}: six equal nav columns, 16px icon over an 11px mono label`, new Set(g.widths).size <= 2 && Math.max(...g.widths) - Math.min(...g.widths) <= 1 && g.icon === 16 && g.labelPx === "11px" && /mono/i.test(g.labelFont), g);
    check(`${tag}: Study rail hides "Study"/purpose, keeps the place line and count`, !g.railStudy && !g.railPurpose && g.place && g.count, g);
  } else {
    check(`${tag}: desktop rail keeps "Study" and its purpose`, g.railStudy && g.railPurpose, g);
  }
  // Answer field focus: one 2px ink edge, no accent ring.
  await press(c, page.locator("#answer"));
  const f = await page.evaluate(() => {
    const i = document.querySelector("#answer"), s = getComputedStyle(i), line = document.querySelector("[data-answer-line]"), ls = getComputedStyle(line);
    const probe = n => { const e = document.createElement("span"); e.style.color = `var(--${n})`; document.body.appendChild(e); const v = getComputedStyle(e).color; e.remove(); return v; };
    return { focused: document.activeElement === i, outline: s.outlineStyle + " " + s.outlineWidth, shadow: s.boxShadow, lineShadow: ls.boxShadow, lineBorder: ls.borderBottomWidth + " " + ls.borderBottomColor, lineOutline: ls.outlineStyle, accent: probe("accent"), ink: probe("ink") };
  });
  const accentIn = x => (x || "").includes(f.accent);
  check(`${tag}: focused answer field shows no accent ring (no outline or accent shadow)`, f.focused && /^none/.test(f.outline) && !accentIn(f.shadow) && !accentIn(f.lineShadow) && f.lineOutline === "none", f);
  await shot(page, `rc-${tag}-answer-focused`);

  // Answer three NATO cards with a real-ish text answer; every why must be the plain sentence for its verdict.
  const seen = {};
  let skipExactDone = true, pendingLeft = false;
  for (let i = 0; i < 40 && !(skipExactDone && Object.keys(seen).length >= 3); i++) {
    const isExact = await page.evaluate(() => !!document.querySelector("[data-asking] .exact-label"));
    if (isExact && !skipExactDone) {
      await answer(page, c, "");
      const r = await review(page);
      await shot(page, `rc-${tag}-exact-skip-review`);
      check(`${tag}: exact text, empty answer: "No answer given.", every word highlighted as missing, Incorrect, "${SKIP}"`,
        r.kind === "exact" && r.skipped && r.missingWords > 0 && r.sameWords === 0 && r.missingWords === r.canonicalWords && r.label === "Incorrect" && r.why === SKIP, { kind: r.kind, skipped: r.skipped, missing: r.missingWords, same: r.sameWords, words: r.canonicalWords, label: r.label, why: r.why });
      skipExactDone = true;
      await next(page, c); continue;
    }
    if (isExact || Object.keys(seen).length >= 3) { await answer(page, c, isExact ? "I believe" : "guess"); await next(page, c); continue; }
    await answer(page, c, "Alfa");
    const r = await review(page);
    if (!seen[r.verdict]) {
      seen[r.verdict] = true;
      check(`${tag}: ${r.label} review: the why is the plain sentence "${PLAIN[r.verdict]}", no rubric`, r.why === PLAIN[r.verdict] && !RUBRIC.test(r.text), r.why);
      const b = r.badge;
      const fillOK = r.verdict === "close" ? b.bg !== "rgba(0, 0, 0, 0)" : (b.bg === "rgba(0, 0, 0, 0)" || b.bg === "transparent");
      check(`${tag}: ${r.label} badge restrained: 1px border, 4px radius, mono, 16px icon, ${r.verdict === "close" ? "highlighter fill" : "no fill"}, no glow`, b.border === "1px solid" && b.radius === "4px" && /mono/i.test(b.font) && b.icon === 16 && fillOK && b.shadow === "none", b);
      if (Object.keys(seen).length === 1) {
        const cells = r.cells, rows = [...new Set(cells.map(x => x.top))];
        const rev = cells.find(x => x.label === "Reviews"), others = cells.filter(x => x.label !== "Reviews");
        const perRow = c.mobile ? 2 : 4;
        check(`${tag}: schedule table: ${perRow} across, Reviews full width below, 14px icons, hairline frame with no fill or shadow`,
          cells.length === 5 && new Set(others.slice(0, perRow).map(x => x.top)).size === 1 && rev.top > others[others.length - 1].top && rev.w >= Math.max(...others.map(x => x.w)) * (perRow - 0.2) && cells.every(x => x.icon === 14) && r.telBorder === "1px solid" && r.telRadius === "0px" && (r.telBg === "rgba(0, 0, 0, 0)") && r.telShadow === "none",
          { rows, cells, telBorder: r.telBorder, telRadius: r.telRadius, telBg: r.telBg });
        await shot(page, `rc-${tag}-review-${r.verdict}-schedule`);
        // Card history shows the plain why, never the stored criterion.
        const hist = await ctx.newPage();
        await hist.goto(base + "/library/items/" + r.itemId); await settle(hist);
        const h = await hist.evaluate(() => [...document.querySelectorAll(".history-why")].map(e => e.textContent.trim()));
        check(`${tag}: card history shows plain whys only (${h.length} rows)`, h.length > 0 && h.every(w => Object.values(PLAIN).includes(w) || w === SKIP || !/grader|rubric/i.test(w)) && !h.some(w => RUBRIC.test(w)), h);
        await hist.close();
      }
    }
    if (skipExactDone && Object.keys(seen).length >= 3 && c.mobile) { pendingLeft = true; break; } // leave this review pending for the stale check
    await next(page, c);
  }
  check(`${tag}: all three verdicts reached on the running path`, Object.keys(seen).length >= 3, Object.keys(seen));
  check(`${tag}: no console errors, page errors or 5xx`, !health.console.length && !health.pageErrors.length && !health.http5xx.length, health);
  if (c.mobile && pendingLeft) await ctx.storageState({ path: statePath });
  await ctx.close();
  const v = fs.readdirSync(vdir).find(x => x.endsWith(".webm"));
  if (v) fs.copyFileSync(path.join(vdir, v), path.join(out, `rc-${tag}-walkthrough.webm`));
  await exactSkip(browser, c);
  await browser.close();
}

if (stale) await runStale(); else for (const c of CONFIGS) await run(c);
const failed = results.filter(r => !r.ok).length;
console.error("tooling (screenshot-injected) console reports: " + tooling.length); console.log(JSON.stringify({ tooling, passed: results.length - failed, failed, results }, null, 1));
process.exit(failed ? 1 : 0);
