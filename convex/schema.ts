import { defineSchema, defineTable } from 'convex/server';
import { v } from 'convex/values';

export default defineSchema({
  users: defineTable({
    email: v.string(),
    clerkId: v.optional(v.string()), // Clerk user ID for auth integration
    name: v.optional(v.string()),
    emailVerified: v.optional(v.number()),
    image: v.optional(v.string()),
    currentStreak: v.optional(v.number()), // Consecutive days with >0 reviews
    lastStreakDate: v.optional(v.number()), // Last date streak was calculated
    createdAt: v.optional(v.number()), // TODO: make required after migration seeds all rows
  })
    .index('by_email', ['email'])
    .index('by_clerk_id', ['clerkId'])
    .index('by_created_at', ['createdAt']),

  // Cached card statistics per user (O(1) reads vs O(N) collection scans)
  // Updated incrementally on card state transitions for bandwidth optimization
  userStats: defineTable({
    userId: v.id('users'),
    totalCards: v.number(), // Total non-deleted cards
    newCount: v.number(), // Cards in 'new' state
    learningCount: v.number(), // Cards in 'learning' state
    matureCount: v.number(), // Cards in 'review' state
    dueNowCount: v.number(), // Cards where nextReview <= now (maintained by scheduleReview)
    nextReviewTime: v.optional(v.number()), // Earliest nextReview timestamp across all cards
    lastCalculated: v.number(), // Timestamp of last stats update
  }).index('by_user', ['userId']),

  interactions: defineTable({
    userId: v.id('users'),
    conceptId: v.optional(v.id('concepts')),
    phrasingId: v.optional(v.id('phrasings')),
    userAnswer: v.string(),
    isCorrect: v.boolean(),
    attemptedAt: v.number(),
    sessionId: v.optional(v.string()),
    timeSpent: v.optional(v.number()), // milliseconds
    context: v.optional(
      v.object({
        sessionId: v.optional(v.string()), // for grouping quiz attempts
        isRetry: v.optional(v.boolean()),
        scheduledDays: v.optional(v.number()), // FSRS interval chosen for this attempt
        nextReview: v.optional(v.number()), // Next review timestamp recorded at scheduling time
        fsrsState: v.optional(
          v.union(
            v.literal('new'),
            v.literal('learning'),
            v.literal('review'),
            v.literal('relearning')
          )
        ),
      })
    ),
  })
    .index('by_user', ['userId', 'attemptedAt'])
    .index('by_user_session', ['userId', 'sessionId', 'attemptedAt'])
    .index('by_user_concept', ['userId', 'conceptId', 'attemptedAt'])
    .index('by_user_phrasing', ['userId', 'phrasingId', 'attemptedAt'])
    .index('by_concept', ['conceptId', 'attemptedAt']),

  deployments: defineTable({
    environment: v.string(), // 'development' | 'production' | 'preview'
    deployedBy: v.optional(v.string()), // User email or system identifier
    commitSha: v.optional(v.string()), // Git commit SHA
    commitMessage: v.optional(v.string()), // Git commit message
    branch: v.optional(v.string()), // Git branch name
    deploymentType: v.string(), // 'manual' | 'ci' | 'scheduled'
    status: v.string(), // 'started' | 'success' | 'failed'
    schemaVersion: v.optional(v.string()), // Schema version identifier
    functionCount: v.optional(v.number()), // Number of functions deployed
    duration: v.optional(v.number()), // Deployment duration in ms
    error: v.optional(v.string()), // Error message if failed
    metadata: v.optional(
      v.object({
        buildId: v.optional(v.string()),
        vercelDeploymentId: v.optional(v.string()),
        convexVersion: v.optional(v.string()),
        nodeVersion: v.optional(v.string()),
      })
    ),
    deployedAt: v.number(), // Timestamp
  })
    .index('by_environment', ['environment', 'deployedAt'])
    .index('by_status', ['status', 'deployedAt'])
    .index('by_branch', ['branch', 'deployedAt']),

  rateLimits: defineTable({
    identifier: v.string(), // Email address or IP address
    operation: v.string(), // Type of operation (magicLink, questionGeneration, etc.)
    timestamp: v.number(), // When the attempt was made
    metadata: v.optional(v.any()), // Additional data about the attempt
  })
    .index('by_identifier', ['identifier', 'timestamp'])
    .index('by_operation', ['operation', 'timestamp']),

  generationJobs: defineTable({
    // Ownership
    userId: v.id('users'),

    // Input
    prompt: v.string(), // Raw user input (sanitized)

    // Status (simple state machine)
    status: v.union(
      v.literal('pending'),
      v.literal('processing'),
      v.literal('completed'),
      v.literal('failed'),
      v.literal('cancelled')
    ),

    // Progress (flat fields, no nesting)
    phase: v.union(
      v.literal('clarifying'),
      v.literal('concept_synthesis'),
      v.literal('generating'),
      v.literal('phrasing_generation'),
      v.literal('finalizing')
    ),
    phrasingGenerated: v.number(), // Total AI generated
    phrasingSaved: v.number(), // Successfully saved to DB
    estimatedTotal: v.optional(v.number()), // AI's estimate

    // Results (flat fields)
    // Note: topic field kept here for generation metadata/grouping
    // Removed from questions table (PR #44) but still used here for job classification
    topic: v.optional(v.string()), // Extracted topic
    conceptIds: v.optional(v.array(v.id('concepts'))), // Concepts generated in Stage A (optional for backward compat)
    pendingConceptIds: v.optional(v.array(v.id('concepts'))), // Concepts remaining for Stage B (optional for backward compat)
    durationMs: v.optional(v.number()), // Total generation time

    // Error handling (flat fields)
    errorMessage: v.optional(v.string()),
    errorCode: v.optional(v.string()), // 'RATE_LIMIT' | 'API_KEY' | 'NETWORK' | 'UNKNOWN'
    retryable: v.optional(v.boolean()),

    // Timestamps
    createdAt: v.number(),
    startedAt: v.optional(v.number()),
    completedAt: v.optional(v.number()),

    // Rate limiting
    ipAddress: v.optional(v.string()),
  })
    .index('by_user_status', ['userId', 'status', 'createdAt'])
    .index('by_status_created', ['status', 'createdAt']),

  // ============================================================================
  // Concepts & Phrasings Architecture (v2.3.0)
  // ============================================================================
  // New table architecture supporting atomic knowledge concepts with multiple
  // phrasing variations. FSRS scheduling moves to concept-level to eliminate
  // duplicate scheduling pressure from near-identical questions.
  //
  // Migration Path (3-phase):
  // - Phase 1: Add tables + optional conceptId to questions (this deployment)
  // - Phase 2: Run backfill migration (1 concept per existing question)
  // - Phase 3: Enforce conceptId requirement, deprecate question-level FSRS

  // Concepts: Atomic units of knowledge with concept-level FSRS scheduling
  concepts: defineTable({
    userId: v.id('users'),
    title: v.string(), // Concise, user-facing concept name
    description: v.optional(v.string()), // Optional detailed explanation
    contentType: v.optional(
      v.union(
        v.literal('verbatim'),
        v.literal('enumerable'),
        v.literal('conceptual'),
        v.literal('mixed')
      )
    ), // Learning content classification for generation/routing
    originIntent: v.optional(v.string()), // Serialized intent object that produced this concept

    // FSRS state (single source of truth for scheduling)
    fsrs: v.object({
      stability: v.number(),
      difficulty: v.number(),
      lastReview: v.optional(v.number()),
      nextReview: v.number(),
      elapsedDays: v.optional(v.number()),
      retrievability: v.optional(v.number()),
      scheduledDays: v.optional(v.number()),
      reps: v.optional(v.number()),
      lapses: v.optional(v.number()),
      state: v.optional(
        v.union(
          v.literal('new'),
          v.literal('learning'),
          v.literal('review'),
          v.literal('relearning')
        )
      ),
    }),
    canonicalPhrasingId: v.optional(v.id('phrasings')),

    // IQC (Intelligent Quality Control) signals
    phrasingCount: v.number(), // Number of phrasings for this concept
    conflictScore: v.optional(v.number()), // Heuristic for "overloaded" concepts
    thinScore: v.optional(v.number()), // Heuristic for "needs more phrasings"
    qualityScore: v.optional(v.number()), // Overall quality signal

    // Vector embeddings for semantic search and clustering
    embedding: v.optional(v.array(v.float64())), // 768-dim from text-embedding-004
    embeddingGeneratedAt: v.optional(v.number()),

    // Timestamps
    createdAt: v.number(),
    updatedAt: v.optional(v.number()),
    deletedAt: v.optional(v.number()), // Soft delete support
    archivedAt: v.optional(v.number()), // Archive support
    generationJobId: v.optional(v.id('generationJobs')),
  })
    .index('by_user', ['userId', 'createdAt'])
    .index('by_user_next_review', ['userId', 'deletedAt', 'archivedAt', 'fsrs.nextReview'])
    .index('by_user_active', ['userId', 'deletedAt', 'archivedAt', 'createdAt'])
    .vectorIndex('by_embedding', {
      vectorField: 'embedding',
      dimensions: 768,
      filterFields: ['userId'],
    })
    .searchIndex('search_concepts', {
      searchField: 'title',
      filterFields: ['userId'],
    }),

  // Phrasings: Different ways to test the same concept
  phrasings: defineTable({
    userId: v.id('users'),
    conceptId: v.id('concepts'),

    // Question content (preserves compatibility with existing question schema)
    question: v.string(),
    explanation: v.optional(v.string()),
    type: v.optional(
      v.union(
        v.literal('multiple-choice'),
        v.literal('true-false'),
        v.literal('cloze'),
        v.literal('short-answer') // Scaffold for future free-response
      )
    ),
    options: v.optional(v.array(v.string())), // For MCQ
    correctAnswer: v.optional(v.string()),

    // Local attempt statistics (analytics only, not scheduling)
    attemptCount: v.optional(v.number()),
    correctCount: v.optional(v.number()),
    lastAttemptedAt: v.optional(v.number()),

    // Soft delete and update tracking
    createdAt: v.number(),
    updatedAt: v.optional(v.number()),
    archivedAt: v.optional(v.number()),
    deletedAt: v.optional(v.number()),

    // Vector embeddings per phrasing (for similarity detection)
    embedding: v.optional(v.array(v.float64())),
    embeddingGeneratedAt: v.optional(v.number()),
  })
    .index('by_user_concept', ['userId', 'conceptId', 'createdAt'])
    .index('by_user_active', ['userId', 'deletedAt', 'archivedAt', 'createdAt'])
    .searchIndex('search_phrasings', {
      searchField: 'question',
      filterFields: ['userId', 'deletedAt', 'archivedAt'],
    })
    .vectorIndex('by_embedding', {
      vectorField: 'embedding',
      dimensions: 768,
      filterFields: ['userId', 'deletedAt', 'archivedAt'],
    }),

  // Reclustering Jobs: Track IQC background processing
  reclusterJobs: defineTable({
    userId: v.id('users'),
    status: v.union(
      v.literal('queued'),
      v.literal('running'),
      v.literal('done'),
      v.literal('failed')
    ),
    createdAt: v.number(),
    completedAt: v.optional(v.number()),
    stats: v.optional(v.any()), // Job statistics (concepts processed, proposals created, etc.)
  }).index('by_user_status', ['userId', 'status', 'createdAt']),

  // Action Cards: IQC proposals for user review
  actionCards: defineTable({
    userId: v.id('users'),
    kind: v.union(
      v.literal('MERGE_CONCEPTS'), // Duplicate concepts detected
      v.literal('SPLIT_CONCEPT'), // Overloaded concept detected
      v.literal('ASSIGN_ORPHANS'), // Orphaned phrasings need concept
      v.literal('FILL_OUT_CONCEPT'), // Thin concept needs more phrasings
      v.literal('RENAME_CONCEPT') // Ambiguous concept title
    ),
    payload: v.any(), // Concrete proposal (concept IDs, reason strings, preview diffs)

    // Lifecycle
    createdAt: v.number(),
    expiresAt: v.optional(v.number()), // Auto-expire stale proposals
    resolvedAt: v.optional(v.number()), // When user accepted/rejected
    resolution: v.optional(v.union(v.literal('accepted'), v.literal('rejected'))),
  }).index('by_user_open', ['userId', 'resolvedAt', 'createdAt']),
});
