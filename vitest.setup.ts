import React from 'react';
import { cleanup } from '@testing-library/react';
import { afterEach, vi } from 'vitest';
import '@testing-library/jest-dom/vitest';
import * as Sentry from '@sentry/nextjs';
// Make React globally available for JSX in tests
globalThis.React = React;

// Cleanup after each test
afterEach(() => {
  cleanup();
});

// Mock Convex for testing
vi.mock('convex/react', () => ({
  useQuery: vi.fn(),
  useMutation: vi.fn(),
  useAction: vi.fn(),
}));

// Silence Sentry/analytics during unit tests
const noop = () => undefined;
vi.spyOn(Sentry, 'captureException').mockImplementation(noop as typeof Sentry.captureException);
vi.spyOn(Sentry, 'captureMessage').mockImplementation(noop as typeof Sentry.captureMessage);
vi.spyOn(Sentry, 'captureEvent').mockImplementation(noop as typeof Sentry.captureEvent);
vi.spyOn(Sentry, 'captureCheckIn').mockImplementation(noop as typeof Sentry.captureCheckIn);
vi.mock('@/convex/lib/analytics', () => ({
  trackEvent: vi.fn(),
}));

// Global test utilities
global.ResizeObserver = vi.fn().mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));
