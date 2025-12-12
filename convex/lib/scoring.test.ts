import { generateObject } from 'ai';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { GeneratedPhrasing } from './generationContracts';
import { evaluatePhrasingQuality, isScoringEnabled } from './scoring';

// Mock the ai module
vi.mock('ai', () => ({
  generateObject: vi.fn(),
}));

const mockGenerateObject = vi.mocked(generateObject);

const createMockPhrasing = (overrides: Partial<GeneratedPhrasing> = {}): GeneratedPhrasing => ({
  question: 'What is the capital of France?',
  correctAnswer: 'Paris',
  options: ['Paris', 'London', 'Berlin', 'Madrid'],
  explanation: 'Paris is the capital and largest city of France.',
  type: 'multiple-choice',
  ...overrides,
});

const mockModel = { id: 'test-model' } as any;

const originalEnv = { ...process.env };

describe('scoring module', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    process.env = { ...originalEnv };
    delete process.env.SKIP_SCORING;
  });

  describe('isScoringEnabled', () => {
    it('returns true by default', () => {
      expect(isScoringEnabled()).toBe(true);
    });

    it('returns false when SKIP_SCORING is "true"', () => {
      process.env.SKIP_SCORING = 'true';
      expect(isScoringEnabled()).toBe(false);
    });

    it('returns true when SKIP_SCORING is any other value', () => {
      process.env.SKIP_SCORING = 'false';
      expect(isScoringEnabled()).toBe(true);

      process.env.SKIP_SCORING = '1';
      expect(isScoringEnabled()).toBe(true);
    });
  });

  describe('evaluatePhrasingQuality', () => {
    it('returns sentinel result for empty phrasings', async () => {
      const result = await evaluatePhrasingQuality([], 'Test Concept', mockModel);

      expect(result.overall).toBe(0);
      expect(result.standalone).toBe(0);
      expect(result.distractors).toBe(0);
      expect(result.explanation).toBe(0);
      expect(result.difficulty).toBe(0);
      expect(result.reasoning).toBe('No phrasings to evaluate');
      expect(result.issues).toContain('Empty phrasing batch');
      expect(mockGenerateObject).not.toHaveBeenCalled();
    });

    it('evaluates phrasings successfully', async () => {
      const mockScoringResult = {
        overall: 4.5,
        standalone: 5,
        distractors: 4,
        explanation: 4,
        difficulty: 5,
        reasoning: 'High quality questions with good distractors',
        issues: [],
      };

      mockGenerateObject.mockResolvedValue({ object: mockScoringResult } as any);

      const phrasings = [
        createMockPhrasing(),
        createMockPhrasing({ question: 'Another question?' }),
      ];
      const result = await evaluatePhrasingQuality(phrasings, 'Geography', mockModel);

      expect(result).toEqual(mockScoringResult);
      expect(mockGenerateObject).toHaveBeenCalledWith(
        expect.objectContaining({
          model: mockModel,
          prompt: expect.stringContaining('Geography'),
        })
      );
    });

    it('includes phrasing details in prompt', async () => {
      mockGenerateObject.mockResolvedValue({
        object: {
          overall: 4,
          standalone: 4,
          distractors: 4,
          explanation: 4,
          difficulty: 4,
          reasoning: 'Good',
          issues: [],
        },
      } as any);

      const phrasing = createMockPhrasing({ question: 'Unique test question?' });
      await evaluatePhrasingQuality([phrasing], 'Test Concept', mockModel);

      const callArgs = mockGenerateObject.mock.calls[0][0];
      expect(callArgs.prompt).toContain('Unique test question?');
      expect(callArgs.prompt).toContain('Test Concept');
    });

    it('returns sentinel values on evaluation error', async () => {
      mockGenerateObject.mockRejectedValue(new Error('LLM API error'));

      const result = await evaluatePhrasingQuality([createMockPhrasing()], 'Test', mockModel);

      expect(result.overall).toBe(-1);
      expect(result.standalone).toBe(-1);
      expect(result.distractors).toBe(-1);
      expect(result.explanation).toBe(-1);
      expect(result.difficulty).toBe(-1);
      expect(result.reasoning).toContain('Evaluation failed');
      expect(result.reasoning).toContain('LLM API error');
      expect(result.issues).toContain('Scoring evaluation failed - scores are invalid');
    });

    it('handles non-Error exceptions', async () => {
      mockGenerateObject.mockRejectedValue('String error');

      const result = await evaluatePhrasingQuality([createMockPhrasing()], 'Test', mockModel);

      expect(result.overall).toBe(-1);
      expect(result.reasoning).toContain('String error');
    });

    it('uses reduced thinking budget for evaluation', async () => {
      mockGenerateObject.mockResolvedValue({
        object: {
          overall: 3,
          standalone: 3,
          distractors: 3,
          explanation: 3,
          difficulty: 3,
          reasoning: 'Average',
          issues: [],
        },
      } as any);

      await evaluatePhrasingQuality([createMockPhrasing()], 'Test', mockModel);

      expect(mockGenerateObject).toHaveBeenCalledWith(
        expect.objectContaining({
          providerOptions: {
            openrouter: {
              reasoning: {
                max_tokens: 1024,
              },
            },
          },
        })
      );
    });
  });
});
