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

// Answer judges the learner's answer to the current item and records it.
//
// The learner never grades themself. A text marked exact is compared
// word for word; every other answer goes to the model, which returns a
// verdict and a one-line reason. If the model cannot be reached the
// answer is not judged at all.
//
// [S113-1] Models only: a non-exact answer always reaches the grading
// chain (ai.Chain in production and in test mode), even one identical to
// the expected answer; there is no string-equality or other shortcut. One
// submission is one reservation, sized for the whole chain (JudgeBudget),
// settled once with the usage of every model it asked. ErrJudgeUnavailable
// is returned only when no model in the chain nor the escalation gave a
// usable verdict.
//
// Answering is a session transition (see store.StudySession): a scored
// answer is taken only in PhaseAsking for the current item, a practice
// attempt only in PhaseRetryingExact for the exact text being retried.
// The store checks the same guard again inside the write transaction that
// records the review, so a double submit, a second tab or an answer racing
// the end of the session is ErrStale and never reschedules an item twice.
//
// Every recorded answer, right ones included, leaves the session in
// PhasePendingReview: the post-answer review stays on screen until the
// learner presses Next. Answer never advances or ends the session itself.
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
			if err := us.Advance(ctx, s.Observed(), 0); err != nil && !errors.Is(err, store.ErrIllegalTransition) {
				return store.Review{}, err
			}
			a.endIfDone(ctx, us)
			return store.Review{}, ErrStale
		}
	}
	answer = NormalizeAnswer(answer)
	if via != "spoken" {
		via = "typed"
	}
	now := a.now()
	// The graded question and expected answer are kept with the verdict, so
	// "Why?" explains this judgment even if the item is edited afterwards.
	r := store.Review{ItemID: item.ID, SessionID: s.ID, At: now, Answer: answer, Via: via, Practice: practice,
		GradedPrompt: item.Prompt, GradedExpected: item.Answer}
	var detail Detail
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
		reservation, err := a.reserve(ctx, u.ID, a.JudgeBudget())
		if err != nil {
			return store.Review{}, err
		}
		j, use, err := a.judge(ctx, u.ID, item.ID, reservation, ai.JudgeInput{Question: item.Prompt, Expected: item.Answer, Answer: answer})
		if err != nil || !j.Verdict.Valid() || strings.TrimSpace(j.Reason) == "" {
			if err == nil {
				err = errors.New("judgment had no verdict or no reason")
			}
			a.Log.ErrorContext(ctx, "judge failed", "err", err, "user", u.ID, "item", item.ID)
			a.Alert.Notify(ctx, "Answers cannot be judged", err.Error())
			return store.Review{}, ErrJudgeUnavailable
		}
		r.Verdict, r.Reason, r.Judge = j.Verdict, strings.Join(strings.Fields(j.Reason), " "), use.Model
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
	// Held: the review of this answer is what the learner sees next.
	id, err := us.RecordReview(ctx, r, next, true)
	if errors.Is(err, store.ErrIllegalTransition) {
		return store.Review{}, ErrStale
	}
	if err != nil {
		return store.Review{}, err
	}
	r.ID = id
	return r, nil
}

// NormalizeAnswer tidies a submitted answer while keeping the line breaks
// the learner typed, pasted or dictated (S111-2): \r\n and \r become \n,
// spaces and tabs within a line collapse to one space, more than one blank
// line between paragraphs becomes one, the whole is trimmed, and it is cut
// at 2,000 bytes on a rune boundary. Nothing here judges the answer; an
// exact text's word diff still treats a line break as any other space.
func NormalizeAnswer(answer string) string {
	answer = strings.ReplaceAll(answer, "\r\n", "\n")
	answer = strings.ReplaceAll(answer, "\r", "\n")
	lines := strings.Split(answer, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, l := range lines {
		l = strings.Join(strings.Fields(l), " ")
		if l == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	answer = strings.Trim(strings.Join(out, "\n"), "\n")
	if len(answer) > MaxAnswerLen {
		end := MaxAnswerLen
		for end > 0 && !utf8.RuneStart(answer[end]) {
			end--
		}
		answer = strings.TrimRight(answer[:end], " \n")
	}
	return answer
}

// Limits on a card's text, shared by every way of writing one.
const (
	MaxPromptLen = 1000
	MaxAnswerLen = 2000
)

// EditCard changes the question and the correct answer of one of the
// learner's own cards, from the post-answer review or anywhere else. It
// writes only the item's text: whether the card is judged word for word,
// its schedule, its history and the judgment on screen all stay as they
// are, and a review already recorded keeps the text it was graded against
// (store.Review.GradedPrompt and GradedExpected), so the edit applies from
// the next review. Another learner's card, or one that was removed, is
// store.ErrNotFound; an empty or over-long question or answer is a
// *FieldError and changes nothing.
func (a *App) EditCard(ctx context.Context, u store.User, itemID int64, prompt, answer string) (store.Item, error) {
	us := a.Store.For(u.ID)
	it, err := us.Item(ctx, itemID) // scoped: another learner's item is not found
	if err != nil {
		return store.Item{}, err
	}
	prompt, answer = strings.TrimSpace(prompt), strings.TrimSpace(answer)
	switch {
	case prompt == "" && answer == "":
		return it, &FieldError{Field: "prompt", Msg: "A card needs both a question and a correct answer."}
	case prompt == "":
		return it, &FieldError{Field: "prompt", Msg: "The question can't be empty."}
	case answer == "":
		return it, &FieldError{Field: "answer", Msg: "The correct answer can't be empty."}
	case len(prompt) > MaxPromptLen:
		return it, &FieldError{Field: "prompt", Msg: "That question is too long. Keep it under 1,000 characters."}
	case len(answer) > MaxAnswerLen:
		return it, &FieldError{Field: "answer", Msg: "That answer is too long. Keep it under 2,000 characters."}
	}
	if err := us.UpdateItem(ctx, itemID, prompt, answer, it.Exact); err != nil {
		return it, err
	}
	return us.Item(ctx, itemID)
}

// judge calls the model on a reserved budget and decides the reservation
// exactly once on every path, as runDraft does (G2, see NOTES.md):
//   - the model reported usage (a judgment, or a refusal it billed or
//     reported at zero cost): settled at that cost;
//   - the model was called and failed with no usage, or panicked: the
//     outcome is unknown (the request may have been sent and billed, for
//     example when the response is lost after the request went out), so
//     the hold is kept, never forgiven, until an operator reconciles it or
//     the TTL sweep or the startup sweep releases it. A retry therefore
//     cannot spend unmetered;
//   - a panic before the model is called: released.
func (a *App) judge(ctx context.Context, userID, itemID int64, reservation string, in ai.JudgeInput) (ai.Judgment, ai.Usage, error) {
	called := false
	defer func() {
		if r := recover(); r != nil {
			if called {
				a.holdUnknown(ctx, userID, itemID, reservation, "panic during the judge call")
			} else {
				a.settle(userID, reservation, "judge", ai.Usage{}, 0)
			}
			panic(r)
		}
	}()
	// The whole grading chain runs inside this deadline: each decisions
	// model has 8 s and the escalation 15 s (ai.Chain), well under the
	// server's 90 s write timeout.
	jctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	called = true
	j, use, err := a.Model.Judge(jctx, in)
	if err != nil && use.Model == "" {
		a.holdUnknown(ctx, userID, itemID, reservation, "judge error with no usage")
		return j, use, err
	}
	a.settle(userID, reservation, "judge", use, 0)
	return j, use, err
}

func (a *App) holdUnknown(ctx context.Context, userID, itemID int64, reservation, why string) {
	a.Log.WarnContext(ctx, "judge provider outcome unknown; reservation held", "why", why, "user", userID, "item", itemID, "reservation", reservation)
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
	// The store moves the session on only if it is still as read here, so a
	// delayed Next can never dismiss a judgment another tab put on screen
	// since; that refusal is the same as no move.
	if err := us.Advance(ctx, s.Observed(), retry); err != nil {
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
