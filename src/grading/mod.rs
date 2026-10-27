//! Pure `exact-v2` word grading. No application state or effects enter this seam.
//! See README.md for range, Diff span, alignment tie and sibling conventions.

use serde::{Deserialize, Serialize};
use std::{collections::BTreeSet, ops::Range};

mod unicode;
mod unicode_tables;

/// Shared pinned Unicode word normalization, without grading or Slip tolerance.
pub fn normalized_words(text: &str) -> Vec<String> {
    unicode::tokens(text)
        .into_iter()
        .map(|token| token.normalized)
        .collect()
}

/// Version-pinned mapping normalization for key identity and answer feedback.
pub fn mapping_words(text: &str) -> Vec<String> {
    unicode::mapping_tokens(text)
        .into_iter()
        .map(|token| token.normalized)
        .collect()
}
pub const MAPPING_GRADER_VERSION: &str = "1;unicode=17.0.0;mapping=1";

pub const AUTHORITY: &str = "exact-v2";
pub const GRADER_VERSION: &str = "1;unicode=17.0.0";

/// All Units of the studied Version, including Units outside this Prompt.
#[derive(Clone, Copy, Debug)]
pub struct Version<'a> {
    pub units: &'a [&'a str],
    pub spelling_strict: bool,
}

/// A sibling of the same Work. The caller supplies the text corresponding to
/// this Prompt, which may have different Unit boundaries from the studied text.
#[derive(Clone, Copy, Debug)]
pub struct SiblingVersion<'a> {
    pub id: &'a str,
    pub name: &'a str,
    pub expected: &'a str,
}

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub enum Verdict {
    Good,
    Again,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub struct Authority {
    pub name: String,
    pub grader_version: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub struct UnitVerdict {
    pub unit_index: usize,
    pub verdict: Verdict,
}

/// Original text and UTF-8 byte span. Expected spans are relative to their Unit;
/// typed spans are relative to the answer. Normalization never rewrites either.
#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub struct Word {
    pub text: String,
    pub span: Range<usize>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub enum Change {
    Correct {
        expected: Word,
        typed: Word,
    },
    Slip {
        expected: Word,
        typed: Word,
    },
    Wrong {
        expected: Word,
        typed: Word,
        out_of_order: bool,
    },
    Missing {
        expected: Word,
    },
    Extra {
        typed: Word,
    },
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub struct Diff {
    pub unit_index: usize,
    #[serde(flatten)]
    pub change: Change,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub struct WrongVersionHint {
    pub id: String,
    pub name: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
pub struct Grade {
    pub authority: Authority,
    pub verdict: Verdict,
    pub units: Vec<UnitVerdict>,
    pub diff: Vec<Diff>,
    pub wrong_version_hint: Option<WrongVersionHint>,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum GradeError {
    InvalidUnitRange,
}

impl std::fmt::Display for GradeError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str("select a nonempty range within the Version's Units")
    }
}

impl std::error::Error for GradeError {}

/// Grade a single Unit or contiguous Recitation. Unit indices always refer to
/// the full Version. No Attempts, schedules, history or model calls are made.
pub fn grade(
    version: Version<'_>,
    units: Range<usize>,
    answer: &str,
    siblings: &[SiblingVersion<'_>],
) -> Result<Grade, GradeError> {
    grade_with_policy(version, units, answer, siblings, false)
}

/// Same deterministic alignment and spelling policy, with punctuation as word
/// boundaries. Historical exact-v2 grades retain their original normalization.
pub fn grade_mapping(
    version: Version<'_>,
    units: Range<usize>,
    answer: &str,
) -> Result<Grade, GradeError> {
    grade_with_policy(version, units, answer, &[], true)
}
fn grade_with_policy(
    version: Version<'_>,
    units: Range<usize>,
    answer: &str,
    siblings: &[SiblingVersion<'_>],
    mapping: bool,
) -> Result<Grade, GradeError> {
    let tokens = if mapping {
        unicode::mapping_tokens
    } else {
        unicode::tokens
    };
    if units.start >= units.end || units.end > version.units.len() {
        return Err(GradeError::InvalidUnitRange);
    }
    let version_tokens: Vec<_> = version.units.iter().map(|text| tokens(text)).collect();
    let vocabulary: BTreeSet<_> = version_tokens
        .iter()
        .flatten()
        .map(|token| token.normalized.as_str())
        .collect();
    let expected: Vec<_> = units
        .clone()
        .flat_map(|unit| version_tokens[unit].iter().map(move |token| (unit, token)))
        .collect();
    let typed = tokens(answer);
    let expected_words: Vec<_> = expected
        .iter()
        .map(|(_, token)| token.normalized.as_str())
        .collect();
    let typed_words: Vec<_> = typed
        .iter()
        .map(|token| token.normalized.as_str())
        .collect();
    let alignment = align(&expected_words, &typed_words);
    let mut diff = Vec::new();
    let mut unit_verdicts: Vec<_> = units
        .clone()
        .map(|unit_index| UnitVerdict {
            unit_index,
            verdict: Verdict::Good,
        })
        .collect();

    for edit in alignment.edits {
        let (unit_index, change) = match edit {
            Edit::Pair {
                expected: i,
                typed: j,
                out_of_order,
            } => {
                let (unit, expected) = expected[i];
                let typed = &typed[j];
                let change = if out_of_order {
                    Change::Wrong {
                        expected: expected.word.clone(),
                        typed: typed.word.clone(),
                        out_of_order: true,
                    }
                } else if expected.normalized == typed.normalized {
                    Change::Correct {
                        expected: expected.word.clone(),
                        typed: typed.word.clone(),
                    }
                } else if !vocabulary.contains(typed.normalized.as_str())
                    && is_slip(&expected.normalized, &typed.normalized)
                {
                    Change::Slip {
                        expected: expected.word.clone(),
                        typed: typed.word.clone(),
                    }
                } else {
                    Change::Wrong {
                        expected: expected.word.clone(),
                        typed: typed.word.clone(),
                        out_of_order: false,
                    }
                };
                (unit, change)
            }
            Edit::Missing(i) => (
                expected[i].0,
                Change::Missing {
                    expected: expected[i].1.word.clone(),
                },
            ),
            Edit::Extra {
                typed: j,
                after_expected,
            } => {
                let unit = after_expected
                    .checked_sub(1)
                    .map(|i| expected[i].0)
                    .unwrap_or(units.start);
                (
                    unit,
                    Change::Extra {
                        typed: typed[j].word.clone(),
                    },
                )
            }
        };
        let correct = matches!(change, Change::Correct { .. })
            || (!version.spelling_strict && matches!(change, Change::Slip { .. }));
        if !correct {
            unit_verdicts[unit_index - units.start].verdict = Verdict::Again;
        }
        diff.push(Diff { unit_index, change });
    }
    let verdict = if unit_verdicts
        .iter()
        .all(|unit| unit.verdict == Verdict::Good)
    {
        Verdict::Good
    } else {
        Verdict::Again
    };
    let wrong_version_hint = if verdict == Verdict::Again {
        closer_sibling(siblings, &typed_words, alignment.distance)
    } else {
        None
    };
    Ok(Grade {
        authority: Authority {
            name: AUTHORITY.into(),
            grader_version: if mapping {
                MAPPING_GRADER_VERSION
            } else {
                GRADER_VERSION
            }
            .into(),
        },
        verdict,
        units: unit_verdicts,
        diff,
        wrong_version_hint,
    })
}

/// One edit must change a letter, not a digit, apostrophe, symbol or diacritic.
fn is_slip(expected: &str, typed: &str) -> bool {
    let expected: Vec<_> = expected.chars().collect();
    if expected
        .iter()
        .filter(|&&value| unicode::is_letter(value))
        .count()
        < 5
    {
        return false;
    }
    let typed: Vec<_> = typed.chars().collect();
    if expected.len().abs_diff(typed.len()) > 1 {
        return false;
    }
    let prefix = expected
        .iter()
        .zip(&typed)
        .take_while(|(a, b)| a == b)
        .count();
    match expected.len().cmp(&typed.len()) {
        std::cmp::Ordering::Equal => {
            if prefix == expected.len() {
                return false;
            }
            let e = &expected[prefix..];
            let t = &typed[prefix..];
            (unicode::is_letter(e[0]) && unicode::is_letter(t[0]) && e[1..] == t[1..])
                || (e.len() >= 2
                    && unicode::is_letter(e[0])
                    && unicode::is_letter(e[1])
                    && e[0] == t[1]
                    && e[1] == t[0]
                    && e[2..] == t[2..])
        }
        std::cmp::Ordering::Less => {
            unicode::is_letter(typed[prefix]) && expected[prefix..] == typed[prefix + 1..]
        }
        std::cmp::Ordering::Greater => {
            unicode::is_letter(expected[prefix]) && expected[prefix + 1..] == typed[prefix..]
        }
    }
}

#[derive(Clone, Copy)]
enum Step {
    Start,
    Pair,
    Transpose,
    Missing,
    Extra,
}

#[derive(Clone, Copy)]
struct Cell {
    distance: usize,
    step: Step,
}

enum Edit {
    Pair {
        expected: usize,
        typed: usize,
        out_of_order: bool,
    },
    Missing(usize),
    Extra {
        typed: usize,
        after_expected: usize,
    },
}

struct Alignment {
    distance: usize,
    edits: Vec<Edit>,
}

/// Word edit distance with adjacent transpositions and a deterministic backtrace.
/// On equal costs prefer a diagonal, then a transposition, missing, then extra.
fn align(expected: &[&str], typed: &[&str]) -> Alignment {
    let width = typed.len() + 1;
    let mut cells = vec![
        Cell {
            distance: 0,
            step: Step::Start
        };
        (expected.len() + 1) * width
    ];
    for i in 1..=expected.len() {
        cells[i * width] = Cell {
            distance: i,
            step: Step::Missing,
        };
    }
    for (j, cell) in cells.iter_mut().enumerate().take(width).skip(1) {
        *cell = Cell {
            distance: j,
            step: Step::Extra,
        };
    }
    for i in 1..=expected.len() {
        for j in 1..=typed.len() {
            let mut best = Cell {
                distance: cells[(i - 1) * width + j - 1].distance
                    + usize::from(expected[i - 1] != typed[j - 1]),
                step: Step::Pair,
            };
            let mut consider = |distance, step| {
                if distance < best.distance {
                    best = Cell { distance, step };
                }
            };
            if i > 1 && j > 1 && expected[i - 1] == typed[j - 2] && expected[i - 2] == typed[j - 1]
            {
                consider(cells[(i - 2) * width + j - 2].distance + 1, Step::Transpose);
            }
            consider(cells[(i - 1) * width + j].distance + 1, Step::Missing);
            consider(cells[i * width + j - 1].distance + 1, Step::Extra);
            cells[i * width + j] = best;
        }
    }
    let (mut i, mut j) = (expected.len(), typed.len());
    let distance = cells[i * width + j].distance;
    let mut edits = Vec::new();
    while i > 0 || j > 0 {
        match cells[i * width + j].step {
            Step::Pair => {
                edits.push(Edit::Pair {
                    expected: i - 1,
                    typed: j - 1,
                    out_of_order: false,
                });
                i -= 1;
                j -= 1;
            }
            Step::Transpose => {
                edits.push(Edit::Pair {
                    expected: i - 1,
                    typed: j - 1,
                    out_of_order: true,
                });
                edits.push(Edit::Pair {
                    expected: i - 2,
                    typed: j - 2,
                    out_of_order: true,
                });
                i -= 2;
                j -= 2;
            }
            Step::Missing => {
                edits.push(Edit::Missing(i - 1));
                i -= 1;
            }
            Step::Extra => {
                edits.push(Edit::Extra {
                    typed: j - 1,
                    after_expected: i,
                });
                j -= 1;
            }
            Step::Start => unreachable!("only the origin has no predecessor"),
        }
    }
    edits.reverse();
    Alignment { distance, edits }
}

fn closer_sibling(
    siblings: &[SiblingVersion<'_>],
    typed: &[&str],
    studied_distance: usize,
) -> Option<WrongVersionHint> {
    siblings
        .iter()
        .filter_map(|sibling| {
            let tokens = unicode::tokens(sibling.expected);
            let expected: Vec<_> = tokens
                .iter()
                .map(|token| token.normalized.as_str())
                .collect();
            let distance = align(&expected, typed).distance;
            (distance < studied_distance).then_some((distance, sibling.id, sibling.name))
        })
        .min()
        .map(|(_, id, name)| WrongVersionHint {
            id: id.into(),
            name: name.into(),
        })
}
