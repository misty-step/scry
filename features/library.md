# Private Library

Behavior authority: [US-015](../SPEC.md#us-015-find-a-work-before-creating-it).

Build `scry`, log in with a local synthetic Owner token, and follow the
[CLI and API reference](../docs/owner-api.md#private-library). Search an empty
Library, add supplied text with Sections and Units, repeat it, then add another
wording through an alias. Show the Work and Versions, supersede wording, edit
Work metadata, delete unused content, and export only the Library.

`qa/walk --stories "US-015"` runs eight criterion walks through the compiled
CLI and actual isolated Worker with SQLite and local R2. This CLI-only story
needs no browser. `node --test qa/library.test.mjs` reuses those journeys;
`tests/library.rs` proves the durable Study-reference refusal contract.
Actual used Study and Attempt behavior needs #317, which must retain references
when a Study is dropped. The fixture is caller-authored synthetic text under
Creed title/version metadata. It does not prove the owner's full Roman Missal
2011 input, hosted admission, phone experience, or independent remote recovery.

Source: `src/library.rs`, `src/api.rs`, `src/engine.rs`, `src/persistence.rs`,
`src/runtime.rs`, `src/bin/scry.rs`. Library records and exports contain no
Study state, Owner tokens, or audit receipts. Complete private recovery archives
retain the separate records. [ADR 004](../docs/adr/004-rust-cloudflare-candidate.md#2026-10-20-library-storage-amendment-316)
owns the storage and rollback decision.
