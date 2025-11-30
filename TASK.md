# Kill Questions Table: Complete Dual Data Model Migration

## Executive Summary

**Problem**: Two parallel data systems (`questions` + `concepts/phrasings`) running simultaneously. Every feature requires dual implementation. Invisible complexity compounds with each change.

**Solution**: Atomic removal of the legacy `questions` system. Delete all questions-related modules, replace `/library` with `/concepts`, drop `questionId` from interactions, remove table from schema.

**User Value**: Every future feature is 30% simpler. Single scheduling system. No migration code maintenance. Clean mental model.

**Success Criteria**: Zero references to questions table. `migrations.ts` deleted. `/library` redirects to `/concepts`. ~4,500 lines removed.

## Requirements

### Functional
1. `/library` route redirects to `/concepts` (or replaced entirely)
2. All card management operations work through concepts system
3. Review flow continues functioning (already uses concepts)
4. AI generation continues functioning (already uses concepts)
5. Search/embeddings work on concepts/phrasings only

### Non-Functional
1. Single atomic deployment (no phased rollout)
2. No data migration required (already complete)
3. No backwards compatibility shims
4. Clean schema with no deprecated tables

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

### Backend Modules to Delete (~4,200 lines)

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

### Frontend to Delete/Modify

```
app/library/ (entire directory - 8 files)
├── page.tsx → DELETE or redirect to /concepts
├── _components/library-client.tsx
├── _components/library-table.tsx
├── _components/library-cards.tsx
├── _components/library-tab-content.tsx
├── _components/library-pagination.tsx
├── _components/library-empty-states.tsx
└── _components/bulk-actions-bar.tsx

hooks/use-question-mutations.ts → DELETE
components/question-edit-modal.tsx → DELETE or migrate to concepts
types/questions.ts → DELETE
```

### Schema Changes

```typescript
// convex/schema.ts - REMOVE:
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
- Keep conceptIds only
```

### Files Requiring Updates (Id<'questions'> references)

```
components/review-question-display.tsx
hooks/use-review-flow.ts
hooks/use-review-flow.test.ts
lib/test-utils/fixtures.ts
lib/test-utils/largeFixtures.ts
convex/lib/validation.ts
convex/lib/embeddingHelpers.ts
convex/embeddings.ts
tests/convex/*.test.ts (multiple)
```

## Implementation Phases

### Phase 1: Backend Purge
1. Delete `convex/questions*.ts` (6 files)
2. Delete `convex/migrations.ts`
3. Delete `convex/migrations/` directory
4. Remove questions/questionEmbeddings/quizResults from schema
5. Update interactions schema (remove questionId)
6. Update generationJobs schema (remove questionIds)
7. Update `convex/embeddings.ts` (concepts-only)
8. Update `convex/lib/embeddingHelpers.ts`
9. Update `convex/lib/validation.ts`

### Phase 2: Frontend Purge
1. Delete `app/library/` directory
2. Add redirect: `/library` → `/concepts`
3. Delete `hooks/use-question-mutations.ts`
4. Delete `components/question-edit-modal.tsx`
5. Delete `types/questions.ts`
6. Update `components/review-question-display.tsx`
7. Update `hooks/use-review-flow.ts`

### Phase 3: Test Cleanup
1. Delete `tests/convex/migrations.test.ts`
2. Update `tests/convex/embeddings.test.ts`
3. Update `tests/convex/spacedRepetition.test.ts`
4. Update `lib/test-utils/fixtures.ts`
5. Update `lib/test-utils/largeFixtures.ts`
6. Run full test suite, fix any remaining references

### Phase 4: Verification
1. `npx convex dev` - verify schema compiles
2. `pnpm build` - verify no TypeScript errors
3. `pnpm test` - verify tests pass
4. Grep for "questions" - verify no remaining references
5. Deploy to production

## Risks & Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Hidden questions references | Medium | Low | Grep verification before deploy |
| Schema migration fails | Low | High | Test in dev environment first |
| Review flow breaks | Low | High | Already uses concepts; manual test |
| Search breaks | Medium | Medium | Update embeddings to concepts-only |

## Test Scenarios

### Critical Path
- [ ] New content generation creates concepts/phrasings
- [ ] Review flow loads due concepts
- [ ] Review interaction updates concept FSRS state
- [ ] Concept library displays all views (active, archived, deleted)
- [ ] Bulk actions work (archive, delete, restore)
- [ ] Search returns concepts/phrasings

### Regression
- [ ] `/library` redirects to `/concepts`
- [ ] No 404s on removed routes
- [ ] No TypeScript errors referencing questions
- [ ] No runtime errors in console

## Key Decisions

| Decision | Alternatives | Rationale |
|----------|--------------|-----------|
| Delete vs deprecate | Deprecate with 30-day TTL | No users = no need for grace period |
| Atomic vs phased | 3-phase deployment | Complexity cost > risk |
| Redirect vs 404 | 404 on /library | Redirect preserves bookmarks |
| Drop interactions history | Keep with null questionId | Clean data model > historical reference |

## Estimated Impact

- **Lines Removed**: ~4,500
- **Files Deleted**: ~20
- **Files Modified**: ~15
- **Complexity Reduced**: Every future feature is 30% simpler
- **Mental Model**: Single concepts/phrasings system

---

*Next: Run `/plan` to break this into implementation tasks, or `/architect` for detailed module design.*
