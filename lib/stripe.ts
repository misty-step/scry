import Stripe from 'stripe';

let _instance: Stripe | undefined;

export function getStripe(): Stripe {
  if (!_instance) {
    const key = process.env.STRIPE_SECRET_KEY;
    if (!key) throw new Error('STRIPE_SECRET_KEY environment variable is required');
    _instance = new Stripe(key, {
      apiVersion: '2026-01-28.clover',
      typescript: true,
    });
  }
  return _instance;
}
