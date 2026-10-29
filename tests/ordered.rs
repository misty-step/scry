use scry::{
    grading::Verdict,
    library::{Library, Version, VersionInput},
    ordered::{self, ItemStatus},
};
use serde_json::json;

fn input() -> VersionInput {
    serde_json::from_value(json!({
        "title":"Five synthetic landmarks", "kind":"ordered_list", "name":"Synthetic v1",
        "text":"Red gate\nBlue bridge\nGreen field\nWhite tower\nBlack lake\n",
        "sections":[{"heading":"Route", "units":["Red gate\n","Blue bridge\n","Green field\n","White tower\n","Black lake\n"]}],
        "ordered_aliases":{"0":["Scarlet gate"],"1":["Azure bridge"]}
    })).unwrap()
}
fn version() -> Version {
    let mut library = Library::default();
    let id = library
        .add(input(), "synthetic-ordered-first", None)
        .unwrap();
    library.validate().unwrap();
    library.versions[&id].clone()
}
#[test]
fn complete_sequence_accepts_authored_names_not_unwritten_synonyms() {
    let v = version();
    let answer = "SCARLET GATE!\r\nAzure bridge\r\nGreen field\r\nWhite tower\r\nBlack lake";
    let result = ordered::grade(&v, 0..5, answer).unwrap();
    assert_eq!(result.verdict, Verdict::Good);
    assert!(result.units.iter().all(|u| u.verdict == Verdict::Good));
    assert_eq!(result.authority.name, "ordered-v1");
    assert!(result.diff.is_empty());
    assert_eq!(
        ordered::grade(&v, 0..1, "Crimson door").unwrap().verdict,
        Verdict::Again
    );
    assert_eq!(
        ordered::grade(&v, 0..1, "Red gat").unwrap().verdict,
        Verdict::Again
    );
}
#[test]
fn omission_does_not_poison_later_in_order_items() {
    let v = version();
    let result =
        ordered::feedback(&v, 0..5, "Red gate\nGreen field\nWhite tower\nBlack lake").unwrap();
    assert!(!result.complete);
    assert_eq!(result.recalled, 4);
    assert_eq!(result.in_order, 4);
    assert_eq!(result.items[1].status, ItemStatus::Missing);
    assert_eq!(result.items[2].status, ItemStatus::Correct);
    assert_eq!(result.items[2].answered_positions, vec![2]);
    assert_eq!(result.items[4].status, ItemStatus::Correct);
}
#[test]
fn swap_is_identity_success_but_order_failure_for_both_items() {
    let v = version();
    let result = ordered::feedback(
        &v,
        0..5,
        "Red gate\nGreen field\nBlue bridge\nWhite tower\nBlack lake",
    )
    .unwrap();
    assert_eq!(result.recalled, 5);
    assert_eq!(result.in_order, 3);
    assert_eq!(result.items[1].status, ItemStatus::Misplaced);
    assert_eq!(result.items[2].status, ItemStatus::Misplaced);
    assert!(!result.complete);
}
#[test]
fn repeated_and_unknown_lines_are_not_silently_discarded() {
    let v = version();
    let answer =
        "Red gate\nScarlet gate\nUnknown place\nBlue bridge\nGreen field\nWhite tower\nBlack lake";
    let result = ordered::feedback(&v, 0..5, answer).unwrap();
    assert_eq!(result.items[0].status, ItemStatus::Repeated);
    assert_eq!(result.items[0].answered_positions, vec![1, 2]);
    assert_eq!(result.items[1].status, ItemStatus::Correct);
    assert_eq!(result.extras.len(), 2);
    assert!(!result.complete);
    let all_plus_extra = "Red gate\nBlue bridge\nGreen field\nWhite tower\nBlack lake\nWrong";
    assert_eq!(
        ordered::grade(&v, 0..5, all_plus_extra).unwrap().verdict,
        Verdict::Again
    );
}
#[test]
fn single_item_and_subsequence_use_absolute_version_identity() {
    let v = version();
    let result = ordered::grade(&v, 1..2, "Azure bridge").unwrap();
    assert_eq!(result.units[0].unit_index, 1);
    assert_eq!(result.verdict, Verdict::Good);
    assert_eq!(
        ordered::grade(&v, 1..2, "Red gate").unwrap().verdict,
        Verdict::Again
    );
    assert!(ordered::grade(&v, 0..0, "").is_err());
    assert!(ordered::grade(&v, 0..6, "").is_err());
    assert!(ordered::grade(&v, 0..5, &"x".repeat(32769)).is_err());
    assert!(ordered::grade(&v, 0..5, &"x\n".repeat(257)).is_err());
}
#[test]
fn aliases_are_immutable_policy_and_replay_survives_roundtrip() {
    let mut library = Library::default();
    let first = library.add(input(), "first", None).unwrap();
    let original = library.versions[&first].clone();
    assert_eq!(library.add(input(), "same", None).unwrap(), first);
    let mut changed = input();
    changed
        .ordered_aliases
        .get_mut(&0)
        .unwrap()
        .push("Vermilion gate".into());
    let second = library.add(changed, "second", None).unwrap();
    assert_ne!(first, second);
    assert_ne!(
        library.versions[&first].normalized_hash,
        library.versions[&second].normalized_hash
    );
    assert_eq!(library.versions[&first], original);
    let reloaded: Library =
        serde_json::from_value(serde_json::to_value(&library).unwrap()).unwrap();
    reloaded.validate().unwrap();
    assert_eq!(
        ordered::grade(&reloaded.versions[&first], 0..1, "Vermilion gate")
            .unwrap()
            .verdict,
        Verdict::Again
    );
    assert_eq!(
        ordered::grade(&reloaded.versions[&second], 0..1, "Vermilion gate")
            .unwrap()
            .verdict,
        Verdict::Good
    );
    library
        .versions
        .get_mut(&first)
        .unwrap()
        .ordered_aliases
        .get_mut(&0)
        .unwrap()
        .push("Tampered alias".into());
    assert!(library.validate().is_err());
}
#[test]
fn collisions_invalid_aliases_and_cross_kind_aliases_are_refused_atomically() {
    for aliases in [
        json!({"0":["Blue bridge"]}),
        json!({"5":["Elsewhere"]}),
        json!({"0":[]}),
        json!({"0":["one\ntwo"]}),
        json!({"0":["!!!"]}),
    ] {
        let mut payload = serde_json::to_value(input()).unwrap();
        payload["ordered_aliases"] = aliases;
        let mut library = Library::default();
        assert!(
            library
                .add(serde_json::from_value(payload).unwrap(), "invalid", None)
                .is_err()
        );
        assert!(library.is_empty());
    }
    let mut text = input();
    text.kind = scry::library::Kind::Text;
    assert!(Library::default().add(text, "text", None).is_err());
}
#[test]
fn old_no_alias_versions_keep_their_serialized_shape_and_hash() {
    let mut library = Library::default();
    let mut old = input();
    old.ordered_aliases.clear();
    let id = library.add(old, "historical", None).unwrap();
    let value = serde_json::to_value(&library).unwrap();
    assert!(value["versions"][&id].get("ordered_aliases").is_none());
    let restored: Library = serde_json::from_value(value).unwrap();
    restored.validate().unwrap();
    assert_eq!(library, restored);
    assert_eq!(
        ordered::grade(&restored.versions[&id], 0..1, "Scarlet gate")
            .unwrap()
            .verdict,
        Verdict::Again
    );
}

#[test]
fn bundled_lesson_inputs_validate_without_alias_collisions() {
    let seeds: Vec<serde_json::Value> =
        serde_json::from_str(include_str!("../assets/lessons.json")).unwrap();
    let mut library = Library::default();
    for seed in seeds {
        let id = seed["id"].as_str().unwrap();
        let input: VersionInput = serde_json::from_value(seed["input"].clone()).unwrap();
        let version_id = library.add(input, id, None).unwrap();
        let version = &library.versions[&version_id];
        if version.kind == scry::library::Kind::OrderedList {
            ordered::validate(version).unwrap();
            let count = version.sections.iter().map(|s| s.units.len()).sum();
            assert_eq!(
                ordered::grade(version, 0..count, &version.text)
                    .unwrap()
                    .verdict,
                Verdict::Good
            );
        }
    }
    library.validate().unwrap();
}
