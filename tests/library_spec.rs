//! SPEC § Library through the engine every client shares: duplicates return
//! the existing Version, corrections always create one, Provenance is recorded,
//! manifest Relations appear on both Works, and the stored shape a notebook
//! takes only when it uses one of these.
use scry::{
    api, engine,
    library::Relation,
    model::*,
    owner_token::{MutationActor, OwnerToken},
    persistence, web,
};
use serde_json::{Value, json};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;
const PLAIN_FORMAT: &str = "scry-rust-durable-sqlite-library-v1";
const NEW_FORMAT: &str = "scry-rust-durable-sqlite-library-v5";
const MARKER: &str = "library_compatibility/v1";

fn app() -> App {
    let mut app = App::new("synthetic-csrf-at-least-24-chars".into());
    let token = OwnerToken::issue(
        "Library spec agent",
        None,
        false,
        NOW,
        "0123456789abcdef0123456789abcdef0123456789abcdef",
    )
    .unwrap();
    app.owner_tokens.insert(token.info.id.clone(), token);
    app
}
fn call(app: &mut App, route: &str, op: &str, mut payload: Value) -> AppResult<MutationResult> {
    payload["operation_id"] = json!(op);
    let fields = if route.starts_with("/api/v1/library/") {
        api::library_fields(payload)?
    } else {
        api::fields(payload)?
    };
    let token = app.owner_tokens.values().next().unwrap().info.clone();
    let path = api::mutation_path(route).unwrap();
    let result = engine::mutate_as(app, path, &fields, NOW, op, MutationActor::Token(&token))?;
    engine::validate_app(app).unwrap();
    Ok(result)
}
fn add(app: &mut App, op: &str, data: Value) -> AppResult<String> {
    Ok(
        call(app, "/api/v1/library/versions", op, json!({"data":data}))?
            .entity_id
            .unwrap(),
    )
}
fn correct(app: &mut App, op: &str, old: &str, data: Value) -> AppResult<String> {
    let work = &app.library.versions[old].work_id;
    let revision = app.library.works[work].revision;
    Ok(call(
        app,
        "/api/v1/library/versions/supersede",
        op,
        json!({"version_id":old,"revision":revision,"data":data}),
    )?
    .entity_id
    .unwrap())
}
fn text(title: &str, words: &str) -> Value {
    json!({"title":title,"kind":"text","text":words,"name":"Synthetic edition"})
}
fn stored(app: &App, id: &str) -> Value {
    serde_json::to_value(&app.library.versions[id]).unwrap()
}
fn archive(app: &App) -> persistence::Archive {
    persistence::Archive::new(app.clone(), vec![], NOW)
}
fn page(app: &App, path: &str) -> String {
    web::render(app, path, &BTreeMap::new(), NOW, None)
}
fn work_of(app: &App, version: &str) -> String {
    app.library.versions[version].work_id.clone()
}

#[test]
fn a_duplicate_returns_the_existing_version_whatever_work_its_names_point_to() {
    let mut app = app();
    let first = add(
        &mut app,
        "spec-dup-first-op",
        text("First Work", "Shared synthetic words, twice.\n"),
    )
    .unwrap();
    let second = add(
        &mut app,
        "spec-dup-second-op",
        text("Second Work", "Other synthetic words.\n"),
    )
    .unwrap();
    let before = app.library.clone();
    // Under another Work's title, with case, punctuation and spacing changed.
    let again = add(
        &mut app,
        "spec-dup-other-title-op",
        text("Second Work", "shared SYNTHETIC words twice"),
    )
    .unwrap();
    assert_eq!(again, first);
    assert_eq!(app.library, before, "Nothing is created or attached.");
    // Under names that point at two different Works.
    let mut ambiguous = text("First Work", "Shared synthetic words twice!");
    ambiguous["aliases"] = json!(["Second Work"]);
    assert_eq!(
        add(&mut app, "spec-dup-ambiguous-op", ambiguous.clone()).unwrap(),
        first
    );
    assert_eq!(app.library, before);
    // Replay returns the committed result; reuse with another payload conflicts.
    let snapshot = app.clone();
    assert_eq!(
        add(
            &mut app,
            "spec-dup-other-title-op",
            text("Second Work", "shared SYNTHETIC words twice")
        )
        .unwrap(),
        first
    );
    assert_eq!(app, snapshot, "Replay changes nothing.");
    let reuse = add(
        &mut app,
        "spec-dup-other-title-op",
        text("Second Work", "Different words."),
    )
    .unwrap_err();
    assert_eq!(reuse.status, 409);
    assert_eq!(app, snapshot);
    // A new text under ambiguous names is still refused, with nothing stored.
    ambiguous["text"] = json!("Words in no Version yet.\n");
    assert_eq!(
        add(&mut app, "spec-dup-ambiguous-new-op", ambiguous)
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(app.library, before);
    assert_ne!(first, second);
}

#[test]
fn a_correction_always_creates_a_new_version_and_keeps_the_old_one_and_its_attempts() {
    let mut app = app();
    let old = add(
        &mut app,
        "spec-correct-original-op",
        text("Corrected Work", "Synthetic first clause, second clause.\n"),
    )
    .unwrap();
    // A real Study and Attempt against the old Version.
    let study = call(
        &mut app,
        "/api/v1/learn",
        "spec-correct-learn-op",
        json!({"version_id":old,"pace":"steady"}),
    )
    .unwrap();
    let study = study.entity_id.unwrap();
    call(
        &mut app,
        "/api/v1/next",
        "spec-correct-next-op",
        json!({"study_id":study}),
    )
    .unwrap();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    call(
        &mut app,
        "/api/v1/quiz",
        "spec-correct-quiz-op",
        json!({"prompt_id":prompt}),
    )
    .unwrap();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    call(
        &mut app,
        "/api/v1/answer",
        "spec-correct-answer-op",
        json!({"prompt_id":prompt,"answer":"Synthetic first clause,"}),
    )
    .unwrap();
    let original = app.library.versions[&old].clone();
    let attempts = app.practice.attempts.clone();
    assert_eq!(attempts.len(), 1);
    let studies = app.practice.studies.clone();
    // Only case, punctuation and spacing change.
    let new = correct(
        &mut app,
        "spec-correct-case-op",
        &old,
        text(
            "Corrected Work",
            "synthetic FIRST clause;  second clause!\n",
        ),
    )
    .unwrap();
    assert_ne!(new, old);
    assert_eq!(app.library.versions[&old], original);
    assert_eq!(
        app.practice.attempts, attempts,
        "Every Attempt is unchanged."
    );
    assert_eq!(
        app.practice.studies, studies,
        "The Study keeps its Version."
    );
    assert_eq!(
        app.library.versions[&new].normalized_hash,
        original.normalized_hash
    );
    assert!(app.library.show_version(&old).unwrap().relations.iter().any(|r| matches!(r, Relation::Supersedes{version_id, previous_id} if version_id == &new && previous_id == &old)));
    // Replay returns the same correction.
    let snapshot = app.clone();
    let revision = app.library.works[&original.work_id].revision - 1;
    let replay = call(&mut app, "/api/v1/library/versions/supersede", "spec-correct-case-op", json!({"version_id":old,"revision":revision,"data":text("Corrected Work", "synthetic FIRST clause;  second clause!\n")})).unwrap();
    assert_eq!(replay.entity_id.as_deref(), Some(new.as_str()));
    assert_eq!(app, snapshot);
    // The old Version has its correction; the newer one is what to correct.
    let twice = correct(
        &mut app,
        "spec-correct-twice-op",
        &old,
        text("Corrected Work", "Synthetic first clause second clause.\n"),
    )
    .unwrap_err();
    assert_eq!(twice.status, 409);
    assert!(twice.message.contains("already has a correction"));
    assert_eq!(app, snapshot);
    // A correction that changes nothing at all is refused.
    let same = correct(
        &mut app,
        "spec-correct-same-op",
        &new,
        text(
            "Corrected Work",
            "synthetic FIRST clause;  second clause!\n",
        ),
    )
    .unwrap_err();
    assert_eq!(same.status, 422);
    assert_eq!(app, snapshot);
    // Adding either wording again returns the current Version.
    assert_eq!(
        add(
            &mut app,
            "spec-correct-dedupe-op",
            text("Corrected Work", "Synthetic first clause, second clause.\n")
        )
        .unwrap(),
        new
    );
    // A correction to wording another Work holds also creates a Version.
    add(
        &mut app,
        "spec-correct-other-op",
        text("Another Work", "Words another Work holds.\n"),
    )
    .unwrap();
    let third = correct(
        &mut app,
        "spec-correct-into-other-op",
        &new,
        text("Corrected Work", "Words another Work holds.\n"),
    )
    .unwrap();
    assert_eq!(app.library.versions[&third].work_id, original.work_id);
    // The notebook keeps every record through rows and the complete archive.
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&app).unwrap()).unwrap(),
        app
    );
    let bytes = archive(&app).encode().unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        app
    );
}

#[test]
fn provenance_is_recorded_as_supplied_and_checked_sources_must_be_cited() {
    let mut app = app();
    let plain = add(
        &mut app,
        "spec-provenance-plain-op",
        text("Plain Work", "Supplied synthetic words.\n"),
    )
    .unwrap();
    assert_eq!(stored(&app, &plain)["provenance"], "supplied");
    let mut recalled = text("Recalled Work", "A model wrote these synthetic words.\n");
    recalled["provenance"] = json!("model_from_memory");
    let recalled_id = add(&mut app, "spec-provenance-model-op", recalled.clone()).unwrap();
    assert_eq!(
        stored(&app, &recalled_id)["provenance"],
        "model_from_memory"
    );
    // Checked against cited sources needs a citation; nothing is stored without.
    let before = app.clone();
    let mut checked = recalled.clone();
    checked["provenance"] = json!("checked_against_sources");
    let refused = correct(
        &mut app,
        "spec-provenance-uncited-op",
        &recalled_id,
        checked.clone(),
    )
    .unwrap_err();
    assert_eq!(refused.status, 422);
    assert_eq!(app, before);
    // The same wording, now checked, is a correction: a new Version.
    checked["sources"] = json!(["Synthetic citation, page 1"]);
    let checked_id = correct(
        &mut app,
        "spec-provenance-checked-op",
        &recalled_id,
        checked,
    )
    .unwrap();
    assert_ne!(checked_id, recalled_id);
    assert_eq!(
        stored(&app, &checked_id)["provenance"],
        "checked_against_sources"
    );
    assert_eq!(
        stored(&app, &recalled_id)["provenance"],
        "model_from_memory"
    );
    // Mapping manifests carry it too.
    let mapping = json!({"title":"Synthetic signals","direction":"forward","provenance":"model_from_memory","pairs":[{"key":"Red","value":"Stop"},{"key":"Green","value":"Go"}]});
    let id = call(
        &mut app,
        "/api/v1/library/mappings",
        "spec-provenance-mapping-op",
        json!({"data":mapping}),
    )
    .unwrap()
    .entity_id
    .unwrap();
    assert_eq!(stored(&app, &id)["provenance"], "model_from_memory");
    // Shown where the Version is shown.
    let shown = page(&app, &format!("/library/versions/{recalled_id}"));
    assert!(shown.contains("Written by a model from memory · Private only"));
    assert!(
        page(&app, &format!("/library/versions/{checked_id}"))
            .contains("Checked against cited sources · Private only")
    );
    assert!(
        page(&app, &format!("/library/versions/{plain}")).contains("Supplied text · Private only")
    );
}

#[test]
fn manifest_relations_appear_on_both_works_and_go_with_a_deleted_work() {
    let mut app = app();
    let rosary = add(
        &mut app,
        "spec-relation-rosary-op",
        text("Synthetic Rosary", "Synthetic whole devotion.\n"),
    )
    .unwrap();
    let rosary_work = work_of(&app, &rosary);
    let mut glorious = text(
        "Synthetic Glorious Mysteries",
        "First synthetic mystery.\nSecond synthetic mystery.\n",
    );
    glorious["relations"] = json!([{"kind":"part_of","work":"synthetic rosary"}]);
    let before = app.clone();
    // Malformed relations refuse the whole manifest and store nothing.
    for (n, relations) in [
        json!([{"kind":"part_of","work":"A Work not in the Library"}]),
        json!([{"kind":"part_of","work":"Synthetic Rosary"},{"kind":"part_of","work":"SYNTHETIC ROSARY"}]),
        json!([{"kind":"part_of","work":"Synthetic Glorious Mysteries"}]),
        json!([{"kind":"supersedes","work":"Synthetic Rosary"}]),
        json!([{"kind":"part_of","work":"Synthetic Rosary","note":"extra"}]),
    ]
    .into_iter()
    .enumerate()
    {
        let mut bad = glorious.clone();
        bad["relations"] = relations;
        let error = add(&mut app, &format!("spec-relation-bad-{n}-op"), bad).unwrap_err();
        assert_eq!(error.status, 422, "case {n}");
        assert_eq!(app, before, "case {n}");
    }
    let glorious_id = add(&mut app, "spec-relation-glorious-op", glorious.clone()).unwrap();
    let glorious_work = work_of(&app, &glorious_id);
    let part = Relation::PartOf {
        work_id: glorious_work.clone(),
        parent_work_id: rosary_work.clone(),
    };
    for work in [&glorious_work, &rosary_work] {
        assert!(
            app.library
                .show_work(work)
                .unwrap()
                .relations
                .contains(&&part),
            "on both Works"
        );
    }
    // Replay changes nothing; a related Work cannot also be its own part.
    let snapshot = app.clone();
    assert_eq!(
        add(&mut app, "spec-relation-glorious-op", glorious).unwrap(),
        glorious_id
    );
    assert_eq!(app, snapshot);
    let mut cycle = text(
        "Synthetic Rosary",
        "Another wording of the whole devotion.\n",
    );
    cycle["relations"] = json!([{"kind":"part_of","work":"Synthetic Glorious Mysteries"}]);
    assert_eq!(
        add(&mut app, "spec-relation-cycle-op", cycle)
            .unwrap_err()
            .status,
        422
    );
    assert_eq!(app, snapshot);
    // Related to, declared from the other side, is one Relation shown on both.
    let mut related = text(
        "Synthetic Sorrowful Mysteries",
        "Third synthetic mystery.\n",
    );
    related["relations"] = json!([{"kind":"related_to","work":"Synthetic Glorious Mysteries"},{"kind":"part_of","work":"Synthetic Rosary"}]);
    let sorrowful = add(&mut app, "spec-relation-sorrowful-op", related).unwrap();
    let sorrowful_work = work_of(&app, &sorrowful);
    let mut back = text(
        "Synthetic Glorious Mysteries",
        "Another synthetic glorious wording.\n",
    );
    back["relations"] = json!([{"kind":"related_to","work":"Synthetic Sorrowful Mysteries"}]);
    add(&mut app, "spec-relation-back-op", back).unwrap();
    let related_count = |app: &App| {
        app.library
            .relations
            .values()
            .filter(|r| matches!(r, Relation::RelatedTo { .. }))
            .count()
    };
    assert_eq!(related_count(&app), 1);
    // The app shows each Relation on both Works, and on the Version pages.
    let rosary_page = page(&app, &format!("/library/works/{rosary_work}"));
    assert!(rosary_page.contains(&format!("Includes <a class=\"text-link\" href=\"/library/works/{glorious_work}\">Synthetic Glorious Mysteries</a>")));
    assert!(rosary_page.contains(&format!("Includes <a class=\"text-link\" href=\"/library/works/{sorrowful_work}\">Synthetic Sorrowful Mysteries</a>")));
    let glorious_page = page(&app, &format!("/library/works/{glorious_work}"));
    assert!(glorious_page.contains(&format!(
        "Part of <a class=\"text-link\" href=\"/library/works/{rosary_work}\">Synthetic Rosary</a>"
    )));
    assert!(glorious_page.contains("Related to <a class=\"text-link\""));
    assert!(
        page(&app, &format!("/library/works/{sorrowful_work}")).contains(&format!(
            "Related to <a class=\"text-link\" href=\"/library/works/{glorious_work}\">"
        ))
    );
    assert!(
        page(&app, &format!("/library/versions/{glorious_id}"))
            .contains("Part of <a class=\"text-link\"")
    );
    assert!(page(&app, "/library").contains(&format!(
        "Includes <a class=\"text-link\" href=\"/library/works/{glorious_work}\">"
    )));
    // Relations already in the stored shape keep the earlier format name.
    assert_eq!(archive(&app).format, PLAIN_FORMAT);
    // Deleting an unused Work removes its Relations from both sides.
    let revision = app.library.works[&sorrowful_work].revision;
    call(
        &mut app,
        "/api/v1/library/works/delete",
        "spec-relation-delete-op",
        json!({"work_id":sorrowful_work,"revision":revision}),
    )
    .unwrap();
    assert_eq!(related_count(&app), 0);
    assert!(
        !page(&app, &format!("/library/works/{rosary_work}"))
            .contains("Synthetic Sorrowful Mysteries")
    );
}

#[test]
fn corrections_are_shown_with_the_version_they_supersede() {
    let mut app = app();
    let old = add(
        &mut app,
        "spec-shown-original-op",
        text("Shown Work", "Synthetic shown words.\n"),
    )
    .unwrap();
    let mut data = text("Shown Work", "SYNTHETIC SHOWN WORDS!\n");
    data["name"] = json!("Synthetic edition, corrected");
    let new = correct(&mut app, "spec-shown-correct-op", &old, data).unwrap();
    let old_page = page(&app, &format!("/library/versions/{old}"));
    assert!(old_page.contains(&format!("Corrected by <a class=\"text-link\" href=\"/library/versions/{new}\">Synthetic edition, corrected</a>")));
    let new_page = page(&app, &format!("/library/versions/{new}"));
    assert!(new_page.contains(&format!(
        "Corrects <a class=\"text-link\" href=\"/library/versions/{old}\">Synthetic edition</a>"
    )));
    let work = work_of(&app, &old);
    let work_page = page(&app, &format!("/library/works/{work}"));
    assert!(work_page.contains(&format!(
        "href=\"/library/versions/{old}\">Synthetic edition</a>"
    )));
    assert!(work_page.contains(&format!(
        "href=\"/library/versions/{new}\">Synthetic edition, corrected</a>"
    )));
    // An unused corrected Version can still be deleted, and the rest stays valid.
    let revision = app.library.works[&work].revision;
    call(
        &mut app,
        "/api/v1/library/versions/delete",
        "spec-shown-delete-op",
        json!({"version_id":old,"revision":revision}),
    )
    .unwrap();
    assert!(!page(&app, &format!("/library/versions/{new}")).contains("Corrects"));
}

#[test]
fn protected_recall_hides_every_library_page_and_relation() {
    let mut app = app();
    let rosary = add(
        &mut app,
        "spec-protected-rosary-op",
        text("Protected Rosary", "Synthetic protected whole.\n"),
    )
    .unwrap();
    let mut part = text("Protected Part", "Synthetic protected answer words.\n");
    part["relations"] = json!([{"kind":"part_of","work":"Protected Rosary"}]);
    part["provenance"] = json!("model_from_memory");
    let vid = add(&mut app, "spec-protected-part-op", part).unwrap();
    let work = work_of(&app, &vid);
    let study = call(
        &mut app,
        "/api/v1/learn",
        "spec-protected-learn-op",
        json!({"version_id":vid,"pace":"steady"}),
    )
    .unwrap()
    .entity_id
    .unwrap();
    call(
        &mut app,
        "/api/v1/next",
        "spec-protected-next-op",
        json!({"study_id":study}),
    )
    .unwrap();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    call(
        &mut app,
        "/api/v1/quiz",
        "spec-protected-quiz-op",
        json!({"prompt_id":prompt}),
    )
    .unwrap();
    assert!(app.practice.protected());
    let rosary_work = work_of(&app, &rosary);
    for path in [
        "/library".to_string(),
        format!("/library/works/{work}"),
        format!("/library/works/{rosary_work}"),
        format!("/library/versions/{vid}"),
    ] {
        let html = page(&app, &path);
        assert!(html.contains("data-state=\"library-gate\""), "{path}");
        for hidden in [
            "answer words",
            "Protected Part",
            "Includes",
            "Written by a model",
        ] {
            assert!(!html.contains(hidden), "{path} shows {hidden}");
        }
    }
    assert!(web::library_destination(&format!("/library/works/{work}")));
}

#[test]
fn a_notebook_that_uses_no_new_capability_keeps_its_format_and_rows() {
    let mut app = app();
    let old = add(
        &mut app,
        "spec-format-plain-op",
        text("Format Work", "Synthetic format words.\n"),
    )
    .unwrap();
    // A correction that changes the normalized text needs no new shape.
    correct(
        &mut app,
        "spec-format-reworded-op",
        &old,
        text("Format Work", "Different synthetic format words.\n"),
    )
    .unwrap();
    let rows = persistence::rows(&app).unwrap();
    assert!(!rows.contains_key(MARKER));
    assert_eq!(archive(&app).format, PLAIN_FORMAT);
    let mut marked = rows.clone();
    marked.insert(MARKER.into(), "1".into());
    assert!(
        persistence::app_from_rows(&marked).is_err(),
        "A marker on a plain notebook is refused."
    );
    let mut forged = archive(&app);
    forged.format = NEW_FORMAT.into();
    assert!(forged.validate().is_err());
}

#[test]
fn a_notebook_that_uses_provenance_or_a_same_text_correction_is_named_for_it() {
    for case in ["provenance", "case-only correction"] {
        let mut app = app();
        let old = add(
            &mut app,
            "spec-format-base-op",
            text("Named Work", "Synthetic named words.\n"),
        )
        .unwrap();
        if case == "provenance" {
            let mut data = text("Named Work", "Other synthetic named words.\n");
            data["provenance"] = json!("model_from_memory");
            add(&mut app, "spec-format-model-op", data).unwrap();
        } else {
            correct(
                &mut app,
                "spec-format-case-op",
                &old,
                text("Named Work", "synthetic NAMED words\n"),
            )
            .unwrap();
        }
        let rows = persistence::rows(&app).unwrap();
        assert_eq!(rows.get(MARKER).map(String::as_str), Some("1"), "{case}");
        assert_eq!(persistence::app_from_rows(&rows).unwrap(), app, "{case}");
        let mut unmarked = rows.clone();
        unmarked.remove(MARKER);
        assert!(persistence::app_from_rows(&unmarked).is_err(), "{case}");
        let named = archive(&app);
        assert_eq!(named.format, NEW_FORMAT, "{case}");
        let bytes = named.encode().unwrap();
        let decoded = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
        assert_eq!(decoded.app, app);
        assert_eq!(decoded.encode().unwrap(), bytes);
        for earlier in [PLAIN_FORMAT, "scry-rust-durable-sqlite-study-judged-v4"] {
            let mut forged = named.clone();
            forged.format = earlier.into();
            assert!(forged.validate().is_err(), "{case} under {earlier}");
        }
    }
}

#[test]
fn text_without_boundaries_is_divided_at_line_breaks_and_clauses_and_rejoins_exactly() {
    let mut app = app();
    for (n, words) in [
        "One clause, two clause; three: four. Five! Six?\nNew line\r\nLast line with no end",
        "\n\nLeading blank lines,\n\n\nand trailing ones.\n\n",
        "Pi is 3.14, e is 2.71; done.",
        "No boundary at all",
        "  Spaced ,  oddly ;  punctuated .\n",
    ]
    .into_iter()
    .enumerate()
    {
        let id = add(
            &mut app,
            &format!("spec-segment-{n}-op"),
            text(&format!("Segment {n}"), words),
        )
        .unwrap();
        let v = &app.library.versions[&id];
        let units: Vec<_> = v
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .map(|u| u.text.as_str())
            .collect();
        assert_eq!(units.concat(), words, "case {n}");
        let lines = words
            .split_inclusive('\n')
            .filter(|l| !l.trim().is_empty())
            .count();
        assert_eq!(
            v.sections.len(),
            lines.max(1),
            "case {n}: one Section per line"
        );
        for unit in &units[..units.len() - 1] {
            assert!(
                unit.trim_end().ends_with([',', ';', ':', '.', '!', '?']) || unit.ends_with('\n'),
                "case {n}: {unit:?} ends at a clause or line"
            );
        }
    }
    let first = &app
        .library
        .versions
        .values()
        .find(|v| v.text.starts_with("One clause"))
        .unwrap();
    let units: Vec<_> = first.sections[0]
        .units
        .iter()
        .map(|u| u.text.as_str())
        .collect();
    assert_eq!(
        units,
        [
            "One clause,",
            " two clause;",
            " three:",
            " four.",
            " Five!",
            " Six?\n"
        ]
    );
}

// --- Review findings: starters, same text with different structure, cycles ---

fn engine_call(
    app: &mut App,
    path: &str,
    op: &str,
    now: i64,
    pairs: Value,
) -> AppResult<MutationResult> {
    let mut fields = api::fields(pairs)?;
    fields.insert("operation_id".into(), op.into());
    let token = app.owner_tokens.values().next().unwrap().info.clone();
    let result = engine::mutate_as(app, path, &fields, now, op, MutationActor::Token(&token))?;
    engine::validate_app(app).unwrap();
    Ok(result)
}
/// Answer whatever is held, trying it from memory first.
fn answer_held(app: &mut App, op: &str, now: i64) -> String {
    let held = app.practice.current.clone().unwrap();
    if held.source_visible() {
        engine_call(
            app,
            "/practice/quiz",
            &format!("{op}-quiz"),
            now,
            json!({"prompt_id":held.prompt.id}),
        )
        .unwrap();
    }
    let pid = app.practice.current.as_ref().unwrap().prompt.id.clone();
    engine_call(
        app,
        "/practice/answer",
        &format!("{op}-answer"),
        now,
        json!({"prompt_id":pid,"answer":"synthetic miss"}),
    )
    .unwrap();
    pid
}
/// Correct a Version with exactly its own text and Units, adding one source.
fn cite(app: &mut App, op: &str, old: &str) -> String {
    let v = app.library.versions[old].clone();
    let w = app.library.works[&v.work_id].clone();
    let sections: Vec<Value> = v
        .sections
        .iter()
        .map(|s| json!({"heading":s.heading,"units":s.units.iter().map(|u| u.text.clone()).collect::<Vec<_>>()}))
        .collect();
    let mut sources = v.sources.clone();
    sources.push("Synthetic added citation".into());
    let data = json!({"title":w.title,"kind":v.kind,"text":v.text,"name":v.name,"sources":sources,
        "rights":v.rights,"spelling_strict":v.spelling_strict,"ordered_aliases":v.ordered_aliases,"sections":sections});
    correct(app, op, old, data).unwrap()
}

#[test]
fn a_starter_entry_keeps_the_studied_version_after_a_same_text_correction() {
    for entry in ["nato", "lesson", "mix"] {
        let mut app = app();
        let mut now = NOW;
        let start = |app: &mut App, op: &str, now: i64| match entry {
            "nato" => engine_call(app, "/practice/nato/start", op, now, json!({})),
            "lesson" => {
                let after = app
                    .practice
                    .current
                    .as_ref()
                    .filter(|p| p.attempt_id.is_some())
                    .map(|p| p.prompt.id.clone());
                let mut fields = json!({"starter":"st-michael-en"});
                if let Some(after) = after {
                    fields["after"] = json!(after);
                }
                engine_call(app, "/practice/lesson/start", op, now, fields)
            }
            _ => match app
                .practice
                .current
                .as_ref()
                .filter(|p| p.attempt_id.is_some())
            {
                Some(p) => {
                    let after = p.prompt.id.clone();
                    engine_call(
                        app,
                        "/practice/review/next",
                        op,
                        now,
                        json!({"after":after}),
                    )
                }
                None => engine_call(app, "/practice/review/start", op, now, json!({})),
            },
        };
        start(&mut app, &format!("spec-starter-{entry}-first-op"), now).unwrap();
        let v1 = app
            .practice
            .current
            .as_ref()
            .unwrap()
            .prompt
            .version_id
            .clone();
        answer_held(&mut app, &format!("spec-starter-{entry}-one"), now);
        let cards = app.practice.cards.clone();
        let studies = app.practice.studies.clone();
        assert_eq!(studies.len(), 1, "{entry}");
        let v2 = cite(&mut app, &format!("spec-starter-{entry}-cite-op"), &v1);
        assert_ne!(v1, v2);
        now += 2 * DAY_MS;
        start(&mut app, &format!("spec-starter-{entry}-again-op"), now)
            .unwrap_or_else(|e| panic!("{entry}: {} {}", e.status, e.message));
        let held = &app.practice.current.as_ref().unwrap().prompt;
        assert_eq!(held.version_id, v1, "{entry}: the chosen Version is kept");
        assert_eq!(app.practice.studies.len(), 1, "{entry}: no second Study");
        assert!(
            app.practice.studies.values().all(|s| s.version_id == v1),
            "{entry}"
        );
        for (id, card) in &cards {
            assert_eq!(
                app.practice.cards[id], *card,
                "{entry}: V1's schedule is unchanged"
            );
        }
    }
}

#[test]
fn a_starter_with_no_study_resolves_to_one_unsuperseded_version() {
    let mut app = app();
    // The canonical NATO text is in the Library, corrected once, never studied.
    let mapping: Value =
        serde_json::from_str(include_str!("../examples/nato-alphabet.json")).unwrap();
    let v1 = call(
        &mut app,
        "/api/v1/library/mappings",
        "spec-unstudied-nato-op",
        json!({"data":mapping}),
    )
    .unwrap()
    .entity_id
    .unwrap();
    let v2 = cite(&mut app, "spec-unstudied-cite-op", &v1);
    engine_call(
        &mut app,
        "/practice/nato/start",
        "spec-unstudied-start-op",
        NOW,
        json!({}),
    )
    .unwrap();
    assert_eq!(app.practice.current.as_ref().unwrap().prompt.version_id, v2);
    assert_eq!(app.practice.studies.len(), 1);
}

#[test]
fn the_same_text_with_different_units_or_spelling_strict_setting_is_refused_and_says_why() {
    let mut app = app();
    let words = "Synthetic hail, full of tests.\nBlessed art thou.\n";
    let first = add(&mut app, "spec-same-first-op", text("Same Work", words)).unwrap();
    let before = app.clone();
    let mut lines = text("Same Work", words);
    lines["sections"] = json!([{"heading":"Lines","units":["Synthetic hail, full of tests.\n","Blessed art thou.\n"]}]);
    let mut strict = text("Same Work", words);
    strict["spelling_strict"] = json!(true);
    let mut both = lines.clone();
    both["spelling_strict"] = json!(true);
    both["title"] = json!("Another Work");
    for (n, (draft, named)) in [
        (lines, "Units"),
        (strict, "Spelling-strict setting"),
        (both, "Units and Spelling-strict setting"),
    ]
    .into_iter()
    .enumerate()
    {
        let error = add(&mut app, &format!("spec-same-refused-{n}-op"), draft).unwrap_err();
        assert_eq!(error.status, 409, "case {n}");
        assert!(
            error.message.contains(&format!("different {named}.")),
            "case {n}: {}",
            error.message
        );
        assert!(error.message.contains("correct it"), "case {n}");
        assert_eq!(app, before, "case {n}: nothing is written");
    }
    // Text supplied without Units matches whatever Units Scry gave it.
    assert_eq!(
        add(
            &mut app,
            "spec-same-pasted-op",
            text("Same Work", "SYNTHETIC HAIL FULL OF TESTS BLESSED ART THOU")
        )
        .unwrap(),
        first
    );
    // A different edition name, source, Rights status or Provenance does not refuse.
    let mut meta = text("Same Work", words);
    meta["name"] = json!("Another edition");
    meta["sources"] = json!(["Synthetic source"]);
    meta["rights"] = json!("public_domain");
    meta["provenance"] = json!("model_from_memory");
    assert_eq!(add(&mut app, "spec-same-meta-op", meta).unwrap(), first);
    assert_eq!(app.library, before.library);
    // Mappings: the same pairs with another Spelling-strict setting.
    let pairs = |strict: bool| json!({"data":{"title":"Same pairs","direction":"forward","spelling_strict":strict,"pairs":[{"key":"Red","value":"Stop"}]}});
    call(
        &mut app,
        "/api/v1/library/mappings",
        "spec-same-pairs-op",
        pairs(true),
    )
    .unwrap();
    let error = call(
        &mut app,
        "/api/v1/library/mappings",
        "spec-same-pairs-lax-op",
        pairs(false),
    )
    .unwrap_err();
    assert_eq!(error.status, 409);
    assert!(error.message.contains("Spelling-strict setting"));
}

#[test]
fn the_same_ordered_list_with_different_aliases_is_refused_and_earlier_pairs_still_load() {
    let mut app = app();
    let list = |title: &str, aliases: Value| json!({"title":title,"kind":"ordered_list","text":"Red gate\nBlue bridge\n","ordered_aliases":aliases});
    let first = add(
        &mut app,
        "spec-alias-first-op",
        list("Alias List", json!({})),
    )
    .unwrap();
    let before = app.clone();
    for (n, draft) in [
        list("Alias List", json!({"0":["Scarlet gate"]})),
        list("Other List", json!({"1":["Azure bridge"]})),
    ]
    .into_iter()
    .enumerate()
    {
        let error = add(&mut app, &format!("spec-alias-refused-{n}-op"), draft).unwrap_err();
        assert_eq!(error.status, 409, "case {n}");
        assert!(
            error.message.contains("different aliases."),
            "case {n}: {}",
            error.message
        );
        assert_eq!(app, before, "case {n}");
    }
    assert_eq!(
        add(
            &mut app,
            "spec-alias-same-op",
            list("Alias List", json!({}))
        )
        .unwrap(),
        first
    );
    let aliased = add(
        &mut app,
        "spec-alias-aliased-op",
        list("Aliased List", json!({"0":["Scarlet gate"]})),
    );
    assert_eq!(aliased.unwrap_err().status, 409);
    // A notebook an earlier build admitted, holding both, still validates,
    // stores and backs up unchanged.
    let mut earlier = app.clone();
    let mut scratch = scry::library::Library::default();
    let input: scry::library::VersionInput =
        serde_json::from_value(list("Earlier aliased List", json!({"0":["Scarlet gate"]})))
            .unwrap();
    let id = scratch.add(input, "spec-alias-earlier-seed", None).unwrap();
    let version = scratch.versions[&id].clone();
    earlier.library.works.insert(
        version.work_id.clone(),
        scratch.works[&version.work_id].clone(),
    );
    earlier.library.relations.extend(scratch.relations.clone());
    earlier.library.versions.insert(id, version);
    engine::validate_app(&earlier).unwrap();
    assert_eq!(
        persistence::app_from_rows(&persistence::rows(&earlier).unwrap()).unwrap(),
        earlier
    );
    let named = archive(&earlier);
    assert_eq!(named.format, PLAIN_FORMAT);
    let bytes = named.encode().unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        earlier
    );
}

#[test]
fn restored_data_with_a_part_of_cycle_is_refused() {
    let mut app = app();
    let a = add(
        &mut app,
        "spec-cycle-first-op",
        text("Cycle A", "Synthetic cycle words A.\n"),
    )
    .unwrap();
    let mut b = text("Cycle B", "Synthetic cycle words B.\n");
    b["relations"] = json!([{"kind":"part_of","work":"Cycle A"}]);
    let b = add(&mut app, "spec-cycle-second-op", b).unwrap();
    engine::validate_app(&app).unwrap();
    let (wa, wb) = (work_of(&app, &a), work_of(&app, &b));
    let mut cycle = app.clone();
    cycle.library.relations.insert(
        format!("part-of-{wa}-{wb}"),
        Relation::PartOf {
            work_id: wa,
            parent_work_id: wb,
        },
    );
    assert!(engine::validate_app(&cycle).is_err());
    let forged = archive(&cycle);
    assert!(forged.encode().is_err());
    let bytes = serde_json::to_vec(&forged).unwrap();
    assert!(persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).is_err());
}

#[test]
fn an_add_that_finds_its_text_already_there_says_so() {
    let mut app = app();
    let created = call(
        &mut app,
        "/api/v1/library/versions",
        "spec-existing-first-op",
        json!({"data":text("Existing Work", "Synthetic existing words.\n")}),
    )
    .unwrap();
    assert_eq!(
        created.data, None,
        "A creation keeps its earlier receipt shape."
    );
    let again = call(
        &mut app,
        "/api/v1/library/versions",
        "spec-existing-again-op",
        json!({"data":text("Existing Work", "SYNTHETIC EXISTING WORDS")}),
    )
    .unwrap();
    assert_eq!(again.entity_id, created.entity_id);
    assert_eq!(again.data, Some(json!({"existing":true})));
    assert_eq!(
        api::library_existing("/library/version/add", &again),
        Some(true)
    );
    assert_eq!(
        api::library_existing("/library/version/add", &created),
        Some(false)
    );
    let replay = call(
        &mut app,
        "/api/v1/library/versions",
        "spec-existing-again-op",
        json!({"data":text("Existing Work", "SYNTHETIC EXISTING WORDS")}),
    )
    .unwrap();
    assert_eq!(replay, again);
    let mapping = json!({"data":{"title":"Existing pairs","direction":"forward","pairs":[{"key":"Up","value":"North"}]}});
    call(
        &mut app,
        "/api/v1/library/mappings",
        "spec-existing-map-op",
        mapping.clone(),
    )
    .unwrap();
    assert_eq!(
        call(
            &mut app,
            "/api/v1/library/mappings",
            "spec-existing-map-again",
            mapping
        )
        .unwrap()
        .data,
        Some(json!({"existing":true}))
    );
    let id = created.entity_id.unwrap();
    let mut query = BTreeMap::new();
    query.insert("existing".to_string(), "1".to_string());
    let shown = web::render(&app, &format!("/library/versions/{id}"), &query, NOW, None);
    assert!(shown.contains("That text was already in the Library. Nothing was added."));
    assert!(!page(&app, &format!("/library/versions/{id}")).contains("already in the Library"));
}

// --- Second review: the Starter shelf is always startable ---

/// Exactly the fields the app's add form produces.
fn app_form_add(app: &mut App, op: &str, title: &str, words: &str) -> AppResult<String> {
    let mut fields = BTreeMap::from([
        ("operation_id".to_string(), op.to_string()),
        ("title".to_string(), title.to_string()),
        ("text".to_string(), words.to_string()),
        ("name".to_string(), String::new()),
        ("source".to_string(), String::new()),
    ]);
    api::library_form_fields(&mut fields)?;
    let token = app.owner_tokens.values().next().unwrap().info.clone();
    let result = engine::mutate_as(
        app,
        "/library/version/add",
        &fields,
        NOW,
        op,
        MutationActor::Token(&token),
    )?;
    engine::validate_app(app).unwrap();
    Ok(result.entity_id.unwrap())
}
fn starter_input(id: &str) -> Value {
    let lessons: Value = serde_json::from_str(include_str!("../assets/lessons.json")).unwrap();
    lessons
        .as_array()
        .unwrap()
        .iter()
        .find(|l| l["id"] == id)
        .unwrap()["input"]
        .clone()
}
fn nato_text() -> String {
    scry::library::nato_input().unwrap().text
}
fn held_version(app: &App) -> String {
    app.practice
        .current
        .as_ref()
        .unwrap()
        .prompt
        .version_id
        .clone()
}
fn copy_rows(app: &App, copy: &str) -> (Value, Value) {
    let v = &app.library.versions[copy];
    (
        serde_json::to_value(v).unwrap(),
        serde_json::to_value(&app.library.works[&v.work_id]).unwrap(),
    )
}
/// Every default starter but the one under test is represented by a dropped
/// Study, so Study's next choice is that starter.
fn only_this_starter_left(app: &mut App, keep: &str) {
    let titles = [
        ("nato", "NATO spelling alphabet"),
        ("st-michael-en", "Saint Michael the Archangel"),
        ("rosary-joyful", "Joyful Mysteries"),
        ("rosary-luminous", "Luminous Mysteries"),
        ("rosary-sorrowful", "Sorrowful Mysteries"),
        ("rosary-glorious", "Glorious Mysteries"),
        ("baby-soothing", "Safe soothing"),
        ("baby-everyday", "Everyday readiness"),
        ("baby-montessori", "Montessori-informed care"),
    ];
    for (i, (id, title)) in titles.iter().enumerate().filter(|(_, (id, _))| *id != keep) {
        let v = add(
            app,
            &format!("spec-filler-{i:02}-{id}-op"),
            text(title, &format!("Filler words number {i}.\n")),
        )
        .unwrap();
        let s = call(
            app,
            "/api/v1/learn",
            &format!("spec-filler-learn-{i:02}-op"),
            json!({"version_id":v}),
        )
        .unwrap()
        .entity_id
        .unwrap();
        call(
            app,
            "/api/v1/studies/drop",
            &format!("spec-filler-drop-{i:02}-op"),
            json!({"study_id":s}),
        )
        .unwrap();
    }
}
fn start_entry(app: &mut App, entry: &str, op: &str) -> AppResult<MutationResult> {
    let after = app
        .practice
        .current
        .as_ref()
        .filter(|p| p.attempt_id.is_some())
        .map(|p| p.prompt.id.clone());
    match entry {
        "nato" => engine_call(app, "/practice/nato/start", op, NOW, json!({})),
        "mix" => match after {
            Some(after) => engine_call(
                app,
                "/practice/review/next",
                op,
                NOW,
                json!({"after":after}),
            ),
            None => engine_call(app, "/practice/review/start", op, NOW, json!({})),
        },
        starter => {
            let mut fields = json!({"starter":starter});
            if let Some(after) = after {
                fields["after"] = json!(after);
            }
            engine_call(app, "/practice/lesson/start", op, NOW, fields)
        }
    }
}

#[test]
fn an_unstudied_copy_never_blocks_a_starter_which_starts_on_its_own_version() {
    let michael = starter_input("st-michael-en");
    for entry in ["st-michael-en", "nato", "rosary-joyful"] {
        let mut app = app();
        let copy = match entry {
            "nato" => {
                app_form_add(&mut app, "spec-copy-nato-op", "My alphabet", &nato_text()).unwrap()
            }
            // An agent's copy of the list, without the starter's aliases.
            "rosary-joyful" => {
                let joy = starter_input("rosary-joyful");
                add(&mut app, "spec-copy-joyful-op", json!({"title":"My Joyful set","kind":"ordered_list","text":joy["text"],"sections":joy["sections"]})).unwrap()
            }
            _ => app_form_add(
                &mut app,
                "spec-copy-michael-op",
                "My Saint Michael",
                michael["text"].as_str().unwrap(),
            )
            .unwrap(),
        };
        let before = copy_rows(&app, &copy);
        start_entry(&mut app, entry, &format!("spec-starter-own-{entry}-op"))
            .unwrap_or_else(|e| panic!("{entry}: {} {}", e.status, e.message));
        let own = held_version(&app);
        assert_ne!(own, copy, "{entry}: the starter's own Version");
        let v = &app.library.versions[&own];
        if entry == "nato" {
            assert!(scry::library::is_nato(v));
        } else {
            assert!(
                scry::lessons::catalog()
                    .iter()
                    .any(|l| l.id == entry && l.matches(v)),
                "{entry}"
            );
        }
        assert_eq!(
            copy_rows(&app, &copy),
            before,
            "{entry}: the copy is unchanged"
        );
        assert!(
            app.practice.studies.values().all(|s| s.version_id != copy),
            "{entry}"
        );
        // Repeating the entry returns the same Version and Study.
        let studies = app.practice.studies.clone();
        answer_held(&mut app, &format!("spec-starter-own-{entry}-answer"), NOW);
        start_entry(
            &mut app,
            entry,
            &format!("spec-starter-own-{entry}-again-op"),
        )
        .unwrap();
        assert_eq!(held_version(&app), own, "{entry}: repeated");
        assert_eq!(
            app.practice.studies.len(),
            studies.len(),
            "{entry}: no new Study"
        );
    }
}

#[test]
fn study_offers_an_unstudied_copy_as_a_library_work_and_never_refuses() {
    // Library Works not yet started come before the Starter shelf, so with an
    // unstudied copy Study starts the copy's Work; the starter is never refused.
    let mut app = app();
    let michael = starter_input("st-michael-en");
    let copy = app_form_add(
        &mut app,
        "spec-mix-copy-op",
        "My Saint Michael",
        michael["text"].as_str().unwrap(),
    )
    .unwrap();
    only_this_starter_left(&mut app, "st-michael-en");
    let versions = app.library.versions.len();
    start_entry(&mut app, "mix", "spec-mix-copy-start-op").unwrap();
    assert_eq!(held_version(&app), copy);
    assert_eq!(app.library.versions.len(), versions);
    answer_held(&mut app, "spec-mix-copy-answer", NOW);
    start_entry(&mut app, "mix", "spec-mix-copy-again-op").unwrap();
    assert_eq!(
        app.library.versions.len(),
        versions,
        "the starter is represented by the copy now"
    );
    assert!(
        app.practice
            .studies
            .values()
            .filter(|s| s.status == scry::study::Status::Active)
            .all(|s| s.version_id == copy)
    );
}

#[test]
fn a_studied_copy_is_kept_by_the_starter_entry_whatever_its_status() {
    let michael = starter_input("st-michael-en");
    for status in ["active", "paused", "dropped"] {
        for entry in ["st-michael-en", "mix"] {
            let mut app = app();
            let copy = app_form_add(
                &mut app,
                "spec-kept-copy-op",
                "My Saint Michael",
                michael["text"].as_str().unwrap(),
            )
            .unwrap();
            if entry == "mix" {
                only_this_starter_left(&mut app, "st-michael-en");
            }
            let sid = call(
                &mut app,
                "/api/v1/learn",
                "spec-kept-learn-op",
                json!({"version_id":copy}),
            )
            .unwrap()
            .entity_id
            .unwrap();
            if status != "active" {
                let route = if status == "paused" {
                    "/api/v1/studies/pause"
                } else {
                    "/api/v1/studies/drop"
                };
                call(
                    &mut app,
                    route,
                    "spec-kept-status-op",
                    json!({"study_id":sid}),
                )
                .unwrap();
            }
            let versions = app.library.versions.len();
            let studies = app.practice.studies.len();
            let result = start_entry(&mut app, entry, &format!("spec-kept-{status}-{entry}-op"));
            assert_eq!(
                app.library.versions.len(),
                versions,
                "{status} {entry}: no second Version"
            );
            assert_eq!(
                app.practice.studies.len(),
                studies,
                "{status} {entry}: no second Study"
            );
            if entry == "mix" {
                // Study never bypasses the Owner's chosen wording with the starter.
                result.unwrap();
                assert!(
                    app.practice
                        .current
                        .as_ref()
                        .is_none_or(|p| p.prompt.version_id == copy),
                    "{status}"
                );
                assert_eq!(
                    app.practice.studies[&sid].status == scry::study::Status::Active,
                    status == "active"
                );
            } else {
                result.unwrap_or_else(|e| panic!("{status}: {} {}", e.status, e.message));
                assert_eq!(
                    held_version(&app),
                    copy,
                    "{status}: the copy's Study is kept or resumed"
                );
                assert_eq!(
                    app.practice.studies[&sid].status,
                    scry::study::Status::Active
                );
                assert_eq!(app.practice.current.as_ref().unwrap().prompt.study_id, sid);
                // Repeating returns the same Version and Study.
                answer_held(&mut app, &format!("spec-kept-{status}-answer"), NOW);
                start_entry(&mut app, entry, &format!("spec-kept-{status}-again-op")).unwrap();
                assert_eq!(held_version(&app), copy);
                assert_eq!(app.practice.studies.len(), studies);
            }
        }
    }
    // A list starter keeps the agent's studied copy without aliases.
    let mut app = app();
    let joy = starter_input("rosary-joyful");
    let copy = add(&mut app, "spec-kept-joyful-op", json!({"title":"My Joyful set","kind":"ordered_list","text":joy["text"],"sections":joy["sections"]})).unwrap();
    call(
        &mut app,
        "/api/v1/learn",
        "spec-kept-joyful-learn",
        json!({"version_id":copy}),
    )
    .unwrap();
    start_entry(&mut app, "rosary-joyful", "spec-kept-joyful-start-op").unwrap();
    assert_eq!(held_version(&app), copy);
    assert_eq!(app.library.versions.len(), 1);
}

#[test]
fn what_the_owner_or_an_agent_adds_is_still_refused_for_the_same_text_with_another_structure() {
    let mut app = app();
    start_entry(&mut app, "st-michael-en", "spec-refuse-start-op").unwrap();
    let starter = held_version(&app);
    let words = app.library.versions[&starter].text.clone();
    let before = app.library.clone();
    let form =
        app_form_add(&mut app, "spec-refuse-form-op", "My Saint Michael", &words).unwrap_err();
    assert_eq!(form.status, 409);
    assert!(form.message.contains("Nothing was saved"));
    let mut manifest = text("My Saint Michael", &words);
    manifest["sections"] = json!([{"heading":"All","units":[words]}]);
    let agent = add(&mut app, "spec-refuse-agent-op", manifest).unwrap_err();
    assert_eq!(agent.status, 409);
    assert_eq!(app.library, before);
}

#[test]
fn a_starter_beside_an_owner_copy_is_named_for_the_shared_text_or_keeps_its_bytes() {
    // Text: the starter and the copy share a normalized hash.
    let mut app = app();
    let michael = starter_input("st-michael-en");
    app_form_add(
        &mut app,
        "spec-beside-copy-op",
        "My Saint Michael",
        michael["text"].as_str().unwrap(),
    )
    .unwrap();
    assert_eq!(archive(&app).format, PLAIN_FORMAT);
    start_entry(&mut app, "st-michael-en", "spec-beside-start-op").unwrap();
    let rows = persistence::rows(&app).unwrap();
    assert_eq!(rows.get(MARKER).map(String::as_str), Some("1"));
    let mut unmarked = rows.clone();
    unmarked.remove(MARKER);
    assert!(persistence::app_from_rows(&unmarked).is_err());
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), app);
    let named = archive(&app);
    assert_eq!(named.format, NEW_FORMAT);
    let bytes = named.encode().unwrap();
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .encode()
            .unwrap(),
        bytes
    );
    // A list: the starter's aliases give it another stored hash, so no marker.
    let mut app = self::app();
    let joy = starter_input("rosary-joyful");
    add(&mut app, "spec-beside-joyful-op", json!({"title":"My Joyful set","kind":"ordered_list","text":joy["text"],"sections":joy["sections"]})).unwrap();
    start_entry(&mut app, "rosary-joyful", "spec-beside-joyful-start").unwrap();
    assert_eq!(app.library.versions.len(), 2);
    let hashes: std::collections::BTreeSet<_> = app
        .library
        .versions
        .values()
        .map(|v| &v.normalized_hash)
        .collect();
    assert_eq!(hashes.len(), 2);
    assert!(!persistence::rows(&app).unwrap().contains_key(MARKER));
    assert_ne!(archive(&app).format, NEW_FORMAT);
}

#[test]
fn a_long_part_of_chain_validates_quickly_and_a_cycle_at_any_depth_is_refused() {
    let mut library = scry::library::Library::default();
    let depth = 5000;
    let ids: Vec<String> = (0..depth).map(|i| format!("{i:032x}")).collect();
    for (i, id) in ids.iter().enumerate() {
        let work: scry::library::Work = serde_json::from_value(json!({"id":id,"revision":1,"title":format!("Chain work {i}"),"aliases":[],"kind":"text","context":""})).unwrap();
        library.works.insert(id.clone(), work);
        if i > 0 {
            library.relations.insert(
                format!("part-of-{}-{}", ids[i - 1], id),
                Relation::PartOf {
                    work_id: ids[i - 1].clone(),
                    parent_work_id: id.clone(),
                },
            );
        }
    }
    let started = std::time::Instant::now();
    library.validate().unwrap();
    let took = started.elapsed();
    assert!(
        took < std::time::Duration::from_secs(2),
        "{depth} part-of Relations took {took:?}"
    );
    for back in [1, 2, depth / 2, depth - 1] {
        let mut cycle = library.clone();
        cycle.relations.insert(
            "part-of-cycle".into(),
            Relation::PartOf {
                work_id: ids[depth - 1].clone(),
                parent_work_id: ids[depth - 1 - back].clone(),
            },
        );
        assert!(cycle.validate().is_err(), "a cycle {back} deep");
    }
}
