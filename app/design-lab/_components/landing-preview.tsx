'use client';

import { useRef } from 'react';
import Link from 'next/link';
import { ThemeToggle } from '@/components/theme-toggle';
import { Button } from '@/components/ui/button';
import { useParticleField, type ParticleSettings } from '@/hooks/use-particle-field';

// Re-export for consumers of this module
export type { ParticleSettings };

export type TypographySettings = {
  titleTracking: number;
  titleSizeVw: number;
  titleLineHeight: number;
  taglineSize: number;
  taglineTracking: number;
};

export type SpacingSettings = {
  titleTaglineGap: number;
  taglineCtaGap: number;
};

export type LandingConfig = {
  particle: ParticleSettings;
  typography: TypographySettings;
  spacing: SpacingSettings;
};

export const defaultLandingConfig: LandingConfig = {
  particle: {
    particleCount: 60,
    connectionDistance: 150,
    velocity: 0.3,
    particleAlphaMin: 0.1,
    particleAlphaMax: 0.4,
    particleSizeMin: 1,
    particleSizeMax: 3,
    connectionAlpha: 0.15,
  },
  typography: {
    titleTracking: -0.04,
    titleSizeVw: 18,
    titleLineHeight: 0.85,
    taglineSize: 1.5,
    taglineTracking: 0,
  },
  spacing: {
    titleTaglineGap: 2.5,
    taglineCtaGap: 1,
  },
};

/**
 * Landing page preview with parameterized typography, spacing, and particles.
 */
export function LandingPreview({ config }: { config: LandingConfig }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  useParticleField(canvasRef, { settings: config.particle, sizingMode: 'parent' });

  return (
    <div className="relative h-full min-h-screen bg-background">
      <canvas ref={canvasRef} className="absolute inset-0 pointer-events-none" />

      <div className="absolute top-6 right-6 z-20">
        <ThemeToggle />
      </div>

      <div className="relative z-10 min-h-screen flex items-center justify-center px-6 py-12">
        <div className="w-full max-w-5xl">
          <div className="flex flex-col">
            <h1
              className="font-bold text-foreground"
              style={{
                fontFeatureSettings: '"ss01"',
                fontSize: `clamp(5rem, ${config.typography.titleSizeVw}vw, 14rem)`,
                letterSpacing: `${config.typography.titleTracking}em`,
                lineHeight: config.typography.titleLineHeight,
              }}
            >
              Scry
            </h1>

            <p
              className="text-muted-foreground font-light"
              style={{
                marginTop: `${config.spacing.titleTaglineGap}rem`,
                fontSize: `${config.typography.taglineSize}rem`,
                letterSpacing: `${config.typography.taglineTracking}em`,
              }}
            >
              Remember everything.
            </p>

            <div style={{ marginTop: `${config.spacing.taglineCtaGap}rem` }}>
              <Button asChild size="lg" className="text-base px-8">
                <Link href="/sign-in">Get Started</Link>
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
