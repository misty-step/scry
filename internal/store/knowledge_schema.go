package store

const schemaV2 = `
DROP INDEX one_live_source_job;
ALTER TABLE jobs ADD COLUMN kind TEXT NOT NULL DEFAULT 'capture' CHECK(kind IN ('capture','enrich','bridge','expand'));
ALTER TABLE jobs ADD COLUMN goal_id TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN goal_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN target_material_id TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN target_material_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN target_presentation_id TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN target_presentation_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN suggestion_id TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN observation_id TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN parent_job_id TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN retry_root_id TEXT NOT NULL DEFAULT '';
ALTER TABLE jobs ADD COLUMN context_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(context_json));
ALTER TABLE jobs ADD COLUMN new_materials INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN reused_materials INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN new_quizzes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN coverage_json TEXT NOT NULL DEFAULT '{"kind":"unmapped","complete":false,"missing":["Historical generation has no authored knowledge coverage"]}' CHECK(json_valid(coverage_json));
CREATE UNIQUE INDEX one_live_knowledge_job ON jobs(source_id,source_revision,goal_id,goal_revision,kind,target_material_id,target_material_version,target_presentation_id,suggestion_id) WHERE status IN ('queued','running','retry','paused');
CREATE TABLE goals (
 id TEXT PRIMARY KEY, source_id TEXT NOT NULL UNIQUE REFERENCES sources(id), source_revision INTEGER NOT NULL,
 title TEXT NOT NULL, revision INTEGER NOT NULL CHECK(revision>0), archived INTEGER NOT NULL DEFAULT 0 CHECK(archived IN(0,1)),
 settings_json TEXT NOT NULL CHECK(json_valid(settings_json)), coverage_json TEXT NOT NULL CHECK(json_valid(coverage_json)),
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 FOREIGN KEY(source_id,source_revision) REFERENCES source_revisions(source_id,revision)
) STRICT;
CREATE TABLE goal_versions (
 goal_id TEXT NOT NULL REFERENCES goals(id), revision INTEGER NOT NULL, snapshot TEXT NOT NULL CHECK(json_valid(snapshot)), created_at INTEGER NOT NULL,
 PRIMARY KEY(goal_id,revision)
) STRICT;
CREATE TABLE knowledge_units (
 id TEXT PRIMARY KEY, version INTEGER NOT NULL CHECK(version>0), archived INTEGER NOT NULL DEFAULT 0 CHECK(archived IN(0,1)), created_at INTEGER NOT NULL
) STRICT;
CREATE TABLE unit_versions (
 unit_id TEXT NOT NULL REFERENCES knowledge_units(id), version INTEGER NOT NULL CHECK(version>0), statement TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('foundation','concept','composition','procedure','exact_text')),
 provenance_json TEXT NOT NULL CHECK(json_valid(provenance_json)), created_at INTEGER NOT NULL, PRIMARY KEY(unit_id,version)
) STRICT;
CREATE TABLE goal_units (
 goal_id TEXT NOT NULL REFERENCES goals(id), unit_id TEXT NOT NULL, unit_version INTEGER NOT NULL, origin_job_id TEXT REFERENCES jobs(id), created_at INTEGER NOT NULL,
 PRIMARY KEY(goal_id,unit_id,unit_version), FOREIGN KEY(unit_id,unit_version) REFERENCES unit_versions(unit_id,version)
) STRICT;
CREATE TABLE knowledge_relations (
 id TEXT PRIMARY KEY, version INTEGER NOT NULL CHECK(version>0), archived INTEGER NOT NULL DEFAULT 0 CHECK(archived IN(0,1)), created_at INTEGER NOT NULL
) STRICT;
CREATE TABLE relation_versions (
 relation_id TEXT NOT NULL REFERENCES knowledge_relations(id), version INTEGER NOT NULL CHECK(version>0),
 from_id TEXT NOT NULL, from_version INTEGER NOT NULL, to_id TEXT NOT NULL, to_version INTEGER NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('prerequisite','composition','contrast')), proposed INTEGER NOT NULL CHECK(proposed IN(0,1)),
 provenance_json TEXT NOT NULL CHECK(json_valid(provenance_json)), created_at INTEGER NOT NULL,
 PRIMARY KEY(relation_id,version), CHECK(from_id<>to_id),
 FOREIGN KEY(from_id,from_version) REFERENCES unit_versions(unit_id,version), FOREIGN KEY(to_id,to_version) REFERENCES unit_versions(unit_id,version)
) STRICT;
CREATE TABLE materials (
 id TEXT PRIMARY KEY, source_id TEXT NOT NULL REFERENCES sources(id), version INTEGER NOT NULL CHECK(version>0),
 kind TEXT NOT NULL CHECK(kind IN('quiz','explanation','worked_example','diagram','article','video')),
 archived INTEGER NOT NULL DEFAULT 0 CHECK(archived IN(0,1)), created_at INTEGER NOT NULL,
 first_presented_at INTEGER NOT NULL DEFAULT 0, origin_job_id TEXT REFERENCES jobs(id), origin_index INTEGER NOT NULL, UNIQUE(origin_job_id,kind,origin_index)
) STRICT;
CREATE TABLE material_versions (
 material_id TEXT NOT NULL REFERENCES materials(id), version INTEGER NOT NULL CHECK(version>0),
 content TEXT NOT NULL CHECK(json_valid(content)), quiz_id TEXT REFERENCES quizzes(id), quiz_version INTEGER,
 provenance_json TEXT NOT NULL CHECK(json_valid(provenance_json)), created_at INTEGER NOT NULL,
 PRIMARY KEY(material_id,version), FOREIGN KEY(quiz_id,quiz_version) REFERENCES quiz_versions(quiz_id,version)
) STRICT;
CREATE TABLE material_links (
 id TEXT PRIMARY KEY, material_id TEXT NOT NULL, material_version INTEGER NOT NULL, unit_id TEXT NOT NULL, unit_version INTEGER NOT NULL,
 role TEXT NOT NULL CHECK(role IN('assesses','teaches','assumes','mentions')), provenance_json TEXT NOT NULL CHECK(json_valid(provenance_json)),
 UNIQUE(material_id,material_version,unit_id,unit_version,role),
 FOREIGN KEY(material_id,material_version) REFERENCES material_versions(material_id,version), FOREIGN KEY(unit_id,unit_version) REFERENCES unit_versions(unit_id,version)
) STRICT;
CREATE INDEX coverage_unit ON material_links(unit_id,unit_version,material_id,material_version);
CREATE TABLE goal_materials (
 goal_id TEXT NOT NULL REFERENCES goals(id), material_id TEXT NOT NULL, material_version INTEGER NOT NULL,
 origin_job_id TEXT REFERENCES jobs(id), created_at INTEGER NOT NULL,
 PRIMARY KEY(goal_id,material_id,material_version), FOREIGN KEY(material_id,material_version) REFERENCES material_versions(material_id,version)
) STRICT;
CREATE TABLE knowledge_corrections (
 id TEXT PRIMARY KEY, kind TEXT NOT NULL CHECK(kind IN('unit','material','coverage','relation','evidence','goal')), entity_id TEXT NOT NULL,
 before_version INTEGER NOT NULL, after_version INTEGER NOT NULL, reason TEXT NOT NULL, created_at INTEGER NOT NULL
) STRICT;
CREATE TABLE suggestions (
 id TEXT PRIMARY KEY, goal_id TEXT NOT NULL REFERENCES goals(id), goal_revision INTEGER NOT NULL, kind TEXT NOT NULL CHECK(kind IN('advance','lateral')),
 title TEXT NOT NULL, reason TEXT NOT NULL, unit_ids_json TEXT NOT NULL CHECK(json_valid(unit_ids_json)), material_ids_json TEXT NOT NULL CHECK(json_valid(material_ids_json)),
 unit_versions_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(unit_versions_json)), material_versions_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(material_versions_json)),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN('pending','accepted','declined','superseded','obsolete')), origin_job_id TEXT NOT NULL REFERENCES jobs(id), created_at INTEGER NOT NULL,
 supersedes_id TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE TABLE plan_decisions (
 id TEXT PRIMARY KEY, goal_id TEXT NOT NULL REFERENCES goals(id), goal_revision INTEGER NOT NULL, kind TEXT NOT NULL,
 reason TEXT NOT NULL, policy TEXT NOT NULL, material_id TEXT NOT NULL DEFAULT '', material_version INTEGER NOT NULL DEFAULT 0,
 suggestion_id TEXT NOT NULL DEFAULT '', before_json TEXT NOT NULL CHECK(json_valid(before_json)), after_json TEXT NOT NULL CHECK(json_valid(after_json)),
 evidence_ids_json TEXT NOT NULL CHECK(json_valid(evidence_ids_json)), unit_versions_json TEXT NOT NULL CHECK(json_valid(unit_versions_json)),
 reconsider_at INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, undone_at INTEGER NOT NULL DEFAULT 0, undo_of TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX plan_material ON plan_decisions(material_id,material_version,reconsider_at);
CREATE TABLE bridges (
 id TEXT PRIMARY KEY, goal_id TEXT NOT NULL REFERENCES goals(id), target_presentation_id TEXT NOT NULL REFERENCES presentations(id),
 target_material_id TEXT NOT NULL, target_material_version INTEGER NOT NULL,
 status TEXT NOT NULL CHECK(status IN('pending','active','ready_return','returned','unavailable')),
 reason TEXT NOT NULL, path_json TEXT NOT NULL CHECK(json_valid(path_json)), path_versions_json TEXT NOT NULL CHECK(json_valid(path_versions_json)), position INTEGER NOT NULL DEFAULT 0 CHECK(position>=0),
 job_id TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 FOREIGN KEY(target_material_id,target_material_version) REFERENCES material_versions(material_id,version)
) STRICT;
CREATE UNIQUE INDEX one_active_bridge ON bridges((1)) WHERE status IN('pending','active','ready_return');
CREATE TABLE presentations_v2 (
 id TEXT PRIMARY KEY, quiz_id TEXT REFERENCES quizzes(id), content_version INTEGER NOT NULL,
 schedule_version INTEGER NOT NULL, snapshot TEXT NOT NULL CHECK(json_valid(snapshot)), created_at INTEGER NOT NULL,
 answer TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL DEFAULT '', assisted INTEGER NOT NULL DEFAULT 0 CHECK(assisted IN(0,1)),
 graded INTEGER NOT NULL DEFAULT 0 CHECK(graded IN(0,1)), rating INTEGER NOT NULL DEFAULT 0,
 due_at INTEGER NOT NULL, reviewed_at INTEGER NOT NULL DEFAULT 0, review_id TEXT NOT NULL DEFAULT '',
 kind TEXT NOT NULL CHECK(kind IN('quiz','reference')), material_id TEXT NOT NULL, material_version INTEGER NOT NULL,
 material_snapshot TEXT NOT NULL CHECK(json_valid(material_snapshot)), bridge_id TEXT NOT NULL DEFAULT '', planning_reason TEXT NOT NULL DEFAULT '',
 practice INTEGER NOT NULL DEFAULT 0 CHECK(practice IN(0,1)),
 goal_id TEXT NOT NULL REFERENCES goals(id), goal_revision INTEGER NOT NULL CHECK(goal_revision>0),
 goal_pin_origin TEXT NOT NULL CHECK(goal_pin_origin IN('selected-plan','migration-v1-origin-source')),
 FOREIGN KEY(goal_id,goal_revision) REFERENCES goal_versions(goal_id,revision),
 FOREIGN KEY(quiz_id,content_version) REFERENCES quiz_versions(quiz_id,version),
 FOREIGN KEY(material_id,material_version) REFERENCES material_versions(material_id,version)
) STRICT;
CREATE TABLE interactions (
 id TEXT PRIMARY KEY, presentation_id TEXT NOT NULL REFERENCES presentations(id), material_id TEXT NOT NULL, material_version INTEGER NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN('review','practice','continue','bridge_request','return','inspection','assistance')), outcome TEXT NOT NULL, assisted INTEGER NOT NULL CHECK(assisted IN(0,1)),
 at INTEGER NOT NULL, snapshot TEXT NOT NULL CHECK(json_valid(snapshot)), reason TEXT NOT NULL DEFAULT '',
 answer TEXT NOT NULL DEFAULT '', mode TEXT NOT NULL DEFAULT '', practice INTEGER NOT NULL DEFAULT 0 CHECK(practice IN(0,1)),
 FOREIGN KEY(material_id,material_version) REFERENCES material_versions(material_id,version)
) STRICT;
CREATE UNIQUE INDEX one_reference_continue ON interactions(presentation_id) WHERE kind='continue';
CREATE TABLE inspection_events (
 id TEXT PRIMARY KEY, kind TEXT NOT NULL, entity_id TEXT NOT NULL, source_ids_json TEXT NOT NULL CHECK(json_valid(source_ids_json)),
 source_versions_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(source_versions_json)), material_versions_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(material_versions_json)),
 unit_versions_json TEXT NOT NULL CHECK(json_valid(unit_versions_json)), at INTEGER NOT NULL, expires_at INTEGER NOT NULL
) STRICT;
CREATE TABLE estimate_records (
 id TEXT PRIMARY KEY, unit_id TEXT NOT NULL, unit_version INTEGER NOT NULL, mode TEXT NOT NULL, as_of INTEGER NOT NULL, target_at INTEGER NOT NULL,
 policy TEXT NOT NULL, estimate_json TEXT NOT NULL CHECK(json_valid(estimate_json)), created_at INTEGER NOT NULL,
 FOREIGN KEY(unit_id,unit_version) REFERENCES unit_versions(unit_id,version)
) STRICT;
CREATE TABLE job_changes (
 id TEXT PRIMARY KEY, job_id TEXT NOT NULL REFERENCES jobs(id), before_goal_revision INTEGER NOT NULL,
 after_goal_revision INTEGER NOT NULL, reason TEXT NOT NULL, created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX interactions_activity_order ON interactions(at DESC,id DESC);
CREATE INDEX interactions_material_time ON interactions(material_id,material_version,at);
CREATE INDEX estimates_unit_time ON estimate_records(unit_id,unit_version,mode,as_of DESC);
CREATE INDEX knowledge_correction_entity ON knowledge_corrections(entity_id,created_at);
CREATE INDEX correction_review_time ON corrections(review_id,created_at);
CREATE INDEX relation_from_time ON relation_versions(from_id,from_version,created_at);
CREATE INDEX relation_to_time ON relation_versions(to_id,to_version,created_at);
CREATE INDEX inspection_expiry ON inspection_events(expires_at);
CREATE INDEX material_source_availability ON materials(source_id,archived,created_at,id);
CREATE INDEX goal_unit_membership ON goal_units(unit_id,unit_version,goal_id);
CREATE INDEX goal_material_membership ON goal_materials(material_id,material_version,goal_id);
CREATE INDEX jobs_source_scope ON jobs(source_id,source_revision,kind,status);
CREATE INDEX jobs_retry_chain ON jobs(retry_root_id,parent_job_id,created_at);
CREATE INDEX jobs_goal_state ON jobs(goal_id,goal_revision,status);
CREATE INDEX attempts_job_authority ON job_attempts(job_id,state,number);
CREATE INDEX presentation_bridge_state ON presentations_v2(bridge_id,graded,kind,created_at);
CREATE INDEX inspection_entity_expiry ON inspection_events(kind,entity_id,expires_at);
CREATE INDEX presentation_selected_goal_time ON presentations_v2(goal_id,created_at);
CREATE INDEX inspection_activity_order ON inspection_events(at DESC,id DESC);
CREATE INDEX jobs_observation_scope ON jobs(observation_id,goal_id,goal_revision,target_material_id,target_material_version);
CREATE TRIGGER immutable_finalized_job_result BEFORE UPDATE ON jobs WHEN OLD.result_json IS NOT NULL AND NEW.result_json IS NOT OLD.result_json BEGIN SELECT RAISE(ABORT,'finalized generation payloads are immutable'); END;
`

var immutableKnowledgeTables = []string{"source_revisions", "corrections", "goal_versions", "unit_versions", "relation_versions", "material_versions", "material_links", "knowledge_corrections", "interactions", "inspection_events", "estimate_records", "job_changes"}
