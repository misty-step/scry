use scry::{
    engine,
    model::*,
    persistence,
    study::{self, StudyPhase},
    web,
};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
struct Owner {
    app: App,
    n: u64,
}
impl Owner {
    fn new() -> Self {
        Self {
            app: App::new("source-quiz-synthetic-csrf-0123456789".into()),
            n: 0,
        }
    }
    fn fields(&mut self, pairs: &[(&str, String)]) -> BTreeMap<String, String> {
        self.n += 1;
        let mut f = BTreeMap::from([
            ("csrf".into(), self.app.csrf.clone()),
            ("revision".into(), self.app.revision.to_string()),
            (
                "operation_id".into(),
                format!("source-quiz-operation-{:06}", self.n),
            ),
        ]);
        f.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.clone())));
        f
    }
    fn call(&mut self, path: &str, pairs: &[(&str, String)], now: i64) -> MutationResult {
        let f = self.fields(pairs);
        let r = engine::mutate(
            &mut self.app,
            path,
            &f,
            now,
            &format!("source-quiz-seed-{}", self.n),
        )
        .unwrap();
        engine::validate_app(&self.app).unwrap();
        r
    }
    fn p(&self) -> study::Presentation {
        self.app.practice.current.clone().unwrap()
    }
    fn source(&mut self) {
        self.call(
            "/practice/lesson/start",
            &[("starter", "st-michael-en".into())],
            NOW,
        );
    }
    fn quiz(&mut self) {
        self.call("/practice/quiz", &[("prompt_id", self.p().prompt.id)], NOW);
    }
    fn expected(&self) -> String {
        let p = self.p();
        self.app.library.versions[&p.prompt.version_id]
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .filter(|u| p.prompt.unit_ids.contains(&u.id))
            .map(|u| u.text.as_str())
            .collect()
    }
    fn answer(&mut self, now: i64) {
        let text = self.expected();
        self.call(
            "/practice/answer",
            &[("prompt_id", self.p().prompt.id), ("answer", text)],
            now,
        );
    }
}
#[test]
fn source_to_memory_is_fenced_replayable_and_not_a_graded_copy() {
    let mut o = Owner::new();
    o.source();
    let source = o.p();
    let expected = o.expected();
    assert_eq!(source.phase, Some(StudyPhase::Study));
    assert!(source.source_visible());
    let html = web::render(&o.app, "/", &BTreeMap::new(), NOW, None);
    assert!(html.contains("Try from memory"));
    assert!(!html.contains("action=\"/practice/answer\""));
    assert!(!html.contains("id=\"answer\""));
    let invalid = o.fields(&[
        ("prompt_id", source.prompt.id.clone()),
        ("answer", expected.clone()),
    ]);
    let before = o.app.clone();
    assert_eq!(
        engine::mutate(&mut o.app, "/practice/answer", &invalid, NOW, "copy")
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(o.app, before);
    let f = o.fields(&[("prompt_id", source.prompt.id.clone())]);
    let cards = o.app.practice.cards.clone();
    let result = engine::mutate(&mut o.app, "/practice/quiz", &f, NOW, "memory").unwrap();
    let saved = o.app.clone();
    assert_eq!(
        engine::mutate(&mut o.app, "/practice/quiz", &f, NOW + 1, "retry").unwrap(),
        result
    );
    assert_eq!(o.app, saved);
    assert_ne!(o.p().prompt.id, source.prompt.id);
    assert_eq!(o.p().prompt.scope, source.prompt.scope);
    assert_eq!(o.p().prompt.unit_ids, source.prompt.unit_ids);
    assert_eq!(o.app.practice.cards, cards);
    assert!(o.app.practice.attempts.is_empty());
    assert!(o.app.practice.protected());
    let html = web::render(&o.app, "/", &BTreeMap::new(), NOW, None);
    assert!(!html.contains(expected.trim()));
    assert!(html.contains("action=\"/practice/answer\""));
    assert!(!html.contains("Copy the phrase"));
    let stale = o.fields(&[("prompt_id", source.prompt.id), ("answer", expected)]);
    let saved = o.app.clone();
    assert_eq!(
        engine::mutate(&mut o.app, "/practice/answer", &stale, NOW, "stale")
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(o.app, saved);
    o.answer(NOW);
    let a = o.app.practice.attempts.values().next().unwrap();
    assert!(!a.assisted);
    assert!(!a.disclosure);
    assert!(a.effects[0].scheduled);
}
#[test]
fn peek_to_hidden_retry_preserves_assistance_and_does_not_extend_schedule() {
    let mut o = Owner::new();
    o.source();
    o.quiz();
    let expected = o.expected();
    o.call("/practice/reveal", &[("prompt_id", o.p().prompt.id)], NOW);
    assert!(o.p().source_visible());
    assert_eq!(o.app.practice.attempts.len(), 1);
    assert!(o.app.practice.attempts.values().all(|a| a.disclosure));
    let cards = o.app.practice.cards.clone();
    let old = o.app.practice.attempts.clone();
    o.quiz();
    assert!(o.p().revealed);
    assert!(!o.p().source_visible());
    assert!(o.app.practice.protected());
    assert_eq!(o.app.practice.attempts, old);
    let page = web::render(&o.app, "/", &BTreeMap::new(), NOW, None);
    assert!(!page.contains(expected.trim()));
    assert!(study::history(&o.app).is_err());
    o.answer(NOW);
    let a = o
        .app
        .practice
        .attempts
        .values()
        .max_by_key(|a| a.sequence)
        .unwrap();
    assert!(a.assisted);
    assert!(a.effects.iter().all(|e| !e.scheduled));
    for (id, c) in cards {
        assert_eq!(o.app.practice.cards[&id].state.schedule, c.state.schedule);
    }
    // Later independent review is genuinely source-free, without rewriting cue history.
    o.call(
        "/practice/lesson/next",
        &[("after", o.p().prompt.id), ("mode", "middle".into())],
        NOW,
    );
    assert!(o.p().source_visible()); // another unseen phrase is source study, not a copy test
}
#[test]
fn due_review_hides_source_even_when_retained_card_cue_is_read() {
    let mut o = Owner::new();
    o.source();
    o.quiz();
    o.answer(NOW);
    let a = o.app.practice.attempts.values().next().unwrap();
    let card = a.effects[0].card_id.clone();
    let due = o.app.practice.cards[&card].state.schedule.due_ms;
    assert_eq!(o.app.practice.cards[&card].state.cue, study::Cue::Read);
    o.call("/practice/review/start", &[("after", o.p().prompt.id)], due);
    assert_eq!(o.p().targets[0].card_id, card);
    assert_eq!(o.p().phase, Some(StudyPhase::Quiz));
    assert!(!o.p().revealed);
    assert!(!o.p().prompt.assisted);
    assert!(o.p().prompt.context.is_none());
    assert!(!web::render(&o.app, "/", &BTreeMap::new(), due, None).contains(o.expected().trim()));
}
/// A notebook stored before every Study became study-then-quiz: it holds a
/// first-letters Prompt. See fixtures/cue-ladder-legacy.README.md.
fn stored_ladder() -> (Owner, i64) {
    let fixture: serde_json::Value =
        serde_json::from_str(include_str!("fixtures/cue-ladder-legacy.json")).unwrap();
    let saved = &fixture["ladder"]["states"]["held_first_letters"];
    let archive = persistence::Archive::decode(
        saved["archive"].as_str().unwrap().as_bytes(),
        saved["sha256"].as_str().unwrap(),
    )
    .unwrap();
    assert_eq!(
        archive.format,
        "scry-rust-durable-sqlite-study-scheduled-v2"
    );
    (
        Owner {
            app: archive.app,
            n: 0,
        },
        saved["now_ms"].as_i64().unwrap() + 1_000,
    )
}
#[test]
fn phase_archive_is_explicit_and_legacy_prompt_and_attempt_bytes_stay_compatible() {
    let (mut o, now) = stored_ladder();
    assert_eq!(o.p().phase, None);
    assert_eq!(o.p().prompt.cue_level, study::Cue::FirstLetters);
    let page = web::render(&o.app, "/", &BTreeMap::new(), now, None);
    assert!(page.contains("Try from memory"));
    assert!(!page.contains("id=\"answer\""));
    let earlier = o.app.practice.attempts.clone();
    o.answer(now);
    let answered = o.app.practice.attempts[o.p().attempt_id.as_ref().unwrap()].clone();
    assert!(answered.assisted && answered.prompt.cue_level == study::Cue::FirstLetters);
    assert!(
        earlier
            .iter()
            .all(|(id, a)| &o.app.practice.attempts[id] == a)
    );
    let legacy = serde_json::to_value(&o.app.practice.attempts).unwrap();
    o.call("/practice/next", &[("after", o.p().prompt.id)], now);
    assert_eq!(o.p().phase, Some(StudyPhase::Quiz));
    assert!(!o.p().prompt.assisted);
    assert_eq!(
        serde_json::to_value(&o.app.practice.attempts).unwrap(),
        legacy
    );
    let archive = persistence::Archive::new(o.app.clone(), vec![], now);
    assert_eq!(archive.format, "scry-rust-durable-sqlite-study-phase-v3");
    let bytes = archive.encode().unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        o.app
    );
    let mut forged = archive;
    forged.format = "scry-rust-durable-sqlite-study-scheduled-v2".into();
    assert!(forged.validate().is_err());
}

#[test]
fn study_then_quiz_survives_an_empty_current_for_every_study() {
    let mut o = Owner::new();
    o.call(
        "/practice/lesson/start",
        &[("starter", "baby-soothing".into())],
        NOW,
    );
    let sid = o.p().prompt.study_id;
    assert!(o.app.practice.studies[&sid].source_quiz);
    // A retained, valid exhausted/dismissed session has no held presentation.
    o.app.practice.current = None;
    engine::validate_app(&o.app).unwrap();
    o.call("/practice/next", &[("study_id", sid.clone())], NOW);
    // The pair was shown but never answered: it is studied again, not quizzed cold.
    assert_eq!(o.p().phase, Some(StudyPhase::Study));
    let refuse = |o: &mut Owner, seed: &str| {
        let before = o.app.clone();
        let fields = o.fields(&[("prompt_id", o.p().prompt.id), ("answer", "swaddle".into())]);
        assert_eq!(
            engine::mutate(&mut o.app, "/practice/answer", &fields, NOW, seed)
                .unwrap_err()
                .status,
            409
        );
        assert_eq!(o.app, before);
    };
    refuse(&mut o, "source-copy");
    o.quiz();
    assert_eq!(o.p().phase, Some(StudyPhase::Quiz));
    o.call("/practice/reveal", &[("prompt_id", o.p().prompt.id)], NOW);
    assert_eq!(o.p().phase, Some(StudyPhase::Study));
    refuse(&mut o, "empty-copy");
    // A Study started by learn alone is no different, for any client.
    let mut learned = Owner::new();
    let vid = learned
        .app
        .library
        .add(
            scry::lessons::input("st-michael-en").unwrap(),
            "learn-default",
            None,
        )
        .unwrap();
    let sid = learned
        .call("/practice/learn", &[("version_id", vid)], NOW)
        .entity_id
        .unwrap();
    assert!(!learned.app.practice.studies[&sid].source_quiz);
    for _ in 0..2 {
        learned.call("/practice/next", &[("study_id", sid.clone())], NOW);
        assert_eq!(learned.p().phase, Some(StudyPhase::Study));
        assert!(learned.app.practice.studies[&sid].source_quiz);
        learned.app.practice.current = None;
        engine::validate_app(&learned.app).unwrap();
    }
    let archive = persistence::Archive::new(learned.app.clone(), vec![], NOW);
    assert_eq!(archive.format, "scry-rust-durable-sqlite-study-phase-v3");
}
