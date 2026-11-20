# AGENTS.md

You are building Scry from scratch. PRODUCT.md is what it does. DESIGN.md is how it looks and feels. Those two files are the whole contract. You have no other source: do not look for the old app, and do not fetch it from the network.

## Rules
- Language: Go (current stable), one deployable service. Server-rendered HTML with light progressive enhancement; keyboard first. SQLite for data unless you have a stronger reason.
- Every PRODUCT.md user story and business rule must be implemented and have an automated test that names it. Every DESIGN.md token, layout and state must be implemented.
- External services (email/password auth provider, Stripe billing, the AI model provider, speech) sit behind small interfaces with a test-mode adapter that runs with no keys. In test mode, auth uses Clerk only with both Clerk keys and billing uses Stripe only with all four Stripe settings; otherwise each uses its fake. Model and speech stay fake, and no operator alerts are sent. Live mode requires all adapter credentials from environment variables; credentials are never committed. Stripe is pinned to one explicit mode whose keys and webhooks must match it: live Stripe in live mode, test Stripe in test mode, and test Stripe in live mode only with the explicit staged-cutover switch SCRY_STRIPE_MODE=test (surfaced in /healthz). Test mode never accepts live Stripe.
- Judging an answer must use a model with a one-line reason. No keyword, fuzzy or heuristic matcher decides correctness, except texts marked exact, which are judged word for word.
- Data durability: your design must make per-user data private and recoverable. Document backup in README.md.
- Reliability: expose /healthz and structured logs so outages are noticed.

## Run and test
- `go build ./...`, `go test ./...`, `go vet ./...` must pass.
- `make run` starts the app on PORT (default 8080) in test mode with seeded demo data. `make test` runs all tests.
- Provide a Dockerfile that builds a small static image and runs the app.
- Write README.md covering run, test, env vars, the adapters, and a story-by-story table mapping each PRODUCT.md story and rule to its test.

## Deliver
Work in this directory. Commit as you go with git. Finish with everything building and tests green, and write NOTES.md listing every place PRODUCT.md or DESIGN.md was ambiguous or silent and the choice you made.

## Monitoring layout
- cmd/scry-watch and internal/watch own durable incidents and service restarts separately from app SQLite. deploy/fault-drill.py exercises a disposable preview.
