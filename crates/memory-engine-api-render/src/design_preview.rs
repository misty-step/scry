//! Design verification harness (dev-only; never compiled into the shipped
//! binary).
//!
//! `emit_preview_pages` renders every learner-facing UI state to standalone
//! HTML under `target/design-preview/` so the whole surface can be reviewed in a
//! browser without booting the server or driving auth. Run it with:
//!
//! ```text
//! cargo test -p memory-engine-api-render --lib emit_preview_pages -- --ignored --nocapture
//! ```
//!
//! Non-ignored tests protect observable study, security, and accessibility
//! contracts. Visual acceptance belongs to the actual PWA, not wording or
//! stylesheet-source conformance assertions.

use std::fmt::Write as _;
use std::fs;
use std::path::PathBuf;

use memory_engine_core::{Rating, ReviewUnitId, Verdict};
use memory_engine_persistence::{GeneratedLearningActivityKind, SourcePermission};
use memory_engine_study::{
    BetaStudyConceptProgress, BetaStudyCurrent, BetaStudyDraftRow, BetaStudyFeedback,
    BetaStudyGrade, BetaStudyItemHistory, BetaStudySummary,
};

use crate::{render_app_shell, render_entry_requested};
use memory_engine_api_state::{
    AccountRegistry, ApiState, AppAccount, AuthConfig, GenerationJob, JobStatus, SourceRecord,
    StudyViewResponse,
};

fn account_context() -> (ApiState, AppAccount) {
    let state = ApiState::new(
        AccountRegistry::default().with_auth_config(
            AuthConfig::allow_emails(["preview@example.com".to_owned()])
                .with_anonymous_account_creation(true),
        ),
    );
    let created = state
        .create_account("preview@example.com")
        .unwrap_or_else(|error| panic!("preview account must be valid: {}", error.message));
    let account = state
        .create_browser_session(&created)
        .unwrap_or_else(|error| panic!("preview browser session must be valid: {}", error.message));
    (state, account)
}

fn source(id: &str, title: &str) -> SourceRecord {
    SourceRecord {
        source_id: id.to_owned(),
        title: title.to_owned(),
        body: String::new(),
        permission: SourcePermission::default(),
        project_key: None,
        ttl_expires_at: None,
    }
}

fn summary() -> BetaStudySummary {
    BetaStudySummary {
        source_count: 2,
        accepted_draft_count: 3,
        approved_review_unit_count: 4,
        attempt_count: 12,
        last_outcome: Some(Verdict::Correct),
        next_review_unit_id: None,
    }
}

fn concept(
    label: &str,
    attempts: usize,
    correct: usize,
    health: &str,
    trend: &str,
    summary_text: &str,
) -> BetaStudyConceptProgress {
    let pct = correct
        .saturating_mul(100)
        .checked_div(attempts)
        .unwrap_or(0);
    BetaStudyConceptProgress {
        concept_key: label.to_lowercase().replace([' ', ':'], "-"),
        concept_label: label.to_owned(),
        attempts,
        correct,
        success_rate: format!("{correct} of {attempts} correct ({pct}%)"),
        trend: trend.to_owned(),
        average_response_time_ms: Some(2100),
        response_time_trend: "steady".to_owned(),
        health: health.to_owned(),
        summary: summary_text.to_owned(),
    }
}

fn item_history() -> BetaStudyItemHistory {
    BetaStudyItemHistory {
        attempts: 3,
        correct: 2,
        success_rate: "2 of 3 correct (67%)".to_owned(),
        trend: "improving".to_owned(),
        last_seen: Some(1_779_000_000_000),
        last_seen_summary: "last seen 2 days ago".to_owned(),
        last_response_time_ms: Some(1800),
        average_response_time_ms: Some(2200),
        response_time_trend: "faster than before".to_owned(),
        stage: "review".to_owned(),
        next_review: "next review in 4 days".to_owned(),
    }
}

fn job(
    id: &str,
    title: &str,
    status: JobStatus,
    card_count: usize,
    error: Option<&str>,
    created_at: i64,
) -> GenerationJob {
    GenerationJob {
        id: id.to_owned(),
        account_id: "preview-account-id".to_owned(),
        source_id: format!("src-{id}"),
        title: title.to_owned(),
        status,
        card_count,
        attempts: 1,
        retryable: true,
        error: error.map(str::to_owned),
        created_at,
        updated_at: created_at,
        retry_at: None,
        lease_expires_at: None,
    }
}

/// A representative activity log: one job running, one failed (retryable), and
/// two finished decks already scheduled — newest first.
fn nato_jobs() -> Vec<GenerationJob> {
    vec![
        job("krebs", "Krebs cycle", JobStatus::Running, 0, None, 400),
        job(
            "spanish",
            "Spanish irregular verbs",
            JobStatus::Failed,
            0,
            Some("Couldn't generate. The model timed out."),
            300,
        ),
        job(
            "nato",
            "NATO phonetic alphabet",
            JobStatus::Succeeded,
            26,
            None,
            200,
        ),
        job(
            "photosynthesis",
            "Photosynthesis",
            JobStatus::Succeeded,
            12,
            None,
            100,
        ),
    ]
}

fn current(prompt: &str) -> BetaStudyCurrent {
    BetaStudyCurrent {
        review_unit_id: ReviewUnitId::new("ru-current"),
        concept_key: Some("nato-phonetic-alphabet".to_owned()),
        prompt_id: "prompt-current".to_owned(),
        activity_kind: GeneratedLearningActivityKind::Quiz,
        activity_stage: "recognition".to_owned(),
        prompt: prompt.to_owned(),
        choices: Vec::new(),
        revision_expected_answer: "efímero".to_owned(),
        expected_answer: None,
        reference_text: None,
        worked_solution: None,
        grade: None,
        review_state: None,
        schedule_change: None,
        feedback: None,
        content_feedback_head_id: None,
    }
}

fn graded(verdict: Verdict, rating: Rating, with_feedback: bool) -> BetaStudyCurrent {
    let mut current = current("Translate to Spanish: “ephemeral”.");
    current.expected_answer = Some("efímero".to_owned());
    current.grade = Some(BetaStudyGrade {
        verdict,
        rating,
        is_correct: verdict == Verdict::Correct,
    });
    if with_feedback {
        current.feedback = Some(BetaStudyFeedback {
            verdict: format!("{verdict:?}").to_lowercase(),
            expected_answer: "efímero".to_owned(),
            item_history: item_history(),
            concept_progress: Some(concept(
                "Spanish: ephemeral words",
                11,
                6,
                "watch",
                "slipping",
                "One more like that lifts this concept above half.",
            )),
            remediation_drafts_pending: false,
        });
    }
    current
}

fn graded_mcq(verdict: Verdict, rating: Rating) -> BetaStudyCurrent {
    let mut current = graded(verdict, rating, true);
    current.choices = vec![
        "fugaz".to_owned(),
        "efímero".to_owned(),
        "duradero".to_owned(),
        "cotidiano".to_owned(),
    ];
    current
}

fn view(
    drafts: Vec<BetaStudyDraftRow>,
    current: Option<BetaStudyCurrent>,
    concepts: Vec<BetaStudyConceptProgress>,
    due_count: usize,
    generation_notices: Vec<String>,
) -> StudyViewResponse {
    StudyViewResponse {
        drafts,
        queue: Vec::new(),
        current,
        concept_progress: concepts,
        summary: summary(),
        due_count,
        generation_notices,
        library: Vec::new(),
    }
}

/// Every learner-facing UI state, in journey order, as `(slug, full-page html)`.
// Inherently a long enumeration of fixtures, one per UI state; splitting it
// would scatter the state matrix without making it clearer.
#[allow(clippy::too_many_lines)]
fn pages() -> Vec<(&'static str, String)> {
    let (_, acct) = account_context();
    let sources = vec![
        source("src-feynman", "The Feynman Lectures — Chapter 1"),
        source("src-spanish", "Spanish vocabulary — week 3"),
    ];
    let concepts = vec![
        concept(
            "Photosynthesis",
            7,
            6,
            "healthy",
            "improving",
            "Strong recall — intervals are widening.",
        ),
        concept(
            "Spanish: ephemeral words",
            11,
            6,
            "watch",
            "slipping",
            "Mixed recall — intervals staying short.",
        ),
    ];

    let mut multiple_choice =
        current("Which process converts light energy into chemical energy in plants?");
    multiple_choice.choices = vec![
        "Respiration".to_owned(),
        "Photosynthesis".to_owned(),
        "Transpiration".to_owned(),
        "Fermentation".to_owned(),
    ];

    let open = current("Translate to Spanish: “ephemeral”.");

    let mut revealed_without_feedback = graded(Verdict::Revealed, Rating::Again, false);
    revealed_without_feedback.reference_text = Some(
        "“efímero” — lasting a very short time; from the Greek ephēmeros, ‘lasting a day’."
            .to_owned(),
    );

    let jobs = nato_jobs();

    vec![
        (
            "01-signed-out",
            render_app_shell(None, &[], None, &[], None),
        ),
        (
            "02-capture-queued",
            crate::render_capture_waiting_page(
                &acct,
                &job(
                    "preview-queued",
                    "Spanish vocabulary",
                    JobStatus::Queued,
                    0,
                    None,
                    100,
                ),
            ),
        ),
        (
            "03-activity",
            crate::render_library_page(
                &acct,
                &sources,
                Some(&view(vec![], None, concepts.clone(), 8, vec![])),
                &jobs,
                None,
            ),
        ),
        (
            "04-root-question",
            render_app_shell(
                Some(&acct),
                &sources,
                Some(&view(
                    vec![],
                    Some(open.clone()),
                    concepts.clone(),
                    3,
                    vec![],
                )),
                &[],
                None,
            ),
        ),
        (
            "05-review-choices",
            render_app_shell(
                Some(&acct),
                &sources,
                Some(&view(
                    vec![],
                    Some(multiple_choice),
                    concepts.clone(),
                    3,
                    vec![],
                )),
                &[],
                None,
            ),
        ),
        (
            "06-review-open",
            render_app_shell(
                Some(&acct),
                &sources,
                Some(&view(vec![], Some(open), concepts.clone(), 3, vec![])),
                &[],
                None,
            ),
        ),
        (
            "07-graded-correct-free",
            render_app_shell(
                Some(&acct),
                &sources,
                Some(&view(
                    vec![],
                    Some(graded(Verdict::Correct, Rating::Good, true)),
                    concepts.clone(),
                    2,
                    vec![],
                )),
                &[],
                None,
            ),
        ),
        (
            "08-graded-close-free",
            render_app_shell(
                Some(&acct),
                &sources,
                Some(&view(
                    vec![],
                    Some(graded(Verdict::Close, Rating::Again, true)),
                    concepts.clone(),
                    3,
                    vec![],
                )),
                &[],
                None,
            ),
        ),
        (
            "09-graded-wrong-free",
            render_app_shell(
                Some(&acct),
                &sources,
                Some(&view(
                    vec![],
                    Some(graded(Verdict::Wrong, Rating::Again, true)),
                    concepts.clone(),
                    3,
                    vec![],
                )),
                &[],
                None,
            ),
        ),
        (
            "10-graded-revealed-free",
            render_app_shell(
                Some(&acct),
                &sources,
                Some(&view(
                    vec![],
                    Some(graded(Verdict::Revealed, Rating::Again, true)),
                    concepts.clone(),
                    3,
                    vec![],
                )),
                &[],
                None,
            ),
        ),
        (
            "11-graded-correct-mcq",
            render_app_shell(
                Some(&acct),
                &sources,
                Some(&view(
                    vec![],
                    Some(graded_mcq(Verdict::Correct, Rating::Good)),
                    concepts.clone(),
                    2,
                    vec![],
                )),
                &[],
                None,
            ),
        ),
        (
            "12-graded-close-mcq",
            render_app_shell(
                Some(&acct),
                &sources,
                Some(&view(
                    vec![],
                    Some(graded_mcq(Verdict::Close, Rating::Again)),
                    concepts.clone(),
                    3,
                    vec![],
                )),
                &[],
                None,
            ),
        ),
        (
            "13-graded-wrong-mcq",
            render_app_shell(
                Some(&acct),
                &sources,
                Some(&view(
                    vec![],
                    Some(graded_mcq(Verdict::Wrong, Rating::Again)),
                    concepts.clone(),
                    3,
                    vec![],
                )),
                &[],
                None,
            ),
        ),
        (
            "14-graded-revealed-mcq",
            render_app_shell(
                Some(&acct),
                &sources,
                Some(&view(
                    vec![],
                    Some(graded_mcq(Verdict::Revealed, Rating::Again)),
                    concepts.clone(),
                    3,
                    vec![],
                )),
                &[],
                None,
            ),
        ),
        (
            "15-graded-revealed-no-feedback",
            render_app_shell(
                Some(&acct),
                &sources,
                Some(&view(
                    vec![],
                    Some(revealed_without_feedback),
                    concepts,
                    3,
                    vec![],
                )),
                &[],
                None,
            ),
        ),
        (
            "16-check-email",
            render_entry_requested(Some("/app/login/verify?token=preview-token")),
        ),
    ]
}

/// Additional real renderer surfaces for the browser walk. Fixture data is
/// authored here; no provider requests or live accounts are used.
fn supplemental_pages() -> Vec<(&'static str, String)> {
    let (state, acct) = account_context();
    let source_text = "Photosynthesis takes place in chloroplasts. Chlorophyll absorbs light energy, which the plant uses to convert carbon dioxide and water into sugars.\n\nA chloroplast contains thylakoid membranes and a surrounding fluid called the stroma. The light-dependent reactions take place in the thylakoid membranes. Carbon fixation takes place in the stroma.";
    let source = state.save_app_source(
        &acct,
        &memory_engine_api_state::CreateSourceRequest {
            title: "Plant physiology: how chloroplast structure supports photosynthesis and carbon fixation".to_owned(),
            body: source_text.to_owned(),
            permission: SourcePermission::LocalOnly,
        },
    ).expect("preview source");
    let draft = BetaStudyDraftRow {
        id: "preview-draft".to_owned(),
        review_unit_id: ReviewUnitId::new("preview-draft-quiz"),
        activity_kind: GeneratedLearningActivityKind::Quiz,
        activity_stage: "recognition".to_owned(),
        prompt: "Which plant cell structure carries out photosynthesis?".to_owned(),
        answer: "Chloroplast".to_owned(),
        choices: vec!["Chloroplast".to_owned(), "Mitochondrion".to_owned(), "Nucleus".to_owned()],
        concept_label: "Location of photosynthesis".to_owned(),
        validation_status: memory_engine_persistence::GeneratedPromptValidationStatus::Accepted,
        validation_reasons: vec![],
        worked_solution: Some("Chloroplasts contain the structures for capturing light and fixing carbon. The question asks for the organelle, not one of its internal membranes.".to_owned()),
        approved: true,
        learner_decision: None,
        source_spans: vec![memory_engine_study::BetaStudyReferenceSpanRow {
            id: "preview-span".to_owned(),
            source_document_id: source.source_id.clone(),
            label: source.title.clone(),
            text: source_text.to_owned(),
            locator: "Saved source".to_owned(),
        }],
        provenance: None,
    };
    let mut library_view = view(vec![draft], None, vec![], 1, vec![]);
    library_view.library = vec![memory_engine_study::LibrarySourceRow {
        source_id: source.source_id.clone(),
        title: source.title.clone(),
        active_card_count: 1,
        concepts: vec![memory_engine_study::LibraryConceptRow {
            concept_label: "Location of photosynthesis".to_owned(),
            card_count: 1,
        }],
    }];
    let mut reading = current("Where does photosynthesis take place in a plant cell?");
    reading.reference_text = Some(format!("Source: {}\n\n{source_text}", source.title));
    let reading_view = view(vec![], Some(reading), vec![], 1, vec![]);
    let revealed_view = view(
        vec![],
        Some(graded(Verdict::Revealed, Rating::Again, true)),
        vec![],
        1,
        vec![],
    );
    let progress = view(
        vec![],
        None,
        vec![
            concept(
                "Location of photosynthesis",
                3,
                1,
                "struggling",
                "slipping",
                "One correct answer in three recorded attempts.",
            ),
            concept(
                "Carbon fixation",
                0,
                0,
                "untried",
                "untried",
                "No review evidence yet.",
            ),
        ],
        1,
        vec![],
    );
    let create_view = state.app_study_view(&acct).expect("preview study view");
    vec![
        ("17-home-new", render_app_shell(Some(&acct), &[], None, &[], None)),
        ("18-home-caught-up", render_app_shell(Some(&acct), &[], Some(&view(vec![], None, vec![], 0, vec![])), &[], None)),
        ("19-create", crate::render_create_page(&acct, Some(&create_view), &[], None)),
        ("20-create-recovery", crate::render_create_page(&acct, Some(&create_view), &[], Some("The source could not be saved. Your material has not entered the review queue."))),
        ("21-library-published-quiz", crate::render_library_page(&acct, std::slice::from_ref(&source), Some(&library_view), &[], None)),
        ("22-study-note", crate::render_reference_page(&acct, &reading_view)),
        ("23-assisted-practice", render_app_shell(Some(&acct), &[], Some(&revealed_view), &[], None)),
        ("24-progress", crate::render_analytics_page(&acct, &progress, crate::AnalyticsViewOptions::default())),
        ("25-waitlist-or-link", render_entry_requested(None)),
        ("26-link-recovery", crate::render_auth_recovery("This sign-in link has expired", "Request a fresh link to return to your study space.")),
        ("27-entry-recovery", crate::render_entry_recovery("That email needs another look", "Enter a complete email address to request access.")),
        ("28-review-recovery", crate::render_submit_recovery("Your review needs a fresh page", "The quiz has changed since this page opened. Return to your review to continue safely.")),
        ("29-reminder-confirmation", crate::render_return_notification_confirmation("preview-unsubscribe-token")),
        ("30-reminders-disabled", crate::render_return_notification_disabled()),
        ("31-reminder-recovery", crate::render_return_notification_recovery("This reminder link has expired", "Open Scry to change your reminder settings.")),
        ("32-edit-quiz", crate::render_edit_review_html(&acct, &reading_view, &[], None)),
        ("33-capture-waiting", crate::render_capture_waiting_page(&acct, &job("preview-waiting", "Plant physiology", JobStatus::Running, 0, None, 100))),
        ("34-capture-failed", crate::render_capture_waiting_page(&acct, &job("preview-failed", "Plant physiology", JobStatus::Failed, 0, Some("The provider could not complete this request. Open Library to retry."), 100))),
        ("35-capture-finished", crate::render_capture_waiting_page(&acct, &job("preview-finished", "Plant physiology", JobStatus::Succeeded, 1, None, 100))),
    ]
}

#[test]
#[ignore = "writes HTML preview files; run explicitly with --ignored"]
fn emit_preview_pages() -> Result<(), Box<dyn std::error::Error>> {
    let out = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../target/design-preview");
    fs::create_dir_all(out.join("static"))?;
    // Mirror the served path `/static/ledger.css` so the pages resolve their
    // stylesheet when this directory is served at the root over HTTP.
    fs::write(
        out.join("static/ledger.css"),
        include_str!("../assets/ledger.css"),
    )?;
    fs::create_dir_all(out.join("static/fonts"))?;
    fs::write(
        out.join("static/fonts/literata-latin-variable.woff2"),
        crate::LITERATA_WOFF2,
    )?;
    fs::write(
        out.join("static/fonts/manrope-latin-variable.woff2"),
        crate::MANROPE_WOFF2,
    )?;
    fs::write(
        out.join("static/fonts/OFL.txt"),
        include_str!("../assets/fonts/OFL.txt"),
    )?;

    let mut pages = pages();
    pages.extend(supplemental_pages());
    let mut index = String::from(
        "<!doctype html><html lang=en><meta charset=utf-8>\
<meta name=viewport content='width=device-width,initial-scale=1'>\
<title>Scry design preview</title><link rel=stylesheet href=/static/ledger.css>\
<body><main class=ae-view><h1 class=me-display>Scry surface preview</h1>\
<p class=ae-lede>Each frame is a rendered study surface. Open a frame on its own to inspect phone widths and keyboard focus.</p>",
    );
    for (name, html) in &pages {
        fs::write(out.join(format!("{name}.html")), html)?;
        let _ = write!(
            index,
            "<section class=ae-group>\
<h2 class=ae-h><a href='{name}.html'>{name}</a></h2>\
<iframe title='{name}' src='{name}.html' loading='lazy' style='display:block;width:100%;height:860px;border:0'></iframe>\
</section>"
        );
    }
    index.push_str("</main></body></html>");
    fs::write(out.join("index.html"), index)?;

    eprintln!("\ndesign preview written to {}\n", out.display());
    Ok(())
}

#[test]
fn graded_review_requires_protected_explicit_next() {
    let (_, acct) = account_context();
    let html = render_app_shell(
        Some(&acct),
        &[],
        Some(&view(
            vec![],
            Some(graded_mcq(Verdict::Correct, Rating::Good)),
            vec![],
            2,
            vec![],
        )),
        &[],
        None,
    );
    assert!(html.contains("efímero"));
    let advance = form_for_action(&html, "/app/next");
    assert!(advance.contains(r#"method="post""#));
    assert!(advance.contains(&format!(
        r#"name="csrfToken" value="{}""#,
        acct.csrf_token()
    )));
    assert!(!html.contains(r#"action="/app/submit""#));
    assert!(!html.contains(r#"action="/app/reveal""#));
    let feedback = form_for_action(&html, "/app/content-feedback");
    assert!(feedback.contains(r#"name="idempotencyKey""#));
    assert!(feedback.contains(r#"value="kept""#));
    assert!(feedback.contains(r#"value="dropped""#));
}

/// A scheduled retry remains in flight; completed jobs cannot leave a stale
/// generating notice behind. Unrelated confirmations remain visible.
#[test]
fn conformance_generating_notice_only_shows_while_a_job_is_actually_in_flight() {
    let (_, acct) = account_context();
    let generating = "Generating your cards. They'll appear below as they're ready.";

    let in_flight = vec![job(
        "run-1",
        "Spanish irregular verbs",
        JobStatus::Retry,
        0,
        None,
        100,
    )];
    let live = render_app_shell(Some(&acct), &[], None, &in_flight, Some(generating));
    assert!(
        live.contains(generating),
        "a live job must show the generating notice: {live}"
    );

    let resolved = vec![job(
        "run-1",
        "Spanish irregular verbs",
        JobStatus::Succeeded,
        12,
        None,
        100,
    )];
    let stale = render_app_shell(Some(&acct), &[], None, &resolved, Some(generating));
    assert!(
        !stale.contains(generating),
        "the generating notice must not linger once nothing is in flight: {stale}"
    );

    // Notices unrelated to generation are unconditional.
    let removed = render_app_shell(Some(&acct), &[], None, &resolved, Some("Source removed."));
    assert!(removed.contains("Source removed."));
}

/// Account and study mutations remain protected on every signed-in surface.
#[test]
fn signed_in_forms_preserve_session_scope() {
    let (_, acct) = account_context();
    let empty = view(vec![], None, vec![], 0, vec![]);
    for html in [
        render_app_shell(Some(&acct), &[], None, &[], None),
        crate::render_create_page(&acct, Some(&empty), &[], None),
        crate::render_library_page(&acct, &[], Some(&empty), &[], None),
        crate::render_analytics_page(&acct, &empty, crate::AnalyticsViewOptions::default()),
        render_app_shell(
            Some(&acct),
            &[],
            Some(&view(
                vec![],
                Some(current("What do plants need?")),
                vec![],
                1,
                vec![],
            )),
            &[],
            None,
        ),
    ] {
        for action in [
            "/app/logout",
            "/app/logout-all",
            "/app/return-notifications",
        ] {
            let form = form_for_action(&html, action);
            assert!(form.contains(r#"method="post""#));
            assert!(form.contains(&format!(
                r#"name="csrfToken" value="{}""#,
                acct.csrf_token()
            )));
        }
        assert!(!html.contains(acct.session_token()));
    }
    let review = render_app_shell(
        Some(&acct),
        &[],
        Some(&view(
            vec![],
            Some(current("What do plants need?")),
            vec![],
            1,
            vec![],
        )),
        &[],
        None,
    );
    let submit = form_for_action(&review, "/app/submit");
    let reveal = form_for_action(&review, "/app/reveal");
    for form in [submit, reveal] {
        assert!(form.contains(r#"name="responseTimeMs" value="""#));
        assert!(form.contains(r#"name="idempotencyKey""#));
        assert!(form.contains(&format!(
            r#"name="csrfToken" value="{}""#,
            acct.csrf_token()
        )));
    }
    assert!(review.contains(r#"data-review-status role="status" aria-live="polite""#));
}

fn form_for_action<'a>(html: &'a str, action: &str) -> &'a str {
    let action_at = html
        .find(&format!(r#"action="{action}""#))
        .unwrap_or_else(|| panic!("missing form action {action}"));
    let start = html[..action_at].rfind("<form").expect("opening form");
    let end = action_at + html[action_at..].find("</form>").expect("closing form");
    &html[start..end]
}

#[test]
fn study_note_escapes_material_and_returns_to_the_same_quiz() {
    let (_, acct) = account_context();
    let mut quiz = current("Explain <photosynthesis> & energy.");
    quiz.reference_text =
        Some("Source: <script>alert('source')</script>\n\nLight & carbon dioxide.".to_owned());
    let study_view = view(vec![], Some(quiz), vec![], 1, vec![]);
    let html = crate::render_reference_page(&acct, &study_view);
    assert!(html.contains("&lt;script&gt;alert('source')&lt;/script&gt;"));
    assert!(html.contains("Light &amp; carbon dioxide."));
    assert!(!html.contains("<script>alert("));
    assert!(!html.contains("Explain <photosynthesis>"));
    let resume = form_for_action(&html, "/app/resume");
    assert!(resume.contains(r#"method="post""#));
    assert!(resume.contains(r#"name="reviewUnitId" value="ru-current""#));
    assert!(resume.contains(&format!(
        r#"name="csrfToken" value="{}""#,
        acct.csrf_token()
    )));
    assert!(!html.contains(r#"action="/app/next""#));
    assert!(!html.contains(r#"action="/app/submit""#));
}

#[test]
fn absent_study_material_does_not_invent_reading_or_lose_the_quiz() {
    let (_, acct) = account_context();
    let study_view = view(
        vec![],
        Some(current("A question without a note.")),
        vec![],
        1,
        vec![],
    );
    let html = crate::render_reference_page(&acct, &study_view);
    assert!(!html.contains(r#"class="me-reading""#));
    assert!(
        !html.contains("efímero"),
        "the answer must not become fabricated reading"
    );
    assert!(
        form_for_action(&html, "/app/resume").contains(r#"name="reviewUnitId" value="ru-current""#)
    );
}
