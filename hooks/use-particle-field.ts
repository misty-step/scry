'use client';

import { useEffect, type RefObject } from 'react';

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

type SizingMode = 'window' | 'parent';

interface UseParticleFieldOptions {
  settings: ParticleSettings;
  /** 'window' uses window.innerWidth/Height, 'parent' uses parent container dimensions */
  sizingMode?: SizingMode;
}

/**
 * Particle field animation for canvas backgrounds.
 *
 * Deep module: encapsulates all particle physics, canvas rendering,
 * and animation frame management. Caller provides canvas ref + config.
 */
export function useParticleField(
  canvasRef: RefObject<HTMLCanvasElement | null>,
  options: UseParticleFieldOptions
) {
  const { settings, sizingMode = 'parent' } = options;
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
    // Pre-compute squared distance for O(n^2) connection loop optimization
    const connectionDistanceSq = connectionDistance * connectionDistance;

    const resize = () => {
      if (sizingMode === 'window') {
        canvas.width = window.innerWidth;
        canvas.height = window.innerHeight;
      } else {
        const parent = canvas.parentElement;
        if (parent) {
          canvas.width = parent.clientWidth;
          canvas.height = parent.clientHeight;
        }
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
      // Optimization: compare squared distances first, only sqrt when needed for alpha
      for (let i = 0; i < particles.length; i++) {
        for (let j = i + 1; j < particles.length; j++) {
          const dx = particles[i].x - particles[j].x;
          const dy = particles[i].y - particles[j].y;
          const distSq = dx * dx + dy * dy;

          if (distSq < connectionDistanceSq) {
            const dist = Math.sqrt(distSq);
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
    sizingMode,
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
