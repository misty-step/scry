//! SPEC § Keyed mappings, § Baby-care readiness and the mapping parts of
//! § Grading, through the shared engine every client uses. Synthetic content.
//! Read through JSON only, so the same file also runs against earlier source.
use scry::{
    engine::{self, WorkConfig},
    model::*,
    owner_token::MutationActor,
    persistence, study, web,
};
use serde_json::{Value, json};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;
const MINUTE: i64 = 60_000;
const MARKER: &str = "mapping_compatibility/v1";
const FORMAT: &str = "scry-rust-durable-sqlite-mapping-v6";
const YEAR: &str = "1;unicode=17.0.0;mapping=1;year=1";

struct Owner {
    app: App,
    n: u64,
    now: i64,
    jev: bool,
}
impl Owner {
    fn new() -> Self {
        Self {
            app: App::new("mapping-spec-synthetic-csrf-0123456789".into()),
            n: 0,
            now: NOW,
            jev: false,
        }
    }
    fn judged() -> Self {
        let mut o = Self::new();
        o.jev = true;
        o.app.preferences.paid_judgment = true;
        o
    }
    fn try_call(&mut self, path: &str, pairs: &[(&str, &str)]) -> AppResult<MutationResult> {
        self.n += 1;
        let operation = format!("mapping-spec-operation-{:06}", self.n);
        let mut f = BTreeMap::from([
            ("csrf".to_string(), self.app.csrf.clone()),
            ("operation_id".to_string(), operation.clone()),
        ]);
        f.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.to_string())));
        let seed = format!("mapping-spec-seed-{operation}");
        let result = engine::mutate_judged(
            &mut self.app,
            path,
            &f,
            self.now,
            &seed,
            MutationActor::Browser,
            self.jev,
        );
        self.sound();
        result
    }
    fn call(&mut self, path: &str, pairs: &[(&str, &str)]) -> MutationResult {
        self.try_call(path, pairs)
            .unwrap_or_else(|e| panic!("{path}: {} {}", e.status, e.message))
    }
    fn sound(&self) {
        engine::validate_app(&self.app).unwrap();
        let rows = persistence::rows(&self.app).unwrap();
        assert_eq!(persistence::app_from_rows(&rows).unwrap(), self.app);
    }
    fn add(&mut self, manifest: Value) -> AppResult<String> {
        self.try_call("/library/mapping/add", &[("data", &manifest.to_string())])
            .map(|r| r.entity_id.unwrap())
    }
    fn learn(&mut self, vid: &str, pace: &str) -> String {
        let r = self.call("/practice/learn", &[("version_id", vid), ("pace", pace)]);
        r.entity_id.unwrap()
    }
    fn held(&self) -> Value {
        serde_json::to_value(self.app.practice.current.as_ref().unwrap()).unwrap()
    }
    fn held_card(&self) -> Value {
        let id = self.held()["targets"][0]["card_id"]
            .as_str()
            .unwrap()
            .to_string();
        serde_json::to_value(&self.app.practice.cards[&id]).unwrap()
    }
    fn next(&mut self, sid: &str) -> Value {
        let mut fields = vec![("study_id", sid.to_string())];
        if let Some(p) = &self.app.practice.current {
            fields.push(("after", p.prompt.id.clone()));
        }
        let fields = fields
            .iter()
            .map(|(k, v)| (*k, v.as_str()))
            .collect::<Vec<_>>();
        let r = self.call("/practice/next", &fields);
        study::wire_response(&self.app, &r).unwrap()
    }
    fn quiz(&mut self) -> Value {
        let pid = self
            .app
            .practice
            .current
            .as_ref()
            .unwrap()
            .prompt
            .id
            .clone();
        let r = self.call("/practice/quiz", &[("prompt_id", &pid)]);
        study::wire_response(&self.app, &r).unwrap()
    }
    fn answer(&mut self, text: &str) -> Value {
        if self.app.practice.current.as_ref().unwrap().source_visible() {
            self.quiz();
        }
        let pid = self
            .app
            .practice
            .current
            .as_ref()
            .unwrap()
            .prompt
            .id
            .clone();
        let r = self.call("/practice/answer", &[("prompt_id", &pid), ("answer", text)]);
        self.now += MINUTE;
        study::wire_response(&self.app, &r).unwrap()
    }
    /// Ask the same Card again after a held result: optional early practice.
    fn again(&mut self) {
        let p = self.app.practice.current.as_ref().unwrap();
        let (pid, cid) = (p.prompt.id.clone(), p.targets[0].card_id.clone());
        self.call("/practice/next", &[("after", &pid), ("card_id", &cid)]);
    }
    fn page(&self) -> String {
        web::render(
            &self.app,
            "/recall",
            &BTreeMap::from([("__nonce".into(), "mapping-spec-nonce".into())]),
            self.now,
            None,
        )
    }
    fn cards(&self) -> Vec<Value> {
        self.app
            .practice
            .cards
            .values()
            .map(|c| serde_json::to_value(c).unwrap())
            .collect()
    }
}
fn verdict(answer: &Value) -> &str {
    answer["attempt"]["grade"]["verdict"].as_str().unwrap()
}
fn sky(direction: &str) -> Value {
    json!({"title":"Synthetic sky","direction":direction,"pairs":[
        {"key":"North star","value":"Polaris"},
        {"key":"Red planet","value":"Mars"},
        {"key":"Ringed planet","value":"Saturn"}
    ]})
}

// --- Direction ---------------------------------------------------------------

#[test]
fn a_mapping_declared_both_ways_is_practised_forward_and_in_reverse() {
    let mut o = Owner::new();
    let vid = o.add(sky("both")).unwrap();
    let version = serde_json::to_value(&o.app.library.versions[&vid]).unwrap();
    assert_eq!(
        version["direction"], "both",
        "the Version stores what it declares"
    );
    let sid = o.learn(&vid, "steady");

    // Forward: studied, then quizzed from the key alone.
    let source = o.next(&sid);
    assert_eq!(source["phase"], "study");
    assert_eq!(o.held_card()["direction"], "forward");
    let quiz = o.quiz();
    assert_eq!(quiz["prompt"]["cue_level"], "mapping_key");
    assert_eq!(quiz["prompt"]["cue"], "North star");
    assert_eq!(verdict(&o.answer("Polaris")), "Good");

    // The reverse Card of the pair just answered waits while another pair is new.
    o.next(&sid);
    assert_eq!(o.held_card()["direction"], "forward");
    assert_eq!(o.held_card()["unit_index"], 1);
    assert_eq!(verdict(&o.answer("Mars")), "Good");

    // Reverse: its own Source study, then a Quiz that shows the value alone.
    let source = o.next(&sid);
    assert_eq!(source["phase"], "study");
    assert_eq!(o.held_card()["direction"], "reverse");
    assert_eq!(o.held_card()["unit_index"], 0);
    let page = o.page();
    assert!(
        page.contains("<h1>Polaris</h1>"),
        "reverse study leads with the value"
    );
    let quiz = o.quiz();
    assert_eq!(quiz["prompt"]["cue_level"], "mapping_value");
    assert_eq!(quiz["prompt"]["cue"], "Polaris");
    assert_eq!(quiz["prompt"]["title"], "Recall the matching key");
    // Protected recall: the key is the answer, and the forward Card's Prompt,
    // its result and its history are answer-bearing for it.
    assert!(o.app.practice.protected());
    assert!(!quiz.to_string().contains("North star"));
    let page = o.page();
    assert!(
        !page.contains("North star"),
        "the reverse Quiz page hides its key"
    );
    assert!(page.contains("Recall the matching key"));
    for path in ["/history", "/library", &format!("/library/versions/{vid}")] {
        let html = web::render(&o.app, path, &BTreeMap::new(), o.now, None);
        assert!(
            !html.contains("North star"),
            "{path} shows the reverse answer"
        );
    }
    assert!(study::history(&o.app).is_err());
    let due = study::due(&o.app, o.now).to_string();
    assert!(!due.contains("North star") && !due.contains("Polaris"));

    let answered = o.answer("north star");
    assert_eq!(verdict(&answered), "Good");
    assert_eq!(answered["attempt"]["prompt"]["cue_level"], "mapping_value");
    assert_eq!(answered["mapping_feedback"]["expected"], "North star");
    assert_eq!(answered["mapping_feedback"]["direction"], "reverse");
    let page = o.page();
    assert!(page.contains(
        "<span>Polaris</span> <span aria-hidden=\"true\">→</span> <strong>North star</strong>"
    ));

    // Each Card keeps its own schedule; the forward Card is untouched.
    let cards = o.cards();
    assert_eq!(cards.len(), 4, "two Units, each in both directions");
    assert_eq!(
        cards.iter().filter(|c| c["direction"] == "reverse").count(),
        2
    );
    // The result's evidence counts this direction only.
    let attempt: study::Attempt = serde_json::from_value(answered["attempt"].clone()).unwrap();
    let evidence = scry::result_progress::summarize(&o.app, &attempt);
    assert_eq!((evidence.correct, evidence.unaided), (1, 1));
}

#[test]
fn a_reverse_answer_that_is_another_pairs_key_is_the_wrong_key() {
    let mut o = Owner::new();
    let vid = o.add(sky("both")).unwrap();
    let sid = o.learn(&vid, "light");
    o.next(&sid);
    o.answer("Polaris");
    o.next(&sid);
    o.answer("Mars");
    o.next(&sid);
    assert_eq!(o.held_card()["direction"], "reverse");
    assert_eq!(o.held_card()["unit_index"], 0);
    let answered = o.answer("Red planet");
    assert_eq!(verdict(&answered), "Again");
    assert_eq!(answered["mapping_feedback"]["kind"], "wrong_key");
    assert_eq!(answered["mapping_feedback"]["answered_value"], "Mars");
    assert!(o.page().contains("That answer is the key for Mars."));
}

#[test]
fn a_pair_counts_once_against_pace_in_both_directions() {
    let mut o = Owner::new();
    let pairs = (0..5)
        .map(|i| json!({"key":format!("Synthetic key {i}"),"value":format!("Synthetic value {i}")}))
        .collect::<Vec<_>>();
    let vid = o
        .add(json!({"title":"Synthetic paced pairs","direction":"both","pairs":pairs}))
        .unwrap();
    let sid = o.learn(&vid, "light");
    let mut asked = Vec::new();
    loop {
        let r = o.next(&sid);
        if r["prompt"].is_null() {
            break;
        }
        let card = o.held_card();
        asked.push((
            card["unit_index"].as_u64().unwrap(),
            card["direction"].clone(),
        ));
        let unit = card["unit_index"].as_u64().unwrap();
        let answer = if card["direction"] == "reverse" {
            format!("Synthetic key {unit}")
        } else {
            format!("Synthetic value {unit}")
        };
        assert_eq!(verdict(&o.answer(&answer)), "Good");
        assert!(asked.len() < 20, "practice ends: {asked:?}");
    }
    // Light Pace is three Units a day: three pairs, each in both directions.
    let cards = o.cards();
    assert_eq!(cards.len(), 6);
    assert_eq!(asked.len(), 6, "{asked:?}");
    // The other direction of the pair just answered waits while another Card
    // is eligible; the last pair's reverse comes when nothing else is.
    let order = asked
        .iter()
        .map(|(unit, direction)| (*unit, direction.as_str().unwrap()))
        .collect::<Vec<_>>();
    assert_eq!(
        order,
        [
            (0, "forward"),
            (1, "forward"),
            (0, "reverse"),
            (1, "reverse"),
            (2, "forward"),
            (2, "reverse")
        ]
    );
}

#[test]
fn a_forward_mapping_has_no_reverse_card_and_keeps_its_stored_shape() {
    let mut o = Owner::new();
    let vid = o.add(sky("forward")).unwrap();
    let version = serde_json::to_value(&o.app.library.versions[&vid]).unwrap();
    assert!(version.get("direction").is_none() && version.get("safety_bearing").is_none());
    let sid = o.learn(&vid, "intense");
    for value in ["Polaris", "Mars", "Saturn"] {
        o.next(&sid);
        assert_eq!(verdict(&o.answer(value)), "Good");
    }
    assert!(o.cards().iter().all(|c| c["direction"] == "forward"));
    assert_eq!(o.cards().len(), 3);
    let rows = persistence::rows(&o.app).unwrap();
    assert!(!rows.contains_key(MARKER));
    assert_ne!(
        persistence::Archive::new(o.app.clone(), vec![], o.now).format,
        FORMAT
    );
}

#[test]
fn both_directions_need_unique_values_and_no_safety_bearing_pair() {
    let mut o = Owner::new();
    let before = o.app.clone();
    let duplicate = json!({"title":"Synthetic duplicate values","direction":"both","pairs":[
        {"key":"First key","value":"Same value"},{"key":"Second key","value":"same  value"}]});
    let e = o.add(duplicate).unwrap_err();
    assert_eq!(e.status, 422);
    assert!(
        e.message.contains("needs a different value for every pair"),
        "{}",
        e.message
    );
    let safety = json!({"title":"Synthetic safety both","direction":"both","pairs":[
        {"key":"A synthetic hazard","value":"step back","safety_bearing":true}]});
    let e = o.add(safety).unwrap_err();
    assert_eq!(e.status, 422);
    assert!(
        e.message.contains("practised forward only"),
        "{}",
        e.message
    );
    let text = json!({"title":"Synthetic text","kind":"text","text":"Words.\n","direction":"both"});
    let e = o
        .try_call("/library/version/add", &[("data", &text.to_string())])
        .unwrap_err();
    assert_eq!(e.status, 422);
    let reverse_only = json!({"title":"Synthetic reverse","direction":"reverse","pairs":[{"key":"K","value":"V"}]});
    assert_eq!(o.add(reverse_only).unwrap_err().status, 422);
    assert_eq!(o.app.library, before.library, "nothing was admitted");
    // The same pairs in another direction are another Version, never a silent reuse.
    o.add(sky("forward")).unwrap();
    let e = o.add(sky("both")).unwrap_err();
    assert_eq!(e.status, 409);
    assert!(e.message.contains("different direction"), "{}", e.message);
}

#[test]
fn a_notebook_practised_both_ways_is_named_for_it_and_round_trips() {
    let mut o = Owner::new();
    let vid = o.add(sky("both")).unwrap();
    let rows = persistence::rows(&o.app).unwrap();
    assert_eq!(rows.get(MARKER).map(String::as_str), Some("1"));
    let sid = o.learn(&vid, "steady");
    o.next(&sid);
    o.answer("Polaris");
    o.next(&sid);
    o.answer("Mars");
    o.next(&sid);
    o.answer("North star");
    let rows = persistence::rows(&o.app).unwrap();
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), o.app);
    let mut unmarked = rows.clone();
    unmarked.remove(MARKER);
    assert!(persistence::app_from_rows(&unmarked).is_err());
    let archive = persistence::Archive::new(o.app.clone(), vec![], o.now);
    assert_eq!(archive.format, FORMAT);
    let bytes = archive.encode().unwrap();
    let decoded = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(decoded.app, o.app);
    assert_eq!(decoded.encode().unwrap(), bytes);
    for earlier in [
        "scry-rust-durable-sqlite-library-v5",
        "scry-rust-durable-sqlite-study-phase-v3",
    ] {
        let mut forged = archive.clone();
        forged.format = earlier.into();
        assert!(forged.validate().is_err(), "{earlier}");
    }
}

#[test]
fn a_mapping_is_never_recited_as_a_whole_set() {
    let mut o = Owner::new();
    let vid = o.add(sky("both")).unwrap();
    let sid = o.learn(&vid, "intense");
    o.next(&sid);
    o.answer("Polaris");
    let before = o.app.clone();
    let after = o.app.practice.current.as_ref().unwrap().prompt.id.clone();
    for (path, fields) in [
        (
            "/practice/next",
            vec![
                ("after", after.as_str()),
                ("study_id", sid.as_str()),
                ("recite", "whole"),
            ],
        ),
        (
            "/practice/lesson/next",
            vec![("after", after.as_str()), ("mode", "whole")],
        ),
        (
            "/practice/lesson/next",
            vec![("after", after.as_str()), ("mode", "join")],
        ),
    ] {
        let e = o.try_call(path, &fields).unwrap_err();
        assert_eq!(e.status, 422, "{path} {fields:?}");
    }
    assert_eq!(o.app.practice, before.practice);
}

// --- Years -------------------------------------------------------------------

fn years(o: &mut Owner, strict: bool) -> String {
    let vid = o
        .add(json!({"title":"Synthetic dates","direction":"forward","spelling_strict":strict,"pairs":[
            {"key":"Synthetic battle","value":"1066"},
            {"key":"Synthetic coronation","value":"AD 800"},
            {"key":"Synthetic founding","value":"44 BC"}
        ]}))
        .unwrap();
    o.learn(&vid, "intense")
}

#[test]
fn a_year_is_accepted_with_or_without_ad_or_ce_and_graded_as_an_exact_number() {
    let mut o = Owner::new();
    let sid = years(&mut o, true);
    o.next(&sid);
    let first = o.answer("1066 AD");
    assert_eq!(verdict(&first), "Good");
    assert_eq!(
        first["attempt"]["grade"]["authority"]["grader_version"],
        YEAR
    );
    for (answer, expected) in [
        ("AD 1066", "Good"),
        ("1066 CE", "Good"),
        ("1066 a.d.", "Good"),
        ("1066", "Good"),
        ("1067", "Again"),
        ("1067 AD", "Again"),
        ("1066 BC", "Again"),
        ("10 66", "Again"),
    ] {
        o.again();
        let r = o.answer(answer);
        assert_eq!(verdict(&r), expected, "{answer}");
        let version = r["attempt"]["grade"]["authority"]["grader_version"].clone();
        // The year rule is recorded only where it turned a miss into Good.
        assert_eq!(
            version == YEAR,
            expected == "Good" && answer != "1066",
            "{answer}"
        );
    }
    o.next(&sid);
    assert_eq!(verdict(&o.answer("800")), "Good");
    o.again();
    assert_eq!(verdict(&o.answer("800 CE")), "Good");
    o.next(&sid);
    // A year before the era is not the same number with its marker dropped.
    assert_eq!(verdict(&o.answer("44")), "Again");
    o.again();
    assert_eq!(verdict(&o.answer("44 AD")), "Again");
    let rows = persistence::rows(&o.app).unwrap();
    assert_eq!(rows.get(MARKER).map(String::as_str), Some("1"));
}

#[test]
fn with_paid_judgment_on_a_different_year_is_a_local_miss_never_sent() {
    let mut o = Owner::judged();
    let sid = years(&mut o, false);
    o.next(&sid);
    let r = o.answer("1067 AD");
    assert_eq!(verdict(&r), "Again");
    assert!(
        r["attempt"]["judgment"].is_null(),
        "an exact number is not sent to Jev"
    );
    assert!(o.app.practice.checks.is_empty());
    // A year before the common era is graded as written, by Scry alone.
    for answer in ["1066 BC", "1066 B.C.E."] {
        o.again();
        let r = o.answer(answer);
        assert_eq!(verdict(&r), "Again", "{answer}");
        assert!(r["attempt"]["judgment"].is_null(), "{answer}");
    }
    o.next(&sid);
    let r = o.answer("800 BC");
    assert_eq!(verdict(&r), "Again");
    assert!(r["attempt"]["judgment"].is_null());
    // "44 BC" is a year: "44 BCE" is not the same written year, and is never sent.
    loop {
        o.next(&sid);
        let unit = o.held()["prompt"]["unit_ids"][0].clone();
        let founding = serde_json::to_value(
            &o.app.library.versions[&o.held()["prompt"]["version_id"]
                .as_str()
                .unwrap()
                .to_string()]
                .sections[0]
                .units[2]
                .id,
        )
        .unwrap();
        if unit == founding {
            break;
        }
        o.answer(if o.held_card()["unit_index"] == 0 {
            "1066"
        } else {
            "AD 800"
        });
    }
    let r = o.answer("44 BCE");
    assert_eq!(verdict(&r), "Again");
    assert!(r["attempt"]["judgment"].is_null());
    let r = {
        o.again();
        o.answer("44 BC")
    };
    assert_eq!(verdict(&r), "Good");
    assert!(o.app.practice.checks.is_empty() && o.app.spend.is_empty());
    o.again();
    let r = o.answer("the year of the synthetic founding");
    assert!(
        !r["attempt"]["judgment"].is_null(),
        "other wording is still judged"
    );
}

// --- Wrong key ---------------------------------------------------------------

fn config() -> WorkConfig {
    WorkConfig {
        generation: false,
        jev: true,
        critic: false,
        model: String::new(),
        jev_model: "synthetic-jev".into(),
    }
}

/// SPEC § Keyed mappings: "An answer that is exactly another pair's value, or
/// another pair's key in reverse, is Again and is identified as the wrong key.
/// It is never accepted as a near match and never sent to be judged."
#[test]
fn with_paid_judgment_on_the_wrong_key_is_a_local_miss_never_sent() {
    let mut o = Owner::judged();
    let vid = o
        .add(
            json!({"title":"Synthetic harbour","direction":"both","spelling_strict":false,"pairs":[
                {"key":"Before leaving the berth","value":"check the bilge pump"},
                {"key":"In fog","value":"sound the horn"}
            ]}),
        )
        .unwrap();
    let sid = o.learn(&vid, "intense");
    o.next(&sid);
    // Forward: another pair's value, under the exact grader's normalization.
    let r = o.answer("Sound  the HORN");
    assert_eq!(verdict(&r), "Again");
    assert!(
        r["attempt"]["judgment"].is_null(),
        "never sent to be judged"
    );
    assert_eq!(r["attempt"]["grade"]["authority"]["name"], "exact-v2");
    assert_eq!(r["mapping_feedback"]["kind"], "wrong_key");
    assert_eq!(r["mapping_feedback"]["answered_key"], "In fog");
    assert_eq!(r["attempt"]["effects"][0]["scheduled"], true);
    assert!(o.app.practice.checks.is_empty() && o.app.spend.is_empty());
    // Reverse: another pair's key.
    loop {
        o.next(&sid);
        if o.held_card()["direction"] == "reverse" && o.held_card()["unit_index"] == 0 {
            break;
        }
        let unit = o.held_card()["unit_index"].as_u64().unwrap() as usize;
        let answer = if o.held_card()["direction"] == "reverse" {
            ["Before leaving the berth", "In fog"][unit]
        } else {
            ["check the bilge pump", "sound the horn"][unit]
        };
        o.answer(answer);
    }
    let r = o.answer("in fog");
    assert_eq!(verdict(&r), "Again");
    assert!(r["attempt"]["judgment"].is_null());
    assert_eq!(r["mapping_feedback"]["kind"], "wrong_key");
    assert_eq!(r["mapping_feedback"]["answered_value"], "sound the horn");
    assert!(o.app.practice.checks.is_empty() && o.app.spend.is_empty());
    // Any other wrong answer is still judged.
    o.again();
    let r = o.answer("before casting off");
    assert!(!r["attempt"]["judgment"].is_null());
    assert_eq!(o.app.practice.checks.len(), 1);
}

// --- Safety-bearing pairs ----------------------------------------------------

fn safety(o: &mut Owner, strict: bool) -> String {
    let vid = o
        .add(json!({"title":"Synthetic workshop safety","direction":"forward","spelling_strict":strict,"pairs":[
            {"key":"Synthetic lathe running, loose sleeve. What first?","value":"stop the lathe","safety_bearing":true},
            {"key":"Synthetic workshop year","value":"1999"}
        ]}))
        .unwrap();
    let version = serde_json::to_value(&o.app.library.versions[&vid]).unwrap();
    assert_eq!(version["safety_bearing"], json!([0]));
    o.learn(&vid, "intense")
}

#[test]
fn a_safety_bearing_pair_keeps_its_question_and_cue_in_study_and_quiz() {
    let mut o = Owner::new();
    let sid = safety(&mut o, true);
    let source = o.next(&sid);
    assert_eq!(source["prompt"]["safety_bearing"], true);
    let page = o.page();
    assert!(page.contains("Synthetic lathe running, loose sleeve. What first?"));
    assert!(page.contains("Safety practice: answer with the safe practice itself."));
    let quiz = o.quiz();
    assert_eq!(quiz["prompt"]["safety_bearing"], true);
    assert_eq!(
        quiz["prompt"]["cue"],
        "Synthetic lathe running, loose sleeve. What first?"
    );
    let page = o.page();
    assert!(page.contains("Safety practice: answer with the safe practice itself."));
    assert!(!page.contains("stop the lathe"));
    // A negation, an added word or another practice is a miss; Scry judges no approximation.
    assert_eq!(verdict(&o.answer("do not stop the lathe")), "Again");
    assert!(o.page().contains(
        "A safety answer states the safe practice itself: no other practice, negation or added words."
    ));
    for answer in [
        "stop the lathe and reach in",
        "slow the lathe",
        "Stop the  lathe",
    ] {
        o.again();
        let expected = if answer == "Stop the  lathe" {
            "Good"
        } else {
            "Again"
        };
        assert_eq!(verdict(&o.answer(answer)), expected, "{answer}");
    }
    // The pair that is not flagged carries no cue.
    o.next(&sid);
    assert!(o.held()["prompt"].get("safety_bearing").is_none());
    assert!(!o.page().contains("Safety practice"));
    assert_eq!(
        persistence::rows(&o.app)
            .unwrap()
            .get(MARKER)
            .map(String::as_str),
        Some("1")
    );
}

#[test]
fn with_paid_judgment_on_a_safety_bearing_answer_is_judged_as_the_safe_practice_itself() {
    let mut o = Owner::judged();
    let sid = safety(&mut o, false);
    o.next(&sid);
    o.answer("halt the lathe");
    let work = engine::claim_check_work(&mut o.app, o.now, "mapping-spec-lease-2", &config())
        .unwrap()
        .expect("a queued Check");
    let objective = work.request["state"]["objective"]
        .as_str()
        .unwrap()
        .to_string();
    assert!(objective.contains("safe practice"), "{objective}");
    assert!(objective.contains("dangerous alternative"), "{objective}");
    assert!(objective.contains("added unsafe words"), "{objective}");
    assert!(objective.contains("never whether another practice is clinically acceptable"));
}

// --- Every entry, replay and history -------------------------------------------

#[test]
fn studys_mix_and_a_lesson_serve_both_directions_and_history_names_reverse() {
    for entry in ["mix", "lesson"] {
        let mut o = Owner::new();
        let vid = o.add(sky("both")).unwrap();
        let start = |o: &mut Owner| match entry {
            "mix" => o.call("/practice/review/start", &[]),
            _ => o.call("/practice/lesson/start", &[("version_id", &vid)]),
        };
        start(&mut o);
        let mut asked = Vec::new();
        for _ in 0..150 {
            let card = o.held_card();
            let held = o.app.practice.current.clone().unwrap();
            let version = &o.app.library.versions[&held.prompt.version_id];
            let units = version
                .sections
                .iter()
                .flat_map(|s| &s.units)
                .map(|u| u.text.clone())
                .collect::<Vec<_>>();
            // Study's mix also starts the Starter shelf: answer it exactly.
            let answer = if held.prompt.version_id != vid {
                if version.kind == scry::library::Kind::KeyedMapping {
                    scry::library::mapping_pair(&units[held.targets[0].unit_index])
                        .unwrap()
                        .1
                        .to_string()
                } else {
                    held.targets
                        .iter()
                        .map(|t| units[t.unit_index].trim().to_string())
                        .collect::<Vec<_>>()
                        .join("\n")
                }
            } else {
                let unit = card["unit_index"].as_u64().unwrap() as usize;
                asked.push((unit, card["direction"].as_str().unwrap().to_string()));
                let (key, value) = scry::library::mapping_pair(&units[unit]).unwrap();
                if card["direction"] == "reverse" {
                    key
                } else {
                    value
                }
                .to_string()
            };
            o.answer(&answer);
            let pid = o.app.practice.current.as_ref().unwrap().prompt.id.clone();
            match entry {
                "mix" => o.call("/practice/review/next", &[("after", &pid)]),
                _ => o.call("/practice/lesson/next", &[("after", &pid)]),
            };
            let seen = asked.iter().collect::<std::collections::BTreeSet<_>>();
            if o.app.practice.current.is_none() || seen.len() == 6 {
                break;
            }
        }
        // Every pair is served in both directions, each Card on its own schedule.
        let seen = asked.iter().collect::<std::collections::BTreeSet<_>>();
        assert_eq!(seen.len(), 6, "{entry} {asked:?}");
        let history = web::render(&o.app, "/history", &BTreeMap::new(), o.now, None);
        assert!(history.contains("Unit 1, in reverse"), "{entry}");
        assert!(
            history.contains("<p class=\"eyebrow\">Value</p><p class=\"reading\">Polaris</p>"),
            "{entry}"
        );
    }
}

#[test]
fn every_new_mutation_replays_its_committed_result_and_refuses_another_payload() {
    let mut o = Owner::new();
    let op = |o: &mut Owner, id: &str, path: &str, pairs: &[(&str, &str)]| {
        let mut f = BTreeMap::from([
            ("csrf".to_string(), o.app.csrf.clone()),
            ("operation_id".to_string(), id.to_string()),
        ]);
        f.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.to_string())));
        let seed = format!("replay-seed-{id}-{}", o.app.revision);
        let r = engine::mutate_judged(
            &mut o.app,
            path,
            &f,
            o.now,
            &seed,
            MutationActor::Browser,
            false,
        );
        o.sound();
        r
    };
    let manifest = sky("both").to_string();
    let added = op(
        &mut o,
        "replay-mapping-add-0001",
        "/library/mapping/add",
        &[("data", &manifest)],
    )
    .unwrap();
    let before = o.app.clone();
    let again = op(
        &mut o,
        "replay-mapping-add-0001",
        "/library/mapping/add",
        &[("data", &manifest)],
    )
    .unwrap();
    assert_eq!(again.entity_id, added.entity_id);
    assert_eq!(o.app.library, before.library);
    let other = sky("forward").to_string();
    assert_eq!(
        op(
            &mut o,
            "replay-mapping-add-0001",
            "/library/mapping/add",
            &[("data", &other)]
        )
        .unwrap_err()
        .status,
        409
    );
    let vid = added.entity_id.unwrap();
    let sid = o.learn(&vid, "steady");
    o.next(&sid);
    o.answer("Polaris");
    o.next(&sid);
    o.answer("Mars");
    o.next(&sid);
    o.quiz();
    assert_eq!(o.held()["prompt"]["cue_level"], "mapping_value");
    let pid = o.app.practice.current.as_ref().unwrap().prompt.id.clone();
    let fields = [("prompt_id", pid.as_str()), ("answer", "North star")];
    let answered = op(
        &mut o,
        "replay-reverse-answer-0001",
        "/practice/answer",
        &fields,
    )
    .unwrap();
    let committed = o.app.clone();
    let replayed = op(
        &mut o,
        "replay-reverse-answer-0001",
        "/practice/answer",
        &fields,
    )
    .unwrap();
    assert_eq!(replayed.entity_id, answered.entity_id);
    assert_eq!(
        o.app.practice, committed.practice,
        "one Attempt, one schedule effect"
    );
    let changed = [("prompt_id", pid.as_str()), ("answer", "Red planet")];
    assert_eq!(
        op(
            &mut o,
            "replay-reverse-answer-0001",
            "/practice/answer",
            &changed
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(o.app.practice, committed.practice);
}

// --- Review: the year rule is recorded only where it turns a miss into Good ---

fn plain_grade(value: &str, answer: &str, strict: bool) -> Value {
    let units = [value];
    let version = scry::grading::Version {
        units: &units,
        spelling_strict: strict,
    };
    serde_json::to_value(scry::grading::grade_mapping(version, 0..1, answer).unwrap()).unwrap()
}

/// A Diff's expected spans as stored: relative to the complete Unit.
fn shifted(mut grade: Value, offset: usize) -> Value {
    for d in grade["diff"].as_array_mut().unwrap() {
        if let Some(span) = d.get_mut("expected").and_then(|e| e.get_mut("span")) {
            for end in ["start", "end"] {
                span[end] = json!(span[end].as_u64().unwrap() as usize + offset);
            }
        }
    }
    grade
}

#[test]
fn a_year_miss_and_a_plain_answer_are_stored_exactly_as_the_plain_grader_stores_them() {
    for (value, answers) in [
        ("1969 CE", &["1968", "1969 CE", "1970 CE", "19 69"][..]),
        ("AD 1953", &["0800", "AD 1953", "1953 BC"][..]),
        ("1066 AD", &["8", "1066 AD", "1066 ad", "AD"][..]),
    ] {
        let mut o = Owner::new();
        let key = "Synthetic event";
        let vid = o
            .add(json!({"title":"Synthetic era dates","direction":"forward","pairs":[{"key":key,"value":value}]}))
            .unwrap();
        let sid = o.learn(&vid, "intense");
        o.next(&sid);
        for (n, answer) in answers.iter().enumerate() {
            if n > 0 {
                o.again();
            }
            let r = o.answer(answer);
            let mut stored = r["attempt"]["grade"].clone();
            stored["wrong_version_hint"] = Value::Null;
            assert_eq!(
                stored,
                shifted(plain_grade(value, answer, true), key.len() + 1),
                "{value} {answer}"
            );
        }
        let rows = persistence::rows(&o.app).unwrap();
        assert!(!rows.contains_key(MARKER), "{value}");
        assert_ne!(
            persistence::Archive::new(o.app.clone(), vec![], o.now).format,
            FORMAT,
            "{value}"
        );
        // Validation agrees: an Attempt under the year version must be one the
        // rule made Good, and a miss forged under it is refused.
        let mut forged = o.app.clone();
        let miss = forged
            .practice
            .attempts
            .values_mut()
            .find(|a| a.verdict() == Some(scry::grading::Verdict::Again))
            .unwrap();
        miss.grade.authority.grader_version = YEAR.into();
        assert!(engine::validate_app(&forged).is_err(), "{value}");
    }
}

#[test]
fn a_year_turned_good_by_the_rule_cannot_be_replayed_as_a_plain_grade() {
    let mut o = Owner::new();
    let vid = o
        .add(json!({"title":"Synthetic era dates","direction":"forward","pairs":[{"key":"Synthetic event","value":"1969 CE"}]}))
        .unwrap();
    let sid = o.learn(&vid, "intense");
    o.next(&sid);
    let r = o.answer("1969");
    assert_eq!(verdict(&r), "Good");
    assert_eq!(r["attempt"]["grade"]["authority"]["grader_version"], YEAR);
    let mut forged = o.app.clone();
    let good = forged.practice.attempts.values_mut().next().unwrap();
    good.grade.authority.grader_version = "1;unicode=17.0.0;mapping=1".into();
    assert!(engine::validate_app(&forged).is_err());
}

#[test]
fn a_year_is_a_year_only_as_typed_and_the_rule_only_adds() {
    let cases: [(&str, &[(&str, &str)]); 3] = [
        (
            "1969 CE",
            &[
                ("-1969", "Again"),
                ("\u{2212}1969", "Again"),
                ("1969 CE.", "Good"),
                ("1969 c.e.", "Good"),
                ("1969 c-e", "Again"),
                ("in 1969", "Again"),
                ("1969 to 1970", "Again"),
                ("１９６９", "Good"),
            ],
        ),
        (
            "AD 1066",
            &[("-1066", "Again"), ("1066", "Good"), ("1066!", "Again")],
        ),
        // The plain grader already took a dash before a bare value; it still does.
        (
            "1066",
            &[
                ("-1066", "Good"),
                ("1066 AD", "Good"),
                ("1066 -AD", "Again"),
            ],
        ),
    ];
    for (n, (value, answers)) in cases.iter().enumerate() {
        let mut o = Owner::new();
        let vid = o
            .add(json!({"title":format!("Synthetic typed year {n}"),"direction":"forward","pairs":[{"key":"Synthetic event","value":value}]}))
            .unwrap();
        let sid = o.learn(&vid, "intense");
        o.next(&sid);
        for (i, (answer, expected)) in answers.iter().enumerate() {
            if i > 0 {
                o.again();
            }
            let r = o.answer(answer);
            assert_eq!(verdict(&r), *expected, "{value} {answer}");
            let plain = plain_grade(value, answer, true)["verdict"].clone();
            let year = r["attempt"]["grade"]["authority"]["grader_version"] == YEAR;
            assert_eq!(
                year,
                plain == "Again" && *expected == "Good",
                "{value} {answer}"
            );
        }
    }
}

#[test]
fn values_that_are_the_same_year_are_the_same_value_both_ways_and_may_share_a_pair_forward() {
    let mut o = Owner::new();
    let both = json!({"title":"Synthetic both years","direction":"both","pairs":[
        {"key":"Synthetic battle","value":"1066"},{"key":"Synthetic crossing","value":"AD 1066"}]});
    let e = o.add(both).unwrap_err();
    assert_eq!(e.status, 422);
    assert!(
        e.message.contains("needs a different value for every pair"),
        "{}",
        e.message
    );
    // Forward only, two pairs may share a value, year-equal or not, and a
    // right answer for the asked pair is Good, never the wrong key.
    let vid = o
        .add(
            json!({"title":"Synthetic shared values","direction":"forward","pairs":[
                {"key":"Synthetic battle","value":"1066"},
                {"key":"Synthetic crossing","value":"AD 1066"},
                {"key":"First shelf colour","value":"grey"},
                {"key":"Second shelf colour","value":"grey"}
            ]}),
        )
        .unwrap();
    let sid = o.learn(&vid, "intense");
    for answer in ["1066", "1066", "grey", "grey"] {
        o.next(&sid);
        let r = o.answer(answer);
        assert_eq!(verdict(&r), "Good", "{answer}");
        assert_eq!(r["mapping_feedback"]["kind"], "correct", "{answer}");
        assert!(r["mapping_feedback"].get("answered_key").is_none());
    }
}
