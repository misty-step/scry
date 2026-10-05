#!/usr/bin/env node
import { readFile, open, rename, stat } from "node:fs/promises";
import { homedir } from "node:os";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { setTimeout as sleep } from "node:timers/promises";

const ORIGIN = "https://scry-app-host-staging.misty-step.workers.dev";
const WORKER = "scry-app-host-staging";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const IMAGE = /^registry\.cloudflare\.com\/[a-zA-Z0-9/_-]+@sha256:[0-9a-f]{64}$/;

function requireValue(ok, message) {
  if (!ok) throw new Error(message);
}

export function parseArtifact(value) {
  requireValue(value?.format === "scry-release-artifact-v1", "invalid artifact format");
  requireValue(value.environment === "staging" && value.data_class === "synthetic", "v0 requires synthetic staging");
  requireValue(UUID.test(value.version_id) && UUID.test(value.previous?.deployment_id)
    && UUID.test(value.previous?.version_id) && UUID.test(value.container_application_id), "invalid artifact identity");
  requireValue(IMAGE.test(value.image) && IMAGE.test(value.previous?.image), "images must have immutable registry digests");
  requireValue(/^[0-9a-f]{40}$/.test(value.revision) && /^[0-9a-f]{64}$/.test(value.binary_sha256), "missing committed artifact identity");
  for (const key of ["evidence", "compatibility_evidence"]) {
    requireValue(typeof value[key] === "string" && /^https:\/\/[^\s]+$/.test(value[key]), `missing ${key}`);
  }
  requireValue(value.database_rollback === false && value.rollback_compatible === true, "compatible code-only rollback required");
  requireValue(value.version_id !== value.previous.version_id && value.image !== value.previous.image, "artifact must be a new release");
  return value;
}

export function parseCredentials(contents) {
  const values = {};
  const allowed = new Set(["SCRY_RELEASE_CF_TOKEN", "SCRY_RELEASE_ACCOUNT_ID", "SCRY_PROBE_TOKEN",
    "SCRY_ACCESS_JWT", "SCRY_ACCESS_CLIENT_ID", "SCRY_ACCESS_CLIENT_SECRET"]);
  for (const raw of contents.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line.startsWith("#")) continue;
    const match = /^([A-Z_]+)=(.*)$/.exec(line);
    requireValue(match && allowed.has(match[1]) && !(match[1] in values), "invalid release credential entry");
    let value = match[2];
    if (value.startsWith('"') || value.startsWith("'")) {
      requireValue(value.length >= 2 && value.at(-1) === value[0], "invalid credential quoting");
      value = value.slice(1, -1);
    }
    requireValue(value && !/[\s\x00-\x1f\x7f]/.test(value), "invalid credential value");
    values[match[1]] = value;
  }
  for (const key of ["SCRY_RELEASE_CF_TOKEN", "SCRY_RELEASE_ACCOUNT_ID", "SCRY_PROBE_TOKEN", "SCRY_ACCESS_JWT"]) {
    requireValue(values[key], `missing ${key}`);
  }
  requireValue(/^[0-9a-f]{32}$/.test(values.SCRY_RELEASE_ACCOUNT_ID), "invalid release account ID");
  requireValue(Boolean(values.SCRY_ACCESS_CLIENT_ID) === Boolean(values.SCRY_ACCESS_CLIENT_SECRET), "Access service token requires both values");
  return values;
}

export async function loadCredentials() {
  const path = resolve(homedir(), ".config/scry/release.env");
  const info = await stat(path);
  requireValue(info.isFile() && (info.mode & 0o077) === 0 && info.uid === process.getuid(), "release.env must be owner-only");
  return parseCredentials(await readFile(path, "utf8"));
}

export function cloudflare(credentials, fetcher = fetch) {
  const base = `https://api.cloudflare.com/client/v4/accounts/${credentials.SCRY_RELEASE_ACCOUNT_ID}`;
  async function call(path, method = "GET", body) {
    // Do not expose provider bodies, URLs, headers, or transport errors in receipts.
    const response = await fetcher(base + path, {
      method, redirect: "error", signal: AbortSignal.timeout(15000),
      headers: { Authorization: `Bearer ${credentials.SCRY_RELEASE_CF_TOKEN}`, "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    requireValue(response.ok, "Cloudflare API unavailable");
    const envelope = await response.json();
    requireValue(envelope.success === true && envelope.result, "Cloudflare API response unverified");
    return envelope.result;
  }
  return {
    async current() {
      const result = await call(`/workers/scripts/${WORKER}/deployments`);
      const current = result.deployments?.[0];
      requireValue(UUID.test(current?.id) && current.versions?.length === 1
        && current.versions[0].percentage === 100 && UUID.test(current.versions[0].version_id), "current deployment unverified");
      return { deployment_id: current.id, version_id: current.versions[0].version_id };
    },
    async deploy(version, operation) {
      const result = await call(`/workers/scripts/${WORKER}/deployments`, "POST", {
        strategy: "percentage", versions: [{ version_id: version, percentage: 100 }],
        annotations: { "workers/message": operation },
      });
      requireValue(UUID.test(result.id) && result.versions?.length === 1
        && result.versions[0].version_id === version && result.versions[0].percentage === 100, "deployment response unverified");
      return { deployment_id: result.id, version_id: version };
    },
    async application(id) {
      const result = await call(`/containers/applications/${id}`);
      requireValue(result.name === "scry-app-container-staging" && IMAGE.test(result.configuration?.image)
        && result.scheduling_policy !== "durable_object", "staging container configuration unverified");
      return { image: result.configuration.image };
    },
    async rollout(id, image, operation) {
      // Match Wrangler's configuration update followed by a single 100% rollout.
      await call(`/containers/applications/${id}`, "PATCH", { configuration: { image } });
      const result = await call(`/containers/applications/${id}/rollouts`, "POST", {
        description: operation, strategy: "rolling", kind: "full_auto", step_percentage: 100,
        target_configuration: { image },
      });
      requireValue(UUID.test(result.id), "rollout response unverified");
      return result.id;
    },
    async rolloutStatus(id, rolloutId, image) {
      const result = await call(`/containers/applications/${id}/rollouts/${rolloutId}`);
      requireValue(result.target_configuration?.image === image, "rollout target unverified");
      return result.status;
    },
  };
}

export function checker(credentials, fetcher = fetch) {
  const access = {
    Cookie: `CF_Authorization=${credentials.SCRY_ACCESS_JWT}`,
    "Cf-Access-Jwt-Assertion": credentials.SCRY_ACCESS_JWT,
  };
  if (credentials.SCRY_ACCESS_CLIENT_ID) {
    access["CF-Access-Client-Id"] = credentials.SCRY_ACCESS_CLIENT_ID;
    access["CF-Access-Client-Secret"] = credentials.SCRY_ACCESS_CLIENT_SECRET;
  }
  return async () => {
    try {
      for (const [path, expected] of [["/healthz", "ok"], ["/readyz", "ready"], ["/map", null]]) {
        const headers = { ...access, "Cache-Control": "no-cache", Accept: path === "/map" ? "text/html" : "text/plain" };
        if (expected) headers.Authorization = `Bearer ${credentials.SCRY_PROBE_TOKEN}`;
        const response = await fetcher(ORIGIN + path, {
          method: "GET", headers, redirect: "manual", signal: AbortSignal.timeout(15000),
        });
        if ([401, 403, 407, 429].includes(response.status) || (response.status >= 300 && response.status < 400)) {
          return { status: "UNVERIFIED", reason: "checker_inaccessible", path, http_status: response.status };
        }
        const body = await response.text();
        if (response.status !== 200) return { status: "FAIL", reason: "journey_http_failure", path, http_status: response.status };
        if (expected ? body.trim() !== expected : !response.headers.get("content-type")?.includes("text/html")
          || !body.includes('data-view="map"') || !body.includes('data-http-status="200"') || !body.includes('class="map-stage"')) {
          return { status: "FAIL", reason: "journey_assertion_failure", path, http_status: 200 };
        }
      }
      return { status: "PASS", reason: "health_ready_private_map" };
    } catch {
      return { status: "UNVERIFIED", reason: "checker_inaccessible" };
    }
  };
}

function same(a, b) {
  return a.deployment_id === b.deployment_id && a.version_id === b.version_id;
}

export async function release(artifact, { api, check, wait = sleep, now = Date.now, save = async () => {} }) {
  const outcome = {
    format: "scry-release-outcome-v1", environment: "staging", status: "UNVERIFIED", reason: "in_progress",
    artifact: { revision: artifact.revision, binary_sha256: artifact.binary_sha256,
      version_id: artifact.version_id, image: artifact.image, evidence: artifact.evidence },
    previous: artifact.previous, deployment: { status: "NOT_ATTEMPTED" },
    observation: { status: "UNVERIFIED", checks: [] }, rollback: { status: "NOT_ATTEMPTED", attempts: 0 },
    recovery: { status: "NOT_ATTEMPTED", checks: [] }, database_rollback: false,
  };
  async function finish(status, reason) {
    Object.assign(outcome, { status, reason });
    await save(outcome);
    return outcome;
  }
  async function sample(expected, image, rolloutId, phase, offset) {
    const before = await api.current();
    if (!same(before, expected)) return { status: "SUPERSEDED", reason: "newer_deployment_present", observed: before };
    const app = await api.application(artifact.container_application_id);
    if (app.image !== image) return { status: "SUPERSEDED", reason: "container_changed" };
    const rollout = rolloutId ? await api.rolloutStatus(artifact.container_application_id, rolloutId, image) : "completed";
    const journey = await check();
    const after = await api.current();
    const result = { ...journey, phase, offset_ms: offset, observed: after, rollout };
    if (!same(after, expected)) Object.assign(result, { status: "SUPERSEDED", reason: "newer_deployment_present" });
    else if (journey.status === "PASS" && rollout !== "completed") Object.assign(result, { status: "UNVERIFIED", reason: "container_rollout_incomplete" });
    return result;
  }
  try {
    await save(outcome);
    requireValue(same(await api.current(), artifact.previous), "release baseline changed");
    requireValue((await api.application(artifact.container_application_id)).image === artifact.previous.image, "container baseline changed");
    const baseline = await sample(artifact.previous, artifact.previous.image, null, "baseline", 0);
    outcome.baseline = baseline;
    if (baseline.status !== "PASS") return finish("UNVERIFIED", "baseline_unverified");
    requireValue(same(await api.current(), artifact.previous), "release baseline changed");
    outcome.deployment.status = "ATTEMPTED";
    await save(outcome);
    const deployed = await api.deploy(artifact.version_id, `scry-release:${artifact.revision}`);
    Object.assign(outcome.deployment, deployed);
    await save(outcome);
    if (!same(await api.current(), deployed)) return finish("SUPERSEDED", "newer_deployment_present");
    outcome.deployment.rollout_id = await api.rollout(artifact.container_application_id, artifact.image, `scry-release:${artifact.revision}`);
    outcome.deployment.status = "DEPLOYED";
    await save(outcome);
    const started = now();
    for (const offset of [0, 150000, 300000]) {
      while (now() - started < offset) await wait(Math.min(60000, offset - (now() - started)));
      const result = await sample(deployed, artifact.image, outcome.deployment.rollout_id, "observation", offset);
      outcome.observation.checks.push(result);
      await save(outcome);
      if (result.status === "SUPERSEDED") return finish("SUPERSEDED", result.reason);
      if (result.status !== "FAIL") continue;
      await wait(10000);
      const confirm = await sample(deployed, artifact.image, outcome.deployment.rollout_id, "confirmation", now() - started);
      outcome.observation.checks.push(confirm);
      await save(outcome);
      if (confirm.status === "SUPERSEDED") return finish("SUPERSEDED", confirm.reason);
      if (confirm.status === "UNVERIFIED") return finish("UNVERIFIED", confirm.reason);
      if (confirm.status !== "FAIL") continue;
      outcome.observation.status = "FAIL";
      // Caller holds the shared release slot through this final read and recovery.
      if (!same(await api.current(), deployed)) return finish("SUPERSEDED", "newer_deployment_present");
      if ((await api.application(artifact.container_application_id)).image !== artifact.image) return finish("SUPERSEDED", "container_changed");
      outcome.rollback = { status: "ATTEMPTED", attempts: 1 };
      await save(outcome);
      const restored = await api.deploy(artifact.previous.version_id, `scry-rollback:${artifact.revision}`);
      Object.assign(outcome.rollback, restored);
      await save(outcome);
      if (!same(await api.current(), restored)) return finish("SUPERSEDED", "newer_deployment_present");
      outcome.rollback.rollout_id = await api.rollout(artifact.container_application_id, artifact.previous.image, `scry-rollback:${artifact.revision}`);
      outcome.rollback.status = "ROLLED_BACK";
      await save(outcome);
      await wait(10000);
      const recovery = await sample(restored, artifact.previous.image, outcome.rollback.rollout_id, "recovery", 10000);
      outcome.recovery.checks.push(recovery);
      outcome.recovery.status = recovery.status;
      return finish(recovery.status === "PASS" ? "ROLLED_BACK" : recovery.status,
        recovery.status === "PASS" ? "recovery_verified" : "recovery_unverified_or_failed");
    }
    const checks = outcome.observation.checks;
    outcome.observation.status = checks.every((result) => result.status === "PASS") ? "PASS" : "UNVERIFIED";
    return finish(outcome.observation.status, outcome.observation.status === "PASS" ? "observation_verified" : "observation_unverified");
  } catch {
    return finish("UNVERIFIED", "release_operation_unverified");
  }
}

export async function main(args) {
  let outcome;
  let handle;
  try {
    requireValue(args.length === 2, "usage: node release.mjs ARTIFACT.json OUTCOME.json");
    const credentials = await loadCredentials();
    const artifact = parseArtifact(JSON.parse(await readFile(args[0], "utf8")));
    // An existing receipt is never permission to repeat uncertain external effects.
    handle = await open(args[1], "wx", 0o600);
    await handle.close();
    handle = null;
    const save = async (value) => {
      const temporary = `${args[1]}.tmp`;
      const file = await open(temporary, "w", 0o600);
      await file.writeFile(JSON.stringify(value, null, 2) + "\n");
      await file.sync();
      await file.close();
      await rename(temporary, args[1]);
    };
    outcome = await release(artifact, { api: cloudflare(credentials), check: checker(credentials), save });
  } catch {
    outcome = { format: "scry-release-outcome-v1", status: "UNVERIFIED", reason: "release_precondition_unverified", database_rollback: false };
  } finally {
    if (handle) await handle.close();
  }
  process.stdout.write(JSON.stringify(outcome) + "\n");
  return outcome.status === "PASS" ? 0 : 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  process.exitCode = await main(process.argv.slice(2));
}
