'use client';

import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';

interface TrueFalseEditorProps {
  correctAnswer: string;
  onCorrectAnswerChange: (answer: string) => void;
}

/**
 * True/False answer selector component.
 *
 * Extracted from PhrasingEditForm for reuse in inline editing.
 *
 * Simple radio group for selecting "True" or "False" as the correct answer.
 */
export function TrueFalseEditor({ correctAnswer, onCorrectAnswerChange }: TrueFalseEditorProps) {
  return (
    <div className="space-y-2">
      <Label>Correct Answer</Label>
      <RadioGroup value={correctAnswer} onValueChange={onCorrectAnswerChange} className="space-y-2">
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
  );
}
