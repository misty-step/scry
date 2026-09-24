package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/misty-step/scry/internal/store"
)

// chartStar and chartLine are one goal's star chart: prerequisites sit to
// the left of the ideas that need them, so reading left to right follows the
// order Scry introduces them. Brightness is concept strength. The drawing is
// decorative; the concept list beside it carries the same facts in text.
type chartStar struct {
	X, Y, R, LabelX, LabelY float64
	Label, Class, Anchor    string
}
type chartLine struct{ X1, Y1, X2, Y2 float64 }
type chartView struct {
	Width, Height float64
	Stars         []chartStar
	Lines         []chartLine
}

const chartWidth, chartRow = 340.0, 58.0

func goalChart(view store.GoalView) *chartView {
	n := len(view.Concepts)
	if n == 0 {
		return nil
	}
	// Depth is the longest prerequisite chain below a concept. Edges run from
	// a concept to its prerequisite; n passes settle any acyclic order and
	// bound a cyclic one.
	depth := make([]int, n)
	for pass := 0; pass < n; pass++ {
		for _, e := range view.Edges {
			if e[0] < n && e[1] < n && depth[e[0]] < depth[e[1]]+1 && depth[e[1]]+1 < n {
				depth[e[0]] = depth[e[1]] + 1
			}
		}
	}
	columns := map[int][]int{}
	levels := 0
	for i, d := range depth {
		if len(view.Edges) == 0 {
			d = i % min(n, 4) // no relations: a loose row of up to four
		}
		columns[d] = append(columns[d], i)
		levels = max(levels, d+1)
	}
	rows := 1
	for _, col := range columns {
		rows = max(rows, len(col))
	}
	single := rows == 1
	chart := &chartView{Width: chartWidth, Height: 30 + float64(rows)*chartRow}
	colWidth := chartWidth / float64(levels)
	maxChars := max(7, int(colWidth/6.4))
	if single {
		// One row: labels alternate above and below, so each may use the
		// width of two columns.
		chart.Height = 104
		maxChars = max(7, int(min(colWidth*2, chartWidth/2)/6.4))
	}
	points := make([][2]float64, n)
	for level := 0; level < levels; level++ {
		col := columns[level]
		top := (chart.Height - float64(len(col))*chartRow) / 2
		for k, i := range col {
			x := colWidth * (float64(level) + .5)
			y := top + float64(k)*chartRow + 18
			if single {
				y = chart.Height / 2
			}
			points[i] = [2]float64{x, y}
		}
	}
	for _, e := range view.Edges {
		if e[0] < n && e[1] < n {
			a, b := points[e[1]], points[e[0]]
			chart.Lines = append(chart.Lines, chartLine{a[0], a[1], b[0], b[1]})
		}
	}
	level := make([]int, n)
	for d, col := range columns {
		for _, i := range col {
			level[i] = d
		}
	}
	for i, c := range view.Concepts {
		b := min(max(c.Brightness, 0), 5)
		// Edge columns anchor their labels inward so text never leaves the sky.
		anchor, labelX := "middle", points[i][0]
		if levels > 1 && level[i] == 0 {
			anchor, labelX = "start", points[i][0]-10
		} else if levels > 1 && level[i] == levels-1 {
			anchor, labelX = "end", points[i][0]+10
		}
		labelY := points[i][1] + 20
		if single && depth[i]%2 == 1 || single && len(view.Edges) == 0 && i%2 == 1 {
			labelY = points[i][1] - 13
		}
		chart.Stars = append(chart.Stars, chartStar{
			X: points[i][0], Y: points[i][1], R: 3.5 + float64(b)*.8, LabelX: labelX, LabelY: labelY, Anchor: anchor,
			Label: clip(c.Name, maxChars), Class: fmt.Sprintf("b%d", b),
		})
	}
	return chart
}

func clip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

func (s *server) mapPage(w http.ResponseWriter, r *http.Request) {
	view, err := s.store.Map(r.Context())
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	current, err := s.coldReview(r)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	// Material from the current question's own capture stays hidden until
	// assistance has been recorded.
	if current != nil {
		for i := range view.Unmapped {
			// Replace the whole value so fields added later stay hidden too.
			if u := view.Unmapped[i]; u.ID == current.Quiz.SourceID {
				view.Unmapped[i] = store.Source{ID: u.ID, Kind: u.Kind, Mode: u.Mode, CreatedAt: u.CreatedAt, Text: currentMaterial}
			}
		}
		hideCurrentMaterial(current, view.Preparing)
		for i := range view.Goals {
			g := &view.Goals[i]
			if g.Goal.SourceID != current.Quiz.SourceID {
				continue
			}
			g.Goal.Title = currentMaterial
			if g.Preparing != nil {
				receipt := *g.Preparing
				receipt.Title = currentMaterial
				g.Preparing = &receipt
			}
		}
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"map": view, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken()})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "map", Title: "Map", Active: "map", Map: view})
}

func (s *server) conceptPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	current, err := s.coldReview(r)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	if current != nil {
		linked, err := s.linkedConcepts(r, current)
		if err != nil {
			s.fail(w, r, err, page{})
			return
		}
		if linked[id] {
			s.gate(w, r, current)
			return
		}
	}
	view, err := s.store.ConceptPage(r.Context(), id)
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	// A concept page links to its capture; the capture's text is served only
	// by the gated Source page.
	if view.Source != nil {
		view.Source = &store.Source{ID: view.Source.ID}
	}
	if current != nil {
		for i := range view.Goals {
			if view.Goals[i].SourceID == current.Quiz.SourceID {
				view.Goals[i].Title = currentMaterial
			}
		}
	}
	if wantsJSON(r) {
		jsonResponse(w, http.StatusOK, map[string]any{"concept": view, "csrf": r.Context().Value(csrfKey{}), "operation_id": randomToken()})
		return
	}
	s.render(w, r, http.StatusOK, page{View: "concept", Title: view.Concept.Name, Active: "map", Concept: view})
}

func conceptOperation(r *http.Request) (string, error) {
	op := r.PostForm.Get("operation_id")
	if op == "" || len(op) > 128 {
		return "", fmt.Errorf("%w: reload and try again", store.ErrInvalid)
	}
	return op, nil
}
func (s *server) practiceConcept(w http.ResponseWriter, r *http.Request) {
	op, err := conceptOperation(r)
	if err == nil {
		err = s.store.PracticeConcept(r.Context(), r.PathValue("id"), op)
	}
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/", map[string]any{"practicing": r.PathValue("id")})
}
func (s *server) requestQuestions(w http.ResponseWriter, r *http.Request) {
	op, err := conceptOperation(r)
	if err == nil {
		err = s.store.RequestQuestions(r.Context(), r.PathValue("id"), op)
	}
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/concepts/"+r.PathValue("id"), map[string]any{"requested": true})
}
func (s *server) archiveConcept(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ArchiveConcept(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/map", map[string]any{"archived": true})
}
func (s *server) updateGoal(w http.ResponseWriter, r *http.Request) {
	action := r.PostForm.Get("action")
	switch action {
	case "pause", "resume", "focus", "unfocus":
	default:
		s.fail(w, r, fmt.Errorf("%w: choose an available action", store.ErrInvalid), page{})
		return
	}
	if err := s.store.UpdateGoal(r.Context(), r.PathValue("id"), action); err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/map", map[string]any{"action": action})
}
func (s *server) fixQuiz(w http.ResponseWriter, r *http.Request) {
	instruction := strings.TrimSpace(r.PostForm.Get("instruction"))
	if instruction == "" || len(instruction) > 1000 {
		s.fail(w, r, fmt.Errorf("%w: describe the correction in 1–1000 bytes", store.ErrInvalid), page{})
		return
	}
	op, err := conceptOperation(r)
	if err == nil {
		err = s.store.RequestFix(r.Context(), r.PathValue("id"), instruction, op)
	}
	if err != nil {
		s.fail(w, r, err, page{})
		return
	}
	s.finish(w, r, "/quizzes/"+r.PathValue("id")+"/edit", map[string]any{"requested": true})
}

// decideProposal applies or discards a suggested fix; nothing changes until
// the learner chooses.
