#!/usr/bin/env python3
"""Static install-layout contract for the off-host backup pipeline."""

from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
INSTALLER = (ROOT / "bin/install-scry-backup.sh").read_text()
UPLOADER = (ROOT / "bin/scry-backup-offhost").read_text()

assert "bin/retention-preflight.py" in INSTALLER
assert (
    'install -m 0755 "$stage/bin/retention-preflight.py" '
    "/usr/local/lib/scry/retention-preflight.py"
) in INSTALLER
assert 'PREFLIGHT="$SCRIPT_DIR/../lib/scry/retention-preflight.py"' in UPLOADER
for rel in (
    "bin/scry-restore",
    "bin/scry-backup-freshness",
    "bin/scry-backup-alert",
    "scripts/lib/scry_ops.py",
):
    assert (ROOT / rel).is_file(), rel
    assert rel in INSTALLER, rel
print("OK (installed preflight path matches uploader path)")
