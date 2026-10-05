import assert from "node:assert/strict";
import { test } from "node:test";
import { mkdtemp, mkdir, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { checker, cloudflare, parseArtifact, parseCredentials, release } from "./release.mjs";

const id = (n) => `00000000-0000-4000-8000-${String(n).padStart(12, "0")}`;
const oldImage = `registry.cloudflare.com/test/scry@sha256:${"a".repeat(64)}`;
const newImage = `registry.cloudflare.com/test/scry@sha256:${"b".repeat(64)}`;
const artifact = parseArtifact({
  format: "scry-release-artifact-v1", environment: "staging", data_class: "synthetic",
  version_id: id(2), container_application_id: id(10), image: newImage,
  revision: "c".repeat(40), binary_sha256: "d".repeat(64),
  evidence: "https://example.test/candidate", compatibility_evidence: "https://example.test/compatibility",
  previous: { deployment_id: id(100), version_id: id(1), image: oldImage },
  rollback_compatible: true, database_rollback: false,
});
const credentials = parseCredentials([
  "SCRY_RELEASE_CF_TOKEN=dedicated-test-token", `SCRY_RELEASE_ACCOUNT_ID=${"e".repeat(32)}`,
  "SCRY_PROBE_TOKEN=synthetic-probe-token", "SCRY_ACCESS_JWT=synthetic-owner-jwt",
].join("\n"));
const pass = { status: "PASS", reason: "health_ready_private_map" };
const fail = { status: "FAIL", reason: "journey_assertion_failure" };
const unavailable = { status: "UNVERIFIED", reason: "checker_inaccessible" };

function fixture(options = {}) {
  let current = { id: id(100), versions: [{ version_id: id(1), percentage: 100 }] };
  let image = oldImage;
  let elapsed = 0;
  let checks = 0;
  const writes = [];
  const reads = [];
  const receipts = [];
  const rollouts = new Map();
  function newer() {
    current = { id: id(999), versions: [{ version_id: id(3), percentage: 100 }] };
  }
  const fetcher = async (url, init) => {
    assert.equal(init.headers.Authorization, "Bearer dedicated-test-token");
    assert.equal(init.redirect, "error");
    const path = new URL(url).pathname.replace(/^\/client\/v4\/accounts\/[a-f0-9]+/, "");
    const body = init.body ? JSON.parse(init.body) : undefined;
    if (init.method !== "GET") writes.push({ method: init.method, path, body });
    else reads.push(path);
    if (options.apiUnavailable?.(path, init, body)) throw new Error("secret-provider-message");
    let result;
    if (path.endsWith("/deployments")) {
      if (init.method === "POST") {
        current = { id: id(100 + writes.length), versions: body.versions };
        result = current;
      } else {
        if (options.newerBeforeRollback && receipts.at(-1)?.rollback.status === "ATTEMPTED") newer();
        result = { deployments: [current] };
      }
    } else if (path.endsWith(`/applications/${id(10)}`)) {
      if (init.method === "PATCH") image = body.configuration.image;
      result = { id: id(10), name: "scry-app-container-staging", scheduling_policy: "default", configuration: { image, instance_type: "lite" } };
    } else if (path.endsWith("/rollouts")) {
      const rolloutId = id(200 + writes.length);
      result = { id: rolloutId, status: "completed", target_configuration: body.target_configuration };
      rollouts.set(rolloutId, result);
    } else if (path.includes("/rollouts/")) {
      result = rollouts.get(path.split("/").at(-1));
      result = { ...result, status: options.rolloutPending ? "progressing" : "completed" };
    } else {
      assert.fail(`unexpected Cloudflare path ${path}`);
    }
    return Response.json({ success: true, result });
  };
  return {
    writes, reads, receipts,
    async run() {
      return release(artifact, {
        api: cloudflare(credentials, fetcher),
        check: async () => {
          checks++;
          if (options.newerAtCheck === checks) newer();
          return options.journeys?.[checks - 1] ?? pass;
        },
        now: () => elapsed,
        wait: async (ms) => { assert.ok(ms <= 60000); elapsed += ms; },
        save: async (value) => { receipts.push(JSON.parse(JSON.stringify(value))); },
      });
    },
  };
}

test("checks expected version and private journey immediately and at 2.5 and 5 minutes", async () => {
  const f = fixture();
  const result = await f.run();
  assert.equal(result.status, "PASS");
  assert.equal(result.deployment.version_id, id(2));
  assert.ok(result.deployment.deployment_id);
  assert.deepEqual(result.observation.checks.map((r) => r.offset_ms), [0, 150000, 300000]);
  assert.deepEqual(result.observation.checks.map((r) => r.status), ["PASS", "PASS", "PASS"]);
  assert.equal(result.rollback.attempts, 0);
  assert.equal(f.writes.filter((r) => r.path.endsWith("/deployments")).length, 1);
});

test("confirmed failure rolls back Worker and image exactly once, then proves recovery", async () => {
  const f = fixture({ journeys: [pass, fail, fail, pass] });
  const result = await f.run();
  assert.equal(result.status, "ROLLED_BACK");
  assert.equal(result.observation.status, "FAIL");
  assert.equal(result.rollback.attempts, 1);
  assert.equal(result.rollback.version_id, id(1));
  assert.equal(result.recovery.status, "PASS");
  assert.equal(result.database_rollback, false);
  assert.deepEqual(f.writes.filter((r) => r.path.endsWith("/deployments")).map((r) => r.body.versions), [
    [{ version_id: id(2), percentage: 100 }], [{ version_id: id(1), percentage: 100 }],
  ]);
  assert.deepEqual(f.writes.filter((r) => r.method === "PATCH").map((r) => r.body), [
    { configuration: { image: newImage, instance_type: "lite" } }, { configuration: { image: oldImage, instance_type: "lite" } },
  ]);
  assert.deepEqual(f.writes.filter((r) => r.path.endsWith("/rollouts")).map((r) => r.body.step_percentage), [100, 100]);
  assert.ok(f.receipts.some((r) => r.rollback.status === "ATTEMPTED" && r.rollback.attempts === 1));
});

test("newer deployment during confirmation prevents rollback", async () => {
  const f = fixture({ journeys: [pass, fail, fail], newerAtCheck: 3 });
  const result = await f.run();
  assert.equal(result.status, "SUPERSEDED");
  assert.equal(result.rollback.attempts, 0);
  assert.equal(f.writes.filter((r) => r.path.endsWith("/deployments")).length, 1);
});

test("newer deployment at the final rollback guard prevents rollback", async () => {
  const f = fixture({ journeys: [pass, fail, fail], newerBeforeRollback: true });
  const result = await f.run();
  assert.equal(result.status, "SUPERSEDED");
  assert.equal(result.rollback.attempts, 0);
  assert.equal(f.writes.filter((r) => r.path.endsWith("/deployments")).length, 1);
});

test("one inaccessible observation remains unverified even if later checks pass", async () => {
  const f = fixture({ journeys: [pass, unavailable, pass, pass] });
  const result = await f.run();
  assert.equal(result.status, "UNVERIFIED");
  assert.equal(result.observation.status, "UNVERIFIED");
  assert.equal(result.rollback.attempts, 0);
  assert.equal(result.observation.checks.length, 3);
});

test("inaccessible confirmation cannot confirm a failure or trigger rollback", async () => {
  const f = fixture({ journeys: [pass, fail, unavailable] });
  const result = await f.run();
  assert.equal(result.status, "UNVERIFIED");
  assert.equal(result.rollback.attempts, 0);
});

test("inaccessible baseline stops before any deployment", async () => {
  const f = fixture({ journeys: [unavailable] });
  const result = await f.run();
  assert.equal(result.status, "UNVERIFIED");
  assert.equal(result.deployment.status, "NOT_ATTEMPTED");
  assert.deepEqual(f.writes, []);
});

test("a single failure followed by success is unverified and does not roll back", async () => {
  const f = fixture({ journeys: [pass, fail, pass, pass, pass] });
  const result = await f.run();
  assert.equal(result.status, "UNVERIFIED");
  assert.equal(result.rollback.attempts, 0);
});

test("incomplete container rollout cannot be green", async () => {
  const result = await fixture({ rolloutPending: true }).run();
  assert.equal(result.status, "UNVERIFIED");
  assert.equal(result.observation.checks[2].reason, "container_rollout_incomplete");
});

test("unknown rollback POST outcome is never retried", async () => {
  const f = fixture({ journeys: [pass, fail, fail], apiUnavailable: (path, init, body) =>
    init.method === "POST" && body?.versions?.[0].version_id === id(1) });
  const result = await f.run();
  assert.equal(result.status, "UNVERIFIED");
  assert.equal(result.rollback.attempts, 1);
  assert.equal(result.rollback.status, "ATTEMPTED");
  assert.equal(f.writes.filter((r) => r.body?.versions?.[0].version_id === id(1)).length, 1);
  assert.ok(!JSON.stringify(result).includes("secret-provider-message"));
});

test("failed recovery cannot claim a successful rollback", async () => {
  const result = await fixture({ journeys: [pass, fail, fail, fail] }).run();
  assert.equal(result.status, "FAIL");
  assert.equal(result.recovery.status, "FAIL");
  assert.equal(result.rollback.attempts, 1);
});

test("unreachable recovery is unverified after one rollback", async () => {
  const result = await fixture({ journeys: [pass, fail, fail, unavailable] }).run();
  assert.equal(result.status, "UNVERIFIED");
  assert.equal(result.recovery.status, "UNVERIFIED");
  assert.equal(result.rollback.attempts, 1);
});

test("checker exercises health, readiness, and owner-authenticated Map using only GET", async () => {
  const requests = [];
  const check = checker(credentials, async (url, init) => {
    requests.push({ path: new URL(url).pathname, method: init.method });
    assert.equal(init.redirect, "manual");
    assert.equal(init.headers.Cookie, "CF_Authorization=synthetic-owner-jwt");
    if (url.endsWith("/map")) {
      assert.equal(init.headers.Authorization, undefined);
      return new Response('<div id="main" data-view="map" data-http-status="200"><section class="map-stage"></section></div>', { headers: { "Content-Type": "text/html" } });
    }
    assert.equal(init.headers.Authorization, "Bearer synthetic-probe-token");
    return new Response(url.endsWith("/healthz") ? "ok\n" : "ready\n");
  });
  assert.deepEqual(await check(), pass);
  assert.deepEqual(requests, [{ path: "/healthz", method: "GET" }, { path: "/readyz", method: "GET" }, { path: "/map", method: "GET" }]);
});

for (const status of [302, 401, 403, 429]) {
  test(`checker HTTP ${status} is unverified`, async () => {
    assert.equal((await checker(credentials, async () => new Response("private", { status }))()).status, "UNVERIFIED");
  });
}

test("checker transport failure is unverified without exposing secrets", async () => {
  const result = await checker(credentials, async () => { throw new Error("Bearer secret-token"); })();
  assert.deepEqual(result, unavailable);
});

test("checker returns failure for an accessible broken journey", async () => {
  const result = await checker(credentials, async (url) => new Response(url.endsWith("/healthz") ? "ok" : "broken"))();
  assert.deepEqual(result, { status: "FAIL", reason: "journey_assertion_failure", path: "/readyz", http_status: 200 });
});

test("HTTP 200 Access login pages are unverified, never a rollback trigger", async () => {
  const result = await checker(credentials, async (url) => {
    if (url.endsWith("/healthz")) return new Response("ok");
    if (url.endsWith("/readyz")) return new Response("ready");
    return new Response("<html><title>Sign in to Cloudflare Access</title></html>", { headers: { "Content-Type": "text/html" } });
  })();
  assert.deepEqual(result, { status: "UNVERIFIED", reason: "checker_response_not_application", path: "/map", http_status: 200 });
});

test("an application error page fails the critical journey", async () => {
  const result = await checker(credentials, async (url) => {
    if (url.endsWith("/healthz")) return new Response("ok");
    if (url.endsWith("/readyz")) return new Response("ready");
    return new Response('<div id="main" data-view="error" data-http-status="500"></div>', { headers: { "Content-Type": "text/html" } });
  })();
  assert.equal(result.status, "FAIL");
  assert.equal(result.path, "/map");
});

async function cliHome(run) {
  const directory = await mkdtemp(join(tmpdir(), "scry-release-cli-"));
  try {
    await run(directory);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
}

function cli(directory, artifactPath, outcomePath) {
  return spawnSync(process.execPath, [new URL("./release.mjs", import.meta.url).pathname, artifactPath, outcomePath], {
    env: { PATH: process.env.PATH, HOME: directory, CLOUDFLARE_API_TOKEN: "must-never-be-used" }, encoding: "utf8", timeout: 5000,
  });
}

test("CLI without release.env emits JSON unverified and ignores inherited account-wide credentials", async () => {
  await cliHome(async (directory) => {
    const result = cli(directory, join(directory, "artifact.json"), join(directory, "outcome.json"));
    assert.equal(result.status, 1);
    assert.equal(result.stderr, "");
    assert.deepEqual(JSON.parse(result.stdout), {
      format: "scry-release-outcome-v1", status: "UNVERIFIED", reason: "release_precondition_unverified", database_rollback: false,
    });
    assert.ok(!result.stdout.includes("must-never-be-used"));
  });
});

test("CLI refuses an existing receipt without repeating effects or overwriting evidence", async () => {
  await cliHome(async (directory) => {
    await mkdir(join(directory, ".config/scry"), { recursive: true });
    await writeFile(join(directory, ".config/scry/release.env"), Object.entries(credentials).map(([k, v]) => `${k}=${v}`).join("\n"), { mode: 0o600 });
    const artifactPath = join(directory, "artifact.json");
    const outcomePath = join(directory, "outcome.json");
    await writeFile(artifactPath, JSON.stringify(artifact));
    await writeFile(outcomePath, "original evidence");
    const result = cli(directory, artifactPath, outcomePath);
    assert.equal(result.status, 1);
    assert.equal(JSON.parse(result.stdout).status, "UNVERIFIED");
    assert.equal(await readFile(outcomePath, "utf8"), "original evidence");
  });
});

test("CLI refuses credentials readable by other users", async () => {
  await cliHome(async (directory) => {
    await mkdir(join(directory, ".config/scry"), { recursive: true });
    await writeFile(join(directory, ".config/scry/release.env"), Object.entries(credentials).map(([k, v]) => `${k}=${v}`).join("\n"), { mode: 0o644 });
    const result = cli(directory, join(directory, "artifact.json"), join(directory, "outcome.json"));
    assert.equal(result.status, 1);
    assert.equal(JSON.parse(result.stdout).status, "UNVERIFIED");
  });
});

test("artifact rejects production, mutable images, incompatible rollback, and dirty identity", () => {
  for (const changed of [{ environment: "production" }, { image: "registry.cloudflare.com/test/scry:latest" },
    { rollback_compatible: false }, { database_rollback: true }, { revision: "worktree-abc" }]) {
    assert.throws(() => parseArtifact({ ...artifact, ...changed }));
  }
});

test("credential parser requires a complete file and rejects shell execution", () => {
  for (const input of ["CLOUDFLARE_API_TOKEN=secret", "export SCRY_RELEASE_CF_TOKEN=secret", "SCRY_RELEASE_CF_TOKEN=$(cat secret)"]) {
    assert.throws(() => parseCredentials(input));
  }
});

test("Maren's file key names normalize to the dedicated release capability without ambient lookup", () => {
  const contents = ["CLOUDFLARE_API_TOKEN=dedicated-test-token", `CLOUDFLARE_ACCOUNT_ID=${"e".repeat(32)}`,
    "SCRY_PROBE_TOKEN=synthetic-probe-token", "SCRY_ACCESS_JWT=synthetic-owner-jwt"].join("\n");
  assert.deepEqual(parseCredentials(contents), credentials);
  assert.throws(() => parseCredentials(contents + "\nSCRY_RELEASE_CF_TOKEN=duplicate"));
});

test("failed checkpoint prevents deployment and still returns an unverified JSON outcome", async () => {
  const result = await release(artifact, {
    api: { current: async () => { assert.fail("must not contact Cloudflare without a checkpoint"); } },
    check: async () => { assert.fail("must not contact checker without a checkpoint"); },
    save: async () => { throw new Error("disk unavailable"); },
  });
  assert.equal(result.status, "UNVERIFIED");
  assert.equal(result.reason, "receipt_write_unverified");
  assert.equal(result.deployment.status, "NOT_ATTEMPTED");
});
