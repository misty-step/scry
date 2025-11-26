'use client';

import { useCallback, useState } from 'react';
import { useMutation } from 'convex/react';
import { toast } from 'sonner';
import { api } from '@/convex/_generated/api';
import { useUndoableAction } from './use-undoable-action';

interface UseConceptActionsArgs {
  conceptId: string;
}

export function useConceptActions({ conceptId }: UseConceptActionsArgs) {
  const setCanonicalMutation = useMutation(api.concepts.setCanonicalPhrasing);
  const archivePhrasingMutation = useMutation(api.concepts.archivePhrasing);
  const unarchivePhrasingMutation = useMutation(api.concepts.unarchivePhrasing);
  const archiveConceptMutation = useMutation(api.concepts.archiveConcept);
  const unarchiveConceptMutation = useMutation(api.concepts.unarchiveConcept);
  const updateConceptMutation = useMutation(api.concepts.updateConcept);
  const updatePhrasingMutation = useMutation(api.concepts.updatePhrasing);
  const requestGenerationMutation = useMutation(api.concepts.requestPhrasingGeneration);

  const undoableAction = useUndoableAction();

  const [pendingAction, setPendingAction] = useState<
    'canonical' | 'archive' | 'generate' | 'edit-concept' | 'edit-phrasing' | null
  >(null);

  const setCanonical = useCallback(
    async (phrasingId: string | null) => {
      try {
        setPendingAction('canonical');
        await setCanonicalMutation({
          conceptId,
          phrasingId: phrasingId ?? undefined,
        });
        toast.success(phrasingId ? 'Canonical phrasing updated' : 'Canonical phrasing cleared');
      } catch (error) {
        const message =
          error instanceof Error ? error.message : 'Failed to update canonical phrasing';
        toast.error(message);
      } finally {
        setPendingAction((action) => (action === 'canonical' ? null : action));
      }
    },
    [conceptId, setCanonicalMutation]
  );

  const archivePhrasing = useCallback(
    async (phrasingId: string) => {
      try {
        setPendingAction('archive');
        await archivePhrasingMutation({
          conceptId,
          phrasingId,
        });
        toast.success('Phrasing archived');
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Failed to archive phrasing';
        toast.error(message);
      } finally {
        setPendingAction((action) => (action === 'archive' ? null : action));
      }
    },
    [conceptId, archivePhrasingMutation]
  );

  const requestMorePhrasings = useCallback(async () => {
    try {
      setPendingAction('generate');
      await requestGenerationMutation({
        conceptId,
      });
      toast.success('Generation job started');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Failed to start generation';
      toast.error(message);
    } finally {
      setPendingAction((action) => (action === 'generate' ? null : action));
    }
  }, [conceptId, requestGenerationMutation]);

  const editConcept = useCallback(
    async (data: { title: string; description?: string }) => {
      try {
        setPendingAction('edit-concept');
        await updateConceptMutation({
          conceptId,
          ...data,
        });
        toast.success('Concept updated');
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Failed to update concept';
        toast.error(message);
        throw error;
      } finally {
        setPendingAction((action) => (action === 'edit-concept' ? null : action));
      }
    },
    [conceptId, updateConceptMutation]
  );

  const editPhrasing = useCallback(
    async (data: {
      phrasingId: string;
      question: string;
      correctAnswer: string;
      explanation?: string;
      options?: string[];
    }) => {
      try {
        setPendingAction('edit-phrasing');
        await updatePhrasingMutation({
          ...data,
        });
        toast.success('Phrasing updated');
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Failed to update phrasing';
        toast.error(message);
        throw error;
      } finally {
        setPendingAction((action) => (action === 'edit-phrasing' ? null : action));
      }
    },
    [updatePhrasingMutation]
  );

  const archivePhrasingWithUndo = useCallback(
    async (phrasingId: string) => {
      await undoableAction({
        action: () =>
          archivePhrasingMutation({
            conceptId,
            phrasingId,
          }),
        message: 'Phrasing archived',
        undo: () =>
          unarchivePhrasingMutation({
            conceptId,
            phrasingId,
          }),
        duration: 8000,
      });
    },
    [conceptId, archivePhrasingMutation, unarchivePhrasingMutation, undoableAction]
  );

  const archiveConceptWithUndo = useCallback(async () => {
    await undoableAction({
      action: () =>
        archiveConceptMutation({
          conceptId,
        }),
      message: 'Concept archived',
      undo: () =>
        unarchiveConceptMutation({
          conceptId,
        }),
      duration: 8000,
    });
  }, [conceptId, archiveConceptMutation, unarchiveConceptMutation, undoableAction]);

  return {
    setCanonical,
    archivePhrasing,
    archivePhrasingWithUndo,
    archiveConceptWithUndo,
    requestMorePhrasings,
    editConcept,
    editPhrasing,
    isSettingCanonical: pendingAction === 'canonical',
    isArchiving: pendingAction === 'archive',
    isRequestingGeneration: pendingAction === 'generate',
    isEditingConcept: pendingAction === 'edit-concept',
    isEditingPhrasing: pendingAction === 'edit-phrasing',
  };
}
