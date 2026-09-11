package store

// Additive migration: historical v1 rows and schedules are never rewritten.
const schemaV2 = `
CREATE TABLE foundation_requests (
 job_id TEXT PRIMARY KEY REFERENCES jobs(id), quiz_id TEXT NOT NULL, quiz_version INTEGER NOT NULL,
 FOREIGN KEY(quiz_id,quiz_version) REFERENCES quiz_versions(quiz_id,version)
) STRICT;
CREATE TABLE foundation_bundles (
 id TEXT PRIMARY KEY, job_id TEXT NOT NULL UNIQUE REFERENCES foundation_requests(job_id),
 source_id TEXT NOT NULL, source_revision INTEGER NOT NULL, quiz_id TEXT NOT NULL, quiz_version INTEGER NOT NULL,
 model TEXT NOT NULL, prompt_version TEXT NOT NULL, note TEXT NOT NULL, created_at INTEGER NOT NULL,
 FOREIGN KEY(source_id,source_revision) REFERENCES source_revisions(source_id,revision),
 FOREIGN KEY(quiz_id,quiz_version) REFERENCES quiz_versions(quiz_id,version)
) STRICT;
CREATE TABLE foundation_units (
 id TEXT NOT NULL, version INTEGER NOT NULL CHECK(version>0), definition TEXT NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('foundation','composition')),
 provenance TEXT NOT NULL, PRIMARY KEY(id,version)
) STRICT;
CREATE TABLE foundation_materials (
 id TEXT NOT NULL, version INTEGER NOT NULL CHECK(version>0), bundle_id TEXT NOT NULL REFERENCES foundation_bundles(id),
 position INTEGER NOT NULL, content TEXT NOT NULL CHECK(json_valid(content)), provenance TEXT NOT NULL,
 PRIMARY KEY(id,version), UNIQUE(bundle_id,position)
) STRICT;
CREATE TABLE foundation_links (
 material_id TEXT NOT NULL, material_version INTEGER NOT NULL, unit_id TEXT NOT NULL, unit_version INTEGER NOT NULL,
 role TEXT NOT NULL CHECK(role IN ('teaches','directly-assesses','assumes','mentions')), provenance TEXT NOT NULL,
 PRIMARY KEY(material_id,material_version,unit_id,unit_version,role),
 FOREIGN KEY(material_id,material_version) REFERENCES foundation_materials(id,version),
 FOREIGN KEY(unit_id,unit_version) REFERENCES foundation_units(id,version)
) STRICT;
CREATE TABLE foundation_bridges (
 id TEXT PRIMARY KEY, presentation_id TEXT NOT NULL UNIQUE REFERENCES presentations(id),
 source_revision INTEGER NOT NULL, job_id TEXT REFERENCES jobs(id), bundle_id TEXT REFERENCES foundation_bundles(id),
 revision INTEGER NOT NULL DEFAULT 1, phase TEXT NOT NULL CHECK(phase IN ('waiting','ready','instruction','practice','feedback','return')),
 position INTEGER NOT NULL DEFAULT 0, answer TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL DEFAULT '', draft TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL
) STRICT;
CREATE TABLE foundation_interactions (
 id TEXT PRIMARY KEY, bridge_id TEXT NOT NULL REFERENCES foundation_bridges(id),
 material_id TEXT, material_version INTEGER, action TEXT NOT NULL CHECK(action IN ('read','practice','return')),
 answer TEXT NOT NULL, outcome TEXT NOT NULL, created_at INTEGER NOT NULL,
 FOREIGN KEY(material_id,material_version) REFERENCES foundation_materials(id,version)
) STRICT;
CREATE TRIGGER immutable_foundation_bundle_update BEFORE UPDATE ON foundation_bundles BEGIN SELECT RAISE(ABORT,'foundation versions are immutable'); END;
CREATE TRIGGER immutable_foundation_bundle_delete BEFORE DELETE ON foundation_bundles BEGIN SELECT RAISE(ABORT,'foundation versions are immutable'); END;
CREATE TRIGGER immutable_foundation_unit_update BEFORE UPDATE ON foundation_units BEGIN SELECT RAISE(ABORT,'foundation versions are immutable'); END;
CREATE TRIGGER immutable_foundation_unit_delete BEFORE DELETE ON foundation_units BEGIN SELECT RAISE(ABORT,'foundation versions are immutable'); END;
CREATE TRIGGER immutable_foundation_material_update BEFORE UPDATE ON foundation_materials BEGIN SELECT RAISE(ABORT,'foundation versions are immutable'); END;
CREATE TRIGGER immutable_foundation_material_delete BEFORE DELETE ON foundation_materials BEGIN SELECT RAISE(ABORT,'foundation versions are immutable'); END;
CREATE TRIGGER immutable_foundation_link_update BEFORE UPDATE ON foundation_links BEGIN SELECT RAISE(ABORT,'foundation coverage is immutable'); END;
CREATE TRIGGER immutable_foundation_link_delete BEFORE DELETE ON foundation_links BEGIN SELECT RAISE(ABORT,'foundation coverage is immutable'); END;
CREATE TRIGGER immutable_foundation_interaction_update BEFORE UPDATE ON foundation_interactions BEGIN SELECT RAISE(ABORT,'foundation observations are immutable'); END;
CREATE TRIGGER immutable_foundation_interaction_delete BEFORE DELETE ON foundation_interactions BEGIN SELECT RAISE(ABORT,'foundation observations are immutable'); END;
PRAGMA user_version=2;
`
