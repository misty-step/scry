# Scry feature map

[VISION](../VISION.md) explains intent, [SPEC](../SPEC.md) alone owns stories and
acceptance, and [DESIGN](../DESIGN.md) owns the
curiosity notebook. These source mappings describe the Rust replacement, not
production activation. US-012 is retired and its identifier remains reserved.

- [One question and honest judgment](study.md): US-002, US-003, US-007, US-008, US-011.
- [Create, preparation, and refinement](capture.md): US-004, US-013/S11.
- [Concepts, references, and goals](concepts.md): US-001, US-006, US-009, US-010.
- [Changes in this build](../SPEC.md#s12--inspect-the-changes-in-this-build):
  follow the footer's Changelog link to `/changelog`, then return through the
  header. Source: `src/web.rs`, `src/runtime.rs`. The existing `qa/walk --all`
  includes `qa/browser-release.mjs` for phone/desktop and light/dark navigation.
- [Private ingress and origin changes](../docs/runbook.md#canonical-origin-change--prepared-2026-10-20):
  S09, alternate-host reads/writes, and S10 backup handoff. Drive
  `qa/runtime-helper.mjs` with synthetic identity and the candidate origin vars;
  `qa/runtime.test.mjs` covers the broader runtime boundary. Deployed
  observations stay in the runbook.

This is the shared navigation inventory for verification and fitness. Intent
stays in SPEC; feature pages add only reach/drive instructions and source hints.
Use [verify-scry](../.agents/skills/verify-scry/SKILL.md), [Scry QA](../.agents/skills/scry-qa/SKILL.md) and
[verification](../docs/qa/system.md) for an isolated local workerd session and
actual browser/story proof. Old Go/retired-Worker receipts and synthetic material
do not establish provider usefulness, private deployment, phone acceptance, or
independent remote recovery.
