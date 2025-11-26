'use client';

import { useState } from 'react';
import { AlertCircle, Plus, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Textarea } from '@/components/ui/textarea';
import type { useInlineEdit } from '@/hooks/use-inline-edit';

interface PhrasingEditFormProps {
  questionType: 'multiple-choice' | 'true-false';
  editState: ReturnType<
    typeof useInlineEdit<{
      question: string;
      correctAnswer: string;
      explanation: string;
      options: string[];
    }>
  >;
}

/**
 * Deep module for inline phrasing editing during review.
 *
 * **Hidden Complexity** (8:1 functionality/interface ratio):
 * - Validation: 5 rules (question, options count, correctAnswer in options, non-empty fields)
 * - Edge case handling: Removing correct option, changing correct option text
 * - Question type branching: Multiple-choice vs True-false UI
 * - Option management: Add/remove with automatic correctAnswer updates
 *
 * **Simple Interface**: Just questionType and editState hook
 *
 * **Information Hiding**: Validation errors, option manipulation logic, edge cases
 * all handled internally. Parent only provides data and receives updates via hook.
 */
export function PhrasingEditForm({ questionType, editState }: PhrasingEditFormProps) {
  const [validationErrors, setValidationErrors] = useState<string[]>([]);

  // ============================================================================
  // Validation (Internal - mirrors backend + EditQuestionModal)
  // ============================================================================

  const validateBeforeSave = (): boolean => {
    const errors: string[] = [];
    const { question, correctAnswer, options } = editState.localData;

    // Rule 1: Non-empty question
    if (!question.trim()) {
      errors.push('Question text is required');
    }

    // Rule 2: Multiple-choice validation
    if (questionType === 'multiple-choice') {
      if (options.length < 2) {
        errors.push('At least 2 answer options are required');
      }
      if (options.length > 6) {
        errors.push('Maximum 6 answer options allowed');
      }
      const nonEmptyOptions = options.filter((opt) => opt.trim());
      if (nonEmptyOptions.length !== options.length) {
        errors.push('All answer options must have text');
      }
      if (!options.includes(correctAnswer)) {
        errors.push('Correct answer must be one of the options');
      }
    }

    // Rule 3: True-false validation
    if (questionType === 'true-false') {
      if (!['True', 'False'].includes(correctAnswer)) {
        errors.push('Correct answer must be True or False');
      }
    }

    setValidationErrors(errors);
    return errors.length === 0;
  };

  // ============================================================================
  // Option Management (Internal - handles edge cases)
  // ============================================================================

  const handleAddOption = () => {
    if (editState.localData.options.length >= 6) return;
    editState.updateField('options', [...editState.localData.options, '']);
  };

  const handleRemoveOption = (index: number) => {
    if (editState.localData.options.length <= 2) return; // Enforce minimum

    const newOptions = editState.localData.options.filter((_, i) => i !== index);
    const removedOption = editState.localData.options[index];

    editState.updateField('options', newOptions);

    // Edge case 1: If removed option was correct, auto-select first remaining
    if (removedOption === editState.localData.correctAnswer) {
      editState.updateField('correctAnswer', newOptions[0]);
    }
  };

  const handleOptionChange = (index: number, newValue: string) => {
    const oldValue = editState.localData.options[index];
    const newOptions = [...editState.localData.options];
    newOptions[index] = newValue;

    editState.updateField('options', newOptions);

    // Edge case 2: Track correct answer by value - update if this was correct
    if (oldValue === editState.localData.correctAnswer) {
      editState.updateField('correctAnswer', newValue);
    }
  };

  // ============================================================================
  // Save Handler (Validates then delegates to hook)
  // ============================================================================

  const handleSave = async () => {
    if (!validateBeforeSave()) {
      return; // Show validation errors, don't proceed
    }

    setValidationErrors([]); // Clear errors on successful validation
    await editState.save();
  };

  // ============================================================================
  // Render
  // ============================================================================

  return (
    <div className="space-y-4">
      {/* Error Display */}
      {validationErrors.length > 0 && (
        <div className="bg-error-background border border-error-border rounded-lg p-3">
          <div className="flex items-start gap-2">
            <AlertCircle className="h-5 w-5 text-error flex-shrink-0 mt-0.5" />
            <div className="space-y-1">
              {validationErrors.map((error, i) => (
                <p key={i} className="text-sm text-error">
                  {error}
                </p>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Question Field */}
      <div className="space-y-2">
        <Label htmlFor="phrasing-question">Question</Label>
        <Textarea
          id="phrasing-question"
          value={editState.localData.question}
          onChange={(e) => editState.updateField('question', e.target.value)}
          placeholder="Enter question text..."
          className="min-h-[100px]"
        />
      </div>

      {/* Multiple-Choice Options */}
      {questionType === 'multiple-choice' && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <Label>Answer Options</Label>
            <Button
              variant="outline"
              size="sm"
              onClick={handleAddOption}
              disabled={editState.localData.options.length >= 6}
            >
              <Plus className="mr-2 h-4 w-4" />
              Add option
            </Button>
          </div>

          <RadioGroup
            value={editState.localData.correctAnswer}
            onValueChange={(val) => editState.updateField('correctAnswer', val)}
          >
            {editState.localData.options.map((option, idx) => (
              <div key={idx} className="flex items-start gap-3 border rounded-lg p-3">
                <RadioGroupItem value={option} id={`opt-${idx}`} className="mt-2" />
                <div className="flex-1">
                  <Label htmlFor={`opt-${idx}-text`} className="text-xs text-muted-foreground">
                    Option {idx + 1}
                  </Label>
                  <Textarea
                    id={`opt-${idx}-text`}
                    value={option}
                    onChange={(e) => handleOptionChange(idx, e.target.value)}
                    className="min-h-[60px] mt-1"
                    placeholder={`Enter option ${idx + 1}...`}
                  />
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => handleRemoveOption(idx)}
                  disabled={editState.localData.options.length <= 2}
                  className="text-muted-foreground hover:text-destructive"
                  aria-label={`Remove option ${idx + 1}`}
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </RadioGroup>
        </div>
      )}

      {/* True-False Selection */}
      {questionType === 'true-false' && (
        <div className="space-y-2">
          <Label>Correct Answer</Label>
          <RadioGroup
            value={editState.localData.correctAnswer}
            onValueChange={(val) => editState.updateField('correctAnswer', val)}
            className="space-y-2"
          >
            <div className="flex items-center gap-2">
              <RadioGroupItem value="True" id="tf-true" />
              <Label htmlFor="tf-true" className="font-normal cursor-pointer">
                True
              </Label>
            </div>
            <div className="flex items-center gap-2">
              <RadioGroupItem value="False" id="tf-false" />
              <Label htmlFor="tf-false" className="font-normal cursor-pointer">
                False
              </Label>
            </div>
          </RadioGroup>
        </div>
      )}

      {/* Internal Save Button (validation before delegating to hook) */}
      <div className="pt-2">
        <Button
          onClick={handleSave}
          disabled={editState.isSaving || !editState.isDirty}
          className="w-full sm:w-auto"
        >
          {editState.isSaving ? 'Validating & Saving...' : 'Save Changes'}
        </Button>
      </div>
    </div>
  );
}
