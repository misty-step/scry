package app

import (
	"context"
	"errors"
	"strings"
	"sync"

	"scry/internal/ai"
	"scry/internal/srs"
	"scry/internal/store"
)

// ErrNotExplainable is a review that has nothing to explain on demand: a
// right answer, or an exact text (whose word-by-word diff already says
// what differed).
var ErrNotExplainable = errors.New("only close and missed model-judged answers can be explained")

// Explanation is what the learner sees after asking "Why?".
type Explanation struct {
	Text     string
	Fallback bool // the explainer was unavailable; Text is the fixed criterion sentence
}

// explainCache keeps recent explanations so a second click costs nothing.
type explainCache struct {
	mu sync.Mutex
	m  map[int64]string
}

func (c *explainCache) get(id int64) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.m[id]
	return s, ok
}

func (c *explainCache) put(id int64, s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil || len(c.m) > 2000 {
		c.m = map[int64]string{}
	}
	c.m[id] = s
}

// Explain writes the on-demand "what differed" explanation of one of the
// learner's own judged answers. It runs strictly after the verdict and only
// when asked: it reads the review and never writes it or its item, so the
// recorded grade, review row and scheduling cannot change. It explains the
// question and expected answer snapshotted on the review when it was
// graded, never the item's current text, which may have been edited since. It is
// bounded (reserved budget, timeout, output length) and falls back to the
// review's fixed criterion sentence whenever the explainer is missing,
// slow, failing or out of allowance; it never fails the learner outright.
func (a *App) Explain(ctx context.Context, u store.User, reviewID int64) (Explanation, error) {
	us := a.Store.For(u.ID)
	r, err := us.Review(ctx, reviewID) // scoped: another learner's review is not found
	if err != nil {
		return Explanation{}, err
	}
	if r.Judge == "exact" || (r.Verdict != srs.Close && r.Verdict != srs.Missed) {
		return Explanation{}, ErrNotExplainable
	}
	fallback := Explanation{Text: r.Reason, Fallback: true}
	if fallback.Text == "" {
		fallback.Text = ai.CriterionSentence(r.Verdict)
	}
	if text, ok := a.explained.get(r.ID); ok {
		return Explanation{Text: text}, nil
	}
	if a.Explainer == nil {
		return fallback, nil
	}
	if r.GradedPrompt == "" && r.GradedExpected == "" {
		// Recorded before graded inputs were kept: the item may have been
		// edited since, so only the verdict's own sentence is certain.
		return fallback, nil
	}
	reservation, err := a.reserve(ctx, u.ID, ai.ExplainBudgetMicros)
	if err != nil {
		if !errors.Is(err, ErrAllowance) {
			a.Log.ErrorContext(ctx, "explanation reservation failed", "err", err, "user", u.ID)
		}
		return fallback, nil
	}
	timeout := a.Cfg.ExplainTimeout
	if timeout <= 0 {
		timeout = ai.ExplainTimeout
	}
	ectx, cancel := context.WithTimeout(ctx, timeout)
	text, use, err := a.Explainer.Explain(ectx, ai.ExplainInput{Question: r.GradedPrompt, Expected: r.GradedExpected, Answer: r.Answer, Verdict: r.Verdict})
	cancel()
	// Metered with judging, which it belongs to; an unused reservation is released.
	a.settle(u.ID, reservation, "judge", use, 0)
	text = ai.ClampExplanation(text)
	if err != nil || strings.TrimSpace(text) == "" {
		a.Log.WarnContext(ctx, "explanation unavailable; showing the criterion sentence", "err", err, "user", u.ID, "review", r.ID)
		return fallback, nil
	}
	a.explained.put(r.ID, text)
	return Explanation{Text: text}, nil
}
