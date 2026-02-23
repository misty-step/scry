'use client';

import { useEffect } from 'react';
import { useReviewShortcuts } from '@/hooks/use-keyboard-shortcuts';
import { useReviewSession } from './session-context';

export function ReviewKeyboardHandler() {
  const {
    feedbackState,
    selectedAnswer,
    isTransitioning,
    unifiedEdit,
    handleSubmit,
    handleNext,
    handleStartInlineEdit,
    handleArchiveViaShortcut,
    handleAnswerSelect,
    question,
  } = useReviewSession();

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

  useEffect(() => {
    const handleEscape = () => {
      if (!unifiedEdit.isEditing) return;
      unifiedEdit.save().catch(() => {
        // save() already shows inline validation.
      });
    };

    window.addEventListener('escape-pressed', handleEscape);
    return () => window.removeEventListener('escape-pressed', handleEscape);
  }, [unifiedEdit]);

  return null;
}
