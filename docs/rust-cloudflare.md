# Build and deploy Scry

Scry is one Rust/WASM Cloudflare Worker with a SQLite Durable Object and private
R2 storage. [SPEC](../SPEC.md) owns application behavior; [ADR 004](adr/004-rust-cloudflare-candidate.md)
owns architecture. [QA](qa/system.md) describes application checks.

The owner's October 29, 2026 direction is to keep only the machinery needed to
build, test and deploy. The custom release/recovery controller, temporary
recovery environments, approval windows, consumed-attempt markers and custody
fingerprint gates are retired. They are not prerequisites for deployment.

## Build and isolated development

Use the pinned Rust, Node and package toolchains in the repository:

```sh
bun run ci
bun run dev
```

The development command uses disposable local SQLite/R2 and synthetic identity.
It does not load production secrets or call paid providers. The complete CI
command tests native Rust, WASM and actual local browser journeys, then exports
the tested Worker modules to `target/ci-release/`:

```sh
bun run ci:full -- --out target/ci-release --require-committed
```

A committed-source CI build is the deployment input. Do not rebuild the Worker
between the tests and deployment. Local fixtures prove application behavior,
not authenticated production use or independent disaster recovery.

## Reviewable release procedure

1. Make the change in a source PR, with the affected tests and independent review.
2. Merge after required CI passes.
3. The master workflow builds/tests once, deploys those exact Worker modules with
   the pinned Wrangler CLI, and reads back the deployed module hashes and source
   identity. The protected story-walk status reuses the walk inside CI.
4. Check the deployment result. A source merge alone is not a deployed release;
   a deployed module match is not authenticated learner acceptance.

The deployment uses `scripts/scry-deploy.py`, the build output in
`target/ci-release/`, and [deploy/rust-production.toml](../deploy/rust-production.toml).
The existing `CLOUDFLARE_API_TOKEN` Actions secret supplies deployment access.
No generated release PR, release tag, artifact-retention fallback, separate
recovery approval, remote restore drill or new credential is required.

## Existing production infrastructure

Keep the existing Worker, Durable Object namespace, R2 buckets, custom domain,
Access protection and secrets. Deploy code into that target; do not create a
parallel application or copy learner data into a temporary environment.
Production configuration is not a source of local test credentials. Never log
secret values, learner bodies or private archive contents.

This simplification does not delete stored content, reset learning history,
rotate credentials, change Access policies or reactivate historical stores.
Application persistence, archive validation and authentication tests remain.
If a future change intentionally migrates or deletes stored data, describe and
authorize that change separately rather than hiding it inside deployment.

## Verification and troubleshooting

Use the Actions log and deployment readback to distinguish a failed build, a
failed upload, and a deployed build. Investigate the actual failing step rather
than opening another release PR or repeating a paid recovery exercise.

After successful deployment, exercise the authorized isolated QA learner's
review journey through the existing sign-in path. Do not use the owner's
notebook or create credentials as a shortcut. The [verification skill](../.agents/skills/verify-scry/SKILL.md)
describes the QA identity boundary. Report any unavailable live acceptance
honestly; do not turn a missing observation into a claim of success.

Unexpected failures reach the Owner's triage route as a fixed category and the
deployment identity only; an unhealthy `/healthz` that names a stale backup,
and ordinary denials, send nothing. Access signing keys the provider or the
network cannot supply are reported as `access_keys_unavailable`, and a backup
requested from Settings that fails as `backup_failure`
([custody, 2026-11-15](runbook.md#custody--source-as-of-2026-11-15)). The
verification space takes no backups.

Storage capacity keeps a 4 MiB reserve below the 16 MiB complete-archive limit
(`persistence::RESERVE_BYTES`). Ordinary growth may not take a notebook at or
under 12 MiB over that line; withdrawing, settling and Settings backup requests
may cross it, and a notebook already over 12 MiB keeps the earlier protection,
admitted up to 16 MiB. The reserve covers every write that never asks the rule:
seven paid outcomes at most (3.50 USD rolling allowance over 0.50 USD
reservations), each up to 256 KiB of raw response that JSON may escape to twice
its length plus 8 KiB of Verdict, effects and ledger, 7 x 520 KiB = 3,640 KiB;
a thousand open Checks refused or made unknown by a claim, recovery or restore
at 150 bytes each, 147 KiB; a restore marker and a backup's status and failure
text, under 1 KiB. Total 3,788 KiB, inside 4,096 KiB. A raw response made of
control characters, each escaped to six bytes, is not covered. Only production holds model authority: a
preview reaches Jev only at a `.invalid` test host. The
[runbook](runbook.md#failure-reports-and-model-authority--source-as-of-2026-11-15)
lists the categories and the bindings paid judgment needs.

Rollback is a deliberate deployment of compatible application code, never an
automatic restore over current data. No cloud restore drill is part of normal
shipping. Keep existing application backup/storage behavior; do not remove
stored archives as part of this operational cleanup.

## Historical procedures

The [previous operational manual at 74567a6](https://github.com/misty-step/scry/blob/74567a69911009ad3ad2bb324f6d7c7d026a6969/docs/rust-cloudflare.md)
retains the old recovery, daily-trigger, initial namespace and cutover procedures
and their dated observations. Those are historical records, not current
instructions to provision, run or revive that infrastructure.
[The runbook](runbook.md) retains dated serving and migration observations.
Historical source, tags, releases, learner content and receipts remain intact;
they do not establish the current production version or a new acceptance pass.

### Due-review policy compatibility (2026-11-02)

New exact-recall Attempts identify the pinned memory model plus `schedule=due-v1`.
Legacy Attempts retain their original algorithm and omitted policy fields; this
build reads both without rewriting history. Once a new-policy Attempt is stored,
a pre-policy binary is not a compatible writer: its algorithm validation must
refuse the new record, not drop metadata or recalculate old evidence.

Archives containing new-policy Attempts, mixed-review mode or retained mixed
review receipts use `scry-rust-durable-sqlite-study-scheduled-v2`; old readers
refuse them before restore writes. Truly legacy archives retain their format.
Use a compatible code fix for rollback, never an old snapshot over acknowledged
writes. The archive fence also protects mixed-only session state; it does not
claim old code can preserve that state when pointed directly at live rows.


Studies presented as study or Quiz, explicit presentation phases and transition
receipts use `scry-rust-durable-sqlite-study-phase-v3`. Every new Prompt carries
a phase, whichever client asked for it, and marks its Study, so the format
survives an empty held prompt. A Study from an older record keeps its earlier
format until its next Prompt is held. New archives must be refused by the published predecessor
before restore writes. As above, this is an archive fence, not a claim that old
code can preserve unknown fields when pointed directly at live rows. After
source/quiz writes, rollback requires a compatible fix, never a direct downgrade.
