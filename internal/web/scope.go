package web

import "net/http"

// Each answer-bearing page names the same concrete inspection scope used by its
// read guard. The browser receives server-relative expiry, never a fresh sliding
// lease inferred from navigation, focus, polling, or an estimate read.
func pageScope(p *page) (kind, id string) {
	switch p.View {
	case "library":
		return "library", p.Query
	case "source":
		return "source", p.Source.ID
	case "goal":
		return "goal", p.Goal.ID
	case "unit":
		return "unit", p.Unit.Unit.ID
	case "material":
		return "material", p.Material.ID
	case "edit":
		return "quiz", p.Quiz.ID
	case "history":
		if !p.HistoryHidden {
			return "history", ""
		}
	case "dispute":
		if p.Quiz.Prompt != "" {
			return "history", ""
		}
	case "review":
		if p.Review.Current != nil && (p.Review.Current.Kind == "reference" || p.Review.Current.Graded) {
			return "presentation", p.Review.Current.ID
		}
	}
	return "", ""
}

func (s *server) prepareScope(w http.ResponseWriter, r *http.Request, p *page) bool {
	kind, id := pageScope(p)
	if kind == "" {
		return true
	}
	access, err := s.store.InspectionAccess(r.Context(), kind, id)
	if err != nil {
		http.Error(w, "The saved reading scope could not be checked. Your work is retained; reload to recover it.", http.StatusServiceUnavailable)
		return false
	}
	destination := inspectionDestination(kind, id)
	if p.View == "review" {
		destination = "/"
	}
	if !access.Allowed || access.RequiresAssistance {
		s.render(w, r, http.StatusOK, page{View: "gate", Title: "Continue reading", Active: p.Active, Gate: access.RequiresAssistance, GateKind: kind, GateID: id, ReturnTo: destination})
		return false
	}
	p.ScopeKind, p.ScopeID, p.ScopeReturn = kind, id, destination
	p.ScopeExpiresAt, p.ScopeCheckedAt = access.ExpiresAt, access.CheckedAt
	return true
}

func (s *server) sessionStatus(w http.ResponseWriter, r *http.Request) {
	kind, id := r.URL.Query().Get("kind"), r.URL.Query().Get("id")
	result := map[string]any{"authenticated": true, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken()}
	if kind != "" {
		access, err := s.store.InspectionAccess(r.Context(), kind, id)
		if err != nil {
			s.fail(w, r, err, page{})
			return
		}
		result["access"] = access
	}
	jsonResponse(w, http.StatusOK, result)
}
