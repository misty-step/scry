package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"scry/internal/srs"
)

// Review is one judged answer in the learner's history.
type Review struct {
	ID        int64
	ItemID    int64
	SessionID int64
	At        time.Time
	Answer    string
	Verdict   srs.Verdict
	Reason    string // the one-line why
	Detail    string // JSON: close parts, or the word-for-word diff
	Via       string // typed or spoken
	Judge     string // which judge decided: a model id, or "exact"
	Practice  bool   // an "Again" attempt; it does not move the schedule
	// The question and expected answer exactly as they were when this
	// answer was judged. Editing the item later does not change them, so
	// an explanation of this verdict explains what was actually graded.
	// Empty on reviews recorded before they were kept.
	GradedPrompt   string
	GradedExpected string
}

// reviewColumns are the columns added to reviews after it was first created.
var reviewColumns = [][2]string{
	{"graded_prompt", "TEXT NOT NULL DEFAULT ''"},
	{"graded_expected", "TEXT NOT NULL DEFAULT ''"},
}

// SessionEntry is one item of a study session and, once answered, its review.
type SessionEntry struct {
	Position int
	Item     Item
	Review   *Review
}

// StudySession is a run of due items worked through in order.
//
// A session is an explicit state machine whose phase is derived from its
// row (see Phase and the transition table below):
//
//	Asking        --AnswerHeld-->     PendingReview  (the post-answer review)
//	RetryingExact --AnswerPractice--> PendingReview  (an exact text tried again)
//	PendingReview --Advance-->        Asking
//	PendingReview --Retry-->          RetryingExact  (only for an exact text)
//	RetryingExact --Advance-->        Asking         (the retry is skipped)
//	Asking | PendingReview | RetryingExact --End--> Ended
//
// Every scored answer, whatever its verdict, is held in PendingReview (the
// post-answer review) until the learner moves on: a session never advances
// by itself. Ended is terminal: every event on an ended session is refused.
type StudySession struct {
	ID              int64
	Started, Ended  time.Time
	PendingReviewID int64 // a judgment on screen waiting for Enter
	RetryItemID     int64 // an exact text being tried again
	Entries         []SessionEntry
}

// SessionPhase is where a study session stands.
type SessionPhase uint8

const (
	// PhaseAsking waits for an answer to the current item (or, with no
	// item left, for the session to be ended).
	PhaseAsking SessionPhase = iota
	// PhasePendingReview shows a judgment until the learner moves on.
	PhasePendingReview
	// PhaseRetryingExact puts an exact text back up for a practice attempt.
	PhaseRetryingExact
	// PhaseEnded is a finished session. It is terminal.
	PhaseEnded
)

func (p SessionPhase) String() string {
	switch p {
	case PhaseAsking:
		return "asking"
	case PhasePendingReview:
		return "pending-review"
	case PhaseRetryingExact:
		return "retrying-exact"
	case PhaseEnded:
		return "ended"
	}
	return fmt.Sprintf("SessionPhase(%d)", uint8(p))
}

// SessionEvent is something that moves a study session.
type SessionEvent uint8

const (
	// EventAnswerHeld is a scored answer whose judgment stays on screen.
	EventAnswerHeld SessionEvent = iota
	// EventAnswerPassed is a scored answer that would move straight on
	// without a review. No phase accepts it: a right answer used to pass,
	// and is now held like every other, so recording one is refused.
	EventAnswerPassed
	// EventAnswerPractice is an "Again" attempt at an exact text; it is
	// held on screen and never moves the schedule.
	EventAnswerPractice
	// EventAdvance clears the judgment on screen, or skips a retry.
	EventAdvance
	// EventRetry puts the exact text on screen back up for another attempt.
	EventRetry
	// EventEnd closes the session.
	EventEnd
)

func (e SessionEvent) String() string {
	switch e {
	case EventAnswerHeld:
		return "answer-held"
	case EventAnswerPassed:
		return "answer-passed"
	case EventAnswerPractice:
		return "answer-practice"
	case EventAdvance:
		return "advance"
	case EventRetry:
		return "retry"
	case EventEnd:
		return "end"
	}
	return fmt.Sprintf("SessionEvent(%d)", uint8(e))
}

type sessionStep struct {
	from SessionPhase
	on   SessionEvent
}

// sessionTransitions is the whole study session state machine. A pair
// that is not here is an illegal transition.
var sessionTransitions = map[sessionStep]SessionPhase{
	{PhaseAsking, EventAnswerHeld}:            PhasePendingReview,
	{PhaseRetryingExact, EventAnswerPractice}: PhasePendingReview,
	{PhasePendingReview, EventAdvance}:        PhaseAsking,
	{PhasePendingReview, EventRetry}:          PhaseRetryingExact,
	{PhaseRetryingExact, EventAdvance}:        PhaseAsking,
	{PhaseAsking, EventEnd}:                   PhaseEnded,
	{PhasePendingReview, EventEnd}:            PhaseEnded,
	{PhaseRetryingExact, EventEnd}:            PhaseEnded,
}

// ErrIllegalTransition is an event the study session cannot take in its
// current phase, or whose guard fails: answering an ended session or an
// item already answered, moving on from the wrong phase, retrying an item
// that is not an exact text. Every *TransitionError matches it, and so do
// the refused top-up and subscription transitions (*TopupTransitionError,
// *SubscriptionTransitionError).
var ErrIllegalTransition = errors.New("illegal state transition")

// TransitionError is a refused session transition.
type TransitionError struct {
	From   SessionPhase
	Event  SessionEvent
	Reason string // the guard that failed, when the table allowed the pair
}

func (e *TransitionError) Error() string {
	msg := fmt.Sprintf("%v: %s from %s", ErrIllegalTransition, e.Event, e.From)
	if e.Reason != "" {
		msg += ": " + e.Reason
	}
	return msg
}

func (e *TransitionError) Is(target error) bool { return target == ErrIllegalTransition }

// Transition is the phase an event moves a session to, or a
// *TransitionError when the session cannot take it.
func Transition(from SessionPhase, ev SessionEvent) (SessionPhase, error) {
	if to, ok := sessionTransitions[sessionStep{from, ev}]; ok {
		return to, nil
	}
	return from, &TransitionError{From: from, Event: ev}
}

// phaseOf derives a session's phase from its row.
func phaseOf(ended, pendingReviewID, retryItemID int64) SessionPhase {
	switch {
	case ended != 0:
		return PhaseEnded
	case pendingReviewID != 0:
		return PhasePendingReview
	case retryItemID != 0:
		return PhaseRetryingExact
	}
	return PhaseAsking
}

// Phase is where the session stands.
func (s *StudySession) Phase() SessionPhase {
	if s == nil {
		return PhaseEnded
	}
	ended := int64(0)
	if !s.Ended.IsZero() {
		ended = 1
	}
	return phaseOf(ended, s.PendingReviewID, s.RetryItemID)
}

// Current is the entry waiting for an answer, or nil when all are done.
func (s *StudySession) Current() *SessionEntry {
	for i := range s.Entries {
		if s.RetryItemID != 0 && s.Entries[i].Item.ID == s.RetryItemID {
			return &s.Entries[i]
		}
	}
	for i := range s.Entries {
		if s.Entries[i].Review == nil {
			return &s.Entries[i]
		}
	}
	return nil
}

// Reviewed counts answered entries.
func (s *StudySession) Reviewed() int {
	n := 0
	for _, e := range s.Entries {
		if e.Review != nil {
			n++
		}
	}
	return n
}

// DueItems lists items due before the given moment: reviews first, most
// overdue first, then items never seen, oldest first.
func (u *UserStore) DueItems(ctx context.Context, before time.Time, limit int) ([]Item, error) {
	return scanItems(u.s.db.QueryContext(ctx, `SELECT `+itemCols+itemFrom+`
		WHERE t.user_id = ? AND t.deleted_at = 0 AND i.deleted_at = 0 AND t.due_at < ?
		ORDER BY (t.reps = 0), CASE WHEN t.reps = 0 THEN t.id ELSE t.due_at END, t.id LIMIT ?`, u.uid, ts(before), limit))
}

// StartSession opens a session over the given items, in order.
func (u *UserStore) StartSession(ctx context.Context, itemIDs []int64, now time.Time) (int64, error) {
	var id int64
	err := u.tx(ctx, func(tx *sql.Tx) error {
		// Starting a session ends the open one (EventEnd): like EndSession,
		// an ended session keeps no judgment on screen and no retry.
		if _, err := tx.ExecContext(ctx, `UPDATE study_sessions SET ended_at = ?, pending_review_id = 0, retry_item_id = 0
			WHERE user_id = ? AND ended_at = 0`, ts(now), u.uid); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO study_sessions (user_id, started_at) VALUES (?, ?)`, u.uid, ts(now))
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		for pos, item := range itemIDs {
			// The SELECT keeps other learners' items out of the session.
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO session_items (user_id, session_id, position, item_id)
				SELECT ?1, ?2, ?3, id FROM items WHERE id = ?4 AND user_id = ?1 AND deleted_at = 0`, u.uid, id, pos+1, item); err != nil {
				return err
			}
		}
		return nil
	})
	return id, err
}

// ActiveSession returns the open session, or nil when there is none.
func (u *UserStore) ActiveSession(ctx context.Context) (*StudySession, error) {
	return u.session(ctx, `ended_at = 0 ORDER BY id DESC LIMIT 1`)
}

// LastEndedSession returns the most recently finished session, or nil.
func (u *UserStore) LastEndedSession(ctx context.Context) (*StudySession, error) {
	return u.session(ctx, `ended_at > 0 ORDER BY ended_at DESC, id DESC LIMIT 1`)
}

func (u *UserStore) session(ctx context.Context, where string) (*StudySession, error) {
	var s StudySession
	var started, ended int64
	err := u.s.db.QueryRowContext(ctx, `
		SELECT id, started_at, ended_at, pending_review_id, retry_item_id FROM study_sessions WHERE user_id = ? AND `+where, u.uid).
		Scan(&s.ID, &started, &ended, &s.PendingReviewID, &s.RetryItemID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Started, s.Ended = tm(started), tm(ended)
	reviews, err := u.sessionReviews(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	rows, err := u.s.db.QueryContext(ctx, `SELECT si.position, si.review_id, `+itemCols+`
		FROM session_items si JOIN items t ON t.id = si.item_id AND t.user_id = si.user_id
		JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id
		WHERE si.session_id = ? AND si.user_id = ? AND (t.deleted_at = 0 OR si.review_id != 0)
		ORDER BY si.position`, s.ID, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e SessionEntry
		var reviewID int64
		var it Item
		var exact int
		var created, due, last int64
		if err := rows.Scan(&e.Position, &reviewID, &it.ID, &it.IdeaID, &it.IdeaTitle, &it.Prompt, &it.Answer, &exact, &it.TextTitle,
			&it.LineNo, &it.Source, &created, &due, &it.Sched.Stability, &it.Sched.Difficulty, &it.Sched.Reps, &it.Sched.Lapses, &last); err != nil {
			return nil, err
		}
		it.Exact, it.Created = exact == 1, tm(created)
		it.Sched.Due, it.Sched.Last = tm(due), tm(last)
		e.Item = it
		if r, ok := reviews[reviewID]; ok {
			e.Review = &r
		}
		s.Entries = append(s.Entries, e)
	}
	return &s, rows.Err()
}

const reviewCols = `id, item_id, session_id, at, answer, verdict, reason, detail, via, judge, practice, graded_prompt, graded_expected`

func scanReview(row interface{ Scan(...any) error }) (Review, error) {
	var r Review
	var at int64
	var practice int
	var verdict string
	err := row.Scan(&r.ID, &r.ItemID, &r.SessionID, &at, &r.Answer, &verdict, &r.Reason, &r.Detail, &r.Via, &r.Judge, &practice,
		&r.GradedPrompt, &r.GradedExpected)
	r.At, r.Verdict, r.Practice = tm(at), srs.Verdict(verdict), practice == 1
	return r, notFound(err)
}

func (u *UserStore) sessionReviews(ctx context.Context, sessionID int64) (map[int64]Review, error) {
	rows, err := u.s.db.QueryContext(ctx, `SELECT `+reviewCols+` FROM reviews WHERE session_id = ? AND user_id = ?`, sessionID, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Review{}
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out[r.ID] = r
	}
	return out, rows.Err()
}

// Review loads one review from the learner's history.
func (u *UserStore) Review(ctx context.Context, id int64) (Review, error) {
	return scanReview(u.s.db.QueryRowContext(ctx, `SELECT `+reviewCols+` FROM reviews WHERE id = ? AND user_id = ?`, id, u.uid))
}

// History lists the reviews of one item, newest first.
func (u *UserStore) History(ctx context.Context, itemID int64) ([]Review, error) {
	rows, err := u.s.db.QueryContext(ctx, `SELECT `+reviewCols+` FROM reviews WHERE item_id = ? AND user_id = ? ORDER BY at DESC, id DESC LIMIT 50`, itemID, u.uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Review
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// sessionRow is a session's state as read inside a write transaction.
type sessionRow struct {
	ended, pending, retry int64
}

func (r sessionRow) phase() SessionPhase { return phaseOf(r.ended, r.pending, r.retry) }

func (u *UserStore) lockedSession(ctx context.Context, tx *sql.Tx, id int64) (sessionRow, error) {
	var r sessionRow
	err := tx.QueryRowContext(ctx, `SELECT ended_at, pending_review_id, retry_item_id FROM study_sessions WHERE id = ? AND user_id = ?`, id, u.uid).
		Scan(&r.ended, &r.pending, &r.retry)
	return r, notFound(err)
}

// answerEvent is the session event a review is.
func answerEvent(r Review, hold bool) SessionEvent {
	switch {
	case r.Practice:
		return EventAnswerPractice
	case hold:
		return EventAnswerHeld
	}
	return EventAnswerPassed
}

// guardAnswer checks, under the write lock, that the session can take
// the review: the table allows the event in the session's phase, and the
// event's guard holds. A scored answer must be for the session's current
// item: the first of its items, in order, that is still in the library and
// not yet answered (specs/study_session.qnt, commitWith); a practice attempt
// must be for the item the session is retrying, still an exact text, and
// is always held on screen. A failure is a *TransitionError, so a double
// submit, a second tab or an answer racing the session's end can never
// reschedule an item twice or write into an ended session.
func (u *UserStore) guardAnswer(ctx context.Context, tx *sql.Tx, r Review, hold bool) error {
	row, err := u.lockedSession(ctx, tx, r.SessionID)
	if errors.Is(err, ErrNotFound) {
		return &TransitionError{From: PhaseEnded, Event: answerEvent(r, hold), Reason: "no such session"}
	}
	if err != nil {
		return err
	}
	from, ev := row.phase(), answerEvent(r, hold)
	if _, err := Transition(from, ev); err != nil {
		return err
	}
	refuse := func(reason string) error { return &TransitionError{From: from, Event: ev, Reason: reason} }
	if r.Practice {
		if !hold {
			return refuse("a practice attempt is always held on screen")
		}
		if row.retry != r.ItemID {
			return refuse("the session is retrying another item")
		}
		var exact int
		err := tx.QueryRowContext(ctx, `SELECT exact FROM items WHERE id = ? AND user_id = ? AND deleted_at = 0`, r.ItemID, u.uid).Scan(&exact)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && exact != 1) {
			return refuse("the retried item is no longer an exact text in the library")
		}
		return err
	}
	// The scored item must still be the session's current item: the first
	// entry, by position, that is in the library and not yet answered. This
	// is StudySession.Current() (the entries session() loads, in PhaseAsking
	// where there is no retry), read again under the write lock. An item
	// that was current when its answer went to the judge may not be any
	// more: an earlier item can come back (its idea restored from Trash)
	// while the judge is out, and then the answer is stale.
	var current int64
	err = tx.QueryRowContext(ctx, `
		SELECT si.item_id FROM session_items si JOIN items t ON t.id = si.item_id AND t.user_id = si.user_id
		JOIN ideas i ON i.id = t.idea_id AND i.user_id = t.user_id
		WHERE si.session_id = ? AND si.user_id = ? AND si.review_id = 0 AND t.deleted_at = 0
		ORDER BY si.position LIMIT 1`,
		r.SessionID, u.uid).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if current != r.ItemID {
		return refuse("the item is not the current item of this session")
	}
	return nil
}

// RecordReview writes a judged answer to the history, reschedules the item
// (unless it was practice), and marks the session entry answered. In a
// session hold must be set: the judgment stays on screen, as the
// post-answer review, until the learner moves on.
//
// A review in a session (SessionID set) is a session transition and is
// checked against the session's phase inside the same write transaction
// (see guardAnswer); a refused one changes nothing and returns a
// *TransitionError. A review with no session (seeded history) only
// writes the history and the schedule.
func (u *UserStore) RecordReview(ctx context.Context, r Review, next srs.State, hold bool) (int64, error) {
	var id int64
	err := u.tx(ctx, func(tx *sql.Tx) error {
		if r.SessionID != 0 {
			if err := u.guardAnswer(ctx, tx, r, hold); err != nil {
				return err
			}
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO reviews (user_id, item_id, session_id, at, answer, verdict, reason, detail, via, judge, practice, graded_prompt, graded_expected)
			SELECT ?1, id, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13 FROM items WHERE id = ?2 AND user_id = ?1`,
			u.uid, r.ItemID, r.SessionID, ts(r.At), r.Answer, string(r.Verdict), r.Reason, r.Detail, r.Via, r.Judge, b2i(r.Practice),
			r.GradedPrompt, r.GradedExpected)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		id, _ = res.LastInsertId()
		if !r.Practice {
			if _, err := tx.ExecContext(ctx, `
				UPDATE items SET due_at = ?, stability = ?, difficulty = ?, reps = ?, lapses = ?, last_review = ?
				WHERE id = ? AND user_id = ?`,
				ts(next.Due), next.Stability, next.Difficulty, next.Reps, next.Lapses, ts(next.Last), r.ItemID, u.uid); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE session_items SET review_id = ? WHERE session_id = ? AND user_id = ? AND item_id = ? AND review_id = 0`,
				id, r.SessionID, u.uid, r.ItemID); err != nil {
				return err
			}
		}
		if r.SessionID == 0 {
			return nil
		}
		pending := int64(0)
		if hold {
			pending = id
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE study_sessions SET pending_review_id = ?, retry_item_id = 0 WHERE id = ? AND user_id = ? AND ended_at = 0`, pending, r.SessionID, u.uid)
		return err
	})
	return id, err
}

// SessionObserved is the state of a session a caller read before deciding
// to move it on: which session, and the judgment on screen or the retry it
// saw. Advance takes it as the expected state of the session.
type SessionObserved struct {
	SessionID       int64
	PendingReviewID int64 // the judgment on screen, or 0
	RetryItemID     int64 // the exact text being retried, or 0
}

// Observed is what a caller has seen of this session, for Advance.
func (s *StudySession) Observed() SessionObserved {
	return SessionObserved{SessionID: s.ID, PendingReviewID: s.PendingReviewID, RetryItemID: s.RetryItemID}
}

// Advance clears the judgment on screen. retryItemID, when not zero, puts
// that item back up for another attempt.
//
// It is a session transition checked under the write lock: clearing a
// judgment (EventAdvance) needs one on screen, or a retry to skip; a
// retry (EventRetry) needs the judgment on screen to be a word-for-word
// judgment of retryItemID, still an exact text in the library. The session
// must also still be as the caller observed it: the same judgment on screen
// and the same retry (specs/study_session.qnt, commitNextWith). A delayed
// move from a second tab that read an earlier judgment can therefore never
// dismiss a newer one. Anything else, and any event on an ended session, is
// a *TransitionError and changes nothing.
func (u *UserStore) Advance(ctx context.Context, from SessionObserved, retryItemID int64) error {
	return u.tx(ctx, func(tx *sql.Tx) error {
		ev := EventAdvance
		if retryItemID != 0 {
			ev = EventRetry
		}
		row, err := u.lockedSession(ctx, tx, from.SessionID)
		if errors.Is(err, ErrNotFound) {
			return &TransitionError{From: PhaseEnded, Event: ev, Reason: "no such session"}
		}
		if err != nil {
			return err
		}
		phase := row.phase()
		if _, err := Transition(phase, ev); err != nil {
			return err
		}
		if row.pending != from.PendingReviewID || row.retry != from.RetryItemID {
			return &TransitionError{From: phase, Event: ev, Reason: "the session moved on since it was read"}
		}
		if ev == EventRetry {
			var exact int
			err := tx.QueryRowContext(ctx, `
				SELECT t.exact FROM reviews r JOIN items t ON t.id = r.item_id AND t.user_id = r.user_id
				WHERE r.id = ? AND r.user_id = ? AND r.item_id = ? AND r.judge = 'exact' AND t.deleted_at = 0`, row.pending, u.uid, retryItemID).Scan(&exact)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && exact != 1) {
				return &TransitionError{From: phase, Event: ev, Reason: "only an exact text judged word for word on screen can be tried again"}
			}
			if err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE study_sessions SET pending_review_id = 0, retry_item_id = ? WHERE id = ? AND user_id = ? AND ended_at = 0`,
			retryItemID, from.SessionID, u.uid)
		return err
	})
}

// EndSession closes a session. Ending a session that is already ended
// (or is not the learner's) is a *TransitionError.
func (u *UserStore) EndSession(ctx context.Context, id int64, now time.Time) error {
	res, err := u.s.db.ExecContext(ctx, `
		UPDATE study_sessions SET ended_at = ?, pending_review_id = 0, retry_item_id = 0 WHERE id = ? AND user_id = ? AND ended_at = 0`,
		ts(now), id, u.uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return &TransitionError{From: PhaseEnded, Event: EventEnd, Reason: "the session is already over"}
	}
	return nil
}

// Explainable reports whether a review may be explained on demand ("Ask
// Why?"): any answer the model judged, right, close or missed. An exact
// text's word-by-word diff already says what differed.
func (r Review) Explainable() bool {
	return r.Judge != "exact" && r.Verdict.Valid()
}

// ReviewCounts is the factual history of one item: how often it was
// answered for a grade, with what verdicts, and how many practice attempts
// ("Again") there were besides.
type ReviewCounts struct {
	Total, Right, Close, Missed int // scored reviews
	Practice                    int // attempts that did not move the schedule
}

// ReviewCounts counts the learner's reviews of one item.
func (u *UserStore) ReviewCounts(ctx context.Context, itemID int64) (ReviewCounts, error) {
	var c ReviewCounts
	err := u.s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(practice = 0), 0),
		       COALESCE(SUM(practice = 0 AND verdict = 'right'), 0),
		       COALESCE(SUM(practice = 0 AND verdict = 'close'), 0),
		       COALESCE(SUM(practice = 0 AND verdict = 'missed'), 0),
		       COALESCE(SUM(practice = 1), 0)
		FROM reviews WHERE item_id = ? AND user_id = ?`, itemID, u.uid).
		Scan(&c.Total, &c.Right, &c.Close, &c.Missed, &c.Practice)
	return c, err
}

// RecentVerdicts counts the learner's recent verdicts, for shaping new
// material to how they are doing.
func (u *UserStore) RecentVerdicts(ctx context.Context, since time.Time) (right, total int, err error) {
	err = u.s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(verdict = 'right'), 0), COUNT(*) FROM reviews WHERE user_id = ? AND practice = 0 AND at >= ?`,
		u.uid, ts(since)).Scan(&right, &total)
	return
}
