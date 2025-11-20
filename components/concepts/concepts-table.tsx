'use client';

import Link from 'next/link';
import type { CheckedState } from '@radix-ui/react-checkbox';
import { formatDistanceToNow } from 'date-fns';
import {
  AlertTriangle,
  ArrowUpRight,
  BookOpenCheck,
  Clock3,
  Loader2,
  Sparkles,
} from 'lucide-react';
import { ConceptActions } from '@/components/concepts/concept-actions';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type { Doc, Id } from '@/convex/_generated/dataModel';
import type { ConceptsView } from '@/hooks/use-concepts-query';
import { cn } from '@/lib/utils';

interface ConceptsTableProps {
  concepts: Doc<'concepts'>[];
  serverTime: number;
  view: ConceptsView;
  selectedIds: Record<string, true>;
  onToggleConcept: (conceptId: Id<'concepts'>) => void;
  onTogglePage: (conceptIds: Id<'concepts'>[], select: boolean) => void;
  onGenerateThin?: (conceptId: Id<'concepts'>) => Promise<void> | void;
  pendingGenerationIds?: Record<string, true>;
}

export function ConceptsTable({
  concepts,
  serverTime,
  view,
  selectedIds,
  onToggleConcept,
  onTogglePage,
  onGenerateThin,
  pendingGenerationIds = {},
}: ConceptsTableProps) {
  if (concepts.length === 0) {
    return null;
  }

  const isArchivedView = view === 'archived';
  const isDeletedView = view === 'deleted';
  const isPassiveView = isArchivedView || isDeletedView;

  const conceptIds = concepts.map((concept) => concept._id);
  const selectedOnPage = conceptIds.filter((id) => Boolean(selectedIds[id]));

  const headerState: CheckedState =
    conceptIds.length === 0
      ? false
      : selectedOnPage.length === conceptIds.length
        ? true
        : selectedOnPage.length > 0
          ? 'indeterminate'
          : false;

  const handleTogglePage: (value: CheckedState) => void = () => {
    if (conceptIds.length === 0) return;
    const shouldSelect = selectedOnPage.length !== conceptIds.length;
    onTogglePage(conceptIds, shouldSelect);
  };

  return (
    <div className="overflow-hidden rounded-lg border bg-card">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10">
              <Checkbox
                aria-label="Select all concepts on this page"
                checked={headerState}
                onCheckedChange={handleTogglePage}
              />
            </TableHead>
            <TableHead className="w-[45%]">Concept</TableHead>
            <TableHead className="w-[25%]">
              {isPassiveView ? (isArchivedView ? 'Archived' : 'Deleted') : 'Next review'}
            </TableHead>
            <TableHead className="w-[20%] text-center">
              {isPassiveView ? 'Status note' : 'Phrasings'}
            </TableHead>
            <TableHead className="w-[10%]"></TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {concepts.map((concept) => {
            const dueState = getDueState(concept, serverTime);
            const hasThinSignal = (concept.thinScore ?? 0) > 0;
            const hasTensionSignal = (concept.conflictScore ?? 0) > 0;
            const thinDelta = concept.thinScore ?? 0;
            const isSelected = Boolean(selectedIds[concept._id]);
            const pendingGeneration = Boolean(pendingGenerationIds[concept._id]);
            const allowGeneration =
              hasThinSignal && onGenerateThin && !isPassiveView && !concept.deletedAt;
            const lifecycle = getLifecycleState(concept, view);

            return (
              <TableRow key={concept._id} className={cn(isSelected && 'bg-muted/40')}>
                <TableCell className="align-top">
                  <Checkbox
                    aria-label={`Select ${concept.title}`}
                    checked={isSelected}
                    onCheckedChange={() => onToggleConcept(concept._id)}
                  />
                </TableCell>
                <TableCell className="align-top">
                  <div className="space-y-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <Link
                        href={`/concepts/${concept._id}`}
                        className="group inline-flex items-center gap-1 font-medium text-foreground underline-offset-4 hover:underline"
                        aria-label={`View concept: ${concept.title}`}
                      >
                        {concept.title}
                        <ArrowUpRight className="h-3 w-3 text-muted-foreground opacity-0 transition group-hover:opacity-100" aria-hidden="true" />
                      </Link>
                      {concept.fsrs.state && !isPassiveView ? (
                        <Badge variant="outline" className="text-xs capitalize">
                          {concept.fsrs.state}
                        </Badge>
                      ) : null}
                      {!isPassiveView && hasThinSignal ? (
                        <Badge
                          variant="outline"
                          className="bg-amber-500/10 text-amber-900 dark:text-amber-200"
                        >
                          Thin{thinDelta ? ` (${thinDelta})` : ''}
                        </Badge>
                      ) : null}
                      {!isPassiveView && hasTensionSignal ? (
                        <Badge
                          variant="outline"
                          className="bg-rose-500/10 text-rose-900 dark:text-rose-200"
                        >
                          Tension
                        </Badge>
                      ) : null}
                      {(isArchivedView && concept.archivedAt) ||
                      (isDeletedView && concept.deletedAt) ? (
                        <Badge variant="outline" className="text-xs">
                          {isArchivedView ? 'Archived' : 'Deleted'}
                        </Badge>
                      ) : null}
                    </div>
                    {concept.description && (
                      <p className="text-sm text-muted-foreground line-clamp-2">
                        {concept.description}
                      </p>
                    )}
                    <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                      <span className="inline-flex items-center gap-1">
                        <Clock3 className="h-3 w-3" aria-hidden />
                        {concept.updatedAt
                          ? `Updated ${formatDistanceToNow(concept.updatedAt, { addSuffix: true })}`
                          : 'Auto-generated'}
                      </span>
                      {!isPassiveView && hasTensionSignal ? (
                        <span className="inline-flex items-center gap-1 text-rose-600 dark:text-rose-300">
                          <AlertTriangle className="h-3 w-3" aria-hidden />
                          Needs attention
                        </span>
                      ) : null}
                      <span className="inline-flex items-center gap-1">
                        <BookOpenCheck className="h-3 w-3" aria-hidden />
                        {concept.phrasingCount}{' '}
                        {concept.phrasingCount === 1 ? 'phrasing' : 'phrasings'}
                      </span>
                    </div>
                    {allowGeneration ? (
                      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                        <Button
                          size="sm"
                          variant="secondary"
                          className="h-7 px-3 text-xs"
                          disabled={pendingGeneration}
                          onClick={() => onGenerateThin(concept._id)}
                        >
                          {pendingGeneration ? (
                            <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" aria-hidden />
                          ) : (
                            <Sparkles className="mr-2 h-3.5 w-3.5" aria-hidden />
                          )}
                          Generate phrasings
                        </Button>
                        <span>
                          {thinDelta === 1
                            ? 'Add 1 more phrasing to stabilize'
                            : `Add ${thinDelta} more phrasings to stabilize`}
                        </span>
                      </div>
                    ) : null}
                  </div>
                </TableCell>
                <TableCell className="align-top">
                  {isPassiveView && lifecycle ? (
                    <div className="space-y-1 text-sm text-muted-foreground">
                      <div className="flex items-center gap-2">
                        <Badge variant="secondary" className="bg-muted text-foreground">
                          {lifecycle.label}
                        </Badge>
                        {lifecycle.relative && (
                          <span className="text-xs text-muted-foreground">
                            {lifecycle.relative}
                          </span>
                        )}
                      </div>
                      {lifecycle.timestampLabel ? (
                        <span className="text-xs text-muted-foreground">
                          {lifecycle.timestampLabel}
                        </span>
                      ) : null}
                    </div>
                  ) : (
                    <div className="flex flex-col gap-1">
                      <div className="flex items-center gap-2">
                        <Badge className={dueState.badgeClass}>{dueState.label}</Badge>
                        {dueState.nextReviewLabel && (
                          <span className="text-xs text-muted-foreground">
                            {dueState.nextReviewLabel}
                          </span>
                        )}
                      </div>
                      {dueState.sublabel ? (
                        <span className="text-xs text-muted-foreground">{dueState.sublabel}</span>
                      ) : null}
                    </div>
                  )}
                </TableCell>
                <TableCell className="text-center align-top">
                  {!isPassiveView ? (
                    <div className="flex flex-col items-center gap-1">
                      <Badge variant="secondary" className="justify-center text-sm">
                        {concept.phrasingCount}{' '}
                        {concept.phrasingCount === 1 ? 'phrasing' : 'phrasings'}
                      </Badge>
                      {hasThinSignal && (
                        <span className="text-xs text-muted-foreground">
                          Needs {thinDelta} more
                        </span>
                      )}
                    </div>
                  ) : (
                    <div className="text-xs text-muted-foreground">
                      {isArchivedView
                        ? 'Not scheduling while archived.'
                        : 'Pending permanent deletion.'}
                    </div>
                  )}
                </TableCell>
                <TableCell className="align-top text-right">
                  <ConceptActions concept={concept} />
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}

function getDueState(concept: Doc<'concepts'>, now: number) {
  const nextReview = concept.fsrs.nextReview;
  const hasNeverBeenReviewed = (concept.fsrs.reps ?? 0) === 0;

  if (!nextReview || hasNeverBeenReviewed) {
    return {
      label: 'New',
      badgeClass: 'bg-blue-500/10 text-blue-900 dark:text-blue-200',
      nextReviewLabel: null,
      sublabel: hasNeverBeenReviewed ? 'No reviews yet' : null,
    };
  }

  if (nextReview <= now) {
    return {
      label: 'Due now',
      badgeClass: 'bg-red-500 text-white',
      nextReviewLabel: null,
      sublabel: null,
    };
  }

  return {
    label: 'Scheduled',
    badgeClass: 'bg-muted text-foreground',
    nextReviewLabel: formatDistanceToNow(nextReview, { addSuffix: true }),
    sublabel: null,
  };
}

function getLifecycleState(concept: Doc<'concepts'>, view: ConceptsView) {
  if (view === 'archived' && concept.archivedAt) {
    return {
      label: 'Archived',
      relative: formatDistanceToNow(concept.archivedAt, { addSuffix: true }),
      timestampLabel: new Date(concept.archivedAt).toLocaleString(),
    };
  }

  if (view === 'deleted' && concept.deletedAt) {
    return {
      label: 'Deleted',
      relative: formatDistanceToNow(concept.deletedAt, { addSuffix: true }),
      timestampLabel: new Date(concept.deletedAt).toLocaleString(),
    };
  }

  return null;
}
