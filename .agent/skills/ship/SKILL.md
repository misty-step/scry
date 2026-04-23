---
name: ship
description: |
  Final mile for scry. Land a merge-ready backlog branch, archive the
  shipped backlog files into backlog.d/_done/ before squash, preserve
  backlog trailers in the squash commit, pull master, invoke /reflect,
  and route any reflect harness edits to harness/reflect-outputs.
  Assumes /settle already proved the branch merge-ready against scry's
  Quality Checks merge-gate; /ship does not refactor, review, or run a
  fresh CI campaign.
  Use when: "ship it", "merge and close out", "land this scry ticket",
  "finish this backlog item", "merge and reflect".
  Trigger: /ship.
argument-hint: "[branch-or-pr]"
---

# /ship

The final mile for scry. The branch is already settled; `/ship` lands it,
archives the file-driven backlog ticket, preserves closure trailers,
syncs master, runs `/reflect`, and keeps any harness-learning edits off
master.

scry's gate statement from `.spellbook/repo-brief.md` is authoritative:

```text
The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.
```

## Stance

1. **Act within the final-mile boundary.** Archive, merge, pull, reflect,
   and apply reflect outputs. If the branch is not settled against the
   scry gate, refuse and send it back to `/settle`.
2. **Never lose backlog trailers.** `/groom` closes work by trailers on
   master. The squash commit must preserve `Closes-backlog:`,
   `Ships-backlog:`, and `Refs-backlog:` values as bare numeric IDs.
3. **Archive before squash.** Move `backlog.d/<id>-*.md` into
   `backlog.d/_done/` on the shipping branch, commit that move, then
   squash. Closure is one master commit, not a post-merge cleanup.
4. **Keep scry's release boundary intact.** `.github/workflows/release.yml`
   runs only after pushes to `master`; it creates a Changesets version PR
   or tags a publish and may create a Sentry release. `/ship` lands code;
   it does not deploy Convex/Vercel or run production release commands.
5. **Reflect harness edits never touch master.** Apply reflect's harness
   proposals only on `harness/reflect-outputs`, never on `master`.

## scry Anchors

- Package manager: pnpm only. `package.json` pins `pnpm@10.12.1`, Node
  `>=20.19.0`, and the CI workflow uses Node `20.20.0`.
- Active backlog layout: `backlog.d/NNN-slug.md` plus optional
  `backlog.d/NNN-slug.ctx.md`. Archive matching `backlog.d/<id>-*.md`
  files into `backlog.d/_done/` before squash. Create `_done/` only as
  part of that archival move.
- Branches for shippable work match
  `^(feat|fix|chore|refactor|docs|test|perf)/(\d+)-`. The numeric
  capture is the primary backlog ID.
- There is no drift-contract file in this repo as of this tailoring pass.
  Do not invent one. If future scry adds a real drift contract, read it
  then; until then, doc sync is based on changed paths and existing docs.
- Forbidden without explicit operator approval: `pnpm build:local`,
  `pnpm build:prod`, `pnpm convex:deploy`,
  `./scripts/deploy-production.sh`, production migration scripts, or any
  command that changes non-local Convex/Vercel state.

## Prerequisites

Assert every item at start; refuse on any miss.

- You are on a feature branch, not `master`, `main`, or the protected
  default branch.
- Branch name matches
  `^(feat|fix|chore|refactor|docs|test|perf)/(\d+)-`.
- Working tree is clean: `git status --short` is empty.
- No merge, rebase, or cherry-pick is in progress.
- Same-HEAD landability evidence exists. Acceptable evidence: GitHub mode
  has the required `Quality Checks / merge-gate` green and a mergeable PR;
  git-native mode has a `ship` or `conditional` verdict; or git-native
  mode has operator-provided/current-session local gate receipts for the
  scry gate on this exact HEAD. Do not require a PR or verdict solely to
  land a locally verified git-native branch. A `dont-ship` verdict still
  blocks.
- For git-native local gate evidence, use the scry gate above:
  `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, plus
  `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/
  workflow/dependency surfaces, and `pnpm audit --audit-level=critical`
  for dependency or lockfile changes. A current-session successful run
  after the latest branch commit is sufficient evidence.
- Prefer verdict storage when available:

```sh
source scripts/lib/verdicts.sh
verdict_check_landable "$branch"
```

## Mode Detection

Prefer GitHub mode only when all three probes succeed:

```sh
remote_url="$(git remote get-url origin 2>/dev/null || true)"
if printf '%s\n' "$remote_url" | grep -Eq 'github\\.com[:/]'; then
  if command -v gh >/dev/null 2>&1 && gh pr view --json number >/dev/null 2>&1; then
    mode=github
  else
    mode=git-native
  fi
else
  mode=git-native
fi
```

GitHub mode records the merge in the PR timeline. Git-native mode is the
local fallback for non-GitHub remotes, missing `gh`, or branches without
a viewable PR. Both modes are squash-only and trailer-preserving.

## Process

### 1. Identify Backlog IDs

Extract the primary ID from the branch regex. Then parse trailers from
each branch commit relative to `master`:

```sh
git rev-list master..HEAD |
while read -r sha; do
  git log -1 --format=%B "$sha" |
    git interpret-trailers --parse --no-divider
done
```

Recognize only these trailers:

- `Closes-backlog: <id>` closes a ticket.
- `Ships-backlog: <id>` closes a ticket.
- `Refs-backlog: <id>` references a ticket without closing it.

IDs are bare numeric strings, such as `007`, never `BACKLOG-007`.

- Closing set: primary ID plus every `Closes-backlog:` and
  `Ships-backlog:` value.
- Reference set: every `Refs-backlog:` value. Report these, but never
  archive them.

When available, use the repo helper for closing IDs:

```sh
source scripts/lib/backlog.sh
backlog_ids_from_range master..HEAD
```

Do not concatenate commit messages and parse them once; `git
interpret-trailers --parse` only sees the final trailer block.

### 2. Archive Backlog Files On The Branch

For each ID in the closing set:

```sh
source scripts/lib/backlog.sh
backlog_archive "$id"
```

This moves every matching `backlog.d/<id>-*.md` file, including `.ctx.md`
packets, into `backlog.d/_done/` with `git mv`. The move happens on the
shipping branch before squash.

If the primary ID has no matching `backlog.d/<id>-*.md` file and the
branch has no closing trailers, refuse. scry's backlog is file-driven; a
branch with neither a ticket file nor a closing trailer has no closure
contract.

If an ID is trailer-only and has no file, keep going but report it. Hotfix
and spike branches may close trailer-only work.

### 3. Check Existing Docs Without Inventing A Drift Contract

Inspect changed paths:

```sh
git diff --name-only master..HEAD
```

scry currently has no drift-contract file. Do not create one during
`/ship`. If changed code obviously invalidates an existing doc already in
the diff scope, update that existing doc on the shipping branch before
the archive commit. Use focused, bounded edits only.

Common scry cases:

- Convex schema/query/mutation changes may require existing docs under
  `docs/guides/`, `docs/adr/`, or relevant backlog `.ctx.md` packets to
  stay accurate.
- Build config, workflow, package, or dependency changes may require
  existing workflow/deployment docs to match the live `package.json` and
  `.github/workflows/**` truth.
- Pure UI changes with no doc contract usually require no doc work.

### 4. Commit The Archive And Any Required Doc Sync

Create exactly one prep commit on the feature branch. Subject:

```text
chore(backlog): archive shipped tickets
```

Inject closure trailers with `git interpret-trailers`; do not hand-format
the block:

```sh
msg="chore(backlog): archive shipped tickets"
for id in $CLOSING_IDS; do
  msg="$(printf '%s\n' "$msg" |
    git interpret-trailers \
      --if-exists addIfDifferent \
      --trailer "Closes-backlog: $id")"
done
git commit -m "$msg"
```

If no archive/doc changes were needed, do not create an empty prep commit;
the squash body still must carry the closing trailers.

### 5. Build The Explicit Squash Message

The squash body must preserve all backlog trailers because GitHub's
default squash body can drop commit trailers.

Construct a one-line subject plus a contiguous trailer block. Include
every closing ID as `Closes-backlog: <id>` and every reference ID as
`Refs-backlog: <id>`, using `git interpret-trailers`:

```sh
subject="$(git log --format=%s master..HEAD | tail -1)"
body="$subject"
for id in $CLOSING_IDS; do
  body="$(printf '%s\n' "$body" |
    git interpret-trailers \
      --if-exists addIfDifferent \
      --trailer "Closes-backlog: $id")"
done
for id in $REF_IDS; do
  body="$(printf '%s\n' "$body" |
    git interpret-trailers \
      --if-exists addIfDifferent \
      --trailer "Refs-backlog: $id")"
done
```

Keep trailers in one final contiguous block. No blank line may split
`Closes-backlog:` from `Refs-backlog:` or `Co-Authored-By:`.

### 6. Squash Merge

In GitHub mode:

```sh
pr_number="$(gh pr view --json number --jq .number)"
gh pr checks "$pr_number" --required
gh pr merge "$pr_number" --squash --body "$body"
```

If `gh pr view --json mergeable,mergeStateStatus` reports conflicts,
blocked mergeability, or non-green required checks, refuse. Do not pass
force flags.

In git-native mode:

```sh
branch="$(git branch --show-current)"
git checkout master
git pull --ff-only
git merge --squash "$branch"
tmp="$(mktemp)"
printf '%s\n' "$body" > "$tmp"
git commit -F "$tmp"
rm -f "$tmp"
```

Git-native mode still requires same-HEAD landability evidence from the
prerequisites: a local verdict or operator-provided/current-session gate
receipt.

### 7. Pull Master And Verify Trailers

After the squash:

```sh
git checkout master
git pull --ff-only
git log -1 --format=%B | git interpret-trailers --parse --no-divider
```

Verify the latest master commit contains `Closes-backlog: <id>` for every
ID in the closing set. If any are missing, stop and escalate immediately.
Do not let `/groom` sweep against a malformed closure commit.

### 8. Invoke `/reflect`

Invoke `/reflect` after merge, scoped to the just-shipped work. Provide:

- Original branch name.
- Merged SHA on `master`.
- Closing backlog IDs.
- Reference-only backlog IDs.
- Whether mode was GitHub or git-native, plus PR number if present.
- The scry gate statement quoted above.

Capture reflect's outputs:

- Backlog mutations: new tickets, edits, reprioritizations, or deletions.
- Harness proposals: skill, agent, hook, AGENTS.md, bridge, or settings
  edits.
- Retro notes and coaching.

### 9. Apply Reflect Outputs

Backlog mutations may land on `master` after the merge:

```sh
git checkout master
# Apply only reflect's backlog.d changes.
git commit -m "chore(backlog): apply reflect outputs from shipping <primary-id>"
```

Skip the commit if reflect proposed no backlog changes.

Harness proposals must not land on master. Route them to the dedicated
branch:

```sh
git fetch origin harness/reflect-outputs:harness/reflect-outputs 2>/dev/null || true
git checkout -B harness/reflect-outputs master
# Apply only reflect's harness edits.
git commit -m "chore(harness): apply reflect outputs from shipping <primary-id>"
git push -u origin harness/reflect-outputs
git checkout master
```

If `harness/reflect-outputs` already exists with prior suggestions,
rebase it onto current `master` before adding new commits. Never edit
`.agent/`, `.claude/`, `.codex/`, `.pi/`, `AGENTS.md`, `CLAUDE.md`,
hooks, or harness settings directly on `master` as part of reflect.

## Release Workflow Boundary

`.github/workflows/release.yml` runs on push to `master`. It installs
pnpm `10.12.1`, Node `20.20.0`, then uses Changesets to create a version
PR or publish tags. If publishing occurs, it creates and finalizes a
Sentry release for project `scry`.

`/ship` does not invoke this workflow manually, does not create release
PRs itself, and does not run deployment commands. Its responsibility is a
clean, trailer-preserving squash commit on `master`; the release workflow
reacts afterward.

## Refuse Conditions

Stop and report the exact unblock action when any condition holds:

- Branch name does not match
  `^(feat|fix|chore|refactor|docs|test|perf)/(\d+)-`.
- Current branch is `master`, `main`, or the protected default branch.
- Working tree is dirty.
- Merge, rebase, or cherry-pick is in progress.
- `verdict_check_landable "$branch"` returns `2` (`dont-ship`).
- No same-HEAD landability evidence exists: no green PR checks, no
  landable verdict, and no operator-provided/current-session local gate
  receipt.
- In GitHub mode, required checks are red, missing, cancelled, or pending.
- If a PR exists, it is not mergeable per
  `gh pr view --json mergeable,mergeStateStatus`.
- Primary ID has no `backlog.d/<id>-*.md` file and the branch has no
  closing trailers.
- The squash commit on master is missing any required
  `Closes-backlog: <id>` trailer.
- Shipping would require forbidden production commands or non-local
  Convex/Vercel mutation.

## Gotchas

- scry's live CI source of truth is `.github/workflows/ci.yml` plus the
  repo brief gate statement, not README drift. The workflow job is
  `Quality Checks` / `merge-gate`.
- `package.json` has both `typecheck` and direct `tsc --noEmit` parity.
  The GitHub workflow runs `pnpm typecheck`; the repo brief local parity
  says `pnpm tsc --noEmit`.
- `.github/workflows/release.yml` is reactive after `master` changes.
  Do not treat it as the pre-merge quality gate.
- `backlog_archive` may fail when `backlog.d/_done/` does not exist yet;
  the helper creates it during the first archive.
- `backlog.d/*.ctx.md` files are part of the backlog record. Archive them
  with their paired ticket ID when they match `backlog.d/<id>-*.md`.
- `Ships-backlog:` is closure-intent, but the final squash may normalize
  closing IDs to `Closes-backlog:` so downstream closure is unambiguous.
- `Refs-backlog:` is not closure. Never move a referenced-only ticket to
  `_done/`.
- Do not broaden `/ship` into deploy. Convex deploy and production build
  commands require explicit operator approval in this repo.

## Output

Plain text, one compact block:

```text
/ship complete

Merged:     <sha> on master (PR #<n> | git-native)
Closed:     007, 008
Referenced: 003
Docs:       none required | <paths>
Gate:       Quality Checks / merge-gate green; local parity noted
Reflect:    <backlog mutations>; harness proposals on harness/reflect-outputs; retro notes
Release:    release.yml will react on master push; no deploy command run
Residual:   none | <specific risk>
```

On refuse, report the failed prerequisite and the smallest concrete action
that re-enables shipping.
