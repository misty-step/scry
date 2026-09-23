import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { appEnvVars, backupExec, backupExecEnv, containerSleepAfter, semanticStartSummary } from "./runtime-env.mjs";

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
  SCRY_GENERATION_DAILY_BUDGET_MICROS: "3500000",
  SCRY_GENERATION_RESERVATION_MICROS: "500000",
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
  assert.equal(vars.SCRY_GENERATION_DAILY_BUDGET_MICROS, "3500000");
  assert.equal(vars.SCRY_GENERATION_RESERVATION_MICROS, "500000");
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
  assert.equal(vars.SCRY_GENERATION_DAILY_BUDGET_MICROS, "3500000");
  assert.equal(vars.SCRY_GENERATION_RESERVATION_MICROS, "500000");
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

const SEMANTIC = {
  SCRY_SEMANTIC_ENDPOINT: "https://openrouter.ai/api/alpha/decisions",
  SCRY_SEMANTIC_MODEL: "typesafe/jev-1.13",
  SCRY_SEMANTIC_RESERVATION_MICROS: "2000",
};
const SEMANTIC_NAMES = ["SCRY_SEMANTIC_ENDPOINT", "SCRY_SEMANTIC_API_KEY", "SCRY_SEMANTIC_MODEL", "SCRY_SEMANTIC_RESERVATION_MICROS"];

test("semantic mode is off and sends nothing unless an endpoint is configured", () => {
  const vars = appEnvVars(complete);
  // Explicitly empty, so the application reserves nothing and never sends.
  for (const name of SEMANTIC_NAMES) {
    assert.equal(vars[name], "", `${name} must be forwarded empty when semantic mode is off`);
  }
});

test("semantic mode forwards a complete Decisions configuration and reuses the model key", () => {
  const vars = appEnvVars({ ...complete, ...SEMANTIC });
  assert.equal(vars.SCRY_SEMANTIC_ENDPOINT, SEMANTIC.SCRY_SEMANTIC_ENDPOINT);
  assert.equal(vars.SCRY_SEMANTIC_MODEL, "typesafe/jev-1.13");
  assert.equal(vars.SCRY_SEMANTIC_RESERVATION_MICROS, "2000");
  // Empty key: the application reuses SCRY_MODEL_API_KEY (same capped key).
  assert.equal(vars.SCRY_SEMANTIC_API_KEY, "");
  assert.equal(vars.SCRY_MODEL_API_KEY, complete.SCRY_MODEL_API_KEY);
  // Generation limits stay pinned; semantic checks share that allowance.
  assert.equal(vars.SCRY_GENERATION_DAILY_BUDGET_MICROS, "3500000");
  assert.equal(vars.SCRY_GENERATION_RESERVATION_MICROS, "500000");

  const dedicated = appEnvVars({ ...complete, ...SEMANTIC, SCRY_SEMANTIC_API_KEY: "j".repeat(64) });
  assert.equal(dedicated.SCRY_SEMANTIC_API_KEY, "j".repeat(64));
});

test("semantic settings never enter the scheduled backup process", () => {
  const vars = appEnvVars({ ...complete, ...SEMANTIC, SCRY_SEMANTIC_API_KEY: "j".repeat(64) });
  const exec = backupExecEnv(vars);
  assert.deepEqual(Object.keys(exec).sort(), [
    "SCRY_BACKUP_DIR",
    "SCRY_BACKUP_INTERVAL",
    "SCRY_BACKUP_KEEP",
    "SCRY_BACKUP_REMOTE_TOKEN",
    "SCRY_BACKUP_REMOTE_URL",
  ]);
  for (const name of [...SEMANTIC_NAMES, "SCRY_MODEL_API_KEY"]) {
    assert.equal(name in exec, false, `${name} must not enter the backup process`);
  }
});

test("partial semantic configuration fails closed", () => {
  // Any semantic setting without an endpoint is a mistake, not a silent off.
  for (const name of ["SCRY_SEMANTIC_API_KEY", "SCRY_SEMANTIC_MODEL", "SCRY_SEMANTIC_RESERVATION_MICROS"]) {
    const value = name === "SCRY_SEMANTIC_API_KEY" ? "j".repeat(64) : SEMANTIC[name];
    assert.throws(() => appEnvVars({ ...complete, [name]: value }), new RegExp(`without SCRY_SEMANTIC_ENDPOINT.*${name}`));
  }
  // An endpoint without an explicit model or reservation does not fall back to defaults.
  assert.throws(() => appEnvVars({ ...complete, ...SEMANTIC, SCRY_SEMANTIC_MODEL: "" }), /SCRY_SEMANTIC_MODEL/);
  assert.throws(() => appEnvVars({ ...complete, ...SEMANTIC, SCRY_SEMANTIC_RESERVATION_MICROS: "" }), /SCRY_SEMANTIC_RESERVATION_MICROS/);
  // An endpoint with no key at all (model disabled, no dedicated key) cannot start.
  const synthetic = { ...complete, SCRY_DATA_CLASS: "synthetic", ...SEMANTIC };
  for (const name of ["SCRY_MODEL_ENDPOINT", "SCRY_MODEL_API_KEY", "SCRY_MODEL", "SCRY_MODEL_PROVIDER"]) {
    delete synthetic[name];
  }
  assert.throws(() => appEnvVars(synthetic), /SCRY_SEMANTIC_API_KEY or the complete model configuration/);
  assert.equal(appEnvVars({ ...synthetic, SCRY_SEMANTIC_API_KEY: "j".repeat(64) }).SCRY_SEMANTIC_API_KEY, "j".repeat(64));
});

test("semantic endpoint must be a confidential Decisions route", () => {
  for (const endpoint of [
    "http://openrouter.ai/api/alpha/decisions",
    "http://127.0.0.1:9000/api/alpha/decisions",
    "https://user:pass@openrouter.ai/api/alpha/decisions",
    "https://openrouter.ai/api/alpha/decisions?key=1",
    "https://openrouter.ai/api/alpha/decisions#x",
    " https://openrouter.ai/api/alpha/decisions",
    "openrouter.ai/api/alpha/decisions",
  ]) {
    assert.throws(() => appEnvVars({ ...complete, ...SEMANTIC, SCRY_SEMANTIC_ENDPOINT: endpoint }), /SCRY_SEMANTIC_ENDPOINT/, endpoint);
  }
  // Chat completions is not the Decisions route.
  assert.throws(
    () => appEnvVars({ ...complete, ...SEMANTIC, SCRY_SEMANTIC_ENDPOINT: "https://openrouter.ai/api/v1/chat/completions" }),
    /Jev Decisions route/,
  );
});

test("semantic model and reservation are explicit and bounded", () => {
  for (const model of ["openrouter/auto", "jev 1.13", "jev\n1.13", "m".repeat(201)]) {
    assert.throws(() => appEnvVars({ ...complete, ...SEMANTIC, SCRY_SEMANTIC_MODEL: model }), /SCRY_SEMANTIC_MODEL/);
  }
  for (const reservation of ["0", "-1", "2000.5", "1e3", "02000", "3500001", "two"]) {
    assert.throws(
      () => appEnvVars({ ...complete, ...SEMANTIC, SCRY_SEMANTIC_RESERVATION_MICROS: reservation }),
      /SCRY_SEMANTIC_RESERVATION_MICROS/,
      reservation,
    );
  }
  assert.equal(appEnvVars({ ...complete, ...SEMANTIC, SCRY_SEMANTIC_RESERVATION_MICROS: "3500000" }).SCRY_SEMANTIC_RESERVATION_MICROS, "3500000");
  assert.throws(() => appEnvVars({ ...complete, ...SEMANTIC, SCRY_SEMANTIC_API_KEY: "short" }), /SCRY_SEMANTIC_API_KEY/);
});

test("container start log names the semantic mode without any credential", () => {
  assert.deepEqual(semanticStartSummary(appEnvVars(complete)), { semantic: "off" });
  const shared = semanticStartSummary(appEnvVars({ ...complete, ...SEMANTIC }));
  assert.deepEqual(shared, {
    semantic: "on", host: "openrouter.ai", model: "typesafe/jev-1.13", reservation_micros: "2000", key: "model",
  });
  const dedicated = semanticStartSummary(appEnvVars({ ...complete, ...SEMANTIC, SCRY_SEMANTIC_API_KEY: "j".repeat(64) }));
  assert.equal(dedicated.key, "semantic");
  const logged = JSON.stringify([shared, dedicated]);
  for (const secret of ["j".repeat(64), complete.SCRY_MODEL_API_KEY, complete.SCRY_SECRET, complete.SCRY_BACKUP_REMOTE_TOKEN]) {
    assert.equal(logged.includes(secret), false);
  }
});

test("committed environments: only production configures Decisions, and it validates", () => {
  const source = readFileSync(new URL("./wrangler.jsonc", import.meta.url), "utf8");
  // Whole-line comments only; URLs inside strings also contain "//".
  const config = JSON.parse(source.split("\n").filter(line => !/^\s*\/\//.test(line)).join("\n"));
  for (const name of ["staging", "mistystep-prod"]) {
    const vars = config.env[name].vars;
    assert.deepEqual(SEMANTIC_NAMES.filter(key => key in vars), [], `${name} must not configure semantic assessments`);
  }
  const production = config.env.production.vars;
  // The provider key is a secret binding, never a committed var.
  assert.equal("SCRY_SEMANTIC_API_KEY" in production, false);
  assert.equal("SCRY_MODEL_API_KEY" in production, false);
  const secrets = Object.fromEntries(Object.entries(complete).filter(([key]) => !(key in production) && !key.startsWith("SCRY_SEMANTIC_")));
  const vars = appEnvVars({ ...secrets, ...production });
  assert.equal(vars.SCRY_SEMANTIC_ENDPOINT, "https://openrouter.ai/api/alpha/decisions");
  assert.equal(vars.SCRY_SEMANTIC_MODEL, "typesafe/jev-1.13");
  assert.equal(vars.SCRY_SEMANTIC_RESERVATION_MICROS, "2000");
  assert.equal(vars.SCRY_SEMANTIC_API_KEY, "");
});

test("optional Exa settings reach only the application, never the scheduled backup", () => {
  const absent = appEnvVars(complete);
  assert.equal(absent.SCRY_EXA_API_KEY, "");
  assert.equal(absent.SCRY_EXA_ENDPOINT, "");
  const configured = appEnvVars({
    ...complete,
    SCRY_EXA_API_KEY: "e".repeat(64),
    SCRY_EXA_ENDPOINT: "https://api.exa.ai",
  });
  assert.equal(configured.SCRY_EXA_API_KEY, "e".repeat(64));
  assert.equal(configured.SCRY_EXA_ENDPOINT, "https://api.exa.ai");
  for (const name of ["SCRY_EXA_API_KEY", "SCRY_EXA_ENDPOINT"]) {
    assert.equal(name in backupExecEnv(configured), false);
    assert.equal(name in backupExec(configured).options.env, false);
  }
});

test("invalid Exa secrets and endpoints fail closed", () => {
  for (const key of ["short", "x".repeat(4097), "x".repeat(32) + "\n", " ".repeat(32), 42]) {
    assert.throws(() => appEnvVars({ ...complete, SCRY_EXA_API_KEY: key }), /SCRY_EXA_API_KEY/);
  }
  for (const endpoint of [
    "http://api.exa.ai", "http://127.0.0.1:9000", "https://user:pass@api.exa.ai",
    "https://api.exa.ai/other", "https://api.exa.ai?key=x", "https://api.exa.ai#x",
    " api.exa.ai", "https://api.exa.ai ",
  ]) {
    assert.throws(() => appEnvVars({ ...complete, SCRY_EXA_ENDPOINT: endpoint }), /SCRY_EXA_ENDPOINT/, endpoint);
  }
  assert.throws(() => appEnvVars({ ...complete, SCRY_EXA_ENDPOINT: null }), /SCRY_EXA_ENDPOINT/);
  assert.throws(() => appEnvVars({ ...complete, SCRY_GENERATION_DAILY_BUDGET_MICROS: "1000000" }), /generation limits/);
  assert.throws(() => appEnvVars({ ...complete, SCRY_GENERATION_RESERVATION_MICROS: "200000" }), /generation limits/);
});

test("container idle policy is explicit and bounded", () => {
  assert.equal(containerSleepAfter({ SCRY_SLEEP_AFTER: "1m" }), "1m");
  assert.equal(containerSleepAfter({ SCRY_SLEEP_AFTER: "24h" }), "24h");
  assert.throws(() => containerSleepAfter({ SCRY_SLEEP_AFTER: "never" }), /SCRY_SLEEP_AFTER/);
});

test("scheduled backup exec receives only the server's recovery configuration", () => {
  const vars = appEnvVars(complete);
  const exec = backupExecEnv(vars);
  // Same remote, capability, directory (and therefore lock), cadence and keep
  // as the in-process server timer; nothing the backup CLI does not read.
  assert.deepEqual(exec, {
    SCRY_BACKUP_DIR: "/var/lib/scry/backups",
    SCRY_BACKUP_REMOTE_URL: complete.SCRY_BACKUP_REMOTE_URL,
    SCRY_BACKUP_REMOTE_TOKEN: complete.SCRY_BACKUP_REMOTE_TOKEN,
    SCRY_BACKUP_INTERVAL: "24h",
    SCRY_BACKUP_KEEP: "30",
  });
  for (const name of ["SCRY_SECRET", "SCRY_MODEL_API_KEY", "SCRY_OWNER_ID", "SCRY_CONTAINER_RESTORE_KEY"]) {
    assert.equal(name in exec, false, `${name} must not enter the backup process`);
  }
  for (const name of Object.keys(exec)) {
    assert.throws(() => backupExecEnv({ ...vars, [name]: "" }), new RegExp(name));
  }
  assert.throws(() => backupExecEnv(undefined), /SCRY_BACKUP_DIR/);
});

test("scheduled backup exec runs the remote-verified CLI with only its settings", () => {
  const vars = appEnvVars(complete);
  const { argv, options } = backupExec(vars);
  assert.deepEqual(argv, ["/usr/local/bin/scry", "backup", "--db", "/var/lib/scry/data/scry.sqlite", "--require-remote"]);
  // Only `env`. The regression was an exec with no env; `user` is unsafe here.
  assert.deepEqual(Object.keys(options), ["env"]);
  assert.deepEqual(options.env, backupExecEnv(vars));
  assert.throws(() => backupExec({ ...vars, SCRY_BACKUP_REMOTE_URL: undefined }), /SCRY_BACKUP_REMOTE_URL/);
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
