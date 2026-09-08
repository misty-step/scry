//! Server-rendered study UI.
//!
//! Reading-first Scry surfaces use the repo-owned `assets/ledger.css` sheet,
//! served at the stable `/static/ledger.css` path. `DESIGN.md` binds palette,
//! typography, accessibility, and the deliberate study loop.
//! Every action remains a full-page form POST; `assets/app.js` progressively
//! enhances timing, generation status, and save-in-place feedback. A graded
//! Quiz never advances without Continue. Study notes return to the same Quiz.

use std::fmt::Write as _;

use memory_engine_study::{
    BetaStudyConceptProgress, BetaStudyCurrent, BetaStudyFeedback, BetaStudyGrade,
    LibrarySourceRow, SourcePermission,
};

#[cfg(test)]
use memory_engine_api_state::ApiState;
use memory_engine_api_state::{
    AppAccount, GenerationJob, JobStatus, SourceRecord, StudyViewResponse,
};

#[cfg(test)]
fn render_test_state(email: &str) -> ApiState {
    use memory_engine_api_state::{AccountRegistry, AuthConfig};
    ApiState::new(AccountRegistry::default().with_auth_config(
        AuthConfig::allow_emails([email.to_owned()]).with_anonymous_account_creation(true),
    ))
}

pub struct ContentFeedbackRecovery<'a> {
    pub review_unit_id: &'a str,
    pub verdict: &'a str,
    pub rationale: Option<&'a str>,
    pub idempotency_key: &'a str,
    pub supersedes_id: Option<&'a str>,
    pub message: &'a str,
}

pub const SKIP_CONFIRM_NOTICE: &str = "You'll see this later this session.";
pub const SNOOZE_CONFIRM_NOTICE: &str = "You'll see this tomorrow.";
pub const SNOOZE_CONCEPT_CONFIRM_NOTICE: &str = "You'll see this concept tomorrow.";

#[must_use]
pub fn render_content_feedback_result_html(
    account: &AppAccount,
    view: &StudyViewResponse,
    jobs: &[GenerationJob],
    notice: &str,
) -> String {
    if view.current.is_none() && view.summary.approved_review_unit_count > 0 {
        let inner = render_signed_in(
            account,
            &[],
            Some(view),
            &[],
            Some(notice),
            SignedInSurface::ReviewComplete,
        );
        document_with_head(&inner, "")
    } else {
        render_account_page(account, Some(view), jobs, Some(notice))
    }
}

#[must_use]
pub fn render_content_feedback_recovery_html(
    account: &AppAccount,
    due_count: usize,
    recovery: &ContentFeedbackRecovery<'_>,
) -> String {
    let supersedes = recovery.supersedes_id.map_or_else(String::new, |id| {
        format!(
            r#"<input type="hidden" name="supersedesId" value="{}">"#,
            escape_html(id)
        )
    });
    let choice = if recovery.verdict == "dropped" {
        "Drop this quiz"
    } else {
        "Keep this quiz"
    };
    let body = format!(
        r#"<section class="ae-group me-content-feedback">
<p class="me-kicker">Feedback not saved</p>
<h1 class="me-display">Your feedback needs another try</h1>
<p class="ae-lede">Selected: <strong>{choice}</strong></p>
<form action="/app/content-feedback" method="post">
{csrf}<input type="hidden" name="reviewUnitId" value="{review_unit_id}">
<input type="hidden" name="verdict" value="{verdict}">
<input type="hidden" name="idempotencyKey" value="{idempotency_key}">
{supersedes}
<label class="ae-label" for="me-content-feedback-retry-rationale">Why? <span class="ae-dim">(optional)</span></label>
<textarea class="ae-input me-content-feedback-rationale" id="me-content-feedback-retry-rationale" name="rationale" rows="2">{rationale}</textarea>
<div class="me-actions"><button class="ae-button" type="submit">Retry feedback</button></div>
</form>
</section>"#,
        choice = choice,
        csrf = hidden_csrf_input(account),
        review_unit_id = escape_html(recovery.review_unit_id),
        verdict = escape_html(recovery.verdict),
        idempotency_key = escape_html(recovery.idempotency_key),
        supersedes = supersedes,
        rationale = escape_html(recovery.rationale.unwrap_or_default()),
    );
    document(&render_signed_in_body(
        account,
        due_count,
        Some(recovery.message),
        &[],
        &body,
        None,
    ))
}

#[must_use]
pub fn render_submit_action_result_html(
    account: &AppAccount,
    view: Option<&StudyViewResponse>,
    jobs: &[GenerationJob],
    notice: Option<&str>,
    request_id: &str,
    trace_id: Option<&str>,
) -> String {
    let trace = trace_id.map_or_else(String::new, |trace_id| {
        format!(
            r#"<meta name="memory-engine-submit-handoff" content="{}">"#,
            escape_html(trace_id)
        )
    });
    let head = format!(
        r#"<meta name="memory-engine-csrf-token" content="{}">
<meta name="memory-engine-submit-request" content="{}">
{}"#,
        escape_html(account.csrf_token()),
        escape_html(request_id),
        trace,
    );
    render_app_shell_with_head(Some(account), &[], view, jobs, notice, &head)
}

#[must_use]
pub fn render_account_page(
    account: &AppAccount,
    view: Option<&StudyViewResponse>,
    jobs: &[GenerationJob],
    notice: Option<&str>,
) -> String {
    render_app_shell(Some(account), &[], view, jobs, notice)
}

#[must_use]
pub fn render_edit_review_html(
    account: &AppAccount,
    view: &StudyViewResponse,
    jobs: &[GenerationJob],
    notice: Option<&str>,
) -> String {
    document(&render_signed_in(
        account,
        &[],
        Some(view),
        jobs,
        notice,
        SignedInSurface::Edit,
    ))
}

/// Read the current Quiz's saved Study note without advancing its occurrence.
/// The route resolves the note; rendering never loads or invents material.
#[must_use]
pub fn render_reference_page(account: &AppAccount, view: &StudyViewResponse) -> String {
    let body = view.current.as_ref().map_or_else(
        || {
            r#"<section class="me-study-note"><h1 class="me-display">No quiz is open</h1><p class="ae-lede">Open a quiz to read its study note.</p><a class="ae-button" href="/">Back to Home</a></section>"#.to_owned()
        },
        |current| {
            let reading = current
                .reference_text
                .as_deref()
                .filter(|text| !text.trim().is_empty())
                .map_or_else(
                    || r#"<p class="ae-lede">No study note is available for this quiz. Return to the quiz or use its More actions to request easier practice.</p>"#.to_owned(),
                    |text| format!(r#"<div class="me-reading">{}</div>"#, escape_html(text)),
                );
            format!(
                r#"<article class="me-study-note" aria-labelledby="me-study-note-title">
<h1 class="me-display" id="me-study-note-title">Study note</h1>
<p class="me-prompt">{prompt}</p>
{reading}
<form class="me-resume" action="/app/resume" method="post">{csrf}<input type="hidden" name="reviewUnitId" value="{id}"><button class="ae-button" type="submit">Return to this quiz</button></form>
</article>"#,
                prompt = escape_html(&current.prompt),
                csrf = hidden_csrf_input(account),
                id = escape_html(&current.review_unit_id.to_string()),
            )
        },
    );
    document(&render_signed_in_body(
        account,
        view.due_count,
        None,
        &[],
        &body,
        None,
    ))
}

#[must_use]
pub fn render_app_shell(
    account: Option<&AppAccount>,
    sources: &[SourceRecord],
    view: Option<&StudyViewResponse>,
    jobs: &[GenerationJob],
    notice: Option<&str>,
) -> String {
    render_app_shell_with_head(account, sources, view, jobs, notice, "")
}

fn render_app_shell_with_head(
    account: Option<&AppAccount>,
    sources: &[SourceRecord],
    view: Option<&StudyViewResponse>,
    jobs: &[GenerationJob],
    notice: Option<&str>,
    head: &str,
) -> String {
    let inner = match account {
        Some(account) => {
            render_signed_in(account, sources, view, jobs, notice, SignedInSurface::Home)
        }
        None => render_signed_out(notice),
    };
    document_with_head(&inner, head)
}

/// The Create view: the capture form alone, with persistent nav
/// (memory-engine-087). Capture is one job-to-be-done — it never shares a
/// scroll with source management or analytics.
#[must_use]
pub fn render_create_page(
    account: &AppAccount,
    view: Option<&StudyViewResponse>,
    jobs: &[GenerationJob],
    notice: Option<&str>,
) -> String {
    let inner = render_signed_in(account, &[], view, jobs, notice, SignedInSurface::Create);
    document(&inner)
}

/// A saved capture follows one durable job directly into review. Failed jobs
/// stay on this surface with their source intact and an authorized retry.
#[must_use]
pub fn render_capture_waiting_page(account: &AppAccount, job: &GenerationJob) -> String {
    let heading = match job.status {
        JobStatus::Succeeded => "Your quizzes are ready",
        JobStatus::Failed => "Your text is safe",
        JobStatus::Queued | JobStatus::Running | JobStatus::Retry => {
            "Turning this into something you’ll remember"
        }
    };
    let body = format!(
        r#"<section class="me-generation-waiting" data-generation-job-id="{id}" data-terminal-url="/" data-status="{status}" aria-labelledby="me-generation-title">
<h1 class="me-display" id="me-generation-title">{heading}</h1>
<p class="me-generation-source">{title}</p>
<p class="me-generation-status" data-generation-status role="status" aria-live="polite" aria-atomic="true">{message}</p>
<div data-generation-recovery>{retry}</div>
<a class="ae-button" href="/">Keep reviewing</a>
<p class="me-hint me-generation-help">You can leave this open or come back later. Your text is already saved.</p>
<noscript><p class="me-hint"><a href="/app/capture?jobId={id}">Check progress</a>, then open review when your quizzes are ready.</p></noscript>
</section>"#,
        id = escape_html(&job.id),
        status = job.status.as_str(),
        title = escape_html(&job.title),
        message = job_meta(job),
        retry = render_job_retry(account, job),
    );
    // This API has no study view. Keep the enhancement hook without inventing
    // a zero due count or fetching unrelated account state.
    let header = format!(r#"<span class="me-due"></span>{}"#, account_menu(account),);
    document(&screen(&header, &body, &render_nav("create")))
}

/// The Library view: saved material with per-source active-card counts and
/// concept drilldown, plus the generation activity log (memory-engine-087).
/// Source management is one job-to-be-done — it never shares a scroll with
/// capture or analytics.
#[must_use]
pub fn render_library_page(
    account: &AppAccount,
    sources: &[SourceRecord],
    view: Option<&StudyViewResponse>,
    jobs: &[GenerationJob],
    notice: Option<&str>,
) -> String {
    let inner = render_signed_in(
        account,
        sources,
        view,
        jobs,
        notice,
        SignedInSurface::Library,
    );
    document(&inner)
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum AnalyticsConceptFilter {
    #[default]
    All,
    AtRisk,
    Struggling,
    Mixed,
    Solid,
    Untried,
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub enum AnalyticsConceptSort {
    #[default]
    Health,
    Name,
    Success,
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct AnalyticsViewOptions {
    pub filter: AnalyticsConceptFilter,
    pub sort: AnalyticsConceptSort,
    pub page: usize,
}

/// Render the focused, bounded Analytics read surface. The API route owns
/// authentication and query parsing; this crate owns the presentation policy
/// so the default ordering and page window cannot drift between callers.
#[must_use]
pub fn render_analytics_page(
    account: &AppAccount,
    view: &StudyViewResponse,
    options: AnalyticsViewOptions,
) -> String {
    let due = view.due_count;
    let header_right = format!(
        r#"<span class="me-due">{due} due</span>{account_menu}"#,
        account_menu = account_menu(account),
    );
    let footer = render_nav("analytics");
    let body = format!(
        r#"<section class="me-analytics">
<h1 class="me-display me-analytics-title">Progress</h1>
<p class="ae-lede ae-dim me-analytics-support">See what is becoming familiar and which concepts need another pass. These are your recorded reviews, not a score to chase.</p>
{surface}
</section>"#,
        surface = render_concept_health_surface(&view.concept_progress, options),
    );
    document(&screen(&header_right, &body, &footer))
}

#[must_use]
pub fn render_entry_requested(debug_link: Option<&str>) -> String {
    let (heading, delivery) = if debug_link.is_some() {
        (
            "Open your sign-in link",
            "For this local session, use the link below or the configured local outbox. Local outbox mode does not send email.",
        )
    } else {
        (
            "Check your email",
            "If you’re invited, a sign-in link is on the way. Otherwise, your place on the waitlist is saved and we’ll email when a spot opens.",
        )
    };
    let debug = debug_link.map_or_else(String::new, |link| {
        format!(
            r#"<p><a href="{}" class="ae-accent">Open sign-in link</a></p>"#,
            escape_html(link)
        )
    });
    let view = format!(
        r#"<div class="me-cover">
<h1 class="me-display">{heading}</h1>
<p class="ae-lede ae-dim me-support">{delivery}</p>
<p class="me-entry-explainer">Use the newest link to open your study space. You do not need a password.</p>
{debug}
<p><a class="ae-accent" href="/">Back to start</a></p>
</div>"#
    );
    document(&screen_centered("", &view, FOOTER_TAGLINE))
}

#[must_use]
pub fn render_entry_recovery(title: &str, message: &str) -> String {
    render_email_recovery("Access to Scry", title, message, "Try again")
}

#[must_use]
pub fn render_auth_recovery(title: &str, message: &str) -> String {
    render_email_recovery("Sign in to Scry", title, message, "Request a new link")
}

fn render_email_recovery(kicker: &str, title: &str, message: &str, action: &str) -> String {
    let view = format!(
        r#"<div class="me-cover">
<p class="me-kicker">{}</p>
<h1 class="me-display">{}</h1>
<p class="ae-lede ae-dim me-support" role="alert">{}</p>
<section class="ae-group me-capture-hero">
<form action="/app/account" method="post">
<label class="ae-label" for="me-recovery-email">Your email</label>
<input class="ae-input me-hero-email" id="me-recovery-email" name="email" type="email" autocomplete="email" required placeholder="you@example.com" aria-label="Email address">
<div class="me-actions"><button class="ae-button" type="submit">{}</button></div>
</form>
</section>
<p><a class="ae-accent" href="/">Back to start</a></p>
</div>"#,
        escape_html(kicker),
        escape_html(title),
        escape_html(message),
        escape_html(action),
    );
    document(&screen_centered("", &view, FOOTER_TAGLINE))
}
#[must_use]
pub fn render_submit_recovery(title: &str, message: &str) -> String {
    let view = format!(
        r#"<div class="me-cover">
<p class="me-kicker">Review safely</p>
<h1 class="me-display">{}</h1>
<p class="ae-lede ae-dim me-support">{}</p>
<p><a class="ae-button" href="/">Return to your review</a></p>
</div>"#,
        escape_html(title),
        escape_html(message),
    );
    document(&screen_centered("", &view, FOOTER_TAGLINE))
}

#[must_use]
pub fn render_return_notification_confirmation(token: &str) -> String {
    let view = format!(
        r#"<div class="me-cover">
<h1 class="me-display">Turn off study reminders?</h1>
<p class="ae-lede ae-dim me-support">This stops reminder emails for the address that received this link. Your sources, quizzes, and review history stay unchanged.</p>
<section class="ae-group me-capture-hero">
<form action="/app/return-notifications" method="post">
<input type="hidden" name="unsubscribeToken" value="{}">
<div class="me-actions"><button class="ae-button" type="submit">Turn off reminders</button></div>
</form>
</section>
<p><a class="ae-accent" href="/">Keep reminders on</a></p>
</div>"#,
        escape_html(token)
    );
    document(&screen_centered("", &view, FOOTER_TAGLINE))
}

#[must_use]
pub fn render_return_notification_disabled() -> String {
    let view = r#"<div class="me-cover">
<h1 class="me-display">Reminders are off</h1>
<p class="ae-lede ae-dim me-support">You will not receive more study reminders. You can turn them on again from Home.</p>
<p><a class="ae-accent" href="/">Back to Scry</a></p>
</div>"#;
    document(&screen_centered("", view, FOOTER_TAGLINE))
}

#[must_use]
pub fn render_return_notification_recovery(title: &str, message: &str) -> String {
    let view = format!(
        r#"<div class="me-cover">
<h1 class="me-display">{}</h1>
<p class="ae-lede ae-dim me-support" role="alert">{}</p>
<p><a class="ae-accent" href="/">Back to Scry</a></p>
</div>"#,
        escape_html(title),
        escape_html(message),
    );
    document(&screen_centered("", &view, FOOTER_TAGLINE))
}

/// Wrap a `.ae-screen` body in the full document, linking the design system.
fn document(inner: &str) -> String {
    document_with_head(inner, "")
}

fn document_with_head(inner: &str, head: &str) -> String {
    format!(
        r##"<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<meta name="referrer" content="no-referrer">
<meta name="theme-color" content="#f4f8fa" media="(prefers-color-scheme: light)">
<meta name="theme-color" content="#0f2430" media="(prefers-color-scheme: dark)">
<meta name="mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-status-bar-style" content="default">
<title>Scry</title>
<link rel="manifest" href="/manifest.webmanifest">
<link rel="icon" href="/favicon.png" type="image/png">
<link rel="apple-touch-icon" href="/apple-touch-icon.png" sizes="180x180">
<link rel="preload" href="/static/fonts/manrope-latin-variable.woff2" as="font" type="font/woff2" crossorigin>
<link rel="preload" href="/static/fonts/literata-latin-variable.woff2" as="font" type="font/woff2" crossorigin>
<link rel="stylesheet" href="/static/ledger.css">
{head}
<script src="/static/app.js" defer></script>
</head>
<body>
{inner}
</body>
</html>"##
    )
}

/// Shared chrome: a scrollable document with a thumb-reachable standing-view
/// dock. The study loop has no persistent navigation competing with the Quiz.
fn screen(header_right: &str, view: &str, footer: &str) -> String {
    screen_with("ae-stage ae-stage-scroll", header_right, view, footer)
}

/// A short, focused screen (sign-in, check-email) whose content sits centered.
fn screen_centered(header_right: &str, view: &str, footer: &str) -> String {
    screen_with("ae-stage", header_right, view, footer)
}

fn screen_with(stage: &str, header_right: &str, view: &str, footer: &str) -> String {
    format!(
        r##"<div class="ae-screen">
<a class="me-skip-link" href="#me-main">Skip to content</a>
<header class="ae-bar">
<a class="ae-name" href="/" aria-label="Scry home">Scry</a>
{header_right}
</header>
<main class="{stage}" id="me-main" tabindex="-1">
<div class="ae-view">
{view}
</div>
</main>
<footer class="ae-bar">
{footer}
</footer>
</div>"##
    )
}

fn render_signed_out(notice: Option<&str>) -> String {
    let view = format!(
        r#"<div class="me-cover">
{notice}
<div class="me-cover-intro">
<h1 class="me-display">Remember what matters to you.</h1>
<p class="ae-lede ae-dim me-support">A word, an idea, a whole essay. We turn it into useful questions and bring them back when you need them.</p>
</div>
<section class="ae-group me-capture-hero" aria-label="Sign in or join the waitlist">
<form class="me-entry-form" action="/app/account" method="post">
<label class="ae-label" for="me-email">Your email</label>
<input class="ae-input me-hero-email" id="me-email" name="email" type="email" autocomplete="email" required placeholder="you@example.com" aria-label="Email address">
<div class="me-actions"><button class="ae-button" type="submit">Email me a sign-in link</button></div>
<p class="me-entry-explainer">Scry is invite-only. If a place is not available yet, this saves your place on the waitlist. No password needed.</p>
<p class="me-entry-status ae-dim" role="status" aria-live="polite"></p>
</form>
</section>
</div>"#,
        notice = render_notice(notice, &[]),
    );
    screen("", &view, FOOTER_TAGLINE)
}

#[derive(Clone, Copy)]
enum SignedInSurface {
    Home,
    Create,
    Library,
    Edit,
    ReviewComplete,
}

fn render_signed_in(
    account: &AppAccount,
    sources: &[SourceRecord],
    view: Option<&StudyViewResponse>,
    jobs: &[GenerationJob],
    notice: Option<&str>,
    surface: SignedInSurface,
) -> String {
    let due = view.map_or(0, |view| view.due_count);
    match surface {
        SignedInSurface::Edit => {
            let body = view.and_then(|view| view.current.as_ref()).map_or_else(
                || render_home_body(account, view),
                |current| render_edit_review(account, current),
            );
            render_signed_in_body(account, due, notice, jobs, &body, None)
        }
        SignedInSurface::ReviewComplete => {
            let body = render_home_body(account, view);
            render_signed_in_body(account, due, notice, jobs, &body, Some("home"))
        }
        SignedInSurface::Home => {
            if let Some(current) = view.and_then(|view| view.current.as_ref()) {
                // The question owns the screen; management stays in the menu.
                let body = render_current_review(account, current);
                render_signed_in_body(account, due, notice, jobs, &body, None)
            } else {
                let body = render_home_body(account, view);
                render_signed_in_body(account, due, notice, jobs, &body, Some("home"))
            }
        }
        SignedInSurface::Create => {
            let body = format!("{}{}", render_capture(account), render_create_status(jobs));
            render_signed_in_body(account, due, notice, jobs, &body, Some("create"))
        }
        SignedInSurface::Library => {
            let body = render_library_body(account, sources, view, jobs);
            render_signed_in_body(account, due, notice, jobs, &body, Some("library"))
        }
    }
}

fn render_signed_in_body(
    account: &AppAccount,
    due: usize,
    notice: Option<&str>,
    jobs: &[GenerationJob],
    body: &str,
    nav_active: Option<&str>,
) -> String {
    // Navigation is secondary to the question. Adding material stays one tap away.
    let header_right = format!(
        r#"<span class="me-due">{due} due</span><a class="me-add" href="/app/create" aria-label="Learn something new">{ICON_PLUS}<span>Add</span></a>{account_menu}"#,
        account_menu = account_menu(account),
    );
    let footer = match nav_active {
        Some(active) => render_nav(active),
        None => String::new(),
    };
    let view_inner = format!("{}{}", render_notice(notice, jobs), body);
    screen(&header_right, &view_inner, &footer)
}

/// Two destinations: practice and add. Management lives in the account menu.
fn render_nav(active: &str) -> String {
    let item = |label: &str, href: &str, key: &str| {
        let current = if key == active {
            r#" aria-current="page""#
        } else {
            ""
        };
        format!(r#"<a class="me-nav-item" href="{href}"{current}>{label}</a>"#)
    };
    format!(
        r#"<nav class="me-nav" aria-label="Study">{}{}</nav>"#,
        item("Review", "/", "home"),
        item("Add something", "/app/create", "create"),
    )
}

fn render_edit_review(account: &AppAccount, current: &BetaStudyCurrent) -> String {
    format!(
        r#"<section class="ae-group me-edit">
<h1 class="me-display">Edit quiz</h1>
<form class="me-edit-form" action="/app/edit/save" method="post">
{csrf}
<input type="hidden" name="reviewUnitId" value="{id}">
<label class="ae-label" for="me-edit-prompt">Question</label>
<textarea class="ae-input" id="me-edit-prompt" name="prompt" rows="4" required>{prompt}</textarea>
<label class="ae-label" for="me-edit-answer">Answer</label>
<input class="ae-input" id="me-edit-answer" name="expectedAnswer" value="{answer}" required autocomplete="off">
<div class="me-actions"><button class="ae-button" type="submit">Save changes</button></div>
</form>
<form class="me-resume" action="/app/resume" method="post">{csrf}<input type="hidden" name="reviewUnitId" value="{id}"><button class="ae-button-quiet" type="submit">Cancel</button></form>
</section>"#,
        csrf = hidden_csrf_input(account),
        id = escape_html(&current.review_unit_id.to_string()),
        prompt = escape_html(&current.prompt),
        answer = escape_html(&current.revision_expected_answer),
    )
}

fn render_home_body(account: &AppAccount, view: Option<&StudyViewResponse>) -> String {
    let mut html = String::new();
    if let Some(view) = view {
        html.push_str(&render_review_status(account, view));
    }
    html.push_str(&render_capture(account));
    html
}

/// Saved topics and source controls, not an approval inbox.
fn render_library_body(
    account: &AppAccount,
    sources: &[SourceRecord],
    view: Option<&StudyViewResponse>,
    jobs: &[GenerationJob],
) -> String {
    let library = view.map_or(&[][..], |v| v.library.as_slice());
    let mut html = String::from(
        r#"<header class="me-page-heading"><h1 class="me-display">Your learning</h1><p class="ae-lede">Everything you’ve added. Your quizzes are already in the review schedule.</p></header>"#,
    );
    html.push_str(&render_library_sources(account, sources, library, jobs));
    if let Some(view) = view {
        for notice in &view.generation_notices {
            let _ = write!(
                html,
                r#"<p class="me-notice" role="status">{ICON_INFO}<span>{}</span></p>"#,
                escape_html(notice),
            );
        }
    }
    html.push_str(&render_jobs(account, jobs));
    html
}

/// Saved material with per-source active-card counts and concept drilldown
/// (memory-engine-087). Replaces the flat source list from the workspace —
/// each source row shows its card count and a collapsible concept breakdown
/// so duplicates and gaps are visible at a glance.
fn render_library_sources(
    account: &AppAccount,
    sources: &[SourceRecord],
    library: &[LibrarySourceRow],
    jobs: &[GenerationJob],
) -> String {
    if sources.is_empty() {
        return r#"<section class="ae-group"><h2 class="ae-h">Nothing here yet</h2><p class="ae-lede">Start with a word, an idea, or something you’ve read.</p><a class="ae-button" href="/app/create">Add something to learn</a></section>"#.to_owned();
    }
    let mut rows = String::new();
    for source in sources {
        let lib = library.iter().find(|row| row.source_id == source.source_id);
        let card_count = lib.map_or(0, |row| row.active_card_count);
        let count_line = format!(
            r#"<span class="ae-dim me-source-count">{card_count} {cards}</span>"#,
            cards = plural(card_count, "quiz", "quizzes"),
        );
        let concept_detail = lib.map_or_else(String::new, |row| {
            if row.concepts.is_empty() {
                return String::new();
            }
            let items = row.concepts.iter().fold(String::new(), |mut acc, concept| {
                let _ = write!(
                    acc,
                    r#"<li class="me-concept-item"><span>{}</span><span class="me-concept-count">{} {}</span></li>"#,
                    escape_html(&concept.concept_label),
                    concept.card_count,
                    plural(concept.card_count, "quiz", "quizzes"),
                );
                acc
            });
            format!(
                r#"<details class="me-concepts"><summary class="me-concepts-toggle">{count} {concepts}</summary><ul class="me-concepts-list">{items}</ul></details>"#,
                count = row.concepts.len(),
                concepts = plural(row.concepts.len(), "concept", "concepts"),
            )
        });
        let generate = if source_generation_in_progress_or_done(source, jobs) {
            String::new()
        } else {
            format!(
                r#"<form action="/app/generate" method="post">{csrf_generate}<input type="hidden" name="sourceId" value="{id}"><button class="ae-button ae-button-compact" type="submit">Generate quizzes</button></form>"#,
                csrf_generate = hidden_csrf_input(account),
                id = escape_html(&source.source_id),
            )
        };
        let permission = match &source.permission {
            SourcePermission::LocalOnly => {
                "<span class=\"ae-dim me-source-permission\">Local only. This source is not sent to a model.</span>"
            }
            SourcePermission::ModelEligible => {
                "<span class=\"ae-dim me-source-permission\">Model help allowed for this source.</span>"
            }
        };
        let edit_permission = format!(
            r#"<form class="me-source-permission" action="/app/source/permission" method="post">{csrf}<input type="hidden" name="sourceId" value="{id}"><label class="ae-label" for="permission-{id_label}">Model permission</label><select class="ae-input" id="permission-{id_label}" name="permission" aria-label="Model permission for {title_attr}"><option value="model-eligible" {model_selected}>Allow model help</option><option value="local-only" {local_selected}>Keep local only</option></select><p class="me-hint">This controls future model use. It does not undo a request already sent.</p><button class="ae-button-quiet ae-button-compact" type="submit">Save permission</button></form>"#,
            csrf = hidden_csrf_input(account),
            id = escape_html(&source.source_id),
            id_label = escape_html(&source.source_id),
            title_attr = escape_html(&source.title),
            model_selected = if source.permission == SourcePermission::ModelEligible {
                "selected"
            } else {
                ""
            },
            local_selected = if source.permission == SourcePermission::LocalOnly {
                "selected"
            } else {
                ""
            },
        );
        let _ = write!(
            rows,
            r#"<article class="me-source">
<header class="me-source-header"><h3 class="me-source-title">{title}</h3>{count_line}</header>
{concept_detail}
<details class="me-source-settings">
<summary>Source settings</summary>
<div class="me-source-settings-panel">
{permission}
{edit_permission}
<div class="me-row-actions">
{generate}
<details class="me-remove-confirm">
<summary>Remove source</summary>
<p class="me-remove-warning">This removes the source from Library and stops all quizzes generated from it, across every generation run, from appearing in review.</p>
<form action="/app/source/archive" method="post">{csrf_archive}<input type="hidden" name="sourceId" value="{id_archive}"><button class="ae-button-quiet ae-button-compact" type="submit">Remove source and quizzes</button></form>
</details>
</div>
</div>
</details>
</article>"#,
            title = escape_html(&source.title),
            permission = permission,
            csrf_archive = hidden_csrf_input(account),
            id_archive = escape_html(&source.source_id),
        );
    }
    format!(
        r#"<section class="ae-group me-material"><h2 class="ae-h">Sources</h2>{rows}</section>"#
    )
}

fn render_return_notifications(account: &AppAccount) -> String {
    // Collapsed by default so Home stays calm: one due-hero primary action
    // in the open view, reminders one tap deeper (operator dogfood).
    format!(
        r#"<section class="ae-group me-return-channel" id="me-reminders">
<details class="me-return-details">
<summary class="me-return-summary">Daily reminders</summary>
<p class="ae-lede">Opt in to one quiet email a day when reviews are waiting. No scores or promotional mail.</p>
<form class="me-return-enable" action="/app/return-notifications" method="post">
{csrf}<label class="ae-label" for="me-reminder-email">Reminder email</label>
<input class="ae-input" id="me-reminder-email" name="reminderEmail" type="email" autocomplete="email" required placeholder="you@example.com">
<input type="hidden" name="enabled" value="on">
<button class="ae-button" type="submit">Turn on reminders</button>
</form>
<form class="me-return-off" action="/app/return-notifications" method="post">
{csrf}<input type="hidden" name="enabled" value="off">
<button class="ae-button-quiet" type="submit">Turn off reminders</button>
</form>
</details>
</section>"#,
        csrf = hidden_csrf_input(account),
    )
}

fn render_notice(message: Option<&str>, jobs: &[GenerationJob]) -> String {
    let message = message.filter(|text| generating_notice_is_live(text, jobs));
    message.map_or_else(String::new, |message| {
        format!(
            r#"<p class="me-notice" role="status" aria-live="polite">{ICON_INFO}<span>{}</span></p>"#,
            escape_html(message)
        )
    })
}

/// A "Generating…" notice only tells the truth while a job it could be
/// describing is actually queued or running. Every other notice (errors,
/// confirmations like "Source removed.") is unconditional — only a
/// generation-in-progress notice needs to be checked against live job state,
/// so it never lingers once nothing is left in flight (operator dogfood
/// finding, memory-engine-081).
#[must_use]
pub fn is_generating_notice(text: &str) -> bool {
    text.contains("Generating")
}

fn generating_notice_is_live(text: &str, jobs: &[GenerationJob]) -> bool {
    if !is_generating_notice(text) {
        return true;
    }
    jobs.iter().any(|job| {
        matches!(
            job.status,
            JobStatus::Queued | JobStatus::Running | JobStatus::Retry
        )
    })
}

fn render_review_status(account: &AppAccount, view: &StudyViewResponse) -> String {
    if view.due_count > 0 {
        // Recovery for callers without a selected occurrence; normal entry
        // selects the question at the server before rendering.
        return format!(
            r#"<form class="me-next" action="/app/next" method="post">{csrf}<button class="ae-button" type="submit">Back to your next question</button></form>"#,
            csrf = hidden_csrf_input(account),
        );
    }
    if view.summary.approved_review_unit_count > 0 {
        return format!(
            r#"<section class="me-caughtup"><p class="me-caughtup-line">{ICON_OK}<span>You’re caught up.</span></p><p class="ae-lede">Your next review will be here when it’s time. Want to learn something else?</p></section>"#
        );
    }
    String::new()
}

fn render_capture(account: &AppAccount) -> String {
    // One input is enough; validated quizzes are scheduled in the background.
    format!(
        r#"<section class="ae-group me-capture">
<h1 class="me-display">What do you want to know?</h1>
<p class="ae-lede">One word or a whole essay. We’ll take it from here.</p>
<form class="me-capture-form" action="/app/capture" method="post">
{csrf}
<label class="ae-label me-capture-label" for="me-capture">Anything you want to learn</label>
<textarea class="ae-input" id="me-capture" name="capture" rows="5" required maxlength="65536" aria-describedby="me-capture-help" placeholder="Photosynthesis. First principles thinking. Or paste something you’ve been reading."></textarea>
<div class="me-actions"><button class="ae-button" type="submit">Learn this</button><span class="me-live-hint" role="status" aria-live="polite"></span></div>
<p class="me-hint" id="me-capture-help">AI turns your text into quizzes. Don’t include anything you don’t want sent to a model. Up to 64 KB.</p>
</form>
</section>"#,
        csrf = hidden_csrf_input(account),
    )
}

fn render_create_status(jobs: &[GenerationJob]) -> String {
    let Some(job) = jobs.iter().find(|job| job.status != JobStatus::Succeeded) else {
        return String::new();
    };
    format!(
        r#"<p class="me-capture-next me-hint"><a href="/app/capture?jobId={}">Your saved request: {}</a></p>"#,
        escape_html(&job.id),
        job_meta(job),
    )
}

/// Do not offer duplicate generation while a request is live or completed.
fn source_generation_in_progress_or_done(source: &SourceRecord, jobs: &[GenerationJob]) -> bool {
    jobs.iter().any(|job| {
        job.source_id == source.source_id
            && matches!(
                job.status,
                JobStatus::Queued | JobStatus::Running | JobStatus::Retry | JobStatus::Succeeded
            )
    })
}

/// The activity log: one row per background generation job, newest first.
///
/// Server-rendered and authoritative on every full load (works with JS off);
/// `app.js` enhances it to update live over SSE. Each row carries
/// `data-job-id` + `data-status` so the script can patch a single row in place,
/// and CSS drives the status glyph + retry visibility off `data-status`.
fn render_jobs(account: &AppAccount, jobs: &[GenerationJob]) -> String {
    if jobs.is_empty() {
        return String::new();
    }
    let mut rows = String::new();
    for job in jobs {
        rows.push_str(&render_job_row(account, job));
    }
    format!(
        r#"<details class="ae-group me-jobs"><summary>Recent activity</summary><ul id="me-jobs" class="me-jobs-list" aria-live="polite" aria-relevant="text">{rows}</ul></details>"#
    )
}

fn render_job_row(account: &AppAccount, job: &GenerationJob) -> String {
    format!(
        r#"<li class="me-job" data-job-id="{id}" data-status="{status}">
<span class="me-job-glyphs" aria-hidden="true"><span class="g-queued"></span><span class="g-running"><span class="me-spinner"></span></span><span class="g-succeeded"></span><span class="g-failed"></span></span>
<div class="me-job-body">
<p class="me-job-title">{title}</p>
<p class="me-job-meta">{meta}</p>
</div>
{retry}
</li>"#,
        id = escape_html(&job.id),
        status = job.status.as_str(),
        title = escape_html(&job.title),
        meta = job_meta(job),
        retry = render_job_retry(account, job),
    )
}

/// Live updates may reveal this server-authorized control, never invent a token.
fn render_job_retry(account: &AppAccount, job: &GenerationJob) -> String {
    if job.status == JobStatus::Succeeded || (job.status == JobStatus::Failed && !job.retryable) {
        return String::new();
    }
    format!(
        r#"<form class="me-job-retry" action="/app/jobs/retry" method="post"{hidden}>{csrf}<input type="hidden" name="jobId" value="{id}"><button class="me-job-retry-btn" type="submit">Try again</button></form>"#,
        csrf = hidden_csrf_input(account),
        id = escape_html(&job.id),
        hidden = if job.status == JobStatus::Failed {
            ""
        } else {
            " hidden"
        },
    )
}

/// The human meta line for a job's current status. Kept in sync with the
/// `metaFor` switch in `app.js`, which recomputes it on each SSE update.
fn job_meta(job: &GenerationJob) -> String {
    match job.status {
        JobStatus::Queued => "Queued for generation.".to_owned(),
        JobStatus::Running => "Generating quizzes…".to_owned(),
        JobStatus::Retry => "Retrying after a temporary failure…".to_owned(),
        JobStatus::Succeeded => "Your quizzes are ready and scheduled.".to_owned(),
        JobStatus::Failed => escape_html(
            job.error
                .as_deref()
                .unwrap_or("Generation failed. Try again."),
        ),
    }
}

enum ReviewCard<'a> {
    Answering(&'a BetaStudyCurrent),
    Graded(GradedReviewCard<'a>),
}

struct GradedReviewCard<'a> {
    current: &'a BetaStudyCurrent,
    grade: &'a BetaStudyGrade,
    accepted_answer: &'a str,
    dossier: GradedDossier<'a>,
}

enum GradedDossier<'a> {
    Complete(&'a BetaStudyFeedback),
    HistoryUnavailable,
}

impl<'a> ReviewCard<'a> {
    fn project(current: &'a BetaStudyCurrent) -> Self {
        let Some(grade) = current.grade.as_ref() else {
            return Self::Answering(current);
        };
        let accepted_answer = current
            .expected_answer
            .as_deref()
            .or_else(|| {
                current
                    .feedback
                    .as_ref()
                    .map(|feedback| feedback.expected_answer.as_str())
            })
            .unwrap_or(&current.revision_expected_answer);
        let dossier = current
            .feedback
            .as_ref()
            .map_or(GradedDossier::HistoryUnavailable, GradedDossier::Complete);
        Self::Graded(GradedReviewCard {
            current,
            grade,
            accepted_answer,
            dossier,
        })
    }
}

fn render_current_review(account: &AppAccount, current: &BetaStudyCurrent) -> String {
    match ReviewCard::project(current) {
        ReviewCard::Answering(card) => render_answering(account, card),
        ReviewCard::Graded(card) => render_graded_review(account, &card),
    }
}

/// Before grading: the prompt and the answer mechanism (clickable options or a
/// free-response box), plus the reveal and the escape hatches. The question owns
/// the screen — nothing reflective competes with it. If the learner used "Reveal
/// answer", the answer shows in place (the reveal button is then gone) while they
/// can still answer.
fn render_answering(account: &AppAccount, current: &BetaStudyCurrent) -> String {
    let revealed = current
        .expected_answer
        .as_deref()
        .map_or_else(String::new, |answer| {
            format!(
                r#"<p class="me-answer"><span class="me-answer-label">Revealed answer</span><span class="ae-item">{}</span></p><p class="me-assistance-note">This review now counts as assisted practice, even if you answer correctly.</p>"#,
                escape_html(answer)
            )
        });
    format!(
        r#"<section class="me-review" aria-labelledby="me-question">
<h1 class="me-prompt" id="me-question">{prompt}</h1>
{revealed}
{answer_block}
<p data-review-status role="status" aria-live="polite" aria-atomic="true"></p>
<div class="me-hatch-row">
{reveal}
{escape_hatches}
</div>
</section>"#,
        prompt = escape_html(&current.prompt),
        answer_block = render_answer_block(account, current),
        reveal = render_reveal_form(account, current),
        escape_hatches = render_escape_hatches(account, current),
    )
}

/// The graded card keeps only the immediate learning loop in the default view.
/// Reflective history and generated-card controls remain available in one
/// collapsed disclosure after the deliberate Continue action.
fn render_graded_review(account: &AppAccount, card: &GradedReviewCard<'_>) -> String {
    format!(
        r#"<section class="me-review" aria-labelledby="me-question">
<h1 class="me-prompt" id="me-question">{prompt}</h1>
{verdict}
{reveal}
{reason}
{bridge}
{next}
{details}
<p data-review-status role="status" aria-live="polite" aria-atomic="true"></p>
</section>"#,
        prompt = escape_html(&card.current.prompt),
        verdict = render_verdict(card.grade),
        reveal = render_answer_reveal(card),
        reason = render_grading_reason(card.grade),
        bridge = render_bridge_message(card),
        next = render_next(account),
        details = render_graded_details(account, card),
    )
}

fn render_grading_reason(grade: &BetaStudyGrade) -> String {
    let reason = match verdict_label(grade.verdict) {
        "Correct" => "Your answer matches the accepted answer.",
        "Close" => "Your answer is close, but it misses part of the accepted answer.",
        "Try again" => "Your answer does not match the accepted answer.",
        "Revealed" => "You revealed the answer. This is assisted practice, not independent recall.",
        _ => "This answer needs review.",
    };
    format!(r#"<p class="me-grade-reason">{}</p>"#, escape_html(reason))
}

fn render_bridge_message(card: &GradedReviewCard<'_>) -> String {
    let GradedDossier::Complete(feedback) = card.dossier else {
        return String::new();
    };
    if !feedback.remediation_drafts_pending {
        return String::new();
    }
    r#"<p class="me-bridge">We’re making the next practice a little easier.</p>"#.to_owned()
}

fn render_graded_details(account: &AppAccount, card: &GradedReviewCard<'_>) -> String {
    format!(
        r#"<details class="me-dossier">
<summary class="me-dossier-summary">Details</summary>
<div class="me-dossier-panel">
{history}
{reference}
{content_feedback}
</div>
</details>"#,
        history = render_dossier_history(&card.dossier),
        reference = render_reference(account, card.current),
        content_feedback = render_content_feedback(account, card),
    )
}

fn render_dossier_history(dossier: &GradedDossier<'_>) -> String {
    let GradedDossier::Complete(feedback) = dossier else {
        return r#"<p class="me-dossier-empty">Review history is unavailable for this result.</p>"#
            .to_owned();
    };
    let horizon = if feedback.item_history.next_review.is_empty() {
        String::new()
    } else {
        format!(
            r#"<p class="me-next-when">{}</p>"#,
            escape_html(&feedback.item_history.next_review)
        )
    };
    format!(
        "{horizon}{concept}{ledger}",
        concept = render_concept_dossier(feedback),
        ledger = render_meta_ledger(feedback),
    )
}

fn render_concept_dossier(feedback: &BetaStudyFeedback) -> String {
    feedback
        .concept_progress
        .as_ref()
        .map_or_else(String::new, |concept| {
            format!(
                r#"<section class="me-dossier-concept">
<h2 class="me-dossier-label">Concept</h2>
<p class="me-dossier-value">{label}</p>
<p class="me-dossier-note">{health}</p>
</section>"#,
                label = escape_html(&concept.concept_label),
                health = escape_html(&concept.health),
            )
        })
}
/// Capture a binary judgment about the generated card after the learner has
/// seen its answer. The stable id makes an accidental double submit a replay,
/// while a later client can supply `supersedesId` for a revised judgment.
fn render_content_feedback(account: &AppAccount, card: &GradedReviewCard<'_>) -> String {
    let current = card.current;
    let reps = current.review_state.as_ref().map_or(0, |state| state.reps);
    let head_id = current.content_feedback_head_id.as_deref().unwrap_or("new");
    let feedback_id = format!("feedback-{}-{reps}-{head_id}", current.review_unit_id);
    let supersedes = current
        .content_feedback_head_id
        .as_ref()
        .map(|head_id| {
            format!(
                r#"<input type="hidden" name="supersedesId" value="{}">"#,
                escape_html(head_id)
            )
        })
        .unwrap_or_default();
    format!(
        r#"<section class="me-content-feedback" aria-labelledby="me-content-feedback-title">
<h2 class="ae-h" id="me-content-feedback-title">Quiz quality</h2>
<p class="ae-dim">Is this quiz useful and accurate? Feedback saves here without advancing your review.</p>
<form action="/app/content-feedback" method="post">
{csrf}<input type="hidden" name="reviewUnitId" value="{review_unit_id}">
<input type="hidden" name="idempotencyKey" value="{feedback_id}">
{supersedes}
<label class="ae-label" for="me-content-feedback-rationale">What would improve it? <span class="ae-dim">(optional)</span></label>
<textarea class="ae-input me-content-feedback-rationale" id="me-content-feedback-rationale" name="rationale" rows="2"></textarea>
<div class="me-feedback-actions">
<button class="ae-button-quiet" type="submit" name="verdict" value="kept">Keep this quiz</button>
<button class="ae-button-quiet" type="submit" name="verdict" value="dropped">Drop this quiz</button>
</div>
</form>
</section>"#,
        csrf = hidden_csrf_input(account),
        review_unit_id = escape_html(&current.review_unit_id.to_string()),
        feedback_id = escape_html(&feedback_id),
        supersedes = supersedes,
    )
}

fn render_meta_ledger(feedback: &BetaStudyFeedback) -> String {
    let history = &feedback.item_history;
    let mut rows = String::new();
    let _ = write!(
        rows,
        r"<div><dt>Stage</dt><dd>{}</dd></div>",
        escape_html(&history.stage)
    );
    let _ = write!(
        rows,
        r"<div><dt>Last seen</dt><dd>{}</dd></div>",
        escape_html(&history.last_seen_summary)
    );
    let _ = write!(
        rows,
        r"<div><dt>Recall history</dt><dd>{}<br>{}</dd></div>",
        escape_html(&history.success_rate),
        escape_html(&history.trend)
    );
    format!(r#"<dl class="me-meta-ledger">{rows}</dl>"#)
}

/// Reveal the accepted answer in place. Multiple-choice keeps presentation
/// order, marks the correct option, and dims the rest.
fn render_answer_reveal(card: &GradedReviewCard<'_>) -> String {
    if card.current.choices.is_empty() {
        return format!(
            r#"<p class="me-answer"><span class="me-answer-label">Accepted answer</span><span class="ae-item">{}</span></p>"#,
            escape_html(card.accepted_answer)
        );
    }

    let mut rows = String::new();
    for choice in &card.current.choices {
        let is_correct = choice == card.accepted_answer;
        let class = if is_correct {
            "me-graded-choice me-graded-choice-correct"
        } else {
            "me-graded-choice me-graded-choice-dim"
        };
        let mark = if is_correct {
            r#"<span class="me-answer-label">Accepted answer</span>"#
        } else {
            ""
        };
        let _ = write!(
            rows,
            r#"<li class="{class}"><span>{}</span>{mark}</li>"#,
            escape_html(choice)
        );
    }
    format!(r#"<ol class="me-choices me-choices-graded">{rows}</ol>"#)
}

/// The answer mechanism, chosen by card shape. Pre-grade, multiple-choice cards
/// get clickable option buttons (no typing, no letter-guessing) and free-
/// response cards get a prominent text box. Post-grade, multiple-choice cards
/// show a read-only recap of the options.
fn render_answer_block(account: &AppAccount, current: &BetaStudyCurrent) -> String {
    if current.choices.is_empty() {
        render_free_response_form(account, current)
    } else {
        render_choice_buttons(account, current)
    }
}

/// Clickable multiple-choice options. Each button submits its exact choice text,
/// so grading matches without the learner typing or mapping an option letter.
fn render_choice_buttons(account: &AppAccount, current: &BetaStudyCurrent) -> String {
    let mut buttons = String::new();
    for choice in &current.choices {
        let _ = write!(
            buttons,
            r#"<button class="me-choice" type="submit" name="answer" value="{value}">{label}</button>"#,
            value = escape_html(choice),
            label = escape_html(choice),
        );
    }
    format!(
        r#"<form class="me-choices-form" action="/app/submit" method="post" aria-labelledby="me-question">{hidden}{buttons}</form>"#,
        hidden = review_submit_fields(account, current),
    )
}

/// Free-response answer field. This is the whole interaction for a non-MCQ card,
/// so it is a clearly bounded, labelled box — not the hairline underline that
/// read as a divider.
fn render_free_response_form(account: &AppAccount, current: &BetaStudyCurrent) -> String {
    format!(
        r#"<form class="me-submit" action="/app/submit" method="post">{hidden}
<label class="ae-label" for="me-answer">Your answer</label>
<input class="ae-input me-answer-input" id="me-answer" name="answer" required autocomplete="off" autocapitalize="off" placeholder="Type your answer…">
<div class="me-actions"><button class="ae-button" type="submit">Check answer</button></div>
</form>"#,
        hidden = review_submit_fields(account, current),
    )
}

/// The hidden fields every `/app/submit` form carries.
fn review_submit_fields(account: &AppAccount, current: &BetaStudyCurrent) -> String {
    // The idempotency key must be unique per review *attempt*, not per card: a
    // card is reviewed many times over its life, and applied-review receipts are
    // persisted, so a key keyed only on the review unit id collides on the
    // second-ever review ("Duplicate applied review"). The rep count is the
    // per-attempt discriminator — stable across an accidental double-submit of
    // one answer (so that stays idempotent), and incremented before the card is
    // shown again — so each attempt gets its own key. Fresh card: reps 0.
    // The browser fills response time from the actual presentation clock.
    // Missing timing stays conservative; timing never infers self-assessed ease.
    let reps = current.review_state.as_ref().map_or(0, |state| state.reps);
    format!(
        r#"{csrf}
<input type="hidden" name="reviewUnitId" value="{id}">
<input type="hidden" name="responseTimeMs" value="">
<input type="hidden" name="idempotencyKey" value="review-{id}-{reps}">"#,
        csrf = hidden_csrf_input(account),
        id = escape_html(&current.review_unit_id.to_string()),
    )
}

fn render_reference(account: &AppAccount, current: &BetaStudyCurrent) -> String {
    format!(
        r#"<section class="me-reference"><h2 class="ae-h">Study note</h2>{action}</section>"#,
        action = render_review_action(
            account,
            current,
            "/app/reference",
            "Read study note",
            ICON_REFERENCE,
            "Read the study material and its provenance, then return to this same quiz.",
        ),
    )
}

fn render_verdict(grade: &BetaStudyGrade) -> String {
    format!(
        r#"<p class="me-result">{icon}<span class="me-verdict" tabindex="-1">{label}</span></p>"#,
        icon = verdict_icon(grade.verdict),
        label = verdict_label(grade.verdict),
    )
}

fn render_next(account: &AppAccount) -> String {
    // Feedback remains until one deliberate tap, keypress, or upward swipe.
    format!(
        r#"<form class="me-next" action="/app/next" method="post">{csrf}<button class="ae-button" type="submit">Next question</button></form>"#,
        csrf = hidden_csrf_input(account),
    )
}

const ANALYTICS_PAGE_SIZE: usize = 12;

fn render_concept_health_surface(
    concepts: &[BetaStudyConceptProgress],
    options: AnalyticsViewOptions,
) -> String {
    let has_concepts = !concepts.is_empty();
    let mut concepts = concepts
        .iter()
        .filter(|concept| concept_matches_filter(concept, options.filter))
        .collect::<Vec<_>>();
    concepts.sort_by(|left, right| compare_concepts(left, right, options.sort));

    let total = concepts.len();
    let page_count = total.div_ceil(ANALYTICS_PAGE_SIZE).max(1);
    let page = options.page.max(1).min(page_count);
    let start = (page - 1) * ANALYTICS_PAGE_SIZE;
    let end = (start + ANALYTICS_PAGE_SIZE).min(total);
    let rows = concepts[start..end]
        .iter()
        .map(|concept| render_concept_row(concept))
        .collect::<String>();

    let count = if total == 0 {
        if has_concepts {
            r#"<p class="me-analytics-empty ae-dim">No concepts match this filter. <a href="/app/analytics">Show all concepts</a>.</p>"#.to_owned()
        } else {
            r#"<section class="me-analytics-empty"><h2 class="ae-h">Progress starts with practice</h2><p class="ae-lede">Add something to learn. Your history will appear here as you review.</p><a class="ae-button-quiet" href="/">Back to review</a></section>"#.to_owned()
        }
    } else {
        format!(
            r#"<p class="me-analytics-count ae-dim">Showing {}–{} of {} {}</p>"#,
            start + 1,
            end,
            total,
            concept_count_label(total, options.filter),
        )
    };
    let controls = render_analytics_controls(options);
    let pagination = render_analytics_pagination(page, page_count, options);
    let list = if rows.is_empty() {
        String::new()
    } else {
        format!(r#"<div class="me-analytics-list">{rows}</div>"#)
    };

    format!(
        r#"<section class="ae-group me-analytics-group">
{controls}
{count}
{list}
{pagination}
</section>"#,
    )
}

fn render_analytics_controls(options: AnalyticsViewOptions) -> String {
    format!(
        r#"<form class="me-analytics-controls" action="/app/analytics" method="get">
<label class="ae-label" for="me-analytics-filter">Health<select class="ae-input me-analytics-select" id="me-analytics-filter" name="filter">
{filter_options}
</select></label>
<label class="ae-label" for="me-analytics-sort">Sort<select class="ae-input me-analytics-select" id="me-analytics-sort" name="sort">
{sort_options}
</select></label>
<button class="ae-button ae-button-compact" type="submit">Apply</button>
</form>"#,
        filter_options = analytics_filter_options(options.filter),
        sort_options = analytics_sort_options(options.sort),
    )
}

fn analytics_filter_options(selected: AnalyticsConceptFilter) -> String {
    let mut options = String::new();
    for (filter, value, label) in [
        (AnalyticsConceptFilter::All, "all", "All concepts"),
        (AnalyticsConceptFilter::AtRisk, "at-risk", "At risk"),
        (
            AnalyticsConceptFilter::Struggling,
            "struggling",
            "Struggling",
        ),
        (AnalyticsConceptFilter::Mixed, "mixed", "Mixed"),
        (AnalyticsConceptFilter::Solid, "solid", "Solid"),
        (AnalyticsConceptFilter::Untried, "untried", "Untried"),
    ] {
        let selected_attribute = if filter == selected { " selected" } else { "" };
        let _ = write!(
            options,
            r#"<option value="{value}"{selected_attribute}>{label}</option>"#
        );
    }
    options
}

fn analytics_sort_options(selected: AnalyticsConceptSort) -> String {
    let mut options = String::new();
    for (sort, value, label) in [
        (
            AnalyticsConceptSort::Health,
            "health",
            "Health · at risk first",
        ),
        (AnalyticsConceptSort::Name, "name", "Name"),
        (AnalyticsConceptSort::Success, "success", "Success rate"),
    ] {
        let selected_attribute = if sort == selected { " selected" } else { "" };
        let _ = write!(
            options,
            r#"<option value="{value}"{selected_attribute}>{label}</option>"#
        );
    }
    options
}

fn render_analytics_pagination(
    page: usize,
    page_count: usize,
    options: AnalyticsViewOptions,
) -> String {
    if page_count <= 1 {
        return String::new();
    }
    let previous = if page > 1 {
        format!(
            r#"<a class="ae-button-quiet ae-button-compact" href="{}">Previous</a>"#,
            analytics_page_href(options, page - 1)
        )
    } else {
        r#"<span class="me-pagination-spacer" aria-hidden="true"></span>"#.to_owned()
    };
    let next = if page < page_count {
        format!(
            r#"<a class="ae-button-quiet ae-button-compact" href="{}">Next</a>"#,
            analytics_page_href(options, page + 1)
        )
    } else {
        r#"<span class="me-pagination-spacer" aria-hidden="true"></span>"#.to_owned()
    };
    format!(
        r#"<nav class="me-analytics-pagination" aria-label="Concept pages">{previous}<span>{page} of {page_count}</span>{next}</nav>"#,
    )
}

fn analytics_page_href(options: AnalyticsViewOptions, page: usize) -> String {
    format!(
        "/app/analytics?filter={}&amp;sort={}&amp;page={page}",
        analytics_filter_value(options.filter),
        analytics_sort_value(options.sort),
    )
}

fn analytics_filter_value(filter: AnalyticsConceptFilter) -> &'static str {
    match filter {
        AnalyticsConceptFilter::All => "all",
        AnalyticsConceptFilter::AtRisk => "at-risk",
        AnalyticsConceptFilter::Struggling => "struggling",
        AnalyticsConceptFilter::Mixed => "mixed",
        AnalyticsConceptFilter::Solid => "solid",
        AnalyticsConceptFilter::Untried => "untried",
    }
}

fn analytics_sort_value(sort: AnalyticsConceptSort) -> &'static str {
    match sort {
        AnalyticsConceptSort::Health => "health",
        AnalyticsConceptSort::Name => "name",
        AnalyticsConceptSort::Success => "success",
    }
}

fn concept_count_label(total: usize, filter: AnalyticsConceptFilter) -> String {
    let noun = if total == 1 { "concept" } else { "concepts" };
    match filter {
        AnalyticsConceptFilter::All => noun.to_owned(),
        AnalyticsConceptFilter::AtRisk => format!("at-risk {noun}"),
        AnalyticsConceptFilter::Struggling => format!("struggling {noun}"),
        AnalyticsConceptFilter::Mixed => format!("mixed {noun}"),
        AnalyticsConceptFilter::Solid => format!("solid {noun}"),
        AnalyticsConceptFilter::Untried => format!("untried {noun}"),
    }
}

fn concept_matches_filter(
    concept: &BetaStudyConceptProgress,
    filter: AnalyticsConceptFilter,
) -> bool {
    match filter {
        AnalyticsConceptFilter::All => true,
        AnalyticsConceptFilter::AtRisk => is_at_risk(&concept.health),
        AnalyticsConceptFilter::Struggling => concept.health == "struggling",
        AnalyticsConceptFilter::Mixed => concept.health == "mixed",
        AnalyticsConceptFilter::Solid => concept.health == "solid",
        AnalyticsConceptFilter::Untried => concept.health == "untried",
    }
}

fn is_at_risk(health: &str) -> bool {
    matches!(health, "struggling" | "mixed")
}

fn compare_concepts(
    left: &BetaStudyConceptProgress,
    right: &BetaStudyConceptProgress,
    sort: AnalyticsConceptSort,
) -> std::cmp::Ordering {
    let ordering = match sort {
        AnalyticsConceptSort::Health => health_rank(&left.health).cmp(&health_rank(&right.health)),
        AnalyticsConceptSort::Name => left.concept_label.cmp(&right.concept_label),
        AnalyticsConceptSort::Success => compare_success_rate(left, right),
    };
    ordering
        .then_with(|| right.attempts.cmp(&left.attempts))
        .then_with(|| left.concept_label.cmp(&right.concept_label))
}

fn health_rank(health: &str) -> u8 {
    match health {
        "struggling" | "at risk" | "at-risk" | "weak" => 0,
        "mixed" | "watch" => 1,
        "solid" | "healthy" | "strong" => 2,
        _ => 3,
    }
}

fn compare_success_rate(
    left: &BetaStudyConceptProgress,
    right: &BetaStudyConceptProgress,
) -> std::cmp::Ordering {
    match (left.attempts == 0, right.attempts == 0) {
        (true, true) => std::cmp::Ordering::Equal,
        (true, false) => std::cmp::Ordering::Greater,
        (false, true) => std::cmp::Ordering::Less,
        (false, false) => {
            // Compare left.correct / left.attempts with right.correct /
            // right.attempts without floating-point rounding. u128 keeps the
            // cross-products lossless for usize-sized counters.
            let left_cross = (left.correct as u128) * (right.attempts as u128);
            let right_cross = (right.correct as u128) * (left.attempts as u128);
            right_cross.cmp(&left_cross)
        }
    }
}

fn render_concept_row(concept: &BetaStudyConceptProgress) -> String {
    let pct = concept
        .correct
        .saturating_mul(100)
        .checked_div(concept.attempts)
        .unwrap_or(0)
        .min(100);
    format!(
        r#"<article class="me-concept" data-health="{health}">
<div class="me-concept-head"><div class="me-concept-label"><strong>{label}</strong><span class="me-health-label {fill}">{health}</span></div><span class="me-trend ae-dim">{trend_icon} {trend}</span></div>
<div class="ae-meter" aria-hidden="true"><div class="ae-meter-fill {fill}" style="width:{pct}%"></div></div>
<p class="me-concept-note ae-dim">{success_rate}<span>{summary}</span></p>
</article>"#,
        label = escape_html(&concept.concept_label),
        health = escape_html(&concept.health),
        trend_icon = trend_icon(&concept.trend),
        trend = escape_html(&concept.trend),
        fill = health_fill_class(&concept.health),
        success_rate = escape_html(&concept.success_rate),
        summary = escape_html(&concept.summary),
    )
}

fn plural(count: usize, singular: &'static str, plural: &'static str) -> &'static str {
    if count == 1 {
        singular
    } else {
        plural
    }
}

fn render_escape_hatches(account: &AppAccount, current: &BetaStudyCurrent) -> String {
    // Only Reveal stays beside the Quiz. Other actions expand into the
    // document with visible, truthful scope descriptions for touch users.
    format!(
        r#"<details class="me-more"><summary aria-label="Question options">Options</summary><div class="me-more-sheet">{reference}{skip}{snooze}{concept_snooze}{bridge}{edit}<details class="me-hatch-delete"><summary>Delete quiz</summary>{delete}</details></div></details>"#,
        reference = render_review_action(
            account,
            current,
            "/app/reference",
            "Study note",
            ICON_REFERENCE,
            "Read the study material and its provenance, then return to this quiz.",
        ),
        skip = render_review_action(
            account,
            current,
            "/app/skip",
            "Skip",
            ICON_SKIP,
            "Set this quiz aside until later in this session.",
        ),
        snooze = render_review_action(
            account,
            current,
            "/app/snooze",
            "Snooze quiz",
            ICON_SNOOZE,
            "Hide only this quiz until tomorrow.",
        ),
        concept_snooze = current
            .concept_key
            .as_deref()
            .filter(|key| !key.trim().is_empty())
            .map_or_else(String::new, |_| {
                render_review_action(
                    account,
                    current,
                    "/app/snooze-concept",
                    "Snooze concept",
                    ICON_SNOOZE,
                    "Hide quizzes for this exact concept until tomorrow, not the entire source.",
                )
            }),
        bridge = render_review_action(
            account,
            current,
            "/app/bridge",
            "Make this easier",
            ICON_BRIDGE,
            "Practice a simpler part of this idea first.",
        ),
        edit = render_review_action(
            account,
            current,
            "/app/edit",
            "Edit quiz",
            ICON_EDIT,
            "Correct the question or answer without changing review history.",
        ),
        delete = render_review_action(
            account,
            current,
            "/app/delete",
            "Delete this quiz",
            ICON_TRASH,
            "Remove this quiz from review for good. The source stays in Library.",
        ),
    )
}

fn render_review_action(
    account: &AppAccount,
    current: &BetaStudyCurrent,
    action: &str,
    label: &str,
    icon: &str,
    title: &str,
) -> String {
    format!(
        r#"<form action="{action}" method="post">{csrf}<input type="hidden" name="reviewUnitId" value="{id}"><button class="ae-button-quiet ae-button-compact" type="submit" title="{title}">{icon}<span class="me-action-copy"><span class="me-action-label">{label}</span><span class="me-action-description">{title}</span></span></button></form>"#,
        csrf = hidden_csrf_input(account),
        id = escape_html(&current.review_unit_id.to_string()),
        label = escape_html(label),
        title = escape_html(title),
    )
}

fn render_reveal_form(account: &AppAccount, current: &BetaStudyCurrent) -> String {
    if current.expected_answer.is_some() || current.grade.is_some() {
        return String::new();
    }

    format!(
        r#"<form class="me-reveal" action="/app/reveal" method="post">{fields}<button class="ae-button-quiet" type="submit">I don’t know yet</button></form>"#,
        fields = review_submit_fields(account, current),
    )
}

fn verdict_label(verdict: impl std::fmt::Debug) -> &'static str {
    match format!("{verdict:?}").as_str() {
        "Correct" => "Correct",
        "Close" => "Close",
        "Wrong" => "Try again",
        "Revealed" => "Revealed",
        _ => "Needs review",
    }
}

fn verdict_icon(verdict: impl std::fmt::Debug) -> &'static str {
    match format!("{verdict:?}").as_str() {
        "Correct" => ICON_OK,
        "Close" => ICON_WARN,
        "Wrong" => ICON_ERR,
        "Revealed" => ICON_REVEALED,
        _ => ICON_INFO,
    }
}

fn health_fill_class(health: &str) -> &'static str {
    match health {
        "healthy" | "strong" | "solid" => "ae-ok",
        "watch" | "mixed" => "ae-warn",
        "struggling" | "at risk" | "at-risk" | "weak" => "ae-err",
        _ => "",
    }
}

fn trend_icon(trend: &str) -> &'static str {
    match trend {
        "improving" | "rising" => ICON_UP,
        "slipping" | "declining" | "falling" => ICON_DOWN,
        _ => "",
    }
}

fn hidden_csrf_input(account: &AppAccount) -> String {
    format!(
        r#"<input type="hidden" name="csrfToken" value="{}">"#,
        escape_html(account.csrf_token())
    )
}

/// Secondary management stays out of the review feed.
fn account_menu(account: &AppAccount) -> String {
    let csrf = hidden_csrf_input(account);
    format!(
        r#"<details class="me-account">
<summary class="me-account-summary">More</summary>
<div class="me-account-sheet">
<a href="/app/library">Your learning</a>
<a href="/app/analytics">Progress</a>
{reminders}
<form class="me-foot-form" action="/app/logout" method="post">{csrf}<button class="ae-button-quiet ae-button-compact" type="submit">Sign out</button></form>
<form class="me-foot-form" action="/app/logout-all" method="post">{csrf}<button class="ae-button-quiet ae-button-compact" type="submit">Sign out all browsers</button></form>
<p>Signing out here doesn’t affect service sessions.</p>
</div>
</details>"#,
        reminders = render_return_notifications(account),
    )
}

fn escape_html(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
}

const FOOTER_TAGLINE: &str =
    r#"<span class="ae-dim">Scry. A little practice, kept with you.</span>"#;

// Lucide icons (ISC), inlined for `.ae-icon`: 1.5px stroke, currentColor, no
// fill. Status hue rides the glyph; the sentence stays ink.
const ICON_OK: &str = r#"<svg class="ae-icon ae-ok" viewBox="0 0 24 24" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg>"#;
const ICON_WARN: &str = r#"<svg class="ae-icon ae-warn" viewBox="0 0 24 24" aria-hidden="true"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><path d="M12 9v4"/><path d="M12 17h.01"/></svg>"#;
const ICON_ERR: &str = r#"<svg class="ae-icon ae-err" viewBox="0 0 24 24" aria-hidden="true"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>"#;
const ICON_REVEALED: &str = r#"<svg class="ae-icon ae-revealed" viewBox="0 0 24 24" aria-hidden="true"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="3"/></svg>"#;
const ICON_INFO: &str = r#"<svg class="ae-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></svg>"#;
const ICON_UP: &str = r#"<svg class="ae-icon ae-ok" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 17 17 7"/><path d="M7 7h10v10"/></svg>"#;
const ICON_DOWN: &str = r#"<svg class="ae-icon ae-warn" viewBox="0 0 24 24" aria-hidden="true"><path d="M7 7 17 17"/><path d="M17 7v10H7"/></svg>"#;

// More-sheet action icons (memory-engine-081): each escape hatch carries a
// leading Lucide-style glyph (`.ae-icon`: 24x24 viewBox, 1.5px stroke).
const ICON_REFERENCE: &str = r#"<svg class="ae-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 7v14"/><path d="M3 18V6a1 1 0 0 1 1-1h5a3 3 0 0 1 3 3 3 3 0 0 1 3-3h5a1 1 0 0 1 1 1v12a1 1 0 0 1-1 1h-6a2 2 0 0 0-2 2 2 2 0 0 0-2-2H4a1 1 0 0 1-1-1z"/></svg>"#;
const ICON_SKIP: &str = r#"<svg class="ae-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m6 5 9 7-9 7z"/><path d="M19 5v14"/></svg>"#;
const ICON_SNOOZE: &str = r#"<svg class="ae-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>"#;
const ICON_BRIDGE: &str = r#"<svg class="ae-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 19h18"/><path d="M6 19v-6a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v6"/><path d="M9 11V6"/><path d="M15 11V6"/></svg>"#;
const ICON_TRASH: &str = r#"<svg class="ae-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7h16"/><path d="M9 7V5a2 2 0 0 1 2-2h2a2 2 0 0 1 2 2v2"/><path d="M18 7l-1 13a2 2 0 0 1-2 2H9a2 2 0 0 1-2-2L6 7"/><path d="M10 11v6"/><path d="M14 11v6"/></svg>"#;
const ICON_EDIT: &str = r#"<svg class="ae-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L8 18l-4 1 1-4Z"/></svg>"#;
const ICON_PLUS: &str = r#"<svg class="ae-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M12 5v14"/><path d="M5 12h14"/></svg>"#;

#[cfg(test)]
mod analytics_tests {
    use memory_engine_api_state::{AppAccount, StudyViewResponse};
    use memory_engine_study::BetaStudyConceptProgress;
    use memory_engine_study::BetaStudySummary;

    use super::{
        render_concept_health_surface, AnalyticsConceptFilter, AnalyticsConceptSort,
        AnalyticsViewOptions,
    };

    fn concept(
        label: &str,
        health: &str,
        correct: usize,
        attempts: usize,
    ) -> BetaStudyConceptProgress {
        BetaStudyConceptProgress {
            concept_key: label.to_owned(),
            concept_label: label.to_owned(),
            attempts,
            correct,
            success_rate: format!("{correct} of {attempts} correct"),
            trend: "steady correct".to_owned(),
            average_response_time_ms: Some(900),
            response_time_trend: "steady".to_owned(),
            health: health.to_owned(),
            summary: format!("{label} is {health}"),
        }
    }

    #[test]
    fn analytics_surface_filters_risk_sorts_and_paginates() {
        let concepts = (0..13)
            .map(|index| {
                concept(
                    &format!("Concept {index:02}"),
                    if index == 0 { "solid" } else { "struggling" },
                    if index == 0 { 9 } else { 1 },
                    10,
                )
            })
            .collect::<Vec<_>>();

        let page = render_concept_health_surface(
            &concepts,
            AnalyticsViewOptions {
                filter: AnalyticsConceptFilter::AtRisk,
                sort: AnalyticsConceptSort::Health,
                page: 1,
            },
        );

        assert_eq!(page.matches("class=\"me-concept\"").count(), 12);
        assert!(page.contains("Concept 01"));
        assert!(!page.contains("Concept 00"));
        assert!(page.contains("Showing 1–12 of 12 at-risk concepts"));
        assert!(!page.contains("page=2"));
    }

    #[test]
    fn analytics_surface_keeps_page_two_bounded_and_preserves_controls() {
        let concepts = (0..25)
            .map(|index| concept(&format!("Concept {index:02}"), "solid", 9, 10))
            .collect::<Vec<_>>();

        let page = render_concept_health_surface(
            &concepts,
            AnalyticsViewOptions {
                filter: AnalyticsConceptFilter::All,
                sort: AnalyticsConceptSort::Name,
                page: 2,
            },
        );

        assert_eq!(page.matches("class=\"me-concept\"").count(), 12);
        assert!(!page.contains("Concept 00"));
        assert!(page.contains("Concept 12"));
        assert!(page.contains("Showing 13–24 of 25 concepts"));
        assert!(page.contains("filter=all&amp;sort=name&amp;page=1"));
        assert!(page.contains("filter=all&amp;sort=name&amp;page=3"));
    }

    #[test]
    fn success_sort_uses_rate_not_lexicographic_counts_and_puts_untried_last() {
        let concepts = vec![
            concept("Nine of ten", "solid", 9, 10),
            concept("Perfect eight", "solid", 8, 8),
            concept("More evidence", "solid", 8, 10),
            concept("Less evidence", "solid", 4, 5),
            concept("Untried", "untried", 0, 0),
        ];

        let page = render_concept_health_surface(
            &concepts,
            AnalyticsViewOptions {
                filter: AnalyticsConceptFilter::All,
                sort: AnalyticsConceptSort::Success,
                page: 1,
            },
        );

        let perfect = page
            .find("<strong>Perfect eight</strong>")
            .expect("perfect");
        let nine = page.find("<strong>Nine of ten</strong>").expect("nine");
        let more_evidence = page
            .find("<strong>More evidence</strong>")
            .expect("more evidence");
        let less_evidence = page
            .find("<strong>Less evidence</strong>")
            .expect("less evidence");
        let untried = page.find("<strong>Untried</strong>").expect("untried");
        assert!(
            perfect < nine,
            "success rate must outrank raw correct count: {page}"
        );
        assert!(
            more_evidence < less_evidence,
            "equal rates prefer more evidence: {page}"
        );
        assert!(
            nine < untried,
            "untried concepts sort after measured concepts: {page}"
        );
    }

    #[test]
    fn analytics_page_is_a_complete_document_with_one_asset_contract() {
        let state = super::render_test_state("analytics-document@example.com");
        let created = state
            .create_account("analytics-document@example.com")
            .expect("account");
        let account: AppAccount = state.create_browser_session(&created).expect("session");
        let view = StudyViewResponse {
            drafts: Vec::new(),
            queue: Vec::new(),
            current: None,
            concept_progress: Vec::new(),
            summary: BetaStudySummary {
                source_count: 0,
                accepted_draft_count: 0,
                approved_review_unit_count: 0,
                attempt_count: 0,
                last_outcome: None,
                next_review_unit_id: None,
            },
            due_count: 0,
            generation_notices: Vec::new(),
            library: Vec::new(),
        };

        let page = super::render_analytics_page(&account, &view, AnalyticsViewOptions::default());

        assert!(page.starts_with("<!doctype html>"));
        assert!(page
            .contains(r#"<meta name="viewport" content="width=device-width, initial-scale=1">"#));
        assert!(page.contains(r#"<meta name="color-scheme" content="light dark">"#));
        assert_eq!(page.matches(r#"href="/static/ledger.css""#).count(), 1);
        assert_eq!(page.matches(r#"src="/static/app.js""#).count(), 1);
    }

    #[test]
    fn analytics_filter_separates_untried_from_at_risk() {
        let concepts = vec![
            concept("Needs work", "struggling", 1, 10),
            concept("Needs data", "untried", 0, 0),
        ];

        let page = super::render_concept_health_surface(
            &concepts,
            AnalyticsViewOptions {
                filter: AnalyticsConceptFilter::Untried,
                sort: AnalyticsConceptSort::Health,
                page: 1,
            },
        );

        assert!(page.contains(r#"value="untried" selected>Untried</option>"#));
        assert!(page.contains("<strong>Needs data</strong>"));
        assert!(!page.contains("<strong>Needs work</strong>"));
        assert!(page.contains("Showing 1–1 of 1 untried concept"));
    }

    #[test]
    fn analytics_page_applies_untried_filter_to_a_study_view_response() {
        let state = super::render_test_state("analytics-untried@example.com");
        let created = state
            .create_account("analytics-untried@example.com")
            .expect("account");
        let account = state.create_browser_session(&created).expect("session");
        let view = StudyViewResponse {
            drafts: Vec::new(),
            queue: Vec::new(),
            current: None,
            concept_progress: vec![
                concept("Needs data", "untried", 0, 0),
                concept("Needs work", "struggling", 1, 10),
            ],
            summary: BetaStudySummary {
                source_count: 1,
                accepted_draft_count: 2,
                approved_review_unit_count: 2,
                attempt_count: 1,
                last_outcome: None,
                next_review_unit_id: None,
            },
            due_count: 0,
            generation_notices: Vec::new(),
            library: Vec::new(),
        };

        let page = super::render_analytics_page(
            &account,
            &view,
            AnalyticsViewOptions {
                filter: AnalyticsConceptFilter::Untried,
                sort: AnalyticsConceptSort::Health,
                page: 1,
            },
        );

        assert!(page.contains(r#"<option value="untried" selected>Untried</option>"#));
        assert!(page.contains("<strong>Needs data</strong>"));
        assert!(!page.contains("<strong>Needs work</strong>"));
    }
}
