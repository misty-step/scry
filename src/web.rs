//! The private, server-rendered notebook. All authored and generated text is escaped.
use crate::learning::{Attempt, ConceptState, compute_concept_state};
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

/// What `/notebook` leads with when no question is held: nothing, or where
/// the notebook's saved generation requests stopped.
#[derive(Clone, Copy)]
enum EmptyState<'a> {
    Nothing,
    Preparing,
    Paused,
    Stopped(&'a Goal),
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
    fn goal_title(&self, goal: &Goal) -> String {
        escape(&goal.title)
    }
    fn concept_name(&self, concept: &Concept) -> String {
        escape(&idea_title(self.app, concept))
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
            Some(g) if legacy_idea(concept) => {
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
    /// Review evidence exists for a Card only once it has an unaided Attempt.
    fn attempted_unaided(&self, card_id: &str) -> bool {
        self.app
            .practice
            .attempts
            .values()
            .any(|a| !a.disclosure && !a.assisted && a.card_ids().any(|id| id == card_id))
    }
    /// A due review asks only for Units answered unaided before and due when asked.
    fn due_review(&self, p: &crate::study::Presentation) -> bool {
        p.targets
            .iter()
            .all(|t| t.before.schedule.due_ms <= p.created_ms && self.attempted_unaided(&t.card_id))
    }
    /// Whether a previously answered Card of an in-scope Study is due now.
    fn review_due(&self, in_scope: impl Fn(&crate::study::Study) -> bool) -> bool {
        self.app.practice.cards.values().any(|c| {
            c.state.schedule.due_ms <= self.now
                && self
                    .app
                    .practice
                    .studies
                    .get(&c.study_id)
                    .is_some_and(|s| s.status != crate::study::Status::Dropped && in_scope(s))
                && self.attempted_unaided(&c.id)
        })
    }
    /// A mapping pair's authored question: its key, in study and in Quiz alike.
    fn mapping_question(&self, prompt: &crate::study::Prompt) -> Option<&str> {
        let v = self.app.library.versions.get(&prompt.version_id)?;
        if v.kind != crate::library::Kind::KeyedMapping {
            return None;
        }
        v.sections
            .iter()
            .flat_map(|s| &s.units)
            .find(|u| prompt.unit_ids.first() == Some(&u.id))
            .and_then(|u| crate::library::mapping_pair(&u.text))
            .map(|(key, _)| key)
    }
    /// Context for a phrase Quiz: closed until asked for, and never the answer.
    fn previous_phrase(&self, p: &crate::study::Presentation) -> String {
        let v = &self.app.library.versions[&p.prompt.version_id];
        if v.kind != crate::library::Kind::Text
            || !matches!(p.prompt.scope, crate::study::Scope::Unit { .. })
        {
            return String::new();
        }
        p.targets[0]
            .unit_index
            .checked_sub(1)
            .and_then(|i| v.sections.iter().flat_map(|s| &s.units).nth(i))
            .map_or(String::new(), |u| {
                format!(
                    "<details class=\"lesson-context\"><summary>Previous phrase</summary><p>{}</p></details>",
                    escape(&u.text)
                )
            })
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
                    "<section class=\"library-work\"><h2><a href=\"/library/works/{}\">{}</a></h2>",
                    escape(&item.work.id),
                    escape(&item.work.title)
                );
                if !item.work.aliases.is_empty() {
                    let _ = write!(
                        out,
                        "<p class=\"fineprint\">Also known as {}</p>",
                        escape(&item.work.aliases.join(" · "))
                    );
                }
                out.push_str(&self.work_relations(&item.work.id));
                out.push_str(&self.version_list(&item.versions, false));
                out.push_str("</section>");
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
    fn work_link(&self, id: &str) -> String {
        let title = self
            .app
            .library
            .works
            .get(id)
            .map_or("", |w| w.title.as_str());
        format!(
            "<a class=\"text-link\" href=\"/library/works/{}\">{}</a>",
            escape(id),
            escape(title)
        )
    }
    fn version_link(&self, id: &str) -> String {
        let name = self.app.library.versions.get(id).map_or("", version_name);
        format!(
            "<a class=\"text-link\" href=\"/library/versions/{}\">{}</a>",
            escape(id),
            escape(name)
        )
    }
    /// The Relations of a Work, shown on both Works each one links.
    fn work_relations(&self, id: &str) -> String {
        use crate::library::Relation;
        let mut out = String::new();
        for relation in self.app.library.relations.values() {
            let (label, other) = match relation {
                Relation::PartOf {
                    work_id,
                    parent_work_id,
                } if work_id == id => ("Part of", parent_work_id),
                Relation::PartOf {
                    work_id,
                    parent_work_id,
                } if parent_work_id == id => ("Includes", work_id),
                Relation::RelatedTo {
                    work_id,
                    related_work_id,
                } if work_id == id => ("Related to", related_work_id),
                Relation::RelatedTo {
                    work_id,
                    related_work_id,
                } if related_work_id == id => ("Related to", work_id),
                _ => continue,
            };
            let _ = write!(
                out,
                "<p class=\"fineprint library-relation\">{label} {}</p>",
                self.work_link(other)
            );
        }
        out
    }
    /// A correction and the Version it supersedes each name the other.
    fn version_relations(&self, id: &str) -> String {
        use crate::library::Relation;
        let mut out = String::new();
        for relation in self.app.library.relations.values() {
            if let Relation::Supersedes {
                version_id,
                previous_id,
            } = relation
            {
                if version_id == id {
                    let _ = write!(
                        out,
                        "<p class=\"fineprint library-relation\">Corrects {}</p>",
                        self.version_link(previous_id)
                    );
                } else if previous_id == id {
                    let _ = write!(
                        out,
                        "<p class=\"fineprint library-relation\" data-superseded>Corrected by {}</p>",
                        self.version_link(version_id)
                    );
                }
            }
        }
        out
    }
    /// With `detail`, each Version also shows its Provenance and Rights status.
    fn version_list(&self, versions: &[&crate::library::Version], detail: bool) -> String {
        let mut out = String::from("<ul class=\"library-versions\">");
        for v in versions {
            let _ = write!(
                out,
                "<li><a class=\"version-link\" href=\"/library/versions/{}\">{}</a><p class=\"fineprint\">{}{}</p>{}{}</li>",
                escape(&v.id),
                escape(version_name(v)),
                version_metadata(v),
                if detail {
                    format!(
                        "<br>{} · {}",
                        provenance_label(v.provenance),
                        rights_label(v.rights)
                    )
                } else {
                    String::new()
                },
                self.version_relations(&v.id),
                self.study_status(&v.id)
            );
        }
        out.push_str("</ul>");
        out
    }
    fn library_work(&self, id: &str) -> String {
        if self.app.practice.protected() {
            return self.library_gate();
        }
        let Ok(item) = self.app.library.show_work(id) else {
            return self.not_found();
        };
        let mut out = format!(
            "<section class=\"library-page\" data-state=\"library-work\"><a class=\"text-link\" href=\"/library\">Back to Library</a><h1>{}</h1>",
            escape(&item.work.title)
        );
        if !item.work.aliases.is_empty() {
            let _ = write!(
                out,
                "<p class=\"fineprint\">Also known as {}</p>",
                escape(&item.work.aliases.join(" · "))
            );
        }
        if !item.work.context.is_empty() {
            let _ = write!(
                out,
                "<p class=\"fineprint source-context\">{}</p>",
                escape(&item.work.context)
            );
        }
        out.push_str(&self.work_relations(id));
        if item.versions.is_empty() {
            out.push_str("<p class=\"lede\">This Work has no Versions.</p>");
        } else {
            out.push_str(&self.version_list(&item.versions, true));
        }
        out.push_str("</section>");
        out
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
        if self.query.get("existing").is_some_and(|v| v == "1") {
            out = out.replacen(
                "data-state=\"library-version\">",
                "data-state=\"library-version\"><p class=\"notice\" role=\"status\">That text was already in the Library. Nothing was added.</p>",
                1,
            );
        }
        let corrections = self.version_relations(id);
        if !corrections.is_empty() {
            out = out.replacen(
                "<details class=\"library-details version-details\">",
                &format!("{corrections}<details class=\"library-details version-details\">"),
                1,
            );
        }
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
        let rights = rights_label(v.rights);
        let _ = write!(
            out,
            "<p class=\"fineprint\">{} · {rights}</p>{}<a class=\"text-link\" href=\"/library/works/{}\">Every Version of {}</a></details>",
            provenance_label(v.provenance),
            self.work_relations(&item.work.id),
            escape(&item.work.id),
            escape(&item.work.title)
        );
        let reviewable = crate::study::reviewable(v).is_ok();
        if !reviewable && v.kind == crate::library::Kind::Text {
            let _ = write!(
                out,
                "<p role=\"status\">Practice is unavailable. {}</p>",
                escape(&crate::study::reviewable(v).unwrap_err().message)
            );
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
            out.push_str("</div>");
            let mut paces = String::new();
            for (value, label, pace) in [
                ("light", "3 · Light", crate::study::Pace::Light),
                ("steady", "6 · Steady", crate::study::Pace::Steady),
                ("intense", "12 · Intense", crate::study::Pace::Intense),
            ] {
                let _ = write!(
                    paces,
                    "<label><input type=\"radio\" name=\"pace\" value=\"{value}\"{}> {label}</label>",
                    if study.pace == pace { " checked" } else { "" }
                );
            }
            out.push_str(&self.form(
                "/practice/pace",
                "pace",
                "",
                "",
                &format!(
                    "{}<fieldset><legend>Pace · new Units each day</legend>{paces}</fieldset><button class=\"button quiet\">Set Pace</button>",
                    hidden("study_id", &study.id)
                ),
            ));
            out.push_str("</details>");
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
        if !ready {
            let sid = study.map(|s| s.id.as_str());
            return self.rest(&lesson.input.title, |s| Some(s.id.as_str()) == sid, "");
        }
        let sid = study.map(|s| s.id.as_str());
        let timing = match next {
            Some(at) if at > self.now => next_review_line(at),
            Some(_) if self.review_due(|s| Some(s.id.as_str()) == sid) => "Ready to review.".into(),
            _ => "Start with a few short questions.".into(),
        };
        let mut out = format!(
            "<section class=\"nato-screen\" data-state=\"mapping-starter-home\"><h1>{}</h1><p>{}</p>",
            escape(&lesson.input.title),
            escape(&timing)
        );
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
        out.push_str(&self.lesson_picker(false));
        out.push_str("</section>");
        out
    }
    fn unavailable_practice(&self, p: &crate::study::Presentation) -> String {
        let mut out = format!(
            "<section class=\"empty-stage exact-study\" data-state=\"practice-unavailable\" data-prompt-id=\"{}\"><h1>Practice unavailable</h1><p>{}</p>",
            escape(&p.prompt.id),
            escape(
                &crate::study::reviewable(&self.app.library.versions[&p.prompt.version_id])
                    .expect_err("unavailable Version")
                    .message
            )
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
            return self.mixed_home();
        };
        if crate::study::reviewable(&self.app.library.versions[&p.prompt.version_id]).is_err() {
            return self.unavailable_practice(p);
        }
        let prompt = &p.prompt;
        let attempt = p
            .attempt_id
            .as_ref()
            .and_then(|id| self.app.practice.attempts.get(id));
        let judged = attempt.and_then(|a| self.judged_state(a));
        let state = if attempt.is_some() {
            judged.unwrap_or("exact-result")
        } else {
            "exact-prompt"
        };
        let readiness = crate::lessons::catalog().into_iter().find(|l| {
            l.id.starts_with("baby-") && l.matches(&self.app.library.versions[&prompt.version_id])
        });
        let question = self.mapping_question(prompt);
        let mut out = format!(
            "<section class=\"study exact-study{}\" data-state=\"{state}\" data-prompt-id=\"{}\"{}>",
            if readiness.is_some() {
                " mapping-study readiness-study"
            } else if question.is_some() {
                " mapping-study"
            } else {
                ""
            },
            escape(&prompt.id),
            Self::judged_poll(judged)
        );
        if self.app.practice.mixed_review {
            out.push_str("<p class=\"eyebrow\">All subjects</p>");
        }
        if let Some(a) = attempt.filter(|_| judged.is_some()) {
            let retry = self.form(
                "/practice/lesson/next",
                "exact-retry",
                "exact-form",
                "data-next",
                &format!(
                    "{}{}<button class=\"button primary\">Try this phrase again</button>",
                    hidden("after", &a.prompt.id),
                    hidden("mode", "retry")
                ),
            );
            let (body, actions) = self.judged_result(a, &retry, |secondary| {
                self.form(
                    if self.app.practice.mixed_review {
                        "/practice/review/next"
                    } else {
                        "/practice/next"
                    },
                    "exact-next",
                    "exact-form",
                    if secondary { "" } else { "data-next" },
                    &format!(
                        "{}{}<button class=\"{}\">Next</button>",
                        hidden("after", &a.prompt.id),
                        // A focused mapping lesson stays in its own Study.
                        if !self.app.practice.mixed_review
                            && crate::lessons::mapping_route(
                                &self.app.library.versions[&a.version_id]
                            )
                            .is_some()
                        {
                            hidden("study_id", &a.prompt.study_id)
                        } else {
                            String::new()
                        },
                        if secondary {
                            "text-button"
                        } else {
                            "button primary"
                        }
                    ),
                )
            });
            let _ = write!(out, "{body}<div class=\"result-actions\">{actions}</div>");
        } else if let Some(a) = attempt {
            out.push_str(&self.exact_result(a));
        } else if p.source_visible() {
            // A pair is studied under its authored question, safety cue included,
            // set as its Quiz sets it.
            if let (Some(lesson), Some(question)) = (&readiness, question) {
                let _ = write!(
                    out,
                    "<p class=\"eyebrow\">{}</p><div class=\"exact-cue\"><h1 class=\"reading\">{}</h1></div>",
                    escape(&lesson.input.title),
                    escape(question)
                );
            } else {
                let _ = write!(
                    out,
                    "<h1>{}</h1>",
                    escape(question.unwrap_or_else(|| crate::study::visible_prompt_title(prompt)))
                );
            }
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
                if prompt.context.is_none() {
                    out.push_str(&self.previous_phrase(p));
                }
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
                    "<label class=\"answer-label\" for=\"answer\">Your answer</label><textarea id=\"answer\" name=\"answer\" rows=\"4\" maxlength=\"32768\" enterkeyhint=\"done\" spellcheck=\"false\" autocapitalize=\"sentences\"{}>\n{saved}</textarea>",
                    // Enter starts a new line in an answer of several Units.
                    if matches!(prompt.scope, crate::study::Scope::Unit { .. }) {
                        ""
                    } else {
                        " data-multiline"
                    }
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
        let good = a.verdict() == Some(Verdict::Good);
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
        // A missed phrase shows its expected wording at once and offers a guided retry.
        let retry = !good
            && self.app.library.versions[&a.version_id].kind == crate::library::Kind::Text
            && matches!(a.prompt.scope, crate::study::Scope::Unit { .. });
        if retry {
            let _ = write!(
                out,
                "<div class=\"lesson-target\"><h2>The phrase to practise</h2>{}</div>",
                self.exact_expected()
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
        if retry {
            out.push_str(&self.form(
                "/practice/lesson/next",
                "exact-retry",
                "exact-form",
                "data-next",
                &format!(
                    "{}{}<button class=\"button primary\">Try this phrase again</button>",
                    hidden("after", &a.prompt.id),
                    hidden("mode", "retry")
                ),
            ));
        }
        out.push_str(&self.form(
            if self.app.practice.mixed_review {
                "/practice/review/next"
            } else {
                "/practice/next"
            },
            "exact-next",
            "exact-form",
            if retry { "" } else { "data-next" },
            &format!(
                    "{}{}<button class=\"{}\">Next</button>",
                    hidden("after", &a.prompt.id),
                    if !self.app.practice.mixed_review
                        && crate::lessons::mapping_route(&self.app.library.versions[&a.version_id])
                            .is_some()
                    {
                        hidden("study_id", &a.prompt.study_id)
                    } else {
                        String::new()
                    },
                    if retry { "text-button" } else { "button primary" }
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
    /// The `data-state` of a result whose answer was left for Jev to judge.
    fn judged_state(&self, a: &crate::study::Attempt) -> Option<&'static str> {
        use crate::study::JudgmentState;
        Some(match crate::study::judgment_view(self.app, a)?.state {
            JudgmentState::Checking => "exact-checking",
            JudgmentState::Ungraded => "exact-ungraded",
            JudgmentState::Decided => "exact-result",
        })
    }
    /// Only a result still being checked asks the page to look again.
    fn judged_poll(state: Option<&str>) -> &'static str {
        if state == Some("exact-checking") {
            " data-poll"
        } else {
            ""
        }
    }
    /// The local comparison of an answer left for Jev. It is evidence beside
    /// the judgment and is always labelled as not the Verdict.
    fn judged_comparison(&self, a: &crate::study::Attempt, expected: &str) -> String {
        let mut out = format!("<h2>Expected text</h2>{expected}");
        let version = &self.app.library.versions[&a.version_id];
        if version.kind == crate::library::Kind::OrderedList
            && let Some(judgment) = &a.judgment
        {
            // The same items the comparison graded: the Prompt's own scope,
            // which a Section of a longer list does not start at its first item.
            let count = judgment.targets.len();
            let start = judgment.targets[0].unit_index;
            if let Ok(feedback) =
                crate::ordered::feedback(version, start..start + count, &a.raw_answer)
            {
                let _ = write!(
                    out,
                    "<h2>Item-by-item comparison, not the Verdict</h2><p class=\"fineprint\">{} of {count} items matched · {} in order.</p>",
                    feedback.recalled, feedback.in_order
                );
            }
        } else {
            out.push_str(
                "<h2>Word-by-word comparison, not the Verdict</h2><ol class=\"exact-diff\" aria-label=\"Diff\">",
            );
            for d in &a.grade.diff {
                out.push_str(&Self::exact_diff_word(d, false, a.grade.units.len() > 1));
            }
            out.push_str("</ol>");
        }
        let _ = write!(
            out,
            "<p class=\"fineprint exact-authority\">{} · {} · comparison only, not the Verdict</p>",
            escape(&a.grade.authority.name),
            escape(&a.grade.authority.grader_version)
        );
        out
    }
    /// "Judged by Jev (recall-v1; model)".
    fn judged_by(decided: &crate::study::Decided) -> String {
        format!(
            "Judged by Jev ({})",
            escape(&decided.authority.grader_version.replace(";model=", "; "))
        )
    }
    /// A deliberate, paid action: it says what it reserves.
    fn check_again(&self, a: &crate::study::Attempt, class: &str) -> String {
        format!(
            "{}<p class=\"fineprint\">Check again asks Jev once more. It reserves ${:.2} of today's allowance.</p>",
            self.form(
                "/practice/check",
                &format!("check-{}", a.id),
                class,
                "",
                &format!(
                    "{}<button class=\"button secondary\">Check again</button>",
                    hidden("attempt_id", &a.id)
                ),
            ),
            RESERVATION as f64 / 1_000_000.0
        )
    }
    /// The held result of an answer left for Jev, in each of its states: being
    /// checked, saved and not graded, or decided. Returns the result and its
    /// actions; `next(secondary)` is the caller's own way onward. An answer
    /// with no Verdict is never styled or counted as a miss or a success.
    fn judged_result(
        &self,
        a: &crate::study::Attempt,
        retry: &str,
        next: impl Fn(bool) -> String,
    ) -> (String, String) {
        use crate::grading::{Change, Verdict};
        use crate::study::JudgmentState;
        let view = crate::study::judgment_view(self.app, a).expect("an answer left for Jev");
        let version = &self.app.library.versions[&a.version_id];
        let answer = format!(
            "<div class=\"answer-pair\"><span>You answered</span><p>{}</p></div>",
            escape(&a.raw_answer)
        );
        let full = |summary: &str, answer: &str| {
            format!(
                "<details class=\"exact-full-answer\"><summary>{summary}</summary><h2>{}</h2>{}{answer}{}</details>",
                escape(&a.prompt.title),
                Self::exact_prompt_context(&a.prompt),
                self.judged_comparison(a, &self.exact_expected())
            )
        };
        let Some(decided) = view.decided else {
            if view.state == JudgmentState::Checking {
                return (
                    format!(
                        "<div class=\"checking\" role=\"status\"><p class=\"eyebrow\">Your answer is saved</p><h1>Checking it.</h1><p>Jev is judging your answer. You can leave and come back; it stays here.</p></div>{answer}"
                    ),
                    format!(
                        "<a class=\"text-link\" href=\"{}\">Refresh</a>{}",
                        escape(self.path),
                        next(false)
                    ),
                );
            }
            let again = if view.checks_left {
                self.check_again(a, "exact-form")
            } else {
                "<p class=\"fineprint\">Three checks are saved for this answer. It stays saved and not graded.</p>".into()
            };
            return (
                format!(
                    "<div class=\"feedback feedback-self\" role=\"status\"><h1>Saved, not graded.</h1></div><p class=\"notice\">{}</p><p class=\"fineprint\">This answer has no Verdict and its schedule is unchanged.</p>{answer}{}",
                    escape(
                        view.check
                            .and_then(|c| c.reason.as_deref())
                            .unwrap_or("Jev did not judge this answer.")
                    ),
                    full("Expected text and comparison", "")
                ),
                format!("{again}{}", next(false)),
            );
        };
        let good = decided.verdict == Verdict::Good;
        let verdict = if good { "Good" } else { "Again" };
        let mut out = format!(
            "<div class=\"feedback feedback-{}\" role=\"status\"><h1 data-verdict=\"{verdict}\">{verdict}.</h1></div><p class=\"fineprint exact-authority\" data-authority=\"jev\">{}</p>",
            if good { "correct" } else { "miss" },
            Self::judged_by(decided)
        );
        if a.prompt.cue_level == crate::study::Cue::MappingKey
            && let Some(feedback) = crate::study::mapping_feedback(self.app, a)
        {
            let _ = write!(
                out,
                "<p class=\"mapping-result\"><span>{}</span> <span aria-hidden=\"true\">→</span> <strong>{}</strong></p>",
                escape(&feedback.key),
                escape(&feedback.expected)
            );
            if let Some(answered_key) = feedback.answered_key {
                let _ = write!(
                    out,
                    "<p class=\"fineprint\">That answer belongs to key {}.</p>",
                    escape(&answered_key)
                );
            }
        }
        if good
            && (version.kind == crate::library::Kind::OrderedList
                || a.grade
                    .diff
                    .iter()
                    .any(|d| !matches!(d.change, Change::Correct { .. })))
        {
            out.push_str("<p class=\"fineprint\">Jev accepted this answer as the same recall, though it differs from the expected text in the comparison below.</p>");
        }
        if let Some(hint) = &a.grade.wrong_version_hint {
            let _ = write!(
                out,
                "<p class=\"notice\">Your answer is closer to Version {}.</p>",
                escape(&hint.name)
            );
        }
        // A missed phrase shows its expected wording at once and offers a guided retry.
        let retry_offered = !good
            && version.kind == crate::library::Kind::Text
            && matches!(a.prompt.scope, crate::study::Scope::Unit { .. });
        if retry_offered {
            let _ = write!(
                out,
                "<div class=\"lesson-target\"><h2>The phrase to practise</h2>{}</div>",
                self.exact_expected()
            );
        }
        if a.effects.iter().any(|e| e.superseded) {
            out.push_str("<p class=\"fineprint\" data-schedule-effect=\"superseded\">A newer answer was already recorded; schedule unchanged.</p>");
        } else if a.assisted {
            out.push_str(
                "<p class=\"helped-label\">Assisted practice · the schedule stayed unchanged.</p>",
            );
        } else if a.effects.iter().all(|e| e.extra_practice) {
            out.push_str("<p class=\"fineprint\">Extra practice · schedule unchanged.</p>");
        }
        out.push_str(&full("Full answer", &answer));
        out.push_str(&self.result_progress(a));
        (
            out,
            format!(
                "{}{}",
                if retry_offered { retry } else { "" },
                next(retry_offered)
            ),
        )
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
    /// The earlier concept notebook's held question, as it was left. It is
    /// history: nothing here is a form, and no state of it asks to be polled.
    fn earlier(&self) -> String {
        let Some(o) = self.app.occurrence.as_ref() else {
            return self.earlier_empty();
        };
        let state = if o.phase == "result" && o.result.is_some() {
            "earlier-result"
        } else if o.answer.is_some() {
            "earlier-ungraded"
        } else {
            "earlier-unanswered"
        };
        let mut out = format!("<section class=\"study\" data-state=\"{state}\">");
        if let Some(c) = self.app.concepts.get(&o.concept_id) {
            let (label, href) = self.study_chip(c);
            let _ = write!(
                out,
                "<a class=\"concept-chip\" href=\"{href}\"><span aria-hidden=\"true\">✦</span>{label}</a>"
            );
        }
        let _ = write!(
            out,
            "<h1 class=\"question\">{}</h1>",
            escape(&o.presentation.prompt)
        );
        out.push_str(&match state {
            "earlier-result" => self.earlier_result(o),
            "earlier-ungraded" => self.earlier_ungraded(o),
            _ => self.earlier_unanswered(o),
        });
        out.push_str("</section>");
        format!("{}{out}", self.pending())
    }
    fn earlier_unanswered(&self, o: &Occurrence) -> String {
        let mut out = String::new();
        if !o.presentation.choices.is_empty() {
            out.push_str("<ol class=\"reading earlier-choices\">");
            for choice in &o.presentation.choices {
                let _ = write!(out, "<li>{}</li>", escape(choice));
            }
            out.push_str("</ol>");
        }
        out.push_str("<div class=\"feedback feedback-self\"><div><h2>Not answered.</h2><p>Scry no longer serves or grades the earlier concept notebook.</p></div></div>");
        if o.assisted {
            out.push_str("<p class=\"helped-label\">The reference had been opened.</p>");
        }
        out
    }
    /// A saved answer with no Verdict. The check is named by its stored
    /// status alone; the message stored with it stays in export.
    fn earlier_ungraded(&self, o: &Occurrence) -> String {
        let status = o
            .assessment_id
            .as_ref()
            .and_then(|id| self.app.assessments.get(id))
            .map(|check| check.status.as_str());
        let (name, label) = status.map_or(("none", "No check was recorded."), check_label);
        format!(
            "<div class=\"feedback feedback-self\"><div><h2>Your answer is saved, not graded.</h2><p data-check=\"{name}\">{label}</p></div></div>{}",
            self.answer_pair(o)
        )
    }
    fn earlier_result(&self, o: &Occurrence) -> String {
        let Some(result) = &o.result else {
            return String::new();
        };
        let helped = o.assisted || result.outcome == "helped" || result.authority == "reveal";
        let correct = matches!(
            result.outcome.as_str(),
            "correct" | "success" | "recalled" | "warm_correct"
        );
        let (class, icon, title) = if helped {
            ("helped", "◉", "With help.")
        } else if correct {
            ("correct", "✓", "Recalled.")
        } else {
            ("miss", "×", "Missed.")
        };
        let authority = if result.corrected {
            "Your Correction"
        } else {
            authority_label(&result.authority)
        };
        format!(
            "<div class=\"feedback feedback-{class}\"><span class=\"result-mark\" aria-hidden=\"true\">{icon}</span><div><h2>{title}</h2><p>{authority}</p></div></div>{}",
            self.answer_pair(o)
        )
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
    fn empty_state(&self) -> EmptyState<'_> {
        let mut state = EmptyState::Nothing;
        for goal in self.app.goals.values().filter(|g| !g.archived) {
            if goal.paused {
                continue;
            }
            match self.goal_phase(goal) {
                "notebook-paused" => return EmptyState::Paused,
                "queued" | "candidates" | "sent" | "critic-sent" | "preparing" => {
                    return EmptyState::Preparing;
                }
                "ready" | "partial" | "imported-unfinished" => {}
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
    /// Nothing is held and nothing is selected: the notebook is history. A
    /// saved generation request still says where it stopped, as before.
    fn earlier_empty(&self) -> String {
        let saved = !self.app.goals.is_empty() || !self.app.events.is_empty();
        let (state_name, title, desc, destination, action) = match self.empty_state() {
            EmptyState::Nothing => {
                let lede = if saved {
                    "No question is held. Scry no longer serves or grades this notebook; everything saved is on the Map and in History."
                } else {
                    "Nothing is saved in the earlier concept notebook."
                };
                return format!(
                    "{}<section class=\"empty-stage\" data-state=\"earlier-empty\">{}<h1>Earlier practice</h1><p class=\"lede\">{lede}</p></section>",
                    self.pending(),
                    emblem("large")
                );
            }
            EmptyState::Preparing => (
                "preparing",
                "Your saved request is preserved.",
                "Native content generation is retired. Your request and any existing material remain available.",
                "/library".to_string(),
                "Browse Library",
            ),
            EmptyState::Paused => (
                "preparation-paused",
                "Preparation is paused.",
                "Your requests are saved. Native content generation is retired. Meaning checks can resume separately in Settings. Your existing notes and history are still available.",
                "/settings#preparation".to_string(),
                "Resume meaning checks",
            ),
            EmptyState::Stopped(goal) => (
                "preparation-stopped",
                "Your request needs attention.",
                "Your input is saved, but preparation is not running. Open the request to see what stopped it and choose what to do next.",
                format!("/goals/{}", escape(&goal.id)),
                "Review your saved request",
            ),
        };
        format!(
            "{}<section class=\"empty-stage\" data-state=\"{state_name}\">{}<p class=\"eyebrow\">A private space for understanding</p><h1>{title}</h1><p class=\"lede\">{desc}</p><div class=\"actions\"><a class=\"button primary\" href=\"{destination}\">{action} <span aria-hidden=\"true\">→</span></a><a class=\"text-link\" href=\"/map\">Explore your map ↗</a></div><div class=\"notebook-margin\"><span>01 / Curiosity</span><span>Understanding starts with a question.</span></div></section>",
            self.pending(),
            emblem("large")
        )
    }
    fn nato_reference(&self) -> String {
        // A reference must never disclose an answer behind a protected prompt.
        if self.app.practice.protected() {
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
            .filter(|a| a.version_id == version.id && !a.disclosure && a.verdict().is_some())
            .collect::<Vec<_>>();
        let whole = attempts
            .iter()
            .filter(|a| matches!(a.prompt.scope, crate::study::Scope::Whole) && !a.assisted)
            .collect::<Vec<_>>();
        let correct = whole
            .iter()
            .filter(|a| a.verdict() == Some(crate::grading::Verdict::Good))
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
        let judged = a.and_then(|a| self.judged_state(a));
        let mut out = format!(
            "<section class=\"nato-screen lesson-screen\" data-state=\"lesson-home\"><header class=\"practice-heading\"><div><h1>{}</h1><p class=\"practice-edition\">{}</p></div><p class=\"lesson-position\">{}</p></header><div class=\"lesson-stage\" data-state=\"{}\" data-prompt-id=\"{}\"{}>",
            escape(&work.title),
            escape(&v.name),
            escape(&scope),
            if a.is_some() {
                judged.unwrap_or("exact-result")
            } else {
                "exact-prompt"
            },
            escape(&p.prompt.id),
            Self::judged_poll(judged)
        );
        if self.app.practice.mixed_review {
            out.push_str("<p class=\"eyebrow\">All subjects</p>");
        }
        if let Some(a) = a.filter(|_| judged.is_some()) {
            let retry = self.form(
                "/practice/lesson/next",
                "lesson-retry",
                "lesson-action",
                "data-next",
                &format!(
                    "{}{}<button class=\"button primary\">Try this phrase again</button>",
                    hidden("after", &p.prompt.id),
                    hidden("mode", "retry")
                ),
            );
            let (body, actions) = self.judged_result(a, &retry, |secondary| {
                let class = if secondary {
                    "text-button"
                } else {
                    "button primary"
                };
                let extra = if secondary { "" } else { "data-next" };
                if self.app.practice.mixed_review {
                    self.form(
                        "/practice/review/next",
                        "mixed-next",
                        "lesson-action",
                        extra,
                        &format!(
                            "{}<button class=\"{class}\">Next</button>",
                            hidden("after", &p.prompt.id)
                        ),
                    )
                } else {
                    self.form(
                        "/practice/lesson/next",
                        "lesson-next",
                        "lesson-action",
                        extra,
                        &format!(
                            "{}{}<button class=\"{class}\">{}</button>",
                            hidden("after", &p.prompt.id),
                            hidden("mode", if ordered { "whole" } else { "phrases" }),
                            if ordered {
                                "Try the sequence again"
                            } else if whole {
                                "Practise phrases"
                            } else if secondary {
                                "Continue to next phrase"
                            } else {
                                "Next phrase"
                            }
                        ),
                    )
                }
            });
            out.push_str(&body);
            out.push_str(&actions);
        } else if let Some(a) = a {
            let good = a.verdict() == Some(Verdict::Good);
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
                        "<p class=\"lesson-note\">{} of {} items matched · {} in order.</p><ol class=\"sequence-feedback\" aria-label=\"Expected order\">",
                        feedback.recalled,
                        all.len(),
                        feedback.in_order
                    );
                    for item in &feedback.items {
                        use crate::ordered::ItemStatus;
                        let (state, label) = match item.status {
                            ItemStatus::Correct => ("correct", "In order".to_string()),
                            ItemStatus::Misplaced => (
                                "misplaced",
                                format!("You put #{}", item.answered_positions[0]),
                            ),
                            ItemStatus::Missing => ("missing", "Not matched".to_string()),
                            ItemStatus::Repeated => (
                                "repeated",
                                format!("Repeated {} times", item.answered_positions.len()),
                            ),
                        };
                        let _ = write!(
                            out,
                            "<li data-item-status=\"{state}\"><span>{}</span><small>{label}</small></li>",
                            escape(all[item.unit_index].text.trim())
                        );
                    }
                    out.push_str("</ol>");
                    if !feedback.extras.is_empty() {
                        let lines = a
                            .raw_answer
                            .lines()
                            .filter(|line| !line.trim().is_empty())
                            .collect::<Vec<_>>();
                        out.push_str("<div class=\"sequence-extras\"><p>Entries to check</p><ul>");
                        for extra in &feedback.extras {
                            let label = match extra.kind {
                                crate::ordered::ExtraKind::Unknown => "Not recognized",
                                crate::ordered::ExtraKind::Repeated => "Repeated",
                            };
                            let _ = write!(
                                out,
                                "<li><span>{}. {}</span><small>{label}</small></li>",
                                extra.position,
                                escape(lines[extra.position - 1].trim())
                            );
                        }
                        out.push_str("</ul></div>");
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
            } else if a
                .grade
                .diff
                .iter()
                .any(|d| !matches!(d.change, crate::grading::Change::Correct { .. }))
            {
                // An accepted Slip is still shown.
                out.push_str(
                    "<ol class=\"exact-diff exact-changes\" aria-label=\"Words to revisit\">",
                );
                for d in a
                    .grade
                    .diff
                    .iter()
                    .filter(|d| !matches!(d.change, crate::grading::Change::Correct { .. }))
                {
                    out.push_str(&Self::exact_diff_word(d, true, a.grade.units.len() > 1));
                }
                out.push_str("</ol>");
            }
            if let Some(hint) = a.grade.wrong_version_hint.as_ref().filter(|_| !ordered) {
                let _ = write!(
                    out,
                    "<p class=\"notice\">Your answer is closer to Version {}.</p>",
                    escape(&hint.name)
                );
            }
            if a.assisted {
                out.push_str("<p class=\"lesson-note\">With help · schedule unchanged.</p>");
            }
            out.push_str(&self.result_progress(a));
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
            if self.app.practice.mixed_review {
                out.push_str(&self.form(
                    "/practice/review/next",
                    "mixed-next",
                    "lesson-action",
                    if phrase && !good { "" } else { "data-next" },
                    &format!(
                        "{}<button class=\"{}\">Next</button>",
                        hidden("after", &p.prompt.id),
                        if phrase && !good {
                            "text-button"
                        } else {
                            "button primary"
                        }
                    ),
                ));
            } else {
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
            if ordered {
                let _ = write!(
                    out,
                    "<details class=\"lesson-answer\"><summary>Your submitted answer</summary><p class=\"reading\">{}</p></details>",
                    escape(&a.raw_answer)
                );
            } else if good {
                let _ = write!(
                    out,
                    "<details class=\"lesson-answer\"><summary>Compare the text</summary><h2>Your answer</h2><p class=\"reading\">{}</p><h2>Selected wording</h2>{}<h2>Word by word</h2><ol class=\"exact-diff\" aria-label=\"Diff\">",
                    escape(&a.raw_answer),
                    self.exact_expected()
                );
                for d in &a.grade.diff {
                    out.push_str(&Self::exact_diff_word(d, false, a.grade.units.len() > 1));
                }
                out.push_str("</ol></details>");
            }
        } else if p.source_visible() {
            out.push_str(&self.source_study(p));
        } else {
            // An introduction is never labelled as review.
            let _ = write!(
                out,
                "<p class=\"eyebrow\" data-study-phase=\"quiz\">{}</p>",
                if self.due_review(p) { "Review" } else { "Quiz" }
            );
            if ordered {
                let _ = write!(
                    out,
                    "<p class=\"lesson-instruction\">Recall all {} items in order, one per line.</p>",
                    all.len()
                );
            } else if whole {
                out.push_str("<p class=\"lesson-instruction\">Type the full text from memory.</p>");
            } else if let Scope::Section { section_id } = &p.prompt.scope {
                out.push_str(
                    "<p class=\"lesson-instruction\">Join these phrases from memory, in order.</p>",
                );
                // A Section Recitation is prompted by its heading.
                if let Some(section) = v
                    .sections
                    .iter()
                    .find(|s| &s.id == section_id && !s.heading.trim().is_empty())
                {
                    let _ = write!(
                        out,
                        "<p class=\"lesson-cue\">{}</p>",
                        escape(&section.heading)
                    );
                }
            } else {
                out.push_str(&self.previous_phrase(p));
                out.push_str("<p class=\"lesson-instruction\">Type this phrase from memory</p>");
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
            if !p.revealed || p.phase == Some(crate::study::StudyPhase::Quiz) {
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
        let multi = a.card_units().len() > 1;
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
            return self.rest("All subjects", |_| true, "");
        }
        out.push_str("</div>");
        out.push_str(&self.browse());
        out.push_str("</section>");
        out
    }
    fn browse(&self) -> String {
        let mut out = self.lesson_picker(false).replacen(
            "<summary><span>Lessons</span>",
            "<summary><span>Browse</span>",
            1,
        );
        // Browse opens the Library as well as the lesson shelf.
        out.insert_str(
            out.len() - "</details>".len(),
            "<a class=\"text-link\" href=\"/library\">Library</a>",
        );
        out
    }
    /// The earliest future review among Studies that still serve reviews.
    fn next_review(&self, in_scope: impl Fn(&crate::study::Study) -> bool) -> Option<i64> {
        self.app
            .practice
            .cards
            .values()
            .filter(|c| {
                c.state.schedule.due_ms > self.now
                    && self.app.practice.studies.get(&c.study_id).is_some_and(|s| {
                        s.status != crate::study::Status::Dropped
                            && crate::study::reviewable(&self.app.library.versions[&s.version_id])
                                .is_ok()
                            && in_scope(s)
                    })
            })
            .map(|c| c.state.schedule.due_ms)
            .min()
    }
    /// The one nothing-due state, under Study and inside a focused lesson.
    /// `tools` are the lesson's own disclosures, shown after Browse. While an
    /// answer in scope is still with Jev there is nothing to rest from yet: the
    /// page says the answer is being checked and looks again by itself.
    fn rest(
        &self,
        context: &str,
        in_scope: impl Fn(&crate::study::Study) -> bool,
        tools: &str,
    ) -> String {
        if let Some(a) = crate::study::checking_attempt(self.app, &in_scope) {
            return format!(
                "<section class=\"nato-screen study-home\" data-state=\"exact-checking\" data-poll><div class=\"study-entry\"><p class=\"study-context\">{}</p><div class=\"checking\" role=\"status\"><p class=\"eyebrow\">Your answer is saved</p><h1>Checking it.</h1><p>Jev is judging your answer. Nothing else is due until it has a Verdict.</p></div><div class=\"answer-pair\"><span>You answered</span><p>{}</p></div><a class=\"text-link\" href=\"{}\">Refresh</a> <a class=\"text-link\" href=\"/history\">History</a></div>{}{tools}</section>",
                escape(context),
                escape(&a.raw_answer),
                escape(self.path),
                self.browse()
            );
        }
        let next_review = self.next_review(&in_scope);
        let mut out = format!(
            "<section class=\"nato-screen study-home\" data-state=\"rest\"><div class=\"study-entry\"><p class=\"study-context\">{}</p><h1>Rest for now</h1>",
            escape(context)
        );
        if let Some(at) = next_review {
            let _ = write!(
                out,
                "<p class=\"study-context\" data-next-review>{}</p>",
                escape(&next_review_line(at))
            );
        }
        out.push_str("</div>");
        out.push_str(&self.browse());
        out.push_str(tools);
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
            let letter = self.mapping_question(prompt).unwrap_or(&prompt.cue);
            let _ = write!(
                out,
                "<div class=\"nato-stage\" data-state=\"{state}\" data-prompt-id=\"{}\"><div class=\"nato-cue-panel\"><h1 class=\"nato-letter\" id=\"nato-letter\" aria-label=\"Letter {}\">{}</h1></div><div class=\"nato-response-panel\">",
                escape(&prompt.id),
                escape(letter),
                escape(letter)
            );
            if let Some(a) = attempt {
                let good = a.verdict() == Some(crate::grading::Verdict::Good);
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
            if !other_prompt {
                out.push_str(
                    "<div class=\"nato-specimen\" aria-hidden=\"true\">Aa<span>Alfa</span></div>",
                );
            }
            if other_prompt {
                out.push_str("<h1>Your review is waiting.</h1><p>Finish your saved question before starting NATO.</p><a class=\"button primary\" href=\"/recall\">Resume review</a>");
            } else if has_study && self.query.get("practice").is_some_and(|v| v == "nato") {
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
                    // Only answered letters that are due again are review.
                    out.push_str(
                        if self.review_due(|s| nato_versions.contains(s.version_id.as_str())) {
                            "<h1>Ready to review.</h1>"
                        } else {
                            "<h1>A little, often.</h1>"
                        },
                    );
                    out.push_str(&self.form(
                        "/practice/nato/start",
                        "nato-continue",
                        "nato-action",
                        "",
                        "<button class=\"button primary\">Continue practice</button>",
                    ));
                } else {
                    return self.rest(
                        "NATO alphabet",
                        |s| nato_versions.contains(s.version_id.as_str()),
                        &format!(
                            "<div class=\"practice-tools\">{}{}</div>",
                            self.nato_progress(),
                            self.nato_reference()
                        ),
                    );
                }
            } else {
                let (title, desc, action) = if has_study {
                    ("A little, often.", "", "Continue practice")
                } else {
                    (
                        "From Alfa to Zulu.",
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
        // Live Goals first, as before; archived ones after them, labelled.
        let mut goals: Vec<_> = self.app.goals.values().collect();
        goals.sort_by_key(|g| (g.archived, g.paused, !g.focused, g.created_ms));
        let mut out = String::from(
            "<section class=\"map-page\" data-state=\"map\"><div class=\"page-heading\"><p class=\"eyebrow\">Earlier concept notebook</p><h1>A map of ideas.</h1><p class=\"lede\">Every Goal and Concept saved before Study, as it was left.</p></div>",
        );
        if goals.is_empty() {
            out.push_str(
                "<p class=\"quiet-empty\">Nothing is saved in the earlier concept notebook.</p>",
            );
        }
        for (index, goal) in goals.iter().enumerate() {
            let all: Vec<_> = goal
                .concept_ids
                .iter()
                .filter_map(|id| self.app.concepts.get(id))
                .collect();
            let live: Vec<_> = all.iter().copied().filter(|c| !c.archived).collect();
            let _ = write!(
                out,
                "<article class=\"goal-map {}\"><div class=\"goal-map-heading\"><span class=\"folio\">{:02}</span><div><h2><a href=\"/goals/{}\">{}</a></h2><p class=\"goal-meta\">{} ideas{}{}{} · {}</p></div></div>",
                if goal.paused || goal.archived {
                    "paused"
                } else {
                    ""
                },
                index + 1,
                escape(&goal.id),
                self.goal_title(goal),
                live.len(),
                if goal.focused { " · In focus" } else { "" },
                if goal.paused { " · Paused" } else { "" },
                if goal.archived { " · Archived" } else { "" },
                status_label(self.goal_phase(goal))
            );
            if !live.is_empty() {
                out.push_str(&constellation(&live));
            }
            out.push_str("<ol class=\"concept-list\">");
            for c in live.iter().chain(all.iter().filter(|c| c.archived)) {
                let _ = write!(
                    out,
                    "<li><a href=\"/concepts/{}\"><span class=\"idea-dot\" aria-hidden=\"true\"></span><span>{}</span><span class=\"idea-status\">{}{}</span><span aria-hidden=\"true\">↗</span></a></li>",
                    escape(&c.id),
                    self.concept_name(c),
                    if c.archived { "Archived · " } else { "" },
                    answers_recorded(self.app, c)
                );
            }
            out.push_str("</ol><div class=\"goal-actions\">");
            let _ = write!(
                out,
                "<a class=\"text-link\" href=\"/goals/{}\">Reference &amp; input ↗</a></div></article>",
                escape(&goal.id)
            );
        }
        out.push_str("</section>");
        out
    }
    fn goal(&self, id: &str) -> String {
        let Some(g) = self.app.goals.get(id) else {
            return self.not_found();
        };
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
            let concepts: Vec<_> = g
                .concept_ids
                .iter()
                .filter_map(|id| self.app.concepts.get(id))
                .collect();
            // Archived Concepts follow the live ones, labelled.
            for c in concepts
                .iter()
                .filter(|c| !c.archived)
                .chain(concepts.iter().filter(|c| c.archived))
            {
                let _ = write!(
                    out,
                    "<section class=\"reference-idea\"><h2><a href=\"/concepts/{}\">{}</a></h2>{}{}{}<a class=\"text-link\" href=\"/concepts/{}\">Explore this idea ↗</a></section>",
                    escape(&c.id),
                    self.concept_name(c),
                    if c.archived {
                        "<p class=\"fineprint\">Archived</p>"
                    } else {
                        ""
                    },
                    self.lede("lede small", c),
                    self.notes(c),
                    escape(&c.id)
                );
            }
            out.push_str("</section>");
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
        if g.photo.is_some() {
            let _ = write!(
                out,
                "<a class=\"text-link\" href=\"/photos/{}\">The photo you sent ↗</a>",
                escape(&g.id)
            );
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
        let state = concept_evidence(self.app, c);
        let unaided = state.unaided;
        let helped = state.helped;
        let missed = state.missed;
        let mut out = format!(
            "<article class=\"concept-page\" data-state=\"concept\"><a class=\"back-link\" href=\"/map\">← Your map</a><div class=\"page-heading\"><h1>{}</h1>{}{}</div><section class=\"concept-study\"><h2>Notes</h2>{}</section><section class=\"practice-evidence\"><h2>Recorded answers</h2><p><strong>{unaided}</strong> recalled <span>·</span> <strong>{helped}</strong> with help <span>·</span> <strong>{missed}</strong> missed</p><p class=\"fineprint\">Graded answers to its saved questions, as recorded. History lists every answer.</p>{}</section>{}",
            self.concept_name(c),
            self.lede("lede", c),
            if c.already_known {
                "<p class=\"fineprint\">Marked as already known.</p>"
            } else {
                ""
            },
            all_notes(self.app, c),
            if self.active_concept(c) {
                ""
            } else {
                "<p>Archived: retained for reference.</p>"
            },
            tally(&state)
        );
        out.push_str("<section class=\"saved-questions\"><h3>Saved questions</h3>");
        let questions: Vec<_> = self
            .app
            .questions
            .values()
            .filter(|q| q.concept_id == id)
            .collect();
        // Archived questions follow the live ones, labelled.
        for q in questions
            .iter()
            .filter(|q| !q.archived)
            .chain(questions.iter().filter(|q| q.archived))
        {
            let content = q.content();
            let _ = write!(
                out,
                "<details class=\"question-reference\"><summary>{}{}</summary><div class=\"reading\"><p><strong>{}</strong></p><p>{}</p></div>",
                escape(&content.prompt),
                if q.archived {
                    " <span class=\"fineprint\">Archived</span>"
                } else {
                    ""
                },
                escape(&content.answer),
                escape(&content.explanation)
            );
            let _ = write!(
                out,
                "<a class=\"text-link\" href=\"/questions/{}/edit\">The saved question ↗</a></details>",
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
        // Links to archived Concepts stay, labelled, after the live ones.
        let mut dependents: Vec<_> = self
            .app
            .concepts
            .values()
            .filter(|other| other.prerequisites.contains(&c.id))
            .collect();
        dependents.sort_by_key(|p| !self.active_concept(p));
        let mut prerequisites: Vec<_> = c
            .prerequisites
            .iter()
            .filter_map(|id| self.app.concepts.get(id))
            .collect();
        prerequisites.sort_by_key(|p| !self.active_concept(p));
        if !prerequisites.is_empty() || !dependents.is_empty() {
            out.push_str("<section class=\"connected\"><h2>Related</h2>");
            if !prerequisites.is_empty() {
                out.push_str("<h3>Builds on</h3>");
                for p in prerequisites {
                    out.push_str(&self.related_chip(p));
                }
            }
            if !dependents.is_empty() {
                out.push_str("<h3>Used by</h3>");
                for p in dependents {
                    out.push_str(&self.related_chip(p));
                }
            }
            out.push_str("</section>");
        }
        out.push_str(&self.feedback_history(&c.goal_id, Some(id)));
        out.push_str("</article>");
        out
    }
    fn related_chip(&self, p: &Concept) -> String {
        format!(
            "<a class=\"concept-chip\" href=\"/concepts/{}\">{}</a>{}",
            escape(&p.id),
            self.concept_name(p),
            if self.active_concept(p) {
                ""
            } else {
                "<span class=\"fineprint related-archived\">Archived</span>"
            }
        )
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
            let mut attempts = self.app.practice.attempts.values().collect::<Vec<_>>();
            attempts.sort_by_key(|a| std::cmp::Reverse(a.sequence));
            for attempt in attempts {
                out.push_str(&self.exact_history_attempt(attempt));
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
        let judged = crate::study::judgment_view(self.app, a);
        let label = if a.disclosure {
            "Shown before answering"
        } else {
            match (a.verdict(), &judged) {
                (Some(v), _) => verdict(v),
                (None, Some(view)) if view.state == crate::study::JudgmentState::Checking => {
                    "Checking"
                }
                (None, _) => "Saved, not graded",
            }
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
        // Where Jev was asked, its judgment comes first and the comparison
        // beneath is labelled as a comparison.
        if let Some(view) = &judged {
            match (view.decided, view.state) {
                (Some(decided), _) => {
                    let _ = write!(
                        out,
                        "<p class=\"fineprint exact-authority\" data-authority=\"jev\">{} · {}</p>",
                        Self::judged_by(decided),
                        date(decided.decided_ms)
                    );
                }
                (None, crate::study::JudgmentState::Checking) => {
                    out.push_str("<p class=\"fineprint\">Jev is judging this answer. It has no Verdict yet.</p>");
                }
                (None, _) => {
                    let _ = write!(
                        out,
                        "<p class=\"notice\">{}</p><p class=\"fineprint\">This answer has no Verdict and its schedule is unchanged.</p>",
                        escape(
                            view.check
                                .and_then(|c| c.reason.as_deref())
                                .unwrap_or("Jev did not judge this answer.")
                        )
                    );
                    if view.checks_left {
                        out.push_str(&self.check_again(a, "exact-history-check"));
                    } else {
                        out.push_str("<p class=\"fineprint\">Three checks are saved for this answer. It stays saved and not graded.</p>");
                    }
                }
            }
        }
        let _ = write!(
            out,
            "<h3>{}</h3><ol class=\"exact-diff\" aria-label=\"Diff\">",
            if a.disclosure {
                "Saved comparison · no answer submitted"
            } else if judged.is_some() {
                "Word-by-word comparison, not the Verdict"
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
            } else if judged.is_some() {
                " · comparison only, not the Verdict"
            } else {
                ""
            }
        );
        for e in &a.effects {
            let (kind, status) = if e.superseded {
                (
                    "superseded",
                    "A newer answer was already recorded · schedule unchanged".into(),
                )
            } else if e.assisted {
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
    /// The Owner's switch for paid judgment. It says what leaves Scry and what
    /// each check reserves before it offers anything, and offers no control
    /// where this Scry cannot reach Jev. A page rendered without the runtime's
    /// answer to that question shows no section at all.
    fn paid_judgment(&self) -> String {
        let Some(capable) = self.query.get("__judgment") else {
            return String::new();
        };
        let reserve = RESERVATION as f64 / 1_000_000.0;
        let allowance = DAILY_ALLOWANCE as f64 / 1_000_000.0;
        let on = self.app.preferences.paid_judgment;
        let explain = format!(
            "<p>When this is on, an answer that is not an exact match is saved and then sent to Jev, a paid judgment service, to be judged. Your answer and the expected text leave Scry for that check. So does what names the question: the Work's title and the place in it, a mapping's key, and the accepted aliases of a list's items. Each check reserves ${reserve:.2} of the ${allowance:.2} daily allowance before it is sent. Spelling-strict content, empty answers and exact matches are never sent.</p>"
        );
        let switch = |value: &str, label: &str, class: &str| {
            self.form(
                "/settings/judgment",
                "paid-judgment",
                "judgment-form",
                "",
                &format!(
                    "{}<button class=\"button {class}\">{label}</button>",
                    hidden("paid_judgment", value)
                ),
            )
        };
        let body = if capable != "true" {
            format!(
                "<p data-paid-judgment=\"unavailable\"><strong>Paid judgment is not available here.</strong> Nothing leaves Scry: every answer is graded by exact comparison.</p>{}",
                if on {
                    "<p class=\"fineprint\">It is turned on in this notebook and stays unused here.</p>"
                } else {
                    ""
                }
            )
        } else if on {
            format!(
                "<p data-paid-judgment=\"on\"><strong>Paid judgment is on.</strong></p>{explain}{}",
                switch("off", "Turn paid judgment off", "secondary")
            )
        } else {
            format!(
                "<p data-paid-judgment=\"off\"><strong>Paid judgment is off.</strong> Nothing leaves Scry: every answer is graded by exact comparison.</p>{explain}{}",
                switch("on", "Turn paid judgment on", "primary")
            )
        };
        format!(
            "<section class=\"settings-section\" id=\"paid-judgment\"><h2>Paid judgment</h2>{body}</section>"
        )
    }
    fn settings(&self) -> String {
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
                "<section class=\"notice\" id=\"preparation\"><h2>Preparation is paused.</h2><p>This restored notebook has background work paused. Resume future meaning checks when you are ready. Native content generation stays retired. Historical uncertain work stays stopped; its allowance and your learning history stay saved.</p>{}</section>",
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
            "<section class=\"settings-page\" data-state=\"settings\"><div class=\"page-heading\"><h1>Settings</h1></div>{resume}<details class=\"settings-section\"><summary>Usage allowance</summary><p class=\"spend-number\">${spent:.2}<span> / $3.50</span></p><p>Used or reserved in the last 24 hours. Each external meaning check reserves $0.50 before it begins. Historical generation usage remains accounted; uncertain outcomes keep their reservation.</p><p class=\"fineprint\">{unknown} reservations without confirmed final usage. Provider limits apply independently.</p></details><section class=\"settings-section\"><h2>Backup</h2><p>{backup}</p><details><summary>Recovery targets</summary><p class=\"fineprint\">Daily remote snapshots and 30-day retention target 24 hours of recoverable work and a 60-minute recovery. A target is not a guarantee.</p></details>{}{backup_action}</section><section class=\"settings-section\"><h2>Export</h2><a class=\"button secondary\" href=\"/export\" download>Export your notebook ↓</a></section><details class=\"settings-section font-colophon\"><summary>About</summary><p>Literata and Atkinson Hyperlegible Next.</p><a href=\"/assets/fonts/OFL.txt\">Font licenses</a></details></section>",
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
        out.push_str(&self.paid_judgment());
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
    /// A saved question's whole record, read-only: its current wording,
    /// every earlier version and any unapplied fix draft, then every answer
    /// recorded to it with its Verdict, Authority, Corrections and checks, and
    /// any schedule reset. The URL is the one the edit form had.
    fn question_page(&self, id: &str) -> String {
        let Some(q) = self.app.questions.get(id) else {
            return self.not_found();
        };
        let c = q.content();
        let mut out = format!(
            "<article class=\"study\" data-state=\"earlier-question\"><a class=\"back-link\" href=\"/concepts/{}\">← Back to the idea</a><p class=\"eyebrow\">A saved question</p><h1 class=\"question\">{}</h1><section class=\"question-current\">{}</section><p class=\"fineprint\">Version {} · Saved {}{}</p>{}",
            escape(&q.concept_id),
            escape(&c.prompt),
            question_content(c, false),
            c.version,
            date(c.created_ms),
            if q.archived { " · Archived" } else { "" },
            self.fix_status(q)
        );
        if let Some(draft) = &q.draft {
            let _ = write!(
                out,
                "<details class=\"question-record\" id=\"draft\"><summary>Fix draft, saved and never applied</summary><p class=\"fineprint\">Drafted for version {} · Saved {}</p>{}</details>",
                q.draft_for_version
                    .map_or_else(|| "unknown".to_string(), |v| v.to_string()),
                date(draft.created_ms),
                question_content(draft, true)
            );
        }
        for v in q.versions.iter().rev().skip(1) {
            let _ = write!(
                out,
                "<details class=\"question-record\" id=\"version-{}\"><summary>Version {} · Saved {}</summary>{}</details>",
                v.version,
                v.version,
                date(v.created_ms),
                question_content(v, true)
            );
        }
        out.push_str("<section class=\"question-answers\"><h2>Recorded answers</h2>");
        let events: Vec<_> = self
            .app
            .events
            .iter()
            .filter(|e| e.question_id == q.id)
            .collect();
        let saved: Vec<_> = unrecorded(self.app)
            .into_iter()
            .filter(|u| u.question_id == q.id)
            .collect();
        if events.is_empty() && saved.is_empty() {
            out.push_str("<p class=\"fineprint\">No answer was recorded.</p>");
        }
        for e in events.iter().rev() {
            out.push_str(&self.recorded_answer(e));
        }
        for u in saved.iter().rev() {
            let _ = write!(
                out,
                "<article class=\"record-entry\" id=\"saved-{}\"><p class=\"history-meta\"><span>Ungraded</span> · {} · Version {}</p><p class=\"record-answer\">{}</p>{}</article>",
                escape(u.occurrence_id),
                date(u.at),
                u.version,
                escape(u.answer),
                checks(&u.checks, u.answer)
            );
        }
        out.push_str("</section>");
        let resets: Vec<_> = self
            .app
            .resets
            .iter()
            .filter(|r| r.question_id == q.id)
            .collect();
        if !resets.is_empty() {
            out.push_str("<section class=\"question-answers\"><h2>Schedule resets</h2>");
            for r in resets {
                let cause = self
                    .app
                    .feedback
                    .iter()
                    .find(|f| f.id == r.feedback_id)
                    .map(|f| {
                        format!(
                            "<p class=\"reading preserve\" id=\"feedback-{}\">{}</p>",
                            escape(&f.id),
                            escape(&f.text)
                        )
                    })
                    .unwrap_or_default();
                let _ = write!(
                    out,
                    "<article class=\"record-entry\" id=\"reset-{}\"><p class=\"history-meta\"><span>Schedule reset</span> · {} · Version {}</p>{cause}</article>",
                    escape(&r.id),
                    date(r.created_ms),
                    r.content_version
                );
            }
            out.push_str("</section>");
        }
        out.push_str("</article>");
        out
    }
    /// One Event on its question's page: the Verdict and Authority as saved,
    /// the version it was given to, then any Correction and check.
    fn recorded_answer(&self, e: &Event) -> String {
        let outcome = if e.is_ungraded() {
            "Ungraded"
        } else if e.assisted {
            "With help"
        } else {
            outcome_label(&e.result.outcome)
        };
        let mut out = format!(
            "<article class=\"record-entry\" id=\"attempt-{}\"><p class=\"history-meta\"><span>{outcome}</span> · {} · {} · Version {}</p><p class=\"record-answer\">{}</p>",
            escape(&e.id),
            date(e.result.reviewed_ms),
            authority_label(&e.result.authority),
            e.presentation.version,
            escape(if e.answer.is_empty() {
                "You looked at the answer"
            } else {
                &e.answer
            })
        );
        for o in self.app.overrides.iter().filter(|o| o.event_id == e.id) {
            let _ = write!(
                out,
                "<p class=\"record-correction\" id=\"correction-{}\">Your Correction: counted as {} · {}</p>",
                escape(&o.id),
                if o.correct { "recalled" } else { "missed" },
                date(o.created_ms)
            );
        }
        let mut saved: Vec<_> = self
            .app
            .assessments
            .values()
            .filter(|a| a.occurrence_id == e.occurrence_id)
            .collect();
        saved.sort_by_key(|a| a.created_ms);
        out.push_str(&checks(&saved, &e.answer));
        out.push_str("</article>");
        out
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
    shell("Changelog", "/changelog", &content, None, false, "/")
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
        || path
            .strip_prefix("/library/versions/")
            .or_else(|| path.strip_prefix("/library/works/"))
            .is_some_and(|id| {
                id.len() == 32
                    && id
                        .bytes()
                        .all(|b| b.is_ascii_hexdigit() && !b.is_ascii_uppercase())
            })
}
fn rights_label(r: crate::library::Rights) -> &'static str {
    match r {
        crate::library::Rights::PrivateOnly => "Private only",
        crate::library::Rights::Licensed => "Licensed",
        crate::library::Rights::PublicDomain => "Public domain",
    }
}
fn provenance_label(p: crate::library::Provenance) -> &'static str {
    match p {
        crate::library::Provenance::Supplied => "Supplied text",
        crate::library::Provenance::ModelFromMemory => "Written by a model from memory",
        crate::library::Provenance::CheckedAgainstSources => "Checked against cited sources",
    }
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
    let count = |n: usize, one: &str| format!("{n} {one}{}", if n == 1 { "" } else { "s" });
    format!(
        "{kind} · {} · {}",
        count(v.sections.len(), "Section"),
        count(v.sections.iter().map(|s| s.units.len()).sum(), "Unit")
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

pub fn mutation_redirect<'a>(app: &App, path: &str, redirect: &'a str) -> &'a str {
    if matches!(path, "/library/version/add" | "/library/mapping/add") {
        redirect.strip_prefix("/api/v1").unwrap_or("/library")
    } else if path == "/practice/pace" {
        // A Study's Pace is chosen on its Version's page and returns there.
        redirect
    } else if path == "/practice/check" && redirect == "/history" {
        // An earlier answer was checked again from History: return there.
        "/history"
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
    } else if path == "/settings/resume" && redirect == "/" {
        // Resuming is done from Settings and returns there.
        "/settings"
    } else if redirect == "/"
        && (path.starts_with("/review/")
            || path
                .strip_prefix("/concepts/")
                .is_some_and(|p| p.ends_with("/practice")))
    {
        // A retired earlier-notebook action is refused on the page it came from.
        "/notebook"
    } else {
        redirect
    }
}

pub fn mutation_error_path<'a>(app: &App, path: &'a str) -> &'a str {
    if path == "/library/mapping/add" {
        "/library"
    } else if path == "/library/version/add" {
        "/library/add"
    } else if matches!(path, "/create" | "/add" | "/settings") || path.ends_with("/edit") {
        path
    } else if let Some(page) = earlier_page(path) {
        page
    } else {
        mutation_redirect(app, path, "/")
    }
}

/// A refused write to an earlier Goal or Concept is shown on that Goal's or
/// Concept's own page; any other refused earlier-notebook write, on Earlier
/// practice. Retired generation actions keep their own landing.
fn earlier_page(path: &str) -> Option<&str> {
    let parts: Vec<_> = path.trim_matches('/').split('/').collect();
    match parts.as_slice() {
        [_, _, "retry" | "refine" | "fix"] => None,
        ["concepts", _, "practice"] => Some("/notebook"),
        ["goals" | "concepts", _, _] => path.rfind('/').map(|end| &path[..end]),
        ["questions", _, _] => Some("/notebook"),
        _ if path == "/__fixture" => Some("/notebook"),
        _ => None,
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
                let study = &app.practice.studies[&p.prompt.study_id];
                // A text that Study's mix reached first offers lesson controls
                // only once every phrase has a Card; until then, outside the
                // mix, it is ordinary Study practice at its Pace.
                study.lesson
                    && !crate::library::is_nato(&app.library.versions[&p.prompt.version_id])
                    && (app.practice.mixed_review || crate::study::fully_introduced(app, study))
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
        // Opening practice with nothing held selects nothing: it offers Study.
        "/recall"
            if app.practice.current.is_none()
                && (app.practice.mixed_review || exact_stream_active(app)) =>
        {
            view.mixed_home()
        }
        "/recall" if exact_stream_active(app) => view.exact_stream(),
        "/recall" | "/library" => view.library(),
        "/library/add" => view.library_add(),
        "/notebook" => format!(
            "<nav class=\"understanding-nav\" aria-label=\"Understanding\"><a href=\"/\">Review</a><a href=\"/map\">Map</a></nav>{}",
            view.earlier()
        ),
        "/create" | "/add" => view.create(),
        "/map" => view.map(),
        "/history" => view.history(),
        "/settings" => view.settings(),
        _ => {
            let segments: Vec<_> = path.trim_matches('/').split('/').collect();
            match segments.as_slice() {
                ["library", "versions", id] => view.library_version(id),
                ["library", "works", id] => view.library_work(id),
                ["goals", id] => view.goal(id),
                ["concepts", id] => view.concept(id),
                ["questions", id, "edit"] => view.question_page(id),
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
    shell(
        title,
        path,
        &content,
        notice,
        query.get("__synthetic").is_some_and(|v| v == "true"),
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
    practice_href: &str,
) -> String {
    let notice = notice
        .map(|n| format!("<div class=\"notice\" role=\"status\">{}</div>", escape(n)))
        .unwrap_or_default();
    format!(
        "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1, viewport-fit=cover, interactive-widget=resizes-content\"><meta name=\"color-scheme\" content=\"light dark\"><meta name=\"theme-color\" media=\"(prefers-color-scheme: light)\" content=\"#f5f2ea\"><meta name=\"theme-color\" media=\"(prefers-color-scheme: dark)\" content=\"#191e1d\"><meta name=\"robots\" content=\"noindex,nofollow\"><title>{} · Scry</title><link rel=\"icon\" href=\"/assets/icon.svg\" type=\"image/svg+xml\"><link rel=\"stylesheet\" href=\"/assets/app.css\"><script src=\"/assets/app.js\" defer></script></head><body><a class=\"skip-link\" href=\"#main\">Skip to the page</a><div class=\"page-frame\"><header class=\"masthead\"><a class=\"wordmark\" href=\"/\" aria-label=\"Scry, home\">{}<span>scry<span class=\"wordmark-period\">.</span></span></a><details class=\"account-menu\"><summary aria-label=\"Open menu\">Menu</summary><nav aria-label=\"Main navigation\"><a href=\"{practice_href}\" {}>Practice</a><a href=\"/lessons\">Lessons</a><a href=\"/library\">Library</a><a href=\"/history\">History</a><a href=\"/settings\">Settings</a><a href=\"/notebook\">Earlier practice</a><a href=\"/changelog\">Changelog</a></nav></details></header>{}<div id=\"request-status\" class=\"request-status\" role=\"status\" aria-live=\"polite\" hidden></div><main id=\"main\" tabindex=\"-1\" >{notice}{content}</main></div></body></html>",
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
        }
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
/// A Concept's latest note, as the Goal page summarizes it.
fn notes(app: &App, c: &Concept) -> String {
    c.notes.last().map_or_else(
        || "<p class=\"fineprint\">No note is available yet.</p>".into(),
        |note| note_section(app, c, note, false),
    )
}
/// Every note of a Concept, in the order saved, each with its provenance.
fn all_notes(app: &App, c: &Concept) -> String {
    if c.notes.len() < 2 {
        return notes(app, c);
    }
    c.notes
        .iter()
        .map(|note| note_section(app, c, note, true))
        .collect()
}
fn note_section(app: &App, c: &Concept, note: &Note, dated: bool) -> String {
    let paragraphs = note_text(app, c, &note.text)
        .split("\n\n")
        .filter(|p| !p.trim().is_empty())
        .map(|p| format!("<p>{}</p>", escape(p)))
        .collect::<String>();
    let saved = if dated {
        format!("<p class=\"fineprint\">Saved {}</p>", date(note.created_ms))
    } else {
        String::new()
    };
    format!(
        "<section class=\"concept-note\" aria-label=\"Saved explanation\">{saved}<div class=\"reading preserve\">{paragraphs}</div>{}</section>",
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
/// Decorative: prerequisites place the stars; every star is drawn alike, since
/// nothing about the earlier notebook is predicted.
fn constellation(concepts: &[&Concept]) -> String {
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
    for (i, &(x, y)) in positions.iter().enumerate() {
        let _ = write!(
            out,
            "<circle class=\"chart-halo\" cx=\"{x}\" cy=\"{y}\" r=\"11\"/><circle class=\"chart-star\" cx=\"{x}\" cy=\"{y}\" r=\"3.35\"/><text class=\"chart-number\" x=\"{}\" y=\"{}\">{:02}</text>",
            x + 13,
            y - 10,
            i + 1
        );
    }
    out.push_str("<circle class=\"chart-distant\" cx=\"596\" cy=\"31\" r=\"1\"/><circle class=\"chart-distant\" cx=\"203\" cy=\"153\" r=\"1\"/><circle class=\"chart-distant\" cx=\"510\" cy=\"142\" r=\"1\"/></svg>");
    out
}
fn concept_evidence(app: &App, c: &Concept) -> ConceptState {
    let attempts: Vec<_> = app
        .questions
        .values()
        .filter(|q| q.concept_id == c.id && !q.archived)
        .flat_map(|q| {
            app.events
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
        })
        .collect();
    compute_concept_state(&attempts)
}
/// How many answers to this Concept were recorded, graded or not: an observed
/// count, never an estimate.
fn answers_recorded(app: &App, c: &Concept) -> String {
    match app.events.iter().filter(|e| e.concept_id == c.id).count() {
        0 => "No answers".into(),
        1 => "1 answer".into(),
        n => format!("{n} answers"),
    }
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
            "h" => ("helped", "Answered with help"),
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
/// A saved question version as it was presented: choices, the answer and
/// what else was accepted, the explanation and its provenance. `wording`
/// repeats the question when it is not the page's heading.
fn question_content(c: &Content, wording: bool) -> String {
    let mut out = String::new();
    if wording {
        let _ = write!(
            out,
            "<p class=\"reading\"><strong>{}</strong></p>",
            escape(&c.prompt)
        );
    }
    if !c.choices.is_empty() {
        out.push_str("<ol class=\"reading earlier-choices\">");
        for choice in &c.choices {
            let _ = write!(out, "<li>{}</li>", escape(choice));
        }
        out.push_str("</ol>");
    }
    let _ = write!(
        out,
        "<dl class=\"answer-pair\"><div><dt>The answer</dt><dd>{}</dd></div>{}</dl><div class=\"reading explanation\"><p>{}</p></div>{}",
        escape(&c.answer),
        if c.variants.is_empty() {
            String::new()
        } else {
            format!(
                "<div><dt>Also accepted</dt><dd>{}</dd></div>",
                escape(&c.variants.join("\n"))
            )
        },
        escape(&c.explanation),
        provenance(&c.basis, &c.quotes)
    );
    out
}
/// Saved checks of one answer, oldest first, each by its fixed status label
/// and the policy it ran under. An answer that differs from the one shown
/// above it is shown with its check.
fn checks(saved: &[&Assessment], answer: &str) -> String {
    let mut out = String::new();
    for a in saved {
        let (name, label) = check_label(&a.status);
        let _ = write!(
            out,
            "<p class=\"fineprint\" id=\"check-{}\" data-check=\"{name}\">{label} Policy {} · {}</p>",
            escape(&a.id),
            escape(&a.policy),
            date(a.created_ms)
        );
        if a.answer != answer {
            let _ = write!(
                out,
                "<p class=\"reading preserve\">{}</p>",
                escape(&a.answer)
            );
        }
    }
    out
}
/// An answer saved in the earlier notebook with no Event: the held
/// question's answer, or the answer a check was recorded for. Ungraded.
struct Unrecorded<'a> {
    occurrence_id: &'a str,
    question_id: &'a str,
    version: u64,
    answer: &'a str,
    at: i64,
    checks: Vec<&'a Assessment>,
}
fn unrecorded(app: &App) -> Vec<Unrecorded<'_>> {
    let graded = |id: &str| app.events.iter().any(|e| e.occurrence_id == id);
    let mut out: Vec<Unrecorded<'_>> = Vec::new();
    if let Some(o) = app.occurrence.as_ref().filter(|o| !graded(&o.id))
        && let Some(answer) = o.answer.as_deref()
    {
        out.push(Unrecorded {
            occurrence_id: &o.id,
            question_id: &o.question_id,
            version: o.presentation.version,
            answer,
            at: o.created_ms,
            checks: Vec::new(),
        });
    }
    let mut saved: Vec<_> = app
        .assessments
        .values()
        .filter(|a| !graded(&a.occurrence_id))
        .collect();
    saved.sort_by_key(|a| a.created_ms);
    for a in saved {
        if let Some(u) = out.iter_mut().find(|u| u.occurrence_id == a.occurrence_id) {
            u.at = u.at.max(a.created_ms);
            u.checks.push(a);
            continue;
        }
        out.push(Unrecorded {
            occurrence_id: &a.occurrence_id,
            question_id: &a.question_id,
            version: a.content_version,
            answer: &a.answer,
            at: a.created_ms,
            checks: vec![a],
        });
    }
    out
}
/// A saved check is named by its stored status alone, never by the message
/// stored with it.
fn check_label(status: &str) -> (&'static str, &'static str) {
    match status {
        "queued" => ("queued", "Check recorded: queued, never sent."),
        "sent" => ("sent", "Check recorded: sent, never confirmed."),
        "failed" => ("failed", "Check recorded: failed."),
        "unknown" => ("unknown", "Check recorded: outcome unknown."),
        "paused" => ("paused", "Check recorded: paused, never sent."),
        "unsure" => ("unsure", "Check recorded: inconclusive."),
        "superseded" => ("superseded", "Check recorded: superseded."),
        "complete" => ("complete", "Check recorded: complete."),
        _ => ("recorded", "A check was recorded."),
    }
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
fn next_review_line(at: i64) -> String {
    format!("Next review {}.", date(at))
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
    shell("Owner token", "/settings", &content, None, false, "/")
}
