package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode"

	"github.com/misty-step/scry/internal/learning"
)

var schemaShapes [3]struct {
	once  sync.Once
	shape map[string]string
	err   error
}

func canonicalDDL(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '"' || r == '`' || r == '[' || r == ']' {
			return -1
		}
		return unicode.ToLower(r)
	}, strings.TrimSuffix(text, ";"))
}

func schemaShape(ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT type,name,tbl_name,sql FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	shape := map[string]string{}
	for rows.Next() {
		var kind, name, table, text string
		if err = rows.Scan(&kind, &name, &table, &text); err != nil {
			return nil, err
		}
		shape[kind+"/"+name+"/"+table] = canonicalDDL(text)
	}
	return shape, rows.Err()
}

func expectedSchema(version int) (map[string]string, error) {
	cached := &schemaShapes[version]
	cached.once.Do(func() {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			cached.err = err
			return
		}
		defer db.Close()
		db.SetMaxOpenConns(1)
		if _, err = db.Exec(schemaV1); err != nil {
			cached.err = err
			return
		}
		if version == 2 {
			if _, err = db.Exec(schemaV2); err != nil {
				cached.err = err
				return
			}
			if _, err = db.Exec("DROP TABLE presentations; ALTER TABLE presentations_v2 RENAME TO presentations;"); err != nil {
				cached.err = err
				return
			}
			for _, table := range immutableKnowledgeTables {
				for _, action := range []string{"UPDATE", "DELETE"} {
					name := "immutable_" + table + "_" + strings.ToLower(action)
					if _, err = db.Exec(fmt.Sprintf("CREATE TRIGGER %s BEFORE %s ON %s BEGIN SELECT RAISE(ABORT,'knowledge history is immutable'); END", name, action, table)); err != nil {
						cached.err = err
						return
					}
				}
			}
		}
		cached.shape, cached.err = schemaShape(context.Background(), db)
	})
	return cached.shape, cached.err
}

func validateCanonicalSchema(ctx context.Context, tx *sql.Tx, version int) error {
	expected, err := expectedSchema(version)
	if err != nil {
		return err
	}
	actual, err := schemaShape(ctx, tx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, actual) {
		for key, value := range expected {
			if actual[key] != value {
				return fmt.Errorf("%w: schema definition mismatch for %s", ErrInvalid, key)
			}
		}
		return fmt.Errorf("%w: database contains unexpected schema objects", ErrInvalid)
	}
	return nil
}

func validateKnowledgeJSON(ctx context.Context, tx *sql.Tx) error {
	checks := []string{
		`SELECT count(*) FROM materials m JOIN material_versions v ON v.material_id=m.id WHERE json_type(v.content)<>'object' OR json_extract(v.content,'$.id') IS NOT m.id OR json_extract(v.content,'$.version') IS NOT v.version OR json_extract(v.content,'$.kind') IS NOT m.kind OR json_extract(v.content,'$.source_id') IS NOT m.source_id`,
		`SELECT count(*) FROM material_versions WHERE (quiz_id IS NULL)<>(quiz_version IS NULL) OR json_type(provenance_json)<>'object' OR json_type(provenance_json,'$.basis') IS NOT 'text' OR json_type(provenance_json,'$.created_at') IS NOT 'integer'`,
		`SELECT count(*) FROM unit_versions WHERE json_type(provenance_json)<>'object' OR json_type(provenance_json,'$.reason') IS NOT 'text' OR length(trim(statement))=0 OR version<1`,
		`SELECT count(*) FROM relation_versions WHERE json_type(provenance_json)<>'object' OR json_type(provenance_json,'$.reason') IS NOT 'text'`,
		`SELECT count(*) FROM material_links WHERE json_type(provenance_json)<>'object' OR json_type(provenance_json,'$.reason') IS NOT 'text'`,
		`SELECT count(*) FROM goals WHERE json_type(settings_json)<>'object' OR json_type(settings_json,'$.time_budget_seconds') IS NOT 'integer' OR json_extract(settings_json,'$.time_budget_seconds') NOT BETWEEN 30 AND 3600 OR json_type(settings_json,'$.new_assessments_per_day') IS NOT 'integer' OR json_extract(settings_json,'$.new_assessments_per_day') NOT BETWEEN 0 AND 100 OR json_extract(settings_json,'$.focus') NOT IN('goal','foundation','practice') OR json_type(coverage_json,'$.kind') IS NOT 'text' OR json_type(coverage_json,'$.complete') NOT IN('true','false') OR json_type(coverage_json,'$.missing') IS NOT 'array'`,
		`SELECT count(*) FROM goal_versions WHERE json_type(snapshot)<>'object' OR json_extract(snapshot,'$.id') IS NOT goal_id OR json_extract(snapshot,'$.revision') IS NOT revision`,
		`SELECT count(*) FROM bridges WHERE json_type(path_json) IS NOT 'array' OR json_type(path_versions_json) IS NOT 'array' OR json_array_length(path_json)<>json_array_length(path_versions_json) OR position>json_array_length(path_json)`,
		`SELECT count(*) FROM bridges b,json_each(b.path_json) p WHERE p.type<>'text' OR NOT EXISTS(SELECT 1 FROM material_versions v WHERE v.material_id=p.value AND v.version=json_extract(b.path_versions_json,'$['||p.key||']'))`,
		`SELECT count(*) FROM inspection_events WHERE json_type(source_ids_json) IS NOT 'array' OR json_type(unit_versions_json) IS NOT 'array' OR json_type(source_versions_json) IS NOT 'array' OR json_type(material_versions_json) IS NOT 'array' OR expires_at<at OR expires_at-at<>1800000`,
		`SELECT count(*) FROM suggestions WHERE json_type(unit_ids_json) IS NOT 'array' OR json_type(material_ids_json) IS NOT 'array'`,
		`SELECT count(*) FROM plan_decisions WHERE json_type(before_json) IS NOT 'object' OR json_type(after_json) IS NOT 'object' OR json_type(evidence_ids_json) IS NOT 'array' OR json_type(unit_versions_json) IS NOT 'array'`,
		`SELECT count(*) FROM presentations WHERE json_type(material_snapshot) IS NOT 'object' OR json_extract(material_snapshot,'$.id') IS NOT material_id OR json_extract(material_snapshot,'$.version') IS NOT material_version`,
		`SELECT count(*) FROM presentations p JOIN goals g ON g.id=p.goal_id WHERE (p.goal_pin_origin='selected-plan' AND (json_extract(p.material_snapshot,'$.goal_id') IS NOT p.goal_id OR json_extract(p.material_snapshot,'$.goal_revision') IS NOT p.goal_revision)) OR (p.goal_pin_origin='migration-v1-origin-source' AND json_extract(p.material_snapshot,'$.source_id') IS NOT g.source_id)`,
		`SELECT count(*) FROM bridges b JOIN presentations p ON p.id=b.target_presentation_id WHERE b.goal_id<>p.goal_id OR b.target_material_id<>p.material_id OR b.target_material_version<>p.material_version`,
		`SELECT count(*) FROM jobs j WHERE NOT EXISTS(SELECT 1 FROM goals g JOIN goal_versions v ON v.goal_id=g.id WHERE g.id=j.goal_id AND g.source_id=j.source_id AND v.revision=j.goal_revision) OR (j.target_material_id='' AND j.target_material_version<>0) OR (j.target_material_id<>'' AND NOT EXISTS(SELECT 1 FROM material_versions v WHERE v.material_id=j.target_material_id AND v.version=j.target_material_version)) OR (j.target_presentation_id='' AND j.target_presentation_version<>0) OR (j.target_presentation_id<>'' AND NOT EXISTS(SELECT 1 FROM presentations p WHERE p.id=j.target_presentation_id AND p.goal_id=j.goal_id AND p.material_id=j.target_material_id AND p.material_version=j.target_material_version AND p.material_version=j.target_presentation_version))`,
		`SELECT count(*) FROM jobs j WHERE (j.observation_id<>'' AND (j.kind<>'enrich' OR NOT EXISTS(SELECT 1 FROM review_events e WHERE e.id=j.observation_id AND e.presentation_id=j.target_presentation_id))) OR (j.kind='enrich' AND ((j.target_material_id='')<>(j.observation_id=''))) OR (j.kind='bridge' AND j.target_presentation_id='')`,
		`SELECT count(*) FROM inspection_events i WHERE i.kind='delivery' AND NOT EXISTS(SELECT 1 FROM presentations p WHERE p.id=i.entity_id AND (p.kind='reference' OR p.graded=1))`,
		`SELECT count(*) FROM interactions i LEFT JOIN review_events e ON e.id=i.id WHERE (i.kind='review' AND (e.id IS NULL OR e.presentation_id<>i.presentation_id OR e.reviewed_at<>i.at OR e.answer<>i.answer OR e.assisted<>i.assisted OR e.outcome<>i.outcome)) OR (i.kind='practice' AND (e.id IS NOT NULL OR i.practice<>1))`,
		`SELECT count(*) FROM estimate_records WHERE json_type(estimate_json) IS NOT 'object' OR json_type(estimate_json,'$.policy') IS NOT 'text' OR json_type(estimate_json,'$.unit') IS NOT 'object'`,
	}
	for _, query := range checks {
		var count int
		if err := tx.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: corrupt knowledge JSON/pinned references (%d rows)", ErrInvalid, count)
		}
	}
	rows, err := tx.QueryContext(ctx, "SELECT unit_versions_json,material_versions_json,source_versions_json FROM inspection_events")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a, b, c string
		if err = rows.Scan(&a, &b, &c); err != nil {
			return err
		}
		for _, text := range []string{a, b, c} {
			var versions []learning.UnitVersion
			if err = json.Unmarshal([]byte(text), &versions); err != nil {
				return err
			}
			for _, v := range versions {
				if v.ID == "" || v.Version < 1 {
					return fmt.Errorf("%w: invalid inspection version scope", ErrInvalid)
				}
			}
		}
	}
	return rows.Err()
}

func (s *Store) checkSchemaCookie(ctx context.Context, tx *sql.Tx) error {
	var version, app, cookie int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&app); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&cookie); err != nil {
		return err
	}
	if version != SchemaVersion || app != ApplicationID || cookie != s.schemaCookie {
		return fmt.Errorf("%w: database schema changed since validated open; no job claimed", ErrInvalid)
	}
	return nil
}
