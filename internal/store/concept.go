package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// SaveConcept persists a concept record.
func (s *Store) SaveConcept(ctx context.Context, c Concept) error {
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("%w: concept id and name are required", ErrInvalid)
	}
	createdAt := c.CreatedAt
	if createdAt <= 0 {
		createdAt = s.now()
	}
	const query = `
INSERT INTO concepts(id, name, description, created_at)
VALUES(?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
 name = excluded.name,
 description = excluded.description;
`
	_, err := s.db.ExecContext(ctx, query, c.ID, c.Name, c.Description, createdAt)
	return err
}

// GetConcept retrieves a concept with its linked references, quizzes, and prerequisites.
func (s *Store) GetConcept(ctx context.Context, id string) (*ConceptDetail, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("%w: empty concept id", ErrInvalid)
	}
	var detail ConceptDetail
	const conceptQ = `SELECT id, name, description, created_at FROM concepts WHERE id = ?`
	err := s.db.QueryRowContext(ctx, conceptQ, id).Scan(&detail.ID, &detail.Name, &detail.Description, &detail.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Linked references
	const refsQ = `
SELECT r.id, r.title, r.content, r.format, r.source_url, r.created_at
FROM concept_references cr
JOIN "references" r ON r.id = cr.reference_id
WHERE cr.concept_id = ?
ORDER BY r.created_at, r.id;
`
	refRows, err := s.db.QueryContext(ctx, refsQ, id)
	if err != nil {
		return nil, err
	}
	for refRows.Next() {
		var r Reference
		if err = refRows.Scan(&r.ID, &r.Title, &r.Content, &r.Format, &r.SourceURL, &r.CreatedAt); err != nil {
			return nil, err
		}
		detail.References = append(detail.References, r)
	}
	if err = refRows.Err(); err != nil {
		return nil, err
	}

	// Linked quizzes
	const quizQ = `
SELECT q.id, q.source_id, q.version, q.archived,
       COALESCE(s.due_at, 0)
FROM concept_quizzes cq
JOIN quizzes q ON q.id = cq.quiz_id
LEFT JOIN schedules s ON s.quiz_id = q.id
WHERE cq.concept_id = ?
ORDER BY q.created_at, q.id;
`
	qRows, err := s.db.QueryContext(ctx, quizQ, id)
	if err != nil {
		return nil, err
	}
	defer qRows.Close()
	for qRows.Next() {
		var q Quiz
		if err = qRows.Scan(&q.ID, &q.SourceID, &q.Version, &q.Archived, &q.DueAt); err != nil {
			return nil, err
		}
		detail.Quizzes = append(detail.Quizzes, q)
	}
	if err = qRows.Err(); err != nil {
		return nil, err
	}

	// Prerequisites
	const prereqQ = `
SELECT c.id, c.name, c.description, c.created_at
FROM concept_prerequisites cp
JOIN concepts c ON c.id = cp.prerequisite_id
WHERE cp.concept_id = ?
ORDER BY c.name, c.id;
`
	pRows, err := s.db.QueryContext(ctx, prereqQ, id)
	if err != nil {
		return nil, err
	}
	defer pRows.Close()
	for pRows.Next() {
		var p Concept
		if err = pRows.Scan(&p.ID, &p.Name, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		detail.Prerequisites = append(detail.Prerequisites, p)
	}
	return &detail, pRows.Err()
}

// SaveReference persists a reference and links it to the specified concepts.
func (s *Store) SaveReference(ctx context.Context, r Reference, conceptIDs []string) error {
	if strings.TrimSpace(r.ID) == "" || strings.TrimSpace(r.Title) == "" {
		return fmt.Errorf("%w: reference id and title are required", ErrInvalid)
	}
	validFormats := map[string]bool{
		"explanation": true, "diagram": true, "article": true, "video": true, "reference": true,
	}
	if !validFormats[r.Format] {
		r.Format = "explanation"
	}
	createdAt := r.CreatedAt
	if createdAt <= 0 {
		createdAt = s.now()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const refQ = `
INSERT INTO "references"(id, title, content, format, source_url, created_at)
VALUES(?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
 title = excluded.title,
 content = excluded.content,
 format = excluded.format,
 source_url = excluded.source_url;
`
	if _, err = tx.ExecContext(ctx, refQ, r.ID, r.Title, r.Content, r.Format, r.SourceURL, createdAt); err != nil {
		return err
	}

	for _, cID := range conceptIDs {
		cID = strings.TrimSpace(cID)
		if cID == "" {
			continue
		}
		const linkQ = `INSERT OR IGNORE INTO concept_references(concept_id, reference_id, created_at) VALUES(?, ?, ?);`
		if _, err = tx.ExecContext(ctx, linkQ, cID, r.ID, createdAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetReference retrieves a reference with its linked concepts.
func (s *Store) GetReference(ctx context.Context, id string) (*ReferenceDetail, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("%w: empty reference id", ErrInvalid)
	}
	var detail ReferenceDetail
	const refQ = `SELECT id, title, content, format, source_url, created_at FROM "references" WHERE id = ?`
	err := s.db.QueryRowContext(ctx, refQ, id).Scan(&detail.ID, &detail.Title, &detail.Content, &detail.Format, &detail.SourceURL, &detail.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	const conceptsQ = `
SELECT c.id, c.name, c.description, c.created_at
FROM concept_references cr
JOIN concepts c ON c.id = cr.concept_id
WHERE cr.reference_id = ?
ORDER BY c.name, c.id;
`
	cRows, err := s.db.QueryContext(ctx, conceptsQ, id)
	if err != nil {
		return nil, err
	}
	defer cRows.Close()
	for cRows.Next() {
		var c Concept
		if err = cRows.Scan(&c.ID, &c.Name, &c.Description, &c.CreatedAt); err != nil {
			return nil, err
		}
		detail.Concepts = append(detail.Concepts, c)
	}
	return &detail, cRows.Err()
}

// AddConceptPrerequisite records a prerequisite relationship between two concepts.
func (s *Store) AddConceptPrerequisite(ctx context.Context, conceptID, prerequisiteID string) error {
	conceptID = strings.TrimSpace(conceptID)
	prerequisiteID = strings.TrimSpace(prerequisiteID)
	if conceptID == "" || prerequisiteID == "" || conceptID == prerequisiteID {
		return fmt.Errorf("%w: invalid concept prerequisite pair (%s, %s)", ErrInvalid, conceptID, prerequisiteID)
	}
	const q = `INSERT OR IGNORE INTO concept_prerequisites(concept_id, prerequisite_id, created_at) VALUES(?, ?, ?);`
	_, err := s.db.ExecContext(ctx, q, conceptID, prerequisiteID, s.now())
	return err
}

// LinkQuizToConcept links a quiz to a concept.
func (s *Store) LinkQuizToConcept(ctx context.Context, quizID, conceptID string) error {
	quizID = strings.TrimSpace(quizID)
	conceptID = strings.TrimSpace(conceptID)
	if quizID == "" || conceptID == "" {
		return fmt.Errorf("%w: quiz id and concept id are required", ErrInvalid)
	}
	const q = `INSERT OR IGNORE INTO concept_quizzes(concept_id, quiz_id, created_at) VALUES(?, ?, ?);`
	_, err := s.db.ExecContext(ctx, q, conceptID, quizID, s.now())
	return err
}

// GetQuizConcepts returns the concepts linked to a quiz.
func (s *Store) GetQuizConcepts(ctx context.Context, quizID string) ([]Concept, error) {
	if strings.TrimSpace(quizID) == "" {
		return nil, fmt.Errorf("%w: empty quiz id", ErrInvalid)
	}
	const q = `
SELECT c.id, c.name, c.description, c.created_at
FROM concept_quizzes cq
JOIN concepts c ON c.id = cq.concept_id
WHERE cq.quiz_id = ?
ORDER BY c.name, c.id;
`
	rows, err := s.db.QueryContext(ctx, q, quizID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var concepts []Concept
	for rows.Next() {
		var c Concept
		if err = rows.Scan(&c.ID, &c.Name, &c.Description, &c.CreatedAt); err != nil {
			return nil, err
		}
		concepts = append(concepts, c)
	}
	return concepts, rows.Err()
}

// SearchConceptsAndReferences searches both concepts and references by text query.
// It returns matching concepts and references with their linked counterparts.
// If query matches nothing or is empty, it returns an empty SearchResult without error.
func (s *Store) SearchConceptsAndReferences(ctx context.Context, query string) (*SearchResult, error) {
	trimmed := strings.TrimSpace(query)
	result := &SearchResult{
		Concepts:   []ConceptDetail{},
		References: []ReferenceDetail{},
	}
	if trimmed == "" {
		return result, nil
	}

	likePattern := "%" + trimmed + "%"

	// Find matching concept IDs
	const searchConceptsQ = `
SELECT id FROM concepts
WHERE name LIKE ? OR description LIKE ?
ORDER BY name, id;
`
	cRows, err := s.db.QueryContext(ctx, searchConceptsQ, likePattern, likePattern)
	if err != nil {
		return nil, err
	}
	defer cRows.Close()
	var conceptIDs []string
	for cRows.Next() {
		var id string
		if err = cRows.Scan(&id); err != nil {
			return nil, err
		}
		conceptIDs = append(conceptIDs, id)
	}
	if err = cRows.Err(); err != nil {
		return nil, err
	}

	for _, id := range conceptIDs {
		detail, err := s.GetConcept(ctx, id)
		if err != nil {
			return nil, err
		}
		result.Concepts = append(result.Concepts, *detail)
	}

	// Find matching reference IDs
	const searchRefsQ = `
SELECT id FROM "references"
WHERE title LIKE ? OR content LIKE ?
ORDER BY created_at DESC, id;
`
	rRows, err := s.db.QueryContext(ctx, searchRefsQ, likePattern, likePattern)
	if err != nil {
		return nil, err
	}
	defer rRows.Close()
	var refIDs []string
	for rRows.Next() {
		var id string
		if err = rRows.Scan(&id); err != nil {
			return nil, err
		}
		refIDs = append(refIDs, id)
	}
	if err = rRows.Err(); err != nil {
		return nil, err
	}

	for _, id := range refIDs {
		detail, err := s.GetReference(ctx, id)
		if err != nil {
			return nil, err
		}
		result.References = append(result.References, *detail)
	}

	return result, nil
}
