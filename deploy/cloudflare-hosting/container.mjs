// Scry Container Durable Object: one instance, one writer, exactly like the
// single-VM contract. Before each cold start the DO resolves the newest
// private R2 snapshot key and injects it as SCRY_CONTAINER_RESTORE_KEY, so
// the container always wakes with the latest acknowledged state.
import { Container } from "@cloudflare/containers";
import { runBackupCycle, stopAfterBackup } from "./backup-cycle.mjs";
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
    this.activityGeneration = (this.activityGeneration || 0) + 1;
    this.activeRequests = (this.activeRequests || 0) + 1;
    try {
      return await this.startupOnlyFetch(request);
    } finally {
      this.activeRequests--;
      this.activityGeneration++;
    }
  }

  forwardToContainer(request) {
    return super.fetch(request);
  }

  async scheduledBackup() {
    if (this.backupInFlight) return this.backupInFlight;
    const task = runBackupCycle({
      running: () => this.ctx.container.running,
      latest: () => latestSnapshotKey(this.env.RECOVERY),
      head: key => this.env.RECOVERY.head(key),
      execute: async () => {
        // The existing CLI snapshots SQLite consistently and checks the exact
        // uploaded bytes. Never log stdout: the receipt has local paths. The
        // cycle logs only a redacted last stderr line when the command fails.
        const process = await this.ctx.container.exec([
          "/usr/local/bin/scry", "backup", "--db", "/var/lib/scry/data/scry.sqlite", "--require-remote",
        ]);
        const output = await process.output();
        const decoder = new TextDecoder();
        return { exitCode: output.exitCode, stdout: decoder.decode(output.stdout), stderr: decoder.decode(output.stderr) };
      },
    });
    this.backupInFlight = task;
    try {
      return await task;
    } finally {
      if (this.backupInFlight === task) this.backupInFlight = undefined;
    }
  }

  async onActivityExpired() {
    const before = this.activityGeneration || 0;
    await stopAfterBackup({
      backup: () => this.scheduledBackup(),
      stop: () => this.stop(),
      log: message => console.error(message),
      canStop: () => this.activityGeneration === before && !this.activeRequests,
    });
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
