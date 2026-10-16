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
import time
import zipfile

mode = sys.argv[1] if len(sys.argv) > 1 else "valid"
db = sqlite3.connect(":memory:")
db.executescript(Path("target/revival-schema.sql").read_text())
now = 1700000000000
money_now = int(time.time() * 1000)
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
    if mode == "zero-rating-wrong":
        q.update(kind="choice", choices=["freshness", "revalidation"])
    contents.append(q)
    newer_version = n == 0 and not mode.startswith("selfcheck")
    db.execute("INSERT INTO quizzes VALUES(?,?,?,?,?,?,?)", (f"quiz{n}", source, 2 if newer_version else 1, 0, now, "job0" if n < 2 else "job1", n))
    db.execute("INSERT INTO quiz_versions VALUES(?,?,?,?,?,?)", (f"quiz{n}", 1, encode(q), "synthetic-original-model", "prompt-v1", now))
    if newer_version:
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
if mode.startswith("selfcheck"):
    # Pre-f966 Go Grade/Submit saved an ungraded exact-policy self-check
    # before any later learner grade, sometimes repeatedly on one presentation.
    db.execute("DROP TRIGGER immutable_review_update")
    before = go_card(transition["before"])
    if mode in {"selfcheck-history", "selfcheck-repeated"}:
        for index in range(2 if mode == "selfcheck-repeated" else 1):
            db.execute("INSERT INTO review_events VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", (f"ungraded{index}", "presentation0", encode(snapshot), "FRESHNESS", "selfcheck", 0, 0, transition["now_ms"] - 1, 0, algorithm, before, before, 1, 1, "exact-v1"))
    else:
        outcome = "warm_selfcheck" if mode == "selfcheck-helped" else "selfcheck"
        assisted = int(mode == "selfcheck-helped")
        db.execute("UPDATE review_events SET rating=0,assisted=?,answer='FRESHNESS',outcome=?,schedule_version_after=1,schedule_after=?,due_at=0", (assisted, outcome, before))
        db.execute("UPDATE presentations SET rating=0,graded=?,assisted=?,answer='FRESHNESS',outcome=?,due_at=0", (assisted, assisted, outcome))
        db.execute("UPDATE schedules SET version=1,card=?,due_at=? WHERE quiz_id='quiz0'", (before, transition["before"]["due_ms"]))
        if mode == "selfcheck-changing-card":
            db.execute("UPDATE review_events SET schedule_after=?", (go_card(transition["after"]),))
        elif mode == "selfcheck-changing-version":
            db.execute("UPDATE review_events SET schedule_version_after=2")
        elif mode == "selfcheck-bad-policy":
            db.execute("UPDATE review_events SET grading='learner-v1'")
        else:
            assert mode in {"selfcheck", "selfcheck-helped"}
    db.execute("CREATE TRIGGER immutable_review_update BEFORE UPDATE ON review_events BEGIN SELECT RAISE(ABORT,'review events are immutable'); END")
elif mode.startswith("zero-rating"):
    db.execute("DROP TRIGGER immutable_review_update")
    before = go_card(transition["before"])
    if mode == "zero-rating-chain":
        # Go applyGrade persists assisted observations without a schedule write.
        # Include repeated epochs before and after a real FSRS transition.
        for index, epoch, c, when in [(1, 1, transition["before"], transition["now_ms"]),
                                      (2, 2, transition["after"], transition["now_ms"]),
                                      (3, 2, transition["after"], transition["now_ms"])]:
            presentation, event = f"warm-presentation{index}", f"warm-event{index}"
            db.execute("INSERT INTO presentations VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)", (presentation, "quiz0", 1, epoch, encode(snapshot), when, "freshness", "warm_correct", 1, 1, 0, 0, when, event))
            db.execute("INSERT INTO review_events VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", (event, presentation, encode(snapshot), "freshness", "warm_correct", 0, 1, when, 0, algorithm, go_card(c), go_card(c), epoch, epoch, "exact-v1"))
        db.execute("UPDATE review_session SET current_id='warm-presentation3'")
    else:
        db.execute("UPDATE review_events SET rating=0,assisted=1,outcome='warm_correct',schedule_version_after=1,schedule_after=?,due_at=0", (before,))
        db.execute("UPDATE presentations SET rating=0,assisted=1,outcome='warm_correct',due_at=0")
        db.execute("UPDATE schedules SET version=1,card=?,due_at=? WHERE quiz_id='quiz0'", (before, transition["before"]["due_ms"]))
        if mode == "zero-rating-changing-card":
            db.execute("UPDATE review_events SET schedule_after=?", (go_card(transition["after"]),))
        elif mode == "zero-rating-changing-version":
            db.execute("UPDATE review_events SET schedule_version_after=2")
        elif mode == "zero-rating-unassisted":
            db.execute("UPDATE review_events SET assisted=0")
        elif mode == "zero-rating-bad-outcome":
            db.execute("UPDATE review_events SET outcome='correct'")
        elif mode == "zero-rating-bad-due":
            db.execute("UPDATE review_events SET due_at=1")
        elif mode == "zero-rating-wrong":
            db.execute("UPDATE review_events SET outcome='warm_wrong',answer='revalidation'")
            db.execute("UPDATE presentations SET outcome='warm_wrong',answer='revalidation'")
        else:
            assert mode == "zero-rating"
    db.execute("CREATE TRIGGER immutable_review_update BEFORE UPDATE ON review_events BEGIN SELECT RAISE(ABORT,'review events are immutable'); END")
elif mode in {"diagnostic-multiclass", "diagnostic-same-row"}:
    db.execute("DROP TRIGGER immutable_review_update")
    if mode == "diagnostic-multiclass":
        db.execute("UPDATE schedules SET algorithm='SYNTHETIC_PRIVATE_UNSUPPORTED'")
        db.execute("UPDATE review_events SET schedule_version_after=99")
        db.execute("UPDATE job_attempts SET cost_micros=NULL,reserved_micros=1")
        db.execute("DROP TRIGGER immutable_version_update")
        bad = dict(contents[1], grading=canary)
        db.execute("UPDATE quiz_versions SET content=? WHERE quiz_id='quiz1'", (encode(bad),))
        db.execute("CREATE TRIGGER immutable_version_update BEFORE UPDATE ON quiz_versions BEGIN SELECT RAISE(ABORT,'quiz versions are immutable'); END")
        db.execute("INSERT INTO corrections VALUES(?,?,?,?,?,?,?)", ("correction0", "event0", canary, 1, now, go_card(transition["after"]), go_card(transition["before"])))
        db.execute("INSERT INTO semantic_assessments(id,presentation_id,operation_id,content_version,schedule_version,answer,status,policy_version,request_model,request_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
                   ("assessment0", "presentation0", "assessment-operation", 1, 1, canary, "failed", "short-v1", "synthetic", "{}", now))
        for index in range(2):
            db.execute("INSERT INTO source_documents VALUES(?,?,?,?,?,?,?,?,?,?,?)", (f"web{index}", "source0", "job0", "page", index, "https://synthetic.invalid", canary, "", canary, "synthetic", now))
    else:
        db.execute("UPDATE review_events SET rating=0,assisted=0,outcome='correct',schedule_version_after=99,due_at=123")
    db.execute("CREATE TRIGGER immutable_review_update BEFORE UPDATE ON review_events BEGIN SELECT RAISE(ABORT,'review events are immutable'); END")
elif mode.startswith("diagnostic-event-"):
    # Independent, parseable nested faults. In the middle-dataset case, rows
    # before and after the mismatched snapshot must both contribute counts.
    fault = mode.removeprefix("diagnostic-event-")
    assert fault in {"version", "grading", "combined", "nested", "middle"}
    bad: dict[str, object] = dict(snapshot)
    if fault != "grading":
        bad["version"] = 99
    if fault != "version":
        bad["grading"] = canary
    if fault in {"nested", "middle"}:
        bad.update(kind=canary, basis=canary, evidence=[1])
    db.execute("DROP TRIGGER immutable_review_update")
    db.execute("UPDATE review_events SET snapshot=?", (encode(bad),))
    db.execute("CREATE TRIGGER immutable_review_update BEFORE UPDATE ON review_events BEGIN SELECT RAISE(ABORT,'review events are immutable'); END")
    if fault == "middle":
        for label, when, content in [("before", transition["now_ms"] - 1, dict(snapshot, grading=canary)),
                                     ("after", transition["now_ms"] + 1, dict(snapshot, basis=canary))]:
            presentation_id = "presentation-" + label
            db.execute("INSERT INTO presentations SELECT ?,quiz_id,content_version,schedule_version,snapshot,created_at,answer,outcome,assisted,graded,rating,due_at,reviewed_at,review_id FROM presentations WHERE id='presentation0'", (presentation_id,))
            db.execute("INSERT INTO review_events SELECT ?,?,?,answer,outcome,rating,assisted,?,due_at,algorithm,schedule_before,schedule_after,schedule_version_before,schedule_version_after,grading FROM review_events WHERE id='event0'",
                       ("event-" + label, presentation_id, encode(content), when))
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
elif mode == "private-shape-error":
    db.execute("DROP TRIGGER immutable_version_update")
    bad = dict(contents[1], rubric={"required": canary})
    db.execute("UPDATE quiz_versions SET content=? WHERE quiz_id='quiz1'", (encode(bad),))
    db.execute("CREATE TRIGGER immutable_version_update BEFORE UPDATE ON quiz_versions BEGIN SELECT RAISE(ABORT,'quiz versions are immutable'); END")
elif mode == "legacy-generation-reservations":
    db.execute("DELETE FROM job_attempts")
    for index, reserve in enumerate([0, 1, 500000, 900000]):
        db.execute("INSERT INTO job_attempts(token,job_id,number,started_at,finished_at,reserved_micros,cost_micros,state) VALUES(?,?,?,?,?,?,?,?)",
                   (f"legacy-attempt{index}", "job0", index + 1, money_now, money_now + 1, reserve, None, "unknown"))
elif mode in {"settled-generation", "unknown-generation", "unsupported-old-active"}:
    db.execute("UPDATE job_attempts SET started_at=?,finished_at=?", (money_now, money_now + 1))
    if mode == "settled-generation":
        db.execute("UPDATE job_attempts SET reserved_micros=0,cost_micros=CASE WHEN token='attempt0' THEN 1000 ELSE 0 END")
    else:
        db.execute("UPDATE job_attempts SET cost_micros=NULL,state=CASE WHEN token='attempt0' THEN 'active' ELSE 'unknown' END")
        if mode == "unsupported-old-active":
            db.execute("UPDATE job_attempts SET started_at=? WHERE state='active'", (money_now - 86400001,))
elif mode.startswith("critic-") or mode.startswith("unsupported-critic-") or mode.startswith("legacy-critic-"):
    # All records, including retries of the same candidate, remain independent
    # paid obligations. The source's native SQL below is the allowance oracle.
    entries = [("judged", 1, 700000, 0, money_now), ("failed", 1, None, 500000, money_now)]
    if mode == "critic-lifecycles":
        entries += [("failed", 1, 0, 0, money_now), ("pending", 1, None, 500000, money_now),
                    ("pending", 0, None, 0, money_now), ("failed", 0, 0, 0, money_now),
                    ("judged", 1, 200000, 0, money_now - 86400001),
                    ("failed", 1, None, 500000, money_now - 86400001)]
    elif mode == "critic-budget":
        entries[0] = ("judged", 1, 3100000, 0, money_now)
    elif mode in {"legacy-critic-reservations", "legacy-critic-budget"}:
        entries = [("failed", 1, None, reserve, money_now) for reserve in
                   [0, 1, 500000, 2500001 if mode == "legacy-critic-budget" else 900000]]
    else:
        assert mode in {"critic-spend", "unsupported-critic-purpose", "unsupported-critic-owner",
                        "unsupported-critic-unsent", "unsupported-critic-old-pending"}
    for index, (status, sent, cost, reserve, when) in enumerate(entries):
        # Separate retry claims for one candidate: never collapse to latest.
        token = f"critic-attempt{index}"
        db.execute("INSERT INTO job_attempts(token,job_id,number,started_at,finished_at,reserved_micros,cost_micros,state) VALUES(?,?,?,?,?,?,?,?)",
                   (token, "job0", index + 2, now, now + 1, 500000, 0, "settled"))
        db.execute("INSERT INTO content_assessments(id,job_id,attempt_token,candidate_index,candidate_json,status,policy_version,request_model,request_json,created_at,finished_at,transmissions,cost_micros,reserved_micros) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
                   (f"critic{index}", "job0", token, 0, "{}", status, "critic-v1", "synthetic-critic-model" if sent else "", "{}", when, when + 1 if status != "pending" else 0, sent, cost, reserve))
    if mode == "unsupported-critic-purpose":
        db.execute("UPDATE content_assessments SET purpose='dedupe' WHERE id='critic0'")
    elif mode == "unsupported-critic-owner":
        db.execute("UPDATE content_assessments SET attempt_token='attempt1' WHERE id='critic0'")
    elif mode == "unsupported-critic-unsent":
        db.execute("UPDATE content_assessments SET transmissions=0 WHERE id='critic1'")
    elif mode == "unsupported-critic-old-pending":
        db.execute("UPDATE content_assessments SET status='pending',created_at=? WHERE id='critic1'", (money_now - 86400001,))
else:
    assert mode == "valid"
db.commit()
assert db.execute("PRAGMA integrity_check").fetchall() == [("ok",)]
assert db.execute("PRAGMA foreign_key_check").fetchall() == []
data = db.serialize()
manifest = {"format": 1, "id": "scry-20261015T003631.153157340Z-" + "a" * 32,
            "created_at": money_now, "database": {"schema": 5, "application_id": 0x53435259},
            "integrity": "ok", "bytes": len(data), "sha256": sha(data)}
archive = io.BytesIO()
with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_STORED) as z:
    z.writestr("manifest.json", encode(manifest))
    z.writestr("scry.sqlite", data)
print(json.dumps({"bytes": base64.b64encode(archive.getvalue()).decode(),
                  "photo_sha256": sha(photo), "transition": transition, "original_content": contents[0],
                  "spend_at_ms": money_now,
                  "conservative_spent_micros": db.execute("SELECT COALESCE((SELECT sum(COALESCE(cost_micros,max(reserved_micros,500000))) FROM job_attempts WHERE started_at>=? OR state='active'),0)+COALESCE((SELECT sum(COALESCE(cost_micros,max(reserved_micros,500000))) FROM content_assessments WHERE transmissions>0 AND (created_at>=? OR status='pending')),0)",
                                                          (money_now - 86400000, money_now - 86400000)).fetchone()[0],
                  "spent_micros": db.execute("SELECT COALESCE((SELECT sum(COALESCE(cost_micros,reserved_micros)) FROM job_attempts WHERE started_at>=? OR state='active'),0)+COALESCE((SELECT sum(COALESCE(cost_micros,reserved_micros)) FROM content_assessments WHERE created_at>=? OR status='pending'),0)",
                                             (money_now - 86400000, money_now - 86400000)).fetchone()[0]}))
