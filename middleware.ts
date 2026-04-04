import { clerkMiddleware, createRouteMatcher } from '@clerk/nextjs/server';

const isPublicApiRoute = createRouteMatcher(['/api/stripe/webhook', '/api/health(.*)']);

export default clerkMiddleware(async (auth, req) => {
  // Skip auth for public API routes (they have their own auth mechanisms)
  // Webhook uses Stripe signature verification, health is public
  if (isPublicApiRoute(req)) {
    return;
  }
});

export const config = {
  matcher: [
    // Skip Next.js internals and all static files, unless found in search params
    '/((?!_next|[^?]*\\.(?:html?|css|js(?!on)|jpe?g|webp|png|gif|svg|ttf|woff2?|ico|csv|docx?|xlsx?|zip|webmanifest)).*)',
    // Always run for API routes
    '/(api|trpc)(.*)',
  ],
};
