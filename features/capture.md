# Capture and preparation

Stories: US-004, US-013
Source: internal/web/library.go, internal/web/templates/library.html, internal/generation/*.go, internal/semantic/critic*.go, internal/store/critic*.go, cmd/scry/main.go

## Sub-features

One private text field and optional photo on Add; one goal/source per operation. Legacy explicit Topic/Link callers retain their mode-specific research paths, but the screen never infers or authorizes web research.

## How to get to it (user POV)

Choose Add in the masthead (`/add`), type or paste into `form.add`, optionally attach a photo, then choose Add. The saved material and preparation status appear at `/sources/{id}` and on `/map`. A stopped source has Try again at `/sources/{id}/retry`. Share-target `/add?text=&url=&title=` pre-fills private text.

## Driving it

`qa/walk --stories US-013` exercises the one-field Add, saved Source, and privacy boundary on a fresh isolated database. `qa/walk --stories US-005` retains independent tests for historical mode-specific ingestion. `seed-fixture` publishes authored questions without a provider call. Live generation quality requires separate authorized evidence.

## Gotchas

Anything typed or pasted, even a standalone URL, stays My text and is never searched or opened. A selected JPEG/PNG/WebP photo transcribes before planning; unsupported or oversize input creates no source. The existing explicit Topic and Link APIs remain for historical callers. Without a model endpoint, newly captured work stops with a configuration failure, not ready questions. Do not mistake fixture publication for a model-quality pass.
