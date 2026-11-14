//! NATO entry uses the same ordered pair identity as Library deduplication.
use scry::{
    api, engine,
    grading::Verdict,
    library,
    model::*,
    owner_token::{MutationActor, OwnerToken},
    persistence,
};
use serde_json::{Value, json};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
fn app() -> App {
    let mut app = App::new("synthetic-nato-import-csrf-32chars".into());
    let token = OwnerToken::issue(
        "NATO importer",
        None,
        false,
        NOW,
        "abcdef0123456789abcdef0123456789abcdef0123456789",
    )
    .unwrap();
    app.owner_tokens.insert(token.info.id.clone(), token);
    app
}
fn manifest(variant: &str) -> Value {
    let mut m: Value =
        serde_json::from_str(include_str!("../examples/nato-alphabet.json")).unwrap();
    match variant {
        "uppercase" => {
            for pair in m["pairs"].as_array_mut().unwrap() {
                pair["value"] = json!(pair["value"].as_str().unwrap().to_uppercase());
            }
        }
        "x-space" => m["pairs"][23]["value"] = json!("X ray"),
        "normalized-keys" => {
            for pair in m["pairs"].as_array_mut().unwrap() {
                pair["key"] = json!(format!("{}.", pair["key"].as_str().unwrap().to_lowercase()));
            }
        }
        "canonical" => {}
        _ => panic!("Unknown fixture"),
    }
    m
}
fn api_mutate(a: &mut App, path: &str, op: &str, mut data: Value) -> AppResult<MutationResult> {
    data["operation_id"] = json!(op);
    let fields = api::library_fields(data)?;
    let actor = a.owner_tokens.values().next().unwrap().info.clone();
    let result = engine::mutate_as(
        a,
        api::mutation_path(path).unwrap(),
        &fields,
        NOW,
        op,
        MutationActor::Token(&actor),
    )?;
    engine::validate_app(a)?;
    Ok(result)
}
fn focused(a: &mut App, op: &str) -> AppResult<MutationResult> {
    let fields = BTreeMap::from([
        ("csrf".into(), a.csrf.clone()),
        ("operation_id".into(), op.into()),
    ]);
    engine::mutate(a, "/practice/nato/start", &fields, NOW, op)
}
fn recovered(a: &App) -> App {
    let bytes = persistence::Archive::new(a.clone(), vec![], NOW)
        .encode()
        .unwrap();
    persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
        .unwrap()
        .app
}
#[test]
fn normalized_owner_import_dedupes_and_starts_focused_and_lessons_without_rewriting_history() {
    for variant in ["canonical", "uppercase", "x-space", "normalized-keys"] {
        let mut a = app();
        let supplied = manifest(variant);
        let vid = api_mutate(
            &mut a,
            "/api/v1/library/mappings",
            "nato-import-original",
            json!({"data":supplied}),
        )
        .unwrap()
        .entity_id
        .unwrap();
        let library = a.library.clone();
        assert!(library::is_nato(&library.versions[&vid]), "{variant}");
        let study = api_mutate(
            &mut a,
            "/api/v1/learn",
            "nato-import-learn",
            json!({"version_id":vid,"pace":"light"}),
        )
        .unwrap()
        .entity_id
        .unwrap();
        api_mutate(
            &mut a,
            "/api/v1/next",
            "nato-import-next",
            json!({"study_id":study}),
        )
        .unwrap();
        // The supplied pair is studied before the agent's first answer.
        let source = a.practice.current.as_ref().unwrap().prompt.id.clone();
        api_mutate(
            &mut a,
            "/api/v1/quiz",
            "nato-import-quiz",
            json!({"prompt_id":source}),
        )
        .unwrap();
        let prompt = a.practice.current.as_ref().unwrap().prompt.clone();
        api_mutate(
            &mut a,
            "/api/v1/answer",
            "nato-import-answer",
            json!({"prompt_id":prompt.id,"answer":"Alpha"}),
        )
        .unwrap();
        assert_eq!(
            a.practice.attempts.values().next().unwrap().grade.verdict,
            Verdict::Again
        );
        let before = a.clone();
        for op in ["nato-import-canonical", "nato-import-canonical"] {
            assert_eq!(
                api_mutate(
                    &mut a,
                    "/api/v1/library/mappings",
                    op,
                    json!({"data":manifest("canonical")})
                )
                .unwrap()
                .entity_id
                .as_deref(),
                Some(vid.as_str())
            );
            assert_eq!(a.library, library);
            assert_eq!(a.practice, before.practice);
        }
        let first = focused(&mut a, "nato-import-focus").unwrap();
        let focused_state = a.clone();
        assert_eq!(focused(&mut a, "nato-import-focus").unwrap(), first);
        assert_eq!(a, focused_state);
        let lesson = api_mutate(
            &mut a,
            "/api/v1/lessons/start",
            "nato-import-lesson",
            json!({"starter":"nato"}),
        )
        .unwrap();
        let lesson_state = a.clone();
        assert_eq!(
            api_mutate(
                &mut a,
                "/api/v1/lessons/start",
                "nato-import-lesson",
                json!({"starter":"nato"})
            )
            .unwrap(),
            lesson
        );
        assert_eq!(a, lesson_state);
        assert_eq!(a.library, library);
        assert_eq!(a.practice.studies.len(), 1);
        let retained = &a.practice.studies[&study];
        assert_eq!(
            retained.version_hash,
            before.practice.studies[&study].version_hash
        );
        assert_eq!(retained.pace, before.practice.studies[&study].pace);
        assert_eq!(
            retained.created_ms,
            before.practice.studies[&study].created_ms
        );
        assert!(retained.nato_all_letters);
        assert_eq!(a.practice.cards, before.practice.cards);
        assert_eq!(a.practice.attempts, before.practice.attempts);
        assert_eq!(a.practice.current, before.practice.current);
        assert_eq!(recovered(&a), a);
        let stored = persistence::rows(&a).unwrap();
        assert_eq!(persistence::app_from_rows(&stored).unwrap(), a);
        assert!(a.jobs.is_empty() && a.spend.is_empty());
        engine::validate_app(&a).unwrap();
    }
}
#[test]
fn nato_identity_requires_all_ordered_correct_pairs_strict_policy_and_consistent_text() {
    let mut cases = vec![];
    for (index, wrong) in [(0, "Alpha"), (9, "Juliet"), (23, "Xray"), (1, "Alfa")] {
        let mut m = manifest("canonical");
        m["pairs"][index]["value"] = json!(wrong);
        cases.push(m);
    }
    let mut m = manifest("canonical");
    m["pairs"].as_array_mut().unwrap().swap(0, 1);
    cases.push(m);
    let mut m = manifest("canonical");
    m["pairs"].as_array_mut().unwrap().pop();
    cases.push(m);
    let mut m = manifest("canonical");
    m["pairs"]
        .as_array_mut()
        .unwrap()
        .push(json!({"key":"27","value":"Extra"}));
    cases.push(m);
    let mut m = manifest("canonical");
    m["spelling_strict"] = json!(false);
    cases.push(m);
    for m in cases {
        let mut a = app();
        let vid = api_mutate(
            &mut a,
            "/api/v1/library/mappings",
            "nato-negative-import",
            json!({"data":m}),
        )
        .unwrap()
        .entity_id
        .unwrap();
        assert!(!library::is_nato(&a.library.versions[&vid]));
        let before = a.clone();
        if !a.library.versions[&vid].spelling_strict {
            assert_eq!(
                focused(&mut a, "nato-negative-focus").unwrap_err().status,
                409
            );
            assert_eq!(a, before);
        }
        assert_eq!(
            api_mutate(
                &mut a,
                "/api/v1/lessons/start",
                "nato-negative-lesson",
                json!({"version_id":vid})
            )
            .unwrap_err()
            .status,
            422
        );
        assert_eq!(a, before);
    }
    let mut a = app();
    let vid = a
        .library
        .add(library::nato_input().unwrap(), "identity-check", None)
        .unwrap();
    let original = a.library.versions[&vid].clone();
    let mut v = original.clone();
    v.kind = library::Kind::Text;
    assert!(!library::is_nato(&v));
    let mut v = original.clone();
    v.text = v.text.replace("Alfa", "Alpha");
    assert!(!library::is_nato(&v));
    let mut v = original.clone();
    v.sections[0].units[0].text.pop();
    v.text = v
        .sections
        .iter()
        .flat_map(|s| &s.units)
        .map(|u| u.text.as_str())
        .collect();
    assert!(
        !library::is_nato(&v),
        "Non-final pairs need structural line endings"
    );
    let mut v = original;
    v.sections[0].units[0].text = "A\tAlfa\tExtra\n".into();
    v.text = v
        .sections
        .iter()
        .flat_map(|s| &s.units)
        .map(|u| u.text.as_str())
        .collect();
    assert!(!library::is_nato(&v));
}
