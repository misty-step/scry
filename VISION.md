# Scry Product Vision

Scry gets fixed texts and sets into phaedrus's head word for word, and keeps
them there for the fewest minutes a day. Examples are the Apostles' Creed,
a Frost poem, the Mysteries of the Rosary and the NATO alphabet. Scry is for
him alone: one owner, no other accounts, nothing public.

Direction set by the owner on 2026-10-20
([#296](https://github.com/misty-step/scry/issues/296)):

- **Exact recall first.** He names something, and Scry finds it in his
  Library or creates it. He then recites it unit by unit as the cues fade.
  Every word gets a deterministic verdict and a diff. A memory model decides
  when each unit comes back. Understanding, meaning concepts and explanations,
  comes later and builds on the same Library.
- **An agent with tools creates the material.** A generation agent writes a
  Version from model memory when that is good enough, and searches the web
  when it judges a check is worth it. Code checks the result deterministically
  before it enters the Library, and every run is kept.
- **Find before create.** The Library is a cache. Search comes first, and
  each thing to learn is created once. It is private today but shaped so it
  could be shared later without exposing his study.
- **Agent-drivable.** Every action is an API call that an agent can drive
  through the `scry` CLI with an owner token. His phone is one client of the
  same behavior.
- **Cheap to run.** Practice costs nothing. Only creating new material spends
  money.

Scry is not about collection size, streaks, engagement time or a general
learning platform.

## Where things are decided

[SPEC](SPEC.md) alone owns stories, acceptance criteria and policy.
[GLOSSARY](GLOSSARY.md) owns the words. [DESIGN](DESIGN.md) owns visuals.
[ADRs](docs/adr/README.md) own technical decisions. [The runbook](docs/runbook.md)
owns operational records. [QA](docs/qa/system.md) owns verification procedures.

Earlier intent, the 2026-10-02 prompt-first curiosity notebook, is preserved in
[specification lineage](docs/history/specification-lineage.md) and the
[concept study](docs/design/concept-centered-study.md).
[Learning-science research](docs/science/README.md) supplies rationale and
limits, not requirements.
