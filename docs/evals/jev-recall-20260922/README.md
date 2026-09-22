# Jev recall grading evaluation

This directory contains the frozen public and synthetic evaluation from 2026-09-22.

No private learner data is present.

## Files

- `corpus.json` contains 14 concepts, 14 questions, and 199 authored responses.
- `gold.json` contains the human-style labels that were frozen before the first Jev call.
- `raw.jsonl` contains one full pass and three added holdout passes.
- `report.md` contains the findings and the frozen recommendation.
- `../../../scripts/evals/jev-recall/main.go` validates, runs, and summarizes the evaluation.

Exact-mode questions do not carry semantic rubrics in `corpus.json`.
The runner sends them to Jev only as diagnostic controls.
The product decision for these items remains deterministic.

## Key source

The runner reads `OPENROUTER_API_KEY` from:

```text
/home/phaedrus/development/misty-step/scry/.env
```

The key is not printed or written to an artifact.
Use `--env PATH` to select another environment file.

## Validation

Run from the repository root:

```sh
go run ./scripts/evals/jev-recall validate
```

## Exact live run sequence

These are the commands used for the committed receipt.
The tune pass used the initial D5 relation threshold.
The tune-only sweep then raised `T_rel` from `0.75` to `0.85`.
No other threshold changed.

```sh
go run ./scripts/evals/jev-recall run \
  --split tune \
  --run-id full \
  --t-rel 0.75 \
  --concurrency 8 \
  --max-spend 0.05

go run ./scripts/evals/jev-recall summarize

go run ./scripts/evals/jev-recall run \
  --split holdout \
  --run-id full \
  --t-rel 0.85 \
  --concurrency 8 \
  --max-spend 0.05

go run ./scripts/evals/jev-recall run \
  --split holdout \
  --run-id holdout-repeat \
  --repeats 3 \
  --t-rel 0.85 \
  --concurrency 8 \
  --max-spend 0.05

go run ./scripts/evals/jev-recall summarize
```

The runner appends to `raw.jsonl`.
It rejects a duplicate `run_id` and `response_id` pair.
Move the committed raw receipt before a fresh reproduction run.

## Verification

Run the required Go checks:

```sh
gofmt -w scripts/evals/jev-recall/main.go
go vet ./...
go test ./...
```

The `summarize` subcommand computes every table in `report.md` from `raw.jsonl`.
It also prints a machine-readable JSON block with the main rates, spend, call count, and frozen parameters.
