'use client';

import Link from 'next/link';
import { Authenticated, Unauthenticated } from 'convex/react';
import { ReviewChat } from '@/components/agent/review-chat';
import { Button } from '@/components/ui/button';

export default function Home() {
  return (
    <>
      <Authenticated>
        <div className="h-[calc(100dvh-var(--navbar-height))] overflow-hidden">
          <ReviewChat />
        </div>
      </Authenticated>
      <Unauthenticated>
        <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 px-6 text-center">
          <div className="space-y-2">
            <h1 className="text-2xl font-semibold tracking-tight">Sign in to start reviewing</h1>
            <p className="text-muted-foreground">
              Review chat now lives on the homepage. Sign in to continue where you left off.
            </p>
          </div>
          <div className="flex flex-col gap-3 sm:flex-row">
            <Button asChild>
              <Link href="/sign-in">Sign in</Link>
            </Button>
            <Button asChild variant="outline">
              <Link href="/sign-up">Create account</Link>
            </Button>
          </div>
        </div>
      </Unauthenticated>
    </>
  );
}
