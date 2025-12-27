'use client';

/**
 * Prompt Diff - Side-by-side comparison of seed vs evolved prompt
 */
import { useState } from 'react';
import { ArrowRightIcon, ColumnsIcon, FileTextIcon } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface PromptDiffProps {
  seedPrompt: string;
  evolvedPrompt: string;
}

type ViewMode = 'side-by-side' | 'evolved-only';

export function PromptDiff({ seedPrompt, evolvedPrompt }: PromptDiffProps) {
  const [viewMode, setViewMode] = useState<ViewMode>('side-by-side');

  // Truncate long prompts for display
  const truncate = (text: string, maxLines: number = 20) => {
    const lines = text.split('\n');
    if (lines.length <= maxLines) return text;
    return lines.slice(0, maxLines).join('\n') + `\n... (${lines.length - maxLines} more lines)`;
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <h4 className="font-medium flex items-center gap-2">
          <FileTextIcon className="h-4 w-4" />
          Prompts
        </h4>
        <div className="flex gap-1">
          <Button
            variant={viewMode === 'side-by-side' ? 'secondary' : 'ghost'}
            size="sm"
            onClick={() => setViewMode('side-by-side')}
          >
            <ColumnsIcon className="h-4 w-4 mr-1" />
            Compare
          </Button>
          <Button
            variant={viewMode === 'evolved-only' ? 'secondary' : 'ghost'}
            size="sm"
            onClick={() => setViewMode('evolved-only')}
          >
            <ArrowRightIcon className="h-4 w-4 mr-1" />
            Evolved
          </Button>
        </div>
      </div>

      {viewMode === 'side-by-side' ? (
        <div className="grid grid-cols-2 gap-4">
          <div>
            <div className="text-sm font-medium text-muted-foreground mb-2">Seed Prompt</div>
            <pre
              className={cn(
                'text-xs p-3 rounded-md bg-muted overflow-auto max-h-80',
                'whitespace-pre-wrap break-words font-mono'
              )}
            >
              {truncate(seedPrompt)}
            </pre>
          </div>
          <div>
            <div className="text-sm font-medium text-muted-foreground mb-2">Evolved Prompt</div>
            <pre
              className={cn(
                'text-xs p-3 rounded-md bg-primary/5 border border-primary/20 overflow-auto max-h-80',
                'whitespace-pre-wrap break-words font-mono'
              )}
            >
              {truncate(evolvedPrompt)}
            </pre>
          </div>
        </div>
      ) : (
        <div>
          <div className="text-sm font-medium text-muted-foreground mb-2">
            Evolved Prompt (Final)
          </div>
          <pre
            className={cn(
              'text-xs p-3 rounded-md bg-primary/5 border border-primary/20 overflow-auto max-h-96',
              'whitespace-pre-wrap break-words font-mono'
            )}
          >
            {evolvedPrompt}
          </pre>
        </div>
      )}
    </div>
  );
}
