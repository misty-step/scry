package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

func scanNote(row interface{ Scan(...any) error }) (Note, error) {
	var n Note
	var evidence, citations string
	err := row.Scan(&n.ID, &n.ConceptID, &n.Title, &n.Body, &n.Basis, &evidence, &citations, &n.SourceID, &n.JobID, &n.Model, &n.PromptVersion, &n.Supersedes, &n.CreatedAt)
	if err != nil {
		return n, err
	}
	if err = json.Unmarshal([]byte(evidence), &n.Evidence); err != nil {
		return n, err
	}
	err = json.Unmarshal([]byte(citations), &n.Citations)
	return n, err
}

const noteColumns = `id,concept_id,title,body,basis,evidence,citations,source_id,job_id,model,prompt_version,supersedes,created_at`

func currentNote(ctx context.Context, tx *sql.Tx, conceptID string) (*Note, error) {
	n, err := scanNote(tx.QueryRowContext(ctx, `SELECT `+noteColumns+` FROM notes WHERE concept_id=? ORDER BY created_at DESC,rowid DESC LIMIT 1`, conceptID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

type conceptRow struct {
	Concept
	order int64
}

// loadConcepts returns every active concept visible in v5 surfaces.
func loadConcepts(ctx context.Context, tx *sql.Tx) (map[string]conceptRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,name,description,created_at,origin,status,source_id,rowid FROM concepts WHERE status='active' AND origin<>'foundation'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]conceptRow{}
	for rows.Next() {
		var c conceptRow
		if err = rows.Scan(&c.ID, &c.Name, &c.Description, &c.CreatedAt, &c.Origin, &c.Status, &c.SourceID, &c.order); err != nil {
			return nil, err
		}
		result[c.ID] = c
	}
	return result, rows.Err()
}

// conceptStates derives each requested concept's state from its questions'
// FSRS cards and attempts. A nil filter means every concept.
func conceptStates(ctx context.Context, tx *sql.Tx, filter map[string]bool, now int64) (map[string]learning.ConceptState, error) {
	rows, err := tx.QueryContext(ctx, `SELECT cq.concept_id,q.id,sc.card,v.content FROM concept_quizzes cq JOIN quizzes q ON q.id=cq.quiz_id
	 JOIN sources src ON src.id=q.source_id JOIN schedules sc ON sc.quiz_id=q.id JOIN quiz_versions v ON v.quiz_id=q.id AND v.version=q.version
	 WHERE cq.role='assesses' AND q.archived=0 AND src.archived=0`)
	if err != nil {
		return nil, err
	}
	questions := map[string]*learning.QuestionEvidence{}
	conceptOf := map[string]string{}
	for rows.Next() {
		var concept, quizID, cardJSON, content string
		if err = rows.Scan(&concept, &quizID, &cardJSON, &content); err != nil {
			rows.Close()
			return nil, err
		}
		if filter != nil && !filter[concept] {
			continue
		}
		var card learning.Card
		if err = json.Unmarshal([]byte(cardJSON), &card); err != nil {
			rows.Close()
			return nil, err
		}
		var generated GeneratedQuiz
		if err = json.Unmarshal([]byte(content), &generated); err != nil {
			rows.Close()
			return nil, err
		}
		questions[quizID] = &learning.QuestionEvidence{Card: card, Level: generated.Level}
		conceptOf[quizID] = concept
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	attempts, err := tx.QueryContext(ctx, `SELECT p.quiz_id,e.reviewed_at,e.rating,e.assisted,e.outcome,COALESCE(o.direction,'')
	 FROM review_events e JOIN presentations p ON p.id=e.presentation_id LEFT JOIN grade_overrides o ON o.review_id=e.id
	 WHERE e.rating>0 OR e.assisted=1 OR substr(e.outcome,1,5)='warm_' ORDER BY e.reviewed_at,e.rowid`)
	if err != nil {
		return nil, err
	}
	for attempts.Next() {
		var quizID, outcome, override string
		var a learning.Attempt
		if err = attempts.Scan(&quizID, &a.At, &a.Rating, &a.Assisted, &outcome, &override); err != nil {
			attempts.Close()
			return nil, err
		}
		q, ok := questions[quizID]
		if !ok {
			continue
		}
		a.Outcome = outcome
		switch override {
		case "correct":
			a.Rating, a.Assisted, a.Outcome = 3, false, "correct"
		case "missed":
			a.Rating, a.Outcome = 1, "wrong"
		}
		q.Attempts = append(q.Attempts, a)
	}
	if err = attempts.Err(); err != nil {
		attempts.Close()
		return nil, err
	}
	attempts.Close()
	grouped := map[string][]learning.QuestionEvidence{}
	for quizID, q := range questions {
		grouped[conceptOf[quizID]] = append(grouped[conceptOf[quizID]], *q)
	}
	states := map[string]learning.ConceptState{}
	at := time.UnixMilli(now)
	for concept := range filter {
		states[concept] = learning.ComputeConceptState(grouped[concept], at)
	}
	if filter == nil {
		for concept, list := range grouped {
			states[concept] = learning.ComputeConceptState(list, at)
		}
	}
	return states, nil
}

func brief(c conceptRow, state learning.ConceptState) ConceptBrief {
	if state.Status == "" {
		state = learning.ComputeConceptState(nil, time.Now())
	}
	return ConceptBrief{ID: c.ID, Name: c.Name, Summary: c.Description, Status: state.Status, Recall: state.Recall, Brightness: state.Brightness}
}

func conceptBriefs(ctx context.Context, tx *sql.Tx, ids []string, now int64) ([]ConceptBrief, error) {
	concepts, err := loadConcepts(ctx, tx)
	if err != nil {
		return nil, err
	}
	filter := map[string]bool{}
	for _, id := range ids {
		filter[id] = true
	}
	states, err := conceptStates(ctx, tx, filter, now)
	if err != nil {
		return nil, err
	}
	briefs := make([]ConceptBrief, 0, len(ids))
	for _, id := range ids {
		if c, ok := concepts[id]; ok {
			briefs = append(briefs, brief(c, states[id]))
		}
	}
	return briefs, nil
}

var stageLabels = map[string]string{
	"research": "Searching the web", "transcribe": "Reading your photo", "plan": "Mapping the ideas",
	"questions": "Writing questions", "contrast": "Writing a comparison", "quizzes": "Writing questions",
}

// preparingFor describes a source's live preparation, or one that stopped
// within window milliseconds (0 means at any age).
func preparingFor(ctx context.Context, tx *sql.Tx, src Source, jobs []Job, now, window int64) (*Preparing, error) {
	last, ok := lastPreparation(jobs)
	if src.Archived || !ok {
		return nil, nil
	}
	live := last.Status == "queued" || last.Status == "running" || last.Status == "retry"
	stopped := last.Status == "failed" || last.Status == "paused" || (last.Status == "canceled" && last.Kind != "quizzes")
	if !live && !(stopped && (window == 0 || last.UpdatedAt >= now-window)) {
		return nil, nil
	}
	p := &Preparing{SourceID: src.ID, Stage: last.Kind, Label: stageLabels[last.Kind]}
	if last.Kind == "research" && src.Mode == "link" {
		p.Label = "Reading the page"
	}
	if stopped {
		p.Stage, p.Label, p.Failed, p.Error = "failed", "Preparation stopped", true, last.Error
	}
	var title string
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE((SELECT title FROM goals WHERE source_id=?),'')", src.ID).Scan(&title); err != nil {
		return nil, err
	}
	if title == "" {
		title = excerptRunes(src.Text, 120)
	}
	p.Title = title
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM goal_concepts gc JOIN goals g ON g.id=gc.goal_id WHERE g.source_id=?`, src.ID).Scan(&p.Concepts); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM quizzes WHERE source_id=? AND archived=0`, src.ID).Scan(&p.Questions); err != nil {
		return nil, err
	}
	return p, nil
}

// preparingList describes captures still being prepared, and ones whose
// preparation stopped within the last day.
func preparingList(ctx context.Context, tx *sql.Tx, now int64) ([]Preparing, error) {
	ids, err := stringSet(ctx, tx, `SELECT DISTINCT j.source_id FROM jobs j JOIN sources s ON s.id=j.source_id WHERE s.archived=0 AND j.kind<>'fix'
	 AND (j.status IN ('queued','running','retry') OR (j.status IN ('failed','paused','canceled') AND j.updated_at>=?))`, now-remedialWindow)
	if err != nil {
		return nil, err
	}
	list := []Preparing{}
	for id := range ids {
		src, err := source(ctx, tx, id, false)
		if err != nil {
			return nil, err
		}
		jobs, err := sourceJobs(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		p, err := preparingFor(ctx, tx, src, jobs, now, remedialWindow)
		if err != nil {
			return nil, err
		}
		if p != nil {
			list = append(list, *p)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].SourceID < list[j].SourceID })
	return list, nil
}

type relation struct{ from, to, kind string }

func liveRelations(ctx context.Context, tx *sql.Tx) ([]relation, error) {
	rows, err := tx.QueryContext(ctx, `SELECT from_id,to_id,kind FROM concept_relations WHERE retired_at=0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []relation
	for rows.Next() {
		var r relation
		if err = rows.Scan(&r.from, &r.to, &r.kind); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// prerequisiteOrder sorts concepts so prerequisites come first, keeping
// creation order otherwise.
func prerequisiteOrder(ids []string, concepts map[string]conceptRow, relations []relation) []string {
	in := map[string]bool{}
	for _, id := range ids {
		in[id] = true
	}
	requires := map[string][]string{}
	for _, r := range relations {
		if r.kind == "requires" && in[r.from] && in[r.to] {
			requires[r.from] = append(requires[r.from], r.to)
		}
	}
	sorted := append([]string(nil), ids...)
	sort.SliceStable(sorted, func(i, j int) bool { return concepts[sorted[i]].order < concepts[sorted[j]].order })
	var result []string
	done, visiting := map[string]bool{}, map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if done[id] || visiting[id] {
			return
		}
		visiting[id] = true
		deps := requires[id]
		sort.SliceStable(deps, func(i, j int) bool { return concepts[deps[i]].order < concepts[deps[j]].order })
		for _, dep := range deps {
			visit(dep)
		}
		visiting[id], done[id] = false, true
		result = append(result, id)
	}
	for _, id := range sorted {
		visit(id)
	}
	return result
}

type questionStat struct{ total, due int }

func questionStats(ctx context.Context, tx *sql.Tx, now int64) (map[string]questionStat, error) {
	rows, err := tx.QueryContext(ctx, `SELECT cq.concept_id,`+quizAvailableAtSQL+` FROM concept_quizzes cq JOIN quizzes q ON q.id=cq.quiz_id
	 JOIN sources src ON src.id=q.source_id JOIN schedules sc ON sc.quiz_id=q.id WHERE cq.role='assesses' AND q.archived=0 AND src.archived=0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats := map[string]questionStat{}
	for rows.Next() {
		var concept string
		var available int64
		if err = rows.Scan(&concept, &available); err != nil {
			return nil, err
		}
		s := stats[concept]
		s.total++
		if available <= now {
			s.due++
		}
		stats[concept] = s
	}
	return stats, rows.Err()
}

func (s *Store) Map(ctx context.Context) (MapView, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MapView{}, err
	}
	defer tx.Rollback()
	now := s.now()
	view := MapView{Goals: []GoalView{}, Unmapped: []Source{}}
	concepts, err := loadConcepts(ctx, tx)
	if err != nil {
		return view, err
	}
	states, err := conceptStates(ctx, tx, nil, now)
	if err != nil {
		return view, err
	}
	relations, err := liveRelations(ctx, tx)
	if err != nil {
		return view, err
	}
	stats, err := questionStats(ctx, tx, now)
	if err != nil {
		return view, err
	}
	if view.Preparing, err = preparingList(ctx, tx, now); err != nil {
		return view, err
	}
	preparingBySource := map[string]*Preparing{}
	for i := range view.Preparing {
		preparingBySource[view.Preparing[i].SourceID] = &view.Preparing[i]
	}
	rows, err := tx.QueryContext(ctx, `SELECT g.id,g.title,g.source_id,g.status,g.focus,g.created_at FROM goals g JOIN sources s ON s.id=g.source_id
	 WHERE g.status<>'archived' AND s.archived=0 ORDER BY (g.status='active') DESC,g.focus DESC,g.created_at DESC,g.rowid DESC`)
	if err != nil {
		return view, err
	}
	var goals []Goal
	for rows.Next() {
		var g Goal
		if err = rows.Scan(&g.ID, &g.Title, &g.SourceID, &g.Status, &g.Focus, &g.CreatedAt); err != nil {
			rows.Close()
			return view, err
		}
		goals = append(goals, g)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return view, err
	}
	rows.Close()
	members := map[string][]string{}
	memberRows, err := tx.QueryContext(ctx, `SELECT goal_id,concept_id FROM goal_concepts ORDER BY created_at,rowid`)
	if err != nil {
		return view, err
	}
	for memberRows.Next() {
		var goal, concept string
		if err = memberRows.Scan(&goal, &concept); err != nil {
			memberRows.Close()
			return view, err
		}
		if _, ok := concepts[concept]; ok {
			members[goal] = append(members[goal], concept)
		}
	}
	if err = memberRows.Err(); err != nil {
		memberRows.Close()
		return view, err
	}
	memberRows.Close()
	for _, g := range goals {
		ids := prerequisiteOrder(members[g.ID], concepts, relations)
		gv := GoalView{Goal: g, Concepts: []ConceptBrief{}, Edges: [][2]int{}, Preparing: preparingBySource[g.SourceID]}
		position := map[string]int{}
		for i, id := range ids {
			position[id] = i
			gv.Concepts = append(gv.Concepts, brief(concepts[id], states[id]))
			gv.Questions += stats[id].total
			gv.Due += stats[id].due
		}
		for _, r := range relations {
			from, okFrom := position[r.from]
			to, okTo := position[r.to]
			if r.kind == "requires" && okFrom && okTo {
				gv.Edges = append(gv.Edges, [2]int{from, to})
			}
		}
		if len(gv.Concepts) == 0 && gv.Preparing == nil {
			// A capture whose preparation stopped stays reachable here at any
			// age, so its retry is never lost; other concept-less goals are
			// legacy material listed under unmapped questions.
			src, err := source(ctx, tx, g.SourceID, false)
			if err != nil {
				return view, err
			}
			jobs, err := sourceJobs(ctx, tx, g.SourceID)
			if err != nil {
				return view, err
			}
			if gv.Preparing, err = preparingFor(ctx, tx, src, jobs, now, 0); err != nil {
				return view, err
			}
			if gv.Preparing == nil {
				continue
			}
		}
		view.Goals = append(view.Goals, gv)
	}
	unmapped, err := stringSet(ctx, tx, `SELECT DISTINCT q.source_id FROM quizzes q JOIN sources s ON s.id=q.source_id WHERE q.archived=0 AND s.archived=0
	 AND NOT EXISTS(SELECT 1 FROM concept_quizzes cq WHERE cq.quiz_id=q.id AND cq.role='assesses')`)
	if err != nil {
		return view, err
	}
	for id := range unmapped {
		src, err := source(ctx, tx, id, false)
		if err != nil {
			return view, err
		}
		view.Unmapped = append(view.Unmapped, src)
	}
	sort.Slice(view.Unmapped, func(i, j int) bool { return view.Unmapped[i].CreatedAt > view.Unmapped[j].CreatedAt })
	return view, tx.Commit()
}

// SearchConcepts finds active concepts related to text, for reuse during
// generation. It never returns retired foundation units.
func (s *Store) SearchConcepts(ctx context.Context, text string, limit int) ([]ConceptBrief, error) {
	if limit <= 0 || limit > 50 {
		limit = 12
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := searchConcepts(ctx, tx, text, limit, s.now())
	if err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

func searchConcepts(ctx context.Context, tx *sql.Tx, text string, limit int, now int64) ([]ConceptBrief, error) {
	match := ftsAny(text)
	if match == "" {
		return []ConceptBrief{}, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT ref FROM concept_index WHERE concept_index MATCH ? ORDER BY rank LIMIT ?`, match, limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return conceptBriefs(ctx, tx, ids, now)
}

func (s *Store) ConceptPage(ctx context.Context, id string) (ConceptView, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ConceptView{}, err
	}
	defer tx.Rollback()
	now := s.now()
	concepts, err := loadConcepts(ctx, tx)
	if err != nil {
		return ConceptView{}, err
	}
	c, ok := concepts[id]
	if !ok {
		return ConceptView{}, fmt.Errorf("%w: concept", ErrNotFound)
	}
	view := ConceptView{Concept: c.Concept, Goals: []Goal{},
		Requires: []ConceptBrief{}, RequiredBy: []ConceptBrief{}, PartOf: []ConceptBrief{}, Parts: []ConceptBrief{}, ConfusedWith: []ConceptBrief{},
		Questions: []Quiz{}, Documents: []SourceDocument{}}
	states, err := conceptStates(ctx, tx, nil, now)
	if err != nil {
		return view, err
	}
	view.State = states[id]
	if view.State.Status == "" {
		view.State = learning.ComputeConceptState(nil, time.UnixMilli(now))
	}
	if view.Note, err = currentNote(ctx, tx, id); err != nil {
		return view, err
	}
	relations, err := liveRelations(ctx, tx)
	if err != nil {
		return view, err
	}
	add := func(list *[]ConceptBrief, other string) {
		if o, ok := concepts[other]; ok {
			*list = append(*list, brief(o, states[other]))
		}
	}
	for _, r := range relations {
		switch {
		case r.kind == "requires" && r.from == id:
			add(&view.Requires, r.to)
		case r.kind == "requires" && r.to == id:
			add(&view.RequiredBy, r.from)
		case r.kind == "part_of" && r.from == id:
			add(&view.PartOf, r.to)
		case r.kind == "part_of" && r.to == id:
			add(&view.Parts, r.from)
		case r.kind == "confused_with" && r.from == id:
			add(&view.ConfusedWith, r.to)
		case r.kind == "confused_with" && r.to == id:
			add(&view.ConfusedWith, r.from)
		}
	}
	goalRows, err := tx.QueryContext(ctx, `SELECT g.id,g.title,g.source_id,g.status,g.focus,g.created_at FROM goals g JOIN goal_concepts gc ON gc.goal_id=g.id
	 WHERE gc.concept_id=? AND g.status<>'archived' ORDER BY g.created_at`, id)
	if err != nil {
		return view, err
	}
	for goalRows.Next() {
		var g Goal
		if err = goalRows.Scan(&g.ID, &g.Title, &g.SourceID, &g.Status, &g.Focus, &g.CreatedAt); err != nil {
			goalRows.Close()
			return view, err
		}
		view.Goals = append(view.Goals, g)
	}
	if err = goalRows.Err(); err != nil {
		goalRows.Close()
		return view, err
	}
	goalRows.Close()
	quizIDs, err := orderedIDs(ctx, tx, `SELECT q.id FROM concept_quizzes cq JOIN quizzes q ON q.id=cq.quiz_id JOIN sources s ON s.id=q.source_id
	 WHERE cq.concept_id=? AND cq.role='assesses' AND q.archived=0 AND s.archived=0 ORDER BY q.created_at,q.rowid`, id)
	if err != nil {
		return view, err
	}
	for _, qid := range quizIDs {
		q, err := quiz(ctx, tx, qid)
		if err != nil {
			return view, err
		}
		view.Questions = append(view.Questions, q)
	}
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE status IN ('queued','running','retry') AND kind='questions'
	 AND (json_extract(payload,'$.concept_id')=? OR (payload='{}' AND source_id=?)))`, id, c.SourceID).Scan(&view.QuestionsPending); err != nil {
		return view, err
	}
	cited := map[string]bool{}
	if view.Note != nil {
		for _, citation := range view.Note.Citations {
			cited[citation.DocumentID] = true
		}
	}
	for _, q := range view.Questions {
		for _, citation := range q.Citations {
			cited[citation.DocumentID] = true
		}
	}
	if c.SourceID != "" {
		src, err := source(ctx, tx, c.SourceID, false)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return view, err
		}
		if err == nil {
			view.Source = &src
			documents, err := sourceDocuments(ctx, tx, c.SourceID)
			if err != nil {
				return view, err
			}
			for _, d := range documents {
				if cited[d.ID] || d.Kind == "page" {
					d.Text = ""
					view.Documents = append(view.Documents, d)
				}
			}
		}
	}
	return view, tx.Commit()
}

func orderedIDs(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// QuizConcepts lists every concept a question is linked to: the one it
// assesses and any it contrasts with. Answer-secrecy gates cover all of them.
func (s *Store) QuizConcepts(ctx context.Context, quizID string) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ids, err := orderedIDs(ctx, tx, `SELECT concept_id FROM concept_quizzes WHERE quiz_id=? AND role IN ('assesses','contrasts') ORDER BY concept_id`, quizID)
	if err != nil {
		return nil, err
	}
	return ids, tx.Commit()
}

// activeConcept loads a concept that learner actions may target.
func activeConcept(ctx context.Context, tx *sql.Tx, id string) (Concept, error) {
	var c Concept
	err := tx.QueryRowContext(ctx, `SELECT id,name,description,created_at,origin,status,source_id FROM concepts WHERE id=? AND status='active' AND origin<>'foundation'`, id).
		Scan(&c.ID, &c.Name, &c.Description, &c.CreatedAt, &c.Origin, &c.Status, &c.SourceID)
	return c, notFound(err, "concept")
}

// onceOperation runs mutate at most once per operation ID and payload.
func (s *Store) onceOperation(ctx context.Context, operationID, kind string, payload any, mutate func(*sql.Tx, int64) (string, error)) error {
	if err := validOperation(operationID); err != nil {
		return err
	}
	hash := payloadHash(payload)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, _, found, err := existingOperation(ctx, tx, operationID, kind, hash)
	if err != nil || found {
		if err == nil {
			err = tx.Commit()
		}
		return err
	}
	now := s.now()
	resultID, err := mutate(tx, now)
	if err != nil {
		return err
	}
	if err = saveOperation(ctx, tx, operationID, kind, hash, resultID, nil, now); err != nil {
		return err
	}
	return tx.Commit()
}

func conceptSource(ctx context.Context, tx *sql.Tx, c Concept) (string, int, error) {
	var revision int
	var archived bool
	err := tx.QueryRowContext(ctx, "SELECT revision,archived FROM sources WHERE id=?", c.SourceID).Scan(&revision, &archived)
	if err != nil || archived {
		return "", 0, fmt.Errorf("%w: this concept's material is no longer available", ErrConflict)
	}
	return c.SourceID, revision, nil
}

func (s *Store) PracticeConcept(ctx context.Context, conceptID, operationID string) error {
	return s.onceOperation(ctx, operationID, "practice", conceptID, func(tx *sql.Tx, now int64) (string, error) {
		if _, err := activeConcept(ctx, tx, conceptID); err != nil {
			return "", err
		}
		var questions int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM concept_quizzes cq JOIN quizzes q ON q.id=cq.quiz_id JOIN sources s ON s.id=q.source_id
		 WHERE cq.concept_id=? AND cq.role='assesses' AND q.archived=0 AND s.archived=0`, conceptID).Scan(&questions); err != nil {
			return "", err
		}
		if questions == 0 {
			return "", fmt.Errorf("%w: this concept has no questions yet", ErrInvalid)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE review_session SET focus_concept=?,focus_until=? WHERE singleton=1", conceptID, now+focusWindow); err != nil {
			return "", err
		}
		// An untouched question of another concept steps aside; an answer in
		// progress (being checked or awaiting self-check) is never discarded.
		if _, err := tx.ExecContext(ctx, `UPDATE review_session SET current_id=NULL WHERE singleton=1 AND current_id IN
		 (SELECT p.id FROM presentations p WHERE p.graded=0 AND p.answer=''
		  AND NOT EXISTS(SELECT 1 FROM concept_quizzes cq WHERE cq.quiz_id=p.quiz_id AND cq.concept_id=? AND cq.role='assesses'))`, conceptID); err != nil {
			return "", err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO evidence(id,kind,concept_id,created_at) VALUES(?,?,?,?)`, newID(), "practice", conceptID, now)
		return conceptID, err
	})
}

func (s *Store) RequestQuestions(ctx context.Context, conceptID, operationID string) error {
	return s.onceOperation(ctx, operationID, "request-questions", conceptID, func(tx *sql.Tx, now int64) (string, error) {
		c, err := activeConcept(ctx, tx, conceptID)
		if err != nil {
			return "", err
		}
		sourceID, revision, err := conceptSource(ctx, tx, c)
		if err != nil {
			return "", err
		}
		return conceptID, enqueue(ctx, tx, sourceID, revision, "questions", map[string]string{"concept_id": conceptID}, now)
	})
}

func (s *Store) RequestFix(ctx context.Context, quizID, instruction, operationID string) error {
	if err := validText("what to fix", instruction, 1000, true); err != nil {
		return err
	}
	return s.onceOperation(ctx, operationID, "request-fix", struct{ Quiz, Instruction string }{quizID, instruction}, func(tx *sql.Tx, now int64) (string, error) {
		q, err := quiz(ctx, tx, quizID)
		if err != nil {
			return "", err
		}
		if q.Archived {
			return "", fmt.Errorf("%w: this question is archived", ErrConflict)
		}
		var revision int
		if err = tx.QueryRowContext(ctx, "SELECT revision FROM sources WHERE id=?", q.SourceID).Scan(&revision); err != nil {
			return "", err
		}
		// Asking again is the explicit retry of this question's paused fix,
		// as Try again is for a capture: the paused request is canceled first.
		if _, err = tx.ExecContext(ctx, `UPDATE jobs SET status='canceled',lease_token='',lease_until=0,updated_at=?
		 WHERE kind='fix' AND status='paused' AND json_extract(payload,'$.quiz_id')=?`, now, quizID); err != nil {
			return "", err
		}
		payload := struct {
			QuizID      string `json:"quiz_id"`
			Version     int    `json:"version"`
			Instruction string `json:"instruction"`
		}{quizID, q.Version, instruction}
		return quizID, enqueue(ctx, tx, q.SourceID, revision, "fix", payload, now)
	})
}

func (s *Store) ArchiveConcept(ctx context.Context, conceptID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = activeConcept(ctx, tx, conceptID); err != nil {
		return err
	}
	now := s.now()
	if _, err = tx.ExecContext(ctx, "UPDATE concepts SET status='archived',updated_at=? WHERE id=?", now, conceptID); err != nil {
		return err
	}
	quizIDs, err := orderedIDs(ctx, tx, `SELECT quiz_id FROM concept_quizzes WHERE concept_id=? AND role='assesses'`, conceptID)
	if err != nil {
		return err
	}
	for _, qid := range quizIDs {
		if _, err = tx.ExecContext(ctx, "UPDATE quizzes SET archived=1 WHERE id=?", qid); err != nil {
			return err
		}
		if err = retireUnanswered(ctx, tx, qid, ""); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM concept_index WHERE ref=?", conceptID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO evidence(id,kind,concept_id,created_at) VALUES(?,?,?,?)`, newID(), "dismiss", conceptID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateGoal(ctx context.Context, goalID, action string) error {
	var assignment string
	switch action {
	case "pause":
		assignment = "status='paused'"
	case "resume":
		assignment = "status='active'"
	case "focus":
		assignment = "focus=1"
	case "unfocus":
		assignment = "focus=0"
	default:
		return fmt.Errorf("%w: unknown goal action", ErrInvalid)
	}
	result, err := s.db.ExecContext(ctx, "UPDATE goals SET "+assignment+",updated_at=? WHERE id=? AND status<>'archived'", s.now(), goalID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed == 0 {
		if err == nil {
			err = fmt.Errorf("%w: goal", ErrNotFound)
		}
		return err
	}
	return nil
}

func (s *Store) Preferences(ctx context.Context) (Preferences, error) {
	var p Preferences
	err := s.db.QueryRowContext(ctx, "SELECT pace FROM preferences WHERE singleton=1").Scan(&p.Pace)
	return p, err
}

func (s *Store) SetPace(ctx context.Context, pace string) error {
	if pace != "light" && pace != "steady" && pace != "intense" {
		return fmt.Errorf("%w: choose light, steady, or intense", ErrInvalid)
	}
	_, err := s.db.ExecContext(ctx, "UPDATE preferences SET pace=?,updated_at=? WHERE singleton=1", pace, s.now())
	return err
}

func (s *Store) CaptureImage(ctx context.Context, sourceID string) (*CaptureImage, error) {
	var image CaptureImage
	err := s.db.QueryRowContext(ctx, "SELECT mime,bytes FROM capture_images WHERE source_id=?", sourceID).Scan(&image.MIME, &image.Bytes)
	if err != nil {
		return nil, notFound(err, "photo")
	}
	return &image, nil
}

func conceptContext(ctx context.Context, tx *sql.Tx, id string, relations []relation, names map[string]conceptRow) (ConceptContext, error) {
	c, ok := names[id]
	if !ok {
		return ConceptContext{}, fmt.Errorf("%w: concept", ErrNotFound)
	}
	cc := ConceptContext{ID: id, Name: c.Name, Summary: c.Description, Requires: []string{}, ExistingPrompts: []string{}}
	note, err := currentNote(ctx, tx, id)
	if err != nil {
		return cc, err
	}
	cc.Note = note
	for _, r := range relations {
		if r.kind == "requires" && r.from == id {
			if other, ok := names[r.to]; ok {
				cc.Requires = append(cc.Requires, other.Name)
			}
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT json_extract(v.content,'$.prompt') FROM concept_quizzes cq JOIN quizzes q ON q.id=cq.quiz_id
	 JOIN quiz_versions v ON v.quiz_id=q.id AND v.version=q.version WHERE cq.concept_id=? AND q.archived=0 ORDER BY q.created_at`, id)
	if err != nil {
		return cc, err
	}
	defer rows.Close()
	for rows.Next() {
		var prompt string
		if err = rows.Scan(&prompt); err != nil {
			return cc, err
		}
		cc.ExistingPrompts = append(cc.ExistingPrompts, prompt)
	}
	return cc, rows.Err()
}

// JobContext returns the kind-specific input for one generation job.
func (s *Store) JobContext(ctx context.Context, jobID string) (JobContext, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return JobContext{}, err
	}
	defer tx.Rollback()
	j, err := job(ctx, tx, jobID)
	if err != nil {
		return JobContext{}, err
	}
	now := s.now()
	result := JobContext{Concepts: []ConceptContext{}}
	if result.Documents, err = sourceDocuments(ctx, tx, j.SourceID); err != nil {
		return result, err
	}
	var goal Goal
	err = tx.QueryRowContext(ctx, "SELECT id,title,source_id,status,focus,created_at FROM goals WHERE source_id=?", j.SourceID).
		Scan(&goal.ID, &goal.Title, &goal.SourceID, &goal.Status, &goal.Focus, &goal.CreatedAt)
	if err == nil {
		result.Goal = &goal
	} else if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	names, err := loadConcepts(ctx, tx)
	if err != nil {
		return result, err
	}
	relations, err := liveRelations(ctx, tx)
	if err != nil {
		return result, err
	}
	var payload struct {
		ConceptID   string   `json:"concept_id"`
		Concepts    []string `json:"concepts"`
		QuizID      string   `json:"quiz_id"`
		Instruction string   `json:"instruction"`
	}
	if err = json.Unmarshal([]byte(j.Payload), &payload); err != nil {
		return result, err
	}
	addConcept := func(id string) error {
		cc, err := conceptContext(ctx, tx, id, relations, names)
		if err == nil {
			result.Concepts = append(result.Concepts, cc)
		}
		return err
	}
	switch j.Kind {
	case "transcribe":
		var image CaptureImage
		if err = tx.QueryRowContext(ctx, "SELECT mime,bytes FROM capture_images WHERE source_id=?", j.SourceID).Scan(&image.MIME, &image.Bytes); err != nil {
			return result, notFound(err, "photo")
		}
		result.Image = &image
	case "plan":
		text := j.SourceText
		for _, d := range result.Documents {
			if d.Kind == "page" || d.Kind == "transcript" {
				text += "\n" + d.Title + "\n" + d.Text
			}
		}
		if len(text) > 4000 {
			text = text[:4000]
		}
		if result.ExistingConcepts, err = searchConcepts(ctx, tx, text, 12, now); err != nil {
			return result, err
		}
	case "questions":
		if payload.ConceptID != "" {
			if err = addConcept(payload.ConceptID); err != nil {
				return result, err
			}
			break
		}
		if result.Goal == nil {
			break
		}
		ids, err := orderedIDs(ctx, tx, `SELECT gc.concept_id FROM goal_concepts gc JOIN concepts c ON c.id=gc.concept_id WHERE gc.goal_id=? AND c.status='active' ORDER BY gc.created_at,gc.rowid`, result.Goal.ID)
		if err != nil {
			return result, err
		}
		ordered := prerequisiteOrder(ids, names, relations)
		var missing []string
		for _, id := range ordered {
			var questions int
			if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM concept_quizzes cq JOIN quizzes q ON q.id=cq.quiz_id WHERE cq.concept_id=? AND cq.role='assesses' AND q.archived=0`, id).Scan(&questions); err != nil {
				return result, err
			}
			if questions == 0 {
				missing = append(missing, id)
			}
		}
		if len(missing) == 0 {
			missing = ordered
		}
		for _, id := range missing {
			if err = addConcept(id); err != nil {
				return result, err
			}
		}
	case "contrast":
		for _, id := range payload.Concepts {
			if err = addConcept(id); err != nil {
				return result, err
			}
		}
	case "fix":
		q, err := quiz(ctx, tx, payload.QuizID)
		if err != nil {
			return result, err
		}
		result.Quiz, result.Instruction = &q, payload.Instruction
		if q.ConceptID != "" {
			if err = addConcept(q.ConceptID); err != nil {
				return result, err
			}
		}
	}
	return result, tx.Commit()
}
