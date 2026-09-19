// Scry edge Worker: Cloudflare Access gate in front of the single private
// application container. The Access JWT (Cf-Access-Jwt-Assertion) is
// validated against the account's Zero Trust team-domain JWKS before any
// request reaches the container. A narrow probe token lets operations reach
// /healthz and /readyz without an Access login; it grants nothing else.
import { jwtVerify, createRemoteJWKSet } from "jose";
import { getContainer } from "@cloudflare/containers";
import { ScryContainer } from "./container.mjs";

export { ScryContainer };

const PROBE_PATHS = new Set(["/healthz", "/readyz"]);

let jwksCache = null;
function jwks(teamDomain) {
  const url = new URL(`${teamDomain}/cdn-cgi/access/certs`);
  if (!jwksCache || jwksCache.url !== url.href) {
    jwksCache = { url: url.href, set: createRemoteJWKSet(url) };
  }
  return jwksCache.set;
}

function constantTimeEqual(a, b) {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}

function probeAuthorized(request, env) {
  if (!env.SCRY_PROBE_TOKEN) return false;
  const supplied = request.headers.get("authorization") || "";
  return constantTimeEqual(supplied, `Bearer ${env.SCRY_PROBE_TOKEN}`);
}

async function accessAuthorized(request, env) {
  if (!env.SCRY_ACCESS_TEAM_DOMAIN || !env.SCRY_ACCESS_AUD) return false;
  const token = request.headers.get("cf-access-jwt-assertion");
  if (!token) return false;
  try {
    await jwtVerify(token, jwks(env.SCRY_ACCESS_TEAM_DOMAIN), {
      issuer: env.SCRY_ACCESS_TEAM_DOMAIN,
      audience: env.SCRY_ACCESS_AUD,
    });
    return true;
  } catch {
    return false;
  }
}

function denied(body, status) {
  return new Response(body, {
    status,
    headers: { "content-type": "text/plain; charset=utf-8", "cache-control": "private, no-store" },
  });
}

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    // Operation probes: anonymous callers get nothing; the token holder gets
    // liveness/readiness only, straight from the container.
    if (PROBE_PATHS.has(url.pathname) && (request.method === "GET" || request.method === "HEAD")) {
      if (!probeAuthorized(request, env)) return denied("Unauthorized\n", 401);
    } else if (!(await accessAuthorized(request, env))) {
      return denied("Private access required.\n", 403);
    }

    // Single named instance: one writer, mirroring the one-VM contract.
    const container = getContainer(env.SCRY_CONTAINER, "singleton");
    return container.fetch(request);
  },
};
