#!/usr/bin/env npx tsx
/**
 * Comparative Prompt Evaluation
 *
 * Runs production baseline vs. single-variable mutation side-by-side.
 * Scientific approach: change ONE thing, measure the delta.
 *
 * Usage:
 *   npx tsx scripts/compare-prompts.ts --target concept-synthesis
 *   npx tsx scripts/compare-prompts.ts --target intent-extraction --variable model --value gemini-3-flash
 *   npx tsx scripts/compare-prompts.ts --target phrasing-generation --variable prompt --strategy simplification
 */
import { execSync } from 'child_process';
import { existsSync, readFileSync, writeFileSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';
import { EvalResult, evaluatePrompt } from './lib/evaluate';
import { logExperiment } from './lib/experiment-log';
import { mutatePrompt } from './lib/mutation';

// ============================================================================
// Types
// ============================================================================

type VariableType = 'model' | 'temperature' | 'prompt' | 'thinking_budget';

interface ComparisonConfig {
  /** Which prompt to test */
  target: 'concept-synthesis' | 'intent-extraction' | 'phrasing-generation';
  /** Which variable to mutate */
  variable: VariableType;
  /** Specific value for non-prompt variables */
  value?: string;
  /** Mutation strategy for prompt changes */
  strategy?: string;
  /** Path to promptfoo config */
  evalConfig: string;
  /** Path to prompts directory */
  promptDir: string;
  /** Whether to log experiment */
  logExperiment: boolean;
  /** Output format */
  format: 'json' | 'markdown' | 'both';
}

interface ComparisonResult {
  target: string;
  variable: VariableType;
  strategy?: string;
  baseline: EvalResult;
  variant: EvalResult;
  variantPrompt?: string;
  variantConfig?: string;
  delta: {
    passRate: number;
    llmScore: number;
    latencyP95: number;
  };
  winner: 'baseline' | 'variant' | 'tie';
  recommendation: string;
}

const DEFAULT_CONFIG: ComparisonConfig = {
  target: 'concept-synthesis',
  variable: 'prompt',
  evalConfig: 'evals/promptfoo.yaml',
  promptDir: 'evals/prompts',
  logExperiment: true,
  format: 'both',
};

// Mutation strategies available (from mutation.ts)
// Used for --help documentation and future strategy selection
const _MUTATION_STRATEGIES = [
  'restructure',
  'tone_shift',
  'constraint_emphasis',
  'example_injection',
  'role_framing',
  'chain_of_thought',
  'negative_constraints',
  'simplification',
  'specificity',
];

// Model alternatives for A/B testing
const MODEL_ALTERNATIVES = [
  'google/gemini-3-flash',
  'google/gemini-3-pro-preview',
  'anthropic/claude-sonnet-4.5',
  'openai/gpt-4o',
];

// Temperature variations
const TEMPERATURE_DELTAS = [-0.1, +0.1, -0.2, +0.2];

// Thinking budget variations
const THINKING_BUDGET_OPTIONS = [4096, 8192, 16384, 32768];

// ============================================================================
// Core Logic
// ============================================================================

/**
 * Get current production prompt from file.
 */
function getProductionPrompt(target: string, promptDir: string): string {
  const path = join(promptDir, `${target}.txt`);
  if (!existsSync(path)) {
    throw new Error(`Prompt not found: ${path}`);
  }
  return readFileSync(path, 'utf8');
}

/**
 * Create a mutated promptfoo config for model/temperature/thinking_budget changes.
 */
function createVariantConfig(
  baseConfigPath: string,
  variable: VariableType,
  value: string
): string {
  const baseConfig = readFileSync(baseConfigPath, 'utf8');
  let variantConfig = baseConfig;

  switch (variable) {
    case 'model':
      // Replace the provider ID
      variantConfig = baseConfig.replace(/- id:\s*openrouter:[^\n]+/, `- id: openrouter:${value}`);
      break;
    case 'temperature':
      // Replace temperature value
      variantConfig = baseConfig.replace(/temperature:\s*[\d.]+/, `temperature: ${value}`);
      break;
    case 'thinking_budget':
      // This would need to be handled differently - thinking config is in production code
      console.warn('thinking_budget changes require modifying production config');
      break;
  }

  return variantConfig;
}

/**
 * Run comparative evaluation.
 */
async function runComparison(config: ComparisonConfig): Promise<ComparisonResult> {
  console.log('🔬 Comparative Prompt Evaluation');
  console.log('================================\n');

  const productionPrompt = getProductionPrompt(config.target, config.promptDir);
  let variantPrompt = productionPrompt;
  let variantConfigPath = config.evalConfig;
  let variantDescription = '';

  // Generate variant based on variable type
  switch (config.variable) {
    case 'prompt': {
      console.log(`📝 Generating prompt mutation (strategy: ${config.strategy || 'random'})...`);
      const variants = await mutatePrompt(productionPrompt, { count: 1 });
      variantPrompt = variants[0] || productionPrompt;
      variantDescription = config.strategy || 'random mutation';
      break;
    }
    case 'model': {
      const newModel =
        config.value || MODEL_ALTERNATIVES[Math.floor(Math.random() * MODEL_ALTERNATIVES.length)];
      console.log(`🤖 Testing model variant: ${newModel}`);
      const variantConfig = createVariantConfig(config.evalConfig, 'model', newModel);
      variantConfigPath = join(tmpdir(), 'variant-config.yaml');
      writeFileSync(variantConfigPath, variantConfig);
      variantDescription = `model: ${newModel}`;
      break;
    }
    case 'temperature': {
      const delta = config.value
        ? parseFloat(config.value)
        : TEMPERATURE_DELTAS[Math.floor(Math.random() * TEMPERATURE_DELTAS.length)];
      const currentTemp = 0.4; // From promptfoo.yaml
      const newTemp = Math.max(0, Math.min(1, currentTemp + delta));
      console.log(`🌡️ Testing temperature variant: ${newTemp}`);
      const variantConfig = createVariantConfig(config.evalConfig, 'temperature', String(newTemp));
      variantConfigPath = join(tmpdir(), 'variant-config.yaml');
      writeFileSync(variantConfigPath, variantConfig);
      variantDescription = `temperature: ${newTemp}`;
      break;
    }
    case 'thinking_budget': {
      const budget = config.value
        ? parseInt(config.value, 10)
        : THINKING_BUDGET_OPTIONS[Math.floor(Math.random() * THINKING_BUDGET_OPTIONS.length)];
      console.log(`🧠 Testing thinking budget: ${budget}`);
      variantDescription = `thinking_budget: ${budget}`;
      // Note: This requires runtime config change, not just promptfoo config
      console.warn('⚠️  thinking_budget changes affect production code, not just prompts');
      break;
    }
  }

  // Run baseline evaluation
  console.log('\n📊 Evaluating baseline (production)...');
  const baseline = await evaluatePrompt(productionPrompt, 'baseline', config.evalConfig);
  console.log(`   Pass rate: ${(baseline.passRate * 100).toFixed(1)}%`);
  console.log(`   LLM score: ${baseline.avgLlmScore.toFixed(2)}/5`);
  console.log(`   Latency p95: ${baseline.latencyP95}ms`);

  // Run variant evaluation
  console.log('\n📊 Evaluating variant...');
  const variant = await evaluatePrompt(variantPrompt, 'variant', variantConfigPath);
  console.log(`   Pass rate: ${(variant.passRate * 100).toFixed(1)}%`);
  console.log(`   LLM score: ${variant.avgLlmScore.toFixed(2)}/5`);
  console.log(`   Latency p95: ${variant.latencyP95}ms`);

  // Calculate delta
  const delta = {
    passRate: variant.passRate - baseline.passRate,
    llmScore: variant.avgLlmScore - baseline.avgLlmScore,
    latencyP95: variant.latencyP95 - baseline.latencyP95,
  };

  // Determine winner (weighted fitness)
  const baselineFitness = baseline.passRate * 0.4 + (baseline.avgLlmScore / 5) * 0.6;
  const variantFitness = variant.passRate * 0.4 + (variant.avgLlmScore / 5) * 0.6;

  let winner: 'baseline' | 'variant' | 'tie';
  if (Math.abs(variantFitness - baselineFitness) < 0.01) {
    winner = 'tie';
  } else if (variantFitness > baselineFitness) {
    winner = 'variant';
  } else {
    winner = 'baseline';
  }

  // Generate recommendation
  let recommendation: string;
  if (winner === 'variant' && delta.passRate >= 0.02) {
    recommendation = '✅ Merge: Variant shows significant improvement';
  } else if (winner === 'variant') {
    recommendation = '🤔 Consider: Variant is slightly better, but run more tests';
  } else if (winner === 'tie') {
    recommendation = '🤝 Tie: No significant difference, prefer simpler option';
  } else {
    recommendation = '❌ Reject: Baseline performs better';
  }

  const result: ComparisonResult = {
    target: config.target,
    variable: config.variable,
    strategy: config.strategy,
    baseline,
    variant,
    variantPrompt: config.variable === 'prompt' ? variantPrompt : undefined,
    variantConfig: variantDescription,
    delta,
    winner,
    recommendation,
  };

  // Log experiment if requested
  if (config.logExperiment) {
    const gitSha = getGitSha();
    logExperiment({
      variable: config.variable,
      strategy: config.strategy,
      target: config.target,
      baseline: {
        passRate: baseline.passRate,
        llmScore: baseline.avgLlmScore,
        latencyP95: baseline.latencyP95,
      },
      variant: {
        passRate: variant.passRate,
        llmScore: variant.avgLlmScore,
        latencyP95: variant.latencyP95,
      },
      commitSha: gitSha,
      variantDescription,
    });
    console.log(`\n📝 Logged experiment to experiments/log.jsonl`);
  }

  return result;
}

/**
 * Get current git SHA.
 */
function getGitSha(): string | undefined {
  try {
    return execSync('git rev-parse HEAD', { encoding: 'utf8' }).trim();
  } catch {
    return undefined;
  }
}

/**
 * Format result as markdown for PR comments.
 */
function formatAsMarkdown(result: ComparisonResult): string {
  const emoji = result.winner === 'variant' ? '🎉' : result.winner === 'tie' ? '🤝' : '📊';
  const deltaPassStr =
    result.delta.passRate >= 0
      ? `**+${(result.delta.passRate * 100).toFixed(1)}%** ✅`
      : `${(result.delta.passRate * 100).toFixed(1)}%`;
  const deltaLlmStr =
    result.delta.llmScore >= 0
      ? `**+${result.delta.llmScore.toFixed(2)}** ✅`
      : `${result.delta.llmScore.toFixed(2)}`;
  const deltaLatencyStr =
    result.delta.latencyP95 <= 0
      ? `**${result.delta.latencyP95}ms** ✅`
      : `+${result.delta.latencyP95}ms`;

  return `## ${emoji} Prompt Comparison Results

**Target:** \`${result.target}\`
**Variable:** ${result.variable}${result.strategy ? ` (${result.strategy})` : ''}
${result.variantConfig ? `**Variant:** ${result.variantConfig}` : ''}

### Baseline (Production)
| Metric | Value |
|--------|-------|
| Pass Rate | ${(result.baseline.passRate * 100).toFixed(1)}% |
| LLM Score | ${result.baseline.avgLlmScore.toFixed(2)}/5 |
| Latency p95 | ${result.baseline.latencyP95}ms |

### Variant
| Metric | Value | Delta |
|--------|-------|-------|
| Pass Rate | ${(result.variant.passRate * 100).toFixed(1)}% | ${deltaPassStr} |
| LLM Score | ${result.variant.avgLlmScore.toFixed(2)}/5 | ${deltaLlmStr} |
| Latency p95 | ${result.variant.latencyP95}ms | ${deltaLatencyStr} |

### Recommendation
${result.recommendation}

---
*Generated by [compare-prompts.ts](/scripts/compare-prompts.ts)*`;
}

// ============================================================================
// CLI
// ============================================================================

function parseArgs(): Partial<ComparisonConfig> {
  const args = process.argv.slice(2);
  const config: Partial<ComparisonConfig> = {};

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    const next = args[i + 1];

    switch (arg) {
      case '--target':
      case '-t':
        config.target = next as ComparisonConfig['target'];
        i++;
        break;
      case '--variable':
      case '-v':
        config.variable = next as VariableType;
        i++;
        break;
      case '--value':
        config.value = next;
        i++;
        break;
      case '--strategy':
      case '-s':
        config.strategy = next;
        i++;
        break;
      case '--no-log':
        config.logExperiment = false;
        break;
      case '--format':
      case '-f':
        config.format = next as ComparisonConfig['format'];
        i++;
        break;
      case '--help':
      case '-h':
        console.log(`
Comparative Prompt Evaluation

Usage:
  npx tsx scripts/compare-prompts.ts [options]

Options:
  --target, -t <name>     Prompt to test (concept-synthesis, intent-extraction, phrasing-generation)
  --variable, -v <type>   Variable to mutate (model, temperature, prompt, thinking_budget)
  --value <val>           Specific value for model/temperature changes
  --strategy, -s <name>   Mutation strategy for prompt changes
  --no-log                Don't log experiment to experiments/log.jsonl
  --format, -f <fmt>      Output format (json, markdown, both)
  --help, -h              Show this help

Examples:
  # Random prompt mutation
  npx tsx scripts/compare-prompts.ts --target concept-synthesis

  # Specific model comparison
  npx tsx scripts/compare-prompts.ts --target intent-extraction --variable model --value google/gemini-3-flash

  # Temperature tweak
  npx tsx scripts/compare-prompts.ts --target phrasing-generation --variable temperature --value 0.5

  # Specific mutation strategy
  npx tsx scripts/compare-prompts.ts --target concept-synthesis --variable prompt --strategy simplification
`);
        process.exit(0);
    }
  }

  return config;
}

async function main() {
  const cliConfig = parseArgs();
  const config: ComparisonConfig = { ...DEFAULT_CONFIG, ...cliConfig };

  try {
    const result = await runComparison(config);

    console.log('\n' + '='.repeat(60) + '\n');

    if (config.format === 'json' || config.format === 'both') {
      console.log('📋 JSON Output:');
      console.log(JSON.stringify(result, null, 2));
    }

    if (config.format === 'markdown' || config.format === 'both') {
      console.log('\n📝 Markdown Output:');
      console.log(formatAsMarkdown(result));
    }

    // Write markdown to file for CI use
    const resultPath = join(tmpdir(), 'comparison-result.md');
    writeFileSync(resultPath, formatAsMarkdown(result));
    console.log(`\n✅ Markdown saved to ${resultPath}`);
  } catch (error) {
    console.error('\n❌ Comparison failed:', error);
    process.exit(1);
  }
}

main();
