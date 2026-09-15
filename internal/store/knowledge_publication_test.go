package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestReferenceReuseIsUsefulWithoutDuplicatingContentOrClaimingRecall(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	original := publishKnowledgeFixture(t, s, "Original reusable foundation", referenceBundle())
	goal, err := s.Goal(ctx, original.GoalID)
	if err != nil {
		t.Fatal(err)
	}
	reuse := referenceBundle()
	reuse.Units[0].ReuseID = goal.Units[0].ID
	reuse.Materials[0].ReuseID = original.Materials[0].ID
	second := publishKnowledgeFixture(t, s, "Another chosen goal sharing the foundation", reuse)
	if second.Job.Published != 1 || second.Job.ReusedMaterials != 1 || second.Job.NewMaterials != 0 || second.Job.NewQuizzes != 0 {
		t.Fatalf("useful reuse-only output treated as empty: %+v", second.Job)
	}
	var materials, versions, reviews, schedules int
	for _, query := range []struct {
		sql    string
		result *int
	}{
		{"SELECT count(*) FROM materials", &materials}, {"SELECT count(*) FROM material_versions", &versions},
		{"SELECT count(*) FROM review_events", &reviews}, {"SELECT count(*) FROM schedules", &schedules},
	} {
		if err = s.db.QueryRow(query.sql).Scan(query.result); err != nil {
			t.Fatal(err)
		}
	}
	if materials != 1 || versions != 1 || reviews != 0 || schedules != 0 {
		t.Fatalf("reuse created duplicate content or learning: material=%d versions=%d events=%d cards=%d", materials, versions, reviews, schedules)
	}
	shared, err := s.Goal(ctx, second.GoalID)
	if err != nil || len(shared.Materials) != 1 || shared.Materials[0].ID != original.Materials[0].ID {
		t.Fatalf("new goal cannot use shared reference: %+v %v", shared, err)
	}
}

func TestStalePaidBridgeCannotPublishAndExactRetryRetainsSpend(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	src := publishFixture(t, s, authoredChoice("Which target needs prerequisite help?"))
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := s.StartBridge(ctx, state.Current.ID, "request-stale-bridge")
	if err != nil || bridge.Bridge == nil || bridge.Bridge.JobID == "" {
		t.Fatalf("request failed to retain pending bridge: %+v %v", bridge, err)
	}
	claim, err := s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || claim == nil || claim.Kind != "bridge" {
		t.Fatalf("bridge claim: %+v %v", claim, err)
	}
	g, err := s.Goal(ctx, src.GoalID)
	if err != nil {
		t.Fatal(err)
	}
	settings := g.Settings
	settings.ExpectedRevision = g.Revision
	settings.Focus = "foundation"
	settings.Reason = "Reconsider the retained target without authorizing stale output"
	if _, err = s.PlanGoal(ctx, g.ID, settings, "change-after-paid-claim"); err != nil {
		t.Fatal(err)
	}
	cost := int64(29)
	if err = s.CompleteJob(ctx, claim.ID, claim.LeaseToken, referenceBundle(), &cost); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale goal revision published: %v", err)
	}
	var publications int
	if err = s.db.QueryRow("SELECT count(*) FROM materials WHERE origin_job_id=?", claim.ID).Scan(&publications); err != nil || publications != 0 {
		t.Fatalf("stale generation escaped atomic fence: %d %v", publications, err)
	}
	var accounted int64
	if err = s.db.QueryRow("SELECT cost_micros FROM job_attempts WHERE token=?", claim.LeaseToken).Scan(&accounted); err != nil || accounted != cost {
		t.Fatalf("stale content rejection lost real charge: %d %v", accounted, err)
	}
	retried, err := s.RetryJob(ctx, claim.ID, "explicit-retry-exact-bridge")
	if err != nil || retried.ParentJobID != claim.ID || retried.ID == claim.ID || retried.Kind != "bridge" {
		t.Fatalf("bounded explicit retry lost target lineage: %+v %v", retried, err)
	}
	again, err := s.RetryJob(ctx, claim.ID, "explicit-retry-exact-bridge")
	if err != nil || again.ID != retried.ID {
		t.Fatalf("retry operation produced another paid-work candidate: %+v %v", again, err)
	}
	var oldContext, newContext string
	if err = s.db.QueryRow("SELECT context_json FROM jobs WHERE id=?", claim.ID).Scan(&oldContext); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT context_json FROM jobs WHERE id=?", retried.ID).Scan(&newContext); err != nil {
		t.Fatal(err)
	}
	if oldContext == "{}" || newContext != "{}" {
		t.Fatal("retry replaced original paid context or reused its authority")
	}
}

func TestBackgroundEnrichmentLeavesForegroundAllowanceAndPriority(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	src := publishFixture(t, s, authoredChoice("Which card awaits mapping maintenance?"))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = enqueueKnowledge(ctx, tx, src.ID, src.Revision, "enrich", "", 0, "", 0, s.now())
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// Move only the accounting day; no fake observation or schedule rewrite.
	*now = now.Add(25 * time.Hour)
	if claim, err := s.ClaimJob(ctx, time.Minute, 100, 100); !errors.Is(err, ErrBudget) || claim != nil {
		t.Fatalf("background work consumed the only foreground reservation: %+v %v", claim, err)
	}
	foreground, err := s.Capture(ctx, "Foreground chosen goal must not sit behind migration maintenance", "foreground-priority")
	if err != nil {
		t.Fatal(err)
	}
	claim, err := s.ClaimJob(ctx, time.Minute, 100, 100)
	if err != nil || claim == nil || claim.SourceID != foreground.ID || claim.Kind != "capture" {
		t.Fatalf("foreground priority/reserved allowance ignored: %+v %v", claim, err)
	}
	if other, err := s.ClaimJob(ctx, time.Minute, 100, 10000); err != nil || other != nil {
		t.Fatalf("two live paid requests admitted concurrently: %+v %v", other, err)
	}
}

func TestV1EmptyVariantReusePreservesRawVersionsAndHistoricalUnmapping(t *testing.T) {
	raw, path := populatedV1(t)
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err = s.PauseRestoredJobs(ctx); err != nil {
		t.Fatal(err)
	}
	old, err := s.Material(ctx, "historical-quiz")
	if err != nil {
		t.Fatal(err)
	}
	var before string
	if err = s.db.QueryRow("SELECT content FROM quiz_versions WHERE quiz_id='historical-quiz' AND version=2").Scan(&before); err != nil {
		t.Fatal(err)
	}
	q := GeneratedQuiz{Key: "reused", ReuseID: old.ID, Level: "target", Kind: old.Quiz.Kind, Prompt: old.Quiz.Prompt, Answer: old.Quiz.Answer, Explanation: old.Quiz.Explanation, Basis: old.Quiz.Basis, Evidence: old.Quiz.Evidence, Choices: append([]string{}, old.Quiz.Choices...), Variants: []string{}, EstimatedSeconds: 60, Links: []GeneratedLink{{UnitKey: "mapped", Role: "assesses"}}}
	bundle := GenerationResult{Model: "authored-reuse", PromptVersion: "reuse-v2", Coverage: CoverageReport{Kind: "concepts", Complete: false, Missing: []string{"Instruction remains unprepared"}}, Units: []GeneratedUnit{{Key: "mapped", Kind: "concept", Statement: "The synthetic choice differentiates first and second alternatives"}}, Quizzes: []GeneratedQuiz{q}}
	result := publishKnowledgeFixture(t, s, "Current goal may map future reuse without rewriting old evidence", bundle)
	if result.Job.ReusedMaterials != 1 || result.Job.NewQuizzes != 0 {
		t.Fatalf("valid empty-array reuse rejected: %+v", result.Job)
	}
	var after string
	if err = s.db.QueryRow("SELECT content FROM quiz_versions WHERE quiz_id='historical-quiz' AND version=2").Scan(&after); err != nil || before != after {
		t.Fatalf("reuse rewrote original JSON: %v", err)
	}
	var pastLinks int
	if err = s.db.QueryRow("SELECT count(*) FROM material_links WHERE material_id='historical-quiz' AND material_version<=2").Scan(&pastLinks); err != nil || pastLinks != 0 {
		t.Fatalf("reuse retroactively attached historical evidence: %d %v", pastLinks, err)
	}
	events, err := s.History(ctx, 10)
	if err != nil || len(events) != 1 || len(events[0].Coverage) != 0 || events[0].Quiz.Version != 1 {
		t.Fatalf("original review was reinterpreted: %+v %v", events, err)
	}
	if !reflect.DeepEqual(old.Quiz.Choices, q.Choices) {
		t.Fatal("fixture changed actual displayed alternatives")
	}
}

func TestChangedSchemaCannotClaimAgainstEarlierIntegrityApproval(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Capture(ctx, "A chosen goal before a foreign schema mutation", "schema-cookie-capture"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DROP TRIGGER immutable_material_links_update"); err != nil {
		t.Fatal(err)
	}
	if claim, err := s.ClaimJob(ctx, time.Minute, 100, 10000); !errors.Is(err, ErrInvalid) || claim != nil {
		t.Fatalf("schema mutation reused earlier integrity approval: %+v %v", claim, err)
	}
	var attempts int
	if err := s.db.QueryRow("SELECT count(*) FROM job_attempts").Scan(&attempts); err != nil || attempts != 0 {
		t.Fatalf("unvalidated claim created paid authority: %d %v", attempts, err)
	}
}
