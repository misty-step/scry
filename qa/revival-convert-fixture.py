#!/usr/bin/env python3
"""Synthetic Go schema-5 archive, independently built with native SQLite."""
import base64
import datetime
import hashlib
import io
import json
from pathlib import Path
import sqlite3
import sys
import zipfile

mode = sys.argv[1] if len(sys.argv) > 1 else "valid"
db = sqlite3.connect(":memory:")
db.executescript(Path("target/revival-schema.sql").read_text())
now = 1700000000000
canary = "SYNTHETIC_PRIVATE_CONVERSION_CANARY"
encode = lambda v: json.dumps(v, ensure_ascii=False, separators=(",", ":"))
sha = lambda b: hashlib.sha256(b).hexdigest()
golden = json.loads(Path("tests/fixtures/fsrs-golden.json").read_text())
transition = golden["cases"][0]["transitions"][0]

def go_card(c):
    date = lambda ms: datetime.datetime.fromtimestamp(ms / 1000, datetime.timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")
    return encode({"Due": date(c["due_ms"]), "Stability": c["stability"], "Difficulty": c["difficulty"],
                   "ScheduledDays": c["scheduled_days"], "Reps": c["reps"], "Lapses": c["lapses"],
                   "State": ["new", "learning", "review", "relearning"].index(c["state"]),
                   "LastReview": date(c["last_review_ms"]) if c["last_review_ms"] is not None else "0001-01-01T00:00:00Z",
                   "RemainingSteps": c["remaining_steps"]})

algorithm = golden["scheduler"] + ";grading=exact-v1"
photo = b"\x89PNG\r\n\x1a\nSYNTHETIC_PRIVATE_PHOTO"
for n in range(2):
    source, goal, job = f"source{n}", f"goal{n}", f"job{n}"
    text = canary + "\nA cache can reuse a fresh response."
    db.execute("INSERT INTO sources(id,text,kind,revision,created_at) VALUES(?,?,?,?,?)", (source, text, "source", 1, now))
    db.execute("INSERT INTO source_revisions VALUES(?,?,?,?,?)", (source, 1, text, "source", now))
    db.execute("INSERT INTO goals VALUES(?,?,?,?,?,?,?)", (goal, canary + str(n), source, "active", n, now, now))
    db.execute("INSERT INTO jobs(id,source_id,source_revision,status,attempts,created_at,updated_at,available_at,model) VALUES(?,?,?,?,?,?,?,?,?)",
               (job, source, 1, "complete", 1, now, now, now, "synthetic-original-model"))
    db.execute("INSERT INTO job_attempts(token,job_id,number,started_at,finished_at,reserved_micros,cost_micros,state) VALUES(?,?,?,?,?,?,?,?)",
               (f"attempt{n}", job, 1, now, now + 1, 500000, 1000, "settled"))
    db.execute("INSERT INTO operations VALUES(?,?,?,?,?,?)", (f"original-operation-{n:02}", "capture", str(n) * 64, source, encode({"private": canary}), now))
    if n:
        db.execute("INSERT INTO capture_images VALUES(?,?,?,?)", (source, "image/png", photo, now))

db.execute("INSERT INTO concepts(id,name,description,created_at,origin,source_id) VALUES(?,?,?,?,?,?)", ("concept0", "Cache freshness", "Fresh responses may be reused.", now, "generated", "source0"))
db.execute("INSERT INTO concepts(id,name,description,created_at,origin,source_id) VALUES(?,?,?,?,?,?)", ("foundation0", "Old foundation", canary, now, "foundation", "source0"))
db.execute("INSERT INTO notes VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)", ("note0", "concept0", "Freshness", "A cache can reuse a fresh response.", "source", encode(["A cache can reuse a fresh response."]), "[]", "source0", "job0", "synthetic-original-model", "prompt-v1", "", now))
db.execute("INSERT INTO goal_concepts VALUES(?,?,?)", ("goal0", "concept0", now))
db.execute("INSERT INTO evidence VALUES(?,?,?,?,?,?,?,?)", ("read0", "read", "concept0", "", "", "", "{}", now))
contents = []
for n in range(3):
    source = "source0" if n < 2 else "source1"
    q = {"kind": "recall", "prompt": "Which policy lets a cache reuse a stored response?", "answer": "freshness",
         "explanation": "Freshness permits reuse without contacting the origin.", "evidence": "", "basis": "topic", "choices": [], "variants": []}
    contents.append(q)
    db.execute("INSERT INTO quizzes VALUES(?,?,?,?,?,?,?)", (f"quiz{n}", source, 2 if n == 0 else 1, 0, now, "job0" if n < 2 else "job1", n))
    db.execute("INSERT INTO quiz_versions VALUES(?,?,?,?,?,?)", (f"quiz{n}", 1, encode(q), "synthetic-original-model", "prompt-v1", now))
    if n == 0:
        newer = dict(q, prompt="Which policy governs fresh cached response reuse?")
        db.execute("INSERT INTO quiz_versions VALUES(?,?,?,?,?,?)", ("quiz0", 2, encode(newer), "synthetic-original-model", "prompt-v2", now + 1))
    c = transition["after"] if n == 0 else transition["before"]
    db.execute("INSERT INTO schedules VALUES(?,?,?,?,?)", (f"quiz{n}", 2 if n == 0 else 1, go_card(c), c["due_ms"], algorithm))
db.execute("INSERT INTO concept_quizzes VALUES(?,?,?,?)", ("concept0", "quiz0", now, "assesses"))
db.execute("INSERT INTO concept_quizzes VALUES(?,?,?,?)", ("foundation0", "quiz1", now, "foundation"))
snapshot = dict(contents[0], id="quiz0", source_id="source0", version=1)
outcome = "correct" if transition["rating"] == 3 else "revealed"
answer = "freshness" if outcome == "correct" else ""
assisted = int(outcome == "revealed")
db.execute("INSERT INTO presentations VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)", ("presentation0", "quiz0", 1, 1, encode(snapshot), now, answer, outcome, assisted, 1, transition["rating"], transition["after"]["due_ms"], transition["now_ms"], "event0"))
db.execute("INSERT INTO review_events VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", ("event0", "presentation0", encode(snapshot), answer, outcome, transition["rating"], assisted, transition["now_ms"], transition["after"]["due_ms"], algorithm, go_card(transition["before"]), go_card(transition["after"]), 1, 2, "exact-v1"))
db.execute("UPDATE review_session SET current_id='presentation0'")
if mode == "zero-rating":
    db.execute("DROP TRIGGER immutable_review_update")
    db.execute("UPDATE review_events SET rating=0,assisted=1,outcome='warm_correct',schedule_version_after=1")
    db.execute("CREATE TRIGGER immutable_review_update BEFORE UPDATE ON review_events BEGIN SELECT RAISE(ABORT,'review events are immutable'); END")
elif mode == "unsupported-correction":
    db.execute("INSERT INTO corrections VALUES(?,?,?,?,?,?,?)", ("correction0", "event0", canary, 1, now, go_card(transition["after"]), go_card(transition["before"])))
elif mode == "unsupported-assessment":
    db.execute("INSERT INTO semantic_assessments(id,presentation_id,operation_id,content_version,schedule_version,answer,status,policy_version,request_model,request_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
               ("assessment0", "presentation0", "assessment-operation", 1, 1, canary, "failed", "short-v1", "synthetic", "{}", now))
elif mode == "unsupported-algorithm":
    db.execute("UPDATE schedules SET algorithm='different'")
elif mode == "inexact-timestamp":
    c = json.loads(go_card(transition["before"]))
    c["Due"] = "2023-11-14T22:13:20.000000001Z"
    db.execute("UPDATE schedules SET card=? WHERE quiz_id='quiz1'", (encode(c),))
elif mode == "unsupported-web":
    db.execute("INSERT INTO source_documents VALUES(?,?,?,?,?,?,?,?,?,?,?)", ("web0", "source0", "job0", "page", 0, "https://synthetic.invalid", canary, "", canary, "synthetic", now))
elif mode == "unsafe-integer":
    db.execute("UPDATE sources SET created_at=9007199254740993")
elif mode == "bad-content":
    db.execute("DROP TRIGGER immutable_version_update")
    bad = dict(contents[1], prompt="")
    db.execute("UPDATE quiz_versions SET content=? WHERE quiz_id='quiz1'", (encode(bad),))
    db.execute("CREATE TRIGGER immutable_version_update BEFORE UPDATE ON quiz_versions BEGIN SELECT RAISE(ABORT,'quiz versions are immutable'); END")
else:
    assert mode == "valid"
db.commit()
assert db.execute("PRAGMA integrity_check").fetchall() == [("ok",)]
assert db.execute("PRAGMA foreign_key_check").fetchall() == []
data = db.serialize()
manifest = {"format": 1, "id": "scry-20261015T003631.153157340Z-" + "a" * 32,
            "created_at": now + 100000, "database": {"schema": 5, "application_id": 0x53435259},
            "integrity": "ok", "bytes": len(data), "sha256": sha(data)}
archive = io.BytesIO()
with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_STORED) as z:
    z.writestr("manifest.json", encode(manifest))
    z.writestr("scry.sqlite", data)
print(json.dumps({"bytes": base64.b64encode(archive.getvalue()).decode(),
                  "photo_sha256": sha(photo), "transition": transition, "original_content": contents[0]}))
