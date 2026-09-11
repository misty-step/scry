package store

import (
	"context"
	"testing"
	"time"
)

func TestFoundationExpiredAttemptPausesAndSupersededTargetFreesQueue(t *testing.T) {
	ctx := context.Background()
	s, now := newTestStore(t)
	publishFixture(t, s, authoredChoice("Original target?"))
	p, id, _ := startFoundation(t, s)
	*now = now.Add(2 * time.Minute)
	claim, err := s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || claim != nil {
		t.Fatal("expired uncertain foundation automatically retried")
	}
	b, err := s.FoundationBridge(ctx, id)
	if err != nil || b.Job.Status != "paused" || !b.Job.CostUnknown || b.Job.CostMicros != 100 {
		t.Fatalf("expiry lost unknown reservation: %+v %v", b, err)
	}
	if _, err = s.EditQuiz(ctx, p.Quiz.ID, p.Quiz.Version, authoredChoice("Corrected target?")); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	newID, err := s.RequestFoundation(ctx, state.Current.ID, "new-target-foundations", "", state.Current.BridgeRevision)
	if err != nil {
		t.Fatalf("superseded job pinned source queue: %v", err)
	}
	claim, err = s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || claim == nil || claim.FoundationTarget.Prompt != "Corrected target?" {
		t.Fatalf("new target could not claim bounded work: %+v %v", claim, err)
	}
	fresh, err := s.FoundationBridge(ctx, newID)
	if err != nil || fresh.SourceRevision != b.SourceRevision || fresh.PresentationID == b.PresentationID {
		t.Fatal("new target confused with historical occurrence")
	}
}
