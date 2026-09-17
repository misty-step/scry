# Scry

Scry is a personal, quiz-first learning app. Add something to remember, answer
useful questions, and return when review is worthwhile. One Go process owns
SQLite, the phone-first HTML/HTMX interface, bounded AI generation, and recovery.

[Product direction](VISION.md) · [Behavior and architecture](SPEC.md) ·
[Operations](docs/runbook.md) · [Verification](docs/qa/system.md)

## Access and scope

The private application is at **https://scry.study**, behind exe.dev login and
an exact owner-identity check. The operator approved the earlier phone flow,
not every later experience; [current product authority](VISION.md) records the
foundations rejection and design reset.
`www.scry.study` and `scry-app.exe.xyz` redirect reads to the canonical origin;
alternate-host mutations are rejected, not replayed. Old production/staging
Workers are paused and their recovery material is preserved, not imported.

There is no public signup, billing, separate frontend service, or public
CLI/MCP/API compatibility requirement. The old Rust workspace and clients are
retired. Historical learning-science research and recovery tools remain.

## Start or resume work

Begin with the current operator request as authority; a ticket is not a
prerequisite. Reconcile any existing owning issue in the
[Scry Linear project](https://linear.app/misty-step/project/scry-7d7eb0cc48bb),
not the checkout's apparent age or a historical approval. Linear owns recorded
work state: current owner, scope, pause/blocker, and next permitted action; use the
[existing ticket contract](SPEC.md#ticket-contract), not a second status ledger.

Locate the checkout for that work before developing: `git worktree list --porcelain`
shows candidates; in the selected checkout, `git status --short --branch` and
`git rev-parse HEAD` identify local changes and the committed base. Reconcile
these with the work record and inspect relevant changed/untracked docs. A cwd,
branch name, or latest commit does not establish authorization. Preserve dirty
work and isolated experiments; neither is automatically the release baseline.
Local design notes may carry newer direction than HEAD without being published
specification or shipped behavior.

Read [VISION](VISION.md), [SPEC authority](SPEC.md#authority-and-open-decisions),
and the linked [concept-centered design note](docs/design/concept-centered-study.md)
before choosing an action. Operator direction, accepted criteria, implemented
behavior, and unaccepted proposals are distinct; a design pause is not permission
to continue the old build plan.

For authorized work, select [changed-surface proof](docs/qa/system.md#choose-proof-before-running-checks).
For deployed state, follow the [runbook's authority](docs/runbook.md#current-authority)
and [release evidence](docs/runbook.md#evidence), not HEAD or a green gate.
Keep exact source/artifact and remaining evidence linked from the owning issue/PR
so another agent can resume without reconstructing the session.

## Develop

For an empty-state development start, use a shell without production Scry
configuration (an existing database at this path is reused):

```sh
go test ./...
go run ./cmd/scry serve --dev --db data/scry.sqlite --addr 127.0.0.1:8080
```

Development identity is explicitly loopback-only; `--dev` does **not** clear
inherited model or backup configuration. For reviewable authored data without
provider spend or production recovery authority, follow the
[isolated synthetic QA recipe](docs/qa/system.md#local-authored-fixture).
`data/`, build outputs, and private `.env` files are ignored; do not attach
production capabilities to a development workspace.

The UI is embedded in the binary. There is no frontend build step. Vendored
HTMX and its license live in `internal/web/assets/`; browser JavaScript handles
presentation and interrupted-request reconciliation, not durable storage.

## Check and build

```sh
bun run ci
bun run ci:full -- --out target/ci-release --require-committed
```

The host gate requires Go 1.27.1 on Linux amd64, Node 22+, and Gitleaks 8.30.1.
The full gate supplies pinned tooling through Dagger. Both snapshot the actual
non-ignored source, check Go and retained recovery contracts, scan for secrets,
and exercise the exact built binary against isolated synthetic data.

The exported artifact contains `scry`, `SHA256SUMS`, `source.json`,
`source.tar.gz`, `proof.json`, and command/smoke evidence. Worktree output is
explicitly labeled; `--require-committed` rejects it. Artifact destinations must
be unused. Deploy those tested bytes rather than rebuilding them.

`bun run test` runs Go tests; `bun run test:recovery` checks the retained
historical recovery tools. `.github/workflows/ci.yml`, Buildkite, and the
pre-push hook use the same current gate.

## Deployment and recovery

`deploy/install.sh` stages an immutable binary. `deploy/activate.sh` drains the
service, verifies an off-VM backup and schema compatibility, switches the active
release, and checks the actual process and readiness. Incompatible rollback
never overwrites the live database. `deploy/restore.sh` restores only into an
unused path, leaving uncertain jobs paused and activation explicit.

Production configuration is a private systemd environment file, not a sourced
shell script. See `deploy/scry.env.example` and the [runbook](docs/runbook.md).
The Scry-only OpenRouter key is retained in ignored `.env` as
`OPENROUTER_API_KEY` (mode `0600`). The app receives its model capability through
the private `scry-model` exe integration, not a VM-held provider key. Limits are
$1/day UTC at the provider and $1 rolling 24 hours in the app, with $0.20
conservative per-attempt reservations.

The approved recovery policy is daily and pre-release off-VM snapshots with
30-day new-app retention, targeting RPO 24 hours and RTO 60 minutes. These are
objectives, not an availability guarantee. Old Worker/native recovery material
is preserved separately; do not reactivate or import a frozen store.

## Source boundaries

- `internal/learning`: pure grading and pinned FSRS policy.
- `internal/store`: SQLite, atomic review/history, leases, and spend accounting.
- `internal/generation`: bounded model calls and validated publication.
- `internal/web`: private routes, CSRF, HTML, and embedded browser assets.
- `internal/recovery`: consistent archives, remote readback, and safe restore.
- `deploy/backup-gateway`: narrow append/read-only Worker over private R2.
- `scripts/scry-ci` and `.dagger/`: source-bound checks and release evidence.
- `bin/`, `scripts/scry-cloudflare-data`, and `etc/`: retained historical
  Postgres/Worker recovery tools, not current application deployment paths.

[The private acceptance receipt](docs/qa/personal-go-acceptance-20260909.json)
records the earlier live generation, touch/interruption, and data-restore
observations. Passing checks and an approved phone flow do not establish
longitudinal learning gains.
