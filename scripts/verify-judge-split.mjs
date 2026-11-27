#!/usr/bin/env node
// verify-judge-split.mjs: check SCRY-38 ("judge failed": a split Jev decision
// -> HTTP 503) on the real running path, in a real, headed Chrome, without
// calling a provider or spending anything.
//
//   make build BUILD_SHA=$(git rev-parse HEAD)
//   DISPLAY=:0 node scripts/verify-judge-split.mjs --bin ./bin/scry --label candidate \
//        [--media ~/chief-evidence/media] [--video] [--expect graded|refused] \
//        [--scenarios desktop:almost,mobile:almost,desktop:correct,desktop:llm-down] [--port 18038]
//   (xvfb-run -a node ... when there is no display)
//
// The script starts two things of its own:
//
//   1. A local stand-in for OpenRouter. Scry's provider origin is fixed
//      (https://openrouter.ai), so the stand-in is an HTTPS proxy with its own
//      throwaway CA: Scry is started with HTTPS_PROXY and SSL_CERT_FILE
//      pointing at it, and with a dummy OPENROUTER_API_KEY. Every provider
//      request Scry makes lands here; nothing leaves the machine. It answers
//      - POST /api/alpha/decisions for typesafe/jev-1.14 with the response
//        recorded live for Phaedrus's question and answer on 2026-11-25:
//        choice "correct", probabilities correct 0.64 / almost correct 0.36,
//        confidence 0.52 (the SCRY-38 split);
//      - POST /api/alpha/decisions for any other decisions model with a 503,
//        counted as "shopped" (an uncertain decision must never get there);
//      - POST /api/v1/chat/completions (the escalation LLM) with the
//        scenario's verdict, or a 502 in the llm-down scenario.
//   2. The --bin Scry in SCRY_MODE=test SCRY_JUDGE_MODE=live on a fresh
//      SQLite file: real Study handler, real grading chain, real Jev and
//      escalation adapters; auth, billing and drafting are the test fakes.
//
// Each scenario (viewport:llm, desktop 1440x900 or mobile 390x844) signs up
// a disposable learner, imports one card from a CSV file on Create
// (Phaedrus's exact production question), opens Study, types Phaedrus's
// exact multi-sentence answer and presses Check.
//
// --expect graded (default): the answer must stop on its review with the
// verdict the escalation LLM gave (Jev asked once, no other decisions model
// asked, the LLM asked once), never "Couldn't check that just now"; in the
// llm-down scenario, the documented dead end is expected instead (503, the
// answer still in the field). --expect refused is for the incident release
// (8a14e49, no grading chain): every scenario must show the 503.
//
// Counted per page: securitypolicyviolation events, console errors, page
// errors, failed requests, HTTP >= 400 responses, and horizontal overflow
// (documentElement.scrollWidth - clientWidth) on every screen visited.
// Exit 0 only when every check passes.

import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import http from "node:http";
import https from "node:https";
import net from "node:net";
import { spawn, execFileSync } from "node:child_process";
import { chromium } from "playwright-core";

const args = Object.fromEntries(process.argv.slice(2).reduce((a, v, i, all) => {
  if (v.startsWith("--")) a.push([v.slice(2), all[i + 1] && !all[i + 1].startsWith("--") ? all[i + 1] : "1"]);
  return a;
}, []));
const BIN = path.resolve(args.bin || "bin/scry");
const LABEL = args.label || "run";
const MEDIA = args.media ? path.resolve(args.media) : "";
const VIDEO = !!args.video;
const EXPECT = args.expect || "graded";
const PORT = Number(args.port || 18038);
const SCENARIOS = (args.scenarios || "desktop:almost,mobile:almost,desktop:correct,desktop:llm-down").split(",").map(s => {
  const [viewport, llm] = s.split(":");
  return { name: `${viewport}-${llm}`, viewport, llm };
});
const CHROME = process.env.CHROME_BIN || "/usr/bin/google-chrome";
if (!["graded", "refused"].includes(EXPECT)) { console.error("--expect must be graded or refused"); process.exit(2); }
if (MEDIA) fs.mkdirSync(MEDIA, { recursive: true });

// Phaedrus's production repro (scry.mistystep.io item 27, 2026-11-25 22:34Z).
const QUESTION = "What is the testing effect (retrieval practice) in learning science, and why does it outperform rereading?";
const EXPECTED = "Recalling information from memory strengthens long-term retention more than rereading, because effortful retrieval reinforces and reorganizes the memory trace and reveals gaps, while rereading builds only familiarity.";
const ANSWER = "the testing effect states that it is better for recall to quiz a student and have them actually try to answer a question rather than just reread the material because it is challenging, it forces you to dig through your mind and find the material and the understanding yourself rather than just let the words on the page pass over you again";
// typesafe/jev-1.14 on OpenRouter, live, for that state (id redacted).
const JEV_SPLIT = '{"model":"typesafe/jev-1.14-20261014","answers":{"judgment":{"type":"choice","choice":"correct","probabilities":{"almost correct":0.36,"correct":0.64,"incorrect":0,"none_of_these":0},"confidence":0.52}},"usage":{"input_tokens":520,"output_tokens":51,"cost":0.00002184,"is_byok":false},"id":"gen-dec-redacted","provider":"TypeSafe"}';
const JEV = "typesafe/jev-1.14";
const LLM = { almost: "almost correct", correct: "correct", incorrect: "incorrect" };
const VIEW = { almost: ["close", /^Almost( Correct)?$/], correct: ["right", /^Correct$/], incorrect: ["missed", /^Incorrect$/] };
const DUMMY_KEY = "sk-or-v1-local-standin-not-a-real-key";
const csvCell = s => '"' + s.replace(/"/g, '""') + '"';
const CSV = "question,answer\n" + csvCell(QUESTION) + "," + csvCell(EXPECTED) + "\n";
const VIEWPORTS = {
  desktop: { viewport: { width: 1440, height: 900 }, mobile: false, dpr: 1 },
  mobile: { viewport: { width: 390, height: 844 }, mobile: true, dpr: 2 },
};

const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "scry-judge-split-"));
const results = [];
function check(name, ok, detail) {
  results.push({ name, ok: !!ok, detail });
  console.log((ok ? "PASS " : "FAIL ") + name + (detail !== undefined ? " " + JSON.stringify(detail) : ""));
}

// ---- 1. The OpenRouter stand-in -------------------------------------------

function makeCerts(dir) {
  const run = (...a) => execFileSync("openssl", a, { cwd: dir, stdio: "ignore" });
  run("req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "2", "-subj", "/CN=scry judge-split stand-in CA", "-keyout", "ca.key", "-out", "ca.pem",
    "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign");
  run("req", "-newkey", "rsa:2048", "-nodes", "-subj", "/CN=openrouter.ai", "-keyout", "leaf.key", "-out", "leaf.csr");
  fs.writeFileSync(path.join(dir, "leaf.ext"), "subjectAltName=DNS:openrouter.ai\nbasicConstraints=CA:FALSE\nextendedKeyUsage=serverAuth\n");
  run("x509", "-req", "-in", "leaf.csr", "-CA", "ca.pem", "-CAkey", "ca.key", "-CAcreateserial", "-days", "2", "-out", "leaf.pem", "-extfile", "leaf.ext");
  return { ca: path.join(dir, "ca.pem"), key: fs.readFileSync(path.join(dir, "leaf.key")), cert: fs.readFileSync(path.join(dir, "leaf.pem")) };
}

// state.llm is the current scenario's escalation behaviour; state.calls is
// what Scry asked for since the scenario began.
const state = { llm: "almost", calls: [], foreign: [] };

async function startStandIn() {
  const certs = makeCerts(tmp);
  const tls = https.createServer({ key: certs.key, cert: certs.cert }, (req, res) => {
    let raw = "";
    req.on("data", d => raw += d);
    req.on("end", () => {
      let body = {};
      try { body = JSON.parse(raw); } catch { /* counted below as it came */ }
      const auth = req.headers.authorization === "Bearer " + DUMMY_KEY;
      const call = { path: req.url, model: body.model || "", auth, exact: false };
      const send = (status, text) => { res.writeHead(status, { "Content-Type": "application/json" }); res.end(text); };
      if (req.method === "POST" && req.url === "/api/alpha/decisions") {
        call.exact = !!body.state && body.state.card_prompt === QUESTION && body.state.learner_answer === ANSWER;
        state.calls.push(call);
        if (body.model === JEV) return send(200, JEV_SPLIT);
        return send(503, '{"error":{"code":503,"message":"stand-in: an uncertain decision was shopped to another decisions model"}}');
      }
      if (req.method === "POST" && req.url === "/api/v1/chat/completions") {
        const user = ((body.messages || [])[1] || {}).content || "";
        call.exact = user.includes(JSON.stringify(ANSWER)) && user.includes(JSON.stringify(QUESTION));
        state.calls.push(call);
        if (state.llm === "llm-down") return send(502, '{"error":{"code":502,"message":"stand-in: escalation down"}}');
        return send(200, JSON.stringify({ id: "gen-standin", model: body.model, choices: [{ message: { role: "assistant", content: JSON.stringify({ verdict: LLM[state.llm] }) }, finish_reason: "stop" }],
          usage: { prompt_tokens: 610, completion_tokens: 9, cost: 0.00065 } }));
      }
      state.calls.push(call);
      send(404, '{"error":{"code":404}}');
    });
  });
  await new Promise(r => tls.listen(0, "127.0.0.1", r));
  const proxy = http.createServer((req, res) => { state.foreign.push(req.method + " " + req.url); res.writeHead(502); res.end(); });
  proxy.on("connect", (req, socket, head) => {
    if (req.url !== "openrouter.ai:443") { // nothing else may leave
      state.foreign.push("CONNECT " + req.url);
      socket.end("HTTP/1.1 403 Forbidden\r\n\r\n");
      return;
    }
    const up = net.connect(tls.address().port, "127.0.0.1", () => {
      socket.write("HTTP/1.1 200 Connection Established\r\n\r\n");
      if (head.length) up.write(head);
      up.pipe(socket);
      socket.pipe(up);
    });
    up.on("error", () => socket.destroy());
    socket.on("error", () => up.destroy());
  });
  await new Promise(r => proxy.listen(0, "127.0.0.1", r));
  return { ca: certs.ca, proxyURL: "http://127.0.0.1:" + proxy.address().port, close: () => { proxy.close(); tls.close(); proxy.closeAllConnections(); tls.closeAllConnections(); } };
}

// ---- 2. Scry ---------------------------------------------------------------

async function startScry(standIn) {
  const emptyCerts = path.join(tmp, "no-system-certs");
  fs.mkdirSync(emptyCerts);
  const logPath = path.join(tmp, "scry.log");
  const log = fs.openSync(logPath, "w");
  const proc = spawn(BIN, [], {
    // A minimal environment: no real provider key is passed on.
    env: {
      PATH: process.env.PATH, HOME: tmp, SCRY_MODE: "test", SCRY_JUDGE_MODE: "live", OPENROUTER_API_KEY: DUMMY_KEY,
      SCRY_DB: path.join(tmp, "judge-split.db"), PORT: String(PORT),
      HTTPS_PROXY: standIn.proxyURL, HTTP_PROXY: standIn.proxyURL, NO_PROXY: "", SSL_CERT_FILE: standIn.ca, SSL_CERT_DIR: emptyCerts,
    },
    stdio: ["ignore", log, log],
  });
  const base = "http://127.0.0.1:" + PORT;
  for (let i = 0; i < 100; i++) {
    try {
      const r = await fetch(base + "/healthz");
      if (r.ok) return { proc, base, logPath, health: await r.json() };
    } catch { /* not up yet */ }
    if (proc.exitCode !== null) break;
    await new Promise(r => setTimeout(r, 100));
  }
  proc.kill();
  throw new Error("Scry did not start:\n" + fs.readFileSync(logPath, "utf8").slice(-2000));
}

// ---- 3. The learner's path -------------------------------------------------

function watch(page, h) {
  page.on("console", m => {
    if (m.type() === "error") h.console.push(m.text().slice(0, 200));
    if (m.text().startsWith("SJ-CSP ")) h.csp.push(m.text().slice(7));
  });
  page.on("pageerror", e => h.pageErrors.push(String(e).slice(0, 200)));
  page.on("requestfailed", r => h.failed.push(r.url() + " " + ((r.failure() || {}).errorText || "")));
  page.on("response", r => { if (r.status() >= 400) h.http.push(r.status() + " " + r.request().method() + " " + new URL(r.url()).pathname); });
}

async function runScenario(sc, base, logPath) {
  const c = VIEWPORTS[sc.viewport];
  const refused = EXPECT === "refused" || sc.llm === "llm-down"; // the 503 page is the expected outcome
  const h = { console: [], pageErrors: [], failed: [], http: [], csp: [], overflow: [] };
  state.llm = sc.llm;
  state.calls = [];
  const logFrom = fs.statSync(logPath).size;
  const browser = await chromium.launch({ executablePath: CHROME, headless: false });
  const videoDir = fs.mkdtempSync(path.join(tmp, "video-"));
  const context = await browser.newContext({
    viewport: c.viewport, deviceScaleFactor: c.dpr, isMobile: c.mobile, hasTouch: c.mobile,
    ...(VIDEO ? { recordVideo: { dir: videoDir, size: c.viewport } } : {}),
  });
  await context.addInitScript(() => {
    document.addEventListener("securitypolicyviolation", e => {
      console.log("SJ-CSP " + e.violatedDirective + " " + (e.blockedURI || "") + " " + location.pathname);
    }, true);
  });
  const page = await context.newPage();
  watch(page, h);
  const press = l => (c.mobile ? l.tap() : l.click());
  const overflow = async where => h.overflow.push({ where, px: await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth) });
  const shot = async name => { if (MEDIA) await page.screenshot({ path: path.join(MEDIA, `${LABEL}-${sc.name}-${name}.png`), caret: "initial" }); };
  const out = { scenario: sc.name };
  const n = sc.name;
  try {
    // Sign up a disposable learner (test-mode auth).
    await page.goto(base + "/signup", { waitUntil: "load" });
    await overflow("signup");
    await page.fill("#email", `sj-${LABEL}-${sc.name}-${Date.now()}@example.com`.toLowerCase());
    await page.fill("#password", "judge-split-disposable-pass");
    await Promise.all([page.waitForURL(u => new URL(u).pathname !== "/signup"), press(page.locator("main form button.btn-accent").first())]);

    // Create -> Import a deck: one CSV card with Phaedrus's question.
    await page.goto(base + "/learn", { waitUntil: "load" });
    await overflow("learn");
    await page.setInputFiles("#import-file", { name: "testing-effect.csv", mimeType: "text/csv", buffer: Buffer.from(CSV) });
    await page.fill("#import-idea", "Learning science");
    await Promise.all([page.waitForURL(u => new URL(u).pathname.startsWith("/library/ideas/"), { timeout: 15000 }), press(page.locator("[data-import-submit]"))]);
    await page.waitForLoadState("load");
    await overflow("library-idea");

    // Study: the imported card is the current one.
    await page.goto(base + "/study", { waitUntil: "load" });
    await page.waitForSelector("[data-answer-form]");
    const prompt = (await page.locator("#prompt").innerText()).trim();
    check(`${n}: Study shows Phaedrus's exact question`, prompt === QUESTION, prompt);
    const input = page.locator("#answer");
    await press(input);
    await input.pressSequentially(ANSWER, { delay: VIDEO ? 4 : 0 });
    check(`${n}: the exact multi-sentence answer is typed`, (await input.inputValue()) === ANSWER);
    await overflow("study-answer-typed");
    await shot("1-answer-typed");

    // Check: the real chain grades it against the stand-in.
    const answerResp = page.waitForResponse(r => new URL(r.url()).pathname === "/study/answer" && r.request().method() === "POST", { timeout: 60000 });
    await press(page.locator("[data-check]"));
    const resp = await answerResp;
    out.answerStatus = resp.status();
    await page.waitForLoadState("load");
    await page.waitForFunction(() => document.querySelector("[data-review]") || /Couldn't check/.test((document.querySelector("[data-status]") || {}).textContent || ""), null, { timeout: 60000 });
    await page.waitForTimeout(600); // past the arrive animation
    const facts = await page.evaluate(() => {
      const q = s => document.querySelector(s);
      return {
        review: !!q("[data-review]"),
        verdict: (q("[data-verdict]") || { dataset: {} }).dataset.verdict || "",
        label: (q("[data-verdict-label]") || {}).textContent?.trim() || "",
        why: (q("[data-review] .why") || {}).textContent?.trim() || "",
        status: (q("[data-status]") || {}).textContent?.trim() || "",
        askWhy: !!q("[data-explain]"),
        kept: (q("#answer") || {}).value || "",
      };
    });
    Object.assign(out, facts);
    await overflow("after-check");
    await shot(refused ? "2-refused" : "2-judged");
    // A long answer can push the verdict below the fold on a phone: the
    // review stays framed at the question, so show the verdict as well.
    const badge = page.locator("[data-verdict]").first();
    if (facts.review && !(await badge.evaluate(el => { const r = el.getBoundingClientRect(); return r.top >= 0 && r.bottom <= innerHeight; }))) {
      if (VIDEO) await page.waitForTimeout(900);
      await badge.evaluate(el => el.scrollIntoView({ block: "center", behavior: "instant" }));
      await page.waitForTimeout(300);
      await shot("3-verdict");
    }
    if (VIDEO) await page.waitForTimeout(1500); // hold the outcome on the recording

    const jev = state.calls.filter(k => k.path === "/api/alpha/decisions" && k.model === JEV);
    const shopped = state.calls.filter(k => k.path === "/api/alpha/decisions" && k.model !== JEV);
    const llm = state.calls.filter(k => k.path === "/api/v1/chat/completions");
    out.calls = { jev: jev.length, otherDecisionsModels: shopped.length, llm: llm.length, llmModel: (llm[0] || {}).model || "" };
    check(`${n}: Jev was asked exactly once, with the exact question and answer, on the dummy key`, jev.length === 1 && jev[0].exact && jev[0].auth, jev);
    check(`${n}: the split was not shopped to another decisions model`, shopped.length === 0, shopped);
    if (EXPECT === "refused") {
      // The incident release: no chain, so the split is a dead end.
      check(`${n}: POST /study/answer is the 503 of the incident`, resp.status() === 503, resp.status());
      check(`${n}: "Couldn't check that just now" is shown, no review`, !facts.review && /Couldn't check that just now/.test(facts.status), facts.status);
      check(`${n}: no escalation LLM was asked`, llm.length === 0, llm);
    } else if (sc.llm === "llm-down") {
      // The one documented dead end: the escalation fails too.
      check(`${n}: the escalation LLM was asked exactly once`, llm.length === 1 && llm[0].exact && llm[0].auth, llm);
      check(`${n}: with the escalation down, POST /study/answer is 503`, resp.status() === 503, resp.status());
      check(`${n}: "Couldn't check that just now" is shown, no review`, !facts.review && /Couldn't check that just now/.test(facts.status), facts.status);
      check(`${n}: the typed answer is still in the field`, facts.kept === ANSWER, facts.kept.length);
    } else {
      const [verdict, label] = VIEW[sc.llm];
      check(`${n}: the escalation LLM was asked exactly once, with the exact question and answer`, llm.length === 1 && llm[0].exact && llm[0].auth, llm);
      check(`${n}: POST /study/answer is not a 503`, resp.status() < 400, resp.status());
      check(`${n}: the post-answer review opens`, facts.review, facts.status);
      check(`${n}: the verdict is the LLM's (${LLM[sc.llm]})`, facts.verdict === verdict && label.test(facts.label), { verdict: facts.verdict, label: facts.label });
      check(`${n}: a one-line why`, facts.why.length > 0, facts.why);
      check(`${n}: never "Couldn't check that just now"`, !/Couldn't check that just now/.test(facts.status), facts.status);
      check(`${n}: Ask Why? is offered on the model-graded answer`, facts.askWhy);
    }
    // What the server logged for this submission (never the answer or key).
    const logs = fs.readFileSync(logPath, "utf8").slice(logFrom);
    const line = msg => logs.split("\n").filter(l => l.includes(`"msg":"${msg}"`)).map(l => { try { return JSON.parse(l); } catch { return {}; } });
    const chain = line("judge chain")[0];
    out.chain = chain ? { path: chain.path, escalated_because: chain.escalated_because, verdict_by: chain.verdict_by, answered: chain.answered } : null;
    if (EXPECT === "graded") {
      const want = sc.llm === "llm-down" ? "primary:uncertain,escalation:unavailable" : "primary:uncertain,escalation:answered";
      check(`${n}: the server logged the chain path ${want}`, !!chain && chain.path === want && chain.escalated_because === "uncertain", out.chain);
    }
    check(`${n}: the server log carries neither the answer nor the key`, !logs.includes("dig through your mind") && !logs.includes(DUMMY_KEY));
  } catch (e) {
    check(`${n}: flow completed`, false, String(e).slice(0, 300));
  }
  await context.close();
  await browser.close();
  if (VIDEO && MEDIA) {
    const vids = fs.readdirSync(videoDir).filter(f => f.endsWith(".webm"));
    if (vids[0]) {
      out.video = path.join(MEDIA, `${LABEL}-${sc.name}-walkthrough.webm`);
      fs.copyFileSync(path.join(videoDir, vids[0]), out.video);
    }
  }
  // The 503 page is itself a >= 400 response (and Chrome logs it as a
  // console error); when it is the expected outcome only it is allowed.
  const allowed = s => refused && /^503 POST \/study\/answer$/.test(s);
  const http4 = h.http.filter(s => !allowed(s));
  const cons = h.console.filter(s => !(refused && /503/.test(s)));
  const maxOverflow = Math.max(0, ...h.overflow.map(o => o.px));
  check(`${n}: 0 securitypolicyviolation events`, h.csp.length === 0, h.csp);
  check(`${n}: 0 console errors`, cons.length === 0, cons);
  check(`${n}: 0 page errors`, h.pageErrors.length === 0, h.pageErrors);
  check(`${n}: 0 failed requests`, h.failed.length === 0, h.failed);
  check(`${n}: 0 unexpected 4xx/5xx responses`, http4.length === 0, http4);
  check(`${n}: 0px horizontal overflow on every screen`, maxOverflow === 0, h.overflow);
  out.health = h;
  return out;
}

// ---- main ------------------------------------------------------------------

let standIn, scry, code = 1;
const runs = [];
try {
  standIn = await startStandIn();
  scry = await startScry(standIn);
  const hz = scry.health;
  console.log(`scry ${BIN} build_sha=${hz.build_sha} mode=${hz.mode} grading_chain=${JSON.stringify(hz.grading_chain || null)}`);
  check("Scry runs in test mode with the real Jev grader", hz.status === "ok" && hz.mode === "test" && hz.grading === true, { mode: hz.mode, grading: hz.grading, providers: (hz.providers || {}).grading });
  for (const sc of SCENARIOS) runs.push(await runScenario(sc, scry.base, scry.logPath));
  check("nothing but openrouter.ai was asked for through the proxy", state.foreign.length === 0, state.foreign);
  const failed = results.filter(r => !r.ok);
  const summary = { label: LABEL, bin: BIN, build_sha: hz.build_sha, expect: EXPECT, grading_chain: hz.grading_chain || null, headed: true, display: process.env.DISPLAY || "", runs, checks: results.length, failed: failed.map(f => f.name) };
  if (MEDIA) fs.writeFileSync(path.join(MEDIA, `${LABEL}-summary.json`), JSON.stringify(summary, null, 2));
  console.log(`\n${LABEL}: ${results.length - failed.length}/${results.length} checks passed`);
  for (const r of runs) console.log(`  ${r.scenario}: POST /study/answer ${r.answerStatus} verdict=${JSON.stringify(r.label)} status=${JSON.stringify(r.status)} calls=${JSON.stringify(r.calls)}`);
  code = failed.length === 0 ? 0 : 1;
} catch (e) {
  console.error(String(e && e.stack || e));
} finally {
  if (scry) scry.proc.kill();
  if (standIn) standIn.close();
  fs.rmSync(tmp, { recursive: true, force: true });
}
process.exit(code);
