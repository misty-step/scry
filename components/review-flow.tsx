/** @jest-environment jsdom */
'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQuery } from 'convex/react';
import { ArrowRight, Brain, Calendar, Clock, Info, Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { PageContainer } from '@/components/page-container';
import { ReviewPhrasingDisplay } from '@/components/review-phrasing-display';
import { LearningModeExplainer } from '@/components/review/learning-mode-explainer';
import { ReviewActionsDropdown } from '@/components/review/review-actions-dropdown';
import { ReviewEmptyState } from '@/components/review/review-empty-state';
import { ReviewErrorBoundary } from '@/components/review/review-error-boundary';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { LiveRegion } from '@/components/ui/live-region';
import { QuizFlowSkeleton } from '@/components/ui/loading-skeletons';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Textarea } from '@/components/ui/textarea';
import { api } from '@/convex/_generated/api';
import { useConceptActions } from '@/hooks/use-concept-actions';
import { useInstantFeedback } from '@/hooks/use-instant-feedback';
import { useReviewShortcuts } from '@/hooks/use-keyboard-shortcuts';
import { useQuizInteractions } from '@/hooks/use-quiz-interactions';
import { useReviewFlow } from '@/hooks/use-review-flow';
import { useUnifiedEdit } from '@/hooks/use-unified-edit';
import type { QuestionType } from '@/lib/unified-edit-validation';
import { cn } from '@/lib/utils';

/**
 * Unified ReviewFlow component that combines ReviewMode + ReviewSession
 * Eliminates intermediate data transformations and prop drilling
 * Works directly with single questions from the review flow
 */
export function ReviewFlow() {
  // Get review state and handlers from custom hook
  const {
    phase,
    question,
    conceptTitle,
    conceptId,
    phrasingId,
    phrasingIndex,
    totalPhrasings,
    selectionReason,
    interactions,
    isTransitioning,
    conceptFsrs,
    skippedCount: _skippedCount, // For future "X skipped" indicator
    handlers,
  } = useReviewFlow();

  // Instant feedback hook for immediate visual response
  const {
    feedbackState: instantFeedback,
    showFeedback: _showFeedback,
    clearFeedback: _clearFeedback,
  } = useInstantFeedback();

  // Track component mount status to prevent setState on unmounted component
  const isMountedRef = useRef(true);
  useEffect(() => {
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  // Track previous phrasingId to detect same-phrasing re-reviews (FSRS immediate re-review)
  const prevPhrasingRef = useRef<string | null>(null);

  // Local UI state for answer selection and feedback
  const [selectedAnswer, setSelectedAnswer] = useState<string>('');

  // Combined feedback state to batch updates
  const [feedbackState, setFeedbackState] = useState<{
    showFeedback: boolean;
    nextReviewInfo: {
      nextReview: Date | null;
      scheduledDays: number;
    } | null;
  }>({
    showFeedback: false,
    nextReviewInfo: null,
  });

  // Track whether user has submitted an answer for this question
  // Separate from showFeedback which can be forced true by edit mode
  // Prevents users from skipping questions by editing without answering
  const [hasAnsweredCurrentQuestion, setHasAnsweredCurrentQuestion] = useState(false);

  const { trackAnswer } = useQuizInteractions();
  const [sessionId] = useState(() => Math.random().toString(36).substring(7));
  const [questionStartTime, setQuestionStartTime] = useState(() => Date.now());
  const selectionReasonDescriptions: Record<string, string> = {
    canonical: 'Your preferred phrasing',
    'least-seen': 'Least practiced',
    random: 'Random rotation',
    none: 'Default selection',
  };
  const hasPhrasingPosition =
    typeof phrasingIndex === 'number' &&
    typeof totalPhrasings === 'number' &&
    phrasingIndex > 0 &&
    totalPhrasings > 0;
  const phrasingPositionLabel = hasPhrasingPosition
    ? `Phrasing ${phrasingIndex} of ${totalPhrasings}`
    : null;
  const selectionReasonLabel = selectionReason
    ? (selectionReasonDescriptions[selectionReason] ?? 'Active phrasing')
    : null;

  // Query for current due count - reactive via Convex WebSockets
  const dueCountData = useQuery(api.concepts.getConceptsDueCount);

  // Cache the last known due count to prevent flicker during refetch
  const [cachedData, setCachedData] = useState({ conceptsDue: 0 });
  useEffect(() => {
    if (dueCountData !== undefined) {
      setCachedData(dueCountData);
    }
  }, [dueCountData]);

  // Edit/Delete functionality
  // Legacy modal-based edit/delete removed; unified inline edit handles edits.

  // ============================================================================
  // Unified Edit Integration
  // ============================================================================
  // Replaces previous dual-edit pattern (separate conceptEdit/phrasingEdit hooks)
  // with single unified interface. Benefits:
  // - Simpler UX: One "Edit" button instead of two
  // - Smart dirty detection: Only saves changed domains (concept vs phrasing)
  // - Parallel mutations: 50% latency reduction when both domains dirty
  // - Graceful partial failures: If concept saves but phrasing fails, retry only phrasing
  const conceptActions = useConceptActions({ conceptId: conceptId ?? '' });

  // Optimistic update state: holds mutation result until Convex reactivity catches up
  const [optimisticConcept, setOptimisticConcept] = useState<{
    title: string;
  } | null>(null);

  const [optimisticPhrasing, setOptimisticPhrasing] = useState<{
    question: string;
    correctAnswer: string;
    explanation: string;
    options: string[];
  } | null>(null);

  const questionType: QuestionType =
    question?.type === 'true-false' ? 'true-false' : 'multiple-choice';

  const unifiedEdit = useUnifiedEdit(
    {
      conceptTitle: conceptTitle ?? '',
      question: question?.question ?? '',
      correctAnswer: question?.correctAnswer ?? '',
      explanation: question?.explanation ?? '',
      options: question?.options ?? [],
    },
    async (data) => {
      if (!conceptId) return;
      await conceptActions.editConcept(data);

      // Set optimistic state for immediate UI feedback
      // This bridges the ~50-200ms gap before Convex's reactive query updates
      const optimisticData = { title: data.title };
      setOptimisticConcept(optimisticData);
      return optimisticData;
    },
    async (data) => {
      if (!phrasingId) return;
      const updated = await conceptActions.editPhrasing({
        phrasingId,
        question: data.question,
        correctAnswer: data.correctAnswer,
        explanation: data.explanation,
        options: data.options,
      });

      // Set optimistic state for immediate UI feedback
      // This bridges the ~50-200ms gap before Convex's reactive query updates
      if (updated) {
        const optimisticData = {
          question: updated.question,
          correctAnswer: updated.correctAnswer ?? '',
          explanation: updated.explanation ?? '',
          options: updated.options ?? [],
        };
        setOptimisticPhrasing(optimisticData);
        return optimisticData;
      }
    },
    questionType
  );

  // Clear optimistic concept state when Convex reactive query catches up
  useEffect(() => {
    if (optimisticConcept && conceptTitle) {
      const conceptMatches = conceptTitle === optimisticConcept.title;

      if (conceptMatches) {
        setOptimisticConcept(null); // Convex has caught up, clear optimistic overlay
      }
    }
  }, [optimisticConcept, conceptTitle]);

  // Clear optimistic phrasing state when Convex reactive query catches up
  // This auto-heals the UI once the authoritative data source updates
  useEffect(() => {
    if (optimisticPhrasing && question) {
      const questionsMatch =
        question.question === optimisticPhrasing.question &&
        question.correctAnswer === optimisticPhrasing.correctAnswer &&
        question.explanation === optimisticPhrasing.explanation &&
        JSON.stringify(question.options) === JSON.stringify(optimisticPhrasing.options);

      if (questionsMatch) {
        setOptimisticPhrasing(null); // Convex has caught up, clear optimistic overlay
      }
    }
  }, [optimisticPhrasing, question]);

  // ============================================================================
  // Display Properties (3-tier logic: editing → optimistic → real)
  // ============================================================================
  // Provides seamless UI transitions without flicker:
  // 1. During edit: Show localData from useUnifiedEdit (user's current input)
  // 2. After save: Show optimistic data immediately (~50-200ms before Convex catches up)
  // 3. Otherwise: Show real Convex data (authoritative source)

  const displayConceptTitle = useMemo(() => {
    if (unifiedEdit.isEditing) {
      return unifiedEdit.localData.conceptTitle;
    }
    if (optimisticConcept) {
      return optimisticConcept.title;
    }
    return conceptTitle ?? '';
  }, [unifiedEdit.isEditing, unifiedEdit.localData.conceptTitle, optimisticConcept, conceptTitle]);

  const displayQuestion = useMemo(() => {
    if (!question) return question;

    // If editing, use localData for all fields
    if (unifiedEdit.isEditing) {
      return {
        ...question,
        question: unifiedEdit.localData.question,
        correctAnswer: unifiedEdit.localData.correctAnswer,
        explanation: unifiedEdit.localData.explanation,
        options: unifiedEdit.localData.options,
      };
    }

    // After save but before Convex update, use optimistic data
    if (optimisticPhrasing) {
      return {
        ...question,
        question: optimisticPhrasing.question,
        correctAnswer: optimisticPhrasing.correctAnswer,
        explanation: optimisticPhrasing.explanation,
        options: optimisticPhrasing.options,
      };
    }

    // Default: use real Convex data
    return question;
  }, [question, optimisticPhrasing, unifiedEdit.isEditing, unifiedEdit.localData]);

  // Reset state when question changes OR when transition completes
  // This handles both normal question changes AND FSRS immediate re-review (same phrasing)
  useEffect(() => {
    if (phrasingId && !isTransitioning) {
      // Track if this is the same phrasing returning (FSRS re-review)
      const isSamePhrasing = prevPhrasingRef.current === phrasingId;
      prevPhrasingRef.current = phrasingId;

      // Always reset answer state after transition completes (even for same phrasing)
      setSelectedAnswer('');
      setHasAnsweredCurrentQuestion(false);
      _clearFeedback();
      setFeedbackState({
        showFeedback: false,
        nextReviewInfo: null,
      });
      setQuestionStartTime(Date.now());

      // Clear optimistic state only on phrasing change (not same-phrasing re-review)
      if (!isSamePhrasing) {
        setOptimisticConcept(null);
        setOptimisticPhrasing(null);
      }
    }
  }, [phrasingId, isTransitioning, _clearFeedback]);

  const handleAnswerSelect = useCallback(
    (answer: string) => {
      if (feedbackState.showFeedback) return;
      setSelectedAnswer(answer);
    },
    [feedbackState.showFeedback]
  );

  const handleSubmit = useCallback(() => {
    if (!selectedAnswer || !question || !conceptId || !phrasingId) return;

    const isCorrect = selectedAnswer === question.correctAnswer;

    // Mark that user has answered this question (before any async operations)
    // This prevents users from skipping by editing without answering
    setHasAnsweredCurrentQuestion(true);

    // 1. INSTANT: Show visual feedback (synchronous, <16ms)
    _showFeedback(isCorrect);

    // 2. INSTANT: Enable feedback section and Next button immediately
    setFeedbackState({
      showFeedback: true,
      nextReviewInfo: null, // Will be filled progressively when backend responds
    });

    // 3. BACKGROUND: Track with FSRS (fire-and-forget for Phase 1 MVP)
    // Calculate time spent from question load to submission (milliseconds)
    const timeSpent = Date.now() - questionStartTime;
    trackAnswer(conceptId, phrasingId, selectedAnswer, isCorrect, timeSpent, sessionId)
      .then((reviewInfo) => {
        // 4. PROGRESSIVE: Show scheduling details when backend responds
        // Only update state if component is still mounted (race condition protection)
        if (isMountedRef.current) {
          setFeedbackState({
            showFeedback: true,
            nextReviewInfo: reviewInfo
              ? {
                  nextReview: reviewInfo.nextReview,
                  scheduledDays: reviewInfo.scheduledDays,
                }
              : null,
          });
        }
      })
      .catch((error) => {
        // Phase 1: Log error and notify user, Phase 2 will add retry logic
        console.error('Failed to track answer:', error);
        toast.error('Failed to save your answer', {
          description: "Your progress wasn't saved. Please try again.",
        });
      });
  }, [
    selectedAnswer,
    question,
    conceptId,
    phrasingId,
    questionStartTime,
    trackAnswer,
    sessionId,
    _showFeedback,
  ]);

  const handleNext = useCallback(() => {
    // Tell the review flow we're done with this question
    handlers.onReviewComplete();

    // State will reset when new question arrives via useEffect
  }, [handlers]);

  // Edit handler (legacy modal-based edit, currently unused - inline editing is used instead)
  // Archive phrasing handler with undo
  const handleArchivePhrasing = useCallback(async () => {
    if (!phrasingId) return;

    // Prevent archiving the last phrasing - suggest archiving concept instead
    if (totalPhrasings === 1) {
      toast.error('This is the last phrasing. Archive the entire concept instead?');
      return;
    }

    await conceptActions.archivePhrasingWithUndo(phrasingId);
    // Move to next question after archiving
    handlers.onReviewComplete();
  }, [phrasingId, totalPhrasings, conceptActions, handlers]);

  // Archive concept handler with undo
  const handleArchiveConcept = useCallback(async () => {
    if (!conceptId) return;

    await conceptActions.archiveConceptWithUndo();
    // Move to next question after archiving
    handlers.onReviewComplete();
  }, [conceptId, conceptActions, handlers]);

  // Skip handler: move current concept to end of session queue
  const [skipAnnouncement, setSkipAnnouncement] = useState('');
  const handleSkip = useCallback(() => {
    // Clear optimistic state since we're moving to a different concept
    setOptimisticConcept(null);
    setOptimisticPhrasing(null);
    handlers.onSkipConcept();
    toast("Skipped. You'll see this again shortly.", { duration: 2000 });
    // Screen reader announcement
    setSkipAnnouncement('Concept skipped. Will reappear shortly.');
    setTimeout(() => setSkipAnnouncement(''), 3000);
  }, [handlers]);

  // Shortcut-friendly archive that chooses phrasing when possible, else concept
  const handleArchiveViaShortcut = useCallback(() => {
    if (!feedbackState.showFeedback) return;
    if (phrasingId && totalPhrasings !== null && totalPhrasings > 1) {
      void handleArchivePhrasing();
      return;
    }
    if (conceptId) {
      void handleArchiveConcept();
    }
  }, [
    feedbackState.showFeedback,
    phrasingId,
    totalPhrasings,
    conceptId,
    handleArchivePhrasing,
    handleArchiveConcept,
  ]);

  // Handler for starting unified edit mode (E key)
  const handleStartInlineEdit = useCallback(() => {
    // Allow editing anytime when not already editing
    if (!unifiedEdit.isEditing) {
      // Force feedback section visible to show edit form
      setFeedbackState((prev) => ({ ...prev, showFeedback: true }));
      // Start unified edit mode
      unifiedEdit.startEdit();
    }
  }, [unifiedEdit, setFeedbackState]);

  // Listen for Escape key to save and exit unified edit mode
  useEffect(() => {
    const handleEscape = () => {
      if (unifiedEdit.isEditing) {
        unifiedEdit.save().catch(() => {
          // Error already handled by save (toast shown, field-level errors displayed)
        });
      }
    };

    window.addEventListener('escape-pressed', handleEscape);
    return () => window.removeEventListener('escape-pressed', handleEscape);
  }, [unifiedEdit]);

  // Reset to answer mode when exiting edit mode without having answered
  // Learning science: Editing is content improvement, not retrieval practice.
  // User must still answer the question to log FSRS interaction.
  useEffect(() => {
    if (!unifiedEdit.isEditing && !hasAnsweredCurrentQuestion && feedbackState.showFeedback) {
      // User edited without answering - return to answer mode
      setFeedbackState({ showFeedback: false, nextReviewInfo: null });
    }
  }, [unifiedEdit.isEditing, hasAnsweredCurrentQuestion, feedbackState.showFeedback]);

  // Wire up keyboard shortcuts
  useReviewShortcuts({
    onSelectAnswer: !feedbackState.showFeedback
      ? (index: number) => {
          if (question && question.options[index]) {
            handleAnswerSelect(question.options[index]);
          }
        }
      : undefined,
    onSubmit: !feedbackState.showFeedback && selectedAnswer ? handleSubmit : undefined,
    onNext: feedbackState.showFeedback && !isTransitioning ? handleNext : undefined,
    onEdit: unifiedEdit.isEditing ? undefined : handleStartInlineEdit,
    onArchive: handleArchiveViaShortcut,
    showingFeedback: feedbackState.showFeedback,
    canSubmit: !!selectedAnswer,
  });

  // Render based on phase
  if (phase === 'loading') {
    return <QuizFlowSkeleton />;
  }

  if (phase === 'empty') {
    return <ReviewEmptyState />;
  }

  if (phase === 'reviewing' && question) {
    return (
      <PageContainer className="py-6">
        {/* ARIA live region for screen reader feedback announcements */}
        <LiveRegion politeness="polite" atomic={true}>
          {skipAnnouncement ||
            (instantFeedback.visible ? (instantFeedback.isCorrect ? 'Correct' : 'Incorrect') : '')}
        </LiveRegion>

        <div className="max-w-[760px]">
          <article className="space-y-6">
            {/* Header row: Due count + Actions dropdown - always visible, maintains value during refetch */}
            <div className="mb-6 flex items-center justify-between">
              <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-card border border-border/50 shadow-sm">
                <Clock className="h-4 w-4 text-muted-foreground" />
                <span className="text-sm font-medium tabular-nums">
                  <span className="text-foreground">{cachedData.conceptsDue}</span>
                  <span className="text-muted-foreground ml-1">concepts due</span>
                  {/* Orphaned questions removed with legacy questions system */}
                </span>
              </div>
              {conceptId && (
                <ReviewActionsDropdown
                  totalPhrasings={totalPhrasings}
                  onEdit={() => {
                    setFeedbackState((prev) => ({ ...prev, showFeedback: true }));
                    unifiedEdit.startEdit();
                  }}
                  onSkip={handleSkip}
                  canSkip={!isTransitioning}
                  onArchiveConcept={handleArchiveConcept}
                  onArchivePhrasing={handleArchivePhrasing}
                />
              )}
            </div>

            {/* FSRS State Badge - Learning Mode Indicator */}
            {conceptFsrs?.state === 'learning' && (
              <Badge
                variant="outline"
                className="border-blue-500 text-blue-700 bg-blue-50 dark:border-blue-400 dark:text-blue-300 dark:bg-blue-950"
              >
                <Brain className="h-3 w-3 mr-1" />
                Learning Mode • Step {(conceptFsrs.reps ?? 0) + 1} of 4
              </Badge>
            )}

            {/* First-Time Learning Mode Explainer */}
            {conceptFsrs?.state === 'learning' && conceptFsrs.reps === 0 && (
              <LearningModeExplainer />
            )}

            {/* Question display with inline editing support */}
            <ReviewErrorBoundary
              fallbackMessage="Unable to display this question. Try refreshing or moving to the next question."
              onReset={() => {
                // Reset local state and try to move to next question
                setSelectedAnswer('');
                setFeedbackState({ showFeedback: false, nextReviewInfo: null });
                handlers.onReviewComplete();
              }}
            >
              <ReviewPhrasingDisplay
                question={displayQuestion!}
                phrasingId={phrasingId ?? undefined}
                selectedAnswer={selectedAnswer}
                showFeedback={feedbackState.showFeedback}
                onAnswerSelect={handleAnswerSelect}
                instantFeedback={instantFeedback}
                isEditing={unifiedEdit.isEditing}
                editState={
                  unifiedEdit.isEditing
                    ? {
                        question: unifiedEdit.localData.question,
                        options: unifiedEdit.localData.options,
                        correctAnswer: unifiedEdit.localData.correctAnswer,
                        onQuestionChange: (value) => unifiedEdit.updateField('question', value),
                        onOptionsChange: (options) => unifiedEdit.updateField('options', options),
                        onCorrectAnswerChange: (answer) =>
                          unifiedEdit.updateField('correctAnswer', answer),
                        errors: unifiedEdit.errors,
                      }
                    : undefined
                }
              />
            </ReviewErrorBoundary>

            {/* Action button - hidden during edit mode */}
            {!unifiedEdit.isEditing && (
              <div className="flex items-center justify-end mt-6 mb-4">
                {/* Use hasAnsweredCurrentQuestion to determine button state
                    This ensures users must answer after editing without submitting */}
                {!hasAnsweredCurrentQuestion ? (
                  <Button onClick={handleSubmit} disabled={!selectedAnswer} size="lg">
                    Submit
                  </Button>
                ) : (
                  <Button
                    onClick={handleNext}
                    disabled={isTransitioning}
                    size="lg"
                    aria-busy={isTransitioning}
                  >
                    {isTransitioning ? (
                      <>
                        Loading
                        <Loader2 className="ml-2 h-4 w-4 animate-spin" aria-hidden="true" />
                      </>
                    ) : (
                      <>
                        Next
                        <ArrowRight className="ml-2 h-4 w-4" />
                      </>
                    )}
                  </Button>
                )}
              </div>
            )}

            {/* Feedback section */}
            {(feedbackState.showFeedback || unifiedEdit.isEditing) &&
              (displayConceptTitle ||
                question.explanation ||
                interactions.length > 0 ||
                feedbackState.nextReviewInfo?.nextReview ||
                unifiedEdit.isEditing) && (
                <div className="space-y-3 p-4 bg-muted/30 rounded-lg border border-border/50 animate-fadeIn">
                  {/* Concept Title - Inline editable */}
                  {(displayConceptTitle || unifiedEdit.isEditing) && (
                    <div className="space-y-1 border-b border-border/30 pb-3">
                      <div className="flex items-center gap-2 text-xs uppercase tracking-wide text-muted-foreground">
                        <span>Concept</span>
                      </div>
                      {unifiedEdit.isEditing ? (
                        <div className="space-y-2">
                          <Input
                            value={unifiedEdit.localData.conceptTitle}
                            onChange={(e) =>
                              unifiedEdit.updateField('conceptTitle', e.target.value)
                            }
                            className={cn(
                              'text-xl font-semibold',
                              unifiedEdit.errors.conceptTitle && 'border-destructive'
                            )}
                            aria-invalid={!!unifiedEdit.errors.conceptTitle}
                            placeholder="Concept title"
                          />
                          {unifiedEdit.errors.conceptTitle && (
                            <p className="text-sm text-destructive">
                              {unifiedEdit.errors.conceptTitle}
                            </p>
                          )}
                        </div>
                      ) : (
                        <h3 className="text-xl font-semibold text-foreground break-words">
                          {displayConceptTitle}
                        </h3>
                      )}
                      {/* Hide phrasing labels when editing */}
                      {!unifiedEdit.isEditing &&
                        (phrasingPositionLabel || selectionReasonLabel) && (
                          <p className="text-sm text-muted-foreground">
                            {phrasingPositionLabel}
                            {phrasingPositionLabel && selectionReasonLabel && ' • '}
                            {selectionReasonLabel}
                          </p>
                        )}
                    </div>
                  )}

                  {/* Explanation - editable when in edit mode */}
                  {(question.explanation || unifiedEdit.isEditing) && (
                    <div className="space-y-2">
                      {unifiedEdit.isEditing ? (
                        <>
                          <Label
                            htmlFor="explanation"
                            className="text-xs uppercase tracking-wide text-muted-foreground"
                          >
                            Explanation (optional)
                          </Label>
                          <Textarea
                            id="explanation"
                            value={unifiedEdit.localData.explanation}
                            onChange={(e) => unifiedEdit.updateField('explanation', e.target.value)}
                            placeholder="Explanation shown after answering (optional)"
                            className="min-h-[80px]"
                          />
                        </>
                      ) : (
                        <p className="text-sm text-foreground/90">{question.explanation}</p>
                      )}
                    </div>
                  )}

                  {/* Divider between explanation and other content */}
                  {question.explanation &&
                    (interactions.length > 0 || feedbackState.nextReviewInfo?.nextReview) && (
                      <hr className="border-border/30" />
                    )}

                  {/* Question history UI removed with legacy questions system; interactions preserved for future analytics/visualization. */}
                  {/* Next Review - inline and subtle (hidden when editing) */}
                  {!unifiedEdit.isEditing &&
                    feedbackState.nextReviewInfo &&
                    feedbackState.nextReviewInfo.nextReview && (
                      <div className="flex items-center gap-2 text-sm text-muted-foreground pt-1">
                        <Calendar className="h-3.5 w-3.5" />
                        <span>
                          Next review:{' '}
                          {feedbackState.nextReviewInfo.scheduledDays === 0
                            ? 'Today'
                            : feedbackState.nextReviewInfo.scheduledDays === 1
                              ? 'Tomorrow'
                              : `In ${feedbackState.nextReviewInfo.scheduledDays} days`}
                          {' at '}
                          {new Date(feedbackState.nextReviewInfo.nextReview).toLocaleTimeString(
                            'en-US',
                            {
                              hour: 'numeric',
                              minute: '2-digit',
                            }
                          )}
                        </span>
                      </div>
                    )}
                </div>
              )}

            {/* Save/Cancel buttons - outside card for page-level context */}
            {unifiedEdit.isEditing && (
              <div className="flex items-center gap-2 pt-4">
                <Button
                  onClick={() => unifiedEdit.save()}
                  disabled={unifiedEdit.isSaving || !unifiedEdit.isDirty}
                >
                  {unifiedEdit.isSaving ? 'Saving...' : 'Save Changes'}
                </Button>
                <Button
                  variant="ghost"
                  onClick={() => unifiedEdit.cancel()}
                  disabled={unifiedEdit.isSaving}
                >
                  Cancel
                </Button>
                {/* FSRS Preservation Info - Popover for click/tap support */}
                <Popover>
                  <PopoverTrigger asChild>
                    <button
                      type="button"
                      className="ml-auto text-muted-foreground hover:text-foreground transition-colors"
                      aria-label="Information about FSRS preservation"
                    >
                      <Info className="h-5 w-5" />
                    </button>
                  </PopoverTrigger>
                  <PopoverContent className="max-w-xs">
                    <p className="text-sm font-medium">Edits preserve your learning progress.</p>
                    <p className="text-xs text-muted-foreground mt-1">
                      Your FSRS scheduling state (difficulty, stability, next review date) remains
                      unchanged. For major content changes, consider archiving and creating a new
                      concept instead.
                    </p>
                  </PopoverContent>
                </Popover>
              </div>
            )}
          </article>

          {/* Legacy edit modal removed */}
        </div>
      </PageContainer>
    );
  }

  // Fallback for unexpected states
  return null;
}
