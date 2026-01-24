'use client';

/**
 * Test Results Table - Shows per-test evaluation details with expandable judge reasoning
 */
import { useState } from 'react';
import { CheckCircleIcon, ChevronDownIcon, ChevronRightIcon, XCircleIcon } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { cn } from '@/lib/utils';
import type { TestResultRecord } from '@/types/evolve';

interface TestResultsTableProps {
  testResults: TestResultRecord[];
}

export function TestResultsTable({ testResults }: TestResultsTableProps) {
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());

  const toggleRow = (index: number) => {
    setExpandedRows((prev) => {
      const next = new Set(prev);
      if (next.has(index)) {
        next.delete(index);
      } else {
        next.add(index);
      }
      return next;
    });
  };

  const formatLatency = (ms?: number) => {
    if (ms === undefined) return '-';
    if (ms < 1000) return `${Math.round(ms)}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  };

  const formatScore = (score?: number) => {
    if (score === undefined) return '-';
    return score.toFixed(1);
  };

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="w-8"></TableHead>
          <TableHead>Test</TableHead>
          <TableHead className="w-20">Status</TableHead>
          <TableHead className="w-16 text-right">Score</TableHead>
          <TableHead className="w-20 text-right">Latency</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {testResults.map((test, index) => {
          const hasReason = test.judgeReason || test.error;
          const isExpanded = expandedRows.has(index);

          return (
            <>
              <TableRow
                key={index}
                className={cn(hasReason && 'cursor-pointer')}
                onClick={() => hasReason && toggleRow(index)}
              >
                <TableCell className="p-1">
                  {hasReason &&
                    (isExpanded ? (
                      <ChevronDownIcon className="h-4 w-4 text-muted-foreground" />
                    ) : (
                      <ChevronRightIcon className="h-4 w-4 text-muted-foreground" />
                    ))}
                </TableCell>
                <TableCell className="font-medium">
                  {test.description}
                  {test.assertionType === 'llm-rubric' && (
                    <Badge variant="outline" className="ml-2 text-xs">
                      LLM Judge
                    </Badge>
                  )}
                </TableCell>
                <TableCell>
                  {test.passed ? (
                    <div className="flex items-center gap-1 text-green-600">
                      <CheckCircleIcon className="h-4 w-4" />
                      <span className="text-xs">Pass</span>
                    </div>
                  ) : (
                    <div className="flex items-center gap-1 text-red-600">
                      <XCircleIcon className="h-4 w-4" />
                      <span className="text-xs">Fail</span>
                    </div>
                  )}
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {formatScore(test.score)}
                </TableCell>
                <TableCell className="text-right font-mono text-sm text-muted-foreground">
                  {formatLatency(test.latencyMs)}
                </TableCell>
              </TableRow>
              {isExpanded && hasReason && (
                <TableRow key={`${index}-reason`} className="bg-muted/30">
                  <TableCell></TableCell>
                  <TableCell colSpan={4} className="py-3">
                    {test.judgeReason && (
                      <div className="space-y-1">
                        <p className="text-xs font-medium text-muted-foreground">
                          Judge Reasoning:
                        </p>
                        <pre className="whitespace-pre-wrap text-sm font-mono bg-muted/50 p-2 rounded max-h-48 overflow-y-auto">
                          {test.judgeReason}
                        </pre>
                      </div>
                    )}
                    {test.error && (
                      <div className="space-y-1 mt-2">
                        <p className="text-xs font-medium text-red-600">Error:</p>
                        <pre className="whitespace-pre-wrap text-sm font-mono bg-red-50 dark:bg-red-950/30 text-red-700 dark:text-red-400 p-2 rounded">
                          {test.error}
                        </pre>
                      </div>
                    )}
                  </TableCell>
                </TableRow>
              )}
            </>
          );
        })}
      </TableBody>
    </Table>
  );
}
