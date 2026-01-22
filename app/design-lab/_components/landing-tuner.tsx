'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Slider } from '@/components/ui/slider';
import { defaultLandingConfig, LandingConfig, LandingPreview } from './landing-preview';

const createDefaultConfig = (): LandingConfig => ({
  particle: { ...defaultLandingConfig.particle },
  typography: { ...defaultLandingConfig.typography },
  spacing: { ...defaultLandingConfig.spacing },
});

const formatValue = (value: number, precision = 2) => {
  const fixed = value.toFixed(precision);
  return fixed.replace(/\.?0+$/, '');
};

type SliderFieldProps = {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  precision?: number;
  suffix?: string;
  onChange: (value: number) => void;
};

function SliderField({
  label,
  value,
  min,
  max,
  step,
  precision,
  suffix,
  onChange,
}: SliderFieldProps) {
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between text-sm">
        <span className="font-medium text-foreground">{label}</span>
        <span className="text-muted-foreground">
          {formatValue(value, precision)}
          {suffix ?? ''}
        </span>
      </div>
      <Slider
        min={min}
        max={max}
        step={step}
        value={[value]}
        onValueChange={(next) => {
          const nextValue = next[0];
          if (typeof nextValue === 'number') {
            onChange(nextValue);
          }
        }}
      />
    </div>
  );
}

export function LandingTuner() {
  const [config, setConfig] = useState<LandingConfig>(createDefaultConfig);

  const updateParticle = (key: keyof LandingConfig['particle'], value: number) =>
    setConfig((prev) => ({
      ...prev,
      particle: { ...prev.particle, [key]: value },
    }));

  const updateTypography = (key: keyof LandingConfig['typography'], value: number) =>
    setConfig((prev) => ({
      ...prev,
      typography: { ...prev.typography, [key]: value },
    }));

  const updateSpacing = (key: keyof LandingConfig['spacing'], value: number) =>
    setConfig((prev) => ({
      ...prev,
      spacing: { ...prev.spacing, [key]: value },
    }));

  const handleReset = () => {
    setConfig(createDefaultConfig());
  };

  const handleCopy = async () => {
    if (!navigator.clipboard) return;
    try {
      await navigator.clipboard.writeText(JSON.stringify(config, null, 2));
    } catch {
      // Ignore clipboard errors in dev.
    }
  };

  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="flex min-h-screen flex-col lg:flex-row">
        <div className="relative lg:basis-[70%] lg:max-w-[70%] border-b border-border lg:border-b-0 lg:border-r">
          <LandingPreview config={config} />
        </div>

        <div className="lg:basis-[30%] lg:max-w-[30%]">
          <div className="h-full max-h-screen overflow-y-auto px-6 py-8">
            <div className="space-y-8">
              <div className="space-y-4">
                <div>
                  <h2 className="text-xl font-semibold">Design Lab</h2>
                  <p className="text-sm text-muted-foreground">
                    Tweak the landing page in real time.
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button variant="secondary" onClick={handleReset}>
                    Reset to defaults
                  </Button>
                  <Button variant="outline" onClick={handleCopy}>
                    Copy config
                  </Button>
                </div>
              </div>

              <section className="space-y-4">
                <h3 className="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                  Particle System
                </h3>
                <SliderField
                  label="Particle count"
                  value={config.particle.particleCount}
                  min={20}
                  max={200}
                  step={10}
                  precision={0}
                  onChange={(value) => updateParticle('particleCount', value)}
                />
                <SliderField
                  label="Connection distance"
                  value={config.particle.connectionDistance}
                  min={50}
                  max={400}
                  step={25}
                  precision={0}
                  onChange={(value) => updateParticle('connectionDistance', value)}
                />
                <SliderField
                  label="Velocity"
                  value={config.particle.velocity}
                  min={0}
                  max={2}
                  step={0.1}
                  precision={2}
                  onChange={(value) => updateParticle('velocity', value)}
                />
                <SliderField
                  label="Particle alpha min"
                  value={config.particle.particleAlphaMin}
                  min={0.05}
                  max={0.5}
                  step={0.05}
                  precision={2}
                  onChange={(value) => updateParticle('particleAlphaMin', value)}
                />
                <SliderField
                  label="Particle alpha max"
                  value={config.particle.particleAlphaMax}
                  min={0.1}
                  max={0.8}
                  step={0.05}
                  precision={2}
                  onChange={(value) => updateParticle('particleAlphaMax', value)}
                />
                <SliderField
                  label="Particle size min"
                  value={config.particle.particleSizeMin}
                  min={0.5}
                  max={3}
                  step={0.5}
                  precision={1}
                  onChange={(value) => updateParticle('particleSizeMin', value)}
                />
                <SliderField
                  label="Particle size max"
                  value={config.particle.particleSizeMax}
                  min={1}
                  max={5}
                  step={0.5}
                  precision={1}
                  onChange={(value) => updateParticle('particleSizeMax', value)}
                />
                <SliderField
                  label="Connection alpha"
                  value={config.particle.connectionAlpha}
                  min={0.05}
                  max={0.4}
                  step={0.05}
                  precision={2}
                  onChange={(value) => updateParticle('connectionAlpha', value)}
                />
              </section>

              <section className="space-y-4">
                <h3 className="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                  Typography
                </h3>
                <SliderField
                  label="Title tracking"
                  value={config.typography.titleTracking}
                  min={-0.1}
                  max={0.05}
                  step={0.01}
                  precision={2}
                  suffix="em"
                  onChange={(value) => updateTypography('titleTracking', value)}
                />
                <SliderField
                  label="Title size (vw)"
                  value={config.typography.titleSizeVw}
                  min={10}
                  max={25}
                  step={1}
                  precision={0}
                  suffix="vw"
                  onChange={(value) => updateTypography('titleSizeVw', value)}
                />
                <SliderField
                  label="Title line height"
                  value={config.typography.titleLineHeight}
                  min={0.7}
                  max={1.2}
                  step={0.05}
                  precision={2}
                  onChange={(value) => updateTypography('titleLineHeight', value)}
                />
                <SliderField
                  label="Tagline size"
                  value={config.typography.taglineSize}
                  min={1}
                  max={3}
                  step={0.125}
                  precision={3}
                  suffix="rem"
                  onChange={(value) => updateTypography('taglineSize', value)}
                />
                <SliderField
                  label="Tagline tracking"
                  value={config.typography.taglineTracking}
                  min={0}
                  max={0.3}
                  step={0.02}
                  precision={2}
                  suffix="em"
                  onChange={(value) => updateTypography('taglineTracking', value)}
                />
              </section>

              <section className="space-y-4">
                <h3 className="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
                  Spacing
                </h3>
                <SliderField
                  label="Title to tagline"
                  value={config.spacing.titleTaglineGap}
                  min={1}
                  max={6}
                  step={0.5}
                  precision={1}
                  suffix="rem"
                  onChange={(value) => updateSpacing('titleTaglineGap', value)}
                />
                <SliderField
                  label="Tagline to CTA"
                  value={config.spacing.taglineCtaGap}
                  min={1}
                  max={6}
                  step={0.5}
                  precision={1}
                  suffix="rem"
                  onChange={(value) => updateSpacing('taglineCtaGap', value)}
                />
              </section>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
