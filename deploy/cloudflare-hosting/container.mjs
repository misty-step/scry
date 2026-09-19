// Scry Container Durable Object: one instance, one writer, exactly like the
// single-VM contract. Before each cold start the DO resolves the newest
// private R2 snapshot key and injects it as SCRY_CONTAINER_RESTORE_KEY, so
// the container always wakes with the latest acknowledged state.
import { Container } from "@cloudflare/containers";

const SNAPSHOT_KEY = /^scry-\d{8}T\d{6}\.\d{9}Z-[a-f0-9]{32}\.scry-backup\.zip$/;

function appEnvVars(env) {
  return {
    SCRY_MODE: "production",
    SCRY_OWNER_ID: env.SCRY_OWNER_ID || "",
    SCRY_SECRET: env.SCRY_SECRET || "",
    SCRY_BASE_URL: env.SCRY_BASE_URL || "https://scry.study",
    SCRY_REDIRECT_HOSTS: env.SCRY_REDIRECT_HOSTS || "",
    // The local nginx peer is the only connection the application ever sees.
    SCRY_TRUSTED_PROXY_IPS: "127.0.0.1",
    SCRY_BACKUP_REMOTE_URL: env.SCRY_BACKUP_REMOTE_URL || "",
    SCRY_BACKUP_REMOTE_TOKEN: env.SCRY_BACKUP_REMOTE_TOKEN || "",
    SCRY_CONTAINER_RESTORE_KEY: "",
    // Model generation stays paused in this move: the exe-internal model
    // integration is unreachable from Cloudflare. An empty endpoint leaves
    // the generation worker idle; nothing is spent or retried.
    SCRY_MODEL_ENDPOINT: "",
    SCRY_MODEL: "",
    SCRY_MODEL_PROVIDER: "",
    SCRY_DB: "/var/lib/scry/data/scry.sqlite",
    SCRY_ADDR: "127.0.0.1:8081",
    SCRY_BACKUP_DIR: "/var/lib/scry/backups",
    SCRY_BACKUP_INTERVAL: "24h",
    SCRY_BACKUP_KEEP: "5",
  };
}

export class ScryContainer extends Container {
  defaultPort = 8080;
  // Let the instance sleep after idle. The entrypoint's final verified
  // backup on shutdown bounds unacknowledged writes; the next wake restores.
  sleepAfter = "15m";
  enableInternet = true; // private snapshot gateway (+ future model endpoint)

  constructor(ctx, env) {
    super(ctx, env);
    this.envVars = appEnvVars(env);
  }

  async fetch(request) {
    // Refresh the restore key before any (re)start consumes envVars. When the
    // instance is already running its environment is fixed and this is a
    // no-op assignment, so the R2 listing only matters on cold starts.
    this.envVars = { ...this.envVars, SCRY_CONTAINER_RESTORE_KEY: await this.latestSnapshotKey() };
    return super.fetch(request);
  }

  // Resolve the newest snapshot key through the R2 binding. The bucket is
  // append/read-only by contract; listing is bounded by the 30-day lifecycle.
  async latestSnapshotKey() {
    if (!this.env?.RECOVERY) return "";
    let cursor;
    let latest = "";
    for (let i = 0; i < 100; i++) {
      const listing = await this.env.RECOVERY.list({ cursor, limit: 1000 });
      for (const item of listing.objects) {
        if (SNAPSHOT_KEY.test(item.key) && item.key > latest) latest = item.key;
      }
      if (!listing.truncated) return latest;
      cursor = listing.cursor;
    }
    return latest;
  }
}
