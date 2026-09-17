package store

// Schema 3 introduces concepts, references, and their relationships.
// Historical foundation tables and rows are preserved.
const schemaV3 = `
CREATE TABLE concepts (
 id TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 description TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL
) STRICT;
CREATE TABLE "references" (
 id TEXT PRIMARY KEY,
 title TEXT NOT NULL,
 content TEXT NOT NULL,
 format TEXT NOT NULL CHECK(format IN ('explanation','diagram','article','video','reference')),
 source_url TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL
) STRICT;
CREATE TABLE concept_prerequisites (
 concept_id TEXT NOT NULL REFERENCES concepts(id),
 prerequisite_id TEXT NOT NULL REFERENCES concepts(id),
 created_at INTEGER NOT NULL,
 PRIMARY KEY(concept_id,prerequisite_id),
 CHECK(concept_id!=prerequisite_id)
) STRICT;
CREATE TABLE concept_references (
 concept_id TEXT NOT NULL REFERENCES concepts(id),
 reference_id TEXT NOT NULL REFERENCES "references"(id),
 created_at INTEGER NOT NULL,
 PRIMARY KEY(concept_id,reference_id)
) STRICT;
CREATE TABLE concept_quizzes (
 concept_id TEXT NOT NULL REFERENCES concepts(id),
 quiz_id TEXT NOT NULL REFERENCES quizzes(id),
 created_at INTEGER NOT NULL,
 PRIMARY KEY(concept_id,quiz_id)
) STRICT;
CREATE INDEX concept_name ON concepts(name);
CREATE INDEX reference_title ON "references"(title);
CREATE INDEX concept_ref_lookup ON concept_references(reference_id,concept_id);
CREATE INDEX concept_quiz_lookup ON concept_quizzes(quiz_id,concept_id);
CREATE INDEX concept_prereq_lookup ON concept_prerequisites(prerequisite_id,concept_id);
PRAGMA user_version=3;
`

const migrationV2ToV3 = `
INSERT OR IGNORE INTO concepts(id, name, description, created_at)
SELECT u.id, u.id, u.definition, COALESCE(b.created_at, strftime('%s','now'))
FROM foundation_units u
LEFT JOIN foundation_links l ON l.unit_id = u.id AND l.unit_version = u.version
LEFT JOIN foundation_materials m ON m.id = l.material_id AND m.version = l.material_version
LEFT JOIN foundation_bundles b ON b.id = m.bundle_id;

INSERT OR IGNORE INTO "references"(id, title, content, format, source_url, created_at)
SELECT m.id, COALESCE(NULLIF(json_extract(m.content, '$.title'), ''), m.id), m.content,
 CASE
  WHEN json_extract(m.content, '$.kind') = 'diagram' THEN 'diagram'
  WHEN json_extract(m.content, '$.kind') = 'reference' THEN 'reference'
  WHEN json_extract(m.content, '$.kind') = 'article' THEN 'article'
  WHEN json_extract(m.content, '$.kind') = 'video' THEN 'video'
  ELSE 'explanation'
 END,
 COALESCE(json_extract(m.content, '$.url'), ''),
 COALESCE(b.created_at, strftime('%s','now'))
FROM foundation_materials m
LEFT JOIN foundation_bundles b ON b.id = m.bundle_id;

INSERT OR IGNORE INTO concept_references(concept_id, reference_id, created_at)
SELECT l.unit_id, l.material_id, COALESCE(b.created_at, strftime('%s','now'))
FROM foundation_links l
JOIN foundation_materials m ON m.id = l.material_id AND m.version = l.material_version
JOIN foundation_bundles b ON b.id = m.bundle_id;

INSERT OR IGNORE INTO concept_quizzes(concept_id, quiz_id, created_at)
SELECT l.unit_id, b.quiz_id, b.created_at
FROM foundation_links l
JOIN foundation_materials m ON m.id = l.material_id AND m.version = l.material_version
JOIN foundation_bundles b ON b.id = m.bundle_id;
`
