#!/usr/bin/env python3
"""Isolated restore refuses missing dumps and existing state."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "bin/scry-restore"


def run(args, env):
    return subprocess.run([sys.executable, str(SCRIPT), *args], env=env, capture_output=True, text=True)


def test_missing_dump_and_state():
    with tempfile.TemporaryDirectory() as tmp:
        tmp = Path(tmp)
        env = os.environ.copy()
        missing = run(["--input", str(tmp / "no.dump"), "--state", str(tmp / "pg")], env=env)
        if missing.returncode == 0 or "missing" not in missing.stderr:
            raise AssertionError(missing.stderr)
        state = tmp / "pg"
        state.mkdir()
        dump = tmp / "scry.dump"
        dump.write_bytes(b"DUMP")
        exists = run(["--input", str(dump), "--state", str(state)], env=env)
        if exists.returncode == 0 or "already exists" not in exists.stderr:
            raise AssertionError(exists.stderr)
        both = run(["--input", str(dump), "--offhost", "--state", str(tmp / "other")], env=env)
        if both.returncode == 0 or "not both" not in both.stderr:
            raise AssertionError(both.stderr)


def test_offhost_without_env_fails():
    with tempfile.TemporaryDirectory() as tmp:
        env = os.environ.copy()
        env["SCRY_BACKUP_ENV_FILE"] = str(Path(tmp) / "missing.env")
        result = run(["--offhost", "--state", str(Path(tmp) / "pg")], env=env)
        if result.returncode == 0:
            raise AssertionError("off-host restore succeeded without credentials")


def main():
    test_missing_dump_and_state()
    test_offhost_without_env_fails()
    print("OK scry-restore")


if __name__ == "__main__":
    main()
