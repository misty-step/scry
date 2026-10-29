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
