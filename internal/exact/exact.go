// Package exact judges texts the learner marked exact, word for word.
// It is the one place Scry decides correctness without a model, and it
// does so only by comparing words in order.
package exact

import (
	"strings"
	"unicode"
)

// Kind says what happened to one word.
type Kind string

const (
	Same    Kind = "same"    // the learner's word matches
	Missing Kind = "missing" // a word of the text the learner left out
	Wrong   Kind = "wrong"   // the learner's word, with the right word in Fix
	Extra   Kind = "extra"   // a word the learner added
)

// Op is one word of the judged text, in reading order.
type Op struct {
	Kind Kind   `json:"kind"`
	Word string `json:"word"`
	Fix  string `json:"fix,omitempty"`
}

// Result is a word-for-word judgment.
type Result struct {
	Ops     []Op `json:"ops"`
	Missing int  `json:"missing"`
	Swapped int  `json:"swapped"`
	Extra   int  `json:"extra"`
	Words   int  `json:"words"` // words in the text
}

// norm folds a word for comparison: case and surrounding punctuation are
// not words, so they do not count as differences.
func norm(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '’', '‘', 'ʼ':
			return '\''
		}
		return unicode.ToLower(r)
	}, s)
	return strings.TrimFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func words(s string) []string {
	var out []string
	for _, f := range strings.Fields(s) {
		if norm(f) != "" {
			out = append(out, f)
		}
	}
	return out
}

// Diff compares the learner's answer with the text, word by word.
func Diff(text, answer string) Result {
	e, a := words(text), words(answer)
	n, m := len(e), len(a)
	// Longest common subsequence over normalized words.
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if norm(e[i]) == norm(a[j]) {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	res := Result{Words: n}
	var eRun, aRun []string
	flush := func() {
		k := min(len(eRun), len(aRun))
		for x := 0; x < k; x++ {
			res.Ops = append(res.Ops, Op{Kind: Wrong, Word: aRun[x], Fix: eRun[x]})
			res.Swapped++
		}
		for _, wd := range eRun[k:] {
			res.Ops = append(res.Ops, Op{Kind: Missing, Word: wd})
			res.Missing++
		}
		for _, wd := range aRun[k:] {
			res.Ops = append(res.Ops, Op{Kind: Extra, Word: wd})
			res.Extra++
		}
		eRun, aRun = nil, nil
	}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case norm(e[i]) == norm(a[j]):
			flush()
			res.Ops = append(res.Ops, Op{Kind: Same, Word: e[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			eRun = append(eRun, e[i])
			i++
		default:
			aRun = append(aRun, a[j])
			j++
		}
	}
	eRun = append(eRun, e[i:]...)
	aRun = append(aRun, a[j:]...)
	flush()
	return res
}

// Differences is the number of words that differ.
func (r Result) Differences() int { return r.Missing + r.Swapped + r.Extra }

// Perfect reports a word-for-word match.
func (r Result) Perfect() bool { return r.Differences() == 0 }

// Verdict grades the attempt: right only when word for word; close when
// at most two words (or a tenth of the text) differ; otherwise missed.
func (r Result) Verdict() string {
	d := r.Differences()
	switch {
	case d == 0:
		return "right"
	default:
		return "missed"
	}
}

var numerals = []string{"no", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve"}

func count(n int, one, many string) string {
	num := ""
	if n < len(numerals) {
		num = numerals[n]
	} else {
		num = itoa(n)
	}
	if n == 1 {
		return num + " " + one
	}
	return num + " " + many
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// Why counts the differences in words, as one line.
func (r Result) Why() string {
	if r.Perfect() {
		return "Word for word."
	}
	var parts []string
	if r.Missing > 0 {
		parts = append(parts, count(r.Missing, "word missing", "words missing"))
	}
	if r.Swapped > 0 {
		if len(parts) == 0 {
			parts = append(parts, count(r.Swapped, "word swapped", "words swapped"))
		} else {
			parts = append(parts, count(r.Swapped, "swapped", "swapped"))
		}
	}
	if r.Extra > 0 {
		if len(parts) == 0 {
			parts = append(parts, count(r.Extra, "extra word", "extra words"))
		} else {
			parts = append(parts, count(r.Extra, "extra", "extra"))
		}
	}
	s := strings.Join(parts, ", ") + "."
	return strings.ToUpper(s[:1]) + s[1:]
}
