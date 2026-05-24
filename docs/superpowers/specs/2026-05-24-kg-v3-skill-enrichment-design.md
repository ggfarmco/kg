# kg v3 — Skill-Driven LLM Enrichment Design Spec

**Date:** 2026-05-24
**Status:** Approved for implementation planning
**Builds on:** `2026-05-23-kg-mvp-design.md` (v0), `2026-05-23-kg-v1-extractor-design.md` (v1), `2026-05-24-kg-v2-provenance-design.md` (v2)

## Background

v0–v2 shipped a generic provenance-aware graph engine (`kg` CLI), an extractor system (`kg-extractor` + tree-sitter Go plugin), and a declarative apply pipeline (`kg apply` consumes JSON snapshots; multi-source coexistence via namespaced properties and ref-counted edge claims).

The end-state after v2 is an empty room with great plumbing: kg knows how to ingest structural data from multiple writers without conflict, but nothing produces semantic data. The graph today has nodes for every package/file/decl and edges for imports/calls, but no summaries, no architectural understanding, no learning paths. Pure skeleton.

v3 adds the meat — LLM-driven semantic enrichment — as a **Claude Code plugin** shipped alongside the kg engine. The plugin orchestrates Claude to read source files, generate per-decl summaries and semantic edges, infer architectural layers, and build pedagogical tours. The architecture mirrors [Understand-Anything](../../../../../Understand-Anything/)'s skill+agent+bundled-script pattern, adapted to kg's namespace-aware engine.

### Why skill-driven instead of a Go binary

A Go `cmd/kg-enricher` would have to: pin a Claude SDK, manage API keys, implement batching/retry/concurrency, version prompts in Go code, ship as a separate binary users install. All real work, none of it adding leverage to kg's core value.

A Claude Code plugin instead: the Claude Code runtime already handles LLM calls, key management, subagent dispatch with bounded concurrency. We write markdown (skills + agents) and small bundled bash scripts. The plugin is `git pull`-installed via `/plugin marketplace add`. Prompts are diff-friendly. Works from any Claude Code session (terminal, IDE, web).

Trade-off: skill-driven means LLM enrichment is unavailable in CI/headless contexts. We accept this in v3; a Go binary can be added later if real demand emerges.

### What v2 made possible

v2's multi-source design was built specifically with this v3 in mind:

- Tree-sitter writes under `tree-sitter:0.2.0`; LLM agents write under their own source IDs (`kg-summary:0.1.0`, `kg-arch:0.1.0`, `kg-tours:0.1.0`). Properties live in disjoint namespaces. No conflict, no merge logic needed.
- Semantic edges added by LLM agents become **claims** alongside any structural edges tree-sitter has on the same `(src, tgt, type)` triple. Multi-claim refcounting means semantic and structural pieces of evidence coexist without one displacing the other.
- `kg apply` with `scope: "additive"` cleanly expresses "I'm adding to the graph, not replacing anything." No engine code re-extracts handle this — it's a first-class wire-protocol option.

The headline v2 design statement was: "the infrastructure makes v3-enrichment a pure plugin concern, no engine rewrites needed." v3 puts that to the test, with one small caveat (see [Engine changes](#engine-changes)).

## Goals

1. **LLM enrichment of tree-sitter output.** Every file/decl node gets a `summary`, `tags`, and `complexity` rating under `kg-summary:0.1.0` namespace.
2. **Semantic edges.** LLM emits cross-decl relations (`depends_on`, `implements`, `exposes`, `documented_in`, etc.) as claims under `kg-summary:0.1.0`.
3. **Architectural layer inference.** `architecture-analyzer` agent creates a new domain `<orig>-arch` with layer-nodes and cross-domain `contains` edges to original-domain file nodes.
4. **Pedagogical tour generation.** `tour-builder` agent creates `<orig>-tours` domain with ordered step-nodes and cross-domain `teaches` edges.
5. **Onboarding doc generation.** `/kg-onboard` skill produces `docs/ONBOARDING.md` from the enriched graph.
6. **Single-node explanation.** `/kg-explain <id>` answers questions about a specific node + 1-hop context, no graph mutation.
7. **Multi-source coexistence proven end-to-end.** After running `/kg-enrich`, the graph holds disjoint contributions from tree-sitter and three LLM agents, with kg's `--merged` view returning a unified picture.

## Non-Goals (deferred)

- **Dashboard.** Web UI for interactive graph exploration. Punted to v4 (fork UA's React/Vite dashboard + write a kg-namespaced → flat-shape converter).
- **More languages in tree-sitter.** v3 enriches whatever tree-sitter has put in the graph. Currently only Go. Adding Python/TS/Rust is a v5+ concern (one `languages/<lang>/` per).
- **Multi-environment plugin packaging.** v3 ships as Claude Code plugin only. Codex/Cursor/Gemini/Copilot variants follow the same skill+agent files but require platform-specific manifests, deferred.
- **Domain analyzer.** `/kg-domain` (code → business processes mapping) per Understand-Anything. Deferred to v3.1.
- **Knowledge-base mode.** `/kg-knowledge` for Karpathy-style LLM wikis. Deferred to v3.1.
- **Fingerprint-based incremental update.** Understand-Anything has per-file content+signature fingerprints to skip re-analysis of cosmetic changes. kg's `kg apply` is already idempotent; re-running `/kg-enrich` after small changes is cheap on the engine side. The expensive part is LLM tokens, but we don't try to be clever about it in v3 — explicit `--max-files N` flag and user discretion are enough.
- **Cost tracking / budget guards.** Claude Code session handles auth and any rate limiting. kg has no opinion about cost; user manages it via Claude Code's own controls.
- **Heuristic fallbacks** when LLM unavailable. UA has heuristic layer-detector and tour-generator as backstops. v3 takes the simpler path: if LLM call fails, the agent fails clean and the skill reports an error.
- **`/kg-diff` and `/kg-chat`.** Diff-aware re-enrichment and conversational graph exploration. Deferred to v3.1.

## Architecture

Five new moving parts, all in a single `.claude-plugin/` directory at the kg repo root:

```
User runs /kg-enrich in Claude Code (no args; or [--domain X] [--source Y])
       ↓
SKILL.md (kg-enrich) orchestrates:
       ↓
Phase 1: dump-files.sh → list file-layer nodes from tree-sitter source
       ↓
Phase 2: batch into groups of 20-30 files
       ↓
Phase 3: dispatch 5 parallel file-summarizer agents
         (each agent: read source files → LLM analyzes → emit per-batch snapshot → pipe to kg apply)
       ↓
Phase 4: dispatch architecture-analyzer (single instance, emits to <orig>-arch domain)
       ↓
Phase 5: dispatch tour-builder (single instance, emits to <orig>-tours domain)
       ↓
Phase 6: skill prints summary report
```

Each agent follows the **two-phase deterministic-then-LLM contract** (from UA):
- Phase 1 (deterministic): run a bundled bash script that wraps `kg ... | jq ...` to dump the agent's inputs in structured JSON.
- Phase 2 (LLM): agent reads the structured input + source files (via `Read` tool), synthesizes summaries/tags/edges, emits a snapshot.
- Output: snapshot piped through stdin to `kg apply --source <agent-source> --domain <target> --scope <scope>`.

Skill prompts (markdown) are the controllers. Agents are subagents dispatched via the Task tool with bounded concurrency. Bundled scripts are the deterministic glue between kg CLI and LLM judgment.

### Why per-batch apply (not merge-then-apply)

UA pipes all batch outputs into a Python `merge-batch-graphs.py` script, then writes the single `knowledge-graph.json` file. UA's single-file storage forces this.

kg's namespace + multi-claim model makes per-batch apply safe and faster:
- All 5 file-summarizer agents write under one source `kg-summary:0.1.0`, but each batch covers a disjoint set of file nodes — no property conflict.
- Each `kg apply` is atomic (one `Store.InTx`). 5 parallel applies = 5 short transactions. SQLite WAL handles this fine.
- If one batch fails, the other 4 are already committed. Partial state is acceptable because `/kg-enrich` is idempotent — rerunning just fills in the missing batch.

The merge-then-apply alternative requires a bundled merge script (additional surface to maintain, has to track snapshot schema evolution). Skipped.

## Plugin packaging

```
kg/                                          # existing repo
└── .claude-plugin/                          # NEW
    ├── plugin.json                          # Claude Code plugin manifest
    ├── marketplace.json                     # for /plugin marketplace add
    ├── skills/
    │   ├── kg-enrich/
    │   │   ├── SKILL.md                     # the orchestrator (~2k words)
    │   │   └── scripts/
    │   │       ├── dump-files.sh
    │   │       ├── dump-batch-context.sh
    │   │       ├── dump-graph-shape.sh
    │   │       ├── dump-topology.sh
    │   │       └── apply-snapshot.sh
    │   ├── kg-explain/SKILL.md              # read-only Q&A
    │   ├── kg-tour/SKILL.md                 # standalone tour-builder trigger
    │   └── kg-onboard/SKILL.md              # generate ONBOARDING.md
    └── agents/
        ├── file-summarizer.md               # ~3k words
        ├── architecture-analyzer.md         # ~2k words
        └── tour-builder.md                  # ~2k words
```

### plugin.json shape

```json
{
  "name": "kg",
  "displayName": "kg Knowledge Graph",
  "version": "0.3.0",
  "description": "LLM-driven enrichment over kg structural extraction. Generates summaries, architectural layers, and pedagogical tours for codebases ingested via the kg engine.",
  "skills": "./skills",
  "agents": "./agents"
}
```

### marketplace.json shape

For users to install via `/plugin marketplace add github:ggfarmco/kg`:

```json
{
  "schema": "marketplace-v1",
  "name": "kg",
  "repository": "github.com/ggfarmco/kg",
  "plugin": "./.claude-plugin"
}
```

The kg CLI binary is a prerequisite (`kg --version` must work in PATH). Skills check for this at startup and prompt the user to install if missing. Install instructions point at `make install` or the prebuilt binary.

## Skills (orchestrators)

### `/kg-enrich`

The headline skill. Orchestrates the full enrichment pipeline.

**Args:** `[--domain <id>] [--source <id>] [--max-files <N>]`
- `--domain`: target domain (default: detect from kg.db — if only one, use it; else prompt)
- `--source`: structural source to enrich over (default: `tree-sitter:0.2.0`)
- `--max-files`: cap the number of files processed (cost guard for huge codebases)

**Pre-checks:**
- `kg --version` works
- `kg.db` exists (cwd or `--db` overridden)
- Domain exists in kg
- Source has nodes in that domain (else nothing to enrich)

**Phase flow** (see [Architecture](#architecture) above for the diagram):

1. Query kg via `dump-files.sh <domain> <source>` → JSON list of file-layer nodes with `{node_id, path, package_node_id, children_count}`. Save to `intermediate/files.json`.
2. Batch files into groups of ~25 (capped by `--max-files`). Each batch's input gets written to `intermediate/batch-N-input.json` by `dump-batch-context.sh` which adds the decl-level node info per file.
3. Dispatch **5 concurrent** `file-summarizer` agents (one per batch, more batches → more dispatch waves). Each agent: reads file, LLM judges, emits snapshot, pipes to `kg apply --source kg-summary:0.1.0 --scope additive`.
4. After all summarizer batches resolve: dispatch single `architecture-analyzer`. Input: full file-list + import adjacency from `dump-graph-shape.sh`. Output: snapshot for `<orig>-arch` domain piped to `kg apply --source kg-arch:0.1.0 --domain <orig>-arch --scope domain-source`.
5. After architecture-analyzer: dispatch single `tour-builder`. Input: file-list + topology + arch layers from `dump-topology.sh`. Output: snapshot for `<orig>-tours` domain piped to `kg apply --source kg-tours:0.1.0 --domain <orig>-tours --scope domain-source`.
6. Print summary report: `{batches_succeeded, batches_failed, nodes_enriched, semantic_edges_added, arch_layers, tour_steps}`. If failures: surface details with retry hint.

**Idempotency:** Running twice on the same project re-enriches everything. file-summarizer's `kg-summary:0.1.0` snapshot under `scope: additive` overwrites properties in that namespace (no double-stacking). architecture-analyzer's `scope: domain-source` cleans up old arch nodes (since same source = "I own everything here"). Same for tour-builder.

**Failure modes:** see [Error handling](#error-handling-and-recovery) section.

### `/kg-explain`

Read-only. Asks Claude to explain a specific node using its enriched context.

**Args:** `<node-id>` (e.g., `myapp:graph/handler-go::serve`)

**Flow:**
1. `kg node get <id> --merged` to fetch all-source properties union
2. `kg edge list-from <id>` and `kg edge list-to <id>` for 1-hop context
3. For each neighbor, fetch its `--merged` properties
4. Skill prompt instructs Claude (no subagent) to answer "what does this node do, how does it fit in, what should I read next?" with all that context in-prompt

No graph mutation. Output is text to user.

### `/kg-tour`

Standalone trigger for `tour-builder` agent only. For when user wants to regenerate the tour without re-running file summarization (e.g., after manual edits to summaries via `/kg-enrich --only-summary` followed by a re-tour).

**Args:** `[--domain <id>]`

Dispatches tour-builder per /kg-enrich Phase 5.

### `/kg-onboard`

Read-only. Generates `docs/ONBOARDING.md` from the enriched graph.

**Args:** `[--domain <id>] [--output <path>]`

**Flow:**
1. Query kg for project description (from `kg node list --domain X --layer package`), arch layers (`kg node list --domain X-arch`), tour steps (`kg node list --domain X-tours` sorted by `properties.kg-tours:0.1.0.order`)
2. Format as markdown: title + project description + architectural overview + tour with step-by-step file references and summaries
3. Prompt user to save to `docs/ONBOARDING.md` (or `--output` path)

## Agents

Every agent follows the two-phase pattern. Frontmatter:

```yaml
---
name: <agent-name>
description: <one-line, used by dispatcher>
model: inherit
allowed-tools: Read, Bash, Edit
---
```

### `file-summarizer`

Single most important agent. Annotates a batch of file/decl nodes with summaries, tags, complexity, and semantic edges.

**Dispatcher input** (passed via Task tool prompt):

```json
{
  "batch_id": 3,
  "domain": "myapp",
  "structural_source": "tree-sitter:0.2.0",
  "files": [
    {
      "node_id": "myapp:graph/handler-go",
      "file_path": "/abs/path/internal/graph/handler.go",
      "package_node_id": "myapp:graph",
      "decls": [
        {"node_id": "myapp:graph/handler-go::serve", "name": "Serve", "kind": "function", "line_range": [42, 87]},
        {"node_id": "myapp:graph/handler-go::route", "name": "Route", "kind": "function", "line_range": [90, 105]}
      ]
    }
  ]
}
```

**Phase 1 (deterministic):** Agent reads each file via the `Read` tool, takes the exact `line_range` excerpts for each decl, formats into a per-decl reading buffer. No script bundled — this is just Read+slice.

**Phase 2 (LLM):** Agent emits a JSON snapshot. Shape:

```json
{
  "protocol_version": 2,
  "source": "kg-summary:0.1.0",
  "domain": "myapp",
  "scope": "additive",
  "nodes": [
    {
      "id": "myapp:graph/handler-go",
      "properties": {
        "summary": "HTTP handler for /api/users endpoint.",
        "tags": ["api", "http", "users"],
        "complexity": "moderate",
        "language_notes": "Uses gorilla/mux router pattern"
      }
    },
    {
      "id": "myapp:graph/handler-go::serve",
      "properties": {
        "summary": "Main HTTP handler. Validates auth token, calls UserService.Get.",
        "tags": ["entrypoint", "auth"],
        "complexity": "moderate"
      }
    }
  ],
  "edges": [
    {"src": "myapp:graph/handler-go::serve", "target": "myapp:service/user-go::get", "type": "depends_on"},
    {"src": "myapp:graph/handler-go::serve", "target": "myapp:auth/middleware-go::validate", "type": "uses"}
  ]
}
```

**Output contract:**
- `nodes[].id` MUST come from the batch input (no new node creation)
- `nodes[].layer` / `nodes[].parent` / `nodes[].name` are OMITTED. Engine in additive-scope-foreign-node mode (see [Engine changes](#engine-changes)) writes properties only.
- `edges[].src` and `edges[].target` MUST be existing node IDs. Engine rejects with `NODE_NOT_FOUND` if not.
- `edges[].type` is from a curated vocabulary (see [Edge types](#semantic-edge-vocabulary) below).

**Output piping:** Snapshot piped through stdin to:

```
kg apply --source kg-summary:0.1.0 --domain <domain> --scope additive
```

**Retry behavior:** If `kg apply` returns a schema error (exit 1, error code `INVALID_NODE_ID` etc.), agent reads the error envelope, corrects the snapshot, retries once. Second failure: exit non-zero with details for the orchestrator.

**Edge cases:**
- File with zero decls: emit just the file-level property block, skip decls.
- File that's all generated code (e.g., `_gen.go`): emit a tag `["generated"]` and skip detailed summary, just `summary: "Generated code."`.
- Large file (>2000 lines): agent is responsible for reading only the line_ranges for decls (Phase 1 already does this).

### `architecture-analyzer`

Single instance. Infers 3-10 architectural layers from the graph shape and assigns each file to a layer.

**Dispatcher input:**

```json
{
  "domain": "myapp",
  "structural_source": "tree-sitter:0.2.0",
  "graph_shape_path": "/abs/path/.kg-enrich-tmp/graph-shape.json"
}
```

`graph-shape.json` is produced by `dump-graph-shape.sh` and contains: file list with package paths, import adjacency matrix, directory grouping, intra-group density. The agent reads this file directly.

**Phase 1 (deterministic):** Read graph-shape.json + read README.md from project root (for additional context about architectural intent).

**Phase 2 (LLM):** Synthesize 3-10 layer names (e.g., "API Layer", "Service Layer", "Storage Layer", "Domain", "Infrastructure"), assign each file node to exactly one layer.

**Output snapshot:**

```json
{
  "protocol_version": 2,
  "source": "kg-arch:0.1.0",
  "domain": "myapp-arch",
  "scope": "domain-source",
  "domain_spec": {
    "id": "myapp-arch",
    "layers": ["layer"],
    "description": "Architectural layers of myapp"
  },
  "nodes": [
    {
      "id": "myapp-arch:api-layer",
      "layer": "layer",
      "name": "API Layer",
      "properties": {
        "description": "HTTP endpoints and request handlers. Maps URLs to service calls and translates responses to JSON.",
        "order": 1
      }
    },
    {
      "id": "myapp-arch:service-layer",
      "layer": "layer",
      "name": "Service Layer",
      "properties": {
        "description": "Business logic. Orchestrates domain operations.",
        "order": 2
      }
    }
  ],
  "edges": [
    {"src": "myapp-arch:api-layer", "target": "myapp:graph/handler-go", "type": "contains"},
    {"src": "myapp-arch:service-layer", "target": "myapp:service/user-go", "type": "contains"}
  ]
}
```

**Notes:**
- `properties.order` is integer 1..N for rendering order in dashboard / onboarding doc (top→bottom).
- Cross-domain `contains` edges work because kg v0 explicitly designed for cross-domain relationships.
- `domain_spec` creates `myapp-arch` if missing; if it exists, the spec is silently skipped (per v2 apply algorithm).

### `tour-builder`

Single instance. Generates 5-15 ordered tour steps that teach the codebase in dependency order.

**Dispatcher input:**

```json
{
  "domain": "myapp",
  "structural_source": "tree-sitter:0.2.0",
  "arch_domain": "myapp-arch",
  "topology_path": "/abs/path/.kg-enrich-tmp/topology.json"
}
```

`topology.json` from `dump-topology.sh`: fan-in/fan-out ranking, entry point candidates (scoring system: `main.go` +5, `cmd/*/main.go` +3, README mention +5), import chains from entries via BFS.

**Phase 2:** Design tour steps. Each step references 1-3 nodes (usually 1 file + maybe 1-2 helper decls).

**Output snapshot:**

```json
{
  "protocol_version": 2,
  "source": "kg-tours:0.1.0",
  "domain": "myapp-tours",
  "scope": "domain-source",
  "domain_spec": {
    "id": "myapp-tours",
    "layers": ["step"],
    "description": "Onboarding tour for myapp"
  },
  "nodes": [
    {
      "id": "myapp-tours:01-overview",
      "layer": "step",
      "name": "Project Overview",
      "properties": {
        "order": 1,
        "description": "Start with the README and main.go to understand the high-level shape.",
        "estimated_minutes": 5
      }
    },
    {
      "id": "myapp-tours:02-entry-point",
      "layer": "step",
      "name": "Entry Point",
      "properties": {
        "order": 2,
        "description": "Trace startup from main() through dependency injection.",
        "estimated_minutes": 10
      }
    }
  ],
  "edges": [
    {"src": "myapp-tours:01-overview", "target": "myapp:docs/readme-md", "type": "teaches"},
    {"src": "myapp-tours:02-entry-point", "target": "myapp:cmd/cmd-main-go::main", "type": "teaches"}
  ]
}
```

**Notes:**
- `properties.order` is integer, used by `/kg-onboard` to sort steps deterministically.
- `properties.estimated_minutes` optional — useful for dashboard later.
- Each step has 1..N `teaches` edges to file/decl nodes; the same file can appear in multiple steps.

## Bundled scripts

All in `skills/kg-enrich/scripts/`. Pure bash + `jq` + `kg` CLI. No Python, no Node.

### `dump-files.sh <domain> <source>`

```bash
#!/usr/bin/env bash
set -euo pipefail
domain="$1"
source_id="$2"

kg node list --domain "$domain" --layer file --source "$source_id" \
  | jq --arg dom "$domain" --arg src "$source_id" '
    .data | map({
      node_id: .id,
      file_path: .properties[$src].path,
      package_node_id: .parent_id,
      name: .name
    })
  '
```

Output: JSON array of file descriptors. Consumed by `kg-enrich` SKILL.md to batch.

### `dump-batch-context.sh <files-json>`

For a batch of file IDs (passed via stdin or file arg), enriches each with its decl list.

```bash
#!/usr/bin/env bash
set -euo pipefail
input="$1"  # path to JSON array of file descriptors

jq -c '.[]' "$input" | while read -r file; do
  fileId=$(echo "$file" | jq -r '.node_id')
  decls=$(kg node children "$fileId" | jq '[.data[] | select(.layer == "decl") | {
    node_id: .id,
    name: .name,
    kind: .properties["tree-sitter:0.2.0"].kind,
    line_range: [.properties["tree-sitter:0.2.0"].line_start, .properties["tree-sitter:0.2.0"].line_end]
  }]')
  echo "$file" | jq --argjson decls "$decls" '. + {decls: $decls}'
done | jq -s '.'
```

Output: same JSON array but each file now has `decls[]` populated.

### `dump-graph-shape.sh <domain> <source>`

Bigger script for architecture-analyzer. Builds the per-package import adjacency + directory grouping. ~80 lines. Output: `{packages: [{slug, path, files: [...]}], imports: [{from, to}], directory_tree: {...}}`.

### `dump-topology.sh <domain> <source>`

For tour-builder. Computes per-node fan-in/fan-out from `imports` and `calls` edges, ranks entry points (heuristic scoring: `main.go` +5, exported function +1, etc.), traces BFS chains from top entries. Output: `{entries: [...ranked...], chains: [[id1, id2, id3], ...], hotspots: [...high-fan-in...]}`.

### `apply-snapshot.sh <source> <domain> <scope>`

Thin wrapper around `kg apply`:

```bash
#!/usr/bin/env bash
set -euo pipefail
exec kg apply --source "$1" --domain "$2" --scope "$3"
```

Agents pipe snapshot via stdin: `cat snap.json | apply-snapshot.sh kg-summary:0.1.0 myapp additive`.

The wrapper exists so the SKILL.md doesn't have to remember the right flag order. Trivially small but improves readability of agent prompts.

## Source IDs and domains

### Source IDs

| Agent | Source ID | Purpose |
|---|---|---|
| file-summarizer | `kg-summary:0.1.0` | Per-decl summaries, tags, complexity, semantic edges |
| architecture-analyzer | `kg-arch:0.1.0` | Architectural layer nodes + `contains` edges |
| tour-builder | `kg-tours:0.1.0` | Tour step nodes + `teaches` edges |

Different source IDs let users:
- Re-run summaries without touching tours (`/kg-enrich --only-summaries` — Future)
- Inspect just the LLM-added layer with `kg node get <id> --source kg-summary:0.1.0`
- Trust-tune per agent: `kg sources update kg-summary:0.1.0 --description "LLM enrichment ..."`

### Domains created

| Domain | Layers | Created by | Notes |
|---|---|---|---|
| `<orig>` | (existing) | tree-sitter | Original structural domain. v3 enricher writes properties + edges here under `kg-summary:0.1.0`. |
| `<orig>-arch` | `[layer]` | `kg-arch:0.1.0` | One node per architectural layer. Cross-domain `contains` edges to `<orig>:*` file nodes. |
| `<orig>-tours` | `[step]` | `kg-tours:0.1.0` | One node per tour step. Cross-domain `teaches` edges to `<orig>:*` file/decl nodes. |

`<orig>` is detected by the skill — usually the user has one main domain per kg.db.

### Semantic edge vocabulary

The vocabulary file-summarizer is allowed to emit:

| Edge type | Direction | Meaning |
|---|---|---|
| `depends_on` | A→B | A's correctness depends on B's behavior (not just import) |
| `implements` | A→B | A is a concrete implementation of interface/contract B |
| `exposes` | A→B | A makes B accessible (e.g., handler exposes service endpoint) |
| `documented_in` | A→B | A is documented in B (typically a `.md` file) |
| `configured_by` | A→B | A's runtime is configured by B (config file or env var) |
| `uses` | A→B | Weaker than depends_on; A invokes B but is not blocked by B |
| `extends` | A→B | A extends/inherits B's structure |
| `tested_by` | A→B | A's correctness is verified by test B |

Vocabulary is enforced by the file-summarizer's prompt (we list the valid types). The engine accepts ANY edge type (no schema validation), so a hallucinated type just becomes a weird edge — harmless but inconsistent. We rely on agent prompt discipline rather than engine enforcement.

architecture-analyzer adds only `contains`. tour-builder adds only `teaches`.

## Engine changes

Three changes to the kg engine, two blocking (must ship for v3 to work) and one nice-to-have.

### Must-have: additive scope writes properties on foreign-owned nodes

**Problem.** Current `Service.applyNodeSpec` (in `internal/graph/service_apply.go`) handles the case "snapshot has a node id that we don't own" with:

```go
if scope == snapshot.ScopeAdditive {
    return nil  // skip foreign nodes silently
}
```

This is correct for the case "additive snapshot tries to redefine a node" but wrong for "additive snapshot wants to annotate a foreign node with properties in its own namespace." The latter is the v3 enricher's primary use case.

**Fix.** In additive scope, when encountering a foreign-owned node:
- If `spec.Properties` is empty → skip (matches current behavior; nothing to add).
- If `spec.Properties` non-empty → call `Service.SetNodeProperties(ctx, id, source, spec.Properties)`. This writes properties under the writer's source namespace, leaves layer/parent/name (which the writer doesn't own) untouched.

**NodeSpec relaxation.** Also: make `NodeSpec.Layer`, `Parent`, `Name` optional when scope is additive AND the node already exists. Agent doesn't know structural info; engine shouldn't require it. Validation (`snapshot.Validate`) tightens only on `scope: domain-source` and `scope: domain`.

**Test.** New test `TestApplyAdditiveAnnotatesForeignNode` in `service_apply_test.go`:
- Source A creates node `d:x` with property `{a-key: a-val}` under namespace A.
- Source B applies additive snapshot with `{id: "d:x", properties: {b-key: b-val}}` (NO layer/parent/name).
- Resulting node has properties `{A: {a-key: a-val}, B: {b-key: b-val}}` — both namespaces present.

### Must-have: `kg export --domain X --source Y [--format snapshot]`

New CLI verb. Outputs the current `(domain, source)` state as a valid snapshot JSON document.

```sh
kg export --domain myapp --source tree-sitter:0.2.0 > tree-sitter-state.json
```

**Why needed:** agents that want a baseline of "what does my source currently have in kg" need to read it cleanly. Yes, this is composable from `kg node list ... | jq ...`, but the jq pipeline is non-trivial (have to assemble nodes, edges via list calls, format as snapshot). A proper verb avoids agents reinventing it and getting it wrong.

**Implementation:** wrapper that calls existing Service methods and uses the snapshot package's Encode. ~50 lines + tests.

**Output:** valid snapshot JSON, `scope: "domain-source"` by default (since exporting a single-source view), suitable for piping to `kg apply` for re-import or diffing.

### Nice-to-have (deferred): combined query

`kg node list --decl-with-file` would give agents decl + parent file info in one call. Currently requires `kg node list --layer decl` + per-decl `kg node get` + per-decl `kg node get <parent>`. ~3x query count for `file-summarizer`'s setup phase.

Not blocking for MVP — `dump-batch-context.sh` does the joining via jq + nested kg calls. Re-evaluate after v3 ships if it becomes a hot path.

## Error handling and recovery

### Per-batch failure (file-summarizer)

Scenarios:
- LLM API timeout / hard error
- Agent script crashes (e.g., bash error)
- Agent emits malformed JSON
- `kg apply` rejects snapshot (e.g., `INVALID_NODE_ID`)

Behavior:
- file-summarizer agent retries up to 2 times for `kg apply` rejections (reads error envelope, corrects snapshot, re-applies). For other failures, exits non-zero immediately with structured error info.
- Orchestrator skill logs failure, includes batch in `failed[]` list, continues other batches.
- After all batches: skill summary says "X/Y batches succeeded. Failed: [batch_id, reason, ...]." Prompts user "retry failed batches?" → loops if yes.

### Apply rejection at any stage

`kg apply` enforces schema and FK constraints. If a snapshot references unknown nodes (e.g., agent hallucinated an id), engine returns error envelope with `NODE_NOT_FOUND` and the id. Agent reads, drops the bad edge, retries. After 2 failed retries → escalate to orchestrator.

### Architecture / tour failure

If architecture-analyzer fails, tour-builder still runs (tour can use topology without architectural layers — it just gets a flatter view). Both failures don't block `/kg-onboard` (which works on whatever's in the graph).

If both fail, /kg-enrich exits with code 1 and prints what survived.

### No partial commits within a phase

Each `kg apply` is atomic. Within a phase, we never get a half-committed state.

### Cross-phase partial state IS possible

Possible: summary done, arch failed, tour done. This is acceptable. Re-running `/kg-enrich` will retry failed phases. The DB stays consistent — every committed phase's data is good.

### What's NOT auto-retried

- LLM cost failures (no API quota): agent fails clean, skill reports, user fixes.
- Permission errors (read-only fs, missing executable): agent fails clean, skill suggests fix.

## Concurrency

- file-summarizer: 5 parallel agents (matches UA's bounded concurrency, validated by them at scale)
- architecture-analyzer + tour-builder: single instance each
- `kg apply` calls happen per agent; SQLite WAL handles 5 concurrent writers fine (we tested 5×short-tx WAL writes in kg v2 e2e implicitly)

## Testing strategy

Three tiers (engine + scripts + skills/agents). Engine fully automated; scripts mostly automated; skills/agents human-validated.

### Tier 1 — engine changes

New unit tests in `internal/graph/service_apply_test.go`:
- `TestApplyAdditiveAnnotatesForeignNode` — additive scope writes properties on foreign node without touching layer/parent/name.
- `TestApplyAdditiveSkipsForeignNodeWithoutProperties` — backward-compat for the old behavior when spec has no properties.
- `TestNodeSpecOptionalFieldsOnAdditive` — validation accepts missing layer/parent/name in additive scope when the id exists.

New unit tests in `cmd/kg/export_cmd_test.go`:
- `TestExportRoundTrip` — `kg export domain X source Y | kg apply --source Y --domain X` is a no-op.
- `TestExportEmptySource` — exporting a source with no nodes returns a valid empty snapshot.

### Tier 2 — bundled scripts

Each script gets a `scripts/<name>.test.sh` companion. Pure bash test: feed fixed input via mock kg (a shell function that returns canned JSON), assert output via `diff` against expected fixture. Run via `make test-scripts`.

### Tier 3 — skills + agents

Hardest to automate. Two parts:

**Skill prompt smoke** (manual): run `/kg-enrich` against a tiny fixture project (`testdata/v3-fixture/` — 5 Go files, controlled output). Verify resulting graph has:
- summary/tags on every decl node under `kg-summary:0.1.0` namespace
- `<fixture>-arch` domain exists with at least 2 layer nodes
- `<fixture>-tours` domain exists with at least 3 step nodes
- cross-domain edges with correct sources

**Agent prompt unit test** (limited): for each agent's "Phase 1 deterministic" part, the inputs and bash invocations can be tested with a mock kg. The Phase 2 LLM part is fundamentally non-deterministic; we test that the output JSON is schema-valid (via `kg apply --dry-run`) but not its semantic content.

### Tier 4 — e2e

A new test `e2e/enrich_self_test.go` (build tag `e2e-enrich`):

Manual / CI: run `/kg-enrich` against `internal/graph` (kg's own code). Assertions:
- Pre: tree-sitter ran first (already covered by v1/v2 e2e)
- All file nodes have `kg-summary:0.1.0` namespace properties
- `selfg-arch` domain has 3+ layers
- `selfg-tours` domain has 5+ steps
- `kg node get internal/graph/service-go --merged` returns merged props from both tree-sitter and kg-summary

This test costs LLM credits (real Claude calls). Run in CI only on `main` after merge, not on every PR.

## Open Risks

| Risk | Mitigation |
|---|---|
| Skill orchestration fragility — markdown-driven control flow is brittle | Aggressive inline guard rails in SKILL.md; bundled scripts for deterministic glue; copy battle-tested patterns from UA |
| LLM hallucinated node IDs reach `kg apply` and clutter graph | Agent validates IDs against batch input before emitting; engine rejects unknown ids; retry-with-correction |
| LLM cost balloons on large projects | `--max-files N` flag; document expected cost per LOC in README; future: per-source cost tracking |
| Per-batch parallel applies → tx contention on busy SQLite | WAL mode; bounded concurrency 5; if it becomes painful, fall back to serial applies |
| Architectural layer names drift between runs | scope: domain-source ensures same source replaces same domain — last apply wins; old layers cleanly removed |
| Cross-domain `contains` / `teaches` edges not surfaced well in kg CLI | Add CLI helper later: `kg node show <id> --cross-domain` (defer) |
| Skills require kg CLI in PATH | Skill pre-checks; clear error message + install hint |
| Multi-source schema drift (tree-sitter properties shape changes) | Document the shape contract; integration tests catch incompatibility before release |
| Engine tweak for additive-foreign-properties is a v3 dependency that retroactively changes v2 semantics | Document in CHANGELOG; the change is purely additive (old behavior preserved when no properties) |
| `/kg-enrich` on huge repo (100k+ LOC) is unbounded LLM cost | `--max-files` + per-domain trial-run advice in README; v3.1 may add fingerprint-based incremental |
| LLM-generated semantic edges contradict structural ones | Both coexist as separate claims; consumer (or `--merged` view) decides ranking. Document this. |
| Token gates / dashboard concerns | N/A in v3 (no dashboard); v4 inherits UA's token-gating approach |

## Implementation plan

To be authored next via `writing-plans` skill. The plan will sequence (approximately 15-18 tasks across 5 phases):

**Phase 1 — Engine prep (3 tasks)**
1. Engine tweak: additive scope writes properties on foreign-owned nodes (with relaxed NodeSpec required-fields validation).
2. New CLI verb: `kg export --domain X --source Y --format snapshot`.
3. Update README + CHANGELOG with v3 engine changes.

**Phase 2 — Plugin scaffold (2 tasks)**
4. Create `.claude-plugin/` directory structure with `plugin.json` and `marketplace.json`.
5. Implement bundled scripts: dump-files.sh, dump-batch-context.sh, dump-graph-shape.sh, dump-topology.sh, apply-snapshot.sh. Tests for each.

**Phase 3 — Agents (3 tasks)**
6. Write `agents/file-summarizer.md` — the big one. Two-phase contract, output spec, retry logic, semantic edge vocabulary, error handling.
7. Write `agents/architecture-analyzer.md`.
8. Write `agents/tour-builder.md`.

**Phase 4 — Skills (4 tasks)**
9. Write `skills/kg-enrich/SKILL.md` — orchestrator with 5 dispatch waves.
10. Write `skills/kg-explain/SKILL.md`.
11. Write `skills/kg-tour/SKILL.md`.
12. Write `skills/kg-onboard/SKILL.md`.

**Phase 5 — Tests + polish (4 tasks)**
13. Manual smoke test against `testdata/v3-fixture/` (small Go project), capture observed behavior in `docs/v3-fixture-trace.md`.
14. E2E test `e2e/enrich_self_test.go` (build tag, manual-trigger CI job).
15. README extractor section update for v3 (skill install instructions, usage examples, cost expectations).
16. Branch close-out + PR.

Estimated scope: ~3-5k lines of markdown (skills + agents + README) + ~500 lines of Go (engine changes + export verb + tests) + ~300 lines of bash (bundled scripts + script tests). Smaller than v2 by code volume, but heavier in prompt-engineering iteration.
