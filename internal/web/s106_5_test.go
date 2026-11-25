package web_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"scry/internal/ai"
	"scry/internal/srs"
	"scry/internal/store"
)

// [S106-5] Manage a card without leaving Study: Edit card covers the
// question, the correct answer and the idea; Snooze puts a card away until
// tomorrow in the learner's time zone; Delete removes it, restorable for
// 30 days in Settings → Recently removed. Snooze and Delete work while a
// card waits and in its review, and go straight to the next card (or the
// session's end) under one quiet line.

// between is the part of body from the first start up to the next end.
func between(t *testing.T, body, start, end string) string {
	t.Helper()
	i := strings.Index(body, start)
	if i < 0 {
		t.Fatalf("page has no %q", start)
	}
	j := strings.Index(body[i:], end)
	if j < 0 {
		t.Fatalf("page has no %q after %q", end, start)
	}
	return body[i : i+j]
}

func sessionHas(t *testing.T, lib *store.UserStore, itemID int64) bool {
	t.Helper()
	s, err := lib.ActiveSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s == nil {
		return false
	}
	for _, e := range s.Entries {
		if e.Item.ID == itemID {
			return true
		}
	}
	return false
}

// sameSchedule reports whether everything but the due time is unchanged.
func sameSchedule(a, b srs.State) bool {
	return a.Stability == b.Stability && a.Difficulty == b.Difficulty && a.Reps == b.Reps && a.Lapses == b.Lapses && a.Last.Equal(b.Last)
}

func TestS106_5_SnoozeBeforeAnswerRecordsNoReview(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("snooze@example.com")
	b.addSet("nato-alphabet")
	lib := w.lib("snooze@example.com")
	// 09:00 UTC on Nov 17 is 22:00 in Auckland: the learner's tomorrow
	// starts in two hours, at 11:00 UTC, not at UTC midnight.
	if err := lib.SetTZ(ctx, "Pacific/Auckland"); err != nil {
		t.Fatal(err)
	}
	tomorrow := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)

	page := b.mustGet("/study")
	item, ok := b.current(page)
	if !ok {
		t.Fatal("no card waits")
	}
	m := countRE.FindStringSubmatch(page)
	total, _ := strconv.Atoi(m[2])
	before := mustItem(t, lib, item.ID).Sched

	status, page := b.post("/study/snooze", "item_id", strconv.FormatInt(item.ID, 10))
	if status != http.StatusOK || b.path != "/study" {
		t.Fatalf("snooze: %d at %s", status, b.path)
	}
	has(t, page, `data-study-note>Snoozed until tomorrow.<`)
	// No review is recorded, and only the due time moved: to tomorrow.
	if hist, _ := lib.History(ctx, item.ID); len(hist) != 0 {
		t.Fatalf("snoozing a waiting card recorded %d reviews", len(hist))
	}
	after := mustItem(t, lib, item.ID).Sched
	if !sameSchedule(before, after) {
		t.Fatalf("the schedule moved: %+v -> %+v", before, after)
	}
	if before.Due.After(tomorrow) {
		t.Fatalf("a new card is due %v; the test needs it due today", before.Due)
	}
	if !after.Due.Equal(tomorrow) {
		t.Fatalf("due %v, want the start of the learner's tomorrow %v", after.Due, tomorrow)
	}
	// It left the session, which went on to the next card with no gap.
	next, ok := b.current(page)
	if !ok || next.ID == item.ID || sessionHas(t, lib, item.ID) {
		t.Fatalf("the session did not move on from the snoozed card (next %d)", next.ID)
	}
	has(t, page, fmt.Sprintf(`<span class="margin-count">1 / %d</span>`, total-1))
	// It is not due again today.
	if due, _ := lib.DueItems(ctx, tomorrow, 200); func() bool {
		for _, it := range due {
			if it.ID == item.ID {
				return true
			}
		}
		return false
	}() {
		t.Fatal("the snoozed card is still due today")
	}

	// A stale or repeated post is harmless.
	if status, _ := b.post("/study/snooze", "item_id", strconv.FormatInt(item.ID, 10)); status != http.StatusOK {
		t.Fatalf("repeated snooze: %d", status)
	}
	if hist, _ := lib.History(ctx, item.ID); len(hist) != 0 {
		t.Fatal("a repeated snooze recorded a review")
	}
	// A card that is not in the open session is snoozed all the same.
	all, _ := lib.Search(ctx, "NATO alphabet")
	var outside store.Item
	for _, it := range all {
		if !sessionHas(t, lib, it.ID) && it.ID != item.ID {
			outside = it
			break
		}
	}
	if outside.ID == 0 {
		t.Fatal("every card is in the session")
	}
	if status, _ := b.post("/study/snooze", "item_id", strconv.FormatInt(outside.ID, 10)); status != http.StatusOK {
		t.Fatalf("snoozing a card outside the session: %d", status)
	}
	if got := mustItem(t, lib, outside.ID).Sched.Due; got.Before(tomorrow) {
		t.Fatalf("a card outside the session was not snoozed: due %v", got)
	}

	// Another learner's card is not found, and nothing changes.
	m2 := w.browser()
	m2.signUp("mallory@example.com")
	m2.addSet("kitchen-numbers")
	m2.get("/study")
	for _, path := range []string{"/study/snooze", "/study/delete", "/study/restore"} {
		if status, _ := m2.post(path, "item_id", strconv.FormatInt(next.ID, 10)); status != http.StatusNotFound {
			t.Errorf("%s on another learner's card: %d, want 404", path, status)
		}
	}
	if got := mustItem(t, lib, next.ID); !got.Sched.Due.Equal(next.Sched.Due) {
		t.Fatal("another learner moved the card")
	}
	// CSRF is enforced.
	form := url.Values{"item_id": {strconv.FormatInt(next.ID, 10)}}
	req, _ := http.NewRequest("POST", w.srv.URL+"/study/snooze", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if status, _ := b.do(req); status != http.StatusForbidden {
		t.Errorf("snooze without a CSRF token: %d, want 403", status)
	}
}

func TestS106_5_SnoozeAfterAnswerDueAtLeastTomorrow(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("later@example.com")
	b.addSet("nato-alphabet")
	lib := w.lib("later@example.com")
	// 09:00 UTC is 03:00 in Chicago: tomorrow starts at 06:00 UTC on Nov 18.
	if err := lib.SetTZ(ctx, "America/Chicago"); err != nil {
		t.Fatal(err)
	}
	tomorrow := time.Date(2026, 11, 18, 6, 0, 0, 0, time.UTC)

	// Every graded answer is due a day or more later, so straight after one
	// Snooze leaves its next review where it is: already at least tomorrow.
	item, _ := b.current(b.mustGet("/study"))
	w.model.script = []ai.Judgment{{Verdict: srs.Missed, Reason: "Not it."}}
	review := b.answer(item, "apple")
	if id, ok := b.reviewed(review); !ok || id != item.ID {
		t.Fatal("no review")
	}
	has(t, review, `action="/study/snooze"`, `data-snooze`, `action="/study/delete"`, `data-delete`)
	hist, _ := lib.History(ctx, item.ID)
	graded := mustItem(t, lib, item.ID).Sched
	_, page := b.post("/study/snooze", "item_id", strconv.FormatInt(item.ID, 10))
	has(t, page, "Snoozed until tomorrow.")
	if b.path != "/study" {
		t.Fatalf("after Snooze the learner is at %s", b.path)
	}
	if after, _ := lib.History(ctx, item.ID); len(after) != 1 || after[0] != hist[0] {
		t.Fatalf("the review already recorded changed: %+v", after)
	}
	if got := mustItem(t, lib, item.ID).Sched; got != graded || got.Due.Before(tomorrow) {
		t.Fatalf("snoozed schedule %+v, want %+v (not before %v)", got, graded, tomorrow)
	}
	if next, ok := b.current(page); !ok || next.ID == item.ID {
		t.Fatal("Snooze did not open the next card")
	}

	// A review left open overnight: answered at 03:00, snoozed at 01:00 the
	// next local day. Its next review (24 hours after the answer) is sooner
	// than the learner's new tomorrow, so it moves to tomorrow's start.
	item, _ = b.current(page)
	w.model.script = []ai.Judgment{{Verdict: srs.Missed, Reason: "Not it."}}
	b.answer(item, "apple")
	graded = mustItem(t, lib, item.ID).Sched
	w.advance(22 * time.Hour) // 07:00 UTC on Nov 18 is 01:00 in Chicago
	later := time.Date(2026, 11, 19, 6, 0, 0, 0, time.UTC)
	if !graded.Due.Before(later) {
		t.Fatalf("graded due %v; the test needs it before %v", graded.Due, later)
	}
	b.post("/study/snooze", "item_id", strconv.FormatInt(item.ID, 10))
	got := mustItem(t, lib, item.ID).Sched
	if !got.Due.Equal(later) || !sameSchedule(graded, got) {
		t.Fatalf("snoozed schedule %+v, want %+v due %v", got, graded, later)
	}
	// The snoozed answers still count toward the session.
	if s, _ := lib.ActiveSession(ctx); s == nil || s.Reviewed() != 2 || s.PendingReviewID != 0 {
		t.Fatalf("session after two snoozed reviews: %+v", s)
	}
}

func TestS106_5_DeleteThenRestoreFromSettings(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("tidy@example.com")
	b.addSet("nato-alphabet")
	lib := w.lib("tidy@example.com")

	// While it waits: removed, and straight on to the next card.
	item, _ := b.current(b.mustGet("/study"))
	status, page := b.post("/study/delete", "item_id", strconv.FormatInt(item.ID, 10))
	if status != http.StatusOK || b.path != "/study" {
		t.Fatalf("delete: %d at %s", status, b.path)
	}
	note := between(t, page, "data-study-note", "</div>")
	has(t, note, "Card removed", `action="/study/restore"`, fmt.Sprintf(`name="item_id" value="%d"`, item.ID), ">Restore</button>", `name="csrf"`)
	if _, err := lib.Item(ctx, item.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("the deleted card is still in the library: %v", err)
	}
	if next, ok := b.current(page); !ok || next.ID == item.ID {
		t.Fatal("Delete did not open the next card")
	}

	// Settings → Recently removed lists it with Restore, beside removed ideas.
	settings := b.mustGet("/settings")
	section := between(t, settings, "data-recently-removed", "</section>")
	has(t, section, "Recently removed", "Removed ideas and cards are kept for 30 days.", item.Prompt, "Card · Letters A–I",
		fmt.Sprintf(`action="/settings/restore/%d"`, item.ID), `name="kind" value="card"`, "Restore")
	// Restoring from Settings puts it back in its idea and in study.
	if status, page := b.post(fmt.Sprintf("/settings/restore/%d", item.ID), "kind", "card"); status != http.StatusOK || !strings.Contains(page, "Restored to your library.") {
		t.Fatalf("restore from Settings: %d", status)
	}
	back := mustItem(t, lib, item.ID)
	if back.IdeaID != item.IdeaID || back.Sched != item.Sched {
		t.Fatalf("restored card %+v, was %+v", back, item)
	}
	if due, _ := lib.DueItems(ctx, w.now().Add(time.Hour), 200); len(due) == 0 || !func() bool {
		for _, it := range due {
			if it.ID == item.ID {
				return true
			}
		}
		return false
	}() {
		t.Fatal("the restored card is not back in study")
	}
	hasNot(t, b.mustGet("/settings"), "data-recently-removed")

	// In its review, then restored from the quiet line.
	page = b.mustGet("/study")
	cur, _ := b.current(page)
	w.model.script = []ai.Judgment{{Verdict: srs.Right, Reason: "Yes."}}
	b.answer(cur, cur.Answer)
	_, page = b.post("/study/delete", "item_id", strconv.FormatInt(cur.ID, 10))
	has(t, page, "Card removed")
	if hist, _ := lib.History(ctx, cur.ID); len(hist) != 1 {
		t.Fatal("deleting in the review lost its review")
	}
	if s, _ := lib.ActiveSession(ctx); s == nil || s.PendingReviewID != 0 {
		t.Fatal("Delete in the review did not move the session on")
	}
	_, page = b.post("/study/restore", "item_id", strconv.FormatInt(cur.ID, 10))
	has(t, page, "Card restored.")
	mustItem(t, lib, cur.ID)
	// Restoring again, or offering Restore for a card that is back, is harmless.
	if status, _ := b.post("/study/restore", "item_id", strconv.FormatInt(cur.ID, 10)); status != http.StatusOK {
		t.Fatalf("restoring twice: %d", status)
	}
	hasNot(t, b.mustGet(fmt.Sprintf("/study?note=removed&card=%d", cur.ID)), "data-restore")

	// Removed ideas are still listed, as before.
	ideas, _ := lib.Outline(ctx)
	b.post(fmt.Sprintf("/library/ideas/%d/remove", ideas[len(ideas)-1].ID))
	b.post("/study/delete", "item_id", strconv.FormatInt(cur.ID, 10))
	section = between(t, b.mustGet("/settings"), "data-recently-removed", "</section>")
	has(t, section, `data-removed="idea"`, `data-removed="card"`, ideas[len(ideas)-1].Title)

	// Kept 30 days: the sweep purges a card removed longer ago.
	w.advance(29 * 24 * time.Hour)
	if err := w.store.Sweep(ctx, w.now(), 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.ItemRemovedAt(ctx, cur.ID); err != nil {
		t.Fatalf("a card removed 29 days ago was purged: %v", err)
	}
	w.advance(2 * 24 * time.Hour)
	if err := w.store.Sweep(ctx, w.now(), 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.ItemRemovedAt(ctx, cur.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a card removed 31 days ago is still kept: %v", err)
	}
}

func TestS106_5_EditCardMovesToNewOrExistingIdea(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("mover@example.com")
	lib := w.lib("mover@example.com")
	// An older idea elsewhere with the same name as one in the set: the one
	// under the card's own topic is preferred.
	elsewhere, _ := lib.CreateIdea(ctx, 0, "Elsewhere", "", "", w.now())
	decoy, _ := lib.CreateIdea(ctx, elsewhere, "Letters J–R", "", "", w.now())
	b.addSet("nato-alphabet")

	item, _ := b.current(b.mustGet("/study"))
	w.model.script = []ai.Judgment{{Verdict: srs.Missed, Reason: "Not it."}}
	review := b.answer(item, "apple")
	form := between(t, review, "data-edit-form", "</form>")
	has(t, form, `<label class="label" for="edit-idea">Idea</label>`, `name="idea" value="Letters A–I"`, `list="edit-ideas"`,
		`<option value="Letters J–R">`, `<option value="NATO phonetic alphabet">`)
	has(t, review, `data-study-header>NATO phonetic alphabet · Letters A–I<`)
	hist, _ := lib.History(ctx, item.ID)
	sched := mustItem(t, lib, item.ID).Sched
	id := strconv.FormatInt(item.ID, 10)

	// An existing idea, named in another case.
	status, out, _ := b.postJSON("/study/edit", "item_id", id, "prompt", "NATO: A", "answer", "Alfa", "idea", "letters j–r")
	if status != 200 || out["saved"] != true || out["idea"] != "Letters J–R" || out["header"] != "NATO phonetic alphabet · Letters J–R" {
		t.Fatalf("edit to an existing idea: %d %v", status, out)
	}
	moved := mustItem(t, lib, item.ID)
	if moved.IdeaID == decoy || moved.IdeaTitle != "Letters J–R" || moved.Prompt != "NATO: A" {
		t.Fatalf("moved to %+v (decoy %d)", moved, decoy)
	}
	// A new name: a new idea, inside the same top-level idea.
	status, out, _ = b.postJSON("/study/edit", "item_id", id, "prompt", "NATO: A", "answer", "Alfa", "idea", "  First  letters ")
	if status != 200 || out["saved"] != true || out["idea"] != "First letters" || out["header"] != "NATO phonetic alphabet · First letters" {
		t.Fatalf("edit to a new idea: %d %v", status, out)
	}
	moved = mustItem(t, lib, item.ID)
	if place, _ := lib.IdeaPlace(ctx, moved.IdeaID); !place.Nested || place.Topic != "NATO phonetic alphabet" || place.Title != "First letters" {
		t.Fatalf("the new idea sits at %+v", place)
	}
	// The grade and its schedule stay; the review stays on screen.
	if after, _ := lib.History(ctx, item.ID); len(after) != 1 || after[0] != hist[0] {
		t.Fatalf("the edit changed the review: %+v", after)
	}
	if moved.Sched != sched {
		t.Fatal("the edit moved the schedule")
	}
	page := b.mustGet("/study")
	if rid, ok := b.reviewed(page); !ok || rid != item.ID {
		t.Fatal("the edit left the review")
	}
	has(t, page, `data-study-header>NATO phonetic alphabet · First letters<`, `name="idea" value="First letters"`, `data-verdict="missed"`)

	// An empty field is refused in words under it.
	status, out, _ = b.postJSON("/study/edit", "item_id", id, "prompt", "NATO: A", "answer", "Alfa", "idea", "   ")
	if status != 200 || out["saved"] != false || out["field"] != "idea" || out["error"] != "Name an idea for this card." {
		t.Fatalf("empty idea: %d %v", status, out)
	}
	status, page = b.post("/study/edit", "item_id", id, "prompt", "", "answer", "Alfa", "idea", "First letters")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("empty question by form: %d", status)
	}
	has(t, page, `data-edit-error="prompt">The question can't be empty.</p>`, `data-edit-error="idea" hidden`, `data-edit-error="answer" hidden`)
	status, page = b.post("/study/edit", "item_id", id, "prompt", "NATO: A", "answer", "Alfa", "idea", "")
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("empty idea by form: %d", status)
	}
	has(t, page, `data-edit-error="idea">Name an idea for this card.</p>`, `aria-invalid="true"`)
	if got := mustItem(t, lib, item.ID); got.IdeaTitle != "First letters" {
		t.Fatalf("a refused edit moved the card: %+v", got)
	}

	// Without JavaScript: the form posts and the review returns with "Saved"
	// and the new place.
	status, page = b.post("/study/edit", "item_id", id, "prompt", "NATO alphabet: A", "answer", "Alfa", "idea", "Letters A–I")
	if status != 200 || b.path != "/study" {
		t.Fatalf("edit by form post: %d at %s", status, b.path)
	}
	has(t, page, "data-edit open", "Saved", `data-study-header>NATO phonetic alphabet · Letters A–I<`, "This grade and its schedule stay as they are.")
	if got := mustItem(t, lib, item.ID); got.IdeaTitle != "Letters A–I" || got.Prompt != "NATO alphabet: A" {
		t.Fatalf("stored card = %+v", got)
	}
}

func TestS106_5_WaitingCardNeverShowsAnswer(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("spoiler@example.com")
	b.addSet("nato-alphabet")
	page := b.mustGet("/study")
	item, _ := b.current(page)
	actions := between(t, page, "data-manage>", "</div>")
	has(t, actions, `action="/study/snooze"`, `action="/study/delete"`, fmt.Sprintf(`name="item_id" value="%d"`, item.ID), ">Snooze</button>", ">Delete</button>")
	hasNot(t, actions, item.Answer, item.Prompt, "<svg", `class="btn`)
	hasNot(t, page, item.Answer, "data-canonical")
	// Under the answer line, after the answer form.
	if strings.Index(page, "data-manage>") < strings.Index(page, "data-answer-line") {
		t.Fatal("Snooze and Delete sit above the answer line")
	}
	// The next waiting card, under the quiet line, keeps its answer too, and
	// the line names nothing of the card it put away.
	for _, path := range []string{"/study/snooze", "/study/delete"} {
		_, page = b.post(path, "item_id", strconv.FormatInt(item.ID, 10))
		next, ok := b.current(page)
		if !ok {
			t.Fatalf("%s: no next card", path)
		}
		note := between(t, page, "data-study-note", "</div>")
		hasNot(t, note, item.Prompt, item.Answer)
		hasNot(t, page, next.Answer, "data-canonical")
		item = next
	}
}

func TestS106_5_KeysAndIcons(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("keys@example.com")
	b.addSet("nato-alphabet")
	item, _ := b.current(b.mustGet("/study"))
	w.model.script = []ai.Judgment{{Verdict: srs.Missed, Reason: "Not it."}}
	review := b.answer(item, "apple")
	foot := between(t, review, `class="review-foot"`, `data-next-form`)
	// Bordered secondary controls, each its icon and word, beside Edit card
	// and before Next card.
	has(t, foot, `aria-keyshortcuts="E">`, "icon-pencil", "Edit card",
		`class="btn manage-btn" data-snooze aria-keyshortcuts="S">`, "icon-snooze", "Snooze</button>",
		`class="btn manage-btn" data-delete>`, "icon-bin", "Delete</button>")
	has(t, review, "enter or space for next card")

	css, _ := os.ReadFile("static/app.css")
	has(t, string(css), ".manage-btn .icon { width: 16px; height: 16px; }", ".edit-card summary .icon { width: 16px; height: 16px; }")
	js, _ := os.ReadFile("static/app.js")
	src := string(js)
	keys := between(t, src, "// Keys: in the review E opens Edit card", "function initNext")
	// Never while typing, never with a modifier, never a key another binding took.
	has(t, keys, "e.defaultPrevented", "e.metaKey", "e.ctrlKey", "e.altKey", "typing(e.target)",
		`key === "e" && $("[data-review]")`, `edit.open = true`, `key === "s"`, `snooze.click()`)
	// Enter and Space for Next card and Space to listen are untouched.
	has(t, src, `if (e.key === " " && !nativeControl && next)`, `if (e.key === " " && !nativeControl && $("[data-mic]"))`)
}

// Snooze or Delete on the last card goes to the session's end as Next card
// does; with nothing reviewed, Study says what is next, with the line.
func TestS106_5_SessionEndAfterSnoozeOrDelete(t *testing.T) {
	w := newWorld(t)
	b := w.browser()
	b.signUp("end@example.com")
	lib := w.lib("end@example.com")
	idea, _ := lib.CreateIdea(ctx, 0, "Capitals", "", "", w.now())
	one, _ := lib.AddItem(ctx, store.NewItem{IdeaID: idea, Prompt: "Capital of France", Answer: "Paris"}, w.now())
	two, _ := lib.AddItem(ctx, store.NewItem{IdeaID: idea, Prompt: "Capital of Peru", Answer: "Lima"}, w.now())

	// Answer the first, then delete the second while it waits: the end.
	page := b.mustGet("/study")
	if it, _ := b.current(page); it.ID != one {
		t.Fatalf("first card %d", it.ID)
	}
	w.model.script = []ai.Judgment{{Verdict: srs.Right, Reason: "Yes."}}
	b.answerNext(store.Item{ID: one}, "Paris")
	_, page = b.post("/study/delete", "item_id", strconv.FormatInt(two, 10))
	if b.path != "/study/done" {
		t.Fatalf("after deleting the last card the learner is at %s", b.path)
	}
	has(t, page, "Done.", "1 reviewed", "Card removed", fmt.Sprintf(`name="item_id" value="%d"`, two))

	// Restored, then snoozed with nothing reviewed: Study says nothing is
	// due, with the line; no loop.
	b.post("/study/restore", "item_id", strconv.FormatInt(two, 10))
	page = b.mustGet("/study")
	if it, ok := b.current(page); !ok || it.ID != two {
		t.Fatal("the restored card is not up")
	}
	status, page := b.post("/study/snooze", "item_id", strconv.FormatInt(two, 10))
	if status != http.StatusOK || b.path != "/study" {
		t.Fatalf("snoozing the only card: %d at %s", status, b.path)
	}
	has(t, page, "Nothing due.", "Snoozed until tomorrow.")

	// Deleting every card leaves an empty library, with Restore on the line.
	w.advance(48 * time.Hour)
	b.mustGet("/study")
	b.post("/study/delete", "item_id", strconv.FormatInt(one, 10))
	b.mustGet("/study")
	_, page = b.post("/study/delete", "item_id", strconv.FormatInt(two, 10))
	has(t, page, "No cards to study yet.", "Card removed", fmt.Sprintf(`name="item_id" value="%d"`, two))
}
