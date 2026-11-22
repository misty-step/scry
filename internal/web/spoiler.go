package web

import (
	"fmt"
	"strings"
	"unicode"

	"scry/internal/store"
)

// The Study screen must not give an answer away before it is given
// (DESIGN.md, "No spoilers"). While a card waits, the rail names where the
// learner is without ever printing an idea's own title: starter sections
// used to be called things like "Alfa to India", which is two answers. It
// prints the card's topic (the top-level set or idea) and a neutral set
// number, and drops any part that shares a word with the answer to this
// card or to any card beside it. Hand-made and AI-drafted ideas go through
// the same guard, so a title that holds an answer falls back to "Set 1" or
// "Current set".
//
// The same holds for every cue on the screen (the prompt, the exact-text
// label, the "coming up" list and the scrollback): none quotes the answer
// still owed, however short it is. See quotesAnswer, cueWhile and
// waitingPrompt below.

// spoilerStop are words too common to count as giving anything away.
var spoilerStop = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "of": true, "to": true, "in": true, "on": true,
	"at": true, "by": true, "for": true, "from": true, "with": true, "is": true, "are": true, "was": true,
	"be": true, "it": true, "its": true, "as": true, "that": true, "this": true, "not": true, "no": true,
	"but": true, "than": true, "then": true, "into": true, "per": true,
}

// apostrophes drops apostrophes, so the letters after one stay in its word.
var apostrophes = strings.NewReplacer("'", "", "’", "")

// spoilerWords are the words of s that could give an answer away:
// lowercased runs of letters and digits, without common words, and with a
// plural "s" dropped so "Capitals" matches "capital". A letter standing
// alone is a word ("Vitamin C" names the answer "C"); an apostrophe joins
// its word, so "Newton's" is "Newton" and never a stray "s".
func spoilerWords(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(apostrophes.Replace(strings.ToLower(s)), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if spoilerStop[w] {
			continue
		}
		numeric := strings.IndexFunc(w, func(r rune) bool { return !unicode.IsDigit(r) }) < 0
		if !numeric && len([]rune(w)) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") {
			w = strings.TrimSuffix(w, "s")
		}
		out[w] = true
	}
	return out
}

// leaksAnswer reports whether label holds a word of any of the answers, or
// the whole of one: an answer made only of common words ("No", "It") is
// still given away by a label that says it.
func leaksAnswer(label string, answers []string) bool {
	if label == "" {
		return false
	}
	words := spoilerWords(label)
	for _, a := range answers {
		if quotesAnswer(label, a) {
			return true
		}
		for w := range spoilerWords(a) {
			if words[w] {
				return true
			}
		}
	}
	return false
}

// neutralSetLabel is the label that can never spoil: a set number for an
// idea inside a topic, "Current set" otherwise.
func neutralSetLabel(place store.IdeaPlace) string {
	if place.Nested && place.Ordinal > 0 {
		return fmt.Sprintf("Set %d", place.Ordinal)
	}
	return "Current set"
}

// spoilerSafeHeader is what the Study rail shows while the card is still
// to be answered. answers are the correct answers of the current card and
// of every card of the same idea. It tries, in order, the topic with the
// set number, the topic alone, the set number alone and "Current set", and
// takes the first that shares no word with any of the answers. Even the
// neutral labels are checked: "Set 3" beside a card whose answer is
// "3 teaspoons" would print a word of the answer. If every label does, the
// rail names no place at all and shows only the count.
func spoilerSafeHeader(place store.IdeaPlace, answers []string) string {
	topic := strings.TrimSpace(place.Topic)
	var candidates []string
	if place.Nested && place.Ordinal > 0 {
		set := neutralSetLabel(place)
		if topic != "" {
			candidates = append(candidates, topic+" · "+set, topic)
		}
		candidates = append(candidates, set)
	} else if topic != "" {
		candidates = append(candidates, topic)
	}
	candidates = append(candidates, "Current set")
	for _, label := range candidates {
		if !leaksAnswer(label, answers) {
			return label
		}
	}
	return ""
}

// revealedHeader is the real place, shown once the answer is in.
func revealedHeader(place store.IdeaPlace) string {
	if place.Nested && place.Topic != "" && place.Topic != place.Title {
		return place.Topic + " · " + place.Title
	}
	return place.Title
}

// phrase lowercases s and joins its runs of letters and digits with single
// spaces, so two texts compare whatever their case and punctuation.
func phrase(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

// quotesAnswer reports whether text holds the whole answer, word for word,
// whatever the answer's length: "Go" is quoted by "what follows “Go”" as
// surely as a long line is. Whole words only ("Go" is not in "Gothic"), and
// a leading article does not hide it ("The daffodils" is given away by
// "Daffodils"). The cue for a line of an exact text quotes the line before
// it, so the "coming up" list would print the answer to the card on screen.
func quotesAnswer(text, answer string) bool {
	a := phrase(answer)
	if a == "" {
		// An answer with no letter or digit ("* * *") is matched as written.
		raw := strings.ToLower(strings.TrimSpace(answer))
		return raw != "" && strings.Contains(strings.ToLower(text), raw)
	}
	t := " " + phrase(text) + " "
	if strings.Contains(t, " "+a+" ") {
		return true
	}
	for _, article := range []string{"the ", "a ", "an "} {
		if rest, ok := strings.CutPrefix(a, article); ok {
			return strings.Contains(t, " "+rest+" ")
		}
	}
	return false
}

// neutralExactTitle stands in for the title of an exact text wherever the
// title itself would give the waiting answer away.
const neutralExactTitle = "Exact text"

// titleGivesAway reports whether the title of the exact text it belongs to
// holds the whole of answer: a text called "Daffodils" whose first line is
// "Daffodils". A title that only shares a word with a line ("Preamble to
// the United States Constitution" and "We the People of the United
// States…") is the cue the learner needs to know which text is asked for,
// and stays.
func titleGivesAway(it store.Item, answer string) bool {
	return it.Exact && strings.TrimSpace(it.TextTitle) != "" && quotesAnswer(it.TextTitle, answer)
}

// cueWhile is the cue of it as it may be printed while answer is still
// owed: in the "coming up" list and in the scrollback. A cue that quotes
// the answer is held back (ok is false), unless it is only the text's own
// title that gives it away: then the title is replaced by a neutral one.
func cueWhile(it store.Item, answer string) (cue string, ok bool) {
	if !quotesAnswer(it.Prompt, answer) {
		return it.Prompt, true
	}
	if titleGivesAway(it, answer) {
		if rest, found := strings.CutPrefix(it.Prompt, it.TextTitle+": "); found {
			if cue = neutralExactTitle + ": " + rest; !quotesAnswer(cue, answer) {
				return cue, true
			}
		}
	}
	return "", false
}

// waitingPrompt keeps an exact-text cue only if the whole rendered prompt
// is safe, including after a title replacement. An edited cue can quote
// the answer elsewhere, and even a neutral fallback can be the answer.
func waitingPrompt(it store.Item) string {
	if !it.Exact {
		return it.Prompt
	}
	if cue, ok := cueWhile(it, it.Answer); ok {
		return cue
	}
	var candidates []string
	if it.LineNo > 0 {
		candidates = append(candidates, fmt.Sprintf("Line %d — recite from memory", it.LineNo), fmt.Sprintf("Recite line %d", it.LineNo))
	}
	candidates = append(candidates, "Recite this line from memory", "Next line", "…")
	for _, prompt := range candidates {
		if !quotesAnswer(prompt, it.Answer) {
			return prompt
		}
	}
	return ""
}
