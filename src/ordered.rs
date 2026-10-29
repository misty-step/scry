//! Deterministic identity-and-order recall. Authored aliases are not fuzzy matches.
use crate::{
    grading::{self, Authority, Grade, UnitVerdict, Verdict},
    library::{Kind, Version},
    model::{AppError, AppResult, TEXT_LIMIT},
};
use serde::{Deserialize, Serialize};
use std::{
    collections::{BTreeMap, BTreeSet},
    ops::Range,
};

pub const AUTHORITY: &str = "ordered-v1";
pub const GRADER_VERSION: &str = "1;unicode=17.0.0;lines=1";
pub const MAX_ITEMS: usize = 128;
const MAX_ALIASES: usize = 16;
const MAX_NAME_BYTES: usize = 512;

pub fn name_key(value: &str) -> String {
    grading::mapping_words(value).join(" ")
}

pub fn validate_names(units: &[&str], aliases: &BTreeMap<usize, Vec<String>>) -> AppResult<()> {
    if units.is_empty() || units.len() > MAX_ITEMS || aliases.keys().any(|i| *i >= units.len()) {
        return Err(AppError::new(
            422,
            "An ordered exercise needs 1–128 items and valid alias positions.",
        ));
    }
    let mut known = BTreeMap::new();
    for (index, text) in units.iter().enumerate() {
        let names = aliases.get(&index);
        if names.is_some_and(|a| a.is_empty() || a.len() > MAX_ALIASES) {
            return Err(AppError::new(422, "Use 1–16 aliases for an authored item."));
        }
        for raw in std::iter::once(*text).chain(names.into_iter().flatten().map(String::as_str)) {
            let value = raw.trim();
            let key = name_key(value);
            if value.len() > MAX_NAME_BYTES || value.chars().any(char::is_control) || key.is_empty()
            {
                return Err(AppError::new(
                    422,
                    "Ordered item names must be single, nonempty lines of at most 512 bytes.",
                ));
            }
            if known
                .insert(key, index)
                .is_some_and(|previous| previous != index)
            {
                return Err(AppError::new(
                    422,
                    "An ordered item name or alias identifies more than one item.",
                ));
            }
        }
    }
    Ok(())
}

pub fn validate(version: &Version) -> AppResult<()> {
    if version.kind != Kind::OrderedList {
        return Err(AppError::new(
            422,
            "This exercise requires an ordered list.",
        ));
    }
    let units = version
        .sections
        .iter()
        .flat_map(|s| &s.units)
        .map(|u| u.text.as_str())
        .collect::<Vec<_>>();
    validate_names(&units, &version.ordered_aliases)
}

#[derive(Clone, Copy, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum ItemStatus {
    Correct,
    Misplaced,
    Missing,
    Repeated,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
pub struct ItemFeedback {
    /// Absolute zero-based Unit index in the immutable Version.
    pub unit_index: usize,
    pub status: ItemStatus,
    /// One-based submitted nonempty line positions. More than one is a repeat.
    pub answered_positions: Vec<usize>,
}
#[derive(Clone, Copy, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum ExtraKind {
    Unknown,
    Repeated,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
pub struct ExtraFeedback {
    pub position: usize,
    pub kind: ExtraKind,
}
#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
pub struct OrderedFeedback {
    pub items: Vec<ItemFeedback>,
    pub extras: Vec<ExtraFeedback>,
    pub recalled: usize,
    pub in_order: usize,
    pub complete: bool,
}

/// Omission does not shift every later item into an error. Compare the order of
/// recognized unique identities against those same identities in canonical order.
/// Swapped identities are misplaced; repeats are separately diagnosed. Unknown
/// lines never become accepted identities and make the complete response fail.
pub fn feedback(
    version: &Version,
    range: Range<usize>,
    answer: &str,
) -> AppResult<OrderedFeedback> {
    validate(version)?;
    let units = version
        .sections
        .iter()
        .flat_map(|s| &s.units)
        .collect::<Vec<_>>();
    if range.is_empty() || range.end > units.len() || answer.len() > TEXT_LIMIT {
        return Err(AppError::new(
            422,
            "Choose a valid ordered scope and an answer of at most 32768 bytes.",
        ));
    }
    let mut identities = BTreeMap::new();
    for index in range.clone() {
        identities.insert(name_key(&units[index].text), index);
        for alias in version.ordered_aliases.get(&index).into_iter().flatten() {
            identities.insert(name_key(alias), index);
        }
    }
    let lines = answer
        .lines()
        .filter(|line| !line.trim().is_empty())
        .collect::<Vec<_>>();
    if lines.len() > MAX_ITEMS * 2 {
        return Err(AppError::new(422, "Use at most 256 response lines."));
    }
    let mut positions: BTreeMap<usize, Vec<usize>> = BTreeMap::new();
    let mut submitted = Vec::new();
    let mut extras = Vec::new();
    for (position, line) in lines.iter().enumerate() {
        let position = position + 1;
        match identities.get(&name_key(line)) {
            Some(index) => {
                let at = positions.entry(*index).or_default();
                if at.is_empty() {
                    submitted.push(*index);
                } else {
                    extras.push(ExtraFeedback {
                        position,
                        kind: ExtraKind::Repeated,
                    });
                }
                at.push(position);
            }
            None => extras.push(ExtraFeedback {
                position,
                kind: ExtraKind::Unknown,
            }),
        }
    }
    let canonical = positions.keys().copied().collect::<Vec<_>>();
    let ordered = submitted
        .iter()
        .zip(canonical.iter())
        .filter_map(|(actual, expected)| (actual == expected).then_some(*actual))
        .collect::<BTreeSet<_>>();
    let items = range
        .map(|unit_index| {
            let answered_positions = positions.remove(&unit_index).unwrap_or_default();
            let status = match answered_positions.len() {
                0 => ItemStatus::Missing,
                1 if ordered.contains(&unit_index) => ItemStatus::Correct,
                1 => ItemStatus::Misplaced,
                _ => ItemStatus::Repeated,
            };
            ItemFeedback {
                unit_index,
                status,
                answered_positions,
            }
        })
        .collect::<Vec<_>>();
    let recalled = items
        .iter()
        .filter(|item| item.status != ItemStatus::Missing)
        .count();
    let in_order = items
        .iter()
        .filter(|item| item.status == ItemStatus::Correct)
        .count();
    let complete = in_order == items.len() && extras.is_empty();
    Ok(OrderedFeedback {
        items,
        extras,
        recalled,
        in_order,
        complete,
    })
}

pub fn grade(version: &Version, range: Range<usize>, answer: &str) -> AppResult<Grade> {
    let evidence = feedback(version, range, answer)?;
    Ok(Grade {
        authority: Authority {
            name: AUTHORITY.into(),
            grader_version: GRADER_VERSION.into(),
        },
        verdict: if evidence.complete {
            Verdict::Good
        } else {
            Verdict::Again
        },
        units: evidence
            .items
            .iter()
            .map(|item| UnitVerdict {
                unit_index: item.unit_index,
                verdict: if item.status == ItemStatus::Correct {
                    Verdict::Good
                } else {
                    Verdict::Again
                },
            })
            .collect(),
        // Item-level feedback is deliberately not a fabricated word-level diff.
        diff: Vec::new(),
        wrong_version_hint: None,
    })
}
