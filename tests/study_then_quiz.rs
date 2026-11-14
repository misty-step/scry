//! Practice has two forms for every Kind and every client: Source study, then
//! Quiz. Records made under the earlier cue ladder stay exactly as they were.
use scry::{
    api, engine,
    library::{self, Kind},
    model::*,
    owner_token::{MutationActor, OwnerToken},
    persistence,
    study::{self, Cue, Scope, StudyPhase},
    web,
};
use serde_json::{Value, json};
use std::collections::{BTreeMap, BTreeSet};

const NOW: i64 = 1_800_000_000_000;
const ROSARY: [&str; 5] = [
    "Annunciation",
    "Visitation",
    "Nativity",
    "Presentation",
    "Finding in the Temple",
];
const LANDMARKS: [&str; 5] = [
    "Red gate",
    "Blue bridge",
    "Green field",
    "White tower",
    "Black lake",
];

struct Owner {
    app: App,
    n: u64,
    now: i64,
}
impl Owner {
    fn new() -> Self {
        let mut app = App::new("study-then-quiz-synthetic-csrf-0123".into());
        let token = OwnerToken::issue(
            "Study then quiz agent",
            None,
            false,
            NOW,
            "abcdef0123456789abcdef0123456789abcdef0123456789",
        )
        .unwrap();
        app.owner_tokens.insert(token.info.id.clone(), token);
        Self {
            app,
            n: 0,
            now: NOW,
        }
    }
    /// A stored notebook, given a token so its API can be driven as an agent does.
    fn stored(mut app: App, now: i64) -> Self {
        let token = OwnerToken::issue(
            "Stored notebook agent",
            None,
            false,
            now,
            "0123456789abcdef0123456789abcdef0123456789abcdef",
        )
        .unwrap();
        app.owner_tokens.insert(token.info.id.clone(), token);
        Self { app, n: 0, now }
    }
    fn fields(&mut self, pairs: &[(&str, &str)]) -> BTreeMap<String, String> {
        self.n += 1;
        let mut f = BTreeMap::from([
            ("csrf".to_string(), self.app.csrf.clone()),
            (
                "operation_id".to_string(),
                format!("study-then-quiz-operation-{:06}", self.n),
            ),
        ]);
        f.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.to_string())));
        f
    }
    /// The app's native form route.
    fn app_route(&mut self, path: &str, pairs: &[(&str, &str)]) -> AppResult<MutationResult> {
        let f = self.fields(pairs);
        let seed = format!("study-then-quiz-seed-{:06}", self.n);
        let result = engine::mutate(&mut self.app, path, &f, self.now, &seed);
        engine::validate_app(&self.app).unwrap();
        result
    }
    fn act(&mut self, path: &str, pairs: &[(&str, &str)]) -> MutationResult {
        self.app_route(path, pairs).unwrap()
    }
    /// The versioned API as an Owner token drives it, CLI and tool server included.
    fn api(&mut self, path: &str, mut data: Value) -> AppResult<Value> {
        self.n += 1;
        data["operation_id"] = json!(format!("study-then-quiz-operation-{:06}", self.n));
        let fields = api::library_fields(data)?;
        let token = self.app.owner_tokens.values().next().unwrap().info.clone();
        let seed = format!("study-then-quiz-seed-{:06}", self.n);
        let result = engine::mutate_as(
            &mut self.app,
            api::mutation_path(path).unwrap(),
            &fields,
            self.now,
            &seed,
            MutationActor::Token(&token),
        );
        engine::validate_app(&self.app).unwrap();
        let result = result?;
        Ok(study::wire_response(&self.app, &result).unwrap_or(json!({"id":result.entity_id})))
    }
    fn held(&self) -> study::Presentation {
        self.app.practice.current.clone().unwrap()
    }
    fn page(&self) -> String {
        web::render(&self.app, "/", &BTreeMap::new(), self.now, None)
    }
    fn units(&self) -> Vec<String> {
        let p = self.held();
        self.app.library.versions[&p.prompt.version_id]
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .filter(|u| p.prompt.unit_ids.contains(&u.id))
            .map(|u| u.text.clone())
            .collect()
    }
    /// What a correct answer to the held Prompt is, taken from the Library.
    fn expected(&self) -> String {
        let p = self.held();
        let v = &self.app.library.versions[&p.prompt.version_id];
        let units = self.units();
        if v.kind == Kind::KeyedMapping {
            library::mapping_pair(&units[0]).unwrap().1.to_string()
        } else {
            units.concat()
        }
    }
    /// Source study: nothing to answer, from the app or the API, and nothing recorded.
    fn assert_source_study(&mut self, shown: &[&str]) {
        let p = self.held();
        assert_eq!(p.phase, Some(StudyPhase::Study));
        assert_eq!(p.prompt.cue_level, Cue::Read);
        assert!(p.source_visible() && !p.revealed && p.attempt_id.is_none());
        assert!(!self.app.practice.protected());
        assert_eq!(p.prompt.cue, self.units().concat());
        let page = self.page();
        assert!(page.contains("data-study-phase=\"study\""));
        assert!(page.contains(">Try from memory</button>"));
        for control in [
            "id=\"answer\"",
            "name=\"answer\"",
            "action=\"/practice/answer\"",
            "action=\"/practice/reveal\"",
            "action=\"/practice/draft\"",
        ] {
            assert!(!page.contains(control), "{control}");
        }
        let mut from = 0;
        for text in shown {
            let at = page[from..]
                .find(&escape(text))
                .unwrap_or_else(|| panic!("{text} is shown, in order"));
            from += at + text.len();
        }
        let answer = self.expected();
        let before = self.app.clone();
        for path in ["/practice/answer", "/practice/draft"] {
            let refused = self
                .app_route(
                    path,
                    &[
                        ("prompt_id", &p.prompt.id),
                        ("answer", &answer),
                        ("generation", "0"),
                    ],
                )
                .unwrap_err();
            assert_eq!(refused.status, 409, "{path}");
            assert_eq!(self.app, before, "{path}");
        }
        let refused = self
            .app_route(
                "/practice/answer",
                &[("prompt_id", &p.prompt.id), ("answer", &answer)],
            )
            .unwrap_err();
        assert_eq!(refused.status, 409);
        assert!(refused.message.contains("source study"));
        let refused = self
            .api(
                "/api/v1/answer",
                json!({"prompt_id":p.prompt.id,"answer":answer}),
            )
            .unwrap_err();
        assert_eq!(refused.status, 409);
        assert_eq!(self.app, before, "A refused answer leaves no trace");
    }
    /// Try from memory: the same scope, the source hidden, and nothing recorded.
    fn try_from_memory(&mut self) -> study::Presentation {
        let source = self.held();
        let before = self.app.practice.clone();
        let due = study::due(&self.app, self.now);
        let f = self.fields(&[("prompt_id", &source.prompt.id)]);
        let result = engine::mutate(&mut self.app, "/practice/quiz", &f, self.now, "quiz").unwrap();
        let saved = self.app.clone();
        assert_eq!(
            engine::mutate(&mut self.app, "/practice/quiz", &f, self.now + 1, "again").unwrap(),
            result
        );
        assert_eq!(self.app, saved, "Repeating the move changes nothing");
        engine::validate_app(&self.app).unwrap();
        let quiz = self.held();
        assert_eq!(quiz.phase, Some(StudyPhase::Quiz));
        assert_ne!(quiz.prompt.id, source.prompt.id);
        assert_eq!(quiz.prompt.scope, source.prompt.scope);
        assert_eq!(quiz.prompt.unit_ids, source.prompt.unit_ids);
        assert!(!quiz.prompt.assisted && quiz.prompt.context.is_none());
        assert_eq!(self.app.practice.cards, before.cards);
        assert_eq!(self.app.practice.attempts, before.attempts);
        assert_eq!(self.app.practice.studies, before.studies);
        assert_eq!(study::due(&self.app, self.now), due);
        assert!(self.app.practice.protected());
        quiz
    }
    /// A Quiz carries nothing answer-bearing, in the page or the API response.
    fn assert_quiz_hides(&self, hidden: &[&str]) {
        let p = self.held();
        assert_eq!(p.phase, Some(StudyPhase::Quiz));
        assert!(!p.prompt.assisted && !p.revealed);
        assert!(matches!(
            p.prompt.cue_level,
            Cue::MappingKey | Cue::WholeTitle | Cue::SectionHeading
        ));
        let page = self.page();
        assert!(page.contains("id=\"answer\"") && page.contains("action=\"/practice/answer\""));
        assert!(!page.contains("exact-expected") && !page.contains("Try from memory"));
        let wire = serde_json::to_string(&p.prompt).unwrap();
        let receipts = serde_json::to_string(
            &self
                .app
                .operations
                .values()
                .filter(|r| r.path.as_deref() == Some("/practice/quiz"))
                .collect::<Vec<_>>(),
        )
        .unwrap();
        for text in hidden {
            assert!(!page.contains(&escape(text)), "page shows {text}");
            assert!(!wire.contains(text), "Prompt carries {text}");
            assert!(!receipts.contains(text), "quiz receipt carries {text}");
        }
        assert_eq!(study::history(&self.app).unwrap_err().status, 409);
    }
    /// The Quiz after Source study is Free recall and schedules its Cards.
    fn answer_unaided(&mut self) -> study::Attempt {
        let p = self.held();
        let answer = self.expected();
        let before = self.app.practice.cards.clone();
        self.act(
            "/practice/answer",
            &[("prompt_id", &p.prompt.id), ("answer", &answer)],
        );
        let attempt = self.app.practice.attempts[self.held().attempt_id.as_ref().unwrap()].clone();
        assert_eq!(attempt.grade.verdict, scry::grading::Verdict::Good);
        assert!(!attempt.assisted && !attempt.revealed && !attempt.disclosure);
        assert_eq!(attempt.effects.len(), p.targets.len());
        for e in &attempt.effects {
            assert!(e.scheduled && !e.assisted && !e.extra_practice);
            let card = &self.app.practice.cards[&e.card_id];
            assert_eq!(card.state.schedule.last_review_ms, Some(self.now));
            assert_ne!(card.state.schedule, before[&e.card_id].state.schedule);
        }
        attempt
    }
}
fn escape(text: &str) -> String {
    text.replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
        .replace('\'', "&#39;")
}

#[test]
fn a_new_nato_letter_is_studied_then_quizzed_unassisted() {
    let mut o = Owner::new();
    o.act("/practice/nato/start", &[]);
    assert_eq!(o.app.practice.cards.len(), 1);
    assert_eq!(o.held().prompt.cue, "A\tAlfa\n");
    let page = o.page();
    assert!(page.contains("aria-label=\"Letter A\"") && !page.contains("Bravo"));
    o.assert_source_study(&["Alfa"]);
    o.try_from_memory();
    assert_eq!(o.held().prompt.cue, "A");
    assert!(o.page().contains("aria-label=\"Letter A\""));
    o.assert_quiz_hides(&["Alfa", "Bravo"]);
    let attempt = o.answer_unaided();
    assert_eq!(attempt.prompt.cue_level, Cue::MappingKey);
    // The next new letter is studied in its turn.
    let (after, study_id) = (o.held().prompt.id, o.held().prompt.study_id);
    o.act(
        "/practice/next",
        &[("after", &after), ("study_id", &study_id)],
    );
    assert_eq!(o.held().prompt.cue, "B\tBravo\n");
    o.assert_source_study(&["Bravo"]);
}

#[test]
fn a_new_baby_care_pair_is_studied_with_its_question_then_quizzed_unassisted() {
    let mut o = Owner::new();
    let started = o.act("/practice/lesson/start", &[("starter", "baby-soothing")]);
    assert_eq!(started.redirect, "/?practice=baby-soothing");
    let unit = o.units().remove(0);
    let (question, answer) = library::mapping_pair(&unit).unwrap();
    assert!(question.contains("Stop at any signs of trying to roll."));
    assert_eq!(answer, "swaddle");
    // The authored question, safety cue included, stays above its answer.
    o.assert_source_study(&[question, answer]);
    o.try_from_memory();
    assert_eq!(o.held().prompt.cue, question);
    assert!(o.page().contains(&escape(question)));
    o.assert_quiz_hides(&["swaddle"]);
    o.answer_unaided();
    assert_eq!(
        o.app.practice.cards.len(),
        1,
        "A mapping lesson keeps its Pace"
    );
}

#[test]
fn a_new_rosary_set_is_studied_whole_then_quizzed_unassisted() {
    let mut o = Owner::new();
    o.act("/practice/lesson/start", &[("starter", "rosary-joyful")]);
    let p = o.held();
    assert_eq!(p.prompt.scope, Scope::Whole);
    assert_eq!(p.targets.len(), 5);
    assert_eq!(o.app.practice.cards.len(), 5);
    let mut shown = vec!["Joyful Mysteries"];
    shown.extend(ROSARY);
    o.assert_source_study(&shown);
    let quiz = o.try_from_memory();
    assert_eq!(quiz.prompt.cue_level, Cue::WholeTitle);
    assert!(o.page().contains("Joyful Mysteries"));
    o.assert_quiz_hides(&ROSARY[1..]);
    let attempt = o.answer_unaided();
    assert_eq!(attempt.effects.len(), 5);
    // Practised now, the set returns as a Quiz, never as its source.
    let after = o.held().prompt.id;
    o.act(
        "/practice/lesson/next",
        &[("after", &after), ("mode", "whole")],
    );
    assert_eq!(o.held().prompt.scope, Scope::Whole);
    o.assert_quiz_hides(&ROSARY[1..]);
}

#[test]
fn study_mix_presents_every_subject_as_source_before_its_quiz() {
    let mut o = Owner::new();
    o.act("/practice/review/start", &[]);
    let mut kinds = BTreeMap::new();
    for _ in 0..9 {
        let p = o.held();
        let version = &o.app.library.versions[&p.prompt.version_id];
        *kinds.entry(format!("{:?}", version.kind)).or_insert(0) += 1;
        assert_eq!(p.phase, Some(StudyPhase::Study), "{}", p.prompt.title);
        if version.kind == Kind::OrderedList {
            assert_eq!(p.prompt.scope, Scope::Whole);
        } else {
            assert!(matches!(p.prompt.scope, Scope::Unit { .. }));
        }
        let before = o.app.clone();
        let refused = o
            .app_route(
                "/practice/answer",
                &[("prompt_id", &p.prompt.id), ("answer", "anything")],
            )
            .unwrap_err();
        assert_eq!(refused.status, 409);
        assert_eq!(o.app, before);
        o.try_from_memory();
        o.answer_unaided();
        let after = o.held().prompt.id;
        o.act("/practice/review/next", &[("after", &after)]);
    }
    assert_eq!(
        kinds,
        BTreeMap::from([
            ("KeyedMapping".into(), 4),
            ("OrderedList".into(), 4),
            ("Text".into(), 1)
        ])
    );
    assert_eq!(o.app.practice.studies.len(), 9);
}

fn add(o: &mut Owner, data: Value) -> String {
    let path = if data.get("pairs").is_some() {
        "/library/mapping/add"
    } else {
        "/library/version/add"
    };
    o.act(path, &[("data", &data.to_string())])
        .entity_id
        .unwrap()
}
fn landmarks() -> Value {
    json!({"title":"Five synthetic landmarks","kind":"ordered_list","name":"Synthetic v1",
        "text":LANDMARKS.map(|l| format!("{l}\n")).concat(),
        "sections":[{"heading":"Route","units":LANDMARKS.map(|l| format!("{l}\n"))}]})
}
fn lamps() -> Value {
    json!({"title":"Synthetic harbour lamps","direction":"forward","pairs":[
        {"key":"Red lamp","value":"Stop"},{"key":"Green lamp","value":"Enter"},
        {"key":"White lamp","value":"Anchor"}]})
}
const CANTICLE: [&str; 8] = [
    "Still waters hold the morning light,\n",
    "and quiet hills receive the rain;\n",
    "the patient reed bends low at night,\n",
    "and rises with the dawn again.\n",
    "No hurried wind disturbs the shore,\n",
    "no shadow dims the silver stream;\n",
    "the heron waits, and nothing more,\n",
    "beside the slow unbroken gleam.\n",
];
fn canticle() -> Value {
    json!({"title":"Synthetic Canticle","name":"Synthetic v1","kind":"text","text":CANTICLE.concat(),
        "sections":[{"heading":"Opening","units":CANTICLE[..4]},{"heading":"Closing","units":CANTICLE[4..]}]})
}

#[test]
fn studies_started_from_the_library_study_first_whatever_their_kind() {
    for (data, whole, shown) in [
        (lamps(), false, vec!["Red lamp", "Stop"]),
        (landmarks(), true, LANDMARKS.to_vec()),
        (canticle(), false, vec![CANTICLE[0].trim()]),
    ] {
        let mut o = Owner::new();
        let vid = add(&mut o, data);
        let learn = web::render(
            &o.app,
            &format!("/library/versions/{vid}"),
            &BTreeMap::new(),
            NOW,
            None,
        );
        assert!(learn.contains("action=\"/practice/study/start\""));
        o.act("/practice/study/start", &[("version_id", &vid)]);
        let p = o.held();
        assert_eq!(p.prompt.scope == Scope::Whole, whole);
        assert_eq!(p.targets.len(), if whole { 5 } else { 1 });
        o.assert_source_study(&shown);
        o.try_from_memory();
        o.assert_quiz_hides(&shown[shown.len() - 1..]);
        o.answer_unaided();
    }
}

#[test]
fn a_learn_started_study_never_receives_a_cue_and_quizzes_what_was_practised() {
    let mut o = Owner::new();
    let vid = o
        .api("/api/v1/library/versions", json!({"data":canticle()}))
        .unwrap()["id"]
        .as_str()
        .unwrap()
        .to_string();
    let sid = o.api("/api/v1/learn", json!({"version_id":vid})).unwrap()["study"]["id"]
        .as_str()
        .unwrap()
        .to_string();
    let mut practised = BTreeSet::new();
    let mut quizzes_of_practised = 0;
    for step in 0..60 {
        let after = o
            .app
            .practice
            .current
            .as_ref()
            .filter(|p| p.attempt_id.is_some())
            .map(|p| p.prompt.id.clone());
        let mut request = json!({"study_id":sid});
        if let Some(after) = after {
            request["after"] = json!(after);
        }
        let next = o.api("/api/v1/next", request).unwrap();
        if next["prompt"].is_null() {
            // Nothing eligible until reviews fall due and the next day's Pace opens.
            o.now += DAY_MS;
            continue;
        }
        let unit = next["prompt"]["unit_ids"][0].as_str().unwrap().to_string();
        let text = o.units().concat();
        let quiz = if practised.contains(&unit) {
            quizzes_of_practised += 1;
            assert_eq!(next["phase"], "quiz", "A practised phrase is a Quiz");
            next
        } else {
            // A new phrase is a presentation the client cannot answer.
            assert_eq!(next["phase"], "study");
            assert_eq!(next["prompt"]["cue_level"], "read");
            assert_eq!(next["prompt"]["cue"], text);
            let refused = o
                .api(
                    "/api/v1/answer",
                    json!({"prompt_id":next["prompt"]["id"],"answer":text}),
                )
                .unwrap_err();
            assert_eq!(refused.status, 409);
            assert!(o.app.practice.attempts.len() == practised.len() || step > 0);
            o.api("/api/v1/quiz", json!({"prompt_id":next["prompt"]["id"]}))
                .unwrap()
        };
        assert_eq!(quiz["phase"], "quiz");
        assert_eq!(quiz["prompt"]["cue_level"], "whole_title");
        assert_eq!(quiz["prompt"]["cue"], "Synthetic Canticle");
        assert_eq!(quiz["prompt"]["assisted"], false);
        assert!(quiz["prompt"]["context"].is_null());
        let distinctive = text.split_whitespace().max_by_key(|w| w.len()).unwrap();
        assert!(!quiz.to_string().contains(distinctive), "{distinctive}");
        // Misses and successes alike: the next Prompt for this phrase is a Quiz.
        let answer = if step % 4 == 2 {
            "not this"
        } else {
            text.trim()
        };
        let attempt = o
            .api(
                "/api/v1/answer",
                json!({"prompt_id":quiz["prompt"]["id"],"answer":answer}),
            )
            .unwrap()["attempt"]
            .clone();
        assert_eq!(attempt["assisted"], false);
        assert_eq!(attempt["prompt"]["cue_level"], "whole_title");
        practised.insert(unit);
    }
    assert_eq!(
        practised.len(),
        8,
        "Every phrase was introduced at its Pace"
    );
    assert!(quizzes_of_practised >= 8);
    assert!(
        o.app
            .practice
            .attempts
            .values()
            .all(|a| a.prompt.cue_level == Cue::WholeTitle && !a.prompt.assisted)
    );
    assert!(o.app.practice.studies[&sid].source_quiz && !o.app.practice.studies[&sid].lesson);
    // The retained Card level is history only: nothing above ever read it.
    assert!(
        o.app
            .practice
            .cards
            .values()
            .all(|c| c.state.cue == Cue::Read)
    );
    // A chosen Recitation is prompted from its heading or title alone.
    let after = o.held().prompt.id;
    let whole = o
        .api(
            "/api/v1/next",
            json!({"study_id":sid,"after":after,"recite":"whole"}),
        )
        .unwrap();
    assert_eq!(whole["phase"], "quiz");
    assert_eq!(whole["prompt"]["cue_level"], "whole_title");
    assert!(!whole.to_string().contains("heron"));
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
fn an_ordered_list_started_with_learn_is_presented_whole() {
    let mut o = Owner::new();
    let vid = o
        .api("/api/v1/library/versions", json!({"data":landmarks()}))
        .unwrap()["id"]
        .as_str()
        .unwrap()
        .to_string();
    let learned = o
        .api("/api/v1/learn", json!({"version_id":vid,"pace":"light"}))
        .unwrap();
    let sid = learned["study"]["id"].as_str().unwrap().to_string();
    assert!(
        o.app.practice.cards.is_empty(),
        "Learning introduces nothing"
    );
    let source = o.api("/api/v1/next", json!({"study_id":sid})).unwrap();
    assert_eq!(source["phase"], "study");
    assert_eq!(source["prompt"]["scope"], json!({"kind":"whole"}));
    assert_eq!(source["prompt"]["unit_ids"].as_array().unwrap().len(), 5);
    assert_eq!(
        source["prompt"]["cue"],
        LANDMARKS.map(|l| format!("{l}\n")).concat()
    );
    assert_eq!(
        o.app.practice.cards.len(),
        5,
        "Introduced whole, beyond a light Pace"
    );
    assert_eq!(o.app.practice.studies[&sid].pace, study::Pace::Light);
    o.assert_source_study(&LANDMARKS);
    let quiz = o
        .api("/api/v1/quiz", json!({"prompt_id":source["prompt"]["id"]}))
        .unwrap();
    assert_eq!(quiz["prompt"]["scope"], json!({"kind":"whole"}));
    assert_eq!(quiz["prompt"]["cue_level"], "whole_title");
    assert!(!quiz.to_string().contains("Blue bridge"));
    let attempt = o
        .api(
            "/api/v1/answer",
            json!({"prompt_id":quiz["prompt"]["id"],"answer":LANDMARKS.join("\n")}),
        )
        .unwrap()["attempt"]
        .clone();
    assert_eq!(attempt["grade"]["verdict"], "Good");
    assert_eq!(attempt["assisted"], false);
    assert_eq!(attempt["effects"].as_array().unwrap().len(), 5);
    // Asking for one item's Card still recalls the whole sequence.
    let card = o.app.practice.cards.keys().nth(3).unwrap().clone();
    let again = o
        .api(
            "/api/v1/next",
            json!({"after":quiz["prompt"]["id"],"card_id":card}),
        )
        .unwrap();
    assert_eq!(again["phase"], "quiz");
    assert_eq!(again["prompt"]["scope"], json!({"kind":"whole"}));
    assert_eq!(again["prompt"]["unit_ids"].as_array().unwrap().len(), 5);
    assert!(
        o.app
            .practice
            .attempts
            .values()
            .all(|a| a.prompt.scope == Scope::Whole)
    );
}

// Records captured on the source before this change. See
// fixtures/cue-ladder-legacy.README.md.
fn fixture() -> Value {
    let fixture: Value =
        serde_json::from_str(include_str!("fixtures/cue-ladder-legacy.json")).unwrap();
    assert_eq!(
        fixture["source_commit"],
        "fb9c5d281f84dbd6c1d3541c23d2d16f0963af97"
    );
    fixture
}
const STATES: [(&str, &str); 7] = [
    ("ladder", "held_first_letters"),
    ("ladder", "held_list_item"),
    ("ladder", "held_mapping_key"),
    ("quiz_first", "baby_held_quiz"),
    ("quiz_first", "rosary_held_quiz"),
    ("quiz_first", "rosary_answered"),
    ("quiz_first", "nato_held_quiz"),
];
fn archive(group: &str, state: &str) -> (Value, persistence::Archive) {
    let saved = fixture()[group]["states"][state].clone();
    let archive = persistence::Archive::decode(
        saved["archive"].as_str().unwrap().as_bytes(),
        saved["sha256"].as_str().unwrap(),
    )
    .unwrap();
    (saved, archive)
}
fn stored(group: &str, state: &str) -> (Value, Owner) {
    let (saved, archive) = archive(group, state);
    let now = saved["now_ms"].as_i64().unwrap();
    (saved, Owner::stored(archive.app, now))
}
fn without_tokens(app: &App) -> App {
    let mut app = app.clone();
    app.owner_tokens.clear();
    app
}
fn request(value: &Value) -> (String, BTreeMap<String, String>, i64, String) {
    (
        value["path"].as_str().unwrap().into(),
        serde_json::from_value(value["fields"].clone()).unwrap(),
        value["now_ms"].as_i64().unwrap(),
        value["seed"].as_str().unwrap().into(),
    )
}
/// Submit the answer recorded with the fixture and prove the notebook is byte
/// for byte what the earlier source produced from the same request.
fn answer_as_before(saved: &Value, o: &mut Owner) -> study::Attempt {
    let (path, fields, now, seed) = request(&saved["answer"]["request"]);
    assert_eq!(path, "/practice/answer");
    engine::mutate(&mut o.app, &path, &fields, now, &seed).unwrap();
    engine::validate_app(&o.app).unwrap();
    o.now = now;
    // Only the test's own token differs from the stored notebook.
    let bytes = persistence::Archive::new(without_tokens(&o.app), vec![], now)
        .encode()
        .unwrap();
    assert_eq!(
        persistence::sha256(&bytes),
        saved["answer"]["sha256"].as_str().unwrap(),
        "The held Prompt is recorded exactly as before"
    );
    let again = o
        .app_route(
            "/practice/answer",
            &[("prompt_id", &fields["prompt_id"]), ("answer", "again")],
        )
        .unwrap_err();
    assert_eq!(again.status, 409, "A held Prompt is answered once");
    o.app.practice.attempts[o.held().attempt_id.as_ref().unwrap()].clone()
}

#[test]
fn archives_made_before_the_change_restore_validate_and_export_unchanged() {
    for (group, state) in STATES {
        let (saved, archive) = archive(group, state);
        let bytes = saved["archive"].as_str().unwrap().as_bytes();
        assert_eq!(archive.format, saved["format"].as_str().unwrap(), "{state}");
        engine::validate_app(&archive.app).unwrap();
        assert_eq!(
            archive.encode().unwrap(),
            bytes,
            "{state} exports unchanged"
        );
        let exported = persistence::Archive::new(archive.app.clone(), vec![], archive.created_ms);
        assert_eq!(exported.format, archive.format, "{state}");
        assert_eq!(exported.encode().unwrap(), bytes, "{state}");
        // The stored notebook rows load to the same notebook and write back unchanged.
        let rows = persistence::rows(&archive.app).unwrap();
        assert_eq!(persistence::app_from_rows(&rows).unwrap(), archive.app);
        if let Some(saved_rows) = saved["rows"].as_object() {
            let saved_rows: BTreeMap<String, String> =
                serde_json::from_value(json!(saved_rows)).unwrap();
            assert_eq!(rows, saved_rows, "{state} rows are byte-stable");
            assert_eq!(
                persistence::app_from_rows(&saved_rows).unwrap(),
                archive.app
            );
        }
        // Every recorded operation replays its committed receipt or is fenced by
        // the protected Prompt. Neither changes anything.
        let requests = fixture()[group]["requests"].clone();
        let recorded = saved["requests"].as_u64().unwrap() as usize - (group == "ladder") as usize;
        let mut app = archive.app.clone();
        let mut replayed = 0;
        for value in &requests.as_array().unwrap()[..recorded] {
            let (path, fields, now, seed) = request(value);
            match engine::mutate(&mut app, &path, &fields, now + 1, &seed) {
                Ok(result) => {
                    assert_eq!(result, app.operations[&fields["operation_id"]].result);
                    replayed += 1;
                }
                Err(error) => assert_eq!(error.status, 409, "{path}: {}", error.message),
            }
            assert_eq!(app, archive.app, "{state} {path}");
        }
        assert!(replayed >= 2, "{state}");
    }
    let (saved, _) = archive("ladder", "held_first_letters");
    assert!(saved["rows"].is_object());
}

#[test]
fn a_stored_ladder_notebook_validates_answers_its_held_cue_once_then_quizzes() {
    let (saved, mut o) = stored("ladder", "held_first_letters");
    let rows: BTreeMap<String, String> = serde_json::from_value(saved["rows"].clone()).unwrap();
    assert_eq!(
        persistence::app_from_rows(&rows).unwrap(),
        without_tokens(&o.app)
    );
    engine::validate_app(&o.app).unwrap();
    let levels = |app: &App| {
        app.practice
            .cards
            .values()
            .map(|c| format!("{:?}", c.state.cue))
            .collect::<BTreeSet<_>>()
    };
    assert_eq!(
        levels(&o.app),
        [
            "FirstLetters",
            "MappingKey",
            "PreviousLine",
            "Read",
            "SectionHeading"
        ]
        .map(String::from)
        .into()
    );
    assert!(
        o.app
            .practice
            .studies
            .values()
            .all(|s| !s.source_quiz && !s.lesson)
    );
    let held = o.held();
    assert_eq!(held.phase, None);
    assert_eq!(held.prompt.cue_level, Cue::FirstLetters);
    assert_eq!(held.prompt.cue, "t p r b l a n");
    // The app shows the visible source as study, never as a form to copy from.
    let page = o.page();
    assert!(page.contains(">Try from memory</button>"));
    assert!(!page.contains("id=\"answer\"") && !page.contains("using the initials"));
    let cards = o.app.practice.cards.clone();
    let earlier = o.app.practice.attempts.clone();
    let attempt = answer_as_before(&saved, &mut o);
    assert!(attempt.assisted && !attempt.revealed);
    assert_eq!(attempt.prompt, held.prompt);
    assert_eq!(attempt.effects[0].cue_level, Cue::FirstLetters);
    assert!(!attempt.effects[0].scheduled);
    assert_eq!(attempt.effects[0].after.successes, 1);
    for (id, a) in &earlier {
        assert_eq!(
            &o.app.practice.attempts[id], a,
            "Earlier Attempts are untouched"
        );
    }
    // Every Prompt after it is a Quiz, whatever level its Card retains.
    let card = held.targets[0].card_id.clone();
    let (after, sid) = (held.prompt.id.clone(), held.prompt.study_id.clone());
    for level in ["FirstLetters", "SectionHeading", "PreviousLine"] {
        let id = cards
            .values()
            .find(|c| c.study_id == sid && format!("{:?}", c.state.cue) == level)
            .unwrap()
            .id
            .clone();
        let mut pairs = vec![("card_id", id.as_str())];
        let released = o.held().prompt.id;
        if o.held().attempt_id.is_some() {
            pairs.push(("after", &released));
        }
        o.act("/practice/next", &pairs);
        let quiz = o.held();
        assert_eq!(quiz.targets[0].card_id, id);
        assert_eq!(quiz.prompt.cue_level, Cue::WholeTitle);
        let text = o.expected();
        o.assert_quiz_hides(&[text.trim()]);
        let unaided = {
            let p = o.held();
            let answer = o.expected();
            o.act(
                "/practice/answer",
                &[("prompt_id", &p.prompt.id), ("answer", &answer)],
            );
            o.app.practice.attempts[o.held().attempt_id.as_ref().unwrap()].clone()
        };
        assert!(!unaided.assisted, "{level}");
        assert_eq!(unaided.prompt.cue_level, Cue::WholeTitle);
    }
    assert_eq!(after, held.prompt.id);
    assert!(
        o.app.practice.cards[&card]
            .state
            .schedule
            .last_review_ms
            .is_some()
    );
    assert!(o.app.practice.studies[&sid].source_quiz);
    // The phrase never introduced under the ladder is studied first.
    let released = o.held().prompt.id;
    o.now += 2 * DAY_MS;
    let mut introduced = false;
    for _ in 0..6 {
        let released_now = o.held().prompt.id;
        let after = if o.held().attempt_id.is_some() {
            released_now.as_str()
        } else {
            released.as_str()
        };
        o.act("/practice/next", &[("after", after), ("study_id", &sid)]);
        if o.held().phase == Some(StudyPhase::Study) {
            assert_eq!(o.held().prompt.cue, CANTICLE[3]);
            introduced = true;
            break;
        }
        assert_eq!(o.held().phase, Some(StudyPhase::Quiz));
        let p = o.held();
        let answer = o.expected();
        o.act(
            "/practice/answer",
            &[("prompt_id", &p.prompt.id), ("answer", &answer)],
        );
    }
    assert!(introduced);
    let archive = persistence::Archive::new(o.app.clone(), vec![], o.now);
    assert_eq!(archive.format, "scry-rust-durable-sqlite-study-phase-v3");
    let bytes = archive.encode().unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        o.app
    );
}

#[test]
fn a_stored_item_drilled_list_answers_its_held_item_once_then_is_recalled_whole() {
    let (saved, mut o) = stored("ladder", "held_list_item");
    let held = o.held();
    assert_eq!(held.phase, None);
    assert_eq!(held.prompt.cue_level, Cue::Read);
    assert!(matches!(held.prompt.scope, Scope::Unit { .. }));
    let sid = held.prompt.study_id.clone();
    assert!(!o.app.practice.studies[&sid].lesson);
    let attempt = answer_as_before(&saved, &mut o);
    assert!(attempt.assisted);
    assert_eq!(attempt.effects.len(), 1);
    let cards = o.app.practice.cards.len();
    // A paused Study receives no new material, so its list waits whole.
    let paused = {
        let mut paused = Owner::stored(o.app.clone(), o.now);
        paused.act("/practice/pause", &[("study_id", &sid)]);
        let result = paused.act(
            "/practice/next",
            &[("after", &held.prompt.id), ("study_id", &sid)],
        );
        assert!(result.data.unwrap()["prompt"].is_null());
        assert_eq!(paused.app.practice.cards.len(), cards);
        let card = held.targets[0].card_id.clone();
        assert_eq!(
            paused
                .app_route("/practice/next", &[("card_id", &card)])
                .unwrap_err()
                .status,
            409
        );
        paused.act("/practice/resume", &[("study_id", &sid)]);
        paused
    };
    for mut o in [o, paused] {
        let mut pairs = vec![("study_id", sid.as_str())];
        if o.app.practice.current.is_some() {
            pairs.push(("after", &held.prompt.id));
        }
        o.act("/practice/next", &pairs);
        let whole = o.held();
        assert_eq!(whole.prompt.scope, Scope::Whole);
        assert_eq!(whole.targets.len(), 5);
        assert_eq!(o.app.practice.cards.len(), cards + 4);
        assert!(o.app.practice.studies[&sid].lesson);
        // Four items were never shown, so the whole list is studied first.
        let mut shown = vec!["Five synthetic landmarks"];
        shown.extend(LANDMARKS);
        o.assert_source_study(&shown);
        o.try_from_memory();
        o.assert_quiz_hides(&LANDMARKS[1..]);
        let attempt = o.answer_unaided();
        assert_eq!(attempt.effects.len(), 5);
        assert_eq!(attempt.prompt.scope, Scope::Whole);
    }
}

#[test]
fn stored_quiz_first_prompts_stay_answerable_and_new_material_is_studied() {
    // A mapping pair held as a Quiz by the earlier API is answered as before.
    let (saved, mut o) = stored("ladder", "held_mapping_key");
    let held = o.held();
    assert_eq!(held.phase, None);
    assert_eq!(held.prompt.cue, "White lamp");
    let attempt = answer_as_before(&saved, &mut o);
    assert!(!attempt.assisted && attempt.effects[0].scheduled);
    // Its practised pairs return as Quizzes once due.
    o.now += DAY_MS;
    o.act(
        "/practice/next",
        &[
            ("after", &held.prompt.id),
            ("study_id", &held.prompt.study_id),
        ],
    );
    o.assert_quiz_hides(&["Stop", "Enter", "Anchor"]);

    for (state, hidden) in [
        ("baby_held_quiz", "swaddle"),
        ("rosary_held_quiz", "Visitation"),
        ("nato_held_quiz", "Bravo"),
    ] {
        let (saved, mut o) = stored("quiz_first", state);
        let held = o.held();
        assert_eq!(held.phase, Some(StudyPhase::Quiz), "{state}");
        o.assert_quiz_hides(&[hidden]);
        let attempt = answer_as_before(&saved, &mut o);
        assert!(!attempt.assisted, "{state}");
        if state == "rosary_held_quiz" {
            let (_, answered) = archive("quiz_first", "rosary_answered");
            assert_eq!(without_tokens(&o.app), answered.app);
            continue;
        }
        // The next pair has never been shown: it is studied before its Quiz.
        o.act(
            "/practice/next",
            &[
                ("after", &held.prompt.id),
                ("study_id", &held.prompt.study_id),
            ],
        );
        let unit = o.units().remove(0);
        let (question, answer) = library::mapping_pair(&unit).unwrap();
        o.assert_source_study(&[question, answer]);
    }
}

#[test]
fn recovery_refuses_an_answer_recorded_against_a_source_only_presentation() {
    for starter in ["nato", "rosary-joyful"] {
        let mut o = Owner::new();
        o.act("/practice/lesson/start", &[("starter", starter)]);
        let source = o.held();
        o.try_from_memory();
        o.answer_unaided();
        // A source held as anything but study is not a record Scry can have written.
        let mut held = o.app.clone();
        held.practice.current = Some(source.clone());
        held.practice.current.as_mut().unwrap().phase = None;
        assert_eq!(engine::validate_app(&held).unwrap_err().status, 422);
        // Nor is an answer graded beside it.
        let mut forged = o.app.clone();
        let id = o.held().attempt_id.unwrap();
        let attempt = forged.practice.attempts.get_mut(&id).unwrap();
        attempt.prompt = source.prompt.clone();
        attempt.assisted = true;
        for e in &mut attempt.effects {
            e.cue_level = Cue::Read;
            e.assisted = true;
        }
        forged.practice.current.as_mut().unwrap().prompt = source.prompt;
        assert_eq!(engine::validate_app(&forged).unwrap_err().status, 422);
        engine::validate_app(&o.app).unwrap();
    }
}
