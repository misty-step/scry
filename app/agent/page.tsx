'use client';

import { Authenticated, Unauthenticated } from 'convex/react';
import { ReviewChat } from '@/components/agent/review-chat';

export default function AgentPage() {
  return (
    <>
      <Authenticated>
        <div className="h-[calc(100dvh-var(--navbar-height))] overflow-hidden">
          <ReviewChat />
        </div>
      </Authenticated>
      <Unauthenticated>
        <div className="flex min-h-[60vh] items-center justify-center">
          <p className="text-muted-foreground">Sign in to start reviewing.</p>
        </div>
      </Unauthenticated>
    </>
  );
}
