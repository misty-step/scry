# Scry

Scry is a private, prompt-first learning app. Read `VISION.md` →
`USER_STORIES.md` → `SPEC.md`, especially US-013/S11: useful tailored reference
and practice from learning intent before setup, then refinement through real
answers and feedback. `README.md` is the work entrypoint.

The operator authorized a full redesign and Rust-on-Cloudflare rewrite on
2026-10-07. Active replacement source is Rust/WASM Worker + one SQLite-backed
Durable Object + private R2, with server-rendered HTML and a small vanilla
controller. Source changes and green gates are not deployment proof. The
production Go/SQLite Container remains governed by `docs/runbook.md` until an
explicitly approved activation replaces it.

## Product, privacy, and historical stores

- One private instance owns learning writes and background effects. The new
  Worker validates the Access JWT signature, issuer, audience, expiration, and
  exact owner subject before forwarding to the singleton through an internal
  capability. A caller-supplied identity header is never authentication.
- `docs/runbook.md` owns current origins, activation, recovery, and retirement
  evidence. The old Go source is retained in Git at `c6ba395`; its operational
  runbook and Container artifacts do not become rewrite acceptance receipts.
- Start the rewrite in a fresh namespace. Old Rust Workers stay paused with
  crons removed; historical Worker SQLite/R2, frozen Postgres/backups, and the
  retained VM remain recovery material. Do not reactivate, import, rename, or
  delete them without explicit scope and migration proof. Native backups are
  recovery, not an old application writer.
- No public signup, billing, collaboration, CLI/MCP parity, or five-face
  compatibility. Create accepts private intent; pasted URLs do not authorize
  inferred browsing. Sources and model claims must retain exact provenance.

## Architecture and boundaries

- `src/learning.rs` is pure grading, concept evidence, and the Rust port of the
  pinned Go FSRS adapter. No persistence, HTTP, authentication, UI, logging, or
  model-client dependency. Preserve `learning::SCHEDULER` / `ALGORITHM` identity
  and the full scheduler state; 260 golden trajectories protect compatibility.
- `src/model.rs`, `src/engine.rs`, and `src/persistence.rs` own typed records,
  immutable content/history, occurrence identity, schedules, idempotency,
  durable ownership, shared spend, and complete recovery archives.
- `src/generation.rs` builds bounded model requests and validates references,
  questions, evidence, and independent critic results. `src/runtime.rs` makes
  external requests outside SQLite transactions and rechecks saved ownership
  and source/content revision before publishing. Never hide unknown paid work.
- `src/runtime.rs` owns Access, Host/origin/CSRF, the singleton SQLite Durable
  Object, alarms, private R2 assets/backups, remote readback, and fresh restore.
- `src/web.rs` / `assets/` own escaped server-rendered HTML and presentation.
  Browser JS owns reconciliation, not durable state or an offline mutation
  queue. Core forms work without JavaScript; no frontend framework/build.
- Alternate hosts redirect reads only. Never replay a mutation across origins,
  broaden identity trust for convenience, or attach production capabilities to
  development. The `.dagger/` gate is the only TypeScript surface; browser and
  retained narrow recovery tools are deliberate plain-JS exceptions.

## Runtime contracts

- Answer, immutable event, schedule, held result, and exact operation receipt
  commit atomically. Identical retry returns the saved result; changed payload
  or stale identity conflicts. Model calls never enter that transaction.
- Assistance, intro reading, self-check answer exposure, deliberate extra
  practice, and “I know this already” cannot become unaided success. Unknown
  grading stays ungraded. Edits and disputes preserve historical wording and
  original attempts; overrides remain separate records.
- Feedback remains until deliberate Next. Offline work pauses, drafts remain
  in-page, and lost responses reconcile with the same frozen operation/payload.
  Never preload protected answers, notes, goal titles/intent, or edit fields.
- Generation/meaning/criticism share bounded allowance. Independent hard critic
  judgments at 0.80 veto; missing/malformed judgments do not publish. Candidates
  and unknown reservations survive failure; critic-only retry reuses saved
  judgments and creates new send ownership without regenerating candidates.
- Daily and pre-release complete JSON+photo snapshots go to private R2 with
  exact checksum/full-byte readback and 30-day retention. Targets are RPO 24
  hours / RTO 60 minutes, not guarantees. Preserve compatible artifacts and
  private configuration independently of the application state.
- Restore into an unused isolated Durable Object namespace. Uncertain work
  remains paused; never clone live integrations or writer ownership. Old Go
  SQLite snapshots retain their historical formats and compatible binaries.
- `.env` / `.dev.vars` are ignored private operator material, never inputs to
  copy wholesale. Production secrets use Worker bindings; never source retained
  `/etc/scry/scry.env`, expose credentials, or place them in artifacts/logs.

## Gates and proof

- Start verification with `.agents/skills/scry-qa/SKILL.md`; read it explicitly
  when ambient skill discovery is disabled. `docs/qa/system.md` separates
  synthetic, browser, live-provider, private-ingress, and recovery evidence.
- Native checks: `cargo test --locked`,
  `cargo clippy --all-targets -- -D warnings`. Target checks:
  `cargo check --locked --target wasm32-unknown-unknown`,
  `worker-build --release --locked`. `bun run dev` / `bun run rust-dev` create
  an isolated fresh workerd/SQLite/R2 session with no inherited capabilities.
- `bun run ci` / `bun run ci:local` run the source-snapshot gate in
  `scripts/scry-ci`. Release artifacts need `--require-committed`; stage only
  tested bytes and never rebuild between proof and activation. No deployment
  credentials enter a build gate. A smoke pass is candidate evidence.
- Exercise the changed real surface. Browser/story receipts bind source,
  environment, story, and criterion; Jev critics remain advisory. Use actual
  pointer/keyboard events on isolated synthetic data. UNVERIFIED is not PASS.
- `qa/walk` and `scripts/critics/` need receipts for the current Worker target;
  historical Go/retired-Rust walks cannot silently certify it. Phone acceptance,
  model quality, deployed JWT ingress, and independent remote recovery need
  their own observations. Never lower gates or claim unrun canaries.
- `deploy/install.sh`, `deploy/activate.sh`, and `deploy/restore.sh` belong to
  retained VM recovery; `deploy/cloudflare-hosting/` describes existing Go
  production. Do not restart its stopped predecessor or overwrite live state.
- `bin/`, `scripts/scry-cloudflare-data`, and `scripts/lib/scry_ops.py` preserve
  old formats intentionally. Their checks remain separate from the rewrite.

## Sources of truth and work

VISION owns intent, USER_STORIES owns root stories, SPEC owns criteria and
architecture, DESIGN owns the notebook, and docs/qa/system owns verification.
Current code/tests own implemented behavior. Dated migration, beta, dogfood,
research, and Go receipts remain history; `docs/runbook.md` owns deployed state.

Work from the current request, preserve overlapping work, and keep ownership
and evidence in the session/PR. Glass in scope `misty-step/scry` owns current
operational work state via README; MIS/Linear links remain provenance. Close a
named item only after accepted scope and required proof are satisfied.

## Merging

`master` requires `ci`, `foundation`, `story-walk` and `foundation-review`, up to date, with linear history. `foundation-review` wants the `kaylee-agent[bot]` App's `agent-review` approval on the PR head (`agent-review --repo misty-step/scry --pr N`, under `pass-env`); push again and it is stale, so review again. Merge with `gh pr merge --squash --match-head-commit <sha>` once every check is green. Never use `--admin`: a block means a check is red or missing, the head moved, or the branch is behind `master`.
