const EXPLICIT_RESCHEDULE_VERB = /\b(resched(?:ule)?|postpone|delay)\b/;
const SOFT_RESCHEDULE_VERB = /\b(push|move)\b/;
const RESCHEDULE_TARGET_HINT = /\b(card|question|review|session|this|it)\b/;
const TIME_HINT = /\b(tomorrow|today|week|day|\d+\s*(?:day|days|week|weeks))\b/;
const NEGATED_INTENT_PATTERNS = [
  /\b(?:don't|dont|do not|never)\b[\w\s]{0,24}\b(resched(?:ule)?|postpone|delay|push|move)\b/,
  /\b(resched(?:ule)?|postpone|delay|push|move)\b[\w\s]{0,24}\bnot\b/,
];

function hasNegatedRescheduleIntent(text: string) {
  return NEGATED_INTENT_PATTERNS.some((pattern) => pattern.test(text));
}

export function parseRescheduleIntent(prompt: string): number | null {
  const text = prompt.toLowerCase();
  const hasExplicitVerb = EXPLICIT_RESCHEDULE_VERB.test(text);
  const hasSoftVerb =
    SOFT_RESCHEDULE_VERB.test(text) && RESCHEDULE_TARGET_HINT.test(text) && TIME_HINT.test(text);

  if (!hasExplicitVerb && !hasSoftVerb) return null;
  if (hasNegatedRescheduleIntent(text)) return null;

  if (text.includes('week') && !/\d+\s*week/.test(text)) return 7;
  if (text.includes('tomorrow')) return 1;
  if (text.includes('today')) return 1;

  const weekMatch = text.match(/(\d+)\s*week/);
  if (weekMatch?.[1]) return Math.max(1, Number.parseInt(weekMatch[1], 10) * 7);

  const dayMatch = text.match(/(\d+)\s*day/);
  if (dayMatch?.[1]) return Math.max(1, Number.parseInt(dayMatch[1], 10));

  return 1;
}
