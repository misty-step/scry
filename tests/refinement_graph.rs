use scry::{engine, generation, model::*};
use serde_json::json;
use std::collections::BTreeMap;

const NOW: i64 = 1_791_410_400_000;

fn config() -> engine::WorkConfig {
    engine::WorkConfig {
        generation: true,
        jev: true,
        critic: false,
        model: "synthetic-generator".into(),
        jev_model: "synthetic-jev".into(),
    }
}

fn refine(app: &mut App, goal: &str, operation: &str, batch: generation::Batch) {
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), operation.into()),
        (
            "feedback".into(),
            "Include useful conditional-request examples and their dependencies.".into(),
        ),
    ]);
    engine::mutate(
        app,
        &format!("/goals/{goal}/refine"),
        &fields,
        NOW,
        operation,
    )
    .unwrap();
    let work = engine::claim_work(app, NOW, &format!("{operation}-lease"), &config())
        .unwrap()
        .unwrap();
    let raw = json!({"choices":[{"message":{"content":serde_json::to_string(&batch).unwrap()}}]})
        .to_string();
    engine::finish_work(
        app,
        &work,
        raw,
        "synthetic-generator".into(),
        Some(20_000),
        NOW + 1,
    )
    .unwrap();
    assert!(
        engine::claim_work(app, NOW + 2, &format!("{operation}-publish"), &config())
            .unwrap()
            .is_none()
    );
}

fn candidate(key: &str, prerequisite: &str, prompt: &str) -> generation::Batch {
    serde_json::from_value(json!({"title":"Understanding HTTP caching","complete":true,"concepts":[{
        "key":key,"name":"Conditional request checks","summary":"A client can ask whether its stored representation is unchanged.",
        "note":"A conditional request sends the saved validator. For example, matching the current representation lets a server reply without sending its body again.",
        "basis":"general","quotes":[],"prerequisites":if prerequisite.is_empty(){vec![]}else{vec![prerequisite]},
        "questions":[{"prompt":prompt,"kind":"recall","answer":"If-None-Match","variants":[],"choices":[],
            "explanation":"The request header carries the saved ETag for comparison with the current representation.","basis":"general","quotes":[]}]
    }]})).unwrap()
}

#[test]
fn publication_links_unchanged_saved_ideas_and_updates_edges_without_rewriting_notes_or_questions()
{
    let mut app = App::new("refinement-graph-owner-csrf-32-characters".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    let goal = app.goals.values().next().unwrap().id.clone();
    let prerequisite = app
        .concepts
        .values()
        .find(|c| c.prerequisites.is_empty())
        .unwrap()
        .id
        .clone();
    let old_questions = app.questions.clone();
    let old_notes = app.concepts[&prerequisite].notes.clone();
    refine(
        &mut app,
        &goal,
        "graph-new-idea-00001",
        candidate(
            "new-request-check",
            &prerequisite,
            "Which request header carries a saved validator for comparison?",
        ),
    );
    let added = app
        .concepts
        .values()
        .find(|c| c.name == "Conditional request checks")
        .unwrap();
    assert_eq!(
        added.prerequisites.as_slice(),
        std::slice::from_ref(&prerequisite)
    );
    assert_eq!(app.concepts[&prerequisite].notes, old_notes);
    for (id, question) in &old_questions {
        assert_eq!(&app.questions[id], question);
    }
    let updated_id = added.id.clone();
    let old_note = added.notes[0].clone();
    refine(
        &mut app,
        &goal,
        "graph-update-idea-01",
        candidate(
            &updated_id,
            "",
            "Which conditional request header tests a saved representation identifier?",
        ),
    );
    assert!(app.concepts[&updated_id].prerequisites.is_empty());
    assert_eq!(app.concepts[&updated_id].notes[0], old_note);
    assert_eq!(app.concepts[&updated_id].notes.len(), 2);
    for (id, question) in &old_questions {
        assert_eq!(&app.questions[id], question);
    }
    engine::validate_app(&app).unwrap();
}

#[test]
fn lifecycle_failure_in_a_later_concept_cannot_partially_publish_earlier_material() {
    let mut app = App::new("graph-atomic-publication-csrf-32-characters".into());
    engine::seed_fixture(&mut app, NOW).unwrap();
    let goal = app.goals.keys().next().unwrap().clone();
    let prerequisite = app
        .concepts
        .values()
        .find(|c| c.prerequisites.is_empty())
        .unwrap()
        .id
        .clone();
    for (operation, path, extra) in [
        (
            "atomic-graph-archive01",
            format!("/concepts/{prerequisite}/archive"),
            None,
        ),
        (
            "atomic-graph-refine01",
            format!("/goals/{goal}/refine"),
            Some("Provide useful examples of conditional requests."),
        ),
    ] {
        let mut fields = BTreeMap::from([
            ("csrf".into(), app.csrf.clone()),
            ("operation_id".into(), operation.into()),
        ]);
        if let Some(text) = extra {
            fields.insert("feedback".into(), text.into());
        }
        engine::mutate(&mut app, &path, &fields, NOW, operation).unwrap();
    }
    let mut batch = candidate(
        "first-good-new",
        "",
        "Which request header carries a saved validator for comparison?",
    );
    batch.concepts.push(
        candidate(
            "later-bad-new",
            &prerequisite,
            "Which conditional header compares a validator from an archived idea?",
        )
        .concepts
        .remove(0),
    );
    let work = engine::claim_work(&mut app, NOW, "atomic-refine-generation-lease", &config())
        .unwrap()
        .unwrap();
    let raw = json!({"choices":[{"message":{"content":serde_json::to_string(&batch).unwrap()}}]})
        .to_string();
    engine::finish_work(
        &mut app,
        &work,
        raw,
        "synthetic-generator".into(),
        Some(20_000),
        NOW + 1,
    )
    .unwrap();
    let old_concepts = app.concepts.clone();
    let old_questions = app.questions.clone();
    let old_ids = app.goals[&goal].concept_ids.clone();
    assert!(
        engine::claim_work(&mut app, NOW + 2, "unused-atomic-publish-lease", &config())
            .unwrap()
            .is_none()
    );
    assert_eq!(app.concepts, old_concepts);
    assert_eq!(app.questions, old_questions);
    assert_eq!(app.goals[&goal].concept_ids, old_ids);
    assert_eq!(app.jobs[&work.id].status, "failed");
    assert!(
        app.jobs[&work.id]
            .error
            .as_ref()
            .unwrap()
            .contains("archived idea")
    );
    engine::validate_app(&app).unwrap();
}
