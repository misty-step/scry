'use client';

import { useMutation } from 'convex/react';
import { Archive, ArchiveRestore, MoreHorizontal, Trash2, Undo2 } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { api } from '@/convex/_generated/api';
import type { Doc } from '@/convex/_generated/dataModel';

interface ConceptActionsProps {
  concept: Doc<'concepts'>;
}

export function ConceptActions({ concept }: ConceptActionsProps) {
  const archiveConcept = useMutation(api.concepts.archiveConcept);
  const unarchiveConcept = useMutation(api.concepts.unarchiveConcept);
  const softDeleteConcept = useMutation(api.concepts.softDeleteConcept);
  const restoreConcept = useMutation(api.concepts.restoreConcept);

  const isDeleted = !!concept.deletedAt;
  const isArchived = !!concept.archivedAt;

  const handleArchive = async () => {
    try {
      await archiveConcept({ conceptId: concept._id });
      toast.success('Concept archived');
    } catch {
      toast.error('Failed to archive concept');
    }
  };

  const handleUnarchive = async () => {
    try {
      await unarchiveConcept({ conceptId: concept._id });
      toast.success('Concept restored from archive');
    } catch {
      toast.error('Failed to unarchive concept');
    }
  };

  const handleDelete = async () => {
    // Soft delete doesn't need heavy confirmation, but let's be safe if it's already archived
    // Actually, soft delete is reversible, so standard confirm is fine or even no confirm if we have undo (toast).
    // But let's stick to "Quiet Context" - direct action with toast feedback is usually best.
    // However, since this removes it from view, let's just do it.
    try {
      await softDeleteConcept({ conceptId: concept._id });
      toast.success('Concept moved to trash');
    } catch {
      toast.error('Failed to delete concept');
    }
  };

  const handleRestore = async () => {
    try {
      await restoreConcept({ conceptId: concept._id });
      toast.success('Concept restored');
    } catch {
      toast.error('Failed to restore concept');
    }
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" className="h-8 w-8 p-0">
          <span className="sr-only">Open menu</span>
          <MoreHorizontal className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {isDeleted ? (
          <DropdownMenuItem onClick={handleRestore}>
            <Undo2 className="mr-2 h-4 w-4" />
            Restore
          </DropdownMenuItem>
        ) : (
          <>
            {isArchived ? (
              <DropdownMenuItem onClick={handleUnarchive}>
                <ArchiveRestore className="mr-2 h-4 w-4" />
                Unarchive
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem onClick={handleArchive}>
                <Archive className="mr-2 h-4 w-4" />
                Archive
              </DropdownMenuItem>
            )}
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={handleDelete}
              className="text-destructive focus:text-destructive"
            >
              <Trash2 className="mr-2 h-4 w-4" />
              Delete
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
