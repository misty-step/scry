'use client';

import { CalendarClock, CheckCircle2, Gauge, XCircle } from 'lucide-react';

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
  compact?: boolean;
}

function formatRelativeNextReview(nextReview?: number, scheduledDays?: number): string {
  let ms: number | null = null;
  if (typeof nextReview === 'number' && Number.isFinite(nextReview)) {
    ms = nextReview - Date.now();
  } else if (typeof scheduledDays === 'number' && Number.isFinite(scheduledDays)) {
    ms = scheduledDays * 24 * 60 * 60 * 1000;
  }
  if (ms == null) return '—';
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

function formatStage(state?: string) {
  if (!state) return '—';
  if (state === 'relearning') return 'Relearning';
  if (state === 'learning') return 'Learning';
  if (state === 'review') return 'Review';
  if (state === 'new') return 'New';
  return state;
}

function getStageProgress(state?: string): { index: number; total: number } {
  if (state === 'review') return { index: 3, total: 3 };
  if (state === 'learning' || state === 'relearning') return { index: 2, total: 3 };
  return { index: 1, total: 3 };
}

function renderInlineMarkdown(text: string) {
  const tokens = text.split(/(`[^`]+`|\*\*[^*]+\*\*|\*[^*]+\*)/g);
  return tokens.map((token, i) => {
    if (token.startsWith('`') && token.endsWith('`')) {
      return (
        <code key={i} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.9em]">
          {token.slice(1, -1)}
        </code>
      );
    }
    if (token.startsWith('**') && token.endsWith('**')) {
      return <strong key={i}>{token.slice(2, -2)}</strong>;
    }
    if (token.startsWith('*') && token.endsWith('*')) {
      return <em key={i}>{token.slice(1, -1)}</em>;
    }
    return token;
  });
}

function renderExplanation(text: string) {
  const normalized = normalizeExplanationText(text);
  const sections = normalized
    .trim()
    .split(/\n\s*\n/g)
    .map((section) => section.trim())
    .filter(Boolean);

  return sections.map((section, i) => {
    const lines = section.split('\n').map((line) => line.trim());
    const bulletLines = lines.filter((line) => /^[-*]\s+/.test(line));
    const orderedLines = lines.filter((line) => /^\d+\.\s+/.test(line));

    if (bulletLines.length === lines.length) {
      return (
        <ul key={i} className="ml-6 list-disc space-y-2">
          {bulletLines.map((line, idx) => (
            <li key={idx}>{renderInlineMarkdown(line.replace(/^[-*]\s+/, ''))}</li>
          ))}
        </ul>
      );
    }

    if (orderedLines.length === lines.length) {
      return (
        <ol key={i} className="ml-6 list-decimal space-y-2">
          {orderedLines.map((line, idx) => (
            <li key={idx}>{renderInlineMarkdown(line.replace(/^\d+\.\s+/, ''))}</li>
          ))}
        </ol>
      );
    }

    return (
      <p key={i} className="whitespace-pre-wrap">
        {renderInlineMarkdown(section)}
      </p>
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

export function FeedbackCard({ data, questionText, compact = false }: FeedbackCardProps) {
  if (typeof data !== 'object' || data === null) return null;
  const fb = data as FeedbackData;
  const sectionPadding = compact ? 'p-3.5 md:p-5' : 'p-4 md:p-8';
  const labelClass = compact ? 'mb-2 text-[11px] font-medium' : 'mb-3 text-xs font-medium';
  const answerClass = compact
    ? 'font-serif text-lg md:text-xl text-foreground'
    : 'font-serif text-xl md:text-2xl text-foreground';
  const explanationClass = compact
    ? 'max-w-none space-y-3 font-serif text-base leading-relaxed text-foreground/85'
    : 'max-w-none space-y-4 font-serif text-lg leading-relaxed text-foreground/85';
  const statValueClass = compact ? 'text-sm font-semibold' : 'text-base font-semibold';

  return (
    <div className={compact ? 'max-w-none' : 'max-w-4xl'}>
      {/* Status badge */}
      <div className={compact ? 'mb-3 flex items-center gap-2.5' : 'mb-4 flex items-center gap-3'}>
        {fb.isCorrect ? (
          <div className="flex items-center gap-2 rounded-full bg-success-background px-3 py-1.5">
            <CheckCircle2 className="h-4 w-4 text-success" />
            <span className="text-sm font-medium text-success">Correct</span>
          </div>
        ) : (
          <div className="flex items-center gap-2 rounded-full bg-error-background px-3 py-1.5">
            <XCircle className="h-4 w-4 text-error" />
            <span className="text-sm font-medium text-error">Incorrect</span>
          </div>
        )}
        {fb.conceptTitle && (
          <span className="text-sm text-muted-foreground">{fb.conceptTitle}</span>
        )}
      </div>

      <div className="overflow-hidden rounded-2xl border border-border bg-background shadow-sm">
        {/* Question context */}
        {questionText && (
          <div className={`border-b border-border bg-secondary ${sectionPadding}`}>
            <p className={`${labelClass} text-muted-foreground`}>Question</p>
            <p
              className={
                compact
                  ? 'font-serif text-base leading-snug text-muted-foreground'
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
                  ? 'mb-3 text-[11px] font-medium text-muted-foreground'
                  : 'mb-4 text-xs font-medium text-muted-foreground'
              }
            >
              Answer Breakdown
            </p>
            <div className={explanationClass}>{renderExplanation(fb.explanation)}</div>
          </div>
        )}

        {/* Review schedule metadata */}
        <div className={compact ? 'bg-muted/80 p-3.5 md:p-5' : 'bg-muted/80 p-4 md:p-6'}>
          <p
            className={
              compact
                ? 'mb-3 text-[11px] font-medium text-muted-foreground'
                : 'mb-4 text-xs font-medium text-muted-foreground'
            }
          >
            Review Snapshot
          </p>
          <div className="grid grid-cols-3 gap-2">
            <div className="rounded-lg border border-border/70 bg-background/80 px-2.5 py-2">
              <p className="mb-1 inline-flex items-center gap-1 text-[10px] text-muted-foreground">
                <Gauge className="h-3 w-3" />
                Stage
              </p>
              <p className={compact ? 'text-sm font-semibold' : statValueClass}>
                {formatStage(fb.newState)} {getStageProgress(fb.newState).index}/
                {getStageProgress(fb.newState).total}
              </p>
              <div className="mt-1 flex items-center gap-1">
                {[1, 2, 3].map((step) => (
                  <span
                    key={step}
                    className={`h-1.5 flex-1 rounded-full ${step <= getStageProgress(fb.newState).index ? 'bg-primary' : 'bg-border'}`}
                  />
                ))}
              </div>
            </div>
            <div className="rounded-lg border border-border/70 bg-background/80 px-2.5 py-2">
              <p className="mb-1 inline-flex items-center gap-1 text-[10px] text-muted-foreground">
                <CheckCircle2 className="h-3 w-3" />
                Reviews
              </p>
              <p className={`${compact ? 'text-sm font-semibold' : statValueClass} tabular-nums`}>
                {fb.reps ?? '—'}
              </p>
            </div>
            <div className="rounded-lg border border-border/70 bg-background/80 px-2.5 py-2">
              <p className="mb-1 inline-flex items-center gap-1 text-[10px] text-muted-foreground">
                <CalendarClock className="h-3 w-3" />
                Next review
              </p>
              <p className={compact ? 'text-sm font-semibold' : statValueClass}>
                {formatRelativeNextReview(fb.nextReview, fb.scheduledDays)}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
