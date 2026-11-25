package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"scry/internal/store"
)

// Managing a card without leaving Study (S106-5).

// MaxIdeaTitleLen bounds an idea's name typed in Edit card.
const MaxIdeaTitleLen = 200

// EditCardIdea is "Edit card" with all three fields: the question, the
// correct answer and the idea the card sits in (an existing idea, matched
// whatever its case, or a new name; see store.EditItemIdea for which idea
// a name picks and where a new one goes). Like EditCard it writes only the
// card: its schedule, its history and the judgment on screen stay as they
// are. An empty or over-long field is a *FieldError naming that field and
// changes nothing; another learner's card, or a removed one, is
// store.ErrNotFound.
func (a *App) EditCardIdea(ctx context.Context, u store.User, itemID int64, prompt, answer, idea string) (store.Item, error) {
	us := a.Store.For(u.ID)
	it, err := us.Item(ctx, itemID) // scoped: another learner's item is not found
	if err != nil {
		return store.Item{}, err
	}
	prompt, answer, idea = strings.TrimSpace(prompt), strings.TrimSpace(answer), strings.Join(strings.Fields(idea), " ")
	switch {
	case prompt == "":
		return it, &FieldError{Field: "prompt", Msg: "The question can't be empty."}
	case answer == "":
		return it, &FieldError{Field: "answer", Msg: "The correct answer can't be empty."}
	case idea == "":
		return it, &FieldError{Field: "idea", Msg: "Name an idea for this card."}
	case len(prompt) > MaxPromptLen:
		return it, &FieldError{Field: "prompt", Msg: "That question is too long. Keep it under 1,000 characters."}
	case len(answer) > MaxAnswerLen:
		return it, &FieldError{Field: "answer", Msg: "That answer is too long. Keep it under 2,000 characters."}
	case len(idea) > MaxIdeaTitleLen:
		return it, &FieldError{Field: "idea", Msg: "That idea's name is too long. Keep it under 200 characters."}
	}
	if _, err := us.EditItemIdea(ctx, itemID, prompt, answer, idea, a.now()); err != nil {
		return it, err
	}
	return us.Item(ctx, itemID)
}

// SnoozeUntil is when a card snoozed now comes back: the start of the
// learner's next day, in their time zone.
func SnoozeUntil(u store.User, now time.Time) time.Time { return EndOfDay(u, now) }

// SnoozeCard puts one of the learner's cards away until tomorrow, in their
// time zone, and moves the session on.
//
// While the card waits for its answer, nothing is recorded: no review, and
// the schedule is untouched but for its due time, which moves to tomorrow
// if it was sooner. After the answer (in the post-answer review), the
// review already recorded stays, and the next review becomes the later of
// the scheduled one and tomorrow. Either way the card leaves the open
// session, which goes on to its next card or ends. A card that is not in
// the open session is snoozed all the same. Another learner's card is
// store.ErrNotFound; a card already removed changes nothing.
func (a *App) SnoozeCard(ctx context.Context, u store.User, itemID int64) error {
	us := a.Store.For(u.ID)
	del, err := us.ItemRemovedAt(ctx, itemID)
	if err != nil {
		return err
	}
	if del.IsZero() {
		if err := us.SnoozeItem(ctx, itemID, SnoozeUntil(u, a.now())); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	return a.moveOnFrom(ctx, us, itemID)
}

// DeleteCard removes one of the learner's cards (restorable for 30 days,
// in Settings → Recently removed, or at once with RestoreCard) and moves
// the session on, as Snooze does. Removing it again changes nothing.
// Another learner's card is store.ErrNotFound.
func (a *App) DeleteCard(ctx context.Context, u store.User, itemID int64) error {
	us := a.Store.For(u.ID)
	del, err := us.ItemRemovedAt(ctx, itemID)
	if err != nil {
		return err
	}
	if del.IsZero() {
		if err := us.RemoveItem(ctx, itemID, a.now()); err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	return a.moveOnFrom(ctx, us, itemID)
}

// RestoreCard brings a removed card back into its idea and study queues.
// Restoring one already in the library changes nothing; a card whose idea
// was removed comes back with the idea instead (store.ErrNotFound here).
func (a *App) RestoreCard(ctx context.Context, u store.User, itemID int64) error {
	return a.Store.For(u.ID).RestoreItem(ctx, itemID)
}

// moveOnFrom leaves the card the open session is showing, when it is this
// one: its post-answer review (or an exact text's retry) is cleared, as
// Next card does. A card waiting for its answer has already left the
// session (snoozed) or is skipped as removed. A session with nothing left
// is ended. A move another request made first is the same as no move.
func (a *App) moveOnFrom(ctx context.Context, us *store.UserStore, itemID int64) error {
	s, err := us.ActiveSession(ctx)
	if err != nil || s == nil {
		return err
	}
	showing := false
	switch s.Phase() {
	case store.PhasePendingReview:
		if r, err := us.Review(ctx, s.PendingReviewID); err == nil && r.ItemID == itemID {
			showing = true
		}
	case store.PhaseRetryingExact:
		showing = s.RetryItemID == itemID
	}
	if showing {
		if err := us.Advance(ctx, s.Observed(), 0); err != nil && !errors.Is(err, store.ErrIllegalTransition) {
			return err
		}
	}
	a.endIfDone(ctx, us)
	return nil
}
