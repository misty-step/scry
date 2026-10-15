// One-off revival custody only: opaque archive bytes never leave Cloudflare.
// No import, learner route, arbitrary key, listing, deletion or external fetch.
const LIMIT = 16 * 1024 * 1024;
const reply = (value, status = 200) => Response.json(value, {
  status, headers: {'Cache-Control': 'private, no-store', 'X-Content-Type-Options': 'nosniff'},
});
const error = (reason, status) => reply({error: reason}, status);
const equal = (a, b) => a.length === b.length && a.every((value, index) => value === b[index]);

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.pathname !== '/preserve' || url.search) return error('NOT_FOUND', 404);
    if (request.method !== 'POST') return error('METHOD_NOT_ALLOWED', 405);
    if (typeof env.PRESERVE_KEY !== 'string' || env.PRESERVE_KEY.length < 32) return error('UNCONFIGURED', 503);
    if (request.headers.get('Authorization') !== 'Bearer ' + env.PRESERVE_KEY) return error('UNAUTHORIZED', 403);
    const key = env.SOURCE_KEY;
    const size = Number(env.SOURCE_BYTES);
    if (typeof key !== 'string' || !/^scry-[A-Za-z0-9.-]{1,180}\.scry-backup\.zip$/.test(key) ||
        !/^[a-f0-9]{32}(?:-\d+)?$/.test(env.SOURCE_ETAG || '') ||
        !Number.isSafeInteger(size) || size <= 0 || size > LIMIT) return error('INVALID_SOURCE_PIN', 503);
    try {
      // Inputs are deployment-pinned metadata, never caller-selected records.
      if (request.body) {
        const reader = request.body.getReader();
        const first = await reader.read();
        await reader.cancel();
        if (!first.done) return error('BODY_NOT_ALLOWED', 400);
      }
      const source = await env.SOURCE.get(key, {onlyIf: {etagMatches: env.SOURCE_ETAG}});
      if (!source) return error('SOURCE_MISSING', 404);
      if (!source.body || source.etag !== env.SOURCE_ETAG || source.size !== size) return error('SOURCE_CHANGED', 409);
      const bytes = new Uint8Array(await source.arrayBuffer());
      if (bytes.length !== size) return error('SOURCE_CHANGED', 409);
      const hash = await crypto.subtle.digest('SHA-256', bytes);
      const sha256 = Array.from(new Uint8Array(hash), value => value.toString(16).padStart(2, '0')).join('');
      const destinationKey = 'preserved-go/' + sha256 + '/' + key;
      // Conditional creation also fences concurrent/replayed requests. An existing
      // object is independently read, never overwritten even if it is corrupt.
      await env.DESTINATION.put(destinationKey, bytes, {onlyIf: {etagDoesNotMatch: '*'}, sha256: hash});
      const stored = await env.DESTINATION.get(destinationKey);
      if (!stored || stored.size !== size) return error('DESTINATION_UNVERIFIED', 409);
      const readback = new Uint8Array(await stored.arrayBuffer());
      if (!equal(bytes, readback)) return error('DESTINATION_UNVERIFIED', 409);
      return reply({format: 'scry-preserved-go-archive-v1', verified: true,
        source_key: key, source_etag: source.etag, bytes: size, sha256, destination_key: destinationKey});
    } catch {
      // R2 exceptions, object metadata and archive bytes are private diagnostics.
      return error('PRESERVATION_FAILED', 503);
    }
  },
};
