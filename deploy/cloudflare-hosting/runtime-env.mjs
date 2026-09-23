const REQUIRED = [
  "SCRY_BOOT_MODE",
  "SCRY_DATA_CLASS",
  "SCRY_OWNER_ID",
  "SCRY_SECRET",
  "SCRY_BASE_URL",
  "SCRY_BACKUP_REMOTE_URL",
  "SCRY_BACKUP_REMOTE_TOKEN",
  "SCRY_GENERATION_DAILY_BUDGET_MICROS",
  "SCRY_GENERATION_RESERVATION_MICROS",
];
const MODEL_FIELDS = [
  "SCRY_MODEL_ENDPOINT",
  "SCRY_MODEL_API_KEY",
  "SCRY_MODEL",
  "SCRY_MODEL_PROVIDER",
];
const EXA_FIELDS = ["SCRY_EXA_API_KEY", "SCRY_EXA_ENDPOINT"];
// Jev Decisions for semantic recall and the prepublication critic. The key is
// optional: empty means the application reuses SCRY_MODEL_API_KEY.
const SEMANTIC_FIELDS = [
  "SCRY_SEMANTIC_ENDPOINT",
  "SCRY_SEMANTIC_API_KEY",
  "SCRY_SEMANTIC_MODEL",
  "SCRY_SEMANTIC_RESERVATION_MICROS",
];
const DECISIONS_PATH = "/api/alpha/decisions";
// Exactly what `scry backup` reads (cmd/scry recoveryConfig). The shared
// directory also keeps the CLI and the in-process timer behind one lock.
const BACKUP_EXEC_FIELDS = [
  "SCRY_BACKUP_DIR",
  "SCRY_BACKUP_REMOTE_URL",
  "SCRY_BACKUP_REMOTE_TOKEN",
  "SCRY_BACKUP_INTERVAL",
  "SCRY_BACKUP_KEEP",
];

// A live staging exec of `scry backup` saw no SCRY_BACKUP_REMOTE_URL: exec'd
// processes do not get the container's per-start environment. Pass the backup
// its recovery configuration explicitly, and nothing else.
export function backupExecEnv(envVars) {
  const result = {};
  for (const name of BACKUP_EXEC_FIELDS) {
    if (typeof envVars?.[name] !== "string" || envVars[name].length === 0) {
      throw new Error(`${name} is required for the scheduled backup`);
    }
    result[name] = envVars[name];
  }
  return result;
}

// The scheduled backup exec. Options carry only the backup settings. Never set
// `user`: exec already runs as the image USER (uid 1000, scry), and on live
// staging `user: "scry"` failed internally while `user: "1000"` ran as root.
export function backupExec(envVars) {
  return {
    argv: ["/usr/local/bin/scry", "backup", "--db", "/var/lib/scry/data/scry.sqlite", "--require-remote"],
    options: { env: backupExecEnv(envVars) },
  };
}

export function containerSleepAfter(env) {
  if (!new Set(["1m", "24h"]).has(env.SCRY_SLEEP_AFTER)) {
    throw new Error("SCRY_SLEEP_AFTER must be the staged 1m proof window or the reviewed 24h production policy");
  }
  return env.SCRY_SLEEP_AFTER;
}

function boundedSecret(value) {
  return value.length >= 32 && value.length <= 4096 && !/[\r\n\0]/.test(value);
}

// Semantic mode is off unless an endpoint is configured. Off means every
// semantic setting is empty, so the application provably sends nothing. On
// requires a complete HTTPS Decisions URL, one explicit model, a positive
// reservation inside the daily allowance, and a bearer key. Every request
// carries that key and private learner text, so partial or unsafe settings
// refuse to start the container rather than degrade.
function semanticEnvVars(env, modelApiKey) {
  const semantic = Object.fromEntries(SEMANTIC_FIELDS.map(name => [name, typeof env[name] === "string" ? env[name] : ""]));
  if (semantic.SCRY_SEMANTIC_ENDPOINT.length === 0) {
    const stray = SEMANTIC_FIELDS.filter(name => semantic[name].length > 0);
    if (stray.length > 0) {
      throw new Error(`semantic configuration without SCRY_SEMANTIC_ENDPOINT is incomplete; unset ${stray.join(", ")} or configure the endpoint`);
    }
    return semantic;
  }
  let endpoint;
  try {
    endpoint = new URL(semantic.SCRY_SEMANTIC_ENDPOINT);
  } catch {
    throw new Error("SCRY_SEMANTIC_ENDPOINT must be a complete HTTPS URL");
  }
  if (endpoint.protocol !== "https:" || endpoint.username || endpoint.password || endpoint.search || endpoint.hash
      || semantic.SCRY_SEMANTIC_ENDPOINT !== semantic.SCRY_SEMANTIC_ENDPOINT.trim()) {
    throw new Error("SCRY_SEMANTIC_ENDPOINT must be a complete HTTPS URL without credentials, query, or fragment");
  }
  if (!endpoint.pathname.endsWith(DECISIONS_PATH)) {
    throw new Error(`SCRY_SEMANTIC_ENDPOINT must be a Jev Decisions route ending in ${DECISIONS_PATH}`);
  }
  const model = semantic.SCRY_SEMANTIC_MODEL;
  if (model.length === 0 || model.length > 200 || /[\s\0]/.test(model) || model === "openrouter/auto") {
    throw new Error("SCRY_SEMANTIC_MODEL must be one explicit bounded model identifier");
  }
  const reservation = semantic.SCRY_SEMANTIC_RESERVATION_MICROS;
  if (!/^[1-9][0-9]{0,15}$/.test(reservation) || Number(reservation) > Number(env.SCRY_GENERATION_DAILY_BUDGET_MICROS)) {
    throw new Error("SCRY_SEMANTIC_RESERVATION_MICROS must be a positive integer no larger than SCRY_GENERATION_DAILY_BUDGET_MICROS");
  }
  if (semantic.SCRY_SEMANTIC_API_KEY.length > 0) {
    if (!boundedSecret(semantic.SCRY_SEMANTIC_API_KEY)) {
      throw new Error("SCRY_SEMANTIC_API_KEY must be a bounded non-control secret");
    }
  } else if (modelApiKey.length === 0) {
    throw new Error("semantic assessments require SCRY_SEMANTIC_API_KEY or the complete model configuration's SCRY_MODEL_API_KEY");
  }
  return semantic;
}
// Web research is optional. A missing key means no Exa request; a supplied
// endpoint is still validated so accidental credential disclosure fails closed.
function exaEnvVars(env) {
  for (const name of EXA_FIELDS) {
    if (env[name] !== undefined && typeof env[name] !== "string") {
      throw new Error(`${name} must be a string when configured`);
    }
  }
  const exa = Object.fromEntries(EXA_FIELDS.map(name => [name, env[name] ?? ""]));
  if (exa.SCRY_EXA_API_KEY && (!boundedSecret(exa.SCRY_EXA_API_KEY) || /\s/.test(exa.SCRY_EXA_API_KEY))) {
    throw new Error("SCRY_EXA_API_KEY must be a bounded non-control secret");
  }
  if (exa.SCRY_EXA_ENDPOINT) {
    let url;
    try {
      url = new URL(exa.SCRY_EXA_ENDPOINT);
    } catch {
      throw new Error("SCRY_EXA_ENDPOINT must be a complete HTTPS URL");
    }
    if (url.protocol !== "https:" || !url.hostname || url.username || url.password ||
        url.search || url.hash || url.pathname !== "/" ||
        exa.SCRY_EXA_ENDPOINT !== exa.SCRY_EXA_ENDPOINT.trim()) {
      throw new Error("SCRY_EXA_ENDPOINT must be an HTTPS origin without credentials, path, query, or fragment");
    }
  }
  return exa;
}


// A non-secret summary of the semantic settings a Container starts with, for
// the Worker log. It names the key source, never a key or learner text.
export function semanticStartSummary(vars) {
  if (!vars.SCRY_SEMANTIC_ENDPOINT) return { semantic: "off" };
  return {
    semantic: "on",
    host: new URL(vars.SCRY_SEMANTIC_ENDPOINT).host,
    model: vars.SCRY_SEMANTIC_MODEL,
    reservation_micros: vars.SCRY_SEMANTIC_RESERVATION_MICROS,
    key: vars.SCRY_SEMANTIC_API_KEY ? "semantic" : "model",
  };
}

export function appEnvVars(env) {
  for (const name of REQUIRED) {
    if (typeof env[name] !== "string" || env[name].length === 0) {
      throw new Error(`${name} is required for the container runtime`);
    }
  }
  if (!new Set(["restore-required", "synthetic-fresh"]).has(env.SCRY_BOOT_MODE)) {
    throw new Error("SCRY_BOOT_MODE is invalid");
  }
  if (!new Set(["synthetic", "recovered"]).has(env.SCRY_DATA_CLASS)) {
    throw new Error("SCRY_DATA_CLASS is invalid");
  }
  if (env.SCRY_BOOT_MODE === "synthetic-fresh" && env.SCRY_DATA_CLASS !== "synthetic") {
    throw new Error("synthetic-fresh requires SCRY_DATA_CLASS=synthetic");
  }
  if (!/^[A-Za-z0-9._:@-]{1,200}$/.test(env.SCRY_OWNER_ID)) {
    throw new Error("SCRY_OWNER_ID contains unsafe header or nginx characters");
  }
  for (const name of ["SCRY_SECRET", "SCRY_BACKUP_REMOTE_TOKEN"]) {
    if (env[name].length < 32 || env[name].length > 4096 || /[\r\n\0]/.test(env[name])) {
      throw new Error(`${name} must be a bounded non-control secret`);
    }
  }
  for (const name of ["SCRY_BASE_URL", "SCRY_BACKUP_REMOTE_URL"]) {
    let url;
    try {
      url = new URL(env[name]);
    } catch {
      throw new Error(`${name} must be a complete HTTPS URL`);
    }
    if (url.protocol !== "https:" || url.username || url.password || url.search || url.hash) {
      throw new Error(`${name} must be a complete HTTPS URL without credentials, query, or fragment`);
    }
  }

  const model = Object.fromEntries(MODEL_FIELDS.map(name => [name, typeof env[name] === "string" ? env[name] : ""]));
  const configuredModelFields = MODEL_FIELDS.filter(name => model[name].length > 0);
  if (configuredModelFields.length > 0 && configuredModelFields.length < MODEL_FIELDS.length) {
    const missing = MODEL_FIELDS.filter(name => model[name].length === 0);
    throw new Error(`model configuration must be either complete or disabled; missing ${missing.join(", ")}`);
  }
  if (env.SCRY_DATA_CLASS === "recovered" && configuredModelFields.length === 0) {
    throw new Error(`recovered hosting requires ${MODEL_FIELDS.join(", ")}`);
  }
  if (configuredModelFields.length === MODEL_FIELDS.length) {
    let endpoint;
    try {
      endpoint = new URL(model.SCRY_MODEL_ENDPOINT);
    } catch {
      throw new Error("SCRY_MODEL_ENDPOINT must be a complete HTTPS URL");
    }
    if (endpoint.protocol !== "https:" || endpoint.username || endpoint.password || endpoint.search || endpoint.hash) {
      throw new Error("SCRY_MODEL_ENDPOINT must be a complete HTTPS URL without credentials, query, or fragment");
    }
    if (!boundedSecret(model.SCRY_MODEL_API_KEY)) {
      throw new Error("SCRY_MODEL_API_KEY must be a bounded non-control secret");
    }
    if (model.SCRY_MODEL.length > 200 || /[\r\n\0]/.test(model.SCRY_MODEL) || model.SCRY_MODEL === "openrouter/auto") {
      throw new Error("SCRY_MODEL must be one explicit bounded model identifier");
    }
    if (!new Set(["openrouter", "openai"]).has(model.SCRY_MODEL_PROVIDER)) {
      throw new Error("SCRY_MODEL_PROVIDER must be openrouter or openai");
    }
  }
  if (env.SCRY_GENERATION_DAILY_BUDGET_MICROS !== "3500000" || env.SCRY_GENERATION_RESERVATION_MICROS !== "500000") {
    throw new Error("generation limits must remain pinned to the product's $3.50 rolling-day allowance and $0.50 conservative reservation");
  }
  const semantic = semanticEnvVars(env, model.SCRY_MODEL_API_KEY);
  const exa = exaEnvVars(env);
  return {
    SCRY_MODE: "production",
    SCRY_BOOT_MODE: env.SCRY_BOOT_MODE,
    SCRY_DATA_CLASS: env.SCRY_DATA_CLASS,
    SCRY_OWNER_ID: env.SCRY_OWNER_ID,
    SCRY_SECRET: env.SCRY_SECRET,
    SCRY_BASE_URL: env.SCRY_BASE_URL,
    SCRY_REDIRECT_HOSTS: env.SCRY_REDIRECT_HOSTS || "",
    SCRY_TRUSTED_PROXY_IPS: "127.0.0.1",
    SCRY_BACKUP_REMOTE_URL: env.SCRY_BACKUP_REMOTE_URL,
    SCRY_BACKUP_REMOTE_TOKEN: env.SCRY_BACKUP_REMOTE_TOKEN,
    SCRY_CONTAINER_RESTORE_KEY: "",
    SCRY_MODEL_ENDPOINT: model.SCRY_MODEL_ENDPOINT,
    SCRY_MODEL_API_KEY: model.SCRY_MODEL_API_KEY,
    SCRY_MODEL: model.SCRY_MODEL,
    SCRY_MODEL_PROVIDER: model.SCRY_MODEL_PROVIDER,
    SCRY_EXA_API_KEY: exa.SCRY_EXA_API_KEY,
    SCRY_EXA_ENDPOINT: exa.SCRY_EXA_ENDPOINT,
    SCRY_SEMANTIC_ENDPOINT: semantic.SCRY_SEMANTIC_ENDPOINT,
    SCRY_SEMANTIC_API_KEY: semantic.SCRY_SEMANTIC_API_KEY,
    SCRY_SEMANTIC_MODEL: semantic.SCRY_SEMANTIC_MODEL,
    SCRY_SEMANTIC_RESERVATION_MICROS: semantic.SCRY_SEMANTIC_RESERVATION_MICROS,
    SCRY_GENERATION_DAILY_BUDGET_MICROS: env.SCRY_GENERATION_DAILY_BUDGET_MICROS,
    SCRY_GENERATION_RESERVATION_MICROS: env.SCRY_GENERATION_RESERVATION_MICROS,
    SCRY_DB: "/var/lib/scry/data/scry.sqlite",
    SCRY_ADDR: "127.0.0.1:8081",
    SCRY_BACKUP_DIR: "/var/lib/scry/backups",
    SCRY_BACKUP_INTERVAL: "24h",
    SCRY_BACKUP_KEEP: "30",
  };
}
