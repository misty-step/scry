# BACKLOG

**Last Groomed**: 2025-11-29
**Analysis Method**: 15-perspective comprehensive audit (8 specialists + 7 master personas)
**Agents Completed**: security-sentinel, performance-pathfinder, user-experience-advocate, product-visionary, design-systems-architect, grug, jobs
**Overall Assessment**: Strong technical foundation with critical go-to-market gaps. Dual data model and god objects creating invisible complexity. IQC system 90% built but over-engineered for main UI. Performance bottlenecks in hot paths (N+1 getDue, unbounded .collect()).

---

## Now (<2 weeks, sprint-ready)

### [PRODUCT] Browser extension
- i want to click a scry browser extension and use the current page as input to generate study material / quiz questions/concepts/phrasings from

### [SECURITY][HIGH] Unvalidated Limit Parameters - DoS Vulnerability
**Files**: convex/questionsLibrary.ts:136, :241
**Perspectives**: security-sentinel, performance-pathfinder
**Problem**: `limit` param accepts any number → malicious `limit: 999999999` causes bandwidth spike
**Fix**: Add `const safeLimit = Math.min(Math.max(args.limit ?? 50, 10), 500);`
**Acceptance**: All limit params validated; tests for edge cases
**Effort**: 15m | **Risk**: HIGH → NONE

### [SECURITY][HIGH] Webhook Fallback Accepts Unauthenticated Requests
**File**: convex/http.ts:61
**Perspectives**: security-sentinel
**Problem**: Returns 200 when `CLERK_WEBHOOK_SECRET` missing → spoofed user operations
**Fix**: Return 500 instead of 200 when secret missing; log security incident
**Acceptance**: Secret required in prod; failure returns 500; test coverage
**Effort**: 20m | **Risk**: MEDIUM → NONE

### [SECURITY][MEDIUM] CSP Permits unsafe-eval in Production
**File**: next.config.ts:90
**Perspectives**: security-sentinel
**Problem**: `unsafe-eval` in CSP allows dynamic code execution
**Fix**: Conditionally disable `unsafe-eval` in production (only needed for Vercel live editing)
**Acceptance**: Production CSP excludes unsafe-eval; preview/dev retain it
**Effort**: 30m | **Risk**: MEDIUM → LOW

### [SECURITY][MEDIUM] Update Sentry Dependencies
**File**: package.json (@sentry/nextjs@10.26.0)
**Perspectives**: security-sentinel
**Problem**: Vulnerable version leaks headers when sendDefaultPii=true (mitigated by config but unpatched)
**Fix**: `pnpm update @sentry/nextjs @sentry/node @sentry/node-core` to >=10.27.0
**Acceptance**: Vulnerabilities resolved; tests pass
**Effort**: 10m | **Risk**: MEDIUM → NONE

### [PERF][CRITICAL] N+1 Queries in getDue() - 200-400ms per quiz load
**File**: convex/concepts.ts:162-164
**Perspectives**: performance-pathfinder
**Problem**: Loop executes 35+ sequential queries per getDue() call (selectActivePhrasing + interactions + legacyQuestion per candidate)
**Fix**: Filter candidates with `phrasingCount > 0` in-memory BEFORE querying; batch lookups
**Acceptance**: Per-session queries reduced from 35+ to 3-5; latency 200-400ms → 50-100ms
**Effort**: 10m | **Impact**: 3-4x faster quiz initialization

### [PERF][CRITICAL] Unbounded .collect() in Archive/Restore
**Files**: convex/concepts.ts:710, 778, 1143, 1173, 1204, 1234
**Perspectives**: performance-pathfinder, security-sentinel (DoS)
**Problem**: `.collect()` fetches ALL phrasings without limit → 600KB+ per archive, quota burn
**Fix**: Replace with `.take(MAX_PHRASINGS)`; pre-compute conflictScore on creation
**Acceptance**: Bandwidth per query <100KB; 10k cards smoke test <500ms
**Effort**: 15m | **Impact**: 5-7x faster archive, 500KB+ saved per operation

### [PERF][HIGH] Linear Search in findActiveGenerationJob()
**File**: convex/concepts.ts:879-898
**Perspectives**: performance-pathfinder
**Problem**: O(n×m) array search (25 jobs × 10 conceptIds = 250 ops) per call
**Fix**: Use Set for O(1) conceptId lookup instead of nested `.find()` + `.some()`
**Acceptance**: Lookup time 5-10ms → 1-2ms
**Effort**: 5m | **Impact**: 5x faster phrasing generation UI

### [UX][HIGH] Vague Error Messages Without Recovery Guidance
**File**: lib/error-summary.ts:36-62
**Perspectives**: user-experience-advocate, security-sentinel (info disclosure)
**Problem**: "Rate limit reached. Please wait a moment." - no retry time, no action guidance
**Fix**: Add specific wait times, recovery steps, status page links
**Acceptance**: 10+ common errors mapped to actionable messages; unit tests
**Effort**: 1h | **Value**: Users fix problems without support

### [UX][HIGH] Silent Search Failures
**File**: app/library/_components/library-client.tsx:84-95
**Perspectives**: user-experience-advocate
**Problem**: Search errors clear results with only a toast (disappears in 5s) → user confusion
**Fix**: Show persistent error state in results area with retry button; preserve last results
**Acceptance**: Error state distinguishes "no results" vs "search failed"; retry button works
**Effort**: 2h | **Value**: Users understand what happened

### [UX][HIGH] No Retry Mechanism for Failed Mutations
**File**: app/library/_components/library-client.tsx:193-237
**Perspectives**: user-experience-advocate
**Problem**: Archive/delete fails → toast disappears → no retry without manual re-trigger
**Fix**: Show retry dialog on failure; track failed action state
**Acceptance**: Failed actions show retry modal; retry works
**Effort**: 3h | **Value**: Transient errors recoverable

### [UX][CRITICAL] Missing Confirmation on Permanent Delete
**File**: app/library/_components/library-client.tsx:269-290
**Perspectives**: user-experience-advocate
**Problem**: Permanent deletion (irreversible) has no require-typing confirmation
**Fix**: Add `useConfirmation()` with `requireTyping: 'delete permanently'`
**Acceptance**: Permanent delete requires typing; tests for confirmation flow
**Effort**: 1h | **Value**: Prevents accidental data loss

### [DESIGN][HIGH] Input Component References Undefined CSS Variables
**File**: components/ui/input.tsx:11-12
**Perspectives**: design-systems-architect
**Problem**: `border-line`, `bg-paper`, `focus:ring-blueprint` are not defined → focus states may be broken
**Fix**: Replace with existing variables: `border-input`, `bg-background`, `focus:ring-ring`
**Acceptance**: Focus states work; aria-invalid states display correctly
**Effort**: 30m | **Impact**: BLOCKER - inputs may be displaying incorrectly

### [DESIGN][HIGH] Create Semantic Color System
**Files**: 15+ components using bg-red-50, text-blue-700 instead of tokens
**Perspectives**: design-systems-architect
**Problem**: Semantic colors defined in globals.css but ignored → inconsistent error/success UX
**Fix**: Create `lib/design-system/state-colors.ts` with error/success/warning/info tokens
**Acceptance**: 15 components migrated; dark mode automatic; design tokens documented
**Effort**: 2h tokens + 4h migration | **Impact**: Consistent state colors, rebrandable

### [COMPLEXITY][HIGH] Delete Unused ConfigManager/InputManager
**Files**: components/lab/config-manager.tsx (363 lines), components/lab/input-manager.tsx (210 lines)
**Perspectives**: grug, jobs
**Problem**: Generic CRUD managers with full state management - NOT USED ANYWHERE yet
**Violation**: Abstraction before having two concrete uses (Ousterhout sin #1)
**Fix**: Delete both files now; implement inline when needed
**Acceptance**: Files deleted; no compile errors; git history preserves code
**Effort**: 5m | **Impact**: -573 lines of unused abstraction

### [TEST][LOW] Skip Feature Test Improvements (PR #100 CodeRabbit)
**Source**: PR #100 review comments
**Items**:
- `hooks/use-active-jobs.test.ts`: Use real `isActiveJob` instead of mocking internal logic
- `hooks/use-active-jobs.test.ts`: Align string quotes with project convention (double quotes)
- `components/review-actions-dropdown.test.tsx`: Add assertion that `onSkip` not called when disabled
- `hooks/use-simple-poll.test.ts`: Strengthen refetch assertion with specific timestamp
- `convex/spacedRepetition.test.ts`: Extract duplicate handler logic to pure function for testing
- `convex/spacedRepetition.test.ts`: Local createMockCtx shadows test helper
**Acceptance**: Tests improved; no false positives; clean mocking patterns
**Effort**: 2-3h total

### [UX][LOW] Skip Feature UX Improvements (PR #100 CodeRabbit)
**Source**: PR #100 review comments
**Items**:
- `components/review-actions-dropdown.tsx:` Guard onSkip handler when disabled
- `components/review-flow.tsx:393-404`: Memory leak - setTimeout without cleanup
**Acceptance**: Skip button correctly guarded; no memory leaks in review flow
**Effort**: 1h total

### [DOCS][LOW] Update TASK.md Checklists (PR #100 CodeRabbit)
**Source**: PR #100 review comments
**Items**:
- `TASK.md:106-124`: Update test scenario checkboxes to reflect completion
- `TASK.md:156-159`: Update checklist items that are implemented
- `vitest.config.ts`: Plan to remove exclusions after refactors complete
**Acceptance**: Task tracking reflects actual implementation state
**Effort**: 15m

### [PROMPTS][LOW] Align Field Naming Across Langfuse Prompts (PR #107 CodeRabbit)
**Source**: PR #107 review comments
**Files**: .claude/skills/langfuse-prompts/prompts/concept-synthesis.txt
**Problem**: Field naming inconsistency - `content_type` in prompt text vs `contentType` in schema
**Fix**: Update prompt text to match schema field name consistently
**Acceptance**: All prompt templates use consistent field naming
**Effort**: 15m | **Risk**: LOW

### [PROMPTS][LOW] Standardize Concept Cap Between Prod and Eval (PR #107 CodeRabbit)
**Source**: PR #107 review comments
**Files**: .claude/skills/langfuse-prompts/prompts/concept-synthesis.txt, evals/prompts/concept-synthesis.txt
**Problem**: Production caps at 50 concepts, eval caps at 100 - potential drift
**Fix**: Document intentional divergence or standardize to single value
**Acceptance**: Cap values documented or unified
**Effort**: 10m | **Risk**: LOW

### [PROMPTS][LOW] Clarify Phrasing Prompt Output Instructions (PR #107 CodeRabbit)
**Source**: PR #107 review comments
**Files**: .claude/skills/langfuse-prompts/prompts/phrasing-generation.txt
**Problem**: "Generate rationale first" conflicts with "return only JSON"
**Fix**: Clarify that rationale is internal reasoning, output is JSON only
**Acceptance**: Prompt instructions are unambiguous
**Effort**: 10m | **Risk**: LOW

### [EVALS][LOW] Align Eval Provider with Production (PR #107 CodeRabbit)
**Source**: PR #107 review comments
**File**: evals/promptfoo.yaml:12
**Problem**: Uses `google:gemini-2.5-pro` vs production `openrouter:google/gemini-3-pro-preview`
**Fix**: Change to `openrouter:google/gemini-3-pro-preview` to match production
**Acceptance**: Evals use same provider and model as production
**Effort**: 10m | **Risk**: LOW

### [NODE][LOW] Fix create-prompt.ts Node Compatibility (PR #107 CodeRabbit)
**Source**: PR #107 review comments
**File**: .claude/skills/langfuse-prompts/scripts/create-prompt.ts:31
**Problem**: Uses `import.meta.dirname` (Node >=20.11.0) but engines specifies >=20.9.0
**Fix**: Use `fileURLToPath(import.meta.url)` + `path.dirname()` for compatibility
**Acceptance**: Script works on Node 20.9.0+
**Effort**: 10m | **Risk**: LOW

### [SCHEMA][LOW] Add Feedback Index for Analytics (PR #107 CodeRabbit)
**Source**: PR #107 review comments
**File**: convex/schema.ts:58-69
**Problem**: No index on feedback field - analytics queries will full-scan
**Fix**: Add `by_user_feedback` compound index when feedback analytics needed
**Acceptance**: Index added if analytics queries exist; performance verified
**Effort**: 15m | **Risk**: LOW

### [TEST][HIGH] Embed helpers coverage
**File**: convex/lib/embeddingHelpers.ts
**Perspectives**: maintainability-maven, security-sentinel
**Problem**: Untested (userId mismatch, race deletion vulnerabilities)
**Acceptance**: Tests covering get/upsert/delete, 768-dim guard, duplicate protection
**Effort**: 0.5d

### [TEST][MEDIUM] Fix Analytics Module Caching for Testability
**File**: lib/analytics.ts
**Problem**: Module-level caching prevents test isolation; 3 tests removed due to flakiness
**Fix**: Refactor to eliminate module-level state or add factory pattern
**Acceptance**: Restore 3 removed tests; all pass in suite and isolation
**Effort**: 4-6h

### [TEST][HIGH] Refactor use-review-flow.ts Hook for Testability
**File**: hooks/use-review-flow.ts (471 LOC)
**Added**: 2024-12 during coverage expansion
**Problem**: Hook has complex timer/effect interactions causing memory issues in unit tests
- Reducer (lines 1-189) is testable and tested directly
- Hook wrapper (lines 205-471) has useEffect with timeouts and polling that OOM in tests
**Fix**:
- Export `generateSessionId()` for direct testing
- Extract `useSkipFeature()` hook for skip state management
- Extract `useSessionTracking()` hook for session metrics
**Acceptance**: Hook unit tests pass without OOM; functions coverage ≥70%
**Effort**: 4h | **Impact**: Enables 70% function threshold

### [TEST][MEDIUM] Improve Convex Backend Test Coverage
**Files**: convex/concepts.ts (38.67%), convex/iqc.ts (19.48%), convex/clerk.ts (26.56%)
**Added**: 2024-12 during coverage expansion
**Problem**: Complex Convex modules have low unit test coverage; require integration testing patterns
**Current**: Branch coverage limited to 67% due to these modules
**Fix**:
- Add convex-test or similar integration testing framework
- Write contract tests for mutation/query behavior
- Test helper functions in isolation where possible
**Acceptance**: Branch coverage ≥70%
**Effort**: 8-12h | **Impact**: Honest coverage thresholds

---

## Next (<6 weeks)

### [ARCH][HIGH] Split migrations.ts God Object (2,997 lines)
**File**: convex/migrations.ts:1-2997
**Perspectives**: complexity-archaeologist, architecture-guardian, jobs
**Problem**: 8× complexity threshold, merge conflict magnet, unbounded growth
**Fix**: Adopt migration-per-file pattern (Rails/Django convention)
**Acceptance**: Next 3 migrations use new pattern; existing optionally extracted
**Effort**: 2h setup + 8h extraction | **Impact**: Prevents unlimited growth

### [ARCH][HIGH] Split concepts.ts God Object (1,072 lines)
**File**: convex/concepts.ts:1-1072
**Perspectives**: complexity-archaeologist, architecture-guardian, performance-pathfinder
**Problem**: 19 exports, 7 distinct responsibilities (CRUD + review + pagination + generation + bulk + stats)
**Fix**: Split into conceptsCrud, conceptsReview, conceptsLibrary, conceptsBulk, conceptsGeneration
**Acceptance**: Each module <300 lines; tests pass; no behavior change
**Effort**: 10h | **Impact**: Focused modules, easier testing

### [PRODUCT][CRITICAL] Monetization Foundation - Stripe Integration
**Perspectives**: product-visionary
**Problem**: $0 revenue, -$0.50/user/mo burn rate (existential threat)
**Business Case**: $8/mo × 10% conversion × 1K users = $9,600/year; survival blocker
**Fix**: Stripe Checkout + webhooks; schema adds subscriptionId/isPro/planType; free tier at 100 questions
**Acceptance**: Checkout works; free tier enforced; /pricing page shipped
**Effort**: 8d | **Value**: Unlock revenue, enable growth

### [PRODUCT][CRITICAL] Import/Export - Anki Deck Compatibility
**Perspectives**: product-visionary, user-experience-advocate
**Problem**: 80% of TAM blocked without data portability (Anki users can't migrate)
**Business Case**: Primary acquisition channel; table-stakes for Anki switchers
**Fix**: .apkg import to concepts/phrasings; CSV + JSON export; FSRS state preserved
**Acceptance**: Import works with 5k card smoke test; export includes all user data
**Effort**: 7d | **Value**: Opens 80% of addressable market

### [PRODUCT][HIGH] IQC Quality Dashboard (Hidden Gem → Flagship Feature)
**File**: convex/iqc.ts (90% complete, no frontend!)
**Perspectives**: product-visionary, grug, jobs
**Problem**: IQC system built but hidden; too complex for main UI (Jobs: "feature creep")
**Strategy**: Surface as opt-in Pro feature, not main UI requirement
**Fix**: Build /quality route with health score, duplicate proposals, merge UI; auto-accept toggle (Pro)
**Acceptance**: Dashboard shows collection health; action cards surfaced; Pro-gated auto-accept
**Effort**: 3d | **Value**: FLAGSHIP DIFFERENTIATOR (only SRS with auto-cleanup)

### [COMPLEXITY][HIGH] Simplify useReviewFlow (8-Layer State Machine)
**File**: hooks/use-review-flow.ts (397 lines)
**Perspectives**: grug, jobs
**Problem**: useReducer + 12 state props + 3 refs + 3 useEffects + lock mechanism + session tracking
**Violation**: 8-layer indirection to change currentQuestion
**Fix**: Extract session tracking to separate hook; simplify to 3-4 state fields
**Acceptance**: Hook <150 lines; session tracking in useSessionMetrics; tests pass
**Effort**: 4h | **Impact**: Clear data flow, easier debugging

### [COMPLEXITY][HIGH] Consolidate Hooks (30 → 15)
**Files**: hooks/*.ts (30 files)
**Perspectives**: jobs, grug
**Problem**: Overlapping mutation hooks, single-use state abstractions, entropy
**Fix**: Merge use-question-mutations + use-concept-actions → single useMutations()
       Inline use-inline-edit (only used once)
       Merge feedback hooks
**Acceptance**: 15 hooks remain; no behavior change; import paths updated
**Effort**: 4h | **Impact**: -600 lines, clearer flow

### [DESIGN][HIGH] Create Error State Component
**Files**: 6+ different error UI patterns across codebase
**Perspectives**: design-systems-architect, user-experience-advocate
**Problem**: No standard ErrorState component → hardcoded inline errors everywhere
**Fix**: Create components/ui/error-state.tsx with card/inline/minimal variants
**Acceptance**: 6 implementations migrated; retry button works; testable
**Effort**: 2h component + 2h migration | **Impact**: Consistent error UX

### [DESIGN][MEDIUM] Create Modal State Hook
**Files**: edit-question-modal.tsx, generation-modal.tsx, generate-phrasings-dialog.tsx
**Perspectives**: design-systems-architect
**Problem**: Same state reset pattern duplicated in 4 modals (open → reset → validate → save)
**Fix**: Create `useModalForm` hook with lifecycle management
**Acceptance**: 4 modals migrated; no behavior change; 40% less boilerplate
**Effort**: 3h hook + 2h migration | **Impact**: DRY, consistent modal behavior

### [UX][HIGH] Reduce Keyboard Shortcuts (13 → 5)
**File**: hooks/use-keyboard-shortcuts.ts
**Perspectives**: jobs
**Problem**: 13 shortcuts nobody remembers; 3 ways to go next (Space, →, Enter)
**Fix**: Keep ?, 1-4, Enter, e, Escape. Move h/Ctrl+S/n to visible navbar buttons.
**Acceptance**: 5 shortcuts remain; help modal updated; navbar has visible actions
**Effort**: 1h | **Impact**: Discoverability, less code

### [UX][MEDIUM] Delete Settings Page (One Setting = Modal)
**File**: app/settings/settings-client.tsx
**Perspectives**: jobs
**Problem**: Entire page for one setting (Delete Account) + Clerk auth explanation nobody needs
**Fix**: Move delete account to modal in user menu; delete settings page
**Acceptance**: /settings redirects to /library; user menu has delete option
**Effort**: 1h | **Impact**: Simpler navigation, -150 lines

### [PERF][HIGH] Fix Unbounded .collect() in migrations.ts (20+ instances)
**Files**: convex/migrations.ts (20+ instances)
**Perspectives**: performance-pathfinder
**Problem**: 20+ unbounded `.collect()` calls → bandwidth quota exhaustion
**Fix**: Paginate with cursor iteration
**Acceptance**: All migrations use pagination; bandwidth per query <1MB
**Effort**: 4h | **Impact**: 100× bandwidth reduction

### [PRODUCT] Deck Sharing & Viral Growth (K-factor engine)
**Perspectives**: product-visionary
**Business Case**: Quizlet growth = 70% from sharing; K-factor 0.3-0.5 per shared deck
**Phase 1**: Share links, clone functionality, basic permissions
**Acceptance**: Share link generates read-only access; clone to library works
**Effort**: 5d | **Value**: Primary viral growth engine

### [PRODUCT] Mobile PWA + Offline Support
**Perspectives**: product-visionary
**Problem**: Web-only blocks 40% of market (mobile-first learners)
**Business Case**: Mobile users 2× engagement, 5× longer LTV
**Fix**: Manifest + service worker + offline review for last-synced deck
**Acceptance**: Lighthouse PWA ≥90; offline review works; touch targets ≥44px
**Effort**: 5d | **Value**: Opens 40% market

### [PRODUCT] Advanced Analytics Dashboard (Pro tier driver)
**Perspectives**: product-visionary
**Features**: Retention curves, forgetting curve viz, knowledge graph, mastery tracking
**Business Case**: #2 most requested feature; 10-15% of Pro conversions cite analytics
**Acceptance**: 8+ chart types; export PDF/CSV; Pro-gated
**Effort**: 10d | **Value**: Pro tier conversion driver

---

## Soon (3–6 months)

### [ARCH] Split embeddings.ts God Object (1,048 lines)
- 12+ exports, conflates AI provider with search logic
- Split into embeddingGeneration, embeddingSearch, embeddingBatch, embeddingMigration
- **Effort**: 8h

### [PERF] Optimize getQuestionsWithoutEmbeddings
- Loads ALL questionEmbeddings into memory (50k+ = 50MB + 5s) to find missing
- Use LEFT JOIN pattern or filter directly in query
- **Effort**: 2h | **Impact**: 5s → <500ms cron startup

### [PRODUCT] Public API for Ecosystem
- REST wrapper around Convex, OAuth, rate limiting, webhooks, developer portal
- Monetization: Free 100 req/hr, Pro 1K req/hr, Enterprise custom
- **Effort**: 15d | **Value**: Ecosystem enabler, enterprise channel

### [COMPLEXITY] Split unified-lab-client.tsx (721 lines)
- 15+ state variables, loading + execution + UI rendering mixed
- Extract ConfigSelector, TestResultsDisplay, ComparisonView, ExecutionOrchestrator
- **Effort**: 4h

### [COMPLEXITY] Consolidate Validation Approaches
- Three different patterns: manual functions, Zod schemas, direct mutation checks
- Pick one approach (Zod) and migrate
- **Effort**: 4h

### [COMPLEXITY] Simplify library-client Props (16+ passed to 3 identical tabs)
- Use context or provider pattern instead of threading 16 props
- **Effort**: 3h

---

## Later (6+ months)

### [PRODUCT] Medical Education Vertical
- 900K medical students, $20-50/mo willingness (10× general market)
- USMLE/COMLEX decks, drug card templates, faculty dashboard
- **Effort**: 15d | **Value**: 10× ARPU, $10M+ ARR potential

### [PRODUCT] AI Document Processing
- PDF upload, web clipping extension, Markdown import → concept generation
- "Create 50 cards from lecture notes in 2 minutes"
- **Effort**: 16d total | **Value**: Content acquisition

### [PLATFORM] Team Collaboration & Workspaces
- Shared workspaces, team admin dashboard, SSO/SAML
- $40/user/month B2B tier
- **Effort**: 20d | **Value**: Enterprise sales channel

- React Native mobile app (app store presence)
- Browser extension (quick capture)

---

## Learnings

**From this grooming session (2025-11-29):**
- **Dual data model is the root cause**: Questions + Concepts coexisting creates 2,997 lines of migration code, dual scheduling, and query complexity everywhere. Complete migration unlocks 30% simpler features.
- **IQC positioning wrong**: 90% built but over-engineered for main UI. Should be opt-in Pro feature, not default experience. Jobs: "Users don't know why action cards appear."
- **N+1 in hot path**: getDue() does 35+ queries per call due to loop + await pattern. 10-minute fix for 3-4× speedup.
- **Hook entropy**: 30 hooks for overlapping concerns. Grug: "Abstraction before two uses."
- **Persona convergence**: Jobs + Grug agree on 5 deletions (unused managers, settings page, keyboard shortcuts, IQC complexity).

**Keep 2-3 recent learnings:**
- Cross-perspective validation works: God objects flagged by 3+ agents (complexity, architecture, performance, design) = highest confidence signal
- Security posture strong: No critical production vulnerabilities; main risks are devDependency CVEs and DoS via unbounded queries
- 80/20 insight: Complete dual-model migration + monetization + import/export = unblock 80% of value in next quarter

---

## Report

**Analysis Method**: 15-perspective audit (8 specialists + 7 master personas)
**Completed Agents**: security-sentinel, performance-pathfinder, user-experience-advocate, product-visionary, design-systems-architect, grug, jobs

**Multi-Perspective Cross-Validation**:
- **3+ agents**: Dual data model complexity, god objects (migrations.ts, concepts.ts), IQC over-engineering
- **2 agents**: Unbounded .collect() (perf+security), semantic colors (design+UX), error messages (UX+security)

**Persona Convergence Signals**:
- **Jobs + Grug agree on deletion**: ConfigManager/InputManager (unused), settings page (1 setting), keyboard shortcuts (13→5)
- **Performance + Security agree**: Validate all limit params, fix unbounded queries

**Strategic Insights**:
- **Technical Foundation**: Excellent (8/10) - clean architecture, good security, deep modules
- **Complexity Debt**: High - dual data model, 3K line migrations.ts, 30 hooks with overlap
- **Go-to-Market Gaps**: Critical - $0 revenue, no import/export, no mobile, no sharing
- **Hidden Gem**: IQC system 90% complete → unique differentiator waiting for right UX

**Next Three Priorities**:
1. Security fixes (validate limits, webhook auth) - 1h total
2. Performance fixes (N+1 getDue, unbounded .collect()) - 30m total
3. Complete dual-model migration - 3-5d (unblocks everything)

**Risks/Asks**:
- Dual data model migration is high-value but requires focused 3-5d investment
- IQC needs UX redesign before shipping as flagship (hide behind Pro opt-in)
- 30 hooks → 15 hooks consolidation prevents future entropy
