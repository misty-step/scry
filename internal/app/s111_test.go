package app_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"scry/internal/app"
	"scry/internal/srs"
	"scry/internal/store"
)

// [S111-1] Punch in and study: one interleaved session across every deck.
// These tests build decks with due reviews of known FSRS urgency and new
// cards, start sessions through App.Study / App.StudyIdea (the code /study
// and "Study this idea" run) and check what the session holds and in what
// order.

type deckWorld struct {
	*studyWorld
	t     *testing.T
	deck  map[int64]string // item -> deck name
	n     int
	ideas map[string]int64
}

func newDeckWorld(t *testing.T) *deckWorld {
	t.Helper()
	w := newStudyWorld(t, filepath.Join(t.TempDir(), "s111.db"))
	t.Cleanup(func() { w.st.Close() })
	return &deckWorld{studyWorld: w, t: t, deck: map[int64]string{}, ideas: map[string]int64{}}
}

// idea is a top-level idea (a deck), or with parent set an idea inside one.
func (w *deckWorld) idea(name, parent string) int64 {
	w.t.Helper()
	if id, ok := w.ideas[name]; ok {
		return id
	}
	pid := int64(0)
	if parent != "" {
		pid = w.idea(parent, "")
	}
	id, err := w.us.CreateIdea(w.ctx, pid, name, "", "", w.now())
	if err != nil {
		w.t.Fatal(err)
	}
	w.ideas[name] = id
	return id
}

// fresh adds a new card (never reviewed) to an idea; deck names its deck.
func (w *deckWorld) fresh(idea, deck string, exact bool) int64 {
	w.t.Helper()
	w.n++
	id, err := w.us.AddItem(w.ctx, store.NewItem{IdeaID: w.ideas[idea], Prompt: fmt.Sprintf("q%d", w.n),
		Answer: fmt.Sprintf("a%d", w.n), Exact: exact}, w.now())
	if err != nil {
		w.t.Fatal(err)
	}
	w.deck[id] = deck
	// Each new card a second younger than the one before: oldest first.
	w.advance(time.Second)
	return id
}

// review adds a card already reviewed, last seen daysAgo with the given
// stability, due dueAgo before now. Its retrievability now is known.
func (w *deckWorld) review(idea, deck string, stability, daysAgo float64, dueAgo time.Duration) int64 {
	w.t.Helper()
	id := w.fresh(idea, deck, false)
	now := w.now()
	last := now.Add(-time.Duration(daysAgo * 24 * float64(time.Hour)))
	if _, err := w.st.DB().Exec(`UPDATE items SET reps = 1, stability = ?, difficulty = 5, last_review = ?, due_at = ? WHERE id = ?`,
		stability, last.Unix(), now.Add(-dueAgo).Unix(), id); err != nil {
		w.t.Fatal(err)
	}
	return id
}

func (w *deckWorld) exactLine(idea, deck, title string, line int) int64 {
	w.t.Helper()
	w.n++
	id, err := w.us.AddItem(w.ctx, store.NewItem{IdeaID: w.ideas[idea], Prompt: fmt.Sprintf("%s line %d", title, line),
		Answer: fmt.Sprintf("words of line %d", line), Exact: true, TextTitle: title, LineNo: line}, w.now())
	if err != nil {
		w.t.Fatal(err)
	}
	w.deck[id] = deck
	w.advance(time.Second)
	return id
}

// study starts (or goes on with) the interleaved session and returns its
// item ids in order.
func (w *deckWorld) study() []int64 {
	w.t.Helper()
	s, err := w.app.Study(w.ctx, w.user)
	if err != nil {
		w.t.Fatal(err)
	}
	return ids(s)
}

func ids(s *store.StudySession) []int64 {
	if s == nil {
		return nil
	}
	var out []int64
	for _, e := range s.Entries {
		out = append(out, e.Item.ID)
	}
	return out
}

// finish answers every card of the open session (Correct) and moves on.
func (w *deckWorld) finish() int {
	w.t.Helper()
	n := 0
	for {
		s, err := w.us.ActiveSession(w.ctx)
		if err != nil {
			w.t.Fatal(err)
		}
		if s == nil || s.Current() == nil {
			return n
		}
		if _, err := w.app.Answer(w.ctx, w.user, s.Current().Item.ID, "x", "typed"); err != nil {
			w.t.Fatal(err)
		}
		if err := w.app.Next(w.ctx, w.user, false); err != nil {
			w.t.Fatal(err)
		}
		n++
	}
}

// checkInterleaved fails if two consecutive cards share a deck while a
// card of another deck was still to come.
func (w *deckWorld) checkInterleaved(order []int64) {
	w.t.Helper()
	for i := 1; i < len(order); i++ {
		if w.deck[order[i]] != w.deck[order[i-1]] {
			continue
		}
		for _, later := range order[i:] {
			if w.deck[later] != w.deck[order[i-1]] {
				w.t.Fatalf("cards %d and %d are both from %s while %s still had a card: %v", i, i+1, w.deck[order[i]], w.deck[later], w.names(order))
			}
		}
	}
}

func (w *deckWorld) names(order []int64) []string {
	var out []string
	for _, id := range order {
		out = append(out, fmt.Sprintf("%d:%s", id, w.deck[id]))
	}
	return out
}

func indexOf(order []int64, id int64) int {
	for i, o := range order {
		if o == id {
			return i
		}
	}
	return -1
}

// [S111-1] A due review's urgency is its current FSRS retrievability.
func TestS111_1_RetrievabilityAtIsTheFSRSForgettingCurve(t *testing.T) {
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	s := srs.State{Reps: 1, Stability: 4, Difficulty: 5, Last: now.Add(-4 * 24 * time.Hour)}
	if got, want := s.RetrievabilityAt(now), srs.Retrievability(4, 4); got != want || got < 0.89 || got > 0.91 {
		t.Fatalf("R after one stability = %v, want %v (about 0.9)", got, want)
	}
	older := s
	older.Last = now.Add(-10 * 24 * time.Hour)
	if !(older.RetrievabilityAt(now) < s.RetrievabilityAt(now)) {
		t.Fatal("a longer gap must mean a lower retrievability")
	}
	if (srs.State{}).RetrievabilityAt(now) != 0 {
		t.Fatal("a card never reviewed has no retrievability")
	}
}

// [S111-1] Proved: the first card is the most urgent due review (lowest
// retrievability, then earliest due); no two consecutive cards share a
// deck while another deck has an eligible card; and the most urgent
// eligible review from another deck wins over a new card.
func TestS111_1_FirstCardIsMostUrgent_DecksAlternate_ReviewsBeatNewCards(t *testing.T) {
	w := newDeckWorld(t)
	w.idea("NATO", "")
	w.idea("NATO set 1", "NATO") // a sub-idea: its cards are in the NATO deck
	w.idea("Models", "")
	w.idea("Kitchen", "")
	// Uneven: NATO has four reviews, Models two, Kitchen one; new cards in
	// Kitchen and Models.
	n1 := w.review("NATO set 1", "NATO", 2, 6, time.Hour)    // R ~ 0.83
	n2 := w.review("NATO", "NATO", 2, 3, 2*time.Hour)        // R ~ 0.9
	n3 := w.review("NATO set 1", "NATO", 10, 2, 3*time.Hour) // R ~ 0.98
	n4 := w.review("NATO", "NATO", 1, 9, 30*time.Minute)     // R ~ 0.7: most urgent
	m1 := w.review("Models", "Models", 3, 3, 26*time.Hour)   // R ~ 0.93
	m2 := w.review("Models", "Models", 3, 3, 25*time.Hour)   // same R, due later
	k1 := w.review("Kitchen", "Kitchen", 20, 1, 4*time.Hour) // R ~ 0.995: least urgent
	newK := []int64{w.fresh("Kitchen", "Kitchen", false), w.fresh("Kitchen", "Kitchen", false)}
	newM := w.fresh("Models", "Models", false)

	order := w.study()
	if len(order) != 10 {
		t.Fatalf("session of %d cards, want all 10: %v", len(order), w.names(order))
	}
	if order[0] != n4 {
		t.Fatalf("first card %s, want the most urgent review %d: %v", w.deck[order[0]], n4, w.names(order))
	}
	w.checkInterleaved(order)
	// After NATO's n4, the most urgent review of another deck (m1: same R
	// as m2, due earlier) wins over any new card.
	if order[1] != m1 || indexOf(order, m1) > indexOf(order, m2) {
		t.Fatalf("second card should be Models' most urgent review: %v", w.names(order))
	}
	// Reviews of another deck always come before new cards of one.
	if indexOf(order, k1) > indexOf(order, newK[0]) {
		t.Fatalf("a new Kitchen card came before Kitchen's due review: %v", w.names(order))
	}
	// Reviews by urgency, alternating decks; then the new cards, oldest
	// first, still alternating (Kitchen, Models, Kitchen).
	want := []int64{n4, m1, n1, m2, n2, k1, n3, newK[0], newM, newK[1]}
	for i, id := range want {
		if order[i] != id {
			t.Fatalf("card %d = %s, want %d: %v", i+1, w.names(order)[i], id, w.names(order))
		}
	}
}

// [S111-1] Proved: a paced new card breaks a streak when only the previous
// deck has due reviews left, and a new card used that way never takes a
// review's place: every due review that fits is still in the session.
func TestS111_1_PacedNewCardBreaksAStreak_NeverTakingAReviewsPlace(t *testing.T) {
	w := newDeckWorld(t)
	w.idea("Geo", "")
	w.idea("Words", "")
	var reviews []int64
	for i := 0; i < 4; i++ {
		reviews = append(reviews, w.review("Geo", "Geo", 2, float64(5+i), time.Hour))
	}
	news := []int64{w.fresh("Words", "Words", false), w.fresh("Words", "Words", false)}
	order := w.study()
	if len(order) != 6 {
		t.Fatalf("session = %v", w.names(order))
	}
	// Geo, Words(new), Geo, Words(new), Geo, Geo: the new cards break the
	// Geo streak; the last Geo reviews repeat only once no other deck has a
	// card left, reviews by urgency.
	wantDecks := []string{"Geo", "Words", "Geo", "Words", "Geo", "Geo"}
	for i, d := range wantDecks {
		if w.deck[order[i]] != d {
			t.Fatalf("decks = %v, want %v", w.names(order), wantDecks)
		}
	}
	if order[0] != reviews[3] || order[1] != news[0] || order[3] != news[1] {
		t.Fatalf("order = %v", w.names(order))
	}
	for _, r := range reviews {
		if indexOf(order, r) < 0 {
			t.Fatalf("review %d lost its place to a new card", r)
		}
	}
}

// [S111-1] Proved: a backlog of 15 or more eligible due reviews, all
// ordinary cards, fills the session (15 cards, all due reviews) and leaves
// no room for new cards. The session considers at most 200 due review
// candidates.
func TestS111_1_BacklogOfFifteenReviewsFillsTheSession_NoNewCards(t *testing.T) {
	w := newDeckWorld(t)
	w.idea("A", "")
	w.idea("B", "")
	w.idea("C", "")
	reviews := map[int64]bool{}
	for i := 0; i < 18; i++ {
		d := []string{"A", "B", "C"}[i%3]
		reviews[w.review(d, d, 3, float64(2+i%7), time.Duration(i+1)*time.Minute)] = true
	}
	for i := 0; i < 6; i++ {
		w.fresh("C", "C", false)
	}
	order := w.study()
	if len(order) != 15 {
		t.Fatalf("session of %d cards, want 15", len(order))
	}
	for _, id := range order {
		if !reviews[id] {
			t.Fatalf("a new card %d took room from the due reviews", id)
		}
	}
	w.checkInterleaved(order)
	if presented, _ := w.us.PresentedOn(w.ctx, app.LearnerDay(w.user, w.now())); len(presented) != 0 {
		t.Fatalf("%d first presentations with no new card in the session", len(presented))
	}
}

// [S111-1] At most 200 due review candidates are considered: the 200
// earliest due, ranked by retrievability. A review due after them waits,
// however low its retrievability.
func TestS111_1_ConsidersAtMost200DueReviewCandidates(t *testing.T) {
	w := newDeckWorld(t)
	w.idea("A", "")
	for i := 0; i < app.ReviewCandidates; i++ {
		w.review("A", "A", 5, 2, time.Duration(300-i)*time.Hour)
	}
	late := w.review("A", "A", 0.5, 30, time.Minute) // lowest R, due last
	order := w.study()
	if len(order) != 15 || indexOf(order, late) >= 0 {
		t.Fatalf("the 201st candidate was considered: %d cards, at %d", len(order), indexOf(order, late))
	}
}

// [S111-1] Proved: with exactly 10 ordinary due reviews, at least 5
// eligible ordinary new cards and at least 5 first presentations left
// today, the session holds 10 due reviews and 5 new cards.
func TestS111_1_TenReviewsAndFiveNewCards(t *testing.T) {
	w := newDeckWorld(t)
	w.idea("A", "")
	w.idea("B", "")
	w.idea("C", "")
	reviews := map[int64]bool{}
	for i := 0; i < 10; i++ {
		d := []string{"A", "A", "B"}[i%3]
		reviews[w.review(d, d, 3, 4, time.Hour)] = true
	}
	var news []int64
	for i := 0; i < 8; i++ {
		d := []string{"C", "B"}[i%2]
		news = append(news, w.fresh(d, d, false))
	}
	order := w.study()
	nr, nn := 0, 0
	for _, id := range order {
		if reviews[id] {
			nr++
		} else {
			nn++
		}
	}
	if nr != 10 || nn != 5 {
		t.Fatalf("%d reviews and %d new cards, want 10 and 5: %v", nr, nn, w.names(order))
	}
	// The new cards are the oldest five.
	for _, id := range news[:5] {
		if indexOf(order, id) < 0 {
			t.Fatalf("new card %d (among the oldest five) is missing: %v", id, w.names(order))
		}
	}
	w.checkInterleaved(order)
}

// [S111-1] An exact-text line counts 30 seconds: a session of exact lines
// holds 10 (fewer than 15), and the eligible lines of an exact text keep
// their order even when a later line is more urgent.
func TestS111_1_ExactLinesCountThirtySecondsAndKeepTheirOrder(t *testing.T) {
	w := newDeckWorld(t)
	w.idea("Creed", "")
	w.idea("Poem", "")
	var creed []int64
	for line := 1; line <= 12; line++ {
		creed = append(creed, w.exactLine("Creed", "Creed", "Creed", line))
	}
	order := w.study()
	if len(order) != 10 {
		t.Fatalf("a session of exact lines holds %d, want 10 (300 s / 30 s)", len(order))
	}
	for i := range order {
		if order[i] != creed[i] {
			t.Fatalf("exact lines out of order: %v", order)
		}
	}
	// Reviews: line 3 is more urgent than line 1 and 2, yet they keep
	// their order; the other deck still interleaves.
	w2 := newDeckWorld(t)
	w2.idea("Poem", "")
	w2.idea("Other", "")
	l1 := w2.exactLine("Poem", "Poem", "Poem", 1)
	l2 := w2.exactLine("Poem", "Poem", "Poem", 2)
	l3 := w2.exactLine("Poem", "Poem", "Poem", 3)
	o1 := w2.review("Other", "Other", 3, 3, time.Hour)
	now := w2.now()
	for i, id := range []int64{l1, l2, l3} {
		days := []float64{2, 3, 20}[i] // line 3 the most urgent
		if _, err := w2.st.DB().Exec(`UPDATE items SET reps = 1, stability = 3, difficulty = 5, last_review = ?, due_at = ? WHERE id = ?`,
			now.Add(-time.Duration(days*24)*time.Hour).Unix(), now.Add(-time.Hour).Unix(), id); err != nil {
			t.Fatal(err)
		}
	}
	got := w2.study()
	if indexOf(got, l1) > indexOf(got, l2) || indexOf(got, l2) > indexOf(got, l3) {
		t.Fatalf("lines of one exact text out of order: %v", w2.names(got))
	}
	if got[0] != l1 || got[1] != o1 {
		t.Fatalf("the most urgent slot goes to the text's first line, then another deck: %v", w2.names(got))
	}
}

// [S111-1] Proved: with more than 10 new cards ready, reopening Study or
// switching to focused study on the same learner day still allows no more
// than 10 first presentations in all; the next day brings more.
func TestS111_1_AtMostTenFirstPresentationsADay_AcrossReopenAndFocusedStudy(t *testing.T) {
	w := newDeckWorld(t)
	w.idea("A", "")
	w.idea("B", "")
	for i := 0; i < 30; i++ {
		d := []string{"A", "B"}[i%2]
		w.fresh(d, d, false)
	}
	shown := map[int64]bool{}
	see := func(order []int64) {
		for _, id := range order {
			shown[id] = true
		}
	}
	// The first session: 10 new cards fit the budget and the day's limit.
	first := w.study()
	if len(first) != app.DailyNewCards {
		t.Fatalf("first session %d cards, want %d", len(first), app.DailyNewCards)
	}
	// Reopening Study goes on with the same session: no new presentations.
	if again := w.study(); fmt.Sprint(again) != fmt.Sprint(first) {
		t.Fatalf("reopening Study changed the session: %v then %v", first, again)
	}
	// Answer three, then switch to a focused session on B.
	for i := 0; i < 3; i++ {
		s, _ := w.us.ActiveSession(w.ctx)
		see([]int64{s.Current().Item.ID})
		w.app.Answer(w.ctx, w.user, s.Current().Item.ID, "x", "typed")
		w.app.Next(w.ctx, w.user, false)
	}
	s, _ := w.us.ActiveSession(w.ctx)
	see([]int64{s.Current().Item.ID}) // the fourth card is on screen
	focused, err := w.app.StudyIdea(w.ctx, w.user, w.ideas["B"])
	if err != nil || focused == nil || focused.Scope != w.ideas["B"] {
		t.Fatalf("focused session = %+v, %v", focused, err)
	}
	for _, e := range focused.Entries {
		if w.deck[e.Item.ID] != "B" {
			t.Fatalf("a focused session on B holds a card of %s", w.deck[e.Item.ID])
		}
	}
	see(ids(focused))
	// The first session's new cards still waiting behind the one on screen
	// were never shown: the switch released them (unless the focused
	// session holds them now).
	after, _ := w.us.PresentedOn(w.ctx, app.LearnerDay(w.user, w.now()))
	inFocus := map[int64]bool{}
	for _, id := range ids(focused) {
		inFocus[id] = true
	}
	for _, id := range first[4:] {
		if after[id] != 0 && !inFocus[id] {
			t.Fatalf("card %d was never shown but still counts as presented", id)
		}
	}
	for _, id := range first[:4] {
		if after[id] == 0 {
			t.Fatalf("card %d was shown but no longer counts", id)
		}
	}
	w.finish()
	// Back to every deck, and again, all on the same day.
	for i := 0; i < 3; i++ {
		see(w.study())
		w.finish()
	}
	if len(shown) > app.DailyNewCards {
		t.Fatalf("%d first presentations on one day, want at most %d", len(shown), app.DailyNewCards)
	}
	presented, _ := w.us.PresentedOn(w.ctx, app.LearnerDay(w.user, w.now()))
	if len(presented) > app.DailyNewCards {
		t.Fatalf("%d first presentations recorded, want at most %d", len(presented), app.DailyNewCards)
	}
	// The cards released by the switch were never shown, so the day's limit
	// was not spent on them: all 10 were presented.
	if len(presented) != app.DailyNewCards {
		t.Fatalf("%d first presentations, want the full %d (unseen cards released on the switch)", len(presented), app.DailyNewCards)
	}
	if s, _ := w.app.Study(w.ctx, w.user); s != nil {
		t.Fatalf("with today's new cards spent and nothing due, Study started a session of %d", len(s.Entries))
	}
	// Tomorrow, in the learner's time zone, there are new cards again.
	w.advance(24 * time.Hour)
	if s, _ := w.app.Study(w.ctx, w.user); s == nil {
		t.Fatal("no session the next day")
	}
}

// [S111-1] The day of the limit is the learner's own, in their stored
// time zone: just before and just after their midnight are two days.
func TestS111_1_DailyLimitFollowsTheLearnersTimeZone(t *testing.T) {
	w := newDeckWorld(t)
	if err := w.us.SetTZ(w.ctx, "America/Chicago"); err != nil {
		t.Fatal(err)
	}
	u, err := w.st.UserByID(w.ctx, w.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	w.user = u
	w.idea("A", "")
	// 09:00 UTC is 03:00 in Chicago; move to 23:30 Chicago (05:30 UTC next day).
	chicago, _ := time.LoadLocation("America/Chicago")
	n := w.now().In(chicago)
	w.advance(time.Date(n.Year(), n.Month(), n.Day(), 23, 30, 0, 0, chicago).Sub(w.now()))
	for i := 0; i < 25; i++ {
		w.fresh("A", "A", false)
	}
	day1 := app.LearnerDay(w.user, w.now())
	if got := len(w.study()); got != 10 {
		t.Fatalf("first session %d", got)
	}
	w.finish()
	if s, _ := w.app.Study(w.ctx, w.user); s != nil {
		t.Fatal("more than 10 new cards before the learner's midnight")
	}
	w.advance(time.Hour) // 00:30 in Chicago, still 06:30 UTC on the same UTC day
	if day2 := app.LearnerDay(w.user, w.now()); day2 == day1 {
		t.Fatalf("the learner's day did not turn at their midnight: %s", day2)
	}
	if s, _ := w.app.Study(w.ctx, w.user); s == nil || len(s.Entries) != 10 {
		t.Fatal("no new cards after the learner's midnight")
	}
}

// [S111-1] A session starts only when at least one card is eligible:
// cards held by bridges, snoozed or removed are not; an empty library is
// no session.
func TestS111_1_StartsOnlyWithAnEligibleCard(t *testing.T) {
	w := newDeckWorld(t)
	if s, err := w.app.Study(w.ctx, w.user); s != nil || err != nil {
		t.Fatalf("empty library: %+v, %v", s, err)
	}
	w.idea("A", "")
	id := w.fresh("A", "A", false)
	if err := w.app.SnoozeCard(w.ctx, w.user, id); err != nil {
		t.Fatal(err)
	}
	if s, _ := w.app.Study(w.ctx, w.user); s != nil {
		t.Fatal("a snoozed card started a session")
	}
	w.advance(25 * time.Hour)
	if s, _ := w.app.Study(w.ctx, w.user); s == nil || len(s.Entries) != 1 {
		t.Fatal("the snoozed card did not come back the next day")
	}
}

// [S111-1] "Study this idea" holds only that idea and its sub-ideas, under
// the same rules (urgency first, budget, daily limit); an idea with nothing
// eligible starts nothing and leaves the open session alone; another
// learner's idea is not found.
func TestS111_1_StudyThisIdeaHoldsOnlyThatIdeaAndItsSubIdeas(t *testing.T) {
	w := newDeckWorld(t)
	w.idea("NATO", "")
	w.idea("NATO A", "NATO")
	w.idea("NATO B", "NATO")
	w.idea("Other", "")
	w.idea("Empty", "")
	inNATO := map[int64]bool{}
	for i := 0; i < 3; i++ {
		inNATO[w.review("NATO A", "NATO", 2, float64(3+i), time.Hour)] = true
		inNATO[w.fresh("NATO B", "NATO", false)] = true
	}
	urgent := w.review("NATO", "NATO", 1, 20, time.Hour)
	inNATO[urgent] = true
	for i := 0; i < 5; i++ {
		w.review("Other", "Other", 1, 30, time.Hour) // more urgent, but not in scope
	}
	all := w.study()
	s, err := w.app.StudyIdea(w.ctx, w.user, w.ideas["NATO"])
	if err != nil || s == nil {
		t.Fatal(err)
	}
	if len(s.Entries) != len(inNATO) {
		t.Fatalf("focused session of %d cards, want %d", len(s.Entries), len(inNATO))
	}
	for _, e := range s.Entries {
		if !inNATO[e.Item.ID] {
			t.Fatalf("card %d of %s in a session over NATO", e.Item.ID, w.deck[e.Item.ID])
		}
	}
	if s.Entries[0].Item.ID != urgent {
		t.Fatal("the focused session does not open on its most urgent review")
	}
	// Reopening it goes on with it; Study (the redirect after an answer)
	// goes on with it too.
	if again, _ := w.app.StudyIdea(w.ctx, w.user, w.ideas["NATO"]); again.ID != s.ID {
		t.Fatal("Study this idea twice started a second session")
	}
	if again, _ := w.app.Study(w.ctx, w.user); again.ID != s.ID {
		t.Fatal("/study left the focused session")
	}
	// A sub-idea alone.
	sub, _ := w.app.StudyIdea(w.ctx, w.user, w.ideas["NATO B"])
	for _, e := range sub.Entries {
		if e.Item.IdeaID != w.ideas["NATO B"] {
			t.Fatalf("a session over NATO B holds a card of idea %d", e.Item.IdeaID)
		}
	}
	// Nothing eligible: no session, and the open one stays.
	if empty, err := w.app.StudyIdea(w.ctx, w.user, w.ideas["Empty"]); empty != nil || err != nil {
		t.Fatalf("an empty idea started %+v, %v", empty, err)
	}
	if open, _ := w.us.ActiveSession(w.ctx); open == nil || open.ID != sub.ID {
		t.Fatal("an empty Study this idea ended the open session")
	}
	// Study all decks leaves the focused session for every deck.
	back, _ := w.app.StudyAll(w.ctx, w.user)
	if back == nil || back.Scope != 0 || len(back.Entries) != len(all) {
		t.Fatalf("Study all decks = %+v", back)
	}
	if _, err := w.app.StudyIdea(w.ctx, w.user, 99999); err == nil {
		t.Fatal("a missing idea is not found")
	}
}
