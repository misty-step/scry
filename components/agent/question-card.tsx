'use client';

import { useState } from 'react';
import { Zap } from 'lucide-react';
import { formatReviewStageLabel } from '@/lib/review-stage';
import { cn } from '@/lib/utils';

interface QuestionData {
  conceptTitle?: string;
  fsrsState?: string;
  question?: string;
  type?: string;
  options?: string[];
  retrievability?: number;
  lapses?: number;
  reps?: number;
  stability?: number;
}

interface QuestionCardProps {
  question: Record<string, unknown>;
  onAnswer?: (answer: string) => void;
  disabled?: boolean;
}

const LETTERS = ['A', 'B', 'C', 'D', 'E', 'F'];

export function QuestionCard({
  question,
  onAnswer,
  disabled: externalDisabled,
}: QuestionCardProps) {
  const q = (typeof question === 'object' && question !== null ? question : {}) as QuestionData;
  const [selected, setSelected] = useState<string | null>(null);

  if (!q.question) return null;

  const isDisabled = externalDisabled || selected !== null;

  const handleClick = (option: string) => {
    if (isDisabled) return;
    setSelected(option);
    onAnswer?.(option);
  };

  const stabilityText = q.stability
    ? q.stability < 1
      ? 'Stability <1 day (90% recall)'
      : `Stability ~${Math.round(q.stability)} day${Math.round(q.stability) === 1 ? '' : 's'} (90% recall)`
    : null;

  const stageLabel = formatReviewStageLabel(q.fsrsState);

  return (
    <div className="max-w-4xl">
      {/* Progress / meta row */}
      <div className="mb-2 flex items-center justify-between md:mb-4">
        <div className="flex items-center gap-2 md:gap-3">
          <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground md:px-2.5 md:py-1 md:text-xs">
            {stageLabel}
          </span>
          {q.conceptTitle && (
            <span className="text-xs text-muted-foreground md:text-sm">{q.conceptTitle}</span>
          )}
        </div>
        {stabilityText && (
          <div className="hidden items-center gap-2 text-muted-foreground md:flex">
            <Zap className="h-4 w-4" />
            <span className="text-xs">{stabilityText}</span>
          </div>
        )}
      </div>

      {/* Question card */}
      <div className="overflow-hidden rounded-xl border border-border bg-background shadow-sm md:rounded-2xl">
        <div className="border-b border-border px-3 py-3 md:p-8">
          <p className="mb-2 text-[11px] font-medium text-muted-foreground md:mb-4 md:text-xs">Question</p>
          <h3 className="font-serif text-lg leading-snug tracking-tight text-foreground md:text-4xl md:leading-[1.15]">
            {q.question}
          </h3>
        </div>

        <div className="space-y-1.5 bg-secondary/35 p-2.5 md:space-y-2.5 md:p-4">
          {q.options?.map((opt, i) => (
            <button
              key={i}
              onClick={() => handleClick(opt)}
              disabled={isDisabled}
              className={cn(
                'group w-full rounded-lg border bg-background p-2 text-left md:rounded-xl md:p-3.5',
                !isDisabled && 'border-border hover:border-primary hover:bg-primary/5',
                selected === opt && 'border-primary bg-primary/10 font-medium',
                isDisabled && selected !== opt && 'border-muted text-muted-foreground opacity-60'
              )}
            >
              <div className="flex items-start gap-2.5 md:gap-4">
                <span
                  className={cn(
                    'flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-[11px] font-semibold md:h-7 md:w-7 md:rounded-lg md:text-xs',
                    !isDisabled &&
                      'bg-muted text-muted-foreground group-hover:bg-background group-hover:text-primary',
                    selected === opt && 'bg-primary text-primary-foreground',
                    isDisabled && selected !== opt && 'bg-muted text-muted-foreground'
                  )}
                >
                  {LETTERS[i] ?? i + 1}
                </span>
                <p className="flex-1 pt-0.5 text-sm text-foreground md:text-base">{opt}</p>
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
