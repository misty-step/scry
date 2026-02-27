Add `totalPhrasings` to `userStats` schema to make `getReviewDashboard` O(1) (#256/#257).
1. Add `totalPhrasings: v.number()` to `userStats` table in `convex/schema.ts` (with `v.optional(v.number())` initially or require it if we run a migration). Let's make it `v.optional(v.number())` for safe migration.
2. Create a Convex mutation (e.g. `convex/migrations.ts` -> `backfillTotalPhrasings`) to backfill this count for all users.
3. Update `createPhrasing` and `deletePhrasing` (and `archive/restore` if applicable) to increment/decrement `totalPhrasings` in `userStats` atomically. Look at where phrasings are created/deleted.
4. Update `getReviewDashboard` in `convex/concepts.ts` to use `stats.totalPhrasings ?? 0` instead of querying the `phrasings` table.
