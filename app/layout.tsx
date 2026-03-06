import type { Metadata } from 'next';
import { Geist, Geist_Mono } from 'next/font/google';
import './globals.css';
import { SpeedInsights } from '@vercel/speed-insights/react';
import { AnalyticsWrapper } from '@/components/analytics-wrapper';
import { ConditionalNavbar } from '@/components/conditional-navbar';
import { ConvexErrorBoundary } from '@/components/convex-error-boundary';
import { DeploymentVersionGuard } from '@/components/deployment-version-guard';
import { Footer } from '@/components/footer';
import { PostHogProvider } from '@/components/providers/PostHogProvider';
import { ThemeProvider } from '@/components/theme-provider';
import { Toaster } from '@/components/ui/sonner';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ConfirmationProvider } from '@/hooks/use-confirmation';
import { validateEnv } from '@/lib/env';
import { getLayoutClassName, needsNavbarSpacer } from '@/lib/layout-mode';
import { getSiteUrl, getSiteUrlObject } from '@/lib/site-url';
import { ClerkConvexProvider } from './clerk-provider';

// Validate environment variables at build/dev time
if (process.env.NODE_ENV === 'development') {
  validateEnv();
}

const geistSans = Geist({
  variable: '--font-geist-sans',
  subsets: ['latin'],
});

const geistMono = Geist_Mono({
  variable: '--font-geist-mono',
  subsets: ['latin'],
});

const siteUrl = getSiteUrl();

const softwareApplicationJsonLd = {
  '@context': 'https://schema.org',
  '@type': 'SoftwareApplication',
  name: 'Scry',
  description: 'Transform any topic into quiz questions with AI',
  applicationCategory: 'EducationalApplication',
  operatingSystem: 'Web',
  url: siteUrl,
};

export const metadata: Metadata = {
  metadataBase: getSiteUrlObject(),
  title: 'Scry - Simple Learning',
  description: 'Transform any topic into quiz questions with AI',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className={`${geistSans.variable} ${geistMono.variable} antialiased`}>
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: JSON.stringify(softwareApplicationJsonLd) }}
        />
        <ThemeProvider
          attribute="class"
          defaultTheme="system"
          enableSystem
          disableTransitionOnChange
        >
          <TooltipProvider delayDuration={300}>
            <ConfirmationProvider>
              <ClerkConvexProvider>
                <PostHogProvider>
                  <ConvexErrorBoundary>
                    <DeploymentVersionGuard>
                      <div className={getLayoutClassName()}>
                        <ConditionalNavbar />
                        {needsNavbarSpacer() && <div className="h-16" />}
                        <main className="mx-auto w-full max-w-7xl">{children}</main>
                        <Footer />
                      </div>
                      <Toaster />
                      <AnalyticsWrapper />
                      <SpeedInsights />
                    </DeploymentVersionGuard>
                  </ConvexErrorBoundary>
                </PostHogProvider>
              </ClerkConvexProvider>
            </ConfirmationProvider>
          </TooltipProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
