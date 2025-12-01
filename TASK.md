# 70%+ Coverage Sprint

## Executive Summary

Reach 70%+ test coverage in a dedicated sprint, then lock it in with hard thresholds and dynamic PR coverage reporting. Replace broken Codecov with fully self-hosted solution.

**User Value**: Confidence that every PR maintains quality. Regressions caught before merge. No external dependencies.

**Success Criteria**:
- Coverage ≥70% lines/statements/functions
- PRs blocked on coverage regression
- PR comments show coverage delta
- README badge shows real coverage
- Zero external service dependencies

## Current State

- **Coverage**: ~39% (target: 70%+)
- **Codecov**: Broken, showing "unknown" badge
- **Thresholds**: Set too low (27% lines)
- **Gap**: ~31 percentage points requiring dedicated test sprint

## Requirements

### Functional Requirements

1. **70%+ Coverage**: Reach and enforce 70%+ line/statement/function coverage
2. **PR Coverage Comments**: Every PR gets markdown comment showing coverage delta
3. **Threshold Enforcement**: PRs blocked when overall coverage drops below 70%
4. **README Badge**: Self-hosted SVG badge showing current coverage percentage

### Non-Functional Requirements

- **Cost**: $0 - no paid services
- **Reliability**: No external service dependencies
- **Performance**: Coverage check adds <2 min to CI
- **Maintainability**: Simple, auditable GitHub Actions workflow

## Test Sprint Priority Files

### Tier 1: CRITICAL (Must Test First)

| # | File | Lines | Rationale |
|---|------|-------|-----------|
| 1 | `convex/fsrs/engine.ts` | 146 | Pure FSRS algorithm - core memory science |
| 2 | `convex/spacedRepetition.ts` | 275 | Queue prioritization - user experience |
| 3 | `convex/rateLimit.ts` | 319 | Rate limiting - security + monetization |
| 4 | `lib/ai-client.ts` | 464 | AI client + fallback logic |
| 5 | `convex/aiGeneration.ts` | 984 | AI content generation - quality + cost |
| 6 | `convex/generationJobs.ts` | 363 | Job queue state machine |

### Tier 2: HIGH-VALUE (Secondary)

| # | File | Lines | Rationale |
|---|------|-------|-----------|
| 7 | `convex/concepts.ts` | 1,247 | Core concept CRUD |
| 8 | `convex/embeddings.ts` | 1,048 | Vector embedding lifecycle |
| 9 | `convex/userStats.ts` | 341 | Analytics computation |
| 10 | `convex/questionsCrud.ts` | 366 | Question lifecycle |

**Estimated effort**: 12-16 hours focused test writing

## Architecture Decision

### Selected Approach: Native Vitest + davelosert/vitest-coverage-report-action

**Why this approach**:
- **Free forever**: GitHub Actions minutes only (already using)
- **No external dependencies**: No Codecov/Coveralls that can break or change pricing
- **Best PR experience**: `file-coverage-mode: changes` shows only changed files
- **Native thresholds**: Vitest's built-in threshold enforcement is battle-tested
- **Self-hosted badge**: shields.io endpoint reads coverage percentage

### Alternatives Considered

| Approach | Why Not |
|----------|---------|
| **Codecov** | Already broken, external dependency, pricing changes |
| **Coveralls** | Same external dependency risk |
| **SonarQube** | Overkill, requires server, paid for private repos |
| **Gradual ratchet** | User wants hard 70% target now |

## Implementation Phases

### Phase 1: Test Sprint (12-16 hours)

Write tests for Tier 1+2 files using established patterns:
- Factory pattern via `/tests/helpers/convexFixtures.ts`
- Database mocking via `createMockDb()`, `createMockCtx()`
- Logger stub via `/tests/helpers/loggerStub.ts`
- Deterministic timestamps via `fixedNow`

### Phase 2: Infrastructure (30 min)

1. **Update vitest.config.ts**
   - Set thresholds: 70% lines/statements/functions, 60% branches
   - Add `reportOnFailure: true`
   - Remove `lcov` reporter (Codecov-specific)

2. **Update .github/workflows/ci.yml**
   - Remove Codecov action
   - Enable vitest-coverage-report-action
   - Add `pull-requests: write` permission
   - Add `file-coverage-mode: changes`

3. **Update README.md**
   - Replace Codecov badges with shields.io static badge

### Phase 3: Documentation (15 min)

- Update BACKLOG.md (remove obsolete items, add sprint item)
- Update TODO.md with test file checklist

## Test Patterns to Use

```typescript
// Factory pattern
import { makeQuestion, makeConcept } from '@/tests/helpers/convexFixtures';

// Database mocking
import { createMockDb, createMockCtx } from '@/tests/helpers/convexFixtures';

// Deterministic timestamps
const fixedNow = new Date('2025-01-16T12:00:00Z').getTime();

// Logger stub
import { createLoggerStub } from '@/tests/helpers/loggerStub';
```

## Critical CI Configuration

```yaml
test:
  runs-on: ubuntu-latest
  timeout-minutes: 10
  permissions:
    contents: read
    pull-requests: write  # REQUIRED for PR comments
  steps:
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
```

## Success Metrics

- [ ] `pnpm test:coverage` shows ≥70% lines/statements/functions
- [ ] CI enforces 70% threshold (PRs blocked if below)
- [ ] PRs get coverage comments automatically
- [ ] README shows accurate coverage percentage
- [ ] No external service dependencies
- [ ] CI completes in <5 minutes total

## Risks & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| 70% not reachable with Tier 1+2 | Medium | Low | Add Tier 3 files or adjust target |
| Test sprint takes longer | Medium | Medium | Focus on critical paths first |
| vitest-coverage-report-action breaks | Low | Medium | Pin version, action is stable |

## Files to Modify

1. `vitest.config.ts` - Set 70% thresholds, add reportOnFailure
2. `.github/workflows/ci.yml` - Remove Codecov, enable vitest-coverage-report-action
3. `README.md` - Replace Codecov badges with shields.io
4. 10 test files to create for priority modules
