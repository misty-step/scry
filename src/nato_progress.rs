//! Read-only evidence for the canonical NATO alphabet. No mastery prediction.
use crate::{grading::Verdict, library, model::App, study};
use std::collections::{BTreeMap, BTreeSet};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Outcome {
    Correct,
    Miss,
    Assisted,
}
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct ReviewMark {
    pub outcome: Outcome,
    pub created_ms: i64,
}
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct LetterProgress {
    pub letter: char,
    /// Completed answers, never reveal-only records.
    pub reviews: usize,
    pub correct: usize,
    pub unassisted: usize,
    pub assisted: usize,
    pub reveals: usize,
    pub last_answered_ms: Option<i64>,
    /// Scheduler recommendation, not the next position in A–Z practice.
    pub due_ms: Option<i64>,
    /// At most eight completed answers, oldest to newest. No answer text.
    pub recent: Vec<ReviewMark>,
}
impl LetterProgress {
    pub fn accuracy_percent(&self) -> Option<usize> {
        (self.unassisted > 0).then(|| (self.correct * 100 + self.unassisted / 2) / self.unassisted)
    }
}
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Progress {
    pub letters: Vec<LetterProgress>,
    pub reviews: usize,
    pub correct: usize,
    pub unassisted: usize,
    pub assisted: usize,
    pub practiced: usize,
}
impl Progress {
    pub fn accuracy_percent(&self) -> Option<usize> {
        (self.unassisted > 0).then(|| (self.correct * 100 + self.unassisted / 2) / self.unassisted)
    }
}

pub fn summarize(app: &App) -> Progress {
    let versions = app
        .library
        .versions
        .values()
        .filter(|v| library::is_nato(v))
        .map(|v| (v.id.as_str(), v))
        .collect::<BTreeMap<_, _>>();
    let mut letters = (b'A'..=b'Z')
        .map(|letter| LetterProgress {
            letter: char::from(letter),
            reviews: 0,
            correct: 0,
            unassisted: 0,
            assisted: 0,
            reveals: 0,
            last_answered_ms: None,
            due_ms: None,
            recent: Vec::new(),
        })
        .collect::<Vec<_>>();
    // Unit identity, not an editable title or cue string, attributes the evidence.
    let mut attempts = app
        .practice
        .attempts
        .values()
        .filter_map(|a| {
            let v = versions.get(a.version_id.as_str())?;
            if a.prompt.unit_ids.len() != 1 {
                return None;
            }
            let index = v
                .sections
                .iter()
                .flat_map(|s| &s.units)
                .position(|u| u.id == a.prompt.unit_ids[0])?;
            Some((index, a))
        })
        .collect::<Vec<_>>();
    attempts.sort_by_key(|(_, a)| (a.created_ms, a.sequence, a.id.as_str()));
    let mut answered = BTreeSet::new();
    let mut revealed = BTreeSet::new();
    for (index, a) in attempts {
        let row = &mut letters[index];
        if a.disclosure {
            if revealed.insert(a.prompt.id.as_str()) {
                row.reveals += 1;
            }
            continue;
        }
        if !answered.insert(a.prompt.id.as_str()) {
            continue;
        }
        row.reviews += 1;
        row.last_answered_ms = Some(a.created_ms);
        let outcome = if a.assisted {
            row.assisted += 1;
            Outcome::Assisted
        } else {
            row.unassisted += 1;
            if a.grade.verdict == Verdict::Good {
                row.correct += 1;
                Outcome::Correct
            } else {
                Outcome::Miss
            }
        };
        row.recent.push(ReviewMark {
            outcome,
            created_ms: a.created_ms,
        });
        if row.recent.len() > 8 {
            row.recent.remove(0);
        }
    }
    // Prefer the held active Study. A retained paused/dropped Study cannot
    // promise a next review; otherwise choose the newest active NATO Study.
    let active = |s: &&study::Study| {
        s.status == study::Status::Active && versions.contains_key(s.version_id.as_str())
    };
    let held = app
        .practice
        .current
        .as_ref()
        .and_then(|p| app.practice.studies.get(&p.prompt.study_id))
        .filter(active);
    let selected = held.or_else(|| {
        app.practice
            .studies
            .values()
            .filter(active)
            .max_by_key(|s| (s.created_ms, s.id.as_str()))
    });
    if let Some(s) = selected {
        let v = versions[s.version_id.as_str()];
        for card in app.practice.cards.values().filter(|c| c.study_id == s.id) {
            if card.state.schedule.last_review_ms.is_none() {
                continue;
            }
            if let Some(index) = v
                .sections
                .iter()
                .flat_map(|section| &section.units)
                .position(|u| u.id == card.unit_id)
            {
                letters[index].due_ms = Some(card.state.schedule.due_ms);
            }
        }
    }
    Progress {
        reviews: letters.iter().map(|r| r.reviews).sum(),
        correct: letters.iter().map(|r| r.correct).sum(),
        unassisted: letters.iter().map(|r| r.unassisted).sum(),
        assisted: letters.iter().map(|r| r.assisted).sum(),
        practiced: letters.iter().filter(|r| r.unassisted > 0).count(),
        letters,
    }
}
