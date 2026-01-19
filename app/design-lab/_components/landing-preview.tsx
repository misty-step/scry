'use client';

import { useEffect, useRef, type RefObject } from 'react';
import Link from 'next/link';
import { ThemeToggle } from '@/components/theme-toggle';
import { Button } from '@/components/ui/button';

export type ParticleSettings = {
  particleCount: number;
  connectionDistance: number;
  velocity: number;
  particleAlphaMin: number;
  particleAlphaMax: number;
  particleSizeMin: number;
  particleSizeMax: number;
  connectionAlpha: number;
};

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
 * Particle field animation for the landing page background.
 *
 * Deep module: encapsulates all particle physics, canvas rendering,
 * and animation frame management. Caller just provides a canvas ref + config.
 */
function useParticleField(
  canvasRef: RefObject<HTMLCanvasElement | null>,
  settings: ParticleSettings
) {
  const {
    particleCount,
    connectionDistance,
    velocity,
    particleAlphaMin,
    particleAlphaMax,
    particleSizeMin,
    particleSizeMax,
    connectionAlpha,
  } = settings;

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    let animationId: number;

    interface Particle {
      x: number;
      y: number;
      vx: number;
      vy: number;
      radius: number;
      alpha: number;
    }

    const particles: Particle[] = [];

    const resize = () => {
      // Use parent container dimensions, not canvas rect (which is 0 when absolutely positioned)
      const parent = canvas.parentElement;
      if (parent) {
        canvas.width = parent.clientWidth;
        canvas.height = parent.clientHeight;
      }
    };

    const initParticles = () => {
      particles.length = 0;
      for (let i = 0; i < particleCount; i++) {
        particles.push({
          x: Math.random() * canvas.width,
          y: Math.random() * canvas.height,
          vx: (Math.random() - 0.5) * velocity,
          vy: (Math.random() - 0.5) * velocity,
          radius: Math.random() * (particleSizeMax - particleSizeMin) + particleSizeMin,
          alpha: Math.random() * (particleAlphaMax - particleAlphaMin) + particleAlphaMin,
        });
      }
    };

    const draw = () => {
      const { width, height } = canvas;
      ctx.clearRect(0, 0, width, height);

      const isDark = document.documentElement.classList.contains('dark');
      const particleColor = isDark ? [148, 163, 184] : [100, 116, 139];

      // Update positions and draw particles
      for (const p of particles) {
        p.x += p.vx;
        p.y += p.vy;

        // Wrap around edges
        if (p.x < 0) p.x = width;
        if (p.x > width) p.x = 0;
        if (p.y < 0) p.y = height;
        if (p.y > height) p.y = 0;

        ctx.fillStyle = `rgba(${particleColor.join(',')}, ${p.alpha})`;
        ctx.beginPath();
        ctx.arc(p.x, p.y, p.radius, 0, Math.PI * 2);
        ctx.fill();
      }

      // Draw connections between nearby particles
      for (let i = 0; i < particles.length; i++) {
        for (let j = i + 1; j < particles.length; j++) {
          const dx = particles[i].x - particles[j].x;
          const dy = particles[i].y - particles[j].y;
          const dist = Math.sqrt(dx * dx + dy * dy);

          if (dist < connectionDistance) {
            const alpha = (1 - dist / connectionDistance) * connectionAlpha;
            ctx.strokeStyle = `rgba(${particleColor.join(',')}, ${alpha})`;
            ctx.lineWidth = 0.5;
            ctx.beginPath();
            ctx.moveTo(particles[i].x, particles[i].y);
            ctx.lineTo(particles[j].x, particles[j].y);
            ctx.stroke();
          }
        }
      }

      animationId = requestAnimationFrame(draw);
    };

    const handleResize = () => {
      resize();
      initParticles();
    };

    resize();
    initParticles();
    window.addEventListener('resize', handleResize);
    draw();

    return () => {
      window.removeEventListener('resize', handleResize);
      cancelAnimationFrame(animationId);
    };
  }, [
    canvasRef,
    particleCount,
    connectionDistance,
    velocity,
    particleAlphaMin,
    particleAlphaMax,
    particleSizeMin,
    particleSizeMax,
    connectionAlpha,
  ]);
}

/**
 * Landing page preview with parameterized typography, spacing, and particles.
 */
export function LandingPreview({ config }: { config: LandingConfig }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  useParticleField(canvasRef, config.particle);

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
