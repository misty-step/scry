package generation

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/misty-step/scry/internal/store"
)

// quoteSpan indexes normalized bytes back into the original material. The
// returned evidence is always sliced from that original, never reconstructed.
type quoteSpan struct {
	text   string
	starts []int
	ends   []int
}

func canonicalQuote(text string) quoteSpan {
	var b strings.Builder
	span := quoteSpan{}
	space, spaceStart, spaceEnd := false, 0, 0
	for start, r := range text {
		end := start + utf8.RuneLen(r)
		if unicode.IsSpace(r) {
			if !space {
				spaceStart = start
			}
			space, spaceEnd = true, end
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
			span.starts = append(span.starts, spaceStart)
			span.ends = append(span.ends, spaceEnd)
		}
		space = false
		mapped := string(r)
		switch r {
		case '“', '”':
			mapped = `"`
		case '‘', '’':
			mapped = "'"
		case '–', '—':
			mapped = "-"
		case '…':
			mapped = "..."
		}
		b.WriteString(mapped)
		for range len(mapped) {
			span.starts = append(span.starts, start)
			span.ends = append(span.ends, end)
		}
	}
	span.text = b.String()
	return span
}

func snapV5Evidence(basis string, provided []string, citations []store.Citation, job *store.Job, input store.JobContext) (string, []string, []store.Citation, error) {
	if basis == "topic" {
		return basis, provided, citations, nil
	}
	if basis != "source" && basis != "web" {
		return basis, nil, nil, errors.New("unknown evidence basis")
	}
	type material struct {
		text     string
		mapped   quoteSpan
		citation store.Citation
	}
	var eligible []material
	add := func(text string, citation store.Citation) {
		eligible = append(eligible, material{text: text, mapped: canonicalQuote(text), citation: citation})
	}
	if basis == "source" {
		if job.SourceMode == "text" || job.SourceMode == "" {
			add(job.SourceText, store.Citation{})
		}
		for _, doc := range input.Documents {
			if doc.Kind == "page" || doc.Kind == "transcript" {
				add(doc.Text, store.Citation{})
			}
		}
	} else {
		// Prefer a cited, metadata-matched document; an uncited supplied result
		// can still be cited truthfully if it contains the actual quotation.
		for _, citation := range citations {
			for _, doc := range input.Documents {
				if doc.Kind == "search_result" && doc.ID == citation.DocumentID && doc.Title == citation.Title && doc.URL == citation.URL {
					add(doc.Text, citation)
				}
			}
		}
		for _, doc := range input.Documents {
			if doc.Kind == "search_result" {
				add(doc.Text, store.Citation{DocumentID: doc.ID, Title: doc.Title, URL: doc.URL})
			}
		}
	}
	var quotes []string
	var keptCitations []store.Citation
	seenCitation := map[string]bool{}
	for _, quote := range provided {
		needle := canonicalQuote(quote).text
		if needle == "" {
			continue
		}
		for _, item := range eligible {
			mapped := item.mapped
			at := strings.Index(mapped.text, needle)
			if at < 0 {
				continue
			}
			original := item.text[mapped.starts[at]:mapped.ends[at+len(needle)-1]]
			limit := 1024
			if job.Kind == "questions" || job.Kind == "contrast" || job.Kind == "fix" {
				limit = 8192
			}
			if len(original) > limit || !plainV5(original, limit) {
				continue
			}
			quotes = append(quotes, original)
			if basis == "web" && !seenCitation[item.citation.DocumentID] {
				keptCitations = append(keptCitations, item.citation)
				seenCitation[item.citation.DocumentID] = true
			}
			break
		}
	}
	if len(quotes) == 0 {
		if job.SourceKind == "topic" {
			return "topic", nil, nil, nil
		}
		if basis == "source" {
			return basis, nil, nil, errors.New("source evidence is not an exact quotation")
		}
		return basis, nil, nil, errors.New("web evidence could not be matched to a supplied excerpt")
	}
	if basis == "source" {
		return basis, quotes, nil, nil
	}
	return basis, quotes, keptCitations, nil
}
