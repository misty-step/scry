#!/usr/bin/env python3
"""US-001.1 only: run the pinned Go migration against disposable SQLite fixtures."""
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile

REVISION = "c6ba395a63814c1f973d8cda5057d54fbb587e9b"
ARCHIVE_SHA256 = "5787d0a07c4d28730f259649b50e53a9a16e5d468437fb69203acf97ea91222b"
PATHS = ("go.mod", "go.sum", "internal/store", "internal/learning")
TEST = "TestSchemaV4ToV5MigrationUS001"


def archive(root):
    return subprocess.check_output(
        ["git", "-C", str(root), "archive", "--format=tar", REVISION, *PATHS]
    )


def main():
    root = Path(__file__).resolve().parent.parent
    output = Path(sys.argv[1]).resolve()
    output.mkdir(parents=True, exist_ok=True)
    metadata = os.environ.get("SCRY_SOURCE_META")
    data = (Path(metadata).parent / "retained-go.tar").read_bytes() if metadata else archive(root)
    if hashlib.sha256(data).hexdigest() != ARCHIVE_SHA256:
        raise RuntimeError("retained Go source checksum mismatch")
    go = shutil.which("go")
    if not go:
        raise RuntimeError("pinned Go 1.27.1 required for retained US-001.1 compatibility")
    # Neither host app configuration nor Go's user environment enters this test.
    with tempfile.TemporaryDirectory(prefix="scry-retained-go-") as temporary:
        work = Path(temporary)
        source = work / "source"
        source.mkdir()
        with tarfile.open(fileobj=io.BytesIO(data)) as tar:
            for member in tar.getmembers():
                path = Path(member.name)
                if (path.is_absolute() or ".." in path.parts or
                    not (member.isfile() or member.isdir()) or
                    not any(member.name == p or member.name.startswith(p + "/") or
                            member.isdir() and p.startswith(member.name.rstrip("/") + "/") for p in PATHS)):
                    raise RuntimeError("unexpected retained Go archive entry")
            # Extract only validated regular bytes; works on the gate's Python
            # without delegating symlink/device/permission handling to tarfile.
            for member in tar.getmembers():
                destination = source / member.name
                if member.isdir():
                    destination.mkdir(parents=True, exist_ok=True)
                else:
                    destination.parent.mkdir(parents=True, exist_ok=True)
                    with tar.extractfile(member) as incoming, destination.open("xb") as outgoing:
                        shutil.copyfileobj(incoming, outgoing)
        home = work / "home"
        home.mkdir()
        env = {"PATH": os.environ["PATH"], "HOME": str(home), "GOENV": "off",
               "GOTOOLCHAIN": "local", "GOPROXY": "https://proxy.golang.org",
               "GOSUMDB": "sum.golang.org", "LANG": "C.UTF-8"}
        version = subprocess.check_output([go, "version"], env=env, text=True).strip()
        if not version.startswith("go version go1.27.1 "):
            raise RuntimeError("retained migration requires Go 1.27.1")
        result = subprocess.run([go, "test", "-count=1", "-v", "-run", "^" + TEST + "$", "./internal/store"],
                                cwd=source, env=env, capture_output=True, text=True)
        log = result.stdout + result.stderr
        (output / "retained-go-migration.txt").write_text(log)
        if result.returncode or "--- PASS: " + TEST + " (" not in log:
            raise RuntimeError("retained migration failed; see retained-go-migration.txt")
        receipt = {"format": "scry-retained-go-compatibility-v1", "target": "retained-go-compatibility",
                   "story": "US-001", "criterion": 1, "status": "pass", "source_revision": REVISION,
                   "archive_sha256": ARCHIVE_SHA256, "toolchain": version, "test": TEST,
                   "limits": ["Disposable schema-4 SQLite fixtures; no historical or live database.",
                              "Preserves the retained Go contract; does not prove Rust import or migration."]}
        (output / "retained-go-migration.json").write_text(json.dumps(receipt, indent=2) + "\n")
        print("PASS US-001.1 retained Go migration at " + REVISION)


if __name__ == "__main__":
    main()
