'use client';

import { AlertCircle, Info } from 'lucide-react';
import { OptionsEditor } from '@/components/review/options-editor';
import { TrueFalseEditor } from '@/components/review/true-false-editor';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import type { useUnifiedEdit } from '@/hooks/use-unified-edit';
import type { QuestionType } from '@/lib/unified-edit-validation';

interface UnifiedEditFormProps {
  /** Type of question ('multiple-choice' | 'true-false') - determines answer editor UI */
  questionType: QuestionType;
  /** State object from useUnifiedEdit hook containing all form state and actions */
  editState: ReturnType<typeof useUnifiedEdit>;
}

/**
 * Unified edit form for editing both concept and phrasing fields in one interface.
 *
 * This component replaces the previous dual-edit pattern (separate concept/phrasing editors)
 * with a single unified form that intelligently saves only what changed. Used within the
 * review flow when user clicks "Edit" or presses E key.
 *
 * **UX Improvement:**
 * - Before: Two separate edit buttons → Two separate forms → User confusion about which to use
 * - After: Single "Edit" button → One unified form → Clear and intuitive
 *
 * **Key Features:**
 * - **Smart dirty detection**: Only saves concept if title/description changed, only saves
 *   phrasing if question/answer/explanation/options changed
 * - **Field-level errors**: Each input shows aria-invalid when it has validation/mutation errors
 * - **FSRS preservation**: Tooltip educates users that FSRS scheduling state is preserved
 *   (stability, difficulty, nextReview all unchanged)
 * - **Parallel mutations**: When both concept and phrasing are dirty, saves execute in parallel
 *   via Promise.all for 50% latency reduction
 *
 * **Layout Structure:**
 * 1. Error summary (if validation/mutation errors exist)
 * 2. Concept section - Title (required), Description (optional)
 * 3. Phrasing section - Question (required), Answer editor (type-specific), Explanation (required)
 * 4. Action buttons - Save (with FSRS tooltip), Cancel
 *
 * **Answer Editors:**
 * - Multiple-choice: OptionsEditor component (add/remove/reorder options, select correct)
 * - True-false: TrueFalseEditor component (radio buttons for True/False)
 *
 * @example
 * ```tsx
 * const editState = useUnifiedEdit(initialData, onSaveConcept, onSavePhrasing, 'multiple-choice');
 *
 * {editState.isEditing && (
 *   <UnifiedEditForm
 *     questionType="multiple-choice"
 *     editState={editState}
 *   />
 * )}
 * ```
 */
export function UnifiedEditForm({ questionType, editState }: UnifiedEditFormProps) {
  const hasErrors = Object.keys(editState.errors).length > 0;

  return (
    <div className="space-y-4 p-4 bg-muted/30 rounded-lg border border-border/50">
      {/* Field-level Error Display */}
      {hasErrors && (
        <div className="bg-destructive/10 border border-destructive/30 rounded-lg p-3">
          <div className="flex items-start gap-2">
            <AlertCircle className="h-5 w-5 text-destructive flex-shrink-0 mt-0.5" />
            <div className="space-y-1">
              {Object.entries(editState.errors).map(([field, message]) => (
                <p key={field} className="text-sm text-destructive">
                  {message}
                </p>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Concept Section */}
      <div className="space-y-2 pb-3 border-b border-border/30">
        <Label className="text-xs uppercase tracking-wide text-muted-foreground">Concept</Label>

        <div className="space-y-2">
          <Label htmlFor="concept-title">Title</Label>
          <Input
            id="concept-title"
            value={editState.localData.conceptTitle}
            onChange={(e) => editState.updateField('conceptTitle', e.target.value)}
            placeholder="Concept title"
            className="text-xl font-semibold"
            aria-invalid={!!editState.errors.conceptTitle}
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="concept-description">Description (optional)</Label>
          <Textarea
            id="concept-description"
            value={editState.localData.conceptDescription || ''}
            onChange={(e) => editState.updateField('conceptDescription', e.target.value)}
            placeholder="Optional description..."
            className="min-h-[80px]"
            aria-invalid={!!editState.errors.conceptDescription}
          />
        </div>
      </div>

      {/* Phrasing Section */}
      <div className="space-y-3">
        <Label className="text-xs uppercase tracking-wide text-muted-foreground">Phrasing</Label>

        {/* Question Field */}
        <div className="space-y-2">
          <Label htmlFor="question">Question</Label>
          <Textarea
            id="question"
            value={editState.localData.question}
            onChange={(e) => editState.updateField('question', e.target.value)}
            placeholder="Enter question text..."
            className="min-h-[100px]"
            aria-invalid={!!editState.errors.question}
          />
        </div>

        {/* Options (Multiple-Choice) */}
        {questionType === 'multiple-choice' && (
          <OptionsEditor
            options={editState.localData.options}
            correctAnswer={editState.localData.correctAnswer}
            onOptionsChange={(options) => editState.updateField('options', options)}
            onCorrectAnswerChange={(answer) => editState.updateField('correctAnswer', answer)}
          />
        )}

        {/* Correct Answer (True-False) */}
        {questionType === 'true-false' && (
          <TrueFalseEditor
            correctAnswer={editState.localData.correctAnswer}
            onCorrectAnswerChange={(answer) => editState.updateField('correctAnswer', answer)}
          />
        )}

        {/* Explanation */}
        <div className="space-y-2">
          <Label htmlFor="explanation">Explanation (optional)</Label>
          <Textarea
            id="explanation"
            value={editState.localData.explanation}
            onChange={(e) => editState.updateField('explanation', e.target.value)}
            placeholder="Explanation shown after answering (optional)"
            className="min-h-[80px]"
          />
        </div>
      </div>

      {/* Save/Cancel Buttons with FSRS Tooltip */}
      <div className="flex items-center gap-2 pt-2">
        <Button
          onClick={() => editState.save()}
          disabled={editState.isSaving || !editState.isDirty}
        >
          {editState.isSaving ? 'Saving...' : 'Save Changes'}
        </Button>

        <Button variant="ghost" onClick={() => editState.cancel()} disabled={editState.isSaving}>
          Cancel
        </Button>

        {/* FSRS Preservation Tooltip */}
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              className="ml-auto text-muted-foreground hover:text-foreground transition-colors"
              aria-label="Information about FSRS preservation"
            >
              <Info className="h-5 w-5" />
            </button>
          </TooltipTrigger>
          <TooltipContent className="max-w-xs">
            <p className="text-sm">
              <strong>Edits preserve your learning progress.</strong>
            </p>
            <p className="text-xs text-muted-foreground mt-1">
              Your FSRS scheduling state (difficulty, stability, next review date) remains
              unchanged. For major content changes, consider archiving and creating a new concept
              instead.
            </p>
          </TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
