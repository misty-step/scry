//! Task-specific reference-note probes. Mechanical coverage is not a truth judge.

use std::fmt::Write as _;

use memory_engine_generation::{
    ProviderUsage, ReferenceNoteProvider, ReferenceNoteRequest, SourceAuthorizationContext,
};
use memory_engine_persistence::{SourceDocument, SourceDocumentKind, SourcePermission};
use serde::Deserialize;

use super::{format_cost, fraction, normalize, CorpusSource, NOW};

#[derive(Clone, Debug, Deserialize)]
pub(super) struct Expectation {
    concept: String,
    question: String,
    answer: String,
    explanation_terms: Vec<String>,
    example_terms: Vec<String>,
    distinction_terms: Vec<String>,
    source_supported: bool,
    #[serde(default)]
    forbidden_claims: Vec<String>,
}

pub(super) struct Score {
    source_id: String,
    section_words: [usize; 3],
    explanation: f64,
    example: f64,
    distinctions: f64,
    retrieval: bool,
    provenance: bool,
    forbidden_free: bool,
    usage: Option<ProviderUsage>,
    error: Option<String>,
    note: Option<String>,
}

impl Score {
    fn passes(&self) -> bool {
        self.error.is_none()
            && self.section_words[0] >= 35
            && self.section_words[1] >= 8
            && self.section_words[2] >= 6
            && self.explanation >= 0.6
            && self.example >= 0.5
            && self.distinctions >= 0.5
            && self.retrieval
            && self.provenance
            && self.forbidden_free
    }
}

pub(super) fn score<P: ReferenceNoteProvider + ?Sized>(
    provider: &P,
    source: &CorpusSource,
    expect: &Expectation,
) -> Score {
    let document = SourceDocument {
        id: source.id.clone(),
        kind: SourceDocumentKind::Text,
        title: source.title.clone(),
        project_key: None,
        body: Some(source.body.clone()),
        uri: None,
        permission: SourcePermission::ModelEligible,
        freshness: Some(NOW),
        ttl_expires_at: None,
        created_at: NOW,
        archived_at: None,
    };
    let authorization =
        SourceAuthorizationContext::from_sources(&[document]).expect("active eval fixture");
    let request = ReferenceNoteRequest::new(
        &expect.concept,
        &expect.concept,
        &expect.question,
        &expect.answer,
        Vec::new(),
        authorization,
    );
    match provider.explain_concept_with_usage(&request) {
        Ok((note, usage)) => judge(&source.id, &source.body, expect, note.body, usage),
        Err(failure) => Score {
            source_id: source.id.clone(),
            section_words: [0; 3],
            explanation: 0.0,
            example: 0.0,
            distinctions: 0.0,
            retrieval: false,
            provenance: false,
            forbidden_free: false,
            usage: failure.usage().cloned(),
            error: Some(failure.to_string()),
            note: None,
        },
    }
}

fn section<'a>(body: &'a str, heading: &str, next: &str) -> &'a str {
    body.split_once(heading)
        .map_or("", |(_, rest)| {
            rest.split_once(next).map_or(rest, |(text, _)| text)
        })
        .trim()
}

fn coverage(text: &str, terms: &[String]) -> f64 {
    let normalized = normalize(text);
    fraction(
        terms
            .iter()
            .filter(|term| normalized.contains(&normalize(term)))
            .count(),
        terms.len(),
    )
}

fn judge(
    id: &str,
    source: &str,
    expect: &Expectation,
    body: String,
    usage: Option<ProviderUsage>,
) -> Score {
    let explanation = section(&body, "\n\nExplanation\n", "\n\nExample\n");
    let example = section(&body, "\n\nExample\n", "\n\nKey distinctions\n");
    let distinctions = section(&body, "\n\nKey distinctions\n", "\n\nRetrieval cue\n");
    let cue = section(&body, "\n\nRetrieval cue\n", "\n\nSource context\n");
    let source_context = section(&body, "\n\nSource context\n", "\u{0}");
    let quotes: Vec<_> = source_context
        .lines()
        .map(str::trim)
        .filter(|line| !line.is_empty() && !line.ends_with("]:"))
        .collect();
    let provenance = if expect.source_supported {
        body.starts_with("Source-informed model explanation.")
            && !quotes.is_empty()
            && quotes.iter().all(|quote| source.contains(quote))
            && quotes.iter().any(|quote| {
                memory_engine_generation::answer_has_evidence_support(quote, &expect.answer)
            })
    } else {
        body.starts_with("Model-expanded study material.") && quotes.is_empty()
    };
    let normalized = normalize(&body);
    Score {
        source_id: id.to_owned(),
        section_words: [
            explanation.split_whitespace().count(),
            example.split_whitespace().count(),
            distinctions.split_whitespace().count(),
        ],
        explanation: coverage(explanation, &expect.explanation_terms),
        example: coverage(example, &expect.example_terms),
        distinctions: coverage(distinctions, &expect.distinction_terms),
        retrieval: cue.ends_with('?') && cue.split_whitespace().count() >= 4,
        provenance,
        forbidden_free: expect
            .forbidden_claims
            .iter()
            .all(|claim| !normalized.contains(&normalize(claim))),
        usage,
        error: None,
        note: Some(body),
    }
}

pub(super) fn render(receipt: &mut String, scores: &[Score], baseline: Option<&str>) {
    let _ = writeln!(receipt, "\n## Reference quality\n");
    let _ = writeln!(receipt, "Mechanical probes are necessary, not sufficient: factual accuracy, example usefulness, and pedagogical depth remain **human-unassessed** until blinded review. The fake provider is deliberately not a quality baseline. Quotes are checked against actual authorized source bodies; topic expansion must not claim quotations.\n");
    let _ = writeln!(receipt, "| reference | words explanation/example/distinctions | explanation | example | distinctions | retrieval | provenance | forbidden-free | pass | tokens in/out | cost | latency |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |");
    let mut rates = Vec::with_capacity(scores.len());
    let mut known_cost = 0_i64;
    let mut unknown_cost = 0;
    for score in scores {
        let passed = score.passes();
        rates.push((score.source_id.clone(), f64::from(u8::from(passed))));
        let usage = score.usage.as_ref();
        if let Some(cost) = usage.and_then(|usage| usage.cost_usd_micros) {
            known_cost = known_cost.saturating_add(cost);
        } else {
            unknown_cost += 1;
        }
        let _ = writeln!(
            receipt,
            "| {} | {}/{}/{} | {:.0}% | {:.0}% | {:.0}% | {} | {} | {} | {} | {}/{} | {} | {}ms |",
            score.source_id,
            score.section_words[0],
            score.section_words[1],
            score.section_words[2],
            score.explanation * 100.0,
            score.example * 100.0,
            score.distinctions * 100.0,
            score.retrieval,
            score.provenance,
            score.forbidden_free,
            u8::from(passed),
            usage.map_or(0, |usage| usage.input_tokens),
            usage.map_or(0, |usage| usage.output_tokens),
            format_cost(usage.and_then(|usage| usage.cost_usd_micros)),
            usage.map_or(0, |usage| usage.latency_ms)
        );
        if let Some(error) = &score.error {
            let _ = writeln!(receipt, "\nReference {} failed: {error}\n", score.source_id);
        }
    }
    let _ = writeln!(receipt, "\n- Reference reported cost subtotal: {} · unreported/uncertain: {unknown_cost}/{} calls. Failed responses retain reported usage; zero tokens may mean unreported, not free.", format_cost(Some(known_cost)), scores.len());
    render_comparison(
        receipt,
        "Reference mechanical pass",
        &rates,
        baseline.map(parse_rates).as_deref(),
    );
    let _ = writeln!(receipt, "\n### Reference material for blinded human review\n\nJudge each note for factual faithfulness/provenance, explanatory depth, a useful example, an accurate distinction, and a retrieval cue. Compare paired old/new notes with provider labels hidden and order randomized; record keep/revise/reject and the concrete defect. Do not treat this automated pass as human approval.\n");
    for score in scores {
        if let Some(note) = &score.note {
            let _ = writeln!(receipt, "#### {}\n", score.source_id);
            // Quote every line: corpus/model content cannot inject receipt headings.
            for line in note.lines() {
                let _ = writeln!(receipt, "> {line}");
            }
            let _ = writeln!(receipt);
        }
    }
}

fn parse_rates(receipt: &str) -> Vec<(String, f64)> {
    let Some((_, section)) = receipt.split_once("## Reference quality\n") else {
        return Vec::new();
    };
    section
        .split("### Reference material")
        .next()
        .unwrap_or_default()
        .lines()
        .filter_map(|line| {
            if !line.starts_with('|') {
                return None;
            }
            let cells: Vec<_> = line.split('|').map(str::trim).collect();
            let rate = match *cells.get(9)? {
                "0" => 0.0,
                "1" => 1.0,
                _ => return None,
            };
            Some((cells[1].to_owned(), rate))
        })
        .collect()
}

pub(super) fn render_comparison(
    receipt: &mut String,
    label: &str,
    current: &[(String, f64)],
    baseline: Option<&[(String, f64)]>,
) {
    let values: Vec<_> = current.iter().map(|(_, rate)| *rate).collect();
    if let Some(ci) = crate::stats::mean_ci_95(&values) {
        let _ = writeln!(receipt, "- {label}: {:.1}% (source-clustered n={}, 95% CI ±{:.1}pp; small-n interval, not a truth guarantee)", ci.mean * 100.0, values.len(), ci.half_width * 100.0);
    } else {
        let _ = writeln!(
            receipt,
            "- {label}: fewer than two sources; spread cannot be estimated."
        );
    }
    if let Some(baseline) = baseline {
        if let Some(paired) = crate::stats::paired_verdict(current, baseline) {
            let _ = writeln!(receipt, "- Paired {label} vs baseline: Δ {:+.1}pp ±{:.1}pp (95% CI, n={} matched of {} current/{} baseline); {}. Unmatched cases are not compared.",
                paired.mean_delta * 100.0, paired.half_width * 100.0, paired.paired, current.len(), baseline.len(),
                if paired.within_noise { "within noise" } else { "difference detected, not causal proof" });
        } else {
            let _ = writeln!(
                receipt,
                "- Paired {label}: fewer than two matching cases; no comparison claim."
            );
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn source_context_cannot_make_an_answer_echo_a_useful_reference() {
        let expect = Expectation {
            concept: "HTTP revalidation".into(),
            question: "What does no-cache require?".into(),
            answer: "revalidation".into(),
            explanation_terms: vec!["revalidation".into()],
            example_terms: vec!["request".into()],
            distinction_terms: vec!["storage".into()],
            source_supported: true,
            forbidden_claims: Vec::new(),
        };
        let source = "The no-cache directive requires revalidation before reuse.";
        let body = format!("Source-informed model explanation.\n\nExplanation\nrevalidation\n\nExample\nrequest\n\nKey distinctions\nstorage\n\nRetrieval cue\nWhat does this directive require?\n\nSource context\nHTTP [http]:\n{source}");
        let score = judge("http", source, &expect, body, None);
        assert!(score.provenance);
        assert!(!score.passes());
    }

    #[test]
    fn a_missing_reference_baseline_is_not_an_improvement() {
        assert!(parse_rates("# Older quiz-only receipt\n").is_empty());
        assert!(crate::stats::paired_verdict(&[("new-reference".into(), 1.0)], &[]).is_none());
    }
}
