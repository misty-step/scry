#!/bin/sh
# Scry container entrypoint: restore the latest private snapshot on every
# instance start, run the application, and attempt one final remote-verified
# backup when the platform signals shutdown. Container disk is ephemeral by
# contract; the R2 sink is the durable store.
#
# Environment (injected by the Container Durable Object at start):
#   SCRY_OWNER_ID, SCRY_SECRET        application identity + signing secret
#   SCRY_BASE_URL                     canonical origin
#   SCRY_BACKUP_REMOTE_URL            private gateway object prefix
#   SCRY_BACKUP_REMOTE_TOKEN          narrow append/read gateway token
#   SCRY_CONTAINER_RESTORE_KEY        latest snapshot key (resolved by the DO)
#   SCRY_REDIRECT_HOSTS               comma-separated alias hosts (optional)
set -eu

state_dir=${SCRY_STATE_DIR:-/var/lib/scry}
DB=${SCRY_DB:-$state_dir/data/scry.sqlite}
CONF=$state_dir/nginx.conf
restore_dir=$state_dir/restore
SCRY_BIN=${SCRY_BIN:-/usr/local/bin/scry}
NGINX_BIN=${SCRY_NGINX_BIN:-nginx}
NGINX_TEMPLATE=${SCRY_NGINX_TEMPLATE:-/etc/nginx/nginx.conf}
startup_timeout=${SCRY_STARTUP_TIMEOUT_SECONDS:-60}

say() { printf '[entrypoint] %s\n' "$*" >&2; }

validate_boot_contract() {
  case "$startup_timeout" in
    ''|*[!0-9]*) say "SCRY_STARTUP_TIMEOUT_SECONDS must be an integer"; return 78 ;;
  esac
  if [ "$startup_timeout" -lt 1 ] || [ "$startup_timeout" -gt 300 ]; then
    say "SCRY_STARTUP_TIMEOUT_SECONDS must be between 1 and 300"
    return 78
  fi
  case "${SCRY_BOOT_MODE:-}" in
    restore-required)
      missing=""
      [ -n "${SCRY_CONTAINER_RESTORE_KEY:-}" ] || missing="$missing SCRY_CONTAINER_RESTORE_KEY"
      [ -n "${SCRY_BACKUP_REMOTE_URL:-}" ] || missing="$missing SCRY_BACKUP_REMOTE_URL"
      [ -n "${SCRY_BACKUP_REMOTE_TOKEN:-}" ] || missing="$missing SCRY_BACKUP_REMOTE_TOKEN"
      if [ -n "$missing" ]; then
        say "restore-required boot needs$missing"
        return 78
      fi
      if ! printf '%s\n' "$SCRY_CONTAINER_RESTORE_KEY" | grep -Eq '^scry-[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[a-f0-9]{32}\.scry-backup\.zip$'; then
        say "SCRY_CONTAINER_RESTORE_KEY is not a completed snapshot key"
        return 78
      fi
      case "$SCRY_BACKUP_REMOTE_URL" in
        https://*) ;;
        *) say "SCRY_BACKUP_REMOTE_URL must be an HTTPS object-prefix endpoint"; return 78 ;;
      esac
      ;;
    synthetic-fresh)
      if [ "${SCRY_DATA_CLASS:-}" != "synthetic" ]; then
        say "synthetic-fresh boot is allowed only for explicitly synthetic data"
        return 78
      fi
      ;;
    *)
      say "SCRY_BOOT_MODE must be restore-required or synthetic-fresh"
      return 78
      ;;
  esac
}

restore_latest() {
  key="${SCRY_CONTAINER_RESTORE_KEY:-}"
  if [ -s "$DB" ]; then
    say "database already present on this disk; checking before serve"
    if ! "$SCRY_BIN" check --db "$DB"; then
      say "existing database failed compatibility or integrity check; refusing to serve"
      return 78
    fi
    return 0
  fi
  if [ "${SCRY_BOOT_MODE:-}" = "synthetic-fresh" ]; then
    say "explicit synthetic-only fresh boot"
    return 0
  fi
  mkdir -p "$restore_dir" "$(dirname "$DB")"
  rm -f "$restore_dir/snapshot.zip"
  say "fetching snapshot $key"
  code=$(printf 'Authorization: Bearer %s\n' "$SCRY_BACKUP_REMOTE_TOKEN" | \
    curl -sS -o "$restore_dir/snapshot.zip" -w '%{http_code}' \
      -H @- "${SCRY_BACKUP_REMOTE_URL}/${key}") || {
      say "snapshot fetch failed (network); refusing to serve"
      rm -f "$restore_dir/snapshot.zip"
      return 78
    }
  if [ "$code" != "200" ]; then
    say "snapshot fetch returned HTTP $code; refusing to serve"
    rm -f "$restore_dir/snapshot.zip"
    return 78
  fi
  if "$SCRY_BIN" restore --snapshot "$restore_dir/snapshot.zip" --destination "$DB"; then
    say "restored $key"
    rm -f "$restore_dir/snapshot.zip"
  else
    say "restore rejected the snapshot; refusing to serve"
    rm -f "$DB" "$DB-wal" "$DB-shm" "$restore_dir/snapshot.zip"
    return 78
  fi
}

final_backup() {
  # A graceful platform stop gives this process an opportunity to publish one
  # final verified snapshot. This is not a durability guarantee: crashes,
  # SIGKILL, host loss, stalled egress, or a failed upload can lose every write
  # newer than the most recent remote-verified snapshot.
  if [ -z "$SCRY_BACKUP_REMOTE_URL" ] || [ ! -s "$DB" ]; then
    say "no remote sink or database; skipping final backup"
    return 0
  fi
  say "final remote-verified backup"
  if "$SCRY_BIN" backup --db "$DB" --require-remote; then
    say "final backup uploaded and verified"
  else
    say "final backup failed; durability remains at the last remote-verified snapshot and newer acknowledged writes may be lost"
  fi
}

nginx_config() {
  # Substitute the owner identity into the template; it is never accepted
  # from a client. nginx runs as the current (unprivileged) user.
  owner=$(printf '%s' "${SCRY_OWNER_ID:-}" | sed 's/[&/\]/\\&/g')
  sed "s/__SCRY_OWNER_ID__/$owner/" "$NGINX_TEMPLATE" > "$CONF"
}

wait_for_scry() {
  elapsed=0
  while [ "$elapsed" -lt "$startup_timeout" ]; do
    if ! kill -0 "$SCRY_PID" 2>/dev/null; then
      wait "$SCRY_PID" 2>/dev/null || true
      say "application exited before listening; refusing readiness"
      return 70
    fi
    if awk '$2 ~ /:1F91$/ && $4 == "0A" { found=1 } END { exit !found }' /proc/net/tcp /proc/net/tcp6 2>/dev/null; then
      say "application listener ready"
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  say "application did not listen before startup deadline; refusing readiness"
  kill -TERM "$SCRY_PID" 2>/dev/null || true
  wait "$SCRY_PID" 2>/dev/null || true
  return 70
}

validate_boot_contract
restore_latest
nginx_config

mkdir -p "$state_dir/nginx_tmp/body" "$state_dir/nginx_tmp/proxy"

"$SCRY_BIN" serve --db "$DB" --addr 127.0.0.1:8081 &
SCRY_PID=$!
say "scry process started on loopback; waiting for listener"
wait_for_scry

# -e redirects the early-startup error log (opened before the config is
# read) so the unprivileged default path under /var/lib/nginx is never used.
"$NGINX_BIN" -e /dev/stderr -c "$CONF"
NGINX_PID=$(cat "$state_dir/nginx.pid")

shutdown() {
  say "shutdown signal"
  kill -TERM "$NGINX_PID" 2>/dev/null || nginx -s stop 2>/dev/null || true
  kill -TERM "$SCRY_PID" 2>/dev/null || true
  wait "$SCRY_PID" 2>/dev/null || true
  final_backup
  exit 0
}
trap shutdown TERM INT

wait "$SCRY_PID"
say "application exited; attempting final backup"
final_backup
