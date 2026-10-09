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

struct View<'a> {
    app: &'a App,
    path: &'a str,
    query: &'a BTreeMap<String, String>,
    now: i64,
}
impl View<'_> {
    fn form(&self, action: &str, suffix: &str, class: &str, extra: &str, body: &str) -> String {
        let nonce = self.query.get("__nonce").map(String::as_str).unwrap_or("");
        let identity = format!(
            "{nonce}:{}:{}:{}:{action}:{suffix}",
            self.now, self.app.revision, self.path
        );
        let operation = hex::encode(Sha256::digest(identity.as_bytes()));
        format!(
            "<form method=\"post\" action=\"{}\" class=\"{}\" {}>{}{}{}{body}</form>",
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
            escape(&concept.name)
        }
    }
    fn gate(&self, destination: &str) -> String {
        if self
            .app
            .occurrence
            .as_ref()
            .is_some_and(|occurrence| occurrence.phase == "checking")
        {
            return format!(
                "<section class=\"empty-stage gate\" data-state=\"checking-reference\" data-poll>{}<p class=\"eyebrow\">Your answer is saved</p><h1>Your check is still in progress.</h1><p class=\"lede\">This reference may contain the answer. It will be available after the check finishes.</p><div class=\"actions\"><a class=\"button primary\" href=\"/\">Return to your saved answer →</a></div></section>",
                emblem("large")
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
            "<section class=\"empty-stage gate\" data-state=\"gate\">{}<p class=\"eyebrow\">A little help is part of learning</p><h1>Open your reference?</h1><p class=\"lede\">Your notes may contain the answer. Opening them records this attempt as helped practice.</p><div class=\"actions\">{action}<a class=\"button quiet\" href=\"/\">Keep trying</a></div></section>",
            emblem("large")
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
            let phase = self
                .app
                .jobs
                .get(&goal.job_id)
                .map(|j| j.status.as_str())
                .unwrap_or(goal.status.as_str());
            let text = match phase {
                "queued" | "pending" => "Preparing your material",
                "running" | "generating" | "critic" | "sent" | "candidates" | "critic-sent" => {
                    "Working on your explanation and practice"
                }
                "unknown" => "Preparation stopped with an uncertain outcome",
                "failed" | "blocked" => "Preparation needs your attention",
                "partial" => "Some material is ready; preparation is incomplete",
                _ => "Preparation is in progress",
            };
            let stopped = matches!(phase, "unknown" | "failed" | "blocked" | "paused");
            let _ = write!(
                out,
                "<a class=\"preparation-receipt {}\" href=\"/goals/{}\"><span class=\"status-dot\" aria-hidden=\"true\"></span><span><strong>{}</strong><span>{text}</span></span><span aria-hidden=\"true\">↗</span></a>",
                if stopped { "stopped" } else { "" },
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
                let _ = write!(
                    out,
                    "<p class=\"lede\">{}</p>{}",
                    escape(&c.summary),
                    notes(c)
                );
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
            if o.phase == "result" || o.assisted {
                let _ = write!(
                    out,
                    "<a class=\"concept-chip\" href=\"/concepts/{}\"><span aria-hidden=\"true\">✦</span>{}</a>",
                    escape(&c.id),
                    self.concept_name(c)
                );
            } else {
                let _ = write!(
                    out,
                    "<span class=\"concept-chip\"><span aria-hidden=\"true\">✦</span>{}</span>",
                    self.concept_name(c)
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
                out.push_str("<div class=\"checking\" role=\"status\"><p class=\"eyebrow\">Your answer is saved</p><h2>Checking your meaning.</h2><p>You can leave and come back. Your question will stay here.</p></div>");
                if let Some(answer) = &o.answer {
                    let _ = write!(
                        out,
                        "<div class=\"answer-pair\"><span>You answered</span><p>{}</p></div>",
                        escape(answer)
                    );
                }
                out.push_str("<a class=\"text-link\" href=\"/\">Check again</a>");
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
        if !correct
            && result.rating > 0
            && !helped
            && matches!(result.authority.as_str(), "exact" | "jev")
            && !result.corrected
        {
            out.push_str(&self.review_form(
                "override",
                "right",
                "",
                "",
                &format!(
                    "{}<button class=\"text-button\">I was right</button>",
                    hidden("correct", "true")
                ),
            ));
        }
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
        let failed = assessment
            .is_some_and(|a| matches!(a.status.as_str(), "failed" | "unknown" | "superseded"));
        let mut out = format!(
            "<div class=\"feedback feedback-self\" role=\"status\"><span class=\"result-mark\" aria-hidden=\"true\">≈</span><div><h2>You know your answer best.</h2><p>{}</p></div></div>{}<p class=\"fineprint\">Your judgment will be saved as a learner check. Reading this answer makes the next practice helped.</p><div class=\"thumb-actions split-actions\">",
            if failed {
                "The check couldn't finish. Compare your answer below."
            } else {
                "The check wasn't certain. Compare your answer below."
            },
            self.answer_pair(o)
        );
        out.push_str(&self.review_form(
            "self",
            "right",
            "",
            "",
            &format!(
                "{}<button class=\"button primary\">I was right</button>",
                hidden("correct", "true")
            ),
        ));
        out.push_str(&self.review_form(
            "self",
            "miss",
            "",
            "",
            &format!(
                "{}<button class=\"button secondary\">Not quite</button>",
                hidden("correct", "false")
            ),
        ));
        out.push_str("</div>");
        if failed {
            out.push_str(&self.review_form(
                "retry",
                "retry",
                "",
                "",
                "<button class=\"text-button\">Retry the check</button>",
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
        if let Some(r) = &o.result
            && r.rating > 0
            && !o.assisted
            && r.outcome == "correct"
            && matches!(r.authority.as_str(), "exact" | "jev")
            && !r.corrected
        {
            out.push_str(&self.review_form(
                "override",
                "miss",
                "",
                "",
                &format!(
                    "{}<button>Count as a miss</button>",
                    hidden("correct", "false")
                ),
            ));
        }
        let _ = write!(
            out,
            "<a href=\"/questions/{}/edit\">Edit this question</a>",
            escape(&o.question_id)
        );
        out.push_str(&self.form(
            &format!("/questions/{}/fix", o.question_id),
            "fix",
            "",
            "",
            &format!(
                "{}<button>Prepare a better question</button>",
                hidden("content_version", &o.presentation.version.to_string())
            ),
        ));
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
    fn empty(&self) -> String {
        let any = self.app.goals.values().any(|g| !g.archived);
        let pending = self
            .app
            .goals
            .values()
            .any(|g| !g.archived && g.status != "ready");
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
        let (title, desc) = if !any {
            ("Follow your curiosity.","A word, a question, a train of thought. Tell Scry what you want to understand, and turn it into something you can remember.".to_string())
        } else if pending {
            ("Your next idea is taking shape.","Your request is saved. You can leave while Scry prepares an explanation and practice, or explore what you already have.".to_string())
        } else {
            ("A good place to pause.",due.map(|d|format!("You're caught up for now. Your next practice is {}.",relative(d,self.now))).unwrap_or_else(||"You have no eligible questions right now. Explore your notes, resume a paused goal, or follow a new curiosity.".into()))
        };
        format!(
            "{demo}{}<section class=\"empty-stage\" data-state=\"{}\">{}<p class=\"eyebrow\">A private space for understanding</p><h1>{title}</h1><p class=\"lede\">{desc}</p><div class=\"actions\"><a class=\"button primary\" href=\"/create\">Create something to learn <span aria-hidden=\"true\">→</span></a>{}</div><div class=\"notebook-margin\"><span>01 / Curiosity</span><span>Understanding starts with a question.</span></div></section>",
            self.pending(),
            if !any {
                "empty"
            } else if pending {
                "preparing"
            } else {
                "caught-up"
            },
            emblem("large"),
            if any {
                "<a class=\"text-link\" href=\"/map\">Explore your map ↗</a>"
            } else {
                ""
            }
        )
    }
    fn create(&self) -> String {
        let prefill = self
            .query
            .get("text")
            .or_else(|| self.query.get("intent"))
            .or_else(|| self.query.get("url"))
            .map(String::as_str)
            .unwrap_or("");
        let body = format!(
            "<label for=\"intent\" class=\"sr-only\">What do you want to learn?</label><div class=\"writing-area\"><textarea id=\"intent\" name=\"intent\" rows=\"7\" maxlength=\"32768\" placeholder=\"Why do we forget things?\n\nA word is enough. Or tell me everything on your mind.\" aria-describedby=\"intent-hint\">{}</textarea><span class=\"writing-mark\" aria-hidden=\"true\">✦</span></div><p class=\"control-hint\" id=\"intent-hint\">A topic, your own words, or a dictated ramble. No setup needed.</p><details class=\"optional-material\"><summary>Add a photo for context <span aria-hidden=\"true\">＋</span></summary><label for=\"photo\">Optional photo</label><input type=\"file\" id=\"photo\" name=\"photo\" accept=\"image/jpeg,image/png,image/webp\" aria-describedby=\"photo-hint\"><p class=\"fineprint\" id=\"photo-hint\">JPEG, PNG or WebP, up to 4 MB, with a caption up to 1 KB. Scry reads the words; your prompt guides the result.</p></details><div class=\"thumb-actions\"><button class=\"button primary\">Make it understandable <span aria-hidden=\"true\">→</span></button></div>",
            escape(prefill)
        );
        format!(
            "<section class=\"create-page\" data-state=\"create\"><div class=\"page-heading\"><p class=\"eyebrow\">From curiosity to understanding</p><h1>What do you want<br class=\"desktop-break\"> to learn?</h1><p class=\"lede\">Start with what is on your mind. Get a clear explanation and a little practice. Shape it as you go.</p></div>{}<p class=\"privacy-note\">{}<span>Private by default. Your words are used to prepare your material; pasted links do not trigger web browsing.</span></p><div class=\"notebook-margin\"><span>Write freely</span><span>Your first result is a beginning, not a test.</span></div></section>",
            self.form(
                "/create",
                "capture",
                "create-form",
                "enctype=\"multipart/form-data\"",
                &body
            ),
            lock_icon()
        )
    }
    fn map(&self) -> String {
        let mut goals: Vec<_> = self.app.goals.values().filter(|g| !g.archived).collect();
        goals.sort_by_key(|g| (g.paused, !g.focused, g.created_ms));
        let mut out = String::from(
            "<section class=\"map-page\" data-state=\"map\"><div class=\"page-heading\"><p class=\"eyebrow\">Your growing understanding</p><h1>A map of ideas.</h1><p class=\"lede\">Follow the connections. Revisit a note. Give the ideas that matter a little more attention.</p></div>",
        );
        if goals.is_empty() {
            out.push_str("<div class=\"quiet-empty\"><p>Your map begins with one curiosity.</p><a class=\"button primary\" href=\"/create\">Create your first material →</a></div>");
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
                status_label(&goal.status)
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
        out.push_str("<a class=\"text-link add-curiosity\" href=\"/create\">Follow another curiosity <span aria-hidden=\"true\">＋</span></a></section>");
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
                        "<section class=\"reference-idea\"><h2><a href=\"/concepts/{}\">{}</a></h2><p class=\"lede small\">{}</p>{}<a class=\"text-link\" href=\"/concepts/{}\">Explore this idea &amp; practice ↗</a></section>",
                        escape(&c.id),
                        self.concept_name(c),
                        escape(&c.summary),
                        if self.app.protected_concept(&c.id) {
                            "<p>This idea is in your current question. Open its page to record assistance before reading.</p>".into()
                        } else {
                            notes(c)
                        },
                        escape(&c.id)
                    );
                }
            }
            out.push_str("</section><a class=\"button primary\" href=\"/\">Go to practice →</a>");
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
            let feedback = String::from(
                "<label for=\"feedback\">What would make this more useful?</label><textarea id=\"feedback\" name=\"feedback\" rows=\"3\" required maxlength=\"4000\" placeholder=\"A simpler explanation, a practical example, something you are still wondering…\"></textarea><button class=\"button secondary\">Refine my material →</button>",
            );
            out.push_str(&format!("<section class=\"refinement\"><h2>Shape what comes next.</h2><p>Your answers and feedback guide the next material. Your existing notes and attempts stay saved.</p>{}</section>",self.form(&format!("/goals/{id}/refine"),"refine","","",&feedback)));
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
        let status = job.map(|j| j.status.as_str()).unwrap_or(g.status.as_str());
        let stopped = matches!(
            status,
            "failed" | "unknown" | "blocked" | "budget" | "paused" | "superseded"
        );
        let pending = matches!(status, "queued" | "candidates" | "sent" | "critic-sent");
        let title = match status {
            "unknown" => "The outcome needs checking.",
            "failed" | "blocked" => "Preparation stopped.",
            "paused" => "Preparation is paused after recovery.",
            "budget" => "Your preparation allowance is in use.",
            "partial" => "A useful part is ready.",
            "superseded" => "Your request has changed.",
            _ => "Your material is taking shape.",
        };
        let mut out = format!(
            "<section class=\"preparation-panel {}\" {}><p class=\"eyebrow\">{} saved</p><h2>{title}</h2><p>{}</p>",
            if stopped { "stopped" } else { "" },
            if pending { "data-poll" } else { "" },
            if g.photo.is_some() {
                "Your photo and request are"
            } else {
                "Your request is"
            },
            if status == "unknown" {
                "A request may have reached the model. Its allowance remains accounted for. Retrying is a deliberate new attempt, not a claim that the earlier one failed."
            } else if status == "partial" {
                "Your saved material is ready to read and practice. Add feedback below to shape the parts that still need work."
            } else if status == "superseded" {
                "This preparation belongs to an earlier request. Your input and existing material are saved. Add feedback below to prepare material for your newer direction."
            } else if stopped {
                "Your input is safe. Inspect the reason, add feedback, or keep studying existing material. Available retries are deliberate new attempts."
            } else {
                "Scry is preparing an explanation, examples, and practice. You can leave this page; your progress is saved."
            }
        );
        if let Some(error) = job.and_then(|j| j.error.as_ref()) {
            let _ = write!(
                out,
                "<details><summary>Why preparation stopped</summary><p>{}</p></details>",
                escape(error)
            );
        }
        if matches!(status, "failed" | "unknown" | "paused") && job.is_some_and(|j| j.attempt < 3) {
            out.push_str(&self.form(
                &format!("/goals/{}/retry", g.id),
                "retry",
                "",
                "",
                "<button class=\"button secondary\">Try preparation again</button>",
            ));
        }
        if stopped {
            let accounted = self
                .app
                .spend
                .iter()
                .filter(|s| job.is_some_and(|j| s.work_id == j.id))
                .map(|s| s.cost_micros.unwrap_or(s.reserved_micros))
                .sum::<u64>() as f64
                / 1_000_000.0;
            let _ = write!(
                out,
                "<p class=\"fineprint\">${accounted:.2} used or reserved for this preparation. Uncertain outcomes remain accounted.</p>"
            );
            if job.is_some_and(|j| j.attempt >= 3) {
                out.push_str("<p class=\"fineprint\">The attempt limit is reached. Save a new request or add feedback to change what comes next.</p><a class=\"text-link\" href=\"/create\">Create a new request →</a>");
            }
        }
        out.push_str("<a class=\"text-link\" href=\"/\">Return to practice →</a></section>");
        out
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
            "<article class=\"concept-page\" data-state=\"concept\"><a class=\"back-link\" href=\"/map\">← Your map</a><div class=\"page-heading\"><p class=\"eyebrow\">{} · One connected idea</p><h1>{}</h1><p class=\"lede\">{}</p></div><section class=\"practice-evidence\" aria-label=\"Your practice record\"><p><strong>{unaided}</strong> recalled <span>·</span> <strong>{helped}</strong> with help <span>·</span> <strong>{missed}</strong> missed</p><p class=\"fineprint\">{}</p>{}</section>{}",
            concept_state(self.app, c, self.now),
            self.concept_name(c),
            escape(&c.summary),
            recall_label(self.app, c, self.now),
            if c.archived {
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
            format_args!("{}{}", tally(&state), notes(c))
        );
        if !c.prerequisites.is_empty() {
            out.push_str("<section class=\"connected\"><h2>Builds on</h2>");
            for prerequisite in &c.prerequisites {
                if let Some(p) = self.app.concepts.get(prerequisite) {
                    let _ = write!(
                        out,
                        "<a class=\"concept-chip\" href=\"/concepts/{}\">✦ {}</a>",
                        escape(&p.id),
                        self.concept_name(p)
                    );
                }
            }
            out.push_str("</section>");
        }
        out.push_str("<section class=\"saved-questions\"><h2>Practice, in different ways.</h2>");
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
        if let Some(g) = self.app.goals.get(&c.goal_id) {
            let _ = write!(
                out,
                "<a class=\"text-link\" href=\"/goals/{}\">Original request &amp; preparation ↗</a>",
                escape(&g.id)
            );
        }
        out.push_str(&self.feedback_history(&c.goal_id, Some(id)));
        out.push_str("</article>");
        out
    }
    fn history(&self) -> String {
        let mut out = String::from(
            "<section class=\"history-page\" data-state=\"history\"><div class=\"page-heading\"><p class=\"eyebrow\">A record, not a score</p><h1>The work of remembering.</h1><p class=\"lede\">Your attempts stay as they happened. Help, misses, and corrections are part of the story.</p></div>",
        );
        if self.app.events.is_empty() {
            out.push_str("<p class=\"quiet-empty\">Your first practice will appear here.</p>");
        }
        for event in self.app.events.iter().rev() {
            if self.app.protected_concept(&event.concept_id) {
                out.push_str("<p class=\"fineprint\">An earlier attempt is hidden while you work on its question from memory.</p>");
                continue;
            }
            let corrected = self.app.overrides.iter().any(|o| o.event_id == event.id);
            let outcome = if event.assisted {
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
    fn settings(&self) -> String {
        let mut pace = String::from(
            "<fieldset class=\"pace-options\"><legend>How many new ideas in a day?</legend>",
        );
        for (value, title, desc) in [
            ("light", "A little", "Up to 3 new ideas"),
            ("steady", "A steady rhythm", "Up to 6 new ideas"),
            ("intense", "A deeper dive", "Up to 12 new ideas"),
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
        pace.push_str("</fieldset><button class=\"button secondary\">Save my rhythm</button>");
        let spent = self.app.spent(self.now) as f64 / 1_000_000.0;
        let unknown = self
            .app
            .spend
            .iter()
            .filter(|s| s.cost_micros.is_none())
            .count();
        let backup=match self.app.backup.completed_ms{Some(time)=>format!("Last verified backup: {}.{}",date(time),if self.now-time>DAY_MS{" A fresh backup needs attention."}else{""}),None=>"No verified remote backup is recorded. Recovery needs attention before personal use.".into()};
        let mut out = format!(
            "<section class=\"settings-page\" data-state=\"settings\"><div class=\"page-heading\"><p class=\"eyebrow\">Make room for learning</p><h1>Your rhythm.</h1><p class=\"lede\">A little practice you return to is better than a pile of work you don't.</p></div>{}<section class=\"settings-section\"><h2>Preparation allowance</h2><p class=\"spend-number\">${spent:.2}<span> / $3.50</span></p><p>Used or reserved in the last 24 hours. Each preparation reserves $0.50 before it begins. Uncertain outcomes keep their reservation.</p><p class=\"fineprint\">{unknown} reservations without confirmed final usage. Provider limits apply independently.</p></section><section class=\"settings-section\"><h2>Your notebook is worth keeping.</h2><p>{backup}</p><p class=\"fineprint\">Daily remote snapshots and 30-day retention target 24 hours of recoverable work and a 60-minute recovery. A target is not a guarantee.</p>{}</section><section class=\"settings-section\"><h2>Take your learning with you.</h2><p>A portable record of your requests, notes, content versions, attempts, and schedules.</p><a class=\"button secondary\" href=\"/export\" download>Export your notebook ↓</a></section><section class=\"settings-section font-colophon\"><p>Set in Literata and Atkinson Hyperlegible Next. Thoughtful letterforms, served from Scry.</p><a href=\"/assets/fonts/OFL.txt\">Font licenses ↗</a></section></section>",
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
        out.push_str(&self.form(
            "/settings/backup",
            "backup",
            "backup-form",
            "",
            "<button class=\"button secondary\">Create a verified backup</button>",
        ));
        if self.app.restored_paused {
            out.push_str("<p class=\"notice\">This restored notebook has background work paused. Review uncertain outcomes and recovery configuration before deliberately resuming preparation.</p>");
        }
        if self.query.get("__synthetic").is_some_and(|v| v == "true") {
            out.push_str("<p class=\"synthetic-note\">This is an isolated synthetic notebook. It does not prove production access, model usefulness, or remote recovery.</p>");
        }
        out
    }
    fn fix_status(&self, q: &Question) -> String {
        let job = self
            .app
            .jobs
            .values()
            .filter(|j| j.question_id.as_deref() == Some(q.id.as_str()) && j.kind == "fix")
            .max_by_key(|j| j.created_ms);
        let Some(job) = job else {
            return String::new();
        };
        if matches!(
            job.status.as_str(),
            "queued" | "sent" | "candidates" | "critic-sent"
        ) {
            return "<aside class=\"notice\" data-poll><strong>A better version is being prepared.</strong><p>Your current wording stays saved. The result will be a draft for you to review.</p></aside>".into();
        }
        if matches!(job.status.as_str(), "failed" | "unknown" | "paused") {
            let accounted = self
                .app
                .spend
                .iter()
                .filter(|s| s.work_id == job.id)
                .map(|s| s.cost_micros.unwrap_or(s.reserved_micros))
                .sum::<u64>() as f64
                / 1_000_000.0;
            return format!(
                "<aside class=\"notice\"><strong>Draft preparation stopped.</strong><p>{}</p><p class=\"fineprint\">${accounted:.2} used or reserved. Your saved question has not changed.</p>{}</aside>",
                escape(
                    job.error
                        .as_deref()
                        .unwrap_or("Its outcome has not been confirmed.")
                ),
                if job.attempt < 3 {
                    self.form(
                        &format!("/questions/{}/fix", q.id),
                        "retry-fix",
                        "",
                        "",
                        &format!(
                            "{}<button class=\"text-button\">Try drafting again</button>",
                            hidden("content_version", &q.content().version.to_string())
                        ),
                    )
                } else {
                    "<p>The attempt limit is reached. You can edit the question yourself.</p>"
                        .into()
                }
            );
        }
        String::new()
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
            "<section class=\"empty-stage\" data-state=\"not-found\">{}<p class=\"eyebrow\">A page has gone astray</p><h1>Nothing on this page.</h1><p class=\"lede\">Your saved notebook is still here. Return to your practice or explore the map.</p><a class=\"button primary\" href=\"/\">Return to practice →</a></section>",
            emblem("large")
        )
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
        "/" => view.study(),
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
                ["goals", id] => view.goal(id),
                ["concepts", id] => view.concept(id),
                ["questions", id, "edit"] => view.edit(id),
                _ => view.not_found(),
            }
        }
    };
    let title = match path {
        "/create" | "/add" => "Create",
        "/map" => "Your map",
        "/history" => "History",
        "/settings" => "Your rhythm",
        _ => "Your notebook",
    };
    let poll = app
        .occurrence
        .as_ref()
        .is_some_and(|o| matches!(o.phase.as_str(), "checking" | "pending"))
        || app.jobs.values().any(|j| {
            matches!(
                j.status.as_str(),
                "queued" | "sent" | "candidates" | "critic-sent"
            )
        });
    shell(
        title,
        path,
        &content,
        notice,
        query.get("__synthetic").is_some_and(|v| v == "true"),
        poll,
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
        "<section class=\"empty-stage\" data-state=\"error\">{}<p class=\"eyebrow\">Request {status}</p><h1>{title}</h1><p class=\"lede\">{}</p><a class=\"button primary\" href=\"/\">Return to your notebook →</a></section>",
        emblem("large"),
        escape(message)
    );
    shell("A moment to pause", "/", &content, None, false, false)
}

fn shell(
    title: &str,
    path: &str,
    content: &str,
    notice: Option<&str>,
    synthetic: bool,
    poll: bool,
) -> String {
    let notice = notice
        .map(|n| format!("<div class=\"notice\" role=\"status\">{}</div>", escape(n)))
        .unwrap_or_default();
    format!(
        "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1, viewport-fit=cover\"><meta name=\"color-scheme\" content=\"light dark\"><meta name=\"theme-color\" content=\"#f5f2ea\"><meta name=\"robots\" content=\"noindex,nofollow\"><title>{} · Scry</title><link rel=\"icon\" href=\"/assets/icon.svg\" type=\"image/svg+xml\"><link rel=\"stylesheet\" href=\"/assets/app.css\"><script src=\"/assets/app.js\" defer></script></head><body><a class=\"skip-link\" href=\"#main\">Skip to the page</a><div class=\"page-frame\"><header class=\"masthead\"><a class=\"wordmark\" href=\"/\" aria-label=\"Scry, return to practice\">{}<span>scry<span class=\"wordmark-period\">.</span></span></a><nav aria-label=\"Main navigation\"><a href=\"/create\" {}>Create</a><a href=\"/map\" {}>Map</a></nav></header>{}<div id=\"request-status\" class=\"request-status\" role=\"status\" aria-live=\"polite\" hidden></div><main id=\"main\" tabindex=\"-1\" {}>{notice}{content}</main><footer class=\"footer\"><span class=\"footer-motto\">A little understanding, kept.</span><div><a href=\"/history\">History</a><a href=\"/settings\">Settings</a></div></footer></div></body></html>",
        escape(title),
        emblem("wordmark-emblem"),
        if matches!(path, "/create" | "/add") {
            "aria-current=\"page\""
        } else {
            ""
        },
        if path == "/map" {
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
    )
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
fn lock_icon() -> &'static str {
    "<svg width=\"17\" height=\"17\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"1.5\" aria-hidden=\"true\"><rect x=\"5\" y=\"10\" width=\"14\" height=\"11\" rx=\"2\"/><path d=\"M8 10V7a4 4 0 0 1 8 0v3\"/></svg>"
}
fn notes(c: &Concept) -> String {
    let Some(note) = c.notes.last() else {
        return "<p class=\"fineprint\">No note is available yet.</p>".into();
    };
    let paragraphs = note
        .text
        .split("\n\n")
        .filter(|p| !p.trim().is_empty())
        .map(|p| format!("<p>{}</p>", escape(p)))
        .collect::<String>();
    format!(
        "<section class=\"concept-note\" aria-label=\"Saved explanation\"><div class=\"reading\">{paragraphs}</div>{}</section>",
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
                .filter(|e| e.question_id == q.id)
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
        _ => "Preparing",
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
        format!("in {} minutes", (diff / 60_000).max(1))
    } else if diff < DAY_MS {
        format!("in {} hours", (diff / 3_600_000).max(1))
    } else {
        format!("in {} days", (diff / DAY_MS).max(1))
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
            include_bytes!("../assets/fonts/Literata.woff2"),
        )),
        "/assets/fonts/Literata-Italic.woff2" => Some((
            "font/woff2",
            include_bytes!("../assets/fonts/Literata-Italic.woff2"),
        )),
        "/assets/fonts/AtkinsonHyperlegibleNext.woff2" => Some((
            "font/woff2",
            include_bytes!("../assets/fonts/AtkinsonHyperlegibleNext.woff2"),
        )),
        "/assets/fonts/OFL.txt" => Some((
            "text/plain; charset=utf-8",
            include_bytes!("../assets/fonts/OFL.txt"),
        )),
        _ => None,
    }
}
