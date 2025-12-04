import { describe, expect, it, vi } from 'vitest';
import type { Doc, Id } from './_generated/dataModel';
import { initializeConceptFsrs } from './fsrs';
import {
  accumulateStatDelta,
  buildMergePayload,
  buildMergePrompt,
  buildProposalKey,
  computeTitleSimilarity,
  shouldConsiderMerge,
  snapshotConcept,
  tokenizeTitle,
  type MergeCandidate,
  type MergeDecision,
} from './iqc';
import { DEFAULT_REPLAY_LIMIT, replayInteractionsIntoState } from './lib/fsrsReplay';
import { logConceptEvent, type ConceptsLogger } from './lib/logger';

describe('IQC helpers', () => {
  describe('buildProposalKey', () => {
    it('produces stable key regardless of order', () => {
      const keyA = buildProposalKey('1' as any, '2' as any);
      const keyB = buildProposalKey('2' as any, '1' as any);

      expect(keyA).toBe(keyB);
      expect(keyA).toBe('1::2');
    });
  });

  describe('tokenizeTitle', () => {
    it('lowercases, strips punctuation, and filters short tokens', () => {
      const tokens = tokenizeTitle('Hello, an AI-based world!');
      expect(tokens).toEqual(new Set(['hello', 'based', 'world']));
    });
  });

  describe('accumulateStatDelta', () => {
    it('ignores nullish deltas', () => {
      const target: any = { totalCards: 1 };
      accumulateStatDelta(target, null);
      accumulateStatDelta(target, undefined);
      expect(target).toEqual({ totalCards: 1 });
    });

    it('sums numeric deltas per key', () => {
      const target: any = { totalCards: 1, newCount: 2 };
      accumulateStatDelta(target, { totalCards: -1, newCount: 3, matureCount: 5 });
      expect(target).toEqual({ totalCards: 0, newCount: 5, matureCount: 5 });
    });
  });

  describe('computeTitleSimilarity', () => {
    it('returns 1.0 for identical titles ignoring case/punctuation', () => {
      const similarity = computeTitleSimilarity('Grace & Nature', 'grace nature!!');
      expect(similarity).toBeCloseTo(1);
    });

    it('returns 0 when tokens do not overlap', () => {
      const similarity = computeTitleSimilarity('Eucharistic theology', 'Quantum entanglement');
      expect(similarity).toBe(0);
    });
  });

  describe('shouldConsiderMerge', () => {
    it('accepts very high vector score even with modest title overlap', () => {
      const result = shouldConsiderMerge(0.98, 0.2, 5, 4);
      expect(result).toBe(true);
    });

    it('rejects low similarity even if phrasing counts small', () => {
      const result = shouldConsiderMerge(0.85, 0.4, 2, 1);
      expect(result).toBe(false);
    });

    it('accepts thin concepts when both heuristics moderately high', () => {
      const result = shouldConsiderMerge(0.945, 0.55, 1, 1);
      expect(result).toBe(true);
    });

    it('accepts vector score >= 0.97 regardless of title similarity', () => {
      expect(shouldConsiderMerge(0.97, 0.1, 10, 10)).toBe(true);
      expect(shouldConsiderMerge(0.99, 0, 10, 10)).toBe(true);
    });

    it('rejects scores below minimum vector threshold', () => {
      expect(shouldConsiderMerge(0.91, 0.9, 1, 1)).toBe(false);
      expect(shouldConsiderMerge(0.89, 1.0, 1, 1)).toBe(false);
    });

    it('accepts high title similarity with moderate vector score', () => {
      expect(shouldConsiderMerge(0.93, 0.7, 5, 5)).toBe(true);
      expect(shouldConsiderMerge(0.92, 0.75, 3, 3)).toBe(true);
    });

    it('rejects thin concepts heuristic when not both thin', () => {
      expect(shouldConsiderMerge(0.945, 0.55, 3, 1)).toBe(false);
      expect(shouldConsiderMerge(0.945, 0.55, 1, 4)).toBe(false);
    });
  });

  describe('snapshotConcept', () => {
    const makeConcept = (): Doc<'concepts'> => ({
      _id: 'concept123' as Id<'concepts'>,
      _creationTime: Date.now(),
      userId: 'user1' as Id<'users'>,
      title: 'Test Concept',
      description: 'A test description',
      fsrs: initializeConceptFsrs(new Date('2024-01-01')),
      phrasingCount: 5,
      conflictScore: 0.2,
      thinScore: 0.8,
      qualityScore: undefined,
      embedding: undefined,
      embeddingGeneratedAt: undefined,
      createdAt: Date.now(),
      updatedAt: undefined,
      generationJobId: undefined,
      canonicalPhrasingId: undefined,
    });

    it('extracts relevant fields from concept document', () => {
      const concept = makeConcept();
      const snapshot = snapshotConcept(concept);

      expect(snapshot.conceptId).toBe('concept123');
      expect(snapshot.title).toBe('Test Concept');
      expect(snapshot.description).toBe('A test description');
      expect(snapshot.phrasingCount).toBe(5);
      expect(snapshot.conflictScore).toBe(0.2);
      expect(snapshot.thinScore).toBe(0.8);
      expect(snapshot.fsrs).toEqual(concept.fsrs);
    });

    it('handles undefined optional fields', () => {
      const concept = makeConcept();
      concept.description = undefined;
      concept.conflictScore = undefined;
      concept.thinScore = undefined;

      const snapshot = snapshotConcept(concept);

      expect(snapshot.description).toBeUndefined();
      expect(snapshot.conflictScore).toBeUndefined();
      expect(snapshot.thinScore).toBeUndefined();
    });
  });

  describe('buildMergePrompt', () => {
    const makeCandidate = (): MergeCandidate => ({
      source: {
        _id: 'source1' as Id<'concepts'>,
        _creationTime: Date.now(),
        userId: 'user1' as Id<'users'>,
        title: 'Grace and Nature',
        description: 'Relationship between divine grace and human nature',
        phrasingCount: 3,
        fsrs: initializeConceptFsrs(new Date('2024-01-01')),
        conflictScore: undefined,
        thinScore: undefined,
        qualityScore: undefined,
        embedding: undefined,
        embeddingGeneratedAt: undefined,
        createdAt: Date.now(),
        updatedAt: undefined,
        generationJobId: undefined,
        canonicalPhrasingId: undefined,
      },
      target: {
        _id: 'target1' as Id<'concepts'>,
        _creationTime: Date.now(),
        userId: 'user1' as Id<'users'>,
        title: 'Nature and Grace',
        description: 'How grace perfects nature',
        phrasingCount: 5,
        fsrs: initializeConceptFsrs(new Date('2024-01-01')),
        conflictScore: undefined,
        thinScore: undefined,
        qualityScore: undefined,
        embedding: undefined,
        embeddingGeneratedAt: undefined,
        createdAt: Date.now(),
        updatedAt: undefined,
        generationJobId: undefined,
        canonicalPhrasingId: undefined,
      },
      vectorScore: 0.95,
      titleSimilarity: 0.85,
    });

    it('includes source and target titles in prompt', () => {
      const candidate = makeCandidate();
      const prompt = buildMergePrompt(candidate);

      expect(prompt).toContain('Grace and Nature');
      expect(prompt).toContain('Nature and Grace');
    });

    it('includes source and target descriptions', () => {
      const candidate = makeCandidate();
      const prompt = buildMergePrompt(candidate);

      expect(prompt).toContain('Relationship between divine grace and human nature');
      expect(prompt).toContain('How grace perfects nature');
    });

    it('shows n/a for missing descriptions', () => {
      const candidate = makeCandidate();
      candidate.source.description = undefined;
      const prompt = buildMergePrompt(candidate);

      expect(prompt).toContain('Description: n/a');
    });

    it('includes phrasing counts', () => {
      const candidate = makeCandidate();
      const prompt = buildMergePrompt(candidate);

      expect(prompt).toContain('Phrasing count: 3');
      expect(prompt).toContain('Phrasing count: 5');
    });

    it('formats similarity scores to 3 decimal places', () => {
      const candidate = makeCandidate();
      const prompt = buildMergePrompt(candidate);

      expect(prompt).toContain('Vector similarity: 0.950');
      expect(prompt).toContain('Title similarity: 0.850');
    });

    it('includes instruction keywords for LLM', () => {
      const candidate = makeCandidate();
      const prompt = buildMergePrompt(candidate);

      expect(prompt).toContain('MERGE');
      expect(prompt).toContain('KEEP');
      expect(prompt).toContain('SOURCE');
      expect(prompt).toContain('TARGET');
      expect(prompt).toContain('duplicates');
    });
  });

  describe('buildMergePayload', () => {
    const makeCandidate = (): MergeCandidate => ({
      source: {
        _id: 'source1' as Id<'concepts'>,
        _creationTime: Date.now(),
        userId: 'user1' as Id<'users'>,
        title: 'Source Concept',
        description: 'Source desc',
        phrasingCount: 3,
        fsrs: initializeConceptFsrs(new Date('2024-01-01')),
        conflictScore: 0.1,
        thinScore: 0.5,
        qualityScore: undefined,
        embedding: undefined,
        embeddingGeneratedAt: undefined,
        createdAt: Date.now(),
        updatedAt: undefined,
        generationJobId: undefined,
        canonicalPhrasingId: undefined,
      },
      target: {
        _id: 'target1' as Id<'concepts'>,
        _creationTime: Date.now(),
        userId: 'user1' as Id<'users'>,
        title: 'Target Concept',
        description: 'Target desc',
        phrasingCount: 5,
        fsrs: initializeConceptFsrs(new Date('2024-01-01')),
        conflictScore: 0.2,
        thinScore: 0.6,
        qualityScore: undefined,
        embedding: undefined,
        embeddingGeneratedAt: undefined,
        createdAt: Date.now(),
        updatedAt: undefined,
        generationJobId: undefined,
        canonicalPhrasingId: undefined,
      },
      vectorScore: 0.95,
      titleSimilarity: 0.85,
    });

    const makeDecision = (canonical: 'SOURCE' | 'TARGET'): MergeDecision => ({
      decision: 'MERGE',
      reason: 'These concepts are clearly duplicates',
      confidence: 0.9,
      canonical,
    });

    const keyDiagnostics = { present: true, length: 32, fingerprint: 'abc123' };

    it('sets canonical to target when decision prefers TARGET', () => {
      const candidate = makeCandidate();
      const decision = makeDecision('TARGET');
      const payload = buildMergePayload(candidate, decision, 'key1', 'google', keyDiagnostics);

      expect(payload.canonicalConceptId).toBe('target1');
      expect(payload.mergeConceptId).toBe('source1');
    });

    it('sets canonical to source when decision prefers SOURCE', () => {
      const candidate = makeCandidate();
      const decision = makeDecision('SOURCE');
      const payload = buildMergePayload(candidate, decision, 'key2', 'google', keyDiagnostics);

      expect(payload.canonicalConceptId).toBe('source1');
      expect(payload.mergeConceptId).toBe('target1');
    });

    it('includes similarity scores', () => {
      const candidate = makeCandidate();
      const decision = makeDecision('TARGET');
      const payload = buildMergePayload(candidate, decision, 'key', 'google', keyDiagnostics);

      expect(payload.similarity).toBe(0.95);
      expect(payload.titleSimilarity).toBe(0.85);
    });

    it('includes concept snapshots for both concepts', () => {
      const candidate = makeCandidate();
      const decision = makeDecision('TARGET');
      const payload = buildMergePayload(candidate, decision, 'key', 'google', keyDiagnostics);

      expect(payload.conceptSnapshots).toHaveLength(2);
      expect(payload.conceptSnapshots[0].title).toBe('Source Concept');
      expect(payload.conceptSnapshots[1].title).toBe('Target Concept');
    });

    it('includes LLM decision metadata', () => {
      const candidate = makeCandidate();
      const decision = makeDecision('TARGET');
      const payload = buildMergePayload(candidate, decision, 'key', 'google', keyDiagnostics);

      expect(payload.llmDecision.provider).toBe('google');
      expect(payload.llmDecision.reason).toBe('These concepts are clearly duplicates');
      expect(payload.llmDecision.confidence).toBe(0.9);
      expect(payload.llmDecision.canonicalPreference).toBe('TARGET');
      expect(payload.llmDecision.keyDiagnostics).toEqual(keyDiagnostics);
    });

    it('preserves proposal key', () => {
      const candidate = makeCandidate();
      const decision = makeDecision('SOURCE');
      const payload = buildMergePayload(
        candidate,
        decision,
        'unique-key-123',
        'google',
        keyDiagnostics
      );

      expect(payload.proposalKey).toBe('unique-key-123');
    });
  });

  describe('replayInteractionsIntoState', () => {
    const baseConcept = (): Doc<'concepts'> => ({
      _id: 'concept1' as Id<'concepts'>,
      _creationTime: Date.now(),
      userId: 'user1' as Id<'users'>,
      title: 'Concept',
      description: undefined,
      fsrs: initializeConceptFsrs(new Date('2024-01-01')),
      phrasingCount: 1,
      conflictScore: undefined,
      thinScore: undefined,
      qualityScore: undefined,
      embedding: undefined,
      embeddingGeneratedAt: undefined,
      createdAt: Date.now(),
      updatedAt: undefined,
      generationJobId: undefined,
      canonicalPhrasingId: undefined,
    });

    const makeInteraction = (
      attemptedAt: number,
      isCorrect: boolean,
      idSuffix: string
    ): Doc<'interactions'> => ({
      _id: `interaction-${idSuffix}` as Id<'interactions'>,
      _creationTime: attemptedAt,
      userId: 'user1' as Id<'users'>,
      conceptId: 'concept2' as Id<'concepts'>,
      phrasingId: undefined,
      userAnswer: isCorrect ? 'correct' : 'incorrect',
      isCorrect,
      attemptedAt,
      timeSpent: undefined,
      context: undefined,
    });

    it('applies interactions in chronological order', () => {
      const concept = baseConcept();
      const interactions = [
        makeInteraction(Date.parse('2024-01-02'), false, 'b'),
        makeInteraction(Date.parse('2024-01-03'), true, 'c'),
        makeInteraction(Date.parse('2024-01-01'), true, 'a'),
      ];

      const result = replayInteractionsIntoState(concept, interactions);
      expect(result.applied).toBe(interactions.length);
      expect(result.fsrs.lastReview).toBeGreaterThan(concept.fsrs.lastReview ?? 0);
    });

    it('respects replay limit', () => {
      const concept = baseConcept();
      const interactions = Array.from({ length: DEFAULT_REPLAY_LIMIT + 5 }).map((_, idx) =>
        makeInteraction(Date.now() + idx * 1000, true, String(idx))
      );

      const result = replayInteractionsIntoState(concept, interactions, { limit: 5 });
      expect(result.applied).toBe(5);
    });
  });
});

describe('IQC logging', () => {
  it('includes action card metadata in apply logs', () => {
    const infoSpy = vi.fn();
    const stubLogger: ConceptsLogger = {
      info: infoSpy,
      warn: vi.fn(),
      error: vi.fn(),
      debug: vi.fn(),
    };

    logConceptEvent(stubLogger, 'info', 'IQC merge apply completed', {
      phase: 'iqc_apply',
      event: 'completed',
      correlationId: 'corr-iqc-apply',
      actionCardId: 'card_123',
      conceptIds: ['concept_a', 'concept_b'],
      movedPhrasings: 4,
    });

    expect(infoSpy).toHaveBeenCalledTimes(1);
    const [message, context] = infoSpy.mock.calls[0];
    expect(message).toBe('IQC merge apply completed');
    expect(context?.event).toBe('concepts.iqc_apply.completed');
    expect(context?.actionCardId).toBe('card_123');
    expect(context?.conceptIds).toEqual(['concept_a', 'concept_b']);
    expect(context?.correlationId).toBe('corr-iqc-apply');
  });
});
