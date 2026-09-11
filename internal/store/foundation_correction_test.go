package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestFoundationWarmConsumptionDoesNotStarveQueue(t *testing.T) {
	ctx := context.Background()
	s, now := newTestStore(t)
	source := publishFixture(t, s, authoredChoice("A?"), authoredChoice("B?"), authoredChoice("C?"))
	before := exportSection(t, s, "schedules")
	for _, prompt := range []string{"A?", "B?"} {
		p, bridge, job := startFoundation(t, s)
		if p.Quiz.Prompt != prompt {
			t.Fatalf("want %s, got %s", prompt, p.Quiz.Prompt)
		}
		finishFoundation(t, s, job)
		stepFoundation(t, s, bridge, "open", "")
		if _, err := s.Submit(ctx, p.ID, newID(), "second", false); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Next(ctx, p.ID); err != nil {
			t.Fatal(err)
		}
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.Quiz.Prompt != "C?" {
		t.Fatalf("warm A/B starved C: %+v %v", state, err)
	}
	if state.Due != 1 || state.Preview != nil {
		t.Fatalf("consumed warm quizzes remain due/previewable: %+v", state)
	}
	if !reflect.DeepEqual(before, exportSection(t, s, "schedules")) {
		t.Fatal("consumption changed FSRS")
	}
	summary, err := s.Summary(ctx)
	if err != nil || summary.Due != 1 || summary.NextDueAt != now.Add(24*time.Hour).UnixMilli() {
		t.Fatalf("availability summary disagrees: %+v %v", summary, err)
	}
	library, err := s.Source(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range library.Quizzes[:2] {
		if q.DueAt != now.UnixMilli() || q.AvailableAt != now.Add(24*time.Hour).UnixMilli() {
			t.Fatalf("library conflated FSRS and availability: %+v", q)
		}
	}
	// A newer explicit schedule reset overrides the old warm consumption.
	var warmReview string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM review_events WHERE presentation_id IN (SELECT id FROM presentations WHERE quiz_id=?)", source.Quizzes[0].ID).Scan(&warmReview); err != nil {
		t.Fatal(err)
	}
	if err := s.Dispute(ctx, warmReview, "Synthetic deliberate schedule reset", true); err != nil {
		t.Fatal(err)
	}
	summary, err = s.Summary(ctx)
	if err != nil || summary.Due != 2 {
		t.Fatalf("old consumption overrode newer reset: %+v %v", summary, err)
	}
}

func TestFoundationSoleWarmConsumptionSurvivesReloadAndRestart(t *testing.T) {
	ctx := context.Background()
	s, now := newTestStore(t)
	publishFixture(t, s, authoredChoice("A?"))
	p, bridge, job := startFoundation(t, s)
	finishFoundation(t, s, job)
	stepFoundation(t, s, bridge, "open", "")
	if _, err := s.Submit(ctx, p.ID, newID(), "second", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Next(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	path := s.Path()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openAt(t, path, now)
	state, err := s.Review(ctx)
	if err != nil || state.Current != nil || state.Due != 0 || state.NextDueAt != now.Add(24*time.Hour).UnixMilli() {
		t.Fatalf("reload resurrected consumed warm quiz: %+v %v", state, err)
	}
	*now = now.Add(24 * time.Hour)
	state, err = s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.Quiz.ID != p.Quiz.ID {
		t.Fatalf("availability never returned: %+v %v", state, err)
	}
}

func TestFoundationRepeatedHelpAcknowledgesLatestDraft(t *testing.T) {
	ctx := context.Background()
	s, now := newTestStore(t)
	publishFixture(t, s, authoredChoice("Original?"))
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.RequestFoundation(ctx, state.Current.ID, "draft-first", "first draft", state.Current.BridgeRevision)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RequestFoundation(ctx, first.ID, "draft-second", "revised draft", first.BridgeRevision); err != nil {
		t.Fatal(err)
	}
	path := s.Path()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openAt(t, path, now)
	current, err := s.Current(ctx)
	if err != nil || current.Draft != "revised draft" {
		t.Fatalf("acknowledged revised draft was lost: %+v %v", current, err)
	}
	if replayed, err := s.RequestFoundation(ctx, state.Current.ID, "draft-first", "first draft", state.Current.BridgeRevision); err != nil || replayed != id {
		t.Fatalf("original operation receipt changed: %s %v", replayed, err)
	}
	current, err = s.Current(ctx)
	if err != nil || current.Draft != "revised draft" {
		t.Fatal("exact old retry reverted newer draft")
	}
	job, err := s.ClaimJob(ctx, time.Minute, 100, 10000)
	if err != nil || job == nil {
		t.Fatalf("saved job unavailable: %v", err)
	}
	finishFoundation(t, s, job)
	before := stepFoundation(t, s, id, "open", "")
	if _, err = s.RequestFoundation(ctx, first.ID, "draft-stale", "stale overwrite", first.BridgeRevision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale draft acknowledged: %v", err)
	}
	if _, err = s.RequestFoundation(ctx, first.ID, "draft-second", "revised draft", first.BridgeRevision); err != nil {
		t.Fatal(err)
	}
	after, err := s.FoundationBridge(ctx, id)
	if err != nil || after.Revision != before.Revision || after.Phase != before.Phase || after.Position != before.Position {
		t.Fatal("old request reverted newer bridge progress")
	}
	current, err = s.Current(ctx)
	if err != nil || current.Draft != "revised draft" {
		t.Fatal("stale request changed latest draft")
	}
	if _, err = s.RequestFoundation(ctx, current.ID, "clear-draft", "", current.BridgeRevision); err != nil {
		t.Fatal(err)
	}
	current, err = s.Current(ctx)
	if err != nil || current.Draft != "" {
		t.Fatal("acknowledged draft clearing was lost")
	}
}

func TestFoundationOldWarmConsumptionCannotOverrideExplicitReveal(t *testing.T) {
	ctx := context.Background()
	s, now := newTestStore(t)
	source := publishFixture(t, s, authoredChoice("Original?"))
	p, bridge, job := startFoundation(t, s)
	finishFoundation(t, s, job)
	stepFoundation(t, s, bridge, "open", "")
	if _, err := s.Submit(ctx, p.ID, newID(), "second", false); err != nil {
		t.Fatal(err)
	}
	// A previously opened duplicate can survive an upgrade from the old selector.
	// Its explicit Reveal must remain a real Again, not inherit the old fence.
	_, err := s.db.ExecContext(ctx, `INSERT INTO presentations(id,quiz_id,content_version,schedule_version,snapshot,created_at,due_at)
	 SELECT 'opened-before-fence',quiz_id,content_version,schedule_version,snapshot,?,due_at FROM presentations WHERE id=?`, now.UnixMilli(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE review_session SET current_id='opened-before-fence'"); err != nil {
		t.Fatal(err)
	}
	again, err := s.Submit(ctx, "opened-before-fence", newID(), "", true)
	if err != nil || again.Rating != 1 || !again.Assisted {
		t.Fatalf("explicit Reveal lost real Again: %+v %v", again, err)
	}
	library, err := s.Source(ctx, source.ID)
	if err != nil || library.Quizzes[0].AvailableAt != again.DueAt || library.Quizzes[0].DueAt != again.DueAt {
		t.Fatalf("old warm completion overrode Reveal schedule: %+v %v", library, err)
	}
	*now = time.UnixMilli(again.DueAt)
	state, err := s.Next(ctx, again.ID)
	if err != nil || state.Current == nil || state.Current.Quiz.ID != p.Quiz.ID {
		t.Fatalf("Again was not available when due: %+v %v", state, err)
	}
}
