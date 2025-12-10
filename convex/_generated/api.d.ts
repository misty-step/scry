/* eslint-disable */
/**
 * Generated `api` utility.
 *
 * THIS CODE IS AUTOMATICALLY GENERATED.
 *
 * To regenerate, run `npx convex dev`.
 * @module
 */

import type * as aiGeneration from "../aiGeneration.js";
import type * as clerk from "../clerk.js";
import type * as concepts from "../concepts.js";
import type * as cron from "../cron.js";
import type * as deployments from "../deployments.js";
import type * as embeddings from "../embeddings.js";
import type * as evals_cases from "../evals/cases.js";
import type * as evals_runner from "../evals/runner.js";
import type * as fsrs from "../fsrs.js";
import type * as fsrs_conceptScheduler from "../fsrs/conceptScheduler.js";
import type * as fsrs_engine from "../fsrs/engine.js";
import type * as fsrs_selectionPolicy from "../fsrs/selectionPolicy.js";
import type * as generationJobs from "../generationJobs.js";
import type * as health from "../health.js";
import type * as http from "../http.js";
import type * as iqc from "../iqc.js";
import type * as lab from "../lab.js";
import type * as lib_aiProviders from "../lib/aiProviders.js";
import type * as lib_analytics from "../lib/analytics.js";
import type * as lib_chunkArray from "../lib/chunkArray.js";
import type * as lib_conceptConstants from "../lib/conceptConstants.js";
import type * as lib_conceptFsrsHelpers from "../lib/conceptFsrsHelpers.js";
import type * as lib_conceptHelpers from "../lib/conceptHelpers.js";
import type * as lib_constants from "../lib/constants.js";
import type * as lib_envDiagnostics from "../lib/envDiagnostics.js";
import type * as lib_fsrsReplay from "../lib/fsrsReplay.js";
import type * as lib_generationContracts from "../lib/generationContracts.js";
import type * as lib_interactionContext from "../lib/interactionContext.js";
import type * as lib_langfuse from "../lib/langfuse.js";
import type * as lib_logger from "../lib/logger.js";
import type * as lib_productionConfig from "../lib/productionConfig.js";
import type * as lib_promptTemplates from "../lib/promptTemplates.js";
import type * as lib_prompts from "../lib/prompts.js";
import type * as lib_scoring from "../lib/scoring.js";
import type * as lib_userStatsHelpers from "../lib/userStatsHelpers.js";
import type * as phrasings from "../phrasings.js";
import type * as rateLimit from "../rateLimit.js";
import type * as schemaVersion from "../schemaVersion.js";
import type * as spacedRepetition from "../spacedRepetition.js";
import type * as system from "../system.js";
import type * as types from "../types.js";

import type {
  ApiFromModules,
  FilterApi,
  FunctionReference,
} from "convex/server";

declare const fullApi: ApiFromModules<{
  aiGeneration: typeof aiGeneration;
  clerk: typeof clerk;
  concepts: typeof concepts;
  cron: typeof cron;
  deployments: typeof deployments;
  embeddings: typeof embeddings;
  "evals/cases": typeof evals_cases;
  "evals/runner": typeof evals_runner;
  fsrs: typeof fsrs;
  "fsrs/conceptScheduler": typeof fsrs_conceptScheduler;
  "fsrs/engine": typeof fsrs_engine;
  "fsrs/selectionPolicy": typeof fsrs_selectionPolicy;
  generationJobs: typeof generationJobs;
  health: typeof health;
  http: typeof http;
  iqc: typeof iqc;
  lab: typeof lab;
  "lib/aiProviders": typeof lib_aiProviders;
  "lib/analytics": typeof lib_analytics;
  "lib/chunkArray": typeof lib_chunkArray;
  "lib/conceptConstants": typeof lib_conceptConstants;
  "lib/conceptFsrsHelpers": typeof lib_conceptFsrsHelpers;
  "lib/conceptHelpers": typeof lib_conceptHelpers;
  "lib/constants": typeof lib_constants;
  "lib/envDiagnostics": typeof lib_envDiagnostics;
  "lib/fsrsReplay": typeof lib_fsrsReplay;
  "lib/generationContracts": typeof lib_generationContracts;
  "lib/interactionContext": typeof lib_interactionContext;
  "lib/langfuse": typeof lib_langfuse;
  "lib/logger": typeof lib_logger;
  "lib/productionConfig": typeof lib_productionConfig;
  "lib/promptTemplates": typeof lib_promptTemplates;
  "lib/prompts": typeof lib_prompts;
  "lib/scoring": typeof lib_scoring;
  "lib/userStatsHelpers": typeof lib_userStatsHelpers;
  phrasings: typeof phrasings;
  rateLimit: typeof rateLimit;
  schemaVersion: typeof schemaVersion;
  spacedRepetition: typeof spacedRepetition;
  system: typeof system;
  types: typeof types;
}>;

/**
 * A utility for referencing Convex functions in your app's public API.
 *
 * Usage:
 * ```js
 * const myFunctionReference = api.myModule.myFunction;
 * ```
 */
export declare const api: FilterApi<
  typeof fullApi,
  FunctionReference<any, "public">
>;

/**
 * A utility for referencing Convex functions in your app's internal API.
 *
 * Usage:
 * ```js
 * const myFunctionReference = internal.myModule.myFunction;
 * ```
 */
export declare const internal: FilterApi<
  typeof fullApi,
  FunctionReference<any, "internal">
>;

export declare const components: {};
