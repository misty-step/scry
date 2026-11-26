package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"scry/internal/ai"
	"scry/internal/app"
	"scry/internal/srs"
	"scry/internal/store"
)

// Slice 75 (the Strugglometer): the detection rule at its boundaries. A
// card is struggling with 3 or more FSRS lapses, or when its latest two
// graded reviews are both Incorrect; nothing else counts.
func TestSlice75_Detection_Boundaries(t *testing.T) {
	M, C, R := srs.Missed, srs.Close, srs.Right
	for _, tc := range []struct {
		name   string
		lapses int
		recent []srs.Verdict // newest first
		want   bool
	}{
		{"no history", 0, nil, false},
		{"two lapses, nothing else", 2, []srs.Verdict{R, R}, false},
		{"three lapses", 3, []srs.Verdict{R, R}, true},
		{"four lapses", 4, nil, true},
		{"one Incorrect", 0, []srs.Verdict{M}, false},
		{"latest two Incorrect", 0, []srs.Verdict{M, M}, true},
		{"latest two Incorrect after a Correct", 1, []srs.Verdict{M, M, R}, true},
		{"Incorrect then Almost (newest)", 0, []srs.Verdict{C, M}, false},
		{"Almost then Incorrect (newest)", 0, []srs.Verdict{M, C}, false},
		{"Correct between two Incorrect", 0, []srs.Verdict{M, R, M}, false},
		{"two lapses and one Incorrect", 2, []srs.Verdict{M, R}, false},
	} {
		if got := app.Struggling(tc.lapses, tc.recent); got != tc.want {
			t.Errorf("%s: Struggling(%d, %v) = %v, want %v", tc.name, tc.lapses, tc.recent, got, tc.want)
		}
	}
}

// Slice 75: practice attempts ("Again" on an exact text) never count
// toward the latest two reviews, and the history is read newest first.
func TestSlice75_Detection_IgnoresPracticeAttempts(t *testing.T) {
	at := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	it := store.Item{Sched: srs.State{Lapses: 0}}
	history := []store.Review{ // newest first
		{Verdict: srs.Missed, At: at.Add(3 * time.Minute)},
		{Verdict: srs.Right, Practice: true, At: at.Add(2 * time.Minute)},
		{Verdict: srs.Missed, At: at.Add(time.Minute)},
	}
	if s := app.StruggleOf(it, history, store.BridgeState{}); !s.Struggling || !s.Offer() {
		t.Fatalf("practice between two misses broke the rule: %+v", s)
	}
	history[1].Practice = false
	if s := app.StruggleOf(it, history, store.BridgeState{}); s.Struggling {
		t.Fatalf("a graded Correct between two misses: %+v", s)
	}
	// Offered only when it can take bridges.
	ex := store.Item{Exact: true, Sched: srs.State{Lapses: 3, Reps: 3}}
	if s := app.StruggleOf(ex, nil, store.BridgeState{}); !s.Struggling || s.Offer() {
		t.Fatalf("an exact text is never offered bridges: %+v", s)
	}
	if s := app.StruggleOf(store.Item{Sched: srs.State{Lapses: 3, Reps: 3}}, nil, store.BridgeState{HeldBy: 2}); s.Offer() {
		t.Fatalf("a held card is not offered more bridges: %+v", s)
	}
	if s := app.StruggleOf(store.Item{Sched: srs.State{Lapses: 3, Reps: 3}}, nil, store.BridgeState{BridgeFor: 9}); s.Offer() {
		t.Fatalf("a bridge is not offered bridges: %+v", s)
	}
}

// struggleWorld is a study world with a test-mode bridger and two cards.
func struggleWorld(t *testing.T) (*studyWorld, *ai.FakeBridger, []int64) {
	t.Helper()
	w := newStudyWorld(t, filepath.Join(t.TempDir(), "s75.db"))
	t.Cleanup(func() { w.st.Close() })
	fb := &ai.FakeBridger{}
	w.app.Bridger = fb
	return w, fb, w.addItems(t, false, false)
}

// answerOnce studies the item (alone in a session) with the given verdict.
func (w *studyWorld) answerOnce(t *testing.T, item int64, v srs.Verdict, text string) store.Review {
	t.Helper()
	if s, _ := w.us.ActiveSession(w.ctx); s != nil {
		w.us.EndSession(w.ctx, s.ID, w.now())
	}
	if _, err := w.us.StartSession(w.ctx, []int64{item}, w.now()); err != nil {
		t.Fatal(err)
	}
	w.model.set(v)
	r, err := w.app.Answer(w.ctx, w.user, item, text, "typed")
	if err != nil {
		t.Fatalf("answer %d: %v", item, err)
	}
	if err := w.app.Next(w.ctx, w.user, false); err != nil {
		t.Fatal(err)
	}
	return r
}

// Slice 75: two Incorrect answers in a row (through the real answer path)
// make a card struggling; bridges are drafted from the card's question and
// answer and the recent misses, metered as drafting, with nothing written
// to the library until the learner accepts.
func TestSlice75_Generation_UsesTheParentAndRecentMisses(t *testing.T) {
	w, fb, ids := struggleWorld(t)
	p := ids[0]
	if _, err := w.app.DraftBridges(w.ctx, w.user, p); !errors.Is(err, app.ErrNotStruggling) {
		t.Fatalf("drafting for a card that is not struggling = %v", err)
	}
	if fb.Calls() != 0 {
		t.Fatal("a card not offered bridges must not reach the model")
	}
	w.answerOnce(t, p, srs.Missed, "first wrong try")
	if _, st, _ := w.app.StruggleFor(w.ctx, w.user, p); st.Struggling {
		t.Fatal("one Incorrect is not a struggle")
	}
	w.advance(25 * time.Hour)
	w.answerOnce(t, p, srs.Missed, "second wrong try")
	_, st, err := w.app.StruggleFor(w.ctx, w.user, p)
	if err != nil || !st.Struggling || !st.Offer() {
		t.Fatalf("after two Incorrect: %+v, %v", st, err)
	}
	items, _, _ := w.us.Counts(w.ctx, w.now())
	cards, err := w.app.DraftBridges(w.ctx, w.user, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) < ai.MinBridges || len(cards) > ai.MaxBridges {
		t.Fatalf("%d cards", len(cards))
	}
	in := fb.Last()
	if in.Question != "question 0" || in.Answer != "answer number 0" ||
		len(in.Misses) != 2 || in.Misses[0] != "second wrong try" || in.Misses[1] != "first wrong try" {
		t.Fatalf("bridger input = %+v", in)
	}
	for _, c := range cards {
		if strings.Contains(c.Question+c.Answer, "answer number 0") {
			t.Fatalf("a fixture bridge gives the parent's answer away: %+v", c)
		}
	}
	if after, _, _ := w.us.Counts(w.ctx, w.now()); after != items {
		t.Fatal("drafting bridges wrote to the library")
	}
	var drafts int
	w.st.DB().QueryRow(`SELECT COUNT(*) FROM usage WHERE user_id = ? AND kind = 'draft'`, w.user.ID).Scan(&drafts)
	if drafts != 1 {
		t.Fatalf("%d draft usage rows, want 1", drafts)
	}
	if held, _ := w.us.ReservedMicros(w.ctx); held != 0 {
		t.Fatalf("%d micros still reserved", held)
	}
	// A failing model is said plainly and adds nothing.
	fb.Mode = "fail"
	if _, err := w.app.DraftBridges(w.ctx, w.user, p); !errors.Is(err, app.ErrBridgesUnavailable) {
		t.Fatalf("failing bridger = %v", err)
	}
	// Another learner's card is not found.
	other, _ := w.st.CreateUser(w.ctx, "other@example.com", "other", "UTC", w.now())
	fb.Mode = ""
	if _, err := w.app.DraftBridges(w.ctx, other, p); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign draft = %v", err)
	}
}

// Slice 75: accepting validates every input, is atomic and applies a
// preview at most once.
func TestSlice75_Accept_ValidatesAndIsIdempotent(t *testing.T) {
	w, _, ids := struggleWorld(t)
	p := ids[0]
	key := app.BridgeKey()
	good := []store.BridgeCard{{Prompt: "b1 q", Answer: "b1 a"}, {Prompt: "b2 q", Answer: "b2 a"}}
	// Not struggling yet: refused.
	if _, err := w.app.AcceptBridges(w.ctx, w.user, p, key, good); !errors.Is(err, app.ErrNotStruggling) {
		t.Fatalf("accept before struggling = %v", err)
	}
	w.answerOnce(t, p, srs.Missed, "x")
	w.advance(25 * time.Hour)
	w.answerOnce(t, p, srs.Missed, "y")
	var fe *app.FieldError
	for name, tc := range map[string]struct {
		key   string
		cards []store.BridgeCard
	}{
		"bad key":       {"short", good},
		"none":          {key, nil},
		"all emptied":   {key, []store.BridgeCard{{}, {}}},
		"four":          {key, append(append([]store.BridgeCard{}, good...), good...)},
		"no question":   {key, []store.BridgeCard{{Prompt: " ", Answer: "a"}}},
		"no answer":     {key, []store.BridgeCard{{Prompt: "q", Answer: "  "}}},
		"long question": {key, []store.BridgeCard{{Prompt: strings.Repeat("q", app.MaxPromptLen+1), Answer: "a"}}},
		"long answer":   {key, []store.BridgeCard{{Prompt: "q", Answer: strings.Repeat("a", app.MaxAnswerLen+1)}}},
	} {
		if _, err := w.app.AcceptBridges(w.ctx, w.user, p, tc.key, tc.cards); !errors.As(err, &fe) {
			t.Errorf("%s: err = %v, want a field error", name, err)
		}
	}
	if st, _ := w.us.BridgeState(w.ctx, p); st.HeldBy != 0 {
		t.Fatal("a refused accept held the card")
	}
	// An emptied row is a removed row; the rest are accepted.
	res, err := w.app.AcceptBridges(w.ctx, w.user, p, key, append([]store.BridgeCard{{}}, good...))
	if err != nil || res.Bridges != 2 || res.Duplicate {
		t.Fatalf("accept = %+v, %v", res, err)
	}
	again, err := w.app.AcceptBridges(w.ctx, w.user, p, key, good)
	if err != nil || !again.Duplicate || again.Bridges != 2 {
		t.Fatalf("retry = %+v, %v", again, err)
	}
	if _, err := w.app.AcceptBridges(w.ctx, w.user, p, app.BridgeKey(), good); !errors.Is(err, app.ErrNotStruggling) {
		t.Fatalf("a second preview for a held card = %v", err)
	}
	other, _ := w.st.CreateUser(w.ctx, "other@example.com", "other", "UTC", w.now())
	if _, err := w.app.AcceptBridges(w.ctx, other, p, app.BridgeKey(), good); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("foreign accept = %v", err)
	}
}

// Slice 75, end to end through Study: a held parent is not in any session;
// an Incorrect on a bridge keeps it held; Correct and Almost on the
// two bridges release it automatically, with its schedule untouched; a
// manual release brings a held card straight back.
func TestSlice75_Journey_HoldBridgesAndAutomaticReentry(t *testing.T) {
	w, _, ids := struggleWorld(t)
	p, other := ids[0], ids[1]
	w.answerOnce(t, other, srs.Right, "fine") // other is reviewed and not due for a while
	w.answerOnce(t, p, srs.Missed, "x")
	w.advance(25 * time.Hour)
	w.answerOnce(t, p, srs.Missed, "y")
	res, err := w.app.AcceptBridges(w.ctx, w.user, p, app.BridgeKey(),
		[]store.BridgeCard{{Prompt: "b1 q", Answer: "b1 a"}, {Prompt: "b2 q", Answer: "b2 a"}})
	if err != nil || res.Bridges != 2 {
		t.Fatal(res, err)
	}
	parentBefore, _ := w.us.Item(w.ctx, p)
	w.advance(48 * time.Hour) // the parent is now overdue
	sessionItems := func() []int64 {
		t.Helper()
		if s, _ := w.us.ActiveSession(w.ctx); s != nil {
			w.us.EndSession(w.ctx, s.ID, w.now())
		}
		s, err := w.app.Study(w.ctx, w.user)
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		if s != nil {
			for _, e := range s.Entries {
				out = append(out, e.Item.ID)
			}
		}
		return out
	}
	got := sessionItems()
	bridges, _ := w.us.Bridges(w.ctx, p)
	if len(bridges) != 2 || len(got) != 2 || got[0] == p || got[1] == p {
		t.Fatalf("session while held = %v (parent %d)", got, p)
	}
	b1, b2 := bridges[0].ID, bridges[1].ID
	w.answerOnce(t, b1, srs.Missed, "nope")
	if st, _ := w.us.BridgeState(w.ctx, p); st.HeldBy != 2 {
		t.Fatalf("Incorrect on a bridge: held by %d", st.HeldBy)
	}
	w.answerOnce(t, b2, srs.Right, "b2 a")
	if st, _ := w.us.BridgeState(w.ctx, p); st.HeldBy != 1 {
		t.Fatalf("Correct on a bridge: held by %d", st.HeldBy)
	}
	for _, id := range sessionItems() {
		if id == p {
			t.Fatal("one bridge left: the parent must not be studied")
		}
	}
	w.answerOnce(t, b1, srs.Close, "b1-ish")
	if st, _ := w.us.BridgeState(w.ctx, p); st.HeldBy != 0 {
		t.Fatalf("Almost on the last bridge: held by %d", st.HeldBy)
	}
	parentAfter, _ := w.us.Item(w.ctx, p)
	if parentAfter.Sched != parentBefore.Sched {
		t.Fatalf("automatic release changed the schedule: %+v -> %+v", parentBefore.Sched, parentAfter.Sched)
	}
	if got := sessionItems(); len(got) == 0 || got[0] != p {
		t.Fatalf("after automatic release the overdue parent should lead the session: %v", got)
	}

	// Manual release.
	if s, _ := w.us.ActiveSession(w.ctx); s != nil {
		w.us.EndSession(w.ctx, s.ID, w.now())
	}
	if _, err := w.app.AcceptBridges(w.ctx, w.user, p, app.BridgeKey(), []store.BridgeCard{{Prompt: "b3 q", Answer: "b3 a"}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range sessionItems() {
		if id == p {
			t.Fatal("held again: the parent must not be studied")
		}
	}
	if n, err := w.app.ReleaseHold(w.ctx, w.user, p); err != nil || n != 1 {
		t.Fatalf("release = %d, %v", n, err)
	}
	if got := sessionItems(); len(got) == 0 || got[0] != p {
		t.Fatalf("after manual release the parent should be due: %v", got)
	}
	if after, _ := w.us.Item(w.ctx, p); after.Sched != parentBefore.Sched {
		t.Fatal("manual release changed the schedule")
	}
}

// Slice 75: three lapses alone make a card struggling.
func TestSlice75_Detection_ThreeLapsesThroughTheStore(t *testing.T) {
	w, _, ids := struggleWorld(t)
	p := ids[0]
	at := w.now()
	sched := srs.State{Stability: 2, Difficulty: 6, Reps: 5, Lapses: 3, Last: at.Add(-24 * time.Hour), Due: at.Add(24 * time.Hour)}
	if _, err := w.us.RecordReview(w.ctx, store.Review{ItemID: p, At: sched.Last, Answer: "a", Verdict: srs.Right, Reason: "r"}, sched, false); err != nil {
		t.Fatal(err)
	}
	if _, st, _ := w.app.StruggleFor(w.ctx, w.user, p); !st.Struggling || !st.Offer() {
		t.Fatalf("three lapses: %+v", st)
	}
	sched.Lapses = 2
	w.us.RecordReview(w.ctx, store.Review{ItemID: p, At: sched.Last.Add(time.Minute), Answer: "a", Verdict: srs.Right, Reason: "r"}, sched, false)
	if _, st, _ := w.app.StruggleFor(w.ctx, w.user, p); st.Struggling {
		t.Fatalf("two lapses and a Correct: %+v", st)
	}
}

// Slice 75 (pass 2): through the real drafting adapter on a loopback
// provider, a spoiler among the drafted cards never reaches the preview:
// two safe cards plus one quoting the parent's answer is a two-card
// preview, and one safe card left is the plain "unavailable" failure.
func TestSlice75_Generation_RealAdapterDropsSpoilers(t *testing.T) {
	w, _, ids := struggleWorld(t)
	p := ids[0]
	reply := ""
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		json.NewEncoder(rw).Encode(map[string]any{"model": ai.DefaultOpenRouterDraftModel,
			"usage":   map[string]any{"prompt_tokens": 300, "completion_tokens": 90},
			"choices": []map[string]any{{"message": map[string]any{"content": reply}, "finish_reason": "stop"}}})
	}))
	t.Cleanup(srv.Close)
	c, err := ai.NewClaudeWithRouter("", "sk-or-v1-standin", "", "")
	if err != nil {
		t.Fatal(err)
	}
	c.BaseURL, c.HTTP = srv.URL, srv.Client()
	w.app.Bridger = c
	w.answerOnce(t, p, srs.Missed, "first wrong try")
	w.advance(25 * time.Hour)
	w.answerOnce(t, p, srs.Missed, "second wrong try")

	reply = `{"cards":[{"q":"What is the first safe bridge?","a":"Safe one"},{"q":"Is it Answer Number 0?","a":"Yes"},{"q":"What is the second safe bridge?","a":"Safe two"}]}`
	cards, err := w.app.DraftBridges(w.ctx, w.user, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 || cards[0].Answer != "Safe one" || cards[1].Answer != "Safe two" {
		t.Fatalf("cards %+v, want the two safe ones", cards)
	}
	reply = `{"cards":[{"q":"What is the first safe bridge?","a":"Safe one"},{"q":"question 0","a":"x"},{"q":"What is it?","a":"answer number 0."}]}`
	if cards, err := w.app.DraftBridges(w.ctx, w.user, p); !errors.Is(err, app.ErrBridgesUnavailable) || cards != nil {
		t.Fatalf("one safe card left = %+v, %v; want ErrBridgesUnavailable", cards, err)
	}
}

// Slice 75 (pass 3): a parent whose answer is only symbols ("!=") has no
// words to match, so the production OpenRouter adapter matches it
// literally: the bridge asking which languages use != never reaches the
// preview, the two safe bridges do, and safe bridges alone are all kept.
func TestSlice75_Generation_RealAdapterDropsSymbolOnlyAnswerLeak(t *testing.T) {
	w, _, ids := struggleWorld(t)
	p := ids[0]
	if err := w.us.UpdateItem(w.ctx, p, "In C, which operator tests that two values are not equal?", "!=", false); err != nil {
		t.Fatal(err)
	}
	reply := ""
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		asked = string(raw)
		json.NewEncoder(rw).Encode(map[string]any{"model": ai.DefaultOpenRouterDraftModel,
			"usage":   map[string]any{"prompt_tokens": 300, "completion_tokens": 90},
			"choices": []map[string]any{{"message": map[string]any{"content": reply}, "finish_reason": "stop"}}})
	}))
	t.Cleanup(srv.Close)
	c, err := ai.NewClaudeWithRouter("", "sk-or-v1-standin", "", "")
	if err != nil {
		t.Fatal(err)
	}
	c.BaseURL, c.HTTP = srv.URL, srv.Client()
	w.app.Bridger = c
	w.answerOnce(t, p, srs.Missed, "=/=")
	w.advance(25 * time.Hour)
	w.answerOnce(t, p, srs.Missed, "<>")

	reply = `{"cards":[{"q":"In C, what does the == operator test?","a":"That two values are equal"},{"q":"Which programming languages use != to mean not equal?","a":"C, Java, Python and many others"},{"q":"In C, what value does a false comparison produce?","a":"0"}]}`
	cards, err := w.app.DraftBridges(w.ctx, w.user, p)
	if err != nil {
		t.Fatal(err)
	}
	var req struct{ Messages []struct{ Content string } }
	json.Unmarshal([]byte(asked), &req)
	if len(req.Messages) != 2 || !strings.Contains(req.Messages[1].Content, "<parent_answer>!=</parent_answer>") {
		t.Fatalf("the parent's answer was not sent: %s", asked)
	}
	if len(cards) != 2 || cards[0].Answer != "That two values are equal" || cards[1].Answer != "0" {
		t.Fatalf("cards %+v, want the two safe ones", cards)
	}
	reply = `{"cards":[{"q":"In C, what does the == operator test?","a":"That two values are equal"},{"q":"In C, what value does a false comparison produce?","a":"0"},{"q":"In C, which operator negates a boolean value?","a":"!"}]}`
	if cards, err := w.app.DraftBridges(w.ctx, w.user, p); err != nil || len(cards) != 3 {
		t.Fatalf("three safe bridges = %+v, %v; want all three", cards, err)
	}
	reply = `{"cards":[{"q":"In C, what does the == operator test?","a":"That two values are equal"},{"q":"Which programming languages use != to mean not equal?","a":"C and Java"},{"q":"How is inequality written in C?","a":"!="}]}`
	if cards, err := w.app.DraftBridges(w.ctx, w.user, p); !errors.Is(err, app.ErrBridgesUnavailable) || cards != nil {
		t.Fatalf("one safe card left = %+v, %v; want ErrBridgesUnavailable", cards, err)
	}
}

// notSentBridger refuses every request before dispatch, as the real
// adapter does for input that cannot fit its cap.
type notSentBridger struct{ calls int }

func (b *notSentBridger) Name() string { return "not-sent" }
func (b *notSentBridger) Bridge(context.Context, ai.BridgeInput) ([]ai.BridgeCard, ai.Usage, error) {
	b.calls++
	return nil, ai.Usage{}, ai.ErrBridgeNotSent
}

// Slice 75 (pass 4): an otherwise valid card whose question, answer and
// recent multibyte misses exceed the request cap is fitted before any
// budget is reserved (the oldest misses go), so the real adapter sends it
// and nothing stays reserved. A request refused before dispatch releases
// its reservation; a failure after dispatch with no usage (outcome
// unknown) keeps it pending, as before (G2).
func TestSlice75_Generation_OverCapInputNeverStrandsAReservation(t *testing.T) {
	w, fb, ids := struggleWorld(t)
	p := ids[0]
	q, a := strings.Repeat("q", 899)+"?", strings.Repeat("a", 1900)
	if err := w.us.UpdateItem(w.ctx, p, q, a, false); err != nil {
		t.Fatal(err)
	}
	miss := strings.Repeat("𝔸", 300)
	w.answerOnce(t, p, srs.Missed, "one "+miss)
	w.advance(25 * time.Hour)
	w.answerOnce(t, p, srs.Missed, "two "+miss)
	w.advance(25 * time.Hour)
	w.answerOnce(t, p, srs.Missed, "three "+miss)

	var calls int
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		var req struct{ Messages []struct{ Content string } }
		json.Unmarshal(raw, &req)
		if len(req.Messages) == 2 {
			sent = req.Messages[1].Content
		}
		json.NewEncoder(rw).Encode(map[string]any{"model": ai.DefaultOpenRouterDraftModel,
			"usage":   map[string]any{"prompt_tokens": 1500, "completion_tokens": 90},
			"choices": []map[string]any{{"message": map[string]any{"content": `{"cards":[{"q":"What is the first safe bridge?","a":"Safe one"},{"q":"What is the second safe bridge?","a":"Safe two"}]}`}, "finish_reason": "stop"}}})
	}))
	t.Cleanup(srv.Close)
	c, err := ai.NewClaudeWithRouter("", "sk-or-v1-standin", "", "")
	if err != nil {
		t.Fatal(err)
	}
	c.BaseURL, c.HTTP = srv.URL, srv.Client()
	w.app.Bridger = c
	cards, err := w.app.DraftBridges(w.ctx, w.user, p)
	if err != nil || len(cards) != 2 {
		t.Fatalf("over-cap input = %+v, %v; want it fitted and drafted", cards, err)
	}
	if calls != 1 || len(sent) > ai.BridgeInputChars || !strings.Contains(sent, q) || !strings.Contains(sent, a) || !strings.Contains(sent, ">three 𝔸") || !strings.Contains(sent, ">two 𝔸") || strings.Contains(sent, ">one 𝔸") {
		t.Fatalf("calls %d, sent %d bytes; want the two newest misses, the oldest dropped", calls, len(sent))
	}
	if held, _ := w.us.ReservedMicros(w.ctx); held != 0 {
		t.Fatalf("%d micros still reserved after a fitted draft", held)
	}

	ns := &notSentBridger{}
	w.app.Bridger = ns
	for i := 0; i < 3; i++ { // retries strand nothing either
		if _, err := w.app.DraftBridges(w.ctx, w.user, p); !errors.Is(err, app.ErrBridgesUnavailable) {
			t.Fatalf("not sent = %v", err)
		}
	}
	if held, _ := w.us.ReservedMicros(w.ctx); held != 0 || ns.calls != 3 {
		t.Fatalf("%d micros reserved after %d refused-before-dispatch requests; want 0", held, ns.calls)
	}

	fb.Mode = "fail" // dispatched, outcome unknown, no usage
	w.app.Bridger = fb
	if _, err := w.app.DraftBridges(w.ctx, w.user, p); !errors.Is(err, app.ErrBridgesUnavailable) {
		t.Fatalf("ambiguous failure = %v", err)
	}
	if held, _ := w.us.ReservedMicros(w.ctx); held != w.app.BridgeBudget() {
		t.Fatalf("ambiguous failure: %d micros reserved, want the pending %d", held, w.app.BridgeBudget())
	}
}

// Slice 75 (pass 4): the draft parser and acceptance use the same limits
// (bytes, as for every card), so any card a preview shows can be accepted
// unchanged: a 400-character ASCII question is accepted, and a 350-
// character Han question (1,050 bytes) is never drafted into a preview,
// and typed in by hand it is refused with copy that names the unit.
func TestSlice75_Accept_BoundsMatchTheDraftParser(t *testing.T) {
	if ai.BridgeQuestionMaxBytes != app.MaxPromptLen || ai.BridgeAnswerMaxBytes != app.MaxAnswerLen {
		t.Fatalf("parser limits %d/%d, card limits %d/%d", ai.BridgeQuestionMaxBytes, ai.BridgeAnswerMaxBytes, app.MaxPromptLen, app.MaxAnswerLen)
	}
	w, _, ids := struggleWorld(t)
	p := ids[0]
	w.answerOnce(t, p, srs.Missed, "x")
	w.advance(25 * time.Hour)
	w.answerOnce(t, p, srs.Missed, "y")
	ascii := "Q" + strings.Repeat("abcdefghij", 39) + "klmnopqr?"
	han := strings.Repeat("桥", 349) + "？"
	if cards, err := ai.ParseBridges(`{"cards":[{"q":"`+han+`","a":"桥"},{"q":"What is y?","a":"y"},{"q":"What is z?","a":"z"}]}`, ai.BridgeInput{Question: "parent q", Answer: "parent a"}); err != nil || len(cards) != 2 || strings.Contains(cards[0].Question, "桥") {
		t.Fatalf("parser kept %+v, %v", cards, err)
	}
	var fe *app.FieldError
	_, err := w.app.AcceptBridges(w.ctx, w.user, p, app.BridgeKey(), []store.BridgeCard{{Prompt: han, Answer: "桥"}})
	if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "1,000 bytes") {
		t.Fatalf("350 Han characters by hand = %v", err)
	}
	res, err := w.app.AcceptBridges(w.ctx, w.user, p, app.BridgeKey(), []store.BridgeCard{{Prompt: ascii, Answer: "x"}, {Prompt: "What is y?", Answer: "y"}})
	if err != nil || res.Bridges != 2 {
		t.Fatalf("400 ASCII characters = %+v, %v", res, err)
	}
}
