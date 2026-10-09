# Historical critic provenance tests

These unchanged .historical.mjs copies record the former Go candidate lifecycle
and buildinfo batteries. They are excluded from current test discovery and do
not validate the Rust Worker.

For runnable historical code, use the original paths in the retained Git
baseline c6ba395 in a separate isolated checkout. Do not run these copies
against the Rust tree, restore the former Go writer, or relabel its receipts.
lib/provenance.mjs remains a historical format reader; current candidates use
lib/worker-artifact.mjs and a source snapshot plus exact Worker module hashes.
