# Takeover builder rerun, 2026-10-20 20:54 UTC

A second builder (Claude Opus 5.5) took over [PR #345](https://github.com/misty-step/scry/pull/345)
after the first builder stopped before reporting. This entry adds one more
observation of the [recovery-link receipt](../README.md) beside it. It does not
edit that receipt, its `SHA256SUMS`, the [fix-345 receipt](../../fix-345/README.md)
or the [original phone receipt](../../README.md). SPEC alone owns intent:
US-018/016/017/007, preserved US-002.2, S02.2/.6, D3, S06 and US-013/S11.

## Identity

- Source: committed `b8155fab94740813e7e8530a45887d7701216bb1`, from a fresh
  `git archive`. Its SHA-256 `9202c5eb…256d6` equals the recorded `source_tar_sha256`.
- Artifact: the same frozen 0.3.0 Worker, WASM SHA-256
  `a55d8e5050152fda4b0ea96ecd9631dd17b8f2a43d30701b6029812943dc9773`, which
  [browser-results.json](browser-results.json) re-hashed at walk time. I did not
  rebuild it, so this rerun makes no reproducible-build claim.
- Branch head before this entry: `e8968b295929e97aa0e74e160682f28881a9d63d`. It
  changes only docs/evidence relative to b8155fa.

## Observed

- **Browser, first run: PASS 2/2 links, 10/10 no-Study walks.** I ran the
  committed [browser-walk.mjs](../browser-walk.mjs) once in the fresh owned
  directory `~/.cache/tmp/scry-345-opus-walk.Iz8FS8`. It used `env -i`, Node
  22.22.0, `npm ci` from the committed lockfile, Playwright 1.63.0 and local
  headless Chromium build 1243. This was a local headless run, not the VM; the
  viewport was 390px, dark, touch. Provider bindings were absent and egress was denied.
  I inspected the screenshots myself:
  - [Without JavaScript, Check again](screens/legacy-home-native-checking-after.png)
    lands on `/notebook` showing "My saved uncertain freshness answer".
  - [Keep trying](screens/legacy-home-js-kept-question.png) from
    [the gate](screens/legacy-home-native-gate.png) returns to the open legacy question.
  - [Root afterwards](screens/legacy-home-native-exact-unchanged.png) shows
    the same exact Prompt. The walk also asserts that SQLite exact practice is unchanged.
- **Native census mutants: all caught.** I reverted each of the three view
  sites to `"/"` in turn. Each one fails `mixed_notebook_recovery_links_follow_legacy_home`:
  [checking-reference](mutants/mutant-1.txt) with 8 wrong destinations,
  [Keep trying](mutants/mutant-2.txt) with 10, and [Check again](mutants/mutant-3.txt)
  with 2. These sum to the receipt's 20. The [unmutated](mutants/unmutated.txt)
  test passes. The full `--test study --test web_render` suite passes 34/34.

## Limits

This entry inherits every limit in [the receipt](../README.md#retained-acceptance-and-limits).
Physical phone, canonical wording and owner acceptance (#320), hosted ingress
(#335), fresh/independent hosted restore (#318) and provider quality/spend
(#322) remain UNVERIFIED. Independent delta QA, review, exact-head CI and judgment
are still required. This entry claims no merge, deploy or activation.
