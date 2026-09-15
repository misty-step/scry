package web

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"

	"github.com/misty-step/scry/internal/learning"
	"github.com/misty-step/scry/internal/store"
)

func materialKind(kind string) string {
	switch kind {
	case "quiz":
		return "Assessment"
	case "explanation":
		return "Explanation"
	case "worked_example":
		return "Worked example"
	case "diagram":
		return "Diagram"
	case "article":
		return "Article reference"
	case "video":
		return "Video reference"
	default:
		return "Learning material"
	}
}

func knowledgeKind(kind string) string {
	switch kind {
	case "foundation":
		return "Foundation"
	case "concept":
		return "Concept"
	case "composition":
		return "Composition or application"
	case "procedure":
		return "Procedure"
	case "exact_text":
		return "Exact wording"
	default:
		return "Knowledge unit"
	}
}

func coverageRole(role string) string {
	switch role {
	case "assesses":
		return "Directly assesses"
	case "teaches":
		return "Teaches"
	case "assumes":
		return "Assumes; not assessed"
	case "mentions":
		return "Mentions; not assessed"
	default:
		return "Unmapped"
	}
}

func safeReference(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || strings.ContainsAny(raw, "\\\r\n\t ") {
		return ""
	}
	return u.String()
}

func diagramLabel(diagram *store.Diagram, id string) string {
	if diagram != nil {
		for _, node := range diagram.Nodes {
			if node.ID == id {
				return node.Label
			}
		}
	}
	return "Unresolved diagram connection"
}

func publicSource(source store.Source) store.Source {
	if source.Job != nil {
		job := *source.Job
		job.Context = store.KnowledgeContext{}
		job.SourceText = ""
		source.Job = &job
	}
	return source
}

func publicGoal(goal store.Goal) store.Goal {
	if goal.Jobs != nil {
		goal.Jobs = append([]store.Job(nil), goal.Jobs...)
		for i := range goal.Jobs {
			goal.Jobs[i].Context = store.KnowledgeContext{}
			goal.Jobs[i].SourceText = ""
		}
	}
	return goal
}

func estimateState(state string) string {
	switch state {
	case "exposed":
		return "Seen, not demonstrated"
	case "assumed":
		return "Assumed, not assessed"
	case "ambiguous":
		return "Attribution is uncertain"
	case "disputed":
		return "Disputed evidence"
	case "assisted":
		return "Helped evidence"
	case "gap":
		return "Observed difficulty"
	case "supported":
		return "Inferred support"
	case "demonstrated":
		return "Recent direct evidence"
	case "stale":
		return "Direct evidence is aging"
	default:
		return "Insufficient direct evidence"
	}
}

func probability(value *float64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%.0f%%", *value*100)
}

type templateContext struct {
	Page  *page
	Value any
}

func within(p *page, value any) templateContext { return templateContext{Page: p, Value: value} }

type inspectionNavigation struct {
	CSRF        string
	Operation   string
	Kind        string
	ID          string
	Destination string
	Label       string
	Helped      bool
	Current     bool
}

func inspectionDestination(kind, id string) string {
	switch kind {
	case "presentation":
		return "/"
	case "library":
		if id != "" {
			return "/library?" + url.Values{"q": {id}}.Encode()
		}
		return "/library"
	case "source":
		return "/sources/" + id
	case "quiz":
		return "/quizzes/" + id + "/edit"
	case "goal":
		return "/goals/" + id
	case "unit":
		return "/units/" + id
	case "material":
		return "/materials/" + id
	case "history":
		return "/history"
	case "export":
		return "/export"
	default:
		return ""
	}
}

func openLink(p *page, kind, id, label string) inspectionNavigation {
	digest := sha256.Sum256([]byte(p.Operation + "\x00" + kind + "\x00" + id))
	return inspectionNavigation{CSRF: p.CSRF, Operation: hex.EncodeToString(digest[:]), Kind: kind, ID: id, Destination: inspectionDestination(kind, id), Label: label, Helped: p.InspectionHelp, Current: kind == "library" && p.Active == "library" || kind == "history" && p.Active == "history"}
}

func formAt(values url.Values, key string, index int) string {
	if index < 0 || index >= len(values[key]) {
		return ""
	}
	return values[key][index]
}

func planKind(kind string) string {
	switch kind {
	case "defer":
		return "Practice deferred by other direct evidence"
	case "plan", "pace":
		return "Pace or focus changed"
	case "suggestion_accept":
		return "Goal-aligned option accepted"
	case "suggestion_decline":
		return "Option declined"
	case "undo":
		return "Planning effects reversed"
	case "suggestion_refresh":
		return "Goal-aligned options refreshed"
	case "suggestion_obsolete":
		return "Outdated option retained for inspection"
	default:
		return "Recorded planning decision"
	}
}

func planUndoable(kind string) bool {
	switch kind {
	case "plan", "suggestion_accept", "suggestion_decline", "defer", "pace":
		return true
	default:
		return false
	}
}

func activityKind(kind string) string {
	switch kind {
	case "reference", "read", "continue":
		return "Reading continued"
	case "inspection":
		return "Content inspected"
	case "bridge_request":
		return "Foundations requested"
	case "bridge_return":
		return "Returned to target"
	case "assistance":
		return "Help recorded"
	default:
		return "Learning activity"
	}
}

func unitState(estimates []learning.Estimate, id string) string {
	for _, estimate := range estimates {
		if estimate.Unit.ID == id {
			return estimateState(estimate.State)
		}
	}
	return estimateState("unknown")
}
