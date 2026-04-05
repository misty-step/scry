'use client';

import { useEffect, useState } from 'react';
import { useTheme } from 'next-themes';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { UserButton, useUser } from '@clerk/nextjs';
import { Library, Plus } from 'lucide-react';
import { GenerationModal } from '@/components/generation-modal';
import { Button } from '@/components/ui/button';
import { useActiveJobs } from '@/hooks/use-active-jobs';
import { useClerkAppearance } from '@/hooks/use-clerk-appearance';
import { getNavbarClassName } from '@/lib/layout-mode';

export function Navbar() {
  const { isLoaded, isSignedIn } = useUser();
  const clerkAppearance = useClerkAppearance();
  const pathname = usePathname();
  const { setTheme, theme, systemTheme } = useTheme();
  const [generateOpen, setGenerateOpen] = useState(false);
  const { activeCount } = useActiveJobs();
  const resolvedTheme = theme === 'system' ? (systemTheme ?? 'light') : (theme ?? 'light');
  const nextTheme = resolvedTheme === 'dark' ? 'light' : 'dark';
  const generateLabel =
    activeCount > 0
      ? `Generate questions (${activeCount} active ${activeCount === 1 ? 'job' : 'jobs'})`
      : 'Generate questions';

  const handleThemeToggle = () => {
    const shouldMatchSystem = systemTheme === nextTheme;
    setTheme(shouldMatchSystem ? 'system' : nextTheme);
  };

  // Listen for keyboard shortcut to open generation modal
  useEffect(() => {
    const handleOpenGenerationModal = () => {
      setGenerateOpen(true);
    };

    window.addEventListener('open-generation-modal', handleOpenGenerationModal);
    return () => window.removeEventListener('open-generation-modal', handleOpenGenerationModal);
  }, []);

  // Hide navbar completely when unauthenticated
  if (!isSignedIn && isLoaded) return null;

  return (
    <>
      <nav
        className={`${getNavbarClassName()} h-16 bg-background/80 backdrop-blur-sm border-b border-border`}
      >
        <div className="mx-auto flex h-full w-full max-w-7xl items-center justify-between px-4 md:px-8">
          <Link
            href="/"
            className="text-xl md:text-2xl font-semibold tracking-tight text-foreground/80 hover:text-foreground border-b-0 transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 rounded-sm"
          >
            Scry.
          </Link>

          <div className="flex items-center gap-4">
            {isSignedIn && (
              <>
                <Button
                  variant="ghost"
                  size="icon"
                  className="relative size-9 rounded-full bg-accent/50 text-muted-foreground transition-all duration-200 hover:bg-accent/70 hover:text-foreground hover:scale-105 active:scale-95 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-0"
                  onClick={() => setGenerateOpen(true)}
                  title={`${generateLabel} (G)`}
                  aria-label={generateLabel}
                >
                  <Plus className="relative h-4 w-4" />
                  {activeCount > 0 && (
                    <span className="absolute -top-1 -right-1 flex h-5 w-5 items-center justify-center rounded-full bg-primary text-[10px] font-bold text-primary-foreground">
                      {activeCount}
                    </span>
                  )}
                  <span className="sr-only">Generate questions</span>
                </Button>
                <Link
                  href="/concepts"
                  className={`relative size-9 rounded-full flex items-center justify-center transition-all duration-200 hover:scale-105 active:scale-95 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-0 ${
                    pathname?.startsWith('/concepts')
                      ? 'bg-accent text-foreground'
                      : 'bg-accent/50 text-muted-foreground hover:bg-accent/70 hover:text-foreground'
                  }`}
                  aria-label="Concepts"
                  title="Concepts"
                >
                  <Library className="h-4 w-4" />
                </Link>
                <div
                  data-testid="user-menu"
                  className="flex size-9 items-center justify-center rounded-full"
                >
                  <UserButton
                    afterSignOutUrl="/"
                    appearance={clerkAppearance}
                    customMenuItems={[
                      { label: 'Settings', href: '/settings' },
                      {
                        label: `Switch to ${nextTheme} theme`,
                        onClick: handleThemeToggle,
                      },
                    ]}
                  />
                </div>
              </>
            )}
          </div>
        </div>
      </nav>

      <GenerationModal open={generateOpen} onOpenChange={setGenerateOpen} />
    </>
  );
}
