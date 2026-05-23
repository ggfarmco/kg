# kg v2 — Provenance + Declarative Apply Design Spec

**Date:** 2026-05-24
**Status:** Approved for implementation planning
**Builds on:** `docs/superpowers/specs/2026-05-23-kg-mvp-design.md` (v0), `docs/superpowers/specs/2026-05-23-kg-v1-extractor-design.md` (v1)

## Background

v1 shipped a working extractor pipeline (`plugin → kg-extractor → kg batch`) with a JSONL imperative-ops wire protocol. The pipeline works end-to-end: kg can extract its own `internal/graph` package via the tree-sitter Go plugin and ends up with a navigable `package → file → decl` graph plus `imports` and `calls` edges.

v1 also surfaced two architectural gaps that no amount of plugin tuning could close:

1. **Re-extract leaks stale state.** When a plugin runs a second time after the source code has changed, additions land cleanly (`if_not_exists` skips duplicates), but deletions, renames, property updates, and file removals do not. The graph drifts further from the real codebase with every run. This is documented in v1's "Open Risks" as "Stale nodes accumulate over re-extracts" — punted to v2 as snapshot semantics.

2. **No model for multiple sources of truth.** A future LLM enricher (v2 in the original roadmap) would add summaries and semantic edges to nodes that the structural extractor already populated. A second structural extractor (e.g., `go/packages` running alongside tree-sitter to add cross-package edges) wants to co-exist with the first. Cross-domain edges (`myapp:engine → physics:thermo (governed_by)`) added by hand or by an LLM must survive when the structural extractor for `myapp` re-runs. Today's model has no way to attribute writes to a writer, so any of those scenarios silently corrupts the graph.

These are two faces of the same missing concept: **provenance**. Once entities carry information about *who* asserted them, both problems become solvable: re-extract can scope its diff to "what *I* claimed last time"; multiple writers can coexist because each touches only its own claims and namespaced properties.

This spec proposes a clean rebuild of v0's schema (no data exists yet) plus a new **declarative wire format** for plugins, layered alongside the existing imperative ops. The user describes desired state; kg computes the diff. The imperative path remains as a low-level wire protocol for streaming, testing, and non-snapshot use cases.

### Why a clean rebuild instead of migration

The project is in active development with no production data. Rather than ship migration 0002 layering source columns onto existing tables, migration 0001 is rewritten in place to include provenance from day one. This avoids the back-compat acrobatics of nullable-source-with-default-`'manual'`, namespaced-or-flat-properties-detection, and dual-mode validation that would otherwise haunt the codebase.

### Why declarative is layered, not replacing

Imperative ops (`kg batch` with JSONL) remain the **wire protocol** — the lowest-level way to mutate the graph. Declarative apply (`kg apply` with a JSON snapshot) is a **convenience layer** sitting on top, implemented as: parse snapshot → compute diff → generate imperative ops → apply atomically. Plugins choose the form that fits their shape: streaming-heavy or per-event extractors stay imperative; one-shot snapshot extractors (tree-sitter, bash-demo, future markdown-with-wikilinks) go declarative. Both writers ultimately produce the same mutations against the same store.

## Goals

1. **Provenance on every mutation.** Every node and every edge claim is attributed to a named source. Service mutations require a source; CLI commands take `--source` (default `cli`).
2. **Multi-source edges via reference counting.** An edge `(src, tgt, type)` is one row in `edges`; the set of sources that assert it lives in `edge_claims`. Adding a claim is idempotent; removing the last claim deletes the edge.
3. **Single-source nodes with namespaced properties.** A node has one owner (immutable: `layer`, `parent_id`, `name`). Properties are JSON keyed by source id, so writers extend each other's contributions without conflict.
4. **Declarative apply with computed diff.** A plugin emits a full snapshot of what it currently owns in a domain; `kg apply` loads its previous state for that `(domain, source)`, computes add/update/delete, and applies them in one transaction.
5. **Re-extract converges on the source.** Running the same plugin twice with no code changes is a no-op; running after changes produces exactly the minimal diff. Deletions, renames, and property updates land cleanly.
6. **Cross-source edges survive re-extract.** If pyflakes-plugin claims `imports` edges and tree-sitter-go also does, removing tree-sitter's claim leaves the edge alive as long as pyflakes still claims it.
7. **Trust-aware queries.** Sources have a `trust` integer (0-100). CLI can filter or rank by trust when conflicts arise (currently relevant only for properties merge view).
8. **Audit through `changes`.** Every mutation appends a `changes` row with the originating source, enabling per-source change streams in v6 collaboration work.

## Non-Goals (deferred to v3+)

- **Multi-source ownership of nodes.** A node still has one owner. If a second source attempts to add a node with an existing id, it gets `NODE_OWNED_BY_DIFFERENT_SOURCE`. Future work (v3) may add `node_claims` symmetric to `edge_claims`, but core fields (layer, parent_id, name) remain single-owner — they're topological, not assertive.
- **Property merge / conflict resolution policy.** Properties are namespaced by source; they don't conflict. A `--merged` CLI view does best-effort union with last-write-wins fallback for keys that collide across namespaces, but no semantic merging.
- **Distributed sync between kg instances.** The `changes` log + `revision` foundation supports this in v6.
- **Plugin sandboxing.** Plugins run with user privileges; same trust model as `kubectl` plugins.
- **Incremental file-level extraction** (plugin sees previous state, only re-extracts changed files). v2 always full-snapshots; v3+ may add this as an optimization for very large repos.
- **Property history.** When a source updates its namespace, the previous value is lost (not versioned). v6 collaboration may revisit if temporal queries become a need.
- **Plugin contract negotiation.** v2 keeps `protocol_version: 1` and adds `protocol_version: 2` for declarative; both versions ship side by side. v3+ may add capability negotiation.

## Architecture

Two wire formats sitting on the same underlying mutation API:

```
Imperative path (low-level, streaming, same as v1):

┌─────────────────┐       ┌─────────────────┐       ┌─────────────┐
│   plugin        │ JSONL │  kg-extractor   │ JSONL │     kg      │
│  (any runtime)  ├──────►│  (validator)    ├──────►│    batch    │
└─────────────────┘       └─────────────────┘       └─────────────┘

Declarative path (snapshot-based, recommended default):

┌─────────────────┐       ┌─────────────────┐       ┌─────────────┐
│   plugin        │ JSON  │  kg-extractor   │ JSON  │     kg      │
│  (declarative   │snap-  │  (validator,    │snap-  │    apply    │
│   runtime)      │shot   │   passthrough)  │shot   │  (diff+app) │
└─────────────────┘       └─────────────────┘       └─────────────┘
```

Both ultimately write through `graph.Service`, which is now source-aware. `kg apply` internally synthesizes imperative ops from the diff and runs them through the same `Service.InTx` envelope `kg batch` uses, so atomicity and the `changes` log are uniform.

### Module / CGO isolation (unchanged from v1)

The kg root module stays pure Go. `plugins/tree-sitter/` is still a separate module with its own go.mod/go.sum holding CGO dependencies. `go.work` at the repo root unifies local dev; CI uses `GOWORK=off` to validate external-consumer builds.

## Database Schema

Five tables. Migration `0001_init.sql` rewritten in place. Pure Go SQLite via `modernc.org/sqlite`. Migrations via `pressly/goose`.

```sql
-- +goose Up

CREATE TABLE sources (
  id          TEXT PRIMARY KEY,                     -- "tree-sitter:0.1.0", "llm-enricher:1.0", "cli", "manual"
  description TEXT,
  trust       INTEGER NOT NULL DEFAULT 100,          -- 0-100, used in --merged property view ranking
  first_seen  INTEGER NOT NULL,                      -- unix ms
  last_seen   INTEGER NOT NULL
);

CREATE TABLE domains (
  id          TEXT PRIMARY KEY,
  description TEXT,
  layers      TEXT NOT NULL,                         -- JSON array, ordered top→bottom
  revision    INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL
);

CREATE TABLE nodes (
  id          TEXT PRIMARY KEY,                     -- "domain:slug" or compound "domain:pkg/file::name"
  domain      TEXT NOT NULL REFERENCES domains(id) ON DELETE RESTRICT,
  layer       TEXT NOT NULL,
  name        TEXT NOT NULL,
  parent_id   TEXT REFERENCES nodes(id) ON DELETE RESTRICT,
  source      TEXT NOT NULL REFERENCES sources(id), -- single owner of core fields
  properties  TEXT NOT NULL DEFAULT '{}',           -- namespaced JSON: {"<source-id>": {...}, ...}
  revision    INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);

CREATE INDEX idx_nodes_domain_layer  ON nodes(domain, layer);
CREATE INDEX idx_nodes_parent        ON nodes(parent_id);
CREATE INDEX idx_nodes_domain_source ON nodes(domain, source);

CREATE TABLE edges (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id   TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  target_id   TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  type        TEXT NOT NULL,
  properties  TEXT NOT NULL DEFAULT '{}',           -- namespaced JSON, same shape as nodes
  revision    INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL,
  UNIQUE(source_id, target_id, type)
);

CREATE INDEX idx_edges_source ON edges(source_id, type);
CREATE INDEX idx_edges_target ON edges(target_id, type);

CREATE TABLE edge_claims (
  edge_id     INTEGER NOT NULL REFERENCES edges(id) ON DELETE CASCADE,
  source      TEXT NOT NULL REFERENCES sources(id),
  claimed_at  INTEGER NOT NULL,
  PRIMARY KEY (edge_id, source)
);

CREATE INDEX idx_edge_claims_source ON edge_claims(source);

CREATE TABLE changes (
  seq         INTEGER PRIMARY KEY AUTOINCREMENT,
  entity      TEXT NOT NULL,                        -- "domain" | "node" | "edge" | "edge_claim" | "source"
  entity_id   TEXT NOT NULL,
  source      TEXT REFERENCES sources(id),          -- writer attribution (NULL for system events)
  op          TEXT NOT NULL,                        -- "create" | "update" | "delete" | "claim" | "unclaim"
  revision    INTEGER,                              -- post-op revision (NULL for deletes/unclaims)
  at          INTEGER NOT NULL
);

CREATE INDEX idx_changes_seq    ON changes(seq);
CREATE INDEX idx_changes_entity ON changes(entity, entity_id);
CREATE INDEX idx_changes_source ON changes(source);

-- +goose Down
DROP INDEX IF EXISTS idx_changes_source;
DROP INDEX IF EXISTS idx_changes_entity;
DROP INDEX IF EXISTS idx_changes_seq;
DROP TABLE IF EXISTS changes;
DROP INDEX IF EXISTS idx_edge_claims_source;
DROP TABLE IF EXISTS edge_claims;
DROP INDEX IF EXISTS idx_edges_target;
DROP INDEX IF EXISTS idx_edges_source;
DROP TABLE IF EXISTS edges;
DROP INDEX IF EXISTS idx_nodes_domain_source;
DROP INDEX IF EXISTS idx_nodes_parent;
DROP INDEX IF EXISTS idx_nodes_domain_layer;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS domains;
DROP TABLE IF EXISTS sources;
```

### Schema invariants

- `sources` is bootstrap-populated by `kg init` with two rows: `cli` (trust 100) and `manual` (trust 100). All other sources are auto-registered on first write.
- Every node has exactly one row in `sources` for its `source` (FK enforced).
- An edge is one physical row; the set of writers that assert it lives entirely in `edge_claims`.
- Edge GC: when the last row in `edge_claims` for a given `edge_id` is deleted, the edge row is deleted in the same transaction. This is enforced in Service, not via DB triggers (keeps the engine portable).
- `properties` is always namespaced JSON. Top-level keys are source ids; values are arbitrary maps. The empty case is `{}`, not `{"source": {}}`.
- `revision` bumps on UPDATE in the same transaction (unchanged from v0).
- Every mutation appends one row to `changes`. Edge claim operations get their own `entity = "edge_claim"`, `op = "claim" | "unclaim"`.

### ON DELETE policies

- `domains → nodes`: RESTRICT (unchanged from v0).
- `nodes → parent_id`: RESTRICT (unchanged from v0).
- `nodes → edges`: CASCADE — but with a twist. If a node deletion would orphan edges that have **non-owner claims**, the DELETE is blocked at the Service layer with `NODE_HAS_FOREIGN_CLAIMS`. This protects multi-source assertions from being collateral damage. Override via `--force-cascade` on `kg apply` and `kg node delete`.
- `edges → edge_claims`: CASCADE.
- `sources → nodes` / `sources → edge_claims` / `sources → changes`: RESTRICT — a source cannot be deleted while it owns anything. Sources are append-mostly; deletion is a v6 GC concern.

## Source registry

`sources` is a small registry table that gives every writer a stable identity, a trust score, and audit timestamps.

### Bootstrap

`kg init` inserts:

```sql
INSERT INTO sources(id, description, trust, first_seen, last_seen) VALUES
  ('cli',    'kg CLI commands',     100, ?, ?),
  ('manual', 'Manually authored',   100, ?, ?);
```

### Auto-registration

When a Service mutation arrives with a `source` that doesn't exist, the Service auto-registers it with `trust = 100` and `description = ''`. This keeps the happy path frictionless — plugins don't need a separate "register me" step. The CLI offers `kg sources update` to refine `description` and `trust` after the fact.

### Registration via CLI

```sh
kg sources register \
    --id tree-sitter:0.1.0 \
    --description "Tree-sitter Go structural extractor" \
    --trust 100
```

Returns the created row in the standard envelope. If the source already exists, `--if-not-exists` skips with `skipped: true`.

### Trust

`trust` is a 0-100 integer used in one place: the `--merged` property view (described below). When two sources contribute properties with the same top-level key, the higher-trust source wins; ties break alphabetically by source id. No other v2 mechanic uses `trust`; it's a knob for future ranking work.

### Manifest binding

Plugin manifests declare `source_id`:

```json
{
  "name": "tree-sitter",
  "version": "0.1.0",
  "source_id": "tree-sitter:0.1.0",
  "trust": 100,
  "runtime": "declarative-native",
  ...
}
```

If `source_id` is omitted, kg-extractor derives it as `<name>:<version>`. If `trust` is omitted, defaults to 100. The first run auto-registers; later runs use the existing row.

## Ownership Model

### Nodes: single owner

A node is created by exactly one source. That source owns the **structural** fields:

- `id` — immutable forever (already true in v0).
- `domain`, `layer`, `parent_id` — set on create; cannot be changed by any source thereafter.
- `name` — mutable, but only by the owner.

Rationale: these fields describe *what* a node is and *where* it sits in the hierarchy. Two sources disagreeing about a node's layer is a real conflict, not a merging opportunity. By giving nodes a single owner, we make this conflict impossible at the schema level.

What other sources can do to a node they don't own:
- **Add properties in their own namespace.** `Service.SetNodeProperties(ctx, id, source, props)` writes only into `properties.<source>`. Always allowed (assuming the node exists).
- **Claim edges incident to it.** Adding a claim on `edge(id=42, src=this-node, ...)` is allowed regardless of node ownership. Edges are independent assertions.

What other sources cannot do:
- Add a node with the same id (gets `NODE_OWNED_BY_DIFFERENT_SOURCE`).
- Modify name, layer, parent.
- Delete the node directly.

If two sources independently want to claim "node X exists" (e.g., both extractors emit a snapshot containing `myapp:graph`), the second writer's `node.add` for the same id is a no-op when the existing node has a different owner AND `if_not_exists: true`. Without `if_not_exists`, the conflict is reported.

### Edges: multi-source via reference counting

Edges are assertions, not topology. "auth depends on db" can be discovered by tree-sitter, by pyflakes, by hand, or by an LLM — all of these are valid simultaneously, and the edge surviving requires only that *someone* still asserts it.

Schema: one row in `edges` per unique `(source_id, target_id, type)` (enforced by `UNIQUE`). The set of sources asserting that edge lives in `edge_claims`, keyed by `(edge_id, source)`.

Lifecycle:

```
CreateEdge(ctx, src, tgt, type, source):
  1. Try INSERT INTO edges(source_id, target_id, type, ...) — get edge_id (or fetch existing)
  2. INSERT OR IGNORE INTO edge_claims(edge_id, source, claimed_at)
  3. Append changes row: entity=edge or entity=edge_claim

DeleteEdgeClaim(ctx, edge_id, source):
  1. DELETE FROM edge_claims WHERE edge_id=? AND source=?
  2. SELECT COUNT(*) FROM edge_claims WHERE edge_id=?
  3. If 0 → DELETE FROM edges WHERE id=? (the GC)
  4. Append changes row(s)
```

This is **reference counting**: edges live as long as they have ≥1 claim. Edge GC happens in the same transaction as the unclaim; no background process needed.

### Why nodes single-source and edges multi-source

It's tempting to unify (either both single or both multi), but they differ in nature:

- A node represents **identity** — "function ParseSlug at internal/graph/node.go". Two writers asserting the same identity is good (the node exists); two writers asserting different layer/parent for the same identity is a contradiction (which is right?). Identity belongs to its creator.
- An edge represents **belief** — "I believe `auth` imports `db`". Two writers asserting the same belief strengthens it (more evidence); they can't contradict the relation itself, only stop believing. Belief is naturally multi-claimant.

This split keeps node identity unambiguous while making edge assertions composable. v3 may extend to `node_claims` if cross-source identity assertion becomes a real need, but v2 doesn't need it.

## Properties: namespaced

Properties are JSON keyed by source id at the top level. Each writer owns its own sub-object.

```json
{
  "tree-sitter:0.1.0": {
    "kind": "function",
    "name": "ParseSlug",
    "exported": true,
    "line_start": 40,
    "line_end": 45,
    "params": ["s"],
    "returns": "(SlugID, error)"
  },
  "llm-enricher:1.0": {
    "summary": "Validates a string as a kg slug and returns a typed SlugID, or ErrInvalidSlug.",
    "complexity": "low",
    "side_effects": "none"
  },
  "git:1.0": {
    "last_modified": "2026-05-24T10:30:00Z",
    "author": "alice",
    "commits_touching": 7
  }
}
```

### Writer-side: flat map in, namespaced storage out

Plugins (and CLI) write properties as a flat map. The Service wraps them:

```go
// Plugin/CLI gives:
{"kind": "function", "exported": true}

// Service.SetNodeProperties(ctx, nodeID, source="tree-sitter:0.1.0", props):
//   existing := getProperties(nodeID)  // {"llm-enricher:1.0": {...}}
//   existing["tree-sitter:0.1.0"] = props
//   setProperties(nodeID, existing)
```

`SetNodeProperties` is **replace within namespace** — the new map fully replaces any prior content for that source. To delete a single property, the writer sends the full updated map without it.

`DeleteNodeProperties(ctx, nodeID, source)` removes the entire source-namespace block.

### Reader-side: three views

```sh
# Default: raw namespaced JSON (the storage shape)
kg node get myapp:graph/node-go::parseslug
# → {"properties": {"tree-sitter:0.1.0": {...}, "llm-enricher:1.0": {...}}}

# --source: only one namespace, flattened
kg node get myapp:graph/node-go::parseslug --source tree-sitter:0.1.0
# → {"properties": {"kind": "function", "exported": true, ...}}

# --merged: union of all namespaces, conflicts resolved by trust desc, then source id asc
kg node get myapp:graph/node-go::parseslug --merged
# → {"properties": {"kind": "function", "exported": true, "summary": "...", "last_modified": "..."}, "_property_sources": {"kind": "tree-sitter:0.1.0", "summary": "llm-enricher:1.0", ...}}
```

The merged view includes a sibling `_property_sources` map so consumers can see which source contributed each top-level key. Sub-object merging is shallow only.

### Edge properties

Edge properties follow the same namespaced shape. `kg apply` from a single source writes only into its namespace; edge claims and edge properties are independent (you can claim an edge without adding properties, or contribute properties to an edge you don't claim, though the latter is uncommon).

## Declarative Wire Format

A single JSON document on the plugin's stdout. Not JSONL — declarative is atomic by nature, the whole snapshot must be present before diff can run.

```json
{
  "protocol_version": 2,
  "source": "tree-sitter:0.1.0",
  "domain": "myapp",
  "scope": "domain-source",
  "domain_spec": {
    "id": "myapp",
    "layers": ["package", "file", "decl"],
    "description": "Go source code extracted from internal/graph"
  },
  "nodes": [
    {
      "id": "myapp:graph",
      "layer": "package",
      "name": "graph",
      "properties": {"import_path": "github.com/ggfarmco/kg/internal/graph"}
    },
    {
      "id": "myapp:graph/node-go",
      "layer": "file",
      "parent": "myapp:graph",
      "name": "node.go",
      "properties": {"path": "internal/graph/node.go", "lines": 62}
    },
    {
      "id": "myapp:graph/node-go::parseslug",
      "layer": "decl",
      "parent": "myapp:graph/node-go",
      "name": "ParseSlug",
      "properties": {"kind": "function", "exported": true, "line_start": 40, "line_end": 45}
    }
  ],
  "edges": [
    {"source": "myapp:graph", "target": "myapp:store", "type": "imports"},
    {"source": "myapp:graph/node-go::parseslug", "target": "myapp:graph/node-go::slugre", "type": "calls"}
  ]
}
```

### Top-level fields

- `protocol_version` (required) — set to `2` for declarative. Plugins MUST refuse on unknown values.
- `source` (required) — writer identifier. Must match the plugin's manifested `source_id` if invoked via `kg-extractor`.
- `domain` (required) — target domain id.
- `scope` (required) — one of:
  - `"domain-source"`: I own everything in `domain` with my `source`. Diff against `WHERE domain=? AND source=?`. Most common.
  - `"domain"`: I own the entire domain (no other sources should be writing here). Stronger contract; if other sources have written, `kg apply` errors out unless `--force-domain-takeover` is set. Useful for greenfield domains owned by one plugin.
  - `"additive"`: Don't compute deletions; just add or update what's in the snapshot, leave the rest alone. Useful for enrichers that want to add summaries without claiming exhaustive coverage.
- `domain_spec` (optional) — if present, kg upserts the domain. If absent, the domain must already exist. Plugins should always send this on first run.
- `nodes` (required, may be empty) — list of node specs.
- `edges` (required, may be empty) — list of edge specs.

### Node spec fields

- `id` (required) — must be a valid kg slug (`^[a-z0-9-]+(?:(?:/|::)[a-z0-9-]+)*$` per v1's relaxed grammar).
- `layer` (required) — must exist in the domain's layers.
- `parent` (optional) — required for non-top-layer nodes; must reference a node defined earlier in this snapshot OR an existing node in kg with the same layer-rule constraint as v0.
- `name` (required) — human-readable.
- `properties` (optional) — flat map; kg wraps in the plugin's source namespace.

### Edge spec fields

- `source` (required) — node id (this clashes with the top-level `source: <source-id>`; renaming the top-level field is too churnful, so we accept the visual collision and document clearly).
- `target` (required) — node id.
- `type` (required) — edge type.
- `properties` (optional) — flat map, namespaced like node properties.

(For clarity in the implementation, when parsing the snapshot we'll alias edge `source` to `src` internally to avoid confusion with the writer source id.)

### Ordering

Plugins SHOULD emit nodes in any order they like; `kg apply` topologically sorts by parent reference before applying. Edges may reference nodes defined in `nodes[]` OR pre-existing nodes in kg (the diff machinery validates references after the sort).

### Validation

`kg-extractor` validates the declarative snapshot before forwarding to `kg apply`:
- Valid top-level JSON
- `protocol_version: 2`
- `source` matches manifest's `source_id` (or the value plugin authored if invoked directly)
- `scope` is a known value
- All node ids are valid slugs
- All edges reference nodes (in snapshot or pre-existing) — though full reference validation deferred to `kg apply` because it needs DB access

## `kg apply` verb

```
kg --db ./kg.db apply --source <id> --domain <id> [--scope <scope>] [--dry-run] [--force-cascade] [--force-domain-takeover] [--progress] < snapshot.json
```

### Algorithm

All steps run inside `Store.InTx`:

```
1. Parse snapshot from stdin (JSON document, not JSONL)
2. Validate required fields, scope, slug shapes
3. UPSERT into sources (creates row if missing, bumps last_seen)
4. If snapshot.domain_spec is present:
     INSERT or skip-if-equal into domains
5. Load my current state:
     existing_nodes := SELECT id, name, properties FROM nodes
                       WHERE domain=? AND source=?
     existing_edge_claims := SELECT edge_id FROM edge_claims WHERE source=?
6. Topologically sort snapshot.nodes (parents before children)
7. For each node in snapshot.nodes:
     existing := existing_nodes[node.id]
     if existing is None:
         INSERT (verifying layer-rules, parent existence, owner consistency)
         (or: if a node with this id exists owned by another source → error
              NODE_OWNED_BY_DIFFERENT_SOURCE unless scope="additive")
     else:
         if existing.layer != node.layer or existing.parent != node.parent:
             error CORE_FIELDS_IMMUTABLE
         if existing.name != node.name or properties_differ:
             UPDATE name, properties[source-namespace], bump revision
         delete existing_nodes[node.id]  # mark as seen
8. For each edge in snapshot.edges:
     edge_row := UPSERT into edges  # by (source_id, target_id, type)
     INSERT OR IGNORE INTO edge_claims(edge_row.id, source, now)
     delete existing_edge_claims[edge_row.id]  # mark as seen
9. Cleanup phase (handle scope semantics):
     if scope == "additive":
         skip cleanup (no deletions)
     else:
         # existing_nodes residual = mine, not in snapshot → delete
         for node_id in existing_nodes:
             if node has children OR has incident edges with foreign claims:
                 if --force-cascade:
                     proceed (cascade delete, foreign claims drop with edges)
                 else:
                     error NODE_HAS_DEPENDENTS / NODE_HAS_FOREIGN_CLAIMS
             else:
                 DELETE FROM nodes WHERE id=?
         # existing_edge_claims residual = mine, not in snapshot → unclaim
         for edge_id in existing_edge_claims:
             DELETE FROM edge_claims WHERE edge_id=? AND source=?
             # GC: if no claims remain on this edge, drop the edge
             if (SELECT COUNT(*) FROM edge_claims WHERE edge_id=?) == 0:
                 DELETE FROM edges WHERE id=?
10. If dry_run: rollback via sentinel error (same pattern as kg batch --dry-run)
11. Return envelope
```

### Atomicity

Whole apply is one `Store.InTx`. Failure at any step rolls back everything. There's no `--chunk-size` analog for apply (snapshots are atomic units by definition); for very large snapshots, the cost is held WAL transaction size — same constraint as `kg batch` default mode.

### Conflict handling

Each error class is a distinct exit code (extending the v0 envelope):

- `NODE_OWNED_BY_DIFFERENT_SOURCE` (exit 2): a node with the same id exists owned by another source. Re-extract should never hit this; only happens if two plugins claim the same id. Resolution: change one plugin's id strategy, or use `scope: "additive"`.
- `CORE_FIELDS_IMMUTABLE` (exit 1): snapshot tries to change layer/parent/name of an existing node owned by this source via a different value. Caused by extractor refactor that changed slug computation; resolution is to delete and re-add (manually for now).
- `NODE_HAS_FOREIGN_CLAIMS` (exit 1): cleanup wants to delete a node, but edges incident to it have claims from other sources. Resolution: `--force-cascade` (destructive — drops foreign claims along with edges), or fix the plugin to keep the node alive.
- `NODE_HAS_DEPENDENTS` (exit 1): cleanup wants to delete a node that has child nodes. Resolution: `--force-cascade` or fix the snapshot.
- `DOMAIN_FOREIGN_WRITERS` (exit 1): `scope: "domain"` requested, but the domain contains nodes from other sources. Resolution: `--force-domain-takeover` (destructive), narrow scope to `"domain-source"`, or coordinate writers.

### Envelope

Success:

```json
{
  "ok": true,
  "data": {
    "source": "tree-sitter:0.1.0",
    "domain": "myapp",
    "scope": "domain-source",
    "nodes_added": 12,
    "nodes_updated": 105,
    "nodes_removed": 3,
    "edges_added": 24,
    "claims_added": 24,
    "claims_removed": 8,
    "edges_gc": 5,
    "took_ms": 234
  }
}
```

Dry-run:

```json
{
  "ok": true,
  "data": {
    "dry_run": true,
    "would_add_nodes": 12,
    "would_update_nodes": 105,
    "would_remove_nodes": 3,
    "would_add_claims": 24,
    "would_remove_claims": 8,
    "would_gc_edges": 5
  }
}
```

Conflict:

```json
{
  "ok": false,
  "error": {
    "code": "NODE_HAS_FOREIGN_CLAIMS",
    "message": "cleanup would remove 'myapp:auth' but its edges have claims from llm-enricher:1.0",
    "hint": "re-run with --force-cascade to drop foreign claims, or fix the snapshot to keep this node"
  },
  "blocking": [
    {"node": "myapp:auth", "foreign_claim_edges": [42, 87]}
  ]
}
```

### Flags

- `--source <id>` (required) — writer identifier; must match snapshot's `source` field.
- `--domain <id>` (required) — target domain; must match snapshot's `domain` field.
- `--scope <s>` (optional) — overrides snapshot's `scope`. Useful for testing.
- `--dry-run` — compute diff, report counts, rollback.
- `--force-cascade` — allow cleanup to drop nodes that have children or foreign-claimed incident edges.
- `--force-domain-takeover` — allow `scope: "domain"` even if other sources have written here.
- `--progress` — emit `applied N/total` to stderr every ~100ms (same shape as `kg batch --progress`).

## `kg batch` (imperative) updates

`kg batch` survives as the low-level wire protocol. Changes from v1:

### Source field required on mutation ops

Every mutation op gains a required `source` field in `args`:

```jsonl
{"op":"node.add",   "args":{"source":"tree-sitter:0.1.0", "domain":"myapp", "layer":"package", "name":"graph", "id":"graph"}}
{"op":"node.update","args":{"source":"llm-enricher:1.0",   "id":"myapp:graph", "properties":{"summary":"..."}}}
{"op":"node.delete","args":{"source":"tree-sitter:0.1.0", "id":"myapp:graph"}}
{"op":"edge.add",   "args":{"source":"tree-sitter:0.1.0", "src":"myapp:a", "target":"myapp:b", "type":"imports"}}
{"op":"edge.delete","args":{"source":"tree-sitter:0.1.0", "id":42}}
```

Note: `edge.add` renames the wire field `source` (the originating node) to `src` to avoid colliding with the new writer-source field. Both `src` and `target` are required for edge identification.

### Op semantics changes

- `node.add`: as before, with required `source`. If a node with this id exists, behavior depends on `if_not_exists`:
  - Without flag: `NODE_OWNED_BY_DIFFERENT_SOURCE` if different owner, else `NODE_ALREADY_EXISTS`.
  - With flag: silently skipped (counts as `skipped`).
- `node.update`: now writes only into the `source` namespace of properties. If the writer is not the node's owner, only `properties` can be updated, not `name`. Updating name as non-owner → `NODE_NOT_OWNER`.
- `node.delete`: only the owner can delete. Foreign sources get `NODE_NOT_OWNER`. The owner gets the same cascade protection as `kg apply` (cannot delete if foreign claims exist on edges, unless `--force` flag on batch).
- `edge.add`: adds the edge + claim. Idempotent for the `(src, tgt, type, source)` quadruple.
- `edge.delete` (deprecated, kept for compatibility): removes ALL claims for the writer source on this edge id. If no claims remain, GC's the edge. Equivalent to `edge.unclaim`.
- `edge.unclaim` (new, recommended): explicit claim removal. Same semantics as `edge.delete` but reads clearly in scripts.

### Cross-source ops in one batch

`kg batch` allows mixed sources in one stream — the `source` field is per-op, not per-batch. This is useful for migration scripts and manual edits. There's no enforcement that ops from one source must precede another.

### Flags (unchanged from v1)

`--continue-on-error`, `--chunk-size N`, `--dry-run`, `--progress` work as in v1. The new `source` field is purely additive to the args.

## Plugin contract evolution

### Manifest fields (new)

```json
{
  "name": "tree-sitter",
  "version": "0.1.0",
  "description": "Tree-sitter Go structural extractor",
  "source_id": "tree-sitter:0.1.0",
  "trust": 100,
  "runtime": "declarative-native",
  "executable": "kg-extractor-tree-sitter",
  "supported_layers": ["package", "file", "decl"],
  "supported_languages": ["go"],
  "supported_scopes": ["domain-source"]
}
```

New fields:
- `source_id` (optional, default `"<name>:<version>"`) — what writer id this plugin uses.
- `trust` (optional, default 100) — initial trust score; ignored after first registration.
- `supported_scopes` (optional, default `["domain-source"]`) — which scopes this plugin can emit.

### Runtimes (extended)

- `native` / `command` — imperative JSONL ops (unchanged from v1, low-level).
- `declarative-native` / `declarative-command` — single JSON snapshot on stdout (v2 recommended).
- `wasm` / `declarative-wasm` — reserved for v3+.

`kg-extractor` reads the manifest's `runtime` and dispatches accordingly:

```
runtime is imperative (native/command):
    plugin stdout = JSONL ops
    kg-extractor validates per-op
    pipes to `kg --db <path> batch --source <plugin.source_id>` (when --db set)
    or to stdout (pass-through)

runtime is declarative-* :
    plugin stdout = one JSON snapshot
    kg-extractor validates snapshot shape
    pipes to `kg --db <path> apply --source <plugin.source_id> --domain <user-flag>` (when --db set)
    or to stdout (pass-through)
```

Note: the `--source` passed to kg apply/batch comes from the plugin's manifest, NOT a user flag. This prevents accidentally overwriting another source's content. Users can override only by editing the manifest.

### Plugin protocol_version

Imperative plugins use `protocol_version: 1` in their stdin config (unchanged from v1).

Declarative plugins use `protocol_version: 2` in BOTH the stdin config and the output snapshot. The bump is to signal the contract change explicitly; v1 plugins continue to work unmodified because they never look at the bumped number for declarative ops.

## CLI surface

New and changed commands:

### Sources

```sh
kg sources list                                              # all known sources
kg sources show <id>                                         # one source
kg sources register --id <id> [--description ...] [--trust N] [--if-not-exists]
kg sources update <id> [--description ...] [--trust N]
kg sources delete <id>                                       # only if no owned entities
```

### Nodes (changed)

```sh
# add gets --source (default 'cli')
kg node add --domain <id> --layer <name> --name <text> \
            [--source <id>] [--id <slug>] [--parent <node-id>] \
            [--summary <text>] [--properties '<json>'] \
            [--if-not-exists] [--dry-run]

# get supports view modes
kg node get <node-id>                       # raw namespaced
kg node get <node-id> --source <id>         # one namespace, flattened
kg node get <node-id> --merged              # trust-ranked union with _property_sources

# list filters by source
kg node list [--domain <id>] [--layer <name>] [--source <id>] [--limit N]

# update writes into caller's source namespace
kg node update <node-id> [--source <id>] [--name ...] [--summary ...] \
              [--properties '<json>'] [--dry-run]

# delete fails if not owner or has foreign claims; --force-cascade overrides
kg node delete <node-id> [--source <id>] [--force-cascade]
```

### Edges (changed)

```sh
kg edge add <src> <tgt> --type <name> [--source <id>] [--if-not-exists] [--dry-run]
kg edge list-from <node-id> [--type <name>] [--source <id>]
kg edge list-to <node-id> [--type <name>] [--source <id>]
kg edge claims <edge-id>                    # who claims this edge
kg edge unclaim <edge-id> [--source <id>]   # remove caller's claim; GC if last
kg edge delete <edge-id> [--force]          # force-remove all claims and edge
```

### Apply

```sh
kg apply --source <id> --domain <id> \
         [--scope <s>] [--dry-run] [--force-cascade] \
         [--force-domain-takeover] [--progress] < snapshot.json
```

### Batch (unchanged shape, source added per-op)

```sh
kg batch [--chunk-size N] [--continue-on-error] [--dry-run] [--progress] < ops.jsonl
```

## kg-extractor changes

Two new responsibilities on top of v1:

1. **Dispatch by runtime kind.** Read `manifest.runtime`; for declarative-* runtimes, capture single JSON snapshot from stdout (not JSONL); validate snapshot shape; pipe to `kg apply` (not `kg batch`).

2. **Source binding.** Pass `--source <manifest.source_id>` to whichever kg verb is invoked. Users do not override this — preventing accidental cross-source corruption.

The list/info/extract subcommand structure is unchanged. `extract` grows a `--snapshot` flag to invoke a plugin and emit the raw snapshot to stdout (skipping forwarding), useful for inspection and debugging.

## Project layout (delta from v1)

```
ggfarmco/kg/
├── batch/                                 # MOSTLY UNCHANGED
│   ├── op.go                              # +source field on each Args struct
│   ├── codec.go                           # unchanged
│   └── *_test.go                          # updated to include source field
├── snapshot/                              # NEW public package for declarative wire format
│   ├── snapshot.go                        # Snapshot, NodeSpec, EdgeSpec, scope types
│   ├── codec.go                           # JSON encode/decode helpers
│   ├── validate.go                        # shape validation
│   └── *_test.go
├── cmd/kg/
│   ├── apply_cmd.go                       # NEW: kg apply
│   ├── batch_cmd.go                       # source field handling
│   ├── sources_cmds.go                    # NEW: kg sources subcommand
│   ├── node_cmds.go                       # +--source, +--merged, +--properties
│   ├── edge_cmds.go                       # +--source, +claims/unclaim
│   ├── ... (others updated)
├── cmd/kg-extractor/
│   ├── ... (existing files)
│   ├── declarative.go                     # NEW: declarative runtime dispatch
│   └── snapshot_validator.go              # NEW: validates declarative snapshots
├── internal/graph/
│   ├── source.go                          # NEW: SourceID type, Source struct
│   ├── domain.go                          # unchanged
│   ├── node.go                            # +Source field, Properties as map[string]map[string]any
│   ├── edge.go                            # +Claims []SourceID
│   ├── service.go                         # source-aware: AddNode/Update/Delete, Apply
│   ├── service_apply.go                   # NEW: Service.Apply (declarative diff+apply)
│   └── *_test.go                          # updated
├── internal/store/
│   ├── queries.sql                        # rewritten for new schema
│   ├── ... (sqlc regenerated)
│   ├── sources.go                         # NEW: source registry CRUD
│   ├── edge_claims.go                     # NEW: claim management + GC
│   └── *_test.go
├── plugins/tree-sitter/                   # SIGNIFICANTLY UPDATED
│   ├── (existing files)
│   ├── snapshot.go                        # NEW: emit snapshot/* shape instead of batch ops
│   └── ... (manifest gets source_id, runtime → declarative-native)
├── examples/kg-extractor-plugins/bash-demo/
│   ├── manifest.json                      # runtime → declarative-command
│   ├── extract.sh                         # rewritten to emit JSON snapshot
│   └── README.md                          # updated
├── migrations/
│   └── 0001_init.sql                      # FULL REWRITE — no migration 0002, no data
└── docs/superpowers/specs/
    ├── 2026-05-23-kg-mvp-design.md        # v0 (historical)
    ├── 2026-05-23-kg-v1-extractor-design.md # v1 (historical, foundation)
    └── 2026-05-24-kg-v2-provenance-design.md # this spec
```

The `batch/` package stays public (unchanged role). A new `snapshot/` package joins it — also public — because both plugins and consumers need the snapshot shape. Plugins in separate Go modules import both.

## Testing strategy

Three tiers (unchanged structure, expanded coverage):

### Tier 1 — unit

- **`batch/`**: codec round-trips with source field; per-op argument validation.
- **`snapshot/`**: snapshot round-trips; topological sort of nodes; scope semantics validation.
- **`internal/graph/`**: source-aware Service methods via `FakeStore`; namespaced property merging; claim ref-counting logic.
- **`cmd/kg/`**: op router with sources; apply algorithm against `FakeStore`; sources subcommand.

### Tier 2 — integration

- **`kg apply` happy path**: snapshot → DB has expected nodes/edges/claims.
- **Re-apply same snapshot**: zero changes (idempotent).
- **Re-apply modified snapshot**: diff produces correct adds/updates/deletes.
- **Multi-source coexistence**:
  - Source A adds nodes with properties. Source B adds properties to same nodes.
  - Source A re-applies (removing some properties from its namespace). Source B's properties survive.
  - Source A adds edges. Source B claims same edges. Source A unclaims. Edges survive.
  - Source A's last claim on an edge → edge GC'd, source B's properties (if any) were on a now-deleted edge — that's fine, also gone.
- **Scope enforcement**:
  - `scope: "domain"` with foreign writers → error.
  - `scope: "additive"` skips cleanup entirely.
- **Conflict cases**:
  - Same node id, different source → `NODE_OWNED_BY_DIFFERENT_SOURCE`.
  - Apply tries to delete node with foreign claims on incident edges → `NODE_HAS_FOREIGN_CLAIMS`.
  - `--force-cascade` overrides cleanly.
- **kg batch with source**: every op carries source; sources auto-register.

### Tier 3 — e2e

Update `e2e/extract_self_test.go`:
- First pass: extract `internal/graph` via tree-sitter declarative. Assert nodes/edges as before, plus assert all nodes carry `source = tree-sitter:0.1.0`.
- Second pass: extract again with no code changes. Assert envelope shows `nodes_added: 0, nodes_updated: 0, nodes_removed: 0` (idempotent).
- Third pass: simulate code change (rename a function in a tmp copy). Assert `nodes_removed: 1, nodes_added: 1` (the rename).
- Fourth pass: add a node manually via `kg node add --source manual`. Re-run tree-sitter. Assert the manual node survives.

### Golden tests update

`plugins/tree-sitter/languages/golang/testdata/golden/*/expected.jsonl` becomes `expected.snapshot.json` — same shape transition, snapshot format. Test runner diffs JSON structures (with field ordering normalized), not raw byte equality.

### What we don't test

Same as v1: tree-sitter library internals, cobra framework internals, SQLite driver.

## Open Risks

| Risk | Mitigation |
|---|---|
| Single-source for nodes blocks legitimate multi-writer scenarios | Documented limitation; future v3 may add `node_claims`. Current escape hatches: shared `manual` source via CLI, or `scope: "additive"` to extend without claiming. |
| Edge claim GC in same tx can slow huge apply transactions | Cleanup phase scans `existing_*` residuals; for snapshots with 100k+ nodes this is one extra SELECT per residual. Profile in v2.1 if it becomes painful. |
| `--force-cascade` is too easy to type | Default is RESTRICT; force is opt-in; envelope shows what would be cascaded in dry-run. Document loudly in README. |
| Plugin's `source_id` collision (two plugins declare `tree-sitter:0.1.0`) | First-write wins; second plugin's writes will be attributed to the registered description. CLI to inspect/disambiguate. Recommend `<org>/<name>:<version>` in docs. |
| Property merge view (`--merged`) hides which source contributed which key | `_property_sources` sibling map shows attribution per key. |
| Snapshot for very large domains held in memory | Same constraint as v1 buffered mode; v3 may add streaming snapshot for huge cases. |
| `kg batch` with mixed sources is hard to reason about | Documented as "for power users"; recommend declarative `kg apply` for plugin work, imperative `kg batch` for tests/scripts. |
| Re-extract that hits `NODE_HAS_FOREIGN_CLAIMS` is non-obvious to debug | Error envelope includes `blocking` list with edge ids; hint suggests `--force-cascade` and what it would destroy. CLI `kg edge claims <id>` shows current state. |
| `protocol_version: 2` for declarative might collide with future v3 protocol_version: 2 imperative redesign | Reserve `protocol_version >= 100` for non-incremental redesigns; 1-99 for additive imperative versions. Documented in `batch/op.go`. |

## Implementation plan

To be authored next via the `writing-plans` skill. The plan will sequence (approximately 25-30 tasks across ~7 phases):

**Phase 1 — Schema and store rewrite**
1. Rewrite `migrations/0001_init.sql` with sources, edge_claims, namespaced properties shape.
2. `internal/store/sources.go` — source registry CRUD with auto-register.
3. Update `internal/store/queries.sql` for new node/edge shape; regenerate sqlc.
4. `internal/store/edge_claims.go` — claim CRUD with GC in same tx.

**Phase 2 — Graph core updates**
5. `internal/graph/source.go` — `SourceID`, `Source` type, sentinel errors.
6. `internal/graph/node.go` + `edge.go` — add Source/Claims fields, namespaced Properties type.
7. `internal/graph/service.go` — source-aware AddNode/UpdateNode/DeleteNode with ownership checks.
8. Service methods for property namespace management (Get/Set/Delete per source).
9. Service methods for edge claims (AddClaim, RemoveClaim, ListClaims).

**Phase 3 — Snapshot package**
10. `snapshot/` public package: types, codec, validation.
11. Topological sort helper for node spec lists.

**Phase 4 — Service.Apply (the diff engine)**
12. `internal/graph/service_apply.go` — load current state, compute diff, apply, cleanup.
13. Scope semantics: domain-source, domain, additive.
14. Conflict detection (NODE_OWNED_BY_DIFFERENT_SOURCE, NODE_HAS_FOREIGN_CLAIMS, CORE_FIELDS_IMMUTABLE, DOMAIN_FOREIGN_WRITERS).
15. `--force-cascade` and `--force-domain-takeover` handling.

**Phase 5 — CLI**
16. `cmd/kg/sources_cmds.go` — list/show/register/update/delete.
17. `cmd/kg/apply_cmd.go` — new verb.
18. Update `node_cmds.go` / `edge_cmds.go` / `batch_cmd.go` for `--source` and new flags.
19. `--merged` / `--source <id>` view modes for `kg node get` / `kg edge list-*`.

**Phase 6 — kg-extractor declarative runtime**
20. Manifest parser updates (`source_id`, `trust`, declarative-* runtimes).
21. Snapshot validator (`cmd/kg-extractor/snapshot_validator.go`).
22. Declarative dispatch in `extract_cmd.go` (snapshot collection, pipe to `kg apply`).
23. Update integration test (bash-demo declarative end-to-end).

**Phase 7 — Plugins**
24. Rewrite `examples/kg-extractor-plugins/bash-demo/` to declarative-command + snapshot output.
25. Switch `plugins/tree-sitter/` to declarative-native: replace `emit.go` (batch ops) with `snapshot.go` (single JSON document).
26. Update lang_go_adapter to populate snapshot shape directly.

**Phase 8 — Tests and polish**
27. Rewrite `e2e/extract_self_test.go` with idempotency + foreign claim survival assertions.
28. Convert golden fixtures (`expected.jsonl` → `expected.snapshot.json`) with structural diff comparison.
29. Update Makefile (no new targets, but `make e2e` should still work).
30. README extractor section update (declarative vs imperative, sources concept).

Estimated scope: similar in size to v1 (~5,000-7,000 lines of plan), but with several mostly-mechanical phases (CLI surface updates) and a couple of higher-judgment ones (Service.Apply algorithm, conflict semantics).
