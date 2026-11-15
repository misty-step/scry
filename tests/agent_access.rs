//! SPEC § Agent access and the Owner's control of a Study, through the same
//! route table the Worker uses for the API, CLI and tool server: Study's mix,
//! lessons for any supplied Version, a Study's Pace, and what showing a
//! Version returns.
use scry::{api, engine, model::*, owner_token::MutationActor, study, web};
use serde_json::{Value, json};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;

struct Agent {
    app: App,
    now: i64,
}
impl Agent {
    fn new() -> Self {
        Self {
            app: App::new("agent-access-synthetic-csrf-0123".into()),
            now: NOW,
        }
    }
    fn call(&mut self, route: &str, op: &str, mut payload: Value) -> AppResult<MutationResult> {
        payload["operation_id"] = json!(op);
        let fields = if route.starts_with("/api/v1/library/") {
            api::library_fields(payload)?
        } else {
            api::fields(payload)?
        };
        let path =
            api::mutation_path(route).ok_or_else(|| AppError::new(404, "API action not found."))?;
        engine::admit_current_mutation(path)?;
        let result = engine::mutate_as(
            &mut self.app,
            path,
            &fields,
            self.now,
            op,
            MutationActor::Browser,
        )?;
        engine::validate_app(&self.app).unwrap();
        Ok(result)
    }
    fn ok(&mut self, route: &str, op: &str, payload: Value) -> MutationResult {
        self.call(route, op, payload)
            .unwrap_or_else(|e| panic!("{route}: {} {}", e.status, e.message))
    }
    fn text(&mut self, title: &str, text: &str) -> String {
        self.ok(
            "/api/v1/library/versions",
            &format!("add-{}", title.replace(' ', "-")),
            json!({"data":{"title":title,"kind":"text","text":text}}),
        )
        .entity_id
        .unwrap()
    }
    fn held(&self) -> &study::Presentation {
        self.app.practice.current.as_ref().unwrap()
    }
    fn expected(&self) -> String {
        let p = &self.held().prompt;
        let v = &self.app.library.versions[&p.version_id];
        v.sections
            .iter()
            .flat_map(|s| &s.units)
            .filter(|u| p.unit_ids.contains(&u.id))
            .map(|u| {
                scry::library::mapping_pair(&u.text)
                    .filter(|_| v.kind == scry::library::Kind::KeyedMapping)
                    .map_or(u.text.as_str(), |(_, value)| value)
            })
            .collect()
    }
    /// Try from memory, then answer with the supplied words.
    fn recall(&mut self, n: usize) -> Value {
        let pid = self.held().prompt.id.clone();
        if self.held().phase == Some(study::StudyPhase::Study) {
            self.ok(
                "/api/v1/quiz",
                &format!("quiz-op-{n:012}"),
                json!({"prompt_id":pid}),
            );
        }
        let quiz = self.held().prompt.clone();
        let answer = self.expected();
        let result = self.ok(
            "/api/v1/answer",
            &format!("answer-op-{n:010}"),
            json!({"prompt_id":quiz.id,"answer":answer}),
        );
        study::wire_response(&self.app, &result).unwrap()
    }
    fn page(&self, path: &str) -> String {
        web::render(&self.app, path, &BTreeMap::new(), self.now, None)
    }
}

const FOUR: &str = "First synthetic phrase,\nsecond synthetic phrase,\nthird synthetic phrase,\nfourth synthetic phrase.\n";

/// SPEC § Agent access: no memorization action is web-only. The app's Study
/// and Next serve Study's mix; an agent reaches the same action.
#[test]
fn studys_mix_is_an_api_action_that_happens_once_and_protects_recall() {
    let mut a = Agent::new();
    let vid = a.text("Synthetic Mix Text", FOUR);
    let before = a.app.clone();
    // Asking what is due starts nothing.
    let _ = study::due(&a.app, a.now);
    assert_eq!(a.app, before);
    let first = a.ok("/api/v1/mix", "mix-operation-000001", json!({}));
    assert!(a.app.practice.mixed_review);
    let sid = a.held().prompt.study_id.clone();
    assert_eq!(a.app.practice.studies[&sid].version_id, vid);
    assert_eq!(a.held().phase, Some(study::StudyPhase::Study));
    // Replay returns the committed result and changes nothing.
    let committed = a.app.clone();
    assert_eq!(
        a.ok("/api/v1/mix", "mix-operation-000001", json!({})),
        first
    );
    assert_eq!(a.app, committed);
    // Reuse with a different payload is a conflict.
    let conflict = a
        .call(
            "/api/v1/mix",
            "mix-operation-000001",
            json!({"after":"a".repeat(32)}),
        )
        .unwrap_err();
    assert_eq!(conflict.status, 409);
    assert_eq!(a.app, committed);
    // While a Quiz awaits an unaided answer, the mix's response bears no answer.
    let pid = a.held().prompt.id.clone();
    a.ok(
        "/api/v1/quiz",
        "mix-quiz-operation-0001",
        json!({"prompt_id":pid}),
    );
    let expected = a.expected();
    let held = a.ok("/api/v1/mix", "mix-operation-000002", json!({}));
    assert!(a.app.practice.protected());
    let wire = study::wire_response(&a.app, &held).unwrap();
    assert!(!wire.to_string().contains(expected.trim_end()), "{wire}");
    assert_eq!(wire["phase"], "quiz");
    // A held unanswered Prompt is not released by after, only by leave.
    assert_eq!(
        a.call(
            "/api/v1/mix",
            "mix-operation-000003",
            json!({"after":a.held().prompt.id})
        )
        .unwrap_err()
        .status,
        409
    );
    // Answer, then the mix's Next serves the next material.
    a.recall(1);
    let answered = a.held().prompt.id.clone();
    a.ok(
        "/api/v1/mix",
        "mix-operation-000004",
        json!({"after":answered}),
    );
    assert_ne!(a.held().prompt.id, answered);
    assert!(a.app.practice.mixed_review);
    // leave closes the identified unanswered Prompt without a grade.
    let attempts = a.app.practice.attempts.len();
    let open = a.held().prompt.id.clone();
    a.ok("/api/v1/mix", "mix-operation-000005", json!({"leave":open}));
    assert_eq!(a.app.practice.attempts.len(), attempts);
    assert_ne!(a.held().prompt.id, open);
    // A client-supplied selector or grade is refused.
    for (n, field) in ["study_id", "card_id", "verdict", "pace"]
        .iter()
        .enumerate()
    {
        let error = a
            .call(
                "/api/v1/mix",
                &format!("mix-refused-operation-{n:03}"),
                json!({(*field):"Good"}),
            )
            .unwrap_err();
        assert_eq!(error.status, 422, "{field}");
    }
}

/// SPEC § Agent access: a lesson for any supplied Version, not only the
/// Starter shelf, including a mapping and a text of more than 128 phrases.
#[test]
fn a_lesson_starts_and_continues_for_a_supplied_mapping_and_a_long_text() {
    let mut a = Agent::new();
    let mapping = a
        .ok(
            "/api/v1/library/mappings",
            "add-mapping-operation-01",
            json!({"data":{"title":"Synthetic Capitals","direction":"forward","pairs":[
                {"key":"Synthland","value":"Testville"},
                {"key":"Mockovia","value":"Fixture City"},
                {"key":"Stubrania","value":"Port Sample"}]}}),
        )
        .entity_id
        .unwrap();
    let started = a.ok(
        "/api/v1/lessons/start",
        "lesson-mapping-op-0001",
        json!({"version_id":mapping}),
    );
    let committed = a.app.clone();
    assert_eq!(
        a.ok(
            "/api/v1/lessons/start",
            "lesson-mapping-op-0001",
            json!({"version_id":mapping})
        ),
        started,
        "replay returns the committed lesson"
    );
    assert_eq!(a.app, committed);
    assert_eq!(a.held().prompt.version_id, mapping);
    assert_eq!(a.held().phase, Some(study::StudyPhase::Study));
    let wire = a.recall(1);
    assert_eq!(wire["attempt"]["grade"]["verdict"], "Good");
    let answered = a.held().prompt.id.clone();
    let next = a.ok(
        "/api/v1/lessons/next",
        "lesson-mapping-op-0002",
        json!({"after":answered}),
    );
    let committed = a.app.clone();
    assert_eq!(
        a.ok(
            "/api/v1/lessons/next",
            "lesson-mapping-op-0002",
            json!({"after":answered})
        ),
        next
    );
    assert_eq!(a.app, committed);
    assert_eq!(
        a.call(
            "/api/v1/lessons/next",
            "lesson-mapping-op-0002",
            json!({"after":answered,"mode":"phrases"})
        )
        .unwrap_err()
        .status,
        409,
        "reuse with another payload conflicts"
    );
    assert_ne!(a.held().prompt.id, answered);
    assert_eq!(a.held().prompt.version_id, mapping);
    assert_eq!(
        a.call(
            "/api/v1/lessons/next",
            "lesson-mapping-op-0003",
            json!({"after":a.held().prompt.id,"mode":"join"})
        )
        .unwrap_err()
        .status,
        409,
        "an unanswered Prompt is not released"
    );
    // A text longer than 128 phrases is a lesson too: every phrase is available.
    let long = (1..=130)
        .map(|i| format!("Synthetic long line number {i}.\n"))
        .collect::<String>();
    let vid = a.text("Synthetic Long Text", &long);
    let open = a.held().prompt.id.clone();
    a.ok(
        "/api/v1/lessons/start",
        "lesson-long-op-000001",
        json!({"version_id":vid,"leave":open}),
    );
    let sid = a.held().prompt.study_id.clone();
    assert_eq!(a.app.practice.studies[&sid].version_id, vid);
    assert_eq!(
        a.app
            .practice
            .cards
            .values()
            .filter(|c| c.study_id == sid)
            .count(),
        130
    );
    assert_eq!(
        a.app.practice.studies[&sid].pace,
        study::Pace::Steady,
        "a lesson leaves the stored Pace alone"
    );
}

/// SPEC § Scheduling: the Owner chooses an existing Study's Pace.
#[test]
fn an_existing_studys_pace_changes_once_and_only_limits_new_units() {
    let mut a = Agent::new();
    let lines = (1..=8)
        .map(|i| format!("Paced synthetic line {i}.\n"))
        .collect::<String>();
    let vid = a.text("Synthetic Paced Text", &lines);
    let sid = a
        .ok(
            "/api/v1/learn",
            "learn-paced-operation-1",
            json!({"version_id":vid,"pace":"light"}),
        )
        .entity_id
        .unwrap();
    // Light introduces three new Units in a rolling day.
    let mut n = 0;
    let mut after = None::<String>;
    loop {
        n += 1;
        let mut payload = json!({"study_id":sid});
        if let Some(after) = &after {
            payload["after"] = json!(after);
        }
        a.ok("/api/v1/next", &format!("paced-next-op-{n:06}"), payload);
        if a.app.practice.current.is_none() {
            break;
        }
        a.recall(n);
        after = Some(a.held().prompt.id.clone());
        assert!(n < 20);
    }
    let cards = |a: &Agent| {
        a.app
            .practice
            .cards
            .values()
            .filter(|c| c.study_id == sid)
            .cloned()
            .collect::<Vec<_>>()
    };
    assert_eq!(cards(&a).len(), 3);
    let before = cards(&a);
    let page = a.page(&format!("/library/versions/{vid}"));
    assert!(page.contains("action=\"/practice/pace\""), "{page}");
    assert!(page.contains("value=\"light\" checked"));
    let set = a.ok(
        "/api/v1/studies/pace",
        "pace-operation-000001",
        json!({"study_id":sid,"pace":"intense"}),
    );
    assert_eq!(set.entity_id.as_deref(), Some(sid.as_str()));
    assert_eq!(a.app.practice.studies[&sid].pace, study::Pace::Intense);
    assert_eq!(cards(&a), before, "Cards and schedules are untouched");
    assert!(
        a.page(&format!("/library/versions/{vid}"))
            .contains("value=\"intense\" checked")
    );
    let committed = a.app.clone();
    assert_eq!(
        a.ok(
            "/api/v1/studies/pace",
            "pace-operation-000001",
            json!({"study_id":sid,"pace":"intense"})
        ),
        set
    );
    assert_eq!(a.app, committed, "replay changes nothing");
    assert_eq!(
        a.call(
            "/api/v1/studies/pace",
            "pace-operation-000001",
            json!({"study_id":sid,"pace":"light"})
        )
        .unwrap_err()
        .status,
        409
    );
    for (n, payload) in [
        json!({"study_id":sid,"pace":"fast"}),
        json!({"study_id":sid}),
        json!({"study_id":sid,"pace":"light","verdict":"Good"}),
        json!({"study_id":sid,"pace":"light","due_ms":0}),
    ]
    .into_iter()
    .enumerate()
    {
        let error = a
            .call(
                "/api/v1/studies/pace",
                &format!("pace-refused-op-{n:04}"),
                payload,
            )
            .unwrap_err();
        assert_eq!(error.status, 422);
    }
    assert_eq!(
        a.call(
            "/api/v1/studies/pace",
            "pace-unknown-op-00001",
            json!({"study_id":"b".repeat(32),"pace":"light"})
        )
        .unwrap_err()
        .status,
        404
    );
    assert_eq!(a.app, committed, "refusals write nothing");
    // Intense now allows more new Units the same day.
    a.ok(
        "/api/v1/next",
        "paced-next-op-after-pace",
        json!({"study_id":sid}),
    );
    assert_eq!(cards(&a).len(), 4);
    // Pause, resume and drop keep the chosen Pace.
    for (n, route) in ["/api/v1/studies/pause", "/api/v1/studies/resume"]
        .into_iter()
        .enumerate()
    {
        a.ok(
            route,
            &format!("pace-control-op-{n:04}"),
            json!({"study_id":sid}),
        );
    }
    assert_eq!(a.app.practice.studies[&sid].pace, study::Pace::Intense);
}

/// SPEC § Texts and prayers and § Lessons: a text that Study's mix first
/// reached introduces its phrases at its Pace. Outside the mix, its page offers
/// only controls that work, and a lesson's Next phrase without a Card says so.
#[test]
fn a_text_reached_from_the_mix_offers_no_next_phrase_it_cannot_serve() {
    let mut a = Agent::new();
    a.text("Synthetic Mix Lesson", FOUR);
    a.ok("/api/v1/mix", "mixtext-operation-0001", json!({}));
    a.recall(1);
    let answered = a.held().prompt.id.clone();
    // An agent's Next returns the held result outside the mix.
    a.ok("/api/v1/next", "mixtext-operation-0002", json!({}));
    assert!(!a.app.practice.mixed_review);
    let page = a.page("/");
    assert!(!page.contains("Next phrase"), "{page}");
    assert!(!page.contains("value=\"phrases\""), "{page}");
    assert!(page.contains("action=\"/practice/next\""), "{page}");
    let before = a.app.clone();
    let refusal = a
        .call(
            "/api/v1/lessons/next",
            "mixtext-operation-0003",
            json!({"after":answered,"mode":"phrases"}),
        )
        .unwrap_err();
    assert_eq!(refusal.status, 409);
    assert!(
        refusal.message.contains("not been introduced"),
        "{}",
        refusal.message
    );
    assert_eq!(a.app, before);
    // The control the page offers works.
    a.ok(
        "/api/v1/next",
        "mixtext-operation-0004",
        json!({"after":answered}),
    );
    assert_ne!(
        a.app.practice.current.as_ref().map(|p| p.prompt.id.clone()),
        Some(answered)
    );
    // Entering the lesson makes every phrase available, and Next phrase works.
    let vid = a.held().prompt.version_id.clone();
    let open = a.held().prompt.id.clone();
    a.ok(
        "/api/v1/lessons/start",
        "mixtext-operation-0005",
        json!({"version_id":vid,"leave":open}),
    );
    a.recall(2);
    assert!(a.page("/").contains("Next phrase"));
    let answered = a.held().prompt.id.clone();
    a.ok(
        "/api/v1/lessons/next",
        "mixtext-operation-0006",
        json!({"after":answered,"mode":"phrases"}),
    );
}

/// SPEC § Library: showing a Version shows its Work's Relations too.
#[test]
fn showing_a_version_shows_its_works_relations_and_the_library_reads_by_title() {
    let mut a = Agent::new();
    a.text("Zeta Synthetic Whole", "The whole synthetic text.\n");
    let part = a
        .ok(
            "/api/v1/library/versions",
            "add-part-operation-001",
            json!({"data":{"title":"Alpha Synthetic Part","kind":"text","text":"A part of it.\n",
                "relations":[{"kind":"part_of","work":"Zeta Synthetic Whole"}]}}),
        )
        .entity_id
        .unwrap();
    let related = a
        .ok(
            "/api/v1/library/versions",
            "add-related-operation-1",
            json!({"data":{"title":"middle synthetic neighbour","kind":"text","text":"Nearby words.\n",
                "relations":[{"kind":"related_to","work":"Zeta Synthetic Whole"}]}}),
        )
        .entity_id
        .unwrap();
    let kinds = |id: &str| {
        let shown =
            api::library_read(&a.app, &format!("/api/v1/library/versions/{id}"), "").unwrap();
        shown["relations"]
            .as_array()
            .unwrap()
            .iter()
            .map(|r| r["kind"].as_str().unwrap().to_string())
            .collect::<Vec<_>>()
    };
    assert!(
        kinds(&part).contains(&"part_of".into()),
        "{:?}",
        kinds(&part)
    );
    assert!(kinds(&related).contains(&"related_to".into()));
    let whole = a
        .app
        .library
        .versions
        .values()
        .find(|v| v.text.starts_with("The whole"))
        .unwrap()
        .id
        .clone();
    let both = kinds(&whole);
    assert!(both.contains(&"part_of".into()) && both.contains(&"related_to".into()));
    let titles = a
        .app
        .library
        .search("")
        .matches
        .iter()
        .map(|m| m.work.title.clone())
        .collect::<Vec<_>>();
    assert_eq!(
        titles,
        [
            "Alpha Synthetic Part",
            "middle synthetic neighbour",
            "Zeta Synthetic Whole"
        ]
    );
    let page = a.page(&format!("/library/versions/{part}"));
    assert!(page.contains("1 Section · 1 Unit<"), "{page}");
}
