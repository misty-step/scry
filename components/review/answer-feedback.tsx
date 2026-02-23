'use client';

import { Calendar, ThumbsDown, ThumbsUp } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import { useReviewSession } from './session-context';

export function AnswerFeedback() {
  const {
    displayConceptTitle,
    displayQuestion,
    feedbackState,
    unifiedEdit,
    phrasingPositionLabel,
    selectionReasonLabel,
    interactions,
    currentInteractionId,
    userFeedback,
    handleUserFeedback,
  } = useReviewSession();

  const shouldShow = feedbackState.showFeedback || unifiedEdit.isEditing;
  const hasContent =
    !!displayConceptTitle ||
    !!displayQuestion?.explanation ||
    interactions.length > 0 ||
    !!feedbackState.nextReviewInfo?.nextReview ||
    unifiedEdit.isEditing;

  if (!shouldShow || !hasContent) {
    return null;
  }

  return (
    <div className="space-y-3 p-4 bg-muted/30 rounded-lg border border-border/50 animate-fadeIn">
      {(displayConceptTitle || unifiedEdit.isEditing) && (
        <div className="space-y-1 border-b border-border/30 pb-3">
          <div className="flex items-center gap-2 text-xs uppercase tracking-wide text-muted-foreground">
            <span>Concept</span>
          </div>
          {unifiedEdit.isEditing ? (
            <div className="space-y-2">
              <Input
                value={unifiedEdit.localData.conceptTitle}
                onChange={(e) => unifiedEdit.updateField('conceptTitle', e.target.value)}
                className={cn(
                  'text-xl font-semibold',
                  unifiedEdit.errors.conceptTitle && 'border-destructive'
                )}
                aria-invalid={!!unifiedEdit.errors.conceptTitle}
                placeholder="Concept title"
              />
              {unifiedEdit.errors.conceptTitle && (
                <p className="text-sm text-destructive">{unifiedEdit.errors.conceptTitle}</p>
              )}
            </div>
          ) : (
            <h3 className="text-xl font-semibold text-foreground break-words">
              {displayConceptTitle}
            </h3>
          )}
          {!unifiedEdit.isEditing && (phrasingPositionLabel || selectionReasonLabel) && (
            <p className="text-sm text-muted-foreground">
              {phrasingPositionLabel}
              {phrasingPositionLabel && selectionReasonLabel && ' • '}
              {selectionReasonLabel}
            </p>
          )}
        </div>
      )}

      {(displayQuestion?.explanation || unifiedEdit.isEditing) && (
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
            <p className="text-sm text-foreground/90">{displayQuestion?.explanation}</p>
          )}
        </div>
      )}

      {displayQuestion?.explanation &&
        (interactions.length > 0 || feedbackState.nextReviewInfo?.nextReview) && (
          <hr className="border-border/30" />
        )}

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
              {new Date(feedbackState.nextReviewInfo.nextReview).toLocaleTimeString('en-US', {
                hour: 'numeric',
                minute: '2-digit',
              })}
            </span>
          </div>
        )}

      {!unifiedEdit.isEditing && currentInteractionId && (
        <div className="flex items-center justify-between pt-2 border-t border-border/30 mt-3">
          <span className="text-sm text-muted-foreground">Was this question helpful?</span>
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => handleUserFeedback('helpful')}
              disabled={userFeedback !== null}
              className={cn(
                'p-2 rounded-md transition-colors',
                userFeedback === 'helpful'
                  ? 'text-green-600 bg-green-100 dark:text-green-400 dark:bg-green-950'
                  : userFeedback === null
                    ? 'text-muted-foreground hover:text-foreground hover:bg-muted'
                    : 'text-muted-foreground/50'
              )}
              aria-label="Helpful"
              aria-pressed={userFeedback === 'helpful'}
            >
              <ThumbsUp className="h-4 w-4" />
            </button>
            <button
              type="button"
              onClick={() => handleUserFeedback('unhelpful')}
              disabled={userFeedback !== null}
              className={cn(
                'p-2 rounded-md transition-colors',
                userFeedback === 'unhelpful'
                  ? 'text-red-600 bg-red-100 dark:text-red-400 dark:bg-red-950'
                  : userFeedback === null
                    ? 'text-muted-foreground hover:text-foreground hover:bg-muted'
                    : 'text-muted-foreground/50'
              )}
              aria-label="Not helpful"
              aria-pressed={userFeedback === 'unhelpful'}
            >
              <ThumbsDown className="h-4 w-4" />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
