'use client';

import { Archive, MoreVertical, Pencil, PenSquare } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';

interface ReviewActionsDropdownProps {
  totalPhrasings: number | null;
  onEditPhrasing: () => void;
  onEditConcept: () => void;
  onArchivePhrasing: () => void;
  onArchiveConcept: () => void;
}

export function ReviewActionsDropdown({
  totalPhrasings,
  onEditPhrasing,
  onEditConcept,
  onArchivePhrasing,
  onArchiveConcept,
}: ReviewActionsDropdownProps) {
  const canArchivePhrasing = totalPhrasings === null || totalPhrasings > 1;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label="Open review actions"
          className="h-11 w-11 p-0 rounded-full border border-border/60 hover:border-border"
        >
          <MoreVertical className="h-5 w-5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-[220px]" sideOffset={6}>
        <DropdownMenuItem onClick={onEditPhrasing} className="gap-2" style={{ minHeight: 44 }}>
          <Pencil className="h-4 w-4" />
          Edit Phrasing
        </DropdownMenuItem>
        <DropdownMenuItem onClick={onEditConcept} className="gap-2" style={{ minHeight: 44 }}>
          <PenSquare className="h-4 w-4" />
          Edit Concept
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {canArchivePhrasing && (
          <DropdownMenuItem
            onClick={onArchivePhrasing}
            className="gap-2 text-muted-foreground"
            style={{ minHeight: 44 }}
          >
            <Archive className="h-4 w-4" />
            Archive Phrasing
          </DropdownMenuItem>
        )}
        <DropdownMenuItem
          onClick={onArchiveConcept}
          className="gap-2 text-destructive focus:text-destructive"
          style={{ minHeight: 44 }}
        >
          <Archive className="h-4 w-4" />
          Archive Concept
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
