// Temporary CF-only conversion into private R2; never returns learner bodies.
import {handle} from './inspect.mjs';
import {project, diagnose, ProjectionRefusal} from './project.mjs';
const encoder = new TextEncoder();
const hash = async bytes => Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)),
  b => b.toString(16).padStart(2, '0')).join('');
const require = condition => {if (!condition) throw new Error('CONVERSION_FAILED');};
function table(db, sqlite, name) {
  const statement = db.prepare('SELECT * FROM "' + name.replaceAll('"', '""') + '"');
  const records = [];
  try {
    const names = statement.getColumnNames();
    while (statement.step()) {
      const record = {};
      for (let i = 0; i < names.length; i++) {
        const kind = sqlite.capi.sqlite3_column_type(statement.pointer, i);
        let value;
        if (kind === sqlite.capi.SQLITE_NULL) value = null;
        else if (kind === sqlite.capi.SQLITE_INTEGER) {
          const integer = sqlite.capi.sqlite3_column_int64(statement.pointer, i);
          value = Number(integer);
          require(Number.isSafeInteger(value) && BigInt(value) === integer);
        } else if (kind === sqlite.capi.SQLITE_FLOAT) {
          value = sqlite.capi.sqlite3_column_double(statement.pointer, i);
          require(Number.isFinite(value));
        } else {
          const pointer = sqlite.capi.sqlite3_column_blob(statement.pointer, i);
          const size = sqlite.capi.sqlite3_column_bytes(statement.pointer, i);
          const bytes = sqlite.wasm.heap8u().slice(pointer, pointer + size);
          value = kind === sqlite.capi.SQLITE_TEXT ? new TextDecoder('utf-8', {fatal: true}).decode(bytes) : bytes;
        }
        record[names[i]] = value;
      }
      records.push(record);
    }
  } finally {statement.finalize();}
  return records;
}
export default {async fetch(request, env) {
  const url = new URL(request.url);
  const dryRun = url.pathname === '/dry-run';
  if ((!dryRun && url.pathname !== '/convert') || url.search) return Response.json({error: 'NOT_FOUND'}, {status: 404});
  url.pathname = '/inspect';
  // Reuse every reviewed archive/auth/pin/expiry check. Keys are separate from
  // read-only inventory authority; no caller controls a bucket, key, SQL or time.
  const response = await handle(new Request(url, request), {...env, INSPECT_KEY: env.CONVERT_KEY,
    INSPECT_UNTIL_MS: env.CONVERT_UNTIL_MS}, async ({db, sqlite, inventory, manifest, bytes}) => {
    if (!dryRun) require(env.DESTINATION && env.SOURCE !== env.DESTINATION);
    const tables = {};
    for (const name of ['sources', 'goals', 'jobs', 'concepts', 'notes', 'concept_relations', 'evidence',
      'assistance_exposures', 'quizzes', 'quiz_versions', 'schedules', 'concept_quizzes', 'presentations',
      'review_events', 'grade_overrides', 'review_session', 'operations', 'job_attempts', 'content_assessments', 'preferences',
      'capture_images', 'corrections', 'semantic_assessments', 'source_documents']) tables[name] = table(db, sqlite, name);
    for (const image of tables.capture_images) image.sha256 = await hash(image.bytes);
    // No destination binding or storage access in this branch. Never return the
    // partial projection, original inventory, pins, hashes or private values.
    if (dryRun) return {refusals: diagnose(tables, bytes, env.SOURCE_SHA256, manifest.created_at)};
    const {archive, counts} = project(tables, bytes, env.SOURCE_SHA256, manifest.created_at);
    const encoded = encoder.encode(JSON.stringify(archive)), checksum = await hash(encoded);
    const key = 'converted-go/' + env.SOURCE_SHA256 + '/' + checksum + '.json';
    // Conditional create and complete readback: no overwrite/reset on replay,
    // including a corrupt pre-existing destination or concurrent invocation.
    if (!await env.DESTINATION.head(key)) await env.DESTINATION.put(key, encoded,
      {onlyIf: {etagDoesNotMatch: '*'}, sha256: checksum,
        httpMetadata: {contentType: 'application/json'}});
    const stored = await env.DESTINATION.get(key);
    require(stored && stored.size === encoded.length);
    const readback = new Uint8Array(await stored.arrayBuffer());
    require(readback.length === encoded.length && readback.every((byte, i) => byte === encoded[i]));
    const sections = {};
    for (const name of ['goals', 'concepts', 'questions', 'events', 'overrides', 'operations', 'jobs', 'spend', 'revival_go', 'revival_schedule_origins'])
      sections[name] = await hash(encoder.encode(JSON.stringify(archive.app[name])));
    return {format: 'scry-go-projection-v1', verified_remote_bytes: true, rust_restore_verified: false,
      destination_key: key, destination_bytes: encoded.length, destination_sha256: checksum,
      counts, projection_sha256: sections, source_inventory: inventory,
      limits: 'Prepared archive only; exact Rust restore, learner reconciliation, final writer consistency and cutover remain separate.'};
  }, error => error instanceof ProjectionRefusal ?
    {error: 'UNSUPPORTED_GO_PROJECTION', reason: error.message} : undefined);
  if (!dryRun || response.status !== 200) return response;
  return Response.json((await response.json()).refusals, {headers: response.headers});
}};
