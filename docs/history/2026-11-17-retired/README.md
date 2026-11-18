# Retired 2026-11-17

`SPEC.md` and `VISION.md` described the Rust implementation that runs in
production today. On 2026-11-17 they were replaced by
[`PRODUCT.md`](../../../PRODUCT.md), the product contract Phaedrus shaped
(chat:phaedrus/t3479u, t3480u; MIS-1210). They are kept here as history.
The recompiler reads `PRODUCT.md` and `DESIGN.md` only, never this folder.
The current code's story walk (`qa/walk`) still binds to the retired
`SPEC.md` here until the Go recompile replaces that code.
