import crypto from 'crypto';
import { NextRequest } from 'next/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
// Import after mocking
import { GET, POST } from '@/app/api/webhooks/sentry/route';

// Mock the logger before importing the route
vi.mock('@/lib/logger', () => ({
  createContextLogger: () => ({
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
    debug: vi.fn(),
  }),
}));

// Mock fetch for GitHub API calls
const mockFetch = vi.fn();
global.fetch = mockFetch;

const MOCK_SECRET = 'test-webhook-secret';
const MOCK_GITHUB_TOKEN = 'ghp_test_token';

/**
 * Helper to create a valid Sentry webhook signature
 */
function createSignature(body: string, secret: string): string {
  const hmac = crypto.createHmac('sha256', secret);
  hmac.update(body, 'utf8');
  return hmac.digest('hex');
}

/**
 * Helper to create a mock Sentry webhook payload
 */
function createMockPayload(overrides: Record<string, unknown> = {}) {
  return {
    action: 'created',
    installation: { uuid: 'test-uuid' },
    data: {
      issue: {
        id: '123456789',
        shortId: 'SCRY-1A',
        title: 'Test Error: Something went wrong',
        culprit: 'app/api/test/route.ts',
        level: 'error',
        status: 'unresolved',
        platform: 'javascript',
        project: {
          id: 'proj-123',
          name: 'scry',
          slug: 'scry',
        },
        metadata: {
          type: 'Error',
          value: 'Something went wrong',
        },
        count: '5',
        userCount: 2,
        firstSeen: '2026-01-24T20:00:00Z',
        lastSeen: '2026-01-24T21:00:00Z',
      },
    },
    actor: {
      type: 'application',
    },
    ...overrides,
  };
}

/**
 * Helper to create a NextRequest with proper headers and body
 */
function createRequest(
  body: string,
  options: {
    signature?: string | null;
    resource?: string;
  } = {}
): NextRequest {
  const headers = new Headers();
  if (options.signature !== null) {
    headers.set('sentry-hook-signature', options.signature || '');
  }
  headers.set('sentry-hook-resource', options.resource || 'issue');
  headers.set('content-type', 'application/json');

  return new NextRequest('http://localhost/api/webhooks/sentry', {
    method: 'POST',
    headers,
    body,
  });
}

describe('/api/webhooks/sentry', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    process.env.SENTRY_WEBHOOK_SECRET = MOCK_SECRET;
    process.env.GITHUB_TOKEN = MOCK_GITHUB_TOKEN;
    process.env.GITHUB_REPOSITORY = 'test-owner/test-repo';
    process.env.SENTRY_ORG = 'test-org';

    // Default successful GitHub API response
    mockFetch.mockResolvedValue({
      ok: true,
      status: 204,
      text: async () => '',
    });
  });

  afterEach(() => {
    delete process.env.SENTRY_WEBHOOK_SECRET;
    delete process.env.GITHUB_TOKEN;
    delete process.env.GITHUB_REPOSITORY;
    delete process.env.SENTRY_ORG;
  });

  describe('signature verification', () => {
    it('accepts valid signatures', async () => {
      const payload = createMockPayload();
      const body = JSON.stringify(payload);
      const signature = createSignature(body, MOCK_SECRET);

      const request = createRequest(body, { signature });
      const response = await POST(request);
      const data = await response.json();

      expect(response.status).toBe(200);
      expect(data.status).toBe('success');
      expect(data.github_action_triggered).toBe(true);
    });

    it('rejects invalid signatures', async () => {
      const payload = createMockPayload();
      const body = JSON.stringify(payload);
      const invalidSignature = createSignature(body, 'wrong-secret');

      const request = createRequest(body, { signature: invalidSignature });
      const response = await POST(request);
      const data = await response.json();

      expect(response.status).toBe(401);
      expect(data.error).toBe('Invalid signature');
    });

    it('rejects missing signatures', async () => {
      const payload = createMockPayload();
      const body = JSON.stringify(payload);

      const request = createRequest(body, { signature: null });
      const response = await POST(request);
      const data = await response.json();

      expect(response.status).toBe(401);
      expect(data.error).toBe('Invalid signature');
    });

    it('rejects signatures with wrong length (length oracle protection)', async () => {
      const payload = createMockPayload();
      const body = JSON.stringify(payload);
      // SHA256 hex digest is 64 chars, use wrong length
      const wrongLengthSignature = 'abc123';

      const request = createRequest(body, { signature: wrongLengthSignature });
      const response = await POST(request);
      const data = await response.json();

      expect(response.status).toBe(401);
      expect(data.error).toBe('Invalid signature');
    });
  });

  describe('payload validation', () => {
    it('accepts valid issue created events', async () => {
      const payload = createMockPayload({ action: 'created' });
      const body = JSON.stringify(payload);
      const signature = createSignature(body, MOCK_SECRET);

      const request = createRequest(body, { signature });
      const response = await POST(request);
      const data = await response.json();

      expect(response.status).toBe(200);
      expect(data.status).toBe('success');
    });

    it('skips non-issue events', async () => {
      const payload = createMockPayload();
      const body = JSON.stringify(payload);
      const signature = createSignature(body, MOCK_SECRET);

      const request = createRequest(body, { signature, resource: 'metric_alert' });
      const response = await POST(request);
      const data = await response.json();

      expect(response.status).toBe(200);
      expect(data.status).toBe('skipped');
      expect(data.reason).toBe('not an issue event');
    });

    it('skips non-created actions', async () => {
      const payload = createMockPayload({ action: 'resolved' });
      const body = JSON.stringify(payload);
      const signature = createSignature(body, MOCK_SECRET);

      const request = createRequest(body, { signature });
      const response = await POST(request);
      const data = await response.json();

      expect(response.status).toBe(200);
      expect(data.status).toBe('skipped');
      expect(data.reason).toBe('action is resolved');
    });

    it('rejects malformed payloads', async () => {
      const payload = { action: 'created', data: {} }; // Missing issue
      const body = JSON.stringify(payload);
      const signature = createSignature(body, MOCK_SECRET);

      const request = createRequest(body, { signature });
      const response = await POST(request);
      const data = await response.json();

      expect(response.status).toBe(400);
      expect(data.error).toBe('Invalid payload');
    });
  });

  describe('GitHub integration', () => {
    it('triggers repository_dispatch with correct payload', async () => {
      const payload = createMockPayload();
      const body = JSON.stringify(payload);
      const signature = createSignature(body, MOCK_SECRET);

      const request = createRequest(body, { signature });
      await POST(request);

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.github.com/repos/test-owner/test-repo/dispatches',
        expect.objectContaining({
          method: 'POST',
          headers: expect.objectContaining({
            Authorization: `Bearer ${MOCK_GITHUB_TOKEN}`,
          }),
        })
      );

      // Verify payload content
      const fetchCall = mockFetch.mock.calls[0];
      const fetchBody = JSON.parse(fetchCall[1].body);
      expect(fetchBody.event_type).toBe('sentry-issue');
      expect(fetchBody.client_payload.issue_id).toBe('SCRY-1A');
      expect(fetchBody.client_payload.issue_url).toContain('test-org');
    });

    it('handles GitHub API errors', async () => {
      mockFetch.mockResolvedValue({
        ok: false,
        status: 403,
        text: async () => 'Forbidden',
      });

      const payload = createMockPayload();
      const body = JSON.stringify(payload);
      const signature = createSignature(body, MOCK_SECRET);

      const request = createRequest(body, { signature });
      const response = await POST(request);
      const data = await response.json();

      expect(response.status).toBe(500);
      expect(data.status).toBe('partial');
      expect(data.github_action_triggered).toBe(false);
    });

    it('handles GitHub API timeouts', async () => {
      const abortError = new Error('Aborted');
      abortError.name = 'AbortError';
      mockFetch.mockRejectedValue(abortError);

      const payload = createMockPayload();
      const body = JSON.stringify(payload);
      const signature = createSignature(body, MOCK_SECRET);

      const request = createRequest(body, { signature });
      const response = await POST(request);
      const data = await response.json();

      expect(response.status).toBe(500);
      expect(data.status).toBe('partial');
      expect(data.github_action_triggered).toBe(false);
    });
  });

  describe('error handling', () => {
    it('returns 500 on missing SENTRY_WEBHOOK_SECRET', async () => {
      delete process.env.SENTRY_WEBHOOK_SECRET;

      const payload = createMockPayload();
      const body = JSON.stringify(payload);

      const request = createRequest(body, { signature: 'any' });
      const response = await POST(request);
      const data = await response.json();

      expect(response.status).toBe(500);
      expect(data.error).toBe('Webhook not configured');
    });

    it('handles invalid GITHUB_REPOSITORY format', async () => {
      process.env.GITHUB_REPOSITORY = 'invalid-format-no-slash';

      const payload = createMockPayload();
      const body = JSON.stringify(payload);
      const signature = createSignature(body, MOCK_SECRET);

      const request = createRequest(body, { signature });
      const response = await POST(request);
      const data = await response.json();

      expect(response.status).toBe(500);
      expect(data.github_action_triggered).toBe(false);
    });
  });

  describe('health check', () => {
    it('returns ok status when configured', async () => {
      const response = await GET();
      const data = await response.json();

      expect(response.status).toBe(200);
      expect(data.status).toBe('ok');
      expect(data.endpoint).toBe('sentry-webhook');
      expect(data.configured).toBe(true);
    });

    it('returns configured false when secrets missing', async () => {
      delete process.env.SENTRY_WEBHOOK_SECRET;
      delete process.env.GITHUB_TOKEN;

      const response = await GET();
      const data = await response.json();

      expect(data.configured).toBe(false);
    });
  });
});
