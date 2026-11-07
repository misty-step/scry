//! Frozen text-boundary-repair-v1 dependency closure, copied from #466.
//! Rust 1.98.1 supplies char::is_numeric/is_whitespace and trim semantics.
//! Do not edit this policy when normal Library/grading evolves. Version changes
//! require explicit reviewed migration policy, never automatic extra copies.
#![allow(dead_code)]
use crate::{library::SectionInput, model::*};
use std::ops::Range;
mod unicode;
mod unicode_tables;
struct Word {
    text: String,
    span: Range<usize>,
}
fn normalized_words(text: &str) -> Vec<String> {
    unicode::tokens(text)
        .into_iter()
        .map(|t| t.normalized)
        .collect()
}
fn text_word_spans(text: &str) -> Vec<Range<usize>> {
    unicode::tokens(text)
        .into_iter()
        .map(|t| t.word.span)
        .collect()
}
fn segment(text: &str) -> Vec<SectionInput> {
    let mut sections: Vec<SectionInput> = Vec::new();
    for line in text.split_inclusive('\n') {
        if line.trim().is_empty()
            && let Some(last) = sections.last_mut()
        {
            last.units.last_mut().unwrap().push_str(line);
            continue;
        }
        let mut units: Vec<String> = Vec::new();
        for clause in line.split_inclusive([',', ';', ':', '.', '!', '?', '\n']) {
            if clause.trim().is_empty()
                && let Some(last) = units.last_mut()
            {
                last.push_str(clause);
            } else {
                units.push(clause.into());
            }
        }
        sections.push(SectionInput {
            heading: format!("Section {}", sections.len() + 1),
            units,
        });
    }
    sections
}
// Versioned repair partition: retain this implementation when normal admission evolves.
pub fn segments(text: &str) -> Vec<SectionInput> {
    let mut sections: Vec<SectionInput> = Vec::new();
    let mut prefix = String::new();
    // A punctuation cut can lie inside one exact-v2 word (3.14, a,b, ...).
    // Inspect the whole source once, rather than repeatedly normalizing an
    // ever-growing Unit for punctuation-heavy input.
    let spans = text_word_spans(text);
    let mut words = spans.iter().peekable();
    let mut offset = 0;
    for section in segment(text) {
        let mut units: Vec<String> = Vec::new();
        for unit in section.units {
            while words.peek().is_some_and(|span| span.end <= offset) {
                words.next();
            }
            let splits_word = words.peek().is_some_and(|span| span.start < offset);
            offset += unit.len();
            if splits_word && let Some(last) = units.last_mut() {
                last.push_str(&unit);
            } else if normalized_words(&unit).is_empty() {
                if let Some(last) = units.last_mut() {
                    last.push_str(&unit);
                } else {
                    prefix.push_str(&unit);
                }
            } else {
                units.push(std::mem::take(&mut prefix) + &unit);
            }
        }
        if units.is_empty() {
            if let Some(last) = sections.last_mut() {
                last.units.last_mut().unwrap().push_str(&prefix);
                prefix.clear();
            }
        } else {
            sections.push(SectionInput {
                heading: format!("Section {}", sections.len() + 1),
                units,
            });
        }
    }
    sections
}
/// New practice requires nonempty Units whose concatenation preserves the
/// pinned normalized words. Keep this separate from historical validation.
pub fn check_units<'a>(units: impl Iterator<Item = &'a str>) -> AppResult<()> {
    let mut joined = String::new();
    let mut expected = Vec::new();
    for text in units {
        let words = normalized_words(text);
        if words.is_empty() {
            return Err(AppError::new(
                422,
                "A text Unit has no words to recall. Supply boundaries that include words; existing content and history are unchanged.",
            ));
        }
        joined.push_str(text);
        expected.extend(words);
    }
    // Flatten every Section too: an authored Section boundary cannot turn a
    // single native word into two independently graded words.
    if expected != normalized_words(&joined) {
        return Err(AppError::new(
            422,
            "Text Unit boundaries split a word or change its normalization. Supply boundaries between complete words; existing content and history are unchanged.",
        ));
    }
    Ok(())
}

// Cargo build.rs and the native prototype runner verify the actual selected
// compiler before setting this value. Direct unverified library builds fail.
const _: () = {
    let actual = env!("SCRY_REPAIR_POLICY_RUSTC").as_bytes();
    let expected = b"1.98.1";
    assert!(actual.len() == expected.len());
    let mut i = 0;
    while i < expected.len() {
        assert!(actual[i] == expected[i]);
        i += 1;
    }
};
