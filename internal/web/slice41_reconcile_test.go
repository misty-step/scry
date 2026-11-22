package web_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"scry/internal/store"
)

// Reconciling 92350ed3: what the running app said that PRODUCT.md and
// DESIGN.md no longer say.

// "An empty question or answer is refused in words under the fields." The
// browser's own required-field bubble stopped the form before Scry could.
func TestSlice41Reconcile_Edit_EmptyFieldIsRefusedInWordsNotByTheBrowser(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("novalidate@example.com")
	b.addSet("nato-alphabet")
	item, _ := b.current(b.mustGet("/study"))
	review := b.answer(item, "Alfa")
	i := strings.Index(review, `action="/study/edit"`)
	if i < 0 {
		t.Fatal("the review has no edit form")
	}
	has(t, review[i:i+strings.Index(review[i:], ">")], "novalidate")
	id := item.ID
	if _, out, _ := b.postJSON("/study/edit", "item_id", strconv.FormatInt(id, 10), "prompt", "", "answer", "Alfa"); out["saved"] != false || out["error"] != "The question can't be empty." {
		t.Errorf("an empty question answered %v", out)
	}
}

// "Every top-level destination opens with its name and one plain line":
// Study with nothing due opened with neither. Session end stays one statement.
func TestSlice41Reconcile_NothingDue_OpensWithStudyAndItsPurpose(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("rest@example.com")
	b.addSet("nato-alphabet")
	b.finish(func(i store.Item) string { return i.Answer })
	w.advance(time.Minute)
	for n := 0; n < 10; n++ {
		if page := b.mustGet("/study"); !strings.Contains(page, "data-answer-form") {
			has(t, page, "Nothing due.", "Study · Recall what's due today", `icon-cards`)
			has(t, b.mustGet("/study/done"), "Done.")
			hasNot(t, visibleText(b.mustGet("/study/done")), "Recall what's due today")
			return
		}
		b.finish(func(i store.Item) string { return i.Answer })
	}
	t.Fatal("the set never ran out")
}

// Create and "your own scripts or tools", wherever the old words were left.
func TestSlice41Reconcile_Copy_CreateAndScriptsOrTools(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	landing := b.mustGet("/")
	has(t, landing, "Your own scripts or tools can suggest cards from what you read; nothing is added until you approve it.", "from the NATO phonetic alphabet to")
	hasNot(t, landing, "agents", "the NATO alphabet")
	req, _ := http.NewRequest(http.MethodPost, w.srv.URL+"/api/v1/suggestions", strings.NewReader("{}"))
	status, body := b.do(req)
	if status != http.StatusUnauthorized {
		t.Fatalf("no token = %d", status)
	}
	has(t, body, "send your API token as")
}

// "An item count is a compact card-stack icon and the number in tabular
// figures", in search results and a draft too.
func TestSlice41Reconcile_Counts_CarryTheCardStackIcon(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("counts@example.com")
	b.addSet("nato-alphabet")
	page := b.mustGet("/library?q=NATO")
	i := strings.Index(page, `class="item-count"`)
	if i < 0 {
		t.Fatal("search results carry no item count")
	}
	has(t, page[i:i+700], "icon-cards", `</svg>26 items</span>`)
	b.post("/learn", "q", "The water cycle")
	w.app.WaitDrafts()
	draft := b.mustGet(b.path)
	has(t, draft, `list-count item-count"><svg class="icon icon-cards"`)
	_, done := b.post(b.path + "/approve-all")
	has(t, done, "Create something else", `Start studying<svg class="icon icon-arrow"`)
	hasNot(t, done, "Learn something else")
	if one := b.mustGet("/library?q=Alfa"); !strings.Contains(one, `</svg>1 item</span>`) {
		t.Error("one result is not counted as one item")
	}
}
