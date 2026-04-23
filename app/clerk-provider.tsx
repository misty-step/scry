'use client';

import { useEffect, useRef, useState } from 'react';
import { ClerkProvider, useAuth, useUser } from '@clerk/nextjs';
import { ConvexReactClient, useMutation } from 'convex/react';
import { ConvexProviderWithClerk } from 'convex/react-clerk';
import { api } from '@/convex/_generated/api';
import { useClerkAppearance } from '@/hooks/use-clerk-appearance';

const convex = new ConvexReactClient(process.env.NEXT_PUBLIC_CONVEX_URL!);

function ThemedClerkProvider({ children }: { children: React.ReactNode }) {
  const appearance = useClerkAppearance();

  return (
    <ClerkProvider
      publishableKey={process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY!}
      appearance={appearance}
    >
      <ConvexProviderWithClerk client={convex} useAuth={useAuth}>
        <EnsureConvexUser>{children}</EnsureConvexUser>
      </ConvexProviderWithClerk>
    </ClerkProvider>
  );
}

export function ClerkConvexProvider({ children }: { children: React.ReactNode }) {
  return <ThemedClerkProvider>{children}</ThemedClerkProvider>;
}

function EnsureConvexUser({ children }: { children: React.ReactNode }) {
  const { isLoaded, isSignedIn, user } = useUser();
  const ensureUser = useMutation(api.clerk.ensureUser);
  const [ensuredUserId, setEnsuredUserId] = useState<string | null>(null);
  const [failedUserId, setFailedUserId] = useState<string | null>(null);
  const inFlightUserIdRef = useRef<string | null>(null);
  const userId = isSignedIn ? (user?.id ?? null) : null;
  const ready = isLoaded && (!userId || ensuredUserId === userId || failedUserId === userId);

  useEffect(() => {
    if (!userId) {
      inFlightUserIdRef.current = null;
      return;
    }

    if (
      ensuredUserId === userId ||
      failedUserId === userId ||
      inFlightUserIdRef.current === userId
    ) {
      return;
    }

    let cancelled = false;
    inFlightUserIdRef.current = userId;

    void (async () => {
      try {
        await ensureUser();
        if (!cancelled) {
          setEnsuredUserId(userId);
          setFailedUserId((current) => (current === userId ? null : current));
        }
      } catch (error) {
        console.error('Failed to ensure Convex user', error);
        if (!cancelled) {
          setFailedUserId(userId);
        }
      } finally {
        if (!cancelled && inFlightUserIdRef.current === userId) {
          inFlightUserIdRef.current = null;
        }
      }
    })();

    return () => {
      cancelled = true;
      if (inFlightUserIdRef.current === userId) {
        inFlightUserIdRef.current = null;
      }
    };
  }, [ensureUser, ensuredUserId, failedUserId, userId]);

  if (!ready) {
    return null;
  }

  return <>{children}</>;
}
