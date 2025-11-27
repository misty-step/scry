import { useCallback, useState } from 'react';
import {
  validateUnifiedEdit,
  validationErrorsToRecord,
  type QuestionType,
  type UnifiedEditData,
} from '@/lib/unified-edit-validation';

/**
 * Unified hook for editing both concept and phrasing fields in a single interface.
 *
 * **Key Features:**
 * - Smart dirty detection per domain (concept vs phrasing)
 * - Parallel mutation execution when both dirty (50% latency reduction)
 * - Field-level error mapping for precise feedback
 * - Graceful partial failure handling
 *
 * **Save Orchestration:**
 * 1. Validates all fields first (fail fast)
 * 2. Determines which mutations to call based on dirty tracking
 * 3. Executes mutations in parallel with Promise.all
 * 4. Maps errors to specific fields on failure
 * 5. Keeps edit mode open if any mutation fails
 * 6. Updates initialData for successful saves to prevent re-saving
 *
 * @example
 * ```tsx
 * const unifiedEdit = useUnifiedEdit(
 *   {
 *     conceptTitle: 'JavaScript Basics',
 *     conceptDescription: 'Fundamentals of JS',
 *     question: 'What is a closure?',
 *     correctAnswer: 'A function with access to outer scope',
 *     explanation: 'Closures maintain references...',
 *     options: ['Option 1', 'Option 2'],
 *   },
 *   async (data) => await updateConcept({ conceptId, ...data }),
 *   async (data) => await updatePhrasing({ phrasingId, ...data }),
 *   'multiple-choice'
 * );
 *
 * // Only saves concept if conceptTitle/conceptDescription changed
 * // Only saves phrasing if question/correctAnswer/explanation/options changed
 * await unifiedEdit.save();
 * ```
 */
export function useUnifiedEdit(
  initialData: UnifiedEditData,
  onSaveConcept: (data: {
    title: string;
    description?: string;
  }) => Promise<{ title: string; description?: string } | void>,
  onSavePhrasing: (data: {
    question: string;
    correctAnswer: string;
    explanation: string;
    options: string[];
  }) => Promise<{
    question: string;
    correctAnswer: string;
    explanation: string;
    options: string[];
  } | void>,
  questionType: QuestionType
) {
  const [isEditing, setIsEditing] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [localData, setLocalData] = useState<UnifiedEditData>(initialData);
  const [errors, setErrors] = useState<Record<string, string>>({});

  // Track baseline for partial saves - update when domain succeeds
  const [baselineData, setBaselineData] = useState<UnifiedEditData>(initialData);

  // ============================================================================
  // Dirty Detection (per domain)
  // ============================================================================

  const conceptIsDirty =
    localData.conceptTitle !== baselineData.conceptTitle ||
    localData.conceptDescription !== baselineData.conceptDescription;

  const phrasingIsDirty =
    localData.question !== baselineData.question ||
    localData.correctAnswer !== baselineData.correctAnswer ||
    localData.explanation !== baselineData.explanation ||
    JSON.stringify(localData.options) !== JSON.stringify(baselineData.options);

  const isDirty = conceptIsDirty || phrasingIsDirty;

  // ============================================================================
  // Actions
  // ============================================================================

  const startEdit = useCallback(() => {
    setIsEditing(true);
    setLocalData({ ...initialData });
    setBaselineData({ ...initialData });
    setErrors({});
  }, [initialData]);

  const updateField = useCallback(
    <K extends keyof UnifiedEditData>(key: K, value: UnifiedEditData[K]) => {
      setLocalData((prev) => ({ ...prev, [key]: value }));

      // Clear error for this field when user edits it
      setErrors((prev) => {
        const updated = { ...prev };
        delete updated[key as string];
        return updated;
      });
    },
    []
  );

  const cancel = useCallback(() => {
    setIsEditing(false);
    setLocalData({ ...initialData });
    setBaselineData({ ...initialData });
    setErrors({});
  }, [initialData]);

  const save = useCallback(async () => {
    setIsSaving(true);

    try {
      // Step 1: Validate all fields (fail fast)
      const validationErrors = validateUnifiedEdit(localData, questionType);
      if (validationErrors.length > 0) {
        const errorRecord = validationErrorsToRecord(validationErrors);
        setErrors(errorRecord);
        setIsSaving(false);
        return;
      }

      // Step 2: Determine which mutations to call
      const promises: Promise<void>[] = [];
      const errorMap: Record<string, string> = {};
      let conceptSucceeded = false;
      let phrasingSucceeded = false;

      // Step 3: Execute mutations in parallel (if both dirty)
      if (conceptIsDirty) {
        promises.push(
          onSaveConcept({
            title: localData.conceptTitle,
            description: localData.conceptDescription,
          })
            .then(() => {
              conceptSucceeded = true;
            })
            .catch((error) => {
              // Map mutation error to field
              const message = error.message || 'Failed to update concept';
              if (message.toLowerCase().includes('title')) {
                errorMap.conceptTitle = message;
              } else {
                errorMap.conceptTitle = 'Failed to update concept';
              }
            })
        );
      }

      if (phrasingIsDirty) {
        promises.push(
          onSavePhrasing({
            question: localData.question,
            correctAnswer: localData.correctAnswer,
            explanation: localData.explanation,
            options: localData.options,
          })
            .then(() => {
              phrasingSucceeded = true;
            })
            .catch((error) => {
              // Map mutation error to field
              const message = error.message || 'Failed to update phrasing';
              if (message.toLowerCase().includes('question')) {
                errorMap.question = message;
              } else if (
                message.toLowerCase().includes('correct answer') ||
                message.toLowerCase().includes('option')
              ) {
                errorMap.correctAnswer = message;
              } else {
                errorMap.question = 'Failed to update phrasing';
              }
            })
        );
      }

      // Wait for all mutations to complete
      await Promise.all(promises);

      // Step 4: Handle partial failures
      if (Object.keys(errorMap).length > 0) {
        setErrors(errorMap);

        // Update baseline for successful saves to prevent re-saving
        if (conceptSucceeded) {
          setBaselineData((prev) => ({
            ...prev,
            conceptTitle: localData.conceptTitle,
            conceptDescription: localData.conceptDescription,
          }));
        }

        if (phrasingSucceeded) {
          setBaselineData((prev) => ({
            ...prev,
            question: localData.question,
            correctAnswer: localData.correctAnswer,
            explanation: localData.explanation,
            options: localData.options,
          }));
        }

        // Don't exit edit mode - allow user to fix errors and retry
        throw new Error('Some fields failed to save');
      }

      // Step 5: All succeeded - exit edit mode
      setIsEditing(false);
      setErrors({});
      setBaselineData(localData); // Update baseline to current data
    } catch (error) {
      // Error already logged and errors state set
      // Don't rollback - keep localData so user can fix and retry
      throw error;
    } finally {
      setIsSaving(false);
    }
  }, [
    localData,
    baselineData,
    questionType,
    conceptIsDirty,
    phrasingIsDirty,
    onSaveConcept,
    onSavePhrasing,
  ]);

  return {
    isEditing,
    isSaving,
    localData,
    isDirty,
    conceptIsDirty,
    phrasingIsDirty,
    errors,
    startEdit,
    updateField,
    save,
    cancel,
  };
}
