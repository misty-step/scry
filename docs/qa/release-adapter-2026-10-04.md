# Scry release adapter evidence, 2026-10-04

Lane 3 built the release adapter on `cursor/scry-release-v0` from Scry's default
`master` branch. This receipt covers adapter code and synthetic HTTP fixtures.
It does not claim PR #221 was deployed, its grading story passed QA, or Phaedrus
used it on a phone. The binding input was
`/workspace/navi-sdlc-verdict-2026-10-04.md`.

The [adapter](../../deploy/cloudflare-hosting/release.mjs) and
[tests](../../deploy/cloudflare-hosting/release.test.mjs) use Node's standard
library. [release.md](../../deploy/cloudflare-hosting/release.md) describes the
credential keys, immutable Worker/image manifest, caller-held release slot,
packet outcome, and production exclusion.

## Checks

Run these commands from the repository root:

```sh
node --test deploy/cloudflare-hosting/release.test.mjs
node --test deploy/cloudflare-hosting/*.test.mjs
git diff --check
```

The adapter suite passed 28 tests with no failures or skips. The hosting suite
passed 82 tests with no failures or skips on Node v24.21.0. No Rust build,
application rebuild, Cloudflare deployment, or model call ran. The full Scry
source-snapshot/Dagger gate has not run in this lane and remains required.

| Case | Observed result |
| --- | --- |
| Healthy baseline and release | `PASS`; checks at simulated 0, 150000, and 300000 ms; one candidate promotion |
| Two accessible failures | One previous Worker promotion and one previous-image rollout; `ROLLED_BACK` only after recovery passes |
| Newer deployment during confirmation | `SUPERSEDED`; zero rollback attempts |
| Newer deployment at final rollback guard | `SUPERSEDED`; zero rollback attempts |
| Inaccessible observation followed by passes | `UNVERIFIED`; zero rollback attempts |
| Inaccessible confirmation | `UNVERIFIED`; zero rollback attempts |
| Inaccessible baseline | `UNVERIFIED`; no deployment requests |
| Incomplete image rollout | `UNVERIFIED` even with healthy probes and Map |
| Unknown rollback response | `UNVERIFIED`; exactly one rollback POST attempt; no retry |
| Failed or inaccessible recovery | `FAIL` or `UNVERIFIED`; no second rollback |
| Access redirect, 401, 403, 429, timeout, or HTTP 200 login page | `UNVERIFIED`; never green |
| Missing private credential file | CLI exits 1 with JSON `UNVERIFIED`; inherited account-wide token ignored |
| Existing receipt or world-readable credentials | CLI exits 1 before effects; existing evidence remains byte-identical |
| Checkpoint write failure | `UNVERIFIED`; no Cloudflare requests |

Each failure test runs the release function against mocked Cloudflare HTTP
envelopes, including the current deployment list, Worker deployment creation,
container configuration update, and rollout status. Assertions inspect the
resulting packet and the actual serialized writes. The schedule uses a fake
clock so it proves timing logic without making a live five-minute claim.

## Remaining live proof

Maren's owner-only `~/.config/scry/release.env` appeared during this lane's work.
Only its mode and key names were inspected. It contains the release/account
aliases and probe key, but no staging owner Access JWT. The parser accepts
those file aliases without reading ambient Cloudflare credentials.
The release authority must add a staging owner Access JWT and ensure the file's
token has staging release scope. No token was printed or used. A service token
alone cannot satisfy Scry's immutable owner-subject guard. The release authority
must supply the exact approved Worker version/image manifest, previous baseline,
compatibility evidence, and controller-held shared release slot.

The Cloudflare APIs do not make a Worker promotion and image rollout atomic.
The receipt preserves both identities and leaves uncertain effects unverified.
The final current-deployment read is safe against an already-present newer
deployment; the controller's shared slot must exclude simultaneous writers
between that read and the deployment POST.

Production image replacement still needs executable quiescence and verified
final-backup stages from the runbook. The adapter refuses production tonight.
Private Map availability is a read-only release journey, not grading acceptance.
The QA lane must independently prove US-008/S2 on PR #221's exact artifact.
