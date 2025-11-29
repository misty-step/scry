/**
 * Unified validation for concept and phrasing edits during review.
 *
 * Consolidates validation rules from PhrasingEditForm and adds concept validation.
 * Returns field-level errors for precise UI feedback.
 */

export interface UnifiedEditData {
  // Concept fields
  conceptTitle: string;

  // Phrasing fields
  question: string;
  correctAnswer: string;
  explanation: string;
  options: string[];
}

export interface ValidationError {
  field: keyof UnifiedEditData;
  message: string;
}

export type QuestionType = 'multiple-choice' | 'true-false';

/**
 * Validates unified edit data for both concept and phrasing.
 *
 * @param data - The unified edit data containing both concept and phrasing fields
 * @param questionType - The type of question (affects phrasing validation)
 * @returns Array of field-level validation errors (empty if valid)
 */
export function validateUnifiedEdit(
  data: UnifiedEditData,
  questionType: QuestionType
): ValidationError[] {
  const errors: ValidationError[] = [];

  // ============================================================================
  // Concept Validation
  // ============================================================================

  // Rule 1: Concept title must be non-empty
  if (!data.conceptTitle.trim()) {
    errors.push({
      field: 'conceptTitle',
      message: 'Concept title cannot be empty',
    });
  }

  // ============================================================================
  // Phrasing Validation (from PhrasingEditForm lines 44-79)
  // ============================================================================

  // Rule 2: Question text must be non-empty
  if (!data.question.trim()) {
    errors.push({
      field: 'question',
      message: 'Question text is required',
    });
  }

  // Rule 3: Multiple-choice specific validation
  if (questionType === 'multiple-choice') {
    // At least 2 options required
    if (data.options.length < 2) {
      errors.push({
        field: 'options',
        message: 'At least 2 answer options are required',
      });
    }

    // Maximum 6 options allowed
    if (data.options.length > 6) {
      errors.push({
        field: 'options',
        message: 'Maximum 6 answer options allowed',
      });
    }

    // All options must have text (no empty options)
    const nonEmptyOptions = data.options.filter((opt) => opt.trim());
    if (nonEmptyOptions.length !== data.options.length) {
      errors.push({
        field: 'options',
        message: 'All answer options must have text',
      });
    }

    // Correct answer must exist in options
    if (!data.options.includes(data.correctAnswer)) {
      errors.push({
        field: 'correctAnswer',
        message: 'Correct answer must be one of the options',
      });
    }
  }

  // Rule 4: True-false specific validation
  if (questionType === 'true-false') {
    if (!['True', 'False'].includes(data.correctAnswer)) {
      errors.push({
        field: 'correctAnswer',
        message: 'Correct answer must be True or False',
      });
    }
  }

  return errors;
}

/**
 * Maps validation errors to a Record<field, message> for easier consumption.
 *
 * Useful for displaying errors in forms where each field shows its own error.
 *
 * @param errors - Array of validation errors
 * @returns Object mapping field names to error messages
 */
export function validationErrorsToRecord(errors: ValidationError[]): Record<string, string> {
  const record: Record<string, string> = {};

  for (const error of errors) {
    // If multiple errors for same field, concatenate with semicolon
    if (record[error.field]) {
      record[error.field] += `; ${error.message}`;
    } else {
      record[error.field] = error.message;
    }
  }

  return record;
}
