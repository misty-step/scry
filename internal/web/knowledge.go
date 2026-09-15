package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/misty-step/scry/internal/store"
)

func (s *server) goal(w http.ResponseWriter, r *http.Request) {
	if s.guardInspection(w, r, "goal", r.PathValue("id")) {
		return
	}
	goal, err := s.store.Goal(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	goal = publicGoal(goal)
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"goal": goal, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken()})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "goal", Title: "Your learning goal", Active: "library", Goal: goal})
}

// Pacing does not require looking up an answer. This read model deliberately
// omits all unit/material/suggestion text before serving the separate plan form.
func (s *server) planPage(w http.ResponseWriter, r *http.Request) {
	goal, err := s.store.Goal(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	goal, err = s.privatePlan(r, goal)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"goal": goal, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken()})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "plan", Title: "Set your pace", Active: "review", Goal: goal, Form: planForm(goal)})
}

func (s *server) privatePlan(r *http.Request, goal store.Goal) (store.Goal, error) {
	access, err := s.store.InspectionAccess(r.Context(), "goal", goal.ID)
	if err != nil {
		return store.Goal{}, err
	}
	safe := store.Goal{ID: goal.ID, SourceID: goal.SourceID, Title: goal.Title, Revision: goal.Revision, MetadataOnly: true, Archived: goal.Archived, Settings: goal.Settings}
	if access.RequiresAssistance || !access.Allowed {
		safe.Title = "Your current learning goal"
		safe.Settings.Reason = ""
	}
	return safe, nil
}

func planForm(goal store.Goal) url.Values {
	return url.Values{"version": {strconv.Itoa(goal.Revision)}, "time_budget_seconds": {strconv.Itoa(goal.Settings.TimeBudgetSeconds)}, "new_assessments_per_day": {strconv.Itoa(goal.Settings.NewAssessmentsPerDay)}, "focus": {goal.Settings.Focus}, "reason": {""}}
}

func (s *server) planGoal(w http.ResponseWriter, r *http.Request) {
	id, op := r.PathValue("id"), r.PostForm.Get("operation_id")
	goal, err := s.store.Goal(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, page{Operation: op})
		return
	}
	goal, err = s.privatePlan(r, goal)
	if err != nil {
		s.fail(w, r, err, page{Operation: op})
		return
	}
	p := page{View: "plan", Title: "Set your pace", Active: "review", Goal: goal, Form: r.PostForm, Operation: op}
	seconds, e1 := strconv.Atoi(r.PostForm.Get("time_budget_seconds"))
	newPerDay, e2 := strconv.Atoi(r.PostForm.Get("new_assessments_per_day"))
	revision, e3 := strconv.Atoi(r.PostForm.Get("version"))
	if e1 != nil || e2 != nil || e3 != nil || op == "" || len(op) > 128 {
		s.fail(w, r, store.ErrInvalid, p)
		return
	}
	settings := store.PlanSettings{TimeBudgetSeconds: seconds, NewAssessmentsPerDay: newPerDay, Focus: r.PostForm.Get("focus"), Reason: r.PostForm.Get("reason"), ExpectedRevision: revision}
	updated, err := s.store.PlanGoal(r.Context(), id, settings, op)
	if err != nil {
		s.fail(w, r, err, p)
		return
	}
	s.finish(w, r, "/goals/"+id+"/plan", map[string]any{"goal_id": updated.ID, "revision": updated.Revision})
}

func (s *server) chooseSuggestion(w http.ResponseWriter, r *http.Request) {
	op := r.PostForm.Get("operation_id")
	if op == "" || len(op) > 128 {
		s.fail(w, r, store.ErrInvalid, page{Operation: op})
		return
	}
	goal, err := s.store.ChooseSuggestion(r.Context(), r.PathValue("id"), r.PostForm.Get("action"), op)
	if err != nil {
		s.fail(w, r, err, page{Operation: op})
		return
	}
	s.finish(w, r, "/goals/"+goal.ID, map[string]any{"goal_id": goal.ID, "revision": goal.Revision})
}

func (s *server) undoPlan(w http.ResponseWriter, r *http.Request) {
	op := r.PostForm.Get("operation_id")
	if op == "" || len(op) > 128 {
		s.fail(w, r, store.ErrInvalid, page{Operation: op})
		return
	}
	goal, err := s.store.UndoPlan(r.Context(), r.PathValue("id"), op)
	if err != nil {
		s.fail(w, r, err, page{Operation: op})
		return
	}
	s.finish(w, r, "/goals/"+goal.ID, map[string]any{"goal_id": goal.ID, "revision": goal.Revision})
}

func unitQuery(r *http.Request) (string, time.Time, error) {
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "recall"
	}
	if mode != "choice" && mode != "recall" {
		return "", time.Time{}, fmt.Errorf("%w: choose recognition or cued recall", store.ErrInvalid)
	}
	var at time.Time
	if raw := r.URL.Query().Get("at"); raw != "" {
		var err error
		at, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("%w: use an ISO time with timezone, for example 2026-09-20T12:00:00Z", store.ErrInvalid)
		}
	}
	return mode, at, nil
}

func (s *server) unit(w http.ResponseWriter, r *http.Request) {
	if s.guardInspection(w, r, "unit", r.PathValue("id")) {
		return
	}
	mode, at, err := unitQuery(r)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	detail, err := s.store.Unit(r.Context(), r.PathValue("id"), mode, at)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"unit": detail, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken()})
		return
	}
	form := unitForm(detail)
	if selected := r.URL.Query().Get("relation"); selected != "" {
		found := false
		for _, relation := range detail.Relations {
			if relation.ID == selected {
				fillRelationForm(form, relation)
				found = true
				break
			}
		}
		if !found {
			s.fail(w, r, store.ErrNotFound, page{})
			return
		}
	}
	s.render(w, r, http.StatusOK, page{View: "unit", Title: "Knowledge and evidence", Active: "library", Unit: detail, Mode: mode, EstimateAt: r.URL.Query().Get("at"), Form: form})
}

func unitForm(detail store.KnowledgeUnitDetail) url.Values {
	return url.Values{"version": {strconv.Itoa(detail.Unit.Version)}, "statement": {detail.Unit.Statement}, "kind": {detail.Unit.Kind}, "from_id": {detail.Unit.ID}, "from_version": {strconv.Itoa(detail.Unit.Version)}, "relation_version": {"0"}, "relation_kind": {"prerequisite"}, "proposed": {"yes"}}
}

func fillRelationForm(form url.Values, relation store.KnowledgeRelation) {
	form.Set("relation_id", relation.ID)
	form.Set("relation_version", strconv.Itoa(relation.Version))
	form.Set("from_id", relation.FromID)
	form.Set("from_version", strconv.Itoa(relation.FromVersion))
	form.Set("to_id", relation.ToID)
	form.Set("to_version", strconv.Itoa(relation.ToVersion))
	form.Set("relation_kind", relation.Kind)
	form.Set("relation_evidence", relation.Provenance.Evidence)
	form.Set("proposed", "")
	if relation.Proposed {
		form.Set("proposed", "yes")
	}
	form.Set("archived", "")
	if relation.Archived {
		form.Set("archived", "yes")
	}
}

func (s *server) editUnit(w http.ResponseWriter, r *http.Request) {
	id, op := r.PathValue("id"), r.PostForm.Get("operation_id")
	detail, err := s.store.Unit(r.Context(), id, "recall", time.Time{})
	if err != nil {
		s.fail(w, r, err, page{Operation: op})
		return
	}
	p := page{View: "unit", Title: "Correct a knowledge definition", Active: "library", Unit: detail, Mode: "recall", Form: mergeForm(unitForm(detail), r.PostForm), Operation: op}
	version, err := strconv.Atoi(r.PostForm.Get("version"))
	if err != nil || op == "" || len(op) > 128 {
		s.fail(w, r, store.ErrInvalid, p)
		return
	}
	updated, err := s.store.EditUnit(r.Context(), id, version, r.PostForm.Get("statement"), r.PostForm.Get("kind"), r.PostForm.Get("reason"), op)
	if err != nil {
		s.fail(w, r, err, p)
		return
	}
	s.finish(w, r, "/units/"+id, map[string]any{"unit_id": updated.ID, "version": updated.Version})
}

func (s *server) material(w http.ResponseWriter, r *http.Request) {
	if s.guardInspection(w, r, "material", r.PathValue("id")) {
		return
	}
	material, err := s.store.Material(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"material": material, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken()})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "material", Title: "Saved learning material", Active: "library", Material: material, Form: materialForm(material)})
}

func materialForm(material store.Material) url.Values {
	form := url.Values{"version": {strconv.Itoa(material.Version)}, "title": {material.Title}, "body": {material.Body}, "evidence": {material.Evidence}, "reference_url": {material.ReferenceURL}, "start_seconds": {strconv.Itoa(material.StartSeconds)}, "end_seconds": {strconv.Itoa(material.EndSeconds)}, "estimated_seconds": {strconv.Itoa(material.EstimatedSeconds)}}
	if material.Diagram != nil {
		var nodes, edges []string
		for _, node := range material.Diagram.Nodes {
			nodes = append(nodes, node.ID+" | "+node.Label)
		}
		for _, edge := range material.Diagram.Edges {
			edges = append(edges, edge.From+" | "+edge.To+" | "+edge.Label)
		}
		form.Set("diagram_nodes", strings.Join(nodes, "\n"))
		form.Set("diagram_edges", strings.Join(edges, "\n"))
		form.Set("diagram_caption", material.Diagram.Caption)
	}
	for _, link := range material.Links {
		form.Add("unit_id", link.UnitID)
		form.Add("unit_version", strconv.Itoa(link.UnitVersion))
		form.Add("role", link.Role)
	}
	return form
}

func mergeForm(base, submitted url.Values) url.Values {
	for key, values := range submitted {
		base[key] = values
	}
	return base
}

func (s *server) editMaterial(w http.ResponseWriter, r *http.Request) {
	id, op := r.PathValue("id"), r.PostForm.Get("operation_id")
	original, err := s.store.Material(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, page{Operation: op})
		return
	}
	p := page{View: "material", Title: "Correct learning material", Active: "library", Material: original, Form: mergeForm(materialForm(original), r.PostForm), Operation: op}
	version, e1 := strconv.Atoi(r.PostForm.Get("version"))
	start, e2 := strconv.Atoi(r.PostForm.Get("start_seconds"))
	end, e3 := strconv.Atoi(r.PostForm.Get("end_seconds"))
	estimated, e4 := strconv.Atoi(r.PostForm.Get("estimated_seconds"))
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || op == "" || len(op) > 128 {
		s.fail(w, r, store.ErrInvalid, p)
		return
	}
	diagram, err := diagramFromForm(r.PostForm)
	if err != nil {
		s.fail(w, r, err, p)
		return
	}
	draft := store.GeneratedMaterial{Kind: original.Kind, Title: r.PostForm.Get("title"), Body: r.PostForm.Get("body"), Basis: original.Basis, Evidence: r.PostForm.Get("evidence"), ReferenceURL: r.PostForm.Get("reference_url"), StartSeconds: start, EndSeconds: end, EstimatedSeconds: estimated, Diagram: diagram}
	updated, err := s.store.EditMaterial(r.Context(), id, version, draft, r.PostForm.Get("reason"), op)
	if err != nil {
		s.fail(w, r, err, p)
		return
	}
	s.finish(w, r, "/materials/"+id, map[string]any{"material_id": updated.ID, "version": updated.Version})
}

func diagramFromForm(form url.Values) (*store.Diagram, error) {
	nodes, edges, caption := form.Get("diagram_nodes"), form.Get("diagram_edges"), form.Get("diagram_caption")
	if strings.TrimSpace(nodes) == "" && strings.TrimSpace(edges) == "" && strings.TrimSpace(caption) == "" {
		return nil, nil
	}
	diagram := &store.Diagram{Caption: caption}
	for _, row := range nonemptyLines(nodes) {
		parts := strings.SplitN(row, "|", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%w: write each diagram node as id | label", store.ErrInvalid)
		}
		diagram.Nodes = append(diagram.Nodes, store.DiagramNode{ID: strings.TrimSpace(parts[0]), Label: strings.TrimSpace(parts[1])})
	}
	for _, row := range nonemptyLines(edges) {
		parts := strings.SplitN(row, "|", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("%w: write each connection as from-id | to-id | label", store.ErrInvalid)
		}
		diagram.Edges = append(diagram.Edges, store.DiagramEdge{From: strings.TrimSpace(parts[0]), To: strings.TrimSpace(parts[1]), Label: strings.TrimSpace(parts[2])})
	}
	return diagram, nil
}

func (s *server) editCoverage(w http.ResponseWriter, r *http.Request) {
	id, op := r.PathValue("id"), r.PostForm.Get("operation_id")
	material, err := s.store.Material(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, page{Operation: op})
		return
	}
	p := page{View: "material", Title: "Correct coverage", Active: "library", Material: material, Form: mergeForm(materialForm(material), r.PostForm), Operation: op}
	version, err := strconv.Atoi(r.PostForm.Get("version"))
	if err != nil || op == "" || len(op) > 128 {
		s.fail(w, r, store.ErrInvalid, p)
		return
	}
	ids, versions, roles := r.PostForm["unit_id"], r.PostForm["unit_version"], r.PostForm["role"]
	if len(ids) != len(versions) || len(ids) != len(roles) || len(ids) > store.MaxMaterialLinks+1 {
		s.fail(w, r, store.ErrInvalid, p)
		return
	}
	links := make([]store.CoverageLink, 0, len(ids))
	for i, unitID := range ids {
		if roles[i] == "remove" {
			continue
		}
		if strings.TrimSpace(unitID) == "" && strings.TrimSpace(versions[i]) == "" && roles[i] == "" {
			continue
		}
		unitVersion, err := strconv.Atoi(versions[i])
		if err != nil || strings.TrimSpace(unitID) == "" || roles[i] == "" {
			s.fail(w, r, fmt.Errorf("%w: each coverage link needs a unit ID, definition version and role", store.ErrInvalid), p)
			return
		}
		links = append(links, store.CoverageLink{UnitID: strings.TrimSpace(unitID), UnitVersion: unitVersion, Role: roles[i]})
	}
	updated, err := s.store.EditCoverage(r.Context(), id, version, links, r.PostForm.Get("reason"), op)
	if err != nil {
		s.fail(w, r, err, p)
		return
	}
	s.finish(w, r, "/materials/"+id, map[string]any{"material_id": updated.ID, "version": updated.Version})
}

func (s *server) editRelation(w http.ResponseWriter, r *http.Request) {
	unitID, op := r.PostForm.Get("return_unit"), r.PostForm.Get("operation_id")
	detail, err := s.store.Unit(r.Context(), unitID, "recall", time.Time{})
	if err != nil {
		s.fail(w, r, err, page{Operation: op})
		return
	}
	p := page{View: "unit", Title: "Correct a relationship", Active: "library", Unit: detail, Mode: "recall", Form: mergeForm(unitForm(detail), r.PostForm), Operation: op}
	p.Form.Set("proposed", r.PostForm.Get("proposed"))
	p.Form.Set("archived", r.PostForm.Get("archived"))
	version, e1 := strconv.Atoi(r.PostForm.Get("relation_version"))
	fromVersion, e2 := strconv.Atoi(r.PostForm.Get("from_version"))
	toVersion, e3 := strconv.Atoi(r.PostForm.Get("to_version"))
	if e1 != nil || e2 != nil || e3 != nil || op == "" || len(op) > 128 {
		s.fail(w, r, store.ErrInvalid, p)
		return
	}
	change := store.RelationChange{ID: r.PostForm.Get("relation_id"), ExpectedVersion: version, FromID: r.PostForm.Get("from_id"), FromVersion: fromVersion, ToID: r.PostForm.Get("to_id"), ToVersion: toVersion, Kind: r.PostForm.Get("relation_kind"), Proposed: r.PostForm.Get("proposed") == "yes", Archived: r.PostForm.Get("archived") == "yes", Reason: r.PostForm.Get("reason"), Evidence: r.PostForm.Get("relation_evidence")}
	updated, err := s.store.EditRelation(r.Context(), change, op)
	if err != nil {
		s.fail(w, r, err, p)
		return
	}
	s.finish(w, r, "/units/"+unitID, map[string]any{"relation_id": updated.ID, "version": updated.Version})
}
