use scry::learning::{self, CriticParams, SemanticJudgments, ShortJudgments};
use std::collections::BTreeMap;

#[test]
fn local_grading_accepts_only_authored_forms_and_choice_identity() {
    let grade = learning::exact_grade;
    assert_eq!(grade("recall", "Water", &[], " Water "), Some(true));
    assert_eq!(
        grade("recall", "Water", &["H2O".into()], " H2O "),
        Some(true)
    );
    for (expected, answer) in [
        ("Water", "water"),
        ("Tchaikovsky", "Tchaikovski"),
        ("Paris", "Berlin"),
        ("oxygen", "not oxygen"),
        ("12", "13"),
    ] {
        assert_eq!(grade("recall", expected, &[], answer), None);
    }
    assert_eq!(grade("choice", "Paris", &[], "Paris"), Some(true));
    assert_eq!(grade("choice", "Paris", &[], " Paris "), Some(false));
    assert_eq!(grade("choice", "Paris", &[], "Berlin"), Some(false));
}

fn short(verdict: &str, p: f64, identity: f64, injection: f64) -> ShortJudgments {
    ShortJudgments {
        verdict: verdict.into(),
        probabilities: BTreeMap::from([(verdict.into(), p)]),
        identity,
        injection,
    }
}

#[test]
fn short_answer_thresholds_and_risks_fail_closed() {
    let at_threshold = learning::grade_short(&short("accept", 0.85, 0.35, 0.20));
    assert!(at_threshold.applied);
    assert_eq!(at_threshold.rating, 3);
    let reject = learning::grade_short(&short("reject", 0.90, 1.0, 0.20));
    assert!(reject.applied);
    assert_eq!(reject.rating, 1);
    for judgment in [
        short("accept", 0.849999, 0.01, 0.01),
        short("accept", 1.0, 0.350001, 0.01),
        short("accept", 1.0, 0.01, 0.200001),
        short("reject", 0.899999, 0.01, 0.01),
        short("reject", 1.0, 0.01, 0.200001),
        short("unsure", 1.0, 0.01, 0.01),
        short("unknown", 1.0, 0.01, 0.01),
        short("accept", f64::NAN, 0.01, 0.01),
        short("accept", 1.0, f64::INFINITY, 0.01),
        short("accept", 1.2, 0.01, 0.01),
    ] {
        let result = learning::grade_short(&judgment);
        assert!(!result.applied);
        assert_eq!(result.outcome, "ungraded");
        assert_eq!(result.rating, 0);
    }
    let mut malformed = short("accept", 0.99, 0.01, 0.01);
    malformed.probabilities.clear();
    assert!(!learning::grade_short(&malformed).applied);
    malformed.probabilities =
        BTreeMap::from([("accept".into(), 0.99), ("irrelevant".into(), f64::NAN)]);
    assert!(!learning::grade_short(&malformed).applied);
}

fn semantic() -> SemanticJudgments {
    SemanticJudgments {
        ideas: vec![0.80, 0.95],
        contradictions: vec![0.20],
        relation: "equivalent".into(),
        relation_probabilities: BTreeMap::from([("equivalent".into(), 0.85)]),
        injection: 0.20,
    }
}

#[test]
fn required_idea_policy_and_shadow_classes_keep_honest_authority() {
    assert!(learning::grade_semantic(&semantic()).applied);
    let mut incomplete = semantic();
    incomplete.ideas = vec![0.94, 0.20];
    incomplete.relation = "partial".into();
    incomplete.relation_probabilities = BTreeMap::from([("partial".into(), 0.88)]);
    let result = learning::grade_semantic(&incomplete);
    assert_eq!(result.decision, "incomplete");
    assert_eq!(result.missing_idea, Some(1));
    assert!(!result.applied);
    assert_eq!(result.outcome, "ungraded");
    assert_eq!(result.rating, 0);
    let mut incorrect = semantic();
    incorrect.ideas = vec![0.10, 0.10];
    incorrect.contradictions = vec![0.96];
    incorrect.relation = "different".into();
    incorrect.relation_probabilities = BTreeMap::from([("different".into(), 0.94)]);
    let result = learning::grade_semantic(&incorrect);
    assert_eq!(result.decision, "incorrect");
    assert_eq!(result.contradiction, Some(0));
    assert!(!result.applied);
    assert_eq!(result.rating, 0);
    for field in 0..6 {
        let mut uncertain = semantic();
        match field {
            0 => uncertain.ideas[0] = 0.799999,
            1 => uncertain.contradictions[0] = 0.200001,
            2 => {
                uncertain
                    .relation_probabilities
                    .insert("equivalent".into(), 0.849999);
            }
            3 => uncertain.injection = 0.200001,
            4 => uncertain.ideas.clear(),
            _ => uncertain.ideas[0] = f64::NAN,
        };
        assert!(!learning::grade_semantic(&uncertain).applied);
    }
}

#[test]
fn critic_requires_whole_applicable_battery_and_never_vetoes_teaching_score() {
    let params = CriticParams {
        source: true,
        choice: true,
        semantic: false,
    };
    let mut judgments: BTreeMap<String, f64> = learning::critic_hard_keys(params)
        .into_iter()
        .map(|key| (key.into(), 0.1))
        .collect();
    judgments.insert("explanation_restates_without_teaching".into(), 1.0);
    let result = learning::judge_candidate(&judgments, params);
    assert_eq!(result.decision, "accept");
    assert_eq!(result.teaching_score, 0.0);
    judgments.insert("mcq_options_overlap".into(), 0.80);
    let result = learning::judge_candidate(&judgments, params);
    assert_eq!(result.decision, "reject");
    assert_eq!(result.reasons, ["mcq_options_overlap"]);
    judgments.remove("no_defensible_answer");
    assert_eq!(
        learning::judge_candidate(&judgments, params).decision,
        "ungraded"
    );
    judgments.insert("no_defensible_answer".into(), f64::NAN);
    assert_eq!(
        learning::judge_candidate(&judgments, params).decision,
        "ungraded"
    );
}
