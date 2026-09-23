package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/misty-step/scry/internal/learning"
)

// material is everything a job's output may quote. Source-basis content must
// quote the learner's own material (pasted text, a fetched page, or a photo
// transcript); web-basis content must quote a stored search result it cites.
type material struct {
	Kind      string // topic | source
	Mode      string
	Text      string
	Documents []SourceDocument
}

func loadMaterial(ctx context.Context, tx *sql.Tx, sourceID string, revision int) (material, error) {
	var m material
	err := tx.QueryRowContext(ctx, `SELECT r.text,r.kind,src.mode FROM source_revisions r JOIN sources src ON src.id=r.source_id
	 WHERE r.source_id=? AND r.revision=?`, sourceID, revision).Scan(&m.Text, &m.Kind, &m.Mode)
	if err != nil {
		return m, notFound(err, "source revision")
	}
	m.Documents, err = sourceDocuments(ctx, tx, sourceID)
	return m, err
}

func sourceDocuments(ctx context.Context, tx *sql.Tx, sourceID string) ([]SourceDocument, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,source_id,job_id,kind,position,url,title,published,text,provider,created_at
	 FROM source_documents WHERE source_id=? ORDER BY created_at,position`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	documents := []SourceDocument{}
	for rows.Next() {
		var d SourceDocument
		if err = rows.Scan(&d.ID, &d.SourceID, &d.JobID, &d.Kind, &d.Position, &d.URL, &d.Title, &d.Published, &d.Text, &d.Provider, &d.CreatedAt); err != nil {
			return nil, err
		}
		documents = append(documents, d)
	}
	return documents, rows.Err()
}

// ownTexts are the texts that count as the learner's own material.
func (m material) ownTexts() []string {
	var texts []string
	if m.Kind == "source" && (m.Mode == "text" || m.Mode == "") {
		texts = append(texts, m.Text)
	}
	for _, d := range m.Documents {
		if d.Kind == "page" || d.Kind == "transcript" {
			texts = append(texts, d.Text)
		}
	}
	return texts
}

func (m material) webDocument(id string) (SourceDocument, bool) {
	for _, d := range m.Documents {
		if d.ID == id && d.Kind == "search_result" {
			return d, true
		}
	}
	return SourceDocument{}, false
}

func (m material) hasWeb() bool {
	for _, d := range m.Documents {
		if d.Kind == "search_result" {
			return true
		}
	}
	return false
}

func containsAny(texts []string, quote string) bool {
	for _, text := range texts {
		if strings.Contains(text, quote) {
			return true
		}
	}
	return false
}

// checkBasis enforces honest provenance for one piece of generated content:
// the learner's material is quoted exactly, web knowledge quotes and cites a
// stored search result, and general knowledge claims no evidence at all.
// It returns the citations normalized from stored documents.
func checkBasis(m material, basis string, evidence []string, citations []Citation) ([]Citation, error) {
	switch basis {
	case "source":
		if m.Kind != "source" {
			return nil, fmt.Errorf("%w: only your own material can be cited as your source", ErrInvalid)
		}
		if len(evidence) == 0 || len(citations) != 0 {
			return nil, fmt.Errorf("%w: material-based content needs an exact quotation and no web citations", ErrInvalid)
		}
		for _, quote := range evidence {
			if strings.TrimSpace(quote) == "" || !containsAny(m.ownTexts(), quote) {
				return nil, fmt.Errorf("%w: material evidence must be an exact quotation from the saved input", ErrInvalid)
			}
		}
		return nil, nil
	case "web":
		if m.Kind != "topic" || !m.hasWeb() {
			return nil, fmt.Errorf("%w: web-based content needs stored web results for this topic", ErrInvalid)
		}
		if len(evidence) == 0 || len(citations) == 0 {
			return nil, fmt.Errorf("%w: web-based content needs an exact quotation and a citation", ErrInvalid)
		}
		cited := make([]string, 0, len(citations))
		normalized := make([]Citation, 0, len(citations))
		seen := map[string]bool{}
		for _, citation := range citations {
			document, ok := m.webDocument(citation.DocumentID)
			if !ok {
				return nil, fmt.Errorf("%w: a citation does not name a stored web result", ErrInvalid)
			}
			if seen[document.ID] {
				continue
			}
			seen[document.ID] = true
			cited = append(cited, document.Text)
			normalized = append(normalized, Citation{DocumentID: document.ID, Title: document.Title, URL: document.URL})
		}
		for _, quote := range evidence {
			if strings.TrimSpace(quote) == "" || !containsAny(cited, quote) {
				return nil, fmt.Errorf("%w: web evidence must be an exact quotation from a cited result", ErrInvalid)
			}
		}
		return normalized, nil
	case "topic":
		if m.Kind != "topic" {
			return nil, fmt.Errorf("%w: content from your material must quote it", ErrInvalid)
		}
		if len(evidence) != 0 || len(citations) != 0 {
			return nil, fmt.Errorf("%w: general knowledge must not claim evidence or citations", ErrInvalid)
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: unknown content basis", ErrInvalid)
	}
}

func quizEvidence(q GeneratedQuiz) []string {
	if q.Evidence == "" {
		return nil
	}
	return []string{q.Evidence}
}

// validateQuiz checks one question's structure and provenance against its
// material. Concept fences are checked separately at publication.
func validateQuiz(q *GeneratedQuiz, m material) error {
	for _, field := range []struct {
		name, value string
		max         int
		required    bool
	}{
		{"prompt", q.Prompt, 4096, true}, {"answer", q.Answer, 1024, true}, {"explanation", q.Explanation, 8192, true}, {"evidence", q.Evidence, 8192, false},
	} {
		if err := validText(field.name, field.value, field.max, field.required); err != nil {
			return err
		}
	}
	citations, err := checkBasis(m, q.Basis, quizEvidence(*q), q.Citations)
	if err != nil {
		return err
	}
	q.Citations = citations
	if len(q.Variants) > 16 {
		return fmt.Errorf("%w: at most 16 explicit answer variants", ErrInvalid)
	}
	seen := map[string]bool{}
	for _, variant := range q.Variants {
		if err := validText("answer variant", variant, 1024, true); err != nil {
			return err
		}
		key := strings.TrimSpace(variant)
		if key == strings.TrimSpace(q.Answer) || seen[key] {
			return fmt.Errorf("%w: answer variants must be distinct", ErrInvalid)
		}
		seen[key] = true
	}
	if err := validateGrading(*q); err != nil {
		return err
	}
	switch q.Level {
	case "", "recognize", "recall", "explain", "apply":
	default:
		return fmt.Errorf("%w: question level must be recognize, recall, explain, or apply", ErrInvalid)
	}
	switch q.Kind {
	case "choice":
		if len(q.Choices) < 2 || len(q.Choices) > 6 || len(q.Variants) != 0 {
			return fmt.Errorf("%w: a choice quiz needs 2–6 choices and no typed variants", ErrInvalid)
		}
		if q.AnswerForm != "" {
			return fmt.Errorf("%w: choice questions have no typed answer form", ErrInvalid)
		}
		seen = map[string]bool{}
		matches := 0
		for _, choice := range q.Choices {
			if err := validText("choice", choice, 1024, true); err != nil {
				return err
			}
			key := strings.TrimSpace(choice)
			if seen[key] {
				return fmt.Errorf("%w: choices must be distinct", ErrInvalid)
			}
			seen[key] = true
			if choice == q.Answer {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("%w: answer must exactly equal one displayed choice", ErrInvalid)
		}
	case "recall":
		if len(q.Choices) != 0 {
			return fmt.Errorf("%w: recall quizzes cannot contain choices", ErrInvalid)
		}
		switch q.AnswerForm {
		case "", "exact", "flexible":
		default:
			return fmt.Errorf("%w: recall answer form must be exact or flexible", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: quiz kind must be choice or recall", ErrInvalid)
	}
	return nil
}

// validateGrading enforces the structural contract only. The author's explicit
// grading mode is the task contract: a semantic explanation may legitimately
// contain digits or symbols, and a letter-only identifier may need exactness,
// so no heuristic on the answer text overrides the authored mode.
func validateGrading(q GeneratedQuiz) error {
	switch q.Grading {
	case "", "exact":
		if q.Rubric != nil {
			return fmt.Errorf("%w: exact grading cannot carry a semantic rubric", ErrInvalid)
		}
		return nil
	case "semantic":
	default:
		return fmt.Errorf("%w: grading must be exact or semantic", ErrInvalid)
	}
	if q.Kind != "recall" || q.Rubric == nil {
		return fmt.Errorf("%w: semantic grading requires a recall quiz and rubric", ErrInvalid)
	}
	if len(q.Rubric.Required) < 1 || len(q.Rubric.Required) > 6 || len(q.Rubric.Contradictions) > 6 {
		return fmt.Errorf("%w: a semantic rubric needs 1–6 required ideas and at most 6 contradictions", ErrInvalid)
	}
	for _, idea := range q.Rubric.Required {
		if err := validText("required idea", idea.Text, 1024, true); err != nil {
			return err
		}
		if err := validText("missing-idea cue", idea.Cue, 800, false); err != nil {
			return err
		}
		if utf8.RuneCountInString(idea.Cue) > 200 {
			return fmt.Errorf("%w: missing-idea cue must be at most 200 characters", ErrInvalid)
		}
		if idea.Cue != "" && (strings.Contains(idea.Cue, strings.TrimSpace(q.Answer)) || strings.Contains(idea.Cue, strings.TrimSpace(idea.Text))) {
			return fmt.Errorf("%w: a missing-idea cue must not contain the expected answer or required idea verbatim", ErrInvalid)
		}
	}
	for _, claim := range q.Rubric.Contradictions {
		if err := validText("contradiction", claim.Text, 1024, true); err != nil {
			return err
		}
		if err := validText("contradiction feedback", claim.Feedback, 1600, false); err != nil {
			return err
		}
		if utf8.RuneCountInString(claim.Feedback) > 400 {
			return fmt.Errorf("%w: contradiction feedback must be at most 400 characters", ErrInvalid)
		}
	}
	return nil
}

func validateNote(n *NoteContent, m material) error {
	if n == nil {
		return fmt.Errorf("%w: a new concept needs a note", ErrInvalid)
	}
	if err := validText("note title", n.Title, 200, true); err != nil {
		return err
	}
	if err := validText("note", n.Body, 2400, true); err != nil {
		return err
	}
	if len(strings.TrimSpace(n.Body)) < 40 {
		return fmt.Errorf("%w: a note must explain the idea, not only name it", ErrInvalid)
	}
	if len(n.Evidence) > 6 {
		return fmt.Errorf("%w: at most 6 quotations per note", ErrInvalid)
	}
	for _, quote := range n.Evidence {
		if err := validText("note quotation", quote, 1024, true); err != nil {
			return err
		}
	}
	citations, err := checkBasis(m, n.Basis, n.Evidence, n.Citations)
	if err != nil {
		return err
	}
	n.Citations = citations
	if n.Evidence == nil {
		n.Evidence = []string{}
	}
	if n.Citations == nil {
		n.Citations = []Citation{}
	}
	return nil
}

// quizTargets returns the concepts a quizzes-producing job may assess.
func quizTargets(ctx context.Context, tx *sql.Tx, j Job) (map[string]bool, error) {
	targets := map[string]bool{}
	var payload struct {
		ConceptID string `json:"concept_id"`
		QuizID    string `json:"quiz_id"`
	}
	if err := json.Unmarshal([]byte(j.Payload), &payload); err != nil {
		return nil, err
	}
	// Only active concepts may receive new questions: a concept archived while
	// its job was generating must not come back through late output.
	active := func(id string) (bool, error) {
		var ok bool
		err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM concepts WHERE id=? AND status='active' AND origin<>'foundation')", id).Scan(&ok)
		return ok, err
	}
	switch j.Kind {
	case "questions":
		if payload.ConceptID != "" {
			ok, err := active(payload.ConceptID)
			if ok {
				targets[payload.ConceptID] = true
			}
			return targets, err
		}
		rows, err := tx.QueryContext(ctx, `SELECT gc.concept_id FROM goal_concepts gc JOIN goals g ON g.id=gc.goal_id JOIN concepts c ON c.id=gc.concept_id
		 WHERE g.source_id=? AND c.status='active'`, j.SourceID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				return nil, err
			}
			targets[id] = true
		}
		return targets, rows.Err()
	case "fix":
		var concept string
		err := tx.QueryRowContext(ctx, "SELECT COALESCE((SELECT concept_id FROM concept_quizzes WHERE quiz_id=? AND role='assesses'),'')", payload.QuizID).Scan(&concept)
		targets[concept] = true
		return targets, err
	default: // legacy quizzes: no concept links
		targets[""] = true
		return targets, nil
	}
}

// validateQuizBatch checks every question of a quizzes-producing job against
// its material and concept targets. It normalizes citations in place.
func validateQuizBatch(ctx context.Context, tx *sql.Tx, j Job, result *GenerationResult) error {
	if result.Plan != nil || len(result.Documents) != 0 {
		return fmt.Errorf("%w: unexpected content for a questions job", ErrInvalid)
	}
	limit := MaxGeneratedQuizzes
	if j.Kind == "fix" {
		limit = 1
	}
	if len(result.Quizzes) == 0 || len(result.Quizzes) > limit {
		return fmt.Errorf("%w: generation must contain 1–%d complete questions", ErrInvalid, limit)
	}
	m, err := loadMaterial(ctx, tx, j.SourceID, j.SourceRevision)
	if err != nil {
		return err
	}
	targets, err := quizTargets(ctx, tx, j)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(result.Quizzes))
	for i := range result.Quizzes {
		q := &result.Quizzes[i]
		if err := validateQuiz(q, m); err != nil {
			return fmt.Errorf("quiz %d: %w", i+1, err)
		}
		if !targets[q.Concept] {
			return fmt.Errorf("%w: quiz %d assesses a concept outside this request", ErrInvalid, i+1)
		}
		key := strings.TrimSpace(q.Prompt)
		if seen[key] {
			return fmt.Errorf("%w: duplicate generated prompt", ErrInvalid)
		}
		seen[key] = true
	}
	return nil
}

func validateDocuments(j Job, docs []DocumentContent) error {
	want, count := "", len(docs)
	switch {
	case j.Kind == "transcribe":
		want = "transcript"
		if count != 1 {
			return fmt.Errorf("%w: the photo could not be read; try a clearer photo", ErrInvalid)
		}
	case j.SourceMode == "link":
		want = "page"
		if count != 1 {
			return fmt.Errorf("%w: that page could not be read; check the link or paste the text instead", ErrInvalid)
		}
	default:
		want = "search_result"
		if count > 12 {
			return fmt.Errorf("%w: too many web results", ErrInvalid)
		}
	}
	for _, d := range docs {
		if d.Kind != want {
			return fmt.Errorf("%w: unexpected document kind", ErrInvalid)
		}
		if err := validText("document text", d.Text, 256<<10, true); err != nil {
			return err
		}
		if validText("document title", d.Title, 500, false) != nil || validText("document date", d.Published, 64, false) != nil || validText("document provider", d.Provider, 40, true) != nil {
			return fmt.Errorf("%w: document metadata is invalid", ErrInvalid)
		}
		if want != "transcript" {
			u, err := url.Parse(d.URL)
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || len(d.URL) > 2048 {
				return fmt.Errorf("%w: a document has an invalid address", ErrInvalid)
			}
		} else if d.URL != "" {
			return fmt.Errorf("%w: a transcript has no address", ErrInvalid)
		}
	}
	return nil
}

func validatePlan(ctx context.Context, tx *sql.Tx, p *PlanContent, m material) error {
	if p == nil {
		return fmt.Errorf("%w: missing study plan", ErrInvalid)
	}
	if err := validText("goal", p.Goal, 480, true); err != nil || utf8.RuneCountInString(p.Goal) > 120 {
		return fmt.Errorf("%w: the goal title must be short", ErrInvalid)
	}
	if len(p.Concepts) == 0 || len(p.Concepts) > MaxPlanConcepts {
		return fmt.Errorf("%w: a plan needs 1–%d concepts", ErrInvalid, MaxPlanConcepts)
	}
	keys := map[string]bool{}
	for i := range p.Concepts {
		c := &p.Concepts[i]
		if err := validText("concept key", c.Key, 40, true); err != nil || strings.ContainsAny(c.Key, " \t\n") {
			return fmt.Errorf("%w: concept keys must be short identifiers", ErrInvalid)
		}
		if keys[c.Key] {
			return fmt.Errorf("%w: duplicate concept key", ErrInvalid)
		}
		keys[c.Key] = true
		if err := validText("concept name", c.Name, 120, true); err != nil {
			return err
		}
		if err := validText("concept summary", c.Summary, 600, true); err != nil {
			return err
		}
		if c.ExistingID != "" {
			var reusable bool
			if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM concepts WHERE id=? AND status='active' AND origin IN ('generated','learner'))", c.ExistingID).Scan(&reusable); err != nil {
				return err
			}
			if !reusable {
				return fmt.Errorf("%w: a plan reuses an unknown concept", ErrInvalid)
			}
			if c.Note != nil {
				if err := validateNote(c.Note, m); err != nil {
					return err
				}
			}
			continue
		}
		if err := validateNote(c.Note, m); err != nil {
			return fmt.Errorf("concept %q: %w", c.Name, err)
		}
	}
	for _, c := range p.Concepts {
		for _, ref := range append(append(append([]string{}, c.Requires...), c.PartOf...), c.ConfusedWith...) {
			if ref == c.Key || ref == c.ExistingID {
				return fmt.Errorf("%w: a concept cannot relate to itself", ErrInvalid)
			}
			if keys[ref] {
				continue
			}
			var active bool
			if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM concepts WHERE id=? AND status='active' AND origin IN ('generated','learner'))", ref).Scan(&active); err != nil {
				return err
			}
			if !active {
				return fmt.Errorf("%w: a relation names an unknown concept", ErrInvalid)
			}
		}
	}
	return nil
}

// publish validates and writes one job result inside a savepoint, so invalid
// output never leaves partial rows behind the recorded failure.
func publish(ctx context.Context, tx *sql.Tx, j Job, result GenerationResult, now int64) (int, error) {
	if _, err := tx.ExecContext(ctx, "SAVEPOINT publish"); err != nil {
		return 0, err
	}
	count, err := publishKind(ctx, tx, j, result, now)
	if err != nil {
		if _, rollbackErr := tx.ExecContext(ctx, "ROLLBACK TO publish"); rollbackErr != nil {
			return 0, errors.Join(err, rollbackErr)
		}
	}
	if _, releaseErr := tx.ExecContext(ctx, "RELEASE publish"); releaseErr != nil {
		return 0, errors.Join(err, releaseErr)
	}
	return count, err
}

func publishKind(ctx context.Context, tx *sql.Tx, j Job, result GenerationResult, now int64) (int, error) {
	switch j.Kind {
	case "research", "transcribe":
		if len(result.Quizzes) != 0 || result.Plan != nil {
			return 0, fmt.Errorf("%w: unexpected content for a reading step", ErrInvalid)
		}
		if err := validateDocuments(j, result.Documents); err != nil {
			return 0, err
		}
		for position, d := range result.Documents {
			_, err := tx.ExecContext(ctx, `INSERT INTO source_documents(id,source_id,job_id,kind,position,url,title,published,text,provider,created_at)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?)`, newID(), j.SourceID, j.ID, d.Kind, position, d.URL, d.Title, d.Published, d.Text, d.Provider, now)
			if err != nil {
				return 0, err
			}
		}
		return len(result.Documents), nil
	case "plan":
		if len(result.Quizzes) != 0 || len(result.Documents) != 0 {
			return 0, fmt.Errorf("%w: unexpected content for a plan", ErrInvalid)
		}
		m, err := loadMaterial(ctx, tx, j.SourceID, j.SourceRevision)
		if err != nil {
			return 0, err
		}
		if result.Plan == nil {
			return 0, fmt.Errorf("%w: missing study plan", ErrInvalid)
		}
		plan := *result.Plan
		if err = validatePlan(ctx, tx, &plan, m); err != nil {
			return 0, err
		}
		return publishPlan(ctx, tx, j, plan, result, now)
	default:
		if !quizKind(j.Kind) {
			return 0, fmt.Errorf("%w: unknown job kind", ErrInvalid)
		}
		if err := validateQuizBatch(ctx, tx, j, &result); err != nil {
			return 0, err
		}
		if j.Kind == "fix" {
			return publishFix(ctx, tx, j, result, now)
		}
		return publishQuizzes(ctx, tx, j, result, now)
	}
}

func ensureGoal(ctx context.Context, tx *sql.Tx, sourceID, title string, now int64) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, "SELECT id FROM goals WHERE source_id=?", sourceID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	id = newID()
	_, err = tx.ExecContext(ctx, "INSERT INTO goals(id,title,source_id,status,focus,created_at,updated_at) VALUES(?,?,?,'active',0,?,?)", id, title, sourceID, now, now)
	return id, err
}

func insertNote(ctx context.Context, tx *sql.Tx, conceptID, sourceID, jobID string, n NoteContent, model, promptVersion string, now int64) (string, error) {
	var supersedes string
	err := tx.QueryRowContext(ctx, `SELECT id FROM notes WHERE concept_id=? ORDER BY created_at DESC,rowid DESC LIMIT 1`, conceptID).Scan(&supersedes)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	evidence, err := marshal(n.Evidence)
	if err != nil {
		return "", err
	}
	citations, err := marshal(n.Citations)
	if err != nil {
		return "", err
	}
	id := newID()
	_, err = tx.ExecContext(ctx, `INSERT INTO notes(id,concept_id,title,body,basis,evidence,citations,source_id,job_id,model,prompt_version,supersedes,created_at)
	 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, conceptID, n.Title, n.Body, n.Basis, evidence, citations, sourceID, jobID, model, promptVersion, supersedes, now)
	if err != nil {
		return "", err
	}
	return id, nil
}

// requiresPath reports whether from already reaches to through live requires
// relations. A new requires edge that would close a cycle is skipped.
func requiresPath(ctx context.Context, tx *sql.Tx, from, to string) (bool, error) {
	var found bool
	err := tx.QueryRowContext(ctx, `WITH RECURSIVE reach(id) AS (SELECT ? UNION SELECT r.to_id FROM concept_relations r JOIN reach ON r.from_id=reach.id
	 WHERE r.kind='requires' AND r.retired_at=0) SELECT EXISTS(SELECT 1 FROM reach WHERE id=?)`, from, to).Scan(&found)
	return found, err
}

func addRelation(ctx context.Context, tx *sql.Tx, from, to, kind, origin, jobID string, now int64) error {
	if from == to || from == "" || to == "" {
		return nil
	}
	if kind == "requires" {
		cycle, err := requiresPath(ctx, tx, to, from)
		if err != nil || cycle {
			return err
		}
	}
	if kind == "confused_with" {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM concept_relations WHERE kind='confused_with' AND retired_at=0 AND from_id=? AND to_id=?)`, to, from).Scan(&exists); err != nil || exists {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO concept_relations(id,from_id,to_id,kind,origin,job_id,created_at) VALUES(?,?,?,?,?,?,?)`,
		newID(), from, to, kind, origin, jobID, now)
	return err
}

func publishPlan(ctx context.Context, tx *sql.Tx, j Job, plan PlanContent, result GenerationResult, now int64) (int, error) {
	goalID, err := ensureGoal(ctx, tx, j.SourceID, plan.Goal, now)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE goals SET title=?,updated_at=? WHERE id=?", plan.Goal, now, goalID); err != nil {
		return 0, err
	}
	ids := make(map[string]string, len(plan.Concepts))
	origin := "model"
	if j.SourceKind == "source" {
		origin = "source"
	}
	for _, c := range plan.Concepts {
		id := c.ExistingID
		if id == "" {
			id = newID()
			_, err = tx.ExecContext(ctx, `INSERT INTO concepts(id,name,description,created_at,origin,status,source_id,updated_at) VALUES(?,?,?,?,'generated','active',?,?)`,
				id, c.Name, c.Summary, now, j.SourceID, now)
			if err != nil {
				return 0, err
			}
			if err = indexConcept(ctx, tx, id, c.Name, c.Summary); err != nil {
				return 0, err
			}
		}
		ids[c.Key] = id
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO goal_concepts(goal_id,concept_id,created_at) VALUES(?,?,?)", goalID, id, now); err != nil {
			return 0, err
		}
		if c.Note != nil {
			var hasStandard bool
			if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM notes WHERE concept_id=?)", id).Scan(&hasStandard); err != nil {
				return 0, err
			}
			if !hasStandard {
				if _, err = insertNote(ctx, tx, id, j.SourceID, j.ID, *c.Note, result.Model, result.PromptVersion, now); err != nil {
					return 0, err
				}
			}
		}
	}
	resolve := func(ref string) string {
		if id, ok := ids[ref]; ok {
			return id
		}
		return ref
	}
	for _, c := range plan.Concepts {
		from := ids[c.Key]
		for _, ref := range c.Requires {
			if err = addRelation(ctx, tx, from, resolve(ref), "requires", origin, j.ID, now); err != nil {
				return 0, err
			}
		}
		for _, ref := range c.PartOf {
			if err = addRelation(ctx, tx, from, resolve(ref), "part_of", origin, j.ID, now); err != nil {
				return 0, err
			}
		}
		for _, ref := range c.ConfusedWith {
			if err = addRelation(ctx, tx, from, resolve(ref), "confused_with", origin, j.ID, now); err != nil {
				return 0, err
			}
		}
	}
	return len(plan.Concepts), nil
}

func publishQuizzes(ctx context.Context, tx *sql.Tx, j Job, result GenerationResult, now int64) (int, error) {
	for index, content := range result.Quizzes {
		originIndex := index
		if j.Candidates != nil {
			for originalIndex, candidate := range j.Candidates.Result.Quizzes {
				if candidate.Prompt == content.Prompt {
					originIndex = originalIndex
					break
				}
			}
		}
		id := newID()
		encoded, err := marshal(content)
		if err != nil {
			return 0, err
		}
		card, err := marshal(learning.NewCard(time.UnixMilli(now)))
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO quizzes(id,source_id,version,created_at,origin_job_id,origin_index) VALUES(?,?,1,?,?,?)", id, j.SourceID, now, j.ID, originIndex); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO quiz_versions(quiz_id,version,content,model,prompt_version,created_at) VALUES(?,1,?,?,?,?)", id, encoded, result.Model, result.PromptVersion, now); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO schedules(quiz_id,version,card,due_at,algorithm) VALUES(?,1,?,?,?)", id, card, now, learning.Algorithm); err != nil {
			return 0, err
		}
		if content.Concept != "" {
			if _, err = tx.ExecContext(ctx, "INSERT INTO concept_quizzes(concept_id,quiz_id,created_at,role) VALUES(?,?,?,'assesses')", content.Concept, id, now); err != nil {
				return 0, err
			}
		}
	}
	return len(result.Quizzes), nil
}

// publishFix saves a fix job's corrected question as a proposal. The live
// question is unchanged until the learner accepts it (DecideProposal); a newer
// suggestion supersedes an undecided older one.
func publishFix(ctx context.Context, tx *sql.Tx, j Job, result GenerationResult, now int64) (int, error) {
	var payload struct {
		QuizID      string `json:"quiz_id"`
		Version     int    `json:"version"`
		Instruction string `json:"instruction"`
	}
	if err := json.Unmarshal([]byte(j.Payload), &payload); err != nil {
		return 0, err
	}
	current, err := quiz(ctx, tx, payload.QuizID)
	if err != nil {
		return 0, err
	}
	if current.Archived || current.Version != payload.Version {
		return 0, fmt.Errorf("%w: the question changed before this fix was ready; nothing was suggested", ErrInvalid)
	}
	encoded, err := marshal(carryRubric(current, result.Quizzes[0]))
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE quiz_proposals SET status='superseded',decided_at=? WHERE quiz_id=? AND status='pending'", now, current.ID); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO quiz_proposals(id,quiz_id,base_version,job_id,instruction,content,model,prompt_version,created_at)
	 VALUES(?,?,?,?,?,?,?,?,?)`, newID(), current.ID, current.Version, j.ID, payload.Instruction, encoded, result.Model, result.PromptVersion, now); err != nil {
		return 0, err
	}
	return 0, nil
}
