# ADR 008: Concept Material compatibility floor

Status: accepted implementation decision for [#393](https://github.com/misty-step/scry/issues/393),
Phase A of [#124](https://github.com/misty-step/scry/issues/124). This is not
authoring activation or a production recovery receipt. [SPEC](../../SPEC.md)
owns behavior; [the release procedure](../rust-cloudflare.md) owns promotion
and recovery. The owner chose Concepts with multiple study Materials and quizzes
on 2026-10-24. The compatibility floor ships before the normal authoring writer.

## Decision

Keep Concepts as the idea identity. A generated Concept retains its existing
Goal association, complete Note chain, prerequisite graph and practice records.
Its optional Goal serializes as the same JSON string, with no new origin field.
Only an owner-authored Concept has no Goal and explicit `owner-authored` origin.
It requires at least one Material and contains no invented Note, prerequisite,
introduction, exposure or practice evidence. No placeholder Goal or Job is made.

`src/materials.rs` owns Material identity, immutable MaterialRevision records,
validation and read projection. Each Material belongs to one existing Concept,
has a stable ID, creation timestamp and current revision ID. Each revision has
its own ID, Material owner, predecessor, ordinal, title, exact supplied UTF-8,
`owner-supplied` provenance and timestamp. The complete chain is linear and
contiguous from ordinal one to the current head, with no missing, orphaned,
cross-owner, cyclic or disconnected revisions. New identities share the existing
global record-identity check. Text remains bounded at 32 KiB; no trimming,
normalization, URL retrieval or sourced-content claim follows from supplied text.

A generated Concept's existing Note chain is projected as one read-only Material
with its original notes, timestamps, evidence and model attribution. It is not
copied into the new store, assigned new identities, or split into one Material
per historical revision. The exact-recall Library remains a separate domain.

## Durable compatibility

New state uses distinct `materials/` and `material_revisions/` SQL rows plus the
sticky `material_capability/v1` marker. That marker is not an optional metadata
field: an older row reader must reject it before committing an unrelated write.
Empty, never-used old state omits the new fields and rows. Once used, the marker
remains even if the maps are empty. No deletion route is added by this floor.

Complete archives use `scry-rust-durable-sqlite-materials-v1` whenever the
capability is present. It takes precedence over practice, attributed spend,
Library and revival identities, including marker-only and mixed-domain states.
An incompatible old archive reader rejects the envelope, rather than ignoring
the optional new JSON fields. New-format rows and archives validate the whole
App graph before acceptance. Existing 1 MiB row and 16 MiB complete-archive
bounds, photo custody, and fresh-space-only restore remain in force.

The later writer's canonical POST receipts are reserved now so rollback does
not discard acknowledged operations: `/concepts/create` returns the authored
Concept identity, `/materials/add` returns the Material identity, and
`/materials/revise` returns the new non-initial revision identity. Each redirects
to `/concepts/{owning-concept-id}`, has no nested result data, and records a
timestamp no earlier than the referenced creation. The existing operation-ID,
payload-hash and acting-token checks remain binding. Exact path, entity kind,
ownership and capability are validated; arbitrary unknown receipt entities are
not admitted. POST is the existing canonical mutation ingress contract, not an
independently recorded receipt fact: Receipt has no method field. Payload hashes
retain the native hash of the canonical path and fields excluding CSRF; archive
validation checks the retained hash shape, not absent original request bodies.
Phase B must prove that GET and other methods refuse before mutation and that
same-path payload replay follows the existing exact-operation semantics. These
are simulated future-writer records in synthetic fixtures, not operations
enabled by this release. Phase B must use this contract or first ship a new
compatible floor.

## Deployment and rollback

Phase A has no normal Concept/Material authoring API, CLI command or UI form.
All three reserved authoring paths refuse before generic receipt replay, even
with an exact retained payload; no storage revision or other write follows.
The receipts remain validated and retained as data. Ordinary existing-operation
replay stays unchanged. Phase B must explicitly enable its separately qualified
handlers rather than treating retained receipts as enabled routes.
The existing private export/CLI export and Concept read surface retain and show
new material from a compatible archive without creating learning observations.
Existing Goal-backed read exposure and assistance fences stay unchanged.
Existing unrelated writes retain every Material and revision.

Before enabling authoring in Phase B, retain this floor's exact tested Worker
artifact and prove that it reads the later format, displays its material, and
preserves new and old records on the first unrelated ordinary mutation. That is
the compatible rollback writer. Earlier artifacts refusing the new rows/archive
prevent silent data loss, but refusal alone is not a functioning rollback.
Never restore an old snapshot over a used namespace or discard accepted new
writes to make an old artifact run. No historical-store reactivation, provider
enablement, spend change or graph prediction is part of either compatibility
mechanism.

## Proof boundaries

Native tests construct a fully valid synthetic domain fixture containing both
learning histories, immutable wording, corrections, resets, spend and opaque
revival custody before adding owner-authored Concepts/Materials. The existing
runtime gate exercises compiled CLI reads, the built Worker, SQLite, local R2,
fresh restore, the first unrelated write, and refusal by a checksum-pinned
previous Worker. Restore-only metadata differences are enumerated, not excluded
wholesale. Tests preserve existing native/historical gates. Local runtime proof
is not independently retrieved hosted recovery or physical-phone acceptance.
