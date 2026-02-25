import { describe, expect, it } from 'vitest';
import { parseRescheduleIntent } from './review-intent';

describe('parseRescheduleIntent', () => {
  it('returns null for unrelated chat input', () => {
    expect(parseRescheduleIntent('Explain this concept again')).toBeNull();
  });

  it('parses day and week intervals from explicit reschedule requests', () => {
    expect(parseRescheduleIntent('Please reschedule this card in 3 days')).toBe(3);
    expect(parseRescheduleIntent('Postpone this review by 2 weeks')).toBe(14);
    expect(parseRescheduleIntent('Reschedule this next week')).toBe(7);
  });

  it('defaults to 1 day for explicit reschedule intent without an interval', () => {
    expect(parseRescheduleIntent('Can you reschedule this?')).toBe(1);
  });

  it('ignores negated reschedule requests', () => {
    expect(parseRescheduleIntent("Don't reschedule this")).toBeNull();
    expect(parseRescheduleIntent('Do not postpone this card')).toBeNull();
  });

  it('avoids move/push false positives when no timing hint is present', () => {
    expect(parseRescheduleIntent('Can we move on to the next question?')).toBeNull();
  });

  it('supports move/push phrasing when tied to review timing', () => {
    expect(parseRescheduleIntent('Can we move this review by 3 days?')).toBe(3);
    expect(parseRescheduleIntent('Push this question to tomorrow')).toBe(1);
  });
});
