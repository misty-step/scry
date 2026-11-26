package web

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"scry/internal/app"
	"scry/internal/store"
)

// Bridge cards (Slice 75, the Strugglometer): the cue beside a struggling
// card, the editable preview of drafted bridges, accepting them (which
// holds the card), and releasing a hold by hand. Every route is under the
// learner's own card (/library/items/{id}/bridges...), read through the
// scoped store, so another learner's card is a 404.

// bridgeView is what the cue and hold partial ("struggle") shows for one
// card: the cue and "Build bridge cards" for a struggling card, "Held by N
// bridges" and "Release hold" for a held one, and a note on a bridge card.
type bridgeView struct {
	Struggle app.Struggle
	ItemID   int64
	CSRF     string
	From     string // "study" or "item": where accepting and releasing return to
	Parent   *store.Item
	Bridges  []store.Item // the active bridges holding this card (item page)
	Cleared  string       // on a bridge card just answered well: what it changed
}

// bridgeFrom is where a bridge form returns to.
func bridgeFrom(r *http.Request) string {
	if r.FormValue("from") == "study" {
		return "study"
	}
	return "item"
}

func itemPath(id int64) string { return "/library/items/" + strconv.FormatInt(id, 10) }

// bridgeBack is the page a bridge action returns to.
func bridgeBack(from string, id int64, ok string) string {
	if from == "study" {
		return studyOn("")
	}
	if ok != "" {
		return itemPath(id) + "?ok=" + ok
	}
	return itemPath(id)
}

// previewCard is one editable bridge in the preview.
type previewCard struct{ Prompt, Answer string }

// maxPostedBridges bounds how many rows a preview form may carry.
const maxPostedBridges = 6

// postedBridges reads the preview's rows from the form, in order.
func postedBridges(r *http.Request) ([]previewCard, bool) {
	_ = r.ParseForm()
	ps, as := r.PostForm["prompt"], r.PostForm["answer"]
	if len(ps) != len(as) || len(ps) > maxPostedBridges {
		return nil, false
	}
	out := make([]previewCard, 0, len(ps))
	for i := range ps {
		out = append(out, previewCard{Prompt: ps[i], Answer: as[i]})
	}
	return out, true
}

func (s *Server) renderBridges(w http.ResponseWriter, r *http.Request, c *rc, status int, parent store.Item, from, key string, cards []previewCard, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	data["Title"], data["Nav"] = "Bridge cards", "library"
	data["Parent"], data["From"], data["Key"], data["Cards"] = parent, from, key, cards
	data["Back"] = bridgeBack(from, parent.ID, "")
	data["CanRemove"] = len(cards) > 1
	return s.render(w, r, c, status, "bridges.html", data)
}

// bridgesDraft asks the model for bridge cards and shows them for editing.
// Nothing is added to the library here.
func (s *Server) bridgesDraft(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	from := bridgeFrom(r)
	parent, err := c.us.Item(r.Context(), id)
	if err != nil {
		return err
	}
	cards, err := s.App.DraftBridges(r.Context(), c.user, id)
	data := map[string]any{}
	switch {
	case err == nil:
		pc := make([]previewCard, 0, len(cards))
		for _, b := range cards {
			pc = append(pc, previewCard{Prompt: b.Question, Answer: b.Answer})
		}
		return s.renderBridges(w, r, c, http.StatusOK, parent, from, app.BridgeKey(), pc, nil)
	case errors.Is(err, app.ErrNotStruggling):
		return back(w, r, bridgeBack(from, id, "nobridges"))
	case errors.Is(err, app.ErrAllowance) && c.sub.NoCard():
		data["Problem"], data["ProblemLink"], data["ProblemHref"] = "You've used the trial's allowance, so Scry can't build bridge cards now.", "Add a card", "/signup/card"
	case errors.Is(err, app.ErrAllowance):
		data["Problem"], data["ProblemLink"], data["ProblemHref"] = "You've reached this month's allowance, so Scry can't build bridge cards now.", "Top up in Billing", "/billing"
	case errors.Is(err, app.ErrBridgesUnavailable):
		data["Problem"], data["Retry"] = "Scry couldn't build bridge cards just now. Nothing was added. Try again in a moment.", true
	default:
		return err
	}
	return s.renderBridges(w, r, c, http.StatusServiceUnavailable, parent, from, "", nil, data)
}

// bridgesPreview re-shows the preview without one row: "Remove" without
// JavaScript. The rows come from the form, as the learner edited them; the
// model is not asked again.
func (s *Server) bridgesPreview(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	parent, err := c.us.Item(r.Context(), id)
	if err != nil {
		return err
	}
	from := bridgeFrom(r)
	cards, ok := postedBridges(r)
	if !ok {
		return back(w, r, bridgeBack(from, id, ""))
	}
	if i, err := strconv.Atoi(r.FormValue("remove")); err == nil && i >= 0 && i < len(cards) && len(cards) > 1 {
		cards = append(cards[:i:i], cards[i+1:]...)
	}
	return s.renderBridges(w, r, c, http.StatusOK, parent, from, r.FormValue("key"), cards, nil)
}

// bridgesAccept adds the learner's bridge cards and holds the parent card,
// atomically and at most once per preview.
func (s *Server) bridgesAccept(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	parent, err := c.us.Item(r.Context(), id)
	if err != nil {
		return err
	}
	from := bridgeFrom(r)
	cards, ok := postedBridges(r)
	if !ok {
		return s.renderBridges(w, r, c, http.StatusUnprocessableEntity, parent, from, r.FormValue("key"), nil,
			map[string]any{"Error": "Keep at most three bridge cards."})
	}
	in := make([]store.BridgeCard, 0, len(cards))
	for _, pc := range cards {
		in = append(in, store.BridgeCard{Prompt: pc.Prompt, Answer: pc.Answer})
	}
	_, err = s.App.AcceptBridges(r.Context(), c.user, id, r.FormValue("key"), in)
	var fe *app.FieldError
	switch {
	case err == nil:
		return back(w, r, bridgeBack(from, id, "held"))
	case errors.As(err, &fe):
		return s.renderBridges(w, r, c, http.StatusUnprocessableEntity, parent, from, r.FormValue("key"), cards, map[string]any{"Error": fe.Msg})
	case errors.Is(err, app.ErrNotStruggling):
		return back(w, r, bridgeBack(from, id, "nobridges"))
	}
	return err
}

// bridgesRelease releases a held card by hand.
func (s *Server) bridgesRelease(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if _, err := s.App.ReleaseHold(r.Context(), c.user, id); err != nil {
		return err
	}
	return back(w, r, bridgeBack(bridgeFrom(r), id, "released"))
}

// bridgeViewFor builds the cue/hold view of one card for a page: the
// card's page (from "item") or its post-answer review (from "study"). An
// active bridge names the card it leads up to, read through the learner's
// scoped store, so only the learner's own card can ever be named; it shows
// that card's question and a link, never its answer.
func bridgeViewFor(ctx context.Context, c *rc, it store.Item, st app.Struggle, from string) bridgeView {
	v := bridgeView{Struggle: st, ItemID: it.ID, CSRF: c.sess.CSRF, From: from}
	if st.BridgeFor != 0 {
		if p, err := c.us.Item(ctx, st.BridgeFor); err == nil {
			v.Parent = &p
		}
	}
	if st.HeldBy > 0 && from == "item" {
		v.Bridges, _ = c.us.Bridges(ctx, it.ID)
	}
	return v
}

// clearedLine says, on the review of a bridge card answered Correct or
// Almost Correct, what clearing it did to the card it leads to.
func clearedLine(ctx context.Context, c *rc, rv store.Review) string {
	if rv.Practice || (rv.Verdict != "right" && rv.Verdict != "close") {
		return ""
	}
	parent, reason, at, err := c.us.BridgeEnd(ctx, rv.ItemID)
	if err != nil || parent == 0 || reason != "mastered" || !at.Equal(rv.At) {
		return ""
	}
	bs, err := c.us.BridgeState(ctx, parent)
	if err != nil {
		return ""
	}
	if bs.HeldBy == 0 {
		return "Bridge cleared · the card it leads to is back in your reviews, on its own schedule."
	}
	return "Bridge cleared · " + plural(bs.HeldBy, "bridge still holds", "bridges still hold") + " the card it leads to."
}
