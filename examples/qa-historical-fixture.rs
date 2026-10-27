//! Test-only native fixture transformer. No HTTP routes, provider calls or
//! persistence capabilities: stdin rows -> retained historical engine -> stdout
//! validated rows. It is never part of the deployed Worker or normal CLI.
use scry::{engine, model::*, owner_token::MutationActor, persistence, study};
use serde::Deserialize;
use serde_json::{Value, json};
use std::{
    collections::BTreeMap,
    io::{self, Read},
};

#[derive(Deserialize)]
struct Input {
    rows: BTreeMap<String, String>,
    #[serde(default)]
    path: Option<String>,
    #[serde(default)]
    fields: BTreeMap<String, String>,
    now_ms: i64,
    seed: String,
    #[serde(default)]
    actor_token_id: Option<String>,
    #[serde(default)]
    actions: Vec<Action>,
}
#[derive(Deserialize)]
#[serde(tag = "kind", rename_all = "snake_case")]
enum Action {
    Mutate {
        path: String,
        fields: BTreeMap<String, String>,
        seed: String,
        now_ms: i64,
        #[serde(default)]
        actor_token_id: Option<String>,
    },
    Claim {
        lease: String,
        now_ms: i64,
        generation: bool,
        jev: bool,
        critic: bool,
    },
    ReceivedFailure {
        work: engine::Work,
        now_ms: i64,
        raw: String,
        model: String,
        cost_micros: Option<u64>,
        message: String,
    },
    SeedDemo {
        now_ms: i64,
    },
}
fn run(input: Input) -> AppResult<Value> {
    let mut app = persistence::app_from_rows(&input.rows)?;
    engine::validate_app(&app)?;
    let mut actions = input.actions;
    if let Some(path) = input.path {
        actions.push(Action::Mutate {
            path,
            fields: input.fields,
            seed: input.seed,
            now_ms: input.now_ms,
            actor_token_id: input.actor_token_id,
        });
    }
    let mut results = Vec::new();
    for action in actions {
        let value = match action {
            Action::Mutate {
                path,
                mut fields,
                seed,
                now_ms,
                actor_token_id,
            } => {
                fields.insert("csrf".into(), app.csrf.clone());
                let replayed = fields
                    .get("operation_id")
                    .is_some_and(|id| app.operations.contains_key(id));
                let result = if let Some(token_id) = actor_token_id {
                    let token = app
                        .owner_tokens
                        .get(&token_id)
                        .ok_or_else(|| {
                            AppError::new(422, "Historical fixture token does not exist.")
                        })?
                        .info
                        .clone();
                    engine::mutate_as(
                        &mut app,
                        &path,
                        &fields,
                        now_ms,
                        &seed,
                        MutationActor::Token(&token),
                    )?
                } else {
                    engine::mutate(&mut app, &path, &fields, now_ms, &seed)?
                };
                json!({"practice":study::wire_response(&app,&result),"result":result,"replayed":replayed})
            }
            Action::Claim {
                lease,
                now_ms,
                generation,
                jev,
                critic,
            } => {
                let config = engine::WorkConfig {
                    generation,
                    jev,
                    critic,
                    model: "synthetic-historical-generator".into(),
                    jev_model: "synthetic-historical-grader".into(),
                };
                json!({"work":engine::claim_work(&mut app,now_ms,&lease,&config)?})
            }
            Action::ReceivedFailure {
                work,
                now_ms,
                raw,
                model,
                cost_micros,
                message,
            } => {
                engine::fail_received_work(
                    &mut app,
                    &work,
                    now_ms,
                    raw,
                    model,
                    cost_micros,
                    &message,
                )?;
                json!({"historical_received_failure": true})
            }
            Action::SeedDemo { now_ms } => {
                engine::seed_fixture(&mut app, now_ms)?;
                json!({"authored_demo":true})
            }
        };
        engine::validate_app(&app)?;
        persistence::archive_size(&app)?;
        results.push(value);
    }
    let mut output = results.last().cloned().unwrap_or(json!({}));
    output["rows"] = serde_json::to_value(persistence::rows(&app)?)
        .map_err(|_| AppError::new(500, "Cannot encode fixture rows."))?;
    output["results"] = json!(results);
    output["provenance"] =
        json!("synthetic-historical-engine-fixture; not a supported current HTTP write");
    Ok(output)
}
fn main() {
    let mut bytes = Vec::new();
    let result = io::stdin()
        .take(32 * 1024 * 1024 + 1)
        .read_to_end(&mut bytes)
        .map_err(|_| AppError::new(422, "Cannot read synthetic fixture input."))
        .and_then(|_| {
            if bytes.len() > 32 * 1024 * 1024 {
                Err(AppError::new(413, "Synthetic fixture input is too large."))
            } else {
                Ok(())
            }
        })
        .and_then(|_| {
            serde_json::from_slice::<Input>(&bytes)
                .map_err(|_| AppError::new(422, "Invalid synthetic fixture input."))
        })
        .and_then(run);
    match result {
        Ok(value) => println!("{value}"),
        Err(error) => {
            println!("{}", json!({"error":error.message,"status":error.status}));
            std::process::exit(1);
        }
    }
}
