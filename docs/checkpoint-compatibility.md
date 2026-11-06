# Checkpoint compatibility verification

The normal CI workflow does not run the old-Worker and fresh-space restore tests.
Until a separate owned gate is established, these checks must be run explicitly
against the exact proposed source before release; a skipped test is not a pass.
Do not merge or deploy merely because the ordinary CI is green.

Use an isolated environment without private capabilities. Independently authenticate
the candidate pull-request GitHub Actions run, its successful conclusion, PR head,
base, synthetic merge commit and artifact ZIP SHA-256. The run API head SHA is
the PR head; the checked-out CI revision and artifact source revision are the
synthetic merge commit. Obtain the latter from authenticated CI metadata and
the artifact source metadata, and confirm it has the exact base and head as its
first and second parents. Check out that merge commit in a clean isolated Git
checkout; do not relabel it as the PR head. The fixture and CLI checks run from
the same merge tree used to build the Worker. If the merge object, parents or
current PR head/base cannot be verified, stop. The helper checks
that ZIP and its complete source inventory against the clean committed checkout,
but cannot authenticate GitHub, the run ID, the supplied digest, or CI itself.
Obtain the prior production
Worker artifact for commit `3936893491f7c88a025a3b59fb3e9ae63b3f0f90` from
[run 42279802289](https://github.com/misty-step/scry/actions/runs/42279802289).
The `worker/` modules must match the previous deployment receipt:

- `index.js`: `12de1dd5eef381a6c75b8b27d483ab559af466b1658a205258122472136ccfd2`
- `index_bg.wasm`: `73d9bc59b7cb6a668935ec20053c353294b4f462fdbf76a536b2e7fe6c4c9d69`

Run from that authenticated merge checkout with the previous `worker/` directory supplied:

```sh
python3 scripts/checkpoint-manifest create > /path/to/source-manifest.json
SCRY_CHECKPOINT_SOURCE_SHA=$(git rev-parse HEAD) \
SCRY_CHECKPOINT_PR_BASE=<authenticated-base-commit> \
SCRY_CHECKPOINT_PR_HEAD=<authenticated-pr-head-commit> \
SCRY_CHECKPOINT_SOURCE_MANIFEST=/path/to/source-manifest.json \
SCRY_CHECKPOINT_OLD_WORKER=/path/to/previous/worker \
SCRY_CHECKPOINT_CI_ZIP=/path/to/candidate-ci-artifact.zip \
SCRY_CHECKPOINT_CI_SHA256=<authenticated-zip-sha256> \
SCRY_CHECKPOINT_CI_RUN_ID=<authenticated-successful-run-id> scripts/checkpoint-compat
```

The command fails if inputs are absent or tracked source files or symlinks differ from the manifest,
checks the complete tracked tree and CI source archive, extracts and hashes the CI Worker,
builds the CLI from the clean checkout, builds synthetic saved/cleared state,
checks the old module hashes, proves that the same old Worker and storage path
read a compatible baseline before each old-Worker row refusal case, restores a
compatible archive through the same old Worker before each old archive refusal,
restores a complete archive into a fresh synthetic notebook,
and exercises CLI lost-response reconciliation.
Retain the PR head, base, merge commit and merge tree, authenticated run and artifact metadata and ZIP digest,
candidate and previous module hashes, command output and exit code. A run ID or
ZIP hash supplied without independently authenticated CI evidence is not proof of origin. These checks do not
establish production acceptance, actual authenticated browser recovery, or an
independent remote archive restore. Hosted source-bound browser CI and independent
review remain separate requirements.
