# scry Repo Brief

Generated: 2026-04-23

## Vision and Purpose

scry is an agentic spaced-repetition app for turning arbitrary source material into durable memory. The product center is the review experience at `/`: Willow, the review chat, retrieves due concepts and records feedback through concept-level FSRS. The app is intentionally austere about memory science: no daily limits, no comfort-mode shortcuts, no algorithmic "improvements" to FSRS. If 300 concepts are due, the product should surface the learning debt honestly.

The long-term product thesis is "agentic Anki killer": AI generation creates and refines atomic concepts, IQC keeps the library clean, and the review surface becomes dynamic enough to handle multiple question types without hardcoded UI branches. The current backlog reflects that direction: prompt/eval hardening, content-type completeness, agent-ready module extraction, and schema-driven generative UI.

## Stack and Boundaries

- Frontend: Next.js 16.1.6 App Router, React 19.2.4, TypeScript, Tailwind v4, shadcn/Radix components, Clerk auth, Sentry, PostHog, Vercel Analytics and Speed Insights.
- Backend: Convex functions in `convex/`, with generated API types under `convex/_generated/`. Convex owns schema, queries, mutations, actions, FSRS state, generation jobs, embeddings, IQC, and webhook-side subscription updates.
- AI/LLM: Google/OpenRouter through AI SDK providers, Langfuse tracing, promptfoo evals in `evals/`, prompt templates in `convex/lib/promptTemplates.ts` and `evals/prompts/`.
- Payments/ops: Stripe API routes in `app/api/stripe/**`, Vercel build via `scripts/vercel-build.sh`, production deployment documented as Convex backend first, then Vercel frontend.
- Harness: existing `.pi/` is scaffolded human/local foundation and must be preserved. `.claude/context.md` is historical pattern memory. This tailor run installs spellbook skills into `.agent/skills/` and uses `.claude/skills/` plus `.codex/skills/` plus `.pi/skills/` as bridges only.

Backend-before-frontend is load-bearing. For Convex work: change schema/query/mutation first, validate generated API/type readiness, then wire UI. Destructive semantics require reverses: archive/unarchive, softDelete/restore; hard delete needs explicit confirmation UX.

## Load-Bearing Gate

The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

Pre-commit also runs TruffleHog, `scripts/tooling/topology-hygiene-check.sh`, `pnpm exec tsc --noEmit`, and lint-staged. Pre-push runs `scripts/test-changed-batches.sh` and `pnpm test:contract`. These hooks are defense in depth; they do not replace the ship gate above.

Forbidden without explicit operator approval: `pnpm build:local`, `pnpm build:prod`, `pnpm convex:deploy`, `./scripts/deploy-production.sh`, production migration scripts, or anything that changes non-local Convex/Vercel state.

## Invariants

- Package manager is `pnpm` today. Bun is a migration spike, not the default. `ci-bun.yml` and `docs/architecture/bun-migration-spike.md` are parity evidence, not permission to switch defaults.
- Pure FSRS lives in `convex/fsrs/engine.ts`, `convex/fsrs/conceptScheduler.ts`, and selection policy. Do not add daily caps, artificial review limits, or "ease" affordances that change the curve.
- Convex bandwidth matters: use indexes and bounded `.take()` or pagination. Runtime paths must not use unbounded `.collect()`. `docs/guides/convex-bandwidth.md` and `docs/adr/0001-optimize-bandwidth-for-large-collections.md` are the policy anchors.
- `convex/schema.ts` is an API contract. Removals require the optional-field migration pattern from `docs/guides/writing-migrations.md`: make optional, dry-run/backfill, verify diagnostics, then remove.
- Generated API types in `convex/_generated/**` are ground truth after Convex codegen; stale model memory is not.
- `/` is the review home. `/agent` redirects to `/`. `/tasks` and `/action-inbox` remain routes but are not primary navigation.
- Source-of-truth order: `package.json`, `.github/workflows/**`, and `lefthook.yml`; then `CLAUDE.md`; then docs, which must be verified for freshness.
- Backlog is file-driven in `backlog.d/`. Branches for shippable tickets use `<type>/<id>-<slug>` with bare numeric IDs so `/ship` can preserve `Closes-backlog:` and `Ships-backlog:` trailers.

## Known Debts

- `BACKLOG-007` Agent-Ready Architecture: split `components/agent/review-chat.tsx` (1389 lines), `convex/concepts.ts` (1295), `convex/aiGeneration.ts` (1202), and `convex/iqc.ts` (827) by extraction only, with zero public API changes.
- `BACKLOG-003` and `BACKLOG-004`: phrasing-generation evals and content-type eval suite must mature before broadening generation behavior.
- `BACKLOG-006`: cloze and short-answer are schema-scaffolded but generation contracts, validation, review UI, grading, and eval cases are incomplete.
- `BACKLOG-008`: review artifact rendering is still hardcoded in `review-chat.tsx`; target is a thin Zod-validated renderSpec registry, not a general LLM-generated UI framework.
- Bun migration is in progress but not complete. Keep `pnpm` guidance unless the parity matrix and CI cutover land.
- Root topology debt remains around duplicate config surfaces and root doc sprawl; `scripts/tooling/topology-hygiene-check.sh` is the current guard.
- Documentation drift exists: README still says Next.js 15 and references some scripts that are not in `package.json`; treat README as orientation, not authority.

No P0 unfiled debt was found during this run. Operational P0s should become GitHub issues or `backlog.d/NNN-*.md` before appearing in `AGENTS.md`.

## Terminology

- Concept: atomic unit of knowledge, scheduled with concept-level FSRS.
- Phrasing: one quizable wording for a concept.
- Willow: agentic review chat persona/experience.
- IQC: intelligent quality control, including merge/action-card flows.
- Generation job: async AI generation state machine in `generationJobs`.
- Thin/Tension: library/IQC quality signals, not primary navigation tabs.
- Action inbox: IQC triage route at `/action-inbox`.
- Ship gate: the single quality contract above, not a convenience script.

## Session Signal

Recurring corrections and failure modes:

- Preserve scaffolded or human-authored harness content. The prior Pi bootstrap postmortem explicitly called accidental `AGENTS.md` overwrite and malformed newline output a system defect.
- Do not trust generic docs over live config. README/older docs drift; package scripts, workflows, and lefthook decide.
- CI parity must match protected GitHub reality. Prior context warned against worker/CI mismatch and non-blocking advisory workflows masquerading as gates.
- Keep deployment/migration commands explicit opt-in. The repo has deploy-coupled scripts, but local agents must not run them as routine verification.
- Hindsight review of PR #243 identified `review-chat.tsx` as a mixed-responsibility hot file; follow-up architecture work should extract, not rewrite.

Validated patterns the user has ratified:

- Backend-first Convex sequencing with bounded reads and contract tests.
- Aggressive simplification by deletion when the product surface has dead routes or duplicated flows.
- File-driven backlog with shaped `.ctx.md` packets and explicit oracle checklists.
- Reviewer focus on concrete, fixable risks over style commentary.
- Tamiyo voice and pure-FSRS doctrine are repo identity, not decorative prose.
