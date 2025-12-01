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
      reporter: ['text', 'json', 'json-summary', 'html'],
      reportOnFailure: true,
      thresholds: {
        // Ratcheted to actual coverage (43.8%) - increase as tests improve
        lines: 43.8,
        functions: 38.3,
        branches: 34.8,
        statements: 43.6,
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
        'convex/schema.ts',
        'convex/system.ts',
        'convex/types.ts',
      ],
    },

    // Test organization
    include: ['**/*.{test,spec}.{js,ts,jsx,tsx}'],
    exclude: [
      'node_modules/',
      'dist/',
      '.next/',
      'tests/e2e/**', // Keep Playwright E2E tests separate
      'tests/perf/**', // Run via pnpm test:perf (large fixtures)
      'lib/generated/**',
    ],

    // Performance configuration
    testTimeout: 10000,
    hookTimeout: 10000,

    // Use threads pool with memory limit for worker stability
    pool: 'threads',
    maxWorkers: 1, // Sequential execution for stability

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
