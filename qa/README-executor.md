# Independent story QA

Rule B executor work is based on PR #221 head
`9489d43aa0d5b501f875f89dc81cf8d64694b118`, built by a separate runner-01 lane.
The verifier freezes the story contract, derives required stories from that
contract and the actual Git diff, and runs a Chrome walkthrough on an isolated
synthetic preview. Missing, blocked, skipped, or failed criteria fail the run.

The packet must bind candidate/tree, executable checksum, verifier identity,
action, expected and observed behavior, screenshots, and video. Synthetic
provider responses establish policy mechanics; holdout quality, production
Access ingress, deployment and physical-phone acceptance remain separate.
