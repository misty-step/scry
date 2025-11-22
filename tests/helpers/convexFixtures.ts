import { vi } from 'vitest';
import type { Doc, Id, TableNames } from '@/convex/_generated/dataModel';

// Stable timestamp for deterministic tests
export const fixedNow = new Date('2025-01-16T12:00:00Z').getTime();

const makeId = <T extends TableNames>(table: T, suffix = '1') => `${table}_${suffix}` as Id<T>;

type QuestionDoc = Doc<'questions'>;
type ConceptDoc = Doc<'concepts'>;
type GenerationJobDoc = Doc<'generationJobs'>;

export function makeQuestion(overrides: Partial<QuestionDoc> = {}): QuestionDoc {
  const base: QuestionDoc = {
    _id: makeId('questions'),
    _creationTime: fixedNow,
    userId: makeId('users'),
    question: 'What is ATP?',
    type: 'multiple-choice',
    options: ['A', 'B', 'C'],
    correctAnswer: 'A',
    generatedAt: fixedNow,
    attemptCount: 0,
    correctCount: 0,
  };

  return {
    ...base,
    ...overrides,
  };
}

export function makeConcept(overrides: Partial<ConceptDoc> = {}): ConceptDoc {
  const base: ConceptDoc = {
    _id: makeId('concepts'),
    _creationTime: fixedNow,
    userId: makeId('users'),
    title: 'Cellular Respiration',
    description: 'How cells produce ATP',
    phrasingCount: 0,
    conflictScore: undefined,
    thinScore: undefined,
    qualityScore: undefined,
    embedding: undefined,
    embeddingGeneratedAt: undefined,
    canonicalPhrasingId: undefined,
    fsrs: {
      stability: 1,
      difficulty: 1.5,
      nextReview: fixedNow,
      lastReview: undefined,
      elapsedDays: undefined,
      retrievability: undefined,
      scheduledDays: undefined,
      reps: undefined,
      lapses: undefined,
      state: 'new',
    },
    createdAt: fixedNow,
    updatedAt: undefined,
    deletedAt: undefined,
    archivedAt: undefined,
    generationJobId: undefined,
  };

  return {
    ...base,
    ...overrides,
  };
}

export function makeGenerationJob(overrides: Partial<GenerationJobDoc> = {}): GenerationJobDoc {
  const base: GenerationJobDoc = {
    _id: makeId('generationJobs'),
    _creationTime: fixedNow,
    userId: makeId('users'),
    prompt: 'Explain mitochondria',
    status: 'pending',
    phase: 'clarifying',
    questionsGenerated: 0,
    questionsSaved: 0,
    estimatedTotal: undefined,
    topic: undefined,
    questionIds: [],
    conceptIds: [],
    pendingConceptIds: [],
    durationMs: undefined,
    errorMessage: undefined,
    errorCode: undefined,
    retryable: undefined,
    createdAt: fixedNow,
    startedAt: undefined,
    completedAt: undefined,
    ipAddress: undefined,
  };

  return {
    ...base,
    ...overrides,
  };
}

// ------------------------------------------------------------
// Mock Convex db + ctx helpers
// ------------------------------------------------------------

export type MockDb = {
  get: ReturnType<typeof vi.fn>;
  insert: ReturnType<typeof vi.fn>;
  patch: ReturnType<typeof vi.fn>;
  replace: ReturnType<typeof vi.fn>;
  delete: ReturnType<typeof vi.fn>;
  query: ReturnType<typeof vi.fn>;
};

export function createQueryChain<T>(items: T[]) {
  const chain = {
    withIndex: vi.fn().mockReturnThis(),
    filter: vi.fn().mockReturnThis(),
    order: vi.fn().mockReturnThis(),
    paginate: vi.fn().mockReturnValue({ page: items, continueCursor: null, isDone: true }),
    collect: vi.fn().mockResolvedValue(items),
    first: vi.fn().mockResolvedValue(items[0] ?? null),
    take: vi.fn().mockResolvedValue(items),
  };
  return chain;
}

export function createMockDb(options: Partial<MockDb> = {}, queryItems: unknown[] = []): MockDb {
  const queryChain = createQueryChain(queryItems);

  const db: MockDb = {
    get: vi.fn(),
    insert: vi.fn(),
    patch: vi.fn(),
    replace: vi.fn(),
    delete: vi.fn(),
    query: vi.fn().mockReturnValue(queryChain),
    ...options,
  };

  return db;
}

export function createMockCtx(overrides: { db?: MockDb; scheduler?: unknown } = {}) {
  return {
    db: overrides.db ?? createMockDb(),
    scheduler: overrides.scheduler,
  };
}
