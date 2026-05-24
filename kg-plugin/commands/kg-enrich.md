---
description: Orchestrates LLM enrichment over a kg knowledge graph. Reads structural data extracted by tree-sitter, dispatches batched file-summarizer agents (5 parallel) to add per-decl summaries + semantic edges, then runs architecture-analyzer and tour-builder. Outputs a summary report. Use when the user wants to enrich an already-extracted kg.db.
argument-hint: [--domain <id>] [--source <id>] [--max-files <N>]
allowed-tools: Read, Bash, Task, AskUserQuestion
---

# /kg-enrich

You orchestrate three LLM subagents to enrich a kg knowledge graph with summaries, layers, and tours.

## Arguments

Parse `$ARGUMENTS`:
- `--domain <id>`: target domain. If omitted, auto-detect (see Pre-check below).
- `--source <id>`: structural source to enrich over. Default: `tree-sitter:0.2.0`.
- `--max-files <N>`: cap files processed (cost guard). Default: unlimited.

If multiple domains exist and `--domain` is missing, ask the user via `AskUserQuestion` which one to enrich.

## Pre-check

Run these in sequence; abort with a clear error if any fail.

### 1. Locate the kg CLI

Try each candidate in priority order; first executable wins. Run this bash block via the Bash tool:

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

- **If output is `NOT_FOUND`:** read the expected version from the plugin manifest:

  ```bash
  jq -r '.version' "${CLAUDE_PLUGIN_ROOT}/.claude-plugin/plugin.json"
  ```

  Dispatch `AskUserQuestion`:
  - **question:** `kg CLI is not installed. Download v<VERSION> from github.com/ggfarmco/kg/releases? (~10MB, verified by SHA-256)`
  - **options:** `Yes, install`, `No, abort`

  - On `Yes`: run `bash "${CLAUDE_PLUGIN_ROOT}/scripts/bootstrap.sh"` via Bash. If exit ≠ 0: surface the bootstrap error and abort. If exit 0: re-execute the locate bash block from Step 1 above (one retry). If `KG_BIN` is still empty after the retry, abort with "bootstrap succeeded but kg binary still not found at expected locations — file an issue at https://github.com/ggfarmco/kg/issues."
  - On `No`: abort with `kg CLI required. Manual install: see https://github.com/ggfarmco/kg#install`.

- **If output is a path:** prepend its directory to `PATH` so bundled scripts find `kg`:

  ```bash
  export PATH="$(dirname "$KG_BIN"):$PATH"
  ```

### 2. Check installed version matches plugin's expected version (managed installs only)

Only when `$KG_BIN` is under `${KG_HOME:-$HOME/.config/kg}/bin/`. Skip this check otherwise (developer builds are exempt — don't pester them).

```bash
INSTALL_ROOT="${KG_HOME:-$HOME/.config/kg}"
INSTALL_ROOT="${INSTALL_ROOT%/}"
case "$KG_BIN" in
  "$INSTALL_ROOT/bin/kg")
    EXPECTED="v$(jq -r '.version' "${CLAUDE_PLUGIN_ROOT}/.claude-plugin/plugin.json")"
    INSTALLED="$(cat "$INSTALL_ROOT/VERSION" 2>/dev/null || echo unknown)"
    [ "$EXPECTED" = "$INSTALLED" ] || echo "VERSION_MISMATCH expected=$EXPECTED installed=$INSTALLED"
    ;;
esac
```

If output contains `VERSION_MISMATCH`: dispatch `AskUserQuestion`:
- **question:** `Installed kg is <INSTALLED>; plugin needs <EXPECTED>. Upgrade?`
- **options:** `Yes, upgrade`, `No, use current`
- On `Yes`: run `bash "${CLAUDE_PLUGIN_ROOT}/scripts/bootstrap.sh"` (it will overwrite to the new version).
- On `No`: continue with the current binary; warn user that some features may not work.

### 3. Locate or create kg.db

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

### 4. Check graph has structural data; offer auto-extract if empty

```bash
DOMAIN_COUNT=$(kg --db "$KG_DB" domain list 2>/dev/null | jq -r '.data | length' || echo 0)
echo "DOMAIN_COUNT=$DOMAIN_COUNT"
```

If `DOMAIN_COUNT=0`: dispatch `AskUserQuestion`:
- **question:** `kg.db has no domains yet. Run kg-extractor against the current directory now? (default domain: $(basename "$(pwd)"), plugin: tree-sitter, language: go)`
- **options:**
  - `Yes — extract Go code`
  - `Yes — but pick language` (then second AskUserQuestion: `go`, `typescript`, `python`)
  - `No, I'll extract manually` (abort with: `Run: kg-extractor extract --plugin tree-sitter --language <lang> --input . --domain <id> --db "$KG_DB" --kg-binary "$KG_BIN"`)

On `Yes — extract Go code`:
```bash
DOMAIN="$(basename "$(pwd)")"
kg-extractor extract \
  --plugin tree-sitter --language go \
  --input "$(pwd)" --domain "$DOMAIN" \
  --db "$KG_DB" --kg-binary "$KG_BIN"
```

(If user pre-specified `--domain` to `/kg-enrich`, use that value instead of basename.)

If exit ≠ 0: surface the extractor error and abort. If exit 0: continue with enrichment workflow (the next step "Detect domain" now finds the freshly-extracted domain).

### 5. Detect domain (if --domain omitted)

```bash
kg domain list
```

If exactly one domain: use it. If multiple: ask the user via AskUserQuestion. If zero: tell the user to extract first.

### 6. Source has nodes in that domain

```bash
kg node list --domain "<domain>" --layer file --source "<source>" --limit 1
```

On empty: "Source '<source>' has no file nodes in domain '<domain>'. Did you run kg-extractor? Or did you mean a different --source?"

### 7. Create scratch dir

```bash
mkdir -p .kg-enrich-tmp
```

## Phase 1 — Dump file list

```bash
bash "${CLAUDE_PLUGIN_ROOT}/scripts/dump-files.sh" \
  "<domain>" "<source>" > .kg-enrich-tmp/files.json
```

Inspect `.kg-enrich-tmp/files.json`. Count entries. If `--max-files N` was passed, truncate the list to N before batching.

## Phase 2 — Batch

Split `files.json` into batches of ~25 (configurable; adjust upward to 30 for tiny files, downward to 15 for files with many decls).

For each batch N:

1. Write `.kg-enrich-tmp/batch-N-files.json` (the batch's slice of `files.json`).
2. Run `dump-batch-context.sh` to enrich with per-file decl info:
   ```bash
   bash "${CLAUDE_PLUGIN_ROOT}/scripts/dump-batch-context.sh" \
     ".kg-enrich-tmp/batch-N-files.json" "<source>" \
     > ".kg-enrich-tmp/batch-N-input.json"
   ```

## Phase 3 — Dispatch file-summarizer (5 parallel)

For each wave of up to 5 batches, dispatch concurrently. Use a **single message** with multiple Task tool invocations (this is required to get parallel execution — sequential messages run serially).

Each dispatch:

```
Task(
  subagent_type="file-summarizer",
  description="Enrich batch N",
  prompt=<<contents of .kg-enrich-tmp/batch-N-input.json plus a one-line preamble: "You are batch N of M. Process every file in this batch. Pipe your snapshot to: bash ${CLAUDE_PLUGIN_ROOT}/scripts/apply-snapshot.sh kg-summary:0.1.0 <domain> additive">>
)
```

Collect results. Track `succeeded[]` and `failed[]` lists of batch IDs.

**Failure handling:** if an agent returns `{"status": "failed", "reason": ...}`, log it and continue. Do not retry within this phase — the user gets a chance to retry from the summary report.

## Phase 4 — Dispatch architecture-analyzer

Generate graph shape input:

```bash
bash "${CLAUDE_PLUGIN_ROOT}/scripts/dump-graph-shape.sh" \
  "<domain>" "<source>" > .kg-enrich-tmp/graph-shape.json
```

Dispatch single agent:

```
Task(
  subagent_type="architecture-analyzer",
  description="Infer architectural layers for <domain>",
  prompt='{"domain": "<domain>", "structural_source": "<source>", "graph_shape_path": "<abs-path-to>/.kg-enrich-tmp/graph-shape.json"}'
)
```

If it fails: record the failure but DO NOT abort. Tour-builder can run without arch.

## Phase 5 — Dispatch tour-builder

Generate topology:

```bash
bash "${CLAUDE_PLUGIN_ROOT}/scripts/dump-topology.sh" \
  "<domain>" "<source>" > .kg-enrich-tmp/topology.json
```

Dispatch:

```
Task(
  subagent_type="tour-builder",
  description="Build onboarding tour for <domain>",
  prompt='{"domain": "<domain>", "structural_source": "<source>", "arch_domain": "<domain>-arch", "topology_path": "<abs-path-to>/.kg-enrich-tmp/topology.json"}'
)
```

(If architecture-analyzer failed, omit `arch_domain` from the prompt — tour-builder degrades gracefully.)

## Phase 6 — Summary report

Compute and print:

```bash
nodes_enriched=$(kg node list --domain "<domain>" --limit 0 \
  | jq '[.data[] | select(.properties["kg-summary:0.1.0"] != null)] | length')
semantic_edges=$(kg export --domain "<domain>" --source kg-summary:0.1.0 | jq '.edges | length')
arch_layers=$(kg node list --domain "<domain>-arch" --source kg-arch:0.1.0 --limit 0 2>/dev/null | jq '.data | length // 0')
tour_steps=$(kg node list --domain "<domain>-tours" --source kg-tours:0.1.0 --limit 0 2>/dev/null | jq '.data | length // 0')
```

Print to user:

```
/kg-enrich complete for domain <domain>:
  ✓ file-summarizer: <succeeded.length>/<batch_count> batches
  ✓ architecture-analyzer: <ok|failed>
  ✓ tour-builder: <ok|failed>

Graph deltas:
  nodes enriched (kg-summary:0.1.0): <nodes_enriched>
  semantic edges added: <semantic_edges>
  arch layers (<domain>-arch): <arch_layers>
  tour steps (<domain>-tours): <tour_steps>

Failures: <list of failed batch IDs with reasons, or "none">

Next steps:
- /kg-onboard --domain <domain> — generate docs/ONBOARDING.md
- /kg-explain <node-id> — ask Claude about a specific node
- /kg-tour --domain <domain> — re-run tour-builder only
```

If there were failures: prompt the user via AskUserQuestion: "Retry N failed batches?" If yes, re-dispatch only those.

## Cleanup

Leave `.kg-enrich-tmp/` in place — it's useful for debugging. Document it in the user-facing summary: "Intermediate files in .kg-enrich-tmp/ (safe to delete)."

## Idempotency

Re-running `/kg-enrich` overwrites all property/edge contributions in this source's namespace. Tree-sitter's data is untouched (different source ID, different namespace).
