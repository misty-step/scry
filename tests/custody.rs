//! Custody of the Owner's notebook: storage capacity, restore of retired
//! generation records, backup status in Settings, a paused restored notebook
//! and the expiry an Owner token may grant. SPEC § Backup and recovery,
//! § Retired generation records and § Owner tokens.
mod common;

use scry::{
    engine::{self, WorkConfig},
    model::*,
    owner_token::{MutationActor, OwnerToken},
    persistence, web,
};
use serde_json::json;
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;
const MINUTE: i64 = 60_000;
const CREED: &str = "I believe in one God,\nthe Father almighty,\nmaker of heaven and earth,\nof all things visible and invisible.\n";

struct Owner {
    app: App,
    n: u64,
    now: i64,
    jev: bool,
}
impl Owner {
    fn new() -> Self {
        Self {
            app: App::new("custody-synthetic-csrf-0123456789".into()),
            n: 0,
            now: NOW,
            jev: false,
        }
    }
    fn fields(&mut self, pairs: &[(&str, &str)]) -> BTreeMap<String, String> {
        self.n += 1;
        let mut f = BTreeMap::from([
            ("csrf".to_string(), self.app.csrf.clone()),
            (
                "operation_id".to_string(),
                format!("custody-operation-{:06}", self.n),
            ),
        ]);
        f.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.to_string())));
        f
    }
    fn try_as(
        &mut self,
        path: &str,
        pairs: &[(&str, &str)],
        actor: MutationActor<'_>,
    ) -> AppResult<MutationResult> {
        let f = self.fields(pairs);
        let seed = format!("{:048x}", self.n);
        engine::mutate_judged(&mut self.app, path, &f, self.now, &seed, actor, self.jev)
    }
    fn call(&mut self, path: &str, pairs: &[(&str, &str)]) -> MutationResult {
        self.try_as(path, pairs, MutationActor::Browser)
            .unwrap_or_else(|e| panic!("{path}: {} {}", e.status, e.message))
    }
    fn held(&self) -> String {
        self.app
            .practice
            .current
            .as_ref()
            .unwrap()
            .prompt
            .id
            .clone()
    }
    fn text(mut self) -> Self {
        let vid = self
            .call(
                "/library/version/add",
                &[(
                    "data",
                    &json!({"title":"Synthetic Creed","name":"Authored wording","kind":"text","text":CREED,"spelling_strict":false}).to_string(),
                )],
            )
            .entity_id
            .unwrap();
        self.call("/practice/lesson/start", &[("version_id", &vid)]);
        let pid = self.held();
        self.call("/practice/quiz", &[("prompt_id", &pid)]);
        self
    }
    fn page(&self, path: &str, extra: &[(&str, &str)]) -> String {
        let mut query = BTreeMap::from([("__nonce".to_string(), "custody-nonce".to_string())]);
        query.extend(extra.iter().map(|(k, v)| (k.to_string(), v.to_string())));
        web::render(&self.app, path, &query, self.now, None)
    }
}

/// Test-owned receipts that each fit a row and together approach the
/// archive bound, leaving `room` bytes, as the Worker capacity tests do.
fn fill(app: &mut App, room: usize) {
    fill_to(app, persistence::MAX_ARCHIVE_BYTES, room);
}
/// Test-owned receipts that bring the archive to within `room` bytes of `limit`.
fn fill_to(app: &mut App, limit: usize, room: usize) {
    let mut i = 0;
    loop {
        let size = persistence::archive_size(app).unwrap();
        let left = limit - size;
        if left <= room + 2_000 {
            break;
        }
        let pad = (left - room - 1_000).min(900_000);
        app.operations.insert(
            format!("custody-capacity-{i:04}"),
            Receipt {
                acting_token: None,
                path: None,
                created_ms: None,
                payload_hash: "a".repeat(64),
                result: MutationResult {
                    data: Some(json!({"synthetic": "x".repeat(pad)})),
                    ..MutationResult::default()
                },
            },
        );
        i += 1;
    }
    assert!(persistence::archive_size(app).is_ok());
}

// --- Item 1: one capacity rule ------------------------------------------------

#[test]
fn growth_past_capacity_is_refused_by_one_rule_and_revocation_is_not() {
    for client in [persistence::Client::Browser, persistence::Client::Api] {
        let mut o = Owner::new().text();
        let token = OwnerToken::issue("custody agent", None, false, NOW, &"c".repeat(48)).unwrap();
        let token_id = token.info.id.clone();
        o.app.owner_tokens.insert(token_id.clone(), token);
        fill(&mut o.app, 600);
        let before = o.app.clone();
        let pid = o.held();
        // An answer grows the notebook past one complete archive.
        o.call(
            "/practice/answer",
            &[("prompt_id", &pid), ("answer", "I believe in one God")],
        );
        let error = persistence::admit_growth("/practice/answer", before.revision, &o.app, client)
            .unwrap_err();
        assert_eq!(error.status, 413);
        assert_eq!(error.message, persistence::CAPACITY_REFUSAL);
        // Nothing changed is nothing refused: a replay returns its committed result.
        assert!(
            persistence::admit_growth("/practice/answer", before.revision, &before, client).is_ok()
        );
        // Access can always be withdrawn, whatever storage holds.
        o.app = before.clone();
        o.call("/tokens/revoke", &[("token_id", &token_id)]);
        assert!(o.app.owner_tokens[&token_id].info.revoked_ms.is_some());
        assert!(
            persistence::admit_growth("/tokens/revoke", before.revision, &o.app, client).is_ok()
        );
        // Well within capacity the same answer is admitted.
        let mut roomy = Owner::new().text();
        let start = roomy.app.revision;
        let pid = roomy.held();
        roomy.call(
            "/practice/answer",
            &[("prompt_id", &pid), ("answer", "I believe")],
        );
        assert!(persistence::admit_growth("/practice/answer", start, &roomy.app, client).is_ok());
    }
}

// --- Review 1: only size refuses -----------------------------------------------

/// A notebook whose archive cannot be built for a reason other than its size:
/// one stored photo larger than the photo limit, or two Goals naming the same
/// photo with different records.
fn unsizable(kind: &str) -> App {
    let (mut app, _) = common::earlier("history-rich");
    let mut goals = app.goals.values().filter(|g| g.photo.is_some()).cloned();
    let mut goal = goals.next().expect("a saved photo");
    match kind {
        "oversized" => goal.photo.as_mut().unwrap().size = scry::model::PHOTO_LIMIT + 1,
        _ => {
            let mut other = app
                .goals
                .values()
                .find(|g| g.photo.is_none())
                .unwrap()
                .clone();
            let mut photo = goal.photo.clone().unwrap();
            photo.sha256 = "0".repeat(64);
            other.photo = Some(photo);
            app.goals.insert(other.id.clone(), other);
        }
    }
    app.goals.insert(goal.id.clone(), goal);
    app
}

#[test]
fn a_notebook_that_cannot_be_measured_for_another_reason_is_answered_as_before() {
    for kind in ["oversized", "conflicting"] {
        let app = unsizable(kind);
        let reason = persistence::archive_size(&app).unwrap_err();
        assert_ne!(reason.message, persistence::CAPACITY_REFUSAL);
        assert!(matches!(
            persistence::footprint(&app),
            persistence::Footprint::Unsizable(_)
        ));
        let mut changed = app.clone();
        changed.revision += 1;

        // Writes the browser never checked before stay admitted.
        for path in [
            "/practice/answer",
            "/practice/next",
            "/practice/quiz",
            "/practice/reveal",
            "/settings/backup",
            "/settings/judgment",
            "/settings/resume",
            "/tokens/create",
            "/tokens/revoke",
        ] {
            assert!(
                persistence::admit_growth(
                    path,
                    app.revision,
                    &changed,
                    persistence::Client::Browser
                )
                .is_ok(),
                "{kind}: {path}"
            );
        }
        // The browser paths checked before keep that check and its reason.
        for path in [
            "/practice/draft",
            "/practice/draft/discard",
            "/practice/nato/start",
            "/practice/lesson/start",
            "/library/version/add",
            "/library/mapping/add",
            "/library/work/edit",
        ] {
            let error = persistence::admit_growth(
                path,
                app.revision,
                &changed,
                persistence::Client::Browser,
            )
            .unwrap_err();
            assert_eq!(
                (error.status, &error.message),
                (reason.status, &reason.message),
                "{kind}: {path}"
            );
        }
        // The API checked every mutation before, a replay included.
        for (after, path) in [(&changed, "/practice/answer"), (&app, "/practice/answer")] {
            let error =
                persistence::admit_growth(path, app.revision, after, persistence::Client::Api)
                    .unwrap_err();
            assert_eq!(error.message, reason.message, "{kind}: API");
        }
        // Export says nothing about storage being full.
        assert!(
            !Owner {
                app: app.clone(),
                n: 0,
                now: NOW,
                jev: false
            }
            .page("/settings", &[])
            .contains("data-capacity")
        );
    }
}

// --- One limit: 16 MiB --------------------------------------------------------

/// A small receipt, standing in for what any write adds.
fn grow(app: &App, bytes: usize) -> App {
    let mut next = app.clone();
    next.revision += 1;
    next.operations.insert(
        format!("custody-growth-{}", next.revision),
        Receipt {
            acting_token: None,
            path: None,
            created_ms: None,
            payload_hash: "a".repeat(64),
            result: MutationResult {
                data: Some(json!({"synthetic": "x".repeat(bytes)})),
                ..MutationResult::default()
            },
        },
    );
    next
}
/// Exactly `limit - room` bytes of complete archive.
fn exactly(room_below: usize, limit: usize) -> App {
    let mut o = Owner::new().text();
    fill_to(&mut o.app, limit, room_below);
    let gap = limit - room_below - persistence::archive_size(&o.app).unwrap();
    let pad = o.app.operations["custody-capacity-0000"]
        .result
        .data
        .as_ref()
        .unwrap()["synthetic"]
        .as_str()
        .unwrap()
        .len();
    o.app
        .operations
        .get_mut("custody-capacity-0000")
        .unwrap()
        .result
        .data = Some(json!({"synthetic": "x".repeat(pad + gap)}));
    assert_eq!(
        persistence::archive_size(&o.app).unwrap(),
        limit - room_below
    );
    o.app
}
#[test]
fn ordinary_growth_is_refused_only_past_the_full_limit() {
    let mib = 1024 * 1024;
    for client in [persistence::Client::Browser, persistence::Client::Api] {
        // 13 MiB: every write that fits is admitted.
        let large = exactly(
            persistence::MAX_ARCHIVE_BYTES - 13 * mib,
            persistence::MAX_ARCHIVE_BYTES,
        );
        for path in [
            "/practice/answer",
            "/practice/next",
            "/library/version/add",
            "/tokens/create",
            "/settings/backup",
        ] {
            assert!(
                persistence::admit_growth(path, large.revision, &grow(&large, 5_000), client)
                    .is_ok(),
                "{path}"
            );
        }
        // 15.99 MiB: a write that would pass the limit is refused, one that fits is not.
        let near = exactly(
            persistence::MAX_ARCHIVE_BYTES - (15 * mib + 99 * mib / 100),
            persistence::MAX_ARCHIVE_BYTES,
        );
        let room = persistence::MAX_ARCHIVE_BYTES - persistence::archive_size(&near).unwrap();
        for path in [
            "/practice/answer",
            "/settings/backup",
            "/settings/resume",
            "/practice/drop",
        ] {
            let error =
                persistence::admit_growth(path, near.revision, &grow(&near, room + 1_000), client)
                    .unwrap_err();
            assert_eq!(
                (error.status, error.message.as_str()),
                (413, persistence::CAPACITY_REFUSAL),
                "{path}"
            );
        }
        assert!(
            persistence::admit_growth(
                "/practice/answer",
                near.revision,
                &grow(&near, 1_000),
                client
            )
            .is_ok()
        );
        // Revocation never is.
        assert!(
            persistence::admit_growth(
                "/tokens/revoke",
                near.revision,
                &grow(&near, room + 1_000),
                client
            )
            .is_ok()
        );
    }
    // Settings: nothing under three quarters; over it, the size, quietly.
    let o = |app: App| Owner {
        app,
        n: 0,
        now: NOW,
        jev: false,
    };
    assert_eq!(persistence::SIZE_NOTICE_BYTES, 12 * mib);
    let under = o(exactly(1_000, persistence::SIZE_NOTICE_BYTES)).page("/settings", &[]);
    assert!(!under.contains("data-capacity"));
    let large = o(exactly(
        persistence::MAX_ARCHIVE_BYTES - 13 * mib,
        persistence::MAX_ARCHIVE_BYTES,
    ))
    .page("/settings", &[]);
    assert!(large.contains("<p class=\"fineprint\" data-capacity=\"large\">The notebook uses 13.0 MiB of the 16 MiB one complete backup can hold.</p>"));
    assert!(!large.contains("Storage is full"));
}

// The edge this change leaves, pinned as it is today so that a later reserve
// has a failing-first test to flip: a write that never asks the rule carries a
// notebook a few bytes under the limit past it.
#[test]
fn today_a_failed_backup_can_carry_a_notebook_just_under_the_limit_past_it() {
    let edge = exactly(20, persistence::MAX_ARCHIVE_BYTES);
    assert!(matches!(
        persistence::footprint(&edge),
        persistence::Footprint::Fits(_)
    ));
    // A failed backup records its reason without asking the rule.
    let mut failed = edge.clone();
    failed.backup.error = Some(FAILED.into());
    assert!(matches!(
        persistence::footprint(&failed),
        persistence::Footprint::TooLarge
    ));
    assert_eq!(persistence::archive_size(&failed).unwrap_err().status, 413);
    // Every growing write is now refused; nothing was deleted.
    let error = persistence::admit_growth(
        "/practice/answer",
        failed.revision,
        &grow(&failed, 10),
        persistence::Client::Browser,
    )
    .unwrap_err();
    assert_eq!(error.message, persistence::CAPACITY_REFUSAL);
    assert_eq!(failed.operations.len(), edge.operations.len());
    let settings = main_of(
        &Owner {
            app: failed,
            n: 0,
            now: NOW,
            jev: false,
        }
        .page("/settings", &[]),
    );
    assert!(settings.contains("<p data-backup=\"failed\">"));
    assert!(settings.contains("<p data-capacity=\"full\"><strong>Storage is full.</strong>"));
}

#[test]
fn a_notebook_past_capacity_reads_and_says_so_without_losing_anything() {
    let mut o = Owner::new().text();
    fill(&mut o.app, 0);
    o.app.operations.insert(
        "custody-capacity-over".into(),
        Receipt {
            acting_token: None,
            path: None,
            created_ms: None,
            payload_hash: "a".repeat(64),
            result: MutationResult {
                data: Some(json!({"synthetic": "x".repeat(10_000)})),
                ..MutationResult::default()
            },
        },
    );
    assert_eq!(persistence::archive_size(&o.app).unwrap_err().status, 413);
    let rows = persistence::rows(&o.app).unwrap();
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), o.app);
    let settings = o.page("/settings", &[]);
    assert!(settings.contains("data-capacity=\"full\""));
    assert!(settings.contains("<strong>Storage is full.</strong>"));
    assert!(settings.contains("Nothing has been deleted."));
    for path in ["/", "/history", "/library"] {
        assert!(o.page(path, &[]).contains("<main"), "{path}");
    }
    // Within capacity Settings says nothing about storage.
    assert!(
        !Owner::new()
            .page("/settings", &[])
            .contains("data-capacity")
    );
}

// --- Item 2: restore leaves retired generation records unchanged -------------

fn generation() -> WorkConfig {
    WorkConfig {
        generation: true,
        jev: true,
        critic: true,
        model: "synthetic-generator".into(),
        jev_model: "synthetic-grader".into(),
    }
}
fn job_rows(app: &App) -> BTreeMap<String, String> {
    persistence::rows(app)
        .unwrap()
        .into_iter()
        .filter(|(key, _)| key.starts_with("jobs/"))
        .collect()
}
/// A restore as the runtime makes one: through a complete archive's bytes.
fn restored(app: &App) -> App {
    let bytes = persistence::Archive::new(app.clone(), vec![], NOW)
        .encode()
        .unwrap();
    let mut archive = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    engine::pause_restored(&mut archive.app, NOW + 5).unwrap();
    engine::validate_app(&archive.app).unwrap();
    let again = persistence::Archive::new(archive.app.clone(), vec![], NOW + 5)
        .encode()
        .unwrap();
    persistence::Archive::decode(&again, &persistence::sha256(&again))
        .unwrap()
        .app
}

#[test]
fn a_restore_leaves_every_retired_generation_job_byte_for_byte() {
    // Every non-terminal state a generation job was saved in.
    let mut app = App::new("custody-restore-csrf-at-least-24".into());
    let f = BTreeMap::from([
        ("csrf".into(), app.csrf.clone()),
        ("operation_id".into(), "custody-historical-create".into()),
        ("intent".into(), "Historical saved request".into()),
    ]);
    engine::mutate(&mut app, "/create", &f, NOW, "custody-old-create").unwrap();
    let mut states = vec![("queued", app.clone())];
    let generated = engine::claim_work(&mut app, NOW, "custody-generation-lease", &generation())
        .unwrap()
        .unwrap();
    states.push(("sent", app.clone()));
    let (fixture, _) = common::earlier("question-cold-choice");
    let batch = fixture
        .jobs
        .values()
        .next()
        .unwrap()
        .candidate_json
        .clone()
        .unwrap();
    let raw = json!({"choices":[{"message":{"content":batch}}]}).to_string();
    engine::finish_work(
        &mut app,
        &generated,
        raw,
        "synthetic-generator".into(),
        Some(10_000),
        NOW + 1,
    )
    .unwrap();
    states.push(("candidates", app.clone()));
    engine::claim_work(&mut app, NOW + 2, "custody-critic-lease0001", &generation())
        .unwrap()
        .unwrap();
    states.push(("critic-sent", app.clone()));
    for (status, saved) in states {
        assert!(
            saved.jobs.values().any(|j| j.status == status),
            "{status} was not constructed"
        );
        engine::validate_app(&saved).unwrap();
        let after = restored(&saved);
        assert!(after.restored_paused, "{status}");
        assert_eq!(
            job_rows(&after),
            job_rows(&saved),
            "{status}: a job was rewritten"
        );
        assert_eq!(after.jobs, saved.jobs, "{status}");
        assert_eq!(after.spend, saved.spend, "{status}: usage is kept");
        // Resuming leaves them as stored too, and nothing claims them.
        let mut resumed = after.clone();
        engine::resume_work(&mut resumed).unwrap();
        assert!(
            engine::claim_check_work(
                &mut resumed,
                NOW + 10 * MINUTE,
                "custody-check",
                &generation()
            )
            .unwrap()
            .is_none()
        );
        assert!(!engine::recover_checks(&mut resumed, NOW + 10 * MINUTE));
        assert_eq!(resumed.jobs, saved.jobs, "{status}: resume");
    }
}

#[test]
fn a_restore_of_every_committed_fixture_keeps_its_generation_jobs() {
    let fixture: serde_json::Value = serde_json::from_str(
        &std::fs::read_to_string(concat!(
            env!("CARGO_MANIFEST_DIR"),
            "/tests/fixtures/earlier-notebook.json"
        ))
        .unwrap(),
    )
    .unwrap();
    for name in fixture.as_object().unwrap().keys() {
        // Some hold photos, which live outside the archive's rows: pause the
        // decoded notebook as the restore does after decoding it.
        let (app, now) = common::earlier(name);
        let mut after = app.clone();
        engine::pause_restored(&mut after, now + 1).unwrap();
        engine::validate_app(&after).unwrap();
        assert_eq!(job_rows(&after), job_rows(&app), "{name}");
        assert_eq!(after.jobs, app.jobs, "{name}");
    }
}

#[test]
fn a_restore_still_pauses_uncertain_practice_checks() {
    let mut o = Owner::new().text();
    o.jev = true;
    o.app.preferences.paid_judgment = true;
    let pid = o.held();
    let aid = o
        .call(
            "/practice/answer",
            &[("prompt_id", &pid), ("answer", "I believe in a single God")],
        )
        .entity_id
        .unwrap();
    let check = |app: &App| {
        app.practice
            .checks
            .values()
            .find(|c| c.attempt_id == aid)
            .unwrap()
            .clone()
    };
    assert_eq!(check(&o.app).status, scry::study::CheckStatus::Queued);
    let queued = o.app.clone();
    let after = restored(&queued);
    assert_eq!(check(&after).status, scry::study::CheckStatus::Refused);
    let config = generation();
    let mut sent = queued.clone();
    engine::claim_check_work(&mut sent, NOW, "custody-sent-check-lease", &config)
        .unwrap()
        .unwrap();
    assert_eq!(check(&sent).status, scry::study::CheckStatus::Sent);
    let after = restored(&sent);
    assert_eq!(check(&after).status, scry::study::CheckStatus::Unknown);
    assert_eq!(after.spend, sent.spend);
}

// --- Item 4: a failed backup is visibly failed ---------------------------------

const FAILED: &str = "The complete snapshot could not be verified. Review remains available; inspect operator recovery configuration.";

fn main_of(html: &str) -> String {
    // Everything outside closed disclosures.
    let mut out = String::new();
    let mut rest = html;
    while let Some(start) = rest.find("<details") {
        out.push_str(&rest[..start]);
        let end = rest[start..].find("</details>").unwrap() + start + "</details>".len();
        rest = &rest[end..];
    }
    out.push_str(rest);
    out
}

#[test]
fn settings_leads_with_a_failed_backup_outside_any_disclosure() {
    let mut o = Owner::new();
    o.app.backup = BackupStatus {
        completed_ms: Some(NOW - 2 * DAY_MS),
        key: Some("snapshots/synthetic/one.json".into()),
        sha256: Some("a".repeat(64)),
        error: Some(FAILED.into()),
    };
    let settings = main_of(&o.page("/settings", &[]));
    assert!(settings.contains(
        "<p data-backup=\"failed\"><strong>The last backup failed.</strong> Last verified backup: "
    ));
    assert!(!settings.contains("A fresh backup needs attention"));
    assert!(settings.contains("action=\"/settings/backup\""));
    o.app.backup = BackupStatus {
        error: Some(FAILED.into()),
        ..BackupStatus::default()
    };
    let settings = main_of(&o.page("/settings", &[]));
    assert!(settings.contains(
        "<p data-backup=\"failed\"><strong>The last backup failed.</strong> No verified backup exists.</p>"
    ));
    // A verified backup and no failure reads as before.
    o.app.backup = BackupStatus {
        completed_ms: Some(NOW - MINUTE),
        key: Some("snapshots/synthetic/two.json".into()),
        sha256: Some("b".repeat(64)),
        error: None,
    };
    let settings = o.page("/settings", &[]);
    assert!(settings.contains("<p>Last verified backup: "));
    assert!(!settings.contains("data-backup"));
}

// --- Item 3: Settings in the verification space --------------------------------

#[test]
fn settings_in_the_verification_space_says_backups_do_not_apply() {
    let o = Owner::new();
    let settings = o.page("/settings", &[("__verification", "true")]);
    assert!(settings.contains(
        "<p data-backup=\"not-applicable\">Backups do not apply to this verification space.</p>"
    ));
    assert!(!settings.contains("action=\"/settings/backup\""));
    assert!(!settings.contains("Recovery targets"));
    assert!(settings.contains("href=\"/export\""));
    // The Owner's Settings is unchanged by the flag's absence.
    assert!(
        o.page("/settings", &[])
            .contains("action=\"/settings/backup\"")
    );
}

// --- Item 5: a paused restored notebook points to Settings ---------------------

#[test]
fn practice_paused_by_a_restore_links_to_where_checks_resume() {
    let mut o = Owner::new().text();
    o.jev = true;
    o.app.preferences.paid_judgment = true;
    engine::pause_restored(&mut o.app, NOW).unwrap();
    let pid = o.held();
    o.call(
        "/practice/answer",
        &[("prompt_id", &pid), ("answer", "I believe in a single God")],
    );
    let page = o.page("/", &[]);
    assert!(page.contains("Checks are paused in this restored notebook."));
    assert!(page.contains(
        "Your answer is saved and not graded. <a href=\"/settings#preparation\">Resume checks in Settings</a></p>"
    ));
    assert!(!page.contains("action=\"/practice/check\""));
    assert!(o.page("/settings", &[]).contains("id=\"preparation\""));
    // Once resumed, practice no longer points there.
    o.call("/settings/resume", &[]);
    assert!(!o.page("/", &[]).contains("/settings#preparation"));
}

// --- Item 7: a token cannot outlive the token that creates it ------------------

#[test]
fn a_token_cannot_create_one_that_outlives_it() {
    let mut o = Owner::new();
    let limit = NOW + DAY_MS;
    let parent =
        OwnerToken::issue("expiring agent", Some(limit), true, NOW, &"d".repeat(48)).unwrap();
    let forever = OwnerToken::issue("lasting agent", None, true, NOW, &"e".repeat(48)).unwrap();
    o.app
        .owner_tokens
        .insert(parent.info.id.clone(), parent.clone());
    o.app
        .owner_tokens
        .insert(forever.info.id.clone(), forever.clone());
    let later = (limit + 1).to_string();
    let same = limit.to_string();
    let earlier = (limit - MINUTE).to_string();
    for expiry in [None, Some(later.as_str())] {
        let before = persistence::rows(&o.app).unwrap();
        let mut pairs = vec![("name", "child")];
        if let Some(t) = expiry {
            pairs.push(("expires_ms", t));
        }
        let error = o
            .try_as("/tokens/create", &pairs, MutationActor::Token(&parent.info))
            .unwrap_err();
        assert_eq!(error.status, 403, "{expiry:?}");
        assert!(
            error.message.contains("expire no later than it does"),
            "{expiry:?}"
        );
        assert_eq!(
            persistence::rows(&o.app).unwrap(),
            before,
            "{expiry:?}: a write"
        );
    }
    for expiry in [same.as_str(), earlier.as_str()] {
        let id = o
            .try_as(
                "/tokens/create",
                &[("name", "child"), ("expires_ms", expiry)],
                MutationActor::Token(&parent.info),
            )
            .unwrap()
            .entity_id
            .unwrap();
        assert_eq!(o.app.owner_tokens[&id].info.expires_ms, expiry.parse().ok());
    }
    // A token without an expiry, and the signed-in Owner, are unaffected.
    for actor in [MutationActor::Token(&forever.info), MutationActor::Browser] {
        let id = o
            .try_as("/tokens/create", &[("name", "unbounded child")], actor)
            .unwrap()
            .entity_id
            .unwrap();
        assert_eq!(o.app.owner_tokens[&id].info.expires_ms, None);
    }
}

// --- Review 4: a restore is not a failed backup ---------------------------------

#[test]
fn settings_after_a_restore_says_it_was_restored_not_that_a_backup_failed() {
    for verified in [Some(NOW - 3_600_000), None] {
        let mut o = Owner::new();
        o.app.backup.completed_ms = verified;
        o.app.backup.key = verified.map(|_| "snapshots/synthetic/1-daily-y.json".into());
        let stored = {
            let mut app = o.app.clone();
            engine::pause_restored(&mut app, NOW).unwrap();
            app
        };
        assert!(
            stored
                .backup
                .error
                .as_deref()
                .unwrap()
                .starts_with(engine::RESTORE_MARKER)
        );
        o.app = stored;
        let settings = main_of(&o.page("/settings", &[]));
        assert!(!settings.contains("The last backup failed"), "{verified:?}");
        let expected = match verified {
            Some(_) => {
                "<p data-backup=\"restored\"><strong>Restored here; no backup has been taken here yet.</strong> Last verified backup: "
            }
            None => {
                "<p data-backup=\"restored\"><strong>Restored here; no backup has been taken here yet.</strong> No verified backup exists.</p>"
            }
        };
        assert!(settings.contains(expected), "{verified:?}");
        // As the runtime restores: the status starts empty and checks are paused.
        o.app.backup = BackupStatus::default();
        assert!(main_of(&o.page("/settings", &[])).contains(
            "<p data-backup=\"restored\"><strong>Restored here; no backup has been taken here yet.</strong> No verified backup exists.</p>"
        ));
        // A real failure after the restore reads as a failure.
        o.app.backup.error = Some(FAILED.into());
        assert!(main_of(&o.page("/settings", &[])).contains("<p data-backup=\"failed\">"));
    }
}
