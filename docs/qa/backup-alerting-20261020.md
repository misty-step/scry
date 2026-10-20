# Daily backup cron alerting — 2026-10-20

Scope: [#309](https://github.com/misty-step/scry/issues/309), part of
[#276](https://github.com/misty-step/scry/issues/276). Protects SPEC D5/S10.2;
SPEC's private-runtime reporting privacy boundary informs the metadata allowlist.
The existing `features/README.md` navigation map was read. This operations-only
change has no learner UI journey; no phone/provider/recovery acceptance is claimed.

Base: `59e7ad1f82ffdbead6e533be6328e212f34d9895` (`origin/master`, fetched before
creating `fix/backup-alerting`). The implementation commit and reviewed head are
recorded in the PR. Script SHA-256 exercised by the single remote drill:
`62b8e2a404c1bd910e779d9be52ed2d631dc0e1ada2f2768d26ae44e5bc40c0e`.

## Offline before/after

New tests were added before implementation, then run with
`python3 qa/daily-backup.test.py`:

```text
Before: Ran 21 tests; FAILED (failures=10, errors=3)
  success lifecycle: expected two check-ins, got zero
  sink failure: expected nonzero monitoring result, got zero
  envelope tests: send_checkin did not exist
After:  Ran 21 tests; OK
```

The existing backup transport is stubbed and the alert sender is stubbed at the
orchestration seam. Envelope tests inspect real serialized requests against a
synthetic DSN and a stub HTTP transport. Assertions cover the shared execution
ID, verified-only `ok`, failures before credential/network access, HTTP 421,
no backup credentials in telemetry, one-failure threshold, 09:30 UTC schedule,
15-minute margin, three-minute timeout, two-second sends, no retries, rejected
sinks, and successful backups despite sink failure with a nonzero service exit.

Additional observed checks:

- `python3 qa/promote.test.py`: 7 tests pass.
- `python3 qa/release-publish.test.py`: 5 tests pass.
- `cargo test --locked --all-targets`: 102 tests across 15 binaries pass.
- `cargo clippy --locked --all-targets -- -D warnings`: pass.
- `systemd-analyze --user verify deploy/mirrodin/scry-daily-backup.service deploy/mirrodin/scry-daily-backup.timer`: pass (read-only, no installation).
- `systemd-analyze calendar '*-*-* 09:30:00 UTC'`: next run
  `2026-10-21 09:30:00 UTC`.
- Redacted Gitleaks over the source snapshot: no leaks. `git diff --check`: pass.

The first `bun run ci` attempt stopped at its pinned-runtime gate: Node 26.12.0
instead of 22.22.0. A pinned-runtime rerun is recorded in the PR. No gate was
weakened. These are development checks, not release or deployment proof.

## One real controlled failure

Only one run was sent to Sentry. A scratch copy retained the tested script and
existing public Worker ingest configuration. Its production TOML was replaced
with `CANONICAL_ORIGIN = "https://example.invalid"`; pass lookups were stubbed
with synthetic strings. No production credentials were read for this drill,
no production backup route was called, and no live app/store/Cloudflare change
was made. Actual urllib transport attempted the invalid origin and reported a
fixed network failure. Sentry ingestion used the existing Scry public DSN,
without printing it or creating/rotating a credential.

```text
2026-10-20T14:38:45.779143Z in_progress -> HTTP 200
Scry backup failed: network request failed.
2026-10-20T14:38:45.878732Z error       -> HTTP 200
exit 1
check_in_id: 33fbba132b7c4abfb475accc2d7f51f6
```

Readback with the existing Sentry token (GET only) confirmed monitor
`027ba15e-0cee-4653-b8d4-c4c8634492d4`, slug `scry-mirrodin-daily-backup`,
project `scry`, active and unmuted, production environment in `error`, with the
same check-in ID and an active incident. Configuration returned:

```json
{"schedule":"30 9 * * *","timezone":"UTC","max_runtime":3,"schedule_type":"crontab","checkin_margin":15,"recovery_threshold":1,"failure_issue_threshold":1}
```

Sentry returned `nextCheckIn = 2026-10-21T09:30:00Z` and
`nextCheckInLatest = 2026-10-21T09:45:00Z`. This establishes a remotely registered
missed-run deadline, not an observed missed-run notification. The next deadline
is in the future; no second shortened-schedule test or real check-in was sent.

## Owner routing and remaining handoff

Read-only discovery found enabled workflow `2777562`, named
`scry: production issues and monitor failures to #ops`, linked to Scry's Issue
Stream detector `6109865`, using existing approved intake. The new cron detector
is `10702175`. Readback found its latest issue:
[Cron failure: scry-mirrodin-daily-backup](https://misty-step.sentry.io/issues/7802576540/),
`SCRY-W`, type `monitor_check_in_failure`, category `outage`, first seen
`2026-10-20T14:38:49Z`, unresolved with reason `An error check-in was detected.`
The Issue Stream workflow covers the new cron issue without a direct workflow
association on the cron detector; no remote routing configuration was changed.

**PASS — forced failure reached the owner's existing channel.** Read-only
Discord lookup found the matching Sentry issue link in
[#ops message 1562113111710896239](https://discord.com/channels/1468712458536354050/1560044721181696042/1562113111710896239),
delivered at `2026-10-20T14:39:48.490Z`. The channel is `ops`, ID
`1560044721181696042`. The archived Hermes job's `failure_deliver` points to
`discord:1560049697781981205`, currently named `incidents`; initial reads of
that historical channel did not establish current Sentry delivery. Likewise,
the initial `issue.category:cron` query did not find this `outage` issue. The
final detector and correct channel readbacks resolve those earlier limits.
No Hermes job was changed and no Discord message was sent directly.

No new credential or Sentry admin provisioning is needed. The existing public
DSN successfully upserted the native monitor and the existing workflow delivered
the notification. Privileged Sentry and Discord tokens were used only for GET
readbacks through `pass-env`; no token/DSN is retained in this evidence. Do not
retire Hermes based only on HTTP 200, source merge, or offline tests.

The conductor must install the merged units from master, verify the backup and
check-in, complete owner-delivery and missed-run observations, then disable
Hermes `37ca5bbdd570` before #301's promotion. Installed-timer execution, a
natural daily run, missed-run channel delivery, and independent R2 readback
remain separate observations. This builder did not install/start units, disable
Hermes, deploy, merge, or read `.env`/`.dev.vars`.

Protocol references: [Sentry check-in wire format and DSN upsert](https://develop.sentry.dev/sdk/telemetry/check-ins/),
[code-configured cron monitors](https://docs.sentry.io/platforms/python/crons/).

Independent code review of implementation `5cd4d4e84b535c96daf544a091b29a5ecc762529`
against the recorded base: Standards 0 findings; Spec 0 concrete defects or
scope creep. The Spec review initially recorded an owner-delivery gap, then
cleared it after receiving the actual issue and #ops message readback above.
Both reviewers reran the 21 offline backup tests successfully. The subsequent
receipt update changes documentation only.
