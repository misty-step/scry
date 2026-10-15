use scry::model::BackupStatus;

#[test]
fn health_requires_a_verified_snapshot_within_26_hours() {
    let now = 2_000_000_000_000;
    let limit = 26 * 60 * 60_000;
    for (completed_ms, fresh) in [
        (None, false),
        (Some(now), true),
        (Some(now - limit), true),
        (Some(now - limit - 1), false),
        (Some(now + 1), false),
        (Some(i64::MIN), false),
    ] {
        let backup = BackupStatus {
            completed_ms,
            ..Default::default()
        };
        assert_eq!(backup.fresh(now), fresh, "{completed_ms:?}");
    }
}

#[test]
fn a_failed_attempt_does_not_refresh_the_last_verified_snapshot() {
    let now = 2_000_000_000_000;
    let mut backup = BackupStatus {
        completed_ms: Some(now - 27 * 60 * 60_000),
        error: Some("A snapshot could not be verified.".into()),
        ..Default::default()
    };
    assert!(!backup.fresh(now));
    backup.completed_ms = Some(now);
    backup.error = None;
    assert!(backup.fresh(now));
}
