// Rehearsal-only entrypoint. The separately verified index.js/WASM are unchanged.
// This wrapper never exposes native web routes, scheduled handlers or alarms.
import {DurableObject} from 'cloudflare:workers';
import {LearningSpace as NativeLearningSpace} from './index.js';
const LIMIT = 1024 * 1024, WINDOW = 15 * 60_000, DATABASE_LIMIT = 32 * 1024 * 1024;
const reply = (value, status = 200) => Response.json(value, {status, headers: {'Cache-Control': 'private, no-store'}});
const refuse = () => reply({error: 'RECOVERY_REFUSED'}, 403);
export class LearningSpace extends DurableObject {
  constructor(ctx, env) {super(ctx, env); this.ctx = ctx; this.env = env; this.native = new NativeLearningSpace(ctx, env);}
  allowed(request) {
    const env = this.env, issued = Number(env.TRANSFER_ISSUED_MS), until = Number(env.TRANSFER_UNTIL_MS), now = Date.now();
    return env.SCRY_ENV === 'restore' && !env.OPENROUTER_API_KEY && !env.JEV_API_KEY &&
      typeof env.INTERNAL_KEY === 'string' && env.INTERNAL_KEY.length >= 32 &&
      request.headers.get('x-scry-internal-key') === env.INTERNAL_KEY &&
      Number.isSafeInteger(issued) && Number.isSafeInteger(until) && issued > 0 &&
      until > issued && until - issued <= WINDOW && issued <= now && now < until;
  }
  size() {
    const rows = this.ctx.storage.sql.exec('PRAGMA page_size').toArray();
    const page = rows[0]?.page_size, bytes = this.ctx.storage.sql.databaseSize;
    if (rows.length !== 1 || ![512, 1024, 2048, 4096].includes(page) ||
        !Number.isSafeInteger(bytes) || bytes < 0 || bytes > DATABASE_LIMIT) throw Error('STORAGE_BOUND');
    return {format: 'scry-recovery-storage-v1', page_size: page, database_bytes: bytes};
  }
  async fetch(request) {
    if (!this.allowed(request)) return refuse();
    const url = new URL(request.url);
    if (url.search) return refuse();
    try {
      const size = this.size();
      if (url.pathname === '/__recovery/storage' && request.method === 'GET') return reply(size);
      if (request.method !== 'POST' || !['/operator/restore', '/__internal/daily'].includes(url.pathname)) return refuse();
      const restoring = url.pathname === '/operator/restore';
      let forwarded = request;
      if (restoring) {
        // Do not rely on a caller's Content-Length, even on this private binding.
        const reader = request.body?.getReader(), chunks = []; let length = 0;
        if (!reader) return refuse();
        for (;;) {
          const next = await reader.read(); if (next.done) break;
          length += next.value.length;
          if (length > LIMIT) {await reader.cancel(); return refuse();}
          chunks.push(next.value);
        }
        const body = new Uint8Array(length); let offset = 0;
        for (const chunk of chunks) {body.set(chunk, offset); offset += chunk.length;}
        const source = JSON.parse(new TextDecoder('utf-8', {fatal: true}).decode(body));
        if (!Array.isArray(source.photos) || source.photos.length > 32) return refuse();
        let count = 0;
        const visit = value => {
          if (!value || typeof value !== 'object') return;
          const keys = Object.keys(value); count += keys.length;
          if (count > 1000 || keys.some(key => new TextEncoder().encode(key).length > 256)) throw Error('STORAGE_BOUND');
          for (const item of Object.values(value)) visit(item);
        };
        visit(source.app);
        forwarded = new Request(request.url, {method: 'POST', headers: request.headers, body});
      } else if (request.body) {
        const reader = request.body.getReader(), first = await reader.read(); await reader.cancel();
        if (!first.done) return refuse();
      }
      if (!this.allowed(request)) return refuse();
      const admitted = await this.ctx.storage.transaction(async storage => {
        const key = restoring ? 'restore-admitted' : 'backup-admitted';
        if (await storage.get(key) || (!restoring && !await storage.get('restore-completed'))) return false;
        if (restoring && size.database_bytes > 65536) return false;
        await storage.put(key, true); return true;
      });
      if (!admitted) return reply({error: 'RECOVERY_ALREADY_SPENT'}, 409);
      const response = await this.native.fetch(forwarded);
      await response.body?.cancel();
      this.size();
      if (response.status !== 200) return reply({error: 'NATIVE_REFUSED'}, 503);
      if (restoring) await this.ctx.storage.put('restore-completed', true);
      return reply({ok: true});
    } catch {return reply({error: 'RECOVERY_FAILED'}, 503);}
  }
  // Deliberately no native alarm: restore mode also prevents native arm().
}
export default {fetch() {return reply({error: 'NOT_FOUND'}, 404);}};
