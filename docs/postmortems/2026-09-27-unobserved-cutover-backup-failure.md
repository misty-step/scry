# Postmortem: unobserved backup execution failure

- **Incident date:** 2026-09-19; detected 2026-09-27 (UTC).
- **Status:** obsolete attempt closed by operator-approved no-code-fix exception;
  class-closing work remains open in [MIS-175][follow-up].
- **Operational owner:** Scry operations.
- **Tracker:** [MIS-173][incident]; [investigation and readback receipts][evidence].

## Summary

A one-off `scry-cutover-backup.service` on the retained `scry-app` VM failed to
change directory before executing its backup command. It remained failed for
eight days without reaching the incident route. The investigation also exposed
a current gap: production has moved to Cloudflare, but its backup failures have
no external failure alerting. Verified backups do not close that detection gap.

## Impact

The failed attempt produced no archive. A replacement backup completed 21 seconds
later. Independent readback verified that archive, the final VM cutover archive,
and current production backups; no missing retained Scry database content was
found. No production data loss was observed. Writes after a verified recovery
point and full service-restoration time were not established by these checks.

## Timeline

| Time (UTC) | Observation |
| --- | --- |
| September 19, 00:25:34 | One-off unit exited with systemd `200/CHDIR`, before exec. |
| September 19, 00:25:55 | Replacement archive completed; its remote-success receipt and R2 bytes were subsequently verified. |
| September 22, 21:02:34 | Final VM cutover archive uploaded; the old application remains disabled and inactive. |
| September 27, 16:28:37 | Alert intake received the eight-day-old unit failure. |
| September 27, 16:33–16:36 | Independent readback and retained-database comparison established the bounded recovery evidence below. |
| September 27 | Kaylee, deciding for Phaedrus, approved obsolete/no-code-fix closure, then required this merged postmortem and a tracked prevention follow-up. |

## Evidence and mechanism

The transient unit specified `User=scry` and `WorkingDirectory=/home/exedev`.
That directory was owned by `exedev:exedev`, mode `0750`; a safe `cd` check as
`scry` failed. The original backup command was not rerun. `Restart=no` and
`CollectMode=inactive` let the failed one-shot persist without retrying.
Systemd timestamps establish the old failure; its journal entries were gone.
The reported installation of host-alerts explains the late discovery only as
routing context, not independently verified installation history.

Fifteen R2 archives passed byte-size/SHA-256 comparison, ZIP integrity, embedded
database checksum and SQLite integrity checks. Twelve archives covered the ten
midnight/noon slots on September 23–27, including extra same-window runs; all
twelve had successful application readback receipts. The newest verified archive
was from September 27 at 15:17:34, schema 5, within the 24-hour recovery objective
when checked. Exact keys and hashes are in the [incident evidence][evidence].

All 24 non-ledger tables in the retained VM database matched the final remote
cutover snapshot. Its 37 backup ledger rows differed from the snapshot's 36 only
by that snapshot's own subsequent success receipt; all prior rows matched.
The VM database was preserved unchanged.

The [recovery runbook](../runbook.md#recovery-policy-and-observation) records
Cloudflare scheduled-backup failures and deferred idle stops in Workers Logs,
but no external alert. A retired VM's unit monitor is not production backup
coverage. The original closure also lacked a merged postmortem link; Kaylee
reported that the alert-route closure guard then failed every five minutes.

## Pokayoke

**Not implemented; this failure class remains open.** [MIS-175][follow-up] owns
the structural change: make the supported backup operation own its execution
context and completion result, and make failed or missing verified backups
externally actionable rather than silently healthy. Any supported native
recovery path must reuse the existing fixed-directory
[`scry_command`](../../deploy/common.sh) helper, not inherit an operator's home.
This does not authorize restarting the obsolete unit or changing VM permissions.

For current production, route launch/upload/readback failures to the existing
agent-triage intake and independently detect missing expected readback receipts
and observer failure. Distinguish a running writer from an intentionally stopped
instance. Require regression proof for inaccessible caller directories, failed
readback and missing receipts, plus a real isolated failure-to-alert-to-recovery
exercise. Alerts cannot prevent every provider failure; the enforced contract is
that backup failure or absent success cannot silently count as healthy.
Clearing the stale unit, a reminder, or this document alone is not that fix.

## Follow-up

Scry operations owns [MIS-175][follow-up], the single prevention follow-up. Link
its implementation PR and verification receipts here before claiming prevention
complete. Monitoring remains unchanged in this docs-only closeout. VM retirement
is separate and requires an inventory of retained binaries, private configuration
and recovery stores plus owner approval; no database, store or VM was removed.
The approved code-fix exception does not waive this postmortem or its link on
MIS-173.

[incident]: https://linear.app/misty-step/issue/MIS-173/sentry-workstation-scry-app-system-unit-scry-cutover-backupservice
[evidence]: https://linear.app/misty-step/issue/MIS-173/sentry-workstation-scry-app-system-unit-scry-cutover-backupservice#comment-10e9dc61
[follow-up]: https://linear.app/misty-step/issue/MIS-175/scry-prevent-silent-backup-failures-with-owned-execution-and
