'use client';

import { ArrowRight, Loader2 } from 'lucide-react';
import { PageContainer } from '@/components/page-container';
import { AnswerFeedback } from '@/components/review/answer-feedback';
import { InlineEditor } from '@/components/review/inline-editor';
import { ReviewKeyboardHandler } from '@/components/review/keyboard-handler';
import { QuestionDisplay } from '@/components/review/question-display';
import { ReviewActionsDropdown } from '@/components/review/review-actions-dropdown';
import { ReviewDueCount } from '@/components/review/review-due-count';
import { ReviewEmptyState } from '@/components/review/review-empty-state';
import { ReviewSessionProvider, useReviewSession } from '@/components/review/session-context';
import { Button } from '@/components/ui/button';
import { LiveRegion } from '@/components/ui/live-region';
import { QuizFlowSkeleton } from '@/components/ui/loading-skeletons';

function ReviewFlowContent() {
  const {
    phase,
    question,
    skipAnnouncement,
    instantFeedback,
    cachedDueCount,
    conceptId,
    totalPhrasings,
    handleStartInlineEdit,
    handleSkip,
    isTransitioning,
    handleArchiveConcept,
    handleArchivePhrasing,
    unifiedEdit,
    hasAnsweredCurrentQuestion,
    selectedAnswer,
    handleSubmit,
    handleNext,
  } = useReviewSession();

  if (phase === 'loading') {
    return <QuizFlowSkeleton />;
  }

  if (phase === 'empty') {
    return <ReviewEmptyState />;
  }

  if (phase !== 'reviewing' || !question) {
    return null;
  }

  return (
    <PageContainer className="py-6">
      <LiveRegion politeness="polite" atomic={true}>
        {skipAnnouncement ||
          (instantFeedback.visible ? (instantFeedback.isCorrect ? 'Correct' : 'Incorrect') : '')}
      </LiveRegion>

      <div className="max-w-[760px]">
        <article className="space-y-6">
          <ReviewKeyboardHandler />
          <div className="mb-6 flex items-center justify-between">
            <ReviewDueCount count={cachedDueCount} />
            {conceptId && (
              <ReviewActionsDropdown
                totalPhrasings={totalPhrasings}
                onEdit={handleStartInlineEdit}
                onSkip={handleSkip}
                canSkip={!isTransitioning}
                onArchiveConcept={handleArchiveConcept}
                onArchivePhrasing={handleArchivePhrasing}
              />
            )}
          </div>

          <QuestionDisplay />

          {!unifiedEdit.isEditing && (
            <div className="flex items-center justify-end mt-6 mb-4">
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

          <AnswerFeedback />
          <InlineEditor />
        </article>
      </div>
    </PageContainer>
  );
}

export function ReviewFlow() {
  return (
    <ReviewSessionProvider>
      <ReviewFlowContent />
    </ReviewSessionProvider>
  );
}
