#!/usr/bin/env node
// verify-slice130.mjs: check [S125-1] (the L3-S3 brand mark, drawn from
// DESIGN.md's canonical SVG) and [S130-1] (the middle verdict reads
// "Almost") on the real running path, in a real headed Chrome.
//
//   make build BUILD_SHA=$(git rev-parse --short HEAD)
//   DISPLAY=:0 node scripts/verify-slice130.mjs --bin ./bin/scry --sha <sha> \
//        --media ~/chief-evidence/media [--port 18130]
//
// For each of desktop 1440x900 and phone 390x844, light and dark, it starts
// its own Scry (--bin) in SCRY_MODE=test with the demo learner seeded on a
// fresh SQLite file in a new temporary directory (no provider keys: the
// grader is the fixed fixture, scripted close, right, missed; auth and
// billing are local fakes; nothing leaves the machine), records a video and
// checks:
//
//   S125-1 the header mark is the canonical 32 x 32 drawing at 24 px, 8 px
//          before `scry`, showing exactly the active mode's fill set (and
//          switching when Settings' theme overrides the system); the cobalt
//          top card is #0562EF / #3B82F6 rotated 10° around (19, 14); the
//          SVG favicon, 16/32 px PNG favicons, 64 px app icons and home
//          screen icons decode at their sizes; the head links them.
//   S130-1 an answer graded close stops on its review with the badge
//          "Almost" (never "Almost Correct"), the trail says "Almost";
//          Correct and Incorrect keep their words; once the next card waits
//          the scrollback tag reads "almost"; the card's page lists it as
//          "almost".
//
// Every page is checked for sideways scroll, loaded styles, decoded images
// and CSP violations; console errors, page errors, failed requests and
// responses >= 400 must be 0. Exit 0 only when every check passes.

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
const PORT0 = Number(args.port || 18130);
const SHA = args.sha || "";
const CHROME = process.env.CHROME_BIN || "/usr/bin/google-chrome";
const CONFIGS = [
  { name: "desktop-light", viewport: { width: 1440, height: 900 }, colorScheme: "light" },
  { name: "desktop-dark", viewport: { width: 1440, height: 900 }, colorScheme: "dark" },
  { name: "phone-light", viewport: { width: 390, height: 844 }, colorScheme: "light", phone: true },
  { name: "phone-dark", viewport: { width: 390, height: 844 }, colorScheme: "dark", phone: true },
];
const TOP = { light: "rgb(5, 98, 239)", dark: "rgb(59, 130, 246)" };

fs.mkdirSync(MEDIA, { recursive: true });
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "scry-s130-"));
const results = [];
let failed = 0;
function check(config, name, ok, detail = "") {
  results.push({ config, name, ok: !!ok, detail: String(detail) });
  if (!ok) failed++;
  console.log(`${ok ? "PASS" : "FAIL"} [${config}] ${name}${detail !== "" ? " — " + detail : ""}`);
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function startServer(config, port) {
  const dir = fs.mkdtempSync(path.join(tmp, config + "-"));
  const log = fs.createWriteStream(path.join(dir, "server.log"));
  const proc = spawn(BIN, [], {
    env: {
      PATH: process.env.PATH, HOME: dir, SCRY_MODE: "test", SCRY_SEED_DEMO: "1", SCRY_DB: path.join(dir, "s130.db"), PORT: String(port),
      SCRY_TEST_JUDGMENTS: "close,right,missed",
    },
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
  check(config, `${label}: 0 CSP violations`, h.csp.length === 0, h.csp.join("; "));
}
async function settle(page) {
  await page.evaluate(async () => {
    const until = performance.now() + 1500;
    const frame = () => new Promise((r) => requestAnimationFrame(() => r()));
    while (performance.now() < until) {
      const moving = document.getAnimations().filter((a) => a.playState === "running" && a.effect?.getTiming().iterations !== Infinity);
      if (!moving.length && !document.documentElement.hasAttribute("data-vt")) break;
      await frame();
    }
    await frame();
    await frame();
  }).catch(() => {});
}
async function shot(page, name, opts = {}) {
  await settle(page);
  await page.screenshot({ path: path.join(MEDIA, name + ".png"), ...opts });
}

// The header mark as the browser draws it.
const readMark = (page) => page.evaluate(() => {
  const m = document.querySelector("header .mark"), w = document.querySelector("header .wordmark-text");
  const mb = m.getBoundingClientRect(), wb = w.getBoundingClientRect();
  const groups = [...m.querySelectorAll(":scope > g")];
  const shown = groups.filter((g) => getComputedStyle(g).display !== "none");
  const top = shown[0]?.querySelector("g[transform] rect");
  const bar = shown[0]?.querySelector("g[transform] rect + rect");
  const cards = shown[0] ? [...shown[0].querySelectorAll(":scope > rect")].map((r) => [r.getAttribute("x"), r.getAttribute("y"), r.getAttribute("width"), r.getAttribute("height"), getComputedStyle(r).fill]) : [];
  const icons = [...document.querySelectorAll("link[rel~=icon], link[rel=apple-touch-icon], link[rel=manifest], meta[property='og:image']")].map((l) => l.getAttribute("href") || l.getAttribute("content"));
  return {
    viewBox: m.getAttribute("viewBox"), w: mb.width, h: mb.height, gap: wb.left - mb.right, dy: (mb.top + mb.height / 2) - (wb.top + wb.height / 2),
    shown: shown.map((g) => g.getAttribute("class")), cards,
    top: top ? [top.getAttribute("x"), top.getAttribute("y"), top.getAttribute("width"), top.getAttribute("height"), getComputedStyle(top).fill, top.parentNode.getAttribute("transform")] : null,
    bar: bar ? [bar.getAttribute("width"), bar.getAttribute("height"), bar.getAttribute("opacity")] : null,
    animations: m.getAnimations ? m.getAnimations({ subtree: true }).length : 0, icons,
  };
});

async function journey(browser, c, port) {
  const config = c.name;
  const mode = c.colorScheme;
  const other = mode === "dark" ? "light" : "dark";
  const srv = startServer(config, port);
  const base = srv.base;
  let ctx;
  try {
    const health = await waitHealthy(base);
    check(config, "/healthz 200 with the build sha", health.status === "ok" && (!SHA || SHA.startsWith(health.build_sha) || health.build_sha.startsWith(SHA)), health.build_sha);
    const opts = { viewport: c.viewport, colorScheme: c.colorScheme, timezoneId: "America/Chicago", locale: "en-US",
      ...(c.phone ? { isMobile: true, hasTouch: true, deviceScaleFactor: 2 } : {}) };
    ctx = await browser.newContext({ ...opts, recordVideo: { dir: tmp, size: opts.viewport } });
    await ctx.addInitScript(initScript);
    const page = await ctx.newPage();
    const problems = [];
    instrument(page, problems);

    // ---- S125-1: the L3-S3 mark (signed out) ------------------------------
    await page.goto(base + "/");
    await page.mouse.move(1, 1);
    const mark = await readMark(page);
    check(config, "S125-1: header mark is the 32 x 32 drawing at 24 px, 8 px before scry, centred on the wordmark",
      mark.viewBox === "0 0 32 32" && mark.w === 24 && mark.h === 24 && Math.abs(mark.gap - 8) < 0.6 && Math.abs(mark.dy) <= 1, JSON.stringify({ viewBox: mark.viewBox, w: mark.w, gap: mark.gap, dy: mark.dy }));
    check(config, `S125-1: only the ${mode} fill set is shown`, mark.shown.length === 1 && mark.shown[0] === "mark-" + mode, mark.shown.join(","));
    check(config, "S125-1: two upright solid portrait cards 16 x 19 at (5, 10) and (7, 7.5)",
      mark.cards.length === 2 && mark.cards[0].slice(0, 4).join() === "5,10,16,19" && mark.cards[1].slice(0, 4).join() === "7,7.5,16,19", JSON.stringify(mark.cards));
    check(config, `S125-1: cobalt top card ${TOP[mode]} rotated 10° around (19, 14), with its prompt bar`,
      mark.top && mark.top.slice(0, 4).join() === "11,3.5,16,19" && mark.top[4] === TOP[mode] && mark.top[5] === "rotate(10 19 14)" && mark.bar && mark.bar.join() === "9,2,0.85", JSON.stringify({ top: mark.top, bar: mark.bar }));
    check(config, "S125-1: the mark never animates", mark.animations === 0, mark.animations);
    check(config, "S125-1: head links the SVG favicon, /favicon.ico, apple-touch icon, manifest and social preview",
      ["/static/favicon.svg", "/favicon.ico", "/static/icon-180.png", "/static/manifest.webmanifest"].every((u) => mark.icons.includes(u)) && mark.icons.some((i) => i.endsWith("/static/icon-maskable-512.png")), mark.icons.join(" "));
    const assets = await page.evaluate(async (urls) => {
      const out = {};
      for (const u of urls) {
        const img = new Image();
        img.src = u;
        try { await img.decode(); out[u] = [img.naturalWidth, img.naturalHeight]; } catch { out[u] = "undecodable"; }
      }
      return out;
    }, ["/static/favicon.svg", "/static/favicon-16.png", "/static/favicon-32.png", "/favicon.ico", "/static/app-icon.svg", "/static/app-icon-dark.svg", "/static/app-icon-64.png", "/static/app-icon-64-dark.png", "/static/icon-180.png", "/static/icon-192.png", "/static/icon-512.png", "/static/icon-maskable-192.png", "/static/icon-maskable-512.png"]);
    const sizes = { "/static/favicon-16.png": 16, "/static/favicon-32.png": 32, "/static/app-icon-64.png": 64, "/static/app-icon-64-dark.png": 64, "/static/icon-180.png": 180, "/static/icon-192.png": 192, "/static/icon-512.png": 512, "/static/icon-maskable-192.png": 192, "/static/icon-maskable-512.png": 512 };
    check(config, "S125-1: SVG favicon, 16/32 px PNG favicons, /favicon.ico, app icons and home-screen icons decode at their sizes",
      Object.entries(sizes).every(([u, s]) => assets[u]?.[0] === s && assets[u]?.[1] === s) && ["/static/favicon.svg", "/favicon.ico", "/static/app-icon.svg", "/static/app-icon-dark.svg"].every((u) => Array.isArray(assets[u])), JSON.stringify(assets));
    await shot(page, `${config}-landing`);
    await shot(page, `${config}-header-mark`, { clip: { x: 0, y: 0, width: Math.min(c.viewport.width, 480), height: 64 } });
    // An explicit theme from Settings wins over the system: the mark follows.
    await page.evaluate((t) => document.documentElement.setAttribute("data-theme", t), other);
    const flipped = await readMark(page);
    check(config, `S125-1: with the ${other} theme chosen in Settings the mark shows its ${other} fills`, flipped.shown.join() === "mark-" + other && flipped.top && flipped.top[4] === TOP[other], JSON.stringify({ shown: flipped.shown, top: flipped.top && flipped.top[4] }));
    await page.evaluate(() => document.documentElement.removeAttribute("data-theme"));
    await pageHealth(page, config, "landing");

    // ---- S130-1: the middle verdict reads "Almost" ------------------------
    await page.goto(base + "/signin");
    await page.fill("#email", "phaedrus@example.com");
    await page.fill("#password", "know-thyself");
    await Promise.all([page.waitForNavigation(), page.click("button.btn-accent")]);
    await page.goto(base + "/study");
    await page.waitForSelector("[data-asking]", { timeout: 10000 });
    const want = [["close", "Almost"], ["right", "Correct"], ["missed", "Incorrect"]];
    let closeItem = "";
    for (let i = 0; i < want.length; i++) {
      const [verdict, word] = want[i];
      await page.click("#answer");
      await page.keyboard.type(i === 0 ? "roughly what the card says" : "an answer", { delay: 5 });
      await page.keyboard.press("Enter");
      await page.waitForSelector("[data-judged] [data-verdict-label]", { timeout: 15000 });
      await settle(page);
      const rv = await page.evaluate(() => ({
        verdict: document.querySelector("[data-judged] [data-verdict]")?.getAttribute("data-verdict") || document.querySelector("[data-verdict]")?.getAttribute("data-verdict"),
        label: document.querySelector("[data-verdict-label]")?.textContent.trim(),
        trail: document.querySelector("[data-trail]")?.getAttribute("aria-label") || "",
        item: document.querySelector("[data-edit-form] input[name=item_id]")?.value || "",
        text: document.body.innerText,
        html: document.documentElement.outerHTML,
      }));
      check(config, `S130-1: a ${verdict} answer stops on its review with the badge "${word}"`, rv.verdict === verdict && rv.label === word, JSON.stringify({ verdict: rv.verdict, label: rv.label }));
      check(config, `S130-1: ${verdict} review never says "Almost Correct"`, !/almost correct/i.test(rv.html));
      if (verdict === "close") {
        closeItem = rv.item;
        check(config, "S130-1: the review trail names the verdict \"Almost\"", /oldest first: .*\bAlmost$/.test(rv.trail) && !/Almost Correct/.test(rv.trail), rv.trail);
        await page.screenshot({ path: path.join(MEDIA, `${config}-review-almost.png`) });
        const badge = await page.$("[data-judged] .verdict");
        if (badge) await badge.screenshot({ path: path.join(MEDIA, `${config}-review-almost-badge.png`) }).catch(() => {});
        await pageHealth(page, config, "review (Almost)");
      }
      await Promise.all([page.waitForSelector("[data-asking], [data-session-end], .session-end", { timeout: 10000 }), page.click("[data-next]")]);
      await settle(page);
      if (verdict === "close") {
        const tags = await page.evaluate(() => [...document.querySelectorAll(".scrollback .tag")].map((t) => [t.className, t.textContent.trim()]));
        check(config, "S130-1: once the next card waits the scrollback tag reads \"almost\"", tags.length >= 1 && tags[tags.length - 1][0].includes("tag-close") && tags[tags.length - 1][1] === "almost", JSON.stringify(tags));
        await shot(page, `${config}-scrollback-almost`);
        await pageHealth(page, config, "waiting card with scrollback");
      }
    }
    const end = await page.evaluate(() => [...document.querySelectorAll(".scrollback .tag")].map((t) => t.textContent.trim()));
    check(config, "S130-1: the scrollback reads almost, correct, incorrect", end.join(",") === "almost,correct,incorrect", end.join(","));
    await shot(page, `${config}-scrollback-all-three`);

    // The card's own page lists the answer under "almost".
    await page.goto(base + "/library/items/" + closeItem);
    const hist = await page.evaluate(() => ({ tags: [...document.querySelectorAll(".history-row .tag")].map((t) => [t.className, t.textContent.trim()]), html: document.documentElement.outerHTML }));
    check(config, "S130-1: the card's page lists the answer as \"almost\"", closeItem && hist.tags.some(([cls, t]) => cls.includes("tag-close") && t === "almost") && !/almost correct/i.test(hist.html), JSON.stringify(hist.tags));
    await shot(page, `${config}-card-history-almost`, { fullPage: !c.phone });
    await pageHealth(page, config, "card page");

    check(config, "0 console errors, page errors, failed requests and responses >= 400", problems.length === 0, problems.slice(0, 5).join(" | "));
    const video = page.video();
    await ctx.close();
    ctx = null;
    if (video) fs.copyFileSync(await video.path(), path.join(MEDIA, `${config}-walkthrough.webm`));
  } catch (e) {
    check(config, "journey ran to the end", false, e.stack || e.message);
  } finally {
    if (ctx) await ctx.close().catch(() => {});
    srv.proc.kill();
  }
}

const browser = await chromium.launch({ executablePath: CHROME, headless: false, args: ["--no-first-run", "--disable-extensions"] });
console.log("browser", browser.version(), "headless: false, DISPLAY", process.env.DISPLAY);
for (let i = 0; i < CONFIGS.length; i++) await journey(browser, CONFIGS[i], PORT0 + i);
await browser.close();
fs.writeFileSync(path.join(MEDIA, "verify-slice130.json"), JSON.stringify({ sha: SHA, browser: "Google Chrome (headed)", passed: results.filter((r) => r.ok).length, failed, results }, null, 2));
console.log(`\n${results.length - failed}/${results.length} checks passed`);
process.exit(failed ? 1 : 0);
