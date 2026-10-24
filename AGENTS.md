# Scry

For each change load [verify-scry](.agents/skills/verify-scry/SKILL.md), use the
existing `features/` navigation map and retain actual journey proof in the card
and PR. New Hermes profiles review the checkout, then use native
`hermes skills trust` for project discovery.

Shared standard: [Constitution](https://foundations.mistystep.io/) and [Way of working](https://foundations.mistystep.io/way-of-working).

Read [SPEC.md](SPEC.md), especially "Exact recall first" and US-014 to US-021,
before scoping or verifying behavior. [GLOSSARY](GLOSSARY.md) owns the words. It is the only story/acceptance authority. [VISION](VISION.md) explains
intent; [DESIGN](DESIGN.md) owns visuals; [ADRs](docs/adr/README.md) own technical
decisions. Do not restate their contracts here.

## Architecture and boundaries

[ADR 004](docs/adr/004-rust-cloudflare-candidate.md) owns the current Rust
candidate's architecture, module boundaries, ingress and persistence decisions.
[The candidate procedure](docs/rust-cloudflare.md) owns build/release/isolated
restore instructions. Historical Go decisions are labeled in the ADR index;
retired Rust architecture and receipts remain historical, not candidate proof.

## Product, privacy, and historical stores

[The runbook](docs/runbook.md) owns origins, recovery and operational evidence.
Source changes and green gates do not settle serving provenance or authorize
activation. Do not reactivate, import, rename or delete historical stores,
copy live integrations, or overwrite acknowledged data without explicit scope.

`.env`, `.dev.vars`, and retained `/etc/scry/scry.env` are private operator
material: never source or copy them into builds, logs or evidence. Build and
preview work uses isolated synthetic data without production capabilities.

## Gates and proof

Read [QA](docs/qa/system.md) and [.agents/skills/scry-qa/SKILL.md](.agents/skills/scry-qa/SKILL.md).
QA runs against SPEC, never edits intent to fit implementation. Use the existing
native, target, runtime and browser gates documented there; `qa/walk --all`
discovers numbered stories from SPEC. Worktree receipts are development proof;
release artifacts require committed source and the exact tested bytes.

Phone experience, real-provider usefulness, deployed private ingress and
independent recovery require their own observations. Historical receipts and
synthetic success do not transfer acceptance. UNVERIFIED is not PASS.

## Sources of truth and work

[README § Start or resume work](README.md#start-or-resume-work) owns the work
entrypoint. Preserve overlapping work; link story/criterion IDs, exact source,
evidence and remaining limits in the owning task/PR. Dated research, migration,
dogfood, beta and Go receipts retain their original meanings and bytes.

## Shipping

Use the native task's required independent review and exact-head protected CI.
Merge only verified source through normal protection. A source merge is not
deployment or closure of the rewrite acceptance; promotion follows the
[candidate procedure's verifiable cutover conditions](docs/rust-cloudflare.md#reviewable-release-procedure).
Foundation bot approval and adoption checks are retired.

## Agent skills

### Issue tracker

GitHub issues on `misty-step/scry`, initiative #273. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five roles (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: root `GLOSSARY.md` (created lazily) and `docs/adr/`. See `docs/agents/domain.md`.
Run `sitrep where <task-kind>` before dispatching.

## No fallbacks

No fallbacks, backup paths or masking safety nets. Write the one best solution: direct, clean, simple, elegant. If it fails, it must fail loudly and visibly so we can fix it. Security checks (auth, signatures, schema validation) stay, but fail loudly, never silently.

## Pooled accounts

Pooled accounts: all subscriptions and AI accounts (personal, Misty Step, R90) are pooled for all work in every direction. When one is exhausted (e.g. codex-r90 out of credits), switch to another pooled account; never stop, downgrade or buy credits.
