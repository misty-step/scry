export function createStartupOnlyFetch({
  baseEnvVars,
  bootMode,
  forward,
  getState,
  isRunning,
  resolveSnapshotKey,
  startAndWaitForPorts,
}) {
  let startupInFlight;

  async function ensureReady() {
    const state = await getState();
    const running = isRunning();
    if (running && state.status === "healthy") return;
    if (running && state.status === "running") {
      // A Worker isolate can be recreated while its container is still starting.
      // Join that start without resolving or replacing its per-start environment.
      await startAndWaitForPorts();
      return;
    }
    if (state.status === "stopping" || (running && state.status !== "stopped")) {
      throw new Error(`container lifecycle is ${state.status}; refusing a competing start`);
    }

    // After a lost platform connection the SDK can report "stopped" while
    // the native process is still running. Its readiness join does not issue
    // a second start while running; if the process exits meanwhile, supply
    // the latest snapshot key rather than the empty class default.
    const key = bootMode === "synthetic-fresh" ? "" : await resolveSnapshotKey();
    await startAndWaitForPorts({
      startOptions: {
        envVars: { ...baseEnvVars, SCRY_CONTAINER_RESTORE_KEY: key },
      },
    });
  }

  return async function startupOnlyFetch(request) {
    let attempt = startupInFlight;
    if (!attempt) {
      attempt = ensureReady();
      startupInFlight = attempt;
    }
    try {
      await attempt;
    } finally {
      if (startupInFlight === attempt) startupInFlight = undefined;
    }
    return forward(request);
  };
}

export function fatalContainerError(error, log = console.error) {
  log("[scry-container] error:", String(error));
  throw error;
}
