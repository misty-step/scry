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

    def log_message(self, *_args):
        pass

    def do_GET(self):
        if self.path == "/statusz":
            value = {"schema": MONITOR.HEALTH_SCHEMA, "status": "healthy",
                     "maintenance": False, "backupAgeMs": 0}
            if self.health == "missing_backup":
                value.pop("backupAgeMs")
            elif self.health == "boolean_backup":
                value["backupAgeMs"] = True
        else:
            value = {"status": "ok" if self.path == "/healthz" else "ready"}
        self.respond(200, value)

    def do_POST(self):
        assert self.path == "/emails"
        body = self.rfile.read(int(self.headers["Content-Length"]))
        self.messages.append((self.headers["Idempotency-Key"], body))
        self.respond(self.mail_status, {"id": "11111111-1111-4111-8111-111111111111"})

    def respond(self, status, value):
        body = json.dumps(value).encode()
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
    print("OK scry-monitor: missing backup, unconfirmed acceptance, stable retry, incident/recovery deduplication, redirect refusal (isolated HTTP only)")


if __name__ == "__main__":
    main()
