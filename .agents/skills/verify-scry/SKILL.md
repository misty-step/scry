---
name: verify-scry
description: Use Scry as a returning learner on desktop and phone and retain real journey evidence.
---

# Verify Scry

Start with [the shared feature map](../../../features/README.md) and its
[SPEC](../../../SPEC.md) references. This is the navigation inventory for both
verification and fitness, not another acceptance contract.

## Launch and diagnose

[scry-qa](../scry-qa/SKILL.md) and [QA](../../../docs/qa/system.md) own the
existing Rust/workerd launch, synthetic identity, authored demo, commands and
cleanup. Use Mirrodin under `/srv/hermes-chief-workspaces/<task-id>`, the pinned
runtimes in README and fresh run-owned state. `bun run dev` starts the real
Worker with local SQLite/R2 and no provider credentials. Inspect the printed
loopback target before driving; never import a live notebook to populate it.

For deployed inspection, first read the [runbook](../../../docs/runbook.md):
source HEAD and serving architecture can differ. Use only the approved owner
Access login for the owner's notebook, or the separately approved QA identity
for its own notebook under [S09](../../../SPEC.md#s09--keep-the-application-private).
Source support does not establish deployed QA configuration or authorize its
credentials. A login redirect is denial evidence, not a completed learner
journey. Do not borrow a protected human session or spend on generation to
qualify this documentation.

For an origin change, start with the runbook's
[dated promotion observation and remaining proof](../../../docs/runbook.md#canonical-origin-serving-at-1635-utc-2026-10-20)
and the [backup handoff](../../../docs/rust-cloudflare.md#canonical-origin-promotion).
Retain separate local and deployed ingress observations; an isolated story walk
does not prove that the serving origin changed.

## Walk as Phaedrus

Treat this as a short learning break on his phone, not an admin inspection.
Use actual pointer/keyboard actions at desktop and 390px touch widths in both
color schemes, following these map entries:

1. Create: open Create, enter a curiosity, submit, follow its saved preparation,
   leave and return. In the credential-free stack inspect the honest stopped
   preparation rather than pretending generated material exists.
2. Study: in a separate fresh synthetic notebook, explicitly load the authored
   demo. It opens a question directly; use Start practicing only if an actual
   introduction offers it. Answer, read the held feedback, refresh, then choose
   Next. Try an incorrect answer and the visible help route too.
3. Map/reference: open Map, follow a displayed goal/concept, inspect reference
   through its assistance gate when offered, then return to the same study.

Judge whether you know what to do next, can read and type comfortably, learn
anything from feedback, and understand a failure without knowing the backend.
Record the moment you hesitated and the exact screen/action, not a generic
"looks good". Inspect screenshots for clipped controls, obscured feedback,
keyboard focus and long text. Synthetic demo mechanics do not prove useful
model output or lasting recall; emulation is not physical-phone acceptance.

## Evidence and cleanup

Keep the actual command/result transcript and inspected screenshots/video with
source SHA, target and serving identity where available, SPEC IDs, data mode,
actions and observed success/refusal/recovery. Attach sanitized media through
card top-level `artifacts`, and include usable proof in the PR How to review.
Do not export private notebook content, auth state, cookies or raw traces.
Unobserved behavior stays unverified. Stop only the run-owned process and
preserve evidence before removing its temporary state. Use the shared
`maintain-verification-skill` for navigation drift; never redefine SPEC to make
a failed walk pass.
