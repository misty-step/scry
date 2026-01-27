'use client';

import { Suspense, useEffect, type ReactNode } from 'react';
import { usePathname, useSearchParams } from 'next/navigation';
import { useUser } from '@clerk/nextjs';
import posthog from 'posthog-js';
import { PostHogProvider as PostHogReactProvider, usePostHog } from 'posthog-js/react';

const POSTHOG_KEY = process.env.NEXT_PUBLIC_POSTHOG_KEY;
const POSTHOG_HOST = process.env.NEXT_PUBLIC_POSTHOG_HOST ?? '/ingest';
const POSTHOG_ENABLED = Boolean(POSTHOG_KEY);
type SearchParams = ReturnType<typeof useSearchParams>;

function buildCurrentUrl(pathname: string, searchParams: SearchParams): string {
  const query = searchParams?.toString();
  const suffix = query ? `?${query}` : '';
  return `${window.location.origin}${pathname}${suffix}`;
}

function CapturePostHogPageView() {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const posthogClient = usePostHog();

  useEffect(() => {
    if (!pathname || !posthogClient) return;
    posthogClient.capture('$pageview', {
      $current_url: buildCurrentUrl(pathname, searchParams),
    });
  }, [pathname, searchParams, posthogClient]);

  return null;
}

export function PostHogProvider({ children }: { children: ReactNode }) {
  const { user, isLoaded } = useUser();

  useEffect(() => {
    if (!POSTHOG_KEY || typeof window === 'undefined') return;

    posthog.init(POSTHOG_KEY, {
      api_host: POSTHOG_HOST,
      person_profiles: 'identified_only',
      capture_pageview: false, // We capture manually for SPA navigation
      capture_pageleave: true,
      mask_all_text: true, // Prevent autocapture from sending text content
      autocapture: {
        dom_event_allowlist: ['click', 'submit'],
        element_allowlist: ['button', 'a', 'input', 'form'],
      },
      session_recording: {
        maskAllInputs: true, // Mask all input values in session recordings
      },
    });
  }, []);

  // Identify logged-in users to PostHog (user.id only, no PII)
  useEffect(() => {
    if (!POSTHOG_ENABLED || !isLoaded) return;

    if (user) {
      posthog.identify(user.id);
    } else {
      posthog.reset();
    }
  }, [user, isLoaded]);

  if (!POSTHOG_ENABLED) return <>{children}</>;

  return (
    <PostHogReactProvider client={posthog}>
      <Suspense fallback={null}>
        <CapturePostHogPageView />
      </Suspense>
      {children}
    </PostHogReactProvider>
  );
}
