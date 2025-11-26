import { useCallback, useState } from 'react';

/**
 * Generic hook for inline editing with optimistic updates and rollback.
 *
 * @template T - The shape of the data being edited
 * @param initialData - Initial data to edit
 * @param onSave - Async function called when saving changes
 * @returns State and actions for inline editing
 *
 * @example
 * ```tsx
 * const conceptEdit = useInlineEdit(concept, async (data) => {
 *   await updateConcept({ conceptId: concept._id, ...data });
 * });
 *
 * // In render:
 * {conceptEdit.isEditing ? (
 *   <input
 *     value={conceptEdit.localData.title}
 *     onChange={(e) => conceptEdit.updateField('title', e.target.value)}
 *   />
 * ) : (
 *   <h2>{concept.title}</h2>
 * )}
 * ```
 */
export function useInlineEdit<T extends Record<string, unknown>>(
  initialData: T,
  onSave: (data: T) => Promise<void>
) {
  const [isEditing, setIsEditing] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [localData, setLocalData] = useState<T>(initialData);

  const startEdit = useCallback(() => {
    setIsEditing(true);
    // Create a copy of initialData for local editing
    setLocalData({ ...initialData });
  }, [initialData]);

  const updateField = useCallback(<K extends keyof T>(key: K, value: T[K]) => {
    setLocalData((prev) => ({ ...prev, [key]: value }));
  }, []);

  const cancel = useCallback(() => {
    setIsEditing(false);
    setLocalData({ ...initialData });
  }, [initialData]);

  const save = useCallback(async () => {
    setIsSaving(true);
    try {
      await onSave(localData);
      setIsEditing(false);
    } catch (error) {
      // Rollback on error - reuse cancel logic
      cancel();
      throw error;
    } finally {
      setIsSaving(false);
    }
  }, [localData, onSave, cancel]);

  // Calculate dirty state: compare localData with initialData
  const isDirty = JSON.stringify(localData) !== JSON.stringify(initialData);

  return {
    isEditing,
    isSaving,
    localData,
    isDirty,
    startEdit,
    updateField,
    save,
    cancel,
  };
}
