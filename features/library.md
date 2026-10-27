# Private Library

The owner searches his Library before adding a text, keeps each Work and Version
once, and builds Studies on what is already there. Behavior authority:
[US-015](../SPEC.md#us-015-find-a-work-before-creating-it).
[ADR 004](../docs/adr/004-rust-cloudflare-candidate.md#2026-10-20-library-storage-amendment-316)
owns storage and rollback.

## Sub-features

- `library-search` finds Works by title, alias or opening words, or says "Nothing matches."
- `library-add` stores a Version with Kind, Sections, Units, Provenance and Rights; units must rejoin to the text exactly.
- `library-keep` attaches a Version to an existing Work by title or alias and returns an identical Version instead of duplicating it.
- `library-supersede` corrects wording as a new Version and keeps the old one and its Attempts.
- `library-show` shows Works and Versions without changing any Study.
- `library-delete` removes unused content and refuses content a Study uses.
- `library-export` exports the Library alone, with no Study, Attempt or token.

## How to get to it (user POV)

- Phone or desktop: footer Text practice (`/recall`), then Library; before a Study, Text practice opens Library entry.
  Search → choose a Version → Learn. Add a text accepts supplied wording with
  optional edition/source. Protected practice requires deliberate help before
  Library content is shown.
- Terminal: `scry search`, `scry version add|show|supersede|delete`,
  `scry work show|edit|delete` and `scry export --library`
  (see the [command reference](../docs/owner-api.md#private-library)).

## Driving it with verify-scry helpers

Preconditions:

- Built Worker and CLI; a CLI login made with an Owner token ([owner-tokens.md](owner-tokens.md)).

- **Phone first study.** `qa/walk --stories "US-018"` drives `qa/phone-library.mjs`
  through native forms, compiled CLI readback, protected Library access, duplicate
  saves, interrupted responses and phone layouts.
- **Find and keep.** Run `.agents/skills/verify-scry/scripts/local-journeys.mjs`.
  `library.find-keep` runs `scry search` → `{"empty":true}`, then `scry version add`
  with authored text, an alias and a sections file. Searching the alias must
  return that Version, and repeating the identical add must return the same
  Version ID.
- **Story walk.** `qa/walk --stories "US-015"` runs eight criterion walks through
  the compiled CLI and isolated Worker with SQLite and local R2.
  `node --test qa/library.test.mjs` reuses them, and `tests/library.rs` proves
  the Study-reference refusal. The [exact Study journey](exact-study.md) proves
  real deletion refusal after active, paused and dropped Studies.
- **CLI transport regression.** `node --test qa/cli-process.test.mjs` covers fast
  no-input exits, drained output, real stdin, explicit refusals and failed input
  delivery. The Library journey keeps all exit, JSON and state assertions; its
  sanitized runtime transcript is `target/library-cli.json` (see #358 / #407).
- **Proof.** `cli-transcript.json` (stdin redacted) under
  the `<stamp>-local-journeys` evidence directory.

## Gotchas

- Use `work.revision` from show for edit, delete and supersede; never reuse example numbers.
- The fixture is synthetic text under Creed metadata. It does not prove the
  owner's full Roman Missal 2011 input, hosted admission, phone experience or
  independent remote recovery.
- Library records and exports contain no Study state, Owner tokens or receipts;
  complete recovery archives keep them separately.
- Hosted CLI use is blocked by Access until #335 (see [sign-in](sign-in.md)).
- Source: `src/library.rs`, `src/api.rs`, `src/engine.rs`, `src/persistence.rs`,
  `src/runtime.rs`, `src/web.rs`, `src/bin/scry.rs`.
