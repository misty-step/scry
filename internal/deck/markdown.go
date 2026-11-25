package deck

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ExportHeading is the first line of Scry's Markdown export.
const ExportHeading = "# Scry export"

// WriteMarkdown writes cards in Scry's Markdown export format. Cards keep
// their order; consecutive cards of the same idea share one heading.
//
// # The format
//
// The file starts with the line "# Scry export". Each idea is a level-two
// heading, "## <idea title>" ("##" alone for a card with no idea), followed
// by its cards. A card is a "Q: " line with the question, an "A: " line with
// the answer and, for a card judged word for word, the line "exact: true".
// Cards are separated by a line "---" and blank lines may stand anywhere
// between cards:
//
//	# Scry export
//
//	## The Federalist Papers
//
//	Q: Who wrote Federalist No. 10?
//	A: James Madison
//
//	---
//
//	Q: What does it argue?
//	A: A large republic
//	  controls faction better
//	  than a small one.
//	exact: true
//
// A question or answer of several lines continues on lines indented by two
// spaces; a blank line inside one is written as an empty line and belongs
// to it when an indented line follows. The two spaces are not part of the
// text, so a line of the text may itself start with spaces, "#", "---",
// "Q:" or anything else, and nothing in a question or answer can be read
// as structure: structure lines are never indented. Questions, answers and
// idea titles are written in their clean form (trimmed, "\n" line endings,
// an idea title on one line), so an export reads back with every idea,
// question, answer and exact flag intact, in order.
func WriteMarkdown(w io.Writer, cards []Card) error {
	bw := bufio.NewWriter(w)
	bw.WriteString(ExportHeading + "\n")
	idea, started := "", false
	for _, c := range cards {
		c = clean(c)
		if !started || c.Idea != idea {
			idea, started = c.Idea, true
			if c.Idea == "" {
				bw.WriteString("\n##\n\n")
			} else {
				bw.WriteString("\n## " + c.Idea + "\n\n")
			}
		} else {
			bw.WriteString("\n---\n\n")
		}
		field(bw, "Q: ", c.Question)
		field(bw, "A: ", c.Answer)
		if c.Exact {
			bw.WriteString("exact: true\n")
		}
	}
	return bw.Flush()
}

func field(w *bufio.Writer, label, v string) {
	for i, l := range strings.Split(v, "\n") {
		switch {
		case i == 0:
			w.WriteString(label + l)
		case l == "":
			// An empty line, kept free of trailing spaces.
		default:
			w.WriteString("  " + l)
		}
		w.WriteByte('\n')
	}
}

// ParseMarkdown reads a Markdown file. It returns structured=true with the
// cards when the text is Scry's export format or simple card blocks, and
// structured=false (no cards, no error) for anything else, which the caller
// sends to the drafting agent as material.
//
// Simple card blocks are text split on lines that are exactly "---". Each
// block is split at its first "?": the question runs to and includes the
// "?", the answer is the rest. The text is structured when there is at
// least one "---" line and every non-empty block splits into a question and
// an answer without a Markdown heading among them; a "# Title" as the first
// line of the file names the idea of every card.
//
// A structured file with a card over the limits, or a Scry export that has
// been damaged, is refused in words (structured=true and an error).
func ParseMarkdown(s string) (cards []Card, structured bool, err error) {
	s, err = text([]byte(s))
	if err != nil {
		return nil, false, err
	}
	lines := strings.Split(s, "\n")
	firstLine := 0
	for firstLine < len(lines) && strings.TrimSpace(lines[firstLine]) == "" {
		firstLine++
	}
	if firstLine < len(lines) && strings.EqualFold(strings.TrimSpace(lines[firstLine]), ExportHeading) {
		cards, err := parseExport(lines, firstLine+1)
		return cards, true, err
	}
	return parseBlocks(lines, firstLine)
}

var heading = regexp.MustCompile(`^ {0,3}#{1,6}(\s|$)`)

func parseBlocks(lines []string, firstLine int) ([]Card, bool, error) {
	idea := ""
	if firstLine < len(lines) {
		if t, ok := strings.CutPrefix(strings.TrimSpace(lines[firstLine]), "# "); ok {
			idea = strings.TrimSpace(t)
			lines = lines[firstLine+1:]
		}
	}
	var blocks [][]string
	var cur []string
	seps := 0
	for _, l := range lines {
		if strings.TrimRight(l, " \t") == "---" {
			blocks, cur = append(blocks, cur), nil
			seps++
			continue
		}
		cur = append(cur, l)
	}
	blocks = append(blocks, cur)
	if seps == 0 {
		return nil, false, nil
	}
	var cards []Card
	for _, b := range blocks {
		body := strings.TrimSpace(strings.Join(b, "\n"))
		if body == "" {
			continue
		}
		for _, l := range b {
			if heading.MatchString(l) {
				return nil, false, nil
			}
		}
		q, a, ok := strings.Cut(body, "?")
		if !ok {
			return nil, false, nil
		}
		c := clean(Card{Idea: idea, Question: q + "?", Answer: a})
		if strings.TrimSpace(q) == "" || c.Answer == "" {
			return nil, false, nil
		}
		cards = append(cards, c)
	}
	if len(cards) == 0 {
		return nil, false, nil
	}
	if len(cards) > MaxCards {
		return nil, true, tooMany()
	}
	for i, c := range cards {
		if err := check(c, fmt.Sprintf("Card %d", i+1)); err != nil {
			return nil, true, err
		}
	}
	return cards, true, nil
}

// parseExport reads Scry's export format from lines[start:].
func parseExport(lines []string, start int) ([]Card, error) {
	var cards []Card
	idea, inIdea := "", false
	var c *Card        // the card being read
	hasAnswer := false // c has had its "A:" line
	var f *string      // the field being read
	pendingBlank := 0  // blank lines that may still belong to f
	damaged := func(i int, why string) error {
		return fmt.Errorf("Line %d of this Scry export %s, so Scry can't read it. Export it again, or fix that line.", i+1, why)
	}
	finish := func(i int) error {
		if c == nil {
			return nil
		}
		if !hasAnswer {
			return damaged(i, "follows a question that has no answer")
		}
		*c = clean(*c)
		if err := check(*c, fmt.Sprintf("Card %d", len(cards)+1)); err != nil {
			return err
		}
		if len(cards) == MaxCards {
			return tooMany()
		}
		cards = append(cards, *c)
		c, f, hasAnswer = nil, nil, false
		return nil
	}
	for i := start; i < len(lines); i++ {
		l := lines[i]
		if rest, ok := strings.CutPrefix(l, "  "); ok {
			if f == nil {
				if strings.TrimSpace(rest) == "" {
					continue // stray spaces between cards
				}
				return nil, damaged(i, "is indented but doesn't continue a question or answer")
			}
			*f += strings.Repeat("\n", pendingBlank+1) + rest
			pendingBlank = 0
			continue
		}
		if strings.TrimSpace(l) == "" {
			if f != nil {
				pendingBlank++
			}
			continue
		}
		pendingBlank, f = 0, nil
		switch {
		case l == "##" || strings.HasPrefix(l, "## "):
			if err := finish(i); err != nil {
				return nil, err
			}
			idea, inIdea = strings.TrimSpace(l[2:]), true
		case l == "---":
			if err := finish(i); err != nil {
				return nil, err
			}
		case strings.HasPrefix(l, "Q:"):
			if !inIdea {
				return nil, damaged(i, "has a question before any \"## \" idea heading")
			}
			if err := finish(i); err != nil {
				return nil, err
			}
			c = &Card{Idea: idea, Question: label(l, "Q:")}
			f = &c.Question
		case strings.HasPrefix(l, "A:"):
			if c == nil || hasAnswer {
				return nil, damaged(i, "has an answer without a question")
			}
			c.Answer, hasAnswer = label(l, "A:"), true
			f = &c.Answer
		case l == "exact: true" || l == "exact: false":
			if !hasAnswer {
				return nil, damaged(i, "marks a card exact before its answer")
			}
			c.Exact = l == "exact: true"
		default:
			return nil, damaged(i, "isn't an idea heading, a Q: or A: line, \"exact: true\" or \"---\"")
		}
	}
	if err := finish(len(lines) - 1); err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, errors.New("This Scry export has no cards in it.")
	}
	return cards, nil
}

func label(l, prefix string) string {
	return strings.TrimPrefix(strings.TrimPrefix(l, prefix), " ")
}
