import type { UIMessage } from '@convex-dev/agent/react';

export interface ToolResult {
  toolName: string;
  data: Record<string, unknown>;
}

export interface ExtractedToolResults {
  latestQuestion: ToolResult | null;
  latestFeedback: {
    data: Record<string, unknown>;
    questionText: string | null;
    token: string;
  } | null;
}

export function extractLatestToolResults(messages: UIMessage[]): ExtractedToolResults {
  let latestQuestionBeforeFeedback: ToolResult | null = null;
  let latestQuestionAfterFeedback: ToolResult | null = null;
  let latestFeedback: {
    data: Record<string, unknown>;
    questionText: string | null;
    token: string;
  } | null = null;
  let currentQuestionText: string | null = null;

  for (const msg of messages) {
    if (msg.role !== 'assistant') continue;
    for (const [partIndex, part] of msg.parts.entries()) {
      if (typeof part.type !== 'string' || !part.type.startsWith('tool-') || !('state' in part)) {
        continue;
      }

      const toolName = part.type.slice(5);
      const typedPart = part as { type: string; state: string; output?: unknown };
      if (typedPart.state !== 'output-available' || typedPart.output == null) continue;

      const output = typedPart.output as Record<string, unknown>;

      if (toolName === 'fetchDueConcept') {
        const questionResult = { toolName, data: output };
        latestQuestionBeforeFeedback = questionResult;
        currentQuestionText = (output.question as string) ?? null;

        if (latestFeedback) {
          latestQuestionAfterFeedback = questionResult;
        }
        continue;
      }

      if (toolName === 'submitAnswer') {
        latestFeedback = {
          data: output,
          questionText: currentQuestionText,
          token: `${msg.key}:${partIndex}`,
        };
        latestQuestionAfterFeedback = null;
      }
    }
  }

  return {
    latestQuestion: latestFeedback ? latestQuestionAfterFeedback : latestQuestionBeforeFeedback,
    latestFeedback,
  };
}
