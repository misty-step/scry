// Scry Container Durable Object: one instance, one writer, exactly like the
// single-VM contract. Before each cold start the DO resolves the newest
// private R2 snapshot key and injects it as SCRY_CONTAINER_RESTORE_KEY, so
// the container always wakes with the latest acknowledged state.
import { Container } from "@cloudflare/containers";
import { createActivityGate, runBackupCycle, stopWhenIdle } from "./backup-cycle.mjs";
import { createStartupOnlyFetch, fatalContainerError } from "./container-lifecycle.mjs";
import { latestSnapshotKey } from "./recovery-key.mjs";
import { appEnvVars, backupExec, containerSleepAfter, semanticStartSummary } from "./runtime-env.mjs";

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
    this.activity = createActivityGate();
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
    return this.activity.track(() => this.startupOnlyFetch(request));
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
        // exec does not inherit the start environment; backupExec supplies it.
        const { argv, options } = backupExec(this.envVars);
        const process = await this.ctx.container.exec(argv, options);
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
    await stopWhenIdle({
      gate: this.activity,
      backup: () => this.scheduledBackup(),
      stop: () => this.stop(),
    });
  }

  // Lifecycle observability: the platform reports container start/stop/error
  // through these hooks; surface them in the Worker log tail.
  onStart() {
    // envVars reach the process only at start; record which semantic mode
    // this instance received, without any credential.
    console.log("[scry-container] started", semanticStartSummary(this.envVars));
  }
  onStop({ exitCode, reason } = {}) {
    console.log("[scry-container] stopped", { exitCode, reason });
  }
  onError(error) {
    fatalContainerError(error);
  }
}
