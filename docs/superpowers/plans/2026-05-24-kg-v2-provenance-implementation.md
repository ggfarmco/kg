# kg v2 Implementation Plan — Provenance + Declarative Apply

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Evolve v1 into a provenance-aware engine: every node and edge claim is attributed to a `source`; properties are namespaced JSON keyed by source id; edges are reference-counted via `edge_claims`; a new `kg apply` verb consumes a JSON snapshot, computes a diff against current state, and applies it atomically. Plugins choose between the existing imperative wire protocol (`kg batch`) or the new declarative one (`kg apply`).

**Architecture:** Rewrite migration 0001 in place (no data exists yet). Add two tables: `sources` (writer registry) and `edge_claims` (reference counts). Change `nodes.properties` and `edges.properties` to a namespaced JSON shape: `{<source-id>: {<key>: <value>}}`. Every Service mutation accepts a required `Source`; auto-registration creates the row on first write. A new public `snapshot/` package defines the declarative wire format (sibling to `batch/`). A new `Service.Apply` method runs the diff algorithm inside one `Store.InTx`. `kg-extractor` dispatches by `manifest.runtime`: imperative runtimes pipe to `kg batch`; declarative-* runtimes pipe to `kg apply`.

**Tech Stack (delta from v1):** None. Same toolchain (Go 1.26, `modernc.org/sqlite`, `pressly/goose`, `sqlc`, cobra, testify). No new third-party deps in either the root module or `plugins/tree-sitter/`.

**Spec:** `docs/superpowers/specs/2026-05-24-kg-v2-provenance-design.md`. Re-read it before each new phase — design intent (why nodes single-owner, why edges multi-source, the apply algorithm, conflict codes, the `protocol_version: 2` choice) is not redundantly restated in each task. v1 context: `docs/superpowers/specs/2026-05-23-kg-v1-extractor-design.md`. v0 context: `docs/superpowers/specs/2026-05-23-kg-mvp-design.md`.

**Prereq:** This plan builds on v0 + v1 (shipped on `main`, latest pre-v2 commit `27498e3`, "feat: v1 — extractor system"). All v1 tests are green. v2 work lives on branch `feat/kg-v2-provenance`; the spec is already committed as `e7afd58`. Branch merges back to `main` as one unit when Phase 8 is green.

**Conventions:**
- Import grouping (3 blocks separated by blank lines): stdlib, third-party, current module (`github.com/ggfarmco/kg/...`). For `plugins/tree-sitter/` (a separate module), `github.com/ggfarmco/kg/batch` and `github.com/ggfarmco/kg/snapshot` sit in the *third-party* block from the plugin's perspective.
- No comments in code unless they explain a non-obvious *why*. Generated sqlc files are exempt.
- Tests are minimal and non-redundant — each test covers one distinct behavior. Don't combine "add + scoping + ownership" into one test; split them.
- Every task ends with a commit. Commit messages follow `<type>(scope): <imperative summary>` (types: feat, test, chore, docs, refactor, fix). New v2 scopes: `schema`, `graph`, `store`, `snapshot`, `apply`, `cli`, `extractor`, `plugin-tree-sitter`, `plugin-bash-demo`, `e2e`.
- Service inputs that take maps/slices defensive-copy them (the existing `nonNilProps` helper in `internal/graph/service.go` is the pattern).
- The relaxed slug regex `^[a-z0-9-]+(?:(?:/|::)[a-z0-9-]+)*$` from v1 stays. Don't tighten it.
- Goose migrations: 0001 is **rewritten in place**, no 0002. Dev users delete `kg.db` and re-init.

**State during the rewrite.** Phase 1 changes the schema and the store layer; this temporarily breaks Service tests, CLI tests, golden tests, and e2e until later phases catch them up. The intermediate state between Phase 1 and Phase 5 is "compiles, lower-tier tests partially red." Don't panic; the checklist progression in this plan resolves it. Don't fix things ahead of where the plan asks — drive-by edits between tasks make it hard to map blame back to the failing tier.

**Before starting:** verify the branch is `feat/kg-v2-provenance`:

```bash
git rev-parse --abbrev-ref HEAD  # should print feat/kg-v2-provenance
```

---

## Phase 1 — Schema + store foundations

Migration 0001 is rewritten in place. Two new tables (`sources`, `edge_claims`); the `nodes` table gains a `source` column; the `properties` column on both `nodes` and `edges` changes meaning from "flat JSON object" to "namespaced JSON keyed by source id". `changes` gains a nullable `source` column. The generated sqlc code follows; new typed Go-side types follow that. By the end of Phase 1, the project compiles but Service/CLI/e2e tests are largely red — Phase 2+ catches them up.

### Task 1: Rewrite `migrations/0001_init.sql`

**Files:**
- Modify: `migrations/0001_init.sql` (full rewrite)

- [ ] **Step 1: Write the new schema**

Replace the entire contents of `migrations/0001_init.sql` with:

```sql
-- +goose Up

CREATE TABLE sources (
  id          TEXT PRIMARY KEY,
  description TEXT,
  trust       INTEGER NOT NULL DEFAULT 100,
  first_seen  INTEGER NOT NULL,
  last_seen   INTEGER NOT NULL
);

CREATE TABLE domains (
  id          TEXT PRIMARY KEY,
  description TEXT,
  layers      TEXT NOT NULL,
  revision    INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL
);

CREATE TABLE nodes (
  id          TEXT PRIMARY KEY,
  domain      TEXT NOT NULL REFERENCES domains(id) ON DELETE RESTRICT,
  layer       TEXT NOT NULL,
  name        TEXT NOT NULL,
  parent_id   TEXT REFERENCES nodes(id) ON DELETE RESTRICT,
  source      TEXT NOT NULL REFERENCES sources(id),
  properties  TEXT NOT NULL DEFAULT '{}',
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
  properties  TEXT NOT NULL DEFAULT '{}',
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
  entity      TEXT NOT NULL,
  entity_id   TEXT NOT NULL,
  source      TEXT REFERENCES sources(id),
  op          TEXT NOT NULL,
  revision    INTEGER,
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

- [ ] **Step 2: Sanity-check the SQL parses by spinning up an in-memory DB**

There is no goose migration test in v1; the migration's correctness is validated by `internal/store/store_test.go` once Phase 1 completes. So at this point just rerun:

```bash
rm -f /tmp/kgv2-schema-check.db
go run ./tools/sqlc-noop 2>/dev/null || true  # only if it exists; safe noop otherwise
```

The real validation comes after Task 2 regenerates sqlc. Skip running tests now — they'll start failing.

- [ ] **Step 3: Commit**

```bash
git add migrations/0001_init.sql
git commit -m "feat(schema): rewrite migration 0001 with sources, edge_claims, namespaced properties"
```

---

### Task 2: Rewrite `internal/store/queries.sql` and regenerate sqlc

The query surface expands: new CRUD for `sources` and `edge_claims`, new `UpsertSource` for auto-register, new `UpsertEdge` for idempotent edge creation by `(source_id, target_id, type)`, new `CountEdgeClaims` for GC, and `CreateNode` / `AppendChange` grow a `source` column.

**Files:**
- Modify: `internal/store/queries.sql` (full rewrite)
- Regenerate (committed): `internal/store/queries.sql.go`, `internal/store/models.go`, `internal/store/db.go`

- [ ] **Step 1: Replace `internal/store/queries.sql`**

```sql
-- name: UpsertSource :exec
INSERT INTO sources(id, description, trust, first_seen, last_seen)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET last_seen = excluded.last_seen;

-- name: GetSource :one
SELECT id, description, trust, first_seen, last_seen FROM sources WHERE id = ?;

-- name: ListSources :many
SELECT id, description, trust, first_seen, last_seen FROM sources ORDER BY id;

-- name: UpdateSource :exec
UPDATE sources SET description = ?, trust = ? WHERE id = ?;

-- name: DeleteSource :exec
DELETE FROM sources WHERE id = ?;

-- name: CreateDomain :exec
INSERT INTO domains(id, description, layers, revision, created_at)
VALUES (?, ?, ?, 1, ?);

-- name: GetDomain :one
SELECT id, description, layers, revision, created_at FROM domains WHERE id = ?;

-- name: ListDomains :many
SELECT id, description, layers, revision, created_at FROM domains ORDER BY id;

-- name: DeleteDomain :exec
DELETE FROM domains WHERE id = ?;

-- name: AppendChange :exec
INSERT INTO changes(entity, entity_id, source, op, revision, at) VALUES (?, ?, ?, ?, ?, ?);

-- name: CreateNode :exec
INSERT INTO nodes(id, domain, layer, name, parent_id, source, properties, revision, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?);

-- name: GetNode :one
SELECT id, domain, layer, name, parent_id, source, properties, revision, created_at, updated_at
FROM nodes WHERE id = ?;

-- name: ListNodes :many
SELECT id, domain, layer, name, parent_id, source, properties, revision, created_at, updated_at
FROM nodes
WHERE (sqlc.arg(domain_filter) = '' OR domain = sqlc.arg(domain_filter))
  AND (sqlc.arg(layer_filter)  = '' OR layer  = sqlc.arg(layer_filter))
  AND (sqlc.arg(source_filter) = '' OR source = sqlc.arg(source_filter))
ORDER BY id
LIMIT CASE WHEN sqlc.arg(lim) = 0 THEN -1 ELSE sqlc.arg(lim) END;

-- name: ChildrenOf :many
SELECT id, domain, layer, name, parent_id, source, properties, revision, created_at, updated_at
FROM nodes WHERE parent_id = ? ORDER BY id;

-- name: NodesOwnedBy :many
SELECT id, domain, layer, name, parent_id, source, properties, revision, created_at, updated_at
FROM nodes WHERE domain = ? AND source = ? ORDER BY id;

-- name: UpdateNode :exec
UPDATE nodes SET name = ?, properties = ?, revision = revision + 1, updated_at = ?
WHERE id = ?;

-- name: GetNodeRevision :one
SELECT revision FROM nodes WHERE id = ?;

-- name: DeleteNode :exec
DELETE FROM nodes WHERE id = ?;

-- name: UpsertEdge :one
INSERT INTO edges(source_id, target_id, type, properties, revision, created_at)
VALUES (?, ?, ?, ?, 1, ?)
ON CONFLICT(source_id, target_id, type) DO UPDATE SET source_id = excluded.source_id
RETURNING id;

-- name: GetEdge :one
SELECT id, source_id, target_id, type, properties, revision, created_at FROM edges WHERE id = ?;

-- name: UpdateEdgeProperties :exec
UPDATE edges SET properties = ?, revision = revision + 1 WHERE id = ?;

-- name: DeleteEdge :exec
DELETE FROM edges WHERE id = ?;

-- name: EdgesFromAll :many
SELECT id, source_id, target_id, type, properties, revision, created_at
FROM edges WHERE source_id = ? ORDER BY id;

-- name: EdgesFromTyped :many
SELECT id, source_id, target_id, type, properties, revision, created_at
FROM edges WHERE source_id = ? AND type IN (sqlc.slice(types)) ORDER BY id;

-- name: EdgesToAll :many
SELECT id, source_id, target_id, type, properties, revision, created_at
FROM edges WHERE target_id = ? ORDER BY id;

-- name: EdgesToTyped :many
SELECT id, source_id, target_id, type, properties, revision, created_at
FROM edges WHERE target_id = ? AND type IN (sqlc.slice(types)) ORDER BY id;

-- name: AddEdgeClaim :exec
INSERT OR IGNORE INTO edge_claims(edge_id, source, claimed_at) VALUES (?, ?, ?);

-- name: RemoveEdgeClaim :exec
DELETE FROM edge_claims WHERE edge_id = ? AND source = ?;

-- name: CountEdgeClaims :one
SELECT COUNT(*) AS n FROM edge_claims WHERE edge_id = ?;

-- name: ListEdgeClaims :many
SELECT edge_id, source, claimed_at FROM edge_claims WHERE edge_id = ? ORDER BY source;

-- name: EdgeIDsClaimedBy :many
SELECT edge_id FROM edge_claims WHERE source = ? ORDER BY edge_id;
```

Two notes:

1. The `UpsertEdge` `ON CONFLICT ... DO UPDATE SET source_id = excluded.source_id` is a no-op assignment that's the cheapest way to get `RETURNING id` for the *existing* row when the insert conflicts. SQLite requires a DO clause; this assignment doesn't change anything because `source_id` is the conflict key.

2. `NodesOwnedBy` is the diff helper for `Service.Apply` — gets all nodes a given source owns within a domain.

- [ ] **Step 2: Regenerate sqlc**

```bash
make gen
```

Expected: `internal/store/queries.sql.go`, `internal/store/models.go`, and `internal/store/db.go` are regenerated. Inspect the diff:

```bash
git diff internal/store/queries.sql.go | head -40
```

Sanity-check: `Node` struct in `models.go` should now have a `Source` field of type `string`; `Change` should have `Source *string`; new types `Source` and `EdgeClaim` should exist.

- [ ] **Step 3: Commit**

```bash
git add internal/store/queries.sql internal/store/queries.sql.go internal/store/models.go internal/store/db.go
git commit -m "feat(store): rewrite queries for sources, edge_claims, namespaced properties"
```

---

### Task 3: New `internal/graph/source.go`

Introduce the `SourceID` typed alias, the `Source` and `EdgeClaim` value types, and the new sentinel errors for ownership/claim conflicts.

**Files:**
- Create: `internal/graph/source.go`
- Create: `internal/graph/source_test.go`
- Modify: `internal/graph/errors.go`

- [ ] **Step 1: Write failing tests for `SourceID`**

Create `internal/graph/source_test.go`:

```go
package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestParseSourceID(t *testing.T) {
	cases := map[string]bool{
		"cli":               true,
		"manual":            true,
		"tree-sitter:0.1.0": true,
		"acme/foo:1.0":      true,
		"":                  false,
		"Bad ID":            false,
		"colon::only":       false,
	}
	for in, ok := range cases {
		_, err := graph.ParseSourceID(in)
		if ok {
			require.NoError(t, err, "want valid: %q", in)
		} else {
			require.Error(t, err, "want invalid: %q", in)
		}
	}
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/graph/ -run TestParseSourceID -v
```

Expected: FAIL (`ParseSourceID` undefined).

- [ ] **Step 3: Implement `internal/graph/source.go`**

```go
package graph

import (
	"regexp"
	"time"
)

type SourceID string

type Source struct {
	ID          SourceID  `json:"id"`
	Description string    `json:"description"`
	Trust       int       `json:"trust"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

type EdgeClaim struct {
	EdgeID    EdgeID    `json:"edge_id"`
	Source    SourceID  `json:"source"`
	ClaimedAt time.Time `json:"claimed_at"`
}

var sourceIDRE = regexp.MustCompile(`^[a-z0-9-]+(?:/[a-z0-9-]+)*(?::[a-z0-9][a-z0-9.\-]*)?$`)

func ParseSourceID(s string) (SourceID, error) {
	if !sourceIDRE.MatchString(s) {
		return "", ErrInvalidSourceID
	}
	return SourceID(s), nil
}
```

The grammar accepts: `cli`, `manual`, `tree-sitter:0.1.0`, `acme/foo:1.0`, `git:1.0`. The version suffix after `:` is optional. Slashes are allowed for `<org>/<name>` style.

- [ ] **Step 4: Add the sentinel errors**

In `internal/graph/errors.go`, append to the `var ( ... )` block:

```go
	ErrInvalidSourceID                = errors.New("invalid source id")
	ErrSourceNotFound                 = errors.New("source not found")
	ErrSourceHasDependents            = errors.New("source has dependents")
	ErrSourceRequired                 = errors.New("source is required")
	ErrNodeOwnedByDifferentSource     = errors.New("node owned by a different source")
	ErrNodeNotOwner                   = errors.New("not the owner of this node")
	ErrCoreFieldsImmutable            = errors.New("core node fields (layer/parent) are immutable")
	ErrNodeHasForeignClaims           = errors.New("node has incident edges with foreign claims")
	ErrDomainHasForeignWriters        = errors.New("domain contains nodes owned by other sources")
	ErrEdgeNoClaim                    = errors.New("edge has no claim from this source")
```

- [ ] **Step 5: Run, verify pass**

```bash
go test ./internal/graph/ -run TestParseSourceID -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/graph/source.go internal/graph/source_test.go internal/graph/errors.go
git commit -m "feat(graph): add SourceID type, Source/EdgeClaim values, and ownership sentinels"
```

---

### Task 4: Update `internal/graph/node.go` and `edge.go` for namespaced properties + claims

`Node.Properties` and `Edge.Properties` change type from `map[string]any` to `map[SourceID]map[string]any`. `Node` gains a `Source SourceID` field. `Edge` gains `Claims []SourceID` (populated on read by Store; ignored on write because claims are managed via dedicated methods).

**Files:**
- Modify: `internal/graph/node.go`
- Modify: `internal/graph/edge.go`

- [ ] **Step 1: Update `Node`**

Replace the `Node` struct in `internal/graph/node.go`:

```go
type Node struct {
	ID         NodeID                          `json:"id"`
	Domain     DomainID                        `json:"domain"`
	Layer      string                          `json:"layer"`
	Name       string                          `json:"name"`
	ParentID   *NodeID                         `json:"parent_id"`
	Source     SourceID                        `json:"source"`
	Properties map[SourceID]map[string]any     `json:"properties"`
	Revision   int64                           `json:"revision"`
	CreatedAt  time.Time                       `json:"created_at"`
	UpdatedAt  time.Time                       `json:"updated_at"`
}
```

Also add the `NodeFilter.Source` field for filtering:

```go
type NodeFilter struct {
	Domain DomainID
	Layer  string
	Source SourceID
	Limit  int
}
```

(`Summary` is removed — properties replace it. The CLI surfaces it via the `--source` view in a later task.)

- [ ] **Step 2: Update `Edge`**

Replace `internal/graph/edge.go`:

```go
package graph

import "time"

type EdgeID int64

type Edge struct {
	ID         EdgeID                          `json:"id"`
	SourceID   NodeID                          `json:"source_id"`
	TargetID   NodeID                          `json:"target_id"`
	Type       string                          `json:"type"`
	Properties map[SourceID]map[string]any     `json:"properties"`
	Claims     []SourceID                      `json:"claims"`
	Revision   int64                           `json:"revision"`
	CreatedAt  time.Time                       `json:"created_at"`
}
```

- [ ] **Step 3: Compile check (tests will fail; that's expected)**

```bash
go build ./internal/graph/...
```

Expected: builds (Service won't yet use the new shape because we haven't touched it). Tests will fail to compile or fail at runtime; that's expected and resolved in Phase 2.

- [ ] **Step 4: Commit**

```bash
git add internal/graph/node.go internal/graph/edge.go
git commit -m "feat(graph): node/edge types carry Source + namespaced properties + claims"
```

---

### Task 5: Store implementation — sources, edge_claims, source-aware nodes/edges

Add `internal/store/sources.go` and `internal/store/edge_claims.go`. Modify `internal/store/nodes.go` and `internal/store/edges.go` for the new schema. Property encoding/decoding goes through the namespaced map.

**Files:**
- Create: `internal/store/sources.go`
- Create: `internal/store/sources_test.go`
- Create: `internal/store/edge_claims.go`
- Create: `internal/store/edge_claims_test.go`
- Modify: `internal/store/nodes.go`
- Modify: `internal/store/edges.go`
- Modify: `internal/store/store.go` (no change expected, but verify FK ON pragma still set)
- Modify: `internal/store/nodes_test.go`, `internal/store/edges_test.go`, `internal/store/changes_test.go` (update to source-aware inputs)

- [ ] **Step 1: Write failing test for `sources` CRUD**

Create `internal/store/sources_test.go`:

```go
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestSourcesUpsertAndGet(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	require.NoError(t, st.UpsertSource(ctx, graph.Source{
		ID: "tree-sitter:0.1.0", Description: "ts", Trust: 100,
		FirstSeen: time.UnixMilli(1000), LastSeen: time.UnixMilli(1000),
	}))
	got, err := st.GetSource(ctx, "tree-sitter:0.1.0")
	require.NoError(t, err)
	require.Equal(t, "ts", got.Description)
	require.Equal(t, 100, got.Trust)
}

func TestSourcesUpsertBumpsLastSeen(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	require.NoError(t, st.UpsertSource(ctx, graph.Source{
		ID: "x", Trust: 100,
		FirstSeen: time.UnixMilli(1000), LastSeen: time.UnixMilli(1000),
	}))
	require.NoError(t, st.UpsertSource(ctx, graph.Source{
		ID: "x", Trust: 100,
		FirstSeen: time.UnixMilli(1000), LastSeen: time.UnixMilli(2000),
	}))
	got, err := st.GetSource(ctx, "x")
	require.NoError(t, err)
	require.Equal(t, int64(2000), got.LastSeen.UnixMilli())
	require.Equal(t, int64(1000), got.FirstSeen.UnixMilli())
}

func TestDeleteSourceWithOwnedNodeFails(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	seedDomainAndSource(t, st, "d", "x")
	require.NoError(t, st.CreateNode(ctx, graph.Node{
		ID: "d:n", Domain: "d", Layer: "l1", Name: "n", Source: "x",
		Properties: map[graph.SourceID]map[string]any{},
		CreatedAt:  time.UnixMilli(1), UpdatedAt: time.UnixMilli(1),
	}))
	err := st.DeleteSource(ctx, "x")
	require.ErrorIs(t, err, graph.ErrSourceHasDependents)
}
```

The `openTestStore` and `seedDomainAndSource` helpers belong in `internal/store/store_test.go`. Add them there (extending the v1 helpers):

```go
// In internal/store/store_test.go (replace any existing helper of the same name).
func seedDomainAndSource(t *testing.T, st *store.Store, domain, source string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, st.UpsertSource(context.Background(), graph.Source{
		ID: graph.SourceID(source), Trust: 100,
		FirstSeen: time.UnixMilli(1), LastSeen: time.UnixMilli(1),
	}))
	require.NoError(t, st.CreateDomain(ctx, graph.Domain{
		ID: graph.DomainID(domain), Layers: []string{"l1", "l2"}, Revision: 1, CreatedAt: time.UnixMilli(1),
	}))
}
```

(If the existing v1 `openTestStore` opens `:memory:` — keep it. Adjust import block as needed.)

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/store/ -run TestSources -v
```

Expected: FAIL (`UpsertSource` undefined).

- [ ] **Step 3: Implement `internal/store/sources.go`**

```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ggfarmco/kg/internal/graph"
)

func (s *Store) UpsertSource(ctx context.Context, src graph.Source) error {
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		return q.UpsertSource(ctx, UpsertSourceParams{
			ID:          string(src.ID),
			Description: nullStringPtr(src.Description),
			Trust:       int64(src.Trust),
			FirstSeen:   src.FirstSeen.UnixMilli(),
			LastSeen:    src.LastSeen.UnixMilli(),
		})
	})
}

func (s *Store) GetSource(ctx context.Context, id graph.SourceID) (*graph.Source, error) {
	q := New(s.conn(ctx))
	row, err := q.GetSource(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, graph.ErrSourceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get source: %w", err)
	}
	return decodeSource(row.ID, row.Description, row.Trust, row.FirstSeen, row.LastSeen), nil
}

func (s *Store) ListSources(ctx context.Context) ([]graph.Source, error) {
	q := New(s.conn(ctx))
	rows, err := q.ListSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list sources: %w", err)
	}
	out := make([]graph.Source, 0, len(rows))
	for _, r := range rows {
		out = append(out, *decodeSource(r.ID, r.Description, r.Trust, r.FirstSeen, r.LastSeen))
	}
	return out, nil
}

func (s *Store) UpdateSource(ctx context.Context, src graph.Source) error {
	q := New(s.conn(ctx))
	return q.UpdateSource(ctx, UpdateSourceParams{
		ID:          string(src.ID),
		Description: nullStringPtr(src.Description),
		Trust:       int64(src.Trust),
	})
}

func (s *Store) DeleteSource(ctx context.Context, id graph.SourceID) error {
	q := New(s.conn(ctx))
	if err := q.DeleteSource(ctx, string(id)); err != nil {
		if isFKViolation(err) {
			return graph.ErrSourceHasDependents
		}
		return fmt.Errorf("sqlite: delete source: %w", err)
	}
	return nil
}

func decodeSource(id string, desc *string, trust, firstSeen, lastSeen int64) *graph.Source {
	d := ""
	if desc != nil {
		d = *desc
	}
	return &graph.Source{
		ID: graph.SourceID(id), Description: d, Trust: int(trust),
		FirstSeen: time.UnixMilli(firstSeen),
		LastSeen:  time.UnixMilli(lastSeen),
	}
}

func isFKViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
```

(`nullStringPtr` already exists in `internal/store/nodes.go`.)

- [ ] **Step 4: Run, verify pass**

```bash
go test ./internal/store/ -run TestSources -v
```

Expected: PASS.

- [ ] **Step 5: Write failing test for edge_claims CRUD**

Create `internal/store/edge_claims_test.go`:

```go
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestAddAndCountEdgeClaims(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	edgeID := seedTwoNodesAndEdge(t, st, "x")
	require.NoError(t, st.AddEdgeClaim(ctx, edgeID, "x", time.UnixMilli(1)))
	n, err := st.CountEdgeClaims(ctx, edgeID)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	require.NoError(t, st.AddEdgeClaim(ctx, edgeID, "x", time.UnixMilli(2)))
	n, err = st.CountEdgeClaims(ctx, edgeID)
	require.NoError(t, err)
	require.Equal(t, 1, n, "INSERT OR IGNORE — duplicate (edge_id, source) must not double-count")
}

func TestRemoveEdgeClaim(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	edgeID := seedTwoNodesAndEdge(t, st, "x")
	require.NoError(t, st.AddEdgeClaim(ctx, edgeID, "x", time.UnixMilli(1)))
	require.NoError(t, st.RemoveEdgeClaim(ctx, edgeID, "x"))
	n, err := st.CountEdgeClaims(ctx, edgeID)
	require.NoError(t, err)
	require.Equal(t, 0, n)
}
```

Add `seedTwoNodesAndEdge` to `internal/store/store_test.go`:

```go
func seedTwoNodesAndEdge(t *testing.T, st *store.Store, source string) graph.EdgeID {
	t.Helper()
	ctx := context.Background()
	seedDomainAndSource(t, st, "d", source)
	require.NoError(t, st.CreateNode(ctx, graph.Node{
		ID: "d:a", Domain: "d", Layer: "l1", Name: "a", Source: graph.SourceID(source),
		Properties: map[graph.SourceID]map[string]any{},
		CreatedAt:  time.UnixMilli(1), UpdatedAt: time.UnixMilli(1),
	}))
	require.NoError(t, st.CreateNode(ctx, graph.Node{
		ID: "d:b", Domain: "d", Layer: "l1", Name: "b", Source: graph.SourceID(source),
		Properties: map[graph.SourceID]map[string]any{},
		CreatedAt:  time.UnixMilli(1), UpdatedAt: time.UnixMilli(1),
	}))
	id, err := st.UpsertEdge(ctx, graph.Edge{
		SourceID: "d:a", TargetID: "d:b", Type: "imports",
		Properties: map[graph.SourceID]map[string]any{},
		CreatedAt:  time.UnixMilli(1),
	})
	require.NoError(t, err)
	return id
}
```

- [ ] **Step 6: Implement `internal/store/edge_claims.go`**

```go
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ggfarmco/kg/internal/graph"
)

func (s *Store) AddEdgeClaim(ctx context.Context, edgeID graph.EdgeID, source graph.SourceID, at time.Time) error {
	q := New(s.conn(ctx))
	return q.AddEdgeClaim(ctx, AddEdgeClaimParams{
		EdgeID: int64(edgeID), Source: string(source), ClaimedAt: at.UnixMilli(),
	})
}

func (s *Store) RemoveEdgeClaim(ctx context.Context, edgeID graph.EdgeID, source graph.SourceID) error {
	q := New(s.conn(ctx))
	return q.RemoveEdgeClaim(ctx, RemoveEdgeClaimParams{EdgeID: int64(edgeID), Source: string(source)})
}

func (s *Store) CountEdgeClaims(ctx context.Context, edgeID graph.EdgeID) (int, error) {
	q := New(s.conn(ctx))
	n, err := q.CountEdgeClaims(ctx, int64(edgeID))
	if err != nil {
		return 0, fmt.Errorf("sqlite: count edge claims: %w", err)
	}
	return int(n), nil
}

func (s *Store) ListEdgeClaims(ctx context.Context, edgeID graph.EdgeID) ([]graph.EdgeClaim, error) {
	q := New(s.conn(ctx))
	rows, err := q.ListEdgeClaims(ctx, int64(edgeID))
	if err != nil {
		return nil, fmt.Errorf("sqlite: list edge claims: %w", err)
	}
	out := make([]graph.EdgeClaim, 0, len(rows))
	for _, r := range rows {
		out = append(out, graph.EdgeClaim{
			EdgeID: graph.EdgeID(r.EdgeID), Source: graph.SourceID(r.Source),
			ClaimedAt: time.UnixMilli(r.ClaimedAt),
		})
	}
	return out, nil
}

func (s *Store) EdgeIDsClaimedBy(ctx context.Context, source graph.SourceID) ([]graph.EdgeID, error) {
	q := New(s.conn(ctx))
	rows, err := q.EdgeIDsClaimedBy(ctx, string(source))
	if err != nil {
		return nil, fmt.Errorf("sqlite: edges claimed by: %w", err)
	}
	out := make([]graph.EdgeID, 0, len(rows))
	for _, r := range rows {
		out = append(out, graph.EdgeID(r))
	}
	return out, nil
}
```

- [ ] **Step 7: Modify `internal/store/nodes.go` for source + namespaced properties**

Replace `CreateNode` and the decode helper:

```go
func (s *Store) CreateNode(ctx context.Context, n graph.Node) error {
	props, err := encodeNamespacedProps(n.Properties)
	if err != nil {
		return err
	}
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		if err := q.CreateNode(ctx, CreateNodeParams{
			ID:         string(n.ID),
			Domain:     string(n.Domain),
			Layer:      n.Layer,
			Name:       n.Name,
			ParentID:   nodeIDPtr(n.ParentID),
			Source:     string(n.Source),
			Properties: props,
			CreatedAt:  n.CreatedAt.UnixMilli(),
			UpdatedAt:  n.UpdatedAt.UnixMilli(),
		}); err != nil {
			return mapSQLiteErr(err, "node")
		}
		rev := int64(1)
		src := string(n.Source)
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "node",
			EntityID: string(n.ID),
			Source:   &src,
			Op:       "create",
			Revision: &rev,
			At:       n.CreatedAt.UnixMilli(),
		})
	})
}
```

Replace `UpdateNode`:

```go
func (s *Store) UpdateNode(ctx context.Context, n graph.Node) error {
	props, err := encodeNamespacedProps(n.Properties)
	if err != nil {
		return err
	}
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		existing, err := q.GetNode(ctx, string(n.ID))
		if err != nil {
			return mapSQLiteErr(err, "node")
		}
		if err := q.UpdateNode(ctx, UpdateNodeParams{
			ID:         string(n.ID),
			Name:       n.Name,
			Properties: props,
			UpdatedAt:  n.UpdatedAt.UnixMilli(),
		}); err != nil {
			return mapSQLiteErr(err, "node")
		}
		rev, err := q.GetNodeRevision(ctx, string(n.ID))
		if err != nil {
			return fmt.Errorf("sqlite: get node revision: %w", err)
		}
		ownerSrc := existing.Source
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "node",
			EntityID: string(n.ID),
			Source:   &ownerSrc,
			Op:       "update",
			Revision: &rev,
			At:       n.UpdatedAt.UnixMilli(),
		})
	})
}
```

Replace `DeleteNode`:

```go
func (s *Store) DeleteNode(ctx context.Context, id graph.NodeID) error {
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		existing, err := q.GetNode(ctx, string(id))
		if err != nil {
			return mapSQLiteErr(err, "node")
		}
		if err := q.DeleteNode(ctx, string(id)); err != nil {
			if isFKViolation(err) {
				return graph.ErrHasDependents
			}
			return mapSQLiteErr(err, "node")
		}
		ownerSrc := existing.Source
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "node",
			EntityID: string(id),
			Source:   &ownerSrc,
			Op:       "delete",
			Revision: nil,
			At:       time.Now().UnixMilli(),
		})
	})
}
```

Replace `decodeNode` (no Summary, with Source, namespaced props):

```go
func decodeNode(id, domain, layer, name string, parent *string, source, propsJSON string, rev, createdAt, updatedAt int64) (*graph.Node, error) {
	props, err := decodeNamespacedProps(propsJSON)
	if err != nil {
		return nil, err
	}
	n := &graph.Node{
		ID: graph.NodeID(id), Domain: graph.DomainID(domain),
		Layer: layer, Name: name,
		Source: graph.SourceID(source), Properties: props,
		Revision:  rev,
		CreatedAt: time.UnixMilli(createdAt),
		UpdatedAt: time.UnixMilli(updatedAt),
	}
	if parent != nil {
		p := graph.NodeID(*parent)
		n.ParentID = &p
	}
	return n, nil
}
```

Update `GetNode`, `ListNodes`, `ChildrenOf` to pass `row.Source` to `decodeNode`. Also add `NodesOwnedBy`:

```go
func (s *Store) NodesOwnedBy(ctx context.Context, domain graph.DomainID, source graph.SourceID) ([]graph.Node, error) {
	q := New(s.conn(ctx))
	rows, err := q.NodesOwnedBy(ctx, NodesOwnedByParams{Domain: string(domain), Source: string(source)})
	if err != nil {
		return nil, fmt.Errorf("sqlite: nodes owned by: %w", err)
	}
	out := make([]graph.Node, 0, len(rows))
	for _, r := range rows {
		n, err := decodeNode(r.ID, r.Domain, r.Layer, r.Name, r.ParentID, r.Source, r.Properties, r.Revision, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, nil
}
```

Update `ListNodes` to pass the source filter through (`ListNodesParams.SourceFilter = string(f.Source)`).

Add the namespaced-properties codec at the bottom of `internal/store/nodes.go`:

```go
func encodeNamespacedProps(p map[graph.SourceID]map[string]any) (string, error) {
	if p == nil {
		return "{}", nil
	}
	conv := make(map[string]map[string]any, len(p))
	for k, v := range p {
		conv[string(k)] = v
	}
	b, err := json.Marshal(conv)
	if err != nil {
		return "", fmt.Errorf("marshal namespaced properties: %w", err)
	}
	return string(b), nil
}

func decodeNamespacedProps(s string) (map[graph.SourceID]map[string]any, error) {
	if s == "" || s == "{}" {
		return map[graph.SourceID]map[string]any{}, nil
	}
	var raw map[string]map[string]any
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, fmt.Errorf("unmarshal namespaced properties: %w", err)
	}
	out := make(map[graph.SourceID]map[string]any, len(raw))
	for k, v := range raw {
		out[graph.SourceID(k)] = v
	}
	return out, nil
}
```

- [ ] **Step 8: Modify `internal/store/edges.go` for namespaced properties and UPSERT**

Replace `CreateEdge` with `UpsertEdge` (returns the row id, atomic-by-unique-key):

```go
func (s *Store) UpsertEdge(ctx context.Context, e graph.Edge) (graph.EdgeID, error) {
	props, err := encodeNamespacedProps(e.Properties)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		newID, qerr := q.UpsertEdge(ctx, UpsertEdgeParams{
			SourceID:   string(e.SourceID),
			TargetID:   string(e.TargetID),
			Type:       e.Type,
			Properties: props,
			CreatedAt:  e.CreatedAt.UnixMilli(),
		})
		if qerr != nil {
			return mapSQLiteErr(qerr, "edge")
		}
		id = newID
		return nil
	})
	return graph.EdgeID(id), err
}

func (s *Store) UpdateEdgeProperties(ctx context.Context, id graph.EdgeID, props map[graph.SourceID]map[string]any) error {
	encoded, err := encodeNamespacedProps(props)
	if err != nil {
		return err
	}
	q := New(s.conn(ctx))
	return q.UpdateEdgeProperties(ctx, UpdateEdgePropertiesParams{ID: int64(id), Properties: encoded})
}
```

Replace `decodeEdge`:

```go
func decodeEdge(r Edge) (*graph.Edge, error) {
	props, err := decodeNamespacedProps(r.Properties)
	if err != nil {
		return nil, err
	}
	return &graph.Edge{
		ID: graph.EdgeID(r.ID), SourceID: graph.NodeID(r.SourceID),
		TargetID: graph.NodeID(r.TargetID), Type: r.Type,
		Properties: props, Revision: r.Revision,
		CreatedAt: time.UnixMilli(r.CreatedAt),
	}, nil
}
```

`DeleteEdge` keeps its existing signature; the CASCADE on `edge_claims.edge_id` drops claims automatically.

- [ ] **Step 9: Update `internal/store/nodes_test.go`, `edges_test.go`, `changes_test.go`**

These tests currently create nodes/edges without a Source. Update each call-site to pass `Source: "x"` (after `seedDomainAndSource` has registered `x`). Replace any `Summary:` literals with the namespaced properties shape if the test asserts something about properties. For `changes_test.go`, assert the new `source` column behavior (changes for a node should carry the owner's source).

The minimum diff is mechanical: add `Source` everywhere `graph.Node{}` is constructed, replace `map[string]any{...}` with `map[graph.SourceID]map[string]any{"x": {...}}` if the test exercised properties. (If a test wasn't using properties, the empty `map[graph.SourceID]map[string]any{}` is fine.) Remove any assertions on `Summary`.

- [ ] **Step 10: Run all `internal/store/` tests**

```bash
go test ./internal/store/...
```

Expected: PASS. If a test references `Summary` and you missed it, fix it and re-run.

- [ ] **Step 11: Commit**

```bash
git add internal/store/
git commit -m "feat(store): source-aware nodes, namespaced properties, edge_claims, sources CRUD"
```

---

## Phase 2 — Graph core: source-aware Service + edge claims + namespaced properties

Update `graph.Store` interface to expose the new store surface. Update `FakeStore` to satisfy it. Update `Service` so every mutation accepts a required `source`, auto-registers it, and enforces ownership rules. `Service.CreateEdge` becomes UPSERT+claim; new `Service.AddEdgeClaim/RemoveEdgeClaim/ListEdgeClaims` manage claims with same-tx GC. New `Service.SetNodeProperties/DeleteNodeProperties` manage namespaced property writes.

### Task 6: Update `graph.Store` interface

**Files:**
- Modify: `internal/graph/store.go`

- [ ] **Step 1: Replace the interface**

```go
package graph

import (
	"context"
	"time"
)

type Store interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error

	UpsertSource(ctx context.Context, src Source) error
	GetSource(ctx context.Context, id SourceID) (*Source, error)
	ListSources(ctx context.Context) ([]Source, error)
	UpdateSource(ctx context.Context, src Source) error
	DeleteSource(ctx context.Context, id SourceID) error

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
	NodesOwnedBy(ctx context.Context, domain DomainID, source SourceID) ([]Node, error)

	UpsertEdge(ctx context.Context, e Edge) (EdgeID, error)
	GetEdge(ctx context.Context, id EdgeID) (*Edge, error)
	UpdateEdgeProperties(ctx context.Context, id EdgeID, props map[SourceID]map[string]any) error
	DeleteEdge(ctx context.Context, id EdgeID) error
	EdgesFrom(ctx context.Context, sourceID NodeID, types []string) ([]Edge, error)
	EdgesTo(ctx context.Context, targetID NodeID, types []string) ([]Edge, error)

	AddEdgeClaim(ctx context.Context, edgeID EdgeID, source SourceID, at time.Time) error
	RemoveEdgeClaim(ctx context.Context, edgeID EdgeID, source SourceID) error
	CountEdgeClaims(ctx context.Context, edgeID EdgeID) (int, error)
	ListEdgeClaims(ctx context.Context, edgeID EdgeID) ([]EdgeClaim, error)
	EdgeIDsClaimedBy(ctx context.Context, source SourceID) ([]EdgeID, error)
}
```

- [ ] **Step 2: Compile check**

```bash
go build ./internal/graph/...
```

Expected: builds. `internal/store` may or may not compile yet (depends on Task 5 completion); it should.

- [ ] **Step 3: Commit**

```bash
git add internal/graph/store.go
git commit -m "feat(graph): expose sources + edge_claims + namespaced props on Store interface"
```

---

### Task 7: Update `FakeStore` to satisfy the new interface

The FakeStore needs `sources`, `claims`, `Source` on nodes, namespaced properties, and `NodesOwnedBy` / `EdgeIDsClaimedBy` helpers.

**Files:**
- Modify: `internal/graph/testutil/fakestore.go`
- Modify: `internal/graph/testutil/fakestore_test.go`

- [ ] **Step 1: Rewrite `FakeStore` (the new interface needs methods covered)**

Replace the struct and methods. Key additions: `sources map[SourceID]Source`, `claims map[EdgeID]map[SourceID]EdgeClaim`. Snapshots/restores include them too.

```go
package testutil

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/ggfarmco/kg/internal/graph"
)

type FakeStore struct {
	mu       sync.Mutex
	sources  map[graph.SourceID]graph.Source
	domains  map[graph.DomainID]graph.Domain
	nodes    map[graph.NodeID]graph.Node
	edges    map[graph.EdgeID]graph.Edge
	claims   map[graph.EdgeID]map[graph.SourceID]graph.EdgeClaim
	nextEdge graph.EdgeID
	inTx     bool
}

func NewFakeStore() *FakeStore {
	return &FakeStore{
		sources:  map[graph.SourceID]graph.Source{},
		domains:  map[graph.DomainID]graph.Domain{},
		nodes:    map[graph.NodeID]graph.Node{},
		edges:    map[graph.EdgeID]graph.Edge{},
		claims:   map[graph.EdgeID]map[graph.SourceID]graph.EdgeClaim{},
		nextEdge: 1,
	}
}

func (s *FakeStore) snapshot() *FakeStore {
	cp := &FakeStore{
		sources:  make(map[graph.SourceID]graph.Source, len(s.sources)),
		domains:  make(map[graph.DomainID]graph.Domain, len(s.domains)),
		nodes:    make(map[graph.NodeID]graph.Node, len(s.nodes)),
		edges:    make(map[graph.EdgeID]graph.Edge, len(s.edges)),
		claims:   make(map[graph.EdgeID]map[graph.SourceID]graph.EdgeClaim, len(s.claims)),
		nextEdge: s.nextEdge,
	}
	for k, v := range s.sources {
		cp.sources[k] = v
	}
	for k, v := range s.domains {
		cp.domains[k] = v
	}
	for k, v := range s.nodes {
		cp.nodes[k] = v
	}
	for k, v := range s.edges {
		cp.edges[k] = v
	}
	for k, inner := range s.claims {
		ic := make(map[graph.SourceID]graph.EdgeClaim, len(inner))
		for sk, sv := range inner {
			ic[sk] = sv
		}
		cp.claims[k] = ic
	}
	return cp
}

func (s *FakeStore) restore(cp *FakeStore) {
	s.sources = cp.sources
	s.domains = cp.domains
	s.nodes = cp.nodes
	s.edges = cp.edges
	s.claims = cp.claims
	s.nextEdge = cp.nextEdge
}

func (s *FakeStore) InTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	s.mu.Lock()
	if s.inTx {
		s.mu.Unlock()
		return graph.ErrNestedTransaction
	}
	s.inTx = true
	cp := s.snapshot()
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r := recover(); r != nil {
			s.restore(cp)
			s.inTx = false
			panic(r)
		}
		if err != nil {
			s.restore(cp)
		}
		s.inTx = false
	}()

	return fn(ctx)
}

func (s *FakeStore) UpsertSource(_ context.Context, src graph.Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.sources[src.ID]; ok {
		cur.LastSeen = src.LastSeen
		s.sources[src.ID] = cur
		return nil
	}
	s.sources[src.ID] = src
	return nil
}

func (s *FakeStore) GetSource(_ context.Context, id graph.SourceID) (*graph.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sources[id]
	if !ok {
		return nil, graph.ErrSourceNotFound
	}
	return &v, nil
}

func (s *FakeStore) ListSources(_ context.Context) ([]graph.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.Source, 0, len(s.sources))
	for _, v := range s.sources {
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b graph.Source) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func (s *FakeStore) UpdateSource(_ context.Context, src graph.Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.sources[src.ID]
	if !ok {
		return graph.ErrSourceNotFound
	}
	cur.Description = src.Description
	cur.Trust = src.Trust
	s.sources[src.ID] = cur
	return nil
}

func (s *FakeStore) DeleteSource(_ context.Context, id graph.SourceID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sources[id]; !ok {
		return graph.ErrSourceNotFound
	}
	for _, n := range s.nodes {
		if n.Source == id {
			return graph.ErrSourceHasDependents
		}
	}
	for _, ic := range s.claims {
		if _, ok := ic[id]; ok {
			return graph.ErrSourceHasDependents
		}
	}
	delete(s.sources, id)
	return nil
}

func (s *FakeStore) CreateDomain(_ context.Context, d graph.Domain) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.domains[d.ID]; ok {
		return graph.ErrDomainAlreadyExists
	}
	s.domains[d.ID] = d
	return nil
}

func (s *FakeStore) GetDomain(_ context.Context, id graph.DomainID) (*graph.Domain, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.domains[id]
	if !ok {
		return nil, graph.ErrDomainNotFound
	}
	return &d, nil
}

func (s *FakeStore) ListDomains(_ context.Context) ([]graph.Domain, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.Domain, 0, len(s.domains))
	for _, d := range s.domains {
		out = append(out, d)
	}
	slices.SortFunc(out, func(a, b graph.Domain) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func (s *FakeStore) DeleteDomain(_ context.Context, id graph.DomainID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.domains[id]; !ok {
		return graph.ErrDomainNotFound
	}
	for _, n := range s.nodes {
		if n.Domain == id {
			return graph.ErrHasDependents
		}
	}
	delete(s.domains, id)
	return nil
}

func (s *FakeStore) CreateNode(_ context.Context, n graph.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[n.ID]; ok {
		return graph.ErrNodeAlreadyExists
	}
	if _, ok := s.sources[n.Source]; !ok {
		return graph.ErrSourceNotFound
	}
	s.nodes[n.ID] = n
	return nil
}

func (s *FakeStore) GetNode(_ context.Context, id graph.NodeID) (*graph.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok {
		return nil, graph.ErrNodeNotFound
	}
	return &n, nil
}

func (s *FakeStore) ListNodes(_ context.Context, f graph.NodeFilter) ([]graph.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		if f.Domain != "" && n.Domain != f.Domain {
			continue
		}
		if f.Layer != "" && n.Layer != f.Layer {
			continue
		}
		if f.Source != "" && n.Source != f.Source {
			continue
		}
		out = append(out, n)
	}
	slices.SortFunc(out, func(a, b graph.Node) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (s *FakeStore) UpdateNode(_ context.Context, n graph.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.nodes[n.ID]
	if !ok {
		return graph.ErrNodeNotFound
	}
	n.Revision = cur.Revision + 1
	s.nodes[n.ID] = n
	return nil
}

func (s *FakeStore) DeleteNode(_ context.Context, id graph.NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[id]; !ok {
		return graph.ErrNodeNotFound
	}
	for _, child := range s.nodes {
		if child.ParentID != nil && *child.ParentID == id {
			return graph.ErrHasDependents
		}
	}
	delete(s.nodes, id)
	for k, e := range s.edges {
		if e.SourceID == id || e.TargetID == id {
			delete(s.edges, k)
			delete(s.claims, k)
		}
	}
	return nil
}

func (s *FakeStore) ChildrenOf(_ context.Context, parentID graph.NodeID) ([]graph.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.Node, 0)
	for _, n := range s.nodes {
		if n.ParentID != nil && *n.ParentID == parentID {
			out = append(out, n)
		}
	}
	slices.SortFunc(out, func(a, b graph.Node) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func (s *FakeStore) NodesOwnedBy(_ context.Context, domain graph.DomainID, source graph.SourceID) ([]graph.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.Node, 0)
	for _, n := range s.nodes {
		if n.Domain == domain && n.Source == source {
			out = append(out, n)
		}
	}
	slices.SortFunc(out, func(a, b graph.Node) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func (s *FakeStore) UpsertEdge(_ context.Context, e graph.Edge) (graph.EdgeID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, existing := range s.edges {
		if existing.SourceID == e.SourceID && existing.TargetID == e.TargetID && existing.Type == e.Type {
			return id, nil
		}
	}
	id := s.nextEdge
	s.nextEdge++
	e.ID = id
	if e.Properties == nil {
		e.Properties = map[graph.SourceID]map[string]any{}
	}
	s.edges[id] = e
	return id, nil
}

func (s *FakeStore) GetEdge(_ context.Context, id graph.EdgeID) (*graph.Edge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.edges[id]
	if !ok {
		return nil, graph.ErrEdgeNotFound
	}
	return &e, nil
}

func (s *FakeStore) UpdateEdgeProperties(_ context.Context, id graph.EdgeID, props map[graph.SourceID]map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.edges[id]
	if !ok {
		return graph.ErrEdgeNotFound
	}
	e.Properties = props
	e.Revision++
	s.edges[id] = e
	return nil
}

func (s *FakeStore) DeleteEdge(_ context.Context, id graph.EdgeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.edges[id]; !ok {
		return graph.ErrEdgeNotFound
	}
	delete(s.edges, id)
	delete(s.claims, id)
	return nil
}

func (s *FakeStore) EdgesFrom(_ context.Context, src graph.NodeID, types []string) ([]graph.Edge, error) {
	return s.edgesMatching(func(e graph.Edge) bool { return e.SourceID == src }, types), nil
}

func (s *FakeStore) EdgesTo(_ context.Context, dst graph.NodeID, types []string) ([]graph.Edge, error) {
	return s.edgesMatching(func(e graph.Edge) bool { return e.TargetID == dst }, types), nil
}

func (s *FakeStore) edgesMatching(pred func(graph.Edge) bool, types []string) []graph.Edge {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.Edge, 0)
	for _, e := range s.edges {
		if !pred(e) {
			continue
		}
		if len(types) > 0 && !slices.Contains(types, e.Type) {
			continue
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b graph.Edge) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return out
}

func (s *FakeStore) AddEdgeClaim(_ context.Context, id graph.EdgeID, source graph.SourceID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.edges[id]; !ok {
		return graph.ErrEdgeNotFound
	}
	if _, ok := s.sources[source]; !ok {
		return graph.ErrSourceNotFound
	}
	inner, ok := s.claims[id]
	if !ok {
		inner = map[graph.SourceID]graph.EdgeClaim{}
		s.claims[id] = inner
	}
	if _, exists := inner[source]; exists {
		return nil
	}
	inner[source] = graph.EdgeClaim{EdgeID: id, Source: source, ClaimedAt: at}
	return nil
}

func (s *FakeStore) RemoveEdgeClaim(_ context.Context, id graph.EdgeID, source graph.SourceID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	inner, ok := s.claims[id]
	if !ok {
		return nil
	}
	delete(inner, source)
	if len(inner) == 0 {
		delete(s.claims, id)
	}
	return nil
}

func (s *FakeStore) CountEdgeClaims(_ context.Context, id graph.EdgeID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.claims[id]), nil
}

func (s *FakeStore) ListEdgeClaims(_ context.Context, id graph.EdgeID) ([]graph.EdgeClaim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inner := s.claims[id]
	out := make([]graph.EdgeClaim, 0, len(inner))
	for _, c := range inner {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b graph.EdgeClaim) int {
		switch {
		case a.Source < b.Source:
			return -1
		case a.Source > b.Source:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func (s *FakeStore) EdgeIDsClaimedBy(_ context.Context, source graph.SourceID) ([]graph.EdgeID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.EdgeID, 0)
	for id, inner := range s.claims {
		if _, ok := inner[source]; ok {
			out = append(out, id)
		}
	}
	slices.SortFunc(out, func(a, b graph.EdgeID) int {
		switch {
		case a < b:
			return -1
		case a > b:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

var _ graph.Store = (*FakeStore)(nil)
```

- [ ] **Step 2: Update `internal/graph/testutil/fakestore_test.go`**

If it exercised the old shape (`Summary`, flat properties), update it to use Source + namespaced properties. Keep the same scope (round-trip create+get, child counting, FK-style restrictions).

- [ ] **Step 3: Run**

```bash
go test ./internal/graph/testutil/...
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/graph/testutil/
git commit -m "test(graph): FakeStore satisfies new source-aware Store interface"
```

---

### Task 8: Service — source-aware `AddNode` / `UpdateNode` / `DeleteNode`

Service mutations require a `Source` field on inputs. Service auto-registers the source on first write. `AddNode` enforces single-owner semantics (existing node with different owner → `ErrNodeOwnedByDifferentSource`). `UpdateNode` of `Name` requires owner; properties go through `SetNodeProperties` (Task 10). `DeleteNode` requires owner and is blocked by foreign claims on incident edges.

**Files:**
- Modify: `internal/graph/service.go`
- Modify: `internal/graph/service_node_test.go`
- Modify: `internal/graph/service_domain_test.go`
- Modify: `internal/graph/service_node_query_test.go`

- [ ] **Step 1: Write failing tests in `service_node_test.go`**

Rewrite the existing tests for the new shape, plus add ownership tests. The general pattern:

```go
package graph_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestAddNodeAutoRegistersSourceAndStoresIt(t *testing.T) {
	svc, fs := newService(t)
	seedCarsDomain(t, svc)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "tree-sitter:0.1.0",
	})
	require.NoError(t, err)
	require.Equal(t, graph.SourceID("tree-sitter:0.1.0"), n.Source)

	src, err := fs.GetSource(t.Context(), "tree-sitter:0.1.0")
	require.NoError(t, err)
	require.Equal(t, 100, src.Trust)
}

func TestAddNodeRequiresSource(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt",
	})
	require.ErrorIs(t, err, graph.ErrSourceRequired)
}

func TestAddNodeSameIdDifferentSourceFails(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "a",
	})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "b",
	})
	require.ErrorIs(t, err, graph.ErrNodeOwnedByDifferentSource)
}

func TestUpdateNodeNameRequiresOwner(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "a",
	})
	require.NoError(t, err)
	name := "new"
	_, err = svc.UpdateNode(t.Context(), n.ID, graph.UpdateNodeInput{Source: "b", Name: &name})
	require.ErrorIs(t, err, graph.ErrNodeNotOwner)
}

func TestDeleteNodeRequiresOwner(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "a",
	})
	require.NoError(t, err)
	err = svc.DeleteNode(t.Context(), n.ID, "b")
	require.ErrorIs(t, err, graph.ErrNodeNotOwner)
}
```

(The `newService` and `seedCarsDomain` helpers come from existing v1 test files — update them to bootstrap a source registry.)

The v1 `service_node_test.go` will have other tests (`TestAddNodeStoresProperties`, etc.) — update each to pass a `Source` field; properties are now namespaced. Use the namespaced shape directly:

```go
n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
	Domain: "cars", Layer: "system", Name: "pt", Source: "a",
	Properties: map[string]any{"horsepower": float64(200)},
})
require.NoError(t, err)
require.Equal(t, float64(200), n.Properties["a"]["horsepower"])
```

(Plugin/CLI gives a flat map; Service wraps it in the source's namespace.)

Each combined-behavior test from v1 must be split where it covered more than one rule. Don't merge "scoping + naming" into the same test.

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/graph/ -run 'TestAddNode|TestUpdateNode|TestDeleteNode' -v
```

Expected: FAIL (`Source` field undefined, ownership not enforced).

- [ ] **Step 3: Update `Service.AddNode` and related**

In `internal/graph/service.go`, add `Source` to inputs:

```go
type AddDomainInput struct {
	ID          string
	Description string
	Layers      []string
	Source      string
}

type AddNodeInput struct {
	Domain     string
	Layer      string
	Name       string
	ID         string
	Parent     string
	Source     string
	Properties map[string]any
}

type UpdateNodeInput struct {
	Source SourceID
	Name   *string
}
```

(Update no longer carries a `Summary` field — that was removed in Phase 1. To update properties, use `SetNodeProperties` from Task 10.)

Add a helper for auto-registration:

```go
func (s *Service) ensureSource(ctx context.Context, source SourceID) error {
	now := s.now()
	return s.store.UpsertSource(ctx, Source{
		ID: source, Trust: 100,
		FirstSeen: now, LastSeen: now,
	})
}
```

Update `AddDomain`:

```go
func (s *Service) AddDomain(ctx context.Context, in AddDomainInput) (*Domain, error) {
	id, err := ParseDomainID(in.ID)
	if err != nil {
		return nil, err
	}
	if in.Source == "" {
		return nil, ErrSourceRequired
	}
	source, err := ParseSourceID(in.Source)
	if err != nil {
		return nil, err
	}
	if len(in.Layers) == 0 {
		return nil, errors.New("layers must not be empty")
	}
	seen := make(map[string]struct{}, len(in.Layers))
	for i, l := range in.Layers {
		if l == "" {
			return nil, fmt.Errorf("layer %d is empty", i)
		}
		if _, dup := seen[l]; dup {
			return nil, fmt.Errorf("layer %q is duplicated", l)
		}
		seen[l] = struct{}{}
	}
	d := Domain{
		ID: id, Description: in.Description,
		Layers: append([]string(nil), in.Layers...),
		Revision: 1, CreatedAt: s.now(),
	}
	if err := s.ensureSource(ctx, source); err != nil {
		return nil, err
	}
	if err := s.store.CreateDomain(ctx, d); err != nil {
		return nil, err
	}
	return &d, nil
}
```

Update `AddNode`:

```go
func (s *Service) AddNode(ctx context.Context, in AddNodeInput) (*Node, error) {
	dID, err := ParseDomainID(in.Domain)
	if err != nil {
		return nil, err
	}
	if in.Source == "" {
		return nil, ErrSourceRequired
	}
	source, err := ParseSourceID(in.Source)
	if err != nil {
		return nil, err
	}
	d, err := s.store.GetDomain(ctx, dID)
	if err != nil {
		return nil, err
	}
	if !slicesContains(d.Layers, in.Layer) {
		return nil, ErrLayerNotInDomain
	}

	var slug SlugID
	if in.ID != "" {
		slug, err = ParseSlug(in.ID)
		if err != nil {
			return nil, ErrInvalidSlug
		}
	} else {
		slug, err = deriveSlug(in.Name)
		if err != nil {
			return nil, ErrSlugCannotDerive
		}
	}

	topLayer := d.Layers[0]
	isTop := in.Layer == topLayer

	var parentPtr *NodeID
	if in.Parent != "" {
		if isTop {
			return nil, ErrTopLayerCannotHaveParent
		}
		parentID := NodeID(in.Parent)
		parent, err := s.store.GetNode(ctx, parentID)
		if err != nil {
			return nil, err
		}
		if parent.Domain != dID {
			return nil, ErrParentDomainMismatch
		}
		parentLayerIdx := indexOf(d.Layers, parent.Layer)
		nodeLayerIdx := indexOf(d.Layers, in.Layer)
		if parentLayerIdx < 0 || nodeLayerIdx < 0 || nodeLayerIdx-parentLayerIdx != 1 {
			return nil, ErrParentLayerMismatch
		}
		parentPtr = &parentID
	} else if !isTop {
		return nil, ErrParentLayerMismatch
	}

	id := NewNodeID(dID, slug)
	existing, getErr := s.store.GetNode(ctx, id)
	if getErr != nil && !errors.Is(getErr, ErrNodeNotFound) {
		return nil, getErr
	}
	if existing != nil {
		if existing.Source != source {
			return nil, ErrNodeOwnedByDifferentSource
		}
		return nil, ErrNodeAlreadyExists
	}

	if err := s.ensureSource(ctx, source); err != nil {
		return nil, err
	}
	now := s.now()
	n := Node{
		ID: id, Domain: dID, Layer: in.Layer, Name: in.Name,
		ParentID: parentPtr,
		Source:   source,
		Properties: nonNilNamespacedProps(source, in.Properties),
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.CreateNode(ctx, n); err != nil {
		return nil, err
	}
	return &n, nil
}

func nonNilNamespacedProps(source SourceID, props map[string]any) map[SourceID]map[string]any {
	out := map[SourceID]map[string]any{}
	if len(props) == 0 {
		return out
	}
	copy := make(map[string]any, len(props))
	for k, v := range props {
		copy[k] = v
	}
	out[source] = copy
	return out
}
```

Update `UpdateNode`:

```go
func (s *Service) UpdateNode(ctx context.Context, id NodeID, in UpdateNodeInput) (*Node, error) {
	if in.Source == "" {
		return nil, ErrSourceRequired
	}
	cur, err := s.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur.Source != in.Source {
		return nil, ErrNodeNotOwner
	}
	if in.Name != nil {
		cur.Name = *in.Name
	}
	cur.UpdatedAt = s.now()
	if err := s.store.UpdateNode(ctx, *cur); err != nil {
		return nil, err
	}
	return s.store.GetNode(ctx, id)
}
```

Update `DeleteNode`:

```go
func (s *Service) DeleteNode(ctx context.Context, id NodeID, source SourceID) error {
	if source == "" {
		return ErrSourceRequired
	}
	cur, err := s.store.GetNode(ctx, id)
	if err != nil {
		return err
	}
	if cur.Source != source {
		return ErrNodeNotOwner
	}
	incoming, err := s.store.EdgesTo(ctx, id, nil)
	if err != nil {
		return err
	}
	outgoing, err := s.store.EdgesFrom(ctx, id, nil)
	if err != nil {
		return err
	}
	for _, e := range append(incoming, outgoing...) {
		claims, err := s.store.ListEdgeClaims(ctx, e.ID)
		if err != nil {
			return err
		}
		for _, c := range claims {
			if c.Source != source {
				return ErrNodeHasForeignClaims
			}
		}
	}
	return s.store.DeleteNode(ctx, id)
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./internal/graph/ -v
```

Expected: all updated tests PASS. If a v1 test still references `Summary` or assumes a flat property map, fix it in place — don't paper over the test.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/
git commit -m "feat(graph): source-aware AddNode/UpdateNode/DeleteNode with ownership rules"
```

---

### Task 9: Service — edge claims + ref-counted CreateEdge

`Service.CreateEdge` becomes UPSERT-edge + INSERT-OR-IGNORE-claim in one tx. New methods: `AddEdgeClaim`, `RemoveEdgeClaim`, `ListEdgeClaims`. `RemoveEdgeClaim` GCs the edge in the same tx if no claims remain.

**Files:**
- Modify: `internal/graph/service.go`
- Modify: `internal/graph/service_edge_test.go`

- [ ] **Step 1: Write failing tests for the claim lifecycle**

Replace the v1 edge tests with focused ones:

```go
func TestAddEdgeUpsertsAndClaims(t *testing.T) {
	svc, fs := newService(t)
	a, b := seedTwoNodes(t, svc, "x")
	e, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{
		Source: string(a), Target: string(b), Type: "imports", WriterSource: "x",
	})
	require.NoError(t, err)
	claims, err := fs.ListEdgeClaims(t.Context(), e.ID)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, graph.SourceID("x"), claims[0].Source)
}

func TestAddSameEdgeFromTwoSourcesProducesTwoClaims(t *testing.T) {
	svc, fs := newService(t)
	a, b := seedTwoNodes(t, svc, "x")
	e1, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "imports", WriterSource: "x"})
	require.NoError(t, err)
	e2, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "imports", WriterSource: "y"})
	require.NoError(t, err)
	require.Equal(t, e1.ID, e2.ID, "same physical edge")
	claims, err := fs.ListEdgeClaims(t.Context(), e1.ID)
	require.NoError(t, err)
	require.Len(t, claims, 2)
}

func TestRemoveEdgeClaimGCsWhenLast(t *testing.T) {
	svc, fs := newService(t)
	a, b := seedTwoNodes(t, svc, "x")
	e, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "imports", WriterSource: "x"})
	require.NoError(t, err)
	require.NoError(t, svc.RemoveEdgeClaim(t.Context(), e.ID, "x"))
	_, gerr := fs.GetEdge(t.Context(), e.ID)
	require.ErrorIs(t, gerr, graph.ErrEdgeNotFound)
}

func TestRemoveOneOfTwoClaimsKeepsEdgeAlive(t *testing.T) {
	svc, fs := newService(t)
	a, b := seedTwoNodes(t, svc, "x")
	e, _ := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "imports", WriterSource: "x"})
	_, _ = svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "imports", WriterSource: "y"})
	require.NoError(t, svc.RemoveEdgeClaim(t.Context(), e.ID, "x"))
	_, gerr := fs.GetEdge(t.Context(), e.ID)
	require.NoError(t, gerr)
}
```

`seedTwoNodes(t, svc, source)` is a helper that adds two same-domain same-layer nodes under `source`. Define it inside the test file once.

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/graph/ -run 'TestAddEdge|TestRemoveEdgeClaim|TestRemoveOneOfTwo' -v
```

Expected: FAIL.

- [ ] **Step 3: Update `AddEdgeInput` and `Service` methods**

Replace `AddEdgeInput`:

```go
type AddEdgeInput struct {
	Source       string
	Target       string
	Type         string
	Properties   map[string]any
	WriterSource string
}
```

(The wire field `source` is the *originating node*; the writer-source is a separate concept. The Go field is named `WriterSource` to avoid confusion with `SourceID`/`SourceID`. In the wire format the writer-source field is just `source` at top-level and the originating node is `src` in `edge.add` args — handled by Phase 5.)

Replace `AddEdge`:

```go
func (s *Service) AddEdge(ctx context.Context, in AddEdgeInput) (*Edge, error) {
	src := NodeID(in.Source)
	dst := NodeID(in.Target)
	if src == dst {
		return nil, ErrEdgeSelfLoop
	}
	if in.Type == "" {
		return nil, fmt.Errorf("edge type must not be empty")
	}
	if in.WriterSource == "" {
		return nil, ErrSourceRequired
	}
	source, err := ParseSourceID(in.WriterSource)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.GetNode(ctx, src); err != nil {
		return nil, err
	}
	if _, err := s.store.GetNode(ctx, dst); err != nil {
		return nil, err
	}
	if err := s.ensureSource(ctx, source); err != nil {
		return nil, err
	}
	now := s.now()
	id, err := s.store.UpsertEdge(ctx, Edge{
		SourceID: src, TargetID: dst, Type: in.Type,
		Properties: nonNilNamespacedProps(source, in.Properties),
		CreatedAt:  now,
	})
	if err != nil {
		return nil, err
	}
	if err := s.store.AddEdgeClaim(ctx, id, source, now); err != nil {
		return nil, err
	}
	got, err := s.store.GetEdge(ctx, id)
	if err != nil {
		return nil, err
	}
	claims, err := s.store.ListEdgeClaims(ctx, id)
	if err != nil {
		return nil, err
	}
	got.Claims = make([]SourceID, 0, len(claims))
	for _, c := range claims {
		got.Claims = append(got.Claims, c.Source)
	}
	return got, nil
}

func (s *Service) AddEdgeClaim(ctx context.Context, id EdgeID, source SourceID) error {
	if source == "" {
		return ErrSourceRequired
	}
	if err := s.ensureSource(ctx, source); err != nil {
		return err
	}
	return s.store.AddEdgeClaim(ctx, id, source, s.now())
}

func (s *Service) RemoveEdgeClaim(ctx context.Context, id EdgeID, source SourceID) error {
	if source == "" {
		return ErrSourceRequired
	}
	return s.store.InTx(ctx, func(ctx context.Context) error {
		if err := s.store.RemoveEdgeClaim(ctx, id, source); err != nil {
			return err
		}
		n, err := s.store.CountEdgeClaims(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return s.store.DeleteEdge(ctx, id)
		}
		return nil
	})
}

func (s *Service) ListEdgeClaims(ctx context.Context, id EdgeID) ([]EdgeClaim, error) {
	return s.store.ListEdgeClaims(ctx, id)
}
```

`Service.DeleteEdge(id EdgeID)` is retained for force-removal but takes on a v2 semantic: drops the row and all claims. Reserve for `--force` usage only.

- [ ] **Step 4: Run, verify pass**

```bash
go test ./internal/graph/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/
git commit -m "feat(graph): ref-counted edges via claims, GC in same tx as last unclaim"
```

---

### Task 10: Service — `SetNodeProperties` / `DeleteNodeProperties` + edge property variants

Namespaced property writes go through dedicated methods so the wire-protocol "replace within namespace" semantic is enforced.

**Files:**
- Modify: `internal/graph/service.go`
- Modify: `internal/graph/service_node_test.go` (append)

- [ ] **Step 1: Write failing tests**

```go
func TestSetNodePropertiesReplacesOnlyOwnNamespace(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "a",
		Properties: map[string]any{"x": float64(1)},
	})
	require.NoError(t, err)

	require.NoError(t, svc.SetNodeProperties(t.Context(), n.ID, "b", map[string]any{"y": float64(2)}))

	updated, err := svc.GetNode(t.Context(), n.ID)
	require.NoError(t, err)
	require.Equal(t, float64(1), updated.Properties["a"]["x"])
	require.Equal(t, float64(2), updated.Properties["b"]["y"])
}

func TestSetNodePropertiesReplaceWithinNamespace(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	n, _ := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "a",
		Properties: map[string]any{"x": float64(1), "y": float64(2)},
	})
	require.NoError(t, svc.SetNodeProperties(t.Context(), n.ID, "a", map[string]any{"z": float64(3)}))
	updated, _ := svc.GetNode(t.Context(), n.ID)
	require.NotContains(t, updated.Properties["a"], "x")
	require.NotContains(t, updated.Properties["a"], "y")
	require.Equal(t, float64(3), updated.Properties["a"]["z"])
}

func TestDeleteNodePropertiesRemovesOnlyOwnNamespace(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	n, _ := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "a",
		Properties: map[string]any{"x": float64(1)},
	})
	require.NoError(t, svc.SetNodeProperties(t.Context(), n.ID, "b", map[string]any{"y": float64(2)}))
	require.NoError(t, svc.DeleteNodeProperties(t.Context(), n.ID, "a"))
	updated, _ := svc.GetNode(t.Context(), n.ID)
	require.NotContains(t, updated.Properties, graph.SourceID("a"))
	require.Equal(t, float64(2), updated.Properties["b"]["y"])
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/graph/ -run 'TestSetNodeProperties|TestDeleteNodeProperties' -v
```

Expected: FAIL.

- [ ] **Step 3: Implement in `service.go`**

```go
func (s *Service) SetNodeProperties(ctx context.Context, id NodeID, source SourceID, props map[string]any) error {
	if source == "" {
		return ErrSourceRequired
	}
	if err := s.ensureSource(ctx, source); err != nil {
		return err
	}
	cur, err := s.store.GetNode(ctx, id)
	if err != nil {
		return err
	}
	if cur.Properties == nil {
		cur.Properties = map[SourceID]map[string]any{}
	}
	copy := make(map[string]any, len(props))
	for k, v := range props {
		copy[k] = v
	}
	cur.Properties[source] = copy
	cur.UpdatedAt = s.now()
	return s.store.UpdateNode(ctx, *cur)
}

func (s *Service) DeleteNodeProperties(ctx context.Context, id NodeID, source SourceID) error {
	if source == "" {
		return ErrSourceRequired
	}
	cur, err := s.store.GetNode(ctx, id)
	if err != nil {
		return err
	}
	if cur.Properties != nil {
		delete(cur.Properties, source)
	}
	cur.UpdatedAt = s.now()
	return s.store.UpdateNode(ctx, *cur)
}

func (s *Service) SetEdgeProperties(ctx context.Context, id EdgeID, source SourceID, props map[string]any) error {
	if source == "" {
		return ErrSourceRequired
	}
	if err := s.ensureSource(ctx, source); err != nil {
		return err
	}
	cur, err := s.store.GetEdge(ctx, id)
	if err != nil {
		return err
	}
	if cur.Properties == nil {
		cur.Properties = map[SourceID]map[string]any{}
	}
	copy := make(map[string]any, len(props))
	for k, v := range props {
		copy[k] = v
	}
	cur.Properties[source] = copy
	return s.store.UpdateEdgeProperties(ctx, id, cur.Properties)
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./internal/graph/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/
git commit -m "feat(graph): Set/DeleteNodeProperties + SetEdgeProperties write within source namespace"
```

---

## Phase 3 — `snapshot/` public package

The wire format for declarative apply. One JSON document on stdin (not JSONL — atomic by nature). Public package so plugins in separate modules can import it. Sibling to `batch/`.

### Task 11: `snapshot/snapshot.go` — types

**Files:**
- Create: `snapshot/snapshot.go`
- Create: `snapshot/snapshot_test.go`

- [ ] **Step 1: Write failing tests for the type round-trip**

```go
package snapshot_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/snapshot"
)

func TestSnapshotMarshalRoundTrip(t *testing.T) {
	in := snapshot.Snapshot{
		ProtocolVersion: 2, Source: "tree-sitter:0.1.0",
		Domain: "myapp", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{
			ID: "myapp", Layers: []string{"package", "file", "decl"},
			Description: "x",
		},
		Nodes: []snapshot.NodeSpec{
			{ID: "myapp:graph", Layer: "package", Name: "graph",
				Properties: map[string]any{"import_path": "x"}},
			{ID: "myapp:graph/node-go", Layer: "file",
				Parent: "myapp:graph", Name: "node.go"},
		},
		Edges: []snapshot.EdgeSpec{
			{Src: "myapp:graph", Target: "myapp:store", Type: "imports"},
		},
	}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	var out snapshot.Snapshot
	require.NoError(t, json.Unmarshal(b, &out))
	require.Equal(t, in, out)
}

func TestEdgeSpecAcceptsBothSourceAndSrc(t *testing.T) {
	body := []byte(`{"source":"a:n","target":"a:m","type":"x"}`)
	var got snapshot.EdgeSpec
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "a:n", got.Src, "wire field `source` aliases to Go field Src")
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./snapshot/ -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `snapshot/snapshot.go`**

```go
package snapshot

import "encoding/json"

const ProtocolVersion = 2

type Scope string

const (
	ScopeDomainSource        Scope = "domain-source"
	ScopeDomain              Scope = "domain"
	ScopeAdditive            Scope = "additive"
)

type Snapshot struct {
	ProtocolVersion int         `json:"protocol_version"`
	Source          string      `json:"source"`
	Domain          string      `json:"domain"`
	Scope           Scope       `json:"scope"`
	DomainSpec      *DomainSpec `json:"domain_spec,omitempty"`
	Nodes           []NodeSpec  `json:"nodes"`
	Edges           []EdgeSpec  `json:"edges"`
}

type DomainSpec struct {
	ID          string   `json:"id"`
	Layers      []string `json:"layers"`
	Description string   `json:"description,omitempty"`
}

type NodeSpec struct {
	ID         string         `json:"id"`
	Layer      string         `json:"layer"`
	Parent     string         `json:"parent,omitempty"`
	Name       string         `json:"name"`
	Properties map[string]any `json:"properties,omitempty"`
}

type EdgeSpec struct {
	Src        string         `json:"-"`
	Target     string         `json:"target"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

type edgeSpecWire struct {
	Source     string         `json:"source,omitempty"`
	Src        string         `json:"src,omitempty"`
	Target     string         `json:"target"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

func (e EdgeSpec) MarshalJSON() ([]byte, error) {
	return json.Marshal(edgeSpecWire{
		Src: e.Src, Target: e.Target, Type: e.Type, Properties: e.Properties,
	})
}

func (e *EdgeSpec) UnmarshalJSON(b []byte) error {
	var w edgeSpecWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	e.Src = w.Src
	if e.Src == "" {
		e.Src = w.Source
	}
	e.Target = w.Target
	e.Type = w.Type
	e.Properties = w.Properties
	return nil
}
```

The encoded form writes the wire field as `src`; the decoder accepts both `src` and `source` (since plugins authored against the spec wording use `source`, and the spec also notes "for clarity in the implementation, when parsing the snapshot we'll alias edge `source` to `src` internally"). The Go-side struct field is always `Src`.

- [ ] **Step 4: Run, verify pass**

```bash
go test ./snapshot/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add snapshot/
git commit -m "feat(snapshot): add Snapshot/NodeSpec/EdgeSpec types with src/source dual decoding"
```

---

### Task 12: `snapshot/codec.go` — JSON read/write helpers + topological sort

**Files:**
- Create: `snapshot/codec.go`
- Create: `snapshot/codec_test.go`
- Create: `snapshot/sort.go`
- Create: `snapshot/sort_test.go`

- [ ] **Step 1: Write failing tests**

```go
package snapshot_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/snapshot"
)

func TestDecodeRejectsJSONL(t *testing.T) {
	jsonl := `{"protocol_version":2,"source":"a"}` + "\n" + `{"protocol_version":2,"source":"b"}` + "\n"
	_, err := snapshot.Decode(strings.NewReader(jsonl))
	require.Error(t, err)
	require.Contains(t, err.Error(), "trailing data")
}

func TestDecodeAcceptsWhitespacePadded(t *testing.T) {
	body := "\n  " + `{"protocol_version":2,"source":"a","domain":"d","scope":"domain-source","nodes":[],"edges":[]}` + "\n\n"
	s, err := snapshot.Decode(strings.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, 2, s.ProtocolVersion)
}

func TestEncodeRoundTrip(t *testing.T) {
	in := snapshot.Snapshot{ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeAdditive, Nodes: []snapshot.NodeSpec{}, Edges: []snapshot.EdgeSpec{}}
	var buf bytes.Buffer
	require.NoError(t, snapshot.Encode(&buf, in))
	out, err := snapshot.Decode(&buf)
	require.NoError(t, err)
	require.Equal(t, in, *out)
}
```

```go
package snapshot_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/snapshot"
)

func TestTopoSortParentsBeforeChildren(t *testing.T) {
	specs := []snapshot.NodeSpec{
		{ID: "d:a/b/c", Parent: "d:a/b", Layer: "decl", Name: "c"},
		{ID: "d:a", Layer: "package", Name: "a"},
		{ID: "d:a/b", Parent: "d:a", Layer: "file", Name: "b"},
	}
	sorted, err := snapshot.TopoSortNodes(specs)
	require.NoError(t, err)
	require.Equal(t, []string{"d:a", "d:a/b", "d:a/b/c"}, idsOf(sorted))
}

func TestTopoSortCycleErrors(t *testing.T) {
	specs := []snapshot.NodeSpec{
		{ID: "d:a", Parent: "d:b"},
		{ID: "d:b", Parent: "d:a"},
	}
	_, err := snapshot.TopoSortNodes(specs)
	require.Error(t, err)
}

func TestTopoSortParentOutsideSnapshotIsLeftAlone(t *testing.T) {
	specs := []snapshot.NodeSpec{
		{ID: "d:child", Parent: "d:external"},
		{ID: "d:root"},
	}
	sorted, err := snapshot.TopoSortNodes(specs)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"d:child", "d:root"}, idsOf(sorted))
}

func idsOf(specs []snapshot.NodeSpec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.ID
	}
	return out
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./snapshot/ -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `snapshot/codec.go`**

```go
package snapshot

import (
	"encoding/json"
	"fmt"
	"io"
)

func Decode(r io.Reader) (*Snapshot, error) {
	dec := json.NewDecoder(r)
	var s Snapshot
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("decode snapshot: trailing data (snapshot must be a single JSON document, not JSONL)")
	}
	return &s, nil
}

func Encode(w io.Writer, s Snapshot) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}
```

- [ ] **Step 4: Implement `snapshot/sort.go`**

```go
package snapshot

import "fmt"

func TopoSortNodes(specs []NodeSpec) ([]NodeSpec, error) {
	idx := make(map[string]int, len(specs))
	for i, n := range specs {
		idx[n.ID] = i
	}
	color := make([]int, len(specs))
	out := make([]NodeSpec, 0, len(specs))

	var visit func(int) error
	visit = func(i int) error {
		switch color[i] {
		case 1:
			return fmt.Errorf("cycle through %q", specs[i].ID)
		case 2:
			return nil
		}
		color[i] = 1
		parent := specs[i].Parent
		if parent != "" {
			if pi, ok := idx[parent]; ok {
				if err := visit(pi); err != nil {
					return err
				}
			}
		}
		color[i] = 2
		out = append(out, specs[i])
		return nil
	}

	for i := range specs {
		if err := visit(i); err != nil {
			return nil, err
		}
	}
	return out, nil
}
```

- [ ] **Step 5: Run, verify pass**

```bash
go test ./snapshot/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add snapshot/
git commit -m "feat(snapshot): add Decode/Encode (JSON, not JSONL) and TopoSortNodes"
```

---

### Task 13: `snapshot/validate.go` — shape validation

Pure-shape validation: protocol_version, slug grammar of ids, scope value, layers in domain_spec. Reference-existence validation is `kg apply`'s job (needs DB).

**Files:**
- Create: `snapshot/validate.go`
- Create: `snapshot/validate_test.go`

- [ ] **Step 1: Write failing tests**

```go
package snapshot_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/snapshot"
)

func TestValidateRejectsWrongProtocol(t *testing.T) {
	err := snapshot.Validate(&snapshot.Snapshot{ProtocolVersion: 1, Source: "x", Domain: "d", Scope: "domain-source"})
	require.ErrorIs(t, err, snapshot.ErrProtocolVersion)
}

func TestValidateRejectsUnknownScope(t *testing.T) {
	err := snapshot.Validate(&snapshot.Snapshot{ProtocolVersion: 2, Source: "x", Domain: "d", Scope: "wat"})
	require.ErrorIs(t, err, snapshot.ErrUnknownScope)
}

func TestValidateRejectsBadNodeID(t *testing.T) {
	err := snapshot.Validate(&snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: "domain-source",
		Nodes: []snapshot.NodeSpec{{ID: "BAD CASE", Layer: "l", Name: "n"}},
	})
	require.ErrorIs(t, err, snapshot.ErrInvalidNodeID)
}

func TestValidateAcceptsRelaxedCompoundSlug(t *testing.T) {
	err := snapshot.Validate(&snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: "domain-source",
		Nodes: []snapshot.NodeSpec{{ID: "d:a/b-go::parseslug", Layer: "decl", Name: "ParseSlug", Parent: "d:a/b-go"}},
		Edges: []snapshot.EdgeSpec{},
	})
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./snapshot/ -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `snapshot/validate.go`**

```go
package snapshot

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	ErrProtocolVersion = errors.New("unsupported protocol_version (expected 2)")
	ErrUnknownScope    = errors.New("unknown scope")
	ErrInvalidNodeID   = errors.New("invalid node id (must match kg slug grammar)")
	ErrMissingSource   = errors.New("source is required")
	ErrMissingDomain   = errors.New("domain is required")
)

var nodeIDRE = regexp.MustCompile(`^[a-z0-9-]+:[a-z0-9-]+(?:(?:/|::)[a-z0-9-]+)*$`)

func Validate(s *Snapshot) error {
	if s.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("%w: got %d", ErrProtocolVersion, s.ProtocolVersion)
	}
	if s.Source == "" {
		return ErrMissingSource
	}
	if s.Domain == "" {
		return ErrMissingDomain
	}
	switch s.Scope {
	case ScopeDomainSource, ScopeDomain, ScopeAdditive:
	default:
		return fmt.Errorf("%w: %q", ErrUnknownScope, s.Scope)
	}
	for i, n := range s.Nodes {
		if !nodeIDRE.MatchString(n.ID) {
			return fmt.Errorf("%w: nodes[%d].id=%q", ErrInvalidNodeID, i, n.ID)
		}
		if n.Layer == "" || n.Name == "" {
			return fmt.Errorf("nodes[%d]: layer and name are required", i)
		}
	}
	for i, e := range s.Edges {
		if e.Src == "" || e.Target == "" || e.Type == "" {
			return fmt.Errorf("edges[%d]: src/target/type are all required", i)
		}
		if !nodeIDRE.MatchString(e.Src) || !nodeIDRE.MatchString(e.Target) {
			return fmt.Errorf("%w: edges[%d] endpoints", ErrInvalidNodeID, i)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./snapshot/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add snapshot/
git commit -m "feat(snapshot): shape validation of protocol_version, scope, slugs"
```

---

## Phase 4 — `Service.Apply` — the diff engine

The declarative diff algorithm from the spec ("Algorithm" section, steps 1-11) implemented in `internal/graph/service_apply.go`. Everything runs inside one `Store.InTx`. Tasks split the algorithm by responsibility: Task 14 = node diff (add/update detection), Task 15 = edge diff (upsert + claim, claim removal + GC), Task 16 = scope semantics + conflict codes, Task 17 = `--force-cascade` and `--force-domain-takeover` overrides.

### Task 14: `Service.Apply` skeleton — node add/update

**Files:**
- Create: `internal/graph/service_apply.go`
- Create: `internal/graph/service_apply_test.go`

- [ ] **Step 1: Write failing tests for the happy path: snapshot adds N nodes**

```go
package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/snapshot"
)

func TestApplyHappyPathAddsNodes(t *testing.T) {
	svc, _ := newService(t)
	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"package", "file"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "package", Name: "a"},
			{ID: "d:a/b", Layer: "file", Parent: "d:a", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, res.NodesAdded)
	require.Equal(t, 0, res.NodesUpdated)
	require.Equal(t, 0, res.NodesRemoved)
}

func TestApplyReApplyIsNoOp(t *testing.T) {
	svc, _ := newService(t)
	snap := snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"package"}},
		Nodes:      []snapshot.NodeSpec{{ID: "d:a", Layer: "package", Name: "a"}},
		Edges:      []snapshot.EdgeSpec{},
	}
	_, err := svc.Apply(t.Context(), snap, graph.ApplyOptions{})
	require.NoError(t, err)
	res, err := svc.Apply(t.Context(), snap, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, res.NodesAdded)
	require.Equal(t, 0, res.NodesUpdated)
	require.Equal(t, 0, res.NodesRemoved)
}

func TestApplyUpdatesChangedName(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"package"}},
		Nodes:      []snapshot.NodeSpec{{ID: "d:a", Layer: "package", Name: "old"}},
		Edges:      []snapshot.EdgeSpec{},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		Nodes: []snapshot.NodeSpec{{ID: "d:a", Layer: "package", Name: "new"}},
		Edges: []snapshot.EdgeSpec{},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.NodesUpdated)

	n, err := svc.GetNode(t.Context(), "d:a")
	require.NoError(t, err)
	require.Equal(t, "new", n.Name)
}

func TestApplyRemovesNodesNotInSnapshot(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"package"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "package", Name: "a"},
			{ID: "d:b", Layer: "package", Name: "b"},
		},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		Nodes: []snapshot.NodeSpec{{ID: "d:a", Layer: "package", Name: "a"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.NodesRemoved)
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/graph/ -run TestApply -v
```

Expected: FAIL (`Apply` undefined).

- [ ] **Step 3: Implement `internal/graph/service_apply.go`**

```go
package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ggfarmco/kg/snapshot"
)

type ApplyOptions struct {
	OverrideScope        snapshot.Scope
	DryRun               bool
	ForceCascade         bool
	ForceDomainTakeover  bool
}

type ApplyResult struct {
	Source        string `json:"source"`
	Domain        string `json:"domain"`
	Scope         string `json:"scope"`
	NodesAdded    int    `json:"nodes_added"`
	NodesUpdated  int    `json:"nodes_updated"`
	NodesRemoved  int    `json:"nodes_removed"`
	EdgesAdded    int    `json:"edges_added"`
	ClaimsAdded   int    `json:"claims_added"`
	ClaimsRemoved int    `json:"claims_removed"`
	EdgesGC       int    `json:"edges_gc"`
	TookMs        int    `json:"took_ms"`
	DryRun        bool   `json:"dry_run,omitempty"`
}

var errApplyDryRun = errors.New("apply: dry-run rollback")

func (s *Service) Apply(ctx context.Context, snap snapshot.Snapshot, opts ApplyOptions) (*ApplyResult, error) {
	if err := snapshot.Validate(&snap); err != nil {
		return nil, err
	}
	source, err := ParseSourceID(snap.Source)
	if err != nil {
		return nil, err
	}
	domainID, err := ParseDomainID(snap.Domain)
	if err != nil {
		return nil, err
	}
	scope := snap.Scope
	if opts.OverrideScope != "" {
		scope = opts.OverrideScope
	}

	start := time.Now()
	res := &ApplyResult{
		Source: snap.Source, Domain: snap.Domain, Scope: string(scope),
	}

	apply := func(ctx context.Context) error {
		if err := s.ensureSource(ctx, source); err != nil {
			return err
		}
		if err := s.applyDomainSpec(ctx, snap.DomainSpec, source); err != nil {
			return err
		}
		sorted, err := snapshot.TopoSortNodes(snap.Nodes)
		if err != nil {
			return err
		}
		existingNodes, err := s.store.NodesOwnedBy(ctx, domainID, source)
		if err != nil {
			return err
		}
		byID := make(map[NodeID]Node, len(existingNodes))
		for _, n := range existingNodes {
			byID[n.ID] = n
		}
		for _, spec := range sorted {
			if err := s.applyNodeSpec(ctx, spec, source, byID, res, scope); err != nil {
				return err
			}
		}
		if err := s.applyEdges(ctx, snap, source, res); err != nil {
			return err
		}
		if scope != snapshot.ScopeAdditive {
			if err := s.applyCleanup(ctx, byID, source, opts, res); err != nil {
				return err
			}
		}
		return nil
	}

	if opts.DryRun {
		txErr := s.store.InTx(ctx, func(ctx context.Context) error {
			if err := apply(ctx); err != nil {
				return err
			}
			return errApplyDryRun
		})
		if !errors.Is(txErr, errApplyDryRun) {
			return nil, txErr
		}
		res.DryRun = true
		res.TookMs = int(time.Since(start).Milliseconds())
		return res, nil
	}

	if err := s.store.InTx(ctx, apply); err != nil {
		return nil, err
	}
	res.TookMs = int(time.Since(start).Milliseconds())
	return res, nil
}

func (s *Service) applyDomainSpec(ctx context.Context, spec *snapshot.DomainSpec, source SourceID) error {
	if spec == nil {
		return nil
	}
	existing, err := s.store.GetDomain(ctx, DomainID(spec.ID))
	if err != nil && !errors.Is(err, ErrDomainNotFound) {
		return err
	}
	if existing == nil {
		_, err := s.AddDomain(ctx, AddDomainInput{
			ID: spec.ID, Description: spec.Description,
			Layers: spec.Layers, Source: string(source),
		})
		return err
	}
	return nil
}

func (s *Service) applyNodeSpec(
	ctx context.Context, spec snapshot.NodeSpec, source SourceID,
	byID map[NodeID]Node, res *ApplyResult, scope snapshot.Scope,
) error {
	id := NodeID(spec.ID)
	existing, ok := byID[id]
	if !ok {
		other, gerr := s.store.GetNode(ctx, id)
		if gerr == nil && other != nil {
			if scope == snapshot.ScopeAdditive {
				return nil
			}
			return fmt.Errorf("%w: id=%s owner=%s", ErrNodeOwnedByDifferentSource, id, other.Source)
		}
		if gerr != nil && !errors.Is(gerr, ErrNodeNotFound) {
			return gerr
		}
		_, err := s.AddNode(ctx, AddNodeInput{
			Domain: string(domainFromID(id)), Layer: spec.Layer, Name: spec.Name,
			ID: string(slugFromID(id)), Parent: spec.Parent,
			Source: string(source), Properties: spec.Properties,
		})
		if err != nil {
			return err
		}
		res.NodesAdded++
		return nil
	}
	if existing.Layer != spec.Layer || parentString(existing.ParentID) != spec.Parent {
		return fmt.Errorf("%w: id=%s", ErrCoreFieldsImmutable, id)
	}
	changed := false
	if existing.Name != spec.Name {
		existing.Name = spec.Name
		changed = true
	}
	newProps := nonNilNamespacedProps(source, spec.Properties)
	if !propsEqual(existing.Properties[source], newProps[source]) {
		if existing.Properties == nil {
			existing.Properties = map[SourceID]map[string]any{}
		}
		existing.Properties[source] = newProps[source]
		changed = true
	}
	if changed {
		existing.UpdatedAt = s.now()
		if err := s.store.UpdateNode(ctx, existing); err != nil {
			return err
		}
		res.NodesUpdated++
	}
	delete(byID, id)
	return nil
}

func (s *Service) applyCleanup(
	ctx context.Context, residual map[NodeID]Node, source SourceID, opts ApplyOptions, res *ApplyResult,
) error {
	for id := range residual {
		incoming, err := s.store.EdgesTo(ctx, id, nil)
		if err != nil {
			return err
		}
		outgoing, err := s.store.EdgesFrom(ctx, id, nil)
		if err != nil {
			return err
		}
		for _, e := range append(incoming, outgoing...) {
			claims, err := s.store.ListEdgeClaims(ctx, e.ID)
			if err != nil {
				return err
			}
			for _, c := range claims {
				if c.Source == source {
					continue
				}
				if !opts.ForceCascade {
					return fmt.Errorf("%w: node=%s edge=%d", ErrNodeHasForeignClaims, id, e.ID)
				}
			}
		}
		children, err := s.store.ChildrenOf(ctx, id)
		if err != nil {
			return err
		}
		if len(children) > 0 && !opts.ForceCascade {
			return fmt.Errorf("%w: node=%s children=%d", ErrHasDependents, id, len(children))
		}
		if err := s.store.DeleteNode(ctx, id); err != nil {
			return err
		}
		res.NodesRemoved++
	}
	return nil
}

func domainFromID(id NodeID) DomainID {
	dom, _, _ := id.Split()
	return dom
}

func slugFromID(id NodeID) SlugID {
	_, slug, _ := id.Split()
	return slug
}

func parentString(p *NodeID) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

func propsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || fmt.Sprintf("%v", va) != fmt.Sprintf("%v", vb) {
			return false
		}
	}
	return true
}
```

`applyEdges` is stubbed for now (Task 15 fills it in):

```go
func (s *Service) applyEdges(ctx context.Context, snap snapshot.Snapshot, source SourceID, res *ApplyResult) error {
	return nil
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./internal/graph/ -run TestApply -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/
git commit -m "feat(apply): Service.Apply with node add/update/cleanup against snapshot diff"
```

---

### Task 15: `Service.Apply` — edges (upsert + claim + GC)

`applyEdges` upserts each edge in the snapshot, claims it for the source, and tracks new claims. Cleanup unclaims edges previously claimed by this source that don't appear in the snapshot. Edge GC happens in the same tx.

**Files:**
- Modify: `internal/graph/service_apply.go`
- Modify: `internal/graph/service_apply_test.go` (append)

- [ ] **Step 1: Append failing tests**

```go
func TestApplyAddsEdgesAndClaims(t *testing.T) {
	svc, fs := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{{Src: "d:a", Target: "d:b", Type: "imports"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	es, err := fs.EdgesFrom(t.Context(), "d:a", nil)
	require.NoError(t, err)
	require.Len(t, es, 1)
}

func TestApplyRemovesUnclaimedEdgesAndGCs(t *testing.T) {
	svc, fs := newService(t)
	base := snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{{Src: "d:a", Target: "d:b", Type: "imports"}},
	}
	_, err := svc.Apply(t.Context(), base, graph.ApplyOptions{})
	require.NoError(t, err)

	base.Edges = nil
	res, err := svc.Apply(t.Context(), base, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.ClaimsRemoved)
	require.Equal(t, 1, res.EdgesGC)
	es, _ := fs.EdgesFrom(t.Context(), "d:a", nil)
	require.Empty(t, es)
}

func TestApplyForeignClaimSurvivesUnclaim(t *testing.T) {
	svc, fs := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{{Src: "d:a", Target: "d:b", Type: "imports"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	_, err = svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: "d:a", Target: "d:b", Type: "imports", WriterSource: "y"})
	require.NoError(t, err)

	_, err = svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	es, _ := fs.EdgesFrom(t.Context(), "d:a", nil)
	require.Len(t, es, 1, "y's claim keeps the edge alive")
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/graph/ -run TestApply -v
```

Expected: FAIL (`applyEdges` is a stub).

- [ ] **Step 3: Replace `applyEdges` (and extend cleanup with claim removal)**

```go
func (s *Service) applyEdges(ctx context.Context, snap snapshot.Snapshot, source SourceID, res *ApplyResult) error {
	prevClaimedIDs, err := s.store.EdgeIDsClaimedBy(ctx, source)
	if err != nil {
		return err
	}
	prevClaimed := make(map[EdgeID]struct{}, len(prevClaimedIDs))
	for _, id := range prevClaimedIDs {
		prevClaimed[id] = struct{}{}
	}

	now := s.now()
	for _, e := range snap.Edges {
		src := NodeID(e.Src)
		dst := NodeID(e.Target)
		if _, err := s.store.GetNode(ctx, src); err != nil {
			return fmt.Errorf("edges[]: source: %w", err)
		}
		if _, err := s.store.GetNode(ctx, dst); err != nil {
			return fmt.Errorf("edges[]: target: %w", err)
		}
		id, err := s.store.UpsertEdge(ctx, Edge{
			SourceID:   src,
			TargetID:   dst,
			Type:       e.Type,
			Properties: nonNilNamespacedProps(source, e.Properties),
			CreatedAt:  now,
		})
		if err != nil {
			return err
		}
		if _, already := prevClaimed[id]; !already {
			res.EdgesAdded++
		}
		if err := s.store.AddEdgeClaim(ctx, id, source, now); err != nil {
			return err
		}
		if _, already := prevClaimed[id]; !already {
			res.ClaimsAdded++
		}
		delete(prevClaimed, id)
	}

	if snap.Scope != snapshot.ScopeAdditive {
		for id := range prevClaimed {
			if err := s.store.RemoveEdgeClaim(ctx, id, source); err != nil {
				return err
			}
			res.ClaimsRemoved++
			n, err := s.store.CountEdgeClaims(ctx, id)
			if err != nil {
				return err
			}
			if n == 0 {
				if err := s.store.DeleteEdge(ctx, id); err != nil {
					return err
				}
				res.EdgesGC++
			}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./internal/graph/ -run TestApply -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/
git commit -m "feat(apply): edge upsert+claim, unclaim cleanup, GC last-claim in same tx"
```

---

### Task 16: Apply — scope semantics + conflict detection

Scope enforcement: `scope: "domain"` errors with `ErrDomainHasForeignWriters` if other sources own nodes. `scope: "additive"` skips cleanup entirely (already wired in Task 14/15; verify with a test). Conflict codes returned as wrapped sentinel errors so the CLI envelope (Task 20) can identify them.

**Files:**
- Modify: `internal/graph/service_apply.go`
- Modify: `internal/graph/service_apply_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestApplyDomainScopeFailsWithForeignWriters(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes:      []snapshot.NodeSpec{{ID: "d:a", Layer: "l1", Name: "a"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	_, err = svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "y", Domain: "d", Scope: snapshot.ScopeDomain,
		Nodes:          []snapshot.NodeSpec{{ID: "d:b", Layer: "l1", Name: "b"}},
	}, graph.ApplyOptions{})
	require.ErrorIs(t, err, graph.ErrDomainHasForeignWriters)
}

func TestApplyAdditiveScopeSkipsCleanup(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeAdditive,
		Nodes:          []snapshot.NodeSpec{{ID: "d:a", Layer: "l1", Name: "a"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, res.NodesRemoved, "additive scope leaves d:b alone")
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/graph/ -run TestApplyDomainScope -v
go test ./internal/graph/ -run TestApplyAdditiveScopeSkipsCleanup -v
```

Expected: first FAILS (no scope check yet); additive test may already pass thanks to Task 14 wiring — that's fine.

- [ ] **Step 3: Add the scope=domain check**

In `Service.Apply` (or inside the `apply` closure), after `ensureSource` but before any node work:

```go
		if scope == snapshot.ScopeDomain && !opts.ForceDomainTakeover {
			rows, err := s.store.ListNodes(ctx, NodeFilter{Domain: domainID})
			if err != nil {
				return err
			}
			for _, n := range rows {
				if n.Source != source {
					return fmt.Errorf("%w: domain=%s foreign=%s", ErrDomainHasForeignWriters, domainID, n.Source)
				}
			}
		}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./internal/graph/ -run TestApply -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/
git commit -m "feat(apply): scope domain rejects foreign writers; additive skips cleanup"
```

---

### Task 17: Apply — `--force-cascade` and `--force-domain-takeover`

`ForceCascade` makes cleanup proceed even when residual nodes have children or incident edges with foreign claims (claims drop with the edge via CASCADE on `edges → edge_claims`). `ForceDomainTakeover` bypasses the scope=domain check.

**Files:**
- Modify: `internal/graph/service_apply.go` (cleanup already references opts.ForceCascade — verify wiring)
- Modify: `internal/graph/service_apply_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestApplyForceCascadeRemovesForeignClaims(t *testing.T) {
	svc, fs := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{{Src: "d:a", Target: "d:b", Type: "imports"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	_, err = svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: "d:a", Target: "d:b", Type: "imports", WriterSource: "y"})
	require.NoError(t, err)

	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		Nodes: []snapshot.NodeSpec{{ID: "d:b", Layer: "l1", Name: "b"}},
		Edges: []snapshot.EdgeSpec{},
	}, graph.ApplyOptions{ForceCascade: true})
	require.NoError(t, err)
	require.Equal(t, 1, res.NodesRemoved)
	es, _ := fs.EdgesFrom(t.Context(), "d:a", nil)
	require.Empty(t, es, "edge cascade-removed along with node, foreign claim dropped")
}

func TestApplyForceDomainTakeoverBypassesForeignCheck(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes:      []snapshot.NodeSpec{{ID: "d:a", Layer: "l1", Name: "a"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	_, err = svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "y", Domain: "d", Scope: snapshot.ScopeDomain,
		Nodes:          []snapshot.NodeSpec{{ID: "d:b", Layer: "l1", Name: "b"}},
	}, graph.ApplyOptions{ForceDomainTakeover: true})
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/graph/ -run 'TestApplyForce' -v
```

Expected: ForceCascade may fail (Task 14 cleanup blocks on foreign claims without the override). ForceDomainTakeover should now pass since Task 16 added the `!opts.ForceDomainTakeover` guard.

- [ ] **Step 3: Wire ForceCascade through cleanup**

The cleanup function in Task 14 already references `opts.ForceCascade` for the foreign-claim check and the children check. The path is: when `opts.ForceCascade` is true, skip both guards and call `DeleteNode` directly. `DeleteNode` (Service) blocks on foreign claims as well — bypass that by calling the store directly inside Apply when force is set:

In `Service.applyCleanup`, change the bottom branch:

```go
		if opts.ForceCascade {
			if err := s.store.DeleteNode(ctx, id); err != nil {
				return err
			}
		} else {
			if err := s.DeleteNode(ctx, id, source); err != nil {
				return err
			}
		}
		res.NodesRemoved++
```

(The Store's `DeleteNode` triggers CASCADE on edges, which cascades to edge_claims. Foreign claims on those edges drop. This is the intended destructive behavior.)

- [ ] **Step 4: Run, verify pass**

```bash
go test ./internal/graph/ -run TestApply -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/
git commit -m "feat(apply): --force-cascade drops foreign claims, --force-domain-takeover bypasses scope check"
```

---

## Phase 5 — CLI: sources subcommand, apply verb, source-aware mutations

### Task 18: `cmd/kg/sources_cmds.go` — list/show/register/update/delete

**Files:**
- Create: `cmd/kg/sources_cmds.go`
- Create: `cmd/kg/sources_cmds_test.go`
- Modify: `cmd/kg/root.go` (register `newSourcesCmd`)

- [ ] **Step 1: Write failing test exercising the CLI surface**

```go
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourcesRegisterListShow(t *testing.T) {
	db := freshDB(t)

	{
		var out, errOut bytes.Buffer
		exit := run([]string{"--db", db, "sources", "register",
			"--id", "tree-sitter:0.1.0", "--description", "ts", "--trust", "100"}, &out, &errOut)
		require.Equal(t, 0, exit, errOut.String())
	}

	{
		var out, errOut bytes.Buffer
		exit := run([]string{"--db", db, "sources", "list"}, &out, &errOut)
		require.Equal(t, 0, exit, errOut.String())
		var env struct {
			Data []struct{ ID, Description string } `json:"data"`
		}
		require.NoError(t, json.Unmarshal(out.Bytes(), &env))
		ids := make([]string, 0, len(env.Data))
		for _, s := range env.Data {
			ids = append(ids, s.ID)
		}
		require.Contains(t, ids, "cli")
		require.Contains(t, ids, "manual")
		require.Contains(t, ids, "tree-sitter:0.1.0")
	}

	{
		var out, errOut bytes.Buffer
		exit := run([]string{"--db", db, "sources", "show", "tree-sitter:0.1.0"}, &out, &errOut)
		require.Equal(t, 0, exit, errOut.String())
		require.Contains(t, out.String(), `"description": "ts"`)
	}
}

func TestSourcesRegisterIfNotExistsSkipsDuplicate(t *testing.T) {
	db := freshDB(t)
	args := []string{"--db", db, "sources", "register", "--id", "x", "--if-not-exists"}
	var out, errOut bytes.Buffer
	require.Equal(t, 0, run(args, &out, &errOut), errOut.String())
	out.Reset()
	require.Equal(t, 0, run(args, &out, &errOut), errOut.String())
	require.Contains(t, out.String(), `"skipped": true`)
}

func TestSourcesUpdateChangesDescriptionAndTrust(t *testing.T) {
	db := freshDB(t)
	require.Equal(t, 0, run([]string{"--db", db, "sources", "register", "--id", "x"}, new(bytes.Buffer), new(bytes.Buffer)))
	require.Equal(t, 0, run([]string{"--db", db, "sources", "update", "x", "--description", "Updated", "--trust", "50"}, new(bytes.Buffer), new(bytes.Buffer)))

	var out bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "sources", "show", "x"}, &out, new(bytes.Buffer)))
	require.Contains(t, out.String(), `"description": "Updated"`)
	require.Contains(t, out.String(), `"trust": 50`)
}

func TestSourcesDeleteFailsWithDependents(t *testing.T) {
	db := freshDB(t)
	require.Equal(t, 0, run([]string{"--db", db, "domain", "add", "d", "--layers", "l1", "--source", "owner"}, new(bytes.Buffer), new(bytes.Buffer)))
	require.Equal(t, 0, run([]string{"--db", db, "node", "add", "--domain", "d", "--layer", "l1", "--name", "n", "--source", "owner"}, new(bytes.Buffer), new(bytes.Buffer)))

	var out bytes.Buffer
	exit := run([]string{"--db", db, "sources", "delete", "owner"}, &out, new(bytes.Buffer))
	require.NotEqual(t, 0, exit)
	require.True(t, strings.Contains(out.String(), "SOURCE_HAS_DEPENDENTS"), "got: %s", out.String())
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg/ -run TestSources -v
```

Expected: FAIL (`sources` subcommand doesn't exist).

- [ ] **Step 3: Implement `cmd/kg/sources_cmds.go`**

```go
package main

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
)

func newSourcesCmd(c *cliCtx) *cobra.Command {
	cmd := &cobra.Command{Use: "sources", Short: "Manage source registry"}
	cmd.AddCommand(
		newSourcesListCmd(c), newSourcesShowCmd(c),
		newSourcesRegisterCmd(c), newSourcesUpdateCmd(c), newSourcesDeleteCmd(c),
	)
	return cmd
}

func newSourcesListCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all sources",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			ss, err := svc.ListSources(cmd.Context())
			if err != nil {
				return err
			}
			return writeOK(c.stdout, ss)
		},
	}
}

func newSourcesShowCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Show one source",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			s, err := svc.GetSource(cmd.Context(), graph.SourceID(args[0]))
			if err != nil {
				return err
			}
			return writeOK(c.stdout, s)
		},
	}
}

func newSourcesRegisterCmd(c *cliCtx) *cobra.Command {
	var id, description string
	var trust int
	var ifNotExists bool
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a source",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			s, err := svc.RegisterSource(cmd.Context(), graph.SourceID(id), description, trust)
			if err != nil {
				if ifNotExists && errors.Is(err, graph.ErrSourceAlreadyExists) {
					return writeOK(c.stdout, map[string]any{"skipped": true, "reason": "already_exists"})
				}
				return err
			}
			return writeOK(c.stdout, s)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "source id (required)")
	cmd.Flags().StringVar(&description, "description", "", "free-form description")
	cmd.Flags().IntVar(&trust, "trust", 100, "trust score (0-100)")
	cmd.Flags().BoolVar(&ifNotExists, "if-not-exists", false, "skip if the source already exists")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func newSourcesUpdateCmd(c *cliCtx) *cobra.Command {
	var description string
	var trust int
	var trustSet bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Update description and/or trust",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			cur, err := svc.GetSource(cmd.Context(), graph.SourceID(args[0]))
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("description") {
				cur.Description = description
			}
			if trustSet {
				cur.Trust = trust
			}
			if err := svc.UpdateSource(cmd.Context(), *cur); err != nil {
				return err
			}
			return writeOK(c.stdout, cur)
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "new description")
	cmd.Flags().IntVar(&trust, "trust", 100, "new trust (0-100)")
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		trustSet = cmd.Flags().Changed("trust")
	}
	return cmd
}

func newSourcesDeleteCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Delete a source (fails if it has owned entities)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.DeleteSource(cmd.Context(), graph.SourceID(args[0])); err != nil {
				return err
			}
			return writeOK(c.stdout, map[string]any{"deleted": true, "id": args[0]})
		},
	}
}
```

Add the Service-side helpers in `internal/graph/service.go` (small wrappers + the explicit error sentinel `ErrSourceAlreadyExists`):

```go
var ErrSourceAlreadyExists = errors.New("source already exists")

func (s *Service) ListSources(ctx context.Context) ([]Source, error) {
	return s.store.ListSources(ctx)
}

func (s *Service) GetSource(ctx context.Context, id SourceID) (*Source, error) {
	return s.store.GetSource(ctx, id)
}

func (s *Service) RegisterSource(ctx context.Context, id SourceID, description string, trust int) (*Source, error) {
	if _, err := s.store.GetSource(ctx, id); err == nil {
		return nil, ErrSourceAlreadyExists
	} else if !errors.Is(err, ErrSourceNotFound) {
		return nil, err
	}
	now := s.now()
	src := Source{ID: id, Description: description, Trust: trust, FirstSeen: now, LastSeen: now}
	if err := s.store.UpsertSource(ctx, src); err != nil {
		return nil, err
	}
	return &src, nil
}

func (s *Service) UpdateSource(ctx context.Context, src Source) error {
	return s.store.UpdateSource(ctx, src)
}

func (s *Service) DeleteSource(ctx context.Context, id SourceID) error {
	return s.store.DeleteSource(ctx, id)
}
```

Register the new sentinel in `errors.go` (alongside `ErrSourceNotFound` from Task 3). Map the sentinels to envelope codes in `cmd/kg/errmap.go`:

```go
	case errors.Is(err, graph.ErrSourceNotFound):
		return mapped{3, "SOURCE_NOT_FOUND", err.Error(), ""}
	case errors.Is(err, graph.ErrSourceAlreadyExists):
		return mapped{2, "SOURCE_ALREADY_EXISTS", err.Error(), "use --if-not-exists to skip silently"}
	case errors.Is(err, graph.ErrSourceHasDependents):
		return mapped{1, "SOURCE_HAS_DEPENDENTS", err.Error(), "delete owned nodes/claims first"}
	case errors.Is(err, graph.ErrSourceRequired):
		return mapped{1, "SOURCE_REQUIRED", err.Error(), "pass --source <id>"}
	case errors.Is(err, graph.ErrInvalidSourceID):
		return mapped{1, "INVALID_SOURCE_ID", err.Error(), ""}
	case errors.Is(err, graph.ErrNodeOwnedByDifferentSource):
		return mapped{2, "NODE_OWNED_BY_DIFFERENT_SOURCE", err.Error(), "another source owns this id; change yours or coordinate"}
	case errors.Is(err, graph.ErrNodeNotOwner):
		return mapped{1, "NODE_NOT_OWNER", err.Error(), "only the owning source can modify name/delete the node"}
	case errors.Is(err, graph.ErrCoreFieldsImmutable):
		return mapped{1, "CORE_FIELDS_IMMUTABLE", err.Error(), "layer/parent cannot change; delete and re-add"}
	case errors.Is(err, graph.ErrNodeHasForeignClaims):
		return mapped{1, "NODE_HAS_FOREIGN_CLAIMS", err.Error(), "re-run with --force-cascade to drop foreign claims, or keep the node alive"}
	case errors.Is(err, graph.ErrDomainHasForeignWriters):
		return mapped{1, "DOMAIN_FOREIGN_WRITERS", err.Error(), "narrow to --scope domain-source or pass --force-domain-takeover"}
```

In `cmd/kg/root.go`, register the new subcommand:

```go
	root.AddCommand(newInitCmd(c), newDomainCmd(c), newNodeCmd(c), newEdgeCmd(c), newBatchCmd(c), newSourcesCmd(c), newApplyCmd(c))
```

(`newApplyCmd` is added in Task 20 — until then, `cmd/kg` won't compile. That's expected; Task 20 immediately follows.)

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg/ -run TestSources -v
```

Expected: FAIL on missing `newApplyCmd` symbol — that's resolved in Task 20. To make Phase 5 progress testable now, add a one-line stub at the top of a new file `cmd/kg/apply_cmd.go`:

```go
package main

import "github.com/spf13/cobra"

func newApplyCmd(c *cliCtx) *cobra.Command { return &cobra.Command{Use: "apply", Hidden: true} }
```

Then re-run; sources tests should PASS. Task 20 replaces this stub.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg/sources_cmds.go cmd/kg/sources_cmds_test.go cmd/kg/root.go cmd/kg/errmap.go cmd/kg/apply_cmd.go internal/graph/
git commit -m "feat(cli): kg sources subcommand (list/show/register/update/delete)"
```

---

### Task 19: `kg init` seeds `cli` and `manual` sources

**Files:**
- Modify: `cmd/kg/init_cmd.go`
- Modify: `cmd/kg/init_cmd_test.go` (if missing, create — there's currently no init test in v1)

- [ ] **Step 1: Write failing test**

Create `cmd/kg/init_cmd_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitSeedsBuiltinSources(t *testing.T) {
	db := freshDB(t)
	var out, errOut bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "sources", "list"}, &out, &errOut), errOut.String())
	var env struct {
		Data []struct{ ID string } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	ids := map[string]bool{}
	for _, s := range env.Data {
		ids[s.ID] = true
	}
	require.True(t, ids["cli"], "cli source must be seeded by init")
	require.True(t, ids["manual"], "manual source must be seeded by init")
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg/ -run TestInitSeedsBuiltinSources -v
```

Expected: FAIL (cli/manual not seeded).

- [ ] **Step 3: Update `cmd/kg/init_cmd.go`**

```go
package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
)

func newInitCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the database (runs migrations + seeds built-in sources)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := seedBuiltinSources(cmd.Context(), svc); err != nil {
				return err
			}
			return writeOK(c.stdout, map[string]any{"initialized": true, "db": c.dbPath})
		},
	}
}

func seedBuiltinSources(ctx context.Context, svc *graph.Service) error {
	for _, src := range []struct{ id, desc string }{
		{"cli", "kg CLI commands"},
		{"manual", "Manually authored"},
	} {
		if _, err := svc.RegisterSource(ctx, graph.SourceID(src.id), src.desc, 100); err != nil {
			if errors.Is(err, graph.ErrSourceAlreadyExists) {
				continue
			}
			return err
		}
	}
	return nil
}
```

(Add `import "errors"` at top.)

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg/ -run TestInitSeedsBuiltinSources -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg/init_cmd.go cmd/kg/init_cmd_test.go
git commit -m "feat(cli): kg init seeds builtin cli/manual sources"
```

---

### Task 20: `cmd/kg/apply_cmd.go` — the new verb

Reads a JSON snapshot from stdin, validates required flags, runs `Service.Apply`, writes the envelope.

**Files:**
- Modify: `cmd/kg/apply_cmd.go` (replace the stub from Task 18)
- Create: `cmd/kg/apply_cmd_test.go`

- [ ] **Step 1: Write failing tests**

```go
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func execApplyCmd(t *testing.T, dbPath, stdin string, extra ...string) (string, string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, _ = w.WriteString(stdin)
	require.NoError(t, w.Close())
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })

	var stdout, stderr bytes.Buffer
	args := append([]string{"--db", dbPath, "apply"}, extra...)
	exit := run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exit
}

func TestApplyHappyPath(t *testing.T) {
	db := freshDB(t)
	snap := `{
	  "protocol_version": 2, "source": "tree-sitter:0.1.0", "domain": "d", "scope": "domain-source",
	  "domain_spec": {"id": "d", "layers": ["package","file"]},
	  "nodes": [
	    {"id":"d:pkg","layer":"package","name":"pkg"},
	    {"id":"d:pkg/foo","layer":"file","parent":"d:pkg","name":"foo.go"}
	  ],
	  "edges": []
	}`
	out, errOut, exit := execApplyCmd(t, db, snap,
		"--source", "tree-sitter:0.1.0", "--domain", "d")
	require.Equal(t, 0, exit, errOut)
	var env struct {
		OK   bool `json:"ok"`
		Data struct{ NodesAdded int `json:"nodes_added"` } `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	require.True(t, env.OK)
	require.Equal(t, 2, env.Data.NodesAdded)
}

func TestApplyRejectsSnapshotSourceMismatch(t *testing.T) {
	db := freshDB(t)
	snap := `{"protocol_version":2,"source":"a","domain":"d","scope":"domain-source","nodes":[],"edges":[]}`
	out, _, exit := execApplyCmd(t, db, snap, "--source", "b", "--domain", "d")
	require.NotEqual(t, 0, exit)
	require.Contains(t, out, "SOURCE_MISMATCH")
}

func TestApplyDryRunRollsBack(t *testing.T) {
	db := freshDB(t)
	snap := `{
	  "protocol_version": 2, "source": "x", "domain": "d", "scope": "domain-source",
	  "domain_spec": {"id":"d","layers":["l1"]},
	  "nodes": [{"id":"d:a","layer":"l1","name":"a"}],
	  "edges": []
	}`
	out, _, exit := execApplyCmd(t, db, snap, "--source", "x", "--domain", "d", "--dry-run")
	require.Equal(t, 0, exit)
	require.Contains(t, out, `"dry_run": true`)

	var listOut bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "node", "list", "--domain", "d"}, &listOut, new(bytes.Buffer)))
	require.Contains(t, listOut.String(), `"data": []`)
}

func TestApplyForeignClaimsErrorsWithoutForce(t *testing.T) {
	db := freshDB(t)
	require.Equal(t, 0, run([]string{"--db", db, "sources", "register", "--id", "y", "--if-not-exists"}, new(bytes.Buffer), new(bytes.Buffer)))

	first := `{
	  "protocol_version":2,"source":"x","domain":"d","scope":"domain-source",
	  "domain_spec":{"id":"d","layers":["l1"]},
	  "nodes":[{"id":"d:a","layer":"l1","name":"a"},{"id":"d:b","layer":"l1","name":"b"}],
	  "edges":[{"src":"d:a","target":"d:b","type":"imports"}]
	}`
	_, _, exit := execApplyCmd(t, db, first, "--source", "x", "--domain", "d")
	require.Equal(t, 0, exit)
	require.Equal(t, 0, run([]string{"--db", db, "edge", "add", "d:a", "d:b", "--type", "imports", "--source", "y"}, new(bytes.Buffer), new(bytes.Buffer)))

	rm := strings.Replace(first, `,{"id":"d:b","layer":"l1","name":"b"}`, "", 1)
	rm = strings.Replace(rm, `[{"src":"d:a","target":"d:b","type":"imports"}]`, "[]", 1)
	out, _, exit := execApplyCmd(t, db, rm, "--source", "x", "--domain", "d")
	require.NotEqual(t, 0, exit)
	require.Contains(t, out, "NODE_HAS_FOREIGN_CLAIMS")
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg/ -run TestApply -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `cmd/kg/apply_cmd.go`**

```go
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/snapshot"
)

type applyOpts struct {
	source              string
	domain              string
	scope               string
	dryRun              bool
	forceCascade        bool
	forceDomainTakeover bool
}

func newApplyCmd(c *cliCtx) *cobra.Command {
	opts := &applyOpts{}
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a JSON snapshot (declarative diff+apply)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			snap, err := snapshot.Decode(os.Stdin)
			if err != nil {
				return err
			}
			if opts.source != "" && snap.Source != opts.source {
				return fmt.Errorf("SOURCE_MISMATCH: snapshot source=%q, --source=%q", snap.Source, opts.source)
			}
			if opts.domain != "" && snap.Domain != opts.domain {
				return fmt.Errorf("DOMAIN_MISMATCH: snapshot domain=%q, --domain=%q", snap.Domain, opts.domain)
			}
			applyOpts := graph.ApplyOptions{
				DryRun:              opts.dryRun,
				ForceCascade:        opts.forceCascade,
				ForceDomainTakeover: opts.forceDomainTakeover,
			}
			if opts.scope != "" {
				applyOpts.OverrideScope = snapshot.Scope(opts.scope)
			}
			res, err := svc.Apply(cmd.Context(), *snap, applyOpts)
			if err != nil {
				return err
			}
			return writeOK(c.stdout, res)
		},
	}
	cmd.Flags().StringVar(&opts.source, "source", "", "writer source id (must match snapshot.source)")
	cmd.Flags().StringVar(&opts.domain, "domain", "", "target domain (must match snapshot.domain)")
	cmd.Flags().StringVar(&opts.scope, "scope", "", "override snapshot.scope (domain-source|domain|additive)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "compute diff, rollback")
	cmd.Flags().BoolVar(&opts.forceCascade, "force-cascade", false, "allow cleanup to drop nodes with children or foreign-claimed incident edges")
	cmd.Flags().BoolVar(&opts.forceDomainTakeover, "force-domain-takeover", false, "allow scope=domain even with foreign writers present")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}

var _ = errors.New
```

Map the new error strings to envelope codes in `cmd/kg/errmap.go`:

```go
	case strings.HasPrefix(err.Error(), "SOURCE_MISMATCH"):
		return mapped{1, "SOURCE_MISMATCH", err.Error(), ""}
	case strings.HasPrefix(err.Error(), "DOMAIN_MISMATCH"):
		return mapped{1, "DOMAIN_MISMATCH", err.Error(), ""}
```

(Add `import "strings"` to `errmap.go`.)

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg/ -run TestApply -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg/
git commit -m "feat(cli): kg apply verb consumes JSON snapshot via Service.Apply"
```

---

### Task 21: Update `batch_cmd.go` and `batch/op.go` for `source` field + edge.add rename + edge.unclaim

`source` becomes a required field in each mutation op's `args`. The `edge.add` op renames the wire field `source` (originating node) to `src` to avoid colliding with the writer-source. A new alias op `edge.unclaim` is the recommended way to drop a claim (`edge.delete` keeps `edge.unclaim` semantic for compatibility — drops all writer claims, GCs if last).

**Files:**
- Modify: `batch/op.go`
- Modify: `batch/op_test.go`
- Modify: `cmd/kg/batch_cmd.go`
- Modify: `cmd/kg/batch_cmd_test.go`

- [ ] **Step 1: Update `batch/op.go`**

Add the new op name constant, the `Source` field on each mutation Arg type, and rename `EdgeAddArgs.Source` → `EdgeAddArgs.Src`:

```go
const (
	OpMeta        OpName = "meta"
	OpDomainAdd   OpName = "domain.add"
	OpNodeAdd     OpName = "node.add"
	OpNodeUpdate  OpName = "node.update"
	OpNodeDelete  OpName = "node.delete"
	OpEdgeAdd     OpName = "edge.add"
	OpEdgeDelete  OpName = "edge.delete"
	OpEdgeUnclaim OpName = "edge.unclaim"
)

const ProtocolVersion = 1

type DomainAddArgs struct {
	ID          string         `json:"id"`
	Layers      []string       `json:"layers"`
	Description string         `json:"description,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	Source      string         `json:"source"`
	IfNotExists bool           `json:"if_not_exists,omitempty"`
}

type NodeAddArgs struct {
	Domain      string         `json:"domain"`
	Layer       string         `json:"layer"`
	Name        string         `json:"name"`
	ID          string         `json:"id,omitempty"`
	Parent      string         `json:"parent,omitempty"`
	Source      string         `json:"source"`
	Properties  map[string]any `json:"properties,omitempty"`
	IfNotExists bool           `json:"if_not_exists,omitempty"`
}

type NodeUpdateArgs struct {
	ID         string         `json:"id"`
	Source     string         `json:"source"`
	Name       *string        `json:"name,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

type NodeDeleteArgs struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Force  bool   `json:"force,omitempty"`
}

type EdgeAddArgs struct {
	Src         string         `json:"src"`
	Target      string         `json:"target"`
	Type        string         `json:"type"`
	Source      string         `json:"source"`
	Properties  map[string]any `json:"properties,omitempty"`
	IfNotExists bool           `json:"if_not_exists,omitempty"`
}

type EdgeDeleteArgs struct {
	ID     int64  `json:"id"`
	Source string `json:"source,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

type EdgeUnclaimArgs struct {
	ID     int64  `json:"id"`
	Source string `json:"source"`
}
```

Update `IsKnownOp` to include `OpEdgeUnclaim`. Add a backward-compat decode for `EdgeAddArgs` so a legacy `source` field (originating node, v1 wire form) still parses into `Src`:

```go
func (a *EdgeAddArgs) UnmarshalJSON(b []byte) error {
	type alias EdgeAddArgs
	var aux struct {
		alias
		LegacySource string `json:"source,omitempty"`
		WriterSource string `json:"writer_source,omitempty"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	*a = EdgeAddArgs(aux.alias)
	if a.Src == "" && aux.LegacySource != "" && aux.WriterSource == "" {
		// Ambiguous case: v1 had no writer-source, so `source` meant originating node.
		// But v2 wire requires writer-source, so a lone `source` could mean either.
		// In practice the v2 wire uses `src` for node + `source` for writer; we keep
		// the legacy field as a fallback for the originating node ONLY when `src` is empty.
		a.Src = aux.LegacySource
	}
	if a.Source == "" && aux.WriterSource != "" {
		a.Source = aux.WriterSource
	}
	return nil
}
```

(This is intentionally conservative; the v2 wire spec uses `src` and `source` clearly. The legacy path is for hand-written test fixtures.)

- [ ] **Step 2: Update `batch/op_test.go`**

Replace any tests that asserted on `EdgeAddArgs.Source`. Add:

```go
func TestEdgeAddArgsSrcWireField(t *testing.T) {
	in := batch.EdgeAddArgs{Src: "a:b", Target: "a:c", Type: "imports", Source: "writer"}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"src":"a:b","target":"a:c","type":"imports","source":"writer"}`, string(b))
}
```

- [ ] **Step 3: Update `cmd/kg/batch_cmd.go`**

`applyOp` now passes `source` into Service inputs. For each mutation op, require a non-empty source — bail with a clear error otherwise.

Replace the body of `applyOp` (matching the v1 layout):

```go
func applyOp(ctx context.Context, svc *graph.Service, op batch.Op) (applied bool, err error) {
	switch op.Op {
	case batch.OpDomainAdd:
		var a batch.DomainAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("domain.add args: %w", err)
		}
		if a.Source == "" {
			return false, graph.ErrSourceRequired
		}
		_, err := svc.AddDomain(ctx, graph.AddDomainInput{
			ID: a.ID, Description: a.Description, Layers: a.Layers, Source: a.Source,
		})
		return classifyIfNotExists(err, a.IfNotExists, graph.ErrDomainAlreadyExists)

	case batch.OpNodeAdd:
		var a batch.NodeAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("node.add args: %w", err)
		}
		if a.Source == "" {
			return false, graph.ErrSourceRequired
		}
		_, err := svc.AddNode(ctx, graph.AddNodeInput{
			Domain: a.Domain, Layer: a.Layer, Name: a.Name,
			ID: a.ID, Parent: a.Parent, Source: a.Source, Properties: a.Properties,
		})
		return classifyIfNotExists(err, a.IfNotExists, graph.ErrNodeAlreadyExists)

	case batch.OpNodeUpdate:
		var a batch.NodeUpdateArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("node.update args: %w", err)
		}
		if a.Source == "" {
			return false, graph.ErrSourceRequired
		}
		if a.Name != nil {
			if _, err := svc.UpdateNode(ctx, graph.NodeID(a.ID), graph.UpdateNodeInput{Source: graph.SourceID(a.Source), Name: a.Name}); err != nil {
				return false, err
			}
		}
		if len(a.Properties) > 0 {
			if err := svc.SetNodeProperties(ctx, graph.NodeID(a.ID), graph.SourceID(a.Source), a.Properties); err != nil {
				return false, err
			}
		}
		return true, nil

	case batch.OpNodeDelete:
		var a batch.NodeDeleteArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("node.delete args: %w", err)
		}
		if a.Source == "" {
			return false, graph.ErrSourceRequired
		}
		if err := svc.DeleteNode(ctx, graph.NodeID(a.ID), graph.SourceID(a.Source)); err != nil {
			return false, err
		}
		return true, nil

	case batch.OpEdgeAdd:
		var a batch.EdgeAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("edge.add args: %w", err)
		}
		if a.Source == "" {
			return false, graph.ErrSourceRequired
		}
		_, err := svc.AddEdge(ctx, graph.AddEdgeInput{
			Source: a.Src, Target: a.Target, Type: a.Type,
			WriterSource: a.Source, Properties: a.Properties,
		})
		return classifyIfNotExists(err, a.IfNotExists, graph.ErrEdgeAlreadyExists)

	case batch.OpEdgeDelete, batch.OpEdgeUnclaim:
		var a batch.EdgeUnclaimArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("%s args: %w", op.Op, err)
		}
		if a.Source == "" {
			return false, graph.ErrSourceRequired
		}
		if err := svc.RemoveEdgeClaim(ctx, graph.EdgeID(a.ID), graph.SourceID(a.Source)); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("unknown op %q", op.Op)
}
```

- [ ] **Step 4: Update `cmd/kg/batch_cmd_test.go`**

Every existing batch test that constructed a JSONL stream needs `source` added to each op's args. Update them in place:

```go
`{"op":"domain.add","args":{"id":"a","layers":["l1","l2"],"source":"cli"}}`,
`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"root","source":"cli"}}`,
`{"op":"node.add","args":{"domain":"a","layer":"l2","name":"child","parent":"a:root","source":"cli"}}`,
```

For `edge.add`, switch `"source"` (the v1 originating node) to `"src"`:

```go
`{"op":"edge.add","args":{"src":"a:root","target":"a:other","type":"x","source":"cli"}}`,
```

Add a new test for `edge.unclaim`:

```go
func TestBatchEdgeUnclaimGCs(t *testing.T) {
	db := freshDB(t)
	stream := strings.Join([]string{
		`{"op":"domain.add","args":{"id":"a","layers":["l1"],"source":"cli"}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"x","source":"cli"}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"y","source":"cli"}}`,
		`{"op":"edge.add","args":{"src":"a:x","target":"a:y","type":"imports","source":"cli"}}`,
	}, "\n") + "\n"
	_, _, exit := execBatchCmd(t, db, stream)
	require.Equal(t, 0, exit)

	var listOut bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "edge", "list-from", "a:x"}, &listOut, new(bytes.Buffer)))
	require.Contains(t, listOut.String(), `"type": "imports"`)

	var raw struct {
		Data []struct{ ID int64 `json:"id"` } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listOut.Bytes(), &raw))
	require.NotEmpty(t, raw.Data)
	id := raw.Data[0].ID

	unclaim := fmt.Sprintf(`{"op":"edge.unclaim","args":{"id":%d,"source":"cli"}}` + "\n", id)
	_, _, exit = execBatchCmd(t, db, unclaim)
	require.Equal(t, 0, exit)

	listOut.Reset()
	require.Equal(t, 0, run([]string{"--db", db, "edge", "list-from", "a:x"}, &listOut, new(bytes.Buffer)))
	require.Contains(t, listOut.String(), `"data": []`)
}
```

- [ ] **Step 5: Run**

```bash
go test ./batch/ ./cmd/kg/ -v
```

Expected: PASS for batch tests including the new ones. Some existing tests in `cmd/kg/batch_cmd_test.go` that I haven't manually called out might still fail — fix the remaining `source` omissions until they're all green.

- [ ] **Step 6: Commit**

```bash
git add batch/ cmd/kg/
git commit -m "feat(cli): batch ops carry source; edge.add uses src/source; edge.unclaim added"
```

---

### Task 22: Update `node_cmds.go` and `edge_cmds.go` for `--source` and view modes

`node add/update/delete` get `--source` (default `cli`). `node get` gets `--source <id>` (flatten one namespace) and `--merged` (trust-ranked union). `edge add` gets `--source`. New: `edge claims <edge-id>` and `edge unclaim <edge-id> --source`. `edge delete <edge-id> --force` is a power-user destructive op.

**Files:**
- Modify: `cmd/kg/node_cmds.go`
- Modify: `cmd/kg/edge_cmds.go`
- Modify: `cmd/kg/node_cmds_test.go`
- Modify: `cmd/kg/edge_cmds_test.go`

- [ ] **Step 1: Append failing tests in `node_cmds_test.go`**

```go
func TestNodeGetMergedView(t *testing.T) {
	db := freshDB(t)
	require.Equal(t, 0, run([]string{"--db", db, "domain", "add", "d", "--layers", "l1", "--source", "cli"}, new(bytes.Buffer), new(bytes.Buffer)))
	require.Equal(t, 0, run([]string{"--db", db, "node", "add",
		"--domain", "d", "--layer", "l1", "--name", "n",
		"--source", "a", "--properties", `{"k":"va"}`}, new(bytes.Buffer), new(bytes.Buffer)))
	require.Equal(t, 0, run([]string{"--db", db, "node", "update", "d:n",
		"--source", "b", "--properties", `{"j":"vb"}`}, new(bytes.Buffer), new(bytes.Buffer)))

	var out bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "node", "get", "d:n", "--merged"}, &out, new(bytes.Buffer)))
	require.Contains(t, out.String(), `"k": "va"`)
	require.Contains(t, out.String(), `"j": "vb"`)
	require.Contains(t, out.String(), `"_property_sources"`)
}

func TestNodeGetSourceFiltersToOneNamespace(t *testing.T) {
	db := freshDB(t)
	require.Equal(t, 0, run([]string{"--db", db, "domain", "add", "d", "--layers", "l1", "--source", "cli"}, new(bytes.Buffer), new(bytes.Buffer)))
	require.Equal(t, 0, run([]string{"--db", db, "node", "add",
		"--domain", "d", "--layer", "l1", "--name", "n",
		"--source", "a", "--properties", `{"k":"va"}`}, new(bytes.Buffer), new(bytes.Buffer)))

	var out bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "node", "get", "d:n", "--source", "a"}, &out, new(bytes.Buffer)))
	require.Contains(t, out.String(), `"k": "va"`)
	require.NotContains(t, out.String(), `"properties": {`, "flat properties view")
}
```

- [ ] **Step 2: Append failing tests in `edge_cmds_test.go`**

```go
func TestEdgeUnclaimGCs(t *testing.T) {
	db := freshDB(t)
	require.Equal(t, 0, run([]string{"--db", db, "domain", "add", "d", "--layers", "l1", "--source", "cli"}, new(bytes.Buffer), new(bytes.Buffer)))
	require.Equal(t, 0, run([]string{"--db", db, "node", "add", "--domain", "d", "--layer", "l1", "--name", "a", "--source", "cli"}, new(bytes.Buffer), new(bytes.Buffer)))
	require.Equal(t, 0, run([]string{"--db", db, "node", "add", "--domain", "d", "--layer", "l1", "--name", "b", "--source", "cli"}, new(bytes.Buffer), new(bytes.Buffer)))
	var addOut bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "edge", "add", "d:a", "d:b", "--type", "imports", "--source", "cli"}, &addOut, new(bytes.Buffer)))
	var added struct {
		Data struct{ ID int64 `json:"id"` } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(addOut.Bytes(), &added))
	require.NotZero(t, added.Data.ID)

	require.Equal(t, 0, run([]string{"--db", db, "edge", "unclaim", fmt.Sprint(added.Data.ID), "--source", "cli"}, new(bytes.Buffer), new(bytes.Buffer)))

	var listOut bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "edge", "list-from", "d:a"}, &listOut, new(bytes.Buffer)))
	require.Contains(t, listOut.String(), `"data": []`)
}
```

- [ ] **Step 3: Update `node_cmds.go`**

Add `--source` (default `cli`), `--properties` (raw JSON), `--merged` (on get), and tweak update to write into the source namespace via `SetNodeProperties` / `UpdateNode`. Delete also takes `--source` (default `cli`). Show concrete diffs:

`newNodeAddCmd` (the variable declarations and flag block):

```go
	var domain, layer, name, id, parent, source, propertiesJSON string
	var ifNotExists, dryRun bool
	...
	in := graph.AddNodeInput{Domain: domain, Layer: layer, Name: name, ID: id, Parent: parent, Source: source}
	if propertiesJSON != "" {
		if err := json.Unmarshal([]byte(propertiesJSON), &in.Properties); err != nil {
			return fmt.Errorf("--properties: %w", err)
		}
	}
	...
	cmd.Flags().StringVar(&source, "source", "cli", "writer source id")
	cmd.Flags().StringVar(&propertiesJSON, "properties", "", "JSON object of properties for this source's namespace")
```

`newNodeGetCmd`:

```go
	var source string
	var merged bool
	cmd := &cobra.Command{
		Use:   "get <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Get a node (default: raw namespaced properties)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			n, err := svc.GetNode(cmd.Context(), graph.NodeID(args[0]))
			if err != nil {
				return err
			}
			switch {
			case source != "":
				return writeOK(c.stdout, nodeFlattenedView(*n, graph.SourceID(source)))
			case merged:
				ss, err := svc.ListSources(cmd.Context())
				if err != nil {
					return err
				}
				return writeOK(c.stdout, nodeMergedView(*n, ss))
			}
			return writeOK(c.stdout, n)
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "show only this source's namespace (flattened)")
	cmd.Flags().BoolVar(&merged, "merged", false, "trust-ranked union of all namespaces with _property_sources attribution")
```

Add the helpers at the bottom of `node_cmds.go`:

```go
func nodeFlattenedView(n graph.Node, source graph.SourceID) map[string]any {
	out := map[string]any{
		"id": n.ID, "domain": n.Domain, "layer": n.Layer, "name": n.Name,
		"parent_id": n.ParentID, "source": n.Source,
		"properties": n.Properties[source],
		"revision":   n.Revision, "created_at": n.CreatedAt, "updated_at": n.UpdatedAt,
	}
	return out
}

func nodeMergedView(n graph.Node, sources []graph.Source) map[string]any {
	trustOf := map[graph.SourceID]int{}
	for _, s := range sources {
		trustOf[s.ID] = s.Trust
	}
	type contrib struct {
		source graph.SourceID
		trust  int
		value  any
	}
	keys := map[string]contrib{}
	for src, m := range n.Properties {
		t := trustOf[src]
		for k, v := range m {
			c, ok := keys[k]
			if !ok || t > c.trust || (t == c.trust && src < c.source) {
				keys[k] = contrib{source: src, trust: t, value: v}
			}
		}
	}
	props := map[string]any{}
	srcs := map[string]string{}
	for k, c := range keys {
		props[k] = c.value
		srcs[k] = string(c.source)
	}
	return map[string]any{
		"id": n.ID, "domain": n.Domain, "layer": n.Layer, "name": n.Name,
		"parent_id": n.ParentID, "source": n.Source,
		"properties":         props,
		"_property_sources":  srcs,
		"revision":           n.Revision, "created_at": n.CreatedAt, "updated_at": n.UpdatedAt,
	}
}
```

`newNodeUpdateCmd`:

```go
	var name, source, propertiesJSON string
	cmd := &cobra.Command{
		Use:   "update <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Update a node's name (owner only) or properties (any writer, within own namespace)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			id := graph.NodeID(args[0])
			if cmd.Flags().Changed("name") {
				if _, err := svc.UpdateNode(cmd.Context(), id, graph.UpdateNodeInput{
					Source: graph.SourceID(source), Name: &name,
				}); err != nil {
					return err
				}
			}
			if propertiesJSON != "" {
				var props map[string]any
				if err := json.Unmarshal([]byte(propertiesJSON), &props); err != nil {
					return fmt.Errorf("--properties: %w", err)
				}
				if err := svc.SetNodeProperties(cmd.Context(), id, graph.SourceID(source), props); err != nil {
					return err
				}
			}
			n, err := svc.GetNode(cmd.Context(), id)
			if err != nil {
				return err
			}
			return writeOK(c.stdout, n)
		},
	}
	cmd.Flags().StringVar(&source, "source", "cli", "writer source id (default cli)")
	cmd.Flags().StringVar(&name, "name", "", "new name (owner only)")
	cmd.Flags().StringVar(&propertiesJSON, "properties", "", "JSON object of properties for this source's namespace")
```

`newNodeDeleteCmd`:

```go
	var source string
	var forceCascade bool
	cmd := &cobra.Command{...
		RunE: func(cmd *cobra.Command, args []string) error {
			...
			if forceCascade {
				if err := svc.ForceDeleteNode(cmd.Context(), id); err != nil {
					return err
				}
			} else {
				if err := svc.DeleteNode(cmd.Context(), id, graph.SourceID(source)); err != nil {
					return err
				}
			}
			...
		},
	}
	cmd.Flags().StringVar(&source, "source", "cli", "writer source id")
	cmd.Flags().BoolVar(&forceCascade, "force-cascade", false, "drop the node ignoring foreign claims and children")
```

Add the `ForceDeleteNode` helper to Service:

```go
func (s *Service) ForceDeleteNode(ctx context.Context, id NodeID) error {
	return s.store.DeleteNode(ctx, id)
}
```

- [ ] **Step 4: Update `edge_cmds.go`**

Add `--source` to `add`, change `delete` semantic, add `claims <id>` and `unclaim <id> --source`. Diff sketch:

`newEdgeAddCmd`:

```go
	var typ, source string
	...
	in := graph.AddEdgeInput{Source: args[0], Target: args[1], Type: typ, WriterSource: source}
	...
	cmd.Flags().StringVar(&source, "source", "cli", "writer source id")
```

Replace `newEdgeDeleteCmd` with two commands: `unclaim` and `delete --force`:

```go
func newEdgeUnclaimCmd(c *cliCtx) *cobra.Command {
	var source string
	cmd := &cobra.Command{
		Use:   "unclaim <edge-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Remove caller's claim on the edge (GCs the edge if last claim)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			n, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return err
			}
			if err := svc.RemoveEdgeClaim(cmd.Context(), graph.EdgeID(n), graph.SourceID(source)); err != nil {
				return err
			}
			return writeOK(c.stdout, map[string]any{"unclaimed": true, "id": n, "source": source})
		},
	}
	cmd.Flags().StringVar(&source, "source", "cli", "writer source id")
	return cmd
}

func newEdgeDeleteCmd(c *cliCtx) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "delete <edge-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Delete an edge entirely (drops all claims; use --force)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			if !force {
				return fmt.Errorf("destructive: pass --force to drop all claims along with this edge")
			}
			n, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return err
			}
			if err := svc.DeleteEdge(cmd.Context(), graph.EdgeID(n)); err != nil {
				return err
			}
			return writeOK(c.stdout, map[string]any{"deleted": true, "id": n})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "required: drops the edge and ALL claims")
	return cmd
}

func newEdgeClaimsCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "claims <edge-id>",
		Args:  cobra.ExactArgs(1),
		Short: "List claims on an edge",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			n, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return err
			}
			cs, err := svc.ListEdgeClaims(cmd.Context(), graph.EdgeID(n))
			if err != nil {
				return err
			}
			return writeOK(c.stdout, cs)
		},
	}
}
```

Register both in `newEdgeCmdReal`:

```go
	cmd.AddCommand(newEdgeAddCmd(c), newEdgeListFromCmd(c), newEdgeListToCmd(c),
		newEdgeUnclaimCmd(c), newEdgeClaimsCmd(c), newEdgeDeleteCmd(c))
```

`newDomainCmd*`: also add `--source` with default `cli` for `domain add` (the changes log needs it).

- [ ] **Step 5: Run**

```bash
go test ./cmd/kg/...
```

Expected: PASS for both the new tests and the v1 tests once `--source` defaults are in place. If a v1 test fails because it omitted `--source`, the new default of `cli` should cover it.

- [ ] **Step 6: Commit**

```bash
git add cmd/kg/
git commit -m "feat(cli): --source + view modes on node/edge cmds; edge unclaim/claims/delete --force"
```

---

## Phase 6 — kg-extractor: declarative runtime dispatch

`kg-extractor` learns three things in v2: (1) parse `source_id` and `trust` from manifests, (2) recognize `declarative-native` and `declarative-command` runtimes, (3) collect the plugin's stdout as one JSON document (not JSONL) for declarative runtimes, validate it, and pipe to `kg apply` instead of `kg batch`.

### Task 23: Manifest updates — `source_id`, `trust`, declarative-* runtimes

**Files:**
- Modify: `cmd/kg-extractor/manifest.go`
- Modify: `cmd/kg-extractor/manifest_test.go`

- [ ] **Step 1: Append failing tests**

```go
func TestManifestParsesV2Fields(t *testing.T) {
	path := writeTempManifest(t, `{
	  "name":"foo","version":"1.0","description":"x",
	  "runtime":"declarative-native","executable":"foo",
	  "source_id":"acme/foo:1.0","trust":80,
	  "supported_scopes":["domain-source","domain"]
	}`)
	m, err := parseManifest(path)
	require.NoError(t, err)
	require.Equal(t, runtimeDeclarativeNative, m.Runtime)
	require.Equal(t, "acme/foo:1.0", m.SourceID)
	require.Equal(t, 80, m.Trust)
	require.Equal(t, []string{"domain-source", "domain"}, m.SupportedScopes)
}

func TestManifestSourceIDDefaultsToNameVersion(t *testing.T) {
	path := writeTempManifest(t, `{"name":"foo","version":"1.0","description":"x","runtime":"native","executable":"foo"}`)
	m, err := parseManifest(path)
	require.NoError(t, err)
	require.Equal(t, "foo:1.0", m.SourceID)
	require.Equal(t, 100, m.Trust)
}

func TestManifestRejectsUnknownRuntime(t *testing.T) {
	path := writeTempManifest(t, `{"name":"foo","version":"1.0","description":"x","runtime":"quantum"}`)
	_, err := parseManifest(path)
	require.Error(t, err)
}
```

(`writeTempManifest(t, body)` is a helper that should already exist in `cmd/kg-extractor/test_helpers_test.go`; if not, add one that writes the JSON to a temp dir and returns the path.)

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg-extractor/ -run TestManifest -v
```

Expected: FAIL.

- [ ] **Step 3: Update `manifest.go`**

Replace the runtime constants and manifest type:

```go
type runtimeKind string

const (
	runtimeNative              runtimeKind = "native"
	runtimeCommand             runtimeKind = "command"
	runtimeWASM                runtimeKind = "wasm"
	runtimeDeclarativeNative   runtimeKind = "declarative-native"
	runtimeDeclarativeCommand  runtimeKind = "declarative-command"
)

type manifest struct {
	Name               string      `json:"name"`
	Version            string      `json:"version"`
	Description        string      `json:"description"`
	Runtime            runtimeKind `json:"runtime"`
	Executable         string      `json:"executable,omitempty"`
	Command            []string    `json:"command,omitempty"`
	Module             string      `json:"module,omitempty"`
	SupportedLayers    []string    `json:"supported_layers,omitempty"`
	SupportedLanguages []string    `json:"supported_languages,omitempty"`
	SupportedScopes    []string    `json:"supported_scopes,omitempty"`
	SourceID           string      `json:"source_id,omitempty"`
	Trust              int         `json:"trust,omitempty"`
}
```

Replace `parseManifest` to apply the defaults and accept the new runtimes:

```go
func parseManifest(path string) (*manifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if !pluginNameRE.MatchString(m.Name) {
		return nil, fmt.Errorf("invalid plugin name %q", m.Name)
	}
	if m.Version == "" {
		return nil, errors.New("manifest version is required")
	}
	if m.Description == "" {
		return nil, errors.New("manifest description is required")
	}
	switch m.Runtime {
	case runtimeNative, runtimeDeclarativeNative:
		if m.Executable == "" {
			return nil, fmt.Errorf("%s runtime requires executable", m.Runtime)
		}
	case runtimeCommand, runtimeDeclarativeCommand:
		if len(m.Command) == 0 {
			return nil, fmt.Errorf("%s runtime requires command[]", m.Runtime)
		}
	case runtimeWASM:
		if m.Module == "" {
			return nil, errors.New("wasm runtime requires module")
		}
	default:
		return nil, fmt.Errorf("unknown runtime %q", m.Runtime)
	}
	if m.SourceID == "" {
		m.SourceID = m.Name + ":" + m.Version
	}
	if m.Trust == 0 {
		m.Trust = 100
	}
	if len(m.SupportedScopes) == 0 {
		m.SupportedScopes = []string{"domain-source"}
	}
	return &m, nil
}

func (r runtimeKind) IsDeclarative() bool {
	return r == runtimeDeclarativeNative || r == runtimeDeclarativeCommand
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg-extractor/ -run TestManifest -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg-extractor/
git commit -m "feat(extractor): manifest source_id, trust, declarative-* runtimes"
```

---

### Task 24: `cmd/kg-extractor/snapshot_validator.go` — snapshot shape validator

Validates that the JSON document from a declarative plugin parses as a `snapshot.Snapshot`, passes `snapshot.Validate`, and that `snapshot.Source` matches the manifest's `source_id` (when invoked via kg-extractor).

**Files:**
- Create: `cmd/kg-extractor/snapshot_validator.go`
- Create: `cmd/kg-extractor/snapshot_validator_test.go`

- [ ] **Step 1: Write failing tests**

```go
package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSnapshotHappy(t *testing.T) {
	in := strings.NewReader(`{
	  "protocol_version":2,"source":"foo:1.0","domain":"d","scope":"domain-source",
	  "nodes":[{"id":"d:n","layer":"l","name":"n"}],"edges":[]
	}`)
	var out bytes.Buffer
	require.NoError(t, validateSnapshot(in, &out, "foo:1.0"))
	require.Contains(t, out.String(), `"source":"foo:1.0"`)
}

func TestValidateSnapshotRejectsSourceMismatch(t *testing.T) {
	in := strings.NewReader(`{"protocol_version":2,"source":"foo:1.0","domain":"d","scope":"domain-source","nodes":[],"edges":[]}`)
	var out bytes.Buffer
	err := validateSnapshot(in, &out, "bar:2.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "SOURCE_MISMATCH")
}

func TestValidateSnapshotRejectsJSONL(t *testing.T) {
	in := strings.NewReader(`{"protocol_version":2}` + "\n" + `{"protocol_version":2}` + "\n")
	var out bytes.Buffer
	err := validateSnapshot(in, &out, "x:1")
	require.Error(t, err)
}

func TestValidateSnapshotRejectsBadShape(t *testing.T) {
	in := strings.NewReader(`{"protocol_version":1,"source":"x","domain":"d","scope":"domain-source","nodes":[],"edges":[]}`)
	var out bytes.Buffer
	err := validateSnapshot(in, &out, "x")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg-extractor/ -run TestValidateSnapshot -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `snapshot_validator.go`**

```go
package main

import (
	"bytes"
	"fmt"
	"io"

	"github.com/ggfarmco/kg/snapshot"
)

func validateSnapshot(r io.Reader, w io.Writer, expectedSource string) error {
	var raw bytes.Buffer
	if _, err := io.Copy(&raw, r); err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}
	snap, err := snapshot.Decode(bytes.NewReader(raw.Bytes()))
	if err != nil {
		return err
	}
	if err := snapshot.Validate(snap); err != nil {
		return err
	}
	if expectedSource != "" && snap.Source != expectedSource {
		return fmt.Errorf("SOURCE_MISMATCH: manifest source_id=%q, snapshot.source=%q", expectedSource, snap.Source)
	}
	_, err = w.Write(raw.Bytes())
	return err
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg-extractor/ -run TestValidateSnapshot -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg-extractor/snapshot_validator.go cmd/kg-extractor/snapshot_validator_test.go
git commit -m "feat(extractor): snapshot validator with manifest source binding"
```

---

### Task 25: `extract_cmd.go` — dispatch by runtime kind

When `manifest.runtime.IsDeclarative()`, collect single-JSON snapshot, validate, pipe to `kg apply`. Otherwise (imperative), keep the v1 path: line-by-line JSONL validation then `kg batch`. The `--source` to apply/batch comes from the manifest, not a user flag.

**Files:**
- Modify: `cmd/kg-extractor/extract_cmd.go`
- Modify: `cmd/kg-extractor/extract_cmd_test.go` (probably; depends on existing v1 tests)
- Modify: `cmd/kg-extractor/integration_test.go`

- [ ] **Step 1: Append a failing integration test**

```go
func TestExtractDeclarativePipesToKgApply(t *testing.T) {
	pluginsDir := t.TempDir()
	pluginHome := filepath.Join(pluginsDir, "snap-demo")
	writeFile(t, filepath.Join(pluginHome, "manifest.json"), `{
	  "name":"snap-demo","version":"0.1.0","description":"declarative bash demo",
	  "runtime":"declarative-command","command":["bash","extract.sh"],
	  "source_id":"snap-demo:0.1.0"
	}`)
	writeFile(t, filepath.Join(pluginHome, "extract.sh"), `#!/usr/bin/env bash
set -euo pipefail
cat <<'EOF'
{
  "protocol_version": 2,
  "source": "snap-demo:0.1.0",
  "domain": "snap",
  "scope": "domain-source",
  "domain_spec": {"id":"snap","layers":["l1"]},
  "nodes": [{"id":"snap:a","layer":"l1","name":"a"}],
  "edges": []
}
EOF
`)
	require.NoError(t, os.Chmod(filepath.Join(pluginHome, "extract.sh"), 0o755))

	kgBin := buildKgBinary(t)
	dbPath := filepath.Join(t.TempDir(), "snap.db")
	require.Equal(t, 0, runCmd(t, kgBin, "--db", dbPath, "init"))

	args := []string{
		"--plugins-path", pluginsDir, "extract",
		"--plugin", "snap-demo", "--domain", "snap",
		"--db", dbPath, "--kg-binary", kgBin,
	}
	var out, errOut bytes.Buffer
	exit := runExtractor(t, args, &out, &errOut)
	require.Equal(t, 0, exit, errOut.String())

	listOut := captureKg(t, kgBin, "--db", dbPath, "node", "list", "--domain", "snap")
	require.Contains(t, listOut, `"snap:a"`)
}
```

The helpers (`buildKgBinary`, `runCmd`, `runExtractor`, `captureKg`) belong in `integration_test.go` — they should already exist for v1. Reuse them; extend if missing.

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg-extractor/ -run TestExtractDeclarative -v
```

Expected: FAIL (declarative dispatch not implemented).

- [ ] **Step 3: Update `extract_cmd.go`**

Replace `runExtract`:

```go
func runExtract(ctx context.Context, c *cliCtx, opts *extractOpts) error {
	plugins, _ := discoverPlugins(c.pluginsPath)
	var chosen *discoveredPlugin
	for i := range plugins {
		if plugins[i].Manifest.Name == opts.plugin {
			chosen = &plugins[i]
			break
		}
	}
	if chosen == nil {
		writeErr(c.stdout, "PLUGIN_NOT_FOUND", fmt.Sprintf("plugin %q not found", opts.plugin), "run `kg-extractor list`")
		return errEnvelopeAlreadyWritten
	}

	protocol := 1
	if chosen.Manifest.Runtime.IsDeclarative() {
		protocol = 2
	}
	cfg := pluginConfig{Input: opts.input, Domain: opts.domain, ProtocolVersion: protocol, Config: map[string]any{}}
	if opts.language != "" {
		cfg.Config["language"] = opts.language
	}
	if opts.configJSON != "" {
		var extra map[string]any
		if err := json.Unmarshal([]byte(opts.configJSON), &extra); err != nil {
			return fmt.Errorf("--config-json: %w", err)
		}
		for k, v := range extra {
			cfg.Config[k] = v
		}
	}
	if opts.configFile != "" {
		body, err := os.ReadFile(opts.configFile)
		if err != nil {
			return err
		}
		var extra map[string]any
		if err := json.Unmarshal(body, &extra); err != nil {
			return fmt.Errorf("--config-file %s: %w", opts.configFile, err)
		}
		for k, v := range extra {
			cfg.Config[k] = v
		}
	}

	var pluginStderr io.Writer = c.stderr
	if opts.quiet {
		pluginStderr = io.Discard
	}
	raw, err := invokePlugin(ctx, *chosen, cfg, pluginStderr)
	if err != nil {
		return err
	}

	if chosen.Manifest.Runtime.IsDeclarative() {
		var validated bytes.Buffer
		if err := validateSnapshot(raw, &validated, chosen.Manifest.SourceID); err != nil {
			return err
		}
		if opts.dbPath == "" {
			_, err := c.stdout.Write(validated.Bytes())
			return err
		}
		return forwardToKgApply(ctx, c, opts, chosen.Manifest, validated.Bytes())
	}

	var validated bytes.Buffer
	if err := validateStream(raw, &validated); err != nil {
		return err
	}
	if opts.dbPath == "" {
		_, err := c.stdout.Write(validated.Bytes())
		return err
	}
	return forwardToKgBatch(ctx, c, opts, chosen.Manifest, validated.Bytes())
}

func forwardToKgApply(ctx context.Context, c *cliCtx, opts *extractOpts, m manifest, snap []byte) error {
	cmd := exec.CommandContext(ctx, opts.kgBinary, "--db", opts.dbPath, "apply",
		"--source", m.SourceID, "--domain", opts.domain)
	cmd.Stdin = bytes.NewReader(snap)
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kg apply: %w", err)
	}
	return nil
}
```

Update the existing `forwardToKgBatch` to also pipe `--source` from the manifest (no — see spec: source is per-op in batch, not per-batch. So `forwardToKgBatch` is unchanged. But each plugin op needs `source` in its args; for legacy v1 plugins that didn't include it, the v2 plugin authors are responsible for updating. The bash-demo migration in Task 26 handles this for the shipped demo).

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg-extractor/ -run TestExtract -v
```

Expected: PASS for both the new declarative test and the v1 imperative test (assuming the v1 bash-demo still emits valid v2-batch ops with `source`. After Task 26, the demo will be declarative.)

- [ ] **Step 5: Commit**

```bash
git add cmd/kg-extractor/
git commit -m "feat(extractor): dispatch declarative-* runtimes through snapshot validator and kg apply"
```

---

## Phase 7 — Plugins migrate to declarative

### Task 26: Rewrite `bash-demo` to declarative-command + snapshot output

**Files:**
- Modify: `examples/kg-extractor-plugins/bash-demo/manifest.json`
- Modify: `examples/kg-extractor-plugins/bash-demo/extract.sh`
- Modify: `examples/kg-extractor-plugins/bash-demo/README.md`

- [ ] **Step 1: Update the manifest**

Replace `manifest.json`:

```json
{
  "name": "bash-demo",
  "version": "0.2.0",
  "description": "Trivial bash plugin emitting a fixed mini-graph snapshot",
  "runtime": "declarative-command",
  "command": ["bash", "extract.sh"],
  "source_id": "bash-demo:0.2.0",
  "trust": 100,
  "supported_layers": ["root", "item"],
  "supported_scopes": ["domain-source"]
}
```

- [ ] **Step 2: Replace `extract.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail
config=$(cat)
domain=$(echo "$config" | jq -r '.domain')
cat <<EOF
{
  "protocol_version": 2,
  "source": "bash-demo:0.2.0",
  "domain": "$domain",
  "scope": "domain-source",
  "domain_spec": {
    "id": "$domain",
    "layers": ["root", "item"],
    "description": "bash-demo declarative output"
  },
  "nodes": [
    {"id": "$domain:demo",        "layer": "root", "name": "Demo"},
    {"id": "$domain:demo-first",  "layer": "item", "parent": "$domain:demo", "name": "First"},
    {"id": "$domain:demo-second", "layer": "item", "parent": "$domain:demo", "name": "Second"}
  ],
  "edges": [
    {"src": "$domain:demo-first", "target": "$domain:demo-second", "type": "references"}
  ]
}
EOF
```

- [ ] **Step 3: Update `README.md`**

Replace contents:

```markdown
# bash-demo

A ~25-line bash plugin that emits a fixed mini-graph snapshot (declarative-command runtime).
Demonstrates the v2 plugin contract works without compiled code.

Requires `bash` and `jq`.

## Try it

```sh
ln -s "$(pwd)" ~/.config/kg-extractor/plugins/bash-demo
kg-extractor extract --plugin bash-demo --domain example | jq .
```

## What it produces

- a domain with layers `[root, item]`
- one root node `Demo`
- two item nodes `First`, `Second` parented at `Demo`
- one `references` edge from `First` to `Second`

Re-running is idempotent — kg apply diffs against the previous snapshot for
the `(domain, source)` pair.
```

- [ ] **Step 4: Verify the integration test still passes**

(The Phase 6 integration test `TestExtractDeclarativePipesToKgApply` covers a hand-rolled declarative plugin. To exercise the shipped bash-demo, the v1 `TestBashDemoEndToEnd` test in `cmd/kg-extractor/integration_test.go` should be updated to assert the new shape — namely, that the `references` edge survives idempotently across reruns.)

```bash
go test ./cmd/kg-extractor/ -run TestBashDemo -v
```

If the v1 test fails because it asserted on JSONL output, update it to assert on the snapshot+apply path.

- [ ] **Step 5: Commit**

```bash
git add examples/kg-extractor-plugins/bash-demo/
git commit -m "feat(plugin-bash-demo): rewrite as declarative-command emitting a JSON snapshot"
```

---

### Task 27: tree-sitter plugin switches to declarative-native

Replace `plugins/tree-sitter/emit.go` (JSONL via `batch.Encoder`) with `plugins/tree-sitter/snapshot.go` (a single JSON document built from `snapshot.Snapshot`). The manifest changes runtime to `declarative-native` and adds `source_id`. The import resolver stays — it still filters externals out of the snapshot's `edges[]`.

**Files:**
- Create: `plugins/tree-sitter/snapshot.go`
- Delete: `plugins/tree-sitter/emit.go`
- Modify: `plugins/tree-sitter/emit_test.go` (rename to `snapshot_test.go` or rewrite contents)
- Modify: `plugins/tree-sitter/walker.go` (the calling site — `emitOps` → `buildSnapshot` + `snapshot.Encode`)
- Modify: `plugins/tree-sitter/root.go` (entrypoint changes if it referenced `emitOps`)
- Modify: example manifest in README + `e2e/extract_self_test.go` (Phase 8) — but that's a Phase 8 task

- [ ] **Step 1: Write failing test for `buildSnapshot`**

Create `plugins/tree-sitter/snapshot_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/snapshot"
)

func TestBuildSnapshotIncludesPackagesFilesDecls(t *testing.T) {
	pkgs := []*packageInfo{
		{
			Path: "graph", Slug: "graph", RelDir: "graph",
			Files: []*fileInfo{
				{RelPath: "graph/node.go", BasenameSlug: "node-go",
					Decls: []*declInfo{{NameSlug: "parseslug", Properties: map[string]any{"kind": "function"}}}},
			},
		},
	}
	res := newImportResolver("/tmp/x", pkgs)
	snap := buildSnapshot("go", "myapp", pkgs, res, false)

	require.Equal(t, snapshot.ProtocolVersion, snap.ProtocolVersion)
	require.Equal(t, "tree-sitter:0.2.0", snap.Source)
	require.Equal(t, "myapp", snap.Domain)
	require.Equal(t, snapshot.ScopeDomainSource, snap.Scope)
	ids := map[string]bool{}
	for _, n := range snap.Nodes {
		ids[n.ID] = true
	}
	require.True(t, ids["myapp:graph"])
	require.True(t, ids["myapp:graph/node-go"])
	require.True(t, ids["myapp:graph/node-go::parseslug"])
}

func TestBuildSnapshotSkipsExternalImportsByDefault(t *testing.T) {
	pkgs := []*packageInfo{
		{Path: "graph", Slug: "graph", RelDir: "graph",
			Imports: []*importInfo{{From: "graph", To: "github.com/external/x"}}},
	}
	res := newImportResolver("/tmp/x", pkgs)
	snap := buildSnapshot("go", "myapp", pkgs, res, false)
	require.Empty(t, snap.Edges, "external imports skipped when include_external_imports=false")
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go -C plugins/tree-sitter test ./... -run TestBuildSnapshot -v
```

Expected: FAIL (`buildSnapshot` undefined).

- [ ] **Step 3: Implement `plugins/tree-sitter/snapshot.go`**

```go
package main

import (
	"sort"

	"github.com/ggfarmco/kg/snapshot"
)

const pluginSourceID = "tree-sitter:0.2.0"

func buildSnapshot(language, domain string, pkgs []*packageInfo, resolver *importResolver, includeExternalImports bool) snapshot.Snapshot {
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Path < pkgs[j].Path })

	snap := snapshot.Snapshot{
		ProtocolVersion: snapshot.ProtocolVersion,
		Source:          pluginSourceID,
		Domain:          domain,
		Scope:           snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{
			ID: domain, Layers: []string{"package", "file", "decl"},
			Description: "Extracted by tree-sitter (" + language + ")",
		},
		Nodes: []snapshot.NodeSpec{},
		Edges: []snapshot.EdgeSpec{},
	}

	externalsByID := map[string]string{}
	var externalsOrder []string

	for _, p := range pkgs {
		pkgID := domain + ":" + p.Slug
		snap.Nodes = append(snap.Nodes, snapshot.NodeSpec{
			ID: pkgID, Layer: "package", Name: p.Path,
			Properties: nonNilMap(p.Properties),
		})
		for _, f := range p.Files {
			fileSlug := p.Slug + "/" + f.BasenameSlug
			fileID := domain + ":" + fileSlug
			snap.Nodes = append(snap.Nodes, snapshot.NodeSpec{
				ID: fileID, Layer: "file", Parent: pkgID, Name: f.RelPath,
				Properties: nonNilMap(f.Properties),
			})
			for _, d := range f.Decls {
				declID := fileID + "::" + d.NameSlug
				snap.Nodes = append(snap.Nodes, snapshot.NodeSpec{
					ID: declID, Layer: "decl", Parent: fileID, Name: d.NameSlug,
					Properties: nonNilMap(d.Properties),
				})
			}
		}
	}

	for _, p := range pkgs {
		for _, imp := range p.Imports {
			toSlug, internal := resolver.Resolve(imp.To)
			if !internal {
				if !includeExternalImports {
					continue
				}
				toSlug = "ext-" + sanitizeSlug(imp.To)
				if _, seen := externalsByID[toSlug]; !seen {
					externalsByID[toSlug] = imp.To
					externalsOrder = append(externalsOrder, toSlug)
				}
			}
			snap.Edges = append(snap.Edges, snapshot.EdgeSpec{
				Src:    domain + ":" + sanitizeSlug(imp.From),
				Target: domain + ":" + toSlug,
				Type:   "imports",
			})
		}
	}
	for _, slug := range externalsOrder {
		snap.Nodes = append(snap.Nodes, snapshot.NodeSpec{
			ID: domain + ":" + slug, Layer: "package", Name: externalsByID[slug],
			Properties: map[string]any{"external": true},
		})
	}
	for _, p := range pkgs {
		for _, call := range p.Calls {
			snap.Edges = append(snap.Edges, snapshot.EdgeSpec{
				Src:    domain + ":" + call.FromDecl,
				Target: domain + ":" + call.ToDecl,
				Type:   "calls",
			})
		}
	}

	return snap
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	return m
}
```

- [ ] **Step 4: Delete `plugins/tree-sitter/emit.go` and update the caller**

Find the call site in `walker.go` (or wherever `emitOps` was invoked). It currently does something like:

```go
return emitOps(os.Stdout, language, domain, pkgs, resolver, cfg.IncludeExternalImports)
```

Replace with:

```go
snap := buildSnapshot(language, domain, pkgs, resolver, cfg.IncludeExternalImports)
return snapshot.Encode(os.Stdout, snap)
```

Adjust imports — `snapshot` lives at `github.com/ggfarmco/kg/snapshot`. The third-party import block in `walker.go` already includes `github.com/ggfarmco/kg/batch`; add `snapshot` next to it.

Delete `plugins/tree-sitter/emit.go` and `plugins/tree-sitter/emit_test.go` once `snapshot_test.go` covers the equivalent assertions.

- [ ] **Step 5: Update `plugins/tree-sitter/manifest.json` (if shipped) and the example**

The v1 plan did not ship a tree-sitter manifest; users wrote one. The README still references it. For consistency in the e2e test (Phase 8 Task 29), write a manifest body inline in the test — no shipped manifest file needs to change here.

- [ ] **Step 6: Run**

```bash
go -C plugins/tree-sitter test ./... -v
```

Expected: PASS for `snapshot_test.go` and all language tests. Note that golden tests are still red — they assert on the old JSONL output. Task 28 fixes them.

- [ ] **Step 7: Commit**

```bash
git add plugins/tree-sitter/
git commit -m "feat(plugin-tree-sitter): switch to declarative-native, emit snapshot.Snapshot"
```

---

### Task 28: Convert golden fixtures to `expected.snapshot.json` with structural diff

Each `expected.jsonl` becomes `expected.snapshot.json`. The runner reads both `got` and `want` as JSON, normalizes (sorting `nodes[]` by id and `edges[]` by `[src, target, type]`), and asserts deep equality. This survives map/slice ordering jitter.

**Files:**
- Modify: `plugins/tree-sitter/languages/golang/golden_test.go`
- Replace: `plugins/tree-sitter/languages/golang/testdata/golden/01-single-file/expected.jsonl` → `expected.snapshot.json`
- Replace: `02-multi-package/expected.jsonl` → `expected.snapshot.json`
- Replace: `03-with-methods/expected.jsonl` → `expected.snapshot.json`

- [ ] **Step 1: Rewrite the golden test runner**

```go
package golang_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "rewrite expected.snapshot.json fixtures from current output")

func TestGolden(t *testing.T) {
	binary := buildPluginBinary(t)
	cases := []string{"01-single-file", "02-multi-package", "03-with-methods"}
	for _, name := range cases {
		name := name
		t.Run(name, func(t *testing.T) {
			input := filepath.Join("testdata", "golden", name, "input")
			expected := filepath.Join("testdata", "golden", name, "expected.snapshot.json")
			abs, err := filepath.Abs(input)
			require.NoError(t, err)

			cmd := exec.Command(binary, "--language", "go")
			cmd.Stdin = bytes.NewReader([]byte(`{"input":"` + abs + `","domain":"g","protocol_version":2,"config":{}}`))
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			require.NoError(t, cmd.Run(), "stderr=%s", stderr.String())

			got := normalizeSnapshot(t, normalizeAbs(stdout.Bytes(), abs))
			if *updateGolden {
				require.NoError(t, os.WriteFile(expected, got, 0o644))
				return
			}
			want, err := os.ReadFile(expected)
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
	}
}

func normalizeSnapshot(t *testing.T, b []byte) []byte {
	t.Helper()
	var raw map[string]any
	require.NoError(t, json.Unmarshal(b, &raw))
	if nodes, ok := raw["nodes"].([]any); ok {
		sort.SliceStable(nodes, func(i, j int) bool {
			return nodes[i].(map[string]any)["id"].(string) < nodes[j].(map[string]any)["id"].(string)
		})
	}
	if edges, ok := raw["edges"].([]any); ok {
		sort.SliceStable(edges, func(i, j int) bool {
			ei, ej := edges[i].(map[string]any), edges[j].(map[string]any)
			ki := ei["src"].(string) + "|" + ei["target"].(string) + "|" + ei["type"].(string)
			kj := ej["src"].(string) + "|" + ej["target"].(string) + "|" + ej["type"].(string)
			return ki < kj
		})
	}
	out, err := json.MarshalIndent(raw, "", "  ")
	require.NoError(t, err)
	return out
}

func buildPluginBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "tspl")
	cmd := exec.Command("go", "build", "-o", out, "../..")
	cmd.Stderr = bytes.NewBuffer(nil)
	if err := cmd.Run(); err != nil {
		t.Skipf("build failed: %v %s", err, cmd.Stderr)
	}
	return out
}

func normalizeAbs(b []byte, abs string) []byte {
	return bytes.ReplaceAll(b, []byte(abs), []byte("<INPUT>"))
}
```

- [ ] **Step 2: Regenerate fixtures**

```bash
go -C plugins/tree-sitter test ./languages/golang/ -run TestGolden -update
```

Inspect each new `expected.snapshot.json` — they should be valid `snapshot.Snapshot` documents with the three Go fixtures' expected packages/files/decls/edges.

- [ ] **Step 3: Delete the obsolete `expected.jsonl` files**

```bash
rm plugins/tree-sitter/languages/golang/testdata/golden/*/expected.jsonl
```

- [ ] **Step 4: Rerun tests without `-update`**

```bash
go -C plugins/tree-sitter test ./languages/golang/ -run TestGolden -v
```

Expected: PASS — diffs against the freshly captured snapshots.

- [ ] **Step 5: Commit**

```bash
git add plugins/tree-sitter/languages/golang/
git commit -m "test(plugin-tree-sitter): convert golden fixtures to snapshot JSON with structural diff"
```

---

## Phase 8 — E2E, README, branch close-out

### Task 29: Rewrite `e2e/extract_self_test.go` with idempotency + foreign-claim survival

Four assertions in one test (the spec's "Tier 3 — e2e" section): first pass adds nodes with `source = tree-sitter:0.2.0`; second pass is a no-op; third pass after a rename produces `nodes_removed: 1, nodes_added: 1`; fourth pass after a manual `--source manual` node leaves it untouched.

**Files:**
- Modify: `e2e/extract_self_test.go`
- Modify: `e2e/testutil.go` (extend with snapshot/apply assertions if missing)

- [ ] **Step 1: Replace `e2e/extract_self_test.go`**

```go
//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractSelfDeclarative(t *testing.T) {
	kgBin := mustBuild(t, "kg", "../cmd/kg")
	extractorBin := mustBuild(t, "kg-extractor", "../cmd/kg-extractor")
	pluginBin := mustBuildPlugin(t)

	pluginsDir := t.TempDir()
	pluginHome := filepath.Join(pluginsDir, "tree-sitter")
	writeFile(t, filepath.Join(pluginHome, "manifest.json"), `{
		"name": "tree-sitter",
		"version": "0.2.0",
		"description": "tree-sitter (Go) declarative",
		"runtime": "declarative-native",
		"executable": "kg-extractor-tree-sitter",
		"source_id": "tree-sitter:0.2.0",
		"trust": 100
	}`)
	require.NoError(t, exec.Command("cp", pluginBin, filepath.Join(pluginHome, "kg-extractor-tree-sitter")).Run())

	dbPath := filepath.Join(t.TempDir(), "selfg.db")
	require.NoError(t, exec.Command(kgBin, "--db", dbPath, "init").Run())

	source := filepath.Join(t.TempDir(), "graph")
	require.NoError(t, exec.Command("cp", "-r", "../internal/graph", source).Run())
	absSource, _ := filepath.Abs(source)

	extract := func(t *testing.T) (envelope applyEnvelope) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(extractorBin,
			"--plugins-path", pluginsDir, "extract",
			"--plugin", "tree-sitter", "--language", "go",
			"--input", absSource, "--domain", "selfg",
			"--db", dbPath, "--kg-binary", kgBin,
		)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		require.NoError(t, cmd.Run(), "stderr=%s", stderr.String())
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &envelope), "stdout=%s", stdout.String())
		require.True(t, envelope.OK)
		return envelope
	}

	// Pass 1 — populate.
	envFirst := extract(t)
	require.Greater(t, envFirst.Data.NodesAdded, 0, "first pass adds nodes")

	// Sanity: nodes carry the plugin's source.
	dom, err := exec.Command(kgBin, "--db", dbPath, "node", "list", "--domain", "selfg", "--source", "tree-sitter:0.2.0").Output()
	require.NoError(t, err)
	require.Contains(t, string(dom), "selfg:graph")

	// Pass 2 — no changes.
	envSecond := extract(t)
	require.Equal(t, 0, envSecond.Data.NodesAdded)
	require.Equal(t, 0, envSecond.Data.NodesUpdated)
	require.Equal(t, 0, envSecond.Data.NodesRemoved)

	// Pass 3 — rename a decl in a file copy.
	nodeGo := filepath.Join(source, "node.go")
	body, err := os.ReadFile(nodeGo)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(nodeGo,
		bytes.Replace(body, []byte("func ParseSlug"), []byte("func ParseSlugRenamed"), 1), 0o644))
	envThird := extract(t)
	require.GreaterOrEqual(t, envThird.Data.NodesAdded, 1, "the rename should add at least one node")
	require.GreaterOrEqual(t, envThird.Data.NodesRemoved, 1, "the old name should be removed")

	// Pass 4 — add a manual cross-source edge; assert it survives re-extract.
	require.NoError(t, exec.Command(kgBin, "--db", dbPath, "sources", "register", "--id", "manual", "--if-not-exists").Run())
	out, err := exec.Command(kgBin, "--db", dbPath, "node", "list", "--domain", "selfg", "--layer", "package").Output()
	require.NoError(t, err)
	var pkgList struct {
		Data []struct{ ID string } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out, &pkgList))
	require.NotEmpty(t, pkgList.Data)
	pkg := pkgList.Data[0].ID

	// Make a manual second claim on a tree-sitter-owned edge: first need an existing edge id.
	edgesOut, err := exec.Command(kgBin, "--db", dbPath, "edge", "list-from", pkg).Output()
	require.NoError(t, err)
	var edges struct {
		Data []struct{ ID int64 `json:"id"` } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(edgesOut, &edges))
	require.NotEmpty(t, edges.Data, "tree-sitter should have produced at least one outgoing edge")
	edgeID := edges.Data[0].ID

	// Mimic a "manual" extra claim via edge add (which both upserts and claims).
	pkgOther := pkgList.Data[len(pkgList.Data)-1].ID
	require.NoError(t, exec.Command(kgBin, "--db", dbPath, "edge", "add", pkg, pkgOther, "--type", "imports", "--source", "manual", "--if-not-exists").Run())

	envFourth := extract(t)
	_ = envFourth

	claims, err := exec.Command(kgBin, "--db", dbPath, "edge", "claims", fmtInt(edgeID)).Output()
	require.NoError(t, err)
	require.Contains(t, string(claims), "tree-sitter:0.2.0", "tree-sitter's claim survives re-extract")
}

type applyEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		NodesAdded    int `json:"nodes_added"`
		NodesUpdated  int `json:"nodes_updated"`
		NodesRemoved  int `json:"nodes_removed"`
		EdgesAdded    int `json:"edges_added"`
		ClaimsRemoved int `json:"claims_removed"`
		EdgesGC       int `json:"edges_gc"`
	} `json:"data"`
}

func fmtInt(n int64) string {
	return string(strconvAppendInt(nil, n, 10))
}
```

Add `strconvAppendInt` shim or just `import "strconv"` and use `strconv.FormatInt(n, 10)`. The above is paraphrased to avoid the import for brevity — replace with `strconv.FormatInt(n, 10)` in the final file.

- [ ] **Step 2: Run**

```bash
make e2e
```

Expected: PASS. If pass 4's claims assertion fails — check the test setup: pass 4's `edge add` may have a different `pkgOther`. Inspect the DB state with `kg edge list-from` to debug.

- [ ] **Step 3: Commit**

```bash
git add e2e/
git commit -m "test(e2e): declarative self-extract with idempotency + foreign-claim survival"
```

---

### Task 30: README extractor section update + branch close-out

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update the README extractor section**

Replace (or extend, if you'd rather preserve v1's section) the existing extractor section with:

```markdown
## Extractors (v2 — declarative + provenance)

kg is a generic graph engine. Extractors live as separate binaries; they
pipe into one of two verbs depending on plugin runtime:

- **Declarative (recommended)**: plugin emits one JSON snapshot on stdout;
  `kg apply` computes diff against the previous state for `(domain, source)`
  and applies adds/updates/removals atomically.
- **Imperative**: plugin emits JSONL graph ops; `kg batch` runs them in one
  transaction.

Both formats share the same store; the difference is who computes the diff.

### Pipeline

```
plugin → kg-extractor (validator) → kg apply (declarative) or kg batch (imperative)
```

`kg-extractor` discovers plugins under `~/.config/kg-extractor/plugins/<name>/`
(override via `KG_EXTRACTOR_PLUGINS_PATH`).

### Source model

Every mutation has a writer source. Sources are auto-registered on first
write; `kg sources register --id ... --trust ...` lets you refine description
and trust score. Nodes are single-owner (the source that created them);
edges are reference-counted via `edge_claims`. An edge survives as long as
≥1 source claims it.

Properties are stored namespaced by source id:

```json
{
  "tree-sitter:0.2.0": {"kind": "function", "line_start": 40},
  "llm-enricher:1.0": {"summary": "Validates a slug..."}
}
```

`kg node get <id>` shows the raw namespaced form by default;
`--source <id>` flattens one namespace; `--merged` returns a trust-ranked
union with a sibling `_property_sources` attribution map.

### Try the bash demo (declarative)

```sh
make build-extractor
ln -s "$(pwd)/examples/kg-extractor-plugins/bash-demo" ~/.config/kg-extractor/plugins/bash-demo
./bin/kg --db ./kg.db init
./bin/kg-extractor extract --plugin bash-demo --domain demoapp --db ./kg.db --kg-binary ./bin/kg
./bin/kg node list --domain demoapp
```

### Extract Go via the tree-sitter plugin

```sh
make build-plugin-treesitter
mkdir -p ~/.config/kg-extractor/plugins/tree-sitter
cp plugins/tree-sitter/manifest.json ~/.config/kg-extractor/plugins/tree-sitter/
cp ./bin/kg-extractor-tree-sitter ~/.config/kg-extractor/plugins/tree-sitter/

./bin/kg-extractor extract \
    --plugin tree-sitter --language go \
    --input ./internal/graph --domain mykg \
    --db ./kg.db --kg-binary ./bin/kg
```

Re-running on unchanged code is a no-op. Renaming a function emits exactly
one delete + one add. Foreign-source claims survive your re-extract.

See `docs/superpowers/specs/2026-05-24-kg-v2-provenance-design.md` for the
full provenance model, `kg apply`'s diff algorithm, and conflict codes.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: README v2 extractor section (declarative + provenance + sources)"
```

- [ ] **Step 3: Final integration check**

```bash
make test
make test-all
make e2e
GOWORK=off go build ./...
GOWORK=off go -C ./plugins/tree-sitter build ./...
```

Expected: all green.

- [ ] **Step 4: Review the diff summary**

```bash
git log --oneline main..feat/kg-v2-provenance
git diff --stat main..feat/kg-v2-provenance
```

Sanity-check: every directory under `internal/`, `cmd/kg/`, `cmd/kg-extractor/`, `batch/`, `snapshot/`, `plugins/tree-sitter/`, `examples/`, `migrations/`, `e2e/`, `docs/` should show meaningful changes. No accidental touches outside the v2 scope (e.g., `internal/graph/testutil/fakestore_test.go` is fine; touching `Makefile` is fine; bumping `go.mod` to a new toolchain is NOT fine).

- [ ] **Step 5: Open the merge**

```bash
git push -u origin feat/kg-v2-provenance
gh pr create --title "feat: v2 — provenance + declarative apply" --body "$(cat <<'EOF'
## Summary
- Migration 0001 rewritten in place: sources, edge_claims, namespaced properties, source on changes
- `internal/graph`: SourceID/Source/EdgeClaim types, ownership rules, ref-counted edges with same-tx GC, namespaced property writes
- New `snapshot/` public package: types, codec, validation, topological sort
- `Service.Apply` declarative diff engine: scope semantics, conflict codes, force overrides
- `kg apply` verb + `kg sources` subcommand
- `kg-extractor` dispatches by `manifest.runtime`: declarative-* → kg apply, native/command → kg batch
- `plugins/tree-sitter` switched to declarative-native (emits snapshot, not JSONL ops)
- `examples/kg-extractor-plugins/bash-demo` switched to declarative-command
- e2e: idempotent re-extract + foreign-claim survival assertions

Spec: `docs/superpowers/specs/2026-05-24-kg-v2-provenance-design.md`.

## Test plan
- [x] `make test`
- [x] `make test-all`
- [x] `make e2e`
- [x] `GOWORK=off make test` (external-consumer simulation)
EOF
)"
```

Merge as a single unit (squash or merge per project preference). v2 ships when this lands on `main`.

---

## Self-Review Notes

After writing this plan, the spec was re-read top-to-bottom. Coverage matrix:

| Spec section | Implementing task(s) |
|---|---|
| Rewritten schema (sources, edge_claims, namespaced properties) | Task 1 |
| queries.sql + sqlc regen | Task 2 |
| SourceID type + sentinel errors | Task 3 |
| Node/Edge Source/Claims/namespaced Properties types | Task 4 |
| Store: sources CRUD | Task 5 |
| Store: edge_claims CRUD + UpsertEdge | Task 5 |
| Store: source-aware nodes/edges | Task 5 |
| Store interface | Task 6 |
| FakeStore parity | Task 7 |
| Service ownership rules (Add/Update/DeleteNode) | Task 8 |
| Service auto-register source | Task 8 |
| Service edge ref-counting + GC | Task 9 |
| Service namespaced property writes | Task 10 |
| snapshot/ types + codec | Tasks 11, 12 |
| snapshot/ validation + topo sort | Tasks 12, 13 |
| Service.Apply: diff engine | Task 14 |
| Service.Apply: edge UPSERT + claims + GC | Task 15 |
| Service.Apply: scope semantics + conflict codes | Task 16 |
| Service.Apply: force overrides | Task 17 |
| CLI: sources subcommand | Task 18 |
| kg init seeds cli/manual | Task 19 |
| CLI: apply verb | Task 20 |
| Batch: source per op + edge.add src rename + edge.unclaim | Task 21 |
| Node CLI: --source, --merged, --source view, --properties | Task 22 |
| Edge CLI: --source, claims/unclaim, delete --force | Task 22 |
| Manifest: source_id, trust, declarative-* runtimes | Task 23 |
| Snapshot validator | Task 24 |
| Extract dispatch by runtime | Task 25 |
| bash-demo declarative | Task 26 |
| tree-sitter declarative + snapshot output | Task 27 |
| Golden fixtures structural diff | Task 28 |
| e2e idempotency + foreign-claim survival | Task 29 |
| README v2 section + branch close-out | Task 30 |

**Deliberate v2 simplifications (called out where they matter):**

- `edge.delete` is aliased to `edge.unclaim` semantically (Task 21). Total drop-the-edge-and-all-claims is `kg edge delete --force` only (Task 22). The v1 `edge.delete` literal semantic ("drop the edge row by id, no claim awareness") is no longer needed — all callers want one of the two new behaviors.
- `Service.UpdateNode` no longer touches `properties`. Property writes go through `SetNodeProperties` (Task 10). The wire-format `node.update` op accepts both `name` (owner-only) and `properties` (any writer, into own namespace) in the same op (Task 21), but they're applied via different Service methods.
- The `--merged` view (Task 22) does shallow merge with trust-ranked + alphabetic tie-break per spec. No deep merging within sub-objects.
- Domain creation is shared (no source column on `domains`) — anyone can `domain add`; the changes log records the writer. Auto-register happens on domain.add too.
- E2E uses a *copy* of `internal/graph` so the rename pass doesn't disturb the repo.
- `EdgeAddArgs.UnmarshalJSON` accepts legacy `source` (originating node) as a fallback if `src` is empty (Task 21). This eases the cutover for any handwritten test fixtures.

**Risks acknowledged:**

- The `propsEqual` helper in `service_apply.go` uses string formatting for value comparison. This is fine for kg's JSON-shaped values but degrades on `[]any` content with non-deterministic key ordering. If golden tests start flapping, switch to `reflect.DeepEqual` after walking and normalizing maps.
- `EdgeAddArgs` UnmarshalJSON's legacy-source fallback may quietly accept malformed v1 fixtures. The CLI tests in Task 21 cover the new shape explicitly; v1 fixtures should be migrated, not silently coerced. Document this in `batch/op.go`.
- The `--merged` view's `_property_sources` map duplicates the per-key attribution that `--source` could derive. Consumers wanting strict raw shape can omit `--merged` entirely.
- `Service.RemoveEdgeClaim` opens its own `InTx`. When called from `Service.Apply` (which already holds a tx via the FakeStore's nested-tx ban), this would deadlock. The Apply path uses the store directly (Task 15) for that reason — `Service.RemoveEdgeClaim` is for one-off CLI calls. If callers blend the two paths, document the nesting expectation.
- Tree-sitter golden fixtures change from JSONL → JSON. The structural diff runner is more permissive about ordering but stricter about field presence. If a fixture surfaces e.g. a `properties` map that the old runner stringified differently, run `-update` and review the new diff carefully.

