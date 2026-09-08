#!/usr/bin/env python3
"""Exercise the real monitor against isolated HTTP health/mail boundaries."""
from contextlib import redirect_stdout
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.machinery
import importlib.util
import io
import json
import os
from pathlib import Path
import tempfile
from threading import Thread
from unittest.mock import patch


ROOT = Path(__file__).resolve().parent
LOADER = importlib.machinery.SourceFileLoader("scry_monitor", str(ROOT / "scry-monitor"))
SPEC = importlib.util.spec_from_loader(LOADER.name, LOADER)
MONITOR = importlib.util.module_from_spec(SPEC)
LOADER.exec_module(MONITOR)


class Provider(BaseHTTPRequestHandler):
    health = "healthy"
    mail_status = 200
    messages = []
    requests = []

    def log_message(self, *_args):
        pass

    def do_GET(self):
        assert self.path in ("/healthz", "/readyz", "/statusz"), "probe left the public health surface"
        self.requests.append((self.command, self.path, self.headers.get("Authorization"),
                              self.headers.get("Cookie"), self.headers.get("X-Admin-Token")))
        if self.health in ("paused", "not_ready") and self.path == "/readyz":
            self.respond(503, {"status": "paused"})
            return
        if self.path == "/statusz":
            if self.health == "redirect":
                self.respond(307, {})
                return
            if self.health == "malformed":
                self.respond(200, b'{"status":')
                return
            value = {"schema": MONITOR.HEALTH_SCHEMA, "status": "healthy",
                     "maintenance": False, "backupAgeMs": 0}
            if self.health == "missing_backup":
                value.pop("backupAgeMs")
            elif self.health == "boolean_backup":
                value["backupAgeMs"] = True
            elif self.health == "stale_backup":
                value["backupAgeMs"] = MONITOR.MAX_BACKUP_AGE_MS + 1
            elif self.health == "future_backup":
                value["backupAgeMs"] = -1
            elif self.health == "paused":
                value["maintenance"] = True
        else:
            value = {"status": "ok" if self.path == "/healthz" else "ready"}
        self.respond(200, value)

    def do_POST(self):
        assert self.path == "/emails"
        body = self.rfile.read(int(self.headers["Content-Length"]))
        self.messages.append((self.headers["Idempotency-Key"], body))
        self.respond(self.mail_status, {"id": "11111111-1111-4111-8111-111111111111"})

    def respond(self, status, value):
        body = value if isinstance(value, bytes) else json.dumps(value).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        if status == 307:
            self.send_header("Location", "https://must-not-receive-credentials.invalid/")
        self.end_headers()
        self.wfile.write(body)


def main():
    server = ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    origin = f"http://127.0.0.1:{server.server_port}"
    real_http_connection = MONITOR.http.client.HTTPConnection

    def mail_connection(host, _port=None, **kwargs):
        assert host == "api.resend.com", "a credential-bearing redirect was followed"
        return real_http_connection("127.0.0.1", server.server_port, **kwargs)

    try:
        with tempfile.TemporaryDirectory(prefix="scry-monitor-regression-") as directory:
            state = Path(directory) / "state.json"
            receipt = Path(directory) / "receipt.json"
            argv = [str(ROOT / "scry-monitor"), "--environment", "staging",
                    "--local-probe-origin", origin, "--state-file", str(state),
                    "--receipt-file", str(receipt)]

            def run(*extra):
                with patch("sys.argv", argv + list(extra)), redirect_stdout(io.StringIO()):
                    status = MONITOR.main()
                return status, json.loads(receipt.read_text())

            def status_only(*extra):
                output = io.StringIO()
                status_argv = [str(ROOT / "scry-monitor"), "--status", "--environment", "staging",
                               "--local-probe-origin", origin, *extra]
                with patch("sys.argv", status_argv), redirect_stdout(output):
                    status = MONITOR.main()
                return status, json.loads(output.getvalue())

            with patch.dict(os.environ, {}, clear=True):
                status, result = status_only()
                assert status == 0 and result["status"] == "healthy" and result["result"] == "ok"
                assert Provider.requests == [
                    ("GET", path, None, None, None) for path in ("/healthz", "/readyz", "/statusz")
                ], "read-only status requested private routes or sent ambient credentials"
                Provider.health = "not_ready"
                status, result = status_only()
                assert status == 1 and result["checks"][1]["errorCode"] == "response_non_2xx"
                assert result["checks"][-1]["status"] == "healthy", "readiness fixture also degraded recovery"
                assert list(Path(directory).iterdir()) == [], "status wrote notification state, locks, or receipts"
                for health, error_code in (
                    ("paused", "health_not_active"),
                    ("missing_backup", "health_backup_invalid_or_stale"),
                    ("stale_backup", "health_backup_invalid_or_stale"),
                    ("future_backup", "health_backup_invalid_or_stale"),
                    ("malformed", "response_invalid_json"),
                    ("redirect", "response_redirect"),
                ):
                    Provider.health = health
                    status, result = status_only()
                    assert status == 1 and result["status"] == "unhealthy"
                    assert result["checks"][-1]["errorCode"] == error_code
                    assert result["notifications"] == [] and result["errors"] == []
                assert Provider.messages == [], "read-only status attempted mail"
                assert list(Path(directory).iterdir()) == [], "unhealthy status had filesystem side effects"
                Provider.health = "healthy"
                status, result = status_only("--receipt-file", str(receipt))
                assert status == 0 and json.loads(receipt.read_text()) == result
                assert set(Path(directory).iterdir()) == {receipt}, "optional receipt created notification state"

            environment = {"RESEND_API_KEY": "isolated-provider-key",
                           "MEMORY_ENGINE_MAIL_FROM": "Scry <sender@example.test>",
                           "MEMORY_ENGINE_ALERT_TO": "operator@example.test"}
            with patch.dict(os.environ, environment, clear=True), patch.object(
                    MONITOR.http.client, "HTTPSConnection", side_effect=mail_connection):
                with patch.dict(os.environ, {"RESEND_API_KEY": ""}):
                    status, result = run()
                    assert status == 1 and result["status"] == "healthy"
                    assert "notification_configuration_invalid" in result["errors"]
                status, result = run()
                assert status == 0 and result["status"] == "healthy"
                assert Provider.messages == [], "first healthy observation sent mail"

                Provider.health = "missing_backup"
                Provider.mail_status = 503
                status, result = run()
                assert status == 1 and result["pending"]["health"]
                assert result["checks"][-1]["errorCode"] == "health_backup_invalid_or_stale"
                assert result["notifications"][0]["delivery"] == "acceptance_unconfirmed"
                assert json.loads(state.read_text())["notifiedStatus"] == "healthy"
                failed_request = Provider.messages[-1]
                pending_state = state.read_bytes()
                with patch.dict(os.environ, {}, clear=True):
                    status, result = status_only()
                assert status == 1 and Provider.messages == [failed_request]
                assert state.read_bytes() == pending_state, "read-only status reconciled pending mail state"

                Provider.mail_status = 200
                status, result = run()
                assert status == 1 and not result["pending"]["health"]
                assert result["notifications"][0]["delivery"] == "provider_accepted"
                assert Provider.messages[-1] == failed_request, "retry changed its provider identity or payload"
                status, result = run()
                assert status == 1 and len(Provider.messages) == 2, "ongoing incident sent duplicate mail"

                Provider.health = "healthy"
                status, result = run()
                assert status == 0 and result["notifications"][0]["kind"] == "recovery"
                assert Provider.messages[-1][0] != failed_request[0]
                status, result = run()
                assert status == 0 and len(Provider.messages) == 3, "steady recovery sent duplicate mail"

                Provider.health = "boolean_backup"
                assert MONITOR.probe(origin, "/statusz")["status"] == "unhealthy"
                Provider.health = "healthy"
                Provider.mail_status = 307
                status, result = run("--delivery-drill", "redirect-boundary")
                assert status == 1 and result["pending"]["drill"]
                assert result["notifications"][0]["errorCode"] == "response_redirect"
                assert len(Provider.messages) == 4
    finally:
        server.shutdown()
        server.server_close()
        thread.join()
    print("OK scry-monitor: read-only status, recovery freshness, unconfirmed acceptance, stable retry, incident/recovery deduplication, redirect refusal (isolated HTTP only)")


if __name__ == "__main__":
    main()
