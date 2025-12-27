'use client';

/**
 * Run Config Panel - Displays evolution configuration and pipeline settings
 */
import { ClockIcon, CpuIcon, SettingsIcon, ThermometerIcon } from 'lucide-react';
import type { EvolutionConfig, PipelineConfig } from '@/types/evolve';

interface RunConfigPanelProps {
  config?: EvolutionConfig;
  pipelineConfig?: PipelineConfig;
  startedAt?: string;
  completedAt?: string;
}

export function RunConfigPanel({
  config,
  pipelineConfig,
  startedAt,
  completedAt,
}: RunConfigPanelProps) {
  const formatDuration = (start?: string, end?: string) => {
    if (!start || !end) return null;
    const durationMs = new Date(end).getTime() - new Date(start).getTime();
    const seconds = Math.floor(durationMs / 1000);
    const minutes = Math.floor(seconds / 60);
    const remainingSeconds = seconds % 60;
    if (minutes > 0) {
      return `${minutes}m ${remainingSeconds}s`;
    }
    return `${seconds}s`;
  };

  const formatTimestamp = (ts?: string) => {
    if (!ts) return null;
    const date = new Date(ts);
    return date.toLocaleString('en-US', {
      month: 'short',
      day: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
      hour12: true,
    });
  };

  const duration = formatDuration(startedAt, completedAt);

  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-4 p-3 bg-muted/30 rounded-lg text-sm">
      {/* Evolution Config */}
      {config && (
        <>
          <div className="space-y-1">
            <div className="flex items-center gap-1.5 text-muted-foreground">
              <SettingsIcon className="h-3.5 w-3.5" />
              <span className="text-xs">Evolution</span>
            </div>
            <div className="font-medium">
              pop={config.populationSize}, top-k={config.selectTopK}
            </div>
            <div className="text-xs text-muted-foreground">
              max {config.maxGenerations} gen, converge@{config.convergenceThreshold}
            </div>
          </div>
        </>
      )}

      {/* Pipeline Config */}
      {pipelineConfig && (
        <>
          <div className="space-y-1">
            <div className="flex items-center gap-1.5 text-muted-foreground">
              <CpuIcon className="h-3.5 w-3.5" />
              <span className="text-xs">Provider</span>
            </div>
            <div className="font-medium font-mono text-xs truncate" title={pipelineConfig.provider}>
              {pipelineConfig.provider.split('/').pop()}
            </div>
            <div className="text-xs text-muted-foreground">
              {pipelineConfig.testCount} tests, {pipelineConfig.maxConcurrency}x concurrent
            </div>
          </div>

          <div className="space-y-1">
            <div className="flex items-center gap-1.5 text-muted-foreground">
              <ThermometerIcon className="h-3.5 w-3.5" />
              <span className="text-xs">Temperature</span>
            </div>
            <div className="font-medium">{pipelineConfig.temperature}</div>
            <div className="text-xs text-muted-foreground">{pipelineConfig.delay}ms delay</div>
          </div>
        </>
      )}

      {/* Duration */}
      <div className="space-y-1">
        <div className="flex items-center gap-1.5 text-muted-foreground">
          <ClockIcon className="h-3.5 w-3.5" />
          <span className="text-xs">Duration</span>
        </div>
        {duration ? (
          <>
            <div className="font-medium">{duration}</div>
            <div className="text-xs text-muted-foreground">{formatTimestamp(startedAt)}</div>
          </>
        ) : (
          <div className="text-muted-foreground">-</div>
        )}
      </div>
    </div>
  );
}
