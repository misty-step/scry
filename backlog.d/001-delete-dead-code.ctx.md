# Context Packet: Delete Dead Code

## Spec

Delete ~6,800 lines of dead code in two categories:

1. **Dev-only routes** (4 route directories + supporting components, types, libs, APIs, Convex module, script)
2. **Legacy review flow** (the card-based ReviewFlow at `/` — replaced by agentic chat at `/agent`)

After deletion, `app/page.tsx` redirects to `/agent`. The agentic review experience is untouched.

**Decision: redirect vs inline render.** Use a Next.js server-side redirect (`redirect('/agent')` from `next/navigation`) in `app/page.tsx`. This is simpler than duplicating the agentic render, avoids importing Convex `Authenticated`/`Unauthenticated` into the root page, and preserves `/agent` as the single source of truth. Keep the `SignInLanding` for unauthenticated users — the redirect should only fire for authenticated users, or alternatively just always redirect and let `/agent` handle the unauthenticated case (it already does).

Simplest implementation: make `app/page.tsx` a server component that calls `redirect('/agent')`. The `/agent` page already handles unauthenticated users with a "Sign in to start reviewing" message. If a dedicated landing page is desired for unauthenticated users at `/`, keep the client component with `useUser` check and only redirect authenticated users.

**Recommendation:** Always redirect. The `/agent` page handles unauth. If a marketing landing page is needed later, it can be added back. Delete `components/sign-in-landing.tsx` as well since it becomes unreachable.

## File Inventory

### Files to DELETE (pure deletion)

#### Dev-Only Route: `/app/lab/` (1,205 lines)
| File | Lines |
|------|-------|
| `app/lab/page.tsx` | 25 |
| `app/lab/_components/unified-lab-client.tsx` | 684 |
| `app/lab/configs/page.tsx` | 25 |
| `app/lab/configs/_components/config-manager-page.tsx` | 471 |

#### Dev-Only Route: `/app/evolve/` (898 lines)
| File | Lines |
|------|-------|
| `app/evolve/page.tsx` | 28 |
| `app/evolve/_components/evolve-dashboard.tsx` | 119 |
| `app/evolve/_components/experiment-card.tsx` | 315 |
| `app/evolve/_components/test-results-table.tsx` | 140 |
| `app/evolve/_components/run-config-panel.tsx` | 112 |
| `app/evolve/_components/prompt-diff.tsx` | 97 |
| `app/evolve/_components/variants-selector.tsx` | 87 |

#### Dev-Only Route: `/app/design-lab/` (418 lines)
| File | Lines |
|------|-------|
| `app/design-lab/page.tsx` | 22 |
| `app/design-lab/_components/landing-tuner.tsx` | 290 |
| `app/design-lab/_components/landing-preview.tsx` | 106 |

#### Dev-Only Route: `/app/test-error/` (167 lines)
| File | Lines |
|------|-------|
| `app/test-error/page.tsx` | 167 |

#### Evolve API Routes (155 lines)
| File | Lines |
|------|-------|
| `app/api/evolve/experiments/route.ts` | 83 |
| `app/api/evolve/experiments/[id]/route.ts` | 72 |

#### Lab Supporting Components: `components/lab/` (1,820 lines)
| File | Lines |
|------|-------|
| `components/lab/results-grid.tsx` | 479 |
| `components/lab/config-management-dialog.tsx` | 443 |
| `components/lab/config-editor.tsx` | 373 |
| `components/lab/config-manager.tsx` | 315 |
| `components/lab/input-manager.tsx` | 210 |

#### Lab/Evolve Supporting Types, Libs, Convex (1,837 lines)
| File | Lines |
|------|-------|
| `scripts/evolve-prompts.ts` | 484 |
| `convex/lab.ts` | 328 |
| `lib/lab-storage.ts` | 160 |
| `lib/lab-storage.test.ts` | 330 |
| `types/evolve.ts` | 107 |
| `types/lab.ts` | 161 |
| `types/lab.test.ts` | 267 |

#### Legacy Review Flow: Root Components (802 lines)
| File | Lines |
|------|-------|
| `components/review-flow.tsx` | 119 |
| `components/review-flow.test.tsx.skip` | 425 |
| `components/review-phrasing-display.tsx` | 258 |

#### Legacy Review Flow: `components/review/` (2,430 lines — entire directory)
| File | Lines |
|------|-------|
| `components/review/session-context.tsx` | 476 |
| `components/review/phrasing-edit-form.tsx` | 252 |
| `components/review/review-error-boundary.tsx` | 215 |
| `components/review/session-context.test.tsx` | 189 |
| `components/review/answer-feedback.tsx` | 167 |
| `components/review/review-session-test-utils.tsx` | 131 |
| `components/review/options-editor.tsx` | 116 |
| `components/review/review-empty-state.tsx` | 111 |
| `components/review/review-actions-dropdown.test.tsx` | 83 |
| `components/review/review-actions-dropdown.tsx` | 80 |
| `components/review/question-display.tsx` | 71 |
| `components/review/answer-feedback.test.tsx` | 69 |
| `components/review/review-mode.tsx` | 60 |
| `components/review/keyboard-handler.tsx` | 50 |
| `components/review/review-complete-state.tsx` | 49 |
| `components/review/keyboard-handler.test.tsx` | 48 |
| `components/review/inline-editor.tsx` | 46 |
| `components/review/inline-editor.test.tsx` | 43 |
| `components/review/learning-mode-explainer.tsx` | 42 |
| `components/review/true-false-editor.tsx` | 38 |
| `components/review/question-display.test.tsx` | 36 |
| `components/review/index.tsx` | 29 |
| `components/review/review-due-count.tsx` | 17 |
| `components/review/review-due-count.test.tsx` | 12 |

#### Orphaned Hooks After Legacy Flow Deletion (2,274 lines)
| File | Lines |
|------|-------|
| `hooks/use-unified-edit.test.ts` | 570 |
| `hooks/use-review-flow.ts` | 473 |
| `hooks/use-quiz-interactions.test.ts` | 322 |
| `hooks/use-review-flow.test.ts` | 337 |
| `hooks/use-unified-edit.ts` | 270 |
| `hooks/use-instant-feedback.test.ts` | 96 |
| `hooks/use-shuffled-options.test.ts` | 61 |
| `hooks/use-quiz-interactions.ts` | 57 |
| `hooks/use-instant-feedback.ts` | 50 |
| `hooks/use-shuffled-options.ts` | 18 |
| `lib/unified-edit-validation.ts` | 133 |
| `lib/unified-edit-validation.test.ts` | 295 |

Note: `hooks/use-shuffled-options` is only used by `review-phrasing-display.tsx` (deleted).

#### Orphaned Lib/Utility Files (248 lines)
| File | Lines |
|------|-------|
| `lib/format-review-time.ts` | 101 |
| `lib/format-review-time.test.ts` | 147 |

Note: Only imported by `review-empty-state.tsx`. The `empty-states.tsx` component defines its own local `formatNextReviewTime`.

#### Other Dead Files (405 lines)
| File | Lines |
|------|-------|
| `components/generation-modal.test.tsx.skip` | 294 |
| `components/quiz-generation-skeleton.tsx` | 111 |

Note: `generation-modal.test.tsx.skip` tests dead code patterns. `quiz-generation-skeleton.tsx` is never imported anywhere (duplicate of component in `loading-skeletons.tsx`).

#### Potentially Dead: Sign-In Landing (58 lines)
| File | Lines |
|------|-------|
| `components/sign-in-landing.tsx` | 58 |

Only imported by `app/page.tsx`. If page becomes a redirect, this is dead.

### Files to MODIFY

#### `app/page.tsx` (34 lines -> ~5 lines)
Replace legacy ReviewFlow render with redirect to `/agent`.
```tsx
import { redirect } from 'next/navigation';

export default function Home() {
  redirect('/agent');
}
```

#### `middleware.ts` (27 lines -> ~21 lines)
Remove the `isDesignLabRoute` matcher and its production-blocking logic (lines 4, 8-11).

#### `components/analytics-wrapper.tsx`
Remove the `/test-error` exclusion from the `beforeSend` filter (line 25).

#### `components/navbar.tsx` (140 lines)
Remove the `start-review-after-generation` event dispatch in `onGenerationSuccess` (lines 131-135). After the redirect, `pathname === '/'` will never be true for authenticated users. Clean up: simplify to just close the modal.

#### `hooks/use-keyboard-shortcuts.ts` (274 lines -> ~114 lines)
Remove the `useReviewShortcuts` export (lines 116-274). Keep `useKeyboardShortcuts` and `ShortcutDefinition` — they're used by `footer.tsx`, `use-action-cards.ts`, and `keyboard-shortcuts-help.tsx`.

#### `hooks/use-keyboard-shortcuts.test.ts` (518 lines -> reduced)
Remove all `useReviewShortcuts` test cases (the `describe('useReviewShortcuts', ...)` block starting at line 272, plus any `useReviewShortcuts` imports/usage in earlier tests around lines 91, 110).

#### `components/ui/loading-skeletons.tsx`
Remove the `QuizFlowSkeleton` export (lines 196-213) and its internal `QuizGenerationSkeleton` usage. Keep all other skeletons — they're used by other pages.

#### `vitest.config.ts`
Remove two coverage exclusion entries:
- `'convex/lab.ts'` (line 65) — file no longer exists
- `'hooks/use-review-flow.ts'` (line 92) — file no longer exists

#### `package.json`
Remove the `"evolve"` script entry (line ~53).

#### `docs/CODEBASE_MAP.md`
Remove references to deleted files:
- `components/review-flow.tsx` (line 99)
- `components/generation-modal.tsx` line if it references legacy patterns
- `hooks/use-review-flow.ts` (line 101)

## Dependency Check

### Components shared between legacy and agentic -- DO NOT DELETE
| Component | Used By Legacy | Used By Agentic/Other |
|-----------|---------------|----------------------|
| `components/generation-modal.tsx` | Indirectly (via review-empty-state event) | `navbar.tsx` (primary consumer) |
| `components/page-container.tsx` | `review-flow.tsx`, `review-empty-state.tsx` | `tasks-client`, `concepts-client`, `settings-client`, etc. |
| `components/ui/loading-skeletons.tsx` (non-QuizFlow parts) | N/A | Multiple pages |
| `components/ui/live-region.tsx` | `review-flow.tsx` | Generic utility, keep |
| `hooks/use-concept-actions.ts` | `session-context.tsx` | `concept-detail-client.tsx` |
| `hooks/use-keyboard-shortcuts.ts` (`useKeyboardShortcuts` only) | `keyboard-handler.tsx` | `footer.tsx`, `use-action-cards.ts` |
| `components/keyboard-shortcuts-help.tsx` | N/A | Uses `ShortcutDefinition` type |

### Orphaned after deletion (safe to delete)
| Item | Only Consumer(s) |
|------|-----------------|
| `hooks/use-review-flow.ts` + test | `session-context.tsx`, `review-mode.tsx` |
| `hooks/use-quiz-interactions.ts` + test | `session-context.tsx` |
| `hooks/use-unified-edit.ts` + test | `session-context.tsx` |
| `hooks/use-instant-feedback.ts` + test | `session-context.tsx` |
| `hooks/use-shuffled-options.ts` + test | `review-phrasing-display.tsx` |
| `lib/unified-edit-validation.ts` + test | `use-unified-edit.ts`, `session-context.tsx` |
| `lib/format-review-time.ts` + test | `review-empty-state.tsx` |
| `components/sign-in-landing.tsx` | `app/page.tsx` |
| `components/quiz-generation-skeleton.tsx` | Never imported |

## Implementation Sequence

1. **Delete dev-only route directories** (atomic, no cross-dependencies):
   ```
   rm -rf app/lab/ app/evolve/ app/design-lab/ app/test-error/
   rm -rf app/api/evolve/
   rm -rf components/lab/
   rm types/evolve.ts types/lab.ts types/lab.test.ts
   rm lib/lab-storage.ts lib/lab-storage.test.ts
   rm convex/lab.ts
   rm scripts/evolve-prompts.ts
   ```

2. **Delete legacy review flow** (entire directory + root components):
   ```
   rm -rf components/review/
   rm components/review-flow.tsx
   rm components/review-flow.test.tsx.skip
   rm components/review-phrasing-display.tsx
   rm components/generation-modal.test.tsx.skip
   rm components/quiz-generation-skeleton.tsx
   rm components/sign-in-landing.tsx
   ```

3. **Delete orphaned hooks and libs**:
   ```
   rm hooks/use-review-flow.ts hooks/use-review-flow.test.ts
   rm hooks/use-quiz-interactions.ts hooks/use-quiz-interactions.test.ts
   rm hooks/use-unified-edit.ts hooks/use-unified-edit.test.ts
   rm hooks/use-instant-feedback.ts hooks/use-instant-feedback.test.ts
   rm hooks/use-shuffled-options.ts hooks/use-shuffled-options.test.ts
   rm lib/unified-edit-validation.ts lib/unified-edit-validation.test.ts
   rm lib/format-review-time.ts lib/format-review-time.test.ts
   ```

4. **Rewrite `app/page.tsx`** to a simple redirect:
   ```tsx
   import { redirect } from 'next/navigation';
   export default function Home() {
     redirect('/agent');
   }
   ```

5. **Modify `middleware.ts`**: Remove `isDesignLabRoute` and its production block.

6. **Modify `components/analytics-wrapper.tsx`**: Remove `/test-error` line from beforeSend filter.

7. **Modify `components/navbar.tsx`**: Remove `start-review-after-generation` dispatch logic from `onGenerationSuccess`. Simplify to no-op or remove the callback entirely.

8. **Modify `hooks/use-keyboard-shortcuts.ts`**: Delete the entire `useReviewShortcuts` function (lines 116-274).

9. **Modify `hooks/use-keyboard-shortcuts.test.ts`**: Remove all `useReviewShortcuts` imports and test blocks.

10. **Modify `components/ui/loading-skeletons.tsx`**: Remove the `QuizFlowSkeleton` function export.

11. **Modify `vitest.config.ts`**: Remove `'convex/lab.ts'` and `'hooks/use-review-flow.ts'` from coverage exclusions.

12. **Modify `package.json`**: Remove `"evolve"` script.

13. **Modify `docs/CODEBASE_MAP.md`**: Remove references to deleted files.

14. **Run `npx convex dev`** (or let it run) to regenerate `convex/_generated/api.d.ts` after deleting `convex/lab.ts`. Commit the regenerated file.

15. **Verify**: `pnpm build:local && pnpm test`

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Bookmarks/links to `/` break | Low — redirect handles it | Redirect preserves the URL pattern |
| `convex/_generated/api.d.ts` stale after `convex/lab.ts` deletion | Build failure | Run `npx convex dev` to regenerate, commit result |
| Navbar "Scry." logo links to `/` which now redirects | Minor UX — double navigation | Consider updating navbar `href="/"` to `href="/agent"` |
| `docs/` references to deleted files become stale | Low — docs, not code | Update CODEBASE_MAP.md; other docs in `docs/archive/` are inherently archival |
| `generation-modal.test.tsx.skip` deletion removes test coverage | None — it was already skipped/broken | The modal itself is kept and working |
| Some keyboard shortcut tests reference review flow | Test failures | Step 9 cleans these up |

## Verification

```bash
# 1. Build succeeds
pnpm build:local

# 2. Tests pass
pnpm test

# 3. No dangling imports (grep for deleted module names)
grep -r "review-flow\|session-context\|review-phrasing\|use-review-flow\|use-quiz-interactions\|use-unified-edit\|use-instant-feedback\|use-shuffled-options\|unified-edit-validation\|format-review-time\|quiz-generation-skeleton\|sign-in-landing\|review-error-boundary\|review-empty-state\|review-complete-state\|review-mode\|review-due-count\|review-actions-dropdown\|answer-feedback\|inline-editor\|keyboard-handler\|question-display\|learning-mode-explainer\|phrasing-edit-form\|lab-storage\|types/lab\|types/evolve\|convex/lab\|evolve-prompts" --include="*.ts" --include="*.tsx" -l . | grep -v node_modules | grep -v .pi/ | grep -v docs/

# 4. Deleted directories are gone
test ! -d app/lab && test ! -d app/evolve && test ! -d app/design-lab && test ! -d app/test-error && test ! -d components/review && test ! -d components/lab && test ! -d app/api/evolve && echo "All dead directories removed"

# 5. Redirect works (manual: navigate to / in browser, should land on /agent)

# 6. TypeScript strict check
pnpm tsc --noEmit
```

## Line Count Summary

| Category | Lines Deleted |
|----------|-------------|
| Dev-only routes (`app/lab`, `app/evolve`, `app/design-lab`, `app/test-error`) | 2,688 |
| Evolve API routes | 155 |
| Lab components (`components/lab/`) | 1,820 |
| Lab/evolve types, libs, convex, scripts | 1,837 |
| Legacy review flow (`components/review/`, `review-flow.tsx`, etc.) | 3,232 |
| Orphaned hooks + tests | 2,274 |
| Orphaned libs | 676 |
| Other dead files | 463 |
| **Total deleted** | **~13,145** |
| Files modified (net reduction) | ~350 |
| **Estimated net reduction** | **~12,800 lines** |
