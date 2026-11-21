package evals

import (
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// foldDiacritics maps the accented Latin letters the datasets use (and their
// common neighbours) to plain ASCII, so "Gödel" and "Godel" match.
var foldDiacritics = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ñ", "n", "ç", "c", "ß", "ss",
)

// numberWords turns the spelled-out numbers a transcriber may choose into
// digits, so "eight tablespoons" and "8 tablespoons" are the same words.
var numberWords = map[string]string{
	"zero": "0", "one": "1", "two": "2", "three": "3", "four": "4", "five": "5", "six": "6", "seven": "7",
	"eight": "8", "nine": "9", "ten": "10", "eleven": "11", "twelve": "12", "thirteen": "13", "fourteen": "14",
	"fifteen": "15", "sixteen": "16", "seventeen": "17", "eighteen": "18", "nineteen": "19", "twenty": "20",
	"thirty": "30", "forty": "40", "fifty": "50", "sixty": "60", "seventy": "70", "eighty": "80", "ninety": "90",
}

// NormalizeWords is the WER normalization: lowercase, diacritics folded,
// apostrophes and hyphens dropped (can't -> cant, multi-version ->
// multiversion), every other non-letter/digit is a
// word break, and number words 0-20 and the tens become digits.
func NormalizeWords(s string) []string {
	s = foldDiacritics.Replace(strings.ToLower(s))
	s = strings.NewReplacer("'", "", "’", "", "‘", "", "-", "", "‐", "").Replace(s)
	words := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for i, w := range words {
		if d, ok := numberWords[w]; ok {
			words[i] = d
		}
	}
	return words
}

// WER is the word error rate of hyp against ref after NormalizeWords:
// (substitutions + deletions + insertions) / reference words. An empty
// reference scores 0 for an empty hypothesis and 1 otherwise.
func WER(ref, hyp string) float64 {
	r, h := NormalizeWords(ref), NormalizeWords(hyp)
	if len(r) == 0 {
		if len(h) == 0 {
			return 0
		}
		return 1
	}
	prev := make([]int, len(h)+1)
	cur := make([]int, len(h)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(r); i++ {
		cur[0] = i
		for j := 1; j <= len(h); j++ {
			sub := prev[j-1]
			if r[i-1] != h[j-1] {
				sub++
			}
			cur[j] = min(sub, prev[j]+1, cur[j-1]+1)
		}
		prev, cur = cur, prev
	}
	return float64(prev[len(h)]) / float64(len(r))
}

// TermPresent reports whether a critical term survived transcription: its
// normalized words appear contiguously in the hypothesis, or (for terms of
// at least four characters) the term with spaces removed appears in the
// hypothesis with spaces removed, so "M.V.C.C." still counts as MVCC.
func TermPresent(term, hyp string) bool {
	t, h := NormalizeWords(term), NormalizeWords(hyp)
	if len(t) == 0 {
		return false
	}
	for i := 0; i+len(t) <= len(h); i++ {
		match := true
		for j := range t {
			if h[i+j] != t[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	compact := strings.Join(t, "")
	return len(compact) >= 4 && strings.Contains(strings.Join(h, ""), compact)
}

// Percentile is the nearest-rank percentile (p in 0..1) of xs; 0 when empty.
func Percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	i := int(math.Ceil(p*float64(len(s)))) - 1
	if i < 0 {
		i = 0
	}
	return s[i]
}

// ErrBudget is returned when a call would take spend past the cap.
var ErrBudget = errors.New("spend cap reached: call not dispatched")

// Budget is the live spend circuit-breaker. Every call reserves a
// worst-case-ish estimate before it is dispatched, and is refused when
// spent + in-flight reservations + estimate would pass Max. Settling
// replaces the reservation with the provider's exact usage.cost; a call
// whose cost was not reported is charged its estimate, so unknown costs
// can only make the breaker trip earlier, never later.
type Budget struct {
	Max float64

	mu       sync.Mutex
	spent    float64 // exact usage.cost plus estimates charged for unknown costs
	exact    float64 // exact usage.cost only
	reserved float64
	calls    int
	unknown  int
	refused  int
}

// Ticket is one reserved call.
type Ticket struct {
	b   *Budget
	est float64
}

// Reserve asks to dispatch a call estimated to cost est dollars.
func (b *Budget) Reserve(est float64) (*Ticket, error) {
	if est < 0 || math.IsNaN(est) {
		est = 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.spent+b.reserved+est > b.Max {
		b.refused++
		return nil, ErrBudget
	}
	b.reserved += est
	return &Ticket{b: b, est: est}, nil
}

// Settle records the call's exact cost, or nil when none was reported.
func (t *Ticket) Settle(cost *float64) {
	b := t.b
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reserved -= t.est
	b.calls++
	if cost == nil {
		b.unknown++
		b.spent += t.est
		return
	}
	b.spent += *cost
	b.exact += *cost
}

// BudgetState is a snapshot for the summary.
type BudgetState struct {
	MaxUSD             float64 `json:"max_spend_usd"`
	ExactUSD           float64 `json:"exact_usage_cost_usd"`
	ChargedUSD         float64 `json:"guard_charged_usd"`
	Calls              int     `json:"settled_calls"`
	UnknownCostCalls   int     `json:"unknown_cost_calls"`
	RefusedForBudget   int     `json:"refused_for_budget"`
	ReservedInFlightUS float64 `json:"reserved_in_flight_usd"`
}

func (b *Budget) State() BudgetState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return BudgetState{MaxUSD: b.Max, ExactUSD: b.exact, ChargedUSD: b.spent, Calls: b.calls, UnknownCostCalls: b.unknown, RefusedForBudget: b.refused, ReservedInFlightUS: b.reserved}
}
