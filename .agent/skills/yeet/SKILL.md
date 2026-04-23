---
name: yeet
description: |
  End-to-end "commit this scry work and push it" command. Reads the dirty
  worktree without trampling other workers, filters scaffold/harness drift,
  runs the scry ship gate, slices owned work into conventional commits, and
  pushes the current branch when safe.
  Use when: "yeet", "yeet this", "commit and push", "ship it", "tidy and
  commit", "wrap this up and push", "get this off my machine".
  Trigger: /yeet, /ship-local (alias).
argument-hint: "[--dry-run] [--single-commit] [--no-push]"
---

# /yeet

Take the scry work you own in the current tree -> conventional commit(s) ->
remote. This is a judgment layer over git, not a blind wrapper.

## Stance

1. **Act on owned work only.** Stage what belongs to the current task, leave
   out user changes and other-worker edits, delete only unambiguous local debris,
   split logically, and push unless `--no-push` or a refuse condition applies.
2. **Dirty worktrees are normal here.** scry tailor runs often leave untracked
   `.agent/skills/**`, `.claude/skills/**`, `.codex/skills/**`, `.pi/skills/**`,
   `.spellbook/**`, and `scripts/lib/**` scaffold drift. Do not stage that drift
   unless the current request explicitly owns those paths.
3. **Reviewability is the product.** A stack of three focused commits beats
   one 2,000-line "wip" commit. Split on semantic boundaries even if the tree
   was built in one session.
4. **Never lose work.** Untracked scratch that might be the user's in-flight
   thinking is reported and left alone, not deleted, unless it is unambiguous
   debris such as `.DS_Store` or editor swap files.
5. **Conventional Commits, always.** Type, optional scope, imperative subject.
   Body explains why, not what.
6. **pnpm only.** `packageManager` is `pnpm@10.12.1`. `bun.lock` is migration
   evidence, not permission to use Bun. Never use `npm`, `yarn`, or `bun` for
   verification or dependency changes in this repo.

## Modes

- Default: classify -> verify -> stage owned paths -> split into commits -> push.
- `--dry-run`: report the plan (commit boundaries, messages, skips), do not execute.
- `--single-commit`: skip the split pass; one commit for everything that belongs.
- `--no-push`: commit locally but don't push. Useful when the user wants to
  amend before going remote.

## Scry Ship Gate

Use the repo brief's gate statement verbatim:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`,
> `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via
> the `.only` grep, and `pnpm test:ci` must pass; local parity is
> `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract`
> for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces,
> and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

Default verification before push is:

```bash
pnpm lint
pnpm tsc --noEmit
pnpm test:ci
```

Additive checks:

- Run `pnpm test:contract` when any owned commit touches `convex/**` or generated
  Convex API contract expectations.
- Run `pnpm build` when owned work touches dependencies, build config, workflow,
  deployment config, Next/Vercel config, TypeScript/Vitest/Playwright/ESLint
  config, or scripts used by build/deploy surfaces.
- Run `pnpm audit --audit-level=critical` when `package.json`,
  `pnpm-lock.yaml`, dependency overrides, or package-manager metadata change.
- Never run `pnpm build:local`, `pnpm build:prod`, `pnpm convex:deploy`,
  `./scripts/deploy-production.sh`, production migration scripts, or anything
  that mutates non-local Convex/Vercel/Stripe state without explicit operator
  approval.

## Process

### 1. Read the worktree and branch

- `git status --short` (untracked, modified, staged - full picture, don't truncate)
- `git diff --stat` and `git diff --stat --cached` (sizes + files)
- `git log --oneline -n 20` (recent commit style)
- `git rev-parse --abbrev-ref HEAD` (branch, for push target)
- `git status` for rebase/merge/cherry-pick in progress (see Refuse Conditions)
- `git diff --name-only` and `git diff --name-only --cached` for path ownership

If no owned paths are dirty, say so and exit. Do not "clean up" unrelated user,
tailor, or scaffold work merely because it appears in `git status`.

### 2. Classify every file

For each changed or untracked path, assign one of:

| Class | Meaning | Action |
|---|---|---|
| **owned signal** | Work the user asked this session to ship | Include in a commit |
| **owned support** | Tests, generated API types, docs, changesets, or config required by owned signal | Include with the signal commit |
| **harness drift** | Tailor/scaffold files outside current ownership, especially `.agent/skills/**`, `.claude/**`, `.codex/**`, `.pi/**`, `.spellbook/**`, `scripts/lib/**` | Leave unstaged and report |
| **user drift** | Plausible human or other-worker edits unrelated to this request | Leave unstaged and report |
| **debris** | `.DS_Store`, `Thumbs.db`, `*.swp`, `*.swo`, `*~`, `.orig`, accidental logs, `node_modules` | Delete only if clearly local trash |
| **evidence** | Screenshots, QA notes, logs, walkthrough artifacts proving the change | Commit only if the repo convention or request says to preserve it |
| **secret-risk** | Credentials, tokens, private keys, `.env*`, prod state | Refuse |
| **deploy/migration/prod state** | Deployment outputs, Convex/Vercel/prod migration run state, generated prod env material, operator logs from deploy commands | Refuse unless the user explicitly asked to commit that exact artifact |

Secret scan the owned diff before staging. At minimum check for private-key
blocks, `api[_-]?key` assignments with long values, `AKIA`, `ghp_`,
`github_pat_`, `sk-`, Clerk/Stripe/Convex/Vercel tokens, and `.env*` content.

### 3. Respect scry boundaries

- Backend-before-frontend for Convex work: schema/query/mutation first, generated
  API readiness next, UI last. If the diff violates this shape, refuse and ask
  for implementation repair before committing.
- Pure FSRS is non-negotiable. Do not commit daily caps, comfort-mode shortcuts,
  artificial review limits, or "FSRS but better" behavior.
- Convex runtime paths must avoid unbounded `.collect()`. If owned code adds one,
  refuse until it uses indexes with bounded `.take()` or pagination.
- Destructive semantics need reverses: archive/unarchive, softDelete/restore.
  Hard delete requires explicit confirmation UX.
- Do not push deploy/migration/prod state. Code changes to deploy tooling may be
  committed only after the build/audit gates above and only if no command mutated
  non-local state.
- Respect existing dirty state. Never revert, format, delete, or stage paths not
  owned by the current task.

### 4. Group owned work into semantic commits

Group rules:
- **One concern per commit.** Separate feature from refactor from chore.
- **Co-changed tests belong with their code.** A new feature + its tests is one commit; don't split them.
- **Config that enables the feature goes with the feature.** The env.ts change
  that adds a knob for the new lane ships with the lane.
- **Cross-cutting infrastructure changes are their own commit.** Workflow,
  hook, formatter, or harness wiring stays separate from feature work.
- **Refactors before features.** If the diff contains a pure refactor AND a
  feature that builds on it, commit the refactor first (makes bisect sane).
- **Backend before frontend.** For Convex-backed features, commit schema/query/
  mutation/API contract support before UI usage when that split is reviewable.
- **Carmack's stapled-PR rule.** If you'd describe the change as "X and also Y,"
  it's two commits.
- **Harness changes are narrow.** A rewrite of one skill is one docs/chore commit;
  do not sweep in adjacent skill bridge/scaffold churn from another worker.

If the user passed `--single-commit`, skip grouping; everything signal-class
becomes one commit.

### 5. Write scry commit messages

Conventional Commits. Format:

```
<type>(<scope>): <imperative subject under 72 chars>

<optional body: why, not what. Wrap at 72.>

<optional footer: BREAKING CHANGE, refs, co-author>
```

Types used here: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `build`,
`ci`, `chore`, `style`, and `ops` when the change is explicitly operational.
No `wip`, `misc`, or `update`.

Scope should match recent scry history and the changed surface:

- `convex` for backend functions/schema/FSRS scheduling contract changes.
- `agent` for Willow/review-chat/agentic review surfaces.
- `qa`, `tooling`, `git`, `docs`, `pi`, or `architecture` when recent log style
  supports it.
- Omit scope when the recent log and change are cleaner without one.

Subject rules:
- Imperative ("add", not "added" or "adds").
- No trailing period.
- Do not add PR numbers. GitHub squash/merge adds `(#NNN)` when appropriate.
- Do not use backlog IDs in the subject unless the existing branch convention
  already does so for the exact slice.

Body rules:
- Omit entirely if the subject is self-explanatory.
- When present, explain the why: the constraint, incident, or reason
  this was the right call over alternatives.
- Do NOT restate the file-level diff.

Backlog-linked work:

- File-driven backlog lives in `backlog.d/`.
- Shippable ticket branches use `<type>/<id>-<slug>` with a bare numeric ID,
  e.g. `feat/007-agent-ready-architecture`, so `/ship` can preserve
  `Closes-backlog:` and `Ships-backlog:` trailers.
- If current work clearly completes or ships a `backlog.d/NNN-*.md` ticket but
  the branch does not follow that pattern, stop before push and ask whether to
  rename the branch or commit locally with `--no-push`.
- `/yeet` does not invent backlog trailers. Preserve existing trailers if they
  are already part of the branch workflow; otherwise leave trailer ownership to
  `/ship`.

### 6. Verify, stage, commit, push

- Run the required gate commands for the owned path set before pushing. Use
  `pnpm` commands only.
- If hooks fail, fix the underlying issue or refuse. Never use `--no-verify`.
- Stage path-by-path for each commit. Never use root `git add -A`.
- `git commit` per group. Allow hooks to run (`lefthook`, `pre-commit`). If
  a hook fails, investigate and fix the underlying issue - do not `--no-verify`.
- After the final commit: `git push`. If upstream is missing:
  `git push -u origin <branch>`.
- If `git push` is rejected (upstream moved), pull-rebase (if linear) and retry
  once. Do not force-push.
- After push, rerun `git status --short --untracked-files=all`. Report remaining
  unowned dirty paths as preserved user/worker drift; do not call the worktree
  globally clean unless it actually is.

### 7. Report

What got committed (one line per commit: sha, type, subject).
What checks ran and their result.
What was intentionally left unstaged and why.
What was deleted as debris, if anything.
Push target and result.
Final owned-work status (`shipped`, `committed-no-push`, `dry-run`, or `refused`).

## Refuse Conditions

Stop and surface to the user instead of committing:

- `.git/MERGE_HEAD`, `.git/CHERRY_PICK_HEAD`, or `rebase-*` dir exists - mid-operation.
- Diff contains unresolved conflict markers (`<<<<<<<`, `=======`, `>>>>>>>`).
- Any file classified `secret-risk`.
- Any owned path is deploy/migration/prod state not explicitly requested.
- Current branch is `main`, `master`, or the default protected branch and push
  would write to that branch.
- HEAD is detached.
- A backlog-linked branch is misnamed and push would hide the ticket linkage.
- Required scry gate fails and the fix is outside owned scope.
- The worktree has >500 changed files and no obvious owned subset.

## Safety Rails

- Never force-push.
- Never `--no-verify` to bypass hooks.
- Never `git add -A` at the repo root without classifying first.
- Never `git clean -fdx` or delete directories without individual-file classification.
- Never commit files whose content matches known secret patterns (above).
- Never use `npm`, `yarn`, or `bun`.
- Never run deploy/prod/migration commands without explicit operator approval.
- Never stage unrelated `.agent`, `.claude`, `.codex`, `.pi`, `.spellbook`, or
  `scripts/lib` churn from other tailor workers.
- Never declare global clean while `git status --short` still shows paths.

## Scry Gotchas

- **"Tidy" is not refactor.** This skill stages and commits - it does not
  edit source code to make it prettier. If the diff is messy, that's a
  `/refactor` concern, not `/yeet`.
- **The root can look noisy during tailor.** Untracked skill and bridge files
  may belong to other workers. Path ownership beats cleanliness.
- **README can drift.** For commands, trust `package.json`, `.github/workflows/**`,
  and `lefthook.yml`, then the repo brief. Do not copy stale README scripts.
- **Bun is not the default.** `bun.lock` and Bun parity workflows are migration
  evidence. Use `pnpm`.
- **Build scripts are not equal.** `pnpm build` is allowed when required.
  `pnpm build:local`, `pnpm build:prod`, `pnpm convex:deploy`, and
  `./scripts/deploy-production.sh` are forbidden without explicit approval.
- **Convex changes need contract attention.** Schema/query/mutation changes
  need generated API/type readiness and `pnpm test:contract`.
- **Dependencies are expensive.** If `package.json` or `pnpm-lock.yaml` changes,
  run both `pnpm build` and `pnpm audit --audit-level=critical` in addition to
  local parity.
- **Pre-commit hooks can reformat.** If lefthook's `stage_fixed: true`
  mutates owned files during commit, they're still part of that commit.
  Don't panic and re-stage.
- **Push rejection on first try is usually benign.** Upstream moved.
  Rebase-pull + push once. If it rejects again, stop - something weirder is
  happening.

## Output

```markdown
## /yeet Report

Gate: pnpm lint; pnpm tsc --noEmit; pnpm test:ci; pnpm test:contract

Commits:
  abc1234 refactor(convex): extract review scheduling helpers
  def5678 feat(agent): render cloze review artifacts

Pushed feat/006-cloze-review-artifacts -> origin (2 new commits).
Preserved unstaged worker drift: .agent/skills/qa/SKILL.md, .codex/skills/qa
Owned work: shipped
```

On refuse:

```markdown
## /yeet - REFUSED
Reason: .env.production contains plausible secret
  (matches /sk-[A-Za-z0-9]{32}/ at line 12).
Action: remove or gitignore the file before re-running.
```
