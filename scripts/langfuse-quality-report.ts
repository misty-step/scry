#!/usr/bin/env npx tsx
/**
 * Langfuse Quality Report
 *
 * Aggregates LLM-as-judge quality scores from Langfuse traces.
 * Tracks trends and alerts on quality degradation.
 *
 * Usage:
 *   pnpm quality:report              # Last 7 days
 *   pnpm quality:report --days 30    # Last 30 days
 *   pnpm quality:report --min 3.5    # Alert if avg < 3.5
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
  source: string;
  dataType: string;
}

interface LangfuseScoresResponse {
  data: LangfuseScore[];
  meta: { page: number; limit: number; totalItems: number; totalPages: number };
}

interface QualityBreakdown {
  totalScores: number;
  scoreSum: number;
  byDay: Record<string, { sum: number; count: number; scores: number[] }>;
  byScoreType: Record<string, { sum: number; count: number }>;
  distribution: Record<string, number>; // 0-1, 1-2, 2-3, 3-4, 4-5
  lowScoreTraces: Array<{ traceId: string; score: number; date: string }>;
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

async function generateQualityReport(days: number, minThreshold: number): Promise<void> {
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

  console.log(`\n📈 Langfuse Quality Report`);
  console.log(`   Period: Last ${days} day(s) (since ${fromDate.toISOString().split('T')[0]})`);
  console.log(`   Minimum threshold: ${minThreshold}/5`);
  console.log('');

  const breakdown: QualityBreakdown = {
    totalScores: 0,
    scoreSum: 0,
    byDay: {},
    byScoreType: {},
    distribution: { '0-1': 0, '1-2': 0, '2-3': 0, '3-4': 0, '4-5': 0 },
    lowScoreTraces: [],
  };

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
        // Skip non-numeric scores
        if (typeof score.value !== 'number') continue;

        const value = score.value;
        const day = new Date(score.timestamp).toISOString().split('T')[0];
        const scoreType = score.name || 'unknown';

        // Aggregate
        breakdown.totalScores++;
        breakdown.scoreSum += value;

        // By day
        if (!breakdown.byDay[day]) {
          breakdown.byDay[day] = { sum: 0, count: 0, scores: [] };
        }
        breakdown.byDay[day].sum += value;
        breakdown.byDay[day].count++;
        breakdown.byDay[day].scores.push(value);

        // By score type
        if (!breakdown.byScoreType[scoreType]) {
          breakdown.byScoreType[scoreType] = { sum: 0, count: 0 };
        }
        breakdown.byScoreType[scoreType].sum += value;
        breakdown.byScoreType[scoreType].count++;

        // Distribution (for 0-5 scale)
        if (value < 1) breakdown.distribution['0-1']++;
        else if (value < 2) breakdown.distribution['1-2']++;
        else if (value < 3) breakdown.distribution['2-3']++;
        else if (value < 4) breakdown.distribution['3-4']++;
        else breakdown.distribution['4-5']++;

        // Track low scores for investigation
        if (value < minThreshold && score.traceId) {
          breakdown.lowScoreTraces.push({
            traceId: score.traceId,
            score: value,
            date: day,
          });
        }
      }

      page++;
      hasMore = scores.data.length === 100;
    }
  } catch (error) {
    console.error('❌ Error fetching Langfuse data:', error);
    process.exit(1);
  }

  const avgScore = breakdown.totalScores > 0 ? breakdown.scoreSum / breakdown.totalScores : 0;

  // Print report
  console.log('═══════════════════════════════════════════════════════════════');
  console.log('                         SUMMARY');
  console.log('═══════════════════════════════════════════════════════════════');
  console.log(`  Total Scores:    ${breakdown.totalScores.toLocaleString()}`);
  console.log(
    `  Average Score:   ${avgScore.toFixed(2)}/5 ${avgScore >= 4 ? '✅' : avgScore >= 3 ? '⚠️' : '❌'}`
  );
  console.log(`  Score Range:     ${breakdown.distribution['0-1'] > 0 ? '0' : '1'}-5`);
  console.log('');

  console.log('───────────────────────────────────────────────────────────────');
  console.log('                    SCORE DISTRIBUTION');
  console.log('───────────────────────────────────────────────────────────────');
  const maxCount = Math.max(...Object.values(breakdown.distribution));
  for (const [range, count] of Object.entries(breakdown.distribution)) {
    const bar = '█'.repeat(Math.ceil((count / (maxCount || 1)) * 30));
    const pct = ((count / (breakdown.totalScores || 1)) * 100).toFixed(1);
    console.log(`  ${range}:  ${count.toString().padStart(5)}  (${pct.padStart(5)}%)  ${bar}`);
  }
  console.log('');

  console.log('───────────────────────────────────────────────────────────────');
  console.log('                    BY SCORE TYPE');
  console.log('───────────────────────────────────────────────────────────────');
  const sortedTypes = Object.entries(breakdown.byScoreType).sort(
    (a, b) => b[1].sum / b[1].count - a[1].sum / a[1].count
  );
  for (const [type, data] of sortedTypes) {
    const avg = data.sum / data.count;
    const status = avg >= 4 ? '✅' : avg >= 3 ? '⚠️' : '❌';
    console.log(`  ${type.padEnd(25)} ${avg.toFixed(2)}/5 ${status} (n=${data.count})`);
  }
  console.log('');

  console.log('───────────────────────────────────────────────────────────────');
  console.log('                    DAILY TREND');
  console.log('───────────────────────────────────────────────────────────────');
  const sortedDays = Object.entries(breakdown.byDay).sort((a, b) => a[0].localeCompare(b[0]));

  // Calculate trend
  let prevAvg = 0;
  for (const [day, data] of sortedDays) {
    const avg = data.sum / data.count;
    const trend = prevAvg === 0 ? '  ' : avg > prevAvg ? '📈' : avg < prevAvg ? '📉' : '➡️';
    const status = avg >= 4 ? '✅' : avg >= 3 ? '⚠️' : '❌';

    // Simple sparkline of scores
    const sorted = data.scores.sort((a, b) => a - b);
    const min = sorted[0].toFixed(1);
    const max = sorted[sorted.length - 1].toFixed(1);

    console.log(
      `  ${day}  ${avg.toFixed(2)}/5 ${status} ${trend}  [${min}-${max}] (n=${data.count})`
    );
    prevAvg = avg;
  }
  console.log('');

  // Low score traces for investigation
  if (breakdown.lowScoreTraces.length > 0) {
    console.log('───────────────────────────────────────────────────────────────');
    console.log(`               LOW SCORE TRACES (< ${minThreshold})`);
    console.log('───────────────────────────────────────────────────────────────');
    const recent = breakdown.lowScoreTraces.slice(-10); // Show last 10
    for (const trace of recent) {
      console.log(`  ${trace.date}  ${trace.score.toFixed(1)}/5  ${trace.traceId}`);
    }
    if (breakdown.lowScoreTraces.length > 10) {
      console.log(`  ... and ${breakdown.lowScoreTraces.length - 10} more`);
    }
    console.log('');
  }

  // Alert check
  if (avgScore < minThreshold && breakdown.totalScores > 0) {
    console.log('═══════════════════════════════════════════════════════════════');
    console.log(
      `  ⚠️  ALERT: Average score (${avgScore.toFixed(2)}) below threshold (${minThreshold})`
    );
    console.log('═══════════════════════════════════════════════════════════════');
    process.exit(1);
  } else if (breakdown.totalScores === 0) {
    console.log('ℹ️  No scores found in the specified period');
  } else {
    console.log(`✅ Quality (${avgScore.toFixed(2)}/5) meets threshold (${minThreshold})`);
  }
}

// Parse CLI args
const args = process.argv.slice(2);
let days = 7;
let minThreshold = 3.5;

for (let i = 0; i < args.length; i++) {
  if (args[i] === '--days' && args[i + 1]) {
    days = parseInt(args[i + 1], 10);
    i++;
  } else if (args[i] === '--min' && args[i + 1]) {
    minThreshold = parseFloat(args[i + 1]);
    i++;
  } else if (args[i] === '--help') {
    console.log(`
Langfuse Quality Report

Usage:
  pnpm quality:report [options]

Options:
  --days <n>    Number of days to report (default: 7)
  --min <score> Minimum acceptable average score (default: 3.5)
  --help        Show this help

Examples:
  pnpm quality:report                 # Last 7 days
  pnpm quality:report --days 30       # Last 30 days
  pnpm quality:report --min 4.0       # Alert if avg < 4.0
`);
    process.exit(0);
  }
}

generateQualityReport(days, minThreshold);

// Make this file an ES module to avoid TypeScript global scope collisions
export {};
