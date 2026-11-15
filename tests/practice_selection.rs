//! What an agent sees through the Owner API: another Version's wording is
//! Scry's own Again even with paid judgment on, Next with nothing named
//! selects as Study does, and asking what is due lists only practised Cards.
use scry::{
    api,
    engine::{self, WorkConfig},
    grading::Verdict,
    model::*,
    owner_token::{MutationActor, OwnerToken},
    persistence, study, web,
};
use serde_json::{Value, json};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;
const MINUTE: i64 = 60_000;
const CREED: &str = "I believe in one God,\nthe Father almighty,\nmaker of heaven and earth,\nof all things visible and invisible.\n";
const CANTICLE: &str = "Light on the hill,\nwater in the vale,\nbread at the door.\n";
const PSALM: &str = "Lift the lamp,\nopen the gate,\nkeep the watch.\n";
const CREED_OLDER: &str = "We believe in one God,\nthe Father, the Almighty,\nmaker of heaven and earth,\nof all that is, seen and unseen.\n";

struct Agent {
    app: App,
    op: u64,
    now: i64,
    /// Whether the serving Scry can reach Jev.
    jev: bool,
}
impl Agent {
    /// An agent whose Owner token carries paid permission.
    fn new() -> Self {
        let mut app = App::new("selection-synthetic-csrf-0123456789".into());
        let t = OwnerToken::issue(
            "Selection local agent",
            None,
            true,
            NOW,
            "abcdef0123456789abcdef0123456789abcdef0123456789",
        )
        .unwrap();
        app.owner_tokens.insert(t.info.id.clone(), t);
        Self {
            app,
            op: 0,
            now: NOW,
            jev: false,
        }
    }
    /// The same agent where the Owner has turned paid judgment on and Scry
    /// can reach Jev.
    fn paid() -> Self {
        let mut a = Self::new();
        a.jev = true;
        a.app.preferences.paid_judgment = true;
        a
    }
    fn try_call(&mut self, path: &str, mut data: Value) -> AppResult<Value> {
        self.op += 1;
        data["operation_id"] = json!(format!("selection-operation-{:06}", self.op));
        let fields = api::library_fields(data)?;
        let t = self.app.owner_tokens.values().next().unwrap().info.clone();
        let r = engine::mutate_judged(
            &mut self.app,
            api::mutation_path(path).unwrap(),
            &fields,
            self.now,
            &format!("selection-seed-{}", self.op),
            MutationActor::Token(&t),
            self.jev,
        )?;
        engine::validate_app(&self.app)?;
        let rows = persistence::rows(&self.app).unwrap();
        assert_eq!(persistence::app_from_rows(&rows).unwrap(), self.app);
        Ok(study::wire_response(&self.app, &r).unwrap_or(json!({"id":r.entity_id})))
    }
    fn call(&mut self, path: &str, data: Value) -> Value {
        self.try_call(path, data)
            .unwrap_or_else(|e| panic!("{path}: {} {}", e.status, e.message))
    }
    fn add(&mut self, title: &str, name: &str, text: &str) -> String {
        self.call(
            "/api/v1/library/versions",
            json!({"data":{"title":title,"name":name,"kind":"text","text":text}}),
        )["id"]
            .as_str()
            .unwrap()
            .into()
    }
    fn learn(&mut self, version: &str) -> String {
        self.call("/api/v1/learn", json!({"version_id":version}))["study"]["id"]
            .as_str()
            .unwrap()
            .into()
    }
    fn held(&self) -> &study::Presentation {
        self.app.practice.current.as_ref().unwrap()
    }
    /// Next with nothing named, after the held result when there is one.
    fn next(&mut self) -> Value {
        let mut d = json!({});
        if let Some(p) = self
            .app
            .practice
            .current
            .as_ref()
            .filter(|p| p.attempt_id.is_some())
        {
            d["after"] = json!(p.prompt.id);
        }
        self.call("/api/v1/next", d)
    }
    /// Answer the held Prompt, trying from memory first where it is studied.
    fn answer(&mut self, text: &str) -> Value {
        if self.held().source_visible() {
            let pid = self.held().prompt.id.clone();
            self.call("/api/v1/quiz", json!({"prompt_id":pid}));
        }
        let pid = self.held().prompt.id.clone();
        let r = self.call("/api/v1/answer", json!({"prompt_id":pid,"answer":text}));
        self.now += MINUTE;
        r
    }
    /// Answer the held phrase with its own exact wording.
    fn answer_exactly(&mut self) -> Value {
        let p = self.held().clone();
        let v = &self.app.library.versions[&p.prompt.version_id];
        let text = v
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .filter(|u| p.prompt.unit_ids.contains(&u.id))
            .map(|u| {
                if v.kind == scry::library::Kind::KeyedMapping {
                    scry::library::mapping_pair(&u.text).unwrap().1
                } else {
                    u.text.as_str()
                }
            })
            .collect::<String>();
        self.answer(&text)
    }
    fn due(&self) -> Value {
        study::due(&self.app, self.now)
    }
}

/// Lesson entry for the 2011 Creed beside its earlier wording, answered with
/// the earlier wording of the first phrase.
fn other_wording(mut a: Agent) -> (Agent, Value) {
    let vid = a.add(
        "Synthetic Creed",
        "Roman Missal 2011 metadata; authored text",
        CREED,
    );
    a.add("Synthetic Creed", "Earlier authored wording", CREED_OLDER);
    a.call("/api/v1/lessons/start", json!({"version_id":vid}));
    let answered = a.answer("We believe in one God,");
    (a, answered)
}

#[test]
fn another_versions_wording_is_again_decided_by_scry_and_never_sent() {
    let (mut paid, wire) = other_wording(Agent::paid());
    let attempt = &wire["attempt"];
    assert_eq!(attempt["grade"]["verdict"], "Again");
    assert_eq!(
        attempt["grade"]["wrong_version_hint"]["name"], "Earlier authored wording",
        "identified as the other wording: {attempt}"
    );
    assert!(wire.get("judgment").is_none() && wire["practice"].get("judgment").is_none());
    let aid = attempt["id"].as_str().unwrap().to_string();
    let a = &paid.app.practice.attempts[&aid];
    assert_eq!(a.verdict(), Some(Verdict::Again));
    assert!(a.judgment.is_none() && !a.effects.is_empty());
    // No Check, no reservation, and nothing for the Worker to send.
    assert!(paid.app.practice.checks.is_empty());
    assert!(paid.app.spend.is_empty());
    let config = WorkConfig {
        generation: false,
        jev: true,
        critic: false,
        model: String::new(),
        jev_model: "synthetic-jev".into(),
    };
    assert!(
        engine::claim_check_work(&mut paid.app, paid.now, "selection-lease", &config)
            .unwrap()
            .is_none()
    );
    assert!(paid.app.spend.is_empty());
    // The page names the other wording.
    let page = web::render(&paid.app, "/", &BTreeMap::new(), paid.now, None);
    assert!(page.contains("Earlier authored wording"), "{page}");
    // Exactly what paid judgment off stores for the same answer.
    let (off, _) = other_wording(Agent::new());
    assert_eq!(paid.app.practice.attempts, off.app.practice.attempts);
    paid.app.preferences.paid_judgment = false;
    assert_eq!(paid.app, off.app);
    assert_eq!(
        persistence::rows(&paid.app).unwrap(),
        persistence::rows(&off.app).unwrap()
    );
}

#[test]
fn an_answer_matching_no_version_is_still_sent_to_jev() {
    // The other wording rule is narrow: a mismatch of no Version's wording
    // keeps going to Jev.
    let mut a = Agent::paid();
    let vid = a.add(
        "Synthetic Creed",
        "Roman Missal 2011 metadata; authored text",
        CREED,
    );
    a.add("Synthetic Creed", "Earlier authored wording", CREED_OLDER);
    a.call("/api/v1/lessons/start", json!({"version_id":vid}));
    let wire = a.answer("I believe in a single God");
    assert_eq!(wire["attempt"]["grade"]["wrong_version_hint"], Value::Null);
    assert_eq!(a.app.practice.checks.len(), 1);
}

#[test]
fn next_with_nothing_named_interleaves_studies_as_study_does() {
    let mut a = Agent::new();
    let canticle = a.add("Synthetic Canticle", "", CANTICLE);
    let psalm = a.add("Synthetic Psalm", "", PSALM);
    let studies = [a.learn(&canticle), a.learn(&psalm)];
    // Nothing served yet: one Study's first phrase, studied first.
    let shown = a.next();
    assert_eq!(shown["phase"], "study");
    let first = a.held().prompt.study_id.clone();
    let other = studies.iter().find(|s| **s != first).unwrap().clone();
    a.answer_exactly();
    // That Study was just served; the other, never served, is next, as
    // Study's mix chooses from the same notebook.
    let mut mixed = Agent {
        app: a.app.clone(),
        op: a.op,
        now: a.now,
        jev: a.jev,
    };
    let after = mixed.held().prompt.id.clone();
    mixed.call("/api/v1/mix", json!({"after":after}));
    let shown = a.next();
    assert_eq!(shown["phase"], "study");
    assert_eq!(a.held().prompt.study_id, other);
    assert_eq!(a.held().prompt.unit_ids, mixed.held().prompt.unit_ids);
    a.answer_exactly();
    // Then back to the first, now least recently served.
    a.next();
    assert_eq!(a.held().prompt.study_id, first);
    assert_eq!(a.held().targets[0].unit_index, 1);
    // A Next that names a Study keeps to it.
    a.answer_exactly();
    let after = a.held().prompt.id.clone();
    a.call("/api/v1/next", json!({"after":after,"study_id":first}));
    assert_eq!(a.held().prompt.study_id, first);
    // Next with nothing named starts no Work not yet started: the Starter
    // shelf stays unstarted.
    assert_eq!(a.app.practice.studies.len(), 2);
}

#[test]
fn next_with_nothing_named_serves_due_practised_material_before_introductions() {
    let mut a = Agent::new();
    let canticle = a.add("Synthetic Canticle", "", CANTICLE);
    let psalm = a.add("Synthetic Psalm", "", PSALM);
    let s1 = a.learn(&canticle);
    a.next();
    a.answer_exactly();
    let practised = a.held().targets[0].card_id.clone();
    // A lesson entered later introduces every phrase of the Psalm at once;
    // only its first is answered.
    let after = a.held().prompt.id.clone();
    a.call(
        "/api/v1/lessons/start",
        json!({"version_id":psalm,"after":after}),
    );
    a.answer_exactly();
    let introduced = a
        .app
        .practice
        .cards
        .values()
        .filter(|c| c.state.schedule.last_review_ms.is_none())
        .count();
    assert_eq!(
        introduced, 2,
        "two Psalm phrases introduced, never answered"
    );
    // When the Canticle phrase falls due, it is served first: the earlier
    // introductions are new learning, not due reviews.
    a.now = a.app.practice.cards[&practised].state.schedule.due_ms + 1;
    a.next();
    assert_eq!(a.held().prompt.study_id, s1);
    assert_eq!(a.held().targets[0].card_id, practised);
    assert_eq!(a.held().phase, Some(study::StudyPhase::Quiz));
    // Asking again while it is held resumes it, never replaces it.
    let held = a.held().prompt.id.clone();
    a.next();
    assert_eq!(a.held().prompt.id, held);
}

#[test]
fn next_with_nothing_named_never_serves_material_scry_cannot_grade() {
    let mut a = Agent::new();
    let oversized = a.add("Synthetic orchard", "", &"orchard ".repeat(1100));
    let sid = a.learn(&oversized);
    let before = a.app.clone();
    let rest = a.next();
    assert_eq!(rest["prompt"], Value::Null, "{rest}");
    assert!(a.app.practice.cards.is_empty());
    assert_eq!(a.app.practice.studies, before.practice.studies);
    // Named, its Study still presents it.
    a.call("/api/v1/next", json!({"study_id":sid}));
    assert!(a.app.practice.current.is_some());
}

#[test]
fn asking_what_is_due_lists_only_practised_cards() {
    let mut a = Agent::new();
    let vid = a.add("Synthetic Canticle", "", CANTICLE);
    // Lesson entry introduces every phrase; none has been answered.
    a.call("/api/v1/lessons/start", json!({"version_id":vid}));
    a.now += MINUTE;
    assert_eq!(a.app.practice.cards.len(), 3);
    let due = a.due();
    assert_eq!(due["due"], json!([]), "{due}");
    assert_eq!(due["next_due_ms"], Value::Null);
    assert_eq!(due["cards"].as_array().unwrap().len(), 3);
    // One answered phrase is due once its time comes; the others never are.
    a.answer_exactly();
    let practised = a.held().targets[0].card_id.clone();
    let due_ms = a.app.practice.cards[&practised].state.schedule.due_ms;
    assert_eq!(a.due()["next_due_ms"], json!(due_ms));
    a.now = due_ms + 1;
    assert_eq!(a.due()["due"], json!([practised]));
}

#[test]
fn the_lesson_says_who_checks_an_answer() {
    let mut a = Agent::new();
    let vid = a.add("Synthetic Canticle", "", CANTICLE);
    a.call("/api/v1/lessons/start", json!({"version_id":vid}));
    a.answer_exactly();
    let page = |app: &App, capable: &str| {
        web::render(
            app,
            "/",
            &BTreeMap::from([("__judgment".to_string(), capable.to_string())]),
            NOW,
            None,
        )
    };
    let local = "Typed wording is checked locally. Case and punctuation do not matter; words and order do. Accents remain significant; the supplied Latin edition uses unaccented spelling. No pronunciation assessment.";
    let judged = "Scry checks the typed wording, ignoring case and punctuation, and Jev checks an answer Scry cannot match.";
    // Off, or on where Jev cannot be reached: the line is as it was.
    assert!(page(&a.app, "true").contains(local));
    a.app.preferences.paid_judgment = true;
    assert!(page(&a.app, "false").contains(local));
    // On, where Jev can be reached: what is true.
    let on = page(&a.app, "true");
    assert!(on.contains(judged), "{on}");
    assert!(!on.contains("checked locally"));
}

#[test]
fn practice_and_error_pages_carry_no_reassurance() {
    let mut app = App::new("selection-synthetic-csrf-0123456789".into());
    let fields = BTreeMap::from([
        ("csrf".to_string(), app.csrf.clone()),
        (
            "operation_id".to_string(),
            "selection-nato-start-0001".to_string(),
        ),
    ]);
    engine::mutate(&mut app, "/practice/nato/start", &fields, NOW, "nato-seed").unwrap();
    // The NATO welcome once a Study exists and nothing is held.
    app.practice.current = None;
    let nato = BTreeMap::from([("practice".to_string(), "nato".to_string())]);
    let pages = [
        web::render(&app, "/", &nato, NOW, None),
        web::error_page(&app, 409, "Operation conflict.", NOW),
        web::error_page(&app, 500, "Scry could not save.", NOW),
    ];
    for page in &pages {
        for phrase in [
            "A little, often.",
            "Your practice is waiting.",
            "Let's return to what is saved.",
            "A moment to pause",
        ] {
            assert!(!page.contains(phrase), "{phrase}: {page}");
        }
    }
    assert!(pages[0].contains("<h1>From Alfa to Zulu.</h1>"));
    assert!(pages[1].contains("This conflicts with what is saved."));
    assert!(pages[2].contains("Scry could not complete this request."));
}

#[test]
fn next_inside_focused_nato_practice_keeps_its_own_order() {
    // An agent's NATO lesson drills every letter in order, beyond a Study's
    // Pace: Next with nothing named stays inside it.
    let mut a = Agent::new();
    a.call("/api/v1/lessons/start", json!({"starter":"nato"}));
    let sid = a.held().prompt.study_id.clone();
    for letter in 0..8 {
        assert_eq!(a.held().prompt.study_id, sid);
        assert_eq!(a.held().targets[0].unit_index, letter);
        a.answer_exactly();
        a.next();
    }
}
