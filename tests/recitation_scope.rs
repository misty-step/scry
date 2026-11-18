//! A Recitation covers every Unit of its Section or Version. No client can
//! hold or record one over Units not all introduced: recovery would refuse the
//! notebook from then on, and an Attempt is never removed.
use scry::{
    api, engine,
    grading::Verdict,
    model::*,
    owner_token::{MutationActor, OwnerToken},
    persistence,
    study::{self, Cue, Scope, StudyPhase},
};
use serde_json::{Value, json};

const NOW: i64 = 1_800_000_000_000;
const REFUSAL: &str = "Introduce every Unit in this scope before Recitation.";
const CANTICLE: [&str; 8] = [
    "Still waters hold the morning light,\n",
    "and quiet hills receive the rain;\n",
    "the patient reed bends low at night,\n",
    "and rises with the dawn again.\n",
    "No hurried wind disturbs the shore,\n",
    "no shadow dims the silver stream;\n",
    "the heron waits, and nothing more,\n",
    "beside the slow unbroken gleam.\n",
];

#[derive(Clone, Copy, Debug)]
enum Client {
    /// The app's own form route.
    App,
    /// The versioned API as an Owner token drives it: CLI and tool server too.
    Token,
}
struct Owner {
    app: App,
    n: u64,
}
impl Owner {
    fn stored(mut app: App) -> Self {
        let token = OwnerToken::issue(
            "Recitation scope agent",
            None,
            false,
            NOW,
            "abcdef0123456789abcdef0123456789abcdef0123456789",
        )
        .unwrap();
        app.owner_tokens.insert(token.info.id.clone(), token);
        Self { app, n: 0 }
    }
    /// One mutation through the path the API maps to. Nothing is asserted here:
    /// each test states what the notebook must be afterwards.
    fn send(&mut self, client: Client, path: &str, data: Value) -> AppResult<MutationResult> {
        self.n += 1;
        let mut data = data;
        data["operation_id"] = json!(format!("recitation-scope-operation-{:06}", self.n));
        let mut fields = api::library_fields(data)?;
        let path = api::mutation_path(path).unwrap_or(path);
        let (now, seed) = (
            NOW + 1_000 + self.n as i64,
            format!("recitation-scope-seed-{:06}", self.n),
        );
        match client {
            Client::App => {
                fields.insert("csrf".into(), self.app.csrf.clone());
                engine::mutate(&mut self.app, path, &fields, now, &seed)
            }
            Client::Token => {
                let token = self.app.owner_tokens.values().next().unwrap().info.clone();
                engine::mutate_as(
                    &mut self.app,
                    path,
                    &fields,
                    now,
                    &seed,
                    MutationActor::Token(&token),
                )
            }
        }
    }
    fn ok(&mut self, path: &str, data: Value) -> MutationResult {
        let result = self.send(Client::Token, path, data).unwrap();
        self.assert_recoverable();
        result
    }
    fn held(&self) -> study::Presentation {
        self.app.practice.current.clone().unwrap()
    }
    fn expected(&self) -> String {
        let p = self.held();
        self.app.library.versions[&p.prompt.version_id]
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .filter(|u| p.prompt.unit_ids.contains(&u.id))
            .map(|u| u.text.clone())
            .collect()
    }
    /// Source study of the held phrase, then its Quiz answered exactly.
    fn learn_held_phrase(&mut self) {
        let source = self.held();
        assert_eq!(source.phase, Some(StudyPhase::Study));
        assert!(matches!(source.prompt.scope, Scope::Unit { .. }));
        self.ok("/api/v1/quiz", json!({"prompt_id":source.prompt.id}));
        self.answer_exactly();
    }
    fn answer_exactly(&mut self) -> study::Attempt {
        let (id, answer) = (self.held().prompt.id, self.expected());
        let result = self.ok("/api/v1/answer", json!({"prompt_id":id,"answer":answer}));
        let attempt = self.app.practice.attempts[&result.entity_id.unwrap()].clone();
        assert_eq!(attempt.grade.verdict, Verdict::Good);
        attempt
    }
    /// Backup and export depend on exactly this: the notebook validates and its
    /// complete archive encodes and reads back.
    fn assert_recoverable(&self) {
        engine::validate_app(&self.app).unwrap();
        let bytes = persistence::Archive::new(self.app.clone(), vec![], NOW)
            .encode()
            .unwrap();
        let read = persistence::Archive::decode(&bytes, &persistence::sha256(&bytes)).unwrap();
        assert_eq!(read.app, self.app);
    }
    fn refused(&self) -> AppError {
        engine::validate_app(&self.app).unwrap_err();
        persistence::Archive::new(self.app.clone(), vec![], NOW)
            .encode()
            .unwrap_err()
    }
}

/// Study's mix starts the text as a lesson and introduces its first phrase
/// only. That phrase is studied, quizzed and answered: its result is held.
fn mix_started() -> (Owner, String) {
    let mut o = Owner::stored(App::new("recitation-scope-synthetic-csrf-012".into()));
    let canticle = json!({"title":"Synthetic Canticle","name":"Synthetic v1","kind":"text",
        "text":CANTICLE.concat(),
        "sections":[{"heading":"Opening","units":CANTICLE[..4]},
            {"heading":"Closing","units":CANTICLE[4..]}]});
    o.ok("/api/v1/library/versions", json!({"data":canticle}));
    let result = o.send(Client::App, "/practice/review/start", json!({}));
    result.unwrap();
    o.assert_recoverable();
    let sid = o.held().prompt.study_id;
    assert!(o.app.practice.mixed_review && o.app.practice.studies[&sid].lesson);
    assert_eq!(o.app.practice.cards.len(), 1, "One phrase of eight");
    o.learn_held_phrase();
    (o, sid)
}
fn recitations(sid: &str, after: &str, section: &str) -> Vec<(&'static str, Value)> {
    vec![
        (
            "/api/v1/lessons/next",
            json!({"after":after,"mode":"whole"}),
        ),
        ("/api/v1/lessons/next", json!({"after":after,"mode":"join"})),
        (
            "/api/v1/next",
            json!({"after":after,"study_id":sid,"recite":"whole"}),
        ),
        (
            "/api/v1/next",
            json!({"after":after,"study_id":sid,"recite":"section","section_id":section}),
        ),
    ]
}
fn section_of(o: &Owner, sid: &str, index: usize) -> String {
    o.app.library.versions[&o.app.practice.studies[sid].version_id].sections[index]
        .id
        .clone()
}

#[test]
fn a_recitation_over_phrases_not_yet_introduced_is_refused_and_writes_nothing() {
    for client in [Client::App, Client::Token] {
        let (mut o, sid) = mix_started();
        let after = o.held().prompt.id;
        for (path, data) in recitations(&sid, &after, &section_of(&o, &sid, 0)) {
            let before = o.app.clone();
            let refused = o.send(client, path, data.clone()).unwrap_err();
            assert_eq!(refused.status, 409, "{client:?} {path} {data}");
            assert_eq!(refused.message, REFUSAL, "{client:?} {path} {data}");
            assert_eq!(
                o.app, before,
                "{client:?} {path} {data}: nothing is written"
            );
            o.assert_recoverable();
        }
        assert_eq!(o.app.practice.cards.len(), 1, "No Unit is introduced");
        assert_eq!(o.app.practice.attempts.len(), 1);
    }
}

#[test]
fn a_refused_recitation_keeps_the_held_question_and_its_draft() {
    let (mut o, sid) = mix_started();
    let after = o.held().prompt.id;
    // The next phrase is held as a Quiz with an acknowledged draft.
    o.ok("/api/v1/next", json!({"after":after,"study_id":sid}));
    let source = o.held().prompt.id;
    o.ok("/api/v1/quiz", json!({"prompt_id":source}));
    let quiz = o.held().prompt.id;
    o.ok(
        "/api/v1/draft",
        json!({"prompt_id":quiz,"generation":"0","answer":"and quiet hills"}),
    );
    for client in [Client::App, Client::Token] {
        for (path, data) in recitations(&sid, &quiz, &section_of(&o, &sid, 0))
            .into_iter()
            .chain(recitations(&sid, "", &section_of(&o, &sid, 0)))
        {
            let before = o.app.clone();
            let refused = o.send(client, path, data.clone()).unwrap_err();
            assert_eq!(refused.status, 409, "{client:?} {path} {data}");
            assert_eq!(o.app, before, "{client:?} {path} {data}");
        }
    }
    let draft = o.app.practice.answer_checkpoint.clone().unwrap().draft;
    assert_eq!(draft.unwrap().answer, "and quiet hills");
    assert_eq!(o.held().prompt.id, quiz);
    o.assert_recoverable();
}

#[test]
fn a_section_is_recited_once_its_phrases_are_introduced_and_the_whole_still_waits() {
    for client in [Client::App, Client::Token] {
        let (mut o, sid) = mix_started();
        // Three more phrases arrive at Pace: the Section "Opening" is complete.
        for _ in 1..4 {
            let after = o.held().prompt.id;
            o.ok("/api/v1/next", json!({"after":after,"study_id":sid}));
            o.learn_held_phrase();
        }
        assert_eq!(o.app.practice.cards.len(), 4);
        let after = o.held().prompt.id;
        let before = o.app.clone();
        let whole = json!({"after":after,"mode":"whole"});
        let refused = o.send(client, "/api/v1/lessons/next", whole).unwrap_err();
        assert_eq!((refused.status, refused.message.as_str()), (409, REFUSAL));
        assert_eq!(o.app, before);

        let join = json!({"after":after,"mode":"join"});
        o.send(client, "/api/v1/lessons/next", join).unwrap();
        o.assert_recoverable();
        let held = o.held();
        assert_eq!(
            held.prompt.scope,
            Scope::Section {
                section_id: section_of(&o, &sid, 0)
            }
        );
        assert_eq!(held.phase, Some(StudyPhase::Quiz));
        assert_eq!(held.prompt.cue_level, Cue::SectionHeading);
        assert_eq!(held.prompt.cue, "Opening");
        assert_eq!(held.targets.len(), 4);
        assert_eq!(o.expected(), CANTICLE[..4].concat());
        assert_eq!(o.answer_exactly().effects.len(), 4);
        assert_eq!(o.app.practice.cards.len(), 4, "No Unit is introduced");
    }
}

#[test]
fn a_fully_introduced_text_is_recited_by_section_and_whole_as_before() {
    for client in [Client::App, Client::Token] {
        let (mut o, sid) = mix_started();
        // Entering the text's lesson makes its whole content available.
        let version = o.app.practice.studies[&sid].version_id.clone();
        o.ok("/api/v1/lessons/start", json!({"version_id":version}));
        assert_eq!(o.app.practice.cards.len(), 8);

        let after = o.held().prompt.id;
        let join = json!({"after":after,"mode":"join"});
        o.send(client, "/api/v1/lessons/next", join).unwrap();
        o.assert_recoverable();
        assert_eq!(o.held().prompt.cue_level, Cue::SectionHeading);
        assert_eq!(o.expected(), CANTICLE[..4].concat());
        assert_eq!(o.answer_exactly().effects.len(), 4);

        let after = o.held().prompt.id;
        let whole = json!({"after":after,"mode":"whole"});
        o.send(client, "/api/v1/lessons/next", whole).unwrap();
        o.assert_recoverable();
        let held = o.held();
        assert_eq!(held.prompt.scope, Scope::Whole);
        assert_eq!(held.phase, Some(StudyPhase::Quiz));
        assert_eq!(held.prompt.cue_level, Cue::WholeTitle);
        assert_eq!(held.prompt.cue, "Synthetic Canticle");
        assert_eq!(o.expected(), CANTICLE.concat());
        assert_eq!(o.answer_exactly().effects.len(), 8);

        for (recite, units) in [("whole", 8), ("section", 4)] {
            let after = o.held().prompt.id;
            let data = json!({"after":after,"study_id":sid,"recite":recite,
                "section_id":if recite == "section" { section_of(&o, &sid, 1) } else { String::new() }});
            o.send(client, "/api/v1/next", data).unwrap();
            o.assert_recoverable();
            assert_eq!(o.answer_exactly().effects.len(), units);
        }
    }
}

// Notebooks captured on the source before this change. See
// fixtures/partial-recitation-legacy.README.md.
fn earlier(state: &str) -> Owner {
    let fixture: Value =
        serde_json::from_str(include_str!("fixtures/partial-recitation-legacy.json")).unwrap();
    assert_eq!(
        fixture["source_commit"],
        "f87db8aa4a6f93e7b3f461dfe9ff88ab01d4a751"
    );
    Owner::stored(serde_json::from_value(fixture["states"][state].clone()).unwrap())
}

#[test]
fn a_recitation_held_by_the_earlier_source_is_never_recorded_and_can_be_left() {
    for (state, scope) in [("whole_held", 1), ("join_held", 1)] {
        for client in [Client::App, Client::Token] {
            let mut o = earlier(state);
            let held = o.held();
            assert!(matches!(
                held.prompt.scope,
                Scope::Whole | Scope::Section { .. }
            ));
            assert_eq!(held.targets.len(), scope, "One phrase of eight");
            assert_eq!(o.refused().status, 422, "{state} already blocks backup");
            let before = o.app.clone();
            let answer = o.expected();
            for (path, data) in [
                (
                    "/api/v1/answer",
                    json!({"prompt_id":held.prompt.id,"answer":answer}),
                ),
                ("/api/v1/reveal", json!({"prompt_id":held.prompt.id})),
                ("/api/v1/quiz", json!({"prompt_id":held.prompt.id})),
            ] {
                let refused = o.send(client, path, data).unwrap_err();
                assert_eq!(refused.status, 409, "{state} {client:?} {path}");
                assert_eq!(o.app, before, "{state} {client:?} {path}");
            }
            // Leaving it closes it: nothing was recorded against it, so the
            // notebook is recoverable again.
            let (path, leave) = match client {
                Client::App => ("/practice/review/start", json!({"leave":held.prompt.id})),
                Client::Token => (
                    "/api/v1/lessons/start",
                    json!({"starter":"nato","leave":held.prompt.id}),
                ),
            };
            o.send(client, path, leave).unwrap();
            assert_ne!(o.held().prompt.id, held.prompt.id);
            assert_eq!(o.app.practice.attempts, before.practice.attempts);
            o.assert_recoverable();
        }
    }
}

#[test]
fn recovery_still_refuses_a_recitation_the_earlier_source_recorded() {
    for state in ["whole_answered", "join_answered"] {
        let mut o = earlier(state);
        let held = o.held();
        let attempt = &o.app.practice.attempts[held.attempt_id.as_ref().unwrap()];
        assert_eq!(attempt.effects.len(), 1, "One phrase of eight");
        let refusal = o.refused();
        assert_eq!(refusal.status, 422, "{state}");
        assert_eq!(refusal.message, "Invalid frozen Study Prompt.");
        // History is never rewritten: moving on and dropping the Study keep
        // the Attempt, so the validation rule keeps refusing the notebook.
        let attempts = o.app.practice.attempts.clone();
        let sid = held.prompt.study_id.clone();
        let next = json!({"after":held.prompt.id,"study_id":sid});
        o.send(Client::Token, "/api/v1/next", next).unwrap();
        o.send(
            Client::Token,
            "/api/v1/studies/drop",
            json!({"study_id":sid}),
        )
        .unwrap();
        assert!(o.app.practice.current.is_none());
        assert_eq!(o.app.practice.attempts, attempts);
        assert_eq!(o.refused().status, 422, "{state}");
    }
}
