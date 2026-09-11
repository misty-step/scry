package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"html"
	"net/http/httptest"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/store"
)

func TestOldFoundationJSONCannotExposeLaterColdTarget(t *testing.T) {
	ctx := context.Background()
	s, app := privateApp(t)
	if _, err := s.Capture(ctx, "Synthetic cold-boundary topic", "cold-boundary-capture"); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, time.Minute, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	cost := int64(0)
	q := store.GeneratedQuiz{Kind: "recall", Prompt: "Name the synthetic electron donor.", Answer: "secret-answer", Explanation: "secret-explanation", Variants: []string{"secret-variant"}, Basis: "topic"}
	if err = s.CompleteJob(ctx, job.ID, job.LeaseToken, store.GenerationResult{Quizzes: []store.GeneratedQuiz{q}, Model: "synthetic-authored", PromptVersion: "test"}, &cost); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	oldID := state.Current.ID
	bridge, err := s.RequestFoundation(ctx, oldID, "old-foundation", "secret-draft", state.Current.BridgeRevision)
	if err != nil {
		t.Fatal(err)
	}
	job, err = s.ClaimJob(ctx, time.Minute, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	content := &store.FoundationContent{Units: []store.FoundationUnit{{Key: "donor", Definition: "Synthetic donor definition", Kind: "foundation"}}, Materials: []store.FoundationMaterial{
		{Kind: "instruction", Title: "Donor instruction", Body: "The synthetic donor supplies electrons rather than carbon atoms.", Links: []store.FoundationLink{{Unit: "donor", Role: "teaches", Provenance: "Synthetic authored"}}},
		{Kind: "practice", Title: "Donor practice", Quiz: &q, Links: []store.FoundationLink{{Unit: "donor", Role: "directly-assesses", Provenance: "Synthetic authored"}}},
	}}
	if err = s.CompleteJob(ctx, job.ID, job.LeaseToken, store.GenerationResult{Foundation: content, Model: "synthetic-authored", PromptVersion: "test"}, &cost); err != nil {
		t.Fatal(err)
	}
	b, err := s.FoundationBridge(ctx, bridge)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdvanceFoundation(ctx, bridge, b.Revision, "old-read", "open", ""); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"next", "answer"} {
		b, err = s.FoundationBridge(ctx, bridge)
		if err != nil {
			t.Fatal(err)
		}
		answer := ""
		if action == "answer" {
			answer = "secret-answer"
		}
		if err = s.AdvanceFoundation(ctx, bridge, b.Revision, "old-"+action, action, answer); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.Submit(ctx, oldID, "old-warm-answer", "secret-answer", false); err != nil {
		t.Fatal(err)
	}
	history, err := s.History(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic future occurrence: mirror the unchanged quiz/schedule version
	// after the existing exposure window, without altering immutable old history.
	db, err := sql.Open("sqlite", s.Path())
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO presentations(id,quiz_id,content_version,schedule_version,snapshot,created_at,due_at)
	 SELECT 'later-cold',quiz_id,content_version,schedule_version,snapshot,?,due_at FROM presentations WHERE id=?`, time.Now().Add(25*time.Hour).UnixMilli(), oldID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "UPDATE review_session SET current_id='later-cold'"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	r := ownerRequest("GET", "/foundations/"+bridge, nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("old bridge JSON: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Bridge store.FoundationBridge `json:"bridge"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	target := response.Bridge.Target
	if response.Bridge.Phase != "ready" || response.Bridge.Current != nil || response.Bridge.Answer != "" || target.Quiz.Answer != "" || target.Quiz.Explanation != "" || target.Quiz.Evidence != "" || len(target.Quiz.Variants) != 0 || target.Answer != "" || target.Draft != "" {
		t.Fatalf("unacknowledged old-bridge JSON disclosed answers: %s", w.Body.String())
	}
	after, err := s.History(ctx, 100)
	if err != nil || !reflect.DeepEqual(history, after) {
		t.Fatal("redaction rewrote explicit past history")
	}
}

func TestFoundationLatestDraftWinsEditableTextAfterUngradedAttempt(t *testing.T) {
	ctx := context.Background()
	s, app := privateApp(t)
	if _, err := s.Capture(ctx, "Synthetic editable draft precedence", "draft-capture"); err != nil {
		t.Fatal(err)
	}
	job, err := s.ClaimJob(ctx, time.Minute, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	cost := int64(0)
	if err = s.CompleteJob(ctx, job.ID, job.LeaseToken, store.GenerationResult{Quizzes: []store.GeneratedQuiz{{Kind: "recall", Prompt: "Name the synthetic donor.", Answer: "NADPH", Explanation: "NADPH supplies reducing electrons.", Basis: "topic"}}, Model: "synthetic-authored", PromptVersion: "test"}, &cost); err != nil {
		t.Fatal(err)
	}
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := state.Current.ID
	if p, err := s.Submit(ctx, id, "earlier-unclear", "nadph", false); err != nil || p.Graded {
		t.Fatalf("expected retained ungraded attempt: %+v %v", p, err)
	}
	request := func(op, draft string) {
		t.Helper()
		p, err := s.Current(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.RequestFoundation(ctx, id, op, draft, p.BridgeRevision); err != nil {
			t.Fatal(err)
		}
	}
	assertEditable := func(want string) {
		t.Helper()
		w := httptest.NewRecorder()
		app.ServeHTTP(w, ownerRequest("GET", "/", nil))
		match := regexp.MustCompile(`(?s)<textarea[^>]*id="recall-answer"[^>]*>(.*?)</textarea>`).FindStringSubmatch(w.Body.String())
		if w.Code != 200 || len(match) != 2 || html.UnescapeString(match[1]) != want {
			t.Fatalf("acknowledged editable draft not shown: want %q, match %q, status %d", want, match, w.Code)
		}
	}
	request("draft-first", "first draft")
	request("draft-revised", "revised acknowledged draft")
	assertEditable("revised acknowledged draft")
	if p, err := s.Submit(ctx, id, "newer-unclear", "Nadph", false); err != nil || p.Graded {
		t.Fatalf("unexpected grade: %+v %v", p, err)
	}
	assertEditable("Nadph")
	request("draft-clear", "")
	assertEditable("")
}
