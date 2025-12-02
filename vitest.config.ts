import path from 'path';
import { defineConfig } from 'vitest/config';

// Choose test worker pool based on Node version to keep tests stable
// - Node <22: use threads (faster startup)
// - Node >=22: use forks to avoid worker heap fragmentation / OOM issues
const nodeMajor = Number(process.versions.node.split('.')[0] || '20');
const pool: 'threads' | 'forks' = nodeMajor >= 22 ? 'forks' : 'threads';

// For modern Node, pre-set heap for test workers so callers don't need to pass flags
if (nodeMajor >= 22 && !process.env.NODE_OPTIONS?.includes('--max-old-space-size')) {
  const base = process.env.NODE_OPTIONS ? `${process.env.NODE_OPTIONS} ` : '';
  process.env.NODE_OPTIONS = `${base}--max-old-space-size=8192`;
}

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
        // Final targets for coverage gate
        lines: 70,
        functions: 65,
        branches: 55,
        statements: 70,
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
      'lib/generated/**',
    ],

    // Performance configuration
    testTimeout: 10000,
    hookTimeout: 10000,

    // Use a single worker pool tuned per Node runtime for stability
    pool,
    maxWorkers: 1, // Sequential execution for stability across Node versions

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
