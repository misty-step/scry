import { generateObject } from 'ai';
import { action } from '../_generated/server';
import { prepareConceptIdeas } from '../aiGeneration';
import { initializeGoogleProvider } from '../lib/aiProviders';
import { conceptIdeasSchema } from '../lib/generationContracts';
import { buildConceptSynthesisPrompt } from '../lib/promptTemplates';
import { EVAL_CASES } from './cases';

export const run = action({
  args: {},
  handler: async (_ctx) => {
    const results = [];

    // Configuration - always use Google Gemini 3 Pro
    const modelName = process.env.AI_MODEL || 'gemini-3-pro-preview';

    // Initialize provider once
    const { model } = initializeGoogleProvider(modelName, {
      logContext: { source: 'evals' },
    });

    // eslint-disable-next-line no-console
    console.log(`Starting Eval Run with Google / ${modelName}...`);

    for (const testCase of EVAL_CASES) {
      // eslint-disable-next-line no-console
      console.log(`Running case: "${testCase.prompt}"...`);
      const startTime = Date.now();

      try {
        const prompt = buildConceptSynthesisPrompt(testCase.prompt);

        const response = await generateObject({
          model,
          schema: conceptIdeasSchema,
          prompt,
          providerOptions: {
            google: {
              thinkingConfig: {
                thinkingBudget: 8192,
                includeThoughts: true,
              },
            },
          },
        });

        const prepared = prepareConceptIdeas(
          response.object.concepts,
          undefined,
          undefined,
          testCase.prompt
        );
        const duration = Date.now() - startTime;
        const passed = prepared.concepts.length >= testCase.expectedMin;

        results.push({
          prompt: testCase.prompt,
          expectedMin: testCase.expectedMin,
          actualCount: prepared.concepts.length,
          passed,
          durationMs: duration,
          concepts: prepared.concepts.map((c) => c.title), // Just titles for brevity in summary
          fullConcepts: prepared.concepts, // Keep full details available
        });
      } catch (error) {
        console.error(`Failed case "${testCase.prompt}":`, error);
        results.push({
          prompt: testCase.prompt,
          expectedMin: testCase.expectedMin,
          error: error instanceof Error ? error.message : String(error),
          passed: false,
        });
      }
    }

    return {
      timestamp: new Date().toISOString(),
      configuration: { provider: 'google', model: modelName },
      summary: {
        total: results.length,
        passed: results.filter((r) => r.passed).length,
        failed: results.filter((r) => !r.passed).length,
      },
      results,
    };
  },
});
