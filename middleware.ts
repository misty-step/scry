import { NextResponse } from 'next/server';
import { clerkMiddleware, createRouteMatcher } from '@clerk/nextjs/server';

const isDesignLabRoute = createRouteMatcher(['/design-lab(.*)']);

export default clerkMiddleware(async (auth, req) => {
  // Block design-lab route in production (dev-only tool)
  if (isDesignLabRoute(req) && process.env.NODE_ENV === 'production') {
    return new NextResponse(null, { status: 404 });
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
