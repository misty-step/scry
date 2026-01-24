'use client';

/**
 * Experiment Card - Expandable card showing evolution results
 */
import { useState } from 'react';
import {
  CalendarIcon,
  ChevronDownIcon,
  ChevronUpIcon,
  CopyIcon,
  DownloadIcon,
  FlaskConicalIcon,
  TrendingUpIcon,
} from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import type { EvolutionHistory, ExperimentSummary, GenerationRecord } from '@/types/evolve';
import { PromptDiff } from './prompt-diff';
import { RunConfigPanel } from './run-config-panel';
import { TestResultsTable } from './test-results-table';
import { VariantsSelector } from './variants-selector';

interface ExperimentCardProps {
  experiment: ExperimentSummary;
}

export function ExperimentCard({ experiment }: ExperimentCardProps) {
  const [expanded, setExpanded] = useState(false);
  const [history, setHistory] = useState<EvolutionHistory | null>(null);
  const [loading, setLoading] = useState(false);
  const [selectedGeneration, setSelectedGeneration] = useState<number | null>(null);
  const [selectedVariantId, setSelectedVariantId] = useState<string | null>(null);

  const fetchHistory = async () => {
    if (history) return; // Already loaded
    setLoading(true);
    try {
      const res = await fetch(`/api/evolve/experiments/${experiment.id}`);
      if (!res.ok) throw new Error('Failed to fetch history');
      const data = await res.json();
      setHistory(data.history);
    } catch {
      toast.error('Failed to load experiment history');
    } finally {
      setLoading(false);
    }
  };

  const handleExpand = () => {
    if (!expanded) {
      fetchHistory();
    }
    setExpanded(!expanded);
  };

  const copyPrompt = async () => {
    if (!history?.evolvedPrompt) return;
    await navigator.clipboard.writeText(history.evolvedPrompt);
    toast.success('Copied to clipboard');
  };

  const downloadPrompt = () => {
    if (!history?.evolvedPrompt) return;
    const blob = new Blob([history.evolvedPrompt], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${experiment.promptName}-evolved.txt`;
    a.click();
    URL.revokeObjectURL(url);
  };

  // Format date nicely
  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr);
    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    });
  };

  // Fitness badge color
  const fitnessColor =
    experiment.fitness >= 0.9
      ? 'bg-green-500/10 text-green-600 border-green-200'
      : experiment.fitness >= 0.7
        ? 'bg-yellow-500/10 text-yellow-600 border-yellow-200'
        : 'bg-red-500/10 text-red-600 border-red-200';

  return (
    <Card className="overflow-hidden">
      {/* Header - Always visible */}
      <CardHeader
        className="cursor-pointer hover:bg-muted/50 transition-colors"
        onClick={handleExpand}
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <h3 className="text-lg font-semibold">{experiment.promptName}</h3>
            <Badge variant="outline" className={fitnessColor}>
              {(experiment.fitness * 100).toFixed(0)}%
            </Badge>
          </div>
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
              <CalendarIcon className="h-4 w-4" />
              {formatDate(experiment.date)}
            </div>
            {expanded ? (
              <ChevronUpIcon className="h-5 w-5 text-muted-foreground" />
            ) : (
              <ChevronDownIcon className="h-5 w-5 text-muted-foreground" />
            )}
          </div>
        </div>

        {/* Quick stats */}
        <div className="flex gap-6 mt-2 text-sm">
          <div>
            <span className="text-muted-foreground">Pass Rate:</span>{' '}
            <span className="font-medium">{(experiment.passRate * 100).toFixed(0)}%</span>
          </div>
          <div>
            <span className="text-muted-foreground">LLM Score:</span>{' '}
            <span className="font-medium">{experiment.avgLlmScore.toFixed(1)}/5</span>
          </div>
          <div>
            <span className="text-muted-foreground">Generation:</span>{' '}
            <span className="font-medium">{experiment.generation}</span>
          </div>
          {experiment.hasHistory && (
            <Badge variant="secondary" className="text-xs">
              Full History
            </Badge>
          )}
        </div>
      </CardHeader>

      {/* Expanded content */}
      {expanded && (
        <CardContent className="pt-0 border-t">
          {loading && <div className="py-8 text-center text-muted-foreground">Loading...</div>}

          {!loading && history && (
            <div className="space-y-6 pt-4">
              {/* Run Configuration */}
              <RunConfigPanel
                config={history.config}
                pipelineConfig={history.pipelineConfig}
                startedAt={history.startedAt}
                completedAt={history.completedAt}
              />

              {/* Generation History */}
              {history.generations && history.generations.length > 0 && (
                <div>
                  <h4 className="font-medium flex items-center gap-2 mb-3">
                    <TrendingUpIcon className="h-4 w-4" />
                    Generation History
                    <span className="text-xs text-muted-foreground font-normal">
                      (click to view details)
                    </span>
                  </h4>
                  <div className="space-y-2">
                    {history.generations.map((gen: GenerationRecord) => {
                      const isBest = gen.generation === history.finalGeneration;
                      const isSelected = selectedGeneration === gen.generation;

                      return (
                        <div
                          key={gen.generation}
                          onClick={() => {
                            if (isSelected) {
                              setSelectedGeneration(null);
                              setSelectedVariantId(null);
                            } else {
                              setSelectedGeneration(gen.generation);
                              // Auto-select the best variant or first one
                              const firstVariant = gen.variants?.[0];
                              setSelectedVariantId(firstVariant?.variantId || null);
                            }
                          }}
                          className={cn(
                            'flex items-center gap-4 text-sm p-2 rounded cursor-pointer transition-colors',
                            isSelected
                              ? 'bg-primary/10 border-2 border-primary/40'
                              : isBest
                                ? 'bg-primary/5 border border-primary/20 hover:bg-primary/10'
                                : 'bg-muted/50 hover:bg-muted'
                          )}
                        >
                          <span className="font-mono w-16">Gen {gen.generation}</span>
                          <span className="text-muted-foreground">
                            fitness={gen.fitness.toFixed(3)}
                          </span>
                          <span className="text-muted-foreground">
                            pass={Math.round(gen.passRate * 100)}%
                          </span>
                          <span className="text-muted-foreground">
                            llm={gen.llmScore.toFixed(1)}
                          </span>
                          {gen.variants && gen.variants.length > 0 && (
                            <Badge variant="secondary" className="text-xs">
                              {gen.variants.length} variants
                            </Badge>
                          )}
                          {isBest && (
                            <Badge variant="outline" className="ml-auto text-xs">
                              Best
                            </Badge>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>
              )}

              {/* Selected Generation Details */}
              {selectedGeneration !== null &&
                (() => {
                  const gen = history.generations.find((g) => g.generation === selectedGeneration);
                  if (!gen) return null;

                  const variants = gen.variants || [];
                  const selectedVariant =
                    variants.find((v) => v.variantId === selectedVariantId) || variants[0];

                  return (
                    <div className="space-y-4 border-t pt-4">
                      <h4 className="font-medium flex items-center gap-2">
                        <FlaskConicalIcon className="h-4 w-4" />
                        Generation {selectedGeneration} Details
                      </h4>

                      {/* Variant Selector */}
                      {variants.length > 0 && (
                        <VariantsSelector
                          variants={variants}
                          selectedVariantId={selectedVariantId || variants[0]?.variantId}
                          onSelectVariant={setSelectedVariantId}
                          bestVariantId={
                            variants.reduce(
                              (best, v) => (v.fitness > (best?.fitness || 0) ? v : best),
                              variants[0]
                            )?.variantId
                          }
                        />
                      )}

                      {/* Test Results for Selected Variant */}
                      {selectedVariant?.testResults && selectedVariant.testResults.length > 0 && (
                        <div className="space-y-2">
                          <p className="text-sm text-muted-foreground">
                            Test Results for {selectedVariant.variantId}:
                          </p>
                          <TestResultsTable testResults={selectedVariant.testResults} />
                        </div>
                      )}

                      {/* Fallback to generation-level test results */}
                      {!selectedVariant?.testResults &&
                        gen.testResults &&
                        gen.testResults.length > 0 && (
                          <div className="space-y-2">
                            <p className="text-sm text-muted-foreground">
                              Test Results (best variant):
                            </p>
                            <TestResultsTable testResults={gen.testResults} />
                          </div>
                        )}

                      {/* Show prompt for selected variant */}
                      {selectedVariant?.prompt && (
                        <div className="space-y-2">
                          <p className="text-sm text-muted-foreground">Prompt:</p>
                          <pre className="text-xs font-mono bg-muted/50 p-3 rounded max-h-40 overflow-y-auto whitespace-pre-wrap">
                            {selectedVariant.prompt.length > 500
                              ? selectedVariant.prompt.slice(0, 500) + '...'
                              : selectedVariant.prompt}
                          </pre>
                        </div>
                      )}
                    </div>
                  );
                })()}

              {/* Prompt Diff */}
              {history.seedPrompt && history.evolvedPrompt && (
                <PromptDiff seedPrompt={history.seedPrompt} evolvedPrompt={history.evolvedPrompt} />
              )}

              {/* Actions */}
              <div className="flex gap-2 pt-2">
                <Button variant="outline" size="sm" onClick={copyPrompt}>
                  <CopyIcon className="h-4 w-4 mr-2" />
                  Copy Evolved Prompt
                </Button>
                <Button variant="outline" size="sm" onClick={downloadPrompt}>
                  <DownloadIcon className="h-4 w-4 mr-2" />
                  Download
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      )}
    </Card>
  );
}
