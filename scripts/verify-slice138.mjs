#!/usr/bin/env node
// verify-slice138.mjs: check Slice 138 ([S138-1]) on the real running path,
// in a real headed Chrome: the landing page says "Remember everything."
//
//   make build BUILD_SHA=$(git rev-parse HEAD)
//   DISPLAY=:0 node scripts/verify-slice138.mjs --bin ./bin/scry --sha <sha> \
//        --media ~/chief-evidence/media [--baseline-bin /path/to/pre-change/scry] \
//        [--port 18138] [--only desktop-light,...] [--expect-fail]
//
// For each of desktop 1440x900 (DPR 1) and phone 390x844 (touch, DPR 2),
// light and dark, it starts its own Scry (the --bin binary) in SCRY_MODE=test
// with the demo learner seeded on a fresh SQLite file in a new temporary
// directory and a minimal environment (no provider keys; nothing leaves the
// machine), records a video, and walks a newcomer's path:
//
//   landing (signed out) -> read the hero, step the Specimen -> "Start the
//   7-day trial" (sign-up page) -> back -> header "Sign in" -> sign in as
//   the demo learner -> Study.
//
// On the landing page: the one h1 reads exactly "Remember everything." in
// Display type (Geist, weight 500, the --display size), set as "Remember"
// over "everything." (two lines, no word broken across lines), nothing
// clipped; document.title and og:title are "Remember everything · scry"; the
// page text and HTML contain no "Keep what's worth knowing" in any escaping;
// the description is unchanged. With --baseline-bin (the pre-change binary)
// the rest of the hero, the How Scry works rows and the head are compared
// with the baseline's, text and layout, and must be identical but for the
// headline. Every page is checked for sideways scroll, loaded styles,
// decoded images and CSP violations; console errors, page errors, failed
// requests and responses >= 400 must be 0. Exit 0 only when every check
// passes (with --expect-fail, the run is a negative control and exits 0
// only when at least one S138-1 check fails).

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
const BASE_BIN = args["baseline-bin"] ? path.resolve(args["baseline-bin"]) : "";
const MEDIA = path.resolve(args.media || path.join(os.homedir(), "chief-evidence", "media"));
const PORT0 = Number(args.port || 18138);
const SHA = args.sha || "";
const ONLY = args.only ? args.only.split(",") : null;
const EXPECT_FAIL = !!args["expect-fail"];
const PREFIX = args.prefix || "s138";
const CHROME = process.env.CHROME_BIN || "/usr/bin/google-chrome";

const HEADLINE = "Remember everything.";
const TITLE = "Remember everything · scry";
const OLD = ["Keep what's worth knowing", "Keep what’s worth knowing", "Keep what&#39;s worth knowing", "Keep what&rsquo;s worth knowing"];
const DESCRIPTION = "Scry gets what's worth knowing into your head and keeps it there, for the fewest minutes a day.";

fs.mkdirSync(MEDIA, { recursive: true });
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "scry-s138-"));
const results = [];
let failed = 0;
function check(config, name, ok, detail = "") {
  results.push({ config, name, ok: !!ok, detail: typeof detail === "string" ? detail : JSON.stringify(detail) });
  if (!ok) failed++;
  console.log(`${ok ? "PASS" : "FAIL"} [${config}] ${name}${detail !== "" ? " — " + (typeof detail === "string" ? detail : JSON.stringify(detail)) : ""}`);
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ---- the server ---------------------------------------------------------

function startServer(bin, label, port) {
  const dir = fs.mkdtempSync(path.join(tmp, label + "-"));
  const log = fs.createWriteStream(path.join(dir, "server.log"));
  const proc = spawn(bin, [], {
    env: { PATH: process.env.PATH, HOME: dir, SCRY_MODE: "test", SCRY_SEED_DEMO: "1", SCRY_DB: path.join(dir, "s138.db"), PORT: String(port) },
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
    // Chrome cancels a favicon fetch still in flight when the page navigates.
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

// The headline, read from the live DOM: its text, its type, and how it sets
// (each word's line boxes, from a Range over the word).
async function readHeadline(page) {
  return page.evaluate(() => {
    const h1s = [...document.querySelectorAll("h1")];
    const h = document.querySelector(".hero h1");
    if (!h) return { count: h1s.length, missing: true };
    const cs = getComputedStyle(h), hb = h.getBoundingClientRect();
    const node = [...h.childNodes].find((n) => n.nodeType === 3);
    const words = [];
    if (node) {
      const re = /\S+/g;
      let m;
      while ((m = re.exec(node.data))) {
        const r = document.createRange();
        r.setStart(node, m.index);
        r.setEnd(node, m.index + m[0].length);
        const rects = [...r.getClientRects()].filter((b) => b.width > 0);
        const all = r.getBoundingClientRect();
        words.push({ word: m[0], lines: rects.length, top: rects[0] ? Math.round(rects[0].top) : null, left: rects[0]?.left, right: rects[rects.length - 1]?.right,
          box: { left: all.left, right: all.right, top: all.top, bottom: all.bottom } });
      }
    }
    const lineTops = [...new Set(words.map((w) => w.top))];
    const lines = lineTops.map((t) => words.filter((w) => w.top === t).map((w) => w.word).join(" "));
    // --display resolved for this viewport, and 16ch of this face, read
    // from probe elements set the same way as the headline.
    const probe = document.createElement("span");
    probe.className = "display";
    probe.style.cssText = "position:absolute;visibility:hidden;white-space:nowrap;width:16ch";
    h.parentElement.appendChild(probe);
    const display = getComputedStyle(probe).fontSize, ch16 = probe.getBoundingClientRect().width;
    probe.remove();
    // Clipping: every ancestor that does not let overflow show, and the
    // viewport, must hold every word's full line box.
    const clippers = [];
    for (let el = h; el && el !== document.documentElement; el = el.parentElement) {
      const o = getComputedStyle(el);
      if (o.overflowX !== "visible" || o.overflowY !== "visible" || (o.clipPath && o.clipPath !== "none")) clippers.push(el);
    }
    return {
      count: h1s.length, text: h.textContent, cls: h.className, tag: h.tagName,
      family: cs.fontFamily, weight: cs.fontWeight, size: cs.fontSize, lineHeight: cs.lineHeight, maxWidth: cs.maxWidth, display,
      lines, words, ch16, clipperCount: clippers.length,
      clipped: h.scrollWidth > h.clientWidth + 0.5 || words.some((w) => w.box && (w.box.left < -0.5 || w.box.right > window.innerWidth + 0.5 || clippers.some((c) => {
        const b = c.getBoundingClientRect();
        return w.box.left < b.left - 0.5 || w.box.right > b.right + 0.5 || w.box.top < b.top - 0.5 || w.box.bottom > b.bottom + 0.5;
      }))),
      box: { left: hb.left, right: hb.right, top: hb.top, bottom: hb.bottom, width: hb.width, height: hb.height }, vw: window.innerWidth,
      parentRight: h.parentElement.getBoundingClientRect().right,
      inkInBox: words.every((w) => w.left >= hb.left - 0.5 && w.right <= hb.right + 0.5),
      visible: cs.visibility === "visible" && cs.display !== "none" && +cs.opacity > 0,
    };
  });
}

// Everything else in the hero, the rows below and the head: text and
// layout, headline excluded, for comparison with the pre-change build.
async function readRest(page) {
  return page.evaluate(() => {
    const q = (s) => document.querySelector(s);
    const r = (el) => { if (!el) return null; const b = el.getBoundingClientRect(); return [b.left, b.width, b.height].map((v) => Math.round(v)); };
    const meta = (sel) => q(sel)?.getAttribute("content") ?? null;
    return {
      lede: q(".hero .lede")?.textContent.trim(),
      actions: [...document.querySelectorAll(".hero .actions a")].map((a) => a.textContent.trim() + " -> " + a.getAttribute("href") + " " + a.className),
      terms: q(".hero .trial-terms-line")?.textContent.replace(/\s+/g, " ").trim(),
      specimen: q(".specimen")?.innerText.replace(/\s+/g, " ").trim(),
      specimenBox: r(q(".specimen")),
      ledeBox: r(q(".hero .lede")), actionsBox: r(q(".hero .actions")), termsBox: r(q(".hero .trial-terms-line")),
      rows: [...document.querySelectorAll(".plain-list .plain-row")].map((x) => x.innerText.replace(/\s+/g, " ").trim()),
      description: meta('meta[name="description"]'), ogDescription: meta('meta[property="og:description"]'),
      ogImage: (meta('meta[property="og:image"]') || "").replace(location.origin, ""), header: q("header.top")?.innerText.replace(/\s+/g, " ").trim(),
      foot: q("[data-public-foot]")?.innerText.replace(/\s+/g, " ").trim() ?? null,
    };
  });
}

async function baselineRest(c, port) {
  if (!BASE_BIN) return null;
  const srv = startServer(BASE_BIN, "baseline-" + c.name, port);
  const browser = await chromium.launch({ headless: false, executablePath: CHROME });
  try {
    await waitHealthy(srv.base);
    const ctx = await browser.newContext(contextOptions(c));
    const page = await ctx.newPage();
    await page.goto(srv.base + "/");
    await page.mouse.move(1, 1).catch(() => {});
    await sleep(600);
    const rest = await readRest(page);
    const head = await readHeadline(page);
    await ctx.close();
    return { rest, head };
  } finally {
    await browser.close().catch(() => {});
    srv.proc.kill("SIGTERM");
  }
}

function contextOptions(c) {
  return { viewport: c.viewport, colorScheme: c.colorScheme, timezoneId: "America/Chicago", locale: "en-US",
    ...(c.phone ? { isMobile: true, hasTouch: true, deviceScaleFactor: 2 } : {}) };
}

// The browser tab, as the X display shows it (the real window, headed).
function grabTab(file, width) {
  if (!process.env.DISPLAY) return false;
  try {
    execFileSync("import", ["-window", "root", "-crop", `${Math.min(width, 900)}x90+0+0`, "+repage", file], { stdio: "ignore" });
    return true;
  } catch { return false; }
}

// ---- one config ---------------------------------------------------------

async function journey(c, port) {
  const config = c.name;
  const base0 = await baselineRest(c, port + 100).catch((e) => { check(config, "baseline build read", false, String(e)); return null; });
  const srv = startServer(BIN, config, port);
  const base = srv.base;
  const browser = await chromium.launch({ headless: false, executablePath: CHROME, args: [`--window-size=${c.viewport.width},${c.viewport.height + 140}`, "--window-position=0,0"] });
  try {
    const health = await waitHealthy(base);
    check(config, "/healthz 200 with the build sha", health.status === "ok" && (!SHA || SHA.startsWith(health.build_sha)), `status=${health.status} build_sha=${health.build_sha} mode=${health.mode}`);
    const ctx = await browser.newContext({ ...contextOptions(c), recordVideo: { dir: tmp, size: c.viewport } });
    await ctx.addInitScript(initScript);
    const page = await ctx.newPage();
    const problems = [];
    instrument(page, problems);

    // ---- Landing, signed out: the first thing a newcomer reads ----------
    await page.goto(base + "/");
    await page.mouse.move(1, 1).catch(() => {});
    await sleep(900);
    const h = await readHeadline(page);
    check(config, "[S138-1] the landing page has exactly one h1", h.count === 1, `count=${h.count}`);
    check(config, `[S138-1] the h1 reads exactly "${HEADLINE}"`, h.text === HEADLINE, JSON.stringify(h.text));
    check(config, "[S138-1] Display type: .display, Geist, weight 500, the --display size, line-height 1.1",
      h.cls === "display" && /^Geist/.test(h.family) && h.weight === "500" && h.size === h.display && Math.abs(parseFloat(h.lineHeight) - 1.1 * parseFloat(h.size)) < 0.6,
      { cls: h.cls, family: h.family.slice(0, 20), weight: h.weight, size: h.size, display: h.display, lineHeight: h.lineHeight });
    check(config, "[S138-1] in the hero's existing 16ch headline measure (max-width 16ch, never wider)", Math.abs(parseFloat(h.maxWidth) - h.ch16) < 0.6 && h.box.width <= parseFloat(h.maxWidth) + 0.5, { maxWidth: h.maxWidth, ch16: h.ch16, width: h.box.width });
    check(config, '[S138-1] sets as two lines, "Remember" over "everything."', JSON.stringify(h.lines) === JSON.stringify(["Remember", "everything."]), h.lines);
    check(config, "[S138-1] no word broken across lines", h.words.length === 2 && h.words.every((w) => w.lines === 1), h.words.map((w) => `${w.word}:${w.lines}`));
    check(config, "[S138-1] nothing clipped (no sideways overflow in the h1; every word's line box inside the h1's width, the viewport and any clipping ancestor)",
      !h.clipped && h.inkInBox && h.box.left >= 0 && h.box.right <= h.vw && h.box.right <= h.parentRight + 0.5 && h.visible, { box: h.box, vw: h.vw, clippers: h.clipperCount, clipped: h.clipped });
    const head = await page.evaluate(() => ({ title: document.title, og: document.querySelector('meta[property="og:title"]')?.getAttribute("content"), html: document.documentElement.outerHTML, text: document.body.innerText }));
    check(config, `[S138-1] document.title is "${TITLE}"`, head.title === TITLE, head.title);
    check(config, `[S138-1] og:title is "${TITLE}"`, head.og === TITLE, head.og);
    check(config, `[S138-1] the page text contains no "Keep what's worth knowing"`, !OLD.some((o) => head.text.includes(o)), "");
    const raw = await (await fetch(base + "/")).text();
    check(config, `[S138-1] the served HTML (head included) contains no "Keep what's worth knowing" in any escaping`, !OLD.some((o) => raw.includes(o) || head.html.includes(o)), "");
    const rest = await readRest(page);
    check(config, "[S138-1] the site's description is unchanged", rest.description === DESCRIPTION && rest.ogDescription === DESCRIPTION, rest.description);
    check(config, '[S138-1] one hero accent action, "Start the 7-day trial"', rest.actions.filter((a) => /btn-accent/.test(a)).length === 1 && rest.actions.some((a) => /^Start the 7-day trial -> \/signup/.test(a)), rest.actions);
    if (base0) {
      const same = (k) => JSON.stringify(rest[k]) === JSON.stringify(base0.rest[k]);
      const keys = Object.keys(rest);
      const diff = keys.filter((k) => !same(k));
      check(config, "[S138-1] the rest of the hero, the four rows, the header, footer and head are identical to the pre-change build (text and layout)", diff.length === 0 && rest.rows.length === 4,
        diff.length ? diff.map((k) => `${k}: ${JSON.stringify(base0.rest[k])} -> ${JSON.stringify(rest[k])}`).join(" | ") : `${keys.length} fields identical; baseline h1 was ${JSON.stringify(base0.head.text)} on ${base0.head.lines?.length} lines`);
    }
    await page.screenshot({ path: path.join(MEDIA, `${PREFIX}-${config}-landing.png`) });
    await page.locator(".hero-copy").screenshot({ path: path.join(MEDIA, `${PREFIX}-${config}-headline.png`) });
    if (!c.phone && grabTab(path.join(MEDIA, `${PREFIX}-${config}-browser-tab.png`), c.viewport.width)) {
      check(config, "browser tab captured from the X display", fs.existsSync(path.join(MEDIA, `${PREFIX}-${config}-browser-tab.png`)));
    }
    await pageHealth(page, config, "landing");

    // The Specimen still steps (a visitor clicks or taps its controls).
    await page.locator(".specimen").scrollIntoViewIfNeeded();
    for (const step of ["prompt", "answer", "judged"]) {
      const btn = page.locator(`[data-specimen-step=${step}]`);
      if (c.phone) await btn.tap(); else await btn.click();
      await sleep(450);
      const shown = await page.evaluate((s) => document.querySelector(`[data-specimen-stage=${s}]`)?.classList.contains("is-shown") && document.querySelector(`[data-specimen-step=${s}]`)?.getAttribute("aria-pressed") === "true", step);
      check(config, `Specimen: choosing "${step}" shows that stage`, shown);
    }
    await page.evaluate(() => window.scrollTo(0, 0));
    await sleep(400);

    // ---- "Start the 7-day trial" opens sign-up -------------------------
    const cta = page.locator(".hero .btn-accent");
    await Promise.all([page.waitForURL(/\/signup/), c.phone ? cta.tap() : cta.click()]);
    await sleep(500);
    check(config, "Start the 7-day trial opens the sign-up page", new URL(page.url()).pathname === "/signup", page.url());
    await pageHealth(page, config, "sign up");
    await page.goBack();
    await page.waitForURL((u) => new URL(u).pathname === "/");
    await sleep(500);
    check(config, "back on the landing page, the headline is still the first h1", (await readHeadline(page)).text === HEADLINE);

    // ---- Sign in from the header, as the demo learner ------------------
    const signin = page.locator("header a[href='/signin']");
    await Promise.all([page.waitForURL(/\/signin/), c.phone ? signin.tap() : signin.click()]);
    await page.fill("#email", "phaedrus@example.com");
    await page.fill("#password", "know-thyself");
    await Promise.all([page.waitForURL((u) => new URL(u).pathname !== "/signin"), page.click("main form button.btn-accent")]);
    await sleep(600);
    check(config, "signing in from the header lands in Study", new URL(page.url()).pathname.startsWith("/study"), page.url());
    await pageHealth(page, config, "Study");
    const signedIn = await page.goto(base + "/");
    check(config, "signed in, / goes straight to Study (the landing headline is for newcomers)", new URL(page.url()).pathname.startsWith("/study") && signedIn.ok(), page.url());
    await sleep(400);

    check(config, "0 console errors, page errors, failed requests or responses >= 400", problems.length === 0, problems.slice(0, 8).join(" | "));
    const video = page.video();
    await ctx.close();
    if (video) {
      const webm = path.join(MEDIA, `${PREFIX}-${config}-walkthrough.webm`);
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
  const s138 = results.filter((r) => r.name.startsWith("[S138-1]") && !r.ok).length;
  console.log(`negative control: ${s138} [S138-1] checks failed`);
  process.exit(s138 > 0 ? 0 : 1);
}
process.exit(failed ? 1 : 0);
