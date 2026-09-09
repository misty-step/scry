#!/usr/bin/env bash
# Explicit local traffic cutover. Requires a staged immutable release and the
# private environment. A failed candidate is rolled back only after the old
# binary independently accepts the current database schema/integrity.
set -Eeuo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$script_dir/common.sh"

usage() {
  cat <<'USAGE'
Usage: sudo bash deploy/activate.sh --release ID [--allow-local-backup]

Stop/drain Scry, take a completed off-VM/readback-verified backup of an existing
DB using the previous binary, check candidate schema compatibility read-only,
switch the immutable release link, start and check the actual process/readiness.
The initial empty installation has no database to back up.

A failed candidate is stopped. The previous binary is restarted only if its
read-only check accepts the current DB; incompatible-schema rollback fails
closed and leaves the service stopped. Never copy a backup over the live DB.
Use deploy/restore.sh in an isolated unused location for an explicit restore.

--allow-local-backup is an explicit synthetic-preview exception when there is
no configured off-VM sink. It does NOT establish S10 recovery or production
readiness. Ordinary releases require verified private remote backup.

After the first successful activation, boot startup is enabled. No release or
remote recovery artifact is deleted. Private exe ingress/owner identity and
remote retention must already have been proved by the operator.
USAGE
}

release_id= local_only=0
while (($#)); do
  case $1 in
    --release) (($# >= 2)) || fail '--release needs an ID'; release_id=$2; shift 2 ;;
    --allow-local-backup) local_only=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done
[[ -n $release_id ]] || { usage >&2; exit 2; }
require_root
require_commands systemctl systemd-run flock stat sha256sum readlink curl sync
validate_release_id "$release_id"
require_private_env
lock_deployment
candidate=$release_root/$release_id
verify_release "$candidate"
previous=
if [[ -e $app_root/current || -L $app_root/current ]]; then
  [[ -L $app_root/current ]] || fail 'current must be a managed release symlink'
  previous=$(readlink -f -- "$app_root/current") || fail 'current release target is unavailable'
  [[ $(dirname -- "$previous") == "$release_root" ]] || fail 'current points outside managed immutable releases'
  validate_release_id "$(basename -- "$previous")"
  verify_release "$previous"
fi
if [[ $previous == "$candidate" ]]; then
  systemctl start "$unit"
  wait_ready "$candidate" || fail 'current release did not become ready; inspect journalctl -u scry.service'
  systemctl enable "$unit"
  printf 'Release %s is already current and ready.\n' "$release_id"
  exit 0
fi

was_active=0
if systemctl is-active --quiet "$unit"; then was_active=1; fi
stopped=0 switched=0
recover_failure() {
  local status=$?
  trap - EXIT
  ((status != 0 && stopped)) || exit "$status"
  set +e
  systemctl stop "$unit"
  if [[ -n $previous && -f $live_db ]] && scry_command "$previous/scry" check --db "$live_db"; then
    if ((switched)) && ! set_current "$previous"; then
      printf 'Could not select the verified prior release; service remains stopped.\n' >&2
      exit "$status"
    fi
    if ((was_active)); then
      if systemctl start "$unit" && wait_ready "$previous"; then
        printf 'Candidate failed; compatible prior release is running again.\n' >&2
      else
        systemctl stop "$unit"
        printf 'Prior release failed readiness; service remains stopped.\n' >&2
      fi
    else
      printf 'Candidate failed; compatible prior release selected, service remains stopped as before.\n' >&2
    fi
  else
    printf 'No verified compatible rollback is available. Service remains stopped; restore only into an unused path and reconcile jobs explicitly.\n' >&2
  fi
  exit "$status"
}
trap recover_failure EXIT
# Mark before stop so a bounded-shutdown failure cannot leave a partial cutover.
stopped=1
systemctl stop "$unit"
if [[ -e $live_db || -L $live_db ]]; then
  backup_release=${previous:-$candidate}
  scry_command "$backup_release/scry" check --db "$live_db"
  if ((local_only)); then
    printf 'Synthetic-preview exception: requiring a complete LOCAL backup only.\n' >&2
    scry_command "$backup_release/scry" backup --db "$live_db"
  else
    scry_command "$backup_release/scry" backup --db "$live_db" --require-remote
  fi
  scry_command "$candidate/scry" check --db "$live_db"
else
  [[ ! -e $live_db-wal && ! -L $live_db-wal && ! -e $live_db-shm && ! -L $live_db-shm && ! -e $live_db-journal && ! -L $live_db-journal ]] || fail 'live database is absent but SQLite sidecars exist; do not initialize over uncertain recovery state'
fi
switched=1
set_current "$candidate"
systemctl start "$unit"
wait_ready "$candidate" || fail 'candidate did not become ready within the bounded readiness window'
systemctl enable "$unit"
if [[ -n $previous ]]; then
  previous_link=$app_root/.previous-$$
  [[ ! -e $previous_link && ! -L $previous_link ]] || fail 'temporary previous-release link already exists'
  ln -s -- "$previous" "$previous_link"
  mv -Tf -- "$previous_link" "$app_root/previous"
  sync -f "$app_root"
fi
trap - EXIT
printf 'Activated release %s; loopback readiness passed and boot startup is enabled.\n' "$release_id"
