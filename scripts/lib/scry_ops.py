"""Small standard-library boundaries shared by native Scry operations."""
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import socket
import stat
import subprocess
import tarfile
import time
from urllib.parse import quote
import urllib.error
import urllib.request


class OperationError(Exception):
    pass


RELEASE_SCHEMA = "scry.release.v1"
HASHED_FILES = (
    "memory-engine-api",
    "memory-engine-canary-receipt",
    "bin/send-magic-link",
    "bin/scry-receipt",
    "etc/release-schema.json",
    "etc/caddy/scry-ingress.caddy",
    "etc/caddy/scry-ingress.maintenance.caddy",
)
ARTIFACT_FILES = HASHED_FILES + ("manifest.json",)
EXECUTABLE_FILES = {
    "memory-engine-api",
    "memory-engine-canary-receipt",
    "bin/send-magic-link",
    "bin/scry-receipt",
}


def run(argv, **kwargs):
    """Do not echo argv: operational commands may carry private connection data."""
    try:
        return subprocess.run(argv, check=True, **kwargs)
    except subprocess.CalledProcessError as error:
        raise OperationError(f"{Path(argv[0]).name} failed (exit {error.returncode})") from None


def digest(path):
    with open(path, "rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def write_json(path, value, mode=0o644):
    path = Path(path)
    temporary = path.with_name(path.name + ".next")
    with open(temporary, "x", encoding="utf-8") as stream:
        os.chmod(temporary, mode)
        json.dump(value, stream, sort_keys=True, indent=2)
        stream.write("\n")
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temporary, path)


def load_json(path):
    return json.loads(Path(path).read_text())


def load_schema(path):
    schema = load_json(path)
    for key in ("postgres_major", "migration_version", "minimum_supported_version", "maximum_supported_version", "rollback_floor"):
        if key not in schema:
            raise OperationError(f"release schema missing {key}")
    if int(schema["postgres_major"]) != 16:
        raise OperationError("release schema is not Postgres 16")
    return schema


def private_env(path, owner=None):
    """Read literal KEY=value, never execute a shell or expand substitutions.

    Matches the simple assignment subset shared by systemd EnvironmentFile and
    repository env files. Multiline/escaped values are deliberately rejected.
    """
    path = Path(path)
    info = path.lstat()
    owner = os.geteuid() if owner is None else owner
    if not stat.S_ISREG(info.st_mode) or info.st_uid != owner or stat.S_IMODE(info.st_mode) != 0o600:
        raise OperationError("environment file must be a regular owner-owned mode-0600 file")
    values = {}
    for number, line in enumerate(path.read_text().splitlines(), 1):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        match = re.fullmatch(r"([A-Za-z_][A-Za-z0-9_]*)=(.*)", line)
        if not match:
            raise OperationError(f"unsupported environment assignment on line {number}")
        key, value = match.groups()
        if value.startswith(("'", '"')):
            quote_char = value[0]
            if len(value) < 2 or value[-1] != quote_char or quote_char in value[1:-1]:
                raise OperationError(f"unsupported environment quoting on line {number}")
            value = value[1:-1]
        elif any(char.isspace() for char in value) or "'" in value or '"' in value:
            raise OperationError(f"quote environment value on line {number}")
        if "\\" in value or "\x00" in value:
            raise OperationError(f"unsupported environment escape on line {number}")
        if key in values:
            raise OperationError(f"duplicate environment key on line {number}")
        values[key] = value
    return values


def apply_env(values):
    os.environ.update(values)


def write_env(path, values, mode=0o600):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    body = "".join(f"{key}={values[key]}\n" for key in sorted(values))
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    with os.fdopen(fd, "w") as stream:
        stream.write(body)
        stream.flush()
        os.fsync(stream.fileno())


def free_port():
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def wait_ready(base, seconds=45, alive=lambda: True):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if not alive():
            raise OperationError("API exited before readiness; inspect the private API log")
        try:
            with urllib.request.urlopen(base + "/readyz", timeout=min(2, max(0.1, deadline - time.monotonic()))) as response:
                if response.status == 200 and json.load(response).get("status") == "ready":
                    return
        except (OSError, ValueError, json.JSONDecodeError):
            pass
        time.sleep(0.25)
    raise OperationError(f"API readiness deadline exceeded ({seconds}s)")


def smoke(base, strict=True):
    for route, expected in (("/healthz", "ok"), ("/readyz", "ready")):
        with urllib.request.urlopen(base + route, timeout=10) as response:
            if response.status != 200 or json.load(response).get("status") != expected:
                raise OperationError(f"smoke failed at {route}")
    for route in ("/", "/manifest.webmanifest", "/favicon.png", "/apple-touch-icon.png"):
        with urllib.request.urlopen(base + route, timeout=10) as response:
            if response.status != 200:
                raise OperationError(f"smoke failed at {route}")
    if not strict:
        return
    request = urllib.request.Request(
        base + "/v1/accounts/smoke-anonymous/sources/smoke-source/generation-jobs",
        method="POST",
    )
    try:
        urllib.request.urlopen(request, timeout=10).close()
    except urllib.error.HTTPError as error:
        if error.code == 401:
            return
        raise OperationError(f"anonymous mutation failed with {error.code}, not 401") from None
    raise OperationError("anonymous mutation did not fail with 401")


def enforce_traversal(root):
    root = Path(root)
    root.chmod(0o755)
    for path in [root, *root.rglob("*")]:
        if path.is_dir():
            path.chmod(0o755)


def verify_release(root, expected=None):
    root = Path(root)
    if not root.is_dir():
        raise OperationError("release directory is missing")
    enforce_traversal(root)
    manifest_path = root / "manifest.json"
    if not manifest_path.is_file():
        raise OperationError("release manifest is missing")
    manifest = load_json(manifest_path)
    if manifest.get("schema") != RELEASE_SCHEMA:
        raise OperationError("unsupported release manifest schema")
    files = manifest.get("files") or {}
    if set(files) != set(HASHED_FILES):
        raise OperationError("release manifest file set does not match the artifact contract")
    for name in ARTIFACT_FILES:
        path = root / name
        if not path.is_file():
            raise OperationError(f"release is missing {name}")
        if name in HASHED_FILES:
            recorded = files[name]
            if digest(path) != recorded.get("sha256"):
                raise OperationError(f"checksum mismatch for {name}")
        if name in EXECUTABLE_FILES:
            path.chmod(0o755)
            if not os.access(path, os.X_OK):
                raise OperationError(f"{name} is not executable")
        else:
            path.chmod(0o644)
    if expected:
        for key, value in expected.items():
            if manifest.get(key) != value:
                raise OperationError(f"manifest {key} does not match the calling source")
    return manifest


def pack_release(stage, archive):
    stage = Path(stage)
    archive = Path(archive)
    verify_release(stage)
    archive.parent.mkdir(parents=True, exist_ok=True)
    added_dirs = set()
    with open(archive, "wb") as raw, gzip.GzipFile(fileobj=raw, mode="wb", mtime=0, compresslevel=9) as compressed:
        with tarfile.open(fileobj=compressed, mode="w", format=tarfile.GNU_FORMAT) as tar:
            for name in sorted(ARTIFACT_FILES):
                path = stage / name
                parts = Path(name).parts
                for depth in range(1, len(parts)):
                    directory = str(Path(*parts[:depth]))
                    if directory in added_dirs:
                        continue
                    info = tarfile.TarInfo(directory)
                    info.type = tarfile.DIRTYPE
                    info.mode = 0o755
                    info.mtime = 0
                    info.uid = 0
                    info.gid = 0
                    info.uname = "root"
                    info.gname = "root"
                    tar.addfile(info)
                    added_dirs.add(directory)
                info = tar.gettarinfo(path, arcname=name)
                info.mtime = 0
                info.uid = 0
                info.gid = 0
                info.uname = "root"
                info.gname = "root"
                info.mode = 0o755 if name in EXECUTABLE_FILES else 0o644
                with open(path, "rb") as payload:
                    tar.addfile(info, payload)
    return digest(archive)

def extract_archive(archive, dest):
    dest = Path(dest)
    if dest.exists():
        raise OperationError("refusing to overwrite an immutable release directory")
    dest.mkdir(mode=0o755, parents=True)
    try:
        with tarfile.open(archive, "r:gz") as tar:
            tar.extractall(dest, filter="data")
    except (tarfile.TarError, OSError) as error:
        shutil.rmtree(dest, ignore_errors=True)
        raise OperationError("archive extraction failed") from error
    enforce_traversal(dest)
    return verify_release(dest)


def archive_manifest(archive):
    with tarfile.open(archive, "r:gz") as tar:
        member = tar.extractfile("manifest.json")
        if member is None:
            raise OperationError("archive is missing manifest.json")
        return json.load(member)


def resolve_current(root):
    link = Path(root) / "current"
    if not link.exists():
        return None
    if not link.is_symlink():
        raise OperationError("current release pointer must be a symlink")
    return link.resolve()


def atomic_current(root, release_dir):
    root = Path(root)
    release_dir = Path(release_dir)
    relative = os.path.relpath(release_dir, root)
    next_link = root / "current.next"
    next_link.unlink(missing_ok=True)
    next_link.symlink_to(relative)
    os.replace(next_link, root / "current")


class IsolatedPostgres:
    """A unique PG16 cluster with socket-only access; never an existing database."""

    def __init__(self, state, bindir=None):
        self.state = Path(state)
        self.data = self.state / "pgdata"
        self.socket = self.state / "socket"
        self.port = free_port()
        if bindir is None:
            bindir = os.environ.get("SCRY_PG_BINDIR")
        if bindir is None:
            candidates = [Path("/usr/lib/postgresql/16/bin")]
            if shutil.which("pg_config"):
                candidates.append(Path(run(["pg_config", "--bindir"], capture_output=True, text=True).stdout.strip()))
            bindir = next((path for path in candidates if (path / "initdb").is_file()), None)
        if bindir is None:
            raise OperationError("Postgres 16 server tools required; set SCRY_PG_BINDIR to their bin directory")
        self.bindir = Path(bindir)
        for binary in ("initdb", "pg_ctl", "psql", "pg_restore", "pg_dump"):
            version = run([str(self.bindir / binary), "--version"], capture_output=True, text=True).stdout
            if not re.search(r"\b16(?:\.|\b)", version):
                raise OperationError(f"{binary} must be Postgres 16")
        if os.geteuid() == 0:
            raise OperationError("isolated PG16 must run as an unprivileged user, not root")
        self.started = False

    def command(self, binary, *arguments, **kwargs):
        environment = {key: value for key, value in os.environ.items() if not key.startswith("PG")}
        environment.update(
            PGHOST=str(self.socket),
            PGPORT=str(self.port),
            PGUSER="scry",
            PGDATABASE="scry",
            PGCONNECT_TIMEOUT="5",
        )
        return run([str(self.bindir / binary), *arguments], env=environment, **kwargs)

    def start(self):
        self.state.mkdir(parents=True, exist_ok=True)
        self.socket.mkdir(mode=0o700)
        self.command(
            "initdb",
            "-D",
            str(self.data),
            "-U",
            "scry",
            "--auth-local=trust",
            "--auth-host=reject",
            "--no-locale",
            "--encoding=UTF8",
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        with (self.data / "postgresql.conf").open("a") as config:
            config.write(
                f"\nlisten_addresses = ''\nport = {self.port}\nunix_socket_directories = '{self.socket}'\n"
            )
        self.command(
            "pg_ctl",
            "-D",
            str(self.data),
            "-l",
            str(self.state / "postgres.log"),
            "-w",
            "-t",
            "30",
            "start",
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        self.started = True
        return self

    @property
    def url(self):
        return f"postgresql://scry@/scry?host={quote(str(self.socket), safe='')}&port={self.port}&sslmode=disable"

    def stop(self):
        if self.started:
            self.command(
                "pg_ctl",
                "-D",
                str(self.data),
                "-m",
                "fast",
                "-w",
                "-t",
                "30",
                "stop",
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
            self.started = False
