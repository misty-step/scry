# Jev recall grading evaluation

This directory contains the frozen public and synthetic evaluation from 2026-09-22.

No private learner data is present.

## Files

- `corpus.json` contains 14 concepts, 14 questions, and 199 authored responses.
- `gold.json` contains the agent-authored, human-style labels that were frozen before the first Jev call. No human reviewed them before the run.
- `raw.jsonl` contains one full pass and three added holdout passes.
- `report.md` contains the findings and the frozen recommendation.
- `../../../scripts/evals/jev-recall/main.go` validates, runs, and summarizes the evaluation.

Exact-mode questions do not carry semantic rubrics in `corpus.json`.
The runner sends them to Jev only as diagnostic controls.
The product decision for these items remains deterministic.

## Key source

The frozen September 22 receipt used an `OPENROUTER_API_KEY` in Scry's ignored
`.env`; that historical setup is no longer a live-run fallback. A new `run`
requires a distinct `SCRY_SEMANTIC_API_KEY` in the process environment (for
example, injected through a names-only pass-env mapping) or in the selected
`--env PATH` file. The runner rejects a semantic key matching any
`SCRY_MODEL_API_KEY` or `OPENROUTER_API_KEY` available to it and refuses to run
when only those older keys are present. No key is printed or persisted. `verify`
and `summarize` use committed receipts and do not send paid requests.

## Validation

Run from the repository root:

```sh
go run ./scripts/evals/jev-recall validate
```

## Shipped policy verification

This replays the committed raw responses through the product's `learning.GradeSemantic` and sends nothing:

```sh
go run ./scripts/evals/jev-recall verify
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

Every request reserves `--reservation` (default $0.0002) against `--max-spend`
before it is sent and replaces the reservation with the measured cost after
the response arrives, so concurrent workers cannot pass the ceiling together.
A request whose outcome is lost after send (timeout, dropped connection,
unreadable body) is recorded with `cost_unknown` set, keeps its reservation as
spend, and stops the run. Reconcile the provider ledger before running again;
the runner refuses to spend more while an unknown-spend record exists.

`summarize` and `verify` bind every raw record to the loaded corpus and gold
(response identity, split, bucket, learner text, gold action, and the exact
request the shipped builder produces) and refuse a raw file that does not
belong to them.

## Verification

Run the required Go checks:

```sh
gofmt -w scripts/evals/jev-recall/main.go
go vet ./...
go test ./...
```

The `summarize` subcommand computes every table in `report.md` from `raw.jsonl`.
It also prints a machine-readable JSON block with the main rates, spend, call count, and frozen parameters.
