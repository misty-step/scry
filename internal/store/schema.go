package store

// Version one is an atomic fresh-database migration. Existing non-Scry databases
// and newer versions are refused; future migrations must preserve these fences.
const schemaV1 = `
CREATE TABLE sources (
 id TEXT PRIMARY KEY, text TEXT NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('topic','source')),
 revision INTEGER NOT NULL CHECK(revision>0), archived INTEGER NOT NULL DEFAULT 0 CHECK(archived IN (0,1)), created_at INTEGER NOT NULL
) STRICT;
CREATE TABLE source_revisions (
 source_id TEXT NOT NULL REFERENCES sources(id), revision INTEGER NOT NULL,
 text TEXT NOT NULL, kind TEXT NOT NULL, created_at INTEGER NOT NULL,
 PRIMARY KEY(source_id,revision)
) STRICT;
CREATE TABLE jobs (
 id TEXT PRIMARY KEY, source_id TEXT NOT NULL REFERENCES sources(id), source_revision INTEGER NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('queued','running','retry','complete','partial','failed','canceled','paused')),
 error TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', prompt_version TEXT NOT NULL DEFAULT '',
 attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 3),
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, available_at INTEGER NOT NULL,
 lease_token TEXT NOT NULL DEFAULT '', lease_until INTEGER NOT NULL DEFAULT 0,
 published INTEGER NOT NULL DEFAULT 0, result_json TEXT
) STRICT;
CREATE UNIQUE INDEX one_live_source_job ON jobs(source_id) WHERE status IN ('queued','running','retry','paused');
CREATE INDEX claim_queue ON jobs(status,available_at,created_at);
CREATE TABLE job_attempts (
 token TEXT PRIMARY KEY, job_id TEXT NOT NULL REFERENCES jobs(id), number INTEGER NOT NULL,
 started_at INTEGER NOT NULL, finished_at INTEGER,
 reserved_micros INTEGER NOT NULL CHECK(reserved_micros>=0), cost_micros INTEGER CHECK(cost_micros>=0),
 state TEXT NOT NULL CHECK(state IN ('active','unknown','settled')),
 finish_hash TEXT NOT NULL DEFAULT '', finish_code TEXT NOT NULL DEFAULT '',
 UNIQUE(job_id,number)
) STRICT;
CREATE INDEX spend_window ON job_attempts(started_at);
CREATE TABLE quizzes (
 id TEXT PRIMARY KEY, source_id TEXT NOT NULL REFERENCES sources(id), version INTEGER NOT NULL CHECK(version>0),
 archived INTEGER NOT NULL DEFAULT 0 CHECK(archived IN (0,1)), created_at INTEGER NOT NULL,
 origin_job_id TEXT NOT NULL REFERENCES jobs(id), origin_index INTEGER NOT NULL,
 UNIQUE(origin_job_id,origin_index)
) STRICT;
CREATE INDEX source_quizzes ON quizzes(source_id);
CREATE TABLE quiz_versions (
 quiz_id TEXT NOT NULL REFERENCES quizzes(id), version INTEGER NOT NULL, content TEXT NOT NULL CHECK(json_valid(content)),
 model TEXT NOT NULL, prompt_version TEXT NOT NULL, created_at INTEGER NOT NULL,
 PRIMARY KEY(quiz_id,version)
) STRICT;
CREATE TABLE schedules (
 quiz_id TEXT PRIMARY KEY REFERENCES quizzes(id), version INTEGER NOT NULL CHECK(version>0),
 card TEXT NOT NULL CHECK(json_valid(card)), due_at INTEGER NOT NULL, algorithm TEXT NOT NULL
) STRICT;
CREATE INDEX due_schedule ON schedules(due_at,quiz_id);
CREATE TABLE presentations (
 id TEXT PRIMARY KEY, quiz_id TEXT NOT NULL REFERENCES quizzes(id), content_version INTEGER NOT NULL,
 schedule_version INTEGER NOT NULL, snapshot TEXT NOT NULL CHECK(json_valid(snapshot)), created_at INTEGER NOT NULL,
 answer TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL DEFAULT '', assisted INTEGER NOT NULL DEFAULT 0 CHECK(assisted IN (0,1)),
 graded INTEGER NOT NULL DEFAULT 0 CHECK(graded IN (0,1)), rating INTEGER NOT NULL DEFAULT 0,
 due_at INTEGER NOT NULL, reviewed_at INTEGER NOT NULL DEFAULT 0, review_id TEXT NOT NULL DEFAULT '',
 FOREIGN KEY(quiz_id,content_version) REFERENCES quiz_versions(quiz_id,version)
) STRICT;
CREATE TABLE review_session (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1), current_id TEXT REFERENCES presentations(id)
) STRICT;
INSERT INTO review_session(singleton) VALUES(1);
CREATE TABLE review_events (
 id TEXT PRIMARY KEY, presentation_id TEXT NOT NULL REFERENCES presentations(id),
 snapshot TEXT NOT NULL CHECK(json_valid(snapshot)), answer TEXT NOT NULL, outcome TEXT NOT NULL,
 rating INTEGER NOT NULL CHECK(rating IN (0,1,3)), assisted INTEGER NOT NULL CHECK(assisted IN (0,1)),
 reviewed_at INTEGER NOT NULL, due_at INTEGER NOT NULL, algorithm TEXT NOT NULL,
 schedule_before TEXT NOT NULL CHECK(json_valid(schedule_before)), schedule_after TEXT NOT NULL CHECK(json_valid(schedule_after)),
 schedule_version_before INTEGER NOT NULL, schedule_version_after INTEGER NOT NULL
) STRICT;
CREATE UNIQUE INDEX one_graded_event ON review_events(presentation_id) WHERE rating>0;
CREATE INDEX review_chronology ON review_events(reviewed_at DESC,id);
CREATE TABLE corrections (
 id TEXT PRIMARY KEY, review_id TEXT NOT NULL REFERENCES review_events(id), note TEXT NOT NULL,
 reset INTEGER NOT NULL CHECK(reset IN (0,1)), created_at INTEGER NOT NULL,
 schedule_before TEXT, schedule_after TEXT,
 UNIQUE(review_id,note,reset)
) STRICT;
CREATE TABLE operations (
 id TEXT PRIMARY KEY, kind TEXT NOT NULL, payload_hash TEXT NOT NULL,
 result_id TEXT NOT NULL, result_json TEXT, created_at INTEGER NOT NULL
) STRICT;
CREATE TABLE backups (
 id TEXT PRIMARY KEY, path TEXT NOT NULL, remote_key TEXT NOT NULL, sha256 TEXT NOT NULL,
 error TEXT NOT NULL, created_at INTEGER NOT NULL, bytes INTEGER NOT NULL CHECK(bytes>=0), remote INTEGER NOT NULL CHECK(remote IN (0,1))
) STRICT;
CREATE TRIGGER immutable_review_update BEFORE UPDATE ON review_events BEGIN SELECT RAISE(ABORT,'review events are immutable'); END;
CREATE TRIGGER immutable_review_delete BEFORE DELETE ON review_events BEGIN SELECT RAISE(ABORT,'review events are immutable'); END;
CREATE TRIGGER immutable_version_update BEFORE UPDATE ON quiz_versions BEGIN SELECT RAISE(ABORT,'quiz versions are immutable'); END;
CREATE TRIGGER immutable_version_delete BEFORE DELETE ON quiz_versions BEGIN SELECT RAISE(ABORT,'quiz versions are immutable'); END;
CREATE TRIGGER immutable_operation_update BEFORE UPDATE ON operations BEGIN SELECT RAISE(ABORT,'operation receipts are immutable'); END;
CREATE TRIGGER immutable_operation_delete BEFORE DELETE ON operations BEGIN SELECT RAISE(ABORT,'operation receipts are immutable'); END;
PRAGMA application_id=1396920921;
PRAGMA user_version=1;
`
