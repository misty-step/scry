---
name: convex-migrate
description: Safe Convex schema/data migrations in scry, including schema removals, dry-runs, diagnostics, target verification, and production approval gates.
trigger: /convex-migrate
---

# /convex-migrate

Use this skill for any scry Convex schema or data migration. scry treats `convex/schema.ts` as an API contract: data shape changes must be deliberate, observable, reversible where possible, and verified before and after execution.

## Read First

Before planning or editing a migration, read these anchors:

- `.spellbook/repo-brief.md`
- `convex/schema.ts`
- `convex/migrations.ts`
- `scripts/run-migration.sh`
- `scripts/validate-migration-output.sh`
- `docs/guides/writing-migrations.md`
- `docs/runbooks/production-deployment.md`

If docs and live scripts disagree, prefer live repo code, then note the drift in the migration plan.

## Hard Boundaries

- Do not run production migration or deploy commands without explicit operator approval in the current turn.
- Forbidden without approval: `./scripts/run-migration.sh <migration> production`, `pnpm convex:deploy`, `npx convex deploy --yes --cmd-url-env-var-name UNSET`, `./scripts/deploy-production.sh`, `pnpm build:prod`, and any command that mutates non-local Convex or Vercel state.
- Dev target is `amicable-lobster-935`.
- Production target is `uncommon-axolotl-639`.
- Always log and verify the target before any migration. If a production command would not print or validate `uncommon-axolotl-639`, stop.
- Never use `source .env.production`; load the production deploy key with `grep CONVEX_DEPLOY_KEY .env.production | cut -d= -f2` and verify it starts with `prod:`.

## Required Migration Shape

Every migration must have these components before it can run outside local exploration:

- A `dryRun` argument that computes the same candidate set and stats without writing.
- A diagnostic query that returns the remaining unmigrated count and a sample id when present.
- Runtime field checks for removed or deprecated fields: use `'fieldName' in (doc as any)`, not `doc.fieldName !== undefined`.
- Bounded/batched processing. Prefer indexed queries plus `.take(limit)` or explicit `batchSize`; do not add unbounded `.collect()` to runtime paths.
- Environment/deployment-target logging at migration start, including dry-run state and enough context to distinguish dev from prod logs.
- Idempotent behavior: rerunning after success should update zero records and report already-migrated rows.

For scry's current migration module, `convex/migrations.ts` already exposes `backfillTotalPhrasings` and `diagnosticBackfillTotalPhrasings`; preserve that dry-run/diagnostic pairing when adding new work.

## Exact Schema Removal Sequence

Field removal in scry is always three phases:

1. Optional
   Change the schema field from required to `v.optional(...)` in `convex/schema.ts`. Deploy this compatibility schema only after approval if the target is production.

2. Dry-run, backfill, diagnostic
   Add the migration and diagnostic in `convex/migrations.ts`. Run dry-run first, inspect the expected count, run the actual migration only after approval, then run the diagnostic until it reports zero remaining records. Use runtime checks such as `'deprecatedField' in (doc as any)` while the stale field may still exist in stored documents.

3. Remove
   Only after diagnostic success, remove the field from `convex/schema.ts` and deploy the clean schema. If schema validation reports an extra field, revert to optional, rerun the migration workflow, and diagnose before trying removal again.

Do not collapse these phases into one commit or one deployment. The optional phase is what lets Convex accept both old documents and migrated documents while the migration runs.

## Execution Workflow

1. State the migration invariant.
   Name the table, field or data relation, expected old shape, expected new shape, and the diagnostic success condition.

2. Implement backend first.
   Update `convex/schema.ts` first when entering the optional phase. Add or update the `convex/migrations.ts` mutation/query next. Do not wire UI or product behavior before schema/query/mutation readiness.

3. Use the repo runner.
   Prefer `./scripts/run-migration.sh <migrationName> dev` for dev execution. It runs dry-run first, asks for manual confirmation, runs the write pass, then tries `<migrationName>Diagnostic` and falls back to `checkMigrationStatus`.

4. Validate with the helper.
   Use `./scripts/validate-migration-output.sh` when the migration affects concepts/phrasings or the historical question-to-concept migration path. If the helper's hardcoded checks do not match the migration, add a migration-specific diagnostic query rather than trusting dashboard inspection alone.

5. Production is operator-gated.
   For production, present the exact command and expected target (`uncommon-axolotl-639`) and wait for explicit approval before execution. Dry-run on production is still a production migration command and needs approval.

## Checks

Cite this repo gate statement in every migration plan and pre/post migration report:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`, `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint && pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and `pnpm audit --audit-level=critical` for dependency or lockfile changes.

For migration work, run or require:

- Before migration code lands: `pnpm lint`, `pnpm tsc --noEmit`, `pnpm test:ci`, and `pnpm test:contract`.
- Before any production migration: dry-run output captured, deployment target verified, expected change count documented, and diagnostic query available.
- After migration: diagnostic query returns zero remaining records, target is still the intended deployment, and contract tests still pass for any `convex/**` change.

## Review Checklist

- The migration cannot write when `dryRun` is true.
- The diagnostic is independent of the mutation and can prove completion.
- Removed-field detection uses `'field' in (doc as any)` everywhere stale stored data is possible.
- Batches are bounded and progress is logged.
- Dev/prod target names are explicit in logs or operator output: `amicable-lobster-935` for dev, `uncommon-axolotl-639` for prod.
- Production commands are not executed unless the operator approved them in this turn.
- Schema removal follows optional -> dry-run/backfill/diagnostic -> remove exactly.
