---
description: Re-runs only the tour-builder agent against an already-enriched kg graph. Use when the user wants to regenerate /kg-onboard's source material without re-running file-summarizer or architecture-analyzer. Faster + cheaper than /kg-enrich.
argument-hint: [--domain <id>] [--source <id>] [--arch-domain <id>]
allowed-tools: Read, Bash, Task, AskUserQuestion
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

If output is `NOT_FOUND`: read the expected version from the plugin manifest:

```bash
jq -r '.version' "${CLAUDE_PLUGIN_ROOT}/.claude-plugin/plugin.json"
```

Dispatch `AskUserQuestion`:
- **question:** `kg CLI is not installed. Download v<VERSION> from github.com/ggfarmco/kg/releases? (~10MB, verified by SHA-256)`
- **options:** `Yes, install`, `No, abort`

- On `Yes`: run `bash "${CLAUDE_PLUGIN_ROOT}/scripts/bootstrap.sh"` via Bash. If exit ≠ 0: surface the bootstrap error and abort. If exit 0: re-execute the locate bash block above (one retry). If `KG_BIN` is still empty, abort with "bootstrap succeeded but kg binary still not found — file an issue at https://github.com/ggfarmco/kg/issues."
- On `No`: abort with `kg CLI required. Manual install: see https://github.com/ggfarmco/kg#install`.

If found: `export PATH="$(dirname "$KG_BIN"):$PATH"`.

### 2. Locate or create kg.db

Look in priority order (env override → repo-local → global):

```bash
KG_DB_FOUND=""
for c in "${KG_DB:-}" "./kg.db" "${KG_HOME:-$HOME/.config/kg}/kg.db"; do
  if [ -n "$c" ] && [ -f "$c" ]; then KG_DB_FOUND="$c"; break; fi
done
echo "${KG_DB_FOUND:-NOT_FOUND}"
```

- **If output is a path:** `export KG_DB="$KG_DB_FOUND"` and proceed.

- **If output is `NOT_FOUND`:** dispatch `AskUserQuestion`:
  - **question:** `No kg.db found. Where to create it?`
  - **options:**
    - `Local — ./kg.db (this repo only; add to .gitignore)`
    - `Global — ${KG_HOME:-$HOME/.config/kg}/kg.db (shared across all projects)`
    - `Cancel`
  - On `Local`: run via Bash:
    ```bash
    kg --db "$(pwd)/kg.db" init && export KG_DB="$(pwd)/kg.db"
    ```
  - On `Global`: run via Bash:
    ```bash
    GLOBAL_DB="${KG_HOME:-$HOME/.config/kg}/kg.db"
    mkdir -p "$(dirname "$GLOBAL_DB")" && kg --db "$GLOBAL_DB" init && export KG_DB="$GLOBAL_DB"
    ```
  - On `Cancel`: abort with `Cannot proceed without kg.db. See README.md for manual setup.`

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
