# Baby readiness

Choose a short, source-backed baby-care collection and practise one clear answer
at a time. The three collections cover safe soothing (including Karp's five S's),
everyday newborn safety, and Montessori-inspired awake interaction. This is
memory practice, not medical advice, hands-on training, or certification.

## Sub-features

- Three externally authored mappings, 18 original prompts, checked-source notes.
- Safety constraints remain in the visible question, not only hidden notes.
- Native strict grading accepts normalized case/punctuation, not unsafe extra
  clauses, negation, or guessed fuzzy synonyms. No self-rating.
- Focused Next stays in the chosen collection; only scheduled, due work joins
  default mixed review. Browsing never imports or enrolls content.
- Source notes are available in Library after answering or recording help;
  help does not count as unaided knowledge or reschedule the card.

## How to get to it (user POV)

Open Practice → Choose what to practise → Baby care, then choose Safe soothing,
Everyday safety, or Montessori. Type the short answer, check it, and see native
feedback and progress. Open Source notes after the answer for edition, safety
context, and citations. Existing collections and review history remain intact.

## Driving it with verify-scry helpers

Use the existing `verify-scry` launch/doctor. Run `cargo test --locked --test
baby_readiness` and `node --test qa/keyed-mapping.test.mjs`. The latter includes
`babyJourney` against disposable real Worker/SQLite, then `babyWalk` at 320px
without JavaScript and 390/1280px with enhancement. Evidence is under
`target/keyed-mapping/baby-runtime.json`, `baby-protected.html`,
`baby-source-notes.html`, and `baby-*.png`.

## Gotchas

The source packet and editorial exclusions live in
[checked-source notes](../docs/content/baby-readiness.md); SPEC BABY-01–03 owns
acceptance. Karp and Montessori describe approaches, not clinical authorities;
AAP/CDC safety guidance takes precedence. Do not replace source notes with
commercial sleep claims or introduce floor-bed, weighted-swaddle, or noise
volume targets. A synthetic Worker pass is not authenticated live acceptance.
