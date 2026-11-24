#!/usr/bin/env node
// verify-inbox-purpose.mjs: DESIGN.md, Inbox: the purpose line "Cards
// suggested by external scripts or tools wait here until you approve them."
// is always on its own row beneath the title (empty or not), and "Approve
// all" is a text action at the right of the title row.
//
//   node scripts/verify-inbox-purpose.mjs --bin ./bin/scry [--shots DIR] [--port 18830]
//
// Starts the binary in SCRY_MODE=test on a fresh SQLite file with a minimal
// environment (no keys), signs up a disposable learner per viewport in
// headless system Chrome (desktop 1440x900, phone 390x844 touch), checks the
// empty Inbox, then adds two pending suggestions with node:sqlite and checks
// again. Geometry is read from the rendered page, not the style sheet.
import fs from "node:fs"; import path from "node:path"; import os from "node:os"; import crypto from "node:crypto";
import { spawn } from "node:child_process"; import { DatabaseSync } from "node:sqlite";
import { chromium } from "playwright-core";
const argv = process.argv.slice(2); const flag = (n, d) => { const i = argv.indexOf(n); return i >= 0 ? argv[i + 1] : d; };
const BIN = path.resolve(flag("--bin", "./bin/scry")); const SHOTS = flag("--shots", ""); const PORT = Number(flag("--port", "18830"));
if (!fs.existsSync(BIN)) { console.error("no binary at " + BIN); process.exit(2); }
if (SHOTS) fs.mkdirSync(SHOTS, { recursive: true });
const dir = fs.mkdtempSync(path.join(os.tmpdir(), "inbox-purpose-")); const dbFile = path.join(dir, "scry.db");
const log = fs.openSync(path.join(dir, "server.log"), "w");
const proc = spawn(BIN, [], { env: { PATH: process.env.PATH, HOME: dir, TMPDIR: dir, SCRY_MODE: "test", SCRY_DB: dbFile, PORT: String(PORT), BACKUP_DIR: path.join(dir, "backups") }, stdio: ["ignore", log, log], cwd: dir });
const url = `http://127.0.0.1:${PORT}`; let passed = 0, failed = 0;
const check = (n, ok, d) => { ok ? passed++ : failed++; console.log(`${ok ? "PASS" : "FAIL"} ${n}  ${JSON.stringify(d)}`); };
const geometry = page => page.evaluate(() => {
  const r = s => { const e = document.querySelector(s); return e ? e.getBoundingClientRect() : null; };
  const h = r(".inbox h1"), p = r(".list-purpose"), c = r(".list-count"), a = r(".list-action");
  return { titleTop: h.top, titleBottom: h.bottom, titleLeft: h.left, countBottom: c.bottom, purposeTop: p.top, purposeLeft: p.left,
    action: a && { top: a.top, bottom: a.bottom, right: a.right }, headRight: r(".list-head").right,
    scrollX: document.documentElement.scrollWidth <= innerWidth };
});
let browser;
try {
  let healthy = false;
  for (let i = 0; i < 100 && !healthy; i++) { try { healthy = (await fetch(url + "/healthz")).ok; } catch {} if (!healthy) await new Promise(r => setTimeout(r, 100)); }
  check("isolated test-mode server is healthy", healthy, { url: "127.0.0.1:" + PORT });
  browser = await chromium.launch({ executablePath: process.env.CHROME_BIN || "/usr/bin/google-chrome" });
  for (const [name, o] of [["desktop", { viewport: { width: 1440, height: 900 } }], ["phone", { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, deviceScaleFactor: 2 }]]) {
    const ctx = await browser.newContext(o); const page = await ctx.newPage();
    const email = `inbox-${name}-${Date.now()}@example.com`;
    await page.goto(url + "/signup"); await page.fill("#email", email); await page.fill("#password", crypto.randomBytes(12).toString("hex"));
    await Promise.all([page.waitForURL(u => new URL(u).pathname !== "/signup"), page.locator("main form button.btn-accent").first().click()]);
    await page.goto(url + "/inbox");
    let g = await geometry(page);
    const text = await page.locator(".list-purpose").innerText();
    check(`${name} empty Inbox: "Nothing waiting." and the purpose line`, (await page.locator("main").innerText()).includes("Nothing waiting.") && text === "Cards suggested by external scripts or tools wait here until you approve them.", text);
    check(`${name} empty Inbox: purpose on its own row beneath the title, aligned with it`, g.purposeTop >= Math.max(g.titleBottom, g.countBottom) - 1 && Math.abs(g.purposeLeft - g.titleLeft) < 2 && g.scrollX, g);
    if (SHOTS) await page.screenshot({ path: path.join(SHOTS, `inbox-empty-purpose-${name}.png`) });
    const db = new DatabaseSync(dbFile); db.exec("PRAGMA busy_timeout = 5000");
    const uid = db.prepare("SELECT id FROM users WHERE email = ?").get(email).id;
    const add = db.prepare("INSERT INTO suggestions (user_id, agent, source, idea_title, prompt, answer, created_at) VALUES (?, 'reading agent', 'The Federalist Papers', 'Federalist No. 10', ?, ?, ?)");
    add.run(uid, "What does Madison call the violence of faction?", "A dangerous vice of popular governments", Math.floor(Date.now() / 1000));
    add.run(uid, "How does a large republic control faction?", "By extending the sphere, taking in more parties and interests", Math.floor(Date.now() / 1000));
    db.close();
    await page.goto(url + "/inbox");
    g = await geometry(page);
    check(`${name} Inbox with 2 suggestions: purpose on its own row beneath the title`, g.purposeTop >= Math.max(g.titleBottom, g.countBottom) - 1 && Math.abs(g.purposeLeft - g.titleLeft) < 2 && g.scrollX, g);
    check(`${name} Inbox with 2 suggestions: "Approve all" on the title row at the right`, !!g.action && g.action.top < g.purposeTop && g.action.bottom > g.titleTop && Math.abs(g.action.right - g.headRight) < 2, g.action);
    if (SHOTS) await page.screenshot({ path: path.join(SHOTS, `inbox-suggestions-purpose-${name}.png`) });
    await ctx.close();
  }
} catch (e) { check("walkthrough completed", false, String(e.stack || e).slice(0, 500)); }
finally {
  if (browser) await browser.close().catch(() => {});
  await new Promise(r => { proc.once("exit", r); proc.kill("SIGTERM"); setTimeout(r, 5000); });
  fs.rmSync(dir, { recursive: true, force: true });
}
console.log(`${passed} passed, ${failed} failed`); process.exit(failed ? 1 : 0);
