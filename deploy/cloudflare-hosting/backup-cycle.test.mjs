import assert from "node:assert/strict";
import test from "node:test";

import { createActivityGate, failureReason, runBackupCycle, stopAfterBackup, stopWhenIdle } from "./backup-cycle.mjs";

const key = "scry-20260922T210000.000000000Z-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.scry-backup.zip";
const sha256 = "b".repeat(64);
const object = { size: 500, customMetadata: { sha256 } };
const now = Date.parse("2026-09-22T22:00:00Z");

test("awake writer runs the exact remote-verified backup and R2 identity check", async () => {
  let commands = 0;
  const result = await runBackupCycle({
    running: () => true,
    execute: async () => {
      commands++;
      return { exitCode: 0, stdout: JSON.stringify({ remote: true, remote_key: key, sha256, bytes: 500 }) };
    },
    head: async found => { assert.equal(found, key); return object; },
    latest: () => { throw new Error("must not list for an active backup"); },
  });
  assert.equal(commands, 1);
  assert.deepEqual(result, { state: "backed_up", key, sha256, bytes: 500 });
});

test("idle with verified snapshot does not start or exec a container", async () => {
  const base = {
    running: () => false, latest: async () => key, head: async () => object,
    execute: () => { throw new Error("must not execute on a stopped container"); },
  };
  assert.deepEqual(await runBackupCycle({ ...base, now }), { state: "idle", key, ageSeconds: 3600 });
  assert.deepEqual(await runBackupCycle({ ...base, now: now + 86400 * 1000 }), {
    state: "idle_stale", key, ageSeconds: 90000,
  });
});

test("failed command, unverified result, and conflicting R2 metadata fail closed", async () => {
  const base = { running: () => true, head: async () => object };
  for (const execute of [
    async () => ({ exitCode: 1, stdout: "private error" }),
    async () => ({ exitCode: 0, stdout: JSON.stringify({ remote: false, remote_key: key, sha256, bytes: 500 }) }),
    async () => ({ exitCode: 0, stdout: "not json" }),
  ]) await assert.rejects(runBackupCycle({ ...base, execute }), /backup/);
  await assert.rejects(runBackupCycle({ ...base,
    execute: async () => ({ exitCode: 0, stdout: JSON.stringify({ remote: true, remote_key: key, sha256, bytes: 500 }) }),
    head: async () => ({ ...object, size: 499 }),
  }), /differs/);
});

test("idle missing or invalid recovery key fails rather than claiming no change", async () => {
  await assert.rejects(runBackupCycle({
    running: () => false, latest: async () => { throw new Error("no snapshot"); },
    head: async () => object,
  }), /no snapshot/);
  await assert.rejects(runBackupCycle({
    running: () => false, latest: async () => "invalid", head: async () => object,
  }), /invalid/);
});

test("failed command reports its exit and last stderr line without secrets", async () => {
  const capability = "c".repeat(48);
  const stderr = [
    "progress noise",
    `scry: backup upload failed Bearer ${capability} at https://gateway.example/${key} token ${capability}`,
    "",
  ].join("\n");
  const failure = await runBackupCycle({
    running: () => true,
    execute: async () => ({ exitCode: 1, stdout: "{\"path\":\"/var/lib/scry/private\"}", stderr }),
    head: async () => object,
  }).then(() => assert.fail("a failed command must reject"), error => error);
  assert.match(failure.message, /^scheduled remote backup command failed \(exit 1\): scry: backup upload failed /);
  assert.match(failure.message, /<url>/);
  for (const leaked of [capability, "gateway.example", "/var/lib/scry/private", "progress noise"]) {
    assert.equal(failure.message.includes(leaked), false, `${leaked} must not reach logs`);
  }
  assert.equal(failureReason(""), "no error output");
  assert.equal(failureReason("x".repeat(500)).length <= 240, true);
  assert.equal(failureReason(`open ${"/a".repeat(200)}`).length <= 240, true);
});

test("idle stop requires a verified backup and keeps the writer on failure", async () => {
  let stopped = 0;
  const messages = [];
  const stop = async () => { stopped++; };
  const log = (...values) => messages.push(values);
  await stopAfterBackup({ backup: async () => ({ state: "backed_up" }), stop, log });
  assert.equal(stopped, 1);
  await stopAfterBackup({ backup: async () => { throw new Error("remote unavailable"); }, stop, log });
  assert.equal(stopped, 1);
  assert.deepEqual(messages, [["[scry-recovery] idle stop deferred: remote backup failed", { reason: "remote unavailable" }]]);
  await stopAfterBackup({ backup: async () => ({ state: "backed_up" }), stop, log, canStop: () => false });
  assert.equal(stopped, 1, "a request during the backup must cancel the idle stop");
});

const verified = async () => ({ state: "backed_up" });

test("a fresh instance with no requests stops after a verified idle backup", async () => {
  // Every deploy creates a fresh Durable Object before any owner request.
  const gate = createActivityGate();
  let stopped = 0;
  await stopWhenIdle({
    gate, backup: verified, stop: async () => { stopped++; },
    log: () => assert.fail("a verified backup must not log a failure"),
  });
  assert.equal(stopped, 1);
});

test("owner requests during or across the idle backup keep the writer awake", async () => {
  const gate = createActivityGate();
  let stopped = 0;
  const stop = async () => { stopped++; };

  await stopWhenIdle({ gate, stop, backup: async () => {
    await gate.track(async () => "served while the backup ran");
    return { state: "backed_up" };
  } });
  assert.equal(stopped, 0, "a request that began and ended during the backup");

  let release;
  const inflight = gate.track(() => new Promise(done => { release = done; }));
  await stopWhenIdle({ gate, stop, backup: verified });
  assert.equal(stopped, 0, "a request still in flight when the backup ends");
  release("ok");
  assert.equal(await inflight, "ok");

  await assert.rejects(gate.track(async () => { throw new Error("upstream failed"); }), /upstream failed/);
  await stopWhenIdle({ gate, stop, backup: verified });
  assert.equal(stopped, 1, "a quiet gate stops again, even after a failed request");
});

test("a failed idle backup logs its redacted reason and keeps the writer", async () => {
  const gate = createActivityGate();
  const messages = [];
  let stopped = 0;
  await stopWhenIdle({
    gate, stop: async () => { stopped++; }, log: (...values) => messages.push(values),
    backup: async () => { throw new Error(`upload failed at https://gateway.example/${key}`); },
  });
  assert.equal(stopped, 0);
  assert.deepEqual(messages, [["[scry-recovery] idle stop deferred: remote backup failed", { reason: "upload failed at <url>" }]]);
});

test("the default idle log keeps the failure reason for Workers Logs", async () => {
  const original = console.error;
  const seen = [];
  console.error = (...values) => seen.push(values);
  try {
    await stopWhenIdle({
      gate: createActivityGate(),
      stop: async () => assert.fail("a failed backup must not stop the writer"),
      backup: async () => { throw new Error("remote unavailable"); },
    });
  } finally {
    console.error = original;
  }
  assert.deepEqual(seen, [["[scry-recovery] idle stop deferred: remote backup failed", { reason: "remote unavailable" }]]);
});