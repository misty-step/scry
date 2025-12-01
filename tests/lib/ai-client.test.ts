import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { generateObject, generateText } from 'ai';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { generateQuizWithAI } from '@/lib/ai-client';
import { aiLogger, loggers } from '@/lib/logger';

vi.mock('@ai-sdk/google', () => ({
  createGoogleGenerativeAI: vi.fn(),
}));

vi.mock('ai', () => ({
  generateText: vi.fn(),
  generateObject: vi.fn(),
}));

vi.mock('@/lib/logger', () => {
  const time = vi.fn().mockReturnValue({ end: vi.fn().mockReturnValue(123) });
  return {
    aiLogger: {
      info: vi.fn(),
      warn: vi.fn(),
      error: vi.fn(),
    },
    loggers: {
      time,
      error: vi.fn(),
    },
  };
});

const mockedCreateGoogleGenerativeAI = vi.mocked(createGoogleGenerativeAI);
const mockedGenerateText = vi.mocked(generateText);
const mockedGenerateObject = vi.mocked(generateObject);
const mockedLoggers = vi.mocked(loggers);
const mockedAiLogger = vi.mocked(aiLogger);

describe('ai-client.generateQuizWithAI', () => {
  const originalEnv = process.env.GOOGLE_AI_API_KEY;

  beforeEach(() => {
    vi.useFakeTimers().setSystemTime(new Date('2025-01-16T12:00:00Z'));
    mockedCreateGoogleGenerativeAI.mockReturnValue({
      languageModel: vi.fn(),
      chat: vi.fn(),
      image: vi.fn(),
      generativeAI: vi.fn(),
    } as unknown as ReturnType<typeof createGoogleGenerativeAI>);
    mockedGenerateText.mockReset();
    mockedGenerateObject.mockReset();
    mockedAiLogger.info.mockClear();
    mockedAiLogger.warn.mockClear();
    mockedAiLogger.error.mockClear();
    mockedLoggers.time.mockClear();
    mockedLoggers.error.mockClear?.();
    process.env.GOOGLE_AI_API_KEY = 'test-key';
  });

  afterEach(() => {
    vi.useRealTimers();
    process.env.GOOGLE_AI_API_KEY = originalEnv;
  });

  it('throws with diagnostics when API key is missing', async () => {
    process.env.GOOGLE_AI_API_KEY = '';

    await expect(generateQuizWithAI('photosynthesis')).rejects.toMatchObject({
      name: 'generation-error',
      message: expect.stringContaining('GOOGLE_AI_API_KEY'),
      apiKeyDiagnostics: null,
    });

    expect(mockedAiLogger.error).toHaveBeenCalled();
  });

  it('runs two-step happy path with clarified intent', async () => {
    mockedGenerateText.mockResolvedValue({ text: 'clarified intent' } as any);
    mockedGenerateObject.mockResolvedValue({
      object: {
        questions: Array.from({ length: 16 }).map((_, idx) => ({
          question: `Q${idx}`,
          type: 'multiple-choice',
          options: ['a', 'b'],
          correctAnswer: 'a',
          explanation: 'because',
        })),
      },
    } as any);

    const result = await generateQuizWithAI('linear algebra');

    expect(result).toHaveLength(16);
    expect(result[0]).toMatchObject({
      question: 'Q0',
      type: 'multiple-choice',
      options: ['a', 'b'],
      correctAnswer: 'a',
      explanation: 'because',
    });
    expect(mockedGenerateText).toHaveBeenCalledWith({
      model: 'gemini-2.5-flash',
      prompt: expect.stringContaining('Learner input'),
    });
    expect(mockedGenerateObject).toHaveBeenCalledWith({
      model: 'gemini-2.5-flash',
      schema: expect.anything(),
      prompt: expect.stringContaining('The analysis identified'),
    });
    expect(mockedAiLogger.warn).not.toHaveBeenCalled(); // count above MIN_EXPECTED_QUESTION_COUNT
  });

  it('falls back to direct generation when intent clarification fails', async () => {
    mockedGenerateText.mockRejectedValue(new Error('upstream boom'));
    mockedGenerateObject.mockResolvedValue({
      object: {
        questions: [
          {
            question: 'Fallback Q',
            type: 'true-false',
            options: ['True', 'False'],
            correctAnswer: 'True',
            explanation: 'fallback',
          },
        ],
      },
    } as any);

    const result = await generateQuizWithAI('mitochondria');

    expect(result).toHaveLength(1);
    expect(result[0].question).toBe('Fallback Q');
    expect(mockedGenerateObject).toHaveBeenCalledWith({
      model: 'gemini-2.5-flash',
      schema: expect.anything(),
      prompt: expect.stringContaining('TOPIC'),
    });
    expect(mockedAiLogger.warn).toHaveBeenCalledWith(
      expect.objectContaining({
        event: 'ai.intent-clarification.failure',
        fallback: 'direct-generation',
      }),
      expect.any(String)
    );
  });
});
