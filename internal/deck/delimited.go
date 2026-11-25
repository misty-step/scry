package deck

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Columns of a delimited file.
const (
	colIdea = iota
	colQuestion
	colAnswer
	colExact
)

// headerNames are the column names a header row may use.
var headerNames = map[string]int{
	"idea": colIdea, "question": colQuestion, "answer": colAnswer, "exact": colExact,
	"front": colQuestion, "back": colAnswer,
}

// ParseDelimited reads UTF-8 CSV (comma ',') or TSV (comma '\t').
//
// Without a header, a row of 2 columns is question,answer; 3 is
// idea,question,answer; 4 is idea,question,answer,exact. Empty trailing
// columns are ignored when counting. A first row that names its columns
// (question,answer or idea,question,answer[,exact], in any case and order)
// is a header and sets the columns for every row. Blank rows are skipped.
// The idea may be empty (the caller supplies a default). exact is true for
// 1, true, yes, y or exact and false for empty, 0, false, no or n.
func ParseDelimited(data []byte, comma rune) ([]Card, error) {
	s, err := text(data)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(strings.NewReader(s))
	r.Comma = comma
	r.FieldsPerRecord = -1
	r.ReuseRecord = true
	// Tab-separated files rarely quote; a stray quote in one is text.
	r.LazyQuotes = comma != ','
	var cards []Card
	var layout []int // from a header row
	first := true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return nil, fmt.Errorf("Row %d isn't valid %s: check its quotes.", pe.StartLine, kindName(comma))
			}
			return nil, fmt.Errorf("That file isn't valid %s.", kindName(comma))
		}
		row, _ := r.FieldPos(0)
		where := fmt.Sprintf("Row %d", row)
		if blank(rec) {
			continue
		}
		if first {
			first = false
			if l, ok := header(rec); ok {
				layout = l
				continue
			}
		}
		var c Card
		if layout != nil {
			if n := len(trimTrailing(rec)); n > len(layout) {
				return nil, fmt.Errorf("%s has %d columns, but the header names %d.", where, n, len(layout))
			}
			for i, v := range rec {
				if i < len(layout) {
					if err := set(&c, layout[i], v, where); err != nil {
						return nil, err
					}
				}
			}
		} else {
			fields := trimTrailing(rec)
			var cols []int
			switch len(fields) {
			case 2:
				cols = []int{colQuestion, colAnswer}
			case 3:
				cols = []int{colIdea, colQuestion, colAnswer}
			case 4:
				cols = []int{colIdea, colQuestion, colAnswer, colExact}
			case 1:
				return nil, fmt.Errorf("%s has only one column. Each row needs a question and an answer: question,answer or idea,question,answer,exact.", where)
			default:
				return nil, fmt.Errorf("%s has %d columns. Scry reads at most 4: idea,question,answer,exact.", where, len(fields))
			}
			for i, v := range fields {
				if err := set(&c, cols[i], v, where); err != nil {
					return nil, err
				}
			}
		}
		c = clean(c)
		if err := check(c, where); err != nil {
			return nil, err
		}
		if len(cards) == MaxCards {
			return nil, tooMany()
		}
		cards = append(cards, c)
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("That file has no cards in it. Each row needs a question and an answer.")
	}
	return cards, nil
}

func kindName(comma rune) string {
	if comma == ',' {
		return "CSV"
	}
	return "TSV"
}

func set(c *Card, col int, v, where string) error {
	switch col {
	case colIdea:
		c.Idea = v
	case colQuestion:
		c.Question = v
	case colAnswer:
		c.Answer = v
	case colExact:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y", "exact":
			c.Exact = true
		case "", "0", "false", "no", "n":
			c.Exact = false
		default:
			return fmt.Errorf("%s's exact column says %q. Use true or false.", where, truncate(strings.TrimSpace(v), 40))
		}
	}
	return nil
}

func blank(rec []string) bool {
	for _, v := range rec {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

func trimTrailing(rec []string) []string {
	n := len(rec)
	for n > 0 && strings.TrimSpace(rec[n-1]) == "" {
		n--
	}
	return rec[:n]
}

// header reads a row of column names. It is a header only when every
// cell names a column, no column is named twice, and both question and
// answer are there.
func header(rec []string) ([]int, bool) {
	fields := trimTrailing(rec)
	seen := map[int]bool{}
	var layout []int
	for _, v := range fields {
		col, ok := headerNames[strings.ToLower(strings.TrimSpace(v))]
		if !ok || seen[col] {
			return nil, false
		}
		seen[col] = true
		layout = append(layout, col)
	}
	if !seen[colQuestion] || !seen[colAnswer] {
		return nil, false
	}
	return layout, true
}

// WriteCSV writes cards as CSV with the header idea,question,answer,exact.
// Fields are written in their clean form, so ParseDelimited reads back the
// same cards.
func WriteCSV(w io.Writer, cards []Card) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"idea", "question", "answer", "exact"}); err != nil {
		return err
	}
	for _, c := range cards {
		c = clean(c)
		exact := "false"
		if c.Exact {
			exact = "true"
		}
		if err := cw.Write([]string{c.Idea, c.Question, c.Answer, exact}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
