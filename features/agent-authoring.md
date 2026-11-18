# External-agent authoring and review

External agents supply text, ordered lists and keyed mappings (forward or both
ways, with optional safety-bearing pairs) through
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
- Start practice through the same server-owned policy: a new Unit arrives as
  source study, `quiz` moves to its Quiz, and the answer is submitted there.
- Take Study's mix (`scry mix`, `POST /api/v1/mix`, `scry_mix`), the same due
  review across Studies, then unfinished Studies and Works not yet started,
  that the app's Study and Next serve.
- Start and continue a lesson for any supplied Version, a mapping or a text of
  any admitted length included, not only the Starter shelf.
- Pause, resume or drop a Study and choose its Pace (`scry study pace`,
  `POST /api/v1/studies/pace`); these are API and CLI actions, not tools.
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
starts practice, is refused an answer against the studied pair, moves to its
Quiz, submits a wrong spelling, verifies exact-v2 authority, replays
once, refuses conflicting/client-grade submissions and checks assisted review.
The independent SQLite read proves domain/receipt preservation; the fixture
asserts zero external egress and checks binary/WASM hashes before and after.

`cargo test --locked --test agent_access` drives Study's mix, lessons for a
supplied mapping and a long text, a Study's Pace, a text reached from the mix
and Version Relations through the same route table, with replay, conflict and
protected-recall checks. `node --test qa/owner-api.test.mjs` drives the mix,
Pace and supplied lessons through the built Worker and the CLI.

Save stdout, stderr and exit code. The `scry-mcp-workerd-proof-v1` receipt in
stdout binds the compiled MCP and Worker artifacts and lists the proof limits.
`qa/mcp.test.mjs` separately covers stdio protocol mechanics against a loopback
HTTP fixture; that fixture alone is not actual Worker or browser proof.

## Gotchas

Never print real Owner tokens or reuse operator config in a test. A successful
MCP journey does not verify phone UI or hosted admission. Report a missing
runtime or login as unexecuted/UNPROVEN, not PASS through another entry point.
