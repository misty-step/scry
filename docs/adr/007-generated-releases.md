# ADR 007: Generated releases for the Rust application

Status: superseded for shipping by the owner's 2026-10-29 simplification
decision; retained below as history. The [current procedure](../rust-cloudflare.md#reviewable-release-procedure)
deploys the tested merged-source artifact directly. Generated metadata PRs,
release tags and repeated CI are no longer deployment prerequisites. Historical
versions/notes remain intact; intentional version bumps are ordinary source
changes. No current runtime behavior or saved data is changed by retiring the
generator.

Scry has one release unit: the Rust Worker, its presentation assets and bundled
notes. npm is build/QA tooling, not a separately released program. Follow the
[shared release convention](https://github.com/misty-step/harness/blob/master/docs/release-convention.md).

release-please's Rust strategy is the sole version/notes authority. It updates
Cargo.toml/Cargo.lock plus the npm manifest and root lockfile versions, and
creates CHANGELOG.md, release PRs, vX.Y.Z tags and matching GitHub Releases.
Landmark remains synthesis-only, never a second publisher or version authority.
No model call is needed for release notes.

The adoption baseline is explicitly source
`560d711cf71c6f4fc4aafa21fdfda4aeb91a67d5`, with the generator's unreleased
sentinel `0.0.0` in the release manifest. No prior semantic release exists;
neither unmaintained package version is a release baseline. Do not retag or
reinterpret the historical `mis59-*` tag. The first feature after this baseline
is expected to generate the Rust strategy's initial 0.1.0, without a broken
comparison to a nonexistent previous tag. Subsequent breaking changes go to 1.0.0. Manifest
versions only change through generator output in release PRs. Cargo and npm's
previously inconsistent versions converge in that first generated release PR.

State transitions:

    green exact master CI -> generator opens/updates release PR
    -> dispatch real CI on exact bot branch/head -> agent checks version/notes
    -> squash merge release PR -> green exact master CI -> tag/release
    -> attach that CI's tested Worker ZIP -> dispatch existing promotion
    -> scry-promote.py verifies custody and promotes those same bytes

The generator runs after successful master push CI, not from PR code or a
failed/skipped gate. An outdated ancestor head is skipped only while the
current head has no successful master push CI. If that CI has succeeded, the
release proceeds for the current head. A divergent or unknown head, or a
missing or invalid bootstrap, is refused through the existing failure-issue
path. Replays use the same release PR and immutable tag; no-release inputs
leave releases unchanged. Release and promotion serialization do not cancel a
running promotion. Failures use the existing Actions failure-issue path, with a
run URL and no notebook content.
There is no new credential, runner, deploy engine, store, migration or route.

The private `/changelog` is rendered from CHANGELOG.md compiled into the Worker,
with the Cargo version and source revision stamped by the existing source-bound
gate. It bypasses learning-state loading only after the existing owner ingress
check. Health keeps its existing body/status and adds version/revision headers.
A revision without a release still deploys independently using the same existing
CI promotion path. A tag records source, not proof that production serves it.

Rollback remains the existing compatible-code recovery in
[the release procedure](../rust-cloudflare.md). It does not restore old data,
reset learner state or touch credentials. Prove the new page and preservation
on disposable synthetic state; retained revival receipts continue to own the
original production import/cutover proof. Broad rewrite acceptance stays open.
