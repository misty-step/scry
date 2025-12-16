#!/usr/bin/env npx tsx
/**
 * Capture low-score traces as promptfoo test cases.
 *
 * Fetches traces from Langfuse that scored below a threshold and
 * generates promptfoo test cases for regression testing.
 *
 * Usage:
 *   pnpm capture:failures              # Last 24 hours, score < 3.0
 *   pnpm capture:failures --days 7     # Last 7 days
 *   pnpm capture:failures --min 2.5    # Score threshold
 *
 * Environment:
 *   LANGFUSE_SECRET_KEY - Required
 *   LANGFUSE_PUBLIC_KEY - Required
 *   LANGFUSE_HOST - Optional (defaults to US cloud)
 */

interface LangfuseScore {
  id: string;
  traceId: string;
  name: string;
  value: number;
  timestamp: string;
}

interface LangfuseScoresResponse {
  data: LangfuseScore[];
  meta: { page: number; limit: number; totalItems: number };
}

interface LangfuseTrace {
  id: string;
  input: unknown;
  output: unknown;
  metadata: Record<string, unknown>;
}

interface FailedTrace {
  traceId: string;
  scoreName: string;
  score: number;
  input: string;
  date: string;
}

async function fetchScores(
  baseUrl: string,
  authHeader: string,
  fromDate: Date,
  page: number
): Promise<LangfuseScoresResponse> {
  const params = new URLSearchParams({
    fromTimestamp: fromDate.toISOString(),
    limit: '100',
    page: page.toString(),
  });

  const response = await fetch(`${baseUrl}/api/public/scores?${params}`, {
    headers: {
      Authorization: authHeader,
      'Content-Type': 'application/json',
    },
  });

  if (!response.ok) {
    throw new Error(`Langfuse API error: ${response.status} ${response.statusText}`);
  }

  return response.json();
}

async function fetchTrace(
  baseUrl: string,
  authHeader: string,
  traceId: string
): Promise<LangfuseTrace | null> {
  const response = await fetch(`${baseUrl}/api/public/traces/${traceId}`, {
    headers: {
      Authorization: authHeader,
      'Content-Type': 'application/json',
    },
  });

  if (!response.ok) {
    console.warn(`Failed to fetch trace ${traceId}: ${response.status}`);
    return null;
  }

  return response.json();
}

function generateYamlTestCase(failure: FailedTrace): string {
  // Escape the input for YAML
  const escapedInput = failure.input.replace(/'/g, "''");

  return `
  - description: "[auto-captured] ${failure.scoreName} score ${failure.score.toFixed(1)} on ${failure.date}"
    vars:
      intentJson: '${escapedInput}'
    assert:
      - type: is-json
      - type: llm-rubric
        value: "This input previously scored ${failure.score.toFixed(1)}/5 for ${failure.scoreName}. The output should demonstrate improvement."
    metadata:
      traceId: "${failure.traceId}"
      originalScore: ${failure.score}
      capturedAt: "${new Date().toISOString()}"`;
}

async function captureFailures(days: number, minScore: number): Promise<void> {
  // Validate environment
  if (!process.env.LANGFUSE_SECRET_KEY || !process.env.LANGFUSE_PUBLIC_KEY) {
    console.error('❌ Missing LANGFUSE_SECRET_KEY or LANGFUSE_PUBLIC_KEY');
    console.error('Set these environment variables or add them to .env.local');
    process.exit(1);
  }

  const baseUrl = process.env.LANGFUSE_HOST || 'https://us.cloud.langfuse.com';
  const authHeader =
    'Basic ' +
    Buffer.from(`${process.env.LANGFUSE_PUBLIC_KEY}:${process.env.LANGFUSE_SECRET_KEY}`).toString(
      'base64'
    );

  const fromDate = new Date(Date.now() - days * 24 * 60 * 60 * 1000);

  console.log(`\n🔍 Capturing Failed Test Cases`);
  console.log(`   Period: Last ${days} day(s) (since ${fromDate.toISOString().split('T')[0]})`);
  console.log(`   Threshold: < ${minScore}/5`);
  console.log('');

  const failures: FailedTrace[] = [];
  const seenTraces = new Set<string>();

  try {
    // Fetch scores with pagination
    let page = 1;
    let hasMore = true;

    while (hasMore) {
      const scores = await fetchScores(baseUrl, authHeader, fromDate, page);

      if (!scores.data || scores.data.length === 0) {
        hasMore = false;
        break;
      }

      for (const score of scores.data) {
        // Skip non-numeric scores or scores above threshold
        if (typeof score.value !== 'number' || score.value >= minScore) continue;
        if (!score.traceId) continue;

        // Deduplicate by trace
        if (seenTraces.has(score.traceId)) continue;
        seenTraces.add(score.traceId);

        // Fetch trace details to get input
        const trace = await fetchTrace(baseUrl, authHeader, score.traceId);
        if (!trace?.input) continue;

        // Extract intentJson from input if it's the expected structure
        let inputStr: string;
        if (typeof trace.input === 'object' && trace.input !== null) {
          const inputObj = trace.input as Record<string, unknown>;
          // Look for intentJson in common locations
          if (inputObj.intentJson) {
            inputStr = String(inputObj.intentJson);
          } else if (inputObj.vars && typeof inputObj.vars === 'object') {
            const vars = inputObj.vars as Record<string, unknown>;
            inputStr = vars.intentJson ? String(vars.intentJson) : JSON.stringify(trace.input);
          } else {
            inputStr = JSON.stringify(trace.input);
          }
        } else {
          inputStr = String(trace.input);
        }

        failures.push({
          traceId: score.traceId,
          scoreName: score.name,
          score: score.value,
          input: inputStr,
          date: new Date(score.timestamp).toISOString().split('T')[0],
        });
      }

      page++;
      hasMore = scores.data.length === 100;
    }
  } catch (error) {
    console.error('❌ Error fetching Langfuse data:', error);
    process.exit(1);
  }

  if (failures.length === 0) {
    console.log('✅ No failed traces found below threshold');
    console.log('   This is good! Your prompts are performing well.');
    return;
  }

  console.log(`Found ${failures.length} low-score trace(s)`);
  console.log('');

  // Print summary
  console.log('───────────────────────────────────────────────────────────────');
  console.log('                    CAPTURED FAILURES');
  console.log('───────────────────────────────────────────────────────────────');

  for (const failure of failures) {
    console.log(`  ${failure.date}  ${failure.score.toFixed(1)}/5  ${failure.scoreName}`);
    console.log(`    └─ ${failure.traceId}`);
  }
  console.log('');

  // Generate YAML test cases
  const outputPath = 'evals/captured-failures.yaml';

  // Read existing file or create header
  const fs = await import('fs');
  let existingContent = '';

  if (fs.existsSync(outputPath)) {
    existingContent = fs.readFileSync(outputPath, 'utf8');
  } else {
    existingContent = `# Auto-captured failed test cases from Langfuse
# Generated by: pnpm capture:failures
# These test cases represent real production inputs that scored poorly.
# Use them to improve prompts and track regression.

description: "Auto-captured failure cases"

prompts:
  - file://prompts/concept-synthesis.txt

providers:
  - id: openrouter:google/gemini-3-pro-preview
    config:
      temperature: 0.7

tests:`;
  }

  // Check for duplicates (by traceId in existing content)
  const newFailures = failures.filter((f) => !existingContent.includes(f.traceId));

  if (newFailures.length === 0) {
    console.log('ℹ️  All failures already captured in existing file');
    return;
  }

  const newContent = newFailures.map(generateYamlTestCase).join('\n');
  fs.writeFileSync(outputPath, existingContent + newContent);

  console.log(`✅ Added ${newFailures.length} new test case(s) to ${outputPath}`);
  console.log('');
  console.log('Next steps:');
  console.log('  1. Review the captured test cases');
  console.log('  2. Run: npx promptfoo eval -c evals/captured-failures.yaml');
  console.log('  3. Iterate on prompts to improve scores');
}

// Parse CLI args
const args = process.argv.slice(2);
let days = 1;
let minScore = 3.0;

for (let i = 0; i < args.length; i++) {
  if (args[i] === '--days' && args[i + 1]) {
    days = parseInt(args[i + 1], 10);
    i++;
  } else if (args[i] === '--min' && args[i + 1]) {
    minScore = parseFloat(args[i + 1]);
    i++;
  } else if (args[i] === '--help') {
    console.log(`
Capture Failed Test Cases from Langfuse

Fetches low-score traces and generates promptfoo test cases for
regression testing and prompt improvement.

Usage:
  pnpm capture:failures [options]

Options:
  --days <n>    Number of days to look back (default: 1)
  --min <score> Minimum score threshold (default: 3.0)
  --help        Show this help

Examples:
  pnpm capture:failures                 # Last 24 hours, score < 3.0
  pnpm capture:failures --days 7        # Last 7 days
  pnpm capture:failures --min 2.5       # Only very low scores
`);
    process.exit(0);
  }
}

captureFailures(days, minScore);
