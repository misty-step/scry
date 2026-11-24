package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"scry/internal/ai"
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

// shelfSummary is what the shelf's "Start studying" bar says: how many
// starter sets are in the library and how many cards they hold.
type shelfSummary struct {
	Sets, Cards int
}

func (s shelfSummary) String() string {
	if s.Sets == 0 {
		return "Your library has cards"
	}
	return plural(s.Sets, "set", "sets") + " added · " + plural(s.Cards, "card", "cards")
}

func shelfTiles(added map[string]bool) ([]shelfTile, shelfSummary) {
	var tiles []shelfTile
	var sum shelfSummary
	for _, set := range starter.All() {
		t := shelfTile{Set: set, Count: set.Count(), Added: added[set.Slug]}
		if t.Added {
			sum.Sets++
			sum.Cards += t.Count
		}
		tiles = append(tiles, t)
	}
	return tiles, sum
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
	tiles, sum := shelfTiles(added)
	return s.render(w, r, c, http.StatusOK, "shelf.html", map[string]any{
		"Title": "Starter shelf", "Nav": "library", "Tiles": tiles, "HasItems": items > 0, "Summary": sum.String(),
		"Just": r.URL.Query().Get("added"),
	})
}

// wantsJSON reports whether the page's own script asked for a JSON answer
// to update in place, instead of the redirect a plain form post gets.
func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

// shelfAdd adds a starter set. A plain form post redirects back to the
// tile; the shelf's script asks for JSON and confirms in place, so the
// page neither reloads nor scrolls.
func (s *Server) shelfAdd(w http.ResponseWriter, r *http.Request, c *rc) error {
	set, ok := starter.Get(r.FormValue("slug"))
	if !ok {
		if wantsJSON(r) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "That starter set isn't on the shelf."})
			return nil
		}
		return store.ErrNotFound
	}
	if _, _, err := c.us.AddStarterSet(r.Context(), set, s.App.Now()); err != nil {
		return err
	}
	if wantsJSON(r) {
		added, err := c.us.StarterAdded(r.Context())
		if err != nil {
			return err
		}
		_, sum := shelfTiles(added)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slug": set.Slug, "cards": set.Count(),
			"sets": sum.Sets, "total_cards": sum.Cards, "summary": sum.String(), "study": "/study"})
		return nil
	}
	return back(w, r, "/shelf?added="+set.Slug+"#"+set.Slug)
}

// answeredRow is one receded line of the scrollback.
type answeredRow struct {
	Prompt  string
	Answer  string
	Verdict srs.Verdict
}

// judgedView is the post-answer review on screen.
type judgedView struct {
	Kind        string // right, close, missed, exact
	Verdict     srs.Verdict
	Badge       string // Correct, Almost Correct, Incorrect
	Icon        string // the verdict's icon
	Why         string // the fixed criterion sentence (or the exact diff count)
	Answer      string // what the learner submitted
	Expected    string // the card's correct answer, as it stands now
	Ops         []exact.Op
	ReviewID    int64
	Practice    bool   // an "Again" attempt: it did not move the schedule
	Explainable bool   // model-judged: offer "Ask Why?" on demand
	Explanation string // filled in only after the learner asked (no-JS path)
	Skipped     bool   // nothing was submitted: "No answer given."
}

// verdictBadge is the verdict in words, with its icon.
func verdictBadge(v srs.Verdict) (label, icon string) {
	switch v {
	case srs.Right:
		return "Correct", "verdict-right"
	case srs.Close:
		return "Almost Correct", "verdict-close"
	}
	return "Incorrect", "verdict-missed"
}

// telemetryView is the card's schedule and history as the review shows
// them. Every value is read from the stored schedule (srs.State on the
// item) and the review history; nothing here is computed for effect.
//
// One basis (DESIGN.md, Post-answer review): the event ("Next review") and
// the gap ("Interval") are both measured on the same clock from the stored
// instants, so "today" can never sit beside "1 day". The event says how
// long from now; the interval says how long after the graded answer.
type telemetryView struct {
	NextDate   string // "Tue, Nov 24", in the learner's time zone
	NextRel    string // "in 1 day", "in 5 hours", "due now"
	NextISO    string // RFC 3339 instant, for <time datetime>
	Interval   string // "1 day": the gap from the last graded review to the next
	After      string // "after this answer": what the interval is measured from
	Stability  string // "3.7 days": FSRS stability
	Difficulty string // "5.2 / 10": FSRS difficulty
	Reviews    string // "3 · 2 correct · 0 almost · 1 incorrect"
	Trail      []trailMark
	Total      int
	Lapses     int
	Practice   int // practice attempts, which never move the schedule
}

// trailMark is one recent verdict in the "Reviews" fact: a record of what
// happened, oldest first, never a streak count.
type trailMark struct {
	Verdict srs.Verdict
	Label   string // Correct, Almost Correct, Incorrect
	Icon    string
}

// trailLen is how many recent verdicts the review shows.
const trailLen = 8

func daysLabel(n int) string { return plural(n, "day", "days") }

// spanLabel says how long a stretch of time is, on the schedule's one
// clock. The span is rounded to the nearest hour first: under half an hour
// it reads in minutes, from 1 to 21 rounded hours in hours, and from 22
// rounded hours (21 h 30 min) up in whole days, rounded.
func spanLabel(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch h := int(math.Round(d.Hours())); {
	case h < 1:
		return plural(max(int(math.Ceil(d.Minutes())), 1), "minute", "minutes")
	case h < 22:
		return plural(h, "hour", "hours")
	}
	return daysLabel(max(int(math.Round(d.Hours()/24)), 1))
}

// untilLabel is how long from now the next review is.
func untilLabel(due, now time.Time) string {
	if !due.After(now) {
		return "due now"
	}
	return "in " + spanLabel(due.Sub(now))
}

func telemetry(u store.User, it store.Item, counts store.ReviewCounts, history []store.Review, practice bool, now time.Time) telemetryView {
	due := it.Sched.Due.In(u.Location())
	t := telemetryView{NextRel: untilLabel(it.Sched.Due, now), NextISO: due.Format(time.RFC3339), NextDate: due.Format("Mon, Jan 2"),
		Total: counts.Total, Lapses: it.Sched.Lapses, Practice: counts.Practice}
	if due.Year() != now.In(u.Location()).Year() {
		t.NextDate = due.Format("Mon, Jan 2, 2006")
	}
	if !it.Sched.Last.IsZero() && it.Sched.Due.After(it.Sched.Last) {
		t.Interval, t.After = spanLabel(it.Sched.Due.Sub(it.Sched.Last)), "after this answer"
		if practice {
			t.After = "after the last graded answer"
		}
	}
	if it.Sched.Reps > 0 {
		t.Stability = strconv.FormatFloat(it.Sched.Stability, 'f', 1, 64) + " days"
		t.Difficulty = strconv.FormatFloat(it.Sched.Difficulty, 'f', 1, 64) + " / 10"
	}
	t.Reviews = fmt.Sprintf("%d · %d correct · %d almost · %d incorrect", counts.Total, counts.Right, counts.Close, counts.Missed)
	// History is newest first; the trail reads oldest first, graded
	// answers only (a practice attempt moves nothing).
	for _, r := range history {
		if r.Practice || !r.Verdict.Valid() {
			continue
		}
		label, icon := verdictBadge(r.Verdict)
		t.Trail = append([]trailMark{{Verdict: r.Verdict, Label: label, Icon: icon}}, t.Trail...)
		if len(t.Trail) == trailLen {
			break
		}
	}
	return t
}

// exactLabel names the exact text a card is a line of. While the card
// waits, check the entire label (including the retry suffix) and drop
// any candidate that quotes the answer.
func exactLabel(it store.Item, answered, retry bool) string {
	if !it.Exact {
		return ""
	}
	label := neutralExactTitle
	if it.TextTitle != "" {
		label += " · " + it.TextTitle
	}
	var line, numberOnly string
	if it.LineNo > 0 {
		number := strconv.Itoa(it.LineNo)
		line = " · line " + number
		numberOnly = "Line " + number
	}
	if answered {
		return label + line
	}
	suffix := ""
	if retry {
		suffix = " · again"
	}
	for _, candidate := range []string{label + line, neutralExactTitle + line, numberOnly} {
		if candidate != "" && !quotesAnswer(candidate+suffix, it.Answer) {
			return candidate + suffix
		}
	}
	return ""
}

// studyPurpose is Study's one-line purpose, shown in the session rail.
const studyPurpose = "Recall what's due today"

// studyHeader names where the current card sits. While the card waits for
// its answer the label is spoiler-safe (see spoiler.go): never the idea's
// own title, and never a topic sharing a word with the answer to this card
// or to a card beside it. Once the answer is in, it is the real place.
func (s *Server) studyHeader(ctx context.Context, c *rc, it store.Item, answered bool) string {
	answers := []string{it.Answer}
	place, err := c.us.IdeaPlace(ctx, it.IdeaID)
	if err != nil {
		// The idea is gone (removed mid-session): name no topic at all.
		return spoilerSafeHeader(store.IdeaPlace{}, answers)
	}
	if answered {
		return revealedHeader(place)
	}
	siblings, err := c.us.Items(ctx, it.IdeaID)
	if err != nil {
		// Without the sibling answers the topic cannot be cleared.
		return spoilerSafeHeader(store.IdeaPlace{Nested: place.Nested, Ordinal: place.Ordinal}, answers)
	}
	for _, sib := range siblings {
		answers = append(answers, sib.Answer)
	}
	return spoilerSafeHeader(place, answers)
}

func (s *Server) studyData(ctx context.Context, c *rc, sess *store.StudySession) map[string]any {
	data := map[string]any{"Title": "Study", "Nav": "study", "Session": true, "Purpose": studyPurpose}
	var answered []*store.SessionEntry
	var shown *store.SessionEntry
	remaining := 0
	for i := range sess.Entries {
		e := &sess.Entries[i]
		if e.Review == nil {
			remaining++
			continue
		}
		if e.Review.ID == sess.PendingReviewID {
			shown = e
			continue
		}
		answered = append(answered, e)
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
	judged := pendingReview != nil && shown != nil
	if judged {
		d := app.ParseDetail(*pendingReview)
		j := judgedView{Verdict: pendingReview.Verdict, Answer: pendingReview.Answer,
			Why:      ai.ShownWhy(pendingReview.Verdict, pendingReview.Reason, pendingReview.Answer),
			Skipped:  strings.TrimSpace(pendingReview.Answer) == "",
			Expected: shown.Item.Answer, ReviewID: pendingReview.ID, Practice: pendingReview.Practice}
		j.Badge, j.Icon = verdictBadge(pendingReview.Verdict)
		switch {
		case shown.Item.Exact && len(d.Ops) > 0:
			j.Kind, j.Ops = "exact", d.Ops
		case pendingReview.Verdict == srs.Missed:
			j.Kind = "missed"
		case pendingReview.Verdict == srs.Close:
			j.Kind = "close"
		default:
			j.Kind = "right"
		}
		j.Explainable = j.Kind != "exact" && pendingReview.Explainable()
		data["Judged"] = j
		cur = shown
		// The schedule this answer left the card with, and its history.
		counts, err := c.us.ReviewCounts(ctx, shown.Item.ID)
		if err == nil {
			history, _ := c.us.History(ctx, shown.Item.ID)
			data["Telemetry"] = telemetry(c.user, shown.Item, counts, history, pendingReview.Practice, s.App.Now())
			// The Strugglometer (Slice 75): a card missed again and again is
			// offered bridge cards here, after its answer is in (never while
			// it waits); a held or bridge card says so.
			if bs, err := c.us.BridgeState(ctx, shown.Item.ID); err == nil {
				if it, err := c.us.Item(ctx, shown.Item.ID); err == nil {
					// An active bridge's review names the card it leads up
					// to, as the bridge's own page does (DESIGN.md, Held).
					bv := bridgeViewFor(ctx, c, it, app.StruggleOf(it, history, bs), "study")
					bv.Cleared = clearedLine(ctx, c, *pendingReview)
					data["Bridge"] = bv
				}
			}
		}
		data["Last"] = remaining == 0
		data["EditPrompt"], data["EditAnswer"] = shown.Item.Prompt, shown.Item.Answer
	}
	// While a card waits, no cue on the screen may quote its answer: not
	// in the scrollback above it, and not in the "coming up" list below.
	// Such a cue (the next line of an exact text quotes the line before
	// it) is held back until the answer is in; a cue that gives the answer
	// away only by its text's title is shown under a neutral title.
	cue := func(it store.Item) (string, bool) {
		if judged || cur == nil {
			return it.Prompt, true
		}
		return cueWhile(it, cur.Item.Answer)
	}
	var rows []answeredRow
	for _, e := range answered {
		if prompt, ok := cue(e.Item); ok {
			rows = append(rows, answeredRow{Prompt: prompt, Answer: e.Review.Answer, Verdict: e.Review.Verdict})
		}
	}
	if n := len(rows); n > 6 {
		rows = rows[n-6:]
	}
	// The post-answer review is the current card only: no earlier card
	// above it and no next card below it (DESIGN.md, Post-answer review).
	if judged {
		rows = nil
	}
	data["Answered"] = rows
	if cur != nil {
		data["Item"] = cur.Item
		data["Position"] = cur.Position
		data["ExactLabel"] = exactLabel(cur.Item, judged, sess.RetryItemID == cur.Item.ID)
		// The prompt as the waiting card shows it; the review shows the
		// card's own prompt.
		data["Prompt"] = cur.Item.Prompt
		if !judged {
			data["Prompt"] = waitingPrompt(cur.Item)
		}
		data["Header"] = s.studyHeader(ctx, c, cur.Item, judged)
		var up []string
		for _, e := range sess.Entries {
			if judged {
				break
			}
			if e.Review == nil && e.Position > cur.Position && len(up) < 3 {
				if prompt, ok := cue(e.Item); ok {
					up = append(up, prompt)
				}
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
			// An empty library is said on Study itself, with the two ways
			// forward; never a silent redirect to another page.
			return s.render(w, r, c, http.StatusOK, "study_empty.html", map[string]any{"Title": "Study", "Nav": "study", "Purpose": studyPurpose})
		}
		return s.renderRest(w, r, c)
	}
	data := s.studyData(r.Context(), c, sess)
	if _, judged := data["Judged"]; judged && r.URL.Query().Get("saved") == "1" {
		data["Saved"], data["EditOpen"] = true, true
	}
	return s.render(w, r, c, http.StatusOK, "study.html", data)
}

// studyEdit saves an edit made in the post-answer review: the card's
// question and correct answer. The card must be the learner's own (the
// scoped store answers 404 for anyone else's), neither field may be empty,
// and nothing but the card's text changes: not the grade on screen, not
// the schedule, not the session. The review's script asks for JSON and
// updates the panel in place; a plain form post returns to the review.
func (s *Server) studyEdit(w http.ResponseWriter, r *http.Request, c *rc) error {
	id, _ := strconv.ParseInt(r.FormValue("item_id"), 10, 64)
	prompt, answer := r.FormValue("prompt"), r.FormValue("answer")
	it, err := s.App.EditCard(r.Context(), c.user, id, prompt, answer)
	var fe *app.FieldError
	switch {
	case errors.As(err, &fe):
		if wantsJSON(r) {
			// An expected outcome the script shows in words, so it is a
			// normal answer (a 4xx would log an error in the console).
			writeJSON(w, http.StatusOK, map[string]any{"saved": false, "error": fe.Msg, "field": fe.Field})
			return nil
		}
		sess, serr := c.us.ActiveSession(r.Context())
		if serr != nil || sess == nil {
			return back(w, r, "/study")
		}
		data := s.studyData(r.Context(), c, sess)
		if _, judged := data["Judged"]; !judged {
			return back(w, r, "/study")
		}
		data["EditOpen"], data["EditError"], data["EditPrompt"], data["EditAnswer"] = true, fe.Msg, prompt, answer
		return s.render(w, r, c, http.StatusUnprocessableEntity, "study.html", data)
	case errors.Is(err, store.ErrNotFound):
		if wantsJSON(r) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "That card isn't in your library."})
			return nil
		}
		return err
	case err != nil:
		return err
	}
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "saved": true, "item_id": it.ID, "prompt": it.Prompt, "answer": it.Answer})
		return nil
	}
	return back(w, r, "/study?saved=1")
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
	status, msg, link, href := http.StatusServiceUnavailable, "", "", "/billing"
	switch {
	case errors.Is(err, app.ErrJudgeUnavailable):
		msg = "Couldn't check that just now. Your answer is still here; press Enter to try again."
	case errors.Is(err, app.ErrAllowance) && c.sub.NoCard():
		status = http.StatusPaymentRequired
		msg = "You've used the trial's allowance, so Scry can't judge this answer yet. Add a card to keep going."
		link, href = "Add a card", "/signup/card"
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
	data["Message"], data["MessageLink"], data["MessageHref"], data["Draft"] = msg, link, href, answer
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

// studyExplain is the on-demand "Ask Why?" on a model-judged answer. It
// runs after the verdict and never changes it. With JavaScript it answers
// JSON for the inline result; without, it re-renders the judgment with the
// explanation in place. Failures fall back to the criterion sentence.
func (s *Server) studyExplain(w http.ResponseWriter, r *http.Request, c *rc) error {
	asJSON := r.Header.Get("X-Requested-With") == "fetch" || wantsJSON(r)
	id, _ := strconv.ParseInt(r.FormValue("review_id"), 10, 64)
	ex, err := s.App.Explain(r.Context(), c.user, id)
	if err != nil {
		if !asJSON {
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, app.ErrNotExplainable) {
				return back(w, r, "/study")
			}
			return err
		}
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "That answer isn't here to explain."})
		case errors.Is(err, app.ErrNotExplainable):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "An exact text has no explanation: the word-by-word comparison shows what differed."})
		default:
			s.Log.ErrorContext(r.Context(), "explain failed", "err", err, "user", c.user.ID)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Couldn't explain that just now."})
		}
		return nil
	}
	if asJSON {
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

	case errors.Is(err, app.ErrAllowance) && c.sub.NoCard():
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "You've used the trial's allowance, so Scry can't listen right now. Type your answer, or add a card in Billing to keep going."})
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
