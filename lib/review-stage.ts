export function formatReviewStageLabel(
  state: string | null | undefined,
  options?: { fallback?: string }
) {
  const fallback = options?.fallback ?? 'New';
  if (!state) return fallback;
  if (state === 'relearning') return 'Relearning';
  if (state === 'learning') return 'Learning';
  if (state === 'review') return 'Review';
  if (state === 'new') return 'New';
  return state;
}
