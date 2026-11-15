//! The earlier concept notebook is history. For every state stored in
//! `tests/fixtures/earlier-notebook.json`: nothing the runtime runs completes,
//! sends, restarts or rewrites a saved check; no page serves a question; and
//! the notebook stands in the way of nothing else. A Check of a Study answer
//! is still claimed and recovered beside it.
//!
//! These read stored bytes only. `qa/earlier-notebook.test.mjs` proves the same
//! on the built Worker, where the alarm, the provider and SQLite are real.
use scry::{
    engine::{self, WorkConfig},
    model::*,
    owner_token::MutationActor,
    persistence::Archive,
    study::{self, CheckStatus},
    web,
};
use serde_json::{Value, json};
use std::collections::BTreeMap;

const FIXTURE: &str = concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/tests/fixtures/earlier-notebook.json"
);
const SECOND: i64 = 1_000;
const YEAR: i64 = 365 * DAY_MS;

/// Every stored state: its name, the notebook and the clock it was saved at.
fn stored() -> Vec<(String, App, i64)> {
    let fixture: BTreeMap<String, Value> =
        serde_json::from_str(&std::fs::read_to_string(FIXTURE).unwrap()).unwrap();
    assert_eq!(fixture.len(), 29);
    fixture
        .into_iter()
        .map(|(name, v)| {
            let archive = v["archive"].as_str().unwrap().as_bytes();
            let app = Archive::decode(archive, v["sha256"].as_str().unwrap())
                .unwrap()
                .app;
            (name, app, v["now_ms"].as_i64().unwrap())
        })
        .collect()
}

fn one(name: &str) -> (App, i64) {
    stored()
        .into_iter()
        .find(|(n, _, _)| n == name)
        .map(|(_, app, now)| (app, now))
        .unwrap()
}

/// A Scry that can reach Jev: what the alarm claims with in the Owner's space.
fn jev() -> WorkConfig {
    WorkConfig {
        generation: false,
        jev: true,
        critic: false,
        model: String::new(),
        jev_model: "synthetic-jev".into(),
    }
}

/// Everything the earlier concept notebook holds, as one comparable value.
fn earlier(app: &App) -> Value {
    json!({
        "goals": app.goals,
        "concepts": app.concepts,
        "questions": app.questions,
        "events": app.events,
        "overrides": app.overrides,
        "resets": app.resets,
        "feedback": app.feedback,
        "assessments": app.assessments,
        "occurrence": app.occurrence,
        "spend": app.spend,
        "pace": app.preferences.pace,
    })
}

fn page(app: &App, path: &str, now: i64) -> String {
    web::render(
        app,
        path,
        &BTreeMap::from([("__nonce".into(), "earlier-history-nonce".into())]),
        now,
        None,
    )
}

/// The alarm, a Check claim with Jev configured, resuming paused judgment and
/// a restore are everything that once moved a saved check. At one hundred
/// seconds and at ten years they leave every state exactly as stored.
#[test]
fn nothing_the_runtime_runs_completes_a_frozen_check() {
    for (name, saved, saved_ms) in stored() {
        for later in [100 * SECOND, 10 * YEAR] {
            let now = saved_ms + later;
            let at = format!("{name} +{}s", later / SECOND);

            // The alarm: recovery, then a claim.
            let mut app = saved.clone();
            assert!(!engine::recover_checks(&mut app, now), "{at}: recovery");
            assert!(app == saved, "{at}: recovery changed the notebook");
            let claimed = engine::claim_check_work(&mut app, now, "earlier-lease-a", &jev());
            assert!(claimed.unwrap().is_none(), "{at}: something was claimed");
            assert!(app == saved, "{at}: a claim changed the notebook");

            // Resuming paused judgment, then the alarm again.
            match engine::resume_work(&mut app) {
                Ok(()) => assert!(saved.restored_paused, "{at}: resumed an unpaused notebook"),
                Err(e) => assert_eq!((e.status, saved.restored_paused), (409, false), "{at}"),
            }
            assert!(!app.restored_paused);
            assert!(!engine::recover_checks(&mut app, now), "{at}: after resume");
            let claimed = engine::claim_check_work(&mut app, now, "earlier-lease-b", &jev());
            assert!(claimed.unwrap().is_none(), "{at}: resume started a check");
            assert!(earlier(&app) == earlier(&saved), "{at}: resume moved it");
            assert_eq!(app.spend.len(), saved.spend.len(), "{at}: a reservation");

            // A restore, then the alarm in the restored notebook and after its resume.
            let mut restored = saved.clone();
            engine::pause_restored(&mut restored, now).unwrap();
            assert!(restored.restored_paused);
            assert!(
                earlier(&restored) == earlier(&saved),
                "{at}: restore rewrote it"
            );
            engine::validate_app(&restored).unwrap();
            for resumed in [false, true] {
                if resumed {
                    engine::resume_work(&mut restored).unwrap();
                }
                assert!(!engine::recover_checks(&mut restored, now + SECOND));
                let claimed = engine::claim_check_work(
                    &mut restored,
                    now + SECOND,
                    "earlier-lease-c",
                    &jev(),
                );
                assert!(claimed.unwrap().is_none(), "{at}: claimed after restore");
                assert!(
                    earlier(&restored) == earlier(&saved),
                    "{at}: moved after restore"
                );
            }
            engine::validate_app(&restored).unwrap();
        }
    }
}

/// Practice Checks keep working beside a frozen check: a Study answer is
/// queued, claimed and, when its send is interrupted, recovered to unknown,
/// while the earlier check saved as queued or sent is never read.
#[test]
fn a_study_check_is_claimed_and_recovered_beside_a_frozen_one() {
    for name in ["checking-queued", "checking-sent", "queued-while-paused"] {
        let (saved, saved_ms) = one(name);
        let mut app = saved.clone();
        app.restored_paused = false;
        app.preferences.paid_judgment = true;
        let mut now = saved_ms + 10 * YEAR;
        let mut n = 0;
        let mut call = |app: &mut App, now: i64, path: &str, pairs: &[(&str, &str)]| {
            n += 1;
            let mut fields = BTreeMap::from([
                ("csrf".to_string(), app.csrf.clone()),
                (
                    "operation_id".to_string(),
                    format!("earlier-history-{n:06}"),
                ),
            ]);
            fields.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.to_string())));
            let seed = format!("earlier-history-seed-{n}");
            let result =
                engine::mutate_judged(app, path, &fields, now, &seed, MutationActor::Browser, true)
                    .unwrap_or_else(|e| panic!("{name} {path}: {} {}", e.status, e.message));
            engine::validate_app(app).unwrap();
            result
        };
        let data = json!({"title":"Synthetic harbour practice","direction":"forward","spelling_strict":false,"pairs":[
            {"key":"Before leaving the berth","value":"check the bilge pump"},
            {"key":"In fog","value":"sound the horn"}
        ]})
        .to_string();
        let version = call(&mut app, now, "/library/mapping/add", &[("data", &data)])
            .entity_id
            .unwrap();
        call(
            &mut app,
            now,
            "/practice/study/start",
            &[("version_id", &version)],
        );
        let prompt = app.practice.current.clone().unwrap().prompt.id;
        call(&mut app, now, "/practice/quiz", &[("prompt_id", &prompt)]);
        let prompt = app.practice.current.clone().unwrap().prompt.id;
        call(
            &mut app,
            now,
            "/practice/answer",
            &[("prompt_id", &prompt), ("answer", "test the bilge pump")],
        );
        let status = |app: &App| {
            app.practice
                .checks
                .values()
                .map(|c| c.status)
                .collect::<Vec<_>>()
        };
        assert_eq!(status(&app), [CheckStatus::Queued], "{name}");
        assert!(earlier(&app) == earlier(&saved), "{name}: Study moved it");

        // The alarm claims the Study answer's Check and nothing else.
        now += SECOND;
        let work = engine::claim_check_work(&mut app, now, "earlier-check-lease", &jev())
            .unwrap()
            .expect("the Study answer's Check");
        assert_eq!(work.kind, engine::CHECK_WORK);
        assert!(engine::awaits_send(&app, &work));
        assert_eq!(status(&app), [CheckStatus::Sent]);
        assert_eq!(app.spend.len(), saved.spend.len() + 1);
        assert_eq!(app.spend.last().unwrap().work_id, work.id);
        assert!(
            app.spend[..saved.spend.len()] == saved.spend[..],
            "{name}: an earlier reservation"
        );
        let frozen = |app: &App| {
            json!([
                app.assessments,
                app.occurrence,
                app.concepts,
                app.events,
                app.questions
            ])
        };
        assert!(frozen(&app) == frozen(&saved), "{name}: the claim read it");
        assert!(
            engine::claim_check_work(&mut app, now, "earlier-second-lease", &jev())
                .unwrap()
                .is_none(),
            "{name}: one send"
        );

        // The send is interrupted. After the recovery time the Check alone is unknown.
        assert!(!engine::recover_checks(&mut app, now + 89 * SECOND));
        assert_eq!(status(&app), [CheckStatus::Sent]);
        assert!(engine::recover_checks(&mut app, now + 90 * SECOND));
        assert_eq!(status(&app), [CheckStatus::Unknown]);
        assert!(!engine::recover_checks(&mut app, now + 10 * YEAR));
        assert!(frozen(&app) == frozen(&saved), "{name}: recovery read it");
        assert_eq!(
            app.spend.len(),
            saved.spend.len() + 1,
            "{name}: never resent"
        );
        engine::validate_app(&app).unwrap();
    }
}

/// What `/notebook` must show for a stored state, from the stored facts alone.
/// With nothing held, a saved generation request whose preparation stopped
/// still leads the page as it did before: that is a retired generation record,
/// not practice, and only `history-rich` holds one.
fn shown(name: &str, app: &App) -> (&'static str, Option<String>) {
    let Some(held) = &app.occurrence else {
        let stopped = app
            .goals
            .values()
            .any(|g| !g.archived && !g.paused && g.status == "failed");
        assert_eq!(stopped, name == "history-rich");
        return (
            if stopped {
                "preparation-stopped"
            } else {
                "earlier-empty"
            },
            None,
        );
    };
    if held.phase == "result" && held.result.is_some() {
        return ("earlier-result", None);
    }
    if held.answer.is_none() {
        return ("earlier-unanswered", None);
    }
    let status = held
        .assessment_id
        .as_ref()
        .map_or("none".to_string(), |id| app.assessments[id].status.clone());
    ("earlier-ungraded", Some(status))
}

/// No page serves the earlier notebook: `/notebook` shows the held question or
/// answer as it was left, and nothing anywhere answers, reveals, retries,
/// dismisses, polls or opens a gate.
#[test]
fn no_page_serves_an_earlier_question() {
    let mut views = BTreeMap::<&str, usize>::new();
    for (name, app, saved_ms) in stored() {
        let now = saved_ms + 100 * SECOND;
        let mut paths = vec![
            "/".to_string(),
            "/recall".into(),
            "/lessons".into(),
            "/library".into(),
            "/notebook".into(),
            "/map".into(),
            "/history".into(),
            "/settings".into(),
            "/gate".into(),
        ];
        paths.extend(app.goals.keys().map(|id| format!("/goals/{id}")));
        paths.extend(app.concepts.keys().map(|id| format!("/concepts/{id}")));
        paths.extend(
            app.questions
                .keys()
                .map(|id| format!("/questions/{id}/edit")),
        );
        for path in &paths {
            let html = page(&app, path, now);
            for control in [
                "action=\"/review/",
                "/practice\"",
                "data-choice",
                "data-poll",
                "Look it up",
                "href=\"/gate",
                "Practice this idea",
                "Next question",
                "Check my answer",
                "Show me the answer",
                "Retry native check",
                "Resume meaning checks →",
                "I know this already",
            ] {
                assert!(!html.contains(control), "{name} {path}: offers {control}");
            }
            for state in [
                "intro",
                "question",
                "checking",
                "self-check",
                "result",
                "gate",
                "checking-reference",
                "checking-paused",
                "caught-up",
            ] {
                assert!(
                    !html.contains(&format!("data-state=\"{state}\"")),
                    "{name} {path}: renders the practice state {state}"
                );
            }
        }
        assert!(page(&app, "/gate", now).contains("data-state=\"not-found\""));

        let view = page(&app, "/notebook", now);
        let (state, check) = shown(&name, &app);
        *views.entry(state).or_default() += 1;
        assert!(
            view.contains(&format!("data-state=\"{state}\"")),
            "{name}: /notebook shows {state}"
        );
        assert!(!view.contains("<textarea") && !view.contains("name=\"answer\""));
        assert_eq!(
            view.matches("<form").count(),
            0,
            "{name}: a form on /notebook"
        );
        let Some(held) = &app.occurrence else {
            assert!(view.contains("Earlier practice"), "{name}");
            assert!(!view.contains("Your next practice"), "{name}: a due claim");
            continue;
        };
        assert!(
            view.contains(&web::escape(&held.presentation.prompt)),
            "{name}: the question is shown"
        );
        for choice in &held.presentation.choices {
            assert!(
                state != "earlier-unanswered" || view.contains(&web::escape(choice)),
                "{name}: a choice that was presented is shown"
            );
        }
        match state {
            "earlier-unanswered" => {
                assert!(view.contains("<h2>Not answered.</h2>"), "{name}");
                assert_eq!(
                    view.contains("The reference had been opened."),
                    held.assisted,
                    "{name}"
                );
                assert!(!view.contains("answer-pair"), "{name}: shows an answer");
            }
            "earlier-ungraded" => {
                let check = check.unwrap();
                assert!(view.contains("Your answer is saved, not graded."), "{name}");
                assert!(view.contains(&format!("data-check=\"{check}\"")), "{name}");
                assert!(view.contains(&web::escape(held.answer.as_deref().unwrap())));
                // A fixed label for the stored status, never the stored message.
                for a in app.assessments.values() {
                    if let Some(error) = &a.error {
                        assert!(!view.contains(&web::escape(error)), "{name}: {error}");
                    }
                }
            }
            _ => {
                let result = held.result.as_ref().unwrap();
                let authority = if result.corrected {
                    "Your Correction"
                } else {
                    match result.authority.as_str() {
                        "exact" => "Exact match",
                        "jev" => "Meaning checked",
                        "reveal" => "Answer revealed",
                        other => panic!("{name}: authority {other}"),
                    }
                };
                assert!(view.contains(&format!("<p>{authority}</p>")), "{name}");
                assert!(view.contains(&web::escape(&held.presentation.answer)));
            }
        }
    }
    // The fixture exercises every view.
    assert_eq!(
        views,
        BTreeMap::from([
            ("earlier-empty", 2),
            ("earlier-result", 8),
            ("earlier-unanswered", 4),
            ("earlier-ungraded", 14),
            ("preparation-stopped", 1),
        ])
    );
}

/// A held earlier question blocks and masks nothing: history, the Goal, the
/// Concept and its saved answers, and Study's own screens.
#[test]
fn a_held_earlier_question_blocks_and_masks_nothing() {
    for (name, app, saved_ms) in stored() {
        let now = saved_ms + 100 * SECOND;
        assert!(study::history(&app).is_ok(), "{name}: history is refused");
        let history = page(&app, "/history", now);
        assert_eq!(
            history.matches("class=\"history-entry\"").count(),
            app.events.len(),
            "{name}: every earlier answer is in History"
        );
        assert!(!history.contains("hidden while you"), "{name}");
        // Study is not held back: the alphabet's welcome and its reference.
        let welcome = web::render(
            &app,
            "/",
            &BTreeMap::from([("practice".into(), "nato".into())]),
            now,
            None,
        );
        assert!(welcome.contains("data-state=\"nato-home\""), "{name}");
        assert!(welcome.contains("<span>Alfa</span>"), "{name}: specimen");
        assert!(welcome.contains("From Alfa to Zulu."), "{name}: welcome");
        assert!(
            welcome.contains("<summary>Alphabet reference</summary>"),
            "{name}: reference"
        );
        let Some(held) = &app.occurrence else {
            continue;
        };
        let question = &app.questions[&held.question_id];
        let concept = page(&app, &format!("/concepts/{}", held.concept_id), now);
        assert!(concept.contains("data-state=\"concept\""), "{name}");
        assert!(!concept.contains("Current idea"), "{name}: a masked name");
        if !question.archived {
            assert!(
                concept.contains(&web::escape(&question.content().answer)),
                "{name}: the saved answer is on its Concept"
            );
        }
        let goal = page(&app, &format!("/goals/{}", question.goal_id), now);
        assert!(goal.contains("data-state=\"goal\""), "{name}");
        assert!(
            goal.contains(&web::escape(&app.goals[&question.goal_id].title)),
            "{name}: the Goal's title"
        );
        let edit = page(&app, &format!("/questions/{}/edit", question.id), now);
        assert!(edit.contains("data-state=\"edit\""), "{name}");
        let map = page(&app, "/map", now);
        assert!(!map.contains("Current study"), "{name}: a masked Goal");
    }
}

/// A refused action is shown on the page it came from, and resuming returns
/// to Settings, where it was asked for.
#[test]
fn a_refusal_and_a_resume_land_where_they_were_asked() {
    let (app, _) = one("self-check-paused");
    for path in ["/review/answer", "/review/next", "/concepts/any/practice"] {
        let refusal = engine::admit_current_mutation(path).unwrap_err();
        assert_eq!(refusal.status, 410);
        assert_eq!(refusal.message, engine::EARLIER_NOTEBOOK_RETIRED);
        assert_eq!(web::mutation_error_path(&app, path), "/notebook");
    }
    assert_eq!(
        web::mutation_redirect(&app, "/settings/resume", "/"),
        "/settings"
    );
}
