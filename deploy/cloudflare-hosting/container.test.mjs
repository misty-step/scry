import assert from "node:assert/strict";
import test from "node:test";

import { createStartupOnlyFetch, fatalContainerError } from "./container-lifecycle.mjs";

const older = "scry-20260922T140000.000000000Z-00000000000000000000000000000000.scry-backup.zip";
const newer = "scry-20260922T150000.000000000Z-11111111111111111111111111111111.scry-backup.zip";

function deferred() {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return { promise, resolve };
}

function startupFixture({ status = "stopped", running = false, resolveSnapshotKey } = {}) {
  const baseEnvVars = { SCRY_MODE: "production", SCRY_CONTAINER_RESTORE_KEY: "" };
  const starts = [];
  const forwarded = [];
  const nativeStarts = [];
  let state = { status, lastChange: 1 };
  let containerRunning = running;
  let snapshotLookups = 0;
  const fetch = createStartupOnlyFetch({
    baseEnvVars,
    bootMode: "restore-required",
    getState: async () => ({ ...state }),
    isRunning: () => containerRunning,
    resolveSnapshotKey: async () => {
      snapshotLookups += 1;
      return resolveSnapshotKey ? resolveSnapshotKey(snapshotLookups) : older;
    },
    startAndWaitForPorts: async options => {
      if (!containerRunning) nativeStarts.push(options);
      starts.push(options);
      containerRunning = true;
      state = { status: "healthy", lastChange: state.lastChange + 1 };
    },
    forward: async request => {
      forwarded.push(request.url);
      return new Response("ok");
    },
  });
  return {
    baseEnvVars,
    fetch,
    nativeStarts,
    forwarded,
    get snapshotLookups() { return snapshotLookups; },
    starts,
    stop() {
      containerRunning = false;
      state = { status: "stopped", lastChange: state.lastChange + 1 };
    },
  };
}

test("container lifecycle errors remain fatal", () => {
  const failure = new Error("synthetic startup failure");
  const messages = [];
  assert.throws(
    () => fatalContainerError(failure, (...values) => messages.push(values)),
    error => error === failure,
  );
  assert.deepEqual(messages, [["[scry-container] error:", "Error: synthetic startup failure"]]);
});

test("warm requests stay available when R2 snapshot listing is unavailable", async () => {
  const fixture = startupFixture({
    status: "healthy",
    running: true,
    resolveSnapshotKey: async () => { throw new Error("R2 unavailable"); },
  });

  const response = await fixture.fetch(new Request("https://scry.example/review"));

  assert.equal(response.status, 200);
  assert.equal(fixture.snapshotLookups, 0);
  assert.equal(fixture.starts.length, 0);
  assert.deepEqual(fixture.forwarded, ["https://scry.example/review"]);
});

test("cold requests fail closed when the latest snapshot cannot be resolved", async () => {
  const fixture = startupFixture({
    resolveSnapshotKey: async () => { throw new Error("R2 unavailable"); },
  });

  await assert.rejects(
    fixture.fetch(new Request("https://scry.example/review")),
    /R2 unavailable/,
  );
  assert.equal(fixture.snapshotLookups, 1);
  assert.equal(fixture.starts.length, 0);
  assert.deepEqual(fixture.forwarded, []);
});

test("parallel cold requests share one snapshot lookup and one container start", async () => {
  const listing = deferred();
  const fixture = startupFixture({ resolveSnapshotKey: () => listing.promise });

  const first = fixture.fetch(new Request("https://scry.example/first"));
  const second = fixture.fetch(new Request("https://scry.example/second"));
  await Promise.resolve();
  assert.equal(fixture.snapshotLookups, 1);
  assert.equal(fixture.starts.length, 0);

  listing.resolve(older);
  const responses = await Promise.all([first, second]);

  assert.deepEqual(responses.map(response => response.status), [200, 200]);
  assert.equal(fixture.snapshotLookups, 1);
  assert.equal(fixture.starts.length, 1);
  assert.equal(
    fixture.starts[0].startOptions.envVars.SCRY_CONTAINER_RESTORE_KEY,
    older,
  );
  assert.deepEqual(fixture.forwarded.sort(), [
    "https://scry.example/first",
    "https://scry.example/second",
  ]);
});

test("a running container with stale stopped state rejoins before forwarding", async () => {
  const fixture = startupFixture({ status: "stopped", running: true });

  const response = await fixture.fetch(new Request("https://scry.example/review"));

  assert.equal(response.status, 200);
  assert.equal(fixture.nativeStarts.length, 0, "never start a second writer");
  assert.equal(fixture.snapshotLookups, 1);
  assert.equal(fixture.starts[0].startOptions.envVars.SCRY_CONTAINER_RESTORE_KEY, older);
  assert.deepEqual(fixture.forwarded, ["https://scry.example/review"]);
});

test("a process lost during stale-state recovery boots from the latest snapshot", async () => {
  let fixture;
  fixture = startupFixture({
    status: "stopped",
    running: true,
    resolveSnapshotKey: () => {
      fixture.stop();
      return newer;
    },
  });

  const response = await fixture.fetch(new Request("https://scry.example/review"));

  assert.equal(response.status, 200);
  assert.equal(fixture.nativeStarts.length, 1);
  assert.equal(fixture.nativeStarts[0].startOptions.envVars.SCRY_CONTAINER_RESTORE_KEY, newer);
});

test("a stopped container refreshes the latest key without retaining the prior startup key", async () => {
  const keys = [older, newer];
  const fixture = startupFixture({ resolveSnapshotKey: call => keys[call - 1] });

  await fixture.fetch(new Request("https://scry.example/first-boot"));
  fixture.stop();
  await fixture.fetch(new Request("https://scry.example/second-boot"));

  assert.equal(fixture.snapshotLookups, 2);
  assert.deepEqual(
    fixture.starts.map(start => start.startOptions.envVars.SCRY_CONTAINER_RESTORE_KEY),
    [older, newer],
  );
  assert.equal(fixture.baseEnvVars.SCRY_CONTAINER_RESTORE_KEY, "");
});

test("a stop between the readiness check and forwarding cannot reuse a prior startup key", async () => {
  const baseEnvVars = { SCRY_MODE: "production", SCRY_CONTAINER_RESTORE_KEY: "" };
  let state = { status: "healthy", lastChange: 1 };
  let running = true;
  let forwards = 0;
  let snapshotLookups = 0;
  const starts = [];
  const fetch = createStartupOnlyFetch({
    baseEnvVars,
    bootMode: "restore-required",
    getState: async () => ({ ...state }),
    isRunning: () => running,
    resolveSnapshotKey: async () => {
      snapshotLookups += 1;
      return newer;
    },
    startAndWaitForPorts: async options => {
      starts.push(options);
      running = true;
      state = { status: "healthy", lastChange: state.lastChange + 1 };
    },
    forward: async () => {
      forwards += 1;
      if (forwards === 1) {
        running = false;
        state = { status: "stopped", lastChange: state.lastChange + 1 };
        assert.equal(baseEnvVars.SCRY_CONTAINER_RESTORE_KEY, "");
        throw new Error("automatic restart has no restore key");
      }
      return new Response("ok");
    },
  });

  await assert.rejects(
    fetch(new Request("https://scry.example/racing-stop")),
    /automatic restart has no restore key/,
  );
  assert.equal(snapshotLookups, 0);
  assert.equal(starts.length, 0);

  const response = await fetch(new Request("https://scry.example/retry-after-stop"));
  assert.equal(response.status, 200);
  assert.equal(snapshotLookups, 1);
  assert.equal(
    starts[0].startOptions.envVars.SCRY_CONTAINER_RESTORE_KEY,
    newer,
  );
  assert.equal(baseEnvVars.SCRY_CONTAINER_RESTORE_KEY, "");
});
