---
description: Re-runs only the tour-builder agent against an already-enriched kg graph. Use when the user wants to regenerate /kg-onboard's source material without re-running file-summarizer or architecture-analyzer. Faster + cheaper than /kg-enrich.
argument-hint: [--domain <id>] [--source <id>] [--arch-domain <id>]
allowed-tools: Read, Bash, Task
---

# /kg-tour

Standalone re-trigger of tour-builder.

## Arguments

- `--domain <id>` (default: auto-detect single domain, else prompt)
- `--source <id>` (structural source; default `tree-sitter:0.2.0`)
- `--arch-domain <id>` (default: `<domain>-arch`; pass empty to skip)

## Pre-check

Same first 4 checks as /kg-enrich. Plus:
- Verify the structural source has nodes in `<domain>`.
- If `<arch-domain>` is non-empty, verify it has at least one `layer` node. If not, warn and continue without arch.

## Workflow

1. **Generate topology:**
   ```bash
   mkdir -p .kg-enrich-tmp
   bash "${CLAUDE_PLUGIN_ROOT}/scripts/dump-topology.sh" \
     "<domain>" "<source>" > .kg-enrich-tmp/topology.json
   ```

2. **Dispatch tour-builder:**
   Same Task dispatch as /kg-enrich Phase 5.

3. **Report:**
   ```bash
   kg node list --domain "<domain>-tours" --source kg-tours:0.1.0
   ```
   Print step count and the first 3 step names + descriptions as a preview.

## Idempotency

`scope: domain-source` ensures the previous tour is cleanly replaced. The previous step IDs disappear; the new ones may not match.

## Non-goals

- Don't touch summaries (file-summarizer's output).
- Don't touch architecture (architecture-analyzer's output).
- Don't generate ONBOARDING.md — that's /kg-onboard.
