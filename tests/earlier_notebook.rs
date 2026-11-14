//! The earlier concept notebook in every state an existing notebook can hold,
//! kept as stored bytes: `tests/fixtures/earlier-notebook.json`.
//!
//! The builders below drive the engine that can still produce these states and
//! must reproduce the committed fixture exactly. The other tests read only the
//! fixture, so they keep proving that existing notebooks decode, validate and
//! re-encode byte for byte after the builders are removed. See
//! `tests/fixtures/earlier-notebook.README.md`.
use scry::{
    engine::{self, WorkConfig},
    learning,
    model::*,
    owner_token::{MutationActor, OwnerToken, TokenInfo},
    persistence::{self, Archive, ArchivedPhoto},
};
use serde_json::{Value, json};
use std::collections::{BTreeMap, BTreeSet};

const NOW: i64 = 1_800_000_000_000;
const MINUTE: i64 = 60_000;
const FIXTURE: &str = concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/tests/fixtures/earlier-notebook.json"
);
const WRITE: &str = "SCRY_WRITE_EARLIER_NOTEBOOK_FIXTURE";

// ---------------------------------------------------------------------------
// The committed fixture. Everything below this block until the builders reads
// stored bytes only.
// ---------------------------------------------------------------------------

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
    static WRITTEN: std::sync::Once = std::sync::Once::new();
    WRITTEN.call_once(|| {
        if std::env::var_os(WRITE).is_some() {
            let text = serde_json::to_string_pretty(&rebuilt()).unwrap() + "\n";
            std::fs::write(FIXTURE, text).unwrap();
        }
    });
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

// ---------------------------------------------------------------------------
// The builders. They exist while the engine can still produce these states;
// when its concept-practice code is deleted they go with it, and the fixture
// becomes the only source of these states.
// ---------------------------------------------------------------------------

/// Owner tokens are identified by 32 lowercase hexadecimal characters, which
/// issuance derives from a hash. This one is written out so that the stored
/// bytes hold an identifier and verifier nobody could mistake for a credential.
const TOKEN_ID: &str = "00000000000000000000000000000000";

struct Owner {
    app: App,
    photos: Vec<ArchivedPhoto>,
    n: u64,
    leases: u64,
    now: i64,
    name: &'static str,
    token: Option<TokenInfo>,
}
impl Owner {
    fn new(name: &'static str) -> Self {
        Self {
            app: App::new("earlier-notebook-synthetic-csrf".into()),
            photos: vec![],
            n: 0,
            leases: 0,
            now: NOW,
            name,
            token: None,
        }
    }
    fn try_call(&mut self, path: &str, pairs: &[(&str, &str)]) -> AppResult<MutationResult> {
        self.n += 1;
        let mut f = BTreeMap::from([
            ("csrf".to_string(), self.app.csrf.clone()),
            (
                "operation_id".to_string(),
                format!("earlier-{}-{:06}", self.name, self.n),
            ),
        ]);
        if let Some(o) = &self.app.occurrence {
            f.insert("occurrence_id".into(), o.id.clone());
        }
        f.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.to_string())));
        let seed = format!("earlier-seed-{}-{}", self.name, self.n);
        let result = match &self.token {
            None => engine::mutate(&mut self.app, path, &f, self.now, &seed),
            Some(info) => engine::mutate_as(
                &mut self.app,
                path,
                &f,
                self.now,
                &seed,
                MutationActor::Token(info),
            ),
        };
        engine::validate_app(&self.app).unwrap();
        result
    }
    fn call(&mut self, path: &str, pairs: &[(&str, &str)]) -> MutationResult {
        self.try_call(path, pairs)
            .unwrap_or_else(|e| panic!("{path} in {}: {} {}", self.name, e.status, e.message))
    }
    fn held(&self) -> &Occurrence {
        self.app.occurrence.as_ref().unwrap()
    }
    /// The held question is the one expected, in the expected phase and assistance.
    fn expect(&self, prompt: &str, phase: &str, assisted: bool) {
        let o = self.held();
        assert!(
            o.presentation.prompt.starts_with(prompt) && o.phase == phase && o.assisted == assisted,
            "{}: holds {:?} in {} (assisted {})",
            self.name,
            o.presentation.prompt,
            o.phase,
            o.assisted
        );
    }
    fn answer(&mut self, text: &str) {
        self.call("/review/answer", &[("answer", text)]);
    }
    fn next(&mut self) {
        self.call("/review/next", &[]);
    }
    /// What reading a page does today: hold the next due question, if any.
    fn read(&mut self) {
        self.n += 1;
        let seed = format!("earlier-seed-{}-{}", self.name, self.n);
        engine::ensure_occurrence(&mut self.app, self.now, &seed).unwrap();
        engine::validate_app(&self.app).unwrap();
    }
    fn lease(&mut self) -> String {
        self.leases += 1;
        format!("earlier-lease-{}-{:02}", self.name, self.leases)
    }
    /// Claim the queued check as the serving runtime does: reserve, then send.
    fn send(&mut self) -> engine::Work {
        let lease = self.lease();
        let work = engine::claim_grading_work(&mut self.app, self.now, &lease, &config())
            .unwrap()
            .unwrap();
        assert_eq!(work.kind, "assessment");
        engine::validate_app(&self.app).unwrap();
        work
    }
    /// Send the queued check and receive `raw` a minute later.
    fn check(&mut self, raw: String) {
        let work = self.send();
        self.now += MINUTE;
        engine::finish_work(
            &mut self.app,
            &work,
            raw,
            "synthetic-jev".into(),
            Some(19_500),
            self.now,
        )
        .unwrap();
        engine::validate_app(&self.app).unwrap();
    }
    /// Run retained preparation work to its end, answering each request in turn.
    fn prepare(&mut self, responses: &[String]) {
        for raw in responses {
            let lease = self.lease();
            let work = engine::claim_work(&mut self.app, self.now, &lease, &config())
                .unwrap()
                .unwrap();
            self.now += MINUTE;
            engine::finish_work(
                &mut self.app,
                &work,
                raw.clone(),
                "synthetic-generator".into(),
                Some(41_000),
                self.now,
            )
            .unwrap();
        }
        // With no content check configured the saved candidates are published.
        let lease = self.lease();
        assert!(
            engine::claim_work(&mut self.app, self.now, &lease, &config())
                .unwrap()
                .is_none()
        );
        engine::validate_app(&self.app).unwrap();
    }
    fn goal(&self, title: &str) -> String {
        let mut found = self.app.goals.values().filter(|g| g.title == title);
        let id = found.next().unwrap().id.clone();
        assert!(found.next().is_none());
        id
    }
    fn concept(&self, name: &str) -> String {
        let mut found = self.app.concepts.values().filter(|c| c.name == name);
        let id = found.next().unwrap().id.clone();
        assert!(found.next().is_none());
        id
    }
    fn question(&self, prompt: &str) -> Question {
        let mut found = self
            .app
            .questions
            .values()
            .filter(|q| q.versions[0].prompt.starts_with(prompt));
        let q = found.next().unwrap().clone();
        assert!(found.next().is_none());
        q
    }
}

fn config() -> WorkConfig {
    WorkConfig {
        generation: true,
        jev: true,
        critic: false,
        model: "synthetic-generator".into(),
        jev_model: "synthetic-jev".into(),
    }
}
fn accepted() -> String {
    json!({"answers":{"verdict":{"choice":"accept","probabilities":{"accept":0.99,"reject":0.005,"unsure":0.005}},"identity":{"noul":0.0},"injection":{"noul":0.0}}}).to_string()
}
fn rejected() -> String {
    json!({"answers":{"verdict":{"choice":"reject","probabilities":{"accept":0.02,"reject":0.97,"unsure":0.01}},"identity":{"noul":0.9},"injection":{"noul":0.0}}}).to_string()
}
fn unsure() -> String {
    json!({"answers":{"verdict":{"choice":"unsure","probabilities":{"accept":0.3,"reject":0.3,"unsure":0.4}},"identity":{"noul":0.0},"injection":{"noul":0.0}}}).to_string()
}
fn explained() -> String {
    json!({"answers":{"idea_0":{"noul":0.97},"idea_1":{"noul":0.95},"contradiction_0":{"noul":0.01},"relation":{"choice":"equivalent","probabilities":{"equivalent":0.96,"partial":0.02,"different":0.01,"unclear":0.01}},"injection":{"noul":0.0}}}).to_string()
}
/// A provider envelope around generated material.
fn generated(content: Value) -> String {
    json!({"choices":[{"message":{"content":content.to_string()}}]}).to_string()
}

const CHOICE: &str = "A stored response has passed its freshness lifetime.";
const CHOICE_ANSWER: &str = "It may need validation before reuse";
const MAX_AGE: &str = "Which response directive sets how long";
const STATUS: &str = "What HTTP status tells a browser";
const HTTP: &str = "Understand HTTP caching";
const INEXACT: &str = "three hundred and four";

/// The authored demonstration notebook as `engine::seed_fixture` leaves it:
/// its multiple-choice question held, unanswered and unaided.
fn seeded(name: &'static str) -> Owner {
    let mut o = Owner::new(name);
    engine::seed_fixture(&mut o.app, NOW).unwrap();
    engine::validate_app(&o.app).unwrap();
    o.expect(CHOICE, "question", false);
    o
}

/// The same notebook after two exact answers, the second helped by the first
/// result, holding a recall question unanswered and unaided: the state that can
/// leave an answer for Jev.
fn recall(name: &'static str) -> Owner {
    let mut o = seeded(name);
    o.answer(CHOICE_ANSWER);
    o.next();
    o.expect(MAX_AGE, "question", true);
    o.answer("max-age");
    o.next();
    o.expect(STATUS, "question", false);
    o
}

/// A recall answer that is not an exact match, saved and queued for Jev.
fn queued(name: &'static str) -> Owner {
    let mut o = recall(name);
    o.answer(INEXACT);
    o
}

const TIDES: &str = "Tides notes for the harbour course.\nThe Moon's gravity raises two bulges of water on opposite sides of the Earth.\nSpring tides come at new and full Moon, when the Sun and Moon pull along one line.\nNeap tides come at the quarter Moons, when the Sun and Moon pull at right angles.";
const TIDES_TITLE: &str = "Tides for the harbour course";
const BULGES: &str = "Which body's gravity raises the two tidal bulges?";
const TWO_TIDES: &str = "Why are there two high tides on most days";

fn tides() -> Value {
    json!({"title":TIDES_TITLE,"complete":true,"concepts":[
        {"key":"bulges","name":"Two tidal bulges","summary":"The Moon raises water on the near and the far side of the Earth.",
         "note":"The Moon pulls the ocean into a bulge on the side facing it, and a second bulge stands on the opposite side. The Earth turns beneath both.",
         "basis":"source","quotes":["The Moon's gravity raises two bulges of water on opposite sides of the Earth."],"prerequisites":[],
         "questions":[
            {"prompt":BULGES,"kind":"recall","answer":"The Moon","variants":["Moon"],"choices":[],
             "explanation":"The supplied notes say the Moon's gravity raises both bulges.","basis":"source",
             "quotes":["The Moon's gravity raises two bulges of water on opposite sides of the Earth."]}]},
        {"key":"daily","name":"Two high tides a day","summary":"Each place passes under both bulges in one turn of the Earth.",
         "note":"Because the Earth turns once a day beneath two bulges, most coasts see two high tides and two low tides each day.",
         "basis":"general","quotes":[],"prerequisites":["bulges"],
         "questions":[
            {"prompt":"Why are there two high tides on most days rather than one?","kind":"explain",
             "answer":"Water bulges on the side facing the Moon and on the opposite side, and the Earth turns through both bulges each day.",
             "variants":[],"choices":[],
             "explanation":"Each place passes under two bulges in one rotation, so it sees two high tides.","basis":"general","quotes":[],
             "required_ideas":["There are two bulges on opposite sides of the Earth","The Earth rotates through both bulges each day"],
             "contradictions":["The Sun alone causes the tides"]}]},
        {"key":"springs","name":"Spring tides","summary":"The largest tides come when the Sun and Moon pull along one line.",
         "note":"At new and full Moon the Sun's pull adds to the Moon's, so high tides are higher and low tides lower.",
         "basis":"source","quotes":["Spring tides come at new and full Moon, when the Sun and Moon pull along one line."],"prerequisites":["bulges"],
         "questions":[
            {"prompt":SPRING_PHASES,"kind":"choice","answer":"New and full Moon",
             "choices":["New and full Moon","The quarter Moons","Only the full Moon"],
             "explanation":"The Sun and Moon pull along one line at new and full Moon.","basis":"general","quotes":[]},
            {"prompt":SPRING_NAME,"kind":"recall","answer":"Spring tides","variants":["Springs"],"choices":[],
             "explanation":"They are named for springing up, not for the season.","basis":"general","quotes":[]}]},
        {"key":"neaps","name":"Neap tides","summary":"The smallest tides come when the Sun and Moon pull at right angles.",
         "note":"At the quarter Moons the Sun's pull works across the Moon's, so the range between high and low water is least.",
         "basis":"general","quotes":[],"prerequisites":["bulges"],
         "questions":[
            {"prompt":NEAP_NAME,"kind":"recall","answer":"Neap tides","variants":["Neaps"],"choices":[],
             "explanation":"Neap tides have the least range.","basis":"general","quotes":[]}]}]})
}
const SPRING_PHASES: &str = "At which phases of the Moon do the largest tides come?";
const SPRING_NAME: &str = "What are the larger tides at new and full Moon called?";
const NEAP_NAME: &str = "What are the smaller tides at the quarter Moons called?";
const EXPLANATION: &str =
    "There is a bulge on each side of the Earth and we turn through both every day.";

/// A Goal made from supplied material by the retained preparation path:
/// notes and questions that quote the material, an explanation question and
/// Concepts that depend on another. Nothing has been read or practised.
fn prepared(name: &'static str) -> Owner {
    let mut o = Owner::new(name);
    o.call("/create", &[("intent", TIDES)]);
    o.prepare(&[generated(tides())]);
    assert!(o.app.occurrence.is_none());
    o
}

/// The answered question left with a failed check each time, until no further
/// check may be asked.
fn three_checks() -> Owner {
    let mut o = queued("three-checks");
    o.check("not json".into());
    o.call("/review/retry", &[]);
    o.check("{\"answers\":{}}".into());
    o.call("/review/retry", &[]);
    o.check("not json".into());
    assert_eq!(o.try_call("/review/retry", &[]).unwrap_err().status, 422);
    o
}

/// An attempt Go held before answer checks existed, as the revival projection
/// stores it: an ungraded observation and a self-check with no check record.
/// No engine path writes this; it is set on the stored structures directly.
fn go_retained() -> Owner {
    use base64::{Engine as _, engine::general_purpose::STANDARD};
    let mut o = Owner::new("go-retained-self-check");
    let original = b"SYNTHETIC_OPAQUE_GO_RECOVERY for the earlier notebook fixture";
    o.app.revival_go.insert(
        format!("{}/{:08}", persistence::sha256(original), 0),
        STANDARD.encode(original),
    );
    engine::seed_fixture(&mut o.app, NOW).unwrap();
    let mut question = o.question(MAX_AGE);
    o.app.questions.remove(&question.id);
    question.id = format!("legacy-import-quiz-{}", question.id);
    question.schedule_version = 1;
    let result = ReviewResult {
        event_id: "legacy-import-event-earlier-fixture".into(),
        outcome: "selfcheck".into(),
        authority: "ungraded".into(),
        rating: 0,
        reviewed_ms: NOW,
        due_ms: question.card.due_ms,
        corrected: false,
    };
    let event = Event {
        id: result.event_id.clone(),
        occurrence_id: "legacy-import-presentation-earlier-fixture".into(),
        question_id: question.id.clone(),
        concept_id: question.concept_id.clone(),
        presentation: question.content().clone(),
        answer: INEXACT.into(),
        assisted: false,
        before_card: question.card.clone(),
        after_card: question.card.clone(),
        schedule_version: 1,
        algorithm: learning::event_algorithm("exact-v1"),
        result,
    };
    o.app.occurrence = Some(Occurrence {
        id: event.occurrence_id.clone(),
        question_id: question.id.clone(),
        concept_id: question.concept_id.clone(),
        presentation: event.presentation.clone(),
        phase: "self-check".into(),
        assisted: false,
        before_card: question.card.clone(),
        schedule_version: 1,
        created_ms: NOW,
        answer: Some(event.answer.clone()),
        assessment_id: None,
        result: None,
    });
    o.app
        .revival_schedule_origins
        .insert(question.id.clone(), 1);
    o.app.questions.insert(question.id.clone(), question);
    o.app.events.push(event);
    o.app.revision += 1;
    o
}

/// A smallest PNG: one transparent pixel.
const PIXEL: &[u8] = &[
    0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
    0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
    0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
    0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
    0x42, 0x60, 0x82,
];
const KNOTS: &str =
    "A bowline makes a fixed loop that does not slip.\nA clove hitch ties a line to a post.";

/// A notebook with every kind of saved history. Each step is commented with
/// what it leaves stored.
fn history_rich() -> Owner {
    let mut o = seeded("history-rich");
    let http = o.goal(HTTP);

    // The earlier pace preference, set away from its default. It allows three
    // new Concepts a day.
    o.call("/settings", &[("pace", "light")]);

    // A second Goal from supplied material; putting it in focus holds back the
    // first Goal's unseen questions.
    o.call("/create", &[("intent", TIDES)]);
    o.prepare(&[generated(tides())]);
    let tides_goal = o.goal(TIDES_TITLE);
    o.call(&format!("/goals/{tides_goal}/focus"), &[]);

    // A wrong exact answer to a multiple-choice question.
    o.answer("The origin content definitely changed");
    o.next();

    // A new Concept is introduced and marked already known. Its first answer is
    // helped by that introduction, so it is recorded without a schedule change.
    o.expect(BULGES, "intro", false);
    o.call("/review/intro", &[("known", "true")]);
    o.expect(BULGES, "question", true);
    o.answer("Moon");
    o.next();

    // A second introduction, and a helped explanation that Jev accepted.
    o.expect(TWO_TIDES, "intro", false);
    o.call("/review/intro", &[("known", "false")]);
    o.answer(EXPLANATION);
    o.check(explained());
    o.next();

    // The missed question returns the same day, helped by its own result.
    o.expect(CHOICE, "question", true);
    o.answer(CHOICE_ANSWER);
    o.next();

    // A third introduction, a helped wrong choice, and an answer shown without
    // an attempt. The day's three new Concepts are used, so nothing is held.
    o.expect(SPRING_PHASES, "intro", false);
    o.call("/review/intro", &[("known", "false")]);
    o.answer("The quarter Moons");
    o.next();
    o.expect(SPRING_NAME, "question", true);
    o.call("/review/reveal", &[]);
    o.next();
    assert!(o.app.occurrence.is_none());

    // Two days later every answer is unaided: exact, an explanation accepted by
    // Jev under the semantic policy, two choices and, after two more days, a
    // recall answer Jev rejected followed by the Owner's Correction of that
    // Verdict.
    o.now += 2 * DAY_MS;
    o.read();
    o.expect(BULGES, "question", false);
    o.answer("The Moon");
    o.next();
    o.expect(TWO_TIDES, "question", false);
    o.answer(EXPLANATION);
    o.check(explained());
    o.next();
    o.expect(SPRING_PHASES, "question", false);
    o.answer("New and full Moon");
    o.next();
    o.expect(CHOICE, "question", false);
    o.answer(CHOICE_ANSWER);
    o.now += 2 * DAY_MS;
    o.next();
    o.expect(SPRING_NAME, "question", false);
    o.answer("the spring ones");
    o.check(rejected());
    o.call("/review/override", &[("correct", "true")]);
    o.next();

    // The questions answered two days earlier come due again: unaided exact
    // answers, one of them to the explanation in its authored words, and two
    // answers helped by a result dismissed the same day.
    o.expect(BULGES, "question", false);
    o.answer("Moon");
    o.next();
    o.expect(CHOICE, "question", true);
    o.answer(CHOICE_ANSWER);
    o.next();
    o.expect(TWO_TIDES, "question", false);
    let authored = o.held().presentation.answer.clone();
    o.answer(&authored);
    o.next();
    o.expect(SPRING_PHASES, "question", true);
    o.answer("New and full Moon");
    o.next();

    // The last new Concept is introduced and then archived before any answer,
    // which lets go of its held question. One practised question is archived.
    o.expect(NEAP_NAME, "intro", false);
    let neaps = o.concept("Neap tides");
    o.call(&format!("/concepts/{neaps}/archive"), &[]);
    assert!(o.app.occurrence.is_none());
    let phases = o.question(SPRING_PHASES);
    o.call(&format!("/questions/{}/archive", phases.id), &[]);

    // Focus moves to the first Goal and the second is paused.
    o.call(&format!("/goals/{http}/focus"), &[]);
    o.call(&format!("/goals/{tides_goal}/pause"), &[]);

    // A helped answer is queued for Jev and the question is then edited: the
    // check is superseded unsent, and the question holds two versions.
    o.read();
    o.expect(MAX_AGE, "question", true);
    o.answer("the freshness lifetime directive");
    let max_age = o.question(MAX_AGE);
    o.call(
        &format!("/questions/{}/edit", max_age.id),
        &[
            ("content_version", "1"),
            (
                "prompt",
                "Which Cache-Control directive gives a response its freshness lifetime in seconds?",
            ),
            ("kind", "recall"),
            ("answer", "max-age"),
            ("variants", "max-age=N"),
            ("choices", ""),
            (
                "explanation",
                "max-age counts seconds from the time the response was generated.",
            ),
            ("basis", "general"),
            ("quotes", ""),
        ],
    );
    assert!(o.app.occurrence.is_none());
    o.read();
    o.expect("Which Cache-Control directive", "question", true);
    o.answer("max-age");
    o.next();

    // An unaided question whose answer is shown, then a dispute of it with a
    // deliberate schedule reset, and feedback on its Concept.
    o.expect(STATUS, "question", false);
    o.call("/review/reveal", &[]);
    o.next();
    assert!(o.app.occurrence.is_none());
    let status = o.question(STATUS);
    o.call(
        &format!("/questions/{}/dispute", status.id),
        &[
            ("reset", "true"),
            ("content_version", "1"),
            ("schedule_version", &status.schedule_version.to_string()),
            (
                "text",
                "The explanation repeats the answer without saying why.",
            ),
        ],
    );
    let validation = o.concept("Conditional validation");
    o.call(
        &format!("/concepts/{validation}/feedback"),
        &[("text", "I mix up freshness and validation.")],
    );

    // A fix was asked for one question and its draft was never applied.
    let choice = o.question(CHOICE);
    o.call(&format!("/questions/{}/fix", choice.id), &[]);
    o.prepare(&[generated(json!({"title":HTTP,"complete":true,"concepts":[
        {"key":"fix","name":"Cache freshness","summary":"A fresh response can be reused without asking the origin.",
         "note":"A stale response is not a changed response; it is one that may need checking.","basis":"general","quotes":[],"prerequisites":[],
         "questions":[{"prompt":"A cached response is stale. What must happen before it is reused?","kind":"choice",
            "answer":"It may need to be validated with the origin",
            "choices":["It may need to be validated with the origin","It must be deleted","Nothing at all"],
            "explanation":"Stale means the freshness lifetime has passed, so the cache may have to validate.","basis":"general","quotes":[]}]}]}))]);
    assert!(o.app.questions[&choice.id].draft.is_some());

    // A Goal from a photo: the transcription is its supplied material, quoted
    // by its note. The Goal is then archived.
    let photo = Photo {
        key: "photos/earlier-fixture-knots".into(),
        mime: "image/png".into(),
        size: PIXEL.len(),
        sha256: persistence::sha256(PIXEL),
    };
    o.call(
        "/create",
        &[
            ("intent", "Knots page from the sailing handbook"),
            ("__photo", &serde_json::to_string(&photo).unwrap()),
        ],
    );
    o.photos.push(ArchivedPhoto {
        photo,
        bytes: PIXEL.to_vec(),
    });
    o.prepare(&[
        generated(json!({"text":KNOTS})),
        generated(json!({"title":"Knots from the handbook","complete":true,"concepts":[
            {"key":"bowline","name":"The bowline","summary":"A fixed loop at the end of a line.",
             "note":"The bowline is the standard fixed loop: it holds under load and unties easily afterwards.",
             "basis":"source","quotes":["A bowline makes a fixed loop that does not slip."],"prerequisites":[],
             "questions":[{"prompt":"Which knot makes a fixed loop that does not slip?","kind":"recall","answer":"A bowline","variants":["Bowline"],"choices":[],
                "explanation":"The handbook page names the bowline for a fixed loop.","basis":"source",
                "quotes":["A bowline makes a fixed loop that does not slip."]}]}]})),
    ]);
    let knots = o.goal("Knots from the handbook");
    o.call(&format!("/goals/{knots}/archive"), &[]);

    // A Goal whose preparation stopped: nothing was ever made for it.
    o.call("/create", &[("intent", "Why the sky is blue")]);
    let lease = o.lease();
    let unconfigured = WorkConfig {
        generation: false,
        ..config()
    };
    assert!(
        engine::claim_work(&mut o.app, o.now, &lease, &unconfigured)
            .unwrap()
            .is_none()
    );
    engine::validate_app(&o.app).unwrap();
    o
}

fn snapshot(o: &Owner) -> Value {
    let archive = Archive::new(o.app.clone(), o.photos.clone(), o.now);
    let bytes = archive.encode().unwrap();
    json!({
        "format": archive.format,
        "sha256": persistence::sha256(&bytes),
        "bytes": bytes.len(),
        "now_ms": o.now,
        "rows": persistence::rows(&o.app).unwrap(),
        "archive": String::from_utf8(bytes).unwrap(),
    })
}

/// An Owner token with paid permission, as a paid agent's request carries it.
fn paid_token(o: &mut Owner) {
    let info = TokenInfo {
        id: TOKEN_ID.into(),
        name: "Earlier-notebook synthetic agent".into(),
        created_ms: NOW,
        last_used_ms: None,
        expires_ms: None,
        paid: true,
        revoked_ms: None,
    };
    o.app.owner_tokens.insert(
        TOKEN_ID.into(),
        OwnerToken {
            info: info.clone(),
            verifier: "0".repeat(64),
        },
    );
    o.app.revision += 1;
    o.token = Some(info);
}

fn notebooks() -> Vec<Owner> {
    let mut all = vec![prepared("never-practised")];

    // Every question answered and dismissed; one helped answer is still due.
    let mut o = recall("empty-history");
    o.answer("304");
    o.next();
    assert!(o.app.occurrence.is_none());
    all.push(o);

    let mut o = prepared("intro");
    o.read();
    o.expect(BULGES, "intro", false);
    all.push(o);

    all.push(recall("question-cold"));
    all.push(seeded("question-cold-choice"));

    // Look it up: the reference is opened before answering.
    let mut o = recall("question-helped");
    o.call("/review/help", &[]);
    all.push(o);

    // Queued by an agent whose Owner token carries paid permission.
    let mut o = recall("checking-queued");
    paid_token(&mut o);
    o.answer(INEXACT);
    all.push(o);

    // An explanation, helped by its introduction, queued under the semantic policy.
    let mut o = prepared("checking-queued-explain");
    o.read();
    o.call("/review/intro", &[("known", "false")]);
    o.answer("Moon");
    o.next();
    o.expect(TWO_TIDES, "intro", false);
    o.call("/review/intro", &[("known", "false")]);
    o.answer(EXPLANATION);
    all.push(o);

    // Reserved and sent; no outcome has come back.
    let mut o = queued("checking-sent");
    o.send();
    all.push(o);

    // A response that could not be read: its cost is known.
    let mut o = queued("self-check-failed");
    o.check("not json".into());
    all.push(o);

    // Jev was not configured: nothing was reserved or sent.
    let mut o = queued("self-check-failed-unsent");
    let lease = o.lease();
    let unconfigured = WorkConfig {
        jev: false,
        ..config()
    };
    assert!(
        engine::claim_grading_work(&mut o.app, o.now, &lease, &unconfigured)
            .unwrap()
            .is_none()
    );
    all.push(o);

    // The send itself failed: the reservation stays unknown.
    let mut o = queued("self-check-failed-send");
    let work = o.send();
    o.now += MINUTE;
    engine::fail_work(
        &mut o.app,
        &work,
        "The check could not be sent. Your answer is saved and remains ungraded.",
        false,
        o.now,
    )
    .unwrap();
    all.push(o);

    // Jev answered with a refusal, kept as received and never read as a judgment.
    let mut o = queued("self-check-rejected");
    let work = o.send();
    o.now += MINUTE;
    engine::fail_received_work(
        &mut o.app,
        &work,
        o.now,
        "{\"error\":\"synthetic refusal\"}".into(),
        "synthetic-jev".into(),
        None,
        "The check was refused. Your answer is saved and remains ungraded.",
    )
    .unwrap();
    all.push(o);

    // Sent, then no outcome within the recovery time.
    let mut o = queued("self-check-unknown");
    o.send();
    o.now += 2 * MINUTE;
    assert!(engine::recover_grading(&mut o.app, o.now));
    all.push(o);

    let mut o = queued("self-check-unsure");
    o.check(unsure());
    all.push(o);

    // Restored while its check was queued: nothing was sent.
    let mut o = queued("self-check-paused");
    o.now += MINUTE;
    engine::pause_for_restore(&mut o.app, o.now).unwrap();
    all.push(o);

    // Restored while its check was sent: the outcome is unknown.
    let mut o = queued("restored-sent");
    o.send();
    o.now += MINUTE;
    engine::pause_for_restore(&mut o.app, o.now).unwrap();
    all.push(o);

    // Answered in a restored notebook: queued, and never claimed while paused.
    let mut o = recall("queued-while-paused");
    engine::pause_for_restore(&mut o.app, o.now).unwrap();
    o.answer(INEXACT);
    let lease = o.lease();
    assert!(
        engine::claim_grading_work(&mut o.app, o.now, &lease, &config())
            .unwrap()
            .is_none()
    );
    all.push(o);

    all.push(three_checks());

    let mut o = recall("result-held");
    o.answer("304");
    all.push(o);

    let mut o = queued("result-jev-correct");
    o.check(accepted());
    all.push(o);

    let mut o = queued("result-jev-wrong");
    o.check(rejected());
    all.push(o);

    let mut o = recall("result-helped");
    o.call("/review/help", &[]);
    o.answer("304");
    all.push(o);

    // Jev rejected the answer and the Owner corrected that Verdict.
    let mut o = queued("result-corrected");
    o.check(rejected());
    o.call("/review/override", &[("correct", "true")]);
    all.push(o);

    let mut o = recall("result-revealed");
    o.call("/review/reveal", &[]);
    all.push(o);

    // A result stays held when its Goal is archived afterwards.
    let mut o = recall("result-then-archived");
    o.answer("304");
    let http = o.goal(HTTP);
    o.call(&format!("/goals/{http}/archive"), &[]);
    all.push(o);

    // A result stays held, showing the wording answered, when its question is
    // edited afterwards.
    let mut o = recall("result-then-edited");
    o.answer("304");
    let status = o.question(STATUS);
    o.call(
        &format!("/questions/{}/edit", status.id),
        &[
            ("content_version", "1"),
            (
                "prompt",
                "Which status code says a cached representation is still current?",
            ),
            ("kind", "recall"),
            ("answer", "304"),
            ("variants", "304 Not Modified"),
            ("choices", ""),
            (
                "explanation",
                "304 Not Modified answers a conditional request whose validator still matches.",
            ),
            ("basis", "general"),
            ("quotes", ""),
        ],
    );
    all.push(o);

    all.push(go_retained());
    all.push(history_rich());
    for o in &all {
        engine::validate_app(&o.app).unwrap();
    }
    all
}

fn rebuilt() -> Value {
    Value::Object(
        notebooks()
            .iter()
            .map(|o| (o.name.to_string(), snapshot(o)))
            .collect(),
    )
}

/// Rebuilding every state through today's engine gives the committed fixture
/// exactly. This keeps the fixture honest until the engine's concept-practice
/// code is deleted; the builders are then removed with it.
#[test]
fn rebuilding_every_state_gives_the_committed_fixture() {
    let before: Value = {
        committed();
        serde_json::from_str(&std::fs::read_to_string(FIXTURE).unwrap()).unwrap()
    };
    let now = rebuilt();
    let names = |v: &Value| v.as_object().unwrap().keys().cloned().collect::<Vec<_>>();
    assert_eq!(names(&now), names(&before));
    for (name, was) in before.as_object().unwrap() {
        for part in ["format", "sha256", "bytes", "now_ms", "rows", "archive"] {
            assert!(
                now[name][part] == was[part],
                "{name}: {part} differs from the committed fixture"
            );
        }
    }
}
