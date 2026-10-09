use crate::learning::{Card, new_card};
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

pub const SCHEMA: u32 = 1;
pub const TEXT_LIMIT: usize = 32 * 1024;
pub const PHOTO_LIMIT: usize = 4 * 1024 * 1024;
pub const DAY_MS: i64 = 86_400_000;
pub const DAILY_ALLOWANCE: u64 = 3_500_000;
pub const RESERVATION: u64 = 500_000;

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct App {
    pub schema: u32,
    pub revision: u64,
    pub restored_paused: bool,
    pub csrf: String,
    pub goals: BTreeMap<String, Goal>,
    pub concepts: BTreeMap<String, Concept>,
    pub questions: BTreeMap<String, Question>,
    pub occurrence: Option<Occurrence>,
    pub events: Vec<Event>,
    pub overrides: Vec<Override>,
    #[serde(default)]
    pub resets: Vec<Reset>,
    pub operations: BTreeMap<String, Receipt>,
    pub jobs: BTreeMap<String, Job>,
    pub assessments: BTreeMap<String, Assessment>,
    pub spend: Vec<Spend>,
    pub feedback: Vec<Feedback>,
    pub preferences: Preferences,
    pub backup: BackupStatus,
}

impl App {
    pub fn new(csrf: String) -> Self {
        Self {
            schema: SCHEMA,
            revision: 0,
            restored_paused: false,
            csrf,
            goals: BTreeMap::new(),
            concepts: BTreeMap::new(),
            questions: BTreeMap::new(),
            occurrence: None,
            events: Vec::new(),
            overrides: Vec::new(),
            resets: Vec::new(),
            operations: BTreeMap::new(),
            jobs: BTreeMap::new(),
            assessments: BTreeMap::new(),
            spend: Vec::new(),
            feedback: Vec::new(),
            preferences: Preferences::default(),
            backup: BackupStatus::default(),
        }
    }
    pub fn spent(&self, now: i64) -> u64 {
        self.spend
            .iter()
            .filter(|s| s.created_ms > now - DAY_MS)
            .map(|s| s.cost_micros.unwrap_or(s.reserved_micros))
            .fold(0, u64::saturating_add)
    }
    pub fn protected_goal(&self) -> Option<&str> {
        let o = self.occurrence.as_ref()?;
        if !matches!(o.phase.as_str(), "question" | "checking") || o.assisted {
            return None;
        }
        self.questions
            .get(&o.question_id)
            .map(|q| q.goal_id.as_str())
    }
    pub fn protected_concept(&self, id: &str) -> bool {
        self.occurrence.as_ref().is_some_and(|o| {
            matches!(o.phase.as_str(), "question" | "checking") && !o.assisted && o.concept_id == id
        })
    }
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Goal {
    pub id: String,
    pub title: String,
    pub intent: String,
    pub created_ms: i64,
    pub revision: u64,
    pub status: String,
    pub paused: bool,
    pub focused: bool,
    pub archived: bool,
    pub concept_ids: Vec<String>,
    pub job_id: String,
    pub photo: Option<Photo>,
    pub transcript: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Photo {
    pub key: String,
    pub mime: String,
    pub size: usize,
    pub sha256: String,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Note {
    pub text: String,
    pub basis: String,
    pub quotes: Vec<String>,
    pub created_ms: i64,
    pub model: String,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Concept {
    pub id: String,
    pub goal_id: String,
    pub name: String,
    pub summary: String,
    pub notes: Vec<Note>,
    pub prerequisites: Vec<String>,
    pub introduced_ms: Option<i64>,
    pub exposure_ms: Option<i64>,
    pub already_known: bool,
    pub archived: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Content {
    pub version: u64,
    pub prompt: String,
    pub kind: String,
    pub answer: String,
    pub variants: Vec<String>,
    pub choices: Vec<String>,
    pub explanation: String,
    pub basis: String,
    pub quotes: Vec<String>,
    pub required_ideas: Vec<String>,
    pub contradictions: Vec<String>,
    pub model: String,
    pub created_ms: i64,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Question {
    pub id: String,
    pub goal_id: String,
    pub concept_id: String,
    pub versions: Vec<Content>,
    pub card: Card,
    pub schedule_version: u64,
    pub archived: bool,
    pub draft: Option<Content>,
    pub draft_for_version: Option<u64>,
    /// Authored order within an idea; old records retain their original tie-break.
    #[serde(default)]
    pub position: usize,
}
impl Question {
    pub fn content(&self) -> &Content {
        self.versions
            .last()
            .expect("validated question has content")
    }
    pub fn new(
        id: String,
        goal_id: String,
        concept_id: String,
        content: Content,
        now: i64,
    ) -> Self {
        Self {
            id,
            goal_id,
            concept_id,
            versions: vec![content],
            card: new_card(now),
            schedule_version: 0,
            archived: false,
            draft: None,
            draft_for_version: None,
            position: 0,
        }
    }
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Occurrence {
    pub id: String,
    pub question_id: String,
    pub concept_id: String,
    pub presentation: Content,
    pub phase: String,
    pub assisted: bool,
    pub before_card: Card,
    pub schedule_version: u64,
    pub created_ms: i64,
    pub answer: Option<String>,
    pub assessment_id: Option<String>,
    pub result: Option<ReviewResult>,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct ReviewResult {
    pub event_id: String,
    pub outcome: String,
    pub authority: String,
    pub rating: u8,
    pub reviewed_ms: i64,
    pub due_ms: i64,
    pub corrected: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Event {
    pub id: String,
    pub occurrence_id: String,
    pub question_id: String,
    pub concept_id: String,
    pub presentation: Content,
    pub answer: String,
    pub assisted: bool,
    pub before_card: Card,
    pub after_card: Card,
    pub schedule_version: u64,
    pub algorithm: String,
    pub result: ReviewResult,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Override {
    pub id: String,
    pub event_id: String,
    pub correct: bool,
    pub before_card: Card,
    pub after_card: Card,
    pub created_ms: i64,
}

/// A deliberate scheduler reset is its own immutable transition, never an
/// edited review or a fabricated attempt. The cause is preserved as feedback.
#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Reset {
    pub id: String,
    pub question_id: String,
    pub concept_id: String,
    pub goal_id: String,
    pub source_revision: u64,
    pub content_version: u64,
    pub schedule_version_before: u64,
    pub schedule_version_after: u64,
    pub before_card: Card,
    pub after_card: Card,
    pub created_ms: i64,
    pub feedback_id: String,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Receipt {
    pub payload_hash: String,
    pub result: MutationResult,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq)]
pub struct MutationResult {
    pub redirect: String,
    pub entity_id: Option<String>,
    pub message: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Job {
    pub id: String,
    pub goal_id: String,
    pub question_id: Option<String>,
    pub question_version: Option<u64>,
    pub source_revision: u64,
    pub kind: String,
    pub status: String,
    pub created_ms: i64,
    pub lease: Option<String>,
    pub started_ms: Option<i64>,
    pub attempt: u8,
    pub error: Option<String>,
    pub raw_response: Option<String>,
    pub model: Option<String>,
    pub feedback_ids: Vec<String>,
    pub rejected: Vec<String>,
    pub candidate_json: Option<String>,
    pub critic_json: Option<String>,
    /// Complete independently judged batteries; raw provider attempts remain separate.
    #[serde(default)]
    pub critic_cache_json: Option<String>,
    #[serde(default)]
    pub critic_model: Option<String>,
    #[serde(default)]
    pub critic_parent_id: Option<String>,
    #[serde(default)]
    pub critic_inherited_keys: Vec<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Assessment {
    pub id: String,
    pub occurrence_id: String,
    pub question_id: String,
    pub content_version: u64,
    pub schedule_version: u64,
    pub policy: String,
    pub answer: String,
    pub status: String,
    pub lease: Option<String>,
    pub started_ms: Option<i64>,
    pub raw_response: Option<String>,
    pub error: Option<String>,
    pub created_ms: i64,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Spend {
    pub id: String,
    pub work_id: String,
    pub created_ms: i64,
    pub reserved_micros: u64,
    pub cost_micros: Option<u64>,
    pub status: String,
    pub model: String,
    #[serde(default)]
    pub response_hash: Option<String>,
    #[serde(default)]
    pub response_model: Option<String>,
    /// Exact bounded provider bytes, or the bounded prefix of oversized data.
    #[serde(default)]
    pub raw_response: Option<String>,
    #[serde(default)]
    pub response_bytes: Option<usize>,
    #[serde(default)]
    pub response_outcome: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Feedback {
    pub id: String,
    pub goal_id: String,
    pub concept_id: Option<String>,
    pub question_id: Option<String>,
    pub kind: String,
    pub text: String,
    pub created_ms: i64,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
pub struct Preferences {
    pub pace: String,
}
impl Default for Preferences {
    fn default() -> Self {
        Self {
            pace: "steady".into(),
        }
    }
}

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq)]
pub struct BackupStatus {
    pub completed_ms: Option<i64>,
    pub key: Option<String>,
    pub sha256: Option<String>,
    pub error: Option<String>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct AppError {
    pub status: u16,
    pub message: String,
}
impl AppError {
    pub fn new(status: u16, message: impl Into<String>) -> Self {
        Self {
            status,
            message: message.into(),
        }
    }
    pub fn conflict() -> Self {
        Self::new(
            409,
            "This changed in another tab. Reload to continue from your saved progress.",
        )
    }
}
pub type AppResult<T> = Result<T, AppError>;
