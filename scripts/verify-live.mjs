#!/usr/bin/env node
// verify-live.mjs: check a running Scry from the outside, in a real headless
// Chromium, the way a learner's browser sees it.
//
//   node scripts/verify-live.mjs --url https://scry.mistystep.io
//   node scripts/verify-live.mjs --url http://127.0.0.1:18080 --local-signup \
//        --screenshot /tmp/card.png --simulate-violation
//
// Checks /healthz, the signed-out /signup/card redirect and its
// Content-Security-Policy, then loads /, /signin and /signup in a cookie-free
// browser context, asserting zero CSP violations, console errors, page errors
// and failed requests. Signed in (--session-cookie and/or --local-signup,
// each in its own separate context) it checks /signup/card, or /billing/card
// when the account already has a card-backed plan and /signup/card redirects
// to /study or /billing; on a no-card trial both card pages. When Stripe
// Elements is on a card page it waits for the js.stripe.com iframe and checks
// it is visible and sized. It also checks /billing and /study signed in
// (/study answers 200 itself, even for an empty library). --local-signup
// checks that a new sign-up lands straight in Scry (/shelf or /study, no
// card step) with the no-card trial's status line in the chrome, that the
// navigation reads Study, Library, Inbox, Create, Billing, Settings with an
// icon each, and that Study with the new, empty library is its own screen
// with its two ways forward (never a silent redirect). Prints one JSON summary on
// stdout; exits 0 only when every check passes.
//
// Flags (env in brackets):
//   --url <base>                 [SCRY_VERIFY_URL] required
//   --screenshot <file.png>      [SCRY_VERIFY_SCREENSHOT] card-page screenshot (with
//                                both signed-in modes the second gets a -<mode> suffix)
//   --screenshot-dir <dir>       [SCRY_VERIFY_SCREENSHOT_DIR] desktop (1280x800) and mobile
//                                (390x844) full-page PNGs of /, the signed-in landing page,
//                                /billing and the card page(s), named <page>-<viewport>.png
//   --session-cookie name=value  [SCRY_VERIFY_SESSION_COOKIE] a signed-in session
//                                (prefer the env var; the value is never printed)
//   --local-signup               sign up a fresh local-auth learner (refused when
//                                /healthz says mode=live or auth is Clerk)
//   --expect-stripe              require Stripe even if /healthz doesn't say so
//   --expect-clerk               require Clerk even if /healthz doesn't say so
//   --simulate-violation         trigger one img-src violation on / and check it
//                                is POSTed to /api/client-error (this pages
//                                Sentry on a live deploy: use deliberately)
//   --timeout <ms>               per-step timeout (default 30000)
// Chrome: CHROME_BIN, else /usr/bin/google-chrome, google-chrome-stable,
// chromium or chromium-browser. Nothing is downloaded.

import fs from "node:fs";
import path from "node:path";
import { chromium } from "playwright-core";

const usage = "usage: verify-live.mjs --url <base-url> [--screenshot file.png] [--screenshot-dir dir] [--session-cookie name=value] " +
  "[--local-signup] [--expect-stripe] [--expect-clerk] [--simulate-violation] [--timeout ms]";

function parseArgs(argv) {
  const o = {
    url: process.env.SCRY_VERIFY_URL || "",
    screenshot: process.env.SCRY_VERIFY_SCREENSHOT || "",
    screenshotDir: process.env.SCRY_VERIFY_SCREENSHOT_DIR || "",
    sessionCookie: process.env.SCRY_VERIFY_SESSION_COOKIE || "",
    localSignup: false, expectStripe: false, expectClerk: false, simulateViolation: false, timeout: 30000,
  };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    const [flag, inline] = a.includes("=") && a.startsWith("--") ? [a.slice(0, a.indexOf("=")), a.slice(a.indexOf("=") + 1)] : [a, undefined];
    const value = () => {
      if (inline !== undefined) return inline;
      if (i + 1 >= argv.length) throw new Error(flag + " needs a value");
      return argv[++i];
    };
    switch (flag) {
      case "--url": o.url = value(); break;
      case "--screenshot": o.screenshot = value(); break;
      case "--screenshot-dir": o.screenshotDir = value(); break;
      case "--session-cookie": o.sessionCookie = value(); break;
      case "--local-signup": o.localSignup = true; break;
      case "--expect-stripe": o.expectStripe = true; break;
      case "--expect-clerk": o.expectClerk = true; break;
      case "--simulate-violation": o.simulateViolation = true; break;
      case "--timeout": o.timeout = Number(value()); break;
      case "-h": case "--help": console.log(usage); process.exit(0);
      default: throw new Error("unknown flag " + a + "\n" + usage);
    }
  }
  if (!o.url) throw new Error("--url is required\n" + usage);
  o.url = o.url.replace(/\/+$/, "");
  new URL(o.url); // throws on a malformed base
  if (!(o.timeout > 0)) throw new Error("--timeout must be a positive number of milliseconds");
  return o;
}

// clean keeps scheme, host and path, so nothing in the summary carries a
// query string, fragment or credential.
function clean(u) {
  try {
    const x = new URL(u);
    return x.protocol === "data:" ? "data:" : x.protocol === "blob:" ? "blob:" : x.origin + x.pathname;
  } catch { return String(u).split(/[?#]/)[0].slice(0, 200); }
}

function parseCSP(header) {
  const out = {};
  for (const part of (header || "").split(";")) {
    const f = part.trim().split(/\s+/).filter(Boolean);
    if (f.length) out[f[0].toLowerCase()] = f.slice(1);
  }
  return out;
}

function chromePath() {
  const candidates = [process.env.CHROME_BIN, "/usr/bin/google-chrome", "/usr/bin/google-chrome-stable", "/usr/bin/chromium", "/usr/bin/chromium-browser"];
  for (const c of candidates) if (c && fs.existsSync(c)) return c;
  throw new Error("no Chrome found: set CHROME_BIN");
}

const summary = { ok: false, url: "", startedAt: new Date().toISOString(), healthz: null, stripe: false, clerk: false, checks: [], pages: [], failures: [] };

function check(name, ok, detail) {
  summary.checks.push(detail === undefined ? { name, ok } : { name, ok, detail });
  if (!ok) summary.failures.push(detail === undefined ? name : name + ": " + (typeof detail === "string" ? detail : JSON.stringify(detail)));
  return ok;
}

async function main() {
  const opt = parseArgs(process.argv.slice(2));
  const base = opt.url;
  summary.url = base;

  // ---- /healthz ----------------------------------------------------------
  let health = {};
  try {
    const res = await fetch(base + "/healthz", { redirect: "manual", signal: AbortSignal.timeout(opt.timeout) });
    const type = res.headers.get("content-type") || "";
    let body = null;
    try { body = await res.json(); } catch { body = null; }
    health = body || {};
    summary.healthz = body && { status: body.status, build_sha: body.build_sha, mode: body.mode, billing_mode: body.billing_mode, auth_mode: body.auth_mode };
    check("healthz 200 JSON status ok", res.status === 200 && type.includes("json") && body && body.status === "ok",
      { status: res.status, contentType: type, bodyStatus: body && body.status });
  } catch (err) {
    check("healthz reachable", false, String(err.message || err));
  }
  summary.stripe = opt.expectStripe || String(health.billing_mode || "").startsWith("stripe-");
  summary.clerk = opt.expectClerk || String(health.auth_mode || "").startsWith("clerk-");

  // ---- the signed-out card step: redirect and policy ----------------------
  const policies = {};
  for (const p of ["/signup/card", "/"]) {
    try {
      const res = await fetch(base + p, { redirect: "manual", signal: AbortSignal.timeout(opt.timeout) });
      policies[p] = res.headers.get("content-security-policy") || "";
      if (p === "/signup/card") {
        const loc = res.headers.get("location") || "";
        const to = loc ? new URL(loc, base + p) : null;
        check("GET /signup/card signed out: 303 to /signin?next=%2Fsignup%2Fcard",
          res.status === 303 && to && to.pathname + to.search === "/signin?next=%2Fsignup%2Fcard", { status: res.status, location: loc && clean(new URL(loc, base).href) + (to ? to.search : "") });
      }
      await res.body?.cancel();
    } catch (err) {
      check("GET " + p, false, String(err.message || err));
    }
  }
  const header = policies["/signup/card"] || "";
  const csp = parseCSP(header);
  summary.csp = header;
  const has = (d, ...srcs) => srcs.every(s => (csp[d] || []).includes(s));
  check("CSP present on /signup/card", header !== "");
  check("CSP base-uri 'none'", has("base-uri", "'none'"), csp["base-uri"]);
  check("CSP frame-ancestors 'none'", has("frame-ancestors", "'none'"), csp["frame-ancestors"]);
  check("CSP report-uri /api/client-error", has("report-uri", "/api/client-error"), csp["report-uri"]);
  if (summary.stripe) {
    check("CSP frame-src allows Stripe (js.stripe.com, hooks.stripe.com)", has("frame-src", "https://js.stripe.com", "https://hooks.stripe.com"), csp["frame-src"]);
    check("CSP script-src allows https://js.stripe.com", has("script-src", "https://js.stripe.com"), csp["script-src"]);
    check("CSP connect-src allows https://api.stripe.com", has("connect-src", "https://api.stripe.com"), csp["connect-src"]);
  }
  if (summary.clerk) {
    check("CSP frame-src allows https://challenges.cloudflare.com", has("frame-src", "https://challenges.cloudflare.com"), csp["frame-src"]);
  }
  if (policies["/"] !== undefined) check("same CSP on / and /signup/card", policies["/"] === header);

  // ---- the browser ---------------------------------------------------------
  const browser = await chromium.launch({ executablePath: chromePath(), headless: true, args: ["--no-sandbox"] });
  try {
    // Public pages are what a signed-out visitor sees, so they are checked in
    // a context that never holds a session: a signed-in session would
    // redirect / and /signup elsewhere.
    const pub = await newContext(browser, opt);
    await signedOutCard(pub, base);
    for (const p of ["/", "/signin", "/signup"]) await visit(pub, base, p, opt, { expectStatus: 200 });

    // Each signed-in mode gets its own context, apart from the public one.
    const modes = [];
    if (opt.sessionCookie) modes.push("session-cookie");
    if (opt.localSignup) modes.push("local-signup");
    for (const mode of modes) {
      const ctx = await newContext(browser, opt);
      try {
        let signedIn;
        if (mode === "session-cookie") {
          const at = opt.sessionCookie.indexOf("=");
          if (at < 1) throw new Error("--session-cookie must be name=value");
          await ctx.addCookies([{ name: opt.sessionCookie.slice(0, at), value: opt.sessionCookie.slice(at + 1), url: base }]);
          summary.session = Object.assign(summary.session || {}, { cookie: opt.sessionCookie.slice(0, at) }); // the value is never printed
          signedIn = true;
        } else {
          signedIn = await localSignup(ctx, base, opt);
        }
        if (signedIn) {
          const cards = await signedInCard(ctx, base, opt, mode, modes.length > 1 && mode !== modes[0]);
          await signedInPages(ctx, base, opt, mode);
          if (mode === "local-signup") await emptyStudy(ctx, base, opt, mode);
          if (opt.screenshotDir) {
            const landed = (summary.session || {}).landedOn;
            const shots = [];
            if (mode === "local-signup" && landed) shots.push({ path: landed, name: "signup-landing" + landed.replace(/\//g, "-") });
            shots.push({ path: "/billing", name: mode + "-billing" });
            for (const c of cards || []) shots.push({ path: c, name: mode + c.replace(/\//g, "-"), card: true });
            await screens(browser, base, opt, shots, await ctx.storageState());
          }
        }
      } finally {
        await ctx.close();
      }
    }
    if (!modes.length) summary.checks.push({ name: "signed-in card pages", ok: true, skipped: "no --session-cookie or --local-signup" });

    if (opt.simulateViolation) await simulateViolation(pub, base, opt);
    await pub.close();
    if (opt.screenshotDir) await screens(browser, base, opt, [{ path: "/", name: "landing" }]);
  } finally {
    await browser.close();
  }
}

async function newContext(browser, opt) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  context.setDefaultTimeout(opt.timeout);
  await context.addInitScript(() => {
    window.__scryCSP = [];
    document.addEventListener("securitypolicyviolation", e => {
      window.__scryCSP.push({ directive: e.effectiveDirective || e.violatedDirective, blocked: e.blockedURI, source: e.sourceFile, line: e.lineNumber });
    }, true);
  });
  return context;
}

// signedOutCard repeats the signed-out /signup/card check from inside the
// public browser context: it must hold no session (303 to sign-in) and the
// page must carry the same policy.
async function signedOutCard(ctx, base) {
  const cookies = await ctx.cookies(base);
  check("public context holds no cookies", cookies.length === 0, cookies.map(c => c.name));
  try {
    const res = await ctx.request.get(base + "/signup/card", { maxRedirects: 0, failOnStatusCode: false });
    const loc = res.headers()["location"] || "";
    const to = loc ? new URL(loc, base + "/signup/card") : null;
    check("public context: GET /signup/card 303 to /signin?next=%2Fsignup%2Fcard",
      res.status() === 303 && !!to && to.pathname + to.search === "/signin?next=%2Fsignup%2Fcard", { status: res.status(), location: to ? to.pathname + to.search : "" });
    check("public context: /signup/card carries the same CSP", (res.headers()["content-security-policy"] || "") === (summary.csp || "") && !!summary.csp);
    await res.dispose();
  } catch (err) {
    check("public context: GET /signup/card", false, String(err.message || err).split("\n")[0]);
  }
}

// signedInCard checks the card field a signed-in learner sees: on
// /signup/card for an account still choosing a plan, or, when the account
// already has one (/signup/card redirects to /study or /billing), on
// /billing/card. Either way the Stripe iframe must mount cleanly. The first
// redirect hop decides, since /study itself may redirect on (to /shelf).
async function signedInCard(ctx, base, opt, mode, altShot) {
  const shot = opt.screenshot && altShot ? opt.screenshot.replace(/(\.png)?$/i, "-" + mode + ".png") : opt.screenshot;
  let targets = ["/signup/card", "/billing/card"], hop = "";
  try {
    const res = await ctx.request.get(base + "/signup/card", { maxRedirects: 0, failOnStatusCode: false });
    const loc = res.headers()["location"] || "";
    hop = loc ? new URL(loc, base + "/signup/card").pathname : "";
    await res.dispose();
    if (res.status() !== 200) {
      if (res.status() >= 300 && res.status() < 400 && (hop === "/study" || hop === "/billing")) {
        targets = ["/billing/card"];
        (summary.session ||= {})[mode] = "account already has a card-backed plan (/signup/card redirects to " + hop + "): checking /billing/card";
      } else {
        check(mode + ": signed in", false, "GET /signup/card answered " + res.status() + (hop ? " to " + hop : "") + " (the session was not accepted)");
        return [];
      }
    } else {
      // A learner still choosing (the no-card trial, live or ended): both
      // card pages take a card.
      (summary.session ||= {})[mode] = "no card on file (/signup/card answers 200): checking /signup/card and /billing/card";
    }
  } catch (err) {
    check(mode + ": GET /signup/card", false, String(err.message || err).split("\n")[0]);
    return [];
  }
  const done = [];
  for (const target of targets) {
    const rec = await visit(ctx, base, target, opt, { card: true, screenshot: target === targets[0] ? shot : "", mode });
    if (!rec) continue;
    done.push(target);
    if (summary.stripe) check(mode + ": Stripe card field shown on " + target, rec.stripe, rec.stripe ? undefined : target + " rendered no #card-element[data-key]");
    (summary.cardPages ||= []).push({ mode, path: target, signupCardRedirect: hop || null, stripeIframe: rec.stripeIframe || null });
  }
  return done;
}

// signedInPages checks the signed-in pages a learner lives on: /billing,
// /study (which stays on /study whatever is in the library), /shelf, /learn
// (Create), /inbox and /settings, each with zero CSP violations, console
// errors, page errors and failed requests.
async function signedInPages(ctx, base, opt, mode) {
  for (const p of ["/billing", "/study", "/shelf", "/learn", "/inbox", "/settings"]) {
    await visit(ctx, base, p, opt, { expectStatus: 200, mode });
  }
}

// emptyStudy checks what a brand-new learner sees: the navigation with its
// six labelled, icon-carrying destinations (Create, not Learn), and Study
// with an empty library as its own screen with both ways forward.
async function emptyStudy(ctx, base, opt, mode) {
  const page = await ctx.newPage();
  try {
    const res = await page.goto(base + "/study", { waitUntil: "load" });
    const seen = await page.evaluate(() => ({
      path: location.pathname,
      empty: !!document.querySelector("[data-study-empty]"),
      head: (document.querySelector("[data-study-empty] h1") || {}).textContent || "",
      ctas: [...document.querySelectorAll("[data-study-empty] .actions a")].map(a => a.getAttribute("href")),
      nav: [...document.querySelectorAll(".nav a")].map(a => ({ label: (a.querySelector("span") || {}).textContent || "", icon: !!a.querySelector("svg[aria-hidden=true]") })),
    }));
    check(mode + ": GET /study with an empty library answers 200 on /study (no redirect)", res.status() === 200 && seen.path === "/study", { status: res.status(), path: seen.path });
    check(mode + ": empty Study says so and offers a starter set and Create", seen.empty && seen.head.trim() === "No cards to study yet." &&
      seen.ctas.join(",") === "/shelf,/learn", { head: seen.head.trim(), ctas: seen.ctas });
    check(mode + ": navigation reads Study, Library, Inbox, Create, Billing, Settings, each with an icon",
      seen.nav.map(n => n.label).join(",") === "Study,Library,Inbox,Create,Billing,Settings" && seen.nav.every(n => n.icon), seen.nav);
  } catch (err) {
    check(mode + ": empty Study", false, String(err.message || err).split("\n")[0]);
  } finally {
    await page.close();
  }
}

// screens captures full-page PNGs at a desktop and a phone viewport, in
// fresh contexts carrying the given storage state (cookies) if any. Card
// pages wait for the Stripe iframe first. Screenshots are evidence only;
// the checks above are the verdict.
const VIEWPORTS = { desktop: { width: 1280, height: 800 }, mobile: { width: 390, height: 844 } };
async function screens(browser, base, opt, shots, storageState) {
  fs.mkdirSync(path.resolve(opt.screenshotDir), { recursive: true });
  for (const [vp, size] of Object.entries(VIEWPORTS)) {
    const ctx = await browser.newContext({ viewport: size, storageState, isMobile: vp === "mobile", hasTouch: vp === "mobile", deviceScaleFactor: vp === "mobile" ? 2 : 1 });
    ctx.setDefaultTimeout(opt.timeout);
    try {
      for (const s of shots) {
        const page = await ctx.newPage();
        try {
          await page.goto(base + s.path, { waitUntil: "load" });
          await page.waitForLoadState("networkidle", { timeout: Math.min(opt.timeout, 15000) }).catch(() => {});
          if (s.card && await page.locator("#card-element[data-key]").count()) {
            await page.locator('#card-element iframe[src*="js.stripe.com"]').first().waitFor({ state: "visible", timeout: Math.max(opt.timeout, 20000) }).catch(() => {});
            await page.waitForTimeout(1500);
          }
          const file = path.join(opt.screenshotDir, s.name.replace(/^-+|[^a-z0-9-]/gi, "") + "-" + vp + ".png");
          await page.screenshot({ path: file, fullPage: true });
          (summary.screenshots ||= []).push(file);
        } catch (err) {
          check("screenshot " + s.path + " (" + vp + ")", false, String(err.message || err).split("\n")[0]);
        } finally {
          await page.close();
        }
      }
    } finally {
      await ctx.close();
    }
  }
}

// visit opens path in a fresh page and records every way it can fail
// silently in a learner's browser.
async function visit(context, base, p, opt, { expectStatus, card, screenshot, mode, allowFinal = [] } = {}) {
  const page = await context.newPage();
  const rec = { path: p, cspViolations: [], consoleErrors: [], pageErrors: [], failedRequests: [], ignoredRequests: [] };
  if (mode) rec.mode = mode;
  const label = (mode ? mode + " " : "") + p;
  const origin = new URL(base).origin;
  page.on("console", m => { if (m.type() === "error") rec.consoleErrors.push({ text: m.text().slice(0, 300).replace(/\b(https?:\/\/[^\s?#"']+)[?#][^\s"']*/g, "$1"), at: clean(m.location().url || "") }); });
  page.on("pageerror", e => rec.pageErrors.push(String(e.message || e).slice(0, 300)));
  page.on("requestfailed", r => {
    const err = r.failure()?.errorText || "failed";
    // Benign: a navigation the page itself cancels (a redirect, a replaced
    // load), or a request a third-party frame (Stripe's) cancels on its own.
    // A request Scry's policy blocks fails with ERR_BLOCKED_BY_CSP instead.
    let foreignFrame = false;
    try {
      const f = r.frame();
      foreignFrame = f !== page.mainFrame() && new URL(f.url()).origin !== origin;
    } catch { /* not tied to a frame */ }
    if (err.includes("ERR_ABORTED") && (r.isNavigationRequest() || foreignFrame)) {
      rec.ignoredRequests.push({ url: clean(r.url()), type: r.resourceType(), error: err, reason: foreignFrame ? "cancelled by a third-party frame" : "cancelled navigation" });
      return;
    }
    rec.failedRequests.push({ url: clean(r.url()), type: r.resourceType(), error: err });
  });
  page.on("response", r => {
    if (r.status() >= 400) rec.failedRequests.push({ url: clean(r.url()), type: r.request().resourceType(), status: r.status() });
  });
  let res;
  try {
    res = await page.goto(base + p, { waitUntil: "load" });
    await page.waitForLoadState("networkidle", { timeout: Math.min(opt.timeout, 15000) }).catch(() => {});
  } catch (err) {
    check("load " + label, false, String(err.message || err).split("\n")[0]);
    await page.close();
    return null;
  }
  const final = new URL(page.url());
  rec.status = res ? res.status() : 0;
  rec.finalPath = final.pathname;
  if (final.pathname !== p && !allowFinal.includes(final.pathname)) check(label + " stays on " + p, false, "ended at " + final.pathname);
  if (expectStatus) check("GET " + label + " " + expectStatus, rec.status === expectStatus, { status: rec.status });

  let stripe = false;
  if (card) {
    const keyed = await page.locator("#card-element[data-key]").count();
    if (keyed) {
      stripe = true;
      const frame = page.locator('#card-element iframe[src*="js.stripe.com"]').first();
      try {
        await frame.waitFor({ state: "visible", timeout: Math.max(opt.timeout, 20000) });
        await page.waitForTimeout(1500); // let the field finish laying out
        const box = await frame.boundingBox();
        const src = await frame.getAttribute("src");
        rec.stripeIframe = { src: clean(src), width: box && Math.round(box.width), height: box && Math.round(box.height) };
        check(label + ": Stripe iframe visible, height >= 20 and width >= 150", !!box && box.height >= 20 && box.width >= 150, rec.stripeIframe);
      } catch (err) {
        check(label + ": Stripe iframe attached and visible", false, String(err.message || err).split("\n")[0]);
      }
      const shown = (await page.locator("#card-error").textContent().catch(() => "")) || "";
      check(label + ": no card-field error shown", shown.trim() === "", shown.trim());
    }
    if (screenshot) {
      fs.mkdirSync(path.dirname(path.resolve(screenshot)), { recursive: true });
      await page.screenshot({ path: screenshot, fullPage: true });
      (summary.screenshots ||= []).push(screenshot);
      rec.screenshot = screenshot;
    }
  }
  await page.waitForTimeout(500);
  for (const f of page.frames()) {
    try {
      const v = await f.evaluate(() => window.__scryCSP || []);
      for (const x of v) rec.cspViolations.push({ frame: clean(f.url()), directive: x.directive, blocked: clean(x.blocked || ""), source: clean(x.source || ""), line: x.line });
    } catch { /* a frame that went away */ }
  }
  check(label + ": 0 securitypolicyviolation", rec.cspViolations.length === 0, rec.cspViolations);
  check(label + ": 0 console.error", rec.consoleErrors.length === 0, rec.consoleErrors);
  check(label + ": 0 pageerror", rec.pageErrors.length === 0, rec.pageErrors);
  check(label + ": 0 failed or blocked requests", rec.failedRequests.length === 0, rec.failedRequests);
  rec.stripe = stripe;
  summary.pages.push(rec);
  await page.close();
  return rec;
}

async function localSignup(context, base, opt) {
  const h = summary.healthz || {};
  if (h.mode === "live") return check("--local-signup", false, "refused: /healthz says mode=live; use --session-cookie for a live deploy");
  if (String(h.auth_mode || "").startsWith("clerk-")) return check("--local-signup", false, "refused: auth is Clerk; use --session-cookie");
  const page = await context.newPage();
  try {
    const email = "verify-" + Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 8) + "@example.com";
    await page.goto(base + "/signup", { waitUntil: "load" });
    await page.fill("#email", email);
    await page.fill("#password", "verify-live-" + Math.random().toString(36).slice(2));
    await Promise.all([page.waitForURL(u => new URL(u).pathname !== "/signup", { timeout: opt.timeout }), page.click("form[action='/signup'] button, main form button")]);
    const at = new URL(page.url()).pathname;
    summary.session = Object.assign(summary.session || {}, { localSignup: true, landedOn: at });
    // The no-card trial starts at once: straight into Scry, never the card step.
    const landed = check("local sign-up lands in Scry (/shelf or /study), not /signup/card", at === "/shelf" || at === "/study", { landedOn: at });
    const chip = page.locator("[data-trial]");
    const shown = (await chip.count()) > 0 && await chip.first().isVisible();
    const text = shown ? ((await chip.first().textContent()) || "").replace(/\s+/g, " ").trim() : "";
    summary.session.trialIndicator = text || null;
    const chipOk = check("trial status line shown after sign-up (Trial · N days left · Upgrade)",
      /^Trial · (\d+ days? left|last day) · Upgrade$/.test(text), { text });
    const href = shown ? await chip.first().getAttribute("href") : "";
    check("trial status line links to the card step", href === "/signup/card" || href === "/billing", { href });
    return landed && chipOk;
  } catch (err) {
    return check("local sign-up", false, String(err.message || err).split("\n")[0]);
  } finally {
    await page.close();
  }
}

// simulateViolation loads / and makes the page load an image its policy
// forbids, then checks the violation is seen and POSTed to
// /api/client-error, answered 204, with no query string and only closed
// telemetry fields in what app.js sent.
async function simulateViolation(context, base, opt) {
  const page = await context.newPage();
  const posts = [];
  page.on("request", r => {
    if (r.url().startsWith(base + "/api/client-error")) posts.push({ req: r, type: r.headers()["content-type"] || "", body: r.postData() || "" });
  });
  const responses = [];
  page.on("response", r => { if (r.url().startsWith(base + "/api/client-error")) responses.push(r.status()); });
  // sendBeacon bodies are not visible to the protocol, so read them here.
  await page.addInitScript(() => {
    window.__scryBeacons = [];
    const send = navigator.sendBeacon && navigator.sendBeacon.bind(navigator);
    if (!send) return;
    navigator.sendBeacon = (url, data) => {
      if (data instanceof Blob) data.text().then(body => window.__scryBeacons.push({ url: String(url), type: data.type, body }));
      else window.__scryBeacons.push({ url: String(url), type: "", body: String(data) });
      return send(url, data);
    };
  });
  try {
    await page.goto(base + "/", { waitUntil: "load" });
    await page.waitForFunction(() => typeof window.scryReport === "function", null, { timeout: opt.timeout });
    await page.evaluate(() => {
      const img = document.createElement("img");
      img.src = "https://csp-probe.invalid/probe.png?token=verify-probe-secret#frag";
      document.body.appendChild(img);
    });
    const deadline = Date.now() + 8000;
    while (Date.now() < deadline && (posts.length === 0 || responses.length < posts.length)) await page.waitForTimeout(200);
    await page.waitForTimeout(1000);
    const seen = await page.evaluate(() => window.__scryCSP || []);
    const beacons = await page.evaluate(() => window.__scryBeacons || []);
    const fromApp = posts.filter(p => p.type.includes("application/json") && p.body)
      .concat(beacons.filter(b => b.url.endsWith("/api/client-error")));
    const sim = {
      violationsSeen: seen.map(x => ({ directive: x.directive, blocked: clean(x.blocked || "") })),
      posts: posts.map(p => ({ contentType: p.type.split(";")[0], resourceType: p.req.resourceType() })),
      appReports: fromApp.map(p => { try { const j = JSON.parse(p.body); return { kind: j.kind, route: j.route, violatedDirective: j.violatedDirective, blockedURI: j.blockedURI }; } catch { return { unparsed: true }; } }),
      responses,
    };
    summary.simulatedViolation = sim;
    check("simulated violation: img-src violation observed", seen.some(x => x.directive === "img-src"), sim.violationsSeen);
    check("simulated violation: app.js POSTed it to /api/client-error", fromApp.some(p => /"kind":"csp_violation"/.test(p.body) && /"img-src"/.test(p.body)), sim.appReports);
    check("simulated violation: /api/client-error answered 204", responses.length > 0 && responses.every(s => s === 204), responses);
    check("simulated violation: no query string or fragment sent", fromApp.length > 0 && fromApp.every(p => !p.body.includes("verify-probe-secret") && !p.body.includes("#frag")));
    // app.js sends only the closed telemetry fields (no free-text message);
    // the server maps each onto its own allowlists again before logging.
    const fields = new Set(["kind", "route", "violatedDirective", "effectiveDirective", "blockedURI", "disposition", "code", "errorClass", "sourceFile", "line", "column"]);
    check("simulated violation: app.js sent only closed telemetry fields", fromApp.length > 0 && fromApp.every(p => {
      try { return Object.keys(JSON.parse(p.body)).every(k => fields.has(k)); } catch { return false; }
    }), fromApp.map(p => { try { return Object.keys(JSON.parse(p.body)); } catch { return ["unparsed"]; } }));
  } catch (err) {
    check("simulated violation", false, String(err.message || err).split("\n")[0]);
  } finally {
    await page.close();
  }
}

main().then(() => {
  summary.ok = summary.failures.length === 0;
  summary.finishedAt = new Date().toISOString();
  process.stdout.write(JSON.stringify(summary, null, 2) + "\n");
  if (!summary.ok) {
    process.stderr.write("verify-live: " + summary.failures.length + " failure(s):\n" + summary.failures.map(f => "  - " + f).join("\n") + "\n");
    process.exit(1);
  }
}).catch(err => {
  summary.ok = false;
  summary.failures.push(String(err && err.message || err));
  process.stdout.write(JSON.stringify(summary, null, 2) + "\n");
  process.stderr.write("verify-live: " + (err && err.message || err) + "\n");
  process.exit(2);
});
