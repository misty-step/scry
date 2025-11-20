# Scry - AI-Powered Spaced Repetition & Quiz Generation

## Project Overview
Scry is a sophisticated learning application combining AI-powered quiz generation with a pure implementation of the FSRS (Free Spaced Repetition Scheduler) algorithm. Built on the "Hypersimplicity & Pure Memory Science" philosophy, it eschews gamification comfort features in favor of raw efficiency and scientific validity.

**Tech Stack:**
- **Frontend:** Next.js 15 (App Router), React 19, Tailwind CSS v4, shadcn/ui.
- **Backend:** Convex (Real-time database, Auth, Serverless Functions).
- **AI:** OpenAI GPT-5 mini (Production, Reasoning) & Google Gemini 2.5 Flash (Fallback).
- **Auth:** Clerk via Convex Auth (Magic Links).
- **Testing:** Vitest (Unit/Integration), Playwright (E2E).

## Core Architecture & Philosophy

### 1. Pure FSRS (Non-Negotiable)
- **No Daily Limits:** If 300 cards are due, the user sees 300 cards.
- **No Comfort Features:** No artificial interleaving or "easy" buttons that distort the algorithm.
- **Algorithm:** Uses `ts-fsrs` for scientifically accurate review scheduling.

### 2. Backend-First Workflow (CRITICAL)
- **Rule:** Never implement frontend code before backend functions exist.
- **Process:**
    1. Define Schema/Mutation in `convex/`.
    2. Verify via `npx convex dev`.
    3. Implement Frontend UI in `app/` or `components/`.
- **Why:** Prevents "Function not found" runtime errors.

### 3. Convex-Safe Bandwidth
- **Constraint:** Optimize for 10k+ card collections on limited plans.
- **Anti-Pattern:** Never use `.collect()` on user-scoped queries.
- **Pattern:** Always use `.take(limit)` and compound indexes (e.g., `by_user_active`).
- **Stats:** Use incremental counters (`userStats` table), not O(N) aggregations.

### 4. Concept-Based Data Model (v2.3+)
- **Concepts (`concepts` table):** Atomic units of knowledge. Holds FSRS state.
- **Phrasings (`phrasings` table):** Variations of questions for a single concept.
- **Legacy:** `questions` table is deprecated/read-only (migrated to concepts/phrasings).

## Operational Guide

### Development Environment
- **Package Manager:** `pnpm` (v10+) only.
- **Start Dev Server:** `pnpm dev` (Runs Next.js + Convex concurrently).
- **Convex Dashboard:** `npx convex dashboard`.

### Testing
- **Unit/Integration:** `pnpm test` (Vitest).
- **Watch Mode:** `pnpm test:watch`.
- **Coverage:** `pnpm test:coverage`.
- **E2E:** `pnpm test:e2e` (Playwright).
- **API Contract:** `pnpm test:contract`.

### Deployment (Atomic & Safe)
- **Production:** Use `./scripts/deploy-production.sh` (Validates env, health, atomic deploy).
- **Manual Preview:** `vercel` (Creates isolated branch-based backend).
- **Key Rule:** Deployment order is always **Convex Backend -> Validation -> Frontend**.
- **Environment Variables:**
    - **Convex:** Set in Convex Dashboard.
    - **Vercel:** Set in Vercel Dashboard (Critical: `CONVEX_DEPLOY_KEY` for `prod` vs `preview`).
    - **Local:** `.env.local` (Do NOT use `source .env.local`, separate files for Vercel/Convex).

### Migration Workflow
1. **Phase 1:** Make schema field optional. Deploy.
2. **Phase 2:** Run migration script (with dry-run & verification).
3. **Phase 3:** Remove field from schema. Deploy.
- **Tool:** Use `./scripts/run-migration.sh`.

## Key Directories
- `convex/`: Backend schema, mutations, queries.
    - `schema.ts`: Data model definition.
    - `concepts.ts`: Core review logic.
    - `aiGeneration.ts`: AI job processing.
- `app/`: Next.js App Router.
- `components/`: Reusable UI (shadcn).
- `scripts/`: Critical ops scripts (deployment, health checks, migrations).
- `docs/`: Detailed runbooks and ADRs.

## AI Configuration
- **Provider:** Environment-driven (`AI_PROVIDER`).
- **Production:** OpenAI (GPT-5-mini) for reasoning/quality.
- **Fallback:** Google Gemini 2.5 Flash.
- **Genesis Lab:** `convex/lab.ts` for comparing provider quality.

## Common Tasks

### Creating a New Feature
1.  **Schema:** Edit `convex/schema.ts`.
2.  **Backend:** Create `convex/myFeature.ts` with mutations/queries.
3.  **Test:** Write `convex/myFeature.test.ts`.
4.  **UI:** Create component in `components/my-feature.tsx`.
5.  **Integrate:** Add to `app/` page.

### Debugging
- **Logs:** `npx convex logs` (Backend), Browser Console (Frontend).
- **Sentry:** Configured for error tracking (check `docs/observability-runbook.md`).
- **Health:** `./scripts/check-deployment-health.sh`.

### Troubleshooting
**401 Unauthorized on `npx convex deploy`:**
- **Cause:** `CONVEX_DEPLOY_KEY` is present in `.env.local` but is invalid or meant for production.
- **Fix:** Remove `CONVEX_DEPLOY_KEY` from `.env.local`. Use `npx convex dev` for local development, which uses your personal login token automatically. Use `./scripts/deploy-production.sh` for production, which handles keys correctly.
