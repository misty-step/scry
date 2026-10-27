use super::{Word, unicode_tables as data};
use std::ops::Range;

#[derive(Clone)]
struct Scalar {
    value: char,
    span: Range<usize>,
}

pub(super) struct Token {
    pub normalized: String,
    pub word: Word,
}

fn mapping(table: &[(char, &'static str)], value: char) -> Option<&'static str> {
    table
        .binary_search_by_key(&value, |entry| entry.0)
        .ok()
        .map(|i| table[i].1)
}

fn in_ranges(table: &[(char, char)], value: char) -> bool {
    let i = table.partition_point(|entry| entry.0 <= value);
    i > 0 && value <= table[i - 1].1
}

pub(super) fn is_letter(value: char) -> bool {
    in_ranges(data::LETTER, value)
}

fn combining_class(value: char) -> u8 {
    data::COMBINING
        .binary_search_by_key(&value, |entry| entry.0)
        .map(|i| data::COMBINING[i].1)
        .unwrap_or(0)
}

fn compose(a: char, b: char) -> Option<char> {
    let (a_code, b_code) = (u32::from(a), u32::from(b));
    if (0x1100..0x1113).contains(&a_code) && (0x1161..0x1176).contains(&b_code) {
        return char::from_u32(0xAC00 + (a_code - 0x1100) * 588 + (b_code - 0x1161) * 28);
    }
    if (0xAC00..0xD7A4).contains(&a_code)
        && (a_code - 0xAC00).is_multiple_of(28)
        && (0x11A8..0x11C3).contains(&b_code)
    {
        return char::from_u32(a_code + b_code - 0x11A7);
    }
    data::COMPOSITION
        .binary_search_by_key(&(a, b), |entry| entry.0)
        .ok()
        .map(|i| data::COMPOSITION[i].1)
}

fn nfkc(text: &str) -> Vec<Scalar> {
    let mut decomposed = Vec::new();
    for (start, value) in text.char_indices() {
        let span = start..start + value.len_utf8();
        let code = u32::from(value);
        if (0xAC00..0xD7A4).contains(&code) {
            let syllable = code - 0xAC00;
            let mut parts = vec![0x1100 + syllable / 588, 0x1161 + (syllable % 588) / 28];
            if syllable % 28 != 0 {
                parts.push(0x11A7 + syllable % 28);
            }
            decomposed.extend(parts.into_iter().map(|part| Scalar {
                value: char::from_u32(part).expect("Hangul scalar"),
                span: span.clone(),
            }));
        } else if let Some(parts) = mapping(data::DECOMPOSITION, value) {
            decomposed.extend(parts.chars().map(|value| Scalar {
                value,
                span: span.clone(),
            }));
        } else {
            decomposed.push(Scalar { value, span });
        }
    }

    let mut segment = 0;
    for i in 0..decomposed.len() {
        if combining_class(decomposed[i].value) == 0 {
            decomposed[segment..i].sort_by_key(|scalar| combining_class(scalar.value));
            segment = i + 1;
        }
    }
    decomposed[segment..].sort_by_key(|scalar| combining_class(scalar.value));

    let mut composed: Vec<Scalar> = Vec::with_capacity(decomposed.len());
    let mut starter: Option<usize> = None;
    let mut previous_class = 0;
    for scalar in decomposed {
        let class = combining_class(scalar.value);
        if let Some(i) = starter
            && (previous_class == 0 || previous_class < class)
            && let Some(value) = compose(composed[i].value, scalar.value)
        {
            composed[i].value = value;
            composed[i].span.start = composed[i].span.start.min(scalar.span.start);
            composed[i].span.end = composed[i].span.end.max(scalar.span.end);
            continue;
        }
        if class == 0 {
            starter = Some(composed.len());
        }
        previous_class = class;
        composed.push(scalar);
    }
    composed
}

/// NFKC, default full case folding, then the SPEC's word rules. Dashes separate
/// words; other punctuation is stripped. Apostrophes survive only inside words.
/// Ampersands stand for "and". Symbols/digits/diacritics are not discarded.
pub(super) fn tokens(text: &str) -> Vec<Token> {
    tokens_with_policy(text, false)
}

/// Mapping profile: punctuation creates boundaries, never silently joins
/// letters. Keep the historical text tokenizer's Unicode and span mechanics.
pub(super) fn mapping_tokens(text: &str) -> Vec<Token> {
    tokens_with_policy(text, true)
}
fn tokens_with_policy(text: &str, mapping_profile: bool) -> Vec<Token> {
    let mut folded = Vec::new();
    for scalar in nfkc(text) {
        if let Some(parts) = mapping(data::CASE_FOLD, scalar.value) {
            folded.extend(parts.chars().map(|value| Scalar {
                value,
                span: scalar.span.clone(),
            }));
        } else {
            folded.push(scalar);
        }
    }
    for scalar in &mut folded {
        scalar.value = match scalar.value {
            '\u{2018}' | '\u{2019}' | '\u{201a}' | '\u{201b}' | '\u{02bc}' | '\u{02bb}' | '`' => {
                '\''
            }
            value if in_ranges(data::DASH, value) || value == '\u{2212}' => ' ',
            value => value,
        };
    }

    let mut result = Vec::new();
    let mut normalized = String::new();
    let mut span: Option<Range<usize>> = None;
    for (i, scalar) in folded.iter().enumerate() {
        let value = scalar.value;
        if value.is_whitespace() || (!mapping_profile && value == '&') {
            finish_token(text, &mut normalized, &mut span, &mut result);
            if value == '&' {
                result.push(Token {
                    normalized: "and".into(),
                    word: Word {
                        text: text[scalar.span.clone()].into(),
                        span: scalar.span.clone(),
                    },
                });
            }
            continue;
        }
        let inside_word = value == '\''
            && i > 0
            && i + 1 < folded.len()
            && word_character(folded[i - 1].value)
            && word_character(folded[i + 1].value);
        if (in_ranges(data::PUNCTUATION, value) || value == '\'') && !inside_word {
            if mapping_profile {
                finish_token(text, &mut normalized, &mut span, &mut result);
            }
            continue;
        }
        normalized.push(value);
        match &mut span {
            Some(span) => {
                span.start = span.start.min(scalar.span.start);
                span.end = span.end.max(scalar.span.end);
            }
            None => span = Some(scalar.span.clone()),
        }
    }
    finish_token(text, &mut normalized, &mut span, &mut result);
    result
}

fn word_character(value: char) -> bool {
    is_letter(value) || in_ranges(data::MARK, value) || value.is_numeric()
}

fn finish_token(
    text: &str,
    normalized: &mut String,
    span: &mut Option<Range<usize>>,
    result: &mut Vec<Token>,
) {
    if let Some(span) = span.take() {
        result.push(Token {
            normalized: std::mem::take(normalized),
            word: Word {
                text: text[span.clone()].into(),
                span,
            },
        });
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sha2::{Digest, Sha256};

    #[test]
    #[ignore = "requires pinned Unicode 17 NormalizationTest.txt and CaseFolding.txt; see module README"]
    fn unicode_17_conformance() {
        let directory = std::env::var("SCRY_UNICODE_DATA").expect("set SCRY_UNICODE_DATA");
        let normalization = std::fs::read(format!("{directory}/NormalizationTest.txt")).unwrap();
        assert_eq!(
            hex::encode(Sha256::digest(&normalization)),
            "5019ffd530751a741900c849c0e010332f142a3612234639bd200b82138a87db"
        );
        let mut count = 0;
        for line in std::str::from_utf8(&normalization).unwrap().lines() {
            let line = line.split('#').next().unwrap().trim();
            if line.is_empty() || line.starts_with('@') {
                continue;
            }
            let columns: Vec<String> = line
                .split(';')
                .take(5)
                .map(|column| {
                    column
                        .split_whitespace()
                        .map(|part| char::from_u32(u32::from_str_radix(part, 16).unwrap()).unwrap())
                        .collect()
                })
                .collect();
            for column in &columns {
                let normalized: String = nfkc(column).iter().map(|s| s.value).collect();
                assert_eq!(normalized, columns[3], "UAX #15: {line}");
            }
            count += 1;
        }
        println!("Unicode 17 NFKC: {count} rows, all five columns conform");

        let folds = std::fs::read(format!("{directory}/CaseFolding.txt")).unwrap();
        assert_eq!(
            hex::encode(Sha256::digest(&folds)),
            "ff8d8fefbf123574205085d6714c36149eb946d717a0c585c27f0f4ef58c4183"
        );
        for line in std::str::from_utf8(&folds).unwrap().lines() {
            let columns: Vec<_> = line.split('#').next().unwrap().split(';').collect();
            if columns.len() < 3 || !matches!(columns[1].trim(), "C" | "F") {
                continue;
            }
            let value =
                char::from_u32(u32::from_str_radix(columns[0].trim(), 16).unwrap()).unwrap();
            let expected: String = columns[2]
                .split_whitespace()
                .map(|part| char::from_u32(u32::from_str_radix(part, 16).unwrap()).unwrap())
                .collect();
            assert_eq!(mapping(data::CASE_FOLD, value), Some(expected.as_str()));
        }
        println!("Unicode 17 default full case-fold mappings conform");
    }
}
