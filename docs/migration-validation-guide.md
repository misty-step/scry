# Migration Validation Guide (Task 12)

**Purpose**: Validate the question-to-concepts migration on dev data before production execution.

## Prerequisites

- ✅ Task 11 complete (migration script implemented)
- ✅ Build passing (TypeScript compilation successful)
- ✅ Convex dev environment accessible

## Overview

Task 12 validates migration quality through:
1. Dev environment testing (isolated from production)
2. Dry-run validation (preview without mutations)
3. Quality spot-checking (manual review of sample concepts)
4. Review flow testing (end-to-end functionality)

---

## Step-by-Step Validation

### Step 1: Prepare Dev Environment

**Option A: Use existing dev data**
```bash
# If your dev environment already has orphaned questions
npx convex dev  # Ensure dev backend is running
```

**Option B: Copy production data to dev (recommended)**
1. Export from production:
   - Go to https://dashboard.convex.dev
   - Select production backend (uncommon-axolotl-639)
   - Settings → Import/Export → Export Database
   - Download snapshot ZIP

2. Import to dev:
   - Select dev backend (amicable-lobster-935)
   - Settings → Import/Export → Import Database
   - Upload snapshot ZIP
   - Wait for import completion

**Why copy production data?** Validates clustering quality on real questions with actual content/embeddings.

---

### Step 2: Run Dry-Run Migration

```bash
# Use helper script for safety
./scripts/run-migration.sh migrateQuestionsToConceptsV2 dev
```

The script will:
1. Run dry-run automatically (no mutations)
2. Show preview of clusters to be created
3. Ask for confirmation before actual migration
4. When prompted, type `no` to stay in preview mode

**What to look for in dry-run output:**

✅ **Good signs:**
- Cluster count seems reasonable (not 1 cluster of 163 or 163 clusters of 1)
- Average similarity scores are high (>0.85)
- Sample concept titles make sense
- No errors or warnings

❌ **Red flags:**
- Low similarity scores (<0.70) in clusters
- Concept titles with "and", "vs", "or" (non-atomic)
- Clustering errors or exceptions
- Missing embeddings warnings

**Example good output:**
```
[Migration V2] Found 163 orphaned questions
[Migration V2] Formed 42 clusters
[Migration V2] Cluster sizes: 5, 4, 4, 3, 3, 2, 2, 2, 2, 1...
[DRY RUN] Cluster 1/42: Would create concept "Nicene Creed" with 5 phrasing(s) (avg similarity: 0.91)
[DRY RUN] Cluster 2/42: Would create concept "Trinity Doctrine" with 4 phrasing(s) (avg similarity: 0.88)
...
```

---

### Step 3: Review Cluster Quality

Manually inspect a sample of clusters from dry-run output:

**Pick 5-10 random clusters and ask:**
1. Are the questions actually related?
2. Does the concept title capture the common theme?
3. Are there any obvious mis-clusterings?

**Example inspection:**
```
Cluster: "Nicene Creed" (5 phrasings)
  1. "What year was the Nicene Creed written?"
  2. "Who authored the Nicene Creed?"
  3. "What doctrine does the Nicene Creed affirm?"
  4. "Recite the first line of the Nicene Creed"
  5. "Why was the Nicene Creed created?"

✓ Related? YES - all about Nicene Creed
✓ Title atomic? YES - single concept, no "and"/"vs"
✓ Mis-clustering? NO
```

**Common issues to watch for:**
- Questions about different topics clustered together (false positive)
- Nearly identical questions in separate clusters (false negative)
- Singleton clusters that should be merged (threshold too high)

If clustering quality is poor, consider:
- Adjusting `SIMILARITY_THRESHOLD` in `clusterQuestions.ts` (currently 0.85)
- Checking embedding quality (regenerate if needed)
- Manual post-migration cleanup (future Task 16 in BACKLOG.md)

---

### Step 4: Run Actual Migration (Dev)

If dry-run looks good, proceed with actual migration:

```bash
# Run same script, this time confirm when prompted
./scripts/run-migration.sh migrateQuestionsToConceptsV2 dev
# When asked "Do you want to proceed?", type: yes
```

**Expected behavior:**
- Progress updates as clusters are processed
- Final stats: `{ clustersFormed: X, conceptsCreated: X, phrasingsCreated: X, questionsLinked: X }`
- Diagnostic query runs automatically showing 100% migration

**Example output:**
```
[Migration V2] 1/42: Created concept "Nicene Creed" with 5 phrasing(s)
[Migration V2] 2/42: Created concept "Trinity Doctrine" with 4 phrasing(s)
...
[Migration V2] Complete: { clustersFormed: 42, conceptsCreated: 42, phrasingsCreated: 163, questionsLinked: 163 }

Step 4: Running diagnostic query (checkMigrationStatus)...
{ totalQuestions: 163, orphaned: 0, linked: 163, percentMigrated: 100 }
```

---

### Step 5: Verify Migration Completion

```bash
# Run validation script
./scripts/validate-migration-output.sh
```

**Checklist:**
- [ ] `orphaned: 0` (all questions migrated)
- [ ] `linked: 163` (matches total questions)
- [ ] `percentMigrated: 100`

**Manual verification in Convex dashboard:**
1. Go to Data → concepts table
2. Verify `phrasingCount` distribution:
   - Click column header to sort
   - Should see mix of 1-5 phrasings per concept
   - No concepts with `phrasingCount: 0` (orphaned concepts)

3. Spot-check 10 random concepts:
   - Click concept → View related → phrasings
   - Read the question texts
   - Verify they're actually related

---

### Step 6: Test Review Flow

**End-to-end functionality test:**

1. Open dev app in browser (http://localhost:3000)
2. Navigate to /review
3. Observe:
   - [ ] Badge shows accurate due count (not 163 lying count)
   - [ ] Concepts appear in review queue
   - [ ] Phrasing selection works (canonical/least-seen/random)
   - [ ] Interaction history shows phrasing-specific attempts (not concept-wide)

4. Answer a migrated concept question:
   - [ ] Answer recorded correctly
   - [ ] FSRS scheduling updates
   - [ ] No errors in console
   - [ ] Next review shows different concept (not loop)

5. Check interactions table:
   - Convex dashboard → Data → interactions
   - Find your recent interaction
   - Verify fields: `conceptId`, `phrasingId`, `isCorrect`, `attemptedAt`

---

### Step 7: Quality Sampling

Run sample query for detailed inspection:

```bash
npx convex run migrations:sampleConcepts --args '{"limit":10}'
```

**For each sample, verify:**
- Concept title is semantic and atomic
- Description is accurate
- Phrasings are related to each other
- FSRS state is reasonable (preserved from original)
- Phrasing-level stats look correct (attemptCount, correctCount)

**Document any issues:**
- Edge cases found (e.g., two unrelated questions clustered)
- Data quality issues (missing explanations, truncated text)
- FSRS state anomalies (weird values, nulls)

---

## Success Criteria

**Required (blocking production migration):**
- ✅ Dry-run completes without errors
- ✅ 100% migration (orphaned: 0)
- ✅ Review flow works end-to-end
- ✅ Interactions record correctly (phrasing-specific)
- ✅ No TypeScript/linter errors
- ✅ No console errors during review flow

**Quality thresholds (recommended):**
- ✅ At least 70% of spot-checked concepts have good clustering
- ✅ Concept titles are understandable (no gibberish)
- ✅ No concepts with `phrasingCount: 0`
- ✅ FSRS state preserved for reviewed questions

**Acceptable issues (can fix post-migration):**
- ⚠️ 10-20% mis-clustered questions (can manually fix later)
- ⚠️ Some concept titles need refinement (can edit in UI)
- ⚠️ Singleton concepts that could be merged (low priority)

---

## Troubleshooting

### Issue: Migration fails with "Embedding not found"

**Cause:** Some questions missing embeddings (shouldn't happen, but possible edge case)

**Fix:**
```typescript
// In clusterQuestions.ts, embedding generation is already implemented
// If this fails, check:
// 1. OPENAI_API_KEY is set in Convex env vars
// 2. OpenAI API is accessible (check network/quotas)
```

### Issue: Clustering produces 163 singleton clusters

**Cause:** Threshold too high (0.85), questions too dissimilar

**Fix:**
1. Lower threshold in `clusterQuestions.ts`: `const SIMILARITY_THRESHOLD = 0.80;`
2. Redeploy Convex functions: `npx convex dev`
3. Re-run migration

**Note:** Singleton clusters are acceptable if questions are truly unique.

### Issue: Concept title synthesis fails

**Cause:** OpenAI API error or rate limit

**Fix:**
1. Check API key: `npx convex env list | grep OPENAI`
2. Check OpenAI dashboard for quota/errors
3. Retry migration (idempotent - won't duplicate)

### Issue: Review flow shows "No reviews due" after migration

**Possible causes:**
1. Questions had future `nextReview` dates (scheduled far ahead)
2. All concepts in 'new' state but app requires due date
3. `getDue()` query not finding migrated concepts

**Debug:**
```bash
# Check concept FSRS states
npx convex run migrations:sampleConcepts --args '{"limit":5}'
# Look at fsrsState and fsrs.nextReview values

# Manually query concepts
# Convex dashboard → Data → concepts → Filter where nextReview < now
```

---

## Next Steps After Validation

If validation passes:
1. ✅ Mark Task 12 complete in TODO.md
2. Document any edge cases found
3. Create issues for post-migration cleanup (if needed)
4. Proceed to Task 13 (Execute Production Migration)
5. Schedule production migration window

If validation fails:
1. Document failure mode and errors
2. Fix issues in migration code
3. Re-test in dev environment
4. Do NOT proceed to production

---

## References

- Migration script: `convex/migrations/migrateQuestionsToConceptsV2.ts`
- Clustering logic: `convex/migrations/clusterQuestions.ts`
- Synthesis logic: `convex/migrations/synthesizeConcept.ts`
- Helper scripts: `scripts/run-migration.sh`, `scripts/validate-migration-output.sh`
- Schema: `convex/schema.ts` (concepts, phrasings, interactions tables)

---

## Rollback Plan (If Needed)

If migration creates bad data:

1. **Delete migrated concepts:**
   ```bash
   # This deletes concepts and cascades to phrasings
   # (Assuming cascade delete is configured in schema)
   npx convex run migrations:rollbackMigration
   ```

2. **Clear `conceptId` from questions:**
   ```bash
   # Remove foreign key so questions become orphaned again
   npx convex run migrations:unlinkQuestionsFromConcepts
   ```

3. **Re-attempt migration:**
   - Fix issues in code
   - Deploy updated functions
   - Re-run migration from Step 2

**Note:** Rollback helpers not yet implemented. If needed during validation, add to `migrateQuestionsToConceptsV2.ts`.
