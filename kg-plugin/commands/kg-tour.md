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

### 1. Locate the kg CLI

Try each candidate in priority order; first executable wins:

```bash
KG_BIN=""
for c in \
  "$(command -v kg 2>/dev/null || true)" \
  "${KG_HOME:-$HOME/.config/kg}/bin/kg" \
  "${CLAUDE_PLUGIN_ROOT}/../bin/kg" \
  "$(pwd)/bin/kg"; do
  if [ -n "$c" ] && [ -x "$c" ]; then KG_BIN="$c"; break; fi
done
echo "${KG_BIN:-NOT_FOUND}"
```

If `NOT_FOUND`: offer install via `AskUserQuestion` and run `bash "${CLAUDE_PLUGIN_ROOT}/scripts/bootstrap.sh"` on yes. Abort if user declines. (Same flow as `/kg-enrich` Pre-check Step 1.)

If found: `export PATH="$(dirname "$KG_BIN"):$PATH"`.

### 2. `kg.db` exists

```bash
test -f "${KG_DB:-./kg.db}"
```

### 3. Verify the structural source has nodes in `<domain>`

```bash
kg node list --domain "<domain>" --layer file --source "<source>" --limit 1
```

On empty: tell user to run `/kg-enrich` first (or check `--source` argument).

### 4. (Optional) If `<arch-domain>` is non-empty, verify it has at least one `layer` node

```bash
kg node list --domain "<arch-domain>" --layer layer --source kg-arch:0.1.0 --limit 1
```

If empty, warn and continue without arch.

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
