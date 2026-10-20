//! Read-only cost metadata. Allowance enforcement remains App::spent.
use crate::model::{App, AppError, AppResult};
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

#[derive(Clone, Copy, Debug, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum SpendBy {
    Day,
    Purpose,
    Model,
}
impl SpendBy {
    pub fn parse(value: &str) -> AppResult<Self> {
        match value {
            "day" => Ok(Self::Day),
            "purpose" => Ok(Self::Purpose),
            "model" => Ok(Self::Model),
            _ => Err(AppError::new(422, "Use by=day|purpose|model.")),
        }
    }
}

#[derive(Default, Serialize, Deserialize)]
pub struct SpendTotals {
    pub entries: usize,
    pub unknown_entries: usize,
    pub reserved_micros: u64,
    pub known_cost_micros: u64,
    pub unknown_held_micros: u64,
}
impl SpendTotals {
    fn include(&mut self, entry: &SpendEntry) -> AppResult<()> {
        let add = |a: u64, b| {
            a.checked_add(b)
                .ok_or_else(|| AppError::new(422, "Spend total exceeds the reporting range."))
        };
        self.entries += 1;
        self.unknown_entries += usize::from(entry.cost_micros.is_none());
        self.reserved_micros = add(self.reserved_micros, entry.reserved_micros)?;
        self.known_cost_micros = add(self.known_cost_micros, entry.cost_micros.unwrap_or(0))?;
        self.unknown_held_micros = add(self.unknown_held_micros, entry.unknown_held_micros)?;
        Ok(())
    }
}

#[derive(Serialize, Deserialize)]
pub struct SpendEntry {
    pub id: String,
    pub work_id: String,
    pub work_kind: String,
    pub goal_id: Option<String>,
    pub created_ms: i64,
    pub day: String,
    pub purpose: String,
    pub model: String,
    pub status: String,
    pub reserved_micros: u64,
    pub cost_micros: Option<u64>,
    pub unknown_held_micros: u64,
}
#[derive(Serialize, Deserialize)]
pub struct SpendGroup {
    pub key: String,
    pub totals: SpendTotals,
}
#[derive(Serialize, Deserialize)]
pub struct SpendReport {
    pub by: SpendBy,
    pub currency: String,
    pub day_basis: String,
    pub totals: SpendTotals,
    pub groups: Vec<SpendGroup>,
    pub entries: Vec<SpendEntry>,
}

pub fn report(app: &App, by: SpendBy) -> AppResult<SpendReport> {
    let mut totals = SpendTotals::default();
    let mut groups = BTreeMap::<String, SpendTotals>::new();
    let mut entries = Vec::with_capacity(app.spend.len());
    let attribution = |value: &Option<String>| {
        value
            .as_deref()
            .filter(|v| !v.is_empty())
            .unwrap_or("unattributed")
            .to_owned()
    };
    for spend in &app.spend {
        let date = chrono::DateTime::from_timestamp_millis(spend.created_ms)
            .ok_or_else(|| AppError::new(422, "Spend timestamp is out of range."))?;
        let job = app.jobs.get(&spend.work_id);
        let assessment = app.assessments.get(&spend.work_id);
        let entry = SpendEntry {
            id: spend.id.clone(),
            work_id: spend.work_id.clone(),
            work_kind: if job.is_some() {
                "concept_job"
            } else if assessment.is_some() {
                "assessment"
            } else {
                "unattributed"
            }
            .into(),
            goal_id: job.map(|j| j.goal_id.clone()).or_else(|| {
                assessment
                    .and_then(|a| app.questions.get(&a.question_id))
                    .map(|q| q.goal_id.clone())
            }),
            created_ms: spend.created_ms,
            day: date.format("%Y-%m-%d").to_string(),
            purpose: attribution(&spend.purpose),
            model: attribution(&spend.model),
            status: spend.status.clone(),
            reserved_micros: spend.reserved_micros,
            cost_micros: spend.cost_micros,
            unknown_held_micros: if spend.cost_micros.is_none() {
                spend.reserved_micros
            } else {
                0
            },
        };
        let key = match by {
            SpendBy::Day => &entry.day,
            SpendBy::Purpose => &entry.purpose,
            SpendBy::Model => &entry.model,
        };
        groups.entry(key.clone()).or_default().include(&entry)?;
        totals.include(&entry)?;
        entries.push(entry);
    }
    Ok(SpendReport {
        by,
        currency: "USD".into(),
        day_basis: "UTC".into(),
        totals,
        groups: groups
            .into_iter()
            .map(|(key, totals)| SpendGroup { key, totals })
            .collect(),
        entries,
    })
}
