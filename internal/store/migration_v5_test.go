package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/misty-step/scry/internal/learning"
)

// dumpRows renders every row of a query so history can be compared exactly.
func dumpRows(t *testing.T, db interface {
	Query(string, ...any) (*sql.Rows, error)
}, query string) []string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err = rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.TrimSuffix(fmt.Sprintln(values...), "\n"))
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// historyQueries name every record the upgrade must carry over unchanged.
var historyQueries = []string{
	"SELECT * FROM foundation_units ORDER BY id,version",
	"SELECT * FROM foundation_requests ORDER BY job_id",
	"SELECT * FROM foundation_bundles ORDER BY id",
	"SELECT * FROM foundation_materials ORDER BY id,version",
	"SELECT * FROM foundation_links ORDER BY material_id,unit_id,role",
	"SELECT * FROM foundation_bridges ORDER BY id",
	"SELECT * FROM foundation_interactions ORDER BY id",
	"SELECT id,name,description,created_at FROM concepts ORDER BY id",
	"SELECT * FROM concept_prerequisites ORDER BY concept_id,prerequisite_id",
	`SELECT * FROM "references" ORDER BY id`,
	"SELECT * FROM concept_references ORDER BY concept_id,reference_id",
	"SELECT concept_id,quiz_id,created_at FROM concept_quizzes ORDER BY concept_id,quiz_id",
	"SELECT * FROM quiz_versions ORDER BY quiz_id,version",
	"SELECT * FROM schedules ORDER BY quiz_id",
	"SELECT * FROM presentations ORDER BY id",
	"SELECT * FROM review_events ORDER BY id",
	"SELECT token,job_id,number,started_at,reserved_micros,cost_micros FROM job_attempts ORDER BY token",
}

// US-001 criterion 1: upgrading schema 4 keeps foundation rows and relations
// as history, and foundation-origin concepts never surface in Map or Stream.
func TestSchemaV4ToV5MigrationUS001(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	at := now.Add(-time.Hour).UnixMilli()
	path := filepath.Join(t.TempDir(), "v4.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{schemaV1, schemaV2, schemaV3, schemaV4} {
		if _, err = db.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	fresh, err := marshal(learning.NewCard(now.Add(-2 * time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := learning.Schedule(learning.NewCard(now.Add(-2*time.Hour)), 3, now.Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	after, err := marshal(reviewed)
	if err != nil {
		t.Fatal(err)
	}
	quizA := `{"kind":"recall","prompt":"Which protocol secures HTTPS?","answer":"TLS","explanation":"HTTPS runs over TLS.","basis":"source","choices":null,"variants":null}`
	quizB := `{"kind":"recall","prompt":"What must a certificate name?","answer":"The host","explanation":"The certificate names the host.","basis":"source","choices":null,"variants":null}`
	long := "Certificates and trust\n" + strings.Repeat("A long pasted line about certificate validation. ", 4)
	batch := `{"result":{"quizzes":[],"model":"old-model","prompt_version":"old-prompt"},"cost_micros":5}`
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO sources VALUES('src',?,'source',1,0,?)`, []any{long, at}},
		{`INSERT INTO sources VALUES('old','archived topic','topic',1,1,?)`, []any{at}},
		{`INSERT INTO sources VALUES('live','pending topic','topic',1,0,?)`, []any{at}},
		{`INSERT INTO sources VALUES('crit','critic topic','topic',1,0,?)`, []any{at}},
		{`INSERT INTO source_revisions SELECT id,1,text,kind,created_at FROM sources`, nil},
		{`INSERT INTO jobs(id,source_id,source_revision,status,model,prompt_version,attempts,created_at,updated_at,available_at,published) VALUES('job','src',1,'complete','old-model','old-prompt',1,?,?,?,1)`, []any{at, at, at}},
		{`INSERT INTO jobs(id,source_id,source_revision,status,attempts,created_at,updated_at,available_at,published) VALUES('fdone','src',1,'complete',1,?,?,?,1)`, []any{at, at, at}},
		{`INSERT INTO jobs(id,source_id,source_revision,status,attempts,created_at,updated_at,available_at,lease_token,lease_until) VALUES('fjob','src',1,'running',1,?,?,?,'lease',?)`, []any{at, at, at, now.Add(time.Hour).UnixMilli()}},
		{`INSERT INTO jobs(id,source_id,source_revision,status,created_at,updated_at,available_at) VALUES('ljob','live',1,'queued',?,?,?)`, []any{at, at, at}},
		{`INSERT INTO jobs(id,source_id,source_revision,status,attempts,created_at,updated_at,available_at,candidates_json,critic_status) VALUES('cjob','crit',1,'retry',1,?,?,?,?,'pending')`, []any{at, at, at, batch}},
		{`INSERT INTO job_attempts(token,job_id,number,started_at,reserved_micros,state) VALUES('attempt','fjob',1,?,500,'active')`, []any{at}},
		{`INSERT INTO quizzes VALUES('quiz','src',1,0,?,'job',0)`, []any{at}},
		{`INSERT INTO quizzes VALUES('fresh','src',1,0,?,'job',1)`, []any{at}},
		{`INSERT INTO quiz_versions VALUES('quiz',1,?,'old-model','old-prompt',?)`, []any{quizA, at}},
		{`INSERT INTO quiz_versions VALUES('fresh',1,?,'old-model','old-prompt',?)`, []any{quizB, at}},
		{`INSERT INTO schedules VALUES('quiz',2,?,?,?)`, []any{after, reviewed.Due.UnixMilli(), learning.Algorithm}},
		{`INSERT INTO schedules VALUES('fresh',1,?,?,?)`, []any{fresh, at, learning.Algorithm}},
		{`INSERT INTO presentations VALUES('p1','quiz',1,1,?,?,'TLS','correct',0,1,3,?,?,'r1')`, []any{quizA, at, reviewed.Due.UnixMilli(), at}},
		{`INSERT INTO review_events VALUES('r1','p1',?,'TLS','correct',3,0,?,?,?,?,?,1,2,'exact-v1')`, []any{quizA, at, reviewed.Due.UnixMilli(), learning.Algorithm, fresh, after}},
		{`INSERT INTO presentations(id,quiz_id,content_version,schedule_version,snapshot,created_at,due_at) VALUES('bridge-p','fresh',1,1,?,?,?)`, []any{quizB, at, at}},
		{`UPDATE review_session SET current_id='bridge-p'`, nil},
		{`INSERT INTO foundation_requests VALUES('fdone','quiz',1)`, nil},
		{`INSERT INTO foundation_requests VALUES('fjob','fresh',1)`, nil},
		{`INSERT INTO foundation_units VALUES('unit-a',1,'Trust anchors are roots the client already trusts.','foundation','model')`, nil},
		{`INSERT INTO foundation_units VALUES('unit-b',1,'A certificate chain links a server to a trust anchor.','foundation','model')`, nil},
		{`INSERT INTO foundation_bundles VALUES('bundle','fdone','src',1,'quiz',1,'old-model','old-prompt','note',?)`, []any{at}},
		{`INSERT INTO foundation_materials VALUES('mat',1,'bundle',0,'{"title":"Trust anchors"}','model')`, nil},
		{`INSERT INTO foundation_links VALUES('mat',1,'unit-a',1,'teaches','model')`, nil},
		{`INSERT INTO foundation_bridges(id,presentation_id,source_revision,job_id,phase,created_at) VALUES('bridge','bridge-p',1,'fjob','waiting',?)`, []any{at}},
		{`INSERT INTO foundation_interactions VALUES('seen','bridge','mat',1,'read','','',?)`, []any{at}},
		{`INSERT INTO concepts VALUES('unit-a','unit-a','Trust anchors are roots the client already trusts.',?)`, []any{at}},
		{`INSERT INTO concepts VALUES('unit-b','unit-b','A certificate chain links a server to a trust anchor.',?)`, []any{at}},
		{`INSERT INTO concept_prerequisites VALUES('unit-b','unit-a',?)`, []any{at}},
		{`INSERT INTO "references" VALUES('mat','Trust anchors','{"title":"Trust anchors"}','explanation','',?)`, []any{at}},
		{`INSERT INTO concept_references VALUES('unit-a','mat',?)`, []any{at}},
		{`INSERT INTO concept_quizzes VALUES('unit-a','quiz',?)`, []any{at}},
		{`INSERT INTO concept_quizzes VALUES('unit-b','fresh',?)`, []any{at}},
	}
	for _, st := range statements {
		if _, err = db.ExecContext(ctx, st.sql, st.args...); err != nil {
			t.Fatalf("%s: %v", st.sql, err)
		}
	}
	history := map[string][]string{}
	for _, q := range historyQueries {
		history[q] = dumpRows(t, db, q)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	s := openAt(t, path, &now)
	var version int
	if err = s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("user_version=%d %v", version, err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = CheckSchema(ctx, tx, SchemaVersion); err != nil {
		t.Fatalf("migrated schema differs from a fresh v5 schema: %v", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	for _, q := range historyQueries {
		if got := dumpRows(t, s.db, q); !reflect.DeepEqual(got, history[q]) {
			t.Fatalf("history changed: %s\nbefore %v\nafter  %v", q, history[q], got)
		}
	}

	// Relations carry over as foundation history; links stop counting as study.
	if got := dumpRows(t, s.db, "SELECT from_id,to_id,kind,origin,retired_at FROM concept_relations"); !reflect.DeepEqual(got, []string{"unit-b unit-a requires foundation 0"}) {
		t.Fatalf("prerequisites did not carry over as foundation relations: %v", got)
	}
	if got := dumpRows(t, s.db, "SELECT DISTINCT origin FROM concepts UNION ALL SELECT DISTINCT role FROM concept_quizzes"); !reflect.DeepEqual(got, []string{"foundation", "foundation"}) {
		t.Fatalf("foundation concepts or links were classified as study material: %v", got)
	}

	// Every source has its goal; archived material stays archived.
	var srcTitle, oldStatus string
	var goals int
	if err = s.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM goals),(SELECT title FROM goals WHERE source_id='src'),(SELECT status FROM goals WHERE source_id='old')`).Scan(&goals, &srcTitle, &oldStatus); err != nil {
		t.Fatal(err)
	}
	if goals != 4 || oldStatus != "archived" || utf8.RuneCountInString(srcTitle) > 120 || strings.ContainsAny(srcTitle, "\r\n") || !strings.HasPrefix(srcTitle, "Certificates and trust A long") || !strings.HasSuffix(srcTitle, "…") {
		t.Fatalf("goal backfill: goals=%d old=%s title=%q", goals, oldStatus, srcTitle)
	}

	// Retired foundation work is canceled with its spend kept as unknown;
	// unstarted legacy work continues through the v5 chain; a saved candidate
	// batch keeps its kind so only its critic resumes.
	if got := dumpRows(t, s.db, "SELECT id,kind,status FROM jobs WHERE id IN ('fjob','ljob','cjob') ORDER BY id"); !reflect.DeepEqual(got, []string{"cjob quizzes retry", "fjob quizzes canceled", "ljob plan queued"}) {
		t.Fatalf("live jobs after upgrade: %v", got)
	}
	var attemptState string
	var attemptReserved int64
	if err = s.db.QueryRowContext(ctx, "SELECT state,reserved_micros FROM job_attempts WHERE token='attempt'").Scan(&attemptState, &attemptReserved); err != nil || attemptState != "unknown" || attemptReserved != 500 {
		t.Fatalf("foundation attempt: %s %d %v", attemptState, attemptReserved, err)
	}

	// Stream: the retired detour is gone, and legacy questions come back as
	// plain questions with no concept chip or intro.
	state, err := s.Review(ctx)
	if err != nil || state.Current == nil || state.Current.ID == "bridge-p" || state.Intro != nil || state.Current.Concept != nil || state.CurrentConcept != nil {
		t.Fatalf("stream after upgrade: %+v %v", state, err)
	}
	if _, err = s.AcknowledgeIntro(ctx, "unit-a", "intro-foundation", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a foundation concept could be introduced: %v", err)
	}

	// Map: foundation concepts are not listed, reusable, or openable; their
	// questions stay reachable under unmapped material.
	m, err := s.Map(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range m.Goals {
		if len(g.Concepts) != 0 {
			t.Fatalf("goal %q lists concepts after an upgrade with only foundation concepts: %+v", g.Goal.Title, g.Concepts)
		}
	}
	if len(m.Unmapped) != 1 || m.Unmapped[0].ID != "src" {
		t.Fatalf("legacy questions are not reachable as unmapped material: %+v", m.Unmapped)
	}
	if reuse, err := s.SearchConcepts(ctx, "trust anchors", 12); err != nil || len(reuse) != 0 {
		t.Fatalf("a foundation concept was offered for reuse: %+v %v", reuse, err)
	}
	if _, err = s.ConceptPage(ctx, "unit-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foundation concept page opened: %v", err)
	}
	summary, err := s.Summary(ctx)
	if err != nil || summary.Concepts != 0 || summary.Goals != 3 {
		t.Fatalf("summary counted foundation concepts or archived goals: %+v %v", summary, err)
	}
}
