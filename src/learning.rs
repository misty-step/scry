//! Pure learning policy. No persistence, HTTP, model calls, or runtime clock.
//!
//! The scheduler below ports the pinned go-fsrs/v4.0.0 basic scheduler, including
//! its default FSRS v6 weights and UTC-calendar scheduling convention. Full-card
//! comparisons against that exact Go release live in `tests/fsrs_golden.rs`.

// Scheduler portions derived from github.com/open-spaced-repetition/go-fsrs/v4
// v4.0.0, under the following license:
//
// MIT License
// Copyright (c) 2022 open-spaced-repetition
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

pub const SCHEDULER: &str =
    "go-fsrs/v4.0.0;defaults-v1;retention=.9;fuzz=false;steps=1m,10m;relearn=10m";
pub const ALGORITHM: &str =
    "go-fsrs/v4.0.0;defaults-v1;retention=.9;fuzz=false;steps=1m,10m;relearn=10m;grading=exact-v1";
pub const SHORT_POLICY: &str = "short-v1";
pub const SEMANTIC_POLICY: &str = "semantic-v1";
pub const LEARNER_POLICY: &str = "learner-v1";
pub const CRITIC_POLICY: &str = "critic-v1";
pub const CONCEPT_STATE_POLICY: &str = "concept-state-v1";
const DAY_MS: i64 = 86_400_000;
const MAX_INTERVAL: f64 = 36_500.0;
const WEIGHTS: [f64; 21] = [
    0.212, 1.2931, 2.3065, 8.2956, 6.4133, 0.8334, 3.0194, 0.001, 1.8722, 0.1666, 0.796, 1.4835,
    0.0614, 0.2629, 1.6483, 0.6014, 1.8729, 0.5425, 0.0912, 0.0658, 0.1542,
];

pub fn event_algorithm(grading: &str) -> String {
    format!("{SCHEDULER};grading={grading}")
}

#[derive(Clone, Copy, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum State {
    #[default]
    New,
    Learning,
    Review,
    Relearning,
}

/// Every persisted piece of the pinned scheduler's card state.
#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct Card {
    pub due_ms: i64,
    pub stability: f64,
    pub difficulty: f64,
    pub scheduled_days: u64,
    pub reps: u64,
    pub lapses: u64,
    pub state: State,
    pub last_review_ms: Option<i64>,
    pub remaining_steps: i32,
}

impl Card {
    pub fn is_new(&self) -> bool {
        self.state == State::New
    }

    pub fn is_learning(&self) -> bool {
        matches!(self.state, State::Learning | State::Relearning)
    }
}

pub fn new_card(now_ms: i64) -> Card {
    Card {
        due_ms: now_ms,
        stability: 0.0,
        difficulty: 0.0,
        scheduled_days: 0,
        reps: 0,
        lapses: 0,
        state: State::New,
        last_review_ms: None,
        remaining_steps: 0,
    }
}

fn constrain_stability(value: f64) -> f64 {
    value.clamp(0.001, MAX_INTERVAL)
}

fn initial_difficulty(rating: u8) -> f64 {
    WEIGHTS[4] - (WEIGHTS[5] * f64::from(rating - 1)).exp() + 1.0
}

fn next_difficulty(difficulty: f64, rating: u8) -> f64 {
    let delta = -WEIGHTS[6] * (f64::from(rating) - 3.0);
    let next = difficulty + (10.0 - difficulty) * delta / 9.0;
    (WEIGHTS[7] * initial_difficulty(4) + (1.0 - WEIGHTS[7]) * next).clamp(1.0, 10.0)
}

fn short_term_stability(stability: f64, rating: u8) -> f64 {
    let stability = constrain_stability(stability);
    let mut increase = (WEIGHTS[17] * (f64::from(rating) - 3.0 + WEIGHTS[18])).exp()
        * stability.powf(-WEIGHTS[19]);
    if rating >= 2 && increase < 1.0 {
        increase = 1.0;
    }
    constrain_stability(stability * increase)
}

fn forgetting_curve(elapsed_days: f64, stability: f64) -> f64 {
    let decay = -WEIGHTS[20];
    let factor = 0.9_f64.powf(1.0 / decay) - 1.0;
    (1.0 + factor * elapsed_days / constrain_stability(stability)).powf(decay)
}

fn recall_stability(difficulty: f64, stability: f64, recall: f64, rating: u8) -> f64 {
    let hard_penalty = if rating == 2 { WEIGHTS[15] } else { 1.0 };
    let easy_bonus = if rating == 4 { WEIGHTS[16] } else { 1.0 };
    constrain_stability(
        stability
            * (1.0
                + WEIGHTS[8].exp()
                    * (11.0 - difficulty)
                    * stability.powf(-WEIGHTS[9])
                    * (((1.0 - recall) * WEIGHTS[10]).exp() - 1.0)
                    * hard_penalty
                    * easy_bonus),
    )
}

fn forget_stability(difficulty: f64, stability: f64, recall: f64) -> f64 {
    let next = WEIGHTS[11]
        * difficulty.powf(-WEIGHTS[12])
        * ((stability + 1.0).powf(WEIGHTS[13]) - 1.0)
        * ((1.0 - recall) * WEIGHTS[14]).exp();
    let ceiling = stability / (WEIGHTS[17] * WEIGHTS[18]).exp();
    constrain_stability(next.min(ceiling))
}

fn interval(stability: f64) -> f64 {
    let decay = -WEIGHTS[20];
    let factor = 0.9_f64.powf(1.0 / decay) - 1.0;
    let raw = constrain_stability(stability) / factor * (0.9_f64.powf(1.0 / decay) - 1.0);
    raw.round().clamp(1.0, MAX_INTERVAL)
}

fn calendar_elapsed_days(card: &Card, now_ms: i64) -> f64 {
    if card.state == State::New {
        return 0.0;
    }
    card.last_review_ms.map_or(0.0, |last| {
        (now_ms.div_euclid(DAY_MS) - last.div_euclid(DAY_MS)).max(0) as f64
    })
}

fn apply_step(card: &mut Card, now_ms: i64, minutes: i64, state: State) -> Result<(), String> {
    card.due_ms = now_ms
        .checked_add(minutes * 60_000)
        .ok_or("scheduled date is outside the timestamp range")?;
    card.scheduled_days = 0;
    card.state = state;
    Ok(())
}

fn graduate(card: &mut Card, now_ms: i64, days: f64) -> Result<(), String> {
    card.scheduled_days = days as u64;
    card.due_ms = now_ms
        .checked_add((days.min(MAX_INTERVAL) * DAY_MS as f64) as i64)
        .ok_or("scheduled date is outside the timestamp range")?;
    card.state = State::Review;
    card.remaining_steps = 0;
    Ok(())
}

/// One deterministic FSRS transition. Only Again (1) and Good (3) are earned.
pub fn schedule(card: &Card, rating: u8, now_ms: i64) -> Result<Card, String> {
    if !matches!(rating, 1 | 3) {
        return Err(format!("unsupported learning rating {rating}"));
    }
    if card.state != State::New {
        if !card.stability.is_finite() || card.stability < 0.001 {
            return Err("invalid card stability".into());
        }
        if !card.difficulty.is_finite() || card.difficulty < 1.0 {
            return Err("invalid card difficulty".into());
        }
    }
    if card.last_review_ms.is_some_and(|last| last > now_ms) {
        return Err("last review is after the current time".into());
    }
    let elapsed = calendar_elapsed_days(card, now_ms);
    let mut next = card.clone();
    next.last_review_ms = Some(now_ms);
    next.reps = next.reps.checked_add(1).ok_or("review counter overflow")?;
    if card.state == State::New {
        next.difficulty = initial_difficulty(rating).clamp(1.0, 10.0);
        next.stability = WEIGHTS[usize::from(rating - 1)].clamp(0.1, MAX_INTERVAL);
        if rating == 1 {
            next.remaining_steps = 2;
            apply_step(&mut next, now_ms, 1, State::Learning)?;
        } else {
            next.remaining_steps = 1;
            apply_step(&mut next, now_ms, 10, State::Learning)?;
        }
    } else {
        next.difficulty = next_difficulty(card.difficulty, rating);
        next.stability = if elapsed == 0.0 {
            short_term_stability(card.stability, rating)
        } else {
            let recall = forgetting_curve(elapsed, card.stability);
            if rating == 1 {
                forget_stability(card.difficulty, card.stability, recall)
            } else {
                recall_stability(card.difficulty, card.stability, recall, rating)
            }
        };
        if card.state == State::Review {
            if rating == 1 {
                next.remaining_steps = 1;
                next.lapses = next.lapses.checked_add(1).ok_or("lapse counter overflow")?;
                apply_step(&mut next, now_ms, 10, State::Relearning)?;
            } else {
                // go-fsrs computes all rating intervals even on a Good pass.
                let hard_stability = if elapsed == 0.0 {
                    short_term_stability(card.stability, 2)
                } else {
                    recall_stability(
                        card.difficulty,
                        card.stability,
                        forgetting_curve(elapsed, card.stability),
                        2,
                    )
                };
                let good = interval(next.stability);
                let hard = interval(hard_stability).min(good);
                graduate(&mut next, now_ms, good.max(hard + 1.0))?;
            }
        } else {
            let (step_count, first_minutes) = if card.state == State::Relearning {
                (1, 10)
            } else {
                (2, 1)
            };
            let next_index = i64::from(step_count) - i64::from(card.remaining_steps) + 1;
            if rating == 1 && card.remaining_steps > 0 {
                next.remaining_steps = step_count;
                apply_step(&mut next, now_ms, first_minutes, card.state)?;
            } else if rating == 3 && (0..i64::from(step_count)).contains(&next_index) {
                let minutes = if step_count == 2 && next_index == 0 {
                    1
                } else {
                    10
                };
                next.remaining_steps = card.remaining_steps - 1;
                apply_step(&mut next, now_ms, minutes, card.state)?;
            } else {
                let days = interval(next.stability);
                graduate(&mut next, now_ms, days)?;
            }
        }
    }
    if !next.stability.is_finite()
        || next.stability < 0.001
        || !next.difficulty.is_finite()
        || next.difficulty < 1.0
    {
        return Err("computed card state is invalid".into());
    }
    Ok(next)
}

/// A recall estimate, or -1 when no reviewed state supports an estimate.
pub fn retrievability(card: &Card, now_ms: i64) -> f64 {
    let Some(last) = card.last_review_ms else {
        return -1.0;
    };
    if card.state == State::New || !card.stability.is_finite() || card.stability <= 0.0 {
        return -1.0;
    }
    // The pinned Go estimate uses elapsed full days, unlike scheduling's
    // UTC-calendar date difference. Preserve both conventions separately.
    let elapsed = (((now_ms as i128 - last as i128) as f64) / DAY_MS as f64)
        .floor()
        .max(0.0);
    let value = forgetting_curve(elapsed, card.stability);
    if value.is_finite() && (0.0..=1.0).contains(&value) {
        value
    } else {
        -1.0
    }
}

pub fn new_concept_budget(pace: &str) -> usize {
    match pace {
        "light" => 3,
        "intense" => 12,
        _ => 6,
    }
}

pub fn level_rank(level: &str) -> u8 {
    match level {
        "recognize" => 0,
        "explain" => 2,
        "apply" => 3,
        _ => 1,
    }
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct Attempt {
    pub at_ms: i64,
    pub rating: u8,
    pub assisted: bool,
    pub outcome: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct QuestionEvidence {
    pub card: Card,
    pub level: String,
    pub attempts: Vec<Attempt>,
}

/// A display estimate based on observed attempts. It never updates a schedule.
#[derive(Clone, Debug, Deserialize, PartialEq, Serialize)]
pub struct ConceptState {
    pub status: String,
    /// -1 means there is no reviewed card supporting an estimate.
    pub recall: f64,
    pub brightness: u8,
    pub unaided: u64,
    pub helped: u64,
    pub missed: u64,
    pub last_at_ms: Option<i64>,
    pub next_due_ms: Option<i64>,
    pub tally: Vec<String>,
}

pub fn compute_concept_state(questions: &[QuestionEvidence], now_ms: i64) -> ConceptState {
    let mut state = ConceptState {
        status: "new".into(),
        recall: -1.0,
        brightness: 0,
        unaided: 0,
        helped: 0,
        missed: 0,
        last_at_ms: None,
        next_due_ms: None,
        tally: vec![],
    };
    let mut attempts: Vec<&Attempt> = vec![];
    let mut weighted = 0.0;
    let mut weights = 0.0;
    let mut max_stability: f64 = 0.0;
    let mut reviewed = false;
    for question in questions {
        attempts.extend(&question.attempts);
        let recall = retrievability(&question.card, now_ms);
        if recall >= 0.0 {
            let weight = match question.level.as_str() {
                "recognize" => 1.0,
                "explain" | "apply" => 3.0,
                _ => 2.0,
            };
            weighted += recall * weight;
            weights += weight;
            max_stability = max_stability.max(question.card.stability);
            reviewed |= question.card.state == State::Review;
            state.next_due_ms = Some(
                state
                    .next_due_ms
                    .map_or(question.card.due_ms, |due| due.min(question.card.due_ms)),
            );
        }
    }
    attempts.sort_by_key(|attempt| attempt.at_ms);
    for attempt in attempts {
        let code = if attempt.rating == 3 && !attempt.assisted {
            state.unaided += 1;
            "u"
        } else if attempt.assisted
            || attempt.outcome == "revealed"
            || attempt.outcome.starts_with("warm_")
        {
            state.helped += 1;
            "h"
        } else if attempt.rating == 1 {
            state.missed += 1;
            "m"
        } else {
            continue;
        };
        state.tally.push(code.into());
        state.last_at_ms = Some(attempt.at_ms);
    }
    if state.tally.len() > 12 {
        state.tally.drain(..state.tally.len() - 12);
    }
    if weights > 0.0 {
        state.recall = weighted / weights;
    }
    if state.tally.is_empty() {
        return state;
    }
    if state.recall >= 0.90
        && max_stability >= 21.0
        && state.tally.last().is_some_and(|code| code == "u")
    {
        state.status = "solid".into();
    } else if reviewed && state.recall >= 0.0 && state.recall < 0.80 {
        state.status = "fading".into();
    } else {
        state.status = "learning".into();
    }
    state.brightness = if state.recall >= 0.0 {
        (state.recall * 5.0).round().clamp(1.0, 5.0) as u8
    } else {
        1
    };
    state.brightness = match state.status.as_str() {
        "fading" => state.brightness.min(2),
        "solid" => state.brightness.max(4),
        _ => state.brightness.min(4),
    };
    state
}

/// `None` requires a bounded assessor or learner self-check, never similarity.
pub fn exact_grade(kind: &str, expected: &str, variants: &[String], answer: &str) -> Option<bool> {
    if kind == "choice" {
        return Some(answer == expected);
    }
    let answer = answer.trim();
    if answer == expected.trim() || variants.iter().any(|variant| answer == variant.trim()) {
        Some(true)
    } else {
        None
    }
}

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct Rubric {
    pub required: Vec<RubricIdea>,
    #[serde(default)]
    pub contradictions: Vec<RubricClaim>,
}

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct RubricIdea {
    pub text: String,
    #[serde(default)]
    pub cue: String,
}

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct RubricClaim {
    pub text: String,
    #[serde(default)]
    pub feedback: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct ShortJudgments {
    pub verdict: String,
    pub probabilities: BTreeMap<String, f64>,
    pub identity: f64,
    pub injection: f64,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
pub struct SemanticJudgments {
    pub ideas: Vec<f64>,
    pub contradictions: Vec<f64>,
    pub relation: String,
    pub relation_probabilities: BTreeMap<String, f64>,
    pub injection: f64,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub struct Decision {
    pub decision: String,
    pub applied: bool,
    pub outcome: String,
    pub rating: u8,
    pub missing_idea: Option<usize>,
    pub contradiction: Option<usize>,
}

impl Decision {
    pub fn ungraded() -> Self {
        Self {
            decision: "ungraded".into(),
            applied: false,
            outcome: "ungraded".into(),
            rating: 0,
            missing_idea: None,
            contradiction: None,
        }
    }

    fn correct() -> Self {
        Self {
            decision: "correct".into(),
            applied: true,
            outcome: "correct".into(),
            rating: 3,
            ..Self::ungraded()
        }
    }

    fn incorrect() -> Self {
        Self {
            decision: "incorrect".into(),
            applied: true,
            outcome: "wrong".into(),
            rating: 1,
            ..Self::ungraded()
        }
    }
}

fn valid_probability(value: f64) -> bool {
    (0.0..=1.0).contains(&value)
}

pub fn grade_short(judgments: &ShortJudgments) -> Decision {
    if !valid_probability(judgments.identity)
        || !valid_probability(judgments.injection)
        || judgments
            .probabilities
            .values()
            .any(|p| !valid_probability(*p))
        || judgments.injection > 0.20
    {
        return Decision::ungraded();
    }
    let Some(probability) = judgments.probabilities.get(&judgments.verdict) else {
        return Decision::ungraded();
    };
    match judgments.verdict.as_str() {
        "accept" if *probability >= 0.85 && judgments.identity <= 0.35 => Decision::correct(),
        "reject" if *probability >= 0.90 => Decision::incorrect(),
        _ => Decision::ungraded(),
    }
}

pub fn grade_semantic(judgments: &SemanticJudgments) -> Decision {
    if judgments.ideas.is_empty()
        || !valid_probability(judgments.injection)
        || judgments
            .ideas
            .iter()
            .chain(judgments.contradictions.iter())
            .chain(judgments.relation_probabilities.values())
            .any(|p| !valid_probability(*p))
    {
        return Decision::ungraded();
    }
    let Some(probability) = judgments.relation_probabilities.get(&judgments.relation) else {
        return Decision::ungraded();
    };
    let contradictions_low = judgments.contradictions.iter().all(|p| *p <= 0.20);
    if judgments.ideas.iter().all(|p| *p >= 0.80)
        && contradictions_low
        && judgments.relation == "equivalent"
        && *probability >= 0.85
        && judgments.injection <= 0.20
    {
        return Decision::correct();
    }
    // These policy classes remain shadow observations; no misses or help are
    // applied until a separately authorized policy grants that authority.
    if judgments.relation == "different"
        && *probability >= 0.85
        && let Some(index) = judgments.contradictions.iter().position(|p| *p >= 0.90)
    {
        return Decision {
            decision: "incorrect".into(),
            contradiction: Some(index),
            ..Decision::ungraded()
        };
    }
    if contradictions_low && judgments.relation == "partial" && *probability >= 0.60 {
        let mut missing = None;
        for (index, probability) in judgments.ideas.iter().enumerate() {
            if *probability <= 0.35 && missing.is_none() {
                missing = Some(index);
            } else if *probability < 0.80 {
                return Decision::ungraded();
            }
        }
        if missing.is_some() {
            return Decision {
                decision: "incomplete".into(),
                missing_idea: missing,
                ..Decision::ungraded()
            };
        }
    }
    Decision::ungraded()
}

#[derive(Clone, Copy, Debug, Default)]
pub struct CriticParams {
    pub source: bool,
    pub choice: bool,
    pub semantic: bool,
}

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct CriticDecision {
    pub decision: String,
    pub reasons: Vec<String>,
    pub teaching_score: f64,
}

pub fn critic_hard_keys(params: CriticParams) -> Vec<&'static str> {
    let mut keys = vec![
        "evidence_contradicts",
        "qualification_changed",
        "not_answerable_from_context",
        "multiple_defensible_answers",
        "no_defensible_answer",
        "prompt_leaks_answer",
        "adversarial_content",
    ];
    if params.source {
        keys.push("unsupported_by_evidence");
    }
    if params.choice {
        keys.push("mcq_options_overlap");
    }
    if params.semantic {
        keys.push("rubric_misaligned");
    }
    keys
}

pub fn judge_candidate(judgments: &BTreeMap<String, f64>, params: CriticParams) -> CriticDecision {
    const SOFT_KEY: &str = "explanation_restates_without_teaching";
    let keys = critic_hard_keys(params);
    let mut decision = CriticDecision {
        decision: "ungraded".into(),
        reasons: vec![],
        teaching_score: 0.0,
    };
    if judgments.len() != keys.len() + 1
        || keys
            .iter()
            .copied()
            .chain(std::iter::once(SOFT_KEY))
            .any(|key| judgments.get(key).is_none_or(|p| !valid_probability(*p)))
    {
        return decision;
    }
    decision.decision = "accept".into();
    decision.teaching_score = 1.0 - judgments[SOFT_KEY];
    for key in keys {
        if judgments[key] >= 0.80 {
            decision.decision = "reject".into();
            decision.reasons.push(key.into());
        }
    }
    decision
}
