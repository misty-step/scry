package app

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"scry/internal/store"
)

// [S111-1] Punch in and study: one interleaved session across every deck.
//
// Study never asks for a deck. It plans one session over every active
// deck (each top-level idea or starter set), sized to the session budget:
//
//  1. Room goes to due reviews first, most urgent first: lowest current
//     FSRS retrievability, then earliest due, from at most
//     ReviewCandidates of them.
//  2. Only the room left takes new cards (never reviewed, their time
//     come), oldest first, while the day's DailyNewCards first
//     presentations, counted across every session in the learner's time
//     zone, allow.
//  3. The chosen cards are ordered: next is the most urgent due review
//     from a deck other than the last card's, else the oldest new card
//     from another deck, else (no other deck has a card left) the last
//     deck again, due reviews first. The lines of an exact text keep
//     their order.
//
// A new card only ever fills room the reviews left, so one used to break a
// streak never takes a review's place.
const (
	// ReviewCandidates is how many due reviews a session plan considers.
	ReviewCandidates = 200
	// DailyNewCards is how many new cards may be presented in a day.
	DailyNewCards = 10
	// newCandidates is how many new cards a plan reads to fill its room.
	newCandidates = 200
)

// LearnerDay is the learner's local date, the key of the daily limit.
func LearnerDay(u store.User, now time.Time) string {
	return now.In(u.Location()).Format("2006-01-02")
}

// cardCost is what a card spends of the session budget, in seconds.
func cardCost(it store.Item) float64 {
	if it.Exact {
		return exactSeconds
	}
	return itemSeconds
}

// planned is one card of a session plan.
type planned struct {
	store.StudyCandidate
	review bool
}

// exactGroup names the exact text a line belongs to; "" for any other card.
func exactGroup(it store.Item) (string, bool) {
	if !it.Exact || it.LineNo <= 0 {
		return "", false
	}
	return strconv.FormatInt(it.IdeaID, 10) + "\x00" + it.TextTitle, true
}

// planSession chooses and orders a session's cards (see the package
// comment above). reviews and fresh are the candidates as the store lists
// them; newLeft is how many first presentations the day still allows, and
// presented the new cards already presented today, which cost none.
func planSession(reviews, fresh []store.StudyCandidate, now time.Time, budget float64, newLeft int, presented map[int64]int64) []planned {
	urgent := append([]store.StudyCandidate(nil), reviews...)
	sort.SliceStable(urgent, func(i, j int) bool {
		ri, rj := urgent[i].Sched.RetrievabilityAt(now), urgent[j].Sched.RetrievabilityAt(now)
		if ri != rj {
			return ri < rj
		}
		if !urgent[i].Sched.Due.Equal(urgent[j].Sched.Due) {
			return urgent[i].Sched.Due.Before(urgent[j].Sched.Due)
		}
		return urgent[i].ID < urgent[j].ID
	})
	spent := 0.0
	fits := func(it store.Item, n int) bool {
		c := cardCost(it)
		// The first card always fits, so a budget smaller than one card
		// still studies something.
		if n > 0 && spent+c > budget {
			return false
		}
		spent += c
		return true
	}
	var revs, news []planned
	for _, c := range urgent {
		if fits(c.Item, len(revs)) {
			revs = append(revs, planned{c, true})
		}
	}
	for _, c := range fresh {
		_, free := presented[c.ID]
		if !free && newLeft <= 0 {
			continue
		}
		if fits(c.Item, len(revs)+len(news)) {
			news = append(news, planned{c, false})
			if !free {
				newLeft--
			}
		}
	}
	return interleave(revs, news)
}

// interleave orders the chosen cards: due reviews by urgency and new cards
// oldest first, alternating decks whenever another deck has a card left,
// due reviews before new cards. A line of an exact text is always the
// earliest line of that text still to come.
func interleave(revs, news []planned) []planned {
	out := make([]planned, 0, len(revs)+len(news))
	var last int64
	first := true
	pick := func(pool []planned, sameDeck bool) int {
		for i, c := range pool {
			if first || (c.Deck == last) == sameDeck {
				return i
			}
		}
		return -1
	}
	take := func(pool *[]planned, i int) planned {
		c := (*pool)[i]
		*pool = append((*pool)[:i:i], (*pool)[i+1:]...)
		return c
	}
	for len(revs)+len(news) > 0 {
		var c planned
		switch {
		case pick(revs, false) >= 0:
			c = take(&revs, pick(revs, false))
		case pick(news, false) >= 0:
			c = take(&news, pick(news, false))
		case len(revs) > 0:
			c = take(&revs, 0)
		default:
			c = take(&news, 0)
		}
		// The lines of an exact text keep their order: the slot this line
		// won goes to the earliest line of its text still to come.
		if g, ok := exactGroup(c.Item); ok {
			c = earliestLine(c, g, &revs, &news)
		}
		out = append(out, c)
		last, first = c.Deck, false
	}
	return out
}

// earliestLine swaps c for the earliest line of its exact text still in
// the pools, putting c back in its pool.
func earliestLine(c planned, group string, revs, news *[]planned) planned {
	best, pool, at := c, (*[]planned)(nil), -1
	for _, p := range []*[]planned{revs, news} {
		for i, o := range *p {
			if g, ok := exactGroup(o.Item); ok && g == group && o.LineNo < best.LineNo {
				best, pool, at = o, p, i
			}
		}
	}
	if pool == nil {
		return c
	}
	(*pool)[at] = c
	return best
}

// Study returns the learner's open session, or starts one across every
// deck ([S111-1]). It returns nil when no card is eligible.
func (a *App) Study(ctx context.Context, u store.User) (*store.StudySession, error) {
	return a.study(ctx, u, 0, false)
}

// StudyAll is "Study all decks": it leaves a focused session for one
// across every deck. A session already across every deck goes on.
func (a *App) StudyAll(ctx context.Context, u store.User) (*store.StudySession, error) {
	return a.study(ctx, u, 0, true)
}

// StudyIdea is "Study this idea" in Library: a focused session over only
// that idea and the ideas inside it, under the same rules. A session
// already focused on it goes on. Another learner's idea, or one removed, is
// store.ErrNotFound. With nothing eligible in the idea it returns nil and
// leaves the open session as it was.
func (a *App) StudyIdea(ctx context.Context, u store.User, ideaID int64) (*store.StudySession, error) {
	if _, err := a.Store.For(u.ID).Idea(ctx, ideaID); err != nil {
		return nil, err
	}
	return a.study(ctx, u, ideaID, true)
}

// study goes on with the open session or plans a new one over scope (0:
// every deck). switching asks for a session over scope: an open session
// over another scope is replaced (when anything in scope is eligible);
// without it the open session goes on whatever its scope, so every
// redirect back to /study keeps a focused session going.
func (a *App) study(ctx context.Context, u store.User, scope int64, switching bool) (*store.StudySession, error) {
	us := a.Store.For(u.ID)
	s, err := us.ActiveSession(ctx)
	if err != nil {
		return nil, err
	}
	var replacing *store.StudySession
	if s != nil {
		live := s.Current() != nil || s.Phase() != store.PhaseAsking
		if live && (!switching || s.Scope == scope) {
			return s, nil
		}
		if live {
			replacing = s
		} else if err := us.EndSession(ctx, s.ID, a.now()); err != nil && !errors.Is(err, store.ErrIllegalTransition) {
			return nil, err
		}
	}
	now := a.now()
	reviews, fresh, err := us.StudyCandidates(ctx, EndOfDay(u, now), scope, ReviewCandidates, newCandidates)
	if err != nil {
		return nil, err
	}
	day := LearnerDay(u, now)
	presented, err := us.PresentedOn(ctx, day)
	if err != nil {
		return nil, err
	}
	if replacing != nil {
		// The session being replaced gives back the new cards it placed
		// that were never on screen (StartStudy releases them in the same
		// transaction), so the plan may use that room.
		shown := int64(0)
		if replacing.Phase() == store.PhaseAsking {
			if c := replacing.Current(); c != nil {
				shown = c.Item.ID
			}
		}
		for _, e := range replacing.Entries {
			if e.Review == nil && e.Item.ID != shown && presented[e.Item.ID] == replacing.ID {
				delete(presented, e.Item.ID)
			}
		}
	}
	plan := planSession(reviews, fresh, now, a.Cfg.SessionBudget.Seconds(), DailyNewCards-len(presented), presented)
	if len(plan) == 0 {
		return nil, nil
	}
	p := store.SessionPlan{New: map[int64]bool{}, Day: day, DailyNew: DailyNewCards, Scope: scope, Now: now, RequireOne: true}
	for _, c := range plan {
		p.Items = append(p.Items, c.ID)
		if !c.review {
			p.New[c.ID] = true
		}
	}
	if _, err := us.StartStudy(ctx, p); err != nil {
		if errors.Is(err, store.ErrNothingToStudy) {
			return nil, nil
		}
		return nil, err
	}
	return us.ActiveSession(ctx)
}
