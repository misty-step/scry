# Specification lineage

Historical provenance extracted from SPEC during the 2026-10-14 authority
consolidation. These are dated decisions/observations, not current work state,
serving proof, or competing requirements. [SPEC](../../SPEC.md) owns every story
and criterion; [ADRs](../adr/README.md) own implementation; [the runbook](../runbook.md)
retains operational history. Existing receipts have not been rewritten.

## Intent revisions

- 2026-09-12: the operator rejected the MIS-59 saved-foundations interaction.
  [The study](../design/concept-centered-study.md) and
  [rollout receipt](../qa/foundation-rollout-20260911.json) remain dated evidence,
  not a successful usability trial. Go v5 preserved foundation rows and history
  while retiring their routes/UI; an older binary cannot run after migration.
- 2026-09-23: “Execute this reimagining in full” (MIS-162) authorized the
  concept-centered redesign. US-001–003 changed intent; US-005–012 were new IDs.
- 2026-09-24: simplification removed Map search, note levels and confusion-triggered
  contrast. The accepted unreadable-Link failure and removed US-001.2, US-009.3
  and US-011.3 meanings remain in history; old meanings are never reassigned.
  US-012's reserved retirement notice retains its original recorded date; the
  following simplification receipts are provenance, not an excuse to normalize it.
  The operator rejected self-check for “water” versus “Water”, then requested one
  opinionated rule: only exact key/authored variants resolve locally; all other
  recall goes to short-v1. No exact/flexible learner setting remains.
- 2026-09-28: US-013 replaced the Go Add chooser with private text/optional photo.
- 2026-10-02: prompt-first useful material, followed by interaction/refinement,
  was reaffirmed; S11 extends US-013 without discarding retrieval-led study.
- 2026-10-07: full redesign/rewrite as Rust and Cloudflare was authorized, starting
  fresh rather than importing historical stores. Source implementation is not
  migration, writer activation, paid capability or deployment authorization.

## Earlier Go delivery and evidence

MIS-48 and MIS-162 retain the Go experience → durable personal loop → capture →
repair/ownership → personal-acceptance sequence. MIS-42 was Estate inventory
repair. This is historical delivery order, not the Rust candidate's task plan;
[README](../../README.md#start-or-resume-work) routes current work.

The provider limit increase was recorded on 2026-09-23. Current behavioral
spend/recovery policy is D5 in SPEC. The
[earlier private receipt](../qa/personal-go-acceptance-20260909.json) records native
touch, live generation and independent-VM data restoration. A later private HTTPS
export matched and restored Review rendered in approximately 117 seconds on an
existing recovery VM; provisioning and DNS were not timed, and the synthetic
rehearsal had no production integrations. This is not replacement recovery proof.

## Research and retained technical sources

- [Learning-science synthesis](../science/README.md),
  [reference pack](../research/learning-science-references.md), and
  [AI experiments](../research/ai-learning-design-brainstorm.md).
- [Concept study](../design/concept-centered-study.md) and
  [September interface rounds](../design/redesign-2026-09-24.md): retained
  decisions/proposals, not a second Create contract.
- [Generation research](../research/prose-to-quiz-generation.md),
  [generation receipt](../evals/frictionless-generation-20260908.json),
  [review receipt](../qa/frictionless-review-20260908.json), and
  [feedback research](../research/feedback-flywheel.md).
- Historical Go [transactions](https://go.dev/doc/database/execute-transactions),
  [connections](https://go.dev/doc/database/manage-connections), and
  [template security](https://github.com/golang/go/blob/master/src/html/template/doc.go).
- SQLite [WAL advisory](https://sqlite.org/wal.html),
  [backup API](https://sqlite.org/backup.html), and
  [VACUUM INTO](https://sqlite.org/lang_vacuum.html#vacuuminto).
- Historical HTMX [docs](https://htmx.org/docs/),
  [synchronization](https://htmx.org/attributes/hx-sync/) and
  [preload](https://htmx.org/extensions/preload/): not server transactionality.
- Former exe.dev [HTTPS](https://exe.dev/docs/proxy.md),
  [identity](https://exe.dev/docs/login-with-exe.md),
  [machine tokens](https://exe.dev/docs/https-tokens-for-vms.md), and
  [listener migration](https://exe.dev/docs/migrating-to-exe.md).
- [Pinned Go FSRS v4](https://github.com/open-spaced-repetition/go-fsrs/releases/tag/v4.0.0)
  and [pure-Go SQLite driver](https://gitlab.com/cznic/sqlite).
