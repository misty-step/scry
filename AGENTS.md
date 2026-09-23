# Scry

Scry is a personal, quiz-first learning app. `VISION.md` owns product direction;
`SPEC.md` owns behavior, acceptance, and architecture. The production
application is a Go/SQLite process with server-rendered HTML, HTMX, and a small
browser controller in a singleton Cloudflare Container behind Worker
`scry-app-host` and Cloudflare Access. The operator approved the phone flow and
daily recovery policy.

## Product and runtime

- One private application instance owns learning writes and background work.
  Cloudflare Access gates production ingress; `scry-app-host` validates the
  JWT issuer, audience, and exact owner subject before forwarding to the
  singleton Container. nginx strips client identity headers and injects the
  fixed Scry owner ID; Go checks canonical Host, trusted peer, and exact owner
  ID. No public signup, waitlist, billing, or five-face compatibility is implied.
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
- Keep daily and pre-release SQLite snapshots in private R2 with 30-day remote
  retention. Approved targets are RPO 24 hours / RTO 60 minutes, not guarantees.
  Retain compatible binaries and private configuration independently of the
  ephemeral Container.
- Restore into an unused path. Uncertain jobs stay paused; never clone live
  integrations or scheduler ownership into a development/recovery instance.
- `.env` is ignored private operator material, not a deployment input to copy
  wholesale. Production secrets are Cloudflare Worker/container bindings;
  never place them in source, flags, logs, or container images. Any retained
  VM's `/etc/scry/scry.env` is historical recovery-only, root-owned mode 0600,
  parsed by systemd, and never sourced as shell. Do not expose credentials.

## Gates and proof

- Start focused verification with `.agents/skills/scry-qa/SKILL.md`. It owns
  real-surface proof boundaries and run-owned cleanup; read it explicitly when
  the runner disables ambient skill discovery.
- Critic passes start at `scripts/critics/` with run instructions in
  `docs/qa/critics.md`. Their receipts bind revision, environment, story, and
  criterion; Jev output is advisory, and mutating passes stay on isolated
  synthetic data.
- `bun run ci` / `bun run ci:local` run the host source-snapshot gate.
  `bun run ci:full` runs the same gate in pinned Dagger tooling and exports the
  exact smoke-tested Linux amd64 binary, source inventory/archive, and proof.
  `scripts/scry-ci` owns this contract; no deployment credentials enter it.
- Use `--require-committed` for release artifacts. Worktree-labeled output is
  development evidence, not production source provenance. Stage only tested
  bytes; never rebuild between proof and activation.
- `deploy/install.sh`, `deploy/activate.sh`, and `deploy/restore.sh` support the
  retained exe.dev VM recovery path; they are not the current production
  deployment path. Cloudflare releases follow `docs/runbook.md` and
  `deploy/cloudflare-hosting/README.md`, with committed-source proof, the exact
  smoke-tested binary, remote-backup verification, and explicit activation
  approval. Never restart the stopped old writer or overwrite a live database.
- Exercise the changed real surface. Existing Go tests protect behavior; model
  quality, private ingress, physical-phone acceptance, and independent restore
  need their own evidence. Reuse valid receipts; unverified is not passed.
- Historical recovery tools under `bin/`, `scripts/scry-cloudflare-data`, and
  `scripts/lib/scry_ops.py` retain their old formats and names intentionally.
  Their checks remain separate from the Go application contract.

## Sources of truth and work

Use the contract chain: `VISION.md` owns intent, `USER_STORIES.md` owns root
stories, `SPEC.md` owns criteria, behavior, and architecture, and
`docs/qa/system.md` owns verification entrypoints. Current code and tests own
implemented behavior. Critic passes start at `scripts/critics/` with run
instructions in `docs/qa/critics.md`. Dated Rust/beta/dogfood/research receipts,
`docs/rust-migration.md`, `docs/history/SLICE-*.md`, and
`docs/history/exemplars.md` are history, not current deployment instructions.

Work from the current request, preserve overlapping work, and keep ownership
and evidence in the session/PR. Linear owns operational state, not a duplicate
product specification. Close a named issue only after its accepted scope and
required proof are satisfied. Do not lower gates or claim unrun canaries.
