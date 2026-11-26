package web_test

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"scry/internal/app"
	"scry/internal/store"
)

// [S111-1] Punch in and study: one interleaved session across every deck,
// proved through the server as a browser drives it.

// deckOf is the deck (top-level idea) an item belongs to.
func deckOf(t *testing.T, lib *store.UserStore, it store.Item) int64 {
	t.Helper()
	idea, err := lib.Idea(context.Background(), it.IdeaID)
	if err != nil {
		t.Fatal(err)
	}
	if idea.ParentID != 0 {
		return idea.ParentID
	}
	return idea.ID
}

// answerWords are the words of an answer a tag must never hold.
func answerWords(s string) []string {
	var out []string
	for _, w := range regexp.MustCompile(`[\p{L}\p{N}]+`).FindAllString(strings.ToLower(s), -1) {
		if len([]rune(w)) >= 3 {
			out = append(out, w)
		}
	}
	return out
}

// Proved: with due reviews of known FSRS urgency and new cards spread
// unevenly across at least three demo decks, both /study and Study in the
// navigation open directly on a card; the first card is the most urgent
// due review; no two consecutive cards use the same deck while another
// deck has an eligible card; and the waiting cards' deck tags are safe.
func TestS111_1_Web_DemoDecks_StudyOpensOnTheMostUrgentReviewAndInterleaves(t *testing.T) {
	w := newWorld(t)
	if err := w.app.SeedDemo(ctx); err != nil {
		t.Fatal(err)
	}
	demo := w.user(app.DemoEmail)
	lib := w.lib(app.DemoEmail)
	now := w.now()

	// The demo's decks: due reviews and new cards, unevenly spread.
	reviews, fresh, err := lib.StudyCandidates(ctx, app.EndOfDay(demo, now), 0, app.ReviewCandidates, 200)
	if err != nil {
		t.Fatal(err)
	}
	perDeckReviews, perDeckNew := map[int64]int{}, map[int64]int{}
	for _, c := range reviews {
		perDeckReviews[c.Deck]++
	}
	for _, c := range fresh {
		perDeckNew[c.Deck]++
	}
	if len(perDeckReviews) < 3 || len(perDeckNew) < 3 {
		t.Fatalf("demo decks: reviews %v, new %v; want at least three decks of each", perDeckReviews, perDeckNew)
	}
	counts := map[int]bool{}
	for _, n := range perDeckReviews {
		counts[n] = true
	}
	if len(counts) < 2 {
		t.Fatalf("demo due reviews are spread evenly: %v", perDeckReviews)
	}
	// The most urgent due review: lowest retrievability, then earliest due.
	sort.SliceStable(reviews, func(i, j int) bool {
		ri, rj := reviews[i].Sched.RetrievabilityAt(now), reviews[j].Sched.RetrievabilityAt(now)
		if ri != rj {
			return ri < rj
		}
		if !reviews[i].Sched.Due.Equal(reviews[j].Sched.Due) {
			return reviews[i].Sched.Due.Before(reviews[j].Sched.Due)
		}
		return reviews[i].ID < reviews[j].ID
	})

	b := w.browser()
	b.get("/signin")
	_, body := b.post("/signin", "email", app.DemoEmail, "password", app.DemoPassword)
	if b.path != "/study" {
		t.Fatalf("sign-in ended at %s", b.path)
	}
	first, ok := b.current(body)
	if !ok {
		t.Fatal("/study did not open directly on a card")
	}
	if first.ID != reviews[0].ID {
		t.Fatalf("first card %q, want the most urgent due review %q", first.Prompt, reviews[0].Prompt)
	}
	hasNot(t, body, "data-focused", "Study all decks", "Pick a deck")
	// Study in the navigation (and g s) is /study: the same card, at once.
	has(t, body, `<a href="/study" aria-current="page"`)
	navBody := b.mustGet("/study")
	if again, ok := b.current(navBody); !ok || again.ID != first.ID {
		t.Fatal("Study in the navigation did not open on the waiting card")
	}
	// g s goes to the same place.
	if _, js := b.get("/static/app.js"); !strings.Contains(js, `s: "/study"`) {
		t.Fatal("g s does not open /study")
	}

	// Walk the session: decks alternate while another has a card left, and
	// every waiting card's tag is its spoiler-safe place line.
	sess, _ := lib.ActiveSession(ctx)
	var order []int64
	answers := map[int64]string{}
	for _, e := range sess.Entries {
		order = append(order, deckOf(t, lib, e.Item))
		answers[e.Item.ID] = e.Item.Answer
	}
	for i := 1; i < len(order); i++ {
		if order[i] != order[i-1] {
			continue
		}
		for _, later := range order[i:] {
			if later != order[i-1] {
				t.Fatalf("cards %d and %d share a deck while another deck had a card: %v", i, i+1, order)
			}
		}
	}
	if len(sess.Entries) < 2 {
		t.Fatalf("demo session of %d cards", len(sess.Entries))
	}
	seenDecks := map[int64]bool{}
	for _, d := range order {
		seenDecks[d] = true
	}
	if len(seenDecks) < 3 {
		t.Fatalf("the demo session touches %d decks, want at least 3", len(seenDecks))
	}
	body = navBody
	for i := 0; i < len(sess.Entries); i++ {
		it, ok := b.current(body)
		if !ok {
			break
		}
		has(t, body, "data-deck-tag")
		tag := strings.ToLower(studyHeader(t, body))
		for _, word := range answerWords(it.Answer) {
			if strings.Contains(" "+tag+" ", " "+word+" ") {
				t.Fatalf("the deck tag %q gives away %q of %q", tag, word, it.Answer)
			}
		}
		if idea, _ := lib.Idea(ctx, it.IdeaID); idea.ParentID != 0 && strings.Contains(tag, strings.ToLower(idea.Title)) {
			t.Fatalf("the deck tag %q names the idea's own title %q", tag, idea.Title)
		}
		body = b.answerNext(it, "x")
	}
}

// Proved: "Study this idea" in Library opens a session holding only that
// idea and its sub-ideas, marked "Focused · <topic>" with "Study all
// decks" back to every deck; /study goes on with it; with nothing
// eligible, the idea's page says so.
func TestS111_1_Web_StudyThisIdeaOpensAFocusedSessionOverOnlyThatIdea(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("focus@example.com")
	b.addSet("nato-alphabet")
	b.addSet("kitchen-numbers")
	lib := w.lib("focus@example.com")
	outline, _ := lib.Outline(ctx)
	var kitchen, nato store.Idea
	for _, o := range outline {
		switch {
		case o.ParentID == 0 && o.Origin == "starter:kitchen-numbers":
			kitchen = o
		case o.ParentID == 0 && o.Origin == "starter:nato-alphabet":
			nato = o
		}
	}
	if kitchen.ID == 0 || nato.ID == 0 {
		t.Fatalf("outline = %+v", outline)
	}
	// Every deck: no focused line.
	body := b.mustGet("/study")
	if _, ok := b.current(body); !ok {
		t.Fatal("no card")
	}
	hasNot(t, body, "data-focused")

	page := b.mustGet("/library/ideas/" + strconv.FormatInt(kitchen.ID, 10))
	has(t, page, `href="/study/idea/`+strconv.FormatInt(kitchen.ID, 10)+`"`, "Study this idea", "data-study-idea")
	body = b.mustGet("/study/idea/" + strconv.FormatInt(kitchen.ID, 10))
	if b.path != "/study" {
		t.Fatalf("Study this idea ended at %s", b.path)
	}
	has(t, body, "data-focused", "Focused · ", `href="/study?all=1"`, "Study all decks")
	sess, _ := lib.ActiveSession(ctx)
	if sess.Scope != kitchen.ID || len(sess.Entries) == 0 {
		t.Fatalf("focused session: scope %d, %d cards", sess.Scope, len(sess.Entries))
	}
	for _, e := range sess.Entries {
		if deckOf(t, lib, e.Item) != kitchen.ID {
			t.Fatalf("a focused session over %s holds %q", kitchen.Title, e.Item.Prompt)
		}
	}
	// Answering goes back to /study, which keeps the focused session.
	it, _ := b.current(body)
	body = b.answerNext(it, "x")
	has(t, body, "data-focused")
	// Study all decks: back to every deck, with no focused line.
	body = b.mustGet("/study?all=1")
	hasNot(t, body, "data-focused")
	if s, _ := lib.ActiveSession(ctx); s == nil || s.Scope != 0 {
		t.Fatal("Study all decks did not open a session across every deck")
	}
	// An idea with nothing eligible: its page says so.
	empty, _ := lib.CreateIdea(ctx, 0, "Nothing here yet", "", "", w.now())
	page = b.mustGet("/study/idea/" + strconv.FormatInt(empty, 10))
	if !strings.HasPrefix(b.path, "/library/ideas/") {
		t.Fatalf("an empty Study this idea ended at %s", b.path)
	}
	has(t, page, "Nothing in this idea is due today.")
	// Another learner's idea is not found.
	other := w.browser()
	other.signUp("other@example.com")
	if status, _ := other.get("/study/idea/" + strconv.FormatInt(kitchen.ID, 10)); status != http.StatusNotFound {
		t.Fatalf("another learner's Study this idea = %d, want 404", status)
	}
}

// Proved: waiting cards show only safe tags. In an interleaved session the
// card beside the waiting one comes from another deck; a topic holding a
// word of its answer falls back to the neutral set number. The review
// names the real place.
func TestS111_1_Web_WaitingCardsShowOnlySafeDeckTags(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("tags@example.com")
	lib := w.lib("tags@example.com")
	now := w.now()
	trip, _ := lib.CreateIdea(ctx, 0, "Paris weekend", "", "", now)
	museums, _ := lib.CreateIdea(ctx, trip, "Museums", "", "", now)
	capitals, _ := lib.CreateIdea(ctx, 0, "Capitals", "", "", now)
	lib.AddItem(ctx, store.NewItem{IdeaID: museums, Prompt: "Which museum holds the Mona Lisa?", Answer: "The Louvre"}, now)
	w.advance(time.Second)
	lib.AddItem(ctx, store.NewItem{IdeaID: capitals, Prompt: "What is the capital of France?", Answer: "Paris"}, w.now())
	body := b.mustGet("/study")
	it, ok := b.current(body)
	if !ok || it.Answer != "The Louvre" {
		t.Fatalf("first card = %+v", it)
	}
	// "Paris weekend · Set 1" would hold the next card's answer, "Paris".
	if got := studyHeader(t, body); got != "Set 1" {
		t.Fatalf("deck tag = %q, want the neutral %q", got, "Set 1")
	}
	review := b.answer(it, "Louvre")
	if got := studyHeader(t, review); got != "Paris weekend · Museums" {
		t.Fatalf("review place = %q", got)
	}
	body = b.next()
	if got := studyHeader(t, body); got != "Capitals" {
		t.Fatalf("second deck tag = %q, want %q", got, "Capitals")
	}
}

// Proved: with more than 10 new cards ready, reopening Study on the same
// learner day allows no more than 10 first presentations, and Study says
// so in words rather than starting another session.
func TestS111_1_Web_TenNewCardsADay_ReopeningStudySaysSo(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("ten@example.com")
	b.addSet("nato-alphabet") // 26 new cards
	seen := map[int64]bool{}
	for round := 0; round < 3; round++ {
		body := b.mustGet("/study")
		for i := 0; i < 20; i++ {
			it, ok := b.current(body)
			if !ok {
				break
			}
			seen[it.ID] = true
			body = b.answerNext(it, "x")
		}
	}
	if len(seen) != app.DailyNewCards {
		t.Fatalf("%d new cards presented in one day, want %d", len(seen), app.DailyNewCards)
	}
	has(t, b.mustGet("/study"), "That's today's 10 new cards. More come tomorrow.")
	w.advance(24 * time.Hour)
	if _, ok := b.current(b.mustGet("/study")); !ok {
		t.Fatal("no card the next day")
	}
}
