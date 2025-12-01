'use client';

import { useEffect, useMemo, useState } from 'react';
import { useQuery } from 'convex/react';
import { Loader2 } from 'lucide-react';
import { GenerationTaskCard } from '@/components/generation-task-card';
import { PageContainer } from '@/components/page-container';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { api } from '@/convex/_generated/api';
import type { GenerationJob } from '@/types/generation-jobs';
import { isActiveJob, isCancelledJob, isCompletedJob, isFailedJob } from '@/types/generation-jobs';

type StatusFilter = 'all' | 'active' | 'completed' | 'failed';

function filterJobsByStatus(jobs: GenerationJob[] | undefined, filter: StatusFilter) {
  if (!jobs) return undefined;

  switch (filter) {
    case 'all':
      return jobs;
    case 'active':
      return jobs.filter((job) => isActiveJob(job));
    case 'completed':
      return jobs.filter((job) => isCompletedJob(job));
    case 'failed':
      return jobs.filter((job) => isFailedJob(job) || isCancelledJob(job));
    default:
      return jobs;
  }
}

function countByStatus(jobs: GenerationJob[] | undefined, status: StatusFilter) {
  if (!jobs) return 0;

  if (status === 'all') return jobs.length;
  return filterJobsByStatus(jobs, status)?.length ?? 0;
}

export function TasksClient() {
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [cursor, setCursor] = useState<string | null>(null);
  const [cursorStack, setCursorStack] = useState<string[]>([]);
  const [pageSize, setPageSize] = useState<number>(25);

  const paginationData = useQuery(api.generationJobs.getRecentJobs, {
    cursor: cursor ?? undefined,
    pageSize,
  });

  const jobs = paginationData?.results as GenerationJob[] | undefined;
  const continueCursor = paginationData?.continueCursor ?? null;
  const isDone = paginationData?.isDone ?? true;

  const filteredJobs = useMemo(() => filterJobsByStatus(jobs, statusFilter), [jobs, statusFilter]);

  const handleNextPage = () => {
    if (!continueCursor || isDone) return;

    setCursorStack((prev) => [...prev, cursor ?? '']);
    setCursor(continueCursor);
  };

  const handlePrevPage = () => {
    setCursorStack((prev) => {
      if (prev.length === 0) return prev;
      const nextStack = [...prev];
      const previousCursor = nextStack.pop();
      setCursor(previousCursor === '' ? null : (previousCursor ?? null));
      return nextStack;
    });
  };

  useEffect(() => {
    if (cursor === null && cursorStack.length === 0) {
      return;
    }

    // Defer pagination reset to avoid cascading renders in strict mode
    const id = window.setTimeout(() => {
      setCursor(null);
      setCursorStack([]);
    }, 0);

    return () => window.clearTimeout(id);
  }, [statusFilter, pageSize, cursor, cursorStack.length]);

  const isLoading = jobs === undefined;

  const totalAll = countByStatus(jobs, 'all');
  const totalActive = countByStatus(jobs, 'active');
  const totalCompleted = countByStatus(jobs, 'completed');
  const totalFailed = countByStatus(jobs, 'failed');

  return (
    <PageContainer className="py-8 space-y-6">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">Background Tasks</h1>
        <p className="text-sm text-muted-foreground">
          Monitor AI generation jobs, cancel stuck runs, and inspect failures.
        </p>
      </div>

      <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <Tabs
          value={statusFilter}
          onValueChange={(value) => setStatusFilter(value as StatusFilter)}
          className="md:w-auto"
        >
          <TabsList>
            <TabsTrigger value="all">
              All
              {totalAll > 0 && (
                <span className="ml-2 text-xs text-muted-foreground">({totalAll})</span>
              )}
            </TabsTrigger>
            <TabsTrigger value="active">
              Active
              {totalActive > 0 && (
                <span className="ml-2 text-xs text-muted-foreground">({totalActive})</span>
              )}
            </TabsTrigger>
            <TabsTrigger value="completed">
              Completed
              {totalCompleted > 0 && (
                <span className="ml-2 text-xs text-muted-foreground">({totalCompleted})</span>
              )}
            </TabsTrigger>
            <TabsTrigger value="failed">
              Failed
              {totalFailed > 0 && (
                <span className="ml-2 text-xs text-muted-foreground">({totalFailed})</span>
              )}
            </TabsTrigger>
          </TabsList>
        </Tabs>

        <div className="flex items-center gap-2 md:gap-3">
          <div className="flex flex-col gap-1">
            <span className="text-xs text-muted-foreground">Page size</span>
            <Select value={String(pageSize)} onValueChange={(value) => setPageSize(Number(value))}>
              <SelectTrigger className="w-[120px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[10, 25, 50].map((size) => (
                  <SelectItem key={size} value={String(size)}>
                    {size} / page
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      </div>

      {isLoading ? (
        <div className="flex min-h-[200px] items-center justify-center rounded-lg border border-dashed">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading tasks…
          </div>
        </div>
      ) : !filteredJobs || filteredJobs.length === 0 ? (
        <div className="flex min-h-[200px] items-center justify-center rounded-lg border border-dashed">
          <p className="text-sm text-muted-foreground">
            {statusFilter === 'all'
              ? 'No background tasks yet. Generate concepts to see jobs here.'
              : `No ${statusFilter} tasks on this page.`}
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {filteredJobs.map((job) => (
            <GenerationTaskCard key={job._id} job={job} />
          ))}
        </div>
      )}

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-xs text-muted-foreground">
          {jobs && jobs.length > 0
            ? `Showing ${filteredJobs?.length ?? 0} of ${jobs.length} tasks on this page.`
            : 'No tasks to display.'}
        </p>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handlePrevPage}
            disabled={cursorStack.length === 0}
          >
            Previous
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={handleNextPage}
            disabled={!continueCursor || isDone}
          >
            Next
          </Button>
        </div>
      </div>
    </PageContainer>
  );
}
