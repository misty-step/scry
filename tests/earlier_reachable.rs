//! Everything saved in the earlier concept notebook is reachable from the
//! Menu by following links, and no page of it is a control. For every state in
//! `tests/fixtures/earlier-notebook.json` this crawls the rendered pages from
//! the Menu's links, following each `href`, and asserts that every stored
//! Goal, Concept, note, prerequisite link, question version, choice, unapplied
//! draft, recorded answer, Correction, reset, feedback entry, saved check and
//! photo appears on a page it reached. Every recorded answer is reached on its
//! question's page: History's bytes are pinned by `tests/judgment_off.rs` and
//! are unchanged here. `qa/earlier-notebook.test.mjs` runs the same crawl
//! against the built Worker, and journey US-001 in a browser.
use scry::{model::*, persistence::Archive, web};
use serde_json::Value;
use std::collections::{BTreeMap, BTreeSet, VecDeque};

const FIXTURE: &str = concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/tests/fixtures/earlier-notebook.json"
);

fn stored() -> Vec<(String, App, i64)> {
    let fixture: BTreeMap<String, Value> =
        serde_json::from_str(&std::fs::read_to_string(FIXTURE).unwrap()).unwrap();
    assert_eq!(fixture.len(), 29);
    fixture
        .into_iter()
        .map(|(name, v)| {
            let archive = v["archive"].as_str().unwrap().as_bytes();
            let app = Archive::decode(archive, v["sha256"].as_str().unwrap())
                .unwrap()
                .app;
            (name, app, v["now_ms"].as_i64().unwrap() + 100_000)
        })
        .collect()
}

fn page(app: &App, path: &str, now: i64) -> String {
    web::render(app, path, &BTreeMap::new(), now, None)
}

/// Every `href` on a page, unescaped.
fn hrefs(html: &str) -> Vec<String> {
    html.split("href=\"")
        .skip(1)
        .filter_map(|rest| rest.split('"').next())
        .map(|href| href.replace("&amp;", "&"))
        .collect()
}

fn main_of(html: &str) -> &str {
    &html[html.find("<main").unwrap()..html.find("</main>").unwrap()]
}

/// The pages a signed-in Owner reaches from the Menu by following links,
/// keyed by path, and every link seen (fragments kept). Only pages the app
/// renders are opened: no export, photo bytes, asset, changelog or query.
struct Crawl {
    pages: BTreeMap<String, String>,
    links: BTreeSet<String>,
}

fn crawl(app: &App, now: i64) -> Crawl {
    let home = page(app, "/", now);
    let nav = home
        .split("aria-label=\"Main navigation\">")
        .nth(1)
        .and_then(|rest| rest.split("</nav>").next())
        .expect("the Menu");
    let mut queue: VecDeque<String> = hrefs(nav).into();
    let mut links: BTreeSet<String> = queue.iter().cloned().collect();
    let mut pages = BTreeMap::new();
    while let Some(href) = queue.pop_front() {
        let path = href.split('#').next().unwrap().to_string();
        if !path.starts_with('/')
            || path.starts_with("//")
            || path.contains('?')
            || path.starts_with("/assets/")
            || path.starts_with("/photos/")
            || path.starts_with("/api/")
            || ["/export", "/changelog"].contains(&path.as_str())
            || pages.contains_key(&path)
        {
            continue;
        }
        let html = page(app, &path, now);
        for link in hrefs(&html) {
            if links.insert(link.clone()) {
                queue.push_back(link);
            }
        }
        pages.insert(path, html);
    }
    Crawl { pages, links }
}

impl Crawl {
    fn page(&self, path: &str) -> &str {
        self.pages
            .get(path)
            .unwrap_or_else(|| panic!("{path} is not reachable from the Menu"))
    }
    /// One element opened by `id="{id}"`, up to the end of its article.
    fn entry(&self, path: &str, id: &str) -> &str {
        let html = self.page(path);
        let marker = format!("id=\"{id}\"");
        let start = html
            .find(&marker)
            .unwrap_or_else(|| panic!("{path} holds no {marker}"));
        let rest = &html[start..];
        &rest[..rest.find("</article>").unwrap_or(rest.len())]
    }
    fn anywhere(&self, text: &str) -> bool {
        self.pages.values().any(|html| html.contains(text))
    }
}

fn has(html: &str, text: &str, what: &str) {
    assert!(
        html.contains(&web::escape(text)),
        "{what}: missing {text:?}"
    );
}

/// The paragraphs of a note as the Concept page splits them.
fn paragraphs(text: &str) -> impl Iterator<Item = &str> {
    text.split("\n\n").filter(|p| !p.trim().is_empty())
}

fn content_shown(html: &str, c: &Content, what: &str) {
    for text in [&c.prompt, &c.answer, &c.explanation]
        .into_iter()
        .chain(&c.choices)
        .chain(&c.quotes)
    {
        has(html, text, what);
    }
    if !c.variants.is_empty() {
        has(html, &c.variants.join("\n"), what);
    }
}

/// Every stored thing of one notebook is on a page the crawl reached.
fn assert_reachable(name: &str, app: &App, now: i64) {
    let site = crawl(app, now);
    for g in app.goals.values() {
        let path = format!("/goals/{}", g.id);
        let html = site.page(&path);
        has(html, &g.title, &format!("{name} {path}"));
        has(html, &g.intent, &format!("{name} {path}"));
        if let Some(t) = &g.transcript {
            has(html, t, &format!("{name} {path}: transcription"));
        }
        if g.photo.is_some() {
            assert!(
                site.links.contains(&format!("/photos/{}", g.id)),
                "{name}: the photo of {} has no link",
                g.id
            );
        }
        assert_eq!(
            html.contains(" · Archived"),
            g.archived,
            "{name} {path}: archived label"
        );
    }
    for c in app.concepts.values() {
        let path = format!("/concepts/{}", c.id);
        let html = site.page(&path);
        let what = format!("{name} {path}");
        has(html, &c.name, &what);
        for note in &c.notes {
            for p in paragraphs(&note.text) {
                has(html, p, &format!("{what}: a note"));
            }
            for quote in &note.quotes {
                assert!(
                    html.contains(&format!("<blockquote>{}</blockquote>", web::escape(quote))),
                    "{what}: quoted evidence"
                );
            }
            if note.basis == "general" {
                assert!(html.contains("General knowledge"), "{what}: label");
            }
        }
        for p in &c.prerequisites {
            assert!(
                html.contains(&format!("href=\"/concepts/{p}\"")),
                "{what}: builds on {p}"
            );
            assert!(
                site.page(&format!("/concepts/{p}"))
                    .contains(&format!("href=\"/concepts/{}\"", c.id)),
                "{name}: {p} is used by {}",
                c.id
            );
        }
        // Each saved question, live or archived, is listed on its Concept.
        for q in app.questions.values().filter(|q| q.concept_id == c.id) {
            assert!(
                html.contains(&format!("href=\"/questions/{}/edit\"", q.id)),
                "{what}: lists {}",
                q.id
            );
        }
        if c.archived {
            assert!(html.contains("Archived"), "{what}: archived label");
        }
    }
    for q in app.questions.values() {
        let path = format!("/questions/{}/edit", q.id);
        let html = site.page(&path);
        let what = format!("{name} {path}");
        for v in &q.versions {
            content_shown(html, v, &format!("{what} version {}", v.version));
            if v.version != q.content().version {
                assert!(html.contains(&format!("id=\"version-{}\"", v.version)));
            }
        }
        if let Some(draft) = &q.draft {
            let entry = &html[html.find("id=\"draft\"").expect("the draft")..];
            assert!(entry.contains("never applied"), "{what}: draft label");
            content_shown(entry, draft, &format!("{what} draft"));
        }
    }
    for e in &app.events {
        let path = format!("/questions/{}/edit", e.question_id);
        let id = format!("attempt-{}", e.id);
        let entry = site.entry(&path, &id);
        let what = format!("{name} {path} {id}");
        assert!(
            entry.contains(&format!(" · Version {}", e.presentation.version)),
            "{what}: version"
        );
        if !e.answer.is_empty() {
            has(entry, &e.answer, &what);
        }
        let authority = match e.result.authority.as_str() {
            "exact" => "Exact match",
            "jev" => "Meaning checked",
            "reveal" => "Answer revealed",
            "learner" => "Self-checked",
            "ungraded" => "Saved, ungraded",
            _ => "Recorded",
        };
        assert!(entry.contains(authority), "{what}: Authority");
        for o in app.overrides.iter().filter(|o| o.event_id == e.id) {
            assert!(
                entry.contains(&format!(
                    "id=\"correction-{}\">Your Correction: counted as {}",
                    o.id,
                    if o.correct { "recalled" } else { "missed" }
                )),
                "{what}: its Correction"
            );
        }
        for a in app
            .assessments
            .values()
            .filter(|a| a.occurrence_id == e.occurrence_id)
        {
            assert!(
                entry.contains(&format!("id=\"check-{}\"", a.id)),
                "{what}: check {}",
                a.id
            );
        }
    }
    // Answers saved with no Event: the held one and any whose check stands alone.
    let graded: BTreeSet<_> = app
        .events
        .iter()
        .map(|e| e.occurrence_id.as_str())
        .collect();
    let mut saved: BTreeMap<&str, &str> = app
        .assessments
        .values()
        .filter(|a| !graded.contains(a.occurrence_id.as_str()))
        .map(|a| (a.occurrence_id.as_str(), a.question_id.as_str()))
        .collect();
    if let Some(o) = app
        .occurrence
        .as_ref()
        .filter(|o| o.answer.is_some() && !graded.contains(o.id.as_str()))
    {
        saved.insert(&o.id, &o.question_id);
    }
    for (occurrence, question) in &saved {
        let path = format!("/questions/{question}/edit");
        let id = format!("saved-{occurrence}");
        let entry = site.entry(&path, &id);
        assert!(entry.contains("Ungraded"), "{name} {id}");
    }
    for a in app.assessments.values() {
        let path = format!("/questions/{}/edit", a.question_id);
        let html = site.page(&path);
        assert!(
            html.contains(&format!(
                "id=\"check-{}\" data-check=\"{}\"",
                a.id, a.status
            )),
            "{name}: check {} by its stored status",
            a.id
        );
        has(html, &a.answer, &format!("{name}: check {}", a.id));
        if let Some(error) = &a.error {
            assert!(
                !site.anywhere(&web::escape(error)),
                "{name}: the message stored with check {} is shown",
                a.id
            );
        }
    }
    for r in &app.resets {
        let entry = site.entry(
            &format!("/questions/{}/edit", r.question_id),
            &format!("reset-{}", r.id),
        );
        assert!(entry.contains("Schedule reset"), "{name}: reset {}", r.id);
    }
    for f in &app.feedback {
        assert!(
            site.anywhere(&web::escape(&f.text)),
            "{name}: feedback {}",
            f.id
        );
    }
}

/// The richest notebook: archived Goal, Concept and question, a question with
/// two versions, a Correction, a reset with its feedback, an unapplied draft,
/// a photo with its transcription and a superseded check.
#[test]
fn everything_saved_in_history_rich_is_reachable_from_the_menu() {
    let (_, app, now) = stored()
        .into_iter()
        .find(|(name, _, _)| name == "history-rich")
        .unwrap();
    // The fixture holds what this proves.
    assert!(app.goals.values().any(|g| g.archived && g.photo.is_some()));
    assert!(app.concepts.values().any(|c| c.archived));
    assert!(app.questions.values().any(|q| q.archived));
    assert!(app.questions.values().any(|q| q.versions.len() == 2));
    assert!(app.questions.values().any(|q| q.draft.is_some()));
    assert!(!app.overrides.is_empty() && !app.resets.is_empty());
    assert!(app.assessments.values().any(|a| a.status == "superseded"));
    assert_reachable("history-rich", &app, now);
}

#[test]
fn everything_saved_in_every_state_is_reachable_from_the_menu() {
    for (name, app, now) in stored() {
        assert_reachable(&name, &app, now);
    }
}

/// No stored state has a Concept with two notes; a notebook that has one
/// shows both, oldest first, each with its own provenance.
#[test]
fn a_concept_page_shows_every_note_in_order() {
    let (_, mut app, now) = stored()
        .into_iter()
        .find(|(name, _, _)| name == "history-rich")
        .unwrap();
    let id = app
        .concepts
        .values()
        .find(|c| c.notes.last().is_some_and(|n| n.basis == "source"))
        .unwrap()
        .id
        .clone();
    let concept = app.concepts.get_mut(&id).unwrap();
    let first = concept.notes[0].clone();
    concept.notes.push(Note {
        text: "A later general note <em>about</em> this.".into(),
        basis: "general".into(),
        quotes: Vec::new(),
        created_ms: first.created_ms + 1,
        model: "synthetic".into(),
    });
    let html = crawl(&app, now)
        .page(&format!("/concepts/{id}"))
        .to_string();
    let main = main_of(&html);
    let earlier = main
        .find(&web::escape(paragraphs(&first.text).next().unwrap()))
        .unwrap();
    let later = main
        .find("A later general note &lt;em&gt;about&lt;/em&gt; this.")
        .unwrap();
    assert!(earlier < later, "notes in the order saved");
    assert_eq!(main.matches("class=\"concept-note\"").count(), 2);
    let notes: Vec<_> = main.split("class=\"concept-note\"").skip(1).collect();
    assert!(notes[0].contains("From your material") && notes[0].contains("<blockquote>"));
    assert!(notes[1].contains("General knowledge") && !notes[1].contains("<blockquote>"));
}

/// No page of the earlier notebook a crawl reaches holds a form, input,
/// button, text field or poll, in any stored state.
#[test]
fn no_reachable_earlier_page_is_a_control() {
    for (name, app, now) in stored() {
        let site = crawl(&app, now);
        let earlier = site.pages.iter().filter(|(path, _)| {
            path.as_str() == "/notebook"
                || path.as_str() == "/map"
                || path.as_str() == "/history"
                || path.starts_with("/goals/")
                || path.starts_with("/concepts/")
                || path.starts_with("/questions/")
        });
        let mut seen = 0;
        for (path, html) in earlier {
            seen += 1;
            let main = main_of(html);
            for control in ["<form", "<input", "<button", "<textarea", "data-poll"] {
                assert!(!main.contains(control), "{name} {path}: {control}");
            }
        }
        assert!(
            seen >= 3 + app.goals.len() + app.concepts.len() + app.questions.len(),
            "{name}: the crawl reached every earlier page"
        );
    }
}
