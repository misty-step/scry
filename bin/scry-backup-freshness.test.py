#!/usr/bin/env python3
"""Freshness check fails closed for missing or stale recovery."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "bin/scry-backup-freshness"


def run(env):
    return subprocess.run([sys.executable, str(SCRIPT)], env=env, capture_output=True, text=True)


def test_missing_env_fails():
    with tempfile.TemporaryDirectory() as tmp:
        env = os.environ.copy()
        env.update(
            SCRY_BACKUP_ENV_FILE=str(Path(tmp) / "missing.env"),
            SCRY_BACKUP_DIR=str(Path(tmp) / "dumps"),
            SCRY_BACKUP_STATE=str(Path(tmp) / "state"),
        )
        result = run(env)
        if result.returncode == 0 or "not provisioned" not in result.stderr:
            raise AssertionError(result.stderr)


def test_stale_and_success():
    with tempfile.TemporaryDirectory() as tmp:
        tmp = Path(tmp)
        env_file = tmp / "backup.env"
        fd = os.open(env_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        os.close(fd)
        dumps = tmp / "dumps"
        state = tmp / "state"
        dumps.mkdir()
        state.mkdir()
        dump = dumps / "scry-old.dump"
        dump.write_bytes(b"DUMP")
        stale_time = time.time() - 40 * 3600
        os.utime(dump, (stale_time, stale_time))
        (state / "last-run").write_text("2026-01-01T00:00:00Z success\n")
        env = os.environ.copy()
        env.update(
            SCRY_BACKUP_ENV_FILE=str(env_file),
            SCRY_BACKUP_DIR=str(dumps),
            SCRY_BACKUP_STATE=str(state),
            SCRY_BACKUP_MAX_AGE=str(26 * 3600),
        )
        stale = run(env)
        if stale.returncode == 0 or "stale" not in stale.stderr:
            raise AssertionError(stale.stderr)
        dump.write_bytes(b"DUMP")
        os.utime(dump, None)
        ok = run(env)
        if ok.returncode != 0:
            raise AssertionError(ok.stderr)
        (state / "FAILURE").write_text("failed\n")
        flagged = run(env)
        if flagged.returncode == 0 or "FAILURE" not in flagged.stderr:
            raise AssertionError(flagged.stderr)


def main():
    test_missing_env_fails()
    test_stale_and_success()
    print("OK scry-backup-freshness")


if __name__ == "__main__":
    main()
