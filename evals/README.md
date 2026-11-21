# Scry micro-eval bench (`evals/`)

A small, reproducible, live benchmark of the four places Scry calls a model:
**grading**, the **"Why?" explainer**, **drafting**, and **speech**. It runs
Scry's own production adapters against candidate models on OpenRouter, with
one key (`OPENROUTER_API_KEY`), and records exact `usage.cost` and latency for
every call.

Nothing here changes Scry's behaviour: the service never imports `evals/`, and
no production default in `internal/ai` or `internal/speech` is touched. Only an
explicit `make eval` / `go run ./evals/cmd/scry-eval` sends data to a provider.

## Run it

```sh
make eval                                         # all 4 surfaces, cap $1.35, -> evals/results/latest
make eval EVAL_ARGS="--surface grading --limit 3" # a cheap subset (3 cases per model)
go run ./evals/cmd/scry-eval --surface explainer,speech --limit 2 --max-spend 0.10 --out /tmp/eval
go run ./evals/cmd/scry-eval --surface drafting --models drafting=google/gemini-4-hydrogen --limit 1
```

| Flag | Meaning |
| - | - |
| `--surface` | `all` (grading, explainer, speech, drafting, in that order: cheapest first) or a comma list |
| `--out` | output directory (default `evals/results/<UTC timestamp>`; `evals/results/` is gitignored) |
| `--max-spend` | hard cap in USD for this run (default 1.35), enforced before every call |
| `--limit n` | first *n* cases (clips) per surface; drafting cases are interleaved note/topic/book first, so `--limit 3` covers all three kinds |
| `--models key=a,b` | override one model list (keys: `grading`, `explainer`, `drafting`, `drafting-optional`, `speech-chat`, `speech-stt`, `speech-stt-probe`, `speech-audio-grade`); an empty list (`key=`) skips that list |
| `--evidence dir` | also write flat copies with collector-friendly names (`*.jsonl` → `*.jsonl.txt`) |
| `--seed` | blind-label shuffle seed (default `scry-eval-v1`) |

Outputs (in `--out`): `eval-summary.json` (every metric, total spend, git SHA,
catalog verification, notes), `grading-raw.jsonl`, `explainer-raw.jsonl`,
`drafting-raw.jsonl`, `speech-raw.jsonl` (one record per call), and
`blind-samples.json` with its sealed `blind-key.json`.

Offline tests need no key and no network: `go test ./evals/...` (they include a
full 4-surface run against a local stand-in for OpenRouter).

## Spend guard

`--max-spend` is a circuit-breaker, not a forecast. Before each call the runner
reserves an estimate: catalog price × typical tokens until the model has a real
cost, then 2× the highest exact cost that model has had so far. It refuses the
call if spent + in-flight reservations + estimate would pass the cap. After the
call, the reservation is replaced by the provider's exact `usage.cost`. A call
whose cost is not reported (a timeout, an upstream error) is charged at its
estimate, so missing costs make the breaker trip earlier, never later. Refused
calls are logged as `skipped` and left out of the metrics. Cases run in order,
with all models of one case in parallel, so a stop leaves every model with the
same cases. The summary also records OpenRouter's account usage (`GET
/api/v1/key`) before and after the run, which checks the summed costs
independently.

## The four surfaces

### 1. Grading: `datasets/grading.jsonl` (25 cases)

Adapter: `ai.Jev.Judge` with `OpenRouter: true` and `Model` set to the
candidate. The prompt, `state` framing, criteria, 8 KiB request budget and
acceptance gates (confidence ≥ 0.5, chosen probability ≥ 0.8, margin ≥ 0.2) are
exactly production's. Labels follow Scry's 3-way contract:
`correct`/`close`/`incorrect` = `right`/`close`/`missed`.

Mix: 5 exact, 5 paraphrased or terse spoken-style, 3 typos and phonetic
variants (2 `correct`, 1 `close` for "adams myth"), 5 wrong answers that
overlap the card's keywords (cause and effect swapped, wrong sign, jargon that
contradicts the card), 4 incomplete half-answers (`close`), and 3 negation or
prompt-injection attempts (`incorrect`).

Metrics: 3-way accuracy; **FPR**, the share of `incorrect` cases graded
`correct` (the costly SRS error, since a wrong answer gets scheduled as
learned); FNR, the share of `correct` cases graded `incorrect`; abstentions
(gate rejections and `none_of_these`); errors; p50/p95 latency; $/1,000 grades.

Candidates: `openai/gpt-6-luna-decisions` (production default),
`google/gemini-decision-1`, `typesafe/jev-1.14`, `liquid/d1.2`,
`liquid/d1.2-ultra`.

### 2. Explainer ("Why?"): `datasets/explainer.jsonl` (15 cases)

Adapter: `ai.NewClaudeWithRouter(…).Explain`, using production's system
prompt, XML card framing, 1,024 max tokens, the **8 s production timeout**
(`ai.ExplainTimeout`, so a slower model fails here as it would for a learner)
and `ClampExplanation`. There are 3 cases in each of STEM, history, language,
code and finance. Each is a realistic missed or close card with a plausible
misconception, plus `required_concepts`: alternatives separated by `|`,
matched case-insensitively.

Deterministic rubric (0–100), scored on the model's **unclamped** text:
verdict respected (never "you're right" on a missed/close grade) 30; concision
(≤ 40 words 10, ≤ 80 words 5, ≤ 280 characters unclamped 10) 25; format (no
markdown 7, no bullets 6, no preamble 7) 20; contrast (required concepts
covered 15 pro rata, refers to the learner's own answer 10) 25. A failed call
scores 0 in `rubric_mean_all`. Every case also gets a **blind side-by-side**
set (see below).

Candidates: `anthropic/claude-sonnet-5.5` (production default),
`anthropic/claude-haiku-5.5`, `openai/gpt-6-luna`, `google/gemini-4-hydrogen`,
`deepseek/deepseek-v4.1-flash-1017`.

### 3. Drafting: `datasets/drafting.jsonl` (10 requests)

Adapter: `ai.NewClaudeWithRouter(…).Draft`, with production's system prompt,
`<learner>` framing, 16,000 max tokens, streamed chat completions, and the
`draftLines` NDJSON parser feeding a sink. A recording transport also keeps the
raw stream, so the bench checks what the model actually sent, not only what
the lenient parser kept. Requests: 5 pasted source notes, 3 focused topics or
questions, and 2 books (*Thinking in Systems* by Donella Meadows; *The Art of
Doing Science and Engineering* by Richard Hamming). Each has realistic
`learner.knows` titles.

- **Schema valid**: zero malformed lines, head line first (`kind` topic/book),
  4–7 ideas (title ≤ 48 characters with no colon, summary ≤ 140, 1–3 links, 2–4
  items), and the `done` line last.
- **Quality** (0–100, 20 each): standalone questions; non-yes/no questions;
  answers ≤ 25 words; ideas that don't duplicate `learner.knows`; links
  copied exactly from this outline or `learner.knows`.
- Blind side-by-side outlines, p50/p95 latency, time to first idea, and $/100
  drafts.

Candidates: `openai/gpt-6.2-sol`, `google/gemini-4-hydrogen`,
`google/gemini-4-neon`, and `anthropic/claude-opus-5.5` (ceiling).
`anthropic/claude-sonnet-5.5` (production default) is in `drafting-optional`.
It runs only if the budget left after the main pass covers a full pass 1.5×
over, estimated from the main pass's mean tokens at Sonnet's catalog prices.
The decision is logged in `notes`.

### 4. Speech: `datasets/speech/` (12 clips)

**How Scry's speech works today.** The study page records PCM WAV in the
browser and POSTs it to `/study/speech`. `speech.Inspect` checks it, and
silent or too-short recordings never reach a provider. Then
`speech.OpenRouter.Transcribe` sends it to `google/gemini-2.5-flash` as an
`input_audio` part on `/api/v1/chat/completions` with a verbatim-transcript
system prompt, under a 20 s timeout. The transcript fills the answer box
("heard · enter to check"), and the learner presses Enter. Jev then grades
that **text** like a typed answer. There is no audio-to-grade path in
production.

Each clip is a spoken flashcard answer with its card (`prompt`,
`accepted_answers`, `expected_grade`) and 1–3 `critical_terms`: 8 correct
answers and 4 wrong ones. The same clips therefore measure both transcription
and grading. They cover technical terms (PostgreSQL MVCC, mitochondrial ATP
synthase, "O of n log n", idempotent), proper nouns (Treaty of Westphalia,
Gödel's incompleteness theorem, Keynesian multiplier, Milton Friedman) and
numbers with units (113 grams or 8 tablespoons, 40 to 140 degrees Fahrenheit).

Fixtures were generated with **edge-tts 7.3.1** (Microsoft Edge neural TTS, no
credentials) in five voices (US and GB), then re-encoded by ffmpeg to mono
16 kHz 32 kbps MP3 (3.1–5.8 s, about 211 KB in total). `manifest.jsonl` records
the engine, the voice and the ffprobe duration of each clip.
`datasets/speech/make-clips.sh` regenerates them. Production uploads WAV; the
bench sends MP3 to keep the repository small, so only the `input_audio.format`
and the bytes differ.

| Track | What runs | Models |
| - | - | - |
| 4A | Scry's `speech.OpenRouter.Transcribe` unchanged (chat + `input_audio`) | `google/gemini-2.5-flash` (production), `google/gemini-4-hydrogen`, `google/gemini-3.8-flash`, `google/gemini-3.1-flash-lite` |
| 4B | OpenRouter's dedicated `POST /api/v1/audio/transcriptions`. A format probe on clip-01 tries base64 JSON `{model, input_audio:{data, format}}` and a multipart upload, and the bench uses JSON if it works | `x-ai/grok-stt-2.0`, `deepgram/nova-3`, `openai/whisper-large-v3-turbo`, `openai/gpt-4o-mini-transcribe`; `microsoft/mai-transcribe-2` is probed on the shortest clip first and runs on the rest only if that worked and cost ≤ $0.01 |
| 4C | Audio straight into a Decisions model: `POST /api/alpha/decisions` with the clip as an `input_audio` part of the top-level `state` array, plus Jev's exact instructions, criteria and gates (mirrored in `speech.go`; a test fails if they drift from `internal/ai/jev.go`) | `google/gemini-decision-1` |
| 4D | Today's pipeline: 4A's production-model transcript, graded by `ai.Jev.Judge` with the production default Decisions model | `openai/gpt-6-luna-decisions` |

Metrics: WER is computed after normalizing both sides (lowercase, diacritics
folded, apostrophes and hyphens dropped, other punctuation split, number words
→ digits). Term match % is the share of critical terms found in the normalized
transcript. Also p50/p95 latency and $/1,000 five-second clips (Σ exact cost /
Σ audio seconds × 5 × 1,000). A missing `usage.cost` is labelled, and a
catalog-rate estimate is recorded separately, never as exact. 4C and 4D report
accuracy, FPR, FNR, abstentions, latency and cost, like grading.

## Blind comparison

`blind-samples.json` lists every explainer case and every drafting request
with each model's output under `Model A`…`Model E`. The labels are shuffled per
case by a SHA-256-driven Fisher–Yates permutation of the sorted slugs, keyed by
seed, surface and case. The shuffle is deterministic and re-derivable, and it
differs between cases. `blind-key.json` maps surface/case → label → slug. Rate
the outputs first, then open the key.

## Caveats

- The datasets are small (25/15/10/12). Treat a difference of one or two cases as
  noise, not a ranking.
- The rubrics are deterministic proxies (word counts, phrase lists, concept
  substrings). They catch format, length, verdict and coverage failures; they
  don't judge prose quality. That's what the blind sets are for.
- Clips are synthetic TTS: clean audio, no room noise, no accents beyond the
  five voices.
- Catalog prices and model availability change. The summary records exactly
  what the live catalog listed at run start.
