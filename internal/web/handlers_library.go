package web

import (
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"scry/internal/app"
	"scry/internal/store"
)

type itemRow struct {
	Item   store.Item
	Due    string
	Prompt template.HTML
	Answer template.HTML
}

func (s *Server) itemRows(c *rc, items []store.Item, words []string) []itemRow {
	rows := make([]itemRow, 0, len(items))
	for _, it := range items {
		rows = append(rows, itemRow{Item: it, Due: dueLabel(c.user, it, s.App.Now()),
			Prompt: markParts(it.Prompt, words), Answer: markParts(it.Answer, words)})
	}
	return rows
}

var libraryFlash = map[string]string{
	"removed":  "Removed. You can restore it in Settings for 30 days.",
	"saved":    "Saved.",
	"added":    "Added.",
	"approved": "Approved. It's in your library.",
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
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" && r.PathValue("id") == "" {
		items, err := c.us.Search(ctx, q)
		if err != nil {
			return err
		}
		data["Query"], data["Results"] = q, s.itemRows(c, items, strings.Fields(q))
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
	data["Title"], data["Idea"], data["Related"], data["Items"] = idea.Title, idea, related, s.itemRows(c, items, nil)
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

func (s *Server) itemAdd(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	prompt, answer := strings.TrimSpace(r.FormValue("prompt")), strings.TrimSpace(r.FormValue("answer"))
	if prompt == "" || answer == "" || len(prompt) > 1000 || len(answer) > 2000 {
		return back(w, r, "/library/ideas/"+strconv.FormatInt(id, 10)+"?ok=needboth#add")
	}
	idea, err := c.us.Idea(r.Context(), id)
	if err != nil {
		return err
	}
	n := store.NewItem{IdeaID: id, Prompt: prompt, Answer: answer, Exact: r.FormValue("exact") == "1"}
	if n.Exact {
		n.TextTitle = idea.Title
	}
	if _, err := c.us.AddItem(r.Context(), n, s.App.Now()); err != nil {
		return err
	}
	return back(w, r, "/library/ideas/"+strconv.FormatInt(id, 10)+"?ok=added")
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
	return s.render(w, r, c, http.StatusOK, "item.html", map[string]any{
		"Title": "Item", "Nav": "library", "Item": it, "Due": dueLabel(c.user, it, s.App.Now()), "History": history,
		"Flash": libraryFlash[r.URL.Query().Get("ok")], "Error": map[string]string{"needboth": "An item needs both a question and an answer."}[r.URL.Query().Get("err")],
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

func (s *Server) textData(r *http.Request, c *rc) (map[string]any, error) {
	outline, err := c.us.Outline(r.Context())
	if err != nil {
		return nil, err
	}
	var tops []store.Idea
	for _, o := range outline {
		if o.Depth == 0 {
			tops = append(tops, o)
		}
	}
	return map[string]any{"Title": "Add an exact text", "Nav": "library", "Parents": tops}, nil
}

func (s *Server) textForm(w http.ResponseWriter, r *http.Request, c *rc) error {
	data, err := s.textData(r, c)
	if err != nil {
		return err
	}
	return s.render(w, r, c, http.StatusOK, "text.html", data)
}

func (s *Server) textSave(w http.ResponseWriter, r *http.Request, c *rc) error {
	title, text := strings.TrimSpace(r.FormValue("title")), r.FormValue("text")
	parent, _ := strconv.ParseInt(r.FormValue("parent"), 10, 64)
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	msg := ""
	switch {
	case title == "" || len(title) > 120:
		msg = "Give the text a name, such as “Sonnet 18”."
	case len(lines) == 0:
		msg = "Paste the text, one line per line."
	case len(lines) > 200 || len(text) > 20000:
		msg = "That text is too long for one piece. Add it in parts of at most 200 lines."
	}
	if msg == "" {
		id, err := c.us.AddExactText(r.Context(), parent, title, lines, s.App.Now())
		if err == nil {
			return back(w, r, "/library/ideas/"+strconv.FormatInt(id, 10)+"?ok=added")
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		msg = "That idea isn't in your library. Choose another."
	}
	data, err := s.textData(r, c)
	if err != nil {
		return err
	}
	data["Error"], data["TextTitle"], data["Text"], data["Parent"] = msg, title, text, parent
	return s.render(w, r, c, http.StatusUnprocessableEntity, "text.html", data)
}

// ideaGroup is one idea of a draft with the items waiting under it.
type ideaGroup struct {
	Title   string
	Summary string
	Items   []store.Suggestion
}

func (s *Server) inbox(w http.ResponseWriter, r *http.Request, c *rc) error {
	list, err := c.us.Suggestions(r.Context(), 0)
	if err != nil {
		return err
	}
	flash := ""
	if n, err := strconv.Atoi(r.URL.Query().Get("approved")); err == nil && n > 0 {
		flash = "All approved. " + plural(n, "item", "items") + " joined your library."
	}
	return s.render(w, r, c, http.StatusOK, "inbox.html", map[string]any{"Title": "Inbox", "Nav": "inbox", "Suggestions": list, "Flash": flash})
}

func (s *Server) inboxApproveAll(w http.ResponseWriter, r *http.Request, c *rc) error {
	n, err := c.us.ApproveAll(r.Context(), 0, s.App.Now())
	if err != nil {
		return err
	}
	return back(w, r, "/inbox?approved="+strconv.Itoa(n))
}

func suggestionHome(sg store.Suggestion) string {
	if sg.DraftID != 0 {
		return "/learn/" + strconv.FormatInt(sg.DraftID, 10)
	}
	return "/inbox"
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
	if sub.AllowanceMicros-sub.UsedMicros+sub.TopupMicros-reserved < s.App.DraftBudget() {
		hint = "Not enough allowance left for a draft. Top up or check Billing."
		if sub.NoCard() {
			hint = "Not enough of the trial's allowance left for a draft. Add a card in Billing to keep going."
		}
	}

	return s.render(w, r, c, http.StatusOK, "learn.html", map[string]any{"Title": "Learn something new", "Nav": "learn", "Q": r.URL.Query().Get("q"), "Error": hint})
}

func (s *Server) learnAsk(w http.ResponseWriter, r *http.Request, c *rc) error {
	q := r.FormValue("q")
	id, err := s.App.Learn(r.Context(), c.user, q)
	var fe *app.FieldError
	if errors.As(err, &fe) {
		return s.render(w, r, c, http.StatusUnprocessableEntity, "learn.html", map[string]any{"Title": "Learn something new", "Nav": "learn", "Q": q, "Error": fe.Msg})
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
	if d.Kind == "book" {
		kind = "book · core ideas"
	}
	return s.render(w, r, c, http.StatusOK, "draft.html", map[string]any{
		"Title": d.Title, "Nav": "learn", "Draft": d, "Kind": kind, "Groups": groups, "Pending": pending, "Approved": approved,
		"Ideas": len(groups), "Reset": c.sub.PeriodEnd.In(c.user.Location()), "NoCard": c.sub.NoCard(), "Retry": url.QueryEscape(d.Query),
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
