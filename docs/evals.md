# Evals And Benchmarks


Scry's Rust engine uses behavior tests as the first eval layer. The goal is to
catch learning-semantic drift before dogfood clients and experimental AI
features build on top of changed behavior.

## Regression Corpus

Run:

```sh
cargo test -p memory-engine
cargo test -p memory-engine-core
```

The Rust facade and core tests replay stable fixture data through live API
surfaces:

- grading fixtures through `Grader`
- scheduler fixtures through `next`
- progression fixtures through eligibility filters
- queue fixtures through `pickNextQueueCandidate`

Add cases when a behavior matters across clients, not for one-off implementation
details. Good eval names should describe the learning behavior, such as
`near-miss-is-close`, `prerequisite-unlocks-next-stage`, or
`anti-clump-yields-to-urgent-review`.

## Benchmarks

Run:

```sh
bun run bench
```

The benchmark script prints receipts with case name, operation count, elapsed
milliseconds, and operations per millisecond. These receipts are intentionally
non-gating: local machines and CI containers vary too much for stable thresholds
until there is more history.

Current benchmark cases cover:

- deterministic short-answer grading
- FSRS schedule transitions
- FSRS synthetic histories for steady-success versus repaired-lapse behavior
- queue selection over 1,000 candidates
- queue interleaving anti-clumping over same-domain alternatives
- service command composition for grade/apply-review plus next-queue

The learning-science rationale for the FSRS and interleaving benchmark receipts
lives in `docs/science/README.md`. Keep that file synchronized when a benchmark
case exists primarily to protect a learning-science claim rather than a raw
runtime surface.

Use benchmark output to compare branches manually. If a future regression is
large and repeatable, shape a ticket with an explicit budget and enough history
to avoid brittle thresholds.

## Generation, reference, and Bridge evals

The repository-owned corpus in
`crates/memory-engine-bench/corpus/generation/` exercises the production quiz
runner, including the trust gate, source coverage policy, duplicate suppression,
and one bounded repair pass. Annotated reference cases also exercise actual
authorized source context and the reusable study-note provider. The manual Bridge
fixture tests smaller prerequisites for a concrete recent miss.

The default is an explicitly labeled deterministic fake, with no network:

```sh
cargo run -p memory-engine-bench -- generation
```

The fake is an offline plumbing/contract fixture, **not a quality baseline**. Its
reference explanations are intentionally not mistaken for useful model output.
Do not interpret a zero-dollar/unreported-usage fake run as a model quality or
production cost result. CI does not call live models.

### Paired field comparisons

Keep the model, corpus revision, draft ceiling, transport, and judging rubric
fixed when comparing prompt/implementation changes. Repeat paired runs to expose
sampling variance. Existing receipts can be supplied with `--baseline`; model
judge keep rates and reference mechanical pass rates are paired by source ID.
Unmatched sources and old receipts without reference rows do not establish a
reference improvement.

A credential-safe explicit field invocation through the existing Mint path is:

```sh
# These variables are private runtime inputs, not values to commit or print.
OPENROUTER_BASE_URL="${MINT_BASE_URL}/proxy/https/openrouter.ai/api/v1" \
OPENROUTER_PROXY_TOKEN="${OPENROUTER_MINT_ALIAS:?private runtime alias required}" \
cargo run -p memory-engine-bench -- generation --model google/gemini-3.7-flash \
  --prompt principled --baseline docs/evals/generation-baseline.md \
  --out docs/evals/generation-candidate-<date>.md
```

`--prompt minimal` is the comparison variant; runtime remains principled.
`--max-drafts <n>` changes the requested ceiling, clamped to 1–60 by the provider.
The default model stays `google/gemini-3.7-flash` unless dated paired Scry evidence
supports a change. A newer vendor model announcement is not such evidence.
Historical receipts remain historical, including
[`generation-061-live-mint-2026-07-21.md`](evals/generation-061-live-mint-2026-07-21.md)
and [`generation-field-2026-06-11.md`](evals/generation-field-2026-06-11.md).

### What the receipt measures

- **Acceptance and failures:** accepted persisted drafts versus accepted,
  rejected, and pre-persistence failures. Empty output is not perfect acceptance.
  Provider/repair failures remain visible even when the generation runner
  records them instead of throwing.
- **Truthful provenance:** separate source-supported and model-expanded counts;
  quote verification applies to claimed quotes only. A retained topic seed is
  not reinterpreted as evidence by the bench. Answerability is a lexical proxy,
  not semantic entailment or external fact-checking.
- **Quiz quality guards:** duplicate rate, expected counts, atomicity/leakage
  gates, same-concept variant quality, keyed-initial distractor cohesion,
  standalone questions, and content-fit/intent/coverage oracles. Key-term
  coverage counts correct content, **not distractors**.
- **Adversarial behavior:** source-embedded role/delimiter injection, forbidden
  claims, qualified observational findings, conditional numbers, and expected
  grounding lanes. A real quote about an unrelated subject cannot justify an
  invented answer.
- **Reference usefulness guards:** a substantive explanation, concrete example,
  key distinctions/common confusions, and a retrieval question; task-specific
  section term coverage; exact authorized-source quotations or explicit
  model-expanded attribution; forbidden-claim checks. Six corpus cases currently
  cover technical, mathematical, topic-only, and adversarial reference needs.
  These are necessary mechanical checks, not a human usefulness verdict.
- **Manual Bridge:** 2–3 lower-stage, same-concept, non-duplicate prerequisites
  that target the observed missing component, not merely a relabeled parent.
- **Cost and latency:** reported token/cost usage includes rejected paid
  responses and failed repair. Missing costs remain unknown through aggregation;
  a transient retry makes total cost uncertain if the first response was lost.
  Receipts distinguish reported subtotals from totals and report unobserved
  calls. Zero tokens may mean unreported, not free. Latency includes body reads
  and native retry/backoff; source-clustered intervals expose small-sample
  uncertainty rather than claiming that a few points are a win.

Optional `--judge <model-id>` adds the existing anchored quiz rubric. Use a
different model family where possible; the receipt warns about self-preference.
The judge receives serialized untrusted source/candidate data and does not punish
an appropriate short-answer card merely for lacking MCQ options. Its outputs must
be calibrated against human decisions before they are used to select a default.

Receipts include actual accepted quiz and reference text for **blinded human
review**. Hide model/variant labels and randomize left/right order. Record
keep/revise/reject plus the concrete defect for factual faithfulness and
attribution, atomicity, retrieval depth, plausible mutually exclusive options,
explanatory substance, example usefulness, and accurate distinctions. A mechanical
pass without human calibration is labeled human-unassessed, not “verified.”

### Boundaries and regression coverage

`memory-engine-openrouter::content` is transport-free; Worker Fetch and the
optional native HTTP adapter use the same request/schema/parsing contract.
Compile a Worker consumer with the dependency's `default-features = false`.
Every transport still owns credential injection, bounded reads, the deadline,
and the at-most-one transient retry. The portable module itself does no IO.

The production limits and primary-source rationale are recorded in
[the generation research note](research/prose-to-quiz-generation.md). Offline
regressions cover source authorization/body retention, delimiter injection,
unsupported answers, fabricated quotes, strict missing/unknown fields,
truncation/refusal, oversized input/output, usage on rejection/repair,
answer leakage, overlapping/duplicate MCQs, and cached Bridge references.
Study-layer tests own the durable second-reference-read/no-new-model-call
contract and learner approval lifecycle.

No live field result is claimed by the 2026-09-06 implementation slice. The
integration owner records actual validation and field receipts separately.
