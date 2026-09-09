# Learning Science Doctrine


This is the living bibliography for decisions that belong in
`memory-engine` rather than in one client experiment. A principle is adopted
only when it names the source evidence, the product or kernel decision it
drives, and an executable oracle that can fail when the behavior drifts.

## Adopted Principles

| Principle | Source evidence | Engine decision | Executable oracle |
| --- | --- | --- | --- |
| Retrieval practice is the primary learning event. | Roediger and Karpicke (2006), DOI `10.1111/j.1467-9280.2006.01693.x`, [PubMed](https://pubmed.ncbi.nlm.nih.gov/16507066/); [Anki rating guidance](https://docs.ankiweb.net/studying.html#answer-buttons). | Correct recall maps to Good. Close, Wrong, and Revealed map to Again: incorrect or assisted answers cannot take the successful-recall stability path. Speed alone does not establish Easy. | `cargo test -p memory-engine-core grading` and `cargo test -p memory-engine-service` exercise grading, exposure, and atomic schedule application. |
| Spacing is robust, but timing stays an explicit policy. | Cepeda, Pashler, Vul, Wixted, and Rohrer (2006), "Distributed practice in verbal recall tasks," DOI `10.1037/0033-2909.132.3.354`, [PubMed](https://pubmed.ncbi.nlm.nih.gov/16719566/). | The pure scheduler owns replayable `ScheduleState` transitions. The current FSRS constants are a deterministic baseline, not a claim of universal optimality. | `cargo test -p memory-engine-core` runs scheduler parity and JSON-safe schedule-shape checks. `cargo run -p memory-engine-bench` now prints `scheduling.fsrs.synthetic_histories`, which compares steady successful review histories against repaired-lapse histories. |
| Interleaving should mix intentional contrasts, not randomize the queue. | Brunmair and Richter (2019), [PubMed](https://pubmed.ncbi.nlm.nih.gov/31556629/): benefits depend on material and setting. | Study supplies recent persisted attempts to the existing concept/source/domain separation policy. Due learning/relearning stays ahead of review and new variants; the no-dead-end progression fallback is soft, but never revives superseded items. | `cargo test -p memory-engine-study --test beta_study` protects real feed selection; `cargo run -p memory-engine-bench` exercises the kernel anti-clump scenario. |
| Desirable difficulty means purposeful retrieval depth, not arbitrary friction. | Bjork and Bjork, "Creating Desirable Difficulties to Enhance Learning," [UCLA Bjork Lab PDF](https://bjorklab.psych.ucla.edu/wp-content/uploads/sites/13/2016/04/EBjork_RBjork_2011.pdf); Maddox and Balota (2015), "Retrieval Practice and Spacing Effects in Young and Older Adults," [PMC](https://pmc.ncbi.nlm.nih.gov/articles/PMC4480221/). | Choose recognition, recall, or application for the material. Preserve explicitly supplied progression, but do not require a four-stage ladder for every concept. | `cargo test -p memory-engine-generation generation_preserves_retrieval_depth_progression_tiers -- --exact` proves structured generation preserves supplied stages with stage orders 1, 2, 3, and 4. |
| Retrieval variability fights prompt memorization. | Brunmair and Richter (2019) supports conditional interleaving benefits, especially when learners must discriminate between materials; teaching-science summaries in `docs/research/learning-science-references.md` connect retrieval, spacing, interleaving, elaboration, examples, and exercises as complementary strategies. | The beta study boundary treats same-concept same-stage prompts as separate item variants, rotates due variants before repeating the same phrasing, projects MCQ choices in a deterministic changing order, and exposes response-time plus success trends from the existing attempt log. | `cargo test -p memory-engine-study --test beta_study queue_rotates_due_variants_with_the_same_concept_and_stage -- --exact`, `cargo test -p memory-engine-study --test beta_study multiple_choice_choices_rotate_between_reviews_without_changing_answer -- --exact`, and `cargo test -p memory-engine-bench generation::tests::variant_quality_requires_distinct_same_concept_stage_phrasings_without_answer_leakage -- --exact` protect the behavior and eval. |
| FSRS personalization needs history, versioning, and simulation before promotion. | The official `fsrs4anki` README says the optimizer fits parameters to review history, [GitHub](https://github.com/open-spaced-repetition/fsrs4anki). The Rust FSRS project documents optimizer input as review-history items, [fsrs-rs](https://github.com/open-spaced-repetition/fsrs-rs). | `memory-engine-core` keeps fixed default parameters until the boundary has enough persisted review history and an explicit scheduler-version contract. Per-user optimization belongs behind a shaped analytics or boundary-crate slice, not as a silent kernel mutation. | `cargo run -p memory-engine-bench` prints `scheduling.fsrs.synthetic_histories`; future optimizer work must extend this into versioned replay fixtures before replacing defaults. |

## Policy and evidence boundaries

FSRS-6 retains its published 21 default weights and a 90% requested-retention
baseline; see the [official equations](https://github.com/open-spaced-repetition/awesome-fsrs/wiki/The-Algorithm).
An isolated comparison against `ts-fsrs@5.4.2` covered 85 transitions: new,
learning, relearning, mature review, UTC midnight, maximum stability, delayed
reviews, and all four ratings. Before correction, 43 differed; afterward all
85 matched. Five consequential failures remain in `fixtures/scheduler.json`.
This proves those transitions, not full upstream equivalence or learning gains.

The existing wire state has no learning-step counter. The ten-minute scheduled
gap recovers its completed step; an ambiguous Hard gap conservatively repeats
the ten-minute step rather than guessing mastery. No stored schedule or prior
attempt is rewritten. A full counter-preserving upstream contract would need
an explicit state migration.

The 3/2/1 separation windows, three-review progression heuristic, automatic
publication, and 90% retention target are product policies, not universal
scientific findings. Interleaving evidence does not prove that mixing arbitrary
domains helps, nor that every item needs a recognition-to-composition ladder.
Generation should choose useful distinct mechanisms, contrasts, or applications
for concepts and prose, while preserving complete coverage of requested sets
and exact-text learning. Model-expanded facts are not source-verified claims.

## Rejected Principles

| Rejected principle | Source evidence | Why rejected here | Guardrail |
| --- | --- | --- | --- |
| Learning-style matching or "meshing" instruction to visual/auditory/etc. learner profiles. | Pashler, McDaniel, Rohrer, and Bjork, "Learning Styles: Concepts and Evidence," DOI `10.1111/j.1539-6053.2009.01038.x`, [SAGE](https://journals.sagepub.com/doi/abs/10.1111/j.1539-6053.2009.01038.x), [PubMed](https://pubmed.ncbi.nlm.nih.gov/26162104/). | The claim would add identity/profile branches without a strong enough outcome oracle. `memory-engine` should personalize from demonstrated review behavior, source difficulty, and item history before learner-style labels. | Do not add scheduler or generation branches for learner-style labels without a shaped ticket, cited contrary evidence, and a behavior eval that compares outcomes against the current retrieval/spacing/interleaving baseline. |

## Review Checklist

- A new science-backed feature must add or update one row above.
- Every adopted row must name at least one executable oracle.
- Every scheduler policy change must say whether it changes the FSRS default,
  per-user fitting, or only boundary/client behavior.
- Every generation policy change must preserve source provenance and identify
  the retrieval depth it is trying to exercise.
