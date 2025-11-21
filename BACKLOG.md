# BACKLOG

**Last Groomed**: 2025-11-20
**Analysis Method**: 8-perspective comprehensive audit (complexity-archaeologist, architecture-guardian, security-sentinel, performance-pathfinder, maintainability-maven, user-experience-advocate, product-visionary, design-systems-architect)
**Overall Assessment**: Strong technical foundation with critical go-to-market gaps. God objects (migrations.ts, concepts.ts) and unbounded queries need refactoring. Hidden gem: IQC system 90% built, needs UX polish.

---

## Now (<2 weeks, sprint-ready)

### [SECURITY][CRITICAL] Update happy-dom Dependency - RCE Vulnerability
**File**: package.json:118
**Perspectives**: security-sentinel, architecture-guardian
**Problem**: happy-dom@18.0.1 has CRITICAL RCE vulnerability (CVE-2025-XXXXX) via VM context escape
**Impact**: DevDependency only (not in production), but affects test environment and CI/CD
**Fix**: `pnpm update happy-dom@^20.0.2`
**Acceptance**: Tests pass with updated version, CVE resolved, no breaking changes
**Effort**: 15m | **Risk**: CRITICAL → NONE

### [SECURITY][CRITICAL] Update glob Dependency - Command Injection
**File**: package.json (transitive via @vitest/coverage-v8)
**Perspectives**: security-sentinel
**Problem**: glob@<10.5.0 has HIGH command injection vulnerability via `-c/--cmd` flag
**Impact**: DevDependency, unlikely attack vector (requires glob CLI usage with user input)
**Fix**: `pnpm update glob@^10.5.0 vite@^7.2.4`
**Acceptance**: vitest coverage works, vulnerabilities resolved
**Effort**: 15m | **Risk**: HIGH → NONE

### [SECURITY][CRITICAL] Fail-closed Clerk webhooks
**File**: convex/http.ts:56-62
**Perspectives**: security-sentinel, architecture-guardian
**Problem**: Returns 200 when `CLERK_WEBHOOK_SECRET` missing → spoofed user create/delete possible
**Acceptance**: Secret required in prod; failure returns 500 + structured log/alert; test for missing/invalid signature; deploy script check blocks rollout without secret
**Effort**: 0.5d | **Principles**: Fail-closed config, Ousterhout info hiding

### [SECURITY][CRITICAL] HttpOnly session token handling
**File**: lib/auth-cookies.ts:5-33
**Perspectives**: security-sentinel
**Problem**: Sets session token via JS, non-HttpOnly and not always `Secure` → XSS/sniff risk
**Acceptance**: Move to server-set HttpOnly `SameSite=Lax; Secure` cookie; remove JS setters/getters; regression test auth boot; doc migration for existing sessions
**Effort**: 1d | **Principles**: Make misuse hard, least exposure

### [PRODUCT][CRITICAL] Monetization Foundation
**File**: New - Stripe integration
**Perspectives**: product-visionary, business-survival
**Problem**: -$0.50/user/mo, $0 revenue (existential threat)
**Business Case**: $8/mo × 10% conversion × 1K users = $800/mo → $9,600/year; survival blocker
**Acceptance**: Stripe Checkout + webhooks live; schema (`subscriptionId`, `isPro`, `planType`) migrated; free tier enforced at 100 questions in `questionsCrud`; `/pricing` + upgrade modal shipped; happy-path e2e
**Effort**: 8d | **Impact**: Unlock revenue, enable sustainable growth

### [PRODUCT][CRITICAL] Import/Export (Adoption Block)
**File**: New feature
**Perspectives**: user-experience-advocate, product-visionary
**Problem**: Anki users churn without data portability (80% of TAM blocked)
**Business Case**: Primary acquisition channel; table-stakes feature for Anki switchers
**Acceptance**: `.apkg` import to concepts/phrasings; CSV + JSON export; progress/error UI; 5k card smoke test; FSRS state preserved on import
**Effort**: 7d | **Impact**: Opens 80% of addressable market

### [PERF][HIGH] Library selection O(N×M) - Already Tracked
**File**: app/library/_components/library-table.tsx:320-358
**Perspectives**: performance-pathfinder, architecture-guardian
**Problem**: Rebuilds selection with `findIndex` per tick; 50×20 → 1000 ops/render
**Acceptance**: Memoized id→flag map; updates O(M); selection stable after sort/filter; perf check <5ms per change; test for selection persistence
**Effort**: 0.5d | **Principles**: Deep module, avoid temporal decomposition

### [TEST][HIGH] Embed helpers coverage - Already Tracked
**File**: convex/lib/embeddingHelpers.ts
**Perspectives**: maintainability-maven, security-sentinel
**Problem**: Untested (userId mismatch, race deletion vulnerabilities)
**Acceptance**: `convex/lib/embeddingHelpers.test.ts` covering get/upsert/delete, 768-dim guard, duplicate protection; Vitest green in CI
**Effort**: 0.5d | **Principles**: Correctness guardrail

### [TEST][HIGH] Payment/auth test coverage - Already Tracked
**File**: New tests
**Perspectives**: maintainability-maven, security-sentinel
**Problem**: 0% test coverage on payment/subscription logic → no regression detection for money code
**Acceptance**: Tests for subscription validation, upgrade flow, free tier enforcement; auth cookie handling tested; critical paths >80% coverage; CI enforces thresholds
**Effort**: 1.5d | **Principles**: Test critical paths, money code gets tests

### [COMPLEXITY][HIGH] Extract User Stats Counter Logic
**Files**: convex/questionsBulk.ts:140-156, :203-218, :262-279; convex/questionsCrud.ts:63-68
**Perspectives**: complexity-archaeologist, maintainability-maven
**Problem**: State counting logic duplicated 4 times (totalCards, newCount, learningCount, matureCount calculation repeated)
**Violation**: DRY principle, temporal decomposition (logic scattered across lifecycle operations)
**Fix**: Extract `calculateStatsDeltaFromQuestions(questions, 'increment' | 'decrement')` to convex/lib/userStatsHelpers.ts
**Acceptance**: 4 call sites use helper; tests cover increment/decrement; no behavior change
**Effort**: 1.5h | **Impact**: Eliminates 45 lines of duplication, single source of truth for stat deltas

### [UX][HIGH] Error Message Translation Layer
**Files**: convex/generationJobs.ts:27-35, questionsCrud.ts:187-228, validation.ts:54-58
**Perspectives**: user-experience-advocate, security-sentinel (info disclosure)
**Problem**: Backend errors expose technical details ("Question not found or unauthorized: k17abc123")
**Impact**: Users see jargon, attackers get reconnaissance data
**Fix**: Create `translateBackendError()` in lib/error-handlers.ts; map common errors to user-friendly messages
**Acceptance**: 10+ common errors mapped; unit tests for translation; rollout to frontend mutation calls
**Effort**: 2h | **Value**: Users understand errors and how to fix them

### [TEST][HIGH] Increase Convex Backend Test Coverage
**Files**: convex/**/*.ts (124 source files, 23 test files = 18.5% test ratio)
**Perspectives**: maintainability-maven, security-sentinel
**Problem**: Convex backend at 13.16% coverage (vs 24.36% overall) → critical backend logic untested
**Impact**: Backend mutations, FSRS calculations, data integrity logic have no regression protection
**Fix**: Add tests for high-risk convex modules (aiGeneration, generationJobs, questionsInteractions, migrations helpers)
**Acceptance**: Convex coverage from 13% → 30%; critical mutation pairs tested; CI enforces minimum thresholds
**Effort**: 2d | **Value**: Regression protection for backend logic, catch bugs before production

### [TEST][MEDIUM] Add Coverage Thresholds for Critical Modules
**File**: vitest.config.ts:30-36
**Perspectives**: maintainability-maven (quality gates)
**Problem**: Single global threshold (18.2%) → critical paths (payment, auth) can drop to 0% without failing CI
**Impact**: No protection for money code or security-critical modules
**Fix**: Add per-module coverage thresholds in vitest.config.ts
```typescript
coverage: {
  thresholds: {
    'convex/**/*.ts': { lines: 30, functions: 30 },
    'lib/payment/**/*.ts': { lines: 80, functions: 80 },
    'lib/auth/**/*.ts': { lines: 80, functions: 80 },
  }
}
```
**Acceptance**: Critical modules have enforced minimums; CI fails if thresholds drop; non-critical code flexible
**Effort**: 20m | **Value**: Ratcheted protection for critical paths

---

## Next (<6 weeks)

### [INFRA][LOW] Extract Sentry Org/Project to GitHub Actions Variables
**File**: .github/workflows/release.yml:52-53
**Perspectives**: maintainability-maven
**Problem**: `SENTRY_ORG` and `SENTRY_PROJECT` are hardcoded in workflow
**Impact**: Low - values rarely change, but hardcoding reduces portability across forks/environments
**Fix**: Extract to repository variables or add inline comment explaining intentional hardcoding
**Acceptance**: Either (a) values moved to GitHub Actions variables, or (b) comment added explaining stability
**Effort**: 15m | **Impact**: Maintainability improvement for multi-tenant/fork scenarios

### [INFRA][LOW] Verify Sentry Source Map Alignment in Production
**File**: .github/workflows/release.yml + Next.js build
**Perspectives**: user-experience-advocate (debugging UX)
**Problem**: GitHub Actions uses `github.sha` for release name, Vercel uses `VERCEL_GIT_COMMIT_SHA`
**Current state**: Documented alignment (deployment-checklist.md:65-67) states they match for GitHub-triggered deployments
**Validation needed**: Verify in production that source maps uploaded by both paths reference same release ID
**Acceptance**: Test error in production → verify Sentry shows unminified stack trace with correct file/line; document findings
**Effort**: 30m | **Impact**: Debugging UX confidence, validates observability stack integration

### [ARCH][HIGH] Split migrations.ts God Object (2,997 lines)
**File**: convex/migrations.ts:1-2997
**Perspectives**: complexity-archaeologist, architecture-guardian, maintainability-maven
**Problem**: 2,997 lines (8× complexity threshold), unbounded growth pattern, merge conflict magnet
**Violation**: Single Responsibility (quiz migration + field cleanup + clustering + synthesis all in one file)
**Fix**: Adopt migration-per-file pattern (Rails/Django convention)
```
convex/migrations/
  2025_01_15_quiz_results_migration.ts
  2025_01_20_remove_difficulty_field.ts
  2025_02_01_user_created_at_backfill.ts
  index.ts (registry)
```
**Acceptance**: Next 3 migrations use new pattern; existing migrations optionally extracted; migrations/ documented in CLAUDE.md
**Effort**: 2h initial setup + 8h extraction (optional) | **Impact**: Prevents unlimited growth, clearer git history

### [ARCH][HIGH] Split concepts.ts God Object (1,072 lines)
**File**: convex/concepts.ts:1-1072
**Perspectives**: complexity-archaeologist, architecture-guardian, performance-pathfinder
**Problem**: 1,072 lines, 19 exports, 7 distinct responsibilities (CRUD + review + pagination + generation + bulk + stats)
**Violation**: Single Responsibility, high coupling (6/10), O(N) phrasing fetches for counts
**Fix**: Split into 5 focused modules
- conceptsCrud.ts (createMany, getDetail - 200 lines)
- conceptsReview.ts (getDue, recordInteraction, FSRS - 250 lines)
- conceptsLibrary.ts (listForLibrary, pagination - 300 lines)
- conceptsBulk.ts (runBulkAction, archive/restore - 250 lines)
- conceptsGeneration.ts (requestPhrasingGeneration - 100 lines)
**Acceptance**: Tests pass; no behavior change; coupling reduced to 3/10; each module <300 lines
**Effort**: 10h | **Impact**: Focused modules, reduced coupling, easier testing

### [PERF][HIGH] Fix Unbounded .collect() in migrations.ts
**Files**: convex/migrations.ts (20+ instances), spacedRepetition.ts:216-275 (getUserCardStats_DEPRECATED)
**Perspectives**: performance-pathfinder, architecture-guardian
**Problem**: 20+ unbounded `.collect()` calls fetch ALL records → bandwidth quota exhaustion (1GB/month Starter)
**Impact**: User with 10k cards → 2-3s query time, ~10MB response, quota burn
**Fix**: Paginate with cursor iteration; remove deprecated `getUserCardStats_DEPRECATED` if unused
**Acceptance**: All migrations use pagination; bandwidth per query <1MB; smoke test with 10k cards <500ms
**Effort**: 4h | **Impact**: 100× bandwidth reduction, prevents quota exhaustion

### [DESIGN][HIGH] Migrate Hardcoded Semantic Colors to Tokens
**Files**: components/generation-task-card.tsx, review/review-mode.tsx, review/learning-mode-explainer.tsx, concepts/concepts-table.tsx (15 components total)
**Perspectives**: design-systems-architect, maintainability-maven
**Problem**: 15 components use `bg-red-50`, `text-blue-700` instead of semantic tokens → manual dark mode, can't rebrand
**Fix**: Extend CSS token system with `--error-background`, `--success-border`, `--info-foreground`; migrate 15 components
**Acceptance**: All semantic states use tokens; dark mode automatic; design tokens documented
**Effort**: 2h | **Impact**: Consistent semantic colors, automatic dark mode, rebrandable

### [COMPLEXITY][HIGH] Extract Unified EmptyState Component
**Files**: components/empty-states.tsx, app/library/_components/library-empty-states.tsx, components/concepts/concepts-empty-state.tsx, components/review/review-empty-state.tsx
**Perspectives**: complexity-archaeologist, design-systems-architect
**Problem**: 4 files with 80% identical empty state code (icon + title + description + action pattern)
**Violation**: DRY principle, visual drift (inconsistent spacing, icon sizes)
**Fix**: Create components/ui/empty-state.tsx with variants ('default' | 'zen' | 'inline')
**Acceptance**: 4 implementations migrated; visual consistency verified; prop interface documented
**Effort**: 3h | **Impact**: Single source of truth, consistent spacing, faster iteration

### [UX][MEDIUM] Mobile PWA - Already Tracked
**File**: New - PWA configuration
**Perspectives**: product-visionary
**Problem**: Web-only blocks 40% of market (mobile-first learners)
**Business Case**: Mobile users 2× engagement, 5× longer lifetime
**Acceptance**: Manifest + service worker + offline caching; touch targets ≥44px; Lighthouse PWA score ≥90; offline review for last-synced deck
**Effort**: 5d | **Principles**: Availability, user-first

### [PERF][MEDIUM] Search cancel/backpressure - Already Tracked
**File**: app/library/_components/library-client.tsx:62-102
**Perspectives**: performance-pathfinder
**Problem**: Debounces but still fires every change; no Abort/backoff → wasted tokens
**Acceptance**: AbortController or action cancel; rate-limit to 1 in-flight; tests for stale-response ignore and request-count drop
**Effort**: 1d | **Principles**: Efficiency, explicit resource limits

### [DESIGN][MEDIUM] Consolidate empty states - Already Tracked
**File**: components/empty-states.tsx vs app/library/_components/library-empty-states.tsx
**Perspectives**: design-systems-architect
**Problem**: Parallel empty-state components drift
**Acceptance**: Single token-driven empty-state primitive; replace library variants; docs added
**Effort**: 1d (overlaps with Extract Unified EmptyState) | **Principles**: Design-system coherence

### [ARCH][MEDIUM] Retire deprecated questions table - Already Tracked
**File**: convex/schema.ts:33-104
**Perspectives**: architecture-guardian
**Problem**: Keeps deprecated `questions` + vector index beyond 2025-12-17 window
**Acceptance**: Phase-out plan (optional → migrate → drop field/index); diagnostics show zero dependents; deploy after migration
**Effort**: 2d | **Principles**: Remove shallow legacy

### [MAINT][MEDIUM] Component tests for primitives - Already Tracked
**Perspectives**: maintainability-maven
**Acceptance**: Vitest for Button, CustomEmptyState, Card; cover disabled/variants/a11y
**Effort**: 1d | **Principles**: Refactor safety

### [DATA][MEDIUM] Make users.createdAt required - Already Tracked
**File**: convex/schema.ts:13
**Problem**: Optional createdAt left as TODO without migration plan
**Acceptance**: Backfill timestamps; schema required; guard in creates; migration plan documented
**Effort**: 1d | **Principles**: Explicit invariants

### [TEST][MEDIUM] Cleanup skipped tests - Already Tracked
**Perspectives**: maintainability-maven
**Problem**: 7 tests with `.skip` → false green in CI; skipped tests rot
**Acceptance**: Review each skip; fix or delete; document unskip plan; zero skips in main
**Effort**: 0.5d | **Principles**: Tests or no tests, no limbo

### [TEST][MEDIUM] E2E smoke tests foundation - Already Tracked
**Perspectives**: user-experience-advocate, maintainability-maven
**Problem**: Playwright configured but zero tests → happy-path regressions not caught
**Acceptance**: Smoke tests for auth flow, quiz creation, review session; run in CI on PRs; <2min total runtime; or delete config if decision is no E2E
**Effort**: 1.5d | **Principles**: Test user flows, no config theater

### [CI][LOW] Fix Lighthouse workflow Convex deploy - Already Tracked
**File**: .github/workflows/lighthouse.yml
**Perspectives**: architecture-guardian
**Problem**: Runs `pnpm build` without Convex deploy → may fail or test stale backend
**Acceptance**: Use vercel-build.sh or add `npx convex deploy &&` prefix; verify Lighthouse runs post-deploy
**Effort**: 0.25d | **Principles**: Stack-aware automation, Convex-first

### [UX][MEDIUM] Undo Toast for Deletions
**Files**: components/review-flow.tsx:181-201, hooks/use-question-mutations.ts
**Perspectives**: user-experience-advocate
**Problem**: After confirmation, deletion instant with no undo → high cognitive load for accidents
**Fix**: Add undo action to toast (Sonner supports `action` prop)
```typescript
toast.success('Question moved to trash', {
  action: {
    label: 'Undo',
    onClick: async () => {
      await restoreQuestion({ questionId });
      toast.success('Question restored');
    },
  },
  duration: 8000,
});
```
**Acceptance**: Undo button in delete toast; 8s window to undo; restoration works; tests for undo flow
**Effort**: 45m | **Impact**: Users fix mistakes without breaking flow

### [UX][MEDIUM] Generation Progress Indicator
**Files**: components/generation-modal.tsx:76-91, navbar (new badge)
**Perspectives**: user-experience-advocate
**Problem**: Modal closes instantly, no progress visibility unless user navigates to /tasks
**Fix**: Add background job indicator to navbar ("⏳ 1 generating" badge); toast with "View Progress" action
**Acceptance**: Navbar shows active job count; clicking badge navigates to /tasks; toast includes progress link
**Effort**: 2h | **Impact**: Users aware of progress without checking separate page

---

## Soon (3–6 months)

### [PRODUCT] Deck Sharing & Viral Growth
**Perspectives**: product-visionary
**Business Case**: K-factor 0.3-0.5 (each shared deck = 5-20 acquisitions); Quizlet's growth = 70% from sharing
**Phase 1** (5d): Share links, clone functionality, basic permissions
**Phase 2** (10d): Marketplace with discovery, ratings, creator profiles
**Monetization**: Free tier 3 shared decks, Pro unlimited, revenue share on premium decks
**Acceptance**: Share link generates read-only access; clone to library; public/private/link permissions
**Effort**: 5d (Phase 1) | **Value**: Primary viral growth engine

### [PRODUCT] IQC Feature Polish (Hidden Gem)
**File**: convex/iqc.ts (already 90% built!)
**Perspectives**: product-visionary, complexity-archaeologist
**Opportunity**: IQC system (concept clustering, duplicate detection, AI merging) is hidden - no frontend!
**Business Case**: Unique differentiator ("The only SRS app that auto-cleans your deck"), premium feature for auto-accept
**Fix**: Build Quality Dashboard showing health score, duplicate proposals, thin concepts, one-click merge UI
**Acceptance**: /quality route shows collection health; action cards surfaced; merge proposals reviewable; auto-accept toggle (Pro)
**Effort**: 3d | **Impact**: FLAGSHIP FEATURE differentiation

### [ARCH] Split embeddings.ts God Object (1,048 lines)
**File**: convex/embeddings.ts:1-1048
**Perspectives**: complexity-archaeologist, architecture-guardian
**Problem**: 12+ exports, conflates AI provider with search logic
**Fix**: Split into embeddingGeneration.ts, embeddingSearch.ts, embeddingBatch.ts, embeddingMigration.ts
**Effort**: 8h | **Impact**: Decouples AI provider from search

### [PERF] Optimize getQuestionsWithoutEmbeddings
**File**: convex/embeddings.ts:525-572
**Perspectives**: performance-pathfinder
**Problem**: Loads ALL questionEmbeddings into memory (50k+ records = 50MB + 5s) to find missing
**Fix**: Use LEFT JOIN pattern or filter directly in query instead of building full Map
**Effort**: 2h | **Impact**: 5s → <500ms cron startup (10× faster)

### [PRODUCT] Public API for Ecosystem
**Perspectives**: product-visionary
**Business Case**: Developer ecosystem = force multiplier; API access = enterprise requirement; Anki API dated
**Implementation**: REST wrapper around Convex, OAuth (Clerk), rate limiting, webhooks, developer portal
**Monetization**: Free 100 req/hr, Pro 1K req/hr, Enterprise custom + SLA
**Effort**: 15d | **Value**: Ecosystem enabler, enterprise sales channel

### [UX] Bulk Operations UI
**File**: app/library/_components/library-table.tsx
**Perspectives**: user-experience-advocate, performance-pathfinder (already tracked O(N×M) issue)
**Problem**: Managing 1,000+ cards requires one-by-one operations
**Fix**: Selection system (checkbox, select all/none/filtered) + bulk actions bar (archive, delete, export, reschedule)
**Note**: Perf optimization (memoized selection) already in Now section
**Acceptance**: Multi-select works; bulk actions confirmed; performance <50ms for 100 selections
**Effort**: 3d UI (after perf fix) | **Impact**: 10× faster for large collections

### [PRODUCT] Advanced Analytics Dashboard
**Perspectives**: product-visionary
**Features**: Retention curves, forgetting curve viz, knowledge graph, mastery tracking, time invested, streak heatmap
**Monetization**: Free tier basic stats, Pro tier full analytics, Team tier aggregate analytics
**Acceptance**: 8+ chart types; export reports (PDF/CSV); Pro-gated features enforced
**Effort**: 10d | **Value**: Pro tier conversion driver (#2 most requested feature)

- Schema observability: Convex metrics + alerting for webhook/auth failures (post fail-closed work)
- AI prompt hardening (sanitize + allowlist) once monetization covers cost

---

## Later (6+ months)

### [PRODUCT] Medical Education Vertical
**Market**: 900k medical students globally, $2B market, willingness to pay $20-50/mo (10× general)
**Features**: Medical content templates (drug cards, anatomy, clinical cases), pre-made USMLE/COMLEX decks, LMS integration, faculty dashboard
**Monetization**: Individual $20/mo, school license $500/year/student, institutional custom
**Effort**: 15d | **Value**: 10× ARPU, $10M+ ARR potential

### [PRODUCT] AI Document Processing
**Features**: PDF upload, web clipping browser extension, Markdown import, text extraction → concept generation
**Use Case**: "Create 50 cards from lecture notes in 2 minutes"
**Monetization**: Free text input only, Pro unlimited + file upload, Team bulk processing
**Effort**: 3d text → 5d files → 8d extension (16d total) | **Value**: Content acquisition, reduces creation friction

### [PLATFORM] Team Collaboration & Workspaces
**Features**: Shared workspaces, team admin dashboard, usage analytics, SSO/SAML
**Monetization**: $40/user/month B2B tier
**Effort**: 20d | **Value**: Enterprise sales channel

- React Native mobile app (app store presence, native features)
- Browser extension (quick capture)

---

## Learnings

**From this grooming session:**
- **God object bloat**: Time pressure led to accumulation (migrations.ts 3k lines, concepts.ts 1k lines) vs strategic splitting
- **Hidden gem discovered**: IQC system 90% built but not surfaced to users - flagship differentiator waiting for UX
- **Bandwidth anti-patterns**: 20+ unbounded `.collect()` calls risk quota exhaustion (1GB/month Starter plan)
- **Design system strength**: Strong token foundation but 15 components bypass semantic colors (hardcoded blue/red/green)
- **Cross-perspective validation works**: God objects flagged by 3+ agents (complexity, architecture, maintainability) = highest confidence signal
- **Product-market fit gap**: Technical excellence but missing go-to-market features (monetization, import/export, mobile, sharing)
- **Security posture strong**: No critical production vulnerabilities; main risks are devDependency CVEs (happy-dom RCE)
- **80/20 insight**: 5 features in 28 days unlock revenue + 3× adoption + viral growth + differentiation

**Keep 2-3 recent learnings:**
- Fail-open auth surfaced; config validation must be part of deploy scripts
- Library UI still mixes state/render logic; small perf fixes deliver big UX wins
- Quality gates audit: Excellent foundation (Lefthook + Gitleaks + Trivy + Changesets); main gap is test coverage (18% vs 60% target) + Husky/Lefthook conflict resolved

---

## Report

**Analysis Method**: 8 parallel agents (complexity-archaeologist, architecture-guardian, security-sentinel, performance-pathfinder, maintainability-maven, user-experience-advocate, product-visionary, design-systems-architect)

**Multi-Perspective Cross-Validation**:
- **3+ agents**: God objects (migrations.ts, concepts.ts), dependency CVEs (happy-dom RCE)
- **2 agents**: Hardcoded colors breaking tokens, empty state duplication, unbounded `.collect()`, poor error messages, stat counter duplication

**Strategic Insights**:
- **Technical Foundation**: Excellent (7.5/10) - clean architecture, no circular dependencies, deep modules, comprehensive infrastructure
- **Go-to-Market Gaps**: Critical - no revenue ($0), no import/export (80% TAM blocked), no mobile (40% market lost), no sharing (zero viral growth)
- **Hidden Opportunities**: IQC system 90% complete (unique differentiator), design token system strong (needs semantic color migration)
- **High-Leverage Wins**: 5 features × 28 days = revenue + adoption + growth + differentiation

**Shifts from Previous Backlog**:
- **Added**: 10+ new items from 8-perspective audit (god object splits, `.collect()` fixes, UX improvements, design system refinements)
- **Preserved**: All existing CRITICAL items (security, monetization, import/export, perf)
- **Enhanced**: Business justification for product features, effort estimates for technical work
- **Organized**: Clear Now/Next/Soon/Later with detail gradient

**Next Three Priorities**:
1. Security updates (happy-dom, glob CVEs - 30m)
2. Monetization foundation (8d - survival blocker)
3. Import/Export (7d - adoption unlocker)

**Risks/Asks**:
- Dependency updates should be immediate (security)
- God object splits are high-value but require 18h investment (migrations + concepts)
- IQC feature polish is quick win (3d) for unique differentiation
- Product priorities (monetization, import/export, mobile PWA) already well-prioritized in existing backlog
