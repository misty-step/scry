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

// spoilerStop are words too common to count as giving anything away.
var spoilerStop = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "of": true, "to": true, "in": true, "on": true,
	"at": true, "by": true, "for": true, "from": true, "with": true, "is": true, "are": true, "was": true,
	"be": true, "it": true, "its": true, "as": true, "that": true, "this": true, "not": true, "no": true,
	"but": true, "than": true, "then": true, "into": true, "per": true, "one": true, "two": true,
}

// spoilerWords are the words of s that could give an answer away:
// lowercased runs of letters and digits, without common words, and with a
// plural "s" dropped so "Capitals" matches "capital". A single letter is
// not a word here; a number of any length is.
func spoilerWords(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if spoilerStop[w] {
			continue
		}
		numeric := strings.IndexFunc(w, func(r rune) bool { return !unicode.IsDigit(r) }) < 0
		if len([]rune(w)) < 2 && !numeric {
			continue
		}
		if !numeric && len([]rune(w)) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") {
			w = strings.TrimSuffix(w, "s")
		}
		out[w] = true
	}
	return out
}

// leaksAnswer reports whether label holds a word of any of the answers.
func leaksAnswer(label string, answers []string) bool {
	words := spoilerWords(label)
	if len(words) == 0 {
		return false
	}
	for _, a := range answers {
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

// quotesAnswer reports whether text holds the whole answer, word for word.
// The cue for a line of an exact text quotes the line before it, so the
// "coming up" list would print the answer to the card on screen.
func quotesAnswer(text, answer string) bool {
	a := phrase(answer)
	if len([]rune(a)) < 3 {
		return false
	}
	return strings.Contains(" "+phrase(text)+" ", " "+a+" ")
}
