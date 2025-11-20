'use client';

import { useEffect, useMemo, useState } from 'react';
import { useUser } from '@clerk/nextjs';
import { useMutation } from 'convex/react';
import { Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { BulkActionBar } from '@/components/concepts/bulk-action-bar';
import { ConceptsEmptyState } from '@/components/concepts/concepts-empty-state';
import { ConceptsTable } from '@/components/concepts/concepts-table';
import { ViewSelector } from '@/components/concepts/view-selector';
import { PageContainer } from '@/components/page-container';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { api } from '@/convex/_generated/api';
import type { Doc, Id } from '@/convex/_generated/dataModel';
import { useConceptsQuery, type ConceptsSort, type ConceptsView } from '@/hooks/use-concepts-query';
import type { ConceptBulkAction } from '@/types/concepts';

const VIEW_TABS: { value: ConceptsView; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'due', label: 'Due' },
  { value: 'thin', label: 'Thin' },
  { value: 'tension', label: 'Tension' },
  { value: 'archived', label: 'Archived' },
  { value: 'deleted', label: 'Trash' },
];

const VIEW_DESCRIPTIONS: Record<ConceptsView, { title: string; body: string }> = {
  all: {
    title: 'Showing all active concepts.',
    body: 'Search or sort to focus on specific material.',
  },
  due: {
    title: 'Concepts ready for review.',
    body: 'Clearing these keeps FSRS intervals honest.',
  },
  thin: {
    title: 'Thin concepts need more phrasing coverage.',
    body: 'Generate variants to stabilize recall depth.',
  },
  tension: {
    title: 'Tension indicates conflicting or overlapping phrasings.',
    body: 'Resolve duplicates or split overloaded ideas.',
  },
  archived: {
    title: 'Archived concepts are paused.',
    body: 'They stay out of review until you restore them.',
  },
  deleted: {
    title: 'Trash holds concepts before permanent removal.',
    body: 'Restore anything you still need.',
  },
};

const BULK_ACTION_SUCCESS: Record<ConceptBulkAction, string> = {
  archive: 'archived',
  unarchive: 'unarchived',
  delete: 'moved to trash',
  restore: 'restored',
};

export function ConceptsClient() {
  const { isSignedIn } = useUser();
  const [view, setView] = useState<ConceptsView>('all');
  const [sort, setSort] = useState<ConceptsSort>('nextReview');
  const [cursor, setCursor] = useState<string | null>(null);
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [pageSize, setPageSize] = useState(25);
  const [searchQuery, setSearchQuery] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [selectedIds, setSelectedIds] = useState<Record<string, true>>({});
  const [pendingGenerationIds, setPendingGenerationIds] = useState<Record<string, true>>({});
  const [pendingBulkAction, setPendingBulkAction] = useState<ConceptBulkAction | null>(null);
  const runBulkAction = useMutation(api.concepts.runBulkAction);
  const requestMorePhrasings = useMutation(api.concepts.requestPhrasingGeneration);

  useEffect(() => {
    const id = setTimeout(() => setDebouncedSearch(searchQuery.trim()), 300);
    return () => clearTimeout(id);
  }, [searchQuery]);

  useEffect(() => {
    setCursor(null);
    setCursorStack([]);
  }, [view, sort, pageSize]);

  useEffect(() => {
    setSelectedIds({});
  }, [view, sort, pageSize, debouncedSearch]);

  useEffect(() => {
    if (debouncedSearch) {
      setCursor(null);
      setCursorStack([]);
    }
  }, [debouncedSearch]);

  const conceptsData = useConceptsQuery({
    enabled: Boolean(isSignedIn),
    cursor: debouncedSearch ? null : cursor,
    pageSize,
    view,
    search: debouncedSearch,
    sort,
  });

  const isLoading = isSignedIn && conceptsData === undefined;
  const concepts = conceptsData?.concepts ?? [];
  const continueCursor =
    debouncedSearch || !conceptsData ? null : (conceptsData.continueCursor ?? null);
  const isDone = debouncedSearch ? true : (conceptsData?.isDone ?? true);
  const serverTime = conceptsData?.serverTime ?? Date.now();

  const hasResults = concepts.length > 0;

  const paginationDisabled = !!debouncedSearch;

  const selectedConceptIds = useMemo(
    () => Object.keys(selectedIds) as Id<'concepts'>[],
    [selectedIds]
  );
  const selectedCount = selectedConceptIds.length;
  const availableBulkActions = useMemo<ConceptBulkAction[]>(() => {
    if (view === 'archived') {
      return ['unarchive', 'delete'];
    }
    if (view === 'deleted') {
      return ['restore'];
    }
    return ['archive', 'delete'];
  }, [view]);

  const handleToggleConcept = (conceptId: Id<'concepts'>) => {
    setSelectedIds((prev) => {
      const next = { ...prev };
      if (next[conceptId]) {
        delete next[conceptId];
      } else {
        next[conceptId] = true;
      }
      return next;
    });
  };

  const handleTogglePageSelection = (conceptIds: Id<'concepts'>[], shouldSelect: boolean) => {
    if (conceptIds.length === 0) {
      return;
    }
    setSelectedIds((prev) => {
      const next = { ...prev };
      let changed = false;

      if (shouldSelect) {
        for (const id of conceptIds) {
          if (!next[id]) {
            next[id] = true;
            changed = true;
          }
        }
      } else {
        for (const id of conceptIds) {
          if (next[id]) {
            delete next[id];
            changed = true;
          }
        }
      }

      return changed ? next : prev;
    });
  };

  const clearSelection = () => setSelectedIds({});

  const handleGenerateThin = async (conceptId: Id<'concepts'>) => {
    if (view === 'archived' || view === 'deleted') {
      return;
    }
    if (pendingGenerationIds[conceptId]) {
      return;
    }
    setPendingGenerationIds((prev) => ({ ...prev, [conceptId]: true }));
    try {
      await requestMorePhrasings({ conceptId });
      toast.success('Generation job started');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to generate new phrasings';
      toast.error(message);
    } finally {
      setPendingGenerationIds((prev) => {
        const next = { ...prev };
        delete next[conceptId];
        return next;
      });
    }
  };

  const viewDescription = VIEW_DESCRIPTIONS[view];

  const handleBulkAction = async (action: ConceptBulkAction) => {
    if (selectedCount === 0) {
      return;
    }
    try {
      setPendingBulkAction(action);
      const result = await runBulkAction({
        action,
        conceptIds: selectedConceptIds,
      });
      const processed = result?.processed ?? 0;
      const skipped = result?.skipped ?? 0;

      if (processed > 0) {
        const verb = BULK_ACTION_SUCCESS[action];
        const noun = processed === 1 ? 'concept' : 'concepts';
        toast.success(`${processed} ${noun} ${verb}`);
      } else {
        toast.info('No concepts were updated');
      }

      if (skipped > 0) {
        toast.warning(`${skipped} already in that state`);
      }

      setSelectedIds({});
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to run bulk action';
      toast.error(message);
    } finally {
      setPendingBulkAction(null);
    }
  };

  useEffect(() => {
    setSelectedIds((prev) => {
      if (Object.keys(prev).length === 0) {
        return prev;
      }

      if (concepts.length === 0) {
        return {};
      }

      const allowed = new Set(concepts.map((concept: Doc<'concepts'>) => concept._id));
      let changed = false;
      const next: Record<string, true> = {};

      for (const id of Object.keys(prev)) {
        if (allowed.has(id as Id<'concepts'>)) {
          next[id] = true;
        } else {
          changed = true;
        }
      }

      return changed ? next : prev;
    });
  }, [concepts]);

  const searchLabel = useMemo(() => {
    if (!debouncedSearch) return null;
    return `Showing top matches for “${debouncedSearch}”`;
  }, [debouncedSearch]);

  const handleNextPage = () => {
    if (!continueCursor || isDone || paginationDisabled) return;
    if (cursor !== null) {
      setCursorStack([...cursorStack, cursor]);
    } else {
      setCursorStack([...cursorStack, '']);
    }
    setCursor(continueCursor);
  };

  const handlePrevPage = () => {
    if (cursorStack.length === 0 || paginationDisabled) return;
    const nextStack = [...cursorStack];
    const previousCursor = nextStack.pop();
    setCursorStack(nextStack);
    setCursor(previousCursor === '' ? null : (previousCursor ?? null));
  };

  return (
    <PageContainer className="py-8 space-y-6">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">Concepts Library</h1>
        <p className="text-sm text-muted-foreground">
          Concept-first view of your knowledge. Track due status, thin spots, and tension signals at
          a glance.
        </p>
      </div>

      <Card className="p-4 space-y-4">
        {/* View selector: scroll pills on mobile, tabs on desktop */}
        <ViewSelector
          value={view}
          onValueChange={(value) => {
            setView(value as ConceptsView);
            setCursor(null);
            setCursorStack([]);
          }}
          options={VIEW_TABS}
        />

        {/* Controls: stacked on mobile, row on desktop */}
        <div className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          {/* Search - full width on mobile */}
          <div className="flex flex-col gap-1 md:w-64">
            <Label htmlFor="concept-search" className="text-xs text-muted-foreground">
              Search concepts
            </Label>
            <Input
              id="concept-search"
              placeholder="Search by title"
              value={searchQuery}
              onChange={(event) => setSearchQuery(event.target.value)}
            />
          </div>

          {/* Sort and page size */}
          <div className="flex items-end gap-3">
            {/* Sort - always visible */}
            <div className="flex flex-col gap-1 flex-1 md:flex-none">
              <Label className="text-xs text-muted-foreground">Sort</Label>
              <Select value={sort} onValueChange={(value) => setSort(value as ConceptsSort)}>
                <SelectTrigger className="w-full md:w-[160px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="nextReview">Next review</SelectItem>
                  <SelectItem value="recent">Recently created</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {/* Page size - hidden on mobile (non-essential control) */}
            <div className="hidden md:flex flex-col gap-1">
              <Label className="text-xs text-muted-foreground">Page size</Label>
              <Select
                value={String(pageSize)}
                onValueChange={(value) => setPageSize(Number(value))}
              >
                <SelectTrigger className="w-[120px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {[10, 25, 50, 75, 100].map((size) => (
                    <SelectItem key={size} value={String(size)}>
                      {size} / page
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        </div>

        {searchLabel && (
          <Badge variant="secondary" className="w-fit">
            {searchLabel}
          </Badge>
        )}
      </Card>

      {viewDescription ? (
        <div className="rounded-xl border bg-muted/40 px-4 py-3 text-sm">
          <p className="font-semibold text-foreground">{viewDescription.title}</p>
          <p className="text-muted-foreground">{viewDescription.body}</p>
        </div>
      ) : null}

      {isLoading ? (
        <div className="flex min-h-[200px] items-center justify-center rounded-lg border border-dashed">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading concepts…
          </div>
        </div>
      ) : hasResults ? (
        <ConceptsTable
          concepts={concepts}
          serverTime={serverTime}
          view={view}
          selectedIds={selectedIds}
          onToggleConcept={handleToggleConcept}
          onTogglePage={handleTogglePageSelection}
          onGenerateThin={handleGenerateThin}
          pendingGenerationIds={pendingGenerationIds}
        />
      ) : (
        <ConceptsEmptyState view={view} searchTerm={debouncedSearch} />
      )}

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="text-sm text-muted-foreground">
          {debouncedSearch
            ? 'Pagination disabled while searching'
            : concepts.length === 0
              ? 'No concepts to paginate'
              : isDone
                ? 'End of results'
                : 'Use the buttons to navigate'}
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            onClick={handlePrevPage}
            disabled={paginationDisabled || cursorStack.length === 0}
          >
            Previous
          </Button>
          <Button
            variant="outline"
            onClick={handleNextPage}
            disabled={paginationDisabled || !continueCursor || isDone || concepts.length === 0}
          >
            Next
          </Button>
        </div>
      </div>
      <BulkActionBar
        selectedCount={selectedCount}
        actions={availableBulkActions}
        pendingAction={pendingBulkAction}
        onAction={handleBulkAction}
        onClearSelection={clearSelection}
      />
    </PageContainer>
  );
}
