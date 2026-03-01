# Lefthook Audit (Slice 2 Recon)

## Precedence Finding
**`lefthook.yml` wins.**
Running `pnpm exec lefthook dump` confirms that only hooks from `lefthook.yml` are loaded. The hidden file `.lefthook.yml` is silently ignored when the non-hidden variant exists.

## Hook Coverage Gap

| Hook | `lefthook.yml` (ACTIVE) | `.lefthook.yml` (INACTIVE) | Impact |
| :--- | :---: | :---: | :--- |
| **Secret scanning** (`gitleaks`, `git-secrets`) | ✗ | ✓ | **CRITICAL**: No local secret leak prevention. |
| **Linting** (`lint-staged` vs `eslint`) | `lint-staged` | `eslint --fix` | Medium: Overlap, but `lint-staged` is preferred. |
| **Formatting** (`prettier`) | (via `lint-staged`?) | `prettier --write` | Medium: Redundant if in `lint-staged`. |
| **Type Checking** | `tsc --noEmit` | `tsc --noEmit --incremental` | Low: `lefthook.yml` is safer/cleaner. |
| **Convex Contract Check** | ✗ | ✓ | **High**: Contract drift not caught on push. |
| **Unit Tests** | `test-unit-changed` | `pnpm test --changed` | Medium: Redundant. |

## Recommendation for Slice 2
1. **Merge coverage**: Move `gitleaks`, `git-secrets`, and `convex-check` from `.lefthook.yml` into `lefthook.yml`.
2. **Decommission hidden file**: Delete `.lefthook.yml` once merged.
3. **Ignore local overrides**: Add `lefthook-local.yml` to `.gitignore` (standard Lefthook pattern for per-dev overrides).
4. **Verification**: Run `pnpm exec lefthook dump` after merge to ensure the union set is active.

## Factual Verification
```bash
$ pnpm exec lefthook dump
# Output matched lefthook.yml exactly.
# git-secrets/gitleaks were absent from the dump.
```
