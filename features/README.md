# Scry feature map

This is the maintained navigation map for verifying and assessing Scry as its
owner uses it. [VISION](../VISION.md) explains intent, [SPEC](../SPEC.md) alone
owns stories and acceptance, and [DESIGN](../DESIGN.md) owns the curiosity
notebook. Run it through [verify-scry](../.agents/skills/verify-scry/SKILL.md).
US-012 is retired and its identifier remains reserved.

## Baseline preconditions

- Production (https://scry.mistystep.io): read-only. Run `doctor.mjs prod`
  first. Only sign-in and refused or isolated-QA requests touch it.
- Local: built Worker (`worker-build --release --locked`) and CLI
  (`cargo build --locked --bin scry`); each helper starts its own disposable
  workerd SQLite/R2 space with no egress. Never import a live notebook.
- Pinned runtimes from README, `npm ci --ignore-scripts` for Playwright, and an unlocked `pass`.

## Driving conventions

- Helpers live in `.agents/skills/verify-scry/scripts/`. Each prints one
  PASS/FAIL/UNPROVEN line per check and writes `checks.jsonl` and screenshots to
  `target/verify-scry/<stamp>-<helper>/`.
- Prefer roles, labels and `data-state` handles over CSS position.
- Drive write flows only against run-owned local spaces. On production, every
  owner page GET may write (see [sign-in](sign-in.md#gotchas)).

## Proof and skip reporting

- Capture the action and the resulting state; verify the stored side effect
  through a second view (CLI `--json`, reload, list).
- Report an unreachable path as UNPROVEN with the exact missing access.
  Never relabel a redirect, a QA identity 200 or a synthetic pass as owner proof.

## Feature entry contract

Each file has a title and a user-visible paragraph, then `Sub-features`,
`How to get to it (user POV)`, `Driving it with verify-scry helpers` and
`Gotchas`, in that order.

## Features

- [Owner sign-in](sign-in.md): S09 owner admission, signed-out, expired, wrong identity, alternate host, Doctor.
- [Exact text practice](exact-study.md): US-016, US-017, US-018 phone Stream / AG1.
- [Private Library](library.md): US-015 find and keep / AG1.
- [One question and honest judgment](study.md): legacy concept notebook, US-002, US-003, US-007, US-008, US-011.
- [Concepts, references, and goals](concepts.md): US-001, US-006, US-009, US-010.
- [Owner tokens, API and CLI](owner-tokens.md): US-014 / AG1; hosted admission #335 UNPROVEN.
- [Create, preparation, and refinement](capture.md): US-004, US-013/S11.
- [Changes in this build](../SPEC.md#s12--inspect-the-changes-in-this-build):
  follow the footer's Changelog link to `/changelog`, then return through the
  header. `qa/browser-release.mjs` (inside `qa/walk --all`) covers phone and
  desktop in light and dark. Source: `src/web.rs`, `src/runtime.rs`.
- [Private ingress and origin changes](../docs/runbook.md#canonical-origin-change--prepared-2026-10-20):
  S09, alternate-host reads and writes, and the S10 backup handoff. Drive
  `qa/runtime-helper.mjs` with a synthetic identity and the candidate origin
  vars. `qa/runtime.test.mjs` covers the broader runtime boundary. Deployed
  observations stay in the runbook.

Old Go and retired-Worker receipts, as well as synthetic material, do not
establish provider usefulness, private deployment, phone acceptance or
independent remote recovery.
