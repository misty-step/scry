//! A deliberate schedule reset of the earlier concept notebook, as stored. No
//! route can dispute or reset a question any more; `history-rich` in
//! `tests/fixtures/earlier-notebook.json` holds one, and restore must keep it
//! as the only authority for that question's schedule.
use scry::{
    engine, learning,
    model::*,
    persistence::{self, ArchivedPhoto},
};

/// `history-rich` as stored, with the photo its archive carries.
fn stored() -> (App, Vec<ArchivedPhoto>, i64, Reset) {
    let fixture: serde_json::Value = serde_json::from_str(
        &std::fs::read_to_string(concat!(
            env!("CARGO_MANIFEST_DIR"),
            "/tests/fixtures/earlier-notebook.json"
        ))
        .unwrap(),
    )
    .unwrap();
    let state = &fixture["history-rich"];
    let archive = persistence::Archive::decode(
        state["archive"].as_str().unwrap().as_bytes(),
        state["sha256"].as_str().unwrap(),
    )
    .unwrap();
    assert_eq!(archive.app.resets.len(), 1);
    let reset = archive.app.resets[0].clone();
    (
        archive.app,
        archive.photos,
        state["now_ms"].as_i64().unwrap(),
        reset,
    )
}

#[test]
fn a_stored_reset_is_self_contained_and_its_replay_is_refused() {
    let (app, photos, now, reset) = stored();
    engine::validate_app(&app).unwrap();
    // The reset starts a new card on the next schedule epoch and names the
    // dispute that asked for it.
    assert_eq!(reset.after_card, learning::new_card(reset.created_ms));
    assert_eq!(
        reset.schedule_version_after,
        reset.schedule_version_before + 1
    );
    assert!(
        app.feedback
            .iter()
            .any(|f| f.id == reset.feedback_id && f.kind == "dispute")
    );
    // Its receipt is kept, and replaying it is refused before anything runs.
    let (_, receipt) = app
        .operations
        .iter()
        .find(|(_, r)| {
            r.path.as_deref() == Some(&format!("/questions/{}/dispute", reset.question_id))
        })
        .unwrap();
    assert_eq!(
        engine::admit_current_mutation(receipt.path.as_deref().unwrap())
            .unwrap_err()
            .status,
        410
    );
    let rows = persistence::rows(&app).unwrap();
    assert!(rows.keys().any(|key| key.starts_with("resets/")));
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), app);
    let bytes = persistence::Archive::new(app.clone(), photos.clone(), now + 5)
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
fn missing_or_tampered_schedule_history_cannot_become_restore_authority() {
    let (app, photos, now, reset) = stored();
    let question_id = reset.question_id.clone();
    engine::validate_app(&app).unwrap();
    // The untampered notebook is a complete archive, so each refusal below is
    // the tampering's.
    persistence::Archive::new(app.clone(), photos.clone(), now + 5)
        .encode()
        .unwrap();
    let mut variants = Vec::new();
    let mut corrupt = app.clone();
    corrupt.resets.clear();
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt.resets[0].before_card.reps += 1;
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt.resets[0].after_card.due_ms += 1;
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt.resets[0].feedback_id = "missing-dispute".into();
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt.resets[0].content_version = 99;
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt.resets[0].source_revision = 99;
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt.questions.get_mut(&question_id).unwrap().card.due_ms += 1;
    variants.push(corrupt);
    let mut corrupt = app.clone();
    corrupt
        .questions
        .get_mut(&question_id)
        .unwrap()
        .schedule_version += 1;
    variants.push(corrupt);
    for corrupt in variants {
        assert!(engine::validate_app(&corrupt).is_err());
        assert!(
            persistence::Archive::new(corrupt, photos.clone(), now + 5)
                .encode()
                .is_err()
        );
    }
}
