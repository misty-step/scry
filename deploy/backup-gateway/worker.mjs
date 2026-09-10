// A narrow recovery capability, not an application server. Callers can create
// and read snapshots, never replace/delete them or access Cloudflare's account API.
const MAX_BYTES = 16 * 1024 * 1024;
const KEY = /^scry-\d{8}T\d{6}\.\d{9}Z-[a-f0-9]{32}\.scry-backup\.zip$/;
const encoder = new TextEncoder();

function response(body, status, headers = {}) {
  return new Response(body, {
    status,
    headers: { "cache-control": "private, no-store", "x-content-type-options": "nosniff", ...headers },
  });
}

async function authorized(request, env) {
  if (!env.BACKUP_TOKEN || env.BACKUP_TOKEN.length < 32) return false;
  const supplied = request.headers.get("authorization") || "";
  const expected = `Bearer ${env.BACKUP_TOKEN}`;
  if (supplied.length !== expected.length) return false;
  return crypto.subtle.timingSafeEqual(encoder.encode(supplied), encoder.encode(expected));
}

async function boundedBody(request) {
  if (!request.body) return null;
  const reader = request.body.getReader();
  const chunks = [];
  let size = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    size += value.byteLength;
    if (size > MAX_BYTES) {
      await reader.cancel();
      return null;
    }
    chunks.push(value);
  }
  if (size === 0) return null;
  const bytes = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return bytes;
}

export default {
  async fetch(request, env) {
    if (!(await authorized(request, env))) return response("Unauthorized", 401);
    const url = new URL(request.url);
    const key = url.pathname.slice(1);
    if (url.search || !KEY.test(key)) return response("Unknown recovery object", 404);
    if (request.method === "GET" || request.method === "HEAD") {
      const object = request.method === "HEAD" ? await env.RECOVERY.head(key) : await env.RECOVERY.get(key);
      if (!object) return response("Recovery object not found", 404);
      return response(request.method === "HEAD" ? null : object.body, 200, {
        "content-type": "application/zip",
        "content-length": String(object.size),
        "etag": object.httpEtag,
      });
    }
    if (request.method !== "PUT") return response("Method not allowed", 405, { allow: "PUT, GET, HEAD" });
    const declared = request.headers.get("content-length");
    if (declared && (!/^\d+$/.test(declared) || Number(declared) > MAX_BYTES)) {
      return response("Recovery snapshot exceeds the 16 MiB sink limit", 413);
    }
    const bytes = await boundedBody(request);
    if (!bytes) return response("Recovery snapshot must contain 1 byte through 16 MiB", 413);
    const digest = await crypto.subtle.digest("SHA-256", bytes);
    const sha256 = Array.from(new Uint8Array(digest), value => value.toString(16).padStart(2, "0")).join("");
    const claimed = request.headers.get("x-amz-content-sha256");
    if (claimed && claimed !== sha256) return response("Snapshot checksum mismatch", 422);
    const object = await env.RECOVERY.put(key, bytes, {
      onlyIf: new Headers({ "if-none-match": "*" }),
      sha256: digest,
      httpMetadata: { contentType: "application/zip", cacheControl: "private, no-store" },
      customMetadata: { sha256 },
    });
    if (!object) return response("An existing recovery snapshot cannot be overwritten", 412);
    return response(null, 201, { etag: object.httpEtag, "x-content-sha256": sha256 });
  },
};
