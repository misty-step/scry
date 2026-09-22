import assert from "node:assert/strict";
import test from "node:test";

import { runBackupCycle, stopAfterBackup } from "./backup-cycle.mjs";

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

test("idle stop requires a verified backup and keeps the writer on failure", async () => {
  let stopped = 0;
  const messages = [];
  const stop = async () => { stopped++; };
  const log = message => messages.push(message);
  await stopAfterBackup({ backup: async () => ({ state: "backed_up" }), stop, log });
  assert.equal(stopped, 1);
  await stopAfterBackup({ backup: async () => { throw new Error("remote unavailable"); }, stop, log });
  assert.equal(stopped, 1);
  assert.deepEqual(messages, ["[scry-recovery] idle stop deferred: remote backup failed"]);
});