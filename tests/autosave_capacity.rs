use scry::{engine, model::App, persistence};
use std::{collections::BTreeMap, time::Instant};

#[test]
#[ignore = "synthetic capacity measurement"]
fn measure_checkpoint_receipt_growth() {
    let now = 1_800_000_000_000;
    let mut app = App::new("synthetic-csrf-at-least-24-characters".into());
    let start = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "capacity-start-00001".into()),
    ]);
    engine::mutate(&mut app, "/practice/nato/start", &start, now, "capacity").unwrap();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    for count in 0..=2000 {
        if count > 0 {
            let operation = format!("autosave-{count:08x}-0000-4000-8000-{count:012x}");
            let fields = BTreeMap::from([
                ("csrf".into(), app.csrf.clone()),
                ("operation_id".into(), operation),
                ("prompt_id".into(), prompt.clone()),
                ("generation".into(), (count - 1).to_string()),
                ("answer".into(), format!("synthetic answer {count}")),
            ]);
            engine::mutate(&mut app, "/practice/draft", &fields, now, "capacity").unwrap();
        }
        if [0, 1, 100, 500, 1000, 2000].contains(&count) {
            let start = Instant::now();
            let rows = persistence::rows(&app).unwrap();
            let row_time = start.elapsed();
            let total: usize = rows.iter().map(|(k, v)| k.len() + v.len()).sum();
            let start = Instant::now();
            let archive = persistence::Archive::new(app.clone(), vec![], now)
                .encode()
                .unwrap();
            println!(
                "saves={count} operations={} rows_bytes={total} archive_bytes={} rows_us={} archive_us={}",
                app.operations.len(),
                archive.len(),
                row_time.as_micros(),
                start.elapsed().as_micros()
            );
        }
    }
}

#[test]
fn automatic_receipts_have_a_notebook_wide_budget_without_changing_manual_saves() {
    use scry::study::{AUTOSAVE_RECEIPT_LIMIT, autosave_remaining};
    let now = 1_800_000_000_000;
    let mut app = App::new("synthetic-csrf-at-least-24-characters".into());
    let start = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "capacity-start-00001".into()),
    ]);
    engine::mutate(&mut app, "/practice/nato/start", &start, now, "capacity").unwrap();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    for i in 0..AUTOSAVE_RECEIPT_LIMIT {
        let fields = BTreeMap::from([
            ("csrf".into(), app.csrf.clone()),
            ("operation_id".into(), format!("autosave-{i:032x}")),
            ("prompt_id".into(), prompt.clone()),
            ("generation".into(), i.to_string()),
            ("answer".into(), "synthetic".into()),
        ]);
        engine::mutate(&mut app, "/practice/draft", &fields, now, "capacity").unwrap();
    }
    assert_eq!(autosave_remaining(&app), 0);
    let restored = persistence::app_from_rows(&persistence::rows(&app).unwrap()).unwrap();
    assert_eq!(
        autosave_remaining(&restored),
        0,
        "The budget survives reloads and is shared by tabs."
    );
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "explicit-save-still-allowed".into()),
        ("prompt_id".into(), prompt),
        ("generation".into(), AUTOSAVE_RECEIPT_LIMIT.to_string()),
        ("answer".into(), "manual".into()),
    ]);
    engine::mutate(&mut app, "/practice/draft", &fields, now, "capacity").unwrap();
    assert_eq!(autosave_remaining(&app), 0);
}

#[test]
fn receipt_generation_is_not_evidence_that_a_checkpoint_is_current() {
    use scry::study::checkpoint_current;
    let now = 1_800_000_000_000;
    let mut app = App::new("synthetic-csrf-at-least-24-characters".into());
    let start = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "capacity-start-00001".into()),
    ]);
    engine::mutate(&mut app, "/practice/nato/start", &start, now, "capacity").unwrap();
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "autosave-0000000000000001".into()),
        ("prompt_id".into(), prompt.clone()),
        ("generation".into(), "0".into()),
        ("answer".into(), "Al".into()),
    ]);
    engine::mutate(&mut app, "/practice/draft", &fields, now, "capacity").unwrap();
    assert!(checkpoint_current(&app, &fields, 1));
    let answer = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "capacity-answer-00001".into()),
        ("prompt_id".into(), prompt),
        ("answer".into(), "Alfa".into()),
    ]);
    engine::mutate(&mut app, "/practice/answer", &answer, now, "capacity").unwrap();
    assert_eq!(
        app.practice.answer_checkpoint.as_ref().unwrap().generation,
        1
    );
    assert!(!checkpoint_current(&app, &fields, 1));
}

#[test]
fn discard_control_starts_hidden_until_a_checkpoint_exists() {
    let now = 1_800_000_000_000;
    let mut app = App::new("synthetic-csrf-at-least-24-characters".into());
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "autosave-control-start".into()),
    ]);
    engine::mutate(&mut app, "/practice/nato/start", &fields, now, "capacity").unwrap();
    let html = scry::web::render(&app, "/", &Default::default(), now, None);
    assert!(html.contains("data-checkpoint-discard data-checkpoint-present=\"false\""));
    assert!(html.contains("value=\"Discard saved draft\" hidden"));
}
