# E2E Testing

This directory contains end-to-end (E2E) tests for the Scry application using Playwright.

## Test Coverage

### Authentication Flow Tests (`e2e/auth.test.ts`)

The authentication tests cover the complete email-based authentication flow:

1. **Successful Email Submission**: Tests valid email submission and redirect to verification page
2. **Email Validation**: Tests client-side validation for invalid email formats and empty fields
3. **Multi-Domain Support**: Tests authentication with different email domains (.com, .org, .gmail, etc.)
4. **Error Handling**: Tests graceful handling of potentially blocked domains
5. **CSP Compliance**: Verifies no Content Security Policy violations during auth flow
6. **Form State Management**: Tests form state preservation during validation errors
7. **Navigation Flow**: Tests proper page navigation and back button functionality
8. **Rate Limiting**: Tests handling of rapid form submissions

## Running Tests

### Prerequisites

Install Playwright browsers (Chromium + Mobile Chrome only):
```bash
pnpm exec playwright install --with-deps chromium
```

### Test Commands

```bash
# Run all E2E tests (full)
pnpm test:e2e:full

# Run smoke subset (tagged @smoke)
pnpm test:e2e:smoke

# Headed / debug
pnpm exec playwright test --headed
pnpm exec playwright test --debug

# HTML report viewer
pnpm exec playwright show-report
```

### Test Reports

After running tests, view the HTML report:
```bash
pnpm exec playwright show-report
```

## Test Configuration

Tests are configured via `playwright.config.ts` with:

- **Browsers**: Chromium (Desktop) and Mobile Chrome only (Firefox/WebKit removed for speed)
- **Base URL**: https://scry.vercel.app on CI; http://localhost:3000 locally when `pnpm dev` running
- **Timeouts**: 10s actions, 30s navigation, 60s global
- **Retries**: 0 (fix flake instead of retry)
- **Artifacts**: Screenshots/videos on failure, traces on first retry hook (smoke uses same defaults)

## Test Structure

```
tests/
├── e2e/
│   ├── spaced-repetition.test.ts   # Full regression flow
│   ├── spaced-repetition.local.test.ts
│   ├── library-search-race-condition.test.ts
│   └── review-next-button-fix.test.ts
└── README.md                       # This documentation
```

## Writing New Tests

When adding new E2E tests:

1. Create test files in `tests/e2e/`
2. Use the existing authentication tests as a template
3. Follow the test structure: describe → beforeEach → test cases
4. Use proper assertions with `expect()`
5. Include both positive and negative test cases
6. Test error scenarios and edge cases
7. Verify no CSP violations or console errors

## CI/CD Integration

Tests are designed to run in CI/CD pipelines:

- Smoke (`test:e2e:smoke`) runs on preview deploy workflow against Vercel preview URL.
- Full (`test:e2e:full`) runs nightly against production and on demand via workflow dispatch.
- HTML reports are uploaded as artifacts; failures include screenshots/videos.

## Debugging Tests

For debugging failed tests:

1. Run with `--headed` to see browser actions
2. Use `--debug` to step through tests
3. Check screenshots and videos in `test-results/`
4. View traces in Playwright UI with `pnpm test:ui`
5. Check console output for CSP violations or errors

## Best Practices

- Tests should be independent and not rely on each other
- Use proper selectors (role, label, text) over CSS selectors
- Include proper wait conditions for async operations
- Test both happy path and error scenarios
- Keep tests focused and atomic
- Use descriptive test names and organize with describe blocks
