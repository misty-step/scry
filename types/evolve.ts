/**
 * Types for Prompt Evolution System
 */

/**
 * Pipeline configuration from promptfoo.yaml
 */
export interface PipelineConfig {
  provider: string;
  temperature: number;
  testCount: number;
  maxConcurrency: number;
  delay: number;
}

/**
 * Evolution run configuration
 */
export interface EvolutionConfig {
  promptName: string;
  populationSize: number;
  selectTopK: number;
  maxGenerations: number;
  convergenceThreshold: number;
  outputDir: string;
  seedDir: string;
  evalConfig: string;
}

/**
 * Per-test result with full details including judge reasoning
 */
export interface TestResultRecord {
  description: string;
  passed: boolean;
  score?: number;
  latencyMs?: number;
  error?: string;
  judgeReason?: string;
  assertionType?: string;
}

/**
 * A single evaluated variant with its full test results
 */
export interface VariantRecord {
  variantId: string;
  fitness: number;
  passRate: number;
  llmScore: number;
  prompt: string;
  testResults: TestResultRecord[];
}

/**
 * Record of a single generation's best performer
 */
export interface GenerationRecord {
  generation: number;
  fitness: number;
  passRate: number;
  llmScore: number;
  bestPrompt: string;
  populationSize: number;
  timestamp: string;
  /** Per-test details for best variant */
  testResults?: TestResultRecord[];
  /** All evaluated variants with full details */
  variants?: VariantRecord[];
}

/**
 * Full evolution history for a single experiment
 */
export interface EvolutionHistory {
  promptName: string;
  seedPrompt: string;
  evolvedPrompt: string;
  startedAt: string;
  completedAt: string;
  finalGeneration: number;
  generations: GenerationRecord[];
  config: EvolutionConfig;
  pipelineConfig: PipelineConfig;
}

/**
 * Summary of an experiment for list view
 */
export interface ExperimentSummary {
  id: string;
  promptName: string;
  date: string;
  fitness: number;
  passRate: number;
  avgLlmScore: number;
  generation: number;
  hasHistory: boolean;
}

/**
 * API response for /api/evolve/experiments
 */
export interface ExperimentsResponse {
  experiments: ExperimentSummary[];
  count: number;
}
