package store

// Schema 4 adds durable semantic assessment state while preserving all prior
// content, schedules, reviews, operations, and foundation/concept data.
//
// Every assessment row is also a spending record: one transmission is ever
// allowed per row, a reservation is taken under a lease before the send, and
// the reservation stays counted as unknown spend until a measured cost or a
// definite no-send failure replaces it.
//
// assistance_exposures records every cue or feedback text actually shown to
// the learner so a later occurrence of the same content within the exposure
// window cannot manufacture a cold success.
const schemaV4 = `
CREATE TABLE semantic_assessments (
 id TEXT PRIMARY KEY, presentation_id TEXT NOT NULL REFERENCES presentations(id), operation_id TEXT NOT NULL,
 content_version INTEGER NOT NULL, schedule_version INTEGER NOT NULL, answer TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','judged','failed','superseded')),
 policy_version TEXT NOT NULL, request_model TEXT NOT NULL, response_model TEXT NOT NULL DEFAULT '',
 request_json TEXT NOT NULL CHECK(json_valid(request_json)), response_json TEXT CHECK(response_json IS NULL OR json_valid(response_json)),
 decision TEXT NOT NULL DEFAULT '', applied INTEGER NOT NULL DEFAULT 0 CHECK(applied IN (0,1)), detail TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '',
 input_tokens INTEGER, output_tokens INTEGER, cost_micros INTEGER CHECK(cost_micros IS NULL OR cost_micros>=0), latency_ms INTEGER,
 transmissions INTEGER NOT NULL DEFAULT 0 CHECK(transmissions BETWEEN 0 AND 1),
 reserved_micros INTEGER NOT NULL DEFAULT 0 CHECK(reserved_micros>=0),
 lease_token TEXT NOT NULL DEFAULT '', lease_until INTEGER NOT NULL DEFAULT 0,
 review_id TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, finished_at INTEGER NOT NULL DEFAULT 0
) STRICT;
CREATE UNIQUE INDEX one_pending_assessment ON semantic_assessments(presentation_id) WHERE status='pending';
CREATE INDEX assessment_presentation ON semantic_assessments(presentation_id,created_at);
CREATE TABLE content_assessments (
 id TEXT PRIMARY KEY, job_id TEXT NOT NULL REFERENCES jobs(id), attempt_token TEXT NOT NULL, candidate_index INTEGER NOT NULL,
 candidate_json TEXT NOT NULL CHECK(json_valid(candidate_json)),
 status TEXT NOT NULL CHECK(status IN ('pending','judged','failed')), policy_version TEXT NOT NULL,
 request_model TEXT NOT NULL, response_model TEXT NOT NULL DEFAULT '',
 request_json TEXT NOT NULL CHECK(json_valid(request_json)), response_json TEXT CHECK(response_json IS NULL OR json_valid(response_json)),
 decision TEXT NOT NULL DEFAULT '', reasons TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '',
 input_tokens INTEGER, output_tokens INTEGER, cost_micros INTEGER CHECK(cost_micros IS NULL OR cost_micros>=0), latency_ms INTEGER,
 transmissions INTEGER NOT NULL DEFAULT 0 CHECK(transmissions BETWEEN 0 AND 1),
 reserved_micros INTEGER NOT NULL DEFAULT 0 CHECK(reserved_micros>=0),
 lease_token TEXT NOT NULL DEFAULT '', lease_until INTEGER NOT NULL DEFAULT 0,
 created_at INTEGER NOT NULL, finished_at INTEGER NOT NULL DEFAULT 0,
 UNIQUE(job_id,attempt_token,candidate_index)
) STRICT;
CREATE TABLE assistance_exposures (
 id TEXT PRIMARY KEY, presentation_id TEXT NOT NULL REFERENCES presentations(id), quiz_id TEXT NOT NULL REFERENCES quizzes(id),
 content_version INTEGER NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('cue','feedback')), created_at INTEGER NOT NULL
) STRICT;
CREATE INDEX exposure_quiz ON assistance_exposures(quiz_id,created_at);
ALTER TABLE jobs ADD COLUMN candidates_json TEXT CHECK(candidates_json IS NULL OR json_valid(candidates_json));
ALTER TABLE jobs ADD COLUMN critic_status TEXT NOT NULL DEFAULT '';
ALTER TABLE review_events ADD COLUMN grading TEXT NOT NULL DEFAULT 'exact-v1';
PRAGMA user_version=4;
`
