//! Paid judgment of memorization answers, driven natively: the gate, the stored
//! model, every state of a Check, the late schedule effect, validation and the
//! archive. Nothing here sends anything; a Check is claimed and its response is
//! supplied as synthetic raw bytes, as `tests/work_outcomes.rs` does for the
//! earlier concept flow.
use scry::{
    engine::{self, Work, WorkConfig},
    generation,
    grading::Verdict,
    model::*,
    owner_token::{MutationActor, OwnerToken},
    persistence, result_progress, spend,
    study::{self, Attempt, Check, CheckStatus, JudgmentState, StudyPhase},
    web,
};
use serde_json::{Value, json};
use std::collections::BTreeMap;

const NOW: i64 = 1_800_000_000_000;
const MINUTE: i64 = 60_000;
const MODEL: &str = "synthetic-jev-2026";
const JUDGED: &str = "scry-rust-durable-sqlite-study-judged-v4";

fn config() -> WorkConfig {
    WorkConfig {
        generation: false,
        jev: true,
        critic: false,
        model: String::new(),
        jev_model: "synthetic-jev".into(),
    }
}
/// A raw Jev response choosing `choice` with probability `p`.
fn response(choice: &str, p: f64, identity: f64, injection: f64) -> String {
    let rest = (1.0 - p) / 2.0;
    let mut probabilities = json!({"accept":rest,"reject":rest,"unsure":rest});
    probabilities[choice] = json!(p);
    json!({"answers":{"verdict":{"choice":choice,"probabilities":probabilities},"identity":{"noul":identity},"injection":{"noul":injection}}}).to_string()
}
fn accept() -> String {
    response("accept", 0.98, 0.02, 0.0)
}
fn reject() -> String {
    response("reject", 0.97, 0.9, 0.0)
}
fn unsure() -> String {
    response("unsure", 0.6, 0.1, 0.0)
}

struct Owner {
    app: App,
    n: u64,
    now: i64,
    /// Whether the serving Scry can reach Jev: the engine's `jev` argument.
    jev: bool,
    token: Option<String>,
}
impl Owner {
    /// Paid judgment on, as only a native test can have it.
    fn new() -> Self {
        let mut o = Self::off();
        o.jev = true;
        o.app.preferences.paid_judgment = true;
        o
    }
    fn off() -> Self {
        Self {
            app: App::new("judgment-synthetic-csrf-0123456789".into()),
            n: 0,
            now: NOW,
            jev: false,
            token: None,
        }
    }
    fn with_token(mut self, paid: bool) -> Self {
        let token = OwnerToken::issue(
            "Judgment local agent",
            None,
            paid,
            NOW,
            "abcdef0123456789abcdef0123456789abcdef0123456789",
        )
        .unwrap();
        self.token = Some(token.info.id.clone());
        self.app.owner_tokens.insert(token.info.id.clone(), token);
        self
    }
    fn fields(&self, operation: &str, pairs: &[(&str, &str)]) -> BTreeMap<String, String> {
        let mut f = BTreeMap::from([
            ("csrf".to_string(), self.app.csrf.clone()),
            ("operation_id".to_string(), operation.to_string()),
        ]);
        f.extend(pairs.iter().map(|(k, v)| (k.to_string(), v.to_string())));
        f
    }
    /// One named operation, so a test can repeat it exactly.
    fn op(
        &mut self,
        operation: &str,
        path: &str,
        pairs: &[(&str, &str)],
    ) -> AppResult<MutationResult> {
        let f = self.fields(operation, pairs);
        let seed = format!("judgment-seed-{operation}-{}", self.app.revision);
        let token = self
            .token
            .as_ref()
            .map(|id| self.app.owner_tokens[id].info.clone());
        let actor = token
            .as_ref()
            .map_or(MutationActor::Browser, MutationActor::Token);
        let result =
            engine::mutate_judged(&mut self.app, path, &f, self.now, &seed, actor, self.jev);
        self.sound();
        result
    }
    fn try_call(&mut self, path: &str, pairs: &[(&str, &str)]) -> AppResult<MutationResult> {
        self.n += 1;
        self.op(&format!("judgment-operation-{:06}", self.n), path, pairs)
    }
    fn call(&mut self, path: &str, pairs: &[(&str, &str)]) -> MutationResult {
        self.try_call(path, pairs)
            .unwrap_or_else(|e| panic!("{path}: {} {}", e.status, e.message))
    }
    /// Every state the engine reaches is one recovery accepts and stores whole.
    fn sound(&self) {
        engine::validate_app(&self.app).unwrap();
        let rows = persistence::rows(&self.app).unwrap();
        assert_eq!(persistence::app_from_rows(&rows).unwrap(), self.app);
        assert_eq!(
            rows.contains_key("judgment_compatibility/v1"),
            persistence::judged(&self.app)
        );
    }
    fn held(&self) -> study::Presentation {
        self.app.practice.current.clone().unwrap()
    }
    /// Answer the held Prompt, trying from memory first where it is source
    /// study. Returns the Attempt.
    fn answer(&mut self, text: &str) -> String {
        if self.held().source_visible() {
            let pid = self.held().prompt.id;
            self.call("/practice/quiz", &[("prompt_id", &pid)]);
        }
        let pid = self.held().prompt.id;
        let aid = self
            .call("/practice/answer", &[("prompt_id", &pid), ("answer", text)])
            .entity_id
            .unwrap();
        self.now += MINUTE;
        aid
    }
    /// Leave the held result for one Card of the same Study.
    fn next_card(&mut self, card: &str) {
        let pid = self.held().prompt.id;
        self.call("/practice/next", &[("after", &pid), ("card_id", card)]);
    }
    fn check_again(&mut self, attempt: &str) -> AppResult<MutationResult> {
        self.try_call("/practice/check", &[("attempt_id", attempt)])
    }
    fn claim(&mut self) -> Option<Work> {
        self.claim_with(&config())
    }
    fn claim_with(&mut self, config: &WorkConfig) -> Option<Work> {
        self.n += 1;
        let lease = format!("judgment-lease-{:06}", self.n);
        let work = engine::claim_check_work(&mut self.app, self.now, &lease, config).unwrap();
        self.sound();
        work
    }
    fn finish(&mut self, work: &Work, raw: String) -> AppResult<()> {
        self.now += MINUTE / 6;
        let result = engine::finish_work(
            &mut self.app,
            work,
            raw,
            MODEL.into(),
            Some(12_000),
            self.now,
        );
        self.sound();
        result
    }
    /// Claim the queued Check and settle it with `raw`.
    fn judge(&mut self, raw: String) -> Work {
        let work = self.claim().expect("a queued Check");
        self.finish(&work, raw).unwrap();
        work
    }
    fn attempt(&self, id: &str) -> &Attempt {
        &self.app.practice.attempts[id]
    }
    fn checks(&self, attempt: &str) -> Vec<&Check> {
        let mut checks = self
            .app
            .practice
            .checks
            .values()
            .filter(|c| c.attempt_id == attempt)
            .collect::<Vec<_>>();
        checks.sort_by_key(|c| c.sequence);
        checks
    }
    fn check(&self, attempt: &str) -> &Check {
        self.checks(attempt).pop().unwrap()
    }
    fn state(&self, attempt: &str) -> JudgmentState {
        study::judgment_view(&self.app, self.attempt(attempt))
            .unwrap()
            .state
    }
    fn card(&self, id: &str) -> &study::Card {
        &self.app.practice.cards[id]
    }
    fn target(&self) -> String {
        self.held().targets[0].card_id.clone()
    }
    fn wire(&self, attempt: &str) -> Value {
        let result = MutationResult {
            data: Some(json!({"attempt_id": attempt})),
            ..MutationResult::default()
        };
        study::wire_response(&self.app, &result).unwrap()
    }
    fn page(&self, path: &str) -> String {
        web::render(
            &self.app,
            path,
            &BTreeMap::from([("__nonce".into(), "judgment-nonce".into())]),
            self.now,
            None,
        )
    }
    /// The page, also kept for a visual check where `SCRY_JUDGMENT_PAGES` names
    /// a directory: production cannot reach these states to be looked at.
    fn shown(&self, path: &str, name: &str) -> String {
        let page = self.page(path);
        if let Some(dir) = std::env::var_os("SCRY_JUDGMENT_PAGES") {
            let file = std::path::Path::new(&dir).join(format!("{name}.html"));
            std::fs::write(file, &page).unwrap();
        }
        page
    }
    fn archive(&self) -> persistence::Archive {
        persistence::Archive::new(self.app.clone(), vec![], self.now)
    }

    /// A mapping that is not Spelling-strict, held as a Quiz of its first pair.
    fn mapping(mut self) -> Self {
        let vid = self
            .call(
                "/library/mapping/add",
                &[(
                    "data",
                    &json!({"title":"Synthetic harbour practice","direction":"forward","spelling_strict":false,"pairs":[
                        {"key":"Before leaving the berth","value":"check the bilge pump"},
                        {"key":"In fog","value":"sound the horn"},
                        {"key":"A person overboard","value":"never lose sight of them"}
                    ]})
                    .to_string(),
                )],
            )
            .entity_id
            .unwrap();
        self.call("/practice/study/start", &[("version_id", &vid)]);
        self
    }
    /// A text lesson of four phrases that is not Spelling-strict.
    fn text(mut self) -> Self {
        let vid = self.add_text("Synthetic Creed", CREED, false);
        self.call("/practice/lesson/start", &[("version_id", &vid)]);
        self
    }
    fn add_text(&mut self, title: &str, text: &str, strict: bool) -> String {
        self.call(
            "/library/version/add",
            &[(
                "data",
                &json!({"title":title,"name":"Authored wording","kind":"text","text":text,"spelling_strict":strict})
                    .to_string(),
            )],
        )
        .entity_id
        .unwrap()
    }
    /// The Joyful Mysteries: an ordered list of five items with authored aliases.
    fn list(mut self) -> Self {
        self.call("/practice/lesson/start", &[("starter", "rosary-joyful")]);
        self
    }
    fn card_at(&self, index: usize) -> String {
        self.app
            .practice
            .cards
            .values()
            .find(|c| c.unit_index == index)
            .unwrap()
            .id
            .clone()
    }
}

const CREED: &str = "I believe in one God,\nthe Father almighty,\nmaker of heaven and earth,\nof all things visible and invisible.\n";
const MYSTERIES: &str = "Annunciation\nVisitation\nNativity\nPresentation\nFinding in the Temple";

/// What an answer left for Jev looks like before any Verdict.
fn assert_pending(o: &Owner, aid: &str, card: &str, before: &study::Card) {
    let a = o.attempt(aid);
    let judgment = a.judgment.as_ref().unwrap();
    assert_eq!(judgment.policy, "recall-v1");
    assert!(judgment.decided.is_none() && a.effects.is_empty());
    assert_eq!(a.verdict(), None);
    assert_eq!(a.authority(), None);
    assert_eq!(a.grade.verdict, Verdict::Again, "the local comparison");
    assert_eq!(judgment.targets[0].revision, before.revision);
    assert_eq!(judgment.targets[0].before, before.state);
    assert_eq!(o.card(card).state, before.state, "no schedule effect");
    assert_eq!(o.card(card).revision, before.revision + 1);
}

// --- Off is unchanged -------------------------------------------------------

/// The same requests, with and without every part of the gate that can be
/// missing. `drive` returns the notebook it leaves.
fn off_matrix(drive: fn(Owner) -> Owner) {
    let plain = drive(Owner::off());
    assert!(!persistence::judged(&plain.app));
    // Capable, switch off.
    let mut capable = Owner::off();
    capable.jev = true;
    let capable = drive(capable);
    assert_eq!(capable.app, plain.app);
    // Switch on, not capable: the only difference is the stored switch.
    let mut switched = Owner::off();
    switched.app.preferences.paid_judgment = true;
    let mut switched = drive(switched);
    assert_eq!(switched.archive().format, JUDGED);
    switched.app.preferences.paid_judgment = false;
    assert_eq!(switched.app, plain.app);
    assert_eq!(
        switched.archive().encode().unwrap(),
        plain.archive().encode().unwrap()
    );
}
fn drive_mapping(o: Owner) -> Owner {
    let mut o = o.mapping();
    o.answer("test the bilge pump");
    let card = o.target();
    o.next_card(&card);
    o.answer("check the bilge pump");
    o
}
fn drive_text(o: Owner) -> Owner {
    let mut o = o.text();
    o.answer("I believe in a single God");
    let pid = o.held().prompt.id;
    o.call(
        "/practice/lesson/next",
        &[("after", &pid), ("mode", "retry")],
    );
    o.answer("I believe in one God");
    o
}
fn drive_list(o: Owner) -> Owner {
    let mut o = o.list();
    o.answer("Annunciation\nVisitation\nThe birth of Christ\nPresentation\nFinding in the Temple");
    o
}

#[test]
fn off_is_unchanged_whichever_part_of_the_gate_is_missing() {
    off_matrix(drive_mapping);
    off_matrix(drive_text);
    off_matrix(drive_list);
}

#[test]
fn a_token_without_paid_permission_is_graded_exactly_as_off() {
    for drive in [drive_mapping, drive_text, drive_list] {
        let off = drive(Owner::off().with_token(false));
        // Capable and switched on: only the paid permission is missing.
        let mut unpaid = drive(Owner::new().with_token(false));
        assert!(unpaid.app.practice.checks.is_empty() && unpaid.app.spend.is_empty());
        assert!(unpaid.claim().is_none());
        assert_eq!(unpaid.app.practice.attempts, off.app.practice.attempts);
        assert_eq!(unpaid.app.practice, off.app.practice);
        assert!(
            unpaid
                .app
                .practice
                .attempts
                .values()
                .all(|a| a.judgment.is_none() && !a.effects.is_empty())
        );
        // The same token may not ask for a check either, and nothing is written.
        let before = unpaid.app.clone();
        let aid = before.practice.attempts.keys().next().unwrap().clone();
        assert_eq!(unpaid.check_again(&aid).unwrap_err().status, 403);
        assert_eq!(unpaid.app, before);
    }
}

#[test]
fn the_existing_entry_points_never_leave_an_answer_for_jev() {
    // `engine::mutate` and `mutate_as` pass false whatever is stored.
    let mut o = Owner::off();
    o.app.preferences.paid_judgment = true;
    let mut o = o.mapping();
    let pid = o.held().prompt.id;
    let f = o.fields("judgment-existing-quiz-01", &[("prompt_id", &pid)]);
    engine::mutate(&mut o.app, "/practice/quiz", &f, NOW, "existing-quiz").unwrap();
    let pid = o.held().prompt.id;
    let f = o.fields(
        "judgment-existing-answer-01",
        &[("prompt_id", &pid), ("answer", "test the bilge pump")],
    );
    let aid = engine::mutate_as(
        &mut o.app,
        "/practice/answer",
        &f,
        NOW,
        "existing-answer",
        MutationActor::Browser,
    )
    .unwrap()
    .entity_id
    .unwrap();
    let a = o.attempt(&aid);
    assert!(a.judgment.is_none() && a.effects.len() == 1);
    assert_eq!(a.verdict(), Some(Verdict::Again));
    assert!(o.app.practice.checks.is_empty());
    // Nor do they offer a check: nothing is written.
    let before = o.app.clone();
    let f = o.fields("judgment-existing-check-01", &[("attempt_id", &aid)]);
    assert_eq!(
        engine::mutate(&mut o.app, "/practice/check", &f, NOW, "existing-check")
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(o.app, before);
}

// --- The pending Attempt and a decision -------------------------------------

#[test]
fn unlisted_valid_wording_is_accepted_by_jev_and_keeps_the_local_comparison() {
    let mut o = Owner::new().mapping();
    let card = o.target();
    let before = o.card(&card).clone();
    let aid = o.answer("test the bilge pump");
    assert_pending(&o, &aid, &card, &before);
    assert_eq!(o.held().attempt_id.as_deref(), Some(aid.as_str()));
    let check = o.check(&aid).clone();
    assert_eq!(check.status, CheckStatus::Queued);
    assert!(check.lease.is_none() && check.reason.is_none());
    assert!(o.app.spend.is_empty(), "nothing is reserved until claimed");
    assert_eq!(o.state(&aid), JudgmentState::Checking);
    let wire = o.wire(&aid);
    assert_eq!(
        wire["judgment"],
        json!({"state":"checking","verdict":null,"authority":null,"reason":null,"check_id":check.id})
    );
    assert_eq!(wire["mapping_feedback"]["kind"], "ungraded");

    let work = o.claim().unwrap();
    assert_eq!((work.kind.as_str(), &work.id), ("check", &check.id));
    let sent = o.check(&aid).clone();
    assert_eq!(sent.status, CheckStatus::Sent);
    assert_eq!(sent.lease.as_deref(), Some(work.lease.as_str()));
    let reserved = o.app.spend.last().unwrap();
    assert_eq!(
        (reserved.id.as_str(), reserved.work_id.as_str()),
        (work.lease.as_str(), check.id.as_str())
    );
    assert_eq!(reserved.reserved_micros, RESERVATION);
    assert_eq!(
        (reserved.status.as_str(), reserved.cost_micros),
        ("unknown", None)
    );
    assert_eq!(reserved.purpose.as_deref(), Some("grade.recall"));
    assert_eq!(reserved.model.as_deref(), Some("synthetic-jev"));
    assert_pending(&o, &aid, &card, &before);
    assert!(o.claim().is_none(), "one claim");

    let raw = accept();
    o.finish(&work, raw.clone()).unwrap();
    let a = o.attempt(&aid).clone();
    let decided = a.judgment.as_ref().unwrap().decided.clone().unwrap();
    assert_eq!(a.verdict(), Some(Verdict::Good));
    assert_eq!(decided.authority.name, "jev");
    assert_eq!(
        decided.authority.grader_version,
        format!("recall-v1;model={MODEL}")
    );
    assert_eq!(a.authority(), Some(&decided.authority));
    assert_eq!(decided.check_id, check.id);
    assert_eq!(decided.sequence, o.app.revision);
    // `grade` is still the local comparison, never rewritten.
    assert_eq!(a.grade.authority.name, "exact-v2");
    assert_eq!(a.grade.verdict, Verdict::Again);
    let effect = &a.effects[0];
    assert_eq!(effect.verdict, Verdict::Good);
    assert!(effect.scheduled && !effect.superseded && !effect.assisted);
    assert_eq!(effect.revision_before, before.revision + 1);
    assert_eq!(effect.before, before.state);
    assert_eq!(o.card(&card).state, effect.after);
    assert_eq!(o.card(&card).revision, before.revision + 2);
    assert!(o.card(&card).state.schedule.due_ms > a.created_ms);
    assert_eq!(o.check(&aid).status, CheckStatus::Accepted);
    let settled = o.app.spend.last().unwrap();
    assert_eq!(
        (settled.status.as_str(), settled.cost_micros),
        ("known", Some(12_000))
    );
    assert_eq!(settled.raw_response.as_deref(), Some(raw.as_str()));
    assert_eq!(settled.response_outcome.as_deref(), Some("received"));
    assert_eq!(o.state(&aid), JudgmentState::Decided);
    let wire = o.wire(&aid);
    assert_eq!(wire["judgment"]["state"], "decided");
    assert_eq!(wire["judgment"]["verdict"], "Good");
    assert_eq!(wire["judgment"]["authority"]["name"], "jev");
    assert_eq!(wire["attempt"]["grade"]["verdict"], "Again");
    assert_eq!(wire["mapping_feedback"]["kind"], "correct");
    assert_eq!(
        spend::report(&o.app, spend::SpendBy::Purpose)
            .unwrap()
            .entries[0]
            .work_kind,
        "study_check"
    );
}

#[test]
fn a_different_identity_is_rejected_and_a_risky_accept_is_not_a_grade() {
    let mut o = Owner::new().mapping();
    let card = o.target();
    let before = o.card(&card).clone();
    // Another key's value.
    let aid = o.answer("sound the horn");
    o.judge(reject());
    let a = o.attempt(&aid).clone();
    assert_eq!(a.verdict(), Some(Verdict::Again));
    assert_eq!(a.authority().unwrap().name, "jev");
    assert_eq!(o.check(&aid).status, CheckStatus::Rejected);
    assert_eq!(a.effects[0].verdict, Verdict::Again);
    assert_eq!(o.card(&card).state, a.effects[0].after);
    assert_eq!(
        o.card(&card).state.schedule.lapses,
        before.state.schedule.lapses
    );
    assert!(o.card(&card).state.schedule.last_review_ms.is_some());
    let wire = o.wire(&aid);
    assert_eq!(wire["mapping_feedback"]["kind"], "wrong_key");
    assert_eq!(wire["mapping_feedback"]["answered_key"], "In fog");

    // An accept that may name a different thing is no Verdict at all.
    o.next_card(&card);
    let scheduled = o.card(&card).clone();
    let aid = o.answer("inspect the pump in the bilge");
    o.judge(response("accept", 0.97, 0.6, 0.0));
    assert_eq!(o.attempt(&aid).verdict(), None);
    assert!(o.attempt(&aid).effects.is_empty());
    assert_eq!(o.check(&aid).status, CheckStatus::Unsure);
    assert_eq!(o.state(&aid), JudgmentState::Ungraded);
    assert_eq!(o.card(&card).state, scheduled.state);
    assert_eq!(o.card(&card).revision, scheduled.revision + 1);
    // A weak reject is not a miss either.
    o.check_again(&aid).unwrap();
    o.judge(response("reject", 0.8, 0.9, 0.0));
    assert_eq!(o.attempt(&aid).verdict(), None);
    assert_eq!(o.check(&aid).status, CheckStatus::Unsure);
}

#[test]
fn a_list_omission_duplicate_and_swap_keep_their_local_unit_verdicts_on_reject() {
    for answer in [
        // The third item is omitted.
        "Annunciation\nVisitation\nPresentation\nFinding in the Temple",
        // The first item is given twice.
        "Annunciation\nAnnunciation\nVisitation\nNativity\nPresentation\nFinding in the Temple",
        // The first two items are swapped.
        "Visitation\nAnnunciation\nNativity\nPresentation\nFinding in the Temple",
    ] {
        let mut o = Owner::new().list();
        let aid = o.answer(answer);
        let a = o.attempt(&aid).clone();
        assert_eq!(a.verdict(), None);
        assert_eq!(a.judgment.as_ref().unwrap().targets.len(), 5);
        assert_eq!(a.grade.authority.name, "ordered-v1");
        let work = o.claim().unwrap();
        let state = &work.request["state"];
        assert_eq!(state["expected_answer"], MYSTERIES);
        assert_eq!(state["learner_answer"], answer);
        assert_eq!(state["prompt"], "Joyful Mysteries");
        assert_eq!(state["scope"], json!({"kind":"whole"}));
        assert_eq!(
            state["accepted_variants"][2],
            json!({"position":3,"item":"Nativity","aliases":["The Nativity","Nativity of Jesus","Birth of Jesus"]})
        );
        assert_eq!(state["accepted_variants"].as_array().unwrap().len(), 5);
        assert!(
            state["objective"]
                .as_str()
                .unwrap()
                .contains("Reject missing, repeated, extra, misordered or different items")
        );
        o.finish(&work, reject()).unwrap();
        let a = o.attempt(&aid).clone();
        let decided = a.judgment.as_ref().unwrap().decided.as_ref().unwrap();
        assert_eq!(a.verdict(), Some(Verdict::Again));
        // The Units the local matcher found mismatched are Again; the rest Good.
        assert_eq!(decided.units, a.grade.units);
        assert!(decided.units.iter().any(|u| u.verdict == Verdict::Again));
        assert!(decided.units.iter().any(|u| u.verdict == Verdict::Good));
        for (effect, unit) in a.effects.iter().zip(&decided.units) {
            assert_eq!(effect.verdict, unit.verdict);
            assert_eq!(o.card(&effect.card_id).state, effect.after);
        }
    }
    // An unlisted naming of one item, accepted: every Unit is Good.
    let mut o = Owner::new().list();
    let aid = o.answer(
        "Annunciation\nVisitation\nThe birth of Christ\nPresentation\nFinding in the Temple",
    );
    assert_eq!(o.attempt(&aid).grade.units[2].verdict, Verdict::Again);
    o.judge(accept());
    let a = o.attempt(&aid);
    assert_eq!(a.verdict(), Some(Verdict::Good));
    assert!(
        a.effects
            .iter()
            .all(|e| e.verdict == Verdict::Good && e.scheduled)
    );
}

// --- What is never left for Jev ---------------------------------------------

/// The answer was graded by Scry at once and nothing was queued.
fn assert_local(o: &mut Owner, aid: &str, verdict: Verdict) {
    let a = o.attempt(aid).clone();
    assert!(a.judgment.is_none() && !a.effects.is_empty());
    assert_eq!(a.verdict(), Some(verdict));
    assert_eq!(a.verdict(), Some(a.grade.verdict));
    assert!(o.app.practice.checks.is_empty() && o.app.spend.is_empty());
    assert!(o.claim().is_none(), "zero transmissions");
    assert!(o.wire(aid).get("judgment").is_none());
}

#[test]
fn an_exact_or_alias_match_is_good_at_once() {
    let mut o = Owner::new().mapping();
    let aid = o.answer("Check the  bilge-pump");
    assert_local(&mut o, &aid, Verdict::Good);
    let mut o = Owner::new().list();
    let aid = o.answer(
        "The Annunciation\nVisitation\nBirth of Jesus\nPresentation\nThe Finding in the Temple",
    );
    assert_local(&mut o, &aid, Verdict::Good);
    // A Slip counts as recalled.
    let mut o = Owner::new().text();
    let aid = o.answer("I beleive in one God");
    assert_local(&mut o, &aid, Verdict::Good);
}

#[test]
fn a_spelling_strict_mismatch_is_again_at_once_and_never_sent() {
    let mut o = Owner::new();
    o.call("/practice/nato/start", &[]);
    let aid = o.answer("Alpha");
    assert_local(&mut o, &aid, Verdict::Again);
    // A Spelling-strict text and a Spelling-strict starter mapping likewise.
    let mut o = Owner::new();
    let vid = o.add_text("Strict synthetic line", "Juliett keeps the lamp.\n", true);
    o.call("/practice/lesson/start", &[("version_id", &vid)]);
    let aid = o.answer("Juliet keeps the lamp");
    assert_local(&mut o, &aid, Verdict::Again);
    let mut o = Owner::new();
    o.call("/practice/lesson/start", &[("starter", "baby-soothing")]);
    let aid = o.answer("wrap");
    assert_local(&mut o, &aid, Verdict::Again);
}

#[test]
fn an_empty_answer_is_again_at_once_and_never_sent() {
    for empty in ["", "   ", " \n\t "] {
        let mut o = Owner::new().text();
        let aid = o.answer(empty);
        assert_local(&mut o, &aid, Verdict::Again);
    }
    let mut o = Owner::new().mapping();
    let aid = o.answer(" ");
    assert_local(&mut o, &aid, Verdict::Again);
}

#[test]
fn an_answer_and_expected_content_over_32_kib_are_graded_locally() {
    // 600 words of 27 bytes: the phrase alone is 16.8 kB.
    let phrase = (0..600)
        .map(|i| format!("synthetic{i:04}harbourlantern"))
        .collect::<Vec<_>>()
        .join(" ");
    let text = format!("{phrase}\n");
    let start = |o: &mut Owner| {
        let vid = o.add_text("Synthetic long line", &text, false);
        o.call("/practice/lesson/start", &[("version_id", &vid)]);
    };
    // One wrong word in a full-length answer: 33.6 kB with the expected text.
    let long = phrase.replacen("synthetic0000", "synthetic9999", 1);
    assert!(long.len() + text.len() > generation::RECALL_LIMIT);
    let mut o = Owner::new();
    start(&mut o);
    let aid = o.answer(&long);
    assert_local(&mut o, &aid, Verdict::Again);
    // A short wrong answer to the same phrase fits and is left for Jev.
    let mut o = Owner::new();
    start(&mut o);
    let aid = o.answer("a different harbour lantern");
    assert_eq!(o.attempt(&aid).verdict(), None);
    let work = o.claim().unwrap();
    let state = &work.request["state"];
    assert_eq!(state["expected_answer"], text);
    assert!(
        state["expected_answer"].as_str().unwrap().len()
            + state["learner_answer"].as_str().unwrap().len()
            <= generation::RECALL_LIMIT
    );
}

// --- The request ------------------------------------------------------------

#[test]
fn a_mapping_request_carries_the_negation_clause_and_nothing_else_of_the_notebook() {
    let mut o = Owner::new().with_token(true).mapping();
    // Answer the first two pairs, then reach the safety-bearing third.
    for exact in ["check the bilge pump", "sound the horn"] {
        let aid = o.answer(exact);
        assert!(o.attempt(&aid).judgment.is_none());
        let pid = o.held().prompt.id;
        o.call("/practice/next", &[("after", &pid)]);
    }
    assert_eq!(o.held().targets[0].unit_index, 2);
    let card = o.target();
    // The negated practice.
    let aid = o.answer("lose sight of them");
    assert_eq!(o.attempt(&aid).verdict(), None);
    let work = o.claim().unwrap();
    let request = &work.request;
    assert_eq!(request["model"], "synthetic-jev");
    assert_eq!(
        request.as_object().unwrap().keys().collect::<Vec<_>>(),
        ["model", "questions", "state"]
    );
    let state = &request["state"];
    assert_eq!(
        state,
        &json!({
            "prompt":"A person overboard",
            "scope":{"kind":"unit","position":3},
            "objective":"Recall the value that expected_answer gives for the key in prompt. Reject another key's value, an incomplete value, or a negation or reversal of the expected practice.",
            "expected_answer":"never lose sight of them",
            "accepted_variants":[],
            "learner_answer":"lose sight of them"
        })
    );
    // The same keys and labels as the earlier flow, so its parser is reused.
    assert_eq!(
        request["questions"]
            .as_object()
            .unwrap()
            .keys()
            .collect::<Vec<_>>(),
        ["identity", "injection", "verdict"]
    );
    assert_eq!(
        request["questions"]["verdict"]["criteria"]
            .as_object()
            .unwrap()
            .keys()
            .collect::<Vec<_>>(),
        ["accept", "reject", "unsure"]
    );
    // No other Unit, identifier, schedule, history, assistance or token.
    let sent = request.to_string();
    let a = o.attempt(&aid);
    for absent in [
        "bilge",
        "horn",
        a.id.as_str(),
        a.prompt.id.as_str(),
        a.version_id.as_str(),
        a.prompt.study_id.as_str(),
        card.as_str(),
        o.token.as_deref().unwrap(),
        "Judgment local agent",
        "due_ms",
        "assisted",
        "Synthetic harbour practice",
    ] {
        assert!(!sent.contains(absent), "the request carries {absent}");
    }
    o.finish(&work, reject()).unwrap();
    assert_eq!(o.attempt(&aid).verdict(), Some(Verdict::Again));
    assert_eq!(o.check(&aid).acting_token, o.token);
}

#[test]
fn a_text_request_carries_its_own_scope_only() {
    let mut o = Owner::new().text();
    let aid = o.answer("I believe in a single God");
    let work = o.claim().unwrap();
    let state = &work.request["state"];
    assert_eq!(state["prompt"], "Synthetic Creed, phrase 1");
    assert_eq!(state["scope"], json!({"kind":"unit","position":1}));
    assert_eq!(state["expected_answer"], "I believe in one God,\n");
    assert_eq!(state["accepted_variants"], json!([]));
    assert!(
        state["objective"]
            .as_str()
            .unwrap()
            .contains("Reject omitted, added, substituted or paraphrased words")
    );
    assert!(!work.request.to_string().contains("almighty"));
    o.finish(&work, reject()).unwrap();
    // A joined passage sends its Units joined, and a per-Unit reject keeps
    // the phrases the comparison matched as Good.
    assert_eq!(o.attempt(&aid).verdict(), Some(Verdict::Again));
    let pid = o.held().prompt.id;
    o.call(
        "/practice/lesson/next",
        &[("after", &pid), ("mode", "whole")],
    );
    let aid = o.answer(
        "I believe in one God, the Father almighty, creator of heaven and earth, of all things visible and invisible.",
    );
    let work = o.claim().unwrap();
    assert_eq!(
        work.request["state"]["prompt"],
        "Synthetic Creed, whole text"
    );
    assert_eq!(work.request["state"]["expected_answer"], CREED);
    o.finish(&work, reject()).unwrap();
    let a = o.attempt(&aid);
    let units = &a.judgment.as_ref().unwrap().decided.as_ref().unwrap().units;
    assert_eq!(
        units.iter().map(|u| u.verdict).collect::<Vec<_>>(),
        [Verdict::Good, Verdict::Good, Verdict::Again, Verdict::Good]
    );
    assert_eq!(a.verdict(), Some(Verdict::Again));
}

// --- Assistance -------------------------------------------------------------

#[test]
fn an_assisted_retry_accepted_by_jev_is_good_assisted_and_unscheduled() {
    let mut o = Owner::new().text();
    let card = o.target();
    let first = o.answer("I believe in a single God");
    o.judge(reject());
    assert_eq!(o.attempt(&first).verdict(), Some(Verdict::Again));
    let missed = o.card(&card).clone();
    // The guided retry shows the phrase, then asks for it from memory.
    let pid = o.held().prompt.id;
    o.call(
        "/practice/lesson/next",
        &[("after", &pid), ("mode", "retry")],
    );
    assert_eq!(o.held().phase, Some(StudyPhase::Study));
    let aid = o.answer("I do believe in one God");
    let a = o.attempt(&aid).clone();
    assert!(a.assisted && a.revealed && a.verdict().is_none());
    o.judge(accept());
    let a = o.attempt(&aid).clone();
    assert_eq!(a.verdict(), Some(Verdict::Good));
    assert!(a.assisted);
    let effect = &a.effects[0];
    assert!(effect.assisted && !effect.scheduled && !effect.superseded);
    assert_eq!(effect.verdict, Verdict::Good);
    assert_eq!(o.card(&card).state.schedule, missed.state.schedule);
    assert_eq!(o.card(&card).revision, missed.revision + 2);
}

// --- Responses that are not a Verdict ---------------------------------------

#[test]
fn malformed_unsure_injected_and_oversized_responses_leave_the_answer_ungraded() {
    let oversized = "x".repeat(generation::RESPONSE_LIMIT + 1);
    for (raw, status) in [
        ("not json".to_string(), CheckStatus::Failed),
        ("{\"answers\":{}}".to_string(), CheckStatus::Failed),
        (
            json!({"answers":{"verdict":{"choice":"maybe"},"identity":{"noul":0.0},"injection":{"noul":0.0}}})
                .to_string(),
            CheckStatus::Failed,
        ),
        (unsure(), CheckStatus::Unsure),
        // A confident accept beside an instruction addressed to the judge.
        (response("accept", 0.99, 0.0, 0.4), CheckStatus::Unsure),
        (oversized, CheckStatus::Failed),
    ] {
        let mut o = Owner::new().mapping();
        let card = o.target();
        let before = o.card(&card).clone();
        let aid = o.answer("test the bilge pump");
        let pending = o.attempt(&aid).clone();
        let work = o.claim().unwrap();
        let result = o.finish(&work, raw.clone());
        assert_eq!(result.is_err(), raw.len() > generation::RESPONSE_LIMIT);
        let check = o.check(&aid).clone();
        assert_eq!(check.status, status, "{:.40}", raw);
        assert!(check.reason.is_some());
        assert_eq!(o.attempt(&aid), &pending, "the Attempt is untouched");
        assert_eq!(o.card(&card).state, before.state);
        assert_eq!(o.card(&card).revision, before.revision + 1);
        assert_eq!(o.state(&aid), JudgmentState::Ungraded);
        // The paid outcome is ledgered whatever it said.
        let spent = o.app.spend.last().unwrap();
        assert_eq!((spent.status.as_str(), spent.cost_micros), ("known", Some(12_000)));
        assert_eq!(spent.response_bytes, Some(raw.len()));
        assert_eq!(spent.response_outcome.as_deref(), Some("received"));
        let wire = o.wire(&aid);
        assert_eq!(wire["judgment"]["state"], "ungraded");
        assert_eq!(wire["judgment"]["verdict"], Value::Null);
        assert_eq!(wire["judgment"]["reason"], json!(check.reason));
        // The same response delivered twice changes nothing.
        let settled = o.app.clone();
        o.finish(&work, raw).unwrap();
        assert_eq!(o.app, settled);
    }
}

#[test]
fn a_refused_http_response_and_a_lost_send_leave_the_answer_ungraded() {
    // A received non-success response, even one shaped like an accept.
    let mut o = Owner::new().mapping();
    let aid = o.answer("test the bilge pump");
    let pending = o.attempt(&aid).clone();
    let work = o.claim().unwrap();
    engine::fail_received_work(
        &mut o.app,
        &work,
        o.now,
        accept(),
        MODEL.into(),
        None,
        "The check was refused (HTTP 429). Your answer is saved and not graded.",
    )
    .unwrap();
    o.sound();
    assert_eq!(o.check(&aid).status, CheckStatus::Failed);
    assert_eq!(o.attempt(&aid), &pending);
    let spent = o.app.spend.last().unwrap();
    assert_eq!(spent.response_outcome.as_deref(), Some("rejected"));
    assert_eq!(
        (spent.status.as_str(), spent.cost_micros),
        ("unknown", None)
    );
    assert_eq!(o.app.spent(o.now), RESERVATION, "the reservation is kept");

    // A send whose outcome is unknown keeps its reservation too.
    let mut o = Owner::new().mapping();
    let aid = o.answer("test the bilge pump");
    let pending = o.attempt(&aid).clone();
    let work = o.claim().unwrap();
    engine::fail_work(&mut o.app, &work, "The check timed out.", true, o.now).unwrap();
    o.sound();
    assert_eq!(o.check(&aid).status, CheckStatus::Unknown);
    assert_eq!(
        o.check(&aid).reason.as_deref(),
        Some("The check timed out.")
    );
    assert_eq!(o.attempt(&aid), &pending);
    assert_eq!(o.app.spent(o.now), RESERVATION);
    // The same failure reported twice is refused, not repeated.
    assert_eq!(
        engine::fail_work(&mut o.app, &work, "again", true, o.now)
            .unwrap_err()
            .status,
        409
    );
    // A response that arrives after all is ledgered only.
    o.finish(&work, accept()).unwrap_err();
    assert_eq!(o.check(&aid).status, CheckStatus::Unknown);
    assert_eq!(o.attempt(&aid), &pending);
    assert_eq!(o.app.spend.last().unwrap().cost_micros, Some(12_000));
}

#[test]
fn an_interrupted_send_becomes_unknown_once_and_is_never_sent_again() {
    let mut o = Owner::new().mapping();
    let aid = o.answer("test the bilge pump");
    let pending = o.attempt(&aid).clone();
    let work = o.claim().unwrap();
    o.now += 89_000;
    assert!(!engine::recover_grading(&mut o.app, o.now));
    assert_eq!(o.check(&aid).status, CheckStatus::Sent);
    o.now += 1_000;
    assert!(engine::recover_grading(&mut o.app, o.now));
    o.sound();
    assert_eq!(o.check(&aid).status, CheckStatus::Unknown);
    assert!(!engine::recover_grading(&mut o.app, o.now));
    assert!(o.claim().is_none(), "no second send");
    assert_eq!(o.app.spend.len(), 1);
    assert_eq!(o.app.spent(o.now), RESERVATION);
    assert_eq!(o.attempt(&aid), &pending);
    assert_eq!(o.state(&aid), JudgmentState::Ungraded);
    // The late response settles the ledger and decides nothing.
    assert_eq!(o.finish(&work, accept()).unwrap_err().status, 409);
    assert_eq!(o.check(&aid).status, CheckStatus::Unknown);
    assert_eq!(o.attempt(&aid).verdict(), None);
    assert_eq!(o.app.spend[0].cost_micros, Some(12_000));
    // The Owner may still ask again, deliberately.
    o.check_again(&aid).unwrap();
    o.judge(accept());
    assert_eq!(o.attempt(&aid).verdict(), Some(Verdict::Good));
}

// --- The cap ----------------------------------------------------------------

/// Seven checks with unknown outcomes hold the whole 3.50 allowance.
fn exhaust_allowance(o: &mut Owner) {
    let card = o.target();
    for round in 0..7 {
        let aid = o.answer(&format!("test the bilge pump {round}"));
        let work = o.claim().unwrap();
        engine::fail_work(&mut o.app, &work, "The check timed out.", true, o.now).unwrap();
        o.sound();
        assert_eq!(o.check(&aid).status, CheckStatus::Unknown);
        o.next_card(&card);
    }
    assert_eq!(o.app.spent(o.now), DAILY_ALLOWANCE);
}

#[test]
fn a_reached_cap_leaves_the_answer_saved_and_ungraded_with_its_reason() {
    let mut o = Owner::new().mapping();
    exhaust_allowance(&mut o);
    let card = o.target();
    let before = o.card(&card).clone();
    let aid = o.answer("test the bilge pump");
    assert_pending(&o, &aid, &card, &before);
    let check = o.check(&aid).clone();
    assert_eq!(check.status, CheckStatus::Refused);
    assert!(
        check
            .reason
            .as_deref()
            .unwrap()
            .contains("allowance is used")
    );
    assert!(check.lease.is_none());
    assert_eq!(o.state(&aid), JudgmentState::Ungraded);
    assert_eq!(o.app.spend.len(), 7, "nothing reserved");
    assert!(o.claim().is_none(), "zero transmissions");
    assert!(o.page("/recall").contains("allowance is used"));
    // Asking again under the cap is refused with nothing written.
    let saved = o.app.clone();
    assert_eq!(o.check_again(&aid).unwrap_err().status, 429);
    assert_eq!(o.app, saved);
    // Once the allowance has room, a deliberate check succeeds.
    o.now += DAY_MS;
    o.check_again(&aid).unwrap();
    assert_eq!(o.checks(&aid).len(), 2);
    o.judge(accept());
    assert_eq!(o.attempt(&aid).verdict(), Some(Verdict::Good));
    assert_eq!(o.card(&card).revision, before.revision + 2);
}

#[test]
fn a_check_queued_before_the_cap_filled_is_refused_when_claimed() {
    let mut o = Owner::new().text();
    let (first_card, second_card) = (o.card_at(0), o.card_at(1));
    // Six unknown outcomes leave room for one more reservation.
    for round in 0..6 {
        o.answer(&format!("I believe in a single God {round}"));
        let work = o.claim().unwrap();
        engine::fail_work(&mut o.app, &work, "The check timed out.", true, o.now).unwrap();
        o.sound();
        o.next_card(&first_card);
    }
    assert_eq!(o.app.spent(o.now) + RESERVATION, DAILY_ALLOWANCE);
    // Two answers are queued while that room is still there.
    let first = o.answer("I believe in a lone God");
    o.next_card(&second_card);
    let second = o.answer("the Father of might");
    assert_eq!(o.check(&first).status, CheckStatus::Queued);
    assert_eq!(o.check(&second).status, CheckStatus::Queued);
    // The older takes the last reservation; the newer is refused unsent.
    let work = o.claim().unwrap();
    assert_eq!(work.id, o.check(&first).id);
    assert_eq!(o.app.spent(o.now), DAILY_ALLOWANCE);
    assert!(o.claim().is_none());
    let refused = o.check(&second).clone();
    assert_eq!(refused.status, CheckStatus::Refused);
    assert!(refused.lease.is_none());
    assert!(
        refused
            .reason
            .as_deref()
            .unwrap()
            .contains("allowance is used")
    );
    assert_eq!(o.app.spend.len(), 7, "never above the cap");
    assert_eq!(o.attempt(&second).verdict(), None);
    o.finish(&work, accept()).unwrap();
    assert_eq!(o.attempt(&first).verdict(), Some(Verdict::Good));
}

// --- Each action happens once -----------------------------------------------

#[test]
fn replaying_the_answer_or_the_check_returns_the_same_attempt_at_every_state() {
    let mut o = Owner::new().mapping();
    let pid = {
        let pid = o.held().prompt.id;
        o.call("/practice/quiz", &[("prompt_id", &pid)]);
        o.held().prompt.id
    };
    let pairs = [
        ("prompt_id", pid.as_str()),
        ("answer", "test the bilge pump"),
    ];
    let first = o
        .op("judgment-replayed-answer-01", "/practice/answer", &pairs)
        .unwrap();
    let aid = first.entity_id.clone().unwrap();
    let replay = |o: &mut Owner, state: JudgmentState| {
        let saved = o.app.clone();
        let again = o
            .op("judgment-replayed-answer-01", "/practice/answer", &pairs)
            .unwrap();
        assert_eq!(again, first);
        assert_eq!(o.app, saved, "a replay writes nothing");
        assert_eq!(o.app.practice.attempts.len(), 1);
        assert_eq!(o.state(&aid), state);
        // The receipt names the Attempt; the response shows its state now.
        assert_eq!(
            study::wire_response(&o.app, &again).unwrap()["judgment"]["state"],
            json!(state)
        );
        // The same operation with another answer is an explicit conflict.
        let other = [("prompt_id", pid.as_str()), ("answer", "something else")];
        assert_eq!(
            o.op("judgment-replayed-answer-01", "/practice/answer", &other)
                .unwrap_err()
                .status,
            409
        );
        assert_eq!(o.app, saved);
    };
    replay(&mut o, JudgmentState::Checking);
    let work = o.claim().unwrap();
    replay(&mut o, JudgmentState::Checking);
    o.finish(&work, unsure()).unwrap();
    replay(&mut o, JudgmentState::Ungraded);
    // A check operation replays the same way and queues one Check.
    let check = o
        .op(
            "judgment-replayed-check-001",
            "/practice/check",
            &[("attempt_id", &aid)],
        )
        .unwrap();
    assert_eq!(check.entity_id.as_deref(), Some(aid.as_str()));
    let saved = o.app.clone();
    assert_eq!(
        o.op(
            "judgment-replayed-check-001",
            "/practice/check",
            &[("attempt_id", &aid)]
        )
        .unwrap(),
        check
    );
    assert_eq!(o.app, saved);
    assert_eq!(o.checks(&aid).len(), 2);
    replay(&mut o, JudgmentState::Checking);
    o.judge(accept());
    replay(&mut o, JudgmentState::Decided);
    assert_eq!(o.app.spend.len(), 2, "one reservation for each Check sent");
    // A new operation on the answered Prompt is refused: a second tab.
    let saved = o.app.clone();
    assert_eq!(
        o.op("judgment-second-tab-answer", "/practice/answer", &pairs)
            .unwrap_err()
            .status,
        409
    );
    assert_eq!(o.app, saved);
}

#[test]
fn a_second_check_while_one_is_queued_or_sent_is_refused() {
    let mut o = Owner::new().mapping();
    let aid = o.answer("test the bilge pump");
    let saved = o.app.clone();
    assert_eq!(o.check_again(&aid).unwrap_err().status, 409);
    assert_eq!(o.app, saved, "queued: nothing written");
    let work = o.claim().unwrap();
    let saved = o.app.clone();
    assert_eq!(o.check_again(&aid).unwrap_err().status, 409);
    assert_eq!(o.app, saved, "sent: nothing written");
    o.finish(&work, accept()).unwrap();
    // A decided answer has its Verdict once.
    let saved = o.app.clone();
    let refused = o.check_again(&aid).unwrap_err();
    assert_eq!(refused.status, 409);
    assert!(refused.message.contains("already has its Verdict"));
    assert_eq!(o.app, saved);
    assert_eq!(o.checks(&aid).len(), 1);
    // An unknown Attempt, an Attempt Scry graded, and a forged field.
    assert_eq!(o.check_again(&"0".repeat(32)).unwrap_err().status, 404);
    let card = o.target();
    o.next_card(&card);
    let local = o.answer("check the bilge pump");
    assert_eq!(o.check_again(&local).unwrap_err().status, 409);
    assert_eq!(
        o.try_call(
            "/practice/check",
            &[("attempt_id", &aid), ("verdict", "Good")]
        )
        .unwrap_err()
        .status,
        422
    );
}

// --- The late schedule effect -----------------------------------------------

#[test]
fn a_verdict_for_a_card_answered_again_is_recorded_as_superseded() {
    let mut o = Owner::new().mapping();
    let card = o.target();
    let before = o.card(&card).clone();
    let stale = o.answer("test the bilge pump");
    let work = o.claim().unwrap();
    // The provider is still deciding. The Card is served and answered again.
    o.next_card(&card);
    assert_eq!(o.held().phase, Some(StudyPhase::Quiz), "it was answered");
    let newer = o.answer("check the bilge pump");
    let newer = o.attempt(&newer).clone();
    assert_eq!(newer.verdict(), Some(Verdict::Good));
    assert_eq!(newer.effects[0].revision_before, before.revision + 1);
    let answered = o.card(&card).clone();
    assert_eq!(answered.state, newer.effects[0].after);
    // Now the response arrives.
    o.finish(&work, reject()).unwrap();
    let a = o.attempt(&stale).clone();
    assert_eq!(a.verdict(), Some(Verdict::Again), "the Verdict is recorded");
    assert_eq!(o.check(&stale).status, CheckStatus::Rejected);
    let effect = &a.effects[0];
    assert!(effect.superseded && !effect.scheduled && !effect.extra_practice);
    assert_eq!(effect.verdict, Verdict::Again);
    assert_eq!(effect.before, answered.state);
    assert_eq!(effect.after, answered.state);
    assert_eq!(effect.revision_before, answered.revision);
    // The Card equals the newer Attempt's result, untouched by the decision.
    assert_eq!(o.card(&card), &answered);
    assert_eq!(o.card(&card).state, newer.effects[0].after);
    let history = o.page("/history");
    assert!(history.contains("data-schedule-effect=\"superseded\""));
    assert!(history.contains("A newer answer was already recorded · schedule unchanged"));
}

#[test]
fn a_reveal_or_another_pending_answer_also_supersedes_and_each_is_decided_once() {
    // A reveal on the same Card.
    let mut o = Owner::new().mapping();
    let card = o.target();
    let stale = o.answer("test the bilge pump");
    let work = o.claim().unwrap();
    o.next_card(&card);
    let pid = o.held().prompt.id;
    o.call("/practice/reveal", &[("prompt_id", &pid)]);
    let revealed = o.card(&card).clone();
    o.finish(&work, accept()).unwrap();
    assert_eq!(o.attempt(&stale).verdict(), Some(Verdict::Good));
    assert!(o.attempt(&stale).effects[0].superseded);
    assert_eq!(o.card(&card), &revealed);

    // Two answers pending on one Card: the newer decides the schedule.
    let mut o = Owner::new().mapping();
    let card = o.target();
    let before = o.card(&card).clone();
    let older = o.answer("test the bilge pump");
    o.next_card(&card);
    let newer = o.answer("look over the bilge pump");
    assert_eq!(o.card(&card).revision, before.revision + 2);
    assert_eq!(o.card(&card).state, before.state);
    o.judge(accept());
    assert!(o.attempt(&older).effects[0].superseded);
    assert_eq!(o.card(&card).revision, before.revision + 2);
    o.judge(accept());
    let effect = &o.attempt(&newer).effects[0];
    assert!(!effect.superseded && effect.scheduled);
    assert_eq!(o.card(&card).state, effect.after);
    assert_eq!(o.card(&card).revision, before.revision + 3);
}

#[test]
fn a_prompt_held_unanswered_for_the_card_stays_answerable_after_the_verdict() {
    let mut o = Owner::new().mapping();
    let card = o.target();
    let aid = o.answer("test the bilge pump");
    // Next serves the still-due Card again; it is held, not yet answered.
    o.next_card(&card);
    assert!(o.held().attempt_id.is_none());
    o.judge(accept());
    let effect = o.attempt(&aid).effects[0].clone();
    assert!(effect.scheduled && !effect.superseded, "no newer Attempt");
    assert_eq!(o.held().targets[0].before, effect.after);
    assert_eq!(o.held().targets[0].revision, o.card(&card).revision);
    // Answering it now is extra practice on the schedule Jev's Verdict set.
    let later = o.answer("check the bilge pump");
    let later = o.attempt(&later).clone();
    assert_eq!(later.verdict(), Some(Verdict::Good));
    assert!(later.effects[0].extra_practice && !later.effects[0].scheduled);
    assert_eq!(o.card(&card).state.schedule, effect.after.schedule);
}

#[test]
fn a_recitation_schedules_only_the_cards_no_newer_attempt_touched() {
    let mut o = Owner::new().text();
    // Practise every phrase so the whole text can be recited.
    for phrase in CREED.lines() {
        o.answer(phrase);
        let pid = o.held().prompt.id;
        o.call(
            "/practice/lesson/next",
            &[("after", &pid), ("mode", "phrases")],
        );
    }
    assert_eq!(o.held().targets[0].unit_index, 0);
    let first = o.answer("I believe in one God");
    assert!(o.attempt(&first).judgment.is_none());
    let pid = o.held().prompt.id;
    o.call(
        "/practice/lesson/next",
        &[("after", &pid), ("mode", "whole")],
    );
    o.now += 2 * DAY_MS;
    let whole = o.answer(
        "I believe in one God, the Father almighty, creator of heaven and earth, of all things visible and invisible.",
    );
    let work = o.claim().unwrap();
    // Phrase 2 is answered again before the response.
    let second = o.card_at(1);
    o.next_card(&second);
    o.answer("the Father almighty");
    let touched = o.card(&second).clone();
    o.finish(&work, accept()).unwrap();
    let a = o.attempt(&whole).clone();
    assert_eq!(a.verdict(), Some(Verdict::Good));
    assert_eq!(
        a.effects.iter().map(|e| e.superseded).collect::<Vec<_>>(),
        [false, true, false, false]
    );
    assert_eq!(o.card(&second), &touched);
    for effect in a.effects.iter().filter(|e| !e.superseded) {
        assert_eq!(o.card(&effect.card_id).state, effect.after);
        assert_eq!(effect.verdict, Verdict::Good);
    }
}

// --- Check again ------------------------------------------------------------

#[test]
fn check_again_is_a_new_check_of_the_same_attempt_and_stops_at_three() {
    let mut o = Owner::new().mapping();
    let card = o.target();
    let before = o.card(&card).clone();
    let aid = o.answer("test the bilge pump");
    let saved_answer = o.attempt(&aid).clone();
    o.judge(unsure());
    for expected in 2..=3 {
        let result = o.check_again(&aid).unwrap();
        assert_eq!(result.entity_id.as_deref(), Some(aid.as_str()));
        assert_eq!(result.data, Some(json!({"attempt_id": aid})));
        assert_eq!(o.checks(&aid).len(), expected);
        assert_eq!(o.check(&aid).status, CheckStatus::Queued);
        assert_eq!(o.app.practice.attempts.len(), 1, "never a second Attempt");
        assert_eq!(o.attempt(&aid), &saved_answer);
        assert_eq!(
            o.card(&card).revision,
            before.revision + 1,
            "no Card change"
        );
        o.judge(unsure());
    }
    // The fourth request is refused plainly and writes nothing.
    let saved = o.app.clone();
    let refused = o.check_again(&aid).unwrap_err();
    assert_eq!(refused.status, 422);
    assert!(refused.message.contains("Three checks are saved"));
    assert_eq!(o.app, saved);
    assert_eq!(o.app.spend.len(), 3);
    assert!(
        o.page("/recall")
            .contains("Three checks are saved for this answer")
    );
    assert!(!o.page("/recall").contains("action=\"/practice/check\""));
}

#[test]
fn a_check_resolved_on_the_third_try_gives_the_verdict_once() {
    let mut o = Owner::new().mapping();
    let card = o.target();
    let aid = o.answer("test the bilge pump");
    o.judge("not json".into());
    o.check_again(&aid).unwrap();
    o.judge(unsure());
    o.check_again(&aid).unwrap();
    o.judge(accept());
    let a = o.attempt(&aid);
    assert_eq!(a.verdict(), Some(Verdict::Good));
    assert_eq!(
        o.checks(&aid).iter().map(|c| c.status).collect::<Vec<_>>(),
        [
            CheckStatus::Failed,
            CheckStatus::Unsure,
            CheckStatus::Accepted
        ]
    );
    assert_eq!(
        a.judgment
            .as_ref()
            .unwrap()
            .decided
            .as_ref()
            .unwrap()
            .check_id,
        o.check(&aid).id
    );
    assert_eq!(o.card(&card).state, a.effects[0].after);
    assert_eq!(o.check_again(&aid).unwrap_err().status, 409);
}

#[test]
fn check_again_is_refused_while_the_switch_is_off_or_scry_cannot_reach_jev() {
    let mut o = Owner::new().mapping();
    let aid = o.answer("test the bilge pump");
    o.judge(unsure());
    let saved = o.app.clone();
    o.jev = false;
    assert_eq!(o.check_again(&aid).unwrap_err().status, 409);
    assert_eq!(o.app, saved);
    o.jev = true;
    o.app.preferences.paid_judgment = false;
    let saved = o.app.clone();
    assert_eq!(o.check_again(&aid).unwrap_err().status, 409);
    assert_eq!(o.app, saved);
    // And while a Prompt awaits an unaided answer, the earlier answer and its
    // comparison stay out of reach.
    o.app.preferences.paid_judgment = true;
    let card = o.target();
    o.next_card(&card);
    assert!(o.app.practice.protected());
    let saved = o.app.clone();
    assert_eq!(o.check_again(&aid).unwrap_err().status, 409);
    assert_eq!(o.app, saved);
}

// --- Claim refusals ---------------------------------------------------------

#[test]
fn a_queued_check_is_closed_unsent_when_it_may_no_longer_be_sent() {
    // The switch was turned off.
    let mut o = Owner::new().mapping();
    let aid = o.answer("test the bilge pump");
    o.app.preferences.paid_judgment = false;
    assert!(o.claim().is_none());
    assert_eq!(o.check(&aid).status, CheckStatus::Refused);
    assert!(o.app.spend.is_empty());
    assert_eq!(o.archive().format, JUDGED, "the Check remains");

    // This Scry cannot reach Jev.
    let mut o = Owner::new().mapping();
    let aid = o.answer("test the bilge pump");
    let mut unbound = config();
    unbound.jev = false;
    assert!(o.claim_with(&unbound).is_none());
    assert_eq!(o.check(&aid).status, CheckStatus::Refused);
    assert!(o.app.spend.is_empty());

    // The Study was dropped.
    let mut o = Owner::new().mapping();
    let aid = o.answer("test the bilge pump");
    let sid = o.attempt(&aid).prompt.study_id.clone();
    o.call("/practice/drop", &[("study_id", &sid)]);
    assert!(o.claim().is_none());
    assert_eq!(o.check(&aid).status, CheckStatus::Superseded);
    assert!(o.app.spend.is_empty());
    assert_eq!(o.check_again(&aid).unwrap_err().status, 409);

    // The Owner token that answered was revoked.
    let mut o = Owner::new().with_token(true).mapping();
    let aid = o.answer("test the bilge pump");
    let token = o.token.clone().unwrap();
    o.call("/tokens/revoke", &[("token_id", &token)]);
    assert!(o.claim().is_none());
    assert_eq!(o.check(&aid).status, CheckStatus::Refused);
    assert!(
        o.check(&aid)
            .reason
            .as_deref()
            .unwrap()
            .contains("paid permission")
    );
    assert!(o.app.spend.is_empty());
}

#[test]
fn a_response_for_a_dropped_study_is_ledgered_and_decides_nothing() {
    let mut o = Owner::new().mapping();
    let card = o.target();
    let aid = o.answer("test the bilge pump");
    let pending = o.attempt(&aid).clone();
    let work = o.claim().unwrap();
    let sid = pending.prompt.study_id.clone();
    o.call("/practice/drop", &[("study_id", &sid)]);
    let dropped = o.card(&card).clone();
    assert_eq!(o.finish(&work, accept()).unwrap_err().status, 409);
    assert_eq!(o.check(&aid).status, CheckStatus::Superseded);
    assert_eq!(o.attempt(&aid), &pending);
    assert_eq!(o.card(&card), &dropped);
    let spent = o.app.spend.last().unwrap();
    assert_eq!(
        (spent.status.as_str(), spent.cost_micros),
        ("known", Some(12_000))
    );
    assert_eq!(spent.raw_response, Some(accept()));
}

// --- Restore ----------------------------------------------------------------

#[test]
fn a_restored_notebook_sends_nothing_and_says_why() {
    let mut o = Owner::new().text();
    let queued = o.answer("I believe in a single God");
    let second = o.card_at(1);
    o.next_card(&second);
    let sent = o.answer("the Father of might");
    // The older Check is sent; the newer stays queued.
    let work = o.claim().unwrap();
    assert_eq!(work.id, o.check(&queued).id);
    let (sent, queued) = (queued, sent);
    let restored_from = o.app.clone();
    engine::pause_for_restore(&mut o.app, o.now).unwrap();
    o.sound();
    assert_eq!(o.check(&queued).status, CheckStatus::Refused);
    assert!(
        o.check(&queued)
            .reason
            .as_deref()
            .unwrap()
            .contains("restored notebook")
    );
    assert_eq!(o.check(&sent).status, CheckStatus::Unknown);
    assert_eq!(o.app.spend, restored_from.spend, "the reservation is kept");
    assert_eq!(o.app.practice.attempts, restored_from.practice.attempts);
    assert_eq!(o.app.practice.cards, restored_from.practice.cards);
    assert!(o.claim().is_none());
    // A new mismatch is saved and ungraded with the restore reason.
    let third = o.card_at(2);
    o.next_card(&third);
    let aid = o.answer("made of heaven and earth");
    assert_eq!(o.attempt(&aid).verdict(), None);
    assert_eq!(o.check(&aid).status, CheckStatus::Refused);
    assert!(
        o.check(&aid)
            .reason
            .as_deref()
            .unwrap()
            .contains("restored notebook")
    );
    assert!(o.claim().is_none());
    assert_eq!(o.app.spend.len(), 1);
    let saved = o.app.clone();
    assert_eq!(o.check_again(&aid).unwrap_err().status, 409);
    assert_eq!(o.app, saved);
    // Once the Owner resumes, a deliberate check goes out.
    o.call("/settings/resume", &[]);
    o.check_again(&aid).unwrap();
    o.judge(accept());
    assert_eq!(o.attempt(&aid).verdict(), Some(Verdict::Good));
}

// --- Readers of a Verdict ---------------------------------------------------

#[test]
fn an_attempt_with_no_verdict_is_never_counted_as_a_miss_or_a_success() {
    let mut o = Owner::new().mapping();
    let card = o.target();
    let pending = o.answer("test the bilge pump");
    let evidence = result_progress::summarize(&o.app, o.attempt(&pending));
    assert_eq!(
        (evidence.correct, evidence.unaided, evidence.helped),
        (0, 0, 0)
    );
    assert!(evidence.recent.is_empty());
    assert_eq!(evidence.unscheduled_parts, 1);
    o.judge(unsure());
    // A later exact answer is the only evidence there is.
    o.next_card(&card);
    assert_eq!(o.held().phase, Some(StudyPhase::Quiz), "answered before");
    let good = o.answer("check the bilge pump");
    let evidence = result_progress::summarize(&o.app, o.attempt(&good));
    assert_eq!((evidence.correct, evidence.unaided), (1, 1));
    assert_eq!(evidence.recent, [result_progress::Mark::Correct]);
    // A Verdict from Jev counts as any Verdict does.
    o.check_again(&pending).unwrap();
    o.judge(reject());
    let evidence = result_progress::summarize(&o.app, o.attempt(&good));
    assert_eq!((evidence.correct, evidence.unaided), (1, 2));
    assert_eq!(
        evidence.recent,
        [result_progress::Mark::Miss, result_progress::Mark::Correct]
    );
    // An Owner Correction cannot overturn it.
    let pid = o.held().prompt.id;
    let saved = o.app.clone();
    let refused = study::mutate(
        &mut o.app,
        "/practice/correct",
        &BTreeMap::from([
            ("prompt_id".to_string(), pid),
            ("attempt_id".to_string(), good.clone()),
            ("verdict".to_string(), "Again".to_string()),
        ]),
        o.now,
        "judgment-correction",
    );
    assert_eq!(
        refused.unwrap_err().status,
        422,
        "a mapping is never self-graded"
    );
    assert_eq!(o.app, saved);
}

#[test]
fn an_owner_correction_is_refused_on_an_answer_left_for_jev() {
    let mut o = Owner::new().text();
    let aid = o.answer("I believe in a single God");
    o.judge(reject());
    let saved = o.app.clone();
    let pid = o.held().prompt.id;
    let refused = study::mutate(
        &mut o.app,
        "/practice/correct",
        &BTreeMap::from([
            ("prompt_id".to_string(), pid),
            ("attempt_id".to_string(), aid),
            ("verdict".to_string(), "Good".to_string()),
        ]),
        o.now,
        "judgment-correction",
    )
    .unwrap_err();
    assert_eq!(refused.status, 422);
    assert!(refused.message.contains("not self-graded"));
    assert_eq!(o.app, saved);
}

#[test]
fn the_result_and_history_pages_show_each_state_honestly() {
    let mut o = Owner::new().mapping();
    let aid = o.answer("test the bilge pump");
    // Being checked: no Verdict anywhere, and the page looks again.
    let page = o.shown("/recall", "mapping-checking");
    assert!(page.contains("data-state=\"exact-checking\""));
    assert!(page.contains("data-poll"));
    assert!(page.contains("Your answer is saved") && page.contains("Checking it."));
    assert!(page.contains(">Refresh</a>") && page.contains(">Next</button>"));
    assert!(!page.contains("data-verdict"));
    assert!(!page.contains("feedback-miss") && !page.contains("feedback-correct"));
    let history = o.shown("/history", "history-checking");
    assert!(history.contains("<span>Checking</span>"));
    assert!(!history.contains("<span>Again</span>") && !history.contains("data-verdict"));
    // Saved and not graded: the reason, Check again and Next. No miss styling.
    let work = o.claim().unwrap();
    engine::fail_work(&mut o.app, &work, "The check timed out.", true, o.now).unwrap();
    let page = o.shown("/recall", "mapping-ungraded");
    assert!(page.contains("data-state=\"exact-ungraded\""));
    assert!(!page.contains("data-poll"));
    assert!(page.contains("Saved, not graded."));
    assert!(page.contains("The check timed out."));
    assert!(page.contains("action=\"/practice/check\""));
    assert!(page.contains(&format!("name=\"attempt_id\" value=\"{aid}\"")));
    assert!(page.contains(">Check again</button>") && page.contains(">Next</button>"));
    assert!(page.contains("Word-by-word comparison, not the Verdict"));
    assert!(!page.contains("data-verdict"));
    assert!(!page.contains("feedback-miss") && !page.contains("feedback-correct"));
    let history = o.shown("/history", "history-ungraded");
    assert!(history.contains("<span>Saved, not graded</span>"));
    assert!(history.contains("The check timed out."));
    assert!(history.contains("action=\"/practice/check\""));
    assert!(!history.contains("data-verdict"));
    // Decided by Jev: the Verdict, its Authority, and the comparison labelled.
    o.check_again(&aid).unwrap();
    o.judge(accept());
    let page = o.shown("/recall", "mapping-decided-good");
    assert!(page.contains("data-state=\"exact-result\""));
    assert!(page.contains("<h1 data-verdict=\"Good\">Good.</h1>"));
    assert!(page.contains(&format!("Judged by Jev (recall-v1; {MODEL})")));
    assert!(page.contains("Word-by-word comparison, not the Verdict"));
    assert!(page.contains("Jev accepted this answer as the same recall"));
    assert!(page.contains("comparison only, not the Verdict"));
    assert!(!page.contains("action=\"/practice/check\"") && !page.contains("data-poll"));
    assert!(page.contains("data-result-progress"));
    let history = o.shown("/history", "history-decided");
    assert!(history.contains("<span>Good</span>"));
    assert!(history.contains(&format!("Judged by Jev (recall-v1; {MODEL})")));
    assert!(history.contains("Word-by-word comparison, not the Verdict"));
    assert!(history.contains("data-schedule-effect=\"scheduled\" data-verdict=\"Good\""));
}

#[test]
fn a_lesson_result_shows_the_same_states() {
    let mut o = Owner::new().text();
    let aid = o.answer("I believe in a single God");
    let page = o.shown("/recall", "lesson-checking");
    assert!(page.contains("class=\"lesson-stage\" data-state=\"exact-checking\""));
    assert!(page.contains("data-poll") && !page.contains("data-verdict"));
    assert!(!page.contains("Not quite") && !page.contains("nato-miss"));
    o.judge(unsure());
    let page = o.shown("/recall", "lesson-ungraded");
    assert!(page.contains("class=\"lesson-stage\" data-state=\"exact-ungraded\""));
    assert!(page.contains("Saved, not graded.") && page.contains(">Check again</button>"));
    assert!(page.contains("Jev could not decide this answer"));
    assert!(!page.contains("data-verdict") && !page.contains("Not quite"));
    // Lesson progress counts no answer yet.
    assert!(page.contains("Progress <span>0 answers</span>"));
    o.check_again(&aid).unwrap();
    o.judge(reject());
    let page = o.shown("/recall", "lesson-decided-again");
    assert!(page.contains("class=\"lesson-stage\" data-state=\"exact-result\""));
    assert!(page.contains("<h1 data-verdict=\"Again\">Again.</h1>"));
    assert!(page.contains("Judged by Jev"));
    assert!(page.contains("The phrase to practise"));
    assert!(page.contains(">Try this phrase again</button>"));
    assert!(page.contains("Progress <span>1 answer</span>"));
}

// --- Archive, rows and tampering --------------------------------------------

/// A notebook holding every kind of record paid judgment adds.
fn judged_notebook() -> (Owner, String, String) {
    let mut o = Owner::new().with_token(true).mapping();
    let card = o.target();
    let stale = o.answer("test the bilge pump");
    let work = o.claim().unwrap();
    o.next_card(&card);
    o.answer("check the bilge pump");
    o.finish(&work, accept()).unwrap();
    o.next_card(&card);
    let decided = o.answer("look over the bilge pump");
    o.judge(reject());
    o.next_card(&card);
    let ungraded = o.answer("pump the bilge");
    o.judge(unsure());
    o.check_again(&ungraded).unwrap();
    assert!(o.attempt(&stale).effects[0].superseded);
    (o, decided, stale)
}

#[test]
fn a_judged_notebook_round_trips_under_its_own_format_name() {
    let (o, _, _) = judged_notebook();
    let archive = o.archive();
    assert_eq!(archive.format, JUDGED);
    let bytes = archive.encode().unwrap();
    let decoded = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
    assert_eq!(decoded.app, o.app);
    assert_eq!(decoded.encode().unwrap(), bytes);
    // A forged earlier format name is refused, as is the new name on a
    // notebook that holds nothing of paid judgment.
    let mut forged = archive.clone();
    forged.format = "scry-rust-durable-sqlite-study-phase-v3".into();
    assert!(forged.validate().is_err());
    let mut plain = drive_mapping(Owner::off()).archive();
    assert_eq!(plain.format, "scry-rust-durable-sqlite-study-phase-v3");
    plain.format = JUDGED.into();
    assert!(plain.validate().is_err());
    // A saved draft does not hide the judged format.
    let mut drafted = Owner::new().mapping();
    drafted.answer("test the bilge pump");
    let card = drafted.target();
    drafted.next_card(&card);
    let pid = drafted.held().prompt.id;
    drafted.call(
        "/practice/draft",
        &[
            ("prompt_id", &pid),
            ("generation", "0"),
            ("answer", "check"),
        ],
    );
    assert_eq!(drafted.archive().format, JUDGED);
    let rows = persistence::rows(&drafted.app).unwrap();
    assert!(rows.contains_key("checkpoint_compatibility/v1"));
    assert!(rows.contains_key("judgment_compatibility/v1"));
}

#[test]
fn judged_rows_are_named_so_an_older_reader_refuses_them() {
    let (o, _, _) = judged_notebook();
    let rows = persistence::rows(&o.app).unwrap();
    assert_eq!(rows["judgment_compatibility/v1"], "1");
    let checks = rows
        .keys()
        .filter(|key| key.starts_with("practice_checks/"))
        .count();
    assert_eq!(checks, o.app.practice.checks.len());
    assert!(checks >= 4);
    // The metadata row keeps no `checks` map for an older reader to accept.
    let meta: Value = serde_json::from_str(&rows["meta"]).unwrap();
    assert!(meta["practice"].get("checks").is_none());
    assert_eq!(meta["preferences"]["paid_judgment"], true);
    assert_eq!(persistence::app_from_rows(&rows).unwrap(), o.app);
    // The marker must agree with the notebook, both ways.
    let mut unmarked = rows.clone();
    unmarked.remove("judgment_compatibility/v1");
    assert!(persistence::app_from_rows(&unmarked).is_err());
    let plain = drive_mapping(Owner::off());
    let mut marked = persistence::rows(&plain.app).unwrap();
    assert!(!marked.keys().any(|key| key.starts_with("practice_checks/")));
    assert!(!marked.contains_key("judgment_compatibility/v1"));
    marked.insert("judgment_compatibility/v1".into(), "1".into());
    assert!(persistence::app_from_rows(&marked).is_err());
    // A Check row without its Attempt's notebook is refused.
    let mut orphan = persistence::rows(&plain.app).unwrap();
    let (key, value) = rows
        .iter()
        .find(|(key, _)| key.starts_with("practice_checks/"))
        .unwrap();
    orphan.insert(key.clone(), value.clone());
    orphan.insert("judgment_compatibility/v1".into(), "1".into());
    assert!(
        persistence::app_from_rows(&orphan)
            .and_then(|app| engine::validate_app(&app))
            .is_err()
    );
}

#[test]
fn tampering_with_a_verdict_an_effect_or_the_stored_response_is_refused() {
    let (o, decided, stale) = judged_notebook();
    let ungraded = o
        .app
        .practice
        .attempts
        .values()
        .find(|a| a.verdict().is_none())
        .unwrap()
        .id
        .clone();
    let refused = |name: &str, tamper: &dyn Fn(&mut App)| {
        let mut app = o.app.clone();
        tamper(&mut app);
        assert_ne!(app, o.app, "{name} changes nothing");
        assert!(engine::validate_app(&app).is_err(), "{name} was accepted");
        assert!(
            persistence::Archive::new(app, vec![], o.now)
                .encode()
                .is_err(),
            "{name} was archived"
        );
    };
    fn judgment<'a>(app: &'a mut App, id: &str) -> &'a mut study::Decided {
        app.practice
            .attempts
            .get_mut(id)
            .unwrap()
            .judgment
            .as_mut()
            .unwrap()
            .decided
            .as_mut()
            .unwrap()
    }
    refused("a flipped Verdict", &|app| {
        judgment(app, &decided).verdict = Verdict::Good
    });
    refused("a flipped Verdict with its Check", &|app| {
        let check = judgment(app, &decided).check_id.clone();
        judgment(app, &decided).verdict = Verdict::Good;
        app.practice.checks.get_mut(&check).unwrap().status = CheckStatus::Accepted;
    });
    refused("a flipped Unit Verdict", &|app| {
        judgment(app, &decided).units[0].verdict = Verdict::Good
    });
    refused("a flipped effect Verdict", &|app| {
        app.practice.attempts.get_mut(&decided).unwrap().effects[0].verdict = Verdict::Good
    });
    refused("another Authority", &|app| {
        judgment(app, &decided).authority.name = "exact-v2".into()
    });
    refused("another model", &|app| {
        judgment(app, &decided).authority.grader_version = "recall-v1;model=other".into()
    });
    refused("another policy", &|app| {
        app.practice
            .attempts
            .get_mut(&decided)
            .unwrap()
            .judgment
            .as_mut()
            .unwrap()
            .policy = "recall-v2".into()
    });
    refused("a cleared superseded flag", &|app| {
        app.practice.attempts.get_mut(&stale).unwrap().effects[0].superseded = false
    });
    refused("a superseded flag on a scheduled effect", &|app| {
        app.practice.attempts.get_mut(&decided).unwrap().effects[0].superseded = true
    });
    refused("a superseded flag on a local Attempt", &|app| {
        let local = app
            .practice
            .attempts
            .values_mut()
            .find(|a| a.judgment.is_none())
            .unwrap();
        local.effects[0].superseded = true;
    });
    // The stored raw response is re-read, never re-judged: bytes that no
    // longer read as the recorded Verdict are refused, even with their hash.
    let lease = |app: &App, id: &str| {
        let check = app.practice.attempts[id]
            .judgment
            .as_ref()
            .unwrap()
            .decided
            .as_ref()
            .unwrap()
            .check_id
            .clone();
        app.practice.checks[&check].lease.clone().unwrap()
    };
    refused("an edited raw response", &|app| {
        let lease = lease(app, &decided);
        let spend = app.spend.iter_mut().find(|s| s.id == lease).unwrap();
        spend.raw_response = Some(accept());
    });
    refused("a replaced raw response with its hash", &|app| {
        let lease = lease(app, &decided);
        let spend = app.spend.iter_mut().find(|s| s.id == lease).unwrap();
        let raw = accept();
        spend.response_hash = Some(persistence::sha256(raw.as_bytes()));
        spend.response_bytes = Some(raw.len());
        spend.raw_response = Some(raw);
    });
    refused("an inconclusive raw response with its hash", &|app| {
        let lease = lease(app, &decided);
        let spend = app.spend.iter_mut().find(|s| s.id == lease).unwrap();
        let raw = unsure();
        spend.response_hash = Some(persistence::sha256(raw.as_bytes()));
        spend.response_bytes = Some(raw.len());
        spend.raw_response = Some(raw);
    });
    refused("a removed ledger row", &|app| {
        let lease = lease(app, &decided);
        app.spend.retain(|s| s.id != lease);
    });
    // The local comparison and the frozen targets are still tamper-evident.
    refused("an edited local comparison", &|app| {
        app.practice
            .attempts
            .get_mut(&ungraded)
            .unwrap()
            .grade
            .verdict = Verdict::Good
    });
    refused("an edited answer", &|app| {
        app.practice.attempts.get_mut(&ungraded).unwrap().raw_answer = "check the bilge pump".into()
    });
    refused("an effect on an answer with no Verdict", &|app| {
        let effect = app.practice.attempts[&decided].effects[0].clone();
        app.practice.attempts.get_mut(&ungraded).unwrap().effects = vec![effect];
    });
    refused("a Verdict copied onto an answer with none", &|app| {
        let copied = app.practice.attempts[&decided]
            .judgment
            .clone()
            .unwrap()
            .decided;
        let effects = app.practice.attempts[&decided].effects.clone();
        let a = app.practice.attempts.get_mut(&ungraded).unwrap();
        a.judgment.as_mut().unwrap().decided = copied;
        a.effects = effects;
    });
    refused("a moved target", &|app| {
        app.practice
            .attempts
            .get_mut(&ungraded)
            .unwrap()
            .judgment
            .as_mut()
            .unwrap()
            .targets[0]
            .revision += 1
    });
    refused("a removed judgment", &|app| {
        app.practice.attempts.get_mut(&ungraded).unwrap().judgment = None
    });
    refused("a removed Check", &|app| {
        let id = app
            .practice
            .checks
            .values()
            .find(|c| c.attempt_id == ungraded && c.status == CheckStatus::Queued)
            .unwrap()
            .id
            .clone();
        app.practice.checks.remove(&id);
        let other = app
            .practice
            .checks
            .values()
            .find(|c| c.attempt_id == ungraded)
            .unwrap()
            .id
            .clone();
        app.practice.checks.remove(&other);
    });
    refused("a second open Check", &|app| {
        let open = app
            .practice
            .checks
            .values_mut()
            .find(|c| c.attempt_id == ungraded && c.status == CheckStatus::Unsure)
            .unwrap();
        open.status = CheckStatus::Sent;
        open.reason = None;
    });
    refused("a Check marked sent without its reservation", &|app| {
        let queued = app
            .practice
            .checks
            .values_mut()
            .find(|c| c.status == CheckStatus::Queued)
            .unwrap();
        queued.status = CheckStatus::Sent;
        queued.lease = Some("forged-lease-0001".into());
        queued.started_ms = Some(NOW);
    });
    refused("a Check by a missing Owner token", &|app| {
        app.practice
            .checks
            .values_mut()
            .next()
            .unwrap()
            .acting_token = Some("f".repeat(32))
    });
    refused("a moved Card", &|app| {
        app.practice.cards.values_mut().next().unwrap().revision += 1
    });
    // Untampered, it is accepted.
    engine::validate_app(&o.app).unwrap();
}

#[test]
fn a_local_attempt_is_validated_exactly_as_before() {
    // Tampering that the earlier validation refused is still refused, whether
    // or not the notebook also holds answers left for Jev.
    let (judged, _, _) = judged_notebook();
    for o in [drive_mapping(Owner::off()), judged] {
        let local = o
            .app
            .practice
            .attempts
            .values()
            .find(|a| a.judgment.is_none() && !a.disclosure)
            .unwrap()
            .id
            .clone();
        for tamper in [
            (|a: &mut Attempt| a.grade.verdict = Verdict::Again) as fn(&mut Attempt),
            |a| a.effects[0].verdict = Verdict::Again,
            |a| a.effects[0].scheduled = false,
            |a| a.effects[0].after.successes += 1,
            |a| a.effects.clear(),
            |a| a.raw_answer.push_str(" extra"),
            |a| a.assisted = true,
        ] {
            let mut app = o.app.clone();
            tamper(app.practice.attempts.get_mut(&local).unwrap());
            assert!(engine::validate_app(&app).is_err());
        }
    }
}

// --- The Owner's switch -----------------------------------------------------

/// `/settings` as the runtime renders it: `judgment` is its answer to whether
/// this Scry can reach Jev.
fn settings(o: &Owner, judgment: Option<bool>) -> String {
    let mut query = BTreeMap::from([("__nonce".to_string(), "judgment-nonce".to_string())]);
    if let Some(capable) = judgment {
        query.insert("__judgment".into(), capable.to_string());
    }
    web::render(&o.app, "/settings", &query, o.now, None)
}

#[test]
fn the_switch_is_set_once_by_the_signed_in_owner_and_names_the_notebook_judged() {
    let mut o = Owner::off();
    o.jev = true;
    let before = o.app.clone();
    assert!(!persistence::judged(&o.app));
    // On, once: the same operation again changes nothing and a changed payload conflicts.
    let on = o
        .op(
            "judgment-switch-on-000001",
            "/settings/judgment",
            &[("paid_judgment", "on")],
        )
        .unwrap();
    assert_eq!(on.redirect, "/settings");
    assert!(o.app.preferences.paid_judgment);
    assert!(persistence::judged(&o.app));
    assert_eq!(o.archive().format, JUDGED);
    let switched = o.app.clone();
    let replay = o
        .op(
            "judgment-switch-on-000001",
            "/settings/judgment",
            &[("paid_judgment", "on")],
        )
        .unwrap();
    assert_eq!(replay, on);
    assert_eq!(o.app, switched, "a replay writes nothing");
    let conflict = o
        .op(
            "judgment-switch-on-000001",
            "/settings/judgment",
            &[("paid_judgment", "off")],
        )
        .unwrap_err();
    assert_eq!(conflict.status, 409);
    assert_eq!(o.app, switched);
    // Anything but on or off, and anything beside it, is refused whole.
    for pairs in [
        vec![("paid_judgment", "yes")],
        vec![],
        vec![("paid_judgment", "off"), ("verdict", "Good")],
    ] {
        assert_eq!(
            o.try_call("/settings/judgment", &pairs).unwrap_err().status,
            422
        );
        assert_eq!(o.app, switched);
    }
    // The browser's own fence still applies: a forged session token is refused.
    let mut forged = o.fields("judgment-switch-forged-01", &[("paid_judgment", "off")]);
    forged.insert("csrf".into(), "not-this-notebooks-session-token".into());
    let refused = engine::mutate_browser(
        &mut o.app,
        "/settings/judgment",
        &forged,
        o.now,
        "judgment-seed-forged",
        true,
    )
    .unwrap_err();
    assert_eq!(refused.status, 403);
    assert_eq!(o.app, switched);
    // Off again. The notebook used the switch, so it keeps its own format name.
    o.call("/settings/judgment", &[("paid_judgment", "off")]);
    assert!(!o.app.preferences.paid_judgment);
    assert!(persistence::judged(&o.app));
    assert_eq!(o.archive().format, JUDGED);
    let mut plain = o.app.clone();
    plain.operations.clear();
    plain.revision = before.revision;
    assert_eq!(plain, before, "only the receipts and the revision remain");
}

#[test]
fn an_owner_token_never_sets_the_switch_whatever_permission_it_carries() {
    for paid in [false, true] {
        let mut o = Owner::off().with_token(paid);
        o.jev = true;
        let before = o.app.clone();
        for value in ["on", "off"] {
            let refused = o
                .try_call("/settings/judgment", &[("paid_judgment", value)])
                .unwrap_err();
            assert_eq!(refused.status, 403);
            assert!(refused.message.contains("signed-in Owner"));
            assert_eq!(o.app, before, "nothing is written");
        }
    }
    // The API offers no route to it at all.
    assert!(scry::api::mutation_path("/api/v1/settings/judgment").is_none());
    assert_eq!(
        scry::api::mutation_path("/api/v1/check"),
        Some("/practice/check")
    );
}

#[test]
fn the_switch_cannot_be_turned_on_where_scry_cannot_reach_jev() {
    let mut o = Owner::off();
    let before = o.app.clone();
    let refused = o
        .try_call("/settings/judgment", &[("paid_judgment", "on")])
        .unwrap_err();
    assert_eq!(refused.status, 409);
    assert!(refused.message.contains("not available here"));
    assert_eq!(o.app, before);
    // A switch left on from elsewhere can always be turned off.
    o.app.preferences.paid_judgment = true;
    o.call("/settings/judgment", &[("paid_judgment", "off")]);
    assert!(!o.app.preferences.paid_judgment);
}

#[test]
fn settings_says_what_leaves_scry_and_offers_the_switch_only_where_jev_is_reachable() {
    let mut o = Owner::off();
    // Rendered without the runtime's answer, the page is the one before paid judgment.
    let plain = settings(&o, None);
    assert!(!plain.contains("Paid judgment") && !plain.contains("/settings/judgment"));
    // Not reachable: said plainly, with no control, whatever is stored.
    for stored in [false, true] {
        o.app.preferences.paid_judgment = stored;
        let page = settings(&o, Some(false));
        assert!(page.contains("Paid judgment is not available here."));
        assert!(page.contains("data-paid-judgment=\"unavailable\""));
        assert!(!page.contains("action=\"/settings/judgment\""));
        assert!(!page.contains("name=\"paid_judgment\""));
    }
    // Reachable and off: what leaves, what it reserves, and one native form to turn it on.
    o.app.preferences.paid_judgment = false;
    let off = settings(&o, Some(true));
    assert!(off.contains("data-paid-judgment=\"off\""));
    assert!(off.contains("Your answer and the expected text leave Scry"));
    assert!(off.contains("Each check reserves $0.50 of the $3.50 daily allowance"));
    assert!(off.contains("<form method=\"post\" action=\"/settings/judgment\""));
    assert!(off.contains("name=\"paid_judgment\" value=\"on\""));
    assert!(off.contains("Turn paid judgment on"));
    assert!(!off.contains("name=\"paid_judgment\" value=\"off\""));
    // Reachable and on: the same statement and the way back.
    o.app.preferences.paid_judgment = true;
    let on = settings(&o, Some(true));
    assert!(on.contains("data-paid-judgment=\"on\""));
    assert!(on.contains("Your answer and the expected text leave Scry"));
    assert!(on.contains("name=\"paid_judgment\" value=\"off\""));
    assert!(on.contains("Turn paid judgment off"));
    assert!(!on.contains("name=\"paid_judgment\" value=\"on\""));
}

// --- A Card whose answer is still with Jev is not asked again ---------------

fn one_pair(mut o: Owner) -> Owner {
    let vid = o
        .call(
            "/library/mapping/add",
            &[(
                "data",
                &json!({"title":"Synthetic single practice","direction":"forward","spelling_strict":false,"pairs":[
                    {"key":"Before leaving the berth","value":"check the bilge pump"}
                ]})
                .to_string(),
            )],
        )
        .entity_id
        .unwrap();
    o.call("/practice/study/start", &[("version_id", &vid)]);
    o
}
/// A notebook whose Starter shelf has all been begun and dropped, so that
/// Study has no new subject of its own to open and no other Card to serve.
fn shelved(mut o: Owner) -> Owner {
    for starter in [
        "nato",
        "st-michael-en",
        "rosary-joyful",
        "rosary-luminous",
        "rosary-sorrowful",
        "rosary-glorious",
        "baby-soothing",
        "baby-everyday",
        "baby-montessori",
    ] {
        o.call("/practice/lesson/start", &[("starter", starter)]);
        let sid = o.held().prompt.study_id;
        o.call("/practice/drop", &[("study_id", &sid)]);
    }
    o.call("/practice/review/start", &[]);
    assert!(o.app.practice.current.is_none(), "nothing left to open");
    o
}
fn due_ids(o: &Owner, key: &str) -> Vec<String> {
    study::due(&o.app, o.now)
        .get(key)
        .and_then(|v| v.as_array().cloned())
        .unwrap_or_default()
        .into_iter()
        .map(|v| v.as_str().unwrap().to_string())
        .collect()
}

#[test]
fn next_keeps_the_held_result_while_its_only_card_is_being_checked() {
    for path in ["/practice/next", "/practice/review/next"] {
        let mut o = one_pair(shelved(Owner::new()));
        let card = o.target();
        let aid = o.answer("test the bilge pump");
        assert_eq!(o.state(&aid), JudgmentState::Checking);
        assert_eq!(
            study::awaiting_verdict(&o.app)
                .into_iter()
                .collect::<Vec<_>>(),
            std::slice::from_ref(&card)
        );
        // The Card is still due by its schedule, and is named apart, not served.
        assert!(o.card(&card).state.schedule.due_ms <= o.now);
        assert!(due_ids(&o, "due").is_empty());
        assert_eq!(due_ids(&o, "checking"), std::slice::from_ref(&card));
        let held = o.held();
        // Queued, then sent: Next shows the same held result both times.
        for sent in [false, true] {
            if sent {
                o.claim().unwrap();
            }
            let pid = o.held().prompt.id;
            let result = o.call(path, &[("after", &pid)]);
            assert_eq!(o.held(), held, "nothing new is presented");
            assert_eq!(result.data.unwrap()["attempt_id"], aid.as_str());
            assert_eq!(o.app.practice.attempts.len(), 1);
            let page = o.page("/recall");
            assert!(page.contains("data-state=\"exact-checking\" data-prompt-id"));
            assert!(page.contains("data-poll"));
            assert!(!page.contains("Rest for now"));
        }
        // Rejected: the Card has its Verdict, is due again and is served again.
        let work = Work {
            id: o.check(&aid).id.clone(),
            lease: o.check(&aid).lease.clone().unwrap(),
            kind: engine::CHECK_WORK.into(),
            request: json!({}),
            photo_key: None,
        };
        o.finish(&work, reject()).unwrap();
        assert!(study::awaiting_verdict(&o.app).is_empty());
        assert_eq!(due_ids(&o, "due"), std::slice::from_ref(&card));
        assert!(study::due(&o.app, o.now).get("checking").is_none());
        let pid = o.held().prompt.id;
        o.call(path, &[("after", &pid)]);
        assert_ne!(o.held().prompt.id, held.prompt.id);
        assert_eq!(o.held().targets[0].card_id, card);
        assert!(o.held().attempt_id.is_none());
    }
}

#[test]
fn study_serves_other_cards_and_never_the_one_being_checked() {
    let mut o = shelved(Owner::new()).mapping();
    let first = o.target();
    let pending = o.answer("test the bilge pump");
    // Next introduces the second pair, then the third: never the waiting first.
    for exact in ["sound the horn", "never lose sight of them"] {
        let pid = o.held().prompt.id;
        o.call("/practice/next", &[("after", &pid)]);
        assert_ne!(o.target(), first);
        let aid = o.answer(exact);
        assert_eq!(o.attempt(&aid).verdict(), Some(Verdict::Good));
    }
    // Nothing else is due. The held result is a decided one, so it is released,
    // and the page says an answer is being checked instead of resting.
    let pid = o.held().prompt.id;
    let result = o.call("/practice/next", &[("after", &pid)]);
    assert!(o.app.practice.current.is_none());
    assert_eq!(result.data.as_ref().unwrap()["prompt"], Value::Null);
    assert_eq!(result.data.unwrap()["due"]["checking"], json!([first]));
    for path in ["/recall", "/"] {
        let page = o.shown(path, "study-checking-nothing-else");
        assert!(page.contains("data-state=\"exact-checking\" data-poll"));
        assert!(page.contains("Checking it."));
        assert!(page.contains("test the bilge pump"));
        assert!(!page.contains("Rest for now"));
        assert!(!page.contains("action=\"/practice/review/start\""));
    }
    // Study from the home page chooses nothing either.
    o.call("/practice/review/start", &[]);
    assert!(o.app.practice.current.is_none());
    assert!(study::continuous_preview(&o.app, o.now).is_none());
    // A Card the Owner names outright is still presented: that is his choice.
    o.call("/practice/next", &[("card_id", &first)]);
    assert_eq!(o.target(), first);
    let pid = o.held().prompt.id;
    o.call("/practice/review/start", &[("leave", &pid)]);
    assert!(o.app.practice.current.is_none());
    // Accepted: the Card is scheduled, nothing is due and the page rests.
    o.judge(accept());
    assert_eq!(o.attempt(&pending).verdict(), Some(Verdict::Good));
    let page = o.page("/recall");
    assert!(page.contains("data-state=\"rest\""));
    assert!(page.contains("Rest for now"));
}

#[test]
fn study_opens_another_subject_rather_than_the_card_being_checked() {
    // A new notebook still has its Starter shelf to open.
    let mut o = one_pair(Owner::new());
    let card = o.target();
    o.answer("test the bilge pump");
    let pid = o.held().prompt.id;
    o.call("/practice/review/next", &[("after", &pid)]);
    assert_ne!(o.target(), card);
    assert_ne!(o.held().prompt.study_id, o.card(&card).study_id);
    assert!(o.held().attempt_id.is_none());
}

#[test]
fn an_ungraded_card_stays_due_and_is_served_again() {
    let mut o = one_pair(Owner::new());
    let card = o.target();
    let aid = o.answer("test the bilge pump");
    o.judge(unsure());
    assert_eq!(o.state(&aid), JudgmentState::Ungraded);
    assert!(study::awaiting_verdict(&o.app).is_empty());
    assert_eq!(due_ids(&o, "due"), std::slice::from_ref(&card));
    let pid = o.held().prompt.id;
    o.call("/practice/next", &[("after", &pid)]);
    assert_eq!(o.target(), card);
    assert!(o.held().attempt_id.is_none());
}

#[test]
fn a_lesson_passes_over_a_phrase_being_checked_and_holds_when_every_phrase_is() {
    let mut o = Owner::new().text();
    let first = o.target();
    let pending = o.answer("I believe in a single God");
    let held = o.held();
    // A guided retry, a joined passage and the whole text would each ask the
    // waiting phrase again: the held result stays instead.
    for mode in ["retry", "join", "whole"] {
        let result = o.call(
            "/practice/lesson/next",
            &[("after", &held.prompt.id), ("mode", mode)],
        );
        assert_eq!(o.held(), held, "{mode}");
        assert_eq!(result.data.unwrap()["attempt_id"], pending.as_str());
    }
    let page = o.page("/recall");
    assert!(page.contains("data-state=\"lesson-home\""));
    assert!(page.contains("data-state=\"exact-checking\" data-prompt-id"));
    // The next phrases are served in order.
    for (index, phrase) in CREED.lines().enumerate().skip(1) {
        let pid = o.held().prompt.id;
        o.call(
            "/practice/lesson/next",
            &[("after", &pid), ("mode", "phrases")],
        );
        assert_eq!(o.held().targets[0].unit_index, index);
        o.answer(phrase);
    }
    // Wrapping round passes over the first phrase, still with Jev.
    let pid = o.held().prompt.id;
    o.call(
        "/practice/lesson/next",
        &[("after", &pid), ("mode", "phrases")],
    );
    assert_eq!(o.held().targets[0].unit_index, 1);
    assert_ne!(o.target(), first);
    // Starting the same lesson again begins at the first phrase not waiting.
    let vid = o.held().prompt.version_id;
    let pid = o.held().prompt.id;
    o.call(
        "/practice/lesson/start",
        &[("version_id", &vid), ("leave", &pid)],
    );
    assert_eq!(o.held().targets[0].unit_index, 1);
    // Once Jev has decided, the phrase is asked again in its turn.
    o.judge(reject());
    o.answer("the Father almighty");
    for index in [2, 3, 0] {
        let pid = o.held().prompt.id;
        o.call(
            "/practice/lesson/next",
            &[("after", &pid), ("mode", "phrases")],
        );
        assert_eq!(o.held().targets[0].unit_index, index);
        if index != 0 {
            o.answer(CREED.lines().nth(index).unwrap());
        }
    }
    assert_eq!(o.target(), first);
}

#[test]
fn a_list_being_checked_is_not_asked_again_until_it_has_a_verdict() {
    let mut o = shelved(Owner::new()).list();
    let pending = o.answer(
        "Annunciation\nVisitation\nThe birth of Christ\nPresentation\nFinding in the Temple",
    );
    assert_eq!(o.state(&pending), JudgmentState::Checking);
    // One Prompt covers the whole list, so every item waits with it.
    assert_eq!(study::awaiting_verdict(&o.app).len(), 5);
    let held = o.held();
    for (path, pairs) in [
        ("/practice/lesson/next", vec![("mode", "whole")]),
        ("/practice/lesson/next", vec![("mode", "phrases")]),
        ("/practice/review/next", vec![]),
        ("/practice/next", vec![]),
        ("/practice/lesson/start", vec![("starter", "rosary-joyful")]),
    ] {
        let mut pairs = pairs;
        pairs.push(("after", &held.prompt.id));
        o.call(path, &pairs);
        assert_eq!(o.held(), held, "{path}");
        assert_eq!(o.app.practice.attempts.len(), 1);
    }
    o.judge(reject());
    assert_eq!(o.attempt(&pending).verdict(), Some(Verdict::Again));
    o.call(
        "/practice/lesson/next",
        &[("after", &held.prompt.id), ("mode", "whole")],
    );
    assert_ne!(o.held().prompt.id, held.prompt.id);
    assert_eq!(o.held().targets.len(), 5);
}

#[test]
fn only_the_claimed_check_may_still_be_sent() {
    let mut o = one_pair(Owner::new());
    o.answer("test the bilge pump");
    let work = o.claim().unwrap();
    assert!(engine::awaits_send(&o.app, &work));
    let mut other = work.clone();
    other.lease = "judgment-lease-of-another-claim".into();
    assert!(!engine::awaits_send(&o.app, &other));
    // Recovered as unknown: a send that had not started must not start now.
    o.now += 2 * MINUTE;
    assert!(engine::recover_grading(&mut o.app, o.now));
    assert!(!engine::awaits_send(&o.app, &work));
}

// --- The comparison shown is the one that was graded ------------------------

#[test]
fn a_section_of_a_list_is_compared_over_its_own_items() {
    let mut o = Owner::new();
    let vid = o
        .call(
            "/library/version/add",
            &[(
                "data",
                &json!({"title":"Synthetic harbour route","name":"Authored order","kind":"ordered_list",
                    "text":"Red gate\nBlue bridge\nGreen field\nWhite tower\nBlack lake\n",
                    "sections":[
                        {"heading":"Outward","units":["Red gate\n","Blue bridge\n"]},
                        {"heading":"Return","units":["Green field\n","White tower\n","Black lake\n"]}]})
                .to_string(),
            )],
        )
        .entity_id
        .unwrap();
    o.call("/practice/lesson/start", &[("version_id", &vid)]);
    let whole = o.answer("Red gate\nBlue bridge\nGreen field\nWhite tower\nBlack lake");
    assert_eq!(o.attempt(&whole).verdict(), Some(Verdict::Good));
    // Recite the second Section alone: its scope begins at the third item.
    let held = o.held();
    let section = o.app.library.versions[&vid].sections[1].id.clone();
    o.call(
        "/practice/next",
        &[
            ("after", &held.prompt.id),
            ("study_id", &held.prompt.study_id),
            ("recite", "section"),
            ("section_id", &section),
        ],
    );
    assert_eq!(
        o.held()
            .targets
            .iter()
            .map(|t| t.unit_index)
            .collect::<Vec<_>>(),
        [2, 3, 4]
    );
    let aid = o.answer("Green field\nThe pale tower\nBlack lake");
    let a = o.attempt(&aid).clone();
    assert_eq!(a.verdict(), None);
    assert_eq!(
        a.grade.units.iter().map(|u| u.verdict).collect::<Vec<_>>(),
        [Verdict::Good, Verdict::Again, Verdict::Good],
        "the comparison that was graded"
    );
    let request = o.claim().unwrap().request;
    assert_eq!(
        request["state"]["expected_answer"],
        "Green field\nWhite tower\nBlack lake"
    );
    let work = Work {
        id: o.check(&aid).id.clone(),
        lease: o.check(&aid).lease.clone().unwrap(),
        kind: engine::CHECK_WORK.into(),
        request,
        photo_key: None,
    };
    o.finish(&work, unsure()).unwrap();
    // The page counts the same three items the grade compared, not the list's first three.
    let page = o.shown("/recall", "list-section-ungraded");
    assert!(page.contains("Item-by-item comparison, not the Verdict"));
    assert!(
        page.contains("2 of 3 items matched · 2 in order."),
        "the comparison covers the Section that was asked"
    );
}
