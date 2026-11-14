//! With paid judgment off, nothing changes: the stored rows, the complete
//! recovery archive and the rendered result and history pages of representative
//! notebooks are byte for byte what the source before paid judgment produced.
//!
//! `tests/fixtures/judgment-off.json` was written by this file's own builders on
//! unmodified source `701724c`; see `tests/fixtures/judgment-off.README.md`.
use scry::{
    engine::{self, WorkConfig},
    model::*,
    owner_token::{MutationActor, OwnerToken},
    persistence, study, web,
};
use serde_json::{Value, json};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;
const MINUTE: i64 = 60_000;
const FIXTURE: &str = concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/tests/fixtures/judgment-off.json"
);

struct Owner {
    app: App,
    n: u64,
    now: i64,
    name: &'static str,
    token: Option<String>,
}
impl Owner {
    fn new(name: &'static str) -> Self {
        Self {
            app: App::new("judgment-off-synthetic-csrf-0123456789".into()),
            n: 0,
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
                format!("judgment-off-{}-{:06}", self.name, self.n),
            ),
        ]);
        f.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.to_string())));
        let seed = format!("judgment-off-seed-{}-{}", self.name, self.n);
        let result = match self.token.clone() {
            None => engine::mutate(&mut self.app, path, &f, self.now, &seed),
            Some(id) => {
                let info = self.app.owner_tokens[&id].info.clone();
                engine::mutate_as(
                    &mut self.app,
                    path,
                    &f,
                    self.now,
                    &seed,
                    MutationActor::Token(&info),
                )
            }
        };
        engine::validate_app(&self.app).unwrap();
        result
    }
    fn call(&mut self, path: &str, pairs: &[(&str, &str)]) -> MutationResult {
        self.try_call(path, pairs)
            .unwrap_or_else(|e| panic!("{path} in {}: {} {}", self.name, e.status, e.message))
    }
    fn pid(&self) -> String {
        self.app
            .practice
            .current
            .as_ref()
            .unwrap()
            .prompt
            .id
            .clone()
    }
    /// Answer the held Prompt, trying from memory first where it is source study.
    fn answer(&mut self, text: &str) {
        if self.app.practice.current.as_ref().unwrap().source_visible() {
            let pid = self.pid();
            self.call("/practice/quiz", &[("prompt_id", &pid)]);
        }
        let pid = self.pid();
        self.call("/practice/answer", &[("prompt_id", &pid), ("answer", text)]);
        self.now += MINUTE;
    }
    fn reveal(&mut self) {
        let pid = self.pid();
        self.call("/practice/reveal", &[("prompt_id", &pid)]);
    }
    fn lesson(&mut self, mode: &str) {
        let pid = self.pid();
        self.call("/practice/lesson/next", &[("after", &pid), ("mode", mode)]);
    }
    fn next(&mut self, extra: &[(&str, &str)]) {
        let mut pairs = extra.to_vec();
        let after = self
            .app
            .practice
            .current
            .as_ref()
            .filter(|p| p.attempt_id.is_some())
            .map(|p| p.prompt.id.clone());
        if let Some(after) = &after {
            pairs.push(("after", after));
        }
        self.call("/practice/next", &pairs);
    }
    fn add(&mut self, path: &str, data: Value) -> String {
        self.call(path, &[("data", &data.to_string())])
            .entity_id
            .unwrap()
    }
}

const CREED: &str = "I believe in one God,\nthe Father almighty,\nmaker of heaven and earth,\nof all things visible and invisible.\n";
const CREED_OLDER: &str = "We believe in one God,\nthe Father, the Almighty,\nmaker of heaven and earth,\nof all that is, seen and unseen.\n";

/// A text lesson: exact answers, a Slip, mismatches a judge might accept, an
/// empty answer, a reveal, a guided retry, a joined passage and the whole text.
fn text() -> Owner {
    let mut o = Owner::new("text");
    let vid = o.add(
        "/library/version/add",
        json!({"title":"Synthetic Creed","name":"Roman Missal 2011 metadata; authored text","kind":"text","text":CREED}),
    );
    o.add(
        "/library/version/add",
        json!({"title":"Synthetic Creed","name":"Earlier authored wording","kind":"text","text":CREED_OLDER}),
    );
    o.call("/practice/lesson/start", &[("version_id", &vid)]);
    o.answer("I believe in one God");
    o.lesson("phrases");
    o.answer("the Father almigty");
    o.lesson("phrases");
    // Mismatched wording that another judge could accept: stays a local miss.
    o.answer("creator of heaven and earth");
    o.lesson("retry");
    o.answer("maker of heaven and earth");
    o.lesson("phrases");
    o.answer("");
    o.lesson("retry");
    o.answer("of all things visible and invisible");
    o.lesson("join");
    o.answer("I believe in one God the Father almighty maker of heaven and earth");
    o.lesson("whole");
    o.answer(CREED_OLDER);
    o.lesson("whole");
    o.reveal();
    o.answer(CREED);
    o.now += DAY_MS;
    o.lesson("whole");
    o.answer(CREED);
    o.lesson("middle");
    o.answer("made of heaven and earth");
    o
}

/// An ordered list: an omission, a duplicate, a swap, an unlisted wording of an
/// item and a complete answer.
fn list() -> Owner {
    let mut o = Owner::new("list");
    o.call("/practice/lesson/start", &[("starter", "rosary-joyful")]);
    o.answer("The Annunciation\nThe Visitation\nThe Nativity\nThe Presentation");
    o.lesson("whole");
    o.answer("The Annunciation\nThe Annunciation\nThe Visitation\nThe Nativity\nThe Presentation\nThe Finding in the Temple");
    o.lesson("whole");
    o.answer("The Visitation\nThe Annunciation\nThe Nativity\nThe Presentation\nThe Finding in the Temple");
    o.lesson("whole");
    o.answer("The angel Gabriel announces to Mary\nThe Visitation\nThe Nativity\nThe Presentation\nThe Finding in the Temple");
    o.now += DAY_MS;
    o.lesson("whole");
    let complete = o.app.library.versions.values().next().unwrap().text.clone();
    o.answer(&complete);
    o
}

/// Mappings: one that is not Spelling-strict, with an unlisted valid wording,
/// another key's value and a negation; then the Spelling-strict NATO starter.
fn mapping() -> Owner {
    let mut o = Owner::new("mapping");
    let vid = o.add(
        "/library/mapping/add",
        json!({"title":"Synthetic harbour practice","direction":"forward","spelling_strict":false,"pairs":[
            {"key":"Before leaving the berth","value":"check the bilge pump"},
            {"key":"In fog","value":"sound the horn"},
            {"key":"A person overboard","value":"never lose sight of them"}
        ]}),
    );
    o.call("/practice/study/start", &[("version_id", &vid)]);
    o.answer("test the bilge pump");
    o.next(&[]);
    o.answer("check the bilge pump");
    o.next(&[]);
    o.answer("sound the horn");
    o.next(&[]);
    o.answer("lose sight of them");
    o.now += DAY_MS;
    o.next(&[]);
    o.answer("check the bilge pump");
    o.next(&[]);
    o.answer("keep them in view");
    o
}

/// A miss on the Spelling-strict NATO starter, held as its result.
fn nato_result() -> Owner {
    let mut o = Owner::new("nato-result");
    o.call("/practice/nato/start", &[]);
    o.answer("Alfa");
    let sid = o.app.practice.studies.keys().next().unwrap().clone();
    o.next(&[("study_id", &sid)]);
    o.answer("Beta");
    o
}

/// The Spelling-strict NATO starter, answered through an Owner token without
/// paid permission, with a saved draft held at the end.
fn nato() -> Owner {
    let mut o = Owner::new("nato");
    let token = OwnerToken::issue(
        "Judgment-off local agent",
        None,
        false,
        NOW,
        "abcdef0123456789abcdef0123456789abcdef0123456789",
    )
    .unwrap();
    let id = token.info.id.clone();
    o.app.owner_tokens.insert(id.clone(), token);
    o.token = Some(id);
    o.call("/practice/nato/start", &[]);
    o.answer("Alpha");
    let sid = o.app.practice.studies.keys().next().unwrap().clone();
    o.next(&[("study_id", &sid)]);
    o.answer("Bravo");
    o.next(&[("study_id", &sid)]);
    o.reveal();
    o.answer("Charlie");
    o.next(&[("study_id", &sid)]);
    let pid = o.pid();
    o.call("/practice/quiz", &[("prompt_id", &pid)]);
    let pid = o.pid();
    o.call(
        "/practice/draft",
        &[("prompt_id", &pid), ("generation", "0"), ("answer", "Delt")],
    );
    o
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

/// The earlier concept notebook, whose answer check shares the Jev parser. Each
/// response after the first answers a deliberate retry of the same saved answer.
fn concept(name: &'static str, responses: Vec<String>) -> Owner {
    let mut o = Owner::new(name);
    engine::seed_fixture(&mut o.app, NOW).unwrap();
    for q in o
        .app
        .questions
        .values_mut()
        .filter(|q| q.content().kind == "choice")
    {
        q.card.due_ms = NOW + DAY_MS;
    }
    o.app.occurrence = None;
    engine::ensure_occurrence(&mut o.app, NOW, "judgment-off-recall").unwrap();
    let config = WorkConfig {
        generation: true,
        jev: true,
        critic: false,
        model: "synthetic-generator".into(),
        jev_model: "synthetic-jev".into(),
    };
    for (index, raw) in responses.into_iter().enumerate() {
        let occurrence = o.app.occurrence.as_ref().unwrap().id.clone();
        if index == 0 {
            o.call(
                "/review/answer",
                &[
                    ("occurrence_id", &occurrence),
                    ("answer", "nonexact authored response"),
                ],
            );
        } else {
            o.call("/review/retry", &[("occurrence_id", &occurrence)]);
        }
        let lease = format!("judgment-off-lease-{index:02}");
        let work = engine::claim_work(&mut o.app, o.now, &lease, &config)
            .unwrap()
            .unwrap();
        o.now += MINUTE;
        let _ = engine::finish_work(
            &mut o.app,
            &work,
            raw,
            "synthetic-jev".into(),
            Some(19_500),
            o.now,
        );
        engine::validate_app(&o.app).unwrap();
    }
    o
}

fn notebooks() -> Vec<Owner> {
    vec![
        Owner::new("empty"),
        text(),
        list(),
        mapping(),
        nato(),
        nato_result(),
        concept("concept-accepted", vec!["not json".into(), accepted()]),
        concept(
            "concept-rejected",
            vec!["{\"answers\":{}}".into(), rejected()],
        ),
        concept("concept-unsure", vec![unsure()]),
    ]
}

fn page(o: &Owner, path: &str) -> String {
    web::render(
        &o.app,
        path,
        &BTreeMap::from([("__nonce".into(), "judgment-off-nonce".into())]),
        o.now,
        None,
    )
}

fn snapshot(o: &Owner) -> Value {
    let archive = persistence::Archive::new(o.app.clone(), vec![], o.now);
    let bytes = archive.encode().unwrap();
    let attempts = o
        .app
        .practice
        .attempts
        .keys()
        .map(|id| {
            let result = MutationResult {
                data: Some(json!({"attempt_id": id})),
                ..MutationResult::default()
            };
            (id.clone(), study::wire_response(&o.app, &result).unwrap())
        })
        .collect::<BTreeMap<_, _>>();
    json!({
        "format": archive.format,
        "sha256": persistence::sha256(&bytes),
        "bytes": bytes.len(),
        "rows": persistence::rows(&o.app).unwrap(),
        "wire": attempts,
        "pages": {
            "/recall": page(o, "/recall"),
            "/history": page(o, "/history"),
            "/settings": page(o, "/settings"),
        },
    })
}

fn current() -> Value {
    Value::Object(
        notebooks()
            .iter()
            .map(|o| (o.name.to_string(), snapshot(o)))
            .collect(),
    )
}

#[test]
fn off_is_byte_identical_to_the_source_before_paid_judgment() {
    let now = current();
    if std::env::var_os("SCRY_WRITE_JUDGMENT_OFF_FIXTURE").is_some() {
        std::fs::write(FIXTURE, serde_json::to_string_pretty(&now).unwrap() + "\n").unwrap();
    }
    let before: Value = serde_json::from_str(&std::fs::read_to_string(FIXTURE).unwrap()).unwrap();
    let names = |v: &Value| v.as_object().unwrap().keys().cloned().collect::<Vec<_>>();
    assert_eq!(names(&now), names(&before));
    for (name, was) in before.as_object().unwrap() {
        let is = &now[name];
        for part in ["format", "sha256", "bytes", "rows", "wire", "pages"] {
            assert!(
                is[part] == was[part],
                "{name}: {part} differs from the source before paid judgment"
            );
        }
    }
}

/// The notebooks cover what paid judgment would touch: mismatches on text, a
/// list and a mapping that is not Spelling-strict, and every earlier format name.
#[test]
fn the_notebooks_are_representative() {
    let books = notebooks();
    let formats = books
        .iter()
        .map(|o| persistence::Archive::new(o.app.clone(), vec![], o.now).format)
        .collect::<Vec<_>>();
    assert_eq!(
        formats,
        [
            "scry-rust-durable-sqlite-v1",
            "scry-rust-durable-sqlite-study-phase-v3",
            "scry-rust-durable-sqlite-study-phase-v3",
            "scry-rust-durable-sqlite-study-phase-v3",
            "scry-rust-durable-sqlite-checkpoint-v1",
            "scry-rust-durable-sqlite-study-phase-v3",
            "scry-rust-durable-sqlite-attributed-spend-v1",
            "scry-rust-durable-sqlite-attributed-spend-v1",
            "scry-rust-durable-sqlite-attributed-spend-v1",
        ]
    );
    for name in ["text", "list", "mapping"] {
        let o = books.iter().find(|o| o.name == name).unwrap();
        let again = o
            .app
            .practice
            .attempts
            .values()
            .filter(|a| !a.disclosure && a.grade.verdict == scry::grading::Verdict::Again)
            .count();
        assert!(again >= 2, "{name} holds {again} local misses");
        assert!(
            o.app
                .practice
                .attempts
                .values()
                .all(|a| !a.effects.is_empty())
        );
    }
    let outcome = |name: &str| {
        let o = books.iter().find(|o| o.name == name).unwrap();
        let statuses = o
            .app
            .assessments
            .values()
            .map(|a| a.status.clone())
            .collect::<std::collections::BTreeSet<_>>();
        let event = o
            .app
            .events
            .iter()
            .find(|e| e.result.authority == "jev")
            .map(|e| e.result.outcome.clone());
        (o.app.spend.len(), statuses, event)
    };
    let set = |s: &[&str]| s.iter().map(|s| s.to_string()).collect();
    assert_eq!(
        outcome("concept-accepted"),
        (2, set(&["complete", "failed"]), Some("correct".into()))
    );
    assert_eq!(
        outcome("concept-rejected"),
        (2, set(&["complete", "failed"]), Some("wrong".into()))
    );
    assert_eq!(outcome("concept-unsure"), (1, set(&["unsure"]), None));
}
