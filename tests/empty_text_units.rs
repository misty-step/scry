//! New admission is stricter than immutable pre-fix history and receipt replay.
use scry::{engine, grading, model::*, persistence, study};
use serde_json::{Value, json};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
fn app() -> App {
    App::new("synthetic-empty-unit-csrf-32-chars".into())
}
fn fields(a: &App, extra: &[(&str, String)]) -> BTreeMap<String, String> {
    let mut f = BTreeMap::from([
        ("csrf".into(), a.csrf.clone()),
        ("revision".into(), a.revision.to_string()),
        (
            "operation_id".into(),
            format!("empty-unit-operation-{:016}", a.revision),
        ),
    ]);
    f.extend(extra.iter().map(|(k, v)| (k.to_string(), v.clone())));
    f
}
fn call(a: &mut App, path: &str, extra: &[(&str, String)]) -> MutationResult {
    let f = fields(a, extra);
    let result = engine::mutate(a, path, &f, NOW, &f["operation_id"]).unwrap();
    engine::validate_app(a).unwrap();
    result
}
fn reject(a: &mut App, path: &str, extra: &[(&str, String)]) {
    let f = fields(a, extra);
    let saved = a.clone();
    let rows = persistence::rows(a).unwrap();
    // A refusal does not consume an operation or change a draft/held result.
    for _ in 0..2 {
        let error = engine::mutate(a, path, &f, NOW, "refused-empty-unit").unwrap_err();
        assert_eq!(error.status, 422, "{path}: {}", error.message);
        assert!(error.message.contains("no words to recall"));
        assert_eq!(*a, saved, "{path}");
        assert_eq!(persistence::rows(a).unwrap(), rows);
    }
}
fn input(text: &str) -> Value {
    json!({"title":"Synthetic separators","kind":"text","text":text})
}
fn add(a: &mut App, data: Value) -> String {
    call(a, "/library/version/add", &[("data", data.to_string())])
        .entity_id
        .unwrap()
}
fn legacy(state: &str) -> App {
    let fixture: Value =
        serde_json::from_str(include_str!("fixtures/empty-unit-legacy.json")).unwrap();
    assert_eq!(
        fixture["source_commit"],
        "fc3378f9e03e99a5b7bfbcbc7168ededefb7301c"
    );
    let app: App = serde_json::from_value(fixture[state].clone()).unwrap();
    engine::validate_app(&app).unwrap();
    app
}
fn pid(a: &App) -> String {
    a.practice.current.as_ref().unwrap().prompt.id.clone()
}
fn vid(a: &App) -> String {
    a.library.versions.keys().next().unwrap().clone()
}

#[test]
fn automatic_text_segments_preserve_bytes_without_empty_recall_targets() {
    for text in [
        "\nAlpha beta.",
        "... Alpha beta.",
        "\r\n\t...\nAlpha beta.\n...\n",
        "Alpha... Beta?!\n\nGamma; delta.",
        "‘Alpha,’ (beta).",
        "！\nAlpha beta。\n",
        "Alpha beta.\n\n",
        "&",
        "☀",
        "123",
        "Lord's mercy.",
    ] {
        let mut a = app();
        let id = add(&mut a, input(text));
        let v = &a.library.versions[&id];
        let units = v.sections.iter().flat_map(|s| &s.units).collect::<Vec<_>>();
        assert_eq!(
            units.iter().map(|u| u.text.as_str()).collect::<String>(),
            text
        );
        assert!(
            units
                .iter()
                .all(|u| !grading::normalized_words(&u.text).is_empty()),
            "{text:?}"
        );
        call(&mut a, "/practice/lesson/start", &[("version_id", id)]);
        let prompt = pid(&a);
        call(&mut a, "/practice/quiz", &[("prompt_id", prompt)]);
        let prompt = pid(&a);
        let f = fields(&a, &[("prompt_id", prompt), ("answer", String::new())]);
        let r = engine::mutate(&mut a, "/practice/answer", &f, NOW, "empty-is-a-miss").unwrap();
        let attempt = &a.practice.attempts[r.entity_id.as_ref().unwrap()];
        assert_eq!(attempt.grade.verdict, grading::Verdict::Again);
        assert!(
            attempt
                .grade
                .units
                .iter()
                .all(|u| u.verdict == grading::Verdict::Again)
        );
        assert!(attempt.effects.iter().all(|e| e.after.successes == 0));
        assert!(!attempt.assisted);
        let saved = a.clone();
        assert_eq!(
            engine::mutate(&mut a, "/practice/answer", &f, NOW + 1, "replay").unwrap(),
            r
        );
        assert_eq!(a, saved);
        engine::validate_app(&a).unwrap();
    }
}

#[test]
fn explicit_empty_normalized_units_are_rejected_without_a_receipt() {
    for separator in [" ", "\n", "...", "？！", "''", "\t\r\n"] {
        let mut a = app();
        let data = json!({"title":"Synthetic boundaries","kind":"text",
            "text":format!("{separator}Alpha beta."),
            "sections":[{"heading":"Passage","units":[separator,"Alpha beta."]}]});
        reject(
            &mut a,
            "/library/version/add",
            &[("data", data.to_string())],
        );
        // The same unconsumed operation accepts corrected explicit boundaries.
        let joined = format!("{separator}Alpha beta.");
        let data = json!({"title":"Synthetic boundaries","kind":"text","text":joined,
            "sections":[{"heading":"Passage","units":[joined]}]});
        let id = add(&mut a, data);
        call(&mut a, "/practice/lesson/start", &[("version_id", id)]);
        let prompt = pid(&a);
        call(&mut a, "/practice/quiz", &[("prompt_id", prompt)]);
        let prompt = pid(&a);
        call(
            &mut a,
            "/practice/answer",
            &[("prompt_id", prompt), ("answer", "ALPHA BETA!".into())],
        );
        assert_eq!(
            a.practice.attempts.values().next().unwrap().grade.verdict,
            grading::Verdict::Good
        );
    }
}

#[test]
fn legacy_entry_and_reimport_do_not_repair_or_enroll_bad_boundaries() {
    for path in [
        "/practice/learn",
        "/practice/study/start",
        "/practice/lesson/start",
    ] {
        let mut a = legacy("unstarted");
        let id = vid(&a);
        reject(&mut a, path, &[("version_id", id)]);
        assert!(a.practice.studies.is_empty());
    }
    let mut a = legacy("unstarted");
    let saved = a.library.clone();
    let id = add(
        &mut a,
        json!({"title":"Synthetic legacy separators","kind":"text","text":"Alpha beta."}),
    );
    assert_eq!(
        a.library, saved,
        "Dedupe is not permission to rewrite old boundaries."
    );
    reject(&mut a, "/practice/lesson/start", &[("version_id", id)]);
}

#[test]
fn retained_source_quiz_and_phase_less_answers_cannot_create_new_evidence() {
    let mut source = legacy("source");
    let prompt = pid(&source);
    reject(&mut source, "/practice/quiz", &[("prompt_id", prompt)]);
    for state in ["source", "quiz", "whole_quiz"] {
        for phase_less in [false, true] {
            let mut a = legacy(state);
            if phase_less {
                a.practice.current.as_mut().unwrap().phase = None;
            }
            engine::validate_app(&a).unwrap();
            let prompt = pid(&a);
            for answer in ["", "...", "Alpha beta."] {
                reject(
                    &mut a,
                    "/practice/answer",
                    &[("prompt_id", prompt.clone()), ("answer", answer.into())],
                );
            }
            reject(&mut a, "/practice/reveal", &[("prompt_id", prompt)]);
            reject(&mut a, "/practice/next", &[]);
            reject(&mut a, "/practice/review/start", &[]);
        }
    }
}

#[test]
fn retained_card_section_whole_and_all_lesson_modes_are_fenced() {
    let mut a = legacy("answered");
    let study_id = a.practice.studies.keys().next().unwrap().clone();
    let after = pid(&a);
    let card_id = a.practice.cards.keys().next().unwrap().clone();
    let section_id = a.library.versions[&vid(&a)].sections[0].id.clone();
    for extra in [
        vec![("study_id", study_id.clone())],
        vec![("card_id", card_id)],
        vec![("study_id", study_id.clone()), ("recite", "whole".into())],
        vec![
            ("study_id", study_id),
            ("recite", "section".into()),
            ("section_id", section_id),
        ],
    ] {
        let mut f = extra;
        f.push(("after", after.clone()));
        reject(&mut a, "/practice/next", &f);
    }
    for mode in ["", "phrases", "retry", "join", "whole", "middle"] {
        reject(
            &mut a,
            "/practice/lesson/next",
            &[("after", after.clone()), ("mode", mode.into())],
        );
    }
}

#[test]
fn automatic_selection_skips_legacy_versions_but_keeps_other_work_available() {
    let mut a = legacy("answered");
    let bad = vid(&a);
    let original = a.practice.attempts.clone();
    let cards = a.practice.cards.clone();
    assert!(study::mixed_cards(&a, NOW + 60 * DAY_MS).is_empty());
    assert!(
        study::due(&a, NOW + 60 * DAY_MS)["due"]
            .as_array()
            .unwrap()
            .is_empty()
    );
    assert!(!study::study_ready(
        &a,
        a.practice.studies.values().next().unwrap(),
        NOW + 60 * DAY_MS
    ));
    let good = add(
        &mut a,
        json!({"title":"Synthetic valid work","kind":"text","text":"Gamma delta."}),
    );
    call(&mut a, "/practice/learn", &[("version_id", good.clone())]);
    let after = pid(&a);
    call(&mut a, "/practice/next", &[("after", after)]);
    assert_eq!(a.practice.current.as_ref().unwrap().prompt.version_id, good);
    assert_eq!(a.practice.attempts, original);
    for (id, card) in cards {
        assert_eq!(a.practice.cards[&id], card);
    }
    let mut a = legacy("source");
    let leave = pid(&a);
    call(&mut a, "/practice/review/start", &[("leave", leave)]);
    assert_ne!(a.practice.current.as_ref().unwrap().prompt.version_id, bad);
    assert!(a.practice.attempts.is_empty());
}

#[test]
fn legacy_rows_archives_grades_and_exact_receipts_remain_unchanged() {
    let fixture: Value =
        serde_json::from_str(include_str!("fixtures/empty-unit-legacy.json")).unwrap();
    for (state, request, path) in [
        ("unstarted", "import", "/library/version/add"),
        ("source", "start", "/practice/lesson/start"),
        ("quiz", "quiz", "/practice/quiz"),
        ("answered", "answer", "/practice/answer"),
    ] {
        let mut a = legacy(state);
        let rows = persistence::rows(&a).unwrap();
        assert_eq!(persistence::app_from_rows(&rows).unwrap(), a);
        let bytes = persistence::Archive::new(a.clone(), vec![], NOW)
            .encode()
            .unwrap();
        assert_eq!(
            persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
                .unwrap()
                .app,
            a
        );
        let f: BTreeMap<String, String> =
            serde_json::from_value(fixture["requests"][request].clone()).unwrap();
        let receipt = a.operations[&f["operation_id"]].result.clone();
        assert_eq!(
            engine::mutate(&mut a, path, &f, NOW + 1, "historical-replay").unwrap(),
            receipt
        );
        assert_eq!(persistence::rows(&a).unwrap(), rows);
        assert_eq!(
            persistence::Archive::new(a, vec![], NOW).encode().unwrap(),
            bytes
        );
    }
    let a = legacy("answered");
    let original = a.practice.attempts.values().next().unwrap();
    assert_eq!(
        original.grade.verdict,
        grading::Verdict::Good,
        "The pre-fix defect remains historical evidence, not a rewritten result."
    );
    assert!(original.effects[0].scheduled);
    assert!(
        study::history(&a).unwrap()["attempts"]
            .get(&original.id)
            .is_some()
    );
}

#[test]
fn rejection_retains_an_acknowledged_draft_for_a_legacy_quiz() {
    let mut a = legacy("quiz");
    let prompt = pid(&a);
    call(
        &mut a,
        "/practice/draft",
        &[
            ("prompt_id", prompt.clone()),
            ("generation", "0".into()),
            ("answer", "Keep this acknowledged answer.".into()),
        ],
    );
    let page = scry::web::render(&a, "/recall", &BTreeMap::new(), NOW, None);
    assert!(page.contains("id=\"unavailable-draft\" readonly"));
    assert!(page.contains("Keep this acknowledged answer."));
    reject(
        &mut a,
        "/practice/answer",
        &[("prompt_id", prompt), ("answer", String::new())],
    );
    assert_eq!(
        a.practice
            .answer_checkpoint
            .as_ref()
            .unwrap()
            .draft
            .as_ref()
            .unwrap()
            .answer,
        "Keep this acknowledged answer."
    );
}

#[test]
fn quarantined_versions_render_honest_unavailability_and_fenced_escape() {
    let a = legacy("unstarted");
    let page = scry::web::render(
        &a,
        &format!("/library/versions/{}", vid(&a)),
        &BTreeMap::new(),
        NOW,
        None,
    );
    assert!(page.contains("Practice is unavailable"));
    assert!(!page.contains("action=\"/practice/study/start\""));
    assert!(
        page.contains("Alpha beta."),
        "Saved text is still readable."
    );
    let shelf = scry::web::render(&a, "/lessons", &BTreeMap::new(), NOW, None);
    assert!(shelf.contains("Practice unavailable"));
    assert!(!shelf.contains(&format!("name=\"version_id\" value=\"{}\"", vid(&a))));

    for state in ["source", "quiz", "whole_quiz", "answered"] {
        let mut a = legacy(state);
        let before = persistence::rows(&a).unwrap();
        let prompt = pid(&a);
        let fence = if state == "answered" {
            "after"
        } else {
            "leave"
        };
        for route in ["/", "/recall"] {
            let page = scry::web::render(&a, route, &BTreeMap::new(), NOW, None);
            assert!(
                page.contains("data-state=\"practice-unavailable\""),
                "{state} {route}"
            );
            assert!(!page.contains("action=\"/practice/answer\""));
            assert!(!page.contains("action=\"/practice/quiz\""));
            assert!(page.contains(&format!("name=\"{fence}\" value=\"{prompt}\"")));
            assert!(page.contains("Study other material"));
        }
        let shelf = scry::web::render(&a, "/lessons", &BTreeMap::new(), NOW, None);
        let form = shelf
            .split("action=\"/practice/review/start\"")
            .nth(1)
            .unwrap()
            .split("</form>")
            .next()
            .unwrap();
        assert!(form.contains(&format!("name=\"{fence}\" value=\"{prompt}\"")));
        assert_eq!(persistence::rows(&a).unwrap(), before);
        let history = a.practice.attempts.clone();
        call(&mut a, "/practice/review/start", &[(fence, prompt)]);
        assert_ne!(
            vid(&legacy(state)),
            a.practice.current.as_ref().unwrap().prompt.version_id
        );
        assert_eq!(a.practice.attempts, history);
    }
}
