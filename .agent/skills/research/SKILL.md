---
name: research
description: |
  Scry-tailored web research, repo evidence gathering, multi-AI delegation,
  and multi-perspective validation.
  /research [query], /research delegate [task], /research thinktank [topic].
  Triggers: "search for", "look up", "research", "delegate", "get perspectives",
  "web search", "find out", "investigate", "introspect", "session analysis",
  "check readwise", "saved articles", "reading list", "highlights",
  "what are people saying", "X search", "social sentiment", "trending".
argument-hint: "[query] or [web-search|web-deep|web-news|web-docs|delegate|thinktank|introspect|readwise|xai] [args]"
---

# Research

Retrieval-first research for scry: an agentic spaced-repetition app built on
pure FSRS, Convex, Next.js App Router, React, AI SDK/OpenRouter generation,
Langfuse observability, promptfoo evals, Clerk auth, Sentry, Stripe, and
pnpm-only workflows.

## Execution Stance

You are the executive orchestrator.
- Keep query framing, source weighting, and final synthesis on the lead model.
- Delegate retrieval and specialized analysis to focused subagents/tools when
  the question has meaningful uncertainty or design judgment.
- Run independent evidence streams in parallel.
- Treat scry's live package/workflow files as authority; treat README and older
  docs as orientation only because the repo brief records README drift around
  model, framework, and script facts.

## Absorbed Skills

This skill consolidates: `web-search`, `delegate`, `thinktank`, `introspect`.

## Scry Source-Of-Truth Order

For repo facts, resolve conflicts in this order:
1. `package.json`, `.github/workflows/**`, and `lefthook.yml`.
2. `CLAUDE.md`.
3. `docs/**`, after freshness checks against live code and scripts.
4. README or historical context files, only as leads to verify elsewhere.

Before asserting a current framework, model, script, or gate fact, compare it
against `package.json` and relevant workflow/config files. Current package
anchors from `package.json` include:
- Next.js `16.1.6`, React `^19.2.4`, TypeScript `^5.9.3`, Tailwind `^4.1.18`.
- Convex `^1.31.7`, `convex-helpers` `^0.1.111`, `ts-fsrs` `^5.2.3`.
- AI SDK `ai` `^5.0.129`, `@ai-sdk/google` `^2.0.40`,
  `@openrouter/ai-sdk-provider` `^1.2.5`.
- Clerk `@clerk/nextjs` `^6.37.3`, Sentry `@sentry/nextjs` `^10.38.0`,
  Stripe `^20.3.1`, Langfuse `^3.38.6`, promptfoo `^0.121.3`.

Do not freeze these versions into a conclusion without saying when you checked
them; they are research anchors, not permanent facts.

## Required Scry Context Reads

For research that may affect implementation, planning, prompt/eval behavior, or
architecture, read these before synthesis:
- `.spellbook/repo-brief.md` for vision, invariants, hot debts, and the ship
  gate.
- `package.json` for live scripts, package manager, versions, and forbidden
  deployment-adjacent commands.
- `evals/promptfoo.yaml` and `evals/prompts/**` for current LLM eval coverage
  and provider/model assumptions.
- `docs/llm-ops-strategy.md` for Langfuse/promptfoo strategy, but verify stale
  claims against live eval files and package versions.
- `docs/guides/convex-bandwidth.md` for Convex query guardrails: no unbounded
  runtime `.collect()`, index for filters, use `.take()`/pagination, surface
  truncation.

Also inspect relevant shaped packets and artifacts:
- `backlog.d/*.ctx.md` for oracle checklists, source links, and shaped
  implementation context.
- `docs/research/**` if the directory exists; if absent, say so rather than
  inventing a research archive.
- `convex/evals/**`, `evals/**`, and prompt templates for generation/eval work.
- Langfuse report surfaces: `pnpm cost:report`, `pnpm quality:report`,
  `scripts/langfuse-cost-report.ts`, and `scripts/langfuse-quality-report.ts`.
- Source links already embedded in shaped packets before adding new web links.

## Research-To-Build Handoff Gate

Every research packet that recommends implementation must cite this exact repo
brief gate statement:

> The ship gate is GitHub Actions `Quality Checks` `merge-gate`: `pnpm lint`,
> `pnpm typecheck`, `pnpm audit --audit-level=critical`, no focused tests via
> the `.only` grep, and `pnpm test:ci` must pass; local parity is `pnpm lint &&
> pnpm tsc --noEmit && pnpm test:ci`, with `pnpm test:contract` for
> `convex/**`, `pnpm build` for build/config/workflow/dependency surfaces, and
> `pnpm audit --audit-level=critical` for dependency or lockfile changes.

Also carry the operational constraints: `pnpm` only; backend-first Convex flow;
pure FSRS guardrail; no daily limits, comfort-mode shortcuts, or algorithmic
"FSRS but better"; no deploy/build-prod/local deploy commands without explicit
operator approval.

## Routing

### Explicit sub-capability

If first argument matches a keyword, route directly to that reference:

| Keyword | Reference |
|---------|-----------|
| `web-search`, `web-deep`, `web-news`, `web-docs` | `references/web-search.md` |
| `delegate` | `references/delegate.md` |
| `thinktank` | `references/thinktank.md` |
| `introspect` | `references/introspect.md` |
| `readwise` | `references/readwise.md` |
| `xai` | `references/xai-search.md` |
| `exemplars` | `references/exemplars.md` |

### No sub-capability: mandatory parallel fanout

Research means triangulation. A single WebSearch is a lookup, not `/research`.

Launch all applicable streams in parallel:
1. Exa search for primary web/code/academic sources. See
   `references/exa-tools.md`; WebSearch is fallback only if Exa is unavailable.
2. Thinktank from the scry repo root for repo-aware second voices. Use
   `thinktank run research/quick --input "$QUERY" --output /tmp/thinktank-out --json --no-synthesis`
   for quick research, or `thinktank research "$QUERY" --output /tmp/thinktank-out --json`
   for deep work. Add `--paths` for relevant local files such as
   `convex/**`, `evals/**`, `backlog.d/*.ctx.md`, or docs.
3. xAI/Grok for grounded recency checks, contradiction checks, X-native
   discourse, and multimodal web evidence. Use model
   `grok-4.20-beta-latest-non-reasoning`; see `references/xai-search.md`.
4. Codebase search with `rg`/`rg --files` for what scry already does.

If a provider fails or times out, keep its section and mark it `partial` or
`failed` with the observed error. Do not erase the source from the report.

Narrow to a single source only when the user explicitly names one, or when the
task is a narrow version/fact lookup.

## Provider Preferences For Scry

| Query Type | Primary Source |
|------------|----------------|
| Next.js App Router behavior | Official Next.js docs, then compare with `next` `16.1.6` |
| React behavior | Official React docs, then compare with React `^19.2.4` |
| Convex schema/query/action behavior | Official Convex docs and local `convex/**`; apply bandwidth guide |
| AI SDK provider APIs | Official AI SDK docs plus package versions in `package.json` |
| OpenRouter provider/model behavior | Official OpenRouter docs and `evals/promptfoo.yaml` provider config |
| Clerk auth | Official Clerk docs and local Clerk usage |
| Sentry | Official Sentry Next.js docs and local config |
| Stripe | Official Stripe docs and `app/api/stripe/**` |
| OpenAI products | Official OpenAI docs only, and only when the user asks about OpenAI products |
| Prompt/eval design | `evals/promptfoo.yaml`, `evals/prompts/**`, `docs/llm-ops-strategy.md`, Langfuse reports |
| Repo architecture tradeoffs | Thinktank plus codebase search against hot files and shaped packets |
| Current events/model releases/security advisories | xAI/Grok or WebSearch with dated citations |

Prefer primary docs over blog posts for framework/API behavior. Blog posts are
useful only for implementation examples or ecosystem sentiment and must not
override official docs or scry's live package versions.

## Report Format

Every default fanout report must use this structure:

```md
## Exa (neural search)
[Findings with inline URLs. What did Exa specifically surface?]

## xAI / Grok ([web_search | x_search | both])
[Findings with citations from response.citations. Include dates for model,
security, pricing, or API-change claims.]

## Thinktank ([complete | partial | failed])
[Agent findings, disagreements, output directory, and failure mode if any.]

## Codebase
[Relevant local patterns, package versions, scripts, shaped packets, evals,
Langfuse report surfaces, and docs. "None found" is valid.]

## Scry Constraints
[Pure FSRS, backend-first Convex, Convex bandwidth, pnpm-only, forbidden
deploy/build commands, stale README warning if relevant.]

## Synthesis
[Consensus, contradictions, recommendation, and source-backed handoff gate.]
```

If the report will feed `/shape`, `/implement`, `/build`, or backlog grooming,
append a shaped packet section:

```md
## Shaped Packet Addendum
- Backlog/context artifacts read: [e.g. backlog.d/003-...ctx.md]
- Existing source links reused: [links from ctx/docs before new links]
- Eval/Langfuse impact: [promptfoo cases, Langfuse quality/cost reports]
- Required verification: [local parity plus additive checks]
- Open risks: [dated, concrete, owner-ready]
```

## Use When

- Before implementing any scry system >200 LOC or choosing a reference
  architecture.
- Before changing Convex schema, queries, mutations, actions, AI generation,
  IQC, FSRS scheduling, subscriptions, or Stripe/Clerk/Sentry integrations.
- Before changing prompt templates, evals, provider/model choices, Langfuse
  instrumentation, or cost/quality reporting.
- When package/API facts may be stale, especially Next, React, Convex, AI SDK,
  OpenRouter, Clerk, Sentry, Stripe, model names, pricing, deprecations, or
  security advisories.
- During `/groom`, `/shape`, `/build`, and `/implement` when source-backed
  context or external reference architecture is needed.

## Scry Anti-Patterns

- Trusting README model/framework/script facts over `package.json` and
  workflows.
- Reporting generic Next/React/Convex advice without checking scry's installed
  versions.
- Recommending unbounded Convex `.collect()` in runtime paths.
- Recommending FSRS shortcuts, daily caps, comfort modes, or algorithm changes.
- Omitting `backlog.d/*.ctx.md`, eval, Langfuse, or shaped-packet source links
  when researching prompt/generation/IQC work.
- Treating `docs/llm-ops-strategy.md` as current when `evals/promptfoo.yaml`
  has newer provider/test coverage.
- Returning web-only research for a repo-local change without a Codebase
  section.
- Recommending `pnpm build:local`, `pnpm build:prod`, `pnpm convex:deploy`, or
  production migration/deploy scripts as routine verification.
