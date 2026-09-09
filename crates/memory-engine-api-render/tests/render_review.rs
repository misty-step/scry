use std::sync::atomic::{AtomicUsize, Ordering};

use memory_engine_api_render::render_library_page;
use memory_engine_api_state::{
    AccountRegistry, ApiState, AuthConfig, CreateSourceRequest, SourcePermission,
};

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
