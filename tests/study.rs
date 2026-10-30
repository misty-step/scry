use scry::{
    api, engine,
    grading::{Change, Verdict},
    learning,
    model::*,
    owner_token::{MutationActor, OwnerToken},
    persistence,
    study::{self, Cue},
};
use serde_json::{Value, json};
const NOW: i64 = 1_800_000_000_000;
struct Owner {
    app: App,
    op: u64,
    now: i64,
}
impl Owner {
    fn new() -> Self {
        let mut app = App::new("synthetic-csrf-at-least-24-chars".into());
        let t = OwnerToken::issue(
            "Study local agent",
            None,
            false,
            NOW,
            "abcdef0123456789abcdef0123456789abcdef0123456789",
        )
        .unwrap();
        app.owner_tokens.insert(t.info.id.clone(), t);
        Self {
            app,
            op: 0,
            now: NOW,
        }
    }
    fn call(&mut self, path: &str, mut data: Value) -> AppResult<Value> {
        self.op += 1;
        data["operation_id"] = json!(format!("study-operation-{:06}", self.op));
        self.operation(path, data)
    }
    fn operation(&mut self, path: &str, data: Value) -> AppResult<Value> {
        let fields = api::library_fields(data)?;
        let t = self.app.owner_tokens.values().next().unwrap().info.clone();
        let operation_seed = format!("seed-{}", fields.get("operation_id").unwrap());
        let r = engine::mutate_as(
            &mut self.app,
            api::mutation_path(path).unwrap(),
            &fields,
            self.now,
            &operation_seed,
            MutationActor::Token(&t),
        )?;
        engine::validate_app(&self.app)?;
        Ok(study::wire_response(&self.app, &r).unwrap_or(json!({"id":r.entity_id})))
    }
    fn add(&mut self, text: &str, strict: bool) -> String {
        self.call("/api/v1/library/versions",json!({"data":{"title":"Synthetic Creed","name":if strict{"Strict authored wording"}else{"Roman Missal 2011 metadata; authored text"},"kind":"text","text":text,"spelling_strict":strict}})).unwrap()["id"].as_str().unwrap().into()
    }
    fn learn(&mut self, v: &str, pace: &str) -> String {
        self.call("/api/v1/learn", json!({"version_id":v,"pace":pace}))
            .unwrap()["study"]["id"]
            .as_str()
            .unwrap()
            .into()
    }
    fn next(&mut self, card: Option<&str>) -> Value {
        let after = self
            .app
            .practice
            .current
            .as_ref()
            .filter(|p| p.attempt_id.is_some())
            .map(|p| p.prompt.id.clone());
        let mut d = json!({});
        if let Some(after) = after {
            d["after"] = json!(after)
        }
        if let Some(card) = card {
            d["card_id"] = json!(card)
        }
        self.call("/api/v1/next", d).unwrap()
    }
    fn answer(&mut self, text: &str) -> Value {
        let p = self
            .app
            .practice
            .current
            .as_ref()
            .unwrap()
            .prompt
            .id
            .clone();
        self.call("/api/v1/answer", json!({"prompt_id":p,"answer":text}))
            .unwrap()
    }
    fn fade_to_free(&mut self, text: &str) -> String {
        self.next(None);
        let cid = self.app.practice.current.as_ref().unwrap().targets[0]
            .card_id
            .clone();
        for i in 0..6 {
            self.answer(text);
            if i < 5 {
                self.next(Some(&cid));
            }
        }
        self.next(Some(&cid));
        assert_eq!(
            self.app.practice.current.as_ref().unwrap().prompt.cue_level,
            Cue::PreviousLine
        );
        cid
    }
}

#[test]
fn exact_history_shows_a_valid_api_created_attempt_instead_of_false_empty() {
    let mut o = Owner::new();
    let v = o.add("We conceived a patient garden.\n", false);
    o.learn(&v, "steady");
    o.next(None);
    let answer = o.answer("We imagined a patient garden.");
    let attempt = &answer["attempt"];
    assert_eq!(o.app.practice.attempts.len(), 1);
    assert!(o.app.events.is_empty());
    assert_eq!(attempt["grade"]["verdict"], "Again");
    assert_eq!(attempt["assisted"], true);
    let before = o.app.clone();
    let html = scry::web::render(&o.app, "/history", &Default::default(), o.now, None);
    assert!(
        html.contains(&format!(
            "data-attempt-id=\"{}\"",
            attempt["id"].as_str().unwrap()
        )),
        "History must render the saved exact Attempt"
    );
    assert!(!html.contains("Your first practice will appear here."));
    assert!(html.contains("We imagined a patient garden."));
    assert!(html.contains("We conceived a patient garden."));
    assert_eq!(o.app, before, "Rendering History does not mutate practice");
}

#[test]
fn study_receipts_assistance_fading_free_recall_once_daily_and_lapse() {
    let mut o = Owner::new();
    let v = o.add("We conceived a patient garden.\n", false);
    let s = o.learn(&v, "steady");
    assert_eq!(o.learn(&v, "light"), s);
    assert_eq!(o.app.practice.studies.len(), 1);
    let cid = o.fade_to_free("We conceived a patient garden.");
    let c = o.app.practice.cards[&cid].clone();
    assert_eq!(c.state.schedule, learning::new_card(NOW));
    assert!(
        o.app
            .practice
            .attempts
            .values()
            .all(|a| a.assisted && a.effects.iter().all(|e| !e.scheduled))
    );
    let prompt = o.app.practice.current.as_ref().unwrap().prompt.clone();
    assert_eq!(prompt.cue, "");
    let encoded = serde_json::to_string(&prompt).unwrap();
    assert!(!encoded.contains("conceived"));
    let original = o.app.clone();
    assert_eq!(study::history(&o.app).unwrap_err().status, 409);
    assert_eq!(o.app, original);
    let a = o.answer("WE, concieved a patient garden!")["attempt"].clone();
    assert_eq!(a["grade"]["verdict"], "Good");
    assert_eq!(a["effects"][0]["scheduled"], true);
    assert!(
        a["grade"]["diff"]
            .as_array()
            .unwrap()
            .iter()
            .any(|d| d["kind"] == "slip")
    );
    let scheduled = o.app.practice.cards[&cid].state.schedule.clone();
    assert_eq!(
        scheduled,
        learning::schedule(&c.state.schedule, 3, NOW).unwrap()
    );
    assert_eq!(
        o.call("/api/v1/next", json!({})).unwrap()["attempt"],
        a,
        "Reconnect holds feedback."
    );
    o.next(Some(&cid));
    let extra = o.answer("We conceived a patient garden.");
    assert_eq!(extra["attempt"]["effects"][0]["extra_practice"], true);
    assert_eq!(o.app.practice.cards[&cid].state.schedule, scheduled);
    assert_eq!(o.app.practice.cards[&cid].state.cue, Cue::SectionHeading);
    o.next(Some(&cid));
    let bad = o.answer("We imagined a patient garden.");
    assert_eq!(bad["attempt"]["grade"]["verdict"], "Again");
    assert_eq!(o.app.practice.cards[&cid].state.cue, Cue::PreviousLine);
    assert_eq!(o.app.practice.cards[&cid].state.schedule, scheduled);
    o.now += DAY_MS;
    o.next(Some(&cid));
    let again = o.answer("Wrong words");
    assert_eq!(again["attempt"]["effects"][0]["scheduled"], true);
    assert_eq!(o.app.practice.cards[&cid].state.cue, Cue::FirstLetter);
    assert_eq!(
        o.app.practice.cards[&cid].state.schedule,
        learning::schedule(&scheduled, 1, o.now).unwrap()
    );
    let saved = o.app.clone();
    let _ = study::due(&o.app, o.now);
    let _ = study::history(&o.app).unwrap();
    assert_eq!(saved, o.app);
    let archive = persistence::Archive::new(o.app.clone(), vec![], o.now);
    let bytes = archive.encode().unwrap();
    let decoded = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(decoded.app, o.app);
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&o.app).unwrap()).unwrap(),
        o.app
    );
}
#[test]
fn study_answer_replay_conflict_and_correction_keep_original() {
    let mut o = Owner::new();
    let v = o.add("We conceived a patient garden.\n", false);
    o.learn(&v, "steady");
    let cid = o.fade_to_free("We conceived a patient garden.");
    let p = o.app.practice.current.as_ref().unwrap().prompt.id.clone();
    let data = json!({"operation_id":"study-answer-replay-id","prompt_id":p,"answer":"We imagined a patient garden."});
    let first = o.operation("/api/v1/answer", data.clone()).unwrap();
    let before = o.app.clone();
    assert_eq!(o.operation("/api/v1/answer", data.clone()).unwrap(), first);
    assert_eq!(o.app, before);
    let mut changed = data;
    changed["answer"] = json!("changed");
    assert_eq!(
        o.operation("/api/v1/answer", changed).unwrap_err().status,
        409
    );
    assert_eq!(o.app, before);
    let aid = first["attempt"]["id"].as_str().unwrap();
    let original = o.app.practice.attempts[aid].clone();
    let corrected = o
        .call(
            "/api/v1/correct",
            json!({"prompt_id":p,"attempt_id":aid,"verdict":"Good"}),
        )
        .unwrap();
    assert_eq!(o.app.practice.attempts[aid], original);
    assert_eq!(
        o.app.practice.cards[&cid].state.schedule,
        learning::schedule(&original.effects[0].before.schedule, 3, NOW).unwrap()
    );
    assert_eq!(corrected["correction"]["authority"], "owner");
    assert_eq!(
        o.call(
            "/api/v1/correct",
            json!({"prompt_id":p,"attempt_id":aid,"verdict":"Good"})
        )
        .unwrap_err()
        .status,
        409
    );
    o.next(Some(&cid));
    assert_eq!(
        o.call("/api/v1/answer", json!({"prompt_id":p,"answer":"late"}))
            .unwrap_err()
            .status,
        409
    );
    let rows = persistence::rows(&o.app).unwrap();
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), o.app);
    let mut damaged = o.app.clone();
    damaged
        .practice
        .cards
        .get_mut(&cid)
        .unwrap()
        .state
        .schedule
        .reps += 1;
    assert!(engine::validate_app(&damaged).is_err());
}
#[test]
fn study_reveal_lifecycle_real_delete_refusal_and_old_empty_bytes() {
    let old = App::new("synthetic-csrf-at-least-24-chars".into());
    let rows = persistence::rows(&old).unwrap();
    assert!(!rows["meta"].contains("practice"));
    assert_eq!(
        persistence::rows(&persistence::app_from_rows(&rows).unwrap()).unwrap(),
        rows
    );
    let mut o = Owner::new();
    let v = o.add("We conceived a patient garden.\n", false);
    let sid = o.learn(&v, "steady");
    let cid = o.fade_to_free("We conceived a patient garden.");
    let p = o.app.practice.current.as_ref().unwrap().prompt.id.clone();
    let before = o.app.practice.cards[&cid].state.clone();
    let shown = o.call("/api/v1/reveal", json!({"prompt_id":p})).unwrap();
    let aid = shown["disclosure_attempt_id"].as_str().unwrap();
    assert!(o.app.practice.attempts[aid].disclosure);
    assert!(o.app.practice.attempts[aid].assisted);
    assert_eq!(o.app.practice.cards[&cid].state, before);
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&o.app).unwrap()).unwrap(),
        o.app
    );
    let attempts = o.app.practice.attempts.len();
    o.call("/api/v1/reveal", json!({"prompt_id":p})).unwrap();
    assert_eq!(o.app.practice.attempts.len(), attempts);
    let a = o.answer("We conceived a patient garden.");
    assert_eq!(a["attempt"]["assisted"], true);
    assert_eq!(o.app.practice.cards[&cid].state.schedule.reps, 0);
    let wid = o.app.library.versions[&v].work_id.clone();
    for status in ["pause", "resume", "drop"] {
        o.call(
            &format!("/api/v1/studies/{status}"),
            json!({"study_id":sid}),
        )
        .unwrap();
        for (path, payload) in [
            (
                "/api/v1/library/versions/delete",
                json!({"version_id":v,"revision":1}),
            ),
            (
                "/api/v1/library/works/delete",
                json!({"work_id":wid,"revision":1}),
            ),
        ] {
            let before = o.app.clone();
            assert_eq!(o.call(path, payload).unwrap_err().status, 409);
            assert_eq!(o.app, before);
        }
    }
    assert_eq!(o.app.study_version_refs[&sid], v);
    assert_eq!(o.learn(&v, "steady"), sid);
    assert_eq!(o.next(None)["prompt"], Value::Null);
    assert_eq!(o.app.practice.attempts.len(), 8);
    assert_eq!(o.app.spend, vec![]);
}
#[test]
fn phone_stream_uses_shared_prompt_verdict_diff_and_correction_receipts() {
    let mut o = Owner::new();
    let text = "We conceived a patient garden by the water.";
    let v = o.add(text, false);
    o.learn(&v, "steady");
    let cid = o.fade_to_free(text);
    let p = o.app.practice.current.as_ref().unwrap().prompt.clone();
    let query = [("__nonce".into(), "synthetic-render-nonce".into())].into();
    let page = scry::web::render(&o.app, "/recall", &query, NOW, None);
    assert!(page.contains("data-state=\"exact-prompt\""));
    assert!(!page.contains(text));
    assert_eq!(page.matches("<textarea").count(), 1);
    assert_eq!(page.matches(">Check my answer<").count(), 1);
    let a = o.answer("We concieved a tired garden the water extra.");
    let page = scry::web::render(&o.app, "/recall", &query, NOW, None);
    assert!(page.contains("data-verdict=\"Again\""));
    for d in a["attempt"]["grade"]["diff"].as_array().unwrap() {
        assert!(page.contains(&format!(
            "data-diff-kind=\"{}\"",
            d["kind"].as_str().unwrap()
        )));
    }
    assert!(page.contains(text));
    assert!(
        !page.contains("I was right"),
        "Self-rating UI is retired; historical engine receipts remain readable"
    );
    let held = o.app.clone();
    study::ensure_stream(&mut o.app, NOW, "refresh").unwrap();
    assert_eq!(o.app, held);
    let aid = a["attempt"]["id"].as_str().unwrap();
    let corrected = o
        .call(
            "/api/v1/correct",
            json!({"prompt_id":p.id,"attempt_id":aid,"verdict":"Good"}),
        )
        .unwrap();
    assert!(corrected["correction"].is_object());
    assert_eq!(o.app.practice.attempts[aid].grade.verdict, Verdict::Again);
    let page = scry::web::render(&o.app, "/recall", &query, NOW, None);
    assert!(page.contains("Owner Correction · Good"));
    assert!(page.contains("data-state=\"exact-result\""));
    assert_eq!(o.app.practice.cards[&cid].state.schedule.reps, 1);
    o.next(None);
    assert!(o.app.practice.current.is_none());
    let page = scry::web::render(&o.app, "/recall", &query, NOW, None);
    assert!(page.contains("Next due at"));
    assert!(!page.contains("<textarea"));
    assert!(page.contains("Open Library"));
    assert!(page.contains("href=\"/recall\">Review</a>"));
    assert!(page.contains(">Review</a>"));
}
#[test]
fn phone_result_discloses_complete_saved_diff_without_changing_authority() {
    let expected = "We conceived a patient garden by the water.";
    for (answer, strict, verdict) in [
        (expected, false, "Good"),
        ("We concieved a patient garden by the water.", false, "Good"),
        ("We concieved a patient garden by the water.", true, "Again"),
        (
            "We concieved a tired garden the water extra.",
            false,
            "Again",
        ),
        (
            "We conceived patient a garden by the water.",
            false,
            "Again",
        ),
        ("We imagined a patient garden by the water.", false, "Again"),
    ] {
        let mut o = Owner::new();
        let v = o.add(expected, strict);
        o.add("We imagined a patient garden by the water.", false);
        o.learn(&v, "steady");
        o.next(None);
        let a = o.answer(answer)["attempt"].clone();
        let before = o.app.clone();
        let page = notebook_page(&o, "/recall");
        assert_eq!(o.app, before, "Rendering cannot mutate a held Attempt");
        assert!(page.contains(&format!("data-verdict=\"{verdict}\"")));
        assert_eq!(page.matches("<h1").count(), 1);
        let (summary, full) = page
            .split_once("<details class=\"exact-full-answer\">")
            .unwrap();
        let (full, actions) = full.split_once("</details>").unwrap();
        assert!(summary.contains("Assisted practice · the schedule stayed unchanged."));
        assert!(summary.find("data-verdict").unwrap() < summary.find("Assisted practice").unwrap());
        assert!(!summary.contains("You answered"));
        assert!(!summary.contains("data-diff-kind"));
        assert!(!summary.contains("data-feedback-kind=\"correct\""));
        assert!(full.contains("<summary>Full answer</summary>"));
        assert!(full.contains("Synthetic Creed"));
        assert!(full.contains("Read the Unit"));
        assert!(full.contains(answer));
        assert!(full.contains(expected));
        assert!(full.contains(a["grade"]["authority"]["name"].as_str().unwrap()));
        assert!(full.contains(a["grade"]["authority"]["grader_version"].as_str().unwrap()));
        let diff = a["grade"]["diff"].as_array().unwrap();
        assert_eq!(
            summary.matches("data-feedback-kind=").count(),
            diff.iter().filter(|d| d["kind"] != "correct").count()
        );
        let mut remaining = full;
        for d in diff {
            let kind = d["kind"].as_str().unwrap();
            let marker = format!(
                "data-diff-kind=\"{kind}\" data-unit-index=\"{}\"",
                d["unit_index"]
            );
            let (_, tail) = remaining
                .split_once(&marker)
                .expect("Full Diff stays in saved order");
            let (word, rest) = tail.split_once("</li>").unwrap();
            for value in [d["expected"]["text"].as_str(), d["typed"]["text"].as_str()]
                .into_iter()
                .flatten()
            {
                assert!(
                    word.contains(value),
                    "Saved word remains visible in the full Diff"
                );
            }
            if kind == "correct" {
                assert!(word.contains("diff-label sr-only\">Correct</span>"));
            }
            remaining = rest;
        }
        if let Some(hint) = a["grade"]["wrong_version_hint"]["name"].as_str() {
            assert!(summary.contains(&format!("Your answer is closer to Version {hint}.")));
        }
        if answer == "We conceived patient a garden by the water." {
            assert!(summary.contains("Wrong · out of order"));
        }
        assert!(actions.contains("action=\"/practice/next\""));
        assert!(!actions.contains("action=\"/practice/correct\""));
        assert!(actions.contains("name=\"operation_id\""));
        study::ensure_stream(&mut o.app, NOW, "held-feedback-refresh").unwrap();
        assert_eq!(o.app, before);
        assert_eq!(notebook_page(&o, "/recall"), page);
    }
}

#[test]
fn study_recitation_grades_each_unit_and_strict_slips() {
    let mut o = Owner::new();
    let v = o.add(
        "We conceived a patient garden,\na gentle silver orchard.\n",
        false,
    );
    let sid = o.learn(&v, "steady");
    for text in ["We conceived a patient garden,", "a gentle silver orchard."] {
        o.fade_to_free(text);
        o.answer(text);
    }
    let current = o.app.practice.current.as_ref().unwrap().prompt.id.clone();
    let next = o
        .call(
            "/api/v1/next",
            json!({"after":current,"study_id":sid,"recite":"whole"}),
        )
        .unwrap();
    let p = &next["prompt"];
    assert_eq!(p["cue_level"], "whole_title");
    assert_eq!(p["context"], Value::Null);
    assert!(!p.to_string().contains("conceived"));
    let a = o.answer("We concieved a patient garden a harsh silver orchard.");
    let a = &a["attempt"];
    assert_eq!(a["grade"]["units"][0]["verdict"], "Good");
    assert_eq!(a["grade"]["units"][1]["verdict"], "Again");
    assert_eq!(a["effects"].as_array().unwrap().len(), 2);
    assert!(o.app.practice.attempts.values().last().is_some());
    let rendered = notebook_page(&o, "/recall");
    let (summary, full) = rendered
        .split_once("<details class=\"exact-full-answer\">")
        .unwrap();
    for index in [0, 1] {
        assert!(summary.contains(&format!("data-unit-index=\"{index}\"")));
        assert!(summary.contains(&format!("Unit {}", index + 1)));
    }
    assert!(full.contains("We conceived a patient garden,"));
    assert!(full.contains("a gentle silver orchard."));

    let mut strict = Owner::new();
    let v = strict.add("We conceived a patient garden.\n", true);
    strict.learn(&v, "steady");
    strict.next(None);
    let a = strict.answer("We concieved a patient garden.");
    assert_eq!(a["attempt"]["grade"]["verdict"], "Again");
    assert!(
        strict
            .app
            .practice
            .attempts
            .values()
            .next()
            .unwrap()
            .grade
            .diff
            .iter()
            .any(|d| matches!(d.change, Change::Slip { .. }))
    );
    assert_eq!(
        strict
            .app
            .practice
            .attempts
            .values()
            .next()
            .unwrap()
            .grade
            .verdict,
        Verdict::Again
    );
}
#[test]
fn study_paces_are_rolling_caps_and_recitation_never_introduces_units() {
    for (pace, limit) in [("light", 3), ("steady", 6), ("intense", 12)] {
        let mut o = Owner::new();
        let text = (0..13)
            .map(|i| format!("Synthetic clause number {i}.\n"))
            .collect::<String>();
        let v = o.add(&text, false);
        let s = o.learn(&v, pace);
        for i in 0..limit {
            let line = format!("Synthetic clause number {i}.");
            let cid = o.fade_to_free(&line);
            o.answer(&line);
            assert_eq!(o.app.practice.cards[&cid].unit_index, i);
        }
        let p = o.app.practice.current.as_ref().unwrap().prompt.id.clone();
        let before = o.app.clone();
        assert_eq!(
            o.call(
                "/api/v1/next",
                json!({"after":p,"study_id":s,"recite":"whole"})
            )
            .unwrap_err()
            .status,
            409
        );
        assert_eq!(o.app, before);
        assert!(o.next(None)["prompt"].is_null());
        assert_eq!(o.app.practice.cards.len(), limit);
        let ids = o.app.practice.cards.keys().cloned().collect::<Vec<_>>();
        o.now = NOW + DAY_MS - 1;
        for cid in ids {
            let index = o.app.practice.cards[&cid].unit_index;
            o.next(Some(&cid));
            o.answer(&format!("Synthetic clause number {index}."));
        }
        assert!(o.next(None)["prompt"].is_null());
        o.now += 1;
        o.next(None);
        assert_eq!(
            o.app.practice.current.as_ref().unwrap().targets[0].unit_index,
            limit
        );
    }
}
#[test]
fn populated_study_archive_requires_compatible_format_and_intact_payload_chunks() {
    let mut o = Owner::new();
    let v = o.add("We conceived a patient garden.\n", false);
    o.learn(&v, "steady");
    o.next(None);
    o.answer("We concieved a patient garden.");
    let archive = persistence::Archive::new(o.app.clone(), vec![], NOW);
    let bytes = archive.encode().unwrap();
    let mut wrong: Value = serde_json::from_slice(&bytes).unwrap();
    assert_eq!(wrong["format"], "scry-rust-durable-sqlite-study-v1");
    wrong["format"] = json!("scry-rust-durable-sqlite-library-v1");
    let wrong = serde_json::to_vec(&wrong).unwrap();
    assert!(persistence::Archive::decode(&wrong, &persistence::sha256(&wrong)).is_err());
    let mut ambiguous: Value = serde_json::from_slice(&bytes).unwrap();
    for attempt in ambiguous["app"]["practice"]["attempts"]
        .as_object_mut()
        .unwrap()
        .values_mut()
    {
        attempt.as_object_mut().unwrap().remove("revealed");
    }
    let ambiguous = serde_json::to_vec(&ambiguous).unwrap();
    assert!(
        persistence::Archive::decode(&ambiguous, &persistence::sha256(&ambiguous)).is_err(),
        "Pre-fix populated Attempts cannot infer Reveal from generic cue assistance."
    );
    let mut rows = persistence::rows(&o.app).unwrap();
    assert!(rows.keys().any(|k| k.starts_with("practice_payload/")));
    let key = rows
        .keys()
        .find(|k| k.starts_with("practice_payload/"))
        .unwrap()
        .clone();
    rows.remove(&key);
    assert!(persistence::app_from_rows(&rows).is_err());
    let mut changed = o.app.clone();
    changed
        .practice
        .attempts
        .values_mut()
        .next()
        .unwrap()
        .raw_answer = "A different answer".into();
    assert!(engine::validate_app(&changed).is_err());
}

#[test]
fn study_oversized_alignment_refuses_without_consuming_prompt_or_receipt() {
    let mut o = Owner::new();
    let text = "orchard ".repeat(1100);
    let v = o.add(&text, false);
    o.learn(&v, "steady");
    o.next(None);
    let before = o.app.clone();
    let prompt = before.practice.current.as_ref().unwrap().prompt.id.clone();
    let error = o
        .call("/api/v1/answer", json!({"prompt_id":prompt,"answer":text}))
        .unwrap_err();
    assert_eq!(error.status, 422);
    assert!(error.message.contains("capacity"));
    assert_eq!(o.app, before);
}

#[test]
fn recitation_admission_checks_exact_answer_and_normalized_word_boundary() {
    for (text, admitted) in [
        ("orchard ".repeat(999), true),
        ("orchard ".repeat(1000), false),
        ("orchard—garden ".repeat(500), false),
    ] {
        for scope in ["whole", "section"] {
            let mut o = Owner::new();
            let v = o.add(&text, false);
            let sid = o.learn(&v, "steady");
            o.next(None);
            o.answer("");
            let before = o.app.clone();
            let before_rows = persistence::rows(&before).unwrap();
            let mut request = json!({"after":before.practice.current.as_ref().unwrap().prompt.id,
                "study_id":sid,"recite":scope});
            if scope == "section" {
                request["section_id"] = json!(before.library.versions[&v].sections[0].id);
            }
            let result = o.call("/api/v1/next", request);
            if admitted {
                result.unwrap();
                assert_eq!(o.answer(&text)["attempt"]["grade"]["verdict"], "Good");
                assert!(persistence::rows(&o.app).is_ok());
            } else {
                let error = result.unwrap_err();
                assert_eq!(error.status, 422);
                assert!(error.message.contains("1,000,000"));
                assert_eq!(o.app, before);
                assert_eq!(persistence::rows(&o.app).unwrap(), before_rows);
            }
        }
    }
}

#[test]
fn recitation_admission_includes_matching_sibling_scope() {
    for scope in ["whole", "section"] {
        for (sibling_words, admitted) in [(1999, true), (2000, false)] {
            let mut o = Owner::new();
            let text = "orchard ".repeat(499);
            let v = o.add(&text, false);
            let sid = o.learn(&v, "steady");
            o.next(None);
            o.answer("");
            o.add(&"cedar ".repeat(sibling_words), false);
            let before = o.app.clone();
            let mut request = json!({"after":before.practice.current.as_ref().unwrap().prompt.id,
                "study_id":sid,"recite":scope});
            if scope == "section" {
                request["section_id"] = json!(before.library.versions[&v].sections[0].id);
            }
            let result = o.call("/api/v1/next", request);
            if admitted {
                result.unwrap();
                assert_eq!(o.answer(&text)["attempt"]["grade"]["verdict"], "Good");
            } else {
                assert_eq!(result.unwrap_err().status, 422);
                assert_eq!(o.app, before);
            }
        }
    }
}

#[test]
fn exact_stream_escapes_private_text_and_keeps_native_nonce_bound_forms() {
    let mut o = Owner::new();
    let text = "Expected <svg onload=alert(1)> text.";
    let v = o.add(text, false);
    o.learn(&v, "steady");
    o.fade_to_free(text);
    let query = [("__nonce".into(), "one-render".into())].into();
    let other = [("__nonce".into(), "another-render".into())].into();
    let first = scry::web::render(&o.app, "/recall", &query, NOW, None);
    let second = scry::web::render(&o.app, "/recall", &other, NOW, None);
    assert!(!first.contains(text));
    assert!(first.contains("method=\"post\" action=\"/practice/answer\""));
    assert!(first.contains("name=\"csrf\""));
    assert_ne!(first, second);
    o.answer("<script>alert(2)</script>");
    let held = scry::web::render(&o.app, "/recall", &query, NOW, None);
    assert!(!held.contains("<script>alert"));
    assert!(!held.contains("<svg onload"));
    assert!(held.contains("&lt;script&gt;alert(2)&lt;/script&gt;"));
    assert!(held.contains("Expected &lt;svg onload=alert(1)&gt; text."));
}

#[test]
fn study_old_disclosures_cannot_cross_a_new_protected_prompt() {
    for scope in ["unit", "section", "whole"] {
        let mut o = Owner::new();
        let text = "Copper lanterns shelter quiet gardens.";
        let v = o.add(text, false);
        let learn = json!({"operation_id":"replay-safe-learn-operation","version_id":v});
        let learned = o.operation("/api/v1/learn", learn.clone()).unwrap();
        let sid = learned["study"]["id"].as_str().unwrap();
        let next = json!({"operation_id":"replay-old-next-operation","study_id":sid});
        let first = o.operation("/api/v1/next", next.clone()).unwrap();
        let pid = first["prompt"]["id"].as_str().unwrap();
        let cid = o.app.practice.current.as_ref().unwrap().targets[0]
            .card_id
            .clone();
        let reveal = json!({"operation_id":"replay-old-reveal-operation","prompt_id":pid});
        let shown = o.operation("/api/v1/reveal", reveal.clone()).unwrap();
        assert_eq!(
            o.operation("/api/v1/reveal", reveal.clone()).unwrap(),
            shown
        );
        let answer =
            json!({"operation_id":"replay-old-answer-operation","prompt_id":pid,"answer":text});
        let answered = o.operation("/api/v1/answer", answer.clone()).unwrap();
        assert_eq!(
            o.operation("/api/v1/answer", answer.clone()).unwrap(),
            answered
        );
        let held = json!({"operation_id":"replay-old-held-operation"});
        o.operation("/api/v1/next", held.clone()).unwrap();
        let correct = json!({"operation_id":"replay-old-correct-operation","prompt_id":pid,"attempt_id":answered["attempt"]["id"],"verdict":"Again"});
        let corrected = o.operation("/api/v1/correct", correct.clone()).unwrap();
        assert_eq!(
            o.operation("/api/v1/correct", correct.clone()).unwrap(),
            corrected
        );
        for _ in 0..6 {
            o.next(Some(&cid));
            o.answer(text);
        }
        let after = o.app.practice.current.as_ref().unwrap().prompt.id.clone();
        let section = &o.app.library.versions[&v].sections[0].id;
        let selection = match scope {
            "section" => {
                json!({"after":after,"study_id":sid,"recite":"section","section_id":section})
            }
            "whole" => json!({"after":after,"study_id":sid,"recite":"whole"}),
            _ => json!({"after":after,"card_id":cid}),
        };
        let protected = o.call("/api/v1/next", selection).unwrap();
        assert!(o.app.practice.protected());
        assert_eq!(study::history(&o.app).unwrap_err().status, 409);
        for (path, payload) in [
            ("/api/v1/next", next),
            ("/api/v1/reveal", reveal),
            ("/api/v1/answer", answer.clone()),
            ("/api/v1/next", held),
        ] {
            let before = o.app.clone();
            assert_eq!(
                o.operation(path, payload.clone()).unwrap_err().status,
                409,
                "{scope}: {path}"
            );
            assert_eq!(o.app, before);
            let mut fields = api::library_fields(payload).unwrap();
            fields.insert("csrf".into(), o.app.csrf.clone());
            assert_eq!(
                engine::mutate(
                    &mut o.app,
                    api::mutation_path(path).unwrap(),
                    &fields,
                    NOW,
                    "browser-replay"
                )
                .unwrap_err()
                .status,
                409
            );
            assert_eq!(
                o.app, before,
                "The browser and API use the same receipt fence."
            );
        }
        // Corrections contain state, not answer text. Safe receipts still reconcile.
        let before = o.app.clone();
        assert_eq!(o.operation("/api/v1/correct", correct).unwrap(), corrected);
        assert_eq!(o.operation("/api/v1/learn", learn).unwrap(), learned);
        assert_eq!(o.app, before);
        let same = json!({"operation_id":"replay-current-safe-next"});
        assert_eq!(
            o.operation("/api/v1/next", same.clone()).unwrap(),
            protected
        );
        let before = o.app.clone();
        assert_eq!(o.operation("/api/v1/next", same).unwrap(), protected);
        assert_eq!(o.app, before);
        let mut changed = answer;
        changed["answer"] = json!("Changed payload");
        assert_eq!(
            o.operation("/api/v1/answer", changed).unwrap_err().status,
            409
        );
        assert_eq!(o.app, before);
        let final_attempt = o.answer(text);
        assert_eq!(final_attempt["attempt"]["assisted"], false);
        assert_eq!(final_attempt["attempt"]["effects"][0]["scheduled"], true);
        assert_eq!(o.app.practice.cards[&cid].state.schedule.reps, 1);
    }
}

#[test]
fn study_explicit_reveal_breaks_success_runs_at_every_rung_and_correction() {
    let mut o = Owner::new();
    let text = "Copper lanterns shelter quiet gardens.";
    let v = o.add(text, false);
    o.learn(&v, "steady");
    o.next(None);
    let cid = o.app.practice.current.as_ref().unwrap().targets[0]
        .card_id
        .clone();
    let rungs = [
        Cue::Read,
        Cue::FirstLetters,
        Cue::FirstLetter,
        Cue::PreviousLine,
        Cue::SectionHeading,
        Cue::WholeTitle,
    ];
    for (i, cue) in rungs.iter().enumerate() {
        assert_eq!(
            o.app.practice.current.as_ref().unwrap().prompt.cue_level,
            *cue
        );
        o.answer(text);
        assert_eq!(o.app.practice.cards[&cid].state.successes, 1);
        o.next(Some(&cid));
        let pid = o.app.practice.current.as_ref().unwrap().prompt.id.clone();
        o.call("/api/v1/reveal", json!({"prompt_id":pid})).unwrap();
        let schedule = o.app.practice.cards[&cid].state.schedule.clone();
        let copied = o.answer(text);
        assert_eq!(
            o.app.practice.cards[&cid].state.cue, *cue,
            "A revealed Good must not fade {cue:?}."
        );
        assert_eq!(o.app.practice.cards[&cid].state.successes, 0);
        assert_eq!(copied["attempt"]["revealed"], true);
        assert_eq!(o.app.practice.cards[&cid].state.schedule, schedule);
        o.next(Some(&cid));
        o.answer(text);
        o.next(Some(&cid));
        let pid = o.app.practice.current.as_ref().unwrap().prompt.id.clone();
        o.call("/api/v1/reveal", json!({"prompt_id":pid})).unwrap();
        let a = o.answer("Wrong words")["attempt"].clone();
        assert_eq!(
            o.app.practice.cards[&cid].state.cue, *cue,
            "A revealed Again must not lapse."
        );
        assert_eq!(o.app.practice.cards[&cid].state.successes, 0);
        let correction = json!({"operation_id":format!("revealed-correction-rung-{i}"),"prompt_id":pid,"attempt_id":a["id"],"verdict":"Good"});
        let corrected = o.operation("/api/v1/correct", correction.clone()).unwrap();
        assert_eq!(o.app.practice.cards[&cid].state.cue, *cue);
        assert_eq!(
            o.app.practice.cards[&cid].state.successes, 0,
            "A Correction retains explicit Reveal."
        );
        let before = o.app.clone();
        assert_eq!(
            o.operation("/api/v1/correct", correction).unwrap(),
            corrected
        );
        assert_eq!(o.app, before);
        assert_eq!(json!(o.app.practice.attempts[a["id"].as_str().unwrap()]), a);
        let bytes = persistence::Archive::new(o.app.clone(), vec![], o.now)
            .encode()
            .unwrap();
        assert_eq!(
            persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
                .unwrap()
                .app,
            o.app
        );
        let mut dishonest: Value = serde_json::from_slice(&bytes).unwrap();
        dishonest["app"]["practice"]["attempts"][copied["attempt"]["id"].as_str().unwrap()]["revealed"] =
            json!(false);
        let dishonest = serde_json::to_vec(&dishonest).unwrap();
        assert!(
            persistence::Archive::decode(&dishonest, &persistence::sha256(&dishonest)).is_err()
        );
        for _ in 0..2 {
            o.next(Some(&cid));
            o.answer(text);
        }
        let next_cue = rungs[(i + 1).min(rungs.len() - 1)];
        assert_eq!(o.app.practice.cards[&cid].state.cue, next_cue);
        assert_eq!(o.app.practice.cards[&cid].state.successes, 0);
        o.next(Some(&cid));
    }
}

#[test]
fn study_old_free_prompt_context_cannot_disclose_a_new_target() {
    let mut o = Owner::new();
    let first = "Copper lanterns shelter quiet gardens.";
    let second = "Amber streams carry patient wishes.";
    let v = o.add(&format!("{first}\n{second}"), false);
    o.learn(&v, "steady");
    let cid = o.fade_to_free(first);
    o.answer(first);
    o.fade_to_free(second);
    let operation = json!({"operation_id":"old-free-prompt-with-context"});
    let old = o.operation("/api/v1/next", operation.clone()).unwrap();
    assert_eq!(old["prompt"]["assisted"], false);
    assert_eq!(old["prompt"]["context"].as_str().unwrap().trim(), first);
    o.answer(second);
    o.next(Some(&cid));
    let before = o.app.clone();
    assert_eq!(
        o.operation("/api/v1/next", operation).unwrap_err().status,
        409
    );
    assert_eq!(o.app, before);
}

fn diagnostic_study_fixture() -> App {
    let mut o = Owner::new();
    let v = o.add("Copper lanterns shelter quiet gardens.", false);
    let sid = o.learn(&v, "steady");
    o.next(None);
    o.answer("Copper lanterns shelter quiet gardens.");
    o.call("/api/v1/studies/drop", json!({"study_id":sid}))
        .unwrap();
    o.app
}

fn assert_diagnostic_refusal(app: App) {
    let archive = persistence::Archive::new(app, vec![], NOW);
    let bytes = serde_json::to_vec(&archive).unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap_err()
            .status,
        422
    );
    assert_eq!(archive.diagnose().unwrap_err().status, 422);
}

#[test]
fn study_archive_diagnosis_refuses_missing_work_without_panicking() {
    let mut app = diagnostic_study_fixture();
    app.library.works.clear();
    assert_diagnostic_refusal(app);
}

#[test]
fn study_archive_diagnosis_refuses_missing_version_and_malformed_targets() {
    let app = diagnostic_study_fixture();
    let mut missing = app.clone();
    missing.library.versions.clear();
    assert_diagnostic_refusal(missing);
    for field in ["section", "unit", "card", "index", "study", "version"] {
        let mut missing = app.clone();
        let a = missing.practice.attempts.values_mut().next().unwrap();
        match field {
            "section" => {
                a.prompt.scope = study::Scope::Section {
                    section_id: "missing-section".into(),
                }
            }
            "unit" => a.effects[0].unit_id = "missing-unit".into(),
            "card" => a.effects[0].card_id = "missing-card".into(),
            "index" => a.effects[0].unit_index = usize::MAX,
            "study" => a.prompt.study_id = "missing-study".into(),
            "version" => a.prompt.version_id = "missing-version".into(),
            _ => unreachable!(),
        }
        assert_diagnostic_refusal(missing);
    }
}

fn mixed_notebook(exact: bool) -> Owner {
    let mut o = Owner::new();
    engine::seed_fixture(&mut o.app, NOW).unwrap();
    if exact {
        let v = o.add("A separate authored exact Unit.", false);
        o.learn(&v, "steady");
        o.next(None);
    }
    o
}

fn notebook_action(o: &mut Owner, path: &str, extra: &[(&str, &str)]) -> String {
    o.op += 1;
    let operation = format!("notebook-operation-{:06}", o.op);
    let mut fields = std::collections::BTreeMap::from([
        ("csrf".into(), o.app.csrf.clone()),
        ("operation_id".into(), operation.clone()),
    ]);
    if let Some(current) = &o.app.occurrence {
        fields.insert("occurrence_id".into(), current.id.clone());
    }
    fields.extend(extra.iter().map(|(k, v)| (k.to_string(), v.to_string())));
    let result = engine::mutate(&mut o.app, path, &fields, NOW, &operation).unwrap();
    assert_eq!(
        result.redirect, "/",
        "Shared receipt remains transport neutral"
    );
    let saved = o.app.clone();
    assert_eq!(
        engine::mutate(&mut o.app, path, &fields, NOW + 1, "replayed").unwrap(),
        result
    );
    assert_eq!(o.app, saved, "Lost-response replay changes no records");
    engine::validate_app(&o.app).unwrap();
    scry::web::mutation_redirect(&o.app, path, &result.redirect).into()
}

fn notebook_page(o: &Owner, path: &str) -> String {
    scry::web::render(&o.app, path, &Default::default(), NOW, None)
}

#[test]
fn mixed_notebook_holds_legacy_feedback_self_check_override_and_next() {
    for exact in [true, false] {
        let mut o = mixed_notebook(exact);
        let destination = "/notebook";
        let practice = o.app.practice.clone();
        let wrong = o.app.occurrence.as_ref().unwrap().presentation.choices[1].clone();
        assert_eq!(
            notebook_action(&mut o, "/review/answer", &[("answer", &wrong)]),
            destination
        );
        let page = notebook_page(&o, destination);
        assert!(page.contains("data-state=\"result\""));
        assert!(
            !page.contains("I was right"),
            "Self-rating UI is retired; historical engine receipts remain readable"
        );
        assert!(page.contains(&wrong));
        assert_eq!(
            notebook_action(&mut o, "/review/override", &[("correct", "true")]),
            destination
        );
        assert!(notebook_page(&o, destination).contains("You corrected this grade"));
        assert_eq!(o.app.events.len(), 1);
        assert_eq!(o.app.overrides.len(), 1);
        assert_eq!(o.app.events[0].result.outcome, "wrong");
        assert_eq!(notebook_action(&mut o, "/review/next", &[]), destination);
        assert_eq!(o.app.occurrence.as_ref().unwrap().phase, "question");
        assert_eq!(
            o.app.occurrence.as_ref().unwrap().presentation.kind,
            "recall"
        );
        assert_eq!(
            notebook_action(
                &mut o,
                "/review/answer",
                &[("answer", "my own uncertain answer")]
            ),
            destination
        );
        let config = engine::WorkConfig {
            generation: false,
            jev: false,
            critic: false,
            model: "disabled".into(),
            jev_model: "disabled".into(),
        };
        engine::claim_work(&mut o.app, NOW, "missing-model", &config).unwrap();
        assert!(notebook_page(&o, destination).contains("data-state=\"self-check\""));
        assert_eq!(notebook_action(&mut o, "/review/retry", &[]), destination);
        engine::claim_work(&mut o.app, NOW, "still-missing-model", &config).unwrap();
        assert_eq!(
            notebook_action(&mut o, "/review/self", &[("correct", "true")]),
            destination
        );
        assert!(notebook_page(&o, destination).contains("data-state=\"result\""));
        assert_eq!(o.app.events.last().unwrap().result.authority, "learner");
        assert_eq!(
            o.app.practice, practice,
            "Legacy practice never advances exact practice"
        );
        if exact {
            assert!(notebook_page(&o, "/recall").contains("data-state=\"exact-prompt\""));
        }
    }
}

#[test]
fn mixed_notebook_intro_reveal_concept_practice_and_resume_keep_context() {
    for exact in [true, false] {
        let mut o = mixed_notebook(exact);
        let destination = "/notebook";
        let practice = o.app.practice.clone();
        for c in o.app.concepts.values_mut() {
            c.introduced_ms = None;
        }
        o.app.occurrence = None;
        engine::ensure_occurrence(&mut o.app, NOW, "new-concept").unwrap();
        assert_eq!(o.app.occurrence.as_ref().unwrap().phase, "intro");
        assert_eq!(
            notebook_action(&mut o, "/review/intro", &[("known", "false")]),
            destination
        );
        assert!(notebook_page(&o, destination).contains("data-state=\"question\""));
        assert_eq!(notebook_action(&mut o, "/review/reveal", &[]), destination);
        assert!(notebook_page(&o, destination).contains("data-state=\"result\""));
        let concept = o.app.occurrence.as_ref().unwrap().concept_id.clone();
        o.app.occurrence = None;
        assert_eq!(
            notebook_action(&mut o, &format!("/concepts/{concept}/practice"), &[]),
            destination
        );
        assert!(notebook_page(&o, destination).contains("data-state=\"question\""));
        o.app.restored_paused = true;
        assert_eq!(
            notebook_action(&mut o, "/settings/resume", &[]),
            destination
        );
        assert!(!o.app.restored_paused);
        assert_eq!(o.app.practice, practice);
    }
}

#[test]
fn mixed_notebook_409_and_422_render_legacy_and_preserve_exact_prompt() {
    for exact in [true, false] {
        let mut o = mixed_notebook(exact);
        let destination = "/notebook";
        for (occurrence, answer, status) in [("stale", "", 409), ("current", "", 422)] {
            let before = o.app.clone();
            let fields = [
                ("csrf".into(), o.app.csrf.clone()),
                ("operation_id".into(), format!("rejected-notebook-{status}")),
                (
                    "occurrence_id".into(),
                    if occurrence == "current" {
                        o.app.occurrence.as_ref().unwrap().id.clone()
                    } else {
                        occurrence.into()
                    },
                ),
                ("answer".into(), answer.into()),
            ]
            .into();
            let error =
                engine::mutate(&mut o.app, "/review/answer", &fields, NOW, "rejected").unwrap_err();
            assert_eq!(error.status, status);
            assert_eq!(o.app, before);
            let path = scry::web::mutation_error_path(&o.app, "/review/answer");
            assert_eq!(path, destination);
            let page = scry::web::render(&o.app, path, &fields, NOW, Some(&error.message));
            assert!(page.contains("data-state=\"question\""));
            assert!(!page.contains("data-state=\"exact-"));
            assert!(page.contains(&error.message));
        }
        if exact {
            let paths = [
                "/review/intro",
                "/review/answer",
                "/review/reveal",
                "/review/self",
                "/review/retry",
                "/review/override",
                "/review/next",
                "/review/help",
                "/settings/resume",
                "/concepts/synthetic/practice",
            ];
            for path in paths {
                assert_eq!(
                    scry::web::mutation_error_path(&o.app, path),
                    "/notebook",
                    "{path}"
                );
            }
            assert_eq!(
                scry::web::mutation_redirect(&o.app, "/review/help", "/concepts/synthetic"),
                "/concepts/synthetic"
            );
            assert_eq!(
                scry::web::mutation_error_path(&o.app, "/practice/answer"),
                "/recall"
            );
            assert_eq!(
                scry::web::mutation_redirect(&o.app, "/practice/answer", "/practice/held"),
                "/recall"
            );
            assert_eq!(scry::web::mutation_error_path(&o.app, "/create"), "/create");
            assert_eq!(
                scry::web::mutation_error_path(&o.app, "/questions/synthetic/edit"),
                "/questions/synthetic/edit"
            );
            o.app.practice.current = None;
            assert!(notebook_page(&o, "/recall").contains("data-state=\"exact-caught-up\""));
            assert_eq!(
                scry::web::mutation_error_path(&o.app, "/review/answer"),
                "/notebook"
            );
        }
    }
}

fn rendered_links(html: &str) -> Vec<(String, String)> {
    html.split("<a ")
        .skip(1)
        .map(|anchor| {
            let (attributes, content) = anchor.split_once('>').unwrap();
            let attribute = |name: &str| {
                attributes
                    .split_once(&format!("{name}=\""))
                    .map(|(_, value)| value.split('"').next().unwrap().to_string())
            };
            let href = attribute("href").unwrap();
            let label = attribute("aria-label").unwrap_or_else(|| {
                let mut in_tag = false;
                content
                    .split("</a>")
                    .next()
                    .unwrap()
                    .chars()
                    .filter(|ch| match ch {
                        '<' => {
                            in_tag = true;
                            false
                        }
                        '>' => {
                            in_tag = false;
                            false
                        }
                        _ => !in_tag,
                    })
                    .collect::<String>()
                    .split_whitespace()
                    .collect::<Vec<_>>()
                    .join(" ")
            });
            (href, label)
        })
        .collect()
}

#[test]
fn mixed_notebook_recovery_links_follow_legacy_home() {
    use sha2::{Digest, Sha256};
    let recovery = [
        "Check again",
        "Keep trying",
        "Return to your saved answer →",
    ];
    // Home navigation is distinct from explicit recovery of concept practice.
    let practice = [
        "Scry, home",
        "Practice",
        "Review",
        "← Your concepts",
        "Back to review →",
        "Go to practice →",
        "Return to practice →",
    ];
    let mut wrong = Vec::new();
    let mut observed = std::collections::BTreeSet::new();
    for exact in ["none", "prompt", "caught-up"] {
        for phase in ["question", "checking", "result"] {
            let mut o = mixed_notebook(exact != "none");
            if exact == "caught-up" {
                let text = "A separate authored exact Unit.";
                o.fade_to_free(text);
                o.answer(text);
                o.next(None);
                assert!(o.app.practice.current.is_none());
            }
            if phase == "checking" {
                o.app.occurrence = None;
                for q in o
                    .app
                    .questions
                    .values_mut()
                    .filter(|q| q.content().kind == "choice")
                {
                    q.card.due_ms = NOW + DAY_MS;
                }
                engine::ensure_occurrence(&mut o.app, NOW, "unassisted-recall").unwrap();
                notebook_action(
                    &mut o,
                    "/review/answer",
                    &[("answer", "My uncertain freshness answer")],
                );
                assert!(!o.app.occurrence.as_ref().unwrap().assisted);
            } else if phase == "result" {
                let wrong = o.app.occurrence.as_ref().unwrap().presentation.choices[1].clone();
                notebook_action(&mut o, "/review/answer", &[("answer", &wrong)]);
            }
            assert_eq!(o.app.occurrence.as_ref().unwrap().phase, phase);
            let occurrence = o.app.occurrence.as_ref().unwrap();
            let routes = [
                "/notebook".to_string(),
                "/gate".into(),
                format!("/concepts/{}", occurrence.concept_id),
                format!(
                    "/goals/{}",
                    o.app.questions[&occurrence.question_id].goal_id
                ),
                format!("/questions/{}/edit", occurrence.question_id),
                "/missing".into(),
            ];
            let root = notebook_page(&o, "/recall");
            let expected = "/notebook";
            if exact != "none" {
                assert!(root.contains(if exact == "prompt" {
                    "data-state=\"exact-prompt\""
                } else {
                    "data-state=\"exact-caught-up\""
                }));
            }
            let before = o.app.clone();
            for route in routes {
                let page = notebook_page(&o, &route);
                if exact == "none" {
                    println!(
                        "no-Study {phase} {route} {}",
                        hex::encode(Sha256::digest(page.as_bytes()))
                    );
                }
                for (href, label) in rendered_links(&page) {
                    if recovery.contains(&label.as_str()) {
                        observed.insert(label.clone());
                        if href != expected {
                            wrong.push(format!("{exact} {phase} {route}: {label} -> {href}"));
                        }
                    } else if href == "/" && !practice.contains(&label.as_str()) {
                        wrong.push(format!(
                            "Unexpected root link: {exact} {phase} {route}: {label}"
                        ));
                    }
                }
            }
            assert_eq!(
                o.app, before,
                "Rendering recovery links does not mutate either practice"
            );
            assert_eq!(
                notebook_page(&o, "/recall"),
                root,
                "Text practice retains its identical exact Prompt/caught-up view"
            );
        }
    }
    assert_eq!(observed, recovery.into_iter().map(String::from).collect());
    assert!(wrong.is_empty(), "Wrong recovery destinations: {wrong:#?}");
}

fn exact_history_entry<'a>(html: &'a str, id: &str) -> &'a str {
    html.split(&format!("data-attempt-id=\"{id}\""))
        .nth(1)
        .unwrap()
        .split("</article>")
        .next()
        .unwrap()
}

#[test]
fn exact_history_preserves_original_correction_wording_and_dropped_study() {
    let mut o = Owner::new();
    let text = "We conceived a patient garden by the water.\n";
    let v = o.add(text, false);
    let sid = o.learn(&v, "steady");
    o.next(None);
    let result = o.answer("We concieved a tired garden the water extra.");
    let aid = result["attempt"]["id"].as_str().unwrap();
    let original = o.app.practice.attempts[aid].clone();
    let correction = o
        .call(
            "/api/v1/correct",
            json!({"prompt_id":original.prompt.id,"attempt_id":aid,"verdict":"Good"}),
        )
        .unwrap();
    let original_version = o.app.library.versions[&v].clone();
    o.call("/api/v1/library/versions/supersede", json!({"version_id":v,"revision":1,"data":{"title":"Synthetic Creed","name":"Revised wording","kind":"text","text":"The replacement text is never the historical answer.\n"}})).unwrap();
    let wid = original_version.work_id.clone();
    let revision = o.app.library.works[&wid].revision;
    o.call(
        "/api/v1/library/works/edit",
        json!({"data":{"work_id":wid,"revision":revision,"title":"Renamed Work"}}),
    )
    .unwrap();
    o.call("/api/v1/studies/drop", json!({"study_id":sid}))
        .unwrap();
    assert!(o.app.practice.current.is_none());
    let html = notebook_page(&o, "/history");
    let entry = exact_history_entry(&html, aid);
    assert!(entry.contains("<span>Again</span>"));
    assert!(entry.contains("<h2>Synthetic Creed</h2>"));
    assert!(entry.contains(text));
    assert!(!entry.contains("replacement text") && !entry.contains("Renamed Work"));
    for (needle, expected) in [
        ("data-diff-kind=\"", original.grade.diff.len()),
        ("data-correction-id=\"", 1),
    ] {
        assert_eq!(entry.matches(needle).count(), expected);
    }
    for kind in ["correct", "slip", "wrong", "missing", "extra"] {
        assert!(entry.contains(&format!("data-diff-kind=\"{kind}\"")));
    }
    assert!(entry.contains(correction["correction"]["id"].as_str().unwrap()));
    assert!(entry.contains("Authority: owner"));
    assert!(entry.contains(&original.grade.authority.name));
    assert!(entry.contains(&original.grade.authority.grader_version));
    assert!(entry.contains(&original.version_hash));
    assert!(!entry.contains("<form") && !entry.contains("/practice/"));
    assert_eq!(o.app.practice.attempts[aid], original);
    assert_eq!(o.app.library.versions[&v], original_version);
    let restarted = persistence::app_from_rows(&persistence::rows(&o.app).unwrap()).unwrap();
    assert_eq!(
        scry::web::render(&restarted, "/history", &Default::default(), NOW, None),
        html
    );
}

#[test]
fn exact_history_distinguishes_reveal_from_empty_answer_and_orders_by_sequence() {
    let mut o = Owner::new();
    let text = "We conceived a patient garden.\n";
    let v = o.add(text, false);
    o.learn(&v, "steady");
    let cid = o.fade_to_free(text);
    let before = notebook_page(&o, "/history");
    assert!(before.contains("History is hidden while you answer from memory."));
    assert!(!before.contains(text) && !before.contains("data-attempt-id"));
    let pid = o.app.practice.current.as_ref().unwrap().prompt.id.clone();
    let shown = o.call("/api/v1/reveal", json!({"prompt_id":pid})).unwrap();
    let disclosure = shown["disclosure_attempt_id"].as_str().unwrap();
    let empty = o.answer("");
    let empty_id = empty["attempt"]["id"].as_str().unwrap();
    let html = notebook_page(&o, "/history");
    let revealed = exact_history_entry(&html, disclosure);
    assert!(revealed.contains("Shown before answering"));
    assert!(revealed.contains("No answer submitted."));
    assert!(revealed.contains("comparison only, not a recall Verdict"));
    assert!(!revealed.contains("Again") && !revealed.contains("data-verdict"));
    let answered = exact_history_entry(&html, empty_id);
    assert!(answered.contains("<span>Again</span>"));
    assert!(answered.contains("No words submitted."));
    assert!(answered.contains("data-verdict=\"Again\""));
    let mut attempts = o.app.practice.attempts.values().collect::<Vec<_>>();
    attempts.sort_by_key(|a| std::cmp::Reverse(a.sequence));
    let positions = attempts
        .iter()
        .map(|a| html.find(&format!("data-attempt-id=\"{}\"", a.id)).unwrap())
        .collect::<Vec<_>>();
    assert!(positions.windows(2).all(|w| w[0] < w[1]));
    // A later unanswered Prompt must fence every earlier entry, including disclosures.
    o.next(Some(&cid));
    assert!(o.app.practice.protected());
    let protected = notebook_page(&o, "/history");
    for forbidden in [
        text,
        disclosure,
        empty_id,
        "data-diff-kind",
        "data-correction-id",
    ] {
        assert!(
            !protected.contains(forbidden),
            "Protected History leaked {forbidden}"
        );
    }
}

#[test]
fn exact_history_keeps_mixed_recitation_effects_and_scope_honest() {
    let mut o = Owner::new();
    let first = "A patient garden grows.\n";
    let second = "The lantern lights our path.\n";
    let v = o.call("/api/v1/library/versions", json!({"data":{"title":"Two quiet lines","name":"Original wording","kind":"text","text":format!("{first}{second}"),"sections":[{"heading":"The garden","units":[first]},{"heading":"The lantern","units":[second]}]}})).unwrap()["id"].as_str().unwrap().to_owned();
    let sid = o.learn(&v, "steady");
    let cid = o.fade_to_free(first);
    o.answer(first); // Unit 1 is now scheduled today.
    o.next(None);
    assert_ne!(
        o.app.practice.current.as_ref().unwrap().targets[0].card_id,
        cid
    );
    o.answer(second); // Unit 2 is introduced, but still assisted and unscheduled.
    let after = o.app.practice.current.as_ref().unwrap().prompt.id.clone();
    o.call(
        "/api/v1/next",
        json!({"after":after,"study_id":sid,"recite":"whole"}),
    )
    .unwrap();
    let whole = o.answer(&format!("{first}{second}"));
    let aid = whole["attempt"]["id"].as_str().unwrap();
    assert_eq!(whole["attempt"]["effects"][0]["extra_practice"], true);
    assert_eq!(whole["attempt"]["effects"][1]["scheduled"], true);
    let html = notebook_page(&o, "/history");
    let entry = exact_history_entry(&html, aid);
    assert!(entry.contains("Whole text"));
    assert!(entry.contains("data-unit-index=\"0\" data-schedule-effect=\"extra\""));
    assert!(entry.contains("data-unit-index=\"1\" data-schedule-effect=\"scheduled\""));
    assert!(entry.contains(first) && entry.contains(second));
    let section = o.app.library.versions[&v].sections[1].id.clone();
    let after = whole["attempt"]["prompt"]["id"].clone();
    o.call(
        "/api/v1/next",
        json!({"after":after,"study_id":sid,"recite":"section","section_id":section}),
    )
    .unwrap();
    let section_attempt = o.answer(second);
    let html = notebook_page(&o, "/history");
    let section_entry =
        exact_history_entry(&html, section_attempt["attempt"]["id"].as_str().unwrap());
    assert!(section_entry.contains("Section: The lantern"));
    assert!(section_entry.contains("data-unit-index=\"1\""));
    assert!(!section_entry.contains(first));
}

#[test]
fn exact_history_escapes_saved_words_and_coexists_with_legacy_privacy() {
    let mut o = mixed_notebook(true);
    let exact = o.answer("Exact answer <img src=x onerror=alert(1)> & saved");
    let aid = exact["attempt"]["id"].as_str().unwrap();
    assert!(o.app.protected_goal().is_some());
    let gated = notebook_page(&o, "/history");
    assert!(!gated.contains(aid) && !gated.contains("onerror"));
    assert!(gated.contains("History is hidden while you answer from memory."));
    let answer = o
        .app
        .occurrence
        .as_ref()
        .unwrap()
        .presentation
        .answer
        .clone();
    notebook_action(&mut o, "/review/answer", &[("answer", &answer)]);
    let page = notebook_page(&o, "/history");
    let entry = exact_history_entry(&page, aid);
    assert!(entry.contains("Exact answer &lt;img src=x onerror=alert(1)&gt; &amp; saved"));
    assert!(!entry.contains("<img"));
    assert_eq!(page.matches("<article class=\"history-entry").count(), 2);
    assert!(
        page.find("exact-history-entry").unwrap()
            < page.find("<article class=\"history-entry\">").unwrap()
    );
    // Keep the existing per-concept gate for the understanding notebook's own history.
    // Deliberate Next records exposure. Complete the warm session, then let
    // its real exposure window expire before asking again from memory.
    for _ in 0..3 {
        notebook_action(&mut o, "/review/next", &[]);
        let Some(current) = &o.app.occurrence else {
            break;
        };
        let answer = current.presentation.answer.clone();
        notebook_action(&mut o, "/review/answer", &[("answer", &answer)]);
    }
    assert!(o.app.occurrence.is_none());
    engine::ensure_occurrence(&mut o.app, NOW + DAY_MS + 1, "history-next-day").unwrap();
    engine::validate_app(&o.app).unwrap();
    assert!(o.app.protected_goal().is_some());
    let protected = notebook_page(&o, "/history");
    assert!(!protected.contains(aid));
    assert!(
        protected
            .contains("An earlier attempt is hidden while you work on its question from memory.")
    );
}
