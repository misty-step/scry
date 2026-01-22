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
        // Raised from 70/65/55/70 - represents significant coverage improvement
        // Branches lower due to complex Convex backend code requiring integration tests
        lines: 70,
        functions: 69, // Pending refactoring of use-review-flow hook for testability
        branches: 67, // Limited by Convex integration code (clerk, concepts, iqc)
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
        'lib/test-utils/**', // Test utilities - not runtime code
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
        // Re-export only files - underlying modules are tested
        'convex/fsrs.ts',
        // No-op stubs for Convex environment
        'convex/lib/analytics.ts',
        // Browser-only utilities (haptics, layout, etc.)
        'lib/haptic.ts',
        'lib/layout-mode.ts',
        // Third-party integrations that require mocking external services
        'lib/sentry.ts',
        'lib/logger.ts', // Depends on Sentry
        // Note: Thin wrapper hooks now included in coverage (tests written)
        // Environment detection utilities (runtime-only)
        'lib/env.ts',
        'lib/posthog-client.ts',
        'lib/deployment-check.ts',
        'lib/error-handlers.ts', // Browser error boundary integration
        // Clerk/theme integration hooks (browser-only)
        'hooks/use-clerk-appearance.ts',
        // Canvas animation hook (visual, best tested via E2E/visual regression)
        'hooks/use-particle-field.ts',
        // Convex config queries (environment variable readers)
        'convex/lib/productionConfig.ts',
        // use-review-flow.ts: reducer is tested directly, hook has complex timer effects tested via E2E
        'hooks/use-review-flow.ts',
        // Browser-only environment detection
        'lib/environment-client.ts',
        // spacedRepetition.ts: thin wrapper, logic tested via simulation, contracts tested in api-contract.test.ts
        'convex/spacedRepetition.ts',
        // Internal Convex mutations/queries (pure database operations)
        'convex/phrasings.ts', // Internal mutations/queries for phrasings table
        // Schema version constant (single line export)
        'convex/schemaVersion.ts',
        // Generated and artifact files
        '**/.gitkeep',
        'convex/_generated/**',
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
