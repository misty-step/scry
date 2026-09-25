# Stream and honest judgment

Stories: US-002, US-003, US-007, US-008, US-011
Source: internal/web/review.go, internal/web/templates/review.html, internal/web/assets/app.js, internal/learning/*.go, internal/semantic/*.go, internal/store/*.go

## Sub-features

One question or prerequisite introduction at a time; exact/variant local grading, bounded `short-v1` and authored-rubric `semantic-v1`, self-check on uncertainty, assistance/reveal, grade override, and immutable history. Choice keys and variant matches do not need a provider.

## How to get to it (user POV)

Open `/` for the Stream. Answer with a `.choice` in `fieldset.choice-fieldset` or type in `#recall-answer` in `form.answer-form`. Use `/review/intro`'s Got it or I know this already for an introduction. Read the held `role=status` result, then explicitly choose `form[data-next]`'s Next. Open `details.overflow` for Show me or Count as a miss; automatic misses offer I was right below Next. `/history` retains the original result and correction.

## Driving it

On `scry-ws`, `qa/walk --stories "US-002 US-003 US-007 US-008 US-011"` seeds authored DNS/TLS questions with `seed-fixture`, serves with `serve --dev`, and records real pointer/keyboard interaction. For non-browser-observable scheduler, semantic thresholds, lease, and correction contracts, use the focused Go boundaries from `docs/qa/system.md`; do not equate a fixture with live Jev acceptance.

## Gotchas

`--dev` supplies identity only on loopback; isolate inherited `SCRY_*` credentials with `env -i`. Answer/explanation must be absent before self-check or grading. A missing semantic endpoint deliberately yields self-check, not an automatic judgment. Reading an intro or revealing an answer is not cold success. HTMX reconciles uncertain responses by the same operation ID; navigation must not consume the occurrence. The authored fixture pre-acknowledges intros, so prerequisite order requires an independent store-boundary check.
