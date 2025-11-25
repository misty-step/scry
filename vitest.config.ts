import path from 'path';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // Global test settings
    globals: true,

    // Use happy-dom for React component/hook testing
    environment: 'happy-dom',

    // Setup file for React Testing Library
    setupFiles: ['./vitest.setup.ts'],

    // Coverage configuration
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'json-summary', 'html', 'lcov'], // Add lcov for Codecov

      // Coverage thresholds
      // CURRENT STATE: ~28% global (coverage/coverage-summary.json on 2025-11-24)
      // FLOOR (this file): 27% lines/statements, 26% functions, 22% branches; convex 25% lines/functions
      // TARGET: 60%+ (Google research: 60% acceptable, 75% commendable)
      //
      // Improvement plan tracked in BACKLOG.md "Test Coverage Improvement Initiative"
      // Thresholds only ratchet upward; never decrease.
      // Per-path thresholds for critical areas; keep globals as floor.
      thresholds: {
        lines: 27,
        functions: 26,
        branches: 22,
        statements: 27,
        'convex/**/*.ts': { lines: 25, functions: 25 },
        'lib/payment/**/*.ts': { lines: 80, functions: 80 },
        'lib/auth/**/*.ts': { lines: 80, functions: 80 },
      },
      include: ['lib/**', 'convex/**', 'hooks/**'],
      exclude: [
        'node_modules/',
        'dist/',
        '.next/',
        '**/*.d.ts',
        '**/*.config.*',
        '**/test/**',
        '**/tests/**',
        'lib/generated/**',
        'scripts/**',
        // Non-runtime / docs / artifacts that skew coverage
        'convex/**/*.md',
        'convex/**/README.*',
        'convex/**/TYPES.*',
        'convex/**/*.backup',
        'convex/**/*.tsbuildinfo',
        'convex/**/tsconfig.*',
        'convex/migrations/**',
        'convex/evals/**',
        'convex/cron.ts',
        'convex/deployments.ts',
        'convex/health.ts',
        'convex/http.ts',
        'convex/lab.ts',
        'convex/phrasings.ts',
        'convex/scheduling.ts',
        'convex/schema.ts',
        'convex/system.ts',
        'convex/types.ts',
        'convex/fsrs.ts',
      ],
    },

    // Test organization
    include: ['**/*.{test,spec}.{js,ts,jsx,tsx}'],
    exclude: [
      'node_modules/',
      'dist/',
      '.next/',
      'tests/e2e/**', // Keep Playwright E2E tests separate
      'lib/generated/**',
    ],

    // Performance configuration
    testTimeout: 10000,
    hookTimeout: 10000,

    // Enable parallel test execution with Vitest 4 configuration
    pool: 'forks',
    // Single forked worker avoids worker_threads heap limit issues observed in hooks
    maxWorkers: 1,

    // Show test timing to identify slow tests
    reporters: ['verbose'],
  },

  // Path resolution for Next.js aliases
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '.'),
    },
  },
});
