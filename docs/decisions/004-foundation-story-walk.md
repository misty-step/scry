# 004: Keep the pinned Foundation contract and required story-walk gate

- **Status:** accepted
- **Date:** 2026-10-13 (migration; original adoption: MIS-150)
- **Primitives:** foundation-gate
- **Powder:** not recorded; original scope MIS-150

## Context
Scry separates [product stories](../../USER_STORIES.md), [acceptance](../../SPEC.md),
[implementation mapping](../../features/README.md), and [verification procedures](../../.agents/skills/scry-qa/SKILL.md).
The Foundation contract is pinned in [foundation.json](../../foundation.json).
Synthetic browser proof cannot establish real generation, protected production
ingress, recovery, or physical-phone acceptance.

## Decision
Keep the pinned Foundation and required story-walk gates: an unwalked or failed
affected story blocks the gate; a pending disposition is not a compliance claim.
Use the existing test-only Playwright 1.63.0 and distribution Chromium for native
pointer/keyboard walks on the isolated VM, without adding an application dependency
or frontend build.

## Consequences
[CI](../../.github/workflows/ci.yml) validates source-bound, artifact-hashed receipts.
[Setup](../../.exe/setup.sh) and [qa/walk](../../qa/walk) reuse the existing browser
choice: [the Dagger gate](../../.dagger/src/index.ts) already installs 1.63.0
and [the critic resolver](../../scripts/critics/lib/browser.mjs) already locates it.
Setup installs the test-only module in a user-owned prefix and distribution
Chromium through Ubuntu apt only under noninteractive sudo. CI provisions the
same browser; no secrets or provider requests are required. Independent
real-world obligations stay pending until retained evidence exists.

The dated `obl:FND-WLK-001`, `obl:FND-REV-001`, and `obl:FND-SEC-001` allowances
defer Foundation assessment/evidence paperwork, not the required story-walk,
exact-head independent review, or security contracts. There are no `walk:*`
allowances: an unwalked or failed affected story still blocks the receipt gate.
Bootstrap mode makes the structural check advisory about missing walk evidence;
it is not a passing walk or permission to merge or activate.
Revisit if: the pinned contract or existing isolated browser can no longer verify
Scry's affected stories faithfully.
