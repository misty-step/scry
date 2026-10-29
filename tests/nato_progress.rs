use scry::{
    engine, library,
    model::*,
    nato_progress::{self, Outcome},
    study,
};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
fn app() -> App {
    App::new("synthetic-progress-csrf-32-chars".into())
}
fn mutate(a: &mut App, path: &str, extra: &[(&str, String)], now: i64) {
    let op = format!("nato-progress-operation-{:08}", a.revision);
    let mut fields = BTreeMap::from([
        ("csrf".into(), a.csrf.clone()),
        ("operation_id".into(), op.clone()),
        ("revision".into(), a.revision.to_string()),
    ]);
    fields.extend(extra.iter().map(|(k, v)| (k.to_string(), v.clone())));
    engine::mutate(a, path, &fields, now, &op).unwrap();
}
fn start(a: &mut App) {
    mutate(a, "/practice/nato/start", &[], NOW);
}
fn answer(a: &mut App, text: &str, now: i64) {
    let p = a.practice.current.as_ref().unwrap().prompt.clone();
    mutate(
        a,
        "/practice/answer",
        &[("prompt_id", p.id), ("answer", text.into())],
        now,
    );
}
fn next(a: &mut App, now: i64) {
    let p = a.practice.current.as_ref().unwrap().prompt.clone();
    mutate(
        a,
        "/practice/next",
        &[("after", p.id), ("study_id", p.study_id)],
        now,
    );
}
fn reveal(a: &mut App, now: i64) {
    let p = a.practice.current.as_ref().unwrap().prompt.clone();
    mutate(a, "/practice/reveal", &[("prompt_id", p.id)], now);
}
#[test]
fn unseen_and_introduced_letters_have_no_accuracy_or_due_prediction() {
    let mut a = app();
    let p = nato_progress::summarize(&a);
    assert_eq!(p.letters.len(), 26);
    assert_eq!(p.letters.first().unwrap().letter, 'A');
    assert_eq!(p.letters.last().unwrap().letter, 'Z');
    assert_eq!(p.accuracy_percent(), None);
    assert_eq!(p.reviews, 0);
    start(&mut a);
    let before = a.clone();
    let p = nato_progress::summarize(&a);
    assert_eq!(p.letters[0].accuracy_percent(), None);
    assert_eq!(p.letters[0].due_ms, None);
    assert_eq!(a, before);
}
#[test]
fn reveal_only_and_assisted_answer_are_not_two_reviews_or_unaided_success() {
    let mut a = app();
    start(&mut a);
    reveal(&mut a, NOW + 1);
    let p = nato_progress::summarize(&a);
    assert_eq!(p.letters[0].reveals, 1);
    assert_eq!(p.reviews, 0);
    assert_eq!(p.accuracy_percent(), None);
    answer(&mut a, "Alfa", NOW + 2);
    engine::validate_app(&a).unwrap();
    let p = nato_progress::summarize(&a);
    let r = &p.letters[0];
    assert_eq!(
        (r.reviews, r.assisted, r.unassisted, r.correct),
        (1, 1, 0, 0)
    );
    assert_eq!(r.last_answered_ms, Some(NOW + 2));
    assert_eq!(r.due_ms, None);
    assert_eq!(r.recent[0].outcome, Outcome::Assisted);
    assert_eq!(p.practiced, 0);
    assert_eq!(p.accuracy_percent(), None);
}
#[test]
fn correct_miss_extra_practice_and_actual_scheduled_due_are_distinct() {
    let mut a = app();
    start(&mut a);
    answer(&mut a, "Alpha", NOW + 1);
    let p = nato_progress::summarize(&a);
    assert_eq!(p.letters[0].accuracy_percent(), Some(0));
    assert_eq!(p.letters[0].due_ms, Some(NOW + 60_001));
    for i in 1..26 {
        next(&mut a, NOW + i * 1000);
        let prompt = &a.practice.current.as_ref().unwrap().prompt;
        let v = &a.library.versions[&prompt.version_id];
        let u = v
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .find(|u| prompt.unit_ids.contains(&u.id))
            .unwrap();
        let word = library::mapping_pair(&u.text).unwrap().1.to_owned();
        answer(&mut a, &word, NOW + i * 1000 + 1);
    }
    next(&mut a, NOW + 70_000);
    answer(&mut a, "Alfa", NOW + 71_000);
    engine::validate_app(&a).unwrap();
    let p = nato_progress::summarize(&a);
    let r = &p.letters[0];
    assert_eq!((r.reviews, r.correct, r.unassisted), (2, 1, 2));
    assert_eq!(r.accuracy_percent(), Some(50));
    assert_eq!(
        r.recent.iter().map(|m| m.outcome).collect::<Vec<_>>(),
        vec![Outcome::Miss, Outcome::Correct]
    );
    assert_eq!(p.practiced, 26);
    assert_eq!(p.reviews, 27);
    assert_eq!(p.correct, 26);
    // Same-day extra practice is still an observed answer, not a schedule write.
    assert_eq!(r.due_ms, Some(NOW + 60_001));
    let held = a.practice.current.as_ref().unwrap();
    assert!(a.practice.attempts[held.attempt_id.as_ref().unwrap()].effects[0].extra_practice);
}
#[test]
fn retained_false_allowance_flag_preserves_metrics_and_reads_are_pure() {
    let mut a = app();
    start(&mut a);
    answer(&mut a, "Alfa", NOW);
    a.practice
        .studies
        .values_mut()
        .next()
        .unwrap()
        .nato_all_letters = false;
    let a: App = serde_json::from_value(serde_json::to_value(a).unwrap()).unwrap();
    engine::validate_app(&a).unwrap();
    let before = a.clone();
    let p = nato_progress::summarize(&a);
    assert_eq!(p.reviews, 1);
    assert_eq!(p.accuracy_percent(), Some(100));
    assert!(p.letters[0].due_ms.is_some());
    assert_eq!(a, before);
}
#[test]
fn unrelated_mapping_with_nato_title_does_not_enter_evidence() {
    let mut a = app();
    let mut input = library::nato_input().unwrap();
    input.text = "A\tApple\n".into();
    input.sections.as_mut().unwrap()[0].units = vec!["A\tApple\n".into()];
    let vid = a.library.add(input, "other", None).unwrap();
    mutate(&mut a, "/practice/learn", &[("version_id", vid)], NOW);
    mutate(&mut a, "/practice/next", &[], NOW);
    answer(&mut a, "Apple", NOW);
    engine::validate_app(&a).unwrap();
    assert_eq!(nato_progress::summarize(&a).reviews, 0);
}
#[test]
fn paused_schedule_does_not_promise_next_review_but_history_survives() {
    let mut a = app();
    start(&mut a);
    answer(&mut a, "Alfa", NOW);
    let sid = a.practice.studies.keys().next().unwrap().clone();
    mutate(
        &mut a,
        "/practice/pause",
        &[("study_id", sid.clone())],
        NOW + 1,
    );
    assert_eq!(a.practice.studies[&sid].status, study::Status::Paused);
    let p = nato_progress::summarize(&a);
    assert_eq!(p.reviews, 1);
    assert_eq!(p.letters[0].due_ms, None);
}
#[test]
fn copies_of_same_prompt_cannot_inflate_reviews_and_trail_is_bounded() {
    let mut a = app();
    start(&mut a);
    answer(&mut a, "Alfa", NOW);
    let original = a.practice.attempts.values().next().unwrap().clone();
    // Projection is defensive if historical import contains repeated records.
    let mut copy = original.clone();
    copy.id = "duplicate-record".into();
    a.practice.attempts.insert(copy.id.clone(), copy);
    assert_eq!(nato_progress::summarize(&a).reviews, 1);
    for i in 1..11 {
        let mut distinct = original.clone();
        distinct.id = format!("answer-{i}");
        distinct.prompt.id = format!("prompt-{i}");
        distinct.created_ms = NOW + i;
        distinct.sequence += i as u64;
        a.practice.attempts.insert(distinct.id.clone(), distinct);
    }
    let p = nato_progress::summarize(&a);
    assert_eq!(p.reviews, 11);
    assert_eq!(p.letters[0].recent.len(), 8);
    assert_eq!(p.letters[0].recent[0].created_ms, NOW + 3);
    assert_eq!(p.letters[0].recent[7].created_ms, NOW + 10);
}

#[test]
fn unit_identity_wins_over_display_cue_and_answer_text_never_enters_projection() {
    let mut a = app();
    start(&mut a);
    answer(&mut a, "Alfa", NOW);
    // Historical display text cannot move A's evidence onto a different letter.
    let attempt = a.practice.attempts.values_mut().next().unwrap();
    attempt.prompt.cue = "Z".into();
    attempt.raw_answer = "private answer sentinel".into();
    let p = nato_progress::summarize(&a);
    assert_eq!(p.letters[0].reviews, 1);
    assert_eq!(p.letters[25].reviews, 0);
    assert!(!format!("{p:?}").contains("private answer sentinel"));
}
