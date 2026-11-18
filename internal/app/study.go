package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

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
	Close []string    `json:"close,omitempty"` // parts of the answer that were close
	Ops   []exact.Op  `json:"ops,omitempty"`   // the word-for-word comparison of an exact text
	Diff  *exactCount `json:"diff,omitempty"`
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
		if s.Current() != nil || s.PendingReviewID != 0 {
			return s, nil
		}
		if err := us.EndSession(ctx, s.ID, a.now()); err != nil {
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
func (a *App) Answer(ctx context.Context, u store.User, itemID int64, answer, via string) (store.Review, error) {
	us := a.Store.For(u.ID)
	s, err := us.ActiveSession(ctx)
	if err != nil {
		return store.Review{}, err
	}
	if s == nil || s.PendingReviewID != 0 {
		return store.Review{}, ErrStale
	}
	cur := s.Current()
	if cur == nil || cur.Item.ID != itemID {
		return store.Review{}, ErrStale
	}
	item := cur.Item
	answer = strings.Join(strings.Fields(answer), " ")
	if len(answer) > 2000 {
		answer = answer[:2000]
	}
	if via != "spoken" {
		via = "typed"
	}
	now := a.now()
	r := store.Review{ItemID: item.ID, SessionID: s.ID, At: now, Answer: answer, Via: via, Practice: s.RetryItemID == item.ID}
	var detail Detail
	hold := true
	switch {
	case item.Exact:
		// The one exception to model judging: exact texts, word for word.
		d := exact.Diff(item.Answer, answer)
		r.Verdict, r.Reason, r.Judge = srs.Verdict(d.Verdict()), d.Why(), "exact"
		detail.Ops, detail.Diff = d.Ops, &exactCount{d.Missing, d.Swapped, d.Extra}
	case answer == "":
		// Nothing was offered, so there is nothing to judge.
		r.Verdict, r.Reason, r.Judge = srs.Missed, "You gave no answer.", "none"
	default:
		_, err := a.Plan(ctx, u.ID)
		if err != nil {
			return store.Review{}, err
		}
		reservation, err := a.reserve(ctx, u.ID, ai.JudgeBudgetMicros)
		if err != nil {
			return store.Review{}, err
		}
		defer a.settle(u.ID, reservation, "judge", ai.Usage{}, 0)
		jctx, cancel := context.WithTimeout(ctx, 25*time.Second)
		j, use, err := a.Model.Judge(jctx, ai.JudgeInput{Question: item.Prompt, Expected: item.Answer, Answer: answer})
		cancel()
		if use.Model != "" {
			a.settle(u.ID, reservation, "judge", use, 0)
		}
		if err != nil || !j.Verdict.Valid() || strings.TrimSpace(j.Reason) == "" {
			if err == nil {
				err = errors.New("judgment had no verdict or no reason")
			}
			a.Log.ErrorContext(ctx, "judge failed", "err", err, "user", u.ID, "item", item.ID)
			a.Alert.Notify(ctx, "Answers cannot be judged", err.Error())
			return store.Review{}, ErrJudgeUnavailable
		}
		r.Verdict, r.Reason, r.Judge = j.Verdict, strings.Join(strings.Fields(j.Reason), " "), use.Model
		detail.Close = j.Close
		hold = j.Verdict != srs.Right
	}
	raw, _ := json.Marshal(detail)
	r.Detail = string(raw)
	next := srs.Next(item.Sched, r.Verdict, now)
	id, err := us.RecordReview(ctx, r, next, hold)
	if err != nil {
		return store.Review{}, err
	}
	r.ID = id
	if !hold {
		// Nothing is waiting on screen; if that was the last item, the session is over.
		if s, err := us.ActiveSession(ctx); err == nil && s != nil && s.Current() == nil {
			_ = us.EndSession(ctx, s.ID, a.now())
		}
	}
	return r, nil
}

// Next moves on from the judgment on screen. With again set, an exact
// text is put back up for another attempt; the retry is practice and
// does not change when the item is next due.
func (a *App) Next(ctx context.Context, u store.User, again bool) error {
	us := a.Store.For(u.ID)
	s, err := us.ActiveSession(ctx)
	if err != nil || s == nil {
		return err
	}
	var retry int64
	if again && s.PendingReviewID != 0 {
		if r, err := us.Review(ctx, s.PendingReviewID); err == nil {
			if it, err := us.Item(ctx, r.ItemID); err == nil && it.Exact {
				retry = it.ID
			}
		}
	}
	if err := us.Advance(ctx, s.ID, retry); err != nil {
		return err
	}
	if retry == 0 {
		if s, err = us.ActiveSession(ctx); err == nil && s != nil && s.Current() == nil {
			return us.EndSession(ctx, s.ID, a.now())
		}
	}
	return err
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
