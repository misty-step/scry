use base64::{Engine as _, engine::general_purpose::STANDARD};
use scry::{api, authored_media as media, engine, model::*, persistence, study};
use serde_json::{Value, json};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
const PNG: &[u8] = include_bytes!("../fixtures/authored-media/square.png");
const WAV: &[u8] = include_bytes!("../fixtures/authored-media/tone.wav");
fn manifest() -> Value {
    let upload = |bytes: &[u8], mime: &str, name: &str, alt: &str, transcript: &str| {
        json!({
            "unit_index":0,"side":"answer","file_name":name,"alt":alt,"transcript":transcript,
            "source":"Synthetic fixture, authored by repository tests","rights":"public_domain","license":"CC0 fixture",
            "mime":mime,"sha256":persistence::sha256(bytes),"bytes_base64":STANDARD.encode(bytes)
        })
    };
    json!({"mapping":{"title":"Authored media fixture","direction":"forward","pairs":[{"key":"A","value":"Alfa"},{"key":"B","value":"Bravo"}]},
        "attachments":[upload(PNG,"image/png","answer-alfa.png","Answer image: Alfa",""),upload(WAV,"audio/wav","answer-alfa.wav","","Answer transcript: Alfa; synthetic tone, not speech")]})
}
fn call(
    app: &mut App,
    path: &str,
    operation: &str,
    mut payload: Value,
) -> AppResult<MutationResult> {
    payload["operation_id"] = json!(operation);
    let fields = api::library_fields(payload)?;
    engine::mutate_as(
        app,
        api::mutation_path(path).unwrap(),
        &fields,
        NOW,
        operation,
        scry::owner_token::MutationActor::Browser,
    )
}
fn added() -> (App, String) {
    let mut app = App::new("synthetic-media-csrf-at-least-24-characters".into());
    let id = call(
        &mut app,
        "/api/v1/library/media",
        "media-initial-import",
        json!({"data":manifest()}),
    )
    .unwrap()
    .entity_id
    .unwrap();
    engine::validate_app(&app).unwrap();
    (app, id)
}
fn archive(app: &App) -> persistence::Archive {
    let input = media::parse(&manifest().to_string()).unwrap();
    let mut archive = persistence::Archive::new(app.clone(), vec![], NOW);
    archive.media = input.checked().unwrap().1;
    archive
}
#[test]
fn png_and_canonical_short_pcm_wav_only_with_exact_byte_identity() {
    let input = media::parse(&manifest().to_string()).unwrap();
    let (refs, assets) = input.checked().unwrap();
    assert_eq!(refs[0].asset.width, Some(8));
    assert_eq!(refs[0].asset.height, Some(8));
    assert_eq!(refs[1].asset.duration_ms, Some(1000));
    assert_eq!(assets.len(), 2);
    for (bytes, mime) in [(PNG, "image/png"), (WAV, "audio/wav")] {
        assert!(media::inspect(bytes, mime, &"0".repeat(64)).is_err());
        assert!(
            media::inspect(
                &bytes[..bytes.len() - 1],
                mime,
                &persistence::sha256(&bytes[..bytes.len() - 1])
            )
            .is_err()
        );
        assert!(media::inspect(bytes, "image/jpeg", &persistence::sha256(bytes)).is_err());
    }
    let mut png = PNG.to_vec();
    png[20] ^= 1;
    assert!(media::inspect(&png, "image/png", &persistence::sha256(&png)).is_err());
    let mut wav = WAV.to_vec();
    wav[20] = 3;
    assert!(media::inspect(&wav, "audio/wav", &persistence::sha256(&wav)).is_err());
    let mut long = WAV[..44].to_vec();
    long.extend(vec![0; 8000 * 2 * 31]);
    let len = long.len() as u32;
    long[4..8].copy_from_slice(&(len - 8).to_le_bytes());
    long[40..44].copy_from_slice(&(len - 44).to_le_bytes());
    assert!(media::inspect(&long, "audio/wav", &persistence::sha256(&long)).is_err());
    assert!(
        media::inspect(
            &vec![0; media::ASSET_BYTES + 1],
            "audio/wav",
            &"0".repeat(64)
        )
        .is_err()
    );
}
#[test]
fn invalid_second_attachment_metadata_or_bytes_never_partially_admits() {
    let (base, _) = added();
    for (field, value) in [
        ("sha256", json!("0".repeat(64))),
        ("source", json!("")),
        ("license", json!("")),
        ("transcript", json!("")),
        ("unit_index", json!(2)),
        ("mime", json!("audio/mp3")),
        ("file_name", json!("../Alfa.wav")),
        ("bytes_base64", json!("https://example.invalid/a.wav")),
        ("url", json!("https://example.invalid/a.wav")),
    ] {
        let mut app = base.clone();
        let mut data = manifest();
        data["attachments"][1][field] = value;
        assert!(
            call(
                &mut app,
                "/api/v1/library/media",
                "invalid-media-operation",
                json!({"data":data})
            )
            .is_err(),
            "{field}"
        );
        assert_eq!(app, base, "{field}");
    }
    let mut data = manifest();
    let duplicate = data["attachments"][0].clone();
    data["attachments"].as_array_mut().unwrap().push(duplicate);
    assert!(media::parse(&data.to_string()).unwrap().checked().is_err());
    assert_eq!(
        media::parse(&" ".repeat(media::IMPORT_BYTES + 1))
            .unwrap_err()
            .status,
        413
    );
}
#[test]
fn immutable_media_identity_dedupe_and_explicit_same_wording_supersession_preserve_history() {
    let mut app = App::new("synthetic-media-csrf-at-least-24-characters".into());
    let data = manifest();
    let old = call(
        &mut app,
        "/api/v1/library/mappings",
        "media-original-text",
        json!({"data":data["mapping"]}),
    )
    .unwrap()
    .entity_id
    .unwrap();
    let original = app.library.versions[&old].clone();
    call(
        &mut app,
        "/api/v1/learn",
        "media-original-learn",
        json!({"version_id":old}),
    )
    .unwrap();
    call(&mut app, "/api/v1/next", "media-original-next", json!({})).unwrap();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    call(
        &mut app,
        "/api/v1/answer",
        "media-original-answer",
        json!({"prompt_id":prompt,"answer":"Alfa"}),
    )
    .unwrap();
    let history = app.practice.clone();
    let before = app.clone();
    assert_eq!(
        call(
            &mut app,
            "/api/v1/library/media",
            "media-non-explicit-add",
            json!({"data":data})
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, before);
    let revision = app.library.works[&original.work_id].revision;
    let new = call(
        &mut app,
        "/api/v1/library/media/supersede",
        "media-explicit-supersede",
        json!({"data":data,"version_id":old,"revision":revision}),
    )
    .unwrap()
    .entity_id
    .unwrap();
    assert_ne!(old, new);
    assert_eq!(app.library.versions[&old], original);
    assert_eq!(app.practice, history);
    assert_eq!(app.library.versions[&new].text, original.text);
    assert_eq!(
        app.library.versions[&new].normalized_hash,
        original.normalized_hash
    );
    assert!(app.library.versions[&new].media_hash.is_some());
    engine::validate_app(&app).unwrap();
    let before = app.clone();
    let duplicate = call(
        &mut app,
        "/api/v1/library/media",
        "media-identical-dedupe",
        json!({"data":data}),
    )
    .unwrap()
    .entity_id
    .unwrap();
    assert_eq!(duplicate, new);
    assert_eq!(app.library, before.library);
    assert_eq!(app.practice, before.practice);
    let before = app.clone();
    call(
        &mut app,
        "/api/v1/library/media",
        "media-identical-dedupe",
        json!({"data":data}),
    )
    .unwrap();
    assert_eq!(app, before);
    let mut changed = data.clone();
    changed["attachments"][0]["alt"] = json!("Different authored alternative");
    assert_eq!(
        call(
            &mut app,
            "/api/v1/library/media",
            "media-identical-dedupe",
            json!({"data":changed})
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, before);
}
#[test]
fn bundled_nato_starter_can_gain_media_without_rewriting_its_native_attempt_or_schedule() {
    let mut app = App::new("synthetic-media-csrf-at-least-24-characters".into());
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "media-bundled-starter".into()),
        ("revision".into(), app.revision.to_string()),
        ("starter".into(), "nato-v1".into()),
    ]);
    let old = engine::mutate(&mut app, "/library/mapping/add", &fields, NOW, "starter")
        .unwrap()
        .entity_id
        .unwrap();
    call(
        &mut app,
        "/api/v1/learn",
        "media-starter-learn",
        json!({"version_id":old}),
    )
    .unwrap();
    call(&mut app, "/api/v1/next", "media-starter-next", json!({})).unwrap();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    let result = call(
        &mut app,
        "/api/v1/answer",
        "media-starter-answer",
        json!({"prompt_id":prompt,"answer":"Alfa"}),
    )
    .unwrap();
    assert_eq!(
        study::wire_response(&app, &result).unwrap()["attempt"]["grade"]["verdict"],
        "Good"
    );
    let original = app.library.versions[&old].clone();
    let practice = app.practice.clone();
    let mut data = manifest();
    data["mapping"] = serde_json::from_str(include_str!("../examples/nato-alphabet.json")).unwrap();
    let revision = app.library.works[&original.work_id].revision;
    let new = call(
        &mut app,
        "/api/v1/library/media/supersede",
        "media-starter-supersede",
        json!({"data":data,"version_id":old,"revision":revision}),
    )
    .unwrap()
    .entity_id
    .unwrap();
    assert_ne!(old, new);
    assert_eq!(app.library.versions[&old], original);
    assert_eq!(app.library.versions[&new].text, original.text);
    assert_eq!(
        app.library.versions[&new].normalized_hash,
        original.normalized_hash
    );
    assert_eq!(
        app.library.versions[&new]
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .count(),
        26
    );
    assert_eq!(app.practice, practice);
    let library = app.library.clone();
    let mut repeat = fields;
    repeat.insert("operation_id".into(), "media-starter-repeat".into());
    repeat.insert("revision".into(), app.revision.to_string());
    assert_eq!(
        engine::mutate(&mut app, "/library/mapping/add", &repeat, NOW, "repeat")
            .unwrap()
            .entity_id
            .as_deref(),
        Some(old.as_str())
    );
    assert_eq!(app.library, library);
    assert_eq!(app.practice, practice);
    engine::validate_app(&app).unwrap();
}
#[test]
fn complete_archive_roundtrip_rejects_missing_corrupt_extra_media_and_missing_row_marker() {
    let (app, _) = added();
    let a = archive(&app);
    let bytes = a.encode().unwrap();
    assert_eq!(a.format, media::FORMAT);
    assert!(persistence::archive_size(&app).unwrap() >= bytes.len());
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .app,
        app
    );
    let rows = persistence::rows(&app).unwrap();
    assert!(rows.contains_key("authored_media_format/v1"));
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), app);
    let mut missing_marker = rows;
    missing_marker.remove("authored_media_format/v1");
    assert!(persistence::app_from_rows(&missing_marker).is_err());
    let mut missing = a.clone();
    missing.media.pop();
    assert!(missing.encode().is_err());
    let mut corrupt = a.clone();
    corrupt.media[0].bytes[10] ^= 1;
    assert!(corrupt.encode().is_err());
    let mut extra = a.clone();
    extra.media.push(extra.media[0].clone());
    assert!(extra.encode().is_err());
    let mut legacy = a;
    legacy.format = "scry-rust-durable-sqlite-library-v1".into();
    assert!(legacy.validate().is_err());
}
#[test]
fn media_free_state_keeps_exact_old_archive_and_row_shapes() {
    let app = App::new("synthetic-media-csrf-at-least-24-characters".into());
    let a = persistence::Archive::new(app.clone(), vec![], NOW);
    let bytes = a.encode().unwrap();
    assert!(
        serde_json::from_slice::<Value>(&bytes)
            .unwrap()
            .get("media")
            .is_none()
    );
    assert!(
        !persistence::rows(&app)
            .unwrap()
            .contains_key("authored_media_format/v1")
    );
    assert_eq!(
        persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
            .unwrap()
            .encode()
            .unwrap(),
        bytes
    );
}
fn started() -> (App, String) {
    let (mut app, id) = added();
    call(
        &mut app,
        "/api/v1/learn",
        "media-review-learn",
        json!({"version_id":id}),
    )
    .unwrap();
    call(&mut app, "/api/v1/next", "media-review-next", json!({})).unwrap();
    (app, id)
}
fn html(app: &App) -> String {
    scry::web::render(
        app,
        "/recall",
        &BTreeMap::from([("__nonce".into(), "synthetic-media-render".into())]),
        NOW,
        None,
    )
}
#[test]
fn answer_urls_files_alt_transcripts_rights_wait_for_durable_reveal_then_answer_stays_assisted() {
    let (mut app, id) = started();
    let prompt = app.practice.current.as_ref().unwrap().prompt.clone();
    assert!(app.practice.protected());
    assert!(media::for_prompt(&app, &prompt).is_empty());
    assert_eq!(media::lookup(&app, &id, 0).unwrap_err().status, 409);
    for secret in [
        "answer-alfa",
        "Answer image",
        "Answer transcript",
        "CC0 fixture",
        "/media/",
    ] {
        assert!(!html(&app).contains(secret), "{secret}");
    }
    let states = |app: &App| {
        app.practice
            .cards
            .iter()
            .map(|(id, c)| (id.clone(), c.state.clone()))
            .collect::<BTreeMap<_, _>>()
    };
    let before = states(&app);
    let r = call(
        &mut app,
        "/api/v1/reveal",
        "media-durable-reveal",
        json!({"prompt_id":prompt.id}),
    )
    .unwrap();
    let wire = study::wire_response(&app, &r).unwrap();
    assert_eq!(wire["media"].as_array().unwrap().len(), 2);
    assert_eq!(states(&app), before);
    let page = html(&app);
    assert!(page.contains("<audio controls preload=\"none\""));
    assert!(page.contains("Answer transcript"));
    assert!(page.contains("alt=\"Answer image: Alfa\""));
    assert!(media::lookup(&app, &id, 0).is_ok());
    let r = call(
        &mut app,
        "/api/v1/answer",
        "media-assisted-answer",
        json!({"prompt_id":prompt.id,"answer":"Alfa"}),
    )
    .unwrap();
    let wire = study::wire_response(&app, &r).unwrap();
    assert_eq!(wire["attempt"]["grade"]["verdict"], "Good");
    assert_eq!(wire["attempt"]["assisted"], true);
    assert_eq!(states(&app), before);
    call(
        &mut app,
        "/api/v1/next",
        "media-next-protects-old",
        json!({"after":prompt.id}),
    )
    .unwrap();
    assert!(app.practice.protected());
    assert!(media::lookup(&app, &id, 0).is_err());
    assert!(media::for_prompt(&app, &prompt).is_empty());
}
#[test]
fn prompt_declared_image_is_available_but_playback_and_reads_create_no_learning_event() {
    let mut data = manifest();
    data["attachments"][0]["side"] = json!("prompt");
    let mut app = App::new("synthetic-media-csrf-at-least-24-characters".into());
    let id = call(
        &mut app,
        "/api/v1/library/media",
        "media-prompt-side-add",
        json!({"data":data}),
    )
    .unwrap()
    .entity_id
    .unwrap();
    call(
        &mut app,
        "/api/v1/learn",
        "media-prompt-side-learn",
        json!({"version_id":id}),
    )
    .unwrap();
    call(
        &mut app,
        "/api/v1/next",
        "media-prompt-side-next",
        json!({}),
    )
    .unwrap();
    let before = app.clone();
    let prompt = &app.practice.current.as_ref().unwrap().prompt;
    assert_eq!(media::for_prompt(&app, prompt).len(), 1);
    assert!(media::lookup(&app, &id, 0).is_ok());
    assert!(media::lookup(&app, &id, 1).is_err());
    let _ = html(&app);
    assert_eq!(app, before);
    assert!(app.practice.attempts.is_empty());
    let pid = prompt.id.clone();
    let r = call(
        &mut app,
        "/api/v1/answer",
        "media-native-only-grade",
        json!({"prompt_id":pid,"answer":"Alpha"}),
    )
    .unwrap();
    let wire = study::wire_response(&app, &r).unwrap();
    assert_eq!(wire["attempt"]["grade"]["verdict"], "Again");
    assert_eq!(wire["attempt"]["assisted"], false);
}
#[test]
fn complete_archive_capacity_counts_base64_media_before_bodies_are_loaded() {
    let (mut app, id) = added();
    let v = app.library.versions.get_mut(&id).unwrap();
    let a = v.media[0].clone();
    v.media.clear();
    for n in 0..13 {
        let mut a = a.clone();
        a.unit_index = n;
        a.asset.size = media::ASSET_BYTES;
        a.asset.sha256 = format!("{n:064x}");
        v.media.push(a);
    }
    v.media_hash = media::identity(&v.media);
    // Metadata-only capacity counting does not load R2. Supply enough Units so
    // metadata is valid; immutable text/Library validity is checked separately.
    let unit = v.sections[0].units[0].clone();
    v.sections[0].units = vec![unit; 13];
    assert_eq!(persistence::archive_size(&app).unwrap_err().status, 413);
}

#[test]
fn media_dedupe_keeps_canonical_mapping_identity_separate_from_legacy_text_hash() {
    let mut app = App::new("synthetic-media-csrf-at-least-24-characters".into());
    let mut input = manifest();
    input["mapping"]["pairs"][0]["value"] = json!("A&B");
    let id = call(
        &mut app,
        "/api/v1/library/media",
        "media-canonical-original",
        json!({"data":input}),
    )
    .unwrap()
    .entity_id
    .unwrap();
    let before = app.library.clone();
    input["mapping"]["pairs"][0]["value"] = json!("A B");
    assert_eq!(
        call(
            &mut app,
            "/api/v1/library/media",
            "media-canonical-repeat",
            json!({"data":input})
        )
        .unwrap()
        .entity_id
        .unwrap(),
        id
    );
    assert_eq!(app.library, before);
    let revision = app.library.works[&app.library.versions[&id].work_id].revision;
    input["attachments"][0]["source"] = json!("Changed media source");
    assert_eq!(
        call(
            &mut app,
            "/api/v1/library/media",
            "media-canonical-changed",
            json!({"data":input})
        )
        .unwrap_err()
        .status,
        409
    );
    let new = call(
        &mut app,
        "/api/v1/library/media/supersede",
        "media-canonical-supersede",
        json!({"data":input,"version_id":id,"revision":revision}),
    )
    .unwrap()
    .entity_id
    .unwrap();
    assert_ne!(new, id);
    assert_eq!(app.library.versions[&id], before.versions[&id]);
    engine::validate_app(&app).unwrap();
}
#[test]
fn media_identity_cannot_mask_a_cross_kind_legacy_hash_collision() {
    let mut app = App::new("synthetic-media-csrf-at-least-24-characters".into());
    let mut input = manifest();
    input["mapping"]["pairs"][0]["value"] = json!("A&B");
    let id = call(
        &mut app,
        "/api/v1/library/media",
        "media-collision-original",
        json!({"data":input}),
    )
    .unwrap()
    .entity_id
    .unwrap();
    call(
        &mut app,
        "/api/v1/library/versions",
        "media-collision-text",
        json!({"data":{"title":"Independent text","kind":"text","text":"A A B B Bravo"}}),
    )
    .unwrap();
    input["mapping"]["pairs"][0]["value"] = json!("A B");
    input["attachments"][0]["source"] = json!("Changed media source");
    let before = app.clone();
    let revision = app.library.works[&app.library.versions[&id].work_id].revision;
    assert_eq!(
        call(
            &mut app,
            "/api/v1/library/media/supersede",
            "media-collision-refused",
            json!({"data":input,"version_id":id,"revision":revision})
        )
        .unwrap_err()
        .status,
        409
    );
    assert_eq!(app, before);
}
