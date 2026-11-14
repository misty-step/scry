# Notebooks before paid judgment

`judgment-off.json` was written by the builders in `tests/judgment_off.rs`,
run on unmodified source `701724cf67087a693f19337ddad8fad6face11b2`: the last
source with no paid judgment of memorization answers. It contains no production
data, credentials or external-provider output; every response in it is a
synthetic literal from that test file.

Each notebook is built by native `engine::mutate`, `engine::mutate_as` and, for
the earlier concept flow, `engine::claim_work` and `engine::finish_work`:

- `empty`: a new notebook.
- `text`: a text lesson of four phrases with a sibling Version. Exact answers,
  a Slip, mismatches another judge could accept, an empty answer, a guided
  retry, a reveal, a joined passage and whole-text Recitations.
- `list`: the Joyful Mysteries. An omission, a duplicate, a swap, an unlisted
  naming of one item and a complete answer.
- `mapping`: a mapping that is not Spelling-strict. An unlisted wording, an
  exact answer, a negated practice and a paraphrase.
- `nato`: the Spelling-strict NATO starter, answered through an Owner token
  without paid permission, ending on a saved draft.
- `nato-result`: the same starter in the browser, ending on a held miss.
- `concept-accepted`, `concept-rejected`, `concept-unsure`: the earlier concept
  notebook's Jev check, which shares the response parser: an unreadable
  response then an accepted retry, an incomplete response then a rejected
  retry, and an inconclusive response.

For each notebook the fixture holds what that source produced: the archive
`format`, the `sha256` and length in `bytes` of the complete encoded recovery
archive, every stored row (`rows`), the practice response for each Attempt
(`wire`) and the rendered `/recall`, `/history` and `/settings` pages.

## What the regression proves

`off_is_byte_identical_to_the_source_before_paid_judgment` rebuilds every
notebook with the current source through the same entry points, which pass no
Jev capability, and requires each of those values to be identical. With paid
judgment off, the stored bytes, the archive, its format name, the responses and
the result and history screens are what they were.

To rewrite the fixture, check out the source named above, copy
`tests/judgment_off.rs` into it and run
`SCRY_WRITE_JUDGMENT_OFF_FIXTURE=1 cargo test --locked --test judgment_off`
with a `CARGO_TARGET_DIR` of its own. Two checkouts that share a target
directory share compiled artifacts, so the test would link the later library
and the fixture would prove nothing.

The synthetic Owner token's identifier appears as `TOKEN_ID` in the readable
parts (`rows`, `wire`, `pages`): the test masks it on both sides of the
comparison so the fixture holds nothing shaped like a credential. `sha256` and
`bytes` are of the unmasked archive.
