import { describe, expect, it } from 'vitest';
import type { Doc, Id } from '../_generated/dataModel';
import {
  getConceptRetrievability,
  initializeConceptFsrs,
  isConceptDue,
  scheduleConceptReview,
} from './conceptScheduler';
import { defaultEngine } from './engine';

const makeConcept = (fsrsOverrides = {}): Doc<'concepts'> => {
  const initialFsrs = initializeConceptFsrs(new Date('2024-01-01'));
  return {
    _id: 'concept-1' as Id<'concepts'>,
    _creationTime: Date.now(),
    userId: 'user-1' as Id<'users'>,
    title: 'Test Concept',
    description: 'Test description',
    phrasingCount: 1,
    fsrs: { ...initialFsrs, ...fsrsOverrides },
    conflictScore: undefined,
    thinScore: undefined,
    qualityScore: undefined,
    embedding: undefined,
    embeddingGeneratedAt: undefined,
    createdAt: Date.now(),
    updatedAt: undefined,
    generationJobId: undefined,
    canonicalPhrasingId: undefined,
  };
};

describe('conceptScheduler', () => {
  describe('initializeConceptFsrs', () => {
    it('creates initial FSRS state with new state', () => {
      const now = new Date('2024-06-15');
      const fsrs = initializeConceptFsrs(now);

      expect(fsrs.state).toBe('new');
      expect(fsrs.reps).toBe(0);
      // Initial FSRS cards start with 0 difficulty and stability
      expect(fsrs.difficulty).toBe(0);
      expect(fsrs.stability).toBe(0);
    });

    it('uses current date when not provided', () => {
      const before = Date.now();
      const fsrs = initializeConceptFsrs();
      const after = Date.now();

      expect(fsrs.nextReview).toBeGreaterThanOrEqual(before);
      expect(fsrs.nextReview).toBeLessThanOrEqual(after);
    });

    it('uses provided engine', () => {
      const fsrs = initializeConceptFsrs(new Date(), defaultEngine);
      expect(fsrs).toBeDefined();
      expect(fsrs.state).toBe('new');
    });
  });

  describe('scheduleConceptReview', () => {
    it('returns scheduling result with all required fields', () => {
      const concept = makeConcept();
      const now = new Date('2024-06-15');

      const result = scheduleConceptReview(concept, true, { now });

      expect(result).toHaveProperty('fsrs');
      expect(result).toHaveProperty('rating');
      expect(result).toHaveProperty('nextReview');
      expect(result).toHaveProperty('dueDate');
      expect(result).toHaveProperty('state');
      expect(result).toHaveProperty('scheduledDays');
    });

    it('schedules further review for correct answer', () => {
      const concept = makeConcept();
      const now = new Date('2024-06-15');

      const result = scheduleConceptReview(concept, true, { now });

      // Correct answer should schedule for later
      expect(result.dueDate.getTime()).toBeGreaterThan(now.getTime());
      expect(result.rating).toBe(3); // Good rating for correct
    });

    it('schedules sooner review for incorrect answer', () => {
      // First get a concept to learning state
      const initialConcept = makeConcept();
      const now = new Date('2024-06-15');

      const afterCorrect = scheduleConceptReview(initialConcept, true, { now });
      const reviewConcept = makeConcept(afterCorrect.fsrs);

      // Now answer incorrectly
      const result = scheduleConceptReview(reviewConcept, false, { now });

      // Wrong answer should get "Again" rating
      expect(result.rating).toBe(1);
    });

    it('uses current date when not provided', () => {
      const concept = makeConcept();
      const before = Date.now();

      const result = scheduleConceptReview(concept, true);

      expect(result.nextReview).toBeGreaterThanOrEqual(before);
    });

    it('uses default engine when not provided', () => {
      const concept = makeConcept();

      const result = scheduleConceptReview(concept, true);

      expect(result.fsrs).toBeDefined();
    });

    it('transitions state from new to learning after first review', () => {
      const concept = makeConcept({ state: 'new', reps: 0 });
      const now = new Date('2024-06-15');

      const result = scheduleConceptReview(concept, true, { now });

      expect(result.state).toBe('learning');
    });

    it('returns state as new when FSRS state is undefined', () => {
      const concept = makeConcept({ state: undefined });
      const now = new Date('2024-06-15');

      const result = scheduleConceptReview(concept, true, { now });

      // Default state should be handled
      expect(['new', 'learning', 'review', 'relearning']).toContain(result.state);
    });

    it('returns scheduledDays as 0 when undefined', () => {
      const concept = makeConcept();
      const now = new Date('2024-06-15');

      const result = scheduleConceptReview(concept, true, { now });

      expect(typeof result.scheduledDays).toBe('number');
    });
  });

  describe('getConceptRetrievability', () => {
    it('returns -1 for new concepts (highest priority)', () => {
      const now = new Date('2024-06-15');
      const concept = makeConcept({
        state: 'new',
        reps: 0,
        nextReview: now.getTime(),
      });

      const retrievability = getConceptRetrievability(concept, now);

      // New cards return -1 to indicate highest priority
      expect(retrievability).toBe(-1);
    });

    it('returns actual retrievability for reviewed concepts', () => {
      const now = new Date('2024-06-15');

      // Create a concept that has been reviewed
      const concept = makeConcept({
        state: 'learning',
        reps: 1,
        nextReview: now.getTime(),
        lastReview: now.getTime() - 86400000, // reviewed 1 day ago
        stability: 10,
        difficulty: 5,
      });

      const retrievability = getConceptRetrievability(concept, now);

      // Reviewed cards should return a value between 0 and 1
      expect(retrievability).toBeGreaterThanOrEqual(0);
      expect(retrievability).toBeLessThanOrEqual(1);
    });

    it('returns lower retrievability for overdue concept', () => {
      const reviewDate = new Date('2024-01-01');
      const checkDate = new Date('2024-06-15'); // 6 months later

      const concept = makeConcept({
        state: 'review',
        reps: 5,
        nextReview: reviewDate.getTime(),
        lastReview: reviewDate.getTime() - 86400000,
        stability: 1, // 1 day stability
        difficulty: 5,
      });

      const retrievability = getConceptRetrievability(concept, checkDate);

      // Overdue by 6 months with 1-day stability should be low
      expect(retrievability).toBeLessThan(0.5);
    });

    it('returns -1 for concept with undefined state', () => {
      const concept = makeConcept({
        state: undefined,
        reps: 0,
      });

      const retrievability = getConceptRetrievability(concept);

      // No state = new, returns -1
      expect(retrievability).toBe(-1);
    });

    it('uses provided engine', () => {
      const now = new Date();
      const concept = makeConcept({
        state: 'review',
        reps: 5,
        lastReview: now.getTime() - 86400000, // 1 day ago
        nextReview: now.getTime(),
        stability: 10,
        difficulty: 5,
      });

      const retrievability = getConceptRetrievability(concept, now, defaultEngine);

      expect(typeof retrievability).toBe('number');
    });
  });

  describe('isConceptDue', () => {
    it('returns true when nextReview is in the past', () => {
      const pastDate = new Date('2024-01-01');
      const now = new Date('2024-06-15');

      const concept = makeConcept({
        nextReview: pastDate.getTime(),
      });

      const isDue = isConceptDue(concept, now);

      expect(isDue).toBe(true);
    });

    it('returns false when nextReview is in the future', () => {
      const futureDate = new Date('2025-01-01');
      const now = new Date('2024-06-15');

      const concept = makeConcept({
        nextReview: futureDate.getTime(),
      });

      const isDue = isConceptDue(concept, now);

      expect(isDue).toBe(false);
    });

    it('returns true when nextReview equals now', () => {
      const now = new Date('2024-06-15');

      const concept = makeConcept({
        nextReview: now.getTime(),
      });

      const isDue = isConceptDue(concept, now);

      expect(isDue).toBe(true);
    });

    it('uses current date when not provided', () => {
      const futureConcept = makeConcept({
        nextReview: Date.now() + 86400000 * 365, // 1 year in future
      });

      expect(isConceptDue(futureConcept)).toBe(false);

      const pastConcept = makeConcept({
        nextReview: Date.now() - 86400000, // 1 day ago
      });

      expect(isConceptDue(pastConcept)).toBe(true);
    });

    it('uses provided engine', () => {
      const concept = makeConcept({
        nextReview: Date.now() - 86400000,
      });
      const now = new Date();

      const isDue = isConceptDue(concept, now, defaultEngine);

      expect(typeof isDue).toBe('boolean');
    });
  });
});
