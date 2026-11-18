//! The stored earlier concept notebook, for tests whose setup once drove its
//! deleted practice. `tests/fixtures/earlier-notebook.json` is the only source
//! of these states; see its README.
use scry::{model::App, persistence::Archive};

/// One recorded state, decoded against its hash, and the clock it was saved
/// at. `question-cold-choice` is the retired authored demo exactly as it left
/// the notebook: one Goal, its published Concepts and questions, and the
/// choice question held unanswered.
pub fn earlier(name: &str) -> (App, i64) {
    let fixture: serde_json::Value = serde_json::from_str(
        &std::fs::read_to_string(concat!(
            env!("CARGO_MANIFEST_DIR"),
            "/tests/fixtures/earlier-notebook.json"
        ))
        .unwrap(),
    )
    .unwrap();
    let state = &fixture[name];
    let archive = Archive::decode(
        state["archive"].as_str().unwrap().as_bytes(),
        state["sha256"].as_str().unwrap(),
    )
    .unwrap();
    (archive.app, state["now_ms"].as_i64().unwrap())
}
