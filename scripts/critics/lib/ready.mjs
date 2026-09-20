/**
 * scripts/critics/lib/ready.mjs
 *
 * One bounded readiness attempt against `<url>/readyz`.
 *
 * `candidate up` must advance its attempt loop even when a server accepts the
 * connection but never answers: without a per-request timeout the attempt
 * stays pending forever and the attempt bound can never be reached. Each
 * attempt is therefore destroyed after `timeoutMs`; the caller sleeps and
 * tries again.
 */

import { request } from 'node:http';

export function readyAttempt(url, { timeoutMs = 2000 } = {}) {
  return new Promise((resolve, reject) => {
    const req = request(url + '/readyz', (res) => {
      let data = '';
      res.on('data', (chunk) => { data += chunk; });
      res.on('end', () => {
        res.statusCode === 200 && data.trim() === 'ready'
          ? resolve()
          : reject(new Error('not ready'));
      });
    });
    req.setTimeout(timeoutMs, () => req.destroy(new Error('ready request timed out')));
    req.on('error', reject);
    req.end();
  });
}
