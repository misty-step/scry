#!/usr/bin/env bash
# Restore data only. Never activates a release, changes credentials, overwrites
# a live DB, or starts restored jobs. All uncertain jobs are held by the binary.
set -Eeuo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$script_dir/common.sh"

usage() {
  cat <<'USAGE'
Usage: sudo bash deploy/restore.sh --release ID --snapshot FILE --destination PATH

Use a complete downloaded .scry-backup.zip and a compatible staged binary.
Restore into an UNUSED path whose existing parent is inside /var/lib/scry.
The source is preserved. Integrity, checksum, and schema are verified and
nonterminal jobs paused before atomic no-overwrite publication. No service is
started and no external job is reconciled/retried by this script.

On a fresh isolated VM: install the compatible release/private environment,
recover the ZIP from the private off-VM sink using independently retained
credentials, then restore to /var/lib/scry/scry.sqlite while the service is
inactive. Inspect the result before explicitly running deploy/activate.sh.
For a rehearsal, use a distinct unused destination under /var/lib/scry; never
point the live service at the rehearsal DB or enable its integrations.

This wrapper accepts complete ZIPs only. For a closed standalone SQLite
snapshot, use the binary's restore command directly so source-sidecar checks
and any adjacent .manifest.json are preserved. Record elapsed restore time and
the last recovered acknowledged event against the operator's RPO/RTO policy.
USAGE
}

release_id= snapshot= destination=
while (($#)); do
  case $1 in
    --release) (($# >= 2)) || fail '--release needs an ID'; release_id=$2; shift 2 ;;
    --snapshot) (($# >= 2)) || fail '--snapshot needs a path'; snapshot=$2; shift 2 ;;
    --destination) (($# >= 2)) || fail '--destination needs a path'; destination=$2; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) fail "unknown argument: $1" ;;
  esac
done
[[ -n $release_id && -n $snapshot && -n $destination ]] || { usage >&2; exit 2; }
require_root
require_commands systemctl systemd-run flock stat sha256sum install realpath mktemp od
validate_release_id "$release_id"
require_private_env
lock_deployment
release=$release_root/$release_id
verify_release "$release"
[[ -f $snapshot && ! -L $snapshot ]] || fail 'snapshot must be a regular downloaded recovery archive'
signature=$(od -An -tx1 -N4 -- "$snapshot")
[[ ${signature//[[:space:]]/} == 504b0304 ]] || fail 'this wrapper requires a complete Scry ZIP archive; use the binary directly for standalone SQLite'
destination=$(realpath -m -- "$destination")
[[ $destination == "$data_root/"* && -d $(dirname -- "$destination") ]] || fail 'destination parent must already exist inside /var/lib/scry'
if [[ $destination == "$live_db" ]]; then
  state=$(systemctl show --property=ActiveState --value "$unit")
  [[ $state == inactive || $state == failed ]] || fail 'the live service must be explicitly stopped before restoring its unused path'
fi
for path in "$destination" "$destination-wal" "$destination-shm" "$destination-journal"; do
  [[ ! -e $path && ! -L $path ]] || fail 'destination or SQLite sidecar already exists; choose a fresh path'
done

staging=$(mktemp -d "$data_root/.restore-input.XXXXXXXX")
trap 'rm -rf -- "$staging"' EXIT
chown scry:scry "$staging"
install -o scry -g scry -m 0400 -- "$snapshot" "$staging/recovery.scry-backup.zip"
scry_command "$release/scry" restore --snapshot "$staging/recovery.scry-backup.zip" --destination "$destination"
printf 'Restored with nonterminal jobs paused: %s\n' "$destination"
printf 'No service was started. Inspect recovered events and reconcile uncertain paid work explicitly before any retry.\n'
