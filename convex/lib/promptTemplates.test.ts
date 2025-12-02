import { describe, expect, it } from 'vitest';
import {
  buildConceptSynthesisPrompt,
  buildIntentExtractionPrompt,
  buildLearningSciencePrompt,
  buildPhrasingGenerationPrompt,
} from './promptTemplates';

describe('buildIntentExtractionPrompt', () => {
  it('includes user input in the prompt', () => {
    const result = buildIntentExtractionPrompt('Learn the periodic table');
    expect(result).toContain('Learn the periodic table');
  });

  it('includes content type options', () => {
    const result = buildIntentExtractionPrompt('test input');
    expect(result).toContain('verbatim');
    expect(result).toContain('enumerable');
    expect(result).toContain('conceptual');
    expect(result).toContain('mixed');
  });

  it('includes goal options', () => {
    const result = buildIntentExtractionPrompt('test input');
    expect(result).toContain('memorize');
    expect(result).toContain('understand');
    expect(result).toContain('apply');
  });

  it('mentions required output fields', () => {
    const result = buildIntentExtractionPrompt('test');
    expect(result).toContain('atomic_units');
    expect(result).toContain('synthesis_ops');
    expect(result).toContain('confidence');
  });

  it('escapes special characters in input', () => {
    const input = 'Test with "quotes" and special chars';
    const result = buildIntentExtractionPrompt(input);
    expect(result).toContain(input);
  });
});

describe('buildConceptSynthesisPrompt', () => {
  it('includes the intent JSON', () => {
    const intentJson = '{"content_type":"conceptual","goal":"understand"}';
    const result = buildConceptSynthesisPrompt(intentJson);
    expect(result).toContain(intentJson);
  });

  it('mentions concept generation rules', () => {
    const result = buildConceptSynthesisPrompt('{}');
    expect(result).toContain('atomic concepts');
    expect(result).toContain('Title');
    expect(result).toContain('description');
  });

  it('includes max concepts constraint', () => {
    const result = buildConceptSynthesisPrompt('{}');
    expect(result).toMatch(/cap at \d+ highest-value atoms/);
  });

  it('specifies output format', () => {
    const result = buildConceptSynthesisPrompt('{}');
    expect(result).toContain('concepts');
    expect(result).toContain('contentType');
    expect(result).toContain('originIntent');
  });
});

describe('buildPhrasingGenerationPrompt', () => {
  it('includes concept title', () => {
    const result = buildPhrasingGenerationPrompt({
      conceptTitle: 'Photosynthesis Process',
      targetCount: 3,
      existingQuestions: [],
    });
    expect(result).toContain('Photosynthesis Process');
  });

  it('includes target count', () => {
    const result = buildPhrasingGenerationPrompt({
      conceptTitle: 'Test',
      targetCount: 5,
      existingQuestions: [],
    });
    expect(result).toContain('Generate 5 quiz-ready phrasings');
  });

  it('shows existing questions block when empty', () => {
    const result = buildPhrasingGenerationPrompt({
      conceptTitle: 'Test',
      targetCount: 2,
      existingQuestions: [],
    });
    expect(result).toContain('None (generate first phrasings');
  });

  it('includes numbered existing questions', () => {
    const result = buildPhrasingGenerationPrompt({
      conceptTitle: 'Test',
      targetCount: 2,
      existingQuestions: ['What is X?', 'Why does Y happen?'],
    });
    expect(result).toContain('1. What is X?');
    expect(result).toContain('2. Why does Y happen?');
  });

  it('includes content type when provided', () => {
    const result = buildPhrasingGenerationPrompt({
      conceptTitle: 'Test',
      contentType: 'verbatim',
      targetCount: 1,
      existingQuestions: [],
    });
    expect(result).toContain('Content Type: verbatim');
  });

  it('shows unspecified when content type not provided', () => {
    const result = buildPhrasingGenerationPrompt({
      conceptTitle: 'Test',
      targetCount: 1,
      existingQuestions: [],
    });
    expect(result).toContain('Content Type: unspecified');
  });

  it('includes origin intent when provided', () => {
    const result = buildPhrasingGenerationPrompt({
      conceptTitle: 'Test',
      originIntent: '{"goal":"memorize"}',
      targetCount: 1,
      existingQuestions: [],
    });
    expect(result).toContain('Origin Intent: {"goal":"memorize"}');
  });

  it('includes output format specification', () => {
    const result = buildPhrasingGenerationPrompt({
      conceptTitle: 'Test',
      targetCount: 1,
      existingQuestions: [],
    });
    expect(result).toContain('phrasings');
    expect(result).toContain('question');
    expect(result).toContain('explanation');
    expect(result).toContain('correctAnswer');
  });
});

describe('buildLearningSciencePrompt', () => {
  it('includes user input', () => {
    const result = buildLearningSciencePrompt('Learn Python basics');
    expect(result).toContain('Learn Python basics');
  });

  it('describes the three-step process', () => {
    const result = buildLearningSciencePrompt('test');
    expect(result).toContain('three-step plan');
    expect(result).toContain('Classify');
    expect(result).toContain('atomic concepts');
    expect(result).toContain('MC/TF questions');
  });

  it('mentions spaced repetition', () => {
    const result = buildLearningSciencePrompt('test');
    expect(result).toContain('spaced repetition');
  });
});
