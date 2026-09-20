/**
 * scripts/critics/lib/provenance.mjs
 *
 * Determine what a Go binary's own evidence says about how it was built:
 * the revision it was built from, and whether the tree it was built from
 * was modified. `candidate up --binary` uses this so a supplied binary is
 * never recorded under the walking checkout's revision or working-tree
 * state unless the binary's own provenance says so.
 *
 * Sources, in order:
 *  1. buildinfo `vcs.revision` and `vcs.modified` (`go version -m`) — plain
 *     `go build` shapes; the only source that also carries tree state.
 *  2. buildinfo `-ldflags` carrying `-X main.revision=<rev>` — stamped builds
 *     whose ldflags are visible (not built with `-trimpath`). No tree state.
 *  3. the binary's own `version` output (`scry <rev>`) — gate exports built
 *     with `-trimpath`, where the stamp is not visible in buildinfo. No tree
 *     state.
 *
 * A revision is only accepted as 40/64-hex (case-insensitive). Anything else
 * (for example the `development` default) is undeterminable, never a guess.
 * `modified` is true/false only when the evidence determines it
 * (`buildinfo-vcs`); stamp sources report null, and callers must record the
 * state as `unknown` rather than borrow another tree's state.
 */

import { spawnSync } from 'node:child_process';

const REVISION = /^([0-9a-f]{40}|[0-9a-f]{64})$/i;

export function readBinaryProvenance(binaryPath, { env = process.env, timeoutMs = 10000 } = {}) {
  const buildinfo = spawnSync('go', ['version', '-m', binaryPath], { env, encoding: 'utf8', timeout: timeoutMs });
  if (buildinfo.status === 0 && typeof buildinfo.stdout === 'string') {
    const vcs = buildinfo.stdout.match(/^\s*build\s+vcs\.revision=(\S+)\s*$/m);
    if (vcs && REVISION.test(vcs[1])) {
      const modified = buildinfo.stdout.match(/^\s*build\s+vcs\.modified=(true|false)\s*$/m);
      return {
        revision: vcs[1].toLowerCase(),
        source: 'buildinfo-vcs',
        modified: modified ? modified[1] === 'true' : null,
      };
    }
    const ldflags = buildinfo.stdout.match(/^\s*build\s+-ldflags="(.*)"\s*$/m);
    if (ldflags) {
      const stamped = ldflags[1].match(/-X\s+main\.revision=(\S+)/);
      if (stamped && REVISION.test(stamped[1])) {
        return { revision: stamped[1].toLowerCase(), source: 'buildinfo-ldflags', modified: null };
      }
    }
  }

  const version = spawnSync(binaryPath, ['version'], { env, encoding: 'utf8', timeout: timeoutMs });
  if (version.status === 0 && typeof version.stdout === 'string') {
    const token = version.stdout.trim().split(/\s+/).pop() ?? '';
    if (REVISION.test(token)) return { revision: token.toLowerCase(), source: 'version-output', modified: null };
  }

  return { revision: null, source: null, modified: null };
}

/**
 * Map binary provenance to the build-source state a candidate handle
 * records. `clean`/`dirty` only when the binary's own `vcs.modified`
 * evidence determines it; everything else is `unknown` — the walking
 * checkout's state is never substituted for the binary's.
 */
export function sourceStateFromProvenance(provenance) {
  if (provenance && provenance.source === 'buildinfo-vcs') {
    if (provenance.modified === true) return { state: 'dirty', source: 'buildinfo-vcs' };
    if (provenance.modified === false) return { state: 'clean', source: 'buildinfo-vcs' };
  }
  return { state: 'unknown', source: null };
}
