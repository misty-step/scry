use scry::{
    api, engine, library,
    model::*,
    owner_token::{MutationActor, OwnerToken},
    persistence,
};
use serde_json::{Value, json};
const NOW: i64 = 1_800_000_000_000;
fn app() -> App {
    let mut app = App::new("synthetic-mapping-csrf-at-least-24-chars".into());
    let token = OwnerToken::issue(
        "Mapping builder",
        None,
        false,
        NOW,
        "abcdef0123456789abcdef0123456789abcdef0123456789",
    )
    .unwrap();
    app.owner_tokens.insert(token.info.id.clone(), token);
    app
}
fn archive_roundtrip(app: &App) -> App {
    let bytes = persistence::Archive::new(app.clone(), vec![], NOW)
        .encode()
        .unwrap();
    persistence::Archive::decode(&bytes, &persistence::sha256(&bytes))
        .unwrap()
        .app
}
fn manifest() -> Value {
    json!({"title":"NATO spelling alphabet", "name":"Supplied canonical words", "direction":"forward", "spelling_strict":true,
        "pairs":[{"key":"A","value":"Alfa"},{"key":"J","value":"Juliett"},{"key":"X","value":"X-ray"}]})
}
fn mutate(app: &mut App, path: &str, op: &str, data: Value) -> AppResult<String> {
    let fields = api::library_fields(json!({"operation_id":op,"data":data}))?;
    let token = app.owner_tokens.values().next().unwrap().info.clone();
    let path =
        api::mutation_path(path).ok_or_else(|| AppError::new(404, "Mapping route missing"))?;
    let result = engine::mutate_as(app, path, &fields, NOW, op, MutationActor::Token(&token))?;
    engine::validate_app(app)?;
    Ok(result.entity_id.unwrap())
}
fn add(app: &mut App, op: &str, data: Value) -> AppResult<String> {
    mutate(app, "/api/v1/library/mappings", op, data)
}
#[test]
fn mapping_manifest_uses_existing_library_and_exact_receipts_without_generation() {
    let mut a = app();
    let id = add(&mut a, "mapping-initial-operation", manifest()).unwrap();
    let v = &a.library.versions[&id];
    assert_eq!(v.kind, library::Kind::KeyedMapping);
    assert_eq!(v.text, "A\tAlfa\nJ\tJuliett\nX\tX-ray\n");
    assert_eq!(
        v.sections
            .iter()
            .flat_map(|s| &s.units)
            .map(|u| u.text.as_str())
            .collect::<String>(),
        v.text
    );
    assert_eq!(v.sections.iter().map(|s| s.units.len()).sum::<usize>(), 3);
    assert!(v.spelling_strict);
    assert_eq!(v.provenance, library::Provenance::Supplied);
    let before = a.clone();
    assert_eq!(
        add(&mut a, "mapping-initial-operation", manifest()).unwrap(),
        id
    );
    assert_eq!(a, before);
    assert_eq!(
        add(&mut a, "mapping-repeat-operation", manifest()).unwrap(),
        id
    );
    assert_eq!(a.library, before.library);
    assert!(
        a.jobs.is_empty()
            && a.assessments.is_empty()
            && a.spend.is_empty()
            && a.practice.is_empty()
    );
    assert_eq!(archive_roundtrip(&a), a);
}
#[test]
fn mapping_invalid_manifest_refuses_atomically() {
    let mut cases = vec![];
    for (field, value) in [
        ("direction", json!("reverse")),
        ("generate", json!(true)),
        ("grade", json!("Good")),
        ("pairs", json!([])),
    ] {
        let mut m = manifest();
        m[field] = value;
        cases.push(m);
    }
    for (key, value) in [
        ("", "Alfa"),
        ("A\tB", "Alfa"),
        ("A\nB", "Alfa"),
        (" A", "Alfa"),
        ("A", ""),
        ("A", "Alfa\nBravo"),
        ("A", "Alfa\tBravo"),
        ("A", "Alfa\u{0000}"),
        ("A", "Alfa\u{2028}Bravo"),
        ("A\u{2029}B", "Alfa"),
        ("A", "Alfa "),
    ] {
        let mut m = manifest();
        m["pairs"] = json!([{"key":key,"value":value}]);
        cases.push(m);
    }
    for pair in [
        json!({"key":"a","value":"other"}),
        json!({"key":"Ａ","value":"other"}),
    ] {
        let mut m = manifest();
        m["pairs"].as_array_mut().unwrap().push(pair);
        cases.push(m);
    }
    for (i, m) in cases.into_iter().enumerate() {
        let mut a = app();
        let before = a.clone();
        assert_eq!(
            add(&mut a, &format!("mapping-invalid-operation-{i}"), m)
                .unwrap_err()
                .status,
            422
        );
        assert_eq!(a, before);
    }
}
#[test]
fn mapping_text_adapter_preserves_pairs_not_clause_segmentation() {
    let mut a = app();
    let id=mutate(&mut a,"/api/v1/library/versions","mapping-raw-text-operation",json!({"title":"Pairs","kind":"keyed_mapping","spelling_strict":true,"text":"A\tAlfa.\nX\tX-ray\n"})).unwrap();
    assert_eq!(
        a.library.versions[&id]
            .sections
            .iter()
            .map(|s| s.units.len())
            .sum::<usize>(),
        2
    );
    let before = a.clone();
    for text in [
        "A Alfa\n",
        "A\tAlfa\tExtra\n",
        "A\tAlfa\n\n",
        "A\tAlfa\nA\tBravo\n",
    ] {
        assert_eq!(
            mutate(
                &mut a,
                "/api/v1/library/versions",
                "mapping-raw-bad-operation",
                json!({"title":"Invalid pairs","kind":"keyed_mapping","text":text})
            )
            .unwrap_err()
            .status,
            422
        );
        assert_eq!(a, before);
    }
}
#[test]
fn mapping_deduplication_never_erases_pair_boundaries_or_grading_policy() {
    let mut a = app();
    let mut m = manifest();
    m["pairs"] = json!([{"key":"AB","value":"C"}]);
    add(&mut a, "mapping-collision-original", m.clone()).unwrap();
    let before = a.clone();
    m["pairs"] = json!([{"key":"A","value":"BC"}]);
    assert_eq!(
        add(&mut a, "mapping-collision-boundary", m)
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(a, before);
    let mut m = manifest();
    m["pairs"] = json!([{"key":"AB","value":"C"}]);
    m["spelling_strict"] = json!(false);
    assert_eq!(
        add(&mut a, "mapping-collision-policy", m)
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(a, before);
    let mut b = app();
    mutate(
        &mut b,
        "/api/v1/library/versions",
        "mapping-collision-text-original",
        json!({"title":"Text","kind":"text","text":"AB C"}),
    )
    .unwrap();
    let before = b.clone();
    let mut m = manifest();
    m["pairs"] = json!([{"key":"AB","value":"C"}]);
    assert_eq!(
        add(&mut b, "mapping-collision-text-refuse", m)
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(b, before);
}
#[test]
fn historical_metadata_only_mapping_remains_readable_and_exportable() {
    let mut a = app();
    let id=mutate(&mut a,"/api/v1/library/versions","historical-mapping-fixture",json!({"title":"Historical metadata","kind":"text","text":"Previously stored mapping metadata, not pairs."})).unwrap();
    let v = a.library.versions.get_mut(&id).unwrap();
    v.kind = library::Kind::KeyedMapping;
    a.library.works.get_mut(&v.work_id).unwrap().kind = library::Kind::KeyedMapping;
    engine::validate_app(&a).unwrap();
    let before = a.clone();
    assert!(
        !api::library_read(&a, "/api/v1/library/export", "").unwrap()["versions"]
            .as_object()
            .unwrap()
            .is_empty()
    );
    assert_eq!(archive_roundtrip(&a), a);
    assert_eq!(a, before);
}

#[test]
fn supplied_nato_manifest_is_exactly_26_canonical_forward_pairs() {
    let data: Value = serde_json::from_str(include_str!("../examples/nato-alphabet.json")).unwrap();
    let mut a = app();
    let id = add(&mut a, "mapping-nato-canonical-operation", data).unwrap();
    let v = &a.library.versions[&id];
    library::validate_mapping(v).unwrap();
    let pairs = v
        .sections
        .iter()
        .flat_map(|s| &s.units)
        .map(|u| library::mapping_pair(&u.text).unwrap())
        .collect::<Vec<_>>();
    assert_eq!(
        pairs.iter().map(|(key, _)| *key).collect::<String>(),
        "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
    );
    assert_eq!(
        pairs.iter().map(|(_, value)| *value).collect::<Vec<_>>(),
        vec![
            "Alfa", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel", "India",
            "Juliett", "Kilo", "Lima", "Mike", "November", "Oscar", "Papa", "Quebec", "Romeo",
            "Sierra", "Tango", "Uniform", "Victor", "Whiskey", "X-ray", "Yankee", "Zulu"
        ]
    );
    assert!(v.spelling_strict);
    assert_eq!(v.sources.len(), 3);
    assert_eq!(archive_roundtrip(&a), a);
}

#[test]
fn explicit_mapping_unit_boundaries_must_be_delimited() {
    let mut a = app();
    let before = a.clone();
    let data = json!({"title":"Ambiguous pairs","kind":"keyed_mapping","text":"A\tAlfaB\tBravo", "sections":[{"heading":"Pairs","units":["A\tAlfa","B\tBravo"]}]});
    assert_eq!(
        mutate(
            &mut a,
            "/api/v1/library/versions",
            "mapping-unit-boundaries-operation",
            data
        )
        .unwrap_err()
        .status,
        422
    );
    assert_eq!(a, before);
}

#[test]
fn compiled_cli_mapping_add_transmits_manifest_and_stable_operation_id() {
    use std::{
        io::{Read, Write},
        net::TcpListener,
        os::unix::fs::PermissionsExt,
        process::Command,
    };
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let origin = format!("http://{}", listener.local_addr().unwrap());
    let root = std::env::temp_dir().join(format!(
        "scry-mapping-cli-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    ));
    std::fs::create_dir(&root).unwrap();
    let login = root.join("login.json");
    let token =
        scry::owner_token::issuance_secret("abcdef0123456789abcdef0123456789abcdef0123456789");
    std::fs::write(&login, json!({"origin":origin,"token":token}).to_string()).unwrap();
    std::fs::set_permissions(&login, std::fs::Permissions::from_mode(0o600)).unwrap();
    let input = root.join("mapping.json");
    std::fs::write(&input, manifest().to_string()).unwrap();
    let server = std::thread::spawn(move || {
        let (mut stream, _) = listener.accept().unwrap();
        stream
            .set_read_timeout(Some(std::time::Duration::from_secs(10)))
            .unwrap();
        let mut received = vec![];
        let mut chunk = [0; 4096];
        let body = loop {
            let n = stream.read(&mut chunk).unwrap();
            assert!(n > 0);
            received.extend_from_slice(&chunk[..n]);
            if let Some(end) = received.windows(4).position(|bytes| bytes == b"\r\n\r\n") {
                let headers = String::from_utf8_lossy(&received[..end]);
                assert!(headers.starts_with("POST /api/v1/library/mappings HTTP/1.1"));
                let size = headers
                    .lines()
                    .find_map(|line| {
                        line.to_ascii_lowercase()
                            .strip_prefix("content-length:")
                            .and_then(|n| n.trim().parse::<usize>().ok())
                    })
                    .unwrap();
                if received.len() >= end + 4 + size {
                    break serde_json::from_slice::<Value>(&received[end + 4..end + 4 + size])
                        .unwrap();
                }
            }
        };
        let response =
            r#"{"result":{"entity_id":"0123456789abcdef0123456789abcdef"},"replayed":false}"#;
        write!(stream,"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",response.len(),response).unwrap();
        body
    });
    let result = Command::new(env!("CARGO_BIN_EXE_scry"))
        .args([
            "--config",
            login.to_str().unwrap(),
            "mapping",
            "add",
            "--file",
            input.to_str().unwrap(),
            "--operation-id",
            "mapping-cli-stable-operation",
            "--json",
        ])
        .output()
        .unwrap();
    assert!(
        result.status.success(),
        "CLI error: {}",
        String::from_utf8_lossy(&result.stdout)
    );
    let body = server.join().unwrap();
    assert_eq!(body["operation_id"], "mapping-cli-stable-operation");
    assert_eq!(body["data"]["pairs"], manifest()["pairs"]);
    assert_eq!(body["data"]["direction"], "forward");
    assert!(
        serde_json::from_slice::<Value>(&result.stdout).unwrap()["result"]["entity_id"].is_string()
    );
    std::fs::remove_dir_all(root).unwrap();
}

#[test]
fn mapping_identity_uses_punctuation_boundaries_without_rewriting_legacy_hash() {
    let mut a = app();
    let mut m = manifest();
    m["pairs"] = json!([{"key":"A","value":"Alfa"}]);
    add(&mut a, "mapping-punctuation-original", m.clone()).unwrap();
    let before = a.clone();
    m["pairs"] = json!([{"key":"A","value":"A.l.f.a"}]);
    assert_eq!(
        add(&mut a, "mapping-punctuation-collision", m)
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(a, before);
    let mut b = app();
    let mut m = manifest();
    m["title"] = json!("Distinct normalized keys");
    m["pairs"] = json!([{"key":"a.b","value":"First"},{"key":"ab","value":"Second"}]);
    let id = add(&mut b, "mapping-distinct-key-boundaries", m).unwrap();
    assert_eq!(b.library.versions[&id].sections[0].units.len(), 2);
    assert_eq!(archive_roundtrip(&b), b);
}
