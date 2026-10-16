// Temporary CF-only read-only inspection, not an importer or learner API.
import initSQLite from '@sqlite.org/sqlite-wasm';
import wasmModule from '@sqlite.org/sqlite-wasm/sqlite3.wasm';
import {unzipSync} from 'fflate/browser';
import referenceSQL from '../target/revival-schema.sql';

const silent = () => {};
globalThis.sqlite3ApiConfig = {debug: silent, warn: silent, error: silent, log: silent,
  disable: {vfs: {opfs: true, 'opfs-wl': true, 'opfs-sahpool': true}}};
const sqlite = initSQLite({print: silent, printErr: silent,
  instantiateWasm(imports, ready) {
    const instance = new WebAssembly.Instance(wasmModule, imports);
    ready(instance, wasmModule);
    return instance.exports;
  },
});
const LIMIT = 16 * 1024 * 1024;
const hex = bytes => Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
const hash = async bytes => hex(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)));
const encode = value => new TextEncoder().encode(JSON.stringify(value));
const reply = (value, status = 200) => Response.json(value, {status,
  headers: {'Cache-Control': 'private, no-store', 'X-Content-Type-Options': 'nosniff'}});
const schemaSQL = "SELECT type,name,tbl_name,sql FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name";
const rows = (db, sql) => db.exec({sql, rowMode: 'array', returnValue: 'resultRows'});
const require = condition => {if (!condition) throw new Error('INVALID_ARCHIVE');};

async function inspect(bytes, consume) {
  const seen = new Set();
  const entries = unzipSync(bytes, {filter(file) {
    require(['manifest.json', 'scry.sqlite'].includes(file.name) && !seen.has(file.name));
    require(file.compression === 0 && file.size === file.originalSize && file.size > 0 && file.size <= LIMIT);
    if (file.name === 'manifest.json') require(file.size <= 64 * 1024);
    seen.add(file.name);
    return true;
  }});
  require(seen.size === 2);
  const manifest = JSON.parse(new TextDecoder('utf-8', {fatal: true}).decode(entries['manifest.json']));
  const data = entries['scry.sqlite'];
  const databaseHash = await hash(data);
  require(manifest.format === 1 && manifest.integrity === 'ok' && manifest.bytes === data.length &&
    manifest.sha256 === databaseHash && Number.isSafeInteger(manifest.created_at) && manifest.created_at > 0 &&
    /^scry-\d{8}T\d{6}\.\d{9}Z-[a-f0-9]{32}$/.test(manifest.id) &&
    manifest.database?.schema === 5 && manifest.database?.application_id === 0x53435259);
  require(new TextDecoder().decode(data.subarray(0, 16)) === 'SQLite format 3\x00');
  const s = await sqlite;
  const reference = new s.oo1.DB(':memory:');
  const db = new s.oo1.DB(':memory:');
  let pointer;
  try {
    reference.exec(referenceSQL);
    const expected = rows(reference, schemaSQL);
    pointer = s.wasm.allocFromTypedArray(data);
    // Caller-owned fixed buffer: no allocator mixing, resizing or writes.
    require(s.capi.sqlite3_deserialize(db.pointer, 'main', pointer, BigInt(data.length),
      BigInt(data.length), s.capi.SQLITE_DESERIALIZE_READONLY) === s.capi.SQLITE_OK);
    db.exec('PRAGMA trusted_schema=OFF; PRAGMA query_only=ON');
    require(db.selectValue('PRAGMA application_id') === 0x53435259 && db.selectValue('PRAGMA user_version') === 5);
    require(JSON.stringify(rows(db, schemaSQL)) === JSON.stringify(expected));
    require(JSON.stringify(rows(db, 'PRAGMA integrity_check')) === '[["ok"]]');
    require(rows(db, 'PRAGMA foreign_key_check').length === 0);
    const tables = [];
    for (const [, name] of expected.filter(row => row[0] === 'table')) {
      // Names come from the exact retained reference schema, never the request.
      const statement = db.prepare('SELECT * FROM "' + name.replaceAll('"', '""') + '"');
      const digests = [];
      const columns = statement.columnCount;
      try {
        while (statement.step()) {
          const values = [];
          for (let index = 0; index < columns; index++) {
            const kind = s.capi.sqlite3_column_type(statement.pointer, index);
            if (kind === s.capi.SQLITE_NULL) values.push(['null']);
            else if (kind === s.capi.SQLITE_INTEGER) values.push(['integer', s.capi.sqlite3_column_int64(statement.pointer, index).toString()]);
            else if (kind === s.capi.SQLITE_FLOAT) {
              const buffer = new ArrayBuffer(8);
              new DataView(buffer).setFloat64(0, s.capi.sqlite3_column_double(statement.pointer, index));
              values.push(['real-ieee754-be', hex(new Uint8Array(buffer))]);
            } else {
              // Use SQLite's byte lengths, not C-string decoding: embedded NUL
              // and all UTF-8/blob bytes must contribute to reconciliation.
              const p = s.capi.sqlite3_column_blob(statement.pointer, index);
              const size = s.capi.sqlite3_column_bytes(statement.pointer, index);
              values.push([kind === s.capi.SQLITE_TEXT ? 'text-utf8' : 'blob', hex(s.wasm.heap8u().subarray(p, p + size))]);
            }
          }
          digests.push(await hash(encode(values)));
        }
      } finally {statement.finalize();}
      tables.push({name, rows: digests.length, columns, sha256: await hash(encode(digests.sort()))});
    }
    const inventory = {format: 'scry-go-inventory-v1', verified: true, database_sha256: databaseHash,
      schema: 5, application_id: 0x53435259, integrity: 'ok', foreign_keys: 'ok',
      sqlite: s.version.libVersion, schema_sha256: await hash(encode(expected)),
      canonical: 'sorted-row-sha256/type-tagged-sql-values-v1', tables};
    return consume ? await consume({db, sqlite: s, inventory, manifest, bytes}) : inventory;
  } finally {
    db.close();
    if (pointer) s.wasm.dealloc(pointer);
    reference.close();
  }
}

export async function handle(request, env, consume, failure) {
  const url = new URL(request.url);
  if (url.pathname !== '/inspect' || url.search) return reply({error: 'NOT_FOUND'}, 404);
  if (request.method !== 'POST') return reply({error: 'METHOD_NOT_ALLOWED'}, 405);
  if (typeof env.INSPECT_KEY !== 'string' || env.INSPECT_KEY.length < 32) return reply({error: 'UNCONFIGURED'}, 503);
  if (request.headers.get('Authorization') !== 'Bearer ' + env.INSPECT_KEY) return reply({error: 'UNAUTHORIZED'}, 403);
  const size = Number(env.SOURCE_BYTES), until = Number(env.INSPECT_UNTIL_MS);
  if (!/^[a-f0-9]{64}$/.test(env.SOURCE_SHA256 || '') ||
    typeof env.SOURCE_KEY !== 'string' || !new RegExp('^preserved-go/' + env.SOURCE_SHA256 + '/scry-[A-Za-z0-9.-]{1,180}\\.scry-backup\\.zip$').test(env.SOURCE_KEY) ||
    !Number.isSafeInteger(size) || size <= 0 || size > LIMIT ||
    !Number.isSafeInteger(until) || until <= 0) return reply({error: 'INVALID_SOURCE_PIN'}, 503);
  if (Date.now() >= until) return reply({error: 'CAPABILITY_EXPIRED'}, 403);
  try {
    if (request.body) {
      const reader = request.body.getReader();
      const first = await reader.read();
      await reader.cancel();
      if (!first.done) return reply({error: 'BODY_NOT_ALLOWED'}, 400);
    }
    if (Date.now() >= until) return reply({error: 'CAPABILITY_EXPIRED'}, 403);
    const object = await env.SOURCE.get(env.SOURCE_KEY);
    if (!object) return reply({error: 'SOURCE_MISSING'}, 404);
    if (object.size !== size) return reply({error: 'SOURCE_CHANGED'}, 409);
    const bytes = new Uint8Array(await object.arrayBuffer());
    if (bytes.length !== size || await hash(bytes) !== env.SOURCE_SHA256) return reply({error: 'SOURCE_CHANGED'}, 409);
    return reply({...await inspect(bytes, consume), archive_sha256: env.SOURCE_SHA256, archive_bytes: size});
  } catch (error) {
    // Never return/log SQLite messages, SQL, private manifests, rows or media.
    return reply(failure?.(error) || {error: 'INSPECTION_FAILED'}, 422);
  }
}
export default {fetch: (request, env) => handle(request, env)};
