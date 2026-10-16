// Temporary CF-only custody bridge: pinned R2 -> existing unused Rust restore ->
// existing complete backup -> strict reconciliation. No owner route or fetch().
const LIMIT = 16 * 1024 * 1024;
const hex = /^[a-f0-9]{64}$/;
const reply = (value, status = 200) => Response.json(value, {status,
  headers: {'Cache-Control': 'private, no-store', 'X-Content-Type-Options': 'nosniff'}});
const error = (reason, status) => reply({error: reason}, status);
const sha = async bytes => Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)),
  value => value.toString(16).padStart(2, '0')).join('');
const canonical = value => JSON.stringify(value, (_, item) => item && typeof item === 'object' && !Array.isArray(item)
  ? Object.fromEntries(Object.keys(item).sort().map(key => [key, item[key]])) : item);
const require = value => {if (!value) throw Error('Reconciliation failed');};

function reconcile(source, actual, space) {
  require(source.format === actual.format && source.schema === actual.schema && source.algorithm === actual.algorithm);
  const expected = structuredClone(source.app), got = actual.app;
  require(got.restored_paused === true && Number.isSafeInteger(got.revision) && got.revision >= 1);
  require(typeof got.csrf === 'string' && got.csrf.length >= 32 && got.csrf !== expected.csrf);
  require(Object.keys(expected.assessments).length === 0);
  const backup = got.backup;
  require((backup.error === null || backup.error === 'The complete snapshot could not be verified. Review remains available; inspect operator recovery configuration.') && (backup.key === null
    ? backup.completed_ms === null && backup.sha256 === null
    : backup.key.startsWith('snapshots/' + space + '/') && hex.test(backup.sha256) && Number.isSafeInteger(backup.completed_ms)));
  // These are the existing restore's only cross-namespace changes. Never
  // normalize content, schedule, assistance, spend, history or receipt values.
  for (const key of ['revision', 'csrf', 'restored_paused', 'backup']) expected[key] = got[key];
  for (const job of Object.values(expected.jobs)) {
    if (['queued', 'candidates'].includes(job.status)) Object.assign(job,
      {status: 'paused', error: 'Preparation is paused in this restored learning space.'});
    else if (['sent', 'critic-sent'].includes(job.status)) Object.assign(job,
      {status: 'unknown', error: 'Restored request has an uncertain paid outcome. Its usage is retained.'});
    for (const key of ['critic_cache_json', 'critic_model', 'critic_parent_id']) if (!(key in job)) job[key] = null;
    if (!('critic_inherited_keys' in job)) job.critic_inherited_keys = [];
  }
  for (const spend of expected.spend) for (const key of ['response_hash', 'response_model', 'raw_response', 'response_bytes', 'response_outcome']) {
    if (!(key in spend)) spend[key] = null;
  }
  require(source.photos.length === actual.photos.length);
  const beforePhotos = new Map(source.photos.map(item => [item.photo.key, item]));
  const afterPhotos = new Map(actual.photos.map(item => [item.photo.key, item]));
  require(beforePhotos.size === source.photos.length && afterPhotos.size === actual.photos.length);
  const used = new Set();
  for (const [id, goal] of Object.entries(expected.goals)) if (goal.photo) {
    const remapped = got.goals[id]?.photo;
    require(remapped && remapped.key !== goal.photo.key && remapped.key.startsWith('photos/' + space + '/'));
    const before = beforePhotos.get(goal.photo.key), after = afterPhotos.get(remapped.key);
    require(before && after && !used.has(remapped.key));
    require(before.bytes === after.bytes);
    require(canonical({...before.photo, key: remapped.key}) === canonical(after.photo));
    used.add(remapped.key);
    goal.photo.key = remapped.key;
  }
  require(used.size === afterPhotos.size && canonical(expected) === canonical(got));
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (!['/restore', '/verify', '/diagnose'].includes(url.pathname) || url.search) return error('NOT_FOUND', 404);
    if (request.method !== 'POST') return error('METHOD_NOT_ALLOWED', 405);
    if (typeof env.TRANSFER_KEY !== 'string' || env.TRANSFER_KEY.length < 32) return error('UNCONFIGURED', 503);
    if (request.headers.get('Authorization') !== 'Bearer ' + env.TRANSFER_KEY) return error('UNAUTHORIZED', 403);
    const size = Number(env.SOURCE_BYTES), until = Number(env.TRANSFER_UNTIL_MS), space = env.SPACE_NAME;
    if (!/^converted-go\/[a-f0-9]{64}\/[a-f0-9]{64}\.json$/.test(env.SOURCE_KEY || '') || !hex.test(env.SOURCE_SHA256 || '') ||
        !Number.isSafeInteger(size) || size <= 0 || size > LIMIT || !Number.isSafeInteger(until) || until <= 0 ||
        !/^(synthetic|revival)-[a-z0-9-]{1,100}$/.test(space || '') ||
        typeof env.INTERNAL_KEY !== 'string' || env.INTERNAL_KEY.length < 32 ||
        typeof env.RESTORE_TOKEN !== 'string' || env.RESTORE_TOKEN.length < 32) return error('INVALID_PIN', 503);
    const expired = () => Date.now() >= until;
    try {
      if (request.body) {
        const reader = request.body.getReader(), first = await reader.read();
        await reader.cancel();
        if (!first.done) return error('BODY_NOT_ALLOWED', 400);
      }
      if (expired()) return error('CAPABILITY_EXPIRED', 403);
      const object = await env.SOURCE.get(env.SOURCE_KEY);
      if (!object) return error('SOURCE_MISSING', 404);
      if (object.size !== size) return error('SOURCE_CHANGED', 409);
      const bytes = new Uint8Array(await object.arrayBuffer());
      if (bytes.length !== size || await sha(bytes) !== env.SOURCE_SHA256) return error('SOURCE_CHANGED', 409);
      const source = JSON.parse(new TextDecoder('utf-8', {fatal: true}).decode(bytes));
      const stub = env.TARGET.get(env.TARGET.idFromName(space));
      const headers = {'x-scry-internal-key': env.INTERNAL_KEY, 'x-scry-owner': 'private-owner'};
      if (expired()) return error('CAPABILITY_EXPIRED', 403);
      if (url.pathname === '/diagnose') {
        const response = await stub.fetch('https://restore.invalid/operator/restore-dry-run', {method: 'POST',
          headers: {...headers, 'x-scry-restore-token': env.RESTORE_TOKEN, 'x-scry-sha256': env.SOURCE_SHA256}, body: bytes});
        if (response.status !== 200) {
          await response.body?.cancel();
          return error('INCOMPLETE_IMPORT_REPORT', 422);
        }
        const report = await response.json();
        if (!report || Array.isArray(report) || typeof report !== 'object' ||
            !Object.entries(report).every(([key, count]) => /^[a-z][a-z0-9-]{1,150}$/.test(key) && Number.isSafeInteger(count) && count > 0))
          return error('INCOMPLETE_IMPORT_REPORT', 422);
        return reply(report);
      }
      if (url.pathname === '/restore') {
        const response = await stub.fetch('https://restore.invalid/operator/restore', {method: 'POST',
          headers: {...headers, 'x-scry-restore-token': env.RESTORE_TOKEN, 'x-scry-sha256': env.SOURCE_SHA256}, body: bytes});
        await response.body?.cancel();
        if (response.status !== 200) return error('RESTORE_REFUSED', [403, 409, 422].includes(response.status) ? response.status : 503);
      } else {
        // The native restore checks used-space before decoding the body. This
        // deliberately invalid archive can prove existence but cannot initialize
        // an empty target, even if /verify is called before /restore.
        const response = await stub.fetch('https://restore.invalid/operator/restore', {method: 'POST',
          headers: {...headers, 'x-scry-restore-token': env.RESTORE_TOKEN, 'x-scry-sha256': '0'.repeat(64)}, body: new Uint8Array()});
        await response.body?.cancel();
        if (response.status !== 409) return error('VERIFY_TARGET_UNAVAILABLE', 409);
      }
      const prefix = 'snapshots/' + space + '/';
      const before = await env.BACKUPS.list({prefix, limit: 1000});
      if (before.truncated) return error('BACKUP_LIST_INCOMPLETE', 503);
      if (expired()) return error('CAPABILITY_EXPIRED', 403);
      const response = await stub.fetch('https://restore.invalid/__internal/daily', {method: 'POST', headers});
      await response.body?.cancel();
      if (response.status !== 200) return error('BACKUP_FAILED', 503);
      const after = await env.BACKUPS.list({prefix, limit: 1000});
      if (after.truncated) return error('BACKUP_LIST_INCOMPLETE', 503);
      const old = new Set(before.objects.map(item => item.key));
      const added = after.objects.filter(item => !old.has(item.key));
      if (added.length !== 1) return error('BACKUP_UNIDENTIFIED', 409);
      const snapshot = await env.BACKUPS.get(added[0].key);
      if (!snapshot || snapshot.size <= 0 || snapshot.size > LIMIT) return error('BACKUP_UNVERIFIED', 503);
      const readback = new Uint8Array(await snapshot.arrayBuffer());
      if (readback.length !== snapshot.size) return error('BACKUP_UNVERIFIED', 503);
      const actual = JSON.parse(new TextDecoder('utf-8', {fatal: true}).decode(readback));
      try {reconcile(source, actual, space);} catch {return error('RECONCILIATION_FAILED', 409);}
      const sections = {};
      for (const [name, value] of Object.entries(actual.app)) sections[name] = {
        sha256: await sha(new TextEncoder().encode(canonical(value))),
        count: value && typeof value === 'object' ? Object.keys(value).length : null};
      return reply({format: 'scry-revival-restore-proof-v1', canonical_application_equal: true,
        photos_full_bytes_verified: actual.photos.length, source_sha256: env.SOURCE_SHA256,
        snapshot_key: added[0].key, snapshot_bytes: readback.length, snapshot_sha256: await sha(readback), sections});
    } catch {
      // No raw storage/runtime/parser error, learner value, body or identifier.
      return error('TRANSFER_FAILED', 503);
    }
  },
};
