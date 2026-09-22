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

export function containerSleepAfter(env) {
  if (!new Set(["1m", "24h"]).has(env.SCRY_SLEEP_AFTER)) {
    throw new Error("SCRY_SLEEP_AFTER must be the staged 1m proof window or the reviewed 24h production policy");
  }
  return env.SCRY_SLEEP_AFTER;
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
    if (model.SCRY_MODEL_API_KEY.length < 32 || model.SCRY_MODEL_API_KEY.length > 4096 || /[\r\n\0]/.test(model.SCRY_MODEL_API_KEY)) {
      throw new Error("SCRY_MODEL_API_KEY must be a bounded non-control secret");
    }
    if (model.SCRY_MODEL.length > 200 || /[\r\n\0]/.test(model.SCRY_MODEL) || model.SCRY_MODEL === "openrouter/auto") {
      throw new Error("SCRY_MODEL must be one explicit bounded model identifier");
    }
    if (!new Set(["openrouter", "openai"]).has(model.SCRY_MODEL_PROVIDER)) {
      throw new Error("SCRY_MODEL_PROVIDER must be openrouter or openai");
    }
  }
  if (env.SCRY_GENERATION_DAILY_BUDGET_MICROS !== "1000000" || env.SCRY_GENERATION_RESERVATION_MICROS !== "200000") {
    throw new Error("generation limits must remain pinned to the product's $1 daily allowance and $0.20 conservative reservation");
  }
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
    SCRY_GENERATION_DAILY_BUDGET_MICROS: env.SCRY_GENERATION_DAILY_BUDGET_MICROS,
    SCRY_GENERATION_RESERVATION_MICROS: env.SCRY_GENERATION_RESERVATION_MICROS,
    SCRY_DB: "/var/lib/scry/data/scry.sqlite",
    SCRY_ADDR: "127.0.0.1:8081",
    SCRY_BACKUP_DIR: "/var/lib/scry/backups",
    SCRY_BACKUP_INTERVAL: "24h",
    SCRY_BACKUP_KEEP: "30",
  };
}
