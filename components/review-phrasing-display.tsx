'use client';

import React from 'react';
import { CheckCircle, XCircle } from 'lucide-react';
import { OptionsEditor } from '@/components/review/options-editor';
import { TrueFalseEditor } from '@/components/review/true-false-editor';
import { Textarea } from '@/components/ui/textarea';
import { useShuffledOptions } from '@/hooks/use-shuffled-options';
import { cn } from '@/lib/utils';

type SimpleQuestion = {
  question: string;
  type?: 'multiple-choice' | 'true-false' | 'cloze' | 'short-answer';
  options: string[];
  correctAnswer: string;
  explanation?: string;
};

/**
 * Edit state interface for inline editing mode.
 * Passed from useUnifiedEdit hook to enable editing phrasing fields.
 */
interface EditState {
  question: string;
  options: string[];
  correctAnswer: string;
  onQuestionChange: (value: string) => void;
  onOptionsChange: (options: string[]) => void;
  onCorrectAnswerChange: (answer: string) => void;
  errors: Record<string, string>;
}

interface ReviewPhrasingDisplayProps {
  question: SimpleQuestion;
  selectedAnswer: string;
  showFeedback: boolean;
  onAnswerSelect: (answer: string) => void;
  instantFeedback?: {
    isCorrect: boolean;
    visible: boolean;
  };
  // Optional identifier for analytics and future enhancements
  phrasingId?: string;
  /** When true, renders editable fields instead of display elements */
  isEditing?: boolean;
  /** Edit state from useUnifiedEdit hook - required when isEditing is true */
  editState?: EditState;
}

/**
 * Pure component for rendering a quiz question with answer options
 * Memoized to prevent unnecessary re-renders when parent state changes
 * Only re-renders when question ID, selected answer, or feedback state changes
 *
 * Answer options are shuffled randomly (true shuffle per render)
 */
function ReviewPhrasingDisplayComponent({
  question,
  selectedAnswer,
  showFeedback,
  onAnswerSelect,
  instantFeedback,
  isEditing = false,
  editState,
}: ReviewPhrasingDisplayProps) {
  // Shuffle options (true random per render)
  const shuffledOptions = useShuffledOptions(question.options);

  // Use instant feedback if available, otherwise fall back to delayed feedback
  const displayFeedback = instantFeedback?.visible || showFeedback;

  // ============================================================================
  // Edit Mode Rendering
  // ============================================================================
  if (isEditing && editState) {
    return (
      <div className="space-y-4">
        {/* Question text editor */}
        <div className="space-y-2">
          <Textarea
            value={editState.question}
            onChange={(e) => editState.onQuestionChange(e.target.value)}
            placeholder="Enter question text..."
            className={cn(
              'text-xl font-semibold min-h-[100px]',
              editState.errors.question && 'border-destructive'
            )}
            aria-invalid={!!editState.errors.question}
            aria-describedby={editState.errors.question ? 'question-error' : undefined}
          />
          {editState.errors.question && (
            <p id="question-error" className="text-sm text-destructive">
              {editState.errors.question}
            </p>
          )}
        </div>

        {/* Answer options editor - different UI for MC vs TF */}
        {question.type === 'true-false' ? (
          <TrueFalseEditor
            correctAnswer={editState.correctAnswer}
            onCorrectAnswerChange={editState.onCorrectAnswerChange}
          />
        ) : (
          <OptionsEditor
            options={editState.options}
            correctAnswer={editState.correctAnswer}
            onOptionsChange={editState.onOptionsChange}
            onCorrectAnswerChange={editState.onCorrectAnswerChange}
          />
        )}
        {(editState.errors.options || editState.errors.correctAnswer) && (
          <p className="text-sm text-destructive">
            {editState.errors.options || editState.errors.correctAnswer}
          </p>
        )}
      </div>
    );
  }

  // ============================================================================
  // Display Mode Rendering (original behavior)
  // ============================================================================
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
  prevProps: ReviewPhrasingDisplayProps,
  nextProps: ReviewPhrasingDisplayProps
): boolean {
  // Edit mode changes always trigger re-render
  if (prevProps.isEditing !== nextProps.isEditing) return false;

  // In edit mode, compare edit state values
  if (nextProps.isEditing && nextProps.editState) {
    const prevEdit = prevProps.editState;
    const nextEdit = nextProps.editState;
    if (!prevEdit) return false;

    return (
      prevEdit.question === nextEdit.question &&
      prevEdit.correctAnswer === nextEdit.correctAnswer &&
      JSON.stringify(prevEdit.options) === JSON.stringify(nextEdit.options) &&
      JSON.stringify(prevEdit.errors) === JSON.stringify(nextEdit.errors)
    );
  }

  // Display mode comparison (original behavior)
  return (
    prevProps.selectedAnswer === nextProps.selectedAnswer &&
    prevProps.showFeedback === nextProps.showFeedback &&
    prevProps.instantFeedback?.visible === nextProps.instantFeedback?.visible &&
    prevProps.instantFeedback?.isCorrect === nextProps.instantFeedback?.isCorrect &&
    // Deep comparison of question content for inline edit updates
    JSON.stringify(prevProps.question) === JSON.stringify(nextProps.question)
  );
}

// Export memoized component with custom comparison
export const ReviewPhrasingDisplay = React.memo(ReviewPhrasingDisplayComponent, areEqual);
