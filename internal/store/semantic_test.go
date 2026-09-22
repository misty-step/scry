package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

func authoredSemantic() GeneratedQuiz {
	return GeneratedQuiz{
		Kind: "recall", Grading: "semantic", Prompt: "How do HTTP validators support safe cache reuse?",
		Answer:      "They identify a stored representation and let the server confirm whether it changed.",
		Explanation: "A validator identifies a representation; a conditional request lets the origin confirm whether it is still current.",
		Basis:       "topic",
		Rubric: &Rubric{
			Required: []RubricIdea{
				{Text: "A validator identifies a stored representation", Cue: "Think about how a prior representation is recognized."},
				{Text: "The server confirms whether that representation changed"},
			},
			Contradictions: []RubricClaim{{Text: "A validator forces every response body to be downloaded", Feedback: "An unchanged response can be reused without downloading its body again."}},
		},
	}
}

func stageSemantic(t *testing.T, s *Store, operation, answer string) Presentation {
	t.Helper()
	state, err := s.Review(context.Background())
	if err != nil || state.Current == nil {
		t.Fatalf("open semantic review: %+v %v", state, err)
	}
	p, err := s.Submit(context.Background(), state.Current.ID, operation, answer, false)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Pending || p.Graded || p.AssessmentID == "" || p.Answer != answer || p.Quiz.Answer != "" {
		t.Fatalf("semantic answer was not staged safely: %+v", p)
	}
	return p
}

var testSpending = SemanticSpending{ReservationMicros: 2_000, DailyBudgetMicros: 1_000_000}

func beginSemantic(t *testing.T, s *Store, id string) AssessmentLease {
	t.Helper()
	lease, err := s.BeginAssessmentTransmission(context.Background(), id, "typesafe/jev-1.13", []byte(`{"model":"typesafe/jev-1.13","state":{},"questions":{}}`), testSpending)
	if err != nil {
		t.Fatal(err)
	}
	if !lease.Send || lease.Token == "" {
		t.Fatalf("first transmission was not granted a send lease: %+v", lease)
	}
	return lease
}

func semanticResult(ideas []float64, relation string, relationProbability, contradiction, injection float64) AssessmentResult {
	cost := int64(19)
	return AssessmentResult{
		ResponseModel: "typesafe/jev-1.13-20260917",
		ResponseJSON:  []byte(`{"model":"typesafe/jev-1.13-20260917","answers":{"recorded":true}}`),
		Judgments: learning.SemanticJudgments{
			Ideas: ideas, Contradictions: []float64{contradiction}, Relation: relation,
			RelationProbabilities: map[string]float64{relation: relationProbability}, Injection: injection,
		},
		InputTokens: 100, OutputTokens: 20, CostMicros: &cost, LatencyMS: 42,
	}
}

func TestSemanticAssessmentStagesFinalizesAndReplaysIdempotently(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredSemantic())
	var beforeCard string
	if err := s.db.QueryRowContext(ctx, "SELECT card FROM schedules LIMIT 1").Scan(&beforeCard); err != nil {
		t.Fatal(err)
	}
	p := stageSemantic(t, s, "semantic-correct", "Validators name the cached representation so the server can say whether it changed.")
	var events int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM review_events").Scan(&events); err != nil || events != 0 {
		t.Fatalf("staging wrote a review event: %d %v", events, err)
	}
	lease := beginSemantic(t, s, p.AssessmentID)
	a := lease.Assessment
	if a.Transmissions != 1 || a.Answer != p.Answer || a.PolicyVersion != learning.SemanticPolicyVersion || a.ReservedMicros != testSpending.ReservationMicros {
		t.Fatalf("bad durable transmission: %+v", a)
	}
	finalized, err := s.FinalizeAssessment(ctx, p.AssessmentID, lease.Token, semanticResult([]float64{0.96, 0.93}, "equivalent", 0.94, 0.02, 0.01))
	if err != nil {
		t.Fatal(err)
	}
	if !finalized.Graded || finalized.Outcome != "correct" || finalized.Rating != 3 || finalized.AssessmentStatus != "judged" || finalized.AssessmentDecision != "correct" {
		t.Fatalf("semantic correct was not finalized: %+v", finalized)
	}
	var grading, algorithm, afterCard string
	var reserved int64
	if err = s.db.QueryRowContext(ctx, "SELECT grading,algorithm FROM review_events WHERE presentation_id=?", p.ID).Scan(&grading, &algorithm); err != nil || grading != learning.SemanticPolicyVersion {
		t.Fatalf("missing grading provenance: %q %v", grading, err)
	}
	if algorithm != learning.EventAlgorithm(learning.SemanticPolicyVersion) || algorithm == learning.Algorithm {
		t.Fatalf("semantic event carries the exact identity: %q", algorithm)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT algorithm FROM schedules LIMIT 1").Scan(&algorithm); err != nil || algorithm != learning.Algorithm {
		t.Fatalf("schedule identity changed: %q %v", algorithm, err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT card FROM schedules LIMIT 1").Scan(&afterCard); err != nil || afterCard == beforeCard {
		t.Fatalf("correct semantic grade did not advance schedule: %v", err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT reserved_micros FROM semantic_assessments WHERE id=?", p.AssessmentID).Scan(&reserved); err != nil || reserved != 0 {
		t.Fatalf("measured cost did not replace the reservation: %d %v", reserved, err)
	}
	replayed, err := s.Submit(ctx, p.ID, "semantic-correct", p.Answer, false)
	if err != nil || !replayed.Graded || replayed.ReviewID != finalized.ReviewID || replayed.AssessmentID != p.AssessmentID {
		t.Fatalf("judged operation replay changed state: %+v %v", replayed, err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM review_events").Scan(&events); err != nil || events != 1 {
		t.Fatalf("replay duplicated review: %d %v", events, err)
	}
}

func TestSemanticDuplicateCallersNeverObtainSecondSendLease(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredSemantic())
	p := stageSemantic(t, s, "semantic-pending", "Caching validators can help with reuse.")
	if _, err := s.Submit(ctx, p.ID, "concurrent-other-operation", "Another answer", false); !errors.Is(err, ErrConflict) {
		t.Fatalf("concurrent pending submit was accepted: %v", err)
	}
	first := beginSemantic(t, s, p.AssessmentID)
	// A concurrent duplicate of the same operation, while the lease is live,
	// is refused instead of producing a second model request.
	_, err := s.BeginAssessmentTransmission(ctx, p.AssessmentID, "typesafe/jev-1.13", []byte(`{"model":"typesafe/jev-1.13"}`), testSpending)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate caller obtained a lease during a live lease: %v", err)
	}
	var transmissions int
	if err = s.db.QueryRowContext(ctx, "SELECT transmissions FROM semantic_assessments WHERE id=?", p.AssessmentID).Scan(&transmissions); err != nil || transmissions != 1 {
		t.Fatalf("duplicate caller changed transmissions: %d %v", transmissions, err)
	}
	// An exact replay of the same operation reconciles the pending state and
	// never resends.
	replayed, err := s.Submit(ctx, p.ID, "semantic-pending", p.Answer, false)
	if err != nil || !replayed.Pending || replayed.AssessmentID != p.AssessmentID {
		t.Fatalf("pending replay changed state: %+v %v", replayed, err)
	}
	// Only the lease holder can record the outcome.
	if foreign, err := s.FinalizeAssessment(ctx, p.AssessmentID, "not-the-token", semanticResult([]float64{0.99, 0.99}, "equivalent", 0.99, 0.01, 0.01)); err != nil || foreign.Graded || !foreign.Pending {
		t.Fatalf("foreign token finalized: %+v %v", foreign, err)
	}
	done, err := s.FinalizeAssessment(ctx, p.AssessmentID, first.Token, semanticResult([]float64{0.99, 0.99}, "equivalent", 0.99, 0.01, 0.01))
	if err != nil || !done.Graded {
		t.Fatalf("lease holder could not finalize: %+v %v", done, err)
	}
	// A late second finalize with the spent token is inert.
	again, err := s.FinalizeAssessment(ctx, p.AssessmentID, first.Token, semanticResult([]float64{0.1, 0.1}, "different", 0.9, 0.9, 0.01))
	if err != nil || again.ReviewID != done.ReviewID || again.Outcome != "correct" {
		t.Fatalf("stale token re-finalized: %+v %v", again, err)
	}
	var events int
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM review_events").Scan(&events); err != nil || events != 1 {
		t.Fatalf("duplicate finalization wrote events: %d %v", events, err)
	}
}

func TestSemanticCrashAfterSendReconcilesToFailedAndKeepsUnknownSpend(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredSemantic())
	before, err := s.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p := stageSemantic(t, s, "semantic-crash", "A validator lets the server confirm the cached copy.")
	beginSemantic(t, s, p.AssessmentID)
	// The process dies after the request leaves; the lease lapses.
	*now = now.Add(semanticLease + time.Second)
	replayed, err := s.Submit(ctx, p.ID, "semantic-crash", p.Answer, false)
	if err != nil || replayed.Pending || replayed.AssessmentStatus != "failed" || replayed.Graded || replayed.Answer != p.Answer {
		t.Fatalf("interrupted assessment was not reconciled safely: %+v %v", replayed, err)
	}
	var transmissions int
	var reserved int64
	var failure string
	if err = s.db.QueryRowContext(ctx, "SELECT transmissions,reserved_micros,error FROM semantic_assessments WHERE id=?", p.AssessmentID).Scan(&transmissions, &reserved, &failure); err != nil {
		t.Fatal(err)
	}
	if transmissions != 1 || reserved != testSpending.ReservationMicros || failure != assessmentInterruptedErr {
		t.Fatalf("interrupted assessment resent or released unknown spend: transmissions=%d reserved=%d error=%q", transmissions, reserved, failure)
	}
	// A retry after the crash is refused a lease: the first send is unknown.
	lease, err := s.BeginAssessmentTransmission(ctx, p.AssessmentID, "typesafe/jev-1.13", []byte(`{"model":"typesafe/jev-1.13"}`), testSpending)
	if err != nil || lease.Send || lease.Assessment.Status != "failed" {
		t.Fatalf("post-crash retry obtained a send lease: %+v %v", lease, err)
	}
	summary, err := s.Summary(ctx)
	if err != nil || !summary.CostUnknown || summary.CostMicros-before.CostMicros != testSpending.ReservationMicros {
		t.Fatalf("unknown spend not accounted in readout: before=%+v after=%+v %v", before, summary, err)
	}
	// An explicit new operation is the only way forward, and it is charged
	// against an allowance that still counts the unknown spend.
	retry, err := s.Submit(ctx, p.ID, "semantic-crash-retry", p.Answer, false)
	if err != nil || !retry.Pending || retry.AssessmentID == p.AssessmentID {
		t.Fatalf("new operation did not stage: %+v %v", retry, err)
	}
	tight := SemanticSpending{ReservationMicros: 2_000, DailyBudgetMicros: 3_000}
	if _, err = s.BeginAssessmentTransmission(ctx, retry.AssessmentID, "typesafe/jev-1.13", []byte(`{"model":"typesafe/jev-1.13"}`), tight); !errors.Is(err, ErrBudget) {
		t.Fatalf("allowance ignored unknown spend: %v", err)
	}
}

func TestSemanticAllowanceIsReservedBeforeSendAndSharedWithGeneration(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredSemantic())
	p := stageSemantic(t, s, "semantic-budget", "Validators identify the stored copy for the server to confirm.")
	// No budget headroom: the assessment fails durably before any request.
	lease, err := s.BeginAssessmentTransmission(ctx, p.AssessmentID, "typesafe/jev-1.13", []byte(`{"model":"typesafe/jev-1.13"}`), SemanticSpending{ReservationMicros: 2_000, DailyBudgetMicros: 1_000})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("reservation above budget accepted: %v", err)
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE semantic_assessments SET status='pending' WHERE id=?", p.AssessmentID); err != nil {
		t.Fatal(err)
	}
	// A pending generation job with an active reservation shares the allowance.
	if _, err = s.Capture(ctx, "A second captured subject for generation", newID()); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, time.Minute, 999_000, 1_000_000)
	if err != nil || job == nil {
		t.Fatalf("generation claim failed: %+v %v", job, err)
	}
	lease, err = s.BeginAssessmentTransmission(ctx, p.AssessmentID, "typesafe/jev-1.13", []byte(`{"model":"typesafe/jev-1.13"}`), testSpending)
	if !errors.Is(err, ErrBudget) || lease.Send {
		t.Fatalf("semantic reservation ignored the active generation reservation: %+v %v", lease, err)
	}
	var status, failure string
	var transmissions int
	if err = s.db.QueryRowContext(ctx, "SELECT status,error,transmissions FROM semantic_assessments WHERE id=?", p.AssessmentID).Scan(&status, &failure, &transmissions); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || failure != assessmentAllowanceErr || transmissions != 0 {
		t.Fatalf("allowance failure was not durable and send-free: %q %q %d", status, failure, transmissions)
	}
	// The reverse direction: a live semantic reservation counts against a
	// generation claim.
	retry, err := s.Submit(ctx, p.ID, "semantic-budget-retry", p.Answer, false)
	if err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	if err = s.FailJob(ctx, job.ID, job.LeaseToken, "synthetic", false, &zero); err != nil {
		t.Fatal(err)
	}
	held, err := s.BeginAssessmentTransmission(ctx, retry.AssessmentID, "typesafe/jev-1.13", []byte(`{"model":"typesafe/jev-1.13"}`), SemanticSpending{ReservationMicros: 600_000, DailyBudgetMicros: 1_000_000})
	if err != nil || !held.Send {
		t.Fatalf("semantic reservation refused with headroom: %+v %v", held, err)
	}
	if _, err = s.Capture(ctx, "A third captured subject for generation", newID()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimJob(ctx, time.Minute, 500_000, 1_000_000); !errors.Is(err, ErrBudget) {
		t.Fatalf("generation claim ignored the semantic reservation: %v", err)
	}
}

func TestSemanticFailurePreservesAnswerAndAllowsNewOperation(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredSemantic())
	p := stageSemantic(t, s, "semantic-failed", "A saved validator helps ask if data changed.")
	lease := beginSemantic(t, s, p.AssessmentID)
	failed, err := s.FailAssessment(ctx, p.AssessmentID, lease.Token, AssessmentResult{Error: "unavailable", LatencyMS: 8000})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Graded || failed.Pending || failed.AssessmentStatus != "failed" || failed.Answer != p.Answer || failed.Draft != p.Answer {
		t.Fatalf("failure lost or graded the answer: %+v", failed)
	}
	var reserved int64
	if err = s.db.QueryRowContext(ctx, "SELECT reserved_micros FROM semantic_assessments WHERE id=?", p.AssessmentID).Scan(&reserved); err != nil || reserved != testSpending.ReservationMicros {
		t.Fatalf("transport failure released unknown spend: %d %v", reserved, err)
	}
	same, err := s.Submit(ctx, p.ID, "semantic-failed", p.Answer, false)
	if err != nil || same.AssessmentID != p.AssessmentID || same.AssessmentStatus != "failed" {
		t.Fatalf("failed operation replay changed assessment: %+v %v", same, err)
	}
	retry, err := s.Submit(ctx, p.ID, "semantic-failed-retry", p.Answer, false)
	if err != nil || !retry.Pending || retry.AssessmentID == p.AssessmentID {
		t.Fatalf("new operation did not stage explicit retry: %+v %v", retry, err)
	}
	oldReplay, err := s.Submit(ctx, p.ID, "semantic-failed", p.Answer, false)
	if err != nil || oldReplay.Pending || oldReplay.AssessmentID != p.AssessmentID || oldReplay.AssessmentStatus != "failed" || oldReplay.Answer != p.Answer {
		t.Fatalf("old failed replay inherited newer assessment: %+v %v", oldReplay, err)
	}
	var retryTransmissions int
	if err = s.db.QueryRowContext(ctx, "SELECT transmissions FROM semantic_assessments WHERE id=?", retry.AssessmentID).Scan(&retryTransmissions); err != nil || retryTransmissions != 0 {
		t.Fatalf("old replay resumed the newer assessment: transmissions=%d err=%v", retryTransmissions, err)
	}
	// A provable no-send failure releases its reservation.
	retryLease := beginSemantic(t, s, retry.AssessmentID)
	if _, err = s.FailAssessment(ctx, retry.AssessmentID, retryLease.Token, AssessmentResult{Error: "unconfigured", NoSend: true}); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT reserved_micros FROM semantic_assessments WHERE id=?", retry.AssessmentID).Scan(&reserved); err != nil || reserved != 0 {
		t.Fatalf("no-send failure kept a reservation: %d %v", reserved, err)
	}
}

func TestSemanticFinalizeSupersededChangesOnlyAssessment(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	src := publishFixture(t, s, authoredSemantic())
	p := stageSemantic(t, s, "semantic-stale", "Validators let a server check a cached copy.")
	lease := beginSemantic(t, s, p.AssessmentID)
	updated := authoredSemantic()
	updated.Prompt = "Edited future semantic prompt?"
	if _, err := s.EditQuiz(ctx, src.Quizzes[0].ID, 1, updated); err != nil {
		t.Fatal(err)
	}
	finalized, err := s.FinalizeAssessment(ctx, p.AssessmentID, lease.Token, semanticResult([]float64{0.99, 0.99}, "equivalent", 0.99, 0.01, 0.01))
	if err != nil {
		t.Fatal(err)
	}
	if finalized.AssessmentStatus != "superseded" || finalized.Graded {
		t.Fatalf("stale finalization changed presentation: %+v", finalized)
	}
	var status string
	var events int
	if err = s.db.QueryRowContext(ctx, "SELECT status FROM semantic_assessments WHERE id=?", p.AssessmentID).Scan(&status); err != nil || status != "superseded" {
		t.Fatalf("assessment not superseded: %q %v", status, err)
	}
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM review_events").Scan(&events); err != nil || events != 0 {
		t.Fatalf("superseded assessment wrote event: %d %v", events, err)
	}
}

func TestSemanticShadowClassesAreRecordedButNeverApplied(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		result AssessmentResult
		class  string
	}{
		{"incomplete", semanticResult([]float64{0.10, 0.94}, "partial", 0.91, 0.02, 0.01), "incomplete"},
		{"incorrect", semanticResult([]float64{0.96, 0.93}, "different", 0.95, 0.97, 0.01), "incorrect"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestStore(t)
			publishFixture(t, s, authoredSemantic())
			p := stageSemantic(t, s, "semantic-shadow-"+tc.name, "The server can tell whether it changed.")
			lease := beginSemantic(t, s, p.AssessmentID)
			shadow, err := s.FinalizeAssessment(ctx, p.AssessmentID, lease.Token, tc.result)
			if err != nil {
				t.Fatal(err)
			}
			if shadow.Graded || shadow.Assisted || shadow.Pending || shadow.Outcome != "ungraded" || shadow.AssessmentDecision != "" || shadow.AssessmentDetail != "" {
				t.Fatalf("shadow class reached the learner: %+v", shadow)
			}
			var decision string
			var applied bool
			var exposures, events int
			if err = s.db.QueryRowContext(ctx, "SELECT decision,applied FROM semantic_assessments WHERE id=?", p.AssessmentID).Scan(&decision, &applied); err != nil {
				t.Fatal(err)
			}
			if decision != tc.class || applied {
				t.Fatalf("shadow class not recorded for evaluation: decision=%q applied=%v", decision, applied)
			}
			if err = s.db.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM assistance_exposures),(SELECT count(*) FROM review_events)").Scan(&exposures, &events); err != nil || exposures != 0 || events != 0 {
				t.Fatalf("shadow class wrote exposure or event: exposures=%d events=%d %v", exposures, events, err)
			}
			// The occurrence remains ungraded; an exact reveal still works.
			if _, err = s.Submit(ctx, p.ID, "shadow-reveal-"+tc.name, "", true); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Next(ctx, p.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// enabledStore grants the incomplete and incorrect classes authority so the
// fenced paths can be exercised before holdout evidence enables them.
func enabledStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	s, now := newTestStore(t)
	params := learning.SemanticV1Params()
	params.IncompleteEnabled, params.IncorrectEnabled = true, true
	s.semantic = &params
	return s, now
}

func TestSemanticCueFencesAssistanceAndLaterCorrectIsWarm(t *testing.T) {
	s, now := enabledStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredSemantic())
	p := stageSemantic(t, s, "semantic-incomplete", "The server can tell whether it changed.")
	var scheduleVersion int
	if err := s.db.QueryRowContext(ctx, "SELECT version FROM schedules LIMIT 1").Scan(&scheduleVersion); err != nil {
		t.Fatal(err)
	}
	lease := beginSemantic(t, s, p.AssessmentID)
	incomplete, err := s.FinalizeAssessment(ctx, p.AssessmentID, lease.Token, semanticResult([]float64{0.10, 0.94}, "partial", 0.91, 0.02, 0.01))
	if err != nil {
		t.Fatal(err)
	}
	if incomplete.Graded || !incomplete.Assisted || incomplete.AssessmentDecision != "incomplete" || incomplete.AssessmentDetail == "" {
		t.Fatalf("cue was not assistance-fenced: %+v", incomplete)
	}
	var exposures int
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM assistance_exposures WHERE presentation_id=? AND kind='cue'", p.ID).Scan(&exposures); err != nil || exposures != 1 {
		t.Fatalf("cue exposure not durable in the same write: %d %v", exposures, err)
	}
	warm, err := s.Submit(ctx, p.ID, "semantic-after-cue", authoredSemantic().Answer, false)
	if err != nil {
		t.Fatal(err)
	}
	if !warm.Graded || !warm.Assisted || warm.Outcome != "warm_correct" || warm.Rating != 0 {
		t.Fatalf("post-cue exact answer manufactured cold success: %+v", warm)
	}
	var afterVersion int
	if err = s.db.QueryRowContext(ctx, "SELECT version FROM schedules LIMIT 1").Scan(&afterVersion); err != nil || afterVersion != scheduleVersion {
		t.Fatalf("warm completion changed FSRS version: %d -> %d (%v)", scheduleVersion, afterVersion, err)
	}
	if _, err = s.Next(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current != nil || state.NextDueAt != now.Add(24*time.Hour).UnixMilli() {
		t.Fatalf("warm availability fence failed: %+v %v", state, err)
	}
}

func TestSemanticExposureFencesLaterOccurrenceAcrossPresentations(t *testing.T) {
	s, now := enabledStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredSemantic())
	p := stageSemantic(t, s, "semantic-exposed", "The server can tell whether it changed.")
	lease := beginSemantic(t, s, p.AssessmentID)
	if _, err := s.FinalizeAssessment(ctx, p.AssessmentID, lease.Token, semanticResult([]float64{0.10, 0.94}, "partial", 0.91, 0.02, 0.01)); err != nil {
		t.Fatal(err)
	}
	// The learner walks away without answering; the occurrence ends ungraded.
	if _, err := s.Submit(ctx, p.ID, "semantic-exposed-reveal", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Next(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	// A new occurrence of the same content 12h later must still be warm,
	// because the durable exposure record outlives the earlier presentation.
	*now = now.Add(12 * time.Hour)
	if _, err := s.db.ExecContext(ctx, "UPDATE schedules SET due_at=?", now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.ID == p.ID {
		t.Fatalf("no fresh occurrence after reveal: %+v %v", state, err)
	}
	later, err := s.Submit(ctx, state.Current.ID, "semantic-exposed-later", authoredSemantic().Answer, false)
	if err != nil {
		t.Fatal(err)
	}
	if !later.Graded || !later.Assisted || later.Outcome != "warm_correct" || later.Rating != 0 {
		t.Fatalf("exposure record did not fence the later occurrence: %+v", later)
	}
	// Past the 24h window the same content is cold again.
	if _, err = s.Next(ctx, state.Current.ID); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(37 * time.Hour)
	if _, err = s.db.ExecContext(ctx, "UPDATE schedules SET due_at=?", now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	state, err = s.Review(ctx)
	if err != nil || state.Current == nil {
		t.Fatalf("no occurrence after window: %+v %v", state, err)
	}
	cold, err := s.Submit(ctx, state.Current.ID, "semantic-exposed-cold", authoredSemantic().Answer, false)
	if err != nil || cold.Assisted || !cold.Graded || cold.Outcome != "correct" {
		t.Fatalf("expired exposure still fenced: %+v %v", cold, err)
	}
}

func TestSemanticIncorrectFeedbackRecordsExposureAndReschedules(t *testing.T) {
	s, _ := enabledStore(t)
	ctx := context.Background()
	publishFixture(t, s, authoredSemantic())
	p := stageSemantic(t, s, "semantic-incorrect", "Validators force every response body to be downloaded again.")
	lease := beginSemantic(t, s, p.AssessmentID)
	incorrect, err := s.FinalizeAssessment(ctx, p.AssessmentID, lease.Token, semanticResult([]float64{0.96, 0.93}, "different", 0.95, 0.97, 0.01))
	if err != nil {
		t.Fatal(err)
	}
	if !incorrect.Graded || incorrect.Outcome != "wrong" || incorrect.Rating != 1 || incorrect.AssessmentDecision != "incorrect" || incorrect.AssessmentDetail == "" {
		t.Fatalf("incorrect was not applied: %+v", incorrect)
	}
	var exposures int
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM assistance_exposures WHERE presentation_id=? AND kind='feedback'", p.ID).Scan(&exposures); err != nil || exposures != 1 {
		t.Fatalf("feedback exposure not recorded: %d %v", exposures, err)
	}
}

func TestEditQuizValidatesSemanticRubricAndDeterministicTaskBoundary(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	src := publishFixture(t, s, GeneratedQuiz{
		Kind: "recall", Prompt: "Original exact recall?", Answer: "Original answer", Explanation: "Original explanation.", Basis: "topic",
	})
	base := authoredSemantic()
	cases := []struct {
		name   string
		mutate func(*GeneratedQuiz)
	}{
		{"exact carries rubric", func(q *GeneratedQuiz) { q.Grading = "exact" }},
		{"semantic choice", func(q *GeneratedQuiz) { q.Kind = "choice"; q.Choices = []string{q.Answer, "Other"} }},
		{"missing required ideas", func(q *GeneratedQuiz) { q.Rubric.Required = nil }},
		{"cue leaks answer", func(q *GeneratedQuiz) { q.Rubric.Required[0].Cue = q.Answer }},
		{"cue leaks idea", func(q *GeneratedQuiz) { q.Rubric.Required[0].Cue = "Review: " + q.Rubric.Required[0].Text }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := base
			rubric := *base.Rubric
			rubric.Required = append([]RubricIdea(nil), base.Rubric.Required...)
			rubric.Contradictions = append([]RubricClaim(nil), base.Rubric.Contradictions...)
			q.Rubric = &rubric
			tc.mutate(&q)
			if _, err := s.EditQuiz(ctx, src.Quizzes[0].ID, 1, q); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid semantic quiz accepted: %v", err)
			}
		})
	}
	if edited, err := s.EditQuiz(ctx, src.Quizzes[0].ID, 1, base); err != nil || edited.Grading != "semantic" || edited.Rubric == nil {
		t.Fatalf("valid semantic rubric rejected: %+v %v", edited, err)
	}
	// The authored mode is the contract: a semantic explanation may contain
	// digits or symbols, and a plain word may still demand exactness.
	numeric := base
	numeric.Answer = "Compare HTTP/2 to HTTP/1.1: one connection carries 100s of multiplexed streams."
	if edited, err := s.EditQuiz(ctx, src.Quizzes[0].ID, 2, numeric); err != nil || edited.Grading != "semantic" {
		t.Fatalf("semantic explanation with digits and symbols rejected: %+v %v", edited, err)
	}
	exactWord := GeneratedQuiz{Kind: "recall", Grading: "exact", Prompt: "Which HTTP header carries a strong cache validator?", Answer: "ETag", Explanation: "ETag is the validator header.", Basis: "topic"}
	if edited, err := s.EditQuiz(ctx, src.Quizzes[0].ID, 3, exactWord); err != nil || edited.Grading != "exact" || edited.Rubric != nil {
		t.Fatalf("exact letter-only identifier rejected or relabeled: %+v %v", edited, err)
	}
}

func TestSemanticRubricLimitsCountCharactersNotUTF8Bytes(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	src := publishFixture(t, s, GeneratedQuiz{
		Kind: "recall", Prompt: "Original exact recall?", Answer: "Original answer", Explanation: "Original explanation.", Basis: "topic",
	})
	q := authoredSemantic()
	q.Rubric.Required[0].Cue = strings.Repeat("界", 200)
	q.Rubric.Contradictions[0].Feedback = strings.Repeat("界", 400)
	if _, err := s.EditQuiz(ctx, src.Quizzes[0].ID, 1, q); err != nil {
		t.Fatalf("valid character-bounded rubric rejected: %v", err)
	}
}

func TestSchemaV3ToV4MigrationAndAssessmentExport(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "populated-v3.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, schemaV1); err == nil {
		_, err = db.ExecContext(ctx, schemaV2)
	}
	if err == nil {
		_, err = db.ExecContext(ctx, schemaV3)
	}
	if err != nil {
		t.Fatal(err)
	}
	card, err := marshal(learning.NewCard(time.UnixMilli(1_700_000_000_000)))
	if err != nil {
		t.Fatal(err)
	}
	quizJSON := `{"kind":"recall","prompt":"Old v3 prompt","answer":"Old answer","explanation":"Old explanation","basis":"topic","choices":null,"variants":null}`
	statements := []string{
		`INSERT INTO sources VALUES('src','old topic','topic',1,0,1700000000000)`,
		`INSERT INTO source_revisions VALUES('src',1,'old topic','topic',1700000000000)`,
		`INSERT INTO jobs VALUES('job','src',1,'complete','','old-model','old-prompt',1,1700000000000,1700000000000,1700000000000,'',0,1,'{}')`,
		`INSERT INTO quizzes VALUES('quiz','src',1,0,1700000000000,'job',0)`,
		`INSERT INTO quiz_versions VALUES('quiz',1,'` + quizJSON + `','old-model','old-prompt',1700000000000)`,
		`INSERT INTO schedules VALUES('quiz',1,'` + card + `',1700000000000,'` + learning.Algorithm + `')`,
		`INSERT INTO presentations VALUES('presentation','quiz',1,1,'` + quizJSON + `',1700000000001,'Old answer','correct',0,1,3,1700000000000,1700000000002,'review')`,
		`INSERT INTO review_events VALUES('review','presentation','` + quizJSON + `','Old answer','correct',3,0,1700000000002,1700000000000,'` + learning.Algorithm + `','` + card + `','` + card + `',1,1)`,
		`INSERT INTO concepts VALUES('concept','Old concept','Preserved',1700000000000)`,
		`INSERT INTO concept_quizzes VALUES('concept','quiz',1700000000000)`,
	}
	for _, statement := range statements {
		if _, err = db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	readonly, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := readonly.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = CheckSchema(ctx, tx, 3); err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(tx.Rollback(), readonly.Close()); err != nil {
		t.Fatal(err)
	}
	afterCheck, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(before, afterCheck) {
		t.Fatalf("v3 check mutated database: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version, sources, concepts, reviews int
	var migratedGrading string
	if err = s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err == nil {
		err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM sources").Scan(&sources)
	}
	if err == nil {
		err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM concepts").Scan(&concepts)
	}
	if err == nil {
		err = s.db.QueryRowContext(ctx, "SELECT count(*),min(grading) FROM review_events").Scan(&reviews, &migratedGrading)
	}
	if err != nil || version != 4 || sources != 1 || concepts != 1 || reviews != 1 || migratedGrading != "exact-v1" {
		t.Fatalf("migration did not preserve v3 rows: version=%d sources=%d concepts=%d reviews=%d grading=%q err=%v", version, sources, concepts, reviews, migratedGrading, err)
	}

	// Add a semantic assessment in the migrated store and prove both new export
	// sections and the review-event grading column are portable.
	semantic := authoredSemantic()
	if _, err = s.EditQuiz(ctx, "quiz", 1, semantic); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil {
		t.Fatalf("review migrated quiz: %+v %v", state, err)
	}
	staged, err := s.Submit(ctx, state.Current.ID, "migrated-semantic", "Validators support cache checks.", false)
	if err != nil || !staged.Pending {
		t.Fatalf("stage on migrated DB: %+v %v", staged, err)
	}
	exported, err := s.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err = json.Unmarshal(exported, &document); err != nil {
		t.Fatal(err)
	}
	var assessments []map[string]any
	var contentAssessments []map[string]any
	if err = json.Unmarshal(document["semantic_assessments"], &assessments); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(document["content_assessments"], &contentAssessments); err != nil {
		t.Fatal(err)
	}
	if len(assessments) != 1 || len(contentAssessments) != 0 || assessments[0]["request_json"] == nil {
		t.Fatalf("new assessment sections missing from export: semantic=%v content=%v", assessments, contentAssessments)
	}
}
