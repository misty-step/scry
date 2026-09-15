package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

func validatePlan(settings PlanSettings) error {
	if settings.ExpectedRevision < 1 {
		return fmt.Errorf("%w: the displayed goal revision is required", ErrInvalid)
	}
	if settings.TimeBudgetSeconds < 30 || settings.TimeBudgetSeconds > 3600 {
		return fmt.Errorf("%w: available time must be between 30 and 3600 seconds", ErrInvalid)
	}
	if settings.NewAssessmentsPerDay < 0 || settings.NewAssessmentsPerDay > 100 {
		return fmt.Errorf("%w: new assessments must be between 0 and 100 per day", ErrInvalid)
	}
	if settings.Focus != "goal" && settings.Focus != "foundation" && settings.Focus != "practice" {
		return fmt.Errorf("%w: focus must be goal, foundation, or practice", ErrInvalid)
	}
	return validText("planning reason", settings.Reason, 4096, true)
}

// Planning operations retain an exact response receipt. A retry after later
// choices returns its own acknowledgement, not the newer mutable goal view.
func (s *Store) planningOperation(ctx context.Context, operationID, kind string, payload any, change func(*sql.Tx, int64) (Goal, error)) (Goal, error) {
	if err := validOperation(operationID); err != nil {
		return Goal{}, err
	}
	hash := payloadHash(payload)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Goal{}, err
	}
	defer tx.Rollback()
	_, receipt, found, err := existingOperation(ctx, tx, operationID, kind, hash)
	if err != nil {
		return Goal{}, err
	}
	if found {
		var result Goal
		if err = json.Unmarshal([]byte(receipt), &result); err != nil {
			return Goal{}, err
		}
		for i := range result.Jobs {
			hideJobContext(&result.Jobs[i])
		}
		return result, tx.Commit()
	}
	now := s.now()
	result, err := change(tx, now)
	if err != nil {
		return Goal{}, err
	}
	if err = refreshPendingSuggestions(ctx, tx, result, now); err != nil {
		return Goal{}, err
	}
	if err = rebindUnclaimedKnowledgeJobs(ctx, tx, result, now); err != nil {
		return Goal{}, err
	}
	if err = recordKnowledgeEstimates(ctx, tx, now); err != nil {
		return Goal{}, err
	}
	result, err = goal(ctx, tx, result.ID)
	if err != nil {
		return Goal{}, err
	}
	if err = saveOperation(ctx, tx, operationID, kind, hash, result.ID, result, now); err != nil {
		return Goal{}, err
	}
	if err = tx.Commit(); err != nil {
		return Goal{}, err
	}
	return result, nil
}

func (s *Store) PlanGoal(ctx context.Context, id string, settings PlanSettings, operationID string) (Goal, error) {
	if err := validText("goal ID", id, 200, true); err != nil {
		return Goal{}, err
	}
	if err := validatePlan(settings); err != nil {
		return Goal{}, err
	}
	payload := struct {
		GoalID   string
		Settings PlanSettings
	}{id, settings}
	return s.planningOperation(ctx, operationID, "plan_goal", payload, func(tx *sql.Tx, now int64) (Goal, error) {
		g, err := goal(ctx, tx, id)
		if err != nil {
			return Goal{}, err
		}
		if g.Archived || g.Revision != settings.ExpectedRevision {
			return Goal{}, fmt.Errorf("%w: goal or plan changed; reload before choosing a plan", ErrConflict)
		}
		before := g.Settings
		before.ExpectedRevision = 0
		g.Settings = settings
		g.Settings.ExpectedRevision = 0
		if err = advanceGoalPlan(ctx, tx, &g, now); err != nil {
			return Goal{}, err
		}
		decision := PlanDecision{ID: newID(), GoalID: id, GoalRevision: g.Revision, Kind: "plan", Reason: settings.Reason, Policy: learning.SelectionPolicy, Before: before, After: g.Settings, CreatedAt: now}
		if err = savePlanDecision(ctx, tx, decision); err != nil {
			return Goal{}, err
		}
		return goal(ctx, tx, id)
	})
}

func (s *Store) ChooseSuggestion(ctx context.Context, id, action, operationID string) (Goal, error) {
	if err := validText("suggestion ID", id, 200, true); err != nil {
		return Goal{}, err
	}
	if action != "accept" && action != "decline" {
		return Goal{}, fmt.Errorf("%w: suggestion action must be accept or decline", ErrInvalid)
	}
	return s.planningOperation(ctx, operationID, "choose_suggestion", struct{ ID, Action string }{id, action}, func(tx *sql.Tx, now int64) (Goal, error) {
		var goalID, status, reason, kind string
		var revision int
		if err := tx.QueryRowContext(ctx, "SELECT goal_id,goal_revision,status,reason,kind FROM suggestions WHERE id=?", id).Scan(&goalID, &revision, &status, &reason, &kind); err != nil {
			return Goal{}, notFound(err, "suggestion")
		}
		g, err := goal(ctx, tx, goalID)
		if err != nil {
			return Goal{}, err
		}
		if g.Archived || revision != g.Revision || status != "pending" {
			return Goal{}, fmt.Errorf("%w: this suggestion is no longer an undecided choice for the displayed goal revision", ErrConflict)
		}
		proposal, err := suggestion(ctx, tx, id)
		if err != nil {
			return Goal{}, err
		}
		if compatible, why, err := suggestionFits(ctx, tx, proposal, g); err != nil {
			return Goal{}, err
		} else if action == "accept" && !compatible {
			return Goal{}, fmt.Errorf("%w: suggestion needs replanning: %s", ErrConflict, why)
		}
		chosenStatus := "accepted"
		if action == "decline" {
			chosenStatus = "declined"
		}
		changed, err := tx.ExecContext(ctx, "UPDATE suggestions SET status=? WHERE id=? AND status='pending' AND goal_revision=?", chosenStatus, id, revision)
		if err != nil {
			return Goal{}, err
		}
		count, err := changed.RowsAffected()
		if err != nil {
			return Goal{}, err
		}
		if count != 1 {
			return Goal{}, fmt.Errorf("%w: suggestion changed", ErrConflict)
		}
		if err = advanceGoalPlan(ctx, tx, &g, now); err != nil {
			return Goal{}, err
		}
		decision := PlanDecision{ID: newID(), GoalID: g.ID, GoalRevision: g.Revision, Kind: "suggestion_" + action, Reason: "Explicitly " + chosenStatus + " " + kind + " suggestion within the existing goal: " + reason, Policy: learning.SelectionPolicy, SuggestionID: id, Before: g.Settings, After: g.Settings, UnitVersions: proposal.UnitVersions, CreatedAt: now}
		if err = savePlanDecision(ctx, tx, decision); err != nil {
			return Goal{}, err
		}
		if action == "accept" {
			if err = enqueueSuggestionExpansion(ctx, tx, id, g, now); err != nil {
				return Goal{}, err
			}
		}
		return goal(ctx, tx, g.ID)
	})
}

func (s *Store) UndoPlan(ctx context.Context, id, operationID string) (Goal, error) {
	if err := validText("decision ID", id, 200, true); err != nil {
		return Goal{}, err
	}
	return s.planningOperation(ctx, operationID, "undo_plan", id, func(tx *sql.Tx, now int64) (Goal, error) {
		var goalID string
		if err := tx.QueryRowContext(ctx, "SELECT goal_id FROM plan_decisions WHERE id=?", id).Scan(&goalID); err != nil {
			return Goal{}, notFound(err, "planning decision")
		}
		g, err := goal(ctx, tx, goalID)
		if err != nil {
			return Goal{}, err
		}
		var decision *PlanDecision
		for i := range g.Decisions {
			if g.Decisions[i].ID == id {
				decision = &g.Decisions[i]
				break
			}
		}
		if decision == nil || g.Archived || decision.UndoneAt != 0 || decision.Kind == "undo" {
			return Goal{}, fmt.Errorf("%w: this planning decision cannot be undone", ErrConflict)
		}
		switch decision.Kind {
		case "plan", "suggestion_accept", "suggestion_decline", "defer", "pace":
		default:
			return Goal{}, fmt.Errorf("%w: this row explains a change rather than owning an undoable planning effect", ErrConflict)
		}
		if decision.Kind != "defer" && decision.GoalRevision != g.Revision {
			return Goal{}, fmt.Errorf("%w: a newer plan is authoritative; this undo would overwrite it", ErrConflict)
		}
		before := g.Settings
		if decision.Kind == "plan" {
			g.Settings = decision.Before
		}
		if err = advanceGoalPlan(ctx, tx, &g, now); err != nil {
			return Goal{}, err
		}
		changed, err := tx.ExecContext(ctx, "UPDATE plan_decisions SET undone_at=? WHERE id=? AND undone_at=0", now, id)
		if err != nil {
			return Goal{}, err
		}
		count, err := changed.RowsAffected()
		if err != nil {
			return Goal{}, err
		}
		if count != 1 {
			return Goal{}, fmt.Errorf("%w: decision already changed", ErrConflict)
		}
		undo := PlanDecision{ID: newID(), GoalID: g.ID, GoalRevision: g.Revision, Kind: "undo", Reason: "Explicitly undid future effects of " + decision.Kind + ": " + decision.Reason + ". Actual interactions and direct FSRS history are unchanged.", Policy: learning.SelectionPolicy, MaterialID: decision.MaterialID, MaterialVersion: decision.MaterialVersion, SuggestionID: decision.SuggestionID, Before: before, After: g.Settings, EvidenceIDs: decision.EvidenceIDs, UnitVersions: decision.UnitVersions, CreatedAt: now, UndoOf: id}
		if err = savePlanDecision(ctx, tx, undo); err != nil {
			return Goal{}, err
		}
		if decision.Kind == "suggestion_accept" || decision.Kind == "suggestion_decline" {
			proposal, err := suggestion(ctx, tx, decision.SuggestionID)
			if err != nil {
				return Goal{}, err
			}
			if err = refreshSuggestion(ctx, tx, proposal, g, now, false); err != nil {
				return Goal{}, err
			}
		}
		return goal(ctx, tx, g.ID)
	})
}

func advanceGoalPlan(ctx context.Context, tx *sql.Tx, g *Goal, now int64) error {
	previous := g.Revision
	g.Settings.ExpectedRevision = 0
	settings, err := marshal(g.Settings)
	if err != nil {
		return err
	}
	changed, err := tx.ExecContext(ctx, "UPDATE goals SET revision=revision+1,settings_json=?,updated_at=? WHERE id=? AND revision=? AND archived=0", settings, now, g.ID, previous)
	if err != nil {
		return err
	}
	count, err := changed.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("%w: goal revision changed", ErrConflict)
	}
	g.Revision, g.UpdatedAt = previous+1, now
	// Persist only the actual goal definition, not a duplicated mutable library
	// or an estimate computed at whichever moment this command happened to run.
	snapshot, err := marshal(Goal{ID: g.ID, SourceID: g.SourceID, Title: g.Title, Revision: g.Revision, SourceRevision: g.SourceRevision, Archived: g.Archived, Coverage: g.Coverage, Settings: g.Settings, CreatedAt: g.CreatedAt, UpdatedAt: now})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO goal_versions(goal_id,revision,snapshot,created_at) VALUES(?,?,?,?)", g.ID, g.Revision, snapshot, now)
	return err
}

func savePlanDecision(ctx context.Context, tx *sql.Tx, decision PlanDecision) error {
	if decision.EvidenceIDs == nil {
		decision.EvidenceIDs = []string{}
	}
	if decision.UnitVersions == nil {
		decision.UnitVersions = []learning.UnitVersion{}
	}
	before, err := marshal(decision.Before)
	if err != nil {
		return err
	}
	after, err := marshal(decision.After)
	if err != nil {
		return err
	}
	evidence, err := marshal(decision.EvidenceIDs)
	if err != nil {
		return err
	}
	units, err := marshal(decision.UnitVersions)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO plan_decisions(id,goal_id,goal_revision,kind,reason,policy,material_id,material_version,suggestion_id,before_json,after_json,evidence_ids_json,unit_versions_json,reconsider_at,created_at,undone_at,undo_of)
	 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, decision.ID, decision.GoalID, decision.GoalRevision, decision.Kind, decision.Reason, decision.Policy, decision.MaterialID, decision.MaterialVersion, decision.SuggestionID, before, after, evidence, units, decision.ReconsiderAt, decision.CreatedAt, decision.UndoneAt, decision.UndoOf)
	return err
}

type candidateGoal struct {
	ID       string
	Revision int
	Settings PlanSettings
	Pacing   learning.Pacing
}

type plannedMaterial struct {
	GoalID          string
	Material        Material
	ScheduleVersion int
	Candidate       learning.Candidate
	Reason          string
}

// knowledgeCandidate may record visible deferral decisions only when establishing
// an actual occurrence (exclude is empty). Preview evaluation never writes one.
// No candidate computation introduces material, records observation, or edits FSRS.
func knowledgeCandidate(ctx context.Context, tx *sql.Tx, now int64, exclude string) (*Material, int, error) {
	asOf := time.UnixMilli(now).UTC()
	day := time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
	rows, err := tx.QueryContext(ctx, `SELECT g.id,g.revision,g.settings_json FROM goals g JOIN sources s ON s.id=g.source_id WHERE g.archived=0 AND s.archived=0 ORDER BY g.created_at,g.id`)
	if err != nil {
		return nil, 0, err
	}
	goals := make(map[string]*candidateGoal)
	var goalIDs []string
	for rows.Next() {
		var g candidateGoal
		var settings string
		if err = rows.Scan(&g.ID, &g.Revision, &settings); err != nil {
			rows.Close()
			return nil, 0, err
		}
		if err = json.Unmarshal([]byte(settings), &g.Settings); err != nil {
			rows.Close()
			return nil, 0, err
		}
		g.Pacing = learning.Pacing{AsOf: asOf, AvailableSeconds: g.Settings.TimeBudgetSeconds, NewAssessments: g.Settings.NewAssessmentsPerDay}
		goals[g.ID] = &g
		goalIDs = append(goalIDs, g.ID)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	var ownerNew int
	if err = tx.QueryRowContext(ctx, `SELECT count(DISTINCT i.material_id) FROM interactions i
	 WHERE i.kind IN('review','practice') AND i.at>=? AND i.at<=?
	 AND NOT EXISTS(SELECT 1 FROM interactions prior WHERE prior.material_id=i.material_id AND prior.kind IN('review','practice') AND prior.at<?)`, day, now, day).Scan(&ownerNew); err != nil {
		return nil, 0, err
	}
	var reserved bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM presentations p WHERE p.kind='quiz' AND p.graded=0
	 AND p.id IN(SELECT current_id FROM review_session WHERE singleton=1 UNION SELECT target_presentation_id FROM bridges WHERE status IN('pending','active','ready_return'))
	 AND NOT EXISTS(SELECT 1 FROM interactions i WHERE i.material_id=p.material_id AND i.kind IN('review','practice')))`).Scan(&reserved); err != nil {
		return nil, 0, err
	}
	if reserved {
		ownerNew++
	}
	for _, id := range goalIDs {
		g := goals[id]
		var since int64
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(created_at),?) FROM plan_decisions WHERE goal_id=? AND kind='plan' AND undone_at=0 AND created_at>=?`, day, id, day).Scan(&since); err != nil {
			return nil, 0, err
		}
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(sum(CAST(json_extract(v.content,'$.estimated_seconds') AS INTEGER)),0)
		 FROM presentations p JOIN material_versions v ON v.material_id=p.material_id AND v.version=p.material_version
		 WHERE p.created_at>=? AND p.goal_id=?`, since, id).Scan(&g.Pacing.SpentSeconds); err != nil {
			return nil, 0, err
		}
		g.Pacing.NewAssessmentsToday = ownerNew
	}
	// Read selection metadata only. Material bodies and quiz answers are loaded
	// once, after selection, and never for a preview.
	rows, err = tx.QueryContext(ctx, `SELECT DISTINCT gm.goal_id,m.id,m.source_id,m.version,m.kind,m.created_at,m.first_presented_at,
	 COALESCE(json_extract(mv.content,'$.title'),json_extract(qv.content,'$.prompt'),''),
	 COALESCE(json_extract(mv.content,'$.level'),''),CAST(json_extract(mv.content,'$.estimated_seconds') AS INTEGER),
	 COALESCE(json_extract(mv.provenance_json,'$.source_revision'),0),mv.quiz_id,mv.quiz_version,COALESCE(json_extract(qv.content,'$.kind'),''),
	 COALESCE(sc.due_at,0),COALESCE(sc.version,0),
	 EXISTS(SELECT 1 FROM interactions i WHERE i.material_id=m.id AND i.kind IN('review','practice') AND i.at<=?),
	 COALESCE((SELECT max(i.at) FROM interactions i WHERE i.material_id=m.id AND i.material_version=m.version AND i.kind='continue' AND i.at<=?),0)
	 FROM goal_materials gm JOIN goals g ON g.id=gm.goal_id JOIN materials m ON m.id=gm.material_id AND m.version=gm.material_version
	 JOIN sources src ON src.id=m.source_id JOIN material_versions mv ON mv.material_id=m.id AND mv.version=m.version
	 LEFT JOIN quizzes q ON q.id=mv.quiz_id LEFT JOIN quiz_versions qv ON qv.quiz_id=mv.quiz_id AND qv.version=mv.quiz_version LEFT JOIN schedules sc ON sc.quiz_id=mv.quiz_id
	 WHERE g.archived=0 AND m.archived=0 AND src.archived=0 AND (q.id IS NULL OR q.archived=0)
	 AND (m.kind<>'quiz' OR sc.due_at<=?) AND m.id<>? AND COALESCE(q.id,'')<>?
	 AND NOT EXISTS(SELECT 1 FROM bridges b WHERE b.target_material_id=m.id AND b.status IN('pending','active','ready_return'))
	 ORDER BY gm.goal_id,m.id`, now, now, now, exclude, exclude)
	if err != nil {
		return nil, 0, err
	}
	var metadata []plannedMaterial
	continued := make(map[string]int64)
	for rows.Next() {
		var item plannedMaterial
		var quizID sql.NullString
		var quizVersion sql.NullInt64
		var mode string
		var lastContinue int64
		m := &item.Material
		if err = rows.Scan(&item.GoalID, &m.ID, &m.SourceID, &m.Version, &m.Kind, &m.CreatedAt, &m.FirstPresentedAt,
			&m.Title, &m.Level, &m.EstimatedSeconds, &m.Provenance.SourceRevision, &quizID, &quizVersion, &mode, &m.DueAt, &item.ScheduleVersion, &item.Candidate.Assessed, &lastContinue); err != nil {
			rows.Close()
			return nil, 0, err
		}
		if quizID.Valid {
			m.Quiz = &Quiz{ID: quizID.String, SourceID: m.SourceID, Version: int(quizVersion.Int64), Kind: mode, Prompt: m.Title, DueAt: m.DueAt}
		}
		continued[m.ID] = lastContinue
		metadata = append(metadata, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, 0, err
	}
	rows.Close()
	var items []plannedMaterial
	byGoal := make(map[string][]learning.Candidate)
	unitStates := make(map[learning.UnitVersion]unitSelectionState)
	linkCache := make(map[string][]CoverageLink)
	evidenceCache := make(map[string][]learning.Evidence)
	for _, item := range metadata {
		g := goals[item.GoalID]
		if g == nil {
			continue
		}
		m := item.Material
		links, cached := linkCache[m.ID]
		if !cached {
			links, err = selectionLinks(ctx, tx, m.ID, m.Version)
			if err != nil {
				return nil, 0, err
			}
			linkCache[m.ID] = links
		}
		m.Links, m.Unmapped = links, len(links) == 0
		candidate := learning.Candidate{ID: m.ID, Version: m.Version, Kind: m.Kind, Level: m.Level, EstimatedSeconds: m.EstimatedSeconds, Selected: true, AvailableAt: time.UnixMilli(m.CreatedAt), Assessed: item.Candidate.Assessed, Continued: continued[m.ID] != 0}
		if m.FirstPresentedAt != 0 {
			candidate.FirstPresentedAt = time.UnixMilli(m.FirstPresentedAt)
		}
		if m.Quiz != nil {
			candidate.Mode, candidate.DueAt = m.Quiz.Kind, time.UnixMilli(m.DueAt)
			completed, err := completedExposedPractice(ctx, tx, m, now)
			if err != nil {
				return nil, 0, err
			}
			if completed {
				continue
			}
		}
		for _, link := range links {
			if link.Role != "teaches" && link.Role != "assesses" {
				continue
			}
			unit := learning.UnitVersion{ID: link.UnitID, Version: link.UnitVersion}
			state, cached := unitStates[unit]
			if !cached {
				state, err = selectionUnitState(ctx, tx, unit, now)
				if err != nil {
					return nil, 0, err
				}
				unitStates[unit] = state
			}
			if candidate.Kind != "quiz" && state.Kind == "foundation" {
				candidate.Level = "foundation"
			}
			if link.Role == "assesses" {
				candidate.Units = append(candidate.Units, unit)
				candidate.ReadyProbe = candidate.ReadyProbe || !state.TaughtAt.IsZero() && asOf.Sub(state.TaughtAt) < learning.EvidenceFreshness
			}
			if state.State == "gap" && state.ContextAt.UnixMilli() > continued[m.ID] {
				candidate.Gap = true
			}
		}
		if g.Settings.Focus == "foundation" && candidate.Level == "foundation" || g.Settings.Focus == "practice" && m.Kind == "quiz" {
			candidate.Priority = 1
		}
		selected, reason, err := selectedSuggestionMaterial(ctx, tx, g.ID, m.ID, m.Version)
		if err != nil {
			return nil, 0, err
		}
		candidate.Selected = selected || m.Kind == "quiz" && candidate.Assessed
		if reason != "" && selected {
			candidate.Priority++
		}
		if err = applyPacingDecision(ctx, tx, *g, &candidate, now, exclude == ""); err != nil {
			return nil, 0, err
		}
		if m.Kind == "quiz" && m.Level == "foundation" && g.Settings.Focus != "practice" && learning.SelectMaterial([]learning.Candidate{candidate}, g.Pacing).MaterialID != "" {
			scope, err := marshal(candidate.Units)
			if err != nil {
				return nil, 0, err
			}
			evidence, cached := evidenceCache[scope]
			if !cached {
				evidence, err = loadKnowledgeEvidence(ctx, tx, now, scope)
				if err != nil {
					return nil, 0, err
				}
				evidenceCache[scope] = evidence
			}
			deferral := learning.DeferPractice(learning.DeferralInput{MaterialID: m.ID, MaterialVersion: m.Version, Mode: candidate.Mode, Units: candidate.Units, AsOf: asOf, Unmapped: m.Unmapped, Evidence: evidence})
			if deferral.Deferred {
				candidate.Deferred, err = applyPracticeDeferral(ctx, tx, *g, m, deferral, now, exclude == "")
				if err != nil {
					return nil, 0, err
				}
			}
		}
		item.Material, item.Candidate, item.Reason = m, candidate, reason
		items = append(items, item)
		byGoal[g.ID] = append(byGoal[g.ID], candidate)
	}
	var finalists []learning.Candidate
	selectedItems := make(map[string]plannedMaterial)
	for _, id := range goalIDs {
		selection := learning.SelectMaterial(byGoal[id], goals[id].Pacing)
		if selection.MaterialID == "" {
			continue
		}
		for _, item := range items {
			if item.GoalID == id && item.Material.ID == selection.MaterialID {
				if _, exists := selectedItems[item.Material.ID]; !exists {
					finalists = append(finalists, item.Candidate)
					selectedItems[item.Material.ID] = item
				}
				break
			}
		}
	}
	// Per-goal pacing was already applied. This pass orders only eligible goal
	// finalists, so one exhausted goal cannot suppress ordinary other review.
	selection := learning.SelectMaterial(finalists, learning.Pacing{AsOf: asOf, AvailableSeconds: 3600, NewAssessments: 100})
	if selection.MaterialID == "" {
		return nil, 0, nil
	}
	chosen := selectedItems[selection.MaterialID]
	if exclude != "" {
		// Reference titles and learner-authored reasons can themselves disclose
		// answers. A preview receives only generic kind/identity metadata.
		if chosen.Material.Kind != "quiz" {
			chosen.Material.Title = ""
		}
		chosen.Material.SelectionPolicy = selection.Policy
		chosen.Material.GoalID, chosen.Material.GoalRevision = chosen.GoalID, goals[chosen.GoalID].Revision
		return &chosen.Material, chosen.ScheduleVersion, nil
	}
	if exclude == "" {
		chosen.Material, err = material(ctx, tx, chosen.Material.ID)
		if err != nil {
			return nil, 0, err
		}
	}
	chosen.Material.GoalID, chosen.Material.GoalRevision = chosen.GoalID, goals[chosen.GoalID].Revision
	chosen.Material.SelectionPolicy = selection.Policy
	chosen.Material.PlanningReason = selection.Reason + " Per-goal plan epoch: " + goals[chosen.GoalID].Settings.Reason
	if chosen.Reason != "" {
		chosen.Material.PlanningReason += " Accepted option: " + chosen.Reason
	}
	if chosen.Candidate.PacingOverride {
		chosen.Material.PlanningReason += " Pacing override " + chosen.Candidate.PacingOverrideID + " permits this one occurrence despite the limit."
	}
	return &chosen.Material, chosen.ScheduleVersion, nil
}

func selectedSuggestionMaterial(ctx context.Context, tx *sql.Tx, goalID, materialID string, version int) (bool, string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT s.id,s.reason,
	 COALESCE((SELECT d.kind FROM plan_decisions d WHERE d.suggestion_id=s.id AND d.kind IN('suggestion_accept','suggestion_decline') AND d.undone_at=0 ORDER BY d.created_at DESC,d.id DESC LIMIT 1),'')
	 FROM suggestions s WHERE s.goal_id=? AND s.status IN('pending','accepted','declined')
	 AND EXISTS(SELECT 1 FROM json_each(s.material_versions_json) j WHERE json_extract(j.value,'$.id')=? AND json_extract(j.value,'$.version')=?) ORDER BY s.created_at,s.id`, goalID, materialID, version)
	if err != nil {
		return false, "", err
	}
	defer rows.Close()
	restricted, accepted := false, false
	var acceptedReason string
	for rows.Next() {
		var id, reason, decision string
		if err = rows.Scan(&id, &reason, &decision); err != nil {
			return false, "", err
		}
		restricted = true
		if decision == "suggestion_accept" {
			accepted, acceptedReason = true, reason
		}
	}
	return !restricted || accepted, acceptedReason, rows.Err()
}

func applyPracticeDeferral(ctx context.Context, tx *sql.Tx, g candidateGoal, m Material, deferral learning.Deferral, now int64, persist bool) (bool, error) {
	ids, err := marshal(deferral.EvidenceIDs)
	if err != nil {
		return false, err
	}
	var undone, reconsider int64
	err = tx.QueryRowContext(ctx, `SELECT undone_at,reconsider_at FROM plan_decisions WHERE goal_id=? AND material_id=? AND material_version=? AND kind='defer' AND policy=? AND evidence_ids_json=? ORDER BY created_at DESC,id DESC LIMIT 1`, g.ID, m.ID, m.Version, learning.SelectionPolicy, ids).Scan(&undone, &reconsider)
	if err == nil {
		return undone == 0 && reconsider > now, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if !persist {
		return true, nil
	}
	decision := PlanDecision{ID: newID(), GoalID: g.ID, GoalRevision: g.Revision, Kind: "defer", Reason: deferral.Reason, Policy: deferral.Policy, MaterialID: m.ID, MaterialVersion: m.Version, Before: g.Settings, After: g.Settings, EvidenceIDs: deferral.EvidenceIDs, UnitVersions: deferral.Units, ReconsiderAt: deferral.ReconsiderAt.UnixMilli(), CreatedAt: now}
	if err = savePlanDecision(ctx, tx, decision); err != nil {
		return false, err
	}
	return true, nil
}

func completedExposedPractice(ctx context.Context, tx *sql.Tx, m Material, now int64) (bool, error) {
	exposureAt, err := latestMaterialExposure(ctx, tx, m, now)
	if err != nil {
		return false, err
	}
	if exposureAt == 0 {
		return false, nil
	}
	// Feedback keeps a task warm, but only new instruction/explicit inspection
	// starts another automatic practice opportunity for already practiced work.
	exposureAt, err = materialExposure(ctx, tx, m, now, false)
	if err != nil {
		return false, err
	}
	var completed bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM interactions WHERE material_id=? AND material_version=? AND kind='practice' AND at>=?)", m.ID, m.Version, exposureAt).Scan(&completed)
	return completed, err
}

func applyPacingDecision(ctx context.Context, tx *sql.Tx, g candidateGoal, candidate *learning.Candidate, now int64, persist bool) error {
	if !candidate.Selected || candidate.AvailableAt.After(g.Pacing.AsOf) || candidate.EstimatedSeconds < 1 || candidate.Kind == "quiz" && candidate.DueAt.After(g.Pacing.AsOf) || candidate.Kind != "quiz" && candidate.Continued && !candidate.Gap {
		return nil
	}
	var overrideID string
	err := tx.QueryRowContext(ctx, `SELECT d.id FROM plan_decisions d JOIN plan_decisions u ON u.undo_of=d.id
	 WHERE d.goal_id=? AND d.kind='pace' AND d.material_id=? AND d.material_version=? AND d.undone_at>0 AND u.goal_revision=? AND u.undone_at=0
	 AND NOT EXISTS(SELECT 1 FROM presentations p WHERE p.material_id=d.material_id AND p.material_version=d.material_version
	   AND instr(COALESCE(json_extract(p.material_snapshot,'$.planning_reason'),''),'Pacing override '||d.id)>0)
	 ORDER BY u.created_at DESC,u.id DESC LIMIT 1`, g.ID, candidate.ID, candidate.Version, g.Revision).Scan(&overrideID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	candidate.PacingOverride, candidate.PacingOverrideID = overrideID != "", overrideID
	if learning.SelectMaterial([]learning.Candidate{*candidate}, g.Pacing).MaterialID != "" || !persist {
		return nil
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM plan_decisions WHERE goal_id=? AND goal_revision=? AND material_id=? AND material_version=? AND kind='pace' AND reconsider_at>?)`, g.ID, g.Revision, candidate.ID, candidate.Version, now).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	reason := fmt.Sprintf("Explicit pacing admits this material later: %d of %d estimated seconds used and %d of %d owner-wide new assessments used or reserved today. Availability and the direct FSRS schedule are unchanged.", g.Pacing.SpentSeconds, g.Pacing.AvailableSeconds, g.Pacing.NewAssessmentsToday, g.Pacing.NewAssessments)
	tomorrow := time.Date(g.Pacing.AsOf.Year(), g.Pacing.AsOf.Month(), g.Pacing.AsOf.Day()+1, 0, 0, 0, 0, time.UTC)
	return savePlanDecision(ctx, tx, PlanDecision{ID: newID(), GoalID: g.ID, GoalRevision: g.Revision, Kind: "pace", Reason: reason, Policy: learning.SelectionPolicy, MaterialID: candidate.ID, MaterialVersion: candidate.Version, Before: g.Settings, After: g.Settings, UnitVersions: candidate.Units, ReconsiderAt: tomorrow.UnixMilli(), CreatedAt: now})
}

func suggestionFits(ctx context.Context, tx *sql.Tx, proposal Suggestion, g Goal) (bool, string, error) {
	if len(proposal.UnitVersions) == 0 || !completeProposalPins(proposal.UnitIDs, proposal.UnitVersions) || !completeProposalPins(proposal.MaterialIDs, proposal.MaterialVersions) {
		return false, "The proposal lacks complete exact-version scope.", nil
	}
	for _, pin := range proposal.UnitVersions {
		var current int
		var archived, included bool
		if err := tx.QueryRowContext(ctx, `SELECT version,archived,EXISTS(SELECT 1 FROM goal_units WHERE goal_id=? AND unit_id=? AND unit_version=?)
		 FROM knowledge_units WHERE id=?`, g.ID, pin.ID, pin.Version, pin.ID).Scan(&current, &archived, &included); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, "A proposed unit no longer exists.", nil
			}
			return false, "", err
		}
		if archived || current != pin.Version || !included {
			return false, "A proposed unit changed or is outside this goal's exact scope.", nil
		}
	}
	for _, pin := range proposal.MaterialVersions {
		m, err := material(ctx, tx, pin.ID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return false, "A proposed material no longer exists.", nil
			}
			return false, "", err
		}
		if m.Archived || m.Version != pin.Version || m.EstimatedSeconds > g.Settings.TimeBudgetSeconds {
			return false, "A proposed material changed, was archived, or exceeds the chosen per-goal time budget.", nil
		}
		if m.Kind == "quiz" && g.Settings.NewAssessmentsPerDay == 0 {
			var attempted bool
			if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM interactions WHERE material_id=? AND kind IN('review','practice'))", m.ID).Scan(&attempted); err != nil {
				return false, "", err
			}
			if !attempted {
				return false, "The chosen plan explicitly pauses new assessments.", nil
			}
		}
	}
	return true, "", nil
}

// Only an explicit learner planning mutation refreshes undecided proposals.
// Fresh IDs retain immutable lineage; a stale old tab never gains new authority.
func refreshPendingSuggestions(ctx context.Context, tx *sql.Tx, g Goal, now int64) error {
	ids, err := rowIDs(ctx, tx, "SELECT id FROM suggestions WHERE goal_id=? AND status='pending' AND goal_revision<? ORDER BY created_at,id", g.ID, g.Revision)
	if err != nil {
		return err
	}
	for _, id := range ids {
		proposal, err := suggestion(ctx, tx, id)
		if err != nil {
			return err
		}
		if err = refreshSuggestion(ctx, tx, proposal, g, now, true); err != nil {
			return err
		}
	}
	return nil
}

func refreshSuggestion(ctx context.Context, tx *sql.Tx, proposal Suggestion, g Goal, now int64, retire bool) error {
	fits, why, err := suggestionFits(ctx, tx, proposal, g)
	if err != nil {
		return err
	}
	status, decisionKind := "superseded", "suggestion_refresh"
	reason := fmt.Sprintf("Explicit plan choice refreshed proposal %s for goal revision %d under a new ID; old IDs and receipts retain their original authority.", proposal.ID, g.Revision)
	if !fits {
		status, decisionKind = "obsolete", "suggestion_obsolete"
		reason = "Proposal " + proposal.ID + " needs replanning after the explicit choice: " + why
	}
	if retire {
		if _, err = tx.ExecContext(ctx, "UPDATE suggestions SET status=? WHERE id=? AND status='pending'", status, proposal.ID); err != nil {
			return err
		}
	}
	if fits {
		if proposal.UnitIDs == nil {
			proposal.UnitIDs = []string{}
		}
		if proposal.MaterialIDs == nil {
			proposal.MaterialIDs = []string{}
		}
		if proposal.UnitVersions == nil {
			proposal.UnitVersions = []learning.UnitVersion{}
		}
		if proposal.MaterialVersions == nil {
			proposal.MaterialVersions = []learning.UnitVersion{}
		}
		units, err := marshal(proposal.UnitIDs)
		if err != nil {
			return err
		}
		materials, err := marshal(proposal.MaterialIDs)
		if err != nil {
			return err
		}
		unitVersions, err := marshal(proposal.UnitVersions)
		if err != nil {
			return err
		}
		materialVersions, err := marshal(proposal.MaterialVersions)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO suggestions(id,goal_id,goal_revision,kind,title,reason,unit_ids_json,material_ids_json,unit_versions_json,material_versions_json,status,origin_job_id,created_at,supersedes_id)
		 VALUES(?,?,?,?,?,?,?,?,?,?,'pending',?,?,?)`, newID(), g.ID, g.Revision, proposal.Kind, proposal.Title, proposal.Reason, units, materials, unitVersions, materialVersions, proposal.OriginJobID, now, proposal.ID); err != nil {
			return err
		}
	}
	return savePlanDecision(ctx, tx, PlanDecision{ID: newID(), GoalID: g.ID, GoalRevision: g.Revision, Kind: decisionKind, Reason: reason, Policy: learning.SelectionPolicy, SuggestionID: proposal.ID, Before: g.Settings, After: g.Settings, UnitVersions: proposal.UnitVersions, CreatedAt: now})
}

type unitSelectionState struct {
	State     string
	Kind      string
	ContextAt time.Time
	TaughtAt  time.Time
}

func selectionUnitState(ctx context.Context, tx *sql.Tx, unit learning.UnitVersion, now int64) (unitSelectionState, error) {
	state := unitSelectionState{State: "unknown"}
	var contextAt, taughtAt sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT uv.kind,COALESCE(json_extract(e.estimate_json,'$.state'),'unknown'),json_extract(e.estimate_json,'$.context_at'),
	 (SELECT max(json_extract(value,'$.at')) FROM json_each(e.estimate_json,'$.exposure')
	  WHERE json_extract(value,'$.kind')='continue' AND json_extract(value,'$.disputed')=0 AND json_extract(value,'$.ambiguous')=0)
	 FROM unit_versions uv LEFT JOIN estimate_records e ON e.id=(
	  SELECT id FROM estimate_records WHERE unit_id=? AND unit_version=? AND mode='recall' AND as_of<=? AND policy=? ORDER BY as_of DESC,rowid DESC LIMIT 1)
	 WHERE uv.unit_id=? AND uv.version=?`,
		unit.ID, unit.Version, now, learning.InferencePolicy, unit.ID, unit.Version).Scan(&state.Kind, &state.State, &contextAt, &taughtAt)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if contextAt.Valid {
		state.ContextAt, err = time.Parse(time.RFC3339Nano, contextAt.String)
		if err != nil {
			return state, err
		}
	}
	if taughtAt.Valid {
		state.TaughtAt, err = time.Parse(time.RFC3339Nano, taughtAt.String)
		if err != nil {
			return state, err
		}
	}
	return state, nil
}

func selectionLinks(ctx context.Context, tx *sql.Tx, materialID string, version int) ([]CoverageLink, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id,unit_id,unit_version,role FROM material_links WHERE material_id=? AND material_version=? ORDER BY unit_id,unit_version,role,id", materialID, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := []CoverageLink{}
	for rows.Next() {
		link := CoverageLink{MaterialID: materialID, MaterialVersion: version}
		if err = rows.Scan(&link.ID, &link.UnitID, &link.UnitVersion, &link.Role); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func completeProposalPins(ids []string, pins []learning.UnitVersion) bool {
	if len(ids) != len(pins) {
		return false
	}
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" || wanted[id] {
			return false
		}
		wanted[id] = true
	}
	for _, pin := range pins {
		if !wanted[pin.ID] || pin.Version < 1 {
			return false
		}
		delete(wanted, pin.ID)
	}
	return len(wanted) == 0
}
