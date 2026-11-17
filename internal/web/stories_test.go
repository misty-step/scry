package web_test

// One test per PRODUCT.md user story. Each is named for its story and
// drives the real HTTP surface with test-mode adapters.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"scry/internal/ai"
	"scry/internal/app"
	"scry/internal/srs"
	"scry/internal/starter"
	"scry/internal/store"
)

var ctx = context.Background()

// Story: As a newcomer, I can find Scry, sign up with email and password,
// start a trial and be studying within a minute.
func TestStory_Newcomer_SignUpStartTrialAndStudyWithinAMinute(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	pages := 0

	// Find Scry.
	home := b.mustGet("/")
	pages++
	has(t, home, "Scry gets what's worth knowing into your head and keeps it there", `href="/signup"`, "7-day trial")
	_, robots := b.get("/robots.txt")
	has(t, robots, "Allow: /$")

	// Sign up with email and password.
	has(t, b.mustGet("/signup"), `name="email"`, `name="password"`)
	pages++
	_, card := b.post("/signup", "email", "newcomer@example.com", "password", "correct horse")
	pages++
	if b.path != "/signup/card" {
		t.Fatalf("after sign-up the newcomer is at %s, want the card step", b.path)
	}

	// Start the trial: the card step says the terms in plain words.
	trialEnd := w.now().AddDate(0, 0, 7).Format("January 2")
	has(t, card, "7-day trial. You won't be charged until "+trialEnd+". Cancel any time before then in Billing.")
	_, shelf := b.post("/signup/card", "number", goodCard, "expiry", "12/30", "cvc", "123")
	pages++
	if b.path != "/shelf" {
		t.Fatalf("after the card step the newcomer is at %s, want the starter shelf", b.path)
	}
	has(t, shelf, "Start with something worth knowing.")
	sub, err := w.lib("newcomer@example.com").Subscription(ctx)
	if err != nil || sub.Status != "trialing" || !sub.TrialEnd.Equal(w.now().AddDate(0, 0, 7)) {
		t.Fatalf("trial not started: %+v %v", sub, err)
	}

	// Pick a set and be studying.
	b.post("/shelf/add", "slug", "nato-alphabet")
	pages++
	study := b.mustGet("/study")
	pages++
	if _, ok := b.current(study); !ok {
		t.Fatal("no item is waiting for an answer on the study page")
	}
	has(t, study, "NATO alphabet: A", "enter to check · space to speak")

	// "Within a minute": six pages and four forms, no email to confirm,
	// nothing to configure.
	if pages > 6 {
		t.Errorf("it took %d pages to reach the first question; want at most 6", pages)
	}
}

func TestStory_Newcomer_SignUpErrorsAreSaidInWordsUnderTheField(t *testing.T) {
	w := newWorld(t)
	w.browser().signUp("taken@example.com")
	b := w.browser()
	b.get("/signup")
	status, body := b.post("/signup", "email", "taken@example.com", "password", "correct horse")
	if status != http.StatusUnprocessableEntity {
		t.Errorf("status %d", status)
	}
	has(t, body, `id="email-error" role="alert">That email already has an account.`, `value="taken@example.com"`)
	_, body = b.post("/signup", "email", "new@example.com", "password", "short")
	has(t, body, `id="password-error" role="alert">Use at least 8 characters.`)
	_, body = b.post("/signup", "email", "not-an-email", "password", "correct horse")
	has(t, body, "That doesn't look like an email address.")

	// Signing in with the wrong password is also said in words.
	b.get("/signin")
	_, body = b.post("/signin", "email", "taken@example.com", "password", "wrong password")
	has(t, body, "That email and password don't match.")
	_, _ = b.post("/signin", "email", "taken@example.com", "password", "correct horse")
	if b.path != "/study" && b.path != "/shelf" {
		t.Errorf("signed in but ended at %s", b.path)
	}
}

func TestStory_Newcomer_CardDeclinedShowsTheProcessorsReason(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.get("/signup")
	b.post("/signup", "email", "newcomer@example.com", "password", "correct horse")
	for number, reason := range map[string]string{
		"4000 0000 0000 0002": "Your card was declined.",
		"4000 0000 0000 9995": "Your card has insufficient funds.",
		"4000 0000 0000 0069": "Your card has expired.",
	} {
		status, body := b.post("/signup/card", "number", number, "expiry", "12/30", "cvc", "123")
		if status != http.StatusPaymentRequired {
			t.Errorf("%s: status %d", number, status)
		}
		has(t, body, `role="alert">`+reason)
	}
	if _, err := w.lib("newcomer@example.com").Subscription(ctx); err != store.ErrNotFound {
		t.Fatalf("a declined card must not start a trial (err %v)", err)
	}
}

// Story: As a new user with an empty library, I can pick from about 30
// excellent starter sets and start right away.
func TestStory_StarterShelf_EmptyLibraryPicksFromAboutThirtySetsAndStarts(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("new@example.com")

	// An empty library is the starter shelf.
	b.get("/library")
	if b.path != "/shelf" {
		t.Fatalf("an empty library should show the shelf, got %s", b.path)
	}
	b.get("/study")
	if b.path != "/shelf" {
		t.Fatalf("studying with an empty library should show the shelf, got %s", b.path)
	}
	shelf := b.mustGet("/shelf")
	tiles := strings.Count(shelf, `<li class="tile`)
	if tiles < 28 || tiles > 32 {
		t.Errorf("the shelf shows %d sets, want about 30", tiles)
	}
	for _, title := range []string{"Kitchen numbers", "The NATO alphabet", "The Creeds", "Mental models", "Words worth owning",
		"Core ideas in physics", "Core ideas in philosophy", "Core ideas in economics", "Turning points in world history"} {
		has(t, shelf, title)
	}
	hasNot(t, shelf, "Start studying") // nothing added yet

	// One tap adds a set; the main action becomes "Start studying".
	_, shelf = b.post("/shelf/add", "slug", "kitchen-numbers")
	has(t, shelf, "Added", "Start studying")
	items, due, err := w.lib("new@example.com").Counts(ctx, w.now().Add(time.Minute))
	set, _ := starter.Get("kitchen-numbers")
	if err != nil || items != set.Count() || due != items {
		t.Fatalf("after adding: %d items, %d due (want %d, all due): %v", items, due, set.Count(), err)
	}
	// ...and they can start right away.
	if _, ok := b.current(b.mustGet("/study")); !ok {
		t.Fatal("no question after adding a starter set")
	}

	// Adding the same set again changes nothing.
	b.post("/shelf/add", "slug", "kitchen-numbers")
	if again, _, _ := w.lib("new@example.com").Counts(ctx, w.now()); again != items {
		t.Errorf("adding a set twice duplicated items: %d -> %d", items, again)
	}
	// The shelf stays reachable from the library.
	has(t, b.mustGet("/library"), `href="/shelf"`)
}

// Story: As a learner, I can ask Scry for material on any topic and get
// it, shaped to me.
func TestStory_AskForAnyTopic_AndGetItShapedToMe(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("learner@example.com")
	b.addSet("kitchen-numbers")
	lib := w.lib("learner@example.com")
	before, _, _ := lib.Counts(ctx, w.now())

	// Tell Scry who I am.
	b.get("/settings")
	b.post("/settings/about", "about", "I am a nurse learning to cook for a family of five.")

	ask := b.mustGet("/learn")
	has(t, ask, `placeholder="What do you want to know?"`, "A topic, a question, or a book's title")
	b.post("/learn", "q", "mental models")
	w.app.WaitDrafts()
	draftPath := b.path
	page := b.mustGet(draftPath)

	// The model was asked, and was told who it is drafting for.
	if len(w.model.drafts) != 1 {
		t.Fatalf("model drafted %d times, want 1", len(w.model.drafts))
	}
	in := w.model.drafts[0]
	if in.Query != "mental models" || in.Learner.About != "I am a nurse learning to cook for a family of five." {
		t.Errorf("draft input not shaped to the learner: %+v", in)
	}
	if !contains(in.Learner.Knows, "Ratios worth knowing") {
		t.Errorf("the model was not told what the learner already has: %v", in.Learner.Knows)
	}

	// The draft is an outline of ideas with items, and nothing has
	// entered the library.
	has(t, page, "Second-order thinking", "The razors", "Approve all", "Nothing enters your library until you approve it.")
	if after, _, _ := lib.Counts(ctx, w.now()); after != before {
		t.Fatalf("a draft changed the library before approval: %d -> %d", before, after)
	}

	// Each item is approvable on its own...
	pending, _ := lib.Suggestions(ctx, draftID(t, draftPath))
	b.post(fmt.Sprintf("/suggestions/%d/approve", pending[0].ID))
	if after, _, _ := lib.Counts(ctx, w.now()); after != before+1 {
		t.Fatalf("approving one item added %d", after-before)
	}
	// ...or all at once.
	_, page = b.post(draftPath + "/approve-all")
	has(t, page, "All approved.")
	if after, _, _ := lib.Counts(ctx, w.now()); after != before+len(pending) {
		t.Errorf("approve all: library has %d items, want %d", after, before+len(pending))
	}

	// Shaped to me, part two: asking again leaves out what I now have.
	b.get("/learn")
	b.post("/learn", "q", "mental models")
	w.app.WaitDrafts()
	has(t, b.mustGet(b.path), "Nothing useful found.", "already in your library")
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func draftID(t *testing.T, path string) int64 {
	t.Helper()
	id, err := strconv.ParseInt(strings.TrimPrefix(path, "/learn/"), 10, 64)
	if err != nil {
		t.Fatalf("not a draft path: %s", path)
	}
	return id
}

// Story: As a learner, I can turn a favorite book into a set of its core
// ideas.
func TestStory_TurnAFavoriteBookIntoASetOfItsCoreIdeas(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("reader@example.com")
	b.get("/learn")
	b.post("/learn", "q", "Meditations by Marcus Aurelius")
	w.app.WaitDrafts()
	page := b.mustGet(b.path)
	has(t, page, "book · core ideas", ">Meditations</h1>", "The dichotomy of control", "Memento mori", "8 items in 4 ideas")

	_, page = b.post(b.path + "/approve-all")
	has(t, page, "All approved. 8 items joined your library.")

	// The book is now a set: one top-level idea with its core ideas inside.
	lib := w.lib("reader@example.com")
	outline, err := lib.Outline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(outline) != 5 || outline[0].Title != "Meditations" || outline[0].Depth != 0 {
		t.Fatalf("outline = %+v", outline)
	}
	for _, idea := range outline[1:] {
		if idea.Depth != 1 || idea.ParentID != outline[0].ID || idea.Items != 2 {
			t.Errorf("core idea %q: depth %d, parent %d, items %d", idea.Title, idea.Depth, idea.ParentID, idea.Items)
		}
	}
	// Its ideas are connected to one another.
	related, _ := lib.Related(ctx, outline[1].ID)
	if len(related) == 0 {
		t.Errorf("%q has no links", outline[1].Title)
	}
	has(t, b.mustGet("/library"), "Meditations", "1.1", "The dichotomy of control")
}

// Story: As a learner whose agents do work for me, my agents can add
// material to my library or suggest items from my conversations and
// reading. Suggestions wait in an inbox until I approve them with one tap.
func TestStory_Agents_SuggestToInboxOrAdd_ApprovedWithOneTap(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("owner@example.com")
	lib := w.lib("owner@example.com")

	// The learner makes a token for an agent; it is shown once.
	b.get("/settings")
	_, page := b.post("/settings/tokens", "name", "reading agent")
	token := regexp.MustCompile(`scry_[A-Za-z0-9_-]{20,}`).FindString(page)
	if token == "" {
		t.Fatal("no token on the page")
	}
	hasNot(t, b.mustGet("/settings"), token)

	api := func(method, path, bearer, body string) (int, map[string]any) {
		req, _ := http.NewRequest(method, w.srv.URL+path, bytes.NewBufferString(body))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	payload := `{"source": "The Federalist Papers", "items": [
		{"question": "Which Federalist paper is about faction?", "answer": "Federalist No. 10", "idea": "Faction", "summary": "Groups against the common good."},
		{"question": "Who wrote Federalist No. 10?", "answer": "James Madison", "idea": "Faction"}]}`

	// The agent suggests items from the learner's reading.
	status, out := api("POST", "/api/v1/suggestions", token, payload)
	if status != http.StatusCreated || out["suggested"] != float64(2) {
		t.Fatalf("suggest: %d %v", status, out)
	}
	// They wait in the inbox; the library has not changed.
	if n, _, _ := lib.Counts(ctx, w.now()); n != 0 {
		t.Fatalf("suggestions entered the library without approval (%d items)", n)
	}
	inbox := b.mustGet("/inbox")
	has(t, inbox, "2 suggestions", "Which Federalist paper is about faction?", "from reading agent · The Federalist Papers", "Faction", "Approve all")

	// One tap approves one.
	pending, _ := lib.Suggestions(ctx, 0)
	_, inbox = b.post(fmt.Sprintf("/suggestions/%d/approve", pending[0].ID))
	has(t, inbox, "1 suggestion")
	items, _ := lib.Search(ctx, "faction")
	if len(items) != 1 || items[0].IdeaTitle != "Faction" || items[0].Source != "from reading agent · The Federalist Papers" {
		t.Fatalf("approved item = %+v", items)
	}
	// Dismiss drops the other.
	_, inbox = b.post(fmt.Sprintf("/suggestions/%d/dismiss", pending[1].ID))
	has(t, inbox, "Nothing waiting.")

	// Approve all, in one tap.
	api("POST", "/api/v1/suggestions", token, payload)
	_, inbox = b.post("/inbox/approve-all")
	has(t, inbox, "All approved. 2 items joined your library.")

	// A suggest-only token may not add directly...
	status, _ = api("POST", "/api/v1/items", token, payload)
	if status != http.StatusForbidden {
		t.Errorf("suggest-only token adding directly: status %d, want 403", status)
	}
	// ...but a token the learner marked "may add" puts material straight in.
	b.get("/settings")
	_, page = b.post("/settings/tokens", "name", "research agent", "can_add", "1")
	adder := regexp.MustCompile(`scry_[A-Za-z0-9_-]{20,}`).FindString(page)
	n0, _, _ := lib.Counts(ctx, w.now())
	status, out = api("POST", "/api/v1/items", adder, `{"source": "Chat, Nov 16", "items": [{"question": "What is 75% hydration?", "answer": "75 g water per 100 g flour", "idea": "Bread"}]}`)
	if n1, _, _ := lib.Counts(ctx, w.now()); status != http.StatusCreated || n1 != n0+1 {
		t.Errorf("direct add: status %d %v, items %d -> %d", status, out, n0, n1)
	}
	// The agent can see which ideas exist, to file things well.
	status, out = api("GET", "/api/v1/ideas", adder, "")
	if status != 200 || len(out["ideas"].([]any)) < 2 {
		t.Errorf("ideas: %d %v", status, out)
	}

	// No token, a wrong token, or a revoked token gets nothing.
	if status, _ = api("POST", "/api/v1/suggestions", "", payload); status != http.StatusUnauthorized {
		t.Errorf("no token: %d", status)
	}
	if status, _ = api("POST", "/api/v1/suggestions", "scry_not_a_real_token_at_all_0000", payload); status != http.StatusUnauthorized {
		t.Errorf("wrong token: %d", status)
	}
	tokens, _ := lib.Tokens(ctx)
	b.get("/settings")
	b.post(fmt.Sprintf("/settings/tokens/%d/revoke", tokens[0].ID))
	if status, _ = api("POST", "/api/v1/suggestions", token, payload); status != http.StatusUnauthorized {
		t.Errorf("revoked token: %d", status)
	}
	// Malformed material is refused with a reason.
	if status, out = api("POST", "/api/v1/suggestions", adder, `{"items": [{"question": "No answer?", "idea": "X"}]}`); status != http.StatusUnprocessableEntity || out["error"] == "" {
		t.Errorf("bad item: %d %v", status, out)
	}
}

// Story: As a learner, I open Scry, get what's due, answer by typing or
// speaking, and I'm done in about 5 minutes.
func TestStory_OpenScry_GetWhatsDue_AnswerByTypingOrSpeaking_DoneInAboutFiveMinutes(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("learner@example.com")
	b.addSet("nato-alphabet")
	b.addSet("kitchen-numbers")
	lib := w.lib("learner@example.com")

	// Opening Scry goes straight to what is due.
	body := b.mustGet("/")
	if b.path != "/study" {
		t.Fatalf("opening Scry lands on %s, want /study", b.path)
	}
	// 48 items are due, but the session is sized to about five minutes.
	_, due, _ := lib.Counts(ctx, app.EndOfDay(w.user("learner@example.com"), w.now()))
	m := countRE.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("no position counter in the margin")
	}
	total, _ := strconv.Atoi(m[2])
	if due != 48 || total >= due || time.Duration(total)*20*time.Second > 5*time.Minute || total < 10 {
		t.Fatalf("%d due, session of %d: want a session of about five minutes", due, total)
	}

	// Answer the first by speaking: the recording is heard as text...
	first, _ := b.current(body)
	req, _ := http.NewRequest("POST", w.srv.URL+"/study/speech", strings.NewReader("say:"+first.Answer))
	req.Header.Set("Content-Type", "audio/webm")
	req.Header.Set("X-CSRF-Token", b.csrf)
	status, heard := b.do(req)
	if status != 200 || !strings.Contains(heard, first.Answer) {
		t.Fatalf("speech: %d %s", status, heard)
	}
	// ...set on the answer line, and checked like any typed answer.
	body = b.answer(first, first.Answer, "via", "spoken")
	has(t, body, `<span class="tag tag-right">right</span>`)
	hist, _ := lib.History(ctx, first.ID)
	if len(hist) != 1 || hist[0].Via != "spoken" || hist[0].Verdict != srs.Right {
		t.Fatalf("spoken answer recorded as %+v", hist)
	}

	// Type the rest: miss two, get the others right.
	n := 1
	end := b.finish(func(it store.Item) string {
		n++
		if n == 3 || n == 5 {
			w.model.mu.Lock()
			w.model.script = append(w.model.script, ai.Judgment{Verdict: srs.Missed, Reason: "Scripted miss: revisit tomorrow."})
			w.model.mu.Unlock()
			return "no idea"
		}
		return it.Answer
	})
	if b.path != "/study/done" {
		t.Fatalf("after the last answer the learner is at %s", b.path)
	}
	has(t, end, "Done.", fmt.Sprintf("%d reviewed, 2 to revisit tomorrow.", total), "Learn something new")

	// What was missed comes back tomorrow; what was right does not.
	_, dueTomorrow, _ := lib.Counts(ctx, app.EndOfDay(w.user("learner@example.com"), w.now().Add(24*time.Hour)))
	if want := (due - total) + 2; dueTomorrow != want {
		t.Errorf("due tomorrow = %d, want %d (the rest of today's, plus the 2 missed)", dueTomorrow, want)
	}
}

// Story: As a learner, I never grade myself. Every answer is judged
// instantly with a one-line reason why.
// Rule (AGENTS.md): judging an answer must use a model; no keyword, fuzzy
// or heuristic matcher decides correctness.
func TestStory_NeverGradeMyself_EveryAnswerJudgedByTheModelWithAOneLineReason(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("learner@example.com")
	b.addSet("nato-alphabet")
	lib := w.lib("learner@example.com")

	body := b.mustGet("/study")
	// There is nothing on the page to grade yourself with.
	hasNot(t, body, "I was right", "I knew it", `name="grade"`, `name="verdict"`, "Show answer")
	first, _ := b.current(body)
	hasNot(t, body, ">"+first.Answer+"<") // the answer is not on the page before answering

	// The model decides, not the text. An answer worded nothing like the
	// reference is right when the model says so...
	w.model.script = []ai.Judgment{{Verdict: srs.Right, Reason: "Yes: the first code word, with the common spelling."}}
	body = b.answer(first, "alpha, the first one")
	has(t, body, `<span class="tag tag-right">right</span>`, "Yes: the first code word, with the common spelling.")

	// ...and an answer that is letter for letter the reference is missed
	// when the model says so. No string comparison short-circuits it.
	second, _ := b.current(body)
	w.model.script = []ai.Judgment{{Verdict: srs.Missed, Reason: "The model has the final word in this test."}}
	body = b.answer(second, second.Answer)
	has(t, body, `<span class="tag tag-missed">missed</span>`, "The model has the final word in this test.")
	body = b.next()

	// "Close" comes with the parts that were close.
	third, _ := b.current(body)
	w.model.script = []ai.Judgment{{Verdict: srs.Close, Reason: "Right word, but it is spelled Charlie.", Close: []string{"Charly"}}}
	body = b.answer(third, "Charly")
	has(t, body, `<span class="tag tag-close">close</span>`, "<mark>Charly</mark>", "Right word, but it is spelled Charlie.")
	b.next()

	// Every answer went to the model, exactly once each.
	if got := w.model.judgeCalls(); got != 3 {
		t.Fatalf("model judged %d answers, want 3", got)
	}
	if in := w.model.judged[0]; in.Question != first.Prompt || in.Expected != first.Answer || in.Answer != "alpha, the first one" {
		t.Errorf("model was asked %+v", in)
	}
	// Every review in the history carries a verdict and a one-line reason.
	for _, it := range []store.Item{first, second, third} {
		hist, _ := lib.History(ctx, it.ID)
		if len(hist) != 1 {
			t.Fatalf("%q has %d reviews", it.Prompt, len(hist))
		}
		r := hist[0]
		if !r.Verdict.Valid() || r.Reason == "" || strings.Contains(r.Reason, "\n") || r.Judge != "test-judge" {
			t.Errorf("review of %q: %+v", it.Prompt, r)
		}
	}
}

func TestStory_NeverGradeMyself_WhenTheModelIsDownNothingIsGuessed(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("learner@example.com")
	b.addSet("nato-alphabet")
	lib := w.lib("learner@example.com")
	first, _ := b.current(b.mustGet("/study"))

	w.model.err = fmt.Errorf("provider is down")
	status, _ := b.get("/study")
	_ = status
	code, body := b.post("/study/answer", "item_id", strconv.FormatInt(first.ID, 10), "answer", first.Answer)
	if code != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503", code)
	}
	// The answer is kept, nothing is recorded, and someone is told.
	has(t, body, "Couldn't check that just now. Your answer is still here; press Enter to try again.", `value="`+first.Answer+`"`)
	if hist, _ := lib.History(ctx, first.ID); len(hist) != 0 {
		t.Fatalf("an unjudged answer was recorded: %+v", hist)
	}
	if alerts := w.alerts.Titles(); len(alerts) == 0 || !strings.Contains(alerts[0], "Answers cannot be judged") {
		t.Errorf("no alert was raised: %v", alerts)
	}
	// A model that returns no reason is treated the same way.
	w.model.err = nil
	w.model.script = []ai.Judgment{{Verdict: srs.Right, Reason: " "}}
	code, _ = b.post("/study/answer", "item_id", strconv.FormatInt(first.ID, 10), "answer", first.Answer)
	if hist, _ := lib.History(ctx, first.ID); code != http.StatusServiceUnavailable || len(hist) != 0 {
		t.Errorf("a judgment with no reason was accepted (status %d, %d reviews)", code, len(hist))
	}
	// Once the model is back the same answer is judged.
	has(t, b.answer(first, first.Answer), `tag-right`)
}

// Story: Texts I mark exact are judged word for word.
func TestStory_TextsIMarkExactAreJudgedWordForWord(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("learner@example.com")
	lib := w.lib("learner@example.com")

	// Mark a text exact: paste it, one line per line.
	has(t, b.mustGet("/library/texts/new"), "Add an exact text")
	_, page := b.post("/library/texts", "title", "Nicene Creed", "text", "I believe in one God, the Father Almighty,\nMaker of heaven and earth,", "parent", "0")
	has(t, page, "Nicene Creed", "exact")

	body := b.mustGet("/study")
	has(t, body, "Exact text · Nicene Creed · line 1")
	line1, _ := b.current(body)
	if !line1.Exact || line1.Answer != "I believe in one God, the Father Almighty," {
		t.Fatalf("line 1 = %+v", line1)
	}

	// One word left out, one swapped: judged in place, counted in words.
	body = b.answer(line1, "I believe in God, the Lord Almighty")
	has(t, body,
		"Exact text · Nicene Creed · line 1",
		`<ins class="x-missing"><span class="sr">missing: </span>one</ins>`,
		`<del class="x-wrong"><span class="sr">you wrote: </span>Lord</del>`,
		`<span class="x-fix"><span class="sr">the text has: </span>Father</span>`,
		"One word missing, one swapped.",
		`<span class="tag tag-close">close</span>`,
		">Again</button>", ">Next</button>")
	// No model was involved: this is the one exception.
	if n := w.model.judgeCalls(); n != 0 {
		t.Fatalf("an exact text was sent to the model (%d calls)", n)
	}
	hist, _ := lib.History(ctx, line1.ID)
	if len(hist) != 1 || hist[0].Judge != "exact" || hist[0].Verdict != srs.Close {
		t.Fatalf("history = %+v", hist)
	}
	dueAfterFirst := mustItem(t, lib, line1.ID).Sched.Due

	// "Again" puts the line back up. Word for word is right; case and
	// punctuation are not words.
	_, body = b.post("/study/next", "again", "1")
	again, ok := b.current(body)
	if !ok || again.ID != line1.ID {
		t.Fatal("Again did not bring the line back")
	}
	body = b.answer(again, "i believe in one god the father almighty")
	has(t, body, "Word for word.", `<span class="tag tag-right">right</span>`)
	// The retry is practice: the schedule set by the first attempt stands.
	if got := mustItem(t, lib, line1.ID).Sched.Due; !got.Equal(dueAfterFirst) {
		t.Errorf("practice moved the due date: %v -> %v", dueAfterFirst, got)
	}
	// "Next" moves on to line 2.
	line2, _ := b.current(b.next())
	if line2.LineNo != 2 {
		t.Errorf("after Next the item is %+v", line2)
	}

	// Any single item can be marked exact too.
	b.addSet("nato-alphabet")
	items, _ := lib.Search(ctx, "NATO alphabet: A")
	b.get(fmt.Sprintf("/library/items/%d", items[0].ID))
	b.post(fmt.Sprintf("/library/items/%d", items[0].ID), "prompt", items[0].Prompt, "answer", items[0].Answer, "exact", "1")
	if !mustItem(t, lib, items[0].ID).Exact {
		t.Error("the item was not marked exact")
	}
}

func mustItem(t *testing.T, lib *store.UserStore, id int64) store.Item {
	t.Helper()
	it, err := lib.Item(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return it
}

// Story: As a learner, I can see how ideas connect, and each item belongs
// to an idea.
func TestStory_SeeHowIdeasConnect_AndEachItemBelongsToAnIdea(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("learner@example.com")
	b.addSet("kitchen-numbers")
	b.addSet("nato-alphabet")
	lib := w.lib("learner@example.com")
	outline, _ := lib.Outline(ctx)

	// The library is a numbered outline with sub-ideas nested one level.
	page := b.mustGet("/library")
	has(t, page, `<span class="outline-num">1</span>`, `<span class="outline-num">1.1</span>`, "Kitchen numbers", "Spoons and cups", `aria-current="true"`)

	// An idea's page draws its links to related ideas, each label a link.
	var spoons, weight store.Idea
	for _, o := range outline {
		switch o.Title {
		case "Spoons and cups":
			spoons = o
		case "Weight and metric":
			weight = o
		}
	}
	page = b.mustGet(fmt.Sprintf("/library/ideas/%d", spoons.ID))
	has(t, page, `aria-label="How this idea connects"`, fmt.Sprintf(`<a class="links-box" href="/library/ideas/%d">Weight and metric</a>`, weight.ID),
		"<line ", "How many teaspoons are in a tablespoon?", "due today")

	// An idea with no links has no diagram at all.
	lonely, err := lib.CreateIdea(ctx, 0, "Unconnected", "An idea with no links.", "", w.now())
	if err != nil {
		t.Fatal(err)
	}
	hasNot(t, b.mustGet(fmt.Sprintf("/library/ideas/%d", lonely)), `class="links"`, "<line ")

	// Every item belongs to an idea: the schema will not hold one without.
	if _, err := w.store.DB().Exec(`INSERT INTO items (user_id, idea_id, prompt, answer, created_at, due_at) VALUES (?, NULL, 'q', 'a', 0, 0)`, w.user("learner@example.com").ID); err == nil {
		t.Error("an item without an idea was stored")
	}
	var orphans int
	_ = w.store.DB().QueryRow(`SELECT COUNT(*) FROM items t LEFT JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id WHERE i.id IS NULL`).Scan(&orphans)
	if orphans != 0 {
		t.Errorf("%d items belong to no idea", orphans)
	}
	// And an item can only be added to an idea of the learner's own.
	if _, err := lib.AddItem(ctx, store.NewItem{IdeaID: 999999, Prompt: "q", Answer: "a"}, w.now()); err != store.ErrNotFound {
		t.Errorf("adding to a missing idea: %v", err)
	}

	// Search shows the same rows with the matched words highlighted.
	page = b.mustGet("/library?q=teaspoons")
	has(t, page, "Results for “teaspoons”", "<mark>teaspoons</mark>", "1 item")
}

// Story: As a learner, my library and history are mine alone, and nobody
// else can see them.
func TestStory_MyLibraryAndHistoryAreMineAlone(t *testing.T) {
	w := newWorld(t)
	alice, mallory := w.browser(), w.browser()
	alice.signUp("alice@example.com")
	alice.addSet("nato-alphabet")
	first, _ := alice.current(alice.mustGet("/study"))
	alice.answer(first, first.Answer)
	alice.get("/learn")
	alice.post("/learn", "q", "Meditations")
	w.app.WaitDrafts()
	aliceDraft := alice.path
	a := w.lib("alice@example.com")
	ideas, _ := a.Outline(ctx)
	sugs, _ := a.Suggestions(ctx, draftID(t, aliceDraft))

	mallory.signUp("mallory@example.com")
	mallory.addSet("kitchen-numbers")
	m := w.lib("mallory@example.com")

	// Every address of Alice's is "not here" for Mallory.
	mallory.get("/settings")
	for _, path := range []string{
		fmt.Sprintf("/library/ideas/%d", ideas[0].ID),
		fmt.Sprintf("/library/items/%d", first.ID),
		aliceDraft,
	} {
		status, body := mallory.get(path)
		if status != http.StatusNotFound {
			t.Errorf("Mallory GET %s = %d, want 404", path, status)
		}
		hasNot(t, body, "NATO alphabet", "Meditations")
	}
	for _, path := range []string{
		fmt.Sprintf("/library/ideas/%d/remove", ideas[0].ID),
		fmt.Sprintf("/library/ideas/%d/items", ideas[0].ID),
		fmt.Sprintf("/library/items/%d", first.ID),
		fmt.Sprintf("/library/items/%d/remove", first.ID),
		fmt.Sprintf("/suggestions/%d/approve", sugs[0].ID),
		fmt.Sprintf("/suggestions/%d/dismiss", sugs[0].ID),
		aliceDraft + "/approve-all",
	} {
		status, _ := mallory.post(path, "prompt", "hacked", "answer", "hacked")
		if status != http.StatusNotFound {
			t.Errorf("Mallory POST %s = %d, want 404", path, status)
		}
	}
	// Answering Alice's item in Mallory's session does nothing.
	mallory.get("/study")
	mallory.post("/study/answer", "item_id", strconv.FormatInt(first.ID, 10), "answer", "x")
	if hist, _ := a.History(ctx, first.ID); len(hist) != 1 {
		t.Errorf("Alice's history changed: %d reviews", len(hist))
	}
	// Nothing of Alice's was touched.
	if it := mustItem(t, a, first.ID); it.Prompt != first.Prompt {
		t.Errorf("Alice's item was changed: %+v", it)
	}
	if after, _ := a.Suggestions(ctx, draftID(t, aliceDraft)); len(after) != len(sugs) {
		t.Errorf("Alice's draft was changed")
	}
	// Mallory's search, library and export hold only Mallory's things.
	hasNot(t, mallory.mustGet("/library?q=NATO"), "NATO alphabet: A")
	_, export := mallory.get("/settings/export")
	hasNot(t, export, "NATO alphabet", "alice@example.com", "Meditations")
	has(t, export, "mallory@example.com", "How many teaspoons are in a tablespoon?")

	// The same holds one layer down: Mallory's store cannot name Alice's rows.
	if _, err := m.Item(ctx, first.ID); err != store.ErrNotFound {
		t.Errorf("store: Item = %v", err)
	}
	if _, err := m.Idea(ctx, ideas[0].ID); err != store.ErrNotFound {
		t.Errorf("store: Idea = %v", err)
	}
	if hist, _ := m.History(ctx, first.ID); len(hist) != 0 {
		t.Errorf("store: History leaked %d reviews", len(hist))
	}
	if err := m.UpdateItem(ctx, first.ID, "x", "y", false); err != store.ErrNotFound {
		t.Errorf("store: UpdateItem = %v", err)
	}
	if _, err := m.Approve(ctx, sugs[0].ID, w.now()); err != store.ErrNotFound {
		t.Errorf("store: Approve = %v", err)
	}
	if err := m.Link(ctx, ideas[0].ID, ideas[1].ID); err != nil {
		t.Fatal(err)
	}
	var links int
	_ = w.store.DB().QueryRow(`SELECT COUNT(*) FROM idea_links WHERE user_id = ? OR (a_id = ? AND b_id = ? AND user_id != ?)`,
		-1, ideas[0].ID, ideas[1].ID, w.user("alice@example.com").ID).Scan(&links)
	if links != 0 {
		t.Error("store: Mallory linked Alice's ideas")
	}
	if id, _ := m.StartSession(ctx, []int64{first.ID}, w.now()); id != 0 {
		if s, _ := m.ActiveSession(ctx); s != nil && len(s.Entries) != 0 {
			t.Error("store: Mallory's session holds Alice's item")
		}
	}

	// Signed out, nothing is visible at all.
	stranger := w.browser()
	for _, path := range []string{"/library", "/study", "/inbox", "/settings/export", fmt.Sprintf("/library/items/%d", first.ID)} {
		stranger.get(path)
		if stranger.path != "/signin" {
			t.Errorf("signed out, %s ends at %s, want /signin", path, stranger.path)
		}
	}
	// And a forged form without the session's CSRF token is refused.
	mallory.csrf = "forged"
	if status, _ := mallory.post("/shelf/add", "slug", "the-creeds"); status != http.StatusForbidden {
		t.Errorf("forged CSRF token: status %d, want 403", status)
	}
}

// Story: As a paying user, I know what I pay and what my allowance
// covers, and I can top up or cancel any time.
func TestStory_PayingUser_KnowsThePriceAndAllowance_CanTopUpOrCancelAnyTime(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("payer@example.com")
	lib := w.lib("payer@example.com")
	trialEnd := w.now().AddDate(0, 0, 7)

	// On trial: the end date and the first charge, in one sentence.
	page := b.mustGet("/billing")
	has(t, page, "You're on the 7-day trial. It ends on "+trialEnd.Format("January 2")+", when your Visa ending 4242 is first charged $5.",
		"You've used 0% of this month's allowance.")

	// After the trial the plan and price are one sentence, and the
	// allowance another, with what it covers spelled out.
	w.advance(8 * 24 * time.Hour)
	page = b.mustGet("/billing")
	reset := trialEnd.AddDate(0, 1, 0).Format("January 2")
	has(t, page,
		"Scry is $5 a month, charged to your Visa ending 4242. The next charge is on "+reset+".",
		"You've used 0% of this month's allowance. It resets on "+reset+".",
		"a monthly allowance of $3 of model time: every answer judged, every spoken answer heard, and roughly",
		"Top up · $5",
		`class="meter"`,
		"$5.00", // the first charge, in the history
	)

	// Top up: one button, charged at once, credit added, shown in history.
	_, page = b.post("/billing/topup")
	has(t, page, "Topped up.", "You also have $4 of top-up credit, which is used after the allowance and does not expire.")
	sub, _ := lib.Subscription(ctx)
	if sub.TopupMicros != w.app.Cfg.TopupMicros {
		t.Errorf("top-up credit = %d", sub.TopupMicros)
	}
	ledger, _ := lib.Ledger(ctx)
	if len(ledger) != 3 || ledger[0].Kind != "topup" || ledger[0].AmountCents != 500 || ledger[0].Status != "paid" {
		t.Errorf("ledger = %+v", ledger)
	}

	// Cancel any time: one step, which states the end date.
	has(t, page, "Cancel plan", "Your plan will end on "+reset+".", "End my plan on "+reset)
	_, page = b.post("/billing/cancel")
	has(t, page, "Your plan ends on "+reset+". You keep full access until then, and nothing more will be charged.", "Keep my plan")
	// Access continues until that date, then stops; nothing more is charged.
	b.mustGet("/library")
	if b.path == "/billing" {
		t.Fatal("a cancelled plan lost access before its end date")
	}
	w.advance(40 * 24 * time.Hour)
	b.get("/signin") // the browser session has lapsed by now; the account has not
	b.post("/signin", "email", "payer@example.com", "password", "correct horse")
	b.get("/study")
	if b.path != "/billing" {
		t.Fatalf("after the end date the learner is at %s, want /billing", b.path)
	}
	has(t, b.mustGet("/billing"), "Your plan ended on "+reset+". Your library and history are kept.", "Start again")
	if after, _ := lib.Ledger(ctx); len(after) != 3 {
		t.Errorf("a cancelled plan was charged again: %+v", after)
	}

	// Changing their mind before the end date keeps the plan.
	c := w.browser()
	c.signUp("stays@example.com")
	c.get("/billing")
	c.post("/billing/cancel")
	_, page = c.post("/billing/resume")
	has(t, page, "Your plan continues.")
	if s, _ := w.lib("stays@example.com").Subscription(ctx); s.CancelAtEnd {
		t.Error("resume did not keep the plan")
	}
	// A cancelled trial is never charged.
	c.post("/billing/cancel")
	w.advance(8 * 24 * time.Hour)
	c.get("/study")
	if led, _ := w.lib("stays@example.com").Ledger(ctx); c.path != "/billing" || len(led) != 1 {
		t.Errorf("cancelled trial: at %s with ledger %+v", c.path, led)
	}
}
