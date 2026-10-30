//! Exact text practice policy. The engine owns receipts; runtime owns commits.
use crate::{
    grading::{self, Grade, Verdict},
    learning,
    library::{Kind, Version},
    model::*,
};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use std::collections::{BTreeMap, BTreeSet};

#[derive(Clone, Copy, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Status {
    Active,
    Paused,
    Dropped,
}
#[derive(Clone, Copy, Debug, Default, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Pace {
    Light,
    #[default]
    Steady,
    Intense,
}
impl Pace {
    fn parse(value: &str) -> AppResult<Self> {
        match value {
            "" | "steady" => Ok(Self::Steady),
            "light" => Ok(Self::Light),
            "intense" => Ok(Self::Intense),
            _ => Err(AppError::new(422, "Use light, steady or intense Pace.")),
        }
    }
    fn limit(self) -> usize {
        match self {
            Self::Light => 3,
            Self::Steady => 6,
            Self::Intense => 12,
        }
    }
}
#[derive(Clone, Copy, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Direction {
    Forward,
}
#[derive(Clone, Copy, Debug, Default, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum Cue {
    #[default]
    Read,
    FirstLetters,
    FirstLetter,
    PreviousLine,
    SectionHeading,
    WholeTitle,
    /// A supplied key, never an answer-bearing text cue.
    MappingKey,
}
impl Cue {
    fn assisted(self) -> bool {
        matches!(self, Self::Read | Self::FirstLetters | Self::FirstLetter)
    }
    fn fade(self) -> Self {
        match self {
            Self::Read => Self::FirstLetters,
            Self::FirstLetters => Self::FirstLetter,
            Self::FirstLetter => Self::PreviousLine,
            Self::PreviousLine => Self::SectionHeading,
            Self::SectionHeading | Self::WholeTitle => Self::WholeTitle,
            Self::MappingKey => Self::MappingKey,
        }
    }
    fn lapse(self) -> Self {
        match self {
            Self::Read | Self::FirstLetters => Self::Read,
            Self::FirstLetter => Self::FirstLetters,
            Self::PreviousLine => Self::FirstLetter,
            Self::SectionHeading => Self::PreviousLine,
            Self::WholeTitle => Self::SectionHeading,
            Self::MappingKey => Self::MappingKey,
        }
    }
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct Study {
    pub id: String,
    pub version_id: String,
    pub version_hash: String,
    pub pace: Pace,
    /// Set only by deliberate focused entry; retained Pace remains unchanged.
    #[serde(default, skip_serializing_if = "is_false")]
    pub nato_all_letters: bool,
    /// Explicit phrase/sequence lesson; does not alter retained Pace or due dates.
    #[serde(default, skip_serializing_if = "is_false")]
    pub lesson: bool,
    pub status: Status,
    pub created_ms: i64,
}
fn is_false(value: &bool) -> bool {
    !value
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct CardState {
    pub schedule: learning::Card,
    pub cue: Cue,
    pub successes: u8,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct Card {
    pub id: String,
    pub study_id: String,
    pub unit_id: String,
    pub unit_index: usize,
    pub direction: Direction,
    pub introduced_ms: i64,
    pub revision: u64,
    pub state: CardState,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct Target {
    pub card_id: String,
    pub unit_id: String,
    pub unit_index: usize,
    pub cue: Cue,
    pub revision: u64,
    pub before: CardState,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub enum Scope {
    Unit { unit_id: String, position: usize },
    Section { section_id: String },
    Whole,
}
/// Safe wire presentation: no hidden expected text or embedded Version.
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct Prompt {
    pub id: String,
    pub study_id: String,
    pub version_id: String,
    pub version_hash: String,
    pub title: String,
    pub scope: Scope,
    pub unit_ids: Vec<String>,
    pub cue_level: Cue,
    pub context: Option<String>,
    pub cue: String,
    pub assisted: bool,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct Presentation {
    pub prompt: Prompt,
    pub targets: Vec<Target>,
    pub created_ms: i64,
    pub revealed: bool,
    pub attempt_id: Option<String>,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct Effect {
    pub card_id: String,
    pub unit_id: String,
    pub unit_index: usize,
    pub cue_level: Cue,
    pub verdict: Verdict,
    pub assisted: bool,
    pub extra_practice: bool,
    pub scheduled: bool,
    pub revision_before: u64,
    pub before: CardState,
    pub after: CardState,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct Attempt {
    pub id: String,
    pub sequence: u64,
    pub prompt: Prompt,
    pub version_id: String,
    pub version_hash: String,
    pub raw_answer: String,
    pub grade: Grade,
    pub acting_token: Option<String>,
    pub assisted: bool,
    #[serde(default, skip_serializing_if = "std::ops::Not::not")]
    pub disclosure: bool,
    pub revealed: bool,
    pub created_ms: i64,
    pub algorithm: String,
    pub effects: Vec<Effect>,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq)]
pub struct Correction {
    pub id: String,
    pub sequence: u64,
    pub attempt_id: String,
    pub unit_id: String,
    pub verdict: Verdict,
    pub authority: String,
    pub acting_token: Option<String>,
    pub created_ms: i64,
    pub revision_before: u64,
    pub before: CardState,
    pub after: CardState,
}
#[derive(Clone, Debug, Default, Deserialize, Serialize, PartialEq)]
pub struct Practice {
    pub studies: BTreeMap<String, Study>,
    pub cards: BTreeMap<String, Card>,
    pub attempts: BTreeMap<String, Attempt>,
    pub corrections: BTreeMap<String, Correction>,
    pub current: Option<Presentation>,
}
impl Practice {
    pub fn is_empty(&self) -> bool {
        self.studies.is_empty()
            && self.cards.is_empty()
            && self.attempts.is_empty()
            && self.corrections.is_empty()
            && self.current.is_none()
    }
    pub fn protected(&self) -> bool {
        self.current
            .as_ref()
            .is_some_and(|p| p.attempt_id.is_none() && !p.prompt.assisted && !p.revealed)
    }
}
fn field<'a>(fields: &'a BTreeMap<String, String>, key: &str) -> &'a str {
    fields.get(key).map(String::as_str).unwrap_or("")
}
fn id(seed: &str, kind: &str) -> String {
    crate::persistence::sha256(format!("{seed}:study:{kind}").as_bytes())[..32].into()
}
fn initial(now: i64, kind: Kind) -> CardState {
    CardState {
        schedule: learning::new_card(now),
        cue: if kind == Kind::KeyedMapping {
            Cue::MappingKey
        } else {
            Cue::Read
        },
        successes: 0,
    }
}
/// Mapping Units retain their supplied key and delimiter in Library/history.
/// Only their value is an expected answer; clients never supply this projection.
pub(crate) fn expected_unit<'a>(version: &Version, unit: &'a crate::library::Unit) -> &'a str {
    if version.kind == Kind::KeyedMapping {
        crate::library::mapping_pair(&unit.text)
            .expect("validated mapping Unit")
            .1
    } else {
        &unit.text
    }
}
pub(crate) fn studyable(version: &Version) -> AppResult<()> {
    match version.kind {
        Kind::Text => Ok(()),
        Kind::KeyedMapping => crate::library::validate_mapping(version),
        Kind::OrderedList => crate::ordered::validate(version),
    }
}
fn units(version: &Version) -> Vec<&crate::library::Unit> {
    version.sections.iter().flat_map(|s| &s.units).collect()
}
fn response(entity: Option<String>, data: Value) -> MutationResult {
    MutationResult {
        redirect: "/api/v1/practice".into(),
        entity_id: entity,
        message: None,
        data: Some(data),
    }
}
const MAPPING_PROMPT_TITLE: &str = "Recall the matching value";
/// Supplied metadata may contain the answer. Preserve it in Library/history,
/// but never use it as an extra mapping cue, including restored held Prompts.
pub(crate) fn visible_prompt_title(prompt: &Prompt) -> &str {
    if prompt.cue_level == Cue::MappingKey {
        MAPPING_PROMPT_TITLE
    } else {
        &prompt.title
    }
}
fn held(app: &App) -> AppResult<Value> {
    let p = app
        .practice
        .current
        .as_ref()
        .ok_or_else(AppError::conflict)?;
    let mut prompt = p.prompt.clone();
    prompt.title = visible_prompt_title(&prompt).to_string();
    Ok(
        json!({"prompt":prompt,"revealed":p.revealed,"attempt_id":p.attempt_id,
        "correction_ids":app.practice.corrections.values().filter(|c| Some(&c.attempt_id)==p.attempt_id.as_ref()).map(|c| &c.id).collect::<Vec<_>>() }),
    )
}
/// Deterministic, answer-time explanation; it never changes exact-v2's Verdict.
#[derive(Clone, Debug, Serialize)]
pub struct MappingFeedback {
    pub kind: &'static str,
    pub key: String,
    pub expected: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub answered_key: Option<String>,
}
pub fn mapping_feedback(app: &App, attempt: &Attempt) -> Option<MappingFeedback> {
    let version = &app.library.versions[&attempt.version_id];
    if version.kind != Kind::KeyedMapping {
        return None;
    }
    let unit = units(version)[attempt.effects[0].unit_index];
    let (key, expected) = crate::library::mapping_pair(&unit.text)?;
    let answer = grading::mapping_words(&attempt.raw_answer);
    let answered_key = (attempt.grade.verdict == Verdict::Again)
        .then(|| {
            units(version)
                .into_iter()
                .filter(|u| u.id != unit.id)
                .filter_map(|u| crate::library::mapping_pair(&u.text))
                .find(|(_, value)| grading::mapping_words(value) == answer)
                .map(|(key, _)| key.to_string())
        })
        .flatten();
    let kind = if attempt.grade.verdict == Verdict::Good {
        "correct"
    } else if answered_key.is_some() {
        "wrong_key"
    } else if attempt
        .grade
        .diff
        .iter()
        .any(|d| matches!(d.change, grading::Change::Slip { .. }))
    {
        "spelling"
    } else {
        "mismatch"
    };
    Some(MappingFeedback {
        kind,
        key: key.into(),
        expected: expected.into(),
        answered_key,
    })
}
/// Resolve immutable response references only after the runtime has committed.
pub fn wire_response(app: &App, result: &MutationResult) -> Option<Value> {
    let mut data = result.data.clone()?;
    if let Some(id) = data.get("attempt_id").and_then(Value::as_str) {
        let attempt = app.practice.attempts.get(id);
        let feedback = attempt.and_then(|a| mapping_feedback(app, a));
        data["attempt"] = json!(attempt);
        if let Some(feedback) = feedback {
            data["mapping_feedback"] = json!(feedback);
        }
    }
    if let Some(id) = data.get("correction_id").and_then(Value::as_str) {
        data["correction"] = json!(app.practice.corrections.get(id));
    }
    if let Some(ids) = data.get("correction_ids").and_then(Value::as_array) {
        data["corrections"] = json!(
            ids.iter()
                .filter_map(|id| app.practice.corrections.get(id.as_str()?))
                .collect::<Vec<_>>()
        );
    }
    Some(data)
}
/// Receipts retain their original disclosure, but permission to return it is
/// determined by the current Prompt. Attempt references resolve to full text;
/// Corrections and Study metadata contain no answer-bearing content.
pub(crate) fn check_replay(app: &App, result: &MutationResult) -> AppResult<()> {
    if !app.practice.protected() {
        return Ok(());
    }
    let Some(data) = &result.data else {
        return Ok(());
    };
    if data.get("expected").is_some() || data.get("attempt_id").is_some_and(Value::is_string) {
        return Err(AppError::conflict());
    }
    if let Some(value) = data.get("prompt").filter(|v| !v.is_null()) {
        let prompt: Prompt =
            serde_json::from_value(value.clone()).map_err(|_| AppError::conflict())?;
        // Do not rewrite a historical receipt merely to make it replayable.
        // A pre-fix supplied mapping title is unsafe while any recall is protected.
        if prompt.cue_level == Cue::MappingKey && prompt.title != MAPPING_PROMPT_TITLE {
            return Err(AppError::conflict());
        }
        let current = &app.practice.current.as_ref().unwrap().prompt;
        if prompt != *current && (prompt.cue_level.assisted() || prompt.context.is_some()) {
            return Err(AppError::conflict());
        }
    }
    Ok(())
}
fn effect(target: &Target, verdict: Verdict, revealed: bool, now: i64) -> AppResult<Effect> {
    let assisted = target.cue.assisted() || revealed;
    let extra = !assisted
        && target
            .before
            .schedule
            .last_review_ms
            .is_some_and(|t| t.div_euclid(DAY_MS) == now.div_euclid(DAY_MS));
    let scheduled = !assisted && !extra;
    let mut after = target.before.clone();
    if scheduled {
        after.schedule = learning::schedule(
            &after.schedule,
            if verdict == Verdict::Good { 3 } else { 1 },
            now,
        )
        .map_err(|m| AppError::new(422, m))?;
    }
    if verdict == Verdict::Good && !revealed && target.cue == after.cue {
        after.successes += 1;
        if after.successes == 2 {
            after.cue = after.cue.fade();
            after.successes = 0;
        }
    } else {
        after.successes = 0;
        if verdict == Verdict::Again && !assisted {
            after.cue = after.cue.lapse();
        }
    }
    Ok(Effect {
        card_id: target.card_id.clone(),
        unit_id: target.unit_id.clone(),
        unit_index: target.unit_index,
        cue_level: target.cue,
        verdict,
        assisted,
        extra_practice: extra,
        scheduled,
        revision_before: target.revision,
        before: target.before.clone(),
        after,
    })
}
fn disclosure_effect(target: &Target) -> Effect {
    Effect {
        card_id: target.card_id.clone(),
        unit_id: target.unit_id.clone(),
        unit_index: target.unit_index,
        cue_level: target.cue,
        verdict: Verdict::Again,
        assisted: true,
        extra_practice: false,
        scheduled: false,
        revision_before: target.revision,
        before: target.before.clone(),
        after: target.before.clone(),
    }
}
fn target(card: &Card, cue: Cue) -> Target {
    Target {
        card_id: card.id.clone(),
        unit_id: card.unit_id.clone(),
        unit_index: card.unit_index,
        cue,
        revision: card.revision,
        before: card.state.clone(),
    }
}
fn active(app: &App, fields: &BTreeMap<String, String>, answered: bool) -> AppResult<Presentation> {
    let p = app
        .practice
        .current
        .as_ref()
        .ok_or_else(AppError::conflict)?;
    if p.prompt.id != field(fields, "prompt_id")
        || p.attempt_id.is_some() != answered
        || app.practice.studies[&p.prompt.study_id].status == Status::Dropped
    {
        return Err(AppError::conflict());
    }
    for t in &p.targets {
        let c = app
            .practice
            .cards
            .get(&t.card_id)
            .ok_or_else(AppError::conflict)?;
        if !answered && (c.revision != t.revision || c.state != t.before) {
            return Err(AppError::conflict());
        }
    }
    Ok(p.clone())
}
fn make_prompt(
    app: &App,
    study: &Study,
    cards: Vec<&Card>,
    scope: Scope,
    cue: Cue,
    now: i64,
    seed: &str,
) -> Presentation {
    let v = &app.library.versions[&study.version_id];
    let all = units(v);
    let first = cards[0].unit_index;
    let section = v
        .sections
        .iter()
        .find(|s| s.units.iter().any(|u| u.id == cards[0].unit_id))
        .expect("Card Unit in Version");
    let context = if v.kind == Kind::Text
        && matches!(scope, Scope::Unit { .. })
        && !matches!(cue, Cue::SectionHeading | Cue::WholeTitle)
    {
        first.checked_sub(1).map(|i| all[i].text.clone())
    } else {
        None
    };
    let text = &all[first].text;
    let rendered = match cue {
        Cue::Read => text.clone(),
        Cue::FirstLetters => grading::normalized_words(text)
            .iter()
            .filter_map(|w| w.chars().next())
            .map(|c| c.to_string())
            .collect::<Vec<_>>()
            .join(" "),
        Cue::FirstLetter => grading::normalized_words(text)
            .first()
            .and_then(|w| w.chars().next())
            .map(|c| c.to_string())
            .unwrap_or_default(),
        Cue::PreviousLine => String::new(),
        Cue::SectionHeading => section.heading.clone(),
        Cue::WholeTitle => app.library.works[&v.work_id].title.clone(),
        Cue::MappingKey => crate::library::mapping_pair(text)
            .expect("validated mapping Unit")
            .0
            .into(),
    };
    Presentation {
        prompt: Prompt {
            id: id(seed, "prompt"),
            study_id: study.id.clone(),
            version_id: v.id.clone(),
            version_hash: v.normalized_hash.clone(),
            title: if v.kind == Kind::KeyedMapping {
                MAPPING_PROMPT_TITLE.into()
            } else {
                app.library.works[&v.work_id].title.clone()
            },
            scope,
            unit_ids: cards.iter().map(|c| c.unit_id.clone()).collect(),
            cue_level: cue,
            context,
            cue: rendered,
            assisted: cue.assisted(),
        },
        targets: cards.iter().map(|c| target(c, cue)).collect(),
        created_ms: now,
        revealed: false,
        attempt_id: None,
    }
}
fn introduce_card(app: &mut App, study: &Study, index: usize, now: i64, seed: &str) -> String {
    let version = &app.library.versions[&study.version_id];
    let unit = units(version)[index];
    let cid = id(seed, "card");
    app.practice.cards.insert(
        cid.clone(),
        Card {
            id: cid.clone(),
            study_id: study.id.clone(),
            unit_id: unit.id.clone(),
            unit_index: index,
            direction: Direction::Forward,
            introduced_ms: now,
            revision: 0,
            state: initial(now, version.kind),
        },
    );
    cid
}

fn next(
    app: &mut App,
    fields: &BTreeMap<String, String>,
    now: i64,
    seed: &str,
) -> AppResult<MutationResult> {
    if (!field(fields, "card_id").is_empty() && !field(fields, "recite").is_empty())
        || (!field(fields, "section_id").is_empty() && field(fields, "recite") != "section")
    {
        return Err(AppError::new(
            422,
            "Choose one Card or one Recitation scope.",
        ));
    }
    let after = field(fields, "after");
    if let Some(p) = &app.practice.current {
        if after.is_empty() {
            if (!field(fields, "study_id").is_empty()
                && field(fields, "study_id") != p.prompt.study_id)
                || (!field(fields, "card_id").is_empty()
                    && !p
                        .targets
                        .iter()
                        .any(|t| t.card_id == field(fields, "card_id")))
                || (!field(fields, "recite").is_empty()
                    && !matches!(p.prompt.scope, Scope::Section { .. } | Scope::Whole))
            {
                return Err(AppError::conflict());
            }
            if !field(fields, "recite").is_empty() {
                let matches = match &p.prompt.scope {
                    Scope::Whole => field(fields, "recite") == "whole",
                    Scope::Section { section_id } => {
                        field(fields, "recite") == "section"
                            && field(fields, "section_id") == section_id
                    }
                    Scope::Unit { .. } => false,
                };
                if !matches {
                    return Err(AppError::conflict());
                }
            }
            return Ok(response(None, held(app)?));
        }
        if p.prompt.id != after || p.attempt_id.is_none() {
            return Err(AppError::conflict());
        }
    } else if !after.is_empty() {
        return Err(AppError::conflict());
    }
    // Focused NATO is deliberate alphabet practice, not the automatic due queue.
    // Derive the next letter from the held prompt: no cursor or schedule rewrite.
    let nato_next = app.practice.current.as_ref().and_then(|p| {
        let study = &app.practice.studies[&p.prompt.study_id];
        (crate::library::is_nato(&app.library.versions[&study.version_id])
            && study.status == Status::Active
            && field(fields, "card_id").is_empty()
            && field(fields, "recite").is_empty()
            && (field(fields, "study_id").is_empty() || field(fields, "study_id") == study.id))
            .then(|| (study.clone(), (p.targets[0].unit_index + 1) % 26))
    });
    app.practice.current = None;
    let sid = field(fields, "study_id");
    if !sid.is_empty() && !app.practice.studies.contains_key(sid) {
        return Err(AppError::new(404, "Study not found."));
    }
    let recite = field(fields, "recite");
    if !recite.is_empty() {
        let study=app.practice.studies.get(sid).ok_or_else(||AppError::new(422,"Recitation requires study_id and recite=whole|section, with section_id for a Section."))?;
        if study.status == Status::Dropped {
            return Err(AppError::conflict());
        }
        let v = &app.library.versions[&study.version_id];
        if v.kind == Kind::KeyedMapping {
            return Err(AppError::new(
                422,
                "Mapping practice currently supports one key at a time, forward only.",
            ));
        }
        let (scope, cue, ids) = match recite {
            "whole" => (
                Scope::Whole,
                Cue::WholeTitle,
                units(v).iter().map(|u| u.id.as_str()).collect::<Vec<_>>(),
            ),
            "section" => {
                let s = v
                    .sections
                    .iter()
                    .find(|s| s.id == field(fields, "section_id"))
                    .ok_or_else(|| AppError::new(422, "Choose a Section of this Version."))?;
                (
                    Scope::Section {
                        section_id: s.id.clone(),
                    },
                    Cue::SectionHeading,
                    s.units.iter().map(|u| u.id.as_str()).collect(),
                )
            }
            _ => return Err(AppError::new(422, "Use recite=whole|section.")),
        };
        let cards = ids
            .iter()
            .map(|uid| {
                app.practice
                    .cards
                    .values()
                    .find(|c| c.study_id == sid && &c.unit_id == uid)
                    .ok_or_else(|| {
                        AppError::new(409, "Introduce every Unit in this scope before Recitation.")
                    })
            })
            .collect::<AppResult<Vec<_>>>()?;
        let presentation = make_prompt(app, study, cards, scope, cue, now, seed);
        let expected_words = prompt_words(app, &presentation);
        if !fits_grading_capacity(
            expected_words,
            expected_words,
            &matching_siblings(app, &presentation, true),
        ) {
            return Err(AppError::new(
                422,
                "This Recitation exceeds the 1,000,000-cell local grading capacity. Choose a smaller Section.",
            ));
        }
        app.practice.current = Some(presentation);
    } else {
        let explicit = field(fields, "card_id");
        let chosen = if let Some((study, index)) = nato_next {
            Some(
                app.practice
                    .cards
                    .values()
                    .find(|c| c.study_id == study.id && c.unit_index == index)
                    .map(|c| c.id.clone())
                    .unwrap_or_else(|| introduce_card(app, &study, index, now, seed)),
            )
        } else if !explicit.is_empty() {
            let card = app
                .practice
                .cards
                .get(explicit)
                .ok_or_else(|| AppError::new(404, "Card not found."))?;
            if (!sid.is_empty() && card.study_id != sid)
                || app.practice.studies[&card.study_id].status == Status::Dropped
            {
                return Err(AppError::conflict());
            }
            Some(card.id.clone())
        } else {
            app.practice
                .cards
                .values()
                .filter(|c| {
                    (sid.is_empty() || c.study_id == sid)
                        && app.practice.studies[&c.study_id].status != Status::Dropped
                        && c.state.schedule.due_ms <= now
                })
                .min_by_key(|c| {
                    (
                        c.state.schedule.due_ms,
                        app.practice.studies[&c.study_id].created_ms,
                        &c.study_id,
                        c.unit_index,
                    )
                })
                .map(|c| c.id.clone())
        };
        let chosen = if let Some(chosen) = chosen {
            Some(chosen)
        } else {
            let study = app
                .practice
                .studies
                .values()
                .filter(|s| (sid.is_empty() || s.id == sid) && s.status == Status::Active)
                .filter(|s| {
                    app.practice
                        .cards
                        .values()
                        .filter(|c| {
                            c.study_id == s.id && c.introduced_ms > now.saturating_sub(DAY_MS)
                        })
                        .count()
                        < if s.nato_all_letters {
                            26
                        } else {
                            s.pace.limit()
                        }
                })
                .filter(|s| {
                    app.practice
                        .cards
                        .values()
                        .filter(|c| c.study_id == s.id)
                        .count()
                        < units(&app.library.versions[&s.version_id]).len()
                })
                .min_by_key(|s| (s.created_ms, &s.id))
                .cloned();
            study.map(|study| {
                let index = app
                    .practice
                    .cards
                    .values()
                    .filter(|c| c.study_id == study.id)
                    .count();
                introduce_card(app, &study, index, now, seed)
            })
        };
        if let Some(cid) = chosen {
            let c = &app.practice.cards[&cid];
            let study = &app.practice.studies[&c.study_id];
            app.practice.current = Some(make_prompt(
                app,
                study,
                vec![c],
                Scope::Unit {
                    unit_id: c.unit_id.clone(),
                    position: c.unit_index + 1,
                },
                c.state.cue,
                now,
                seed,
            ));
        }
    }
    Ok(response(
        None,
        if app.practice.current.is_some() {
            held(app)?
        } else {
            json!({"prompt":null,"due":due(app,now)})
        },
    ))
}
fn grade_words(version: &Version, text: &str) -> Vec<String> {
    if version.kind == Kind::KeyedMapping {
        grading::mapping_words(text)
    } else {
        grading::normalized_words(text)
    }
}
fn prompt_words(app: &App, p: &Presentation) -> usize {
    let v = &app.library.versions[&p.prompt.version_id];
    let all = units(v);
    p.targets
        .iter()
        .map(|t| grade_words(v, expected_unit(v, all[t.unit_index])).len())
        .sum()
}
fn matching_siblings<'a>(
    app: &'a App,
    p: &Presentation,
    include_siblings: bool,
) -> Vec<(&'a Version, String)> {
    let v = &app.library.versions[&p.prompt.version_id];
    if v.kind == Kind::KeyedMapping {
        return Vec::new();
    }
    let start = p.targets[0].unit_index;
    // Match sibling scope by ordinal for a Unit, by Section position for a
    // Section, and full text for whole recitation. Different boundaries are legal.
    app.library
        .versions
        .values()
        .filter(|s| include_siblings && s.work_id == v.work_id && s.id != v.id)
        .filter_map(|s| {
            let expected = match &p.prompt.scope {
                Scope::Whole => s.text.clone(),
                Scope::Unit { .. } => units(s).get(start)?.text.clone(),
                Scope::Section { section_id } => {
                    let i = v.sections.iter().position(|s| &s.id == section_id)?;
                    s.sections
                        .get(i)?
                        .units
                        .iter()
                        .map(|u| u.text.as_str())
                        .collect()
                }
            };
            Some((s, expected))
        })
        .collect()
}
fn fits_grading_capacity(
    expected_words: usize,
    typed_words: usize,
    siblings: &[(&Version, String)],
) -> bool {
    let fits = |expected: usize| {
        expected
            .checked_add(1)
            .and_then(|n| {
                typed_words
                    .checked_add(1)
                    .and_then(|typed| n.checked_mul(typed))
            })
            .is_some_and(|cells| cells <= 1_000_000)
    };
    fits(expected_words)
        && siblings
            .iter()
            .all(|(_, text)| fits(grading::normalized_words(text).len()))
}
fn grade(app: &App, p: &Presentation, answer: &str, include_siblings: bool) -> AppResult<Grade> {
    let v = &app.library.versions[&p.prompt.version_id];
    let all = units(v);
    if v.kind == Kind::OrderedList {
        let start = p.targets[0].unit_index;
        let end = p.targets.last().unwrap().unit_index + 1;
        return crate::ordered::grade(v, start..end, answer);
    }
    let texts = all.iter().map(|u| expected_unit(v, u)).collect::<Vec<_>>();
    let start = p.targets[0].unit_index;
    let end = p.targets.last().unwrap().unit_index + 1;
    let siblings = matching_siblings(app, p, include_siblings);
    let refs = siblings
        .iter()
        .map(|(s, text)| grading::SiblingVersion {
            id: &s.id,
            name: &s.name,
            expected: text,
        })
        .collect::<Vec<_>>();
    // The shared grader's edit grid is quadratic. Refuse a bounded request
    // before allocating it; the saved Prompt and answer operation stay intact.
    if !fits_grading_capacity(
        prompt_words(app, p),
        grade_words(v, answer).len(),
        &siblings,
    ) {
        return Err(AppError::new(
            422,
            "This answer exceeds the 1,000,000-cell local grading capacity. Use a smaller Section or a shorter answer.",
        ));
    }
    let version = grading::Version {
        units: &texts,
        spelling_strict: v.spelling_strict,
    };
    let mut result = if v.kind == Kind::KeyedMapping {
        grading::grade_mapping(version, start..end, answer)
    } else {
        grading::grade(version, start..end, answer, &refs)
    }
    .map_err(|e| AppError::new(422, e.to_string()))?;
    // exact-v2 spans are relative to its input. Restore expected spans to the
    // immutable complete Unit; typed spans remain relative to the raw answer.
    if v.kind == Kind::KeyedMapping {
        for diff in &mut result.diff {
            use grading::Change;
            let offset = crate::library::mapping_pair(&all[diff.unit_index].text)
                .unwrap()
                .0
                .len()
                + 1;
            let expected = match &mut diff.change {
                Change::Correct { expected, .. }
                | Change::Slip { expected, .. }
                | Change::Wrong { expected, .. }
                | Change::Missing { expected } => Some(expected),
                Change::Extra { .. } => None,
            };
            if let Some(word) = expected {
                word.span.start += offset;
                word.span.end += offset;
            }
        }
    }
    Ok(result)
}
/// Deliberate, bounded lesson entry. Reads never import or abandon a held answer.
fn lesson_start(
    app: &mut App,
    fields: &BTreeMap<String, String>,
    now: i64,
    seed: &str,
) -> AppResult<MutationResult> {
    if fields.keys().any(|k| {
        !matches!(
            k.as_str(),
            "csrf" | "operation_id" | "revision" | "starter" | "version_id" | "after" | "leave"
        )
    }) {
        return Err(AppError::new(
            422,
            "Choose a lesson, without grading or content overrides.",
        ));
    }
    if !field(fields, "after").is_empty() && !field(fields, "leave").is_empty() {
        return Err(AppError::new(422, "Choose after or leave, not both."));
    }
    let starter = field(fields, "starter");
    let supplied = field(fields, "version_id");
    if starter.is_empty() == supplied.is_empty() {
        return Err(AppError::new(
            422,
            "Choose one starter or supplied Version.",
        ));
    }
    let vid = if starter.is_empty() {
        supplied.to_string()
    } else {
        app.library
            .add(crate::lessons::input(starter)?, seed, None)?
    };
    let version = app
        .library
        .versions
        .get(&vid)
        .ok_or_else(|| AppError::new(404, "Version not found."))?;
    studyable(version)?;
    if version.kind == Kind::KeyedMapping && !crate::library::is_nato(version) {
        return Err(AppError::new(
            422,
            "Use normal Study practice for this mapping; lessons support text, ordered lists and the NATO starter.",
        ));
    }
    if units(version).len() > 128 {
        return Err(AppError::new(
            422,
            "Lessons support up to 128 meaningful phrases or items. Split longer material into separate Versions.",
        ));
    }
    let resume = app
        .practice
        .current
        .as_ref()
        .filter(|p| p.prompt.version_id == vid && field(fields, "after").is_empty())
        .cloned();
    if let Some(p) = &app.practice.current {
        if !field(fields, "leave").is_empty() && field(fields, "leave") != p.prompt.id {
            return Err(AppError::conflict());
        }
        let may_leave = p.attempt_id.is_none() && field(fields, "leave") == p.prompt.id;
        let may_finish = p.attempt_id.is_some() && field(fields, "after") == p.prompt.id;
        if resume.is_none() && !may_leave && !may_finish {
            return Err(AppError::new(
                409,
                "Your current answer is still here. Check it before switching lessons.",
            ));
        }
    } else if !field(fields, "after").is_empty() || !field(fields, "leave").is_empty() {
        return Err(AppError::conflict());
    }
    let learn = BTreeMap::from([("version_id".into(), vid.clone())]);
    let result = mutate(app, "/practice/learn", &learn, now, seed)?;
    let sid = result.entity_id.expect("Study identity");
    let s = app.practice.studies.get_mut(&sid).unwrap();
    s.status = Status::Active;
    s.lesson = true;
    if crate::library::is_nato(&app.library.versions[&vid]) {
        s.nato_all_letters = true;
    }
    let s = s.clone();
    let count = units(&app.library.versions[&vid]).len();
    let introduced = app
        .practice
        .cards
        .values()
        .filter(|c| c.study_id == sid)
        .count();
    for index in introduced..count {
        introduce_card(app, &s, index, now, &format!("{seed}:unit:{index}"));
    }
    if resume.is_some() {
        return Ok(response(None, held(app)?));
    }
    app.practice.current = None;
    lesson_present(
        app,
        &sid,
        0,
        app.library.versions[&vid].kind == Kind::OrderedList,
        now,
        seed,
    )
}
fn lesson_present(
    app: &mut App,
    sid: &str,
    index: usize,
    whole: bool,
    now: i64,
    seed: &str,
) -> AppResult<MutationResult> {
    let study = app
        .practice
        .studies
        .get(sid)
        .ok_or_else(AppError::conflict)?;
    let v = &app.library.versions[&study.version_id];
    if whole {
        if v.kind == Kind::KeyedMapping {
            return Err(AppError::new(422, "Mappings use individual key recall."));
        }
        let mut cards = app
            .practice
            .cards
            .values()
            .filter(|c| c.study_id == sid)
            .collect::<Vec<_>>();
        cards.sort_by_key(|c| c.unit_index);
        let p = make_prompt(app, study, cards, Scope::Whole, Cue::WholeTitle, now, seed);
        if !fits_grading_capacity(
            prompt_words(app, &p),
            prompt_words(app, &p),
            &matching_siblings(app, &p, true),
        ) {
            return Err(AppError::new(
                422,
                "This lesson exceeds local grading capacity. Choose a shorter text.",
            ));
        }
        app.practice.current = Some(p);
    } else {
        let card = app
            .practice
            .cards
            .values()
            .find(|c| c.study_id == sid && c.unit_index == index)
            .ok_or_else(AppError::conflict)?;
        app.practice.current = Some(make_prompt(
            app,
            study,
            vec![card],
            Scope::Unit {
                unit_id: card.unit_id.clone(),
                position: index + 1,
            },
            card.state.cue,
            now,
            seed,
        ));
    }
    Ok(response(None, held(app)?))
}
fn lesson_next(
    app: &mut App,
    fields: &BTreeMap<String, String>,
    now: i64,
    seed: &str,
) -> AppResult<MutationResult> {
    if fields.keys().any(|k| {
        !matches!(
            k.as_str(),
            "csrf" | "operation_id" | "revision" | "after" | "mode"
        )
    }) {
        return Err(AppError::new(
            422,
            "Submit the held result and lesson mode only.",
        ));
    }
    let p = app
        .practice
        .current
        .as_ref()
        .ok_or_else(AppError::conflict)?;
    if p.attempt_id.is_none() || field(fields, "after") != p.prompt.id {
        return Err(AppError::conflict());
    }
    let sid = p.prompt.study_id.clone();
    let study = &app.practice.studies[&sid];
    if !study.lesson || study.status == Status::Dropped {
        return Err(AppError::conflict());
    }
    let v = &app.library.versions[&study.version_id];
    let mode = field(fields, "mode");
    if !matches!(mode, "" | "phrases" | "join" | "whole" | "middle") {
        return Err(AppError::new(
            422,
            "Use phrases, join, whole or middle lesson mode.",
        ));
    }
    if mode == "join" && v.kind == Kind::Text {
        let section = v
            .sections
            .iter()
            .find(|s| s.units.iter().any(|u| u.id == p.targets[0].unit_id))
            .ok_or_else(AppError::conflict)?;
        let mut cards = app
            .practice
            .cards
            .values()
            .filter(|c| c.study_id == sid && section.units.iter().any(|u| u.id == c.unit_id))
            .collect::<Vec<_>>();
        cards.sort_by_key(|c| c.unit_index);
        let presentation = make_prompt(
            app,
            study,
            cards,
            Scope::Section {
                section_id: section.id.clone(),
            },
            Cue::SectionHeading,
            now,
            seed,
        );
        if !fits_grading_capacity(
            prompt_words(app, &presentation),
            prompt_words(app, &presentation),
            &matching_siblings(app, &presentation, true),
        ) {
            return Err(AppError::new(
                422,
                "This joined passage exceeds local grading capacity.",
            ));
        }
        app.practice.current = Some(presentation);
        return Ok(response(None, held(app)?));
    }
    let whole = v.kind == Kind::OrderedList || mode == "whole";
    let index = if mode == "middle" {
        units(v).len() / 2
    } else if matches!(p.prompt.scope, Scope::Whole) {
        0
    } else {
        (p.targets
            .last()
            .expect("a held prompt has targets")
            .unit_index
            + 1)
            % units(v).len()
    };
    lesson_present(app, &sid, index, whole, now, seed)
}

pub fn mutate(
    app: &mut App,
    path: &str,
    fields: &BTreeMap<String, String>,
    now: i64,
    seed: &str,
) -> AppResult<MutationResult> {
    match path {
        "/practice/lesson/start" => lesson_start(app, fields, now, seed),
        "/practice/lesson/next" => lesson_next(app, fields, now, seed),
        "/practice/nato/start" => {
            if fields
                .keys()
                .any(|k| !matches!(k.as_str(), "csrf" | "operation_id" | "revision"))
            {
                return Err(AppError::new(
                    422,
                    "Start practice without content or grading overrides.",
                ));
            }
            // Engine applies this whole operation to a clone: a held different
            // Study or import conflict cannot leave a partial starter behind.
            let vid = app.library.add(crate::library::nato_input()?, seed, None)?;
            if !crate::library::is_nato(&app.library.versions[&vid]) {
                return Err(AppError::new(
                    409,
                    "The existing mapping differs from the canonical NATO starter.",
                ));
            }
            if app
                .practice
                .current
                .as_ref()
                .is_some_and(|p| p.prompt.version_id != vid)
            {
                return Err(AppError::new(
                    409,
                    "Finish the saved review before starting NATO practice.",
                ));
            }
            let learn = BTreeMap::from([("version_id".into(), vid)]);
            let result = mutate(app, "/practice/learn", &learn, now, seed)?;
            let sid = result.entity_id.expect("learn returns its Study");
            let study = app
                .practice
                .studies
                .get_mut(&sid)
                .expect("learn retains its Study");
            study.status = Status::Active;
            study.nato_all_letters = true;
            next(app, &BTreeMap::from([("study_id".into(), sid)]), now, seed)
        }
        "/practice/learn" => {
            let vid = field(fields, "version_id");
            let v = app
                .library
                .versions
                .get(vid)
                .ok_or_else(|| AppError::new(404, "Version not found."))?;
            studyable(v)?;
            let pace = Pace::parse(field(fields, "pace"))?;
            let study = if let Some(s) = app.practice.studies.values().find(|s| s.version_id == vid)
            {
                s.clone()
            } else {
                let s = Study {
                    id: id(seed, "study"),
                    version_id: vid.into(),
                    version_hash: v.normalized_hash.clone(),
                    pace,
                    nato_all_letters: false,
                    lesson: false,
                    status: Status::Active,
                    created_ms: now,
                };
                app.study_version_refs.insert(s.id.clone(), vid.into());
                app.practice.studies.insert(s.id.clone(), s.clone());
                s
            };
            Ok(response(Some(study.id.clone()), json!({"study":study})))
        }
        "/practice/pause" | "/practice/resume" | "/practice/drop" => {
            let sid = field(fields, "study_id");
            let s = app
                .practice
                .studies
                .get_mut(sid)
                .ok_or_else(|| AppError::new(404, "Study not found."))?;
            // Resume deliberately reactivates retained history, including a dropped Study.
            if s.status == Status::Dropped && path == "/practice/pause" {
                return Err(AppError::conflict());
            }
            s.status = match path {
                "/practice/pause" => Status::Paused,
                "/practice/resume" => Status::Active,
                _ => Status::Dropped,
            };
            let study = s.clone();
            if study.status == Status::Dropped
                && app
                    .practice
                    .current
                    .as_ref()
                    .is_some_and(|p| p.prompt.study_id == sid)
            {
                app.practice.current = None;
            }
            Ok(response(Some(sid.into()), json!({"study":study})))
        }
        "/practice/next" => next(app, fields, now, seed),
        "/practice/reveal" => {
            let p = active(app, fields, false)?;
            let v = &app.library.versions[&p.prompt.version_id];
            let all = units(v);
            let expected = p
                .targets
                .iter()
                .map(|t| json!({"unit_id":t.unit_id,"text":expected_unit(v, all[t.unit_index])}))
                .collect::<Vec<_>>();
            let aid = if p.revealed {
                app.practice
                    .attempts
                    .values()
                    .find(|a| a.disclosure && a.prompt.id == p.prompt.id)
                    .map(|a| a.id.clone())
            } else {
                let aid = id(seed, "attempt");
                let grade = grade(app, &p, "", false)?;
                let effects = p.targets.iter().map(disclosure_effect).collect::<Vec<_>>();
                for e in &effects {
                    app.practice.cards.get_mut(&e.card_id).unwrap().revision += 1;
                }
                app.practice.attempts.insert(
                    aid.clone(),
                    Attempt {
                        id: aid.clone(),
                        sequence: app.revision + 1,
                        prompt: p.prompt.clone(),
                        version_id: p.prompt.version_id.clone(),
                        version_hash: p.prompt.version_hash.clone(),
                        raw_answer: String::new(),
                        grade,
                        acting_token: None,
                        assisted: true,
                        disclosure: true,
                        revealed: true,
                        created_ms: now,
                        algorithm: learning::ALGORITHM.into(),
                        effects,
                    },
                );
                for t in &mut app.practice.current.as_mut().unwrap().targets {
                    t.revision += 1;
                }
                Some(aid)
            };
            app.practice.current.as_mut().unwrap().revealed = true;
            Ok(response(
                aid.clone(),
                json!({"prompt_id":p.prompt.id,"assisted":true,"disclosure_attempt_id":aid,"expected":expected}),
            ))
        }
        "/practice/answer" => {
            let p = active(app, fields, false)?;
            if fields.keys().any(|k| {
                !matches!(
                    k.as_str(),
                    "csrf" | "operation_id" | "revision" | "prompt_id" | "answer"
                )
            }) {
                return Err(AppError::new(
                    422,
                    "Submit only your answer. Scry owns the expected value, Verdict and schedule.",
                ));
            }
            let answer = fields.get("answer").ok_or_else(|| {
                AppError::new(
                    422,
                    "Supply an answer, including an empty answer for a miss.",
                )
            })?;
            if answer.len() > crate::library::TEXT_BYTES {
                return Err(AppError::new(422, "Answer exceeds the Version text limit."));
            }
            let grade = grade(app, &p, answer, true)?;
            let assisted = p.prompt.assisted || p.revealed;
            let effects = p
                .targets
                .iter()
                .zip(&grade.units)
                .map(|(t, g)| effect(t, g.verdict, p.revealed, now))
                .collect::<AppResult<Vec<_>>>()?;
            for e in &effects {
                let c = app.practice.cards.get_mut(&e.card_id).unwrap();
                c.state = e.after.clone();
                c.revision += 1;
            }
            let aid = id(seed, "attempt");
            app.practice.attempts.insert(
                aid.clone(),
                Attempt {
                    id: aid.clone(),
                    sequence: app.revision + 1,
                    prompt: p.prompt.clone(),
                    version_id: p.prompt.version_id,
                    version_hash: p.prompt.version_hash,
                    raw_answer: answer.clone(),
                    grade,
                    acting_token: None,
                    assisted,
                    disclosure: false,
                    revealed: p.revealed,
                    created_ms: now,
                    algorithm: learning::ALGORITHM.into(),
                    effects,
                },
            );
            app.practice.current.as_mut().unwrap().attempt_id = Some(aid.clone());
            Ok(response(Some(aid.clone()), json!({"attempt_id":aid})))
        }
        "/practice/correct" => {
            let p = active(app, fields, true)?;
            if app.library.versions[&p.prompt.version_id].kind == Kind::KeyedMapping {
                return Err(AppError::new(
                    422,
                    "Mapping Verdicts are determined by Scry, not self-grading.",
                ));
            }
            let aid = field(fields, "attempt_id");
            if p.attempt_id.as_deref() != Some(aid) {
                return Err(AppError::conflict());
            }
            let a = &app.practice.attempts[aid];
            let uid = field(fields, "unit_id");
            let e = a
                .effects
                .iter()
                .find(|e| e.unit_id == uid || (uid.is_empty() && a.effects.len() == 1))
                .ok_or_else(|| AppError::new(422, "Choose a target unit_id for the Correction."))?;
            let verdict = match field(fields, "verdict") {
                "Good" => Verdict::Good,
                "Again" => Verdict::Again,
                _ => return Err(AppError::new(422, "Use verdict Good or Again.")),
            };
            let c = &app.practice.cards[&e.card_id];
            if verdict == e.verdict
                || c.revision != e.revision_before + 1
                || c.state != e.after
                || app
                    .practice
                    .corrections
                    .values()
                    .any(|c| c.attempt_id == aid && c.unit_id == e.unit_id)
            {
                return Err(AppError::conflict());
            }
            let t = Target {
                card_id: e.card_id.clone(),
                unit_id: e.unit_id.clone(),
                unit_index: e.unit_index,
                cue: e.cue_level,
                revision: e.revision_before,
                before: e.before.clone(),
            };
            let replacement = effect(&t, verdict, a.revealed, a.created_ms)?;
            let cid = id(seed, "correction");
            app.practice.corrections.insert(
                cid.clone(),
                Correction {
                    id: cid.clone(),
                    sequence: app.revision + 1,
                    attempt_id: aid.into(),
                    unit_id: e.unit_id.clone(),
                    verdict,
                    authority: "owner".into(),
                    acting_token: None,
                    created_ms: now,
                    revision_before: c.revision,
                    before: c.state.clone(),
                    after: replacement.after.clone(),
                },
            );
            let c = app.practice.cards.get_mut(&e.card_id).unwrap();
            c.state = replacement.after;
            c.revision += 1;
            Ok(response(Some(cid.clone()), json!({"correction_id":cid})))
        }
        _ => Err(AppError::new(404, "Practice action not found.")),
    }
}
pub fn attribute(app: &mut App, previous: &App, actor: Option<String>) {
    for (id, a) in &mut app.practice.attempts {
        if !previous.practice.attempts.contains_key(id) {
            a.acting_token = actor.clone();
        }
    }
    for (id, c) in &mut app.practice.corrections {
        if !previous.practice.corrections.contains_key(id) {
            c.acting_token = actor.clone();
        }
    }
}
pub fn due(app: &App, now: i64) -> Value {
    let mut cards = app
        .practice
        .cards
        .values()
        .filter(|c| app.practice.studies[&c.study_id].status != Status::Dropped)
        .collect::<Vec<_>>();
    cards.sort_by_key(|c| (c.state.schedule.due_ms, &c.study_id, c.unit_index));
    json!({"cards":cards,"due":cards.iter().filter(|c|c.state.schedule.due_ms<=now).map(|c|&c.id).collect::<Vec<_>>(),"next_due_ms":cards.iter().map(|c|c.state.schedule.due_ms).min()})
}
/// The Stream presents through the same selection policy as API `next`.
/// Reads retain an existing Prompt/result and never advance it.
pub fn ensure_stream(app: &mut App, now: i64, seed: &str) -> AppResult<()> {
    if app.practice.current.is_none() {
        let previous = app.practice.clone();
        next(app, &BTreeMap::new(), now, seed)?;
        if app.practice != previous {
            app.revision += 1;
        }
    }
    Ok(())
}
pub fn history(app: &App) -> AppResult<Value> {
    if app.practice.protected() || app.protected_goal().is_some() {
        return Err(AppError::new(
            409,
            "Record assistance or answer before history or export.",
        ));
    }
    let versions = app
        .practice
        .studies
        .values()
        .map(|s| (&s.version_id, &app.library.versions[&s.version_id]))
        .collect::<BTreeMap<_, _>>();
    Ok(
        json!({"studies":app.practice.studies,"cards":app.practice.cards,"attempts":app.practice.attempts,"corrections":app.practice.corrections,"versions":versions,"operations":app.operations}),
    )
}
pub fn receipt_entity(app: &App, r: &Receipt, id: &str) -> bool {
    r.path
        .as_deref()
        .is_some_and(|p| p.starts_with("/practice/"))
        && (app.practice.studies.contains_key(id)
            || app.practice.attempts.contains_key(id)
            || app.practice.corrections.contains_key(id))
}
/// Recovery rejects broken identity, altered grades/effects and inconsistent
/// projections. Empty historical stores retain their original representation.
pub fn validate(app: &App) -> AppResult<()> {
    let fail = || AppError::new(422, "Invalid Study practice history or projection.");
    let p = &app.practice;
    let mut versions = BTreeSet::new();
    for (sid, s) in &p.studies {
        let v = app.library.versions.get(&s.version_id).ok_or_else(fail)?;
        if sid != &s.id
            || !valid_id(sid)
            || !versions.insert(&s.version_id)
            || s.version_hash != v.normalized_hash
            || !app.library.works.contains_key(&v.work_id)
            || studyable(v).is_err()
            || (s.nato_all_letters && !crate::library::is_nato(v))
            || app.study_version_refs.get(sid) != Some(&s.version_id)
        {
            return Err(fail());
        }
        let mut cards = p
            .cards
            .values()
            .filter(|c| &c.study_id == sid)
            .collect::<Vec<_>>();
        cards.sort_by_key(|c| c.unit_index);
        let all = units(v);
        for (i, c) in cards.iter().enumerate() {
            if c.unit_index != i
                || all.get(i).is_none_or(|u| u.id != c.unit_id)
                || c.introduced_ms < s.created_ms
            {
                return Err(fail());
            }
        }
    }
    let mut projection = BTreeMap::new();
    for (cid, c) in &p.cards {
        if cid != &c.id || !valid_id(cid) || !p.studies.contains_key(&c.study_id) {
            return Err(fail());
        }
        projection.insert(
            cid.clone(),
            (
                initial(
                    c.introduced_ms,
                    app.library.versions[&p.studies[&c.study_id].version_id].kind,
                ),
                0u64,
            ),
        );
    }
    let valid_actor = |a: &Option<String>| {
        a.as_ref()
            .is_none_or(|id| app.owner_tokens.contains_key(id))
    };
    enum Entry<'a> {
        Attempt(&'a Attempt),
        Correction(&'a Correction),
    }
    let mut entries = Vec::new();
    for (aid, a) in &p.attempts {
        if aid != &a.id
            || !valid_id(aid)
            || !valid_actor(&a.acting_token)
            || a.algorithm != learning::ALGORITHM
            || a.raw_answer.len() > crate::library::TEXT_BYTES
            || a.sequence > app.revision
            || a.version_id != a.prompt.version_id
            || a.version_hash != a.prompt.version_hash
        {
            return Err(fail());
        }
        let v = app.library.versions.get(&a.version_id).ok_or_else(fail)?;
        if v.normalized_hash != a.version_hash
            || a.effects.is_empty()
            || a.prompt.unit_ids
                != a.effects
                    .iter()
                    .map(|e| e.unit_id.clone())
                    .collect::<Vec<_>>()
        {
            return Err(fail());
        }
        let targets = a
            .effects
            .iter()
            .map(|e| Target {
                card_id: e.card_id.clone(),
                unit_id: e.unit_id.clone(),
                unit_index: e.unit_index,
                cue: e.cue_level,
                revision: e.revision_before,
                before: e.before.clone(),
            })
            .collect::<Vec<_>>();
        validate_targets(app, &a.prompt, &targets)?;
        // Sibling Versions can be added later. The deterministic base grade is
        // immutable; hints retain the sibling name even if that unused Version
        // is deleted later. No contemporary Library state rewrites an Attempt.
        let pres = Presentation {
            prompt: a.prompt.clone(),
            targets,
            created_ms: a.created_ms,
            revealed: a.revealed,
            attempt_id: Some(aid.clone()),
        };
        let actual = grade(app, &pres, &a.raw_answer, false)?;
        let mut expected = a.grade.clone();
        expected.wrong_version_hint = actual.wrong_version_hint.clone();
        if expected != actual
            || a.assisted != (a.prompt.assisted || a.revealed)
            || (a.disclosure && (!a.revealed || !a.raw_answer.is_empty()))
        {
            return Err(fail());
        }
        entries.push((a.sequence, Entry::Attempt(a)));
    }
    for (cid, c) in &p.corrections {
        if cid != &c.id
            || !valid_id(cid)
            || !valid_actor(&c.acting_token)
            || c.authority != "owner"
            || c.sequence > app.revision
        {
            return Err(fail());
        }
        entries.push((c.sequence, Entry::Correction(c)));
    }
    entries.sort_by_key(|e| e.0);
    let mut last = None;
    let mut corrected = BTreeSet::new();
    let mut seen = BTreeSet::new();
    for (seq, entry) in entries {
        if last == Some(seq) {
            return Err(fail());
        }
        last = Some(seq);
        match entry {
            Entry::Attempt(a) => {
                seen.insert(&a.id);
                if a.effects.len() != a.grade.units.len() {
                    return Err(fail());
                }
                for (e, g) in a.effects.iter().zip(&a.grade.units) {
                    let c = p.cards.get(&e.card_id).ok_or_else(fail)?;
                    let (state, revision) = projection.get_mut(&e.card_id).ok_or_else(fail)?;
                    if c.study_id != a.prompt.study_id
                        || c.unit_id != e.unit_id
                        || c.unit_index != e.unit_index
                        || g.unit_index != e.unit_index
                        || g.verdict != e.verdict
                        || e.cue_level != a.prompt.cue_level
                        || e.assisted != a.assisted
                        || *state != e.before
                        || *revision != e.revision_before
                        || a.created_ms < c.introduced_ms
                    {
                        return Err(fail());
                    }
                    let target = target_from_effect(e);
                    let expected = if a.disclosure {
                        disclosure_effect(&target)
                    } else {
                        effect(&target, e.verdict, a.revealed, a.created_ms)?
                    };
                    if expected != *e {
                        return Err(fail());
                    }
                    *state = e.after.clone();
                    *revision += 1;
                }
            }
            Entry::Correction(c) => {
                let a = p.attempts.get(&c.attempt_id).ok_or_else(fail)?;
                if app.library.versions[&a.version_id].kind == Kind::KeyedMapping {
                    return Err(fail());
                }
                let e = a
                    .effects
                    .iter()
                    .find(|e| e.unit_id == c.unit_id)
                    .ok_or_else(fail)?;
                let (state, revision) = projection.get_mut(&e.card_id).ok_or_else(fail)?;
                if !seen.contains(&a.id)
                    || !corrected.insert((&c.attempt_id, &c.unit_id))
                    || c.verdict == e.verdict
                    || c.created_ms < a.created_ms
                    || *revision != e.revision_before + 1
                    || *revision != c.revision_before
                    || *state != c.before
                    || effect(&target_from_effect(e), c.verdict, a.revealed, a.created_ms)?.after
                        != c.after
                {
                    return Err(fail());
                }
                *state = c.after.clone();
                *revision += 1;
            }
        }
    }
    for (id, (state, rev)) in projection {
        if p.cards[&id].state != state || p.cards[&id].revision != rev {
            return Err(fail());
        }
    }
    if let Some(current) = &p.current {
        validate_targets(app, &current.prompt, &current.targets)?;
        if p.studies[&current.prompt.study_id].status == Status::Dropped
            || current.created_ms < p.studies[&current.prompt.study_id].created_ms
        {
            return Err(fail());
        }
        if let Some(aid) = &current.attempt_id {
            let a = p.attempts.get(aid).ok_or_else(fail)?;
            if a.disclosure
                || a.prompt != current.prompt
                || a.revealed != current.revealed
                || a.assisted != (current.revealed || current.prompt.assisted)
                || a.created_ms < current.created_ms
            {
                return Err(fail());
            }
        } else {
            for t in &current.targets {
                let c = &p.cards[&t.card_id];
                if c.state != t.before || c.revision != t.revision {
                    return Err(fail());
                }
            }
        }
    }
    Ok(())
}
fn target_from_effect(e: &Effect) -> Target {
    Target {
        card_id: e.card_id.clone(),
        unit_id: e.unit_id.clone(),
        unit_index: e.unit_index,
        cue: e.cue_level,
        revision: e.revision_before,
        before: e.before.clone(),
    }
}
fn valid_id(id: &str) -> bool {
    id.len() == 32
        && id
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}
fn validate_targets(app: &App, p: &Prompt, targets: &[Target]) -> AppResult<()> {
    let fail = || AppError::new(422, "Invalid frozen Study Prompt.");
    let s = app.practice.studies.get(&p.study_id).ok_or_else(fail)?;
    let v = app.library.versions.get(&s.version_id).ok_or_else(fail)?;
    let all = units(v);
    if !valid_id(&p.id)
        || p.version_id != v.id
        || p.version_hash != v.normalized_hash
        || targets.is_empty()
        || p.assisted != p.cue_level.assisted()
        || (v.kind == Kind::KeyedMapping) != (p.cue_level == Cue::MappingKey)
        || p.unit_ids
            != targets
                .iter()
                .map(|t| t.unit_id.clone())
                .collect::<Vec<_>>()
    {
        return Err(fail());
    }
    for (i, t) in targets.iter().enumerate() {
        let c = app.practice.cards.get(&t.card_id).ok_or_else(fail)?;
        if c.study_id != s.id
            || c.unit_id != t.unit_id
            || c.unit_index != t.unit_index
            || t.cue != p.cue_level
            || all.get(t.unit_index).is_none_or(|u| u.id != t.unit_id)
            || (i > 0 && t.unit_index != targets[i - 1].unit_index + 1)
        {
            return Err(fail());
        }
    }
    let cards = targets
        .iter()
        .map(|t| &app.practice.cards[&t.card_id])
        .collect();
    let rendered = make_prompt(app, s, cards, p.scope.clone(), p.cue_level, 0, "validation");
    let expected_cue = if p.cue_level == Cue::WholeTitle {
        &p.title
    } else {
        &rendered.prompt.cue
    };
    if &p.cue != expected_cue
        || p.context != rendered.prompt.context
        || p.title.trim().is_empty()
        || p.title.len() > 240
    {
        return Err(fail());
    }
    let first = targets[0].unit_index;
    match &p.scope {
        Scope::Unit { unit_id, position }
            if targets.len() == 1 && unit_id == &targets[0].unit_id && *position == first + 1 => {}
        Scope::Whole
            if first == 0
                && targets.len() == all.len()
                && p.cue_level == Cue::WholeTitle
                && p.context.is_none() => {}
        Scope::Section { section_id }
            if v.sections.iter().any(|s| {
                &s.id == section_id && s.units.iter().map(|u| &u.id).eq(p.unit_ids.iter())
            }) && p.cue_level == Cue::SectionHeading
                && p.context.is_none() => {}
        _ => return Err(fail()),
    }
    Ok(())
}
