"""Exercise the supplied Scry binary using loopback and labeled synthetic state.

CLI export is a privileged synthetic oracle, never browser privacy evidence.
The HTTP client below is not native-browser, private-ingress, or provider proof.
"""
from contextlib import closing, contextmanager
import hashlib
from html.parser import HTMLParser
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import socket
import sqlite3
import struct
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile


class Page(HTMLParser):
    def __init__(self):
        super().__init__()
        self.assets = set()
        self.forms = set()

    def handle_starttag(self, tag, attributes):
        attributes = dict(attributes)
        for key in ("src", "href"):
            value = attributes.get(key, "")
            if value.startswith("/assets/"):
                self.assets.add(value)
        if tag == "form":
            self.forms.add(attributes.get("action"))


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, message, headers, newurl):
        raise RuntimeError(f"unexpected smoke redirect: {code} {request.full_url}")


def require(condition, message):
    if not condition:
        raise RuntimeError("binary smoke: " + message)


def canonical(value):
    return (json.dumps(value, sort_keys=True, indent=2) + "\n").encode()


def digest(value):
    return hashlib.sha256(canonical(value)).hexdigest()


def exported(value):
    # Every durable section, including backup receipts and new v2 history, stays.
    return {key: item for key, item in value.items() if key != "exported_at"}


def static_linux_amd64(binary):
    data = binary.read_bytes()
    require(data[:7] == b"\x7fELF\x02\x01\x01", "binary is not little-endian ELF64")
    require(struct.unpack_from("<H", data, 18)[0] == 62, "binary is not x86-64")
    table = struct.unpack_from("<Q", data, 32)[0]
    size, count = struct.unpack_from("<HH", data, 54)
    require(size >= 4 and table + size * count <= len(data), "invalid ELF program headers")
    require(all(struct.unpack_from("<I", data, table + size * n)[0] != 3 for n in range(count)),
            "binary requires a dynamic interpreter")


def raw_tables(path):
    """Read every legacy column verbatim, including raw JSON whitespace."""
    with closing(sqlite3.connect(path.as_uri() + "?mode=ro", uri=True)) as db:
        db.row_factory = sqlite3.Row
        names = [row[0] for row in db.execute(
            "SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")]
        return {name: [dict(row) for row in db.execute(f'SELECT * FROM "{name}" ORDER BY rowid')]
                for name in names}


def migration_smoke(cli, source, work, output):
    legacy = work / "genuine-go-v1.sqlite"
    fixture = source / "internal/recovery/testdata/go-v1.sql"
    with closing(sqlite3.connect(legacy)) as db:
        db.executescript(fixture.read_text())
        identity = db.execute("PRAGMA application_id").fetchone()[0]
        version = db.execute("PRAGMA user_version").fetchone()[0]
    original = legacy.read_bytes()
    original_hash = hashlib.sha256(original).hexdigest()
    before = raw_tables(legacy)
    require(identity == 1396920921 and version == 1 and "materials" not in before,
            "legacy fixture is not a genuine populated Go v1 schema")
    require(before["review_events"] and before["corrections"] and before["job_attempts"],
            "legacy fixture lacks history and uncertain work")
    cli("check", "--db", legacy, succeeds=False)
    accepted = json.loads(cli("check", "--db", legacy, "--allow-migration"))
    require(accepted["accepted"] and not accepted["compatible"] and accepted["source_schema"] == 1
            and accepted["target_schema"] == 2 and accepted["migration_required"]
            and accepted["read_only"] and not accepted["migrated"],
            "compatible preflight pretended to migrate or run v1 without migration")
    require(legacy.read_bytes() == original, "preflight modified the genuine v1 source")
    require(all(not Path(str(legacy) + suffix).exists() for suffix in ("-wal", "-shm", "-journal")),
            "read-only closed v1 preflight created SQLite sidecars")
    archive = work / "genuine-go-v1.scry-backup.zip"
    manifest = {
        "format": 1, "id": "scry-20240101T000000.000000000Z-00000000000000000000000000000000",
        "created_at": 1704067200000,
        "database": {"schema": version, "application_id": identity, "sqlite": sqlite3.sqlite_version},
        "bytes": len(original), "sha256": original_hash, "integrity": "ok",
        "binary": {"module": "github.com/misty-step/scry", "version": "authored-legacy-fixture",
                   "go_version": "synthetic fixture, no second binary", "sha256": "0" * 64, "modified": False},
        "assets": "synthetic fixture; no legacy release binary supplied",
        "configuration": [], "restore_policy": "Unused isolated destination; uncertain work remains paused",
    }
    with zipfile.ZipFile(archive, "x", compression=zipfile.ZIP_STORED) as bundle:
        bundle.writestr("manifest.json", canonical(manifest))
        bundle.writestr("scry.sqlite", original)
    archive_hash = hashlib.sha256(archive.read_bytes()).hexdigest()
    observations = []
    for name, candidate in (("closed", legacy), ("archive", archive)):
        destination = work / f"migrated-{name}.sqlite"
        result = json.loads(cli("restore", "--snapshot", candidate, "--destination", destination))
        require(result["restored"] and not result["activated"], "v1 recovery activated a service")
        checked = json.loads(cli("check", "--db", destination))
        require(checked["compatible"] and checked["source_schema"] == 2 and not checked["migration_required"],
                "prepared recovery is not strictly current schema")
        after = raw_tables(destination)
        paused = []
        for table, rows in before.items():
            original_ids = {row.get("id") for row in rows} if table == "jobs" else None
            retained = [row for row in after[table] if original_ids is None or row["id"] in original_ids]
            require(len(retained) == len(rows), f"v1 migration lost or invented old {table} rows")
            for old, new in zip(rows, retained):
                expected = dict(old)
                if table == "jobs" and old["status"] in {"queued", "retry", "running"}:
                    require(new["updated_at"] >= old["updated_at"], "restored job timestamp moved backwards")
                    expected.update(status="paused", lease_token="", lease_until=0,
                                    error="Restored work paused: reconcile prior requests and spend before explicit retry",
                                    updated_at=new["updated_at"])
                    paused.append(old["id"])
                if table == "job_attempts" and old["state"] == "active":
                    require(new["finished_at"] >= old["started_at"] and new["cost_micros"] is None,
                            "restored uncertain request lost its unknown spend")
                    expected.update(state="unknown", finished_at=new["finished_at"])
                require({key: new[key] for key in old} == expected,
                        f"v1 recovery changed retained {table} fields beyond the explicit job-pause contract")
        new_jobs = [row for row in after["jobs"] if row["id"] not in {row["id"] for row in before["jobs"]}]
        require(all(row["kind"] == "enrich" and row["status"] == "paused" and row["attempts"] == 0
                    for row in new_jobs), "migration created runnable or attempted external work")
        require(not after["knowledge_units"] and not after["material_links"] and not after["estimate_records"],
                "migration fabricated knowledge coverage or learner estimates")
        require({row["id"] for row in after["interactions"]} == {row["id"] for row in before["review_events"]},
                "migration manufactured observations beyond retained direct review history")
        recovered = exported(json.loads(cli("export", "--db", destination)))
        require(recovered["schema_version"] == 2 and recovered["format_version"] == 2,
                "migrated CLI export is not v2")
        (output / f"smoke-v1-{name}-migrated.json").write_bytes(canonical(recovered))
        observations.append({"input": name, "source_schema": 1, "prepared_schema": 2,
                             "raw_legacy_rows_preserved": {table: len(rows) for table, rows in before.items()},
                             "paused_job_ids": paused, "unattempted_enrichment_job_ids": [row["id"] for row in new_jobs],
                             "export_sha256": digest(recovered)})
    require(legacy.read_bytes() == original and hashlib.sha256(archive.read_bytes()).hexdigest() == archive_hash,
            "restore modified its legacy source or archive")
    # Correct checksum is not authority to lie about the embedded identity.
    mismatch = work / "mismatched-v1.zip"
    manifest["database"]["schema"] = 2
    with zipfile.ZipFile(mismatch, "x", compression=zipfile.ZIP_STORED) as bundle:
        bundle.writestr("manifest.json", canonical(manifest))
        bundle.writestr("scry.sqlite", original)
    rejected = work / "must-not-publish.sqlite"
    cli("restore", "--snapshot", mismatch, "--destination", rejected, succeeds=False)
    require(all(not Path(str(rejected) + suffix).exists() for suffix in ("", "-wal", "-shm", "-journal")),
            "mismatched manifest left a published destination")
    return {"fixture_sha256": hashlib.sha256(fixture.read_bytes()).hexdigest(),
            "source_sha256": original_hash, "archive_sha256": archive_hash,
            "preflight": accepted, "restores": observations,
            "mismatched_manifest": "rejected without destination or sidecars"}


def smoke(binary, source, output, revision):
    static_linux_amd64(binary)
    with tempfile.TemporaryDirectory(prefix="scry-smoke-") as temporary:
        work = Path(temporary)
        database, restored = work / "synthetic.sqlite", work / "restored.sqlite"
        env = {
            "PATH": os.defpath, "HOME": str(work), "LANG": "C.UTF-8",
            "SCRY_SECRET": secrets.token_hex(32),
            "SCRY_BACKUP_DIR": str(work / "backups"),
            "SCRY_BACKUP_INTERVAL": "24h", "SCRY_BACKUP_KEEP": "30",
        }
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            port = listener.getsockname()[1]
        origin = f"http://127.0.0.1:{port}"
        client = urllib.request.build_opener(
            urllib.request.ProxyHandler({}), NoRedirect(),
            urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
        )

        def cli(*arguments, succeeds=True):
            result = subprocess.run([str(binary), *map(str, arguments)], cwd=work, env=env,
                                    capture_output=True, timeout=300)
            require((result.returncode == 0) == succeeds,
                    f"{arguments[0]} returned {result.returncode}: {result.stderr.decode(errors='replace')}")
            return result.stdout

        def durable(db=database):
            return exported(json.loads(cli("export", "--db", db)))

        def request(path, data=None, accept="application/json", status=200):
            headers = {"Accept": accept}
            if data is not None:
                headers.update({"Origin": origin, "Content-Type": "application/x-www-form-urlencoded"})
            req = urllib.request.Request(origin + path, data=data, headers=headers)
            try:
                response = client.open(req, timeout=5)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                require(response.status == status, f"{path} returned {response.status}, wanted {status}")
                return response.read(), response.headers.get("Content-Type", "")

        def api(path, data=None, status=200):
            body, content_type = request(path, data, status=status)
            require("application/json" in content_type, f"{path} did not return JSON")
            return json.loads(body)

        def payload(state, **fields):
            return urllib.parse.urlencode({"csrf": state["csrf"], "operation_id": state["operation_id"], **fields}).encode()

        def post(path, state, **fields):
            return api(path, payload(state, **fields))

        def same_scheduling(before, after, action):
            require(before["schedules"] == after["schedules"] and before["review_events"] == after["review_events"],
                    f"{action} manufactured a direct FSRS transition or review event")

        def same_observations(before, after, action):
            same_scheduling(before, after, action)
            require(before["interactions"] == after["interactions"], f"{action} manufactured an interaction")

        @contextmanager
        def server(db, label):
            prior = durable(db)
            with (output / f"smoke-{label}.log").open("wb") as log:
                process = subprocess.Popen(
                    [str(binary), "serve", "--dev", "--db", str(db), "--addr", f"127.0.0.1:{port}"],
                    cwd=work, env=env, stdout=log, stderr=subprocess.STDOUT,
                )
                try:
                    deadline = time.monotonic() + 30
                    while True:
                        require(process.poll() is None, f"server exited; see smoke-{label}.log")
                        try:
                            body, _ = request("/readyz", accept="text/plain")
                            require(body == b"ready\n", "readiness response differs")
                            break
                        except (urllib.error.URLError, TimeoutError):
                            require(time.monotonic() < deadline, f"server did not become ready; see smoke-{label}.log")
                            time.sleep(0.1)
                    # Observe the scheduled startup receipt rather than deleting
                    # the backups section from retry/restart comparisons.
                    while True:
                        started = durable(db)
                        if len(started["backups"]) > len(prior["backups"]):
                            break
                        require(time.monotonic() < deadline, "startup backup did not complete")
                        time.sleep(0.1)
                    require(started["backups"][:-1] == prior["backups"] and not started["backups"][-1]["error"]
                            and not started["backups"][-1]["remote"], "startup backup did not append one local receipt")
                    require(started == {**prior, "backups": started["backups"]},
                            "restart changed durable state beyond its observed startup backup receipt")
                    yield
                finally:
                    if process.poll() is None:
                        process.terminate()
                    try:
                        status = process.wait(timeout=25)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait()
                        raise RuntimeError("binary smoke: server did not drain within 25 seconds")
                    require(status == 0, f"server did not shut down cleanly ({status}); see smoke-{label}.log")

        version = cli("version").decode().strip()
        require(version == f"scry {revision}", "binary revision differs from supplied source")
        fixture = json.loads(cli("seed-fixture", "--db", database))
        require(fixture["model"] == "authored-test-fixture" and fixture["synthetic"]
                and fixture["provider_cost_micros"] == 0, "fixture fabricated external provider work")
        oracle = durable()  # Privileged, before any server or answer-bearing HTTP.
        require(oracle["schema_version"] == 2 and oracle["format_version"] == 2, "fixture/export is not schema v2")
        require(not oracle["review_events"] and not oracle["interactions"] and not oracle["inspection_events"],
                "authored fixture fabricated observations")
        require(all(row["reserved_micros"] == 0 and row["cost_micros"] == 0 and row["state"] == "settled"
                    for row in oracle["job_attempts"]), "authored fixture fabricated spend or uncertain provider usage")
        require(all(row["estimate_json"]["state"] == "unknown" for row in oracle["estimate_records"]),
                "authored fixture fabricated learner mastery")
        goal_id = fixture["goal"]["id"]
        goal_path = f"/goals/{goal_id}/plan"
        materials = {item["id"]: item for item in fixture["goal"]["materials"]}
        contents = {(item["quiz_id"], item["version"]): item["content"] for item in oracle["quiz_versions"]}
        target_id = next(item["id"] for item in materials.values() if item["kind"] == "quiz" and item["level"] == "target")
        foundations = {item["id"] for item in materials.values() if item["kind"] == "quiz" and item["level"] == "foundation"}
        require(len(foundations) == 2 and {item["kind"] for item in materials.values()} ==
                {"quiz", "explanation", "worked_example", "diagram", "article"}, "fixture omitted useful mixed materials")
        require({row["role"] for row in oracle["material_links"]} == {"assesses", "teaches", "assumes", "mentions"},
                "fixture omitted typed many-to-many coverage")

        def answer_for(current):
            return contents[(current["quiz"]["id"], current["quiz"]["version"])]["answer"]

        def plan(focus):
            state = api(goal_path)
            before = durable()
            result = post(goal_path, state, version=state["goal"]["revision"], time_budget_seconds=3600,
                          new_assessments_per_day=100, focus=focus, reason="Synthetic exact-binary pacing exercise")
            require(result["goal_id"] == goal_id, "planning changed the wrong goal")
            same_observations(before, durable(), "planning")

        def choose(kind):
            before = durable()
            candidates = [item for item in before["suggestions"] if item["kind"] == kind and item["status"] == "pending"]
            require(len(candidates) == 1, f"no current undecided {kind} suggestion")
            item = candidates[0]
            post(f"/suggestions/{item['id']}/choose", api(goal_path), action="accept")
            after = durable()
            require(next(row for row in after["suggestions"] if row["id"] == item["id"])["status"] == "accepted",
                    "explicit suggestion choice was not persisted")
            require(after["jobs"] == before["jobs"] and after["job_attempts"] == before["job_attempts"],
                    "already useful authored scope unexpectedly started external generation")
            same_observations(before, after, "suggestion acceptance")
            return item["id"]

        def guarded_export(db=database):
            before = durable(db)
            gate = api("/export", status=409)
            require(gate["kind"] == "export" and gate["id"] == "" and gate["action"] == "/inspection/assist"
                    and gate["return_to"] == "/export" and gate["access"]["requires_assistance"],
                    "cold HTTP export did not require explicit scoped assistance")
            page_bytes, content_type = request("/export", accept="text/html")
            require("text/html" in content_type, "cold HTML export returned private JSON")
            page = Page()
            page.feed(page_bytes.decode())
            require("/inspection/assist" in page.forms, "cold HTML export has no explicit inspection fence")
            for content in contents.values():
                require(content["prompt"] not in page_bytes.decode() and content["prompt"] not in json.dumps(gate),
                        "cold export gate disclosed a protected question")
            require("quiz_versions" not in page_bytes.decode() and "quiz_versions" not in gate,
                    "cold export gate disclosed the full export")
            require(durable(db) == before, "denied HTTP/HTML export changed durable state")
            return gate

        with server(database, "initial"):
            body, _ = request("/healthz", accept="text/plain")
            require(body == b"ok\n", "health response differs")
            plan("practice")
            state = api("/")
            current = state["review"]["current"]
            require(current is not None and current["kind"] == "quiz" and current["quiz"]["id"] in foundations
                    and current["quiz"]["kind"] == "recall" and not current["graded"]
                    and not current["assisted"] and not current["practice"], "fixture did not admit an unassisted foundation recall")
            require(not current["quiz"]["answer"] and not current["quiz"]["explanation"], "cold review disclosed an answer")
            page_bytes, content_type = request("/", accept="text/html")
            require("text/html" in content_type, "UI was not rendered HTML")
            page = Page()
            page.feed(page_bytes.decode())
            require("/review/answer" in page.forms, "rendered UI has no answer form")
            required_assets = {"/assets/app.css", "/assets/app.js", "/assets/htmx-2.0.10.min.js"}
            require(required_assets <= page.assets, "UI is missing embedded asset references")
            asset_hashes = {}
            for path in sorted(page.assets | {"/assets/htmx-LICENSE"}):
                asset, _ = request(path, accept="*/*")
                require(asset == (source / "internal/web" / path.lstrip("/")).read_bytes(),
                        f"{path} differs from supplied source bytes")
                asset_hashes[path] = hashlib.sha256(asset).hexdigest()
            guarded_export()
            before = durable()
            initial_payload = payload(state, presentation_id=current["id"], answer=answer_for(current))
            graded = api("/review/answer", initial_payload)["review"]["current"]
            require(graded["graded"] and graded["outcome"] == "correct" and not graded["assisted"] and graded["rating"] == 3,
                    "correct recall was not saved as unassisted direct success")
            require(graded["due_at"] > graded["reviewed_at"], "grade did not advance due time")
            saved = durable()
            require(len(saved["review_events"]) == 1 and saved["review_events"][0]["id"] == graded["review_id"],
                    "grade did not create exactly one durable review")
            require(len(saved["interactions"]) == 1 and saved["interactions"][0]["id"] == graded["review_id"],
                    "one direct review was duplicated into multiple interactions")
            for old, new in zip(before["schedules"], saved["schedules"]):
                if old["quiz_id"] == current["quiz"]["id"]:
                    require(new["version"] == old["version"] + 1, "grade did not advance its own schedule once")
                else:
                    require(new == old, "cross-item inference advanced another quiz schedule")
            assessed = {(link["unit_id"], link["unit_version"]) for link in materials[current["quiz"]["id"]]["links"]
                        if link["role"] == "assesses"}
            attributed = {}
            for pin in assessed:
                estimates = [row for row in saved["estimate_records"] if (row["unit_id"], row["unit_version"]) == pin
                             and row["mode"] == "recall" and row["estimate_json"]["state"] == "demonstrated"]
                require(estimates, "one multi-unit recall did not produce each direct estimate")
                estimate = estimates[-1]
                direct = estimate["estimate_json"]["direct"]
                require([item["evidence_id"] for item in direct] == [graded["review_id"]],
                        "multi-unit estimate duplicated or invented evidence")
                attributed[pin[0]] = {"estimate_id": estimate["id"], "evidence_id": graded["review_id"], "version": pin[1]}
            require(len(attributed) == 2, "recall did not assess two exact foundation definitions")
            require(api("/review/answer", initial_payload)["review"]["current"] == graded,
                    "exact retry changed saved feedback")
            require(durable() == saved, "exact retry changed any durable exported field")

        with server(database, "feedback-restart"):
            require(api("/")["review"]["current"] == graded, "held feedback did not survive restart")
            resumed = durable()
            require(api("/review/answer", initial_payload)["review"]["current"] == graded,
                    "persisted exact retry changed feedback")
            require(durable() == resumed, "persisted retry changed durable state")
            plan("goal")
            advance_id = choose("advance")
            state = post("/review/next", api("/"), presentation_id=graded["id"])
            target = state["review"]["current"]
            require(target["quiz"]["id"] == target_id and target["quiz"]["kind"] == "choice" and not target["graded"]
                    and target["practice"] and target["assisted"],
                    "chosen integrated target did not preserve the prior feedback exposure as helped practice")
            deferred = durable()
            other = (foundations - {graded["quiz"]["id"]}).pop()
            decisions = [row for row in deferred["plan_decisions"] if row["kind"] == "defer" and row["material_id"] == other]
            require(len(decisions) == 1, "other-material direct evidence did not defer redundant foundation practice")
            decision = decisions[0]
            require(decision["evidence_ids_json"] == [graded["review_id"]] and decision["reason"]
                    and {(pin["id"], pin["version"]) for pin in decision["unit_versions_json"]} == assessed
                    and decision["reconsider_at"] == min(graded["due_at"], graded["reviewed_at"] + 7 * 86400000),
                    "deferral lacks exact IDs, rationale, or fixed original-event bound")
            same_scheduling(saved, deferred, "inference and foundation deferral")
            post(f"/plans/{decision['id']}/undo", state)
            undone = durable()
            require(next(row for row in undone["plan_decisions"] if row["id"] == decision["id"])["undone_at"] > 0,
                    "deferral undo did not retain and retire its decision")
            undo = next(row for row in undone["plan_decisions"] if row["undo_of"] == decision["id"])
            require(undo["evidence_ids_json"] == decision["evidence_ids_json"] and undo["reason"],
                    "deferral undo lost its inspectable evidence lineage")
            same_observations(deferred, undone, "deferral undo")
            state = post("/review/bridge", api("/"), presentation_id=target["id"])
            bridge = state["review"]["bridge"]
            require(bridge["status"] == "active" and bridge["path"] and not bridge["job_id"]
                    and bridge["target_presentation_id"] == target["id"]
                    and bridge["target_material_id"] == target_id, "Too advanced did not reuse useful retained foundations")
            require(any(item["id"] == target["id"] for item in state["review"]["suspended"]),
                    "bridge lost the suspended target")
            same_scheduling(undone, durable(), "Too advanced request and instruction delivery")
            lateral_id = choose("lateral")
            held_bridge = api("/")["review"]

        moments = []
        with server(database, "bridge-restart"):
            state = api("/")
            require(state["review"] == held_bridge, "bridge position or suspended target did not survive restart")
            bridge_before = durable()
            for _ in range(12):
                current = state["review"]["current"]
                if state["review"]["bridge"]["status"] == "ready_return" and any(item["kind"] == "article" for item in moments):
                    break
                if not current or (current["bridge_id"] != bridge["id"] and current["kind"] != "reference"):
                    break
                before = durable()
                if current["kind"] == "reference":
                    material = current["material"]
                    require(material["id"] in materials and material["version"] == materials[material["id"]]["version"],
                            "moment did not use a real retained material version")
                    if material["kind"] == "article":
                        require(material["basis"] == "reference" and not material["body"] and not material["evidence"]
                                and material["reference_url"] == "https://www.rfc-editor.org/rfc/rfc1035.html",
                                "link-only reference fabricated fetched content")
                    else:
                        require(material["body"] == materials[material["id"]]["body"], "instruction body changed")
                    if material["kind"] == "diagram":
                        require(material["diagram"] == materials[material["id"]]["diagram"], "durable diagram changed")
                    continued_payload = payload(state, presentation_id=current["id"])
                    state = api("/review/continue", continued_payload)
                    after = durable()
                    additions = [row for row in after["interactions"] if row["id"] not in {item["id"] for item in before["interactions"]}]
                    require(len(additions) == 1 and additions[0]["kind"] == "continue"
                            and additions[0]["material_id"] == material["id"], "Continue did not create one exact exposure")
                    require(api("/review/continue", continued_payload)["review"] == state["review"] and durable() == after,
                            "Continue retry duplicated exposure or changed persisted state")
                    moments.append({"presentation_id": current["id"], "material_id": material["id"],
                                    "version": material["version"], "kind": material["kind"], "interaction_id": additions[0]["id"]})
                else:
                    require(current["bridge_id"] == bridge["id"] and current["practice"] and current["assisted"],
                            "post-instruction bridge quiz was treated as cold retention")
                    answered = post("/review/answer", state, presentation_id=current["id"], answer=answer_for(current))
                    practice = answered["review"]["current"]
                    require(practice["graded"] and practice["outcome"] == "correct" and practice["rating"] == 0
                            and not practice["review_id"], "helped practice manufactured a scheduled review")
                    after = durable()
                    additions = [row for row in after["interactions"] if row["id"] not in {item["id"] for item in before["interactions"]}]
                    require(len(additions) == 1 and additions[0]["kind"] == "practice", "bridge practice was not one observation")
                    moments.append({"presentation_id": current["id"], "material_id": current["quiz"]["id"],
                                    "version": current["quiz"]["version"], "kind": "practice", "interaction_id": additions[0]["id"]})
                    state = post("/review/next", answered, presentation_id=current["id"])
                same_scheduling(bridge_before, durable(), "bridge exposure/practice/inference")
                active = state["review"].get("bridge")
                if active and active["status"] == "ready_return" and state["review"]["current"] is None:
                    break
            require({item["kind"] for item in moments} == {"explanation", "worked_example", "diagram", "article", "practice"},
                    "mixed review did not exercise every reusable instruction/reference kind and practice")
            require(state["review"]["bridge"]["status"] == "ready_return", "bridge did not finish its bounded useful path")
            returned = post("/review/return", state, bridge_id=bridge["id"])
            retained = returned["review"]["current"]
            require(retained["id"] == target["id"] and retained["quiz"]["id"] == target_id and retained["practice"]
                    and retained["assisted"] and not retained["graded"], "deliberate return replaced or automatically graded the target")
            same_scheduling(bridge_before, durable(), "deliberate target return")
            final_payload = payload(returned, presentation_id=retained["id"], answer=answer_for(retained))
            final_grade = api("/review/answer", final_payload)["review"]["current"]
            require(final_grade["graded"] and final_grade["outcome"] == "correct" and final_grade["practice"]
                    and final_grade["rating"] == 0 and not final_grade["review_id"], "returned helped target fabricated FSRS evidence")
            same_scheduling(bridge_before, durable(), "returned-target practice")
            final_saved = durable()
            require(api("/review/answer", final_payload)["review"]["current"] == final_grade and durable() == final_saved,
                    "returned-target exact retry changed feedback or state")
            # The goal is now explicitly inspectable; opening it is a separate
            # exposure, after the unassisted evidence/deferral assertions above.
            inspect = post("/inspection/assist", api(goal_path), kind="goal", id=goal_id, return_to=f"/goals/{goal_id}")
            require(inspect["inspection_recorded"], "explicit goal inspection was not acknowledged")
            goal_view = api(f"/goals/{goal_id}")["goal"]
            require(any(row["id"] == decision["id"] and row["reason"] and row["evidence_ids"] == [graded["review_id"]]
                        and row["undone_at"] > 0 for row in goal_view["decisions"]), "goal hid the real deferral and undo evidence")
            same_scheduling(final_saved, durable(), "explicit inspection and estimate read")

        before_path = work / "before.json"
        cli("export", "--db", database, "--output", before_path)
        baseline = exported(json.loads(before_path.read_bytes()))
        backup = json.loads(cli("backup", "--db", database))
        snapshot = Path(backup["path"])
        require(snapshot.is_relative_to(work) and snapshot.is_file() and not backup["error"], "local backup did not complete")
        require(not backup["remote"], "synthetic smoke unexpectedly used remote backup authority")
        require(hashlib.sha256(snapshot.read_bytes()).hexdigest() == backup["sha256"], "backup checksum differs")
        restore_result = json.loads(cli("restore", "--snapshot", snapshot, "--destination", restored))
        require(restore_result["restored"] and not restore_result["activated"], "restore did not remain isolated")
        integrity = json.loads(cli("check", "--db", restored))
        require(integrity["integrity"] == "ok" and integrity["compatible"] and not integrity["migration_required"],
                "restored database is incompatible or corrupt")
        restored_path = work / "restored.json"
        cli("export", "--db", restored, "--output", restored_path)
        recovered = exported(json.loads(restored_path.read_bytes()))
        require(all(row["status"] not in {"queued", "retry", "running"} for row in baseline["jobs"]),
                "terminal fixture unexpectedly contains work requiring restore pause differences")
        # Compare EVERY field and section. The new backup receipt is recorded
        # after the snapshot; only exported_at is excluded, never new v2 history.
        require(recovered == baseline, "backup/restore did not preserve complete every-field v2 export")
        require(hashlib.sha256(snapshot.read_bytes()).hexdigest() == backup["sha256"], "restore changed its archive")
        (output / "smoke-before-backup.json").write_bytes(canonical(baseline))
        (output / "smoke-restored.json").write_bytes(canonical(recovered))
        with server(restored, "restored"):
            require(api("/")["review"]["current"] == final_grade, "restored UI lost held returned-target feedback")
            stable = durable(restored)
            require(api("/review/answer", final_payload)["review"]["current"] == final_grade,
                    "restored database lost the exact operation receipt")
            require(durable(restored) == stable, "restored exact retry changed durable state")

        # Separate cold inspection rehearsal: never contaminate the evidence
        # used to establish cross-item unassisted recall and deferral above.
        inspection_db = work / "inspection.sqlite"
        inspected_fixture = json.loads(cli("seed-fixture", "--db", inspection_db))
        inspected_goal = inspected_fixture["goal"]["id"]
        with server(inspection_db, "inspection"):
            pace = api(f"/goals/{inspected_goal}/plan")
            post(f"/goals/{inspected_goal}/plan", pace, version=pace["goal"]["revision"], time_budget_seconds=300,
                 new_assessments_per_day=5, focus="practice", reason="Separate synthetic cold inspection rehearsal")
            cold = api("/")
            require(not cold["review"]["current"]["assisted"], "inspection rehearsal did not begin cold")
            gate = guarded_export(inspection_db)
            before = durable(inspection_db)
            opened = post("/inspection/assist", gate, kind=gate["kind"], id=gate["id"], return_to=gate["return_to"])
            require(opened["inspection_recorded"] and opened["location"] == "/export" and "review" not in opened,
                    "inspection grant returned protected content instead of a rechecked destination")
            helped = api("/")["review"]["current"]
            require(helped["id"] == cold["review"]["current"]["id"] and helped["assisted"] and helped["practice"]
                    and not helped["graded"] and not helped["quiz"]["answer"], "inspection failed to preserve and help the cold target")
            after = durable(inspection_db)
            same_scheduling(before, after, "explicit cold export assistance")
            require(exported(api("/export")) == after, "granted HTTP export differs from the complete privileged CLI export")
            grants = [row for row in after["inspection_events"] if row["kind"] == "export"]
            require(len(grants) == 1 and grants[0]["expires_at"] - grants[0]["at"] == 1800000,
                    "inspection scope did not retain its fixed short-lived expiry")
            inspection_proof = {"cold_presentation_id": helped["id"], "grant_id": grants[0]["id"],
                                "expires_at": grants[0]["expires_at"], "granted_at": grants[0]["at"],
                                "json_guard_status": 409, "html_guard_status": 200,
                                "ungraded_assisted_practice": helped["practice"],
                                "full_http_cli_export_sha256": digest(after)}

        migration = migration_smoke(cli, source, work, output)
        return {
            "binary_version": version, "elf": "ELF64 x86-64 without dynamic interpreter",
            "smoke": {
                "scope": "isolated synthetic SQLite and urllib loopback development identity; no remote credentials",
                "limitations": ["not native-browser interaction proof", "not private-ingress authentication proof",
                                "not live-provider quality or remote-backup proof", "not learner mastery or retention evidence"],
                "health": "ok", "readiness": "ready", "embedded_assets_sha256": asset_hashes,
                "source_id": fixture["source"], "goal_id": goal_id, "direct_grade": graded,
                "cross_item_attribution": attributed,
                "deferral": {"id": decision["id"], "material_id": other, "evidence_ids": decision["evidence_ids_json"],
                             "reason": decision["reason"], "reconsider_at": decision["reconsider_at"], "undo_id": undo["id"]},
                "selected_suggestion_ids": [advance_id, lateral_id],
                "bridge": {"id": bridge["id"], "target_presentation_id": target["id"], "path": bridge["path"],
                           "path_versions": bridge["path_versions"], "moments": moments, "returned_feedback": final_grade},
                "inspection": inspection_proof, "migration": migration,
                "exact_retry": "unchanged held feedback and every durable exported field",
                "restart": "held feedback and bridge position preserved; exactly one observed startup backup receipt added",
                "restore": "every v2 CLI export field equal except exported_at; held feedback and exact retry retained",
                "export_sections": sorted(baseline), "backup_sha256": backup["sha256"],
                "restored_export_sha256": digest(recovered), "remote_backup_exercised": False,
            },
        }
