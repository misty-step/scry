# Library refusal delta

Builder proof for [#316](https://github.com/misty-step/scry/issues/316),
[US-015.4](../../../../SPEC.md#us-015-find-a-work-before-creating-it), and
[PR #340](https://github.com/misty-step/scry/pull/340). Tested implementation
commit: `3f0963c6364d8296a9e099072a0723a3863efc78`, descending from `b81247d`.
[proof.json](proof.json) binds source, exact built Worker and CLI, commands,
logs, negative controls, and limits. This is focused builder verification.
Independent delta QA, review, and a fresh judge remain with the parent.

The repaired journey passes its title through `addArgs` once, requires the
CLI's API-refusal class, and separately requires HTTP 422 with the exact Worker
join error. It compares all SQLite rows after each refusal, including receipts
and token last-use. Capacity checks retain 4,096 Sections and 8,192 total Units,
return distinct static errors, and apply equally to automatic and supplied
Sections. The API reference documents both count bounds and the unchanged
generic CLI diagnostic. No segmentation or stored bytes change.

- [Before](original-join.txt), the original committed journey passed while its
  CLI returned `Unexpected Version arguments.` before HTTP.
- [Duplicate-flag control](duplicate-flag.txt) fails the improved assertion.
  [Removing only the join guard](missing-join-guard.patch) from the built Worker
  [also fails](missing-join-guard.txt), because the bad add exits zero.
  Candidate source was restored byte for byte before the passing build.
- [Native before](native-before.txt) fails on the generic capacity error;
  [native after](native-after.txt) passes all five Library tests.
- [Built Worker and CLI after](library-after.txt) passes all eight shared
  journey functions. [Observed responses](observations.json) include the exact
  join refusal, both limits accepted, each limit plus one refused for default
  and explicit Sections, and 5,000 short lines refused below the byte limit.
  Refusals preserve every SQLite row. Native Clippy and the release Worker
  build also pass. The full story selector and unrelated gates were not rerun.

The [independent full QA report](https://github.com/misty-step/scry/pull/340#issuecomment-6296126221)
passed at `87954f2`: 124 native tests, eight walks, 12 independent probe groups,
and 77/77 combined checks. Its separate CRLF mismatch probe established real
422/no-write behavior despite the faulty committed walk.
The [independent storage review](https://github.com/misty-step/scry/pull/340#pullrequestreview-5588389462)
found no blockers at that source. Both inspected evidence-only `b81247d`;
retain their proof for unchanged behavior without treating either as delta
approval or judge SHIP. The original builder's 76/77 run remains failed.
Independent isolated and combined runs found no persistent listener defect.

Data is caller-authored synthetic text, with Creed/Missal metadata only.
The isolated Worker denies external egress. No private env, live application
credentials, provider calls, spend, production or historical data, or deployment
was used. Used-Study refusal remains native reference-contract proof until
#317. Real Study/Attempt integration, the full private Roman Missal wording,
hosted ingress #335, physical phone, and independent hosted recovery remain
UNVERIFIED. Populated Library rows require a compatible build. The passing
runtime log retains a non-failing Miniflare RPC-stub disposal diagnostic.
