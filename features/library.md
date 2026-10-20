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

- Terminal only for now: `scry search`, `scry version add|show|supersede|delete`,
  `scry work show|edit|delete` and `scry export --library`
  (see the [command reference](../docs/owner-api.md#private-library)). This is
  a CLI-only story, and the browser has no Library page.

## Driving it with verify-scry helpers

Preconditions:

- Built Worker and CLI; a CLI login made with an Owner token ([owner-tokens.md](owner-tokens.md)).

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
- **Proof.** `cli-transcript.json` (stdin redacted) under
  `target/verify-scry/<stamp>-local-journeys/`.

## Gotchas

- Use `work.revision` from show for edit, delete and supersede; never reuse example numbers.
- The fixture is synthetic text under Creed metadata. It does not prove the
  owner's full Roman Missal 2011 input, hosted admission, phone experience or
  independent remote recovery.
- Library records and exports contain no Study state, Owner tokens or receipts;
  complete recovery archives keep them separately.
- Hosted CLI use is blocked by Access until #335 (see [sign-in](sign-in.md)).
- Source: `src/library.rs`, `src/api.rs`, `src/engine.rs`, `src/persistence.rs`,
  `src/runtime.rs`, `src/bin/scry.rs`.
