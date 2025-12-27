import { existsSync, readFileSync } from 'fs';
import { join } from 'path';
import { NextResponse } from 'next/server';
import type { EvolutionHistory } from '@/types/evolve';

export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

const ARTIFACTS_DIR = 'artifacts/optimized-prompts';

interface RouteParams {
  params: Promise<{ id: string }>;
}

export async function GET(_request: Request, { params }: RouteParams) {
  // Dev-only guard
  if (process.env.NODE_ENV === 'production') {
    return NextResponse.json({ error: 'Not available in production' }, { status: 403 });
  }

  const { id } = await params;
  const artifactsPath = join(process.cwd(), ARTIFACTS_DIR);

  // Try history file first (has full data)
  const historyPath = join(artifactsPath, `${id}-history.json`);
  if (existsSync(historyPath)) {
    try {
      const content = readFileSync(historyPath, 'utf8');
      const history: EvolutionHistory = JSON.parse(content);
      return NextResponse.json({ history, source: 'history' });
    } catch {
      return NextResponse.json({ error: 'Failed to parse history file' }, { status: 500 });
    }
  }

  // Fall back to metadata + prompt files
  const metadataPath = join(artifactsPath, `${id}.json`);
  const promptPath = join(artifactsPath, `${id}.txt`);

  if (!existsSync(metadataPath)) {
    return NextResponse.json({ error: 'Experiment not found' }, { status: 404 });
  }

  try {
    const metadata = JSON.parse(readFileSync(metadataPath, 'utf8'));
    const evolvedPrompt = existsSync(promptPath) ? readFileSync(promptPath, 'utf8') : null;

    // Construct partial history from metadata
    const partialHistory: Partial<EvolutionHistory> = {
      promptName: metadata.promptName,
      evolvedPrompt: evolvedPrompt ?? undefined,
      completedAt: metadata.timestamp,
      finalGeneration: metadata.generation,
      config: metadata.config,
      generations: [
        {
          generation: metadata.generation,
          fitness: metadata.fitness,
          passRate: metadata.passRate,
          llmScore: metadata.avgLlmScore,
          bestPrompt: evolvedPrompt ?? '',
          populationSize: 0,
          timestamp: metadata.timestamp,
        },
      ],
    };

    return NextResponse.json({ history: partialHistory, source: 'metadata' });
  } catch {
    return NextResponse.json({ error: 'Failed to load experiment' }, { status: 500 });
  }
}
