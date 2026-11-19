# Shared by scry-deploy, scry-rollback and scry-status. Layout:
#   /srv/scry/builds/scry.<sha>  every installed build, never deleted here
#   /srv/scry/current            symlink to the running build
#   /srv/scry/good.log           shas that passed the health gate, oldest first
#   /srv/scry/bad.log            shas rolled back away from
ROOT=${SCRY_ROOT:-/srv/scry}
PORT=$(sed -n 's/^PORT=//p' /etc/scry/scry.env 2>/dev/null); PORT=${PORT:-8080}
current_sha() { basename "$(readlink "$ROOT/current" 2>/dev/null)" | sed 's/^scry\.//'; }
# Newest good sha never rolled back away from.
last_good() { rollback_target ""; }
# Newest good sha that is not the given sha and was never rolled back away from.
rollback_target() {
  tac "$ROOT/good.log" 2>/dev/null | while read -r s; do
    [ "$s" = "$1" ] && continue
    grep -qx "$s" "$ROOT/bad.log" 2>/dev/null && continue
    echo "$s"; break
  done
}
switch_to() { ln -sfn "$ROOT/builds/scry.$1" "$ROOT/current.new" && mv -T "$ROOT/current.new" "$ROOT/current" && systemctl restart scry; }
healthy() {
  for _ in $(seq 1 30); do
    curl -fsS -m 2 "http://127.0.0.1:$PORT/healthz" 2>/dev/null | grep -q '"status":"ok"' && return 0
    sleep 1
  done
  return 1
}
log() { printf '%s %s\n' "$(date -u +%FT%TZ)" "$*" | tee -a "$ROOT/deploy.log"; }
