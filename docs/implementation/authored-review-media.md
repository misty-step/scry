# Bounded externally authored review media (#404)

This is a source design and verification plan, not deployment acceptance.
[US-022](../../SPEC.md#us-022-review-externally-authored-image-and-audio-cards)
owns behavior. This does not revive Materials #399/#401 or the historical Goal
photo-to-generation flow. Hosted bearer ingress #335 remains separate.

## Authoring and immutable identity

`POST /api/v1/library/media` accepts `{operation_id,data}`. `data` contains a
normal `mapping` manifest plus `attachments`. Each attachment contains:

- zero-based `unit_index`, `side: prompt|answer`;
- `file_name`, `mime`, lowercase SHA-256 `sha256`, canonical `bytes_base64`;
- `alt` for PNG or `transcript` for audio (the other field is empty);
- `source`, `rights: public_domain|licensed|private_only`, and `license` text.

All source/license values are assertions supplied by the author, displayed as
text, not verified rights or fetch instructions. Sources are never fetched.
No URL, filesystem path, generation instruction or expected grade is accepted
in place of media bytes. The CLI reads only the explicitly named manifest:

```sh
scry media add --file supplied-media.json --operation-id YOUR_STABLE_ID --json
scry media supersede VERSION --revision WORK_REVISION \
  --file supplied-media.json --operation-id YOUR_STABLE_ID --json
```

`POST /api/v1/library/media/supersede` also requires `version_id` and the current
Work `revision`. Same pairs with different media require this explicit action;
add refuses instead of ignoring attachments or editing an existing Version.
Identical wording + ordered attachment identity deduplicates on add. Operation
replay returns the original receipt; a changed payload conflicts. The new
Version has its own `media_hash`; historical normalized text hashes and exact
`key<TAB>value<LF>` bytes are unchanged. Old Studies, Cards, Attempts and their
schedules remain pinned to the old Version. Learn the new Version deliberately;
no mastery or schedule migration is inferred. Updating or removing media in
place is unsupported. This slice requires at least one attachment.

The initial stdio MCP surface continues to advertise `multimedia:false` until
an explicit tested extension; API/CLI support does not imply MCP support.

## Precisely bounded formats

- At most 64 attachments; one of each MIME per Unit/side; at most 1 MiB per asset.
- At most **4 MiB for the entire JSON HTTP request**, including base64 and all
  metadata. This is not a 4 MiB decoded-file allowance. Base64 overhead is 4/3.
- PNG only: signature, all chunk lengths/CRC32, IHDR/IDAT/IEND order and exact
  final byte; 8-bit grayscale/RGB/gray-alpha/RGBA, non-interlaced, maximum
  2048×2048. No palettes, ancillary metadata, animation or unknown chunks.
  The browser decodes image pixels; this is not a general image transcoder.
- WAV only: exact RIFF/WAVE, canonical 16-byte PCM fmt and data chunks, mono or
  stereo signed 16-bit PCM at 8–48 kHz; consistent byte rate/block alignment,
  exact data size and no extra metadata; duration at most 30 seconds.
- No JPEG/WebP/MP3/OGG/video/SVG, recording, speech recognition, TTS or providers.
  Only formats exercised by retained fixtures may be claimed as supported.

These intentionally narrow envelopes permit validation without new dependencies.
The fixtures are an 8px square and 1-second tone, not teaching material or speech.

## One durable admission, private content-addressed bytes

The runtime first applies the whole request to an uncommitted App clone and
checks Library/reference validity, every byte/metadata/hash, row capacity and
complete encoded archive capacity. A refusal here writes neither rows nor R2.
It then puts each unique blob at `media/<space>/<sha256>` and verifies exact
readback. After async puts, it rechecks token validity and browser CSRF, reloads
the latest App, and reruns the same operation and capacity checks before the
existing revision-fenced SQLite commit. Thus a concurrent edit conflicts or
reconciles a committed same-operation receipt, never writes partial Versions.

A crash after a put but before the SQLite commit can leave unreferenced private
blobs. They are not Library records, disclosed media or backup dependencies.
Content-addressed keys make same-byte retries reuse the blob; each failed import
is bounded by the request limit. There is deliberately no automatic deletion:
a concurrent request could reference the same blob. Unreferenced object storage
must be inventoried against current live references before any separately scoped
maintenance; this slice neither promises zero orphan storage nor adds a second
writer/queue/cleanup state machine. No production bucket is touched by fixtures.

## Honest disclosure and delivery

Current review responses include only attachments permitted for that Prompt.
Under a protected Prompt, only explicitly prompt-side assets for its own Units
can appear. All answer-side bytes, URLs, names, alt/transcript, source/rights and
hashes are absent until the existing durable Reveal or Answer commits. Library,
history and complete export retain their protected-reference fences.

Private `GET /api/v1/media/VERSION/INDEX` (Owner API) and `/media/VERSION/INDEX`
(browser) verify reference, size, hash and format; after R2 awaits they recheck
the latest Prompt and token revocation before returning bytes. Missing/corrupt
assets fail closed. Responses are no-store and nosniff; browser audio is
same-origin through the existing CSP, native controls, `preload=none`, no autoplay.
The source and transcript use native details. Navigation/private-DOM concealment
pauses and discards the outgoing audio source. Reading/playing bytes creates no
Attempt, native success, schedule transition or generation event.

## Recovery and rollback compatibility

Library-only export contains metadata/references, not a restorable byte bundle.
Use complete export for recovery. The existing complete archive stays capped at 16 MiB, counting every distinct
referenced media asset's base64 and metadata before loading bodies. It stores
all media bytes and rejects missing, duplicate, unexpected or corrupt assets.
Restore verifies every byte, writes/reads back the target's scoped R2 keys, and
commits only into an unused learning space. No historical keys are rewritten.

Empty-media archives/rows retain their prior exact shapes. Once media or its
receipts exist, `authored_media_format/v1` is an explicit SQL marker unknown to
older readers, and the complete archive uses
`scry-rust-durable-sqlite-authored-media-v1`. Older readers must refuse both,
not silently ignore new nested Version fields. Rollback after media admission
requires a media-compatible artifact preserving all acknowledged writes;
pre-media artifacts are not a rollback floor. No stopped historical candidate
is adopted as a floor.

## Verification and remaining evidence

`tests/authored_media.rs` covers pure format/metadata validation, immutable
supersession, privacy, native grading, receipt replay and complete archives.
`qa/authored-media.test.mjs` owns built Worker/compiled CLI/SQLite/R2/browser,
interrupted admission, corruption and fresh-restore observations.
`qa/walk --stories "US-022"` runs that exact journey and maps its retained results
to all six criteria; the protected `--all` gate includes it and preserves its
screenshots and logs. Actual predecessor refusal is mandatory, with the existing
checksum-pinned published predecessor used unless a frozen local development
predecessor is supplied explicitly. Exact source,
artifact, command results and predecessor compatibility belong in #404 and its
PR; existence of these tests is not a PASS. Native/target/aggregate gates and
independent review remain required. Hosted ingress, actual phone, useful authored
content and independent remote recovery are not established by synthetic tests.
