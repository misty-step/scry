# External-agent authoring and review

External agents supply text, ordered lists and forward keyed mappings through
the implemented Owner API, CLI and MCP surfaces. Scry validates content and
owns grading; it does not generate content or accept client success grades.
See [Owner API](../docs/owner-api.md) and [MCP](../docs/mcp.md) for the exact
interface.

Multimedia remains part of the product direction, but media authoring is not
implemented in these surfaces. MCP reports `multimedia: false`. Local source
support and synthetic journeys do not prove hosted Owner API admission, which
remains unverified.

## Sub-features

- Validate and import immutable supplied content, then search/show it.
- Start practice and submit an answer through the same server-owned policy.
- Replay an operation safely without duplicate domain effects.
- Reveal is assisted; rejection leaves stored learning state unchanged.

## How to get to it (user POV)

An already authorized external agent uses the compiled `scry` CLI or `scry-mcp`
stdio server with an existing narrowly scoped Owner login. Tests create only
synthetic credentials in disposable local state. Creating real tokens or
configuring persistent access is not part of running this verification skill.

## Driving it with verify-scry helpers

Run `node --test qa/mcp-worker.test.mjs`. It launches the compiled MCP process,
authenticates to a disposable real Worker, imports the checked-in NATO fixture,
starts practice, submits a wrong spelling, verifies exact-v2 authority, replays
once, refuses conflicting/client-grade submissions and checks assisted review.
The independent SQLite read proves domain/receipt preservation; the fixture
asserts zero external egress and checks binary/WASM hashes before and after.

Save stdout, stderr and exit code. The `scry-mcp-workerd-proof-v1` receipt in
stdout binds the compiled MCP and Worker artifacts and lists the proof limits.
`qa/mcp.test.mjs` separately covers stdio protocol mechanics against a loopback
HTTP fixture; that fixture alone is not actual Worker or browser proof.

## Gotchas

Never print real Owner tokens or reuse operator config in a test. A successful
MCP journey does not verify phone UI or hosted admission. Report a missing
runtime or login as unexecuted/UNPROVEN, not PASS through another entry point.
