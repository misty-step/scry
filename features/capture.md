# Capture and preparation

Stories: US-004, US-013
Source: internal/web/library.go, internal/web/templates/library.html, internal/generation/*.go, internal/semantic/critic*.go, internal/store/critic*.go, cmd/scry/main.go

## Sub-features

One private text field and optional photo on Add; one goal/source per operation. Legacy explicit Topic/Link callers retain their mode-specific research paths, but the screen never infers or authorizes web research.

## How to get to it (user POV)

Choose Add in the masthead (`/add`), type or paste into `form.add`, optionally attach a photo, then choose Add. The saved material and preparation status appear at `/sources/{id}` and on `/map`. A stopped source shows its reason and accounted use, with Try again at `/sources/{id}/retry` only while its durable retry limit permits. Edit as new input opens `/add?from={id}` with the saved text; adding it creates a new source and leaves the original unchanged. Photos offer Add a new photo. Bounded polling ends with Check again, not a fabricated failure. Share-target `/add?text=&url=&title=` pre-fills private text.

## Driving it

`qa/walk --stories US-013` exercises the one-field Add, saved Source, and privacy boundary on a fresh isolated database. `qa/walk --stories US-005` retains independent tests for historical mode-specific ingestion. `seed-fixture` publishes authored questions without a provider call. Live generation quality requires separate authorized evidence.

## Gotchas

Anything typed or pasted, even a standalone URL, stays My text and is never searched or opened. A selected JPEG/PNG/WebP photo transcribes before planning; unsupported or oversize input creates no source. The existing explicit Topic and Link APIs remain for historical callers. Without a model endpoint, newly captured work stops with a configuration failure, not ready questions. Do not mistake fixture publication for a model-quality pass.

Private text can be a learning request rather than factual material. Its generated items may use labeled General knowledge without quotations or citations; saved capture mode, privacy, and history do not change. Claims attributed to supplied material still require exact evidence. Link/photo and authoritative exact-text/complete-set tasks remain source-grounded.
