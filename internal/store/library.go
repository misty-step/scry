package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

const maxTopicBytes = 2048

// firstJob is where each capture mode's chain starts. Only an explicit topic
// may be researched on the web; pasted text is private material and is never
// sent to search.
func firstJob(mode string) string {
	switch mode {
	case "topic", "link":
		return "research"
	case "photo":
		return "transcribe"
	default:
		return "plan"
	}
}

func imageMIME(data []byte) string {
	switch {
	case len(data) >= 3 && bytes.Equal(data[:3], []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}):
		return "image/png"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}

func normalizeLink(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || len(raw) > 2048 || strings.ContainsAny(raw, " \t\r\n") {
		return "", fmt.Errorf("%w: use a complete web address that starts with https://", ErrInvalid)
	}
	u.Fragment = ""
	return u.String(), nil
}

func goalTitle(text, mode string) string {
	switch mode {
	case "link":
		if u, err := url.Parse(text); err == nil {
			return excerptRunes(u.Host+u.Path, 120)
		}
	case "photo":
		if strings.TrimSpace(text) == "" {
			return "Photo"
		}
	}
	return excerptRunes(text, 120)
}

func excerptRunes(text string, limit int) string {
	r := []rune(strings.Join(strings.Fields(text), " "))
	if len(r) <= limit {
		return string(r)
	}
	return string(r[:limit-1]) + "…"
}

// Capture saves one explicit learner capture and starts its preparation chain
// in the same transaction. Retrying the same operation returns the same source.
func (s *Store) Capture(ctx context.Context, in CaptureInput, operationID string) (Source, error) {
	if err := validOperation(operationID); err != nil {
		return Source{}, err
	}
	text, kind := in.Text, "source"
	switch in.Mode {
	case "topic":
		kind = "topic"
		if err := validText("topic (paste longer material as your text instead)", text, maxTopicBytes, true); err != nil {
			return Source{}, err
		}
	case "text":
		if err := validText("text (split longer material into separate captures)", text, MaxSourceBytes, true); err != nil {
			return Source{}, err
		}
	case "link":
		link, err := normalizeLink(text)
		if err != nil {
			return Source{}, err
		}
		text = link
	case "photo":
		if err := validText("photo caption", text, 1024, false); err != nil {
			return Source{}, err
		}
		if len(in.Image) == 0 || len(in.Image) > MaxImageBytes {
			return Source{}, fmt.Errorf("%w: choose a photo up to 4 MiB", ErrInvalid)
		}
		if mime := imageMIME(in.Image); mime == "" || (in.ImageMIME != "" && in.ImageMIME != mime) {
			return Source{}, fmt.Errorf("%w: use a JPEG, PNG, or WebP photo", ErrInvalid)
		}
	default:
		return Source{}, fmt.Errorf("%w: choose Topic, My text, Link, or Photo", ErrInvalid)
	}
	sum := sha256.Sum256(in.Image)
	hash := payloadHash(struct{ Mode, Text, Image string }{in.Mode, text, hex.EncodeToString(sum[:])})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Source{}, err
	}
	defer tx.Rollback()
	id, _, found, err := existingOperation(ctx, tx, operationID, "capture", hash)
	if err != nil {
		return Source{}, err
	}
	if !found {
		id = newID()
		now := s.now()
		web := in.Mode == "topic"
		_, err = tx.ExecContext(ctx, "INSERT INTO sources(id,text,kind,revision,created_at,mode,web) VALUES(?,?,?,1,?,?,?)", id, text, kind, now, in.Mode, web)
		if err != nil {
			return Source{}, err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO source_revisions(source_id,revision,text,kind,created_at) VALUES(?,1,?,?,?)", id, text, kind, now)
		if err != nil {
			return Source{}, err
		}
		if in.Mode == "photo" {
			if _, err = tx.ExecContext(ctx, "INSERT INTO capture_images(source_id,mime,bytes,created_at) VALUES(?,?,?,?)", id, imageMIME(in.Image), in.Image, now); err != nil {
				return Source{}, err
			}
		}
		if _, err = ensureGoal(ctx, tx, id, goalTitle(text, in.Mode), now); err != nil {
			return Source{}, err
		}
		if err = enqueue(ctx, tx, id, 1, firstJob(in.Mode), nil, now); err != nil {
			return Source{}, err
		}
		if err = index(ctx, tx, "source", id, text, ""); err != nil {
			return Source{}, err
		}
		if err = saveOperation(ctx, tx, operationID, "capture", hash, id, nil, now); err != nil {
			return Source{}, err
		}
	}
	result, err := source(ctx, tx, id, true)
	if err != nil {
		return Source{}, err
	}
	if err = tx.Commit(); err != nil {
		return Source{}, err
	}
	return result, nil
}

// RetrySource retries the stopped step of a source's chain. Candidate batches
// resume their saved critic work; other steps get a new job of the same kind,
// at most three per kind.
func (s *Store) RetrySource(ctx context.Context, id, operationID string) (Source, error) {
	if err := validOperation(operationID); err != nil {
		return Source{}, err
	}
	hash := payloadHash(id)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Source{}, err
	}
	defer tx.Rollback()
	_, _, found, err := existingOperation(ctx, tx, operationID, "retry", hash)
	if err != nil {
		return Source{}, err
	}
	result, err := source(ctx, tx, id, true)
	if err != nil {
		return Source{}, err
	}
	if !found {
		last := result.Job
		if result.Archived || last == nil || (last.Status != "failed" && last.Status != "canceled" && last.Status != "paused") {
			return Source{}, fmt.Errorf("%w: only stopped preparation can be retried", ErrConflict)
		}
		if quizKind(last.Kind) && last.Published != 0 {
			return Source{}, fmt.Errorf("%w: these questions were already published; ask for more questions from a concept instead", ErrConflict)
		}
		now := s.now()
		if last.Candidates != nil {
			// Continue the same immutable batch. The first three attempts are
			// automatic; at most two more require deliberate manual retries.
			// Monotonic attempt numbers preserve every prior send/spend record.
			if last.Attempts >= 5 || last.CriticStatus == "judged" {
				return Source{}, fmt.Errorf("%w: saved candidate checks are exhausted or rejected; inspect and revise the input", ErrConflict)
			}
			if _, err = tx.ExecContext(ctx, `UPDATE jobs SET status='queued',error='Explicit retry of saved candidates',lease_token='',lease_until=0,available_at=?,updated_at=? WHERE id=?`, now, now, last.ID); err != nil {
				return Source{}, err
			}
		} else {
			kind := last.Kind
			if kind == "quizzes" {
				kind = "plan" // legacy single-call work retries through the v5 chain
			}
			var runs int
			if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM jobs WHERE source_id=? AND kind=? AND payload=? AND id NOT IN (SELECT job_id FROM foundation_requests)", id, kind, last.Payload).Scan(&runs); err != nil {
				return Source{}, err
			}
			if runs >= 3 {
				return Source{}, fmt.Errorf("%w: this step has been tried three times; inspect the failure before capturing a revised input", ErrConflict)
			}
			if last.Status == "paused" {
				if _, err = tx.ExecContext(ctx, "UPDATE jobs SET status='canceled',lease_token='',lease_until=0,updated_at=? WHERE id=?", now, last.ID); err != nil {
					return Source{}, err
				}
			}
			var payload any
			if last.Kind != "quizzes" {
				payload = json.RawMessage(last.Payload)
			}
			if err = enqueue(ctx, tx, id, result.Revision, kind, payload, now); err != nil {
				return Source{}, err
			}
		}
		if err = saveOperation(ctx, tx, operationID, "retry", hash, id, nil, now); err != nil {
			return Source{}, err
		}
		result, err = source(ctx, tx, id, true)
		if err != nil {
			return Source{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Source{}, err
	}
	return result, nil
}

func (s *Store) Source(ctx context.Context, id string) (Source, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Source{}, err
	}
	defer tx.Rollback()
	result, err := source(ctx, tx, id, true)
	if err != nil {
		return Source{}, err
	}
	if err = tx.Commit(); err != nil {
		return Source{}, err
	}
	return result, nil
}

// sourceJobs lists a source's jobs oldest first, excluding retired foundation work.
func sourceJobs(ctx context.Context, tx *sql.Tx, sourceID string) ([]Job, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+jobColumns+` FROM jobs j JOIN sources src ON src.id=j.source_id
	 JOIN source_revisions r ON r.source_id=j.source_id AND r.revision=j.source_revision
	 WHERE j.source_id=? AND NOT `+retiredJob+` ORDER BY j.created_at,j.rowid`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func source(ctx context.Context, tx *sql.Tx, id string, details bool) (Source, error) {
	var src Source
	err := tx.QueryRowContext(ctx, `SELECT s.id,s.text,s.kind,s.mode,s.web,s.revision,s.archived,s.created_at,COALESCE(g.id,''),
	 EXISTS(SELECT 1 FROM capture_images i WHERE i.source_id=s.id) FROM sources s LEFT JOIN goals g ON g.source_id=s.id WHERE s.id=?`, id).
		Scan(&src.ID, &src.Text, &src.Kind, &src.Mode, &src.Web, &src.Revision, &src.Archived, &src.CreatedAt, &src.GoalID, &src.HasImage)
	if err != nil {
		return src, notFound(err, "source")
	}
	jobs, err := sourceJobs(ctx, tx, id)
	if err != nil {
		return src, err
	}
	if len(jobs) > 0 {
		last := jobs[len(jobs)-1]
		src.Job, src.Status = &last, last.Status
		if src.Status == "complete" {
			src.Status = "ready"
		}
	}
	if src.Archived {
		src.Status = "archived"
	}
	if details {
		src.Jobs = jobs
		if src.Documents, err = sourceDocuments(ctx, tx, id); err != nil {
			return src, err
		}
		rows, err := tx.QueryContext(ctx, "SELECT id FROM quizzes WHERE source_id=? ORDER BY created_at,rowid", id)
		if err != nil {
			return src, err
		}
		var ids []string
		for rows.Next() {
			var qid string
			if err = rows.Scan(&qid); err != nil {
				rows.Close()
				return src, err
			}
			ids = append(ids, qid)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return src, err
		}
		rows.Close()
		src.Quizzes = make([]Quiz, 0, len(ids))
		for _, qid := range ids {
			q, err := quiz(ctx, tx, qid)
			if err != nil {
				return src, err
			}
			src.Quizzes = append(src.Quizzes, q)
		}
	}
	return src, nil
}

func (s *Store) Quiz(ctx context.Context, id string) (Quiz, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Quiz{}, err
	}
	defer tx.Rollback()
	q, err := quiz(ctx, tx, id)
	if err != nil {
		return Quiz{}, err
	}
	if err = tx.Commit(); err != nil {
		return Quiz{}, err
	}
	return q, nil
}

func applyContent(q *Quiz, generated GeneratedQuiz) {
	q.Kind, q.Grading, q.Rubric, q.Prompt, q.Answer, q.Explanation, q.Evidence, q.Basis = generated.Kind, generated.Grading, generated.Rubric, generated.Prompt, generated.Answer, generated.Explanation, generated.Evidence, generated.Basis
	q.Choices, q.Variants = generated.Choices, generated.Variants
	q.Level, q.AnswerForm, q.ChoiceConcepts, q.Citations = generated.Level, generated.AnswerForm, generated.ChoiceConcepts, generated.Citations
}

func quiz(ctx context.Context, tx *sql.Tx, id string) (Quiz, error) {
	var q Quiz
	var content string
	err := tx.QueryRowContext(ctx, `SELECT q.id,q.source_id,q.version,(q.archived OR src.archived),v.content,sc.due_at,`+quizAvailableAtSQL+`,
	 COALESCE((SELECT concept_id FROM concept_quizzes WHERE quiz_id=q.id AND role='assesses'),'')
	 FROM quizzes q JOIN sources src ON src.id=q.source_id JOIN quiz_versions v ON v.quiz_id=q.id AND v.version=q.version
	 JOIN schedules sc ON sc.quiz_id=q.id WHERE q.id=?`, id).Scan(&q.ID, &q.SourceID, &q.Version, &q.Archived, &content, &q.DueAt, &q.AvailableAt, &q.ConceptID)
	if err != nil {
		return q, notFound(err, "quiz")
	}
	var generated GeneratedQuiz
	if err = json.Unmarshal([]byte(content), &generated); err != nil {
		return q, err
	}
	applyContent(&q, generated)
	return q, nil
}

// carryRubric settles grading for an edit that does not author it. The learner
// never chooses grading or writes rubric ideas. A generated rubric describes
// one prompt, expected answer, and quoted evidence, so it carries forward only
// while the response style, prompt, expected answer, and evidence are unchanged.
// Any change to them makes the new version exact; nothing silently keeps a
// rubric written for other wording or no longer supported by its quotation.
// Content that explicitly authors grading (fixtures, tests) is left as given.
func carryRubric(current Quiz, edited GeneratedQuiz) GeneratedQuiz {
	if edited.Grading != "" || edited.Rubric != nil {
		return edited
	}
	if current.Grading == "semantic" && current.Rubric != nil && edited.Kind == current.Kind &&
		strings.TrimSpace(edited.Prompt) == strings.TrimSpace(current.Prompt) &&
		strings.TrimSpace(edited.Answer) == strings.TrimSpace(current.Answer) &&
		strings.TrimSpace(edited.Evidence) == strings.TrimSpace(current.Evidence) {
		rubric := Rubric{
			Required:       append([]RubricIdea(nil), current.Rubric.Required...),
			Contradictions: append([]RubricClaim(nil), current.Rubric.Contradictions...),
		}
		edited.Grading, edited.Rubric = "semantic", &rubric
	}
	return edited
}

// EditQuiz saves a learner edit as a new version. The concept, level, answer
// form, choice concepts, and citations carry over unless the edit changes the
// choices (then choice concepts are dropped) or the quotation (then citations
// are re-derived from the stored documents).
func (s *Store) EditQuiz(ctx context.Context, id string, expectedVersion int, content GeneratedQuiz) (Quiz, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Quiz{}, err
	}
	defer tx.Rollback()
	q, err := quiz(ctx, tx, id)
	if err != nil {
		return Quiz{}, err
	}
	if q.Version != expectedVersion {
		return Quiz{}, fmt.Errorf("%w: the quiz was already edited; reload before editing", ErrConflict)
	}
	m, err := loadMaterial(ctx, tx, q.SourceID, 1)
	if err != nil {
		return Quiz{}, err
	}
	var revision int
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM sources WHERE id=?", q.SourceID).Scan(&revision); err != nil {
		return Quiz{}, err
	}
	if revision != 1 {
		if m, err = loadMaterial(ctx, tx, q.SourceID, revision); err != nil {
			return Quiz{}, err
		}
	}
	content = carryRubric(q, content)
	content.Concept = q.ConceptID
	if content.Level == "" {
		content.Level = q.Level
	}
	if content.Kind == "recall" && content.AnswerForm == "" && q.Kind == "recall" {
		content.AnswerForm = q.AnswerForm
	}
	if content.Kind != "recall" {
		content.AnswerForm = ""
	}
	if content.ChoiceConcepts == nil && content.Kind == "choice" && strings.Join(content.Choices, "\x00") == strings.Join(q.Choices, "\x00") {
		content.ChoiceConcepts = q.ChoiceConcepts
	}
	if content.Citations == nil && content.Basis == "web" {
		content.Citations = q.Citations
	}
	if err = validateQuiz(&content, m); err != nil {
		return Quiz{}, err
	}
	encoded, err := marshal(content)
	if err != nil {
		return Quiz{}, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO quiz_versions(quiz_id,version,content,model,prompt_version,created_at) VALUES(?,?,?,'','manual-edit',?)", id, q.Version+1, encoded, s.now())
	if err != nil {
		return Quiz{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE quizzes SET version=version+1 WHERE id=?", id); err != nil {
		return Quiz{}, err
	}
	if err = retireUnanswered(ctx, tx, id, ""); err != nil {
		return Quiz{}, err
	}
	if err = indexQuizContent(ctx, tx, id, content); err != nil {
		return Quiz{}, err
	}
	q, err = quiz(ctx, tx, id)
	if err != nil {
		return Quiz{}, err
	}
	if err = tx.Commit(); err != nil {
		return Quiz{}, err
	}
	return q, nil
}

func (s *Store) ArchiveQuiz(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = quiz(ctx, tx, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE quizzes SET archived=1 WHERE id=?", id); err != nil {
		return err
	}
	if err = retireUnanswered(ctx, tx, id, ""); err != nil {
		return err
	}
	if err = unindex(ctx, tx, "question", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ArchiveSource(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = source(ctx, tx, id, false); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE sources SET archived=1 WHERE id=?", id); err != nil {
		return err
	}
	now := s.now()
	if _, err = tx.ExecContext(ctx, "UPDATE goals SET status='archived',updated_at=? WHERE source_id=?", now, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE job_attempts SET state='unknown',finished_at=? WHERE state='active'
	 AND job_id IN (SELECT id FROM jobs WHERE source_id=?)`, now, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE jobs SET status='canceled',error='Source archived; any in-flight cost remains accounted',lease_token='',lease_until=0,updated_at=?
	 WHERE source_id=? AND status IN ('queued','running','retry','paused')`, now, id); err != nil {
		return err
	}
	if err = retireUnanswered(ctx, tx, "", id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM search_index WHERE (kind='source' AND ref=?) OR (kind='question' AND ref IN (SELECT id FROM quizzes WHERE source_id=?))`, id, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Dispute(ctx context.Context, reviewID, note string, reset bool) error {
	if err := validText("correction note", note, 4096, true); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var quizID string
	err = tx.QueryRowContext(ctx, "SELECT p.quiz_id FROM review_events e JOIN presentations p ON p.id=e.presentation_id WHERE e.id=?", reviewID).Scan(&quizID)
	if err != nil {
		return notFound(err, "review")
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM corrections WHERE review_id=? AND note=? AND reset=?)", reviewID, note, reset).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return tx.Commit()
	}
	var before, after any
	now := s.now()
	if reset {
		var card string
		if err = tx.QueryRowContext(ctx, "SELECT card FROM schedules WHERE quiz_id=?", quizID).Scan(&card); err != nil {
			return err
		}
		before = card
		encoded, err := marshal(learning.NewCard(time.UnixMilli(now)))
		if err != nil {
			return err
		}
		after = encoded
		if _, err = tx.ExecContext(ctx, "UPDATE schedules SET version=version+1,card=?,due_at=?,algorithm=? WHERE quiz_id=?", encoded, now, learning.Algorithm, quizID); err != nil {
			return err
		}
		if err = retireUnanswered(ctx, tx, quizID, ""); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO corrections(id,review_id,note,reset,created_at,schedule_before,schedule_after) VALUES(?,?,?,?,?,?,?)", newID(), reviewID, note, reset, now, before, after)
	if err != nil {
		return err
	}
	return tx.Commit()
}
