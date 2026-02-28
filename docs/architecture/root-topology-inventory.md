# Root Topology Inventory (Slice 1)

Issue: #271 (parent #270)
Date: 2026-02-27

## Objective

Create a full root-level inventory and classify each item so follow-up cleanup slices can move files safely with minimal CI/script breakage.

## Current root snapshot

```text
.changeset
.claude
.codecov.yml
.env.example
.env.local
.env.preview
.env.production
.env.sentry-build-plugin
.env.test.local
.env.vercel
.env.vercel-prod
.env.vercel.local
.env.vercel.preview
.git
.github
.gitignore
.gitleaks.toml
.lefthook.yml
.lighthouserc.js
.mcp.json
.next
.npmrc
.pi
.playwright-mcp
.prettierignore
.prettierrc.json
.size-limit.json
.trivyignore
.vercel
AGENTS.md
app
CLAUDE.md
components
components.json
convex
coverage
docs
eslint.config.mjs
eval_results.json
evals
experiments
GEMINI.md
hooks
instrumentation-client.ts
instrumentation.ts
lefthook.yml
lib
lighthouserc.json
middleware.ts
next-env.d.ts
next.config.ts
node_modules
package.json
playwright-report
playwright.config.ts
pnpm-lock.yaml
postcss.config.mjs
prettier.config.mjs
public
README.md
scripts
sentry.client.config.ts
sentry.edge.config.ts
sentry.server.config.ts
test-results
tests
thinktank.log
tsconfig.json
tsconfig.tsbuildinfo
types
vercel.json
vision.md
vitest.config.ts
vitest.setup.ts
```

## Classification and disposition

### A) Runtime source (keep at root or canonical app dirs)
- `app/`, `components/`, `hooks/`, `lib/`, `convex/`, `public/`, `tests/`, `types/`, `scripts/`
- `instrumentation.ts`, `instrumentation-client.ts`, `middleware.ts`

Disposition: **keep** (canonical runtime layout).

### B) Build/test/tooling config (keep, but dedupe in later slices)
- `package.json`, `pnpm-lock.yaml`, `tsconfig.json`, `vitest.config.ts`, `vitest.setup.ts`
- `next.config.ts`, `playwright.config.ts`, `postcss.config.mjs`, `prettier.config.mjs`, `eslint.config.mjs`
- `components.json`, `vercel.json`, `.npmrc`, `.mcp.json`
- `.gitleaks.toml`, `.trivyignore`, `.codecov.yml`, `.size-limit.json`
- `.lefthook.yml`, `lefthook.yml` (potential duplication)
- `.lighthouserc.js`, `lighthouserc.json` (potential duplication)

Disposition: **keep for now**, audit duplicates in follow-up cleanup.

### C) Governance/context docs (rationalize location)
- `README.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `vision.md`

Disposition:
- `README.md`, `AGENTS.md`: **keep at root**.
- `CLAUDE.md`, `GEMINI.md`, `vision.md`: candidate move to `docs/context/` or `docs/architecture/` in a later slice once references are updated.

### D) Repository/meta directories (keep)
- `.git/`, `.github/`, `.changeset/`, `docs/`, `evals/`, `experiments/`, `.pi/`, `.claude/`

Disposition: **keep**.

### E) Generated/local-only artifacts (must stay ignored and out of cleanup noise)
- `.next/`, `node_modules/`, `coverage/`, `test-results/`, `playwright-report/`, `.playwright-mcp/`, `.vercel/`
- `tsconfig.tsbuildinfo`, `next-env.d.ts`, `eval_results.json`, `thinktank.log`
- local env variants: `.env.local`, `.env.preview`, `.env.production`, `.env.test.local`, `.env.vercel*`

Disposition: **local-only / generated**. Keep ignored and verify no accidental tracking in cleanup slices.

## High-noise cleanup targets (priority order)

1. **Duplicate config filenames**
   - `lefthook.yml` vs `.lefthook.yml`
   - `.lighthouserc.js` vs `lighthouserc.json`
2. **Context file sprawl at root**
   - `CLAUDE.md`, `GEMINI.md`, `vision.md`
3. **Persistent generated artifacts risk**
   - ensure `eval_results.json`, `thinktank.log`, test/build outputs remain untracked in normal loops.

## Proposed next slices

### Slice 2 (issue #271 follow-up)
- Resolve duplicate config surfaces (`lefthook`, `lighthouse`) with single source-of-truth.
- Update docs/scripts to reference only canonical files.

### Slice 3 (issue #271 follow-up)
- Move non-root-critical context docs (`CLAUDE.md`, `GEMINI.md`, optionally `vision.md`) under `docs/`.
- Add redirect/reference notes where needed.

### Slice 4 (issue #271 follow-up)
- Add automated root hygiene check (simple script or CI guard) for unexpected tracked artifacts.

## Verification for this slice

- Inventory generated from current root.
- No runtime or CI behavior changes in this slice.
