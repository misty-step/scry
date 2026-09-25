# Capture and preparation

Stories: US-004, US-005
Source: internal/web/library.go, internal/web/templates/library.html, internal/generation/*.go, internal/semantic/critic*.go, internal/store/critic*.go, cmd/scry/main.go

## Sub-features

Explicit Topic, My text, Link, or Photo capture; one goal/source per operation; saved preparation, provider-bound generation and prepublication critic with retained attempts and shared spend allowance.

## How to get to it (user POV)

Choose Add in the masthead (`/add`), select `input[name=mode]`, submit `form.capture-form`, then inspect `/sources/{id}` and the preparation receipt on `/map`. A failed source has a Try again action via `/sources/{id}/retry`. Share-target `/add?text=&url=&title=` pre-fills input but never chooses a research mode.

## Driving it

`qa/walk --stories "US-004 US-005"` exercises explicit mode, a My text capture, saved Source and Map preparation state on a fresh isolated database. `seed-fixture` provides authored, already published candidates with no provider call. Focused `go test -p 1 ./internal/generation ./internal/store ./internal/semantic ./internal/learning` covers critical policy/storage branches that a credential-free browser walk cannot observe. Live Exa/Jev/model usefulness requires separate authorization and evidence.

## Gotchas

Topic alone searches Exa when configured; My text must never be sent to web search. Link must fail recoverably if unreadable, not plan from a URL. A Photo transcribes before planning; unsupported or oversize input creates no source. With no model endpoint, newly captured work stops with a configuration failure, not ready questions. Criticism is skipped without its endpoint, and an unknown paid send retains its reservation. Do not treat fixture-published material as a model-quality pass.
