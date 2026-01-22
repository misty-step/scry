import { LandingTuner } from './_components/landing-tuner';

/**
 * Design Lab Route
 *
 * Dev-only route for tuning the landing page visual system.
 * Accessible at /design-lab in development mode only.
 */
export default function DesignLabPage() {
  if (process.env.NODE_ENV === 'production') {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Not Available</h1>
          <p className="text-muted-foreground">Design Lab is only available in development mode.</p>
        </div>
      </div>
    );
  }

  return <LandingTuner />;
}
