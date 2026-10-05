# Scry release adapter v0

`release.mjs` promotes an uploaded Worker version and a published container image
to synthetic staging. It records the deployment ID, version ID, image digest,
container rollout ID, observations, rollback, and recovery in one JSON receipt.
It does not build application code, seed data, restore a database, send model
requests, or change production. The existing full CI remains required.

## Prepare the release inputs

The release authority, separate from the builder, supplies the manifest and
credentials. The builder supplies candidate evidence, not release authority.
The Workflow controller must hold its repository/environment release slot from
baseline reconciliation through recovery. Every deployment writer must honor
that slot. Cloudflare's deployment API has no conditional deployment write.
The adapter's final identity read suppresses a rollback when a newer deployment
is present, but cannot close a concurrent write between its GET and POST.

Prepare the candidate on runner-01 from the exact integration revision using
`ci:full --require-committed`. Preserve its tested binary without rebuilding.
Package and publish the image with that binary, and record the immutable
registry digest. Upload the matching Worker with the pinned Wrangler's
`versions upload --env staging`. Uploading a Worker version does not roll out
a container. Candidate QA and the manifest must bind the same Worker/image pair.
This adapter promotes those uploaded artifacts through Cloudflare's APIs.

Copy [release-artifact.example.json](release-artifact.example.json) and replace
every synthetic value with the approved artifact identity. The previous
deployment ID, Worker version, and image must match the current environment.
`evidence` identifies the exact candidate packet. `compatibility_evidence`
identifies the reviewed proof that the previous binary can read the candidate's
persisted data. `rollback_compatible: true` is a release-authority assertion,
not a compatibility test performed by this adapter. Never permit a schema or
binding migration under that assertion without independent proof.

Keep the existing staging Access owner guard, isolated recovery bucket, synthetic
fixtures, and runtime settings. The container application must already exist as
`scry-app-container-staging` with the scheduler-backed configuration used by the
pinned Scry hosting setup. The adapter preserves its container configuration
and changes only its image. It refuses the newer `durable_object` scheduling
policy because that policy does not support rollout versions.

Production is excluded from v0. The [runbook](../../docs/runbook.md) requires a
quiesced writer and verified final backup before replacing a live-data image,
including during rollback. Those steps must become executable release stages
before production support is enabled. No database rollback belongs in this
adapter.

## Supply credentials

The only credential source is the invoking operator's
`~/.config/scry/release.env`. The file must belong to that operator and have no
group or other permissions. Values are literal `KEY=value`, optionally quoted.
The parser does not source a shell, expand variables, or read inherited token
variables. It rejects unknown keys and never accepts `CLOUDFLARE_API_TOKEN`.

The required keys are:

- `SCRY_RELEASE_CF_TOKEN`, a dedicated release token authorized for this staging
  Worker's versions/deployments and staging container applications/rollouts.
- `SCRY_RELEASE_ACCOUNT_ID`, the account containing those artifacts.
- `SCRY_PROBE_TOKEN`, the staging health/readiness probe capability.
- `SCRY_ACCESS_JWT`, a valid staging owner Access JWT for the exact immutable
  owner subject configured on the Worker.

If the outer Access policy needs a service token, also provide
`SCRY_ACCESS_CLIENT_ID` and `SCRY_ACCESS_CLIENT_SECRET`. A service token alone
does not satisfy the Worker's owner-subject guard. A probe token reaches only
health/readiness, not the private Map journey. An expired or unavailable owner
JWT produces `UNVERIFIED` and prevents deployment at the baseline check.

Do not pass tokens through arguments, URLs, repository files, or feed events.
The receipt omits response bodies, cookies, provider errors, and private page
contents.

## Run the stage

Only run an authorized staging release after the credential file and exact
candidate packet exist, while the controller holds the shared release slot:

```sh
node deploy/cloudflare-hosting/release.mjs /private/artifact.json /private/outcome.json
```

The second path must be new. The adapter checkpoints the outcome before each
deployment attempt and after receiving its identity. An existing receipt
prevents repeating effects. A killed process or lost API response leaves an
unverified attempted operation for the controller to reconcile by identity.
Do not delete its receipt or automatically retry it with a fresh filename.
There is no database, queue, or new orchestration service.

After a healthy baseline, the stage promotes the Worker version and starts one
100% container rollout. It checks the expected Worker deployment/version, target
image, completed rollout, `GET /healthz`, `GET /readyz`, and the authenticated
`GET /map` immediately, at 150 seconds, and at 300 seconds. The Map check asserts
the real application's Map shell and success state without recording content.
This read-only availability journey is separate from PR #221's grading QA.

A failure receives one confirmation ten seconds later. Two accessible failures
trigger one compatible code/image rollback only while the failed deployment
still matches the current deployment and the container image is unchanged.
The stage checks recovery ten seconds after starting that rollback. A pending
rollout or inaccessible recovery stays unverified and needs operator follow-up.
There is no automatic second rollback or retry of an uncertain write.

Standard output contains one JSON object. Exit code 0 means `PASS`; every other
outcome exits 1. `ROLLED_BACK` means the release failed and recovery passed,
so it does not mark the story shipped. `SUPERSEDED` means another deployment
or image replaced this run. `UNVERIFIED` cannot mark the story done. Failed
recovery reports `FAIL`. Feed delivery is the controller's responsibility and
does not block this adapter.

## Verify the adapter

```sh
node --test deploy/cloudflare-hosting/release.test.mjs
```

The tests exercise the actual adapter and Cloudflare HTTP serialization with
synthetic responses and a simulated clock. They prove the five-minute schedule,
confirmed failure and exactly one Worker/image rollback, both newer-deployment
guards, inaccessible observations, incomplete image rollout, failed recovery,
unknown-write handling, and CLI credential/receipt boundaries. They do not prove
Cloudflare credentials, live rollout behavior, grading, or phone acceptance.
The hosting tests also run inside the existing source-snapshot CI gate.

The protocol follows Cloudflare's official
[Worker deployment API](https://developers.cloudflare.com/api/resources/workers/subresources/scripts/subresources/deployments/methods/create/),
[container rollout API](https://developers.cloudflare.com/api/resources/containers/subresources/applications/subresources/rollouts/methods/create/),
and [container deployment guide](https://developers.cloudflare.com/containers/guides/deploy/).
