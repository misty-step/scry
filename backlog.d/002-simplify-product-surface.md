# Simplify Product Surface

Priority: high
Status: ready
Estimate: M

## Goal

Pare the app down to three screens — review (home), generate (modal), library — with everything else tucked away or removed. The navbar should have 3-4 items, not 7.

## Non-Goals

- Deleting the concepts library (it's useful, just over-featured)
- Removing IQC/action inbox entirely (keep the URL, remove the nav button)
- Building new features (this is purely reduction)

## Oracle

- [ ] Navbar reduced to: **Generate (+)** | **Library** | **User Menu** — no more separate Tasks, Inbox, AI Review, Settings buttons
- [ ] `/agent` is the home page (`/`) — user lands directly in the review experience
- [ ] `/tasks` page removed from navbar; active generation status shown as inline indicator (toast, badge on generate button, or small status bar) rather than a separate page
- [ ] `/action-inbox` removed from navbar; accessible via `/action-inbox` URL or link in library for power users
- [ ] `/settings` moved into user menu dropdown (already partially there)
- [ ] Concepts library view modes reduced from 6 (All/Due/Thin/Tension/Archived/Trash) to 3: **All** | **Due** | **Archived** (Thin and Tension are power-user concepts that add cognitive load without clear value; Trash folded into Archived with filter)
- [ ] Concepts library sort options reduced from 5 to 2: **Recently Added** | **Due Date**
- [ ] Bulk actions simplified: keep archive and delete, remove other bulk operations if they exist
- [ ] Total navbar component lines reduced (currently 139 lines)
- [ ] `pnpm build:local` succeeds

## Notes

**Current navbar (7 items):** Logo/Home | + Generate | Library | Tasks | Inbox | AI Review | Theme | User Menu

**Target navbar (3-4 items):** Logo (→ review) | + Generate | Library | User Menu (theme + settings inside)

**Library simplification rationale:**
- "Thin" view (concepts with few phrasings) is an internal quality metric, not a user need
- "Tension" view (concepts with conflicting phrasings) is IQC's job, not the user's
- 5 sort options is analysis paralysis for a personal tool
- Users need: "show me everything" and "show me what's due"

**Depends on:** Item 001 (dead code deleted first so we're not simplifying around dead features)
