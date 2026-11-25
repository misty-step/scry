package web

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"scry/internal/ai"
	"scry/internal/app"
	"scry/internal/store"
)

type itemRow struct {
	Item     store.Item
	Due      string
	Prompt   template.HTML
	Answer   template.HTML
	Struggle app.Struggle // the Strugglometer's cue, hold or bridge note
}

func (s *Server) itemRows(ctx context.Context, c *rc, items []store.Item, words []string) []itemRow {
	// A deck list without its cues is still a deck list: a failure to read
	// them is logged and the rows are shown plain.
	st, err := s.App.Struggles(ctx, c.user, items)
	if err != nil {
		s.Log.ErrorContext(ctx, "struggle cues unavailable", "err", err, "user", c.user.ID)
	}
	rows := make([]itemRow, 0, len(items))
	for _, it := range items {
		row := itemRow{Item: it, Due: dueLabel(c.user, it, s.App.Now()),
			Prompt: markParts(it.Prompt, words), Answer: markParts(it.Answer, words), Struggle: st[it.ID]}
		if row.Struggle.HeldBy > 0 {
			row.Due = "held"
		}
		rows = append(rows, row)
	}
	return rows
}

var libraryFlash = map[string]string{
	"removed":  "Removed. You can restore it in Settings for 30 days.",
	"saved":    "Saved.",
	"approved": "Approved. It's in your library.",
	// Slice 75: bridge cards.
	"held":      "Bridges added. This card is held until each bridge is answered Correct or Almost Correct once.",
	"released":  "Hold released. The card is due again on its own schedule; its bridges stay in your library.",
	"nobridges": "This card isn't offered bridge cards right now.",
}

func (s *Server) library(w http.ResponseWriter, r *http.Request, c *rc) error {
	ctx := r.Context()
	outline, err := c.us.Outline(ctx)
	if err != nil {
		return err
	}
	if len(outline) == 0 {
		return back(w, r, "/shelf")
	}
	data := map[string]any{"Title": "Library", "Nav": "library", "Outline": outline, "Flash": libraryFlash[r.URL.Query().Get("ok")]}
	// After an import, Library opens on the imported idea with a plain
	// count (S106-3): "Imported 42 cards."
	if n, err := strconv.Atoi(r.URL.Query().Get("imported")); err == nil && n > 0 {
		data["Flash"] = "Imported " + plural(n, "card", "cards") + "."
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" && r.PathValue("id") == "" {
		items, err := c.us.Search(ctx, q)
		if err != nil {
			return err
		}
		data["Query"], data["Results"] = q, s.itemRows(ctx, c, items, strings.Fields(q))
		data["Title"] = "Search: " + q
		return s.render(w, r, c, http.StatusOK, "library.html", data)
	}
	id := outline[0].ID
	if r.PathValue("id") != "" {
		if id, err = pathID(r); err != nil {
			return err
		}
	}
	idea, err := c.us.Idea(ctx, id)
	if err != nil {
		return err
	}
	for _, o := range outline {
		if o.ID == id {
			idea.Number, idea.Depth = o.Number, o.Depth
		}
	}
	related, err := c.us.Related(ctx, id)
	if err != nil {
		return err
	}
	if len(related) > 8 {
		related = related[:8]
	}
	items, err := c.us.Items(ctx, id)
	if err != nil {
		return err
	}
	data["Title"], data["Idea"], data["Related"], data["Items"] = idea.Title, idea, related, s.itemRows(ctx, c, items, nil)
	data["OpenParent"] = idea.ParentID
	return s.render(w, r, c, http.StatusOK, "library.html", data)
}

func (s *Server) ideaRemove(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := c.us.RemoveIdea(r.Context(), id, s.App.Now()); err != nil {
		return err
	}
	return back(w, r, "/library?ok=removed")
}

func (s *Server) itemPage(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	it, err := c.us.Item(r.Context(), id)
	if err != nil {
		return err
	}
	history, err := c.us.History(r.Context(), id)
	if err != nil {
		return err
	}
	bs, err := c.us.BridgeState(r.Context(), id)
	if err != nil {
		return err
	}
	st := app.StruggleOf(it, history, bs)
	due := dueLabel(c.user, it, s.App.Now())
	if st.HeldBy > 0 {
		due = "held · " + due + " on its own schedule"
	}
	return s.render(w, r, c, http.StatusOK, "item.html", map[string]any{
		"Title": "Item", "Nav": "library", "Item": it, "Due": due, "History": history,
		"Bridge": bridgeViewFor(r.Context(), c, it, st, "item"),
		"Flash":  libraryFlash[r.URL.Query().Get("ok")], "Error": map[string]string{"needboth": "An item needs both a question and an answer."}[r.URL.Query().Get("err")],
	})
}

func (s *Server) itemSave(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if _, err := c.us.Item(r.Context(), id); err != nil {
		return err
	}
	prompt, answer := strings.TrimSpace(r.FormValue("prompt")), strings.TrimSpace(r.FormValue("answer"))
	if prompt == "" || answer == "" || len(prompt) > 1000 || len(answer) > 2000 {
		return back(w, r, "/library/items/"+strconv.FormatInt(id, 10)+"?err=needboth")
	}
	if err := c.us.UpdateItem(r.Context(), id, prompt, answer, r.FormValue("exact") == "1"); err != nil {
		return err
	}
	return back(w, r, "/library/items/"+strconv.FormatInt(id, 10)+"?ok=saved")
}

func (s *Server) itemRemove(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	it, err := c.us.Item(r.Context(), id)
	if err != nil {
		return err
	}
	if err := c.us.RemoveItem(r.Context(), id, s.App.Now()); err != nil {
		return err
	}
	return back(w, r, "/library/ideas/"+strconv.FormatInt(it.IdeaID, 10)+"?ok=removed")
}

// ideaGroup is one idea of a draft with the items waiting under it.
type ideaGroup struct {
	Title   string
	Summary string
	Items   []store.Suggestion
}

// suggestionHome is where deciding a suggestion returns: its draft, or
// the library for an old inbox suggestion (there is no Inbox, S106-4).
func suggestionHome(sg store.Suggestion) string {
	if sg.DraftID != 0 {
		return "/learn/" + strconv.FormatInt(sg.DraftID, 10)
	}
	return "/library"
}

func (s *Server) suggestionApprove(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	sg, err := c.us.Suggestion(r.Context(), id)
	if err != nil {
		return err
	}
	if _, err := c.us.Approve(r.Context(), id, s.App.Now()); err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return back(w, r, suggestionHome(sg))
}

func (s *Server) suggestionDismiss(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	sg, err := c.us.Suggestion(r.Context(), id)
	if err != nil {
		return err
	}
	if err := c.us.Dismiss(r.Context(), id, s.App.Now()); err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return back(w, r, suggestionHome(sg))
}

func (s *Server) learn(w http.ResponseWriter, r *http.Request, c *rc) error {
	sub, err := s.App.Plan(r.Context(), c.user.ID)
	if err != nil {
		return err
	}
	reserved, err := c.us.ReservedMicros(r.Context())
	if err != nil {
		return err
	}
	hint := ""
	if app.Unreserved(sub, reserved) < s.App.DraftBudget() {
		hint = "Not enough allowance left for a draft. Top up or check Billing."
		if sub.NoCard() {
			hint = "Not enough of the trial's allowance left for a draft. Add a card in Billing to keep going."
		}
	}

	q := r.URL.Query().Get("q")
	// "Ask again" and "Try a narrower topic" on a draft reopen Create with
	// that draft's request (pasted material can be 16,000 characters, too
	// long to carry in a link), so it can be edited and sent again.
	if from, err := strconv.ParseInt(r.URL.Query().Get("draft"), 10, 64); err == nil && from > 0 {
		if d, err := c.us.Draft(r.Context(), from); err == nil {
			q = d.Query
		}
	}
	return s.render(w, r, c, http.StatusOK, "learn.html", map[string]any{"Title": "Create", "Nav": "learn", "Q": q, "Error": hint, "Examples": learnExamples,
		"ImportError": importRefusals[r.URL.Query().Get("import")]})
}

// learnExamples are Create's concrete starting points (DESIGN.md, Create):
// a book's title, a mental model and a technical topic. Tapping one fills
// the field; without script each is a link that opens Create with it filled.
var learnExamples = []string{
	"Meditations by Marcus Aurelius",
	"Second-order thinking",
	"How HTTPS keeps a connection private",
}

func (s *Server) learnAsk(w http.ResponseWriter, r *http.Request, c *rc) error {
	q := r.FormValue("q")
	id, err := s.App.Learn(r.Context(), c.user, q)
	var fe *app.FieldError
	if errors.As(err, &fe) {
		return s.render(w, r, c, http.StatusUnprocessableEntity, "learn.html", map[string]any{"Title": "Create", "Nav": "learn", "Q": q, "Error": fe.Msg, "Examples": learnExamples})
	}
	if err != nil {
		return err
	}
	return back(w, r, "/learn/"+strconv.FormatInt(id, 10))
}

func (s *Server) draft(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	d, err := c.us.Draft(r.Context(), id)
	if err != nil {
		return err
	}
	list, err := c.us.Suggestions(r.Context(), id)
	if err != nil {
		return err
	}
	pending, approved, _, err := c.us.SuggestionCounts(r.Context(), id)
	if err != nil {
		return err
	}
	var groups []ideaGroup
	for _, sg := range list {
		if n := len(groups); n == 0 || groups[n-1].Title != sg.IdeaTitle {
			groups = append(groups, ideaGroup{Title: sg.IdeaTitle, Summary: sg.IdeaSummary})
		}
		groups[len(groups)-1].Items = append(groups[len(groups)-1].Items, sg)
	}
	kind := "topic"
	switch {
	case d.Kind == "book":
		kind = "book · core ideas"
	case ai.IsMaterial(d.Query):
		kind = "from your notes"
	}
	if d.Title == "" {
		// Named before the model answers: never the whole pasted text.
		d.Title = app.DraftTitle(d.Query)
	}
	return s.render(w, r, c, http.StatusOK, "draft.html", map[string]any{
		"Title": d.Title, "Nav": "learn", "Draft": d, "Kind": kind, "Groups": groups, "Pending": pending, "Approved": approved,
		"Flash": map[string]string{"import": "Sent to drafting: that Markdown has no card blocks, so Scry's drafting agent is writing its cards. Approve them here."}[r.URL.Query().Get("from")],
		"Ideas": len(groups), "Reset": c.sub.PeriodEnd.In(c.user.Location()), "NoCard": c.sub.NoCard(),
		"Retry": "/learn?draft=" + strconv.FormatInt(d.ID, 10),
	})
}

func (s *Server) draftApproveAll(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if _, err := c.us.Draft(r.Context(), id); err != nil {
		return err
	}
	if _, err := c.us.ApproveAll(r.Context(), id, s.App.Now()); err != nil {
		return err
	}
	return back(w, r, "/learn/"+strconv.FormatInt(id, 10))
}
