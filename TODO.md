# TODO: 70%+ Coverage Sprint

## Context
- **Current**: ~39% coverage
- **Target**: 70%+ with hard threshold enforcement
- **Approach**: Dedicated test sprint → Set 70% threshold → Enable PR comments → Update badges
- **Patterns**: Use `/tests/helpers/convexFixtures.ts`, `createMockDb()`, `createMockCtx()`

## Test Sprint Tasks

- [x] Test `convex/fsrs/engine.ts` (146 lines)
  ```
  Pure FSRS scheduling algorithm - core memory science
  Test: scheduling calculations, state transitions, edge cases
  Pattern: Unit tests with deterministic timestamps
  ```

- [x] Test `convex/spacedRepetition.ts` (275 lines)
  ```
  Queue prioritization logic - directly affects user experience
  Test: due card ordering, new vs review prioritization, pagination
  Pattern: Mock database with pre-built fixtures
  ```

- [x] Test `convex/rateLimit.ts` (319 lines)
  ```
  Rate limiting for auth + API - security + monetization
  Test: limit enforcement, window resets, per-user tracking
  Pattern: Mock context with time manipulation
  ```

- [x] Test `lib/ai-client.ts` (464 lines)
  ```
  AI API client with OpenAI→Gemini fallback
  Test: provider switching, error handling, retries
  Pattern: Mock AI SDK responses
  ```

- [x] Test `convex/aiGeneration.ts` (984 lines)
  ```
  AI content generation - quality + cost critical
  Test: prompt construction, response parsing, error classification
  Pattern: Mock AI provider, test state machine
  ```

- [x] Test `convex/generationJobs.ts` (363 lines)
  ```
  Job queue state machine - async reliability
  Test: job lifecycle, status transitions, cancellation
  Pattern: createMockDb with job fixtures
  ```

### Tier 2: High-Value Modules (Secondary)

- [x] Test `convex/concepts.ts` (1,247 lines)
  ```
  Core concept CRUD - primary data model
  Test: CRUD operations, pagination, filtering
  Pattern: Mock database, comprehensive fixtures
  ```

- [x] Test `convex/embeddings.ts` (1,048 lines)
  ```
  Vector embedding lifecycle
  Test: embedding generation, search, batch operations
  Pattern: Mock AI provider for embeddings
  ```

- [x] Test `convex/userStats.ts` (341 lines)
  ```
  Analytics computation
  Test: stat calculations, aggregations, edge cases
  Pattern: Unit tests with sample data
  ```

- [x] Test `convex/questionsCrud.ts` (366 lines)
  ```
  Question lifecycle (create/update/delete)
  Test: CRUD operations, validation, permissions
  Pattern: Mock database with user context
  ```

## Infrastructure Tasks

- [x] Update `vitest.config.ts` with 70% thresholds
  ```
  Files: vitest.config.ts (lines 28-36)
  Changes:
  - lines: 70, statements: 70, functions: 70, branches: 60
  - Add reportOnFailure: true
  - Remove 'lcov' from reporters
  ```

- [x] Update `ci.yml` with vitest-coverage-report-action
  ```
  Files: .github/workflows/ci.yml (lines 48-80)
  Changes:
  - Add permissions: { contents: read, pull-requests: write }
  - Remove Codecov action
  - Uncomment vitest-coverage-report-action with file-coverage-mode: changes
  - Add if: always() && github.event_name == 'pull_request'
  ```

- [x] Replace README badges with shields.io
  ```
  Files: README.md (lines 3-4)
  Changes:
  - Remove Codecov badges
  - Add: ![Coverage](https://img.shields.io/badge/coverage-70%25-brightgreen)
  ```

- [x] Update BACKLOG.md
  ```
  Remove:
  - [TESTING][LOW] Coverage Threshold Optimization (lines 163-169)
  - [TESTING][LOW] Coverage File Generation Verification (lines 196-202)
  Add to "Now":
  - [TESTING][CRITICAL] 70%+ Coverage Sprint (in progress)
  ```

## Progress Tracking

Run after each test file:
```bash
pnpm test:coverage
```

Track coverage increases:
| File Completed | Lines% | Statements% | Functions% | Branches% |
|----------------|--------|-------------|------------|-----------|
| Starting point | 39% | 39% | 36% | 30% |
| After Tier 1 | ? | ? | ? | ? |
| After Tier 2 | ? | ? | ? | ? |

## Success Criteria

- [ ] Coverage ≥70% lines/statements/functions
- [ ] Coverage ≥60% branches
- [ ] PRs blocked on coverage regression
- [ ] PR comments show coverage delta
- [ ] README badge shows real coverage
- [ ] Zero external service dependencies
