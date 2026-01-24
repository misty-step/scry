/**
 * Experiment Tracking
 *
 * Append-only log for tracking LLM prompt experiments.
 * Each experiment records: what was changed, baseline vs variant results, winner.
 */

import { appendFileSync, existsSync, mkdirSync, readFileSync } from 'fs';
import { dirname } from 'path';

export interface ExperimentMetrics {
  passRate: number;
  llmScore: number;
  latencyP95?: number;
  cost?: number;
}

export interface ExperimentRecord {
  timestamp: string;
  /** Which variable was modified (model, temperature, prompt, thinking_budget) */
  variable: 'model' | 'temperature' | 'prompt' | 'thinking_budget';
  /** Specific mutation strategy if prompt was changed */
  strategy?: string;
  /** Which prompt was targeted (concept-synthesis, intent-extraction, phrasing-generation) */
  target: string;
  /** Production/baseline metrics */
  baseline: ExperimentMetrics;
  /** Variant metrics */
  variant: ExperimentMetrics;
  /** Which performed better */
  winner: 'baseline' | 'variant' | 'tie';
  /** Delta (variant - baseline) */
  delta: {
    passRate: number;
    llmScore: number;
  };
  /** Git commit SHA */
  commitSha?: string;
  /** PR number if applicable */
  prNumber?: number;
  /** Variant description for human readability */
  variantDescription?: string;
}

const DEFAULT_LOG_PATH = 'experiments/log.jsonl';

/**
 * Append a new experiment record to the log.
 */
export function logExperiment(
  record: Omit<ExperimentRecord, 'timestamp' | 'winner' | 'delta'>,
  logPath = DEFAULT_LOG_PATH
): ExperimentRecord {
  // Calculate delta and winner
  const delta = {
    passRate: record.variant.passRate - record.baseline.passRate,
    llmScore: record.variant.llmScore - record.baseline.llmScore,
  };

  // Winner based on weighted score (same as fitness: 40% pass rate, 60% LLM score)
  const baselineFitness = record.baseline.passRate * 0.4 + (record.baseline.llmScore / 5) * 0.6;
  const variantFitness = record.variant.passRate * 0.4 + (record.variant.llmScore / 5) * 0.6;

  let winner: 'baseline' | 'variant' | 'tie';
  if (Math.abs(variantFitness - baselineFitness) < 0.01) {
    winner = 'tie';
  } else if (variantFitness > baselineFitness) {
    winner = 'variant';
  } else {
    winner = 'baseline';
  }

  const fullRecord: ExperimentRecord = {
    ...record,
    timestamp: new Date().toISOString(),
    winner,
    delta,
  };

  // Ensure directory exists
  const dir = dirname(logPath);
  if (!existsSync(dir)) {
    mkdirSync(dir, { recursive: true });
  }

  // Append as JSONL
  appendFileSync(logPath, JSON.stringify(fullRecord) + '\n');

  return fullRecord;
}

/**
 * Read all experiment records from the log.
 */
export function readExperiments(logPath = DEFAULT_LOG_PATH): ExperimentRecord[] {
  if (!existsSync(logPath)) {
    return [];
  }

  const content = readFileSync(logPath, 'utf8');
  return content
    .split('\n')
    .filter((line) => line.trim())
    .map((line) => JSON.parse(line) as ExperimentRecord);
}

/**
 * Get experiment statistics grouped by variable type.
 */
export function getExperimentStats(logPath = DEFAULT_LOG_PATH): {
  byVariable: Record<string, { wins: number; losses: number; ties: number }>;
  byStrategy: Record<string, { wins: number; losses: number; ties: number }>;
  overall: { total: number; variantWins: number; baselineWins: number; ties: number };
} {
  const experiments = readExperiments(logPath);

  const byVariable: Record<string, { wins: number; losses: number; ties: number }> = {};
  const byStrategy: Record<string, { wins: number; losses: number; ties: number }> = {};
  let variantWins = 0;
  let baselineWins = 0;
  let ties = 0;

  for (const exp of experiments) {
    // Track by variable
    if (!byVariable[exp.variable]) {
      byVariable[exp.variable] = { wins: 0, losses: 0, ties: 0 };
    }
    if (exp.winner === 'variant') {
      byVariable[exp.variable].wins++;
      variantWins++;
    } else if (exp.winner === 'baseline') {
      byVariable[exp.variable].losses++;
      baselineWins++;
    } else {
      byVariable[exp.variable].ties++;
      ties++;
    }

    // Track by strategy (if prompt mutation)
    if (exp.strategy) {
      if (!byStrategy[exp.strategy]) {
        byStrategy[exp.strategy] = { wins: 0, losses: 0, ties: 0 };
      }
      if (exp.winner === 'variant') {
        byStrategy[exp.strategy].wins++;
      } else if (exp.winner === 'baseline') {
        byStrategy[exp.strategy].losses++;
      } else {
        byStrategy[exp.strategy].ties++;
      }
    }
  }

  return {
    byVariable,
    byStrategy,
    overall: {
      total: experiments.length,
      variantWins,
      baselineWins,
      ties,
    },
  };
}

/**
 * Format experiment record for display (PR comment, CLI output).
 */
export function formatExperimentResult(record: ExperimentRecord): string {
  const emoji = record.winner === 'variant' ? '🎉' : record.winner === 'tie' ? '🤝' : '📊';
  const deltaPass =
    record.delta.passRate >= 0
      ? `+${(record.delta.passRate * 100).toFixed(1)}%`
      : `${(record.delta.passRate * 100).toFixed(1)}%`;
  const deltaLlm =
    record.delta.llmScore >= 0
      ? `+${record.delta.llmScore.toFixed(2)}`
      : `${record.delta.llmScore.toFixed(2)}`;

  return `${emoji} **${record.target}** (${record.variable}${record.strategy ? `: ${record.strategy}` : ''})
| Metric | Baseline | Variant | Delta |
|--------|----------|---------|-------|
| Pass Rate | ${(record.baseline.passRate * 100).toFixed(1)}% | ${(record.variant.passRate * 100).toFixed(1)}% | ${deltaPass} |
| LLM Score | ${record.baseline.llmScore.toFixed(2)}/5 | ${record.variant.llmScore.toFixed(2)}/5 | ${deltaLlm} |

**Winner:** ${record.winner}`;
}
