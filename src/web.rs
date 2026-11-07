//! The private, server-rendered notebook. All authored and generated text is escaped.
use crate::learning::{Attempt, ConceptState, QuestionEvidence, compute_concept_state};
use crate::model::*;
use sha2::{Digest, Sha256};
use std::collections::BTreeMap;
use std::fmt::Write;

pub fn escape(value: &str) -> String {
    let mut output = String::with_capacity(value.len());
    for ch in value.chars() {
        match ch {
            '&' => output.push_str("&amp;"),
            '<' => output.push_str("&lt;"),
            '>' => output.push_str("&gt;"),
            '"' => output.push_str("&quot;"),
            '\'' => output.push_str("&#39;"),
            _ => output.push(ch),
        }
    }
    output
}

// ADR 006 gives each imported quiz without an idea of its own a placeholder
// idea and a mechanical note that name their provenance ("Legacy import — quiz
// <id>"). The records keep that provenance; the screen shows the learner's own
// material instead.
const LEGACY_IDEA: &str = "legacy-import-quiz-concept-";
const LEGACY_LABEL: &str = "Legacy import";
const LEGACY_JOB_MARKER: &str = "legacy-import:";

fn legacy_idea(concept: &Concept) -> bool {
    concept.id.starts_with(LEGACY_IDEA) && concept.name.starts_with(LEGACY_LABEL)
}

/// The question an imported placeholder idea was assembled from.
fn legacy_prompt<'a>(app: &'a App, concept: &Concept) -> Option<&'a str> {
    app.questions
        .values()
        .filter(|q| q.concept_id == concept.id)
        .map(|q| q.content().prompt.trim())
        .find(|prompt| !prompt.is_empty())
}

/// An idea's title as the learner reads it: imported placeholders are named
/// by their own question rather than by an import label and quiz identifier.
fn idea_title(app: &App, concept: &Concept) -> String {
    if !legacy_idea(concept) {
        return concept.name.clone();
    }
    legacy_prompt(app, concept).map_or_else(|| "A saved idea".into(), |p| excerpt(p, 120))
}

fn idea_summary(concept: &Concept) -> &str {
    if legacy_idea(concept) {
        ""
    } else {
        &concept.summary
    }
}

/// The mechanical note repeats its label and question above the answer and
/// explanation; the question is already the idea's title.
fn note_text<'a>(app: &App, concept: &Concept, text: &'a str) -> &'a str {
    if !legacy_idea(concept) {
        return text;
    }
    let body = text
        .strip_prefix(LEGACY_LABEL)
        .map_or(text, str::trim_start);
    legacy_prompt(app, concept)
        .and_then(|prompt| body.strip_prefix(prompt))
        .filter(|rest| rest.is_empty() || rest.starts_with('\n'))
        .map_or(body, str::trim_start)
}

/// One line of at most `limit` characters, cut at a word where possible.
fn excerpt(text: &str, limit: usize) -> String {
    let line = text.split_whitespace().collect::<Vec<_>>().join(" ");
    if line.chars().count() <= limit {
        return line;
    }
    let cut: String = line.chars().take(limit).collect();
    let cut = match cut.rfind(' ') {
        Some(space) if space > cut.len() / 2 => &cut[..space],
        _ => cut.as_str(),
    };
    format!("{}…", cut.trim_end_matches([' ', ',', ';', ':']))
}

struct View<'a> {
    app: &'a App,
    path: &'a str,
    query: &'a BTreeMap<String, String>,
    now: i64,
}

#[derive(Clone, Copy)]
enum EmptyState<'a> {
    FirstRun,
    Preparing,
    Paused,
    Stopped(&'a Goal),
    CaughtUp,
}

impl View<'_> {
    fn effective_phase<'a>(&self, phase: &'a str, error: Option<&str>) -> &'a str {
        if error.is_some_and(|error| error.starts_with(LEGACY_JOB_MARKER)) {
            "imported-unfinished"
        } else if self.app.restored_paused
            && matches!(
                phase,
                "queued"
                    | "pending"
                    | "preparing"
                    | "running"
                    | "generating"
                    | "critic"
                    | "sent"
                    | "candidates"
                    | "critic-sent"
                    | "unknown"
                    | "failed"
                    | "blocked"
                    | "paused"
                    | "budget"
            )
        {
            "notebook-paused"
        } else {
            phase
        }
    }
    fn goal_phase<'a>(&'a self, goal: &'a Goal) -> &'a str {
        // Imported material can be ready even when the latest Go job never finished.
        if goal.status == "ready" {
            return "ready";
        }
        let job = self.app.jobs.get(&goal.job_id);
        self.effective_phase(
            job.map_or(&goal.status, |j| &j.status),
            job.and_then(|j| j.error.as_deref()),
        )
    }
    fn check_paused(&self, occurrence: &Occurrence) -> bool {
        self.effective_phase(
            occurrence
                .assessment_id
                .as_ref()
                .and_then(|id| self.app.assessments.get(id))
                .map(|assessment| assessment.status.as_str())
                .unwrap_or("queued"),
            None,
        ) == "notebook-paused"
    }
    fn resume_link(&self) -> &'static str {
        "<a class=\"text-link\" href=\"/settings#preparation\">Resume meaning checks →</a>"
    }
    fn saved_answer(&self, prompt_id: &str) -> &str {
        self.app
            .practice
            .answer_checkpoint
            .as_ref()
            .and_then(|checkpoint| checkpoint.draft.as_ref())
            .filter(|draft| draft.prompt_id == prompt_id)
            .map_or("", |draft| draft.answer.as_str())
    }
    fn form(&self, action: &str, suffix: &str, class: &str, extra: &str, body: &str) -> String {
        let nonce = self.query.get("__nonce").map(String::as_str).unwrap_or("");
        let identity = format!(
            "{nonce}:{}:{}:{}:{action}:{suffix}",
            self.now, self.app.revision, self.path
        );
        let operation = hex::encode(Sha256::digest(identity.as_bytes()));
        let checkpoint = if action == "/practice/answer"
            && class.split_whitespace().any(|c| c == "answer-form")
        {
            let generation = self
                .app
                .practice
                .answer_checkpoint
                .as_ref()
                .map_or(0, |checkpoint| checkpoint.generation);
            let has_draft = self
                .app
                .practice
                .answer_checkpoint
                .as_ref()
                .is_some_and(|checkpoint| checkpoint.draft.is_some());
            let discard = format!(
                "<input type=\"submit\" class=\"button\" formaction=\"/practice/draft/discard\" formnovalidate data-checkpoint-discard data-checkpoint-present=\"{has_draft}\" value=\"Discard saved draft\" hidden>"
            );
            format!(
                "<input type=\"submit\" class=\"button\" formaction=\"/practice/draft\" data-checkpoint-save value=\"Save draft\" hidden>{discard}<p data-checkpoint-custody hidden>Saved drafts are visible to anyone with access to this notebook.</p><span hidden data-checkpoint-generation data-generation=\"{generation}\"></span>"
            )
        } else {
            String::new()
        };
        format!(
            "<form method=\"post\" action=\"{}\" class=\"{}\" {}>{}{}{}{body}{checkpoint}</form>",
            escape(action),
            escape(class),
            extra,
            hidden("csrf", &self.app.csrf),
            hidden("operation_id", &operation),
            hidden("revision", &self.app.revision.to_string())
        )
    }
    fn review_form(
        &self,
        action: &str,
        suffix: &str,
        class: &str,
        extra: &str,
        body: &str,
    ) -> String {
        let id = self
            .app
            .occurrence
            .as_ref()
            .map(|o| o.id.as_str())
            .unwrap_or("");
        self.form(
            &format!("/review/{action}"),
            suffix,
            class,
            extra,
            &format!("{}{body}", hidden("occurrence_id", id)),
        )
    }
    fn goal_title(&self, goal: &Goal) -> String {
        if self.app.protected_goal() == Some(goal.id.as_str()) {
            "Current study".to_string()
        } else {
            escape(&goal.title)
        }
    }
    fn concept_name(&self, concept: &Concept) -> String {
        if self.app.protected_concept(&concept.id) {
            "Current idea".into()
        } else {
            escape(&idea_title(self.app, concept))
        }
    }
    fn active_concept(&self, concept: &Concept) -> bool {
        !concept.archived
            && self
                .app
                .goals
                .get(&concept.goal_id)
                .is_some_and(|goal| !goal.archived)
    }
    /// Above a question, an imported placeholder is named by its goal: its own
    /// title would repeat the question directly beneath it.
    fn study_chip(&self, concept: &Concept) -> (String, String) {
        let goal = self.app.goals.get(&concept.goal_id);
        match goal {
            Some(g) if legacy_idea(concept) && !self.app.protected_concept(&concept.id) => {
                (self.goal_title(g), format!("/goals/{}", escape(&g.id)))
            }
            _ => (
                self.concept_name(concept),
                format!("/concepts/{}", escape(&concept.id)),
            ),
        }
    }
    fn notes(&self, concept: &Concept) -> String {
        notes(self.app, concept)
    }
    fn lede(&self, class: &str, concept: &Concept) -> String {
        let summary = idea_summary(concept);
        if summary.trim().is_empty() {
            String::new()
        } else {
            format!("<p class=\"{class}\">{}</p>", escape(summary))
        }
    }
    fn gate(&self, destination: &str) -> String {
        if self
            .app
            .occurrence
            .as_ref()
            .is_some_and(|occurrence| occurrence.phase == "checking")
        {
            if self
                .app
                .occurrence
                .as_ref()
                .is_some_and(|o| self.check_paused(o))
            {
                return format!(
                    "<section class=\"empty-stage gate\" data-state=\"checking-paused\"><p class=\"eyebrow\">Your answer is saved</p><h1>Your meaning check is paused.</h1><p class=\"lede\">Resume meaning checks in Settings to continue. Your answer stays ungraded and this reference stays protected.</p>{}</section>",
                    self.resume_link()
                );
            }
            return format!(
                "<section class=\"empty-stage gate\" data-state=\"checking-reference\" data-poll>{}<p class=\"eyebrow\">Your answer is saved</p><h1>Your check is still in progress.</h1><p class=\"lede\">This reference may contain the answer. It will be available after the check finishes.</p><div class=\"actions\"><a class=\"button primary\" href=\"{}\">Return to your saved answer →</a></div></section>",
                emblem("large"),
                legacy_home(self.app)
            );
        }
        let action = self.review_form(
            "help",
            "reference",
            "",
            "",
            &format!(
                "{}<button class=\"button primary\">Look it up</button>",
                hidden("return_to", destination)
            ),
        );
        format!(
            "<section class=\"empty-stage gate\" data-state=\"gate\">{}<p class=\"eyebrow\">A little help is part of learning</p><h1>Open your reference?</h1><p class=\"lede\">Your notes may contain the answer. Opening them records this attempt as helped practice.</p><div class=\"actions\">{action}<a class=\"button quiet\" href=\"{}\">Keep trying</a></div></section>",
            emblem("large"),
            legacy_home(self.app)
        )
    }
    fn pending(&self) -> String {
        let mut out = String::new();
        for goal in self
            .app
            .goals
            .values()
            .filter(|g| !g.archived && g.status != "ready")
        {
            let text = "Saved preparation history · generation retired";
            let _ = write!(
                out,
                "<a class=\"preparation-receipt stopped\" href=\"/goals/{}\"><span class=\"status-dot\" aria-hidden=\"true\"></span><span><strong>{}</strong><span>{text}</span></span><span aria-hidden=\"true\">↗</span></a>",
                escape(&goal.id),
                self.goal_title(goal)
            );
        }
        if out.is_empty() {
            out
        } else {
            format!("<aside class=\"preparations\" aria-label=\"Saved preparations\">{out}</aside>")
        }
    }
    fn practice_form(&self, action: &str, suffix: &str, extra: &str, body: &str) -> String {
        let id = self
            .app
            .practice
            .current
            .as_ref()
            .map_or("", |p| p.prompt.id.as_str());
        self.form(
            &format!("/practice/{action}"),
            suffix,
            "exact-form",
            extra,
            &format!("{}{body}", hidden("prompt_id", id)),
        )
    }
    fn library_gate(&self) -> String {
        let destination = if library_destination(self.path) {
            self.path
        } else {
            "/library"
        };
        let action = self.practice_form(
            "reveal",
            "library-help",
            "",
            &format!(
                "{}<button class=\"button primary\">Open Library with help</button>",
                hidden("return_to", destination)
            ),
        );
        format!(
            "<section class=\"empty-stage gate\" data-state=\"library-gate\"><h1>Open your Library?</h1><p class=\"lede\">It contains the words you are recalling. Opening it records help for your current Prompt, without changing its schedule.</p><div class=\"actions\">{action}<a class=\"button quiet\" href=\"/recall\">Keep trying</a></div></section>"
        )
    }
    fn library(&self) -> String {
        if self.app.practice.protected() {
            return self.library_gate();
        }
        let q = self.query.get("q").map(String::as_str).unwrap_or("");
        let found = self.app.library.search(q);
        let mut out = format!(
            "<section class=\"library-page\" data-state=\"library\"><h1>Library</h1><form method=\"get\" action=\"/library\" class=\"library-search\"><label for=\"library-query\">Find a text</label><div class=\"search-controls\"><input id=\"library-query\" name=\"q\" type=\"search\" value=\"{}\" placeholder=\"Title, alias or opening words\"><button class=\"button quiet\">Search</button></div></form>",
            escape(q)
        );
        if !self
            .app
            .library
            .versions
            .values()
            .any(crate::library::is_nato)
        {
            out.push_str(&format!(
            "<section class=\"library-work\" id=\"nato-starter\"><h2>NATO spelling alphabet</h2><p class=\"fineprint\">26 letters · existing progress is kept</p>{}</section>",
            self.form(
                "/library/mapping/add",
                "nato-starter",
                "library-add-form",
                "",
                &format!(
                    "{}<button class=\"button secondary\">Add NATO alphabet</button>",
                    hidden("starter", "nato-v1")
                )
            )
        ));
        }
        if found.empty {
            out.push_str(
                if q.trim().is_empty() && self.app.library.works.is_empty() {
                    "<p class=\"lede\">Start with a text you want to know by heart.</p>"
                } else {
                    "<p class=\"lede\" role=\"status\">Nothing matches.</p>"
                },
            );
        } else {
            out.push_str("<div class=\"library-results\">");
            for item in found.matches {
                let _ = write!(
                    out,
                    "<section class=\"library-work\"><h2>{}</h2>",
                    escape(&item.work.title)
                );
                if !item.work.aliases.is_empty() {
                    let _ = write!(
                        out,
                        "<p class=\"fineprint\">Also known as {}</p>",
                        escape(&item.work.aliases.join(" · "))
                    );
                }
                out.push_str("<ul class=\"library-versions\">");
                for v in item.versions {
                    let _ = write!(
                        out,
                        "<li><a class=\"version-link\" href=\"/library/versions/{}\">{}</a><p class=\"fineprint\">{}</p>{}</li>",
                        escape(&v.id),
                        escape(version_name(v)),
                        version_metadata(v),
                        self.study_status(&v.id)
                    );
                }
                out.push_str("</ul></section>");
            }
            out.push_str("</div>");
        }
        out.push_str("<a class=\"button primary\" href=\"/library/add\">Add a text</a></section>");
        out
    }
    fn library_add(&self) -> String {
        if self.app.practice.protected() {
            return self.library_gate();
        }
        let value = |key| escape(self.query.get(key).map(String::as_str).unwrap_or(""));
        let details = if self.query.get("name").is_some_and(|v| !v.is_empty())
            || self.query.get("source").is_some_and(|v| !v.is_empty())
        {
            " open"
        } else {
            ""
        };
        let body = format!(
            "<label for=\"text-title\">Title</label><input id=\"text-title\" name=\"title\" value=\"{}\" maxlength=\"240\" required><label for=\"supplied-text\">Text</label><textarea id=\"supplied-text\" name=\"text\" rows=\"10\" maxlength=\"262144\" required aria-describedby=\"text-note\">{}</textarea><p id=\"text-note\" class=\"fineprint\">Exact wording · private · up to 256 KiB.</p><details class=\"library-details\"{details}><summary>Edition and source</summary><label for=\"edition\">Edition</label><input id=\"edition\" name=\"name\" value=\"{}\" maxlength=\"240\"><label for=\"text-source\">Source</label><input id=\"text-source\" name=\"source\" value=\"{}\" maxlength=\"2048\"><p class=\"fineprint\">Optional. A source is recorded as supplied, not fetched or verified.</p></details><button class=\"button primary\">Save text</button>",
            value("title"),
            value("text"),
            value("name"),
            value("source")
        );
        format!(
            "<section class=\"library-page\" data-state=\"library-add\"><a class=\"text-link\" href=\"/library\">Back to Library</a><h1>Add a text</h1>{}</section>",
            self.form(
                "/library/version/add",
                "supplied-text",
                "library-add-form",
                "",
                &body
            )
        )
    }
    fn study_status(&self, version_id: &str) -> String {
        self.app
            .practice
            .studies
            .values()
            .find(|s| s.version_id == version_id)
            .map_or_else(String::new, |s| {
                let status = match s.status {
                    crate::study::Status::Active => "Learning",
                    crate::study::Status::Paused => "Paused",
                    crate::study::Status::Dropped => "Dropped · history kept",
                };
                format!("<p class=\"fineprint\">{status}</p>")
            })
    }
    fn library_version(&self, id: &str) -> String {
        if self.app.practice.protected() {
            return self.library_gate();
        }
        let Ok(item) = self.app.library.show_version(id) else {
            return self.not_found();
        };
        let v = item.version;
        let mut out = format!(
            "<section class=\"library-page\" data-state=\"library-version\"><a class=\"text-link\" href=\"/library\">Back to Library</a><h1>{}</h1><p class=\"version-name\">{}</p><details class=\"library-details version-details\"><summary>Edition &amp; source</summary><p class=\"fineprint\">{}</p>",
            escape(&item.work.title),
            escape(version_name(v)),
            version_metadata(v)
        );
        if !v.sources.is_empty() {
            let _ = write!(
                out,
                "<p class=\"fineprint\">Source: {}</p>",
                escape(&v.sources.join(" · "))
            );
        }
        if !item.work.context.is_empty() {
            let _ = write!(
                out,
                "<p class=\"fineprint source-context\">{}</p>",
                escape(&item.work.context)
            );
        }
        let rights = match v.rights {
            crate::library::Rights::PrivateOnly => "Private only",
            crate::library::Rights::Licensed => "Licensed",
            crate::library::Rights::PublicDomain => "Public domain",
        };
        let _ = write!(
            out,
            "<p class=\"fineprint\">Supplied text · {rights}</p></details>"
        );
        let reviewable = crate::study::reviewable(v).is_ok();
        if !reviewable && v.kind == crate::library::Kind::Text {
            out.push_str("<p role=\"status\">Practice is unavailable because a Unit has no words to recall. Saved text and history are unchanged.</p>");
        }
        if let Some(study) = self
            .app
            .practice
            .studies
            .values()
            .find(|s| s.version_id == id)
        {
            out.push_str(&self.study_status(id));
            out.push_str("<div class=\"actions\"><a class=\"button primary\" href=\"/recall\">Return to practice</a></div><details class=\"library-details\"><summary>Manage Study</summary><p class=\"fineprint\">Pause stops new Units; due practice remains. Drop stops this Study. Both keep its history.</p><div class=\"actions\">");
            let (action, label) = if study.status == crate::study::Status::Active {
                ("pause", "Pause")
            } else {
                ("resume", "Resume")
            };
            out.push_str(&self.form(
                &format!("/practice/{action}"),
                action,
                "",
                "",
                &format!(
                    "{}<button class=\"button quiet\">{label}</button>",
                    hidden("study_id", &study.id)
                ),
            ));
            if study.status != crate::study::Status::Dropped {
                out.push_str(&self.form(
                    "/practice/drop",
                    "drop",
                    "",
                    "",
                    &format!(
                        "{}<button class=\"text-button\">Drop</button>",
                        hidden("study_id", &study.id)
                    ),
                ));
            }
            out.push_str("</div></details>");
        } else if reviewable {
            let body = format!(
                "{}<details class=\"library-details\"><summary>Pace: 6 new Units a day</summary><fieldset><legend>New Units each day</legend><label><input type=\"radio\" name=\"pace\" value=\"light\"> 3 · Light</label><label><input type=\"radio\" name=\"pace\" value=\"steady\" checked> 6 · Steady</label><label><input type=\"radio\" name=\"pace\" value=\"intense\"> 12 · Intense</label></fieldset></details><button class=\"button primary\">Learn</button>",
                hidden("version_id", id)
            );
            out.push_str(&self.form("/practice/study/start", "learn", "library-learn", "", &body));
        } else if v.kind != crate::library::Kind::Text {
            out.push_str("<p class=\"fineprint\">Practice for this kind is not available yet.</p>");
        }
        let mapping = v.kind == crate::library::Kind::KeyedMapping
            && crate::library::validate_mapping(v).is_ok();
        out.push_str("<div class=\"library-text\" aria-label=\"Saved text\">");
        for section in &v.sections {
            let _ = write!(out, "<section><h2>{}</h2>", escape(&section.heading));
            if mapping {
                out.push_str("<dl class=\"mapping-pairs\">");
                for unit in &section.units {
                    let (key, value) = crate::library::mapping_pair(&unit.text).unwrap();
                    let _ = write!(
                        out,
                        "<div><dt>{}</dt><dd>{}</dd></div>",
                        escape(key),
                        escape(value)
                    );
                }
                out.push_str("</dl>");
            } else {
                for unit in &section.units {
                    let _ = write!(out, "<p class=\"reading\">{}</p>", escape(&unit.text));
                }
            }
            out.push_str("</section>");
        }
        out.push_str("</div></section>");
        out
    }
    fn mapping_starter_home(&self, starter: &str) -> String {
        let Some(lesson) = crate::lessons::catalog()
            .into_iter()
            .find(|l| l.id == starter && l.input.kind == crate::library::Kind::KeyedMapping)
        else {
            return self.not_found();
        };
        if self.app.practice.current.is_some() {
            return self.exact_stream();
        }
        let study = self
            .app
            .practice
            .studies
            .values()
            .find(|s| lesson.matches(&self.app.library.versions[&s.version_id]));
        let next = study.and_then(|s| {
            self.app
                .practice
                .cards
                .values()
                .filter(|c| c.study_id == s.id)
                .map(|c| c.state.schedule.due_ms)
                .min()
        });
        let ready = study.is_none_or(|s| crate::study::study_ready(self.app, s, self.now));
        let timing = next.map_or_else(
            || "Start with a few short questions.".into(),
            |at| {
                if at <= self.now {
                    "Ready to review.".into()
                } else {
                    format!("Next review {} (UTC).", date(at))
                }
            },
        );
        let mut out = format!(
            "<section class=\"nato-screen\" data-state=\"mapping-starter-home\"><h1>{}</h1><p>{}</p>",
            escape(&lesson.input.title),
            escape(&timing)
        );
        if ready {
            out.push_str(&self.form(
                "/practice/lesson/start",
                "mapping-starter-start",
                "",
                "",
                &format!(
                    "{}<button class=\"button primary\">Continue</button>",
                    hidden("starter", starter)
                ),
            ));
        }
        out.push_str(&self.lesson_picker(false));
        out.push_str("</section>");
        out
    }
    fn unavailable_practice(&self, p: &crate::study::Presentation) -> String {
        let mut out = format!(
            "<section class=\"empty-stage exact-study\" data-state=\"practice-unavailable\" data-prompt-id=\"{}\"><h1>Practice unavailable</h1><p>This Version has a Unit with no words to recall. Saved text and history are unchanged.</p>",
            escape(&p.prompt.id)
        );
        let draft = self.saved_answer(&p.prompt.id);
        if !draft.is_empty() {
            let _ = write!(
                out,
                "<label for=\"unavailable-draft\">Saved answer</label><textarea id=\"unavailable-draft\" readonly>{}</textarea><p>Copy your saved answer before switching.</p>",
                escape(draft)
            );
        }
        if p.attempt_id.is_none() {
            out.push_str("<p>Switching leaves this question ungraded.</p>");
        }
        out.push_str(&self.form(
            "/practice/review/start",
            "unavailable-next",
            "",
            "",
            &format!(
                "{}<button class=\"button primary\">Study other material</button>",
                hidden(
                    if p.attempt_id.is_some() {
                        "after"
                    } else {
                        "leave"
                    },
                    &p.prompt.id
                )
            ),
        ));
        if p.attempt_id.is_some() {
            out.push_str("<a class=\"text-link\" href=\"/history\">View saved history</a>");
        }
        out.push_str("</section>");
        out
    }
    fn exact_stream(&self) -> String {
        use crate::study::Status;
        let Some(p) = &self.app.practice.current else {
            let due = crate::study::due(self.app, self.now);
            let next = due["next_due_ms"].as_i64();
            let message = next.map_or_else(
                || "No next due time is scheduled. Resume a Study to introduce more Units.".into(),
                |at| format!("Next due at {} (UTC).", date(at)),
            );
            return format!(
                "<section class=\"empty-stage exact-study\" data-state=\"exact-caught-up\"><p class=\"eyebrow\">Exact recall</p><h1>A good place to pause.</h1><p class=\"lede\">No Unit is due. {}</p><a class=\"button primary\" href=\"/library\">Open Library</a></section>",
                escape(&message)
            );
        };
        if crate::study::reviewable(&self.app.library.versions[&p.prompt.version_id]).is_err() {
            return self.unavailable_practice(p);
        }
        let prompt = &p.prompt;
        let attempt = p
            .attempt_id
            .as_ref()
            .and_then(|id| self.app.practice.attempts.get(id));
        let state = if attempt.is_some() {
            "exact-result"
        } else {
            "exact-prompt"
        };
        let readiness = crate::lessons::catalog().into_iter().find(|l| {
            l.id.starts_with("baby-") && l.matches(&self.app.library.versions[&prompt.version_id])
        });
        let mut out = format!(
            "<section class=\"study exact-study{}\" data-state=\"{state}\" data-prompt-id=\"{}\">",
            if readiness.is_some() {
                " mapping-study readiness-study"
            } else if prompt.cue_level == crate::study::Cue::MappingKey {
                " mapping-study"
            } else {
                ""
            },
            escape(&prompt.id)
        );
        if self.app.practice.mixed_review {
            out.push_str("<p class=\"eyebrow\">All subjects</p>");
        }
        if let Some(a) = attempt {
            out.push_str(&self.exact_result(a));
        } else if p.source_visible() {
            let _ = write!(
                out,
                "<h1>{}</h1>",
                escape(if prompt.cue_level == crate::study::Cue::MappingKey {
                    &prompt.cue
                } else {
                    crate::study::visible_prompt_title(prompt)
                })
            );
            out.push_str(&self.source_study(p));
        } else {
            out.push_str("<span data-study-phase=\"quiz\" hidden></span>");
            if let Some(lesson) = &readiness {
                let _ = write!(
                    out,
                    "<p class=\"eyebrow\">{}</p><div class=\"exact-cue\"><h1 class=\"reading\">{}</h1></div>",
                    escape(&lesson.input.title),
                    escape(&prompt.cue)
                );
            } else {
                let _ = write!(
                    out,
                    "<p class=\"eyebrow\">Exact recall</p><h1>{}</h1>",
                    escape(crate::study::visible_prompt_title(prompt))
                );
                out.push_str(&Self::exact_prompt_context(prompt));
            }
            if prompt.assisted || p.revealed {
                out.push_str("<p class=\"helped-label\">Assisted practice · the schedule stays unchanged.</p>");
            }
            if p.revealed && p.phase != Some(crate::study::StudyPhase::Quiz) {
                out.push_str("<section class=\"exact-disclosure\"><h2>The Unit</h2><p class=\"fineprint\">Assistance is recorded.</p>");
                out.push_str(&self.exact_expected());
                out.push_str("</section>");
            }
            let mapping = prompt.cue_level == crate::study::Cue::MappingKey;
            let saved = escape(self.saved_answer(&prompt.id));
            let answer = if mapping {
                format!(
                    "<label class=\"answer-label\" for=\"answer\">Type your answer</label><input id=\"answer\" name=\"answer\" type=\"text\" maxlength=\"32768\" enterkeyhint=\"done\" spellcheck=\"false\" autocapitalize=\"none\" autocorrect=\"off\" autocomplete=\"off\" value=\"{saved}\">"
                )
            } else {
                format!(
                    "<label class=\"answer-label\" for=\"answer\">Your answer</label><textarea id=\"answer\" name=\"answer\" rows=\"4\" maxlength=\"32768\" enterkeyhint=\"done\" spellcheck=\"false\" autocapitalize=\"sentences\">\n{saved}</textarea>"
                )
            };
            out.push_str(&self.form(
                "/practice/answer",
                "exact-answer",
                "answer-form exact-answer-form",
                "data-answer-limit=\"32768\"",
                &format!(
                    "{}{answer}<button class=\"button primary\">Check my answer</button>",
                    hidden("prompt_id", &prompt.id)
                ),
            ));
            if !p.revealed || p.phase == Some(crate::study::StudyPhase::Quiz) {
                out.push_str("<details class=\"overflow\"><summary>More</summary>");
                out.push_str(&self.practice_form(
                    "reveal",
                    "exact-reveal",
                    "",
                    "<button class=\"text-button\">Show me</button>",
                ));
                out.push_str("</details>");
            }
        }
        if readiness.is_some() {
            if attempt.is_some() || p.revealed {
                let _ = write!(
                    out,
                    "<a class=\"text-link\" href=\"/library/versions/{}\">Source notes</a>",
                    escape(&prompt.version_id)
                );
            }
            out.push_str(&self.lesson_picker(false));
        } else if self
            .app
            .practice
            .studies
            .values()
            .any(|s| s.status != Status::Dropped)
        {
            out.push_str(
                "<a class=\"text-link exact-notebook\" href=\"/notebook\">Concept practice →</a>",
            );
        }
        out.push_str("</section>");
        out
    }
    fn source_study(&self, p: &crate::study::Presentation) -> String {
        let mut out = String::from(
            "<section class=\"source-study\" data-study-phase=\"study\"><p class=\"eyebrow\">Study</p>",
        );
        out.push_str(&self.exact_expected());
        out.push_str(&self.form(
            "/practice/quiz",
            "memory-quiz",
            "study-to-quiz",
            "",
            &format!(
                "{}<button class=\"button primary\">Try from memory</button>",
                hidden("prompt_id", &p.prompt.id)
            ),
        ));
        out.push_str("</section>");
        out
    }
    fn exact_prompt_context(prompt: &crate::study::Prompt) -> String {
        use crate::study::Cue;
        let mut out = String::new();
        if let Some(context) = &prompt.context {
            let _ = write!(
                out,
                "<div class=\"exact-context\"><p class=\"eyebrow\">Previous Unit</p><p class=\"reading\">{}</p></div>",
                escape(context)
            );
        }
        if !prompt.cue.is_empty() {
            let label = match prompt.cue_level {
                Cue::Read => "Read the Unit",
                Cue::FirstLetters => "First letters",
                Cue::FirstLetter => "First letter",
                Cue::PreviousLine => "Cue",
                Cue::SectionHeading => "Section",
                Cue::WholeTitle => "Title",
                Cue::MappingKey => "Key",
            };
            let _ = write!(
                out,
                "<div class=\"exact-cue\"><p class=\"eyebrow\">{label}</p><p class=\"reading\">{}</p></div>",
                escape(&prompt.cue)
            );
        }
        out
    }
    fn exact_expected(&self) -> String {
        let Some(p) = &self.app.practice.current else {
            return String::new();
        };
        let version = &self.app.library.versions[&p.prompt.version_id];
        version
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .filter(|u| p.prompt.unit_ids.contains(&u.id))
            .map(|u| {
                format!(
                    "<p class=\"reading exact-expected\">{}</p>",
                    escape(crate::study::expected_unit(version, u))
                )
            })
            .collect()
    }
    fn exact_result(&self, a: &crate::study::Attempt) -> String {
        use crate::grading::{Change, Verdict};
        let good = a.grade.verdict == Verdict::Good;
        let verdict = if good { "Good" } else { "Again" };
        let mut out = format!(
            "<div class=\"feedback feedback-{}\" role=\"status\"><h1 data-verdict=\"{verdict}\">{verdict}.</h1></div>",
            if good { "correct" } else { "miss" },
        );
        let mapping = a.prompt.cue_level == crate::study::Cue::MappingKey;
        if mapping {
            let _ = write!(
                out,
                "<p class=\"mapping-result\"><span>{}</span> <span aria-hidden=\"true\">→</span> <strong>{}</strong></p>",
                escape(&a.prompt.cue),
                escape(crate::study::expected_unit(
                    &self.app.library.versions[&a.version_id],
                    self.app.library.versions[&a.version_id]
                        .sections
                        .iter()
                        .flat_map(|s| &s.units)
                        .find(|u| u.id == a.prompt.unit_ids[0])
                        .unwrap()
                ))
            );
            if let Some(feedback) = crate::study::mapping_feedback(self.app, a) {
                if let Some(answered_key) = feedback.answered_key {
                    let _ = write!(
                        out,
                        "<p class=\"fineprint\">That answer belongs to key {}.</p>",
                        escape(&answered_key)
                    );
                } else if feedback.kind == "spelling" {
                    out.push_str(
                        "<p class=\"fineprint\">Spelling mismatch: use the answer shown above.</p>",
                    );
                }
            }
            if !good && self.app.library.versions[&a.version_id].spelling_strict {
                out.push_str(if crate::lessons::mapping_route(&self.app.library.versions[&a.version_id]).is_some() { "<p class=\"fineprint\">Use the short answer shown above.</p>" } else { "<p class=\"fineprint\">Official spelling matters. Scry checks your answer; no self-rating.</p>" });
            }
        }
        let changed = a
            .grade
            .diff
            .iter()
            .filter(|d| !matches!(d.change, Change::Correct { .. }))
            .collect::<Vec<_>>();
        if !changed.is_empty() {
            out.push_str("<ol class=\"exact-diff exact-changes\" aria-label=\"Words to revisit\">");
            for d in changed {
                out.push_str(&Self::exact_diff_word(d, true, a.grade.units.len() > 1));
            }
            out.push_str("</ol>");
        }
        if let Some(hint) = &a.grade.wrong_version_hint {
            let _ = write!(
                out,
                "<p class=\"notice\">Your answer is closer to Version {}.</p>",
                escape(&hint.name)
            );
        }
        if a.assisted {
            out.push_str(
                "<p class=\"helped-label\">Assisted practice · the schedule stayed unchanged.</p>",
            );
        } else if a.effects.iter().all(|e| e.extra_practice) {
            out.push_str("<p class=\"fineprint\">Extra practice · schedule unchanged.</p>");
        }
        let _ = write!(
            out,
            "<details class=\"exact-full-answer\"><summary>Full answer</summary><h2>{}</h2>{}<div class=\"answer-pair\"><span>You answered</span><p>{}</p></div><h2>Expected text</h2>{}<h2>Word by word</h2><ol class=\"exact-diff\" aria-label=\"Diff\">",
            escape(&a.prompt.title),
            Self::exact_prompt_context(&a.prompt),
            escape(&a.raw_answer),
            self.exact_expected()
        );
        for d in &a.grade.diff {
            out.push_str(&Self::exact_diff_word(d, false, a.grade.units.len() > 1));
        }
        let _ = write!(
            out,
            "</ol><p class=\"fineprint exact-authority\">{} · {}</p></details>",
            escape(&a.grade.authority.name),
            escape(&a.grade.authority.grader_version)
        );
        out.push_str(&self.result_progress(a));
        out.push_str("<div class=\"result-actions\">");
        out.push_str(&self.form(
            if self.app.practice.mixed_review {
                "/practice/review/next"
            } else {
                "/practice/next"
            },
            "exact-next",
            "exact-form",
            "data-next",
            &format!(
                    "{}{}<button class=\"button primary\">Next</button>",
                    hidden("after", &a.prompt.id),
                    if !self.app.practice.mixed_review
                        && crate::lessons::mapping_route(&self.app.library.versions[&a.version_id])
                            .is_some()
                    {
                        hidden("study_id", &a.prompt.study_id)
                    } else {
                        String::new()
                    }
                ),
        ));
        for e in &a.effects {
            if mapping {
                continue;
            }
            let corrected = self
                .app
                .practice
                .corrections
                .values()
                .find(|c| c.attempt_id == a.id && c.unit_id == e.unit_id);
            if let Some(c) = corrected {
                let _ = write!(
                    out,
                    "<p class=\"fineprint\">Unit {}: Owner Correction · {}. Original Verdict retained.</p>",
                    e.unit_index + 1,
                    if c.verdict == Verdict::Good {
                        "Good"
                    } else {
                        "Again"
                    }
                );
            }
        }
        out.push_str("</div>");
        out
    }
    fn exact_diff_word(d: &crate::grading::Diff, summary: bool, multiple_units: bool) -> String {
        use crate::grading::Change;
        let (kind, label, text) = match &d.change {
            Change::Correct { expected, .. } => ("correct", "Correct", escape(&expected.text)),
            Change::Slip { expected, typed } => (
                "slip",
                "Slipped",
                format!("{} → {}", escape(&typed.text), escape(&expected.text)),
            ),
            Change::Wrong {
                expected,
                typed,
                out_of_order,
            } => (
                "wrong",
                if *out_of_order {
                    "Wrong · out of order"
                } else {
                    "Wrong"
                },
                format!("{} → {}", escape(&typed.text), escape(&expected.text)),
            ),
            Change::Missing { expected } => ("missing", "Missing", escape(&expected.text)),
            Change::Extra { typed } => ("extra", "Extra", escape(&typed.text)),
        };
        let attribute = if summary {
            "data-feedback-kind"
        } else {
            "data-diff-kind"
        };
        let label_class = if kind == "correct" {
            "diff-label sr-only"
        } else {
            "diff-label"
        };
        let unit = if multiple_units {
            format!(
                "<span class=\"diff-unit{}\">Unit {}</span>",
                if kind == "correct" { " sr-only" } else { "" },
                d.unit_index + 1
            )
        } else {
            String::new()
        };
        format!(
            "<li {attribute}=\"{kind}\" data-unit-index=\"{}\">{unit}<span class=\"{label_class}\">{label}</span><span class=\"diff-text\">{text}</span></li>",
            d.unit_index
        )
    }
    fn study(&self) -> String {
        let Some(o) = self.app.occurrence.as_ref() else {
            return self.empty();
        };
        let concept = self.app.concepts.get(&o.concept_id);
        if o.phase == "intro" {
            let mut out = format!(
                "<section class=\"study intro\" data-state=\"intro\"><p class=\"eyebrow\"><span class=\"small-star\" aria-hidden=\"true\">✦</span> A new idea</p><h1>{}</h1>",
                concept
                    .map(|c| self.concept_name(c))
                    .unwrap_or_else(|| "A useful starting point".into())
            );
            if let Some(c) = concept {
                out.push_str(&self.lede("lede", c));
                out.push_str(&self.notes(c));
            }
            out.push_str("<div class=\"thumb-actions\">");
            out.push_str(&self.review_form("intro", "read", "", "", &format!("{}<button class=\"button primary\">Start practicing <span aria-hidden=\"true\">→</span></button>",hidden("known","false"))));
            out.push_str(&self.review_form(
                "intro",
                "known",
                "",
                "",
                &format!(
                    "{}<button class=\"text-button\">I know this already</button>",
                    hidden("known", "true")
                ),
            ));
            out.push_str("</div><p class=\"fineprint\">Reading is a starting point. Your practice tells us what to revisit.</p></section>");
            return format!("{}{out}", self.pending());
        }
        let mut out = String::from("<section class=\"study\"");
        let _ = write!(out, " data-state=\"{}\">", escape(&o.phase));
        if let Some(c) = concept {
            let (label, href) = self.study_chip(c);
            if o.phase == "result" || o.assisted {
                let _ = write!(
                    out,
                    "<a class=\"concept-chip\" href=\"{href}\"><span aria-hidden=\"true\">✦</span>{label}</a>"
                );
            } else {
                let _ = write!(
                    out,
                    "<span class=\"concept-chip\"><span aria-hidden=\"true\">✦</span>{label}</span>"
                );
            }
        }
        let _ = write!(
            out,
            "<h1 class=\"question\">{}</h1>",
            escape(&o.presentation.prompt)
        );
        match o.phase.as_str() {
            "result" => out.push_str(&self.result(o)),
            "self" | "self-check" => out.push_str(&self.self_check(o)),
            "checking" | "pending" => {
                if self.check_paused(o) {
                    out.push_str("<div class=\"checking\" role=\"status\"><p class=\"eyebrow\">Your answer is saved</p><h2>Your meaning check is paused.</h2><p>Resume meaning checks in Settings to continue. Your answer stays ungraded.</p></div>");
                } else {
                    out.push_str("<div class=\"checking\" role=\"status\"><p class=\"eyebrow\">Your answer is saved</p><h2>Checking your meaning.</h2><p>You can leave and come back. Your question will stay here.</p></div>");
                }
                if let Some(answer) = &o.answer {
                    let _ = write!(
                        out,
                        "<div class=\"answer-pair\"><span>You answered</span><p>{}</p></div>",
                        escape(answer)
                    );
                }
                if self.check_paused(o) {
                    out.push_str(self.resume_link());
                } else {
                    let _ = write!(
                        out,
                        "<a class=\"text-link\" href=\"{}\">Check again</a>",
                        legacy_home(self.app)
                    );
                }
            }
            _ => out.push_str(&self.question(o)),
        }
        out.push_str("</section>");
        format!("{}{out}", self.pending())
    }
    fn question(&self, o: &Occurrence) -> String {
        let mut out = String::new();
        if o.assisted {
            out.push_str(
                "<p class=\"helped-label\">Helped practice · the reference was opened</p>",
            );
        }
        if o.presentation.kind == "choice" || o.presentation.kind == "mcq" {
            let mut choices = String::from(
                "<fieldset class=\"choice-fieldset\"><legend class=\"sr-only\">Choose an answer</legend>",
            );
            for (index, choice) in o.presentation.choices.iter().enumerate() {
                let _ = write!(
                    choices,
                    "<button class=\"choice\" name=\"answer\" value=\"{}\" data-choice=\"{}\"><span class=\"choice-key\" aria-hidden=\"true\">{}</span><span>{}</span><span class=\"choice-arrow\" aria-hidden=\"true\">↗</span></button>",
                    escape(choice),
                    index + 1,
                    index + 1,
                    escape(choice)
                );
            }
            choices.push_str("</fieldset>");
            out.push_str(&self.review_form(
                "answer",
                "answer",
                "answer-form choice-form",
                "",
                &choices,
            ));
            out.push_str("<p class=\"control-hint\">Choose one answer. <span class=\"keyboard-hint\">Or press its number.</span></p>");
        } else {
            let field = format!(
                "<label class=\"answer-label\" for=\"answer\">{}<span>Take a moment. Say it your way.</span></label><textarea id=\"answer\" name=\"answer\" rows=\"3\" maxlength=\"4000\" placeholder=\"Your answer, from memory…\" required autocomplete=\"off\" spellcheck=\"false\">{}</textarea><div class=\"thumb-actions answer-actions\"><button class=\"button primary\">Check my answer <span aria-hidden=\"true\">→</span></button></div>",
                if o.presentation.kind == "explain" {
                    "Explain your thinking"
                } else {
                    "What comes to mind?"
                },
                escape(o.answer.as_deref().unwrap_or(""))
            );
            out.push_str(&self.review_form("answer", "answer", "answer-form", "", &field));
        }
        out.push_str(&self.more(o));
        out
    }
    fn answer_pair(&self, o: &Occurrence) -> String {
        format!(
            "<dl class=\"answer-pair\"><div><dt>You answered</dt><dd>{}</dd></div><div><dt>The answer</dt><dd>{}</dd></div></dl><div class=\"reading explanation\"><p>{}</p></div>{}",
            escape(
                o.answer
                    .as_deref()
                    .filter(|a| !a.is_empty())
                    .unwrap_or("You looked at the answer")
            ),
            escape(&o.presentation.answer),
            escape(&o.presentation.explanation),
            provenance(&o.presentation.basis, &o.presentation.quotes)
        )
    }
    fn result(&self, o: &Occurrence) -> String {
        let Some(result) = &o.result else {
            return "<p role=\"status\">Your result is being reconciled. Refresh to see the saved outcome.</p>".into();
        };
        let helped = o.assisted || result.outcome == "helped" || result.authority == "reveal";
        let correct = matches!(
            result.outcome.as_str(),
            "correct" | "success" | "recalled" | "warm_correct"
        );
        let (class, icon, title) = if helped {
            ("helped", "◉", "With a little help.")
        } else if correct {
            ("correct", "✓", "You recalled it.")
        } else {
            ("miss", "×", "Not quite yet.")
        };
        let authority = if result.corrected {
            "You corrected this grade"
        } else {
            match result.authority.as_str() {
                "exact" => "Matched the saved answer",
                "jev" => "Checked the meaning",
                "learner" => "You checked your answer",
                "reveal" => "You chose to see the answer",
                _ => "Saved learning record",
            }
        };
        let mut out = format!(
            "<div class=\"feedback feedback-{class}\" role=\"status\" aria-live=\"polite\" aria-atomic=\"true\"><span class=\"result-mark\" aria-hidden=\"true\">{icon}</span><div><h2>{title}</h2><p>{authority}{}</p></div></div>{}",
            if helped {
                " · recorded as helped practice"
            } else {
                ""
            },
            self.answer_pair(o)
        );
        out.push_str("<div class=\"thumb-actions result-actions\">");
        out.push_str(&self.review_form("next","next","","data-next", "<button class=\"button primary\">Next question <span aria-hidden=\"true\">→</span></button>"));
        out.push_str("</div>");
        if let Some(c) = self.app.concepts.get(&o.concept_id) {
            let _ = write!(
                out,
                "<a class=\"text-link reference-link\" href=\"/concepts/{}\">Read the explanation for this idea <span aria-hidden=\"true\">↗</span></a>",
                escape(&c.id)
            );
        }
        out.push_str(&self.more(o));
        out
    }
    fn self_check(&self, o: &Occurrence) -> String {
        let assessment = o
            .assessment_id
            .as_ref()
            .and_then(|id| self.app.assessments.get(id));
        let mut out = format!(
            "<div class=\"feedback feedback-self\" role=\"status\"><h2>Your answer is saved, not graded.</h2><p>A native verdict is unavailable for this earlier activity. Self-grading is retired; your answer, history and schedule are unchanged.</p></div>{}<p><a class=\"text-link\" href=\"/\">Return to native review</a></p>",
            self.answer_pair(o)
        );
        if self.check_paused(o) {
            out.push_str(self.resume_link());
        } else if assessment
            .is_some_and(|a| matches!(a.status.as_str(), "failed" | "unknown" | "superseded"))
        {
            out.push_str(&self.review_form(
                "retry",
                "retry",
                "",
                "",
                "<button class=\"text-button\">Retry native check</button>",
            ));
        }
        out
    }
    fn more(&self, o: &Occurrence) -> String {
        let mut out = String::from(
            "<details class=\"overflow\"><summary>More <span aria-hidden=\"true\">＋</span></summary><div class=\"overflow-menu\">",
        );
        if o.phase == "question" {
            out.push_str(&self.review_form(
                "reveal",
                "reveal",
                "",
                "",
                "<button>Show me the answer</button>",
            ));
            let _ = write!(
                out,
                "<a href=\"/gate?concept={}\">Look it up in my notes</a>",
                escape(&o.concept_id)
            );
        }
        let _ = write!(
            out,
            "<a href=\"/questions/{}/edit\">Edit this question</a>",
            escape(&o.question_id)
        );
        out.push_str(&self.form(&format!("/questions/{}/dispute",o.question_id),"dispute","","",&format!("{}<details class=\"dispute-fields\"><summary>Flag faulty material</summary><label for=\"dispute-text\">What needs fixing?</label><textarea id=\"dispute-text\" name=\"text\" rows=\"2\" maxlength=\"4000\" required></textarea><label class=\"checkbox-label\"><input type=\"checkbox\" name=\"reset\" value=\"true\"> Reset this question’s schedule</label><button>Save the concern</button></details>",format_args!("{}{}",hidden("content_version",&o.presentation.version.to_string()),hidden("schedule_version",&self.app.questions.get(&o.question_id).map(|q|q.schedule_version).unwrap_or(o.schedule_version).to_string())))));
        out.push_str(&self.form(
            &format!("/questions/{}/archive", o.question_id),
            "archive",
            "",
            "",
            "<button class=\"danger\">Archive this question</button>",
        ));
        out.push_str("</div></details>");
        out
    }
    fn empty_state(&self) -> EmptyState<'_> {
        let mut state = EmptyState::FirstRun;
        for goal in self.app.goals.values().filter(|g| !g.archived) {
            if goal.paused {
                if matches!(state, EmptyState::FirstRun) {
                    state = EmptyState::CaughtUp;
                }
                continue;
            }
            let phase = self.goal_phase(goal);
            match phase {
                "notebook-paused" => return EmptyState::Paused,
                "queued" | "candidates" | "sent" | "critic-sent" | "preparing" => {
                    return EmptyState::Preparing;
                }
                "ready" | "partial" | "imported-unfinished" => {
                    if matches!(state, EmptyState::FirstRun) {
                        state = EmptyState::CaughtUp;
                    }
                }
                _ => {
                    if !matches!(state, EmptyState::Stopped(previous) if previous.created_ms >= goal.created_ms)
                    {
                        state = EmptyState::Stopped(goal);
                    }
                }
            }
        }
        state
    }
    fn empty(&self) -> String {
        let state = self.empty_state();
        let any = !matches!(state, EmptyState::FirstRun);
        let due = self
            .app
            .questions
            .values()
            .filter(|q| !q.archived)
            .map(|q| q.card.due_ms)
            .filter(|d| *d > self.now)
            .min();
        let demo = if !any && self.query.get("__synthetic").is_some_and(|v| v == "true") {
            self.form(
                "/__fixture",
                "fixture",
                "demo-form",
                "",
                "<button class=\"text-button\">Explore an authored demo</button>",
            )
        } else {
            String::new()
        };
        let (state_name, title, desc) = match state {
            EmptyState::FirstRun => ("empty","Follow your curiosity.","Choose externally authored material from Library. Scry helps you remember it with native answer grading.".to_string()),
            EmptyState::Preparing => ("preparing","Your saved request is preserved.","Native content generation is retired. Your request and any existing material remain available.".to_string()),
            EmptyState::Paused => ("preparation-paused","Preparation is paused.","Your requests are saved. Native content generation is retired. Meaning checks can resume separately in Settings. Your existing notes and practice are still available.".to_string()),
            EmptyState::Stopped(_) => ("preparation-stopped","Your request needs attention.","Your input is saved, but preparation is not running. Open the request to see what stopped it and choose what to do next.".to_string()),
            EmptyState::CaughtUp => ("caught-up","A good place to pause.",due.map(|d|format!("You're caught up for now. Your next practice is {}.",relative(d,self.now))).unwrap_or_else(||"You have no eligible questions right now. Read your saved notes or choose material from Library.".into())),
        };
        let (destination, action) = match state {
            EmptyState::Paused => ("/settings#preparation".into(), "Resume meaning checks"),
            EmptyState::Stopped(goal) => (
                format!("/goals/{}", escape(&goal.id)),
                "Review your saved request",
            ),
            _ => ("/library".into(), "Browse Library"),
        };
        format!(
            "{demo}{}<section class=\"empty-stage\" data-state=\"{state_name}\">{}<p class=\"eyebrow\">A private space for understanding</p><h1>{title}</h1><p class=\"lede\">{desc}</p><div class=\"actions\"><a class=\"button primary\" href=\"{destination}\">{action} <span aria-hidden=\"true\">→</span></a>{}</div><div class=\"notebook-margin\"><span>01 / Curiosity</span><span>Understanding starts with a question.</span></div></section>",
            self.pending(),
            emblem("large"),
            if any {
                "<a class=\"text-link\" href=\"/map\">Explore your map ↗</a>"
            } else {
                ""
            }
        )
    }
    fn nato_reference(&self) -> String {
        // A reference must never disclose an answer behind a protected prompt.
        if self.app.practice.protected()
            || self.app.protected_goal().is_some()
            || self.app.occurrence.is_some()
        {
            return String::new();
        }
        let input = crate::library::nato_input().expect("bundled NATO content is valid");
        let mut out = String::from(
            "<details name=\"practice-tools\" class=\"nato-reference\"><summary>Alphabet reference</summary><dl>",
        );
        for unit in input
            .sections
            .unwrap_or_default()
            .iter()
            .flat_map(|s| &s.units)
        {
            if let Some((key, word)) = crate::library::mapping_pair(unit) {
                let _ = write!(
                    out,
                    "<div><dt>{}</dt><dd>{}</dd></div>",
                    escape(key),
                    escape(word)
                );
            }
        }
        out.push_str("</dl><p>Official NATO spellings. <a href=\"https://www.nato.int/cps/en/natohq/declassified_136216.htm\" rel=\"noreferrer\">Source</a></p></details>");
        out
    }
    fn lesson_picker(&self, open: bool) -> String {
        let current = self.app.practice.current.as_ref();
        let held = current.is_some_and(|p| p.attempt_id.is_none());
        let current_version =
            current.and_then(|p| self.app.library.versions.get(&p.prompt.version_id));
        let after = current.map_or(String::new(), |p| {
            hidden(
                if p.attempt_id.is_some() {
                    "after"
                } else {
                    "leave"
                },
                &p.prompt.id,
            )
        });
        let catalog = crate::lessons::catalog();
        let current_starter = current_version.and_then(|v| {
            if crate::library::is_nato(v) {
                Some("nato")
            } else {
                catalog.iter().find(|l| l.matches(v)).map(|l| l.id.as_str())
            }
        });
        let mut out = format!(
            "<details name=\"practice-tools\" class=\"lesson-picker\" {}><summary><span>Lessons</span><span class=\"picker-toggle\" aria-hidden=\"true\"><span class=\"picker-chevron\">⌄</span></span></summary><div class=\"lesson-shelf\">",
            if open { "open" } else { "" }
        );
        if current.is_some() {
            let state = match current_starter {
                Some("nato") => "NATO alphabet",
                Some("baby-soothing") => "Baby care · Safe soothing",
                Some("baby-everyday") => "Baby care · Everyday safety",
                Some("baby-montessori") => "Baby care · Montessori",
                Some("st-michael-en") => "Saint Michael · English",
                Some("st-michael-la-traditional") => "Saint Michael · Latin",
                Some("st-michael-en-lasance-1911") => "Saint Michael · Historical English",
                Some("rosary-joyful") => "Joyful mysteries",
                Some("rosary-luminous") => "Luminous mysteries",
                Some("rosary-sorrowful") => "Sorrowful mysteries",
                Some("rosary-glorious") => "Glorious mysteries",
                _ => "Current practice",
            };
            out.push_str(&format!("<div class=\"lesson-current\"><p><span class=\"picker-eyebrow\">Current</span><strong>{state}</strong></p>"));
            if self.path == "/lessons" {
                out.push_str("<a class=\"text-link\" href=\"/recall\">Back to practice <span aria-hidden=\"true\">↑</span></a>");
            } else {
                // The enhanced close button never navigates or replaces the draft.
                // Without JS the native summary remains the close control.
                out.push_str("<button type=\"button\" class=\"text-button picker-return\" data-picker-close hidden>Back to practice <span aria-hidden=\"true\">↑</span></button>");
            }
            out.push_str("</div>");
            if held {
                out.push_str(
                    "<p class=\"lesson-switch-note\">Switching leaves this question ungraded.</p>",
                );
            }
        }
        if current.is_some() && !self.app.practice.mixed_review {
            out.push_str(&self.form(
                "/practice/review/start",
                "review-all",
                "",
                "",
                &format!(
                    "{}<button class=\"text-button\">Review all subjects</button>",
                    if current_version.is_some_and(|v| crate::study::reviewable(v).is_err()) {
                        after.as_str()
                    } else {
                        ""
                    }
                ),
            ));
        }
        out.push_str("<div class=\"lesson-groups\">");
        let groups = [
            (
                "01",
                "NATO alphabet",
                "26 letters",
                vec![("nato", "Practise", "")],
            ),
            (
                "02",
                "Saint Michael",
                "Phrase by phrase",
                vec![
                    ("st-michael-en", "English", "8 phrases · supplied wording"),
                    (
                        "st-michael-la-traditional",
                        "Latin",
                        "Traditional · unaccented",
                    ),
                    (
                        "st-michael-en-lasance-1911",
                        "Historical English",
                        "Lasance · 1911",
                    ),
                ],
            ),
            (
                "03",
                "Rosary mysteries",
                "Five in order",
                vec![
                    ("rosary-joyful", "Joyful", ""),
                    ("rosary-luminous", "Luminous", ""),
                    ("rosary-sorrowful", "Sorrowful", ""),
                    ("rosary-glorious", "Glorious", ""),
                ],
            ),
            (
                "04",
                "Baby care",
                "Newborn readiness",
                vec![
                    ("baby-soothing", "Safe soothing", "Karp’s five S’s"),
                    ("baby-everyday", "Everyday safety", "Newborn essentials"),
                    ("baby-montessori", "Montessori", "Awake interaction"),
                ],
            ),
        ];
        for (number, title, description, options) in groups {
            let glyph = match number {
                "01" => {
                    r#"<svg viewBox="0 0 80 80" aria-hidden="true"><path d="M22 60 40 18l18 42M29 44h22"/><path class="glyph-accent" d="M9 27v26m62-26v26M15 34v12m50-12v12"/></svg>"#
                }
                "02" => {
                    r#"<svg viewBox="0 0 80 80" aria-hidden="true"><path d="M40 12 61 21v21c0 12-13 22-21 26-8-4-21-14-21-26V21Z"/><path class="glyph-accent" d="M40 21v36m-9-25h18m-15 6 6-6 6 6"/></svg>"#
                }
                "04" => {
                    r#"<svg viewBox="0 0 80 80" aria-hidden="true"><circle cx="40" cy="26" r="12"/><path d="M22 42q18-12 36 0l-8 24H30Z"/><path class="glyph-accent" d="m25 44 28 16M55 44 28 57"/></svg>"#
                }
                _ => {
                    r#"<svg viewBox="0 0 80 80" aria-hidden="true"><g><circle cx="40" cy="12" r="3"/><circle cx="25" cy="16" r="3"/><circle cx="16" cy="29" r="3"/><circle cx="19" cy="44" r="3"/><circle cx="31" cy="53" r="3"/><circle cx="49" cy="53" r="3"/><circle cx="61" cy="44" r="3"/><circle cx="64" cy="29" r="3"/><circle cx="55" cy="16" r="3"/></g><path class="glyph-accent" d="M40 53v19m-6-7h12"/></svg>"#
                }
            };
            let _ = write!(
                out,
                "<section class=\"lesson-group\" aria-labelledby=\"lesson-group-{number}\"><header><div class=\"lesson-glyph\">{glyph}</div><h2 id=\"lesson-group-{number}\">{title}</h2><p>{description}</p></header><div class=\"lesson-choices\">"
            );
            for (id, label, detail) in options {
                let active = current_starter == Some(id);
                let detail = if detail.is_empty() {
                    String::new()
                } else {
                    format!("<small>{detail}</small>")
                };
                let copy = format!(
                    "<span class=\"lesson-choice-copy\"><strong>{label}</strong>{detail}</span>"
                );
                if active {
                    let _ = write!(
                        out,
                        "<div class=\"lesson-choice is-current\" data-current-lesson=\"{}\">{copy}<span class=\"lesson-choice-state\">Current</span></div>",
                        escape(id)
                    );
                } else {
                    let verb = if held { "Switch to" } else { "Practise" };
                    out.push_str(&self.form(
                        "/practice/lesson/start", &format!("lesson-{id}"), "lesson-start", "",
                        &format!("{}{}<button class=\"lesson-choice\" aria-label=\"{} {}: {}\">{copy}<span class=\"lesson-choice-arrow\" aria-hidden=\"true\">→</span></button>", hidden("starter", id), after, verb, title, label),
                    ));
                }
            }
            out.push_str("</div></section>");
        }
        out.push_str("</div>");
        // Supplied metadata can contain answers. Never render it during
        // protected recall, even in a closed disclosure. Match complete content,
        // not names, so custom versions with the same title remain reachable.
        if !self.app.practice.protected() {
            let supplied = self
                .app
                .library
                .versions
                .values()
                .filter(|v| {
                    v.kind != crate::library::Kind::KeyedMapping
                        && !catalog.iter().any(|l| l.matches(v))
                })
                .collect::<Vec<_>>();
            if !supplied.is_empty() {
                out.push_str("<section class=\"lesson-supplied\" aria-labelledby=\"lesson-supplied-title\"><h2 id=\"lesson-supplied-title\">Your other texts</h2><div class=\"lesson-supplied-list\">");
                for version in supplied {
                    let work = &self.app.library.works[&version.work_id];
                    let copy = format!(
                        "<span class=\"lesson-choice-copy\"><strong>{}</strong><small>{}</small></span>",
                        escape(&work.title),
                        escape(&version.name)
                    );
                    if crate::study::reviewable(version).is_err() {
                        let _ = write!(
                            out,
                            "<a class=\"lesson-choice\" href=\"/library/versions/{}\">{copy}<span class=\"lesson-choice-state\">Practice unavailable</span></a>",
                            escape(&version.id)
                        );
                    } else if current_version.is_some_and(|v| v.id == version.id) {
                        let _ = write!(
                            out,
                            "<div class=\"lesson-choice is-current\">{copy}<span class=\"lesson-choice-state\">Current</span></div>"
                        );
                    } else {
                        out.push_str(&self.form(
                            "/practice/lesson/start", &format!("lesson-version-{}", version.id), "lesson-start", "",
                            &format!("{}{}<button class=\"lesson-choice\" aria-label=\"Practise {}: {}\">{copy}<span class=\"lesson-choice-arrow\" aria-hidden=\"true\">→</span></button>", hidden("version_id", &version.id), after, escape(&work.title), escape(&version.name)),
                        ));
                    }
                }
                out.push_str("</div></section>");
            }
        }
        out.push_str("</div></details>");
        out
    }
    fn lesson_progress(&self, version: &crate::library::Version) -> String {
        let attempts = self
            .app
            .practice
            .attempts
            .values()
            .filter(|a| a.version_id == version.id && !a.disclosure)
            .collect::<Vec<_>>();
        let whole = attempts
            .iter()
            .filter(|a| matches!(a.prompt.scope, crate::study::Scope::Whole) && !a.assisted)
            .collect::<Vec<_>>();
        let correct = whole
            .iter()
            .filter(|a| a.grade.verdict == crate::grading::Verdict::Good)
            .count();
        let phrases = attempts
            .iter()
            .filter(|a| matches!(a.prompt.scope, crate::study::Scope::Unit { .. }) && !a.assisted)
            .count();
        let helped = attempts.iter().filter(|a| a.assisted).count();
        format!(
            "<details name=\"practice-tools\" class=\"lesson-evidence\"><summary>Progress <span>{} answer{}</span></summary><dl><div><dt>Unaided whole attempts</dt><dd>{correct} exact / {}</dd></div><div><dt>Unaided phrase answers</dt><dd>{phrases}</dd></div><div><dt>With help</dt><dd>{helped}</dd></div></dl><p>Whole recall is measured separately. Practising phrases or seeing hints is not proof of whole recall.</p></details>",
            attempts.len(),
            if attempts.len() == 1 { "" } else { "s" },
            whole.len()
        )
    }
    fn lesson_screen(&self) -> String {
        use crate::{
            grading::Verdict,
            library::Kind,
            study::{Cue, Scope},
        };
        let Some(p) = &self.app.practice.current else {
            return format!(
                "<section class=\"nato-screen lesson-screen\"><h1>Practice</h1>{}</section>",
                self.lesson_picker(true)
            );
        };
        let v = &self.app.library.versions[&p.prompt.version_id];
        if crate::study::reviewable(v).is_err() {
            return self.unavailable_practice(p);
        }
        let work = &self.app.library.works[&v.work_id];
        let all = v.sections.iter().flat_map(|s| &s.units).collect::<Vec<_>>();
        let ordered = v.kind == Kind::OrderedList;
        let whole = matches!(p.prompt.scope, Scope::Whole);
        let phrase = !ordered && matches!(p.prompt.scope, Scope::Unit { .. });
        let a = p
            .attempt_id
            .as_ref()
            .and_then(|id| self.app.practice.attempts.get(id));
        let scope = if ordered {
            format!("{} items · in order", all.len())
        } else if whole {
            "Full text".into()
        } else if matches!(p.prompt.scope, Scope::Section { .. }) {
            format!(
                "Joined phrases {}–{}",
                p.targets[0].unit_index + 1,
                p.targets.last().unwrap().unit_index + 1
            )
        } else {
            format!("Phrase {} / {}", p.targets[0].unit_index + 1, all.len())
        };
        let mut extras = String::new();
        let mut out = format!(
            "<section class=\"nato-screen lesson-screen\" data-state=\"lesson-home\"><header class=\"practice-heading\"><div><h1>{}</h1><p class=\"practice-edition\">{}</p></div><p class=\"lesson-position\">{}</p></header><div class=\"lesson-stage\" data-state=\"{}\" data-prompt-id=\"{}\">",
            escape(&work.title),
            escape(&v.name),
            escape(&scope),
            if a.is_some() {
                "exact-result"
            } else {
                "exact-prompt"
            },
            escape(&p.prompt.id)
        );
        if self.app.practice.mixed_review {
            out.push_str("<p class=\"eyebrow\">All subjects</p>");
        }
        if let Some(a) = a {
            let good = a.grade.verdict == Verdict::Good;
            let label = if !good {
                "Not quite"
            } else if a.assisted {
                "That matches · with help"
            } else if whole {
                "Correct · full text"
            } else {
                "Correct"
            };
            let _ = write!(
                out,
                "<div class=\"lesson-verdict nato-{}\" role=\"status\"><p data-verdict=\"{}\">{label}</p></div>",
                if good { "correct" } else { "miss" },
                if good { "Good" } else { "Again" }
            );
            if ordered {
                if let Ok(feedback) = crate::ordered::feedback(v, 0..all.len(), &a.raw_answer) {
                    let _ = write!(
                        out,
                        "<p class=\"lesson-note\">{} of {} items recalled. {} in order.</p><ol class=\"sequence-feedback\">",
                        feedback.recalled,
                        all.len(),
                        feedback.in_order
                    );
                    for item in &feedback.items {
                        use crate::ordered::ItemStatus;
                        let label = match item.status {
                            ItemStatus::Correct => "In order",
                            ItemStatus::Misplaced => "Out of order",
                            ItemStatus::Missing => "Missing",
                            ItemStatus::Repeated => "Repeated",
                        };
                        let _ = write!(
                            out,
                            "<li><span>{}</span><small>{label}</small></li>",
                            escape(all[item.unit_index].text.trim())
                        );
                    }
                    out.push_str("</ol>");
                    if !feedback.extras.is_empty() {
                        out.push_str("<p class=\"lesson-note\">Your answer also contained an unrecognized or repeated entry.</p>");
                    }
                }
            } else if !good {
                let _ = write!(
                    out,
                    "<div class=\"lesson-target\"><h2>{}</h2>{}</div><details class=\"lesson-answer\"><summary>Your answer &amp; word comparison</summary><p class=\"reading\">{}</p><ol class=\"exact-diff exact-changes\" aria-label=\"Words to revisit\">",
                    if phrase {
                        "The phrase to practise"
                    } else {
                        "The selected wording"
                    },
                    self.exact_expected(),
                    escape(&a.raw_answer)
                );
                for d in a
                    .grade
                    .diff
                    .iter()
                    .filter(|d| !matches!(d.change, crate::grading::Change::Correct { .. }))
                {
                    out.push_str(&Self::exact_diff_word(d, true, a.grade.units.len() > 1));
                }
                out.push_str("</ol></details>");
            }
            if a.assisted {
                out.push_str("<p class=\"lesson-note\">With help · schedule unchanged.</p>");
            }
            out.push_str(&self.result_progress(a));
            if self.app.practice.mixed_review {
                out.push_str(&self.form(
                    "/practice/review/next",
                    "mixed-next",
                    "lesson-action",
                    "data-next",
                    &format!(
                        "{}<button class=\"button primary\">Next</button>",
                        hidden("after", &p.prompt.id)
                    ),
                ));
            } else {
                if phrase && !good {
                    out.push_str(&self.form(
                        "/practice/lesson/next",
                        "lesson-retry",
                        "lesson-action",
                        "data-next",
                        &format!(
                            "{}{}<button class=\"button primary\">Try this phrase again</button>",
                            hidden("after", &p.prompt.id),
                            hidden("mode", "retry")
                        ),
                    ));
                }
                out.push_str(&self.form(
                    "/practice/lesson/next",
                    "lesson-next",
                    "lesson-action",
                    if phrase && !good { "" } else { "data-next" },
                    &format!(
                        "{}{}<button class=\"{}\">{}</button>",
                        hidden("after", &p.prompt.id),
                        hidden("mode", if ordered { "whole" } else { "phrases" }),
                        if phrase && !good {
                            "text-button"
                        } else {
                            "button primary"
                        },
                        if ordered {
                            "Try the sequence again"
                        } else if whole {
                            "Practise phrases"
                        } else if phrase && !good {
                            "Continue to next phrase"
                        } else {
                            "Next phrase"
                        }
                    ),
                ));
            }
            if !ordered && !self.app.practice.mixed_review {
                extras.push_str("<div class=\"lesson-challenges\"><h2>Challenges</h2>");
            }
            if !ordered && !whole && !self.app.practice.mixed_review {
                extras.push_str(&self.form(
                    "/practice/lesson/next",
                    "lesson-join",
                    "lesson-action",
                    "",
                    &format!(
                        "{}{}<button class=\"button secondary\">Join these phrases</button>",
                        hidden("after", &p.prompt.id),
                        hidden("mode", "join")
                    ),
                ));
            }
            if !ordered && whole && !self.app.practice.mixed_review {
                extras.push_str(&self.form(
                    "/practice/lesson/next",
                    "lesson-middle",
                    "lesson-action",
                    "",
                    &format!(
                        "{}{}<button class=\"button secondary\">Start midway</button>",
                        hidden("after", &p.prompt.id),
                        hidden("mode", "middle")
                    ),
                ));
            }
            if !ordered && !self.app.practice.mixed_review {
                extras.push_str(&self.form(
                    "/practice/lesson/next",
                    "lesson-whole",
                    "lesson-action",
                    "",
                    &format!(
                        "{}{}<button class=\"button secondary\">Try the full text</button>",
                        hidden("after", &p.prompt.id),
                        hidden("mode", "whole")
                    ),
                ));
            }
            if !ordered && !self.app.practice.mixed_review {
                extras.push_str("</div>");
            }
            if ordered || good {
                let _ = write!(
                    out,
                    "<details class=\"lesson-answer\"><summary>Compare the text</summary><h2>Your answer</h2><p class=\"reading\">{}</p><h2>Selected wording</h2>{}</details>",
                    escape(&a.raw_answer),
                    self.exact_expected()
                );
            }
        } else if p.source_visible() {
            out.push_str(&self.source_study(p));
        } else {
            out.push_str("<p class=\"eyebrow\" data-study-phase=\"quiz\">Review</p>");
            if ordered {
                let _ = write!(
                    out,
                    "<p class=\"lesson-instruction\">Recall all {} items in order, one per line.</p>",
                    all.len()
                );
            } else if whole {
                out.push_str("<p class=\"lesson-instruction\">Type the full text from memory.</p>");
            } else if matches!(p.prompt.scope, Scope::Section { .. }) {
                out.push_str(
                    "<p class=\"lesson-instruction\">Join these phrases from memory, in order.</p>",
                );
            } else {
                if let Some(context) = &p.prompt.context {
                    let _ = write!(
                        out,
                        "<details class=\"lesson-context\"><summary>Previous phrase</summary><p>{}</p></details>",
                        escape(context)
                    );
                }
                let cue_label = match p.prompt.cue_level {
                    Cue::Read => "Study",
                    Cue::FirstLetters => "Type this phrase using the initials",
                    Cue::FirstLetter => "Type this phrase using the first initial",
                    _ => "Type this phrase from memory",
                };
                let _ = write!(out, "<p class=\"lesson-instruction\">{cue_label}</p>");
                if !p.prompt.cue.is_empty() && p.prompt.cue_level != Cue::WholeTitle {
                    let _ = write!(out, "<p class=\"lesson-cue\">{}</p>", escape(&p.prompt.cue));
                }
            }
            if p.revealed && p.phase != Some(crate::study::StudyPhase::Quiz) {
                out.push_str("<div class=\"lesson-disclosure\">");
                out.push_str(&self.exact_expected());
                out.push_str("</div>");
            }
            if p.prompt.assisted || p.revealed {
                out.push_str("<p class=\"lesson-note\">Guided · with help</p>");
            }
            out.push_str(&self.form("/practice/answer","lesson-answer","answer-form lesson-answer-form","data-answer-limit=\"32768\"",&format!("{}<label class=\"visually-hidden\" for=\"answer\">{}</label><textarea id=\"answer\" name=\"answer\" rows=\"{}\" maxlength=\"32768\" spellcheck=\"false\" autocapitalize=\"sentences\" autocomplete=\"off\" {} autofocus>\n{}</textarea><button class=\"button primary\">Check answer</button>",hidden("prompt_id",&p.prompt.id),if ordered{"Items in order"}else{"Your typed answer"},if ordered||whole||matches!(p.prompt.scope,Scope::Section{..}){5}else{3},if ordered||whole||matches!(p.prompt.scope,Scope::Section{..}){"data-multiline"}else{""},escape(self.saved_answer(&p.prompt.id)) )));
            if (!p.revealed || p.phase == Some(crate::study::StudyPhase::Quiz))
                && !matches!(p.prompt.cue_level, Cue::Read)
            {
                out.push_str(&self.practice_form(
                    "reveal",
                    "lesson-reveal",
                    "",
                    "<button class=\"text-button\">Show the text</button>",
                ));
            }
            if ordered {
                out.push_str("<p class=\"lesson-note\">One item per line.</p>");
            }
        }
        out.push_str("</div><div class=\"practice-tools\">");
        out.push_str(&self.lesson_picker(false));
        out.push_str(&self.lesson_progress(v));
        if !self.app.practice.protected() {
            let _ = write!(
                out,
                "<details name=\"practice-tools\" class=\"lesson-edition\"><summary>More</summary><p><strong>{}</strong></p><p>Typed wording is checked locally. Case and punctuation do not matter; words and order do. Accents remain significant; the supplied Latin edition uses unaccented spelling. No pronunciation assessment.</p><ul>",
                escape(&v.name)
            );
            for source in &v.sources {
                let absolute_http = source
                    .strip_prefix("https://")
                    .or_else(|| source.strip_prefix("http://"))
                    .is_some_and(|rest| {
                        let host = rest.split(['/', '?', '#']).next().unwrap_or("");
                        !host.is_empty()
                            && !source
                                .chars()
                                .any(|c| c.is_whitespace() || c.is_control() || c == '\\')
                    });
                if absolute_http {
                    let _ = write!(
                        out,
                        "<li><a href=\"{}\" rel=\"noreferrer\">Source</a></li>",
                        escape(source)
                    );
                } else {
                    let _ = write!(out, "<li>{}</li>", escape(source));
                }
            }
            out.push_str("</ul>");
            out.push_str(&extras);
            out.push_str("</details>");
        }
        out.push_str("</div></section>");
        out
    }
    fn result_progress(&self, a: &crate::study::Attempt) -> String {
        use crate::result_progress::{Mark, summarize};
        let evidence = summarize(self.app, a);
        let scope = match a.prompt.scope {
            crate::study::Scope::Whole
                if self
                    .app
                    .library
                    .versions
                    .get(&a.version_id)
                    .is_some_and(|v| v.kind == crate::library::Kind::OrderedList) =>
            {
                "This sequence"
            }
            crate::study::Scope::Whole => "Full text",
            crate::study::Scope::Section { .. } => "This passage",
            crate::study::Scope::Unit { .. }
                if a.prompt.cue_level == crate::study::Cue::MappingKey =>
            {
                if self
                    .app
                    .library
                    .versions
                    .get(&a.version_id)
                    .is_some_and(crate::library::is_nato)
                {
                    "This letter"
                } else {
                    "This item"
                }
            }
            _ => "This phrase",
        };
        let recall = if evidence.unaided == 0 {
            "With help".to_string()
        } else {
            format!("{} / {}", evidence.correct, evidence.unaided)
        };
        let recall_label = if evidence.unaided == 0 {
            "Unaided recall not tested"
        } else {
            "Unaided correct"
        };
        let multi = a.effects.len() > 1;
        let (due, exact) = if !evidence.active {
            ("Not active".to_string(), "Study dropped".to_string())
        } else if let Some(ms) = evidence.next_due_ms {
            (
                if ms <= self.now {
                    "Now".to_string()
                } else {
                    relative(ms, self.now)
                },
                date(ms),
            )
        } else {
            (
                "Not scheduled".to_string(),
                "First unaided review needed".to_string(),
            )
        };
        let due_label = if multi {
            "Next part review"
        } else {
            "Next review"
        };
        let mut out = format!(
            "<section class=\"result-progress\" aria-label=\"{} recall and review\" data-result-progress><div><span class=\"result-progress-label\">{scope}</span><strong data-result-metric=\"recall\">{recall}</strong><span class=\"result-progress-label\">{recall_label}</span><ol class=\"result-marks\" aria-label=\"Recent answers, oldest first\">",
            scope
        );
        for mark in evidence.recent {
            let (tone, glyph, label) = match mark {
                Mark::Correct => ("correct", "✓", "Correct without help"),
                Mark::Miss => ("miss", "×", "Miss without help"),
                Mark::Helped => ("helped", "+", "With help"),
            };
            let _ = write!(
                out,
                "<li class=\"evidence-{tone}\"><span aria-hidden=\"true\">{glyph}</span><span class=\"visually-hidden\">{label}</span></li>"
            );
        }
        let _ = write!(
            out,
            "</ol></div><div><span class=\"result-progress-label\">{due_label}</span><strong data-result-metric=\"due\">{}</strong><span class=\"result-progress-date\">{}</span>",
            escape(&due),
            escape(&exact)
        );
        if multi && evidence.unscheduled_parts > 0 && evidence.next_due_ms.is_some() {
            out.push_str(
                "<span class=\"result-progress-label\">Some parts not scheduled yet</span>",
            );
        }
        out.push_str("</div></section>");
        out
    }
    fn nato_progress(&self) -> String {
        use crate::nato_progress::{Outcome, summarize};
        let progress = summarize(self.app);
        let current = self
            .app
            .practice
            .current
            .as_ref()
            .filter(|p| {
                self.app
                    .library
                    .versions
                    .get(&p.prompt.version_id)
                    .is_some_and(crate::library::is_nato)
            })
            .and_then(|p| {
                p.prompt
                    .cue
                    .chars()
                    .next()
                    .map(|key| (key, p.attempt_id.is_some()))
            });
        let selected = current.map_or('A', |(key, _)| key);
        let reviews_label = format!(
            "{} review{}",
            progress.reviews,
            if progress.reviews == 1 { "" } else { "s" }
        );
        let mut out = format!(
            "<details name=\"practice-tools\" class=\"nato-progress\" id=\"nato-progress\"><summary><span>Progress</span><span class=\"progress-summary\">{} <span aria-hidden=\"true\">↗</span></span></summary><div class=\"progress-body\"><fieldset class=\"letter-map\"><legend class=\"visually-hidden\">Letter progress</legend>",
            reviews_label
        );
        for letter in &progress.letters {
            let key = letter.letter;
            let position = match current {
                Some((held, false)) if held == key => "On screen now",
                Some((held, true)) if held == key => "Answer saved",
                _ => "",
            };
            let accuracy = letter
                .accuracy_percent()
                .map_or_else(|| "—".into(), |value| format!("{value}%"));
            let tone = match letter.recent.last().map(|r| &r.outcome) {
                Some(Outcome::Correct) => "correct",
                Some(Outcome::Miss) => "miss",
                Some(Outcome::Assisted) => "helped",
                None => "new",
            };
            let due = letter.due_ms.map_or_else(
                || "Not scheduled".into(),
                |ms| {
                    if ms <= self.now {
                        "Due now".into()
                    } else {
                        relative(ms, self.now)
                    }
                },
            );
            let due_detail = letter
                .due_ms
                .map_or_else(|| "No active review schedule".into(), date);
            let last = letter.last_answered_ms.map_or_else(
                || "No reviews yet".into(),
                |ms| {
                    let elapsed = self.now.saturating_sub(ms).max(0);
                    let when = if elapsed < 60_000 {
                        "just now".into()
                    } else if elapsed < 3_600_000 {
                        format!("{}m ago", elapsed / 60_000)
                    } else if elapsed < DAY_MS {
                        format!("{}h ago", elapsed / 3_600_000)
                    } else {
                        format!("{}d ago", elapsed / DAY_MS)
                    };
                    format!("Last reviewed {when}")
                },
            );
            let _ = write!(
                out,
                "<input class=\"progress-radio visually-hidden\" type=\"radio\" name=\"progress-letter\" id=\"progress-{key}\" value=\"{key}\" aria-label=\"Letter {key} progress\" {}><label class=\"letter-key evidence-{tone}\" for=\"progress-{key}\"><span>{key}</span><i aria-hidden=\"true\"></i></label><section class=\"letter-progress\" id=\"letter-progress-{key}\" aria-label=\"Letter {key} statistics\"><div class=\"progress-heading\"><h2>Letter {key}</h2><span data-metric=\"position\">{position}</span></div><dl class=\"progress-metrics\"><div><dt>Reviews</dt><dd data-metric=\"reviews\">{}</dd><small>{} with help</small></div><div><dt>Unaided correct</dt><dd data-metric=\"accuracy\">{accuracy}</dd><small>{} of {} answers</small></div><div><dt>Due for review</dt><dd class=\"due-value\" data-metric=\"due\">{due}</dd><small>{}</small></div></dl><div class=\"progress-history\"><span>Recent reviews</span><ol aria-label=\"Recent reviews, oldest first\">",
                if key == selected { "checked" } else { "" },
                letter.reviews,
                letter.assisted,
                letter.correct,
                letter.unassisted,
                escape(&due_detail)
            );
            for mark in &letter.recent {
                let (tone, label, glyph) = match mark.outcome {
                    Outcome::Correct => ("correct", "Correct", "✓"),
                    Outcome::Miss => ("miss", "Miss", "×"),
                    Outcome::Assisted => ("helped", "With help", "+"),
                };
                let _ = write!(
                    out,
                    "<li class=\"review-mark evidence-{tone}\" title=\"{}\"><span aria-hidden=\"true\">{glyph}</span><span class=\"visually-hidden\">{label}, {}</span></li>",
                    escape(&format!("{label} · {}", date(mark.created_ms))),
                    escape(&date(mark.created_ms))
                );
            }
            if letter.recent.is_empty() {
                out.push_str("<li class=\"no-reviews\">Nothing yet</li>");
            }
            let _ = write!(
                out,
                "</ol></div><p class=\"progress-last\">{}</p></section>",
                escape(&last)
            );
        }
        out.push_str("</fieldset><div class=\"progress-legend\"><span class=\"evidence-correct\"><b aria-hidden=\"true\">✓</b>Correct</span><span class=\"evidence-miss\"><b aria-hidden=\"true\">×</b>Miss</span><span class=\"evidence-helped\"><b aria-hidden=\"true\">+</b>With help</span></div><details class=\"progress-method\"><summary>About these numbers</summary><p>Accuracy uses unaided answers, including extra practice. Reveals aren’t reviews. Practice selects due reviews and new letters.</p></details></div></details>");
        out
    }
    fn mixed_home(&self) -> String {
        let preview = crate::study::continuous_preview(self.app, self.now);
        let mut out = String::from(
            "<section class=\"nato-screen study-home\" data-state=\"mixed-home\"><div class=\"study-entry\">",
        );
        if let Some(subject) = preview {
            let _ = write!(
                out,
                "<p class=\"study-context\">All subjects</p><h1>{}</h1>",
                escape(&subject)
            );
            out.push_str(&self.form(
                "/practice/review/start",
                "mixed-start",
                "",
                "",
                "<button class=\"button primary\"><span>Study</span><span aria-hidden=\"true\">→</span></button>",
            ));
        } else {
            out.push_str("<p class=\"study-context\">All subjects</p><h1>Rest for now</h1>");
        }
        out.push_str("</div>");
        out.push_str(&self.lesson_picker(false).replacen(
            "<summary><span>Lessons</span>",
            "<summary><span>Browse</span>",
            1,
        ));
        out.push_str("</section>");
        out
    }
    fn nato_screen(&self) -> String {
        let nato_versions = self
            .app
            .library
            .versions
            .values()
            .filter(|v| crate::library::is_nato(v))
            .map(|v| v.id.as_str())
            .collect::<std::collections::BTreeSet<_>>();
        let practiced = self
            .app
            .practice
            .attempts
            .values()
            .filter(|a| {
                !a.assisted && !a.disclosure && nato_versions.contains(a.version_id.as_str())
            })
            .map(|a| a.prompt.cue.as_str())
            .collect::<std::collections::BTreeSet<_>>()
            .len();
        let mut out = format!(
            "<section class=\"nato-screen\" data-state=\"nato-home\"><div class=\"nato-meta\"><span><i class=\"signal-dot\" aria-hidden=\"true\"></i>NATO alphabet</span><span title=\"Unique letters answered without revealing the answer; not a mastery score\">{practiced}<span class=\"nato-muted\"> / 26 practiced</span></span></div>"
        );
        if self.app.practice.mixed_review {
            out.push_str("<p class=\"eyebrow\">All subjects</p>");
        }
        if let Some(p) = self.app.practice.current.as_ref().filter(|p| {
            self.app
                .library
                .versions
                .get(&p.prompt.version_id)
                .is_some_and(crate::library::is_nato)
        }) {
            let prompt = &p.prompt;
            let attempt = p
                .attempt_id
                .as_ref()
                .and_then(|id| self.app.practice.attempts.get(id));
            let state = if attempt.is_some() {
                "exact-result"
            } else {
                "exact-prompt"
            };
            let _ = write!(
                out,
                "<div class=\"nato-stage\" data-state=\"{state}\" data-prompt-id=\"{}\"><div class=\"nato-cue-panel\"><h1 class=\"nato-letter\" id=\"nato-letter\" aria-label=\"Letter {}\">{}</h1></div><div class=\"nato-response-panel\">",
                escape(&prompt.id),
                escape(&prompt.cue),
                escape(&prompt.cue)
            );
            if let Some(a) = attempt {
                let good = a.grade.verdict == crate::grading::Verdict::Good;
                let version = &self.app.library.versions[&a.version_id];
                let word = version
                    .sections
                    .iter()
                    .flat_map(|s| &s.units)
                    .find(|u| a.prompt.unit_ids.contains(&u.id))
                    .map(|u| crate::study::expected_unit(version, u))
                    .unwrap_or("");
                let label = if a.assisted {
                    "With help"
                } else if good {
                    "Correct"
                } else {
                    "Not quite"
                };
                let verdict = if good { "Good" } else { "Again" };
                let tone = if a.assisted {
                    "helped"
                } else if good {
                    "correct"
                } else {
                    "miss"
                };
                let _ = write!(
                    out,
                    "<div class=\"nato-feedback nato-{tone}\" role=\"status\"><p data-verdict=\"{verdict}\">{label}</p><p class=\"nato-word\">{}</p></div>",
                    escape(word)
                );
                if a.assisted {
                    out.push_str("<p class=\"nato-note\">Assisted. Schedule unchanged.</p>");
                }
                out.push_str(&self.result_progress(a));
                out.push_str(&self.form(
                    if self.app.practice.mixed_review {
                        "/practice/review/next"
                    } else {
                        "/practice/next"
                    },
                    "nato-next",
                    "nato-action",
                    "data-next",
                    &format!(
                        "{}{}<button class=\"button primary\">Next <kbd>Enter</kbd></button>",
                        hidden("after", &prompt.id),
                        if self.app.practice.mixed_review {
                            String::new()
                        } else {
                            hidden("study_id", &prompt.study_id)
                        }
                    ),
                ));
            } else if p.source_visible() {
                out.push_str(&self.source_study(p));
            } else {
                out.push_str("<span data-study-phase=\"quiz\" hidden></span>");
                out.push_str(
                    "<p class=\"nato-question\" id=\"nato-question\">What’s the code word?</p>",
                );
                if p.revealed {
                    out.push_str("<p class=\"nato-note\">With help</p>");
                }
                out.push_str(&self.form("/practice/answer", "nato-answer", "nato-answer answer-form", "data-answer-limit=\"32768\"", &format!("{}<label class=\"visually-hidden\" for=\"answer\">Type your answer</label><input id=\"answer\" name=\"answer\" type=\"text\" aria-describedby=\"nato-letter nato-question\" placeholder=\"Code word\" maxlength=\"32768\" enterkeyhint=\"done\" spellcheck=\"false\" autocapitalize=\"none\" autocorrect=\"off\" autocomplete=\"off\" value=\"{}\" autofocus><button class=\"button primary\">Check <kbd>Enter</kbd></button>", hidden("prompt_id", &prompt.id), escape(self.saved_answer(&prompt.id)))));
                if !p.revealed || p.phase == Some(crate::study::StudyPhase::Quiz) {
                    out.push_str(&self.practice_form(
                        "reveal",
                        "nato-reveal",
                        "data-nato-help",
                        "<button class=\"text-button\">Show answer</button>",
                    ));
                }
            }
            out.push_str("</div></div>");
            // A result shows only its own correction. The full reference would
            // prime the next question, so keep it outside the active session.
        } else {
            let has_study = self.app.practice.studies.values().any(|s| {
                self.app
                    .library
                    .versions
                    .get(&s.version_id)
                    .is_some_and(crate::library::is_nato)
            });
            let other_prompt = self.app.practice.current.is_some();
            out.push_str("<div class=\"nato-stage nato-welcome\">");
            if !other_prompt && self.app.protected_goal().is_none() && self.app.occurrence.is_none()
            {
                out.push_str(
                    "<div class=\"nato-specimen\" aria-hidden=\"true\">Aa<span>Alfa</span></div>",
                );
            }
            if other_prompt {
                out.push_str("<h1>Your review is waiting.</h1><p>Finish your saved question before starting NATO.</p><a class=\"button primary\" href=\"/recall\">Resume review</a>");
            } else if has_study
                && (self.path == "/recall"
                    || self.query.get("practice").is_some_and(|v| v == "nato"))
            {
                let has_eligible = self.app.practice.studies.values().any(|s| {
                    nato_versions.contains(s.version_id.as_str())
                        && s.status != crate::study::Status::Dropped
                        && (self
                            .app
                            .practice
                            .cards
                            .values()
                            .any(|c| c.study_id == s.id && c.state.schedule.due_ms <= self.now)
                            || (s.status == crate::study::Status::Active
                                && self
                                    .app
                                    .practice
                                    .cards
                                    .values()
                                    .filter(|c| c.study_id == s.id)
                                    .count()
                                    < 26))
                });
                if has_eligible {
                    out.push_str("<h1>Ready to review.</h1>");
                    out.push_str(&self.form(
                        "/practice/nato/start",
                        "nato-continue",
                        "nato-action",
                        "",
                        "<button class=\"button primary\">Continue practice</button>",
                    ));
                } else {
                    let next_due = self
                        .app
                        .practice
                        .cards
                        .values()
                        .filter(|c| {
                            self.app.practice.studies.get(&c.study_id).is_some_and(|s| {
                                s.status != crate::study::Status::Dropped
                                    && nato_versions.contains(s.version_id.as_str())
                            }) && c.state.schedule.due_ms > self.now
                        })
                        .map(|c| c.state.schedule.due_ms)
                        .min();
                    out.push_str("<h1>All set for now.</h1>");
                    if let Some(ms) = next_due {
                        let _ = write!(out, "<p>Next review {}.</p>", relative(ms, self.now));
                    }
                    out.push_str("<a class=\"text-link\" href=\"/\">Back to start</a>");
                }
            } else {
                let (title, desc, action) = if has_study {
                    ("A little, often.", "", "Continue practice")
                } else {
                    (
                        if self.app.occurrence.is_some() || self.app.protected_goal().is_some() {
                            "Learn the alphabet."
                        } else {
                            "From Alfa to Zulu."
                        },
                        "26 code words. One answer at a time.",
                        "Start practice",
                    )
                };
                let _ = write!(out, "<h1>{title}</h1>");
                if !desc.is_empty() {
                    let _ = write!(out, "<p>{desc}</p>");
                }
                out.push_str(&self.form(
                    "/practice/nato/start",
                    "nato-start",
                    "nato-action",
                    "",
                    &format!("<button class=\"button primary\">{action}</button>"),
                ));
            }
            out.push_str("</div>");
        }
        out.push_str("<div class=\"practice-tools\">");
        out.push_str(&self.lesson_picker(false));
        out.push_str(&self.nato_progress());
        if self.app.practice.current.is_none() {
            out.push_str(&self.nato_reference());
        }
        out.push_str("</div></section>");
        out
    }
    fn create(&self) -> String {
        let prefill = self
            .query
            .get("text")
            .or_else(|| self.query.get("intent"))
            .or_else(|| self.query.get("url"))
            .map(String::as_str)
            .unwrap_or("");
        let draft = if prefill.is_empty() {
            String::new()
        } else {
            format!(
                "<label for=\"unsent-material\">Your unsent material</label><textarea id=\"unsent-material\" rows=\"7\" readonly>{}</textarea>",
                escape(prefill)
            )
        };
        format!(
            "<section class=\"create-page\" data-state=\"create\"><h1>Bring your own content.</h1><p class=\"lede\">Native content generation is retired.</p><p>External agents author the content. Scry helps you remember it with native answer grading. Your saved material and history remain available.</p>{draft}<div class=\"actions\"><a class=\"button primary\" href=\"/library\">Open Library</a><a class=\"text-link\" href=\"/\">Back to review</a></div></section>"
        )
    }
    fn map(&self) -> String {
        let mut goals: Vec<_> = self.app.goals.values().filter(|g| !g.archived).collect();
        goals.sort_by_key(|g| (g.paused, !g.focused, g.created_ms));
        let mut out = String::from(
            "<section class=\"map-page\" data-state=\"map\"><div class=\"page-heading\"><p class=\"eyebrow\">Your growing understanding</p><h1>A map of ideas.</h1><p class=\"lede\">Follow the connections. Revisit a note. Give the ideas that matter a little more attention.</p></div>",
        );
        if goals.is_empty() {
            out.push_str("<div class=\"quiet-empty\"><p>Your map begins with one curiosity.</p><a class=\"button primary\" href=\"/library\">Open Library →</a></div>");
        }
        for (index, goal) in goals.iter().enumerate() {
            let concepts: Vec<_> = goal
                .concept_ids
                .iter()
                .filter_map(|id| self.app.concepts.get(id))
                .filter(|c| !c.archived)
                .collect();
            let _ = write!(
                out,
                "<article class=\"goal-map {}\"><div class=\"goal-map-heading\"><span class=\"folio\">{:02}</span><div><h2><a href=\"/goals/{}\">{}</a></h2><p class=\"goal-meta\">{} ideas{}{} · {}</p></div></div>",
                if goal.paused { "paused" } else { "" },
                index + 1,
                escape(&goal.id),
                self.goal_title(goal),
                concepts.len(),
                if goal.focused { " · In focus" } else { "" },
                if goal.paused { " · Paused" } else { "" },
                status_label(self.goal_phase(goal))
            );
            out.push_str(&constellation(self.app, &concepts, self.now));
            out.push_str("<ol class=\"concept-list\">");
            for c in concepts {
                let _ = write!(
                    out,
                    "<li><a href=\"/concepts/{}\"><span class=\"idea-dot\" aria-hidden=\"true\"></span><span>{}</span><span class=\"idea-status\">{}</span><span aria-hidden=\"true\">↗</span></a></li>",
                    escape(&c.id),
                    self.concept_name(c),
                    concept_state(self.app, c, self.now)
                );
            }
            out.push_str("</ol><div class=\"goal-actions\">");
            out.push_str(&self.form(
                &format!("/goals/{}/focus", goal.id),
                "focus",
                "",
                "",
                &format!(
                    "<button class=\"small-button\">{}</button>",
                    if goal.focused {
                        "Remove focus"
                    } else {
                        "Focus here"
                    }
                ),
            ));
            out.push_str(&self.form(
                &format!("/goals/{}/pause", goal.id),
                "pause",
                "",
                "",
                &format!(
                    "<button class=\"small-button\">{}</button>",
                    if goal.paused { "Resume" } else { "Pause" }
                ),
            ));
            let _ = write!(
                out,
                "<a class=\"text-link\" href=\"/goals/{}\">Reference &amp; input ↗</a></div></article>",
                escape(&goal.id)
            );
        }
        out.push_str(
            "<a class=\"text-link add-curiosity\" href=\"/library\">Browse Library</a></section>",
        );
        out
    }
    fn goal(&self, id: &str) -> String {
        let Some(g) = self.app.goals.get(id) else {
            return self.not_found();
        };
        if self.app.protected_goal() == Some(id) {
            return self.gate(&format!("/goals/{id}"));
        }
        let mut out = format!(
            "<article class=\"reference-page\" data-state=\"goal\"><a class=\"back-link\" href=\"/map\">← Your map</a><div class=\"page-heading\"><p class=\"eyebrow\">A page in your notebook</p><h1>{}</h1><p class=\"fineprint\">Saved {}{}</p></div>",
            escape(&g.title),
            date(g.created_ms),
            if g.archived { " · Archived" } else { "" }
        );
        if g.status != "ready" {
            out.push_str(&self.preparation(g));
        }
        if !g.concept_ids.is_empty() {
            out.push_str("<section class=\"reference-contents\"><p class=\"eyebrow\">The useful starting point</p>");
            for id in &g.concept_ids {
                if let Some(c) = self.app.concepts.get(id).filter(|c| !c.archived) {
                    let _ = write!(
                        out,
                        "<section class=\"reference-idea\"><h2><a href=\"/concepts/{}\">{}</a></h2>{}{}<a class=\"text-link\" href=\"/concepts/{}\">Explore this idea &amp; practice ↗</a></section>",
                        escape(&c.id),
                        self.concept_name(c),
                        self.lede("lede small", c),
                        if self.app.protected_concept(&c.id) {
                            "<p>This idea is in your current question. Open its page to record assistance before reading.</p>".into()
                        } else {
                            self.notes(c)
                        },
                        escape(&c.id)
                    );
                }
            }
            let _ = write!(
                out,
                "</section><a class=\"button primary\" href=\"{}\">Go to practice →</a>",
                legacy_home(self.app)
            );
        }
        let _ = write!(
            out,
            "<details class=\"saved-input\"><summary>Your original request</summary><div class=\"reading preserve\">{}</div>{}</details>",
            escape(&g.intent),
            g.transcript
                .as_ref()
                .map(|t| format!(
                    "<h3>What Scry read in your photo</h3><div class=\"reading preserve\">{}</div>",
                    escape(t)
                ))
                .unwrap_or_default()
        );
        if !g.archived {
            out.push_str(&self.form(
                &format!("/goals/{id}/archive"),
                "archive",
                "archive-form",
                "",
                "<button class=\"text-button danger\">Archive this goal</button>",
            ));
        }
        out.push_str(&self.feedback_history(id, None));
        out.push_str("</article>");
        out
    }
    fn feedback_history(&self, goal_id: &str, concept_id: Option<&str>) -> String {
        let feedback: Vec<_> = self
            .app
            .feedback
            .iter()
            .filter(|f| {
                f.goal_id == goal_id
                    && concept_id.is_none_or(|id| f.concept_id.as_deref() == Some(id))
            })
            .collect();
        if feedback.is_empty() {
            return String::new();
        }
        let mut out = String::from(
            "<details class=\"feedback-history\"><summary>Your saved feedback</summary>",
        );
        for f in feedback.iter().rev() {
            let _ = write!(
                out,
                "<div><p class=\"fineprint\">{} · {}</p><p class=\"reading preserve\">{}</p></div>",
                match f.kind.as_str() {
                    "dispute" => "Flagged material",
                    "confusion" => "Something unclear",
                    _ => "Your refinement request",
                },
                date(f.created_ms),
                escape(&f.text)
            );
        }
        out.push_str("</details>");
        out
    }
    fn preparation(&self, g: &Goal) -> String {
        let job = self.app.jobs.get(&g.job_id);
        let accounted = self
            .app
            .spend
            .iter()
            .filter(|s| job.is_some_and(|j| s.work_id == j.id))
            .map(|s| s.cost_micros.unwrap_or(s.reserved_micros))
            .sum::<u64>() as f64
            / 1_000_000.0;
        let detail = if self.goal_phase(g) == "imported-unfinished" {
            "<p>This request didn't finish. It came from your earlier notebook, where its preparation didn't finish. Your words are saved, and nothing has been sent again.</p>".into()
        } else {
            job.and_then(|j| j.error.as_ref()).map_or_else(String::new, |error| {
                format!("<details><summary>Recorded preparation outcome</summary><p>{}</p></details>", escape(error))
            })
        };
        format!(
            "<section class=\"preparation-panel stopped\"><h2>Saved preparation history</h2><p>Native content generation is retired. This request, its outcome and any existing material are kept; no work will be sent again.</p>{detail}<p>Recorded status: {} · job {}</p><p class=\"fineprint\">${accounted:.2} used or reserved for this preparation. Uncertain outcomes remain accounted.</p><a class=\"text-link\" href=\"/\">Back to review</a></section>",
            escape(&g.status),
            escape(job.map_or("unavailable", |j| &j.status))
        )
    }
    fn concept(&self, id: &str) -> String {
        let Some(c) = self.app.concepts.get(id) else {
            return self.not_found();
        };
        if self.app.protected_concept(id) {
            return self.gate(&format!("/concepts/{id}"));
        }
        let state = concept_evidence(self.app, c, self.now);
        let unaided = state.unaided;
        let helped = state.helped;
        let missed = state.missed;
        let mut out = format!(
            "<article class=\"concept-page\" data-state=\"concept\"><a class=\"back-link\" href=\"/\">← Your concepts</a><div class=\"page-heading\"><h1>{}</h1>{}</div><section class=\"concept-study\"><h2>Study</h2>{}</section><section class=\"practice-evidence\" aria-label=\"Your practice record\"><h2>Practice</h2><p><strong>{unaided}</strong> recalled <span>·</span> <strong>{helped}</strong> with help <span>·</span> <strong>{missed}</strong> missed</p><p class=\"fineprint\">{}</p>{}</section>{}",
            self.concept_name(c),
            self.lede("lede", c),
            self.notes(c),
            recall_label(self.app, c, self.now),
            if !self.active_concept(c) {
                "<p>Archived: retained for reference.</p>".into()
            } else {
                self.form(
                    &format!("/concepts/{id}/practice"),
                    "practice",
                    "",
                    "",
                    "<button class=\"button primary\">Practice this idea →</button>",
                )
            },
            tally(&state)
        );
        out.push_str("<section class=\"saved-questions\"><h3>Saved questions</h3>");
        for q in self
            .app
            .questions
            .values()
            .filter(|q| q.concept_id == id && !q.archived)
        {
            let content = q.content();
            let protected = self.app.protected_concept(&q.concept_id);
            let _ = write!(
                out,
                "<details class=\"question-reference\"><summary>{}</summary>",
                escape(&content.prompt)
            );
            if protected {
                out.push_str(
                    "<p>The answer stays private while your current attempt is unaided.</p>",
                );
            } else {
                let _ = write!(
                    out,
                    "<div class=\"reading\"><p><strong>{}</strong></p><p>{}</p></div>",
                    escape(&content.answer),
                    escape(&content.explanation)
                );
            }
            let _ = write!(
                out,
                "<a class=\"text-link\" href=\"/questions/{}/edit\">Edit future wording ↗</a></details>",
                escape(&q.id)
            );
        }
        out.push_str("</section>");
        if let Some(g) = self.app.goals.get(&c.goal_id) {
            let _ = write!(
                out,
                "<section class=\"concept-resources\"><h2>Resources</h2><a class=\"text-link\" href=\"/goals/{}\">Original material &amp; preparation</a></section>",
                escape(&g.id)
            );
        }
        let dependents: Vec<_> = self
            .app
            .concepts
            .values()
            .filter(|other| self.active_concept(other) && other.prerequisites.contains(&c.id))
            .collect();
        let prerequisites: Vec<_> = c
            .prerequisites
            .iter()
            .filter_map(|id| self.app.concepts.get(id))
            .filter(|p| self.active_concept(p))
            .collect();
        if !prerequisites.is_empty() || !dependents.is_empty() {
            out.push_str("<section class=\"connected\"><h2>Related</h2>");
            if !prerequisites.is_empty() {
                out.push_str("<h3>Builds on</h3>");
                for p in prerequisites {
                    let _ = write!(
                        out,
                        "<a class=\"concept-chip\" href=\"/concepts/{}\">{}</a>",
                        escape(&p.id),
                        self.concept_name(p)
                    );
                }
            }
            if !dependents.is_empty() {
                out.push_str("<h3>Used by</h3>");
                for p in dependents {
                    let _ = write!(
                        out,
                        "<a class=\"concept-chip\" href=\"/concepts/{}\">{}</a>",
                        escape(&p.id),
                        self.concept_name(p)
                    );
                }
            }
            out.push_str("</section>");
        }
        let body = "<label for=\"feedback\">What is still unclear?</label><textarea id=\"feedback\" name=\"text\" rows=\"3\" maxlength=\"4000\" required placeholder=\"Tell Scry what you need from this idea…\"></textarea><button class=\"button secondary\">Save feedback</button>";
        out.push_str(&format!("<section class=\"refinement\"><h2>Keep the conversation with your learning.</h2>{}</section>",self.form(&format!("/concepts/{id}/feedback"),"feedback","","",body)));
        if !c.archived {
            out.push_str(&self.form(
                &format!("/concepts/{id}/archive"),
                "archive",
                "archive-form",
                "",
                "<button class=\"text-button danger\">Archive this idea</button>",
            ));
        }
        out.push_str(&self.feedback_history(&c.goal_id, Some(id)));
        out.push_str("</article>");
        out
    }
    fn history(&self) -> String {
        if self.app.practice.protected() {
            return String::from(
                "<section class=\"empty-stage gate\" data-state=\"history\"><h1>History</h1><p class=\"lede\">History is hidden while you answer from memory.</p><a class=\"button primary\" href=\"/recall\">Return to practice</a></section>",
            );
        }
        let mut out = String::from(
            "<section class=\"history-page\" data-state=\"history\"><div class=\"page-heading\"><h1>History</h1></div>",
        );
        if self.app.events.is_empty() && self.app.practice.attempts.is_empty() {
            out.push_str("<p class=\"quiet-empty\">Your first practice will appear here.</p>");
        }
        if !self.app.practice.attempts.is_empty() {
            out.push_str("<section aria-label=\"Exact recall\"><h2>Exact recall</h2>");
            if self.app.protected_goal().is_some() {
                out.push_str("<p class=\"fineprint\">History is hidden while you answer from memory.</p><a class=\"text-link\" href=\"/notebook\">Return to the question</a>");
            } else {
                let mut attempts = self.app.practice.attempts.values().collect::<Vec<_>>();
                attempts.sort_by_key(|a| std::cmp::Reverse(a.sequence));
                for attempt in attempts {
                    out.push_str(&self.exact_history_attempt(attempt));
                }
            }
            out.push_str("</section>");
        }
        if self
            .app
            .operations
            .values()
            .any(|r| r.acting_token.is_some())
        {
            out.push_str("<details class=\"history-operations\"><summary>Agent activity</summary>");
            for (operation, receipt) in &self.app.operations {
                if let Some(id) = &receipt.acting_token {
                    let name = self
                        .app
                        .owner_tokens
                        .get(id)
                        .map(|t| t.info.name.as_str())
                        .unwrap_or(id);
                    out.push_str(&format!(
                        "<p class=\"token-history\">Owner token {} ({}) · {} · operation {}</p>",
                        escape(name),
                        escape(id),
                        escape(receipt.path.as_deref().unwrap_or("saved change")),
                        escape(operation)
                    ));
                }
            }
            out.push_str("</details>");
        }
        for event in self.app.events.iter().rev() {
            if self.app.protected_concept(&event.concept_id) {
                out.push_str("<p class=\"fineprint\">An earlier attempt is hidden while you work on its question from memory.</p>");
                continue;
            }
            let corrected = self.app.overrides.iter().any(|o| o.event_id == event.id);
            let outcome = if event.is_ungraded() {
                "Ungraded"
            } else if event.assisted {
                "With help"
            } else {
                outcome_label(&event.result.outcome)
            };
            let _ = write!(
                out,
                "<article class=\"history-entry\"><p class=\"history-meta\"><span>{}</span> · {} · {}{}</p><h2>{}</h2><details><summary>See this attempt <span aria-hidden=\"true\">＋</span></summary><dl class=\"answer-pair\"><div><dt>You answered</dt><dd>{}</dd></div><div><dt>The answer then</dt><dd>{}</dd></div></dl><p class=\"reading\">{}</p><p class=\"fineprint\">{}</p></details></article>",
                outcome,
                date(event.result.reviewed_ms),
                authority_label(&event.result.authority),
                if corrected {
                    " · Grade corrected later"
                } else {
                    ""
                },
                escape(&event.presentation.prompt),
                escape(&event.answer),
                escape(&event.presentation.answer),
                escape(&event.presentation.explanation),
                if corrected {
                    "The original attempt is preserved. The correction is recorded separately."
                } else {
                    "This is the wording you saw at the time."
                }
            );
        }
        out.push_str("</section>");
        out
    }
    fn exact_history_attempt(&self, a: &crate::study::Attempt) -> String {
        use crate::{grading::Verdict, study::Scope};
        let version = &self.app.library.versions[&a.version_id];
        let scope = match &a.prompt.scope {
            Scope::Unit { position, .. } => format!("Unit {position}"),
            Scope::Section { section_id } => format!(
                "Section: {}",
                version
                    .sections
                    .iter()
                    .find(|s| &s.id == section_id)
                    .unwrap()
                    .heading
            ),
            Scope::Whole => "Whole text".into(),
        };
        let verdict = |v| if v == Verdict::Good { "Good" } else { "Again" };
        let label = if a.disclosure {
            "Shown before answering"
        } else {
            verdict(a.grade.verdict)
        };
        let mut out = format!(
            "<article class=\"history-entry exact-history-entry\" data-attempt-id=\"{}\" data-disclosure=\"{}\"><p class=\"history-meta\"><span>{label}</span> · {} · {}</p><h2>{}</h2><p class=\"fineprint\">{} · {}</p><details><summary>See this attempt <span aria-hidden=\"true\">＋</span></summary>{}",
            escape(&a.id),
            a.disclosure,
            date(a.created_ms),
            if a.assisted {
                "Assisted practice"
            } else {
                "Unassisted practice"
            },
            escape(&a.prompt.title),
            escape(version_name(version)),
            escape(&scope),
            Self::exact_prompt_context(&a.prompt)
        );
        let answer = if a.disclosure {
            "No answer submitted.".into()
        } else if a.raw_answer.is_empty() {
            "No words submitted.".into()
        } else {
            escape(&a.raw_answer)
        };
        let _ = write!(
            out,
            "<dl class=\"answer-pair\"><div><dt>You answered</dt><dd class=\"exact-history-answer\">{answer}</dd></div><div><dt>Expected text</dt><dd>"
        );
        for unit in version
            .sections
            .iter()
            .flat_map(|s| &s.units)
            .filter(|u| a.prompt.unit_ids.contains(&u.id))
        {
            let _ = write!(
                out,
                "<p class=\"reading exact-expected\">{}</p>",
                escape(crate::study::expected_unit(version, unit))
            );
        }
        out.push_str("</dd></div></dl>");
        if let Some(hint) = &a.grade.wrong_version_hint {
            let _ = write!(
                out,
                "<p class=\"notice\">Your answer is closer to Version {}.</p>",
                escape(&hint.name)
            );
        }
        let _ = write!(
            out,
            "<h3>{}</h3><ol class=\"exact-diff\" aria-label=\"Diff\">",
            if a.disclosure {
                "Saved comparison · no answer submitted"
            } else {
                "Word by word"
            }
        );
        for d in &a.grade.diff {
            out.push_str(&Self::exact_diff_word(d, false, a.grade.units.len() > 1));
        }
        let _ = write!(
            out,
            "</ol><p class=\"fineprint exact-authority\">{} · {}{}</p><ul class=\"exact-history-effects\">",
            escape(&a.grade.authority.name),
            escape(&a.grade.authority.grader_version),
            if a.disclosure {
                " · comparison only, not a recall Verdict"
            } else {
                ""
            }
        );
        for e in &a.effects {
            let (kind, status) = if e.assisted {
                ("assisted", "Assisted practice · schedule unchanged".into())
            } else if e.extra_practice {
                ("extra", "Extra practice · already scheduled today".into())
            } else if e.scheduled {
                (
                    "scheduled",
                    format!("Scheduled · next due {}", date(e.after.schedule.due_ms)),
                )
            } else {
                ("unchanged", "Schedule unchanged".into())
            };
            let original = if a.disclosure {
                String::new()
            } else {
                format!(" data-verdict=\"{}\"", verdict(e.verdict))
            };
            let _ = write!(
                out,
                "<li data-unit-index=\"{}\" data-schedule-effect=\"{kind}\"{original}>Unit {}: {}{status}</li>",
                e.unit_index,
                e.unit_index + 1,
                if a.disclosure {
                    String::new()
                } else {
                    format!("{} · ", verdict(e.verdict))
                }
            );
        }
        out.push_str("</ul>");
        let mut corrections = self
            .app
            .practice
            .corrections
            .values()
            .filter(|c| c.attempt_id == a.id)
            .collect::<Vec<_>>();
        corrections.sort_by_key(|c| c.sequence);
        for c in corrections {
            let unit = a
                .effects
                .iter()
                .find(|e| e.unit_id == c.unit_id)
                .unwrap()
                .unit_index
                + 1;
            let _ = write!(
                out,
                "<div class=\"exact-history-correction\" data-correction-id=\"{}\"><h3>Owner Correction · Unit {unit}</h3><p>{} · {} · Authority: {}</p><p class=\"fineprint\">The original Attempt and Diff above are unchanged.</p></div>",
                escape(&c.id),
                verdict(c.verdict),
                date(c.created_ms),
                escape(&c.authority)
            );
            if let Some(token) = &c.acting_token {
                let _ = write!(
                    out,
                    "<p class=\"fineprint\">Correction by Owner token {}</p>",
                    escape(token)
                );
            }
        }
        if let Some(token) = &a.acting_token {
            let _ = write!(
                out,
                "<p class=\"fineprint\">Attempt by Owner token {}</p>",
                escape(token)
            );
        }
        let _ = write!(
            out,
            "<p class=\"fineprint\">Version {} · hash {}</p><p class=\"fineprint\">Schedule authority: {}</p></details></article>",
            escape(&a.version_id),
            escape(&a.version_hash),
            escape(&a.algorithm)
        );
        out
    }
    fn owner_tokens(&self) -> String {
        let mut out = String::from(
            "<details class=\"settings-section\" id=\"owner-tokens\"><summary>Agent access</summary><p>Let your CLI or an agent act for you. Save the secret when it appears; it is shown once.</p>",
        );
        out.push_str(&self.form("/tokens/create", "token-create", "token-form", "", "<label for=\"token-name\">Name</label><input id=\"token-name\" name=\"name\" maxlength=\"120\" required><label for=\"token-expiry\">Expires (UTC, optional)</label><input id=\"token-expiry\" name=\"expires_at\" type=\"datetime-local\"><label><input type=\"checkbox\" name=\"paid\" value=\"true\"> Allow paid work</label><button class=\"button secondary\">Create Owner token</button>"));
        for token in self.app.owner_tokens.values() {
            let t = &token.info;
            out.push_str(&format!(
                "<article><h3>{}</h3><p>Created {} · Last used {} · Expiry {} · Paid work {}</p>",
                escape(&t.name),
                date(t.created_ms),
                t.last_used_ms.map(date).unwrap_or_else(|| "never".into()),
                t.expires_ms.map(date).unwrap_or_else(|| "none".into()),
                if t.paid { "allowed" } else { "off" }
            ));
            if t.revoked_ms.is_some() {
                out.push_str("<p>Revoked</p>");
            } else {
                out.push_str(&self.form(
                    "/tokens/revoke",
                    &t.id,
                    "token-form",
                    "",
                    &format!(
                        "{}<button class=\"button secondary\">Revoke {}</button>",
                        hidden("token_id", &t.id),
                        escape(&t.name)
                    ),
                ));
            }
            out.push_str("</article>");
        }
        out.push_str("</details>");
        out
    }
    fn settings(&self) -> String {
        let mut pace =
            String::from("<fieldset class=\"pace-options\"><legend>New items per day</legend>");
        for (value, title, desc) in [
            ("light", "3", "Light"),
            ("steady", "6", "Steady"),
            ("intense", "12", "Intense"),
        ] {
            let _ = write!(
                pace,
                "<label><input type=\"radio\" name=\"pace\" value=\"{value}\" {}><span><strong>{title}</strong><span>{desc}</span></span></label>",
                if self.app.preferences.pace == value {
                    "checked"
                } else {
                    ""
                }
            );
        }
        pace.push_str("</fieldset><button class=\"button secondary\">Save pace</button>");
        let spent = self.app.spent(self.now) as f64 / 1_000_000.0;
        let unknown = self
            .app
            .spend
            .iter()
            .filter(|s| s.cost_micros.is_none())
            .count();
        let backup=match self.app.backup.completed_ms{Some(time)=>format!("Last verified backup: {}.{}",date(time),if self.now-time>DAY_MS{" A fresh backup needs attention."}else{""}),None=>"No verified remote backup is recorded. Recovery needs attention before personal use.".into()};
        let resume = if self.app.restored_paused {
            format!(
                "<section class=\"notice\" id=\"preparation\"><h2>Preparation is paused.</h2><p>This restored notebook has background work paused. Resume queued and future meaning checks when you are ready. Native content generation stays retired. Historical uncertain work stays stopped; its allowance and your learning history stay saved.</p>{}</section>",
                self.form(
                    "/settings/resume",
                    "resume",
                    "",
                    "",
                    "<button class=\"button primary\">Resume meaning checks</button>"
                )
            )
        } else {
            String::new()
        };
        let backup_action = self.form(
            "/settings/backup",
            "backup",
            "backup-form",
            "",
            "<button class=\"button secondary\">Create a verified backup</button>",
        );
        let mut out = format!(
            "<section class=\"settings-page\" data-state=\"settings\"><div class=\"page-heading\"><h1>Settings</h1></div>{resume}{}<details class=\"settings-section\"><summary>Usage allowance</summary><p class=\"spend-number\">${spent:.2}<span> / $3.50</span></p><p>Used or reserved in the last 24 hours. Each external meaning check reserves $0.50 before it begins. Historical generation usage remains accounted; uncertain outcomes keep their reservation.</p><p class=\"fineprint\">{unknown} reservations without confirmed final usage. Provider limits apply independently.</p></details><section class=\"settings-section\"><h2>Backup</h2><p>{backup}</p><details><summary>Recovery targets</summary><p class=\"fineprint\">Daily remote snapshots and 30-day retention target 24 hours of recoverable work and a 60-minute recovery. A target is not a guarantee.</p></details>{}{backup_action}</section><section class=\"settings-section\"><h2>Export</h2><a class=\"button secondary\" href=\"/export\" download>Export your notebook ↓</a></section><details class=\"settings-section font-colophon\"><summary>About</summary><p>Literata and Atkinson Hyperlegible Next.</p><a href=\"/assets/fonts/OFL.txt\">Font licenses</a></details></section>",
            self.form("/settings", "pace", "pace-form", "", &pace),
            self.app
                .backup
                .error
                .as_ref()
                .map(|e| format!(
                    "<details><summary>Backup details</summary><p>{}</p></details>",
                    escape(e)
                ))
                .unwrap_or_default()
        );
        out.push_str(&self.owner_tokens());
        if self.query.get("__synthetic").is_some_and(|v| v == "true") {
            out.push_str("<p class=\"synthetic-note\">This is an isolated synthetic notebook. It does not prove production access, model usefulness, or remote recovery.</p>");
        }
        out
    }
    fn fix_status(&self, q: &Question) -> String {
        if self
            .app
            .jobs
            .values()
            .any(|j| j.question_id.as_deref() == Some(q.id.as_str()) && j.kind == "fix")
        {
            "<aside class=\"notice\"><strong>Saved draft history</strong><p>Native content generation is retired. Existing drafts and outcomes remain saved; nothing will be sent again.</p></aside>".into()
        } else {
            String::new()
        }
    }
    fn edit(&self, id: &str) -> String {
        let Some(q) = self.app.questions.get(id) else {
            return self.not_found();
        };
        if self.app.protected_concept(&q.concept_id) {
            return self.gate(&format!("/questions/{id}/edit"));
        }
        let use_draft = q.draft_for_version == Some(q.content().version)
            && q.draft.is_some()
            && !self.query.get("current").is_some_and(|v| v == "true");
        let c = if use_draft {
            q.draft.as_ref().unwrap()
        } else {
            q.content()
        };
        let body = format!(
            "{}<label for=\"prompt\">Question</label><textarea id=\"prompt\" name=\"prompt\" rows=\"3\" required maxlength=\"8192\">{}</textarea><label for=\"answer\">Expected answer</label><textarea id=\"answer\" name=\"answer\" rows=\"2\" required maxlength=\"8192\">{}</textarea><label for=\"variants\">Accepted variants <span>One per line, optional</span></label><textarea id=\"variants\" name=\"variants\" rows=\"2\">{}</textarea>{}<label for=\"explanation\">Explanation</label><textarea id=\"explanation\" name=\"explanation\" rows=\"5\" required maxlength=\"16384\">{}</textarea><div class=\"thumb-actions\"><button class=\"button primary\">Save future wording →</button></div>",
            format_args!(
                "{}{}{}{}{}",
                hidden("content_version", &q.content().version.to_string()),
                hidden("use_draft", if use_draft { "true" } else { "false" }),
                hidden("kind", &c.kind),
                hidden("basis", &c.basis),
                hidden("quotes", &c.quotes.join("\n"))
            ),
            escape(
                self.query
                    .get("prompt")
                    .map(String::as_str)
                    .unwrap_or(&c.prompt)
            ),
            escape(
                self.query
                    .get("answer")
                    .map(String::as_str)
                    .unwrap_or(&c.answer)
            ),
            escape(
                self.query
                    .get("variants")
                    .map(String::as_str)
                    .unwrap_or(&c.variants.join("\n"))
            ),
            if c.choices.is_empty() {
                String::new()
            } else {
                format!(
                    "<label for=\"choices\">Answer choices <span>One per line</span></label><textarea id=\"choices\" name=\"choices\" rows=\"4\">{}</textarea>",
                    escape(
                        self.query
                            .get("choices")
                            .map(String::as_str)
                            .unwrap_or(&c.choices.join("\n"))
                    )
                )
            },
            escape(
                self.query
                    .get("explanation")
                    .map(String::as_str)
                    .unwrap_or(&c.explanation)
            )
        );
        format!(
            "<section class=\"edit-page\" data-state=\"edit\"><a class=\"back-link\" href=\"/concepts/{}\">← Back to the idea</a><div class=\"page-heading\"><p class=\"eyebrow\">Keep the material honest</p><h1>A better question.</h1><p class=\"lede\">Edits apply to future practice. The question you already saw and your past attempts remain unchanged.</p></div>{}</section>",
            escape(&q.concept_id),
            format_args!(
                "{}{}{}",
                self.fix_status(q),
                if use_draft {
                    format!(
                        "<aside class=\"notice\"><strong>A better version is ready for your review.</strong><p>It is saved as a draft. Save below to use it in future practice.</p><a href=\"/questions/{id}/edit?current=true\">Use current wording instead</a></aside>"
                    )
                } else if q.draft.is_some() {
                    format!(
                        "<aside class=\"notice\"><a href=\"/questions/{id}/edit\">Review the prepared draft →</a></aside>"
                    )
                } else {
                    String::new()
                },
                self.form(
                    &format!("/questions/{id}/edit"),
                    "edit",
                    "edit-form",
                    "",
                    &body
                )
            )
        )
    }
    fn not_found(&self) -> String {
        format!(
            "<section class=\"empty-stage\" data-state=\"not-found\">{}<p class=\"eyebrow\">A page has gone astray</p><h1>Nothing on this page.</h1><p class=\"lede\">Your saved notebook is still here. Return to your practice or explore the map.</p><a class=\"button primary\" href=\"/\">Back to review →</a></section>",
            emblem("large")
        )
    }
}

pub const VERSION: &str = env!("CARGO_PKG_VERSION");
pub const SOURCE_REVISION: &str = match option_env!("SCRY_SOURCE_REVISION") {
    Some(revision) => revision,
    None => "unverified-development-build",
};

fn release_notes(markdown: &str) -> String {
    use pulldown_cmark::{Event, Tag};
    let events = pulldown_cmark::Parser::new(markdown).map(|mut event| {
        if let Event::Start(Tag::Link { dest_url, .. } | Tag::Image { dest_url, .. }) = &mut event
            && !dest_url.starts_with("https://github.com/misty-step/scry/")
            && !dest_url.starts_with('#')
        {
            *dest_url = "#".into();
        }
        match event {
            pulldown_cmark::Event::Html(text) | pulldown_cmark::Event::InlineHtml(text) => {
                pulldown_cmark::Event::Text(text)
            }
            other => other,
        }
    });
    let mut notes = String::new();
    pulldown_cmark::html::push_html(&mut notes, events);
    notes
}

pub fn changelog() -> String {
    let notes = release_notes(include_str!("../CHANGELOG.md"));
    let content = format!(
        "<section class=\"reading release-notes\"><p class=\"eyebrow\">This build</p><p>Version {} · source <code>{}</code></p>{notes}</section>",
        escape(VERSION),
        escape(SOURCE_REVISION)
    );
    shell("Changelog", "/changelog", &content, None, false, false, "/")
}

#[cfg(test)]
#[test]
fn generated_markdown_cannot_activate_raw_html_or_untrusted_urls() {
    let notes = release_notes(
        "# Changelog\n\n<script>PRIVATE_CANARY</script>\n\n[click](javascript:alert%281%29) ![pixel](https://tracking.invalid/pixel)\n\n[commit](https://github.com/misty-step/scry/commit/abc)",
    );
    assert!(notes.contains("<h1>Changelog</h1>"));
    assert!(!notes.contains("<script>"));
    assert!(!notes.contains("javascript:"));
    assert!(!notes.contains("tracking.invalid"));
    assert!(notes.contains("href=\"https://github.com/misty-step/scry/commit/abc\""));
    assert!(!changelog().contains("style=\""));
}

/// Only known, same-origin Library pages can follow the assistance POST.
pub fn library_destination(path: &str) -> bool {
    matches!(path, "/library" | "/library/add")
        || path.strip_prefix("/library/versions/").is_some_and(|id| {
            id.len() == 32
                && id
                    .bytes()
                    .all(|b| b.is_ascii_hexdigit() && !b.is_ascii_uppercase())
        })
}
fn version_name(v: &crate::library::Version) -> &str {
    if v.name.trim().is_empty() {
        "Supplied text"
    } else {
        &v.name
    }
}
fn version_metadata(v: &crate::library::Version) -> String {
    let kind = match v.kind {
        crate::library::Kind::Text => "Text",
        crate::library::Kind::OrderedList => "Ordered list",
        crate::library::Kind::KeyedMapping => "Keyed mapping",
    };
    format!(
        "{kind} · {} Sections · {} Units",
        v.sections.len(),
        v.sections.iter().map(|s| s.units.len()).sum::<usize>()
    )
}

fn exact_stream_active(app: &App) -> bool {
    app.practice.current.is_some()
        || app
            .practice
            .studies
            .values()
            .any(|s| s.status != crate::study::Status::Dropped)
}

fn legacy_home(_app: &App) -> &'static str {
    "/notebook"
}

pub fn mutation_redirect<'a>(app: &App, path: &str, redirect: &'a str) -> &'a str {
    if matches!(path, "/library/version/add" | "/library/mapping/add") {
        redirect.strip_prefix("/api/v1").unwrap_or("/library")
    } else if path.starts_with("/practice/review/")
        || (path.starts_with("/practice/") && app.practice.mixed_review)
    {
        "/"
    } else if path.starts_with("/practice/") {
        if let Some(route) = app
            .practice
            .current
            .as_ref()
            .and_then(|p| app.library.versions.get(&p.prompt.version_id))
            .and_then(crate::lessons::mapping_route)
        {
            route
        } else if matches!(
            redirect,
            "/?practice=baby-soothing" | "/?practice=baby-everyday" | "/?practice=baby-montessori"
        ) {
            redirect
        } else {
            "/recall"
        }
    } else if redirect == "/"
        && (path.starts_with("/review/")
            || path == "/settings/resume"
            || path
                .strip_prefix("/concepts/")
                .is_some_and(|p| p.ends_with("/practice")))
    {
        legacy_home(app)
    } else {
        redirect
    }
}

pub fn mutation_error_path<'a>(app: &App, path: &'a str) -> &'a str {
    if path == "/library/mapping/add" {
        "/library"
    } else if path == "/library/version/add" {
        "/library/add"
    } else if matches!(path, "/create" | "/add") || path.ends_with("/edit") {
        path
    } else {
        mutation_redirect(app, path, "/")
    }
}

pub fn render(
    app: &App,
    path: &str,
    query: &BTreeMap<String, String>,
    now_ms: i64,
    notice: Option<&str>,
) -> String {
    let view = View {
        app,
        path,
        query,
        now: now_ms,
    };
    let content = match path {
        "/" | "/recall"
            if app.practice.current.as_ref().is_some_and(|p| {
                app.practice.studies[&p.prompt.study_id].lesson
                    && !crate::library::is_nato(&app.library.versions[&p.prompt.version_id])
            }) =>
        {
            view.lesson_screen()
        }
        "/" if !app.practice.mixed_review
            && query.get("practice").is_some_and(|id| {
                matches!(
                    id.as_str(),
                    "baby-soothing" | "baby-everyday" | "baby-montessori"
                )
            }) =>
        {
            view.mapping_starter_home(query.get("practice").unwrap())
        }
        "/" if !app.practice.mixed_review
            && app
                .practice
                .current
                .as_ref()
                .and_then(|p| app.library.versions.get(&p.prompt.version_id))
                .and_then(crate::lessons::mapping_route)
                .is_some() =>
        {
            view.exact_stream()
        }
        "/lessons" => format!(
            "<section class=\"nato-screen lesson-screen\"><h1>Practice</h1>{}</section>",
            view.lesson_picker(true)
        ),
        "/" if app.practice.current.is_none()
            && !query.get("practice").is_some_and(|v| v == "nato") =>
        {
            view.mixed_home()
        }
        "/" if app.practice.current.as_ref().is_some_and(|p| {
            !crate::library::is_nato(&app.library.versions[&p.prompt.version_id])
        }) =>
        {
            view.exact_stream()
        }
        "/" => view.nato_screen(),
        "/recall"
            if app.practice.current.as_ref().is_some_and(|p| {
                app.library
                    .versions
                    .get(&p.prompt.version_id)
                    .is_some_and(crate::library::is_nato)
            }) =>
        {
            view.nato_screen()
        }
        "/recall" if app.practice.current.is_none() && app.practice.mixed_review => {
            view.mixed_home()
        }
        "/recall"
            if app.practice.current.is_none()
                && app.practice.studies.values().any(|s| {
                    app.library
                        .versions
                        .get(&s.version_id)
                        .is_some_and(crate::library::is_nato)
                }) =>
        {
            view.nato_screen()
        }
        "/recall" if exact_stream_active(app) => view.exact_stream(),
        "/recall" | "/library" => view.library(),
        "/library/add" => view.library_add(),
        "/notebook" => format!(
            "<nav class=\"understanding-nav\" aria-label=\"Understanding\"><a href=\"/\">Review</a><a href=\"/map\">Map</a></nav>{}",
            view.study()
        ),
        "/create" | "/add" => view.create(),
        "/map" => view.map(),
        "/history" => view.history(),
        "/settings" => view.settings(),
        "/gate" => {
            let target = query
                .get("return_to")
                .filter(|v| v.starts_with('/') && !v.starts_with("//"))
                .cloned()
                .unwrap_or_else(|| {
                    format!(
                        "/concepts/{}",
                        query
                            .get("concept")
                            .cloned()
                            .or_else(|| app.occurrence.as_ref().map(|o| o.concept_id.clone()))
                            .unwrap_or_default()
                    )
                });
            view.gate(&target)
        }
        _ => {
            let segments: Vec<_> = path.trim_matches('/').split('/').collect();
            match segments.as_slice() {
                ["library", "versions", id] => view.library_version(id),
                ["goals", id] => view.goal(id),
                ["concepts", id] => view.concept(id),
                ["questions", id, "edit"] => view.edit(id),
                _ => view.not_found(),
            }
        }
    };
    let title = match path {
        "/library/add" => "Add a text",
        "/" => "Review",
        "/recall" => "Review",
        p if p.starts_with("/library") => "Library",
        "/create" | "/add" => "Create",
        "/map" => "Your map",
        "/history" => "History",
        "/settings" => "Settings",
        _ => "Your notebook",
    };
    let poll = path != "/"
        && app
            .occurrence
            .as_ref()
            .is_some_and(|o| matches!(o.phase.as_str(), "checking" | "pending"));
    shell(
        title,
        path,
        &content,
        notice,
        query.get("__synthetic").is_some_and(|v| v == "true"),
        poll,
        if exact_stream_active(app) {
            "/recall"
        } else {
            "/"
        },
    )
}

pub fn error_page(app: &App, status: u16, message: &str, now_ms: i64) -> String {
    let _ = (app, now_ms);
    let title = if status == 401 || status == 403 {
        "Your private notebook is locked."
    } else if status == 409 {
        "Let's return to what is saved."
    } else {
        "A moment to pause."
    };
    let content = format!(
        "<section class=\"empty-stage\" data-state=\"error\">{}<p class=\"eyebrow\">Request {status}</p><h1>{title}</h1><p class=\"lede\">{}</p><a class=\"button primary\" href=\"/\">Back to review →</a></section>",
        emblem("large"),
        escape(message)
    );
    shell(
        "A moment to pause",
        "/",
        &content,
        None,
        false,
        false,
        if exact_stream_active(app) {
            "/recall"
        } else {
            "/"
        },
    )
}

fn shell(
    title: &str,
    path: &str,
    content: &str,
    notice: Option<&str>,
    synthetic: bool,
    poll: bool,
    practice_href: &str,
) -> String {
    let notice = notice
        .map(|n| format!("<div class=\"notice\" role=\"status\">{}</div>", escape(n)))
        .unwrap_or_default();
    format!(
        "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1, viewport-fit=cover, interactive-widget=resizes-content\"><meta name=\"color-scheme\" content=\"light dark\"><meta name=\"theme-color\" media=\"(prefers-color-scheme: light)\" content=\"#f5f2ea\"><meta name=\"theme-color\" media=\"(prefers-color-scheme: dark)\" content=\"#191e1d\"><meta name=\"robots\" content=\"noindex,nofollow\"><title>{} · Scry</title><link rel=\"icon\" href=\"/assets/icon.svg\" type=\"image/svg+xml\"><link rel=\"stylesheet\" href=\"/assets/app.css\"><script src=\"/assets/app.js\" defer></script></head><body><a class=\"skip-link\" href=\"#main\">Skip to the page</a><div class=\"page-frame\"><header class=\"masthead\"><a class=\"wordmark\" href=\"/\" aria-label=\"Scry, home\">{}<span>scry<span class=\"wordmark-period\">.</span></span></a><details class=\"account-menu\"><summary aria-label=\"Open menu\">Menu</summary><nav aria-label=\"Main navigation\"><a href=\"{practice_href}\" {}>Practice</a><a href=\"/lessons\">Lessons</a><a href=\"/library\">Library</a><a href=\"/history\">History</a><a href=\"/settings\">Settings</a><a href=\"/notebook\">Earlier practice</a><a href=\"/changelog\">Changelog</a></nav></details></header>{}<div id=\"request-status\" class=\"request-status\" role=\"status\" aria-live=\"polite\" hidden></div><main id=\"main\" tabindex=\"-1\" {}>{notice}{content}</main></div></body></html>",
        escape(title),
        emblem("wordmark-emblem"),
        if path == "/" || path == "/map" || path.starts_with("/concepts/") {
            "aria-current=\"page\""
        } else {
            ""
        },
        if synthetic {
            "<div class=\"synthetic-badge\">Synthetic notebook · isolated preview</div>"
        } else {
            ""
        },
        if poll { "data-poll" } else { "" }
    ).replace("#f5f2ea", if content.contains("nato-screen") { "#f0efe9" } else { "#f5f2ea" })
     .replace("#191e1d", if content.contains("nato-screen") { "#151719" } else { "#191e1d" })
}
fn hidden(name: &str, value: &str) -> String {
    format!(
        "<input type=\"hidden\" name=\"{}\" value=\"{}\">",
        escape(name),
        escape(value)
    )
}
fn emblem(class: &str) -> String {
    format!(
        "<svg class=\"emblem {}\" width=\"48\" height=\"48\" viewBox=\"0 0 64 64\" aria-hidden=\"true\" focusable=\"false\"><circle cx=\"32\" cy=\"32\" r=\"25\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"1\"/><path d=\"M16 41 30 19 47 32 31 45 30 19M16 41l15 4\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"1\"/><circle cx=\"16\" cy=\"41\" r=\"2.5\" fill=\"currentColor\"/><circle cx=\"30\" cy=\"19\" r=\"3\" fill=\"currentColor\"/><circle cx=\"47\" cy=\"32\" r=\"2.5\" fill=\"currentColor\"/><path d=\"m31 39 1.5 4.5L37 45l-4.5 1.5L31 51l-1.5-4.5L25 45l4.5-1.5Z\" fill=\"currentColor\"/></svg>",
        escape(class)
    )
}
fn notes(app: &App, c: &Concept) -> String {
    let Some(note) = c.notes.last() else {
        return "<p class=\"fineprint\">No note is available yet.</p>".into();
    };
    let paragraphs = note_text(app, c, &note.text)
        .split("\n\n")
        .filter(|p| !p.trim().is_empty())
        .map(|p| format!("<p>{}</p>", escape(p)))
        .collect::<String>();
    format!(
        "<section class=\"concept-note\" aria-label=\"Saved explanation\"><div class=\"reading preserve\">{paragraphs}</div>{}</section>",
        provenance(&note.basis, &note.quotes)
    )
}
fn provenance(basis: &str, quotes: &[String]) -> String {
    let label = match basis {
        "source" | "material" | "private" => "From your material",
        "web" => "From saved web material",
        _ => "General knowledge",
    };
    let mut out = format!("<div class=\"provenance\"><span>{label}</span>");
    if !quotes.is_empty() {
        out.push_str("<details><summary>What this draws on</summary>");
        for quote in quotes {
            let _ = write!(out, "<blockquote>{}</blockquote>", escape(quote));
        }
        out.push_str("</details>");
    }
    out.push_str("</div>");
    out
}
fn constellation(app: &App, concepts: &[&Concept], now: i64) -> String {
    fn depth(index: usize, concepts: &[&Concept], visited: &mut Vec<usize>) -> usize {
        if visited.contains(&index) {
            return 0;
        }
        visited.push(index);
        let result = concepts[index]
            .prerequisites
            .iter()
            .filter_map(|id| concepts.iter().position(|c| c.id == *id))
            .map(|i| depth(i, concepts, visited) + 1)
            .max()
            .unwrap_or(0)
            .min(6);
        visited.pop();
        result
    }
    let concepts = &concepts[..concepts.len().min(10)];
    let depths: Vec<_> = (0..concepts.len())
        .map(|i| depth(i, concepts, &mut Vec::new()))
        .collect();
    let max_depth = depths.iter().copied().max().unwrap_or(0);
    let mut columns = BTreeMap::<usize, usize>::new();
    for d in &depths {
        *columns.entry(*d).or_default() += 1;
    }
    let mut indexes = BTreeMap::<usize, usize>::new();
    let positions: Vec<_> = depths
        .iter()
        .map(|d| {
            let i = indexes.entry(*d).or_default();
            let rank = *i;
            *i += 1;
            let count = columns[d];
            let x = (*d * 460)
                .checked_div(max_depth)
                .map_or(320, |value| 90 + value);
            let y = 30 + (rank + 1) * 120 / (count + 1);
            (x, y)
        })
        .collect();
    let mut out = String::from(
        "<svg class=\"constellation\" viewBox=\"0 0 640 180\" aria-hidden=\"true\" focusable=\"false\"><path class=\"chart-guide\" d=\"M20 90H620M320 20V160\"/>",
    );
    for (i, c) in concepts.iter().enumerate() {
        for pre in &c.prerequisites {
            if let Some(j) = concepts.iter().position(|p| p.id == *pre) {
                let _ = write!(
                    out,
                    "<path class=\"chart-edge\" d=\"M{} {}L{} {}\"/>",
                    positions[j].0, positions[j].1, positions[i].0, positions[i].1
                );
            }
        }
    }
    for (i, c) in concepts.iter().enumerate() {
        let (x, y) = positions[i];
        let brightness = concept_evidence(app, c, now).brightness;
        let _ = write!(
            out,
            "<circle class=\"chart-halo\" cx=\"{x}\" cy=\"{y}\" r=\"{}\"/><circle class=\"chart-star brightness-{brightness}\" cx=\"{x}\" cy=\"{y}\" r=\"{}\"/><text class=\"chart-number\" x=\"{}\" y=\"{}\">{:02}</text>",
            7 + usize::from(brightness) * 2,
            2.0 + f64::from(brightness) * 0.45,
            x + 13,
            y - 10,
            i + 1
        );
    }
    out.push_str("<circle class=\"chart-distant\" cx=\"596\" cy=\"31\" r=\"1\"/><circle class=\"chart-distant\" cx=\"203\" cy=\"153\" r=\"1\"/><circle class=\"chart-distant\" cx=\"510\" cy=\"142\" r=\"1\"/></svg>");
    out
}
fn concept_evidence(app: &App, c: &Concept, now: i64) -> ConceptState {
    let questions: Vec<_> = app
        .questions
        .values()
        .filter(|q| q.concept_id == c.id && !q.archived)
        .map(|q| {
            let attempts = app
                .events
                .iter()
                .filter(|e| e.question_id == q.id && !e.is_ungraded())
                .map(|e| {
                    let correction = app.overrides.iter().rev().find(|v| v.event_id == e.id);
                    let correct = correction
                        .map(|v| v.correct)
                        .unwrap_or(e.result.outcome == "correct");
                    let rating = if e.assisted {
                        0
                    } else if correction.is_some() {
                        if correct { 3 } else { 1 }
                    } else {
                        e.result.rating
                    };
                    Attempt {
                        at_ms: e.result.reviewed_ms,
                        rating,
                        assisted: e.assisted,
                        outcome: if correction.is_some() {
                            if correct { "correct" } else { "wrong" }.into()
                        } else {
                            e.result.outcome.clone()
                        },
                    }
                })
                .collect();
            QuestionEvidence {
                card: q.card.clone(),
                level: match q.content().kind.as_str() {
                    "choice" => "recognize",
                    "explain" => "explain",
                    _ => "recall",
                }
                .into(),
                attempts,
            }
        })
        .collect();
    compute_concept_state(&questions, now)
}
fn concept_state(app: &App, c: &Concept, now: i64) -> &'static str {
    if c.archived {
        return "Archived";
    }
    let state = concept_evidence(app, c, now);
    if state.tally.is_empty() && c.introduced_ms.is_some() {
        return "Introduced";
    }
    match state.status.as_str() {
        "solid" => "Solid",
        "fading" => "Fading",
        "learning" => "Learning",
        _ => "New",
    }
}
fn recall_label(app: &App, c: &Concept, now: i64) -> String {
    let state = concept_evidence(app, c, now);
    if state.recall < 0.0 {
        return "Not enough unaided practice for a recall estimate yet. Reading and helped practice are recorded separately.".into();
    }
    format!(
        "Estimated recall now: {:.0}%. This is a scheduling estimate, not a certainty or a measure of understanding.",
        state.recall * 100.0
    )
}
fn tally(state: &ConceptState) -> String {
    if state.tally.is_empty() {
        return String::new();
    }
    let mut out =
        String::from("<ol class=\"practice-tally\" aria-label=\"Recent attempts, oldest first\">");
    for item in &state.tally {
        let (class, label) = match item.as_str() {
            "u" => ("unaided", "Recalled without help"),
            "h" => ("helped", "Practiced with help"),
            _ => ("missed", "Missed"),
        };
        let _ = write!(
            out,
            "<li class=\"tally-{class}\"><span class=\"sr-only\">{label}</span></li>"
        );
    }
    out.push_str("</ol>");
    out
}
fn status_label(value: &str) -> &'static str {
    match value {
        "ready" => "Ready",
        "failed" => "Needs attention",
        "unknown" => "Outcome uncertain",
        "partial" => "Partly ready",
        "paused" | "notebook-paused" | "imported-unfinished" => "Preparation paused",
        "blocked" | "budget" | "superseded" => "Needs attention",
        _ => "Saved preparation history",
    }
}
fn outcome_label(value: &str) -> &'static str {
    match value {
        "correct" | "success" | "recalled" | "warm_correct" => "Recalled",
        "helped" | "shown" => "With help",
        _ => "Missed",
    }
}
fn authority_label(value: &str) -> &'static str {
    match value {
        "ungraded" => "Saved, ungraded",
        "exact" => "Exact match",
        "jev" => "Meaning checked",
        "learner" => "Self-checked",
        "reveal" => "Answer revealed",
        _ => "Recorded",
    }
}
fn date(ms: i64) -> String {
    chrono::DateTime::from_timestamp_millis(ms)
        .map(|d| d.format("%b %-d, %Y · %H:%M UTC").to_string())
        .unwrap_or_else(|| "at an unknown time".into())
}
fn relative(ms: i64, now: i64) -> String {
    let diff = ms - now;
    if diff < 60_000 {
        "in a moment".into()
    } else if diff < 3_600_000 {
        format!(
            "in {} minute{}",
            diff / 60_000,
            if diff / 60_000 == 1 { "" } else { "s" }
        )
    } else if diff < DAY_MS {
        format!(
            "in {} hour{}",
            diff / 3_600_000,
            if diff / 3_600_000 == 1 { "" } else { "s" }
        )
    } else {
        format!(
            "in {} day{}",
            diff / DAY_MS,
            if diff / DAY_MS == 1 { "" } else { "s" }
        )
    }
}

pub fn asset(path: &str) -> Option<(&'static str, &'static [u8])> {
    match path {
        "/assets/app.css" => Some((
            "text/css; charset=utf-8",
            include_bytes!("../assets/app.css"),
        )),
        "/assets/app.js" => Some((
            "text/javascript; charset=utf-8",
            include_bytes!("../assets/app.js"),
        )),
        "/assets/icon.svg" => Some(("image/svg+xml", include_bytes!("../assets/icon.svg"))),
        "/assets/fonts/Literata.woff2" => Some((
            "font/woff2",
            include_bytes!("../internal/web/assets/fonts/Literata.woff2"),
        )),
        "/assets/fonts/Literata-Italic.woff2" => Some((
            "font/woff2",
            include_bytes!("../internal/web/assets/fonts/Literata-Italic.woff2"),
        )),
        "/assets/fonts/AtkinsonHyperlegibleNext.woff2" => Some((
            "font/woff2",
            include_bytes!("../internal/web/assets/fonts/AtkinsonHyperlegibleNext.woff2"),
        )),
        "/assets/fonts/OFL.txt" => Some((
            "text/plain; charset=utf-8",
            include_bytes!("../internal/web/assets/fonts/OFL.txt"),
        )),
        _ => None,
    }
}

/// Deliberately outside render query/state: a refresh must never recover the secret.
pub fn token_created(secret: &str) -> String {
    let content = format!(
        "<section><h1>Save your Owner token</h1><p>This secret is shown once. Use <code>scry login --token-stdin</code> to save it on your computer.</p><textarea aria-label=\"Owner token secret\" readonly>{}</textarea><p><a href=\"/settings#owner-tokens\">Return to Owner tokens</a></p></section>",
        escape(secret)
    );
    shell(
        "Owner token",
        "/settings",
        &content,
        None,
        false,
        false,
        "/",
    )
}
