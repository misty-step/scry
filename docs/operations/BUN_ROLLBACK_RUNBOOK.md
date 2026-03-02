# Bun Rollback Runbook

> Rollback procedure for reverting from Bun to pnpm if issues arise during migration.

## Related Issues

- Parent: #270 (Bun migration program)
- This runbook: #273 (CI/ops cutover and rollback plan)

---

## Rollback Triggers

**Immediate rollback required** if any of the following occur:

1. **CI parity breaks**: Required checks (lint, typecheck, test) fail without quick fix (< 1 hour)
2. **Dependency drift**: Bun lockfile causes unresolved dependency issues or security vulnerabilities
3. **Local workflow non-determinism**: Commands behave differently across clean developer environments
4. **Deployment failure**: Production deploy fails due to package manager-related issues
5. **Performance regression**: Install/build times degrade significantly (> 50% slower)

---

## Pre-Rollback Checklist

Before executing rollback:

- [ ] Confirm issue is package-manager related (not code bug)
- [ ] Document specific error/failure for post-mortem
- [ ] Notify team in #dev channel
- [ ] Pause merges to main

---

## Rollback Procedure (5 minutes)

### Step 1: Revert CI to pnpm

```bash
# Remove Bun CI workflow
git rm .github/workflows/ci-bun.yml

# Restore pnpm as primary CI (already in ci.yml)
# No changes needed - ci.yml remains unchanged
```

### Step 2: Update package.json

```bash
# Revert packageManager field
git checkout HEAD -- package.json

# Or manually edit:
# "packageManager": "pnpm@10.12.1"
```

### Step 3: Remove bun.lock (if exists)

```bash
# Remove Bun lockfile if it was committed
rm -f bun.lock
rm -f bun.lockb
git add -A
```

### Step 4: Restore pnpm lockfile

```bash
# Ensure pnpm-lock.yaml is intact
git checkout HEAD -- pnpm-lock.yaml
```

### Step 5: Commit and push

```bash
git commit -m "ops: rollback to pnpm from Bun (#270)

Rollback triggers:
- [Document specific issue]

Restored:
- pnpm as package manager
- pnpm-lock.yaml as lockfile
- CI using pnpm

Next steps:
- [ ] Root cause analysis
- [ ] Document learnings
- [ ] Re-attempt when blockers resolved"

git push origin HEAD
```

---

## Post-Rollback Verification

Within 10 minutes of rollback:

1. **CI status**: Verify pnpm CI passes on the rollback PR
2. **Local sanity check**:
   ```bash
   pnpm install --frozen-lockfile
   pnpm lint
   pnpm tsc --noEmit
   pnpm test:ci
   ```
3. **Deploy check**: Verify production deploy works (if rollback was mid-deploy)

---

## Team Communication Template

```markdown
🚨 Bun Migration Rollback

We've rolled back from Bun to pnpm due to [issue].

**Impact**: [local dev / CI / production]
**Root cause**: [under investigation / documented in #XXX]

**Current state**: pnpm restored, all workflows operational
**Next steps**:
- Post-mortem scheduled for [time]
- Will re-attempt Bun migration when [blocker] is resolved

See runbook: docs/operations/BUN_ROLLBACK_RUNBOOK.md
```

---

## Re-Attempt Criteria

Do NOT re-attempt Bun migration until:

1. Root cause of previous failure is identified and fixed
2. Fix is validated in a feature branch for 72+ hours
3. Team consensus on re-attempt timing
4. Rollback runbook updated with learnings

---

## Historical Rollbacks

| Date | Issue | Resolution | Re-attempted |
|------|-------|------------|--------------|
| - | - | - | - |

*Record any rollbacks here for institutional memory.*
