package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"scry/internal/ai"
	"scry/internal/exact"
	"scry/internal/srs"
	"scry/internal/store"
)

// ErrJudgeUnavailable means the model could not be reached. Scry never
// falls back to guessing: the answer is kept and nothing is recorded.
var ErrJudgeUnavailable = errors.New("the judge is unavailable")

// ErrStale means the answer was for an item that is no longer current
// (a double submit, or a second tab).
var ErrStale = errors.New("that item is no longer the current one")

// Detail is what a review keeps beyond verdict and reason.
type Detail struct {
	Ops  []exact.Op  `json:"ops,omitempty"` // the word-for-word comparison of an exact text
	Diff *exactCount `json:"diff,omitempty"`
}

type exactCount struct{ Missing, Swapped, Extra int }

// ParseDetail reads a review's detail.
func ParseDetail(r store.Review) Detail {
	var d Detail
	_ = json.Unmarshal([]byte(r.Detail), &d)
	return d
}

const (
	itemSeconds  = 20 // a typical question, read and answered
	exactSeconds = 30 // a line of an exact text
)

// Study returns the learner's open session, or starts one from what is
// due today, sized to the session budget (about five minutes). It
// returns nil when nothing is due.
func (a *App) Study(ctx context.Context, u store.User) (*store.StudySession, error) {
	us := a.Store.For(u.ID)
	s, err := us.ActiveSession(ctx)
	if err != nil {
		return nil, err
	}
	if s != nil {
		if s.Current() != nil || s.Phase() != store.PhaseAsking {
			return s, nil
		}
		if err := us.EndSession(ctx, s.ID, a.now()); err != nil && !errors.Is(err, store.ErrIllegalTransition) {
			return nil, err
		}
	}
	due, err := us.DueItems(ctx, EndOfDay(u, a.now()), 200)
	if err != nil || len(due) == 0 {
		return nil, err
	}
	budget := a.Cfg.SessionBudget.Seconds()
	var ids []int64
	spent := 0.0
	for _, it := range due {
		cost := float64(itemSeconds)
		if it.Exact {
			cost = exactSeconds
		}
		if len(ids) > 0 && spent+cost > budget {
			break
		}
		ids = append(ids, it.ID)
		spent += cost
	}
	if _, err := us.StartSession(ctx, ids, a.now()); err != nil {
		return nil, err
	}
	return us.ActiveSession(ctx)
}

// Answer judges the learner's answer to the current item and records it.
//
// The learner never grades themself. A text marked exact is compared
// word for word; every other answer goes to the model, which returns a
// verdict and a one-line reason. If the model cannot be reached the
// answer is not judged at all.
//
// Answering is a session transition (see store.StudySession): a scored
// answer is taken only in PhaseAsking for the current item, a practice
// attempt only in PhaseRetryingExact for the exact text being retried.
// The store checks the same guard again inside the write transaction that
// records the review, so a double submit, a second tab or an answer racing
// the end of the session is ErrStale and never reschedules an item twice.
func (a *App) Answer(ctx context.Context, u store.User, itemID int64, answer, via string) (store.Review, error) {
	us := a.Store.For(u.ID)
	s, err := us.ActiveSession(ctx)
	if err != nil {
		return store.Review{}, err
	}
	if s == nil {
		return store.Review{}, ErrStale
	}
	phase := s.Phase()
	if phase != store.PhaseAsking && phase != store.PhaseRetryingExact {
		return store.Review{}, ErrStale
	}
	cur := s.Current()
	if cur == nil || cur.Item.ID != itemID {
		return store.Review{}, ErrStale
	}
	item := cur.Item
	practice := phase == store.PhaseRetryingExact
	if practice {
		// A retry is practice at an exact text. If the item has since been
		// removed or edited so it is no longer exact, there is nothing to
		// practise word for word: the retry is dropped (never judged by the
		// model as "practice") and the learner moves on.
		if it, err := us.Item(ctx, item.ID); err != nil || !it.Exact {
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return store.Review{}, err
			}
			if err := us.Advance(ctx, s.ID, 0); err != nil && !errors.Is(err, store.ErrIllegalTransition) {
				return store.Review{}, err
			}
			a.endIfDone(ctx, us)
			return store.Review{}, ErrStale
		}
	}
	answer = strings.Join(strings.Fields(answer), " ")
	if len(answer) > 2000 {
		end := 2000
		for end > 0 && !utf8.RuneStart(answer[end]) {
			end--
		}
		answer = answer[:end]
	}
	if via != "spoken" {
		via = "typed"
	}
	now := a.now()
	// The graded question and expected answer are kept with the verdict, so
	// "Why?" explains this judgment even if the item is edited afterwards.
	r := store.Review{ItemID: item.ID, SessionID: s.ID, At: now, Answer: answer, Via: via, Practice: practice,
		GradedPrompt: item.Prompt, GradedExpected: item.Answer}
	var detail Detail
	hold := true
	switch {
	case item.Exact:
		// The one exception to model judging: exact texts, word for word.
		d := exact.Diff(item.Answer, answer)
		r.Verdict, r.Reason, r.Judge = srs.Verdict(d.Verdict()), d.Why(), "exact"
		detail.Ops, detail.Diff = d.Ops, &exactCount{d.Missing, d.Swapped, d.Extra}
	default:
		_, err := a.Plan(ctx, u.ID)
		if err != nil {
			return store.Review{}, err
		}
		reservation, err := a.reserve(ctx, u.ID, ai.JudgeBudgetMicros)
		if err != nil {
			return store.Review{}, err
		}
		jctx, cancel := context.WithTimeout(ctx, 25*time.Second)
		j, use, err := a.Model.Judge(jctx, ai.JudgeInput{Question: item.Prompt, Expected: item.Answer, Answer: answer})
		cancel()
		// Always settled: what the call used is metered, and a call that
		// failed before using anything releases its reservation instead of
		// holding the allowance forever.
		a.settle(u.ID, reservation, "judge", use, 0)
		if err != nil || !j.Verdict.Valid() || strings.TrimSpace(j.Reason) == "" {
			if err == nil {
				err = errors.New("judgment had no verdict or no reason")
			}
			a.Log.ErrorContext(ctx, "judge failed", "err", err, "user", u.ID, "item", item.ID)
			a.Alert.Notify(ctx, "Answers cannot be judged", err.Error())
			return store.Review{}, ErrJudgeUnavailable
		}
		r.Verdict, r.Reason, r.Judge = j.Verdict, strings.Join(strings.Fields(j.Reason), " "), use.Model
		hold = j.Verdict != srs.Right
	}
	raw, _ := json.Marshal(detail)
	r.Detail = string(raw)
	next := item.Sched
	if !practice {
		next, err = srs.NextChecked(item.Sched, r.Verdict, now)
		if errors.Is(err, srs.ErrInvalidState) {
			// The stored schedule is outside the model's bounds. Say so
			// loudly, and reschedule from a repaired state rather than
			// refusing the learner's answer.
			a.Log.ErrorContext(ctx, "item schedule out of bounds; repaired", "err", err, "user", u.ID, "item", item.ID)
			a.Alert.Notify(ctx, "An item's schedule was out of bounds", err.Error())
			next, err = srs.Next(item.Sched, r.Verdict, now), nil
		}
		if err != nil {
			return store.Review{}, err
		}
	}
	id, err := us.RecordReview(ctx, r, next, hold)
	if errors.Is(err, store.ErrIllegalTransition) {
		return store.Review{}, ErrStale
	}
	if err != nil {
		return store.Review{}, err
	}
	r.ID = id
	if !hold {
		// Nothing is waiting on screen; if that was the last item, the session is over.
		a.endIfDone(ctx, us)
	}
	return r, nil
}

// endIfDone ends the open session once nothing is left to answer and no
// judgment is on screen. A session another request ended first is fine.
func (a *App) endIfDone(ctx context.Context, us *store.UserStore) {
	s, err := us.ActiveSession(ctx)
	if err != nil || s == nil || s.Phase() != store.PhaseAsking || s.Current() != nil {
		return
	}
	if err := us.EndSession(ctx, s.ID, a.now()); err != nil && !errors.Is(err, store.ErrIllegalTransition) {
		a.Log.ErrorContext(ctx, "ending a finished session failed", "err", err, "session", s.ID)
	}
}

// Next moves on from the judgment on screen. With again set, an exact
// text is put back up for another attempt; the retry is practice and
// does not change when the item is next due.
//
// From PhasePendingReview it advances (or, with again on an exact text,
// retries). From PhaseRetryingExact, without again, it skips the retry;
// again while already retrying changes nothing. In PhaseAsking there is
// nothing on screen to move on from (a repeated Enter, a second tab): it
// changes nothing, except that a session with nothing left is ended. A
// move the store refuses because another request got there first is the
// same as no move.
func (a *App) Next(ctx context.Context, u store.User, again bool) error {
	us := a.Store.For(u.ID)
	s, err := us.ActiveSession(ctx)
	if err != nil || s == nil {
		return err
	}
	var retry int64
	switch s.Phase() {
	case store.PhasePendingReview:
		if again {
			if r, err := us.Review(ctx, s.PendingReviewID); err == nil {
				// Only a word-for-word judgment offers "Again" (the item may
				// have been edited to exact after a model judged it).
				if it, err := us.Item(ctx, r.ItemID); err == nil && it.Exact && r.Judge == "exact" {
					retry = it.ID
				}
			}
		}
	case store.PhaseRetryingExact:
		if again {
			return nil
		}
	default:
		a.endIfDone(ctx, us)
		return nil
	}
	if err := us.Advance(ctx, s.ID, retry); err != nil {
		if errors.Is(err, store.ErrIllegalTransition) {
			return nil
		}
		return err
	}
	if retry == 0 {
		a.endIfDone(ctx, us)
	}
	return nil
}

// Summary is the one line a finished session leaves behind.
type Summary struct {
	Reviewed int
	Tomorrow int // items that come back tomorrow
}

// Summarize counts what a session did.
func (a *App) Summarize(u store.User, s *store.StudySession) Summary {
	var sum Summary
	if s == nil {
		return sum
	}
	end := s.Ended
	if end.IsZero() {
		end = a.now()
	}
	tomorrow := EndOfDay(u, end).Add(24 * time.Hour)
	for _, e := range s.Entries {
		if e.Review == nil {
			continue
		}
		sum.Reviewed++
		if e.Item.Sched.Due.Before(tomorrow) {
			sum.Tomorrow++
		}
	}
	return sum
}
