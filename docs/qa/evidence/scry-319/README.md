# Phone Stream builder proof

Ticket [#319](https://github.com/misty-step/scry/issues/319), accepted slice plan
[#313](https://github.com/misty-step/scry/issues/313). Behavior authority is
[SPEC US-018](../../../../SPEC.md#us-018-practise-exact-recall-on-my-phone), with
US-007, US-016, US-017, UX4 and AG1.

The implementation starts at fixed review base
`f10e170d2967c9a7c3f5860a020a673c5cd05bb5` and retains that history. Parent
[Study PR #343](https://github.com/misty-step/scry/pull/343) moved to
`3344c8b16bf23f6f1a242c052b4e0840850d231b`, with the identical complete source
tree and fixed base `294f99808a352e066ca6a59cfaa8320eb37d2ee7`, as recorded in
[the parent integration report](https://github.com/misty-step/scry/pull/343#issuecomment-6297944183).
Root owns parent integration and bottom-first landing.

These are synthetic development observations, not a release receipt. The
fixture supplies authored clauses under Apostles' Creed and Roman Missal 2011
metadata. No canonical text, owner notebook, production capability or paid
provider is used. The compiled Worker uses fresh local SQLite and R2.

## Known prerequisite blockers

Parent QA confirmed [historical operation replay can disclose a target without
marking a newer Free recall Prompt assisted](https://github.com/misty-step/scry/pull/343#issuecomment-6297984170).
The shared handlers need that fence. The phone does not implement a separate
fence or alter immutable Attempts. Parent QA also found a
[malformed restore-dry-run panic](https://github.com/misty-step/scry/pull/343#issuecomment-6298022920).
Both belong to the prerequisite builder. Their fixes and affected verification
must be integrated before this branch's final independent QA.

## Reproduce

Use pinned runtimes and an `env -i` allowlist with no operator or provider keys.
Start `node qa/phone-serve.mjs PORT` and open its printed loopback URL with T3
preview. The second Unit opens in Free recall with its previous Unit visible.
Type `We concieved a tired garden the water extra.` for all five Diff marks.
Refresh the held result, correct it, choose Next, and inspect the next due time.
Start a fresh fixture for Show me before answering.

`qa/walk --stories "US-018"` runs the same journey at 320, 390 and 1280 CSS px
in light and dark. The 320 px cases double all computed text sizes. Native
forms also run with JavaScript disabled. A complete local R2 archive restores
the exact frozen Prompt into another isolated runtime. Compiled `scry answer
--json` must return the same Grade and Diff. Exact-route failure checks cover
unknown-response reconciliation, stale tabs, offline pause, automatic Good
Correction and access loss. The runner retains JSON and screenshots in its
owned `target/walk` directory and preserves all other discovery/gate behavior.

Emulation does not establish physical-phone behavior, keyboard occlusion,
private canonical wording quality, deployed private ingress, production
activation or learning efficacy. Those remain UNVERIFIED. No cold filesystem
nonallocation claim is made. Populated rollback needs a compatible forward
writer that retains disclosure Attempts and their effects.
