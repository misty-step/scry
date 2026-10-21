# Create, preparation, and refinement

The owner writes what he wants to learn, Scry saves it and prepares material,
and he refines it from his attempts and feedback. Stories: US-004, US-013.
Intent: [US-013](../SPEC.md#us-013-create-from-what-i-want-to-learn),
[S11](../SPEC.md#s11--create-useful-material-from-intent-us-013) and
[US-004](../SPEC.md#us-004-check-generated-candidates-before-publication);
[S10](../SPEC.md#s10--keep-my-work-recoverable) owns recovery.

## Sub-features

- `create-save` saves a word, phrase or long ramble from `/create` (`/add` is an alias), with an optional photo.
- `create-preparation` shows pending, partial, failed, missing-provider, exhausted or unknown preparation honestly.
- `create-refine` saves concept feedback, then Refine my material on the goal.
- `create-repair` opens a question's repair draft for a deliberate Save.
- `create-resume` resumes paused preparation from Settings (`/settings#preparation`).

## How to get to it (user POV)

- Masthead Create → "What do you want to learn?" → Make it understandable →
  follow the saved goal, leave for Map and return.

## Driving it with verify-scry helpers

Preconditions:

- A run-owned synthetic Worker; production generation is off and provider calls are never made in verification.

- **Story walk.** `qa/walk --stories "US-004 US-013"`. `qa/browser-create.mjs` and
  `qa/browser-recovery.mjs` (inside `qa/walk --all`) cover stopped preparation,
  Resume preparation and recovery with authored inputs. [QA](../docs/qa/system.md#core-journeys)
  owns the setup, invalid-input and recovery recipes.
- **Production.** [qa-signin.mjs](sign-in.md) loads `/create` in the QA
  account's `scry-qa` space. Never submit it on production.
- **Proof.** Capture the real missing-provider state in the credential-free runtime.

## Gotchas

- Authored fixture JSON does not prove useful generated material or refinement.
- Use the photo chooser only with run-owned synthetic material.
- Source: `src/web.rs`, `assets/app.js`, `src/engine.rs`, `src/generation.rs`,
  `src/runtime.rs`, `src/persistence.rs`.
