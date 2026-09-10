#!/usr/bin/env bash
# Internal helpers for the entry-point scripts. Requires Bash, systemd, GNU
# coreutils and util-linux; never source the private EnvironmentFile as shell.
# All runtime invocations use systemd's EnvironmentFile parser and User=scry.
set -Eeuo pipefail

readonly app_root=/opt/scry
readonly release_root=/opt/scry/releases
readonly data_root=/var/lib/scry
readonly live_db=/var/lib/scry/scry.sqlite
readonly env_file=/etc/scry/scry.env
readonly unit=scry.service

fail() { printf 'scry deploy: %s\n' "$*" >&2; exit 1; }

require_root() {
  [[ $EUID -eq 0 ]] || fail 'run this local installation operation as root (the application itself runs as scry)'
}

require_commands() {
  local command
  for command in "$@"; do
    command -v "$command" >/dev/null || fail "required local command is missing: $command"
  done
}

lock_deployment() {
  [[ -d /run/lock ]] || fail '/run/lock is required'
  exec 9>/run/lock/scry-deploy.lock
  flock -n 9 || fail 'another Scry install, activate, or restore is running'
}

validate_release_id() {
  [[ $1 =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$ ]] || fail 'release ID must be 1-80 letters/digits/dots/dashes/underscores, starting with a letter or digit'
}

require_private_env() {
  [[ -f $env_file && ! -L $env_file ]] || fail "install a real private environment file at $env_file first"
  [[ $(stat -c '%u:%g:%a' -- "$env_file") == 0:0:600 ]] || fail "$env_file must be root:root mode 0600"
}

verify_release() {
  local release=$1
  [[ -d $release && ! -L $release && -f $release/scry && ! -L $release/scry ]] || fail 'release is missing or is not an immutable directory/binary'
  [[ $(stat -c '%u:%g:%a' -- "$release") == 0:0:555 ]] || fail 'release directory must be root:root mode 0555'
  [[ $(stat -c '%u:%g:%a' -- "$release/scry") == 0:0:555 ]] || fail 'release binary must be root:root mode 0555'
  [[ -f $release/SHA256SUMS && ! -L $release/SHA256SUMS ]] || fail 'release checksum metadata is missing'
  (cd -- "$release" && sha256sum --check --strict --status SHA256SUMS) || fail 'immutable release checksum does not match'
}

# A transient service avoids shell interpolation of credentials and never
# invokes an app binary as root. No secrets are copied into argv or printed.
scry_command() {
  local binary=$1
  shift
  systemd-run --quiet --wait --pipe --collect --service-type=exec \
    --property=User=scry --property=Group=scry --property=UMask=0077 \
    --property="EnvironmentFile=$env_file" --property="WorkingDirectory=$data_root" \
    --property=NoNewPrivileges=yes --property=ProtectSystem=strict \
    --property=ProtectHome=yes --property=PrivateTmp=yes \
    --property="ReadWritePaths=$data_root" --property=RuntimeMaxSec=5min \
    --property=TimeoutStopSec=35s "$binary" "$@"
}

set_current() {
  local target=$1 temporary=$app_root/.current-$$-$RANDOM
  [[ ! -e $temporary && ! -L $temporary ]] || fail 'temporary activation link already exists'
  ln -s -- "$target" "$temporary"
  mv -Tf -- "$temporary" "$app_root/current"
  sync -f "$app_root"
}

wait_ready() {
  local expected=$1 attempt pid
  for ((attempt=0; attempt<30; attempt++)); do
    if systemctl is-active --quiet "$unit"; then
      pid=$(systemctl show --property=MainPID --value "$unit")
      if [[ $pid =~ ^[1-9][0-9]*$ && $(readlink -- "/proc/$pid/exe" 2>/dev/null) == "$expected/scry" ]] &&
          curl --noproxy '*' --fail --silent --max-time 2 http://127.0.0.1:8080/readyz >/dev/null; then
        return 0
      fi
    fi
    sleep 1
  done
  return 1
}
