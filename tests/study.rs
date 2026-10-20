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
        let r = engine::mutate_as(
            &mut self.app,
            api::mutation_path(path).unwrap(),
            &fields,
            self.now,
            &format!("seed-{}", self.op),
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
    o.call("/api/v1/reveal", json!({"prompt_id":p})).unwrap();
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
    assert_eq!(o.app.practice.attempts.len(), 7);
    assert_eq!(o.app.spend, vec![]);
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
