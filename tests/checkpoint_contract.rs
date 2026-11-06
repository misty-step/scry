use scry::{engine, model::*, persistence};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;
fn mutation(
    app: &mut App,
    path: &str,
    op: &str,
    values: &[(&str, &str)],
) -> AppResult<MutationResult> {
    let mut fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), op.into()),
    ]);
    fields.extend(values.iter().map(|(k, v)| ((*k).into(), (*v).into())));
    engine::mutate(app, path, &fields, NOW, op)
}
fn presented() -> App {
    let mut app = App::new("synthetic-csrf-at-least-24-characters".into());
    mutation(
        &mut app,
        "/practice/nato/start",
        "checkpoint-start-00001",
        &[],
    )
    .unwrap();
    app
}

#[test]
fn checkpoint_is_explicit_idempotent_and_not_a_grade() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    let before = app.clone();
    let values = [
        ("prompt_id", prompt.as_str()),
        ("generation", "0"),
        ("answer", "synthetic partial answer"),
    ];
    let result = mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-save-000001",
        &values,
    )
    .unwrap();
    let stored = persistence::rows(&app).unwrap();
    let reloaded = persistence::app_from_rows(&stored).unwrap();
    assert_eq!(app, reloaded);
    assert_eq!(app.practice.attempts, before.practice.attempts);
    assert_eq!(app.practice.cards, before.practice.cards);
    assert_eq!(app.practice.current, before.practice.current);
    let receipt = serde_json::to_string(&app.operations["checkpoint-save-000001"]).unwrap();
    assert!(!receipt.contains("synthetic partial answer"));
    let committed = app.clone();
    assert_eq!(
        mutation(
            &mut app,
            "/practice/draft",
            "checkpoint-save-000001",
            &values
        )
        .unwrap(),
        result
    );
    assert_eq!(app, committed);
    assert_eq!(
        mutation(
            &mut app,
            "/practice/draft",
            "checkpoint-stale-00001",
            &values
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, committed);
}

#[test]
fn generations_fence_stale_tabs_and_answer_clears_only_the_draft() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    let save1 = [
        ("prompt_id", prompt.as_str()),
        ("generation", "0"),
        ("answer", "first"),
    ];
    mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-save-000001",
        &save1,
    )
    .unwrap();
    let save2 = [
        ("prompt_id", prompt.as_str()),
        ("generation", "1"),
        ("answer", "second"),
    ];
    mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-save-000002",
        &save2,
    )
    .unwrap();
    let latest = app.clone();
    mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-save-000001",
        &save1,
    )
    .unwrap();
    assert_eq!(
        app, latest,
        "Replaying the older acknowledgment must not restore old text."
    );
    assert_eq!(
        mutation(
            &mut app,
            "/practice/draft",
            "checkpoint-stale-00001",
            &save1
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, latest);
    let archive = persistence::Archive::new(app.clone(), vec![], NOW);
    let bytes = archive.encode().unwrap();
    let restored = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(restored.app, app);
    mutation(
        &mut app,
        "/practice/answer",
        "checkpoint-answer-0001",
        &[("prompt_id", &prompt), ("answer", "alfa")],
    )
    .unwrap();
    let checkpoint = app.practice.answer_checkpoint.as_ref().unwrap();
    assert!(checkpoint.draft.is_none());
    assert_eq!(checkpoint.generation, 2);
    assert!(
        persistence::rows(&app)
            .unwrap()
            .contains_key("checkpoint_compatibility/v1")
    );
    let bytes = persistence::Archive::new(app.clone(), vec![], NOW)
        .encode()
        .unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        app
    );
}

#[test]
fn deliberate_switch_clears_atomically_and_failed_switch_does_not() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-save-000001",
        &[
            ("prompt_id", &prompt),
            ("generation", "0"),
            ("answer", "retained"),
        ],
    )
    .unwrap();
    let before = app.clone();
    assert!(
        mutation(
            &mut app,
            "/practice/lesson/start",
            "checkpoint-bad-switch-1",
            &[("leave", "wrong-prompt"), ("starter", "st-michael-en")]
        )
        .is_err()
    );
    assert_eq!(app, before);
    mutation(
        &mut app,
        "/practice/lesson/start",
        "checkpoint-switch-0001",
        &[("leave", &prompt), ("starter", "st-michael-en")],
    )
    .unwrap();
    assert!(
        app.practice
            .answer_checkpoint
            .as_ref()
            .unwrap()
            .draft
            .is_none()
    );
    assert_ne!(app.practice.current.as_ref().unwrap().prompt.id, prompt);
    assert!(app.practice.attempts.is_empty());
}

#[test]
fn saved_answer_rendering_is_escaped_and_only_matches_current_prompt() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-save-000001",
        &[
            ("prompt_id", &prompt),
            ("generation", "0"),
            ("answer", "<synthetic & answer>"),
        ],
    )
    .unwrap();
    let html = scry::web::render(&app, "/", &Default::default(), NOW, None);
    assert!(html.contains("&lt;synthetic &amp; answer&gt;"));
    assert!(!html.contains("<synthetic & answer>"));
}

#[test]
#[ignore = "requires a dedicated synthetic compatibility fixture directory"]
fn export_synthetic_compatibility_fixtures_when_requested() {
    let path = std::env::var("SCRY_CHECKPOINT_FIXTURE_DIR").expect("synthetic fixture directory");
    let mut app = presented();
    let baseline = persistence::rows(&app).unwrap();
    assert!(!baseline.contains_key("checkpoint_compatibility/v1"));
    std::fs::write(
        format!("{path}/baseline-rows.json"),
        serde_json::to_vec(&baseline).unwrap(),
    )
    .unwrap();
    let baseline_archive = persistence::Archive::new(app.clone(), vec![], NOW)
        .encode()
        .unwrap();
    std::fs::write(format!("{path}/baseline-archive.json"), baseline_archive).unwrap();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-save-000001",
        &[
            ("prompt_id", &prompt),
            ("generation", "0"),
            ("answer", "synthetic checkpoint"),
        ],
    )
    .unwrap();
    for suffix in ["saved", "cleared"] {
        if suffix == "cleared" {
            mutation(
                &mut app,
                "/practice/draft/discard",
                "checkpoint-discard-0001",
                &[("prompt_id", &prompt), ("generation", "1")],
            )
            .unwrap();
            assert!(
                app.practice.attempts.is_empty(),
                "Only checkpoint receipts remain."
            );
        }
        let rows = serde_json::to_vec(&persistence::rows(&app).unwrap()).unwrap();
        std::fs::write(format!("{path}/{suffix}-rows.json"), rows).unwrap();
        let archive = persistence::Archive::new(app.clone(), vec![], NOW)
            .encode()
            .unwrap();
        std::fs::write(format!("{path}/{suffix}-archive.json"), archive).unwrap();
    }
}

#[test]
fn reveal_does_not_discard_a_saved_answer_for_the_same_prompt() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-save-000001",
        &[
            ("prompt_id", &prompt),
            ("generation", "0"),
            ("answer", "retained after reveal"),
        ],
    )
    .unwrap();
    mutation(
        &mut app,
        "/practice/reveal",
        "checkpoint-reveal-0001",
        &[("prompt_id", &prompt)],
    )
    .unwrap();
    assert_eq!(
        app.practice
            .answer_checkpoint
            .as_ref()
            .unwrap()
            .draft
            .as_ref()
            .unwrap()
            .answer,
        "retained after reveal"
    );
    persistence::Archive::new(app, vec![], NOW)
        .encode()
        .unwrap();
}

#[test]
fn unsupported_control_characters_are_refused_without_a_receipt() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    for answer in ["line\rbreak", "null\0byte", "mapping\nnewline"] {
        let before = app.clone();
        let result = mutation(
            &mut app,
            "/practice/draft",
            "checkpoint-refusal-001",
            &[
                ("prompt_id", &prompt),
                ("generation", "0"),
                ("answer", answer),
            ],
        );
        assert_eq!(result.unwrap_err().status, 422);
        assert_eq!(app, before);
    }
}

#[test]
fn switch_winning_a_race_refuses_the_old_prompt_save() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-save-000001",
        &[
            ("prompt_id", &prompt),
            ("generation", "0"),
            ("answer", "saved before switch"),
        ],
    )
    .unwrap();
    mutation(
        &mut app,
        "/practice/lesson/start",
        "checkpoint-switch-0001",
        &[("leave", &prompt), ("starter", "st-michael-en")],
    )
    .unwrap();
    let switched = app.clone();
    assert_eq!(
        mutation(
            &mut app,
            "/practice/draft",
            "checkpoint-late-save-1",
            &[
                ("prompt_id", &prompt),
                ("generation", "1"),
                ("answer", "late")
            ]
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, switched);
}

#[test]
fn archive_rejects_checkpoint_receipts_that_contain_text() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-save-000001",
        &[
            ("prompt_id", &prompt),
            ("generation", "0"),
            ("answer", "synthetic"),
        ],
    )
    .unwrap();
    app.operations
        .get_mut("checkpoint-save-000001")
        .unwrap()
        .result
        .data
        .as_mut()
        .unwrap()["answer"] = serde_json::json!("synthetic");
    assert!(
        persistence::Archive::new(app, vec![], NOW)
            .encode()
            .is_err()
    );
}

#[test]
fn browser_draft_transport_normalizes_crlf_without_relaxing_domain_rules() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    let mut fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "checkpoint-newline-0001".into()),
        ("prompt_id".into(), prompt),
        ("generation".into(), "0".into()),
        ("answer".into(), "\r\nfirst\r\nsecond".into()),
    ]);
    scry::api::normalize_draft_form(&mut fields);
    assert_eq!(fields["answer"], "\nfirst\nsecond");
    // The NATO fixture is a single-line mapping: the domain must still reject LF.
    let before = app.clone();
    assert_eq!(
        engine::mutate(
            &mut app,
            "/practice/draft",
            &fields,
            NOW,
            "checkpoint-newline-0001"
        )
        .unwrap_err()
        .status,
        422
    );
    assert_eq!(app, before);
    fields.insert("answer".into(), "bare\rreturn".into());
    scry::api::normalize_draft_form(&mut fields);
    assert_eq!(fields["answer"], "bare\rreturn");
    assert_eq!(
        engine::mutate(
            &mut app,
            "/practice/draft",
            &fields,
            NOW,
            "checkpoint-newline-0001"
        )
        .unwrap_err()
        .status,
        422
    );
}

#[test]
fn multiline_browser_transport_keeps_leading_and_internal_newlines_and_replays() {
    let mut app = App::new("synthetic-csrf-at-least-24-characters".into());
    mutation(
        &mut app,
        "/practice/lesson/start",
        "checkpoint-rosary-start",
        &[("starter", "rosary-joyful")],
    )
    .unwrap();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    let mut fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "checkpoint-newline-save".into()),
        ("prompt_id".into(), prompt),
        ("generation".into(), "0".into()),
        ("answer".into(), "\r\nfirst\r\nsecond".into()),
    ]);
    scry::api::normalize_draft_form(&mut fields);
    let result = engine::mutate(
        &mut app,
        "/practice/draft",
        &fields,
        NOW,
        "checkpoint-newline-save",
    )
    .unwrap();
    assert_eq!(
        app.practice
            .answer_checkpoint
            .as_ref()
            .unwrap()
            .draft
            .as_ref()
            .unwrap()
            .answer,
        "\nfirst\nsecond"
    );
    let saved = app.clone();
    assert_eq!(
        engine::mutate(
            &mut app,
            "/practice/draft",
            &fields,
            NOW,
            "checkpoint-newline-save"
        )
        .unwrap(),
        result
    );
    assert_eq!(app, saved);
}

#[test]
fn discard_is_prompt_and_generation_fenced_and_replayable() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-discard-save",
        &[
            ("prompt_id", &prompt),
            ("generation", "0"),
            ("answer", "saved text"),
        ],
    )
    .unwrap();
    let before = app.clone();
    assert_eq!(
        mutation(
            &mut app,
            "/practice/draft/discard",
            "checkpoint-discard-stale",
            &[("prompt_id", &prompt), ("generation", "0")]
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, before);
    let fields = [("prompt_id", prompt.as_str()), ("generation", "1")];
    let result = mutation(
        &mut app,
        "/practice/draft/discard",
        "checkpoint-discard-ok",
        &fields,
    )
    .unwrap();
    let checkpoint = app.practice.answer_checkpoint.as_ref().unwrap();
    assert_eq!(checkpoint.generation, 2);
    assert!(checkpoint.draft.is_none());
    assert!(app.practice.attempts.is_empty());
    assert!(
        persistence::rows(&app)
            .unwrap()
            .contains_key("checkpoint_compatibility/v1")
    );
    let after = app.clone();
    assert_eq!(
        mutation(
            &mut app,
            "/practice/draft/discard",
            "checkpoint-discard-ok",
            &fields
        )
        .unwrap(),
        result
    );
    assert_eq!(app, after);
    assert_eq!(
        mutation(
            &mut app,
            "/practice/draft/discard",
            "checkpoint-discard-other",
            &fields
        )
        .unwrap_err()
        .status,
        409
    );
}

#[test]
fn archives_reject_checkpoint_text_that_save_would_refuse() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-text-save-01",
        &[
            ("prompt_id", &prompt),
            ("generation", "0"),
            ("answer", "valid"),
        ],
    )
    .unwrap();
    for text in ["bad\rreturn", "bad\0null", "single\nline"] {
        let mut damaged = app.clone();
        damaged
            .practice
            .answer_checkpoint
            .as_mut()
            .unwrap()
            .draft
            .as_mut()
            .unwrap()
            .answer = text.into();
        assert!(
            persistence::Archive::new(damaged, vec![], NOW)
                .encode()
                .is_err()
        );
    }
}

#[test]
fn old_discard_replay_cannot_erase_a_newer_saved_draft() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    mutation(
        &mut app,
        "/practice/draft",
        "old-discard-start-save",
        &[
            ("prompt_id", &prompt),
            ("generation", "0"),
            ("answer", "first"),
        ],
    )
    .unwrap();
    let discard = [("prompt_id", prompt.as_str()), ("generation", "1")];
    mutation(
        &mut app,
        "/practice/draft/discard",
        "old-discard-operation",
        &discard,
    )
    .unwrap();
    mutation(
        &mut app,
        "/practice/draft",
        "newer-save-operation",
        &[
            ("prompt_id", &prompt),
            ("generation", "2"),
            ("answer", "newer"),
        ],
    )
    .unwrap();
    let before = app.clone();
    mutation(
        &mut app,
        "/practice/draft/discard",
        "old-discard-operation",
        &discard,
    )
    .unwrap();
    assert_eq!(app, before);
    assert_eq!(
        mutation(
            &mut app,
            "/practice/draft/discard",
            "different-discard-op",
            &discard
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, before);
    assert_eq!(
        app.practice
            .answer_checkpoint
            .as_ref()
            .unwrap()
            .draft
            .as_ref()
            .unwrap()
            .answer,
        "newer"
    );
}

#[test]
fn incoming_tampered_checkpoint_archive_is_refused() {
    let mut app = presented();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    mutation(
        &mut app,
        "/practice/draft",
        "checkpoint-tamper-save",
        &[
            ("prompt_id", &prompt),
            ("generation", "0"),
            ("answer", "valid"),
        ],
    )
    .unwrap();
    let original = persistence::Archive::new(app, vec![], NOW)
        .encode()
        .unwrap();
    for text in ["bad\rreturn", "bad\0null", "single\nline"] {
        let mut archive: serde_json::Value = serde_json::from_slice(&original).unwrap();
        archive["app"]["practice"]["answer_checkpoint"]["draft"]["answer"] =
            serde_json::json!(text);
        let bytes = serde_json::to_vec(&archive).unwrap();
        assert!(persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).is_err());
    }
}
