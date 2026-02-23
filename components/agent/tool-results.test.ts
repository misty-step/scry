import type { UIMessage } from '@convex-dev/agent/react';
import { describe, expect, it } from 'vitest';
import { extractLatestToolResults } from './tool-results';

function assistantMessage(
  parts: Array<{ type: string; state: string; output?: unknown }>
): UIMessage {
  return {
    key: `assistant-${parts.length}`,
    role: 'assistant',
    status: 'done',
    text: '',
    parts,
  } as unknown as UIMessage;
}

function toolPart(toolName: string, output: Record<string, unknown>) {
  return {
    type: `tool-${toolName}`,
    state: 'output-available',
    output,
  };
}

describe('extractLatestToolResults', () => {
  it('returns the first fetched question before any answer', () => {
    const messages = [
      assistantMessage([
        toolPart('fetchDueConcept', { conceptId: 'c1', question: 'What is 2 + 2?' }),
      ]),
    ];

    const result = extractLatestToolResults(messages);

    expect(result.latestFeedback).toBeNull();
    expect(result.latestQuestion?.toolName).toBe('fetchDueConcept');
    expect(result.latestQuestion?.data.question).toBe('What is 2 + 2?');
  });

  it('keeps feedback and next question when both tool outputs exist in one assistant turn', () => {
    const messages = [
      assistantMessage([
        toolPart('fetchDueConcept', { conceptId: 'c1', question: 'What is 2 + 2?' }),
      ]),
      assistantMessage([
        toolPart('submitAnswer', { isCorrect: true, conceptTitle: 'Math basics' }),
        toolPart('fetchDueConcept', { conceptId: 'c2', question: 'What is 3 + 3?' }),
      ]),
    ];

    const result = extractLatestToolResults(messages);

    expect(result.latestFeedback?.data.isCorrect).toBe(true);
    expect(result.latestFeedback?.questionText).toBe('What is 2 + 2?');
    expect(result.latestQuestion?.data.question).toBe('What is 3 + 3?');
  });

  it('shows feedback without stale question while waiting for the next fetch', () => {
    const messages = [
      assistantMessage([
        toolPart('fetchDueConcept', { conceptId: 'c1', question: 'What is 2 + 2?' }),
      ]),
      assistantMessage([
        toolPart('submitAnswer', { isCorrect: false, conceptTitle: 'Math basics' }),
      ]),
    ];

    const result = extractLatestToolResults(messages);

    expect(result.latestFeedback?.questionText).toBe('What is 2 + 2?');
    expect(result.latestQuestion).toBeNull();
  });
});
