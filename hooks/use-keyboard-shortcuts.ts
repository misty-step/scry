import { useCallback, useEffect, useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';

export interface ShortcutDefinition {
  key: string;
  ctrl?: boolean;
  alt?: boolean;
  shift?: boolean;
  description: string;
  action: () => void;
  context?: 'global' | 'review' | 'editing';
}

export function useKeyboardShortcuts(shortcuts: ShortcutDefinition[], enabled = true) {
  const router = useRouter();
  const [showHelp, setShowHelp] = useState(false);

  // Global shortcuts that work anywhere
  const globalShortcuts = useMemo<ShortcutDefinition[]>(
    () => [
      {
        key: '?',
        description: 'Show keyboard shortcuts help',
        action: () => setShowHelp((prev) => !prev),
        context: 'global',
      },
      {
        key: 'h',
        description: 'Go to home/review',
        action: () => router.push('/'),
        context: 'global',
      },
      {
        key: 'c',
        description: 'Go to concepts',
        action: () => router.push('/concepts'),
        context: 'global',
      },
      {
        key: 's',
        ctrl: true,
        description: 'Go to settings',
        action: () => {
          router.push('/settings');
          toast.info('Opening settings...');
        },
        context: 'global',
      },
      {
        key: 'g',
        description: 'Generate new questions',
        action: () => {
          // Dispatch event to open generation modal
          window.dispatchEvent(new CustomEvent('open-generation-modal'));
        },
        context: 'global',
      },
      {
        key: 'Escape',
        description: 'Close modals/cancel editing',
        action: () => {
          // Dispatch a custom event that components can listen to
          window.dispatchEvent(new CustomEvent('escape-pressed'));
        },
        context: 'global',
      },
    ],
    [router]
  );
  const allShortcuts = useMemo(
    () => [...globalShortcuts, ...shortcuts],
    [globalShortcuts, shortcuts]
  );

  const handleKeyPress = useCallback(
    (e: KeyboardEvent) => {
      // Don't handle shortcuts if user is typing in an input (unless it's a global shortcut with modifier)
      const isTyping =
        e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement;
      const hasModifier = e.ctrlKey || e.metaKey || e.altKey;

      if (isTyping && !hasModifier) {
        return;
      }

      // Combine user shortcuts with global shortcuts
      // Find matching shortcut
      const matchingShortcut = allShortcuts.find((shortcut) => {
        const keyMatch =
          e.key.toLowerCase() === shortcut.key.toLowerCase() || e.key === shortcut.key;
        const ctrlMatch = !shortcut.ctrl || e.ctrlKey || e.metaKey;
        const altMatch = !shortcut.alt || e.altKey;
        const shiftMatch = !shortcut.shift || e.shiftKey;

        return keyMatch && ctrlMatch && altMatch && shiftMatch;
      });

      if (matchingShortcut) {
        e.preventDefault();
        matchingShortcut.action();
      }
    },
    [allShortcuts]
  );

  useEffect(() => {
    if (!enabled) return;

    window.addEventListener('keydown', handleKeyPress);
    return () => window.removeEventListener('keydown', handleKeyPress);
  }, [handleKeyPress, enabled]);

  return {
    showHelp,
    setShowHelp,
    shortcuts: allShortcuts,
  };
}
