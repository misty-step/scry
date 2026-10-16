#!/usr/bin/env python3
"""Build only the temporary inspector; derive schema from exact retained source."""
import hashlib
import io
import os
from pathlib import Path
import re
import runpy
import subprocess
import tarfile

root = Path(__file__).resolve().parent.parent
retained = runpy.run_path(str(root / "qa/retained-go-migration.py"))
metadata = os.environ.get("SCRY_SOURCE_META")
data = ((Path(metadata).parent / "retained-go.tar").read_bytes() if metadata
        else retained["archive"](root))
if hashlib.sha256(data).hexdigest() != retained["ARCHIVE_SHA256"]:
    raise RuntimeError("retained Go source checksum mismatch")
files = ("schema.go", "foundation_schema.go", "concept_schema.go", "semantic_schema.go", "schema_v5.go")
sql = []
with tarfile.open(fileobj=io.BytesIO(data)) as archive:
    for version, filename in enumerate(files, 1):
        member = archive.extractfile("internal/store/" + filename)
        if member is None:
            raise RuntimeError("retained schema file missing")
        source = member.read().decode()
        matches = re.findall(r"const schemaV" + str(version) + r" = `([^`]+)`", source)
        if len(matches) != 1:
            raise RuntimeError("retained schema definition is ambiguous")
        sql.append(matches[0])
target = root / "target"
target.mkdir(exist_ok=True)
(target / "revival-schema.sql").write_text("\n".join(sql))
package = subprocess.check_output(["node", "-p", "require.resolve('wrangler/package.json')"], cwd=root, text=True).strip()
wrangler = Path(package).parent / "bin/wrangler.js"
for tool in ("inspect", "convert"):
    subprocess.run(["node", wrangler, "deploy", "--config", f"revival/{tool}.wrangler.toml",
                    "--dry-run", "--outdir", str(target / f"revival-{tool}")], cwd=root, check=True)
