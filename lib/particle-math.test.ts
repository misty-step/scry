import { describe, expect, it, vi } from 'vitest';
import {
  calculateConnectionAlpha,
  distanceSquared,
  randomInRange,
  randomVelocity,
  shouldConnect,
  wrapPosition,
} from './particle-math';

describe('particle-math', () => {
  describe('distanceSquared', () => {
    it('returns 0 for same point', () => {
      expect(distanceSquared({ x: 5, y: 5 }, { x: 5, y: 5 })).toBe(0);
    });

    it('returns squared distance for horizontal separation', () => {
      expect(distanceSquared({ x: 0, y: 0 }, { x: 3, y: 0 })).toBe(9);
    });

    it('returns squared distance for vertical separation', () => {
      expect(distanceSquared({ x: 0, y: 0 }, { x: 0, y: 4 })).toBe(16);
    });

    it('returns squared distance for diagonal separation (3-4-5 triangle)', () => {
      expect(distanceSquared({ x: 0, y: 0 }, { x: 3, y: 4 })).toBe(25);
    });

    it('handles negative coordinates', () => {
      expect(distanceSquared({ x: -3, y: -4 }, { x: 0, y: 0 })).toBe(25);
    });
  });

  describe('shouldConnect', () => {
    it('returns true when distance is less than max', () => {
      const p1 = { x: 0, y: 0 };
      const p2 = { x: 3, y: 4 }; // distance = 5, squared = 25
      expect(shouldConnect(p1, p2, 100)).toBe(true); // maxDist = 10
    });

    it('returns false when distance equals max', () => {
      const p1 = { x: 0, y: 0 };
      const p2 = { x: 3, y: 4 }; // distance = 5, squared = 25
      expect(shouldConnect(p1, p2, 25)).toBe(false); // exactly at threshold
    });

    it('returns false when distance exceeds max', () => {
      const p1 = { x: 0, y: 0 };
      const p2 = { x: 10, y: 10 }; // squared = 200
      expect(shouldConnect(p1, p2, 100)).toBe(false);
    });

    it('returns true for same point', () => {
      const p = { x: 5, y: 5 };
      expect(shouldConnect(p, p, 1)).toBe(true);
    });
  });

  describe('calculateConnectionAlpha', () => {
    it('returns baseAlpha at distance 0', () => {
      expect(calculateConnectionAlpha(0, 100, 0.5)).toBe(0.5);
    });

    it('returns 0 at max distance', () => {
      expect(calculateConnectionAlpha(100, 100, 0.5)).toBe(0);
    });

    it('returns half baseAlpha at half distance', () => {
      expect(calculateConnectionAlpha(50, 100, 0.5)).toBe(0.25);
    });

    it('returns 75% baseAlpha at 25% distance', () => {
      expect(calculateConnectionAlpha(25, 100, 1.0)).toBe(0.75);
    });
  });

  describe('wrapPosition', () => {
    it('wraps below min to max', () => {
      expect(wrapPosition(-1, 0, 100)).toBe(100);
    });

    it('wraps above max to min', () => {
      expect(wrapPosition(101, 0, 100)).toBe(0);
    });

    it('returns position unchanged when in bounds', () => {
      expect(wrapPosition(50, 0, 100)).toBe(50);
    });

    it('returns position unchanged at min boundary', () => {
      expect(wrapPosition(0, 0, 100)).toBe(0);
    });

    it('returns position unchanged at max boundary', () => {
      expect(wrapPosition(100, 0, 100)).toBe(100);
    });
  });

  describe('randomInRange', () => {
    it('returns values within range', () => {
      vi.spyOn(Math, 'random').mockReturnValue(0.5);
      expect(randomInRange(10, 20)).toBe(15);
      vi.restoreAllMocks();
    });

    it('returns min when random is 0', () => {
      vi.spyOn(Math, 'random').mockReturnValue(0);
      expect(randomInRange(10, 20)).toBe(10);
      vi.restoreAllMocks();
    });

    it('approaches max when random approaches 1', () => {
      vi.spyOn(Math, 'random').mockReturnValue(0.999);
      expect(randomInRange(10, 20)).toBeCloseTo(19.99, 1);
      vi.restoreAllMocks();
    });
  });

  describe('randomVelocity', () => {
    it('returns 0 when random is 0.5', () => {
      vi.spyOn(Math, 'random').mockReturnValue(0.5);
      expect(randomVelocity(10)).toBe(0);
      vi.restoreAllMocks();
    });

    it('returns negative half max when random is 0', () => {
      vi.spyOn(Math, 'random').mockReturnValue(0);
      expect(randomVelocity(10)).toBe(-5);
      vi.restoreAllMocks();
    });

    it('approaches positive half max when random approaches 1', () => {
      vi.spyOn(Math, 'random').mockReturnValue(0.999);
      expect(randomVelocity(10)).toBeCloseTo(4.99, 1);
      vi.restoreAllMocks();
    });
  });
});
