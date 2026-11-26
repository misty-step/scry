package web_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"scry/internal/ai"
	"scry/internal/app"
	"scry/internal/srs"
	"scry/internal/store"
)

// postValues posts a form that may repeat keys (the preview's rows).
func (b *browser) postValues(path string, form url.Values) (int, string) {
	b.t.Helper()
	if form.Get("csrf") == "" {
		form.Set("csrf", b.csrf)
	}
	req, _ := http.NewRequest(http.MethodPost, b.w.srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return b.do(req)
}

var keyRE = regexp.MustCompile(`name="key" value="([^"]+)"`)

// struggling sets up alice with the NATO set and one card missed once
// already (seeded) and due again, then misses it a second time through the
// real study path. It returns the browser, the card and the review page.
func struggling(t *testing.T) (*world, *browser, store.Item, string) {
	t.Helper()
	w := newWorld(t)
	w.app.Bridger = &ai.FakeBridger{}
	alice := w.browser()
	alice.signUp("alice@example.com")
	alice.addSet("nato-alphabet")
	ctx := context.Background()
	us := w.lib("alice@example.com")
	due, _ := us.DueItems(ctx, w.now().Add(time.Hour), 1)
	p := due[0]
	at := w.now()
	if _, err := us.RecordReview(ctx, store.Review{ItemID: p.ID, At: at, Answer: "Beta", Verdict: srs.Missed, Reason: "r"}, srs.Next(p.Sched, srs.Missed, at), false); err != nil {
		t.Fatal(err)
	}
	w.advance(25 * time.Hour)
	body := alice.mustGet("/study")
	cur, ok := alice.current(body)
	if !ok || cur.ID != p.ID {
		t.Fatalf("the missed card should lead the session, got %+v", cur)
	}
	// While the card waits, nothing says it is struggling (no spoilers,
	// no nudges before the answer).
	hasNot(t, body, app.StruggleCue, "Build bridge cards")
	w.model.mu.Lock()
	w.model.script = []ai.Judgment{{Verdict: srs.Missed, Reason: ai.CriterionSentence(srs.Missed)}}
	w.model.mu.Unlock()
	review := alice.answer(cur, "Beta again")
	return w, alice, cur, review
}

// Slice 75: after a second Incorrect in a row, the post-answer review
// offers bridges with the cue and a 16 px line icon; "Build bridge cards"
// shows an editable preview of the fixture with nothing added yet.
func TestSlice75_Web_PostAnswerCueAndPreview(t *testing.T) {
	w, alice, p, review := struggling(t)
	has(t, review, "data-review", `data-struggle-cue`, "Repeated miss — scaffold prerequisite bridges?", "Build bridge cards",
		`action="/library/items/`+strconv.FormatInt(p.ID, 10)+`/bridges/draft"`, `name="from" value="study"`,
		`class="icon icon-bridge"`, `stroke="currentColor"`)
	before, _, _ := w.lib("alice@example.com").Counts(context.Background(), w.now())
	status, preview := alice.post("/library/items/"+strconv.FormatInt(p.ID, 10)+"/bridges/draft", "from", "study")
	if status != http.StatusOK {
		t.Fatalf("draft = %d", status)
	}
	has(t, preview, "Build bridge cards", "Bridge 1", "Bridge 2", "Bridge 3", "Bridge fixture 1", "Add bridges & hold parent",
		`name="prompt"`, `name="answer"`, "Remove", p.Prompt)
	hasNot(t, preview, "data-spinner")
	if after, _, _ := w.lib("alice@example.com").Counts(context.Background(), w.now()); after != before {
		t.Fatal("the preview added cards")
	}
	if keyRE.FindStringSubmatch(preview) == nil {
		t.Fatal("the preview has no accept key")
	}
}

// Slice 75: the full journey over HTTP: remove a row without script, edit,
// accept (twice, as a retry), see "Held by N bridges" in the review, the
// deck list and the card's page, then release by hand.
func TestSlice75_Web_AcceptHoldAndRelease(t *testing.T) {
	w, alice, p, _ := struggling(t)
	pid := strconv.FormatInt(p.ID, 10)
	_, preview := alice.post("/library/items/"+pid+"/bridges/draft", "from", "study")
	key := keyRE.FindStringSubmatch(preview)[1]
	cards := ai.FakeBridgeCards(p.Prompt)
	form := url.Values{"key": {key}, "from": {"study"}, "remove": {"2"}}
	for _, c := range cards {
		form.Add("prompt", c.Question)
		form.Add("answer", c.Answer)
	}
	status, two := alice.postValues("/library/items/"+pid+"/bridges/preview", form)
	if status != http.StatusOK || strings.Contains(two, "Bridge 3") || strings.Contains(two, cards[2].Question) || !strings.Contains(two, cards[1].Question) {
		t.Fatalf("no-JS remove: %d\n%s", status, two)
	}
	has(t, two, `name="key" value="`+key+`"`)
	accept := url.Values{"key": {key}, "from": {"study"},
		"prompt": {"Edited: what is a phonetic alphabet for?", cards[1].Question},
		"answer": {"Spelling letters clearly by voice", cards[1].Answer}}
	status, back := alice.postValues("/library/items/"+pid+"/bridges", accept)
	if status != http.StatusOK || alice.path != "/study" {
		t.Fatalf("accept from the review: %d at %s", status, alice.path)
	}
	has(t, back, "data-review", "Held by 2 bridges", "Release hold")
	hasNot(t, back, "Build bridge cards")
	// A retried submit writes nothing more.
	if status, _ := alice.postValues("/library/items/"+pid+"/bridges", accept); status != http.StatusOK {
		t.Fatalf("retry = %d", status)
	}
	us := w.lib("alice@example.com")
	bridges, _ := us.Bridges(context.Background(), p.ID)
	if len(bridges) != 2 || bridges[0].Prompt != "Edited: what is a phonetic alphabet for?" {
		t.Fatalf("bridges = %+v", bridges)
	}
	// Deck list and the card's page.
	deck := alice.mustGet("/library/ideas/" + strconv.FormatInt(p.IdeaID, 10))
	has(t, deck, "Held by 2 bridges", "Bridge card", ">held<")
	page := alice.mustGet("/library/items/" + pid)
	has(t, page, "Held by 2 bridges", "Release hold", "Edited: what is a phonetic alphabet for?", "held · ")
	bpage := alice.mustGet("/library/items/" + strconv.FormatInt(bridges[0].ID, 10))
	has(t, bpage, "Bridge card · leads up to", p.Prompt)
	// The held card is not due, so the next Study session leaves it out.
	_, dueN, _ := us.Counts(context.Background(), w.now())
	items, _ := us.DueItems(context.Background(), w.now().Add(24*time.Hour), 200)
	for _, it := range items {
		if it.ID == p.ID {
			t.Fatal("the held card is still in the due queue")
		}
	}
	// Release by hand from the card's page.
	status, released := alice.post("/library/items/"+pid+"/bridges/release", "from", "item")
	if status != http.StatusOK || !strings.Contains(released, "Hold released.") {
		t.Fatalf("release = %d\n%s", status, released)
	}
	hasNot(t, released, "Held by")
	if _, after, _ := us.Counts(context.Background(), w.now()); after <= dueN-1 {
		t.Fatalf("due count after release = %d (before %d)", after, dueN)
	}
}

// Slice 75: a card that has three lapses carries the cue in the deck list
// and on its own page, never on the waiting Study screen.
func TestSlice75_Web_DeckListAndCardDetailCue(t *testing.T) {
	w := newWorld(t)
	w.app.Bridger = &ai.FakeBridger{}
	alice := w.browser()
	alice.signUp("alice@example.com")
	alice.addSet("nato-alphabet")
	ctx := context.Background()
	us := w.lib("alice@example.com")
	due, _ := us.DueItems(ctx, w.now().Add(time.Hour), 1)
	p := due[0]
	at := w.now()
	sched := srs.State{Stability: 1.1, Difficulty: 8, Reps: 5, Lapses: 3, Last: at, Due: at.Add(24 * time.Hour)}
	us.RecordReview(ctx, store.Review{ItemID: p.ID, At: at, Answer: "x", Verdict: srs.Right, Reason: "r"}, sched, false)
	deck := alice.mustGet("/library/ideas/" + strconv.FormatInt(p.IdeaID, 10))
	has(t, deck, "data-row-cue", "Repeated miss — scaffold prerequisite bridges?")
	if strings.Count(deck, "data-row-cue") != 1 {
		t.Fatalf("%d cues in the deck list, want 1", strings.Count(deck, "data-row-cue"))
	}
	page := alice.mustGet("/library/items/" + strconv.FormatInt(p.ID, 10))
	has(t, page, "Repeated miss — scaffold prerequisite bridges?", "Build bridge cards", `name="from" value="item"`)
	w.advance(25 * time.Hour)
	body := alice.mustGet("/study")
	if cur, ok := alice.current(body); !ok || cur.ID != p.ID {
		t.Fatal("the lapsed card should be waiting")
	}
	hasNot(t, body, "Repeated miss", "Build bridge cards")
}

// Slice 75: bridges are the learner's own: every route is a 404 for
// another learner's card, and a post without the CSRF token is refused.
func TestSlice75_Web_OwnershipAndCSRF(t *testing.T) {
	w, alice, p, _ := struggling(t)
	pid := strconv.FormatInt(p.ID, 10)
	bob := w.browser()
	bob.signUp("bob@example.com")
	for _, path := range []string{"/bridges/draft", "/bridges/preview", "/bridges", "/bridges/release"} {
		if status, _ := bob.post("/library/items/" + pid + path); status != http.StatusNotFound {
			t.Errorf("bob POST %s = %d, want 404", path, status)
		}
	}
	form := url.Values{"csrf": {"wrong"}, "from": {"item"}}
	if status, _ := alice.postValues("/library/items/"+pid+"/bridges/draft", form); status != http.StatusForbidden {
		t.Fatalf("no CSRF = %d, want 403", status)
	}
	// A card that is not struggling is not offered bridges.
	other, _ := w.lib("alice@example.com").DueItems(context.Background(), w.now().Add(time.Hour), 5)
	for _, it := range other {
		if it.ID == p.ID {
			continue
		}
		status, body := alice.post("/library/items/"+strconv.FormatInt(it.ID, 10)+"/bridges/draft", "from", "item")
		if status != http.StatusOK || !strings.Contains(body, "This card isn't offered bridge cards right now.") {
			t.Fatalf("draft for a calm card = %d", status)
		}
		break
	}
}

// Slice 75: when the model fails, the preview says so plainly, offers to
// try again and adds nothing.
func TestSlice75_Web_GenerationFailureIsSaidInWords(t *testing.T) {
	w, alice, p, _ := struggling(t)
	w.app.Bridger = &ai.FakeBridger{Mode: "fail"}
	status, body := alice.post("/library/items/"+strconv.FormatInt(p.ID, 10)+"/bridges/draft", "from", "study")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", status)
	}
	has(t, body, "Scry couldn't build bridge cards just now. Nothing was added. Try again in a moment.", "Try again")
	hasNot(t, body, "Add bridges & hold parent")
}

// Slice 75: on a bridge's review, a first Correct says what it changed;
// after the last bridge the card it leads to is back.
func TestSlice75_Web_BridgeReviewSaysWhenTheParentReturns(t *testing.T) {
	w, alice, p, _ := struggling(t)
	pid := strconv.FormatInt(p.ID, 10)
	_, preview := alice.post("/library/items/"+pid+"/bridges/draft", "from", "study")
	key := keyRE.FindStringSubmatch(preview)[1]
	alice.postValues("/library/items/"+pid+"/bridges", url.Values{"key": {key}, "from": {"item"}, "prompt": {"Only bridge q"}, "answer": {"Only bridge a"}})
	// Work through the queue (the default fixture judges Correct) until the
	// bridge is up; the held parent never comes up before it.
	alice.finish(func(store.Item) string { return "x" })
	var cur store.Item
	for i, days := 0, 0; i < 80; i++ {
		body := alice.mustGet("/study")
		it, ok := alice.current(body)
		if !ok && days < 4 {
			// S111-1: at most 10 new cards a day; the bridges, the newest
			// cards, may wait for a later day.
			w.advance(24 * time.Hour)
			days++
			continue
		}
		if !ok {
			t.Fatal("ran out of cards before the bridge came up")
		}
		if it.ID == p.ID {
			t.Fatal("the held parent came up before its bridge was answered")
		}
		if it.Prompt == "Only bridge q" {
			cur = it
			break
		}
		alice.answerNext(it, "x")
	}
	if cur.ID == 0 {
		t.Fatal("the bridge never came up")
	}
	w.model.mu.Lock()
	w.model.script = []ai.Judgment{{Verdict: srs.Right, Reason: ai.CriterionSentence(srs.Right)}}
	w.model.mu.Unlock()
	review := alice.answer(cur, "Only bridge a")
	has(t, review, "Bridge cleared · the card it leads to is back in your reviews, on its own schedule.")
	if st, _ := w.lib("alice@example.com").BridgeState(context.Background(), p.ID); st.HeldBy != 0 {
		t.Fatalf("held by %d after the last bridge", st.HeldBy)
	}
}

// Slice 75 (DESIGN.md, Held): an active bridge's post-answer review says
// "Bridge card · leads up to …" with the card it leads to linked, as the
// bridge's own page does. While the bridge waits nothing names that card,
// its answer is never shown, and the link is the learner's own card only:
// another learner gets a 404 for it and never sees it.
func TestSlice75_Web_BridgeReviewNamesTheCardItLeadsTo(t *testing.T) {
	w, alice, p, _ := struggling(t)
	pid := strconv.FormatInt(p.ID, 10)
	link := `leads up to <a class="link quiet" href="/library/items/` + pid + `">` + p.Prompt + `</a>`
	_, preview := alice.post("/library/items/"+pid+"/bridges/draft", "from", "study")
	key := keyRE.FindStringSubmatch(preview)[1]
	alice.postValues("/library/items/"+pid+"/bridges", url.Values{"key": {key}, "from": {"item"},
		"prompt": {"First bridge q", "Second bridge q"}, "answer": {"First bridge a", "Second bridge a"}})
	alice.finish(func(store.Item) string { return "x" })
	// Up to the first bridge, which waits with no word of its parent.
	var body string
	var cur store.Item
	for i, days := 0, 0; i < 80; i++ {
		body = alice.mustGet("/study")
		it, ok := alice.current(body)
		if !ok && days < 4 {
			// S111-1: at most 10 new cards a day; the bridges, the newest
			// cards, may wait for a later day.
			w.advance(24 * time.Hour)
			days++
			continue
		}
		if !ok {
			t.Fatal("ran out of cards before the bridge came up")
		}
		if it.Prompt == "First bridge q" || it.Prompt == "Second bridge q" {
			cur = it
			break
		}
		alice.answerNext(it, "x")
	}
	if cur.ID == 0 {
		t.Fatal("no bridge came up")
	}
	hasNot(t, body, "leads up to", `href="/library/items/`+pid+`"`, "data-bridge-note")
	// Incorrect: the review names the parent, which stays held.
	w.model.mu.Lock()
	w.model.script = []ai.Judgment{{Verdict: srs.Missed, Reason: ai.CriterionSentence(srs.Missed)}}
	w.model.mu.Unlock()
	review := alice.answer(cur, "wrong")
	has(t, review, "data-review", "data-bridge-note", "Bridge card · "+link,
		"The card it leads to stays held until this is answered Correct or Almost Correct.")
	hasNot(t, review, ">"+p.Answer+"<", "data-bridge-cleared")
	if st, _ := w.lib("alice@example.com").BridgeState(context.Background(), p.ID); st.HeldBy != 2 {
		t.Fatalf("held by %d after a missed bridge, want 2", st.HeldBy)
	}
	// Reloading the review shows the same line.
	has(t, alice.mustGet("/study"), "Bridge card · "+link)
	// The link is the parent's own page, which still says it is held.
	has(t, alice.mustGet("/library/items/"+pid), "Held by 2 bridges")
	// Another learner never sees or reaches it.
	bob := w.browser()
	bob.signUp("bob@example.com")
	bob.addSet("nato-alphabet")
	if status, _ := bob.get("/library/items/" + pid); status != http.StatusNotFound {
		t.Fatalf("bob GET alice's parent = %d, want 404", status)
	}
	hasNot(t, bob.mustGet("/study"), `href="/library/items/`+pid+`"`, "leads up to")
	// Correct on the next bridge: it has done its work, so its review says
	// what clearing it changed rather than naming its parent.
	alice.next()
	body = alice.mustGet("/study")
	nxt, ok := alice.current(body)
	for i := 0; ok && nxt.Prompt != "First bridge q" && nxt.Prompt != "Second bridge q" && i < 80; i++ {
		alice.answerNext(nxt, "x")
		body = alice.mustGet("/study")
		nxt, ok = alice.current(body)
	}
	if !ok || nxt.ID == cur.ID {
		t.Fatalf("the other bridge never came up (got %+v)", nxt)
	}
	hasNot(t, body, "leads up to", `href="/library/items/`+pid+`"`)
	w.model.mu.Lock()
	w.model.script = []ai.Judgment{{Verdict: srs.Right, Reason: ai.CriterionSentence(srs.Right)}}
	w.model.mu.Unlock()
	cleared := alice.answer(nxt, "right")
	has(t, cleared, "Bridge cleared · 1 bridge still holds the card it leads to.")
	hasNot(t, cleared, ">"+p.Answer+"<")
}
