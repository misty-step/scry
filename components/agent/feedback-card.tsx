'use client';

import { CheckCircle2, XCircle } from 'lucide-react';

interface FeedbackData {
  isCorrect?: boolean;
  correctAnswer?: string;
  userAnswer?: string;
  explanation?: string;
  conceptTitle?: string;
  nextReview?: number;
  scheduledDays?: number;
  newState?: string;
  totalAttempts?: number;
  totalCorrect?: number;
  lapses?: number;
  reps?: number;
}

/** Question text passed separately since the feedback tool doesn't return it */
interface FeedbackCardProps {
  data: Record<string, unknown>;
  questionText?: string;
}

function formatInterval(days?: number): string {
  if (days == null) return '—';
  if (days < 1) return '<1d';
  if (days === 1) return '1d';
  if (days < 7) return `${Math.round(days)}d`;
  if (days < 30) return `${Math.round(days / 7)}w`;
  return `${Math.round(days / 30)}mo`;
}

function formatNextDate(days?: number): string {
  if (days == null) return '—';
  if (days < 1) return 'Later today';
  if (days === 1) return 'Tomorrow';
  const date = new Date();
  date.setDate(date.getDate() + Math.round(days));
  return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
}

export function FeedbackCard({ data, questionText }: FeedbackCardProps) {
  if (typeof data !== 'object' || data === null) return null;
  const fb = data as FeedbackData;

  return (
    <div className="max-w-3xl">
      {/* Status badge */}
      <div className="mb-4 flex items-center gap-3">
        {fb.isCorrect ? (
          <div className="flex items-center gap-2 bg-success-background px-3 py-1.5">
            <CheckCircle2 className="h-4 w-4 text-success" />
            <span className="text-sm font-medium text-success">Correct</span>
          </div>
        ) : (
          <div className="flex items-center gap-2 bg-error-background px-3 py-1.5">
            <XCircle className="h-4 w-4 text-error" />
            <span className="text-sm font-medium text-error">Incorrect</span>
          </div>
        )}
        {fb.conceptTitle && (
          <span className="text-sm text-muted-foreground">{fb.conceptTitle}</span>
        )}
      </div>

      <div className="overflow-hidden border border-border bg-background shadow-sm">
        {/* Question context */}
        {questionText && (
          <div className="border-b border-border bg-secondary p-4 md:p-8">
            <p className="mb-3 font-mono text-xs uppercase tracking-wider text-muted-foreground">
              Question
            </p>
            <p className="font-serif text-lg text-muted-foreground">{questionText}</p>
          </div>
        )}

        {fb.isCorrect ? (
          /* Correct answer highlight */
          <div className="border-b border-border bg-success-background/50 p-4 md:p-8">
            <p className="mb-3 font-mono text-xs uppercase tracking-wider text-success">
              Correct Answer
            </p>
            <h3 className="font-serif text-xl md:text-2xl text-foreground">{fb.correctAnswer}</h3>
          </div>
        ) : (
          <>
            {/* User's wrong answer */}
            <div className="border-b border-border bg-error-background/50 p-4 md:p-8">
              <p className="mb-3 font-mono text-xs uppercase tracking-wider text-error">
                Your Answer
              </p>
              <h3 className="font-serif text-xl md:text-2xl text-foreground line-through opacity-60">
                {fb.userAnswer ?? '—'}
              </h3>
            </div>

            {/* Correct answer */}
            <div className="border-b border-border bg-success-background/30 p-4 md:p-8">
              <p className="mb-3 font-mono text-xs uppercase tracking-wider text-success">
                Correct Answer
              </p>
              <h3 className="font-serif text-xl text-foreground">{fb.correctAnswer}</h3>
            </div>
          </>
        )}

        {/* Explanation */}
        {fb.explanation && (
          <div className="border-b border-border p-4 md:p-8">
            <p className="mb-4 font-mono text-xs uppercase tracking-wider text-muted-foreground">
              Why This Matters
            </p>
            <p className="max-w-none font-serif text-lg leading-relaxed text-muted-foreground">
              {fb.explanation}
            </p>
          </div>
        )}

        {/* Review schedule metadata */}
        <div className="bg-muted p-4 md:p-6">
          <p className="mb-4 font-mono text-xs uppercase tracking-wider text-muted-foreground">
            Review Schedule {fb.isCorrect ? 'Updated' : 'Adjusted'}
          </p>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <p className="mb-1 font-mono text-xs text-muted-foreground">New Interval</p>
              <p
                className={`text-lg font-semibold tabular-nums ${fb.isCorrect ? 'text-success' : 'text-error'}`}
              >
                {formatInterval(fb.scheduledDays)}
              </p>
            </div>
            <div>
              <p className="mb-1 font-mono text-xs text-muted-foreground">State</p>
              <p className="text-lg font-semibold tabular-nums">{fb.newState ?? '—'}</p>
            </div>
            <div>
              <p className="mb-1 font-mono text-xs text-muted-foreground">Reps</p>
              <p className="text-lg font-semibold tabular-nums">{fb.reps ?? '—'}</p>
            </div>
            <div>
              <p className="mb-1 font-mono text-xs text-muted-foreground">Next Review</p>
              <p className="text-lg font-semibold tabular-nums">
                {formatNextDate(fb.scheduledDays)}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
