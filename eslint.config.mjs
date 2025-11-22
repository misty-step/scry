import coreWebVitals from 'eslint-config-next/core-web-vitals';
import typescript from 'eslint-config-next/typescript';
import prettier from 'eslint-config-prettier';

const eslintConfig = [
  ...coreWebVitals,
  ...typescript,
  prettier,
  {
    ignores: [
      'lib/generated/**/*',
      '.next/**/*',
      'out/**/*',
      'node_modules/**/*',
      'dist/**/*',
      'build/**/*',
    ],
  },
  {
    // Default rules for all files
    rules: {
      // Allow console.error and console.warn for legitimate error reporting
      // Disallow console.log in production code
      'no-console': ['warn', { allow: ['warn', 'error'] }],
      // Allow variables/args starting with _ to be unused (common pattern for intentionally unused destructured vars)
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          destructuredArrayIgnorePattern: '^_',
        },
      ],
      // TODO: Address React 19 & Next.js 16 stricter rules in separate PR
      // These are pre-existing patterns that need refactoring
      'react-hooks/set-state-in-effect': 'warn', // Downgrade from error to warning
      'react-hooks/purity': 'warn', // Downgrade from error to warning
      'react-hooks/refs': 'warn', // Accessing refs during render - needs refactor
      'react-hooks/preserve-manual-memoization': 'warn', // React Compiler memoization
    },
  },
  {
    // Override for test files and test utilities
    files: [
      '**/*.test.ts',
      '**/*.test.tsx',
      '**/*.spec.ts',
      '**/*.spec.tsx',
      '**/test-utils/**/*.ts',
      '**/test-utils/**/*.tsx',
    ],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-unsafe-function-type': 'off',
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
        },
      ],
      'no-console': 'off', // Allow all console in tests
    },
  },
  {
    // Override for scripts and config files
    files: ['scripts/**/*', '*.config.*', '*.setup.*'],
    rules: {
      'no-console': 'off', // Scripts can use console.log for output
    },
  },
  {
    // Override for Convex files requiring Zod 4 compatibility
    files: ['convex/lib/responsesApi.ts'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off', // Zod 4 type compatibility requires any
    },
  },
];

export default eslintConfig;
