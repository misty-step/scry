# NATO review

Practise the 26 NATO phonetic words with server-owned deterministic grading.
The current product rules are in [SPEC](../SPEC.md#nato-focused-review-2026-10-29).

## Sub-features

- One Start practice action imports and starts the native deck atomically.
- Type, Check, inspect feedback, and Next; no client-supplied grade.
- Start/lesson selection and Next choose due/new letters, interleaving away from
  the just-completed letter when another eligible letter exists. Future-due A
  must not reappear just because the learner reselects NATO. With all letters
  future-due, show caught-up and the next scheduled time; reads do not select.
- Reveal records assistance separately from independent recall.
- Per-letter count, accuracy and next-review visibility reflect stored evidence.

## How to get to it (user POV)

Open `/`, choose Start practice, or choose NATO in the practice chooser. Review
opens at `/recall`. External agents can import `examples/nato-alphabet.json` and
start the same mapping through the documented CLI/MCP entry points.

## Driving it with verify-scry helpers

Run `node --test qa/nato-focus.test.mjs` for the atomic real-Worker journey.
Run `node --test qa/keyed-mapping.test.mjs` for pointer/keyboard, native/enhanced
phone and desktop flows. Use `#answer`, the native answer form, the visible
verdict, and `form[data-next]`, not direct storage mutations. The suites assert
protected recall, replay safety, stored effects and honest progress. Capture
`target/keyed-mapping/focused-runtime.json`, `browser.json` and `nato-focus-*`
screenshots in the external proof directory.

## Gotchas

“Alpha” is not NATO “Alfa”; expected grading must follow the server's frozen
edition. A repeated Start must not create duplicate learning state. Runtime
proof does not establish visual usability, authenticated acceptance or durable
learning efficacy. Do not use production learner data as seed material.
