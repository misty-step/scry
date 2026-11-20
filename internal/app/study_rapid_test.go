package app_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"

	"scry/internal/ai"
	"scry/internal/alert"
	"scry/internal/app"
	"scry/internal/billing"
	"scry/internal/srs"
	"scry/internal/store"
)

// scriptedModel judges with whatever verdict the test chose last. fail
// makes it fail before using anything (no usage, no model name).
type scriptedModel struct {
	mu      sync.Mutex
	verdict srs.Verdict
	fail    bool
	calls   int
	// gate, when set, is waited on by every Judge call before it answers.
	gate    chan struct{}
	arrived chan struct{}
}

func (m *scriptedModel) Name() string { return "scripted" }

func (m *scriptedModel) Judge(ctx context.Context, in ai.JudgeInput) (ai.Judgment, ai.Usage, error) {
	m.mu.Lock()
	m.calls++
	v, fail, gate, arrived := m.verdict, m.fail, m.gate, m.arrived
	m.mu.Unlock()
	if arrived != nil {
		arrived <- struct{}{}
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
		}
	}
	if fail {
		return ai.Judgment{}, ai.Usage{}, errors.New("the model is down")
	}
	return ai.Judgment{Verdict: v, Reason: "scripted reason for " + string(v)},
		ai.Usage{Model: "test-judge", InputTokens: 10, OutputTokens: 5}, nil
}

func (m *scriptedModel) Draft(context.Context, ai.DraftInput, ai.DraftSink) (ai.DraftResult, ai.Usage, error) {
	return ai.DraftResult{}, ai.Usage{}, errors.New("not drafting in study tests")
}

func (m *scriptedModel) set(v srs.Verdict) { m.mu.Lock(); m.verdict = v; m.mu.Unlock() }

// echoExplainer explains by echoing exactly what it was given, so a test
// can see which question and expected answer the explanation was about.
type echoExplainer struct{}

func (echoExplainer) Name() string { return "echo" }

func explanationOf(question, expected, answer string, v srs.Verdict) string {
	return fmt.Sprintf("why %s: q=%s e=%s a=%s", v, question, expected, answer)
}

func (echoExplainer) Explain(_ context.Context, in ai.ExplainInput) (string, ai.Usage, error) {
	return explanationOf(in.Question, in.Expected, in.Answer, in.Verdict), ai.Usage{Model: "test-judge", InputTokens: 3, OutputTokens: 3}, nil
}

type studyWorld struct {
	ctx   context.Context
	st    *store.Store
	app   *app.App
	model *scriptedModel
	user  store.User
	us    *store.UserStore
	mu    sync.Mutex
	clock time.Time
}

func (w *studyWorld) now() time.Time { w.mu.Lock(); defer w.mu.Unlock(); return w.clock }

func (w *studyWorld) advance(d time.Duration) { w.mu.Lock(); w.clock = w.clock.Add(d); w.mu.Unlock() }

type fataler interface {
	Helper()
	Fatal(...any)
}

// newStudyWorld is a real SQLite-backed Scry with an active plan.
func newStudyWorld(t fataler, path string) *studyWorld {
	t.Helper()
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	w := &studyWorld{ctx: context.Background(), st: st, model: &scriptedModel{verdict: srs.Right},
		clock: time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)}
	cfg := app.DefaultConfig()
	cfg.TestMode = true
	w.app = &app.App{Store: st, Pay: billing.Fake{}, Model: w.model, Explainer: echoExplainer{}, Alert: &alert.Recorder{},
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: w.now, Cfg: cfg}
	if w.user, err = st.CreateUser(w.ctx, "learner@example.com", "learner", "UTC", w.clock); err != nil {
		t.Fatal(err)
	}
	w.us = st.For(w.user.ID)
	if err := w.us.SaveSubscription(w.ctx, store.Subscription{UserID: w.user.ID, CustomerRef: "cus_t", SubscriptionRef: "sub_t",
		PaymentRef: "tok_test_ok_Visa_4242", Status: "active", PeriodStart: w.clock, PeriodEnd: w.clock.AddDate(50, 0, 0),
		AllowanceMicros: 1 << 50}); err != nil {
		t.Fatal(err)
	}
	return w
}

func (w *studyWorld) addItems(t fataler, exact ...bool) []int64 {
	t.Helper()
	idea, err := w.us.CreateIdea(w.ctx, 0, "Idea", "", "", w.now())
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i, ex := range exact {
		id, err := w.us.AddItem(w.ctx, store.NewItem{IdeaID: idea, Prompt: fmt.Sprintf("question %d", i),
			Answer: fmt.Sprintf("answer number %d", i), Exact: ex}, w.now())
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

// reviewRow is a review exactly as stored.
type reviewRow struct {
	ID, ItemID, SessionID, At            int64
	Answer, Verdict, Reason, Detail, Via string
	Judge                                string
	Practice                             int
	GradedPrompt, GradedExpected         string
}

func (w *studyWorld) reviews(t fataler) map[int64]reviewRow {
	t.Helper()
	rows, err := w.st.DB().Query(`SELECT id, item_id, session_id, at, answer, verdict, reason, detail, via, judge, practice,
		graded_prompt, graded_expected FROM reviews`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int64]reviewRow{}
	for rows.Next() {
		var r reviewRow
		if err := rows.Scan(&r.ID, &r.ItemID, &r.SessionID, &r.At, &r.Answer, &r.Verdict, &r.Reason, &r.Detail, &r.Via, &r.Judge,
			&r.Practice, &r.GradedPrompt, &r.GradedExpected); err != nil {
			t.Fatal(err)
		}
		out[r.ID] = r
	}
	return out
}

// schedule reads an item's FSRS state, removed or not.
func (w *studyWorld) schedule(t fataler, id int64) srs.State {
	t.Helper()
	var s srs.State
	var last, due int64
	if err := w.st.DB().QueryRow(`SELECT stability, difficulty, reps, lapses, last_review, due_at FROM items WHERE id = ?`, id).
		Scan(&s.Stability, &s.Difficulty, &s.Reps, &s.Lapses, &last, &due); err != nil {
		t.Fatal(err)
	}
	if last != 0 {
		s.Last = time.Unix(last, 0).UTC()
	}
	if due != 0 {
		s.Due = time.Unix(due, 0).UTC()
	}
	return s
}

func (w *studyWorld) count(t fataler, q string, args ...any) int {
	t.Helper()
	var n int
	if err := w.st.DB().QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// studyMachine is the rapid state machine over the real store and app.
// Its model is what the formal spec (specs/study_session.qnt) says must
// hold: the session phase, per-item FSRS history, and every review row
// exactly as first written.
type studyMachine struct {
	w       *studyWorld
	items   []int64
	exact   map[int64]bool
	text    map[int64][2]string // the item's current prompt and answer
	removed map[int64]bool
	sched   map[int64]srs.State // last observed schedule
	reviews map[int64]reviewRow // every review as first written
	graded  map[int64][2]string // the prompt and answer the review was judged against
	edits   int
}

func (m *studyMachine) session(t *rapid.T) *store.StudySession {
	s, err := m.w.us.ActiveSession(m.w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (m *studyMachine) Study(t *rapid.T) {
	if _, err := m.w.app.Study(m.w.ctx, m.w.user); err != nil {
		t.Fatalf("Study: %v", err)
	}
}

// learn records the reviews an action just wrote.
func (m *studyMachine) learn(t *rapid.T) []reviewRow {
	var fresh []reviewRow
	for id, r := range m.w.reviews(t) {
		if _, ok := m.reviews[id]; !ok {
			m.reviews[id] = r
			fresh = append(fresh, r)
		}
	}
	return fresh
}

func (m *studyMachine) Answer(t *rapid.T) {
	before := m.session(t)
	if before == nil {
		t.Skip("no session")
	}
	phase, cur := before.Phase(), before.Current()
	if cur == nil || (phase != store.PhaseAsking && phase != store.PhaseRetryingExact) {
		t.Skip("nothing to answer")
	}
	item := cur.Item
	v := rapid.SampledFrom(srs.Verdicts).Draw(t, "verdict")
	m.w.model.set(v)
	answer := "something else entirely"
	if rapid.Bool().Draw(t, "rightWords") {
		answer = item.Answer
	}
	sched := m.w.schedule(t, item.ID)
	scoredBefore := cur.Review
	calls := m.w.model.calls
	r, err := m.w.app.Answer(m.w.ctx, m.w.user, item.ID, answer, "typed")
	fresh := m.learn(t)
	after := m.session(t)
	if phase == store.PhaseRetryingExact && (!m.exact[item.ID] || m.removed[item.ID]) {
		// The retried text is no longer exact (or is gone): the retry is
		// dropped, nothing is judged and nothing is recorded.
		if !errors.Is(err, app.ErrStale) || len(fresh) != 0 || m.w.model.calls != calls {
			t.Fatalf("answer to a retry that is no longer exact: err=%v, %d reviews, judged %v", err, len(fresh), m.w.model.calls != calls)
		}
		if after != nil && after.Phase() == store.PhaseRetryingExact {
			t.Fatal("the retry of a no-longer-exact item was kept")
		}
		return
	}
	if err != nil {
		t.Fatalf("Answer in %s: %v", phase, err)
	}
	if len(fresh) != 1 || fresh[0].ID != r.ID {
		t.Fatalf("Answer wrote %d reviews", len(fresh))
	}
	m.graded[r.ID] = [2]string{item.Prompt, item.Answer}
	got := m.w.schedule(t, item.ID)
	if phase == store.PhaseRetryingExact {
		if !r.Practice || fresh[0].Practice != 1 || fresh[0].Judge != "exact" {
			t.Fatalf("a retry was not recorded as exact practice: %+v", fresh[0])
		}
		if got != sched {
			t.Fatalf("practice moved the schedule: %+v -> %+v", sched, got)
		}
		for _, e := range after.Entries {
			if e.Item.ID == item.ID && (e.Review == nil || scoredBefore == nil || e.Review.ID != scoredBefore.ID) {
				t.Fatal("practice replaced the scored review of the session entry")
			}
		}
		if after.Phase() != store.PhasePendingReview || after.PendingReviewID != r.ID {
			t.Fatalf("practice judgment not on screen: %s", after.Phase())
		}
		return
	}
	if r.Practice || got.Reps != sched.Reps+1 {
		t.Fatalf("scored answer: practice=%v reps %d -> %d", r.Practice, sched.Reps, got.Reps)
	}
	want, err := srs.NextChecked(sched, r.Verdict, m.w.now())
	if err != nil || want.Reps != got.Reps || want.Lapses != got.Lapses || !want.Due.Truncate(time.Second).Equal(got.Due) {
		t.Fatalf("schedule %+v, want %+v (%v)", got, want, err)
	}
}

// StaleAnswer submits for an item that is not the one on screen, or while
// a judgment waits: it must be refused and change nothing.
func (m *studyMachine) StaleAnswer(t *rapid.T) {
	s := m.session(t)
	item := rapid.SampledFrom(m.items).Draw(t, "item")
	if s != nil && (s.Phase() == store.PhaseAsking || s.Phase() == store.PhaseRetryingExact) {
		if cur := s.Current(); cur != nil && cur.Item.ID == item {
			t.Skip("that is the current item")
		}
	}
	calls := m.w.model.calls
	if _, err := m.w.app.Answer(m.w.ctx, m.w.user, item, "late", "typed"); !errors.Is(err, app.ErrStale) {
		t.Fatalf("stale answer accepted: %v", err)
	}
	if fresh := m.learn(t); len(fresh) != 0 || m.w.model.calls != calls {
		t.Fatal("a stale answer was judged or recorded")
	}
}

func (m *studyMachine) Next(t *rapid.T) {
	before := m.session(t)
	again := rapid.Bool().Draw(t, "again")
	if err := m.w.app.Next(m.w.ctx, m.w.user, again); err != nil {
		t.Fatalf("Next(again=%v): %v", again, err)
	}
	after := m.session(t)
	if before == nil {
		if after != nil {
			t.Fatal("Next opened a session")
		}
		return
	}
	switch before.Phase() {
	case store.PhasePendingReview:
		pend := m.reviews[before.PendingReviewID]
		if again && pend.Judge == "exact" && m.exact[pend.ItemID] && !m.removed[pend.ItemID] {
			if after == nil || after.Phase() != store.PhaseRetryingExact || after.RetryItemID != pend.ItemID {
				t.Fatalf("Again on an exact text did not retry it")
			}
			return
		}
		if after != nil && after.Phase() != store.PhaseAsking {
			t.Fatalf("Next from a judgment left the session %s", after.Phase())
		}
	case store.PhaseRetryingExact:
		if again && (after == nil || after.RetryItemID != before.RetryItemID) {
			t.Fatal("Again while retrying changed the retry")
		}
		if !again && after != nil && after.Phase() != store.PhaseAsking {
			t.Fatal("skipping a retry did not return to asking")
		}
	case store.PhaseAsking:
		if after != nil && (after.ID != before.ID || after.Phase() != store.PhaseAsking) {
			t.Fatal("Next with nothing on screen changed the session")
		}
	}
}

// Edit changes an item's words, and sometimes whether it is exact,
// in the middle of a session.
func (m *studyMachine) Edit(t *rapid.T) {
	id := rapid.SampledFrom(m.items).Draw(t, "item")
	if m.removed[id] {
		t.Skip("removed")
	}
	m.edits++
	ex := m.exact[id]
	if rapid.Bool().Draw(t, "toggleExact") {
		ex = !ex
	}
	prompt, answer := fmt.Sprintf("edited question %d", m.edits), fmt.Sprintf("edited answer %d", m.edits)
	if err := m.w.us.UpdateItem(m.w.ctx, id, prompt, answer, ex); err != nil {
		t.Fatal(err)
	}
	m.exact[id], m.text[id] = ex, [2]string{prompt, answer}
}

func (m *studyMachine) Remove(t *rapid.T) {
	id := rapid.SampledFrom(m.items).Draw(t, "item")
	if m.removed[id] || len(m.removed)+1 >= len(m.items) {
		t.Skip("keep at least one item")
	}
	if err := m.w.us.RemoveItem(m.w.ctx, id, m.w.now()); err != nil {
		t.Fatal(err)
	}
	m.removed[id] = true
}

// Explain asks "Why?" about the judgment on screen or any earlier review.
func (m *studyMachine) Explain(t *rapid.T) {
	if len(m.reviews) == 0 {
		t.Skip("nothing reviewed")
	}
	var ids []int64
	for id := range m.reviews {
		ids = append(ids, id)
	}
	id := rapid.SampledFrom(sortedIDs(ids)).Draw(t, "review")
	if s := m.session(t); s != nil && s.PendingReviewID != 0 && rapid.Bool().Draw(t, "onScreen") {
		id = s.PendingReviewID
	}
	r := m.reviews[id]
	before := m.w.schedule(t, r.ItemID)
	ex, err := m.w.app.Explain(m.w.ctx, m.w.user, id)
	explainable := r.Judge != "exact" && (r.Verdict == string(srs.Close) || r.Verdict == string(srs.Missed))
	switch {
	case !explainable:
		if !errors.Is(err, app.ErrNotExplainable) {
			t.Fatalf("Explain(%s by %s) = %v", r.Verdict, r.Judge, err)
		}
	case err != nil:
		t.Fatalf("Explain: %v", err)
	default:
		g := m.graded[id]
		if want := explanationOf(g[0], g[1], r.Answer, srs.Verdict(r.Verdict)); ex.Fallback || ex.Text != want {
			t.Fatalf("explanation %q (fallback %v), want the graded snapshot %q", ex.Text, ex.Fallback, want)
		}
	}
	if m.w.reviews(t)[id] != r || m.w.schedule(t, r.ItemID) != before {
		t.Fatal("Explain changed the review or its item")
	}
}

func (m *studyMachine) EndSession(t *rapid.T) {
	s := m.session(t)
	if s == nil {
		t.Skip("no session")
	}
	if err := m.w.us.EndSession(m.w.ctx, s.ID, m.w.now()); err != nil {
		t.Fatal(err)
	}
	if err := m.w.us.EndSession(m.w.ctx, s.ID, m.w.now()); !errors.Is(err, store.ErrIllegalTransition) {
		t.Fatalf("ending an ended session: %v", err)
	}
	if err := m.w.us.Advance(m.w.ctx, s.ID, 0); !errors.Is(err, store.ErrIllegalTransition) {
		t.Fatalf("advancing an ended session: %v", err)
	}
}

func (m *studyMachine) Wait(t *rapid.T) {
	m.w.advance(time.Duration(rapid.IntRange(1, 96).Draw(t, "hours")) * time.Hour)
}

func (m *studyMachine) Check(t *rapid.T) {
	m.learn(t)
	s := m.session(t)
	if s != nil {
		inSession := map[int64]bool{}
		for _, e := range s.Entries {
			inSession[e.Item.ID] = true
			if e.Review != nil && (e.Review.Practice || e.Review.ItemID != e.Item.ID || e.Review.SessionID != s.ID) {
				t.Fatalf("entry %d points at review %+v", e.Item.ID, *e.Review)
			}
		}
		switch s.Phase() {
		case store.PhasePendingReview:
			r, ok := m.reviews[s.PendingReviewID]
			if !ok || r.SessionID != s.ID || !inSession[r.ItemID] || (r.Practice == 1 && r.Judge != "exact") {
				t.Fatalf("pending review %d is not a judgment of this session: %+v", s.PendingReviewID, r)
			}
		case store.PhaseRetryingExact:
			if !inSession[s.RetryItemID] || m.w.count(t, `SELECT COUNT(*) FROM reviews WHERE session_id = ? AND item_id = ? AND judge = 'exact'`,
				s.ID, s.RetryItemID) == 0 {
				t.Fatalf("retrying item %d that was never judged as an exact text here", s.RetryItemID)
			}
		}
		if s.PendingReviewID != 0 && s.RetryItemID != 0 {
			t.Fatal("a judgment and a retry at once")
		}
	}
	if n := m.w.count(t, `SELECT COUNT(*) FROM study_sessions WHERE ended_at > 0 AND (pending_review_id != 0 OR retry_item_id != 0)`); n != 0 {
		t.Fatalf("%d ended sessions still hold a judgment or retry", n)
	}
	if n := m.w.count(t, `SELECT COUNT(*) FROM (SELECT 1 FROM reviews WHERE practice = 0 GROUP BY session_id, item_id HAVING COUNT(*) > 1)`); n != 0 {
		t.Fatalf("%d items scored twice in one session", n)
	}
	if n := m.w.count(t, `SELECT COUNT(*) FROM session_items si LEFT JOIN reviews r ON r.id = si.review_id
		WHERE si.review_id != 0 AND (r.id IS NULL OR r.practice = 1 OR r.item_id != si.item_id OR r.session_id != si.session_id)`); n != 0 {
		t.Fatalf("%d session entries point at the wrong review", n)
	}
	now := m.w.reviews(t)
	for id, r := range m.reviews {
		if now[id] != r {
			t.Fatalf("review %d was rewritten: %+v -> %+v", id, r, now[id])
		}
		if g, ok := m.graded[id]; ok && (r.GradedPrompt != g[0] || r.GradedExpected != g[1]) {
			t.Fatalf("review %d graded against %q/%q, the item was %q/%q", id, r.GradedPrompt, r.GradedExpected, g[0], g[1])
		}
	}
	for _, id := range m.items {
		got, prev := m.w.schedule(t, id), m.sched[id]
		if err := srs.ValidateState(got); err != nil {
			t.Fatalf("item %d: %v", id, err)
		}
		if got.Reps < prev.Reps || got.Lapses < prev.Lapses {
			t.Fatalf("item %d went backwards: %+v -> %+v", id, prev, got)
		}
		if scored := m.w.count(t, `SELECT COUNT(*) FROM reviews WHERE item_id = ? AND practice = 0`, id); scored != got.Reps {
			t.Fatalf("item %d has %d reps but %d scored reviews", id, got.Reps, scored)
		}
		m.sched[id] = got
	}
}

func sortedIDs(ids []int64) []int64 {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
	return ids
}

// TestRapidStudyStateMachine is the model-based test of Study against the
// formal invariants of specs/study_session.qnt, on the real SQLite store
// and app: phase consistency, FSRS bounds and monotonicity, practice never
// moving the schedule or replacing the scored review, "Why?" explaining
// the graded snapshot without writing anything, and stale or out-of-phase
// submissions being refused.
func TestRapidStudyStateMachine(t *testing.T) {
	dir := rapidDir(t)
	var n atomic.Int64
	rapid.Check(t, func(t *rapid.T) {
		w := newStudyWorld(t, filepath.Join(dir, fmt.Sprintf("study%d.db", n.Add(1))))
		defer w.st.Close()
		count := rapid.IntRange(1, 4).Draw(t, "items")
		var exact []bool
		for i := 0; i < count; i++ {
			exact = append(exact, rapid.Bool().Draw(t, "exact"))
		}
		m := &studyMachine{w: w, exact: map[int64]bool{}, text: map[int64][2]string{}, removed: map[int64]bool{},
			sched: map[int64]srs.State{}, reviews: map[int64]reviewRow{}, graded: map[int64][2]string{}}
		m.items = w.addItems(t, exact...)
		for i, id := range m.items {
			m.exact[id] = exact[i]
		}
		t.Repeat(rapid.StateMachineActions(m))
	})
}

// TestStudyDoubleSubmitSchedulesOnce is the regression test for the
// double-submit race: two requests answer the same item at once, both
// pass the pre-check before either is recorded, and both are judged. Only
// one may be recorded; the other is ErrStale. Before the transition was
// guarded inside the write transaction, both were recorded and the item's
// schedule moved twice (reps +2).
func TestStudyDoubleSubmitSchedulesOnce(t *testing.T) {
	w := newStudyWorld(t, filepath.Join(t.TempDir(), "race.db"))
	defer w.st.Close()
	items := w.addItems(t, false)
	if _, err := w.app.Study(w.ctx, w.user); err != nil {
		t.Fatal(err)
	}
	gate, arrived := make(chan struct{}), make(chan struct{}, 2)
	w.model.mu.Lock()
	w.model.verdict, w.model.gate, w.model.arrived = srs.Close, gate, arrived
	w.model.mu.Unlock()
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := w.app.Answer(w.ctx, w.user, items[0], "an answer", "typed")
			errs <- err
		}()
	}
	<-arrived
	<-arrived // both requests are being judged: neither has been recorded
	close(gate)
	var ok, stale int
	for i := 0; i < 2; i++ {
		switch err := <-errs; {
		case err == nil:
			ok++
		case errors.Is(err, app.ErrStale):
			stale++
		default:
			t.Fatalf("Answer: %v", err)
		}
	}
	if ok != 1 || stale != 1 {
		t.Fatalf("%d recorded, %d stale; want 1 and 1", ok, stale)
	}
	if s := w.schedule(t, items[0]); s.Reps != 1 {
		t.Fatalf("reps = %d after one judged answer, want 1", s.Reps)
	}
	if n := w.count(t, `SELECT COUNT(*) FROM reviews`); n != 1 {
		t.Fatalf("%d reviews recorded, want 1", n)
	}
	if reserved, err := w.us.ReservedMicros(w.ctx); err != nil || reserved != 0 {
		t.Fatalf("reserved %d after both settled (%v)", reserved, err)
	}
}

// TestStudyJudgeFailureReleasesReservation: a judge that fails before
// using anything leaves no reservation behind. Before, the reservation
// was only settled when the call reported a model, so each such failure
// held the judge's budget forever and shrank the learner's allowance.
func TestStudyJudgeFailureReleasesReservation(t *testing.T) {
	w := newStudyWorld(t, filepath.Join(t.TempDir(), "leak.db"))
	defer w.st.Close()
	items := w.addItems(t, false)
	if _, err := w.app.Study(w.ctx, w.user); err != nil {
		t.Fatal(err)
	}
	w.model.mu.Lock()
	w.model.fail = true
	w.model.mu.Unlock()
	for i := 0; i < 3; i++ {
		if _, err := w.app.Answer(w.ctx, w.user, items[0], "an answer", "typed"); !errors.Is(err, app.ErrJudgeUnavailable) {
			t.Fatalf("Answer with the judge down: %v", err)
		}
	}
	if reserved, err := w.us.ReservedMicros(w.ctx); err != nil || reserved != 0 {
		t.Fatalf("%d micros still reserved after failed judgments (%v)", reserved, err)
	}
	if s := w.schedule(t, items[0]); s.Reps != 0 {
		t.Fatal("an unjudged answer moved the schedule")
	}
}

// TestStudyRetryOfEditedTextIsDropped: an exact text put back up with
// "Again" and then edited to be judged by the model is not practised as
// a model-judged "practice" answer: the retry is dropped, nothing is
// judged or recorded, and the session moves on.
func TestStudyRetryOfEditedTextIsDropped(t *testing.T) {
	w := newStudyWorld(t, filepath.Join(t.TempDir(), "retry.db"))
	defer w.st.Close()
	items := w.addItems(t, true, false)
	if _, err := w.app.Study(w.ctx, w.user); err != nil {
		t.Fatal(err)
	}
	if _, err := w.app.Answer(w.ctx, w.user, items[0], "wrong words", "typed"); err != nil {
		t.Fatal(err)
	}
	if err := w.app.Next(w.ctx, w.user, true); err != nil {
		t.Fatal(err)
	}
	s, _ := w.us.ActiveSession(w.ctx)
	if s.Phase() != store.PhaseRetryingExact || s.RetryItemID != items[0] {
		t.Fatalf("Again did not retry: %s", s.Phase())
	}
	if err := w.us.UpdateItem(w.ctx, items[0], "question 0", "answer number 0", false); err != nil {
		t.Fatal(err)
	}
	if _, err := w.app.Answer(w.ctx, w.user, items[0], "answer number 0", "typed"); !errors.Is(err, app.ErrStale) {
		t.Fatalf("practice of a no-longer-exact text: %v", err)
	}
	if w.model.calls != 0 || w.count(t, `SELECT COUNT(*) FROM reviews`) != 1 {
		t.Fatal("the dropped retry was judged or recorded")
	}
	s, _ = w.us.ActiveSession(w.ctx)
	if s == nil || s.Phase() != store.PhaseAsking || s.Current().Item.ID != items[1] {
		t.Fatal("the session did not move on to the next item")
	}
	// Repeated Enter with nothing on screen changes nothing.
	if err := w.app.Next(w.ctx, w.user, false); err != nil {
		t.Fatal(err)
	}
	if s2, _ := w.us.ActiveSession(w.ctx); s2 == nil || s2.ID != s.ID || s2.Phase() != store.PhaseAsking {
		t.Fatal("Next in the asking phase changed the session")
	}
}

// rapidDir is where a rapid test keeps its throwaway databases: on tmpfs
// when there is one (each check opens a fresh database, and fsyncs to
// disk would dominate), otherwise the test's temporary directory.
func rapidDir(t *testing.T) string {
	if fi, err := os.Stat("/dev/shm"); err == nil && fi.IsDir() {
		if dir, err := os.MkdirTemp("/dev/shm", "scry-rapid-"); err == nil {
			t.Cleanup(func() { os.RemoveAll(dir) })
			return dir
		}
	}
	return t.TempDir()
}
