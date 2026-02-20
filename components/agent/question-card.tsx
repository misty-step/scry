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
      ? '<1d'
      : `${Math.round(q.stability)}d`
    : null;

  return (
    <div className="max-w-3xl">
      {/* Progress / meta row */}
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <span className="bg-muted px-2.5 py-1 font-mono text-xs text-muted-foreground">
            {q.fsrsState ?? 'new'}
          </span>
          {q.conceptTitle && (
            <span className="text-sm text-muted-foreground">{q.conceptTitle}</span>
          )}
        </div>
        {intervalText && (
          <div className="flex items-center gap-2 text-muted-foreground">
            <Zap className="h-4 w-4" />
            <span className="font-mono text-xs">Interval: {intervalText}</span>
          </div>
        )}
      </div>

      {/* Question card */}
      <div className="border border-border bg-background shadow-sm">
        <div className="border-b border-border p-4 md:p-10">
          <p className="mb-4 font-mono text-xs uppercase tracking-wider text-muted-foreground">
            Question
          </p>
          <h3 className="font-serif text-xl md:text-4xl leading-[1.15] tracking-tight text-foreground">
            {q.question}
          </h3>
          <p className="mt-4 text-sm italic text-muted-foreground">
            Select the best answer from the options below.
          </p>
        </div>

        <div className="space-y-3 p-4 md:p-6">
          {q.options?.map((opt, i) => (
            <button
              key={i}
              onClick={() => handleClick(opt)}
              disabled={isDisabled}
              className={cn(
                'group w-full border p-3 md:p-5 text-left',
                !isDisabled && 'border-border hover:border-primary hover:bg-primary/5',
                selected === opt && 'border-primary bg-primary/10 font-medium',
                isDisabled && selected !== opt && 'border-muted text-muted-foreground opacity-60'
              )}
            >
              <div className="flex items-start gap-4">
                <span
                  className={cn(
                    'flex h-8 w-8 shrink-0 items-center justify-center font-mono text-sm font-medium',
                    !isDisabled &&
                      'bg-muted text-muted-foreground group-hover:bg-background group-hover:text-primary',
                    selected === opt && 'bg-primary text-primary-foreground',
                    isDisabled && selected !== opt && 'bg-muted text-muted-foreground'
                  )}
                >
                  {LETTERS[i] ?? i + 1}
                </span>
                <p className="flex-1 text-foreground">{opt}</p>
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
