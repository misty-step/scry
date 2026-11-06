# Checkpoint compatibility verification

The normal CI workflow does not run the old-Worker and fresh-space restore tests.
Until a separate owned gate is established, these checks must be run explicitly
against the exact proposed source before release; a skipped test is not a pass.
Do not merge or deploy merely because the ordinary CI is green.

Build the candidate Worker and CLI with the repository's pinned toolchain in an
isolated environment without private capabilities. Obtain the prior production
Worker artifact for commit `3936893491f7c88a025a3b59fb3e9ae63b3f0f90` from
[run 42279802289](https://github.com/misty-step/scry/actions/runs/42279802289).
The `worker/` modules must match the previous deployment receipt:

- `index.js`: `12de1dd5eef381a6c75b8b27d483ab559af466b1658a205258122472136ccfd2`
- `index_bg.wasm`: `73d9bc59b7cb6a668935ec20053c353294b4f462fdbf76a536b2e7fe6c4c9d69`

Run from the candidate checkout with the previous `worker/` directory supplied:

```sh
python3 scripts/checkpoint-manifest create > /path/to/source-manifest.json
SCRY_CHECKPOINT_SOURCE_SHA=$(git rev-parse HEAD) \
SCRY_CHECKPOINT_SOURCE_MANIFEST=/path/to/source-manifest.json \
SCRY_CHECKPOINT_OLD_WORKER=/path/to/previous/worker scripts/checkpoint-compat
```

The command fails if inputs are absent or tracked source files or symlinks differ from the manifest,
records the candidate commit and binary hashes, builds synthetic saved/cleared state,
checks the old module hashes, drives both old-Worker refusal cases and both old
archive refusals, restores a complete archive into a fresh synthetic notebook,
and exercises CLI lost-response reconciliation.
Retain the exact commit, any worktree diff and untracked-file manifest, candidate
and previous module hashes, command output and exit code. These checks do not
establish production acceptance, actual authenticated browser recovery, or an
independent remote archive restore. Hosted source-bound browser CI and independent
review remain separate requirements.
