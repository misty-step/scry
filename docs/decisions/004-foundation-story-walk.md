# 004: Keep the pinned Foundation contract and required story-walk gate

- **Status:** accepted
- **Date:** 2026-10-13 (migration; original adoption: MIS-150)
- **Primitives:** foundation-gate
- **Powder:** not recorded; original scope MIS-150

## Context
Scry separates [product stories](../../USER_STORIES.md), [acceptance](../../SPEC.md),
[implementation mapping](../../features/README.md), and [verification](../qa/system.md).
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
choice; independent real-world obligations stay pending until retained evidence exists.
Revisit if the pinned contract or existing isolated browser can no longer verify
Scry's affected stories faithfully.
