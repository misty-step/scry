import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { formatCardDate, formatDueTime, formatShortRelativeTime } from './date-format';

const fixedNow = new Date('2025-01-08T12:00:00Z');

describe('formatShortRelativeTime', () => {
  beforeAll(() => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
  });

  afterAll(() => {
    vi.useRealTimers();
  });

  it('returns "now" for current timestamps', () => {
    expect(formatShortRelativeTime(Date.now())).toBe('now');
  });

  it('formats past minutes, hours, days, weeks, and months', () => {
    const now = Date.now();
    expect(formatShortRelativeTime(now - 30 * 60 * 1000)).toBe('30m');
    expect(formatShortRelativeTime(now - 3 * 60 * 60 * 1000)).toBe('3h');
    expect(formatShortRelativeTime(now - 3 * 24 * 60 * 60 * 1000)).toBe('3d');
    expect(formatShortRelativeTime(now - 14 * 24 * 60 * 60 * 1000)).toBe('2w');
    expect(formatShortRelativeTime(now - 90 * 24 * 60 * 60 * 1000)).toBe('3mo');
  });

  it('handles future timestamps gracefully', () => {
    const future = Date.now() + 2 * 60 * 60 * 1000;
    expect(formatShortRelativeTime(future)).toBe('in 2h');
  });

  it('handles future timestamps more than 24h away', () => {
    const twoDaysAhead = Date.now() + 2 * 24 * 60 * 60 * 1000;
    expect(formatShortRelativeTime(twoDaysAhead)).toBe('in 2d');
  });

  it('handles future minutes', () => {
    const thirtyMinutesAhead = Date.now() + 30 * 60 * 1000;
    expect(formatShortRelativeTime(thirtyMinutesAhead)).toBe('in 30m');
  });

  it('formats years-old timestamps', () => {
    const twoYearsAgo = Date.now() - 2 * 365 * 24 * 60 * 60 * 1000;
    expect(formatShortRelativeTime(twoYearsAgo)).toBe('2y');
  });
});

describe('formatCardDate', () => {
  beforeAll(() => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
  });

  afterAll(() => {
    vi.useRealTimers();
  });

  it('appends "ago" when requested', () => {
    const oneHourAgo = Date.now() - 60 * 60 * 1000;
    expect(formatCardDate(oneHourAgo, true)).toBe('1h ago');
  });

  it('does not append "ago" for now', () => {
    expect(formatCardDate(Date.now(), true)).toBe('now');
  });

  it('omits "ago" by default', () => {
    const thirtyMinutesAgo = Date.now() - 30 * 60 * 1000;
    expect(formatCardDate(thirtyMinutesAgo)).toBe('30m');
  });
});

describe('formatDueTime', () => {
  beforeAll(() => {
    vi.useFakeTimers();
    vi.setSystemTime(fixedNow);
  });

  afterAll(() => {
    vi.useRealTimers();
  });

  it('returns "Due now" for past or immediate dues', () => {
    expect(formatDueTime(Date.now() - 1000)).toBe('Due now');
    expect(formatDueTime(Date.now())).toBe('Due now');
  });

  it('formats minutes and hours in the future', () => {
    expect(formatDueTime(Date.now() + 15 * 60 * 1000)).toBe('Due in 15m');
    expect(formatDueTime(Date.now() + 3 * 60 * 60 * 1000)).toBe('Due in 3h');
  });

  it('formats tomorrow and future days', () => {
    expect(formatDueTime(Date.now() + 24 * 60 * 60 * 1000 + 1000)).toBe('Due tomorrow');
    expect(formatDueTime(Date.now() + 3 * 24 * 60 * 60 * 1000)).toBe('Due in 3d');
  });

  it('formats dates a week or more away', () => {
    const tenDaysOut = Date.now() + 10 * 24 * 60 * 60 * 1000;
    expect(formatDueTime(tenDaysOut)).toBe('Due Jan 18');
  });
});
