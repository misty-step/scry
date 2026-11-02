//! Original supplied short-answer content uses existing native import/grade policy.
use scry::{engine, model::*};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
fn mutate(a: &mut App, path: &str, extra: &[(&str, &str)]) -> MutationResult {
    mutate_at(a, path, extra, NOW)
}
fn mutate_at(a: &mut App, path: &str, extra: &[(&str, &str)], now: i64) -> MutationResult {
    let mut f = BTreeMap::from([
        ("csrf".into(), a.csrf.clone()),
        ("revision".into(), a.revision.to_string()),
        ("operation_id".into(), format!("baby-op-{:016}", a.revision)),
    ]);
    f.extend(extra.iter().map(|(k, v)| ((*k).into(), (*v).into())));
    let seed = format!("baby-seed-{}", a.revision);
    let result = engine::mutate(a, path, &f, now, &seed).unwrap();
    engine::validate_app(a).unwrap();
    result
}
fn new_app() -> App {
    App::new("synthetic-baby-csrf-at-least-32".into())
}
#[test]
fn starter_browsing_is_read_only_and_mappings_import_without_study() {
    let a = new_app();
    let before = a.clone();
    let html = scry::web::render(&a, "/lessons", &BTreeMap::new(), NOW, None);
    assert!(html.contains("Baby care"));
    assert!(html.contains("baby-soothing"));
    assert_eq!(a, before);
    let lessons = scry::lessons::catalog()
        .into_iter()
        .filter(|l| l.id.starts_with("baby-"))
        .collect::<Vec<_>>();
    assert_eq!(lessons.len(), 3);
    assert_eq!(
        lessons
            .iter()
            .map(|l| l
                .input
                .sections
                .iter()
                .flatten()
                .map(|s| s.units.len())
                .sum::<usize>())
            .sum::<usize>(),
        18
    );
    let mut library = scry::library::Library::default();
    for l in lessons {
        assert!(l.input.spelling_strict);
        assert!(!l.input.sources.is_empty());
        let vid = library.add(l.input.clone(), &l.id, None).unwrap();
        let v = &library.versions[&vid];
        assert_eq!(v.kind, scry::library::Kind::KeyedMapping);
        assert!(l.matches(v));
        assert!(scry::lessons::mapping_route(v).is_some());
        assert!(
            v.sections
                .iter()
                .flat_map(|s| &s.units)
                .all(|u| scry::library::mapping_pair(&u.text).is_some())
        );
    }
    assert!(a.practice.studies.is_empty());
}
#[test]
fn each_baby_prompt_grades_exactly_without_partial_unsafe_credit() {
    for l in scry::lessons::catalog()
        .into_iter()
        .filter(|l| l.id.starts_with("baby-"))
    {
        let mut a = new_app();
        mutate(&mut a, "/practice/lesson/start", &[("starter", &l.id)]);
        let first = a.practice.current.as_ref().unwrap().clone();
        assert!(!a.practice.studies[&first.prompt.study_id].lesson);
        let v = &a.library.versions[&first.prompt.version_id];
        let unit_texts = v
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .map(|u| u.text.clone())
            .collect::<Vec<_>>();
        for text in unit_texts {
            let (_, value) = scry::library::mapping_pair(&text).unwrap();
            let expected = vec![value];
            let version = scry::grading::Version {
                units: &expected,
                spelling_strict: true,
            };
            assert_eq!(
                scry::grading::grade_mapping(version, 0..1, &value.to_uppercase())
                    .unwrap()
                    .verdict,
                scry::grading::Verdict::Good
            );
            for wrong in [
                format!("not {value}"),
                format!("{value} unsafe"),
                "unrelated".into(),
                match value {
                    "back" => "stomach",
                    "immediately" => "later",
                    "never" => "sometimes",
                    "low" => "high",
                    "awake" => "asleep",
                    "swaddle" => "swadle",
                    _ => "unsafe alternative",
                }
                .into(),
            ] {
                assert_ne!(
                    scry::grading::grade_mapping(version, 0..1, &wrong)
                        .unwrap()
                        .verdict,
                    scry::grading::Verdict::Good
                );
            }
        }
        let page = scry::web::render(
            &a,
            "/",
            &BTreeMap::from([("practice".into(), l.id.clone())]),
            NOW,
            None,
        );
        assert!(page.contains("mapping-study"));
        assert!(!page.contains("nato-letter"));
        assert!(page.contains("readiness-study"));
        let cue = first
            .prompt
            .cue
            .replace('&', "&amp;")
            .replace('<', "&lt;")
            .replace('>', "&gt;")
            .replace('\"', "&quot;")
            .replace('\'', "&#39;");
        assert!(page.contains(&cue), "entire safety cue remains visible");
    }
}
#[test]
fn starter_switch_and_reveal_preserve_history_and_help_does_not_schedule() {
    let mut a = new_app();
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "baby-soothing")],
    );
    let p = a.practice.current.clone().unwrap();
    let schedule = a.practice.cards[&p.targets[0].card_id]
        .state
        .schedule
        .clone();
    let expected = scry::library::mapping_pair(
        &a.library.versions[&p.prompt.version_id].sections[0].units[0].text,
    )
    .unwrap()
    .1
    .to_string();
    mutate(&mut a, "/practice/reveal", &[("prompt_id", &p.prompt.id)]);
    let attempts = a.practice.attempts.clone();
    mutate(&mut a, "/practice/quiz", &[("prompt_id", &p.prompt.id)]);
    assert_eq!(a.practice.attempts, attempts);
    let quiz_id = a.practice.current.as_ref().unwrap().prompt.id.clone();
    mutate(
        &mut a,
        "/practice/answer",
        &[("prompt_id", &quiz_id), ("answer", &expected)],
    );
    assert_eq!(
        a.practice.cards[&p.targets[0].card_id].state.schedule,
        schedule
    );
    let attempt = a.practice.attempts[&a
        .practice
        .current
        .as_ref()
        .unwrap()
        .attempt_id
        .clone()
        .unwrap()]
        .clone();
    assert!(attempt.assisted);
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "baby-montessori"), ("after", &quiz_id)],
    );
    assert_eq!(a.practice.attempts[&attempt.id], attempt);
    assert_ne!(
        a.practice.current.as_ref().unwrap().prompt.version_id,
        p.prompt.version_id
    );
}

#[test]
fn focused_next_preserves_subject_and_source_notes_require_disclosure() {
    let mut a = new_app();
    let route = mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "baby-soothing")],
    );
    assert_eq!(route.redirect, "/?practice=baby-soothing");
    let p = a.practice.current.clone().unwrap();
    let answer = scry::library::mapping_pair(
        &a.library.versions[&p.prompt.version_id].sections[0].units[0].text,
    )
    .unwrap()
    .1
    .to_string();
    let protected = scry::web::render(&a, "/", &BTreeMap::new(), NOW, None);
    assert!(!protected.contains(">Source notes</a>"));
    mutate(
        &mut a,
        "/practice/answer",
        &[("prompt_id", &p.prompt.id), ("answer", &answer)],
    );
    let result = scry::web::render(&a, "/", &BTreeMap::new(), NOW, None);
    assert!(result.contains(">Source notes</a>"));
    assert!(result.contains(&format!(
        "name=\"study_id\" value=\"{}\"",
        p.prompt.study_id
    )));
    let next = mutate(
        &mut a,
        "/practice/next",
        &[("study_id", &p.prompt.study_id), ("after", &p.prompt.id)],
    );
    assert_eq!(next.redirect, "/?practice=baby-soothing");
    assert_eq!(
        a.practice.current.as_ref().unwrap().prompt.study_id,
        p.prompt.study_id
    );
    let snapshot = a.clone();
    scry::web::render(
        &a,
        "/",
        &BTreeMap::from([("practice".into(), "baby-soothing".into())]),
        NOW,
        None,
    );
    assert_eq!(a, snapshot);
}

fn answer_current(a: &mut App, now: i64) -> String {
    let p = a.practice.current.clone().unwrap();
    let v = &a.library.versions[&p.prompt.version_id];
    let u = v
        .sections
        .iter()
        .flat_map(|s| &s.units)
        .find(|u| u.id == p.prompt.unit_ids[0])
        .unwrap();
    let answer = scry::library::mapping_pair(&u.text).unwrap().1.to_string();
    mutate_at(
        a,
        "/practice/answer",
        &[("prompt_id", &p.prompt.id), ("answer", &answer)],
        now,
    );
    p.prompt.id
}
#[test]
fn focused_starter_finishes_its_paced_batch_without_future_repeats() {
    let mut a = new_app();
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "baby-soothing")],
    );
    let sid = a.practice.current.as_ref().unwrap().prompt.study_id.clone();
    let mut seen = std::collections::BTreeSet::new();
    for _ in 0..6 {
        let p = a.practice.current.as_ref().unwrap();
        assert!(
            seen.insert(p.prompt.unit_ids[0].clone()),
            "no future repeat before unseen cards"
        );
        let after = answer_current(&mut a, NOW);
        mutate(
            &mut a,
            "/practice/next",
            &[("study_id", &sid), ("after", &after)],
        );
    }
    assert!(a.practice.current.is_none());
    assert_eq!(a.practice.cards.len(), 6);
    let before = a.clone();
    let page = scry::web::render(
        &a,
        "/",
        &BTreeMap::from([("practice".into(), "baby-soothing".into())]),
        NOW,
        None,
    );
    assert!(page.contains("Next review"));
    assert!(!page.contains(">Continue</button>"));
    assert_eq!(a, before);
}
#[test]
fn scheduled_baby_subjects_join_mixed_due_without_focused_next_override() {
    let mut a = new_app();
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "baby-soothing")],
    );
    let first = a.practice.current.as_ref().unwrap().prompt.study_id.clone();
    let after = answer_current(&mut a, NOW);
    mutate(
        &mut a,
        "/practice/lesson/start",
        &[("starter", "baby-montessori"), ("after", &after)],
    );
    let second = a.practice.current.as_ref().unwrap().prompt.study_id.clone();
    let after = answer_current(&mut a, NOW);
    let due = a
        .practice
        .cards
        .values()
        .map(|c| c.state.schedule.due_ms)
        .max()
        .unwrap()
        + 1;
    mutate_at(&mut a, "/practice/review/start", &[("after", &after)], due);
    assert!(a.practice.mixed_review);
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.study_id, first);
    let after = answer_current(&mut a, due);
    let page = scry::web::render(&a, "/", &BTreeMap::new(), due, None);
    let form = page
        .split("<form")
        .find(|f| f.starts_with(" method=\"post\" action=\"/practice/review/next\""))
        .unwrap()
        .split("</form>")
        .next()
        .unwrap();
    assert!(!form.contains("name=\"study_id\""));
    mutate_at(&mut a, "/practice/review/next", &[("after", &after)], due);
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.study_id, second);
    assert_eq!(
        a.practice.cards.len(),
        2,
        "mixed review does not enroll new items"
    );
}
