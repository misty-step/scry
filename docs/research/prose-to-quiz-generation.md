# Grounded quiz and reusable study-material generation

Research reviewed: **2026-09-06**. This record separates provider guarantees,
application policy, and hypotheses that still need paired field evidence. No
live model result or current price is implied by a code change.

## Primary-source findings

- [OpenRouter structured outputs](https://openrouter.ai/docs/guides/features/structured-outputs)
  documents `response_format.type = json_schema`, `json_schema.strict = true`,
  and `provider.require_parameters = true`. Support is **per endpoint**, not
  merely per model. The current guide explicitly warns that enforcement varies
  by provider: strict output is not a substitute for local parsing and validation.
- [OpenRouter provider routing](https://openrouter.ai/docs/guides/routing/provider-selection)
  documents provider fallbacks and parameter filtering. Scry requires supported
  parameters and disables provider fallbacks for this bounded request contract.
  It does not silently switch models. The application may make one transient
  transport retry; an unreadable, refused, or truncated completion is not a
  reason to run an unbounded retry/critique loop.
- [Gemini structured output](https://ai.google.dev/gemini-api/docs/structured-output)
  supports a subset of JSON Schema, including objects, required properties,
  enums, arrays, `minItems`/`maxItems`, and nullable types. Large/deep schemas may
  be rejected. Its guidance explicitly requires application validation of
  semantically incorrect values even when JSON syntax is correct. Scry therefore
  uses shallow explicit schemas and validates field bounds, retrieval shapes,
  quotation attribution, and answer support separately. It does not depend on
  unsupported string-length schema keywords to enforce application limits.
- [Anthropic long-context guidance](https://platform.claude.com/docs/en/build-with-claude/prompt-engineering/claude-prompting-best-practices#long-context-prompting)
  recommends clearly separated document content/metadata and selecting relevant
  quotes before answering. Scry adopts evidence-first grounding and an explicit
  untrusted-data boundary, not a claim that delimiters alone prevent injection.
  Source text is serialized as JSON data in a separate user message; stable
  task instructions are never interpolated with document text, titles, rejected
  outputs, cached notes, or learner answers.
- [OpenAI evaluation best practices](https://developers.openai.com/api/docs/guides/evaluation-best-practices)
  recommends task-specific realistic and adversarial datasets, paired comparisons,
  explicit rubrics, and calibration against human judgment. We keep the existing
  repository-owned eval harness rather than adding a vendor eval platform.
  Deterministic checks are necessary guards, not a factual or pedagogical judge.

These sources support the **mechanisms**, not a numerical claim about Scry's
accuracy, latency, learning benefit, or a particular model's superiority. Earlier
unverified fabrication-rate and pricing claims are not retained as evidence.

## One bounded pipeline, multiple content products

`memory-engine-openrouter::content` is the portable contract: permission checks,
request builders, stable system instructions, JSON schemas, completion parsing,
and typed response gates. It performs no IO and reads no environment variables.
A Worker Fetch boundary can depend on the crate with `default-features = false`.
The optional `native` adapter uses the same contract for local tools and HTTP
regressions; it is not a separate prompt implementation.

The entry points are `quiz_request`, `repair_request`, `reference_request`, and
`bridge_request`. A prepared `ContentRequest::payload(model)` supplies the exact
OpenRouter envelope. `parse_completion` preserves reported usage, and
`parse_drafts_response`, `parse_reference_response`, and `parse_bridge_response`
perform typed decoding. Quiz candidates must then pass the existing generation
runner's source-coverage, duplicate, answer-support, and retrieval-quality gates
before persistence. Reference and Bridge parsers perform their product-specific
gates directly. There is no multi-agent generation system, vector retrieval,
or new model SDK.

### Explicit resource limits

- Authorized source title/body context: **64 KiB**. Review/repair context also
  has a pre-serialization 64 KiB bound. The final system+user message has a
  **128 KiB** bound. Oversized material is rejected with a split-capture remedy;
  the end of a source is never silently dropped.
- Quiz ceiling: **60 candidates**, not a target. Concept prose favors a few
  valuable atoms. Finite enumerable/verbatim inputs retain deterministic complete
  coverage; a set requiring more than 60 drafts is rejected rather than sampled.
- Quiz repair: **one pass**, addressing at most **four** rejected candidates.
  Reference and Bridge semantic repair budgets are zero: reject an invalid
  product rather than multiply calls invisibly. A learner may explicitly retry.
- OpenRouter output budget: at most **32,000 tokens**; quiz requests use
  `450 × max_drafts + 500` (60 cards means 27,500), references 4,000, Bridge 8,000.
  These are ceilings, not quality targets or token-count estimates.
- Transport: at most **two transmissions per completion**, for transient
  transport/5xx/429 failures only. Quiz+repair can therefore issue at most four
  transmissions per source. Native attempts cap at 60 seconds; proxy reads and
  writes share an absolute deadline. Every transport must bound response reads
  to **1 MiB**, not allocate an unlimited body and check afterward. A Worker
  transport must implement the same timeout/retry/body-read contract using its
  own Fetch facilities; the portable module does not perform transport retries.
- Refusals, non-stop finish reasons, malformed JSON, unknown typed payload
  fields, and excess draft counts fail closed. There is no prose/fence slicing
  to recover a seemingly valid sub-object from an invalid completion.

Usage is retained for rejected paid responses and failed repair, not only accepted
cards. Token and latency sums saturate rather than overflow. A missing cost stays
unknown when aggregated with a known cost. After a lost/transient first response,
the final response's price is not represented as the whole operation's price.
Zero reported tokens can mean unreported, not free; receipts say so. No model-less
ordinary-prose fallback is allowed to masquerade as successful model generation.
The deterministic structured-block parser and explicitly labeled CI fake remain
separate purposes.

## Truthful provenance

A quiz with an `evidence_quote` claims source support. The quote must be a
substantive normalized token-bounded span in the authorized source; an unrelated
real sentence or a topic title cannot prove an arbitrary answer. A conservative
lexical answer-support floor rejects unrelated answers and unsupported numbers.
It is **not semantic entailment**: attribution, negation, conditions, subtle
contradictions, and faithful paraphrases still need calibrated quality review.
False rejection can be repaired using inspectable source wording.

A quiz with an empty quote is **model-expanded topic knowledge**. The captured
seed remains a source-lineage pointer, explicitly labeled “not evidence”; it is
not a fabricated citation. Ordinary topics remain useful without pretending that
“photosynthesis” contains every generated fact about chloroplasts.

All accepted products retain explicit learner review authority. A trust-gate
acceptance means “passed these mechanical checks,” not “independently fact-checked.”

## Quiz quality

The principled instructions require standalone atomic questions, appropriate
retrieval depth, meaningful concept labels, no answer leakage, and no padding.
Conceptual prose should test mechanisms and distinctions, not only definitions.
Procedures isolate a decision or step, with a worked solution when performance
rather than recognition is required. Exact recitation preserves the supplied
wording and sequence. A fixed set tests every required mapping without adding
members from a model's memory.

Recognition options must be plausible same-category confusions with one correct
answer. Runtime gates reject duplicated options/answers, catch-all choices,
overlapping numeric ranges, keyed-initial clues, and literal answer leakage.
Semantic aliases, subtly overlapping meanings, misleading conditions, and merely
“plausible-looking” distractors are still human/model-rubric eval concerns. If
credible wrong options are unavailable, cued recall is preferable to filler MCQs.

## Reusable concept references and manual Bridge

`SourceAuthorizationContext` retains actual source titles and bodies, not only
permission receipts. Archived sources cannot be authorized; local-only context
cannot cross the external provider boundary. Reference output uses a typed
internal schema: grounding, verified source quotations, explanation, concrete
example, distinctions/common confusions, and a retrieval cue. These are serialized
as plain text into the existing `ReferenceNoteDraft { title, body }` and durable
`ConceptReferenceNote` storage. No new wire/storage shape is required.

The explanation must be substantive, not a restatement of the current question
and answer. Source-informed notes identify exactly which quotations were checked
and distinguish generated explanations/examples from quotations. Model-expanded
notes explicitly say the captured seed is not evidence. Claimed source quotes
must be exact authorized spans; unverified multiword quotations elsewhere in the
note are rejected. Section/length checks do not establish usefulness or truth.

The study layer owns caching: first access creates the durable concept note,
second access reads it without a model call. Bridge requests carrying a cached
note return `reference_note: null` internally and reuse the exact stored body;
they cannot rewrite the reference. No automatic pack rollout is part of this
change.

Manual Bridge produces **2–3 inspectable smaller prerequisites**, informed by
recent wrong/close/revealed attempts. Merely lowering the stage label or copying
a multiword parent answer is rejected. Normally the pack includes stage-0
recognition and stage-1 cued recall; a stage-1 parent instead permits different
stage-0 prerequisites. Parents already at stage 0 use study material rather than
an impossible lower rung. The learner's explicit keep/skip authority is unchanged.

## Evaluation and remaining hypotheses

The corpus includes ordinary technical/math prose, enumerations, verbatim
material, a bare topic seed, role/delimiter injection, qualified observational
claims, and conditional numeric facts. Six corpus cases also probe reusable
references with explanation/example/distinction term expectations, verified
source context or truthful topic expansion, retrieval cues, and forbidden claims.
The Bridge fixture targets an observed missing component, not stage labels alone.

Receipts expose accepted/rejected counts, failure usage, provenance lanes,
coverage excluding distractors, task-specific mechanical oracles, cost uncertainty,
source-clustered intervals, and paired reference comparisons. They include actual
accepted quiz/reference material for blinded human review. An old receipt lacking
reference cases cannot establish a reference improvement. Keep the same corpus,
model, budgets, and transport when comparing prompt/implementation variants;
randomize review order and calibrate the rubric against human keep/revise/reject
labels. See [the eval guide](../evals.md).

The default remains **`google/gemini-3.7-flash`**, per the recorded 2026-08-17
operator decision. Documentation advertising a newer model is not paired Scry
evidence. Change defaults only after a dated comparison improves useful accepted
quiz **and reference/Bridge** quality without hiding failures, cost, or uncertainty.
No live model calls or validation runs were performed as part of this research
and implementation slice; the integration owner records executable proof and any
field receipt separately.
