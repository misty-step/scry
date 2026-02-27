# Bootstrap Flow Postmortem (Meta Slice)

Date: 2026-02-26
Repo: `/Users/phaedrus/Development/scry`
Scope: Diagnose why bootstrap was not fully "works out of the box" after running the meta-slice bootstrap in this repo.

## Executive summary
Bootstrap successfully created and activated most repo-local Pi foundations (`.pi/settings.json`, local agents, prompts, teams/pipelines, persona, report). However, two defects in the broader bootstrap/orchestration pipeline caused immediate quality regressions:

1. **Generated pipeline prompts are not parsed correctly by orchestration** when they use YAML block scalars (`prompt: |`).
2. **`AGENTS.md` is overwritten even without `--force`, and fallback AGENTS template writes literal `\\n`** instead of real newlines.

These are not user/operator errors; they are system-level integration defects.

---

## What worked (validated)
- Repo-local `.pi/` scaffolding exists and is discoverable.
- Project agents are runnable (`subagent` with `planner` succeeded).
- Project teams are runnable (`team_run foundation` succeeded).
- Repo prompt templates are present (`discover/design/deliver/review`).

So local customizations are being loaded and used.

---

## Incident A — Pipeline steps receive effectively empty task (`Task: |`)

### Symptoms
- `pipeline_run` on local pipelines (`scry-delivery-v2`, `scry-foundation-v2`) produced agent replies indicating empty/incomplete task.
- Example behavior: planner says task body is empty / asks for task details.

### Evidence chain
1. Generated `.pi/agents/pipelines.yaml` uses YAML block scalar style for prompts:
   - `prompt: |`
   - followed by multi-line prompt body.
2. Orchestration parser (`~/.pi/agent/extensions/orchestration/config.ts`, `parsePipelinesYaml`) only parses prompt values via single-line regex:
   - `const promptMatch = line.match(/^\s{6}prompt:\s*(.+)$/)`
3. For `prompt: |`, parser captures `|` as the prompt value; multi-line body is not consumed.
4. Runtime task assembly (`~/.pi/agent/extensions/orchestration/index.ts`) passes this as:
   - `args.push(`Task: ${options.task}`)`
   - therefore delegated agent receives `Task: |` (no meaningful task payload).

### Root cause
**Format contract mismatch** between bootstrap output (block-scalar YAML prompts) and orchestration parser capabilities (single-line prompt parsing only).

### Impact
- Local pipelines appear available but are operationally degraded.
- `/deliver` route that depends on pipelines is unreliable/non-actionable.

---

## Incident B — `AGENTS.md` overwritten and malformed

### Symptoms
- Repo `AGENTS.md` changed unexpectedly after bootstrap.
- File content contains literal escaped newlines (`\n`) instead of actual line breaks.

### Evidence chain
1. Bootstrap writer marks AGENTS as persona-managed and overwrite-allowed even without `--force`:
   - `personaManagedPath = relativePath === ".pi/persona.md" || relativePath === "AGENTS.md"`
   - overwrite allowed when `personaManagedPath` is true.
2. Fallback file set includes `AGENTS.md` (`"AGENTS.md": agentsTemplate(facts)`).
3. `agentsTemplate` uses `.join("\\n")` (escaped slash-n), producing literal `\n` characters in output.

### Root cause
Two combined issues:
- **Policy issue:** AGENTS is implicitly overwritten by bootstrap without explicit force.
- **String assembly defect:** fallback AGENTS template joins with escaped newline string.

### Impact
- Existing repository guidance in `AGENTS.md` is replaced unexpectedly.
- Resulting AGENTS content quality is degraded (formatting broken), reducing context signal.

---

## Why this escaped bootstrap quality gates

1. **No end-to-end contract test** from bootstrap output -> orchestration parser execution for multi-line pipeline prompts.
2. **No explicit regression test** for AGENTS output formatting (real newline vs escaped literal).
3. **Policy/docs drift**: behavior in code (AGENTS overwritten as persona-managed) conflicts with user expectation of non-force preservation.

---

## Reproduction (minimal)

1. Run bootstrap (meta slice) in repo.
2. Verify files exist:
   - `.pi/agents/pipelines.yaml`
   - `AGENTS.md`
3. Run pipeline sanity check:
   - `pipeline_run pipeline=scry-foundation-v2 goal="Read .pi/persona.md and return objective line only."`
4. Observe planner/reviewer responses indicating missing/empty task.
5. Inspect AGENTS formatting:
   - contains literal `\n` tokens.

---

## Corrective actions (deferred; not applied in this session)

1. **Bootstrap/orchestration compatibility fix (must-have):**
   - Either teach orchestration parser to support YAML block scalars for `prompt: |`,
   - or make bootstrap emit single-line/quoted prompts with explicit `\n` escapes compatible with current parser.
2. **AGENTS overwrite policy hardening:**
   - Do not overwrite existing `AGENTS.md` unless explicit `--force` (or explicit `--refresh-agents`).
3. **AGENTS newline bug fix:**
   - Change fallback AGENTS template join to real newline join.
4. **Integration tests:**
   - Bootstrap output -> parse -> run pipeline with non-empty goal and assert first step receives goal text.
   - Bootstrap AGENTS generation preserves newline formatting.
   - Existing AGENTS preservation behavior verified by default mode.

---

## Current status
- Issues documented only.
- No configuration/code fixes applied in this session by request.
