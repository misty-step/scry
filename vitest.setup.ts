import React from 'react';
import { cleanup } from '@testing-library/react';
import { afterEach, vi } from 'vitest';
import '@testing-library/jest-dom/vitest';

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

// Silence Sentry/analytics during unit tests (must use vi.mock for ESM modules)
vi.mock('@sentry/nextjs', () => ({
  captureException: vi.fn(() => ''),
  captureMessage: vi.fn(() => ''),
  captureEvent: vi.fn(() => ''),
  captureCheckIn: vi.fn(() => ''),
  setUser: vi.fn(),
}));
vi.mock('@/convex/lib/analytics', () => ({
  trackEvent: vi.fn(),
}));

// Global test utilities
class ResizeObserverMock {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
// eslint-disable-next-line @typescript-eslint/no-explicit-any
global.ResizeObserver = ResizeObserverMock as any;
