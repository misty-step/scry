package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func foundationFixture() *FoundationContent {
	return &FoundationContent{Units: []FoundationUnit{{Key: "carbon", Definition: "Carbon fixation incorporates inorganic carbon into organic molecules.", Kind: "foundation"}}, Materials: []FoundationMaterial{
		{Kind: "instruction", Title: "Carbon fixation", Body: "Carbon dioxide supplies carbon atoms. Fixation incorporates those atoms into organic molecules; ATP provides energy instead of carbon atoms.", Links: []FoundationLink{{Unit: "carbon", Role: "teaches", Provenance: "Authored synthetic instruction"}}},
		{Kind: "practice", Title: "Identify the carbon input", Quiz: &GeneratedQuiz{Kind: "recall", Prompt: "Which gas supplies carbon atoms to the Calvin cycle?", Answer: "carbon dioxide", Explanation: "Carbon dioxide supplies carbon atoms incorporated during fixation; ATP supplies energy rather than carbon atoms.", Basis: "topic"}, Links: []FoundationLink{{Unit: "carbon", Role: "directly-assesses", Provenance: "Names the inorganic carbon input"}}},
	}}
}

func startFoundation(t *testing.T, s *Store) (Presentation, string, *Job) {
	t.Helper()
	ctx := context.Background()
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.RequestFoundation(ctx, state.Current.ID, newID(), "ATP and...", state.Current.BridgeRevision)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || j == nil {
		t.Fatalf("foundation claim: %v %v", j, err)
	}
	return *state.Current, id, j
}

func finishFoundation(t *testing.T, s *Store, j *Job) {
	t.Helper()
	cost := int64(50)
	result := GenerationResult{Foundation: foundationFixture(), Model: "synthetic-authored", PromptVersion: "foundation-test"}
	if err := s.CompleteJob(context.Background(), j.ID, j.LeaseToken, result, &cost); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(context.Background(), j.ID, j.LeaseToken, result, &cost); err != nil {
		t.Fatalf("publication exact retry: %v", err)
	}
}

func stepFoundation(t *testing.T, s *Store, id, action, answer string) FoundationBridge {
	t.Helper()
	ctx := context.Background()
	b, err := s.FoundationBridge(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	op := newID()
	if err = s.AdvanceFoundation(ctx, id, b.Revision, op, action, answer); err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceFoundation(ctx, id, b.Revision, op, action, answer); err != nil {
		t.Fatalf("lost response retry: %v", err)
	}
	if err = s.AdvanceFoundation(ctx, id, b.Revision, newID(), action, answer); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale progress accepted: %v", err)
	}
	b, err = s.FoundationBridge(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func exportSection(t *testing.T, s *Store, key string) json.RawMessage {
	t.Helper()
	encoded, err := s.Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &out); err != nil {
		t.Fatal(err)
	}
	return out[key]
}

func TestFoundationRequestExposureWarmReturnAndReuse(t *testing.T) {
	ctx := context.Background()
	s, now := newTestStore(t)
	publishFixture(t, s, authoredChoice("Original immutable target?"))
	beforeSchedule := exportSection(t, s, "schedules")
	p, id, j := startFoundation(t, s)
	if got := exportSection(t, s, "review_events"); string(got) != "[]" {
		t.Fatalf("request fabricated a learning event: %s", got)
	}
	b, err := s.FoundationBridge(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if b.Current != nil || b.Phase != "waiting" || b.Job.SourceText != "" {
		t.Fatalf("pending help disclosed instruction/source content: %+v", b)
	}
	// Ordinary review and its draft remain available while generation runs.
	current, err := s.Current(ctx)
	if err != nil || current.ID != p.ID || current.Draft != "ATP and..." || current.Graded || current.Assisted {
		t.Fatalf("help altered original attempt/draft: %+v %v", current, err)
	}
	finishFoundation(t, s, j)
	b, err = s.FoundationBridge(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if b.Current != nil || b.Phase != "ready" {
		t.Fatal("unacknowledged content disclosed")
	}
	b = stepFoundation(t, s, id, "open", "")
	if b.Current == nil || b.Current.Body == "" || b.Current.Version != 1 {
		t.Fatal("instruction unavailable")
	}
	b = stepFoundation(t, s, id, "next", "")
	if b.Phase != "practice" || b.Current.Quiz.Answer != "" {
		t.Fatal("practice answer revealed before attempt")
	}
	b = stepFoundation(t, s, id, "answer", "carbon dioxide")
	if b.Phase != "feedback" || b.Outcome != "correct" || b.Current.Quiz.Answer != "carbon dioxide" {
		t.Fatalf("warm feedback: %+v", b)
	}
	if got := exportSection(t, s, "review_events"); string(got) != "[]" {
		t.Fatalf("warm practice fabricated review: %s", got)
	}
	if got := exportSection(t, s, "schedules"); !reflect.DeepEqual(got, beforeSchedule) {
		t.Fatal("instruction/practice changed FSRS")
	}
	b = stepFoundation(t, s, id, "next", "")
	if b.Phase != "return" {
		t.Fatal("bridge is not finite")
	}
	stepFoundation(t, s, id, "return", "")
	path := s.Path()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openAt(t, path, now)
	current, err = s.Current(ctx)
	if err != nil || current.ID != p.ID || !reflect.DeepEqual(current.Quiz, p.Quiz) {
		t.Fatalf("exact original lost on restart: %+v %v", current, err)
	}
	result, err := s.Submit(ctx, p.ID, "warm-target", "second", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Rating != 0 || !result.Assisted || !result.Graded || result.Outcome != "warm_correct" {
		t.Fatalf("warm target counted as cold: %+v", result)
	}
	if got := exportSection(t, s, "schedules"); !reflect.DeepEqual(got, beforeSchedule) {
		t.Fatal("warm target changed FSRS")
	}
	state, err := s.Next(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Current != nil {
		t.Fatal("Next immediately repeated the same warm target")
	}
	state, err = s.Next(ctx, p.ID)
	if err != nil || state.Current != nil {
		t.Fatal("repeated Next created an unacknowledged new occurrence")
	}
	state, err = s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Current != nil || state.Due != 0 || state.NextDueAt != now.Add(24*time.Hour).UnixMilli() {
		t.Fatalf("reload resurrected warm work: %+v", state)
	}
	*now = now.Add(25 * time.Hour)
	state, err = s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	jobsBefore := exportSection(t, s, "jobs")
	later, err := s.RequestFoundation(ctx, state.Current.ID, "reuse-foundation", "", state.Current.BridgeRevision)
	if err != nil {
		t.Fatal(err)
	}
	b, err = s.FoundationBridge(ctx, later)
	if err != nil {
		t.Fatal(err)
	}
	if b.BundleID == "" || b.Phase != "ready" || b.Current != nil || !reflect.DeepEqual(jobsBefore, exportSection(t, s, "jobs")) {
		t.Fatal("compatible saved foundations not safely reused")
	}
	old, err := s.FoundationBridge(ctx, id)
	if err != nil || old.Phase != "ready" || old.Current != nil {
		t.Fatal("old URL bypassed fresh exposure fence")
	}
	// A stale return may acknowledge the old bridge but never select its target.
	stepFoundation(t, s, id, "return", "")
	current, err = s.Current(ctx)
	if err != nil || current.ID != state.Current.ID {
		t.Fatal("old bridge replaced newer current occurrence")
	}
}

func TestFoundationRequestOnlyAndExplicitRevealKeepDirectReviewContract(t *testing.T) {
	for _, reveal := range []bool{false, true} {
		t.Run(map[bool]string{false: "request-only", true: "explicit-reveal"}[reveal], func(t *testing.T) {
			ctx := context.Background()
			s, _ := newTestStore(t)
			publishFixture(t, s, authoredChoice("Original?"))
			p, id, j := startFoundation(t, s)
			if reveal {
				finishFoundation(t, s, j)
				stepFoundation(t, s, id, "open", "")
			}
			answer := "second"
			if reveal {
				answer = ""
			}
			result, err := s.Submit(ctx, p.ID, "direct-attempt", answer, reveal)
			if err != nil {
				t.Fatal(err)
			}
			want := 3
			if reveal {
				want = 1
			}
			if result.Rating != want || result.Assisted != reveal {
				t.Fatalf("direct contract changed: %+v", result)
			}
		})
	}
}

func TestFoundationPendingReturnFailureUnknownAndStalePublication(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	src := publishFixture(t, s, authoredChoice("Original?"))
	_, id, j := startFoundation(t, s)
	stepFoundation(t, s, id, "return", "")
	finishFoundation(t, s, j)
	b, err := s.FoundationBridge(ctx, id)
	if err != nil || b.Phase != "ready" || b.BundleID == "" {
		t.Fatal("return while pending orphaned completed material")
	}
	// Editing the exact target rejects an outstanding completion and still settles cost.
	s2, _ := newTestStore(t)
	publishFixture(t, s2, authoredChoice("Original?"))
	p2, id2, j2 := startFoundation(t, s2)
	if _, err = s2.EditQuiz(ctx, p2.Quiz.ID, p2.Quiz.Version, authoredChoice("Corrected future target?")); err != nil {
		t.Fatal(err)
	}
	cost := int64(70)
	err = s2.CompleteJob(ctx, j2.ID, j2.LeaseToken, GenerationResult{Foundation: foundationFixture(), Model: "synthetic", PromptVersion: "test"}, &cost)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("superseded target published: %v", err)
	}
	b, err = s2.FoundationBridge(ctx, id2)
	if err != nil || b.BundleID != "" || b.Available || b.Job.CostUnknown || b.Job.CostMicros != 70 {
		t.Fatalf("stale cost/target fence failed: %+v %v", b, err)
	}
	if _, err = s.RetrySource(ctx, src.ID, "wrong-retry-route"); !errors.Is(err, ErrConflict) {
		t.Fatal("capture retry accepted foundation job")
	}
	s3, _ := newTestStore(t)
	publishFixture(t, s3, authoredChoice("Another?"))
	p3, id3, j3 := startFoundation(t, s3)
	if err = s3.FailJob(ctx, j3.ID, j3.LeaseToken, "synthetic uncertain provider outcome", false, nil); err != nil {
		t.Fatal(err)
	}
	b, err = s3.FoundationBridge(ctx, id3)
	if err != nil {
		t.Fatal(err)
	}
	if err = s3.AdvanceFoundation(ctx, id3, b.Revision, "unsafe-retry", "retry", ""); !errors.Is(err, ErrConflict) {
		t.Fatal("unknown paid work retried")
	}
	result, err := s3.Submit(ctx, p3.ID, "ordinary-while-failed", "second", false)
	if err != nil || result.Rating != 3 {
		t.Fatalf("failed generation blocked review: %+v %v", result, err)
	}
}

func TestFoundationRejectsExecutableReferencesAndUntaughtPractice(t *testing.T) {
	f := foundationFixture()
	f.Materials[0].Kind = "reference"
	f.Materials[0].URL = "javascript:alert(1)"
	if err := ValidateFoundation(f); !errors.Is(err, ErrInvalid) {
		t.Fatal("executable reference accepted")
	}
	f = foundationFixture()
	f.Materials[0].Links[0].Role = "mentions"
	if err := ValidateFoundation(f); !errors.Is(err, ErrInvalid) {
		t.Fatal("mentioned-only material treated as teaching")
	}
}
