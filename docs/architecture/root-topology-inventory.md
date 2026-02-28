# Root Topology Inventory (Slice 1)

Issue: #271 (parent #270)
Date: 2026-02-27

## Objective

Create a full root-level inventory and classify each item so follow-up cleanup slices can move files safely with minimal CI/script breakage.

## Current root snapshot

> This is a filesystem snapshot at time of writing (2026-02-27), not a tracked-files-only list.
> It includes tracked files plus gitignored/local artifacts; classification sections below define disposition.

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
- `sentry.client.config.ts`, `sentry.edge.config.ts`, `sentry.server.config.ts`

Disposition: **keep** (canonical runtime/observability layout).

### B) Build/test/tooling config and tracked templates (keep, rationalize later)
- `package.json`, `pnpm-lock.yaml`, `tsconfig.json`, `vitest.config.ts`, `vitest.setup.ts`
- `next.config.ts`, `playwright.config.ts`, `postcss.config.mjs`, `prettier.config.mjs`, `eslint.config.mjs`
- `components.json`, `vercel.json`, `.npmrc`, `.mcp.json`
- `.gitignore`, `.prettierignore`, `.prettierrc.json`
- `.gitleaks.toml`, `.trivyignore`, `.codecov.yml`, `.size-limit.json`
- `.env.example` (tracked environment template)
- `lefthook.yml` + `.lefthook.yml` (divergent active config surfaces; merge required, not blind deletion)
- `.lighthouserc.js` + `lighthouserc.json` (dual config surfaces; choose canonical path in follow-up)
- `prettier.config.mjs` + `.prettierrc.json` (possible dual Prettier config surface; verify precedence and reduce)

Disposition: **keep for now**, then merge/rationalize duplicates explicitly.

### C) Governance/context docs (root-anchored vs movable)
- `README.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `vision.md`

Disposition:
- `README.md`: **keep at root**.
- `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`: **keep at root (tool discovery anchors)**.
- `vision.md`: candidate move under `docs/` in a later slice if references are updated.

### D) Repository/meta directories (keep)
- `.git/`, `.github/`, `.changeset/`, `docs/`, `evals/`, `experiments/`, `.claude/`, `.pi/`

Disposition: **keep**.

Notes:
- `.pi/` is repo-local Pi foundation config/artifacts.
- `.pi/state/*` is local runtime state and should remain ignored/untracked.

### E1) Generated/build outputs and local artifacts (keep ignored)
- `.next/`, `node_modules/`, `coverage/`, `test-results/`, `playwright-report/`, `.playwright-mcp/`, `.vercel/`
- `tsconfig.tsbuildinfo`, `next-env.d.ts`, `eval_results.json`, `thinktank.log`

Disposition: **generated/local artifacts**. Keep ignored and out of tracked history.

### E2) Local env variants (verify tracked status individually)
- `.env.local`, `.env.preview`, `.env.production`, `.env.test.local`, `.env.vercel*`, `.env.sentry-build-plugin`

Disposition: **treat as local env variants unless explicitly tracked by policy**.

Verification note (current state):
- `git ls-files | rg '^\.env'` returns only `.env.example`.

## High-noise cleanup targets (priority order)

1. **Divergent duplicate config surfaces**
   - `lefthook.yml` vs `.lefthook.yml` (**merge active hooks, then converge to one canonical file**)
   - `.lighthouserc.js` vs `lighthouserc.json`
   - `prettier.config.mjs` vs `.prettierrc.json`
2. **Root context/doc sprawl (non-tool-anchored files)**
   - `vision.md` (candidate relocation)
3. **Persistent generated artifact drift risk**
   - ensure `eval_results.json`, `thinktank.log`, and test/build outputs remain untracked.

## Proposed next slices

### Slice 2 (issue #271 follow-up)
- Resolve divergent config surfaces (`lefthook`, `lighthouse`, `prettier`) with explicit precedence and one canonical source per concern.
- Update docs/scripts to reference canonical paths only.

### Slice 3 (issue #271 follow-up)
- Move non-root-critical docs (starting with `vision.md`) under `docs/`.
- Keep tool-discovery anchors (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`) at root.

### Slice 4 (issue #271 follow-up)
- Add automated root hygiene check (script/CI guard) for unexpected tracked artifacts and duplicate config surfaces.

## Verification for this slice

Commands used for baseline verification:

```bash
git status --short
git ls-files | rg '^\.env'
```

Optional CI-parity spot checks before follow-up cleanup slices:

```bash
pnpm tsc --noEmit
pnpm test:ci
```
