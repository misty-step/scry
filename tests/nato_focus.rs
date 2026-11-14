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
    assert!(
        a.practice.current.is_none(),
        "All future-due letters are caught up"
    );
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
    if answer.is_some() && a.practice.current.as_ref().unwrap().source_visible() {
        let mut f = fields(a, &format!("loop-quiz-{tag}"));
        f.insert(
            "prompt_id".into(),
            a.practice.current.as_ref().unwrap().prompt.id.clone(),
        );
        engine::mutate(a, "/practice/quiz", &f, now, &format!("quiz-{tag}")).unwrap();
    }
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
fn nato_eligible_interleaving_preserves_history_and_schedules_with_realistic_delays() {
    let mut a = app();
    start(&mut a, "loop-two-rounds-start");
    let mut now = NOW;
    for index in 0..52 {
        let p = a.practice.current.as_ref().unwrap().prompt.clone();
        let card = &a.practice.cards[&a.practice.current.as_ref().unwrap().targets[0].card_id];
        assert!(
            card.state.schedule.due_ms <= now,
            "Only due/new letters are presented"
        );
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
        f.insert(
            "after".into(),
            a.practice.current.as_ref().unwrap().prompt.id.clone(),
        );
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
    assert!(!a.practice.cards.is_empty());
    assert!(a.practice.current.is_some());
    assert!(a.practice.attempts.values().all(|a| {
        a.effects
            .iter()
            .all(|e| !e.extra_practice && (e.scheduled || e.assisted))
    }));
}

#[test]
fn nato_existing_overdue_correct_review_advances_without_reset() {
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
    assert!(!latest.effects[0].extra_practice);
    assert!(latest.effects[0].scheduled);
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

fn select_lesson(a: &mut App, starter: &str, now: i64, op: &str) -> MutationResult {
    let mut f = fields(a, op);
    f.insert("starter".into(), starter.into());
    if let Some(p) = &a.practice.current {
        f.insert(
            if p.attempt_id.is_some() {
                "after"
            } else {
                "leave"
            }
            .into(),
            p.prompt.id.clone(),
        );
    }
    engine::mutate(a, "/practice/lesson/start", &f, now, op).unwrap()
}
#[test]
fn selecting_nato_skips_future_a_and_preserves_five_reviews() {
    let mut a = app();
    start(&mut a, "schedule-start-operation");
    let mut now = NOW;
    for n in 0..5 {
        if n > 0 {
            let p = a.practice.current.as_ref().unwrap().prompt.clone();
            let cid = a.practice.current.as_ref().unwrap().targets[0]
                .card_id
                .clone();
            now = a.practice.cards[&cid]
                .state
                .schedule
                .due_ms
                .max(now + 86_400_000);
            let mut f = fields(&a, &format!("explicit-review-{n}"));
            f.insert("after".into(), p.id);
            f.insert("card_id".into(), cid);
            engine::mutate(&mut a, "/practice/next", &f, now, "review").unwrap();
        }
        answer_at(&mut a, Some("Alfa"), now, &format!("correct-{n}"));
    }
    let card = a.practice.cards.values().next().unwrap().clone();
    let attempts = a.practice.attempts.clone();
    assert!(card.state.schedule.due_ms > now);
    assert_eq!(attempts.len(), 5);
    select_lesson(&mut a, "rosary-joyful", now, "schedule-away-operation");
    let result = select_lesson(&mut a, "nato", now, "schedule-return-operation");
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.cue, "B");
    assert_eq!(a.practice.cards[&card.id], card);
    assert_eq!(a.practice.attempts, attempts);
    assert!(result.data.is_some());
    engine::validate_app(&a).unwrap();
}

fn completed_alphabet() -> App {
    let mut a = app();
    start(&mut a, "scheduled-complete-start");
    for index in 0..26 {
        let p = a.practice.current.as_ref().unwrap().prompt.clone();
        let v = &a.library.versions[&p.version_id];
        let u = v
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .find(|u| p.unit_ids.contains(&u.id))
            .unwrap();
        let word = library::mapping_pair(&u.text).unwrap().1.to_string();
        answer_at(&mut a, Some(&word), NOW, &format!("complete-{index}"));
        next_at(&mut a, NOW, &format!("complete-{index}"));
    }
    assert!(a.practice.current.is_none());
    a
}
#[test]
fn all_future_letters_caught_up_without_grading_or_cross_lesson_selection() {
    let mut a = completed_alphabet();
    select_lesson(&mut a, "rosary-joyful", NOW, "all-future-away-operation");
    let before = a.practice.clone();
    let result = select_lesson(&mut a, "nato", NOW, "all-future-return-operation");
    assert!(a.practice.current.is_none());
    assert_eq!(result.redirect, "/?practice=nato");
    assert_eq!(a.practice.cards, before.cards);
    assert_eq!(a.practice.attempts, before.attempts);
    let page = scry::web::render(
        &a,
        "/",
        &BTreeMap::from([("practice".into(), "nato".into())]),
        NOW,
        None,
    );
    // The lesson states its own Study's next review, not another Study's.
    let due = a
        .practice
        .cards
        .values()
        .filter(|c| {
            library::is_nato(&a.library.versions[&a.practice.studies[&c.study_id].version_id])
        })
        .map(|c| c.state.schedule.due_ms)
        .min()
        .unwrap();
    assert!(page.contains("data-state=\"rest\""));
    assert!(page.contains("<h1>Rest for now</h1>"));
    assert!(page.contains(&format!(
        "data-next-review>Next review {}.</p>",
        chrono::DateTime::from_timestamp_millis(due)
            .unwrap()
            .format("%b %-d, %Y · %H:%M UTC")
    )));
    assert!(page.contains("<summary><span>Browse</span>"));
    assert!(page.contains("class=\"nato-progress\""));
    assert!(!page.contains("All set for now") && !page.contains("Back to start"));
    assert!(!page.contains("action=\"/practice/nato/start\""));
    assert!(!page.contains("data-state=\"exact-prompt\""));
    engine::validate_app(&a).unwrap();
}
#[test]
fn scheduled_boundary_tie_and_sole_due_retry_use_same_lesson_policy() {
    let mut a = completed_alphabet();
    let due = a
        .practice
        .cards
        .values()
        .map(|c| c.state.schedule.due_ms)
        .min()
        .unwrap();
    let before = a.practice.clone();
    let query = BTreeMap::from([("practice".into(), "nato".into())]);
    assert!(scry::web::render(&a, "/", &query, due - 1, None).contains("<h1>Rest for now</h1>"));
    assert!(scry::web::render(&a, "/", &query, due, None).contains("Ready to review."));
    assert_eq!(a.practice, before, "Read does not select a prompt");
    select_lesson(&mut a, "nato", due - 1, "boundary-before-operation");
    assert!(a.practice.current.is_none());
    assert_eq!(a.practice.cards, before.cards);
    select_lesson(&mut a, "nato", due, "boundary-exact-operation");
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.cue, "A");
    answer_at(&mut a, Some("wrong"), due, "boundary-wrong");
    let previous = a.practice.current.as_ref().unwrap().prompt.id.clone();
    let mut f = fields(&a, "boundary-lesson-next-operation");
    f.insert("after".into(), previous);
    engine::mutate(
        &mut a,
        "/practice/lesson/next",
        &f,
        due + 70_000,
        "boundary-next",
    )
    .unwrap();
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.cue, "B");
    engine::validate_app(&a).unwrap();
}
#[test]
fn sole_due_previous_card_is_not_reported_caught_up() {
    let mut a = completed_alphabet();
    let now = NOW + 86_400_000;
    select_lesson(&mut a, "nato", now, "sole-nextday-start-operation");
    for index in 0..26 {
        let p = a.practice.current.as_ref().unwrap().prompt.clone();
        assert_eq!(p.cue, char::from(b'A' + index as u8).to_string());
        let v = &a.library.versions[&p.version_id];
        let u = v
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .find(|u| p.unit_ids.contains(&u.id))
            .unwrap();
        let word = library::mapping_pair(&u.text).unwrap().1.to_string();
        answer_at(
            &mut a,
            Some(if index == 25 { "wrong" } else { &word }),
            now,
            &format!("sole-answer-{index}"),
        );
        if index < 25 {
            next_at(&mut a, now, &format!("sole-next-{index}"));
        }
    }
    let p = a.practice.current.clone().unwrap();
    let retry_at = a.practice.cards[&p.targets[0].card_id]
        .state
        .schedule
        .due_ms;
    let before = a.practice.clone();
    next_at(&mut a, retry_at, "sole-due-retry");
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.cue, "Z");
    assert_eq!(a.practice.cards, before.cards);
    assert_eq!(a.practice.attempts, before.attempts);
    engine::validate_app(&a).unwrap();
}
