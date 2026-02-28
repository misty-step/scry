# Bun Migration Spike (Slice scaffold for #272)

Issue: #272 (parent #270)

## Objective

Produce an evidence-backed Bun parity matrix and a go/no-go recommendation without changing production defaults.

## Spike rules

- Keep `pnpm` as current default during spike.
- Do not modify deployment scripts in this slice.
- Record every incompatibility with exact command and failure note.

## Matrix areas

| Area | pnpm baseline | bun target |
|---|---|---|
| Install | `pnpm install` | `bun install` |
| Dev startup | `pnpm dev` | `PORT=<PARITY_PORT> bun run dev` |
| Lint | `pnpm lint` | `bun run lint` |
| Typecheck | `pnpm tsc --noEmit` | `bun run tsc --noEmit` |
| Tests | `pnpm test:ci` | `bun run test:ci` |
| QA smoke | `pnpm qa:dogfood:local` | `bun run qa:dogfood -- --url http://localhost:<PARITY_PORT>` |

## Runner scaffold

Use:

```bash
pnpm spike:bun:matrix
```

This writes a report at:

- `/tmp/bun-parity-<timestamp>.md`

## Deliverables for #272

1. Matrix report (pass/fail for each row).
2. Blocker list with remediation notes.
3. Go/no-go recommendation with rollback implications.

## Exit criteria

- **Go** if all required rows pass and CI workflow impact is understood.
- **No-go** if any required row fails without clear near-term remediation.

If **Go**, open/advance follow-up cutover work to update CI workflows and contributor guidance before changing package-manager defaults.
