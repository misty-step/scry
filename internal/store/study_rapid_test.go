package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"

	"scry/internal/srs"
)

// TestSessionTransitionTable pins the explicit study session state machine
// (specs/study_session.qnt): every legal pair, and every other pair refused
// with a typed error.
func TestSessionTransitionTable(t *testing.T) {
	legal := map[sessionStep]SessionPhase{
		// Every scored answer is held for its post-answer review: there is
		// no (Asking, AnswerPassed) pair, so a right answer cannot skip it.
		{PhaseAsking, EventAnswerHeld}:            PhasePendingReview,
		{PhaseRetryingExact, EventAnswerPractice}: PhasePendingReview,
		{PhasePendingReview, EventAdvance}:        PhaseAsking,
		{PhasePendingReview, EventRetry}:          PhaseRetryingExact,
		{PhaseRetryingExact, EventAdvance}:        PhaseAsking,
		{PhaseAsking, EventEnd}:                   PhaseEnded,
		{PhasePendingReview, EventEnd}:            PhaseEnded,
		{PhaseRetryingExact, EventEnd}:            PhaseEnded,
	}
	for from := PhaseAsking; from <= PhaseEnded; from++ {
		for ev := EventAnswerHeld; ev <= EventEnd; ev++ {
			to, err := Transition(from, ev)
			want, ok := legal[sessionStep{from, ev}]
			if ok {
				if err != nil || to != want {
					t.Errorf("%s --%s--> %s, %v; want %s", from, ev, to, err, want)
				}
				continue
			}
			var te *TransitionError
			if !errors.Is(err, ErrIllegalTransition) || !errors.As(err, &te) || te.From != from || te.Event != ev || to != from {
				t.Errorf("%s --%s--> %s accepted or untyped: %v", from, ev, to, err)
			}
		}
	}
	if (&StudySession{Ended: time.Unix(1, 0), PendingReviewID: 3}).Phase() != PhaseEnded ||
		(&StudySession{PendingReviewID: 3}).Phase() != PhasePendingReview ||
		(&StudySession{RetryItemID: 3}).Phase() != PhaseRetryingExact ||
		(&StudySession{}).Phase() != PhaseAsking || (*StudySession)(nil).Phase() != PhaseEnded {
		t.Error("Phase() derives the wrong phase")
	}
}

// sessionModel is the reference model of one learner's study state.
type sessionModel struct {
	ctx context.Context
	st  *Store
	us  *UserStore
	now time.Time

	items   []int64
	exact   map[int64]bool
	deleted map[int64]bool
	reps    map[int64]int
	ideaOf  map[int64]int64 // each item is in an idea of its own
	trashed map[int64]bool  // ideas in Trash
	viaIdea map[int64]bool  // items removed with their idea (restored with it)

	open     int64           // the open session, or 0
	ended    []int64         // sessions that are over
	entries  map[int64]bool  // items of the open session
	order    []int64         // the open session's items, in order
	answered map[int64]int64 // item -> its scored review in the open session
	pending  int64           // the review on screen
	pendItem int64           // its item
	pendWord bool            // it was judged word for word
	retry    int64
}

func (m *sessionModel) phase() SessionPhase {
	switch {
	case m.open == 0:
		return PhaseEnded
	case m.pending != 0:
		return PhasePendingReview
	case m.retry != 0:
		return PhaseRetryingExact
	}
	return PhaseAsking
}

// current is the item the open session is asking for: the retried item,
// else the first of its items, in order, still in the library and not yet
// answered (StudySession.Current, specs/study_session.qnt currentOf).
func (m *sessionModel) current() int64 {
	if m.retry != 0 {
		return m.retry
	}
	for _, id := range m.order {
		if _, answered := m.answered[id]; !answered && !m.deleted[id] {
			return id
		}
	}
	return 0
}

// observed is the session state a caller that is up to date would pass to Advance.
func (m *sessionModel) observed() SessionObserved {
	return SessionObserved{SessionID: m.open, PendingReviewID: m.pending, RetryItemID: m.retry}
}

func (m *sessionModel) tick() time.Time { m.now = m.now.Add(time.Minute); return m.now }

func (m *sessionModel) item(t *rapid.T) int64 { return rapid.SampledFrom(m.items).Draw(t, "item") }

func (m *sessionModel) schedule(t *rapid.T, id int64) srs.State {
	var s srs.State
	var last, due int64
	if err := m.st.db.QueryRow(`SELECT stability, difficulty, reps, lapses, last_review, due_at FROM items WHERE id = ?`, id).
		Scan(&s.Stability, &s.Difficulty, &s.Reps, &s.Lapses, &last, &due); err != nil {
		t.Fatal(err)
	}
	s.Last, s.Due = tm(last), tm(due)
	return s
}

func (m *sessionModel) expect(t *rapid.T, what string, err error, legal bool) {
	t.Helper()
	switch {
	case legal && err != nil:
		t.Fatalf("%s: legal transition refused: %v", what, err)
	case !legal && !errors.Is(err, ErrIllegalTransition):
		t.Fatalf("%s: illegal transition not refused with ErrIllegalTransition: %v", what, err)
	}
}

func (m *sessionModel) Start(t *rapid.T) {
	var ids []int64
	for _, id := range m.items {
		if !m.deleted[id] {
			ids = append(ids, id)
		}
	}
	id, err := m.us.StartSession(m.ctx, ids, m.tick())
	if err != nil {
		t.Fatal(err)
	}
	if m.open != 0 {
		m.ended = append(m.ended, m.open)
	}
	m.open, m.pending, m.pendItem, m.retry = id, 0, 0, 0
	m.entries, m.answered, m.order = map[int64]bool{}, map[int64]int64{}, ids
	for _, it := range ids {
		m.entries[it] = true
	}
}

func (m *sessionModel) record(t *rapid.T, session, item int64, practice, hold bool) (int64, error) {
	v := rapid.SampledFrom(srs.Verdicts).Draw(t, "verdict")
	s := m.schedule(t, item)
	judge := "model"
	if m.exact[item] {
		judge = "exact"
	}
	return m.us.RecordReview(m.ctx, Review{ItemID: item, SessionID: session, At: m.tick(), Answer: "a", Verdict: v,
		Reason: "because", Detail: "{}", Via: "typed", Judge: judge, Practice: practice}, srs.Next(s, v, m.now), hold)
}

func (m *sessionModel) AnswerScored(t *rapid.T) {
	item, hold := m.item(t), rapid.Bool().Draw(t, "hold")
	session := m.open
	if session == 0 && len(m.ended) > 0 {
		session = m.ended[len(m.ended)-1]
	}
	if session == 0 {
		t.Skip("no session yet")
	}
	_, answered := m.answered[item]
	// A scored answer is legal only when it is held for its review.
	legal := hold && m.open != 0 && m.phase() == PhaseAsking && m.entries[item] && !answered && !m.deleted[item] && m.current() == item
	id, err := m.record(t, session, item, false, hold)
	m.expect(t, fmt.Sprintf("scored answer to %d (hold %v) in %s", item, hold, m.phase()), err, legal)
	if legal {
		m.answered[item], m.reps[item] = id, m.reps[item]+1
		m.pending, m.pendItem, m.pendWord = id, item, m.exact[item]
	}
}

func (m *sessionModel) AnswerPractice(t *rapid.T) {
	if m.open == 0 {
		t.Skip("no open session")
	}
	item, hold := m.item(t), rapid.Bool().Draw(t, "hold")
	if m.retry != 0 && rapid.Bool().Draw(t, "retried") {
		item = m.retry
	}
	legal := m.phase() == PhaseRetryingExact && m.retry == item && m.exact[item] && !m.deleted[item] && hold
	id, err := m.record(t, m.open, item, true, hold)
	m.expect(t, fmt.Sprintf("practice answer to %d (hold %v) in %s", item, hold, m.phase()), err, legal)
	if legal {
		m.pending, m.pendItem, m.pendWord, m.retry = id, item, true, 0
	}
}

func (m *sessionModel) Advance(t *rapid.T) {
	if m.open == 0 {
		t.Skip("no open session")
	}
	var retry int64
	if rapid.Bool().Draw(t, "again") {
		retry = m.item(t)
		if m.pendItem != 0 && rapid.Bool().Draw(t, "onScreen") {
			retry = m.pendItem
		}
	}
	legal := m.phase() == PhasePendingReview || m.phase() == PhaseRetryingExact
	if retry != 0 {
		legal = m.phase() == PhasePendingReview && m.pendItem == retry && m.pendWord && m.exact[retry] && !m.deleted[retry]
	}
	err := m.us.Advance(m.ctx, m.observed(), retry)
	m.expect(t, fmt.Sprintf("advance (retry %d) in %s", retry, m.phase()), err, legal)
	if legal {
		m.pending, m.pendItem, m.pendWord, m.retry = 0, 0, false, retry
	}
}

// StaleAdvance moves the open session on from a state it is no longer in:
// a judgment or retry another tab has since replaced or cleared. It must
// be refused and change nothing (specs/study_session.qnt, commitNextWith).
func (m *sessionModel) StaleAdvance(t *rapid.T) {
	if m.open == 0 {
		t.Skip("no open session")
	}
	seen := m.observed()
	switch rapid.IntRange(0, 2).Draw(t, "stale") {
	case 0:
		seen.PendingReviewID = m.pending + rapid.Int64Range(1, 3).Draw(t, "delta")
	case 1:
		seen.PendingReviewID = 0
		if m.pending == 0 {
			seen.PendingReviewID = rapid.Int64Range(1, 50).Draw(t, "earlier")
		}
	case 2:
		seen.RetryItemID = m.retry + rapid.SampledFrom(m.items).Draw(t, "other")
	}
	var retry int64
	if m.pendItem != 0 && rapid.Bool().Draw(t, "again") {
		retry = m.pendItem
	}
	m.expect(t, fmt.Sprintf("stale advance %+v (retry %d) in %s", seen, retry, m.phase()), m.us.Advance(m.ctx, seen, retry), false)
}

func (m *sessionModel) End(t *rapid.T) {
	if m.open == 0 {
		t.Skip("no open session")
	}
	m.expect(t, "end", m.us.EndSession(m.ctx, m.open, m.tick()), true)
	m.ended = append(m.ended, m.open)
	m.open, m.pending, m.pendItem, m.retry = 0, 0, 0, 0
}

// MutateEnded tries every event on a session that is over.
func (m *sessionModel) MutateEnded(t *rapid.T) {
	if len(m.ended) == 0 {
		t.Skip("nothing ended yet")
	}
	old := rapid.SampledFrom(m.ended).Draw(t, "ended")
	item := m.item(t)
	_, err := m.record(t, old, item, rapid.Bool().Draw(t, "practice"), true)
	m.expect(t, "answer in an ended session", err, false)
	m.expect(t, "advance an ended session", m.us.Advance(m.ctx, SessionObserved{SessionID: old}, 0), false)
	m.expect(t, "retry in an ended session", m.us.Advance(m.ctx, SessionObserved{SessionID: old}, item), false)
	m.expect(t, "end an ended session", m.us.EndSession(m.ctx, old, m.tick()), false)
}

func (m *sessionModel) ToggleExact(t *rapid.T) {
	item := m.item(t)
	if m.deleted[item] {
		t.Skip("removed")
	}
	if err := m.us.UpdateItem(m.ctx, item, "q", "a", !m.exact[item]); err != nil {
		t.Fatal(err)
	}
	m.exact[item] = !m.exact[item]
}

func (m *sessionModel) Remove(t *rapid.T) {
	item := m.item(t)
	if m.deleted[item] {
		t.Skip("removed")
	}
	if err := m.us.RemoveItem(m.ctx, item, m.tick()); err != nil {
		t.Fatal(err)
	}
	m.deleted[item] = true
}

// TrashIdea moves an item's idea to Trash, taking the item with it unless
// it was already removed on its own.
func (m *sessionModel) TrashIdea(t *rapid.T) {
	item := m.item(t)
	idea := m.ideaOf[item]
	if m.trashed[idea] {
		t.Skip("in Trash")
	}
	if err := m.us.RemoveIdea(m.ctx, idea, m.tick()); err != nil {
		t.Fatal(err)
	}
	m.trashed[idea] = true
	if !m.deleted[item] {
		m.deleted[item], m.viaIdea[item] = true, true
	}
}

// RestoreIdea brings an idea back from Trash, with the items it took.
func (m *sessionModel) RestoreIdea(t *rapid.T) {
	item := m.item(t)
	idea := m.ideaOf[item]
	if !m.trashed[idea] {
		t.Skip("not in Trash")
	}
	if err := m.us.RestoreIdea(m.ctx, idea); err != nil {
		t.Fatal(err)
	}
	m.trashed[idea] = false
	if m.viaIdea[item] {
		m.deleted[item], m.viaIdea[item] = false, false
	}
}

func (m *sessionModel) Check(t *rapid.T) {
	s, err := m.us.ActiveSession(m.ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := s.Phase()
	if got != m.phase() {
		t.Fatalf("phase %s, model %s", got, m.phase())
	}
	if s != nil {
		if s.ID != m.open || s.PendingReviewID != m.pending || s.RetryItemID != m.retry {
			t.Fatalf("session %+v, model open=%d pending=%d retry=%d", s, m.open, m.pending, m.retry)
		}
		if s.PendingReviewID != 0 && s.RetryItemID != 0 {
			t.Fatal("a judgment on screen and a retry at once")
		}
		var cur int64
		if e := s.Current(); e != nil {
			cur = e.Item.ID
		}
		if cur != m.current() {
			t.Fatalf("current item %d, model %d", cur, m.current())
		}
		for _, e := range s.Entries {
			want, ok := m.answered[e.Item.ID]
			if (e.Review != nil) != ok || (ok && e.Review.ID != want) {
				t.Fatalf("entry %d review %+v, model %d", e.Item.ID, e.Review, want)
			}
		}
	}
	for _, id := range m.items {
		sched := m.schedule(t, id)
		if sched.Reps != m.reps[id] {
			t.Fatalf("item %d reps %d, model %d (practice or a refused answer moved the schedule)", id, sched.Reps, m.reps[id])
		}
		if err := srs.ValidateState(sched); err != nil {
			t.Fatalf("item %d: %v", id, err)
		}
	}
	var stray int
	if err := m.st.db.QueryRow(`SELECT COUNT(*) FROM study_sessions WHERE ended_at > 0 AND (pending_review_id != 0 OR retry_item_id != 0)`).Scan(&stray); err != nil || stray != 0 {
		t.Fatalf("ended sessions with a pending review or retry: %d %v", stray, err)
	}
}

// TestRapidSessionMachine drives the real SQLite store with arbitrary,
// often illegal, session events (including stale moves and ideas moved to
// Trash and back mid-session) and checks it accepts exactly what the
// transition table and its guards allow, against a reference model.
func TestRapidSessionMachine(t *testing.T) {
	dir := rapidDir(t)
	var n atomic.Int64
	rapid.Check(t, func(t *rapid.T) {
		st, err := Open(filepath.Join(dir, fmt.Sprintf("s%d.db", n.Add(1))))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		ctx := context.Background()
		now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
		u, err := st.CreateUser(ctx, "learner@example.com", "learner", "UTC", now)
		if err != nil {
			t.Fatal(err)
		}
		m := &sessionModel{ctx: ctx, st: st, us: st.For(u.ID), now: now, exact: map[int64]bool{}, deleted: map[int64]bool{},
			reps: map[int64]int{}, ideaOf: map[int64]int64{}, trashed: map[int64]bool{}, viaIdea: map[int64]bool{}}
		count := rapid.IntRange(1, 4).Draw(t, "items")
		for i := 0; i < count; i++ {
			idea, err := m.us.CreateIdea(ctx, 0, fmt.Sprintf("Idea %d", i), "", "", now)
			if err != nil {
				t.Fatal(err)
			}
			ex := rapid.Bool().Draw(t, "exact")
			id, err := m.us.AddItem(ctx, NewItem{IdeaID: idea, Prompt: "q", Answer: "a", Exact: ex}, now)
			if err != nil {
				t.Fatal(err)
			}
			m.items, m.exact[id], m.ideaOf[id] = append(m.items, id), ex, idea
		}
		t.Repeat(rapid.StateMachineActions(m))
	})
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
