use std::{
    process::ExitCode,
    time::{Duration, Instant},
};

use memory_engine_canary::{
    read_performance_timeline, CanaryConfig, CanaryReporter, DeliveryOutcome, DeliveryReport,
    DrainError, ReadbackConfig,
};
use memory_engine_performance::{
    Action, CompletionMarker, CompletionPhase, MachineRouteAction, Outcome,
};

const READ_ENDPOINT_ENV: &str = "CANARY_READ_ENDPOINT";
const READ_API_KEY_ENV: &str = "CANARY_READ_API_KEY";
const OPENAPI_URL_ENV: &str = "MEMORY_ENGINE_RECEIPT_OPENAPI_URL";
const DEFAULT_SERVICE: &str = "memory-engine-api";
const OVERHEAD_SAMPLES: usize = 20_000;
const MAX_ADMISSION_P95_NANOS: u64 = 5_000_000;

fn main() -> ExitCode {
    match run() {
        Ok(()) => ExitCode::SUCCESS,
        Err(message) => {
            eprintln!("memory-engine-canary-receipt: {message}");
            ExitCode::FAILURE
        }
    }
}

fn run() -> Result<(), String> {
    match std::env::args().nth(1).as_deref() {
        Some("emit-openapi") => emit_openapi(),
        Some("readback") => readback(),
        Some("overhead") => overhead(),
        _ => Err(
            "usage: memory-engine-canary-receipt <emit-openapi|readback|overhead>\n\
             emit-openapi env: CANARY_ENDPOINT, CANARY_API_KEY, optional MEMORY_ENGINE_RECEIPT_OPENAPI_URL\n\
             readback env: CANARY_READ_ENDPOINT, CANARY_READ_API_KEY, optional CANARY_READ_SERVICE/CANARY_READ_WINDOW\n\
             overhead: measures bounded report_performance admission p95"
                .to_owned(),
        ),
    }
}

fn emit_openapi() -> Result<(), String> {
    let config = CanaryConfig::from_env()
        .ok_or_else(|| "CANARY_ENDPOINT and CANARY_API_KEY are required".to_owned())?;
    let url = std::env::var(OPENAPI_URL_ENV).unwrap_or_else(|_| {
        let port = std::env::var("PORT").unwrap_or_else(|_| "8080".to_owned());
        format!("http://127.0.0.1:{port}/v1/openapi.json")
    });
    let agent: ureq::Agent = ureq::Agent::config_builder()
        .timeout_global(Some(Duration::from_secs(10)))
        .http_status_as_error(false)
        .build()
        .into();
    let started = Instant::now();
    let action_result = request_openapi(&agent, &url);
    let elapsed = duration_ms(started);
    let outcome = if action_result.is_ok() {
        Outcome::Succeeded
    } else {
        Outcome::ServerFailed
    };
    let marker = CompletionMarker::server(
        Action::Machine(MachineRouteAction::OpenApi),
        CompletionPhase::ImmediateAck,
        outcome,
    )
    .map_err(|error| error.to_string())?;
    let observation = marker
        .observation(elapsed)
        .map_err(|error| error.to_string())?;
    let reporter = CanaryReporter::new(config);
    let admitted = reporter.report_performance(observation);
    let delivery = reporter.shutdown(Duration::from_secs(6));
    let accepted = admitted
        && delivery.as_ref().is_ok_and(|report| {
            report.outcome() == DeliveryOutcome::Accepted
                && report.observations_accepted() == 1
                && report.observations_dropped() == 0
        });
    println!(
        "{}",
        serde_json::json!({
            "schema": "memory_engine.performance_receipt.v1",
            "action": "open_api",
            "duration_ms": elapsed,
            "outcome": if action_result.is_ok() && accepted { "succeeded" } else { "failed" },
            "action_outcome": if action_result.is_ok() { "succeeded" } else { "server_failed" },
            "queue_admission": if admitted { "accepted" } else { "rejected" },
            "drained": delivery.is_ok(),
            "delivery": delivery_receipt(&delivery),
        })
    );
    if !admitted {
        return Err("bounded reporter queue rejected the receipt observation".to_owned());
    }
    let report = delivery.map_err(|error| error.to_string())?;
    if !accepted {
        let reason = report.last_failure().map_or_else(
            || "observation lost before acceptance".to_owned(),
            |error| error.to_string(),
        );
        return Err(format!(
            "Canary did not accept the receipt observation: {reason}"
        ));
    }
    action_result
}

fn request_openapi(agent: &ureq::Agent, url: &str) -> Result<(), String> {
    let mut response = agent.get(url).call().map_err(|error| match error {
        ureq::Error::Timeout(_) => "OpenAPI request timed out".to_owned(),
        _ => "OpenAPI request transport failed".to_owned(),
    })?;
    if !response.status().is_success() {
        return Err(format!(
            "OpenAPI request failed with status {}",
            response.status()
        ));
    }
    std::io::copy(&mut response.body_mut().as_reader(), &mut std::io::sink())
        .map_err(|_| "OpenAPI response read failed".to_owned())?;
    Ok(())
}

fn delivery_receipt(delivery: &Result<DeliveryReport, DrainError>) -> serde_json::Value {
    match delivery {
        Ok(report) => serde_json::json!({
            "status": match report.outcome() {
                DeliveryOutcome::Empty => "empty",
                DeliveryOutcome::Accepted => "accepted",
                DeliveryOutcome::Dropped => "dropped",
            },
            "acceptance_boundary": "ingest_http_2xx",
            "requests_accepted": report.requests_accepted(),
            "requests_dropped": report.requests_dropped(),
            "retries": report.retries(),
            "observations_accepted": report.observations_accepted(),
            "observations_dropped": report.observations_dropped(),
            "last_failure": report.last_failure().map(|failure| match failure {
                memory_engine_canary::DeliveryFailure::Rejected(status) => serde_json::json!({
                    "kind": "rejected", "http_status": status,
                }),
                memory_engine_canary::DeliveryFailure::TimedOut => serde_json::json!({
                    "kind": "timed_out",
                }),
                memory_engine_canary::DeliveryFailure::Transport => serde_json::json!({
                    "kind": "transport",
                }),
            }),
        }),
        Err(error) => serde_json::json!({
            "status": "unconfirmed",
            "error": error.to_string(),
        }),
    }
}

fn readback() -> Result<(), String> {
    let endpoint =
        std::env::var(READ_ENDPOINT_ENV).map_err(|_| format!("{READ_ENDPOINT_ENV} is required"))?;
    let api_key =
        std::env::var(READ_API_KEY_ENV).map_err(|_| format!("{READ_API_KEY_ENV} is required"))?;
    let service =
        std::env::var("CANARY_READ_SERVICE").unwrap_or_else(|_| DEFAULT_SERVICE.to_owned());
    let window = std::env::var("CANARY_READ_WINDOW").unwrap_or_else(|_| "1h".to_owned());
    let config = ReadbackConfig::new(endpoint, api_key, service, window)
        .map_err(|error| error.to_string())?;
    let receipt = read_performance_timeline(&config).map_err(|error| error.to_string())?;
    println!("{}", receipt.as_value());
    Ok(())
}

fn overhead() -> Result<(), String> {
    let config = CanaryConfig::from_parts(
        Some("http://127.0.0.1:9".to_owned()),
        Some("overhead-receipt".to_owned()),
    )
    .ok_or_else(|| "failed to construct overhead reporter".to_owned())?;
    let reporter = CanaryReporter::new(config);
    let marker = CompletionMarker::server(
        Action::Machine(MachineRouteAction::OpenApi),
        CompletionPhase::ImmediateAck,
        Outcome::Succeeded,
    )
    .map_err(|error| error.to_string())?;
    let observation = marker.observation(1).map_err(|error| error.to_string())?;
    let mut durations = Vec::with_capacity(OVERHEAD_SAMPLES);
    let mut accepted = 0_usize;
    for _ in 0..OVERHEAD_SAMPLES {
        let started = Instant::now();
        if reporter.report_performance(observation) {
            accepted += 1;
        }
        durations.push(u64::try_from(started.elapsed().as_nanos()).unwrap_or(u64::MAX));
    }
    durations.sort_unstable();
    let p95_index = (durations.len() * 95).div_ceil(100).saturating_sub(1);
    let p95_nanos = durations[p95_index];
    // This command measures admission only; it does not claim backend delivery.
    let _ = reporter.shutdown(Duration::from_secs(6));
    println!(
        "{}",
        serde_json::json!({
            "schema": "memory_engine.performance_overhead_receipt.v1",
            "samples": OVERHEAD_SAMPLES,
            "accepted": accepted,
            "p95_nanos": p95_nanos,
            "limit_nanos": MAX_ADMISSION_P95_NANOS,
            "passed": p95_nanos <= MAX_ADMISSION_P95_NANOS,
        })
    );
    if p95_nanos > MAX_ADMISSION_P95_NANOS {
        return Err(format!(
            "reporter admission p95 {p95_nanos}ns exceeds {MAX_ADMISSION_P95_NANOS}ns"
        ));
    }
    Ok(())
}

fn duration_ms(started: Instant) -> u64 {
    u64::try_from(started.elapsed().as_millis()).unwrap_or(u64::MAX)
}
