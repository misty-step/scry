use scry::{engine, model::*, persistence};
use std::collections::BTreeMap;

const NOW: i64 = 1_791_410_400_000;

fn fixture() -> App {
    let mut app = App::new("archive-integrity-owner-csrf-32-characters".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    for question in app
        .questions
        .values_mut()
        .filter(|q| q.content().kind == "choice")
    {
        question.card.due_ms = NOW + DAY_MS;
    }
    app.occurrence = None;
    engine::ensure_occurrence(&mut app, NOW, "archive-recall-fixture").unwrap();
    app
}

fn mutate(app: &mut App, path: &str, operation: &str, extra: &[(&str, &str)]) {
    let mut fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), operation.into()),
    ]);
    if let Some(occurrence) = &app.occurrence {
        fields.insert("occurrence_id".into(), occurrence.id.clone());
    }
    for (key, value) in extra {
        fields.insert((*key).into(), (*value).into());
    }
    engine::mutate(app, path, &fields, NOW, operation).unwrap();
}

fn reviewed() -> App {
    let mut app = fixture();
    let answer = app.occurrence.as_ref().unwrap().presentation.answer.clone();
    mutate(
        &mut app,
        "/review/answer",
        "archive-answer-0001",
        &[("answer", &answer)],
    );
    app
}

fn corrupt(mut app: App, mutation: impl FnOnce(&mut App)) {
    mutation(&mut app);
    assert!(engine::validate_app(&app).is_err());
    assert!(
        persistence::Archive::new(app, vec![], NOW)
            .encode()
            .is_err()
    );
}

#[test]
fn valid_history_corrections_warm_attempts_and_paused_work_roundtrip() {
    let mut app = reviewed();
    engine::validate_app(&app).unwrap();
    mutate(
        &mut app,
        "/review/override",
        "archive-override-001",
        &[("correct", "false")],
    );
    engine::validate_app(&app).unwrap();
    let original = app.events[0].clone();
    let question = app.questions.get_mut(&original.question_id).unwrap();
    let mut edited = question.content().clone();
    edited.version += 1;
    edited.prompt = "Which directive gives a cache its freshness lifetime in seconds?".into();
    question.versions.push(edited);
    engine::validate_app(&app).unwrap();
    let bytes = persistence::Archive::new(app.clone(), vec![], NOW)
        .encode()
        .unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        app
    );
    assert_eq!(app.events[0], original);
    let mut warm = fixture();
    mutate(&mut warm, "/review/help", "archive-help-000001", &[]);
    let answer = warm
        .occurrence
        .as_ref()
        .unwrap()
        .presentation
        .answer
        .clone();
    mutate(
        &mut warm,
        "/review/answer",
        "archive-warm-000001",
        &[("answer", &answer)],
    );
    assert_eq!(warm.events[0].result.rating, 0);
    engine::validate_app(&warm).unwrap();
    let mut checking = fixture();
    mutate(
        &mut checking,
        "/review/answer",
        "archive-check-00001",
        &[("answer", "my own explanation of caching")],
    );
    let config = engine::WorkConfig {
        generation: true,
        jev: true,
        critic: false,
        model: "synthetic-generator".into(),
        jev_model: "synthetic-jev".into(),
    };
    engine::claim_work(&mut checking, NOW, "archive-paid-lease-001", &config)
        .unwrap()
        .unwrap();
    engine::validate_app(&checking).unwrap();
    engine::pause_for_restore(&mut checking, NOW + 1);
    engine::validate_app(&checking).unwrap();
}

#[test]
fn historical_schedule_policy_and_assistance_cannot_be_forged() {
    let app = reviewed();
    corrupt(app.clone(), |a| {
        a.events[0].algorithm.push_str(";unknown-policy=true")
    });
    corrupt(app.clone(), |a| a.events[0].after_card.stability += 1.0);
    corrupt(app.clone(), |a| a.events[0].after_card.reps += 1);
    corrupt(app.clone(), |a| a.events[0].result.due_ms += 1);
    corrupt(app.clone(), |a| a.events[0].assisted = true);
    corrupt(app.clone(), |a| {
        a.events[0].result.authority = "invented".into()
    });
    corrupt(app.clone(), |a| {
        a.events[0].presentation.prompt = "A rewritten historical question".into()
    });
    corrupt(app.clone(), |a| {
        a.events[0].concept_id = "missing-concept".into()
    });
    corrupt(app.clone(), |a| {
        a.questions.values_mut().next().unwrap().card.difficulty = -1.0
    });
    corrupt(app.clone(), |a| a.events.push(a.events[0].clone()));
    corrupt(app, |a| {
        a.occurrence
            .as_mut()
            .unwrap()
            .result
            .as_mut()
            .unwrap()
            .outcome = "wrong".into()
    });
}

#[test]
fn corrections_are_unique_eligible_and_replay_the_original_attempt() {
    let mut app = reviewed();
    mutate(
        &mut app,
        "/review/override",
        "archive-override-002",
        &[("correct", "false")],
    );
    corrupt(app.clone(), |a| a.overrides[0].after_card.stability += 1.0);
    corrupt(app.clone(), |a| a.overrides[0].before_card.reps += 1);
    corrupt(app.clone(), |a| a.overrides[0].correct = true);
    corrupt(app.clone(), |a| {
        let mut extra = a.overrides[0].clone();
        extra.id = "second-override".into();
        a.overrides.push(extra);
    });
    corrupt(app, |a| {
        a.overrides[0].event_id = "missing-original-event".into()
    });
}

#[test]
fn broken_references_and_evidence_do_not_become_recovery_authority() {
    let app = fixture();
    corrupt(app.clone(), |a| {
        a.goals
            .values_mut()
            .next()
            .unwrap()
            .concept_ids
            .push("missing-concept".into())
    });
    corrupt(app.clone(), |a| {
        let c = a.concepts.values_mut().next().unwrap();
        c.prerequisites.push(c.id.clone());
    });
    corrupt(app.clone(), |a| {
        let note = &mut a.concepts.values_mut().next().unwrap().notes[0];
        note.basis = "source".into();
        note.quotes = vec!["An invented source quotation".into()];
    });
    corrupt(app.clone(), |a| {
        let q = a.questions.values_mut().next().unwrap();
        q.versions[0].version = 2;
    });
    corrupt(app.clone(), |a| {
        a.occurrence.as_mut().unwrap().phase = "invented-phase".into()
    });
    corrupt(app.clone(), |a| {
        a.jobs.values_mut().next().unwrap().id = "wrong-map-key".into()
    });
    corrupt(app, |a| {
        a.questions.values_mut().next().unwrap().goal_id = "missing-goal".into()
    });
}

#[test]
fn paid_leases_and_saved_checking_answers_have_complete_ownership() {
    let mut app = fixture();
    mutate(
        &mut app,
        "/review/answer",
        "archive-check-00002",
        &[("answer", "an answer in different words")],
    );
    let config = engine::WorkConfig {
        generation: true,
        jev: true,
        critic: false,
        model: "synthetic-generator".into(),
        jev_model: "synthetic-jev".into(),
    };
    engine::claim_work(&mut app, NOW, "archive-paid-lease-002", &config)
        .unwrap()
        .unwrap();
    corrupt(app.clone(), |a| a.spend.clear());
    corrupt(app.clone(), |a| a.spend[0].work_id = "missing-work".into());
    corrupt(app.clone(), |a| a.spend[0].cost_micros = Some(0));
    corrupt(app.clone(), |a| {
        a.assessments.values_mut().next().unwrap().lease = None
    });
    corrupt(app.clone(), |a| {
        a.assessments.values_mut().next().unwrap().policy = "exact-v1".into()
    });
    corrupt(app.clone(), |a| {
        a.assessments.values_mut().next().unwrap().id = "wrong-assessment-key".into()
    });
    corrupt(app, |a| {
        a.occurrence.as_mut().unwrap().answer = Some("a different saved answer".into())
    });
}
