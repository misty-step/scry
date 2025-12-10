---
name: langfuse-prompts
description: |
  Create and manage Scry's Langfuse prompts. Use when:
  - Creating or updating prompts in Langfuse
  - Viewing current prompt templates
  - Syncing local prompt templates to Langfuse
---

# Langfuse Prompt Management (Scry)

Scripts and templates for Scry's AI generation prompts.

## Prompts

| Name | Variables | Purpose |
|------|-----------|---------|
| `scry-intent-extraction` | `{{userInput}}` | Classify user input, produce intent object |
| `scry-concept-synthesis` | `{{intentJson}}` | Generate atomic concepts from intent |
| `scry-phrasing-generation` | `{{conceptTitle}}`, `{{contentType}}`, `{{originIntent}}`, `{{targetCount}}`, `{{existingQuestions}}` | Create quiz questions for a concept |

## Commands

```bash
cd .claude/skills/langfuse-prompts

# Create all prompts in Langfuse
npx tsx scripts/create-prompt.ts --all

# Create specific prompt
npx tsx scripts/create-prompt.ts --name scry-intent-extraction

# Create with production label
npx tsx scripts/create-prompt.ts --name scry-intent-extraction --label production
```

## Environment Variables

Requires (from Convex or shell):
- `LANGFUSE_SECRET_KEY`
- `LANGFUSE_PUBLIC_KEY`
- `LANGFUSE_HOST` (e.g., `https://us.cloud.langfuse.com`)

## Workflow

1. Edit templates in `prompts/` directory
2. Run `npx tsx scripts/create-prompt.ts --all` to push to Langfuse
3. Verify via global skill: `npx tsx ~/.claude/skills/langfuse-observability/scripts/list-prompts.ts --name scry-intent-extraction`
