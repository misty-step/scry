package learning

import "testing"

func candidate(id, concept string, opts ...func(*Candidate)) Candidate {
	c := Candidate{QuizID: id, ConceptID: concept, New: true, AvailableAt: 0, Retrievability: -1, Order: int64(len(id))}
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

func due(r float64) func(*Candidate) {
	return func(c *Candidate) { c.New, c.Retrievability = false, r }
}

func input(candidates ...Candidate) SelectionInput {
	in := SelectionInput{Now: 1000, Candidates: candidates, Ready: map[string]bool{}, Introduced: map[string]bool{},
		LevelUnlocked: map[string]bool{}, Deferred: map[string]bool{}, Remedial: map[string]bool{}, UnmetPrereqs: map[string]int{}, NewBudgetLeft: 6}
	for _, c := range candidates {
		in.Ready[c.ConceptID] = true
		in.LevelUnlocked[c.QuizID] = true
	}
	return in
}

// US-011: a concept whose prerequisite is unmet is not introduced while ready
// material exists, and selection never dead-ends when everything is blocked.
func TestSelectionPrerequisitesFirstUS011(t *testing.T) {
	in := input(candidate("q-advanced", "tls"), candidate("q-basic", "crypto"))
	in.Ready["tls"], in.UnmetPrereqs["tls"] = false, 1
	if got := Select(in); got.QuizID != "q-basic" || got.Reason != "new" {
		t.Fatalf("got %+v, want the prerequisite first", got)
	}
	blocked := input(candidate("q-advanced", "tls"))
	blocked.Ready["tls"], blocked.UnmetPrereqs["tls"] = false, 1
	if got := Select(blocked); got.QuizID != "q-advanced" || got.Reason != "fallback" {
		t.Fatalf("got %+v, want a fallback instead of a dead end", got)
	}
}

// US-010: due reviews come before new material, the most at-risk first; a
// paused goal adds no new material; focused goals start first.
func TestSelectionDueGoalsAndPacingUS010(t *testing.T) {
	in := input(candidate("new", "a"), candidate("steady", "b", due(0.7)), candidate("at-risk", "c", due(0.4)))
	if got := Select(in); got.QuizID != "at-risk" || got.Reason != "due" {
		t.Fatalf("got %+v, want the most at-risk due review", got)
	}
	paused := input(candidate("paused-new", "a", func(c *Candidate) { c.GoalPaused = true }))
	if got := Select(paused); got.QuizID != "" {
		t.Fatalf("paused goal introduced %+v", got)
	}
	focus := input(candidate("older", "a", func(c *Candidate) { c.GoalCreated = 1 }), candidate("focused", "b", func(c *Candidate) { c.GoalCreated = 0; c.GoalFocus = true }))
	if got := Select(focus); got.QuizID != "focused" {
		t.Fatalf("got %+v, want the focused goal's new material", got)
	}
	budget := input(candidate("fresh", "a"), candidate("started", "b"))
	budget.NewBudgetLeft, budget.Introduced["b"] = 0, true
	if got := Select(budget); got.QuizID != "started" {
		t.Fatalf("got %+v, want only already-introduced concepts once the pace budget is used", got)
	}
	locked := input(candidate("explain", "a"))
	locked.LevelUnlocked["explain"] = false
	if got := Select(locked); got.QuizID != "" {
		t.Fatalf("a locked level was chosen: %+v", got)
	}
}

// Interleaving, implied credit, remediation, intros, and practice focus.
func TestSelectionOrderingRules(t *testing.T) {
	interleaved := input(candidate("same", "a", due(0.3)), candidate("other", "b", due(0.5)))
	interleaved.LastConcept = "a"
	if got := Select(interleaved); got.QuizID != "other" {
		t.Fatalf("got %+v, want a different concept than the last one shown", got)
	}
	implied := input(candidate("prereq", "p", due(0.2)), candidate("normal", "n", due(0.6)))
	implied.Deferred["p"] = true
	if got := Select(implied); got.QuizID != "normal" {
		t.Fatalf("got %+v, want the implicitly exercised prerequisite deferred", got)
	}
	remedial := input(candidate("check", "p", func(c *Candidate) { c.New, c.AvailableAt, c.Retrievability = false, 5000, 0.9 }), candidate("fresh", "x"))
	remedial.Remedial["check"] = true
	if got := Select(remedial); got.QuizID != "check" || got.Reason != "remedial" {
		t.Fatalf("got %+v, want the prerequisite check before new material", got)
	}
	intro := input(candidate("just-read", "a"), candidate("next", "b"))
	intro.Introduced["a"], intro.LastIntro = true, "a"
	if got := Select(intro); got.QuizID != "next" {
		t.Fatalf("got %+v, want one other item between an intro and its first question", got)
	}
	focus := input(candidate("not-due", "f", func(c *Candidate) { c.New, c.AvailableAt, c.Retrievability = false, 99999, 0.95 }), candidate("due", "d", due(0.1)))
	focus.Focus = "f"
	if got := Select(focus); got.QuizID != "not-due" || got.Reason != "focus" {
		t.Fatalf("got %+v, want the practice focus even before it is due", got)
	}
	if got := Select(input()); got.QuizID != "" || got.Reason != "none" {
		t.Fatalf("empty input selected %+v", got)
	}
}
