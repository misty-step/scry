const SNAPSHOT_KEY = /^scry-\d{8}T\d{6}\.\d{9}Z-[a-f0-9]{32}\.scry-backup\.zip$/;
const SHA256 = /^[a-f0-9]{64}$/;

// The CLI reports one error line. Keep only that line and strip URLs, bearer
// material, and long opaque runs, so a failed run is diagnosable from logs
// without exposing capabilities or stdout receipts.
export function failureReason(text) {
  const lines = String(text ?? "").split(/\r?\n/).map(line => line.trim()).filter(Boolean);
  return (lines.at(-1) || "no error output")
    .replace(/https?:\/\/\S+/gi, "<url>")
    .replace(/bearer\s+\S+/gi, "Bearer <redacted>")
    .replace(/[A-Za-z0-9+/=_-]{32,}/g, "<redacted>")
    .slice(0, 240);
}

export async function runBackupCycle({ running, execute, latest, head, now = Date.now() }) {
  // A stopped container has no live disk to back up. Never wake it from an
  // archive merely to run the daily check; that could discard unbacked writes.
  if (!running()) {
    const key = await latest();
    const object = await head(key);
    if (!object || !SHA256.test(object.customMetadata?.sha256 || "") || object.size <= 0) {
      throw new Error("idle recovery object is not independently identified");
    }
    const timestamp = Date.parse(key.slice(5, 9) + "-" + key.slice(9, 11) + "-" + key.slice(11, 13) + "T" + key.slice(14, 16) + ":" + key.slice(16, 18) + ":" + key.slice(18, 20) + "Z");
    if (!SNAPSHOT_KEY.test(key) || !Number.isFinite(timestamp) || timestamp > now) {
      throw new Error("idle recovery snapshot identity is invalid");
    }
    const ageSeconds = Math.floor((now - timestamp) / 1000);
    return { state: ageSeconds > 86400 ? "idle_stale" : "idle", key, ageSeconds };
  }

  const result = await execute();
  if (result.exitCode !== 0) {
    throw new Error(`scheduled remote backup command failed (exit ${result.exitCode}): ${failureReason(result.stderr)}`);
  }
  let record;
  try {
    record = JSON.parse(result.stdout);
  } catch {
    throw new Error("scheduled backup returned no valid receipt");
  }
  if (!record.remote || !SNAPSHOT_KEY.test(record.remote_key) || !SHA256.test(record.sha256) || record.bytes <= 0 || record.error) {
    throw new Error("scheduled backup was not remotely verified");
  }
  const object = await head(record.remote_key);
  if (!object || object.size !== record.bytes || object.customMetadata?.sha256 !== record.sha256) {
    throw new Error("scheduled backup R2 identity differs from app readback");
  }
  return { state: "backed_up", key: record.remote_key, sha256: record.sha256, bytes: record.bytes };
}

export async function stopAfterBackup({ backup, stop, log, canStop = () => true }) {
  let result;
  try {
    result = await backup();
    if (result.state !== "backed_up") throw new Error("idle stop lacks a fresh verified backup");
  } catch (error) {
    // An idle timeout is optional. Keep the only writer's disk when the
    // remote snapshot cannot be confirmed; the next cron can retry.
    log("[scry-recovery] idle stop deferred: remote backup failed", { reason: failureReason(error?.message) });
    return;
  }
  if (!canStop()) return;
  await stop();
}

// Tracks owner requests for the idle stop. Counters start at zero so a fresh
// Durable Object instance, which every deploy creates, can still stop after a
// verified idle backup. A request during that backup keeps the writer awake.
export function createActivityGate() {
  let generation = 0;
  let active = 0;
  return {
    async track(work) {
      generation++;
      active++;
      try {
        return await work();
      } finally {
        active--;
        generation++;
      }
    },
    mark: () => generation,
    idleSince: mark => generation === mark && active === 0,
  };
}

export function stopWhenIdle({ gate, backup, stop, log = (...values) => console.error(...values) }) {
  const mark = gate.mark();
  return stopAfterBackup({ backup, stop, log, canStop: () => gate.idleSince(mark) });
}