# Authored Concepts and Materials

This is the Phase B API/CLI adapter reference for supplied text. It is not a
release, deployment, rollback qualification or recovery receipt. The Phase A
compatibility floor and its activation prerequisites remain in
[ADR 008](../adr/008-concept-material-compatibility.md).

Use the existing [Owner token login](../owner-api.md). All routes require an
Owner token; authoring supplied text requires no paid permission, calls no
provider and does not fetch URLs. The CLI reuses its existing origin, private
login file, `--config`, `--json`, refusal exit status and operation-ID handling.
The existing hosted admission and private-access restrictions still apply.

## CLI

```sh
scry concept create --name 'Conditional requests' \
  --summary 'How a cache validates a stored response.' \
  --title 'My reference' --text-file /private/reference.txt \
  --operation-id retained-create-operation --json
scry concept list --json
scry concept show CONCEPT_ID --json
scry material list CONCEPT_ID --json

scry material add CONCEPT_ID --title 'Another explanation' \
  --text-file /private/another-reference.txt \
  --operation-id retained-add-operation --json
scry material show MATERIAL_ID --json
scry material history MATERIAL_ID --json
scry material show MATERIAL_ID --revision-id REVISION_ID --json

scry material revise MATERIAL_ID \
  --expected-revision-id CURRENT_REVISION_ID --title 'Corrected reference' \
  --text-file /private/corrected-reference.txt \
  --operation-id retained-revision-operation --json
```

Each authoring command accepts exactly one of `--text-file PATH` or `--stdin`.
For example, `cat /private/reference.txt | scry material add CONCEPT_ID --title
'My reference' --stdin --operation-id retained-add-operation --json` submits the
same bytes as the file option. No inline text or `--source` option is provided.
A URL inside the text is literal supplied text, not a retrieval instruction or
verified-source claim. There is no `--paid` option for these commands.

`concept create` requires `--name` and creates the first Material atomically.
`--summary` is optional. Add and revise require a complete `--title` and text,
not a partial patch. Read `material.current_revision_id` from Material show and
pass it explicitly to revise. The CLI never silently reads and overwrites a
newer revision. If another revision won first, the mutation conflicts; inspect
the current Material and deliberately reconcile the edit.

Names and titles allow at most 200 UTF-8 bytes, optional summary at most 2,000,
and text at most 32,768. The engine validates the supplied content. The CLI
bounds text reads and refuses oversize or invalid UTF-8 without truncating.
Spaces, tabs, CRLF, Unicode and trailing line breaks are not normalized.
The new JSON authoring routes reject malformed UTF-8 and unpaired surrogate
escapes instead of replacing them. Their native forms likewise reject malformed
raw or percent-encoded UTF-8 and file-valued fields; only supplied text is accepted.

Supply and retain `--operation-id` for a mutation that might need a retry.
Omitting it generates a fresh ID, as for existing CLI mutations. After a lost
response, repeat the exact fields, input bytes and operation ID. A changed
payload under that ID is a conflict rather than a second write.

## Reads

| Method | Route | Result |
| --- | --- | --- |
| GET | `/api/v1/concepts` | `{"concepts":[Concept]}` |
| GET | `/api/v1/concepts/ID` | `{"concept":Concept,"materials":[MaterialView]}` |
| GET | `/api/v1/concepts/ID/materials` | `{"materials":[MaterialView]}` |
| GET | `/api/v1/materials/ID` | `{"material":Material,"revisions":[MaterialRevision]}` |
| GET | `/api/v1/materials/ID/revisions` | Same Material and ordered revision history |
| GET | `/api/v1/materials/ID/revisions/REVISION_ID` | `{"material":Material,"revision":MaterialRevision}` |

The CLI returns the API JSON unchanged. A supplied Material has its own stable
`id`, `concept_id`, `current_revision_id` and `created_ms`. Revisions retain
their `id`, `material_id`, `predecessor_id`, `ordinal`, `title`, exact `text`,
`provenance` (`owner-supplied`) and `created_ms`.

Concept reads include all its Materials. A generated Concept's original Note
chain appears as one `MaterialView` with `kind: "legacy-note"`, `concept_id`
and the original `revisions`. This projection is read-only and has no invented
Material ID. Use Concept show or Material list for that chain; do not treat a
Note ID as a supplied Material to revise. New independently authored Materials
appear with `kind: "supplied"`, their `material` and ordered `revisions`.
Existing assistance/privacy fences remain authoritative for these reads.

## Mutations

| Method | Route | Required fields, besides `operation_id` |
| --- | --- | --- |
| POST | `/api/v1/concepts/create` | `name`, `title`, `text`; optional `summary` |
| POST | `/api/v1/materials/add` | `concept_id`, `title`, `text` |
| POST | `/api/v1/materials/revise` | `material_id`, `expected_revision_id`, `title`, `text` |

Payloads are flat JSON objects. For example:

```json
{
  "operation_id": "retained-revision-operation",
  "material_id": "MATERIAL_ID",
  "expected_revision_id": "CURRENT_REVISION_ID",
  "title": "Corrected reference",
  "text": "Exact supplied text\r\n"
}
```

The routes map to the engine's canonical `/concepts/create`, `/materials/add`
and `/materials/revise` operations. The shared response contains `result` and
`replayed`. `result.entity_id` identifies the Concept, added Material or new
revision respectively, and `result.redirect` points to its owning Concept.
Show that identity or read the owning Concept to inspect the committed content.
Receipts retain acting-token attribution and exact-operation replay semantics.

Creating a Concept invents no Goal, Note, prerequisites, quizzes, exposure,
learning history or spend. Adding a Material does not replace the Concept's
other Materials. Revising appends immutable wording and advances only that
Material's current revision; existing revisions and practice history remain.
These commands do not create or modify an exact-recall Library Work or Version.
