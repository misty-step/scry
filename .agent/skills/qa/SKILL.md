---
name: qa
description: |
  Scry browser QA, exploratory testing, smoke verification, dogfood evidence,
  and merge-readiness handoff. Drive the real Next.js/Convex/Clerk app and
  verify the spaced-repetition UX, not just green test output.
  Use when: "run QA", "test this", "verify the feature", "exploratory test",
  "check the app", "QA this PR", "capture evidence", "manual testing",
  "dogfood scry".
  Trigger: /qa.
argument-hint: "[url|route|feature|dogfood|smoke|full]"
---

# /qa

Scry QA protects the review sanctuary. The core product is `/`: Willow's
review chat over concept-level FSRS. QA must verify the user experience,
console and network health, auth behavior, and mobile Chrome behavior. Passing
tests are evidence, not the verdict.

## Scry Stance

- Treat `/` review chat as the primary route. `/agent` is legacy and should
  redirect to `/`.
- Preserve pure FSRS expectations: no daily caps, no comfort-mode shortcuts,
  no "easy" affordances that change scheduling behavior.
- Use `pnpm` only. The dev server is `pnpm dev`, which runs Next.js with
  Turbopack and `convex dev`.
- Do not run production-affecting commands during QA: no `pnpm build:local`,
  `pnpm build:prod`, `pnpm convex:deploy`, production migrations, or
  `./scripts/deploy-production.sh` unless the operator explicitly approves.

## Commands

| Scope | Command | Use When |
|---|---|---|
| Local app | `pnpm dev` | You need an interactive local target at `http://localhost:3000` |
| PR smoke | `pnpm test:e2e:smoke` | Fast Chromium smoke against local or `PLAYWRIGHT_BASE_URL` |
| Full browser suite | `pnpm test:e2e:full` | Release/merge QA, mobile Chrome coverage, or route-wide confidence |
| Dogfood production | `pnpm qa:dogfood` | Unauthenticated smoke against `https://scry.study` with screenshots/report |
| Dogfood local | `pnpm qa:dogfood:local` | Same dogfood script against local dev |

Playwright defaults live in `playwright.config.ts`: local runs boot
`pnpm dev`; CI defaults to `https://scry.vercel.app`; `PLAYWRIGHT_BASE_URL`
overrides either. The configured projects are `chromium` and `Mobile Chrome`.
The smoke script is Chromium-only, so any merge-relevant visual or layout QA
must include `pnpm test:e2e:full` or an explicit Mobile Chrome pass.

## Critical Routes

| Route | What QA Must Verify |
|---|---|
| `/` | Unauthenticated users see "Sign in to start reviewing" with `/sign-in` and `/sign-up` links. Authenticated users see `ReviewChat`, due-count behavior, Begin Session, answer options, Submit, feedback, Next, deterministic chips like `Explain this concept` and `Reschedule`, and review actions for edit/archive with undo where available. No `/ Review` navbar breadcrumb. |
| `/agent` | Redirects to `/` and does not crash. Keep this as compatibility smoke only; do not treat it as the product home. |
| `/concepts` | Concepts Library loads behind auth, with All/Due/Archived/Trash views, search by title, sort, bounded page sizes, Previous/Next pagination, row selection, bulk archive/unarchive/delete/restore semantics, and "Generation job started" for thin concept phrasing generation when available. |
| Generation modal from navbar | Authenticated navbar has the plus button titled `Generate questions (G)`. It opens `Generate New Questions`, focuses the prompt textarea, restores `scry-generation-draft`, submits with Generate or Cmd/Ctrl+Enter, closes immediately, clears successful drafts, shows `Generation started`, and updates the active-job badge without blocking review. |
| `/action-inbox` | IQC Action Inbox shows loading, empty state, or action cards. Verify Accept, Reject, selected-card behavior, `J`/`K` navigation, Enter-to-accept, and no stuck pending action. |
| `/tasks` | Background Tasks shows All/Active/Completed/Failed filters, page size, pagination, loading/empty states, job cards, failed/cancelled grouping under Failed, and no misleading active-job counts. |
| `/sign-in` and `/sign-up` | Clerk pages render centered cards. Verify unauthenticated home links, sign-in to sign-up transition, invalid credential messaging, no duplicate error spam, and no unattended Cloudflare challenge on the target host. |
| `/api/health` | `GET` returns JSON with `status: "healthy"` and health headers; `HEAD` returns `X-Health-Status`. Preview smoke tests this before Playwright. |

## Playwright Anchors

- `tests/e2e/agent-review-smoke.test.ts` is the current smoke anchor for `/`
  and `/agent`: redirect, no Sentry/module/image runtime crashes, no legacy
  labels, deterministic chips, and no `/ Review` breadcrumb.
- `tests/e2e/spaced-repetition.test.ts` verifies the generation entry point
  and review page structure as unauthenticated or authenticated.
- `tests/e2e/review-editing.test.ts` covers inline edit, archive undo, and
  keyboard shortcuts `E` and `#` after a review answer exists.
- `tests/e2e/review-next-button-fix.test.ts` protects the Next-button reset
  after incorrect answers, rapid Next clicks, loading transition, and FSRS
  immediate re-review of the same question.
- `tests/e2e/spaced-repetition.local.test.ts` is local-only and includes the
  complete generation-to-review flow plus Mobile Chrome touch-target checks.
- `tests/e2e/library-search-race-condition.test.ts` still covers `/library`
  search race behavior. It is not a primary nav route, but failures can reveal
  stale request handling that also matters for concept-library UX.

## Smoke And Dogfood

For PR previews, `.github/workflows/preview-smoke-test.yml` waits for the
Vercel preview, calls `/api/health`, installs Chromium, then runs
`pnpm test:e2e:smoke` with `PLAYWRIGHT_BASE_URL` set to the preview URL.
If Vercel deployment protection blocks the preview, the workflow reports the
skip rather than proving the app is healthy.

For nightly coverage, `.github/workflows/nightly-e2e.yml` runs unit/integration
coverage and the full Playwright suite against `https://scry.vercel.app`.
Use local `pnpm test:e2e:full` as the human-readable command even though the
workflow currently invokes Playwright directly.

For dogfood, `scripts/qa/dogfood-smoke.sh` requires `agent-browser` and `jq`.
It captures landing, sign-in, invalid login, and sign-up screenshots under
`/tmp/dogfood-scry-<timestamp>/screenshots` and writes a markdown report. A
Cloudflare challenge on sign-up is high severity; browser automation failures
are medium; duplicated unknown-account errors are low unless they block auth.

## Manual QA Protocol

1. Define the feature or route under test and the target base URL. If none is
   provided, use local `pnpm dev`.
2. Run the smallest relevant automated check first: smoke for review/home
   changes, full suite for cross-route or mobile-sensitive changes, dogfood for
   unauthenticated production-like auth checks.
3. Drive the critical route manually. Inspect visible UX, keyboard behavior,
   empty states, loading states, toasts, and recovery paths.
4. Capture console errors, page errors, failed network responses, screenshots,
   and short repro steps. Console warnings are not automatically findings, but
   any unhandled exception, failed Convex/Clerk request, hydration mismatch, or
   blocked health/auth/generation call needs classification.
5. Repeat the affected flow in `Mobile Chrome` when layout, navbar, modal,
   review answer buttons, touch targets, pagination, or any route shell changed.
6. Classify findings by user impact and say whether QA passes, passes with
   non-blocking notes, or fails.

## Severity

- P0: `/` review chat unusable, auth prevents normal entry, generation cannot
  start for signed-in users, `/api/health` unhealthy, or data-loss actions lack
  the expected reverse path.
- P1: Review flow state gets stuck, Next does not reset after feedback, concepts
  bulk actions/pagination/search misbehave, action cards cannot be accepted or
  rejected, task state is misleading, or mobile Chrome blocks a core flow.
- P2: Copy, spacing, non-blocking visual polish, low-risk console noise, or
  dogfood artifacts that do not affect entry, review, generation, or recovery.

## Evidence Handoff

Report:

- Target URL and auth state.
- Commands run, including whether `Mobile Chrome` was covered.
- Routes manually exercised from the critical route table.
- Console/network issues and screenshots/report paths.
- Findings with severity, exact repro, expected behavior, actual behavior, and
  whether each is blocking merge.

Merge handoff must cite the repo brief gate exactly:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

QA does not replace that gate. QA adds browser and UX evidence that the gate
does not see.
