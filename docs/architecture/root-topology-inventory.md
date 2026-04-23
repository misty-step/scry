# Root Topology Inventory (Slice 1)

Issue: #271 (parent #270)
Date: 2026-04-23

## Objective

Create a full root-level inventory and classify each item so follow-up cleanup slices can move files safely with minimal CI/script breakage.

## Current root snapshot

> This is a filesystem snapshot at time of writing (2026-04-23), not a tracked-files-only list.
> It includes tracked files plus gitignored/local artifacts; classification sections below define disposition.

```text
.agent
.changeset
.claude
.codecov.yml
.codex
.env.example
.git
.github
.gitignore
.gitleaks.toml
.groom
.lighthouserc.js
.mcp.json
.npmrc
.pi
.prettierignore
.prettierrc.json
.size-limit.json
.spellbook
.trivyignore
AGENTS.md
app
backlog.d
bun.lock
CLAUDE.md
components
components.json
convex
docs
eslint.config.mjs
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
next.config.ts
node_modules
package.json
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
tests
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
- `package.json`, `pnpm-lock.yaml`, `bun.lock`, `tsconfig.json`, `vitest.config.ts`, `vitest.setup.ts`
- `next.config.ts`, `playwright.config.ts`, `postcss.config.mjs`, `prettier.config.mjs`, `eslint.config.mjs`
- `components.json`, `vercel.json`, `.npmrc`, `.mcp.json` (repo-shared MCP tooling config)
- `.gitignore`, `.prettierignore`, `.prettierrc.json`
- `.gitleaks.toml`, `.trivyignore`, `.codecov.yml`, `.size-limit.json`
- `.groom/` (code review quality scores, tracked for trend analysis)
- `.env.example` (tracked environment template)
- `lefthook.yml` (unified config after merging `.lefthook.yml` coverage in Slice 1)
- `.lighthouserc.js` + `lighthouserc.json` (dual config surfaces; `lighthouserc.json` is currently canonical for CI)
- `prettier.config.mjs` + `.prettierrc.json` (confirmed dual Prettier config surface; `.prettierrc.json` is currently active)

Disposition: **keep for now**, then merge/rationalize duplicates explicitly.

### C) Governance/context docs (root-anchored vs movable)
- `README.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `vision.md`

Disposition:
- `README.md`: **keep at root**.
- `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`: **keep at root (repo workflow contract + root-linked references)**.
- `vision.md`: candidate move under `docs/` in a later slice if references are updated.

Note: root retention for these instruction files is a repository policy/convention decision, not an MCP filename auto-discovery claim.

### D) Repository/meta directories (keep)
- `.git/`, `.github/`, `.changeset/`, `docs/`, `evals/`, `experiments/`, `.claude/`, `.pi/`, `.agent/`, `.codex/`, `.spellbook/`, `backlog.d/`

Disposition: **keep**.

Notes:
- `.pi/` is repo-local Pi foundation config/artifacts (12 files tracked, including bootstrap reports).
- `.pi/state/*` is local runtime state and should remain ignored/untracked.
- `.agent/skills/` is the shared Spellbook skill layer. `.claude/skills/`, `.codex/skills/`, and `.pi/skills/` are symlink bridges to it.
- `.spellbook/repo-brief.md` is the tailored harness brief used by skills and agents.

### E1) Generated/build outputs and local artifacts (keep ignored)
- `.next/`, `node_modules/`, `coverage/`, `test-results/`, `playwright-report/`, `.playwright-mcp/`, `.vercel/`
- `tsconfig.tsbuildinfo`, `next-env.d.ts`, `eval_results.json`, `thinktank.log` (previously tracked in history, now ignored)

Disposition: **generated/local artifacts**. Keep ignored and out of tracked history.

Tracked artifact exception resolved:
- `.playwright-mcp/signin-email-sent-state.png` and `.playwright-mcp/signin-page-redesigned.png` were removed from git index and remain local-only under the ignored `.playwright-mcp/` directory.

### E2) Local env variants (verify tracked status individually)
- `.env.local`, `.env.preview`, `.env.production`, `.env.test.local`, `.env.vercel*`, `.env.sentry-build-plugin`

Disposition: **treat as local env variants unless explicitly tracked by policy**.
Note: `.env.production` is currently required by `docs/runbooks/production-deployment.md` for manual deployments.

Verification note (current state):
- `git ls-files | rg '^\.env'` returns only `.env.example`.
- `git ls-files .playwright-mcp` returns no tracked files.
- `pnpm exec prettier --find-config-path docs/architecture/root-topology-inventory.md` resolves to `.prettierrc.json`.

## High-noise cleanup targets (priority order)

1. **Divergent duplicate config surfaces**
   - `lefthook.yml` (**unified config after merge**)
   - `.lighthouserc.js` vs `lighthouserc.json`
   - `prettier.config.mjs` vs `.prettierrc.json` (`.prettierrc.json` currently active)
2. **Root context/doc sprawl (non-root-critical files)**
   - `vision.md` (candidate relocation)
3. **Persistent generated artifact drift risk**
   - ensure `eval_results.json`, `thinktank.log`, and test/build outputs remain untracked.

## Proposed next slices

### Slice 2 (issue #271 follow-up)
- Resolve divergent config surfaces (`lighthouse`, `prettier`) with explicit precedence and one canonical source per concern.
- Canonicalize to `lighthouserc.json` for Lighthouse (align with CI usage) and merge `.lighthouserc.js` assertions.
- Update docs/scripts to reference canonical paths only.

Exit criteria:
- canonical config decision documented for each duplicate surface
- hook/tooling docs reference only canonical config paths

### Slice 3 (issue #271 follow-up)
- Move non-root-critical docs (starting with `vision.md`) under `docs/`.
- Keep root workflow anchors (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`) at root unless repository policy is intentionally changed.

Exit criteria:
- moved docs have updated links/references
- root anchor files unchanged unless an explicit policy-change PR is approved

### Slice 4 (issue #271 follow-up)
- Add automated root hygiene check (script/CI guard) for unexpected tracked artifacts and duplicate config surfaces.

Exit criteria:
- hygiene check runs in CI
- check fails on newly tracked generated artifacts or duplicate canonical config files

## Verification for this slice

Commands used for baseline verification:

```bash
git status --short
git ls-files | rg '^\.env'          # or: git ls-files | grep -E '^\.env'
git ls-files .playwright-mcp
pnpm exec prettier --find-config-path docs/architecture/root-topology-inventory.md
```

Optional CI-parity spot checks before follow-up cleanup slices:

```bash
pnpm tsc --noEmit
pnpm test:ci
```
