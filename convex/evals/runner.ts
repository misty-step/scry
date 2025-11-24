import { generateObject } from 'ai';
import { action } from '../_generated/server';
import { prepareConceptIdeas } from '../aiGeneration';
import { initializeProvider } from '../lib/aiProviders';
import { conceptIdeasSchema } from '../lib/generationContracts';
import { buildConceptSynthesisPrompt } from '../lib/promptTemplates';
import { generateObjectWithResponsesApi } from '../lib/responsesApi';
import { EVAL_CASES } from './cases';

export const run = action({
  args: {},
  handler: async (_ctx) => {
    const results = [];

    // Configuration defaults (matching production mostly, but using env vars if present)
    const providerName = process.env.AI_PROVIDER || 'openai';
    const modelName = process.env.AI_MODEL || 'gpt-5.1'; // Using 5.1 as requested
    const reasoningEffort = 'high';
    const verbosity = 'medium';

    // Initialize provider once
    const providerClient = await initializeProvider(providerName, modelName, {
      logContext: { source: 'evals' },
    });

    const { provider, model, openaiClient } = providerClient;

    // eslint-disable-next-line no-console
    console.log(`Starting Eval Run with ${providerName} / ${modelName}...`);

    for (const testCase of EVAL_CASES) {
      // eslint-disable-next-line no-console
      console.log(`Running case: "${testCase.prompt}"...`);
      const startTime = Date.now();

      try {
        const prompt = buildConceptSynthesisPrompt(testCase.prompt);
        let object;

        if (provider === 'openai' && openaiClient) {
          const response = await generateObjectWithResponsesApi({
            client: openaiClient,
            model: modelName,
            input: prompt,
            schema: conceptIdeasSchema,
            schemaName: 'concepts',
            verbosity: verbosity as 'low' | 'medium' | 'high',
            reasoningEffort: reasoningEffort as 'minimal' | 'low' | 'medium' | 'high',
          });
          object = response.object;
        } else if (provider === 'google' && model) {
          const response = await generateObject({
            model,
            schema: conceptIdeasSchema,
            prompt: prompt,
          });
          object = response.object;
        } else {
          throw new Error('Provider not initialized correctly');
        }

        const prepared = prepareConceptIdeas(
          object.concepts,
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
      configuration: { provider: providerName, model: modelName },
      summary: {
        total: results.length,
        passed: results.filter((r) => r.passed).length,
        failed: results.filter((r) => !r.passed).length,
      },
      results,
    };
  },
});
