---
name: kg-enrich
description: Orchestrates LLM enrichment over a kg knowledge graph. Reads structural data extracted by tree-sitter, dispatches batched file-summarizer agents (5 parallel) to add per-decl summaries + semantic edges, then runs architecture-analyzer and tour-builder. Outputs a summary report. Use when the user wants to enrich an already-extracted kg.db.
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

1. **`kg` on PATH:**
   ```bash
   kg --version
   ```
   On failure: tell user "kg CLI not found. Install: cd into the kg repo and run `make install`."

2. **`kg.db` exists:**
   ```bash
   test -f "${KG_DB:-./kg.db}"
   ```
   On failure: "No kg.db in cwd. Run `kg init` first, then extract structural data with `kg-extractor extract ...`."

3. **Detect domain (if --domain omitted):**
   ```bash
   kg domain list
   ```
   If exactly one domain: use it. If multiple: ask the user. If zero: tell the user to extract first.

4. **Source has nodes in that domain:**
   ```bash
   kg node list --domain "<domain>" --layer file --source "<source>" --limit 1
   ```
   On empty: "Source '<source>' has no file nodes in domain '<domain>'. Did you run kg-extractor? Or did you mean a different --source?"

5. **Create scratch dir:**
   ```bash
   mkdir -p .kg-enrich-tmp
   ```

## Phase 1 — Dump file list

```bash
bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/dump-files.sh" \
  "<domain>" "<source>" > .kg-enrich-tmp/files.json
```

Inspect `.kg-enrich-tmp/files.json`. Count entries. If `--max-files N` was passed, truncate the list to N before batching.

## Phase 2 — Batch

Split `files.json` into batches of ~25 (configurable; adjust upward to 30 for tiny files, downward to 15 for files with many decls).

For each batch N:

1. Write `.kg-enrich-tmp/batch-N-files.json` (the batch's slice of `files.json`).
2. Run `dump-batch-context.sh` to enrich with per-file decl info:
   ```bash
   bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/dump-batch-context.sh" \
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
  prompt=<<contents of .kg-enrich-tmp/batch-N-input.json plus a one-line preamble: "You are batch N of M. Process every file in this batch. Pipe your snapshot to: bash ${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/apply-snapshot.sh kg-summary:0.1.0 <domain> additive">>
)
```

Collect results. Track `succeeded[]` and `failed[]` lists of batch IDs.

**Failure handling:** if an agent returns `{"status": "failed", "reason": ...}`, log it and continue. Do not retry within this phase — the user gets a chance to retry from the summary report.

## Phase 4 — Dispatch architecture-analyzer

Generate graph shape input:

```bash
bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/dump-graph-shape.sh" \
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
bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/dump-topology.sh" \
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
