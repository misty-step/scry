package store

// Schema 5 makes the concept the core identity. It is additive: every earlier
// row, content version, review event, schedule, operation receipt, spend
// record, and foundation/concept record is preserved. Migrated foundation
// units stay as origin='foundation' concepts, hidden from new surfaces.
const schemaV5 = `
ALTER TABLE concepts ADD COLUMN origin TEXT NOT NULL DEFAULT 'foundation' CHECK(origin IN ('foundation','generated','learner'));
ALTER TABLE concepts ADD COLUMN status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','merged','archived'));
ALTER TABLE concepts ADD COLUMN merged_into TEXT NOT NULL DEFAULT '';
ALTER TABLE concepts ADD COLUMN source_id TEXT NOT NULL DEFAULT '';
ALTER TABLE concepts ADD COLUMN updated_at INTEGER NOT NULL DEFAULT 0;
CREATE TABLE concept_relations (
 id TEXT PRIMARY KEY,
 from_id TEXT NOT NULL REFERENCES concepts(id),
 to_id TEXT NOT NULL REFERENCES concepts(id),
 kind TEXT NOT NULL CHECK(kind IN ('requires','part_of','confused_with')),
 origin TEXT NOT NULL CHECK(origin IN ('model','source','learner','foundation')),
 job_id TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL,
 retired_at INTEGER NOT NULL DEFAULT 0,
 CHECK(from_id<>to_id)
) STRICT;
CREATE UNIQUE INDEX one_live_relation ON concept_relations(from_id,to_id,kind) WHERE retired_at=0;
CREATE INDEX relation_target ON concept_relations(to_id,kind);
ALTER TABLE concept_quizzes ADD COLUMN role TEXT NOT NULL DEFAULT 'assesses' CHECK(role IN ('assesses','contrasts','foundation'));
CREATE TABLE goals (
 id TEXT PRIMARY KEY,
 title TEXT NOT NULL,
 source_id TEXT NOT NULL UNIQUE REFERENCES sources(id),
 status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','paused','archived')),
 focus INTEGER NOT NULL DEFAULT 0 CHECK(focus IN (0,1)),
 created_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL
) STRICT;
CREATE TABLE goal_concepts (
 goal_id TEXT NOT NULL REFERENCES goals(id),
 concept_id TEXT NOT NULL REFERENCES concepts(id),
 created_at INTEGER NOT NULL,
 PRIMARY KEY(goal_id,concept_id)
) STRICT;
CREATE INDEX goal_concept_lookup ON goal_concepts(concept_id,goal_id);
CREATE TABLE notes (
 id TEXT PRIMARY KEY,
 concept_id TEXT NOT NULL REFERENCES concepts(id),
 title TEXT NOT NULL,
 body TEXT NOT NULL,
 basis TEXT NOT NULL CHECK(basis IN ('source','web','topic')),
 evidence TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(evidence)),
 citations TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(citations)),
 source_id TEXT NOT NULL REFERENCES sources(id),
 job_id TEXT NOT NULL DEFAULT '',
 model TEXT NOT NULL DEFAULT '',
 prompt_version TEXT NOT NULL DEFAULT '',
 supersedes TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX concept_notes ON notes(concept_id,created_at);
CREATE TABLE source_documents (
 id TEXT PRIMARY KEY,
 source_id TEXT NOT NULL REFERENCES sources(id),
 job_id TEXT NOT NULL REFERENCES jobs(id),
 kind TEXT NOT NULL CHECK(kind IN ('search_result','page','transcript')),
 position INTEGER NOT NULL,
 url TEXT NOT NULL DEFAULT '',
 title TEXT NOT NULL DEFAULT '',
 published TEXT NOT NULL DEFAULT '',
 text TEXT NOT NULL,
 provider TEXT NOT NULL,
 created_at INTEGER NOT NULL,
 UNIQUE(job_id,position)
) STRICT;
CREATE INDEX source_document_lookup ON source_documents(source_id,created_at);
CREATE TABLE capture_images (
 source_id TEXT PRIMARY KEY REFERENCES sources(id),
 mime TEXT NOT NULL CHECK(mime IN ('image/jpeg','image/png','image/webp')),
 bytes BLOB NOT NULL CHECK(length(bytes) BETWEEN 1 AND 4194304),
 created_at INTEGER NOT NULL
) STRICT;
ALTER TABLE sources ADD COLUMN mode TEXT NOT NULL DEFAULT '' CHECK(mode IN ('','topic','text','link','photo'));
ALTER TABLE sources ADD COLUMN web INTEGER NOT NULL DEFAULT 0 CHECK(web IN (0,1));
ALTER TABLE jobs ADD COLUMN kind TEXT NOT NULL DEFAULT 'quizzes' CHECK(kind IN ('quizzes','research','transcribe','plan','questions','contrast','fix'));
ALTER TABLE jobs ADD COLUMN payload TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(payload));
CREATE TABLE evidence (
 id TEXT PRIMARY KEY,
 kind TEXT NOT NULL CHECK(kind IN ('read','know','dismiss','practice','confusion')),
 concept_id TEXT NOT NULL DEFAULT '',
 quiz_id TEXT NOT NULL DEFAULT '',
 presentation_id TEXT NOT NULL DEFAULT '',
 review_id TEXT NOT NULL DEFAULT '',
 detail TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(detail)),
 created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX evidence_concept ON evidence(concept_id,kind,created_at);
CREATE TABLE grade_overrides (
 id TEXT PRIMARY KEY,
 review_id TEXT NOT NULL UNIQUE REFERENCES review_events(id),
 presentation_id TEXT NOT NULL REFERENCES presentations(id),
 direction TEXT NOT NULL CHECK(direction IN ('correct','missed')),
 schedule_before TEXT NOT NULL CHECK(json_valid(schedule_before)),
 schedule_after TEXT NOT NULL CHECK(json_valid(schedule_after)),
 schedule_version_before INTEGER NOT NULL,
 schedule_version_after INTEGER NOT NULL,
 created_at INTEGER NOT NULL
) STRICT;
ALTER TABLE content_assessments ADD COLUMN purpose TEXT NOT NULL DEFAULT 'critic' CHECK(purpose IN ('critic','dedupe'));
ALTER TABLE review_session ADD COLUMN focus_concept TEXT NOT NULL DEFAULT '';
ALTER TABLE review_session ADD COLUMN focus_until INTEGER NOT NULL DEFAULT 0;
CREATE TABLE preferences (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 pace TEXT NOT NULL DEFAULT 'steady' CHECK(pace IN ('light','steady','intense')),
 updated_at INTEGER NOT NULL DEFAULT 0
) STRICT;
INSERT INTO preferences(singleton) VALUES(1);
CREATE VIRTUAL TABLE search_index USING fts5(kind UNINDEXED, ref UNINDEXED, title, body, tokenize='unicode61 remove_diacritics 2');
CREATE TRIGGER immutable_note_update BEFORE UPDATE ON notes BEGIN SELECT RAISE(ABORT,'notes are immutable'); END;
CREATE TRIGGER immutable_note_delete BEFORE DELETE ON notes BEGIN SELECT RAISE(ABORT,'notes are immutable'); END;
CREATE TRIGGER immutable_document_update BEFORE UPDATE ON source_documents BEGIN SELECT RAISE(ABORT,'source documents are immutable'); END;
CREATE TRIGGER immutable_document_delete BEFORE DELETE ON source_documents BEGIN SELECT RAISE(ABORT,'source documents are immutable'); END;
CREATE TRIGGER immutable_image_update BEFORE UPDATE ON capture_images BEGIN SELECT RAISE(ABORT,'captured images are immutable'); END;
CREATE TRIGGER immutable_image_delete BEFORE DELETE ON capture_images BEGIN SELECT RAISE(ABORT,'captured images are immutable'); END;
CREATE TRIGGER immutable_evidence_update BEFORE UPDATE ON evidence BEGIN SELECT RAISE(ABORT,'evidence is immutable'); END;
CREATE TRIGGER immutable_evidence_delete BEFORE DELETE ON evidence BEGIN SELECT RAISE(ABORT,'evidence is immutable'); END;
CREATE TRIGGER immutable_override_update BEFORE UPDATE ON grade_overrides BEGIN SELECT RAISE(ABORT,'grade overrides are immutable'); END;
CREATE TRIGGER immutable_override_delete BEFORE DELETE ON grade_overrides BEGIN SELECT RAISE(ABORT,'grade overrides are immutable'); END;
CREATE TABLE quiz_proposals (
 id TEXT PRIMARY KEY,
 quiz_id TEXT NOT NULL REFERENCES quizzes(id),
 base_version INTEGER NOT NULL,
 job_id TEXT NOT NULL UNIQUE REFERENCES jobs(id),
 instruction TEXT NOT NULL,
 content TEXT NOT NULL CHECK(json_valid(content)),
 model TEXT NOT NULL,
 prompt_version TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','accepted','discarded','superseded')),
 created_at INTEGER NOT NULL,
 decided_at INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE UNIQUE INDEX one_pending_proposal ON quiz_proposals(quiz_id) WHERE status='pending';
PRAGMA user_version=5;
`

// migrationV4ToV5 backfills v5 structures from existing rows. Prerequisites
// carry over as requires relations; every existing source gets its goal; live
// foundation work is canceled with its usage retained, because the foundation
// experience is retired and nothing may resend it.
const migrationV4ToV5 = `
INSERT INTO concept_relations(id,from_id,to_id,kind,origin,created_at)
SELECT lower(hex(randomblob(16))), concept_id, prerequisite_id, 'requires', 'foundation', created_at FROM concept_prerequisites;
INSERT INTO goals(id,title,source_id,status,focus,created_at,updated_at)
SELECT lower(hex(randomblob(16))),
 CASE WHEN length(trim(replace(replace(text,char(10),' '),char(13),' ')))>120
  THEN substr(trim(replace(replace(text,char(10),' '),char(13),' ')),1,119)||'…'
  ELSE trim(replace(replace(text,char(10),' '),char(13),' ')) END,
 id, CASE WHEN archived=1 THEN 'archived' ELSE 'active' END, 0, created_at, created_at FROM sources;
UPDATE job_attempts SET state='unknown',finished_at=CAST(strftime('%s','now') AS INTEGER)*1000
 WHERE state='active' AND job_id IN (SELECT job_id FROM foundation_requests);
UPDATE jobs SET status='canceled',error='Foundations retired; usage retained',lease_token='',lease_until=0,
 updated_at=CAST(strftime('%s','now') AS INTEGER)*1000
 WHERE status IN ('queued','running','retry','paused') AND id IN (SELECT job_id FROM foundation_requests);
UPDATE review_session SET current_id=NULL WHERE current_id IN (SELECT presentation_id FROM foundation_bridges) AND current_id IN (SELECT id FROM presentations WHERE graded=0);
-- Links to foundation concepts are history: their role keeps every live
-- concept query (role='assesses') from treating them as study concepts.
UPDATE concept_quizzes SET role='foundation' WHERE concept_id IN (SELECT id FROM concepts WHERE origin='foundation');
-- Unstarted or interrupted single-call work continues through the v5 chain;
-- a saved candidate batch keeps its kind and resumes only its critic.
UPDATE jobs SET kind='plan' WHERE kind='quizzes' AND candidates_json IS NULL
 AND status IN ('queued','running','retry','paused') AND id NOT IN (SELECT job_id FROM foundation_requests);
`
