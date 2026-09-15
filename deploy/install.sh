#!/usr/bin/env bash
# Stage only; never starts/stops the application or changes the active release.
# Run locally on the intended VM after copying a reviewed Linux Go binary.
set -Eeuo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$script_dir/common.sh"

usage() {
  cat <<'USAGE'
Usage: sudo bash deploy/install.sh --binary PATH --release ID [--env PATH]

Install a reviewed Go binary into /opt/scry/releases/ID, root-owned/read-only.
Create the unprivileged scry account and private /var/lib/scry data directory.
Install the systemd unit but DO NOT enable/start or replace the active release.
Use deploy/activate.sh separately after verifying the private exe ingress.

--env PATH is required for the first install. Supply real production values
following deploy/scry.env.example. The file is copied root:root mode 0600 to
/etc/scry/scry.env and is never interpreted as shell or printed. On later
installs it must match the existing file; credentials/config updates are an
explicit separate operator action, not a side effect of a binary release.

Release IDs are immutable: reusing an ID for different binary bytes is refused.
Keep reviewed release binaries and private configuration off-VM for recovery.
No SSH, provisioning, traffic change, build, or paid model call is performed.
USAGE
}

binary= release_id= private_env=
while (($#)); do
  case $1 in
    --binary) (($# >= 2)) || fail '--binary needs a path'; binary=$2; shift 2 ;;
    --release) (($# >= 2)) || fail '--release needs an ID'; release_id=$2; shift 2 ;;
    --env) (($# >= 2)) || fail '--env needs a path'; private_env=$2; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done
[[ -n $binary && -n $release_id ]] || { usage >&2; exit 2; }
require_root
require_commands install stat sha256sum cmp mktemp flock systemctl systemd-run getent useradd groupadd nologin sync
validate_release_id "$release_id"
[[ -f $binary && ! -L $binary ]] || fail 'binary must be a regular local file, not a symlink'
[[ -z $private_env || ( -f $private_env && ! -L $private_env ) ]] || fail 'private environment must be a regular file, not a symlink'
lock_deployment

if ! getent group scry >/dev/null; then
  groupadd --system scry
fi
if ! getent passwd scry >/dev/null; then
  useradd --system --gid scry --home-dir "$data_root" --shell "$(command -v nologin)" scry
fi
[[ $(id -u scry) != 0 && $(id -g scry) == "$(getent group scry | cut -d: -f3)" ]] || fail 'scry must be a non-root account with primary group scry'
for directory in "$app_root" "$release_root" /etc/scry "$data_root" "$data_root/backups"; do
  [[ ! -L $directory ]] || fail "refusing symlinked installation directory: $directory"
done
install -d -o root -g root -m 0755 "$app_root" "$release_root"
install -d -o root -g root -m 0700 /etc/scry
install -d -o scry -g scry -m 0700 "$data_root" "$data_root/backups"
if [[ -e $env_file || -L $env_file ]]; then
  require_private_env
  if [[ -n $private_env ]]; then
    cmp --silent -- "$private_env" "$env_file" || fail 'environment differs; review and install its change explicitly before activation'
  fi
else
  [[ -n $private_env ]] || fail '--env is required for the first install'
  install -o root -g root -m 0600 -- "$private_env" "$env_file"
fi

release=$release_root/$release_id
if [[ -e $release || -L $release ]]; then
  verify_release "$release"
  cmp --silent -- "$binary" "$release/scry" || fail 'release ID already exists with different bytes; use a new ID'
else
  staging=$(mktemp -d "$release_root/.install.XXXXXXXX")
  trap '[[ -z ${staging:-} ]] || rm -rf -- "$staging"' EXIT
  install -o root -g root -m 0555 -- "$binary" "$staging/scry"
  (cd -- "$staging" && sha256sum scry >SHA256SUMS)
  chmod 0444 "$staging/SHA256SUMS"
  chmod 0555 "$staging"
  mv -T -- "$staging" "$release"
  staging=
  sync -f "$release_root"
fi
install -o root -g root -m 0644 -- "$script_dir/scry.service" /etc/systemd/system/scry.service
systemctl daemon-reload
printf 'Staged immutable release %s; service is not activated.\n' "$release_id"
printf 'Next: sudo bash %s/activate.sh --release %s\n' "$script_dir" "$release_id"
