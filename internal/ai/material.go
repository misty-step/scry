package ai

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// maxTopicChars is the longest request still read as a topic, a question
// or a book's title; anything longer, or on several lines, is material
// the learner pasted (S106-2).
const maxTopicChars = 300

// IsMaterial reports whether a Create request is Markdown, prose or notes
// the learner pasted, rather than a topic, a question or a book's title.
func IsMaterial(query string) bool {
	return strings.Contains(query, "\n") || utf8.RuneCountInString(query) > maxTopicChars
}

var (
	mdLink      = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdLead      = regexp.MustCompile(`^(?:#{1,6}\s+|>\s*|[-*+]\s+(?:\[[ xX]\]\s+)?|\d+[.)]\s+)+`)
	mdEmphasis  = strings.NewReplacer("**", "", "__", "", "`", "", "~~", "")
	sentenceEnd = regexp.MustCompile(`([.!?])\s+`)
	// *italic* and _italic_ around words, not a lone * or snake_case.
	mdItalic = regexp.MustCompile(`(^|[^\pL\pN*_])[*_]([^*_\s][^*_]*?)[*_]($|[^\pL\pN*_])`)
)

// PlainLine is one line of Markdown as plain words: its heading, quote,
// list or checkbox marker, emphasis and link targets dropped, and its
// whitespace collapsed.
func PlainLine(s string) string {
	s = strings.TrimSpace(s)
	s = mdLead.ReplaceAllString(s, "")
	s = mdLink.ReplaceAllString(s, "$1")
	s = mdEmphasis.Replace(s)
	s = mdItalic.ReplaceAllString(s, "$1$2$3")
	return strings.Join(strings.Fields(s), " ")
}

// headingLevel is the level of a Markdown ATX heading line ("## Notes" is
// 2), or 0 for any other line.
func headingLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(line) || (line[n] != ' ' && line[n] != '\t') {
		return 0
	}
	return n
}

// Clip cuts s to at most n characters, at a word where it can.
func Clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n-1])
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.-–—") + "…"
}

type materialSection struct {
	title    string
	points   []string
	preamble bool // the lines before the first heading
}

// materialDraft is the test model's fixed outline for pasted material
// (S106-2): no network, no model, the same cards every time. Ideas come
// from the Markdown headings (at most four; a leading # heading names the
// whole draft), or, for prose with no headings, from its paragraphs. Each
// idea gets up to three atomic cards written to fixed templates from the
// lines under it, so a typical note yields at least two ideas with cards.
func materialDraft(q string) (DraftHead, []DraftIdea) {
	title := ""
	var secs []materialSection
	headings := false
	for _, raw := range strings.Split(q, "\n") {
		if headingLevel(strings.TrimSpace(raw)) > 0 {
			headings = true
			break
		}
	}
	if headings {
		for _, raw := range strings.Split(q, "\n") {
			line := strings.TrimSpace(raw)
			if line == "" || strings.HasPrefix(line, "```") || strings.Trim(line, "-*_= ") == "" {
				continue
			}
			if lvl := headingLevel(line); lvl > 0 {
				text := PlainLine(line)
				if lvl == 1 && title == "" && len(secs) == 0 {
					title = text
					continue
				}
				secs = append(secs, materialSection{title: text})
				continue
			}
			if len(secs) == 0 {
				secs = append(secs, materialSection{title: "Overview", preamble: true})
			}
			secs[len(secs)-1].points = append(secs[len(secs)-1].points, sentences(PlainLine(line))...)
		}
	} else {
		for _, para := range strings.Split(strings.ReplaceAll(q, "\r", ""), "\n\n") {
			text := PlainLine(strings.Join(strings.Fields(para), " "))
			if text == "" {
				continue
			}
			if title == "" {
				title = Clip(text, 80)
			}
			secs = append(secs, materialSection{title: Clip(strings.Join(firstWords(text, 6), " "), 48), points: sentences(text)})
		}
	}
	// Lines before the first heading introduce the note; they are an idea
	// of their own only when nothing else is.
	if len(secs) > 1 && secs[0].preamble {
		secs = secs[1:]
	}
	var kept []materialSection
	for _, s := range secs {
		if len(s.points) > 0 && s.title != "" {
			kept = append(kept, s)
		}
	}
	// One section with several points still makes two ideas.
	if len(kept) == 1 && len(kept[0].points) >= 2 {
		s := kept[0]
		half := (len(s.points) + 1) / 2
		kept = []materialSection{{title: s.title, points: s.points[:half]}, {title: s.title + ", continued", points: s.points[half:]}}
	}
	if len(kept) > 4 {
		kept = kept[:4]
	}
	if title == "" && len(kept) > 0 {
		title = kept[0].title
	}
	head := DraftHead{Kind: "topic", Title: Clip(title, 80)}
	var ideas []DraftIdea
	for i, s := range kept {
		idea := DraftIdea{Title: Clip(s.title, 48), Summary: Clip(s.points[0], 140)}
		if i+1 < len(kept) {
			idea.Links = []string{Clip(kept[i+1].title, 48)}
		} else if i > 0 {
			idea.Links = []string{Clip(kept[i-1].title, 48)}
		}
		for _, p := range s.points {
			if len(idea.Items) == 3 {
				break
			}
			if it, ok := materialCard(idea.Title, p, len(idea.Items)+1); ok {
				idea.Items = append(idea.Items, it)
			}
		}
		if len(idea.Items) > 0 {
			ideas = append(ideas, idea)
		}
	}
	return head, ideas
}

// sentences splits a line of prose at its sentence ends.
func sentences(text string) []string {
	var out []string
	for _, part := range sentenceEnd.Split(text, -1) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func firstWords(s string, n int) []string {
	w := strings.Fields(s)
	if len(w) > n {
		w = w[:n]
	}
	return w
}

var ordinals = []string{"", "first", "second", "third"}

// materialCard writes one fixed-template card from a point under an idea:
// "term: definition" asks for the definition, "X is Y" asks what X is,
// a longer point asks for how it ends, and a short one for its place.
func materialCard(idea, point string, n int) (DraftItem, bool) {
	point = strings.TrimRight(point, ".!?;:, ")
	words := strings.Fields(point)
	if len(words) == 0 {
		return DraftItem{}, false
	}
	if term, def, ok := strings.Cut(point, ": "); ok && len(strings.Fields(term)) <= 6 && strings.TrimSpace(def) != "" {
		return DraftItem{Question: Clip("In your notes on “"+idea+"”, what is "+strings.TrimSpace(term)+"?", 300), Answer: Clip(strings.TrimSpace(def), 200)}, true
	}
	for _, verb := range []string{" is ", " are "} {
		if left, right, ok := strings.Cut(point, verb); ok && len(strings.Fields(left)) <= 8 && len(strings.Fields(right)) >= 1 {
			return DraftItem{Question: Clip("In your notes on “"+idea+"”, what"+verb+lowerFirst(left)+"?", 300), Answer: Clip(strings.TrimSpace(right), 200)}, true
		}
	}
	if len(words) >= 6 {
		half := len(words) / 2
		return DraftItem{Question: Clip("In your notes on “"+idea+"”, how does this end: “"+strings.Join(words[:half], " ")+" …”?", 300),
			Answer: Clip(strings.Join(words[half:], " "), 200)}, true
	}
	return DraftItem{Question: "What is the " + ordinals[n] + " point in your notes on “" + idea + "”?", Answer: Clip(point, 200)}, true
}

func lowerFirst(s string) string {
	s = strings.TrimSpace(s)
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError || size == len(s) {
		return s
	}
	// Keep acronyms and names ("HTTPS is", "Madison is") as written.
	if next, _ := utf8.DecodeRuneInString(s[size:]); next >= 'A' && next <= 'Z' {
		return s
	}
	if r >= 'A' && r <= 'Z' && strings.Contains(s, " ") {
		return strings.ToLower(string(r)) + s[size:]
	}
	return s
}
