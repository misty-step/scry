'use client';

import type { ReactElement } from 'react';
import { CalendarClock, CheckCircle2, Gauge, XCircle } from 'lucide-react';
import { formatReviewStageLabel } from '@/lib/review-stage';
import { cn } from '@/lib/utils';
import { renderInlineMarkdown } from './rich-text';

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
  feedback: Record<string, unknown>;
  questionText?: string;
  compact?: boolean;
}

function formatRelativeNextReview(nextReview?: number, scheduledDays?: number): string {
  let ms: number | null = null;
  if (typeof nextReview === 'number' && Number.isFinite(nextReview)) {
    ms = nextReview - Date.now();
  } else if (typeof scheduledDays === 'number' && Number.isFinite(scheduledDays)) {
    ms = scheduledDays * 24 * 60 * 60 * 1000;
  }
  if (ms === null) return '—';
  if (ms <= 0) return 'now';
  const minutes = Math.round(ms / 60000);
  if (minutes < 60) return `in ${minutes}m`;
  const hours = Math.round(minutes / 60);
  if (hours < 48) return `in ${hours}h`;
  const days = Math.round(hours / 24);
  if (days < 14) return `in ${days}d`;
  const weeks = Math.round(days / 7);
  return `in ${weeks}w`;
}

function getStageProgress(state?: string): { index: number; total: number } {
  if (state === 'review') return { index: 3, total: 3 };
  if (state === 'learning' || state === 'relearning') return { index: 2, total: 3 };
  return { index: 1, total: 3 };
}

function renderExplanation(text: string) {
  const normalized = normalizeExplanationText(text);
  const sections = normalized
    .trim()
    .split(/\n\s*\n/g)
    .map((section) => section.trim())
    .filter(Boolean);

  return sections.map((section, sectionIndex) => {
    const lines = section
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean);

    const blocks: ReactElement[] = [];
    let lineIndex = 0;

    while (lineIndex < lines.length) {
      const line = lines[lineIndex] ?? '';
      const bulletMatch = /^[-*]\s+(.+)$/.exec(line);
      if (bulletMatch) {
        const items: string[] = [];
        while (lineIndex < lines.length) {
          const current = lines[lineIndex] ?? '';
          const match = /^[-*]\s+(.+)$/.exec(current);
          if (!match) break;
          items.push(match[1]);
          lineIndex += 1;
        }
        blocks.push(
          <ul key={`section-${sectionIndex}-ul-${lineIndex}`} className="ml-6 list-disc space-y-2">
            {items.map((item, idx) => (
              <li key={idx}>{renderInlineMarkdown(item)}</li>
            ))}
          </ul>
        );
        continue;
      }

      const orderedMatch = /^\d+\.\s+(.+)$/.exec(line);
      if (orderedMatch) {
        const items: string[] = [];
        while (lineIndex < lines.length) {
          const current = lines[lineIndex] ?? '';
          const match = /^\d+\.\s+(.+)$/.exec(current);
          if (!match) break;
          items.push(match[1]);
          lineIndex += 1;
        }
        blocks.push(
          <ol
            key={`section-${sectionIndex}-ol-${lineIndex}`}
            className="ml-6 list-decimal space-y-2"
          >
            {items.map((item, idx) => (
              <li key={idx}>{renderInlineMarkdown(item)}</li>
            ))}
          </ol>
        );
        continue;
      }

      const paragraphLines: string[] = [];
      while (lineIndex < lines.length) {
        const current = lines[lineIndex] ?? '';
        if (/^[-*]\s+/.test(current) || /^\d+\.\s+/.test(current)) {
          break;
        }
        paragraphLines.push(current);
        lineIndex += 1;
      }

      if (paragraphLines.length > 0) {
        blocks.push(
          <p key={`section-${sectionIndex}-p-${lineIndex}`} className="whitespace-pre-wrap">
            {renderInlineMarkdown(paragraphLines.join('\n'))}
          </p>
        );
      }
    }

    return (
      <div key={`section-${sectionIndex}`} className="space-y-3">
        {blocks}
      </div>
    );
  });
}

function normalizeExplanationText(text: string) {
  const trimmed = text.trim();
  if (!trimmed) return trimmed;
  if (trimmed.includes('\n')) return trimmed;

  const sentences = trimmed
    .split(/(?<=[.?!])\s+(?=[A-Z0-9])/)
    .map((sentence) => sentence.trim())
    .filter(Boolean);

  if (sentences.length < 3) return trimmed;

  const grouped: string[] = [];
  for (let i = 0; i < sentences.length; i += 2) {
    grouped.push(sentences.slice(i, i + 2).join(' '));
  }

  return grouped.join('\n\n');
}

export function FeedbackCard({ feedback, questionText, compact = false }: FeedbackCardProps) {
  if (typeof feedback !== 'object' || feedback === null) return null;
  const fb = feedback as FeedbackData;
  const sectionPadding = compact ? 'px-3 py-2.5 md:p-5' : 'p-4 md:p-8';
  const labelClass = compact ? 'mb-1.5 text-[11px] font-medium' : 'mb-3 text-xs font-medium';
  const answerClass = compact
    ? 'font-serif text-base md:text-xl text-foreground'
    : 'font-serif text-xl md:text-2xl text-foreground';
  const explanationClass = compact
    ? 'max-w-none space-y-2 font-serif text-[0.9375rem] leading-snug text-foreground/85'
    : 'max-w-none space-y-4 font-serif text-lg leading-relaxed text-foreground/85';
  const statValueClass = compact ? 'text-sm font-semibold' : 'text-base font-semibold';
  const stageProgress = getStageProgress(fb.newState);

  return (
    <div className={compact ? 'max-w-none' : 'max-w-4xl'}>
      {/* Status badge */}
      <div className={compact ? 'mb-2 flex items-center gap-2' : 'mb-4 flex items-center gap-3'}>
        {fb.isCorrect ? (
          <div className={cn('flex items-center gap-1.5 rounded-full bg-success-background', compact ? 'px-2.5 py-1' : 'px-3 py-1.5')}>
            <CheckCircle2 className={compact ? 'h-3.5 w-3.5 text-success' : 'h-4 w-4 text-success'} />
            <span className={cn('font-medium text-success', compact ? 'text-xs' : 'text-sm')}>Correct</span>
          </div>
        ) : (
          <div className={cn('flex items-center gap-1.5 rounded-full bg-error-background', compact ? 'px-2.5 py-1' : 'px-3 py-1.5')}>
            <XCircle className={compact ? 'h-3.5 w-3.5 text-error' : 'h-4 w-4 text-error'} />
            <span className={cn('font-medium text-error', compact ? 'text-xs' : 'text-sm')}>Incorrect</span>
          </div>
        )}
        {fb.conceptTitle && (
          <span className={cn('text-muted-foreground', compact ? 'text-xs' : 'text-sm')}>{fb.conceptTitle}</span>
        )}
      </div>

      <div className="overflow-hidden rounded-xl border border-border bg-background shadow-sm md:rounded-2xl">
        {/* Question context */}
        {questionText && (
          <div className={`border-b border-border bg-secondary ${sectionPadding}`}>
            <p className={`${labelClass} text-muted-foreground`}>Question</p>
            <p
              className={
                compact
                  ? 'font-serif text-sm leading-snug text-muted-foreground'
                  : 'font-serif text-lg text-muted-foreground'
              }
            >
              {questionText}
            </p>
          </div>
        )}

        {fb.isCorrect ? (
          /* Correct answer highlight */
          <div className={`border-b border-border bg-success-background/50 ${sectionPadding}`}>
            <p className={`${labelClass} text-success`}>Correct Answer</p>
            <h3 className={answerClass}>{fb.correctAnswer}</h3>
          </div>
        ) : (
          <>
            {/* User's wrong answer */}
            <div className={`border-b border-border bg-error-background/50 ${sectionPadding}`}>
              <p className={`${labelClass} text-error`}>Your Answer</p>
              <h3 className={`${answerClass} line-through opacity-60`}>{fb.userAnswer ?? '—'}</h3>
            </div>

            {/* Correct answer */}
            <div className={`border-b border-border bg-success-background/30 ${sectionPadding}`}>
              <p className={`${labelClass} text-success`}>Correct Answer</p>
              <h3 className={answerClass}>{fb.correctAnswer}</h3>
            </div>
          </>
        )}

        {/* Explanation */}
        {fb.explanation && (
          <div className={`border-b border-border ${sectionPadding}`}>
            <p
              className={
                compact
                  ? 'mb-1.5 text-[11px] font-medium text-muted-foreground'
                  : 'mb-4 text-xs font-medium text-muted-foreground'
              }
            >
              Answer Breakdown
            </p>
            <div className={explanationClass}>{renderExplanation(fb.explanation)}</div>
          </div>
        )}

        {/* Review schedule metadata */}
        <div className={compact ? 'bg-muted/80 px-3 py-2.5 md:p-5' : 'bg-muted/80 p-4 md:p-6'}>
          <p
            className={
              compact
                ? 'mb-2 text-[11px] font-medium text-muted-foreground'
                : 'mb-4 text-xs font-medium text-muted-foreground'
            }
          >
            Review Snapshot
          </p>
          <div className="grid grid-cols-3 gap-1.5 md:gap-2">
            <div className="rounded-md border border-border/70 bg-background/80 px-2 py-1.5 md:rounded-lg md:px-2.5 md:py-2">
              <p className="mb-0.5 inline-flex items-center gap-1 text-[10px] text-muted-foreground md:mb-1">
                <Gauge className="h-3 w-3" />
                Stage
              </p>
              <p className={compact ? 'text-xs font-semibold md:text-sm' : statValueClass}>
                {formatReviewStageLabel(fb.newState, { fallback: '—' })} {stageProgress.index}/
                {stageProgress.total}
              </p>
              <div className="mt-1 flex items-center gap-0.5 md:gap-1">
                {[1, 2, 3].map((step) => (
                  <span
                    key={step}
                    className={`h-1 flex-1 rounded-full md:h-1.5 ${step <= stageProgress.index ? 'bg-primary' : 'bg-border'}`}
                  />
                ))}
              </div>
            </div>
            <div className="rounded-md border border-border/70 bg-background/80 px-2 py-1.5 md:rounded-lg md:px-2.5 md:py-2">
              <p className="mb-0.5 inline-flex items-center gap-1 text-[10px] text-muted-foreground md:mb-1">
                <CheckCircle2 className="h-3 w-3" />
                Reviews
              </p>
              <p className={`${compact ? 'text-xs font-semibold md:text-sm' : statValueClass} tabular-nums`}>
                {fb.reps ?? '—'}
              </p>
            </div>
            <div className="rounded-md border border-border/70 bg-background/80 px-2 py-1.5 md:rounded-lg md:px-2.5 md:py-2">
              <p className="mb-0.5 inline-flex items-center gap-1 text-[10px] text-muted-foreground md:mb-1">
                <CalendarClock className="h-3 w-3" />
                Next review
              </p>
              <p className={compact ? 'text-xs font-semibold md:text-sm' : statValueClass}>
                {formatRelativeNextReview(fb.nextReview, fb.scheduledDays)}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
