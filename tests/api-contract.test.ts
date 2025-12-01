import { describe, expect, it } from 'vitest';
import { api } from '@/convex/_generated/api';

/**
 * API Contract Tests
 *
 * These tests verify that all mutations and queries referenced in the frontend
 * actually exist in the backend. This prevents runtime errors from frontend-backend
 * contract mismatches.
 *
 * IMPORTANT: Update these tests when adding new features that require backend mutations.
 */

describe('API Contract: Generation Jobs', () => {
  it('generation mutations and queries exist', () => {
    expect(api.generationJobs.createJob).toBeDefined();
    expect(api.generationJobs.cancelJob).toBeDefined();
    expect(api.generationJobs.getRecentJobs).toBeDefined();
    expect(api.generationJobs.getJobById).toBeDefined();
  });
});

describe('API Contract: Concepts & Review', () => {
  it('core concept and review endpoints exist', () => {
    // Review pipeline
    expect(api.concepts.getDue).toBeDefined();
    expect(api.concepts.getConceptsDueCount).toBeDefined();
    expect(api.concepts.recordInteraction).toBeDefined();

    // Library/detail views
    expect(api.concepts.listForLibrary).toBeDefined();
    expect(api.concepts.getDetail).toBeDefined();
  });
});

describe('API Contract: Spaced Repetition Stats', () => {
  it('spaced repetition queries exist', () => {
    expect(api.spacedRepetition.getDueCount).toBeDefined();
    expect(api.spacedRepetition.getUserCardStats).toBeDefined();
  });
});
