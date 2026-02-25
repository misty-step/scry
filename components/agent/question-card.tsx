'use client';

import { useState } from 'react';
import { Zap } from 'lucide-react';
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
  data: Record<string, unknown>;
  onAnswer?: (answer: string) => void;
  disabled?: boolean;
}

const LETTERS = ['A', 'B', 'C', 'D', 'E', 'F'];

export function QuestionCard({ data, onAnswer, disabled: externalDisabled }: QuestionCardProps) {
  const q = (typeof data === 'object' && data !== null ? data : {}) as QuestionData;
  const [selected, setSelected] = useState<string | null>(null);

  if (!q.question) return null;

  const isDisabled = externalDisabled || selected !== null;

  const handleClick = (option: string) => {
    if (isDisabled) return;
    setSelected(option);
    onAnswer?.(option);
  };

  const intervalText = q.stability
    ? q.stability < 1
      ? 'Due later today'
      : `Due in ${Math.round(q.stability)} day${Math.round(q.stability) === 1 ? '' : 's'}`
    : null;

  const stageLabel =
    q.fsrsState === 'relearning'
      ? 'Relearning'
      : q.fsrsState === 'learning'
        ? 'Learning'
        : q.fsrsState === 'review'
          ? 'Review'
          : 'New';

  return (
    <div className="max-w-4xl">
      {/* Progress / meta row */}
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <span className="rounded-full bg-muted px-2.5 py-1 text-xs font-medium text-muted-foreground">
            {stageLabel}
          </span>
          {q.conceptTitle && (
            <span className="text-sm text-muted-foreground">{q.conceptTitle}</span>
          )}
        </div>
        {intervalText && (
          <div className="flex items-center gap-2 text-muted-foreground">
            <Zap className="h-4 w-4" />
            <span className="text-xs">{intervalText}</span>
          </div>
        )}
      </div>

      {/* Question card */}
      <div className="overflow-hidden rounded-2xl border border-border bg-background shadow-sm">
        <div className="border-b border-border p-4 md:p-8">
          <p className="mb-4 text-xs font-medium text-muted-foreground">Question</p>
          <h3 className="font-serif text-xl md:text-4xl leading-[1.15] tracking-tight text-foreground">
            {q.question}
          </h3>
        </div>

        <div className="space-y-2 bg-secondary/35 p-3 md:space-y-2.5 md:p-4">
          {q.options?.map((opt, i) => (
            <button
              key={i}
              onClick={() => handleClick(opt)}
              disabled={isDisabled}
              className={cn(
                'group w-full rounded-xl border bg-background p-2.5 text-left md:p-3.5',
                !isDisabled && 'border-border hover:border-primary hover:bg-primary/5',
                selected === opt && 'border-primary bg-primary/10 font-medium',
                isDisabled && selected !== opt && 'border-muted text-muted-foreground opacity-60'
              )}
            >
              <div className="flex items-start gap-4">
                <span
                  className={cn(
                    'flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-xs font-semibold',
                    !isDisabled &&
                      'bg-muted text-muted-foreground group-hover:bg-background group-hover:text-primary',
                    selected === opt && 'bg-primary text-primary-foreground',
                    isDisabled && selected !== opt && 'bg-muted text-muted-foreground'
                  )}
                >
                  {LETTERS[i] ?? i + 1}
                </span>
                <p className="flex-1 pt-0.5 text-foreground">{opt}</p>
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
