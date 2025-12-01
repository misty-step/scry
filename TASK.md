# Kill Questions Table: Complete Dual Data Model Migration

## Executive Summary

**Problem**: Two parallel data systems (`questions` + `concepts/phrasings`) running simultaneously. Every feature requires dual implementation. Invisible complexity compounds with each change.

**Solution**: Atomic removal of the legacy `questions` system. Delete all questions-related modules, delete `/library` route, drop `questionId` from interactions, remove table from schema.

**User Value**: Every future feature is 30% simpler. Single scheduling system. No migration code maintenance. Clean mental model.

**Success Criteria**: Zero references to questions table. `migrations.ts` deleted. `/library` deleted (404 is fine). ~5,000 lines removed.

## Requirements

### Functional
1. `/library` route deleted entirely (404 acceptable - clean break)
2. All card management operations work through concepts system
3. Review flow uses concepts/phrasings exclusively for edit, archive, delete
4. AI generation creates concepts/phrasings only
5. Search/embeddings work on concepts/phrasings only
6. Option shuffling uses true randomness (not deterministic seeding)

### Non-Functional
1. Single atomic deployment (no phased rollout)
2. No data migration required (already complete)
3. No backwards compatibility shims
4. Clean schema with no deprecated tables
5. Nominal consistency: "phrasings" and "concepts" everywhere, never "questions"

## Architecture Decision

### Selected Approach: Atomic Table Kill

Complete removal in single deployment. No feature flags, no gradual sunset, no backwards compatibility.

**Rationale**:
- Data already migrated to concepts
- No production users with significant questions data
- Aggressive risk tolerance acceptable
- Complexity of maintaining dual system > risk of atomic removal

### Alternatives Considered

| Approach | Simplicity | Risk | Why Not |
|----------|------------|------|---------|
| Phased 3-deployment | Low | Low | Maintains complexity longer, more work |
| Feature flags | Medium | Low | Unnecessary given no user data |
| **Atomic removal** | **High** | **Medium** | **Selected - clean break** |

## Deletion Manifest

### Backend Modules to DELETE (~4,200 lines)

```
convex/questions*.ts (6 files, ~1,260 lines)
├── questionsBulk.ts
├── questionsCrud.ts
├── questionsInteractions.ts
├── questionsInteractions.test.ts
├── questionsLibrary.ts
└── questionsRelated.ts

convex/migrations.ts (~2,997 lines)
convex/migrations/ (directory)
├── clusterQuestions.ts
├── clusterQuestionsV3.ts
├── migrateQuestionsToConceptsV2.ts.backup
└── synthesizeConcept.ts
```

### Frontend to DELETE

```
app/library/ (entire directory - 8+ files) → DELETE entirely, 404 is fine
├── page.tsx
├── _components/library-client.tsx
├── _components/library-table.tsx
├── _components/library-cards.tsx
├── _components/library-tab-content.tsx
├── _components/library-pagination.tsx
├── _components/library-empty-states.tsx
├── _components/bulk-actions-bar.tsx
└── _components/use-library-display-mode.ts

hooks/use-question-mutations.ts → DELETE
hooks/use-question-mutations.test.ts → DELETE
components/question-edit-modal.tsx → DELETE
components/question-history.tsx → DELETE (if exists)
contexts/current-question-context.tsx → DELETE (if exists)
types/questions.ts → DELETE
lib/strip-generated-questions.ts → DELETE (if exists)
```

### Schema Changes

```typescript
// convex/schema.ts - REMOVE TABLES:
- questions table (lines 46-103)
- questionEmbeddings table (lines 143-158)
- quizResults table (lines 175-196) // already deprecated

// interactions table - MODIFY:
- Remove questionId field
- Remove by_question index
- Remove by_user_question index
- Keep conceptId, phrasingId only

// generationJobs table - MODIFY:
- Remove questionIds field
- Remove questionsGenerated field
- Remove questionsSaved field
- Keep conceptIds only
```

### Files Requiring Updates

**Backend (Id<'questions'> and questionIds references):**
```
convex/concepts.ts:831         → Remove questionIds: [] initialization
convex/embeddings.ts           → Remove question embedding functions
convex/lib/embeddingHelpers.ts → Remove question-specific logic
convex/lib/validation.ts       → Remove validateBulkOwnership for questions
convex/aiGeneration.ts         → Remove questionIds references
convex/generationJobs.ts       → Remove questionIds field handling
```

**Frontend (legacyQuestionId and question terminology):**
```
components/review-question-display.tsx → Rename to review-phrasing-display.tsx
                                       → Remove questionId prop, use phrasingId only
hooks/use-review-flow.ts              → Remove legacyQuestionId from state
                                       → Remove SimpleQuestion import
hooks/use-review-flow.test.ts         → Update fixtures
components/review-flow.tsx            → Remove legacy question edit/delete
                                       → Wire to phrasing edit/archive operations
components/review-session.tsx         → Remove questionId references
components/generation-task-card.tsx   → Update job.questionIds → job.conceptIds display
hooks/use-shuffled-options.ts         → Remove deterministic seeding entirely
                                       → Use true random shuffle
lib/utils/shuffle.ts                  → Remove getShuffleSeed function
                                       → Or rename param to phrasingId if seed needed
```

**Types:**
```
types/questions.ts                    → DELETE entirely
types/concepts.ts                     → Add SimplePhrasing type (if needed)
```

**Test Files:**
```
tests/convex/questionsLibrary.test.ts           → DELETE
tests/convex/questionsInteractions.record.test.ts → DELETE
tests/convex/migrations.test.ts                 → DELETE (if exists)
tests/convex/generationJobs.test.ts             → Update questionIds → conceptIds
tests/convex/embeddings.test.ts                 → Remove question embedding tests
tests/convex/coverage-regressions.test.ts       → Update fixtures
tests/convex/bandwidth-regressions.test.ts      → Update fixtures
tests/api-contract.test.ts                      → Remove questions API tests
lib/test-utils/fixtures.ts                      → Remove Question fixtures
lib/test-utils/largeFixtures.ts                 → Remove questionId references
```

**Documentation (living docs only, leave ADRs alone):**
```
docs/analytics-events.md              → Update questionId → phrasingId in events
```

## Implementation Phases

### Phase 1: Backend Purge
1. Delete `convex/questions*.ts` (6 files)
2. Delete `convex/migrations.ts`
3. Delete `convex/migrations/` directory
4. Remove questions/questionEmbeddings/quizResults from schema
5. Update interactions schema (remove questionId, indexes)
6. Update generationJobs schema (remove questionIds, questionsGenerated, questionsSaved)
7. Update `convex/concepts.ts` (remove questionIds initialization)
8. Update `convex/embeddings.ts` (concepts-only)
9. Update `convex/lib/embeddingHelpers.ts` (remove question logic)
10. Update `convex/lib/validation.ts` (remove question validation)
11. Update `convex/aiGeneration.ts` (remove questionIds)

### Phase 2: Frontend Purge
1. Delete `app/library/` directory entirely
2. Delete `hooks/use-question-mutations.ts` and test
3. Delete `components/question-edit-modal.tsx`
4. Delete `types/questions.ts`
5. Rename `components/review-question-display.tsx` → `review-phrasing-display.tsx`
6. Update `hooks/use-review-flow.ts` (remove legacyQuestionId)
7. Update `components/review-flow.tsx` (wire to phrasing operations)
8. Update `components/generation-task-card.tsx` (conceptIds display)
9. Fix `hooks/use-shuffled-options.ts` (true random, not deterministic)
10. Update `lib/utils/shuffle.ts` (remove getShuffleSeed or rename param)

### Phase 3: Test Cleanup
1. Delete `tests/convex/questionsLibrary.test.ts`
2. Delete `tests/convex/questionsInteractions.record.test.ts`
3. Delete `tests/convex/migrations.test.ts` (if exists)
4. Update `tests/convex/generationJobs.test.ts`
5. Update `tests/convex/embeddings.test.ts`
6. Update `tests/convex/coverage-regressions.test.ts`
7. Update `lib/test-utils/fixtures.ts`
8. Update `lib/test-utils/largeFixtures.ts`
9. Run full test suite, fix any remaining references

### Phase 4: Documentation & Verification
1. Update `docs/analytics-events.md` (questionId → phrasingId)
2. Leave ADRs unchanged (historical reference)
3. `npx convex dev` - verify schema compiles
4. `pnpm build` - verify no TypeScript errors
5. `pnpm test` - verify tests pass
6. Grep for "questionId", "questions" - verify no remaining references
7. Deploy to production

## Risks & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Hidden questions references | Medium | Low | Grep verification before deploy |
| Schema migration fails | Low | High | Test in dev environment first |
| Review flow breaks | Low | High | Manual test all operations |
| Search breaks | Medium | Medium | Update embeddings to concepts-only |
| Shuffle behavior change | Low | Low | True random is correct behavior |

## Test Scenarios

### Critical Path
- [ ] New content generation creates concepts/phrasings only
- [ ] Review flow loads due concepts
- [ ] Review interaction updates concept FSRS state
- [ ] Phrasing edit works during review (question, options, answer, explanation)
- [ ] Phrasing archive works during review
- [ ] Concept archive works during review
- [ ] Concept library displays all views (active, archived, deleted)
- [ ] Bulk actions work (archive, delete, restore)
- [ ] Search returns concepts/phrasings
- [ ] Option shuffling is truly random (not deterministic)

### Regression
- [ ] `/library` returns 404 (expected)
- [ ] No TypeScript errors referencing questions
- [ ] No runtime errors in console
- [ ] Generation job progress displays correctly (conceptIds count)

## Key Decisions

| Decision | Alternatives | Rationale |
|----------|--------------|-----------|
| Delete vs deprecate | Deprecate with 30-day TTL | No users = no need for grace period |
| Atomic vs phased | 3-phase deployment | Complexity cost > risk |
| Delete /library vs redirect | Redirect preserves bookmarks | Clean break, fuck bookmarks |
| True random shuffle | Deterministic seeding | Deterministic defeats shuffle purpose |
| Rename to phrasing | Keep question naming | Nominal consistency matters |

## Estimated Impact

- **Lines Removed**: ~5,000
- **Files Deleted**: ~25
- **Files Modified**: ~20
- **Complexity Reduced**: Every future feature is 30% simpler
- **Mental Model**: Single concepts/phrasings system, consistent terminology

---

*Next: Run `/plan` to break this into implementation tasks.*
