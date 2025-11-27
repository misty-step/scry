'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQuery } from 'convex/react';
import { ArrowRight, Brain, Calendar, Clock, Info, Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { EditQuestionModal } from '@/components/edit-question-modal';
import { PageContainer } from '@/components/page-container';
import { QuestionHistory } from '@/components/question-history';
import { ReviewQuestionDisplay } from '@/components/review-question-display';
import { LearningModeExplainer } from '@/components/review/learning-mode-explainer';
import { ReviewActionsDropdown } from '@/components/review/review-actions-dropdown';
import { ReviewEmptyState } from '@/components/review/review-empty-state';
import { ReviewErrorBoundary } from '@/components/review/review-error-boundary';
import { UnifiedEditForm } from '@/components/review/unified-edit-form';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { LiveRegion } from '@/components/ui/live-region';
import { QuizFlowSkeleton } from '@/components/ui/loading-skeletons';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useCurrentQuestion } from '@/contexts/current-question-context';
import { api } from '@/convex/_generated/api';
import type { Doc } from '@/convex/_generated/dataModel';
import { useConceptActions } from '@/hooks/use-concept-actions';
import { useConfirmation } from '@/hooks/use-confirmation';
import { useInstantFeedback } from '@/hooks/use-instant-feedback';
import { useReviewShortcuts } from '@/hooks/use-keyboard-shortcuts';
import { useQuestionMutations } from '@/hooks/use-question-mutations';
import { useQuizInteractions } from '@/hooks/use-quiz-interactions';
import { useReviewFlow } from '@/hooks/use-review-flow';
import { useUnifiedEdit } from '@/hooks/use-unified-edit';

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
    legacyQuestionId,
    selectionReason,
    interactions,
    isTransitioning,
    conceptFsrs,
    handlers,
  } = useReviewFlow();

  // Use context for current question
  const { setCurrentQuestion } = useCurrentQuestion();

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

  const { trackAnswer } = useQuizInteractions();
  const [sessionId] = useState(() => Math.random().toString(36).substring(7));
  const [questionStartTime, setQuestionStartTime] = useState(Date.now());
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
  const [cachedData, setCachedData] = useState({ conceptsDue: 0, orphanedQuestions: 0 });
  useEffect(() => {
    if (dueCountData !== undefined) {
      setCachedData(dueCountData);
    }
  }, [dueCountData]);

  // Edit/Delete functionality
  const [editModalOpen, setEditModalOpen] = useState(false);
  const { optimisticEdit, optimisticDelete } = useQuestionMutations();
  const confirm = useConfirmation();

  // Unified inline editing for both concept and phrasing
  const conceptActions = useConceptActions({ conceptId: conceptId ?? '' });

  // Optimistic update state: holds mutation result until Convex reactivity catches up
  const [optimisticPhrasing, setOptimisticPhrasing] = useState<{
    question: string;
    correctAnswer: string;
    explanation: string;
    options: string[];
  } | null>(null);

  const unifiedEdit = useUnifiedEdit(
    {
      conceptTitle: conceptTitle ?? '',
      conceptDescription: '', // Concepts don't have descriptions in current data model
      question: question?.question ?? '',
      correctAnswer: question?.correctAnswer ?? '',
      explanation: question?.explanation ?? '',
      options: question?.options ?? [],
    },
    async (data) => {
      if (!conceptId) return;
      await conceptActions.editConcept(data);
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
    question?.type || 'multiple-choice'
  );

  // Clear optimistic state when Convex reactive query catches up
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

  // Merge optimistic data with real question data for display
  // Uses optimistic overlay if available, falls back to Convex query result
  const displayQuestion = useMemo(() => {
    if (!question) return question;
    if (!optimisticPhrasing) return question;

    return {
      ...question,
      question: optimisticPhrasing.question,
      correctAnswer: optimisticPhrasing.correctAnswer,
      explanation: optimisticPhrasing.explanation,
      options: optimisticPhrasing.options,
    };
  }, [question, optimisticPhrasing]);

  // Update context when current question changes
  useEffect(() => {
    if (question && legacyQuestionId) {
      setCurrentQuestion({
        ...question,
        _id: legacyQuestionId,
        type: question.type || 'multiple-choice',
      } as Doc<'questions'>);
    } else {
      setCurrentQuestion(undefined);
    }
  }, [question, legacyQuestionId, setCurrentQuestion]);

  // Reset state when question changes OR when transition completes
  // This handles both normal question changes AND FSRS immediate re-review (same phrasing)
  useEffect(() => {
    if (phrasingId && !isTransitioning) {
      setSelectedAnswer('');
      _clearFeedback();
      setFeedbackState({
        showFeedback: false,
        nextReviewInfo: null,
      });
      setQuestionStartTime(Date.now());
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
  const _handleEdit = useCallback(() => {
    if (!question || !legacyQuestionId) return;
    setEditModalOpen(true);
  }, [question, legacyQuestionId]);

  // Delete handler with confirmation
  const handleDelete = useCallback(async () => {
    if (!question || !legacyQuestionId) return;

    const confirmed = await confirm({
      title: 'Delete this question?',
      description:
        'This will move the question to trash. You can restore it later from the Library.',
      confirmText: 'Move to Trash',
      cancelText: 'Cancel',
      variant: 'destructive',
    });

    if (confirmed) {
      const result = await optimisticDelete({ questionId: legacyQuestionId });
      if (result.success) {
        // Toast already shown by optimisticDelete hook
        // Move to next question after delete
        handlers.onReviewComplete();
      }
    }
  }, [question, legacyQuestionId, optimisticDelete, handlers, confirm]);

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

  // Handle save from edit modal - now supports all fields
  const handleSaveEdit = useCallback(
    async (updates: {
      question: string;
      options: string[];
      correctAnswer: string;
      explanation: string;
    }) => {
      if (!legacyQuestionId) return;

      // Pass all fields including options and correctAnswer
      const result = await optimisticEdit({
        questionId: legacyQuestionId,
        question: updates.question,
        explanation: updates.explanation,
        options: updates.options,
        correctAnswer: updates.correctAnswer,
      });

      if (result.success) {
        setEditModalOpen(false);
        toast.success('Question updated');
      }
    },
    [legacyQuestionId, optimisticEdit]
  );

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

  // Wire up keyboard shortcuts
  const canModifyLegacyQuestion = Boolean(legacyQuestionId);

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
    onDelete: canModifyLegacyQuestion ? handleDelete : undefined,
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
          {instantFeedback.visible ? (instantFeedback.isCorrect ? 'Correct' : 'Incorrect') : ''}
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
                  {cachedData.orphanedQuestions > 0 && (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Info className="h-3 w-3 ml-1 inline text-muted-foreground cursor-help" />
                      </TooltipTrigger>
                      <TooltipContent>
                        Plus {cachedData.orphanedQuestions} orphaned questions (need migration)
                      </TooltipContent>
                    </Tooltip>
                  )}
                </span>
              </div>
              {conceptId && (
                <ReviewActionsDropdown
                  totalPhrasings={totalPhrasings}
                  onEdit={() => {
                    setFeedbackState((prev) => ({ ...prev, showFeedback: true }));
                    unifiedEdit.startEdit();
                  }}
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

            {/* Conditional: Show unified edit form OR question display */}
            {unifiedEdit.isEditing ? (
              <UnifiedEditForm
                questionType={question.type || 'multiple-choice'}
                editState={unifiedEdit}
              />
            ) : (
              <ReviewErrorBoundary
                fallbackMessage="Unable to display this question. Try refreshing or moving to the next question."
                onReset={() => {
                  // Reset local state and try to move to next question
                  setSelectedAnswer('');
                  setFeedbackState({ showFeedback: false, nextReviewInfo: null });
                  handlers.onReviewComplete();
                }}
              >
                <ReviewQuestionDisplay
                  question={displayQuestion!}
                  questionId={phrasingId ?? undefined}
                  selectedAnswer={selectedAnswer}
                  showFeedback={feedbackState.showFeedback}
                  onAnswerSelect={handleAnswerSelect}
                  instantFeedback={instantFeedback}
                />
              </ReviewErrorBoundary>
            )}

            {/* Action button - positioned above feedback for layout stability */}
            <div className="flex items-center justify-end mt-6 mb-4">
              {!feedbackState.showFeedback ? (
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

            {/* Feedback section */}
            {feedbackState.showFeedback &&
              (conceptTitle ||
                question.explanation ||
                interactions.length > 0 ||
                feedbackState.nextReviewInfo?.nextReview) && (
                <div className="space-y-3 p-4 bg-muted/30 rounded-lg border border-border/50 animate-fadeIn">
                  {/* Concept Title */}
                  {conceptTitle && (
                    <div className="space-y-1 border-b border-border/30 pb-3">
                      <div className="flex items-center gap-2 text-xs uppercase tracking-wide text-muted-foreground">
                        <span>Concept</span>
                      </div>
                      <h3 className="text-xl font-semibold text-foreground break-words">
                        {conceptTitle}
                      </h3>
                      {(phrasingPositionLabel || selectionReasonLabel) && (
                        <p className="text-sm text-muted-foreground">
                          {phrasingPositionLabel}
                          {phrasingPositionLabel && selectionReasonLabel && ' • '}
                          {selectionReasonLabel}
                        </p>
                      )}
                    </div>
                  )}

                  {/* Explanation */}
                  {question.explanation && (
                    <div className="space-y-2">
                      <p className="text-sm text-foreground/90">{question.explanation}</p>
                    </div>
                  )}

                  {/* Divider between explanation and other content */}
                  {question.explanation &&
                    (interactions.length > 0 || feedbackState.nextReviewInfo?.nextReview) && (
                      <hr className="border-border/30" />
                    )}

                  {/* Question History */}
                  {interactions.length > 0 && (
                    <QuestionHistory interactions={interactions} loading={false} />
                  )}

                  {/* Next Review - inline and subtle */}
                  {feedbackState.nextReviewInfo && feedbackState.nextReviewInfo.nextReview && (
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
          </article>

          {/* Edit Question Modal */}
          {question && legacyQuestionId && (
            <EditQuestionModal
              open={editModalOpen}
              onOpenChange={setEditModalOpen}
              question={
                {
                  _id: legacyQuestionId,
                  _creationTime: Date.now(),
                  userId: '' as Doc<'questions'>['userId'], // Type assertion for missing field
                  question: question.question,
                  topic: '', // SimpleQuestion doesn't have topic
                  difficulty: 'medium', // Default since not in SimpleQuestion
                  type: question.type || 'multiple-choice',
                  options: question.options,
                  correctAnswer: question.correctAnswer,
                  explanation: question.explanation,
                  generatedAt: Date.now(),
                  attemptCount: 0, // Not available in SimpleQuestion
                  correctCount: 0, // Not available in SimpleQuestion
                } as Doc<'questions'>
              }
              onSave={handleSaveEdit}
            />
          )}
        </div>
      </PageContainer>
    );
  }

  // Fallback for unexpected states
  return null;
}
