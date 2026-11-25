// Package deck reads and writes decks of cards in the formats a learner
// brings to Scry or takes away from it (S106-3): Anki .apkg packages,
// UTF-8 CSV and TSV, and Markdown (Scry's own export format, or simple
// `---` card blocks).
//
// The package knows nothing of the database: it turns bytes into Cards and
// Cards into bytes. Every error it returns is a plain sentence meant to be
// shown to the learner as it is ("refused in words").
package deck

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Card is one question and its answer, with the idea it belongs to. An
// empty Idea means "the caller's default idea". Exact cards are judged word
// for word.
type Card struct {
	Idea     string
	Question string
	Answer   string
	Exact    bool
}

// The kinds of file Scry imports.
const (
	KindAPKG = "apkg"
	KindCSV  = "csv"
	KindTSV  = "tsv"
	KindMD   = "md"
)

// Limits. They match what the library accepts for a hand-made card.
const (
	MaxBytes         = 20 << 20 // the upload cap
	MaxCards         = 5000     // cards in one import
	MaxQuestionBytes = 1000
	MaxAnswerBytes   = 2000
	MaxIdeaBytes     = 120
)

// DefaultAnkiIdea names the idea for Anki notes whose deck has no useful
// name (missing, or Anki's own "Default").
const DefaultAnkiIdea = "Anki import"

// KindOf picks the importer for a file by its extension, ignoring case.
func KindOf(filename string) (string, error) {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(filename))) {
	case ".apkg":
		return KindAPKG, nil
	case ".csv":
		return KindCSV, nil
	case ".tsv", ".tab":
		return KindTSV, nil
	case ".md", ".markdown":
		return KindMD, nil
	case ".colpkg":
		return "", errors.New("That's a whole Anki collection (.colpkg). In Anki, export a deck as an .apkg package instead.")
	case "":
		return "", errors.New("Scry can't tell what kind of file that is. Choose an Anki .apkg, a .csv or .tsv, or a Markdown .md file.")
	default:
		return "", errors.New("Scry can import an Anki .apkg, a .csv or .tsv, or a Markdown .md file. Choose one of those.")
	}
}

var errTooBig = errors.New("That file is larger than 20 MiB. Split it into smaller files and import each one.")

// text checks raw bytes are UTF-8 text and returns them without a byte
// order mark, with line endings made "\n".
func text(data []byte) (string, error) {
	if len(data) > MaxBytes {
		return "", errTooBig
	}
	s := strings.TrimPrefix(string(data), "\uFEFF")
	if !utf8.ValidString(s) {
		return "", errors.New("That file isn't UTF-8 text. Save it as UTF-8 and import it again.")
	}
	return newlines(s), nil
}

// newlines turns "\r\n" and lone "\r" into "\n".
func newlines(s string) string {
	if !strings.Contains(s, "\r") {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// clean is the normal form of a card: line endings made "\n", every field
// trimmed, and the idea title kept to one line. Importers store exactly
// this, and writers write it, so an export reads back unchanged.
func clean(c Card) Card {
	c.Idea = strings.TrimSpace(strings.NewReplacer("\n", " ", "\t", " ").Replace(newlines(c.Idea)))
	c.Question = strings.TrimSpace(newlines(c.Question))
	c.Answer = strings.TrimSpace(newlines(c.Answer))
	return c
}

// Clean returns the card as an importer would store it: trimmed, with
// "\n" line endings and a one-line idea title.
func Clean(c Card) Card { return clean(c) }

// check refuses a card that is empty or over a limit. where names it for
// the learner ("Row 12", "Card 3").
func check(c Card, where string) error {
	switch {
	case c.Question == "" && c.Answer == "":
		return fmt.Errorf("%s has no question or answer.", where)
	case c.Question == "":
		return fmt.Errorf("%s has no question.", where)
	case c.Answer == "":
		return fmt.Errorf("%s has no answer.", where)
	case len(c.Question) > MaxQuestionBytes:
		return fmt.Errorf("%s's question is longer than 1,000 characters.", where)
	case len(c.Answer) > MaxAnswerBytes:
		return fmt.Errorf("%s's answer is longer than 2,000 characters.", where)
	case len(c.Idea) > MaxIdeaBytes:
		return fmt.Errorf("%s's idea title is longer than 120 characters.", where)
	}
	return nil
}

// fits reports whether a card can be stored as it is.
func fits(c Card) bool { return check(c, "") == nil }

func tooMany() error {
	return fmt.Errorf("That file holds more than %s cards. Split it into smaller files and import each one.", commas(MaxCards))
}

func commas(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// truncate cuts s to at most n bytes without splitting a character.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return strings.TrimSpace(s[:n])
}
