import crypto from 'crypto';
import { NextRequest, NextResponse } from 'next/server';
import { createContextLogger } from '@/lib/logger';

export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

const logger = createContextLogger('api');

/**
 * Sentry Webhook Handler
 *
 * Receives issue events from Sentry and triggers a GitHub Action
 * for AI-powered triage and analysis.
 *
 * Flow:
 * 1. Sentry detects new/updated issue
 * 2. Sentry sends webhook to this endpoint
 * 3. We verify the signature and extract issue data
 * 4. We trigger a GitHub repository_dispatch event
 * 5. GitHub Action runs Claude to analyze and create GitHub issue
 */

interface SentryIssueData {
  id: string;
  shortId: string;
  title: string;
  culprit: string;
  level: string;
  status: string;
  platform: string;
  project: {
    id: string;
    name: string;
    slug: string;
  };
  metadata: {
    type?: string;
    value?: string;
    filename?: string;
    function?: string;
  };
  count: string;
  userCount: number;
  firstSeen: string;
  lastSeen: string;
}

interface SentryWebhookPayload {
  action: 'created' | 'resolved' | 'assigned' | 'archived' | 'unresolved';
  installation: {
    uuid: string;
  };
  data: {
    issue: SentryIssueData;
  };
  actor: {
    type: string;
    id?: string;
    name?: string;
  };
}

/**
 * Verify Sentry webhook signature using HMAC-SHA256
 */
function verifySignature(body: string, signature: string | null, secret: string): boolean {
  if (!signature || !secret) {
    return false;
  }

  const hmac = crypto.createHmac('sha256', secret);
  hmac.update(body, 'utf8');
  const digest = hmac.digest('hex');

  // Timing-safe comparison to prevent timing attacks
  try {
    return crypto.timingSafeEqual(Buffer.from(digest), Buffer.from(signature));
  } catch {
    return false;
  }
}

/**
 * Trigger a GitHub repository_dispatch event
 */
async function triggerGitHubAction(issue: SentryIssueData): Promise<boolean> {
  const token = process.env.GITHUB_TOKEN;
  const repo = process.env.GITHUB_REPOSITORY || 'phrazzld/scry';

  if (!token) {
    logger.error({ event: 'sentry_webhook.github_trigger_failed' }, 'GITHUB_TOKEN not configured');
    return false;
  }

  const [owner, repoName] = repo.split('/');

  const response = await fetch(`https://api.github.com/repos/${owner}/${repoName}/dispatches`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      Accept: 'application/vnd.github.v3+json',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      event_type: 'sentry-issue',
      client_payload: {
        issue_id: issue.shortId,
        issue_url: `https://sentry.io/organizations/misty-step/issues/${issue.id}/`,
        title: issue.title,
        level: issue.level,
        culprit: issue.culprit,
        count: issue.count,
        user_count: issue.userCount,
        first_seen: issue.firstSeen,
        last_seen: issue.lastSeen,
        project: issue.project.slug,
        metadata: issue.metadata,
      },
    }),
  });

  if (!response.ok) {
    const errorText = await response.text();
    logger.error(
      {
        event: 'sentry_webhook.github_trigger_failed',
        status: response.status,
        error: errorText,
      },
      'Failed to trigger GitHub Action'
    );
    return false;
  }

  return true;
}

export async function POST(request: NextRequest) {
  const startTime = Date.now();

  try {
    // Get raw body for signature verification
    const rawBody = await request.text();
    const signature = request.headers.get('sentry-hook-signature');
    const resource = request.headers.get('sentry-hook-resource');

    logger.info(
      {
        event: 'sentry_webhook.received',
        resource,
        hasSignature: !!signature,
      },
      'Sentry webhook received'
    );

    // Verify signature
    const clientSecret = process.env.SENTRY_WEBHOOK_SECRET;
    if (!clientSecret) {
      logger.error(
        { event: 'sentry_webhook.config_error' },
        'SENTRY_WEBHOOK_SECRET not configured'
      );
      return NextResponse.json({ error: 'Webhook not configured' }, { status: 500 });
    }

    if (!verifySignature(rawBody, signature, clientSecret)) {
      logger.warn({ event: 'sentry_webhook.invalid_signature' }, 'Invalid webhook signature');
      return NextResponse.json({ error: 'Invalid signature' }, { status: 401 });
    }

    // Parse the payload
    const payload: SentryWebhookPayload = JSON.parse(rawBody);

    // Only process issue events
    if (resource !== 'issue') {
      logger.debug({ event: 'sentry_webhook.skipped', resource }, 'Skipping non-issue webhook');
      return NextResponse.json({ status: 'skipped', reason: 'not an issue event' });
    }

    // Only trigger for new issues (not resolved, assigned, etc.)
    if (payload.action !== 'created') {
      logger.debug(
        { event: 'sentry_webhook.skipped', action: payload.action },
        'Skipping non-create action'
      );
      return NextResponse.json({ status: 'skipped', reason: `action is ${payload.action}` });
    }

    const issue = payload.data.issue;

    logger.info(
      {
        event: 'sentry_webhook.processing',
        issueId: issue.shortId,
        title: issue.title,
        level: issue.level,
      },
      `Processing Sentry issue: ${issue.shortId}`
    );

    // Trigger GitHub Action for AI triage
    const triggered = await triggerGitHubAction(issue);

    const duration = Date.now() - startTime;

    if (triggered) {
      logger.info(
        {
          event: 'sentry_webhook.success',
          issueId: issue.shortId,
          duration,
        },
        `GitHub Action triggered for ${issue.shortId}`
      );
      return NextResponse.json({
        status: 'success',
        issue_id: issue.shortId,
        github_action_triggered: true,
      });
    } else {
      return NextResponse.json(
        {
          status: 'partial',
          issue_id: issue.shortId,
          github_action_triggered: false,
          error: 'Failed to trigger GitHub Action',
        },
        { status: 500 }
      );
    }
  } catch (error) {
    const duration = Date.now() - startTime;
    logger.error(
      {
        event: 'sentry_webhook.error',
        error: error instanceof Error ? error.message : String(error),
        duration,
      },
      'Sentry webhook processing failed'
    );

    return NextResponse.json({ error: 'Internal server error' }, { status: 500 });
  }
}

// Health check for the webhook endpoint
export async function GET() {
  return NextResponse.json({
    status: 'ok',
    endpoint: 'sentry-webhook',
    configured: !!process.env.SENTRY_WEBHOOK_SECRET && !!process.env.GITHUB_TOKEN,
  });
}
