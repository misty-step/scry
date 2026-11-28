package web

import (
	"context"
	"net/url"
	"strings"
	"unicode/utf8"

	"scry/internal/srs"
	"scry/internal/store"
)

// Connected learning ([S139-2]): the quiet "Related & resources"
// disclosure. It is drawn after a card is graded (never while it waits),
// on a card's own page, and on an idea's page (the idea's own resources
// only). When there is nothing in it, nothing at all is drawn.

// connectedView is one "Related & resources" disclosure.
type connectedView struct {
	Counts    string // "· 3 cards · 2 resources"; a part with none is left out
	Related   []relatedRow
	Resources []resourceRow
}

type relatedRow struct {
	ID      int64
	Preview string // the question, up to 140 characters, cut at a word
	Full    string // the whole question: the link's accessible name
	Idea    string
	Due     string // "due today", "in 6 days"
}

type resourceRow struct {
	Kind  string // EXPLAINER, ARTICLE, VIDEO, DIAGRAM or LINK
	Title string
	URL   string // an absolute http(s) URL, or empty
	Host  string // the link's host ("en.wikipedia.org")
	Body  string // plain text, never rendered as HTML or Markdown
}

// relatedPreviewChars bounds a related card's question in the list.
const relatedPreviewChars = 140

// previewAtWord cuts s to at most n characters at a word, with "…".
func previewAtWord(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)[:n-1]
	cut := string(r)
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.") + "…"
}

func resourceRows(rs []store.Resource) []resourceRow {
	out := make([]resourceRow, 0, len(rs))
	for _, r := range rs {
		row := resourceRow{Kind: strings.ToUpper(r.Kind), Title: r.Title, Body: r.Body}
		if u, err := url.Parse(r.URL); err == nil && r.URL != "" && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			row.URL, row.Host = r.URL, u.Hostname()
		}
		out = append(out, row)
	}
	return out
}

func newConnected(related []relatedRow, resources []resourceRow) *connectedView {
	if len(related) == 0 && len(resources) == 0 {
		return nil
	}
	v := &connectedView{Related: related, Resources: resources}
	if n := len(related); n > 0 {
		v.Counts += " · " + plural(n, "card", "cards")
	}
	if n := len(resources); n > 0 {
		v.Counts += " · " + plural(n, "resource", "resources")
	}
	return v
}

// cardConnected is a card's disclosure: its related cards, then its own
// resources and its idea's. Nil when it has neither, or when they cannot
// be read (the review is still whole without them).
func (s *Server) cardConnected(ctx context.Context, c *rc, it store.Item) *connectedView {
	cards, err := c.us.RelatedCards(ctx, it.ID)
	if err != nil {
		s.Log.ErrorContext(ctx, "related cards failed", "err", err, "item", it.ID)
		return nil
	}
	res, err := c.us.CardResources(ctx, it.ID)
	if err != nil {
		s.Log.ErrorContext(ctx, "card resources failed", "err", err, "item", it.ID)
		return nil
	}
	now := s.App.Now()
	rows := make([]relatedRow, 0, len(cards))
	for _, rc := range cards {
		full := strings.TrimSpace(rc.Prompt)
		rows = append(rows, relatedRow{ID: rc.ID, Preview: previewAtWord(full, relatedPreviewChars), Full: full, Idea: rc.IdeaTitle,
			Due: dueLabel(c.user, store.Item{Sched: srs.State{Due: rc.Due}}, now)})
	}
	return newConnected(rows, resourceRows(res))
}

// ideaConnected is an idea page's disclosure: the idea's own resources
// (its cards are listed under Items).
func (s *Server) ideaConnected(ctx context.Context, c *rc, ideaID int64) *connectedView {
	res, err := c.us.IdeaResources(ctx, ideaID)
	if err != nil {
		s.Log.ErrorContext(ctx, "idea resources failed", "err", err, "idea", ideaID)
		return nil
	}
	return newConnected(nil, resourceRows(res))
}
