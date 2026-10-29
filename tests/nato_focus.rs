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
    assert!(a.practice.current.is_none());
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
