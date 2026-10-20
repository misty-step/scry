# Library #316 proof

[Issue #316](https://github.com/misty-step/scry/issues/316) implements
[SPEC US-015](../../../SPEC.md#us-015-find-a-work-before-creating-it).

The original committed join-refusal walk below rejected duplicate CLI flags
before HTTP, so its unchanged-row assertion is not Worker refusal proof.
The [focused delta receipt](delta/README.md) corrects that walk, retains failing
negative controls and passing real-Worker observations, and links the separate
independent full QA and storage-review reports. Original logs retain their bytes.

The implementation source is `afd1d0b8e852e12f8fa6cf9aec693ea3c8ec0cbb`,
stacked on fixed API base `a0fd5d0306b4a6d39b7a45496b29380d0c968503`.
Integration merge `07ab0adf504753d3f09d86e41fecd3c3f82c32dd` separately brings
merged master `79d0f8b`, including the shared #315 Unicode implementation and
#333 canonical origin. The parent API branch is unchanged. After #332 merged to master as
`28db32deda37488740cacc699adab0068a38dfd4`, ordinary merge
`36e103bf6b0843f40208a4bb984cc742106b1622` integrated it into this branch.
The resulting tree equals the tested feature tree byte for byte; all own
commits remain intact and the PR targets master. #321 remains outside this scope.

[Before](before.json) records the fixed-base compiled CLI rejecting the new
search command before HTTP. [The story receipt](walk-receipt.json) binds all
eight passing criterion walks to committed source, its inventory hash, and
the actual built WASM. [The CLI transcript](logs/US-015-cli.json) retains
sanitized commands and real output through the loopback Worker and SQLite.
It also records full local R2 archive readback.
[Native output](logs/rust-tests.txt) includes the durable reference refusal test.
[Check results](checks.json) records the remaining native, target, and runtime
checks. The pinned combined runtime run passed 76 of 77 checks. Its existing
listener-allocation test had a transient `fetch failed` on a concept-create
retry; the isolated rerun passed. Both logs are retained. Every Library walk
passed in the combined run. No browser is needed for this CLI-only story.

The text is caller-authored synthetic material. Creed and Roman Missal 2011
labels exercise metadata, not a claim to canonical wording. No private env,
live credential, provider, production namespace, or historical store was used.
The isolated runtime rejects unapproved egress.

The used-Study deletion case is native proof of `study_version_refs`, as the
issue permits until #317. #317 must populate that reference in the same
transaction as real Study creation and keep it for dropped Studies and Attempts.
The supersession walk proves that the old Version and Unit identities remain
readable and unchanged. Actual Attempts against that Version require #317.
The owner's complete private wording, physical phone, hosted CLI ingress, and
independent hosted recovery remain unverified. Local R2 is an emulator.

The added evidence commit changes no executable source. This is development
proof of committed code, not a release artifact or authorization to activate it.
