# Scry

Scry is a private, prompt-first learning app. The governing journey is
**Create → express what I want to learn → useful tailored study/reference and
practice material → refine through actual practice and feedback**, without a
mandatory setup questionnaire. [VISION](VISION.md) owns the intent;
[US-013](USER_STORIES.md#us-013-create-from-what-i-want-to-learn) leads to the
concrete [Create acceptance](SPEC.md#s11--create-useful-material-from-intent-us-013).
Concept-centered, retrieval-led study and honest learning history remain core.

**Implemented today:** one Go/SQLite process owns HTML/HTMX, bounded generation,
and recovery. Add accepts private text or an optional photo; URLs stay private
text, not inferred web research. **Approved future target:** a ground-up
Rust-on-Cloudflare rewrite, not shipped or activated by these docs. Existing
capture checks do not prove the complete Create/refinement contract.

[Product direction](VISION.md) · [Stories](USER_STORIES.md) ·
[Feature map](features/README.md) · [Behavior and architecture](SPEC.md) ·
[Decisions](docs/adr/README.md) · [Operations](docs/runbook.md) ·
[Verification](docs/qa/system.md)

## Access and scope

The private production app is at **https://scry.study**, behind Cloudflare
Access and the exact owner-subject check in Worker `scry-app-host`. The shared
Access team domain is `misty-step-pantry.cloudflareaccess.com`; it is the
identity/login domain, not a Pantry application Worker URL. The
[runbook](docs/runbook.md) owns deployed origins and state; see the
[Cloudflare hosting notes](deploy/cloudflare-hosting/README.md) for the request path.
The earlier phone flow and concept-centered v5 decisions (MIS-162) remain
dated provenance, not blanket acceptance of the prompt-first goal or authority
to replace the live runtime. The [runbook](docs/runbook.md#current-authority)
owns deployed state. `www.scry.study`
and `scry.mistystep.io` redirect reads to the
canonical origin; alternate-host mutations are rejected, not replayed. The
former exe.dev app service is stopped and disabled; its database and the old
Rust Worker/Postgres stores remain recovery material, not active writers.

There is no public signup, billing, separate frontend service, or public
CLI/MCP/API compatibility requirement. The old Rust workspace and clients are
retired. Historical learning-science research and recovery tools remain.

## Start or resume work

Begin with the current operator request as authority; a ticket is not a
prerequisite. [Glass](https://mirrodin.tail5f5eb4.ts.net) in scope
`misty-step/scry` owns current work state: owner, accepted scope, pause/blocker,
and next permitted action. Reconcile the existing owning item using the
[ticket contract](SPEC.md#ticket-contract), not a second status ledger.
Imported MIS identities and [legacy Linear project links](https://linear.app/misty-step/project/scry-7d7eb0cc48bb)
preserve provenance, not current execution authority. Off-Linear migration
reconciliation and writer repointing have a separate owner; this repo neither
changes their state nor claims every writer has been cut over.

Locate the checkout for that work before developing: `git worktree list --porcelain`
shows candidates; in the selected checkout, `git status --short --branch` and
`git rev-parse HEAD` identify local changes and the committed base. Reconcile
these with the work record and inspect relevant changed/untracked docs. A cwd,
branch name, or latest commit does not establish authorization. Preserve dirty
work and isolated experiments; neither is automatically the release baseline.
Local design notes may carry newer direction than HEAD without being published
specification or shipped behavior.

Read [VISION](VISION.md), [USER_STORIES](USER_STORIES.md), and
[SPEC authority](SPEC.md#authority-and-open-decisions), including
[Create acceptance](SPEC.md#s11--create-useful-material-from-intent-us-013),
before choosing an action. Earlier design rounds are linked there as provenance;
read [DESIGN](DESIGN.md) only for interface work. Approved future Rust direction
is not permission to ship an imported historical Go build, migrate live data,
or deploy. Operating commands below still belong to the implemented Go app.

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

The host gate requires Go 1.27.1 on Linux amd64, Node 22+, Gitleaks 8.30.1, and
Playwright 1.63.0 with Chromium for the critic browser cases: the Dagger image's
paths by default, or `SCRY_CRITICS_NODE_PATH`/`SCRY_CRITICS_CHROMIUM_PATH` as
exported by `.exe/setup.sh` on the `scry-ws` workspace.
The full gate supplies pinned tooling through Dagger. Both snapshot the actual
non-ignored source, check Go and retained recovery contracts, scan for secrets,
and exercise the exact built binary against isolated synthetic data.

The exported artifact contains `scry`, `SHA256SUMS`, `source.json`,
`source.tar.gz`, `proof.json`, and command/smoke evidence. Worktree output is
explicitly labeled; `--require-committed` rejects it. Artifact destinations must
be unused. Deploy those tested bytes rather than rebuilding them.

`bun run test` runs Go tests; `bun run test:recovery` checks the retained
historical recovery tools. `.github/workflows/ci.yml` and Buildkite run the
current gate; run it off the desktop with `ws run --task T -- bun run ci` on
`scry-ws`. The pre-push hook (`git config core.hooksPath .githooks`) only runs
the sub-second `foundation-check` structural check and lists the stories
required CI will walk. `.github/workflows/nightly.yml` walks every live story
on master daily, retains the receipt for 30 days, and opens or updates one
"Nightly story walk failed" issue on failure.

## Deployment and recovery

Production uses one Cloudflare Container behind Access and `scry-app-host`.
Cloudflare release steps and v5's irreversible migration boundary live in the
[runbook](docs/runbook.md#schema-v5-release-boundary) and
[hosting notes](deploy/cloudflare-hosting/README.md). The retained exe.dev VM
scripts (`deploy/install.sh`, `deploy/activate.sh`, `deploy/restore.sh`) are
recovery tooling, not the active deployment path. Restore an older v4 snapshot
only into an unused path with the previous compatible binary; never overwrite
acknowledged live writes or start a second writer.

Private production configuration must not be sourced or committed.
`deploy/scry.env.example` lists safe empty defaults and
`deploy/cloudflare-hosting/wrangler.jsonc` carries public vars, not secrets.
The dedicated provider key “Scry personal (exe.dev)” has a $25/week limit
(raised 2026-09-23). The app allows $3.50 per rolling 24 hours and reserves
$0.50 per generation attempt; unknown sent usage remains accounted.
`SCRY_MODEL` is an explicit model choice. An optional Exa secret enables Topic
web search and Link contents; pasted text is never searched. Jev short-answer,
semantic prose and the content critic share the same bounded allowance.

The approved recovery policy remains daily and pre-release off-VM snapshots
with 30-day retention, targeting RPO 24 hours and RTO 60 minutes. These are
objectives, not guarantees. Old Worker/native recovery material is preserved
separately; do not reactivate or import a frozen store.


## Source boundaries

- `internal/learning`: pure grading, concept state/selection, and pinned FSRS.
- `internal/store`: SQLite schema v5, atomic history, relations, notes, jobs, and spend.
- `internal/generation` / `internal/semantic`: bounded Exa/model/Jev calls outside SQL.
- `internal/web`: private Stream/Map/Concept routes, CSRF, HTML, embedded assets.
- `internal/recovery`: consistent archives, remote readback, and safe restore.
- `deploy/backup-gateway`: narrow append/read-only Worker over private R2.
- `scripts/scry-ci` and `.dagger/`: source-bound checks and release evidence.
- `bin/`, `scripts/scry-cloudflare-data`, and `etc/`: retained historical
  Postgres/Worker recovery tools, not current application deployment paths.

[The private acceptance receipt](docs/qa/personal-go-acceptance-20260909.json)
records the earlier live generation, touch/interruption, and data-restore
observations. Passing checks and an approved phone flow do not establish
longitudinal learning gains.
