// Scry Container Durable Object: one instance, one writer, exactly like the
// single-VM contract. Before each cold start the DO resolves the newest
// private R2 snapshot key and injects it as SCRY_CONTAINER_RESTORE_KEY, so
// the container always wakes with the latest acknowledged state.
import { Container } from "@cloudflare/containers";
import { fatalContainerError } from "./container-lifecycle.mjs";
import { latestSnapshotKey } from "./recovery-key.mjs";
import { appEnvVars, containerSleepAfter } from "./runtime-env.mjs";

export class ScryContainer extends Container {
  defaultPort = 8080;
  // Staging uses a short idle window to exercise cold-wake restore. Real-data
  // environments use 24h to reduce recurring cold starts, but platform rollouts
  // and host restarts can still replace an instance at any time.
  enableInternet = true; // private snapshot and model gateways

  constructor(ctx, env) {
    super(ctx, env);
    this.sleepAfter = containerSleepAfter(env);
    this.envVars = appEnvVars(env);
  }

  async fetch(request) {
    // A restore-required cold start cannot reach the container until a complete
    // immutable object key has been resolved. Listing failures and empty buckets
    // therefore keep edge readiness false instead of opening an empty writer.
    const key = this.env.SCRY_BOOT_MODE === "synthetic-fresh"
      ? ""
      : await latestSnapshotKey(this.env.RECOVERY);
    this.envVars = { ...this.envVars, SCRY_CONTAINER_RESTORE_KEY: key };
    return super.fetch(request);
  }

  // Lifecycle observability: the platform reports container start/stop/error
  // through these hooks; surface them in the Worker log tail.
  onStart() {
    console.log("[scry-container] started");
  }
  onStop({ exitCode, reason } = {}) {
    console.log("[scry-container] stopped", { exitCode, reason });
  }
  onError(error) {
    fatalContainerError(error);
  }
}
