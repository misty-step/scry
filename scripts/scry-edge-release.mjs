// Code-only Go-edge release. Uses CI's existing scoped token; never uploads a
// Container image, changes routes/schedules, reads data, or rebuilds an artifact.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFile, writeFile, mkdir } from "node:fs/promises";

const account = "b069014f6a46558ea9146fb6c4ff8f6c";
const base = `https://api.cloudflare.com/client/v4/accounts/${account}/workers/scripts`;
const mode = process.argv[2];
assert(["preview", "cleanup", "production", "rollback"].includes(mode));
const suffix = `${process.env.GITHUB_RUN_ID}-${process.env.GITHUB_RUN_ATTEMPT || 1}`;
assert(/^\d+-\d+$/.test(suffix));
const edge = `scry-edge-qa-${suffix}`;
const upstream = `${edge}-upstream`;
const token = process.env.CLOUDFLARE_API_TOKEN;
assert(token, "Scoped CI deploy token required");
const sha = value => createHash("sha256").update(value).digest("hex");
const telemetryBindings = ["SCRY_VERSION", "SENTRY_ENVIRONMENT"];
const unchanged = list => list.filter(binding => !telemetryBindings.includes(binding.name)).sort((a, b) => a.name.localeCompare(b.name));
async function api(path, method = "GET", body) {
  const response = await fetch(`${base}/${path}`, { method,
    headers: { authorization: `Bearer ${token}`, ...(body && !(body instanceof FormData) ? { "content-type": "application/json" } : {}) },
    body: body instanceof FormData ? body : body ? JSON.stringify(body) : undefined,
  });
  assert(response.ok, `Cloudflare ${method} ${path}: HTTP${response.status}`);
  const result = await response.json();
  assert(result.success, `Cloudflare ${method} ${path} failed`);
  return result.result;
}
async function upload(name, script, metadata, version = false) {
  const form = new FormData();
  form.set("metadata", new Blob([JSON.stringify({ main_module: "index.js", ...metadata })], { type: "application/json" }));
  form.set("index.js", new Blob([script], { type: "application/javascript+module" }), "index.js");
  return api(`${name}${version ? "/versions" : ""}`, version ? "POST" : "PUT", form);
}
const containers = version => version.resources.script_runtime.containers || [];
async function uploadPreservingContainers(name, script, metadata, live) {
  // Container attachment is versioned separately from the ordinary DO binding.
  // Omitting it disables Containers even when that binding is unchanged.
  const version = await upload(name, script, { ...metadata, containers: containers(live) }, true);
  const uploaded = await api(`${name}/versions/${version.id}`);
  assert.deepEqual(containers(uploaded), containers(live), "Uploaded version changed Container attachments; do not promote");
  return version;
}
async function readback(name, expected) {
  const response = await fetch(`${base}/${name}/content/v2`, { headers: { authorization: `Bearer ${token}` } });
  assert(response.ok, "Deployed-module readback failed");
  const form = await response.formData();
  const module = form.get("index.js");
  assert(module, "Uploaded module missing from readback");
  assert.equal(sha(Buffer.from(await module.arrayBuffer())), sha(expected), "Deployed bytes differ from tested artifact");
}
async function current(name) {
  const value = await api(`${name}/deployments`);
  const deployment = value.deployments[0];
  assert.equal(deployment.versions.length, 1, "Do not replace an in-progress gradual rollout");
  assert.equal(deployment.versions[0].percentage, 100);
  return deployment.versions[0].version_id;
}
// Native deployment success precedes edge/DO propagation (seconds to minutes:
// https://developers.cloudflare.com/durable-objects/platform/known-issues/#code-updates).
// Wait only for disposable, read-only sentinels, never a mutation or telemetry.
async function converged(url, status, text, options = {}) {
  const deadline = Date.now() + 180_000;
  let observed;
  do {
    const response = await fetch(url, { ...options, redirect: "manual" });
    observed = { status: response.status, body: await response.text() };
    if (observed.status === status && (text === undefined || observed.body === text)) return;
    await new Promise(resolve => setTimeout(resolve, 1000));
  } while (Date.now() < deadline);
  assert.fail(`Disposable edge did not converge: expected HTTP${status} ${text ?? ""}, observed ${JSON.stringify(observed)}`);
}
await mkdir("target", { recursive: true });
if (mode === "cleanup") {
  for (const name of [edge, upstream]) {
    const response = await fetch(`${base}/${name}?force=true`, { method: "DELETE", headers: { authorization: `Bearer ${token}` } });
    assert(response.ok || response.status === 404, `Cleanup ${name}: HTTP${response.status}`);
    const read = await fetch(`${base}/${name}/settings`, { headers: { authorization: `Bearer ${token}` } });
    assert.equal(read.status, 404, `Disposable Worker ${name} still exists`);
  }
  console.log("Both exactly named disposable Workers absent");
} else if (mode === "preview") {
  const script = await readFile("target/edge-release/index.js");
  const fixture = await readFile("qa/edge-fixture.mjs", "utf8");
  const fixtureMetadata = { compatibility_date: "2026-09-09",
    bindings: [{ type: "durable_object_namespace", name: "FIXTURE", class_name: "FixtureContainer" }] };
  // Enable a real Container namespace without creating an application, uploading
  // an image or starting compute. ctx.container proves the attachment at runtime.
  const fixturePriorScript = fixture.replace('const revision = "candidate";', 'const revision = "prior";');
  assert.notEqual(fixturePriorScript, fixture, "Distinct fixture prior code must actually differ");
  await upload(upstream, fixturePriorScript, {
    ...fixtureMetadata, containers: [{ class_name: "FixtureContainer", name: upstream }],
    migrations: { new_sqlite_classes: ["FixtureContainer"], new_tag: "v1" } });
  const fixturePrior = await current(upstream);
  const fixtureLive = await api(`${upstream}/versions/${fixturePrior}`);
  assert.equal(containers(fixtureLive).length, 1);
  // Reproduce the reviewed omission only on this disposable namespace. It must
  // be detectable before promotion; then the shared upload path repairs it.
  const detached = await upload(upstream, fixture, fixtureMetadata, true);
  assert.notDeepEqual(containers(await api(`${upstream}/versions/${detached.id}`)), containers(fixtureLive));
  assert.equal(await current(upstream), fixturePrior, "Negative control was never promoted");
  const fixtureCandidate = await uploadPreservingContainers(upstream, fixture, fixtureMetadata, fixtureLive);
  await api(`${upstream}/deployments`, "POST", { strategy: "percentage", versions: [{ version_id: fixtureCandidate.id, percentage: 100 }] });
  assert.equal(await current(upstream), fixtureCandidate.id);
  const bindings = [
    { type: "durable_object_namespace", name: "SCRY_CONTAINER", class_name: "FixtureContainer", script_name: upstream },
    { type: "plain_text", name: "SCRY_PROBE_TOKEN", text: "synthetic-only" },
    { type: "plain_text", name: "SENTRY_ENVIRONMENT", text: "synthetic-staging" },
    { type: "version_metadata", name: "SCRY_VERSION" },
  ];
  // Exercise the same native version upload/secret-preservation/promotion used
  // in production, then a distinct prior-code rollback. Everything is synthetic.
  await upload(edge, 'export default {fetch(){return new Response("prior synthetic boundary")}}', {
    compatibility_date: "2026-09-09", bindings: [...bindings,
      { type: "secret_text", name: "QA_UNUSED_SECRET", text: "PRIVATE-unused-environment-canary" }],
  });
  const prior = await current(edge);
  const candidate = await uploadPreservingContainers(edge, script, { compatibility_date: "2026-09-09", bindings, keep_bindings: ["secret_text"] }, await api(`${edge}/versions/${prior}`));
  await api(`${edge}/deployments`, "POST", { strategy: "percentage", versions: [{ version_id: candidate.id, percentage: 100 }] });
  assert.equal(await current(edge), candidate.id);
  assert((await api(`${edge}/settings`)).bindings.some(binding => binding.name === "QA_UNUSED_SECRET" && binding.type === "secret_text"));
  await api(`${edge}/subdomain`, "POST", { enabled: true, previews_enabled: false });
  await api(`${upstream}/subdomain`, "POST", { enabled: true, previews_enabled: false });
  await readback(edge, script);
  const subdomain = await fetch(`https://api.cloudflare.com/client/v4/accounts/${account}/workers/subdomain`, { headers: { authorization: `Bearer ${token}` } });
  assert(subdomain.ok); const domain = (await subdomain.json()).result.subdomain;
  const origin = `https://${edge}.${domain}.workers.dev`;
  const fixtureOrigin = `https://${upstream}.${domain}.workers.dev`;
  const request = path => fetch(origin + path, { headers: { authorization: "Bearer synthetic-only", cookie: "PRIVATE-cookie-canary" }, redirect: "manual" });
  await converged(origin + "/healthz", 401);
  assert.equal((await fetch(origin + "/", { redirect: "manual" })).status, 403);
  assert.equal((await request("/healthz")).status, 200);
  const attachment = async revision => {
    // Invoke the Container class through its own deployed Worker, like the
    // operated Go boundary, not an external binding's separate release boundary.
    await readback(upstream, revision === "prior" ? fixturePriorScript : fixture);
    await converged(fixtureOrigin + "/?mode=container-status", 200,
      JSON.stringify({ enabled: true, running: false, revision }));
    const response = await fetch(fixtureOrigin + "/?mode=container-status");
    assert.equal(response.status, 200);
    assert.deepEqual(await response.json(), { enabled: true, running: false, revision });
    assert.deepEqual(containers(await api(`${upstream}/versions/${await current(upstream)}`)), containers(fixtureLive));
    console.log(JSON.stringify({ container_attachment_probe: revision, enabled: true, running: false }));
  };
  await attachment("candidate");
  const failure = await request("/healthz?mode=fail&private=PRIVATE-url-canary");
  assert.equal(failure.status, 503); assert.equal(await failure.text(), "PRIVATE-response-canary");
  const recovered = await request("/healthz");
  assert.equal(recovered.status, 200); assert((await recovered.text()).includes("deliberate Next"));
  const version = await current(edge);
  await api(`${edge}/deployments`, "POST", { strategy: "percentage", versions: [{ version_id: prior, percentage: 100 }] });
  assert.equal(await current(edge), prior);
  await converged(origin + "/healthz", 200, "prior synthetic boundary");
  await api(`${upstream}/deployments`, "POST", { strategy: "percentage", versions: [{ version_id: fixturePrior, percentage: 100 }] });
  assert.equal(await current(upstream), fixturePrior);
  await api(`${edge}/deployments`, "POST", { strategy: "percentage", versions: [{ version_id: version, percentage: 100 }] });
  assert.equal(await current(edge), version);
  await readback(edge, script);
  await converged(origin + "/healthz", 401);
  assert.equal((await request("/healthz")).status, 200);
  await attachment("prior");
  await api(`${upstream}/deployments`, "POST", { strategy: "percentage", versions: [{ version_id: fixtureCandidate.id, percentage: 100 }] });
  assert.equal(await current(upstream), fixtureCandidate.id);
  await attachment("candidate");
  assert.equal((await fetch(origin + "/healthz", { redirect: "manual" })).status, 401);
  // Let the observed request's bounded waitUntil send finish before cleanup.
  await new Promise(resolve => setTimeout(resolve, 2500));
  const proof = { source: process.env.GITHUB_SHA, worker: edge, version, origin, artifact_sha256: sha(script),
    environment: "synthetic-staging", release: `scry-edge@${version}`, denied: [401, 403], failure: 503, recovered: 200,
    native_version_upload_and_promotion: true, synthetic_secret_preserved: true, distinct_prior_rollback: prior, exact_candidate_repromoted: true,
    container_attachment: { containers: containers(fixtureLive), previous: fixturePrior, version: fixtureCandidate.id,
      omission_detected_before_promotion: detached.id, enabled_after_promotion_rollback_and_repromotion: true,
      image_or_application_created: false, instance_started: false },
    limits: "Exact operated Go-edge module; disposable synthetic upstream, not live learner content, Go application or owner-session proof." };
  await writeFile("target/edge-hosted-proof.json", JSON.stringify(proof, null, 2));
  console.log(JSON.stringify(proof));
} else {
  const name = "scry-app-host";
  const previous = await current(name);
  const settings = await api(`${name}/settings`);
  const live = await api(`${name}/versions/${previous}`);
  assert(settings.bindings.some(binding => binding.name === "SCRY_CONTAINER" && binding.class_name === "ScryContainer"),
    "Operated runtime changed: do not overwrite the Rust cutover");
  if (mode === "rollback") {
    const version = process.env.ROLLBACK_VERSION;
    assert(/^[0-9a-f-]{36}$/.test(version));
    const target = await api(`${name}/versions/${version}`);
    assert.deepEqual(unchanged(target.resources.bindings), unchanged(settings.bindings), "Rollback would change existing runtime bindings");
    assert.deepEqual(containers(target), containers(live), "Rollback would change Container attachments");
    assert.equal(await current(name), previous);
    await api(`${name}/deployments`, "POST", { strategy: "percentage", versions: [{ version_id: version, percentage: 100 }], annotations: { "workers/message": "Scry Go edge code-only compatible rollback" } });
    assert.equal(await current(name), version);
    console.log(JSON.stringify({ previous, rollback: version, accepted_data_changed: false }));
  } else {
    const script = await readFile("target/edge-release/index.js");
    const source = /^scry-go-edge ([0-9a-f]{40})$/.exec(live.annotations?.["workers/message"] || "")?.[1];
    if (source) execFileSync("git", ["merge-base", "--is-ancestor", source, process.env.GITHUB_SHA]);
    else assert.equal(previous, "004c1645-5063-465f-b089-a7ad5bf0601a", "Unrecognized pre-CI release; do not overwrite another lane");
    const added = telemetryBindings;
    const bindings = settings.bindings.filter(binding => binding.type !== "secret_text" && !added.includes(binding.name));
    bindings.push({ type: "version_metadata", name: "SCRY_VERSION" },
      { type: "plain_text", name: "SENTRY_ENVIRONMENT", text: "production" });
    const version = await uploadPreservingContainers(name, script, { bindings, keep_bindings: ["secret_text"],
      compatibility_date: settings.compatibility_date, compatibility_flags: settings.compatibility_flags,
      annotations: { "workers/message": `scry-go-edge ${process.env.GITHUB_SHA}` },
    }, live);
    assert.equal(await current(name), previous, "Live release moved; do not overwrite another lane");
    await api(`${name}/deployments`, "POST", { strategy: "percentage", versions: [{ version_id: version.id, percentage: 100 }], annotations: { "workers/message": `scry-go-edge ${process.env.GITHUB_SHA}` } });
    try {
      assert.equal(await current(name), version.id);
      await readback(name, script);
      const after = await api(`${name}/settings`);
      assert.deepEqual(unchanged(after.bindings), unchanged(settings.bindings), "Existing runtime bindings changed");
      assert.deepEqual(containers(await api(`${name}/versions/${version.id}`)), containers(live), "Deployed Container attachments changed");
      for (const key of ["observability", "tags", "tail_consumers", "logpush", "placement", "usage_model"])
        assert.deepEqual(after[key], settings[key], `Existing ${key} changed`);
    } catch (error) {
      // Only revert our own still-active code; never stomp a concurrent cutover.
      if (await current(name) === version.id) {
        await api(`${name}/deployments`, "POST", { strategy: "percentage", versions: [{ version_id: previous, percentage: 100 }], annotations: { "workers/message": "Scry Go edge verification failed; restore prior code" } });
        assert.equal(await current(name), previous);
      }
      throw error;
    }
    const proof = { source: process.env.GITHUB_SHA, previous, version: version.id, artifact_sha256: sha(script),
      existing_bindings_unchanged: true, container_attachments_unchanged: containers(live), container_images_uploaded: false, routes_schedules_or_data_changed: false };
    await writeFile("target/edge-production-proof.json", JSON.stringify(proof, null, 2));
    console.log(JSON.stringify(proof));
  }
}
