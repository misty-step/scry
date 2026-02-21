'use client';

import { Info } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { useReviewSession } from './session-context';

export function InlineEditor() {
  const { unifiedEdit } = useReviewSession();

  if (!unifiedEdit.isEditing) {
    return null;
  }

  return (
    <div className="flex items-center gap-2 pt-4">
      <Button
        onClick={() => unifiedEdit.save()}
        disabled={unifiedEdit.isSaving || !unifiedEdit.isDirty}
      >
        {unifiedEdit.isSaving ? 'Saving...' : 'Save Changes'}
      </Button>
      <Button variant="ghost" onClick={() => unifiedEdit.cancel()} disabled={unifiedEdit.isSaving}>
        Cancel
      </Button>
      <Popover>
        <PopoverTrigger asChild>
          <button
            type="button"
            className="ml-auto text-muted-foreground hover:text-foreground transition-colors"
            aria-label="Information about FSRS preservation"
          >
            <Info className="h-5 w-5" />
          </button>
        </PopoverTrigger>
        <PopoverContent className="max-w-xs">
          <p className="text-sm font-medium">Edits preserve your learning progress.</p>
          <p className="text-xs text-muted-foreground mt-1">
            Your FSRS scheduling state (difficulty, stability, next review date) remains unchanged.
            For major content changes, consider archiving and creating a new concept instead.
          </p>
        </PopoverContent>
      </Popover>
    </div>
  );
}
