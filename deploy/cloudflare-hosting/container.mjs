// Scry Container Durable Object: one instance, one writer, exactly like the
// single-VM contract. Before each cold start the DO resolves the newest
// private R2 snapshot key and injects it as SCRY_CONTAINER_RESTORE_KEY, so
// the container always wakes with the latest acknowledged state.
import { Container } from "@cloudflare/containers";
import { createStartupOnlyFetch, fatalContainerError } from "./container-lifecycle.mjs";
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
    this.startupOnlyFetch = createStartupOnlyFetch({
      baseEnvVars: this.envVars,
      bootMode: env.SCRY_BOOT_MODE,
      getState: () => this.getState(),
      isRunning: () => this.ctx.container.running,
      resolveSnapshotKey: () => latestSnapshotKey(this.env.RECOVERY),
      startAndWaitForPorts: options => this.startAndWaitForPorts(options),
      forward: request => this.forwardToContainer(request),
    });
  }

  async fetch(request) {
    return this.startupOnlyFetch(request);
  }

  forwardToContainer(request) {
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
