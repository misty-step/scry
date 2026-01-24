import { EvolveDashboard } from './_components/evolve-dashboard';

/**
 * Prompt Evolution Lab Route
 *
 * Dev-only route for viewing prompt evolution experiment results.
 * Accessible at /evolve in development mode only.
 *
 * CLI triggers evolution: `pnpm evolve`
 * This UI is read-only results viewer.
 */
export default function EvolvePage() {
  // Dev-only guard - prevent access in production
  if (process.env.NODE_ENV === 'production') {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Not Available</h1>
          <p className="text-muted-foreground">
            Prompt Evolution Lab is only available in development mode.
          </p>
        </div>
      </div>
    );
  }

  return <EvolveDashboard />;
}
