#!/usr/bin/env python3
"""Rollback regression with synthetic fixtures, NEVER real deployment proof.

The release implementation, Git history, checksums and guards are real. Worker
bytes, workerd/operation receipts, GitHub CI/protection and Cloudflare/HTTP state
are explicitly synthetic. Required account/origin names identify fixtures only;
Git uses a temporary file-only origin and every other external call is trapped.
"""
from contextlib import contextmanager, ExitStack, redirect_stdout
import hashlib
import io
import json
import os
from pathlib import Path
import runpy
import shutil
import subprocess
import sys
import tempfile
from types import SimpleNamespace
from unittest.mock import patch
import urllib.error
import urllib.request
import urllib.response


FIXTURE = "SYNTHETIC rollback regression fixture; NOT deployment/workerd/CI proof"
OUTGOING_VERSION = "00000000-0000-4000-8000-000000000001"
TARGET_VERSION = "00000000-0000-4000-8000-000000000002"
DEPLOYMENT = "00000000-0000-4000-8000-000000000003"
OTHER_HASH = hashlib.sha256(b"synthetic mismatching fixture identity").hexdigest()
REAL_RUN = subprocess.run
REAL_POPEN = subprocess.Popen


def encoded(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n").encode()


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def write_json(path, value):
    Path(path).write_bytes(encoded(value))


class RollbackFixture:
    def __init__(self, root):
        self.root = root
        self.repo = root / "checkout"
        self.repo.mkdir()
        home = root / "home"
        home.mkdir()
        self.environment = {
            "PATH": os.defpath, "HOME": str(home), "TMPDIR": str(root),
            "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull,
            "GIT_ALLOW_PROTOCOL": "file", "GIT_TERMINAL_PROMPT": "0",
            "GIT_AUTHOR_NAME": "Synthetic Scry fixture",
            "GIT_AUTHOR_EMAIL": "fixture@example.test",
            "GIT_COMMITTER_NAME": "Synthetic Scry fixture",
            "GIT_COMMITTER_EMAIL": "fixture@example.test",
            "GIT_AUTHOR_DATE": "2000-01-01T00:00:00Z",
            "GIT_COMMITTER_DATE": "2000-01-01T00:00:00Z",
        }
        scripts = Path(__file__).resolve().parent
        tracked = ["scripts/scry-cloudflare", "scripts/lib/scry_ops.py"]
        for name in tracked:
            destination = self.repo / name
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(scripts.parent / name, destination)
        (self.repo / tracked[0]).chmod(0o755)
        # Load from the fixture checkout so even default cwd arguments point at
        # its real Git history. No release helper or guard is replaced.
        with patch.object(sys, "path", sys.path.copy()):
            self.release = SimpleNamespace(**runpy.run_path(str(self.repo / tracked[0])))
        release = self.release
        migration = f"{release.CRATE}/migrations/001-fixture.sql"
        (self.repo / migration).parent.mkdir(parents=True)
        (self.repo / migration).write_text(f"-- {FIXTURE}\nCREATE TABLE fixture (id TEXT PRIMARY KEY);\n")
        config = {
            "fixture": FIXTURE, "account_id": release.ACCOUNT,
            "main": "worker/index.js", "base_dir": "worker", "no_bundle": True,
            "migrations": [{"tag": "v1", "new_sqlite_classes": ["Scry"]}],
            "env": {},
        }
        for environment, name, bucket in (
            ("staging", "scry-staging", "scry-staging-recovery"),
            ("production", "scry", "scry-recovery"),
        ):
            config["env"][environment] = {
                "name": name,
                "durable_objects": {"bindings": [{"name": "SCRY", "class_name": "Scry"}]},
                "r2_buckets": [{"binding": "RECOVERY", "bucket_name": bucket}],
                "vars": {"MEMORY_ENGINE_MAIL_MODE": "resend",
                         "MEMORY_ENGINE_PUBLIC_BASE_URL": f"https://{name}.misty-step.workers.dev"},
            }
        write_json(self.repo / "wrangler.jsonc", config)
        tracked += [migration, "wrangler.jsonc"]

        def git(*args, cwd=self.repo):
            return REAL_RUN(["git", *map(str, args)], cwd=cwd, env=self.environment,
                            capture_output=True, text=True, check=True).stdout.strip()

        origin = root / "fixture-origin.git"
        git("init", "--quiet", "--bare", "--initial-branch=master", origin, cwd=root)
        git("init", "--quiet", "--initial-branch=master")
        git("add", "--", *tracked)
        git("commit", "--quiet", "-m", FIXTURE)
        git("remote", "add", "origin", origin)
        git("push", "--quiet", "origin", "HEAD:refs/heads/master")
        self.revision = git("rev-parse", "HEAD")
        inventory = {name: {"sha256": digest(self.repo / name),
                            "mode": "100755" if (self.repo / name).stat().st_mode & 0o100 else "100644"}
                     for name in tracked}
        source_hash = hashlib.sha256(encoded(inventory)).hexdigest()
        for target in config["env"].values():
            target["vars"].update(SCRY_REVISION=self.revision, SCRY_SOURCE_SHA256=source_hash)
        self.artifact = root / "synthetic-bundle"
        (self.artifact / "worker").mkdir(parents=True)
        (self.artifact / "worker/index.js").write_text(f"// {FIXTURE}\n")
        (self.artifact / "worker/index_bg.wasm").write_bytes(b"\0asm\x01\0\0\0")
        write_json(self.artifact / "source.json", inventory)
        write_json(self.artifact / "wrangler.json", config)
        ledger = [{"version": 1, "name": "001-fixture.sql", "sha256": digest(self.repo / migration)}]
        schema_hash = hashlib.sha256(encoded(ledger)).hexdigest()
        manifest = {
            "fixture": FIXTURE, "schema": release.SCHEMA, "target": "wasm32-unknown-unknown",
            "revision": self.revision, "source_sha256": source_hash,
            "tools": {"worker_build": release.WORKER, "wrangler": release.WRANGLER,
                      "wasm_bindgen": release.BINDGEN},
            "schema_ledger": ledger, "schema_sha256": schema_hash,
            "files": {path.relative_to(self.artifact).as_posix(): digest(path)
                      for path in self.artifact.rglob("*") if path.is_file()},
        }
        write_json(self.artifact / "manifest.json", manifest)
        bundle_hash = digest(self.artifact / "manifest.json")
        # These synthetic proof inputs only exercise receipt validation. They
        # must never be presented as evidence that workerd or CI actually ran.
        self.proof = {"fixture": FIXTURE, "schema": release.PROOF, "status": "passed",
                      "bundle_sha256": bundle_hash, "wrangler": release.WRANGLER}
        receipt = {"fixture": FIXTURE, "schema": release.RECEIPT, "environment": "production",
                   "account_id": release.ACCOUNT, "action": "promote", "schema_sha256": schema_hash}
        outgoing_hash = hashlib.sha256(b"synthetic outgoing bundle").hexdigest()
        self.current = {**receipt, "status": "failed-requires-inspection",
                        "version_id": OUTGOING_VERSION, "bundle_sha256": outgoing_hash}
        self.target = {**receipt, "status": "verified", "version_id": TARGET_VERSION,
                       "bundle_sha256": bundle_hash}
        self.versions = {OUTGOING_VERSION: outgoing_hash, TARGET_VERSION: bundle_hash}
        self.active = OUTGOING_VERSION
        self.deployments = []
        self.fingerprint = {"fixture": FIXTURE, "schema": "memory_engine.cloudflare_fingerprint.v1",
                            "schemaVersions": [1], "storageSchemaVersion": 1,
                            "sha256": hashlib.sha256(b"synthetic durable state").hexdigest()}
        self.ready_status = 200
        self.ci = {"fixture": FIXTURE, "id": 910000001, "name": "ci",
                   "app": {"slug": "github-actions"}, "head_sha": self.revision,
                   "status": "completed", "conclusion": "success"}
        self.admin_token = "synthetic-admin-fixture-not-a-secret"
        admin_env = root / "synthetic-admin.env"
        admin_env.write_text(f"# {FIXTURE}\nMEMORY_ENGINE_ADMIN_TOKEN={self.admin_token}\n")
        admin_env.chmod(0o600)
        package = self.repo / "node_modules/wrangler/package.json"
        package.parent.mkdir(parents=True)
        write_json(package, {"fixture": FIXTURE, "version": release.WRANGLER})
        self.receipt = root / "synthetic-rollback.json"
        self.args = SimpleNamespace(
            command="rollback", artifact=self.artifact, verification=root / "synthetic-workerd-proof.json",
            environment="production", receipt=self.receipt, admin_env=admin_env,
            current=root / "synthetic-current.failed.json", version_receipt=root / "synthetic-target.json",
            staging_receipt=None, secrets_file=None,
        )

    def git_process(self, argv, **kwargs):
        command = tuple(map(str, argv))
        allowed = {
            ("git", "fetch", "origin", "master"),
            ("git", "merge-base", "--is-ancestor", self.revision, "origin/master"),
            ("git", "archive", "--format=tar", self.revision),
        }
        if command not in allowed or Path(kwargs["cwd"]).resolve() != self.repo or kwargs.get("shell"):
            raise AssertionError(f"unexpected fixture process: {command}")
        # clean_environment intentionally strips Git-specific variables. Keep
        # real Git isolated from machine credentials/config and network remotes.
        kwargs["env"] = self.environment
        return REAL_POPEN(command, **kwargs)

    def external_run(self, argv, **kwargs):
        command = tuple(map(str, argv))
        if command[0] == "git":
            return REAL_RUN(command, **kwargs)  # Popen permits only local review reads/fetch.
        repository = self.release.REPOSITORY
        if command == ("gh", "api", f"repos/{repository}/branches/master/protection"):
            value = {"fixture": FIXTURE, "enforce_admins": {"enabled": True},
                     "required_status_checks": {"contexts": ["ci"]}}
        elif command == ("gh", "api", "--paginate", "--slurp",
                         f"repos/{repository}/commits/{self.revision}/check-runs?per_page=100"):
            value = [{"fixture": FIXTURE, "check_runs": [self.ci]}]
        elif command[:2] == ("node", str(self.repo / "node_modules/wrangler/bin/wrangler.js")):
            if command[-4] != "--config" or command[-2:] != ("--env", "production"):
                raise AssertionError(f"unexpected fixture Wrangler scope: {command}")
            value = self.cloudflare(command[2:-4], kwargs.get("env", {}))
        else:
            raise AssertionError(f"unexpected fixture subprocess: {command}")
        return subprocess.CompletedProcess(command, 0, json.dumps(value), "")

    def cloudflare(self, command, environment):
        if command == ("deployments", "status", "--json"):
            return {"fixture": FIXTURE, "versions": [{"version_id": self.active, "percentage": 100}]}
        if len(command) == 4 and command[:2] == ("versions", "view") and command[3] == "--json":
            version = command[2]
            if version not in self.versions:
                raise AssertionError(f"unknown synthetic Worker version: {version}")
            return {"fixture": FIXTURE, "id": version,
                    "annotations": {"workers/message": f"scry bundle sha256:{self.versions[version]}"}}
        if len(command) == 6 and command[:2] == ("versions", "deploy") and command[3:5] == ("--yes", "--message"):
            version, separator, percentage = command[2].partition("@")
            if not separator or percentage != "100" or version not in self.versions:
                raise AssertionError(f"unexpected synthetic deployment: {command}")
            self.active = version
            record = {"fixture": FIXTURE, "type": "version-deploy", "version": 1,
                      "worker_name": "scry", "deployment_id": DEPLOYMENT}
            self.deployments.append({"version_id": version, "deployment_id": DEPLOYMENT})
            with Path(environment["WRANGLER_OUTPUT_FILE_PATH"]).open("ab") as stream:
                stream.write(encoded(record))
            return record
        raise AssertionError(f"unexpected fixture Wrangler invocation: {command}")

    def http(self, request, *, timeout):
        base = "https://scry.misty-step.workers.dev"
        if not isinstance(request, urllib.request.Request) or not request.full_url.startswith(base + "/"):
            raise AssertionError("unexpected fixture HTTP origin")
        route = request.full_url[len(base):]
        method = request.get_method()
        if route.startswith("/internal/") and request.get_header("X-admin-token") != self.admin_token:
            raise AssertionError("fixture admin request lacks its synthetic token")
        key = (method, route)
        responses = {
            ("GET", "/internal/migration/fingerprint"): (200, self.fingerprint),
            ("GET", "/internal/runtime"): (200, {"fixture": FIXTURE, "maintenance": False}),
            ("GET", "/healthz"): (200, {"fixture": FIXTURE, "status": "ok"}),
            ("GET", "/readyz"): (self.ready_status, {"fixture": FIXTURE, "status": "ready"}),
            ("GET", "/manifest.webmanifest"): (200, {"fixture": FIXTURE}),
            ("GET", "/favicon.png"): (200, {"fixture": FIXTURE}),
            ("GET", "/static/app.js"): (200, {"fixture": FIXTURE}),
            ("GET", "/"): (200, {"fixture": FIXTURE}),
            ("GET", "/v1/openapi.json"): (200, {"fixture": FIXTURE}),
            ("POST", "/v1/accounts/anonymous/sources/anonymous/generation-jobs"): (401, {"fixture": FIXTURE}),
        }
        if key not in responses:
            raise AssertionError(f"unexpected fixture HTTP request: {key}")
        # An unhealthy outgoing Worker cannot supply ANY application evidence.
        # The real operation must recover using its explicit receipt/control plane.
        if self.active == OUTGOING_VERSION:
            raise urllib.error.URLError("synthetic outgoing application is unavailable")
        status, body = responses[key]
        headers = {"content-type": "application/json"}
        stream = io.BytesIO(encoded(body))
        if status != 200:
            raise urllib.error.HTTPError(request.full_url, status, FIXTURE, headers, stream)
        return urllib.response.addinfourl(stream, headers, request.full_url, status)

    def rollback(self):
        write_json(self.args.verification, self.proof)
        write_json(self.args.current, self.current)
        write_json(self.args.version_receipt, self.target)
        # Do not emit production-looking success output for synthetic receipts.
        with redirect_stdout(io.StringIO()):
            self.release.deploy_operation(self.args)


@contextmanager
def rollback_fixture():
    with tempfile.TemporaryDirectory(prefix="scry-cloudflare-regression-fixture-") as temporary:
        fixture = RollbackFixture(Path(temporary))
        with ExitStack() as boundaries:
            boundaries.enter_context(patch.dict(os.environ, fixture.environment, clear=True))
            boundaries.enter_context(patch.object(subprocess, "run", side_effect=fixture.external_run))
            boundaries.enter_context(patch.object(subprocess, "Popen", side_effect=fixture.git_process))
            boundaries.enter_context(patch.object(urllib.request.OpenerDirector, "open", side_effect=fixture.http))
            yield fixture


def expect_refusal(fixture, *, switched=False):
    try:
        fixture.rollback()
    except fixture.release.OperationError:
        pass
    else:
        raise AssertionError("unsafe rollback was accepted")
    assert not fixture.receipt.exists(), "failed operation published a successful receipt"
    failed_path = Path(str(fixture.receipt) + ".failed.json")
    if switched:
        assert fixture.active == TARGET_VERSION, "post-switch failure lost the deployed target"
        assert fixture.deployments == [{"version_id": TARGET_VERSION, "deployment_id": DEPLOYMENT}]
        failed = json.loads(failed_path.read_text())
        assert failed["status"] == "failed-requires-inspection", "failed proof was marked verified"
        assert failed["version_id"] == TARGET_VERSION and failed["deployment_id"] == DEPLOYMENT
        assert failed["previous_version_id"] == OUTGOING_VERSION, "failure receipt lost recovery identity"
    else:
        assert fixture.active == OUTGOING_VERSION and not fixture.deployments, "refusal switched traffic"
        if failed_path.exists():
            assert json.loads(failed_path.read_text())["status"] == "failed-requires-inspection"


def test_failed_current_recovers_without_outgoing_http():
    with rollback_fixture() as fixture:
        fixture.rollback()
        receipt = json.loads(fixture.receipt.read_text())
        assert fixture.active == TARGET_VERSION
        assert fixture.deployments == [{"version_id": TARGET_VERSION, "deployment_id": DEPLOYMENT}]
        assert receipt["status"] == "verified" and receipt["version_id"] == TARGET_VERSION
        assert receipt["previous_version_id"] == OUTGOING_VERSION
        assert receipt["outgoing_status"] == "failed-requires-inspection"
        assert receipt["schema_proof"] == {
            "storage_schema_version": 1, "schema_versions": [1],
            "fingerprint_sha256": fixture.fingerprint["sha256"],
        }, "rollback omitted authoritative post-switch schema verification"
        assert receipt["runtime_proof"] == {
            "maintenance": False, "readiness": "ready", "public_smoke": "passed",
        }, "rollback omitted post-switch runtime/public verification"
        assert not Path(str(fixture.receipt) + ".failed.json").exists()


def test_unsafe_receipts_never_switch():
    # Each case crosses a different trust boundary, not a command-order contract.
    corruptions = (
        lambda f: f.current.update(schema_sha256=OTHER_HASH),
        lambda f: f.target.update(schema_sha256=OTHER_HASH),
        lambda f: f.target.update(status="uploaded", action="upload"),
        lambda f: f.target.update(bundle_sha256=OTHER_HASH),
        lambda f: f.target.update(account_id="synthetic-wrong-account"),
        lambda f: f.current.update(version_id=TARGET_VERSION),
        lambda f: f.versions.update({TARGET_VERSION: OTHER_HASH}),
    )
    for corrupt in corruptions:
        with rollback_fixture() as fixture:
            corrupt(fixture)
            expect_refusal(fixture)


def test_real_artifact_source_and_proof_guards_remain_required():
    corruptions = (
        lambda f: (f.artifact / "worker/index.js").write_text("// corrupted synthetic artifact\n"),
        lambda f: (f.repo / "scripts/scry-cloudflare").write_text("# unreviewed synthetic release tooling\n"),
        lambda f: f.proof.update(bundle_sha256=OTHER_HASH),
        lambda f: f.ci.update(conclusion="failure"),
    )
    for corrupt in corruptions:
        with rollback_fixture() as fixture:
            corrupt(fixture)
            expect_refusal(fixture)


def test_post_switch_schema_failure_is_not_verified():
    with rollback_fixture() as fixture:
        fixture.fingerprint.update(schemaVersions=[1, 2], storageSchemaVersion=2)
        expect_refusal(fixture, switched=True)


def test_post_switch_runtime_failure_is_not_verified():
    with rollback_fixture() as fixture:
        fixture.ready_status = 503
        expect_refusal(fixture, switched=True)


def main():
    test_failed_current_recovers_without_outgoing_http()
    test_unsafe_receipts_never_switch()
    test_real_artifact_source_and_proof_guards_remain_required()
    test_post_switch_schema_failure_is_not_verified()
    test_post_switch_runtime_failure_is_not_verified()
    print("OK scry-cloudflare rollback (synthetic fixtures only; NOT deployment proof)")


if __name__ == "__main__":
    main()
