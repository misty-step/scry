The independent Rule B executor is ready on `cursor/scry-qa-v0`, draft PR
https://github.com/misty-step/scry/pull/222.

What works: the frozen spec plus actual diff requires US-003 and US-008. All
eight criteria pass on PR #221's exact head
`9489d43aa0d5b501f875f89dc81cf8d64694b118`. The tested executable SHA-256 is
`ab39b1572dc03460ec4c23c66894292f2964bf21b6235abe01b00f3c0215df28`.
The fresh verifier is `moomooskycow`, separate from the runner-01 builder.

Proof: Chrome with `channel: 'chrome'` produced 28 screenshots and 22 videos.
Actual persisted state, threshold decisions, failure and permission paths,
replay, learner authority, reload, and restart are asserted. Sixteen admission
and selection tests pass. A throwaway commit widened the identity-risk limit;
QA rejected its incorrectly accepted answer, then the commit was reverted.
The restored tree matches #221. The build cache is deleted.

Passing packet:
`/home/box/agent-data/chief-of-staff/factory/audit/evidence/scry-qa-pr221/passing/packet.json`

Negative packet:
`/home/box/agent-data/chief-of-staff/factory/audit/evidence/scry-qa-pr221/negative/packet.json`

Tested executable and build manifest:
`/workspace/lanes/scry-qa/repo/target/pr221-artifact-v2/scry`
`/workspace/lanes/scry-qa/repo/target/pr221-artifact-v2/artifact.json`

What's left: private R2 upload needs the owner's scoped uploader. The controller
and release lane must adopt the packet and promote these same bytes, or rerun
QA on their artifact. Production Access, live Jev calibration, deployment,
and physical-phone acceptance remain outside this synthetic verdict.

Glass request: the existing collector-produced schema-1 snapshot validates
locally but live ingest returns HTTP 400 `request_rejected`. Please reconcile
the deployed validator and collector; a new event shape has not been proven
necessary. QA observations are appended to factory `audit/scry-qa.jsonl`.
Delivery is not a release gate. No Glass repo edits were made.

The lane parent is read-only under this session's filesystem policy. This
`final.md` therefore lives in the writable repo, with a copy in factory
`audit/evidence/scry-qa-pr221/final.md` for handoff.
