use scry::{engine, model::App, result_progress, study, web};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
fn app() -> App {
    App::new("result-progress-synthetic-csrf-32".into())
}
fn act(a: &mut App, path: &str, extra: &[(&str, &str)]) {
    let mut f = BTreeMap::from([
        ("csrf".into(), a.csrf.clone()),
        ("revision".into(), a.revision.to_string()),
        (
            "operation_id".into(),
            format!("result-progress-{}", a.revision),
        ),
    ]);
    f.extend(extra.iter().map(|(k, v)| (k.to_string(), v.to_string())));
    let seed = format!("result-progress-seed-{}", a.revision);
    engine::mutate(a, path, &f, NOW, &seed).unwrap();
}
fn answer(a: &mut App, text: &str) {
    if a.practice.current.as_ref().unwrap().source_visible() {
        let id = a.practice.current.as_ref().unwrap().prompt.id.clone();
        act(a, "/practice/quiz", &[("prompt_id", &id)]);
    }
    let id = a.practice.current.as_ref().unwrap().prompt.id.clone();
    act(
        a,
        "/practice/answer",
        &[("prompt_id", &id), ("answer", text)],
    );
}
fn attempt(a: &App) -> &study::Attempt {
    &a.practice.attempts[a
        .practice
        .current
        .as_ref()
        .unwrap()
        .attempt_id
        .as_ref()
        .unwrap()]
}
fn page(a: &App) -> String {
    web::render(a, "/", &BTreeMap::new(), NOW, None)
}
#[test]
fn result_is_automatic_committed_and_read_only() {
    let mut a = app();
    act(&mut a, "/practice/nato/start", &[]);
    assert!(!page(&a).contains("data-result-progress"));
    answer(&mut a, "Alfa");
    let before = a.clone();
    let e = result_progress::summarize(&a, attempt(&a));
    assert_eq!((e.correct, e.unaided, e.helped), (1, 1, 0));
    assert!(e.next_due_ms.unwrap() > NOW);
    let html = page(&a);
    assert_eq!(html.matches("data-result-progress").count(), 1);
    assert!(html.contains("data-result-metric=\"recall\">1 / 1"));
    assert!(html.contains("Unaided correct"));
    assert!(html.find("data-result-progress").unwrap() < html.find("data-next").unwrap());
    assert_eq!(a, before);
    assert_eq!(page(&a), html);
    let restored: App = serde_json::from_str(&serde_json::to_string(&a).unwrap()).unwrap();
    assert_eq!(page(&restored), html);
    // Read actual cards after a schedule correction, not immutable after-snapshot.
    let card = attempt(&a).effects[0].card_id.clone();
    a.practice
        .cards
        .get_mut(&card)
        .unwrap()
        .state
        .schedule
        .due_ms = NOW + 9 * 86_400_000;
    assert!(page(&a).contains("in 9 days"));
}
#[test]
fn helped_and_reveals_never_become_recall_success() {
    let mut a = app();
    act(&mut a, "/practice/nato/start", &[]);
    let prompt_id = a.practice.current.as_ref().unwrap().prompt.id.clone();
    act(&mut a, "/practice/reveal", &[("prompt_id", &prompt_id)]);
    assert!(!page(&a).contains("data-result-progress"));
    answer(&mut a, "Alfa");
    let e = result_progress::summarize(&a, attempt(&a));
    assert_eq!((e.correct, e.unaided, e.helped), (0, 0, 1));
    assert_eq!(e.recent.len(), 1);
    assert_eq!(e.next_due_ms, None);
    let html = page(&a);
    assert!(html.contains("Unaided recall not tested"));
    assert!(html.contains("Not scheduled"));
    assert!(!html.contains("data-result-metric=\"recall\">1 / 1"));
}
#[test]
fn phrase_join_and_whole_are_distinct_evidence() {
    let mut a = app();
    act(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "st-michael-en")],
    );
    let v = a.library.versions[&a.practice.current.as_ref().unwrap().prompt.version_id].clone();
    answer(&mut a, &v.sections[0].units[0].text);
    assert!(page(&a).contains("This phrase"));
    assert_eq!(result_progress::summarize(&a, attempt(&a)).unaided, 1);
    assert!(!page(&a).contains("Unaided recall not tested"));
    let after = a.practice.current.as_ref().unwrap().prompt.id.clone();
    act(
        &mut a,
        "/practice/lesson/next",
        &[("after", &after), ("mode", "join")],
    );
    let joined = a.practice.current.as_ref().unwrap().prompt.clone();
    assert!(matches!(joined.scope, study::Scope::Section { .. }));
    let joined_text = v
        .sections
        .iter()
        .flat_map(|s| &s.units)
        .filter(|u| joined.unit_ids.contains(&u.id))
        .map(|u| u.text.as_str())
        .collect::<String>();
    answer(&mut a, &joined_text);
    let joined_evidence = result_progress::summarize(&a, attempt(&a));
    assert_eq!(joined_evidence.helped + joined_evidence.unaided, 1);
    assert!(page(&a).contains("This passage"));
    let after = a.practice.current.as_ref().unwrap().prompt.id.clone();
    act(
        &mut a,
        "/practice/lesson/next",
        &[("after", &after), ("mode", "whole")],
    );
    answer(&mut a, &v.text);
    let e = result_progress::summarize(&a, attempt(&a));
    assert_eq!((e.correct, e.unaided, e.helped), (1, 1, 0));
    let html = page(&a);
    assert!(html.contains("Full text"));
    assert!(html.contains("Next part review"));
    let cards = attempt(&a)
        .effects
        .iter()
        .map(|e| e.card_id.clone())
        .collect::<Vec<_>>();
    a.practice
        .cards
        .get_mut(&cards[0])
        .unwrap()
        .state
        .schedule
        .last_review_ms = None;
    a.practice
        .cards
        .get_mut(&cards[1])
        .unwrap()
        .state
        .schedule
        .due_ms = NOW + 60_000;
    let mixed = result_progress::summarize(&a, attempt(&a));
    assert_eq!(mixed.unscheduled_parts, 1);
    assert_eq!(mixed.next_due_ms, Some(NOW + 60_000));
    assert!(page(&a).contains("Some parts not scheduled yet"));
    let id = attempt(&a).prompt.study_id.clone();
    a.practice.studies.get_mut(&id).unwrap().status = study::Status::Paused;
    assert!(!page(&a).contains("Not active"));
    assert_eq!(
        result_progress::summarize(&a, attempt(&a)).next_due_ms,
        Some(NOW + 60_000)
    );
    a.practice.studies.get_mut(&id).unwrap().status = study::Status::Dropped;
    assert!(page(&a).contains("Not active"));
    assert_eq!(
        result_progress::summarize(&a, attempt(&a)).next_due_ms,
        None
    );
}
#[test]
fn original_outcomes_and_same_scope_are_counted_without_other_letters() {
    let mut a = app();
    act(&mut a, "/practice/nato/start", &[]);
    answer(&mut a, "wrong");
    let first = attempt(&a).clone();
    assert_eq!(
        result_progress::summarize(&a, &first).recent,
        vec![result_progress::Mark::Miss]
    );
    let after = first.prompt.id.clone();
    let sid = first.prompt.study_id.clone();
    act(
        &mut a,
        "/practice/next",
        &[("after", &after), ("study_id", &sid)],
    );
    answer(&mut a, "Bravo");
    let e = result_progress::summarize(&a, attempt(&a));
    assert_eq!((e.correct, e.unaided), (1, 1));
    assert_eq!(result_progress::summarize(&a, &first).unaided, 1);
}
