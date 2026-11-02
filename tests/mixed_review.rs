use scry::{engine, library, model::*, persistence, study, web};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
struct Owner {
    app: App,
    n: u32,
}
impl Owner {
    fn new() -> Self {
        Self {
            app: App::new("mixed-review-synthetic-csrf-123456".into()),
            n: 0,
        }
    }
    fn call(&mut self, path: &str, pairs: &[(&str, String)], now: i64) -> MutationResult {
        self.n += 1;
        let id = format!("mixed-operation-{:06}", self.n);
        let mut f = BTreeMap::from([
            ("csrf".into(), self.app.csrf.clone()),
            ("revision".into(), self.app.revision.to_string()),
            ("operation_id".into(), id.clone()),
        ]);
        f.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.clone())));
        let r = engine::mutate(&mut self.app, path, &f, now, &id).unwrap();
        engine::validate_app(&self.app).unwrap();
        r
    }
    fn mapping(&mut self, title: &str, key: &str, now: i64) -> String {
        let mut input = library::nato_input().unwrap();
        input.title = title.into();
        input.text = format!("{key}\tValue\n");
        input.sections.as_mut().unwrap()[0].units = vec![input.text.clone()];
        let vid = self.app.library.add(input, title, None).unwrap();
        self.call("/practice/learn", &[("version_id", vid)], now)
            .entity_id
            .unwrap()
    }
    fn prompt(&self) -> study::Prompt {
        self.app.practice.current.as_ref().unwrap().prompt.clone()
    }
    fn answer(&mut self, answer: &str, now: i64) {
        if self.app.practice.current.as_ref().unwrap().phase == Some(study::StudyPhase::Study) {
            self.call("/practice/quiz", &[("prompt_id", self.prompt().id)], now);
        }
        self.call(
            "/practice/answer",
            &[("prompt_id", self.prompt().id), ("answer", answer.into())],
            now,
        );
    }
    fn focus(&mut self, sid: &str, now: i64) {
        let mut f = vec![("study_id", sid.into())];
        if let Some(p) = &self.app.practice.current {
            f.push(("after", p.prompt.id.clone()));
        }
        self.call("/practice/next", &f, now);
    }
    fn mixed(&mut self, now: i64) {
        let mut f = vec![];
        if let Some(p) = &self.app.practice.current {
            f.push(("after", p.prompt.id.clone()));
        }
        self.call("/practice/review/start", &f, now);
    }
}
#[test]
fn same_day_due_advances_but_early_across_midnight_does_not() {
    let mut o = Owner::new();
    let sid = o.mapping("One", "Q", NOW);
    o.focus(&sid, NOW);
    o.answer("Value", NOW);
    let cid = o.app.practice.cards.keys().next().unwrap().clone();
    let before = o.app.practice.cards[&cid].state.schedule.clone();
    assert_eq!(before.due_ms, NOW + 600_000);
    o.focus(&sid, NOW + 600_000);
    o.answer("Value", NOW + 600_000);
    let a = o
        .app
        .practice
        .attempts
        .values()
        .max_by_key(|a| a.sequence)
        .unwrap();
    assert!(a.effects[0].scheduled);
    assert!(!a.effects[0].extra_practice);
    assert!(o.app.practice.cards[&cid].state.schedule.due_ms > NOW + 600_000);
    let saved = o.app.practice.cards[&cid].state.schedule.clone();
    let tomorrow = (NOW / DAY_MS + 1) * DAY_MS;
    assert!(tomorrow < saved.due_ms);
    let p = o.prompt();
    o.call(
        "/practice/next",
        &[("after", p.id), ("card_id", cid.clone())],
        tomorrow,
    );
    o.answer("Value", tomorrow);
    assert_eq!(o.app.practice.cards[&cid].state.schedule, saved);
    assert!(
        o.app
            .practice
            .attempts
            .values()
            .max_by_key(|a| a.sequence)
            .unwrap()
            .effects[0]
            .extra_practice
    );
}
#[test]
fn mixed_rotates_subjects_due_only_without_import_or_new_units() {
    let mut o = Owner::new();
    let a = o.mapping("First", "A", NOW);
    let b = o.mapping("Second", "B", NOW);
    let untouched = o.mapping("Unstarted", "C", NOW);
    o.focus(&a, NOW);
    o.answer("wrong", NOW);
    o.focus(&b, NOW + 1);
    o.answer("wrong", NOW + 1);
    let old = o.app.practice.attempts.clone();
    o.mixed(NOW + 70_000);
    assert_eq!(o.prompt().study_id, a);
    assert!(o.app.practice.mixed_review);
    assert_eq!(o.app.practice.attempts, old);
    o.answer("wrong", NOW + 70_000);
    let p = o.prompt();
    o.call("/practice/review/next", &[("after", p.id)], NOW + 140_000);
    assert_eq!(o.prompt().study_id, b);
    o.answer("Value", NOW + 140_000);
    o.mixed(NOW + 141_000);
    assert_eq!(o.prompt().study_id, a);
    assert!(
        !o.app
            .practice
            .cards
            .values()
            .any(|c| c.study_id == untouched)
    );
    let held = o.app.practice.current.clone();
    let before = o.app.clone();
    let html = web::render(&o.app, "/", &BTreeMap::new(), NOW + 141_000, None);
    assert!(!html.contains("Value"));
    assert_eq!(o.app, before);
    study::ensure_stream(&mut o.app, NOW + 142_000, "refresh").unwrap();
    assert_eq!(held, o.app.practice.current);
    let archive = persistence::Archive::new(o.app.clone(), vec![], NOW + 142_000);
    let bytes = archive.encode().unwrap();
    let decoded = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(decoded.app.practice, o.app.practice);
}
#[test]
fn continuous_study_introduces_unseen_when_reviews_are_future_and_resumes_safely() {
    let mut o = Owner::new();
    let a = o.mapping("Only", "A", NOW);
    o.focus(&a, NOW);
    let p = o.prompt();
    o.call("/practice/review/start", &[], NOW);
    assert_eq!(o.prompt(), p);
    assert!(o.app.practice.mixed_review);
    o.answer("Value", NOW);
    o.mixed(NOW + 1);
    assert_ne!(o.prompt().study_id, a);
    assert_eq!(o.prompt().cue, "A");
    let held = o.prompt();
    let before = o.app.clone();
    let html = web::render(&o.app, "/", &BTreeMap::new(), NOW + 1, None);
    assert!(!html.contains("No reviews due"));
    assert_eq!(before, o.app);
    o.call("/practice/review/start", &[], NOW + 600_000);
    assert_eq!(o.prompt(), held);
    o.answer("Alfa", NOW + 600_000);
    o.mixed(NOW + 600_001);
    assert_eq!(o.prompt().study_id, a);
}

// Reconstruct the old, valid on-disk policy without using new scheduling code.
// Only the latest effect differs: old daily gating retained its before schedule.
fn legacy_snapshot(o: &mut Owner) -> serde_json::Value {
    for a in o.app.practice.attempts.values_mut() {
        a.schedule_policy = study::SchedulePolicy::CalendarDay;
        a.algorithm = scry::learning::ALGORITHM.into();
        for e in &mut a.effects {
            if !e.assisted
                && e.before
                    .schedule
                    .last_review_ms
                    .is_some_and(|t| t / DAY_MS == a.created_ms / DAY_MS)
            {
                e.extra_practice = true;
                e.scheduled = false;
                e.after.schedule = e.before.schedule.clone();
                o.app.practice.cards.get_mut(&e.card_id).unwrap().state = e.after.clone();
            }
        }
    }
    engine::validate_app(&o.app).unwrap();
    let json = serde_json::to_value(&o.app).unwrap();
    assert!(
        json["practice"]["attempts"]
            .as_object()
            .unwrap()
            .values()
            .all(|a| a.get("schedule_policy").is_none())
    );
    o.app = serde_json::from_value(json.clone()).unwrap();
    engine::validate_app(&o.app).unwrap();
    json
}
#[test]
fn legacy_missing_policy_keeps_old_due_extra_and_new_due_policy_survives_archive() {
    let mut o = Owner::new();
    let sid = o.mapping("Legacy", "L", NOW);
    o.focus(&sid, NOW);
    o.answer("wrong", NOW);
    o.focus(&sid, NOW + 60_000);
    o.answer("Value", NOW + 60_000);
    let snapshot = legacy_snapshot(&mut o);
    assert_eq!(
        persistence::Archive::new(o.app.clone(), vec![], NOW).format,
        "scry-rust-durable-sqlite-study-v1"
    );
    let old = snapshot["practice"]["attempts"].clone();
    let second = o
        .app
        .practice
        .attempts
        .values()
        .max_by_key(|a| a.sequence)
        .unwrap();
    assert!(second.effects[0].extra_practice);
    assert_eq!(second.effects[0].after.schedule.due_ms, NOW + 60_000);
    o.mixed(NOW + 61_000);
    o.answer("Value", NOW + 61_000);
    let latest = o
        .app
        .practice
        .attempts
        .values()
        .max_by_key(|a| a.sequence)
        .unwrap();
    assert_eq!(latest.schedule_policy, study::SchedulePolicy::Due);
    assert!(latest.effects[0].scheduled);
    let json = serde_json::to_value(&o.app).unwrap();
    for (id, a) in old.as_object().unwrap() {
        assert_eq!(&json["practice"]["attempts"][id], a);
    }
    o.app = serde_json::from_value(json).unwrap();
    engine::validate_app(&o.app).unwrap();
}
#[test]
fn paused_due_survives_but_dropped_does_not() {
    let mut o = Owner::new();
    let sid = o.mapping("Ready", "A", NOW);
    o.focus(&sid, NOW);
    o.answer("wrong", NOW);
    o.app.practice.studies.get_mut(&sid).unwrap().status = study::Status::Paused;
    assert_eq!(study::mixed_cards(&o.app, NOW + 60_000).len(), 1);
    o.app.practice.studies.get_mut(&sid).unwrap().status = study::Status::Dropped;
    assert!(study::mixed_cards(&o.app, NOW + 60_000).is_empty());
}

const ROSARY: &str = "Annunciation\nVisitation\nNativity\nPresentation\nFinding in the Temple";
#[test]
fn mixed_sequence_stays_whole_and_only_due_components_advance() {
    let mut o = Owner::new();
    o.call(
        "/practice/lesson/start",
        &[("starter", "rosary-joyful".into())],
        NOW,
    );
    o.answer("Annunciation\nVisitation\nNativity\nPresentation", NOW);
    let before = o.app.practice.cards.clone();
    o.mixed(NOW + 60_000);
    assert!(matches!(o.prompt().scope, study::Scope::Whole));
    assert_eq!(o.prompt().unit_ids.len(), 5);
    o.answer(ROSARY, NOW + 60_000);
    let a = o
        .app
        .practice
        .attempts
        .values()
        .max_by_key(|a| a.sequence)
        .unwrap();
    assert_eq!(a.effects.iter().filter(|e| e.scheduled).count(), 1);
    for e in &a.effects {
        if e.extra_practice {
            assert_eq!(e.after.schedule, before[&e.card_id].state.schedule);
        }
    }
    assert!(
        web::render(&o.app, "/", &BTreeMap::new(), NOW + 60_000, None)
            .contains("action=\"/practice/review/next\"")
    );
}
#[test]
fn legacy_correction_replays_original_policy_not_new_due_policy() {
    let mut o = Owner::new();
    o.call(
        "/practice/lesson/start",
        &[("starter", "rosary-joyful".into())],
        NOW,
    );
    o.answer(ROSARY, NOW);
    let p = o.prompt();
    o.call(
        "/practice/lesson/next",
        &[("after", p.id), ("mode", "whole".into())],
        NOW + 600_000,
    );
    o.answer(
        "Annunciation\nVisitation\nNativity\nPresentation",
        NOW + 600_000,
    );
    let snapshot = legacy_snapshot(&mut o);
    let a = o
        .app
        .practice
        .attempts
        .values()
        .max_by_key(|a| a.sequence)
        .unwrap()
        .clone();
    let e = a
        .effects
        .iter()
        .find(|e| e.verdict == scry::grading::Verdict::Again)
        .unwrap();
    o.call(
        "/practice/correct",
        &[
            ("prompt_id", a.prompt.id.clone()),
            ("attempt_id", a.id.clone()),
            ("unit_id", e.unit_id.clone()),
            ("verdict", "Good".into()),
        ],
        NOW + 601_000,
    );
    assert_eq!(
        o.app.practice.cards[&e.card_id].state.schedule,
        e.before.schedule
    );
    let json = serde_json::to_value(&o.app).unwrap();
    assert_eq!(
        json["practice"]["attempts"],
        snapshot["practice"]["attempts"]
    );
    o.app = serde_json::from_value(json).unwrap();
    engine::validate_app(&o.app).unwrap();
}
#[test]
fn due_reviews_exclude_guided_but_continuous_study_keeps_teaching() {
    let mut o = Owner::new();
    // Genuine retained generic/API guided practice remains readable; the app
    // must present its next turn as a hidden memory quiz, not another copy task.
    let vid = o
        .app
        .library
        .add(
            scry::lessons::input("st-michael-en-lasance-1911").unwrap(),
            "legacy-guided",
            None,
        )
        .unwrap();
    let sid = o
        .call("/practice/learn", &[("version_id", vid)], NOW)
        .entity_id
        .unwrap();
    o.focus(&sid, NOW);
    assert_eq!(o.app.practice.cards.len(), 1);
    assert!(study::mixed_cards(&o.app, NOW).is_empty());
    let p = o.prompt();
    let text = o.app.library.versions[&p.version_id].sections[0].units[0]
        .text
        .clone();
    o.answer(&text, NOW);
    assert!(study::mixed_cards(&o.app, NOW).is_empty());
    o.mixed(NOW + 1);
    assert!(o.app.practice.current.is_some());
    // An unstarted subject receives a turn before the already-served prayer.
    assert_eq!(o.prompt().cue, "A");
    let prayer_sid = p.study_id;
    for i in 0..10 {
        let p = o.prompt();
        if p.study_id == prayer_sid {
            break;
        }
        let version = &o.app.library.versions[&p.version_id];
        let answer = if version.kind == library::Kind::KeyedMapping {
            library::mapping_pair(&version.sections[0].units[0].text)
                .unwrap()
                .1
                .to_string()
        } else {
            version.text.clone()
        };
        o.answer(&answer, NOW + 2 + i);
        o.mixed(NOW + 2 + i);
    }
    assert_eq!(o.prompt().study_id, prayer_sid);
    assert!(!o.prompt().assisted);
    assert_eq!(
        o.app.practice.current.as_ref().unwrap().phase,
        Some(study::StudyPhase::Quiz)
    );
    assert_eq!(o.prompt().unit_ids, p.unit_ids);
}

#[test]
fn mixed_next_replays_original_receipt_and_rejects_overrides_without_mutation() {
    let mut o = Owner::new();
    let sid = o.mapping("Only", "A", NOW);
    o.focus(&sid, NOW);
    o.answer("wrong", NOW);
    o.mixed(NOW + 60_000);
    o.answer("Value", NOW + 60_000);
    let fields = BTreeMap::from([
        ("csrf".into(), o.app.csrf.clone()),
        ("revision".into(), o.app.revision.to_string()),
        (
            "operation_id".into(),
            "mixed-duplicate-next-operation".into(),
        ),
        ("after".into(), o.prompt().id),
    ]);
    let first = engine::mutate(
        &mut o.app,
        "/practice/review/next",
        &fields,
        NOW + 60_001,
        "next",
    )
    .unwrap();
    let saved = o.app.clone();
    assert_eq!(
        engine::mutate(
            &mut o.app,
            "/practice/review/next",
            &fields,
            NOW + 70_000,
            "retry"
        )
        .unwrap(),
        first
    );
    assert_eq!(o.app, saved);
    assert!(o.app.practice.current.is_some());
    assert_eq!(o.app.practice.studies.len(), 2);
    for (key, value) in [
        ("grade", "Good"),
        ("study_id", sid.as_str()),
        ("due_ms", "0"),
    ] {
        let f = BTreeMap::from([
            ("csrf".into(), o.app.csrf.clone()),
            ("revision".into(), o.app.revision.to_string()),
            ("operation_id".into(), format!("mixed-override-{key}")),
            (key.into(), value.into()),
        ]);
        assert_eq!(
            engine::mutate(
                &mut o.app,
                "/practice/review/start",
                &f,
                NOW + 70_000,
                "override"
            )
            .unwrap_err()
            .status,
            422
        );
        assert_eq!(o.app, saved);
    }
    assert_eq!(
        web::mutation_redirect(&o.app, "/practice/review/next", "/recall"),
        "/"
    );
    let page = web::render(&o.app, "/recall", &BTreeMap::new(), NOW + 70_000, None);
    assert!(page.contains("/practice/answer"));
    assert!(page.contains(&format!("name=\"prompt_id\" value=\"{}\"", o.prompt().id)));
    engine::validate_app(&o.app).unwrap();
}

#[test]
fn all_subjects_and_focused_entry_preserve_held_until_explicit_fenced_switch() {
    let mut o = Owner::new();
    let sid = o.mapping("External", "A", NOW);
    o.focus(&sid, NOW);
    let held = o.app.practice.current.clone();
    o.call("/practice/review/start", &[], NOW);
    assert_eq!(o.app.practice.current, held);
    o.call("/practice/next", &[("study_id", sid)], NOW);
    assert!(!o.app.practice.mixed_review);
    assert_eq!(o.app.practice.current, held);
    let p = o.prompt();
    o.call("/practice/review/start", &[("leave", p.id)], NOW);
    assert!(o.app.practice.current.is_some());
    assert!(o.app.practice.attempts.is_empty());
    o.call(
        "/practice/lesson/start",
        &[("starter", "nato".into()), ("leave", o.prompt().id)],
        NOW,
    );
    assert!(!o.app.practice.mixed_review);
    assert_eq!(o.prompt().cue, "A");
    assert!(o.app.practice.attempts.is_empty());
}

#[test]
fn new_policy_and_mixed_receipts_fence_archives_and_reject_tampered_pairs() {
    let mut o = Owner::new();
    assert_eq!(
        persistence::Archive::new(o.app.clone(), vec![], NOW).format,
        persistence::ARCHIVE_FORMAT
    );
    o.call("/practice/review/start", &[], NOW);
    let persisted = persistence::app_from_rows(&persistence::rows(&o.app).unwrap()).unwrap();
    assert!(persisted.practice.mixed_review);
    assert_eq!(persisted, o.app);
    let format = "scry-rust-durable-sqlite-study-phase-v3";
    assert_eq!(
        persistence::Archive::new(o.app.clone(), vec![], NOW).format,
        format
    );
    o.call("/practice/lesson/start", &[("starter", "nato".into())], NOW);
    assert!(!o.app.practice.mixed_review);
    assert!(o.app.practice.attempts.is_empty());
    assert_eq!(
        persistence::Archive::new(o.app.clone(), vec![], NOW).format,
        format,
        "Retained mixed receipt requires its reader after focused switch"
    );
    o.answer("Alfa", NOW);
    let a = o.app.practice.attempts.values().next().unwrap();
    assert_eq!(
        a.algorithm,
        format!("{};schedule=due-v1", scry::learning::ALGORITHM)
    );
    let mut archive = persistence::Archive::new(o.app.clone(), vec![], NOW);
    archive.validate().unwrap();
    archive.format = "scry-rust-durable-sqlite-study-scheduled-v2".into();
    assert_eq!(archive.validate().unwrap_err().status, 422);
    archive.format = "scry-rust-durable-sqlite-study-v1".into();
    assert_eq!(archive.validate().unwrap_err().status, 422);
    let mut stripped = o.app.clone();
    stripped
        .practice
        .attempts
        .values_mut()
        .next()
        .unwrap()
        .schedule_policy = study::SchedulePolicy::CalendarDay;
    assert!(
        engine::validate_app(&stripped).is_err(),
        "Unknown algorithm/policy pair refuses before writes"
    );
    let mut old_algorithm = o.app.clone();
    old_algorithm
        .practice
        .attempts
        .values_mut()
        .next()
        .unwrap()
        .algorithm = scry::learning::ALGORITHM.into();
    assert!(engine::validate_app(&old_algorithm).is_err());
    let mut unknown = serde_json::to_value(&o.app).unwrap();
    unknown["practice"]["attempts"]
        .as_object_mut()
        .unwrap()
        .values_mut()
        .next()
        .unwrap()["schedule_policy"] = "unknown".into();
    assert!(serde_json::from_value::<App>(unknown).is_err());
}

fn park_catalog(o: &mut Owner) {
    for id in [
        "nato",
        "st-michael-en",
        "rosary-joyful",
        "rosary-luminous",
        "rosary-sorrowful",
        "rosary-glorious",
        "baby-soothing",
        "baby-everyday",
        "baby-montessori",
    ] {
        let vid = o
            .app
            .library
            .add(scry::lessons::input(id).unwrap(), id, None)
            .unwrap();
        let sid = o
            .call("/practice/learn", &[("version_id", vid)], NOW)
            .entity_id
            .unwrap();
        o.call("/practice/pause", &[("study_id", sid)], NOW);
    }
}

#[test]
fn empty_mixed_get_is_read_only_and_explicit_study_admits_one_subject() {
    let mut o = Owner::new();
    o.app.practice.mixed_review = true;
    let before = o.app.clone();
    assert!(study::continuous_preview(&o.app, NOW).is_some());
    study::ensure_stream(&mut o.app, NOW, "read").unwrap();
    assert_eq!(o.app, before);
    o.mixed(NOW);
    assert_eq!(o.app.practice.studies.len(), 1);
    assert_eq!(o.app.practice.cards.len(), 1);
    assert_eq!(o.prompt().cue, "A");
    let saved = o.app.clone();
    study::ensure_stream(&mut o.app, NOW, "read-held").unwrap();
    assert_eq!(o.app, saved);
}

#[test]
fn paused_catalog_is_not_reenrolled_or_reactivated_and_rest_is_honest() {
    let mut o = Owner::new();
    park_catalog(&mut o);
    let old = o.app.practice.studies.clone();
    assert!(study::continuous_preview(&o.app, NOW).is_none());
    o.mixed(NOW);
    assert!(o.app.practice.current.is_none());
    assert_eq!(o.app.practice.studies, old);
    assert!(o.app.practice.cards.is_empty());
    let page = web::render(&o.app, "/", &BTreeMap::new(), NOW, None);
    assert!(page.contains("Rest for now"));
    assert!(!page.contains("No reviews due"));
}

#[test]
fn continuous_custom_library_uses_current_version_and_keeps_pace_without_bypass() {
    let mut o = Owner::new();
    park_catalog(&mut o);
    let mut input = library::nato_input().unwrap();
    input.title = "Custom external subject".into();
    input.text = "Legacy\tOld wording".into();
    input.sections.as_mut().unwrap()[0].units = vec![input.text.clone()];
    let old = o
        .app
        .library
        .add(input.clone(), "custom-old", None)
        .unwrap();
    input.name = "Current supplied edition".into();
    input.text =
        "Q\tQuail\nR\tRobin\nS\tSwan\nT\tTern\nU\tUmbrellabird\nV\tVulture\nW\tWren".into();
    input.sections.as_mut().unwrap()[0].units = input
        .text
        .split_inclusive('\n')
        .map(str::to_owned)
        .collect();
    let new = o.app.library.add(input, "custom-new", Some(&old)).unwrap();
    o.mixed(NOW);
    assert_eq!(o.prompt().version_id, new);
    let sid = o.prompt().study_id;
    for i in 0..6 {
        let p = o.prompt();
        assert_eq!(p.version_id, new);
        let v = &o.app.library.versions[&new];
        let unit = v
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .find(|u| u.id == p.unit_ids[0])
            .unwrap();
        let answer = library::mapping_pair(&unit.text).unwrap().1.to_string();
        o.answer(&answer, NOW + i);
        o.mixed(NOW + i);
    }
    assert!(o.app.practice.current.is_none());
    assert_eq!(
        o.app
            .practice
            .cards
            .values()
            .filter(|c| c.study_id == sid)
            .count(),
        6
    );
    assert_eq!(
        o.app
            .practice
            .studies
            .values()
            .filter(|s| s.version_id == old)
            .count(),
        0
    );
    assert!(study::continuous_preview(&o.app, NOW + 10).is_none());
}

#[test]
fn imported_historical_prayer_does_not_become_default_but_selected_alternative_is_respected() {
    let mut o = Owner::new();
    let old = o
        .app
        .library
        .add(
            scry::lessons::input("st-michael-en-lasance-1911").unwrap(),
            "historic",
            None,
        )
        .unwrap();
    o.mixed(NOW);
    o.answer("Alfa", NOW);
    o.mixed(NOW);
    let p = o.prompt();
    assert_ne!(p.version_id, old);
    assert!(
        scry::lessons::catalog()
            .iter()
            .find(|l| l.id == "st-michael-en")
            .unwrap()
            .matches(&o.app.library.versions[&p.version_id])
    );
    let mut o = Owner::new();
    let old = o
        .app
        .library
        .add(
            scry::lessons::input("st-michael-en-lasance-1911").unwrap(),
            "historic",
            None,
        )
        .unwrap();
    let sid = o
        .call("/practice/learn", &[("version_id", old.clone())], NOW)
        .entity_id
        .unwrap();
    o.call("/practice/drop", &[("study_id", sid)], NOW);
    for _ in 0..9 {
        o.mixed(NOW);
        let p = o.prompt();
        assert_ne!(p.version_id, old);
        let v = &o.app.library.versions[&p.version_id];
        assert!(
            !scry::lessons::catalog()
                .iter()
                .any(|l| l.id == "st-michael-en" && l.matches(v))
        );
        let answer = if v.kind == library::Kind::KeyedMapping {
            library::mapping_pair(&v.sections[0].units[0].text)
                .unwrap()
                .1
                .to_string()
        } else {
            v.text.clone()
        };
        o.answer(&answer, NOW);
    }
}

#[test]
fn oversized_authored_text_is_preserved_without_blocking_other_study() {
    let mut o = Owner::new();
    let mut input = scry::lessons::input("st-michael-en").unwrap();
    input.title = "Large external text".into();
    input.text = "word ".repeat(2000);
    input.sections.as_mut().unwrap().truncate(1);
    input.sections.as_mut().unwrap()[0].units = vec![input.text.clone()];
    let vid = o.app.library.add(input, "oversize", None).unwrap();
    let before = o.app.library.versions[&vid].clone();
    o.mixed(NOW);
    assert_eq!(o.prompt().cue, "A");
    assert_eq!(o.app.library.versions[&vid], before);
    assert!(!o.app.practice.studies.values().any(|s| s.version_id == vid));
}

#[test]
fn paused_custom_wording_cannot_be_bypassed_by_canonical_starter() {
    let mut o = Owner::new();
    let mut input = scry::lessons::input("st-michael-en").unwrap();
    input.name = "Custom supplied wording".into();
    input.text = "A privately supplied different sentence.".into();
    input.sections.as_mut().unwrap().truncate(1);
    input.sections.as_mut().unwrap()[0].units = vec![input.text.clone()];
    let vid = o.app.library.add(input, "custom-prayer", None).unwrap();
    let sid = o
        .call("/practice/learn", &[("version_id", vid.clone())], NOW)
        .entity_id
        .unwrap();
    o.call("/practice/pause", &[("study_id", sid.clone())], NOW);
    o.mixed(NOW);
    o.answer("Alfa", NOW);
    o.mixed(NOW);
    assert!(matches!(o.prompt().scope, study::Scope::Whole));
    assert_eq!(o.app.practice.studies[&sid].status, study::Status::Paused);
    let work = &o.app.library.versions[&vid].work_id;
    assert_eq!(
        o.app
            .library
            .versions
            .values()
            .filter(|v| &v.work_id == work)
            .count(),
        1
    );
}

#[test]
fn safe_existing_due_phrase_is_not_hidden_by_an_oversized_unseen_unit() {
    let mut o = Owner::new();
    let mut input = scry::lessons::input("st-michael-en").unwrap();
    input.title = "Mixed-size external text".into();
    let first = "A short phrase.\n";
    let large = "word ".repeat(2000);
    input.text = format!("{first}{large}");
    input.sections.as_mut().unwrap().truncate(1);
    input.sections.as_mut().unwrap()[0].units = vec![first.into(), large];
    let vid = o.app.library.add(input, "mixed-size", None).unwrap();
    let sid = o
        .call("/practice/learn", &[("version_id", vid)], NOW)
        .entity_id
        .unwrap();
    for i in 0..7 {
        o.focus(&sid, NOW + i);
        o.answer(first, NOW + i);
    }
    assert!(
        o.app
            .practice
            .cards
            .values()
            .next()
            .unwrap()
            .state
            .schedule
            .last_review_ms
            .is_some()
    );
    o.mixed(NOW + 600_010);
    assert_eq!(o.prompt().study_id, sid);
    assert_eq!(o.prompt().unit_ids.len(), 1);
    assert_eq!(
        o.app
            .practice
            .cards
            .values()
            .filter(|c| c.study_id == sid)
            .count(),
        1
    );
}
