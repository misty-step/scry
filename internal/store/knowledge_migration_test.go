package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/scry/internal/learning"
)

func populatedV1(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "populated-v1.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(schemaV1); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	at := now.UnixMilli()
	must := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	must("INSERT INTO sources(id,text,kind,revision,created_at) VALUES('source-one','Historical synthetic source','topic',1,?)", at)
	must("INSERT INTO source_revisions VALUES('source-one',1,'Historical synthetic source','topic',?)", at)
	must("INSERT INTO sources(id,text,kind,revision,created_at) VALUES('pending-source','Historical pending topic','topic',1,?)", at)
	must("INSERT INTO source_revisions VALUES('pending-source',1,'Historical pending topic','topic',?)", at)
	must("INSERT INTO sources(id,text,kind,revision,archived,created_at) VALUES('archived-source','Retired synthetic source','topic',1,1,?)", at)
	must("INSERT INTO source_revisions VALUES('archived-source',1,'Retired synthetic source','topic',?)", at)
	rawResult := "{\n  \"quizzes\": [], \"partial\": true, \"note\": \"Original v1 receipt stays byte-for-byte\"\n}"
	must(`INSERT INTO jobs(id,source_id,source_revision,status,model,prompt_version,attempts,created_at,updated_at,available_at,published,result_json) VALUES('historical-job','source-one',1,'partial','original-provider','original-prompt',1,?,?,?,1,?)`, at, at, at, rawResult)
	must("INSERT INTO job_attempts(token,job_id,number,started_at,finished_at,reserved_micros,cost_micros,state,finish_hash) VALUES('settled-token','historical-job',1,?,?,100,37,'settled','original-finish-hash')", at, at)
	must(`INSERT INTO jobs(id,source_id,source_revision,status,attempts,created_at,updated_at,available_at,lease_token,lease_until) VALUES('inflight-job','pending-source',1,'running',2,?,?,?,'active-token',?)`, at, at, at, at+60000)
	must("INSERT INTO job_attempts(token,job_id,number,started_at,finished_at,reserved_micros,state) VALUES('unknown-token','inflight-job',1,?,?,100,'unknown')", at-60000, at-30000)
	must("INSERT INTO job_attempts(token,job_id,number,started_at,reserved_micros,state) VALUES('active-token','inflight-job',2,?,100,'active')", at)
	must("INSERT INTO quizzes(id,source_id,version,created_at,origin_job_id,origin_index) VALUES('historical-quiz','source-one',2,?,'historical-job',0)", at)
	old := authoredChoice("Which original historical alternative was shown?")
	oldJSON, _ := marshal(old)
	changed := old
	changed.Prompt = "Which corrected historical alternative will be shown?"
	changedJSON, _ := marshal(changed)
	must("INSERT INTO quiz_versions VALUES('historical-quiz',1,?,'original-provider','original-prompt',?)", oldJSON, at)
	must("INSERT INTO quiz_versions VALUES('historical-quiz',2,?,'','manual-edit',?)", changedJSON, at+1000)
	beforeCard := learning.NewCard(now)
	afterCard, err := learning.Schedule(beforeCard, 3, now)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := marshal(beforeCard)
	after, _ := marshal(afterCard)
	must("INSERT INTO schedules VALUES('historical-quiz',2,?,?,?)", after, afterCard.Due.UnixMilli(), learning.Algorithm)
	quiz := Quiz{ID: "historical-quiz", SourceID: "source-one", Version: 1, Kind: old.Kind, Prompt: old.Prompt, Answer: old.Answer, Explanation: old.Explanation, Basis: old.Basis, Choices: old.Choices, DueAt: at}
	rawSnapshot, _ := json.MarshalIndent(quiz, "", "  ")
	must(`INSERT INTO presentations(id,quiz_id,content_version,schedule_version,snapshot,created_at,answer,outcome,graded,rating,due_at,reviewed_at,review_id) VALUES('held-occurrence','historical-quiz',1,1,?,?,'second','correct',1,3,?,?,'actual-review')`, string(rawSnapshot), at, afterCard.Due.UnixMilli(), at)
	must("UPDATE review_session SET current_id='held-occurrence' WHERE singleton=1")
	must(`INSERT INTO review_events VALUES('actual-review','held-occurrence',?,'second','correct',3,0,?,?,?, ?,?,1,2)`, string(rawSnapshot), at, afterCard.Due.UnixMilli(), learning.Algorithm, before, after)
	must("INSERT INTO corrections(id,review_id,note,reset,created_at) VALUES('actual-dispute','actual-review','Historical issue retained without a reset',0,?)", at+2000)
	legacyReceipt := map[string]any{
		"id": "held-occurrence", "quiz": quiz, "answer": "second", "outcome": "correct", "assisted": false, "graded": true,
		"disputed": false, "rating": 3, "due_at": afterCard.Due.UnixMilli(), "reviewed_at": at, "review_id": "actual-review",
	}
	receiptJSON, _ := json.MarshalIndent(legacyReceipt, "", "  ")
	receiptHash := payloadHash(struct {
		Presentation, Answer string
		Reveal               bool
	}{"held-occurrence", "second", false})
	must("INSERT INTO operations(id,kind,payload_hash,result_id,result_json,created_at) VALUES('original-operation','submit',?,'held-occurrence',?,?)", receiptHash, string(receiptJSON), at)
	must("INSERT INTO backups VALUES('old-backup','/private/historical.db','private/old-key','retained-digest','',?,100,1)", at)
	return db, path
}

type legacyImage struct {
	columns map[string][]string
	keys    map[string][]int
	rows    map[string]map[string][]any
}

func legacyRows(t *testing.T, db *sql.DB, shape *legacyImage) legacyImage {
	t.Helper()
	result := legacyImage{columns: map[string][]string{}, keys: map[string][]int{}, rows: map[string]map[string][]any{}}
	for _, table := range []string{"sources", "source_revisions", "jobs", "job_attempts", "quizzes", "quiz_versions", "schedules", "presentations", "review_session", "review_events", "corrections", "operations", "backups"} {
		if shape != nil {
			result.columns[table] = shape.columns[table]
			result.keys[table] = shape.keys[table]
		} else {
			rows, err := db.Query("PRAGMA table_info(" + table + ")")
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var cid, required, pk int
				var name, kind string
				var def any
				if err = rows.Scan(&cid, &name, &kind, &required, &def, &pk); err != nil {
					t.Fatal(err)
				}
				result.columns[table] = append(result.columns[table], name)
				if pk > 0 {
					result.keys[table] = append(result.keys[table], cid)
				}
			}
			if err = rows.Err(); err != nil {
				t.Fatal(err)
			}
			rows.Close()
		}
		rows, err := db.Query("SELECT " + strings.Join(result.columns[table], ",") + " FROM " + table)
		if err != nil {
			t.Fatal(err)
		}
		result.rows[table] = map[string][]any{}
		for rows.Next() {
			values := make([]any, len(result.columns[table]))
			pointers := make([]any, len(values))
			for index := range values {
				pointers[index] = &values[index]
			}
			if err = rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			keyValues := []any{}
			for _, index := range result.keys[table] {
				keyValues = append(keyValues, values[index])
			}
			key, _ := marshal(keyValues)
			result.rows[table][key] = values
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	return result
}

func TestPopulatedV1MigrationPreservesEveryOriginalColumnAndRawReceipt(t *testing.T) {
	db, path := populatedV1(t)
	before := legacyRows(t, db, nil)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	fileBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateDatabase(context.Background(), path, true); err != nil {
		t.Fatal(err)
	}
	fileAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(fileBefore) != sha256.Sum256(fileAfter) {
		t.Fatal("read-only compatibility preflight changed v1 database")
	}
	if err = ValidateDatabase(context.Background(), path, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("strict current preflight accepted v1: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	after := legacyRows(t, s.db, &before)
	for table, rows := range before.rows {
		for key, row := range rows {
			if !reflect.DeepEqual(row, after.rows[table][key]) {
				t.Fatalf("migration rewrote original %s row %s", table, key)
			}
		}
	}
	var links, actual, versions int
	if err = s.db.QueryRow("SELECT count(*) FROM material_links").Scan(&links); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT count(*) FROM interactions WHERE id='actual-review' AND kind='review'").Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT count(*) FROM material_versions WHERE material_id='historical-quiz'").Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if links != 0 || actual != 1 || versions != 2 {
		t.Fatalf("migration invented coverage or lost exact versions: links=%d events=%d versions=%d", links, actual, versions)
	}
	var enrich, pending int
	if err = s.db.QueryRow("SELECT count(*) FROM jobs WHERE kind='enrich' AND source_id='source-one'").Scan(&enrich); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT count(*) FROM jobs WHERE kind='enrich' AND source_id='pending-source'").Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if enrich != 1 || pending != 0 {
		t.Fatalf("migration duplicated live/unknown external work: eligible=%d pending=%d", enrich, pending)
	}
	retry, err := s.Submit(context.Background(), "held-occurrence", "original-operation", "second", false)
	if err != nil || retry.Kind != "quiz" || retry.Material == nil || retry.Material.Version != 1 || retry.Quiz.Version != 1 || retry.Rating != 3 || retry.Outcome != "correct" || retry.Assisted || retry.ReviewID != "actual-review" {
		t.Fatalf("old submit retry lost its pinned receipt or read metadata: %+v %v", retry, err)
	}
	if err = ValidateDatabase(context.Background(), path, false); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationRejectsCorruptSemanticJSONAndRollsBackWholly(t *testing.T) {
	db, path := populatedV1(t)
	if _, err := db.Exec("INSERT INTO quiz_versions VALUES('historical-quiz',3,'{}','','corrupt-but-valid-json',1); UPDATE quizzes SET version=3 WHERE id='historical-quiz'"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(path); err == nil {
		s.Close()
		t.Fatal("well-formed invalid quiz object migrated")
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var version, goals int
	if err = raw.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err = raw.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name='goals'").Scan(&goals); err != nil {
		t.Fatal(err)
	}
	if version != 1 || goals != 0 {
		t.Fatalf("failed migration left partial schema: version=%d goals=%d", version, goals)
	}
}

func TestForeignFutureAndMissingImmutableFenceFailBeforeTraffic(t *testing.T) {
	for _, change := range []string{"PRAGMA application_id=99", "PRAGMA user_version=3", "DROP TRIGGER immutable_review_update"} {
		t.Run(change, func(t *testing.T) {
			db, path := populatedV1(t)
			if _, err := db.Exec(change); err != nil {
				t.Fatal(err)
			}
			db.Close()
			if err := ValidateDatabase(context.Background(), path, true); err == nil {
				t.Fatal("incompatible shape passed read-only check")
			}
			if s, err := Open(path); err == nil {
				s.Close()
				t.Fatal("incompatible shape opened for traffic")
			}
		})
	}
}

func TestCurrentSnapshotRejectsForgedKnowledgeShapeAndJSON(t *testing.T) {
	for _, change := range []string{"DROP TRIGGER immutable_material_links_update", "UPDATE goals SET settings_json='{}'", "UPDATE material_versions SET content='{}'"} {
		t.Run(change, func(t *testing.T) {
			s, _ := newTestStore(t)
			publishFixture(t, s, authoredChoice("Which content remains recoverable?"))
			_, err := s.db.Exec(change)
			if strings.HasPrefix(change, "UPDATE material_versions") {
				if err == nil {
					t.Fatal("immutable material history accepted replacement")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = ValidateDatabase(context.Background(), s.Path(), false); err == nil {
				t.Fatal("forged current schema/JSON passed compatibility gate")
			}
		})
	}
}
