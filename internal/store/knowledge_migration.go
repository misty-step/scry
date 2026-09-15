package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/misty-step/scry/internal/learning"
)

// CanUpgradeSchema is compatibility metadata, not proof that a file is valid.
func CanUpgradeSchema(version int) bool { return version == 1 || version == SchemaVersion }

// ValidateDatabase never migrates, changes journal mode, or claims work. A
// rollback binary must require strict current compatibility, not allowUpgrade.
func ValidateDatabase(ctx context.Context, path string, allowUpgrade bool) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: database must be a regular local file", ErrInvalid)
	}
	u := url.URL{Scheme: "file", Path: absolute}
	q := url.Values{"mode": {"ro"}, "_pragma": {"foreign_keys(1)", "trusted_schema(OFF)", "query_only(1)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version, app int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&app); err != nil {
		return err
	}
	if app != ApplicationID || (!allowUpgrade && version != SchemaVersion) || !CanUpgradeSchema(version) {
		return fmt.Errorf("%w: incompatible database application/schema (%d/%d)", ErrInvalid, app, version)
	}
	if err = validateState(ctx, tx, version); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateKnowledge(ctx context.Context, tx *sql.Tx, now int64) error {
	if _, err := tx.ExecContext(ctx, schemaV2); err != nil {
		return fmt.Errorf("migration 2 schema: %w", err)
	}
	ids, err := rowIDs(ctx, tx, "SELECT id FROM sources ORDER BY created_at,id")
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = ensureGoal(ctx, tx, id, now); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE jobs SET goal_id=(SELECT id FROM goals WHERE source_id=jobs.source_id),goal_revision=1,new_quizzes=published,new_materials=published`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT q.id,q.source_id,q.version,q.archived,q.created_at,q.origin_job_id,q.origin_index,v.version,v.content,v.model,v.prompt_version,v.created_at FROM quizzes q JOIN quiz_versions v ON v.quiz_id=q.id ORDER BY q.id,v.version`)
	if err != nil {
		return err
	}
	type oldVersion struct {
		id, source, job, content, model, prompt string
		current, version, index                 int
		archived                                bool
		created, versionAt                      int64
	}
	versions := []oldVersion{}
	for rows.Next() {
		var v oldVersion
		if err = rows.Scan(&v.id, &v.source, &v.current, &v.archived, &v.created, &v.job, &v.index, &v.version, &v.content, &v.model, &v.prompt, &v.versionAt); err != nil {
			rows.Close()
			return err
		}
		versions = append(versions, v)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, v := range versions {
		if v.version == 1 {
			if _, err = tx.ExecContext(ctx, `INSERT INTO materials(id,source_id,version,kind,archived,created_at,first_presented_at,origin_job_id,origin_index) VALUES(?,?,?,'quiz',?,?,COALESCE((SELECT min(created_at) FROM presentations WHERE quiz_id=?),0),?,?)`, v.id, v.source, v.current, v.archived, v.created, v.id, v.job, v.index); err != nil {
				return err
			}
		}
		var q GeneratedQuiz
		if err = json.Unmarshal([]byte(v.content), &q); err != nil {
			return err
		}
		p := Provenance{SourceID: v.source, JobID: v.job, Model: v.model, PromptVersion: v.prompt, Basis: q.Basis, Evidence: q.Evidence, Reason: "Migrated immutable quiz version; historical knowledge coverage remains unmapped", CreatedAt: v.versionAt}
		if err = tx.QueryRowContext(ctx, "SELECT source_revision FROM jobs WHERE id=?", v.job).Scan(&p.SourceRevision); err != nil {
			return err
		}
		m := Material{ID: v.id, SourceID: v.source, Version: v.version, Kind: "quiz", Level: "target", Title: q.Prompt, Basis: q.Basis, Evidence: q.Evidence, EstimatedSeconds: 60, Unmapped: true, Provenance: p, CreatedAt: v.created}
		content, _ := marshal(m)
		provenance, _ := marshal(p)
		if _, err = tx.ExecContext(ctx, `INSERT INTO material_versions(material_id,version,content,quiz_id,quiz_version,provenance_json,created_at) VALUES(?,?,?,?,?,?,?)`, v.id, v.version, content, v.id, v.version, provenance, v.versionAt); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO goal_materials(goal_id,material_id,material_version,origin_job_id,created_at) SELECT id,?,?,?,? FROM goals WHERE source_id=?`, v.id, v.version, v.job, v.versionAt, v.source); err != nil {
			return err
		}
	}
	// Original raw quiz snapshots and all occurrence fields are copied verbatim.
	// The new material snapshot is an additive unmapped view, never a rewrite.
	ids, err = rowIDs(ctx, tx, "SELECT id FROM presentations ORDER BY created_at,id")
	if err != nil {
		return err
	}
	for _, id := range ids {
		var quizID, raw string
		var version int
		if err = tx.QueryRowContext(ctx, "SELECT quiz_id,content_version,snapshot FROM presentations WHERE id=?", id).Scan(&quizID, &version, &raw); err != nil {
			return err
		}
		m, err := materialVersion(ctx, tx, quizID, version)
		if err != nil {
			return err
		}
		var q Quiz
		if err = json.Unmarshal([]byte(raw), &q); err != nil {
			return err
		}
		m.Quiz = &q
		m.DueAt = q.DueAt
		if err = tx.QueryRowContext(ctx, "SELECT id,revision FROM goals WHERE source_id=?", m.SourceID).Scan(&m.GoalID, &m.GoalRevision); err != nil {
			return err
		}
		snapshot, _ := marshal(m)
		if _, err = tx.ExecContext(ctx, `INSERT INTO presentations_v2(id,quiz_id,content_version,schedule_version,snapshot,created_at,answer,outcome,assisted,graded,rating,due_at,reviewed_at,review_id,kind,material_id,material_version,material_snapshot,goal_id,goal_revision,goal_pin_origin)
		 SELECT id,quiz_id,content_version,schedule_version,snapshot,created_at,answer,outcome,assisted,graded,rating,due_at,reviewed_at,review_id,'quiz',quiz_id,content_version,?,?,?,'migration-v1-origin-source' FROM presentations WHERE id=?`, snapshot, m.GoalID, m.GoalRevision, id); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "DROP TABLE presentations; ALTER TABLE presentations_v2 RENAME TO presentations;"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO interactions(id,presentation_id,material_id,material_version,kind,outcome,assisted,at,snapshot,reason,answer,mode)
	 SELECT e.id,e.presentation_id,p.material_id,p.material_version,'review',e.outcome,e.assisted,e.reviewed_at,'{}','Migrated actual direct review; historical coverage is unmapped',e.answer,json_extract(e.snapshot,'$.kind') FROM review_events e JOIN presentations p ON p.id=e.presentation_id`); err != nil {
		return err
	}
	for _, table := range immutableKnowledgeTables {
		for _, action := range []string{"UPDATE", "DELETE"} {
			name := "immutable_" + table + "_" + strings.ToLower(action)
			if _, err = tx.ExecContext(ctx, fmt.Sprintf("CREATE TRIGGER %s BEFORE %s ON %s BEGIN SELECT RAISE(ABORT,'knowledge history is immutable'); END", name, action, table)); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, "PRAGMA user_version=2"); err != nil {
		return err
	}
	// Migration itself does not spend. Existing queued/live/unknown work fences
	// this durable enrichment trigger until it can be considered safely.
	return queueEnrichment(ctx, tx, now)
}

func queueEnrichment(ctx context.Context, tx *sql.Tx, now int64) error {
	ids, err := rowIDs(ctx, tx, `SELECT s.id FROM sources s JOIN goals g ON g.source_id=s.id WHERE s.archived=0 AND json_extract(g.coverage_json,'$.kind')='unmapped'
	 AND EXISTS(SELECT 1 FROM materials m WHERE m.source_id=s.id)
	 AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.source_id=s.id AND j.source_revision=s.revision AND j.kind IN('enrich','capture') AND (j.kind='enrich' OR j.status IN('queued','running','retry','paused')))
	 AND NOT EXISTS(SELECT 1 FROM job_attempts a JOIN jobs j ON j.id=a.job_id WHERE j.source_id=s.id AND a.cost_micros IS NULL AND a.state<>'active') ORDER BY s.created_at,s.id`)
	if err != nil {
		return err
	}
	for _, id := range ids {
		var revision int
		if err = tx.QueryRowContext(ctx, "SELECT revision FROM sources WHERE id=?", id).Scan(&revision); err != nil {
			return err
		}
		if _, err = enqueueKnowledge(ctx, tx, id, revision, "enrich", "", 0, "", 0, now); err != nil {
			return err
		}
	}
	return nil
}

var createTablePattern = regexp.MustCompile(`(?s)CREATE TABLE ([a-z_0-9]+) \((.*?)\) STRICT;`)
var columnPattern = regexp.MustCompile(`(?m)(?:^|,)\s*([a-z_][a-z_0-9]*)\s+(TEXT|INTEGER)\b`)

func validateState(ctx context.Context, tx *sql.Tx, version int) error {
	if err := validateCanonicalSchema(ctx, tx, version); err != nil {
		return err
	}
	if version == 2 {
		if err := validateKnowledgeJSON(ctx, tx); err != nil {
			return err
		}
	}
	for _, pragma := range []string{"PRAGMA integrity_check", "PRAGMA foreign_key_check"} {
		rows, err := tx.QueryContext(ctx, pragma)
		if err != nil {
			return err
		}
		if pragma == "PRAGMA integrity_check" {
			for rows.Next() {
				var result string
				if err = rows.Scan(&result); err != nil {
					rows.Close()
					return err
				}
				if result != "ok" {
					rows.Close()
					return fmt.Errorf("%w: SQLite integrity: %s", ErrInvalid, result)
				}
			}
		} else if rows.Next() {
			rows.Close()
			return fmt.Errorf("%w: SQLite foreign key check failed", ErrInvalid)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	tables := map[string]map[string]string{}
	for _, match := range createTablePattern.FindAllStringSubmatch(schemaV1, -1) {
		cols := map[string]string{}
		for _, c := range columnPattern.FindAllStringSubmatch(match[2], -1) {
			cols[c[1]] = c[2]
		}
		tables[match[1]] = cols
	}
	if version == 2 {
		for _, match := range createTablePattern.FindAllStringSubmatch(schemaV2, -1) {
			name := match[1]
			if name == "presentations_v2" {
				name = "presentations"
			}
			cols := map[string]string{}
			for _, c := range columnPattern.FindAllStringSubmatch(match[2], -1) {
				cols[c[1]] = c[2]
			}
			tables[name] = cols
		}
		for _, match := range regexp.MustCompile(`ALTER TABLE jobs ADD COLUMN ([a-z_]+) (TEXT|INTEGER)`).FindAllStringSubmatch(schemaV2, -1) {
			tables["jobs"][match[1]] = match[2]
		}
	}
	for table, columns := range tables {
		rows, err := tx.QueryContext(ctx, "PRAGMA table_info("+table+")")
		if err != nil {
			return err
		}
		seen := map[string]string{}
		for rows.Next() {
			var cid, notnull, pk int
			var name, kind string
			var d any
			if err = rows.Scan(&cid, &name, &kind, &notnull, &d, &pk); err != nil {
				rows.Close()
				return err
			}
			seen[name] = strings.ToUpper(kind)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if len(seen) != len(columns) {
			return fmt.Errorf("%w: unexpected %s column shape", ErrInvalid, table)
		}
		for name, kind := range columns {
			if seen[name] != kind {
				return fmt.Errorf("%w: missing or invalid %s.%s", ErrInvalid, table, name)
			}
		}
		var strict int
		if err = tx.QueryRowContext(ctx, "SELECT strict FROM pragma_table_list WHERE name=? AND schema='main'", table).Scan(&strict); err != nil || strict != 1 {
			return fmt.Errorf("%w: invalid strict table %s", ErrInvalid, table)
		}
	}
	for _, name := range []string{"immutable_review_update", "immutable_review_delete", "immutable_version_update", "immutable_version_delete", "immutable_operation_update", "immutable_operation_delete"} {
		var sqlText string
		if err := tx.QueryRowContext(ctx, "SELECT sql FROM sqlite_schema WHERE type='trigger' AND name=?", name).Scan(&sqlText); err != nil || !strings.Contains(sqlText, "RAISE(ABORT") {
			return fmt.Errorf("%w: missing immutable history fence %s", ErrInvalid, name)
		}
	}
	if version == 2 {
		for _, table := range immutableKnowledgeTables {
			for _, action := range []string{"update", "delete"} {
				var exists bool
				if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE type='trigger' AND name=?)", "immutable_"+table+"_"+action).Scan(&exists); err != nil || !exists {
					return fmt.Errorf("%w: missing knowledge immutable fence", ErrInvalid)
				}
			}
		}
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM review_session WHERE singleton=1").Scan(&count); err != nil || count != 1 {
		return fmt.Errorf("%w: missing session singleton", ErrInvalid)
	}
	checks := []string{
		`SELECT count(*) FROM sources s LEFT JOIN source_revisions r ON r.source_id=s.id AND r.revision=s.revision WHERE r.source_id IS NULL OR r.text<>s.text OR r.kind<>s.kind`,
		`SELECT count(*) FROM quizzes q LEFT JOIN quiz_versions v ON v.quiz_id=q.id AND v.version=q.version LEFT JOIN schedules sc ON sc.quiz_id=q.id WHERE v.quiz_id IS NULL OR sc.quiz_id IS NULL`,
		`SELECT count(*) FROM jobs j LEFT JOIN source_revisions r ON r.source_id=j.source_id AND r.revision=j.source_revision WHERE r.source_id IS NULL`,
		`SELECT count(*) FROM jobs j WHERE (j.status='running')<>(j.lease_token<>'') OR (j.status='running' AND NOT EXISTS(SELECT 1 FROM job_attempts a WHERE a.job_id=j.id AND a.token=j.lease_token AND a.state='active' AND a.number=j.attempts))`,
		`SELECT count(*) FROM job_attempts a WHERE a.state='active' AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.id=a.job_id AND j.status='running' AND j.lease_token=a.token)`,
	}
	if version == 2 {
		checks = append(checks,
			`SELECT count(*) FROM knowledge_units u LEFT JOIN unit_versions v ON v.unit_id=u.id AND v.version=u.version WHERE v.unit_id IS NULL`,
			`SELECT count(*) FROM materials m LEFT JOIN material_versions v ON v.material_id=m.id AND v.version=m.version WHERE v.material_id IS NULL`,
			`SELECT count(*) FROM knowledge_relations r LEFT JOIN relation_versions v ON v.relation_id=r.id AND v.version=r.version WHERE v.relation_id IS NULL`,
			`SELECT count(*) FROM goals g LEFT JOIN goal_versions v ON v.goal_id=g.id AND v.revision=g.revision WHERE v.goal_id IS NULL`,
			`SELECT count(*) FROM presentations WHERE (kind='quiz')<>(quiz_id IS NOT NULL) OR (practice=1 AND rating<>0)`,
			`SELECT count(*) FROM jobs j LEFT JOIN goals g ON g.id=j.goal_id WHERE g.id IS NULL OR j.goal_revision<1`,
		)
	}
	for _, query := range checks {
		if err := tx.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return fmt.Errorf("validate durable shape: %w", err)
		}
		if count != 0 {
			return fmt.Errorf("%w: inconsistent durable references (%d rows)", ErrInvalid, count)
		}
	}
	for table, columns := range tables {
		for column := range columns {
			if column == "content" || column == "snapshot" || column == "material_snapshot" || column == "card" || column == "schedule_before" || column == "schedule_after" || column == "result_json" || strings.HasSuffix(column, "_json") {
				query := "SELECT " + column + " FROM " + table + " WHERE " + column + " IS NOT NULL"
				rows, err := tx.QueryContext(ctx, query)
				if err != nil {
					return err
				}
				for rows.Next() {
					var text string
					if err = rows.Scan(&text); err != nil {
						rows.Close()
						return err
					}
					if !json.Valid([]byte(text)) || strings.TrimSpace(text) == "null" {
						rows.Close()
						return fmt.Errorf("%w: malformed durable JSON %s.%s", ErrInvalid, table, column)
					}
					if column == "card" || column == "schedule_before" || column == "schedule_after" {
						var card learning.Card
						if err = json.Unmarshal([]byte(text), &card); err != nil || card.Due.IsZero() {
							rows.Close()
							return fmt.Errorf("%w: malformed FSRS card", ErrInvalid)
						}
					}
					if table == "quiz_versions" && column == "content" {
						var q GeneratedQuiz
						if err = json.Unmarshal([]byte(text), &q); err != nil || q.Prompt == "" || q.Answer == "" || (q.Kind != "choice" && q.Kind != "recall") {
							rows.Close()
							return fmt.Errorf("%w: malformed quiz content", ErrInvalid)
						}
					}
					if column == "snapshot" && (table == "presentations" || table == "review_events") {
						var q Quiz
						if err = json.Unmarshal([]byte(text), &q); err != nil {
							rows.Close()
							return fmt.Errorf("%w: invalid presented snapshot", ErrInvalid)
						}
						if q.ID == "" && version == 1 {
							rows.Close()
							return fmt.Errorf("%w: invalid historical quiz identity", ErrInvalid)
						}
					}
				}
				if err = rows.Err(); err != nil {
					rows.Close()
					return err
				}
				rows.Close()
			}
		}
	}
	return nil
}
