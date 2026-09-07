use std::sync::atomic::{AtomicUsize, Ordering};

use memory_engine_api_render::{
    render_account_page, render_content_feedback_result_html, render_library_page,
};
use memory_engine_api_state::{
    AccountRegistry, ApiState, AuthConfig, CreateSourceRequest, EnqueueOutcome, SourcePermission,
    StudyViewResponse,
};
use memory_engine_study::BetaStudySummary;

static TEST_COUNTER: AtomicUsize = AtomicUsize::new(0);

fn render_test_state(email: &str) -> ApiState {
    ApiState::new(AccountRegistry::default().with_auth_config(
        AuthConfig::allow_emails([email.to_owned()]).with_anonymous_account_creation(true),
    ))
}

fn unique_email(label: &str) -> String {
    let serial = TEST_COUNTER.fetch_add(1, Ordering::Relaxed);
    format!("render-085-{label}-{serial}@example.com")
}

fn nato_source_body() -> String {
    [
        "Concept: NATO letter A",
        "Activity: quiz",
        "Stage: recognition-3",
        "Question: What is the NATO phonetic alphabet word for A?",
        "Answer: ALFA",
        "Distractors: BRAVO, CHARLIE",
        "Reference: The NATO phonetic alphabet word for A is ALFA.",
    ]
    .join("\n")
}

#[test]
fn active_review_render_skips_workspace_material() {
    let email = unique_email("active");
    let state = render_test_state(&email);
    let created = state.create_account(&email).expect("account");
    let account = state
        .create_browser_session(&created)
        .expect("browser session");
    let source = state
        .save_app_source(
            &account,
            &CreateSourceRequest {
                title: "NATO practice notes".to_owned(),
                body: nato_source_body(),
                permission: SourcePermission::default(),
            },
        )
        .expect("source");
    assert!(matches!(
        state.enqueue_generation_job_by_source(&account, &source.source_id, &source.title),
        EnqueueOutcome::Started(_)
    ));
    state.run_pending_jobs_blocking();

    let pending = state.next_app_review(&account).expect("pending review");
    let view = state
        .keep_draft(
            account.account_id(),
            account.session_token(),
            &pending.drafts[0].id,
        )
        .expect("keep review");
    assert!(
        view.current.is_some(),
        "fixture must reach an active review"
    );
    let html = render_account_page(&account, Some(&view), &[], None);

    assert!(!html.contains(r#"action="/app/source/archive""#));
    assert!(!html.contains("NATO practice notes"));
    assert!(!html.contains(r#"action="/app/generate""#));
    assert!(html.contains(r#"action="/app/reveal""#));
}

#[test]
fn pending_mcq_draft_shows_every_choice_and_distractor_fields() {
    let email = unique_email("pending-mcq");
    let state = render_test_state(&email);
    let created = state.create_account(&email).expect("account");
    let account = state
        .create_browser_session(&created)
        .expect("browser session");
    let source = state
        .save_app_source(
            &account,
            &CreateSourceRequest {
                title: "NATO practice notes".to_owned(),
                body: nato_source_body(),
                permission: SourcePermission::default(),
            },
        )
        .expect("source");
    assert!(matches!(
        state.enqueue_generation_job_by_source(&account, &source.source_id, &source.title),
        EnqueueOutcome::Started(_)
    ));
    state.run_pending_jobs_blocking();

    let view = state.app_study_view(&account).expect("pending drafts");
    assert!(
        view.drafts.iter().any(|draft| !draft.choices.is_empty()),
        "NATO fixture must produce an MCQ draft: {:?}",
        view.drafts
            .iter()
            .map(|draft| (&draft.prompt, &draft.answer, &draft.choices))
            .collect::<Vec<_>>()
    );
    let html = render_account_page(&account, Some(&view), &[], None);

    assert!(
        html.contains("BRAVO"),
        "pending MCQ must show distractors before Keep: {html}"
    );
    assert!(
        html.contains("CHARLIE"),
        "pending MCQ must show every choice before Keep: {html}"
    );
    assert!(
        html.contains(r#"name="choices""#),
        "edit form must expose distractor fields: {html}"
    );
    assert!(
        html.contains(r#"action="/app/draft/keep" method="post""#),
        "approval must remain an explicit single form action: {html}"
    );
}

#[test]
fn library_render_keeps_saved_material_without_active_review() {
    let email = unique_email("library");
    let state = render_test_state(&email);
    let created = state.create_account(&email).expect("account");
    let account = state
        .create_browser_session(&created)
        .expect("browser session");
    state
        .save_app_source(
            &account,
            &CreateSourceRequest {
                title: "NATO practice notes".to_owned(),
                body: nato_source_body(),
                permission: SourcePermission::default(),
            },
        )
        .expect("source");

    let sources = state.list_app_sources(&account).expect("saved sources");
    let view = state.app_study_view(&account).expect("study view");
    let jobs = state.jobs_for_app_account(&account);
    let html = render_library_page(&account, &sources, Some(&view), &jobs, None);
    assert!(html.contains(r#"action="/app/source/archive""#));
    assert!(html.contains("NATO practice notes"));
}

#[test]
fn completed_feedback_action_requires_an_explicit_workspace_exit() {
    let email = unique_email("complete");
    let state = render_test_state(&email);
    let created = state.create_account(&email).expect("account");
    let account = state
        .create_browser_session(&created)
        .expect("browser session");
    let view = StudyViewResponse {
        drafts: Vec::new(),
        current: None,
        concept_progress: Vec::new(),
        summary: BetaStudySummary {
            source_count: 1,
            accepted_draft_count: 1,
            approved_review_unit_count: 1,
            attempt_count: 1,
            last_outcome: None,
            next_review_unit_id: None,
        },
        due_count: 0,
        generation_notices: Vec::new(),
        library: Vec::new(),
    };

    let html = render_content_feedback_result_html(&account, &view, &[], "Saved.");

    assert!(html.contains(r#"class="ae-group me-review-complete""#));
    assert!(html.contains(r#"href="/""#));
    assert!(!html.contains(r#"action="/app/next""#));
    assert!(!html.contains(r#"action="/app/capture""#));
}

#[test]
fn library_discloses_local_only_source_permission() {
    let email = unique_email("local-only");
    let state = render_test_state(&email);
    let created = state.create_account(&email).expect("account");
    let account = state
        .create_browser_session(&created)
        .expect("browser session");
    state
        .save_app_source(
            &account,
            &CreateSourceRequest {
                title: "Private notes".to_owned(),
                body: "Never send this text to a model.".to_owned(),
                permission: SourcePermission::LocalOnly,
            },
        )
        .expect("source");

    let sources = state.list_app_sources(&account).expect("saved sources");
    let view = state.app_study_view(&account).expect("study view");
    let jobs = state.jobs_for_app_account(&account);
    let html = render_library_page(&account, &sources, Some(&view), &jobs, None);
    assert!(html.contains(r#"value="local-only" selected"#));
    assert!(html.contains(r#"action="/app/source/permission" method="post""#));
}
