// Temporary, private-preview-only recovery gate. Never bind this to a public
// route or production writer. Source bytes remain in Cloudflare throughout.
const LIMIT = 1024 * 1024;
const MAX_PHOTOS = 32;
const MAX_RECORDS = 1000;
const WINDOW_MS = 15 * 60_000;
const canonical = value => JSON.stringify(value, (_, item) => item && typeof item === 'object' && !Array.isArray(item)
  ? Object.fromEntries(Object.keys(item).sort().map(key => [key, item[key]])) : item);
const require = value => { if (!value) throw Error('RECONCILIATION_FAILED'); };
const reply = (body, status = 200) => Response.json(body, {status,
  headers: {'Cache-Control': 'private, no-store', 'X-Content-Type-Options': 'nosniff'}});
const sha = async bytes => Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)),
  value => value.toString(16).padStart(2, '0')).join('');

// Compare the FIRST archive after native restore, before backup() saves its new
// status. No general normalization/defaulting: incompatible formats fail closed.
export function reconcile(source, actual, space, started, finished) {
  require(Number.isSafeInteger(started) && Number.isSafeInteger(finished) && finished >= started);
  require(canonical(Object.keys(source).sort()) === canonical(Object.keys(actual).sort()));
  require(source.format === actual.format && source.schema === actual.schema && source.algorithm === actual.algorithm);
  require(Number.isSafeInteger(actual.created_ms) && actual.created_ms >= started && actual.created_ms <= finished);
  const expected = structuredClone(source.app), got = actual.app;
  require(Number.isSafeInteger(expected.revision) && expected.revision < Number.MAX_SAFE_INTEGER);
  require(got.revision === expected.revision + 1);
  require(got.restored_paused === true);
  require(typeof got.csrf === 'string' && /^[a-f0-9]{48}$/.test(got.csrf) && got.csrf !== expected.csrf);
  expected.revision++;
  expected.csrf = got.csrf;
  expected.restored_paused = true;
  expected.backup = {completed_ms: null, key: null, sha256: null, error: null};
  for (const job of Object.values(expected.jobs)) {
    if (['queued', 'candidates'].includes(job.status)) Object.assign(job,
      {status: 'paused', error: 'Preparation is paused in this restored learning space.'});
    else if (['sent', 'critic-sent'].includes(job.status)) Object.assign(job,
      {status: 'unknown', error: 'Restored request has an uncertain paid outcome. Its usage is retained.'});
  }
  for (const [id, assessment] of Object.entries(expected.assessments)) {
    if (!['queued', 'sent'].includes(assessment.status)) continue;
    assessment.status = assessment.status === 'sent' ? 'unknown' : 'paused';
    assessment.error = 'The check is paused after restore. Its saved answer can be checked by you.';
    const occurrence = expected.occurrence;
    if (occurrence?.assessment_id === id && occurrence.phase === 'checking') {
      occurrence.phase = 'self-check';
      const concept = expected.concepts[occurrence.concept_id];
      if (concept) {
        const exposed = got.concepts[occurrence.concept_id]?.exposure_ms;
        require(Number.isSafeInteger(exposed) && exposed >= started && exposed <= finished);
        concept.exposure_ms = exposed;
      }
    }
  }
  require(source.photos.length === actual.photos.length);
  const before = new Map(source.photos.map(item => [item.photo.key, item]));
  const after = new Map(actual.photos.map(item => [item.photo.key, item]));
  require(before.size === source.photos.length && after.size === actual.photos.length);
  const mapped = new Map();
  for (const [id, goal] of Object.entries(expected.goals)) if (goal.photo) {
    const old = goal.photo.key, remapped = got.goals[id]?.photo;
    require(remapped && remapped.key !== old && remapped.key.startsWith('photos/' + space + '/'));
    if (mapped.has(old)) require(mapped.get(old) === remapped.key);
    else require(![...mapped.values()].includes(remapped.key));
    const a = before.get(old), b = after.get(remapped.key);
    require(a && b && a.bytes === b.bytes);
    require(canonical({...a.photo, key: remapped.key}) === canonical(b.photo));
    mapped.set(old, remapped.key);
    goal.photo.key = remapped.key;
  }
  require(mapped.size === after.size && canonical(expected) === canonical(got));
}

function pin(env) {
  const size = Number(env.SOURCE_BYTES), issued = Number(env.TRANSFER_ISSUED_MS), until = Number(env.TRANSFER_UNTIL_MS);
  if (!/^snapshots\/[a-z0-9-]{1,100}\/[0-9]+-(daily|pre-release)-[a-f0-9]{48}\.json$/.test(env.SOURCE_KEY || '') ||
      !/^(?:[a-f0-9]{32}(?:-[0-9]+)?|"[a-f0-9]{32}(?:-[0-9]+)?")$/.test(env.SOURCE_ETAG || '') || !Number.isSafeInteger(size) || size < 1 || size > LIMIT ||
      !Number.isSafeInteger(issued) || !Number.isSafeInteger(until) || issued <= 0 || until <= issued || until - issued > WINDOW_MS ||
      !/^recovery-[a-z0-9-]{1,80}$/.test(env.SPACE_NAME || '') ||
      !['TRANSFER_KEY', 'INTERNAL_KEY', 'RESTORE_TOKEN'].every(name => typeof env[name] === 'string' && env[name].length >= 32))
    throw Error('INVALID_PIN');
  return {size, issued, until};
}
function fresh(bounds) {
  if (Date.now() < bounds.issued || Date.now() >= bounds.until) throw Error('CAPABILITY_EXPIRED');
}
function bounded(source) {
  require(Array.isArray(source.photos) && source.photos.length <= MAX_PHOTOS);
  let count = 0;
  const visit = value => {
    require(!(typeof value === 'number' && Number.isInteger(value) && !Number.isSafeInteger(value)));
    if (!value || typeof value !== 'object') return;
    const keys = Object.keys(value);
    require(keys.every(key => new TextEncoder().encode(key).length <= 256));
    count += keys.length;
    require(count <= MAX_RECORDS);
    for (const item of Object.values(value)) visit(item);
  };
  visit(source.app);
}

async function restore(env, bounds) {
  fresh(bounds);
  const object = await env.SOURCE.get(env.SOURCE_KEY, {onlyIf: {etagMatches: env.SOURCE_ETAG.replaceAll('"', '')}});
  if (!object || typeof object.arrayBuffer !== 'function' || object.size !== bounds.size ||
      object.etag !== env.SOURCE_ETAG.replaceAll('"', '')) throw Error('SOURCE_CHANGED');
  const bytes = new Uint8Array(await object.arrayBuffer());
  if (bytes.length !== bounds.size) throw Error('SOURCE_CHANGED');
  const sourceHash = await sha(bytes);
  const source = JSON.parse(new TextDecoder('utf-8', {fatal: true}).decode(bytes));
  bounded(source);
  const prefix = 'snapshots/' + env.SPACE_NAME + '/';
  const before = await env.BACKUPS.list({prefix, limit: 1});
  if (before.truncated || before.objects.length) throw Error('TARGET_BACKUPS_NOT_EMPTY');
  const stub = env.TARGET.get(env.TARGET.idFromName(env.SPACE_NAME));
  const headers = {'x-scry-internal-key': env.INTERNAL_KEY, 'x-scry-owner': 'private-owner'};
  const storage = async () => {
    fresh(bounds);
    const response = await stub.fetch('https://restore.invalid/__recovery/storage', {headers});
    if (response.status !== 200) throw Error('STORAGE_BOUND');
    const value = await response.json();
    if (value.format !== 'scry-recovery-storage-v1' || ![512, 1024, 2048, 4096].includes(value.page_size) ||
        !Number.isSafeInteger(value.database_bytes) || value.database_bytes < 0 || value.database_bytes > 32 * 1024 * 1024)
      throw Error('STORAGE_BOUND');
    return value;
  };
  const empty = await storage();
  if (empty.database_bytes > 65536) throw Error('STORAGE_BOUND');
  fresh(bounds);
  const started = Date.now();
  const imported = await stub.fetch('https://restore.invalid/operator/restore', {method: 'POST',
    headers: {...headers, 'x-scry-restore-token': env.RESTORE_TOKEN, 'x-scry-sha256': sourceHash}, body: bytes});
  await imported.body?.cancel();
  if (imported.status !== 200) throw Error('RESTORE_REFUSED');
  fresh(bounds);
  const response = await stub.fetch('https://restore.invalid/__internal/daily', {method: 'POST', headers});
  await response.body?.cancel();
  if (response.status !== 200) throw Error('BACKUP_FAILED');
  fresh(bounds);
  const targetSize = await storage();
  const after = await env.BACKUPS.list({prefix, limit: 2});
  if (after.truncated || after.objects.length !== 1) throw Error('BACKUP_UNIDENTIFIED');
  const snapshot = await env.BACKUPS.get(after.objects[0].key);
  if (!snapshot || snapshot.size <= 0 || snapshot.size > LIMIT) throw Error('BACKUP_UNVERIFIED');
  const readback = new Uint8Array(await snapshot.arrayBuffer());
  if (readback.length !== snapshot.size) throw Error('BACKUP_UNVERIFIED');
  const actual = JSON.parse(new TextDecoder('utf-8', {fatal: true}).decode(readback));
  reconcile(source, actual, env.SPACE_NAME, started, Date.now());
  fresh(bounds);
  return {format: 'scry-cloud-restore-proof-v1', canonical_application_equal: true,
    source_sha256: sourceHash, snapshot_sha256: await sha(readback),
    source_bytes: bytes.length, snapshot_bytes: readback.length,
    photos_full_bytes_verified: actual.photos.length, restore_attempts: 1, backup_attempts: 1,
    elapsed_ms: Date.now() - started, target_page_size: targetSize.page_size, target_database_bytes: targetSize.database_bytes};
}

export class RecoveryBridge {
  constructor(ctx, env) { this.ctx = ctx; this.env = env; }
  async fetch(request) {
    const env = this.env, url = new URL(request.url);
    if (url.pathname !== '/restore' || url.search) return reply({error: 'NOT_FOUND'}, 404);
    if (request.method !== 'POST') return reply({error: 'METHOD_NOT_ALLOWED'}, 405);
    if (request.headers.get('Authorization') !== 'Bearer ' + env.TRANSFER_KEY) return reply({error: 'UNAUTHORIZED'}, 403);
    let bounds;
    try { bounds = pin(env); fresh(bounds); } catch (error) { return reply({error: error.message}, 403); }
    if (request.body) {
      const reader = request.body.getReader(), first = await reader.read(); await reader.cancel();
      if (!first.done) return reply({error: 'BODY_NOT_ALLOWED'}, 400);
    }
    const admitted = await this.ctx.storage.transaction(async storage => {
      if (await storage.get('admitted')) return false;
      await storage.put('admitted', true); return true;
    });
    if (!admitted) return reply({error: 'ATTEMPT_ALREADY_SPENT'}, 409);
    try { return reply(await restore(env, bounds)); }
    catch (error) {
      const known = new Set(['SOURCE_CHANGED', 'TARGET_BACKUPS_NOT_EMPTY', 'RESTORE_REFUSED', 'BACKUP_FAILED',
        'STORAGE_BOUND', 'BACKUP_UNIDENTIFIED', 'BACKUP_UNVERIFIED', 'RECONCILIATION_FAILED', 'CAPABILITY_EXPIRED']);
      return reply({error: known.has(error.message) ? error.message : 'TRANSFER_FAILED'}, 503);
    }
  }
}
export default {
  async fetch(request, env) {
    // Exactly one gate object, regardless of the supplied URL or a caller's ID.
    return env.GATE.get(env.GATE.idFromName('one-authorized-rehearsal')).fetch(request);
  },
};
