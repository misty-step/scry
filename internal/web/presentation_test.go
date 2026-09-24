package web

import (
	"strings"
	"testing"

	"github.com/misty-step/scry/internal/store"
)

// The star chart reads left to right in introduction order: a prerequisite
// must sit left of every idea that needs it, and every label must stay inside
// the drawing whatever its anchor.
func TestGoalChartPlacesPrerequisitesFirst(t *testing.T) {
	names := []string{"Antigen recognition", "Antibody response", "T-cell roles", "Memory cells", "Booster doses"}
	view := store.GoalView{Edges: [][2]int{{1, 0}, {2, 0}, {3, 1}, {3, 2}, {4, 3}}}
	for _, name := range names {
		view.Concepts = append(view.Concepts, store.ConceptBrief{Name: name, Brightness: 2})
	}
	chart := goalChart(view)
	for _, e := range view.Edges {
		if dependent, prerequisite := chart.Stars[e[0]], chart.Stars[e[1]]; dependent.X <= prerequisite.X {
			t.Errorf("%s at x=%.1f is not right of its prerequisite %s at x=%.1f", names[e[0]], dependent.X, names[e[1]], prerequisite.X)
		}
	}
	for i, star := range chart.Stars {
		width := float64(len([]rune(star.Label))) * 6.4
		left, right := star.LabelX-width/2, star.LabelX+width/2
		switch star.Anchor {
		case "start":
			left, right = star.LabelX, star.LabelX+width
		case "end":
			left, right = star.LabelX-width, star.LabelX
		}
		if left < 0 || right > chart.Width || star.LabelY < 0 || star.LabelY > chart.Height {
			t.Errorf("%s label %q leaves the chart: %.1f..%.1f at y=%.1f", names[i], star.Label, left, right, star.LabelY)
		}
	}
	if goalChart(store.GoalView{}) != nil {
		t.Error("a goal without concepts drew a chart")
	}
}

// Stored step errors name internal checks; the learner-facing summary keeps
// the message and drops those identifiers, and a partial step never claims
// the whole step failed.
func TestJobSummaryKeepsInternalsOutOfLearnerCopy(t *testing.T) {
	failed := jobSummary(store.Job{Status: "failed", Error: "Scry could not write material that passed its checks (concept \"Model assumptions\": source evidence is not an exact quotation). Nothing was published."})
	if failed != "Scry could not write material that passed its checks. Nothing was published." {
		t.Errorf("failed summary = %q", failed)
	}
	partial := jobSummary(store.Job{Status: "partial", Error: "2 questions were not published: question failed strengthened_source_claim (2)"})
	if strings.Contains(partial, "strengthened_source_claim") || !strings.Contains(partial, "questions") {
		t.Errorf("partial summary = %q", partial)
	}
}
