#!/usr/bin/env node
// verify-slice75-edges.mjs: real-path checks of Slice 75 (Strugglometer)
// edges that scripts/verify-slice75-strugglometer.mjs does not walk:
//   - SCRY_TEST_BRIDGES=fail: the preview says "Scry couldn't build bridge
//     cards just now. Nothing was added. Try again in a moment." and writes
//     no item_bridges row;
//   - an exact text with 3 lapses shows no cue, and a crafted draft POST for
//     it is refused;
//   - Remove in the preview works in place and renumbers; the last card
//     cannot be removed (Cancel instead); nothing is stored while previewing;
//   - trashing the held card's idea pauses the hold, a stale "Release hold"
//     sent while it is in Trash changes nothing, restoring it restores the
//     hold, and "Release hold" then ends every row as released with the
//     bridges kept as cards.
// Each part starts its own SCRY_MODE=test server (the --bin binary) on a
// fresh SQLite file with a minimal environment (no keys), signs up a
// disposable learner in headless system Chrome (1440x900) and seeds two
// cards' FSRS lapses with node:sqlite.
//
//   node scripts/verify-slice75-edges.mjs --bin ./bin/scry [--shots DIR] [--port 18811]
import fs from "node:fs"; import path from "node:path"; import os from "node:os"; import crypto from "node:crypto";
import { spawn } from "node:child_process"; import { DatabaseSync } from "node:sqlite";
import { chromium } from "playwright-core";
const argv = process.argv.slice(2); const flag = (n, d) => { const i = argv.indexOf(n); return i >= 0 ? argv[i + 1] : d; };
const BIN = path.resolve(flag("--bin", "./bin/scry")); const SHOTS = flag("--shots", ""); const PORT = Number(flag("--port", "18811"));
if (!fs.existsSync(BIN)) { console.error("no binary at " + BIN); process.exit(2); }
if (SHOTS) fs.mkdirSync(SHOTS, { recursive: true });
const servers = [];
const results = []; const check = (n, ok, d) => { results.push({ n, ok }); console.log(`${ok ? "PASS" : "FAIL"} ${n}${d !== undefined ? "  " + JSON.stringify(d) : ""}`); };
const DAY = 86400, now = () => Math.floor(Date.now() / 1000);
async function start(port, extra) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "s75probe-")); const db = path.join(dir, "scry.db");
  const log = fs.openSync(path.join(dir, "server.log"), "w");
  const proc = spawn(BIN, [], { env: { PATH: process.env.PATH, HOME: dir, TMPDIR: dir, SCRY_MODE: "test", SCRY_DB: db, PORT: String(port), BACKUP_DIR: path.join(dir, "b"), ...extra }, stdio: ["ignore", log, log], cwd: dir });
  const url = `http://127.0.0.1:${port}`; const srv = { proc, dir, db, url }; servers.push(srv);
  for (let i = 0; i < 100; i++) { try { if ((await fetch(url + "/healthz")).ok) return srv; } catch {} await new Promise(r => setTimeout(r, 100)); }
  throw new Error("no health");
}
const stop = s => s.proc.exitCode !== null || s.proc.signalCode ? Promise.resolve() : new Promise(r => { s.proc.once("exit", r); s.proc.kill("SIGTERM"); setTimeout(r, 5000); });
async function signUpAndSeed(page, s) {
  const email = `s75-edges-${Date.now()}@example.com`;
  await page.goto(s.url + "/signup"); await page.fill("#email", email); await page.fill("#password", crypto.randomBytes(12).toString("hex"));
  await Promise.all([page.waitForURL(u => new URL(u).pathname !== "/signup"), page.locator("main form button.btn-accent").first().click()]);
  await page.goto(s.url + "/shelf"); await page.locator("form[data-add]").first().locator("button").click();
  await page.waitForSelector("[data-shelf-bar]:not([hidden])", { timeout: 10000 }).catch(() => {});
  const db = new DatabaseSync(s.db); db.exec("PRAGMA busy_timeout = 5000");
  let u; for (let i = 0; i < 100 && !(u = db.prepare("SELECT id FROM users WHERE email = ?").get(email)); i++) await new Promise(r => setTimeout(r, 100));
  if (!u) throw new Error("no user; at " + page.url() + " " + (await page.locator("main").innerText()).slice(0, 300));
  for (let i = 0; i < 100 && !db.prepare("SELECT COUNT(*) n FROM items WHERE user_id = ? AND prompt = 'NATO alphabet: C'").get(u.id).n; i++) await new Promise(r => setTimeout(r, 100));
  const it = Object.fromEntries(db.prepare("SELECT id, prompt, idea_id FROM items WHERE user_id = ?").all(u.id).map(i => [i.prompt, i]));
  const A = it["NATO alphabet: A"], C = it["NATO alphabet: C"];
  db.prepare("UPDATE items SET due_at = ? WHERE user_id = ?").run(now() + 30 * DAY, u.id);
  for (const X of [A, C]) db.prepare("UPDATE items SET reps = 6, lapses = 3, stability = 1.4, difficulty = 7.8, last_review = ? WHERE id = ?").run(now() - 5 * DAY, X.id);
  return { db, uid: u.id, A, C };
}
const active = (db, id) => db.prepare("SELECT COUNT(*) n FROM item_bridges WHERE parent_item_id = ? AND ended_at = 0").get(id).n;
const rows = (db, id) => db.prepare("SELECT COUNT(*) n FROM item_bridges WHERE parent_item_id = ?").get(id).n;
const browser = await chromium.launch({ executablePath: "/usr/bin/google-chrome" });
const ctxOpts = { viewport: { width: 1440, height: 900 }, timezoneId: "America/Chicago" };
try {
  // 1. Failure state (SCRY_TEST_BRIDGES=fail).
  { const s = await start(PORT, { SCRY_TEST_BRIDGES: "fail" }); const ctx = await browser.newContext(ctxOpts); const page = await ctx.newPage();
    const { db, A } = await signUpAndSeed(page, s);
    await page.goto(`${s.url}/library/items/${A.id}`);
    await Promise.all([page.waitForNavigation(), page.locator("[data-build-bridges]").click()]);
    const t = (await page.locator("[data-bridge-problem]").innerText().catch(() => "")).trim();
    check("fail mode: failure says so in words", t.includes("Scry couldn't build bridge cards just now. Nothing was added. Try again in a moment."), t);
    check("fail mode: nothing added (0 item_bridges rows)", rows(db, A.id) === 0);
    if (SHOTS) await page.screenshot({ path: path.join(SHOTS, "s75-edges-desktop-build-failure.png") });
    db.close(); await ctx.close(); await stop(s); }
  // 2. Normal test mode.
  { const s = await start(PORT + 1, {}); const ctx = await browser.newContext(ctxOpts); const page = await ctx.newPage();
    const { db, A, C } = await signUpAndSeed(page, s);
    // Exact texts are not offered bridges.
    db.prepare("UPDATE items SET exact = 1 WHERE id = ?").run(C.id);
    await page.goto(`${s.url}/library/items/${C.id}`);
    check("exact text with 3 lapses: no cue, no Build", (await page.locator("[data-struggle], [data-build-bridges]").count()) === 0);
    const csrf = await page.locator("input[name=csrf]").first().inputValue();
    const r = await page.request.post(`${s.url}/library/items/${C.id}/bridges/draft`, { form: { csrf, from: "item" }, maxRedirects: 0 });
    check("crafted draft POST for an exact text is refused (no preview, no rows)", !(await r.text()).includes("data-bridge-form") && rows(db, C.id) === 0, r.status());
    // Preview: remove down to the last card.
    await page.goto(`${s.url}/library/items/${A.id}`);
    await Promise.all([page.waitForNavigation(), page.locator("[data-build-bridges]").click()]);
    check("preview: 3 drafted bridges", (await page.locator("[data-bridge-card]").count()) === 3);
    await page.locator("[data-bridge-remove]").nth(0).click(); await page.waitForFunction(() => document.querySelectorAll("[data-bridge-card]").length === 2);
    const nums = await page.locator("[data-bridge-num]").allInnerTexts();
    check("remove in place renumbers the rest", nums.join("|") === "Bridge 1|Bridge 2", nums);
    await page.locator("[data-bridge-remove]").nth(0).click(); await page.waitForFunction(() => document.querySelectorAll("[data-bridge-card]").length === 1);
    check("the last card cannot be removed (no Remove, Cancel instead)", (await page.locator("[data-bridge-card]").count()) === 1 && (await page.locator("[data-bridge-remove]:visible").count()) === 0 && (await page.getByRole("link", { name: "Cancel" }).count()) === 1);
    if (SHOTS) await page.screenshot({ path: path.join(SHOTS, "s75-edges-desktop-last-bridge-no-remove.png") });
    check("nothing added while previewing", rows(db, A.id) === 0);
    // Fresh preview, accept all 3.
    await page.goto(`${s.url}/library/items/${A.id}`);
    await Promise.all([page.waitForNavigation(), page.locator("[data-build-bridges]").click()]);
    await Promise.all([page.waitForNavigation(), page.locator("[data-bridge-accept]").click()]);
    check("accepted: held by 3", active(db, A.id) === 3 && (await page.locator("[data-hold-line]").innerText()).includes("Held by 3 bridges"));
    const releaseCsrf = await page.locator("form[action$='/bridges/release'] input[name=csrf]").inputValue();
    // Trash the idea: hold pauses.
    await page.goto(`${s.url}/library/ideas/${A.idea_id}`);
    page.once("dialog", d => d.accept());
    await Promise.all([page.waitForNavigation(), page.locator(`form[action='/library/ideas/${A.idea_id}/remove'] button`).click()]);
    const trashed = db.prepare("SELECT deleted_at FROM ideas WHERE id = ?").get(A.idea_id).deleted_at > 0;
    check("idea in Trash", trashed);
    // Stale Release hold after the idea went to Trash changes nothing.
    const rr = await page.request.post(`${s.url}/library/items/${A.id}/bridges/release`, { form: { csrf: releaseCsrf, from: "item" }, maxRedirects: 0 });
    check("stale Release hold while trashed: rows unchanged (3 active)", active(db, A.id) === 3, { status: rr.status(), active: active(db, A.id) });
    // Restore: hold comes back.
    await page.goto(`${s.url}/settings`);
    await Promise.all([page.waitForNavigation(), page.locator(`form[action^='/settings/restore/'] button`).first().click()]);
    await page.goto(`${s.url}/library/items/${A.id}`);
    const held = (await page.locator("[data-hold-line]").innerText().catch(() => "")).trim();
    check("restored from Trash: hold restored (Held by 3 bridges)", held === "Held by 3 bridges", held);
    if (SHOTS) await page.screenshot({ path: path.join(SHOTS, "s75-edges-desktop-held-after-restore.png") });
    // Release by hand now ends the hold; bridges stay.
    await Promise.all([page.waitForNavigation(), page.locator("[data-release-hold]").click()]);
    const reasons = db.prepare("SELECT end_reason FROM item_bridges WHERE parent_item_id = ?").all(A.id).map(r => r.end_reason);
    check("Release hold ends all rows as released; bridges stay as cards", active(db, A.id) === 0 && reasons.every(r => r === "released") && db.prepare("SELECT COUNT(*) n FROM items WHERE source = 'bridge' AND deleted_at = 0").get().n === 3, reasons);
    db.close(); await ctx.close(); await stop(s); }
} catch (e) { check("probe completed", false, String(e.stack || e).slice(0, 500)); }
await browser.close();
for (const s of servers) { await stop(s); fs.rmSync(s.dir, { recursive: true, force: true }); }
const f = results.filter(r => !r.ok).length; console.log(`${results.length - f} passed, ${f} failed`); process.exit(f ? 1 : 0);
