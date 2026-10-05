# Run independent story QA

The executor implements Navi's Rule B for Scry PR #221, US-008/S2. It uses
`qa/walk-specs.mjs` and a frozen copy of `USER_STORIES.md`. The verifier account
for tonight is `moomooskycow`, separate from the runner-01 builder.

Install Playwright under an isolated writable directory. Chrome must already
be installed for Playwright's `channel: 'chrome'` launch.

```sh
npm install --prefix target/qa-tools --no-save playwright@1.63.0
node qa/build-artifact.mjs <candidate-sha> target/candidate-artifact
QA_VERIFIER_ACCOUNT=<account> node qa/executor.mjs \
  qa/pr-221-spec.json target/candidate-artifact target/candidate-packet \
  <fresh-verifier-session-id>
node qa/check-packet.mjs target/candidate-packet <candidate-sha> <frozen-spec-sha256>
QA_TEST_PACKET=target/candidate-packet node --test \
  qa/check-packet.test.mjs qa/executor-contract.test.mjs
```

Use a new output directory for each build and run. The build archives exactly
the supplied commit, compiles once, and records source archive and binary
checksums. Go build and module caches stay under `target/qa-cache`. Delete that
owned cache after the final build. The executor also accepts an existing bundle
with the same artifact manifest, executable, and compiled boundary test files.
It verifies their checksums and the executable's reported candidate revision.

The required story set is the union of the frozen spec and the verifier's map
of changed paths. Grading changes require US-003 and US-008. Unknown paths,
deleted paths, and renames to unknown paths select every frozen live story.
This map belongs to the verifier, not the candidate. The packet checker derives
the set again from the actual diff; a candidate cannot remove its own tests
and pass an empty test run.

Chrome uses a 390 by 844 viewport, pointer and keyboard actions, screenshots,
and video. Synthetic Jev responses cover thresholds, identity and injection
risk, uncertainty, unavailable and malformed checks, deliberate retry, exact
replay, learner authority, semantic shadow classes, and denied mutations.
The same executable seeds and serves each unused fixture database. Reload and
restart assertions inspect persisted results. Environment allowlists exclude
provider and production credentials.

The packet records each story criterion, actions, expected and observed values,
candidate and artifact identities, verifier, evidence checksums, and exclusions.
Pure Go boundary calls also have literal expected and observed outputs in a
separate compiled verifier probe. Their JSON evidence proves API behavior;
video proves the UI walkthrough. Candidate-authored compiled Go tests are
supplementary evidence and retain their full output. Missing, failed, skipped,
or blocked criteria fail admission. A new candidate invalidates the verdict.

[The dated proof](../docs/qa/rule-b-pr221-20261004.md) records the negative control
and passing packet. Local synthetic preview QA does not establish production
Access ingress, live Jev calibration, physical-phone acceptance, deployment,
remote backup, or release eligibility. The release lane must promote the tested
`target/pr221-artifact-v2/scry` bytes unchanged, or rerun QA on its own artifact.

The controller must assign the verifier separately from the builder. Packet
fields record that assignment; they do not replace authenticated orchestration
or grant release authority. Screen evidence and upload it to private R2 before
using the packet as durable release evidence.
