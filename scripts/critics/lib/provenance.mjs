/**
 * scripts/critics/lib/provenance.mjs
 *
 * Determine the revision a Go binary was built from, from evidence the binary
 * itself carries. `candidate up --binary` uses this so a supplied binary is
 * never recorded under the walking checkout's revision unless the binary's
 * own provenance says so.
 *
 * Sources, in order:
 *  1. buildinfo `vcs.revision` (`go version -m`) — plain `go build` shapes.
 *  2. buildinfo `-ldflags` carrying `-X main.revision=<rev>` — stamped builds
 *     whose ldflags are visible (not built with `-trimpath`).
 *  3. the binary's own `version` output (`scry <rev>`) — gate exports built
 *     with `-trimpath`, where the stamp is not visible in buildinfo.
 *
 * A revision is only accepted as 40/64-hex (case-insensitive). Anything else
 * (for example the `development` default) is undeterminable, never a guess.
 */

import { spawnSync } from 'node:child_process';

const REVISION = /^([0-9a-f]{40}|[0-9a-f]{64})$/i;

export function readBinaryRevision(binaryPath, { env = process.env, timeoutMs = 10000 } = {}) {
  const buildinfo = spawnSync('go', ['version', '-m', binaryPath], { env, encoding: 'utf8', timeout: timeoutMs });
  if (buildinfo.status === 0 && typeof buildinfo.stdout === 'string') {
    const vcs = buildinfo.stdout.match(/^\s*build\s+vcs\.revision=(\S+)\s*$/m);
    if (vcs && REVISION.test(vcs[1])) return { revision: vcs[1].toLowerCase(), source: 'buildinfo-vcs' };
    const ldflags = buildinfo.stdout.match(/^\s*build\s+-ldflags="(.*)"\s*$/m);
    if (ldflags) {
      const stamped = ldflags[1].match(/-X\s+main\.revision=(\S+)/);
      if (stamped && REVISION.test(stamped[1])) return { revision: stamped[1].toLowerCase(), source: 'buildinfo-ldflags' };
    }
  }

  const version = spawnSync(binaryPath, ['version'], { env, encoding: 'utf8', timeout: timeoutMs });
  if (version.status === 0 && typeof version.stdout === 'string') {
    const token = version.stdout.trim().split(/\s+/).pop() ?? '';
    if (REVISION.test(token)) return { revision: token.toLowerCase(), source: 'version-output' };
  }

  return { revision: null, source: null };
}
