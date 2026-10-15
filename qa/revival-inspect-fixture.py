#!/usr/bin/env python3
"""Explicit synthetic native SQLite oracle; never accepts an archive or DB path."""
import base64
import hashlib
import io
import json
from pathlib import Path
import sqlite3
import struct
import sys
import zipfile

mode = sys.argv[1]
db = sqlite3.connect(":memory:")
db.executescript(Path("target/revival-schema.sql").read_text())
canary = "SYNTHETIC_PRIVATE_CANARY\0雪"
db.execute("INSERT INTO sources(id,text,kind,revision,created_at) VALUES(?,?,?,?,?)",
           ("fixture-source", canary, "source", 1, 9007199254740993))
db.execute("INSERT INTO capture_images VALUES(?,?,?,?)", ("fixture-source", "image/png", b"SYNTHETIC_PHOTO\x00\xff", 1))
db.execute("INSERT INTO goals VALUES(?,?,?,?,?,?,?)", ("fixture-goal", canary, "fixture-source", "active", 0, 1, 1))
db.execute("INSERT INTO jobs(id,source_id,source_revision,status,created_at,updated_at,available_at) VALUES(?,?,?,?,?,?,?)",
           ("fixture-job", "fixture-source", 1, "complete", 1, 1, 1))
db.execute("INSERT INTO quizzes VALUES(?,?,?,?,?,?,?)", ("fixture-quiz", "fixture-source", 1, 0, 1, "fixture-job", 0))
db.execute("INSERT INTO quiz_versions VALUES(?,?,?,?,?,?)", ("fixture-quiz", 1, '{"synthetic":true}', "fixture-model", "fixture-prompt", 1))
db.execute("INSERT INTO schedules VALUES(?,?,?,?,?)", ("fixture-quiz", 7, '{"Stability":1.23456789012345,"Difficulty":9,"RemainingSteps":2}', 1234, "synthetic-pinned-scheduler"))
db.execute("INSERT INTO operations VALUES(?,?,?,?,?,?)", ("fixture-operation", "capture", "0" * 64, "fixture-source", '{"synthetic":true}', 1))
db.execute("INSERT INTO concept_index(ref,name,summary) VALUES(?,?,?)", ("fixture-concept", "synthetic", canary))
if mode == "unexpected":
    db.execute('CREATE TABLE "SYNTHETIC_SECRET_TABLE"(value)')
elif mode == "missing":
    db.execute("DROP TABLE capture_images")
elif mode == "foreign-key":
    db.execute("INSERT INTO capture_images VALUES(?,?,?,?)", ("missing-source", "image/png", b"orphan", 1))
elif mode == "application":
    db.execute("PRAGMA application_id=1")
elif mode == "version":
    db.execute("PRAGMA user_version=4")
elif mode == "schema":
    db.execute("ALTER TABLE sources ADD COLUMN hidden TEXT")
elif mode == "changed-text":
    db.execute("UPDATE sources SET text=text || 'changed'")
elif mode == "changed-media":
    db.execute("DROP TRIGGER immutable_image_update")
    db.execute("UPDATE capture_images SET bytes=?", (b"SYNTHETIC_PHOTO\x00\xfe",))
    # Recreate the exact trigger, changing content only, not schema.
    db.execute("CREATE TRIGGER immutable_image_update BEFORE UPDATE ON capture_images BEGIN SELECT RAISE(ABORT,'captured images are immutable'); END")
elif mode == "changed-schedule":
    db.execute("UPDATE schedules SET version=8")
elif mode == "changed-bigint":
    db.execute("UPDATE sources SET created_at=9007199254740992")
else:
    assert mode in ("valid", "corrupt", "manifest-digest", "duplicate", "extra", "compressed")
db.commit()

def digest(data):
    return hashlib.sha256(data).hexdigest()

def encoded(value):
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode()

def cell(value):
    if value is None:
        return ["null"]
    if isinstance(value, int):
        return ["integer", str(value)]
    if isinstance(value, float):
        return ["real-ieee754-be", struct.pack(">d", value).hex()]
    if isinstance(value, str):
        return ["text-utf8", value.encode().hex()]
    assert isinstance(value, bytes)
    return ["blob", value.hex()]

schema = db.execute("SELECT type,name,tbl_name,sql FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name").fetchall()
tables = []
for kind, name, _, _ in schema:
    if kind != "table":
        continue
    cursor = db.execute('SELECT * FROM "' + name.replace('"', '""') + '"')
    hashes = sorted(digest(encoded([cell(value) for value in row])) for row in cursor)
    tables.append({"name": name, "rows": len(hashes), "columns": len(cursor.description), "sha256": digest(encoded(hashes))})
data = db.serialize()
if mode == "corrupt":
    data = data[:100] + b"\xff" * (len(data) - 100)
manifest = {"format": 1, "id": "scry-20261015T003631.153157340Z-" + "a" * 32,
            "created_at": 1792100000000, "database": {"schema": 5, "application_id": 0x53435259},
            "integrity": "ok", "bytes": len(data), "sha256": digest(data)}
if mode == "manifest-digest":
    manifest["sha256"] = "0" * 64
archive = io.BytesIO()
with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED if mode == "compressed" else zipfile.ZIP_STORED) as z:
    z.writestr("manifest.json", encoded(manifest))
    z.writestr("scry.sqlite", data)
    if mode == "duplicate":
        z.writestr("scry.sqlite", data)
    if mode == "extra":
        z.writestr("SYNTHETIC_PRIVATE_EXTRA", b"private-canary")
print(json.dumps({"bytes": base64.b64encode(archive.getvalue()).decode(), "tables": tables,
                  "database_sha256": digest(data), "schema_sha256": digest(encoded(schema))}))
