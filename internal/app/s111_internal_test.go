package app

import (
	"testing"
	"time"

	"scry/internal/srs"
	"scry/internal/store"
)

// [S111-1] The plan's ranking and ordering rules, on their own: equal
// retrievability falls back to the earliest due; budget room goes to due
// reviews before new cards, an exact line that no longer fits is passed
// over for an ordinary card that does; a card already presented today
// costs no first presentation; and the last deck repeats only when no
// other deck has a card, due reviews first within it.
func TestS111_1_PlanRanksAndOrdersCards(t *testing.T) {
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	review := func(id, deck int64, last time.Duration, due time.Duration) store.StudyCandidate {
		return store.StudyCandidate{Deck: deck, Item: store.Item{ID: id, IdeaID: deck,
			Sched: srs.State{Reps: 1, Stability: 3, Difficulty: 5, Last: now.Add(-last), Due: now.Add(-due)}}}
	}
	fresh := func(id, deck int64, exact bool) store.StudyCandidate {
		return store.StudyCandidate{Deck: deck, Item: store.Item{ID: id, IdeaID: deck, Exact: exact}}
	}
	ids := func(p []planned) []int64 {
		var out []int64
		for _, c := range p {
			out = append(out, c.ID)
		}
		return out
	}
	eq := func(a, b []int64) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	// Equal retrievability (same stability, same last review): the earlier
	// due wins; the deck rule then alternates.
	got := ids(planSession([]store.StudyCandidate{
		review(1, 10, 72*time.Hour, time.Hour),
		review(2, 10, 72*time.Hour, 3*time.Hour),
		review(3, 20, 72*time.Hour, 2*time.Hour),
	}, nil, now, 300, 10, nil))
	if want := []int64{2, 3, 1}; !eq(got, want) {
		t.Fatalf("ties by due: %v, want %v", got, want)
	}

	// Only deck 10 has reviews left: new cards of deck 20 break its streak;
	// with none left, deck 10 repeats, reviews before its own new card.
	got = ids(planSession([]store.StudyCandidate{
		review(1, 10, 96*time.Hour, time.Hour),
		review(2, 10, 72*time.Hour, time.Hour),
		review(3, 10, 48*time.Hour, time.Hour),
	}, []store.StudyCandidate{fresh(4, 20, false), fresh(5, 10, false)}, now, 300, 10, nil))
	if want := []int64{1, 4, 2, 3, 5}; !eq(got, want) {
		t.Fatalf("streak breaking: %v, want %v", got, want)
	}

	// Budget: 14 reviews spend 280 s; a 30 s exact line no longer fits,
	// an ordinary 20 s new card still does.
	var many []store.StudyCandidate
	for i := int64(1); i <= 14; i++ {
		many = append(many, review(i, i%2, time.Duration(100+i)*time.Hour, time.Hour))
	}
	got = ids(planSession(many, []store.StudyCandidate{fresh(100, 5, true), fresh(101, 6, false)}, now, 300, 10, nil))
	in := map[int64]bool{}
	for _, id := range got {
		in[id] = true
	}
	if len(got) != 15 || in[100] || !in[101] {
		t.Fatalf("14 reviews (280 s) and one more: %v; the exact line cannot fit, the ordinary card can", got)
	}

	// The daily limit: with none left, only a card already presented today
	// may come.
	got = ids(planSession(nil, []store.StudyCandidate{fresh(7, 1, false), fresh(8, 2, false)}, now, 300, 0, map[int64]int64{8: 1}))
	if want := []int64{8}; !eq(got, want) {
		t.Fatalf("limit spent: %v, want %v", got, want)
	}
}
