import assert from "node:assert/strict";
import test from "node:test";

import { appEnvVars, containerSleepAfter } from "./runtime-env.mjs";

const complete = {
  SCRY_BOOT_MODE: "restore-required",
  SCRY_DATA_CLASS: "recovered",
  SCRY_OWNER_ID: "synthetic-owner",
  SCRY_SECRET: "s".repeat(64),
  SCRY_BASE_URL: "https://scry.example",
  SCRY_REDIRECT_HOSTS: "",
  SCRY_BACKUP_REMOTE_URL: "https://backups.example",
  SCRY_BACKUP_REMOTE_TOKEN: "b".repeat(64),
  SCRY_MODEL_ENDPOINT: "https://openrouter.ai/api/v1/chat/completions",
  SCRY_MODEL_API_KEY: "m".repeat(64),
  SCRY_MODEL: "google/gemini-3.7-flash",
  SCRY_MODEL_PROVIDER: "openrouter",
  SCRY_GENERATION_DAILY_BUDGET_MICROS: "1000000",
  SCRY_GENERATION_RESERVATION_MICROS: "200000",
};

test("container runtime forwards the existing configurable model contract", () => {
  const vars = appEnvVars(complete);
  assert.equal(vars.SCRY_BOOT_MODE, "restore-required");
  assert.equal(vars.SCRY_DATA_CLASS, "recovered");
  assert.equal(vars.SCRY_BACKUP_REMOTE_URL, complete.SCRY_BACKUP_REMOTE_URL);
  assert.equal(vars.SCRY_MODEL_ENDPOINT, complete.SCRY_MODEL_ENDPOINT);
  assert.equal(vars.SCRY_MODEL_API_KEY, complete.SCRY_MODEL_API_KEY);
  assert.equal(vars.SCRY_MODEL, "google/gemini-3.7-flash");
  assert.equal(vars.SCRY_MODEL_PROVIDER, "openrouter");
  assert.equal(vars.SCRY_GENERATION_DAILY_BUDGET_MICROS, "1000000");
  assert.equal(vars.SCRY_GENERATION_RESERVATION_MICROS, "200000");
  assert.equal(vars.SCRY_BACKUP_KEEP, "30");
});

test("synthetic staging disables generation without inventing a provider", () => {
  const synthetic = { ...complete, SCRY_DATA_CLASS: "synthetic" };
  for (const name of ["SCRY_MODEL_ENDPOINT", "SCRY_MODEL_API_KEY", "SCRY_MODEL", "SCRY_MODEL_PROVIDER"]) {
    delete synthetic[name];
  }
  const vars = appEnvVars(synthetic);
  assert.equal(vars.SCRY_MODEL_ENDPOINT, "");
  assert.equal(vars.SCRY_MODEL_API_KEY, "");
  assert.equal(vars.SCRY_MODEL, "");
  assert.equal(vars.SCRY_MODEL_PROVIDER, "");
  assert.equal(vars.SCRY_GENERATION_DAILY_BUDGET_MICROS, "1000000");
  assert.equal(vars.SCRY_GENERATION_RESERVATION_MICROS, "200000");
});

test("recovered hosting refuses a missing or partial model capability", () => {
  for (const name of ["SCRY_MODEL_ENDPOINT", "SCRY_MODEL_API_KEY", "SCRY_MODEL", "SCRY_MODEL_PROVIDER"]) {
    const candidate = { ...complete };
    delete candidate[name];
    assert.throws(() => appEnvVars(candidate), new RegExp(name));
  }
  assert.throws(
    () => appEnvVars({ ...complete, SCRY_DATA_CLASS: "synthetic", SCRY_MODEL_API_KEY: "" }),
    /model configuration must be either complete or disabled/,
  );
});

test("container runtime rejects unsafe identity and endpoint values", () => {
  assert.throws(() => appEnvVars({ ...complete, SCRY_OWNER_ID: "owner\nproxy_set_header" }), /SCRY_OWNER_ID/);
  assert.throws(() => appEnvVars({ ...complete, SCRY_SECRET: "short" }), /SCRY_SECRET/);
  assert.throws(() => appEnvVars({ ...complete, SCRY_BACKUP_REMOTE_URL: "http://backup.example" }), /SCRY_BACKUP_REMOTE_URL/);
  assert.throws(() => appEnvVars({ ...complete, SCRY_MODEL_ENDPOINT: "http://model.example" }), /SCRY_MODEL_ENDPOINT/);
  assert.throws(() => appEnvVars({ ...complete, SCRY_MODEL: "model\nname" }), /SCRY_MODEL/);
  assert.throws(() => appEnvVars({ ...complete, SCRY_MODEL_PROVIDER: "workers-ai" }), /SCRY_MODEL_PROVIDER/);
});

test("container idle policy is explicit and bounded", () => {
  assert.equal(containerSleepAfter({ SCRY_SLEEP_AFTER: "1m" }), "1m");
  assert.equal(containerSleepAfter({ SCRY_SLEEP_AFTER: "24h" }), "24h");
  assert.throws(() => containerSleepAfter({ SCRY_SLEEP_AFTER: "never" }), /SCRY_SLEEP_AFTER/);
});

test("container runtime refuses any missing durability capability", () => {
  for (const name of [
    "SCRY_BOOT_MODE",
    "SCRY_DATA_CLASS",
    "SCRY_OWNER_ID",
    "SCRY_SECRET",
    "SCRY_BASE_URL",
    "SCRY_BACKUP_REMOTE_URL",
    "SCRY_BACKUP_REMOTE_TOKEN",
    "SCRY_GENERATION_DAILY_BUDGET_MICROS",
    "SCRY_GENERATION_RESERVATION_MICROS",
  ]) {
    const candidate = { ...complete };
    delete candidate[name];
    assert.throws(() => appEnvVars(candidate), new RegExp(name));
  }
});
