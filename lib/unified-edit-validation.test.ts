import { describe, expect, it } from 'vitest';
import {
  validateUnifiedEdit,
  validationErrorsToRecord,
  type UnifiedEditData,
} from './unified-edit-validation';

describe('validateUnifiedEdit', () => {
  // Helper to create valid test data
  const createValidData = (overrides?: Partial<UnifiedEditData>): UnifiedEditData => ({
    conceptTitle: 'Test Concept',
    conceptDescription: 'Test description',
    question: 'What is 2 + 2?',
    correctAnswer: '4',
    explanation: 'Two plus two equals four',
    options: ['3', '4', '5', '6'],
    ...overrides,
  });

  describe('Concept Validation', () => {
    it('should pass validation for valid concept title', () => {
      const data = createValidData();
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      const conceptErrors = errors.filter((e) => e.field === 'conceptTitle');
      expect(conceptErrors).toHaveLength(0);
    });

    it('should fail when concept title is empty string', () => {
      const data = createValidData({ conceptTitle: '' });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      const conceptErrors = errors.filter((e) => e.field === 'conceptTitle');
      expect(conceptErrors).toHaveLength(1);
      expect(conceptErrors[0].message).toBe('Concept title cannot be empty');
    });

    it('should fail when concept title is only whitespace', () => {
      const data = createValidData({ conceptTitle: '   ' });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      const conceptErrors = errors.filter((e) => e.field === 'conceptTitle');
      expect(conceptErrors).toHaveLength(1);
    });

    it('should allow empty concept description (optional field)', () => {
      const data = createValidData({ conceptDescription: '' });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      const descErrors = errors.filter((e) => e.field === 'conceptDescription');
      expect(descErrors).toHaveLength(0);
    });
  });

  describe('Phrasing Validation - Question', () => {
    it('should pass validation for valid question text', () => {
      const data = createValidData();
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      const questionErrors = errors.filter((e) => e.field === 'question');
      expect(questionErrors).toHaveLength(0);
    });

    it('should fail when question is empty string', () => {
      const data = createValidData({ question: '' });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      const questionErrors = errors.filter((e) => e.field === 'question');
      expect(questionErrors).toHaveLength(1);
      expect(questionErrors[0].message).toBe('Question text is required');
    });

    it('should fail when question is only whitespace', () => {
      const data = createValidData({ question: '   \n\t  ' });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      const questionErrors = errors.filter((e) => e.field === 'question');
      expect(questionErrors).toHaveLength(1);
    });
  });

  describe('Multiple-Choice Validation', () => {
    it('should pass validation for valid multiple-choice question', () => {
      const data = createValidData({
        options: ['Option 1', 'Option 2', 'Option 3'],
        correctAnswer: 'Option 2',
      });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      expect(errors).toHaveLength(0);
    });

    it('should fail when options array has less than 2 items', () => {
      const data = createValidData({
        options: ['Only one option'],
        correctAnswer: 'Only one option',
      });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      const optionErrors = errors.filter((e) => e.field === 'options');
      expect(optionErrors.length).toBeGreaterThan(0);
      expect(optionErrors[0].message).toBe('At least 2 answer options are required');
    });

    it('should fail when options array has more than 6 items', () => {
      const data = createValidData({
        options: ['1', '2', '3', '4', '5', '6', '7'],
        correctAnswer: '1',
      });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      const optionErrors = errors.filter((e) => e.field === 'options');
      expect(optionErrors.some((e) => e.message === 'Maximum 6 answer options allowed')).toBe(true);
    });

    it('should fail when any option is empty', () => {
      const data = createValidData({
        options: ['Option 1', '', 'Option 3'],
        correctAnswer: 'Option 1',
      });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      const optionErrors = errors.filter((e) => e.field === 'options');
      expect(optionErrors.some((e) => e.message === 'All answer options must have text')).toBe(
        true
      );
    });

    it('should fail when any option is only whitespace', () => {
      const data = createValidData({
        options: ['Option 1', '  \t  ', 'Option 3'],
        correctAnswer: 'Option 1',
      });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      const optionErrors = errors.filter((e) => e.field === 'options');
      expect(optionErrors.some((e) => e.message === 'All answer options must have text')).toBe(
        true
      );
    });

    it('should fail when correct answer is not in options array', () => {
      const data = createValidData({
        options: ['Option 1', 'Option 2', 'Option 3'],
        correctAnswer: 'Non-existent option',
      });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      const answerErrors = errors.filter((e) => e.field === 'correctAnswer');
      expect(answerErrors).toHaveLength(1);
      expect(answerErrors[0].message).toBe('Correct answer must be one of the options');
    });

    it('should pass with exactly 2 options (minimum)', () => {
      const data = createValidData({
        options: ['Yes', 'No'],
        correctAnswer: 'Yes',
      });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      expect(errors).toHaveLength(0);
    });

    it('should pass with exactly 6 options (maximum)', () => {
      const data = createValidData({
        options: ['1', '2', '3', '4', '5', '6'],
        correctAnswer: '3',
      });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      expect(errors).toHaveLength(0);
    });
  });

  describe('True-False Validation', () => {
    it('should pass validation for valid true-false question with True', () => {
      const data = createValidData({
        correctAnswer: 'True',
      });
      const errors = validateUnifiedEdit(data, 'true-false');

      const answerErrors = errors.filter((e) => e.field === 'correctAnswer');
      expect(answerErrors).toHaveLength(0);
    });

    it('should pass validation for valid true-false question with False', () => {
      const data = createValidData({
        correctAnswer: 'False',
      });
      const errors = validateUnifiedEdit(data, 'true-false');

      const answerErrors = errors.filter((e) => e.field === 'correctAnswer');
      expect(answerErrors).toHaveLength(0);
    });

    it('should fail when true-false answer is not True or False', () => {
      const data = createValidData({
        correctAnswer: 'Maybe',
      });
      const errors = validateUnifiedEdit(data, 'true-false');

      const answerErrors = errors.filter((e) => e.field === 'correctAnswer');
      expect(answerErrors).toHaveLength(1);
      expect(answerErrors[0].message).toBe('Correct answer must be True or False');
    });

    it('should fail with lowercase true/false', () => {
      const data = createValidData({
        correctAnswer: 'true', // lowercase
      });
      const errors = validateUnifiedEdit(data, 'true-false');

      const answerErrors = errors.filter((e) => e.field === 'correctAnswer');
      expect(answerErrors).toHaveLength(1);
    });

    it('should not validate options for true-false questions', () => {
      // True-false questions don't need options validation
      const data = createValidData({
        correctAnswer: 'True',
        options: [], // Empty options array should be fine
      });
      const errors = validateUnifiedEdit(data, 'true-false');

      const optionErrors = errors.filter((e) => e.field === 'options');
      expect(optionErrors).toHaveLength(0);
    });
  });

  describe('Multiple Validation Errors', () => {
    it('should return multiple errors when both concept and phrasing are invalid', () => {
      const data = createValidData({
        conceptTitle: '',
        question: '',
      });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      expect(errors.length).toBeGreaterThanOrEqual(2);
      expect(errors.some((e) => e.field === 'conceptTitle')).toBe(true);
      expect(errors.some((e) => e.field === 'question')).toBe(true);
    });

    it('should return all applicable validation errors', () => {
      const data = createValidData({
        conceptTitle: '   ',
        question: '',
        options: ['Only one'],
        correctAnswer: 'Wrong answer',
      });
      const errors = validateUnifiedEdit(data, 'multiple-choice');

      // Should have errors for:
      // - conceptTitle (whitespace)
      // - question (empty)
      // - options (less than 2)
      // - correctAnswer (not in options)
      expect(errors.length).toBeGreaterThanOrEqual(4);
    });
  });
});

describe('validationErrorsToRecord', () => {
  it('should convert errors array to field->message record', () => {
    const errors = [
      { field: 'conceptTitle' as const, message: 'Title is required' },
      { field: 'question' as const, message: 'Question is required' },
    ];

    const record = validationErrorsToRecord(errors);

    expect(record).toEqual({
      conceptTitle: 'Title is required',
      question: 'Question is required',
    });
  });

  it('should concatenate multiple errors for same field', () => {
    const errors = [
      { field: 'options' as const, message: 'At least 2 options required' },
      { field: 'options' as const, message: 'All options must have text' },
    ];

    const record = validationErrorsToRecord(errors);

    expect(record.options).toBe('At least 2 options required; All options must have text');
  });

  it('should return empty object for empty errors array', () => {
    const errors: never[] = [];
    const record = validationErrorsToRecord(errors);

    expect(record).toEqual({});
  });

  it('should handle single error', () => {
    const errors = [{ field: 'question' as const, message: 'Question text is required' }];

    const record = validationErrorsToRecord(errors);

    expect(record).toEqual({
      question: 'Question text is required',
    });
  });
});
