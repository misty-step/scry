#!/usr/bin/env python3
"""Backup alert records failure without printing secrets."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "bin/scry-backup-alert"


def test_records_failure_without_secrets():
    with tempfile.TemporaryDirectory() as tmp:
        tmp = Path(tmp)
        backup = tmp / "backup.env"
        runtime = tmp / "scry.env"
        for path, body in (
            (backup, "SCRY_BACKUP_PASSPHRASE=super-secret\nSCRY_BACKUP_SPACES_SECRET=also-secret\n"),
            (runtime, "RESEND_API_KEY=resend-secret\n"),
        ):
            fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, "w") as stream:
                stream.write(body)
        state = tmp / "state"
        env = os.environ.copy()
        env.update(
            SCRY_BACKUP_STATE=str(state),
            SCRY_BACKUP_ENV_FILE=str(backup),
            SCRY_ENV_FILE=str(runtime),
            SCRY_ENV_OWNER_SELF="1",
        )
        result = subprocess.run([sys.executable, str(SCRIPT)], env=env, capture_output=True, text=True)
        if result.returncode != 0:
            raise AssertionError(result.stderr)
        if not (state / "FAILURE").is_file():
            raise AssertionError("FAILURE flag was not written")
        combined = result.stdout + result.stderr
        for secret in ("super-secret", "also-secret", "resend-secret"):
            if secret in combined:
                raise AssertionError("alert printed a secret")


def main():
    test_records_failure_without_secrets()
    print("OK scry-backup-alert")


if __name__ == "__main__":
    main()
