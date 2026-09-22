# Jev free-response recall evaluation

Date: 2026-09-22.

## Decision

The `correct` action has narrow support.
It made 25 `correct` decisions on holdout.
All 25 matched the gold label.
It made no false success on the 56 holdout responses that were not gold-correct.
The primary false-success result was 0/28 on partial, contradiction, qualifier reversal, and extra-false-claim responses.

The coverage was low.
The policy accepted 25/44 gold-correct holdout responses.
It returned `incomplete` for two gold-correct responses.
It returned `ungraded` for 17 gold-correct responses.
The policy accepted only 1/7 concise correct synonyms on holdout.
It accepted 7/7 long faithful paraphrases.

The `incomplete` action does not have support for authority.
It selected six holdout responses.
Four were gold-incomplete.
Two were gold-correct.
Both false incomplete results were the canonical and case-only vaccine answers.
A cue would wrongly fence those correct attempts as assisted.

The enabled `incorrect` action has narrow support for listed contradictions.
It made 18 holdout `incorrect` decisions.
All 18 matched gold-incorrect labels.
It caught 7/7 explicit contradictions, 7/7 qualifier reversals, and 4/7 extra false claims.
It caught 0/7 related-but-wrong answers.
The rule must not become a general wrong-answer classifier.

Use Jev in shadow mode for `incomplete`.
Use `correct` only with the frozen gates and an honest ungraded fallback.
The evidence supports enabling `incorrect` only for the D5 high-contradiction plus `different` rule.
It does not support incorrect authority for unlisted mistakes, related facts, or all fluent wrong answers.

## Frozen parameters

The tune-only sweep froze these values:

| Parameter | Value |
|---|---:|
| `T_idea` | 0.80 |
| `T_idea_low` | 0.35 |
| `T_contra_low` | 0.20 |
| `T_contra_high` | 0.90 |
| `T_rel` | 0.85 |
| `T_partial` | 0.60 |
| `T_inj` | 0.20 |
| `incorrect` class | Enabled only for the narrow D5 contradiction rule |

`T_rel` increased from 0.75 to 0.85.
No threshold decreased.
The holdout result did not change the parameters.

## Method

The corpus uses public facts and synthetic learner responses.
It contains no private learner data.

The corpus has 14 concepts and 14 recall questions.
It covers six domains.
The domains are HTTP and web systems, biology, history and geography, programming and databases, cooking and chemistry, and mathematics and statistics.
Twelve questions use semantic grading.
Two deterministic controls use exact grading.
Exact questions have no content rubric.

The split was by concept.
Six concepts were assigned to tune.
Eight concepts were assigned to holdout.
The tune split has 85 responses.
The holdout split has 114 responses.
The semantic policy tables use 71 tune responses and 100 holdout responses.

The gold label was written before the first Jev call.
The labels were authored by the Hermes evaluation agent (claude-fable-5-1 via Anthropic) from the public source facts and the authored rubric, in the style of a human rater.
No human reviewed the labels before the run.
The label does not use model consensus, and Jev never saw the gold.
Each label records the objective app action.
Each semantic label also records every required-idea truth and every contradiction truth.
Each label has a rationale.
The gold does not use model consensus.

The frozen hashes are:

- `corpus.json`: `9fb799e7ccc32bcb0412201dd9b9866d5ff67cbc120db57bfc6a465ddbaa2276`
- `gold.json`: `fb74dfa7af5e07a064720d117af4abb79cbb0da892537e4e47cb34ad18645ef5`

The first live pass covered all 199 responses.
The holdout then ran three more times.
This produced 541 logical responses and 541 HTTP calls.
There were no retries.

Each live request used one D8 battery.
The request state contained only the prompt, expected answer, accepted variants, rubric text, and learner answer.
The D8 question text is copied verbatim in the runner.
The model request was `typesafe/jev-1.13`.
The returned model was `typesafe/jev-1.13-20260917` for all 541 calls.

The runner used an 8 second timeout.
It records the request, raw response, model, provider, request ID, tokens, cost, latency, HTTP status, error, baseline result, gold label, and both policy variants.
It retries HTTP 429 or 529 once.
It records both transmissions when a retry occurs.

The current `learning.Grade` function ran on every response.
The exact controls also went to Jev as diagnostic probes.
Their product decision still came only from `learning.Grade`.

## Response counts

Each standard bucket has one response per question.
Three questions also have a non-English correct response.

| Bucket | Responses |
|---|---:|
| exact | 14 |
| authored variant | 14 |
| case-only | 14 |
| concise correct synonym | 14 |
| faithful complete paraphrase | 14 |
| minimally sufficient | 14 |
| partial | 14 |
| related but wrong | 14 |
| explicit contradiction | 14 |
| qualifier reversal | 14 |
| extra false claim | 14 |
| prompt copied | 14 |
| empty or `idk` | 14 |
| adversarial grader instruction | 14 |
| non-English correct | 3 |
| **Total** | **199** |

## Current grader baseline

This table uses the one full pass.
It shows the raw `learning.Grade` outcomes in legacy exact mode.
The shipped semantic mode no longer emits WRONG for prose recall; it leaves unmatched prose ungraded.
The exact-mode baseline stays in this table because it is the failure the evaluation measures Jev against.
The two exact controls remain in the table.

| Bucket | N | correct | close | wrong | ungraded | Gold correct | False-WRONG |
|---|---:|---:|---:|---:|---:|---:|---:|
| adversarial instruction | 14 | 0 | 0 | 0 | 14 | 0 | 0 |
| authored variant | 14 | 14 | 0 | 0 | 0 | 14 | 0 |
| case-only | 14 | 0 | 12 | 2 | 0 | 12 | 0 |
| concise correct synonym | 14 | 0 | 0 | 14 | 0 | 12 | 12 |
| empty or `idk` | 14 | 0 | 0 | 9 | 5 | 0 | 0 |
| exact | 14 | 14 | 0 | 0 | 0 | 14 | 0 |
| explicit contradiction | 14 | 0 | 0 | 2 | 12 | 0 | 0 |
| extra false claim | 14 | 0 | 0 | 1 | 13 | 0 | 0 |
| faithful complete paraphrase | 14 | 0 | 0 | 0 | 14 | 12 | 0 |
| minimally sufficient | 14 | 2 | 0 | 0 | 12 | 14 | 0 |
| non-English correct | 3 | 0 | 0 | 0 | 3 | 3 | 0 |
| partial | 14 | 0 | 0 | 3 | 11 | 0 | 0 |
| prompt copied | 14 | 0 | 0 | 2 | 12 | 0 | 0 |
| qualifier reversal | 14 | 0 | 0 | 2 | 12 | 0 | 0 |
| related but wrong | 14 | 0 | 0 | 4 | 10 | 0 | 0 |

The baseline false-WRONG rate on semantic concise synonyms was 12/12, or 100.0%.
The baseline accepted every exact answer and authored variant.
It left every long faithful paraphrase ungraded.

## Tune-only threshold sweep

The sweep used only the tune split.
It evaluated 280 parameter combinations.
`T_idea` covered 0.60 through 0.95.
`T_contra_high` covered 0.80 through 0.97.
`T_rel` covered 0.60 through 0.90.
The other four parameters stayed at their D5 initial values.

The selection order was consequence-first.
It first minimized false success in the four primary risk buckets.
It then minimized false failure on synonyms and paraphrases.
It then minimized false failure on all gold-correct responses.
It rejected false `incorrect` decisions.
It then favored more true `incorrect` and true `incomplete` decisions.
A candidate below an initial consequential threshold was not eligible for the frozen recommendation.

These were the leading eligible rows:

| `T_idea` | `T_contra_high` | `T_rel` | Risk false success | Synonym/paraphrase false failure | All-correct false failure | Incorrect TP/FP | Incomplete TP | Abstain |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 0.80 | 0.90 | 0.85 | 0/20 | 4/10 | 10/31 | 12/0 | 5/5 | 45 |
| 0.80 | 0.90 | 0.80 | 0/20 | 4/10 | 10/31 | 12/0 | 5/5 | 45 |
| 0.80 | 0.90 | 0.75 | 0/20 | 4/10 | 10/31 | 12/0 | 5/5 | 45 |
| 0.80 | 0.95 | 0.85 | 0/20 | 4/10 | 10/31 | 9/0 | 5/5 | 45 |
| 0.80 | 0.97 | 0.85 | 0/20 | 4/10 | 10/31 | 8/0 | 5/5 | 45 |
| 0.90 | 0.90 | 0.85 | 0/20 | 4/10 | 11/31 | 12/0 | 4/5 | 47 |

`T_rel=0.90` lost one more gold-correct tune response and two true incorrect decisions.
`T_rel=0.85` was the highest relation threshold with no measured loss against 0.75 or 0.80.
The sweep therefore froze `T_rel=0.85`.

## Jev policy confusion

The following tables contain semantic questions only.
`C` means `correct`.
`I` means `incomplete`.
`W` means `incorrect`.
`U` means `ungraded`.

### Tune, incorrect disabled

| Gold \ action | C | I | W | U |
|---|---:|---:|---:|---:|
| correct | 21 | 0 | 0 | 10 |
| incomplete | 0 | 5 | 0 | 0 |
| incorrect | 0 | 0 | 0 | 20 |
| ungraded, not judgeable | 0 | 0 | 0 | 15 |

### Tune by bucket, incorrect disabled

| Bucket | N | Gold | C | I | W | U | False success | False failure |
|---|---:|---|---:|---:|---:|---:|---:|---:|
| adversarial instruction | 5 | U | 0 | 0 | 0 | 5 | 0 | 0 |
| authored variant | 5 | C | 5 | 0 | 0 | 0 | 0 | 0 |
| case-only | 5 | C | 2 | 0 | 0 | 3 | 0 | 3 |
| concise correct synonym | 5 | C | 1 | 0 | 0 | 4 | 0 | 4 |
| empty or `idk` | 5 | U | 0 | 0 | 0 | 5 | 0 | 0 |
| canonical exact wording | 5 | C | 2 | 0 | 0 | 3 | 0 | 3 |
| explicit contradiction | 5 | W | 0 | 0 | 0 | 5 | 0 | 0 |
| extra false claim | 5 | W | 0 | 0 | 0 | 5 | 0 | 0 |
| faithful complete paraphrase | 5 | C | 5 | 0 | 0 | 0 | 0 | 0 |
| minimally sufficient | 5 | C | 5 | 0 | 0 | 0 | 0 | 0 |
| non-English correct | 1 | C | 1 | 0 | 0 | 0 | 0 | 0 |
| partial | 5 | I | 0 | 5 | 0 | 0 | 0 | 0 |
| prompt copied | 5 | U | 0 | 0 | 0 | 5 | 0 | 0 |
| qualifier reversal | 5 | W | 0 | 0 | 0 | 5 | 0 | 0 |
| related but wrong | 5 | W | 0 | 0 | 0 | 5 | 0 | 0 |

Tune false success in the primary risk buckets was 0/20.
Tune false failure on concise synonyms and long paraphrases was 4/10.
Tune abstention was 45/71, or 63.4%.

### Holdout, incorrect disabled

| Gold \ action | C | I | W | U |
|---|---:|---:|---:|---:|
| correct | 25 | 2 | 0 | 17 |
| incomplete | 0 | 4 | 0 | 3 |
| incorrect | 0 | 0 | 0 | 28 |
| ungraded, not judgeable | 0 | 0 | 0 | 21 |

### Holdout by bucket, incorrect disabled

| Bucket | N | Gold | C | I | W | U | False success | False failure |
|---|---:|---|---:|---:|---:|---:|---:|---:|
| adversarial instruction | 7 | U | 0 | 0 | 0 | 7 | 0 | 0 |
| authored variant | 7 | C | 7 | 0 | 0 | 0 | 0 | 0 |
| case-only | 7 | C | 1 | 1 | 0 | 5 | 0 | 6 |
| concise correct synonym | 7 | C | 1 | 0 | 0 | 6 | 0 | 6 |
| empty or `idk` | 7 | U | 0 | 0 | 0 | 7 | 0 | 0 |
| canonical exact wording | 7 | C | 1 | 1 | 0 | 5 | 0 | 6 |
| explicit contradiction | 7 | W | 0 | 0 | 0 | 7 | 0 | 0 |
| extra false claim | 7 | W | 0 | 0 | 0 | 7 | 0 | 0 |
| faithful complete paraphrase | 7 | C | 7 | 0 | 0 | 0 | 0 | 0 |
| minimally sufficient | 7 | C | 6 | 0 | 0 | 1 | 0 | 1 |
| non-English correct | 2 | C | 2 | 0 | 0 | 0 | 0 | 0 |
| partial | 7 | I | 0 | 4 | 0 | 3 | 0 | 0 |
| prompt copied | 7 | U | 0 | 0 | 0 | 7 | 0 | 0 |
| qualifier reversal | 7 | W | 0 | 0 | 0 | 7 | 0 | 0 |
| related but wrong | 7 | W | 0 | 0 | 0 | 7 | 0 | 0 |

The primary holdout false-success rate was 0/28, or 0.0%.
The four risk buckets each contributed seven responses.
No risk response became `correct`.

The holdout false-failure rate on concise synonyms and long paraphrases was 6/14, or 42.9%.
All six failures were concise synonyms.
The policy accepted 1/7 concise synonyms and 7/7 long paraphrases.

The holdout abstention rate was 69/100, or 69.0%.
This rate excludes exact-mode controls.
`incomplete` is not counted as abstention.

### Incorrect enabled

The enabled rule changed only responses with a high listed contradiction and a high `different` relation.

| Split | Gold incorrect | Predicted incorrect | True incorrect | False incorrect | Gold incorrect left ungraded |
|---|---:|---:|---:|---:|---:|
| tune | 20 | 12 | 12 | 0 | 8 |
| holdout | 28 | 18 | 18 | 0 | 10 |

The holdout results by wrong-answer bucket were:

| Bucket | N | Incorrect | Ungraded |
|---|---:|---:|---:|
| explicit contradiction | 7 | 7 | 0 |
| qualifier reversal | 7 | 7 | 0 |
| extra false claim | 7 | 4 | 3 |
| related but wrong | 7 | 0 | 7 |

This precision supports the narrow contradiction rule.
The sample is too small to support a general `incorrect` action.

## Literalness and valid-answer behavior

The main weakness was literal atom matching on short answers.
The policy accepted only 1/7 canonical short answers on holdout.
It accepted only 1/7 case-only versions of those answers.
It accepted only 1/7 concise synonyms.

The policy accepted all seven long faithful paraphrases.
It accepted six of seven minimally sufficient answers.
The one unstable minimally sufficient answer was about kneading and gluten.

The two false `incomplete` results came from the vaccine question.
The short canonical answer said that antigen exposure builds faster memory responses.
The first rubric idea also stated that vaccination does not require the full disease.
Jev treated this as one missing idea.
The frozen agent-authored label treated the prompt context and canonical wording as sufficient.
This result shows that short canonical answers and detailed rubric atoms need exact alignment before runtime use.

## Non-English behavior

The corpus has three non-English correct answers.
They are in Spanish, French, and German.
The policy returned `correct` for all three in the full pass.
The holdout result was 2/2.

This sample is too small for broad non-English authority.
It contains no non-English partial answer, contradiction, or adversarial answer.

## Adversarial behavior

All 12 semantic adversarial instructions returned `ungraded`.
No adversarial instruction became `correct`, `incomplete`, or `incorrect`.
The two exact-mode adversarial controls stayed on the deterministic path and were ungraded.

All 12 semantic copied prompts returned `ungraded`.
All 12 semantic empty or `idk` answers returned `ungraded`.

These probes did not produce a false success.
They do not cover long indirect prompt injection or mixed correct-plus-instruction attacks.

## Numeric and exact controls

The exact controls were HTTP status `304` and the integer result `21`.
They stayed in exact mode.
They carried no content rubric.

The deterministic product path returned six correct decisions, 15 incorrect decisions, and seven ungraded decisions.
All six accepted responses were exact answers or explicit variants.
No disallowed control response became a product success.

The Jev diagnostic shadow returned six correct decisions and 22 ungraded decisions.
It made no diagnostic false success on a disallowed exact response.
These outputs do not grant Jev authority over numbers, identifiers, symbols, spelling, or required format.

## Stability

The stability comparison used the 100 semantic holdout responses.
Each of the three repeat passes was compared with the first holdout pass.
Each repeat had one action flip.
The combined result was three flips in 300 comparisons.
One unique response caused all three flips.

The response was the minimally sufficient kneading answer.
The first pass returned `ungraded`.
All three repeat passes returned `correct`.
The `equivalent` probability was 0.84 in the first pass.
It was 0.89, 0.87, and 0.87 in the repeats.
The frozen relation threshold was 0.85.

For Noul outputs, 70 response-question series came within 0.10 of an active threshold.
Their repeat range had a minimum of 0.0000 and a maximum of 0.0900.
The observed decision flip came from the Choice probability, not from a Noul crossing.

## Latency, cost, and failures

| Measure | Result |
|---|---:|
| Logical responses | 541 |
| HTTP calls | 541 |
| Input tokens | 568,321 |
| Output tokens | 74,612 |
| Usage cost | $0.023869482 |
| Latency p50 | 317 ms |
| Latency p95 | 555 ms |
| HTTP or parse failures | 0 |
| Timeouts | 0 |
| Retries | 0 |

The measured spend is below the $0.05 cap.
The value is the sum of `usage.cost` in `raw.jsonl`.

## Authority verdicts

### `correct`

**Verdict: supported for narrow high-precision acceptance.**

The holdout had 25/25 precision for emitted `correct` actions.
The primary false-success result was 0/28.
The policy also had 19/44 false failures across all gold-correct holdout answers.
The unsupported coverage is short canonical wording, case-only wording, and concise synonyms.
The result does not support broad semantic coverage.

### `incomplete` plus cue

**Verdict: unsupported.**

The holdout had four true incomplete decisions and two false incomplete decisions.
The emitted action precision was 4/6.
The recall on gold-incomplete responses was 4/7.
The unsupported coverage includes short correct answers whose wording does not repeat every rubric qualification.
Do not show a cue or set the assistance fence from this action yet.

### `incorrect`

**Verdict: supported only for the D5 listed-contradiction rule.**

The holdout had 18/18 precision for emitted `incorrect` actions.
Coverage was 18/21 across explicit contradiction, qualifier reversal, and extra-false-claim buckets.
Coverage was 0/7 for related-but-wrong answers.
The unsupported coverage is unlisted misconceptions, merely related answers, three extra-false-claim cases, non-English wrong answers, and production learner text.
Keep every other wrong-looking answer ungraded.

## Limits

This is a small public and synthetic corpus.
The semantic holdout has seven questions and 100 responses.
The result does not estimate population-level error rates.

The corpus has one question per concept.
It does not test repeated cards from the same source.
It does not test private learner language.
It has only three non-English correct answers.

Some canonical answers are compressed summaries of more detailed rubric ideas.
Jev often read the idea text literally.
This is both a model limit and an authoring risk.
Question, expected answer, and rubric alignment need a publication check.

The stability pass repeats the same state.
It measures model variation.
It does not add new semantic coverage.

The exact-control Jev calls are diagnostic only.
Scry must continue to keep deterministic questions exact.

## Shipped policy verification

`go run ./scripts/evals/jev-recall verify` replays every recorded response through the shipped `learning.GradeSemantic` with `learning.SemanticV1Params()`.
It sends nothing.
It refuses to run if the shipped thresholds differ from the frozen table above.

Result on the committed `raw.jsonl` (471 recorded semantic responses, one full pass plus three holdout passes):

- 0 class mismatches between the shipped policy and the runner column.
- The shipped policy applied only `correct`.
- Every `incomplete` and `incorrect` class was recorded as shadow (`applied=false`) and never reached the learner.

| Holdout shipped class | N | Applied | Shadow | Matches gold | Gold-correct in class |
|---|---:|---:|---:|---:|---:|
| correct | 103 | 103 | 0 | 103 | 103 |
| incomplete | 24 | 0 | 24 | 16 | 8 |
| incorrect | 71 | 0 | 71 | 71 | 0 |
| ungraded | 202 | 0 | 0 | 0 | 65 |

The holdout counts cover four passes of the holdout split.
The 103 applied `correct` decisions had 0 false successes.
The 8 gold-correct responses in the `incomplete` shadow class are the reason `incomplete` stays in shadow.
The 65 gold-correct responses left ungraded are the coverage cost of the frozen gates.

Hybrid path: the exact and variant buckets never reach Jev in the product.
They resolve locally before any assessment is staged.
The report's `correct` support is therefore only for the prose buckets that the local grader leaves ungraded.

