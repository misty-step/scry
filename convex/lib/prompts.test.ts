import { describe, expect, it } from 'vitest';
import { getPrompt, getPrompts } from './prompts';

describe('prompts module', () => {
  describe('getPrompt', () => {
    it('returns template source for intent extraction', async () => {
      const result = await getPrompt('scry-intent-extraction', { userInput: 'test input' });

      expect(result.source).toBe('template');
      expect(result.promptId).toBe('scry-intent-extraction');
      expect(result.label).toBe('production');
      expect(result.text).toContain('test input');
    });

    it('generates intent extraction prompt with user input', async () => {
      const result = await getPrompt('scry-intent-extraction', { userInput: 'NATO alphabet' });

      expect(result.text).toContain('NATO alphabet');
      expect(result.text).toContain('content_type');
    });

    it('generates concept synthesis prompt with intent JSON', async () => {
      const intentJson = JSON.stringify({ topic: 'test', items: [] });

      const result = await getPrompt('scry-concept-synthesis', { intentJson });

      expect(result.source).toBe('template');
      expect(result.text).toContain(intentJson);
    });

    it('generates phrasing generation prompt with all parameters', async () => {
      const result = await getPrompt('scry-phrasing-generation', {
        conceptTitle: 'Test Concept',
        contentType: 'verbatim',
        originIntent: 'Learn about testing',
        targetCount: 5,
        existingQuestions: ['What is a test?'],
      });

      expect(result.source).toBe('template');
      expect(result.text).toContain('Test Concept');
      expect(result.text).toContain('verbatim');
      expect(result.text).toContain('5');
    });

    it('handles phrasing generation with empty existing questions', async () => {
      const result = await getPrompt('scry-phrasing-generation', {
        conceptTitle: 'New Concept',
        targetCount: 3,
        existingQuestions: [],
      });

      expect(result.text).toContain('New Concept');
      expect(result.text).toContain('None');
    });
  });

  describe('getPrompts batch', () => {
    it('fetches multiple prompts in parallel', async () => {
      const results = await getPrompts([
        { promptId: 'scry-intent-extraction', variables: { userInput: 'test1' } },
        { promptId: 'scry-intent-extraction', variables: { userInput: 'test2' } },
      ]);

      expect(results).toHaveLength(2);
      expect(results[0].text).toContain('test1');
      expect(results[1].text).toContain('test2');
    });

    it('returns empty array for empty input', async () => {
      const results = await getPrompts([]);

      expect(results).toEqual([]);
    });

    it('handles mixed prompt types', async () => {
      const results = await getPrompts([
        { promptId: 'scry-intent-extraction', variables: { userInput: 'test' } },
        { promptId: 'scry-concept-synthesis', variables: { intentJson: '{}' } },
      ]);

      expect(results).toHaveLength(2);
      expect(results[0].promptId).toBe('scry-intent-extraction');
      expect(results[1].promptId).toBe('scry-concept-synthesis');
    });
  });
});
