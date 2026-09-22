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
    if (running || state.status === "stopping") {
      throw new Error(`container lifecycle is ${state.status}; refusing a competing start`);
    }

    // Resolve the immutable snapshot only after the prior process has fully
    // stopped. Keep the key in per-start options: the class defaults retain an
    // empty key so an automatic restart in a forwarding race fails closed.
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
