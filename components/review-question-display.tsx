'use client';

import React from 'react';
import { CheckCircle, XCircle } from 'lucide-react';
import type { Id } from '@/convex/_generated/dataModel';
import { useShuffledOptions } from '@/hooks/use-shuffled-options';
import { cn } from '@/lib/utils';
import type { SimpleQuestion } from '@/types/questions';

interface ReviewQuestionDisplayProps {
  question: SimpleQuestion;
  questionId?: Id<'questions'> | Id<'phrasings'> | null;
  selectedAnswer: string;
  showFeedback: boolean;
  onAnswerSelect: (answer: string) => void;
  instantFeedback?: {
    isCorrect: boolean;
    visible: boolean;
  };
}

/**
 * Pure component for rendering a quiz question with answer options
 * Memoized to prevent unnecessary re-renders when parent state changes
 * Only re-renders when question ID, selected answer, or feedback state changes
 *
 * Answer options are shuffled deterministically based on questionId + userId
 * to prevent the correct answer from always appearing in the same position
 */
function ReviewQuestionDisplayComponent({
  question,
  questionId,
  selectedAnswer,
  showFeedback,
  onAnswerSelect,
  instantFeedback,
}: ReviewQuestionDisplayProps) {
  // Shuffle options deterministically based on questionId + userId
  const shuffledOptions = useShuffledOptions(question.options, questionId);

  // Use instant feedback if available, otherwise fall back to delayed feedback
  const displayFeedback = instantFeedback?.visible || showFeedback;

  return (
    <>
      <h2 className="text-xl font-semibold">{question.question}</h2>

      <div className="space-y-3">
        {question.type === 'true-false' ? (
          // True/False specific layout
          <div className="grid grid-cols-2 gap-4">
            {shuffledOptions.map((option, index) => (
              <button
                key={index}
                data-testid={`answer-option-${index}`}
                onClick={() => onAnswerSelect(option)}
                className={cn(
                  // Base styles
                  'p-6 rounded-lg border-2 transition-all font-medium',
                  'outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
                  // Default state
                  'border-input hover:bg-accent/50 hover:border-accent',
                  // Selected state (before feedback)
                  selectedAnswer === option &&
                    !displayFeedback &&
                    'border-info-border bg-info-background text-info',
                  // Feedback state - correct answer
                  displayFeedback &&
                    option === question.correctAnswer &&
                    'border-success-border bg-success-background text-success',
                  // Feedback state - wrong answer selected
                  displayFeedback &&
                    selectedAnswer === option &&
                    option !== question.correctAnswer &&
                    'border-error-border bg-error-background text-error'
                )}
                disabled={displayFeedback}
              >
                <div className="flex flex-col items-center justify-center space-y-2">
                  <span className="text-lg">{option}</span>
                  {displayFeedback && option === question.correctAnswer && (
                    <CheckCircle className="h-6 w-6 text-success animate-scaleIn" />
                  )}
                  {displayFeedback &&
                    selectedAnswer === option &&
                    option !== question.correctAnswer && (
                      <XCircle className="h-6 w-6 text-error animate-scaleIn" />
                    )}
                </div>
              </button>
            ))}
          </div>
        ) : (
          // Multiple choice layout
          shuffledOptions.map((option, index) => (
            <button
              key={index}
              data-testid={`answer-option-${index}`}
              onClick={() => onAnswerSelect(option)}
              className={cn(
                // Base styles
                'w-full text-left p-4 rounded-lg border transition-colors',
                'outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
                // Default state
                'border-input hover:bg-accent/50 hover:border-accent',
                // Selected state (before feedback)
                selectedAnswer === option &&
                  !displayFeedback &&
                  'border-info-border bg-info-background',
                // Feedback state - correct answer
                displayFeedback &&
                  option === question.correctAnswer &&
                  'border-success-border bg-success-background',
                // Feedback state - wrong answer selected
                displayFeedback &&
                  selectedAnswer === option &&
                  option !== question.correctAnswer &&
                  'border-error-border bg-error-background'
              )}
              disabled={displayFeedback}
            >
              <div className="flex items-center justify-between">
                <span>{option}</span>
                {displayFeedback && option === question.correctAnswer && (
                  <CheckCircle className="h-5 w-5 text-success animate-scaleIn" />
                )}
                {displayFeedback &&
                  selectedAnswer === option &&
                  option !== question.correctAnswer && (
                    <XCircle className="h-5 w-5 text-error animate-scaleIn" />
                  )}
              </div>
            </button>
          ))
        )}
      </div>
    </>
  );
}

// Custom comparison function for React.memo
// Only re-render if props that affect display change
// Includes instantFeedback to prevent re-renders during instant feedback transitions
// while still maintaining responsiveness to backend-driven showFeedback updates
function areEqual(
  prevProps: ReviewQuestionDisplayProps,
  nextProps: ReviewQuestionDisplayProps
): boolean {
  return (
    prevProps.questionId === nextProps.questionId &&
    prevProps.selectedAnswer === nextProps.selectedAnswer &&
    prevProps.showFeedback === nextProps.showFeedback &&
    prevProps.instantFeedback?.visible === nextProps.instantFeedback?.visible &&
    prevProps.instantFeedback?.isCorrect === nextProps.instantFeedback?.isCorrect
  );
}

// Export memoized component with custom comparison
export const ReviewQuestionDisplay = React.memo(ReviewQuestionDisplayComponent, areEqual);
