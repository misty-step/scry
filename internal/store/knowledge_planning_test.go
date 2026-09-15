package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func planningDecision(t *testing.T, g Goal, kind string) PlanDecision {
	t.Helper()
	for i := len(g.Decisions) - 1; i >= 0; i-- {
		if g.Decisions[i].Kind == kind && g.Decisions[i].UndoneAt == 0 {
			return g.Decisions[i]
		}
	}
	t.Fatalf("no active %s decision in goal %+v", kind, g)
	return PlanDecision{}
}

func TestPlanReceiptsStaleAuthorityAndUndoPreserveCards(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	g := knowledgeFixture(t, s, 1, false, 0)
	assessment, err := s.Material(ctx, g.Materials[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Quiz == nil {
		t.Fatal("fixture material is not an assessment")
	}
	var beforeCard, afterCard string
	var beforeVersion, afterVersion int
	if err := s.db.QueryRow("SELECT card,version FROM schedules WHERE quiz_id=?", assessment.Quiz.ID).Scan(&beforeCard, &beforeVersion); err != nil {
		t.Fatal(err)
	}
	settings := PlanSettings{ExpectedRevision: g.Revision, TimeBudgetSeconds: 600, NewAssessmentsPerDay: 3, Focus: "foundation", Reason: "Use a longer foundation-focused per-goal epoch"}
	op := newID()
	first, err := s.PlanGoal(ctx, g.ID, settings, op)
	if err != nil {
		t.Fatal(err)
	}
	firstDecision := planningDecision(t, first, "plan")
	*now = now.Add(time.Second)
	secondSettings := PlanSettings{ExpectedRevision: first.Revision, TimeBudgetSeconds: 300, NewAssessmentsPerDay: 0, Focus: "practice", Reason: "Pause new assessments while practicing existing material"}
	second, err := s.PlanGoal(ctx, g.ID, secondSettings, newID())
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.PlanGoal(ctx, g.ID, settings, op)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(retry, first) {
		t.Fatalf("lost-response retry returned a newer plan, not its receipt: first=%+v retry=%+v", first, retry)
	}
	if _, err = s.PlanGoal(ctx, g.ID, settings, newID()); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale displayed revision was accepted: %v", err)
	}
	missingRevision := settings
	missingRevision.ExpectedRevision = 0
	if _, err = s.PlanGoal(ctx, g.ID, missingRevision, newID()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing authority silently became current revision: %v", err)
	}
	if _, err = s.UndoPlan(ctx, firstDecision.ID, newID()); !errors.Is(err, ErrConflict) {
		t.Fatalf("old undo overwrote a newer plan: %v", err)
	}
	*now = now.Add(time.Second)
	undoOp := newID()
	undone, err := s.UndoPlan(ctx, planningDecision(t, second, "plan").ID, undoOp)
	if err != nil {
		t.Fatal(err)
	}
	if undone.Settings.Focus != first.Settings.Focus || undone.Settings.TimeBudgetSeconds != first.Settings.TimeBudgetSeconds || undone.Settings.NewAssessmentsPerDay != first.Settings.NewAssessmentsPerDay {
		t.Fatalf("undo did not restore future pacing: %+v", undone.Settings)
	}
	if repeated, err := s.UndoPlan(ctx, planningDecision(t, second, "plan").ID, undoOp); err != nil || !reflect.DeepEqual(repeated, undone) {
		t.Fatalf("undo retry changed acknowledged state: %+v %v", repeated, err)
	}
	if err = s.db.QueryRow("SELECT card,version FROM schedules WHERE quiz_id=?", assessment.Quiz.ID).Scan(&afterCard, &afterVersion); err != nil {
		t.Fatal(err)
	}
	if beforeCard != afterCard || beforeVersion != afterVersion {
		t.Fatal("planning or undo rewrote direct FSRS state")
	}
}

func TestOwnerWideNewAllowanceCannotBeMultipliedByCaptures(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	g := knowledgeFixture(t, s, 5, false, 0)
	state, err := s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var presentations, observations int
	if err = s.db.QueryRow("SELECT (SELECT count(*) FROM materials WHERE first_presented_at>0),(SELECT count(*) FROM interactions)").Scan(&presentations, &observations); err != nil {
		t.Fatal(err)
	}
	if presentations != 1 || observations != 0 {
		t.Fatalf("publication/preview manufactured presentation or evidence: first=%d observations=%d", presentations, observations)
	}
	for range 5 {
		if state.Current == nil {
			t.Fatalf("allowance ended before five actual new assessments: %+v", state)
		}
		*now = now.Add(time.Second)
		if _, err = s.Submit(ctx, state.Current.ID, newID(), "second", false); err != nil {
			t.Fatal(err)
		}
		state, err = s.Next(ctx, state.Current.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if state.Current != nil {
		t.Fatalf("no sixth new material was authored: %+v", state)
	}
	other := knowledgeFixture(t, s, 1, false, 0)
	state, err = s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Current != nil {
		t.Fatalf("fresh capture multiplied the owner-wide five-new allowance: %+v", state.Current)
	}
	other, err = s.Goal(ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	paced := planningDecision(t, other, "pace")
	if paced.MaterialID == "" || paced.Policy == "" || paced.Reason == "" || paced.ReconsiderAt <= now.UnixMilli() {
		t.Fatalf("admission change was not inspectable: %+v", paced)
	}
	*now = now.Add(time.Second)
	if _, err = s.UndoPlan(ctx, paced.ID, newID()); err != nil {
		t.Fatal(err)
	}
	state, err = s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Current == nil || state.Current.Material.SourceID == g.SourceID || state.Current.Material.ID != paced.MaterialID {
		t.Fatalf("explicit pacing undo did not admit its exact target: %+v", state)
	}
}

func TestSuggestionExpansionRetryAndFreshProposalAuthority(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	g := knowledgeFixture(t, s, 1, false, 2)
	if len(g.Suggestions) != 2 {
		t.Fatalf("fixture needs two authored suggestions: %+v", g.Suggestions)
	}
	chosen, staleOther := g.Suggestions[0], g.Suggestions[1]
	*now = now.Add(time.Second)
	op := newID()
	accepted, err := s.ChooseSuggestion(ctx, chosen.ID, "accept", op)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := s.ChooseSuggestion(ctx, chosen.ID, "accept", op)
	if err != nil || !reflect.DeepEqual(accepted, repeated) {
		t.Fatalf("accepted choice retry was not exact: %+v %v", repeated, err)
	}
	var jobs, goals int
	var suggestionID string
	if err = s.db.QueryRow("SELECT count(*),COALESCE(min(suggestion_id),'') FROM jobs WHERE kind='expand'").Scan(&jobs, &suggestionID); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT count(*) FROM goals").Scan(&goals); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || goals != 1 || suggestionID != chosen.ID {
		t.Fatalf("unserved accepted scope was inert/duplicated/new goal: jobs=%d goals=%d target=%s", jobs, goals, suggestionID)
	}
	var replacement Suggestion
	for _, candidate := range accepted.Suggestions {
		if candidate.SupersedesID == staleOther.ID && candidate.Status == "pending" {
			replacement = candidate
		}
	}
	if replacement.ID == "" || replacement.ID == staleOther.ID || replacement.GoalRevision != accepted.Revision || !reflect.DeepEqual(replacement.UnitVersions, staleOther.UnitVersions) {
		t.Fatalf("undecided option lost immutable fresh-ID lineage: %+v", accepted.Suggestions)
	}
	if _, err = s.ChooseSuggestion(ctx, staleOther.ID, "decline", newID()); !errors.Is(err, ErrConflict) {
		t.Fatalf("old proposal ID gained new authority: %v", err)
	}
	if _, err = s.ChooseSuggestion(ctx, chosen.ID, "decline", op); !errors.Is(err, ErrConflict) {
		t.Fatalf("operation reused for a different choice: %v", err)
	}
}

func TestDeferralIsVisibleFixedAndUndoDoesNotChangeFSRS(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	g := knowledgeFixture(t, s, 2, false, 0)
	shared := g.Materials[0].Links[0]
	for _, m := range g.Materials {
		if m.Links[0].UnitID != shared.UnitID {
			if _, err := s.EditCoverage(ctx, m.ID, m.Version, []CoverageLink{{UnitID: shared.UnitID, UnitVersion: shared.UnitVersion, Role: "assesses"}}, "This alternative probe explicitly assesses the same exact foundation", newID()); err != nil {
				t.Fatal(err)
			}
		}
	}
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil {
		t.Fatalf("review fixture: %+v %v", state, err)
	}
	*now = now.Add(time.Second)
	saved, err := s.Submit(ctx, state.Current.ID, newID(), "second", false)
	if err != nil {
		t.Fatal(err)
	}
	state, err = s.Next(ctx, state.Current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Current != nil {
		t.Fatalf("eligible redundant probe was not deferred: %+v", state.Current)
	}
	g, err = s.Goal(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	decision := planningDecision(t, g, "defer")
	if decision.ReconsiderAt != saved.DueAt || !reflect.DeepEqual(decision.EvidenceIDs, []string{saved.ReviewID}) || len(decision.UnitVersions) != 1 {
		t.Fatalf("deferral lacks fixed actual event/card scope: %+v", decision)
	}
	var beforeCard, afterCard string
	var beforeVersion, afterVersion int
	m, err := s.Material(ctx, decision.MaterialID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT card,version FROM schedules WHERE quiz_id=?", m.Quiz.ID).Scan(&beforeCard, &beforeVersion); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Second)
	if _, err = s.Review(ctx); err != nil {
		t.Fatal(err)
	}
	g, err = s.Goal(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	fixed := planningDecision(t, g, "defer")
	if fixed.ID != decision.ID || fixed.ReconsiderAt != decision.ReconsiderAt {
		t.Fatalf("read slid or duplicated fixed deferral: before=%+v after=%+v", decision, fixed)
	}
	if _, err = s.UndoPlan(ctx, decision.ID, newID()); err != nil {
		t.Fatal(err)
	}
	state, err = s.Review(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Current == nil || state.Current.Material.ID != decision.MaterialID {
		t.Fatalf("undo did not restore the explicitly deferred assessment: %+v", state)
	}
	if err = s.db.QueryRow("SELECT card,version FROM schedules WHERE quiz_id=?", m.Quiz.ID).Scan(&afterCard, &afterVersion); err != nil {
		t.Fatal(err)
	}
	if beforeCard != afterCard || beforeVersion != afterVersion {
		t.Fatal("selection deferral or undo manufactured an FSRS transition")
	}
}

func TestPlanAndSuggestionReceiptsReopenWithConcreteArrays(t *testing.T) {
	s, now := newTestStore(t)
	ctx := context.Background()
	g := knowledgeFixture(t, s, 1, false, 1)
	settings := PlanSettings{ExpectedRevision: g.Revision, TimeBudgetSeconds: 300, NewAssessmentsPerDay: 5, Focus: "goal", Reason: "Choose the default pacing explicitly"}
	planned, err := s.PlanGoal(ctx, g.ID, settings, newID())
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(s.Path())
	if err != nil {
		t.Fatalf("valid plan made durable state incompatible: %v", err)
	}
	defer reopened.Close()
	reopened.clock = func() time.Time { return *now }
	restored, err := reopened.Goal(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Revision != planned.Revision || restored.Settings.Focus != planned.Settings.Focus || len(restored.Decisions) != len(planned.Decisions) {
		t.Fatalf("reopened plan lost acknowledged decisions: planned=%+v restored=%+v", planned, restored)
	}
}
