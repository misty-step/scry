//! Cold-agent evidence: spawns the real compiled `memory-engine-mcp` binary
//! (not a mocked transport) and drives a full learn -> review -> feedback ->
//! remove -> project deck invalidation loop over its
//! actual stdin/stdout JSON-RPC pipes, against a real local
//! `memory-engine-api` instance (the same Rust binary that runs in
//! production, an in-process axum server with its background generation
//! worker started here — the same worker the deployed binary starts). A
//! hand-run transcript of this same flow, captured from a real terminal,
//! lives at `docs/dogfood/mcp-review-loop.md`.
//!
//! Generation goes through the durable job queue exclusively
//! (`create_deck` enqueues + polls `/generation-jobs`), proving the supported
//! production route contract even though this fixture's `ApiState` is an
//! ephemeral file store, not Postgres — the legacy synchronous `/generate`
//! route is never called regardless of backend (see
//! `create_deck_enqueues_and_polls_without_ever_requesting_generate` in
//! `src/client.rs` for a behavioral, HTTP-request-capture proof of that;
//! production additionally refuses `/generate` outright with HTTP 409 once
//! `MEMORY_ENGINE_POSTGRES_URL` is set —
//! `memory-engine-api-state::registry::generate_source`). A succeeded job's
//! validated quizzes are immediately reviewable without an approval call.

use std::{
    io::{BufRead, BufReader, Write},
    process::{ChildStdin, Command, Stdio},
    sync::mpsc,
    thread,
};

use serde_json::{json, Value};

#[allow(clippy::too_many_lines)]
#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn cold_agent_completes_a_full_review_loop_over_stdio() {
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind local API listener");
    let address = listener.local_addr().expect("local address");
    let email = format!("memory-engine-mcp-test-{}@example.com", unique_suffix());
    let store_root =
        std::env::temp_dir().join(format!("memory-engine-mcp-api-{}", unique_suffix()));
    let state = memory_engine_api::ApiState::new(
        memory_engine_api::AccountRegistry::with_store_root(&store_root).with_auth_config(
            memory_engine_api::AuthConfig::allow_emails([email.clone()])
                .with_anonymous_account_creation(true),
        ),
    );
    // Matches production's `main.rs`: generation is job-queue-based end to
    // end, so the worker must actually run for a queued job to ever reach
    // `succeeded`.
    state.start_worker();
    let created = state.create_account(&email).expect("pre-provision account");
    let account_id = created.account_id.clone();
    let session_token = created.session_token.clone();
    let server = tokio::spawn(async move {
        axum::serve(listener, memory_engine_api::router(state))
            .await
            .expect("serve local API");
    });

    let base_url = format!("http://{address}");

    let binary = env!("CARGO_BIN_EXE_memory-engine-mcp");
    let mut child = Command::new(binary)
        .env("MEMORY_ENGINE_MCP_BASE_URL", &base_url)
        .env("MEMORY_ENGINE_ACCOUNT_ID", &account_id)
        .env("MEMORY_ENGINE_SESSION_TOKEN", &session_token)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .expect("spawn memory-engine-mcp");

    let mut stdin = child.stdin.take().expect("child stdin");
    let stdout = child.stdout.take().expect("child stdout");
    let (tx, rx) = mpsc::channel::<String>();
    let reader = thread::spawn(move || {
        for line in BufReader::new(stdout).lines() {
            let Ok(line) = line else { break };
            if tx.send(line).is_err() {
                break;
            }
        }
    });

    let mut transcript = Vec::new();
    let mut id = 0_u64;
    let mut next_id = || {
        id += 1;
        id
    };

    // 1. initialize
    let init = call(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "initialize",
        &json!({}),
    );
    assert_eq!(init["result"]["serverInfo"]["name"], "memory-engine");

    // Learn one input string, with no project, title or approval step.
    let body = "Concept: NATO letter A\nActivity: quiz\nStage: recognition-3\n\
         Question: What is the NATO phonetic alphabet word for A?\nAnswer: ALFA\n\
         Distractors: ABLE, ADAM\n\
         Reference: The NATO phonetic alphabet word for A is ALFA.\n\n\
         Concept: NATO letter B\nActivity: quiz\nStage: recognition-3\n\
         Question: What is the NATO phonetic alphabet word for B?\nAnswer: BRAVO\n\
         Distractors: BAKER, BOSTON\n\
         Reference: The NATO phonetic alphabet word for B is BRAVO.";
    let learned = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "learn",
        &json!({"input": body}),
    );
    let learned_payload = tool_payload(&learned);
    assert_eq!(learned["result"]["isError"], false);
    assert_eq!(learned_payload["generation"]["status"], "succeeded");
    assert_eq!(learned_payload["generation"]["job"]["cardCount"], 2);
    assert_eq!(
        learned_payload["generation"]["job"]["sourceId"],
        learned_payload["source"]["sourceId"]
    );
    let quizzes = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "list_quizzes",
        &json!({}),
    );
    let quizzes_payload = tool_payload(&quizzes);
    let published = quizzes_payload
        .as_array()
        .expect("published quiz inventory");
    for quiz in published {
        assert!(quiz["sourceSpans"].is_array());
        assert!(quiz["provenance"].is_object());
        assert_eq!(quiz["learnerDecision"], Value::Null);
    }

    // Both automatically published quizzes are already due.
    let due = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "list_due",
        &json!({}),
    );
    assert_eq!(tool_payload(&due)["dueCount"], 2);

    // Either independently active quiz may be selected; follow the returned id.
    let next = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "review_next",
        &json!({}),
    );
    let next_payload = tool_payload(&next);
    let review_unit_id = next_payload["current"]["reviewUnitId"]
        .as_str()
        .expect("reviewUnitId")
        .to_owned();
    let quiz = published
        .iter()
        .find(|quiz| quiz["reviewUnitId"] == review_unit_id)
        .expect("the selected quiz is published inventory");
    let quiz_id = quiz["id"].as_str().expect("quiz id").to_owned();
    assert_eq!(next_payload["current"]["expectedAnswer"], Value::Null);

    // Reveal without grading or advancing, durably marking this occurrence
    // as assisted.
    let revealed = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "reveal_answer",
        &json!({"review_unit_id": review_unit_id}),
    );
    let revealed_payload = tool_payload(&revealed);
    assert_eq!(revealed_payload["current"]["reviewUnitId"], review_unit_id);
    let expected_answer = revealed_payload["current"]["expectedAnswer"]
        .as_str()
        .expect("revealed expected answer")
        .to_owned();
    assert!(["ALFA", "BRAVO"].contains(&expected_answer.as_str()));
    assert_eq!(revealed_payload["current"]["grade"], Value::Null);
    assert_eq!(revealed_payload["summary"]["attemptCount"], 0);

    // An exact answer after exposure is still Again, never evidence of
    // unassisted correct recall.
    let submitted = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "submit_answer",
        &json!({
            "review_unit_id": review_unit_id,
            "answer": expected_answer,
            "idempotency_key": "stdio-assisted-submit",
        }),
    );
    let submitted_payload = tool_payload(&submitted);
    assert_eq!(submitted_payload["current"]["grade"]["verdict"], "revealed");
    assert_eq!(submitted_payload["current"]["grade"]["rating"], 1);
    assert_eq!(submitted_payload["current"]["grade"]["isCorrect"], false);
    assert_eq!(submitted_payload["summary"]["attemptCount"], 1);
    assert_eq!(
        submitted_payload["current"]["feedback"]["itemHistory"]["correct"],
        0
    );
    assert_eq!(submitted_payload["current"]["reviewUnitId"], review_unit_id);
    assert_eq!(submitted_payload["dueCount"], 1);

    // A retried submit returns the assisted receipt without adding an attempt.
    let replayed = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "submit_answer",
        &json!({
            "review_unit_id": review_unit_id,
            "answer": expected_answer,
            "idempotency_key": "stdio-assisted-submit",
        }),
    );
    let replayed_payload = tool_payload(&replayed);
    assert_eq!(
        replayed_payload["current"]["grade"],
        submitted_payload["current"]["grade"]
    );
    assert_eq!(replayed_payload["summary"]["attemptCount"], 1);

    // Content feedback is optional and distinct from grading the answer.
    let feedback = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "record_content_feedback",
        &json!({"review_unit_id": review_unit_id, "verdict": "kept", "rationale": "clear and correct"}),
    );
    let feedback_payload = tool_payload(&feedback);
    assert_eq!(feedback_payload["verdict"], "kept");
    assert_eq!(feedback_payload["reviewUnitId"], review_unit_id);

    // Removal is a direct optional quiz lifecycle operation after review.
    let removed = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "remove_quiz",
        &json!({"quiz_id": quiz_id}),
    );
    assert_eq!(tool_payload(&removed)["summary"]["attemptCount"], 1);
    let remaining = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "list_quizzes",
        &json!({}),
    );
    let remaining_payload = tool_payload(&remaining);
    let remaining_quizzes = remaining_payload
        .as_array()
        .expect("remaining quiz inventory");
    assert!(remaining_quizzes.iter().all(|quiz| quiz["id"] != quiz_id));
    for unremoved in published.iter().filter(|quiz| quiz["id"] != quiz_id) {
        assert!(
            remaining_quizzes
                .iter()
                .any(|quiz| quiz["id"] == unremoved["id"]),
            "removing one quiz must preserve the other published quizzes"
        );
    }

    // Only an explicit review_next consumes the previous held review.
    let next = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "review_next",
        &json!({}),
    );
    let next_payload = tool_payload(&next);
    let next_review_unit_id = next_payload["current"]["reviewUnitId"]
        .as_str()
        .expect("another published quiz is due");
    assert!(remaining_quizzes
        .iter()
        .any(|quiz| quiz["reviewUnitId"] == next_review_unit_id));
    assert_eq!(next_payload["current"]["grade"], Value::Null);
    assert_eq!(next_payload["current"]["expectedAnswer"], Value::Null);
    let unassisted_answer = if expected_answer == "ALFA" {
        "BRAVO"
    } else {
        "ALFA"
    };
    let answered = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "submit_answer",
        &json!({
            "review_unit_id": next_review_unit_id,
            "answer": unassisted_answer,
            "response_time_ms": 1,
            "idempotency_key": "stdio-unassisted-submit",
        }),
    );
    let answered_payload = tool_payload(&answered);
    assert_eq!(answered_payload["current"]["grade"]["verdict"], "correct");
    assert_eq!(answered_payload["current"]["grade"]["rating"], 3);
    assert_eq!(answered_payload["summary"]["attemptCount"], 2);
    assert_eq!(answered_payload["dueCount"], 0);

    // Project decks still publish without approval and can be invalidated.
    let created_deck = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "create_deck",
        &json!({
            "project_key": "nato-onboarding",
            "title": "NATO letter C",
            "body": "Concept: NATO letter C\nActivity: quiz\nStage: recognition-3\nQuestion: What is the NATO phonetic alphabet word for C?\nAnswer: CHARLIE\nDistractors: CHARLES, CANADA\nReference: The NATO phonetic alphabet word for C is CHARLIE.",
        }),
    );
    let deck_payload = tool_payload(&created_deck);
    assert_eq!(deck_payload["generation"]["status"], "succeeded");
    assert_eq!(deck_payload["generation"]["job"]["cardCount"], 1);
    let deck_id = deck_payload["deck"]["deckId"].as_str().expect("deck id");
    let listed_decks = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "list_decks",
        &json!({"project_key": "nato-onboarding"}),
    );
    assert_eq!(
        tool_payload(&listed_decks)[0]["sourceId"],
        deck_payload["deck"]["source"]["sourceId"]
    );

    // Invalidating a deck retires its newly published quiz.
    let invalidated = call_tool(
        &mut stdin,
        &rx,
        &mut transcript,
        next_id(),
        "invalidate_deck",
        &json!({"deck_id": deck_id, "event": "onboarding project closed"}),
    );
    assert_eq!(tool_payload(&invalidated)["dueCount"], 0);

    drop(stdin);
    let status = child.wait().expect("wait for MCP exit");
    reader.join().expect("stdout reader");
    server.abort();

    assert!(
        status.success(),
        "MCP must exit successfully at stdin EOF: {status}; transcript: {transcript:#?}"
    );
    assert!(
        transcript
            .iter()
            .all(|(_, response)| !response.to_string().contains(&session_token)),
        "MCP responses must never disclose the session token"
    );
}

fn rpc(
    stdin: &mut ChildStdin,
    rx: &mpsc::Receiver<String>,
    transcript: &mut Vec<(Value, Value)>,
    id: u64,
    method: &str,
    params: &Value,
) -> Value {
    let request = json!({"jsonrpc": "2.0", "id": id, "method": method, "params": params});
    let line = serde_json::to_string(&request).expect("serialize request");
    writeln!(stdin, "{line}").expect("write request");
    stdin.flush().expect("flush request");

    let response_line = rx
        .recv_timeout(std::time::Duration::from_secs(30))
        .unwrap_or_else(|error| panic!("no response to {method} within timeout: {error}"));
    let response: Value = serde_json::from_str(&response_line).expect("response is valid json");
    transcript.push((request, response.clone()));
    response
}

fn call(
    stdin: &mut ChildStdin,
    rx: &mpsc::Receiver<String>,
    transcript: &mut Vec<(Value, Value)>,
    id: u64,
    method: &str,
    params: &Value,
) -> Value {
    rpc(stdin, rx, transcript, id, method, params)
}

fn call_tool(
    stdin: &mut ChildStdin,
    rx: &mpsc::Receiver<String>,
    transcript: &mut Vec<(Value, Value)>,
    id: u64,
    name: &str,
    arguments: &Value,
) -> Value {
    rpc(
        stdin,
        rx,
        transcript,
        id,
        "tools/call",
        &json!({"name": name, "arguments": arguments}),
    )
}

fn tool_payload(response: &Value) -> Value {
    let text = response["result"]["content"][0]["text"]
        .as_str()
        .unwrap_or_else(|| panic!("tool response carried no text content: {response}"));
    serde_json::from_str(text).expect("tool text payload is valid json")
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
