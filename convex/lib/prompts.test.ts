import { beforeEach, describe, expect, it, vi } from 'vitest';
import { isLangfuseConfigured } from './langfuse';
import { getPrompt, getPrompts } from './prompts';

// Mock the langfuse module
const mockGetPrompt = vi.fn();
const mockCompile = vi.fn();

vi.mock('./langfuse', () => ({
  isLangfuseConfigured: vi.fn(),
  getLangfuse: vi.fn(() => ({
    getPrompt: mockGetPrompt,
  })),
}));

describe('prompts module', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(isLangfuseConfigured).mockReturnValue(false);
  });

  describe('getPrompt with fallback', () => {
    it('uses fallback when Langfuse is not configured', async () => {
      vi.mocked(isLangfuseConfigured).mockReturnValue(false);

      const result = await getPrompt('scry-intent-extraction', { userInput: 'test input' });

      expect(result.source).toBe('fallback');
      expect(result.promptId).toBe('scry-intent-extraction');
      expect(result.text).toContain('test input');
      expect(result.version).toBeUndefined();
    });

    it('generates fallback for intent extraction prompt', async () => {
      vi.mocked(isLangfuseConfigured).mockReturnValue(false);

      const result = await getPrompt('scry-intent-extraction', { userInput: 'NATO alphabet' });

      expect(result.source).toBe('fallback');
      expect(result.text).toContain('NATO alphabet');
    });

    it('generates fallback for concept synthesis prompt', async () => {
      vi.mocked(isLangfuseConfigured).mockReturnValue(false);
      const intentJson = JSON.stringify({ topic: 'test', items: [] });

      const result = await getPrompt('scry-concept-synthesis', { intentJson });

      expect(result.source).toBe('fallback');
      expect(result.text).toContain(intentJson);
    });

    it('generates fallback for phrasing generation prompt', async () => {
      vi.mocked(isLangfuseConfigured).mockReturnValue(false);

      const result = await getPrompt('scry-phrasing-generation', {
        conceptTitle: 'Test Concept',
        contentType: 'verbatim',
        originIntent: 'Learn about testing',
        targetCount: 5,
        existingQuestions: ['What is a test?'],
      });

      expect(result.source).toBe('fallback');
      expect(result.text).toContain('Test Concept');
    });
  });

  describe('getPrompt with Langfuse', () => {
    it('fetches prompt from Langfuse when configured', async () => {
      vi.mocked(isLangfuseConfigured).mockReturnValue(true);
      mockCompile.mockReturnValue('compiled prompt text');
      mockGetPrompt.mockResolvedValue({
        compile: mockCompile,
        version: 2,
      });

      const result = await getPrompt('scry-intent-extraction', { userInput: 'test' });

      expect(mockGetPrompt).toHaveBeenCalledWith('scry-intent-extraction', undefined, {
        label: 'latest',
      });
      expect(result.source).toBe('langfuse');
      expect(result.text).toBe('compiled prompt text');
      expect(result.version).toBe('2');
    });

    it('uses custom label when provided', async () => {
      vi.mocked(isLangfuseConfigured).mockReturnValue(true);
      mockCompile.mockReturnValue('staging prompt');
      mockGetPrompt.mockResolvedValue({
        compile: mockCompile,
        version: 1,
      });

      await getPrompt('scry-intent-extraction', { userInput: 'test' }, 'staging');

      expect(mockGetPrompt).toHaveBeenCalledWith('scry-intent-extraction', undefined, {
        label: 'staging',
      });
    });

    it('falls back on Langfuse error', async () => {
      vi.mocked(isLangfuseConfigured).mockReturnValue(true);
      mockGetPrompt.mockRejectedValue(new Error('Network error'));
      const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

      const result = await getPrompt('scry-intent-extraction', { userInput: 'test' });

      expect(result.source).toBe('fallback');
      expect(result.text).toContain('test');
      expect(consoleSpy).toHaveBeenCalledWith(
        expect.stringContaining('Langfuse prompt fetch failed'),
        expect.any(Error)
      );

      consoleSpy.mockRestore();
    });

    it('serializes non-string variables to JSON', async () => {
      vi.mocked(isLangfuseConfigured).mockReturnValue(true);
      mockCompile.mockReturnValue('compiled');
      mockGetPrompt.mockResolvedValue({
        compile: mockCompile,
        version: 1,
      });

      await getPrompt('scry-phrasing-generation', {
        conceptTitle: 'Test',
        contentType: 'verbatim',
        targetCount: 5,
        existingQuestions: ['q1', 'q2'],
      });

      // Check that compile was called with serialized arrays
      expect(mockCompile).toHaveBeenCalledWith(
        expect.objectContaining({
          conceptTitle: 'Test',
          existingQuestions: '["q1","q2"]',
        })
      );
    });
  });

  describe('getPrompts batch', () => {
    it('fetches multiple prompts in parallel', async () => {
      vi.mocked(isLangfuseConfigured).mockReturnValue(false);

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
  });
});
