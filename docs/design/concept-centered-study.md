# Concept-centered study

Captured 2026-09-12. This note separates operator direction from assistant
proposals and unresolved design. It is not implementation authorization, shipped
behavior, or an acceptance receipt. [VISION](../../VISION.md) owns product intent;
[SPEC](../../SPEC.md) retains stable criteria and existing safeguards.
[MIS-59](https://linear.app/misty-step/issue/MIS-59/learn-missing-foundations-and-return-to-the-original-question)
owns the rejection, design pause, and next acceptance decision;
[MIS-48](https://linear.app/misty-step/issue/MIS-48/execute-the-personal-go-sqlite-htmx-scry-rewrite)
retains umbrella acceptance.

## Operator findings and direction

The operator tried the foundations experience and rejected it as bad, awkward,
and clunky. Usability has a negative assessment, not merely an outstanding
sign-off. The instruction is to step back, flesh out the product design, and
regroup before further implementation. Earlier phone-flow approval applies to
that earlier flow, not this foundations experience. Historical technical proof
remains valid within its recorded limits; it cannot establish usefulness.

Scry remains **quiz-first ongoing review and study**. Ordinary quiz scheduling
and interleaving stay the default, with reference search and navigation available
when the learner needs understanding rather than another recall attempt.

The user library has two object types:

- **Quizzes:** true/false, multiple choice, free response, and other supportable
  question forms for attempting recall or understanding.
- **References:** material to read, watch, or study to understand concepts, not
  disposable feedback attached to one answer.

**Concepts are the core schema identity.** Quizzes and references each connect
many-to-many to concepts; concepts relate to other concepts, including
prerequisites. The operator prefers explicit graph relationships over
similarity-only matching. Vector/embedding storage was explored, not adopted as
a requirement. This conceptual direction does not prescribe a graph database,
editor, or verified catalog of what the learner knows.

Distinguish difficult-but-attemptable recall from being not ready because assumed
prerequisites are missing. A learner can move from a quiz to relevant references.
If a reference also assumes missing knowledge, they can recursively study deeper
prerequisite material. Repeating the target at the same level does not by itself
provide foundations. This is a general learning need; illustrative biology is
not a domain requirement or evidence of a successful live journey.

Alongside normal quiz creation, generate references as needed when the learner
struggles or requests a bridge. References must be durable, searchable, and
reusable across later study, not discarded after the immediate difficulty.

## Assistant proposals — not yet accepted

These are candidates for design review, not additional approved features:

- Give relationships explicit roles such as **assesses**, **teaches**, **assumes**,
  and **mentions**, rather than treating every association as instruction or
  assessment. The existing implementation's coverage vocabulary is not adoption
  of this proposed concept model.
- Use one continuous study workspace that retains the path and original context.
  Return is available, not compulsory; the learner may continue deeper or stop.
- Make help learner-directed rather than automatically taking over with remedial
  work after every wrong answer. A miss need not mean missing prerequisites.
- Treat a bridge as a path through ordinary references and quizzes, not a third
  library type or an isolated per-question bundle.
- Reuse suitable references first; generate only targeted missing material.
- Begin with SQLite relation tables and text/concept search. Add semantic
  retrieval only if a demonstrated retrieval need justifies it. This is an
  implementation proposal, not a newly mandated storage design.
- Do not count reading or immediately assisted practice as unaided mastery. This
  proposed evidence treatment extends the existing assistance honesty bar;
  exact new scheduling or inference policies are not settled.

Existing assistance, immutable history, privacy, bounded spending, and recovery
safeguards remain binding. This reset neither removes current protection nor
adopts its exact warmth window as the future design. No new estimator, deferral,
or warmth policy is agreed. A generated concept relationship describes a model's
claim about material, not verified learner knowledge or blanket prerequisite
mastery/failure. Understanding, exposure, attempted recall, and later unaided
recall must not be conflated.

## Consequential open choices

- **Concept identity:** useful granularity, overlap and naming, and how incorrect
  concepts or relationships are corrected without rewriting history.
- **Interaction and discovery:** concrete quiz-to-reference, deeper-study, search,
  and return controls; enough retained context without a navigation burden.
- **Readiness:** how to identify a missing prerequisite without falsely diagnosing
  every linked concept from one difficult or wrong answer.
- **Reference quality:** the right unit of reusable explanation and how to judge
  whether it genuinely teaches at a simpler level rather than restating the target.
- **Assistance and scheduling:** what exposure affects, how voluntary return or
  stopping behaves, and how ordinary review resumes without invented mastery.
- **Bounded continuation:** the smallest next slice only after agreement on the
  experience, rather than automatically executing the earlier roadmap.

## Proposed design-review journey

Use this observable journey as a **proposed design oracle**, not a build order or
PASS: a too-hard quiz leads to a same-level reference that is still too advanced;
the learner reaches a genuinely simpler prerequisite reference, attempts distinct
simpler practice, then voluntarily returns or stops. On a later visit they can
search for and reuse that material.

Review this proposal with agreed real learning content, not only illustrative
examples: can the learner find an understandable starting point, retain
orientation, choose their next action, and reuse useful material? Record operator
agreement or rejection and resolve consequential object, session/return, and
learning-evidence boundaries above before selecting engineering work. A working
transition, generated relation, or successful assisted answer alone does not
establish that outcome.

If that review leads to a separately authorized implementation slice, first map
saved foundations to the agreed model without discarding them or rewriting
immutable learning, content, or spend history. Record unresolved mappings rather
than inventing evidence. Paused paid work stays paused; a redesign does not
authorize replay. Any schema transition needs the existing
[QA migration/recovery proof](../qa/system.md#foundation-detour-mis-59) and
[runbook recovery boundary](../runbook.md#schema-2-foundation-release-boundary),
not a second migration or rollback procedure here. This is a conditional
transition plan, not a migration already implemented or permission to resume.
