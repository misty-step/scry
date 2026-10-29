//! The focused entry composes existing import, Study and prompt policies atomically.
use scry::{engine, library, model::*, study};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
fn app() -> App {
    App::new("synthetic-nato-focus-csrf-32-chars".into())
}
fn fields(app: &App, op: &str) -> BTreeMap<String, String> {
    BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), op.into()),
        ("revision".into(), app.revision.to_string()),
    ])
}
fn start(app: &mut App, op: &str) -> MutationResult {
    let f = fields(app, op);
    engine::mutate(app, "/practice/nato/start", &f, NOW, op).unwrap()
}
#[test]
fn one_action_imports_starts_and_holds_without_duplicate_effects() {
    let mut a = app();
    let f = fields(&a, "nato-focus-start-0001");
    let result = engine::mutate(&mut a, "/practice/nato/start", &f, NOW, "initial").unwrap();
    assert_eq!(a.library.versions.len(), 1);
    assert_eq!(a.practice.studies.len(), 1);
    assert_eq!(a.practice.cards.len(), 1);
    assert!(a.practice.attempts.is_empty());
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.cue, "A");
    assert!(library::is_nato(
        a.library.versions.values().next().unwrap()
    ));
    engine::validate_app(&a).unwrap();
    let before = a.clone();
    assert_eq!(
        engine::mutate(&mut a, "/practice/nato/start", &f, NOW, "retry").unwrap(),
        result
    );
    assert_eq!(a, before);
    start(&mut a, "nato-focus-start-0002");
    assert_eq!(a.library, before.library);
    assert_eq!(a.practice, before.practice);
}
#[test]
fn start_preserves_graded_result_schedule_and_resumes_retained_study() {
    let mut a = app();
    start(&mut a, "nato-focus-start-0011");
    let mut f = fields(&a, "nato-focus-answer-0011");
    f.insert(
        "prompt_id".into(),
        a.practice.current.as_ref().unwrap().prompt.id.clone(),
    );
    f.insert("answer".into(), "Alfa".into());
    engine::mutate(&mut a, "/practice/answer", &f, NOW, "answer").unwrap();
    let before = a.practice.clone();
    start(&mut a, "nato-focus-start-0012");
    assert_eq!(a.practice, before);
    let sid = a.practice.studies.keys().next().unwrap().clone();
    let mut f = fields(&a, "nato-focus-pause-0011");
    f.insert("study_id".into(), sid.clone());
    engine::mutate(&mut a, "/practice/pause", &f, NOW, "pause").unwrap();
    start(&mut a, "nato-focus-start-0013");
    assert_eq!(a.practice, before);
    engine::validate_app(&a).unwrap();
}
#[test]
fn forbidden_overrides_are_rejected_without_writes() {
    for key in [
        "grade",
        "answer",
        "version_id",
        "study_id",
        "pace",
        "data",
        "after",
    ] {
        let mut a = app();
        let mut f = fields(&a, "nato-focus-invalid-0001");
        f.insert(key.into(), "override".into());
        let before = a.clone();
        assert_eq!(
            engine::mutate(&mut a, "/practice/nato/start", &f, NOW, "invalid")
                .unwrap_err()
                .status,
            422
        );
        assert_eq!(a, before);
    }
}
#[test]
fn different_held_study_is_not_replaced_or_partially_imported() {
    let mut a = app();
    let mut input = library::nato_input().unwrap();
    input.title = "Other supplied mapping".into();
    input.text = "1\tOne\n".into();
    input.sections.as_mut().unwrap()[0].units = vec!["1\tOne\n".into()];
    let vid = a.library.add(input, "other", None).unwrap();
    let mut f = fields(&a, "nato-focus-other-learn");
    f.insert("version_id".into(), vid);
    engine::mutate(&mut a, "/practice/learn", &f, NOW, "other-learn").unwrap();
    let f = fields(&a, "nato-focus-other-next");
    engine::mutate(&mut a, "/practice/next", &f, NOW, "other-next").unwrap();
    let before = a.clone();
    let f = fields(&a, "nato-focus-conflict-001");
    assert_eq!(
        engine::mutate(&mut a, "/practice/nato/start", &f, NOW, "conflict")
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(a, before);
    engine::validate_app(&a).unwrap();
}
#[test]
fn nato_identity_requires_canonical_pairs_and_strict_policy_not_title() {
    let mut a = app();
    start(&mut a, "nato-focus-identity-001");
    let v = a.library.versions.values().next().unwrap();
    assert!(library::is_nato(v));
    let mut v = v.clone();
    v.name = "Edited display name".into();
    assert!(library::is_nato(&v));
    v.spelling_strict = false;
    assert!(!library::is_nato(&v));
    v.spelling_strict = true;
    v.sections[0].units[0].text = "A\tAlpha\n".into();
    assert!(!library::is_nato(&v));
    assert_eq!(
        a.practice.studies.values().next().unwrap().pace,
        study::Pace::Steady
    );
}
#[test]
fn existing_import_and_dropped_study_resume_without_restarting_learning() {
    let mut a = app();
    let vid = a
        .library
        .add(library::nato_input().unwrap(), "agent-import", None)
        .unwrap();
    let mut f = fields(&a, "nato-focus-agent-learn");
    f.insert("version_id".into(), vid.clone());
    f.insert("pace".into(), "light".into());
    engine::mutate(&mut a, "/practice/learn", &f, NOW, "agent-learn").unwrap();
    start(&mut a, "nato-focus-agent-start");
    let sid = a.practice.studies.keys().next().unwrap().clone();
    let mut f = fields(&a, "nato-focus-agent-drop");
    f.insert("study_id".into(), sid.clone());
    engine::mutate(&mut a, "/practice/drop", &f, NOW, "drop").unwrap();
    let cards = a.practice.cards.clone();
    let library = a.library.clone();
    start(&mut a, "nato-focus-agent-resume");
    assert_eq!(a.library, library);
    assert_eq!(a.practice.cards, cards);
    assert_eq!(a.practice.studies.len(), 1);
    assert_eq!(a.practice.studies[&sid].pace, study::Pace::Light);
    assert!(a.practice.studies[&sid].nato_all_letters);
    assert_eq!(a.practice.studies[&sid].status, study::Status::Active);
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.study_id, sid);
    engine::validate_app(&a).unwrap();
}
#[test]
fn deliberate_nato_entry_allows_all_26_without_changing_general_paces() {
    let mut a = app();
    start(&mut a, "nato-focus-full-start");
    for index in 0..26 {
        let p = a.practice.current.as_ref().unwrap().prompt.clone();
        assert_eq!(p.cue, char::from(b'A' + index).to_string());
        let v = &a.library.versions[&p.version_id];
        let u = v
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .find(|u| p.unit_ids.contains(&u.id))
            .unwrap();
        let answer = library::mapping_pair(&u.text).unwrap().1.to_string();
        let mut f = fields(&a, &format!("nato-focus-full-answer-{index:02}"));
        f.insert("prompt_id".into(), p.id.clone());
        f.insert("answer".into(), answer);
        engine::mutate(
            &mut a,
            "/practice/answer",
            &f,
            NOW,
            &format!("answer-{index}"),
        )
        .unwrap();
        let mut f = fields(&a, &format!("nato-focus-full-next-{index:02}"));
        f.insert("after".into(), p.id);
        f.insert("study_id".into(), p.study_id);
        engine::mutate(&mut a, "/practice/next", &f, NOW, &format!("next-{index}")).unwrap();
    }
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.cue, "A");
    assert_eq!(a.practice.cards.len(), 26);
    assert_eq!(a.practice.attempts.len(), 26);
    engine::validate_app(&a).unwrap();
    let mut b = app();
    let vid = b
        .library
        .add(library::nato_input().unwrap(), "normal-import", None)
        .unwrap();
    let mut f = fields(&b, "nato-focus-illegal-pace");
    f.insert("version_id".into(), vid);
    f.insert("pace".into(), "nato".into());
    let before = b.clone();
    assert_eq!(
        engine::mutate(&mut b, "/practice/learn", &f, NOW, "normal-learn")
            .unwrap_err()
            .status,
        422
    );
    assert_eq!(b, before);
}
#[test]
fn allowance_defaults_off_for_old_studies_and_does_not_rewrite_pace() {
    let mut a = app();
    let vid = a
        .library
        .add(library::nato_input().unwrap(), "compat-import", None)
        .unwrap();
    let mut f = fields(&a, "nato-focus-compat-learn");
    f.insert("version_id".into(), vid);
    engine::mutate(&mut a, "/practice/learn", &f, NOW, "compat-learn").unwrap();
    let before = a.practice.studies.values().next().unwrap().clone();
    let old_json = serde_json::to_value(&before).unwrap();
    assert!(old_json.get("nato_all_letters").is_none());
    let decoded: study::Study = serde_json::from_value(old_json).unwrap();
    assert_eq!(decoded, before);
    start(&mut a, "nato-focus-compat-start");
    let after = a.practice.studies.values().next().unwrap();
    assert_eq!(after.pace, before.pace);
    assert!(after.nato_all_letters);
    // A prior reader ignores the new optional field and still sees its original Pace.
    #[derive(serde::Deserialize)]
    struct PreviousReader {
        pace: study::Pace,
    }
    let prior: PreviousReader =
        serde_json::from_value(serde_json::to_value(after).unwrap()).unwrap();
    assert_eq!(prior.pace, before.pace);
}
#[test]
fn imported_non_nato_study_cannot_claim_the_focused_allowance() {
    let mut a = app();
    let mut input = library::nato_input().unwrap();
    input.title = "Numbers".into();
    input.text = "1\tOne\n".into();
    input.sections.as_mut().unwrap()[0].units = vec!["1\tOne\n".into()];
    let vid = a.library.add(input, "number-import", None).unwrap();
    let mut f = fields(&a, "nato-focus-number-learn");
    f.insert("version_id".into(), vid);
    engine::mutate(&mut a, "/practice/learn", &f, NOW, "number-learn").unwrap();
    engine::validate_app(&a).unwrap();
    a.practice
        .studies
        .values_mut()
        .next()
        .unwrap()
        .nato_all_letters = true;
    assert_eq!(engine::validate_app(&a).unwrap_err().status, 422);
}

fn answer_at(a: &mut App, answer: Option<&str>, now: i64, tag: &str) {
    let p = a.practice.current.as_ref().unwrap().prompt.clone();
    let mut f = fields(a, &format!("loop-answer-{tag}"));
    f.insert("prompt_id".into(), p.id);
    let path = if let Some(answer) = answer {
        f.insert("answer".into(), answer.into());
        "/practice/answer"
    } else {
        "/practice/reveal"
    };
    engine::mutate(a, path, &f, now, &format!("answer-{tag}")).unwrap();
}
fn next_at(a: &mut App, now: i64, tag: &str) {
    let p = a.practice.current.as_ref().unwrap().prompt.clone();
    let mut f = fields(a, &format!("loop-next-{tag}"));
    f.insert("after".into(), p.id);
    f.insert("study_id".into(), p.study_id);
    engine::mutate(a, "/practice/next", &f, now, &format!("next-{tag}")).unwrap();
}
#[test]
fn nato_reveal_then_next_does_not_trap_the_learner_on_alfa() {
    let mut a = app();
    start(&mut a, "loop-start-reveal");
    answer_at(&mut a, None, NOW + 5_000, "reveal-a");
    answer_at(&mut a, Some("Alfa"), NOW + 6_000, "assisted-a");
    let before = a.practice.cards.clone();
    next_at(&mut a, NOW + 8_000, "reveal-a");
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.cue, "B");
    for (id, card) in before {
        assert_eq!(a.practice.cards[&id], card);
    }
    engine::validate_app(&a).unwrap();
}
#[test]
fn nato_wrong_then_delayed_next_does_not_trap_the_learner_on_alfa() {
    let mut a = app();
    start(&mut a, "loop-start-wrong");
    answer_at(&mut a, Some("Alpha"), NOW + 5_000, "wrong-a");
    let due = a
        .practice
        .cards
        .values()
        .next()
        .unwrap()
        .state
        .schedule
        .due_ms;
    next_at(&mut a, due + 5_000, "wrong-a");
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.cue, "B");
    engine::validate_app(&a).unwrap();
}

#[test]
fn nato_two_rounds_preserve_history_and_schedules_with_realistic_delays() {
    let mut a = app();
    start(&mut a, "loop-two-rounds-start");
    let mut now = NOW;
    for index in 0..52 {
        let p = a.practice.current.as_ref().unwrap().prompt.clone();
        assert_eq!(p.cue, char::from(b'A' + (index % 26) as u8).to_string());
        let before = a.practice.clone();
        // Reloading the persisted application preserves both prompt and evidence.
        a = serde_json::from_str(&serde_json::to_string(&a).unwrap()).unwrap();
        assert_eq!(a.practice, before);
        now += 5_000;
        let tag = format!("two-rounds-{index}");
        let unit = a.library.versions[&p.version_id]
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .find(|u| p.unit_ids.contains(&u.id))
            .unwrap();
        let correct = library::mapping_pair(&unit.text).unwrap().1.to_string();
        if index % 3 == 0 {
            answer_at(&mut a, None, now, &format!("{tag}-reveal"));
        }
        answer_at(
            &mut a,
            Some(if index % 3 == 1 { "wrong" } else { &correct }),
            now + 1_000,
            &tag,
        );
        let cards = a.practice.cards.clone();
        let attempts = a.practice.attempts.clone();
        now += 70_000; // The learner reads feedback past Again's one-minute due time.
        let mut f = fields(&a, &format!("loop-repeat-next-{index}"));
        f.insert("after".into(), p.id);
        f.insert("study_id".into(), p.study_id);
        let result = engine::mutate(&mut a, "/practice/next", &f, now, &tag).unwrap();
        let saved = a.clone();
        assert_eq!(
            engine::mutate(&mut a, "/practice/next", &f, now + 2_000, "retry").unwrap(),
            result
        );
        assert_eq!(a, saved, "Duplicate Next cannot skip a letter");
        assert_eq!(a.practice.attempts, attempts);
        for (id, card) in cards {
            assert_eq!(
                a.practice.cards[&id], card,
                "Next is selection, never grading"
            );
        }
        engine::validate_app(&a).unwrap();
    }
    assert_eq!(a.practice.cards.len(), 26);
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.cue, "A");
    assert!(
        a.practice
            .attempts
            .values()
            .any(|a| a.effects.iter().any(|e| e.extra_practice && !e.scheduled))
    );
}

#[test]
fn nato_existing_overdue_correct_extra_practice_advances_without_reset() {
    let mut a = app();
    start(&mut a, "loop-existing-start");
    answer_at(&mut a, Some("Alpha"), NOW + 5_000, "existing-wrong");
    // Reproduce an already stuck learner's held repeated A through the existing
    // explicit-card API. This does not fabricate or rewrite prior evidence.
    let p = a.practice.current.as_ref().unwrap().prompt.clone();
    let mut f = fields(&a, "loop-existing-explicit");
    f.insert("after".into(), p.id);
    f.insert(
        "card_id".into(),
        a.practice.cards.keys().next().unwrap().clone(),
    );
    engine::mutate(
        &mut a,
        "/practice/next",
        &f,
        NOW + 70_000,
        "existing-repeat",
    )
    .unwrap();
    answer_at(&mut a, Some("Alfa"), NOW + 75_000, "existing-correct");
    let latest = a
        .practice
        .attempts
        .values()
        .max_by_key(|a| a.sequence)
        .unwrap();
    assert!(latest.effects[0].extra_practice);
    assert!(!latest.effects[0].scheduled);
    let before = a.practice.cards.clone();
    next_at(&mut a, NOW + 80_000, "existing-correct");
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.cue, "B");
    for (id, card) in before {
        assert_eq!(a.practice.cards[&id], card);
    }
    engine::validate_app(&a).unwrap();
}

#[test]
fn retained_nato_study_without_new_start_advances_without_migration() {
    let mut a = app();
    let vid = a
        .library
        .add(library::nato_input().unwrap(), "legacy", None)
        .unwrap();
    let mut f = fields(&a, "loop-legacy-learn");
    f.insert("version_id".into(), vid);
    engine::mutate(&mut a, "/practice/learn", &f, NOW, "legacy-learn").unwrap();
    let f = fields(&a, "loop-legacy-first");
    engine::mutate(&mut a, "/practice/next", &f, NOW, "legacy-first").unwrap();
    assert!(!a.practice.studies.values().next().unwrap().nato_all_letters);
    answer_at(&mut a, None, NOW + 5_000, "legacy-reveal");
    answer_at(&mut a, Some("Alfa"), NOW + 8_000, "legacy-answer");
    let studies = a.practice.studies.clone();
    let cards = a.practice.cards.clone();
    next_at(&mut a, NOW + 80_000, "legacy-next");
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.cue, "B");
    assert_eq!(a.practice.studies, studies);
    for (id, card) in cards {
        assert_eq!(a.practice.cards[&id], card);
    }
    engine::validate_app(&a).unwrap();
}
