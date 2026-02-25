'use client';

import type { ReactElement } from 'react';
import { CalendarClock, CheckCircle2, Gauge, XCircle } from 'lucide-react';
import { formatReviewStageLabel } from '@/lib/review-stage';

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

function splitInlineCodeSegments(text: string) {
  const segments: Array<{ type: 'text' | 'code'; value: string }> = [];
  let cursor = 0;

  while (cursor < text.length) {
    const codeStart = text.indexOf('`', cursor);
    if (codeStart === -1) {
      segments.push({ type: 'text', value: text.slice(cursor) });
      break;
    }

    if (codeStart > cursor) {
      segments.push({ type: 'text', value: text.slice(cursor, codeStart) });
    }

    const codeEnd = text.indexOf('`', codeStart + 1);
    if (codeEnd === -1) {
      segments.push({ type: 'text', value: text.slice(codeStart) });
      break;
    }

    segments.push({ type: 'code', value: text.slice(codeStart + 1, codeEnd) });
    cursor = codeEnd + 1;
  }

  return segments;
}

function renderEmphasis(text: string, keyPrefix: string) {
  const nodes: Array<string | ReactElement> = [];
  let cursor = 0;
  let key = 0;

  const pushPlainText = (value: string) => {
    if (!value) return;
    nodes.push(value);
  };

  while (cursor < text.length) {
    if (text.startsWith('**', cursor)) {
      const end = text.indexOf('**', cursor + 2);
      if (end !== -1) {
        nodes.push(
          <strong key={`${keyPrefix}-strong-${key++}`}>
            {renderEmphasis(text.slice(cursor + 2, end), `${keyPrefix}-s${key}`)}
          </strong>
        );
        cursor = end + 2;
        continue;
      }
    }

    if (text[cursor] === '*') {
      const end = text.indexOf('*', cursor + 1);
      if (end !== -1) {
        nodes.push(
          <em key={`${keyPrefix}-em-${key++}`}>
            {renderEmphasis(text.slice(cursor + 1, end), `${keyPrefix}-e${key}`)}
          </em>
        );
        cursor = end + 1;
        continue;
      }
    }

    const nextBold = text.indexOf('**', cursor);
    const nextItalic = text.indexOf('*', cursor);
    const nextTokenCandidates = [nextBold, nextItalic].filter((value) => value >= 0);
    const nextToken =
      nextTokenCandidates.length > 0 ? Math.min(...nextTokenCandidates) : text.length;

    if (nextToken <= cursor) {
      pushPlainText(text[cursor] ?? '');
      cursor += 1;
      continue;
    }

    pushPlainText(text.slice(cursor, nextToken));
    cursor = nextToken;
  }

  return nodes;
}

function renderInlineMarkdown(text: string) {
  const nodes: Array<string | ReactElement> = [];

  splitInlineCodeSegments(text).forEach((segment, index) => {
    if (segment.type === 'code') {
      nodes.push(
        <code key={`code-${index}`} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.9em]">
          {segment.value}
        </code>
      );
      return;
    }

    nodes.push(...renderEmphasis(segment.value, `segment-${index}`));
  });

  return nodes;
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
                {formatReviewStageLabel(fb.newState, { fallback: '—' })}{' '}
                {getStageProgress(fb.newState).index}/{getStageProgress(fb.newState).total}
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
