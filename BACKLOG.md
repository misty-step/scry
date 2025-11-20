# BACKLOG

**Last Groomed**: 2025-11-20  
**Analysis Method**: 8-perspective audit (TODO.md/DESIGN.md/PR feedback files not present)  
**Overall Assessment**: Two fail-open auth paths + no revenue. Legacy schema + library perf are primary drag; otherwise arch remains solid.

---

## Now (<2 weeks, sprint-ready)

### [SECURITY][CRITICAL] Fail-closed Clerk webhooks
- Perspectives: security-sentinel, architecture-guardian
- Problem: `convex/http.ts:56-62` returns 200 when `CLERK_WEBHOOK_SECRET` missing → spoofed user create/delete possible.
- Acceptance: secret required in prod; failure returns 500 + structured log/alert; test for missing/invalid signature; deploy script check blocks rollout without secret.
- Effort: 0.5d | Principles: fail-closed config, Ousterhout info hiding.

### [SECURITY][CRITICAL] HttpOnly session token handling
- Perspectives: security-sentinel
- Problem: `lib/auth-cookies.ts:5-33` sets session token via JS, non-HttpOnly and not always `Secure` → XSS/sniff risk.
- Acceptance: move to server-set HttpOnly `SameSite=Lax; Secure` cookie; remove JS setters/getters; regression test auth boot; doc migration for existing sessions.
- Effort: 1d | Principles: make misuse hard, least exposure.

### [PRODUCT][CRITICAL] Monetization Foundation
- Perspectives: product-visionary, business-survival
- Problem: -$0.50/user/mo, $0 revenue.
- Acceptance: Stripe Checkout + webhooks live; schema (`subscriptionId`, `isPro`, `planType`) migrated; free tier enforced at 100 questions in `questionsCrud`; `/pricing` + upgrade modal shipped; happy-path e2e.
- Effort: 8d | Impact: unlock revenue.

### [PRODUCT][CRITICAL] Import/Export (Adoption block)
- Perspectives: user-experience-advocate
- Problem: Anki users churn without data portability.
- Acceptance: `.apkg` import to concepts/phrasings; CSV + JSON export; progress/error UI; 5k card smoke test.
- Effort: 7d | Impact: primary acquisition channel.

### [PERF/ARCH][HIGH] Library selection O(N×M)
- Perspectives: performance-pathfinder, architecture-guardian
- Problem: `app/library/_components/library-table.tsx:320-358` rebuilds selection with `findIndex` per tick; 50×20 → 1000 ops/render.
- Acceptance: memoized id→flag map; updates O(M); selection stable after sort/filter; perf check <5ms per change; test for selection persistence.
- Effort: 0.5d | Principles: deep module, avoid temporal decomposition.

### [TEST][HIGH] Embed helpers coverage
- Perspectives: maintainability-maven, security-sentinel
- Problem: `convex/lib/embeddingHelpers.ts` untested (userId mismatch, race deletion).
- Acceptance: `convex/lib/embeddingHelpers.test.ts` covering get/upsert/delete, 768-dim guard, duplicate protection; Vitest green in CI.
- Effort: 0.5d | Principles: correctness guardrail.

### [TEST][HIGH] Payment/auth test coverage
- Perspectives: maintainability-maven, security-sentinel
- Problem: 0% test coverage on payment/subscription logic → no regression detection for money code; monetization foundation (BACKLOG item) ships without tests.
- Acceptance: Tests for subscription validation, upgrade flow, free tier enforcement; auth cookie handling tested; critical paths >80% coverage; CI enforces thresholds.
- Effort: 1.5d | Principles: test critical paths, money code gets tests.

### [MAINT][MEDIUM] Remove Husky hook system
- Perspectives: complexity-archaeologist, maintainability-maven
- Problem: Both `.husky/_/` and `.lefthook.yml` installed → hooks may run twice or conflict; `core.hookspath .husky/_` overrides Lefthook.
- Acceptance: `.husky/` deleted; `git config --unset core.hookspath`; `package.json` scripts cleaned; hooks verified single execution; doc migration guide.
- Effort: 0.25d | Principles: one way to do it, remove shallow duplicates.

---

## Next (<6 weeks)

### [UX][HIGH] Mobile PWA
- Manifest + service worker + touch targets; lighthouse PWA score ≥90; offline review for last-synced deck. Effort: 5d. Principles: availability, user-first.

### [PERF][MEDIUM] Search cancel/backpressure
- Problem: `app/library/_components/library-client.tsx:62-102` debounces but still fires every change; no Abort/backoff → wasted tokens.
- Acceptance: AbortController or action cancel; rate-limit to 1 in-flight; tests for stale-response ignore and request-count drop.
- Effort: 1d. Principles: efficiency, explicit resource limits.

### [DESIGN][MEDIUM] Consolidate empty states
- Problem: Parallel empty-state components (`components/empty-states.tsx`, `app/library/_components/library-empty-states.tsx`) drift.
- Acceptance: single token-driven empty-state primitive; replace library variants; docs added. Effort: 1d. Principles: design-system coherence.

### [ARCH][MEDIUM] Retire deprecated questions table
- Problem: `convex/schema.ts:33-104` keeps deprecated `questions` + vector index beyond 2025-12-17 window.
- Acceptance: Phase-out plan (optional → migrate → drop field/index); diagnostics show zero dependents; deploy after migration. Effort: 2d. Principles: remove shallow legacy.

### [MAINT][MEDIUM] Component tests for primitives
- Add Vitest for `Button`, `CustomEmptyState`, `Card`; cover disabled/variants/a11y. Effort: 1d. Principles: refactor safety.

### [DATA][MEDIUM] Make `users.createdAt` required
- Problem: `convex/schema.ts:13` optional createdAt left as TODO.
- Acceptance: backfill timestamps; schema required; guard in creates; migration plan. Effort: 1d. Principles: explicit invariants.

### [TEST][MEDIUM] Cleanup skipped tests
- Perspectives: maintainability-maven
- Problem: 7 tests with `.skip` → false green in CI; skipped tests rot and lose value.
- Acceptance: Review each skip; fix underlying issue or delete test; document unskip plan for environment-dependent tests; zero skips in `main`. Effort: 0.5d. Principles: tests or no tests, no limbo.

### [TEST][MEDIUM] E2E smoke tests foundation
- Perspectives: user-experience-advocate, maintainability-maven
- Problem: Playwright configured (`tests/e2e/` exists) but zero tests → happy-path regressions not caught; `playwright.config.ts` configured for unused suite.
- Acceptance: Smoke tests for auth flow, quiz creation, review session; run in CI on PRs; <2min total runtime; delete config if decision is no E2E. Effort: 1.5d. Principles: test user flows, no config theater.

### [CI][LOW] Fix Lighthouse workflow Convex deploy
- Perspectives: architecture-guardian
- Problem: `.github/workflows/lighthouse.yml` runs `pnpm build` without Convex deploy → may fail or test stale backend.
- Acceptance: Use `vercel-build.sh` or add `npx convex deploy &&` prefix; verify Lighthouse runs post-deploy; passes in CI. Effort: 0.25d. Principles: stack-aware automation, Convex-first.

---

## Soon (3–6 months)
- Deck Sharing (viral loop, gated rollout) — depends on import/export telemetry.
- Schema observability: add Convex metrics + alerting for webhook/auth failures (post fail-closed work).
- AI prompt hardening (sanitize + allowlist of system instructions) once monetization covers cost.

---

## Later (6+ months)
- Team Collaboration (seats/roles, $40/user/mo)
- React Native app (store presence)
- Browser extension (quick capture)

---

## Learnings
- Fail-open auth surfaced; config validation must be part of deploy scripts.
- Library UI still mixes state/render logic; small perf fixes deliver big UX wins.
- Docs drift (TODO.md/DESIGN.md missing); guardrails need actual files or removal.
- Quality gates audit: Excellent foundation (Lefthook + Gitleaks + Trivy + Changesets); main gap is test coverage (18% vs 60% target) + Husky/Lefthook conflict.

---

## Report
- Shifts: added payment/auth test coverage item to Now (blocks monetization safely); added Husky removal + skipped test cleanup to Next; identified Lighthouse workflow gap.
- Quality gates status: No security theater detected; all gates catching real issues; CI runtime optimized with parallelization; Lefthook <5s pre-commit achieved.
- Test coverage gap: 18.2% overall, 0% on payment logic → phased improvement plan in vitest.config.ts (18%→30%→45%→60%).
- Next three /prompts:spec: (1) Payment test suite (subscription validation, upgrade flow), (2) Remove Husky conflict, (3) Fix skipped tests.
- Risks/asks: Payment tests should block monetization PR merge; Husky removal requires local `git config --unset` for all devs; E2E decision needed (implement smoke tests or delete Playwright config).
