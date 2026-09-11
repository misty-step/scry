package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/misty-step/scry/internal/learning"
)

type FoundationUnit struct {
	Key        string `json:"key"`
	Definition string `json:"definition"`
	Kind       string `json:"kind"`
}

type FoundationLink struct {
	Unit       string `json:"unit"`
	Role       string `json:"role"`
	Provenance string `json:"provenance"`
}

// Steps are a plain-text ordered process diagram, never executable markup.
// URL is an unfetched reference, not an attributed body or verified citation.
type FoundationMaterial struct {
	ID      string           `json:"id,omitempty"`
	Version int              `json:"version,omitempty"`
	Kind    string           `json:"kind"`
	Title   string           `json:"title"`
	Body    string           `json:"body"`
	Steps   []string         `json:"steps"`
	URL     string           `json:"url"`
	Quiz    *GeneratedQuiz   `json:"quiz"`
	Links   []FoundationLink `json:"links"`
}

type FoundationContent struct {
	Units     []FoundationUnit     `json:"units"`
	Materials []FoundationMaterial `json:"materials"`
}

type FoundationBridge struct {
	ID             string               `json:"id"`
	PresentationID string               `json:"presentation_id"`
	SourceRevision int                  `json:"source_revision"`
	Revision       int                  `json:"revision"`
	Phase          string               `json:"phase"`
	Position       int                  `json:"position"`
	Answer         string               `json:"answer"`
	Outcome        string               `json:"outcome"`
	BundleID       string               `json:"bundle_id"`
	Job            *Job                 `json:"job,omitempty"`
	Target         Presentation         `json:"target"`
	Available      bool                 `json:"available"`
	TargetCurrent  bool                 `json:"target_current"`
	Model          string               `json:"model"`
	PromptVersion  string               `json:"prompt_version"`
	Note           string               `json:"note"`
	Materials      []FoundationMaterial `json:"materials,omitempty"`
	Units          []FoundationUnit     `json:"units,omitempty"`
	Current        *FoundationMaterial  `json:"current,omitempty"`
}

// ValidateFoundation enforces the bounded reusable content contract at both the
// provider boundary and publication. It does not claim factual verification.
func ValidateFoundation(f *FoundationContent) error {
	if f == nil || len(f.Units) < 1 || len(f.Units) > 6 || len(f.Materials) < 2 || len(f.Materials) > 12 {
		return fmt.Errorf("%w: foundations require 1–6 units and 2–12 materials", ErrInvalid)
	}
	units := map[string]bool{}
	for _, u := range f.Units {
		if validText("unit key", u.Key, 80, true) != nil || validText("unit definition", u.Definition, 2048, true) != nil || (u.Kind != "foundation" && u.Kind != "composition") || units[u.Key] {
			return fmt.Errorf("%w: invalid or duplicate foundation unit", ErrInvalid)
		}
		units[u.Key] = true
	}
	instruction, practice := false, false
	taught := map[string]bool{}
	practiceCount := 0
	for _, m := range f.Materials {
		if m.ID != "" || m.Version != 0 || validText("material title", m.Title, 240, true) != nil || validText("material body", m.Body, 8192, m.Kind != "practice") != nil || len(m.Links) < 1 || len(m.Links) > 12 {
			return fmt.Errorf("%w: invalid foundation material", ErrInvalid)
		}
		switch m.Kind {
		case "instruction", "reference", "diagram":
			instruction = instruction || m.Kind == "instruction"
			if practice || m.Quiz != nil {
				return fmt.Errorf("%w: instruction must precede practice", ErrInvalid)
			}
		case "practice":
			practice = true
			practiceCount++
			if practiceCount > 4 {
				return fmt.Errorf("%w: at most four warm practice questions", ErrInvalid)
			}
			if !instruction || m.Quiz == nil {
				return fmt.Errorf("%w: practice needs prior instruction and a question", ErrInvalid)
			}
			if err := validateQuiz(*m.Quiz, Source{Kind: "topic"}); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: unsupported material kind", ErrInvalid)
		}
		if len(m.Steps) > 8 || (m.Kind == "diagram" && len(m.Steps) < 2) || (m.Kind != "diagram" && len(m.Steps) != 0) {
			return fmt.Errorf("%w: diagram requires 2–8 plain-text steps", ErrInvalid)
		}
		for _, step := range m.Steps {
			if err := validText("diagram step", step, 1024, true); err != nil {
				return err
			}
		}
		if m.URL != "" {
			u, err := url.Parse(m.URL)
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(m.URL) > 2048 || strings.ContainsAny(m.URL, "\r\n\t") || m.Kind != "reference" {
				return fmt.Errorf("%w: references require a safe HTTPS URL without credentials", ErrInvalid)
			}
		}
		assesses := false
		seen := map[string]bool{}
		for _, link := range m.Links {
			key := link.Unit + ":" + link.Role
			if !units[link.Unit] || seen[key] || validText("coverage provenance", link.Provenance, 1024, true) != nil {
				return fmt.Errorf("%w: invalid coverage", ErrInvalid)
			}
			seen[key] = true
			if link.Role == "teaches" && m.Kind != "practice" {
				taught[link.Unit] = true
			}
			if link.Role == "directly-assesses" && !taught[link.Unit] {
				return fmt.Errorf("%w: practice must assess a previously taught unit", ErrInvalid)
			}
			switch link.Role {
			case "teaches", "assumes", "mentions":
			case "directly-assesses":
				assesses = true
			default:
				return fmt.Errorf("%w: invalid coverage role", ErrInvalid)
			}
		}
		if assesses != (m.Kind == "practice") {
			return fmt.Errorf("%w: direct assessment belongs only to practice", ErrInvalid)
		}
	}
	if !instruction || !practice {
		return fmt.Errorf("%w: useful instruction and practice are required", ErrInvalid)
	}
	return nil
}

func publishFoundation(ctx context.Context, tx *sql.Tx, j Job, result GenerationResult, now int64) (int, error) {
	id := newID()
	_, err := tx.ExecContext(ctx, `INSERT INTO foundation_bundles(id,job_id,source_id,source_revision,quiz_id,quiz_version,model,prompt_version,note,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, j.ID, j.SourceID, j.SourceRevision, j.FoundationTarget.ID, j.FoundationTarget.Version, result.Model, result.PromptVersion, result.Note, now)
	if err != nil {
		return 0, err
	}
	unitIDs := map[string]string{}
	provenance := "Recorded producer: " + result.Model + " / " + result.PromptVersion + "; source lineage, not verified evidence"
	for _, u := range result.Foundation.Units {
		uid := newID()
		unitIDs[u.Key] = uid
		if _, err = tx.ExecContext(ctx, "INSERT INTO foundation_units(id,version,definition,kind,provenance) VALUES(?,1,?,?,?)", uid, u.Definition, u.Kind, provenance); err != nil {
			return 0, err
		}
	}
	for position, m := range result.Foundation.Materials {
		mid := newID()
		m.Links = append([]FoundationLink(nil), m.Links...)
		for i := range m.Links {
			m.Links[i].Unit = unitIDs[m.Links[i].Unit]
		}
		encoded, err := marshal(m)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO foundation_materials(id,version,bundle_id,position,content,provenance) VALUES(?,1,?,?,?,?)", mid, id, position, encoded, provenance); err != nil {
			return 0, err
		}
		for _, link := range m.Links {
			if _, err = tx.ExecContext(ctx, "INSERT INTO foundation_links(material_id,material_version,unit_id,unit_version,role,provenance) VALUES(?,1,?,1,?,?)", mid, link.Unit, link.Role, link.Provenance); err != nil {
				return 0, err
			}
		}
	}
	_, err = tx.ExecContext(ctx, "UPDATE foundation_bridges SET bundle_id=?,phase='ready',revision=revision+1 WHERE job_id=? AND phase='waiting'", id, j.ID)
	return len(result.Foundation.Materials), err
}

// RequestFoundation records a request, not assistance, an attempt, or failure.
// One bundle is reused only for the same source revision and exact quiz version.
func (s *Store) RequestFoundation(ctx context.Context, presentationID, operationID, draft string, expectedBridgeRevision int) (string, error) {
	if err := validOperation(operationID); err != nil {
		return "", err
	}
	if err := validText("saved target draft", draft, 1024, false); err != nil {
		return "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	hash := payloadHash(struct {
		Presentation, Draft string
		BridgeRevision      int
	}{presentationID, draft, expectedBridgeRevision})
	id, _, found, err := existingOperation(ctx, tx, operationID, "foundation-request", hash)
	if err != nil {
		return "", err
	}
	if found {
		return id, tx.Commit()
	}
	p, err := presentation(ctx, tx, presentationID)
	if err != nil {
		return "", err
	}
	var current string
	var revision int
	var valid bool
	if err = cancelSupersededFoundations(ctx, tx, s.now()); err != nil {
		return "", err
	}
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(r.current_id,''),src.revision,(q.archived=0 AND src.archived=0 AND q.version=?) FROM review_session r,quizzes q JOIN sources src ON src.id=q.source_id WHERE q.id=?`, p.Quiz.Version, p.Quiz.ID).Scan(&current, &revision, &valid)
	if err != nil {
		return "", err
	}
	if current != p.ID || !valid {
		return "", fmt.Errorf("%w: target changed; reload review", ErrConflict)
	}
	if expectedBridgeRevision != p.BridgeRevision {
		return "", fmt.Errorf("%w: bridge or draft changed; reload saved state before changing the draft", ErrConflict)
	}
	err = tx.QueryRowContext(ctx, "SELECT id FROM foundation_bridges WHERE presentation_id=?", p.ID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		id = newID()
		var bundleID, jobID sql.NullString
		err = tx.QueryRowContext(ctx, `SELECT id FROM foundation_bundles WHERE quiz_id=? AND quiz_version=? AND source_revision=? ORDER BY created_at DESC,id LIMIT 1`, p.Quiz.ID, p.Quiz.Version, revision).Scan(&bundleID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		phase := "ready"
		if !bundleID.Valid {
			phase = "waiting"
			err = tx.QueryRowContext(ctx, `SELECT j.id FROM jobs j JOIN foundation_requests f ON f.job_id=j.id WHERE f.quiz_id=? AND f.quiz_version=? AND j.source_revision=? ORDER BY j.created_at DESC,j.id LIMIT 1`, p.Quiz.ID, p.Quiz.Version, revision).Scan(&jobID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return "", err
			}
			if !jobID.Valid {
				var busy bool
				if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE source_id=? AND status IN ('queued','running','retry','paused'))", p.Quiz.SourceID).Scan(&busy); err != nil {
					return "", err
				}
				if busy {
					return "", fmt.Errorf("%w: saved source work is still pending; ordinary review remains available", ErrConflict)
				}
				jobID = sql.NullString{String: newID(), Valid: true}
				now := s.now()
				if _, err = tx.ExecContext(ctx, `INSERT INTO jobs(id,source_id,source_revision,status,created_at,updated_at,available_at) VALUES(?,?,?,'queued',?,?,?)`, jobID.String, p.Quiz.SourceID, revision, now, now, now); err != nil {
					return "", err
				}
				if _, err = tx.ExecContext(ctx, "INSERT INTO foundation_requests(job_id,quiz_id,quiz_version) VALUES(?,?,?)", jobID.String, p.Quiz.ID, p.Quiz.Version); err != nil {
					return "", err
				}
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO foundation_bridges(id,presentation_id,source_revision,job_id,bundle_id,phase,draft,created_at) VALUES(?,?,?,?,?,?,?,?)`, id, p.ID, revision, jobID, bundleID, phase, draft, s.now())
	} else if err == nil {
		_, err = tx.ExecContext(ctx, "UPDATE foundation_bridges SET draft=?,revision=revision+1 WHERE id=?", draft, id)
	}
	if err != nil {
		return "", err
	}
	if err = saveOperation(ctx, tx, operationID, "foundation-request", hash, id, id, s.now()); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

func foundationBridge(ctx context.Context, tx *sql.Tx, id string) (FoundationBridge, error) {
	var b FoundationBridge
	var jobID sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT id,presentation_id,source_revision,revision,phase,position,answer,outcome,COALESCE(bundle_id,''),job_id FROM foundation_bridges WHERE id=?`, id).Scan(&b.ID, &b.PresentationID, &b.SourceRevision, &b.Revision, &b.Phase, &b.Position, &b.Answer, &b.Outcome, &b.BundleID, &jobID)
	if err != nil {
		return b, notFound(err, "foundation bridge")
	}
	b.Target, err = presentation(ctx, tx, b.PresentationID)
	if err != nil {
		return b, err
	}
	err = tx.QueryRowContext(ctx, `SELECT q.archived=0 AND src.archived=0 AND q.version=? AND src.revision=? FROM quizzes q JOIN sources src ON src.id=q.source_id WHERE q.id=?`, b.Target.Quiz.Version, b.SourceRevision, b.Target.Quiz.ID).Scan(&b.Available)
	if err != nil {
		return b, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(current_id=? ,0) FROM review_session", b.PresentationID).Scan(&b.TargetCurrent); err != nil {
		return b, err
	}
	if jobID.Valid {
		j, err := job(ctx, tx, jobID.String)
		if err != nil {
			return b, err
		}
		b.Job = &j
	}
	if b.BundleID == "" {
		return b, nil
	}
	if err = tx.QueryRowContext(ctx, "SELECT model,prompt_version,note FROM foundation_bundles WHERE id=?", b.BundleID).Scan(&b.Model, &b.PromptVersion, &b.Note); err != nil {
		return b, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,version,content FROM foundation_materials WHERE bundle_id=? ORDER BY position", b.BundleID)
	if err != nil {
		return b, err
	}
	for rows.Next() {
		var m FoundationMaterial
		var encoded string
		if err = rows.Scan(&m.ID, &m.Version, &encoded); err != nil {
			rows.Close()
			return b, err
		}
		if err = json.Unmarshal([]byte(encoded), &m); err != nil {
			rows.Close()
			return b, err
		}
		b.Materials = append(b.Materials, m)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return b, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT DISTINCT u.id,u.definition,u.kind FROM foundation_units u JOIN foundation_links l ON l.unit_id=u.id AND l.unit_version=u.version JOIN foundation_materials m ON m.id=l.material_id AND m.version=l.material_version WHERE m.bundle_id=? ORDER BY u.id`, b.BundleID)
	if err != nil {
		return b, err
	}
	for rows.Next() {
		var u FoundationUnit
		if err = rows.Scan(&u.Key, &u.Definition, &u.Kind); err != nil {
			rows.Close()
			return b, err
		}
		b.Units = append(b.Units, u)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if b.Position < len(b.Materials) {
		m := b.Materials[b.Position]
		b.Current = &m
	}
	return b, err
}

func (s *Store) FoundationBridge(ctx context.Context, id string) (FoundationBridge, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FoundationBridge{}, err
	}
	defer tx.Rollback()
	b, err := foundationBridge(ctx, tx, id)
	if err != nil {
		return b, err
	}
	hideAnswer(&b.Target)
	// Reopening old material during a later cold occurrence requires a fresh
	// acknowledged exposure, not a GET that silently reveals the answer.
	if b.Job != nil {
		b.Job.SourceText = ""
	}
	var needsExposure bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM review_session r JOIN presentations p ON p.id=r.current_id
	 WHERE p.quiz_id=? AND p.content_version=? AND p.graded=0 AND NOT EXISTS(
	 SELECT 1 FROM foundation_interactions i JOIN foundation_bridges b ON b.id=i.bridge_id JOIN presentations t ON t.id=b.presentation_id
	 WHERE t.quiz_id=p.quiz_id AND t.content_version=p.content_version AND i.action='read' AND i.created_at>=p.created_at-86400000))`,
		b.Target.Quiz.ID, b.Target.Quiz.Version).Scan(&needsExposure); err != nil {
		return b, err
	}
	if needsExposure {
		// Redact the historical graded target as well as current bridge content.
		// Its immutable history remains available through the explicit history flow.
		b.Target.Quiz.Answer, b.Target.Quiz.Explanation, b.Target.Quiz.Evidence = "", "", ""
		b.Target.Quiz.Variants = nil
		b.Target.Answer, b.Target.Draft, b.Answer, b.Outcome = "", "", "", ""
		if b.BundleID != "" {
			b.Phase = "ready"
		}
	}
	// A help request alone cannot disclose instruction or practice answers.
	if b.Phase == "ready" || b.Phase == "waiting" {
		b.Materials = nil
		b.Units = nil
		b.Current = nil
	} else {
		b.Materials = nil
		if b.Current != nil && b.Current.Quiz != nil && b.Phase != "feedback" {
			q := *b.Current.Quiz
			q.Answer = ""
			q.Explanation = ""
			q.Variants = nil
			q.Evidence = ""
			b.Current.Quiz = &q
		}
	}
	return b, tx.Commit()
}

// AdvanceFoundation is a compare-and-swap finite path. Exact operation replay
// acknowledges its original commit without replaying a transition or old screen.
func (s *Store) AdvanceFoundation(ctx context.Context, id string, revision int, operationID, action, answer string) error {
	if err := validOperation(operationID); err != nil {
		return err
	}
	if err := validText("practice answer", answer, 1024, action == "answer"); err != nil {
		return err
	}
	hash := payloadHash(struct {
		ID             string
		Revision       int
		Action, Answer string
	}{id, revision, action, answer})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, _, found, err := existingOperation(ctx, tx, operationID, "foundation-step", hash)
	if err != nil {
		return err
	}
	if found {
		return tx.Commit()
	}
	b, err := foundationBridge(ctx, tx, id)
	if err != nil {
		return err
	}
	if b.Revision != revision || !b.Available {
		return fmt.Errorf("%w: bridge or target changed; reload saved progress", ErrConflict)
	}
	phase, position, outcome := b.Phase, b.Position, ""
	interaction := ""
	switch {
	case action == "open" && b.BundleID != "":
		phase, position, interaction = "instruction", 0, "read"
		b.Current = &b.Materials[0]
	case action == "next" && (phase == "instruction" || phase == "feedback"):
		position++
		if position == len(b.Materials) {
			phase = "return"
		} else {
			b.Current = &b.Materials[position]
			if b.Current.Kind == "practice" {
				phase = "practice"
			} else {
				phase, interaction = "instruction", "read"
			}
		}
	case action == "answer" && phase == "practice" && b.Current != nil && b.Current.Quiz != nil:
		q := b.Current.Quiz
		if q.Kind == "choice" {
			valid := false
			for _, c := range q.Choices {
				valid = valid || c == answer
			}
			if !valid {
				return fmt.Errorf("%w: select an exact choice", ErrInvalid)
			}
		}
		outcome, _ = learning.Grade(q.Kind, q.Answer, q.Variants, answer, false)
		phase, interaction = "feedback", "practice"
	case action == "retry" && phase == "waiting" && b.Job != nil:
		if b.Job.Status != "failed" || b.Job.Attempts >= 3 || b.Job.CostUnknown {
			return fmt.Errorf("%w: only known-cost failed foundations below three attempts can retry; uncertain work stays paused", ErrConflict)
		}
		var busy bool
		if err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE source_id=? AND status IN ('queued','running','retry','paused'))", b.Job.SourceID).Scan(&busy); err != nil {
			return err
		}
		if busy {
			return fmt.Errorf("%w: source work is already pending", ErrConflict)
		}
		if _, err = tx.ExecContext(ctx, "UPDATE jobs SET status='retry',error='',available_at=?,updated_at=? WHERE id=?", s.now(), s.now(), b.Job.ID); err != nil {
			return err
		}
	case action == "return":
		interaction, answer, outcome = "return", b.Answer, b.Outcome
	default:
		return fmt.Errorf("%w: action does not match saved bridge progress", ErrConflict)
	}
	if interaction != "" {
		var mid any
		var version any
		if interaction != "return" {
			mid, version = b.Current.ID, b.Current.Version
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO foundation_interactions(id,bridge_id,material_id,material_version,action,answer,outcome,created_at) VALUES(?,?,?,?,?,?,?,?)`, newID(), id, mid, version, interaction, answer, outcome, s.now()); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE foundation_bridges SET revision=revision+1,phase=?,position=?,answer=?,outcome=? WHERE id=?", phase, position, answer, outcome, id); err != nil {
		return err
	}
	if err = saveOperation(ctx, tx, operationID, "foundation-step", hash, id, id, s.now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FoundationBridges(ctx context.Context) ([]FoundationBridge, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT b.id,b.presentation_id,b.phase,b.revision,p.snapshot FROM foundation_bridges b JOIN presentations p ON p.id=b.presentation_id ORDER BY b.created_at DESC,b.id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []FoundationBridge{}
	for rows.Next() {
		var b FoundationBridge
		var snapshot string
		if err = rows.Scan(&b.ID, &b.PresentationID, &b.Phase, &b.Revision, &snapshot); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(snapshot), &b.Target.Quiz); err != nil {
			return nil, err
		}
		hideAnswer(&b.Target)
		result = append(result, b)
	}
	return result, rows.Err()
}
