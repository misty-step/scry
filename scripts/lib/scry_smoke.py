"""Exercise a supplied Scry binary using only loopback and synthetic local state."""
from contextlib import contextmanager
import hashlib
from html.parser import HTMLParser
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import socket
import struct
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request


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


def exported(value, *, learning_only=False):
    # Export time and scheduled backup receipts are not learning mutations.
    omitted = {"exported_at", "backups"} if learning_only else {"exported_at"}
    return {key: item for key, item in value.items() if key not in omitted}


def static_linux_amd64(binary):
    data = binary.read_bytes()
    require(data[:7] == b"\x7fELF\x02\x01\x01", "binary is not little-endian ELF64")
    require(struct.unpack_from("<H", data, 18)[0] == 62, "binary is not x86-64")
    table = struct.unpack_from("<Q", data, 32)[0]
    size, count = struct.unpack_from("<HH", data, 54)
    require(size >= 4 and table + size * count <= len(data), "invalid ELF program headers")
    require(all(struct.unpack_from("<I", data, table + size * n)[0] != 3 for n in range(count)),
            "binary requires a dynamic interpreter")


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

        def cli(*arguments):
            result = subprocess.run([str(binary), *map(str, arguments)], cwd=work, env=env,
                                    capture_output=True, timeout=300)
            require(result.returncode == 0,
                    f"{arguments[0]} failed: {result.stderr.decode(errors='replace')}")
            return result.stdout

        def request(path, data=None, accept="application/json"):
            headers = {"Accept": accept}
            if data is not None:
                headers.update({"Origin": origin, "Content-Type": "application/x-www-form-urlencoded"})
            request = urllib.request.Request(origin + path, data=data, headers=headers)
            with client.open(request, timeout=5) as response:
                require(response.status == 200, f"{path} returned {response.status}")
                return response.read(), response.headers.get("Content-Type", "")

        def api(path, data=None):
            body, content_type = request(path, data)
            require("application/json" in content_type, f"{path} did not return JSON")
            return json.loads(body)

        @contextmanager
        def server(db, label):
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
        require(fixture["model"] == "authored-test-fixture", "fixture was not authored synthetic data")
        with server(database, "initial"):
            body, _ = request("/healthz", accept="text/plain")
            require(body == b"ok\n", "health response differs")
            page_bytes, content_type = request("/", accept="text/html")
            require("text/html" in content_type, "UI was not rendered HTML")
            page = Page()
            page.feed(page_bytes.decode())
            require("/review/answer" in page.forms, "rendered UI has no answer form")
            required_assets = {"/assets/app.css", "/assets/app.js", "/assets/htmx-2.0.10.min.js"}
            require(required_assets <= page.assets, "UI is missing its embedded asset references")
            asset_hashes = {}
            for path in sorted(page.assets | {"/assets/htmx-LICENSE"}):
                asset, _ = request(path, accept="*/*")
                require(asset == (source / "internal/web" / path.lstrip("/")).read_bytes(),
                        f"{path} differs from the supplied source bytes")
                asset_hashes[path] = hashlib.sha256(asset).hexdigest()
            state = api("/")
            current = state["review"]["current"]
            require(current is not None and not current["graded"], "fixture is not awaiting an answer")
            require(not current["quiz"]["answer"], "unanswered review exposed its answer")
            before = api("/export")
            require(before["review_events"] == [], "synthetic database already contains reviews")
            quiz = next(item for item in before["quizzes"] if item["id"] == current["quiz"]["id"])
            content = next(item["content"] for item in before["quiz_versions"]
                           if item["quiz_id"] == quiz["id"] and item["version"] == current["quiz"]["version"])
            schedule = next(item for item in before["schedules"] if item["quiz_id"] == quiz["id"])
            payload = urllib.parse.urlencode({
                "presentation_id": current["id"], "operation_id": state["operation_id"],
                "answer": content["answer"], "csrf": state["csrf"],
            }).encode()
            graded = api("/review/answer", payload)["review"]["current"]
            require(graded["graded"] and graded["outcome"] == "correct" and not graded["assisted"],
                    "submitted correct answer was not saved as unassisted success")
            require(graded["due_at"] > graded["reviewed_at"], "grade did not advance the due time")
            saved = api("/export")
            require(len(saved["review_events"]) == 1 and saved["review_events"][0]["id"] == graded["review_id"],
                    "grade did not create exactly its one durable review event")
            after_schedule = next(item for item in saved["schedules"] if item["quiz_id"] == quiz["id"])
            require(after_schedule["version"] == schedule["version"] + 1, "grade did not advance schedule exactly once")
            learning_state = exported(saved, learning_only=True)
            require(api("/review/answer", payload)["review"]["current"] == graded,
                    "exact answer retry changed the saved feedback")
            require(exported(api("/export"), learning_only=True) == learning_state,
                    "exact answer retry changed durable learning state")
        with server(database, "restart"):
            require(api("/")["review"]["current"] == graded, "saved feedback did not survive restart")
            require(api("/review/answer", payload)["review"]["current"] == graded,
                    "identical pre-restart request did not return its saved grade")
            require(exported(api("/export"), learning_only=True) == learning_state,
                    "restart or persisted exact retry changed learning state")
        before_path = work / "before.json"
        cli("export", "--db", database, "--output", before_path)
        baseline = exported(json.loads(before_path.read_bytes()))
        backup = json.loads(cli("backup", "--db", database))
        snapshot = Path(backup["path"])
        require(snapshot.is_relative_to(work) and snapshot.is_file() and not backup["error"], "backup did not complete locally")
        require(not backup["remote"], "synthetic smoke unexpectedly used remote backup authority")
        require(hashlib.sha256(snapshot.read_bytes()).hexdigest() == backup["sha256"], "backup checksum differs")
        restore_result = json.loads(cli("restore", "--snapshot", snapshot, "--destination", restored))
        require(restore_result["restored"] and not restore_result["activated"], "restore did not remain isolated")
        integrity = json.loads(cli("check", "--db", restored))
        require(integrity["integrity"] == "ok" and integrity["compatible"], "restored database is incompatible or corrupt")
        restored_path = work / "restored.json"
        cli("export", "--db", restored, "--output", restored_path)
        recovered = exported(json.loads(restored_path.read_bytes()))
        # Compare every exported field, including earlier backup receipts; only
        # exported_at is different. The new receipt is recorded after snapshot.
        require(recovered == baseline, "backup/restore did not preserve complete exported state")
        (output / "smoke-before-backup.json").write_bytes(canonical(baseline))
        (output / "smoke-restored.json").write_bytes(canonical(recovered))
        with server(restored, "restored"):
            require(api("/")["review"]["current"] == graded, "restored UI lost the saved feedback")
            require(api("/review/answer", payload)["review"]["current"] == graded,
                    "restored database lost the exact-operation receipt")
            require(exported(api("/export"), learning_only=True) == learning_state,
                    "restored API or exact retry changed learning state")
        return {
            "binary_version": version, "elf": "ELF64 x86-64 without dynamic interpreter",
            "smoke": {
                "scope": "isolated synthetic SQLite, loopback development identity, no remote credentials",
                "health": "ok", "readiness": "ready", "rendered_answer_form": True,
                "embedded_assets_sha256": asset_hashes, "grade": graded,
                "exact_retry": "unchanged feedback and full learning export",
                "restart": "same feedback, operation receipt and learning export",
                "restore": "full CLI export equality except exported_at; same API feedback and exact retry",
                "backup_sha256": backup["sha256"],
                "learning_state_sha256": hashlib.sha256(canonical(learning_state)).hexdigest(),
                "restored_export_sha256": hashlib.sha256(canonical(recovered)).hexdigest(),
                "remote_backup_exercised": False,
            },
        }
