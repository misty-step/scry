# Simplify Product Surface

Priority: high
Status: in-progress
Estimate: M

## Goal

Pare the app down to three screens — review (home), generate (modal), library — with everything else tucked away or removed. The navbar should have 3-4 items, not 7.

## Non-Goals

- Deleting the concepts library (it's useful, just over-featured)
- Removing IQC/action inbox entirely (keep the URL, remove the nav button)
- Building new features (this is purely reduction)

## Oracle

- [x] Navbar reduced to: **Generate (+)** | **Library** | **User Menu** — no more separate Tasks, Inbox, AI Review, Settings buttons
- [x] `/` is the home page and `/agent` permanently redirects to `/`
- [x] `/tasks` page removed from navbar; active generation status shown as inline indicator on the Generate button and in generation toasts
- [x] `/action-inbox` removed from navbar; route remains available at `/action-inbox`
- [x] `/settings` moved into the user menu
- [x] Concepts library view modes reduced from 6 to 4: **All** | **Due** | **Archived** | **Trash**. Thin and Tension were removed as top-level views; Trash stayed separate to preserve delete/restore semantics without adding a new filter
- [x] Concepts library sort options already matched the target 2-option surface: **Recently created** | **Next review**
- [x] Bulk actions already matched the reduced surface: archive/delete for active items, unarchive/delete for archived, restore for trash
- [x] Total navbar component lines reduced from 139 to 116
- [ ] `pnpm build:local` succeeds (not run; repo policy requires explicit approval before this command)

## Notes

**Current navbar (7 items):** Logo/Home | + Generate | Library | Tasks | Inbox | AI Review | Theme | User Menu

**Target navbar (3-4 items):** Logo (→ review) | + Generate | Library | User Menu (theme + settings inside)

**Library simplification rationale:**
- "Thin" view (concepts with few phrasings) is an internal quality metric, not a user need
- "Tension" view (concepts with conflicting phrasings) is IQC's job, not the user's
- 5 sort options is analysis paralysis for a personal tool
- Users need: "show me everything" and "show me what's due"

**Depends on:** Item 001 (dead code deleted first so we're not simplifying around dead features)

## What Was Built

- `/` now renders the agent review experience directly, and `/agent` issues a permanent redirect to `/`
- Navbar reduced to logo, Generate, Library, and User menu; the active-job badge moved onto Generate
- Settings and theme controls moved into the user menu, and the standalone theme-toggle component was deleted
- Library tabs now expose only All, Due, Archived, and Trash while preserving thin/tension as row-level signals
- Legacy `/agent` routing, navbar state, and smoke coverage were updated to the new home-route behavior

## Workarounds

- `pnpm build:local` remains intentionally unverified in this pass because repository policy marks it as approval-gated
