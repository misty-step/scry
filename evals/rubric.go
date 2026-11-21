package evals

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ExplainRubric is the deterministic 0-100 score of one explanation.
//
//	30  respects the given verdict (never says the learner was right)
//	25  concision: <=40 words 10, <=80 words 5, <=280 characters unclamped 10
//	20  format: no markdown 7, no bullet list 6, no preamble 7
//	25  contrast: required concepts covered 15 (pro rata), addresses the
//	    learner's own answer 10
type ExplainRubric struct {
	Words            int      `json:"words"`
	Chars            int      `json:"chars"`
	VerdictRespected bool     `json:"verdict_respected"`
	VerdictViolation string   `json:"verdict_violation,omitempty"`
	Within40Words    bool     `json:"within_40_words"`
	Within80Words    bool     `json:"within_80_words"`
	Within280Chars   bool     `json:"within_280_chars_unclamped"`
	NoMarkdown       bool     `json:"no_markdown"`
	NoBullets        bool     `json:"no_bullets"`
	NoPreamble       bool     `json:"no_preamble"`
	ConceptsCovered  int      `json:"concepts_covered"`
	ConceptsTotal    int      `json:"concepts_total"`
	MissingConcepts  []string `json:"missing_concepts,omitempty"`
	AddressesLearner bool     `json:"addresses_learner_answer"`
	Score            int      `json:"score"`
}

var (
	rightClaims = []string{
		"you're right", "you are right", "you were right", "you're correct", "you are correct", "you were correct",
		"your answer is right", "your answer is correct", "your answer was right", "your answer was correct",
		"that's right", "that is right", "that's correct", "that is correct", "exactly right", "spot on",
		"well done", "great job", "nailed it", "correct!",
	}
	markdownRe = regexp.MustCompile("(\\*\\*|__|`|(?m)^\\s*#{1,6}\\s|\\[[^\\]]+\\]\\([^)]+\\))")
	bulletRe   = regexp.MustCompile(`(?m)^\s*([-*•]|\d+[.)])\s+`)
	preambles  = []string{"sure", "certainly", "of course", "here's", "here is", "explanation:", "great question", "good question", "let me", "okay", "ok,", "absolutely", "i'd be happy", "i'm happy", "happy to"}
	refPhrases = []string{"your answer", "your response", "you said", "you wrote", "you answered", "you gave", "you named", "you listed", "you mentioned", "you described", "you picked", "you chose", "you swapped", "you confused", "you mixed", "you left out", "you missed", "you omitted", "you only", "you identified", "you got"}
	stopwords  = map[string]bool{"the": true, "and": true, "that": true, "this": true, "with": true, "from": true, "they": true, "their": true, "think": true, "because": true, "inside": true, "about": true, "only": true, "also": true, "into": true, "it": true, "its": true}
)

// ScoreExplanation scores raw (the model's unclamped text) for a case.
func ScoreExplanation(c ExplainerCase, raw string) ExplainRubric {
	text := strings.TrimSpace(raw)
	low := foldDiacritics.Replace(strings.ToLower(text))
	low = strings.NewReplacer("’", "'", "‘", "'").Replace(low)
	r := ExplainRubric{Words: len(strings.Fields(text)), Chars: utf8.RuneCountInString(oneLine(text)), VerdictRespected: true}
	for _, claim := range rightClaims {
		for i := 0; ; {
			j := strings.Index(low[i:], claim)
			if j < 0 {
				break
			}
			end := i + j + len(claim)
			rest := strings.TrimLeft(low[end:], " ")
			// "you're right that ..." concedes a part; it does not reverse the verdict.
			if !(strings.HasPrefix(rest, "that") || strings.HasPrefix(rest, "about") || strings.HasPrefix(rest, "to ")) {
				r.VerdictRespected, r.VerdictViolation = false, claim
				break
			}
			i = end
		}
		if !r.VerdictRespected {
			break
		}
	}
	r.Within40Words, r.Within80Words, r.Within280Chars = r.Words <= 40, r.Words <= 80, r.Chars <= 280
	r.NoMarkdown = !markdownRe.MatchString(text)
	r.NoBullets = !bulletRe.MatchString(text) && !strings.Contains(text, "•")
	r.NoPreamble = true
	for _, p := range preambles {
		if strings.HasPrefix(low, p) {
			r.NoPreamble = false
		}
	}
	r.ConceptsTotal = len(c.RequiredConcepts)
	for _, concept := range c.RequiredConcepts {
		hit := false
		for _, alt := range strings.Split(concept, "|") {
			if alt = strings.ToLower(strings.TrimSpace(alt)); alt != "" && strings.Contains(low, foldDiacritics.Replace(alt)) {
				hit = true
				break
			}
		}
		if hit {
			r.ConceptsCovered++
		} else {
			r.MissingConcepts = append(r.MissingConcepts, concept)
		}
	}
	r.AddressesLearner = addressesLearner(c, low)

	score := 0.0
	if r.VerdictRespected {
		score += 30
	}
	if r.Within40Words {
		score += 10
	}
	if r.Within80Words {
		score += 5
	}
	if r.Within280Chars {
		score += 10
	}
	if r.NoMarkdown {
		score += 7
	}
	if r.NoBullets {
		score += 6
	}
	if r.NoPreamble {
		score += 7
	}
	if r.ConceptsTotal > 0 {
		score += 15 * float64(r.ConceptsCovered) / float64(r.ConceptsTotal)
	}
	if r.AddressesLearner {
		score += 10
	}
	if text == "" {
		score = 0
	}
	r.Score = int(math.Round(score))
	return r
}

// addressesLearner: the explanation names something distinctive from the
// learner's answer (a word of 4+ letters not in the expected answer,
// matched on its first six letters), or refers to the answer outright.
func addressesLearner(c ExplainerCase, low string) bool {
	for _, p := range refPhrases {
		if strings.Contains(low, p) {
			return true
		}
	}
	expected := map[string]bool{}
	for _, w := range NormalizeWords(c.Expected) {
		expected[stem(w)] = true
	}
	out := map[string]bool{}
	for _, w := range NormalizeWords(low) {
		out[stem(w)] = true
	}
	var distinctive, all []string
	for _, w := range NormalizeWords(c.LearnerAnswer) {
		if len(w) < 4 || stopwords[w] {
			continue
		}
		all = append(all, stem(w))
		if !expected[stem(w)] {
			distinctive = append(distinctive, stem(w))
		}
	}
	if len(distinctive) == 0 {
		distinctive = all
	}
	for _, w := range distinctive {
		if out[w] {
			return true
		}
	}
	return false
}

func stem(w string) string {
	if r := []rune(w); len(r) > 6 {
		return string(r[:6])
	}
	return w
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// OutlineIdea is one parsed idea line of a draft.
type OutlineIdea struct {
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	Links   []string `json:"links"`
	Items   []QAItem `json:"items"`
	Issues  []string `json:"issues,omitempty"`
}

// QAItem is one question and answer.
type QAItem struct {
	Q string `json:"q"`
	A string `json:"a"`
}

type ideaShape struct {
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	Links   []string `json:"links"`
	Items   []QAItem `json:"items"`
}

// DraftCheck is the strict schema check of a draft's raw JSON Lines against
// Scry's drafting contract, plus the deterministic card-quality rubric.
//
// Schema valid: zero malformed lines, a head line first ({kind,title},
// kind topic|book), 4-7 idea lines each with a title <=48 characters and no
// colon, a summary <=140 characters, 1-3 links and 2-4 items with a
// question and answer, then {"done":true} as the last line.
//
// Quality (0-100, 20 each, pro rata over items/ideas/links): standalone
// questions, non-yes/no questions, answers <=25 words, ideas that do not
// duplicate learner.knows, and links copied exactly from this outline's
// titles or learner.knows.
type DraftCheck struct {
	Lines            int           `json:"lines"`
	MalformedLines   int           `json:"malformed_lines"`
	HeadOK           bool          `json:"head_ok"`
	HeadKind         string        `json:"head_kind,omitempty"`
	HeadTitle        string        `json:"head_title,omitempty"`
	KindMatches      bool          `json:"kind_matches_expected"`
	IdeaCount        int           `json:"idea_count"`
	IdeasInRange     bool          `json:"ideas_4_to_7"`
	IdeasAllValid    bool          `json:"ideas_all_valid"`
	DoneLast         bool          `json:"done_line_last"`
	Nothing          string        `json:"nothing,omitempty"`
	SchemaValid      bool          `json:"schema_valid"`
	Ideas            []OutlineIdea `json:"ideas"`
	Items            int           `json:"items"`
	StandaloneRate   float64       `json:"standalone_rate"`
	NonYesNoRate     float64       `json:"non_yes_no_rate"`
	ShortAnswerRate  float64       `json:"short_answer_rate"`
	NonDuplicateRate float64       `json:"non_duplicate_rate"`
	LinkValidRate    float64       `json:"link_exact_rate"`
	Links            int           `json:"links"`
	LibraryLinks     int           `json:"library_links_exact"`
	MiscopiedLinks   []string      `json:"miscopied_or_unknown_links,omitempty"`
	DuplicateIdeas   []string      `json:"duplicate_ideas,omitempty"`
	QualityScore     int           `json:"quality_score"`
}

var (
	yesNoStarts  = map[string]bool{"is": true, "are": true, "am": true, "was": true, "were": true, "does": true, "do": true, "did": true, "can": true, "could": true, "should": true, "would": true, "will": true, "has": true, "have": true, "had": true, "may": true, "might": true, "must": true, "shall": true, "isn't": true, "doesn't": true, "don't": true, "aren't": true}
	pronounStart = regexp.MustCompile(`^(?:(?:what|how|why|which|who|when|where)\s+(?:does|do|did|is|are|was|were|can|should|would|will)\s+)?(it|this|that|they|these|those|he|she|his|her|its|their)\b`)
	contextRefs  = []string{"this idea", "this concept", "the above", "the passage", "the excerpt", "these notes", "the notes", "the lecture", "this book", "the author", "this chapter", "the podcast", "the article", "this text", "the text", "this outline"}
)

// CheckDraft validates raw (the model's whole streamed text).
func CheckDraft(c DraftingCase, raw string) DraftCheck {
	var d DraftCheck
	var lines []string
	for _, l := range strings.Split(raw, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	d.Lines = len(lines)
	doneAt := -1
	for i, l := range lines {
		var obj map[string]json.RawMessage
		if !strings.HasPrefix(l, "{") || json.Unmarshal([]byte(l), &obj) != nil {
			d.MalformedLines++
			continue
		}
		switch {
		case obj["nothing"] != nil:
			_ = json.Unmarshal(obj["nothing"], &d.Nothing)
		case obj["done"] != nil:
			var done bool
			if json.Unmarshal(obj["done"], &done) == nil && done {
				doneAt = i
			}
		case obj["idea"] != nil:
			var s ideaShape
			if json.Unmarshal(obj["idea"], &s) != nil {
				d.MalformedLines++
				continue
			}
			d.Ideas = append(d.Ideas, OutlineIdea{Title: s.Title, Summary: s.Summary, Links: s.Links, Items: s.Items})
		case obj["title"] != nil && i == 0:
			var h struct{ Kind, Title string }
			if json.Unmarshal([]byte(l), &h) == nil && (h.Kind == "topic" || h.Kind == "book") && strings.TrimSpace(h.Title) != "" {
				d.HeadOK, d.HeadKind, d.HeadTitle = true, h.Kind, h.Title
			}
		default:
			d.MalformedLines++
		}
	}
	d.KindMatches = d.HeadOK && d.HeadKind == c.ExpectedKind
	d.IdeaCount = len(d.Ideas)
	d.IdeasInRange = d.IdeaCount >= 4 && d.IdeaCount <= 7
	d.DoneLast = doneAt >= 0 && doneAt == len(lines)-1
	d.IdeasAllValid = d.IdeaCount > 0
	titles := map[string]bool{}
	for _, idea := range d.Ideas {
		titles[strings.TrimSpace(idea.Title)] = true
	}
	knows := map[string]bool{}
	for _, k := range c.Learner.Knows {
		knows[k] = true
	}
	var standalone, nonYesNo, short, items, nonDup, linkOK int
	for i := range d.Ideas {
		idea := &d.Ideas[i]
		if t := strings.TrimSpace(idea.Title); t == "" || utf8.RuneCountInString(t) > 48 || strings.Contains(t, ":") {
			idea.Issues = append(idea.Issues, "title empty, over 48 characters or has a colon")
		}
		if s := strings.TrimSpace(idea.Summary); s == "" || utf8.RuneCountInString(s) > 140 {
			idea.Issues = append(idea.Issues, "summary empty or over 140 characters")
		}
		if len(idea.Links) < 1 || len(idea.Links) > 3 {
			idea.Issues = append(idea.Issues, "links not 1 to 3")
		}
		if len(idea.Items) < 2 || len(idea.Items) > 4 {
			idea.Issues = append(idea.Issues, "items not 2 to 4")
		}
		for _, it := range idea.Items {
			q, a := strings.TrimSpace(it.Q), strings.TrimSpace(it.A)
			if q == "" || a == "" {
				idea.Issues = append(idea.Issues, "item missing question or answer")
				continue
			}
			items++
			if StandaloneQuestion(q) {
				standalone++
			}
			if !YesNoQuestion(q) {
				nonYesNo++
			}
			if len(strings.Fields(a)) <= 25 {
				short++
			}
		}
		if len(idea.Issues) > 0 {
			d.IdeasAllValid = false
		}
		dup := false
		for _, k := range c.Learner.Knows {
			if DuplicateTitle(idea.Title, k) {
				dup = true
			}
		}
		if dup {
			d.DuplicateIdeas = append(d.DuplicateIdeas, idea.Title)
		} else {
			nonDup++
		}
		for _, l := range idea.Links {
			d.Links++
			switch {
			case knows[l]:
				linkOK++
				d.LibraryLinks++
			case titles[l]:
				linkOK++
			default:
				d.MiscopiedLinks = append(d.MiscopiedLinks, l)
			}
		}
	}
	d.Items = items
	d.SchemaValid = d.MalformedLines == 0 && d.HeadOK && d.IdeasInRange && d.IdeasAllValid && d.DoneLast && d.Nothing == ""
	rate := func(n, of int) float64 {
		if of == 0 {
			return 0
		}
		return float64(n) / float64(of)
	}
	d.StandaloneRate, d.NonYesNoRate, d.ShortAnswerRate = rate(standalone, items), rate(nonYesNo, items), rate(short, items)
	d.NonDuplicateRate, d.LinkValidRate = rate(nonDup, d.IdeaCount), rate(linkOK, d.Links)
	d.QualityScore = int(math.Round(20 * (d.StandaloneRate + d.NonYesNoRate + d.ShortAnswerRate + d.NonDuplicateRate + d.LinkValidRate)))
	return d
}

// StandaloneQuestion: the question does not open on a bare pronoun and
// does not lean on unseen context ("the passage", "this idea", ...).
func StandaloneQuestion(q string) bool {
	low := strings.ToLower(strings.TrimSpace(q))
	if pronounStart.MatchString(low) {
		return false
	}
	for _, ref := range contextRefs {
		if strings.Contains(low, ref) {
			return false
		}
	}
	return true
}

// YesNoQuestion: the question opens on an auxiliary verb.
func YesNoQuestion(q string) bool {
	f := strings.Fields(strings.ToLower(q))
	return len(f) > 0 && yesNoStarts[strings.Trim(f[0], ",.?!")]
}

// DuplicateTitle: two titles name the same idea after normalization and a
// plural fold, or share at least 75% of their words.
func DuplicateTitle(a, b string) bool {
	set := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, w := range NormalizeWords(s) {
			if stopwords[w] || w == "of" || w == "a" || w == "an" || w == "in" || w == "vs" {
				continue
			}
			if len(w) > 3 && strings.HasSuffix(w, "s") {
				w = strings.TrimSuffix(w, "s")
			}
			m[w] = true
		}
		return m
	}
	x, y := set(a), set(b)
	if len(x) == 0 || len(y) == 0 {
		return false
	}
	inter := 0
	for w := range x {
		if y[w] {
			inter++
		}
	}
	union := len(x) + len(y) - inter
	return float64(inter)/float64(union) >= 0.75
}
