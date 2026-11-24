package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"scry/internal/ai"
	"scry/internal/srs"
	"scry/internal/store"
)

// The Strugglometer (Slice 75). A card the learner keeps missing is
// "struggling", decided deterministically from what Scry has stored:
//
//   - its FSRS lapse count is StruggleLapses (3) or more, or
//   - its latest two graded reviews (practice attempts left out) are both
//     Incorrect.
//
// A struggling card that is not an exact text, not already held and not
// itself an active bridge is offered bridge cards: two or three atomic
// prerequisite cards, drafted by the model, edited by the learner, then
// added to the card's idea while the card is held out of every due queue.
// Each bridge's first Correct or Almost Correct ends it; when none is left
// the card is due again exactly when its own schedule says. The learner can
// release the hold by hand at any time.

// StruggleLapses is the lapse count at which a card is struggling.
const StruggleLapses = 3

// Struggling is the detection rule. recent holds the card's latest graded
// verdicts, newest first (practice attempts already left out).
func Struggling(lapses int, recent []srs.Verdict) bool {
	if lapses >= StruggleLapses {
		return true
	}
	return len(recent) >= 2 && recent[0] == srs.Missed && recent[1] == srs.Missed
}

// StruggleCue is the one line Scry shows beside a struggling card.
const StruggleCue = "Repeated miss — scaffold prerequisite bridges?"

// Errors of bridge cards.
var (
	// ErrNotStruggling is a request for bridges on a card that is not
	// offered them (not struggling, an exact text, held or itself a bridge).
	ErrNotStruggling = errors.New("that card isn't offered bridge cards")
	// ErrBridgesUnavailable means the model could not draft bridges just now.
	ErrBridgesUnavailable = errors.New("bridge cards are unavailable right now")
)

// Struggle is what Scry knows about one card's struggle and bridges.
type Struggle struct {
	Struggling bool  // the detection rule holds
	HeldBy     int   // active bridges holding it
	BridgeFor  int64 // the parent it is an active bridge for, or 0
	Exact      bool
}

// Offer reports whether the card is offered "Build bridge cards".
func (s Struggle) Offer() bool {
	return s.Struggling && !s.Exact && s.HeldBy == 0 && s.BridgeFor == 0
}

// recentVerdicts is the latest graded verdicts from a newest-first history.
func recentVerdicts(history []store.Review, n int) []srs.Verdict {
	var out []srs.Verdict
	for _, r := range history {
		if r.Practice || !r.Verdict.Valid() {
			continue
		}
		out = append(out, r.Verdict)
		if len(out) == n {
			break
		}
	}
	return out
}

// StruggleOf reads one card's struggle state from a newest-first history.
func StruggleOf(it store.Item, history []store.Review, bs store.BridgeState) Struggle {
	return Struggle{Struggling: Struggling(it.Sched.Lapses, recentVerdicts(history, 2)), HeldBy: bs.HeldBy, BridgeFor: bs.BridgeFor, Exact: it.Exact}
}

// Struggles reads the struggle state of many cards at once (a deck list).
func (a *App) Struggles(ctx context.Context, u store.User, items []store.Item) (map[int64]Struggle, error) {
	us := a.Store.For(u.ID)
	ids := make([]int64, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	latest, err := us.LatestVerdicts(ctx, ids, 2)
	if err != nil {
		return nil, err
	}
	states, err := us.BridgeStates(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]Struggle, len(items))
	for _, it := range items {
		var recent []srs.Verdict
		for _, v := range latest[it.ID] {
			recent = append(recent, srs.Verdict(v))
		}
		bs := states[it.ID]
		out[it.ID] = Struggle{Struggling: Struggling(it.Sched.Lapses, recent), HeldBy: bs.HeldBy, BridgeFor: bs.BridgeFor, Exact: it.Exact}
	}
	return out, nil
}

// StruggleFor reads one of the learner's own cards and its struggle state.
func (a *App) StruggleFor(ctx context.Context, u store.User, itemID int64) (store.Item, Struggle, error) {
	us := a.Store.For(u.ID)
	it, err := us.Item(ctx, itemID) // scoped: another learner's card is not found
	if err != nil {
		return store.Item{}, Struggle{}, err
	}
	history, err := us.History(ctx, itemID)
	if err != nil {
		return it, Struggle{}, err
	}
	bs, err := us.BridgeState(ctx, itemID)
	if err != nil {
		return it, Struggle{}, err
	}
	return it, StruggleOf(it, history, bs), nil
}

// Worst-case tokens of one bridge draft: the input cap plus instructions,
// and every output token.
const bridgeInTokens, bridgeOutTokens = ai.BridgeInputChars/2 + 1024, ai.BridgeMaxTokens

// BridgeBudget is what one bridge draft reserves with the configured model.
func (a *App) BridgeBudget() int64 {
	return BudgetMicros(ai.BridgeModel(a.Bridger), bridgeInTokens, bridgeOutTokens)
}

// bridgeMisses is how many recent incorrect answers the model is shown.
const bridgeMisses = 3

// DraftBridges asks the model for bridge cards for one of the learner's own
// struggling cards, with the card's question and answer and the learner's
// recent incorrect answers. Nothing is written to the library: the cards
// come back for the learner to edit, remove or accept. It is metered like
// drafting (reserved before the call, settled at the reported usage; an
// outcome unknown after the call keeps the hold, G2), refused for a card
// that is not offered bridges, and ErrAllowance / ErrPlanEnded when the
// plan cannot pay for it.
func (a *App) DraftBridges(ctx context.Context, u store.User, itemID int64) ([]ai.BridgeCard, error) {
	us := a.Store.For(u.ID)
	it, st, err := a.StruggleFor(ctx, u, itemID)
	if err != nil {
		return nil, err
	}
	if !st.Offer() {
		return nil, ErrNotStruggling
	}
	if a.Bridger == nil {
		return nil, ErrBridgesUnavailable
	}
	if _, err := a.Plan(ctx, u.ID); err != nil {
		return nil, err
	}
	history, err := us.History(ctx, itemID)
	if err != nil {
		return nil, err
	}
	in := ai.BridgeInput{Question: it.Prompt, Answer: it.Answer}
	for _, r := range history {
		if !r.Practice && r.Verdict == srs.Missed {
			in.Misses = append(in.Misses, r.Answer)
			if len(in.Misses) == bridgeMisses {
				break
			}
		}
	}
	reservation, err := a.reserve(ctx, u.ID, a.BridgeBudget())
	if err != nil {
		return nil, err
	}
	called := false
	decided := false
	defer func() {
		if r := recover(); r != nil {
			if !decided {
				if called {
					a.Log.WarnContext(ctx, "bridge provider outcome unknown; reservation held", "why", "panic during the bridge call", "user", u.ID, "item", itemID, "reservation", reservation)
				} else {
					a.settle(u.ID, reservation, "draft", ai.Usage{}, 0)
				}
			}
			panic(r)
		}
	}()
	bctx, cancel := context.WithTimeout(ctx, ai.BridgeTimeout)
	defer cancel()
	called = true
	cards, use, err := a.Bridger.Bridge(bctx, in)
	decided = true
	if use.Model != "" {
		a.settle(u.ID, reservation, "draft", use, 0)
	} else if err != nil {
		a.Log.WarnContext(ctx, "bridge provider outcome unknown; reservation held", "why", "bridge error with no usage", "user", u.ID, "item", itemID, "reservation", reservation)
	} else {
		a.settle(u.ID, reservation, "draft", use, 0)
	}
	if err != nil {
		a.Log.ErrorContext(ctx, "bridge draft failed", "err", err, "user", u.ID, "item", itemID)
		return nil, ErrBridgesUnavailable
	}
	if len(cards) < ai.MinBridges {
		return nil, ErrBridgesUnavailable
	}
	if len(cards) > ai.MaxBridges {
		cards = cards[:ai.MaxBridges]
	}
	return cards, nil
}

// acceptKey is the shape of a preview's accept key (BridgeKey).
var acceptKey = regexp.MustCompile(`^[A-Za-z0-9_-]{32}$`)

// BridgeKey makes the key of one bridge preview. Accepting the same
// preview twice applies it once.
func BridgeKey() string { return randomToken(24) }

// BridgeResult is what accepting bridges did.
type BridgeResult struct {
	Bridges   int
	Duplicate bool // the same preview was already accepted: nothing new was written
}

// AcceptBridges adds the learner's edited bridge cards (one to three of
// them) to the parent card's idea and holds the parent, atomically. Every
// field is checked first (a *FieldError says what to fix); the key must be
// a preview's; the parent must be the learner's own card in the library and
// offered bridges, unless this same preview was already accepted, which
// writes nothing again (store.AcceptBridges).
func (a *App) AcceptBridges(ctx context.Context, u store.User, parentID int64, key string, cards []store.BridgeCard) (BridgeResult, error) {
	if !acceptKey.MatchString(key) {
		return BridgeResult{}, fieldErr("bridges", "That preview has expired. Build the bridge cards again.")
	}
	var clean []store.BridgeCard
	for i, c := range cards {
		p, an := strings.Join(strings.Fields(c.Prompt), " "), strings.TrimSpace(c.Answer)
		switch {
		case p == "" && an == "":
			continue // a row emptied by the learner is a removed row
		case p == "":
			return BridgeResult{}, fieldErr("bridges", fmt.Sprintf("Bridge %d needs a question.", i+1))
		case an == "":
			return BridgeResult{}, fieldErr("bridges", fmt.Sprintf("Bridge %d needs a correct answer.", i+1))
		case len(p) > MaxPromptLen:
			return BridgeResult{}, fieldErr("bridges", fmt.Sprintf("Bridge %d's question is too long. Keep it under 1,000 characters.", i+1))
		case len(an) > MaxAnswerLen:
			return BridgeResult{}, fieldErr("bridges", fmt.Sprintf("Bridge %d's answer is too long. Keep it under 2,000 characters.", i+1))
		}
		clean = append(clean, store.BridgeCard{Prompt: p, Answer: an})
	}
	switch {
	case len(clean) == 0:
		return BridgeResult{}, fieldErr("bridges", "Keep at least one bridge card, or cancel.")
	case len(clean) > ai.MaxBridges:
		return BridgeResult{}, fieldErr("bridges", "Keep at most three bridge cards.")
	}
	_, st, err := a.StruggleFor(ctx, u, parentID)
	if err != nil {
		return BridgeResult{}, err
	}
	// A held parent may be this very preview, retried: the store tells.
	if !st.Offer() && st.HeldBy == 0 {
		return BridgeResult{}, ErrNotStruggling
	}
	res, err := a.Store.For(u.ID).AcceptBridges(ctx, parentID, key, clean, a.now())
	switch {
	case errors.Is(err, store.ErrAlreadyHeld), errors.Is(err, store.ErrIsBridge), errors.Is(err, store.ErrBridgeKeyReused):
		return BridgeResult{}, ErrNotStruggling
	case err != nil:
		return BridgeResult{}, err
	}
	return BridgeResult{Bridges: len(res.Bridges), Duplicate: res.Duplicate}, nil
}

// ReleaseHold releases a held card by hand. Its bridge cards stay in the
// library as ordinary cards; its own schedule is unchanged.
func (a *App) ReleaseHold(ctx context.Context, u store.User, parentID int64) (int, error) {
	return a.Store.For(u.ID).ReleaseHold(ctx, parentID, a.now())
}
