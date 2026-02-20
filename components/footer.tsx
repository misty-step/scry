'use client';

import { useState } from 'react';
import { useUser } from '@clerk/nextjs';
import { KeyboardIndicator } from '@/components/keyboard-indicator';
import { KeyboardShortcutsHelp } from '@/components/keyboard-shortcuts-help';
import { useKeyboardShortcuts } from '@/hooks/use-keyboard-shortcuts';

export function Footer() {
  const { isSignedIn } = useUser();
  const [showHelp, setShowHelp] = useState(false);
  const { shortcuts } = useKeyboardShortcuts([], true);

  return (
    <>
      <footer className="mt-auto bg-background/80 backdrop-blur-sm">
        <div className="w-full px-4 md:px-8 py-4">
          <div className="flex items-center justify-between gap-4">
            <a
              href="https://mistystep.io"
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs text-muted-foreground/70 hover:text-muted-foreground transition-colors"
            >
              a misty step project
            </a>

            <div className="flex items-center gap-3">
              <a
                href="mailto:hello@mistystep.io"
                className="text-xs text-muted-foreground/70 hover:text-muted-foreground transition-colors"
              >
                Feedback
              </a>
              {isSignedIn && <KeyboardIndicator onClick={() => setShowHelp(true)} />}
            </div>
          </div>
        </div>
      </footer>

      {isSignedIn && (
        <KeyboardShortcutsHelp open={showHelp} onOpenChange={setShowHelp} shortcuts={shortcuts} />
      )}
    </>
  );
}
