# Create, preparation, and refinement

Stories: US-004, US-013

S11 extends the Create acceptance for US-013.

Source: src/web.rs, assets/app.js, src/engine.rs, src/generation.rs, src/runtime.rs, src/persistence.rs

## Sub-features

Prompt-first private intent, optional photo, saved useful reference and practice,
honest preparation/refinement, independent criticism, and bounded known/unknown use.
## How to get to it (user POV)

Choose Create (`/create`, with `/add` as an alias). One large intent field accepts
a word, phrase, private material, or long dictated text. An optional photo gives
context. There is no mode picker, setup questionnaire, imported-source
requirement, or diagnostic quiz before useful explanation and practice.

The saved result at `/goals/{id}` keeps original intent and preparation status,
then gives an explanation for each teachable idea with linked practice. General
knowledge is labeled; supplied-source claims keep exact quotes. A pasted URL
remains private intent and does not authorize search or link reading. Historical
Go explicit Topic/Link paths remain at their historical revision, not a new
public compatibility API.

Slow work is pending, partial work says partial, and a failure preserves input.
An unknown sent outcome keeps its allowance and needs deliberate resolution.
Retry is bounded. Refinement records explicit feedback and uses actual attempts;
existing notes/versions/attempts stay inspectable. Concept feedback is separate
from grading, and a prepared question fix remains a draft until versioned Save.

## Driving it

Run `cargo test --locked --test generation_policy --test application --test web_render`
for deterministic boundaries. `bun run dev` starts a fresh synthetic Worker and
local SQLite/R2; its explicit authored demo uses no provider. Real browser proof
checks input, byte limits, failure draft retention, navigation/polling, saved
reference, retries, and later refinement. S11 quality needs human-reviewed real
word/phrase/dictated inputs and genuine feedback, separately from fixture JSON.

The content critic saves candidates before independent checks, shares bounded
allowance, retains unknown usage, and retries the critic without regenerating
saved candidates. Hard defects veto at 0.80; absent/malformed judgments cannot
publish. A skipped critic is recorded when unconfigured. Export retains the
attempts, reasons, raw responses, and known/unknown usage. Live critic calibration
and private production remain separate evidence.

## Gotchas

Synthetic data proves mechanics only. Follow the current QA recipe, retain
source-bound real observations, and keep protected reference/answer material
behind durable assistance. Never attach live integrations or import historical
stores to make a preview appear complete.
