'use client';

import { useUser } from '@clerk/nextjs';
import { ReviewFlow } from '@/components/review-flow';
import { ReviewErrorBoundary } from '@/components/review/review-error-boundary';
import { SignInLanding } from '@/components/sign-in-landing';

const siteUrl = process.env.NEXT_PUBLIC_APP_URL ?? 'https://scry.study';

const softwareApplicationJsonLd = {
  '@context': 'https://schema.org',
  '@type': 'SoftwareApplication',
  name: 'Scry',
  description: 'Transform any topic into quiz questions with AI',
  applicationCategory: 'EducationalApplication',
  operatingSystem: 'Web',
  url: siteUrl,
};

export default function Home() {
  const { isSignedIn, isLoaded } = useUser();

  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(softwareApplicationJsonLd) }}
      />

      {/* Wait for auth to load */}
      {!isLoaded ? null : !isSignedIn ? (
        // Show landing page for unauthenticated users
        <SignInLanding />
      ) : (
        // Show review flow for authenticated users
        <ReviewErrorBoundary
          fallbackMessage="Unable to load the review session. Please refresh to try again."
          onReset={() => {
            // Optional: Clear any cached state or perform cleanup
            if (typeof window !== 'undefined') {
              window.location.href = '/';
            }
          }}
        >
          <ReviewFlow />
        </ReviewErrorBoundary>
      )}
    </>
  );
}
