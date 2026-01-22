'use client';

import { useRef } from 'react';
import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { useParticleField, type ParticleSettings } from '@/hooks/use-particle-field';

/** Full-screen landing page particle config */
const LANDING_PARTICLE_SETTINGS: ParticleSettings = {
  particleCount: 180,
  connectionDistance: 175,
  velocity: 0.3,
  particleAlphaMin: 0.1,
  particleAlphaMax: 0.4,
  particleSizeMin: 1,
  particleSizeMax: 3,
  connectionAlpha: 0.15,
};

/**
 * Landing page for unauthenticated users.
 *
 * Features a particle field animation representing loose concepts
 * and the connections between them - a visual metaphor for memory.
 */
export function SignInLanding() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  useParticleField(canvasRef, { settings: LANDING_PARTICLE_SETTINGS, sizingMode: 'window' });

  return (
    <div className="min-h-screen relative bg-background">
      <canvas ref={canvasRef} className="absolute inset-0 pointer-events-none" />

      <div className="relative z-10 min-h-screen flex items-center justify-center px-6 py-12">
        <div className="w-full max-w-5xl">
          <div>
            <h1
              className="text-[clamp(5rem,25vw,14rem)] font-bold tracking-[-0.04em] leading-[0.7] text-foreground"
              style={{ fontFeatureSettings: '"ss01"' }}
            >
              Scry
            </h1>

            <p className="mt-4 text-[1.75rem] font-light tracking-[0.02em] text-muted-foreground">
              Remember everything.
            </p>

            <div className="mt-12">
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
