#!/usr/bin/env npx tsx
/**
 * Evolutionary Prompt Optimizer
 *
 * Runs an evolutionary loop to discover better prompts:
 * 1. Start with seed prompt from evals/prompts/
 * 2. Generate mutations via LLM meta-prompts
 * 3. Evaluate all variants with promptfoo
 * 4. Select top performers
 * 5. Repeat until convergence
 * 6. Output best prompt to artifacts/optimized-prompts/
 *
 * Usage:
 *   npx tsx scripts/evolve-prompts.ts
 *   npx tsx scripts/evolve-prompts.ts --prompt concept-synthesis --generations 5
 */
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'fs';
import { join } from 'path';
import { EvalResult, evaluatePrompt } from './lib/evaluate';
import { mutatePrompt } from './lib/mutation';

// ============================================================================
// Pipeline Config Display
// ============================================================================

interface PipelineConfig {
  provider: string;
  temperature: number;
  testCount: number;
  maxConcurrency: number;
  delay: number;
}

/**
 * Parse promptfoo.yaml to extract key pipeline configuration.
 */
function parsePipelineConfig(configPath: string): PipelineConfig {
  const content = readFileSync(configPath, 'utf8');

  // Extract provider ID (first provider listed)
  const providerMatch = content.match(/- id:\s*(.+)/);
  const provider = providerMatch?.[1]?.trim() || 'unknown';

  // Extract temperature
  const tempMatch = content.match(/temperature:\s*([\d.]+)/);
  const temperature = tempMatch ? parseFloat(tempMatch[1]) : 0.7;

  // Count test cases
  const testMatches = content.match(/- description:/g);
  const testCount = testMatches?.length || 0;

  // Extract concurrency settings
  const concurrencyMatch = content.match(/maxConcurrency:\s*(\d+)/);
  const maxConcurrency = concurrencyMatch ? parseInt(concurrencyMatch[1], 10) : 4;

  const delayMatch = content.match(/delay:\s*(\d+)/);
  const delay = delayMatch ? parseInt(delayMatch[1], 10) : 0;

  return { provider, temperature, testCount, maxConcurrency, delay };
}

/**
 * Display pipeline configuration banner.
 */
function displayPipelineConfig(config: PipelineConfig): void {
  console.log('📋 Pipeline Config:');
  console.log(`   Provider: ${config.provider}`);
  console.log(`   Temperature: ${config.temperature}`);
  console.log(`   Tests: ${config.testCount}`);
  console.log(`   Concurrency: ${config.maxConcurrency} (${config.delay}ms delay)`);
  console.log('');
}

// ============================================================================
// Configuration
// ============================================================================

interface Config {
  /** Which prompt to evolve (maps to evals/prompts/{name}.txt) */
  promptName: string;
  /** Number of variants per generation */
  populationSize: number;
  /** How many top performers to keep each generation */
  selectTopK: number;
  /** Maximum generations before stopping */
  maxGenerations: number;
  /** Stop if no improvement for this many generations */
  convergenceThreshold: number;
  /** Output directory for evolved prompts */
  outputDir: string;
  /** Seed prompt directory */
  seedDir: string;
  /** Promptfoo config path */
  evalConfig: string;
}

const DEFAULT_CONFIG: Config = {
  promptName: 'concept-synthesis',
  populationSize: 5,
  selectTopK: 2,
  maxGenerations: 10,
  convergenceThreshold: 3,
  outputDir: 'artifacts/optimized-prompts',
  seedDir: 'evals/prompts',
  evalConfig: 'evals/promptfoo.yaml',
};

// ============================================================================
// Types
// ============================================================================

interface Individual {
  prompt: string;
  fitness: number;
  result: EvalResult;
  generation: number;
  parentId?: string;
}

/**
 * Per-test result with judge reasoning
 */
interface TestResultRecord {
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
interface VariantRecord {
  variantId: string;
  fitness: number;
  passRate: number;
  llmScore: number;
  prompt: string;
  testResults: TestResultRecord[];
}

/**
 * Generation record for history tracking
 */
interface GenerationRecord {
  generation: number;
  fitness: number;
  passRate: number;
  llmScore: number;
  bestPrompt: string;
  populationSize: number;
  timestamp: string;
  testResults?: TestResultRecord[];
  variants?: VariantRecord[];
}

/**
 * Full evolution history for UI display
 */
interface EvolutionHistory {
  promptName: string;
  seedPrompt: string;
  evolvedPrompt: string;
  startedAt: string;
  completedAt: string;
  finalGeneration: number;
  generations: GenerationRecord[];
  config: Config;
  pipelineConfig: PipelineConfig;
}

// ============================================================================
// Fitness Calculation
// ============================================================================

/**
 * Calculate fitness from evaluation results.
 * Weighted combination of pass rate and LLM quality scores.
 */
function calculateFitness(result: EvalResult): number {
  // 40% weight on pass rate (structural correctness)
  // 60% weight on LLM score (semantic quality, normalized 0-1)
  const normalizedLlmScore = result.avgLlmScore / 5;
  return result.passRate * 0.4 + normalizedLlmScore * 0.6;
}

// ============================================================================
// Evolutionary Loop
// ============================================================================

async function evolve(config: Config): Promise<Individual> {
  console.log('🧬 Evolutionary Prompt Optimizer');
  console.log('================================\n');

  // Ensure output directory exists
  if (!existsSync(config.outputDir)) {
    mkdirSync(config.outputDir, { recursive: true });
  }

  // Load seed prompt
  const seedPath = join(config.seedDir, `${config.promptName}.txt`);
  if (!existsSync(seedPath)) {
    throw new Error(`Seed prompt not found: ${seedPath}`);
  }
  const seedPrompt = readFileSync(seedPath, 'utf8');

  console.log(`📄 Seed: ${seedPath}`);
  console.log(
    `🔧 Evolution: pop=${config.populationSize}, select=${config.selectTopK}, maxGen=${config.maxGenerations}`
  );
  console.log('');

  // Display pipeline config
  const pipelineConfig = parsePipelineConfig(config.evalConfig);
  displayPipelineConfig(pipelineConfig);

  // Track state
  let population: Individual[] = [];
  let bestEver: Individual | null = null;
  let gensWithoutImprovement = 0;
  const startedAt = new Date().toISOString();
  const generationHistory: GenerationRecord[] = [];

  // Generation 0: Evaluate seed
  console.log('📊 Generation 0: Evaluating seed prompt...');
  const seedResult = await evaluatePrompt(seedPrompt, 'gen0-seed', config.evalConfig);
  const seedIndividual: Individual = {
    prompt: seedPrompt,
    fitness: calculateFitness(seedResult),
    result: seedResult,
    generation: 0,
  };
  population = [seedIndividual];
  bestEver = seedIndividual;

  console.log(`   Seed fitness: ${seedIndividual.fitness.toFixed(3)}`);
  console.log(`   Pass rate: ${(seedResult.passRate * 100).toFixed(1)}%`);
  console.log(`   LLM score: ${seedResult.avgLlmScore.toFixed(2)}/5`);
  console.log('');

  // Record generation 0
  generationHistory.push({
    generation: 0,
    fitness: seedIndividual.fitness,
    passRate: seedResult.passRate,
    llmScore: seedResult.avgLlmScore,
    bestPrompt: seedPrompt,
    populationSize: 1,
    timestamp: new Date().toISOString(),
    testResults: seedResult.testResults,
    variants: [
      {
        variantId: 'gen0-seed',
        fitness: seedIndividual.fitness,
        passRate: seedResult.passRate,
        llmScore: seedResult.avgLlmScore,
        prompt: seedPrompt,
        testResults: seedResult.testResults,
      },
    ],
  });

  // Evolution loop
  for (let gen = 1; gen <= config.maxGenerations; gen++) {
    console.log(`🔄 Generation ${gen}`);

    // Generate mutations from current population
    const mutations: string[] = [];
    const variantsPerParent = Math.ceil(config.populationSize / population.length);

    for (const parent of population) {
      console.log(`   Mutating from parent (fitness=${parent.fitness.toFixed(3)})...`);
      try {
        const variants = await mutatePrompt(parent.prompt, { count: variantsPerParent });
        mutations.push(...variants);
      } catch (error) {
        console.error(`   Failed to mutate: ${error}`);
      }
    }

    if (mutations.length === 0) {
      console.log('   No mutations generated, using parent population');
      continue;
    }

    // Evaluate all mutations
    const evaluated: Individual[] = [];
    for (let i = 0; i < mutations.length; i++) {
      console.log(`   Evaluating variant ${i + 1}/${mutations.length}...`);
      try {
        const result = await evaluatePrompt(mutations[i], `gen${gen}-${i}`, config.evalConfig);
        const fitness = calculateFitness(result);
        evaluated.push({
          prompt: mutations[i],
          fitness,
          result,
          generation: gen,
          parentId: `gen${gen - 1}`,
        });
        console.log(
          `     fitness=${fitness.toFixed(3)}, pass=${(result.passRate * 100).toFixed(0)}%, llm=${result.avgLlmScore.toFixed(1)}`
        );
      } catch (error) {
        console.error(`     Evaluation failed: ${error}`);
      }
    }

    if (evaluated.length === 0) {
      console.log('   All evaluations failed, keeping current population');
      continue;
    }

    // Selection: combine population + new individuals, keep top K
    const combined = [...population, ...evaluated];
    combined.sort((a, b) => b.fitness - a.fitness);
    population = combined.slice(0, config.selectTopK);

    const genBest = population[0];
    console.log(`   Best this gen: fitness=${genBest.fitness.toFixed(3)}`);

    // Record this generation with all variants
    generationHistory.push({
      generation: gen,
      fitness: genBest.fitness,
      passRate: genBest.result.passRate,
      llmScore: genBest.result.avgLlmScore,
      bestPrompt: genBest.prompt,
      populationSize: evaluated.length,
      timestamp: new Date().toISOString(),
      testResults: genBest.result.testResults,
      variants: evaluated.map((ind) => ({
        variantId: ind.result.promptId,
        fitness: ind.fitness,
        passRate: ind.result.passRate,
        llmScore: ind.result.avgLlmScore,
        prompt: ind.prompt,
        testResults: ind.result.testResults,
      })),
    });

    // Track convergence
    if (genBest.fitness > bestEver.fitness) {
      bestEver = genBest;
      gensWithoutImprovement = 0;
      console.log(`   🎯 New best! fitness=${bestEver.fitness.toFixed(3)}`);
    } else {
      gensWithoutImprovement++;
      console.log(`   No improvement (${gensWithoutImprovement}/${config.convergenceThreshold})`);
    }

    console.log('');

    // Check convergence
    if (gensWithoutImprovement >= config.convergenceThreshold) {
      console.log('✅ Converged! No improvement for', config.convergenceThreshold, 'generations');
      break;
    }
  }

  // Output best prompt
  const timestamp = new Date().toISOString().split('T')[0];
  const outputPath = join(config.outputDir, `${config.promptName}-${timestamp}.txt`);
  writeFileSync(outputPath, bestEver.prompt);

  // Also save metadata
  const metadataPath = join(config.outputDir, `${config.promptName}-${timestamp}.json`);
  writeFileSync(
    metadataPath,
    JSON.stringify(
      {
        promptName: config.promptName,
        timestamp: new Date().toISOString(),
        fitness: bestEver.fitness,
        passRate: bestEver.result.passRate,
        avgLlmScore: bestEver.result.avgLlmScore,
        generation: bestEver.generation,
        config,
      },
      null,
      2
    )
  );

  // Save full evolution history for UI dashboard
  const historyPath = join(config.outputDir, `${config.promptName}-${timestamp}-history.json`);
  const evolutionHistory: EvolutionHistory = {
    promptName: config.promptName,
    seedPrompt,
    evolvedPrompt: bestEver.prompt,
    startedAt,
    completedAt: new Date().toISOString(),
    finalGeneration: bestEver.generation,
    generations: generationHistory,
    config,
    pipelineConfig,
  };
  writeFileSync(historyPath, JSON.stringify(evolutionHistory, null, 2));

  console.log('\n📝 Results');
  console.log('==========');
  console.log(`Best prompt: ${outputPath}`);
  console.log(`Metadata: ${metadataPath}`);
  console.log(`History: ${historyPath}`);
  console.log(`Fitness: ${bestEver.fitness.toFixed(3)}`);
  console.log(`Pass rate: ${(bestEver.result.passRate * 100).toFixed(1)}%`);
  console.log(`LLM score: ${bestEver.result.avgLlmScore.toFixed(2)}/5`);
  console.log(`Generation: ${bestEver.generation}`);

  return bestEver;
}

// ============================================================================
// CLI Entry Point
// ============================================================================

function parseArgs(): Partial<Config> {
  const args = process.argv.slice(2);
  const config: Partial<Config> = {};

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    const next = args[i + 1];

    switch (arg) {
      case '--prompt':
      case '-p':
        config.promptName = next;
        i++;
        break;
      case '--generations':
      case '-g':
        config.maxGenerations = parseInt(next, 10);
        i++;
        break;
      case '--population':
        config.populationSize = parseInt(next, 10);
        i++;
        break;
      case '--select':
        config.selectTopK = parseInt(next, 10);
        i++;
        break;
      case '--help':
      case '-h':
        console.log(`
Evolutionary Prompt Optimizer

Usage:
  npx tsx scripts/evolve-prompts.ts [options]

Options:
  --prompt, -p <name>      Prompt to evolve (default: concept-synthesis)
  --generations, -g <n>    Max generations (default: 10)
  --population <n>         Variants per generation (default: 5)
  --select <n>             Top K to keep (default: 2)
  --help, -h               Show this help

Examples:
  npx tsx scripts/evolve-prompts.ts
  npx tsx scripts/evolve-prompts.ts --prompt phrasing-generation --generations 5
`);
        process.exit(0);
    }
  }

  return config;
}

async function main() {
  const cliConfig = parseArgs();
  const config: Config = { ...DEFAULT_CONFIG, ...cliConfig };

  try {
    await evolve(config);
  } catch (error) {
    console.error('\n❌ Evolution failed:', error);
    process.exit(1);
  }
}

main();
