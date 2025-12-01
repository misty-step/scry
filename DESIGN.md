# DESIGN.md - Free Coverage System Architecture

## Architecture Overview

**Selected Approach**: Native Vitest Thresholds + davelosert/vitest-coverage-report-action

**Rationale**: Zero external dependencies (Codecov is broken and creates vendor lock-in), native Vitest thresholds provide battle-tested enforcement, and the GitHub Action provides rich PR comments showing only changed files. This is the simplest possible architecture that meets all requirements.

**Core Modules**:
- `vitest.config.ts`: Coverage thresholds as enforcement mechanism (blocks CI on regression)
- `.github/workflows/ci.yml`: Coverage generation + PR comment action
- `README.md`: Self-hosted badge via shields.io dynamic endpoint

**Data Flow**:
```
PR opened → CI runs tests → Vitest generates coverage-summary.json
         → vitest-coverage-report-action reads JSON → Posts/updates PR comment
         → If thresholds violated → CI fails → PR blocked
         → On merge to main → Badge auto-updates (reads coverage-summary.json)
```

**Key Design Decisions**:
1. **Thresholds are the enforcement, comments are visibility**: Vitest thresholds exit non-zero on failure; that's what blocks PRs
2. **file-coverage-mode: changes**: Only show coverage for files touched in PR (reduces noise)
3. **Ratchet to current level**: Lock in current coverage as floor, never decrease
4. **Self-hosted badge**: shields.io endpoint reads from coverage-summary.json committed to repo

---

## Module 1: Vitest Coverage Configuration

**Responsibility**: Enforce coverage thresholds and generate coverage artifacts for downstream consumers.

**File**: `vitest.config.ts`

**Current State**:
```typescript
thresholds: {
  lines: 27,      // Outdated - actual is ~39%
  functions: 26,  // Outdated - actual is ~36%
  branches: 22,   // Outdated - actual is ~30%
  statements: 27, // Outdated - actual is ~39%
  ...
}
```

**Target State**:
```typescript
coverage: {
  provider: 'v8',
  reporter: ['text', 'json', 'json-summary', 'html'],  // Remove lcov (was for Codecov)
  reportOnFailure: true,  // CRITICAL: Generate files even when tests fail

  thresholds: {
    // Ratcheted to ~1% below actual (safety margin for floating point)
    lines: 38,
    statements: 38,
    functions: 35,
    branches: 29,
    // Per-path thresholds for critical areas
    'convex/**/*.ts': { lines: 25, functions: 25 },
    'lib/payment/**/*.ts': { lines: 80, functions: 80 },
    'lib/auth/**/*.ts': { lines: 80, functions: 80 },
  },
  // ... existing include/exclude unchanged
}
```

**Implementation Pseudocode**:
```pseudocode
1. Get current coverage levels:
   - Run: pnpm test:ci
   - Read: coverage/coverage-summary.json
   - Extract: total.lines.pct, total.statements.pct, total.functions.pct, total.branches.pct

2. Calculate new thresholds:
   - For each metric:
     - threshold = floor(actual - 1)  # Safety margin for floating point
   - Rationale: ~1% below actual prevents CI flakiness from minor float variations

3. Update vitest.config.ts thresholds:
   - Replace lines/statements/functions/branches values
   - Add reportOnFailure: true
   - Remove 'lcov' from reporters (Codecov-specific)

4. Verify by running tests:
   - pnpm test:ci should pass
   - If fail: threshold too aggressive, reduce by 1%
```

**Error Handling**:
- Threshold violation → Exit code 1 → CI fails → PR blocked (this is intentional)
- No silent failures; explicit enforcement

---

## Module 2: GitHub Actions CI Workflow

**Responsibility**: Run tests, generate coverage, post PR comments, enforce quality gates.

**File**: `.github/workflows/ci.yml`

**Current State Issues**:
1. Codecov action enabled (broken, external dependency)
2. vitest-coverage-report-action commented out
3. Missing `pull-requests: write` permission (required for PR comments)
4. No `if: always()` condition (report doesn't run if tests fail)

**Target State**:
```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: read
      pull-requests: write  # REQUIRED for PR comments
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v2
        with:
          version: ${{ env.PNPM_VERSION }}
      - uses: actions/setup-node@v4
        with:
          node-version: ${{ env.NODE_VERSION }}
          cache: 'pnpm'

      - run: pnpm install --frozen-lockfile

      - name: Run tests with coverage
        run: pnpm test:ci

      - name: Coverage Report
        if: always() && github.event_name == 'pull_request'
        uses: davelosert/vitest-coverage-report-action@v2
        with:
          json-summary-path: ./coverage/coverage-summary.json
          json-final-path: ./coverage/coverage-final.json
          vite-config-path: ./vitest.config.ts
          file-coverage-mode: changes

      # Codecov removed - no external dependencies
```

**Implementation Pseudocode**:
```pseudocode
1. Remove Codecov:
   - Delete: codecov/codecov-action@v4 step
   - Delete: CODECOV_TOKEN reference
   - No need to remove token from secrets (just unused)

2. Enable vitest-coverage-report-action:
   - Uncomment the action block
   - Add file-coverage-mode: changes
   - Ensure json-summary-path and json-final-path match vitest output

3. Add permissions:
   - Add to test job: permissions: { contents: read, pull-requests: write }

4. Add if: always() condition:
   - Ensures coverage report runs even when tests fail
   - Shows what coverage would be even on failing PRs

5. Verify:
   - Push to branch
   - Open PR
   - Confirm comment appears
   - Confirm threshold failure blocks merge
```

**CI Behavior Matrix**:
| Scenario | Threshold | Tests | PR Comment | CI Status |
|----------|-----------|-------|------------|-----------|
| Coverage up, tests pass | OK | PASS | Shows green delta | PASS |
| Coverage down, tests pass | FAIL | PASS | Shows red delta | FAIL |
| Coverage up, tests fail | OK | FAIL | Shows coverage | FAIL |
| Coverage down, tests fail | FAIL | FAIL | Shows coverage | FAIL |

---

## Module 3: README Badge

**Responsibility**: Display current coverage percentage via self-hosted shields.io badge.

**File**: `README.md`

**Current State**:
```markdown
[![codecov](https://codecov.io/gh/phrazzld/scry/graph/badge.svg?flag=project)](https://codecov.io/gh/phrazzld/scry)
[![patch coverage](https://codecov.io/gh/phrazzld/scry/graph/badge.svg?flag=patch)](https://codecov.io/gh/phrazzld/scry)
```

**Target State Options**:

### Option A: Static Badge (Recommended for Simplicity)
```markdown
![Coverage](https://img.shields.io/badge/coverage-39%25-green)
```
- Pros: No infrastructure needed, works immediately
- Cons: Manual updates required (or script on main merge)
- Implementation: Update badge after each main merge

### Option B: Dynamic Badge via shields.io Endpoint
```markdown
![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/phrazzld/scry/main/coverage-badge.json)
```
- Requires: `coverage-badge.json` committed to repo
- Pros: Automatic updates on every main merge
- Cons: Requires additional workflow step

**Selected**: Option B (Dynamic Badge) for zero maintenance after setup.

**Implementation Pseudocode**:
```pseudocode
1. Create badge JSON format:
   coverage-badge.json:
   {
     "schemaVersion": 1,
     "label": "coverage",
     "message": "39%",
     "color": "green"
   }

2. Add workflow step to update badge on main merge:
   - on: push branches: [main]
   - Extract coverage % from coverage-summary.json
   - Generate coverage-badge.json
   - Commit and push to main

3. Update README.md:
   - Replace Codecov badges with shields.io endpoint badge

Color scheme:
   - < 50%: red
   - 50-70%: yellow
   - 70-85%: yellowgreen
   - > 85%: green
```

---

## File Organization

```
.github/
  workflows/
    ci.yml               # Modified: Remove Codecov, enable vitest-coverage-report

vitest.config.ts         # Modified: Ratchet thresholds, add reportOnFailure

README.md                # Modified: Replace Codecov badge with shields.io

coverage/                # Generated (gitignored)
  coverage-summary.json  # Read by vitest-coverage-report-action
  coverage-final.json    # Read by vitest-coverage-report-action

coverage-badge.json      # NEW: Committed, read by shields.io (Option B only)
```

**Files to Modify**:
1. `vitest.config.ts` - Lines 18, 28-36 (thresholds)
2. `.github/workflows/ci.yml` - Lines 48-80 (test job)
3. `README.md` - Lines 3-4 (badges)

**Files to Create** (if Option B):
1. `coverage-badge.json` - Initial badge data
2. `.github/workflows/update-badge.yml` (optional) - Or add to ci.yml

---

## Integration Points

**Vitest Output**:
```json
// coverage/coverage-summary.json (generated by Vitest)
{
  "total": {
    "lines": { "total": 1000, "covered": 390, "pct": 39.0 },
    "statements": { "total": 1000, "covered": 389, "pct": 38.9 },
    "functions": { "total": 100, "covered": 36, "pct": 36.0 },
    "branches": { "total": 200, "covered": 60, "pct": 30.0 }
  }
}
```

**vitest-coverage-report-action Requirements**:
- Reads: `coverage-summary.json` (threshold comparison)
- Reads: `coverage-final.json` (per-file breakdown)
- Reads: `vitest.config.ts` (threshold values for display)
- Requires: `pull-requests: write` permission

**shields.io Endpoint Format**:
```json
// coverage-badge.json
{
  "schemaVersion": 1,
  "label": "coverage",
  "message": "39%",
  "color": "green"
}
```

---

## State Management

**State Sources**:
1. **Coverage thresholds**: Stored in `vitest.config.ts` (source of truth)
2. **Current coverage**: Generated per-run in `coverage/` (ephemeral)
3. **Badge data**: Stored in `coverage-badge.json` (updated on main merge)

**State Update Flow**:
1. Developer pushes PR
2. CI generates fresh coverage data
3. vitest-coverage-report-action compares to thresholds
4. On merge to main: badge JSON updated from coverage-summary.json
5. shields.io fetches latest badge JSON on next README view

---

## Error Handling Strategy

**Error Categories**:

1. **Threshold Violation** (expected, intentional):
   - Vitest exits non-zero
   - CI fails
   - PR blocked
   - PR comment shows what needs improvement

2. **Coverage File Missing**:
   - vitest-coverage-report-action fails gracefully
   - Error message in CI logs
   - Mitigation: `reportOnFailure: true` ensures files always generated

3. **Permission Error** (PR comment fails):
   - Action logs permission error
   - Coverage still enforced via threshold
   - Fix: Ensure `pull-requests: write` in job permissions

4. **Action Version Breaking Change**:
   - Pin to `@v2` (major version only)
   - Monitor action repository for deprecation notices

---

## Testing Strategy

**Pre-Implementation Tests** (manual verification):
1. Run `pnpm test:ci` locally
2. Verify `coverage/coverage-summary.json` exists
3. Verify `coverage/coverage-final.json` exists
4. Note current coverage percentages

**Post-Implementation Tests**:
1. Push to test branch
2. Open PR to main
3. Verify:
   - [ ] PR comment appears with coverage summary
   - [ ] Only changed files shown in report
   - [ ] Overall coverage delta displayed
   - [ ] Thresholds shown from vitest.config.ts

**Threshold Enforcement Test**:
1. Create PR that intentionally drops coverage
2. Verify CI fails
3. Verify failure message mentions threshold

**Badge Test** (if Option B):
1. Merge PR to main
2. Verify coverage-badge.json updated
3. Verify README badge shows new percentage

---

## Performance Considerations

**Expected Impact**:
- Coverage generation: Already running (~30s of current test time)
- vitest-coverage-report-action: <10s additional (reads JSON, posts comment)
- Total CI time impact: <30s added to test job

**Optimization**:
- `file-coverage-mode: changes` reduces comment size (only changed files)
- No additional test runs required (action reads existing coverage data)

---

## Security Considerations

**Threats Mitigated**:
- **External service compromise**: Eliminated (no Codecov/Coveralls)
- **Token exposure**: No secrets required for coverage
- **Supply chain**: Only dependency is vitest-coverage-report-action (well-maintained, OSS)

**Best Practices**:
- Pin action to major version (`@v2`) for security updates
- No secrets stored for coverage system
- PR comments contain no sensitive data (only coverage %)

---

## Alternative Architectures Considered

### Alternative A: Continue with Codecov (Fix It)
- **Pros**: Rich UI, historical trends, badge infrastructure
- **Cons**: External dependency (already broken), pricing changes, single point of failure
- **Verdict**: Rejected - PRD explicitly requires zero external dependencies

### Alternative B: Self-Hosted SonarQube
- **Pros**: Comprehensive code quality, historical data
- **Cons**: Requires server, overkill for solo project, paid for private repos
- **Verdict**: Rejected - violates cost requirement ($0)

### Alternative C: Manual Coverage Checks
- **Pros**: No automation complexity
- **Cons**: Human error, forgotten checks, no PR feedback
- **Verdict**: Rejected - defeats purpose of automated quality gates

### Alternative D: GitHub Native Code Scanning
- **Pros**: Deep GitHub integration
- **Cons**: Doesn't show coverage, different purpose (security, not quality)
- **Verdict**: Not applicable - different problem space

**Selected**: Native Vitest + vitest-coverage-report-action
- **Justification**: Simplest solution meeting all requirements
- **Module Depth**: Deep - simple interface (threshold config), complex implementation hidden (coverage calculation, comment generation)
- **Zero External Dependencies**: GitHub Actions are already in use

---

## Implementation Sequence

**Atomic Changes** (each independently mergeable):

### Step 1: Ratchet Vitest Thresholds
1. Run tests to get current coverage
2. Update `vitest.config.ts` thresholds
3. Add `reportOnFailure: true`
4. Remove `lcov` reporter
5. Verify tests pass

### Step 2: Update CI Workflow
1. Remove Codecov action
2. Uncomment vitest-coverage-report-action
3. Add `file-coverage-mode: changes`
4. Add `pull-requests: write` permission
5. Add `if: always()` condition

### Step 3: Update README Badge
1. Remove Codecov badges
2. Add shields.io static badge (immediate)
3. (Optional) Add dynamic badge workflow

### Step 4: Verify End-to-End
1. Push changes
2. Open test PR
3. Verify all success criteria met

---

## Success Criteria Checklist

From TASK.md:
- [ ] PR comments show coverage delta
- [ ] PRs blocked on coverage regression
- [ ] README badge shows real coverage
- [ ] Zero external service dependencies
- [ ] CI completes in <5 minutes total

---

## Next Steps

Run `/plan` to convert this architecture into atomic implementation tasks.
