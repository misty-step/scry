package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// [S113-1] One ordered grading chain, models only. A non-exact answer goes
// to the primary decisions model (Jev); if it is unavailable, errors or
// returns something that cannot be read, to the next decisions model in
// the fallback chain, in order; as soon as any decisions model answers
// with low certainty (below its confidence, probability or margin bars, or
// "none of these"), or the whole chain gives no confident verdict, the
// answer is escalated once to a fast, capable LLM with the same question,
// accepted answers, rubric and three-way output. A low-certainty verdict
// goes straight to the LLM: it is never shopped to another decisions
// model. No step ever matches the answer text itself.

// The two kinds of "no verdict" a decisions model can give, and why the
// chain treats them differently.
var (
	// ErrDecisionUnavailable: the model could not be reached, errored, or
	// returned something that cannot be read. The chain asks the next
	// decisions model.
	ErrDecisionUnavailable = errors.New("decision unavailable")
	// ErrDecisionUncertain: the model answered, below its certainty bars or
	// with "none of these". The chain escalates to the LLM at once.
	ErrDecisionUncertain = errors.New("decision uncertain")
	// ErrNotSent marks an unavailable step that failed before any request
	// left (a missing key, an over-budget state): it cost nothing.
	ErrNotSent = errors.New("request not sent")
	// ErrNoVerdict: no model in the chain nor the escalation returned a
	// usable verdict.
	ErrNoVerdict = errors.New("no model in the grading chain returned a usable verdict")
)

func unavailable(msg string) error { return fmt.Errorf("%w: %s", ErrDecisionUnavailable, msg) }
func notSent(msg string) error {
	return fmt.Errorf("%w: %w: %s", ErrDecisionUnavailable, ErrNotSent, msg)
}

// keepClass gives a redacted error the class of the original, so a
// message rewritten to drop a key still routes the same way in the chain.
func keepClass(orig, redacted error) error {
	if redacted == nil || redacted == orig {
		return redacted
	}
	switch {
	case errors.Is(orig, ErrDecisionUncertain):
		return fmt.Errorf("%w: %s", ErrDecisionUncertain, redacted.Error())
	case errors.Is(orig, ErrNotSent):
		return fmt.Errorf("%w: %w: %s", ErrDecisionUnavailable, ErrNotSent, redacted.Error())
	default:
		return fmt.Errorf("%w: %s", ErrDecisionUnavailable, redacted.Error())
	}
}

// Outcomes of one attempt, as logged.
const (
	OutcomeAnswered    = "answered"
	OutcomeUncertain   = "uncertain"
	OutcomeUnavailable = "unavailable"
	OutcomeNotSent     = "not_sent"
)

func outcomeOf(err error) string {
	switch {
	case err == nil:
		return OutcomeAnswered
	case errors.Is(err, ErrDecisionUncertain):
		return OutcomeUncertain
	case errors.Is(err, ErrNotSent):
		return OutcomeNotSent
	}
	return OutcomeUnavailable
}

// The fallback decisions models and the escalation LLM on OpenRouter when
// SCRY_JUDGE_FALLBACK_MODELS and SCRY_JUDGE_ESCALATION_MODEL are unset.
const DefaultJudgeEscalationModel = "google/gemini-4-hydrogen"

// DefaultJudgeFallbackModels is the fallback chain, in order.
func DefaultJudgeFallbackModels() []string {
	return []string{"openai/gpt-6-luna-decisions", "google/gemini-decision-1"}
}

// GradingStep is one model the chain may ask: a decisions model or the
// escalation LLM. Judge returns a verdict with its fixed criterion
// sentence, or an error classed ErrDecisionUncertain or (anything else)
// unavailable. Usage.Model is empty when the outcome is unknown (a request
// was sent and nothing came back that says what it cost).
type GradingStep interface {
	JudgeProvider
	RequestedModel() string
}

// Attempt is one step the chain took for one submission.
type Attempt struct {
	Step      string // primary, fallback or escalation
	Requested string // the model asked for
	Outcome   string // answered, uncertain, unavailable or not_sent
	Sent      bool   // a request left (false only for not_sent)
	Usage     Usage  // what the provider reported; Model "" if unknown
}

// Known reports whether the attempt's cost is known: it was never sent, or
// the provider reported usage.
func (a Attempt) Known() bool { return !a.Sent || a.Usage.Model != "" }

// Chain is the ordered grading chain. The same orchestrator runs in every
// mode: live with Jev and OpenRouter (or direct) adapters, test mode with
// fixture steps (Fake), so the fixture paths exercise this logic.
type Chain struct {
	Decisions  []GradingStep // the primary first, then the fallbacks in order
	Escalation GradingStep   // the LLM asked once when no decision is certain; nil: none
	Log        *slog.Logger
	// Per-attempt bounds; zero means the defaults (8 s and 15 s), so the
	// whole chain stays inside the app's 45 s judging deadline.
	DecisionTimeout, EscalationTimeout time.Duration
}

// Name is the primary's name ("jev" in production, "test" for the fake).
func (c *Chain) Name() string {
	if c == nil || len(c.Decisions) == 0 {
		return "none"
	}
	return providerName(c.Decisions[0])
}

// Primary is the first decisions model, or nil.
func (c *Chain) Primary() GradingStep {
	if c == nil || len(c.Decisions) == 0 {
		return nil
	}
	return c.Decisions[0]
}

func (c *Chain) log() *slog.Logger {
	if c.Log != nil {
		return c.Log
	}
	return slog.Default()
}

// Judge walks the chain once for one submission. The usage it returns
// covers every attempt (Usage.Attempts, when there was more than one), so
// the caller settles one reservation once. Only when no step returns a
// usable verdict is the error ErrNoVerdict.
func (c *Chain) Judge(ctx context.Context, in JudgeInput) (Judgment, Usage, error) {
	start := time.Now()
	var tried []Attempt
	why := "no decisions model"
	for i, d := range c.Decisions {
		step := "primary"
		if i > 0 {
			step = "fallback"
		}
		j, att, err := c.attempt(ctx, step, i, d, in, c.timeout(c.DecisionTimeout, 8*time.Second))
		tried = append(tried, att)
		if err == nil {
			return j, c.finish(ctx, start, tried, &tried[len(tried)-1], ""), nil
		}
		if ctx.Err() != nil {
			return Judgment{}, c.finish(ctx, start, tried, nil, "interrupted"), fmt.Errorf("%w: %v", ErrNoVerdict, ctx.Err())
		}
		if errors.Is(err, ErrDecisionUncertain) {
			why = "uncertain"
			break // straight to the LLM, never another decisions model
		}
		why = "no decisions model answered"
	}
	if c.Escalation != nil {
		j, att, err := c.attempt(ctx, "escalation", len(c.Decisions), c.Escalation, in, c.timeout(c.EscalationTimeout, 15*time.Second))
		tried = append(tried, att)
		if err == nil {
			return j, c.finish(ctx, start, tried, &tried[len(tried)-1], why), nil
		}
	}
	return Judgment{}, c.finish(ctx, start, tried, nil, why), ErrNoVerdict
}

func (c *Chain) timeout(set, def time.Duration) time.Duration {
	if set > 0 {
		return set
	}
	return def
}

func (c *Chain) attempt(ctx context.Context, step string, i int, s GradingStep, in JudgeInput, timeout time.Duration) (Judgment, Attempt, error) {
	start := time.Now()
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	j, use, err := s.Judge(actx, in)
	if err == nil && (!j.Verdict.Valid() || strings.TrimSpace(j.Reason) == "") {
		err = unavailable("judgment had no verdict or no reason")
	}
	if err != nil && !errors.Is(err, ErrDecisionUncertain) && !errors.Is(err, ErrDecisionUnavailable) {
		err = fmt.Errorf("%w: %s", ErrDecisionUnavailable, err.Error())
	}
	if step == "escalation" && errors.Is(err, ErrDecisionUncertain) {
		// Nothing comes after the escalation; for it, unsure is unavailable.
		err = fmt.Errorf("%w: escalation uncertain", ErrDecisionUnavailable)
	}
	if err != nil {
		j = Judgment{}
	}
	att := Attempt{Step: step, Requested: s.RequestedModel(), Outcome: outcomeOf(err), Sent: !errors.Is(err, ErrNotSent), Usage: use}
	resolved := use.Model
	if resolved == "" {
		resolved = "unavailable"
	}
	var cost, usageCost any = nil, "unknown"
	if use.CostMicros != nil {
		cost = *use.CostMicros
	}
	if use.CostUSD != nil {
		usageCost = *use.CostUSD
	}
	// Never the answer, the question, an error body or a key.
	c.log().InfoContext(ctx, "judge attempt", "step", step, "position", i+1, "requested_model", att.Requested, "resolved_model", resolved,
		"latency_ms", time.Since(start).Milliseconds(), "cost_micros", cost, "usage_cost", usageCost,
		"input_tokens", use.InputTokens, "output_tokens", use.OutputTokens, "outcome", att.Outcome)
	return j, att, err
}

// finish logs the walk and folds every attempt into one Usage. One attempt
// is returned as it came (its Model empty if its outcome is unknown, as
// for a bare adapter). Several are summed: Model is the model that gave
// the verdict, or on failure the last one asked, or empty when any
// attempt's outcome is unknown (the caller then keeps the hold).
func (c *Chain) finish(ctx context.Context, start time.Time, tried []Attempt, winner *Attempt, escalatedBecause string) Usage {
	var u Usage
	path := make([]string, len(tried))
	unknown := false
	for i, a := range tried {
		path[i] = a.Step + ":" + a.Outcome
		if !a.Known() {
			unknown = true
		}
	}
	verdictBy := ""
	if winner != nil {
		verdictBy = winner.Requested
	}
	c.log().InfoContext(ctx, "judge chain", "attempts", len(tried), "path", strings.Join(path, ","), "escalated_because", escalatedBecause,
		"verdict_by", verdictBy, "answered", winner != nil, "unknown_outcome", unknown, "latency_ms", time.Since(start).Milliseconds())
	if len(tried) == 1 {
		return tried[0].Usage
	}
	u.Attempts = &tried
	for _, a := range tried {
		u.InputTokens += a.Usage.InputTokens
		u.OutputTokens += a.Usage.OutputTokens
	}
	switch {
	case winner != nil:
		u.Model = winner.Usage.Model
		if u.Model == "" {
			u.Model = winner.Requested
		}
	case !unknown && len(tried) > 0:
		last := tried[len(tried)-1]
		u.Model = last.Usage.Model
		if u.Model == "" {
			u.Model = last.Requested
		}
	}
	return u
}
