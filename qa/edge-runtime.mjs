import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile, writeFile } from "node:fs/promises";
import { once } from "node:events";
import { performance } from "node:perf_hooks";
import { Miniflare, convertV4MiniflareOptions, Response as RuntimeResponse } from "miniflare";
import { exportJWK, generateKeyPair, SignJWT } from "jose";

const script = await readFile(process.argv[2] || "target/edge-release/index.js", "utf8");
const fixture = await readFile(new URL("./edge-fixture.mjs", import.meta.url), "utf8");
const { privateKey, publicKey } = await generateKeyPair("RS256", { extractable: true });
const jwk = await exportJWK(publicKey);
jwk.kid = "qa";
const jwt = await new SignJWT({ sub: "synthetic-owner" }).setProtectedHeader({ alg: "RS256", kid: "qa" })
  .setIssuer("https://access.example.test").setAudience("qa").setIssuedAt().setExpirationTime("10m").sign(privateKey);
let mode = "ok";
let received = [];
const sink = createServer(async (request, response) => {
  let body = "";
  for await (const chunk of request) body += chunk;
  received.push(body);
  if (mode === "hang") await new Promise(resolve => setTimeout(resolve, 3000));
  response.writeHead(mode === "reject" ? 429 : 200, { "retry-after": "60" });
  response.end("{}");
});
sink.listen(0, "127.0.0.1");
await once(sink, "listening");
const version = "11111111-2222-4333-8444-555555555555";
const observations = [];
async function instance(overrides = {}) {
  return new Miniflare(convertV4MiniflareOptions({
    workers: [{ name: "edge", modules: true, script, compatibilityDate: "2026-09-09",
      bindings: { SCRY_PROBE_TOKEN: "synthetic-only", SCRY_ACCESS_TEAM_DOMAIN: "https://access.example.test",
        SCRY_ACCESS_AUD: "qa", SCRY_ACCESS_OWNER_SUB: "synthetic-owner", SCRY_VERSION: { id: version },
        SENTRY_ENVIRONMENT: "synthetic-staging", ...overrides },
      durableObjects: { SCRY_CONTAINER: { className: "FixtureContainer", scriptName: "fixture" } },
      outboundService: async request => {
        if (request.url === "https://access.example.test/cdn-cgi/access/certs")
          return new RuntimeResponse(JSON.stringify({ keys: [jwk] }), { headers: { "content-type": "application/json" } });
        assert.equal(new URL(request.url).hostname, "o4510313803677696.ingest.us.sentry.io");
        const response = await fetch(`http://127.0.0.1:${sink.address().port}/envelope`, {
          method: "POST", body: await request.text(), signal: AbortSignal.timeout(4000),
        });
        return new RuntimeResponse(await response.text(), { status: response.status, headers: response.headers });
      },
    }, { name: "fixture", modules: true, script: fixture, compatibilityDate: "2026-09-09",
      durableObjects: { FIXTURE: "FixtureContainer" } }],
  }));
}
const headers = { "cf-access-jwt-assertion": jwt, cookie: "PRIVATE-cookie-canary", "x-private": "PRIVATE-header-canary" };
async function verify(name, action, overrides) {
  received = [];
  const mf = await instance(overrides);
  try { await action(mf); observations.push({ name, pass: true }); }
  finally { await mf.dispose(); }
}
// Workerd owns waitUntil, not the Miniflare controller. Observe beyond the
// transport's hard two-second deadline before asserting sink effects.
async function settle() { await new Promise(resolve => setTimeout(resolve, 2300)); }
try {
  await verify("allowed synthetic study and private ingress denials do not report", async mf => {
    assert.equal((await mf.dispatchFetch("https://scry.test/", { headers })).status, 200);
    assert.equal((await mf.dispatchFetch("https://scry.test/healthz")).status, 401);
    assert.equal((await mf.dispatchFetch("https://scry.test/", { headers: { "cf-access-jwt-assertion": "forged" } })).status, 403);
    assert.equal((await mf.dispatchFetch("https://scry.test/create", { method: "POST", body: "PRIVATE-intent-canary" })).status, 403);
    await settle(mf); assert.equal(received.length, 0);
  });
  await verify("upstream failure is unchanged; one allowlisted envelope excludes all privacy canaries", async mf => {
    const response = await mf.dispatchFetch("https://scry.test/healthz?mode=fail&private=PRIVATE-url-canary", {
      headers: { authorization: "Bearer synthetic-only", cookie: "PRIVATE-cookie-canary" },
    });
    assert.equal(response.status, 503); assert.equal(await response.text(), "PRIVATE-response-canary");
    await settle(mf); assert.equal(received.length, 1);
    assert(!received[0].includes("PRIVATE-")); assert(!received[0].includes("synthetic-only"));
    const event = JSON.parse(received[0].split("\n")[2]);
    assert.equal(event.environment, "synthetic-staging"); assert.equal(event.release, `scry-edge@${version}`);
    assert.deepEqual(Object.keys(event).sort(), ["event_id", "timestamp", "platform", "level", "logger", "environment", "release", "message", "fingerprint", "tags"].sort());
    assert.deepEqual(event.tags, { boundary: "go-container-edge", category: "upstream_5xx" });
    await mf.dispatchFetch("https://scry.test/healthz?mode=fail", { headers: { authorization: "Bearer synthetic-only" } });
    await settle(mf); assert.equal(received.length, 1);
  });
  await verify("thrown private error is reported only as a fixed category", async mf => {
    const response = await mf.dispatchFetch("https://scry.test/healthz?mode=throw", { headers: { authorization: "Bearer synthetic-only" } });
    assert.equal(response.status, 500); await settle(mf);
    assert.equal(received.length, 1); assert(!received[0].includes("PRIVATE-"));
    assert.equal(JSON.parse(received[0].split("\n")[2]).message, "Scry edge request_exception");
  });
  for (const outage of ["reject", "hang"]) {
    mode = outage;
    await verify(`sink ${outage} cannot delay failure or subsequent study`, async mf => {
      // Warm runtime/JWKS outside timing; same real consumer route below.
      await mf.dispatchFetch("https://scry.test/", { headers });
      const start = performance.now();
      const response = await mf.dispatchFetch("https://scry.test/?mode=fail", { headers });
      assert.equal(response.status, 503);
      const study = await mf.dispatchFetch("https://scry.test/", { headers });
      assert.equal(study.status, 200); assert((await study.text()).includes("deliberate Next"));
      const elapsed = performance.now() - start; assert(elapsed < 1000, `sink stalled consumer: ${elapsed}ms`);
      await settle(mf); assert.equal(received.length, 1);
      observations.push({ sink: outage, response_and_study_ms: elapsed });
    });
  }
  mode = "ok";
  await verify("reporting recovers after failed sink and isolate restart", async mf => {
    assert.equal((await mf.dispatchFetch("https://scry.test/?mode=fail", { headers })).status, 503);
    await settle(mf); assert.equal(received.length, 1);
    assert.equal((await mf.dispatchFetch("https://scry.test/", { headers })).status, 200);
  });
  for (const bad of [{ SENTRY_ENVIRONMENT: "unknown" }, { SCRY_VERSION: {} }]) {
    await verify("invalid telemetry configuration leaves study intact without egress", async mf => {
      assert.equal((await mf.dispatchFetch("https://scry.test/?mode=fail", { headers })).status, 503);
      assert.equal((await mf.dispatchFetch("https://scry.test/", { headers })).status, 200);
      await settle(mf); assert.equal(received.length, 0);
    }, bad);
  }
  await writeFile("target/edge-runtime-proof.json", JSON.stringify(observations, null, 2));
  console.log(JSON.stringify(observations, null, 2));
} finally { sink.closeAllConnections(); sink.close(); }
