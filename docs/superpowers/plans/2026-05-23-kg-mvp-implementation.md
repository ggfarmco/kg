# kg MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a domain-agnostic knowledge graph engine in Go (`kg`) with SQLite storage and a cobra-based CLI, exposing four tables (domains, nodes, edges, changes), a versioned `revision` foundation, and an LLM-friendly JSON envelope.

**Architecture:** Hexagonal split: `internal/graph` holds pure Go types, sentinel errors, a `Store` interface, and a `Service` of use cases + validation. `internal/store` is the SQLite adapter built on `modernc.org/sqlite` + sqlc, with a `context`-carried transaction (`InTx`). `cmd/kg` is a thin cobra adapter that opens the store, calls service methods, and emits the JSON envelope.

**Tech Stack:** Go 1.26, `github.com/spf13/cobra`, `modernc.org/sqlite` (pure-Go, no CGO), `github.com/sqlc-dev/sqlc` (typed queries, committed generated code), `github.com/pressly/goose/v3` (embedded migrations), `github.com/stretchr/testify/require`, `golangci-lint`, Makefile.

**Spec:** `docs/superpowers/specs/2026-05-23-kg-mvp-design.md`. Re-read it before each new phase — the Background section justifies foundation choices (revision, changes, `--if-not-exists`) that look unmotivated without context.

**Conventions:**
- Import grouping (3 blocks separated by blank lines): stdlib, third-party, current module (`github.com/ggfarmco/kg/...`).
- No comments in code unless they explain a non-obvious *why*. Generated sqlc files are exempt.
- Tests are minimal and non-redundant — each test covers one distinct behavior.
- Every task ends with a commit. Commit messages follow `<type>: <imperative summary>` (types: feat, test, chore, docs, refactor, fix).

---

## Phase 1 — Foundation

### Task 1: Repository scaffolding

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `.golangci.yml`
- Create: `sqlc.yaml`
- Create: `tools.go`
- Create: `Makefile`
- Create: `README.md`
- Create: `cmd/kg/.keep`, `internal/graph/.keep`, `internal/store/.keep`, `migrations/.keep`

- [ ] **Step 1: Initialize Go module**

```bash
go mod init github.com/ggfarmco/kg
```

Expected: creates `go.mod` containing `module github.com/ggfarmco/kg` and `go 1.26`.

- [ ] **Step 2: Add runtime and test dependencies**

```bash
go get github.com/spf13/cobra@latest
go get modernc.org/sqlite@latest
go get github.com/pressly/goose/v3@latest
go get github.com/stretchr/testify@latest
```

Expected: `go.mod` lists the four modules; `go.sum` is created.

- [ ] **Step 3: Create `tools.go` to pin dev tool versions**

Create `tools.go`:

```go
//go:build tools

package tools

import (
	_ "github.com/pressly/goose/v3/cmd/goose"
	_ "github.com/sqlc-dev/sqlc/cmd/sqlc"
)
```

Then:

```bash
go get github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go mod tidy
```

Expected: `go.mod` lists `github.com/sqlc-dev/sqlc` as a dependency (the `//go:build tools` tag keeps it out of the production binary).

- [ ] **Step 4: Create `.gitignore`**

Create `.gitignore`:

```
/bin/
*.db
*.db-wal
*.db-shm
.DS_Store
```

- [ ] **Step 5: Create `sqlc.yaml`**

Create `sqlc.yaml`:

```yaml
version: "2"
sql:
  - engine: "sqlite"
    queries: "internal/store/queries.sql"
    schema: "migrations"
    gen:
      go:
        package: "store"
        out: "internal/store"
        sql_package: "database/sql"
        emit_json_tags: false
        emit_interface: false
        emit_pointers_for_null_types: true
        emit_empty_slices: true
```

- [ ] **Step 6: Create `.golangci.yml`**

Create `.golangci.yml`:

```yaml
version: "2"
run:
  timeout: 5m
linters:
  default: standard
  enable:
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused
issues:
  exclude-dirs:
    - internal/store
```

(`internal/store` is excluded because sqlc-generated files live there and they're treated as a derived artifact.)

- [ ] **Step 7: Create `Makefile`**

Create `Makefile`:

```makefile
.PHONY: build test gen migrate lint install clean

BIN := ./bin/kg
DB  ?= ./kg.db

build:
	@mkdir -p bin
	go build -o $(BIN) ./cmd/kg

test:
	go test ./...

gen:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc generate

migrate:
	go run github.com/pressly/goose/v3/cmd/goose -dir migrations sqlite3 $(DB) up

lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint run

install:
	go install ./cmd/kg

clean:
	rm -rf bin *.db *.db-wal *.db-shm
```

- [ ] **Step 8: Create directory placeholders**

```bash
mkdir -p cmd/kg internal/graph internal/store migrations
touch cmd/kg/.keep internal/graph/.keep internal/store/.keep migrations/.keep
```

- [ ] **Step 9: Create README stub**

Create `README.md`:

```markdown
# kg

Domain-agnostic knowledge graph engine in Go with SQLite storage.

See `docs/superpowers/specs/2026-05-23-kg-mvp-design.md` for the design.

## Quick start

```sh
make build
./bin/kg init
```

A full walkthrough lands at the end of MVP implementation.
```

- [ ] **Step 10: Verify build passes**

```bash
go build ./...
```

Expected: no output, exit 0 (no Go files yet but the module compiles).

- [ ] **Step 11: Commit**

```bash
git add .
git commit -m "chore: scaffold go module, tooling, and project layout"
```

---

### Task 2: Migration 0001_init.sql + smoke test

**Files:**
- Create: `migrations/0001_init.sql`
- Create: `internal/store/store.go` (placeholder so `internal/store` is a valid package)
- Create: `internal/store/migration_test.go`

- [ ] **Step 1: Write `migrations/0001_init.sql`**

Create `migrations/0001_init.sql`:

```sql
-- +goose Up
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
  summary     TEXT,
  properties  TEXT NOT NULL DEFAULT '{}',
  revision    INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);

CREATE INDEX idx_nodes_domain_layer ON nodes(domain, layer);
CREATE INDEX idx_nodes_parent       ON nodes(parent_id);

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

CREATE TABLE changes (
  seq         INTEGER PRIMARY KEY AUTOINCREMENT,
  entity      TEXT NOT NULL,
  entity_id   TEXT NOT NULL,
  op          TEXT NOT NULL,
  revision    INTEGER,
  at          INTEGER NOT NULL
);

CREATE INDEX idx_changes_seq    ON changes(seq);
CREATE INDEX idx_changes_entity ON changes(entity, entity_id);

-- +goose Down
DROP INDEX IF EXISTS idx_changes_entity;
DROP INDEX IF EXISTS idx_changes_seq;
DROP TABLE IF EXISTS changes;
DROP INDEX IF EXISTS idx_edges_target;
DROP INDEX IF EXISTS idx_edges_source;
DROP TABLE IF EXISTS edges;
DROP INDEX IF EXISTS idx_nodes_parent;
DROP INDEX IF EXISTS idx_nodes_domain_layer;
DROP TABLE IF EXISTS nodes;
DROP TABLE IF EXISTS domains;
```

The spec specifies `nodes.parent_id` with `ON DELETE SET NULL`; we use `ON DELETE RESTRICT` per the "ON DELETE policies" subsection ("RESTRICT was chosen over SET NULL to keep the 'non-top-layer must have a parent' invariant true"). The CREATE TABLE block in the spec is the schema sketch; the policies subsection is the authoritative behavior.

- [ ] **Step 2: Remove `internal/store/.keep` and add `store.go` placeholder**

```bash
rm internal/store/.keep
```

Create `internal/store/store.go`:

```go
package store
```

- [ ] **Step 3: Write the failing migration smoke test**

Create `internal/store/migration_test.go`:

```go
package store

import (
	"database/sql"
	"embed"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

//go:embed ../../migrations/*.sql
var testMigrationsFS embed.FS

func TestMigrationsUp(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	goose.SetBaseFS(testMigrationsFS)
	t.Cleanup(func() { goose.SetBaseFS(nil) })
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpContext(t.Context(), db, "../../migrations"))

	want := []string{"domains", "nodes", "edges", "changes"}
	for _, name := range want {
		var got string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&got)
		require.NoError(t, err, "table %s should exist", name)
		require.Equal(t, name, got)
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/store/ -run TestMigrationsUp -v
```

Expected: PASS. (If it fails on `embed` path resolution, the `//go:embed ../../migrations/*.sql` directive may require migrations referenced via a relative path that climbs out of the package; the test already accounts for this.)

- [ ] **Step 5: Run full build to make sure nothing else broke**

```bash
go build ./... && go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add migrations/0001_init.sql internal/store/store.go internal/store/migration_test.go
git commit -m "feat(store): add initial schema migration and smoke test"
```

---

## Phase 2 — Graph core types

### Task 3: Identifier types and parsers

**Files:**
- Create: `internal/graph/ids.go`
- Create: `internal/graph/ids_test.go`

- [ ] **Step 1: Write failing tests for `ParseSlug`, `ParseDomainID`, `NewNodeID`, `NodeID.Split`**

Remove `internal/graph/.keep` first:

```bash
rm internal/graph/.keep
```

Create `internal/graph/ids_test.go`:

```go
package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestParseSlug(t *testing.T) {
	cases := []struct {
		in      string
		want    graph.SlugID
		wantErr bool
	}{
		{"engine", "engine", false},
		{"v8-engine", "v8-engine", false},
		{"v8", "v8", false},
		{"Engine", "", true},
		{"", "", true},
		{"with space", "", true},
		{"with:colon", "", true},
	}
	for _, tc := range cases {
		got, err := graph.ParseSlug(tc.in)
		if tc.wantErr {
			require.ErrorIs(t, err, graph.ErrInvalidSlug, "input=%q", tc.in)
			continue
		}
		require.NoError(t, err, "input=%q", tc.in)
		require.Equal(t, tc.want, got)
	}
}

func TestParseDomainID(t *testing.T) {
	got, err := graph.ParseDomainID("cars")
	require.NoError(t, err)
	require.Equal(t, graph.DomainID("cars"), got)

	_, err = graph.ParseDomainID("Cars")
	require.ErrorIs(t, err, graph.ErrInvalidSlug)
}

func TestNodeIDRoundtrip(t *testing.T) {
	id := graph.NewNodeID("cars", "engine")
	require.Equal(t, graph.NodeID("cars:engine"), id)

	d, s, err := id.Split()
	require.NoError(t, err)
	require.Equal(t, graph.DomainID("cars"), d)
	require.Equal(t, graph.SlugID("engine"), s)
}

func TestNodeIDSplitInvalid(t *testing.T) {
	_, _, err := graph.NodeID("no-colon").Split()
	require.Error(t, err)

	_, _, err = graph.NodeID("Cars:engine").Split()
	require.ErrorIs(t, err, graph.ErrInvalidSlug)
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/graph/ -run 'Parse|NodeID' -v
```

Expected: FAIL (package doesn't compile — `graph.ParseSlug` undefined).

- [ ] **Step 3: Implement `internal/graph/ids.go`**

Create `internal/graph/ids.go`:

```go
package graph

import (
	"errors"
	"regexp"
	"strings"
)

type (
	DomainID string
	SlugID   string
	NodeID   string
	EdgeID   int64
)

var ErrInvalidSlug = errors.New("invalid slug")

var slugRE = regexp.MustCompile(`^[a-z0-9-]+$`)

func ParseSlug(s string) (SlugID, error) {
	if !slugRE.MatchString(s) {
		return "", ErrInvalidSlug
	}
	return SlugID(s), nil
}

func ParseDomainID(s string) (DomainID, error) {
	if !slugRE.MatchString(s) {
		return "", ErrInvalidSlug
	}
	return DomainID(s), nil
}

func NewNodeID(d DomainID, s SlugID) NodeID {
	return NodeID(string(d) + ":" + string(s))
}

func (n NodeID) Split() (DomainID, SlugID, error) {
	parts := strings.SplitN(string(n), ":", 2)
	if len(parts) != 2 {
		return "", "", errors.New("node id missing ':'")
	}
	d, err := ParseDomainID(parts[0])
	if err != nil {
		return "", "", err
	}
	s, err := ParseSlug(parts[1])
	if err != nil {
		return "", "", err
	}
	return d, s, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
go test ./internal/graph/ -run 'Parse|NodeID' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/ids.go internal/graph/ids_test.go
git commit -m "feat(graph): add typed identifiers and parsers"
```

---

### Task 4: Domain, Node, Edge, NodeFilter structs

**Files:**
- Create: `internal/graph/types.go`

- [ ] **Step 1: Implement the model types**

Create `internal/graph/types.go`:

```go
package graph

import "time"

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
	Domain DomainID
	Layer  string
	Limit  int
}
```

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/graph/types.go
git commit -m "feat(graph): add Domain, Node, Edge, and NodeFilter types"
```

---

### Task 5: Sentinel errors

**Files:**
- Create: `internal/graph/errors.go`

- [ ] **Step 1: Implement sentinel errors**

Create `internal/graph/errors.go` (note `ErrInvalidSlug` is already defined in `ids.go` — do not redeclare):

```go
package graph

import "errors"

var (
	ErrDomainNotFound           = errors.New("domain not found")
	ErrDomainAlreadyExists      = errors.New("domain already exists")
	ErrLayerNotInDomain         = errors.New("layer not in domain")
	ErrSlugCannotDerive         = errors.New("cannot derive slug from name")
	ErrNodeNotFound             = errors.New("node not found")
	ErrNodeAlreadyExists        = errors.New("node already exists")
	ErrParentDomainMismatch     = errors.New("parent in different domain")
	ErrParentLayerMismatch      = errors.New("parent layer not one above")
	ErrTopLayerCannotHaveParent = errors.New("top-layer node cannot have parent")
	ErrEdgeSelfLoop             = errors.New("edge self-loop not allowed")
	ErrEdgeAlreadyExists        = errors.New("edge already exists")
	ErrEdgeNotFound             = errors.New("edge not found")
	ErrNestedTransaction        = errors.New("nested InTx is not supported")
)
```

(`ErrEdgeNotFound` and `ErrNestedTransaction` aren't in the spec's enumerated list but are obvious counterparts; the spec uses "sentinel errors" elastically and explicitly says "CLI consumers identify them via `errors.Is`".)

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/graph/errors.go
git commit -m "feat(graph): add sentinel errors"
```

---

### Task 6: Store interface

**Files:**
- Create: `internal/graph/store.go`

- [ ] **Step 1: Declare the `Store` interface**

Create `internal/graph/store.go`:

```go
package graph

import "context"

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

	CreateEdge(ctx context.Context, e *Edge) error
	GetEdge(ctx context.Context, id EdgeID) (*Edge, error)
	DeleteEdge(ctx context.Context, id EdgeID) error
	EdgesFrom(ctx context.Context, sourceID NodeID, types []string) ([]Edge, error)
	EdgesTo(ctx context.Context, targetID NodeID, types []string) ([]Edge, error)
}
```

(`CreateEdge` takes `*Edge` so the assigned `EdgeID` can flow back to the caller. The spec lists `CreateEdge(ctx, e Edge) error`; we adjust this one signature so the CLI can render the new edge's ID without a follow-up query.)

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/graph/store.go
git commit -m "feat(graph): declare Store interface"
```

---

### Task 7: In-memory fake Store

**Files:**
- Create: `internal/graph/testutil/fakestore.go`
- Create: `internal/graph/testutil/fakestore_test.go`

- [ ] **Step 1: Write a round-trip sanity test for the fake**

Create `internal/graph/testutil/fakestore_test.go`:

```go
package testutil_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/graph/testutil"
)

func TestFakeStoreRoundtrip(t *testing.T) {
	s := testutil.NewFakeStore()
	ctx := t.Context()

	d := graph.Domain{
		ID:        "cars",
		Layers:    []string{"system", "subsystem", "part"},
		CreatedAt: time.UnixMilli(1),
	}
	require.NoError(t, s.CreateDomain(ctx, d))

	got, err := s.GetDomain(ctx, "cars")
	require.NoError(t, err)
	require.Equal(t, d.ID, got.ID)
	require.Equal(t, d.Layers, got.Layers)

	require.ErrorIs(t, s.CreateDomain(ctx, d), graph.ErrDomainAlreadyExists)

	_, err = s.GetDomain(ctx, "missing")
	require.ErrorIs(t, err, graph.ErrDomainNotFound)
}

func TestFakeStoreInTxRollback(t *testing.T) {
	s := testutil.NewFakeStore()
	ctx := t.Context()
	require.NoError(t, s.CreateDomain(ctx, graph.Domain{ID: "cars", Layers: []string{"system"}, CreatedAt: time.UnixMilli(1)}))

	wantErr := graph.ErrDomainAlreadyExists
	err := s.InTx(ctx, func(ctx context.Context) error {
		_ = s.DeleteDomain(ctx, "cars")
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)

	_, err = s.GetDomain(ctx, "cars")
	require.NoError(t, err, "rollback should restore the domain")
}
```

(Add `"context"` to the import block — placed in the test file's stdlib group.)

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/graph/testutil/ -v
```

Expected: FAIL (`testutil` package doesn't exist).

- [ ] **Step 3: Implement the fake**

Create `internal/graph/testutil/fakestore.go`:

```go
package testutil

import (
	"context"
	"slices"
	"sync"

	"github.com/ggfarmco/kg/internal/graph"
)

type FakeStore struct {
	mu       sync.Mutex
	domains  map[graph.DomainID]graph.Domain
	nodes    map[graph.NodeID]graph.Node
	edges    map[graph.EdgeID]graph.Edge
	nextEdge graph.EdgeID
	inTx     bool
}

func NewFakeStore() *FakeStore {
	return &FakeStore{
		domains:  map[graph.DomainID]graph.Domain{},
		nodes:    map[graph.NodeID]graph.Node{},
		edges:    map[graph.EdgeID]graph.Edge{},
		nextEdge: 1,
	}
}

func (s *FakeStore) snapshot() *FakeStore {
	cp := &FakeStore{
		domains:  make(map[graph.DomainID]graph.Domain, len(s.domains)),
		nodes:    make(map[graph.NodeID]graph.Node, len(s.nodes)),
		edges:    make(map[graph.EdgeID]graph.Edge, len(s.edges)),
		nextEdge: s.nextEdge,
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
	return cp
}

func (s *FakeStore) restore(cp *FakeStore) {
	s.domains = cp.domains
	s.nodes = cp.nodes
	s.edges = cp.edges
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
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
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
			return graph.ErrDomainNotFound // surfaced as a generic delete failure; spec uses RESTRICT
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
			return graph.ErrNodeNotFound // surfaced as a generic delete failure; spec uses RESTRICT
		}
	}
	delete(s.nodes, id)
	for k, e := range s.edges {
		if e.SourceID == id || e.TargetID == id {
			delete(s.edges, k)
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

func (s *FakeStore) CreateEdge(_ context.Context, e *graph.Edge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.edges {
		if existing.SourceID == e.SourceID && existing.TargetID == e.TargetID && existing.Type == e.Type {
			return graph.ErrEdgeAlreadyExists
		}
	}
	e.ID = s.nextEdge
	s.nextEdge++
	s.edges[e.ID] = *e
	return nil
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

func (s *FakeStore) DeleteEdge(_ context.Context, id graph.EdgeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.edges[id]; !ok {
		return graph.ErrEdgeNotFound
	}
	delete(s.edges, id)
	return nil
}

func (s *FakeStore) EdgesFrom(_ context.Context, sourceID graph.NodeID, types []string) ([]graph.Edge, error) {
	return s.edgesMatching(func(e graph.Edge) bool { return e.SourceID == sourceID }, types), nil
}

func (s *FakeStore) EdgesTo(_ context.Context, targetID graph.NodeID, types []string) ([]graph.Edge, error) {
	return s.edgesMatching(func(e graph.Edge) bool { return e.TargetID == targetID }, types), nil
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

var _ graph.Store = (*FakeStore)(nil)
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
go test ./internal/graph/testutil/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/testutil/
git commit -m "test(graph): add in-memory fake Store with rollback support"
```

---

## Phase 3 — Service validation

### Task 8: Service struct and AddDomain

**Files:**
- Create: `internal/graph/service.go`
- Create: `internal/graph/service_domain_test.go`

- [ ] **Step 1: Write failing tests for `AddDomain`**

Create `internal/graph/service_domain_test.go`:

```go
package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/graph/testutil"
)

func newService(t *testing.T) (*graph.Service, *testutil.FakeStore) {
	t.Helper()
	fs := testutil.NewFakeStore()
	clock := func() time.Time { return time.UnixMilli(1_700_000_000_000) }
	return graph.NewService(fs, clock), fs
}

func TestAddDomainHappyPath(t *testing.T) {
	svc, fs := newService(t)
	ctx := t.Context()

	d, err := svc.AddDomain(ctx, graph.AddDomainInput{
		ID:          "cars",
		Description: "vehicles",
		Layers:      []string{"system", "subsystem", "part"},
	})
	require.NoError(t, err)
	require.Equal(t, graph.DomainID("cars"), d.ID)
	require.Equal(t, int64(1), d.Revision)

	got, err := fs.GetDomain(ctx, "cars")
	require.NoError(t, err)
	require.Equal(t, []string{"system", "subsystem", "part"}, got.Layers)
}

func TestAddDomainRejectsInvalidID(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.AddDomain(t.Context(), graph.AddDomainInput{ID: "Cars", Layers: []string{"x"}})
	require.ErrorIs(t, err, graph.ErrInvalidSlug)
}

func TestAddDomainRejectsEmptyLayers(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.AddDomain(t.Context(), graph.AddDomainInput{ID: "cars", Layers: nil})
	require.Error(t, err)
}

func TestAddDomainAlreadyExists(t *testing.T) {
	svc, _ := newService(t)
	in := graph.AddDomainInput{ID: "cars", Layers: []string{"system"}}
	_, err := svc.AddDomain(t.Context(), in)
	require.NoError(t, err)
	_, err = svc.AddDomain(t.Context(), in)
	require.ErrorIs(t, err, graph.ErrDomainAlreadyExists)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/graph/ -run AddDomain -v
```

Expected: FAIL (`graph.NewService` / `graph.AddDomainInput` undefined).

- [ ] **Step 3: Implement Service struct and AddDomain**

Create `internal/graph/service.go`:

```go
package graph

import (
	"context"
	"errors"
	"time"
)

type Clock func() time.Time

type Service struct {
	store Store
	now   Clock
}

func NewService(store Store, now Clock) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, now: now}
}

type AddDomainInput struct {
	ID          string
	Description string
	Layers      []string
}

func (s *Service) AddDomain(ctx context.Context, in AddDomainInput) (*Domain, error) {
	id, err := ParseDomainID(in.ID)
	if err != nil {
		return nil, err
	}
	if len(in.Layers) == 0 {
		return nil, errors.New("layers must not be empty")
	}
	d := Domain{
		ID:          id,
		Description: in.Description,
		Layers:      append([]string(nil), in.Layers...),
		Revision:    1,
		CreatedAt:   s.now(),
	}
	if err := s.store.CreateDomain(ctx, d); err != nil {
		return nil, err
	}
	return &d, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
go test ./internal/graph/ -run AddDomain -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/service.go internal/graph/service_domain_test.go
git commit -m "feat(graph): add Service.AddDomain with validation"
```

---

### Task 9: Service.GetDomain / ListDomains / DeleteDomain

**Files:**
- Modify: `internal/graph/service.go`
- Modify: `internal/graph/service_domain_test.go`

- [ ] **Step 1: Append failing tests for Get/List/Delete**

Append to `internal/graph/service_domain_test.go`:

```go
func TestGetDomainNotFound(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.GetDomain(t.Context(), "missing")
	require.ErrorIs(t, err, graph.ErrDomainNotFound)
}

func TestListDomainsSorted(t *testing.T) {
	svc, _ := newService(t)
	for _, id := range []string{"physics", "cars", "music"} {
		_, err := svc.AddDomain(t.Context(), graph.AddDomainInput{ID: id, Layers: []string{"x"}})
		require.NoError(t, err)
	}
	got, err := svc.ListDomains(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, graph.DomainID("cars"), got[0].ID)
	require.Equal(t, graph.DomainID("music"), got[1].ID)
	require.Equal(t, graph.DomainID("physics"), got[2].ID)
}

func TestDeleteDomain(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.AddDomain(t.Context(), graph.AddDomainInput{ID: "cars", Layers: []string{"x"}})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteDomain(t.Context(), "cars"))

	_, err = svc.GetDomain(t.Context(), "cars")
	require.ErrorIs(t, err, graph.ErrDomainNotFound)

	require.ErrorIs(t, svc.DeleteDomain(t.Context(), "cars"), graph.ErrDomainNotFound)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
go test ./internal/graph/ -run 'GetDomain|ListDomains|DeleteDomain' -v
```

Expected: FAIL.

- [ ] **Step 3: Implement the methods**

Append to `internal/graph/service.go`:

```go
func (s *Service) GetDomain(ctx context.Context, id DomainID) (*Domain, error) {
	return s.store.GetDomain(ctx, id)
}

func (s *Service) ListDomains(ctx context.Context) ([]Domain, error) {
	return s.store.ListDomains(ctx)
}

func (s *Service) DeleteDomain(ctx context.Context, id DomainID) error {
	return s.store.DeleteDomain(ctx, id)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
go test ./internal/graph/ -run 'GetDomain|ListDomains|DeleteDomain' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/service.go internal/graph/service_domain_test.go
git commit -m "feat(graph): add domain Get/List/Delete service methods"
```

---

### Task 10: Service.AddNode — happy path and slug derivation

**Files:**
- Create: `internal/graph/service_node_test.go`
- Modify: `internal/graph/service.go`

- [ ] **Step 1: Add seed helper and failing tests**

Create `internal/graph/service_node_test.go`:

```go
package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func seedCarsDomain(t *testing.T, svc *graph.Service) {
	t.Helper()
	_, err := svc.AddDomain(t.Context(), graph.AddDomainInput{
		ID:     "cars",
		Layers: []string{"system", "subsystem", "part"},
	})
	require.NoError(t, err)
}

func TestAddNodeTopLayerHappyPath(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)

	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars",
		Layer:  "system",
		Name:   "Powertrain",
	})
	require.NoError(t, err)
	require.Equal(t, graph.NodeID("cars:powertrain"), n.ID)
	require.Nil(t, n.ParentID)
	require.Equal(t, int64(1), n.Revision)
}

func TestAddNodeExplicitIDOverridesDerivation(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)

	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "Powertrain", ID: "pt",
	})
	require.NoError(t, err)
	require.Equal(t, graph.NodeID("cars:pt"), n.ID)
}

func TestAddNodeRejectsExplicitInvalidSlug(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "x", ID: "Bad ID"})
	require.ErrorIs(t, err, graph.ErrInvalidSlug)
}

func TestAddNodeDerivedSlugUnderivable(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "!!!"})
	require.ErrorIs(t, err, graph.ErrSlugCannotDerive)
}
```

- [ ] **Step 2: Run tests to verify failure**

```bash
go test ./internal/graph/ -run AddNode -v
```

Expected: FAIL.

- [ ] **Step 3: Implement AddNode (top-layer + derivation only)**

Add `"strings"` to the stdlib import group of `internal/graph/service.go`, then append:

```go
type AddNodeInput struct {
	Domain  string
	Layer   string
	Name    string
	ID      string
	Parent  string
	Summary string
}

func deriveSlug(name string) (SlugID, error) {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	return ParseSlug(s)
}

func (s *Service) AddNode(ctx context.Context, in AddNodeInput) (*Node, error) {
	dID, err := ParseDomainID(in.Domain)
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

	if in.Layer != d.Layers[0] || in.Parent != "" {
		return nil, ErrTopLayerCannotHaveParent
	}

	now := s.now()
	n := Node{
		ID:         NewNodeID(dID, slug),
		Domain:     dID,
		Layer:      in.Layer,
		Name:       in.Name,
		Properties: map[string]any{},
		Revision:   1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.store.CreateNode(ctx, n); err != nil {
		return nil, err
	}
	return &n, nil
}

func slicesContains[T comparable](haystack []T, needle T) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
```

The "non-top-layer or parent provided" branch is a placeholder; Task 11 replaces it with the full rule set.

- [ ] **Step 4: Run tests, verify pass**

```bash
go test ./internal/graph/ -run AddNode -v
```

Expected: PASS for the four new tests.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/service.go internal/graph/service_node_test.go
git commit -m "feat(graph): add AddNode top-layer happy path and slug derivation"
```

---

### Task 11: Service.AddNode — parent and layer rules

**Files:**
- Modify: `internal/graph/service.go`
- Modify: `internal/graph/service_node_test.go`

- [ ] **Step 1: Append failing tests**

Append to `internal/graph/service_node_test.go`:

```go
func TestAddNodeRejectsLayerNotInDomain(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "chassis", Name: "x"})
	require.ErrorIs(t, err, graph.ErrLayerNotInDomain)
}

func TestAddNodeTopLayerWithParentRejected(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "p", Parent: "cars:other"})
	require.ErrorIs(t, err, graph.ErrTopLayerCannotHaveParent)
}

func TestAddNodeNonTopRequiresParent(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine"})
	require.ErrorIs(t, err, graph.ErrParentLayerMismatch)
}

func TestAddNodeParentMustExist(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine", Parent: "cars:missing"})
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}

func TestAddNodeParentDomainMismatch(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddDomain(t.Context(), graph.AddDomainInput{ID: "physics", Layers: []string{"law"}})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "physics", Layer: "law", Name: "thermo"})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine", Parent: "physics:thermo"})
	require.ErrorIs(t, err, graph.ErrParentDomainMismatch)
}

func TestAddNodeParentLayerMismatch(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "part", Name: "piston", Parent: "cars:pt"})
	require.ErrorIs(t, err, graph.ErrParentLayerMismatch)
}

func TestAddNodeNonTopWithValidParent(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"})
	require.NoError(t, err)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine", Parent: "cars:pt"})
	require.NoError(t, err)
	require.NotNil(t, n.ParentID)
	require.Equal(t, graph.NodeID("cars:pt"), *n.ParentID)
}

func TestAddNodeAlreadyExists(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	in := graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"}
	_, err := svc.AddNode(t.Context(), in)
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), in)
	require.ErrorIs(t, err, graph.ErrNodeAlreadyExists)
}
```

- [ ] **Step 2: Run tests, verify failure**

```bash
go test ./internal/graph/ -run AddNode -v
```

Expected: FAIL.

- [ ] **Step 3: Replace AddNode placeholder branch with full rules**

In `internal/graph/service.go`, replace the lines from `if in.Layer != d.Layers[0] || in.Parent != "" {` through the closing `}` of `AddNode` with:

```go
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

	now := s.now()
	n := Node{
		ID:         NewNodeID(dID, slug),
		Domain:     dID,
		Layer:      in.Layer,
		Name:       in.Name,
		ParentID:   parentPtr,
		Summary:    in.Summary,
		Properties: map[string]any{},
		Revision:   1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.store.CreateNode(ctx, n); err != nil {
		return nil, err
	}
	return &n, nil
}

func indexOf[T comparable](xs []T, target T) int {
	for i, x := range xs {
		if x == target {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 4: Run tests, verify pass**

```bash
go test ./internal/graph/ -run AddNode -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/service.go internal/graph/service_node_test.go
git commit -m "feat(graph): enforce parent/layer rules in AddNode"
```

---

### Task 12: Service node Get/List/Children/Update/Delete

**Files:**
- Modify: `internal/graph/service.go`
- Create: `internal/graph/service_node_query_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/graph/service_node_query_test.go`:

```go
package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestGetNodeNotFound(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.GetNode(t.Context(), "cars:missing")
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}

func TestListNodesFilterAndLimit(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	for _, name := range []string{"pt", "chassis", "body"} {
		_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: name})
		require.NoError(t, err)
	}
	all, err := svc.ListNodes(t.Context(), graph.NodeFilter{Domain: "cars"})
	require.NoError(t, err)
	require.Len(t, all, 3)

	limited, err := svc.ListNodes(t.Context(), graph.NodeFilter{Domain: "cars", Layer: "system", Limit: 2})
	require.NoError(t, err)
	require.Len(t, limited, 2)
}

func TestChildrenOf(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine", Parent: "cars:pt"})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "transmission", Parent: "cars:pt"})
	require.NoError(t, err)

	kids, err := svc.ChildrenOf(t.Context(), "cars:pt")
	require.NoError(t, err)
	require.Len(t, kids, 2)
}

func TestUpdateNodeBumpsRevision(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"})
	require.NoError(t, err)

	newSummary := "powertrain summary"
	updated, err := svc.UpdateNode(t.Context(), "cars:pt", graph.UpdateNodeInput{Summary: &newSummary})
	require.NoError(t, err)
	require.Equal(t, "powertrain summary", updated.Summary)
	require.Equal(t, int64(2), updated.Revision)
}

func TestUpdateNodeNotFound(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.UpdateNode(t.Context(), "cars:missing", graph.UpdateNodeInput{})
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}

func TestDeleteNode(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteNode(t.Context(), "cars:pt"))
	_, err = svc.GetNode(t.Context(), "cars:pt")
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}
```

- [ ] **Step 2: Run tests, verify failure**

```bash
go test ./internal/graph/ -run 'GetNode|ListNodes|ChildrenOf|UpdateNode|DeleteNode' -v
```

Expected: FAIL.

- [ ] **Step 3: Implement methods**

Append to `internal/graph/service.go`:

```go
type UpdateNodeInput struct {
	Name    *string
	Summary *string
}

func (s *Service) GetNode(ctx context.Context, id NodeID) (*Node, error) {
	return s.store.GetNode(ctx, id)
}

func (s *Service) ListNodes(ctx context.Context, f NodeFilter) ([]Node, error) {
	return s.store.ListNodes(ctx, f)
}

func (s *Service) ChildrenOf(ctx context.Context, id NodeID) ([]Node, error) {
	return s.store.ChildrenOf(ctx, id)
}

func (s *Service) UpdateNode(ctx context.Context, id NodeID, in UpdateNodeInput) (*Node, error) {
	cur, err := s.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		cur.Name = *in.Name
	}
	if in.Summary != nil {
		cur.Summary = *in.Summary
	}
	cur.UpdatedAt = s.now()
	if err := s.store.UpdateNode(ctx, *cur); err != nil {
		return nil, err
	}
	return s.store.GetNode(ctx, id)
}

func (s *Service) DeleteNode(ctx context.Context, id NodeID) error {
	return s.store.DeleteNode(ctx, id)
}
```

- [ ] **Step 4: Run tests, verify pass**

```bash
go test ./internal/graph/ -run 'GetNode|ListNodes|ChildrenOf|UpdateNode|DeleteNode' -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/service.go internal/graph/service_node_query_test.go
git commit -m "feat(graph): add node query, update, and delete service methods"
```

---

### Task 13: Service edge methods

**Files:**
- Modify: `internal/graph/service.go`
- Create: `internal/graph/service_edge_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/graph/service_edge_test.go`:

```go
package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func seedTwoNodes(t *testing.T, svc *graph.Service) (graph.NodeID, graph.NodeID) {
	t.Helper()
	seedCarsDomain(t, svc)
	a, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"})
	require.NoError(t, err)
	b, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "chassis"})
	require.NoError(t, err)
	return a.ID, b.ID
}

func TestAddEdgeHappyPath(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc)
	e, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "depends_on"})
	require.NoError(t, err)
	require.NotZero(t, e.ID)
}

func TestAddEdgeRejectsSelfLoop(t *testing.T) {
	svc, _ := newService(t)
	a, _ := seedTwoNodes(t, svc)
	_, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(a), Type: "x"})
	require.ErrorIs(t, err, graph.ErrEdgeSelfLoop)
}

func TestAddEdgeMissingEndpoint(t *testing.T) {
	svc, _ := newService(t)
	a, _ := seedTwoNodes(t, svc)
	_, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: "cars:missing", Type: "x"})
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}

func TestAddEdgeDuplicate(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc)
	in := graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "depends_on"}
	_, err := svc.AddEdge(t.Context(), in)
	require.NoError(t, err)
	_, err = svc.AddEdge(t.Context(), in)
	require.ErrorIs(t, err, graph.ErrEdgeAlreadyExists)
}

func TestEdgesFromAndTo(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc)
	_, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "depends_on"})
	require.NoError(t, err)

	out, err := svc.EdgesFrom(t.Context(), a, nil)
	require.NoError(t, err)
	require.Len(t, out, 1)

	in, err := svc.EdgesTo(t.Context(), b, nil)
	require.NoError(t, err)
	require.Len(t, in, 1)
}

func TestDeleteEdge(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc)
	e, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "depends_on"})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteEdge(t.Context(), e.ID))
	require.ErrorIs(t, svc.DeleteEdge(t.Context(), e.ID), graph.ErrEdgeNotFound)
}
```

- [ ] **Step 2: Run tests, verify failure**

```bash
go test ./internal/graph/ -run Edge -v
```

Expected: FAIL.

- [ ] **Step 3: Implement edge methods**

Append to `internal/graph/service.go`:

```go
type AddEdgeInput struct {
	Source string
	Target string
	Type   string
}

func (s *Service) AddEdge(ctx context.Context, in AddEdgeInput) (*Edge, error) {
	src := NodeID(in.Source)
	dst := NodeID(in.Target)
	if src == dst {
		return nil, ErrEdgeSelfLoop
	}
	if _, err := s.store.GetNode(ctx, src); err != nil {
		return nil, err
	}
	if _, err := s.store.GetNode(ctx, dst); err != nil {
		return nil, err
	}
	e := &Edge{
		SourceID:   src,
		TargetID:   dst,
		Type:       in.Type,
		Properties: map[string]any{},
		Revision:   1,
		CreatedAt:  s.now(),
	}
	if err := s.store.CreateEdge(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Service) DeleteEdge(ctx context.Context, id EdgeID) error {
	return s.store.DeleteEdge(ctx, id)
}

func (s *Service) EdgesFrom(ctx context.Context, src NodeID, types []string) ([]Edge, error) {
	return s.store.EdgesFrom(ctx, src, types)
}

func (s *Service) EdgesTo(ctx context.Context, dst NodeID, types []string) ([]Edge, error) {
	return s.store.EdgesTo(ctx, dst, types)
}
```

- [ ] **Step 4: Verify the whole graph package is green**

```bash
go test ./internal/graph/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/service.go internal/graph/service_edge_test.go
git commit -m "feat(graph): add edge service methods with validation"
```

---

## Phase 4 — SQLite Store

### Task 14: Store.Open with WAL and embedded migrations

**Files:**
- Modify: `internal/store/store.go`
- Create: `internal/store/store_test.go`
- Delete: `internal/store/migration_test.go` (superseded)

- [ ] **Step 1: Write failing test for Open**

Create `internal/store/store_test.go`:

```go
package store_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/store"
)

func openTestDB(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenAppliesMigrations(t *testing.T) {
	s := openTestDB(t)
	require.NotNil(t, s.DB())
	var got string
	require.NoError(t, s.DB().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='domains'`).Scan(&got))
	require.Equal(t, "domains", got)
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/store/ -run TestOpenAppliesMigrations -v
```

Expected: FAIL.

- [ ] **Step 3: Replace `internal/store/store.go` with full Store**

Overwrite `internal/store/store.go`:

```go
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed ../../migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	if err := runMigrations(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) DB() *sql.DB  { return s.db }

func runMigrations(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose: set dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "../../migrations"); err != nil {
		return fmt.Errorf("goose: up: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Delete the redundant migration smoke test**

```bash
git rm internal/store/migration_test.go
```

- [ ] **Step 5: Run, verify pass**

```bash
go test ./internal/store/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat(store): add Store.Open with WAL, FK pragma, and embedded migrations"
```

---

### Task 15: InTx with context-carried transaction

**Files:**
- Create: `internal/store/tx.go`
- Create: `internal/store/tx_test.go`

- [ ] **Step 1: Write failing tests (uses temporary `ExecForTest` helper since CRUD lands in Task 16)**

Create `internal/store/tx_test.go`:

```go
package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/store"
)

func TestInTxCommit(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	err := s.InTx(ctx, func(ctx context.Context) error {
		_, err := store.ExecForTest(ctx, s, `INSERT INTO domains(id, layers, created_at) VALUES ('x','["a"]',1)`)
		return err
	})
	require.NoError(t, err)

	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM domains`).Scan(&n))
	require.Equal(t, 1, n)
}

func TestInTxRollback(t *testing.T) {
	s := openTestDB(t)
	wantErr := errors.New("boom")
	err := s.InTx(t.Context(), func(ctx context.Context) error {
		_, _ = store.ExecForTest(ctx, s, `INSERT INTO domains(id, layers, created_at) VALUES ('x','["a"]',1)`)
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)

	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM domains`).Scan(&n))
	require.Equal(t, 0, n)
}

func TestInTxNested(t *testing.T) {
	s := openTestDB(t)
	err := s.InTx(t.Context(), func(ctx context.Context) error {
		return s.InTx(ctx, func(ctx context.Context) error { return nil })
	})
	require.ErrorIs(t, err, graph.ErrNestedTransaction)
}

func TestInTxPanicRollsBack(t *testing.T) {
	s := openTestDB(t)
	require.Panics(t, func() {
		_ = s.InTx(t.Context(), func(ctx context.Context) error {
			_, _ = store.ExecForTest(ctx, s, `INSERT INTO domains(id, layers, created_at) VALUES ('p','["a"]',1)`)
			panic("nope")
		})
	})

	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM domains`).Scan(&n))
	require.Equal(t, 0, n)
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/store/ -run InTx -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `internal/store/tx.go`**

Create `internal/store/tx.go`:

```go
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ggfarmco/kg/internal/graph"
)

type txKey struct{}

func (s *Store) InTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return graph.ErrNestedTransaction
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	ctxWithTx := context.WithValue(ctx, txKey{}, tx)

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		if cerr := tx.Commit(); cerr != nil {
			err = fmt.Errorf("commit: %w", cerr)
		}
	}()

	return fn(ctxWithTx)
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Store) conn(ctx context.Context) execer {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx
	}
	return s.db
}

func ExecForTest(ctx context.Context, s *Store, query string, args ...any) (sql.Result, error) {
	return s.conn(ctx).ExecContext(ctx, query, args...)
}
```

(`ExecForTest` is temporary scaffolding — removed in Task 16 once real CRUD covers the same surface.)

- [ ] **Step 4: Run, verify pass**

```bash
go test ./internal/store/ -run InTx -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/tx.go internal/store/tx_test.go
git commit -m "feat(store): add context-carried InTx with rollback and panic recovery"
```

---

### Task 16: sqlc-generated domain CRUD

**Files:**
- Create: `internal/store/queries.sql`
- Generated by sqlc: `internal/store/db.go`, `internal/store/models.go`, `internal/store/queries.sql.go`
- Create: `internal/store/domains.go`
- Create: `internal/store/domains_test.go`
- Modify: `internal/store/tx.go`, `internal/store/tx_test.go` (remove `ExecForTest`)

- [ ] **Step 1: Write `queries.sql`**

Create `internal/store/queries.sql`:

```sql
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
INSERT INTO changes(entity, entity_id, op, revision, at) VALUES (?, ?, ?, ?, ?);
```

- [ ] **Step 2: Generate sqlc code**

```bash
make gen
```

Expected: `internal/store/db.go`, `models.go`, `queries.sql.go` appear. Inspect briefly to learn the generated row type names (`GetDomainRow`, `ListDomainsRow`, etc.).

- [ ] **Step 3: Write failing tests**

Create `internal/store/domains_test.go`:

```go
package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestDomainCRUD(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	d := graph.Domain{
		ID: "cars", Description: "vehicles",
		Layers:    []string{"system", "subsystem"},
		CreatedAt: time.UnixMilli(1700000000000),
	}
	require.NoError(t, s.CreateDomain(ctx, d))

	got, err := s.GetDomain(ctx, "cars")
	require.NoError(t, err)
	require.Equal(t, d.ID, got.ID)
	require.Equal(t, d.Layers, got.Layers)
	require.Equal(t, int64(1), got.Revision)

	list, err := s.ListDomains(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, s.DeleteDomain(ctx, "cars"))
	_, err = s.GetDomain(ctx, "cars")
	require.ErrorIs(t, err, graph.ErrDomainNotFound)
}

func TestCreateDomainDuplicate(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	d := graph.Domain{ID: "cars", Layers: []string{"x"}, CreatedAt: time.UnixMilli(1)}
	require.NoError(t, s.CreateDomain(ctx, d))
	require.ErrorIs(t, s.CreateDomain(ctx, d), graph.ErrDomainAlreadyExists)
}

func TestDomainChangesLogged(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	d := graph.Domain{ID: "cars", Layers: []string{"x"}, CreatedAt: time.UnixMilli(1)}
	require.NoError(t, s.CreateDomain(ctx, d))
	require.NoError(t, s.DeleteDomain(ctx, "cars"))

	rows, err := s.DB().Query(`SELECT entity, op, revision FROM changes ORDER BY seq`)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rows.Close() })

	type ch struct {
		entity, op string
		rev        *int64
	}
	var out []ch
	for rows.Next() {
		var c ch
		require.NoError(t, rows.Scan(&c.entity, &c.op, &c.rev))
		out = append(out, c)
	}
	require.Len(t, out, 2)
	require.Equal(t, "domain", out[0].entity)
	require.Equal(t, "create", out[0].op)
	require.Equal(t, "delete", out[1].op)
	require.Nil(t, out[1].rev)
}
```

- [ ] **Step 4: Run, verify failure**

```bash
go test ./internal/store/ -run Domain -v
```

Expected: FAIL.

- [ ] **Step 5: Implement `internal/store/domains.go`**

Create `internal/store/domains.go`:

```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/ggfarmco/kg/internal/graph"
)

func (s *Store) CreateDomain(ctx context.Context, d graph.Domain) error {
	return s.InTx(ctx, func(ctx context.Context) error {
		layers, err := json.Marshal(d.Layers)
		if err != nil {
			return fmt.Errorf("marshal layers: %w", err)
		}
		q := New(s.conn(ctx))
		if err := q.CreateDomain(ctx, CreateDomainParams{
			ID:          string(d.ID),
			Description: nullString(d.Description),
			Layers:      string(layers),
			CreatedAt:   d.CreatedAt.UnixMilli(),
		}); err != nil {
			return mapSQLiteErr(err, "domain")
		}
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "domain",
			EntityID: string(d.ID),
			Op:       "create",
			Revision: sql.NullInt64{Int64: 1, Valid: true},
			At:       d.CreatedAt.UnixMilli(),
		})
	})
}

func (s *Store) GetDomain(ctx context.Context, id graph.DomainID) (*graph.Domain, error) {
	q := New(s.conn(ctx))
	row, err := q.GetDomain(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, graph.ErrDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get domain: %w", err)
	}
	return decodeDomain(row.ID, row.Description, row.Layers, row.Revision, row.CreatedAt)
}

func (s *Store) ListDomains(ctx context.Context) ([]graph.Domain, error) {
	q := New(s.conn(ctx))
	rows, err := q.ListDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list domains: %w", err)
	}
	out := make([]graph.Domain, 0, len(rows))
	for _, r := range rows {
		d, err := decodeDomain(r.ID, r.Description, r.Layers, r.Revision, r.CreatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, nil
}

func (s *Store) DeleteDomain(ctx context.Context, id graph.DomainID) error {
	return s.InTx(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		if _, err := q.GetDomain(ctx, string(id)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return graph.ErrDomainNotFound
			}
			return fmt.Errorf("sqlite: get domain: %w", err)
		}
		if err := q.DeleteDomain(ctx, string(id)); err != nil {
			return mapSQLiteErr(err, "domain")
		}
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "domain",
			EntityID: string(id),
			Op:       "delete",
			Revision: sql.NullInt64{},
			At:       time.Now().UnixMilli(),
		})
	})
}

func decodeDomain(id string, desc sql.NullString, layersJSON string, rev, createdAt int64) (*graph.Domain, error) {
	var layers []string
	if err := json.Unmarshal([]byte(layersJSON), &layers); err != nil {
		return nil, fmt.Errorf("unmarshal layers: %w", err)
	}
	return &graph.Domain{
		ID:          graph.DomainID(id),
		Description: desc.String,
		Layers:      layers,
		Revision:    rev,
		CreatedAt:   time.UnixMilli(createdAt),
	}, nil
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func mapSQLiteErr(err error, entity string) error {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return err
	}
	if se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE || se.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
		switch entity {
		case "domain":
			return graph.ErrDomainAlreadyExists
		case "node":
			return graph.ErrNodeAlreadyExists
		case "edge":
			return graph.ErrEdgeAlreadyExists
		}
	}
	return err
}
```

(`decodeDomain` accepts the columns positionally so both `GetDomainRow` and `ListDomainsRow` map cleanly even if sqlc emits separate types.)

- [ ] **Step 6: Remove `ExecForTest` and rewrite tx tests to use real CRUD**

Delete the `ExecForTest` function from `internal/store/tx.go`.

Rewrite `internal/store/tx_test.go` to use `s.CreateDomain` / `s.GetDomain`:

```go
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestInTxCommit(t *testing.T) {
	s := openTestDB(t)
	err := s.InTx(t.Context(), func(ctx context.Context) error {
		return s.CreateDomain(ctx, graph.Domain{ID: "x", Layers: []string{"a"}, CreatedAt: time.UnixMilli(1)})
	})
	require.NoError(t, err)
	_, err = s.GetDomain(t.Context(), "x")
	require.NoError(t, err)
}

func TestInTxRollback(t *testing.T) {
	s := openTestDB(t)
	wantErr := errors.New("boom")
	err := s.InTx(t.Context(), func(ctx context.Context) error {
		_ = s.CreateDomain(ctx, graph.Domain{ID: "x", Layers: []string{"a"}, CreatedAt: time.UnixMilli(1)})
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)
	_, err = s.GetDomain(t.Context(), "x")
	require.ErrorIs(t, err, graph.ErrDomainNotFound)
}

func TestInTxNested(t *testing.T) {
	s := openTestDB(t)
	err := s.InTx(t.Context(), func(ctx context.Context) error {
		return s.InTx(ctx, func(ctx context.Context) error { return nil })
	})
	require.ErrorIs(t, err, graph.ErrNestedTransaction)
}

func TestInTxPanicRollsBack(t *testing.T) {
	s := openTestDB(t)
	require.Panics(t, func() {
		_ = s.InTx(t.Context(), func(ctx context.Context) error {
			_ = s.CreateDomain(ctx, graph.Domain{ID: "p", Layers: []string{"a"}, CreatedAt: time.UnixMilli(1)})
			panic("nope")
		})
	})
	_, err := s.GetDomain(t.Context(), "p")
	require.ErrorIs(t, err, graph.ErrDomainNotFound)
}
```

- [ ] **Step 7: Run, verify pass**

```bash
go test ./internal/store/ -v
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/store/
git commit -m "feat(store): add domain CRUD with sqlc and changes-log append"
```

---

### Task 17: SQLite node CRUD with revision bump

**Files:**
- Modify: `internal/store/queries.sql`
- Regenerate: sqlc files
- Create: `internal/store/nodes.go`
- Create: `internal/store/nodes_test.go`

- [ ] **Step 1: Append node queries**

Append to `internal/store/queries.sql`:

```sql
-- name: CreateNode :exec
INSERT INTO nodes(id, domain, layer, name, parent_id, summary, properties, revision, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?);

-- name: GetNode :one
SELECT id, domain, layer, name, parent_id, summary, properties, revision, created_at, updated_at
FROM nodes WHERE id = ?;

-- name: ListNodes :many
SELECT id, domain, layer, name, parent_id, summary, properties, revision, created_at, updated_at
FROM nodes
WHERE (sqlc.arg(domain_filter) = '' OR domain = sqlc.arg(domain_filter))
  AND (sqlc.arg(layer_filter)  = '' OR layer  = sqlc.arg(layer_filter))
ORDER BY id
LIMIT CASE WHEN sqlc.arg(lim) = 0 THEN -1 ELSE sqlc.arg(lim) END;

-- name: ChildrenOf :many
SELECT id, domain, layer, name, parent_id, summary, properties, revision, created_at, updated_at
FROM nodes WHERE parent_id = ? ORDER BY id;

-- name: UpdateNode :exec
UPDATE nodes SET name = ?, summary = ?, properties = ?, revision = revision + 1, updated_at = ?
WHERE id = ?;

-- name: GetNodeRevision :one
SELECT revision FROM nodes WHERE id = ?;

-- name: DeleteNode :exec
DELETE FROM nodes WHERE id = ?;
```

- [ ] **Step 2: Regenerate sqlc**

```bash
make gen
```

- [ ] **Step 3: Write failing tests**

Create `internal/store/nodes_test.go`:

```go
package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/store"
)

func seedDomain(t *testing.T, s *store.Store) {
	t.Helper()
	require.NoError(t, s.CreateDomain(t.Context(), graph.Domain{
		ID: "cars", Layers: []string{"system", "subsystem", "part"}, CreatedAt: time.UnixMilli(1),
	}))
}

func TestNodeCRUDAndRevision(t *testing.T) {
	s := openTestDB(t)
	seedDomain(t, s)
	ctx := t.Context()

	require.NoError(t, s.CreateNode(ctx, graph.Node{
		ID: "cars:pt", Domain: "cars", Layer: "system", Name: "Powertrain",
		Properties: map[string]any{}, CreatedAt: time.UnixMilli(1), UpdatedAt: time.UnixMilli(1),
	}))

	got, err := s.GetNode(ctx, "cars:pt")
	require.NoError(t, err)
	require.Equal(t, "Powertrain", got.Name)
	require.Equal(t, int64(1), got.Revision)

	got.Name = "Drive"
	got.UpdatedAt = time.UnixMilli(2)
	require.NoError(t, s.UpdateNode(ctx, *got))

	after, err := s.GetNode(ctx, "cars:pt")
	require.NoError(t, err)
	require.Equal(t, "Drive", after.Name)
	require.Equal(t, int64(2), after.Revision)

	list, err := s.ListNodes(ctx, graph.NodeFilter{Domain: "cars"})
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, s.DeleteNode(ctx, "cars:pt"))
	_, err = s.GetNode(ctx, "cars:pt")
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}

func TestNodeChangesLog(t *testing.T) {
	s := openTestDB(t)
	seedDomain(t, s)
	ctx := t.Context()

	n := graph.Node{ID: "cars:pt", Domain: "cars", Layer: "system", Name: "PT", Properties: map[string]any{}, CreatedAt: time.UnixMilli(1), UpdatedAt: time.UnixMilli(1)}
	require.NoError(t, s.CreateNode(ctx, n))
	require.NoError(t, s.UpdateNode(ctx, n))
	require.NoError(t, s.DeleteNode(ctx, "cars:pt"))

	rows, err := s.DB().Query(`SELECT op, revision FROM changes WHERE entity='node' ORDER BY seq`)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rows.Close() })

	var ops []string
	var revs []*int64
	for rows.Next() {
		var op string
		var rev *int64
		require.NoError(t, rows.Scan(&op, &rev))
		ops = append(ops, op)
		revs = append(revs, rev)
	}
	require.Equal(t, []string{"create", "update", "delete"}, ops)
	require.NotNil(t, revs[0])
	require.Equal(t, int64(1), *revs[0])
	require.NotNil(t, revs[1])
	require.Equal(t, int64(2), *revs[1])
	require.Nil(t, revs[2])
}

func TestChildrenOf(t *testing.T) {
	s := openTestDB(t)
	seedDomain(t, s)
	ctx := t.Context()

	require.NoError(t, s.CreateNode(ctx, graph.Node{ID: "cars:pt", Domain: "cars", Layer: "system", Name: "PT", Properties: map[string]any{}, CreatedAt: time.UnixMilli(1), UpdatedAt: time.UnixMilli(1)}))
	pt := graph.NodeID("cars:pt")
	require.NoError(t, s.CreateNode(ctx, graph.Node{ID: "cars:engine", Domain: "cars", Layer: "subsystem", Name: "Engine", ParentID: &pt, Properties: map[string]any{}, CreatedAt: time.UnixMilli(2), UpdatedAt: time.UnixMilli(2)}))

	kids, err := s.ChildrenOf(ctx, pt)
	require.NoError(t, err)
	require.Len(t, kids, 1)
	require.Equal(t, graph.NodeID("cars:engine"), kids[0].ID)
}
```

- [ ] **Step 4: Run, verify failure**

```bash
go test ./internal/store/ -run Node -v
```

Expected: FAIL.

- [ ] **Step 5: Implement `internal/store/nodes.go`**

Create `internal/store/nodes.go`:

```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ggfarmco/kg/internal/graph"
)

func (s *Store) CreateNode(ctx context.Context, n graph.Node) error {
	props, err := json.Marshal(n.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}
	return s.InTx(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		if err := q.CreateNode(ctx, CreateNodeParams{
			ID:         string(n.ID),
			Domain:     string(n.Domain),
			Layer:      n.Layer,
			Name:       n.Name,
			ParentID:   nodeIDPtrToNullString(n.ParentID),
			Summary:    nullString(n.Summary),
			Properties: string(props),
			CreatedAt:  n.CreatedAt.UnixMilli(),
			UpdatedAt:  n.UpdatedAt.UnixMilli(),
		}); err != nil {
			return mapSQLiteErr(err, "node")
		}
		return q.AppendChange(ctx, AppendChangeParams{
			Entity: "node", EntityID: string(n.ID), Op: "create",
			Revision: sql.NullInt64{Int64: 1, Valid: true}, At: n.CreatedAt.UnixMilli(),
		})
	})
}

func (s *Store) GetNode(ctx context.Context, id graph.NodeID) (*graph.Node, error) {
	q := New(s.conn(ctx))
	row, err := q.GetNode(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, graph.ErrNodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get node: %w", err)
	}
	return decodeNode(row.ID, row.Domain, row.Layer, row.Name, row.ParentID, row.Summary, row.Properties, row.Revision, row.CreatedAt, row.UpdatedAt)
}

func (s *Store) ListNodes(ctx context.Context, f graph.NodeFilter) ([]graph.Node, error) {
	q := New(s.conn(ctx))
	rows, err := q.ListNodes(ctx, ListNodesParams{
		DomainFilter: string(f.Domain),
		LayerFilter:  f.Layer,
		Lim:          int64(f.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: list nodes: %w", err)
	}
	out := make([]graph.Node, 0, len(rows))
	for _, r := range rows {
		n, err := decodeNode(r.ID, r.Domain, r.Layer, r.Name, r.ParentID, r.Summary, r.Properties, r.Revision, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, nil
}

func (s *Store) ChildrenOf(ctx context.Context, parentID graph.NodeID) ([]graph.Node, error) {
	q := New(s.conn(ctx))
	rows, err := q.ChildrenOf(ctx, sql.NullString{String: string(parentID), Valid: true})
	if err != nil {
		return nil, fmt.Errorf("sqlite: children of: %w", err)
	}
	out := make([]graph.Node, 0, len(rows))
	for _, r := range rows {
		n, err := decodeNode(r.ID, r.Domain, r.Layer, r.Name, r.ParentID, r.Summary, r.Properties, r.Revision, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, nil
}

func (s *Store) UpdateNode(ctx context.Context, n graph.Node) error {
	props, err := json.Marshal(n.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}
	return s.InTx(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		if _, err := q.GetNode(ctx, string(n.ID)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return graph.ErrNodeNotFound
			}
			return fmt.Errorf("sqlite: get node: %w", err)
		}
		if err := q.UpdateNode(ctx, UpdateNodeParams{
			ID:         string(n.ID),
			Name:       n.Name,
			Summary:    nullString(n.Summary),
			Properties: string(props),
			UpdatedAt:  n.UpdatedAt.UnixMilli(),
		}); err != nil {
			return mapSQLiteErr(err, "node")
		}
		rev, err := q.GetNodeRevision(ctx, string(n.ID))
		if err != nil {
			return fmt.Errorf("sqlite: get node revision: %w", err)
		}
		return q.AppendChange(ctx, AppendChangeParams{
			Entity: "node", EntityID: string(n.ID), Op: "update",
			Revision: sql.NullInt64{Int64: rev, Valid: true}, At: n.UpdatedAt.UnixMilli(),
		})
	})
}

func (s *Store) DeleteNode(ctx context.Context, id graph.NodeID) error {
	return s.InTx(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		if _, err := q.GetNode(ctx, string(id)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return graph.ErrNodeNotFound
			}
			return fmt.Errorf("sqlite: get node: %w", err)
		}
		if err := q.DeleteNode(ctx, string(id)); err != nil {
			return mapSQLiteErr(err, "node")
		}
		return q.AppendChange(ctx, AppendChangeParams{
			Entity: "node", EntityID: string(id), Op: "delete",
			Revision: sql.NullInt64{}, At: time.Now().UnixMilli(),
		})
	})
}

func decodeNode(id, domain, layer, name string, parent, summary sql.NullString, propsJSON string, rev, createdAt, updatedAt int64) (*graph.Node, error) {
	var props map[string]any
	if err := json.Unmarshal([]byte(propsJSON), &props); err != nil {
		return nil, fmt.Errorf("unmarshal properties: %w", err)
	}
	var parentPtr *graph.NodeID
	if parent.Valid {
		p := graph.NodeID(parent.String)
		parentPtr = &p
	}
	return &graph.Node{
		ID:         graph.NodeID(id),
		Domain:     graph.DomainID(domain),
		Layer:      layer,
		Name:       name,
		ParentID:   parentPtr,
		Summary:    summary.String,
		Properties: props,
		Revision:   rev,
		CreatedAt:  time.UnixMilli(createdAt),
		UpdatedAt:  time.UnixMilli(updatedAt),
	}, nil
}

func nodeIDPtrToNullString(p *graph.NodeID) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(*p), Valid: true}
}
```

- [ ] **Step 6: Run, verify pass**

```bash
go test ./internal/store/ -run Node -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/store/
git commit -m "feat(store): add node CRUD with revision bump and changes log"
```

---

### Task 18: SQLite edge CRUD

**Files:**
- Modify: `internal/store/queries.sql`
- Regenerate: sqlc files
- Create: `internal/store/edges.go`
- Create: `internal/store/edges_test.go`

- [ ] **Step 1: Append edge queries**

Append to `internal/store/queries.sql`:

```sql
-- name: CreateEdge :one
INSERT INTO edges(source_id, target_id, type, properties, revision, created_at)
VALUES (?, ?, ?, ?, 1, ?) RETURNING id;

-- name: GetEdge :one
SELECT id, source_id, target_id, type, properties, revision, created_at FROM edges WHERE id = ?;

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
```

- [ ] **Step 2: Regenerate**

```bash
make gen
```

- [ ] **Step 3: Write failing tests**

Create `internal/store/edges_test.go`:

```go
package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/store"
)

func seedTwoNodes(t *testing.T, s *store.Store) (graph.NodeID, graph.NodeID) {
	t.Helper()
	now := time.UnixMilli(1)
	require.NoError(t, s.CreateNode(t.Context(), graph.Node{ID: "cars:a", Domain: "cars", Layer: "system", Name: "A", Properties: map[string]any{}, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, s.CreateNode(t.Context(), graph.Node{ID: "cars:b", Domain: "cars", Layer: "system", Name: "B", Properties: map[string]any{}, CreatedAt: now, UpdatedAt: now}))
	return "cars:a", "cars:b"
}

func TestEdgeCRUD(t *testing.T) {
	s := openTestDB(t)
	seedDomain(t, s)
	a, b := seedTwoNodes(t, s)
	ctx := t.Context()

	e := &graph.Edge{SourceID: a, TargetID: b, Type: "depends_on", Properties: map[string]any{}, CreatedAt: time.UnixMilli(1)}
	require.NoError(t, s.CreateEdge(ctx, e))
	require.NotZero(t, e.ID)

	got, err := s.GetEdge(ctx, e.ID)
	require.NoError(t, err)
	require.Equal(t, "depends_on", got.Type)

	from, err := s.EdgesFrom(ctx, a, nil)
	require.NoError(t, err)
	require.Len(t, from, 1)

	to, err := s.EdgesTo(ctx, b, []string{"depends_on"})
	require.NoError(t, err)
	require.Len(t, to, 1)

	require.NoError(t, s.DeleteEdge(ctx, e.ID))
	_, err = s.GetEdge(ctx, e.ID)
	require.ErrorIs(t, err, graph.ErrEdgeNotFound)
}

func TestEdgeUniqueViolation(t *testing.T) {
	s := openTestDB(t)
	seedDomain(t, s)
	a, b := seedTwoNodes(t, s)
	ctx := t.Context()
	e := &graph.Edge{SourceID: a, TargetID: b, Type: "x", Properties: map[string]any{}, CreatedAt: time.UnixMilli(1)}
	require.NoError(t, s.CreateEdge(ctx, e))
	dup := &graph.Edge{SourceID: a, TargetID: b, Type: "x", Properties: map[string]any{}, CreatedAt: time.UnixMilli(2)}
	require.ErrorIs(t, s.CreateEdge(ctx, dup), graph.ErrEdgeAlreadyExists)
}

func TestEdgeCascadeOnNodeDelete(t *testing.T) {
	s := openTestDB(t)
	seedDomain(t, s)
	a, b := seedTwoNodes(t, s)
	ctx := t.Context()
	e := &graph.Edge{SourceID: a, TargetID: b, Type: "x", Properties: map[string]any{}, CreatedAt: time.UnixMilli(1)}
	require.NoError(t, s.CreateEdge(ctx, e))

	require.NoError(t, s.DeleteNode(ctx, b))
	_, err := s.GetEdge(ctx, e.ID)
	require.ErrorIs(t, err, graph.ErrEdgeNotFound)
}
```

- [ ] **Step 4: Run, verify failure**

```bash
go test ./internal/store/ -run Edge -v
```

Expected: FAIL.

- [ ] **Step 5: Implement `internal/store/edges.go`**

Create `internal/store/edges.go`:

```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ggfarmco/kg/internal/graph"
)

func (s *Store) CreateEdge(ctx context.Context, e *graph.Edge) error {
	props, err := json.Marshal(e.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}
	return s.InTx(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		id, err := q.CreateEdge(ctx, CreateEdgeParams{
			SourceID:   string(e.SourceID),
			TargetID:   string(e.TargetID),
			Type:       e.Type,
			Properties: string(props),
			CreatedAt:  e.CreatedAt.UnixMilli(),
		})
		if err != nil {
			return mapSQLiteErr(err, "edge")
		}
		e.ID = graph.EdgeID(id)
		e.Revision = 1
		return q.AppendChange(ctx, AppendChangeParams{
			Entity: "edge", EntityID: fmt.Sprintf("%d", id), Op: "create",
			Revision: sql.NullInt64{Int64: 1, Valid: true}, At: e.CreatedAt.UnixMilli(),
		})
	})
}

func (s *Store) GetEdge(ctx context.Context, id graph.EdgeID) (*graph.Edge, error) {
	q := New(s.conn(ctx))
	row, err := q.GetEdge(ctx, int64(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, graph.ErrEdgeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get edge: %w", err)
	}
	return decodeEdge(row.ID, row.SourceID, row.TargetID, row.Type, row.Properties, row.Revision, row.CreatedAt)
}

func (s *Store) DeleteEdge(ctx context.Context, id graph.EdgeID) error {
	return s.InTx(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		if _, err := q.GetEdge(ctx, int64(id)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return graph.ErrEdgeNotFound
			}
			return fmt.Errorf("sqlite: get edge: %w", err)
		}
		if err := q.DeleteEdge(ctx, int64(id)); err != nil {
			return mapSQLiteErr(err, "edge")
		}
		return q.AppendChange(ctx, AppendChangeParams{
			Entity: "edge", EntityID: fmt.Sprintf("%d", id), Op: "delete",
			Revision: sql.NullInt64{}, At: time.Now().UnixMilli(),
		})
	})
}

func (s *Store) EdgesFrom(ctx context.Context, src graph.NodeID, types []string) ([]graph.Edge, error) {
	q := New(s.conn(ctx))
	if len(types) == 0 {
		rows, err := q.EdgesFromAll(ctx, string(src))
		if err != nil {
			return nil, fmt.Errorf("sqlite: edges from: %w", err)
		}
		return decodeEdges(rows, edgeRowFields)
	}
	rows, err := q.EdgesFromTyped(ctx, EdgesFromTypedParams{SourceID: string(src), Types: types})
	if err != nil {
		return nil, fmt.Errorf("sqlite: edges from typed: %w", err)
	}
	return decodeEdges(rows, edgeRowFields)
}

func (s *Store) EdgesTo(ctx context.Context, dst graph.NodeID, types []string) ([]graph.Edge, error) {
	q := New(s.conn(ctx))
	if len(types) == 0 {
		rows, err := q.EdgesToAll(ctx, string(dst))
		if err != nil {
			return nil, fmt.Errorf("sqlite: edges to: %w", err)
		}
		return decodeEdges(rows, edgeRowFields)
	}
	rows, err := q.EdgesToTyped(ctx, EdgesToTypedParams{TargetID: string(dst), Types: types})
	if err != nil {
		return nil, fmt.Errorf("sqlite: edges to typed: %w", err)
	}
	return decodeEdges(rows, edgeRowFields)
}

type edgeFields struct {
	id, rev, createdAt int64
	src, dst, typ, ps  string
}

func edgeRowFields[R any](r R) edgeFields {
	switch v := any(r).(type) {
	case EdgesFromAllRow:
		return edgeFields{v.ID, v.Revision, v.CreatedAt, v.SourceID, v.TargetID, v.Type, v.Properties}
	case EdgesFromTypedRow:
		return edgeFields{v.ID, v.Revision, v.CreatedAt, v.SourceID, v.TargetID, v.Type, v.Properties}
	case EdgesToAllRow:
		return edgeFields{v.ID, v.Revision, v.CreatedAt, v.SourceID, v.TargetID, v.Type, v.Properties}
	case EdgesToTypedRow:
		return edgeFields{v.ID, v.Revision, v.CreatedAt, v.SourceID, v.TargetID, v.Type, v.Properties}
	}
	panic("unreachable")
}

func decodeEdges[R any](rows []R, extract func(R) edgeFields) ([]graph.Edge, error) {
	out := make([]graph.Edge, 0, len(rows))
	for _, r := range rows {
		f := extract(r)
		e, err := decodeEdge(f.id, f.src, f.dst, f.typ, f.ps, f.rev, f.createdAt)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, nil
}

func decodeEdge(id int64, src, dst, typ, propsJSON string, rev, createdAt int64) (*graph.Edge, error) {
	var props map[string]any
	if err := json.Unmarshal([]byte(propsJSON), &props); err != nil {
		return nil, fmt.Errorf("unmarshal properties: %w", err)
	}
	return &graph.Edge{
		ID:         graph.EdgeID(id),
		SourceID:   graph.NodeID(src),
		TargetID:   graph.NodeID(dst),
		Type:       typ,
		Properties: props,
		Revision:   rev,
		CreatedAt:  time.UnixMilli(createdAt),
	}, nil
}
```

(The type-switch in `edgeRowFields` keeps row-shape divergence between sqlc-generated types localized to a single helper, regardless of whether sqlc emits four distinct row types or shares one. If sqlc shares a single row type across all four queries, simplify by deleting the unused cases.)

- [ ] **Step 6: Run, verify pass**

```bash
go test ./internal/store/ -run Edge -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/store/
git commit -m "feat(store): add edge CRUD with cascade-on-delete"
```

---

### Task 19: Pinning changes-log invariants

**Files:**
- Create: `internal/store/changes_test.go`

- [ ] **Step 1: Write the invariant tests**

Create `internal/store/changes_test.go`:

```go
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestChangesSeqMonotonicAcrossMutationsAndDeletes(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	require.NoError(t, s.CreateDomain(ctx, graph.Domain{ID: "cars", Layers: []string{"system"}, CreatedAt: time.UnixMilli(1)}))
	require.NoError(t, s.CreateNode(ctx, graph.Node{ID: "cars:a", Domain: "cars", Layer: "system", Name: "A", Properties: map[string]any{}, CreatedAt: time.UnixMilli(2), UpdatedAt: time.UnixMilli(2)}))
	require.NoError(t, s.DeleteNode(ctx, "cars:a"))
	require.NoError(t, s.CreateNode(ctx, graph.Node{ID: "cars:b", Domain: "cars", Layer: "system", Name: "B", Properties: map[string]any{}, CreatedAt: time.UnixMilli(3), UpdatedAt: time.UnixMilli(3)}))

	rows, err := s.DB().Query(`SELECT seq FROM changes ORDER BY seq`)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rows.Close() })

	var seqs []int64
	for rows.Next() {
		var seq int64
		require.NoError(t, rows.Scan(&seq))
		seqs = append(seqs, seq)
	}
	require.Len(t, seqs, 4)
	for i := 1; i < len(seqs); i++ {
		require.Greater(t, seqs[i], seqs[i-1], "seq must be strictly increasing")
	}
	require.Equal(t, int64(1), seqs[0])
	require.Equal(t, int64(4), seqs[3])
}

func TestChangesRolledBackWhenTxFails(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	require.NoError(t, s.CreateDomain(ctx, graph.Domain{ID: "cars", Layers: []string{"system"}, CreatedAt: time.UnixMilli(1)}))

	wantErr := errors.New("boom")
	_ = s.InTx(ctx, func(ctx context.Context) error {
		_ = s.CreateNode(ctx, graph.Node{ID: "cars:a", Domain: "cars", Layer: "system", Name: "A", Properties: map[string]any{}, CreatedAt: time.UnixMilli(2), UpdatedAt: time.UnixMilli(2)})
		return wantErr
	})

	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM changes WHERE entity='node'`).Scan(&n))
	require.Equal(t, 0, n)
}
```

- [ ] **Step 2: Run, verify pass**

```bash
go test ./internal/store/ -run Changes -v
```

Expected: PASS (invariants already satisfied by Tasks 16–18; these tests pin them).

- [ ] **Step 3: Commit**

```bash
git add internal/store/changes_test.go
git commit -m "test(store): pin changes log monotonic seq and rollback invariants"
```

---

## Phase 5 — CLI (cobra)

### Task 20: Cobra root + JSON envelope + error map

**Files:**
- Delete: `cmd/kg/.keep`
- Create: `cmd/kg/main.go`
- Create: `cmd/kg/root.go`
- Create: `cmd/kg/output.go`
- Create: `cmd/kg/errmap.go`
- Create: `cmd/kg/root_test.go`

- [ ] **Step 1: Remove placeholder**

```bash
rm cmd/kg/.keep
```

- [ ] **Step 2: Create `cmd/kg/main.go`**

Create `cmd/kg/main.go`:

```go
package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
```

- [ ] **Step 3: Create `cmd/kg/output.go`**

Create `cmd/kg/output.go`:

```go
package main

import (
	"encoding/json"
	"io"
)

type envelope struct {
	OK    bool    `json:"ok"`
	Data  any     `json:"data,omitempty"`
	Error *envErr `json:"error,omitempty"`
}

type envErr struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

func writeOK(w io.Writer, data any) error {
	return writeJSON(w, envelope{OK: true, Data: data})
}

func writeErr(w io.Writer, code, message, hint string) error {
	return writeJSON(w, envelope{OK: false, Error: &envErr{Code: code, Message: message, Hint: hint}})
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
```

- [ ] **Step 4: Create `cmd/kg/errmap.go`**

Create `cmd/kg/errmap.go`:

```go
package main

import (
	"errors"

	"github.com/ggfarmco/kg/internal/graph"
)

type mapped struct {
	exit    int
	code    string
	message string
	hint    string
}

func mapError(err error) mapped {
	switch {
	case errors.Is(err, graph.ErrDomainNotFound):
		return mapped{3, "DOMAIN_NOT_FOUND", err.Error(), "run `kg domain list`"}
	case errors.Is(err, graph.ErrNodeNotFound):
		return mapped{3, "NODE_NOT_FOUND", err.Error(), "run `kg node list` to find existing IDs"}
	case errors.Is(err, graph.ErrEdgeNotFound):
		return mapped{3, "EDGE_NOT_FOUND", err.Error(), ""}
	case errors.Is(err, graph.ErrDomainAlreadyExists):
		return mapped{2, "DOMAIN_ALREADY_EXISTS", err.Error(), "use --if-not-exists to skip silently"}
	case errors.Is(err, graph.ErrNodeAlreadyExists):
		return mapped{2, "NODE_ALREADY_EXISTS", err.Error(), "use --if-not-exists to skip silently"}
	case errors.Is(err, graph.ErrEdgeAlreadyExists):
		return mapped{2, "EDGE_ALREADY_EXISTS", err.Error(), "use --if-not-exists to skip silently"}
	case errors.Is(err, graph.ErrInvalidSlug):
		return mapped{1, "INVALID_SLUG", err.Error(), "slugs must match ^[a-z0-9-]+$"}
	case errors.Is(err, graph.ErrSlugCannotDerive):
		return mapped{1, "SLUG_CANNOT_DERIVE", err.Error(), "pass --id explicitly"}
	case errors.Is(err, graph.ErrLayerNotInDomain):
		return mapped{1, "LAYER_NOT_IN_DOMAIN", err.Error(), ""}
	case errors.Is(err, graph.ErrParentDomainMismatch):
		return mapped{1, "PARENT_DOMAIN_MISMATCH", err.Error(), ""}
	case errors.Is(err, graph.ErrParentLayerMismatch):
		return mapped{1, "PARENT_LAYER_MISMATCH", err.Error(), ""}
	case errors.Is(err, graph.ErrTopLayerCannotHaveParent):
		return mapped{1, "TOP_LAYER_CANNOT_HAVE_PARENT", err.Error(), ""}
	case errors.Is(err, graph.ErrEdgeSelfLoop):
		return mapped{1, "EDGE_SELF_LOOP", err.Error(), ""}
	default:
		return mapped{10, "INTERNAL", err.Error(), ""}
	}
}
```

- [ ] **Step 5: Create `cmd/kg/root.go` with stubbed subcommands**

Create `cmd/kg/root.go`:

```go
package main

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/store"
)

type cliCtx struct {
	dbPath  string
	openSvc func(dbPath string) (*graph.Service, func(), error)
	stdout  io.Writer
	stderr  io.Writer
}

func newRootCmd(c *cliCtx) *cobra.Command {
	root := &cobra.Command{
		Use:           "kg",
		Short:         "kg — domain-agnostic knowledge graph engine",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVar(&c.dbPath, "db", envOr("KG_DB", "./kg.db"), "path to the SQLite database file")
	root.AddCommand(newInitCmd(c), newDomainCmd(c), newNodeCmd(c), newEdgeCmd(c))
	return root
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func openService(dbPath string) (*graph.Service, func(), error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, nil, err
	}
	svc := graph.NewService(st, nil)
	return svc, func() { _ = st.Close() }, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	c := &cliCtx{openSvc: openService, stdout: stdout, stderr: stderr}
	cmd := newRootCmd(c)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		m := mapError(err)
		_ = writeErr(stdout, m.code, m.message, m.hint)
		return m.exit
	}
	return 0
}

func newInitCmd(*cliCtx) *cobra.Command {
	return &cobra.Command{Use: "init", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
func newDomainCmd(*cliCtx) *cobra.Command { return &cobra.Command{Use: "domain", Hidden: true} }
func newNodeCmd(*cliCtx) *cobra.Command   { return &cobra.Command{Use: "node", Hidden: true} }
func newEdgeCmd(*cliCtx) *cobra.Command   { return &cobra.Command{Use: "edge", Hidden: true} }
```

- [ ] **Step 6: Smoke test**

Create `cmd/kg/root_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootHelpExitsZero(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--help"}, &out, &errOut)
	require.Equal(t, 0, code)
	require.Contains(t, out.String()+errOut.String(), "kg")
}

func TestUnknownCommandReturnsEnvelope(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"frob"}, &out, &errOut)
	require.NotEqual(t, 0, code)

	var env envelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.False(t, env.OK)
	require.NotNil(t, env.Error)
}
```

- [ ] **Step 7: Run, verify pass**

```bash
go test ./cmd/kg/ -v
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add cmd/kg/
git commit -m "feat(cli): scaffold cobra root, JSON envelope, and error map"
```

---

### Task 21: `kg init` and `kg domain` subcommands

**Files:**
- Create: `cmd/kg/init_cmd.go`
- Create: `cmd/kg/domain_cmds.go`
- Create: `cmd/kg/skip.go`
- Modify: `cmd/kg/root.go` (replace init+domain stubs)
- Modify: `internal/graph/service.go` (expose `InTx` shim)
- Create: `cmd/kg/domain_cmds_test.go`

- [ ] **Step 1: Add Service.InTx passthrough so the CLI can build `--dry-run`**

Append to `internal/graph/service.go`:

```go
func (s *Service) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return s.store.InTx(ctx, fn)
}
```

- [ ] **Step 2: Create the skip helper**

Create `cmd/kg/skip.go`:

```go
package main

import (
	"errors"
	"io"

	"github.com/ggfarmco/kg/internal/graph"
)

func handleMaybeSkip(w io.Writer, err error, ifNotExists bool) error {
	if err == nil {
		return nil
	}
	if ifNotExists && isAlreadyExists(err) {
		return writeOK(w, map[string]any{"skipped": true, "reason": "already_exists"})
	}
	return err
}

func isAlreadyExists(err error) bool {
	return errors.Is(err, graph.ErrDomainAlreadyExists) ||
		errors.Is(err, graph.ErrNodeAlreadyExists) ||
		errors.Is(err, graph.ErrEdgeAlreadyExists)
}
```

- [ ] **Step 3: Create init command**

Create `cmd/kg/init_cmd.go`:

```go
package main

import "github.com/spf13/cobra"

func newInitCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the database (runs migrations)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			return writeOK(c.stdout, map[string]any{"initialized": true, "db": c.dbPath})
		},
	}
}
```

In `cmd/kg/root.go`, replace `func newInitCmd(*cliCtx) *cobra.Command { ... }` with:

```go
func newInitCmd(c *cliCtx) *cobra.Command { return newInitCmdReal(c) }
```

- [ ] **Step 4: Create domain commands**

Create `cmd/kg/domain_cmds.go`:

```go
package main

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
)

func newDomainCmdReal(c *cliCtx) *cobra.Command {
	cmd := &cobra.Command{Use: "domain", Short: "Manage domains"}
	cmd.AddCommand(newDomainAddCmd(c), newDomainListCmd(c), newDomainGetCmd(c), newDomainDeleteCmd(c))
	return cmd
}

func newDomainAddCmd(c *cliCtx) *cobra.Command {
	var layers, description string
	var ifNotExists, dryRun bool
	cmd := &cobra.Command{
		Use:   "add <id>",
		Short: "Add a domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			in := graph.AddDomainInput{ID: args[0], Description: description, Layers: splitCSV(layers)}
			if dryRun {
				sentinel := errors.New("dry-run rollback")
				err := svc.InTx(cmd.Context(), func(ctx context.Context) error {
					if _, err := svc.AddDomain(ctx, in); err != nil {
						return err
					}
					return sentinel
				})
				if errors.Is(err, sentinel) {
					return writeOK(c.stdout, map[string]any{"dry_run": true})
				}
				return handleMaybeSkip(c.stdout, err, ifNotExists)
			}
			d, err := svc.AddDomain(cmd.Context(), in)
			if err != nil {
				return handleMaybeSkip(c.stdout, err, ifNotExists)
			}
			return writeOK(c.stdout, d)
		},
	}
	cmd.Flags().StringVar(&layers, "layers", "", "comma-separated ordered layer names (required)")
	cmd.Flags().StringVar(&description, "description", "", "free-form description")
	cmd.Flags().BoolVar(&ifNotExists, "if-not-exists", false, "skip with exit 0 if the domain already exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without committing")
	_ = cmd.MarkFlagRequired("layers")
	return cmd
}

func newDomainListCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List domains",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			ds, err := svc.ListDomains(cmd.Context())
			if err != nil {
				return err
			}
			return writeOK(c.stdout, ds)
		},
	}
}

func newDomainGetCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Get a domain by id",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			id, perr := graph.ParseDomainID(args[0])
			if perr != nil {
				return perr
			}
			d, err := svc.GetDomain(cmd.Context(), id)
			if err != nil {
				return err
			}
			return writeOK(c.stdout, d)
		},
	}
}

func newDomainDeleteCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Delete a domain",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			id, perr := graph.ParseDomainID(args[0])
			if perr != nil {
				return perr
			}
			if err := svc.DeleteDomain(cmd.Context(), id); err != nil {
				return err
			}
			return writeOK(c.stdout, map[string]any{"deleted": true, "id": id})
		},
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
```

In `cmd/kg/root.go`, replace `func newDomainCmd(*cliCtx) *cobra.Command { ... }` with:

```go
func newDomainCmd(c *cliCtx) *cobra.Command { return newDomainCmdReal(c) }
```

- [ ] **Step 5: Write end-to-end tests**

Create `cmd/kg/domain_cmds_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func runCLI(dbPath string, args ...string) (int, string) {
	var out, errOut bytes.Buffer
	full := append([]string{"--db", dbPath}, args...)
	code := run(full, &out, &errOut)
	return code, out.String()
}

func TestDomainAddListGetDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kg.db")

	code, body := runCLI(dbPath, "init")
	require.Equal(t, 0, code, body)

	code, body = runCLI(dbPath, "domain", "add", "cars", "--layers", "system,subsystem,part")
	require.Equal(t, 0, code, body)
	var env envelope
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	require.True(t, env.OK)

	code, body = runCLI(dbPath, "domain", "add", "cars", "--layers", "system")
	require.Equal(t, 2, code, body)

	code, body = runCLI(dbPath, "domain", "add", "cars", "--layers", "system", "--if-not-exists")
	require.Equal(t, 0, code, body)
	require.Contains(t, body, `"skipped": true`)

	code, body = runCLI(dbPath, "domain", "get", "cars")
	require.Equal(t, 0, code, body)
	require.Contains(t, body, `"cars"`)

	code, body = runCLI(dbPath, "domain", "list")
	require.Equal(t, 0, code, body)

	code, body = runCLI(dbPath, "domain", "delete", "cars")
	require.Equal(t, 0, code, body)
}

func TestDomainAddDryRun(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kg.db")
	code, body := runCLI(dbPath, "init")
	require.Equal(t, 0, code, body)
	code, body = runCLI(dbPath, "domain", "add", "cars", "--layers", "system", "--dry-run")
	require.Equal(t, 0, code, body)
	require.Contains(t, body, `"dry_run": true`)

	code, body = runCLI(dbPath, "domain", "list")
	require.Equal(t, 0, code, body)
	require.NotContains(t, body, `"cars"`)
}
```

- [ ] **Step 6: Run, verify pass**

```bash
go test ./cmd/kg/ -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/kg/ internal/graph/service.go
git commit -m "feat(cli): add init, domain commands with --if-not-exists and --dry-run"
```

---

### Task 22: `kg node` subcommands

**Files:**
- Create: `cmd/kg/node_cmds.go`
- Modify: `cmd/kg/root.go` (replace node stub)
- Create: `cmd/kg/node_cmds_test.go`

- [ ] **Step 1: Implement node subcommands**

Create `cmd/kg/node_cmds.go`:

```go
package main

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
)

func newNodeCmdReal(c *cliCtx) *cobra.Command {
	cmd := &cobra.Command{Use: "node", Short: "Manage nodes"}
	cmd.AddCommand(
		newNodeAddCmd(c), newNodeGetCmd(c), newNodeListCmd(c),
		newNodeChildrenCmd(c), newNodeUpdateCmd(c), newNodeDeleteCmd(c),
	)
	return cmd
}

func newNodeAddCmd(c *cliCtx) *cobra.Command {
	var domain, layer, name, id, parent, summary string
	var ifNotExists, dryRun bool
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a node",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			in := graph.AddNodeInput{Domain: domain, Layer: layer, Name: name, ID: id, Parent: parent, Summary: summary}
			if dryRun {
				sentinel := errors.New("dry-run rollback")
				err := svc.InTx(cmd.Context(), func(ctx context.Context) error {
					if _, err := svc.AddNode(ctx, in); err != nil {
						return err
					}
					return sentinel
				})
				if errors.Is(err, sentinel) {
					return writeOK(c.stdout, map[string]any{"dry_run": true})
				}
				return handleMaybeSkip(c.stdout, err, ifNotExists)
			}
			n, err := svc.AddNode(cmd.Context(), in)
			if err != nil {
				return handleMaybeSkip(c.stdout, err, ifNotExists)
			}
			return writeOK(c.stdout, n)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "domain id (required)")
	cmd.Flags().StringVar(&layer, "layer", "", "layer name (required)")
	cmd.Flags().StringVar(&name, "name", "", "human-readable name (required)")
	cmd.Flags().StringVar(&id, "id", "", "explicit slug; if omitted, derived from name")
	cmd.Flags().StringVar(&parent, "parent", "", "parent node id (required unless top layer)")
	cmd.Flags().StringVar(&summary, "summary", "", "optional summary text")
	cmd.Flags().BoolVar(&ifNotExists, "if-not-exists", false, "skip with exit 0 if the node already exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without committing")
	for _, f := range []string{"domain", "layer", "name"} {
		_ = cmd.MarkFlagRequired(f)
	}
	return cmd
}

func newNodeGetCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "get <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Get a node by id",
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
			return writeOK(c.stdout, n)
		},
	}
}

func newNodeListCmd(c *cliCtx) *cobra.Command {
	var domain, layer string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List nodes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			ns, err := svc.ListNodes(cmd.Context(), graph.NodeFilter{
				Domain: graph.DomainID(domain), Layer: layer, Limit: limit,
			})
			if err != nil {
				return err
			}
			return writeOK(c.stdout, ns)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "filter by domain id")
	cmd.Flags().StringVar(&layer, "layer", "", "filter by layer name")
	cmd.Flags().IntVar(&limit, "limit", 0, "max rows (0 = unlimited)")
	return cmd
}

func newNodeChildrenCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "children <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "List direct children of a node",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			ns, err := svc.ChildrenOf(cmd.Context(), graph.NodeID(args[0]))
			if err != nil {
				return err
			}
			return writeOK(c.stdout, ns)
		},
	}
}

func newNodeUpdateCmd(c *cliCtx) *cobra.Command {
	var name, summary string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Update a node's name or summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			in := graph.UpdateNodeInput{}
			if cmd.Flags().Changed("name") {
				in.Name = &name
			}
			if cmd.Flags().Changed("summary") {
				in.Summary = &summary
			}
			if dryRun {
				sentinel := errors.New("dry-run rollback")
				err := svc.InTx(cmd.Context(), func(ctx context.Context) error {
					if _, err := svc.UpdateNode(ctx, graph.NodeID(args[0]), in); err != nil {
						return err
					}
					return sentinel
				})
				if errors.Is(err, sentinel) {
					return writeOK(c.stdout, map[string]any{"dry_run": true})
				}
				return err
			}
			n, err := svc.UpdateNode(cmd.Context(), graph.NodeID(args[0]), in)
			if err != nil {
				return err
			}
			return writeOK(c.stdout, n)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&summary, "summary", "", "new summary")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without committing")
	return cmd
}

func newNodeDeleteCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Delete a node",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			id := graph.NodeID(args[0])
			if err := svc.DeleteNode(cmd.Context(), id); err != nil {
				return err
			}
			return writeOK(c.stdout, map[string]any{"deleted": true, "id": id})
		},
	}
}
```

In `cmd/kg/root.go`, replace the `newNodeCmd` stub with:

```go
func newNodeCmd(c *cliCtx) *cobra.Command { return newNodeCmdReal(c) }
```

- [ ] **Step 2: Write tests**

Create `cmd/kg/node_cmds_test.go`:

```go
package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNodeWalkthrough(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kg.db")

	code, body := runCLI(dbPath, "init")
	require.Equal(t, 0, code, body)
	code, body = runCLI(dbPath, "domain", "add", "cars", "--layers", "system,subsystem,part")
	require.Equal(t, 0, code, body)

	code, body = runCLI(dbPath, "node", "add", "--domain", "cars", "--layer", "system", "--name", "Powertrain")
	require.Equal(t, 0, code, body)
	require.Contains(t, body, `"cars:powertrain"`)

	code, body = runCLI(dbPath, "node", "add", "--domain", "cars", "--layer", "subsystem", "--name", "Engine", "--parent", "cars:powertrain")
	require.Equal(t, 0, code, body)

	code, body = runCLI(dbPath, "node", "children", "cars:powertrain")
	require.Equal(t, 0, code, body)
	var env envelope
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	require.True(t, env.OK)
	kids := env.Data.([]any)
	require.Len(t, kids, 1)

	code, body = runCLI(dbPath, "node", "update", "cars:powertrain", "--summary", "the drive train")
	require.Equal(t, 0, code, body)
	require.Contains(t, body, "the drive train")

	code, body = runCLI(dbPath, "node", "delete", "cars:powertrain")
	require.NotEqual(t, 0, code, body) // RESTRICT: child still exists
}

func TestNodeAddIfNotExistsSkips(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kg.db")
	c1, _ := runCLI(dbPath, "init")
	c2, _ := runCLI(dbPath, "domain", "add", "cars", "--layers", "system")
	c3, _ := runCLI(dbPath, "node", "add", "--domain", "cars", "--layer", "system", "--name", "PT")
	require.Equal(t, 0, c1)
	require.Equal(t, 0, c2)
	require.Equal(t, 0, c3)

	code, body := runCLI(dbPath, "node", "add", "--domain", "cars", "--layer", "system", "--name", "PT", "--if-not-exists")
	require.Equal(t, 0, code, body)
	require.Contains(t, body, `"skipped": true`)
}
```

- [ ] **Step 3: Run, verify pass**

```bash
go test ./cmd/kg/ -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/kg/
git commit -m "feat(cli): add node commands with parent rules and --if-not-exists"
```

---

### Task 23: `kg edge` subcommands

**Files:**
- Create: `cmd/kg/edge_cmds.go`
- Modify: `cmd/kg/root.go` (replace edge stub)
- Create: `cmd/kg/edge_cmds_test.go`

- [ ] **Step 1: Implement edge subcommands**

Create `cmd/kg/edge_cmds.go`:

```go
package main

import (
	"context"
	"errors"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
)

func newEdgeCmdReal(c *cliCtx) *cobra.Command {
	cmd := &cobra.Command{Use: "edge", Short: "Manage edges"}
	cmd.AddCommand(newEdgeAddCmd(c), newEdgeListFromCmd(c), newEdgeListToCmd(c), newEdgeDeleteCmd(c))
	return cmd
}

func newEdgeAddCmd(c *cliCtx) *cobra.Command {
	var typ string
	var ifNotExists, dryRun bool
	cmd := &cobra.Command{
		Use:   "add <source-id> <target-id>",
		Args:  cobra.ExactArgs(2),
		Short: "Add an edge",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			in := graph.AddEdgeInput{Source: args[0], Target: args[1], Type: typ}
			if dryRun {
				sentinel := errors.New("dry-run rollback")
				err := svc.InTx(cmd.Context(), func(ctx context.Context) error {
					if _, err := svc.AddEdge(ctx, in); err != nil {
						return err
					}
					return sentinel
				})
				if errors.Is(err, sentinel) {
					return writeOK(c.stdout, map[string]any{"dry_run": true})
				}
				return handleMaybeSkip(c.stdout, err, ifNotExists)
			}
			e, err := svc.AddEdge(cmd.Context(), in)
			if err != nil {
				return handleMaybeSkip(c.stdout, err, ifNotExists)
			}
			return writeOK(c.stdout, e)
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "edge type (required)")
	cmd.Flags().BoolVar(&ifNotExists, "if-not-exists", false, "skip with exit 0 if the edge already exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without committing")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

func newEdgeListFromCmd(c *cliCtx) *cobra.Command {
	var typ string
	cmd := &cobra.Command{
		Use:   "list-from <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "List edges originating at the given node",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			types := []string(nil)
			if typ != "" {
				types = []string{typ}
			}
			es, err := svc.EdgesFrom(cmd.Context(), graph.NodeID(args[0]), types)
			if err != nil {
				return err
			}
			return writeOK(c.stdout, es)
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "filter by edge type")
	return cmd
}

func newEdgeListToCmd(c *cliCtx) *cobra.Command {
	var typ string
	cmd := &cobra.Command{
		Use:   "list-to <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "List edges arriving at the given node",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			types := []string(nil)
			if typ != "" {
				types = []string{typ}
			}
			es, err := svc.EdgesTo(cmd.Context(), graph.NodeID(args[0]), types)
			if err != nil {
				return err
			}
			return writeOK(c.stdout, es)
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "filter by edge type")
	return cmd
}

func newEdgeDeleteCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <edge-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Delete an edge",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			n, perr := strconv.ParseInt(args[0], 10, 64)
			if perr != nil {
				return perr
			}
			if err := svc.DeleteEdge(cmd.Context(), graph.EdgeID(n)); err != nil {
				return err
			}
			return writeOK(c.stdout, map[string]any{"deleted": true, "id": n})
		},
	}
}
```

In `cmd/kg/root.go`, replace the `newEdgeCmd` stub with:

```go
func newEdgeCmd(c *cliCtx) *cobra.Command { return newEdgeCmdReal(c) }
```

- [ ] **Step 2: Write tests**

Create `cmd/kg/edge_cmds_test.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEdgeWalkthrough(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kg.db")
	for _, args := range [][]string{
		{"init"},
		{"domain", "add", "cars", "--layers", "system"},
		{"node", "add", "--domain", "cars", "--layer", "system", "--name", "A"},
		{"node", "add", "--domain", "cars", "--layer", "system", "--name", "B"},
	} {
		code, body := runCLI(dbPath, args...)
		require.Equal(t, 0, code, body)
	}

	code, body := runCLI(dbPath, "edge", "add", "cars:a", "cars:b", "--type", "depends_on")
	require.Equal(t, 0, code, body)

	var env envelope
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	data := env.Data.(map[string]any)
	id := int64(data["ID"].(float64))
	require.NotZero(t, id)

	code, body = runCLI(dbPath, "edge", "list-from", "cars:a")
	require.Equal(t, 0, code, body)

	code, body = runCLI(dbPath, "edge", "list-to", "cars:b", "--type", "depends_on")
	require.Equal(t, 0, code, body)

	code, body = runCLI(dbPath, "edge", "delete", fmt.Sprint(id))
	require.Equal(t, 0, code, body)
}
```

(Asserts the JSON field name `ID` matches Go struct field naming. If the spec later mandates snake_case JSON, add `json:"id"` to `graph.Edge` and update the test.)

- [ ] **Step 3: Run, verify pass**

```bash
go test ./cmd/kg/ -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/kg/
git commit -m "feat(cli): add edge commands"
```

---

### Task 24: `--help --json` machine-readable command tree

**Files:**
- Modify: `cmd/kg/root.go`
- Create: `cmd/kg/help_json.go`
- Create: `cmd/kg/help_json_test.go`

- [ ] **Step 1: Write failing test**

Create `cmd/kg/help_json_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHelpJSONListsCommandTree(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--help", "--json"}, &out, &errOut)
	require.Equal(t, 0, code)

	var env envelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.True(t, env.OK)

	root := env.Data.(map[string]any)
	require.Equal(t, "kg", root["name"])
	cmds := root["commands"].([]any)
	names := []string{}
	for _, c := range cmds {
		names = append(names, c.(map[string]any)["name"].(string))
	}
	require.Contains(t, names, "domain")
	require.Contains(t, names, "node")
	require.Contains(t, names, "edge")
	require.Contains(t, names, "init")
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg/ -run HelpJSON -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `cmd/kg/help_json.go`**

Create `cmd/kg/help_json.go`:

```go
package main

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type helpCmd struct {
	Name     string     `json:"name"`
	Short    string     `json:"short,omitempty"`
	Use      string     `json:"use,omitempty"`
	Flags    []helpFlag `json:"flags,omitempty"`
	Commands []helpCmd  `json:"commands,omitempty"`
}

type helpFlag struct {
	Name    string `json:"name"`
	Short   string `json:"short,omitempty"`
	Type    string `json:"type"`
	Default string `json:"default,omitempty"`
	Usage   string `json:"usage,omitempty"`
}

func commandTree(c *cobra.Command) helpCmd {
	out := helpCmd{Name: c.Name(), Short: c.Short, Use: c.Use}
	c.LocalFlags().VisitAll(func(f *pflag.Flag) {
		out.Flags = append(out.Flags, helpFlag{
			Name: f.Name, Short: f.Shorthand, Type: f.Value.Type(), Default: f.DefValue, Usage: f.Usage,
		})
	})
	for _, sub := range c.Commands() {
		if sub.Hidden {
			continue
		}
		out.Commands = append(out.Commands, commandTree(sub))
	}
	return out
}
```

- [ ] **Step 4: Intercept `--help --json` in `run`**

In `cmd/kg/root.go`, modify `run`:

```go
func run(args []string, stdout, stderr io.Writer) int {
	c := &cliCtx{openSvc: openService, stdout: stdout, stderr: stderr}
	cmd := newRootCmd(c)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	if wantsHelpJSON(args) {
		_ = writeOK(stdout, commandTree(cmd))
		return 0
	}

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		m := mapError(err)
		_ = writeErr(stdout, m.code, m.message, m.hint)
		return m.exit
	}
	return 0
}

func wantsHelpJSON(args []string) bool {
	hasHelp, hasJSON := false, false
	for _, a := range args {
		switch a {
		case "--help", "-h":
			hasHelp = true
		case "--json":
			hasJSON = true
		}
	}
	return hasHelp && hasJSON
}
```

- [ ] **Step 5: Run, verify pass**

```bash
go test ./cmd/kg/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/kg/
git commit -m "feat(cli): add --help --json machine-readable command tree"
```

---

### Task 25: Lint + build baseline

**Files:** none (verification only)

- [ ] **Step 1: Run linter**

```bash
make lint
```

Expected: PASS. If golangci-lint flags issues (unused stubs left behind, unchecked errors, etc.), fix them in place and commit under `chore: lint cleanup`.

- [ ] **Step 2: Run full test suite**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Build the binary and smoke-test help**

```bash
make build && ./bin/kg --help
```

Expected: cobra help renders; exit 0.

- [ ] **Step 4: Try the LLM-introspection path**

```bash
./bin/kg --help --json | head -40
```

Expected: a JSON envelope listing the four top-level commands.

- [ ] **Step 5: Commit any cleanup**

```bash
git add -A
git diff --cached --quiet || git commit -m "chore: lint and build cleanup"
```

---

## Phase 6 — Docs

### Task 26: README walkthrough

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Replace `README.md`**

Overwrite `README.md`:

```markdown
# kg

Domain-agnostic knowledge graph engine in Go with SQLite storage.

## Install

```sh
make build           # produces ./bin/kg
make install         # installs to $GOBIN
```

## Quick start

```sh
kg --db ./kg.db init

kg domain add cars --layers system,subsystem,part \
    --description "Vehicles and their components"

kg node add --domain cars --layer system --name "Powertrain"
kg node add --domain cars --layer subsystem --name "Engine" \
    --parent cars:powertrain
kg node add --domain cars --layer part --name "Piston" \
    --parent cars:engine

kg domain add physics --layers law
kg node add --domain physics --layer law --name "Thermodynamics"

kg edge add cars:engine physics:thermodynamics --type governed_by

kg node children cars:powertrain
kg edge list-from cars:engine
kg edge list-to physics:thermodynamics
```

All output is JSON. Failures emit `{"ok": false, "error": {...}}` with stable exit codes (0 success, 1 validation, 2 conflict, 3 not found, 10 internal).

### LLM-friendly affordances

- `--help --json` returns a machine-readable command tree for tool introspection.
- `--if-not-exists` makes `add` commands idempotent for `domain`, `node`, and `edge`.
- `--dry-run` validates against the live DB without committing.

## Architecture

Hexagonal split:

- `internal/graph` — pure Go types, validation, sentinel errors, `Store` interface.
- `internal/store` — SQLite adapter via `modernc.org/sqlite` + sqlc + goose.
- `cmd/kg` — cobra CLI; emits the JSON envelope and maps sentinel errors to exit codes.

See `docs/superpowers/specs/2026-05-23-kg-mvp-design.md` for the design spec and roadmap (v0 MVP → v7 embeddings).

## Schema (v0)

Four tables: `domains`, `nodes`, `edges`, `changes`. Every successful mutation writes one row to the `changes` log (strictly monotonic `seq`). Per-object `revision` columns are bumped on update inside the same transaction. The change log and revision are not yet exposed via CLI but are populated from day one to make future sync (`ChangesSince(seq)`, `--if-rev N`) a drop-in feature.

## Development

```sh
make test     # all tests (unit, integration, end-to-end)
make gen      # regenerate sqlc code from queries.sql
make migrate  # apply migrations via goose CLI to ./kg.db
make lint     # golangci-lint
```
```

- [ ] **Step 2: Verify**

```bash
make build && make test
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: replace README stub with walkthrough and architecture overview"
```

---

## Self-Review Checklist

**Spec coverage (each spec section → task):**

- Background / roadmap / design intent → captured in plan preamble (read at execution start).
- Database schema (4 tables + indexes + ON DELETE policies) → Task 2. Note: the spec's CREATE TABLE block writes `nodes.parent_id ... ON DELETE SET NULL` but the "ON DELETE policies" subsection mandates RESTRICT. Task 2 uses RESTRICT per the policies (which the spec calls authoritative).
- Schema invariants (revision bump in same tx, one change row per mutation, monotonic seq, AUTOINCREMENT) → Tasks 16, 17, 18, 19.
- AddNode validation (all 6 rules) → Tasks 10, 11.
- AddEdge validation (self-loop, UNIQUE) → Task 13 + Task 18 (DB enforcement).
- Cross-domain edges (free-form) → Task 13 (no domain check in AddEdge).
- Immutable IDs → no rename command implemented; absence is the requirement.
- Identifiers (typed aliases, constructors, parsers) → Task 3.
- Go domain model → Task 4.
- Store interface with InTx contract → Tasks 6, 15.
- Sentinel errors → Task 5.
- CLI commands (init, domain, node, edge) → Tasks 21–23.
- JSON envelope (success + failure) → Task 20.
- Exit codes (0/1/2/3/10) → Task 20 errmap.
- `--if-not-exists` semantics → Tasks 21 (skip helper), 22, 23.
- `--dry-run` semantics → Tasks 21, 22, 23.
- `--help --json` → Task 24.
- Project layout → Task 1.
- Tooling (cobra, modernc.org/sqlite, sqlc, goose, testify, golangci-lint, Makefile) → Task 1.
- Error handling layers (Store → graph sentinels, Service returns sentinels, CLI maps) → Tasks 16, 8–13, 20.
- Testing tiers (unit graph via fake, integration store via :memory:, end-to-end cli) → Tasks 8–13 + 7, 14–19, 21–24.
- Versioning & collaboration foundation (revision populated, changes log appended, no CLI surface for ChangesSince/--if-rev) → Tasks 16–19; absence of CLI commands matches the spec.

**Placeholder scan:** No "TBD" / "implement later" / "similar to Task N". Each step has explicit code or an exact command.

**Type consistency:** Service input types (`AddDomainInput`, `AddNodeInput`, `UpdateNodeInput`, `AddEdgeInput`) defined in first usage and unchanged thereafter. `graph.Service.InTx` (Task 21) added intentionally so the CLI can drive dry-run rollback without leaking Store details. Store methods consistently take/return `graph.*` types — sqlc row types never leave `internal/store`.

**Known plan-time gaps to expect during execution:**

- sqlc row type names depend on whether sqlc shares a single row type for queries with identical column lists. The `decodeNode` / `decodeEdge` helpers take positional arguments to absorb this — adjust call sites if sqlc emits unexpected names. The shape stays right; only identifiers shift.
- The `migration_test.go` from Task 2 is removed in Task 14 once `store.Open` covers the same surface.
- `ExecForTest` in `internal/store/tx.go` is temporary scaffolding for Task 15 — deleted in Task 16 once real CRUD covers tx routing.
- CASCADE-deleted edges (when their endpoint node is deleted) are NOT written to the `changes` log. v6 collaboration will fix this with a trigger migration or by rewriting `DeleteNode` to enumerate-then-delete edges. Plan does not block on this — the spec's "one mutation, one row" reading is consistent with the current design.
- `make gen` in CI requires `go run github.com/sqlc-dev/sqlc/cmd/sqlc generate` to succeed; if sqlc complains about unknown SQL functions (`sqlc.arg`, `sqlc.slice`), check that `sqlc.yaml`'s `engine: "sqlite"` is set (sqlc's `mysql` and `postgresql` engines support different built-ins).

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-23-kg-mvp-implementation.md`. After completing all 26 tasks, the MVP is feature-complete and v1 (first extractor) can begin on a stable foundation.

Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration. Uses `superpowers:subagent-driven-development`.

**2. Inline Execution** — execute tasks in this session in batch with checkpoints. Uses `superpowers:executing-plans`.
