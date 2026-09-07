use std::{
    io::{Read, Write},
    net::{TcpListener, TcpStream},
    process::{Command, Output},
    sync::mpsc,
    thread,
    time::{Duration, Instant},
};

use memory_engine_canary::{
    read_performance_timeline, CanaryConfig, CanaryReporter, DeliveryFailure, DeliveryOutcome,
    DrainError, ErrorEvent, PerformanceBatch, ReadbackConfig, Severity, PERFORMANCE_BATCH_SCHEMA,
    PERFORMANCE_EVENT_NAME,
};
use memory_engine_performance::{
    Action, CompletionMarker, CompletionPhase, GenerationAction, MachineRouteAction, Navigation,
    Outcome, ReviewAction, Viewport,
};
use serde_json::{json, Value};

#[test]
fn aggregates_observations_into_one_bounded_namespace_event() {
    let (endpoint, requests) = serve_responses(vec![(200, json!({"id": "EVT-1"}))]);
    let reporter = CanaryReporter::new(test_config(endpoint));
    let marker = CompletionMarker::server(
        Action::Machine(MachineRouteAction::OpenApi),
        CompletionPhase::ImmediateAck,
        Outcome::Succeeded,
    )
    .expect("marker");

    for duration_ms in [11, 23, 47] {
        assert!(reporter.report_performance(marker.observation(duration_ms).expect("observation")));
    }
    let delivery = reporter.flush(Duration::from_secs(2)).expect("drain");
    assert_eq!(delivery.outcome(), DeliveryOutcome::Accepted);
    assert_eq!(delivery.observations_accepted(), 3);
    assert_eq!(delivery.observations_dropped(), 0);

    let request = requests
        .recv_timeout(Duration::from_secs(2))
        .expect("performance request");
    assert!(request.starts_with("POST /api/v1/events"));
    let body = request_body(&request);
    assert_eq!(body["name"], PERFORMANCE_EVENT_NAME);
    assert_eq!(body["attributes"]["schema"], PERFORMANCE_BATCH_SCHEMA);
    assert_eq!(body["attributes"]["authority"], "non_authoritative_debug");
    let batch = PerformanceBatch::decode(&body["attributes"]).expect("closed batch");
    assert_eq!(batch.snapshots().len(), 1);
    assert_eq!(batch.snapshots()[0].count(), 3);
    assert_eq!(batch.snapshots()[0].sum_ms(), 81);
    assert_eq!(batch.delivery().batches_sent(), 1);
    assert_eq!(batch.delivery().batches_retried(), 0);
    let encoded = body.to_string();
    for forbidden in [
        "account_id",
        "session_id",
        "source_id",
        "review_unit_id",
        "job_id",
    ] {
        assert!(
            !encoded.contains(forbidden),
            "leaked forbidden field {forbidden}"
        );
    }
}

#[test]
fn emits_no_more_than_one_batch_per_namespace_for_a_flush() {
    let responses = (0..3)
        .map(|index| (200, json!({"id": format!("EVT-{index}")})))
        .collect();
    let (endpoint, requests) = serve_responses(responses);
    let reporter = CanaryReporter::new(test_config(endpoint));
    let observations = [
        CompletionMarker::browser(
            Action::Review(ReviewAction::Submit),
            CompletionPhase::ImmediateAck,
            Outcome::Succeeded,
            Navigation::JavascriptEnhanced,
            Viewport::Mobile,
        )
        .expect("browser marker")
        .observation(20)
        .expect("browser observation"),
        CompletionMarker::server(
            Action::Machine(MachineRouteAction::OpenApi),
            CompletionPhase::ImmediateAck,
            Outcome::Succeeded,
        )
        .expect("server marker")
        .observation(30)
        .expect("server observation"),
        CompletionMarker::job(
            Action::Generation(GenerationAction::DurableTerminal),
            CompletionPhase::DurableGenerationTerminal,
            Outcome::Succeeded,
        )
        .expect("job marker")
        .observation(40)
        .expect("job observation"),
    ];
    for observation in observations {
        assert!(reporter.report_performance(observation));
    }
    let delivery = reporter.flush(Duration::from_secs(2)).expect("drain");
    assert_eq!(delivery.outcome(), DeliveryOutcome::Accepted);
    assert_eq!(delivery.requests_accepted(), 3);
    assert_eq!(delivery.observations_accepted(), 3);

    let mut namespaces = Vec::new();
    for _ in 0..3 {
        let request = requests
            .recv_timeout(Duration::from_secs(2))
            .expect("namespace request");
        namespaces.push(
            request_body(&request)["attributes"]["namespace"]
                .as_str()
                .expect("namespace")
                .to_owned(),
        );
    }
    namespaces.sort_unstable();
    assert_eq!(namespaces, ["browser", "job", "server"]);
    assert_eq!(reporter.flush(Duration::from_secs(2)), Ok(delivery));
    assert!(
        requests.try_recv().is_err(),
        "emitted more than three batches"
    );
}

#[test]
fn saturated_network_worker_never_blocks_request_path_and_accounts_drops() {
    let (endpoint, accepted, requests) = serve_delayed_error_then_event();
    let reporter = CanaryReporter::new(test_config(endpoint));
    reporter.report(&ErrorEvent {
        error_class: "SyntheticBlock".to_owned(),
        message: "hold worker".to_owned(),
        severity: Severity::Info,
        context: None,
        fingerprint: Vec::new(),
    });
    accepted
        .recv_timeout(Duration::from_secs(2))
        .expect("worker entered network call");

    let marker = CompletionMarker::server(
        Action::Machine(MachineRouteAction::OpenApi),
        CompletionPhase::ImmediateAck,
        Outcome::Succeeded,
    )
    .expect("marker");
    let observation = marker.observation(7).expect("observation");
    let started = Instant::now();
    let mut rejected = 0_u64;
    for _ in 0..10_000 {
        if !reporter.report_performance(observation) {
            rejected += 1;
        }
    }
    assert!(started.elapsed() < Duration::from_millis(500));
    assert!(rejected > 0, "fixture did not saturate the bounded queue");
    let delivery = reporter.flush(Duration::from_secs(5)).expect("drain");
    assert_eq!(delivery.outcome(), DeliveryOutcome::Dropped);
    assert_eq!(delivery.observations_dropped(), rejected);
    assert_eq!(delivery.requests_dropped(), 0);

    let event_request = requests
        .recv_timeout(Duration::from_secs(2))
        .expect("event request after delayed error");
    let batch = PerformanceBatch::decode(&request_body(&event_request)["attributes"])
        .expect("performance batch");
    assert_eq!(batch.delivery().observations_dropped(), rejected);
}

#[test]
fn retries_once_and_reports_retry_accounting_in_the_delivered_batch() {
    let (endpoint, requests) = serve_responses(vec![
        (503, json!({"error": "temporary"})),
        (200, json!({"id": "EVT-retry"})),
    ]);
    let reporter = CanaryReporter::new(test_config(endpoint));
    let marker = CompletionMarker::server(
        Action::Machine(MachineRouteAction::OpenApi),
        CompletionPhase::ImmediateAck,
        Outcome::Succeeded,
    )
    .expect("marker");
    assert!(reporter.report_performance(marker.observation(9).expect("observation")));
    let delivery = reporter.flush(Duration::from_secs(3)).expect("drain");
    assert_eq!(delivery.outcome(), DeliveryOutcome::Accepted);
    assert_eq!(delivery.observations_accepted(), 1);
    assert_eq!(delivery.requests_dropped(), 0);
    assert_eq!(delivery.retries(), 1);

    let _first_attempt = requests
        .recv_timeout(Duration::from_secs(2))
        .expect("first attempt");
    let second_attempt = requests
        .recv_timeout(Duration::from_secs(2))
        .expect("retry attempt");
    let batch = PerformanceBatch::decode(&request_body(&second_attempt)["attributes"])
        .expect("retry batch");
    assert_eq!(batch.delivery().batches_sent(), 1);
    assert_eq!(batch.delivery().batches_retried(), 1);
    assert_eq!(batch.delivery().batches_dropped(), 0);
}

#[test]
fn timeline_readback_pages_and_merges_two_instances_exactly() {
    let (ingest_endpoint, ingest_requests) = serve_responses(vec![
        (200, json!({"id": "EVT-instance-1"})),
        (200, json!({"id": "EVT-instance-2"})),
    ]);
    let marker = CompletionMarker::server(
        Action::Machine(MachineRouteAction::OpenApi),
        CompletionPhase::ImmediateAck,
        Outcome::Succeeded,
    )
    .expect("marker");
    for duration_ms in [13, 260] {
        let reporter = CanaryReporter::new(test_config(ingest_endpoint.clone()));
        assert!(reporter.report_performance(
            marker
                .observation(duration_ms)
                .expect("instance observation")
        ));
        let delivery = reporter.shutdown(Duration::from_secs(2)).expect("drain");
        assert_eq!(delivery.outcome(), DeliveryOutcome::Accepted);
    }
    let mut attributes = Vec::new();
    for _ in 0..2 {
        attributes.push(
            request_body(
                &ingest_requests
                    .recv_timeout(Duration::from_secs(2))
                    .expect("ingest request"),
            )["attributes"]
                .clone(),
        );
    }
    // Freeze both independent boots into the same deterministic window even
    // when the test happens to straddle a wall-clock minute boundary.
    let window = attributes[0]["window"].clone();
    attributes[1]["window"] = window.clone();
    attributes[1]["snapshots"][0]["window"] = window;

    let mut wrong_schema = attributes[0].clone();
    wrong_schema["schema"] = json!("memory_engine.performance_batch.v2");
    assert!(PerformanceBatch::decode(&wrong_schema).is_err());
    let mut wrong_buckets = attributes[0].clone();
    wrong_buckets["snapshots"][0]["histogram"]["bucket_counts"]
        .as_array_mut()
        .expect("bucket array")
        .pop();
    assert!(PerformanceBatch::decode(&wrong_buckets).is_err());

    let event = |attributes: Value| {
        json!({
            "event_type": "telemetry.event",
            "signal_name": PERFORMANCE_EVENT_NAME,
            "service": "memory-engine-api",
            "attributes": attributes,
        })
    };
    let (read_endpoint, _) = serve_responses(vec![
        (
            200,
            json!({"events": [event(attributes.remove(0))], "cursor": "page-2"}),
        ),
        (
            200,
            json!({"events": [event(attributes.remove(0))], "cursor": null}),
        ),
    ]);
    let result = read_performance_timeline(
        &ReadbackConfig::new(read_endpoint, "read-key", "memory-engine-api", "1h").expect("config"),
    )
    .expect("readback");

    assert_eq!(result.pages(), 2);
    assert_eq!(result.batches(), 2);
    assert_eq!(result.snapshots().len(), 1);
    let snapshot = &result.snapshots()[0];
    assert_eq!(snapshot.count(), 2);
    assert_eq!(snapshot.sum_ms(), 273);
    assert_eq!(snapshot.max_ms(), 260);
    let p95 = snapshot.percentile_bounds(95).expect("p95");
    assert_eq!((p95.lower_ms, p95.upper_ms), (201, 300));
}

#[test]
fn strict_batch_decode_rejects_unknown_fields() {
    let mut attributes = json!({
        "schema": PERFORMANCE_BATCH_SCHEMA,
        "authority": "non_authoritative_debug",
        "namespace": "server",
        "window": {"start_minute": 1},
        "delivery": {
            "batches_sent": 0,
            "batches_retried": 0,
            "batches_dropped": 0,
            "observations_dropped": 0,
            "observations_invalid": 0,
            "series_dropped": 0
        },
        "snapshots": []
    });
    attributes["raw_path"] = json!("/app/submit");
    assert!(PerformanceBatch::decode(&attributes).is_err());
}

#[test]
fn timeline_readback_rejects_events_outside_service_authority() {
    let attributes = json!({
        "schema": PERFORMANCE_BATCH_SCHEMA,
        "authority": "non_authoritative_debug",
        "namespace": "server",
        "window": {"start_minute": 1},
        "delivery": {
            "batches_sent": 0,
            "batches_retried": 0,
            "batches_dropped": 0,
            "observations_dropped": 0,
            "observations_invalid": 0,
            "series_dropped": 0
        },
        "snapshots": []
    });
    let (endpoint, _) = serve_responses(vec![(
        200,
        json!({
            "events": [{
                "event_type": "telemetry.event",
                "signal_name": PERFORMANCE_EVENT_NAME,
                "service": "another-service",
                "attributes": attributes
            }],
            "cursor": null
        }),
    )]);
    let config =
        ReadbackConfig::new(endpoint, "read-key", "memory-engine-api", "1h").expect("config");
    assert!(read_performance_timeline(&config).is_err());
}

#[test]
fn timed_out_shutdown_can_be_completed_by_retry() {
    let (endpoint, accepted, _requests) = serve_delayed_error_then_event();
    let reporter = CanaryReporter::new(test_config(endpoint));
    reporter.report(&ErrorEvent {
        error_class: "SyntheticBlock".to_owned(),
        message: "hold worker".to_owned(),
        severity: Severity::Info,
        context: None,
        fingerprint: Vec::new(),
    });
    accepted
        .recv_timeout(Duration::from_secs(2))
        .expect("worker entered network call");

    let observation = CompletionMarker::server(
        Action::Machine(MachineRouteAction::OpenApi),
        CompletionPhase::ImmediateAck,
        Outcome::Succeeded,
    )
    .expect("marker")
    .observation(5)
    .expect("observation");
    assert!(reporter.report_performance(observation));

    assert_eq!(
        reporter.shutdown(Duration::from_millis(10)),
        Err(DrainError::DeadlineExceeded),
    );
    let delivery = reporter
        .shutdown(Duration::from_secs(2))
        .expect("finished shutdown");
    assert_eq!(delivery.outcome(), DeliveryOutcome::Accepted);
    assert_eq!(delivery.requests_accepted(), 2);
    assert_eq!(delivery.observations_accepted(), 1);
}

#[test]
fn shutdown_flushes_once_and_rejects_new_work() {
    let (endpoint, requests) = serve_responses(vec![(200, json!({"id": "EVT-1"}))]);
    let reporter = CanaryReporter::new(test_config(endpoint));
    let marker = CompletionMarker::server(
        Action::Machine(MachineRouteAction::OpenApi),
        CompletionPhase::ImmediateAck,
        Outcome::Succeeded,
    )
    .expect("marker");
    let observation = marker.observation(5).expect("observation");
    assert!(reporter.report_performance(observation));
    let delivery = reporter.shutdown(Duration::from_secs(2)).expect("drain");
    assert_eq!(delivery.outcome(), DeliveryOutcome::Accepted);
    requests
        .recv_timeout(Duration::from_secs(2))
        .expect("shutdown batch");
    assert!(!reporter.report_performance(observation));
    assert_eq!(reporter.shutdown(Duration::from_millis(10)), Ok(delivery));
    assert_eq!(
        reporter.flush(Duration::from_millis(10)),
        Err(DrainError::Closed)
    );
}

#[test]
fn empty_shutdown_is_not_positive_delivery() {
    let reporter = CanaryReporter::new(test_config("http://127.0.0.1:0".to_owned()));
    let delivery = reporter.shutdown(Duration::from_secs(2)).expect("drain");
    assert_eq!(delivery.outcome(), DeliveryOutcome::Empty);
    assert_eq!(delivery.requests_accepted(), 0);
    assert_eq!(delivery.observations_accepted(), 0);
    assert_eq!(reporter.shutdown(Duration::ZERO), Ok(delivery));
}

#[test]
fn rejection_is_a_completed_drain_and_survives_repeated_shutdown() {
    let (endpoint, requests) = serve_responses(vec![(403, json!({"error": "not authorized"}))]);
    let reporter = CanaryReporter::new(test_config(endpoint));
    assert!(reporter.report_performance(openapi_observation()));

    let delivery = reporter
        .shutdown(Duration::from_secs(2))
        .expect("completed drain");
    assert_eq!(delivery.outcome(), DeliveryOutcome::Dropped);
    assert_eq!(delivery.requests_accepted(), 0);
    assert_eq!(delivery.requests_dropped(), 1);
    assert_eq!(delivery.observations_accepted(), 0);
    assert_eq!(delivery.observations_dropped(), 1);
    assert_eq!(delivery.retries(), 0);
    assert_eq!(
        delivery.last_failure(),
        Some(DeliveryFailure::Rejected(403))
    );
    assert_eq!(reporter.shutdown(Duration::ZERO), Ok(delivery));
    requests
        .recv_timeout(Duration::from_secs(2))
        .expect("ingest attempt");
    assert!(requests.try_recv().is_err());
}

#[test]
fn exhausted_transient_retries_are_reported_as_loss() {
    let (endpoint, requests) = serve_responses(vec![
        (503, json!({"error": "unavailable"})),
        (503, json!({"error": "still unavailable"})),
    ]);
    let reporter = CanaryReporter::new(test_config(endpoint));
    assert!(reporter.report_performance(openapi_observation()));

    let delivery = reporter
        .shutdown(Duration::from_secs(3))
        .expect("completed drain");
    assert_eq!(delivery.outcome(), DeliveryOutcome::Dropped);
    assert_eq!(delivery.requests_accepted(), 0);
    assert_eq!(delivery.requests_dropped(), 1);
    assert_eq!(delivery.observations_dropped(), 1);
    assert_eq!(delivery.retries(), 1);
    assert_eq!(
        delivery.last_failure(),
        Some(DeliveryFailure::Rejected(503))
    );
    for _ in 0..2 {
        requests
            .recv_timeout(Duration::from_secs(2))
            .expect("bounded attempt");
    }
    assert!(requests.try_recv().is_err());
}

#[test]
fn later_acceptance_does_not_erase_a_failed_flush() {
    let (endpoint, requests) = serve_responses(vec![
        (403, json!({"error": "rejected"})),
        (201, json!({"id": "ERR-accepted"})),
    ]);
    let reporter = CanaryReporter::new(test_config(endpoint));
    let event = ErrorEvent {
        error_class: "SyntheticError".to_owned(),
        message: "bounded test event".to_owned(),
        severity: Severity::Info,
        context: None,
        fingerprint: Vec::new(),
    };
    reporter.report(&event);
    let first = reporter.flush(Duration::from_secs(2)).expect("first drain");
    assert_eq!(first.outcome(), DeliveryOutcome::Dropped);
    assert_eq!(first.requests_accepted(), 0);

    reporter.report(&event);
    let second = reporter
        .shutdown(Duration::from_secs(2))
        .expect("second drain");
    assert_eq!(second.outcome(), DeliveryOutcome::Dropped);
    assert_eq!(second.requests_accepted(), 1);
    assert_eq!(second.requests_dropped(), 1);
    assert_eq!(second.last_failure(), Some(DeliveryFailure::Rejected(403)));
    for _ in 0..2 {
        requests
            .recv_timeout(Duration::from_secs(2))
            .expect("event request");
    }
}

#[test]
fn shutdown_deadline_does_not_claim_acceptance_of_an_unanswered_request() {
    let (endpoint, entered, release) = serve_gated_response();
    let reporter = CanaryReporter::new(test_config(endpoint));
    assert!(reporter.report_performance(openapi_observation()));
    let started = Instant::now();
    assert_eq!(
        reporter.shutdown(Duration::from_millis(20)),
        Err(DrainError::DeadlineExceeded),
    );
    assert!(started.elapsed() < Duration::from_millis(500));
    entered
        .recv_timeout(Duration::from_secs(2))
        .expect("ingest request entered");
    assert!(!reporter.report_performance(openapi_observation()));
    release.send(()).expect("release response");

    let delivery = reporter
        .shutdown(Duration::from_secs(2))
        .expect("finished shutdown");
    assert_eq!(delivery.outcome(), DeliveryOutcome::Accepted);
    assert_eq!(delivery.observations_accepted(), 1);
    assert_eq!(reporter.shutdown(Duration::ZERO), Ok(delivery));
}

#[test]
fn shutdown_can_recover_when_its_control_message_missed_a_full_queue_deadline() {
    let (endpoint, entered, requests) = serve_delayed_error_then_event();
    let reporter = CanaryReporter::new(test_config(endpoint));
    reporter.report(&ErrorEvent {
        error_class: "SyntheticBlock".to_owned(),
        message: "hold worker".to_owned(),
        severity: Severity::Info,
        context: None,
        fingerprint: Vec::new(),
    });
    entered
        .recv_timeout(Duration::from_secs(2))
        .expect("worker blocked");
    let observation = openapi_observation();
    let mut admitted = 0_u64;
    for _ in 0..256 {
        if reporter.report_performance(observation) {
            admitted += 1;
        }
    }
    assert!(admitted > 0 && admitted < 256);
    assert_eq!(
        reporter.shutdown(Duration::from_millis(10)),
        Err(DrainError::DeadlineExceeded),
    );
    let delivery = reporter
        .shutdown(Duration::from_secs(3))
        .expect("recovered shutdown");
    assert_eq!(delivery.outcome(), DeliveryOutcome::Dropped);
    assert_eq!(delivery.observations_accepted(), admitted);
    assert_eq!(delivery.observations_dropped(), 256 - admitted);
    assert_eq!(delivery.requests_dropped(), 0);
    requests
        .recv_timeout(Duration::from_secs(2))
        .expect("accepted batch");
}

#[test]
fn unanswered_http_attempts_time_out_and_exhaust_the_bounded_retry() {
    let (endpoint, release) = serve_unanswered_attempts();
    let reporter = CanaryReporter::new(test_config(endpoint));
    assert!(reporter.report_performance(openapi_observation()));
    let delivery = reporter
        .shutdown(Duration::from_secs(6))
        .expect("bounded retry completed");
    release.send(()).expect("release held sockets");

    assert_eq!(delivery.outcome(), DeliveryOutcome::Dropped);
    assert_eq!(delivery.requests_accepted(), 0);
    assert_eq!(delivery.requests_dropped(), 1);
    assert_eq!(delivery.retries(), 1);
    assert_eq!(delivery.last_failure(), Some(DeliveryFailure::TimedOut));
    assert_eq!(delivery.observations_dropped(), 1);
}

#[test]
fn operator_receipt_succeeds_only_after_ingest_accepts_its_observation() {
    let (endpoint, requests) = serve_responses(vec![
        (
            200,
            json!({"openapi": "3.1.0", "private": "do-not-print-openapi-body"}),
        ),
        (202, json!({"id": "EVT-accepted"})),
    ]);
    let output = run_emit_receipt(&endpoint, &endpoint);
    assert!(output.status.success());
    let receipt = receipt_record(&output);
    assert_eq!(receipt["outcome"], "succeeded");
    assert_eq!(receipt["action_outcome"], "succeeded");
    assert_eq!(receipt["queue_admission"], "accepted");
    assert_eq!(receipt["drained"], true);
    assert_eq!(receipt["delivery"]["status"], "accepted");
    assert_eq!(receipt["delivery"]["observations_accepted"], 1);
    assert_eq!(receipt["delivery"]["observations_dropped"], 0);
    assert_receipt_redacted(&output);
    let openapi = requests
        .recv_timeout(Duration::from_secs(2))
        .expect("OpenAPI request");
    assert!(openapi.starts_with("GET /v1/openapi.json"));
    let ingest = requests
        .recv_timeout(Duration::from_secs(2))
        .expect("ingest request");
    assert!(ingest.starts_with("POST /api/v1/events"));
}

#[test]
fn operator_receipt_fails_when_openapi_succeeds_but_ingest_rejects() {
    let (endpoint, _) = serve_responses(vec![
        (200, json!({"openapi": "3.1.0"})),
        (404, json!({"error": "do-not-print-ingest-body"})),
    ]);
    let output = run_emit_receipt(&endpoint, &endpoint);
    assert!(!output.status.success());
    let receipt = receipt_record(&output);
    assert_eq!(receipt["outcome"], "failed");
    assert_eq!(receipt["action_outcome"], "succeeded");
    assert_eq!(receipt["queue_admission"], "accepted");
    assert_eq!(receipt["drained"], true);
    assert_eq!(receipt["delivery"]["status"], "dropped");
    assert_eq!(receipt["delivery"]["observations_accepted"], 0);
    assert_eq!(receipt["delivery"]["observations_dropped"], 1);
    assert_eq!(receipt["delivery"]["requests_dropped"], 1);
    assert_eq!(receipt["delivery"]["retries"], 0);
    assert_eq!(
        receipt["delivery"]["last_failure"],
        json!({"kind": "rejected", "http_status": 404}),
    );
    assert_receipt_redacted(&output);

    let debug = stdout_records(&output)
        .into_iter()
        .find(|record| record["name"] == PERFORMANCE_EVENT_NAME)
        .expect("debug accounting");
    let accounting = &debug["attributes"]["delivery"];
    assert_eq!(accounting["batches_sent"], 0);
    assert_eq!(accounting["batches_dropped"], 1);
    assert_eq!(accounting["observations_dropped"], 1);
}

#[test]
fn operator_receipt_fails_when_ingest_is_unreachable() {
    let (openapi, _) = serve_responses(vec![(200, json!({"openapi": "3.1.0"}))]);
    let output = run_emit_receipt(&openapi, "http://127.0.0.1:0");
    assert!(!output.status.success());
    let receipt = receipt_record(&output);
    assert_eq!(receipt["outcome"], "failed");
    assert_eq!(receipt["action_outcome"], "succeeded");
    assert_eq!(receipt["delivery"]["status"], "dropped");
    assert_eq!(receipt["delivery"]["observations_accepted"], 0);
    assert_eq!(receipt["delivery"]["requests_dropped"], 1);
    assert_eq!(receipt["delivery"]["retries"], 1);
    assert_eq!(
        receipt["delivery"]["last_failure"],
        json!({"kind": "transport"})
    );
    assert_receipt_redacted(&output);
}

#[test]
fn accepted_failure_observation_does_not_make_a_failed_openapi_request_succeed() {
    let (endpoint, requests) = serve_responses(vec![
        (500, json!({"error": "do-not-print-openapi-body"})),
        (201, json!({"id": "EVT-failure"})),
    ]);
    let output = run_emit_receipt(&endpoint, &endpoint);
    assert!(!output.status.success());
    let receipt = receipt_record(&output);
    assert_eq!(receipt["outcome"], "failed");
    assert_eq!(receipt["action_outcome"], "server_failed");
    assert_eq!(receipt["delivery"]["status"], "accepted");
    assert_receipt_redacted(&output);
    requests
        .recv_timeout(Duration::from_secs(2))
        .expect("OpenAPI request");
    let ingest = requests
        .recv_timeout(Duration::from_secs(2))
        .expect("failure observation");
    let batch = PerformanceBatch::decode(&request_body(&ingest)["attributes"]).expect("batch");
    assert_eq!(
        batch.snapshots()[0].marker().outcome(),
        Outcome::ServerFailed
    );
}

fn test_config(endpoint: String) -> CanaryConfig {
    let mut config =
        CanaryConfig::from_parts(Some(endpoint), Some("sk_test_key".to_owned())).expect("config");
    "test".clone_into(&mut config.environment);
    config
}

fn request_body(request: &str) -> Value {
    serde_json::from_str(request.split("\r\n\r\n").nth(1).expect("body")).expect("json")
}

fn serve_responses(responses: Vec<(u16, Value)>) -> (String, mpsc::Receiver<String>) {
    let listener = TcpListener::bind("127.0.0.1:0").expect("bind");
    let address = listener.local_addr().expect("address");
    let (sender, receiver) = mpsc::channel();
    thread::spawn(move || {
        for (status, body) in responses {
            let mut stream = accept_request(&listener);
            let request = read_request(&mut stream);
            let _ = sender.send(request);
            write_response(&mut stream, status, &body);
        }
    });
    (format!("http://{address}"), receiver)
}

fn serve_delayed_error_then_event() -> (String, mpsc::Receiver<()>, mpsc::Receiver<String>) {
    let listener = TcpListener::bind("127.0.0.1:0").expect("bind");
    let address = listener.local_addr().expect("address");
    let (accepted_sender, accepted_receiver) = mpsc::channel();
    let (request_sender, request_receiver) = mpsc::channel();
    thread::spawn(move || {
        let mut error_stream = accept_request(&listener);
        let _ = read_request(&mut error_stream);
        accepted_sender.send(()).expect("accepted signal");
        thread::sleep(Duration::from_millis(750));
        write_response(&mut error_stream, 200, &json!({"id": "ERR-1"}));

        let mut event_stream = accept_request(&listener);
        let request = read_request(&mut event_stream);
        request_sender.send(request).expect("event request");
        write_response(&mut event_stream, 200, &json!({"id": "EVT-1"}));
    });
    (
        format!("http://{address}"),
        accepted_receiver,
        request_receiver,
    )
}

fn read_request(stream: &mut TcpStream) -> String {
    stream
        .set_read_timeout(Some(Duration::from_secs(3)))
        .expect("read timeout");
    let mut request = Vec::new();
    let mut buffer = [0_u8; 4096];
    loop {
        let read = stream.read(&mut buffer).expect("read");
        request.extend_from_slice(&buffer[..read]);
        let text = String::from_utf8_lossy(&request);
        if let Some(header_end) = text.find("\r\n\r\n") {
            let content_length = text
                .lines()
                .find_map(|line| {
                    line.to_ascii_lowercase()
                        .strip_prefix("content-length:")
                        .map(|value| value.trim().parse::<usize>().expect("length"))
                })
                .unwrap_or(0);
            if request.len() >= header_end + 4 + content_length {
                break;
            }
        }
        if read == 0 {
            break;
        }
    }
    String::from_utf8_lossy(&request).into_owned()
}

fn write_response(stream: &mut TcpStream, status: u16, body: &Value) {
    let body = body.to_string();
    let response = format!(
        "HTTP/1.1 {status} OK\r\ncontent-type: application/json\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
        body.len()
    );
    stream.write_all(response.as_bytes()).expect("write");
}

fn openapi_observation() -> memory_engine_performance::Observation {
    CompletionMarker::server(
        Action::Machine(MachineRouteAction::OpenApi),
        CompletionPhase::ImmediateAck,
        Outcome::Succeeded,
    )
    .expect("marker")
    .observation(5)
    .expect("observation")
}

fn run_emit_receipt(openapi_endpoint: &str, ingest_endpoint: &str) -> Output {
    Command::new(env!("CARGO_BIN_EXE_memory-engine-canary-receipt"))
        .arg("emit-openapi")
        .env("CANARY_ENDPOINT", ingest_endpoint)
        .env("CANARY_API_KEY", "receipt-test-key-not-for-output")
        .env(
            "MEMORY_ENGINE_RECEIPT_OPENAPI_URL",
            format!("{openapi_endpoint}/v1/openapi.json"),
        )
        .output()
        .expect("run operator receipt")
}

fn stdout_records(output: &Output) -> Vec<Value> {
    String::from_utf8_lossy(&output.stdout)
        .lines()
        .filter_map(|line| serde_json::from_str(line).ok())
        .collect()
}

fn receipt_record(output: &Output) -> Value {
    stdout_records(output)
        .into_iter()
        .find(|record| record["schema"] == "memory_engine.performance_receipt.v1")
        .expect("operator receipt record")
}

fn assert_receipt_redacted(output: &Output) {
    for bytes in [&output.stdout, &output.stderr] {
        let text = String::from_utf8_lossy(bytes);
        for forbidden in [
            "receipt-test-key-not-for-output",
            "do-not-print-openapi-body",
            "do-not-print-ingest-body",
        ] {
            assert!(!text.contains(forbidden));
        }
    }
}

fn accept_request(listener: &TcpListener) -> TcpStream {
    listener
        .set_nonblocking(true)
        .expect("nonblocking fixture listener");
    let started = Instant::now();
    loop {
        match listener.accept() {
            Ok((stream, _)) => {
                stream
                    .set_nonblocking(false)
                    .expect("blocking fixture stream");
                stream
                    .set_write_timeout(Some(Duration::from_secs(2)))
                    .expect("write timeout");
                return stream;
            }
            Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                assert!(
                    started.elapsed() < Duration::from_secs(8),
                    "HTTP fixture accept deadline"
                );
                thread::sleep(Duration::from_millis(5));
            }
            Err(error) => panic!("HTTP fixture accept failed: {error}"),
        }
    }
}

fn serve_gated_response() -> (String, mpsc::Receiver<()>, mpsc::Sender<()>) {
    let listener = TcpListener::bind("127.0.0.1:0").expect("bind");
    let endpoint = format!("http://{}", listener.local_addr().expect("address"));
    let (entered_sender, entered_receiver) = mpsc::channel();
    let (release_sender, release_receiver) = mpsc::channel();
    thread::spawn(move || {
        let mut stream = accept_request(&listener);
        read_request(&mut stream);
        entered_sender.send(()).expect("request entered");
        release_receiver
            .recv_timeout(Duration::from_secs(8))
            .expect("release response");
        write_response(&mut stream, 201, &json!({"id": "EVT-delayed"}));
    });
    (endpoint, entered_receiver, release_sender)
}

fn serve_unanswered_attempts() -> (String, mpsc::Sender<()>) {
    let listener = TcpListener::bind("127.0.0.1:0").expect("bind");
    let endpoint = format!("http://{}", listener.local_addr().expect("address"));
    let (release_sender, release_receiver) = mpsc::channel();
    thread::spawn(move || {
        let mut held = Vec::with_capacity(2);
        for _ in 0..2 {
            let mut stream = accept_request(&listener);
            read_request(&mut stream);
            held.push(stream);
        }
        release_receiver
            .recv_timeout(Duration::from_secs(8))
            .expect("release held sockets");
        drop(held);
    });
    (endpoint, release_sender)
}
