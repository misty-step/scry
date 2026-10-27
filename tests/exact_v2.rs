use scry::grading::{
    self, AUTHORITY, Change, GRADER_VERSION, Grade, GradeError, SiblingVersion, Verdict, Version,
};

fn grade_text(expected: &str, answer: &str, spelling_strict: bool) -> Grade {
    grading::grade(
        Version {
            units: &[expected],
            spelling_strict,
        },
        0..1,
        answer,
        &[],
    )
    .unwrap()
}

#[test]
fn conceived_slip_is_visible_and_spelling_strict_changes_the_verdict() {
    let ordinary = grade_text("conceived", "concieved", false);
    let strict = grade_text("conceived", "concieved", true);
    assert_eq!(ordinary.verdict, Verdict::Good);
    assert_eq!(strict.verdict, Verdict::Again);
    assert_eq!(ordinary.units[0].verdict, Verdict::Good);
    assert_eq!(strict.units[0].verdict, Verdict::Again);
    assert!(matches!(ordinary.diff[0].change, Change::Slip { .. }));
    assert_eq!(ordinary.diff, strict.diff);
    assert_eq!(ordinary.authority.name, AUTHORITY);
    assert_eq!(ordinary.authority.grader_version, GRADER_VERSION);
    println!("ordinary: {}", serde_json::to_string(&ordinary).unwrap());
    println!(
        "Spelling-strict: {}",
        serde_json::to_string(&strict).unwrap()
    );
}

#[test]
fn normalization_ignores_only_the_declared_differences() {
    for (expected, answer) in [
        ("Patient maker", "PATIENT MAKER"),
        ("Patient, maker!", "patient maker"),
        ("‘Lord’s’ mercy", "\"Lord's\" mercy"),
        ("Lord's mercy", "Lord\u{02bc}s mercy"),
        ("Lord's mercy", "Lord`s mercy"),
        ("Lord's mercy", "Lord‚s mercy"),
        ("sun-moon", "sun–moon"),
        ("sun-moon", "sun—moon"),
        ("sun-moon", "sun−moon"),
        ("sun moon", "sun\n\r\t moon"),
        ("sun and moon", "sun&moon"),
        ("sun & moon", "sun ＆ moon"),
        ("sun & moon", "sun ﹠ moon"),
        ("creed", "c.r.e.e.d."),
        ("word", "'word'"),
        ("recalled", "（recalled）！"),
    ] {
        let result = grade_text(expected, answer, true);
        assert_eq!(result.verdict, Verdict::Good, "{expected:?} / {answer:?}");
        assert!(
            result
                .diff
                .iter()
                .all(|entry| matches!(entry.change, Change::Correct { .. }))
        );
    }
}

#[test]
fn multilingual_nfkc_and_full_case_folding_are_used_in_both_directions() {
    for (a, b) in [
        ("Ｓｔｒａße", "STRASSE"),
        ("oﬃce", "OFFICE"),
        ("Kelvin ① Ⅳ", "kelvin 1 iv"),
        ("Σίσυφος", "σίσυφοσ"),
        ("İ", "i\u{307}"),
        ("ÉCOLE", "e\u{301}cole"),
        ("A\u{315}\u{300}", "à\u{315}"),
        ("ᾲ", "ὰι"),
        ("ЖИЗНЬ", "жизнь"),
        ("ԿԵԱՆՔ", "կեանք"),
        ("ﾊﾟﾝ", "パン"),
        ("각", "각"),
        ("㈎", "(가)"),
        ("مَرْحَبًا", "مَرْحَبًا"),
        ("生命", "生命"),
        ("\u{1c89}", "\u{1c8a}"),
    ] {
        assert_eq!(
            grade_text(a, b, true).verdict,
            Verdict::Good,
            "{a:?} / {b:?}"
        );
        assert_eq!(
            grade_text(b, a, true).verdict,
            Verdict::Good,
            "{b:?} / {a:?}"
        );
    }
}

#[test]
fn digits_symbols_apostrophes_and_meaning_are_not_extra_tolerances() {
    for (expected, answer) in [
        ("Lord's", "Lords"),
        ("can't", "cant"),
        ("maker's", "makers"),
        ("i", "ı"),
        ("café", "cafe"),
        ("12345", "12346"),
        ("conceived1", "conceived2"),
        ("conceived", "conceived1"),
        ("conceived", "conceived+"),
        ("creed", "creed\u{200b}"),
        ("light ☀", "light"),
        ("holy spirit", "holy ghost"),
    ] {
        assert_eq!(
            grade_text(expected, answer, false).verdict,
            Verdict::Again,
            "{expected:?} / {answer:?}"
        );
    }
}

#[test]
fn all_four_single_letter_slips_are_visible_and_good() {
    for (expected, answer) in [
        ("maker", "makers"),
        ("maker", "aker"),
        ("maker", "mker"),
        ("maker", "make"),
        ("maker", "baker"),
        ("maker", "makre"),
        ("conceived", "concieved"),
        ("Juliett", "Juliet"),
        ("светлый", "светлы"),
        ("école", "écolx"),
        ("生命希望愛", "生命希望愛恵"),
    ] {
        let result = grade_text(expected, answer, false);
        assert_eq!(result.verdict, Verdict::Good, "{expected} / {answer}");
        assert_eq!(result.diff.len(), 1);
        assert!(
            matches!(result.diff[0].change, Change::Slip { .. }),
            "{result:?}"
        );
        assert_eq!(grade_text(expected, answer, true).verdict, Verdict::Again);
    }
}

#[test]
fn slip_length_distance_and_version_vocabulary_boundaries() {
    for (expected, answer) in [
        ("Alfa", "Alph"),
        ("four", "fou"),
        ("Alfa", "Afla"),
        ("ab'cd", "ab'ce"),
        ("1abcd", "1abce"),
        ("café", "cafes"),
        ("maker", "mixer"),
        ("maker", "mak"),
        ("maker", "remak"),
        ("maker baker", "baker baker"),
        ("maker BAKER", "baker BAKER"),
    ] {
        let result = grade_text(expected, answer, false);
        assert_eq!(result.verdict, Verdict::Again, "{expected} / {answer}");
        assert!(
            !result
                .diff
                .iter()
                .any(|entry| matches!(entry.change, Change::Slip { .. }))
        );
    }
    assert_eq!(
        grade_text("maker baker", "maker bakers", false).verdict,
        Verdict::Good
    );
    let result = grading::grade(
        Version {
            units: &["maker", "baker"],
            spelling_strict: false,
        },
        0..1,
        "baker",
        &[],
    )
    .unwrap();
    assert_eq!(result.verdict, Verdict::Again);
    assert!(matches!(result.diff[0].change, Change::Wrong { .. }));
}

#[test]
fn missing_extra_substituted_and_swapped_words_are_marked_and_again() {
    let expected = "we walk by quiet water";
    for answer in [
        "we walk by water",
        "we walk by quiet deep water",
        "we walk beside quiet water",
        "we walk quiet by water",
        "walk we by quiet water",
        "we walk by water quiet",
        "",
        "different words altogether",
    ] {
        assert_eq!(
            grade_text(expected, answer, false).verdict,
            Verdict::Again,
            "{answer}"
        );
    }
    assert!(matches!(
        grade_text(expected, "we walk by water", false).diff[3].change,
        Change::Missing { .. }
    ));
    assert!(matches!(
        grade_text(expected, "we walk by quiet deep water", false).diff[4].change,
        Change::Extra { .. }
    ));
    assert!(matches!(
        grade_text(expected, "we walk beside quiet water", false).diff[2].change,
        Change::Wrong {
            out_of_order: false,
            ..
        }
    ));
    let swapped = grade_text(expected, "we walk quiet by water", false);
    assert_eq!(swapped.diff.len(), 5);
    for i in [2, 3] {
        assert!(matches!(
            swapped.diff[i].change,
            Change::Wrong {
                out_of_order: true,
                ..
            }
        ));
    }
}

#[test]
fn synthetic_creed_recitation_attributes_one_wrong_word_to_clause_four() {
    // Authored synthetic clauses; no supplied/private/copyrighted Version is loaded.
    let units = [
        "We affirm a patient maker.",
        "We cherish the fields and rivers.",
        "We remember our first teachers.",
        "Our promise was conceived in kindness.",
        "We carry a lamp through darkness.",
        "We offer shelter to the traveller.",
        "We keep the names of the departed.",
        "We gather when the bells awaken.",
        "We share bread with our neighbours.",
        "We seek a just and gentle world.",
        "We await the turning of the seasons.",
        "We begin again with hope.",
    ];
    let answer = units.join("\n").replace("kindness", "anger");
    let result = grading::grade(
        Version {
            units: &units,
            spelling_strict: false,
        },
        0..12,
        &answer,
        &[],
    )
    .unwrap();
    assert_eq!(result.verdict, Verdict::Again);
    for unit in &result.units {
        assert_eq!(
            unit.verdict,
            if unit.unit_index == 3 {
                Verdict::Again
            } else {
                Verdict::Good
            }
        );
    }
    let edits: Vec<_> = result
        .diff
        .iter()
        .filter(|entry| !matches!(entry.change, Change::Correct { .. }))
        .collect();
    assert_eq!(edits.len(), 1);
    assert_eq!(edits[0].unit_index, 3);
    match &edits[0].change {
        Change::Wrong {
            expected, typed, ..
        } => {
            assert_eq!(expected.text, "kindness");
            assert_eq!(typed.text, "anger");
        }
        change => panic!("expected wrong word, got {change:?}"),
    }
}

#[test]
fn missing_extra_and_cross_unit_transposition_each_keep_unit_attribution() {
    let version = Version {
        units: &["red blue", "green gold", "black white"],
        spelling_strict: false,
    };
    for (answer, verdicts) in [
        (
            "red blue gold black white",
            [Verdict::Good, Verdict::Again, Verdict::Good],
        ),
        (
            "red blue extra green gold black white",
            [Verdict::Again, Verdict::Good, Verdict::Good],
        ),
        (
            "extra red blue green gold black white",
            [Verdict::Again, Verdict::Good, Verdict::Good],
        ),
        (
            "red blue green gold black white extra",
            [Verdict::Good, Verdict::Good, Verdict::Again],
        ),
        (
            "red green blue gold black white",
            [Verdict::Again, Verdict::Again, Verdict::Good],
        ),
        (
            "red blue black white",
            [Verdict::Good, Verdict::Again, Verdict::Good],
        ),
        ("", [Verdict::Again; 3]),
    ] {
        let result = grading::grade(version, 0..3, answer, &[]).unwrap();
        assert_eq!(
            result
                .units
                .iter()
                .map(|unit| unit.verdict)
                .collect::<Vec<_>>(),
            verdicts,
            "{answer}: {result:?}"
        );
    }
    let selected = grading::grade(version, 1..2, "extra green gold", &[]).unwrap();
    assert_eq!(selected.units[0].unit_index, 1);
    assert!(selected.diff.iter().all(|entry| entry.unit_index == 1));
}

#[test]
fn wrong_version_hint_names_the_closer_sibling_without_accepting_its_wording() {
    // A short wording contrast, not a reproduction of either complete Creed.
    let version = Version {
        units: &["the holy spirit"],
        spelling_strict: false,
    };
    let sibling = SiblingVersion {
        id: "bcp-1662",
        name: "English, 1662 BCP",
        expected: "the holy ghost",
    };
    let result = grading::grade(version, 0..1, "the Holy Ghost", &[sibling]).unwrap();
    assert_eq!(result.verdict, Verdict::Again);
    assert_eq!(result.wrong_version_hint.as_ref().unwrap().id, "bcp-1662");
    assert_eq!(
        result.wrong_version_hint.as_ref().unwrap().name,
        "English, 1662 BCP"
    );
    assert!(matches!(result.diff[2].change, Change::Wrong { .. }));
    assert!(
        grading::grade(version, 0..1, "the holy spirit", &[sibling])
            .unwrap()
            .wrong_version_hint
            .is_none()
    );
    assert!(
        grading::grade(version, 0..1, "the holy light", &[sibling])
            .unwrap()
            .wrong_version_hint
            .is_none()
    );
}

#[test]
fn closer_sibling_need_not_match_exactly_and_ties_are_stable_by_id() {
    let version = Version {
        units: &["red green blue gold"],
        spelling_strict: false,
    };
    let a = SiblingVersion {
        id: "a",
        name: "First",
        expected: "red green black white",
    };
    let b = SiblingVersion {
        id: "b",
        name: "Second",
        expected: "red green black white",
    };
    let first = grading::grade(version, 0..1, "red green black gray", &[a, b]).unwrap();
    let reversed = grading::grade(version, 0..1, "red green black gray", &[b, a]).unwrap();
    assert_eq!(first, reversed);
    assert_eq!(first.wrong_version_hint.unwrap().id, "a");
}

#[test]
fn original_words_and_utf8_spans_survive_expansion_composition_and_case_folding() {
    let expected = "École & Straße — ＬＯＲＤ’s";
    let answer = "e\u{301}cole and STRASSE Lord's";
    let result = grade_text(expected, answer, true);
    assert_eq!(result.verdict, Verdict::Good);
    for entry in &result.diff {
        match &entry.change {
            Change::Correct { expected: e, typed } => {
                assert_eq!(&expected[e.span.clone()], e.text);
                assert_eq!(&answer[typed.span.clone()], typed.text);
            }
            change => panic!("expected exact normalized word, got {change:?}"),
        }
    }
    match &result.diff[1].change {
        Change::Correct { expected, typed } => {
            assert_eq!(expected.text, "&");
            assert_eq!(typed.text, "and");
        }
        change => panic!("expected ampersand expansion, got {change:?}"),
    }
}

#[test]
fn random_answers_produce_identical_serialized_bytes_and_cover_every_word() {
    let units = [
        "we walk by water",
        "the promise was conceived",
        "green gold black white",
    ];
    let alphabet = [
        "we",
        "walk",
        "water",
        "the",
        "concieved",
        "promise",
        "gold",
        "black",
        "white",
        "Straße",
        "e\u{301}cole",
        "&",
        "👁",
        "green",
        "",
        "—",
    ];
    let version = Version {
        units: &units,
        spelling_strict: false,
    };
    let mut state = 0xe3ac_7a25_519c_bed1u64;
    for i in 0..1024 {
        let mut answer = Vec::new();
        let mut typed_word_count = 0;
        for _ in 0..i % 19 {
            state ^= state << 13;
            state ^= state >> 7;
            state ^= state << 17;
            let word = alphabet[state as usize % alphabet.len()];
            if !word.is_empty() && word != "—" {
                typed_word_count += 1;
            }
            answer.push(word);
        }
        let answer = answer.join(" ");
        let result = grading::grade(version, 0..3, &answer, &[]).unwrap();
        let bytes = serde_json::to_vec(&result).unwrap();
        for _ in 0..3 {
            assert_eq!(
                bytes,
                serde_json::to_vec(&grading::grade(version, 0..3, &answer, &[]).unwrap()).unwrap()
            );
        }
        assert_eq!(serde_json::from_slice::<Grade>(&bytes).unwrap(), result);
        let expected_words = result
            .diff
            .iter()
            .filter(|entry| !matches!(entry.change, Change::Extra { .. }))
            .count();
        assert_eq!(expected_words, 12);
        assert_eq!(
            result
                .diff
                .iter()
                .filter(|entry| !matches!(entry.change, Change::Missing { .. }))
                .count(),
            typed_word_count
        );
        for unit in &result.units {
            let correct = result
                .diff
                .iter()
                .filter(|entry| entry.unit_index == unit.unit_index)
                .all(|entry| matches!(entry.change, Change::Correct { .. } | Change::Slip { .. }));
            assert_eq!(unit.verdict == Verdict::Good, correct);
        }
    }
}

#[test]
fn invalid_unit_ranges_are_refused_without_panicking() {
    let version = Version {
        units: &["one", "two"],
        spelling_strict: false,
    };
    for range in [0..0, 1..1, 2..3, 0..3, usize::MAX..usize::MAX] {
        assert_eq!(
            grading::grade(version, range, "", &[]),
            Err(GradeError::InvalidUnitRange)
        );
    }
    assert_eq!(
        grading::grade(
            Version {
                units: &[],
                spelling_strict: false
            },
            0..1,
            "",
            &[]
        ),
        Err(GradeError::InvalidUnitRange)
    );
}

#[test]
fn mapping_profile_separates_punctuation_without_rewriting_historical_text_grades() {
    use scry::grading::{self, Verdict, Version};
    let units = ["Alfa", "Juliett", "X-ray"];
    let v = Version {
        units: &units,
        spelling_strict: true,
    };
    let historical = grading::grade(v, 0..1, "a.l.f.a", &[]).unwrap();
    assert_eq!(historical.verdict, Verdict::Good);
    assert_eq!(historical.authority.grader_version, grading::GRADER_VERSION);
    for answer in ["a.l.f.a", "Al/fa", "Alpha", "Alfa Bravo"] {
        let grade = grading::grade_mapping(v, 0..1, answer).unwrap();
        assert_eq!(grade.verdict, Verdict::Again, "{answer}");
        assert_eq!(
            grade.authority.grader_version,
            grading::MAPPING_GRADER_VERSION
        );
    }
    for answer in ["ALFA", "  Alfa!\n", "Ａｌｆａ"] {
        assert_eq!(
            grading::grade_mapping(v, 0..1, answer).unwrap().verdict,
            Verdict::Good
        );
    }
    for answer in ["x-ray", " X ray ", "X—ray"] {
        assert_eq!(
            grading::grade_mapping(v, 2..3, answer).unwrap().verdict,
            Verdict::Good
        );
    }
    assert_eq!(
        grading::grade_mapping(v, 2..3, "Xray").unwrap().verdict,
        Verdict::Again
    );
    assert_ne!(
        grading::mapping_words("a&b"),
        grading::mapping_words("a and b")
    );
    assert_ne!(grading::mapping_words("a.b"), grading::mapping_words("ab"));
}
