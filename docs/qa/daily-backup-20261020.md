# Daily backup trigger verification, 2026-10-20

Scope: [#299](https://github.com/misty-step/scry/issues/299), under
[#297](https://github.com/misty-step/scry/issues/297) and
[#273](https://github.com/misty-step/scry/issues/273). SPEC D5/S10.2 require
verified off-machine backups; S09 keeps the machine capability separate from
learner access. The existing [feature map](../../features/README.md) led to
these recovery/privacy contracts. No browser behavior changed or was certified.

Base: `39665ef7e297fa0fe0d6f646d11cf56f7bf3d618`, fetched from `origin/master`.
Branch: `fix/daily-backup-timer`. The PR records the final committed head.

## Offline proof

The test was written and run before the implementation. It exited 1 with
`FileNotFoundError` for `scripts/scry-daily-backup.py`, which did not yet exist.
After implementation:

```text
$ python3 qa/daily-backup.test.py -v
Ran 13 tests
OK
$ python3 qa/promote.test.py -v
Ran 7 tests
OK
$ cargo test --locked --all-targets
102 passed across 15 test binaries; 0 failed
$ systemd-analyze --user verify deploy/mirrodin/scry-daily-backup.service deploy/mirrodin/scry-daily-backup.timer
exit 0
$ systemd-analyze calendar --iterations=2 '*-*-* 09:30:00 UTC'
Normalized form: *-*-* 09:30:00 UTC
Next UTC occurrences: 2026-10-21 09:30:00, 2026-10-22 09:30:00
```

Tests stub `pass` and HTTP transport. Redirect cases exercise the actual urllib
opener and refuse 301/302/303/307/308 without a second transport request.
They check HTTP/body failures, credentials with trailing newlines and invalid
values, sanitized failures, and origin derivation from a temporary TOML file.
The daily backup suite is wired into `scripts/scry-ci` beside promotion tests.

## Authorized production observation

One manual invocation from the worktree used only the existing local pass
entries for the machine token and backup trigger. It did not read learner
routes, install units, deploy, or change Cloudflare configuration.

```text
$ python3 scripts/scry-daily-backup.py
Scry production snapshot verified.
backup_exit_status=0

2026-10-20T14:15:18.081963+00:00
GET https://scry.study/healthz: HTTP 200
User-Agent: ScryDailyBackup/1.0
health_exit_status=0
```

Health used the same machine token without the backup key. An earlier health
request with urllib's default user agent returned HTTP 403; the successful
request above used the backup script's user agent. No response body, credential,
or raw upstream diagnostic was retained.

SHA-256 of the script exercised in production:
`f3614b978331003179a2a32f7ee4a93684b8f91cc3cf787e9708d7039dd74951`.

## Remaining observations

Timer installation, a systemd journal success line, natural daily execution,
and independent R2 object listing/readback are UNVERIFIED here. The endpoint's
verified acknowledgment is not an independent R2 observation or restore drill.
The old Hermes job was not contacted or disabled. The conductor installs from
master after merge, verifies the journal/health/new R2 object, and disables the
old job or records its reachability limit, following the
[installation procedure](../rust-cloudflare.md#install-or-update-after-merge).
No physical-phone, provider usefulness, release artifact, or recovery acceptance
is claimed by this operations change.
