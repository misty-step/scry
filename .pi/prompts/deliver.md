---
description: Execute a scoped change through plan -> implement -> review
---
Task: $@

Pipeline routing:
- Default: run `scry-delivery-v2`.
- If task is Convex-heavy (schema/query/mutation/backend contract): run `scry-convex-delivery-v1`.
- If task is Pi/config foundation work: run `scry-foundation-v2`.

If pipeline execution is unavailable in this runtime, run the same sequence manually using:
1) `.pi/agents/planner.md`
2) `.pi/agents/worker.md`
3) `.pi/agents/reviewer.md`

Keep scope tight, run CI-parity checks, and report residual risk explicitly.