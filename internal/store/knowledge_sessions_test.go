package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func publishKnowledgeFixture(t *testing.T, s *Store, text string, result GenerationResult) Source {
	t.Helper()
	ctx := context.Background()
	src, err := s.Capture(ctx, text, newID())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || claim == nil || claim.SourceID != src.ID {
		t.Fatalf("claim authored bundle: %+v %v", claim, err)
	}
	cost := int64(10)
	if err = s.CompleteJob(ctx, claim.ID, claim.LeaseToken, result, &cost); err != nil {
		t.Fatal(err)
	}
	src, err = s.Source(ctx, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func referenceBundle() GenerationResult {
	return GenerationResult{
		Model: "authored-knowledge-fixture", PromptVersion: "knowledge-fixture-v2",
		Coverage:  CoverageReport{Kind: "concepts", Complete: false, Missing: []string{"Diagnostic practice has not been authored for this instruction-only fixture."}},
		Units:     []GeneratedUnit{{Key: "foundation", Statement: "The synthetic foundation distinguishes the first and second alternatives", Kind: "foundation"}},
		Materials: []GeneratedMaterial{{Key: "instruction", Kind: "explanation", Title: "Distinguishing the synthetic alternatives", Body: "The synthetic foundation separates a first alternative from a second alternative; this is authored fixture instruction, not a factual learning claim.", Basis: "topic", EstimatedSeconds: 30, Links: []GeneratedLink{{UnitKey: "foundation", Role: "teaches"}}}},
	}
}

func establishFixtureTarget(t *testing.T, s *Store, id string) Presentation {
	t.Helper()
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	m, err := material(ctx, tx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRowContext(ctx, "SELECT id,revision FROM goals WHERE source_id=?", m.SourceID).Scan(&m.GoalID, &m.GoalRevision); err != nil {
		t.Fatal(err)
	}
	version := 0
	if m.Quiz != nil {
		if err = tx.QueryRowContext(ctx, "SELECT version FROM schedules WHERE quiz_id=?", m.Quiz.ID).Scan(&version); err != nil {
			t.Fatal(err)
		}
	}
	p, err := establishPresentation(ctx, tx, m, version, "", "Explicit regression target", s.now())
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReferenceContinueIsOneObservationAndExactRetry(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	src := publishKnowledgeFixture(t, s, "Synthetic reference-only goal", referenceBundle())
	if src.Job.Published != 1 || src.Job.NewQuizzes != 0 || len(src.Quizzes) != 0 {
		t.Fatalf("reference-only publication was discarded: %+v", src.Job)
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.Kind != "reference" {
		t.Fatalf("reference was not selectable: %+v %v", state, err)
	}
	id := state.Current.ID
	access, err := s.InspectionAccess(ctx, "presentation", id)
	if err != nil || !access.Allowed || access.ExpiresAt != 0 {
		t.Fatalf("current instruction has no stable occurrence access: %+v %v", access, err)
	}
	*now = now.Add(time.Hour)
	access, err = s.InspectionAccess(ctx, "presentation", id)
	if err != nil || !access.Allowed || access.ExpiresAt != 0 {
		t.Fatalf("active instruction dismissed by a retention timer: %+v %v", access, err)
	}
	ended, err := s.ContinueMaterial(ctx, id, "continue-once")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.ContinueMaterial(ctx, id, "continue-once")
	if err != nil || !reflect.DeepEqual(ended, again) {
		t.Fatalf("Continue retry changed acknowledged transition: %+v %v", again, err)
	}
	if _, err = s.ContinueMaterial(ctx, id, "continue-another-operation"); !errors.Is(err, ErrConflict) {
		t.Fatalf("second operation manufactured another reading: %v", err)
	}
	var readings, reviews, schedules int
	if err = s.db.QueryRow("SELECT count(*) FROM interactions WHERE kind='continue'").Scan(&readings); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT count(*) FROM review_events").Scan(&reviews); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT count(*) FROM schedules").Scan(&schedules); err != nil {
		t.Fatal(err)
	}
	if readings != 1 || reviews != 0 || schedules != 0 {
		t.Fatalf("instruction fabricated assessment: reading=%d review=%d cards=%d", readings, reviews, schedules)
	}
	access, err = s.InspectionAccess(ctx, "presentation", id)
	if err != nil || !access.Allowed || access.ExpiresAt != now.Add(30*time.Minute).UnixMilli() {
		t.Fatalf("retired instruction lacks fixed trailing access: %+v %v", access, err)
	}
	*now = now.Add(31 * time.Minute)
	access, err = s.InspectionAccess(ctx, "presentation", id)
	if err != nil || access.Allowed {
		t.Fatalf("old occurrence access slid on a read: %+v %v", access, err)
	}
}

func TestInspectionPracticeIsAtomicScopedAndNeverGradesNavigation(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	src := publishFixture(t, s, authoredChoice("Which synthetic alternative belongs here?"))
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := state.Current.ID
	var before string
	if err = s.db.QueryRow("SELECT card FROM schedules WHERE quiz_id=?", src.Quizzes[0].ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	access, err := s.InspectionAccess(ctx, "library", "")
	if err != nil || !access.RequiresAssistance {
		t.Fatalf("library titles omitted an affected target: %+v %v", access, err)
	}
	opened, err := s.AssistInspection(ctx, "library", "", "open-library-once")
	if err != nil || opened.Current == nil || opened.Current.ID != id || !opened.Current.Assisted || !opened.Current.Practice {
		t.Fatalf("inspection fence was not atomic: %+v %v", opened, err)
	}
	if access, err = s.InspectionAccess(ctx, "material", src.Quizzes[0].ID); err != nil || access.Allowed {
		t.Fatalf("title access authorized full answer-bearing body: %+v %v", access, err)
	}
	if _, err = s.AssistInspection(ctx, "library", "", "open-library-once"); err != nil {
		t.Fatal(err)
	}
	var observations int
	if err = s.db.QueryRow("SELECT count(*) FROM inspection_events WHERE kind='library'").Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if observations != 1 {
		t.Fatalf("retry duplicated explicit inspection: %d", observations)
	}
	*now = now.Add(31 * time.Minute)
	answered, err := s.Submit(ctx, id, "after-inspection", "second", false)
	if err != nil || !answered.Graded || !answered.Practice || !answered.Assisted || answered.Rating != 0 || answered.ReviewID != "" {
		t.Fatalf("inspected answer was recorded as cold recall: %+v %v", answered, err)
	}
	var after string
	if err = s.db.QueryRow("SELECT card FROM schedules WHERE quiz_id=?", src.Quizzes[0].ID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("navigation practice mutated direct FSRS")
	}
	var reviews int
	if err = s.db.QueryRow("SELECT count(*) FROM review_events").Scan(&reviews); err != nil || reviews != 0 {
		t.Fatalf("practice manufactured review history: %d %v", reviews, err)
	}
}

func TestBridgeRetainsTargetAndPracticeWithoutReviewFanout(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	bundle := referenceBundle()
	bundle.Units = append(bundle.Units, GeneratedUnit{Key: "target", Statement: "The synthetic integrated target applies the distinction in a second task", Kind: "composition"})
	bundle.Relations = []GeneratedRelation{{From: "foundation", To: "target", Kind: "prerequisite", Evidence: "Proposed relationship: distinguishing the alternatives supports the integrated target task."}}
	foundation := authoredChoice("Which alternative is the synthetic foundation?")
	foundation.Key, foundation.Level, foundation.EstimatedSeconds = "probe", "foundation", 30
	foundation.Links = []GeneratedLink{{UnitKey: "foundation", Role: "assesses"}}
	target := authoredChoice("Which alternative solves the synthetic integrated task?")
	target.Key, target.Level, target.EstimatedSeconds = "target_quiz", "target", 45
	target.Links = []GeneratedLink{{UnitKey: "target", Role: "assesses"}, {UnitKey: "foundation", Role: "assumes"}}
	bundle.Quizzes = []GeneratedQuiz{foundation, target}
	src := publishKnowledgeFixture(t, s, "Synthetic bridge goal", bundle)
	p := establishFixtureTarget(t, s, src.Quizzes[1].ID)
	var cardBefore string
	if err := s.db.QueryRow("SELECT card FROM schedules WHERE quiz_id=?", p.Quiz.ID).Scan(&cardBefore); err != nil {
		t.Fatal(err)
	}
	state, err := s.StartBridge(ctx, p.ID, "bridge-once")
	if err != nil || state.Bridge == nil || state.Current == nil || state.Current.Kind != "reference" || len(state.Suspended) != 1 || state.Suspended[0].ID != p.ID {
		t.Fatalf("bridge did not retain actual target: %+v %v", state, err)
	}
	path := s.Path()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	restarted, err := s.Review(ctx)
	if err != nil || restarted.Current == nil || restarted.Current.ID != state.Current.ID || restarted.Bridge.ID != state.Bridge.ID {
		t.Fatalf("bridge authority lost on restart: %+v %v", restarted, err)
	}
	returned, err := s.ReturnToTarget(ctx, state.Bridge.ID, "return-once")
	if err != nil || returned.Current == nil || returned.Current.ID != p.ID || !returned.Current.Practice {
		t.Fatalf("instruction did not return original target as practice: %+v %v", returned, err)
	}
	again, err := s.ReturnToTarget(ctx, state.Bridge.ID, "return-once")
	if err != nil || !reflect.DeepEqual(returned, again) {
		t.Fatalf("return retry changed occurrence: %+v %v", again, err)
	}
	answer, err := s.Submit(ctx, p.ID, "bridge-practice-answer", "second", false)
	if err != nil || answer.Rating != 0 || !answer.Practice || !answer.Graded {
		t.Fatalf("bridge credited an aided target: %+v %v", answer, err)
	}
	var cardAfter string
	if err = s.db.QueryRow("SELECT card FROM schedules WHERE quiz_id=?", p.Quiz.ID).Scan(&cardAfter); err != nil {
		t.Fatal(err)
	}
	if cardBefore != cardAfter {
		t.Fatal("bridge navigation/practice rewrote original FSRS")
	}
}

func TestExportReadAndInspectionAccessDoNotCreateObservations(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredChoice("Which alternative remains unobserved?"))
	before, err := s.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Goals(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.InspectionAccess(ctx, "export", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Interactions(ctx, 100); err != nil {
		t.Fatal(err)
	}
	after, err := s.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var left, right map[string]json.RawMessage
	if err = json.Unmarshal(before, &left); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(after, &right); err != nil {
		t.Fatal(err)
	}
	delete(left, "exported_at")
	delete(right, "exported_at")
	if !reflect.DeepEqual(left, right) {
		t.Fatal("read projections changed durable portable export")
	}
	if current, err := s.Current(ctx); err != nil || current != nil {
		t.Fatalf("read-only knowledge browsing invented an occurrence: %+v %v", current, err)
	}
}
