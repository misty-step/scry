use std::{cell::Cell, fs, path::PathBuf};

use memory_engine_core::{ExactPromptKind, GradeContext, Grader, Prompt, Verdict};
use memory_engine_generation::{
    classify_learning_intent, run_beta_generation_with_provider, BetaGenerationRequest,
    DraftCandidate, DraftProvider, DraftRejection, FakeModelProvider, FallbackProvider,
    LearningIntent, ProviderDrafts, ProviderFailure, ProviderUsage,
};
use memory_engine_persistence::{
    BetaPersistenceStore, GeneratedLearningActivityKind, GeneratedPromptModel,
    GeneratedPromptValidationStatus, GenerationRunUsage, SourceDocument, SourceDocumentKind,
    SourcePermission,
};

const NOW: i64 = 1_780_162_400_000;

const PROSE: &str = "Mitochondria are organelles that generate most of the cell's supply of \
adenosine triphosphate. The number of mitochondria in a cell varies widely by organism and \
tissue type. They are sometimes called the powerhouse of the cell because they produce usable \
chemical energy.";

#[test]
fn title_only_verbatim_cue_does_not_activate_exhaustive_policy() {
    let source = SourceDocument {
        id: "src-title-only-verbatim".to_owned(),
        kind: SourceDocumentKind::Text,
        title: "Apostles' Creed verbatim sequence".to_owned(),
        project_key: None,
        body: Some(String::new()),
        uri: None,
        permission: SourcePermission::ModelEligible,
        freshness: Some(NOW),
        ttl_expires_at: None,
        created_at: NOW,
        archived_at: None,
    };

    let classification = classify_learning_intent(&source);
    assert!(
        !matches!(
            classification.intent,
            LearningIntent::EnumerableSet | LearningIntent::VerbatimMemorization
        ),
        "title-only prose must not activate exhaustive enumerable/verbatim policy: {:?}",
        classification.intent
    );
}

#[test]
fn fallback_provider_rejects_local_only_source_before_forwarding() {
    let source = SourceDocument {
        id: "src-local-only".to_owned(),
        kind: SourceDocumentKind::Text,
        title: "Private notes".to_owned(),
        project_key: None,
        body: Some("private notes".to_owned()),
        uri: None,
        permission: SourcePermission::LocalOnly,
        freshness: Some(NOW),
        ttl_expires_at: None,
        created_at: NOW,
        archived_at: None,
    };
    let fallback = FallbackProvider::new(&FakeModelProvider);
    let failure = fallback
        .generate_drafts(&source)
        .expect_err("local-only source must not reach either provider");

    assert!(matches!(
        failure.kind(),
        memory_engine_generation::ProviderFailureKind::LocalOnlySource(id)
            if id == "src-local-only"
    ));

    let failure = fallback
        .repair_drafts(&source, &[])
        .expect_err("local-only repair must not reach either provider");
    assert!(matches!(
        failure.kind(),
        memory_engine_generation::ProviderFailureKind::LocalOnlySource(id)
            if id == "src-local-only"
    ));
}

#[test]
fn fallback_provider_rejects_archived_source_before_forwarding() {
    let mut source = SourceDocument {
        id: "archived-source".to_owned(),
        title: "Archived source".to_owned(),
        kind: SourceDocumentKind::Text,
        project_key: None,
        body: Some("must never leave".to_owned()),
        uri: None,
        permission: SourcePermission::ModelEligible,
        freshness: Some(NOW),
        ttl_expires_at: None,
        created_at: NOW,
        archived_at: Some(123),
    };
    let provider = FallbackProvider::new(&FakeModelProvider);

    let failure = provider
        .generate_drafts(&source)
        .expect_err("archived source must fail before forwarding");
    assert!(matches!(
        failure.kind(),
        memory_engine_generation::ProviderFailureKind::ArchivedSource(id) if id == "archived-source"
    ));

    source.archived_at = Some(456);
    let failure = provider
        .repair_drafts(&source, &[])
        .expect_err("archived repair source must fail before forwarding");
    assert!(matches!(
        failure.kind(),
        memory_engine_generation::ProviderFailureKind::ArchivedSource(id) if id == "archived-source"
    ));
}

#[test]
fn fake_model_provider_generates_grounded_drafts_from_arbitrary_prose() {
    let directory = TempDirectory::new("fake-provider");
    let mut store = open_store_with_prose(&directory);

    let result = run_beta_generation_with_provider(
        &mut store,
        &FakeModelProvider,
        request("run-prose", "src-prose"),
    )
    .expect("generation");

    assert!(
        !result.accepted_draft_ids.is_empty(),
        "expected at least one accepted draft from arbitrary prose, got failures: {:?}",
        result.validation_failures
    );
    let snapshot = store.snapshot();
    let reference = &snapshot.reference_spans[0];
    assert!(
        PROSE.contains(&reference.text),
        "reference span must quote the source verbatim: {:?}",
        reference.text
    );
    assert_eq!(snapshot.generation_runs[0].model, "fake-model");
}

#[test]
fn fake_model_provider_branches_draft_shapes_by_learning_intent() {
    let model = FakeModelProvider;
    let verbatim = source_document(
        "src-poem",
        "Emily Dickinson poem 314",
        "\"Hope\" is the thing with feathers -\nThat perches in the soul -\nAnd sings the tune without the words -\nAnd never stops - at all -",
    );
    let concept = source_document(
        "src-concept",
        "Mitochondria",
        "Mitochondria are organelles that generate most of the cell's supply of adenosine triphosphate because cells use ATP as chemical energy.",
    );
    let fact = source_document(
        "src-fact",
        "Cell facts",
        "ATP means cellular energy. DNA means genetic material. RNA means messenger material.",
    );
    let process = source_document(
        "src-process",
        "Sourdough starter",
        "To maintain a sourdough starter, discard all but 50 grams and feed the remainder with equal weights of flour and water. Always let it double before baking.",
    );

    assert_eq!(
        classify_learning_intent(&verbatim).intent,
        LearningIntent::VerbatimMemorization
    );
    assert_eq!(
        classify_learning_intent(&concept).intent,
        LearningIntent::ConceptUnderstanding
    );
    assert_eq!(
        classify_learning_intent(&fact).intent,
        LearningIntent::FactRecall
    );
    assert_eq!(
        classify_learning_intent(&process).intent,
        LearningIntent::ProcedureProcess
    );

    let verbatim_drafts = model.generate_drafts(&verbatim).expect("verbatim");
    assert_eq!(
        verbatim_drafts.learning_intent,
        Some(LearningIntent::VerbatimMemorization)
    );
    assert!(
        verbatim_drafts
            .candidates
            .iter()
            .all(|candidate| candidate.activity_kind == GeneratedLearningActivityKind::Exercise),
        "verbatim captures should produce recitation exercises, not quizzes: {:?}",
        verbatim_drafts.candidates
    );
    assert!(verbatim_drafts
        .candidates
        .iter()
        .any(|candidate| candidate.activity_stage.contains("cued")));
    assert!(verbatim_drafts
        .candidates
        .iter()
        .any(|candidate| candidate.activity_stage.contains("free")));

    let concept_drafts = model.generate_drafts(&concept).expect("concept");
    assert_eq!(
        concept_drafts.learning_intent,
        Some(LearningIntent::ConceptUnderstanding)
    );
    assert!(concept_drafts
        .candidates
        .iter()
        .any(|candidate| candidate.activity_stage.contains("free")));

    let fact_drafts = model.generate_drafts(&fact).expect("fact");
    assert_eq!(
        fact_drafts.learning_intent,
        Some(LearningIntent::FactRecall)
    );
    assert!(fact_drafts
        .candidates
        .iter()
        .any(|candidate| !candidate.distractors.is_empty()));

    let process_drafts = model.generate_drafts(&process).expect("process");
    assert_eq!(
        process_drafts.learning_intent,
        Some(LearningIntent::ProcedureProcess)
    );
    assert!(process_drafts
        .candidates
        .iter()
        .any(|candidate| candidate.activity_stage.contains("composition")));
}

#[test]
fn enumerable_sources_emit_every_mapping_in_the_non_derivable_direction() {
    let source = source_document(
        "src-enumerable",
        "NATO phonetic alphabet",
        "A is Alfa. B is Bravo. C is Charlie. D is Delta.",
    );

    let classification = classify_learning_intent(&source);
    assert_eq!(classification.intent, LearningIntent::EnumerableSet);
    assert_eq!(
        LearningIntent::from_label("enumerable_set"),
        Some(LearningIntent::EnumerableSet)
    );

    let drafts = FakeModelProvider
        .generate_drafts(&source)
        .expect("enumerable generation");
    assert_eq!(drafts.learning_intent, Some(LearningIntent::EnumerableSet));
    assert_eq!(drafts.candidates.len(), 4);
    assert_eq!(
        drafts
            .candidates
            .iter()
            .map(|candidate| candidate.answer.as_str())
            .collect::<Vec<_>>(),
        ["Alfa", "Bravo", "Charlie", "Delta"]
    );
    assert!(drafts.candidates.iter().all(|candidate| {
        candidate.distractors.is_empty()
            && candidate.question.contains("letter")
            && candidate.activity_stage == "production-recall"
            && candidate.evidence.is_some()
    }));
    assert!(drafts
        .candidates
        .iter()
        .all(|candidate| !candidate.question.contains("which letter")));
}

#[test]
fn enumerable_numbered_lists_preserve_order_and_source_evidence() {
    let source = source_document("src-list", "Ordered terms", "1. Alpha\n2. Beta\n3. Gamma");

    let drafts = FakeModelProvider
        .generate_drafts(&source)
        .expect("numbered list generation");
    assert_eq!(drafts.learning_intent, Some(LearningIntent::EnumerableSet));
    assert_eq!(
        drafts
            .candidates
            .iter()
            .map(|candidate| candidate.answer.as_str())
            .collect::<Vec<_>>(),
        ["Alpha", "Beta", "Gamma"]
    );
    assert_eq!(drafts.candidates[0].index, 1);
    assert_eq!(drafts.candidates[2].index, 3);
    assert!(drafts
        .candidates
        .iter()
        .all(|candidate| !candidate.question.contains("letter")));
    assert_eq!(
        drafts
            .candidates
            .iter()
            .map(|candidate| candidate.index)
            .collect::<Vec<_>>(),
        [1, 2, 3]
    );
    assert!(drafts.candidates.iter().all(|candidate| {
        let evidence = candidate.evidence.as_deref().expect("source quotation");
        memory_engine_generation::evidence_quote_matches(source.body.as_deref().unwrap(), evidence)
            && memory_engine_generation::answer_has_evidence_support(evidence, &candidate.answer)
    }));
}

#[test]
fn exhaustive_sets_over_the_limit_are_not_silently_sampled() {
    let body = (1..=61)
        .map(|index| format!("{index}. Entry {index}"))
        .collect::<Vec<_>>()
        .join("\n");
    let source = source_document("src-large-set", "Ordered labels", &body);
    let generated = FakeModelProvider
        .generate_drafts(&source)
        .expect("finite set");
    let governed = memory_engine_generation::enforce_content_policy(&source, generated);
    assert!(
        governed.candidates.is_empty(),
        "partial finite coverage must never enter study"
    );
    assert!(
        !governed.failures.is_empty(),
        "the learner must receive a split-capture failure"
    );
}

#[test]
fn long_verbatim_sources_keep_bounded_substantive_quotes_for_each_unit() {
    let mut lines = vec!["Begin.".to_owned()];
    lines.extend((1..=50).map(|index| format!(
        "Line {index} follows the river beyond the meadow and through the orchard, where the changing seasons bring new colors to the hillside and the village gathers to remember the journey together."
    )));
    let body = lines.join("\n");
    assert!(body.len() > 8_000);
    let source = source_document("src-long-verse", "Long verse", &body);
    let generated = FakeModelProvider
        .generate_drafts(&source)
        .expect("verbatim passage");
    let governed = memory_engine_generation::enforce_content_policy(&source, generated);
    assert_eq!(governed.candidates.len(), lines.len());
    for (candidate, line) in governed.candidates.iter().zip(&lines) {
        assert_eq!(&candidate.answer, line);
        let evidence = candidate.evidence.as_deref().expect("source quotation");
        assert!(evidence.len() <= 8_000);
        assert!(memory_engine_generation::evidence_quote_matches(
            &body, evidence
        ));
        assert!(memory_engine_generation::answer_has_evidence_support(
            evidence, line
        ));
    }
}

#[test]
fn numbered_procedures_keep_process_semantics_instead_of_set_cards() {
    let source = source_document(
        "src-procedure-list",
        "Three-step recipe",
        "1. Mix flour and water.\n2. Knead the dough.\n3. Bake the loaf.",
    );

    assert_eq!(
        classify_learning_intent(&source).intent,
        LearningIntent::ProcedureProcess
    );
    let drafts = FakeModelProvider
        .generate_drafts(&source)
        .expect("procedure generation");
    assert_eq!(
        drafts.learning_intent,
        Some(LearningIntent::ProcedureProcess)
    );
    assert!(drafts
        .candidates
        .iter()
        .all(|candidate| !candidate.question.contains("entry for number")));
    assert!(drafts
        .candidates
        .iter()
        .any(|candidate| candidate.activity_stage == "procedure-composition"));
}

#[test]
fn numbered_imperative_procedures_keep_process_semantics_without_metadata() {
    let source = source_document(
        "src-imperative-procedure",
        "Fried eggs",
        "1. Heat the pan\n2. Add the oil\n3. Cook the eggs",
    );

    assert_eq!(
        classify_learning_intent(&source).intent,
        LearningIntent::ProcedureProcess
    );
    let drafts = FakeModelProvider
        .generate_drafts(&source)
        .expect("imperative procedure generation");
    assert_eq!(
        drafts.learning_intent,
        Some(LearningIntent::ProcedureProcess)
    );
    assert!(drafts
        .candidates
        .iter()
        .any(|candidate| candidate.activity_stage == "procedure-composition"));
}

#[test]
fn non_list_process_prose_stays_bounded_instead_of_one_card_per_sentence() {
    // Regression guard for a real PR46 defect: the non-list ProcedureProcess
    // fallback was rewritten to emit one candidate per `split_sentences()`
    // sentence, silently reintroducing exhaustive-coverage behavior into
    // ordinary prose (e.g. the `spacing-effect` bench fixture grew from 2
    // accepted drafts to 11, blowing through its declared `max_drafts: 8`).
    // Conceptual/procedure prose must stay on a small, bounded fewer-better
    // fallback regardless of source length; only numbered lists (handled by
    // the list_entries branch above) and verbatim/enumerable sources get
    // exhaustive per-unit coverage.
    let source = source_document(
        "src-spacing-effect-like",
        "Spaced review process",
        "Spaced practice is a process that improves retention over many separate study \
         sessions. Each review session should be spaced further apart as recall becomes more \
         reliable. First you study the material once. Then you wait a day and try to recall it \
         without notes. If you succeed, the next review interval doubles. If you fail, the \
         interval resets to the beginning. Always review right before you would otherwise \
         forget, not before or after. This process continues indefinitely as long as the \
         material stays relevant. Finally, keep track of your review history so you can audit \
         the schedule.",
    );

    assert_eq!(
        classify_learning_intent(&source).intent,
        LearningIntent::ProcedureProcess
    );

    let sentence_count = source
        .body
        .as_deref()
        .unwrap_or_default()
        .split(['.', '?', '!'])
        .filter(|sentence| sentence.split_whitespace().count() >= 3)
        .count();
    assert!(
        sentence_count > 3,
        "fixture must outnumber the fewer-better cap to be a meaningful regression guard: \
         {sentence_count} sentences"
    );

    let drafts = FakeModelProvider
        .generate_drafts(&source)
        .expect("non-list procedure generation");
    assert_eq!(
        drafts.learning_intent,
        Some(LearningIntent::ProcedureProcess)
    );
    assert!(
        drafts.candidates.len() <= 3,
        "non-list process prose must stay bounded by the fewer-better fallback cap, not scale \
         with sentence count: got {} candidates from {sentence_count} sentences",
        drafts.candidates.len()
    );
    assert!(
        drafts.candidates.len() < sentence_count,
        "candidate count must not track one-per-sentence: {} candidates for {sentence_count} \
         sentences",
        drafts.candidates.len()
    );
}

#[test]
fn explicit_poem_intent_outranks_generic_list_shape() {
    let source = source_document(
        "src-oath-list",
        "Recite this oath",
        "- We stand together.\n- We keep our word.\n- We serve with care.",
    );

    assert_eq!(
        classify_learning_intent(&source).intent,
        LearningIntent::VerbatimMemorization
    );
    let drafts = FakeModelProvider
        .generate_drafts(&source)
        .expect("oath generation");
    assert_eq!(
        drafts.learning_intent,
        Some(LearningIntent::VerbatimMemorization)
    );
    assert_eq!(
        drafts
            .candidates
            .iter()
            .map(|candidate| candidate.answer.as_str())
            .collect::<Vec<_>>(),
        [
            "- We stand together.",
            "- We keep our word.",
            "- We serve with care."
        ]
    );
    assert!(drafts
        .candidates
        .iter()
        .all(|candidate| candidate.activity_kind == GeneratedLearningActivityKind::Exercise));
}

#[test]
fn finite_sets_with_weak_process_words_keep_enumerable_semantics() {
    let source = source_document(
        "src-planets",
        "The first three planets",
        "1. Mercury\n2. Venus\n3. Earth",
    );

    assert_eq!(
        classify_learning_intent(&source).intent,
        LearningIntent::EnumerableSet
    );
    let drafts = FakeModelProvider
        .generate_drafts(&source)
        .expect("finite-set generation");
    assert_eq!(drafts.learning_intent, Some(LearningIntent::EnumerableSet));
    assert_eq!(drafts.candidates.len(), 3);
    assert!(drafts
        .candidates
        .iter()
        .all(|candidate| candidate.activity_stage == "production-recall"));
}

#[test]
fn two_entry_finite_mappings_receive_exhaustive_enumerable_coverage() {
    let source = source_document(
        "src-binary-toggle",
        "Binary toggle states",
        "0 is off. 1 is on.",
    );

    assert_eq!(
        classify_learning_intent(&source).intent,
        LearningIntent::EnumerableSet,
        "a two-entry key-to-value mapping is still a finite, non-derivable set"
    );
    let drafts = FakeModelProvider
        .generate_drafts(&source)
        .expect("two-entry mapping generation");
    assert_eq!(drafts.learning_intent, Some(LearningIntent::EnumerableSet));
    assert_eq!(drafts.candidates.len(), 2);
    assert_eq!(
        drafts
            .candidates
            .iter()
            .map(|candidate| candidate.answer.as_str())
            .collect::<Vec<_>>(),
        ["off", "on"]
    );
}

#[test]
fn conceptual_prose_with_collision_substrings_is_not_misclassified_as_verbatim() {
    // "universe" and "diverse" contain "verse"; "quoted" contains "quote".
    // A naive substring check on the verbatim keyword gate would wrongly
    // force recitation cards onto this ordinary conceptual paragraph.
    let source = source_document(
        "src-universe-concept",
        "Diverse views of the universe",
        "The universe contains diverse galaxies. Philosophers argue that quoted prose about \
         physics helps students grasp the underlying principle without needing memorization.",
    );

    assert_eq!(
        classify_learning_intent(&source).intent,
        LearningIntent::ConceptUnderstanding,
        "substrings like universe/diverse/quoted must not trip the verbatim keyword gate"
    );

    let drafts = FakeModelProvider
        .generate_drafts(&source)
        .expect("conceptual generation");
    assert_ne!(
        drafts.learning_intent,
        Some(LearningIntent::VerbatimMemorization)
    );
    assert!(drafts
        .candidates
        .iter()
        .all(|candidate| candidate.activity_kind != GeneratedLearningActivityKind::Exercise));
}

#[test]
fn sequential_sources_emit_one_verbatim_card_per_sentence() {
    let source = source_document(
        "src-sequence",
        "A quoted oath excerpt",
        "First faithful line. Second faithful line. Third faithful line.",
    );

    assert_eq!(
        classify_learning_intent(&source).intent,
        LearningIntent::VerbatimMemorization
    );
    let drafts = FakeModelProvider
        .generate_drafts(&source)
        .expect("sequential generation");

    assert_eq!(drafts.candidates.len(), 3);
    assert_eq!(
        drafts
            .candidates
            .iter()
            .map(|candidate| candidate.answer.as_str())
            .collect::<Vec<_>>(),
        [
            "First faithful line.",
            "Second faithful line.",
            "Third faithful line."
        ]
    );
    assert!(drafts.candidates.iter().all(|candidate| {
        candidate.activity_kind == GeneratedLearningActivityKind::Exercise
            && candidate.worked_solution.is_some()
            && candidate.evidence.as_deref().is_some_and(|evidence| {
                memory_engine_generation::evidence_quote_matches(
                    source.body.as_deref().expect("source body"),
                    evidence,
                ) && memory_engine_generation::answer_has_evidence_support(
                    evidence,
                    &candidate.answer,
                )
            })
    }));
    assert_eq!(drafts.candidates[0].activity_stage, "free-recall");
    assert!(drafts.candidates[1..]
        .iter()
        .all(|candidate| candidate.activity_stage == "cued-recall"));
    assert!(drafts.candidates[1]
        .question
        .contains("First faithful line."));
}

#[test]
fn verbatim_intent_persists_recitation_prompt_ladder() {
    let directory = TempDirectory::new("verbatim-recitation");
    let path = directory.path().join("store.json");
    let mut store = BetaPersistenceStore::open(&path).expect("store");
    store
        .save_source_document(source_document(
            "src-poem",
            "Emily Dickinson poem 314",
            "\"Hope\" is the thing with feathers -\nThat perches in the soul -\nAnd sings the tune without the words -\nAnd never stops - at all -",
        ))
        .expect("source");

    let result = run_beta_generation_with_provider(
        &mut store,
        &FakeModelProvider,
        request("run-poem", "src-poem"),
    )
    .expect("generation");

    assert_eq!(result.rejected_draft_ids, Vec::<String>::new());
    let snapshot = store.snapshot();
    let stages = snapshot
        .generated_prompt_drafts
        .iter()
        .map(|draft| draft.activity_stage.as_str())
        .collect::<std::collections::BTreeSet<_>>();
    assert!(
        stages.contains("cued-recall"),
        "missing cued stage: {stages:?}"
    );
    assert!(
        stages.contains("free-recall"),
        "missing free stage: {stages:?}"
    );
    assert!(
        snapshot.generated_prompt_drafts.iter().all(|draft| {
            draft.activity_kind == GeneratedLearningActivityKind::Exercise
                && matches!(
                    &draft.prompt,
                    Prompt::Exact(exact) if exact.kind == ExactPromptKind::Recitation
                )
        }),
        "verbatim sources must not become MC trivia: {:?}",
        snapshot.generated_prompt_drafts
    );

    let free_recall = snapshot
        .generated_prompt_drafts
        .iter()
        .find(|draft| draft.activity_stage == "free-recall")
        .expect("free-recall draft");
    let exact_prompt = match &free_recall.prompt {
        Prompt::Exact(prompt) => prompt,
        other => panic!("unexpected verbatim prompt: {other:?}"),
    };
    let accepted_answer = exact_prompt.accepted_answers[0].clone();
    let grader = Grader::new();
    let context = GradeContext {
        response_time_ms: 5_100,
        prior_reps: 0,
    };
    assert_eq!(
        grader
            .grade(&free_recall.prompt, &accepted_answer, context)
            .verdict,
        Verdict::Correct
    );
    for (label, submission) in [
        ("case", accepted_answer.to_lowercase()),
        ("punctuation", format!("{accepted_answer}!")),
        ("word", accepted_answer.replacen("thing", "things", 1)),
    ] {
        assert_ne!(
            grader
                .grade(&free_recall.prompt, &submission, context)
                .verdict,
            Verdict::Correct,
            "{label} deviation must not be correct"
        );
    }
}

#[test]
fn recitation_cues_do_not_repeat_the_target_line() {
    let directory = TempDirectory::new("recitation-cue-collision");
    let mut store = BetaPersistenceStore::open(directory.path().join("store.json")).expect("store");
    store
        .save_source_document(source_document(
            "src-refrain",
            "Sing the river home",
            "Sing the river home.\nSing the river home.\nLet the oars grow still.",
        ))
        .expect("source");

    let result = run_beta_generation_with_provider(
        &mut store,
        &FakeModelProvider,
        request("run-refrain", "src-refrain"),
    )
    .expect("recitation generation");

    assert_eq!(result.accepted_draft_ids.len(), 3);
    assert!(result.rejected_draft_ids.is_empty());
    let snapshot = store.snapshot();
    assert_eq!(
        snapshot
            .generated_prompt_drafts
            .iter()
            .map(|draft| match &draft.prompt {
                Prompt::Exact(exact) if exact.kind == ExactPromptKind::Recitation => {
                    exact.accepted_answers[0].as_str()
                }
                other => panic!("expected exact recitation, got {other:?}"),
            })
            .collect::<Vec<_>>(),
        [
            "Sing the river home.",
            "Sing the river home.",
            "Let the oars grow still.",
        ]
    );
    assert!(snapshot.generated_prompt_drafts[..2]
        .iter()
        .all(|draft| draft.activity_stage == "cued-recall"));
    assert!(
        snapshot.review_units.is_empty(),
        "generation is not learner approval"
    );
}

#[test]
fn charged_provider_failure_is_recorded_without_fake_success_or_lost_usage() {
    struct FailingProvider;
    impl DraftProvider for FailingProvider {
        fn model(&self) -> GeneratedPromptModel {
            test_model("failing-model")
        }
        fn generate_drafts(
            &self,
            _source: &SourceDocument,
        ) -> Result<ProviderDrafts, ProviderFailure> {
            Err(
                ProviderFailure::new("The paid response was truncated.").with_usage(Some(
                    ProviderUsage {
                        input_tokens: 100,
                        output_tokens: 20,
                        cost_usd_micros: Some(45),
                        latency_ms: 90,
                    },
                )),
            )
        }
    }
    let directory = TempDirectory::new("charged-failure");
    let mut store = open_store_with_prose(&directory);
    let result = run_beta_generation_with_provider(
        &mut store,
        &FailingProvider,
        request("run-fail", "src-prose"),
    )
    .expect("failed response still produces a receipt");
    assert!(result.draft_ids.is_empty());
    let snapshot = store.snapshot();
    let run = &snapshot.generation_runs[0];
    assert!(run.completed_at.is_some());
    let usage = run.usage.as_ref().expect("failure accounting");
    assert_eq!(
        (
            usage.input_tokens,
            usage.output_tokens,
            usage.cost_usd_micros
        ),
        (100, 20, Some(45))
    );
}

#[test]
fn evidence_quote_not_found_in_source_is_rejected() {
    struct FabricatingProvider;

    impl DraftProvider for FabricatingProvider {
        fn model(&self) -> GeneratedPromptModel {
            test_model("fabricating-model")
        }

        fn generate_drafts(
            &self,
            _source: &SourceDocument,
        ) -> Result<ProviderDrafts, ProviderFailure> {
            Ok(ProviderDrafts {
                model: self.model(),
                learning_intent: Some(LearningIntent::FactRecall),
                candidates: vec![DraftCandidate {
                    index: 1,
                    concept: "Mitochondrial DNA".to_owned(),
                    question: "What shape is mitochondrial DNA?".to_owned(),
                    answer: "Circular".to_owned(),
                    evidence: Some("Mitochondrial DNA is circular.".to_owned()),
                    distractors: Vec::new(),
                    worked_solution: None,
                    activity_kind: GeneratedLearningActivityKind::Quiz,
                    activity_stage: "recognition".to_owned(),
                    unsupported: false,
                }],
                failures: Vec::new(),
                usage: None,
            })
        }
    }

    let directory = TempDirectory::new("fabricating-provider");
    let mut store = open_store_with_prose(&directory);

    let result = run_beta_generation_with_provider(
        &mut store,
        &FabricatingProvider,
        request("run-fabricated", "src-prose"),
    )
    .expect("generation");

    assert_eq!(result.rejected_draft_ids.len(), 1);
    assert!(result.accepted_draft_ids.is_empty());
    let drafts = store.snapshot().generated_prompt_drafts;
    assert_eq!(
        drafts[0].validation.status,
        GeneratedPromptValidationStatus::Rejected
    );
    assert_eq!(
        drafts[0].validation.reasons,
        ["Evidence quote not found in cited source"]
    );
}

#[test]
fn provider_usage_is_aggregated_onto_the_generation_run() {
    struct MeteredProvider;

    impl DraftProvider for MeteredProvider {
        fn model(&self) -> GeneratedPromptModel {
            test_model("metered-model")
        }

        fn generate_drafts(
            &self,
            source: &SourceDocument,
        ) -> Result<ProviderDrafts, ProviderFailure> {
            let body = source.body.clone().unwrap_or_default();
            let sentence = body.split('.').next().unwrap_or_default().trim().to_owned();
            Ok(ProviderDrafts {
                model: self.model(),
                learning_intent: Some(LearningIntent::FactRecall),
                candidates: vec![DraftCandidate {
                    index: 1,
                    concept: "Mitochondria energy".to_owned(),
                    question: "What do mitochondria generate?".to_owned(),
                    answer: "Most of the cell's supply of adenosine triphosphate".to_owned(),
                    evidence: Some(sentence),
                    distractors: Vec::new(),
                    worked_solution: None,
                    activity_kind: GeneratedLearningActivityKind::Quiz,
                    activity_stage: "recognition".to_owned(),
                    unsupported: false,
                }],
                failures: Vec::new(),
                usage: Some(ProviderUsage {
                    input_tokens: 1_200,
                    output_tokens: 300,
                    cost_usd_micros: Some(480),
                    latency_ms: 2_000,
                }),
            })
        }
    }

    let directory = TempDirectory::new("metered-provider");
    let mut store = open_store_with_prose(&directory);

    let result = run_beta_generation_with_provider(
        &mut store,
        &MeteredProvider,
        request("run-metered", "src-prose"),
    )
    .expect("generation");

    assert_eq!(result.accepted_draft_ids.len(), 1);
    assert_eq!(
        store.snapshot().generation_runs[0].usage,
        Some(GenerationRunUsage {
            input_tokens: 1_200,
            output_tokens: 300,
            cost_usd_micros: Some(480),
            latency_ms: 2_000,
        })
    );
}

#[test]
fn structured_duplicate_repair_never_calls_the_model_fallback() {
    struct ModelFallback;

    impl DraftProvider for ModelFallback {
        fn model(&self) -> GeneratedPromptModel {
            GeneratedPromptModel {
                provider: "model".to_owned(),
                name: "must-not-run".to_owned(),
                version: "v1".to_owned(),
            }
        }

        fn generate_drafts(
            &self,
            _source: &SourceDocument,
        ) -> Result<ProviderDrafts, ProviderFailure> {
            panic!("structured generation must not call the model fallback")
        }

        fn repair_drafts(
            &self,
            _source: &SourceDocument,
            _rejections: &[DraftRejection],
        ) -> Result<Option<ProviderDrafts>, ProviderFailure> {
            panic!("structured duplicate repair must not call the model fallback")
        }
    }

    let directory = TempDirectory::new("structured-duplicate-repair");
    let mut store = open_store_with_prose(&directory);
    store
        .save_source_document(source_document(
            "src-structured",
            "Stable generation",
            "Concept: Stable generation\nQuestion: What stays stable?\nAnswer: The job identity.",
        ))
        .expect("structured source");
    let provider = FallbackProvider::new(&ModelFallback);

    let first = run_beta_generation_with_provider(
        &mut store,
        &provider,
        request("run-structured-1", "src-structured"),
    )
    .expect("first structured generation");
    assert_eq!(first.accepted_draft_ids.len(), 1);

    let replay = run_beta_generation_with_provider(
        &mut store,
        &provider,
        request("run-structured-2", "src-structured"),
    )
    .expect("replayed structured generation");
    assert!(
        replay.accepted_draft_ids.is_empty(),
        "the duplicate must stay rejected instead of escaping through model repair"
    );
}

#[test]
fn prose_repair_stays_with_the_model_fallback() {
    struct RepairingFallback {
        repaired: Cell<bool>,
    }

    impl DraftProvider for RepairingFallback {
        fn model(&self) -> GeneratedPromptModel {
            test_model("repairing-fallback")
        }

        fn generate_drafts(
            &self,
            _source: &SourceDocument,
        ) -> Result<ProviderDrafts, ProviderFailure> {
            Ok(ProviderDrafts {
                model: self.model(),
                learning_intent: Some(LearningIntent::FactRecall),
                candidates: vec![DraftCandidate {
                    index: 1,
                    concept: "Mitochondria energy".to_owned(),
                    question: "What do mitochondria generate?".to_owned(),
                    answer: "ATP".to_owned(),
                    evidence: Some("fabricated evidence".to_owned()),
                    distractors: Vec::new(),
                    worked_solution: None,
                    activity_kind: GeneratedLearningActivityKind::Quiz,
                    activity_stage: "recognition".to_owned(),
                    unsupported: false,
                }],
                failures: Vec::new(),
                usage: None,
            })
        }

        fn repair_drafts(
            &self,
            source: &SourceDocument,
            _rejections: &[DraftRejection],
        ) -> Result<Option<ProviderDrafts>, ProviderFailure> {
            self.repaired.set(true);
            FakeModelProvider.generate_drafts(source).map(Some)
        }
    }

    let directory = TempDirectory::new("prose-fallback-repair");
    let mut store = open_store_with_prose(&directory);
    let fallback = RepairingFallback {
        repaired: Cell::new(false),
    };
    let provider = FallbackProvider::new(&fallback);

    let result = run_beta_generation_with_provider(
        &mut store,
        &provider,
        request("run-prose-repair", "src-prose"),
    )
    .expect("prose repair");

    assert!(
        fallback.repaired.get(),
        "fallback repair must run for prose"
    );
    assert!(
        !result.accepted_draft_ids.is_empty(),
        "the fallback's repaired draft must pass the shared gate: {:?}",
        result.validation_failures
    );
}

#[test]
fn verbatim_repair_reapplies_policy_and_recitation_intent() {
    struct RepairingVerbatim {
        repaired: Cell<bool>,
    }

    impl DraftProvider for RepairingVerbatim {
        fn model(&self) -> GeneratedPromptModel {
            test_model("repairing-verbatim")
        }

        fn generate_drafts(
            &self,
            _source: &SourceDocument,
        ) -> Result<ProviderDrafts, ProviderFailure> {
            Ok(ProviderDrafts {
                model: self.model(),
                learning_intent: None,
                candidates: vec![DraftCandidate {
                    index: 1,
                    concept: "wrong provider shape".to_owned(),
                    question: "What should be repaired?".to_owned(),
                    answer: "the exact source".to_owned(),
                    evidence: Some("fabricated evidence".to_owned()),
                    distractors: Vec::new(),
                    worked_solution: None,
                    activity_kind: GeneratedLearningActivityKind::Quiz,
                    activity_stage: "recognition".to_owned(),
                    unsupported: false,
                }],
                failures: Vec::new(),
                usage: None,
            })
        }

        fn repair_drafts(
            &self,
            _source: &SourceDocument,
            _rejections: &[DraftRejection],
        ) -> Result<Option<ProviderDrafts>, ProviderFailure> {
            self.repaired.set(true);
            Ok(Some(ProviderDrafts {
                model: self.model(),
                learning_intent: None,
                candidates: vec![DraftCandidate {
                    index: 1,
                    concept: "repair shape".to_owned(),
                    question: "Recite the repaired line exactly.".to_owned(),
                    answer: "Alpha line.".to_owned(),
                    evidence: Some("Alpha line.".to_owned()),
                    distractors: Vec::new(),
                    worked_solution: Some("The exact source line is: Alpha line.".to_owned()),
                    activity_kind: GeneratedLearningActivityKind::Exercise,
                    activity_stage: "free-recall".to_owned(),
                    unsupported: false,
                }],
                failures: Vec::new(),
                usage: None,
            }))
        }
    }

    let directory = TempDirectory::new("verbatim-repair-policy");
    let mut store = BetaPersistenceStore::open(directory.path().join("store.json")).expect("store");
    store
        .save_source_document(source_document(
            "src-verbatim-repair",
            "Source text verbatim",
            "Alpha line.\nBeta line.\nGamma line.",
        ))
        .expect("source");
    let provider = RepairingVerbatim {
        repaired: Cell::new(false),
    };

    let result = run_beta_generation_with_provider(
        &mut store,
        &provider,
        request("run-verbatim-repair", "src-verbatim-repair"),
    )
    .expect("generation");

    assert!(
        provider.repaired.get(),
        "verbatim rejection must enter repair"
    );
    assert_eq!(result.accepted_draft_ids.len(), 2);
    let drafts = store.snapshot().generated_prompt_drafts;
    assert_eq!(
        drafts.len(),
        4,
        "three policy drafts plus one repaired draft"
    );
    assert!(
        drafts.iter().all(|draft| {
            matches!(
                &draft.prompt,
                Prompt::Exact(exact) if exact.kind == ExactPromptKind::Recitation
            )
        }),
        "initial and repaired verbatim drafts must retain Recitation intent"
    );
}

#[test]
fn an_irrelevant_real_quote_cannot_support_an_invented_answer() {
    struct MisattributingProvider;
    impl DraftProvider for MisattributingProvider {
        fn model(&self) -> GeneratedPromptModel {
            test_model("misattributing")
        }
        fn generate_drafts(
            &self,
            source: &SourceDocument,
        ) -> Result<ProviderDrafts, ProviderFailure> {
            Ok(ProviderDrafts {
                model: self.model(),
                learning_intent: Some(LearningIntent::ConceptUnderstanding),
                candidates: vec![DraftCandidate {
                    index: 1,
                    concept: "Mitochondrial discovery".into(),
                    question: "Who first discovered mitochondria?".into(),
                    answer: "Napoleon Bonaparte".into(),
                    evidence: source.body.clone(),
                    distractors: Vec::new(),
                    worked_solution: None,
                    activity_kind: GeneratedLearningActivityKind::Quiz,
                    activity_stage: "cued-recall".into(),
                    unsupported: false,
                }],
                failures: Vec::new(),
                usage: None,
            })
        }
    }
    let directory = TempDirectory::new("unrelated-real-quote");
    let mut store = open_store_with_prose(&directory);
    let result = run_beta_generation_with_provider(
        &mut store,
        &MisattributingProvider,
        request("run-unsupported", "src-prose"),
    )
    .expect("rejected material receipt");
    assert!(result.accepted_draft_ids.is_empty());
    assert_eq!(result.rejected_draft_ids.len(), 1);
}

#[test]
fn failed_repair_usage_is_aggregated_and_unknown_cost_is_not_treated_as_zero() {
    struct RepairFailure;
    impl DraftProvider for RepairFailure {
        fn model(&self) -> GeneratedPromptModel {
            test_model("charged-repair")
        }
        fn generate_drafts(
            &self,
            _source: &SourceDocument,
        ) -> Result<ProviderDrafts, ProviderFailure> {
            Ok(ProviderDrafts {
                model: self.model(),
                learning_intent: Some(LearningIntent::FactRecall),
                candidates: vec![DraftCandidate {
                    index: 1,
                    concept: "Mitochondrial DNA".into(),
                    question: "What shape is mitochondrial DNA?".into(),
                    answer: "circular".into(),
                    evidence: Some("This quote never appeared in the source.".into()),
                    distractors: Vec::new(),
                    worked_solution: None,
                    activity_kind: GeneratedLearningActivityKind::Quiz,
                    activity_stage: "cued-recall".into(),
                    unsupported: false,
                }],
                failures: Vec::new(),
                usage: Some(ProviderUsage {
                    input_tokens: 100,
                    output_tokens: 10,
                    cost_usd_micros: Some(40),
                    latency_ms: 10,
                }),
            })
        }
        fn repair_drafts(
            &self,
            _source: &SourceDocument,
            _rejections: &[DraftRejection],
        ) -> Result<Option<ProviderDrafts>, ProviderFailure> {
            Err(
                ProviderFailure::new("The repair response was incomplete.").with_usage(Some(
                    ProviderUsage {
                        input_tokens: 20,
                        output_tokens: 2,
                        cost_usd_micros: None,
                        latency_ms: 5,
                    },
                )),
            )
        }
    }
    let directory = TempDirectory::new("paid-failed-repair");
    let mut store = open_store_with_prose(&directory);
    let result = run_beta_generation_with_provider(
        &mut store,
        &RepairFailure,
        request("run-paid-repair", "src-prose"),
    )
    .expect("receipt");
    assert!(result.accepted_draft_ids.is_empty());
    let usage = store.snapshot().generation_runs[0]
        .usage
        .clone()
        .expect("combined usage");
    assert_eq!(
        (usage.input_tokens, usage.output_tokens, usage.latency_ms),
        (120, 12, 15)
    );
    assert_eq!(usage.cost_usd_micros, None);
}

#[test]
fn fallback_stamps_drafts_with_the_provider_that_actually_ran() {
    let directory = TempDirectory::new("fallback-attribution");
    let mut store = open_store_with_prose(&directory);
    // A structured source the primary parser handles without the fallback.
    store
        .save_source_document(SourceDocument {
            id: "src-structured".to_owned(),
            kind: SourceDocumentKind::Text,
            title: "Structured".to_owned(),
            project_key: None,
            body: Some(
                [
                    "Concept: Mitochondria role",
                    "Question: What do mitochondria generate?",
                    "Answer: ATP",
                    "Reference: Mitochondria are organelles that generate most of the cell's supply of adenosine triphosphate.",
                ]
                .join("\n"),
            ),
            uri: None,
            permission: SourcePermission::ModelEligible,
            freshness: Some(NOW),
            ttl_expires_at: None,
            created_at: NOW,
            archived_at: None,
        })
        .expect("structured source");

    let model = FakeModelProvider;
    let provider = FallbackProvider::new(&model);

    // Prose: primary finds nothing, fallback (fake model) runs.
    run_beta_generation_with_provider(&mut store, &provider, request("run-prose", "src-prose"))
        .expect("prose generation");
    // Structured: primary handles it, fallback never runs.
    run_beta_generation_with_provider(
        &mut store,
        &provider,
        request("run-structured", "src-structured"),
    )
    .expect("structured generation");

    let snapshot = store.snapshot();
    let prose_run = snapshot
        .generation_runs
        .iter()
        .find(|run| run.id == "run-prose")
        .expect("prose run");
    let structured_run = snapshot
        .generation_runs
        .iter()
        .find(|run| run.id == "run-structured")
        .expect("structured run");

    assert_eq!(prose_run.model, "fake-model");
    assert_eq!(
        structured_run.model, "deterministic-beta-generator",
        "structured drafts must be attributed to the parser that made them, not the fallback model"
    );
}

fn request(run_id: &str, source_id: &str) -> BetaGenerationRequest {
    BetaGenerationRequest {
        run_id: run_id.to_owned(),
        source_document_ids: vec![source_id.to_owned()],
        parent_review_unit_id: None,
        started_at: NOW,
        completed_at: Some(NOW + 1_000),
        default_due: NOW - 60_000,
        model: None,
        pending: false,
    }
}

fn test_model(name: &str) -> GeneratedPromptModel {
    GeneratedPromptModel {
        provider: "test".to_owned(),
        name: name.to_owned(),
        version: "v1".to_owned(),
    }
}

fn open_store_with_prose(directory: &TempDirectory) -> BetaPersistenceStore {
    let path = directory.path().join("store.json");
    let mut store = BetaPersistenceStore::open(&path).expect("store");
    store
        .save_source_document(SourceDocument {
            id: "src-prose".to_owned(),
            kind: SourceDocumentKind::Text,
            title: "Mitochondria notes".to_owned(),
            project_key: None,
            body: Some(PROSE.to_owned()),
            uri: None,
            permission: SourcePermission::ModelEligible,
            freshness: Some(NOW),
            ttl_expires_at: None,
            created_at: NOW,
            archived_at: None,
        })
        .expect("source");

    store
}

fn source_document(id: &str, title: &str, body: &str) -> SourceDocument {
    SourceDocument {
        id: id.to_owned(),
        kind: SourceDocumentKind::Text,
        title: title.to_owned(),
        project_key: None,
        body: Some(body.to_owned()),
        uri: None,
        permission: SourcePermission::ModelEligible,
        freshness: Some(NOW),
        ttl_expires_at: None,
        created_at: NOW,
        archived_at: None,
    }
}

struct TempDirectory {
    path: PathBuf,
}

impl TempDirectory {
    fn new(label: &str) -> Self {
        let stamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .expect("system time")
            .as_nanos();
        let path = std::env::temp_dir().join(format!(
            "memory-engine-provider-{label}-{}-{stamp}",
            std::process::id()
        ));
        fs::create_dir_all(&path).expect("create temp dir");

        Self { path }
    }

    fn path(&self) -> &std::path::Path {
        &self.path
    }
}

impl Drop for TempDirectory {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.path);
    }
}
