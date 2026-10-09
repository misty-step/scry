//! The owner application state machine. Every public mutation is all-or-nothing.
//! External requests are claimed durably, performed by the runtime, then fenced again here.
use crate::{
    generation::{self, Batch},
    learning,
    model::*,
};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use std::collections::{BTreeMap, BTreeSet};

#[derive(Clone, Debug)]
pub struct WorkConfig {
    pub generation: bool,
    pub jev: bool,
    pub critic: bool,
    pub model: String,
    pub jev_model: String,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct Work {
    pub id: String,
    pub lease: String,
    pub kind: String,
    pub request: Value,
    pub photo_key: Option<String>,
}

fn id(seed: &str, label: &str) -> String {
    hex::encode(Sha256::digest(format!("{seed}:{label}").as_bytes()))[..32].into()
}
fn field<'a>(fields: &'a BTreeMap<String, String>, key: &str) -> &'a str {
    fields.get(key).map(String::as_str).unwrap_or("")
}
fn boolean(fields: &BTreeMap<String, String>, key: &str) -> AppResult<bool> {
    match field(fields, key) {
        "true" => Ok(true),
        "false" => Ok(false),
        _ => Err(AppError::new(422, "Choose an explicit result.")),
    }
}
fn version(fields: &BTreeMap<String, String>) -> AppResult<u64> {
    field(fields, "content_version")
        .parse()
        .map_err(|_| AppError::conflict())
}
fn result(redirect: impl Into<String>, entity_id: Option<String>) -> MutationResult {
    MutationResult {
        redirect: redirect.into(),
        entity_id,
        message: None,
    }
}

pub fn mutate(
    app: &mut App,
    path: &str,
    fields: &BTreeMap<String, String>,
    now: i64,
    seed: &str,
) -> AppResult<MutationResult> {
    if field(fields, "csrf") != app.csrf || app.csrf.len() < 24 {
        return Err(AppError::new(
            403,
            "Your session needs to be refreshed before saving.",
        ));
    }
    let operation = field(fields, "operation_id");
    if !(16..=128).contains(&operation.len())
        || !operation
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || matches!(c, '-' | '_'))
    {
        return Err(AppError::new(
            422,
            "Reload this page to create a valid save operation.",
        ));
    }
    let payload: BTreeMap<_, _> = fields
        .iter()
        .filter(|(k, _)| k.as_str() != "csrf")
        .collect();
    let hash = hex::encode(Sha256::digest(
        serde_json::to_vec(&(path, payload)).map_err(|_| AppError::new(422, "Invalid request."))?,
    ));
    if let Some(receipt) = app.operations.get(operation) {
        return if receipt.payload_hash == hash {
            Ok(receipt.result.clone())
        } else {
            Err(AppError::conflict())
        };
    }
    let mut next = app.clone();
    let response = apply(&mut next, path, fields, now, seed)?;
    next.operations.insert(
        operation.into(),
        Receipt {
            payload_hash: hash,
            result: response.clone(),
        },
    );
    next.revision += 1;
    *app = next;
    Ok(response)
}

fn active_occurrence(
    app: &App,
    fields: &BTreeMap<String, String>,
    phase: &[&str],
) -> AppResult<Occurrence> {
    let o = app.occurrence.as_ref().ok_or_else(AppError::conflict)?;
    if o.id != field(fields, "occurrence_id") || !phase.contains(&o.phase.as_str()) {
        return Err(AppError::conflict());
    }
    let q = app
        .questions
        .get(&o.question_id)
        .ok_or_else(AppError::conflict)?;
    if o.phase != "result"
        && (q.archived
            || q.schedule_version != o.schedule_version
            || q.content().version != o.presentation.version
            || app.goals.get(&q.goal_id).is_none_or(|g| g.archived)
            || app.concepts.get(&q.concept_id).is_none_or(|c| c.archived))
    {
        return Err(AppError::conflict());
    }
    Ok(o.clone())
}

fn create_job(
    app: &mut App,
    goal_id: &str,
    kind: &str,
    question_id: Option<String>,
    now: i64,
    seed: &str,
) -> AppResult<String> {
    let goal = app
        .goals
        .get(goal_id)
        .ok_or_else(|| AppError::new(404, "That learning goal does not exist."))?;
    if goal.archived {
        return Err(AppError::conflict());
    }
    if app.jobs.values().any(|j| {
        j.goal_id == goal_id
            && matches!(
                j.status.as_str(),
                "queued" | "sent" | "candidates" | "critic-sent"
            )
    }) {
        return Err(AppError::new(
            409,
            "This material is already being prepared. Your existing study is still available.",
        ));
    }
    let job_id = id(seed, "job");
    let attempt = if kind == "create" {
        app.jobs
            .values()
            .filter(|j| j.goal_id == goal_id && j.kind == kind && j.question_id == question_id)
            .count() as u8
    } else {
        0
    };
    if attempt >= 3 {
        return Err(AppError::new(
            422,
            "Three attempts have been saved. Change the input or feedback before asking again.",
        ));
    }
    let question_version = question_id
        .as_ref()
        .and_then(|id| app.questions.get(id))
        .map(|q| q.content().version);
    app.jobs.insert(
        job_id.clone(),
        Job {
            id: job_id.clone(),
            goal_id: goal_id.into(),
            question_id,
            question_version,
            source_revision: goal.revision,
            kind: kind.into(),
            status: "queued".into(),
            created_ms: now,
            lease: None,
            started_ms: None,
            attempt: attempt + 1,
            error: None,
            raw_response: None,
            model: None,
            feedback_ids: app
                .feedback
                .iter()
                .filter(|f| f.goal_id == goal_id)
                .map(|f| f.id.clone())
                .collect(),
            rejected: Vec::new(),
            candidate_json: None,
            critic_json: None,
            critic_cache_json: None,
            critic_model: None,
            critic_parent_id: None,
            critic_inherited_keys: Vec::new(),
        },
    );
    if kind != "fix" {
        let goal = app.goals.get_mut(goal_id).unwrap();
        goal.job_id = job_id.clone();
        goal.status = "preparing".into();
    }
    Ok(job_id)
}

fn apply(
    app: &mut App,
    path: &str,
    fields: &BTreeMap<String, String>,
    now: i64,
    seed: &str,
) -> AppResult<MutationResult> {
    if matches!(path, "/create" | "/add") {
        let intent = field(fields, "intent");
        let photo = if field(fields, "__photo").is_empty() {
            None
        } else {
            Some(
                serde_json::from_str::<Photo>(field(fields, "__photo"))
                    .map_err(|_| AppError::new(422, "The photo could not be saved."))?,
            )
        };
        if (!generation::text_valid(intent, TEXT_LIMIT) && photo.is_none())
            || intent.len() > TEXT_LIMIT
            || (photo.is_some() && intent.len() > 1024)
        {
            return Err(AppError::new(
                422,
                "Enter what you want to learn, up to 32 KiB of text or a photo with a 1 KiB caption. Your draft is still editable.",
            ));
        }
        if photo.as_ref().is_some_and(|p| {
            p.size == 0
                || p.size > PHOTO_LIMIT
                || !matches!(p.mime.as_str(), "image/jpeg" | "image/png" | "image/webp")
                || p.sha256.len() != 64
        }) {
            return Err(AppError::new(
                422,
                "Choose a supported photo no larger than 4 MiB.",
            ));
        }
        let goal_id = id(seed, "goal");
        let title = if intent.trim().is_empty() {
            "From your photo".into()
        } else {
            intent
                .lines()
                .next()
                .unwrap_or(intent)
                .trim()
                .chars()
                .take(120)
                .collect()
        };
        app.goals.insert(
            goal_id.clone(),
            Goal {
                id: goal_id.clone(),
                title,
                intent: intent.into(),
                created_ms: now,
                revision: 1,
                status: "preparing".into(),
                paused: false,
                focused: false,
                archived: false,
                concept_ids: Vec::new(),
                job_id: String::new(),
                photo,
                transcript: None,
            },
        );
        create_job(app, &goal_id, "create", None, now, seed)?;
        return Ok(result(format!("/goals/{goal_id}"), Some(goal_id)));
    }
    match path {
        "/review/intro" => {
            let o = active_occurrence(app, fields, &["intro"])?;
            let known = boolean(fields, "known")?;
            let c = app.concepts.get_mut(&o.concept_id).unwrap();
            c.introduced_ms = Some(now);
            c.exposure_ms = Some(now);
            c.already_known = known;
            let o = app.occurrence.as_mut().unwrap();
            o.phase = "question".into();
            o.assisted = true;
            Ok(result("/", None))
        }
        "/review/answer" => {
            let o = active_occurrence(app, fields, &["question"])?;
            let answer = field(fields, "answer");
            if !generation::text_valid(answer, 4000) {
                return Err(AppError::new(
                    422,
                    "Write an answer, or choose Show me. Your answer has not been graded.",
                ));
            }
            if o.presentation.kind == "choice"
                && !o.presentation.choices.iter().any(|v| v == answer)
            {
                return Err(AppError::new(
                    422,
                    "Choose one of the answers that was displayed.",
                ));
            }
            let kind = if o.presentation.kind == "choice" {
                "choice"
            } else {
                "recall"
            };
            if let Some(correct) = learning::exact_grade(
                kind,
                &o.presentation.answer,
                &o.presentation.variants,
                answer,
            ) {
                app.occurrence.as_mut().unwrap().answer = Some(answer.into());
                commit_grade(app, correct, false, "exact", now, seed)?;
            } else {
                stage_assessment(app, answer, now, seed)?;
            }
            Ok(result("/", None))
        }
        "/review/reveal" => {
            active_occurrence(app, fields, &["question"])?;
            let o = app.occurrence.as_mut().unwrap();
            o.answer = Some(String::new());
            o.assisted = true;
            commit_grade(app, false, true, "reveal", now, seed)?;
            Ok(result("/", None))
        }
        "/review/help" => {
            let o = active_occurrence(app, fields, &["question"])?;
            app.occurrence.as_mut().unwrap().assisted = true;
            app.concepts.get_mut(&o.concept_id).unwrap().exposure_ms = Some(now);
            let to = field(fields, "return_to");
            let q = &app.questions[&o.question_id];
            let dest = if to == format!("/goals/{}", q.goal_id)
                || to == format!("/concepts/{}", o.concept_id)
                || to == format!("/questions/{}/edit", q.id)
                || to == "/export"
                || to == format!("/photos/{}", q.goal_id)
            {
                to.to_string()
            } else {
                format!("/concepts/{}", o.concept_id)
            };
            Ok(result(dest, None))
        }
        "/review/self" => {
            active_occurrence(app, fields, &["self-check"])?;
            let correct = boolean(fields, "correct")?;
            commit_grade(app, correct, false, "learner", now, seed)?;
            Ok(result("/", None))
        }
        "/review/retry" => {
            let o = active_occurrence(app, fields, &["self-check"])?;
            let assessment = o
                .assessment_id
                .as_ref()
                .and_then(|id| app.assessments.get(id))
                .ok_or_else(AppError::conflict)?;
            if !matches!(
                assessment.status.as_str(),
                "failed" | "unknown" | "paused" | "superseded"
            ) {
                return Err(AppError::new(
                    422,
                    "This check completed. Choose your own result to continue.",
                ));
            }
            let answer = o.answer.clone().unwrap_or_default();
            stage_assessment(app, &answer, now, seed)?;
            Ok(result("/", None))
        }
        "/review/override" => {
            let o = active_occurrence(app, fields, &["result"])?;
            let correct = boolean(fields, "correct")?;
            let e = app
                .events
                .iter()
                .find(|e| Some(&e.id) == o.result.as_ref().map(|r| &r.event_id))
                .ok_or_else(AppError::conflict)?
                .clone();
            if !matches!(e.result.authority.as_str(), "exact" | "jev")
                || e.assisted
                || e.result.rating == 0
                || app.overrides.iter().any(|v| v.event_id == e.id)
                || correct == (e.result.outcome == "correct" || e.result.outcome == "warm_correct")
            {
                return Err(AppError::conflict());
            }
            let q = app
                .questions
                .get_mut(&e.question_id)
                .ok_or_else(AppError::conflict)?;
            if q.schedule_version != e.schedule_version {
                return Err(AppError::conflict());
            }
            let rating = if e.assisted {
                0
            } else if correct {
                3
            } else {
                1
            };
            let after = if rating == 0 {
                e.before_card.clone()
            } else {
                learning::schedule(&e.before_card, rating, e.result.reviewed_ms)
                    .map_err(|m| AppError::new(422, m))?
            };
            let before = q.card.clone();
            q.card = after.clone();
            q.schedule_version += 1;
            app.overrides.push(Override {
                id: id(seed, "override"),
                event_id: e.id.clone(),
                correct,
                before_card: before,
                after_card: after.clone(),
                created_ms: now,
            });
            let r = app.occurrence.as_mut().unwrap().result.as_mut().unwrap();
            r.outcome = if correct { "correct" } else { "wrong" }.into();
            if e.assisted {
                r.outcome = format!("warm_{}", r.outcome)
            }
            r.authority = "learner".into();
            r.corrected = true;
            r.rating = rating;
            r.due_ms = after.due_ms;
            Ok(result("/", None))
        }
        "/review/next" => {
            active_occurrence(app, fields, &["result"])?;
            app.occurrence = None;
            ensure_occurrence(app, now, seed)?;
            Ok(result("/", None))
        }
        "/settings" => {
            let pace = field(fields, "pace");
            if !matches!(pace, "light" | "steady" | "intense") {
                return Err(AppError::new(
                    422,
                    "Choose light, steady, or intense practice.",
                ));
            }
            app.preferences.pace = pace.into();
            Ok(result("/settings", None))
        }
        "/settings/backup" => Ok(result("/settings", None)),
        _ => apply_entity(app, path, fields, now, seed),
    }
}

fn stage_assessment(app: &mut App, answer: &str, now: i64, seed: &str) -> AppResult<()> {
    let o = app.occurrence.as_ref().unwrap();
    if app
        .assessments
        .values()
        .filter(|a| a.occurrence_id == o.id)
        .count()
        >= 3
    {
        return Err(AppError::new(
            422,
            "Three checks are saved. Please check this answer yourself.",
        ));
    }
    let aid = id(seed, "assessment");
    app.assessments.insert(
        aid.clone(),
        Assessment {
            id: aid.clone(),
            occurrence_id: o.id.clone(),
            question_id: o.question_id.clone(),
            content_version: o.presentation.version,
            schedule_version: o.schedule_version,
            policy: if o.presentation.kind == "explain" {
                "semantic-v1"
            } else {
                "short-v1"
            }
            .into(),
            answer: answer.into(),
            status: "queued".into(),
            lease: None,
            started_ms: None,
            raw_response: None,
            error: None,
            created_ms: now,
        },
    );
    let o = app.occurrence.as_mut().unwrap();
    o.phase = "checking".into();
    o.answer = Some(answer.into());
    o.assessment_id = Some(aid);
    Ok(())
}

fn commit_grade(
    app: &mut App,
    correct: bool,
    reveal: bool,
    authority: &str,
    now: i64,
    seed: &str,
) -> AppResult<()> {
    let o = app
        .occurrence
        .as_ref()
        .ok_or_else(AppError::conflict)?
        .clone();
    let q = app
        .questions
        .get_mut(&o.question_id)
        .ok_or_else(AppError::conflict)?;
    if q.schedule_version != o.schedule_version
        || q.content().version != o.presentation.version
        || o.phase == "result"
        || q.archived
        || app.goals.get(&q.goal_id).is_none_or(|g| g.archived)
        || app.concepts.get(&q.concept_id).is_none_or(|c| c.archived)
    {
        return Err(AppError::conflict());
    }
    // Self-check exposes the key only after staging; it cannot change whether the original attempt was aided.
    let assisted = o.assisted || reveal;
    let warm = assisted && !reveal;
    let rating = if warm {
        0
    } else if correct {
        3
    } else {
        1
    };
    let after = if rating == 0 {
        q.card.clone()
    } else {
        learning::schedule(&o.before_card, rating, now).map_err(|m| AppError::new(422, m))?
    };
    let event_id = id(seed, "event");
    let outcome = if reveal {
        "revealed".into()
    } else if warm {
        if correct {
            "warm_correct"
        } else {
            "warm_wrong"
        }
        .into()
    } else if correct {
        "correct".into()
    } else {
        "wrong".into()
    };
    q.card = after.clone();
    q.schedule_version += 1;
    // Feedback exposes the key after the saved attempt. Future presentations retain that help.
    app.concepts.get_mut(&q.concept_id).unwrap().exposure_ms = Some(now);
    let grade_policy = match authority {
        "jev" => {
            if o.presentation.kind == "explain" {
                "semantic-v1"
            } else {
                "short-v1"
            }
        }
        "learner" => "learner-v1",
        _ => "exact-v1",
    };
    let review_result = ReviewResult {
        event_id: event_id.clone(),
        outcome,
        authority: authority.into(),
        rating,
        reviewed_ms: now,
        due_ms: after.due_ms,
        corrected: false,
    };
    app.events.push(Event {
        id: event_id,
        occurrence_id: o.id.clone(),
        question_id: o.question_id,
        concept_id: o.concept_id,
        presentation: o.presentation,
        answer: o.answer.unwrap_or_default(),
        assisted,
        before_card: o.before_card,
        after_card: after,
        schedule_version: q.schedule_version,
        algorithm: learning::event_algorithm(grade_policy),
        result: review_result.clone(),
    });
    let o = app.occurrence.as_mut().unwrap();
    o.phase = "result".into();
    o.result = Some(review_result);
    Ok(())
}

fn apply_entity(
    app: &mut App,
    path: &str,
    fields: &BTreeMap<String, String>,
    now: i64,
    seed: &str,
) -> AppResult<MutationResult> {
    let parts: Vec<_> = path.trim_matches('/').split('/').collect();
    if parts.len() != 3 {
        return Err(AppError::new(404, "That action does not exist."));
    }
    let entity = parts[1];
    let action = parts[2];
    match parts[0] {
        "goals" => {
            let goal = app
                .goals
                .get(entity)
                .ok_or_else(|| AppError::new(404, "That goal does not exist."))?
                .clone();
            if goal.archived {
                return Err(AppError::conflict());
            }
            match action {
                "pause" => {
                    app.goals.get_mut(entity).unwrap().paused = !goal.paused;
                }
                "focus" => {
                    for g in app.goals.values_mut() {
                        g.focused = g.id == entity && !goal.focused;
                    }
                }
                "archive" => {
                    let g = app.goals.get_mut(entity).unwrap();
                    g.archived = true;
                    g.revision += 1;
                    g.status = "archived".into();
                    let questions: Vec<_> = app
                        .questions
                        .values()
                        .filter(|q| q.goal_id == entity)
                        .map(|q| q.id.clone())
                        .collect();
                    for question in questions {
                        invalidate_pending(
                            app,
                            &question,
                            "This goal was archived. Your saved answer was not graded.",
                        );
                    }
                    for j in app.jobs.values_mut().filter(|j| {
                        j.goal_id == entity
                            && !matches!(j.status.as_str(), "ready" | "partial" | "failed")
                    }) {
                        j.status = "superseded".into();
                    }
                }
                "retry" => {
                    let old = app
                        .jobs
                        .get(&goal.job_id)
                        .ok_or_else(AppError::conflict)?
                        .clone();
                    if !matches!(old.status.as_str(), "failed" | "unknown" | "paused") {
                        return Err(AppError::conflict());
                    }
                    if old.attempt >= 3 {
                        return Err(AppError::new(
                            422,
                            "Three attempts have been saved. Change the feedback before asking again.",
                        ));
                    }
                    let new_id =
                        create_job(app, entity, &old.kind, old.question_id.clone(), now, seed)?;
                    app.jobs.get_mut(&new_id).unwrap().attempt = old.attempt + 1;
                    if old.candidate_json.is_some() {
                        let batch: Batch =
                            serde_json::from_str(old.candidate_json.as_deref().unwrap_or(""))
                                .map_err(|_| {
                                    AppError::new(422, "Saved candidates could not be read.")
                                })?;
                        let inherited = generation::read_critic_cache(
                            &batch,
                            old.critic_cache_json.as_deref(),
                        )?;
                        let j = app.jobs.get_mut(&new_id).unwrap();
                        j.critic_inherited_keys = inherited.answers.into_keys().collect();
                        j.critic_parent_id = Some(old.id);
                        j.critic_cache_json = old.critic_cache_json;
                        j.candidate_json = old.candidate_json;
                        j.status = "candidates".into();
                        j.rejected = old.rejected;
                        j.raw_response = old.raw_response;
                        j.model = old.model;
                    }
                }
                "refine" => {
                    let text = field(fields, "feedback");
                    if !text.trim().is_empty() {
                        add_feedback(app, entity, (None, None), "request", text, now, seed)?;
                    }
                    let has_observations = app.events.iter().any(|e| {
                        app.questions
                            .get(&e.question_id)
                            .is_some_and(|q| q.goal_id == entity)
                    });
                    if !has_observations && app.feedback.iter().all(|f| f.goal_id != entity) {
                        return Err(AppError::new(
                            422,
                            "Tell Scry what to improve, or practice first so refinement has something real to use.",
                        ));
                    }
                    create_job(app, entity, "refine", None, now, seed)?;
                }
                _ => return Err(AppError::new(404, "That action does not exist.")),
            }
            Ok(result(
                if matches!(action, "archive" | "pause" | "focus") {
                    "/map".into()
                } else {
                    format!("/goals/{entity}")
                },
                None,
            ))
        }
        "concepts" => {
            let c = app
                .concepts
                .get(entity)
                .ok_or_else(|| AppError::new(404, "That idea does not exist."))?
                .clone();
            if c.archived {
                return Err(AppError::conflict());
            }
            match action {
                "practice" => {
                    if app.occurrence.is_some() {
                        return Err(AppError::new(
                            409,
                            "Finish your current question before starting another practice.",
                        ));
                    }
                    let q = app
                        .questions
                        .values()
                        .filter(|q| q.concept_id == entity && !q.archived)
                        .min_by_key(|q| q.card.due_ms)
                        .ok_or_else(|| AppError::new(422, "This idea has no active practice."))?
                        .clone();
                    // Deliberate extra practice never changes eligibility or masquerades as cold recall.
                    app.concepts.get_mut(entity).unwrap().exposure_ms = Some(now);
                    present(app, &q, "question", now, seed, true);
                    return Ok(result("/", None));
                }
                "archive" => {
                    app.concepts.get_mut(entity).unwrap().archived = true;
                    let questions: Vec<_> = app
                        .questions
                        .values()
                        .filter(|q| q.concept_id == entity)
                        .map(|q| q.id.clone())
                        .collect();
                    for question in questions {
                        invalidate_pending(
                            app,
                            &question,
                            "This idea was archived. Your saved answer was not graded.",
                        );
                    }
                }
                "feedback" => {
                    add_feedback(
                        app,
                        &c.goal_id,
                        (Some(entity.into()), None),
                        "confusion",
                        field(fields, "text"),
                        now,
                        seed,
                    )?;
                }
                _ => return Err(AppError::new(404, "That action does not exist.")),
            }
            Ok(result(
                if action == "archive" {
                    "/map".into()
                } else {
                    format!("/concepts/{entity}")
                },
                None,
            ))
        }
        "questions" => {
            let q = app
                .questions
                .get(entity)
                .ok_or_else(|| AppError::new(404, "That question does not exist."))?
                .clone();
            if q.archived {
                return Err(AppError::conflict());
            }
            match action {
                "edit" => {
                    if app.protected_concept(&q.concept_id) {
                        return Err(AppError::new(
                            409,
                            "Choose Look it up before opening answer-bearing material.",
                        ));
                    }
                    if version(fields)? != q.content().version {
                        return Err(AppError::conflict());
                    }
                    let draft = field(fields, "use_draft") == "true"
                        && q.draft_for_version == Some(q.content().version);
                    let base = if draft {
                        q.draft.as_ref().ok_or_else(AppError::conflict)?
                    } else {
                        q.content()
                    };
                    let choices = field(fields, "choices")
                        .lines()
                        .filter(|s| !s.trim().is_empty())
                        .map(|s| s.trim().to_string())
                        .collect();
                    let variants = field(fields, "variants")
                        .lines()
                        .filter(|s| !s.trim().is_empty())
                        .map(|s| s.trim().to_string())
                        .collect();
                    let preserve = field(fields, "prompt") == base.prompt
                        && field(fields, "answer") == base.answer
                        && field(fields, "quotes")
                            .lines()
                            .map(str::to_string)
                            .collect::<Vec<_>>()
                            == base.quotes;
                    let required_ideas = if preserve {
                        base.required_ideas.clone()
                    } else {
                        Vec::new()
                    };
                    let kind = if field(fields, "kind") == "explain" && !preserve {
                        "recall"
                    } else {
                        field(fields, "kind")
                    };
                    let generated = generation::GeneratedQuestion {
                        prompt: field(fields, "prompt").into(),
                        kind: kind.into(),
                        answer: field(fields, "answer").into(),
                        variants,
                        choices,
                        explanation: field(fields, "explanation").into(),
                        basis: field(fields, "basis").into(),
                        quotes: field(fields, "quotes")
                            .lines()
                            .filter(|s| !s.is_empty())
                            .map(str::to_string)
                            .collect(),
                        required_ideas,
                        contradictions: if preserve {
                            base.contradictions.clone()
                        } else {
                            Vec::new()
                        },
                    };
                    let goal = &app.goals[&q.goal_id];
                    let material = goal.transcript.as_deref().unwrap_or(&goal.intent);
                    generation::validate_question(&generated, material)
                        .map_err(|s| AppError::new(422, s))?;
                    let target = app.questions.get_mut(entity).unwrap();
                    target.versions.push(generation::content(
                        &generated,
                        "learner",
                        now,
                        q.content().version + 1,
                    ));
                    target.draft = None;
                    target.draft_for_version = None;
                    // Presentation remains immutable, but an ungraded outdated occurrence can no longer write.
                    invalidate_pending(
                        app,
                        entity,
                        "The question changed. Your saved answer was not graded.",
                    );
                }
                "archive" => {
                    app.questions.get_mut(entity).unwrap().archived = true;
                    invalidate_pending(
                        app,
                        entity,
                        "The question was archived. Your saved answer was not graded.",
                    );
                }
                "dispute" => {
                    let reset = field(fields, "reset") == "true";
                    if reset
                        && (version(fields)? != q.content().version
                            || field(fields, "schedule_version").parse::<u64>().ok()
                                != Some(q.schedule_version))
                    {
                        return Err(AppError::conflict());
                    }
                    add_feedback(
                        app,
                        &q.goal_id,
                        (Some(q.concept_id.clone()), Some(entity.into())),
                        "dispute",
                        field(fields, "text"),
                        now,
                        seed,
                    )?;
                    if reset {
                        let after = learning::new_card(now);
                        let next_version = q.schedule_version.checked_add(1).ok_or_else(|| {
                            AppError::new(422, "The schedule version is exhausted.")
                        })?;
                        app.resets.push(Reset {
                            id: id(seed, "reset"),
                            question_id: q.id.clone(),
                            concept_id: q.concept_id.clone(),
                            goal_id: q.goal_id.clone(),
                            source_revision: app.goals[&q.goal_id].revision,
                            content_version: q.content().version,
                            schedule_version_before: q.schedule_version,
                            schedule_version_after: next_version,
                            before_card: q.card.clone(),
                            after_card: after.clone(),
                            created_ms: now,
                            feedback_id: id(seed, "feedback"),
                        });
                        let target = app.questions.get_mut(entity).unwrap();
                        target.card = after;
                        target.schedule_version = next_version;
                        invalidate_pending(app, entity, "The schedule was deliberately reset.");
                    }
                }
                "fix" => {
                    create_job(app, &q.goal_id, "fix", Some(entity.into()), now, seed)?;
                }
                _ => return Err(AppError::new(404, "That action does not exist.")),
            }
            Ok(result(format!("/questions/{entity}/edit"), None))
        }
        _ => Err(AppError::new(404, "That action does not exist.")),
    }
}

fn add_feedback(
    app: &mut App,
    goal: &str,
    target: (Option<String>, Option<String>),
    kind: &str,
    text: &str,
    now: i64,
    seed: &str,
) -> AppResult<()> {
    if !generation::text_valid(text, 4000) {
        return Err(AppError::new(
            422,
            "Describe what needs to change, up to 4,000 bytes.",
        ));
    }
    app.feedback.push(Feedback {
        id: id(seed, "feedback"),
        goal_id: goal.into(),
        concept_id: target.0,
        question_id: target.1,
        kind: kind.into(),
        text: text.into(),
        created_ms: now,
    });
    Ok(())
}

fn invalidate_pending(app: &mut App, question: &str, message: &str) {
    for a in app
        .assessments
        .values_mut()
        .filter(|a| a.question_id == question && matches!(a.status.as_str(), "queued" | "sent"))
    {
        a.status = "superseded".into();
        a.error = Some(message.into());
    }
    if app
        .occurrence
        .as_ref()
        .is_some_and(|o| o.question_id == question && o.phase != "result")
    {
        app.occurrence = None;
    }
}

fn present(app: &mut App, q: &Question, phase: &str, now: i64, seed: &str, assisted: bool) {
    app.occurrence = Some(Occurrence {
        id: id(seed, "occurrence"),
        question_id: q.id.clone(),
        concept_id: q.concept_id.clone(),
        presentation: q.content().clone(),
        phase: phase.into(),
        assisted,
        before_card: q.card.clone(),
        schedule_version: q.schedule_version,
        created_ms: now,
        answer: None,
        assessment_id: None,
        result: None,
    });
}

pub fn ensure_occurrence(app: &mut App, now: i64, seed: &str) -> AppResult<()> {
    if app.schema != SCHEMA {
        return Err(AppError::new(
            503,
            "This learning record needs a compatible application version.",
        ));
    }
    if let Some(o) = &app.occurrence {
        if o.phase == "result"
            || app.questions.get(&o.question_id).is_some_and(|q| {
                !q.archived
                    && q.content().version == o.presentation.version
                    && q.schedule_version == o.schedule_version
                    && app.goals.get(&q.goal_id).is_some_and(|g| !g.archived)
                    && app.concepts.get(&q.concept_id).is_some_and(|c| !c.archived)
            })
        {
            return Ok(());
        }
        app.occurrence = None;
    }
    let focused = app
        .goals
        .values()
        .any(|g| g.focused && !g.paused && !g.archived);
    let cap = match app.preferences.pace.as_str() {
        "light" => 3,
        "intense" => 12,
        _ => 6,
    };
    let introduced = app
        .concepts
        .values()
        .filter(|c| c.introduced_ms.is_some_and(|t| t > now - DAY_MS))
        .count();
    let mut candidates = Vec::new();
    for q in app.questions.values() {
        let Some(g) = app.goals.get(&q.goal_id) else {
            continue;
        };
        let Some(c) = app.concepts.get(&q.concept_id) else {
            continue;
        };
        if q.archived || c.archived || g.archived || q.card.due_ms > now {
            continue;
        }
        if app.events.iter().rev().any(|e| {
            e.question_id == q.id && e.result.rating == 0 && e.result.reviewed_ms > now - DAY_MS
        }) {
            continue;
        }
        let new = c.introduced_ms.is_none();
        if q.card.state == learning::State::New && (g.paused || (focused && !g.focused)) {
            continue;
        }
        if new
            && (introduced >= cap
                || c.prerequisites.iter().any(|p| {
                    app.concepts
                        .get(p)
                        .is_none_or(|p| p.introduced_ms.is_none() || p.archived)
                }))
        {
            continue;
        }
        candidates.push((new, q.card.due_ms, c.prerequisites.len(), q.id.clone()));
    }
    candidates.sort();
    if let Some((new, _, _, qid)) = candidates.first() {
        let q = app.questions[qid].clone();
        let assisted = app.concepts[&q.concept_id]
            .exposure_ms
            .is_some_and(|t| t > now - DAY_MS);
        present(
            app,
            &q,
            if *new { "intro" } else { "question" },
            now,
            seed,
            assisted,
        );
        app.revision += 1;
    }
    Ok(())
}

fn reserve(app: &mut App, work_id: &str, lease: &str, model: &str, now: i64) -> AppResult<()> {
    if app.spent(now).saturating_add(RESERVATION) > DAILY_ALLOWANCE {
        return Err(AppError::new(
            429,
            "Today's preparation and checking allowance is used. Your work is saved; try again later.",
        ));
    }
    app.spend.push(Spend {
        id: lease.into(),
        work_id: work_id.into(),
        created_ms: now,
        reserved_micros: RESERVATION,
        cost_micros: None,
        status: "unknown".into(),
        model: model.into(),
        response_hash: None,
        response_model: None,
        raw_response: None,
        response_bytes: None,
        response_outcome: None,
    });
    Ok(())
}

pub fn claim_work(
    app: &mut App,
    now: i64,
    lease: &str,
    config: &WorkConfig,
) -> AppResult<Option<Work>> {
    if app.restored_paused {
        return Ok(None);
    }
    recover(app, now);
    // Assessments take priority over generation, without blocking existing study.
    let assessment = app
        .assessments
        .values()
        .filter(|a| a.status == "queued")
        .min_by_key(|a| a.created_ms)
        .cloned();
    if let Some(a) = assessment {
        let Some(o) = app
            .occurrence
            .as_ref()
            .filter(|o| {
                o.id == a.occurrence_id
                    && o.assessment_id.as_deref() == Some(&a.id)
                    && o.phase == "checking"
            })
            .cloned()
        else {
            app.assessments.get_mut(&a.id).unwrap().status = "superseded".into();
            app.revision += 1;
            return Ok(None);
        };
        if !config.jev {
            assessment_failed(
                app,
                &a.id,
                "Meaning checking is not configured. Your answer is saved; please check it yourself.",
                false,
                now,
            );
            app.revision += 1;
            return Ok(None);
        }
        if let Err(e) = reserve(app, &a.id, lease, &config.jev_model, now) {
            assessment_failed(app, &a.id, &e.message, false, now);
            app.revision += 1;
            return Ok(None);
        }
        let a_mut = app.assessments.get_mut(&a.id).unwrap();
        a_mut.status = "sent".into();
        a_mut.lease = Some(lease.into());
        a_mut.started_ms = Some(now);
        app.revision += 1;
        return Ok(Some(Work {
            id: a.id,
            lease: lease.into(),
            kind: "assessment".into(),
            request: generation::assessment_request(&o, &a.answer, &config.jev_model),
            photo_key: None,
        }));
    }
    let job = app
        .jobs
        .values()
        .filter(|j| matches!(j.status.as_str(), "queued" | "candidates"))
        .min_by_key(|j| j.created_ms)
        .cloned();
    let Some(j) = job else { return Ok(None) };
    let goal = app
        .goals
        .get(&j.goal_id)
        .ok_or_else(AppError::conflict)?
        .clone();
    if goal.archived
        || goal.revision != j.source_revision
        || j.question_id.as_ref().is_some_and(|id| {
            app.questions
                .get(id)
                .is_none_or(|q| q.archived || Some(q.content().version) != j.question_version)
        })
    {
        app.jobs.get_mut(&j.id).unwrap().status = "superseded".into();
        app.revision += 1;
        return Ok(None);
    }
    if j.status == "candidates" {
        let batch: Batch = serde_json::from_str(j.candidate_json.as_deref().unwrap_or(""))
            .map_err(|_| AppError::new(422, "Saved candidates could not be read."))?;
        let cache = generation::read_critic_cache(&batch, j.critic_cache_json.as_deref())?;
        let request = generation::critic_request_remaining(&batch, &config.jev_model, &cache)?;
        if request["questions"]
            .as_object()
            .expect("critic questions")
            .is_empty()
        {
            finish_critic_candidates(
                app,
                &j,
                batch,
                &cache,
                j.model.as_deref().unwrap_or("unreported"),
                now,
            )?;
            app.revision += 1;
            return Ok(None);
        }
        if !config.critic {
            if !cache.answers.is_empty() {
                job_failed(
                    app,
                    &j.id,
                    "Saved content checks remain unfinished. Enable content checking to complete only the remaining judgments.",
                    false,
                );
                app.revision += 1;
                return Ok(None);
            }
            if let Err(error) = publish(
                app,
                &j,
                &batch,
                j.model.as_deref().unwrap_or("unreported"),
                now,
            ) {
                job_failed(app, &j.id, &error.message, false);
            } else {
                app.jobs.get_mut(&j.id).unwrap().critic_json =
                    Some("{\"status\":\"skipped\"}".into());
            }
            app.revision += 1;
            return Ok(None);
        }
        if !config.jev {
            job_failed(
                app,
                &j.id,
                "The content check is configured but unavailable. Candidates are saved.",
                false,
            );
            app.revision += 1;
            return Ok(None);
        }
        if let Err(e) = reserve(app, &j.id, lease, &config.jev_model, now) {
            job_failed(app, &j.id, &e.message, false);
            app.revision += 1;
            return Ok(None);
        }
        let jm = app.jobs.get_mut(&j.id).unwrap();
        jm.status = "critic-sent".into();
        jm.lease = Some(lease.into());
        jm.started_ms = Some(now);
        jm.critic_cache_json =
            Some(serde_json::to_string(&cache).expect("critic cache serialization"));
        app.revision += 1;
        return Ok(Some(Work {
            id: j.id,
            lease: lease.into(),
            kind: "critic".into(),
            request,
            photo_key: None,
        }));
    }
    if !config.generation {
        job_failed(
            app,
            &j.id,
            "Preparation is not configured. Your input is saved; you can try again when it is available.",
            false,
        );
        app.revision += 1;
        return Ok(None);
    }
    if let Err(e) = reserve(app, &j.id, lease, &config.model, now) {
        job_failed(app, &j.id, &e.message, false);
        app.revision += 1;
        return Ok(None);
    }
    let transcribe = goal.photo.is_some() && goal.transcript.is_none();
    let request = if transcribe {
        generation::transcription_request(&goal, &config.model)
    } else {
        generation::batch_request(app, &j, &config.model)
    };
    let jm = app.jobs.get_mut(&j.id).unwrap();
    jm.status = "sent".into();
    jm.lease = Some(lease.into());
    jm.started_ms = Some(now);
    jm.model = Some(config.model.clone());
    app.revision += 1;
    Ok(Some(Work {
        id: j.id,
        lease: lease.into(),
        kind: if transcribe {
            "transcribe"
        } else {
            "generation"
        }
        .into(),
        request,
        photo_key: if transcribe {
            goal.photo.map(|p| p.key)
        } else {
            None
        },
    }))
}

fn assessment_failed(app: &mut App, id: &str, message: &str, unknown: bool, now: i64) {
    if let Some(a) = app.assessments.get_mut(id) {
        a.status = if unknown { "unknown" } else { "failed" }.into();
        a.error = Some(message.into());
    }
    if let Some(o) = app
        .occurrence
        .as_mut()
        .filter(|o| o.assessment_id.as_deref() == Some(id) && o.phase == "checking")
    {
        o.phase = "self-check".into();
        if let Some(c) = app.concepts.get_mut(&o.concept_id) {
            c.exposure_ms = Some(now);
        }
    }
}
fn job_failed(app: &mut App, id: &str, message: &str, unknown: bool) {
    if let Some(j) = app.jobs.get_mut(id) {
        j.status = if unknown { "unknown" } else { "failed" }.into();
        j.error = Some(message.into());
        if j.kind != "fix"
            && let Some(g) = app.goals.get_mut(&j.goal_id)
        {
            g.status = j.status.clone();
        }
    }
}

pub fn recover(app: &mut App, now: i64) -> bool {
    let mut changed = false;
    let jobs: Vec<_> = app
        .jobs
        .values()
        .filter(|j| {
            matches!(j.status.as_str(), "sent" | "critic-sent")
                && j.started_ms.is_some_and(|t| now - t >= 90_000)
        })
        .map(|j| j.id.clone())
        .collect();
    for j in jobs {
        job_failed(
            app,
            &j,
            "The request stopped before its outcome was confirmed. Its reserved usage is retained. Ask deliberately to try again.",
            true,
        );
        changed = true;
    }
    let assessments: Vec<_> = app
        .assessments
        .values()
        .filter(|a| a.status == "sent" && a.started_ms.is_some_and(|t| now - t >= 90_000))
        .map(|a| a.id.clone())
        .collect();
    for a in assessments {
        assessment_failed(
            app,
            &a,
            "The check stopped before its outcome was confirmed. Its reserved usage is retained. You can check the answer yourself.",
            true,
            now,
        );
        changed = true;
    }
    if changed {
        app.revision += 1
    }
    changed
}

fn settle(app: &mut App, work: &Work, cost: Option<u64>) {
    if let Some(s) = app.spend.iter_mut().find(|s| s.id == work.lease) {
        s.cost_micros = cost;
        s.status = if cost.is_some() { "known" } else { "unknown" }.into();
    }
}

/// A stale result may be unusable for learning, but its paid outcome still belongs to the ledger.
pub fn finish_work(
    app: &mut App,
    work: &Work,
    raw: String,
    model: String,
    cost_micros: Option<u64>,
    now: i64,
) -> AppResult<()> {
    if replay_response(app, work, &raw, &model, cost_micros, "received")? {
        return Ok(());
    }
    let outcome = finish_work_inner(app, work, raw.clone(), model.clone(), cost_micros, now);
    if let Err(error) = &outcome
        && app
            .spend
            .iter()
            .any(|s| s.id == work.lease && s.work_id == work.id)
    {
        settle(app, work, cost_micros);
        let mut end = raw.len().min(generation::RESPONSE_LIMIT);
        while !raw.is_char_boundary(end) {
            end -= 1;
        }
        let bounded = raw[..end].to_owned();
        if work.kind == "assessment" {
            if let Some(a) = app
                .assessments
                .get_mut(&work.id)
                .filter(|a| a.lease.as_deref() == Some(&work.lease))
            {
                a.raw_response = Some(bounded);
                a.status = if error.status == 409 {
                    "superseded"
                } else {
                    "failed"
                }
                .into();
                a.error = Some(error.message.clone());
            }
            if error.status != 409 {
                assessment_failed(app, &work.id, &error.message, false, now);
            }
        } else if let Some(j) = app
            .jobs
            .get_mut(&work.id)
            .filter(|j| j.lease.as_deref() == Some(&work.lease))
        {
            if work.kind == "critic" {
                j.critic_json = Some(bounded);
                j.critic_model = Some(model.clone());
                // A publication fence does not erase complete paid judgments.
                // They remain bound to these immutable candidate bytes, even
                // when the old source revision can no longer publish them.
                if let Some(batch) = j
                    .candidate_json
                    .as_deref()
                    .and_then(|v| serde_json::from_str::<Batch>(v).ok())
                    && let Ok(saved) =
                        generation::read_critic_cache(&batch, j.critic_cache_json.as_deref())
                    && let Ok(cache) = generation::merge_critic_cache(
                        &batch,
                        &saved,
                        j.critic_json.as_deref().unwrap_or(""),
                    )
                {
                    j.critic_cache_json =
                        Some(serde_json::to_string(&cache).expect("critic cache serialization"));
                }
            } else {
                j.raw_response = Some(bounded);
                j.model = Some(model.clone());
            }
            j.status = if error.status == 409 {
                "superseded"
            } else {
                "failed"
            }
            .into();
            j.error = Some(error.message.clone());
            if j.kind != "fix"
                && j.status == "failed"
                && let Some(goal) = app.goals.get_mut(&j.goal_id)
            {
                goal.status = "failed".into();
            }
        }
        app.revision += 1;
    }
    record_response(app, work, &raw, &model, cost_micros, "received");
    outcome
}

fn replay_response(
    app: &App,
    work: &Work,
    raw: &str,
    model: &str,
    cost: Option<u64>,
    outcome: &str,
) -> AppResult<bool> {
    let receipt = app
        .spend
        .iter()
        .find(|s| s.id == work.lease && s.work_id == work.id)
        .ok_or_else(AppError::conflict)?;
    if let Some(hash) = &receipt.response_hash {
        if hash == &hex::encode(Sha256::digest(raw.as_bytes()))
            && receipt.response_model.as_deref() == Some(model)
            && receipt.cost_micros == cost
            && receipt.response_outcome.as_deref() == Some(outcome)
        {
            return Ok(true);
        }
        return Err(AppError::conflict());
    }
    Ok(false)
}

fn record_response(
    app: &mut App,
    work: &Work,
    raw: &str,
    model: &str,
    cost: Option<u64>,
    outcome: &str,
) {
    settle(app, work, cost);
    if let Some(receipt) = app
        .spend
        .iter_mut()
        .find(|s| s.id == work.lease && s.work_id == work.id)
    {
        let mut end = raw.len().min(generation::RESPONSE_LIMIT);
        while !raw.is_char_boundary(end) {
            end -= 1;
        }
        receipt.raw_response = Some(raw[..end].to_owned());
        receipt.response_bytes = Some(raw.len());
        receipt.response_hash = Some(hex::encode(Sha256::digest(raw.as_bytes())));
        receipt.response_model = Some(model.into());
        receipt.response_outcome = Some(outcome.into());
    }
}

/// Preserve a received non-success HTTP outcome without interpreting its body
/// as generated material or a judgment, even when it resembles success JSON.
pub fn fail_received_work(
    app: &mut App,
    work: &Work,
    now: i64,
    raw: String,
    model: String,
    cost_micros: Option<u64>,
    message: &str,
) -> AppResult<()> {
    if replay_response(app, work, &raw, &model, cost_micros, "rejected")? {
        return Ok(());
    }
    let mut next = app.clone();
    let mut end = raw.len().min(generation::RESPONSE_LIMIT);
    while !raw.is_char_boundary(end) {
        end -= 1;
    }
    let bounded = raw[..end].to_owned();
    if work.kind == "assessment" {
        let assessment = next
            .assessments
            .get_mut(&work.id)
            .filter(|a| a.lease.as_deref() == Some(work.lease.as_str()))
            .ok_or_else(AppError::conflict)?;
        assessment.raw_response = Some(bounded);
        assessment_failed(&mut next, &work.id, message, false, now);
    } else {
        let job = next
            .jobs
            .get_mut(&work.id)
            .filter(|j| j.lease.as_deref() == Some(work.lease.as_str()))
            .ok_or_else(AppError::conflict)?;
        if work.kind == "critic" {
            job.critic_json = Some(bounded);
            job.critic_model = Some(model.clone());
        } else {
            job.raw_response = Some(bounded);
            job.model = Some(model.clone());
        }
        let goal = next
            .goals
            .get(&job.goal_id)
            .ok_or_else(AppError::conflict)?;
        let stale = goal.archived
            || goal.revision != job.source_revision
            || (job.kind != "fix" && goal.job_id != job.id)
            || job.status == "superseded";
        if stale {
            job.status = "superseded".into();
            job.error = Some(message.into());
        } else {
            job_failed(&mut next, &work.id, message, false);
        }
    }
    record_response(&mut next, work, &raw, &model, cost_micros, "rejected");
    next.revision += 1;
    *app = next;
    Ok(())
}

fn finish_work_inner(
    app: &mut App,
    work: &Work,
    raw: String,
    model: String,
    cost_micros: Option<u64>,
    now: i64,
) -> AppResult<()> {
    let mut next = app.clone();
    settle(&mut next, work, cost_micros);
    if raw.len() > generation::RESPONSE_LIMIT {
        return Err(AppError::new(
            422,
            "The response exceeded the safe size limit. Its paid outcome was saved.",
        ));
    }
    if work.kind == "assessment" {
        let a = next
            .assessments
            .get(&work.id)
            .filter(|a| a.status == "sent" && a.lease.as_deref() == Some(&work.lease))
            .ok_or_else(AppError::conflict)?
            .clone();
        let o = next
            .occurrence
            .as_ref()
            .filter(|o| {
                o.id == a.occurrence_id
                    && o.assessment_id.as_deref() == Some(&a.id)
                    && o.phase == "checking"
            })
            .ok_or_else(AppError::conflict)?
            .clone();
        let q = next
            .questions
            .get(&a.question_id)
            .ok_or_else(AppError::conflict)?;
        if q.archived
            || q.content().version != a.content_version
            || q.schedule_version != a.schedule_version
            || next.goals.get(&q.goal_id).is_none_or(|g| g.archived)
            || next.concepts.get(&q.concept_id).is_none_or(|c| c.archived)
        {
            return Err(AppError::conflict());
        }
        next.assessments.get_mut(&a.id).unwrap().raw_response = Some(raw.clone());
        match generation::assessment_decision(&raw, &o.presentation) {
            Ok(Some(correct)) => {
                next.assessments.get_mut(&a.id).unwrap().status = "complete".into();
                commit_grade(&mut next, correct, false, "jev", now, &work.lease)?;
            }
            Ok(None) => {
                next.assessments.get_mut(&a.id).unwrap().status = "unsure".into();
                next.occurrence.as_mut().unwrap().phase = "self-check".into();
                next.concepts.get_mut(&o.concept_id).unwrap().exposure_ms = Some(now);
            }
            Err(e) => {
                assessment_failed(&mut next, &a.id, &e.message, false, now);
            }
        }
    } else {
        let j = next
            .jobs
            .get(&work.id)
            .filter(|j| {
                matches!(j.status.as_str(), "sent" | "critic-sent")
                    && j.lease.as_deref() == Some(&work.lease)
            })
            .ok_or_else(AppError::conflict)?
            .clone();
        let goal = next.goals.get(&j.goal_id).ok_or_else(AppError::conflict)?;
        if goal.archived || goal.revision != j.source_revision {
            return Err(AppError::conflict());
        }
        match work.kind.as_str() {
            "transcribe" => {
                let outcome = generation::openrouter_content(&raw).and_then(|text| {
                    serde_json::from_str::<Value>(&text)
                        .map_err(|_| AppError::new(422, "The photo transcription was unreadable."))
                });
                match outcome {
                    Ok(v)
                        if v.get("text")
                            .and_then(Value::as_str)
                            .is_some_and(|t| generation::text_valid(t, TEXT_LIMIT)) =>
                    {
                        next.goals.get_mut(&j.goal_id).unwrap().transcript =
                            Some(v["text"].as_str().unwrap().into());
                        let jm = next.jobs.get_mut(&j.id).unwrap();
                        jm.status = "queued".into();
                        jm.lease = None;
                        jm.started_ms = None;
                        jm.raw_response = Some(raw);
                    }
                    _ => {
                        next.jobs.get_mut(&j.id).unwrap().raw_response = Some(raw);
                        job_failed(
                            &mut next,
                            &j.id,
                            "The photo could not be read. Your original photo is saved.",
                            false,
                        );
                    }
                }
            }
            "generation" => {
                let source = goal.transcript.as_deref().unwrap_or(&goal.intent);
                let existing = goal_graph(&next, &j.goal_id);
                let outcome = generation::openrouter_content(&raw)
                    .and_then(|text| {
                        serde_json::from_str::<Batch>(&text).map_err(|_| {
                            AppError::new(
                                422,
                                "Preparation did not return a usable reference and practice.",
                            )
                        })
                    })
                    .and_then(|b| generation::validate_batch_with_existing(b, source, &existing));
                next.jobs.get_mut(&j.id).unwrap().raw_response = Some(raw);
                match outcome {
                    Ok((batch, rejected)) => {
                        let jm = next.jobs.get_mut(&j.id).unwrap();
                        jm.status = "candidates".into();
                        jm.candidate_json = Some(serde_json::to_string(&batch).unwrap());
                        jm.rejected = rejected;
                        jm.model = Some(model);
                        jm.lease = None;
                        jm.started_ms = None;
                    }
                    Err(e) => job_failed(&mut next, &j.id, &e.message, false),
                }
            }
            "critic" => {
                let batch: Batch = serde_json::from_str(j.candidate_json.as_deref().unwrap_or(""))
                    .map_err(|_| AppError::new(422, "Saved candidates could not be read."))?;
                let saved = generation::read_critic_cache(&batch, j.critic_cache_json.as_deref())?;
                let jm = next.jobs.get_mut(&j.id).unwrap();
                jm.critic_json = Some(raw.clone());
                jm.critic_model = Some(model.clone());
                match generation::merge_critic_cache(&batch, &saved, &raw) {
                    Ok(cache) => {
                        next.jobs.get_mut(&j.id).unwrap().critic_cache_json = Some(
                            serde_json::to_string(&cache).expect("critic cache serialization"),
                        );
                        finish_critic_candidates(
                            &mut next,
                            &j,
                            batch,
                            &cache,
                            j.model.as_deref().unwrap_or("unreported"),
                            now,
                        )?;
                    }
                    Err(e) => job_failed(&mut next, &j.id, &e.message, false),
                }
            }
            _ => return Err(AppError::new(422, "Unsupported work type.")),
        }
    }
    next.revision += 1;
    *app = next;
    Ok(())
}

pub fn fail_work(
    app: &mut App,
    work: &Work,
    message: &str,
    unknown: bool,
    _now: i64,
) -> AppResult<()> {
    if work.kind == "assessment" {
        let a = app
            .assessments
            .get(&work.id)
            .ok_or_else(AppError::conflict)?;
        if a.status != "sent" || a.lease.as_deref() != Some(&work.lease) {
            return Err(AppError::conflict());
        }
        assessment_failed(app, &work.id, message, unknown, _now);
    } else {
        let j = app.jobs.get(&work.id).ok_or_else(AppError::conflict)?;
        if !matches!(j.status.as_str(), "sent" | "critic-sent")
            || j.lease.as_deref() != Some(&work.lease)
        {
            return Err(AppError::conflict());
        }
        job_failed(app, &work.id, message, unknown);
    }
    // Once claimed, a failed or rejected send has no proven zero-cost settlement.
    // Keep its conservative reservation until a known provider cost is recorded.
    app.revision += 1;
    Ok(())
}

fn publish(app: &mut App, job: &Job, batch: &Batch, model: &str, now: i64) -> AppResult<()> {
    let mut next = app.clone();
    publish_inner(&mut next, job, batch, model, now)?;
    *app = next;
    Ok(())
}

fn publish_inner(app: &mut App, job: &Job, batch: &Batch, model: &str, now: i64) -> AppResult<()> {
    let goal = &app.goals[&job.goal_id];
    if goal.archived || goal.revision != job.source_revision {
        return Err(AppError::conflict());
    }
    let source = goal.transcript.as_deref().unwrap_or(&goal.intent);
    let (batch, rejected) = generation::validate_batch_with_existing(
        batch.clone(),
        source,
        &goal_graph(app, &job.goal_id),
    )?;
    if !rejected.is_empty() {
        return Err(AppError::new(
            422,
            "Saved candidates no longer pass publication validation. Your material is retained.",
        ));
    }
    if let Some(qid) = &job.question_id {
        if batch.concepts.len() != 1 || batch.concepts[0].questions.len() != 1 {
            return Err(AppError::new(
                422,
                "The fix returned more than one question. Its candidates are retained; no draft was chosen silently.",
            ));
        }
        let q = app
            .questions
            .get_mut(qid)
            .filter(|q| !q.archived)
            .ok_or_else(AppError::conflict)?;
        if Some(q.content().version) != job.question_version {
            return Err(AppError::conflict());
        }
        let first = batch
            .concepts
            .first()
            .and_then(|c| c.questions.first())
            .ok_or_else(|| AppError::new(422, "The fix did not produce a question draft."))?;
        q.draft = Some(generation::content(
            first,
            model,
            now,
            q.content().version + 1,
        ));
        q.draft_for_version = Some(q.content().version);
    } else {
        let mut mapping: BTreeMap<String, String> = app
            .concepts
            .values()
            .filter(|c| c.goal_id == job.goal_id)
            .map(|c| (c.id.clone(), c.id.clone()))
            .collect();
        for c in &batch.concepts {
            if app
                .concepts
                .get(&c.key)
                .is_some_and(|c| c.goal_id == job.goal_id && c.archived)
            {
                return Err(AppError::new(
                    422,
                    "Refinement referred to an archived idea. Its candidates are retained.",
                ));
            }
            let existing = app
                .concepts
                .get(&c.key)
                .filter(|c| c.goal_id == job.goal_id && !c.archived);
            mapping.insert(
                c.key.clone(),
                existing
                    .map(|c| c.id.clone())
                    .unwrap_or_else(|| id(&job.id, &format!("concept:{}", c.key))),
            );
        }
        for c in &batch.concepts {
            let cid = mapping[&c.key].clone();
            let prerequisites = c
                .prerequisites
                .iter()
                .map(|key| {
                    mapping.get(key).cloned().ok_or_else(|| {
                        AppError::new(
                            422,
                            "A prerequisite could not be resolved to saved reference material.",
                        )
                    })
                })
                .collect::<AppResult<Vec<_>>>()?;
            if prerequisites
                .iter()
                .any(|id| app.concepts.get(id).is_some_and(|c| c.archived))
            {
                return Err(AppError::new(
                    422,
                    "Refinement depended on an archived idea. Its candidates are retained.",
                ));
            }
            let note = Note {
                text: c.note.clone(),
                basis: c.basis.clone(),
                quotes: c.quotes.clone(),
                created_ms: now,
                model: model.into(),
            };
            if let Some(existing) = app.concepts.get_mut(&cid) {
                existing.notes.push(note);
                existing.summary = c.summary.clone();
                existing.name = c.name.clone();
                existing.prerequisites = prerequisites;
            } else {
                app.concepts.insert(
                    cid.clone(),
                    Concept {
                        id: cid.clone(),
                        goal_id: job.goal_id.clone(),
                        name: c.name.clone(),
                        summary: c.summary.clone(),
                        notes: vec![note],
                        prerequisites,
                        introduced_ms: None,
                        exposure_ms: None,
                        already_known: false,
                        archived: false,
                    },
                );
                app.goals
                    .get_mut(&job.goal_id)
                    .unwrap()
                    .concept_ids
                    .push(cid.clone());
            }
            for (i, q) in c.questions.iter().enumerate() {
                let qid = id(&job.id, &format!("question:{}:{i}", c.key));
                let content = generation::content(q, model, now, 1);
                if app.questions.values().any(|q| {
                    q.concept_id == cid
                        && !q.archived
                        && q.content().prompt.trim() == content.prompt.trim()
                }) {
                    continue;
                }
                app.questions.insert(
                    qid.clone(),
                    Question::new(qid, job.goal_id.clone(), cid.clone(), content, now),
                );
            }
        }
        let g = app.goals.get_mut(&job.goal_id).unwrap();
        g.title = batch.title.clone();
        g.status = if batch.complete && app.jobs[&job.id].rejected.is_empty() {
            "ready"
        } else {
            "partial"
        }
        .into();
    }
    let j = app.jobs.get_mut(&job.id).unwrap();
    j.status = if batch.complete && j.rejected.is_empty() {
        "ready"
    } else {
        "partial"
    }
    .into();
    j.error = None;
    j.lease = None;
    Ok(())
}

fn goal_graph(app: &App, goal_id: &str) -> BTreeMap<String, Vec<String>> {
    app.concepts
        .values()
        .filter(|c| c.goal_id == goal_id)
        .map(|c| (c.id.clone(), c.prerequisites.clone()))
        .collect()
}

fn finish_critic_candidates(
    app: &mut App,
    job: &Job,
    batch: Batch,
    cache: &generation::CriticCache,
    model: &str,
    now: i64,
) -> AppResult<()> {
    for reason in generation::critic_rejections(&batch, cache)? {
        let rejected = &mut app.jobs.get_mut(&job.id).unwrap().rejected;
        if !rejected.contains(&reason) {
            rejected.push(reason);
        }
    }
    let outcome = generation::apply_critic_cache(batch, cache).and_then(|(batch, mut rejected)| {
        let goal = &app.goals[&job.goal_id];
        let source = goal.transcript.as_deref().unwrap_or(&goal.intent);
        let (batch, validation) = generation::validate_batch_with_existing(
            batch,
            source,
            &goal_graph(app, &job.goal_id),
        )?;
        rejected.extend(validation);
        Ok((batch, rejected))
    });
    match outcome {
        Ok((batch, rejected)) => {
            let intent = &app.goals[&job.goal_id].intent;
            let exact = [
                "verbatim",
                "exact wording",
                "complete set",
                "every item",
                "all items",
                "word for word",
            ]
            .iter()
            .any(|word| intent.to_lowercase().contains(word));
            if exact && !rejected.is_empty() {
                job_failed(
                    app,
                    &job.id,
                    "Some requested material failed its content check. The complete set was not published.",
                    false,
                );
            } else {
                let saved = &mut app.jobs.get_mut(&job.id).unwrap().rejected;
                for reason in rejected {
                    if !saved.contains(&reason) {
                        saved.push(reason);
                    }
                }
                if let Err(error) = publish(app, job, &batch, model, now) {
                    job_failed(app, &job.id, &error.message, false);
                }
            }
        }
        Err(error) => job_failed(app, &job.id, &error.message, false),
    }
    Ok(())
}

pub fn seed_fixture(app: &mut App, now: i64) -> AppResult<()> {
    if !app.goals.is_empty() || !app.events.is_empty() || !app.operations.is_empty() {
        return Err(AppError::new(
            409,
            "The authored fixture requires a fresh learning space.",
        ));
    }
    let gid = "fixture-http".to_string();
    let jid = "fixture-job".to_string();
    app.goals.insert(gid.clone(),Goal{id:gid.clone(),title:"Understand HTTP caching".into(),intent:"Understand why a browser can show an old version of a page, and how caches validate it.".into(),created_ms:now,revision:1,status:"ready".into(),paused:false,focused:false,archived:false,concept_ids:Vec::new(),job_id:jid.clone(),photo:None,transcript:None});
    let batch:Batch=serde_json::from_value(json!({"title":"Understand HTTP caching","complete":true,"concepts":[{"key":"freshness","name":"Cache freshness","summary":"A fresh response can be reused without asking the origin.","note":"A cache stores a response and its freshness lifetime. During that lifetime, the browser can reuse it without contacting the origin. For example, Cache-Control: max-age=60 allows reuse for sixty seconds. When it becomes stale, the browser may need to validate it; stale does not mean the underlying content changed.","basis":"general","quotes":[],"prerequisites":[],"questions":[{"prompt":"Which response directive sets how long a cached response may be reused without validation?","kind":"recall","answer":"max-age","variants":["Cache-Control: max-age"],"choices":[],"explanation":"The max-age directive specifies the freshness lifetime in seconds. Once that time passes, validation may be required.","basis":"general","quotes":[]},{"prompt":"A stored response has passed its freshness lifetime. What does that tell you?","kind":"choice","answer":"It may need validation before reuse","choices":["It may need validation before reuse","The origin content definitely changed","The browser must delete it forever"],"explanation":"Staleness is about permission to reuse the stored response without checking. The origin content may be unchanged.","basis":"general","quotes":[]}]},{"key":"validation","name":"Conditional validation","summary":"A validator lets the origin confirm that a stored response is still current.","note":"An ETag is an opaque identifier for a representation. A browser can send If-None-Match with the saved ETag. If the representation has not changed, the server can answer 304 Not Modified; the browser reuses the stored body. Freshness decides when a check is needed, while validation decides whether the saved content is still suitable.","basis":"general","quotes":[],"prerequisites":["freshness"],"questions":[{"prompt":"What HTTP status tells a browser its cached representation can be reused after a conditional request?","kind":"recall","answer":"304","variants":["304 Not Modified"],"choices":[],"explanation":"304 Not Modified confirms the conditional request's validator still matches. The browser reuses the cached body rather than receiving another full representation.","basis":"general","quotes":[]}]}]})).unwrap();
    let job = Job {
        id: jid.clone(),
        goal_id: gid,
        question_id: None,
        question_version: None,
        source_revision: 1,
        kind: "create".into(),
        status: "candidates".into(),
        created_ms: now,
        lease: None,
        started_ms: None,
        attempt: 1,
        error: None,
        raw_response: None,
        model: Some("authored-test-fixture".into()),
        feedback_ids: Vec::new(),
        rejected: Vec::new(),
        candidate_json: Some(serde_json::to_string(&batch).unwrap()),
        critic_json: Some("{\"status\":\"authored fixture\"}".into()),
        critic_cache_json: None,
        critic_model: None,
        critic_parent_id: None,
        critic_inherited_keys: Vec::new(),
    };
    app.jobs.insert(jid, job.clone());
    publish(app, &job, &batch, "authored-test-fixture", now)?;
    // This is deliberately authored, already-read synthetic material, never provider quality or cold recall proof.
    for c in app.concepts.values_mut() {
        c.introduced_ms = Some(now - DAY_MS - 1);
    }
    app.revision += 1;
    ensure_occurrence(app, now, "fixture-first")?;
    Ok(())
}

pub fn validate_app(app: &App) -> AppResult<()> {
    if app.schema != SCHEMA
        || !(24..=256).contains(&app.csrf.len())
        || !matches!(
            app.preferences.pace.as_str(),
            "light" | "steady" | "intense"
        )
    {
        return Err(archive_error("incompatible schema or owner metadata"));
    }
    let mut ids = BTreeSet::new();
    for (key, goal) in &app.goals {
        archive_id(key, &goal.id, &mut ids)?;
        let concepts: BTreeSet<_> = goal.concept_ids.iter().collect();
        if goal.revision == 0
            || goal.intent.len() > TEXT_LIMIT
            || goal.title.len() > 1000
            || goal
                .transcript
                .as_ref()
                .is_some_and(|t| !generation::text_valid(t, TEXT_LIMIT))
            || concepts.len() != goal.concept_ids.len()
            || goal
                .concept_ids
                .iter()
                .any(|id| app.concepts.get(id).is_none_or(|c| c.goal_id != goal.id))
            || app
                .jobs
                .get(&goal.job_id)
                .is_none_or(|j| j.goal_id != goal.id || j.kind == "fix")
            || !matches!(
                goal.status.as_str(),
                "preparing" | "ready" | "partial" | "failed" | "unknown" | "paused" | "archived"
            )
        {
            return Err(archive_error("broken goal references or saved input"));
        }
        if let Some(photo) = &goal.photo
            && (!photo.key.starts_with("photos/")
                || photo.size == 0
                || photo.size > PHOTO_LIMIT
                || !matches!(
                    photo.mime.as_str(),
                    "image/jpeg" | "image/png" | "image/webp"
                )
                || !archive_hash(&photo.sha256)
                || goal.intent.len() > 1024)
        {
            return Err(archive_error("invalid saved photo metadata"));
        }
    }
    for (key, concept) in &app.concepts {
        archive_id(key, &concept.id, &mut ids)?;
        let goal = app
            .goals
            .get(&concept.goal_id)
            .ok_or_else(|| archive_error("an orphaned idea"))?;
        let prerequisites: BTreeSet<_> = concept.prerequisites.iter().collect();
        if !goal.concept_ids.contains(key)
            || concept.notes.is_empty()
            || !generation::text_valid(&concept.name, 200)
            || !generation::text_valid(&concept.summary, 2000)
            || prerequisites.len() != concept.prerequisites.len()
            || concept.prerequisites.iter().any(|id| {
                id == key
                    || app
                        .concepts
                        .get(id)
                        .is_none_or(|c| c.goal_id != concept.goal_id)
            })
        {
            return Err(archive_error("broken idea references"));
        }
        let source = goal.transcript.as_deref().unwrap_or(&goal.intent);
        for note in &concept.notes {
            if !generation::text_valid(&note.text, 16_000)
                || !archive_grounded(&note.basis, &note.quotes, source)
                || note.model.len() > 256
            {
                return Err(archive_error("invalid reference material or its evidence"));
            }
        }
    }
    // Every prerequisite stays within its goal, and historical archived ideas
    // remain present. A cycle is corrupt structure, never a new learning plan.
    let mut reached = BTreeSet::new();
    for _ in 0..app.concepts.len() {
        for concept in app.concepts.values() {
            if concept.prerequisites.iter().all(|id| reached.contains(id)) {
                reached.insert(concept.id.clone());
            }
        }
    }
    if reached.len() != app.concepts.len() {
        return Err(archive_error("a circular prerequisite graph"));
    }
    for (key, question) in &app.questions {
        archive_id(key, &question.id, &mut ids)?;
        let goal = app
            .goals
            .get(&question.goal_id)
            .ok_or_else(|| archive_error("an orphaned question"))?;
        if app
            .concepts
            .get(&question.concept_id)
            .is_none_or(|c| c.goal_id != question.goal_id)
            || question.versions.is_empty()
            || question.versions[0].version != 1
            || question
                .versions
                .windows(2)
                .any(|v| v[0].version.checked_add(1) != Some(v[1].version))
        {
            return Err(archive_error("broken question versions or idea references"));
        }
        archive_card(&question.card)?;
        let source = goal.transcript.as_deref().unwrap_or(&goal.intent);
        for content in &question.versions {
            archive_content(content, source)?;
        }
        match (&question.draft, question.draft_for_version) {
            (None, None) => {}
            (Some(draft), Some(version))
                if version == question.content().version
                    && version.checked_add(1) == Some(draft.version) =>
            {
                archive_content(draft, source)?
            }
            _ => return Err(archive_error("a draft without its exact source version")),
        }
    }
    let mut occurrences = BTreeSet::new();
    let mut event_versions: BTreeMap<&str, u64> = BTreeMap::new();
    for event in &app.events {
        archive_id(&event.id, &event.id, &mut ids)?;
        let question = app
            .questions
            .get(&event.question_id)
            .ok_or_else(|| archive_error("an orphaned attempt"))?;
        if event.concept_id != question.concept_id
            || !occurrences.insert(&event.occurrence_id)
            || !question.versions.iter().any(|c| c == &event.presentation)
            || event.result.event_id != event.id
            || event.result.corrected
            || event.result.due_ms != event.after_card.due_ms
            || event.schedule_version == 0
            || event.schedule_version > question.schedule_version
            || event_versions
                .get(event.question_id.as_str())
                .is_some_and(|v| *v >= event.schedule_version)
        {
            return Err(archive_error("incompatible or duplicated attempt history"));
        }
        event_versions.insert(&event.question_id, event.schedule_version);
        archive_card(&event.before_card)?;
        archive_card(&event.after_card)?;
        let policy = match event.result.authority.as_str() {
            "exact" | "reveal" => "exact-v1",
            "jev" if event.presentation.kind == "explain" => learning::SEMANTIC_POLICY,
            "jev" => learning::SHORT_POLICY,
            "learner" => learning::LEARNER_POLICY,
            _ => return Err(archive_error("an unknown grading authority")),
        };
        if event.algorithm != learning::event_algorithm(policy) {
            return Err(archive_error("an incompatible event algorithm"));
        }
        let valid_outcome = match event.result.outcome.as_str() {
            "correct" => {
                !event.assisted && event.result.rating == 3 && event.result.authority != "reveal"
            }
            "wrong" => {
                !event.assisted && event.result.rating == 1 && event.result.authority != "reveal"
            }
            "revealed" => {
                event.assisted && event.result.rating == 1 && event.result.authority == "reveal"
            }
            "warm_correct" | "warm_wrong" => {
                event.assisted && event.result.rating == 0 && event.result.authority != "reveal"
            }
            _ => false,
        };
        if !valid_outcome || event.answer.len() > 4000 {
            return Err(archive_error(
                "a grade inconsistent with assistance or authority",
            ));
        }
        if event.result.authority == "exact" {
            let grade = learning::exact_grade(
                &event.presentation.kind,
                &event.presentation.answer,
                &event.presentation.variants,
                &event.answer,
            );
            let expected =
                event.result.outcome == "correct" || event.result.outcome == "warm_correct";
            if grade != Some(expected) {
                return Err(archive_error(
                    "an exact grade without an authored answer match",
                ));
            }
        }
        let expected = if event.result.rating == 0 {
            event.before_card.clone()
        } else {
            learning::schedule(
                &event.before_card,
                event.result.rating,
                event.result.reviewed_ms,
            )
            .map_err(|_| archive_error("an impossible scheduler transition"))?
        };
        if !archive_cards_agree(&expected, &event.after_card) {
            return Err(archive_error(
                "an attempt whose recorded schedule does not agree",
            ));
        }
    }
    let mut corrected = BTreeSet::new();
    for correction in &app.overrides {
        archive_id(&correction.id, &correction.id, &mut ids)?;
        let event = app
            .events
            .iter()
            .find(|e| e.id == correction.event_id)
            .ok_or_else(|| archive_error("a correction without its original attempt"))?;
        if !corrected.insert(&correction.event_id)
            || event.assisted
            || !matches!(event.result.authority.as_str(), "exact" | "jev")
            || !matches!(event.result.outcome.as_str(), "correct" | "wrong")
            || correction.correct == (event.result.outcome == "correct")
            || !archive_cards_agree(&correction.before_card, &event.after_card)
        {
            return Err(archive_error(
                "an ineligible, repeated, or inconsistent grade correction",
            ));
        }
        archive_card(&correction.before_card)?;
        archive_card(&correction.after_card)?;
        let expected = learning::schedule(
            &event.before_card,
            if correction.correct { 3 } else { 1 },
            event.result.reviewed_ms,
        )
        .map_err(|_| archive_error("an impossible correction transition"))?;
        if !archive_cards_agree(&correction.after_card, &expected) {
            return Err(archive_error(
                "a correction that did not replay the original attempt",
            ));
        }
    }
    let mut reset_causes = BTreeSet::new();
    for reset in &app.resets {
        archive_id(&reset.id, &reset.id, &mut ids)?;
        let question = app
            .questions
            .get(&reset.question_id)
            .ok_or_else(|| archive_error("a reset without its question"))?;
        let goal = &app.goals[&question.goal_id];
        if reset.concept_id != question.concept_id
            || reset.goal_id != question.goal_id
            || reset.source_revision == 0
            || reset.source_revision > goal.revision
            || !question
                .versions
                .iter()
                .any(|c| c.version == reset.content_version)
            || reset.schedule_version_before.checked_add(1) != Some(reset.schedule_version_after)
            || reset.schedule_version_after > question.schedule_version
            || !reset_causes.insert(&reset.feedback_id)
            || !app.feedback.iter().any(|feedback| {
                feedback.id == reset.feedback_id
                    && feedback.kind == "dispute"
                    && feedback.goal_id == reset.goal_id
                    && feedback.question_id.as_deref() == Some(&reset.question_id)
                    && feedback.concept_id.as_deref() == Some(&reset.concept_id)
                    && feedback.created_ms == reset.created_ms
            })
            || reset.after_card != learning::new_card(reset.created_ms)
        {
            return Err(archive_error(
                "a reset without its exact version, cause, or new-card transition",
            ));
        }
        archive_card(&reset.before_card)?;
        archive_card(&reset.after_card)?;
    }
    for (key, job) in &app.jobs {
        archive_id(key, &job.id, &mut ids)?;
        let goal = app
            .goals
            .get(&job.goal_id)
            .ok_or_else(|| archive_error("orphaned preparation work"))?;
        if !(1..=3).contains(&job.attempt)
            || job.source_revision == 0
            || job.source_revision > goal.revision
            || !matches!(job.kind.as_str(), "create" | "refine" | "fix")
            || !matches!(
                job.status.as_str(),
                "queued"
                    | "sent"
                    | "candidates"
                    | "critic-sent"
                    | "ready"
                    | "partial"
                    | "failed"
                    | "unknown"
                    | "paused"
                    | "superseded"
            )
            || job.feedback_ids.iter().any(|id| {
                !app.feedback
                    .iter()
                    .any(|f| &f.id == id && f.goal_id == job.goal_id)
            })
            || job
                .raw_response
                .as_ref()
                .is_some_and(|v| v.len() > generation::RESPONSE_LIMIT)
            || job
                .critic_json
                .as_ref()
                .is_some_and(|v| v.len() > generation::RESPONSE_LIMIT)
            || job
                .critic_cache_json
                .as_ref()
                .is_some_and(|v| v.len() > generation::RESPONSE_LIMIT)
            || job
                .critic_model
                .as_ref()
                .is_some_and(|v| !generation::text_valid(v, 1000))
        {
            return Err(archive_error(
                "invalid preparation state or feedback ownership",
            ));
        }
        match (&job.question_id, job.question_version) {
            (None, None) if job.kind != "fix" => {}
            (Some(id), Some(version))
                if job.kind == "fix"
                    && app.questions.get(id).is_some_and(|q| {
                        q.goal_id == job.goal_id && q.versions.iter().any(|c| c.version == version)
                    }) => {}
            _ => return Err(archive_error("a fix without its retained question version")),
        }
        if matches!(job.status.as_str(), "sent" | "critic-sent") {
            archive_lease(app, &job.id, job.lease.as_deref(), job.started_ms)?;
        }
        if let Some(candidate) = &job.candidate_json {
            if candidate.len() > generation::RESPONSE_LIMIT {
                return Err(archive_error("oversized saved candidates"));
            }
            let batch: Batch = serde_json::from_str(candidate)
                .map_err(|_| archive_error("unreadable saved candidates"))?;
            let source = goal.transcript.as_deref().unwrap_or(&goal.intent);
            // Historical candidate edges were judged against their own saved
            // graph revision. Validate their references and internal structure
            // without combining them with later corrected prerequisite edges;
            // live publication performs that combined-graph check again.
            let known = goal_graph(app, &job.goal_id)
                .into_keys()
                .map(|key| (key, Vec::new()))
                .collect();
            let (_, rejected) =
                generation::validate_batch_with_existing(batch.clone(), source, &known)
                    .map_err(|_| archive_error("invalid saved candidates"))?;
            if !rejected.is_empty() {
                return Err(archive_error("saved candidates that bypassed validation"));
            }
            archive_critic_history(app, job, &batch)?;
        } else if matches!(job.status.as_str(), "candidates" | "critic-sent") {
            return Err(archive_error("a content check without saved candidates"));
        } else if job.critic_cache_json.is_some()
            || job.critic_json.is_some()
            || job.critic_parent_id.is_some()
            || job.critic_model.is_some()
            || !job.critic_inherited_keys.is_empty()
        {
            return Err(archive_error("critic history without saved candidates"));
        }
    }
    for (key, assessment) in &app.assessments {
        archive_id(key, &assessment.id, &mut ids)?;
        let question = app
            .questions
            .get(&assessment.question_id)
            .ok_or_else(|| archive_error("an orphaned answer check"))?;
        let content = question
            .versions
            .iter()
            .find(|c| c.version == assessment.content_version)
            .ok_or_else(|| archive_error("an answer check without its presented version"))?;
        let policy = if content.kind == "explain" {
            learning::SEMANTIC_POLICY
        } else {
            learning::SHORT_POLICY
        };
        if assessment.policy != policy
            || assessment.schedule_version > question.schedule_version
            || !generation::text_valid(&assessment.answer, 4000)
            || !matches!(
                assessment.status.as_str(),
                "queued"
                    | "sent"
                    | "complete"
                    | "unsure"
                    | "failed"
                    | "unknown"
                    | "paused"
                    | "superseded"
            )
            || assessment
                .raw_response
                .as_ref()
                .is_some_and(|v| v.len() > generation::RESPONSE_LIMIT)
        {
            return Err(archive_error("invalid answer checking state"));
        }
        if matches!(assessment.status.as_str(), "queued" | "sent")
            && app.occurrence.as_ref().is_none_or(|o| {
                o.id != assessment.occurrence_id
                    || o.phase != "checking"
                    || o.assessment_id.as_deref() != Some(key)
            })
        {
            return Err(archive_error(
                "an active check without its current occurrence",
            ));
        }
        if assessment.status == "sent" {
            archive_lease(
                app,
                &assessment.id,
                assessment.lease.as_deref(),
                assessment.started_ms,
            )?;
        }
    }
    for spend in &app.spend {
        archive_id(&spend.id, &spend.id, &mut ids)?;
        if (!app.jobs.contains_key(&spend.work_id) && !app.assessments.contains_key(&spend.work_id))
            || spend.reserved_micros != RESERVATION
            || spend.model.is_empty()
            || spend.model.len() > 256
            || spend
                .response_hash
                .as_ref()
                .is_some_and(|hash| !archive_hash(hash))
            || spend.response_hash.is_some() != spend.response_model.is_some()
            || spend.response_hash.is_some() != spend.raw_response.is_some()
            || spend.response_hash.is_some() != spend.response_bytes.is_some()
            || spend.response_hash.is_some() != spend.response_outcome.is_some()
            || spend
                .response_outcome
                .as_ref()
                .is_some_and(|status| !matches!(status.as_str(), "received" | "rejected"))
            || spend.raw_response.as_ref().is_some_and(|raw| {
                let length = spend.response_bytes.unwrap_or(0);
                raw.len() > generation::RESPONSE_LIMIT
                    || raw.len() > length
                    || (length <= generation::RESPONSE_LIMIT
                        && (length != raw.len()
                            || spend.response_hash.as_deref()
                                != Some(hex::encode(Sha256::digest(raw.as_bytes())).as_str())))
                    || (length > generation::RESPONSE_LIMIT
                        && generation::RESPONSE_LIMIT - raw.len() > 3)
            })
            || spend
                .response_model
                .as_ref()
                .is_some_and(|model| !generation::text_valid(model, 1000))
            || !matches!(
                (spend.status.as_str(), spend.cost_micros),
                ("known", Some(_)) | ("unknown", None)
            )
        {
            return Err(archive_error("orphaned or inconsistent paid usage"));
        }
    }
    for feedback in &app.feedback {
        archive_id(&feedback.id, &feedback.id, &mut ids)?;
        if !app.goals.contains_key(&feedback.goal_id)
            || !generation::text_valid(&feedback.text, 4000)
            || feedback.concept_id.as_ref().is_some_and(|id| {
                app.concepts
                    .get(id)
                    .is_none_or(|c| c.goal_id != feedback.goal_id)
            })
            || feedback.question_id.as_ref().is_some_and(|id| {
                app.questions.get(id).is_none_or(|q| {
                    q.goal_id != feedback.goal_id
                        || feedback
                            .concept_id
                            .as_ref()
                            .is_some_and(|c| c != &q.concept_id)
                })
            })
        {
            return Err(archive_error("broken feedback references"));
        }
    }
    for (operation, receipt) in &app.operations {
        if !(16..=128).contains(&operation.len())
            || !operation
                .chars()
                .all(|c| c.is_ascii_alphanumeric() || matches!(c, '-' | '_'))
            || !archive_hash(&receipt.payload_hash)
            || !receipt.result.redirect.starts_with('/')
            || receipt.result.redirect.starts_with("//")
            || receipt.result.redirect.chars().any(char::is_control)
            || receipt.result.entity_id.as_ref().is_some_and(|id| {
                !app.goals.contains_key(id)
                    && !app.concepts.contains_key(id)
                    && !app.questions.contains_key(id)
                    && !app.jobs.contains_key(id)
            })
        {
            return Err(archive_error("an invalid exact-operation receipt"));
        }
    }
    if let Some(occurrence) = &app.occurrence {
        archive_id(&occurrence.id, &occurrence.id, &mut ids)?;
        let question = app
            .questions
            .get(&occurrence.question_id)
            .ok_or_else(|| archive_error("a broken current question"))?;
        if occurrence.concept_id != question.concept_id
            || !question
                .versions
                .iter()
                .any(|c| c == &occurrence.presentation)
            || !matches!(
                occurrence.phase.as_str(),
                "intro" | "question" | "checking" | "self-check" | "result"
            )
            || occurrence
                .answer
                .as_ref()
                .is_some_and(|answer| answer.len() > 4000)
        {
            return Err(archive_error("an invalid current occurrence"));
        }
        archive_card(&occurrence.before_card)?;
        if occurrence.phase == "result" {
            let result = occurrence
                .result
                .as_ref()
                .ok_or_else(|| archive_error("held feedback without its result"))?;
            let event = app
                .events
                .iter()
                .find(|e| e.id == result.event_id && e.occurrence_id == occurrence.id)
                .ok_or_else(|| archive_error("held feedback without its immutable attempt"))?;
            if event.question_id != occurrence.question_id
                || event.presentation != occurrence.presentation
                || !archive_cards_agree(&event.before_card, &occurrence.before_card)
                || occurrence.answer.as_deref().unwrap_or("") != event.answer
            {
                return Err(archive_error("held feedback inconsistent with its attempt"));
            }
            let mut expected = event.result.clone();
            if let Some(correction) = app.overrides.iter().find(|o| o.event_id == event.id) {
                expected.outcome = if correction.correct {
                    "correct"
                } else {
                    "wrong"
                }
                .into();
                expected.authority = "learner".into();
                expected.corrected = true;
                expected.rating = if correction.correct { 3 } else { 1 };
                expected.due_ms = correction.after_card.due_ms;
            }
            if result != &expected {
                return Err(archive_error(
                    "held feedback inconsistent with its correction",
                ));
            }
        } else {
            if occurrence.result.is_some()
                || question.archived
                || app.goals[&question.goal_id].archived
                || app.concepts[&question.concept_id].archived
                || question.content() != &occurrence.presentation
                || question.schedule_version != occurrence.schedule_version
                || !archive_cards_agree(&question.card, &occurrence.before_card)
            {
                return Err(archive_error("a stale unanswered occurrence"));
            }
            if matches!(occurrence.phase.as_str(), "checking" | "self-check") {
                let assessment = occurrence
                    .assessment_id
                    .as_ref()
                    .and_then(|id| app.assessments.get(id))
                    .ok_or_else(|| archive_error("checking without its saved assessment"))?;
                let status_valid = if occurrence.phase == "checking" {
                    matches!(assessment.status.as_str(), "queued" | "sent")
                } else {
                    matches!(
                        assessment.status.as_str(),
                        "failed" | "unknown" | "paused" | "unsure"
                    )
                };
                if !status_valid
                    || assessment.occurrence_id != occurrence.id
                    || assessment.question_id != question.id
                    || assessment.content_version != occurrence.presentation.version
                    || assessment.schedule_version != occurrence.schedule_version
                    || occurrence.answer.as_deref() != Some(assessment.answer.as_str())
                {
                    return Err(archive_error("checking inconsistent with its saved answer"));
                }
            } else if occurrence.answer.is_some() || occurrence.assessment_id.is_some() {
                return Err(archive_error(
                    "an unanswered occurrence carrying a later attempt",
                ));
            }
        }
    }
    archive_schedule_chains(app)
}

fn archive_error(reason: &str) -> AppError {
    AppError::new(422, format!("The archive contains {reason}."))
}

fn archive_id(key: &str, id: &str, seen: &mut BTreeSet<String>) -> AppResult<()> {
    if key != id
        || id.is_empty()
        || id.len() > 128
        || !id
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || matches!(c, '-' | '_'))
        || !seen.insert(id.into())
    {
        return Err(archive_error("invalid or duplicated record identities"));
    }
    Ok(())
}

fn archive_hash(value: &str) -> bool {
    value.len() == 64 && value.chars().all(|c| c.is_ascii_hexdigit())
}

fn archive_critic_history(app: &App, job: &Job, batch: &Batch) -> AppResult<()> {
    let baseline = if let Some(parent_id) = &job.critic_parent_id {
        let parent = app
            .jobs
            .get(parent_id)
            .ok_or_else(|| archive_error("missing retained critic attempt"))?;
        if parent.goal_id != job.goal_id
            || parent.source_revision != job.source_revision
            || parent.kind != job.kind
            || parent.question_id != job.question_id
            || parent.question_version != job.question_version
            || parent.attempt.checked_add(1) != Some(job.attempt)
            || parent.candidate_json != job.candidate_json
            || parent.model != job.model
            || parent.raw_response != job.raw_response
            || !matches!(
                parent.status.as_str(),
                "failed" | "unknown" | "paused" | "superseded"
            )
        {
            return Err(archive_error(
                "a critic retry that changes retained candidates or ancestry",
            ));
        }
        let keys: BTreeSet<_> = job.critic_inherited_keys.iter().collect();
        if keys.len() != job.critic_inherited_keys.len() {
            return Err(archive_error("duplicate inherited critic judgments"));
        }
        let mut inherited =
            generation::read_critic_cache(batch, parent.critic_cache_json.as_deref())
                .map_err(|_| archive_error("invalid preceding critic cache"))?;
        if keys.iter().any(|key| !inherited.answers.contains_key(*key)) {
            return Err(archive_error(
                "inherited judgments absent from their retained parent attempt",
            ));
        }
        // A late parent result may add judgments after this explicit retry.
        // Only the whole batteries already inherited at retry time belong to
        // this child's immutable starting snapshot.
        inherited.answers.retain(|key, _| keys.contains(key));
        generation::validate_critic_cache(batch, &inherited)
            .map_err(|_| archive_error("incomplete inherited critic batteries"))?;
        inherited
    } else {
        if !job.critic_inherited_keys.is_empty() {
            return Err(archive_error(
                "inherited critic judgments without a parent attempt",
            ));
        }
        generation::read_critic_cache(batch, None)
            .map_err(|_| archive_error("invalid critic candidate envelope"))?
    };
    let expected = if let Some(raw) = &job.critic_json {
        // Skipped/authored receipts deliberately contain no provider answers.
        // Complete usable groups in a real response retain their own model.
        let response = serde_json::from_str::<Value>(raw).ok();
        if response
            .as_ref()
            .is_some_and(|v| v.get("answers").is_some())
            && job.critic_model.is_none()
        {
            return Err(archive_error("unattributed raw critic judgments"));
        }
        let receipt = job.critic_model.as_ref().and_then(|model| {
            app.spend.iter().find(|receipt| {
                receipt.work_id == job.id
                    && receipt.raw_response.as_deref() == Some(raw.as_str())
                    && receipt.response_model.as_deref() == Some(model.as_str())
            })
        });
        if job.critic_model.is_some() && receipt.is_none() {
            return Err(archive_error(
                "critic judgments without their original paid response receipt",
            ));
        }
        if receipt.is_some_and(|r| r.response_outcome.as_deref() == Some("rejected")) {
            baseline.clone()
        } else {
            generation::merge_critic_cache(batch, &baseline, raw)
                .unwrap_or_else(|_| baseline.clone())
        }
    } else {
        if job.critic_model.is_some() {
            return Err(archive_error(
                "a critic model without its retained response",
            ));
        }
        baseline.clone()
    };
    let saved = generation::read_critic_cache(batch, job.critic_cache_json.as_deref())
        .map_err(|_| archive_error("invalid reusable critic judgments"))?;
    if saved != expected {
        return Err(archive_error(
            "cached critic judgments that differ from retained raw attempts",
        ));
    }
    Ok(())
}

fn archive_grounded(basis: &str, quotes: &[String], source: &str) -> bool {
    match basis {
        "general" => quotes.is_empty(),
        "source" => {
            !quotes.is_empty()
                && quotes.len() <= 8
                && quotes
                    .iter()
                    .all(|quote| generation::text_valid(quote, 4000) && source.contains(quote))
        }
        _ => false,
    }
}

fn archive_content(content: &Content, source: &str) -> AppResult<()> {
    let candidate = generation::GeneratedQuestion {
        prompt: content.prompt.clone(),
        kind: content.kind.clone(),
        answer: content.answer.clone(),
        variants: content.variants.clone(),
        choices: content.choices.clone(),
        explanation: content.explanation.clone(),
        basis: content.basis.clone(),
        quotes: content.quotes.clone(),
        required_ideas: content.required_ideas.clone(),
        contradictions: content.contradictions.clone(),
    };
    if content.version == 0 || content.model.len() > 256 {
        return Err(archive_error("invalid content attribution or version"));
    }
    generation::validate_question(&candidate, source)
        .map_err(|_| archive_error("invalid question content or evidence"))
}

fn archive_card(card: &learning::Card) -> AppResult<()> {
    use learning::State;
    let valid = card.stability.is_finite()
        && card.difficulty.is_finite()
        && card.lapses <= card.reps
        && match card.state {
            State::New => {
                card.stability == 0.0
                    && card.difficulty == 0.0
                    && card.reps == 0
                    && card.lapses == 0
                    && card.scheduled_days == 0
                    && card.remaining_steps == 0
                    && card.last_review_ms.is_none()
            }
            State::Learning | State::Relearning | State::Review => {
                (0.001..=36_500.0).contains(&card.stability)
                    && (1.0..=10.0).contains(&card.difficulty)
                    && card.reps > 0
                    && card.last_review_ms.is_some_and(|last| card.due_ms > last)
                    && match card.state {
                        State::Learning => {
                            card.scheduled_days == 0 && (0..=2).contains(&card.remaining_steps)
                        }
                        State::Relearning => {
                            card.scheduled_days == 0
                                && (0..=1).contains(&card.remaining_steps)
                                && card.lapses > 0
                        }
                        State::Review => {
                            (1..=36_501).contains(&card.scheduled_days) && card.remaining_steps == 0
                        }
                        State::New => false,
                    }
            }
        };
    if valid {
        Ok(())
    } else {
        Err(archive_error("invalid complete scheduler state"))
    }
}

fn archive_cards_agree(left: &learning::Card, right: &learning::Card) -> bool {
    let near = |a: f64, b: f64| {
        a.is_finite() && b.is_finite() && (a - b).abs() <= 1e-12 * a.abs().max(b.abs()).max(1.0)
    };
    left.due_ms == right.due_ms
        && near(left.stability, right.stability)
        && near(left.difficulty, right.difficulty)
        && left.scheduled_days == right.scheduled_days
        && left.reps == right.reps
        && left.lapses == right.lapses
        && left.state == right.state
        && left.last_review_ms == right.last_review_ms
        && left.remaining_steps == right.remaining_steps
}

fn archive_lease(
    app: &App,
    work_id: &str,
    lease: Option<&str>,
    started: Option<i64>,
) -> AppResult<()> {
    let lease = lease.ok_or_else(|| archive_error("a transmitted request without its lease"))?;
    if started.is_none()
        || app
            .spend
            .iter()
            .filter(|s| s.id == lease && s.work_id == work_id)
            .count()
            != 1
    {
        return Err(archive_error(
            "a transmitted request without its reserved usage",
        ));
    }
    Ok(())
}

/// Versions order all scheduler writes even when wall clocks coincide or move
/// backward. A correction occupies the version immediately after its original
/// event; a reset is explicitly positioned by its before/after versions.
fn archive_schedule_chains(app: &App) -> AppResult<()> {
    struct Step<'a> {
        before_version: u64,
        after_version: u64,
        before: &'a learning::Card,
        after: &'a learning::Card,
    }
    for question in app.questions.values() {
        let mut steps = Vec::new();
        for event in app
            .events
            .iter()
            .filter(|event| event.question_id == question.id)
        {
            steps.push(Step {
                before_version: event.schedule_version - 1,
                after_version: event.schedule_version,
                before: &event.before_card,
                after: &event.after_card,
            });
            for correction in app
                .overrides
                .iter()
                .filter(|correction| correction.event_id == event.id)
            {
                steps.push(Step {
                    before_version: event.schedule_version,
                    after_version: event
                        .schedule_version
                        .checked_add(1)
                        .ok_or_else(|| archive_error("an overflowing correction version"))?,
                    before: &correction.before_card,
                    after: &correction.after_card,
                });
            }
        }
        for reset in app
            .resets
            .iter()
            .filter(|reset| reset.question_id == question.id)
        {
            steps.push(Step {
                before_version: reset.schedule_version_before,
                after_version: reset.schedule_version_after,
                before: &reset.before_card,
                after: &reset.after_card,
            });
        }
        steps.sort_by_key(|step| step.before_version);
        let mut version = 0_u64;
        let mut previous: Option<&learning::Card> = None;
        for step in steps {
            if step.before_version != version
                || version.checked_add(1) != Some(step.after_version)
                || previous.is_some_and(|card| !archive_cards_agree(card, step.before))
                || (previous.is_none() && !step.before.is_new())
            {
                return Err(archive_error(
                    "a missing, competing, or disconnected schedule transition",
                ));
            }
            version = step.after_version;
            previous = Some(step.after);
        }
        if question.schedule_version != version
            || previous.is_some_and(|card| !archive_cards_agree(card, &question.card))
            || (previous.is_none() && !question.card.is_new())
        {
            return Err(archive_error(
                "a current schedule that differs from its complete history",
            ));
        }
    }
    Ok(())
}

/// Recovery never clones scheduler/job ownership or quietly sends paid work again.
pub fn pause_for_restore(app: &mut App, now: i64) {
    app.restored_paused = true;
    for j in app.jobs.values_mut() {
        if matches!(j.status.as_str(), "sent" | "critic-sent") {
            j.status = "unknown".into();
            j.error = Some(
                "Restored request has an uncertain paid outcome. Its usage is retained.".into(),
            );
        } else if matches!(j.status.as_str(), "queued" | "candidates") {
            j.status = "paused".into();
            j.error = Some("Preparation is paused in this restored learning space.".into());
        }
    }
    let ids: Vec<_> = app
        .assessments
        .values()
        .filter(|a| matches!(a.status.as_str(), "sent" | "queued"))
        .map(|a| (a.id.clone(), a.status == "sent"))
        .collect();
    for (aid, sent) in ids {
        assessment_failed(
            app,
            &aid,
            "The check is paused after restore. Its saved answer can be checked by you.",
            sent,
            now,
        );
        if !sent {
            app.assessments.get_mut(&aid).unwrap().status = "paused".into();
        }
    }
    app.backup.error = Some(format!(
        "Restored into an isolated space at {now}; external work remains paused."
    ));
    app.revision += 1;
}
