import { describe, expect, it } from 'vitest';
import type { Id } from '@/convex/_generated/dataModel';
import { reviewReducer } from './use-review-flow';

const baseState = {
  phase: 'loading' as const,
  question: null,
  interactions: [],
  conceptId: null,
  conceptTitle: null,
  phrasingId: null,
  phrasingIndex: null,
  totalPhrasings: null,
  selectionReason: null,
  lockId: null,
  isTransitioning: false,
  conceptFsrs: null,
  skippedConceptIds: new Set<Id<'concepts'>>(),
  lastAutoSkippedId: null as Id<'concepts'> | null,
};

describe('reviewReducer', () => {
  it('handles LOAD_START by setting loading and clearing transitioning', () => {
    const state = { ...baseState, isTransitioning: true, phase: 'reviewing' as const };
    const next = reviewReducer(state, { type: 'LOAD_START' });
    expect(next.phase).toBe('loading');
    expect(next.isTransitioning).toBe(false);
  });

  it('handles LOAD_EMPTY by clearing state and setting empty phase', () => {
    const state = {
      ...baseState,
      phase: 'reviewing' as const,
      question: { question: 'Q', options: [], correctAnswer: 'A' },
      interactions: [{ _id: 'i1' } as any],
      conceptId: 'c1' as any,
      conceptTitle: 'Concept',
      phrasingId: 'p1' as any,
      phrasingIndex: 1,
      totalPhrasings: 2,
      selectionReason: 'seed',
      lockId: 'lock',
      isTransitioning: true,
      conceptFsrs: { state: 'review' as const, reps: 2 },
      errorMessage: 'prev',
    };

    const next = reviewReducer(state, { type: 'LOAD_EMPTY' });
    expect(next.phase).toBe('empty');
    expect(next.question).toBeNull();
    expect(next.interactions).toEqual([]);
    expect(next.conceptId).toBeNull();
    expect(next.conceptTitle).toBeNull();
    expect(next.phrasingId).toBeNull();
    expect(next.phrasingIndex).toBeNull();
    expect(next.totalPhrasings).toBeNull();
    expect(next.selectionReason).toBeNull();
    expect(next.lockId).toBeNull();
    expect(next.isTransitioning).toBe(false);
    expect(next.errorMessage).toBeUndefined();
    expect(next.conceptFsrs).toBeNull();
  });

  it('handles LOAD_TIMEOUT by setting error phase with message', () => {
    const next = reviewReducer(baseState, { type: 'LOAD_TIMEOUT' });
    expect(next.phase).toBe('error');
    expect(next.errorMessage).toContain('Loading is taking longer');
  });

  it('handles QUESTION_RECEIVED by populating payload and entering reviewing', () => {
    const payload = {
      question: { question: 'What?', options: ['A'], correctAnswer: 'A' },
      interactions: [{ _id: 'i1' } as any],
      conceptId: 'c1' as any,
      conceptTitle: 'Concept',
      phrasingId: 'p1' as any,
      phrasingStats: { index: 2, total: 4 },
      selectionReason: 'due',
      lockId: 'lock-123',
      conceptFsrs: { state: 'review' as const, reps: 3 },
    };

    const next = reviewReducer(baseState, { type: 'QUESTION_RECEIVED', payload });

    expect(next.phase).toBe('reviewing');
    expect(next.question).toEqual(payload.question);
    expect(next.interactions).toEqual(payload.interactions);
    expect(next.conceptId).toBe(payload.conceptId);
    expect(next.conceptTitle).toBe(payload.conceptTitle);
    expect(next.phrasingId).toBe(payload.phrasingId);
    expect(next.phrasingIndex).toBe(payload.phrasingStats?.index);
    expect(next.totalPhrasings).toBe(payload.phrasingStats?.total);
    expect(next.selectionReason).toBe(payload.selectionReason);
    expect(next.lockId).toBe(payload.lockId);
    expect(next.isTransitioning).toBe(false);
    expect(next.errorMessage).toBeUndefined();
    expect(next.conceptFsrs).toEqual(payload.conceptFsrs);
  });

  it('handles REVIEW_COMPLETE by clearing lock and marking transition while staying reviewing', () => {
    const state = {
      ...baseState,
      phase: 'reviewing' as const,
      lockId: 'lock-123',
      isTransitioning: false,
    };
    const next = reviewReducer(state, { type: 'REVIEW_COMPLETE' });
    expect(next.phase).toBe('reviewing');
    expect(next.lockId).toBeNull();
    expect(next.isTransitioning).toBe(true);
  });

  it('returns state unchanged for IGNORE_UPDATE', () => {
    const next = reviewReducer(baseState, { type: 'IGNORE_UPDATE', reason: 'no change' });
    expect(next).toBe(baseState);
  });

  describe('skip behavior', () => {
    it('handles SKIP_CONCEPT by adding concept to skip set and triggering transition', () => {
      const state = {
        ...baseState,
        phase: 'reviewing' as const,
        conceptId: 'concept_1' as Id<'concepts'>,
        lockId: 'lock-123',
        isTransitioning: false,
      };

      const next = reviewReducer(state, {
        type: 'SKIP_CONCEPT',
        payload: 'concept_1' as Id<'concepts'>,
      });

      expect(next.skippedConceptIds.has('concept_1' as Id<'concepts'>)).toBe(true);
      expect(next.lockId).toBeNull();
      expect(next.isTransitioning).toBe(true);
      // lastAutoSkippedId preserved (not reset) to allow queue exhaustion detection
    });

    it('preserves lastAutoSkippedId when skipping (does not reset it)', () => {
      const state = {
        ...baseState,
        phase: 'reviewing' as const,
        conceptId: 'concept_2' as Id<'concepts'>,
        lastAutoSkippedId: 'concept_1' as Id<'concepts'>,
      };

      const next = reviewReducer(state, {
        type: 'SKIP_CONCEPT',
        payload: 'concept_2' as Id<'concepts'>,
      });

      // Critical: lastAutoSkippedId must NOT be cleared on manual skip
      // This allows AUTO_SKIP to detect queue exhaustion correctly
      expect(next.lastAutoSkippedId).toBe('concept_1');
    });

    it('accumulates multiple skipped concepts in skip set', () => {
      const state = {
        ...baseState,
        phase: 'reviewing' as const,
        skippedConceptIds: new Set(['concept_1'] as Id<'concepts'>[]),
      };

      const next = reviewReducer(state, {
        type: 'SKIP_CONCEPT',
        payload: 'concept_2' as Id<'concepts'>,
      });

      expect(next.skippedConceptIds.size).toBe(2);
      expect(next.skippedConceptIds.has('concept_1' as Id<'concepts'>)).toBe(true);
      expect(next.skippedConceptIds.has('concept_2' as Id<'concepts'>)).toBe(true);
    });

    it('handles AUTO_SKIP by tracking concept for queue exhaustion detection', () => {
      const state = {
        ...baseState,
        phase: 'reviewing' as const,
        skippedConceptIds: new Set(['concept_1'] as Id<'concepts'>[]),
      };

      const next = reviewReducer(state, {
        type: 'AUTO_SKIP',
        payload: { conceptId: 'concept_1' as Id<'concepts'>, reason: 'in_skip_set' },
      });

      expect(next.lastAutoSkippedId).toBe('concept_1');
      expect(next.lockId).toBeNull();
      expect(next.isTransitioning).toBe(true);
    });

    it('handles CLEAR_SKIPPED by emptying skip set and resetting tracker', () => {
      const state = {
        ...baseState,
        phase: 'reviewing' as const,
        skippedConceptIds: new Set(['concept_1', 'concept_2'] as Id<'concepts'>[]),
        lastAutoSkippedId: 'concept_1' as Id<'concepts'>,
      };

      const next = reviewReducer(state, { type: 'CLEAR_SKIPPED' });

      expect(next.skippedConceptIds.size).toBe(0);
      expect(next.lastAutoSkippedId).toBeNull();
    });

    it('clears lastAutoSkippedId when QUESTION_RECEIVED for non-skipped concept', () => {
      const state = {
        ...baseState,
        phase: 'reviewing' as const,
        lastAutoSkippedId: 'concept_1' as Id<'concepts'>,
      };

      const payload = {
        question: { question: 'Q', options: ['A'], correctAnswer: 'A' },
        interactions: [],
        conceptId: 'concept_2' as Id<'concepts'>,
        conceptTitle: 'Concept 2',
        phrasingId: 'p1' as Id<'phrasings'>,
        phrasingStats: null,
        selectionReason: null,
        lockId: 'lock-456',
        conceptFsrs: { state: 'new' as const, reps: 0 },
      };

      const next = reviewReducer(state, { type: 'QUESTION_RECEIVED', payload });

      expect(next.lastAutoSkippedId).toBeNull();
      expect(next.conceptId).toBe('concept_2');
    });

    it('preserves skippedConceptIds across QUESTION_RECEIVED', () => {
      const state = {
        ...baseState,
        phase: 'reviewing' as const,
        skippedConceptIds: new Set(['concept_1', 'concept_3'] as Id<'concepts'>[]),
      };

      const payload = {
        question: { question: 'Q', options: ['A'], correctAnswer: 'A' },
        interactions: [],
        conceptId: 'concept_2' as Id<'concepts'>,
        conceptTitle: 'Concept 2',
        phrasingId: 'p1' as Id<'phrasings'>,
        phrasingStats: null,
        selectionReason: null,
        lockId: 'lock-456',
        conceptFsrs: { state: 'new' as const, reps: 0 },
      };

      const next = reviewReducer(state, { type: 'QUESTION_RECEIVED', payload });

      expect(next.skippedConceptIds.size).toBe(2);
      expect(next.skippedConceptIds.has('concept_1' as Id<'concepts'>)).toBe(true);
      expect(next.skippedConceptIds.has('concept_3' as Id<'concepts'>)).toBe(true);
    });

    describe('queue exhaustion detection', () => {
      it('enables exhaustion detection when same concept returns twice via AUTO_SKIP', () => {
        // Simulate: skip C1 → poll returns C1 → AUTO_SKIP → poll returns C1 again
        // At that point, lastAutoSkippedId === conceptId, signaling queue exhaustion

        // Step 1: User skips C1
        let state = reviewReducer(
          { ...baseState, phase: 'reviewing' as const, conceptId: 'concept_1' as Id<'concepts'> },
          { type: 'SKIP_CONCEPT', payload: 'concept_1' as Id<'concepts'> }
        );
        expect(state.skippedConceptIds.has('concept_1' as Id<'concepts'>)).toBe(true);

        // Step 2: Poll returns C1 (backend doesn't filter skipped), AUTO_SKIP fires
        state = reviewReducer(state, {
          type: 'AUTO_SKIP',
          payload: { conceptId: 'concept_1' as Id<'concepts'>, reason: 'in_skip_set' },
        });
        expect(state.lastAutoSkippedId).toBe('concept_1');

        // Step 3: Poll returns C1 AGAIN - effect would check: lastAutoSkippedId === conceptId
        // If true, dispatch CLEAR_SKIPPED. This test verifies the state enables that check.
        const conceptIdReturned = 'concept_1' as Id<'concepts'>;
        const queueExhausted = state.lastAutoSkippedId === conceptIdReturned;
        expect(queueExhausted).toBe(true);
      });

      it('does NOT signal exhaustion when different skipped concept returns', () => {
        // Skip C1 and C2, AUTO_SKIP for C1, then C2 returns - not exhausted
        let state = reviewReducer(
          { ...baseState, phase: 'reviewing' as const },
          { type: 'SKIP_CONCEPT', payload: 'concept_1' as Id<'concepts'> }
        );
        state = reviewReducer(state, {
          type: 'SKIP_CONCEPT',
          payload: 'concept_2' as Id<'concepts'>,
        });
        state = reviewReducer(state, {
          type: 'AUTO_SKIP',
          payload: { conceptId: 'concept_1' as Id<'concepts'>, reason: 'in_skip_set' },
        });

        // Now C2 returns - different from lastAutoSkippedId
        const conceptIdReturned = 'concept_2' as Id<'concepts'>;
        const queueExhausted = state.lastAutoSkippedId === conceptIdReturned;
        expect(queueExhausted).toBe(false);
        expect(state.lastAutoSkippedId).toBe('concept_1');
      });
    });

    it('clears skippedConceptIds on LOAD_EMPTY (session end)', () => {
      const state = {
        ...baseState,
        phase: 'reviewing' as const,
        skippedConceptIds: new Set(['concept_1', 'concept_2'] as Id<'concepts'>[]),
        lastAutoSkippedId: 'concept_1' as Id<'concepts'>,
      };

      const next = reviewReducer(state, { type: 'LOAD_EMPTY' });

      expect(next.phase).toBe('empty');
      expect(next.skippedConceptIds.size).toBe(0);
      expect(next.lastAutoSkippedId).toBeNull();
    });
  });
});
