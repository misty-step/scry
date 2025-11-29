'use client';

import { Plus, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Textarea } from '@/components/ui/textarea';

interface OptionsEditorProps {
  options: string[];
  correctAnswer: string;
  onOptionsChange: (options: string[]) => void;
  onCorrectAnswerChange: (answer: string) => void;
  minOptions?: number;
  maxOptions?: number;
}

/**
 * Multiple-choice options editor component.
 *
 * Extracted from PhrasingEditForm for reuse in inline editing.
 *
 * **Features:**
 * - Add/remove options (with min/max constraints)
 * - Select correct answer via radio buttons
 * - Automatic correct answer tracking when option text changes
 * - Edge case handling: removing correct option auto-selects first remaining
 */
export function OptionsEditor({
  options,
  correctAnswer,
  onOptionsChange,
  onCorrectAnswerChange,
  minOptions = 2,
  maxOptions = 6,
}: OptionsEditorProps) {
  const handleAddOption = () => {
    if (options.length >= maxOptions) return;
    onOptionsChange([...options, '']);
  };

  const handleRemoveOption = (index: number) => {
    if (options.length <= minOptions) return;

    const newOptions = options.filter((_, i) => i !== index);
    const removedOption = options[index];

    onOptionsChange(newOptions);

    // Edge case: If removed option was correct, auto-select first remaining
    if (removedOption === correctAnswer) {
      onCorrectAnswerChange(newOptions[0]);
    }
  };

  const handleOptionChange = (index: number, newValue: string) => {
    const oldValue = options[index];
    const newOptions = [...options];
    newOptions[index] = newValue;

    onOptionsChange(newOptions);

    // Edge case: Track correct answer by value - update if this was correct
    if (oldValue === correctAnswer) {
      onCorrectAnswerChange(newValue);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <Label>Answer Options</Label>
        <Button
          variant="outline"
          size="sm"
          onClick={handleAddOption}
          disabled={options.length >= maxOptions}
        >
          <Plus className="mr-2 h-4 w-4" />
          Add option
        </Button>
      </div>

      <RadioGroup value={correctAnswer} onValueChange={onCorrectAnswerChange}>
        <p className="text-sm text-muted-foreground mb-2">Select the correct answer:</p>
        {options.map((option, idx) => (
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
              disabled={options.length <= minOptions}
              className="text-muted-foreground hover:text-destructive"
              aria-label={`Remove option ${idx + 1}`}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        ))}
      </RadioGroup>
    </div>
  );
}
