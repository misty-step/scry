//! What the Owner sees during and after a Quiz, through native mutations and render.
use scry::{engine, model::*, study};
use std::collections::BTreeMap;
const NOW: i64 = 1_800_000_000_000;
const FIRST: &str = "First little phrase. ";
const SECOND: &str = "We conceived a patient garden.";
fn app() -> App {
    App::new("synthetic-quiz-csrf-at-least-32-chars".into())
}
fn mutate_at(a: &mut App, path: &str, extra: &[(&str, &str)], now: i64) -> MutationResult {
    let mut f = BTreeMap::from([
        ("csrf".into(), a.csrf.clone()),
        ("revision".into(), a.revision.to_string()),
        (
            "operation_id".into(),
            format!("quiz-operation-{:016}", a.revision),
        ),
    ]);
    for (k, v) in extra {
        f.insert((*k).into(), (*v).into());
    }
    let seed = format!("quiz-seed-{}", a.revision);
    let result = engine::mutate(a, path, &f, now, &seed).unwrap();
    engine::validate_app(a).unwrap();
    result
}
fn mutate(a: &mut App, path: &str, extra: &[(&str, &str)]) -> MutationResult {
    mutate_at(a, path, extra, NOW)
}
fn current(a: &App) -> &study::Presentation {
    a.practice.current.as_ref().unwrap()
}
fn quiz(a: &mut App) {
    if current(a).source_visible() {
        let id = current(a).prompt.id.clone();
        mutate(a, "/practice/quiz", &[("prompt_id", &id)]);
    }
}
fn answer(a: &mut App, text: &str) {
    quiz(a);
    let id = current(a).prompt.id.clone();
    mutate(
        a,
        "/practice/answer",
        &[("prompt_id", &id), ("answer", text)],
    );
}
fn lesson_next(a: &mut App, mode: &str) {
    let id = current(a).prompt.id.clone();
    mutate(
        a,
        "/practice/lesson/next",
        &[("after", &id), ("mode", mode)],
    );
}
fn page(a: &App, path: &str) -> String {
    scry::web::render(a, path, &BTreeMap::new(), NOW, None)
}
fn add(a: &mut App, name: &str, units: [&str; 2]) -> String {
    let data = serde_json::json!({"title":"Synthetic phrase lesson","kind":"text","name":name,"text":units.concat(),"sections":[{"heading":"The garden gate","units":units}]}).to_string();
    mutate(a, "/library/version/add", &[("data", &data)])
        .entity_id
        .unwrap()
}
fn lesson(a: &mut App) -> String {
    let vid = add(a, "Synthetic English", [FIRST, SECOND]);
    mutate(a, "/practice/lesson/start", &[("version_id", &vid)]);
    vid
}
fn attempt(a: &App) -> &study::Attempt {
    &a.practice.attempts[current(a).attempt_id.as_ref().unwrap()]
}

#[test]
fn a_first_quiz_is_not_labelled_review_and_a_due_review_is() {
    let mut a = app();
    lesson(&mut a);
    quiz(&mut a);
    let first = page(&a, "/recall");
    assert!(first.contains("data-study-phase=\"quiz\">Quiz</p>"));
    assert!(!first.contains(">Review</p>"));
    answer(&mut a, FIRST);
    let card = attempt(&a).effects[0].card_id.clone();
    assert!(!attempt(&a).assisted);
    let due = a.practice.cards[&card].state.schedule.due_ms;
    let id = current(&a).prompt.id.clone();
    mutate_at(&mut a, "/practice/review/start", &[("after", &id)], due);
    assert_eq!(current(&a).targets[0].card_id, card);
    assert_eq!(current(&a).phase, Some(study::StudyPhase::Quiz));
    let review = scry::web::render(&a, "/", &BTreeMap::new(), due, None);
    assert!(review.contains("data-study-phase=\"quiz\">Review</p>"));
}

#[test]
fn nato_with_only_new_letters_left_is_not_ready_to_review() {
    let mut a = app();
    mutate(&mut a, "/practice/lesson/start", &[("starter", "nato")]);
    let sid = current(&a).prompt.study_id.clone();
    // Leaving without an answer keeps the one introduced letter unanswered.
    mutate(&mut a, "/practice/drop", &[("study_id", &sid)]);
    mutate(&mut a, "/practice/resume", &[("study_id", &sid)]);
    assert!(a.practice.current.is_none() && a.practice.attempts.is_empty());
    let query = BTreeMap::from([("practice".into(), "nato".into())]);
    let home = scry::web::render(&a, "/", &query, NOW, None);
    assert!(home.contains("action=\"/practice/nato/start\""));
    assert!(!home.contains("Ready to review."));
}

#[test]
fn a_section_recitation_is_prompted_by_its_heading_and_a_whole_one_by_its_title() {
    let mut a = app();
    lesson(&mut a);
    answer(&mut a, FIRST);
    lesson_next(&mut a, "join");
    assert!(matches!(
        current(&a).prompt.scope,
        study::Scope::Section { .. }
    ));
    let section = page(&a, "/recall");
    assert!(section.contains("<p class=\"lesson-cue\">The garden gate</p>"));
    assert!(section.contains("<h1>Synthetic phrase lesson</h1>"));
    assert!(!section.contains(FIRST.trim()) && !section.contains(SECOND));
    answer(&mut a, "nothing recalled");
    lesson_next(&mut a, "whole");
    assert!(matches!(current(&a).prompt.scope, study::Scope::Whole));
    let whole = page(&a, "/recall");
    assert!(whole.contains("<h1>Synthetic phrase lesson</h1>"));
    assert!(!whole.contains("The garden gate"));
    assert!(!whole.contains(FIRST.trim()) && !whole.contains(SECOND));
}

#[test]
fn a_lesson_miss_in_another_versions_wording_names_that_version() {
    let mut a = app();
    let vid = add(&mut a, "Synthetic English", [FIRST, SECOND]);
    add(
        &mut a,
        "Synthetic other wording",
        ["Opening small sentence. ", "We imagined a quiet orchard."],
    );
    mutate(&mut a, "/practice/lesson/start", &[("version_id", &vid)]);
    answer(&mut a, "Opening small sentence.");
    let hint = attempt(&a).grade.wrong_version_hint.clone().unwrap();
    assert_eq!(hint.name, "Synthetic other wording");
    let result = page(&a, "/recall");
    assert!(result.contains("data-verdict=\"Again\""));
    assert!(result.contains(
        "<p class=\"notice\">Your answer is closer to Version Synthetic other wording.</p>"
    ));
}

#[test]
fn a_good_lesson_result_marks_its_slip_outside_any_disclosure() {
    let mut a = app();
    lesson(&mut a);
    answer(&mut a, FIRST);
    lesson_next(&mut a, "phrases");
    answer(&mut a, "We concieved a patient garden.");
    assert_eq!(format!("{:?}", attempt(&a).grade.verdict), "Good");
    let result = page(&a, "/recall");
    assert!(result.contains("data-verdict=\"Good\""));
    let (shown, compare) = result
        .split_once("<details class=\"lesson-answer\">")
        .unwrap();
    assert!(shown.contains("data-feedback-kind=\"slip\""));
    assert!(shown.contains("concieved → conceived"));
    // The full Diff stays available beside the wording.
    assert_eq!(compare.matches("data-diff-kind=").count(), 5);
    // An exact answer has nothing to mark.
    lesson_next(&mut a, "phrases");
    answer(&mut a, FIRST);
    assert!(!page(&a, "/recall").contains("data-feedback-kind"));
}

/// The guided retry and its assisted answer, as recorded for one missed phrase.
fn retried(a: &mut App, path: &str) -> study::Attempt {
    let missed = current(a).clone();
    let attempts = a.practice.attempts.clone();
    let cards = a.practice.cards.clone();
    let mixed = a.practice.mixed_review;
    let result = page(a, path);
    let (shown, rest) = result.split_once("Try this phrase again").unwrap();
    let open = shown
        .split("<details class=\"exact-full-answer\">")
        .next()
        .unwrap();
    assert!(
        open.contains(FIRST.trim()),
        "Expected phrase shows at once, outside any disclosure"
    );
    // The retry is the one primary action; continuing is the secondary choice.
    assert_eq!(result.matches(" data-next").count(), 1);
    assert!(shown.rsplit("<form").next().unwrap().contains(" data-next"));
    assert!(
        rest.contains("class=\"text-button\">Next</button>")
            || rest.contains("class=\"text-button\">Continue to next phrase</button>")
    );
    lesson_next(a, "retry");
    assert_eq!(a.practice.mixed_review, mixed);
    assert_eq!(a.practice.attempts, attempts, "Retry records nothing");
    assert_eq!(a.practice.cards, cards);
    assert_eq!(current(a).prompt.unit_ids, missed.prompt.unit_ids);
    assert_eq!(current(a).phase, Some(study::StudyPhase::Study));
    assert!(current(a).revealed);
    quiz(a);
    assert_eq!(current(a).phase, Some(study::StudyPhase::Quiz));
    let hidden = page(a, path);
    assert!(hidden.contains("name=\"answer\"") && !hidden.contains(FIRST.trim()));
    answer(a, FIRST);
    let helped = attempt(a).clone();
    assert_eq!(format!("{:?}", helped.grade.verdict), "Good");
    assert!(helped.assisted && helped.revealed);
    assert!(helped.effects.iter().all(|e| !e.scheduled));
    for (id, card) in &cards {
        assert_eq!(
            a.practice.cards[id].state, card.state,
            "Help earns no schedule credit"
        );
    }
    helped
}

#[test]
fn a_missed_phrase_offers_the_same_guided_retry_under_studys_mix() {
    // In a focused Lesson, as before.
    let mut focused = app();
    lesson(&mut focused);
    answer(&mut focused, SECOND);
    let lesson_retry = retried(&mut focused, "/recall");

    // The same Lesson, held under Study's mix.
    let mut mixed = app();
    lesson(&mut mixed);
    answer(&mut mixed, SECOND);
    mutate(&mut mixed, "/practice/review/start", &[]);
    assert!(mixed.practice.mixed_review);
    let mixed_retry = retried(&mut mixed, "/");
    assert!(page(&mixed, "/").contains("action=\"/practice/review/next\""));

    // A Study started outside the lesson shelf.
    let mut stream = app();
    let vid = add(&mut stream, "Synthetic English", [FIRST, SECOND]);
    mutate(
        &mut stream,
        "/practice/study/start",
        &[("version_id", &vid)],
    );
    assert!(!stream.practice.studies.values().next().unwrap().lesson);
    answer(&mut stream, SECOND);
    let stream_retry = retried(&mut stream, "/recall");

    // The assisted answer is recorded the same way wherever it was retried.
    for other in [&mixed_retry, &stream_retry] {
        assert_eq!(other.grade, lesson_retry.grade);
        assert_eq!(other.assisted, lesson_retry.assisted);
        assert_eq!(other.revealed, lesson_retry.revealed);
        assert_eq!(other.disclosure, lesson_retry.disclosure);
        assert_eq!(other.prompt.cue_level, lesson_retry.prompt.cue_level);
        assert_eq!(other.prompt.assisted, lesson_retry.prompt.assisted);
        assert_eq!(other.schedule_policy, lesson_retry.schedule_policy);
        assert_eq!(other.effects.len(), 1);
        assert_eq!(other.effects[0].verdict, lesson_retry.effects[0].verdict);
        assert_eq!(other.effects[0].assisted, lesson_retry.effects[0].assisted);
        assert_eq!(
            other.effects[0].scheduled,
            lesson_retry.effects[0].scheduled
        );
        assert_eq!(
            other.effects[0].extra_practice,
            lesson_retry.effects[0].extra_practice
        );
    }
}

#[test]
fn a_retry_is_refused_for_anything_but_a_text_phrase_outside_a_lesson() {
    let mut a = app();
    let mut input = scry::library::nato_input().unwrap();
    input.title = "Synthetic pairs".into();
    input.text = "A\tValue\n".into();
    input.sections.as_mut().unwrap()[0].units = vec![input.text.clone()];
    let vid = a.library.add(input, "synthetic-pairs", None).unwrap();
    mutate(&mut a, "/practice/study/start", &[("version_id", &vid)]);
    answer(&mut a, "wrong");
    assert!(!page(&a, "/recall").contains("Try this phrase again"));
    let before = a.clone();
    let id = current(&a).prompt.id.clone();
    let f = BTreeMap::from([
        ("csrf".into(), a.csrf.clone()),
        ("revision".into(), a.revision.to_string()),
        ("operation_id".into(), "quiz-operation-mapping-retry".into()),
        ("after".into(), id),
        ("mode".into(), "retry".into()),
    ]);
    assert!(engine::mutate(&mut a, "/practice/lesson/next", &f, NOW, "refused").is_err());
    assert_eq!(a, before);
}

#[test]
fn the_previous_phrase_is_a_closed_disclosure_that_is_never_the_answer() {
    for lesson_shelf in [true, false] {
        let mut a = app();
        let vid = add(&mut a, "Synthetic English", [FIRST, SECOND]);
        if lesson_shelf {
            mutate(&mut a, "/practice/lesson/start", &[("version_id", &vid)]);
        } else {
            mutate(&mut a, "/practice/study/start", &[("version_id", &vid)]);
        }
        quiz(&mut a);
        // The first phrase of a text has no previous phrase.
        let first = page(&a, "/recall");
        assert!(first.contains("name=\"answer\""));
        assert!(!first.contains("lesson-context") && !first.contains("Previous phrase"));
        answer(&mut a, FIRST);
        let id = current(&a).prompt.id.clone();
        if lesson_shelf {
            lesson_next(&mut a, "phrases");
        } else {
            mutate(&mut a, "/practice/next", &[("after", &id)]);
        }
        quiz(&mut a);
        assert!(matches!(
            current(&a).prompt.scope,
            study::Scope::Unit { position: 2, .. }
        ));
        let before = a.clone();
        let second = page(&a, "/recall");
        assert_eq!(a, before, "Offering context writes nothing");
        assert!(second.contains(
            "<details class=\"lesson-context\"><summary>Previous phrase</summary><p>First little phrase. </p></details>"
        ));
        assert!(!second.contains(SECOND), "The asked phrase stays hidden");
        // Context is not help: the stored Prompt is unchanged and unassisted.
        assert!(current(&a).prompt.context.is_none());
        assert!(!current(&a).prompt.assisted && !current(&a).revealed);
        answer(&mut a, SECOND);
        assert!(!attempt(&a).assisted);
        assert!(attempt(&a).effects.iter().all(|e| !e.assisted));
    }
}

#[test]
fn only_a_several_unit_answer_keeps_enter_for_new_lines() {
    let mut a = app();
    let vid = add(&mut a, "Synthetic English", [FIRST, SECOND]);
    mutate(&mut a, "/practice/study/start", &[("version_id", &vid)]);
    let sid = current(&a).prompt.study_id.clone();
    let field = |html: &str| {
        let (_, tail) = html.split_once("<textarea id=\"answer\"").unwrap();
        tail.split_once('>').unwrap().0.to_string()
    };
    quiz(&mut a);
    assert!(!field(&page(&a, "/recall")).contains("data-multiline"));
    answer(&mut a, FIRST);
    let id = current(&a).prompt.id.clone();
    mutate(&mut a, "/practice/next", &[("after", &id)]);
    answer(&mut a, SECOND);
    let section = a.library.versions[&vid].sections[0].id.clone();
    for scope in [
        vec![("recite", "section"), ("section_id", section.as_str())],
        vec![("recite", "whole")],
    ] {
        let id = current(&a).prompt.id.clone();
        let mut fields = vec![("after", id.as_str()), ("study_id", sid.as_str())];
        fields.extend(scope);
        mutate(&mut a, "/practice/next", &fields);
        quiz(&mut a);
        assert_eq!(current(&a).targets.len(), 2);
        assert!(field(&page(&a, "/recall")).contains("data-multiline"));
        answer(&mut a, "nothing recalled");
    }
}

#[test]
fn browse_opens_the_library_as_well_as_the_lesson_shelf() {
    let a = app();
    let home = page(&a, "/");
    let (_, browse) = home
        .split_once("<summary><span>Browse</span>")
        .expect("Home offers Browse");
    let browse = browse.split_once("</details>").unwrap().0;
    assert!(browse.contains("class=\"lesson-shelf\""));
    assert_eq!(
        browse
            .matches("<a class=\"text-link\" href=\"/library\">Library</a>")
            .count(),
        1
    );
    assert!(!home.contains("<details name=\"practice-tools\" class=\"lesson-picker\" open"));
}
