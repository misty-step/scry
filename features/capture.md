# Create, preparation, and refinement

Stories: US-004, US-013

Intent: [SPEC US-013](../SPEC.md#us-013-create-from-what-i-want-to-learn),
[S11](../SPEC.md#s11--create-useful-material-from-intent-us-013) and
[US-004](../SPEC.md#us-004-check-generated-candidates-before-publication).

Source: src/web.rs, assets/app.js, src/engine.rs, src/generation.rs, src/runtime.rs, src/persistence.rs

## Reach and drive

Choose Create (`/create`; `/add` is an alias), enter text in the intent field
and submit. Follow the returned goal link, leave for Map and return; inspect
preparation and any offered retry. Use the optional photo chooser only with
run-owned synthetic material. Open a prepared concept's feedback action for
refinement, or the question's repair action and deliberate Save.

[QA](../docs/qa/system.md#core-journeys) owns setup, invalid-input and recovery
recipes. `qa/walk --stories "US-004 US-013"` selects the existing checks.
In the credential-free runtime capture the actual missing-provider state; do
not claim useful generated material or refinement from authored fixture JSON.
