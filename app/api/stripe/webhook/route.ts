import { headers } from 'next/headers';
import { NextResponse } from 'next/server';
import { ConvexHttpClient } from 'convex/browser';
import Stripe from 'stripe';
import { api } from '@/convex/_generated/api';
import { getStripe } from '@/lib/stripe';

const convex = new ConvexHttpClient(process.env.NEXT_PUBLIC_CONVEX_URL!);

export async function POST(req: Request) {
  const body = await req.text();
  const headersList = await headers();
  const signature = headersList.get('stripe-signature');

  if (!signature) {
    console.error('Missing stripe-signature header');
    return NextResponse.json({ error: 'Missing signature' }, { status: 400 });
  }

  const webhookSecret = process.env.STRIPE_WEBHOOK_SECRET;
  if (!webhookSecret) {
    console.error('STRIPE_WEBHOOK_SECRET not configured');
    return NextResponse.json({ error: 'Webhook not configured' }, { status: 500 });
  }

  // Validate CONVEX_WEBHOOK_SECRET exists
  const convexWebhookSecret = process.env.CONVEX_WEBHOOK_SECRET;
  if (!convexWebhookSecret) {
    console.error('CONVEX_WEBHOOK_SECRET not configured');
    return NextResponse.json({ error: 'Convex webhook secret not configured' }, { status: 500 });
  }

  // FIRST: Verify signature (fail-fast)
  let event: Stripe.Event;
  try {
    event = getStripe().webhooks.constructEvent(body, signature, webhookSecret);
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Unknown error';
    console.error('Webhook signature verification failed:', message);
    return NextResponse.json({ error: 'Invalid signature' }, { status: 400 });
  }

  // Process the event
  try {
    switch (event.type) {
      case 'checkout.session.completed': {
        const session = event.data.object as Stripe.Checkout.Session;
        await handleCheckoutCompleted(session, convexWebhookSecret);
        break;
      }

      case 'customer.subscription.created':
      case 'customer.subscription.updated': {
        const subscription = event.data.object as Stripe.Subscription;
        await handleSubscriptionChange(subscription, convexWebhookSecret);
        break;
      }

      case 'customer.subscription.deleted': {
        const subscription = event.data.object as Stripe.Subscription;
        await handleSubscriptionDeleted(subscription, convexWebhookSecret);
        break;
      }

      case 'invoice.paid': {
        const invoice = event.data.object as Stripe.Invoice;
        console.warn('Invoice paid:', invoice.id);
        // Could update subscription status or send confirmation
        break;
      }

      case 'invoice.payment_failed': {
        const invoice = event.data.object as Stripe.Invoice;
        console.error('Invoice payment failed:', invoice.id);
        // Subscription status will be updated via subscription.updated event
        break;
      }

      default:
        console.warn('Unhandled event type:', event.type);
    }

    return NextResponse.json({ received: true });
  } catch (error) {
    console.error('Webhook processing error:', error);
    // Return 200 to prevent Stripe retries for processing errors
    // The error is logged for debugging
    return NextResponse.json({ received: true, error: 'Processing failed' });
  }
}

async function handleCheckoutCompleted(
  session: Stripe.Checkout.Session,
  convexWebhookSecret: string
) {
  if (session.mode !== 'subscription') {
    return;
  }

  const clerkUserId = session.client_reference_id;
  if (!clerkUserId) {
    console.error('No client_reference_id in checkout session');
    return;
  }

  const subscriptionId = session.subscription as string;
  if (!subscriptionId) {
    console.error('No subscription in checkout session');
    return;
  }

  // Fetch full subscription details
  const subscription = await getStripe().subscriptions.retrieve(subscriptionId);
  await syncSubscriptionToConvex(subscription, clerkUserId, convexWebhookSecret);
}

async function handleSubscriptionChange(
  subscription: Stripe.Subscription,
  convexWebhookSecret: string
) {
  // Get clerkUserId from metadata
  const clerkUserId = subscription.metadata?.clerkUserId;
  if (!clerkUserId) {
    console.error('No clerkUserId in subscription metadata');
    return;
  }

  await syncSubscriptionToConvex(subscription, clerkUserId, convexWebhookSecret);
}

async function handleSubscriptionDeleted(
  subscription: Stripe.Subscription,
  convexWebhookSecret: string
) {
  const clerkUserId = subscription.metadata?.clerkUserId;
  if (!clerkUserId) {
    console.error('No clerkUserId in subscription metadata for deletion');
    return;
  }

  // Sync with canceled status
  await syncSubscriptionToConvex(subscription, clerkUserId, convexWebhookSecret);
}

async function syncSubscriptionToConvex(
  subscription: Stripe.Subscription,
  clerkUserId: string,
  convexWebhookSecret: string
) {
  const subscriptionItem = subscription.items.data[0];
  const priceId = subscriptionItem?.price?.id;
  if (!priceId) {
    console.error('No price ID in subscription');
    return;
  }

  // Map Stripe status to our status type
  const status = mapStripeStatus(subscription.status);

  // Current period is on subscription item in newer Stripe SDK
  const currentPeriodStart = subscriptionItem.current_period_start;
  const currentPeriodEnd = subscriptionItem.current_period_end;

  await convex.mutation(api.subscriptions.syncSubscription, {
    webhookSecret: convexWebhookSecret,
    clerkUserId,
    stripeCustomerId: subscription.customer as string,
    stripeSubscriptionId: subscription.id,
    stripePriceId: priceId,
    status,
    trialStart: subscription.trial_start ? subscription.trial_start * 1000 : undefined,
    trialEnd: subscription.trial_end ? subscription.trial_end * 1000 : undefined,
    currentPeriodStart: currentPeriodStart * 1000,
    currentPeriodEnd: currentPeriodEnd * 1000,
    cancelAtPeriodEnd: subscription.cancel_at_period_end,
    canceledAt: subscription.canceled_at ? subscription.canceled_at * 1000 : undefined,
  });

  console.warn('Synced subscription:', subscription.id, 'status:', status);
}

function mapStripeStatus(
  stripeStatus: Stripe.Subscription.Status
):
  | 'trialing'
  | 'active'
  | 'past_due'
  | 'canceled'
  | 'unpaid'
  | 'incomplete'
  | 'incomplete_expired' {
  switch (stripeStatus) {
    case 'trialing':
      return 'trialing';
    case 'active':
      return 'active';
    case 'past_due':
      return 'past_due';
    case 'canceled':
      return 'canceled';
    case 'unpaid':
      return 'unpaid';
    case 'incomplete':
      return 'incomplete';
    case 'incomplete_expired':
      return 'incomplete_expired';
    default:
      // Handle paused or any new status as canceled
      return 'canceled';
  }
}
