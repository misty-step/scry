use scry::{engine, model::App, persistence};
use std::{collections::BTreeMap, time::Instant};

/// Focused NATO practice at its first Quiz: the letter is studied, then tried from memory.
fn quizzed(now: i64) -> App {
    let mut app = App::new("synthetic-csrf-at-least-24-characters".into());
    let start = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "capacity-start-00001".into()),
    ]);
    engine::mutate(&mut app, "/practice/nato/start", &start, now, "capacity").unwrap();
    let quiz = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "capacity-quiz-000001".into()),
        (
            "prompt_id".into(),
            app.practice.current.as_ref().unwrap().prompt.id.clone(),
        ),
    ]);
    engine::mutate(&mut app, "/practice/quiz", &quiz, now, "capacity-quiz").unwrap();
    app
}

#[test]
#[ignore = "synthetic capacity measurement"]
fn measure_checkpoint_receipt_growth() {
    let now = 1_800_000_000_000;
    let mut app = quizzed(now);
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

/// One automatic save as the Worker commits it: room is made, then the save.
fn autosave(app: &mut App, prompt: &str, n: usize, answer: &str) -> scry::model::AppResult<()> {
    let generation = app
        .practice
        .answer_checkpoint
        .as_ref()
        .map_or(0, |c| c.generation);
    let fields = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), format!("autosave-{n:032x}")),
        ("prompt_id".into(), prompt.to_string()),
        ("generation".into(), generation.to_string()),
        ("answer".into(), answer.to_string()),
    ]);
    scry::study::make_room_for_autosave(app);
    engine::mutate(
        app,
        "/practice/draft",
        &fields,
        1_800_000_000_000,
        "capacity",
    )
    .map(|_| ())
}

fn stored_bytes(app: &App) -> usize {
    persistence::rows(app)
        .unwrap()
        .iter()
        .map(|(k, v)| k.len() + v.len())
        .sum()
}

/// SPEC § Drafts and interruptions: automatic saving pauses only when Scry
/// refuses it for capacity. A checkpoint replaces its predecessor, so a
/// notebook past the old lifetime budget of 512 still saves, and its stored
/// size does not grow with repeated saves of one Prompt.
#[test]
fn automatic_saving_has_no_lifetime_budget_and_stays_bounded() {
    use scry::study::AUTOSAVE_RECEIPT_LIMIT;
    let now = 1_800_000_000_000;
    let mut app = quizzed(now);
    let prompt = app.practice.current.as_ref().unwrap().prompt.id.clone();
    let automatic = |app: &App| {
        app.operations
            .keys()
            .filter(|id| id.starts_with("autosave-"))
            .count()
    };
    // Below the limit nothing is dropped: every receipt is kept as before.
    for n in 0..AUTOSAVE_RECEIPT_LIMIT {
        autosave(&mut app, &prompt, n, "synthetic").unwrap();
    }
    assert_eq!(automatic(&app), AUTOSAVE_RECEIPT_LIMIT);
    // Past it, every save still commits, and the receipts stay at the limit.
    let mut sizes = Vec::new();
    for n in AUTOSAVE_RECEIPT_LIMIT..AUTOSAVE_RECEIPT_LIMIT * 4 {
        autosave(&mut app, &prompt, n, "synthetic").unwrap();
        assert_eq!(automatic(&app), AUTOSAVE_RECEIPT_LIMIT);
        if n == AUTOSAVE_RECEIPT_LIMIT * 3 || n == AUTOSAVE_RECEIPT_LIMIT * 4 - 1 {
            sizes.push(stored_bytes(&app));
        }
    }
    let checkpoint = app.practice.answer_checkpoint.as_ref().unwrap();
    assert_eq!(checkpoint.generation, (AUTOSAVE_RECEIPT_LIMIT * 4) as u64);
    assert_eq!(checkpoint.draft.as_ref().unwrap().answer, "synthetic");
    // From save 1537 on, every retained generation has four digits: another
    // 511 saves of the same Prompt leave the stored size exactly as it was.
    assert_eq!(sizes[0], sizes[1], "stored bytes grew with repeated saves");
    engine::validate_app(&app).unwrap();
    let restored = persistence::app_from_rows(&persistence::rows(&app).unwrap()).unwrap();
    assert_eq!(restored, app, "The bounded notebook reads back unchanged.");
    // A replay of a dropped save meets a newer generation: a conflict, never a
    // second write.
    let before = persistence::rows(&app).unwrap();
    let dropped = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), format!("autosave-{:032x}", 0)),
        ("prompt_id".into(), prompt.clone()),
        ("generation".into(), "0".into()),
        ("answer".into(), "synthetic".into()),
    ]);
    let refused =
        engine::mutate(&mut app, "/practice/draft", &dropped, now, "capacity").unwrap_err();
    assert_eq!(refused.status, 409);
    assert_eq!(persistence::rows(&app).unwrap(), before);
    // Deliberate saves keep their receipts and are never dropped.
    let manual = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "explicit-save-still-allowed".into()),
        ("prompt_id".into(), prompt.clone()),
        ("generation".into(), checkpoint_generation(&app).to_string()),
        ("answer".into(), "manual".into()),
    ]);
    engine::mutate(&mut app, "/practice/draft", &manual, now, "capacity").unwrap();
    autosave(&mut app, &prompt, AUTOSAVE_RECEIPT_LIMIT * 4, "synthetic").unwrap();
    assert!(app.operations.contains_key("explicit-save-still-allowed"));
}

fn checkpoint_generation(app: &App) -> u64 {
    app.practice.answer_checkpoint.as_ref().unwrap().generation
}

#[test]
fn receipt_generation_is_not_evidence_that_a_checkpoint_is_current() {
    use scry::study::checkpoint_current;
    let now = 1_800_000_000_000;
    let mut app = quizzed(now);
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
    let app = quizzed(now);
    let html = scry::web::render(&app, "/", &Default::default(), now, None);
    assert!(html.contains("data-checkpoint-discard data-checkpoint-present=\"false\""));
    assert!(html.contains("value=\"Discard saved draft\" hidden"));
}
