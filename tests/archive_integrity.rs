//! Restore accepts the earlier concept notebook only as it was stored. The
//! states come from `tests/fixtures/earlier-notebook.json`: no route can create
//! them any more.
mod common;

use scry::{engine, model::*, persistence};

const NOW: i64 = 1_800_000_000_000;

/// The demo's choice question held unanswered; no answer recorded.
fn fixture() -> App {
    common::earlier("question-cold-choice").0
}

/// Three graded answers, the last an unaided exact recall held as the result.
fn reviewed() -> App {
    common::earlier("result-held").0
}

fn corrupt(mut app: App, mutation: impl FnOnce(&mut App)) {
    // The stored notebook itself is a complete archive, so a refusal below is
    // the tampering's.
    persistence::Archive::new(app.clone(), vec![], NOW)
        .encode()
        .unwrap();
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
    engine::validate_app(&reviewed()).unwrap();
    // Jev's rejection, then the Owner's Correction; then the question gains a
    // version.
    let mut app = common::earlier("result-corrected").0;
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
    // An exact answer given after help is held at rating 0.
    let warm = common::earlier("result-helped").0;
    assert_eq!(warm.events.last().unwrap().result.rating, 0);
    engine::validate_app(&warm).unwrap();
    // A non-exact answer whose check was sent, with its reservation.
    let mut checking = common::earlier("checking-sent").0;
    engine::validate_app(&checking).unwrap();
    // A restore keeps the sent check and its held answer exactly as saved.
    let sent = checking.clone();
    engine::pause_restored(&mut checking, NOW + 1).unwrap();
    engine::validate_app(&checking).unwrap();
    assert_eq!(checking.assessments, sent.assessments);
    assert_eq!(checking.occurrence, sent.occurrence);
    assert_eq!(checking.spend, sent.spend);
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
    let app = common::earlier("result-corrected").0;
    assert_eq!(app.overrides.len(), 1);
    corrupt(app.clone(), |a| a.overrides[0].after_card.stability += 1.0);
    corrupt(app.clone(), |a| a.overrides[0].before_card.reps += 1);
    corrupt(app.clone(), |a| {
        a.overrides[0].correct = !a.overrides[0].correct
    });
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
    // A non-exact answer whose check was sent: its lease and reservation.
    let app = common::earlier("checking-sent").0;
    assert_eq!(app.spend.len(), 1);
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

#[test]
fn restore_advances_source_revision_once_and_refuses_exhaustion_before_mutation() {
    let mut app = reviewed();
    let before = app.clone();
    engine::pause_restored(&mut app, NOW + 1).unwrap();
    assert_eq!(app.revision, before.revision + 1);
    assert_eq!(app.events, before.events);
    engine::validate_app(&app).unwrap();

    let mut exhausted = before;
    exhausted.revision = u64::MAX;
    let unchanged = exhausted.clone();
    let error = engine::pause_restored(&mut exhausted, NOW + 1).unwrap_err();
    assert_eq!(error.status, 422);
    assert_eq!(error.message, "State revision exhausted.");
    assert_eq!(exhausted, unchanged);
}
