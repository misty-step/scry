//! A text's practice screens speak of phrases, SPEC's word for a text's Units,
//! and leave the earlier notebook to the Menu. Other Kinds keep their words.
use scry::{engine, model::*, persistence, study::Cue, web};
use serde_json::{Value, json};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;
const CANTICLE: [&str; 4] = [
    "Still waters hold the morning light,\n",
    "and quiet hills receive the rain;\n",
    "the patient reed bends low at night,\n",
    "and rises with the dawn again.\n",
];

struct Owner {
    app: App,
    n: u64,
}
impl Owner {
    fn act(&mut self, path: &str, pairs: &[(&str, &str)]) -> MutationResult {
        self.n += 1;
        let mut f = BTreeMap::from([
            ("csrf".to_string(), self.app.csrf.clone()),
            (
                "operation_id".to_string(),
                format!("text-words-operation-{:06}", self.n),
            ),
        ]);
        f.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.to_string())));
        let seed = format!("text-words-seed-{:06}", self.n);
        let result = engine::mutate(&mut self.app, path, &f, NOW, &seed).unwrap();
        engine::validate_app(&self.app).unwrap();
        result
    }
    fn page(&self, path: &str) -> String {
        web::render(&self.app, path, &BTreeMap::new(), NOW, None)
    }
    fn add(&mut self, data: Value) -> String {
        let path = if data.get("pairs").is_some() {
            "/library/mapping/add"
        } else {
            "/library/version/add"
        };
        self.act(path, &[("data", &data.to_string())])
            .entity_id
            .unwrap()
    }
}
fn owner() -> Owner {
    Owner {
        app: App::new("text-words-synthetic-csrf-0123456789".into()),
        n: 0,
    }
}
fn canticle() -> Value {
    json!({"title":"Synthetic Canticle","name":"Synthetic v1","kind":"text","text":CANTICLE.concat(),
        "sections":[{"heading":"Opening","units":CANTICLE}]})
}
fn lamps() -> Value {
    json!({"title":"Synthetic harbour lamps","direction":"forward","pairs":[
        {"key":"Red lamp","value":"Stop"},{"key":"Green lamp","value":"Enter"}]})
}

#[test]
fn a_texts_version_page_counts_new_phrases_not_units() {
    let mut o = owner();
    let id = o.add(canticle());
    let path = format!("/library/versions/{id}");
    let fresh = o.page(&path);
    assert!(fresh.contains("<summary>Pace: 6 new phrases a day</summary>"));
    assert!(fresh.contains("<legend>New phrases each day</legend>"));
    o.act("/practice/study/start", &[("version_id", &id)]);
    let studied = o.page(&path);
    assert!(studied.contains("Pause stops new phrases; due practice remains."));
    assert!(studied.contains("<legend>Pace · new phrases each day</legend>"));
    for page in [&fresh, &studied] {
        // The edition's count line keeps "Units": tests/agent_access.rs pins it.
        assert!(!page.contains("new Units") && !page.contains("New Units"));
    }
    // A mapping's Units are not phrases.
    let mut o = owner();
    let lamps = o.add(lamps());
    o.act("/practice/study/start", &[("version_id", &lamps)]);
    let mapping = o.page(&format!("/library/versions/{lamps}"));
    assert!(mapping.contains("Pause stops new Units; due practice remains."));
    assert!(mapping.contains("<legend>Pace · new Units each day</legend>"));
}

#[test]
fn a_texts_study_stream_has_no_concept_practice_link() {
    let mut o = owner();
    let id = o.add(canticle());
    o.act("/practice/study/start", &[("version_id", &id)]);
    let held = o.app.practice.current.clone().unwrap();
    assert_eq!(held.prompt.version_id, id);
    // Source study, then the Quiz, then its result.
    let source = o.page("/recall");
    assert!(source.contains("Try from memory"));
    o.act("/practice/quiz", &[("prompt_id", &held.prompt.id)]);
    let quiz = o.page("/recall");
    let prompt = o.app.practice.current.clone().unwrap().prompt.id;
    o.act(
        "/practice/answer",
        &[("prompt_id", &prompt), ("answer", "Still waters")],
    );
    let result = o.page("/recall");
    assert!(result.contains("data-state=\"exact-result\""));
    for page in [&source, &quiz, &result] {
        assert!(!page.contains("Concept practice"));
        assert!(!page.contains("class=\"text-link exact-notebook\""));
        // The Menu still reaches the earlier notebook.
        assert!(page.contains("<a href=\"/notebook\">Earlier practice</a>"));
    }
}

/// History of a text answered under the earlier cue ladder names its read cue
/// "Read the phrase"; the list item read the same way keeps "Read the Unit".
#[test]
fn a_texts_read_cue_is_the_phrase() {
    let fixture: Value =
        serde_json::from_str(include_str!("fixtures/cue-ladder-legacy.json")).unwrap();
    let saved = &fixture["ladder"]["states"]["held_list_item"];
    let mut app = persistence::Archive::decode(
        saved["archive"].as_str().unwrap().as_bytes(),
        saved["sha256"].as_str().unwrap(),
    )
    .unwrap()
    .app;
    let kind =
        |app: &App, a: &scry::study::Attempt| app.library.versions[&a.prompt.version_id].kind;
    let read: Vec<_> = app
        .practice
        .attempts
        .values()
        .filter(|a| a.prompt.cue_level == Cue::Read && !a.prompt.cue.is_empty())
        .map(|a| kind(&app, a))
        .collect();
    assert!(read.contains(&scry::library::Kind::Text));
    assert!(read.contains(&scry::library::Kind::OrderedList));
    // History is read with nothing held; a held Prompt protects it.
    app.practice.current = None;
    let history = web::render(&app, "/history", &BTreeMap::new(), NOW, None);
    assert!(history.contains("<p class=\"eyebrow\">Read the phrase</p>"));
    assert_eq!(
        history
            .matches("<p class=\"eyebrow\">Read the Unit</p>")
            .count(),
        read.iter()
            .filter(|k| **k != scry::library::Kind::Text)
            .count()
    );
}
