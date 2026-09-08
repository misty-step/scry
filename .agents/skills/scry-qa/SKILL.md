---
name: scry-qa
description: >
  Route changed-surface Scry QA to the canonical lane, cutover, and
  evaluation procedures. Use for QA, verification, smoke tests, or
  checking the app.
argument-hint: "[api|kernel|ui|generation|gate|prod-smoke]"
---

# Scry QA

Choose the surface that changed, then follow the existing procedure for that
surface. This skill does not own live topology, gate command lists, or a
second experiment recipe.

| Need | Authority |
|---|---|
| Executable QA lanes, Worker/runtime proof, and reportable surfaces | [`docs/qa/system.md`](../../../docs/qa/system.md) |
| Source-writer barrier, recovery, traffic, activation, and cutover proof | [`docs/runbook.md`](../../../docs/runbook.md) |
| Authorized model comparison, receipts, and spending limits | [`docs/evals.md`](../../../docs/evals.md) |
| Fast and ship-parity gate names | current `package.json` scripts |

Live production status is whatever the runbook's observed evidence currently
records. Do not treat a hostname, process, or database named in this skill as
current topology.

Do not source, print, or commit credentials. Use authorized environment or
secret-store channels only. Do not call live models or spend without explicit
current authorization. A green fixture or build proves only the machinery it
exercises.

## Report

Return `PASS`, `FAIL`, or `UNVERIFIED`; exact commands from the canonical
procedure; surfaces exercised; artifacts inspected; uncovered surfaces; and
any cutover or spending authority that was not granted.
