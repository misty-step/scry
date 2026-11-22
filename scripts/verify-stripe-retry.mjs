#!/usr/bin/env node
// verify-stripe-retry.mjs: the card step never submits a superseded card.
//
//   node scripts/verify-stripe-retry.mjs --url http://127.0.0.1:18080 [--evidence dir]
//
// verify-slice41.mjs runs it too. It needs a test-mode server with local
// sign-up, and reproduces, in a real headless Chromium with the shipped
// app.js and stripe.js and the real 30 second timer:
//
//   submit #1 (card A): the processor is slow, more than 30 seconds
//   -> the processing state times out and gives the form back
//   -> submit #2 (card B) starts
//   -> #1 answers while #2 is still pending
//   => #1 is ignored: nothing is written to the form, nothing is posted,
//      the form stays locked on #2 and prints no error
//   => only #2's payment method is posted, once.
//
// The late answer to #1 is tried three ways (a payment method, a refusal,
// a network failure). A fourth run stalls the post itself: #1 is confirmed
// and submitted, no page comes back, the form is given back, and the retry
// confirms and posts card B, not card A again.
//
// The page is the server's own card step, served with the Stripe card
// field it renders when Stripe is configured (templates/partials.html,
// "cardfields"), and https://js.stripe.com/v3/ is answered by an
// instrumented stand-in whose createPaymentMethod the test resolves by
// hand. No request leaves the machine. Run alone, it prints one JSON
// summary and exits 0 only when every check passes.

import fs from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { chromium } from "playwright-core";

// The stand-in for Stripe.js v3. Every createPaymentMethod call is kept
// pending in window.__stripe.calls with the card that was in the field when
// it was made; the test settles each call when it chooses.
const STRIPE_STUB = `(function () {
  var state = window.__stripe = { calls: [], card: "", ready: false, key: "" };
  window.Stripe = function (key) {
    state.key = key;
    return {
      elements: function () {
        return { create: function () {
          return {
            mount: function (el) { state.mounted = el.id; },
            on: function (ev, fn) { if (ev === "ready") setTimeout(function () { state.ready = true; fn(); }, 0); }
          };
        } };
      },
      createPaymentMethod: function () {
        return new Promise(function (resolve, reject) {
          state.calls.push({ card: state.card, resolve: resolve, reject: reject, settled: false });
        });
      }
    };
  };
})();`;

// The card field as templates/partials.html renders it with Stripe on.
const STRIPE_FIELDS = `
  <div class="field">
    <label class="label" for="card-element">Card</label>
    <div id="card-element" class="input stripe-card" data-key="pk_test_verify_stripe_retry"></div>
    <input type="hidden" name="payment_method" id="payment_method">
    <p class="field-error" id="card-error" role="alert"></p>
  </div>
`;

// stripeCardPage turns the test-mode card step into the Stripe one.
function stripeCardPage(html) {
  const form = html.indexOf("data-card-form");
  const from = html.indexOf(">", html.indexOf('name="billing_terms"', form)) + 1;
  const to = html.indexOf("<button", from);
  if (form < 0 || from <= 0 || to < 0) throw new Error("the card step did not render its form");
  html = html.slice(0, from) + STRIPE_FIELDS + html.slice(to);
  html = html.replace("Confirming your card…", "Confirming your card with Stripe…");
  return html.replace("</head>", '<script src="https://js.stripe.com/v3/" defer></script><script src="/static/stripe.js" defer></script></head>');
}

// How #1 answers, late, inside the page.
const LATE = ["payment method", "refusal", "network failure"];
const settleLate = name => {
  const call = window.__stripe.calls[0];
  if (name === "payment method") call.resolve({ paymentMethod: { id: "pm_card_A_superseded" } });
  if (name === "refusal") call.resolve({ error: { message: "Card A was declined." } });
  if (name === "network failure") call.reject(new Error("Failed to fetch"));
};

// stripeRetry runs the four races side by side (each waits out the real
// 30 seconds) and reports through check(name, ok, detail).
export async function stripeRetry(browser, base, check, opt = {}) {
  const result = { runs: [] };
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: "light" });
  context.setDefaultTimeout(20000);
  await context.addInitScript(() => {
    window.__csp = [];
    document.addEventListener("securitypolicyviolation", e => window.__csp.push((e.effectiveDirective || e.violatedDirective) + " " + e.blockedURI), true);
  });
  await context.route("https://js.stripe.com/**", route => route.fulfill({ status: 200, contentType: "application/javascript", body: STRIPE_STUB }));

  // formState reads the card form; gone means the page has been left (a post went out).
  const formState = page => page.evaluate(() => {
    const f = document.querySelector("[data-card-form]");
    if (!f || !window.__stripe) return { gone: true, posted: document.querySelector("[data-posted]")?.textContent || "", path: location.pathname };
    const b = f.querySelector("button"), box = f.querySelector("[data-busy]");
    return { busy: f.classList.contains("is-busy"), ariaBusy: f.getAttribute("aria-busy"), disabled: b.disabled, boxShown: !box.hidden && box.getClientRects().length > 0,
      busyText: box.textContent.replace(/\s+/g, " ").trim(), hidden: document.getElementById("payment_method").value, error: document.getElementById("card-error").textContent.trim(),
      attempt: f.dataset.submitAttempt || "", calls: window.__stripe.calls.map(c => c.card), csp: window.__csp || [] };
  });
  const locked = s => !s.gone && s.busy && s.ariaBusy === "true" && s.disabled && s.boxShown;
  const press = page => page.click("[data-card-form] button", { noWaitAfter: true });
  const setCard = (page, card) => page.evaluate(c => { window.__stripe.card = c; }, card);

  async function race(name) {
    const late = LATE.includes(name);
    const at = "(B3) stripe retry, " + (late ? "late " + name + " for #1" : name);
    const run = { name, posts: [] };
    result.runs.push(run);
    const problems = [];
    const page = await context.newPage();
    page.on("console", m => { if (m.type() === "error") problems.push("console.error: " + m.text().slice(0, 200)); });
    page.on("pageerror", e => problems.push("pageerror: " + String(e.message).slice(0, 200)));
    try {
      await page.route(u => u.origin === new URL(base).origin && u.pathname === "/signup/card", async route => {
        const req = route.request();
        if (req.method() !== "POST") {
          const res = await route.fetch();
          const headers = { ...res.headers() };
          // What csp.go adds for Stripe: its script origin.
          if (headers["content-security-policy"]) headers["content-security-policy"] = headers["content-security-policy"].replace(/script-src /, "script-src https://js.stripe.com ");
          return route.fulfill({ status: res.status(), headers, body: stripeCardPage(await res.text()) });
        }
        const pm = new URLSearchParams(req.postData() || "").get("payment_method");
        run.posts.push(pm);
        // The stalled-post run's first post brings no page back (204): the
        // learner is left on the card step, as when a request is stopped.
        if (name === "post that stalls" && run.posts.length === 1) return route.fulfill({ status: 204 });
        return route.fulfill({ status: 200, contentType: "text/html; charset=utf-8", body: "<!doctype html><title>posted</title><p data-posted>" + pm + "</p>" });
      });
      await page.goto(base + "/signup/card", { waitUntil: "load" });
      await page.waitForFunction(() => window.__stripe && window.__stripe.ready && window.scryBusy);
      const fresh = await formState(page);
      check(at + ": the card step shows the Stripe field, unlocked", !fresh.busy && !fresh.disabled && fresh.hidden === "" && fresh.calls.length === 0 && fresh.csp.length === 0, fresh);

      // Submit #1, card A.
      await setCard(page, "A");
      const t0 = Date.now();
      await press(page);
      await page.waitForFunction(() => window.__stripe.calls.length === 1, null, { polling: 50 });
      let s = await formState(page);
      check(at + ": submit #1 locks the form and asks the processor once", locked(s) && s.busyText === "Confirming your card with Stripe…" && s.calls.join() === "A" && run.posts.length === 0, s);
      // A second submit while #1 is with the processor is held: still one request.
      const held = await page.evaluate(() => { const e = new Event("submit", { cancelable: true, bubbles: true }); document.querySelector("[data-card-form]").dispatchEvent(e); return e.defaultPrevented; });
      s = await formState(page);
      check(at + ": a second submit while #1 is pending is held (one request to the processor)", held && s.calls.join() === "A" && locked(s) && run.posts.length === 0, { held, calls: s.calls });
      if (name === "post that stalls") {
        // #1 is confirmed at once and its post brings no page back.
        await page.evaluate(() => window.__stripe.calls[0].resolve({ paymentMethod: { id: "pm_card_A" } }));
        await page.waitForFunction(() => document.getElementById("payment_method").value === "pm_card_A", null, { polling: 50 });
        for (let i = 0; i < 40 && run.posts.length < 1; i++) await page.waitForTimeout(50);
        check(at + ": #1 is confirmed and posted", run.posts.join() === "pm_card_A", run.posts);
      }

      // More than 30 seconds pass: the processing state gives the form back.
      await page.waitForFunction(() => { const f = document.querySelector("[data-card-form]"); return !f || (!f.classList.contains("is-busy") && !f.querySelector("button").disabled); }, null, { timeout: 45000, polling: 100 });
      run.timeoutMs = Date.now() - t0;
      s = await formState(page);
      const before = name === "post that stalls" ? "pm_card_A" : "";
      check(at + ": after 30 s (" + run.timeoutMs + " ms) the timeout gives the form back, nothing new posted", run.timeoutMs >= 29500 && run.timeoutMs < 40000 && !s.busy && !s.disabled && !s.boxShown && s.hidden === before && run.posts.length === (before ? 1 : 0), { s, posts: run.posts });
      const attemptAfterTimeout = Number(s.attempt);

      // Submit #2, card B.
      await setCard(page, "B");
      await press(page);
      await page.waitForFunction(() => !document.querySelector("[data-card-form]") || !window.__stripe || window.__stripe.calls.length === 2, null, { polling: 50 });
      s = await formState(page);
      check(at + ": submit #2 is a new attempt for the card in the field now", locked(s) && s.calls.join() === "A,B" && Number(s.attempt) > attemptAfterTimeout && s.hidden === "", { s, posts: run.posts });
      if (s.gone) {
        check(at + ": the retry confirms card B before anything is posted", false, { posts: run.posts });
        return;
      }

      if (late) {
        // #1 answers while #2 is still pending.
        await page.evaluate(settleLate, name).catch(() => {});
        await page.waitForTimeout(500);
        s = await formState(page).catch(() => ({ gone: true }));
        run.afterLate = { hidden: s.hidden, error: s.error, locked: locked(s), left: !!s.gone, posts: run.posts.slice() };
        check(at + ": #1's answer is ignored: nothing written to the form, nothing posted", !s.gone && s.hidden === "" && run.posts.length === 0, run.afterLate);
        check(at + ": #1's answer does not unlock the form or print an error over #2", locked(s) && s.error === "" && s.busyText === "Confirming your card with Stripe…", s);
        if (s.gone) {
          check(at + ": only #2's payment method is submitted (posts: " + run.posts.join(", ") + ")", false, { posts: run.posts });
          return;
        }
        if (opt.evidence && name === "payment method") {
          const file = path.join(opt.evidence, "15-stripe-retry-superseded-ignored.png");
          await page.screenshot({ path: file });
          result.screenshot = file;
        }
      }

      // #2 answers: its payment method, and only its, is posted.
      await Promise.all([
        page.waitForSelector("[data-posted]"),
        page.evaluate(() => window.__stripe.calls[1].resolve({ paymentMethod: { id: "pm_card_B" } })),
      ]);
      await page.waitForTimeout(400);
      const posted = await page.$eval("[data-posted]", e => e.textContent);
      const want = name === "post that stalls" ? "pm_card_A,pm_card_B" : "pm_card_B";
      check(at + (late ? ": only #2's payment method is submitted" : ": the retry posts card B, not card A again") + " (posts: " + run.posts.join(", ") + ")",
        run.posts.join() === want && posted === "pm_card_B" && !run.posts.includes("pm_card_A_superseded"), { posts: run.posts, posted });
    } catch (err) {
      check(at + ": the run completed", false, String(err.stack || err).split("\n").slice(0, 3).join(" | "));
    } finally {
      check(at + ": 0 console errors or page errors", problems.length === 0, problems);
      await page.close().catch(() => {});
    }
  }

  try {
    // One learner, on the card step.
    const page = await context.newPage();
    await page.goto(base + "/signup", { waitUntil: "load" });
    await page.fill("#email", "stripe-retry-" + Date.now().toString(36) + Math.random().toString(36).slice(2, 6) + "@example.com");
    await page.fill("#password", "stripe-retry-" + Math.random().toString(36).slice(2));
    await Promise.all([page.waitForURL(u => new URL(u).pathname !== "/signup"), page.click("main form button")]);
    await page.close();
    await Promise.all([
      ...LATE.map(race),
      race("post that stalls"),
    ]);
  } catch (err) {
    check("(B3) stripe retry: set up", false, String(err.stack || err).split("\n").slice(0, 3).join(" | "));
  } finally {
    await context.close();
  }
  return result;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const args = process.argv.slice(2);
  const opt = { url: process.env.SCRY_VERIFY_URL || "", evidence: "" };
  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--url") opt.url = args[++i];
    else if (args[i] === "--evidence") opt.evidence = path.resolve(args[++i]);
    else { console.error("usage: verify-stripe-retry.mjs --url <base-url> [--evidence dir]"); process.exit(2); }
  }
  if (!opt.url) { console.error("--url is required"); process.exit(2); }
  const base = opt.url.replace(/\/+$/, "");
  if (opt.evidence) fs.mkdirSync(opt.evidence, { recursive: true });
  const summary = { ok: false, url: base, startedAt: new Date().toISOString(), checks: [], failures: [] };
  const check = (name, ok, detail) => {
    summary.checks.push(detail === undefined || ok ? { name, ok } : { name, ok, detail });
    if (!ok) summary.failures.push(detail === undefined ? name : name + ": " + (typeof detail === "string" ? detail : JSON.stringify(detail)));
    return ok;
  };
  const health = await fetch(base + "/healthz").then(r => r.json()).catch(() => ({}));
  summary.healthz = { status: health.status, mode: health.mode, build_sha: health.build_sha, auth_mode: health.auth_mode, billing_mode: health.billing_mode };
  if (health.mode !== "test" || String(health.auth_mode || "").startsWith("clerk-")) {
    console.error("verify-stripe-retry: needs a test-mode server with local sign-up; /healthz says " + JSON.stringify(summary.healthz));
    process.exit(2);
  }
  const browser = await chromium.launch({ executablePath: [process.env.CHROME_BIN, "/usr/bin/google-chrome", "/usr/bin/chromium"].find(p => p && fs.existsSync(p)), headless: true, args: ["--no-sandbox"] });
  try {
    summary.stripeRetry = await stripeRetry(browser, base, check, opt);
  } finally {
    await browser.close();
  }
  summary.ok = summary.failures.length === 0;
  summary.passed = summary.checks.filter(c => c.ok).length;
  summary.total = summary.checks.length;
  summary.finishedAt = new Date().toISOString();
  process.stdout.write(JSON.stringify(summary, null, 2) + "\n");
  if (!summary.ok) {
    process.stderr.write("verify-stripe-retry: " + summary.failures.length + " failure(s):\n" + summary.failures.map(f => "  - " + f).join("\n") + "\n");
    process.exit(1);
  }
}
