# kg — MVP Design Spec

**Date:** 2026-05-23
**Status:** Approved for implementation planning
**Module:** `github.com/ggfarmco/kg`
**Location:** `~/develop/norimori/ggfarmco/kg/`

## Purpose

A domain-agnostic knowledge graph engine in Go with SQLite storage. The graph structure
is decoupled from the knowledge domain: the same engine supports codebases, system
architectures, learning topics, business processes — any structured knowledge.

Each domain defines its own ordered chain of abstraction layers (e.g. `cars: [system,
subsystem, part]` or `code: [arch, service, package, function]`). Cross-domain edges
are first-class, enabling links like `cars:engine → physics:thermo-law (governed_by)`.

MVP scope: core graph + CLI. No LLM integration, no extractors, no HTTP/MCP server.
Those layers will be added in subsequent iterations on top of the foundation defined
here.

## Goals

1. Validate the generic graph + per-domain layers model against multiple realistic
   domains (code, cars, business, physics, ...).
2. Establish a clean Hexagonal-ish layout (`graph` core / `store` adapter / `cmd/kg`
   CLI) that supports adding extractors, LLM enrichment, and network APIs later
   without rewriting the foundation.
3. Bake versioning and a change log into the schema from day one so collaboration
   (sync, optimistic locking) can be added later without backfilling existing data.
4. LLM-friendly CLI surface (cobra, always-JSON envelope, idempotent flags, structured
   errors) so the same CLI can be driven by humans and by LLM agents.

## Non-Goals (deferred to v2+)

- LLM-based extraction or enrichment.
- Embeddings, vector search, semantic dedup.
- Domain-specific extractors (tree-sitter, markdown, ...).
- HTTP / MCP server.
- Multi-hop graph traversal (only depth-1 neighbors via `node children` and
  `edge list-from / list-to`).
- Edge-rules validation (any edge type is allowed between any pair of nodes).
- `--if-rev N` optimistic-locking flag on update (revision is tracked in the DB and
  exposed in JSON output; clients can already start carrying it).
- Soft deletes / tombstones (CASCADE delete; the change log records deletes).
- Interactive REPL, bulk import, shell completions, watch mode.

## Architecture

Single binary CLI. Three layers:

```
cmd/kg/         # cobra CLI; thin adapter
   ↓
internal/graph  # pure Go: types, Store interface, Service (use cases + validation)
   ↓
internal/store  # SQLite implementation of graph.Store via modernc.org/sqlite + sqlc
```

Boundaries:

- `internal/graph` knows nothing about SQL or CLI.
- `internal/store` knows about SQLite but not about CLI commands.
- `cmd/kg` knows about both but never touches SQL directly.

## Database Schema

Four tables. SQLite via `modernc.org/sqlite` (pure Go, no CGO). Migrations via
`pressly/goose`.

### domains

```sql
CREATE TABLE domains (
  id          TEXT PRIMARY KEY,            -- slug: "cars", "physics", "my-app"
  description TEXT,
  layers      TEXT NOT NULL,               -- JSON array, ordered top→bottom
  revision    INTEGER NOT NULL DEFAULT 1,  -- bumps on UPDATE
  created_at  INTEGER NOT NULL             -- unix ms
);
```

### nodes

```sql
CREATE TABLE nodes (
  id          TEXT PRIMARY KEY,            -- "domain:slug", e.g. "cars:engine"
  domain      TEXT NOT NULL REFERENCES domains(id) ON DELETE RESTRICT,
  layer       TEXT NOT NULL,               -- must be present in domains.layers
  name        TEXT NOT NULL,               -- human-readable, unicode-friendly
  parent_id   TEXT REFERENCES nodes(id) ON DELETE SET NULL,
  summary     TEXT,
  properties  TEXT NOT NULL DEFAULT '{}',  -- opaque JSON (no CLI surface in MVP)
  revision    INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);

CREATE INDEX idx_nodes_domain_layer ON nodes(domain, layer);
CREATE INDEX idx_nodes_parent       ON nodes(parent_id);
```

### edges

```sql
CREATE TABLE edges (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id   TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  target_id   TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  type        TEXT NOT NULL,               -- "depends_on" | "governed_by" | ...
  properties  TEXT NOT NULL DEFAULT '{}',
  revision    INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL,
  UNIQUE(source_id, target_id, type)
);

CREATE INDEX idx_edges_source ON edges(source_id, type);
CREATE INDEX idx_edges_target ON edges(target_id, type);
```

### changes (CDC log)

```sql
CREATE TABLE changes (
  seq         INTEGER PRIMARY KEY AUTOINCREMENT,
  entity      TEXT NOT NULL,               -- "domain" | "node" | "edge"
  entity_id   TEXT NOT NULL,               -- string form of the entity ID
  op          TEXT NOT NULL,               -- "create" | "update" | "delete"
  revision    INTEGER,                     -- post-op revision (NULL for delete)
  at          INTEGER NOT NULL             -- unix ms
);

CREATE INDEX idx_changes_seq    ON changes(seq);
CREATE INDEX idx_changes_entity ON changes(entity, entity_id);
```

### Schema invariants

- `parent_id` is the single source of truth for containment hierarchy.
- All other relationships (including cross-domain) live exclusively in `edges`.
- `UNIQUE(source_id, target_id, type)` provides DB-level edge dedup.
- `revision` is bumped on UPDATE in the same transaction as the mutation.
- Each successful mutation writes exactly one row to `changes` in the same
  transaction.
- `seq` is strictly monotonic. `AUTOINCREMENT` is used (not bare
  `INTEGER PRIMARY KEY`) so that DELETEs in the table do not allow `seq` reuse.
- `changes.at` is unix milliseconds.

### ON DELETE policies

- `domains → nodes`: RESTRICT — a domain cannot be dropped while it has nodes.
- `nodes → parent_id`: RESTRICT — a node cannot be deleted while it has children.
  The user must remove children first, or use a future `--cascade` flag. RESTRICT
  was chosen over SET NULL to keep the "non-top-layer must have a parent" invariant
  true at both write and read time; otherwise orphan nodes could exist that
  `AddNode` would have rejected.
- `nodes → edges`: CASCADE — edges are removed automatically when either endpoint
  is deleted. Edges carry no independent meaning without their endpoints.

## Validation rules

Enforced in `graph.Service`, not in CLI. CLI maps sentinel errors to the JSON
envelope.

### `AddNode`

1. `domain` exists.
2. `layer` is present in `domain.layers`.
3. If `--id` is provided: must match `^[a-z0-9-]+$`.
4. If `--id` is not provided: auto-derive slug from `name` (lowercase, spaces → `-`).
   If the result does not match `^[a-z0-9-]+$` → `ErrSlugCannotDerive`. No silent
   stripping of characters.
5. If `parent_id` is provided:
   - The parent exists.
   - `parent.domain == node.domain`.
   - `parent.layer` is exactly one position above `node.layer` in `domain.layers`.
6. If the node's layer is the top layer of the domain, `parent_id` must be nil.

### `AddEdge`

1. Both endpoints exist.
2. `source_id != target_id` (no self-loops in MVP).
3. UNIQUE `(source_id, target_id, type)` enforced at the DB level.

### Cross-domain edges

Any edge type, any direction, any cross-domain combination is allowed in MVP.
Edge-rules validation will be added when real patterns demand it.

### Immutable IDs

Node, domain, and edge IDs are immutable. There is no rename operation in MVP. If
needed later, it would ship as a separate `kg node rename` command with explicit
re-pointing semantics.

## Identifiers

Typed Go aliases prevent slug / node-id mixups at compile time:

- `DomainID string` — domain slug, e.g. `"cars"`.
- `SlugID string` — local slug, e.g. `"engine"`.
- `NodeID string` — composite, e.g. `"cars:engine"`.
- `EdgeID int64` — surrogate key.

Constructors and parsers:

- `NewNodeID(d DomainID, s SlugID) NodeID`
- `(NodeID).Split() (DomainID, SlugID, error)`
- `ParseSlug(s string) (SlugID, error)`
- `ParseDomainID(s string) (DomainID, error)`

ID conflicts on create raise `ErrNodeAlreadyExists` / `ErrDomainAlreadyExists`. The
`--if-not-exists` CLI flag translates these into silent skip with exit 0.

## Go domain model

### Types

```go
type Domain struct {
    ID          DomainID
    Description string
    Layers      []string
    Revision    int64
    CreatedAt   time.Time
}

type Node struct {
    ID         NodeID
    Domain     DomainID
    Layer      string
    Name       string
    ParentID   *NodeID
    Summary    string
    Properties map[string]any
    Revision   int64
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

type Edge struct {
    ID         EdgeID
    SourceID   NodeID
    TargetID   NodeID
    Type       string
    Properties map[string]any
    Revision   int64
    CreatedAt  time.Time
}

type NodeFilter struct {
    Domain DomainID // zero value → any
    Layer  string   // zero value → any
    Limit  int      // 0 → no limit
}
```

### `Store` interface

```go
type Store interface {
    InTx(ctx context.Context, fn func(ctx context.Context) error) error

    CreateDomain(ctx context.Context, d Domain) error
    GetDomain(ctx context.Context, id DomainID) (*Domain, error)
    ListDomains(ctx context.Context) ([]Domain, error)
    DeleteDomain(ctx context.Context, id DomainID) error

    CreateNode(ctx context.Context, n Node) error
    GetNode(ctx context.Context, id NodeID) (*Node, error)
    ListNodes(ctx context.Context, filter NodeFilter) ([]Node, error)
    UpdateNode(ctx context.Context, n Node) error
    DeleteNode(ctx context.Context, id NodeID) error
    ChildrenOf(ctx context.Context, parentID NodeID) ([]Node, error)

    CreateEdge(ctx context.Context, e Edge) error
    DeleteEdge(ctx context.Context, id EdgeID) error
    EdgesFrom(ctx context.Context, sourceID NodeID, types []string) ([]Edge, error)
    EdgesTo(ctx context.Context, targetID NodeID, types []string) ([]Edge, error)
}
```

#### `InTx` contract

- The active transaction is stored in `context.Value` under a private key.
- All `Store` methods read the tx from ctx if present; otherwise they use the
  connection pool directly.
- If `fn` returns an error, the implementation calls `Rollback` and returns the
  original error.
- Nested `InTx` returns an error in MVP (no save points).
- A panic inside `fn` is recovered, the tx is rolled back, and the panic is
  re-raised.

### Sentinel errors (`internal/graph/errors.go`)

```
ErrDomainNotFound
ErrDomainAlreadyExists
ErrLayerNotInDomain
ErrInvalidSlug
ErrSlugCannotDerive
ErrNodeNotFound
ErrNodeAlreadyExists
ErrParentDomainMismatch
ErrParentLayerMismatch
ErrTopLayerCannotHaveParent
ErrEdgeSelfLoop
ErrEdgeAlreadyExists
```

CLI consumers identify them via `errors.Is`.

## CLI surface

`cobra`-based. Commands follow `noun verb`. Always JSON output. Stable exit codes.

### Global flags

- `--db PATH` (env: `KG_DB`, default `./kg.db`).

### Commands

```
kg init

kg domain add <id> --layers <l1,l2,l3> [--description <text>] [--if-not-exists] [--dry-run]
kg domain list
kg domain get <id>
kg domain delete <id>

kg node add --domain <id> --layer <name> --name <text> \
            [--id <slug>] [--parent <node-id>] [--summary <text>] \
            [--if-not-exists] [--dry-run]
kg node get <node-id>
kg node list [--domain <id>] [--layer <name>] [--limit N]   # --limit 0 (default) = no limit
kg node children <node-id>
kg node update <node-id> [--name ...] [--summary ...] [--dry-run]
kg node delete <node-id>

kg edge add <source-id> <target-id> --type <name> [--if-not-exists] [--dry-run]
kg edge list-from <node-id> [--type <name>]
kg edge list-to <node-id> [--type <name>]
kg edge delete <edge-id>

kg --help
kg --help --json   # machine-readable command tree for LLM introspection
```

### JSON envelope

Success:

```json
{"ok": true, "data": <result>}
```

Failure:

```json
{
  "ok": false,
  "error": {
    "code": "DOMAIN_NOT_FOUND",
    "message": "domain 'cras' does not exist",
    "hint": "did you mean 'cars'? run `kg domain list`"
  }
}
```

### Exit codes

| Code | Meaning                                            |
|------|----------------------------------------------------|
| 0    | Success                                            |
| 1    | Validation error (bad slug, layer mismatch, ...)   |
| 2    | Conflict (already exists)                          |
| 3    | Not found                                          |
| 10   | Internal error (DB unreachable, migration failed)  |

### `--if-not-exists` semantics

Implemented in the CLI layer. It intercepts `ErrXxxAlreadyExists` from the Service
and translates the result into `{"ok": true, "data": {"skipped": true, "reason":
"already_exists"}}` with exit 0. The Service itself never sees this flag — the
intent is a user-facing convenience, not a domain invariant.

### `--dry-run` semantics

Runs all validation against the current DB state but does not commit. On success
returns `{"ok": true, "data": {"dry_run": true}}` with no simulated payload — the
Service is invoked inside `InTx` which rolls back at the end. If the future need
arises to return the would-be record, the contract will extend additively (new
keys under `data`). Useful for LLM agents to validate intent before committing in
long automation chains.

## Project layout

```
ggfarmco/kg/
├── cmd/kg/
│   ├── main.go
│   ├── domain_cmds.go
│   ├── node_cmds.go
│   ├── edge_cmds.go
│   ├── init_cmd.go
│   ├── output.go            # JSON envelope helpers
│   └── errmap.go            # graph.Err* → (code, message, hint, exit)
├── internal/
│   ├── graph/
│   │   ├── domain.go        # Domain + DomainID + ParseDomainID
│   │   ├── node.go          # Node + NodeID/SlugID + NodeFilter + parsers
│   │   ├── edge.go          # Edge + EdgeID
│   │   ├── store.go         # Store interface
│   │   ├── service.go       # Service: use cases + validation
│   │   ├── errors.go        # sentinel errors
│   │   └── *_test.go
│   └── store/
│       ├── store.go         # constructor, Open(path), runs migrations
│       ├── tx.go            # InTx + txFromCtx helpers
│       ├── queries.sql      # sqlc source
│       ├── queries.sql.go   # sqlc-generated, committed
│       ├── models.sql.go    # sqlc-generated, committed
│       ├── db.sql.go        # sqlc-generated, committed
│       └── store_test.go    # integration tests via `:memory:`
├── migrations/
│   └── 0001_init.sql        # all four tables + indexes
├── sqlc.yaml
├── tools.go                 # pin dev tool versions (goose, sqlc)
├── go.mod
├── go.sum
├── Makefile                 # build, test, gen, migrate, lint, install
├── README.md
├── .gitignore               # bin/, kg.db, *.db-wal, *.db-shm
└── .golangci.yml            # defaults, tightened later
```

## Tooling

- Go 1.26.
- `github.com/spf13/cobra` — CLI framework. Chosen over `urfave/cli/v3` for
  LLM-friendly help output that matches `kubectl` / `docker` / `gh` conventions.
- `modernc.org/sqlite` — pure-Go SQLite driver (no CGO; cross-compiles to a single
  binary).
- `github.com/sqlc-dev/sqlc` — typed SQL queries generated from `.sql` source.
- `github.com/pressly/goose/v3` — migrations.
- `github.com/stretchr/testify` — `require` for ergonomic test assertions.
- `golangci-lint` — linting; defaults for MVP, tightened later.
- `Makefile` targets: `build`, `test`, `gen`, `migrate`, `lint`, `install`, `clean`.

## Error handling

Layers:

1. **Store** maps low-level SQLite errors to graph sentinels:
   - `SQLITE_CONSTRAINT_UNIQUE` → `ErrNodeAlreadyExists` / `ErrEdgeAlreadyExists` /
     `ErrDomainAlreadyExists` (depending on the affected constraint).
   - `sql.ErrNoRows` → the appropriate `*NotFound` sentinel.
   - Other errors wrapped: `fmt.Errorf("sqlite: <op>: %w", err)`.
2. **Service** returns sentinels directly for validation failures. Wrapping with
   `%w` is allowed but not required; CLI uses `errors.Is`.
3. **CLI** translates sentinels to `(exitCode, errorCode, hint)` via a table in
   `cmd/kg/errmap.go`. This is the single source of user-facing text.

### Transactions

- If `fn` inside `InTx` returns any error, the implementation calls `Rollback` and
  returns the original error.
- A failed `Commit` returns `fmt.Errorf("commit: %w", err)` (treated as internal).
- A panic inside `fn` is recovered, the tx is rolled back, and the panic is
  re-raised.

## Testing

Three tiers:

1. **Unit (`internal/graph/`)**. In-memory fake `Store` in `internal/graph/testutil`.
   One test per validation rule (per sentinel error). `ParseSlug`, `ParseDomainID`,
   `NodeID.Split` are table-driven.
2. **Integration (`internal/store/`)**. Real SQLite via `:memory:`. A
   `storetest.OpenTestDB(t)` helper opens a fresh DB and registers cleanup. Covers:
   - Happy paths for every `Store` method.
   - Constraint enforcement (UNIQUE, FK).
   - `InTx` rollback / commit / visibility.
   - `revision` increments correctly on every `Update*`.
   - `changes` log has exactly one row per successful mutation and zero rows when a
     tx rolls back.
   - `changes.seq` is strictly monotonic across all mutations including across
     deletes.
3. **End-to-end (`cmd/kg/`)**. In-process via `cobra.Command.Execute()` with
   captured stdout. 5–10 smoke tests covering the user-facing walkthrough:
   `domain add` → `node add` → child `node add` → `edge add` → `node list` →
   `node children` → JSON envelope shape verification.

Not tested: sqlc-generated code, cobra framework internals, the SQLite driver.

## Versioning & collaboration foundation

- **Per-object `revision`** on `domains`, `nodes`, `edges`. Bumped on UPDATE within
  the mutation's transaction. Exposed in JSON output. CLI does not yet support
  `--if-rev N` optimistic-lock flag — clients can carry `revision` for future use.
- **Global `changes` log** with monotonic `seq`. Every successful mutation (create,
  update, delete) appends exactly one row. DELETE operations are tracked with
  `revision = NULL`. Indexes support both stream consumption (`seq > X`) and
  per-entity history lookup.
- `ChangesSince(seq)` query API is intentionally not exposed in MVP. The data
  foundation is in place to add it without backfill.

## Open risks & mitigations

| Risk                                                              | Mitigation                                                                                                       |
|-------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------|
| `properties` column ships unused                                  | Schema-forward-compatible; default `'{}'`; surfaces when first extractor lands.                                  |
| `revision` / `changes` ship without consumers                     | Foundation only; cost is one column + one table with zero query overhead until used.                             |
| sqlc-generated files cause PR churn                               | Committed; reviewers focus on `.sql` source, treat generated as derived artefact.                                 |
| Cross-domain edges of nonsensical types                           | Free-form by design in MVP; edge-rules deferred until real patterns emerge.                                       |
| Single SQLite writer becomes a contention point                   | Irrelevant in MVP (single-process CLI). WAL mode set on Open. Future HTTP server will need queuing.              |
| Slug collisions for differently-named nodes that slugify the same | `ErrNodeAlreadyExists` raised; user passes `--id explicit-slug` or renames.                                       |
| LLM agent loops on "already exists" errors                        | `--if-not-exists` flag on all create commands.                                                                    |

## Implementation plan

To be authored next via the `writing-plans` skill. The plan will sequence:

1. Repo scaffolding (Go module, layout, Makefile, sqlc config, `.gitignore`,
   `.golangci.yml`, `tools.go`).
2. Migration `0001_init.sql` with all four tables and indexes.
3. `internal/graph` types, sentinel errors, fake `Store`, parsers.
4. `internal/graph/service.go` validation logic.
5. Unit tests for validation through the fake `Store`.
6. `internal/store` SQLite implementation, `InTx`, and integration tests.
7. `cmd/kg` cobra commands, JSON envelope, error map, smoke tests.
8. README with the user walkthrough as the canonical example.
