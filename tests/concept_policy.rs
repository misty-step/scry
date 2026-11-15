use scry::learning::{self, Attempt};

fn observation(at_ms: i64, rating: u8, assisted: bool, outcome: &str) -> Attempt {
    Attempt {
        at_ms,
        rating,
        assisted,
        outcome: outcome.into(),
    }
}

#[test]
fn observations_distinguish_exposure_help_and_cold_recall_without_mutations() {
    let now = 1_790_000_000_000;
    let state = learning::compute_concept_state(&[]);
    assert!(state.tally.is_empty());
    assert_eq!((state.unaided, state.helped, state.missed), (0, 0, 0));
    assert!(state.last_at_ms.is_none());
    let attempts = vec![
        observation(now + 5, 3, false, "correct"),
        observation(now + 1, 0, true, "warm_correct"),
        observation(now + 4, 1, false, "wrong"),
        observation(now + 2, 1, true, "revealed"),
        observation(now + 3, 0, false, "known_already"),
    ];
    let state = learning::compute_concept_state(&attempts);
    assert_eq!(state.unaided, 1);
    assert_eq!(state.helped, 2);
    assert_eq!(state.missed, 1);
    assert_eq!(state.tally, ["h", "h", "m", "u"]);
    assert_eq!(state.last_at_ms, Some(now + 5));
}

#[test]
fn the_tally_caps_observations_without_losing_counts() {
    let now = 1_790_000_000_000;
    let attempts: Vec<_> = (0..20)
        .map(|i| observation(now + i, 3, false, "correct"))
        .collect();
    let state = learning::compute_concept_state(&attempts);
    assert_eq!(state.tally.len(), 12);
    assert_eq!(state.unaided, 20);
    assert_eq!(state.last_at_ms, Some(now + 19));
}
