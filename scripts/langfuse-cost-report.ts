#!/usr/bin/env npx tsx
/**
 * Langfuse Cost Report
 *
 * Generates a daily/weekly cost report from Langfuse traces.
 * Alerts if spend exceeds configured budget.
 *
 * Usage:
 *   pnpm cost:report              # Last 24 hours
 *   pnpm cost:report --days 7     # Last 7 days
 *   pnpm cost:report --alert 10   # Alert if > $10
 *
 * Environment:
 *   LANGFUSE_SECRET_KEY - Required
 *   LANGFUSE_PUBLIC_KEY - Required
 *   LANGFUSE_HOST - Optional (defaults to US cloud)
 */
import { Langfuse } from 'langfuse';

// Model pricing (per 1M tokens) - Update as needed
const MODEL_PRICING: Record<string, { input: number; output: number }> = {
  'google/gemini-3-flash-preview': { input: 0.1, output: 0.4 },
  'google/gemini-3-pro-preview': { input: 1.25, output: 5.0 },
  'google/gemini-2.5-pro': { input: 1.25, output: 5.0 },
  'google/gemini-2.5-flash': { input: 0.075, output: 0.3 },
  'google/gemini-2.0-flash-exp': { input: 0.0, output: 0.0 }, // Free tier
  'text-embedding-004': { input: 0.00001, output: 0.0 }, // Embedding model
  // Default fallback
  default: { input: 1.0, output: 3.0 },
};

interface CostBreakdown {
  totalCost: number;
  byModel: Record<string, { cost: number; calls: number; tokens: number }>;
  byPhase: Record<string, { cost: number; calls: number }>;
  byDay: Record<string, number>;
  totalTokens: { input: number; output: number };
  totalCalls: number;
}

function calculateCost(
  model: string | undefined,
  usage: { promptTokens?: number; completionTokens?: number } | undefined
): number {
  if (!usage) return 0;

  const pricing = MODEL_PRICING[model || ''] || MODEL_PRICING.default;
  const inputTokens = usage.promptTokens || 0;
  const outputTokens = usage.completionTokens || 0;

  return (inputTokens * pricing.input + outputTokens * pricing.output) / 1_000_000;
}

async function generateCostReport(days: number, alertThreshold: number): Promise<void> {
  // Validate environment
  if (!process.env.LANGFUSE_SECRET_KEY || !process.env.LANGFUSE_PUBLIC_KEY) {
    console.error('❌ Missing LANGFUSE_SECRET_KEY or LANGFUSE_PUBLIC_KEY');
    console.error('Set these environment variables or add them to .env.local');
    process.exit(1);
  }

  const langfuse = new Langfuse({
    secretKey: process.env.LANGFUSE_SECRET_KEY,
    publicKey: process.env.LANGFUSE_PUBLIC_KEY,
    baseUrl: process.env.LANGFUSE_HOST || 'https://us.cloud.langfuse.com',
  });

  const fromDate = new Date(Date.now() - days * 24 * 60 * 60 * 1000);

  console.log(`\n📊 Langfuse Cost Report`);
  console.log(`   Period: Last ${days} day(s) (since ${fromDate.toISOString().split('T')[0]})`);
  console.log(`   Alert threshold: $${alertThreshold}`);
  console.log('');

  const breakdown: CostBreakdown = {
    totalCost: 0,
    byModel: {},
    byPhase: {},
    byDay: {},
    totalTokens: { input: 0, output: 0 },
    totalCalls: 0,
  };

  try {
    // Fetch traces with pagination
    let page = 1;
    let hasMore = true;

    while (hasMore) {
      const traces = await langfuse.fetchTraces({
        fromTimestamp: fromDate,
        limit: 100,
        page,
      });

      if (!traces.data || traces.data.length === 0) {
        hasMore = false;
        break;
      }

      for (const trace of traces.data) {
        // Get generations for this trace
        const generations = await langfuse.fetchObservations({
          traceId: trace.id,
          type: 'GENERATION',
        });

        for (const gen of generations.data) {
          const model = gen.model || 'unknown';
          const usage = gen.usage as
            | { promptTokens?: number; completionTokens?: number }
            | undefined;
          const cost = calculateCost(model, usage);
          const day = new Date(gen.startTime || trace.timestamp).toISOString().split('T')[0];
          const phase = ((trace.metadata as Record<string, unknown>)?.phase as string) || 'unknown';

          // Aggregate
          breakdown.totalCost += cost;
          breakdown.totalCalls++;

          // By model
          if (!breakdown.byModel[model]) {
            breakdown.byModel[model] = { cost: 0, calls: 0, tokens: 0 };
          }
          breakdown.byModel[model].cost += cost;
          breakdown.byModel[model].calls++;
          breakdown.byModel[model].tokens +=
            (usage?.promptTokens || 0) + (usage?.completionTokens || 0);

          // By phase
          if (!breakdown.byPhase[phase]) {
            breakdown.byPhase[phase] = { cost: 0, calls: 0 };
          }
          breakdown.byPhase[phase].cost += cost;
          breakdown.byPhase[phase].calls++;

          // By day
          breakdown.byDay[day] = (breakdown.byDay[day] || 0) + cost;

          // Total tokens
          breakdown.totalTokens.input += usage?.promptTokens || 0;
          breakdown.totalTokens.output += usage?.completionTokens || 0;
        }
      }

      page++;
      hasMore = traces.data.length === 100;
    }
  } catch (error) {
    console.error('❌ Error fetching Langfuse data:', error);
    process.exit(1);
  }

  // Print report
  console.log('═══════════════════════════════════════════════════════════════');
  console.log('                         SUMMARY');
  console.log('═══════════════════════════════════════════════════════════════');
  console.log(`  Total Cost:      $${breakdown.totalCost.toFixed(4)}`);
  console.log(`  Total Calls:     ${breakdown.totalCalls.toLocaleString()}`);
  console.log(`  Input Tokens:    ${breakdown.totalTokens.input.toLocaleString()}`);
  console.log(`  Output Tokens:   ${breakdown.totalTokens.output.toLocaleString()}`);
  console.log(
    `  Avg Cost/Call:   $${(breakdown.totalCost / (breakdown.totalCalls || 1)).toFixed(6)}`
  );
  console.log('');

  console.log('───────────────────────────────────────────────────────────────');
  console.log('                       BY MODEL');
  console.log('───────────────────────────────────────────────────────────────');
  const sortedModels = Object.entries(breakdown.byModel).sort((a, b) => b[1].cost - a[1].cost);
  for (const [model, data] of sortedModels) {
    const pct =
      breakdown.totalCost > 0 ? ((data.cost / breakdown.totalCost) * 100).toFixed(1) : '0.0';
    console.log(`  ${model.padEnd(35)} $${data.cost.toFixed(4).padStart(8)} (${pct}%)`);
    console.log(`    └─ ${data.calls} calls, ${data.tokens.toLocaleString()} tokens`);
  }
  console.log('');

  console.log('───────────────────────────────────────────────────────────────');
  console.log('                       BY PHASE');
  console.log('───────────────────────────────────────────────────────────────');
  const sortedPhases = Object.entries(breakdown.byPhase).sort((a, b) => b[1].cost - a[1].cost);
  for (const [phase, data] of sortedPhases) {
    const pct =
      breakdown.totalCost > 0 ? ((data.cost / breakdown.totalCost) * 100).toFixed(1) : '0.0';
    console.log(
      `  ${phase.padEnd(20)} $${data.cost.toFixed(4).padStart(8)} (${pct}%) - ${data.calls} calls`
    );
  }
  console.log('');

  console.log('───────────────────────────────────────────────────────────────');
  console.log('                       BY DAY');
  console.log('───────────────────────────────────────────────────────────────');
  const sortedDays = Object.entries(breakdown.byDay).sort((a, b) => a[0].localeCompare(b[0]));
  for (const [day, cost] of sortedDays) {
    const barLength = breakdown.totalCost > 0 ? Math.ceil((cost / breakdown.totalCost) * 30) : 0;
    const bar = '█'.repeat(barLength);
    console.log(`  ${day}  $${cost.toFixed(4).padStart(8)}  ${bar}`);
  }
  console.log('');

  // Alert check
  if (breakdown.totalCost > alertThreshold) {
    console.log('═══════════════════════════════════════════════════════════════');
    console.log(
      `  ⚠️  ALERT: Cost ($${breakdown.totalCost.toFixed(2)}) exceeds threshold ($${alertThreshold})`
    );
    console.log('═══════════════════════════════════════════════════════════════');
    process.exit(1);
  } else {
    console.log(
      `✅ Cost ($${breakdown.totalCost.toFixed(4)}) is within budget ($${alertThreshold})`
    );
  }

  await langfuse.shutdownAsync();
}

// Parse CLI args
const args = process.argv.slice(2);
let days = 1;
let alertThreshold = 20;

for (let i = 0; i < args.length; i++) {
  if (args[i] === '--days' && args[i + 1]) {
    days = parseInt(args[i + 1], 10);
    i++;
  } else if (args[i] === '--alert' && args[i + 1]) {
    alertThreshold = parseFloat(args[i + 1]);
    i++;
  } else if (args[i] === '--help') {
    console.log(`
Langfuse Cost Report

Usage:
  pnpm cost:report [options]

Options:
  --days <n>    Number of days to report (default: 1)
  --alert <$>   Alert threshold in dollars (default: 20)
  --help        Show this help

Examples:
  pnpm cost:report                 # Last 24 hours
  pnpm cost:report --days 7        # Last 7 days
  pnpm cost:report --alert 10      # Alert if > $10
`);
    process.exit(0);
  }
}

generateCostReport(days, alertThreshold);
