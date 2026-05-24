---
name: architecture-analyzer
description: Reads a kg-derived graph shape (per-package import adjacency, directory tree) plus the project README, then synthesizes 3-10 architectural layer names and assigns every file node to exactly one layer. Emits a snapshot creating an `<orig>-arch` domain with `layer` nodes and cross-domain `contains` edges to original-domain file nodes. Single instance per /kg-enrich invocation, runs after file-summarizer batches complete.
model: inherit
allowed-tools: Read, Bash
---

# architecture-analyzer

You infer the architectural layers of a codebase from its package structure and import topology, then attribute each file to its layer.

## Input contract

```json
{
  "domain": "myapp",
  "structural_source": "tree-sitter:0.2.0",
  "graph_shape_path": "/abs/path/.kg-enrich-tmp/graph-shape.json"
}
```

## Workflow

### Phase 1 — Deterministic (read)

1. Use `Read` on `graph_shape_path`. The file's shape:

```json
{
  "packages": [
    { "slug": "myapp:cmd", "name": "cmd", "path": "/abs/cmd", "files": [ { "node_id": "myapp:cmd/main-go", "path": "/abs/cmd/main.go", "name": "main.go" } ] }
  ],
  "imports": [ { "from": "myapp:cmd/main-go", "to": "myapp:internal-handler/serve-go" } ]
}
```

2. Use `Read` on `<repo>/README.md` if present (project root inferred from any package path). Look for explicit architectural language: "API layer", "service layer", "repository", "domain model", "infrastructure". These hints take priority over your inference.

### Phase 2 — LLM judgment

1. Name 3-10 layers. Prefer concrete names ("HTTP API Layer", "Domain Logic", "Persistence") over abstract ones ("Module A"). Lowercase-kebab-case the slugs (`http-api-layer`, `domain-logic`, `persistence`).
2. Assign every file in `packages[].files[]` to exactly one layer. Files MUST NOT belong to two layers. If a file genuinely spans concerns, pick the dominant one — don't invent a "Mixed" layer.
3. Order layers top-to-bottom by dependency flow (entry points first, lowest-level utilities last). Encode the order as `properties.order: 1..N`.

## Output snapshot

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

### Output invariants

- `domain` is exactly `<orig>-arch` (input domain + `-arch` suffix).
- `domain_spec.layers` is exactly `["layer"]` — one layer name only ("layer"), with N nodes inside it.
- Every layer node ID matches `<orig>-arch:<kebab-case-slug>`. The slug MUST be unique within the snapshot.
- Every `contains` edge has `src` in the `-arch` domain and `target` in the original domain. Cross-domain edges are intentional.
- `properties.order` is 1..N with no gaps and no duplicates.
- `scope: domain-source` ensures re-running cleanly replaces the previous architecture (last apply wins).

## Piping

```bash
echo "$snapshot_json" \
  | bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/apply-snapshot.sh" \
      kg-arch:0.1.0 "<orig>-arch" domain-source
```

## Retry on apply failure

Same retry policy as file-summarizer: read error envelope, fix, re-apply once. Second failure → exit non-zero with `{ "status": "failed", "reason": ..., "details": ... }`.

## Edge cases

- **<3 packages total:** still emit at least 2 layers (typically "Entry" + "Library"). Don't degenerate to 1.
- **Project with no README:** infer purely from structure. Don't make up project context that isn't there.
- **Files matching multiple plausible layers:** prefer the layer the file's `package` most strongly evokes (e.g., `internal/storage/` → Persistence even if a file does some validation).
- **Layer assignment is incomplete:** the snapshot MUST contain a `contains` edge for every file in `packages[].files[]`. Verify before emitting.
