# CLAUDE.md

Operational guidance for Claude Code in scry repository.

## Core Operations

**Package Manager:** pnpm only (10.0+)
**Dev Setup:** `pnpm dev` (Next.js + Convex concurrently)
**Convex:** dev=amicable-lobster-935, prod=uncommon-axolotl-639
**Tests:** `pnpm test` | `pnpm test:contract` | `pnpm test:coverage`

## Architecture Rules (MANDATORY)

**Backend-First Workflow:**
1. Implement mutation/query in `convex/` with args schema
2. Wait for `npx convex dev` → "Convex functions ready!"
3. Then import `api` in frontend
4. Never frontend-first (causes runtime "function not found")

**Mutation Pairs:** Archive↔Unarchive | SoftDelete↔Restore | HardDelete (irreversible)
- Both mutations MUST exist before implementing UI
- Use `validateBulkOwnership()` for atomic validation

**Confirmation UX:**
- Reversible → `useUndoableAction()` (soft delete, archive)
- Irreversible → `useConfirmation()` with `requireTyping`

## Pure FSRS Philosophy (NON-NEGOTIABLE)

- No daily limits (300 due = show 300)
- No artificial interleaving/comfort features
- No "improvements" to algorithm
- Pure FSRS calculations only
- Natural consequences teach sustainable habits

## Environment & Deployment

**Vercel ≠ Convex** — separate systems, configure both

| Location | Variables |
|----------|-----------|
| Convex backend | `GOOGLE_AI_API_KEY`, `OPENROUTER_API_KEY`, `AI_MODEL`, optional `REVIEW_AGENT_MODEL` |
| Vercel | `CONVEX_DEPLOY_KEY` (different per env), `CLERK_*` |

**Build Commands:**
- `pnpm dev` — development with hot reload
- `pnpm build:local` — local production testing (deploys to dev)
- `vercel --prod` / `vercel` — production/preview (handles everything)
- Never run `pnpm build` directly (only for vercel-build.sh)

**Scripts:** `./scripts/deploy-production.sh` | `./scripts/check-deployment-health.sh`

**Env Loading Gotcha:** `.env.production` uses Vercel format, not bash export syntax:
```bash
# WRONG: source .env.production (silently fails)
# RIGHT: export CONVEX_DEPLOY_KEY=$(grep CONVEX_DEPLOY_KEY .env.production | cut -d= -f2)
```

Full deployment docs: `docs/operations/`, `docs/runbooks/`

## AI Provider Configuration

**Production:** Google Gemini 3 Flash Preview (`google/gemini-3-flash-preview`)

**Provider:** `convex/lib/aiProviders.ts` — `initializeProvider()` for centralized model initialization

**Agent-specific override:**
- `REVIEW_AGENT_MODEL` (optional) overrides `AI_MODEL` for review chat only.

**Model change:**
```bash
npx convex env set AI_MODEL "google/gemini-3-flash-preview" --prod
```

## Bandwidth Optimization (Convex Starter = 1GB/mo)

**Anti-patterns:** Unbounded `.collect()`, client-side filtering after over-fetch, reactive O(N) calculations

**Best practices:** Always `.take(limit)`, use compound indexes (`by_user_active`), O(1) stats via `userStats` table

**Query checklist:** Scale to 10k? Uses `.take()`? Index filtering? Incremental counter?

ADR: `docs/adr/0001-optimize-bandwidth-for-large-collections.md`

## Backend Modules

`concepts.ts` (review system) | `phrasings.ts` (variations) | `questionsInteractions.ts` (FSRS) | `generationJobs.ts` + `aiGeneration.ts` (AI) | `lib/validation.ts`

## Observability

Sentry + Vercel Analytics. Setup via Vercel Integration. Runbook: `docs/observability-runbook.md`

## Migration Patterns

Required: dry-run, diagnostic query, runtime checks (`'field' in (doc as any)`), batching
3-phase removal: make optional → migrate → remove field. Guide: `docs/migration-development-guide.md`
