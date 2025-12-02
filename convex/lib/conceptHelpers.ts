import type { Doc } from '../_generated/dataModel';

export type ConceptDoc = Doc<'concepts'>;
export type ConceptLibraryView = 'all' | 'due' | 'thin' | 'tension' | 'archived' | 'deleted';

export function clampPageSize(
  pageSize: number | null | undefined,
  defaults: { min: number; max: number; default: number }
): number {
  if (!pageSize) {
    return defaults.default;
  }
  return Math.max(defaults.min, Math.min(defaults.max, pageSize));
}

export function matchesConceptView(
  concept: ConceptDoc,
  now: number,
  view: ConceptLibraryView
): boolean {
  if (view === 'deleted') {
    return !!concept.deletedAt;
  }

  if (concept.deletedAt) {
    return false;
  }

  if (view === 'archived') {
    return !!concept.archivedAt;
  }

  if (concept.archivedAt) {
    return false;
  }

  if (view === 'all') {
    return true;
  }

  const isDue = concept.fsrs.nextReview <= now;
  const isThin = (concept.thinScore ?? 0) > 0;
  const hasTension = (concept.conflictScore ?? 0) > 0;

  if (view === 'due') {
    return isDue;
  }
  if (view === 'thin') {
    return isThin;
  }
  if (view === 'tension') {
    return hasTension;
  }
  return true;
}

export function computeThinScoreFromCount(
  count: number,
  targetPhrasings: number
): number | undefined {
  if (count >= targetPhrasings) {
    return undefined;
  }
  const delta = targetPhrasings - Math.max(0, count);
  return delta > 0 ? delta : undefined;
}

export function prioritizeConcepts(
  concepts: ConceptDoc[],
  now: Date,
  getRetrievability: (fsrs: ConceptDoc['fsrs'], now: Date) => number,
  random: () => number = Math.random
): Array<{ concept: ConceptDoc; retrievability: number }> {
  const prioritized = concepts
    .filter((concept) => concept.phrasingCount > 0)
    .map((concept) => ({
      concept,
      retrievability: concept.fsrs.retrievability ?? getRetrievability(concept.fsrs, now),
    }))
    .sort((a, b) => a.retrievability - b.retrievability);

  const base = prioritized[0]?.retrievability;
  if (base === undefined) {
    return [];
  }

  const URGENCY_DELTA = 0.05;
  const urgentTier: typeof prioritized = [];
  for (const item of prioritized) {
    if (Math.abs(item.retrievability - base) <= URGENCY_DELTA) {
      urgentTier.push(item);
    } else {
      break;
    }
  }

  for (let i = urgentTier.length - 1; i > 0; i--) {
    const j = Math.floor(random() * (i + 1));
    [urgentTier[i], urgentTier[j]] = [urgentTier[j], urgentTier[i]];
  }

  return [...urgentTier, ...prioritized.slice(urgentTier.length)];
}
