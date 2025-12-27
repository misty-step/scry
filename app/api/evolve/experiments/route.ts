import { existsSync, readdirSync, readFileSync } from 'fs';
import { join } from 'path';
import { NextResponse } from 'next/server';
import type { ExperimentsResponse, ExperimentSummary } from '@/types/evolve';

export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

const ARTIFACTS_DIR = 'artifacts/optimized-prompts';

export async function GET() {
  // Dev-only guard
  if (process.env.NODE_ENV === 'production') {
    return NextResponse.json({ error: 'Not available in production' }, { status: 403 });
  }

  try {
    const experiments = listExperiments();
    const response: ExperimentsResponse = {
      experiments,
      count: experiments.length,
    };
    return NextResponse.json(response);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : 'Failed to list experiments' },
      { status: 500 }
    );
  }
}

/**
 * List all experiments from artifacts directory.
 * Parses *-YYYY-MM-DD.json files (not *-history.json).
 */
function listExperiments(): ExperimentSummary[] {
  const artifactsPath = join(process.cwd(), ARTIFACTS_DIR);

  if (!existsSync(artifactsPath)) {
    return [];
  }

  const files = readdirSync(artifactsPath);

  // Find metadata files (not history files)
  const metadataFiles = files.filter((f) => f.endsWith('.json') && !f.includes('-history'));

  const experiments: ExperimentSummary[] = [];

  for (const file of metadataFiles) {
    try {
      const content = readFileSync(join(artifactsPath, file), 'utf8');
      const data = JSON.parse(content);

      // Extract date from filename: concept-synthesis-2025-12-24.json
      const dateMatch = file.match(/(\d{4}-\d{2}-\d{2})\.json$/);
      const date = dateMatch ? dateMatch[1] : 'unknown';

      // Check if history file exists
      const historyFile = file.replace('.json', '-history.json');
      const hasHistory = files.includes(historyFile);

      experiments.push({
        id: file.replace('.json', ''),
        promptName: data.promptName || 'unknown',
        date,
        fitness: data.fitness ?? 0,
        passRate: data.passRate ?? 0,
        avgLlmScore: data.avgLlmScore ?? 0,
        generation: data.generation ?? 0,
        hasHistory,
      });
    } catch {
      // Skip malformed files
      continue;
    }
  }

  // Sort by date descending (most recent first)
  experiments.sort((a, b) => b.date.localeCompare(a.date));

  return experiments;
}
