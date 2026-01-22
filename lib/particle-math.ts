/**
 * Pure math functions for particle field calculations.
 * Extracted for testability - the hook uses these internally.
 */

export interface Point {
  x: number;
  y: number;
}

/**
 * Calculate squared distance between two points.
 * Avoids expensive sqrt for distance comparisons.
 */
export function distanceSquared(p1: Point, p2: Point): number {
  const dx = p1.x - p2.x;
  const dy = p1.y - p2.y;
  return dx * dx + dy * dy;
}

/**
 * Check if two particles should be connected.
 * Uses squared distance for performance.
 */
export function shouldConnect(p1: Point, p2: Point, maxDistanceSq: number): boolean {
  return distanceSquared(p1, p2) < maxDistanceSq;
}

/**
 * Calculate connection line alpha based on distance.
 * Closer particles = stronger connection (higher alpha).
 */
export function calculateConnectionAlpha(
  distance: number,
  maxDistance: number,
  baseAlpha: number
): number {
  return (1 - distance / maxDistance) * baseAlpha;
}

/**
 * Wrap a position around canvas edges (toroidal wrapping).
 */
export function wrapPosition(pos: number, min: number, max: number): number {
  if (pos < min) return max;
  if (pos > max) return min;
  return pos;
}

/**
 * Generate random value in range [min, max).
 */
export function randomInRange(min: number, max: number): number {
  return Math.random() * (max - min) + min;
}

/**
 * Generate random velocity component in range [-v/2, v/2).
 */
export function randomVelocity(maxVelocity: number): number {
  return (Math.random() - 0.5) * maxVelocity;
}
