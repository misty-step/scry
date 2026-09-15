package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func instructionAndQuizBundle() GenerationResult {
	bundle := referenceBundle()
	q := authoredChoice("Which synthetic alternative follows the taught distinction?")
	q.Key, q.Level, q.EstimatedSeconds = "assessment", "target", 60
	q.Links = []GeneratedLink{{UnitKey: "foundation", Role: "assesses"}}
	bundle.Quizzes = []GeneratedQuiz{q}
	bundle.Coverage = CoverageReport{Kind: "concepts", Complete: true, Missing: []string{}}
	return bundle
}

func TestCrossGoalBridgePreservesSelectedGoalAndOriginalMaterialSource(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	original := publishKnowledgeFixture(t, s, "Original material provenance", instructionAndQuizBundle())
	originalGoal, err := s.Goal(ctx, original.GoalID)
	if err != nil {
		t.Fatal(err)
	}
	reuse := instructionAndQuizBundle()
	reuse.Units[0].ReuseID = originalGoal.Units[0].ID
	for _, m := range original.Materials {
		if m.Kind == "quiz" {
			reuse.Quizzes[0].ReuseID = m.ID
		} else {
			reuse.Materials[0].ReuseID = m.ID
		}
	}
	selected := publishKnowledgeFixture(t, s, "Distinct chosen goal using the same material", reuse)
	for _, id := range []string{original.GoalID, selected.GoalID} {
		g, err := s.Goal(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		settings := g.Settings
		settings.ExpectedRevision = g.Revision
		settings.TimeBudgetSeconds = 60
		settings.Reason = "One minute of estimated material for this chosen goal"
		if _, err = s.PlanGoal(ctx, id, settings, newID()); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := material(ctx, tx, original.Quizzes[0].ID)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	g, err := goalHeader(ctx, tx, selected.GoalID)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	m.GoalID, m.GoalRevision = g.ID, g.Revision
	var scheduleVersion int
	if err = tx.QueryRowContext(ctx, "SELECT version FROM schedules WHERE quiz_id=?", m.Quiz.ID).Scan(&scheduleVersion); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	p, err := establishPresentation(ctx, tx, m, scheduleVersion, "", "Explicit cross-goal regression selection", s.now())
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	preview, _, err := knowledgeCandidate(ctx, tx, s.now(), m.ID)
	if err != nil || preview == nil || preview.GoalID != original.GoalID {
		tx.Rollback()
		t.Fatalf("shared membership charged the unselected goal's time allowance: %+v %v", preview, err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// A later explicit plan epoch governs new help, not the old occurrence pin.
	*now = now.Add(time.Millisecond)
	settings := g.Settings
	settings.ExpectedRevision = g.Revision
	settings.TimeBudgetSeconds = 300
	settings.Reason = "Make room for the explicitly requested foundation path"
	if _, err = s.PlanGoal(ctx, g.ID, settings, "allow-cross-goal-bridge"); err != nil {
		t.Fatal(err)
	}
	state, err := s.StartBridge(ctx, p.ID, "cross-goal-bridge")
	if err != nil || state.Bridge == nil || state.Bridge.GoalID != selected.GoalID || state.Current == nil || state.Current.Kind != "reference" || state.Current.GoalID != selected.GoalID {
		t.Fatalf("bridge fell back to material origin goal: %+v %v", state, err)
	}
	if state.Current.Material.SourceID != original.ID || state.Current.Material.Provenance.SourceID != original.ID {
		t.Fatal("selected goal overwrote real original source provenance")
	}
	returned, err := s.ReturnToTarget(ctx, state.Bridge.ID, "cross-goal-return")
	if err != nil || returned.Current == nil || returned.Current.ID != p.ID || returned.Current.GoalID != selected.GoalID || returned.Current.GoalRevision != p.GoalRevision || !returned.Current.Practice {
		t.Fatalf("return lost original goal/occurrence pins: %+v %v", returned, err)
	}
}

func TestSuccessfulPracticeDoesNotCreateAnEndlessTeachingLoop(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	publishKnowledgeFixture(t, s, "A goal that should progress to a later cold check", instructionAndQuizBundle())
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.Kind != "reference" {
		t.Fatalf("expected initial instruction: %+v %v", state, err)
	}
	state, err = s.ContinueMaterial(ctx, state.Current.ID, "read-before-practice")
	if err != nil || state.Current == nil || state.Current.Kind != "quiz" || !state.Current.Practice {
		t.Fatalf("instruction was followed by falsely cold evidence: %+v %v", state, err)
	}
	answer, err := s.Submit(ctx, state.Current.ID, "successful-practice", "second", false)
	if err != nil || !answer.Graded || !answer.Practice || answer.Rating != 0 {
		t.Fatalf("practice answer: %+v %v", answer, err)
	}
	*now = now.Add(time.Minute)
	held, err := s.Review(ctx)
	if err != nil || held.Current == nil || held.Current.ID != answer.ID {
		t.Fatalf("reading held feedback changed the occurrence: %+v %v", held, err)
	}
	*now = now.Add(time.Millisecond)
	next, err := s.Next(ctx, answer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if next.Current != nil {
		t.Fatalf("completed practice recycled teaching or its own feedback exposure: %+v", next.Current)
	}
	*now = now.Add(time.Minute)
	tail, err := s.Review(ctx)
	if err != nil || tail.Current != nil {
		t.Fatalf("read time renewed completed practice during its own feedback tail: %+v %v", tail, err)
	}
	*now = now.Add(25 * time.Hour)
	cold, err := s.Review(ctx)
	if err != nil || cold.Current == nil || cold.Current.Kind != "quiz" || cold.Current.Practice || cold.Current.Assisted {
		t.Fatalf("next plan epoch never offered an honest cold check: %+v %v", cold, err)
	}
}

func TestCoverageCorrectionInvalidatesCompletionAndFencesOlderReuse(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	original := publishKnowledgeFixture(t, s, "Originally complete authored map", instructionAndQuizBundle())
	g, err := s.Goal(ctx, original.GoalID)
	if err != nil || !g.Coverage.Complete {
		t.Fatalf("fixture did not establish complete authored coverage: %+v %v", g, err)
	}
	pending, err := s.Capture(ctx, "Another chosen goal whose outstanding context contains older reusable content", "capture-before-correction")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || claim == nil || claim.SourceID != pending.ID {
		t.Fatalf("outstanding reuse context: %+v %v", claim, err)
	}
	target, err := s.Material(ctx, original.Quizzes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.EditCoverage(ctx, target.ID, target.Version, []CoverageLink{}, "This task does not reliably assess the claimed unit", "remove-assessment-coverage"); err != nil {
		t.Fatal(err)
	}
	changed, err := s.Goal(ctx, g.ID)
	if err != nil || changed.Coverage.Complete || changed.Coverage.Kind != "concepts" || changed.Revision <= g.Revision || len(changed.Coverage.Missing) == 0 {
		t.Fatalf("destructive correction kept obsolete completeness: %+v %v", changed, err)
	}
	reuse := instructionAndQuizBundle()
	reuse.Units[0].ReuseID = g.Units[0].ID
	reuse.Quizzes[0].ReuseID = target.ID
	for _, m := range original.Materials {
		if m.Kind != "quiz" {
			reuse.Materials[0].ReuseID = m.ID
		}
	}
	cost := int64(17)
	if err = s.CompleteJob(ctx, claim.ID, claim.LeaseToken, reuse, &cost); err == nil {
		t.Fatal("pre-correction reuse context republished superseded coverage")
	}
	var reviews int
	if err = s.db.QueryRow("SELECT count(*) FROM review_events").Scan(&reviews); err != nil || reviews != 0 {
		t.Fatalf("coverage correction manufactured learning history: %d %v", reviews, err)
	}
}

func TestObservedGapPinsOriginalFailureAndDisputeRevokesPaidPublication(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	src := publishFixture(t, s, authoredChoice("Which task has a real uncovered failure?"))
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := s.Submit(ctx, state.Current.ID, "actual-unassisted-failure", "first", false)
	if err != nil || answer.ReviewID == "" || answer.Rating != 1 || answer.Assisted {
		t.Fatalf("fixture failed to retain real direct failure: %+v %v", answer, err)
	}
	claim, err := s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || claim == nil || claim.Kind != "enrich" || claim.ObservationID != answer.ReviewID || claim.TargetPresentationID != answer.ID || claim.GoalID != src.GoalID {
		t.Fatalf("gap trigger lost original authority: %+v %v", claim, err)
	}
	if len(claim.Context.Evidence) == 0 || claim.Context.Evidence[0].ID != answer.ReviewID {
		t.Fatal("bounded context omitted or substituted the triggering event")
	}
	if err = s.Dispute(ctx, answer.ReviewID, "The original grading claim is disputed", false); err != nil {
		t.Fatal(err)
	}
	cost := int64(19)
	if err = s.CompleteJob(ctx, claim.ID, claim.LeaseToken, referenceBundle(), &cost); !errors.Is(err, ErrConflict) {
		t.Fatalf("disputed original failure still authorized content publication: %v", err)
	}
	var paid int64
	if err = s.db.QueryRow("SELECT cost_micros FROM job_attempts WHERE token=?", claim.LeaseToken).Scan(&paid); err != nil || paid != cost {
		t.Fatalf("dispute lost real in-flight charge: %d %v", paid, err)
	}
	if _, err = s.RetryJob(ctx, claim.ID, "retry-disputed-failure"); !errors.Is(err, ErrConflict) {
		t.Fatalf("retry bypassed disputed trigger authority: %v", err)
	}
}

func TestSiblingPracticeFeedbackDoesNotRestartEachOther(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	bundle := instructionAndQuizBundle()
	sibling := authoredChoice("Which second synthetic task uses the same distinction?")
	sibling.Key, sibling.Level, sibling.EstimatedSeconds = "sibling", "target", 60
	sibling.Links = []GeneratedLink{{UnitKey: "foundation", Role: "assesses"}}
	bundle.Quizzes = append(bundle.Quizzes, sibling)
	publishKnowledgeFixture(t, s, "Two tasks sharing genuinely taught knowledge", bundle)
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.Kind != "reference" {
		t.Fatalf("initial instruction: %+v %v", state, err)
	}
	state, err = s.ContinueMaterial(ctx, state.Current.ID, "teach-shared-unit")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range 2 {
		if state.Current == nil || state.Current.Kind != "quiz" || !state.Current.Practice || seen[state.Current.Material.ID] {
			t.Fatalf("feedback recycled a previously completed sibling: %+v", state.Current)
		}
		seen[state.Current.Material.ID] = true
		answer, err := s.Submit(ctx, state.Current.ID, newID(), "second", false)
		if err != nil || answer.Rating != 0 {
			t.Fatalf("sibling practice awarded recall credit: %+v %v", answer, err)
		}
		*now = now.Add(time.Millisecond)
		state, err = s.Next(ctx, answer.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if state.Current != nil {
		t.Fatalf("the siblings restarted one another indefinitely: %+v", state.Current)
	}
	*now = now.Add(time.Minute)
	state, err = s.Review(ctx)
	if err != nil || state.Current != nil {
		t.Fatalf("feedback-tail polling restarted siblings: %+v %v", state, err)
	}
	*now = now.Add(25 * time.Hour)
	state, err = s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.Kind != "quiz" || state.Current.Practice || state.Current.Assisted {
		t.Fatalf("finite warm practice never returned to a cold check: %+v %v", state, err)
	}
}

func TestHistoryDisclosureMatchesActualBoundedReferenceTimeline(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	original := publishKnowledgeFixture(t, s, "One actual instruction occurrence", referenceBundle())
	g, err := s.Goal(ctx, original.GoalID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.Kind != "reference" {
		t.Fatalf("reference: %+v %v", state, err)
	}
	ref := state.Current
	if _, err = s.ContinueMaterial(ctx, ref.ID, "actual-reference-continue"); err != nil {
		t.Fatal(err)
	}
	events, err := s.Interactions(ctx, HistoryPageSize)
	if err != nil || len(events) != 1 || events[0].Kind != "continue" || events[0].Material == nil || events[0].Material.Body != ref.Material.Body || events[0].Material.Version != ref.Material.Version {
		t.Fatalf("actual continued reference was missing its pinned shown content: %+v %v", events, err)
	}
	for range HistoryPageSize {
		*now = now.Add(time.Millisecond)
		if _, err = s.AssistInspection(ctx, "history", "", newID()); err != nil {
			t.Fatal(err)
		}
	}
	*now = now.Add(31 * time.Minute)
	q := authoredChoice("Which new task checks the previously exposed but now hidden unit?")
	q.Key, q.Level, q.EstimatedSeconds = "new_check", "target", 60
	q.Links = []GeneratedLink{{UnitKey: "shared", Role: "assesses"}}
	bundle := GenerationResult{
		Coverage: CoverageReport{Kind: "concepts", Complete: false, Missing: []string{"No new instruction requested"}},
		Units:    []GeneratedUnit{{Key: "shared", ReuseID: g.Units[0].ID, Statement: g.Units[0].Statement, Kind: g.Units[0].Kind}},
		Quizzes:  []GeneratedQuiz{q}, Model: "fixture", PromptVersion: "fixture",
	}
	publishKnowledgeFixture(t, s, "A later chosen goal reusing the same unit", bundle)
	state, err = s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.Kind != "quiz" || state.Current.Practice {
		t.Fatalf("expected a genuine later cold target: %+v %v", state, err)
	}
	access, err := s.InspectionAccess(ctx, "history", "")
	if err != nil || access.RequiresAssistance {
		t.Fatalf("unshown out-of-window reference contaminated the history disclosure scope: %+v %v", access, err)
	}
	state, err = s.AssistInspection(ctx, "history", "", "bounded-history-inspection")
	if err != nil || state.Current == nil || state.Current.Practice || state.Current.Assisted {
		t.Fatalf("metadata-only current history assisted an unrelated hidden old reference: %+v %v", state, err)
	}
	events, err = s.Interactions(ctx, HistoryPageSize)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Kind == "continue" || event.Material != nil && event.Material.Body != "" {
			t.Fatal("timeline disclosed content outside its selected activity window")
		}
	}
}

func TestBrowserJobViewsExcludeUnrelatedAnswersWhileClaimsRetainContext(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	secret := "UNRELATED_PRIVATE_KEY_48371"
	private := authoredBundle(GeneratedQuiz{Kind: "recall", Prompt: "What is the unrelated private key?", Answer: secret, Explanation: "The canonical synthetic key is the exact required response", Basis: "topic"})
	private.Units[0].Statement = "The unrelated private key is " + secret
	publishKnowledgeFixture(t, s, "An unrelated private recall goal", private)
	target := publishFixture(t, s, authoredChoice("Which alternative handles the separate target?"))
	p := establishFixtureTarget(t, s, target.Quizzes[0].ID)
	if _, err := s.StartBridge(ctx, p.ID, "private-context-bridge"); err != nil {
		t.Fatal(err)
	}
	claim, err := s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || claim == nil || claim.Kind != "bridge" {
		t.Fatalf("private bounded bridge claim: %+v %v", claim, err)
	}
	raw, err := json.Marshal(claim)
	if err != nil || !strings.Contains(string(raw), secret) || claim.SourceText == "" || claim.MetadataOnly {
		t.Fatalf("worker was deprived of its real bounded reusable context: %v", err)
	}
	source, err := s.Source(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	goal, err := s.Goal(ctx, target.GoalID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := s.StartBridge(ctx, p.ID, "private-context-bridge")
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []any{source, goal, state, replayed} {
		raw, err = json.Marshal(view)
		if err != nil || strings.Contains(string(raw), secret) {
			t.Fatalf("browser-facing view disclosed an unrelated cold answer from worker context: %v", err)
		}
	}
	if source.Job == nil || source.Job.SourceText != "" || !source.Job.MetadataOnly || state.Bridge == nil || state.Bridge.Job == nil || state.Bridge.Job.SourceText != "" || !state.Bridge.Job.MetadataOnly {
		t.Fatal("browser job status retained private source/context authority")
	}
	settings := goal.Settings
	settings.ExpectedRevision, settings.Focus, settings.Reason = goal.Revision, "foundation", "A newly chosen foundation-first plan epoch"
	if _, err = s.PlanGoal(ctx, goal.ID, settings, "private-context-new-plan"); err != nil {
		t.Fatal(err)
	}
	cost := int64(23)
	if err = s.CompleteJob(ctx, claim.ID, claim.LeaseToken, referenceBundle(), &cost); err == nil {
		t.Fatal("superseded claim published")
	}
	retry, err := s.RetryJob(ctx, claim.ID, "private-context-retry")
	if err != nil {
		t.Fatal(err)
	}
	retryReplay, err := s.RetryJob(ctx, claim.ID, "private-context-retry")
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range []Job{retry, retryReplay} {
		raw, err = json.Marshal(job)
		if err != nil || !job.MetadataOnly || job.SourceText != "" || strings.Contains(string(raw), secret) {
			t.Fatalf("retry/replay disclosed private worker input: %v", err)
		}
	}
	var retained string
	if err = s.db.QueryRow("SELECT context_json FROM jobs WHERE id=?", claim.ID).Scan(&retained); err != nil || !strings.Contains(retained, secret) {
		t.Fatalf("redacting browser metadata destroyed the immutable paid context: %v", err)
	}
}
