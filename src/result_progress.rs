//! Read-only, same-scope evidence for a committed native answer. Not a mastery prediction.
use crate::{grading::Verdict, model::App, study};
use std::collections::BTreeSet;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Mark {
    Correct,
    Miss,
    Helped,
}
#[derive(Debug, PartialEq, Eq)]
pub struct Evidence {
    pub correct: usize,
    pub unaided: usize,
    pub helped: usize,
    pub recent: Vec<Mark>,
    pub next_due_ms: Option<i64>,
    pub unscheduled_parts: usize,
    pub active: bool,
}

pub fn summarize(app: &App, answer: &study::Attempt) -> Evidence {
    let mut history = app
        .practice
        .attempts
        .values()
        .filter(|a| {
            !a.disclosure
                && a.version_id == answer.version_id
                && a.version_hash == answer.version_hash
                && a.prompt.scope == answer.prompt.scope
                && a.prompt.unit_ids == answer.prompt.unit_ids
        })
        .collect::<Vec<_>>();
    history.sort_by_key(|a| (a.created_ms, a.sequence, a.id.as_str()));
    let mut seen = BTreeSet::new();
    let mut evidence = Evidence {
        correct: 0,
        unaided: 0,
        helped: 0,
        recent: Vec::new(),
        next_due_ms: None,
        unscheduled_parts: 0,
        active: false,
    };
    for a in history {
        if !seen.insert(&a.prompt.id) {
            continue;
        }
        let mark = if a.assisted {
            evidence.helped += 1;
            Mark::Helped
        } else {
            evidence.unaided += 1;
            if a.grade.verdict == Verdict::Good {
                evidence.correct += 1;
                Mark::Correct
            } else {
                Mark::Miss
            }
        };
        evidence.recent.push(mark);
    }
    evidence.recent = evidence
        .recent
        .into_iter()
        .rev()
        .take(5)
        .collect::<Vec<_>>()
        .into_iter()
        .rev()
        .collect();
    evidence.active = app
        .practice
        .studies
        .get(&answer.prompt.study_id)
        .is_some_and(|s| s.status != study::Status::Dropped);
    if evidence.active {
        // The current persisted cards, not the attempt snapshot: an owner correction
        // may have changed the schedule since this answer was committed.
        for effect in &answer.effects {
            match app
                .practice
                .cards
                .get(&effect.card_id)
                .filter(|c| c.study_id == answer.prompt.study_id && c.unit_id == effect.unit_id)
            {
                Some(card) if card.state.schedule.last_review_ms.is_some() => {
                    let due = card.state.schedule.due_ms;
                    evidence.next_due_ms =
                        Some(evidence.next_due_ms.map_or(due, |old| old.min(due)));
                }
                _ => evidence.unscheduled_parts += 1,
            }
        }
    }
    evidence
}
