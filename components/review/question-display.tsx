'use client';

import { Brain } from 'lucide-react';
import { ReviewPhrasingDisplay } from '@/components/review-phrasing-display';
import { LearningModeExplainer } from '@/components/review/learning-mode-explainer';
import { ReviewErrorBoundary } from '@/components/review/review-error-boundary';
import { Badge } from '@/components/ui/badge';
import { useReviewSession } from './session-context';

export function QuestionDisplay() {
  const {
    conceptFsrs,
    displayQuestion,
    phrasingId,
    selectedAnswer,
    feedbackState,
    handleAnswerSelect,
    instantFeedback,
    unifiedEdit,
    handleQuestionDisplayReset,
  } = useReviewSession();

  if (!displayQuestion) {
    return null;
  }

  return (
    <>
      {conceptFsrs?.state === 'learning' && (
        <Badge
          variant="outline"
          className="border-blue-500 text-blue-700 bg-blue-50 dark:border-blue-400 dark:text-blue-300 dark:bg-blue-950"
        >
          <Brain className="h-3 w-3 mr-1" />
          Learning Mode • Step {(conceptFsrs.reps ?? 0) + 1} of 4
        </Badge>
      )}

      {conceptFsrs?.state === 'learning' && conceptFsrs.reps === 0 && <LearningModeExplainer />}

      <ReviewErrorBoundary
        fallbackMessage="Unable to display this question. Try refreshing or moving to the next question."
        onReset={handleQuestionDisplayReset}
      >
        <ReviewPhrasingDisplay
          question={displayQuestion}
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
    </>
  );
}
