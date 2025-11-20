'use client';

import type { ComponentProps, ReactNode } from 'react';
import { Archive, ArchiveRestore, Loader2, Trash2, Undo2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import type { ConceptBulkAction } from '@/types/concepts';

type ButtonVariant = ComponentProps<typeof Button>['variant'];

const ACTION_CONFIG: Record<
  ConceptBulkAction,
  { label: string; icon: ReactNode; variant?: ButtonVariant }
> = {
  archive: {
    label: 'Archive',
    icon: <Archive className="h-3.5 w-3.5" aria-hidden />,
    variant: 'outline',
  },
  unarchive: {
    label: 'Unarchive',
    icon: <ArchiveRestore className="h-3.5 w-3.5" aria-hidden />,
    variant: 'outline',
  },
  delete: {
    label: 'Delete',
    icon: <Trash2 className="h-3.5 w-3.5" aria-hidden />,
    variant: 'destructive',
  },
  restore: {
    label: 'Restore',
    icon: <Undo2 className="h-3.5 w-3.5" aria-hidden />,
    variant: 'secondary',
  },
};

interface BulkActionBarProps {
  selectedCount: number;
  actions: ConceptBulkAction[];
  pendingAction: ConceptBulkAction | null;
  onAction: (action: ConceptBulkAction) => void;
  onClearSelection: () => void;
  disabled?: boolean;
}

export function BulkActionBar({
  selectedCount,
  actions,
  pendingAction,
  onAction,
  onClearSelection,
  disabled = false,
}: BulkActionBarProps) {
  if (selectedCount === 0) {
    return null;
  }

  const isBusy = pendingAction !== null;
  const actionDisabled = disabled || isBusy;

  return (
    <div className="pointer-events-none fixed bottom-6 left-1/2 z-30 w-full max-w-3xl -translate-x-1/2 px-4">
      <div className="pointer-events-auto flex flex-wrap items-center justify-between gap-3 rounded-2xl border bg-background/95 p-4 shadow-2xl backdrop-blur">
        <div className="text-sm font-medium">
          {selectedCount} {selectedCount === 1 ? 'concept selected' : 'concepts selected'}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {actions.map((action) => {
            const config = ACTION_CONFIG[action];
            if (!config) return null;
            return (
              <Button
                key={action}
                size="sm"
                variant={config.variant ?? 'outline'}
                onClick={() => onAction(action)}
                disabled={actionDisabled}
              >
                {pendingAction === action ? (
                  <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" aria-hidden />
                ) : (
                  config.icon
                )}
                {config.label}
              </Button>
            );
          })}
          <Button size="sm" variant="ghost" onClick={onClearSelection} disabled={actionDisabled}>
            Clear
          </Button>
        </div>
      </div>
    </div>
  );
}
