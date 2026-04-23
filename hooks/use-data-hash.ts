import { useCallback, useEffect, useRef, useSyncExternalStore } from 'react';

/**
 * Simple string hash function for data comparison
 * Uses djb2 algorithm for fast, consistent hashing
 */
export function hashString(str: string): number {
  let hash = 5381;
  for (let i = 0; i < str.length; i++) {
    hash = (hash << 5) + hash + str.charCodeAt(i);
  }
  return hash >>> 0; // Convert to unsigned 32-bit integer
}

/**
 * Hook to detect when data actually changes vs when polling returns identical data
 * Prevents unnecessary renders by comparing data hashes
 *
 * @param data - The data to monitor for changes
 * @returns Object with hasChanged flag and update function
 */
export function useDataHash<T>(data: T): {
  hasChanged: boolean;
  previousHash: number | null;
  currentHash: number | null;
  update: () => void;
} {
  const previousHashRef = useRef<number | null>(null);
  const currentHashRef = useRef<number | null>(null);
  const subscribe = useCallback(() => () => {}, []);
  const previousHash = useSyncExternalStore(
    subscribe,
    () => previousHashRef.current,
    () => previousHashRef.current
  );

  let currentHash: number | null = null;
  let hasChanged = false;

  try {
    if (data !== null && data !== undefined) {
      const dataString = JSON.stringify(data);
      currentHash = hashString(dataString);
      hasChanged = previousHash !== currentHash;
    } else {
      currentHash = null;
      hasChanged = previousHash !== null;
    }
  } catch {
    currentHash = null;
    hasChanged = true;
  }

  // Update function to manually mark the current hash as "previous"
  const update = useCallback(() => {
    previousHashRef.current = currentHashRef.current;
  }, []);

  // Auto-update previous hash when data changes
  useEffect(() => {
    currentHashRef.current = currentHash;
    if (hasChanged) {
      previousHashRef.current = currentHash;
    }
  }, [currentHash, hasChanged]);

  return {
    hasChanged,
    previousHash,
    currentHash,
    update,
  };
}
