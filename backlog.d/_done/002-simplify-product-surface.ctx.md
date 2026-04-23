# Context Packet: Simplify Product Surface

## Spec

### Navbar: from 7 items to 4

**Current items (left to right):**
1. "Scry." logo link (`/`)
2. `+` Generate button (opens `GenerationModal`)
3. Library icon (`/concepts`)
4. Tasks icon (`/tasks`) with active-job badge
5. Inbox icon (`/action-inbox`)
6. AI Review icon (`/agent`)
7. ThemeToggle button
8. Clerk `UserButton` (user menu)

**Target items:**
1. "Scry." logo link (`/`) -- unchanged, but now lands on agentic review
2. `+` Generate button -- unchanged, but add active-job badge here (migrated from Tasks icon)
3. Library icon (`/concepts`) -- unchanged
4. Clerk `UserButton` -- add custom menu items: Theme toggle, Settings link

**Removed from navbar:**
- Tasks icon (`/tasks`) -- replace with badge on Generate button when `activeCount > 0`
- Inbox icon (`/action-inbox`) -- URL still works, just no nav entry
- AI Review icon (`/agent`) -- becomes the home page, no need for a separate button
- ThemeToggle standalone button -- moves into UserButton dropdown

### Home page: `/agent` becomes `/`

**Current state:** `app/page.tsx` renders `ReviewFlow` (the old card-based review).
`app/agent/page.tsx` renders `ReviewChat` (the agentic chat-based review).

**Target:** `app/page.tsx` renders `ReviewChat` (the content currently at `/agent`).
`app/agent/page.tsx` becomes a redirect to `/` (for bookmarks/links).

The old `ReviewFlow` component is NOT deleted -- it stays in the codebase as a fallback.
The home page for unauthenticated users still shows `SignInLanding`.

### Library view simplification

**Current 6 view tabs:** All | Due | Thin | Tension | Archived | Trash

**Target 4 view tabs:** All | Due | Archived | Trash

Thin and Tension are removed from the UI tabs. The backend `listForLibrary` query
still accepts `'thin'` and `'tension'` values (no backend change needed -- IQC system
still uses these internally). Only the frontend tab array and its associated
descriptions/empty-state messages lose these entries.

Trash stays as a separate tab (not folded into Archived). The backlog item suggested
folding Trash into Archived, but these represent fundamentally different lifecycle
states (paused vs pending-deletion) with different available actions (unarchive vs
restore). Merging them would add a filter control that offsets the tab reduction.
Keeping both is simpler. This is 4 tabs, not 3 -- still a meaningful reduction from 6.

**Sort options: already at 2.** The backlog item says "reduce from 5 to 2" but the
codebase already has exactly 2 sort options: "Next review" and "Recently created".
The `ConceptsSort` type is `'recent' | 'nextReview'`. No sort changes needed.

**Bulk actions: already minimal.** The available actions are context-dependent:
- Active views: archive, delete
- Archived view: unarchive, delete
- Trash view: restore

This is already the simplest correct set. No bulk action changes needed.

### Settings in User Menu

Clerk's `<UserButton>` supports custom menu items via the `userProfileMode` prop and
JSX children (`<UserButton.MenuItems>`). Add a "Settings" link and a theme toggle
as custom menu items inside the UserButton.

## Current State

### Navbar (`components/navbar.tsx`, 139 lines)

```
Scry. | [+] | Library | Tasks(badge) | Inbox | AI Review | ThemeToggle | UserButton
```

- Generate button listens for `open-generation-modal` custom event (keyboard shortcut `G`)
- Tasks badge shows `activeCount` from `useActiveJobs()` hook
- All nav icons are `size-9 rounded-full` with active-state highlighting via `pathname`
- ThemeToggle is a standalone `Button` component
- UserButton uses `useClerkAppearance()` for theme-matched styling
- `GenerationModal` dispatches `start-review-after-generation` event when `pathname === '/'`

### Library views (`app/concepts/_components/concepts-client.tsx`, 441 lines)

- `VIEW_TABS` array: 6 entries (all, due, thin, tension, archived, deleted)
- `VIEW_DESCRIPTIONS` record: 6 entries with title/body pairs
- `ConceptsView` type in `hooks/use-concepts-query.ts`: `'all' | 'due' | 'thin' | 'tension' | 'archived' | 'deleted'`
- `ConceptLibraryView` type in `convex/lib/conceptHelpers.ts`: same 6 values
- `matchesConceptView()` in `convex/lib/conceptHelpers.ts`: handles all 6 views
- `ConceptsEmptyState` in `components/concepts/concepts-empty-state.tsx`: `VIEW_MESSAGES` record with 6 entries
- `ConceptsTable` in `components/concepts/concepts-table.tsx`: renders Thin/Tension badges inline on rows (keep these -- they are per-row signals, not view filters)
- `ViewSelector` in `components/concepts/view-selector.tsx`: generic, driven by `options` prop -- no changes needed
- Sort: only 2 options (`nextReview`, `recent`) -- already at target
- Bulk actions: already minimal per-view set -- no changes needed

### Agent page (`app/agent/page.tsx`, 21 lines)

- Renders `ReviewChat` from `components/agent/review-chat.tsx`
- Full-height layout: `h-[calc(100dvh-var(--navbar-height))]`
- Footer is hidden when `pathname.startsWith('/agent')` (see `components/footer.tsx:16`)

### Home page (`app/page.tsx`, 34 lines)

- Renders `ReviewFlow` (old card-based review) wrapped in `ReviewErrorBoundary`
- Shows `SignInLanding` for unauthenticated users

### Settings page (`app/settings/settings-client.tsx`)

- Contains: Security Settings info, Danger Zone (delete account)
- Minimal content -- appropriate for a user-menu link rather than a top-level nav item

### Keyboard shortcuts (`hooks/use-keyboard-shortcuts.ts`)

Global shortcuts affected:
- `h` -> "Go to home/review" (`router.push('/')`) -- still correct after change
- `c` -> "Go to concepts" (`router.push('/concepts')`) -- unchanged
- `Ctrl+S` -> "Go to settings" (`router.push('/settings')`) -- still works, settings page still exists
- `g` -> "Generate new questions" -- unchanged
- `?` -> Toggle help -- unchanged
- `Escape` -> Close modals -- unchanged

No shortcuts reference `/tasks`, `/action-inbox`, or `/agent` directly.

### E2E tests

- `tests/e2e/agent-review-smoke.test.ts` navigates to `/agent` (lines 10, 37, 47) -- must update to `/`

### Redirects (`next.config.ts`)

Current redirects: `/questions` -> `/concepts`, `/library` -> `/concepts`.
Need to add: `/agent` -> `/` (permanent redirect for bookmarks).

## Design Decisions

### Why these views survive

**All:** The default "show me everything" view. Universal need.

**Due:** The core learning action -- "what needs review now." This is the single most
important filter in a spaced repetition app.

**Archived:** Users need to see and recover paused concepts. Removing this forces
browsing-by-URL, which is unacceptable.

**Trash:** Users need to see and recover deleted concepts before permanent removal.
Same rationale as Archived.

### Why these views are removed

**Thin:** "Concepts with few phrasings" is an internal quality metric. Users don't think
in terms of phrasing coverage -- that is IQC's job. The Thin badge still appears inline
on concept rows in the table, so the signal is not lost; it just stops being a top-level
navigation concept.

**Tension:** "Concepts with conflicting phrasings" is even more of an IQC concern. The
Tension badge likewise remains on individual rows. The action-inbox is the proper home
for resolution workflows.

### Why Tasks moves to a badge

The Tasks page (`/tasks`) is a monitoring view for background generation jobs. Users
rarely need to visit it proactively -- they need to know "something is running" and
"something finished." A badge on the Generate button (the origin of those jobs)
communicates this more naturally than a separate page. The `/tasks` page still exists
for detailed inspection.

### Why theme moves into UserButton

Theme toggle is used infrequently (once per session, if that). It does not warrant
permanent navbar real estate. Clerk's UserButton dropdown already contains account
actions; adding theme there is natural.

### UX research alignment

Modern minimal SRS apps (MintDeck, Mochi, Flashrecall) converge on the same pattern:
the home screen IS the review session. Navigation is minimal -- typically just
"review" and "library/decks". Generation/creation is a modal or secondary action, not
a separate page. Settings and account live in a user avatar menu. This matches the
target design exactly.

## Implementation Sequence

### Phase 1: Home page swap (highest risk, do first)

1. **`app/page.tsx`**: Replace `ReviewFlow` with `ReviewChat` for authenticated users.
   Keep `SignInLanding` for unauthenticated users. Wrap in the same
   `h-[calc(100dvh-var(--navbar-height))]` container the agent page uses.

2. **`app/agent/page.tsx`**: Replace component with a redirect to `/`.
   Use `import { redirect } from 'next/navigation'; redirect('/');` or add to
   `next.config.ts` redirects.

3. **`next.config.ts`**: Add permanent redirect `/agent` -> `/`.

4. **`components/footer.tsx`**: Change `pathname.startsWith('/agent')` to
   `pathname === '/'` (or remove the condition entirely if the agentic review
   layout doesn't need a footer at all -- the current ReviewChat is full-height
   and the footer was already hidden for `/agent`).

5. **`components/navbar.tsx`**: Update the `GenerationModal`'s `onGenerationSuccess`
   callback -- currently checks `pathname === '/'` to dispatch
   `start-review-after-generation`. The ReviewChat component may need to listen
   for this event, or the dispatch can be made unconditional.

6. **`tests/e2e/agent-review-smoke.test.ts`**: Change `page.goto('/agent')` to
   `page.goto('/')`.

### Phase 2: Navbar simplification

7. **`components/navbar.tsx`**: Remove the Tasks `<Link>` (lines 73-89). Move the
   active-job badge to the Generate `<Button>` (the `+` icon). The badge rendering
   logic is identical -- just relocate the JSX.

8. **`components/navbar.tsx`**: Remove the Action Inbox `<Link>` (lines 90-101).

9. **`components/navbar.tsx`**: Remove the AI Review `<Link>` (lines 102-113).

10. **`components/navbar.tsx`**: Remove the `<ThemeToggle />` standalone component
    (line 114). Remove the `ThemeToggle` import.

11. **`components/navbar.tsx`**: Add custom menu items to `<UserButton>`:
    ```tsx
    <UserButton afterSignOutUrl="/" appearance={clerkAppearance}>
      <UserButton.MenuItems>
        <UserButton.Link
          label="Settings"
          labelIcon={<Settings className="h-4 w-4" />}
          href="/settings"
        />
        <UserButton.Action label="Theme" labelIcon={<Sun className="h-4 w-4" />}>
          {/* Custom theme toggle action */}
        </UserButton.Action>
      </UserButton.MenuItems>
    </UserButton>
    ```
    NOTE: Clerk's `UserButton.Action` supports custom components. If the theme
    toggle interaction is too complex for Clerk's menu item API, an alternative is
    to create a thin wrapper that calls `setTheme()` on click and shows the
    current theme icon. Research the exact Clerk v5/v6 `<UserButton.MenuItems>` API
    at build time -- the interface may use `<UserButton.UserProfilePage>` or
    `<UserButton.Action>` depending on the installed version.

12. Remove now-unused imports from `navbar.tsx`: `Inbox`, `ListChecks`, `Sparkles`
    from lucide-react. Remove `ThemeToggle` import.

### Phase 3: Library view reduction

13. **`app/concepts/_components/concepts-client.tsx`**: Remove `thin` and `tension`
    entries from `VIEW_TABS` array (lines 33-34). Remove their entries from
    `VIEW_DESCRIPTIONS` (lines 48-55).

14. **`hooks/use-concepts-query.ts`**: Narrow `ConceptsView` type to
    `'all' | 'due' | 'archived' | 'deleted'`. This is a type-only change on the
    frontend; the backend type `ConceptLibraryView` stays unchanged.

15. **`components/concepts/concepts-empty-state.tsx`**: Remove `thin` and `tension`
    entries from `VIEW_MESSAGES` record (lines 16-17).

16. **`convex/lib/conceptHelpers.ts`**: No changes. `matchesConceptView` still
    handles `thin` and `tension` for backend/IQC use.

17. **`components/concepts/concepts-table.tsx`**: No changes. Thin and Tension
    badges still render inline on individual concept rows.

### Phase 4: Cleanup

18. Verify `pnpm build:local` succeeds.
19. Verify `pnpm test` passes (update any snapshots or unit tests that reference
    removed navbar items or view tabs).
20. Check for any remaining imports of `ThemeToggle` in `navbar.tsx` or dead
    references to removed nav items.

## Risks

### Keyboard shortcuts
- No shortcuts directly reference `/tasks`, `/action-inbox`, or `/agent`. Safe.
- `Ctrl+S` still navigates to `/settings` which still exists. Safe.
- `h` navigates to `/` which now shows ReviewChat instead of ReviewFlow. Correct behavior.
- The `s` shortcut in review context means "skip question" -- conflicts with global `s`
  only when NOT in review mode. No change needed.

### Deep links that would break
- `/agent` -- needs permanent redirect to `/` in `next.config.ts`
- `/tasks` -- page still exists, just no navbar entry. No breakage.
- `/action-inbox` -- page still exists, just no navbar entry. No breakage.
- `/settings` -- page still exists, accessible via user menu. No breakage.
- Concepts deep links (`/concepts/[id]`) -- unchanged.

### Footer behavior
- Footer is currently hidden when `pathname.startsWith('/agent')`. After the swap,
  it needs to be hidden when `pathname === '/'` (since home is now the agentic review).
  If the condition is wrong, a footer will appear under the full-height chat layout.

### GenerationModal post-generation event
- Currently dispatches `start-review-after-generation` when `pathname === '/'`.
  ReviewChat may or may not listen for this event. Verify and wire up if needed,
  or remove the dispatch if ReviewChat auto-refreshes via Convex reactivity.

### Clerk UserButton API surface
- The exact API for adding custom menu items varies between Clerk versions.
  Clerk v5+ uses `<UserButton.MenuItems>` with `<UserButton.Link>` and
  `<UserButton.Action>`. Verify the installed Clerk version supports this pattern.
  Fallback: build a custom dropdown that wraps the UserButton.

### Active job badge relocation
- The badge currently uses absolute positioning (`absolute -top-1 -right-1`).
  The Generate button also uses `size-9 rounded-full`. The badge should transfer
  cleanly, but verify it doesn't clip or overlap incorrectly.

### E2E tests
- `tests/e2e/agent-review-smoke.test.ts` has 3 navigations to `/agent` that must
  change to `/`.

## Verification

1. **Build gate:** `pnpm build:local` succeeds with zero errors.
2. **Test gate:** `pnpm test` passes. `pnpm test:contract` passes.
3. **Visual: Navbar** has exactly 4 items: Scry. logo, Generate (+) with badge,
   Library, UserButton. No Tasks, Inbox, AI Review, or standalone theme toggle.
4. **Visual: Home page** shows the agentic review chat (ReviewChat), not the old
   card-based ReviewFlow.
5. **Navigation: `/agent`** redirects to `/`.
6. **Navigation: `/tasks`** still loads the tasks page (no navbar entry).
7. **Navigation: `/action-inbox`** still loads the inbox page (no navbar entry).
8. **Navigation: `/settings`** still loads the settings page. Accessible via
   UserButton dropdown.
9. **UserButton dropdown** contains: Settings link, Theme toggle, Sign out.
10. **Library tabs** show: All | Due | Archived | Trash. No Thin or Tension tabs.
11. **Library table rows** still show Thin and Tension badges inline.
12. **Generate button badge** appears when background jobs are active.
13. **Keyboard shortcut `h`** navigates to `/` and shows ReviewChat.
14. **Keyboard shortcut `g`** opens the generation modal.
15. **Keyboard shortcut `?`** shows help dialog with correct shortcut descriptions.
16. **Footer** is hidden on the home page (agentic review is full-height).
17. **Unauthenticated users** see `SignInLanding` at `/`, not the review chat.
18. **Mobile:** Navbar fits comfortably with 4 items. Library view tabs scroll
    horizontally with 4 pills.

## Files Touched

| File | Change |
|------|--------|
| `app/page.tsx` | Swap ReviewFlow for ReviewChat |
| `app/agent/page.tsx` | Replace with redirect to `/` |
| `next.config.ts` | Add `/agent` -> `/` redirect |
| `components/navbar.tsx` | Remove 4 items, add badge to Generate, add UserButton children |
| `components/footer.tsx` | Update pathname check for footer hiding |
| `app/concepts/_components/concepts-client.tsx` | Remove thin/tension from VIEW_TABS and VIEW_DESCRIPTIONS |
| `hooks/use-concepts-query.ts` | Narrow ConceptsView type |
| `components/concepts/concepts-empty-state.tsx` | Remove thin/tension from VIEW_MESSAGES |
| `tests/e2e/agent-review-smoke.test.ts` | Update `/agent` navigations to `/` |

## Not Touched (Explicitly)

| File | Reason |
|------|--------|
| `convex/lib/conceptHelpers.ts` | Backend types/logic unchanged; IQC still uses thin/tension |
| `convex/concepts.ts` | Backend query still accepts all 6 views |
| `components/concepts/concepts-table.tsx` | Thin/Tension badges stay on individual rows |
| `components/concepts/view-selector.tsx` | Generic component, driven by props |
| `components/concepts/bulk-action-bar.tsx` | Already minimal |
| `components/review-flow.tsx` | Kept in codebase as fallback, just not rendered at `/` |
| `hooks/use-keyboard-shortcuts.ts` | No shortcuts reference removed routes |
| `app/tasks/page.tsx` | Page stays, just loses navbar entry |
| `app/action-inbox/page.tsx` | Page stays, just loses navbar entry |
| `app/settings/settings-client.tsx` | Page stays, accessible via user menu |
