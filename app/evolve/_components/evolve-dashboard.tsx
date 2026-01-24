'use client';

/**
 * Evolve Dashboard - Prompt Evolution Results Viewer
 *
 * Read-only UI for viewing prompt evolution experiment results.
 * Data comes from artifacts/optimized-prompts/ via API.
 */
import { useEffect, useState } from 'react';
import { BeakerIcon, RefreshCwIcon, TerminalIcon } from 'lucide-react';
import { PageContainer } from '@/components/page-container';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import type { ExperimentsResponse, ExperimentSummary } from '@/types/evolve';
import { ExperimentCard } from './experiment-card';

export function EvolveDashboard() {
  const [experiments, setExperiments] = useState<ExperimentSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchExperiments = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch('/api/evolve/experiments');
      if (!res.ok) throw new Error('Failed to fetch experiments');
      const data: ExperimentsResponse = await res.json();
      setExperiments(data.experiments);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchExperiments();
  }, []);

  return (
    <PageContainer className="max-w-5xl mx-auto py-8 px-4">
      {/* Header */}
      <div className="mb-8">
        <div className="flex items-center gap-3 mb-2">
          <BeakerIcon className="h-8 w-8 text-primary" />
          <h1 className="text-3xl font-bold">Prompt Evolution Lab</h1>
        </div>
        <p className="text-muted-foreground mb-4">
          View and analyze prompt evolution experiment results.
        </p>

        {/* CLI Hint */}
        <Card className="bg-muted/50 border-dashed">
          <CardContent className="flex items-center gap-3 py-3">
            <TerminalIcon className="h-5 w-5 text-muted-foreground" />
            <code className="text-sm">pnpm evolve --help</code>
            <span className="text-sm text-muted-foreground">— Run experiments from CLI</span>
          </CardContent>
        </Card>
      </div>

      {/* Toolbar */}
      <div className="flex justify-between items-center mb-6">
        <h2 className="text-xl font-semibold">
          Experiments
          {!loading && (
            <span className="text-muted-foreground font-normal ml-2">({experiments.length})</span>
          )}
        </h2>
        <Button variant="outline" size="sm" onClick={fetchExperiments} disabled={loading}>
          <RefreshCwIcon className={`h-4 w-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {/* Content */}
      {loading && (
        <div className="flex items-center justify-center py-12">
          <RefreshCwIcon className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {error && (
        <Card className="border-destructive">
          <CardHeader>
            <CardTitle className="text-destructive">Error</CardTitle>
          </CardHeader>
          <CardContent>
            <p>{error}</p>
          </CardContent>
        </Card>
      )}

      {!loading && !error && experiments.length === 0 && (
        <Card>
          <CardContent className="py-12 text-center">
            <BeakerIcon className="h-12 w-12 mx-auto text-muted-foreground mb-4" />
            <h3 className="text-lg font-medium mb-2">No experiments yet</h3>
            <p className="text-muted-foreground mb-4">
              Run your first experiment from the command line.
            </p>
            <code className="bg-muted px-3 py-1 rounded text-sm">
              pnpm evolve --prompt concept-synthesis
            </code>
          </CardContent>
        </Card>
      )}

      {!loading && !error && experiments.length > 0 && (
        <div className="space-y-4">
          {experiments.map((exp) => (
            <ExperimentCard key={exp.id} experiment={exp} />
          ))}
        </div>
      )}
    </PageContainer>
  );
}
