//! Real shared API/engine tests for externally authored, server-graded mappings.
use scry::{api, engine, model::*, owner_token::MutationActor, persistence, study};
use serde_json::{Value, json};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;

#[test]
fn mapping_native_html_answer_form_submits_through_shared_engine() {
    let mut owner = Owner::new();
    owner.start(&[("A", "Alfa"), ("B", "Bravo")]);
    let html = scry::web::render(
        &owner.app,
        "/recall",
        &BTreeMap::from([("__nonce".into(), "synthetic-form-proof".into())]),
        NOW,
        None,
    );
    let form = html
        .split("<form")
        .find(|form| form.starts_with(" method=\"post\" action=\"/practice/answer\""))
        .unwrap()
        .split("</form>")
        .next()
        .unwrap();
    let mut fields = BTreeMap::new();
    for input in form.split("<input").skip(1) {
        let input = input.split('>').next().unwrap();
        if input.contains("type=\"hidden\"") {
            let attr = |name: &str| {
                input
                    .split(&format!("{name}=\""))
                    .nth(1)
                    .unwrap()
                    .split('"')
                    .next()
                    .unwrap()
                    .to_string()
            };
            fields.insert(attr("name"), attr("value"));
        }
    }
    assert!(fields.contains_key("revision"));
    fields.insert("answer".into(), "Alfa".into());
    let before = owner.app.clone();
    let mut forged = fields.clone();
    forged.insert("verdict".into(), "Good".into());
    assert_eq!(
        engine::mutate(&mut owner.app, "/practice/answer", &forged, NOW, "forged")
            .unwrap_err()
            .status,
        422
    );
    assert_eq!(owner.app, before);
    let result = engine::mutate(
        &mut owner.app,
        "/practice/answer",
        &fields,
        NOW,
        "rendered-form",
    )
    .unwrap();
    engine::validate_app(&owner.app).unwrap();
    let response = study::wire_response(&owner.app, &result).unwrap();
    assert_eq!(response["attempt"]["grade"]["verdict"], "Good");
    assert_eq!(response["attempt"]["assisted"], false);
}

struct Owner {
    app: App,
    op: u64,
    now: i64,
}

#[test]
fn supplied_mapping_title_never_discloses_answer_before_assistance() {
    let mut o = Owner::new();
    let title = "Alfa: NATO spelling alphabet";
    let vid = o
        .call(
            "/api/v1/library/mappings",
            json!({"data":{
                "title":title,"direction":"forward","pairs":[{"key":"A","value":"Alfa"}]
            }}),
        )
        .unwrap()["id"]
        .as_str()
        .unwrap()
        .to_string();
    o.call("/api/v1/learn", json!({"version_id":vid})).unwrap();
    let response = o.call("/api/v1/next", json!({})).unwrap();
    assert!(
        !response.to_string().contains("Alfa"),
        "Supplied title is answer-bearing metadata, not a permitted cue"
    );
    let html = scry::web::render(&o.app, "/recall", &BTreeMap::new(), o.now, None);
    assert!(
        !html.contains("Alfa"),
        "The entire active HTML must hide the supplied title"
    );
    assert!(o.app.practice.protected());
    assert_eq!(response["prompt"]["cue"], "A");
    assert_eq!(o.app.library.works.values().next().unwrap().title, title);
    let schedule = o
        .app
        .practice
        .cards
        .values()
        .next()
        .unwrap()
        .state
        .schedule
        .clone();
    o.call(
        "/api/v1/reveal",
        json!({"prompt_id":response["prompt"]["id"]}),
    )
    .unwrap();
    let answer = o.answer("Alfa");
    assert_eq!(answer["grade"]["verdict"], "Good");
    assert_eq!(answer["assisted"], true);
    assert_eq!(answer["effects"][0]["scheduled"], false);
    assert_eq!(
        o.app.practice.cards.values().next().unwrap().state.schedule,
        schedule
    );
    assert_eq!(o.app.library.works.values().next().unwrap().title, title);
}

#[test]
fn mapping_normalized_identity_reuses_version_and_study_across_legacy_hashes() {
    let mut o = Owner::new();
    let input = |value: &str, strict: bool| json!({"data":{"title":"Letter pairs","direction":"forward","spelling_strict":strict,"pairs":[{"key":"Q","value":value}]}});
    let vid = o
        .call("/api/v1/library/mappings", input("A&B", true))
        .unwrap()["id"]
        .as_str()
        .unwrap()
        .to_string();
    let sid = o.call("/api/v1/learn", json!({"version_id":vid})).unwrap()["study"]["id"].clone();
    o.call("/api/v1/next", json!({})).unwrap();
    assert_eq!(o.answer("A B")["grade"]["verdict"], "Good");
    let library = o.app.library.clone();
    let practice = o.app.practice.clone();
    assert_eq!(
        o.call("/api/v1/library/mappings", input("A B", true))
            .unwrap()["id"],
        vid
    );
    assert_eq!(
        o.call("/api/v1/learn", json!({"version_id":vid})).unwrap()["study"]["id"],
        sid
    );
    assert_eq!(
        o.app.library, library,
        "Retain exact original bytes, normalized_hash and identity"
    );
    assert_eq!(
        o.app.practice, practice,
        "Equivalent reimport never resets Card schedule or history"
    );
    let before = o.app.clone();
    assert_eq!(
        o.call("/api/v1/library/mappings", input("A B", false))
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(
        o.app, before,
        "Equivalent pairs cannot silently change grading policy"
    );
}

#[test]
fn restored_mapping_title_is_hidden_without_rewriting_historical_receipt() {
    let mut o = Owner::new();
    o.start(&[("A", "Alfa")]);
    let operation = format!("mapping-operation-{:06}", o.op);
    let title = "Alfa: supplied historical title";
    // Model a pre-fix frozen Prompt/receipt, rather than changing its history.
    o.app.practice.current.as_mut().unwrap().prompt.title = title.into();
    o.app
        .operations
        .get_mut(&operation)
        .unwrap()
        .result
        .data
        .as_mut()
        .unwrap()["prompt"]["title"] = json!(title);
    engine::validate_app(&o.app).unwrap();
    let bytes = persistence::Archive::new(o.app.clone(), vec![], o.now)
        .encode()
        .unwrap();
    o.app = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
        .unwrap()
        .app;
    let frozen = o.app.practice.clone();
    let receipt = o.app.operations[&operation].clone();
    let next = o.call("/api/v1/next", json!({})).unwrap();
    assert!(!next.to_string().contains("Alfa"));
    assert!(!scry::web::render(&o.app, "/recall", &BTreeMap::new(), o.now, None).contains("Alfa"));
    assert_eq!(o.app.practice, frozen);
    assert_eq!(o.app.operations[&operation], receipt);
    let before = o.app.clone();
    assert_eq!(
        o.operation("/api/v1/next", json!({"operation_id":operation}))
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(o.app, before);
}

#[test]
fn canonical_mapping_dedupe_does_not_mask_existing_cross_kind_hash_collision() {
    let mut o = Owner::new();
    let mapping = |value: &str| json!({"data":{"title":"Letter pairs","direction":"forward","pairs":[{"key":"Q","value":value}]}});
    o.call("/api/v1/library/mappings", mapping("A&B")).unwrap();
    o.call(
        "/api/v1/library/versions",
        json!({"data":{"title":"Independent text","kind":"text","text":"Q A B"}}),
    )
    .unwrap();
    let before = o.app.clone();
    assert_eq!(
        o.call("/api/v1/library/mappings", mapping("A B"))
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(o.app, before);
}
impl Owner {
    fn new() -> Self {
        Self {
            app: App::new("synthetic-csrf-at-least-24-chars".into()),
            op: 0,
            now: NOW,
        }
    }
    fn call(&mut self, path: &str, mut data: Value) -> AppResult<Value> {
        self.op += 1;
        data["operation_id"] = json!(format!("mapping-operation-{:06}", self.op));
        self.operation(path, data)
    }
    fn operation(&mut self, path: &str, data: Value) -> AppResult<Value> {
        let fields = api::library_fields(data)?;
        let seed = format!("seed-{}", fields.get("operation_id").unwrap());
        let r = engine::mutate_as(
            &mut self.app,
            api::mutation_path(path).unwrap(),
            &fields,
            self.now,
            &seed,
            MutationActor::Browser,
        )?;
        engine::validate_app(&self.app)?;
        Ok(study::wire_response(&self.app, &r).unwrap_or(json!({"id":r.entity_id})))
    }
    fn add(&mut self, pairs: &[(&str, &str)]) -> String {
        let units = pairs
            .iter()
            .map(|(key, value)| format!("{key}\t{value}\n"))
            .collect::<Vec<_>>();
        self.call(
            "/api/v1/library/versions",
            json!({"data":{
                "title":"NATO spelling alphabet", "kind":"keyed_mapping", "spelling_strict":true,
                "text": units.concat(), "sections":[{"heading":"Alphabet", "units":units}]
            }}),
        )
        .unwrap()["id"]
            .as_str()
            .unwrap()
            .into()
    }
    fn start(&mut self, pairs: &[(&str, &str)]) -> Value {
        let v = self.add(pairs);
        self.call("/api/v1/learn", json!({"version_id":v,"pace":"intense"}))
            .unwrap();
        self.call("/api/v1/next", json!({})).unwrap()["prompt"].clone()
    }
    fn answer(&mut self, answer: &str) -> Value {
        let prompt = self
            .app
            .practice
            .current
            .as_ref()
            .unwrap()
            .prompt
            .id
            .clone();
        self.call(
            "/api/v1/answer",
            json!({"prompt_id":prompt,"answer":answer}),
        )
        .unwrap()["attempt"]
            .clone()
    }
}

#[test]
fn nato_mapping_prompts_only_key_and_server_grades_value_without_self_rating() {
    let mut o = Owner::new();
    let p = o.start(&[("A", "Alfa"), ("B", "Bravo")]);
    assert_eq!(p["cue"], "A");
    assert_eq!(p["cue_level"], "mapping_key");
    assert_eq!(p["assisted"], false);
    assert!(p["context"].is_null());
    assert!(!p.to_string().contains("Alfa"));
    assert!(!p.to_string().contains("Bravo"));
    let html = scry::web::render(&o.app, "/recall", &BTreeMap::new(), o.now, None);
    assert!(html.contains("Type your answer"));
    assert!(!html.contains("Alfa"));
    assert!(!html.contains("Bravo"));
    let a = o.answer("alpha");
    assert_eq!(a["grade"]["verdict"], "Again");
    assert_eq!(a["grade"]["authority"]["name"], "exact-v2");
    assert_eq!(a["effects"][0]["scheduled"], true);
    assert_eq!(a["effects"][0]["assisted"], false);
    assert_eq!(a["grade"]["diff"][0]["expected"]["text"], "Alfa");
    assert_eq!(
        a["grade"]["diff"][0]["expected"]["span"],
        json!({"start":2,"end":6})
    );
    let html = scry::web::render(&o.app, "/recall", &BTreeMap::new(), o.now, None);
    assert!(html.contains("Official spelling"));
    assert!(html.contains("Alfa"));
    assert!(!html.contains("I was right"));
    assert!(!html.contains("Count as a miss"));
    assert!(!html.contains("/practice/correct"));
    let before = o.app.clone();
    assert_eq!(
        o.call(
            "/api/v1/correct",
            json!({"prompt_id":p["id"],"attempt_id":a["id"],"verdict":"Good"})
        )
        .unwrap_err()
        .status,
        422
    );
    assert_eq!(o.app, before);
    assert_eq!(o.call("/api/v1/next", json!({})).unwrap()["attempt"], a);
}

#[test]
fn nato_normalization_spelling_and_wrong_word_are_deterministic() {
    for (key, value, answer, verdict, kind) in [
        ("A", "Alfa", "  ALFA!  ", "Good", "correct"),
        ("A", "Alfa", "a.l.f.a", "Again", "extra"),
        ("A", "Alfa", "Bravo", "Again", "wrong"),
        ("J", "Juliett", "Juliet", "Again", "slip"),
        ("W", "Whiskey", "Whisky", "Again", "slip"),
        ("X", "X-ray", "x ray", "Good", "correct"),
        ("X", "X-ray", "Xray", "Again", "wrong"),
        ("A", "Alfa", "", "Again", "missing"),
    ] {
        let mut o = Owner::new();
        o.start(&[(key, value)]);
        let a = o.answer(answer);
        assert_eq!(a["grade"]["verdict"], verdict, "{answer}");
        assert!(
            a["grade"]["diff"]
                .as_array()
                .unwrap()
                .iter()
                .any(|d| d["kind"] == kind),
            "{answer}: {a}"
        );
        let original = o.app.clone();
        engine::validate_app(&o.app).unwrap();
        assert_eq!(o.app, original);
    }
}

#[test]
fn mapping_reveal_is_durable_assistance_not_unaided_success_and_forged_grades_refuse() {
    let mut o = Owner::new();
    let p = o.start(&[("J", "Juliett")]);
    for field in [
        "verdict",
        "grade",
        "rating",
        "expected",
        "spelling_strict",
        "assisted",
    ] {
        let before = o.app.clone();
        let mut data = json!({"prompt_id":p["id"],"answer":"wrong"});
        data[field] = json!("Good");
        assert_eq!(
            o.call("/api/v1/answer", data).unwrap_err().status,
            422,
            "{field}"
        );
        assert_eq!(o.app, before);
    }
    let before = o.app.practice.cards.values().next().unwrap().state.clone();
    let r = o
        .call("/api/v1/reveal", json!({"prompt_id":p["id"]}))
        .unwrap();
    assert_eq!(r["expected"][0]["text"], "Juliett");
    assert_eq!(o.app.practice.attempts.len(), 1);
    assert_eq!(o.app.practice.cards.values().next().unwrap().state, before);
    let a = o.answer("Juliett");
    assert_eq!(a["grade"]["verdict"], "Good");
    assert_eq!(a["assisted"], true);
    assert_eq!(a["effects"][0]["scheduled"], false);
    assert_eq!(o.app.practice.cards.values().next().unwrap().state, before);
    assert!(o.app.events.is_empty());
    assert!(o.app.jobs.is_empty());
}

#[test]
fn mapping_replay_conflict_archive_and_unsupported_recitation_preserve_state() {
    let mut o = Owner::new();
    let p = o.start(&[("A", "Alfa"), ("B", "Bravo")]);
    let data =
        json!({"operation_id":"mapping-atomic-answer-0001","prompt_id":p["id"],"answer":"Alfa"});
    let a = o.operation("/api/v1/answer", data.clone()).unwrap();
    let before = o.app.clone();
    assert_eq!(o.operation("/api/v1/answer", data.clone()).unwrap(), a);
    assert_eq!(o.app, before);
    let mut changed = data;
    changed["answer"] = json!("Bravo");
    assert_eq!(
        o.operation("/api/v1/answer", changed).unwrap_err().status,
        409
    );
    assert_eq!(o.app, before);
    let sid = p["study_id"].clone();
    assert_eq!(
        o.call(
            "/api/v1/next",
            json!({"study_id":sid,"recite":"whole","after":p["id"]})
        )
        .unwrap_err()
        .status,
        422
    );
    assert_eq!(o.app, before);
    let rows = persistence::rows(&o.app).unwrap();
    let restored = persistence::app_from_rows(&rows).unwrap();
    assert_eq!(restored, o.app);
}

#[test]
fn canonical_nato_all_26_pairs_are_independently_scheduled_and_repeat_learn_keeps_them() {
    let mut o = Owner::new();
    let manifest: Value =
        serde_json::from_str(include_str!("../examples/nato-alphabet.json")).unwrap();
    let v = o
        .call("/api/v1/library/mappings", json!({"data":manifest}))
        .unwrap()["id"]
        .as_str()
        .unwrap()
        .to_string();
    let sid = o
        .call("/api/v1/learn", json!({"version_id":v,"pace":"intense"}))
        .unwrap()["study"]["id"]
        .as_str()
        .unwrap()
        .to_string();
    let pairs = manifest["pairs"].as_array().unwrap();
    let mut seen = std::collections::BTreeSet::new();
    for _ in 0..80 {
        let after = o
            .app
            .practice
            .current
            .as_ref()
            .and_then(|p| p.attempt_id.as_ref().map(|_| p.prompt.id.clone()));
        let data = after.map_or_else(|| json!({}), |after| json!({"after":after}));
        let next = o.call("/api/v1/next", data).unwrap();
        if next["prompt"].is_null() {
            o.now += DAY_MS + 1;
            continue;
        }
        let key = next["prompt"]["cue"].as_str().unwrap().to_string();
        let pair = pairs.iter().find(|p| p["key"] == key).unwrap();
        let a = o.answer(pair["value"].as_str().unwrap());
        assert_eq!(a["grade"]["verdict"], "Good");
        assert_eq!(a["assisted"], false);
        seen.insert(key);
        if seen.len() == 26 {
            break;
        }
    }
    assert_eq!(seen.len(), 26);
    assert_eq!(o.app.practice.cards.len(), 26);
    assert!(
        o.app
            .practice
            .cards
            .values()
            .all(|c| c.direction == study::Direction::Forward
                && c.state.schedule.last_review_ms.is_some())
    );
    let cards = o.app.practice.cards.clone();
    assert_eq!(
        o.call("/api/v1/learn", json!({"version_id":v,"pace":"light"}))
            .unwrap()["study"]["id"],
        sid
    );
    assert_eq!(o.app.practice.cards, cards);
    let bytes = persistence::Archive::new(o.app.clone(), vec![], o.now)
        .encode()
        .unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        o.app
    );
}

#[test]
fn mapping_feedback_names_other_key_and_preserves_same_day_schedule() {
    let mut o = Owner::new();
    let p = o.start(&[("A", "Alfa"), ("B", "Bravo")]);
    let a = o
        .call(
            "/api/v1/answer",
            json!({"prompt_id":p["id"],"answer":"Bravo"}),
        )
        .unwrap();
    assert_eq!(a["mapping_feedback"]["kind"], "wrong_key");
    assert_eq!(a["mapping_feedback"]["answered_key"], "B");
    assert_eq!(a["mapping_feedback"]["expected"], "Alfa");
    let c = o.app.practice.cards.values().next().unwrap().clone();
    o.call("/api/v1/next", json!({"after":p["id"],"card_id":c.id}))
        .unwrap();
    let corrected_answer = o.answer("Alfa");
    assert_eq!(corrected_answer["effects"][0]["scheduled"], false);
    assert_eq!(corrected_answer["effects"][0]["extra_practice"], true);
    assert_eq!(o.app.practice.cards[&c.id].state.schedule, c.state.schedule);
}

#[test]
fn supplied_mapping_has_a_native_learn_action_in_library() {
    let mut o = Owner::new();
    let v = o.add(&[("A", "Alfa"), ("B", "Bravo")]);
    let html = scry::web::render(
        &o.app,
        &format!("/library/versions/{v}"),
        &BTreeMap::new(),
        o.now,
        None,
    );
    assert!(
        html.contains("action=\"/practice/learn\""),
        "Mapping Library detail must offer native Learn"
    );
    assert!(!html.contains("Practice for this kind is not available yet."));
    assert!(html.contains("<dt>A</dt><dd>Alfa</dd>"));
    assert!(
        o.app.practice.is_empty(),
        "Reading the supplied pair reference never creates Study evidence"
    );
}
