import { describe, expect, it } from 'vitest';
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
});
