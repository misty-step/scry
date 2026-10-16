// Deliberately use Sentry's envelope protocol, not automatic collectors: no
// Request, Response or thrown value enters this reporter's interface.
// Existing project's public ingestion DSN, not an auth/upload credential.
const DSN = "https://a64d1b367f9a28f503fd81c733198d14@o4510313803677696.ingest.us.sentry.io/4510313841426432";
const CATEGORIES = new Set(["request_exception", "upstream_5xx"]);
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const EDGE = { logger: "scry.edge", release: "scry-edge", label: "Scry edge", boundary: "go-container-edge" };
const RUST = { logger: "scry.rust", release: "scry-rust", label: "Scry Rust", boundary: "rust-worker" };
let nextSendAt = 0;

async function send(env, category, profile) {
  const dsn = new URL(DSN);
  const event = {
    event_id: crypto.randomUUID().replaceAll("-", ""),
    timestamp: Date.now() / 1000,
    platform: "javascript",
    level: "error",
    logger: profile.logger,
    environment: env.SENTRY_ENVIRONMENT,
    release: `${profile.release}@${env.SCRY_VERSION.id}`,
    message: `${profile.label} ${category}`,
    // Do not let a controlled Rust staging drill consume production's
    // first-seen alert. Preserve the already-operated Go grouping unchanged.
    fingerprint: profile === RUST ? [profile.release, env.SENTRY_ENVIRONMENT, category] : [profile.release, category],
    tags: { boundary: profile.boundary, category },
  };
  const response = await fetch(`${dsn.origin}/api${dsn.pathname}/envelope/`, {
    method: "POST",
    redirect: "manual",
    headers: { "content-type": "application/x-sentry-envelope" },
    body: `${JSON.stringify({ event_id: event.event_id, dsn: dsn.href })}\n${JSON.stringify({ type: "event" })}\n${JSON.stringify(event)}\n`,
    signal: AbortSignal.timeout(2000),
  });
  if (response.status === 429) {
    const seconds = Number(response.headers.get("retry-after"));
    // Bound a sink-directed pause to one hour; never retry an event.
    nextSendAt = Math.max(nextSendAt, Date.now() + Math.min(3600, Math.max(60, seconds || 60)) * 1000);
  }
  await response.body?.cancel();
  if (response.status !== 200) throw new Error("Sentry rejected report");
}

function report(env, ctx, category, profile = EDGE) {
  try {
    if (!CATEGORIES.has(category) || !UUID.test(env.SCRY_VERSION?.id || "") ||
        !["production", "synthetic-staging"].includes(env.SENTRY_ENVIRONMENT) || Date.now() < nextSendAt) return;
    // Per-isolate cap: at most one report/minute, no queue, cache or retries.
    nextSendAt = Date.now() + 60_000;
    ctx.waitUntil(send(env, category, profile).catch(() => {
      console.warn(`[${profile.release}] telemetry unavailable; report dropped`);
    }));
  } catch {
    // Even an invalid telemetry configuration/context cannot affect study.
    console.warn(`[${profile.release}] telemetry unavailable; report dropped`);
  }
}

// The Rust entrypoint passes only deployment bindings/context and a literal
// category. Keep one DSN, transport, privacy allowlist and bounded-send policy.
export function reportRust(env, ctx, category) {
  report(env, ctx, category, RUST);
}

export function observeFetch(handler) {
  return async (request, env, ctx) => {
    try {
      const response = await handler(request, env, ctx);
      if (response.status >= 500) report(env, ctx, "upstream_5xx");
      return response;
    } catch (error) {
      report(env, ctx, "request_exception");
      throw error;
    }
  };
}
