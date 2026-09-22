import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { chmodSync, existsSync, mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";

const here = path.dirname(fileURLToPath(import.meta.url));
const entrypoint = path.join(here, "entrypoint.sh");

function fixture() {
  const root = mkdtempSync(path.join(tmpdir(), "scry-hosting-test-"));
  const bin = path.join(root, "bin");
  mkdirSync(bin);
  const scry = path.join(bin, "scry");
  writeFileSync(scry, "#!/bin/sh\nexit 0\n");
  chmodSync(scry, 0o755);
  return { root, bin, scry };
}

function command(dir, name, body) {
  const target = path.join(dir, name);
  writeFileSync(target, `#!/bin/sh\n${body}\n`);
  chmodSync(target, 0o755);
  return target;
}

function boot(extra = {}) {
  return spawnSync("sh", [entrypoint], {
    encoding: "utf8",
    env: {
      PATH: process.env.PATH,
      SCRY_BOOT_MODE: "restore-required",
      SCRY_DATA_CLASS: "recovered",
      ...extra,
    },
  });
}

test("nginx uses inherited stderr when the platform forbids reopening its device path", () => {
  const f = fixture();
  try {
    const state = path.join(f.root, "state");
    const template = path.join(f.root, "nginx.conf");
    const server = path.join(f.root, "listen.mjs");
    mkdirSync(state, { recursive: true });
    writeFileSync(template, "owner __SCRY_OWNER_ID__\n");
    writeFileSync(
      server,
      'import net from "node:net";\nconst listener = net.createServer();\nlistener.listen(8081, "127.0.0.1");\nsetTimeout(() => listener.close(), 2500);\n',
    );
    writeFileSync(
      f.scry,
      '#!/bin/sh\ncase "$1" in\n  serve) exec "$NODE_BIN" "$TCP_SERVER" ;;\n  *) exit 0 ;;\nesac\n',
    );
    chmodSync(f.scry, 0o755);
    command(
      f.bin,
      "nginx",
      'log_target=""\nwhile [ "$#" -gt 0 ]; do\n  case "$1" in\n    -e) log_target=$2; shift 2 ;;\n    *) shift ;;\n  esac\ndone\nif [ "$log_target" != stderr ]; then\n  printf \'nginx: open() "/dev/stderr" failed (6: No such device or address)\\n\' >&2\n  exit 1\nfi\nprintf "%s\\n" "$$" > "$SCRY_STATE_DIR/nginx.pid"\nsleep 4',
    );

    const result = boot({
      PATH: `${f.bin}:${process.env.PATH}`,
      SCRY_BOOT_MODE: "synthetic-fresh",
      SCRY_DATA_CLASS: "synthetic",
      SCRY_STATE_DIR: state,
      SCRY_BIN: f.scry,
      SCRY_NGINX_TEMPLATE: template,
      SCRY_STARTUP_TIMEOUT_SECONDS: "5",
      SCRY_BACKUP_REMOTE_URL: "",
      SCRY_OWNER_ID: "synthetic-owner",
      NODE_BIN: process.execPath,
      TCP_SERVER: server,
    });

    assert.equal(result.status, 70, result.stderr);
    assert.match(result.stderr, /application listener ready/);
    assert.match(result.stderr, /application exited; attempting final backup/);
    assert.doesNotMatch(result.stderr, /nginx exited before the application/);
    assert.doesNotMatch(result.stderr, /open\(\) "\/dev\/stderr" failed/);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test("cold boot keeps the entrypoint alive without waiting for a daemonized nginx pid file", () => {
  const f = fixture();
  try {
    const state = path.join(f.root, "state");
    const template = path.join(f.root, "nginx.conf");
    const server = path.join(f.root, "listen.mjs");
    const nginxArgs = path.join(f.root, "nginx.args");
    mkdirSync(state, { recursive: true });
    writeFileSync(template, "owner __SCRY_OWNER_ID__\n");
    writeFileSync(server, 'import net from "node:net";\nconst listener = net.createServer();\nlistener.listen(8081, "127.0.0.1");\nsetTimeout(() => listener.close(), 2500);\n');
    writeFileSync(f.scry, '#!/bin/sh\ncase "$1" in serve) exec "$NODE_BIN" "$TCP_SERVER" ;; *) exit 0 ;; esac\n');
    chmodSync(f.scry, 0o755);
    // Model nginx returning before its daemon writes nginx.pid. A foreground
    // nginx should instead be tracked by the shell's child PID directly.
    command(f.bin, "nginx", 'printf "%s\\n" "$@" > "$NGINX_ARGS"\ncase " $* " in *"daemon off;"*) sleep 4 ;; *) (sleep 1; printf "%s\\n" "$$" > "$SCRY_STATE_DIR/nginx.pid") & ;; esac');

    const result = boot({
      PATH: `${f.bin}:${process.env.PATH}`,
      SCRY_BOOT_MODE: "synthetic-fresh",
      SCRY_DATA_CLASS: "synthetic",
      SCRY_STATE_DIR: state,
      SCRY_BIN: f.scry,
      SCRY_NGINX_TEMPLATE: template,
      SCRY_STARTUP_TIMEOUT_SECONDS: "5",
      SCRY_BACKUP_REMOTE_URL: "",
      SCRY_OWNER_ID: "synthetic-owner",
      NODE_BIN: process.execPath,
      TCP_SERVER: server,
      NGINX_ARGS: nginxArgs,
    });
    assert.equal(result.status, 70, result.stderr);
    assert.match(readFileSync(nginxArgs, "utf8"), /daemon off;/);
    assert.match(result.stderr, /application exited; attempting final backup/);
    assert.doesNotMatch(result.stderr, /nginx exited before the application/);
    assert.doesNotMatch(result.stderr, /nginx\.pid.*No such file/);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test("an early foreground nginx exit stops the writer", () => {
  const f = fixture();
  try {
    const state = path.join(f.root, "state");
    const template = path.join(f.root, "nginx.conf");
    const server = path.join(f.root, "listen.mjs");
    mkdirSync(state, { recursive: true });
    writeFileSync(template, "owner __SCRY_OWNER_ID__\n");
    writeFileSync(server, 'import net from "node:net";\nconst listener = net.createServer();\nlistener.listen(8081, "127.0.0.1");\nsetTimeout(() => listener.close(), 6000);\n');
    writeFileSync(f.scry, '#!/bin/sh\ncase "$1" in serve) exec "$NODE_BIN" "$TCP_SERVER" ;; *) exit 0 ;; esac\n');
    chmodSync(f.scry, 0o755);
    command(f.bin, "nginx", 'exit 1');
    const started = Date.now();
    const result = boot({
      PATH: `${f.bin}:${process.env.PATH}`,
      SCRY_BOOT_MODE: "synthetic-fresh",
      SCRY_DATA_CLASS: "synthetic",
      SCRY_STATE_DIR: state,
      SCRY_BIN: f.scry,
      SCRY_NGINX_TEMPLATE: template,
      SCRY_STARTUP_TIMEOUT_SECONDS: "5",
      SCRY_BACKUP_REMOTE_URL: "",
      SCRY_OWNER_ID: "synthetic-owner",
      NODE_BIN: process.execPath,
      TCP_SERVER: server,
    });
    assert.notEqual(result.status, 0, result.stderr);
    assert.match(result.stderr, /nginx exited before the application; refusing to remain a writer without ingress/);
    assert.ok(Date.now() - started < 5000, "the dead ingress must not leave Scry serving");
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test("restore-required boot refuses missing recovery configuration before serving", () => {
  const result = boot();

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /restore-required boot needs SCRY_CONTAINER_RESTORE_KEY/);
  assert.doesNotMatch(result.stderr, /starting (with a )?fresh empty database/i);
});

test("restore-required boot refuses a malformed recovery object key", () => {
  const f = fixture();
  try {
    const invoked = path.join(f.root, "curl.invoked");
    command(f.bin, "curl", 'touch "$CURL_INVOKED"\nprintf 200');
    const result = boot({
      PATH: `${f.bin}:${process.env.PATH}`,
      CURL_INVOKED: invoked,
      SCRY_STATE_DIR: path.join(f.root, "state"),
      SCRY_BIN: f.scry,
      SCRY_CONTAINER_RESTORE_KEY: "../not-an-immutable-snapshot",
      SCRY_BACKUP_REMOTE_URL: "https://synthetic.invalid",
      SCRY_BACKUP_REMOTE_TOKEN: "x".repeat(64),
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /SCRY_CONTAINER_RESTORE_KEY is not a completed snapshot key/);
    assert.equal(existsSync(invoked), false);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test("existing live data must pass compatibility and integrity before serving", () => {
  const f = fixture();
  try {
    const state = path.join(f.root, "state");
    mkdirSync(path.join(state, "data"), { recursive: true });
    writeFileSync(path.join(state, "data", "scry.sqlite"), "not-a-compatible-database");
    writeFileSync(f.scry, '#!/bin/sh\n[ "$1" != check ]\n');
    chmodSync(f.scry, 0o755);
    const result = boot({
      PATH: `${f.bin}:${process.env.PATH}`,
      SCRY_STATE_DIR: state,
      SCRY_BIN: f.scry,
      SCRY_CONTAINER_RESTORE_KEY: "scry-20260922T140000.000000000Z-00000000000000000000000000000000.scry-backup.zip",
      SCRY_BACKUP_REMOTE_URL: "https://synthetic.invalid",
      SCRY_BACKUP_REMOTE_TOKEN: "x".repeat(64),
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /existing database failed compatibility or integrity check; refusing to serve/);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test("restore-required boot refuses a recovery network failure", () => {
  const f = fixture();
  try {
    command(f.bin, "curl", "exit 7");
    const result = boot({
      PATH: `${f.bin}:${process.env.PATH}`,
      SCRY_STATE_DIR: path.join(f.root, "state"),
      SCRY_BIN: f.scry,
      SCRY_CONTAINER_RESTORE_KEY: "scry-20260922T140000.000000000Z-00000000000000000000000000000000.scry-backup.zip",
      SCRY_BACKUP_REMOTE_URL: "https://synthetic.invalid",
      SCRY_BACKUP_REMOTE_TOKEN: "x".repeat(64),
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /snapshot fetch failed \(network\); refusing to serve/);
    assert.doesNotMatch(result.stderr, /starting (with a )?fresh empty database/i);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test("recovery bearer is never exposed in curl argv", () => {
  const f = fixture();
  try {
    const argv = path.join(f.root, "curl.argv");
    command(f.bin, "curl", 'printf "%s\\n" "$@" > "$CURL_ARG_LOG"\nexit 7');
    const token = "credential-must-not-enter-argv-" + "x".repeat(40);
    boot({
      PATH: `${f.bin}:${process.env.PATH}`,
      CURL_ARG_LOG: argv,
      SCRY_STATE_DIR: path.join(f.root, "state"),
      SCRY_BIN: f.scry,
      SCRY_CONTAINER_RESTORE_KEY: "scry-20260922T140000.000000000Z-00000000000000000000000000000000.scry-backup.zip",
      SCRY_BACKUP_REMOTE_URL: "https://synthetic.invalid",
      SCRY_BACKUP_REMOTE_TOKEN: token,
    });

    assert.doesNotMatch(readFileSync(argv, "utf8"), new RegExp(token));
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test("nginx readiness stays closed until the application listener exists", () => {
  const f = fixture();
  try {
    const log = path.join(f.root, "process.log");
    const template = path.join(f.root, "nginx.conf");
    writeFileSync(template, "owner __SCRY_OWNER_ID__\n");
    writeFileSync(
      f.scry,
      '#!/bin/sh\ncase "$1" in\n  restore) while [ "$#" -gt 0 ]; do [ "$1" = --destination ] && { touch "$2"; exit 0; }; shift; done ;;\n  check) exit 0 ;;\n  serve) printf "scry-serve\\n" >> "$PROCESS_LOG"; exec sleep 30 ;;\nesac\n',
    );
    chmodSync(f.scry, 0o755);
    command(
      f.bin,
      "curl",
      'out=""\nwhile [ "$#" -gt 0 ]; do\n  case "$1" in -o|--output) out=$2; shift 2 ;; *) shift ;; esac\ndone\nprintf archive > "$out"\nprintf 200',
    );
    command(f.bin, "nginx", 'printf "nginx-start\\n" >> "$PROCESS_LOG"\nexit 0');
    const result = boot({
      PATH: `${f.bin}:${process.env.PATH}`,
      PROCESS_LOG: log,
      SCRY_STATE_DIR: path.join(f.root, "state"),
      SCRY_BIN: f.scry,
      SCRY_NGINX_TEMPLATE: template,
      SCRY_STARTUP_TIMEOUT_SECONDS: "1",
      SCRY_CONTAINER_RESTORE_KEY: "scry-20260922T140000.000000000Z-00000000000000000000000000000000.scry-backup.zip",
      SCRY_BACKUP_REMOTE_URL: "https://synthetic.invalid",
      SCRY_BACKUP_REMOTE_TOKEN: "x".repeat(64),
      SCRY_OWNER_ID: "synthetic-owner",
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /application did not listen before startup deadline; refusing readiness/);
    assert.doesNotMatch(readFileSync(log, "utf8"), /nginx-start/);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test("restore-required boot refuses a non-200 recovery response", () => {
  const f = fixture();
  try {
    command(f.bin, "curl", "printf 503");
    const result = boot({
      PATH: `${f.bin}:${process.env.PATH}`,
      SCRY_STATE_DIR: path.join(f.root, "state"),
      SCRY_BIN: f.scry,
      SCRY_CONTAINER_RESTORE_KEY: "scry-20260922T140000.000000000Z-00000000000000000000000000000000.scry-backup.zip",
      SCRY_BACKUP_REMOTE_URL: "https://synthetic.invalid",
      SCRY_BACKUP_REMOTE_TOKEN: "x".repeat(64),
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /snapshot fetch returned HTTP 503; refusing to serve/);
    assert.doesNotMatch(result.stderr, /starting (with a )?fresh empty database/i);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});

test("restore-required boot refuses a corrupt or incompatible archive", () => {
  const f = fixture();
  try {
    command(
      f.bin,
      "curl",
      'out=""\nwhile [ "$#" -gt 0 ]; do\n  case "$1" in\n    -o|--output) out=$2; shift 2 ;;\n    *) shift ;;\n  esac\ndone\nprintf corrupt > "$out"\nprintf 200',
    );
    writeFileSync(f.scry, "#!/bin/sh\n[ \"$1\" != restore ]\n");
    chmodSync(f.scry, 0o755);
    const state = path.join(f.root, "state");
    const result = boot({
      PATH: `${f.bin}:${process.env.PATH}`,
      SCRY_STATE_DIR: state,
      SCRY_BIN: f.scry,
      SCRY_CONTAINER_RESTORE_KEY: "scry-20260922T140000.000000000Z-00000000000000000000000000000000.scry-backup.zip",
      SCRY_BACKUP_REMOTE_URL: "https://synthetic.invalid",
      SCRY_BACKUP_REMOTE_TOKEN: "x".repeat(64),
    });

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /restore rejected the snapshot; refusing to serve/);
    assert.doesNotMatch(result.stderr, /starting (with a )?fresh empty database/i);
  } finally {
    rmSync(f.root, { recursive: true, force: true });
  }
});
