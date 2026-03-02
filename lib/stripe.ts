import Stripe from 'stripe';

let stripeClient: Stripe | null = null;

export function getStripe(): Stripe {
  if (!stripeClient) {
    if (!process.env.STRIPE_SECRET_KEY) {
      throw new Error('STRIPE_SECRET_KEY environment variable is required');
    }
    // Stripe types only allow the latest API version, but we may use different versions
    // across environments. Using type assertion to allow version flexibility.
    stripeClient = new Stripe(process.env.STRIPE_SECRET_KEY, {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      apiVersion: '2026-02-25.clover' as any,
      typescript: true,
    });
  }
  return stripeClient;
}
