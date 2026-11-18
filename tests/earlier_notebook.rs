//! The earlier concept notebook in every state an existing notebook can hold,
//! kept as stored bytes: `tests/fixtures/earlier-notebook.json`.
//!
//! No route or engine function can produce these states any more, so the
//! fixture is their only source. These tests read only the fixture: existing
//! notebooks decode, validate and re-encode byte for byte. See
//! `tests/fixtures/earlier-notebook.README.md`.
use scry::{
    engine, learning,
    model::*,
    persistence::{self, Archive},
};
use serde_json::Value;
use std::collections::{BTreeMap, BTreeSet};

const FIXTURE: &str = concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/tests/fixtures/earlier-notebook.json"
);

struct Stored {
    archive: Vec<u8>,
    sha256: String,
    bytes: usize,
    format: String,
    now_ms: i64,
    rows: BTreeMap<String, String>,
}
impl Stored {
    fn decoded(&self) -> Archive {
        Archive::decode(&self.archive, &self.sha256).unwrap()
    }
    fn app(&self) -> App {
        self.decoded().app
    }
}

fn committed() -> BTreeMap<String, Stored> {
    let fixture: BTreeMap<String, Value> =
        serde_json::from_str(&std::fs::read_to_string(FIXTURE).unwrap()).unwrap();
    fixture
        .into_iter()
        .map(|(name, v)| {
            let stored = Stored {
                archive: v["archive"].as_str().unwrap().as_bytes().to_vec(),
                sha256: v["sha256"].as_str().unwrap().into(),
                bytes: v["bytes"].as_u64().unwrap() as usize,
                format: v["format"].as_str().unwrap().into(),
                now_ms: v["now_ms"].as_i64().unwrap(),
                rows: serde_json::from_value(v["rows"].clone()).unwrap(),
            };
            (name, stored)
        })
        .collect()
}

/// Stored archive bytes decode against their recorded hash and encode again to
/// the same bytes, hash, length and format name; stored rows become the same
/// notebook and the same rows again.
#[test]
fn every_state_round_trips_byte_for_byte() {
    for (name, stored) in committed() {
        assert_eq!(stored.archive.len(), stored.bytes, "{name}: length");
        assert_eq!(
            persistence::sha256(&stored.archive),
            stored.sha256,
            "{name}: hash"
        );
        let archive = stored.decoded();
        assert_eq!(archive.format, stored.format, "{name}: format");
        assert_eq!(archive.created_ms, stored.now_ms, "{name}: now_ms");
        let again = archive.encode().unwrap();
        assert!(again == stored.archive, "{name}: archive bytes changed");
        assert_eq!(persistence::sha256(&again), stored.sha256, "{name}");
        // The format name is derived from the notebook, not copied from the bytes.
        let photos = archive.photos.clone();
        let fresh = Archive::new(archive.app.clone(), photos, stored.now_ms);
        assert_eq!(fresh.format, stored.format, "{name}: format name");
        assert!(fresh.encode().unwrap() == stored.archive, "{name}");

        let from_rows = persistence::app_from_rows(&stored.rows).unwrap();
        assert!(
            persistence::rows(&from_rows).unwrap() == stored.rows,
            "{name}: rows changed"
        );
        assert!(from_rows == archive.app, "{name}: rows and archive differ");
        assert!(
            persistence::rows(&archive.app).unwrap() == stored.rows,
            "{name}: archive does not store as the recorded rows"
        );
    }
}

#[test]
fn every_state_passes_full_validation() {
    for (name, stored) in committed() {
        let archive = stored.decoded();
        archive.validate().unwrap();
        engine::validate_app(&archive.app).unwrap();
        assert!(
            archive.diagnose().unwrap().is_empty(),
            "{name}: diagnosis reports a refusal"
        );
        let from_rows = persistence::app_from_rows(&stored.rows).unwrap();
        engine::validate_app(&from_rows).unwrap();
    }
}

/// What each state holds, as (phase, assisted, status of the occurrence's
/// check, restored and paused).
fn held(app: &App) -> (String, bool, String, bool) {
    let Some(o) = &app.occurrence else {
        return ("none".into(), false, "none".into(), app.restored_paused);
    };
    let status = o
        .assessment_id
        .as_ref()
        .map(|id| app.assessments[id].status.clone())
        .unwrap_or_else(|| "none".into());
    (o.phase.clone(), o.assisted, status, app.restored_paused)
}

/// The fixture is only worth keeping while each name holds the state it says.
#[test]
fn every_state_holds_what_its_name_says() {
    let states = committed();
    let expected: &[(&str, &str, bool, &str, bool)] = &[
        ("never-practised", "none", false, "none", false),
        ("empty-history", "none", false, "none", false),
        ("intro", "intro", false, "none", false),
        ("question-cold", "question", false, "none", false),
        ("question-cold-choice", "question", false, "none", false),
        ("question-helped", "question", true, "none", false),
        ("checking-queued", "checking", false, "queued", false),
        ("checking-queued-explain", "checking", true, "queued", false),
        ("checking-sent", "checking", false, "sent", false),
        ("self-check-failed", "self-check", false, "failed", false),
        (
            "self-check-failed-unsent",
            "self-check",
            false,
            "failed",
            false,
        ),
        (
            "self-check-failed-send",
            "self-check",
            false,
            "failed",
            false,
        ),
        ("self-check-rejected", "self-check", false, "failed", false),
        ("self-check-unknown", "self-check", false, "unknown", false),
        ("self-check-unsure", "self-check", false, "unsure", false),
        ("self-check-paused", "self-check", false, "paused", true),
        ("restored-sent", "self-check", false, "unknown", true),
        ("queued-while-paused", "checking", false, "queued", true),
        ("three-checks", "self-check", false, "failed", false),
        ("result-held", "result", false, "none", false),
        ("result-jev-correct", "result", false, "complete", false),
        ("result-jev-wrong", "result", false, "complete", false),
        ("result-helped", "result", true, "none", false),
        ("result-corrected", "result", false, "complete", false),
        ("result-revealed", "result", true, "none", false),
        ("result-then-archived", "result", false, "none", false),
        ("result-then-edited", "result", false, "none", false),
        ("go-retained-self-check", "self-check", false, "none", false),
        ("history-rich", "none", false, "none", false),
    ];
    assert_eq!(
        states.keys().map(String::as_str).collect::<BTreeSet<_>>(),
        expected.iter().map(|e| e.0).collect::<BTreeSet<_>>()
    );
    let apps: BTreeMap<&str, App> = states.iter().map(|(n, s)| (n.as_str(), s.app())).collect();
    for (name, phase, assisted, status, restored) in expected {
        assert_eq!(
            held(&apps[name]),
            (phase.to_string(), *assisted, status.to_string(), *restored),
            "{name}"
        );
    }
    let app = |name: &str| &apps[name];
    let due = |name: &str| {
        apps[name]
            .questions
            .values()
            .filter(|q| !q.archived && q.card.due_ms <= states[name].now_ms)
            .count()
    };
    let kind = |a: &App| a.occurrence.as_ref().unwrap().presentation.kind.clone();

    assert!(app("never-practised").events.is_empty() && due("never-practised") > 0);
    assert!(app("empty-history").events.len() == 3 && due("empty-history") > 0);
    let a = app("intro");
    let o = a.occurrence.as_ref().unwrap();
    assert!(a.concepts[&o.concept_id].introduced_ms.is_none() && o.answer.is_none());
    assert_eq!(kind(app("question-cold")), "recall");
    assert_eq!(kind(app("question-cold-choice")), "choice");
    assert!(app("question-cold").protected_goal().is_some());
    assert!(app("question-helped").protected_goal().is_none());

    let a = app("checking-queued");
    let token = a.owner_tokens.values().next().unwrap();
    let check = a.assessments.values().next().unwrap();
    assert!(token.info.paid && check.acting_token.as_ref() == Some(&token.info.id));
    assert!(check.lease.is_none() && a.spend.is_empty());
    let a = app("checking-queued-explain");
    assert_eq!(kind(a), "explain");
    assert_eq!(
        a.assessments.values().last().unwrap().policy,
        learning::SEMANTIC_POLICY
    );

    let a = app("checking-sent");
    let check = a.assessments.values().next().unwrap();
    assert!(check.lease.is_some() && check.started_ms.is_some());
    let reserved = |a: &App| {
        a.spend
            .iter()
            .map(|s| (s.status.clone(), s.cost_micros, s.response_outcome.clone()))
            .collect::<Vec<_>>()
    };
    let unknown = [("unknown".to_string(), None, None)];
    assert_eq!(reserved(a), unknown);
    assert_eq!(
        reserved(app("self-check-failed")),
        [("known".into(), Some(19_500), Some("received".into()))]
    );
    assert!(app("self-check-failed-unsent").spend.is_empty());
    assert_eq!(reserved(app("self-check-failed-send")), unknown);
    assert_eq!(
        reserved(app("self-check-rejected")),
        [("unknown".into(), None, Some("rejected".into()))]
    );
    assert_eq!(reserved(app("self-check-unknown")), unknown);
    assert_eq!(reserved(app("restored-sent")), unknown);
    assert!(app("self-check-paused").spend.is_empty());
    assert!(app("queued-while-paused").spend.is_empty());
    let a = app("three-checks");
    assert!(a.assessments.len() == 3 && a.spend.len() == 3 && a.events.len() == 2);

    let result = |name: &str| {
        let a = app(name);
        let r = a.occurrence.as_ref().unwrap().result.clone().unwrap();
        (r.outcome, r.authority, r.rating, r.corrected)
    };
    let is = |o: &str, a: &str, rating: u8, c: bool| (o.to_string(), a.to_string(), rating, c);
    assert_eq!(result("result-held"), is("correct", "exact", 3, false));
    assert_eq!(result("result-jev-correct"), is("correct", "jev", 3, false));
    assert_eq!(result("result-jev-wrong"), is("wrong", "jev", 1, false));
    assert_eq!(
        result("result-helped"),
        is("warm_correct", "exact", 0, false)
    );
    assert_eq!(
        result("result-corrected"),
        is("correct", "learner", 3, true)
    );
    assert_eq!(
        result("result-revealed"),
        is("revealed", "reveal", 1, false)
    );
    assert_eq!(app("result-corrected").overrides.len(), 1);
    let a = app("result-then-archived");
    let o = a.occurrence.as_ref().unwrap();
    assert!(a.goals[&a.questions[&o.question_id].goal_id].archived);
    let a = app("result-then-edited");
    let o = a.occurrence.as_ref().unwrap();
    let q = &a.questions[&o.question_id];
    assert!(q.versions.len() == 2 && o.presentation == q.versions[0]);

    let a = app("go-retained-self-check");
    let o = a.occurrence.as_ref().unwrap();
    assert!(!a.revival_go.is_empty() && a.assessments.is_empty());
    assert!(a.events.iter().any(|e| e.is_ungraded()
        && e.occurrence_id == o.id
        && Some(&e.answer) == o.answer.as_ref()));

    let a = app("history-rich");
    let outcomes = a
        .events
        .iter()
        .map(|e| (e.result.outcome.as_str(), e.result.authority.as_str()))
        .collect::<BTreeSet<_>>();
    for wanted in [
        ("correct", "exact"),
        ("wrong", "exact"),
        ("warm_correct", "exact"),
        ("revealed", "reveal"),
        ("wrong", "jev"),
        ("correct", "jev"),
    ] {
        assert!(outcomes.contains(&wanted), "history-rich lacks {wanted:?}");
    }
    assert!(a.events.iter().any(|e| e.presentation.kind == "explain"));
    assert_eq!(a.overrides.len(), 1);
    assert_eq!(a.resets.len(), 1);
    let kinds = a
        .feedback
        .iter()
        .map(|f| f.kind.as_str())
        .collect::<BTreeSet<_>>();
    assert_eq!(kinds, BTreeSet::from(["confusion", "dispute"]));
    assert!(a.questions.values().any(|q| q.versions.len() == 2));
    assert!(a.questions.values().any(|q| q.draft.is_some()));
    assert!(a.questions.values().any(|q| q.archived));
    assert!(a.concepts.values().any(|c| c.archived));
    assert!(a.concepts.values().any(|c| c.already_known));
    let goals = |f: fn(&Goal) -> bool| a.goals.values().filter(|g| f(g)).count();
    assert_eq!(goals(|g| g.archived), 1);
    assert_eq!(goals(|g| g.paused), 1);
    assert_eq!(goals(|g| g.focused), 1);
    assert_eq!(goals(|g| g.status == "failed"), 1);
    assert_eq!(goals(|g| g.photo.is_some() && g.transcript.is_some()), 1);
    assert_eq!(a.preferences.pace, "light");
    assert!(a.concepts.values().any(|c| {
        c.notes
            .iter()
            .any(|n| n.basis == "source" && !n.quotes.is_empty())
    }));
    assert!(a.concepts.values().any(|c| c.notes.len() == 1
        && c.notes[0].basis == "general"
        && !c.prerequisites.is_empty()));
    let statuses = a
        .assessments
        .values()
        .map(|c| c.status.as_str())
        .collect::<BTreeSet<_>>();
    assert_eq!(statuses, BTreeSet::from(["complete", "superseded"]));
    assert_eq!(states["history-rich"].decoded().photos.len(), 1);

    let formats = states
        .values()
        .map(|s| s.format.as_str())
        .collect::<BTreeSet<_>>();
    assert_eq!(
        formats,
        BTreeSet::from([
            "scry-rust-durable-sqlite-v1",
            "scry-rust-durable-sqlite-attributed-spend-v1",
            "scry-rust-durable-sqlite-go-projection-v1",
        ])
    );
}

/// Nothing in the fixture is shaped like a credential. Under every field or
/// row named for a token, key, secret or authorization, no text holds twenty or
/// more hexadecimal or base64 characters in a row, except the all-zero
/// identifier and verifier of the one synthetic Owner token.
#[test]
fn the_fixture_holds_nothing_shaped_like_a_credential() {
    fn sensitive(name: &str) -> bool {
        let name = name.to_lowercase();
        ["token", "key", "secret", "auth"]
            .iter()
            .any(|word| name.contains(word))
    }
    fn collect(v: &Value, inside: bool, found: &mut Vec<String>) {
        match v {
            Value::Object(map) => {
                for (key, child) in map {
                    if inside {
                        found.push(key.clone());
                    }
                    collect(child, inside || sensitive(key), found);
                }
            }
            Value::Array(items) => items.iter().for_each(|v| collect(v, inside, found)),
            Value::String(text) if inside => found.push(text.clone()),
            _ => {}
        }
    }
    let mut checked = 0;
    for (name, stored) in committed() {
        let mut found = Vec::new();
        let archive: Value = serde_json::from_slice(&stored.archive).unwrap();
        collect(&archive, false, &mut found);
        for (key, row) in &stored.rows {
            if sensitive(key) {
                found.push(key.clone());
            }
            let row: Value = serde_json::from_str(row).unwrap();
            collect(&row, sensitive(key), &mut found);
        }
        for value in found {
            checked += 1;
            // A slash separates the parts of a row name, so it ends a run here.
            let shaped = value
                .split(|c: char| !(c.is_ascii_alphanumeric() || matches!(c, '+' | '=')))
                .any(|run| run.len() >= 20 && run.chars().any(|c| c != '0'));
            assert!(!shaped, "{name}: credential-shaped value {value}");
        }
    }
    assert!(checked > 0);
}
