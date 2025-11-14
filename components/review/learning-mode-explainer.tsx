'use client';

import { useState } from 'react';
import { Brain, X } from 'lucide-react';

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { safeStorage } from '@/lib/storage';

const STORAGE_KEY = 'hasSeenLearningModeExplainer';

export function LearningModeExplainer() {
  // SSR-safe: start hidden, show on mount if not dismissed
  const [show, setShow] = useState(
    () => typeof window !== 'undefined' && !safeStorage.getItem(STORAGE_KEY)
  );

  if (!show) return null;

  return (
    <Alert className="mb-4 border-blue-500 bg-blue-50 dark:bg-blue-950/20 dark:border-blue-700">
      <Brain className="h-4 w-4 text-blue-700 dark:text-blue-400" />
      <AlertTitle className="text-blue-900 dark:text-blue-100">
        Learning Mode Active
      </AlertTitle>
      <AlertDescription className="text-blue-800 dark:text-blue-200">
        New concept! You&apos;ll see this a few times today with short intervals to encode it into
        long-term memory. This is normal spaced repetition practice.
      </AlertDescription>
      <Button
        variant="ghost"
        size="sm"
        onClick={() => {
          safeStorage.setItem(STORAGE_KEY, 'true');
          setShow(false);
        }}
        className="mt-2 text-blue-700 hover:text-blue-900 dark:text-blue-400 dark:hover:text-blue-200"
        aria-label="Dismiss learning mode explainer"
      >
        <X className="h-4 w-4 mr-1" />
        Got it, don&apos;t show again
      </Button>
    </Alert>
  );
}
