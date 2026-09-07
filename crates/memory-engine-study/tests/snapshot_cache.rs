use std::sync::atomic::{AtomicUsize, Ordering};

use memory_engine_persistence::BetaPersistenceStore;
use memory_engine_study::{BetaStudySession, BetaStudySourceInput};

const NOW: i64 = 1_779_984_000_000;

#[test]
fn source_write_refreshes_cached_view() {
    let directory = tempfile_dir();
    let path = directory.join("study.json");
    let store = BetaPersistenceStore::open(&path).expect("store");
    let mut study = BetaStudySession::from_store(store, || NOW);

    assert!(study.view().expect("initial view").sources.is_empty());

    study
        .add_source(BetaStudySourceInput::from_capture(
            "src-note",
            "ALFA is the NATO phonetic alphabet word for A.",
        ))
        .expect("source");

    let refreshed = study.view().expect("view after source write");
    assert_eq!(refreshed.summary.source_count, 1);
    assert_eq!(
        refreshed
            .sources
            .iter()
            .map(|source| source.id.as_str())
            .collect::<Vec<_>>(),
        ["src-note"]
    );
}

static TEMP_COUNTER: AtomicUsize = AtomicUsize::new(0);
fn tempfile_dir() -> std::path::PathBuf {
    let serial = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
    let path = std::env::temp_dir().join(format!(
        "memory-engine-snapshot-cache-{}-{serial}",
        std::process::id()
    ));
    let _ = std::fs::remove_dir_all(&path);
    std::fs::create_dir_all(&path).expect("temp directory");
    path
}
