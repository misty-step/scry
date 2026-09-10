# Scry

Scry is a personal, quiz-first learning app. `VISION.md` owns product direction;
`SPEC.md` owns behavior, acceptance, and architecture. The application is one Go
process, SQLite, server-rendered HTML, HTMX, and a small browser controller on
exe.dev. The operator approved the phone flow and daily recovery policy.

## Product and runtime

- One private application instance owns learning writes and background work.
  exe.dev provides HTTPS and identity; Scry checks the exact owner UserID,
  canonical Host, and explicitly trusted ingress peer. No public signup,
  waitlist, billing, or five-face compatibility is implied.
- `docs/runbook.md` owns current origins, activation, recovery, and retirement
  evidence. Never infer a deployment from source changes or a green gate.
- The old production/staging Rust Workers are paused and their cron triggers
  are removed. Worker SQLite/R2, exact historical source/artifacts, and frozen
  native Postgres/backups remain recovery material. Do not reactivate, import,
  rename, or delete these stores without explicit scope and migration proof.
  Native backup schedules are retained recovery, not an old application writer.

## Architecture and boundaries

- `cmd/scry` composes `serve`, `check`, `backup`, `restore`, `export`, and the
  explicitly synthetic `seed-fixture` command. It is an operator CLI, not the
  retired public API/CLI/MCP product contract.
- `internal/learning` is pure grading and the pinned Go FSRS adapter. No
  persistence, HTTP, authentication, UI, logging, or model-client dependency.
- `internal/store` owns SQLite transactions, immutable content/history,
  occurrence/session identity, schedules, idempotency, leases, and spend records.
- `internal/generation` makes bounded model requests outside transactions and
  rechecks ownership/source revision before publishing. Never call a model in
  the answer-grading path or hide unknown paid outcomes.
- `internal/web` owns authentication, CSRF, routes, templates, and embedded
  assets. Browser JS is presentation/reconciliation, not durable state or an
  offline mutation queue. Alternate hosts redirect reads only; never replay a
  mutation across origins or broaden identity trust for convenience.
- `internal/recovery` produces complete SQLite archives and requires exact
  remote readback for off-VM success. `deploy/backup-gateway` is the current
  append/read-only Cloudflare Worker over private R2, not the old application.
- `.dagger/src/index.ts` is the only TypeScript surface. Browser JS and the
  small recovery gateway are deliberate plain-JS exceptions. No frontend build
  step, general hosting framework, or new runtime dependency without need.

## Runtime contracts

- Preserve one atomic answer/event/schedule transition and durable exact retry.
  Assistance cannot become unaided success; unsupported grading remains honest
  uncertainty. Content edits/disputes do not rewrite historical presentations.
- Preserve the full scheduler state and `internal/learning.Algorithm` identity.
  No Rust parity, personalized retention, or learning-gain claim is inherited.
- Review feedback remains until deliberate Next. Offline work pauses; unknown
  responses reconcile by the same operation ID rather than inventing success.
- Keep new-app daily and pre-release off-VM snapshots with 30-day remote
  retention. Approved targets are RPO 24 hours / RTO 60 minutes, not guarantees.
  Retain compatible binaries and private configuration independently of the VM.
- Restore into an unused path. Uncertain jobs stay paused; never clone live
  integrations or scheduler ownership into a development/recovery instance.
- `.env` is ignored private operator material, not a deployment input to copy
  wholesale. Production `/etc/scry/scry.env` is root-owned mode 0600 and is
  parsed by systemd, never sourced as shell. Do not expose credentials.

## Gates and proof

- `bun run ci` / `bun run ci:local` run the host source-snapshot gate.
  `bun run ci:full` runs the same gate in pinned Dagger tooling and exports the
  exact smoke-tested Linux amd64 binary, source inventory/archive, and proof.
  `scripts/scry-ci` owns this contract; no deployment credentials enter it.
- Use `--require-committed` for release artifacts. Worktree-labeled output is
  development evidence, not production source provenance. Stage only tested
  bytes; never rebuild between proof and activation.
- `deploy/install.sh` stages immutable releases. `deploy/activate.sh` drains,
  verifies remote backup and schema compatibility, switches the release, and
  verifies the actual process/readiness. Never bypass these guards or overwrite
  the live database during rollback.
- Exercise the changed real surface. Existing Go tests protect behavior; model
  quality, private ingress, physical-phone acceptance, and independent restore
  need their own evidence. Reuse valid receipts; unverified is not passed.
- Historical recovery tools under `bin/`, `scripts/scry-cloudflare-data`, and
  `scripts/lib/scry_ops.py` retain their old formats and names intentionally.
  Their checks remain separate from the Go application contract.

## Sources of truth and work

Use the operator-approved specification for acceptance and current code/tests
for implemented behavior. `docs/qa/system.md` owns verification entrypoints.
Dated Rust/beta/dogfood/research receipts, `docs/rust-migration.md`, `SLICE-*.md`,
and `exemplars.md` are history, not current deployment instructions.

Work from the current request, preserve overlapping work, and keep ownership
and evidence in the session/PR. Linear owns operational state, not a duplicate
product specification. Close a named issue only after its accepted scope and
required proof are satisfied. Do not lower gates or claim unrun canaries.
