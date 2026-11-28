package web_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"scry/internal/ai"
	"scry/internal/srs"
	"scry/internal/store"
)

// s139Fixture is the [S139-1]/[S139-2] fixture the headed walkthrough
// (scripts/verify-slice139.mjs) sends through the API too: a card whose
// correct answer is 250–400 characters across three sentences, carrying
// resources, ideaResources, relatedTags and relatedIdeas, and the cards it
// is related to.
func s139Fixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/s139/fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// s139Learner signs a learner up, makes an API token in Settings and
// returns both.
func s139Learner(t *testing.T, w *world, email string) (*browser, string) {
	t.Helper()
	b := w.browser()
	b.signUp(email)
	b.mustGet("/settings")
	_, page := b.post("/settings/tokens", "name", "session miner")
	token := tokenRE.FindString(page)
	if token == "" {
		t.Fatal("no token on the page")
	}
	return b, token
}

func s139Item(t *testing.T, lib *store.UserStore, question string) store.Item {
	t.Helper()
	all, err := lib.Search(ctx, question)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range all {
		if it.Prompt == question {
			return it
		}
	}
	t.Fatalf("no card %q", question)
	return store.Item{}
}

const (
	s139Long    = "What is opportunity cost, and why does it matter when you decide?"
	s139Same    = "Besides its price, what does a choice cost you?"
	s139Tagged  = "What is a trade-off?"
	s139Linked  = "Why should a sunk cost not sway a new decision?"
	s139Exact   = "Recite the decision motto"
	s139OneWord = "In one word, what is a cost already spent and never recovered called?"
)

// [S139-2] By test: a request in the old shape gets the same 201 response
// as before; the four optional fields are accepted on POST /api/v1/items
// and its alias POST /api/v1/suggestions.
func TestStory_S139_2_APIAcceptsConnectedFieldsBackwardCompatible(t *testing.T) {
	w := newWorld(t)
	_, token := s139Learner(t, w, "compat@example.com")
	status, out := w.api("POST", "/api/v1/items", token, `{"source": "old script", "items": [{"question": "What does TTL stand for?", "answer": "Time to live", "idea": "API cards", "summary": "Cards from a script"}]}`)
	if status != http.StatusCreated || out["added"] != float64(1) || out["status"] != "in the library" || len(out["ids"].([]any)) != 1 || len(out) != 3 {
		t.Fatalf("old shape: %d %v", status, out)
	}
	for _, path := range []string{"/api/v1/items", "/api/v1/suggestions"} {
		status, out = w.api("POST", path, token, `{"items": [{"question": "Q via `+path+`", "answer": "A", "idea": "API cards",
			"resources": [{"title": "Read this", "kind": "link", "url": "https://example.com/read"}],
			"ideaResources": [{"title": "Idea note", "kind": "explainer", "body": "A note."}],
			"relatedTags": ["net"], "relatedIdeas": ["Caching"]}]}`)
		if status != http.StatusCreated || out["added"] != float64(1) || out["status"] != "in the library" {
			t.Fatalf("%s with the new fields: %d %v", path, status, out)
		}
	}
	lib := w.lib("compat@example.com")
	it := s139Item(t, lib, "Q via /api/v1/items")
	res, _ := lib.CardResources(ctx, it.ID)
	tags, _ := lib.ItemTags(ctx, it.ID)
	if len(res) != 2 || res[0].Title != "Read this" || res[1].Title != "Idea note" || len(tags) != 1 || tags[0] != "net" {
		t.Fatalf("resources %+v tags %v", res, tags)
	}
	// The idea's resource was sent twice (once per request): it is kept once.
	if ir, _ := lib.IdeaResources(ctx, it.IdeaID); len(ir) != 1 {
		t.Fatalf("idea resources = %+v", ir)
	}
}

// [S139-2] By test: exceeding a field limit, omitting both url and body,
// a javascript: or relative URL, or an unknown kind is refused with 422
// naming the item and field, and nothing is added. An oversized request,
// an unknown field or a wrong JSON type still gets 400.
func TestStory_S139_2_APIRefusesBadConnectedFields(t *testing.T) {
	w := newWorld(t)
	_, token := s139Learner(t, w, "strict@example.com")
	lib := w.lib("strict@example.com")
	good := `{"question": "Fine", "answer": "Yes", "idea": "Ok"}`
	item := func(extra string) string {
		return `{"items": [` + good + `, {"question": "Q", "answer": "A", "idea": "Strict", ` + extra + `}]}`
	}
	res := func(fields string) string {
		return `"resources": [{"title": "T", "kind": "link", "url": "https://example.com"}, {` + fields + `}]`
	}
	six := strings.TrimSuffix(strings.Repeat(`{"title": "T", "kind": "link", "url": "https://example.com"},`, 6), ",")
	nine := strings.TrimSuffix(strings.Repeat(`"t",`, 9), ",")
	for _, c := range []struct{ name, body, want string }{
		{"unknown kind", item(res(`"title": "T", "kind": "podcast", "url": "https://example.com"`)), "item 2, resource 2: kind must be explainer, article, video, diagram or link"},
		{"neither url nor body", item(res(`"title": "T", "kind": "article"`)), "item 2, resource 2: needs a url or a body"},
		{"javascript: url", item(res(`"title": "T", "kind": "link", "url": "javascript:alert(1)"`)), "item 2, resource 2: url must be an absolute https:// or http:// URL"},
		{"relative url", item(res(`"title": "T", "kind": "link", "url": "/library"`)), "item 2, resource 2: url must be an absolute"},
		{"scheme-relative url", item(res(`"title": "T", "kind": "link", "url": "//evil.example/x"`)), "item 2, resource 2: url must be an absolute"},
		{"data: url", item(res(`"title": "T", "kind": "link", "url": "data:text/html,hi"`)), "item 2, resource 2: url must be an absolute"},
		{"empty title", item(res(`"title": " ", "kind": "link", "url": "https://example.com"`)), "item 2, resource 2: title is required"},
		{"title over 120", item(res(`"title": "` + strings.Repeat("t", 121) + `", "kind": "link", "url": "https://example.com"`)), "item 2, resource 2: title must be at most 120 characters"},
		{"url over 2048 bytes", item(res(`"title": "T", "kind": "link", "url": "https://example.com/` + strings.Repeat("a", 2030) + `"`)), "item 2, resource 2: url must be at most 2,048 bytes"},
		{"body over 1000", item(res(`"title": "T", "kind": "explainer", "body": "` + strings.Repeat("b", 1001) + `"`)), "item 2, resource 2: body must be at most 1,000 characters"},
		{"six resources", item(`"resources": [` + six + `]`), "item 2, resources: send at most 5"},
		{"six idea resources", item(`"ideaResources": [` + six + `]`), "item 2, ideaResources: send at most 5"},
		{"bad idea resource", item(`"ideaResources": [{"title": "T", "kind": "video", "url": "ftp://example.com/v"}]`), "item 2, idea resource 1: url must be an absolute"},
		{"nine tags", item(`"relatedTags": [` + nine + `]`), "item 2, relatedTags: send at most 8"},
		{"tag over 40", item(`"relatedTags": ["` + strings.Repeat("x", 41) + `"]`), "item 2, related tag 1: a tag is 1 to 40 characters"},
		{"empty tag", item(`"relatedTags": ["  "]`), "item 2, related tag 1: a tag is 1 to 40 characters"},
		{"nine related ideas", item(`"relatedIdeas": [` + nine + `]`), "item 2, relatedIdeas: send at most 8"},
	} {
		status, out := w.api("POST", "/api/v1/items", token, c.body)
		msg, _ := out["error"].(string)
		if status != http.StatusUnprocessableEntity || !strings.HasPrefix(msg, c.want) {
			t.Errorf("%s: %d %q, want 422 %q", c.name, status, msg, c.want)
		}
	}
	for _, c := range []struct{ name, body string }{
		{"unknown field on a resource", item(`"resources": [{"title": "T", "kind": "link", "url": "https://example.com", "image": "x"}]`)},
		{"unknown field on an item", item(`"related": ["x"]`)},
		{"resources not a list", item(`"resources": "https://example.com"`)},
		{"tag not a string", item(`"relatedTags": [1]`)},
		{"malformed JSON", `{"items": [`},
		{"oversized request", item(`"resources": [{"title": "T", "kind": "explainer", "body": "` + strings.Repeat("b", 1<<20) + `"}]`)},
	} {
		if status, out := w.api("POST", "/api/v1/items", token, c.body); status != http.StatusBadRequest {
			t.Errorf("%s: %d %v, want 400", c.name, status, out)
		}
	}
	// All or none: nothing of any refused request was added.
	if n, _, _ := lib.Counts(ctx, w.now()); n != 0 {
		t.Fatalf("refused requests added %d cards", n)
	}
	exp, _ := lib.Export(ctx, w.now())
	if len(exp.Tables["learning_resources"]) != 0 || len(exp.Tables["item_tags"]) != 0 || len(exp.Tables["idea_pending_links"]) != 0 {
		t.Fatalf("refused requests left rows: %v", exp.Tables)
	}
}

// [S139-2] Related cards: same idea, then a shared tag, then a linked idea,
// each in Library order, at most 5; never the card itself, a line of an
// exact text or a removed card (a stand-alone exact card is listed). relatedIdeas links an existing idea at once and
// one that arrives later when it does. Resources and tags survive Recently
// removed and restore, and appear in the JSON export.
func TestStory_S139_2_RelatedCardsOrderLimitAndRemoval(t *testing.T) {
	w := newWorld(t)
	_, token := s139Learner(t, w, "related@example.com")
	lib := w.lib("related@example.com")
	// An idea that is already there before the fixture names it.
	if st, out := w.api("POST", "/api/v1/items", token, `{"items": [{"question": "What is a sunk cost?", "answer": "A cost already paid", "idea": "Sunk cost"}]}`); st != http.StatusCreated {
		t.Fatalf("%d %v", st, out)
	}
	// One that arrives only after: "Later idea" is named first, added next.
	if st, out := w.api("POST", "/api/v1/items", token, `{"items": [{"question": "Pending first", "answer": "P", "idea": "Waiting idea", "relatedIdeas": ["Later idea", "waiting IDEA"]}]}`); st != http.StatusCreated {
		t.Fatalf("%d %v", st, out)
	}
	if st, out := w.api("POST", "/api/v1/items", token, s139Fixture(t)); st != http.StatusCreated || out["added"] != float64(6) {
		t.Fatalf("fixture: %d %v", st, out)
	}
	long := s139Item(t, lib, s139Long)
	rel, err := lib.RelatedCards(ctx, long.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range rel {
		got = append(got, r.Prompt)
	}
	// Same idea, then the shared tag ("Trade-offs", "trade-offs" and
	// "TRADE-OFFS" match, so the stand-alone exact card "Recite the
	// decision motto" is listed: it is not a line of an exact text), then
	// the linked idea's cards in Library order (the earlier "What is a
	// sunk cost?" first).
	want := []string{s139Same, s139Tagged, s139Exact, "What is a sunk cost?", s139Linked}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("related =\n%q\nwant\n%q", got, want)
	}
	// relatedIdeas linked the existing "Sunk cost" idea at once.
	related, _ := lib.Related(ctx, long.IdeaID)
	if len(related) != 1 || related[0].Title != "Sunk cost" {
		t.Fatalf("idea links = %+v", related)
	}
	// The lines of an exact text are never related, even through a linked
	// idea: each line's question quotes the line before it.
	verse := w.addExactText("related@example.com", "Decision verse", "Two roads diverged\nAnd I took the one less travelled")
	if err := lib.Link(ctx, long.IdeaID, verse); err != nil {
		t.Fatal(err)
	}
	all, _ := lib.RelatedIDs(ctx, long.ID)
	verseLines, _ := lib.Items(ctx, verse)
	if len(verseLines) != 2 || len(all) != 5 {
		t.Fatalf("%d verse lines, %d related", len(verseLines), len(all))
	}
	for _, l := range verseLines {
		if all[l.ID] {
			t.Errorf("an exact-text line is related: %q", l.Prompt)
		}
	}
	// "Later idea" arrives afterwards and is linked then; an idea is never
	// linked to itself ("waiting IDEA").
	waiting := s139Item(t, lib, "Pending first")
	if r, _ := lib.Related(ctx, waiting.IdeaID); len(r) != 0 {
		t.Fatalf("linked before it arrived: %+v", r)
	}
	if st, _ := w.api("POST", "/api/v1/items", token, `{"items": [{"question": "Arrives later", "answer": "L", "idea": "later IDEA"}]}`); st != http.StatusCreated {
		t.Fatal(st)
	}
	if r, _ := lib.Related(ctx, waiting.IdeaID); len(r) != 1 || r[0].Title != "later IDEA" {
		t.Fatalf("a later idea was not linked: %+v", r)
	}

	// The limit: more same-idea cards push the list to 5, in Library order.
	for i := 1; i <= 6; i++ {
		if st, _ := w.api("POST", "/api/v1/items", token, fmt.Sprintf(`{"items": [{"question": "Extra %d", "answer": "E", "idea": "Opportunity cost"}]}`, i)); st != http.StatusCreated {
			t.Fatal(st)
		}
	}
	rel, _ = lib.RelatedCards(ctx, long.ID)
	got = got[:0]
	for _, r := range rel {
		got = append(got, r.Prompt)
		if r.ID == long.ID {
			t.Fatal("a card is related to itself")
		}
	}
	if strings.Join(got, "|") != strings.Join([]string{s139Same, "Extra 1", "Extra 2", "Extra 3", "Extra 4"}, "|") {
		t.Fatalf("limited related = %q", got)
	}
	// A removed card is left out; restored, it is back.
	same := s139Item(t, lib, s139Same)
	if err := lib.RemoveItem(ctx, same.ID, w.now()); err != nil {
		t.Fatal(err)
	}
	rel, _ = lib.RelatedCards(ctx, long.ID)
	for _, r := range rel {
		if r.ID == same.ID {
			t.Fatal("a removed card is still related")
		}
	}

	// Resources and tags go to Recently removed with their card and come
	// back with it; the idea's resources with the idea.
	tagged := s139Item(t, lib, s139Tagged)
	if err := lib.RemoveItem(ctx, long.ID, w.now()); err != nil {
		t.Fatal(err)
	}
	if res, err := lib.CardResources(ctx, long.ID); err != nil || len(res) != 0 {
		t.Fatalf("a removed card still shows resources: %v %v", res, err)
	}
	// Only the stand-alone exact card shares the tag now.
	if rel, _ := lib.RelatedCards(ctx, tagged.ID); len(rel) != 1 || rel[0].Prompt != s139Exact {
		t.Fatalf("a removed card is related by its tag: %+v", rel)
	}
	if err := lib.RestoreItem(ctx, long.ID); err != nil {
		t.Fatal(err)
	}
	res, _ := lib.CardResources(ctx, long.ID)
	tags, _ := lib.ItemTags(ctx, long.ID)
	if len(res) != 3 || len(tags) != 1 || tags[0] != "trade-offs" {
		t.Fatalf("after restore: resources %+v tags %v", res, tags)
	}
	if rel, _ := lib.RelatedCards(ctx, tagged.ID); len(rel) != 2 || rel[0].ID != long.ID || rel[1].Prompt != s139Exact {
		t.Fatalf("restored tag does not relate: %+v", rel)
	}
	if err := lib.RemoveIdea(ctx, long.IdeaID, w.now()); err != nil {
		t.Fatal(err)
	}
	if err := lib.RestoreIdea(ctx, long.IdeaID); err != nil {
		t.Fatal(err)
	}
	if ir, _ := lib.IdeaResources(ctx, long.IdeaID); len(ir) != 1 || ir[0].Title != "A map of one choice" {
		t.Fatalf("idea resources after restore = %+v", ir)
	}
	// The JSON export holds them; Markdown and CSV are unchanged.
	exp, err := lib.Export(ctx, w.now())
	if err != nil {
		t.Fatal(err)
	}
	if len(exp.Tables["learning_resources"]) != 3 || len(exp.Tables["item_tags"]) != 3 {
		t.Fatalf("export: %d resources, %d tags", len(exp.Tables["learning_resources"]), len(exp.Tables["item_tags"]))
	}
	raw, _ := json.Marshal(exp)
	if !strings.Contains(string(raw), "https://en.wikipedia.org/wiki/Opportunity_cost") || !strings.Contains(string(raw), `"tag":"trade-offs"`) {
		t.Fatal("the JSON export lacks the resources or tags")
	}
}

// [S139-2] An idea title or tag that matches in another learner's library
// never creates a link or exposes their cards or resources.
func TestStory_S139_2_ConnectedLearningStaysInMyLibrary(t *testing.T) {
	w := newWorld(t)
	_, eveToken := s139Learner(t, w, "eve@example.com")
	if st, _ := w.api("POST", "/api/v1/items", eveToken, `{"items": [{"question": "Eve's secret question", "answer": "Eve's answer", "idea": "Sunk cost", "relatedTags": ["trade-offs"],
		"resources": [{"title": "Eve's private link", "kind": "link", "url": "https://eve.example/private"}],
		"ideaResources": [{"title": "Eve's idea note", "kind": "explainer", "body": "Private."}], "relatedIdeas": ["Opportunity cost"]}]}`); st != http.StatusCreated {
		t.Fatal(st)
	}
	b, token := s139Learner(t, w, "me@example.com")
	if st, _ := w.api("POST", "/api/v1/items", token, s139Fixture(t)); st != http.StatusCreated {
		t.Fatal(st)
	}
	lib := w.lib("me@example.com")
	long := s139Item(t, lib, s139Long)
	rel, _ := lib.RelatedCards(ctx, long.ID)
	for _, r := range rel {
		if strings.Contains(r.Prompt, "Eve") {
			t.Fatalf("another learner's card is related: %+v", r)
		}
	}
	if r, _ := lib.Related(ctx, long.IdeaID); len(r) != 1 {
		t.Fatalf("idea links = %+v", r)
	}
	// Eve's "Opportunity cost" relatedIdeas never reached my idea, and my
	// "Sunk cost" never linked to hers.
	eve := w.lib("eve@example.com")
	eveItem := s139Item(t, eve, "Eve's secret question")
	if r, _ := eve.Related(ctx, eveItem.IdeaID); len(r) != 0 {
		t.Fatalf("eve's idea was linked across libraries: %+v", r)
	}
	if rel, _ := eve.RelatedCards(ctx, eveItem.ID); len(rel) != 0 {
		t.Fatalf("eve sees my cards: %+v", rel)
	}
	page := b.mustGet("/library/items/" + strconv.FormatInt(long.ID, 10))
	hasNot(t, page, "Eve's", "eve.example")
	if status, _ := b.get("/library/items/" + strconv.FormatInt(eveItem.ID, 10)); status != http.StatusNotFound {
		t.Fatalf("eve's card page: %d", status)
	}
}

var detailsRE = regexp.MustCompile(`(?s)<details class="connected"[^>]*>.*?</details>`)

// [S139-2] While a card waits, neither the page text nor its markup holds
// "Related & resources", a resource title or a related question. After a
// graded answer the disclosure is there, closed, its summary naming the
// counts; RELATED rows come first, then resources, with plain-text bodies
// and links that open in a new tab. A card's own page shows the same; an
// idea's page shows the idea's resources only. A card with neither shows
// nothing at all.
func TestStory_S139_2_ReviewShowsRelatedAndResourcesOnlyAfterGrading(t *testing.T) {
	w := newWorld(t)
	b, token := s139Learner(t, w, "learner@example.com")
	if st, _ := w.api("POST", "/api/v1/items", token, s139Fixture(t)); st != http.StatusCreated {
		t.Fatal(st)
	}
	lib := w.lib("learner@example.com")
	long := s139Item(t, lib, s139Long)
	spoilers := []string{"Related &amp; resources", "Related & resources", "data-connected", "The road not taken", "A map of one choice",
		"Opportunity cost on Wikipedia", "en.wikipedia.org", s139Same, s139Tagged, s139Exact, s139Linked}
	body := b.mustGet("/study")
	var seen []string
	for i := 0; i < 20; i++ {
		it, ok := b.current(body)
		if !ok {
			t.Fatalf("session ended before the long card (seen %q)", seen)
		}
		seen = append(seen, it.Prompt)
		if it.ID == long.ID {
			break
		}
		body = b.answerNext(it, "x")
	}
	// The waiting page's whole raw markup, not only its text, the
	// session's scrollback and "coming up" list included: a related card
	// keeps its row there under a question-free label ("Card 3").
	_, raw := b.get("/study")
	for _, s := range spoilers {
		if strings.Contains(raw, s) {
			t.Errorf("while the card waits the page holds %q", s)
		}
	}
	if !regexp.MustCompile(`<li>Card \d+</li>|<span class="answered-q">Card \d+</span>`).MatchString(raw) {
		t.Errorf("no question-free row for a related card in the scrollback or coming up:\n%s", regexp.MustCompile(`(?s)<ol class="(scrollback|upcoming)".*?</ol>`).FindAllString(raw, -1))
	}
	w.model.script = []ai.Judgment{{Verdict: srs.Missed, Reason: ai.CriterionSentence(srs.Missed)}}
	review := b.answer(long, "the price")
	d := detailsRE.FindString(review)
	if d == "" {
		t.Fatal("no Related & resources after grading")
	}
	if strings.Contains(d[:strings.Index(d, ">")], " open") {
		t.Error("the disclosure opens open")
	}
	has(t, d, "Related & resources", `<span class="connected-count">· 4 cards</span> <span class="connected-count">· 3 resources</span>`, "Show", "Hide",
		`href="/library/items/`+strconv.FormatInt(s139Item(t, lib, s139Same).ID, 10)+`"`, "RELATED", "ARTICLE", "EXPLAINER", "DIAGRAM",
		`href="https://en.wikipedia.org/wiki/Opportunity_cost" target="_blank" rel="noopener noreferrer nofollow"`, "en.wikipedia.org",
		"Picture two roads.\nThe one you take", "Opportunity cost · due today")
	// RELATED rows first, then resources: the card's own, then its idea's.
	order := []string{s139Same, s139Tagged, s139Exact, s139Linked, "Opportunity cost on Wikipedia", "The road not taken", "A map of one choice"}
	at := -1
	for _, o := range order {
		i := strings.Index(d, o)
		if i <= at {
			t.Fatalf("%q is out of order in\n%s", o, d)
		}
		at = i
	}
	// Never a related card's answer (the stand-alone exact card's
	// question is listed, never its answer).
	hasNot(t, d, "The best option you gave up", "Giving up some of one thing", "It is spent whatever", "Choose with the road")
	// Between Ask Why? and the schedule.
	if a, c, s := strings.Index(review, "data-explain-form"), strings.Index(review, `class="connected"`), strings.Index(review, "data-telemetry"); !(a < c && c < s) {
		t.Errorf("order: explain %d, connected %d, schedule %d", a, c, s)
	}
	// The card's own page shows the same disclosure; the idea page only the
	// idea's own resources.
	card := b.mustGet("/library/items/" + strconv.FormatInt(long.ID, 10))
	has(t, detailsRE.FindString(card), "· 4 cards</span>", "· 3 resources</span>", s139Same, "The road not taken")
	idea := detailsRE.FindString(b.mustGet("/library/ideas/" + strconv.FormatInt(long.IdeaID, 10)))
	has(t, idea, `<span class="connected-count">· 1 resource</span></span>`, "A map of one choice", "DIAGRAM")
	hasNot(t, idea, "RELATED", "The road not taken", "cards")
	// A card with neither related cards nor resources draws nothing.
	lone := s139Item(t, lib, s139OneWord)
	hasNot(t, b.mustGet("/library/items/"+strconv.FormatInt(lone.ID, 10)), "data-connected", "Related & resources")
	sunk := s139Item(t, lib, s139Linked)
	hasNot(t, b.mustGet("/library/ideas/"+strconv.FormatInt(sunk.IdeaID, 10)), "data-connected")
}

// [S139-2] The Ask Why? fixture for Almost and Incorrect ends in one
// "Rebuild:" sentence within 280 characters that names the concrete gap:
// the card's core point set against the learner's answer (for "the price"
// against opportunity cost, the best alternative given up, not the price).
// A Correct one has none; exact texts still have no "Ask Why?".
func TestStory_S139_2_AskWhyNamesTheGapToRebuild(t *testing.T) {
	long := strings.Repeat("A long correct answer that goes on and on. ", 12)
	opportunity := "Opportunity cost is the value of the best alternative you give up when you choose one option over another. It is not the price you pay, but what that same money, time or attention could have done for you elsewhere."
	for _, c := range []struct {
		v        srs.Verdict
		answer   string
		expected string
		rebuild  string // the whole last sentence, when it is known
	}{
		{srs.Missed, "the price", opportunity,
			"Rebuild: the card's point is “Opportunity cost is the value of the best alternative you give up when you choose one option over another”, not “the price”."},
		{srs.Missed, "something else", "Lima", "Rebuild: the card's point is “Lima”, not “something else”."},
		{srs.Close, "nearly", "Lima", "Rebuild: the card's point is “Lima”, not “nearly”."},
		{srs.Close, "", "Lima", "Rebuild: the card's point is “Lima”."},
		{srs.Missed, "the sum\nof two", "Ninety\nand more", "Rebuild: the card's point is “Ninety”, not “the sum of two”."},
		{srs.Missed, long, long, ""},
		{srs.Close, "", long, ""},
	} {
		text := ai.FakeExplanation(ai.ExplainInput{Question: "Q", Expected: c.expected, Answer: c.answer, Verdict: c.v})
		_, last, ok := strings.Cut(text, ". Rebuild:")
		last = "Rebuild:" + last
		if !ok || utf8.RuneCountInString(text) > ai.ExplainMaxChars || strings.Count(text, "Rebuild:") != 1 ||
			!strings.HasPrefix(text, "What differed:") || ai.ClampExplanation(text) != text || !strings.HasSuffix(text, ".") {
			t.Errorf("%s: %q (%d chars)", c.v, text, utf8.RuneCountInString(text))
		}
		if c.rebuild != "" && last != c.rebuild {
			t.Errorf("%s %q: last sentence %q, want %q", c.v, c.answer, last, c.rebuild)
		}
		// The gap is concrete: it quotes the card's point and, when there
		// is one, the learner's answer it differs from.
		if !strings.Contains(last, "the card's point is “") || (c.answer != "" && !strings.Contains(last, ", not “")) {
			t.Errorf("%s %q: the Rebuild sentence names no concrete gap: %q", c.v, c.answer, last)
		}
	}
	right := ai.FakeExplanation(ai.ExplainInput{Question: "Q", Expected: "Lima", Answer: "Lima", Verdict: srs.Right})
	if strings.Contains(right, "Rebuild") || !strings.HasPrefix(right, "Why it holds:") {
		t.Errorf("right: %q", right)
	}
	// Through the running app, on an Incorrect answer ("something else"
	// to a NATO letter): the gap names the card's own answer and the
	// learner's.
	_, b, rv, _ := explainWorld(t, "rebuild@example.com", srs.Missed)
	status, body := b.explain(rv.ID, true)
	var out struct {
		Text     string
		Fallback bool
	}
	_ = json.Unmarshal([]byte(body), &out)
	want := "Rebuild: the card's point is “" + rv.GradedExpected + "”, not “something else”."
	if status != 200 || out.Fallback || !strings.HasSuffix(out.Text, want) {
		t.Fatalf("%d %s, want it to end %q", status, body, want)
	}
	// Exact texts still have no Ask Why? (TestExplain_NotOfferedOnExactTexts
	// proves it on the running app too).
}

var longPairRE = regexp.MustCompile(`data-long-pair`)

// [S139-1] A long answer gets the whole page: an Incorrect pair with either
// answer long (more than 80 characters, or a line break) stacks "Your
// answer" over "Correct answer" at Title size, never in a box; two short
// answers keep Judgment size. The styles say Title size with a 1.55 line
// height at the reading measure, a 2 px strike, kept line breaks, and no
// scroll box, clip or fade anywhere in the review.
func TestStory_S139_1_LongAnswerGetsTheWholePage(t *testing.T) {
	w := newWorld(t)
	b, token := s139Learner(t, w, "long@example.com")
	if st, _ := w.api("POST", "/api/v1/items", token, s139Fixture(t)); st != http.StatusCreated {
		t.Fatal(st)
	}
	answers := map[string]string{
		s139Long:    "the price",                           // short wrong answer, long correct one: stacks
		s139Same:    "the best\noption",                    // 80 or fewer characters with a line break: stacks
		s139Tagged:  strings.Repeat("a longer answer ", 6), // a long learner answer, short correct one: stacks
		s139Linked:  "no",                                  // two short answers: Judgment size
		s139OneWord: "Fixed",                               // two short answers: Judgment size
	}
	wantLong := map[string]bool{s139Long: true, s139Same: true, s139Tagged: true}
	body := b.mustGet("/study")
	done := 0
	for i := 0; i < 20; i++ {
		it, ok := b.current(body)
		if !ok {
			break
		}
		a, ok := answers[it.Prompt]
		if !ok {
			a = "x"
		}
		w.model.script = []ai.Judgment{{Verdict: srs.Missed, Reason: ai.CriterionSentence(srs.Missed)}}
		review := b.answer(it, a)
		if _, isExact := map[string]bool{s139Exact: true}[it.Prompt]; !isExact {
			done++
			if got := longPairRE.MatchString(review); got != wantLong[it.Prompt] {
				t.Errorf("%q answered %q: long pair = %v", it.Prompt, a, got)
			}
			if wantLong[it.Prompt] {
				has(t, review, `<p class="meta long-label">Your answer</p>`, `<p class="meta long-label">Correct answer</p>`, `class="long-struck-text" data-submitted>`+strings.TrimSpace(a)+`<`)
				hasNot(t, review, `class="judgment judgment-struck"`)
				if strings.Index(review, ">Your answer</p>") > strings.Index(review, ">Correct answer</p>") {
					t.Errorf("%q: Correct answer above Your answer", it.Prompt)
				}
			} else {
				has(t, review, `class="judgment judgment-struck"`, `class="judgment judgment-right"`)
			}
		}
		body = b.next()
	}
	if done != 5 {
		t.Fatalf("reviewed %d model-graded cards", done)
	}
	css := b.mustGet("/static/app.css")
	has(t, css,
		// An unbroken URL wraps inside its column and never widens the page:
		// every level is a minmax(0, 1fr) track with min-width: 0.
		".judgment-pair.judgment-long { display: grid; grid-template-columns: minmax(0, 1fr); justify-items: start; gap: var(--s3); min-width: 0; max-width: 100%; }",
		".long-answer { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--s1); min-width: 0; max-width: 100%; }",
		"font: 500 var(--title)/1.55 var(--text)",
		"min-width: 0; max-width: min(var(--measure), 100%); white-space: pre-wrap; overflow-wrap: anywhere; word-break: break-word;",
		"text-decoration-color: var(--missed); text-decoration-thickness: 2px;",
		".judged .judgment-long { animation: rise-in",
		"--title: 24px;", "--measure: 68ch;", "min-height: 100dvh;")
	// The strike on a long answer fades in with it; it is not drawn by clip.
	if strings.Contains(css, ".long-struck-text { animation") {
		t.Error("the long strike draws instead of fading in")
	}
	// Nothing in the review is a box: no height, max-height or overflow on
	// the review's own classes.
	for _, sel := range []string{".long-text", ".long-answer", ".judgment-long", ".connected-row", ".connected-list", ".connected-body", ".compare-answer", ".explanation", ".telemetry", ".review-foot"} {
		re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(sel) + `[ ,{][^}]*\b(max-height|overflow-y: (auto|scroll)|overflow: (auto|scroll|hidden)|height:)`)
		if m := re.FindString(css); m != "" {
			t.Errorf("%s is a box: %s", sel, m)
		}
	}
}

// [S139-2] The disclosure's styles: a closed row at least 44 px tall under
// a 1 px rule; open, hairline-ruled rows with 12 px above and below, no
// fill, no shadow; an 88 px kind column (above the text on phones); it
// opens in height and opacity over 200 ms ease-out, at once under reduced
// motion.
func TestDesign_S139_2_RelatedAndResourcesDisclosure(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	css := b.mustGet("/static/app.css")
	has(t, css,
		".connected { max-width: var(--measure); border-top: 1px solid var(--line);",
		"min-height: var(--touch); padding: var(--s2) 0; cursor: pointer; font: 400 var(--meta)/1.4 var(--mono);",
		"display: grid; grid-template-columns: 88px minmax(0, 1fr);",
		"padding: 12px 0; border-top: 1px solid var(--line); background: none;",
		".connected:not([open]) .connected-when-open, .connected[open] .connected-when-closed { display: none; }",
		".connected-row { grid-template-columns: minmax(0, 1fr); }",
		"transition: block-size var(--motion-arrive) var(--ease), opacity var(--motion-arrive) var(--ease)",
		".connected::details-content { transition: none !important; }",
		".connected-toggle:hover .connected-name { text-decoration-line: underline; }",
		".connected-body { color: var(--ink-body); font: 400 var(--body)/1.55 var(--text); white-space: pre-wrap; overflow-wrap: anywhere;",
		"--motion-arrive: 200ms;", "--ease: cubic-bezier(0.2, 0, 0, 1);")
}
