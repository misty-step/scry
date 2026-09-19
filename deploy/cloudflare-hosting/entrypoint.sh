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

DB=/var/lib/scry/data/scry.sqlite
CONF=/var/lib/scry/nginx.conf
restore_dir=/var/lib/scry/restore

say() { printf '[entrypoint] %s\n' "$*" >&2; }

restore_latest() {
  key="${SCRY_CONTAINER_RESTORE_KEY:-}"
  if [ -s "$DB" ]; then
    say "database already present on this disk; skipping restore"
    return 0
  fi
  if [ -z "$key" ] || [ -z "$SCRY_BACKUP_REMOTE_URL" ] || [ -z "$SCRY_BACKUP_REMOTE_TOKEN" ]; then
    say "no restore configuration; starting with a fresh empty database"
    return 0
  fi
  mkdir -p "$restore_dir"
  rm -f "$restore_dir/snapshot.zip"
  say "fetching snapshot $key"
  code=$(curl -sS -o "$restore_dir/snapshot.zip" -w '%{http_code}' \
    -H "Authorization: Bearer ${SCRY_BACKUP_REMOTE_TOKEN}" \
    "${SCRY_BACKUP_REMOTE_URL}/${key}") || {
      say "snapshot fetch failed (network); starting empty"
      return 0
    }
  if [ "$code" != "200" ]; then
    say "snapshot fetch returned HTTP $code; starting empty"
    rm -f "$restore_dir/snapshot.zip"
    return 0
  fi
  if /usr/local/bin/scry restore --snapshot "$restore_dir/snapshot.zip" --destination "$DB"; then
    say "restored $key"
    rm -f "$restore_dir/snapshot.zip"
  else
    say "restore rejected the snapshot; starting empty"
    rm -f "$DB" "$DB-wal" "$DB-shm" "$restore_dir/snapshot.zip"
  fi
}

final_backup() {
  # Platform shutdown: push a final verified snapshot so the sink holds
  # every acknowledged write from this instance's lifetime. Failure is
  # logged, never fatal to the shutdown path.
  if [ -z "$SCRY_BACKUP_REMOTE_URL" ] || [ ! -s "$DB" ]; then
    say "no remote sink or database; skipping final backup"
    return 0
  fi
  say "final remote-verified backup"
  if /usr/local/bin/scry backup --db "$DB" --require-remote; then
    say "final backup uploaded and verified"
  else
    say "final backup failed; the last interval snapshot bounds the loss"
  fi
}

nginx_config() {
  # Substitute the owner identity into the template; it is never accepted
  # from a client. nginx runs as the current (unprivileged) user.
  owner=$(printf '%s' "${SCRY_OWNER_ID:-}" | sed 's/[&/\]/\\&/g')
  sed "s/__SCRY_OWNER_ID__/$owner/" /etc/nginx/nginx.conf > "$CONF"
}

restore_latest
nginx_config

mkdir -p /var/lib/scry/nginx_tmp/body /var/lib/scry/nginx_tmp/proxy

/usr/local/bin/scry serve --db "$DB" --addr 127.0.0.1:8081 &
SCRY_PID=$!
say "scry serving on loopback"

# -e redirects the early-startup error log (opened before the config is
# read) so the unprivileged default path under /var/lib/nginx is never used.
nginx -e /dev/stderr -c "$CONF"
NGINX_PID=$(cat /var/lib/scry/nginx.pid)

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
