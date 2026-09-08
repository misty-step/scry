//! Production-shaped proof: runs `memory-engine-mcp`'s HTTP client against a
//! Postgres-backed `ApiState` (not the ephemeral `ApiState::default()` file
//! store every other test in this crate uses), the same backend selection
//! `memory-engine-api`'s `main.rs` makes when `MEMORY_ENGINE_POSTGRES_URL` is
//! set — i.e. every real production deployment.
//!
//! Two things this proves that a file-store fixture cannot:
//!
//! 1. The unsupported synchronous `/generate` route remains refused on
//!    Postgres, rather than silently weakening the durable queue boundary.
//! 2. Queued generation publishes validated quizzes on the production
//!    persistence backend without fabricating a learner decision.
//!
//! Skipped when `MEMORY_ENGINE_POSTGRES_TEST_URL` is unset (the same
//! convention every other Postgres-gated test in this workspace uses); point
//! it at a scratch database, e.g. a local `initdb`/`pg_ctl` instance — this
//! test does not require the Dagger `bun run ci:full` lane.

use memory_engine_mcp::client::MemoryEngineClient;
use serde_json::json;

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn queued_generation_succeeds_on_postgres_where_the_legacy_route_is_refused() {
    let Some(database_url) = std::env::var("MEMORY_ENGINE_POSTGRES_TEST_URL").ok() else {
        eprintln!(
            "skipping Postgres production-parity test; MEMORY_ENGINE_POSTGRES_TEST_URL is unset"
        );
        return;
    };

    let email = format!("memory-engine-mcp-pg-test-{}@example.com", unique_suffix());
    let state = memory_engine_api::ApiState::new(
        memory_engine_api::AccountRegistry::with_postgres_url(database_url).with_auth_config(
            memory_engine_api::AuthConfig::allow_emails([email.clone()])
                .with_anonymous_account_creation(true),
        ),
    );
    state.start_worker();

    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind local API listener");
    let address = listener.local_addr().expect("local address");
    let server = tokio::spawn(async move {
        axum::serve(listener, memory_engine_api::router(state))
            .await
            .expect("serve local API");
    });

    let base_url = format!("http://{address}");
    let created: serde_json::Value = ureq::post(format!("{base_url}/v1/accounts"))
        .send_json(json!({ "email": email }))
        .expect("create account")
        .body_mut()
        .read_json()
        .expect("account json");
    let account_id = created["accountId"].as_str().expect("accountId").to_owned();
    let session_token = created["sessionToken"]
        .as_str()
        .expect("sessionToken")
        .to_owned();
    let authorization = format!("Bearer {session_token}");

    // Seed a saved source directly (the same v1 route the MCP client's own
    // `create_deck` uses internally via `project-decks`, called here without
    // that composition so the reproduction below targets exactly one route).
    let source: serde_json::Value = ureq::post(format!(
        "{base_url}/v1/accounts/{account_id}/sources"
    ))
    .header("Authorization", &authorization)
    .send_json(json!({
        "title": "postgres parity fixture",
        "body": "Concept: NATO letter A\nActivity: quiz\nStage: recognition-3\nQuestion: What is the NATO phonetic alphabet word for A?\nAnswer: ALFA\nDistractors: ABLE, ADAM\nReference: The NATO phonetic alphabet word for A is ALFA.",
    }))
    .expect("create source")
    .body_mut()
    .read_json()
    .expect("source json");
    let source_id = source["sourceId"].as_str().expect("sourceId").to_owned();

    // 1. Reproduce the reported production failure: the legacy synchronous
    //    route is refused outright once Postgres is configured.
    let legacy_agent: ureq::Agent = ureq::Agent::config_builder()
        .http_status_as_error(false)
        .build()
        .into();
    let legacy_response = legacy_agent
        .post(format!(
            "{base_url}/v1/accounts/{account_id}/sources/{source_id}/generate"
        ))
        .header("Authorization", &authorization)
        .send_empty()
        .expect("legacy generate request completes");
    assert_eq!(
        legacy_response.status().as_u16(),
        409,
        "the legacy synchronous /generate route must be refused on Postgres"
    );

    // 2. Prove the fix: the same account, same backend, using the queued
    //    generation-jobs route the MCP client now composes exclusively.
    let client = MemoryEngineClient::new(base_url, account_id, session_token);
    let enqueued = client
        .enqueue_generation_job(&source_id)
        .expect("enqueue generation job on postgres backend");

    let job = poll_to_terminal(&client, enqueued.job).await;
    assert_eq!(job.status, "succeeded", "job must succeed: {job:?}");
    assert_eq!(job.card_count, 1);

    assert_published_without_decision(&client);

    server.abort();
}

/// Poll to a bounded terminal state directly, rather than through `create_deck`,
/// which would also create a second source: this keeps the reproduction scoped
/// to exactly the enqueue/poll/publication path.
async fn poll_to_terminal(
    client: &MemoryEngineClient,
    job: memory_engine_mcp::client::GenerationJob,
) -> memory_engine_mcp::client::GenerationJob {
    let mut job = job;
    let mut polls = 0;
    while !job.is_terminal() {
        polls += 1;
        assert!(
            polls < 60,
            "generation job did not terminate within 30s: {job:?}"
        );
        tokio::time::sleep(std::time::Duration::from_millis(500)).await;
        job = client
            .generation_job(&job.id)
            .expect("poll generation job on postgres backend");
    }
    job
}

/// Validated quizzes are due immediately on the production backend, without
/// a synthetic learner approval decision.
fn assert_published_without_decision(client: &MemoryEngineClient) {
    let quizzes = client.quizzes().expect("published quizzes on Postgres");
    assert_eq!(quizzes.len(), 1);
    assert!(quizzes[0].learner_decision.is_none());
    let view = client.next_review().expect("review after generation");
    assert_eq!(view.due_count, 1);
    assert_eq!(
        view.current.expect("published quiz").review_unit_id,
        quizzes[0].review_unit_id
    );
}

fn unique_suffix() -> String {
    use std::sync::atomic::{AtomicU64, Ordering};
    static COUNTER: AtomicU64 = AtomicU64::new(0);

    let millis = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_or(0, |duration| duration.as_millis());
    let counter = COUNTER.fetch_add(1, Ordering::Relaxed);
    format!("{}-{millis}-{counter}", std::process::id())
}
