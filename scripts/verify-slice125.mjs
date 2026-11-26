#!/usr/bin/env node
// verify-slice125.mjs: check Slice 125 ([S125-1], with [S113-3]) on the real
// running path, in a real headed Chrome: the exact L3-S3 brand mark, drawn
// from DESIGN.md's canonical SVG, on every surface.
//
//   make build BUILD_SHA=$(git rev-parse HEAD)
//   DISPLAY=:0 node scripts/verify-slice125.mjs --bin ./bin/scry --sha <sha> \
//        --media ~/chief-evidence/media [--port 18125] [--only desktop-light,...]
//
// For each of desktop 1440x900 (DPR 1) and phone 390x844 (touch, DPR 2),
// light and dark, it starts its own Scry (the --bin binary) in SCRY_MODE=test
// with the demo learner seeded on a fresh SQLite file in a new temporary
// directory and a minimal environment (no provider keys; nothing leaves the
// machine), records a video, and walks a real visitor's path:
//
//   landing (signed out, 56 px header) -> Sign in -> Study -> Library ->
//   Create -> Billing -> Settings, where choosing the other mode switches
//   the mark's fills at once, and back.
//
// On every page: the header mark is DESIGN.md's canonical SVG, verbatim
// (read from DESIGN.md itself, not copied here), 24 px square, 8 px before
// `scry` and centred on it, exactly once; its computed fills are the
// active mode's fixed pair; nothing in it is clipped; a screenshot of it,
// decoded here, shows three solid layered slabs, a cobalt top card tilted
// clockwise and its lighter prompt bar. Every page is checked for sideways
// scroll, loaded styles, decoded images and CSP violations; console errors,
// page errors, failed requests and responses >= 400 must be 0.
//
// The icons are decoded in the page and read back pixel by pixel: the
// adaptive SVG favicon in this mode's fills, the hinted 16/32 px PNGs, the
// 64 px light and dark app icons and the 180/192/512 px home-screen icons
// at their sizes with all three cards inside the maskable safe zone. The
// browser tab itself is captured from the X display. Exit 0 only when every
// check passes.

import { chromium } from "playwright-core";
import { spawn, execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import zlib from "node:zlib";

const args = Object.fromEntries(process.argv.slice(2).reduce((a, v, i, all) => {
  if (v.startsWith("--")) a.push([v.slice(2), all[i + 1] && !all[i + 1].startsWith("--") ? all[i + 1] : "1"]);
  return a;
}, []));
const BIN = path.resolve(args.bin || "bin/scry");
const MEDIA = path.resolve(args.media || path.join(os.homedir(), "chief-evidence", "media"));
const PORT0 = Number(args.port || 18125);
const SHA = args.sha || "";
const ONLY = args.only ? args.only.split(",") : null;
const ROOT = path.resolve(path.dirname(new URL(import.meta.url).pathname), "..");
const CHROME = process.env.CHROME_BIN || "/usr/bin/google-chrome";

// ---- DESIGN.md's canonical SVG, read from the contract itself -------------

const design = fs.readFileSync(path.join(ROOT, "DESIGN.md"), "utf8");
const section = design.slice(design.indexOf("\n## Brand mark and wordmark"));
const blocks = [...section.slice(0, section.indexOf("\n## ", 5)).matchAll(/```svg\n([\s\S]*?)```/g)].map((m) => m[1]);
if (blocks.length !== 2) throw new Error(`DESIGN.md brand section has ${blocks.length} svg blocks`);
const parseMark = (svg) => {
  const rects = [...svg.matchAll(/<rect ([^>]*)\/>/g)].map((m) => Object.fromEntries([...m[1].matchAll(/([a-z]+)="([^"]*)"/g)].map((a) => [a[1], a[2]])));
  return { rects, rotate: svg.match(/<g transform="([^"]+)">/)[1] };
};
const CANON = { light: parseMark(blocks[0]), dark: parseMark(blocks[1]) };
const LAYERS = ["back", "middle", "top", "bar"];
const hex = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16));
const rgb = (h) => `rgb(${hex(h).join(", ")})`;
const FILLS = Object.fromEntries(Object.entries(CANON).map(([m, c]) => [m, Object.fromEntries(LAYERS.map((l, i) => [l, c.rects[i].fill]))]));

fs.mkdirSync(MEDIA, { recursive: true });
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "scry-s125-"));
const results = [];
let failed = 0;
function check(config, name, ok, detail = "") {
  results.push({ config, name, ok: !!ok, detail: typeof detail === "string" ? detail : JSON.stringify(detail) });
  if (!ok) failed++;
  console.log(`${ok ? "PASS" : "FAIL"} [${config}] ${name}${detail !== "" ? " — " + (typeof detail === "string" ? detail : JSON.stringify(detail)) : ""}`);
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ---- a small PNG reader (8-bit RGB/RGBA, non-interlaced: Chrome's own) ----

function readPNG(buf) {
  let p = 8, w = 0, h = 0, type = 0;
  const idat = [];
  while (p < buf.length) {
    const len = buf.readUInt32BE(p), kind = buf.toString("ascii", p + 4, p + 8), data = buf.subarray(p + 8, p + 8 + len);
    if (kind === "IHDR") { w = data.readUInt32BE(0); h = data.readUInt32BE(4); type = data[9]; if (data[8] !== 8 || data[12] !== 0) throw new Error("unsupported PNG"); }
    if (kind === "IDAT") idat.push(data);
    p += 12 + len;
  }
  const bpp = type === 6 ? 4 : type === 2 ? 3 : 0;
  if (!bpp) throw new Error("unsupported PNG colour type " + type);
  const raw = zlib.inflateSync(Buffer.concat(idat)), stride = w * bpp, out = Buffer.alloc(w * h * 4);
  let prev = Buffer.alloc(stride);
  for (let y = 0; y < h; y++) {
    const f = raw[y * (stride + 1)], line = Buffer.from(raw.subarray(y * (stride + 1) + 1, (y + 1) * (stride + 1)));
    for (let x = 0; x < stride; x++) {
      const a = x >= bpp ? line[x - bpp] : 0, b = prev[x], c = x >= bpp ? prev[x - bpp] : 0;
      const pa = Math.abs(b - c), pb = Math.abs(a - c), pc = Math.abs(a + b - 2 * c);
      line[x] = (line[x] + [0, a, b, (a + b) >> 1, pa <= pb && pa <= pc ? a : pb <= pc ? b : c][f]) & 255;
    }
    for (let x = 0; x < w; x++) for (let k = 0; k < 4; k++) out[(y * w + x) * 4 + k] = k < bpp ? line[x * bpp + k] : 255;
    prev = line;
  }
  return { w, h, px: out };
}

// Read the brand mark back from pixels: how many pixels are each layer's
// fill (within a small tolerance), how many are the lighter prompt bar on
// the cobalt card, and whether the cobalt card's left edge leans (a
// clockwise tilt moves it right towards the top).
function analyseMark({ w, h, px }, fills, page) {
  const near = (i, c, tol = 10) => Math.abs(px[i] - c[0]) <= tol && Math.abs(px[i + 1] - c[1]) <= tol && Math.abs(px[i + 2] - c[2]) <= tol;
  const count = Object.fromEntries(LAYERS.map((l) => [l, 0]));
  const top = hex(fills.top), barMix = hex(fills.bar).map((v, k) => Math.round(0.85 * v + 0.15 * top[k]));
  const leftTop = [];
  let edge = 0;
  for (let y = 0; y < h; y++) {
    let left = -1;
    for (let x = 0; x < w; x++) {
      const i = (y * w + x) * 4;
      for (const l of ["back", "middle", "top"]) if (near(i, hex(fills[l]))) count[l]++;
      if (near(i, barMix, 24)) count.bar++;
      if (left < 0 && near(i, top)) left = x;
      if ((x === 0 || y === 0 || x === w - 1 || y === h - 1) && !near(i, hex(page), 3)) edge++;
    }
    leftTop.push(left);
  }
  const rows = leftTop.map((x, y) => [y, x]).filter(([, x]) => x >= 0);
  const upper = rows.slice(Math.floor(rows.length * 0.15), Math.floor(rows.length * 0.3)).map(([, x]) => x);
  const lower = rows.slice(Math.floor(rows.length * 0.7), Math.floor(rows.length * 0.85)).map(([, x]) => x);
  const avg = (a) => a.reduce((s, v) => s + v, 0) / Math.max(1, a.length);
  return { count, lean: avg(upper) - avg(lower), edge, size: [w, h] };
}

// ---- the server ---------------------------------------------------------

function startServer(config, port) {
  const dir = fs.mkdtempSync(path.join(tmp, config + "-"));
  const logPath = path.join(dir, "server.log");
  const log = fs.createWriteStream(logPath);
  const proc = spawn(BIN, [], {
    env: { PATH: process.env.PATH, HOME: dir, SCRY_MODE: "test", SCRY_SEED_DEMO: "1", SCRY_DB: path.join(dir, "s125.db"), PORT: String(port) },
    stdio: ["ignore", "pipe", "pipe"],
  });
  proc.stdout.pipe(log);
  proc.stderr.pipe(log);
  return { proc, logPath, base: `http://127.0.0.1:${port}` };
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
  const ok200 = new Set();
  page.on("console", (m) => { if (m.type() === "error") problems.push("console: " + m.text()); });
  page.on("pageerror", (e) => problems.push("pageerror: " + e.message));
  page.on("requestfailed", (r) => {
    const f = r.failure()?.errorText || "";
    // Chrome cancels a favicon fetch still in flight when the page navigates.
    if (f.includes("ERR_ABORTED") && /\/static\/favicon|\/favicon\.ico/.test(r.url())) return;
    // Settings' mode save is a fetch whose redirected page body the script
    // never reads; once its 200 has arrived, our next navigation cancels
    // that unread body. The save itself succeeded (checked separately).
    if (f.includes("ERR_ABORTED") && r.url().includes("/settings?ok=theme") && ok200.has(r.url())) return;
    problems.push(`requestfailed: ${r.url()} ${f}`);
  });
  page.on("response", (r) => { if (r.status() === 200) ok200.add(r.url()); if (r.status() >= 400) problems.push(`http ${r.status()}: ${r.request().resourceType()} ${r.url()}`); });
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

// The header mark, read from the live DOM and from its own pixels.
async function headerMark(page, config, label, mode, opts = {}) {
  const m = await page.evaluate(() => {
    const svgs = document.querySelectorAll("svg.mark");
    const s = document.querySelector("header .wordmark > svg.mark"), t = document.querySelector("header .wordmark-text");
    if (!s || !t) return { missing: true, count: svgs.length };
    const sb = s.getBoundingClientRect(), tb = t.getBoundingClientRect();
    const brand = document.querySelector("header .brand").getBoundingClientRect(), top = document.querySelector("header.top").getBoundingClientRect();
    const rects = [...s.querySelectorAll("rect")].map((r) => ({
      cls: r.getAttribute("class"), x: r.getAttribute("x"), y: r.getAttribute("y"), width: r.getAttribute("width"), height: r.getAttribute("height"),
      rx: r.getAttribute("rx"), opacity: r.getAttribute("opacity"), attrFill: r.getAttribute("fill"), fill: getComputedStyle(r).fill,
      stroke: getComputedStyle(r).stroke, inRotated: r.parentElement.tagName === "g" ? r.parentElement.getAttribute("transform") : null,
      box: (() => { const b = r.getBoundingClientRect(); return [b.left - sb.left, b.top - sb.top, b.right - sb.left, b.bottom - sb.top]; })(),
    }));
    const anim = s.getAnimations ? s.getAnimations({ subtree: true }).length : 0;
    return {
      count: svgs.length, viewBox: s.getAttribute("viewBox"), w: sb.width, h: sb.height, gap: tb.left - sb.right,
      dy: (sb.top + sb.height / 2) - (tb.top + tb.height / 2), brandH: brand.height, headerH: top.height, rects, anim,
      inHeader: !!s.closest("header"), page: getComputedStyle(document.body).backgroundColor,
    };
  });
  if (m.missing) { check(config, `${label}: header mark beside scry`, false, m); return; }
  const canon = CANON.light;
  const geometry = m.rects.length === 4 && canon.rects.every((c, i) => ["x", "y", "width", "height", "rx"].every((k) => m.rects[i][k] === c[k]))
    && m.rects[3].opacity === canon.rects[3].opacity && m.rects[0].inRotated === null && m.rects[1].inRotated === null
    && m.rects[2].inRotated === CANON.light.rotate && m.rects[3].inRotated === CANON.light.rotate && m.viewBox === "0 0 32 32";
  check(config, `${label}: header mark is DESIGN.md's canonical SVG (32 grid, 3 portrait 16x19 cards rx 2.5, top card + bar in rotate(10 19 14), bar opacity 0.85)`, geometry,
    m.rects.map((r) => `${r.cls}:${r.x},${r.y},${r.width}x${r.height}${r.inRotated ? " " + r.inRotated : ""}`).join(" "));
  check(config, `${label}: exactly one mark, in the header beside scry`, m.count === 1 && m.inHeader, `count=${m.count}`);
  check(config, `${label}: 24 px square, 8 px before scry, centred on it`, m.w === 24 && m.h === 24 && Math.abs(m.gap - 8) < 0.6 && Math.abs(m.dy) <= 1, { w: m.w, h: m.h, gap: m.gap, dy: m.dy });
  if (opts.rowH) check(config, `${label}: ${opts.rowH} px ${opts.rowLabel}`, Math.round(opts.rowLabel === "signed-out header" ? m.headerH : m.brandH) === opts.rowH, { header: m.headerH, brand: m.brandH });
  const want = FILLS[mode];
  const fills = m.rects.map((r) => r.fill), wantFills = LAYERS.map((l) => rgb(want[l]));
  check(config, `${label}: ${mode} fills by the active mode (${LAYERS.map((l) => want[l]).join(" ")}), solid, never stroked`,
    fills.every((f, i) => f === wantFills[i]) && m.rects.every((r) => r.stroke === "none"), fills.join(" "));
  const inside = m.rects.every((r) => r.box[0] >= -0.01 && r.box[1] >= -0.01 && r.box[2] <= 24.01 && r.box[3] <= 24.01);
  check(config, `${label}: nothing in the mark is clipped (every layer inside its 24 px box)`, inside, m.rects.map((r) => r.box.map((v) => v.toFixed(1)).join(",")).join(" | "));
  check(config, `${label}: the mark does not animate`, m.anim === 0, m.anim);
  // Pixels: a screenshot of the mark itself.
  const shot = await page.locator("header .wordmark > svg.mark").screenshot();
  const pageHex = "#" + m.page.match(/\d+/g).slice(0, 3).map((v) => (+v).toString(16).padStart(2, "0")).join("");
  const a = analyseMark(readPNG(shot), want, pageHex);
  const scale = a.size[0] / 24;
  check(config, `${label}: pixels show three solid layered slabs, the cobalt top card tilted clockwise and its lighter prompt bar`,
    a.count.top >= 90 * scale * scale && a.count.middle >= 12 * scale * scale && a.count.back >= 3 * scale * scale && a.count.bar >= 1 && a.lean >= 0.5 * scale && a.edge === 0,
    a);
  return a;
}

// Every icon, decoded in the page at its own size and read back pixel by
// pixel (same origin, so the canvas is readable).
async function icons(page, config, mode) {
  const out = await page.evaluate(async ({ fills }) => {
    const hex = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16));
    const near = (d, i, c, tol) => d[i + 3] > 250 && Math.abs(d[i] - c[0]) <= tol && Math.abs(d[i + 1] - c[1]) <= tol && Math.abs(d[i + 2] - c[2]) <= tol;
    const read = async (url, size) => {
      const img = new Image();
      img.src = url;
      try { await img.decode(); } catch { return { url, error: "undecodable" }; }
      const n = size || img.naturalWidth;
      const c = document.createElement("canvas");
      c.width = c.height = n;
      const g = c.getContext("2d");
      g.drawImage(img, 0, 0, n, n);
      const d = g.getImageData(0, 0, n, n).data;
      return { url, natural: [img.naturalWidth, img.naturalHeight], n, d };
    };
    const res = {};
    const analyse = (r, pal, tol) => {
      if (r.error) return r;
      const { d, n } = r, count = { back: 0, middle: 0, top: 0 };
      let outside = 0, edge = 0;
      for (let y = 0; y < n; y++) for (let x = 0; x < n; x++) {
        const i = (y * n + x) * 4;
        let mark = false;
        for (const l of ["back", "middle", "top"]) if (near(d, i, hex(pal[l]), tol)) { count[l]++; mark = true; }
        if (mark && Math.hypot(x + 0.5 - n / 2, y + 0.5 - n / 2) > 0.4 * n) outside++;
        if ((x === 0 || y === 0 || x === n - 1 || y === n - 1) && d[i + 3] > 0) edge++;
      }
      return { url: r.url, natural: r.natural, count, outside, edge };
    };
    const L = fills.light, D = fills.dark;
    // The adaptive SVG favicon renders this page's mode.
    const mode = matchMedia("(prefers-color-scheme: dark)").matches ? D : L;
    res.faviconSvg = analyse(await read("/static/favicon.svg", 64), mode, 4);
    res.fav16 = analyse(await read("/static/favicon-16.png"), L, 2);
    res.fav32 = analyse(await read("/static/favicon-32.png"), L, 2);
    for (const [k, u, pal] of [["app64", "/static/app-icon-64.png", L], ["app64dark", "/static/app-icon-64-dark.png", D], ["apple180", "/static/icon-180.png", D],
      ["icon192", "/static/icon-192.png", D], ["icon512", "/static/icon-512.png", D], ["mask192", "/static/icon-maskable-192.png", D], ["mask512", "/static/icon-maskable-512.png", D]]) {
      const a = analyse(await read(u), pal, 3);
      a.edge = undefined;
      res[k] = a;
    }
    const man = await (await fetch("/static/manifest.webmanifest")).json();
    res.manifest = man.icons.map((i) => `${i.purpose} ${i.sizes} ${i.src}`);
    res.links = [...document.querySelectorAll("link[rel~=icon], link[rel=apple-touch-icon], link[rel=manifest], meta[property='og:image']")].map((l) => l.getAttribute("href") || l.getAttribute("content"));
    return res;
  }, { fills: FILLS });
  const all3 = (a) => a && a.count && a.count.back > 0 && a.count.middle > 0 && a.count.top > 0;
  check(config, `favicon.svg renders the ${mode} fill set (prefers-color-scheme), all three cards, nothing clipped`, all3(out.faviconSvg) && out.faviconSvg.edge === 0, out.faviconSvg);
  for (const [k, n] of [["fav16", 16], ["fav32", 32]]) {
    check(config, `${n} px PNG favicon decodes at ${n} px with three solid slabs, nothing clipped`, out[k].natural?.[0] === n && all3(out[k]) && out[k].edge === 0, out[k]);
  }
  for (const [k, n] of [["app64", 64], ["app64dark", 64], ["apple180", 180], ["icon192", 192], ["icon512", 512], ["mask192", 192], ["mask512", 512]]) {
    check(config, `${out[k].url} decodes at ${n} px, all three cards in its mode's fills, every mark pixel inside the 80% safe zone`, out[k].natural?.[0] === n && all3(out[k]) && out[k].outside === 0, out[k]);
  }
  check(config, "head links favicon.ico, favicon.svg, apple-touch 180, manifest and the dark social preview icon",
    ["/favicon.ico", "/static/favicon.svg", "/static/icon-180.png", "/static/manifest.webmanifest"].every((u) => out.links.includes(u)) && out.links.some((u) => u.endsWith("/static/icon-maskable-512.png")), out.links.join(" "));
  check(config, "manifest: dark 192/512 any + maskable icons", ["any 192x192 /static/icon-192.png", "any 512x512 /static/icon-512.png", "maskable 192x192 /static/icon-maskable-192.png", "maskable 512x512 /static/icon-maskable-512.png"].every((s) => out.manifest.includes(s)), out.manifest.join("; "));
}

async function signIn(page, base) {
  await page.goto(base + "/signin");
  await page.fill("#email", "phaedrus@example.com");
  await page.fill("#password", "know-thyself");
  await Promise.all([page.waitForNavigation(), page.click("button.btn-accent")]);
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
  const config = c.name, mode = c.colorScheme;
  const other = mode === "dark" ? "light" : "dark";
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

    // Landing, signed out.
    await page.goto(base + "/");
    await page.mouse.move(1, 1).catch(() => {});
    await sleep(600);
    await headerMark(page, config, "landing", mode, c.phone ? { rowH: 56, rowLabel: "signed-out header" } : {});
    await icons(page, config, mode);
    await page.screenshot({ path: path.join(MEDIA, `s125-${config}-landing.png`) });
    await page.locator("header.top").screenshot({ path: path.join(MEDIA, `s125-${config}-header.png`) });
    if (!c.phone && grabTab(path.join(MEDIA, `s125-${config}-browser-tab.png`), c.viewport.width)) {
      check(config, "browser tab captured from the X display", fs.existsSync(path.join(MEDIA, `s125-${config}-browser-tab.png`)));
    }
    await pageHealth(page, config, "landing");

    // Sign in (a real form, typed), then every destination.
    await page.click("header a[href='/signin']");
    await page.waitForURL(/\/signin/);
    await headerMark(page, config, "sign in", mode);
    await page.fill("#email", "phaedrus@example.com");
    await page.fill("#password", "know-thyself");
    await Promise.all([page.waitForNavigation(), page.click("button.btn-accent")]);
    for (const [href, label] of [["/study", "Study"], ["/library", "Library"], ["/learn", "Create"], ["/billing", "Billing"], ["/settings", "Settings"]]) {
      await Promise.all([page.waitForURL((u) => u.pathname.startsWith(href)), page.click(`header nav a[href='${href}']`)]);
      await sleep(350);
      await headerMark(page, config, label, mode, c.phone ? { rowH: 48, rowLabel: "phone brand row" } : {});
      await pageHealth(page, config, label);
      if (href === "/study") await page.locator("header.top").screenshot({ path: path.join(MEDIA, `s125-${config}-header-signed-in.png`) });
    }

    // Settings: choosing the other mode switches the mark's fill pair at
    // once (the data-theme override), and choosing "Follow my system"
    // switches it back.
    await page.locator(`input[name=theme][value=${other}]`).scrollIntoViewIfNeeded();
    // The choice saves by fetch (POST, then its redirect to the page): wait
    // for that to finish so a navigation of ours never cuts it short.
    const saved = (p) => p.waitForResponse((r) => r.url().includes("/settings?ok=theme"), { timeout: 10000 }).catch(() => null);
    let save = saved(page);
    await page.click(`input[name=theme][value=${other}]`);
    await page.waitForFunction((m) => document.documentElement.dataset.theme === m, other, { timeout: 5000 }).catch(() => {});
    await sleep(500);
    await headerMark(page, config, `Settings → ${other}`, other);
    await page.screenshot({ path: path.join(MEDIA, `s125-${config}-settings-${other}.png`) });
    check(config, `Settings → ${other}: the choice saved`, (await save)?.status() === 200);
    await page.reload();
    await headerMark(page, config, `Settings → ${other}, after reload`, other);
    save = saved(page);
    await page.click("input[name=theme][value=system]");
    await page.waitForFunction(() => !document.documentElement.dataset.theme, null, { timeout: 5000 }).catch(() => {});
    await sleep(500);
    await headerMark(page, config, "Settings → Follow my system", mode);
    check(config, "Settings → Follow my system: the choice saved", (await save)?.status() === 200);
    await pageHealth(page, config, "Settings after mode switches");
    await Promise.all([page.waitForURL((u) => u.pathname === "/study"), page.click("header nav a[href='/study']")]);
    await sleep(400);

    check(config, "0 console errors, page errors, failed requests or responses >= 400", problems.length === 0, problems.slice(0, 8).join(" | "));
    const video = page.video();
    await ctx.close();
    if (video) {
      const webm = path.join(MEDIA, `s125-${config}-walkthrough.webm`);
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

// A board of every icon at its real size and magnified, for the owner to
// see at a glance (a static page, not a check).
async function board(port) {
  const srv = startServer("board", port);
  const browser = await chromium.launch({ headless: false, executablePath: CHROME });
  try {
    await waitHealthy(srv.base);
    for (const mode of ["light", "dark"]) {
      const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: mode });
      const page = await ctx.newPage();
      const b = srv.base, bg = mode === "dark" ? "#12101A" : "#FFFFFF", fg = mode === "dark" ? "#fff" : "#0B1015";
      const cell = (label, inner) => `<figure><div class="art">${inner}</div><figcaption>${label}</figcaption></figure>`;
      const img = (u, s, px) => `<img src="${b}${u}" width="${s}" height="${s}" style="${px ? "image-rendering:pixelated" : ""}">`;
      await page.setContent(`<!doctype html><html><head><style>
        body{margin:24px;background:${bg};color:${fg};font:13px ui-monospace,monospace}
        h1{font:600 18px ui-sans-serif,system-ui;margin:0 0 16px} section{display:flex;flex-wrap:wrap;gap:20px;align-items:flex-end;margin-bottom:22px}
        figure{margin:0;display:flex;flex-direction:column;align-items:center;gap:6px} .art{display:flex;align-items:center;justify-content:center;min-height:40px}
        .grey{background:#8A8A90;padding:6px}
      </style></head><body>
        <h1>Scry brand mark L3-S3 · ${mode} mode (prefers-color-scheme: ${mode}) · served by the running app</h1>
        <section>
          ${cell("favicon.svg 16", img("/static/favicon.svg", 16))}${cell("favicon.svg 32", img("/static/favicon.svg", 32))}
          ${cell("favicon.svg ×8", img("/static/favicon.svg", 256))}
          ${cell("favicon-16.png 1x", img("/static/favicon-16.png", 16))}${cell("favicon-16.png ×8", img("/static/favicon-16.png", 128, 1))}
          ${cell("favicon-32.png 1x", img("/static/favicon-32.png", 32))}${cell("favicon-32.png ×8", img("/static/favicon-32.png", 256, 1))}
        </section>
        <section>
          ${cell("app-icon-64.png", `<div class="grey">${img("/static/app-icon-64.png", 64)}</div>`)}${cell("app-icon-64.png ×4", `<div class="grey">${img("/static/app-icon-64.png", 256, 1)}</div>`)}
          ${cell("app-icon-64-dark.png", `<div class="grey">${img("/static/app-icon-64-dark.png", 64)}</div>`)}${cell("app-icon-64-dark.png ×4", `<div class="grey">${img("/static/app-icon-64-dark.png", 256, 1)}</div>`)}
          ${cell("icon-180.png (apple-touch)", img("/static/icon-180.png", 180))}${cell("icon-192.png", `<div class="grey">${img("/static/icon-192.png", 192)}</div>`)}
          ${cell("icon-maskable-512.png (og:image) at 192", img("/static/icon-maskable-512.png", 192))}
        </section></body></html>`);
      await page.waitForFunction(() => [...document.images].every((i) => i.complete && i.naturalWidth > 0));
      await page.screenshot({ path: path.join(MEDIA, `s125-icon-board-${mode}.png`) });
      await ctx.close();
    }
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
if (!ONLY) await board(port++);
fs.writeFileSync(path.join(MEDIA, "s125-results.json"), JSON.stringify({ sha: SHA, checks: results.length, failed, results }, null, 2));
console.log(`\n${results.length - failed}/${results.length} checks PASS`);
process.exit(failed ? 1 : 0);
