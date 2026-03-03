---
description: Close PR feedback loop with strict triage, fixes, and structured replies
---
Task: $@

Role: execute PR feedback closure with minimal churn and explicit evidence.

Workflow:
1. Discover PR context for current branch.
2. Fetch fresh feedback from GitHub:
   - review comments
   - review summaries
   - issue/PR comments
3. Triage each actionable item:
   - Classification: bug | risk | style | question
   - Severity: critical | high | medium | low
   - Decision: fix now | defer | reject (+ reason)
4. Policy:
   - critical/high: fix now unless explicitly blocked
   - medium: fix now or defer with rationale
   - low: optional
5. Implement smallest correct fixes.
6. Verify only what changed (plus CI-parity checks as needed).
7. Commit before replying.
8. Post replies via temp files + `--body-file/-F`.

Reply format (exact):
- Classification: <...>
- Severity: <...>
- Decision: <...>
- Change: <...>
- Verification: <...>

Finish with one PR-level closure comment:
- fixed now
- deferred
- rejected
- verification snapshot
- residual risk
