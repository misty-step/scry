/**
 * Prompt Evaluation Runner
 *
 * Wraps promptfoo to evaluate a single prompt variant against the test suite.
 * Returns structured results for fitness calculation.
 */

import { execSync } from 'child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';

export interface TestResult {
  description: string;
  passed: boolean;
  score?: number;
  latencyMs?: number;
  error?: string;
  judgeReason?: string;
  assertionType?: string;
}

export interface EvalResult {
  promptId: string;
  passRate: number; // 0-1
  avgLlmScore: number; // 1-5 from llm-rubric assertions
  latencyP95: number; // ms
  totalTests: number;
  passedTests: number;
  testResults: TestResult[];
}

interface PromptfooOutput {
  results: {
    results: Array<{
      success: boolean;
      score?: number;
      latencyMs?: number;
      error?: string;
      vars?: Record<string, unknown>;
      gradingResult?: {
        pass: boolean;
        score?: number;
        reason?: string;
        componentResults?: Array<{
          pass: boolean;
          score?: number;
          reason?: string;
          assertion?: {
            type: string;
          };
        }>;
      };
    }>;
  };
  stats?: {
    successes: number;
    failures: number;
  };
}

/**
 * Evaluate a prompt variant against the promptfoo test suite.
 *
 * @param prompt - The prompt text to evaluate
 * @param promptId - Unique identifier for this evaluation
 * @param configPath - Path to promptfoo.yaml (default: evals/promptfoo.yaml)
 * @returns Structured evaluation results
 */
export async function evaluatePrompt(
  prompt: string,
  promptId: string,
  configPath = 'evals/promptfoo.yaml'
): Promise<EvalResult> {
  // Create temp directory for this evaluation
  const tempDir = mkdtempSync(join(tmpdir(), `promptfoo-${promptId}-`));
  const promptPath = join(tempDir, 'prompt.txt');
  const outputPath = join(tempDir, 'results.json');

  try {
    // Write prompt to temp file
    writeFileSync(promptPath, prompt);

    // Run promptfoo evaluation
    // Note: We override the prompts in the config with our variant
    const cmd = [
      'npx promptfoo eval',
      `-c ${configPath}`,
      `--prompts "file://${promptPath}"`,
      `--output ${outputPath}`,
      '--no-cache', // Don't cache - we want fresh results
    ].join(' ');

    try {
      execSync(cmd, {
        encoding: 'utf8',
        stdio: 'inherit', // Show promptfoo progress bar
        timeout: 600000, // 10 minute timeout for full eval
      });
    } catch {
      // promptfoo exits non-zero if any test fails - that's expected
    }

    // Check results file exists (promptfoo may have crashed vs just having failures)
    if (!existsSync(outputPath)) {
      throw new Error('promptfoo did not produce output file');
    }

    // Parse results
    const rawOutput = readFileSync(outputPath, 'utf8');
    const data: PromptfooOutput = JSON.parse(rawOutput);

    return parsePromptfooResults(data, promptId);
  } finally {
    // Cleanup temp directory
    try {
      rmSync(tempDir, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }
  }
}

/**
 * Parse promptfoo output into structured eval results.
 */
function parsePromptfooResults(data: PromptfooOutput, promptId: string): EvalResult {
  const results = data.results?.results || [];

  const testResults: TestResult[] = results.map((r, i) => {
    // Extract test description from vars or use fallback
    const description = (r.vars?.description as string) || `Test ${i + 1}`;

    // Find llm-rubric assertion and extract its reason
    const llmRubricResult = r.gradingResult?.componentResults?.find(
      (c) => c.assertion?.type === 'llm-rubric'
    );
    const judgeReason = llmRubricResult?.reason || r.gradingResult?.reason;
    const assertionType = llmRubricResult?.assertion?.type;

    return {
      description,
      passed: r.success || r.gradingResult?.pass || false,
      score: r.gradingResult?.score ?? r.score,
      latencyMs: r.latencyMs,
      error: r.error,
      judgeReason,
      assertionType,
    };
  });

  const passedTests = testResults.filter((t) => t.passed).length;
  const totalTests = testResults.length;
  const passRate = totalTests > 0 ? passedTests / totalTests : 0;

  // Extract LLM rubric scores (type: llm-rubric assertions)
  const llmScores: number[] = [];
  for (const result of results) {
    const components = result.gradingResult?.componentResults || [];
    for (const comp of components) {
      if (comp.assertion?.type === 'llm-rubric' && comp.score !== undefined) {
        llmScores.push(comp.score);
      }
    }
  }
  const avgLlmScore =
    llmScores.length > 0 ? llmScores.reduce((a, b) => a + b, 0) / llmScores.length : 0;

  // Calculate P95 latency
  const latencies = testResults.filter((t) => t.latencyMs !== undefined).map((t) => t.latencyMs!);
  latencies.sort((a, b) => a - b);
  const p95Index = Math.floor(latencies.length * 0.95);
  const latencyP95 = latencies[p95Index] || 0;

  return {
    promptId,
    passRate,
    avgLlmScore,
    latencyP95,
    totalTests,
    passedTests,
    testResults,
  };
}

/**
 * Run a quick evaluation on a subset of tests.
 * Useful for rapid iteration during evolution.
 */
export async function evaluatePromptQuick(
  prompt: string,
  promptId: string,
  maxTests = 5
): Promise<EvalResult> {
  // For quick eval, we could filter tests or use a smaller config
  // For now, just use the full eval with a note
  console.log(`  [quick-eval] Running on first ${maxTests} tests (full eval for now)`);
  return evaluatePrompt(prompt, promptId);
}
