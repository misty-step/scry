package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"scry/internal/app"
	"scry/internal/exact"
	"scry/internal/speech"
	"scry/internal/srs"
	"scry/internal/starter"
	"scry/internal/store"
)

func readLimited(r *http.Request, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("body too large")
	}
	return b, nil
}

type shelfTile struct {
	Set   starter.Set
	Count int
	Added bool
}

func (s *Server) shelf(w http.ResponseWriter, r *http.Request, c *rc) error {
	added, err := c.us.StarterAdded(r.Context())
	if err != nil {
		return err
	}
	items, _, err := c.us.Counts(r.Context(), s.App.Now())
	if err != nil {
		return err
	}
	var tiles []shelfTile
	for _, set := range starter.All() {
		tiles = append(tiles, shelfTile{Set: set, Count: set.Count(), Added: added[set.Slug]})
	}
	return s.render(w, r, c, http.StatusOK, "shelf.html", map[string]any{
		"Title": "Starter shelf", "Nav": "library", "Tiles": tiles, "HasItems": items > 0, "Just": r.URL.Query().Get("added"),
	})
}

func (s *Server) shelfAdd(w http.ResponseWriter, r *http.Request, c *rc) error {
	set, ok := starter.Get(r.FormValue("slug"))
	if !ok {
		return store.ErrNotFound
	}
	if _, _, err := c.us.AddStarterSet(r.Context(), set, s.App.Now()); err != nil {
		return err
	}
	return back(w, r, "/shelf?added="+set.Slug+"#"+set.Slug)
}

// answeredRow is one receded line of the scrollback.
type answeredRow struct {
	Prompt  string
	Answer  string
	Verdict srs.Verdict
	Why     string // shown only on the most recent row
}

// judgedView is the judgment on screen.
type judgedView struct {
	Kind        string // close, missed, exact
	Verdict     srs.Verdict
	Why         string // the fixed criterion sentence (or the exact diff count)
	Answer      string
	Expected    string
	Ops         []exact.Op
	ReviewID    int64
	Explainable bool   // close or missed, model-judged: offer "Why?" on demand
	Explanation string // filled in only after the learner asked (no-JS path)
}

func exactLabel(it store.Item) string {
	if !it.Exact {
		return ""
	}
	label := "Exact text"
	if it.TextTitle != "" {
		label += " · " + it.TextTitle
	}
	if it.LineNo > 0 {
		label += " · line " + strconv.Itoa(it.LineNo)
	}
	return label
}

func (s *Server) studyData(ctx context.Context, c *rc, sess *store.StudySession) map[string]any {
	data := map[string]any{"Title": "Study", "Nav": "study", "Session": true}
	var rows []answeredRow
	var shown *store.SessionEntry
	for i := range sess.Entries {
		e := &sess.Entries[i]
		if e.Review == nil {
			continue
		}
		if e.Review.ID == sess.PendingReviewID {
			shown = e
			continue
		}
		rows = append(rows, answeredRow{Prompt: e.Item.Prompt, Answer: e.Review.Answer, Verdict: e.Review.Verdict, Why: e.Review.Reason})
	}
	var pendingReview *store.Review
	if sess.PendingReviewID != 0 {
		if shown != nil {
			pendingReview = shown.Review
		} else if rv, err := c.us.Review(ctx, sess.PendingReviewID); err == nil {
			// A practice attempt: the entry keeps its first review.
			pendingReview = &rv
			for i := range sess.Entries {
				if sess.Entries[i].Item.ID == rv.ItemID {
					shown = &sess.Entries[i]
				}
			}
		}
	}
	cur := sess.Current()
	if pendingReview != nil && shown != nil {
		d := app.ParseDetail(*pendingReview)
		j := judgedView{Verdict: pendingReview.Verdict, Why: pendingReview.Reason, Answer: pendingReview.Answer, Expected: shown.Item.Answer, ReviewID: pendingReview.ID}
		switch {
		case shown.Item.Exact && len(d.Ops) > 0:
			j.Kind, j.Ops = "exact", d.Ops
		case pendingReview.Verdict == srs.Missed:
			j.Kind = "missed"
		default:
			j.Kind = "close"
		}
		j.Explainable = j.Kind != "exact" && pendingReview.Explainable()
		data["Judged"] = j
		cur = shown
	}
	if n := len(rows); n > 0 {
		for i := range rows[:n-1] {
			rows[i].Why = ""
		}
		if pendingReview != nil || rows[n-1].Verdict != srs.Right {
			rows[n-1].Why = ""
		}
		if n > 6 {
			rows = rows[n-6:]
		}
	}
	data["Answered"] = rows
	if cur != nil {
		data["Item"] = cur.Item
		data["Position"] = cur.Position
		data["ExactLabel"] = exactLabel(cur.Item)
		data["Retry"] = sess.RetryItemID == cur.Item.ID
		var up []string
		for _, e := range sess.Entries {
			if e.Review == nil && e.Position > cur.Position && len(up) < 3 {
				up = append(up, e.Item.Prompt)
			}
		}
		data["Upcoming"] = up
	}
	data["Total"] = len(sess.Entries)
	return data
}

func (s *Server) study(w http.ResponseWriter, r *http.Request, c *rc) error {
	sess, err := s.App.Study(r.Context(), c.user)
	if err != nil {
		return err
	}
	if sess == nil {
		items, _, err := c.us.Counts(r.Context(), s.App.Now())
		if err != nil {
			return err
		}
		if items == 0 {
			return back(w, r, "/shelf")
		}
		return s.renderRest(w, r, c)
	}
	return s.render(w, r, c, http.StatusOK, "study.html", s.studyData(r.Context(), c, sess))
}

// renderRest is the study page when nothing is due.
func (s *Server) renderRest(w http.ResponseWriter, r *http.Request, c *rc) error {
	line := "Nothing else is due today."
	if next, err := c.us.NextDue(r.Context()); err == nil && !next.IsZero() {
		line = "Your next review is " + strings.TrimPrefix(dueLabel(c.user, store.Item{Sched: srs.State{Due: next}}, s.App.Now()), "due ") + "."
	}
	return s.render(w, r, c, http.StatusOK, "done.html", map[string]any{"Title": "Nothing due", "Nav": "study", "Head": "Nothing due.", "Line": line})
}

func (s *Server) afterStudy(w http.ResponseWriter, r *http.Request, c *rc) error {
	sess, err := c.us.ActiveSession(r.Context())
	if err != nil {
		return err
	}
	if sess == nil {
		return back(w, r, "/study/done")
	}
	return back(w, r, "/study")
}

func (s *Server) studyAnswer(w http.ResponseWriter, r *http.Request, c *rc) error {
	itemID, _ := strconv.ParseInt(r.FormValue("item_id"), 10, 64)
	answer := r.FormValue("answer")
	_, err := s.App.Answer(r.Context(), c.user, itemID, answer, r.FormValue("via"))
	if err == nil || errors.Is(err, app.ErrStale) {
		return s.afterStudy(w, r, c)
	}
	status, msg, link := http.StatusServiceUnavailable, "", ""
	switch {
	case errors.Is(err, app.ErrJudgeUnavailable):
		msg = "Couldn't check that just now. Your answer is still here; press Enter to try again."
	case errors.Is(err, app.ErrAllowance):
		status = http.StatusPaymentRequired
		msg = "You've reached this month's allowance, so Scry can't judge this answer yet. It resets on " +
			c.sub.PeriodEnd.In(c.user.Location()).Format("January 2") + "."
		link = "Top up in Billing"
	default:
		return err
	}
	sess, serr := c.us.ActiveSession(r.Context())
	if serr != nil || sess == nil {
		return back(w, r, "/study")
	}
	data := s.studyData(r.Context(), c, sess)
	data["Message"], data["MessageLink"], data["Draft"] = msg, link, answer
	return s.render(w, r, c, status, "study.html", data)
}

func (s *Server) studyNext(w http.ResponseWriter, r *http.Request, c *rc) error {
	if err := s.App.Next(r.Context(), c.user, r.FormValue("again") == "1"); err != nil {
		return err
	}
	return s.afterStudy(w, r, c)
}

func (s *Server) studyDone(w http.ResponseWriter, r *http.Request, c *rc) error {
	sess, err := c.us.LastEndedSession(r.Context())
	if err != nil {
		return err
	}
	if sess == nil || sess.Reviewed() == 0 {
		return back(w, r, "/study")
	}
	sum := s.App.Summarize(c.user, sess)
	line := strconv.Itoa(sum.Reviewed) + " reviewed"
	if sum.Tomorrow > 0 {
		line += ", " + strconv.Itoa(sum.Tomorrow) + " to revisit tomorrow"
	}
	return s.render(w, r, c, http.StatusOK, "done.html", map[string]any{"Title": "Done", "Nav": "study", "Head": "Done.", "Line": line + "."})
}

// studyExplain is the on-demand "Why?" on a close or missed answer. It
// runs after the verdict and never changes it. With JavaScript it answers
// JSON for the inline result; without, it re-renders the judgment with the
// explanation in place. Failures fall back to the criterion sentence.
func (s *Server) studyExplain(w http.ResponseWriter, r *http.Request, c *rc) error {
	wantsJSON := r.Header.Get("X-Requested-With") == "fetch" || strings.Contains(r.Header.Get("Accept"), "application/json")
	id, _ := strconv.ParseInt(r.FormValue("review_id"), 10, 64)
	ex, err := s.App.Explain(r.Context(), c.user, id)
	if err != nil {
		if !wantsJSON {
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, app.ErrNotExplainable) {
				return back(w, r, "/study")
			}
			return err
		}
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "That answer isn't here to explain."})
		case errors.Is(err, app.ErrNotExplainable):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Only close and missed answers have an explanation."})
		default:
			s.Log.ErrorContext(r.Context(), "explain failed", "err", err, "user", c.user.ID)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Couldn't explain that just now."})
		}
		return nil
	}
	if wantsJSON {
		writeJSON(w, http.StatusOK, map[string]any{"text": ex.Text, "fallback": ex.Fallback})
		return nil
	}
	sess, err := c.us.ActiveSession(r.Context())
	if err != nil {
		return err
	}
	if sess == nil {
		return back(w, r, "/study")
	}
	data := s.studyData(r.Context(), c, sess)
	if j, ok := data["Judged"].(judgedView); ok && j.ReviewID == id {
		j.Explanation = ex.Text
		data["Judged"] = j
		data["ExplainFallback"] = ex.Fallback
	}
	return s.render(w, r, c, http.StatusOK, "study.html", data)
}

// studySpeech hears a recording of a spoken answer and returns its words.
func (s *Server) studySpeech(w http.ResponseWriter, r *http.Request, c *rc) error {
	data, err := readLimited(r, 6<<20)
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "That recording is too long. Try a shorter answer, or type it."})
		return nil
	}
	text, err := s.App.Transcribe(r.Context(), c.user, speech.Audio{Data: data, MIME: r.Header.Get("Content-Type")})
	switch {
	case errors.Is(err, speech.ErrTooLong):
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "That recording is too long. Try a shorter answer, or type it."})
	case errors.Is(err, speech.ErrAudio):
		// Not a recording Scry's recorder makes; nothing to hear in it.
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Scry couldn't read that recording. Type your answer instead."})

	case errors.Is(err, app.ErrAllowance):
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "You've reached this month's allowance, so Scry can't listen right now. Type your answer, or top up in Billing."})
	case errors.Is(err, app.ErrSpeechUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Scry can't listen just now. Type your answer instead."})
	case err != nil:
		return err
	default:
		// Empty text means nothing was caught: short, empty or silent
		// recordings, or a provider that heard no words. The page says
		// DESIGN's "Didn't catch that. Try again or type it."
		writeJSON(w, http.StatusOK, map[string]any{"text": text, "caught": text != ""})
	}
	return nil
}
