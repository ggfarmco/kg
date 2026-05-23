# kg v1 Implementation Plan — Extractor System

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the extractor system on top of the v0 kg engine: a public `batch/` contract package, a `kg batch` subcommand that atomically applies a JSONL op stream, a `cmd/kg-extractor/` dispatcher binary that discovers and invokes plugins, a demo bash plugin proving the contract works for non-Go runtimes, and a CGO-isolated `plugins/tree-sitter/` plugin (separate Go module) that registers a Go grammar and produces a `package → file → decl` graph plus `imports` and intra-package `calls` edges.

**Architecture:** Three new components wired by subprocess pipes — `plugin (executable) → kg-extractor (validator) → kg batch (engine)`. The contract lives at `batch/` (public, not internal, because plugins in separate modules need it). The dispatcher and `kg batch` live in the root module (pure Go). Only `plugins/tree-sitter/` introduces CGO, and only inside its own module — kg's `go.sum` stays clean for users who don't need extractors. A `go.work` file at the repo root makes local dev seamless; CI runs with `GOWORK=off` to validate plugins as external consumers.

**Tech Stack (delta from v0):** `github.com/smacker/go-tree-sitter` and `github.com/smacker/go-tree-sitter/golang` (CGO; in the plugins/tree-sitter module ONLY). Plus standard cobra/testify already in v0. The bash-demo plugin uses `bash` + `jq` (skipped in tests if absent).

**Spec:** `docs/superpowers/specs/2026-05-23-kg-v1-extractor-design.md`. Re-read it before each new phase — the design intent (CGO isolation, why `batch/` is public, the `protocol_version` story, slug sanitization rules, layer model) is not redundantly restated in each task.

**Prereq:** This plan builds on v0 + the v0 polish pass (snake_case JSON tags on graph types, FK RESTRICT enforcement tests, silenced goose logger). Latest commit on `main` before starting: `e25772d` ("docs(v1): switch tree-sitter plugin from per-language to unified"). All work for v1 lands on the branch `feat/kg-v1` and merges back to main as one unit at the end.

**Conventions:**
- Import grouping (3 blocks separated by blank lines): stdlib, third-party, current module (`github.com/ggfarmco/kg/...`). For the `plugins/tree-sitter/` module, the import `github.com/ggfarmco/kg/batch` sits in the *third-party* block (it is a different module from the plugin's perspective).
- No comments in code unless they explain a non-obvious *why*. Generated sqlc files are exempt.
- Tests are minimal and non-redundant — each test covers one distinct behavior.
- Every task ends with a commit. Commit messages follow `<type>(scope): <imperative summary>` (types: feat, test, chore, docs, refactor, fix; scopes used in v0 include `graph`, `store`, `cli`; new scopes for v1: `batch`, `extractor`, `plugin-tree-sitter`, `e2e`).
- Service input structs grow `Properties map[string]any` fields in Phase 2 (extractors need them; CLI does not surface them per spec).

**Before starting:** verify the branch is `feat/kg-v1`:

```bash
git rev-parse --abbrev-ref HEAD  # should print feat/kg-v1
```

---

## Phase 1 — The `batch` contract package

The public package every producer and consumer of the JSONL op stream depends on. Lives at the repo root (`batch/`) so plugins in separate modules can import it. Defines the `Op` type, per-op typed argument structs, and a JSONL Decoder / Encoder.

### Task 1: `batch/op.go` — op types and per-op argument structs

**Files:**
- Create: `batch/op.go`
- Create: `batch/op_test.go`

- [ ] **Step 1: Write failing tests for op constants and argument round-trips**

Create `batch/op_test.go`:

```go
package batch_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/batch"
)

func TestOpNameConstants(t *testing.T) {
	require.Equal(t, batch.OpName("meta"), batch.OpMeta)
	require.Equal(t, batch.OpName("domain.add"), batch.OpDomainAdd)
	require.Equal(t, batch.OpName("node.add"), batch.OpNodeAdd)
	require.Equal(t, batch.OpName("node.update"), batch.OpNodeUpdate)
	require.Equal(t, batch.OpName("node.delete"), batch.OpNodeDelete)
	require.Equal(t, batch.OpName("edge.add"), batch.OpEdgeAdd)
	require.Equal(t, batch.OpName("edge.delete"), batch.OpEdgeDelete)
}

func TestDomainAddArgsRoundTrip(t *testing.T) {
	in := batch.DomainAddArgs{
		ID:          "my-app",
		Layers:      []string{"package", "file", "decl"},
		Description: "...",
		IfNotExists: true,
	}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"id":"my-app","layers":["package","file","decl"],"description":"...","if_not_exists":true}`, string(b))

	var out batch.DomainAddArgs
	require.NoError(t, json.Unmarshal(b, &out))
	require.Equal(t, in, out)
}

func TestNodeAddArgsOmitsEmpty(t *testing.T) {
	in := batch.NodeAddArgs{Domain: "my-app", Layer: "package", Name: "fmt"}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"domain":"my-app","layer":"package","name":"fmt"}`, string(b))
}

func TestNodeUpdateArgsDistinguishesAbsentFromEmpty(t *testing.T) {
	var out batch.NodeUpdateArgs
	require.NoError(t, json.Unmarshal([]byte(`{"id":"x:y"}`), &out))
	require.Nil(t, out.Name, "absent fields must stay nil")
	require.Nil(t, out.Summary)

	require.NoError(t, json.Unmarshal([]byte(`{"id":"x:y","name":""}`), &out))
	require.NotNil(t, out.Name)
	require.Equal(t, "", *out.Name)
}

func TestEdgeAddArgsRoundTrip(t *testing.T) {
	in := batch.EdgeAddArgs{Source: "a:b", Target: "a:c", Type: "imports", IfNotExists: true}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"source":"a:b","target":"a:c","type":"imports","if_not_exists":true}`, string(b))
}

func TestEdgeDeleteArgsUsesInt(t *testing.T) {
	in := batch.EdgeDeleteArgs{ID: 42}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"id":42}`, string(b))
}

func TestMetaArgsTotalOpsOptional(t *testing.T) {
	var out batch.MetaArgs
	require.NoError(t, json.Unmarshal([]byte(`{"plugin":"x"}`), &out))
	require.Equal(t, "x", out.Plugin)
	require.Zero(t, out.TotalOps)
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./batch/ -v
```

Expected: FAIL (package does not exist).

- [ ] **Step 3: Implement `batch/op.go`**

Create `batch/op.go`:

```go
package batch

import "encoding/json"

type OpName string

const (
	OpMeta       OpName = "meta"
	OpDomainAdd  OpName = "domain.add"
	OpNodeAdd    OpName = "node.add"
	OpNodeUpdate OpName = "node.update"
	OpNodeDelete OpName = "node.delete"
	OpEdgeAdd    OpName = "edge.add"
	OpEdgeDelete OpName = "edge.delete"
)

const ProtocolVersion = 1

type Op struct {
	Op   OpName          `json:"op"`
	Args json.RawMessage `json:"args"`
}

type MetaArgs struct {
	Plugin   string `json:"plugin,omitempty"`
	Version  string `json:"version,omitempty"`
	Language string `json:"language,omitempty"`
	TotalOps int64  `json:"total_ops,omitempty"`
}

type DomainAddArgs struct {
	ID          string         `json:"id"`
	Layers      []string       `json:"layers"`
	Description string         `json:"description,omitempty"`
	IfNotExists bool           `json:"if_not_exists,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
}

type NodeAddArgs struct {
	Domain      string         `json:"domain"`
	Layer       string         `json:"layer"`
	Name        string         `json:"name"`
	ID          string         `json:"id,omitempty"`
	Parent      string         `json:"parent,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	IfNotExists bool           `json:"if_not_exists,omitempty"`
}

type NodeUpdateArgs struct {
	ID      string  `json:"id"`
	Name    *string `json:"name,omitempty"`
	Summary *string `json:"summary,omitempty"`
}

type NodeDeleteArgs struct {
	ID string `json:"id"`
}

type EdgeAddArgs struct {
	Source      string         `json:"source"`
	Target      string         `json:"target"`
	Type        string         `json:"type"`
	Properties  map[string]any `json:"properties,omitempty"`
	IfNotExists bool           `json:"if_not_exists,omitempty"`
}

type EdgeDeleteArgs struct {
	ID int64 `json:"id"`
}

func IsKnownOp(name OpName) bool {
	switch name {
	case OpMeta, OpDomainAdd, OpNodeAdd, OpNodeUpdate, OpNodeDelete, OpEdgeAdd, OpEdgeDelete:
		return true
	}
	return false
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./batch/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add batch/
git commit -m "feat(batch): add Op types and per-op argument structs"
```

---

### Task 2: `batch/codec.go` — JSONL Decoder and Encoder

**Files:**
- Create: `batch/codec.go`
- Create: `batch/codec_test.go`

- [ ] **Step 1: Write failing tests for Decoder and Encoder**

Create `batch/codec_test.go`:

```go
package batch_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/batch"
)

func TestDecoderHappyPath(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"op":"meta","args":{"plugin":"x","total_ops":2}}`,
		`{"op":"domain.add","args":{"id":"a","layers":["l1"]}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"n"}}`,
	}, "\n") + "\n")

	d := batch.NewDecoder(in)
	var ops []batch.Op
	for {
		op, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		ops = append(ops, op)
	}
	require.Len(t, ops, 3)
	require.Equal(t, batch.OpMeta, ops[0].Op)
	require.Equal(t, batch.OpDomainAdd, ops[1].Op)
	require.Equal(t, batch.OpNodeAdd, ops[2].Op)
}

func TestDecoderSkipsBlankLines(t *testing.T) {
	in := strings.NewReader("\n  \n" + `{"op":"meta","args":{}}` + "\n\n")
	d := batch.NewDecoder(in)
	op, err := d.Next()
	require.NoError(t, err)
	require.Equal(t, batch.OpMeta, op.Op)

	_, err = d.Next()
	require.ErrorIs(t, err, io.EOF)
}

func TestDecoderUnknownOpReportsLine(t *testing.T) {
	in := strings.NewReader(`{"op":"meta","args":{}}` + "\n" + `{"op":"foo.bar","args":{}}` + "\n")
	d := batch.NewDecoder(in)
	_, err := d.Next()
	require.NoError(t, err)
	_, err = d.Next()
	require.Error(t, err)
	var pe *batch.ParseError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, 2, pe.Line)
	require.Contains(t, pe.Error(), "foo.bar")
}

func TestDecoderInvalidJSONReportsLine(t *testing.T) {
	in := strings.NewReader(`{"op":"meta","args":{}}` + "\n" + `not json` + "\n")
	d := batch.NewDecoder(in)
	_, err := d.Next()
	require.NoError(t, err)
	_, err = d.Next()
	require.Error(t, err)
	var pe *batch.ParseError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, 2, pe.Line)
}

func TestEncoderRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	enc := batch.NewEncoder(&buf)
	require.NoError(t, enc.Meta(batch.MetaArgs{Plugin: "x", TotalOps: 1}))
	require.NoError(t, enc.DomainAdd(batch.DomainAddArgs{ID: "a", Layers: []string{"l"}, IfNotExists: true}))

	d := batch.NewDecoder(&buf)
	first, err := d.Next()
	require.NoError(t, err)
	require.Equal(t, batch.OpMeta, first.Op)
	second, err := d.Next()
	require.NoError(t, err)
	require.Equal(t, batch.OpDomainAdd, second.Op)
}

func TestEncoderEmitsOnePerLine(t *testing.T) {
	var buf bytes.Buffer
	enc := batch.NewEncoder(&buf)
	require.NoError(t, enc.NodeAdd(batch.NodeAddArgs{Domain: "a", Layer: "l", Name: "n"}))
	require.NoError(t, enc.NodeAdd(batch.NodeAddArgs{Domain: "a", Layer: "l", Name: "m"}))
	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	require.Len(t, lines, 2)
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./batch/ -v
```

Expected: FAIL (`NewDecoder`/`NewEncoder` undefined).

- [ ] **Step 3: Implement `batch/codec.go`**

Create `batch/codec.go`:

```go
package batch

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

type ParseError struct {
	Line int
	Err  error
}

func (e *ParseError) Error() string { return fmt.Sprintf("line %d: %v", e.Line, e.Err) }
func (e *ParseError) Unwrap() error { return e.Err }

type Decoder struct {
	r    *bufio.Reader
	line int
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

func (d *Decoder) Next() (Op, error) {
	for {
		raw, err := d.r.ReadBytes('\n')
		if len(raw) == 0 && err == io.EOF {
			return Op{}, io.EOF
		}
		if err != nil && err != io.EOF {
			return Op{}, err
		}
		d.line++
		trimmed := trimSpace(raw)
		if len(trimmed) == 0 {
			if err == io.EOF {
				return Op{}, io.EOF
			}
			continue
		}
		var op Op
		if jerr := json.Unmarshal(trimmed, &op); jerr != nil {
			return Op{}, &ParseError{Line: d.line, Err: jerr}
		}
		if !IsKnownOp(op.Op) {
			return Op{}, &ParseError{Line: d.line, Err: fmt.Errorf("unknown op %q", op.Op)}
		}
		return op, nil
	}
}

func (d *Decoder) Line() int { return d.line }

type Encoder struct {
	w io.Writer
}

func NewEncoder(w io.Writer) *Encoder { return &Encoder{w: w} }

func (e *Encoder) emit(name OpName, args any) error {
	payload, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("marshal args: %w", err)
	}
	line, err := json.Marshal(Op{Op: name, Args: payload})
	if err != nil {
		return fmt.Errorf("marshal op: %w", err)
	}
	line = append(line, '\n')
	_, err = e.w.Write(line)
	return err
}

func (e *Encoder) Meta(a MetaArgs) error            { return e.emit(OpMeta, a) }
func (e *Encoder) DomainAdd(a DomainAddArgs) error  { return e.emit(OpDomainAdd, a) }
func (e *Encoder) NodeAdd(a NodeAddArgs) error      { return e.emit(OpNodeAdd, a) }
func (e *Encoder) NodeUpdate(a NodeUpdateArgs) error { return e.emit(OpNodeUpdate, a) }
func (e *Encoder) NodeDelete(a NodeDeleteArgs) error { return e.emit(OpNodeDelete, a) }
func (e *Encoder) EdgeAdd(a EdgeAddArgs) error      { return e.emit(OpEdgeAdd, a) }
func (e *Encoder) EdgeDelete(a EdgeDeleteArgs) error { return e.emit(OpEdgeDelete, a) }

func trimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && (b[start] == ' ' || b[start] == '\t' || b[start] == '\r' || b[start] == '\n') {
		start++
	}
	end := len(b)
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r' || b[end-1] == '\n') {
		end--
	}
	return b[start:end]
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./batch/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add batch/
git commit -m "feat(batch): add line-counted JSONL Decoder and typed Encoder"
```

---

## Phase 2 — `kg batch` subcommand

A new subcommand on the existing `kg` binary. Reads JSONL from stdin, routes each op through `graph.Service`, and applies them atomically (whole-stream-as-one-tx by default). Flags: `--chunk-size`, `--continue-on-error`, `--dry-run`, `--progress`. Stream parse errors short-circuit before touching the DB.

### Task 3: Extend Service inputs with Properties

The CLI does not surface properties, but extractors do (per spec — package/file/decl nodes all carry properties). Add an optional `Properties map[string]any` field to `AddNodeInput` and `AddEdgeInput`. Existing callers (CLI) pass `nil`, which Service treats as `map[string]any{}`.

**Files:**
- Modify: `internal/graph/service.go`
- Modify: `internal/graph/service_node_test.go`
- Modify: `internal/graph/service_edge_test.go`

- [ ] **Step 1: Write failing test for AddNode passing properties through**

Append to `internal/graph/service_node_test.go`:

```go
func TestAddNodeStoresProperties(t *testing.T) {
	svc, fs := newService(t)
	seedCarsDomain(t, svc)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt",
		Properties: map[string]any{"horsepower": float64(200)},
	})
	require.NoError(t, err)

	got, err := fs.GetNode(t.Context(), n.ID)
	require.NoError(t, err)
	require.Equal(t, float64(200), got.Properties["horsepower"])
}
```

Append to `internal/graph/service_edge_test.go`:

```go
func TestAddEdgeStoresProperties(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc)
	e, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{
		Source: string(a), Target: string(b), Type: "x",
		Properties: map[string]any{"weight": float64(1)},
	})
	require.NoError(t, err)
	require.Equal(t, float64(1), e.Properties["weight"])
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/graph/ -run 'AddNodeStoresProperties|AddEdgeStoresProperties' -v
```

Expected: FAIL (struct field undefined).

- [ ] **Step 3: Add Properties to inputs and use them in Service**

In `internal/graph/service.go`, add the `Properties` field on `AddNodeInput` (after `Summary`) and on `AddEdgeInput` (after `Type`):

```go
type AddNodeInput struct {
	Domain     string
	Layer      string
	Name       string
	ID         string
	Parent     string
	Summary    string
	Properties map[string]any
}
```

```go
type AddEdgeInput struct {
	Source     string
	Target     string
	Type       string
	Properties map[string]any
}
```

In `Service.AddNode`, replace `Properties: map[string]any{},` with:

```go
		Properties: nonNilProps(in.Properties),
```

In `Service.AddEdge`, replace `Properties: map[string]any{},` with:

```go
		Properties: nonNilProps(in.Properties),
```

Add the helper at the bottom of `internal/graph/service.go`:

```go
func nonNilProps(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./internal/graph/...
```

Expected: PASS (new tests pass; existing tests untouched).

- [ ] **Step 5: Commit**

```bash
git add internal/graph/
git commit -m "feat(graph): accept Properties on AddNode/AddEdge inputs"
```

---

### Task 4: `kg batch` skeleton, op router, atomic execution, meta handling

Wires `kg batch` into the cobra root, reads stdin via `batch.Decoder`, runs everything inside a single `svc.InTx`, dispatches each op to Service. The whole-stream-as-one-tx is the default. `meta` ops are logged to stderr (with `total_ops` captured) and are not routed to Service. `--if-not-exists` per-op is implemented here; the other flags arrive in later tasks.

**Files:**
- Create: `cmd/kg/batch_cmd.go`
- Create: `cmd/kg/batch_cmd_test.go`
- Modify: `cmd/kg/root.go` (register `newBatchCmd`)

- [ ] **Step 1: Write failing tests for the router via in-process kg command**

Create `cmd/kg/batch_cmd_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func execBatchCmd(t *testing.T, dbPath, stdin string, extraArgs ...string) (string, string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString(stdin)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	var stdout, stderr bytes.Buffer
	args := append([]string{"--db", dbPath, "batch"}, extraArgs...)
	exit := run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exit
}

func freshDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kg.db")

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--db", path, "init"}, &stdout, &stderr)
	require.Equal(t, 0, exit, "init failed: %s %s", stdout.String(), stderr.String())
	return path
}

func TestBatchHappyPath(t *testing.T) {
	db := freshDB(t)
	stream := strings.Join([]string{
		`{"op":"meta","args":{"plugin":"unit","total_ops":3}}`,
		`{"op":"domain.add","args":{"id":"a","layers":["l1","l2"]}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"root"}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l2","name":"child","parent":"a:root"}}`,
	}, "\n") + "\n"

	stdout, stderr, exit := execBatchCmd(t, db, stream)
	require.Equal(t, 0, exit, "stderr=%s", stderr)

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Applied int `json:"applied"`
			Skipped int `json:"skipped"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &env))
	require.True(t, env.OK)
	require.Equal(t, 3, env.Data.Applied)
	require.Equal(t, 0, env.Data.Skipped)
}

func TestBatchAtomicityRollsBackOnFailure(t *testing.T) {
	db := freshDB(t)
	stream := strings.Join([]string{
		`{"op":"domain.add","args":{"id":"a","layers":["l1"]}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"ok"}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"!!!"}}`,
	}, "\n") + "\n"

	_, _, exit := execBatchCmd(t, db, stream)
	require.NotEqual(t, 0, exit)

	// The first node.add must have been rolled back.
	var stdout, stderr bytes.Buffer
	listExit := run([]string{"--db", db, "node", "list", "--domain", "a"}, &stdout, &stderr)
	require.Equal(t, 0, listExit)
	require.Contains(t, stdout.String(), `"data": []`)
}

func TestBatchIfNotExistsCountsSkipped(t *testing.T) {
	db := freshDB(t)
	stream1 := `{"op":"domain.add","args":{"id":"a","layers":["l1"]}}` + "\n"
	_, _, exit := execBatchCmd(t, db, stream1)
	require.Equal(t, 0, exit)

	stream2 := `{"op":"domain.add","args":{"id":"a","layers":["l1"],"if_not_exists":true}}` + "\n"
	stdout, _, exit := execBatchCmd(t, db, stream2)
	require.Equal(t, 0, exit)
	require.Contains(t, stdout, `"applied": 0`)
	require.Contains(t, stdout, `"skipped": 1`)
}

func TestBatchInvalidJSONShortCircuits(t *testing.T) {
	db := freshDB(t)
	stream := strings.Join([]string{
		`{"op":"domain.add","args":{"id":"a","layers":["l1"]}}`,
		`not json`,
	}, "\n") + "\n"

	stdout, _, exit := execBatchCmd(t, db, stream)
	require.NotEqual(t, 0, exit)
	require.Contains(t, stdout, "INVALID_OP")

	// The first op must have been rolled back (validation pre-tx is even stricter than rollback).
	var listOut, listErr bytes.Buffer
	listExit := run([]string{"--db", db, "domain", "list"}, &listOut, &listErr)
	require.Equal(t, 0, listExit)
	require.Contains(t, listOut.String(), `"data": []`)
}
```

- [ ] **Step 2: Register `newBatchCmd` in the root**

In `cmd/kg/root.go`, change:

```go
	root.AddCommand(newInitCmd(c), newDomainCmd(c), newNodeCmd(c), newEdgeCmd(c))
```

to:

```go
	root.AddCommand(newInitCmd(c), newDomainCmd(c), newNodeCmd(c), newEdgeCmd(c), newBatchCmd(c))
```

- [ ] **Step 3: Run, verify failure**

```bash
go test ./cmd/kg/ -run TestBatch -v
```

Expected: FAIL (`newBatchCmd` undefined).

- [ ] **Step 4: Implement `cmd/kg/batch_cmd.go`**

Create `cmd/kg/batch_cmd.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/batch"
	"github.com/ggfarmco/kg/internal/graph"
)

type batchCounts struct {
	Applied int `json:"applied"`
	Skipped int `json:"skipped"`
	TookMs  int `json:"took_ms"`
}

func newBatchCmd(c *cliCtx) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch",
		Short: "Apply a JSONL stream of operations atomically",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			return runBatch(cmd.Context(), c.stdout, c.stderr, os.Stdin, svc)
		},
	}
	return cmd
}

func runBatch(ctx context.Context, stdout io.Writer, stderr io.Writer, stdin io.Reader, svc *graph.Service) error {
	ops, parseErr := drainStream(stdin, stderr)
	if parseErr != nil {
		writeErr(stdout, "INVALID_OP", parseErr.Error(), "")
		return parseErrSentinel{parseErr}
	}

	start := time.Now()
	counts := batchCounts{}
	txErr := svc.InTx(ctx, func(ctx context.Context) error {
		for _, op := range ops {
			applied, err := applyOp(ctx, svc, op)
			if err != nil {
				return err
			}
			if applied {
				counts.Applied++
			} else {
				counts.Skipped++
			}
		}
		return nil
	})
	if txErr != nil {
		return txErr
	}
	counts.TookMs = int(time.Since(start).Milliseconds())
	return writeOK(stdout, counts)
}

type parseErrSentinel struct{ err error }

func (p parseErrSentinel) Error() string { return p.err.Error() }

func drainStream(r io.Reader, stderr io.Writer) ([]batch.Op, error) {
	d := batch.NewDecoder(r)
	var ops []batch.Op
	for {
		op, err := d.Next()
		if errors.Is(err, io.EOF) {
			return ops, nil
		}
		if err != nil {
			return nil, err
		}
		if op.Op == batch.OpMeta {
			var m batch.MetaArgs
			_ = json.Unmarshal(op.Args, &m)
			fmt.Fprintf(stderr, "meta: plugin=%q total_ops=%d\n", m.Plugin, m.TotalOps)
			continue
		}
		ops = append(ops, op)
	}
}

func applyOp(ctx context.Context, svc *graph.Service, op batch.Op) (applied bool, err error) {
	switch op.Op {
	case batch.OpDomainAdd:
		var a batch.DomainAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("domain.add args: %w", err)
		}
		_, err := svc.AddDomain(ctx, graph.AddDomainInput{ID: a.ID, Description: a.Description, Layers: a.Layers})
		return classifyIfNotExists(err, a.IfNotExists, graph.ErrDomainAlreadyExists)

	case batch.OpNodeAdd:
		var a batch.NodeAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("node.add args: %w", err)
		}
		_, err := svc.AddNode(ctx, graph.AddNodeInput{
			Domain: a.Domain, Layer: a.Layer, Name: a.Name,
			ID: a.ID, Parent: a.Parent, Summary: a.Summary, Properties: a.Properties,
		})
		return classifyIfNotExists(err, a.IfNotExists, graph.ErrNodeAlreadyExists)

	case batch.OpNodeUpdate:
		var a batch.NodeUpdateArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("node.update args: %w", err)
		}
		_, err := svc.UpdateNode(ctx, graph.NodeID(a.ID), graph.UpdateNodeInput{Name: a.Name, Summary: a.Summary})
		if err != nil {
			return false, err
		}
		return true, nil

	case batch.OpNodeDelete:
		var a batch.NodeDeleteArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("node.delete args: %w", err)
		}
		if err := svc.DeleteNode(ctx, graph.NodeID(a.ID)); err != nil {
			return false, err
		}
		return true, nil

	case batch.OpEdgeAdd:
		var a batch.EdgeAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("edge.add args: %w", err)
		}
		_, err := svc.AddEdge(ctx, graph.AddEdgeInput{Source: a.Source, Target: a.Target, Type: a.Type, Properties: a.Properties})
		return classifyIfNotExists(err, a.IfNotExists, graph.ErrEdgeAlreadyExists)

	case batch.OpEdgeDelete:
		var a batch.EdgeDeleteArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("edge.delete args: %w", err)
		}
		if err := svc.DeleteEdge(ctx, graph.EdgeID(a.ID)); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("unknown op %q", op.Op)
}

func classifyIfNotExists(err error, ifNotExists bool, sentinel error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if ifNotExists && errors.Is(err, sentinel) {
		return false, nil
	}
	return false, err
}
```

- [ ] **Step 5: Run, verify pass**

```bash
go test ./cmd/kg/ -run TestBatch -v
```

Expected: PASS for the four new tests. Existing tests stay green.

- [ ] **Step 6: Commit**

```bash
git add batch/ cmd/kg/
git commit -m "feat(cli): add kg batch with atomic execution and per-op if_not_exists"
```

---

### Task 5: `--continue-on-error` flag

When set, each op runs in its own micro-transaction; failures are collected, batch continues. Final envelope reports `failed` count and a `failures[]` list with line number, op, code, message. Mutually exclusive with `--chunk-size` (v1 simplification — error if both set).

**Files:**
- Modify: `cmd/kg/batch_cmd.go`
- Modify: `cmd/kg/batch_cmd_test.go`

- [ ] **Step 1: Append failing tests**

Append to `cmd/kg/batch_cmd_test.go`:

```go
func TestBatchContinueOnErrorReportsFailures(t *testing.T) {
	db := freshDB(t)
	stream := strings.Join([]string{
		`{"op":"domain.add","args":{"id":"a","layers":["l1"]}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"good"}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"!!!"}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"alsoGood"}}`,
	}, "\n") + "\n"

	stdout, _, exit := execBatchCmd(t, db, stream, "--continue-on-error")
	require.NotEqual(t, 0, exit, "any failure causes nonzero exit")

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Applied int `json:"applied"`
			Failed  int `json:"failed"`
		} `json:"data"`
		Failures []struct {
			Line int    `json:"line"`
			Op   string `json:"op"`
			Code string `json:"code"`
		} `json:"failures"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &env))
	require.False(t, env.OK)
	require.Equal(t, 3, env.Data.Applied, "good ops should commit")
	require.Equal(t, 1, env.Data.Failed)
	require.Len(t, env.Failures, 1)
	require.Equal(t, 3, env.Failures[0].Line, "the bad name is on the 3rd op (lines start at 1)")
}

func TestBatchContinueOnErrorAllSuccessReturnsOK(t *testing.T) {
	db := freshDB(t)
	stream := `{"op":"domain.add","args":{"id":"a","layers":["l1"]}}` + "\n"
	stdout, _, exit := execBatchCmd(t, db, stream, "--continue-on-error")
	require.Equal(t, 0, exit)
	require.Contains(t, stdout, `"ok": true`)
	require.NotContains(t, stdout, "failures")
}

func TestBatchChunkAndContinueMutuallyExclusive(t *testing.T) {
	db := freshDB(t)
	_, _, exit := execBatchCmd(t, db, "", "--continue-on-error", "--chunk-size", "10")
	require.NotEqual(t, 0, exit)
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg/ -run TestBatchContinueOnError -v
```

Expected: FAIL.

- [ ] **Step 3: Extend `cmd/kg/batch_cmd.go`**

Replace `newBatchCmd` with:

```go
type batchOpts struct {
	continueOnError bool
	chunkSize       int
	dryRun          bool
	progress        bool
}

func newBatchCmd(c *cliCtx) *cobra.Command {
	opts := &batchOpts{}
	cmd := &cobra.Command{
		Use:   "batch",
		Short: "Apply a JSONL stream of operations atomically",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.continueOnError && opts.chunkSize > 0 {
				return errors.New("--continue-on-error and --chunk-size are mutually exclusive")
			}
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			return runBatch(cmd.Context(), c.stdout, c.stderr, os.Stdin, svc, *opts)
		},
	}
	cmd.Flags().BoolVar(&opts.continueOnError, "continue-on-error", false, "keep applying ops on failure; final envelope lists failures")
	cmd.Flags().IntVar(&opts.chunkSize, "chunk-size", 0, "commit a transaction every N ops (0 = single transaction)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "validate without committing")
	cmd.Flags().BoolVar(&opts.progress, "progress", false, "log progress to stderr roughly every 100ms")
	return cmd
}
```

Replace the `runBatch` signature and body with:

```go
type batchFailure struct {
	Line    int    `json:"line"`
	Op      string `json:"op"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type batchEnvelope struct {
	Applied int  `json:"applied"`
	Skipped int  `json:"skipped"`
	Failed  int  `json:"failed,omitempty"`
	TookMs  int  `json:"took_ms"`
	DryRun  bool `json:"dry_run,omitempty"`
}

func runBatch(ctx context.Context, stdout io.Writer, stderr io.Writer, stdin io.Reader, svc *graph.Service, opts batchOpts) error {
	ops, lines, parseErr := drainStream(stdin, stderr)
	if parseErr != nil {
		writeErr(stdout, "INVALID_OP", parseErr.Error(), "")
		return parseErrSentinel{parseErr}
	}

	start := time.Now()
	switch {
	case opts.continueOnError:
		return runContinue(ctx, stdout, svc, ops, lines, start)
	default:
		return runAtomic(ctx, stdout, svc, ops, start)
	}
}

func runAtomic(ctx context.Context, stdout io.Writer, svc *graph.Service, ops []batch.Op, start time.Time) error {
	env := batchEnvelope{}
	txErr := svc.InTx(ctx, func(ctx context.Context) error {
		for _, op := range ops {
			applied, err := applyOp(ctx, svc, op)
			if err != nil {
				return err
			}
			if applied {
				env.Applied++
			} else {
				env.Skipped++
			}
		}
		return nil
	})
	if txErr != nil {
		return txErr
	}
	env.TookMs = int(time.Since(start).Milliseconds())
	return writeOK(stdout, env)
}

func runContinue(ctx context.Context, stdout io.Writer, svc *graph.Service, ops []batch.Op, lines []int, start time.Time) error {
	env := batchEnvelope{}
	failures := []batchFailure{}
	for i, op := range ops {
		applied, err := applyOp(ctx, svc, op)
		if err != nil {
			m := mapError(err)
			failures = append(failures, batchFailure{
				Line: lines[i], Op: string(op.Op), Code: m.code, Message: m.message,
			})
			env.Failed++
			continue
		}
		if applied {
			env.Applied++
		} else {
			env.Skipped++
		}
	}
	env.TookMs = int(time.Since(start).Milliseconds())
	if env.Failed == 0 {
		return writeOK(stdout, env)
	}
	return writeBatchPartial(stdout, env, failures)
}

func writeBatchPartial(w io.Writer, env batchEnvelope, failures []batchFailure) error {
	body := struct {
		OK       bool           `json:"ok"`
		Data     batchEnvelope  `json:"data"`
		Error    *envErr        `json:"error"`
		Failures []batchFailure `json:"failures"`
	}{
		OK:       false,
		Data:     env,
		Error:    &envErr{Code: "BATCH_PARTIAL", Message: fmt.Sprintf("%d ops failed; see failures[]", env.Failed)},
		Failures: failures,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(body); err != nil {
		return err
	}
	return errExitOne
}

var errExitOne = errors.New("batch partial failure")
```

Update `drainStream` to also return per-op line numbers so we can attribute failures:

```go
func drainStream(r io.Reader, stderr io.Writer) ([]batch.Op, []int, error) {
	d := batch.NewDecoder(r)
	var ops []batch.Op
	var lines []int
	for {
		op, err := d.Next()
		if errors.Is(err, io.EOF) {
			return ops, lines, nil
		}
		if err != nil {
			return nil, nil, err
		}
		if op.Op == batch.OpMeta {
			var m batch.MetaArgs
			_ = json.Unmarshal(op.Args, &m)
			fmt.Fprintf(stderr, "meta: plugin=%q total_ops=%d\n", m.Plugin, m.TotalOps)
			continue
		}
		ops = append(ops, op)
		lines = append(lines, d.Line())
	}
}
```

In `cmd/kg/errmap.go`, append a case so `errExitOne` and `parseErrSentinel` map to exit 1 without re-wrapping the message:

```go
	case errors.Is(err, errExitOne):
		return mapped{1, "BATCH_PARTIAL", "", ""}
	case errors.As(err, new(parseErrSentinel)):
		return mapped{1, "INVALID_OP", err.Error(), ""}
```

(Place these branches *above* the generic `default`. The empty messages prevent double-output — the envelope was already written by `writeBatchPartial` / `runBatch`'s parse-error path.)

In `cmd/kg/root.go`, suppress the second envelope emission for these sentinels. Modify `run` to inspect for them before calling `writeErr`:

```go
	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		m := mapError(err)
		if m.code == "BATCH_PARTIAL" || (m.code == "INVALID_OP" && m.message == "") {
			return m.exit
		}
		_ = writeErr(stdout, m.code, m.message, m.hint)
		return m.exit
	}
```

(`INVALID_OP` with `message == ""` distinguishes "envelope already emitted" from "real cobra error".)

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg/ -run TestBatch -v
```

Expected: PASS for the three new tests plus all earlier batch tests.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg/
git commit -m "feat(cli): add --continue-on-error to kg batch with per-op failure envelope"
```

---

### Task 6: `--chunk-size N` flag

Commits every N successfully-applied ops in its own transaction. A failure inside a chunk rolls back only that chunk; earlier chunks remain committed. Failure halts the batch (it's not `--continue-on-error`).

**Files:**
- Modify: `cmd/kg/batch_cmd.go`
- Modify: `cmd/kg/batch_cmd_test.go`

- [ ] **Step 1: Append failing test**

Append to `cmd/kg/batch_cmd_test.go`:

```go
func TestBatchChunkSizeCommitsEarlierChunks(t *testing.T) {
	db := freshDB(t)
	stream := strings.Join([]string{
		`{"op":"domain.add","args":{"id":"a","layers":["l1"]}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"a"}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"b"}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"!!!"}}`,
	}, "\n") + "\n"

	_, _, exit := execBatchCmd(t, db, stream, "--chunk-size", "2")
	require.NotEqual(t, 0, exit)

	// Chunk 1 (domain.add + node.add a) commits. Chunk 2 (node.add b + bad) rolls back.
	var out, errOut bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "node", "list", "--domain", "a"}, &out, &errOut))
	require.Contains(t, out.String(), `"a:a"`)
	require.NotContains(t, out.String(), `"a:b"`)
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg/ -run TestBatchChunkSizeCommits -v
```

Expected: FAIL (chunking not implemented).

- [ ] **Step 3: Add chunked path to `runBatch`**

Replace the `switch` in `runBatch` with:

```go
	switch {
	case opts.continueOnError:
		return runContinue(ctx, stdout, svc, ops, lines, start)
	case opts.chunkSize > 0:
		return runChunked(ctx, stdout, svc, ops, opts.chunkSize, start)
	default:
		return runAtomic(ctx, stdout, svc, ops, start)
	}
```

Add `runChunked`:

```go
func runChunked(ctx context.Context, stdout io.Writer, svc *graph.Service, ops []batch.Op, chunkSize int, start time.Time) error {
	env := batchEnvelope{}
	for i := 0; i < len(ops); i += chunkSize {
		end := i + chunkSize
		if end > len(ops) {
			end = len(ops)
		}
		chunk := ops[i:end]
		txErr := svc.InTx(ctx, func(ctx context.Context) error {
			for _, op := range chunk {
				applied, err := applyOp(ctx, svc, op)
				if err != nil {
					return err
				}
				if applied {
					env.Applied++
				} else {
					env.Skipped++
				}
			}
			return nil
		})
		if txErr != nil {
			env.TookMs = int(time.Since(start).Milliseconds())
			return txErr
		}
	}
	env.TookMs = int(time.Since(start).Milliseconds())
	return writeOK(stdout, env)
}
```

(Note: when `runChunked` returns `txErr`, applied/skipped counters reflect only fully-committed earlier chunks because they are incremented after `InTx` returns nil — but here they're incremented inside the closure. Adjust: only commit the counters on successful tx by deferring them. Replace the closure with:)

```go
		var chunkApplied, chunkSkipped int
		txErr := svc.InTx(ctx, func(ctx context.Context) error {
			chunkApplied, chunkSkipped = 0, 0
			for _, op := range chunk {
				applied, err := applyOp(ctx, svc, op)
				if err != nil {
					return err
				}
				if applied {
					chunkApplied++
				} else {
					chunkSkipped++
				}
			}
			return nil
		})
		if txErr != nil {
			env.TookMs = int(time.Since(start).Milliseconds())
			return txErr
		}
		env.Applied += chunkApplied
		env.Skipped += chunkSkipped
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg/ -run TestBatch -v
```

Expected: PASS for the new test and all existing batch tests.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg/batch_cmd.go
git commit -m "feat(cli): add --chunk-size to kg batch for incremental commits"
```

---

### Task 7: `--dry-run` flag

Runs the full stream inside `Store.InTx` and returns a sentinel error to force rollback. On success, emits `{"ok": true, "data": {"dry_run": true, "would_apply": N, "would_skip": M}}`.

**Files:**
- Modify: `cmd/kg/batch_cmd.go`
- Modify: `cmd/kg/batch_cmd_test.go`

- [ ] **Step 1: Append failing test**

Append to `cmd/kg/batch_cmd_test.go`:

```go
func TestBatchDryRunDoesNotCommit(t *testing.T) {
	db := freshDB(t)
	stream := `{"op":"domain.add","args":{"id":"a","layers":["l1"]}}` + "\n"
	stdout, _, exit := execBatchCmd(t, db, stream, "--dry-run")
	require.Equal(t, 0, exit)
	require.Contains(t, stdout, `"dry_run": true`)
	require.Contains(t, stdout, `"would_apply": 1`)

	var out, errOut bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "domain", "list"}, &out, &errOut))
	require.Contains(t, out.String(), `"data": []`)
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg/ -run TestBatchDryRun -v
```

Expected: FAIL.

- [ ] **Step 3: Add dry-run path**

In `cmd/kg/batch_cmd.go`, extend the switch:

```go
	switch {
	case opts.dryRun:
		return runDryRun(ctx, stdout, svc, ops, start)
	case opts.continueOnError:
		return runContinue(ctx, stdout, svc, ops, lines, start)
	case opts.chunkSize > 0:
		return runChunked(ctx, stdout, svc, ops, opts.chunkSize, start)
	default:
		return runAtomic(ctx, stdout, svc, ops, start)
	}
```

Add `runDryRun`:

```go
type dryRunResult struct {
	DryRun     bool `json:"dry_run"`
	WouldApply int  `json:"would_apply"`
	WouldSkip  int  `json:"would_skip"`
	TookMs     int  `json:"took_ms"`
}

func runDryRun(ctx context.Context, stdout io.Writer, svc *graph.Service, ops []batch.Op, start time.Time) error {
	var applied, skipped int
	sentinel := errors.New("dry-run rollback")
	err := svc.InTx(ctx, func(ctx context.Context) error {
		for _, op := range ops {
			a, err := applyOp(ctx, svc, op)
			if err != nil {
				return err
			}
			if a {
				applied++
			} else {
				skipped++
			}
		}
		return sentinel
	})
	if errors.Is(err, sentinel) {
		return writeOK(stdout, dryRunResult{
			DryRun: true, WouldApply: applied, WouldSkip: skipped,
			TookMs: int(time.Since(start).Milliseconds()),
		})
	}
	return err
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg/ -run TestBatch -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg/batch_cmd.go
git commit -m "feat(cli): add --dry-run to kg batch via sentinel rollback"
```

---

### Task 8: `--progress` flag + final stream parse error polish

`--progress` emits `applied N/total` lines to stderr at most every 100ms (using `meta.total_ops` as the denominator if present; otherwise just "applied N"). The denominator is captured from the meta op by `drainStream` — extend its signature.

**Files:**
- Modify: `cmd/kg/batch_cmd.go`
- Modify: `cmd/kg/batch_cmd_test.go`

- [ ] **Step 1: Append failing test**

Append to `cmd/kg/batch_cmd_test.go`:

```go
func TestBatchProgressEmitsToStderr(t *testing.T) {
	db := freshDB(t)
	stream := strings.Join([]string{
		`{"op":"meta","args":{"total_ops":2}}`,
		`{"op":"domain.add","args":{"id":"a","layers":["l1"]}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"x"}}`,
	}, "\n") + "\n"

	_, stderr, exit := execBatchCmd(t, db, stream, "--progress")
	require.Equal(t, 0, exit)
	require.Contains(t, stderr, "applied")
	require.Contains(t, stderr, "/2")
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg/ -run TestBatchProgress -v
```

Expected: FAIL.

- [ ] **Step 3: Wire progress through drainStream and runAtomic**

Change `drainStream`'s return to include `total`:

```go
func drainStream(r io.Reader, stderr io.Writer) (ops []batch.Op, lines []int, total int64, err error) {
	d := batch.NewDecoder(r)
	for {
		op, derr := d.Next()
		if errors.Is(derr, io.EOF) {
			return ops, lines, total, nil
		}
		if derr != nil {
			return nil, nil, 0, derr
		}
		if op.Op == batch.OpMeta {
			var m batch.MetaArgs
			_ = json.Unmarshal(op.Args, &m)
			fmt.Fprintf(stderr, "meta: plugin=%q total_ops=%d\n", m.Plugin, m.TotalOps)
			if m.TotalOps > 0 {
				total = m.TotalOps
			}
			continue
		}
		ops = append(ops, op)
		lines = append(lines, d.Line())
	}
}
```

Update `runBatch` to thread `total` and `progress` through:

```go
	ops, lines, total, parseErr := drainStream(stdin, stderr)
	if parseErr != nil {
		writeErr(stdout, "INVALID_OP", parseErr.Error(), "")
		return parseErrSentinel{parseErr}
	}
	var prog *progressTicker
	if opts.progress {
		prog = newProgressTicker(stderr, total)
	}

	switch {
	case opts.dryRun:
		return runDryRun(ctx, stdout, svc, ops, start)
	case opts.continueOnError:
		return runContinue(ctx, stdout, svc, ops, lines, start, prog)
	case opts.chunkSize > 0:
		return runChunked(ctx, stdout, svc, ops, opts.chunkSize, start, prog)
	default:
		return runAtomic(ctx, stdout, svc, ops, start, prog)
	}
```

Add the ticker:

```go
type progressTicker struct {
	w     io.Writer
	total int64
	last  time.Time
}

func newProgressTicker(w io.Writer, total int64) *progressTicker {
	return &progressTicker{w: w, total: total, last: time.Now().Add(-time.Hour)}
}

func (p *progressTicker) tick(applied int) {
	if p == nil {
		return
	}
	now := time.Now()
	if now.Sub(p.last) < 100*time.Millisecond {
		return
	}
	p.last = now
	if p.total > 0 {
		fmt.Fprintf(p.w, "applied %d/%d\n", applied, p.total)
	} else {
		fmt.Fprintf(p.w, "applied %d\n", applied)
	}
}

func (p *progressTicker) flush(applied int) {
	if p == nil {
		return
	}
	if p.total > 0 {
		fmt.Fprintf(p.w, "applied %d/%d\n", applied, p.total)
	} else {
		fmt.Fprintf(p.w, "applied %d\n", applied)
	}
}
```

Update each runner to accept `prog` and call `prog.tick(env.Applied)` after each `applied++` and `prog.flush(env.Applied)` once at the end. For `runAtomic`:

```go
func runAtomic(ctx context.Context, stdout io.Writer, svc *graph.Service, ops []batch.Op, start time.Time, prog *progressTicker) error {
	env := batchEnvelope{}
	txErr := svc.InTx(ctx, func(ctx context.Context) error {
		for _, op := range ops {
			applied, err := applyOp(ctx, svc, op)
			if err != nil {
				return err
			}
			if applied {
				env.Applied++
				prog.tick(env.Applied)
			} else {
				env.Skipped++
			}
		}
		return nil
	})
	if txErr != nil {
		return txErr
	}
	prog.flush(env.Applied)
	env.TookMs = int(time.Since(start).Milliseconds())
	return writeOK(stdout, env)
}
```

Apply the analogous changes to `runChunked` and `runContinue`. `runDryRun` does not need progress (the spec says dry-run does not emit progress).

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg/ -run TestBatch -v
```

Expected: PASS for all batch tests including the new progress test.

- [ ] **Step 5: Run the full test suite**

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/kg/batch_cmd.go
git commit -m "feat(cli): add --progress flag to kg batch with 100ms throttle"
```

---

## Phase 3 — `cmd/kg-extractor/` dispatcher (pure Go, root module)

A new binary that discovers plugins under `~/.config/kg-extractor/plugins/<name>/` (overridable via `KG_EXTRACTOR_PLUGINS_PATH`), parses their `manifest.json`, invokes them as subprocesses, validates each JSONL line they emit via `batch/`, and forwards to `kg batch` (when `--db` is set) or to its own stdout (pass-through). Stays pure Go — only `plugins/tree-sitter/` brings in CGO.

### Task 9: kg-extractor cobra root and main

**Files:**
- Create: `cmd/kg-extractor/main.go`
- Create: `cmd/kg-extractor/root.go`
- Create: `cmd/kg-extractor/root_test.go`

- [ ] **Step 1: Write failing test for the root command**

Create `cmd/kg-extractor/root_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootHelpListsSubcommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := run([]string{"--help"}, &stdout, &stderr)
	require.Equal(t, 0, exit)
	out := stdout.String() + stderr.String()
	for _, sub := range []string{"list", "info", "extract"} {
		require.Contains(t, out, sub)
	}
}

func TestUnknownSubcommandReportsError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := run([]string{"bogus"}, &stdout, &stderr)
	require.NotEqual(t, 0, exit)
	require.True(t, strings.Contains(stdout.String(), "INVALID_INPUT") || strings.Contains(stderr.String(), "unknown command"))
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg-extractor/ -v
```

Expected: FAIL (package does not exist).

- [ ] **Step 3: Implement `main.go` and `root.go`**

Create `cmd/kg-extractor/main.go`:

```go
package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
```

Create `cmd/kg-extractor/root.go`:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

type cliCtx struct {
	pluginsPath string
	stdout      io.Writer
	stderr      io.Writer
}

func newRootCmd(c *cliCtx) *cobra.Command {
	root := &cobra.Command{
		Use:           "kg-extractor",
		Short:         "kg-extractor — discover and dispatch kg extractor plugins",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVar(&c.pluginsPath, "plugins-path", envOr("KG_EXTRACTOR_PLUGINS_PATH", defaultPluginsPath()), "colon-separated plugin discovery path")
	root.AddCommand(newListCmd(c), newInfoCmd(c), newExtractCmd(c))
	return root
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func defaultPluginsPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return fmt.Sprintf("%s/.config/kg-extractor/plugins", home)
	}
	return ""
}

func run(args []string, stdout, stderr io.Writer) int {
	c := &cliCtx{stdout: stdout, stderr: stderr}
	cmd := newRootCmd(c)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		if errors.Is(err, errEnvelopeAlreadyWritten) {
			return 1
		}
		writeErr(stdout, "INVALID_INPUT", err.Error(), "")
		return 1
	}
	return 0
}

var errEnvelopeAlreadyWritten = errors.New("envelope already written")

func newListCmd(c *cliCtx) *cobra.Command    { return newListCmdReal(c) }
func newInfoCmd(c *cliCtx) *cobra.Command    { return newInfoCmdReal(c) }
func newExtractCmd(c *cliCtx) *cobra.Command { return newExtractCmdReal(c) }
```

Create `cmd/kg-extractor/output.go` (the same envelope helpers as kg, duplicated to keep modules independent):

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
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(envelope{OK: true, Data: data})
}

func writeErr(w io.Writer, code, message, hint string) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(envelope{OK: false, Error: &envErr{Code: code, Message: message, Hint: hint}})
}
```

Create placeholder stubs so the build passes — Tasks 12, 13, 15 will overwrite them:

`cmd/kg-extractor/list_cmd.go`:

```go
package main

import "github.com/spf13/cobra"

func newListCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List discoverable plugins", RunE: func(*cobra.Command, []string) error { return nil }}
}
```

`cmd/kg-extractor/info_cmd.go`:

```go
package main

import "github.com/spf13/cobra"

func newInfoCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{Use: "info <name>", Short: "Show plugin info", RunE: func(*cobra.Command, []string) error { return nil }}
}
```

`cmd/kg-extractor/extract_cmd.go`:

```go
package main

import "github.com/spf13/cobra"

func newExtractCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{Use: "extract", Short: "Run a plugin and emit ops", RunE: func(*cobra.Command, []string) error { return nil }}
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg-extractor/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg-extractor/
git commit -m "feat(extractor): scaffold cobra root with list/info/extract stubs"
```

---

### Task 10: Manifest parser

**Files:**
- Create: `cmd/kg-extractor/manifest.go`
- Create: `cmd/kg-extractor/manifest_test.go`

- [ ] **Step 1: Write failing tests**

Create `cmd/kg-extractor/manifest_test.go`:

```go
package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseManifestNative(t *testing.T) {
	path := writeManifest(t, `{
		"name": "tree-sitter",
		"version": "0.1.0",
		"description": "...",
		"runtime": "native",
		"executable": "kg-extractor-tree-sitter"
	}`)
	m, err := parseManifest(path)
	require.NoError(t, err)
	require.Equal(t, "tree-sitter", m.Name)
	require.Equal(t, runtimeNative, m.Runtime)
	require.Equal(t, "kg-extractor-tree-sitter", m.Executable)
}

func TestParseManifestCommand(t *testing.T) {
	path := writeManifest(t, `{
		"name": "bash-demo",
		"version": "0.1.0",
		"description": "...",
		"runtime": "command",
		"command": ["bash", "extract.sh"]
	}`)
	m, err := parseManifest(path)
	require.NoError(t, err)
	require.Equal(t, []string{"bash", "extract.sh"}, m.Command)
}

func TestParseManifestRejectsBadName(t *testing.T) {
	path := writeManifest(t, `{"name":"Bad Name","version":"0.1.0","runtime":"native","executable":"x","description":"x"}`)
	_, err := parseManifest(path)
	require.ErrorContains(t, err, "name")
}

func TestParseManifestRejectsUnknownRuntime(t *testing.T) {
	path := writeManifest(t, `{"name":"x","version":"0.1.0","runtime":"docker","description":"x"}`)
	_, err := parseManifest(path)
	require.ErrorContains(t, err, "runtime")
}

func TestParseManifestWASMReserved(t *testing.T) {
	path := writeManifest(t, `{"name":"x","version":"0.1.0","runtime":"wasm","module":"x.wasm","description":"x"}`)
	m, err := parseManifest(path)
	require.NoError(t, err)
	require.Equal(t, runtimeWASM, m.Runtime)
}

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "manifest.json")
	require.NoError(t, writeFile(p, body))
	return p
}

func writeFile(path, body string) error {
	return os_WriteFile(path, []byte(body), 0o644)
}
```

Append a tiny indirection at the bottom of `cmd/kg-extractor/manifest_test.go`:

```go
import "os"

func os_WriteFile(path string, data []byte, mode os.FileMode) error {
	return os.WriteFile(path, data, mode)
}
```

(The indirection lets us avoid an extra import block reshuffle when we later want to monkey it; in the final implementation file we use `os.WriteFile` directly.)

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg-extractor/ -run ParseManifest -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `manifest.go`**

Create `cmd/kg-extractor/manifest.go`:

```go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
)

type runtimeKind string

const (
	runtimeNative  runtimeKind = "native"
	runtimeCommand runtimeKind = "command"
	runtimeWASM    runtimeKind = "wasm"
)

type manifest struct {
	Name             string      `json:"name"`
	Version          string      `json:"version"`
	Description      string      `json:"description"`
	Runtime          runtimeKind `json:"runtime"`
	Executable       string      `json:"executable,omitempty"`
	Command          []string    `json:"command,omitempty"`
	Module           string      `json:"module,omitempty"`
	SupportedLayers  []string    `json:"supported_layers,omitempty"`
	SupportedLanguages []string  `json:"supported_languages,omitempty"`
}

var pluginNameRE = regexp.MustCompile(`^[a-z0-9-]+$`)

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
		return nil, fmt.Errorf("invalid plugin name %q (must match ^[a-z0-9-]+$)", m.Name)
	}
	if m.Version == "" {
		return nil, errors.New("manifest version is required")
	}
	if m.Description == "" {
		return nil, errors.New("manifest description is required")
	}
	switch m.Runtime {
	case runtimeNative:
		if m.Executable == "" {
			return nil, errors.New("native runtime requires executable")
		}
	case runtimeCommand:
		if len(m.Command) == 0 {
			return nil, errors.New("command runtime requires command[]")
		}
	case runtimeWASM:
		if m.Module == "" {
			return nil, errors.New("wasm runtime requires module")
		}
	default:
		return nil, fmt.Errorf("unknown runtime %q", m.Runtime)
	}
	return &m, nil
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg-extractor/ -run ParseManifest -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg-extractor/
git commit -m "feat(extractor): parse plugin manifest.json with runtime validation"
```

---

### Task 11: Plugin discovery

Walks the colon-separated `--plugins-path`. Each directory entry is a candidate plugin dir; reads its `manifest.json`. Invalid manifests are reported as errors but don't abort discovery.

**Files:**
- Create: `cmd/kg-extractor/discovery.go`
- Create: `cmd/kg-extractor/discovery_test.go`

- [ ] **Step 1: Write failing test**

Create `cmd/kg-extractor/discovery_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoverFindsValidPlugins(t *testing.T) {
	root := t.TempDir()
	mkPlugin(t, root, "good", `{"name":"good","version":"0.1.0","description":"x","runtime":"command","command":["echo"]}`)
	mkPlugin(t, root, "bad", `{"name":"BAD","version":"0.1.0","description":"x","runtime":"native","executable":"x"}`)
	mkPlugin(t, root, "another", `{"name":"another","version":"0.1.0","description":"x","runtime":"native","executable":"a"}`)

	plugins, errs := discoverPlugins(root)
	names := pluginNames(plugins)
	require.ElementsMatch(t, []string{"good", "another"}, names)
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Error(), "bad")
}

func TestDiscoverMultipleDirs(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	mkPlugin(t, a, "x", `{"name":"x","version":"0.1.0","description":"x","runtime":"native","executable":"x"}`)
	mkPlugin(t, b, "y", `{"name":"y","version":"0.1.0","description":"x","runtime":"native","executable":"y"}`)

	plugins, _ := discoverPlugins(a + string(os.PathListSeparator) + b)
	require.ElementsMatch(t, []string{"x", "y"}, pluginNames(plugins))
}

func TestDiscoverIgnoresMissingDirs(t *testing.T) {
	plugins, errs := discoverPlugins(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Empty(t, plugins)
	require.Empty(t, errs)
}

func mkPlugin(t *testing.T, root, name, manifest string) {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644))
}

func pluginNames(ps []discoveredPlugin) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Manifest.Name)
	}
	// Sorted-equality is the caller's responsibility; we leave order as discovered.
	_ = strings.Join(out, ",")
	return out
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg-extractor/ -run Discover -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `discovery.go`**

Create `cmd/kg-extractor/discovery.go`:

```go
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type discoveredPlugin struct {
	Dir      string
	Manifest *manifest
}

func discoverPlugins(path string) ([]discoveredPlugin, []error) {
	var plugins []discoveredPlugin
	var errs []error
	for _, root := range splitPath(path) {
		entries, err := os.ReadDir(root)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			errs = append(errs, fmt.Errorf("read %s: %w", root, err))
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(root, entry.Name())
			m, err := parseManifest(filepath.Join(dir, "manifest.json"))
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
				continue
			}
			plugins = append(plugins, discoveredPlugin{Dir: dir, Manifest: m})
		}
	}
	return plugins, errs
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	out := []string{}
	cur := ""
	for _, r := range path {
		if r == os.PathListSeparator {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg-extractor/ -run Discover -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg-extractor/
git commit -m "feat(extractor): discover plugins under colon-separated KG_EXTRACTOR_PLUGINS_PATH"
```

---

### Task 12: `list` and `info` subcommands

**Files:**
- Modify: `cmd/kg-extractor/list_cmd.go`
- Modify: `cmd/kg-extractor/info_cmd.go`
- Create: `cmd/kg-extractor/list_cmd_test.go`

- [ ] **Step 1: Write failing tests**

Create `cmd/kg-extractor/list_cmd_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListSubcommand(t *testing.T) {
	root := t.TempDir()
	mkPlugin(t, root, "alpha", `{"name":"alpha","version":"0.1.0","description":"first","runtime":"native","executable":"a"}`)
	mkPlugin(t, root, "beta", `{"name":"beta","version":"0.2.0","description":"second","runtime":"command","command":["bash","x.sh"]}`)

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--plugins-path", root, "list"}, &stdout, &stderr)
	require.Equal(t, 0, exit, "stderr=%s", stderr.String())

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Plugins []struct {
				Name        string `json:"name"`
				Version     string `json:"version"`
				Runtime     string `json:"runtime"`
				Description string `json:"description"`
			} `json:"plugins"`
			Errors []string `json:"errors"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &env))
	require.True(t, env.OK)
	require.Len(t, env.Data.Plugins, 2)
}

func TestInfoSubcommandFindsPlugin(t *testing.T) {
	root := t.TempDir()
	mkPlugin(t, root, "alpha", `{"name":"alpha","version":"0.1.0","description":"first","runtime":"native","executable":"a"}`)

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--plugins-path", root, "info", "alpha"}, &stdout, &stderr)
	require.Equal(t, 0, exit, "stderr=%s", stderr.String())
	require.Contains(t, stdout.String(), `"name": "alpha"`)
}

func TestInfoSubcommandMissingPluginErrors(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	exit := run([]string{"--plugins-path", root, "info", "ghost"}, &stdout, &stderr)
	require.NotEqual(t, 0, exit)
	require.Contains(t, stdout.String(), "PLUGIN_NOT_FOUND")
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg-extractor/ -run 'TestList|TestInfo' -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `list_cmd.go`**

Overwrite `cmd/kg-extractor/list_cmd.go`:

```go
package main

import (
	"github.com/spf13/cobra"
)

type listPlugin struct {
	Name              string   `json:"name"`
	Version           string   `json:"version"`
	Runtime           string   `json:"runtime"`
	Description       string   `json:"description"`
	SupportedLayers   []string `json:"supported_layers,omitempty"`
	SupportedLanguages []string `json:"supported_languages,omitempty"`
}

type listResult struct {
	Plugins []listPlugin `json:"plugins"`
	Errors  []string     `json:"errors,omitempty"`
}

func newListCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List discoverable plugins",
		RunE: func(*cobra.Command, []string) error {
			plugins, errs := discoverPlugins(c.pluginsPath)
			out := listResult{}
			for _, p := range plugins {
				out.Plugins = append(out.Plugins, listPlugin{
					Name: p.Manifest.Name, Version: p.Manifest.Version,
					Runtime: string(p.Manifest.Runtime), Description: p.Manifest.Description,
					SupportedLayers: p.Manifest.SupportedLayers, SupportedLanguages: p.Manifest.SupportedLanguages,
				})
			}
			for _, e := range errs {
				out.Errors = append(out.Errors, e.Error())
			}
			return writeOK(c.stdout, out)
		},
	}
}
```

Overwrite `cmd/kg-extractor/info_cmd.go`:

```go
package main

import (
	"github.com/spf13/cobra"
)

func newInfoCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Args:  cobra.ExactArgs(1),
		Short: "Show plugin info",
		RunE: func(_ *cobra.Command, args []string) error {
			plugins, _ := discoverPlugins(c.pluginsPath)
			for _, p := range plugins {
				if p.Manifest.Name == args[0] {
					return writeOK(c.stdout, p.Manifest)
				}
			}
			writeErr(c.stdout, "PLUGIN_NOT_FOUND", "plugin "+args[0]+" not found", "run `kg-extractor list`")
			return errEnvelopeAlreadyWritten
		},
	}
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg-extractor/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg-extractor/
git commit -m "feat(extractor): add list and info subcommands"
```

---

### Task 13: Plugin invocation (subprocess)

Spawns the plugin, writes the JSON config to its stdin, reads stdout line-by-line, forwards stderr to the caller's stderr, returns the captured op stream and an error if the plugin exited non-zero or emitted invalid JSON.

**Files:**
- Create: `cmd/kg-extractor/invoke.go`
- Create: `cmd/kg-extractor/invoke_test.go`

- [ ] **Step 1: Write failing tests using bash inline scripts**

Create `cmd/kg-extractor/invoke_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustBash(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
}

func TestInvokeCapturesStdoutOps(t *testing.T) {
	mustBash(t)
	dir := t.TempDir()
	script := `#!/usr/bin/env bash
set -e
cat > /dev/null
echo '{"op":"meta","args":{"plugin":"t"}}'
echo '{"op":"domain.add","args":{"id":"a","layers":["l1"]}}'
`
	scriptPath := filepath.Join(dir, "extract.sh")
	require.NoError(t, writeFile(scriptPath, script))
	require.NoError(t, exec.Command("chmod", "+x", scriptPath).Run())

	m := &manifest{Name: "t", Runtime: runtimeCommand, Command: []string{"bash", scriptPath}}
	cfg := pluginConfig{Input: "/x", Domain: "a", ProtocolVersion: 1}

	var stderr bytes.Buffer
	stream, err := invokePlugin(context.Background(), discoveredPlugin{Dir: dir, Manifest: m}, cfg, &stderr)
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(stream.String(), "\n"))
	require.Contains(t, stream.String(), "domain.add")
}

func TestInvokePropagatesNonZeroExit(t *testing.T) {
	mustBash(t)
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "extract.sh")
	require.NoError(t, writeFile(scriptPath, `#!/usr/bin/env bash
cat > /dev/null
echo "bad" >&2
exit 7
`))
	require.NoError(t, exec.Command("chmod", "+x", scriptPath).Run())

	m := &manifest{Name: "t", Runtime: runtimeCommand, Command: []string{"bash", scriptPath}}
	var stderr bytes.Buffer
	_, err := invokePlugin(context.Background(), discoveredPlugin{Dir: dir, Manifest: m}, pluginConfig{ProtocolVersion: 1}, &stderr)
	require.Error(t, err)
	require.Contains(t, stderr.String(), "bad")
}

func TestInvokeRejectsWASM(t *testing.T) {
	m := &manifest{Name: "w", Runtime: runtimeWASM, Module: "x.wasm"}
	_, err := invokePlugin(context.Background(), discoveredPlugin{Manifest: m}, pluginConfig{ProtocolVersion: 1}, &bytes.Buffer{})
	require.ErrorContains(t, err, "WASM_NOT_SUPPORTED")
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg-extractor/ -run TestInvoke -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `invoke.go`**

Create `cmd/kg-extractor/invoke.go`:

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
)

type pluginConfig struct {
	Input           string         `json:"input,omitempty"`
	Domain          string         `json:"domain,omitempty"`
	ProtocolVersion int            `json:"protocol_version"`
	Config          map[string]any `json:"config,omitempty"`
}

func invokePlugin(ctx context.Context, p discoveredPlugin, cfg pluginConfig, stderr io.Writer) (*bytes.Buffer, error) {
	cmd, err := buildPluginCommand(ctx, p)
	if err != nil {
		return nil, err
	}

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	cmd.Stdin = bytes.NewReader(configJSON)
	cmd.Stderr = stderr

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("plugin %q failed: %w", p.Manifest.Name, err)
	}
	return &stdout, nil
}

func buildPluginCommand(ctx context.Context, p discoveredPlugin) (*exec.Cmd, error) {
	switch p.Manifest.Runtime {
	case runtimeNative:
		exe := p.Manifest.Executable
		if !filepath.IsAbs(exe) {
			exe = filepath.Join(p.Dir, exe)
		}
		return exec.CommandContext(ctx, exe), nil
	case runtimeCommand:
		if len(p.Manifest.Command) == 0 {
			return nil, errors.New("plugin command[] is empty")
		}
		cmd := exec.CommandContext(ctx, p.Manifest.Command[0], p.Manifest.Command[1:]...)
		cmd.Dir = p.Dir
		return cmd, nil
	case runtimeWASM:
		return nil, errors.New("WASM_NOT_SUPPORTED: wasm runtime is reserved for a future release")
	}
	return nil, fmt.Errorf("unknown runtime %q", p.Manifest.Runtime)
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg-extractor/ -run TestInvoke -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg-extractor/
git commit -m "feat(extractor): invoke plugins via subprocess with config on stdin"
```

---

### Task 14: JSONL validation pipeline

Re-uses `batch/`'s Decoder to validate every line the plugin emitted. Required-args validation per op type lives here (the dispatcher gatekeeps before forwarding).

**Files:**
- Create: `cmd/kg-extractor/validator.go`
- Create: `cmd/kg-extractor/validator_test.go`

- [ ] **Step 1: Write failing tests**

Create `cmd/kg-extractor/validator_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateStreamHappyPath(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"op":"meta","args":{"plugin":"x"}}`,
		`{"op":"domain.add","args":{"id":"a","layers":["l"]}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l","name":"n"}}`,
	}, "\n") + "\n")

	var out bytes.Buffer
	require.NoError(t, validateStream(in, &out))
	require.Equal(t, 3, strings.Count(out.String(), "\n"))
}

func TestValidateStreamRejectsMissingArg(t *testing.T) {
	in := strings.NewReader(`{"op":"node.add","args":{"domain":"a","name":"n"}}` + "\n")
	var out bytes.Buffer
	err := validateStream(in, &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "layer")
}

func TestValidateStreamRejectsBadJSON(t *testing.T) {
	in := strings.NewReader("garbage\n")
	var out bytes.Buffer
	err := validateStream(in, &out)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg-extractor/ -run TestValidate -v
```

Expected: FAIL.

- [ ] **Step 3: Implement `validator.go`**

Create `cmd/kg-extractor/validator.go`:

```go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/ggfarmco/kg/batch"
)

func validateStream(in io.Reader, out io.Writer) error {
	d := batch.NewDecoder(in)
	enc := batch.NewEncoder(out)
	for {
		op, err := d.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := validateOp(op, d.Line()); err != nil {
			return err
		}
		switch op.Op {
		case batch.OpMeta:
			var a batch.MetaArgs
			_ = json.Unmarshal(op.Args, &a)
			if err := enc.Meta(a); err != nil {
				return err
			}
		default:
			if _, werr := out.Write(append(rawOpLine(op), '\n')); werr != nil {
				return werr
			}
		}
	}
}

func rawOpLine(op batch.Op) []byte {
	b, _ := json.Marshal(op)
	return b
}

func validateOp(op batch.Op, line int) error {
	switch op.Op {
	case batch.OpMeta:
		return nil
	case batch.OpDomainAdd:
		var a batch.DomainAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return fmt.Errorf("line %d: domain.add args: %w", line, err)
		}
		if a.ID == "" || len(a.Layers) == 0 {
			return fmt.Errorf("line %d: domain.add requires id and layers", line)
		}
	case batch.OpNodeAdd:
		var a batch.NodeAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return fmt.Errorf("line %d: node.add args: %w", line, err)
		}
		if a.Domain == "" || a.Layer == "" || a.Name == "" {
			return fmt.Errorf("line %d: node.add requires domain, layer, name", line)
		}
	case batch.OpNodeUpdate:
		var a batch.NodeUpdateArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return fmt.Errorf("line %d: node.update args: %w", line, err)
		}
		if a.ID == "" {
			return fmt.Errorf("line %d: node.update requires id", line)
		}
	case batch.OpNodeDelete:
		var a batch.NodeDeleteArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return fmt.Errorf("line %d: node.delete args: %w", line, err)
		}
		if a.ID == "" {
			return fmt.Errorf("line %d: node.delete requires id", line)
		}
	case batch.OpEdgeAdd:
		var a batch.EdgeAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return fmt.Errorf("line %d: edge.add args: %w", line, err)
		}
		if a.Source == "" || a.Target == "" || a.Type == "" {
			return fmt.Errorf("line %d: edge.add requires source, target, type", line)
		}
	case batch.OpEdgeDelete:
		var a batch.EdgeDeleteArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return fmt.Errorf("line %d: edge.delete args: %w", line, err)
		}
		if a.ID == 0 {
			return fmt.Errorf("line %d: edge.delete requires id", line)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg-extractor/ -run TestValidate -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg-extractor/
git commit -m "feat(extractor): validate plugin stream with per-op required-arg checks"
```

---

### Task 15: `extract` subcommand — pass-through mode (no `--db`)

When `--db` is not set, kg-extractor writes the validated stream to its own stdout (for piping to a separate `kg batch` or further `jq` processing).

**Files:**
- Modify: `cmd/kg-extractor/extract_cmd.go`
- Create: `cmd/kg-extractor/extract_cmd_test.go`

- [ ] **Step 1: Write failing test**

Create `cmd/kg-extractor/extract_cmd_test.go`:

```go
package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractPassThrough(t *testing.T) {
	mustBash(t)
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	require.NoError(t, exec_(t, "mkdir", "-p", dir))
	require.NoError(t, writeFile(filepath.Join(dir, "manifest.json"), `{"name":"demo","version":"0.1.0","description":"x","runtime":"command","command":["bash","extract.sh"]}`))
	require.NoError(t, writeFile(filepath.Join(dir, "extract.sh"), `#!/usr/bin/env bash
cat > /dev/null
echo '{"op":"domain.add","args":{"id":"a","layers":["l"]}}'
`))
	require.NoError(t, exec.Command("chmod", "+x", filepath.Join(dir, "extract.sh")).Run())

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--plugins-path", root, "extract", "--plugin", "demo", "--input", "/x", "--domain", "a"}, &stdout, &stderr)
	require.Equal(t, 0, exit, "stderr=%s", stderr.String())
	require.Contains(t, stdout.String(), "domain.add")
}

func exec_(t *testing.T, name string, args ...string) error {
	t.Helper()
	return exec.Command(name, args...).Run()
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg-extractor/ -run TestExtractPassThrough -v
```

Expected: FAIL.

- [ ] **Step 3: Overwrite `extract_cmd.go`**

```go
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

type extractOpts struct {
	plugin     string
	input      string
	domain     string
	language   string
	configJSON string
	configFile string
	dbPath     string
	kgBinary   string
	quiet      bool
}

func newExtractCmdReal(c *cliCtx) *cobra.Command {
	opts := &extractOpts{}
	cmd := &cobra.Command{
		Use:   "extract",
		Short: "Run a plugin and forward its ops to kg batch (or stdout)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExtract(cmd.Context(), c, opts)
		},
	}
	cmd.Flags().StringVar(&opts.plugin, "plugin", "", "plugin name (required)")
	cmd.Flags().StringVar(&opts.input, "input", "", "plugin-specific input path")
	cmd.Flags().StringVar(&opts.domain, "domain", "", "kg domain id")
	cmd.Flags().StringVar(&opts.language, "language", "", "language hint forwarded into config.language")
	cmd.Flags().StringVar(&opts.configJSON, "config-json", "", "inline JSON forwarded into plugin config")
	cmd.Flags().StringVar(&opts.configFile, "config-file", "", "path to JSON file forwarded into plugin config")
	cmd.Flags().StringVar(&opts.dbPath, "db", "", "if set, pipe ops to `kg --db <path> batch`")
	cmd.Flags().StringVar(&opts.kgBinary, "kg-binary", envOr("KG_BINARY", "kg"), "path to the kg binary (used when --db is set)")
	cmd.Flags().BoolVar(&opts.quiet, "quiet", false, "suppress plugin stderr forwarding")
	_ = cmd.MarkFlagRequired("plugin")
	return cmd
}

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

	cfg := pluginConfig{Input: opts.input, Domain: opts.domain, ProtocolVersion: 1, Config: map[string]any{}}
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
		body, err := readFile(opts.configFile)
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

	var validated bytes.Buffer
	if err := validateStream(raw, &validated); err != nil {
		return err
	}

	if opts.dbPath == "" {
		_, err := c.stdout.Write(validated.Bytes())
		return err
	}
	return errors.New("--db forwarding not yet implemented") // wired up in the next task
}
```

Add a small helper at the bottom (and the missing imports `context`, `io`, `os`):

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func readFile(p string) ([]byte, error) {
	return os.ReadFile(p)
}
```

(Replace the existing import block at the top of `extract_cmd.go` with this consolidated one — only one import block per file.)

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg-extractor/ -run TestExtractPassThrough -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg-extractor/
git commit -m "feat(extractor): add extract subcommand pass-through mode (no --db)"
```

---

### Task 16: `extract --db` forwarding to `kg batch`

Spawns `<kg-binary> --db <path> batch`, pipes the validated stream to its stdin, propagates exit code and final envelope.

**Files:**
- Modify: `cmd/kg-extractor/extract_cmd.go`
- Modify: `cmd/kg-extractor/extract_cmd_test.go`

- [ ] **Step 1: Append failing test**

Append to `cmd/kg-extractor/extract_cmd_test.go`:

```go
func TestExtractWithDBForwardsToKgBatch(t *testing.T) {
	mustBash(t)

	// Build kg into a tmp dir so we can point --kg-binary at it.
	kgPath := buildKg(t)
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "kg.db")
	require.NoError(t, exec.Command(kgPath, "--db", dbPath, "init").Run())

	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	require.NoError(t, exec.Command("mkdir", "-p", dir).Run())
	require.NoError(t, writeFile(filepath.Join(dir, "manifest.json"), `{"name":"demo","version":"0.1.0","description":"x","runtime":"command","command":["bash","extract.sh"]}`))
	require.NoError(t, writeFile(filepath.Join(dir, "extract.sh"), `#!/usr/bin/env bash
cat > /dev/null
echo '{"op":"domain.add","args":{"id":"a","layers":["l"]}}'
echo '{"op":"node.add","args":{"domain":"a","layer":"l","name":"n"}}'
`))
	require.NoError(t, exec.Command("chmod", "+x", filepath.Join(dir, "extract.sh")).Run())

	var stdout, stderr bytes.Buffer
	exit := run([]string{
		"--plugins-path", root, "extract",
		"--plugin", "demo", "--input", "/x", "--domain", "a",
		"--db", dbPath, "--kg-binary", kgPath,
	}, &stdout, &stderr)
	require.Equal(t, 0, exit, "stdout=%s stderr=%s", stdout.String(), stderr.String())

	// Verify DB state via kg node list.
	out, err := exec.Command(kgPath, "--db", dbPath, "node", "list", "--domain", "a").Output()
	require.NoError(t, err)
	require.Contains(t, string(out), "a:n")
}

func buildKg(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "kg")
	cmd := exec.Command("go", "build", "-o", out, "../kg")
	cmd.Stderr = bytes.NewBuffer(nil)
	if err := cmd.Run(); err != nil {
		t.Skipf("go build failed (likely no toolchain available): %v %s", err, cmd.Stderr)
	}
	return out
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./cmd/kg-extractor/ -run TestExtractWithDBForwards -v
```

Expected: FAIL (`--db` returns "not yet implemented").

- [ ] **Step 3: Implement the forwarding branch**

Replace the `if opts.dbPath == ""` block in `runExtract` with:

```go
	if opts.dbPath == "" {
		_, err := c.stdout.Write(validated.Bytes())
		return err
	}
	return forwardToKgBatch(ctx, c, opts, validated.Bytes())
```

Add at the bottom of `extract_cmd.go`:

```go
func forwardToKgBatch(ctx context.Context, c *cliCtx, opts *extractOpts, stream []byte) error {
	cmd := exec_command(ctx, opts.kgBinary, "--db", opts.dbPath, "batch")
	cmd.Stdin = bytes.NewReader(stream)
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kg batch: %w", err)
	}
	return nil
}
```

And add an indirection so tests can swap (`exec_command` keeps `os/exec` import already there):

```go
var exec_command = func(ctx context.Context, name string, args ...string) *execCmdShim {
	return execCmdShimFromOSExec(ctx, name, args...)
}

type execCmdShim = exec.Cmd

func execCmdShimFromOSExec(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
```

Add `"os/exec"` to the import block.

(The shim looks like overkill but lets a future tighter test stub `kg batch` without spawning a real binary; for v1 the e2e test in Phase 9 also exercises this path with a real `kg` binary.)

- [ ] **Step 4: Run, verify pass**

```bash
go test ./cmd/kg-extractor/ -v
```

Expected: PASS for all extractor tests including the new forwarding test.

- [ ] **Step 5: Commit**

```bash
git add cmd/kg-extractor/
git commit -m "feat(extractor): forward validated stream to kg batch when --db is set"
```

---

## Phase 4 — `bash-demo` plugin and contract sanity test

A tiny bash plugin proving the contract works without compiled code or CGO. Also serves as a regression target: anything that breaks bash-demo breaks a known-good baseline.

### Task 17: bash-demo plugin files

**Files:**
- Create: `examples/kg-extractor-plugins/bash-demo/manifest.json`
- Create: `examples/kg-extractor-plugins/bash-demo/extract.sh`
- Create: `examples/kg-extractor-plugins/bash-demo/README.md`

- [ ] **Step 1: Create the plugin directory and manifest**

```bash
mkdir -p examples/kg-extractor-plugins/bash-demo
```

Create `examples/kg-extractor-plugins/bash-demo/manifest.json`:

```json
{
  "name": "bash-demo",
  "version": "0.1.0",
  "description": "Trivial bash plugin emitting a fixed mini-graph for contract testing",
  "runtime": "command",
  "command": ["bash", "extract.sh"],
  "supported_layers": ["root", "item"]
}
```

- [ ] **Step 2: Create the extraction script**

Create `examples/kg-extractor-plugins/bash-demo/extract.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
config=$(cat)
domain=$(echo "$config" | jq -r '.domain')
cat <<EOF
{"op":"meta","args":{"plugin":"bash-demo","version":"0.1.0","total_ops":5}}
{"op":"domain.add","args":{"id":"$domain","layers":["root","item"],"if_not_exists":true}}
{"op":"node.add","args":{"domain":"$domain","layer":"root","name":"Demo","if_not_exists":true}}
{"op":"node.add","args":{"domain":"$domain","layer":"item","name":"First","parent":"$domain:demo","if_not_exists":true}}
{"op":"node.add","args":{"domain":"$domain","layer":"item","name":"Second","parent":"$domain:demo","if_not_exists":true}}
{"op":"edge.add","args":{"source":"$domain:demo-first","target":"$domain:demo-second","type":"references","if_not_exists":true}}
EOF
```

Make it executable:

```bash
chmod +x examples/kg-extractor-plugins/bash-demo/extract.sh
```

- [ ] **Step 3: Document the plugin**

Create `examples/kg-extractor-plugins/bash-demo/README.md`:

```markdown
# bash-demo

A 10-line bash plugin that emits a fixed mini-graph. Demonstrates that the
kg-extractor plugin contract works without compiled code or CGO.

Requires `bash` and `jq`.

## Try it

```sh
ln -s "$(pwd)" ~/.config/kg-extractor/plugins/bash-demo
kg-extractor extract --plugin bash-demo --domain example | head
```

## What it produces

- a domain with layers `[root, item]`
- one root node `Demo`
- two item nodes `First`, `Second` parented at `Demo`
- one `references` edge from `First` to `Second`
```

- [ ] **Step 4: Commit**

```bash
git add examples/
git commit -m "feat(extractor): add bash-demo plugin example"
```

---

### Task 18: End-to-end integration test via bash-demo

Spawns `kg-extractor extract --db ...` against bash-demo (which spawns `bash`), verifies kg's DB ends up with the expected nodes and edges.

**Files:**
- Create: `cmd/kg-extractor/integration_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/kg-extractor/integration_test.go`:

```go
package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBashDemoEndToEnd(t *testing.T) {
	mustBash(t)
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}

	kgPath := buildKg(t)
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "kg.db")
	require.NoError(t, exec.Command(kgPath, "--db", dbPath, "init").Run())

	pluginsRoot := t.TempDir()
	require.NoError(t, exec.Command("cp", "-R", "../../examples/kg-extractor-plugins/bash-demo", filepath.Join(pluginsRoot, "bash-demo")).Run())

	var stdout, stderr bytes.Buffer
	exit := run([]string{
		"--plugins-path", pluginsRoot, "extract",
		"--plugin", "bash-demo", "--input", "/x", "--domain", "demoapp",
		"--db", dbPath, "--kg-binary", kgPath,
	}, &stdout, &stderr)
	require.Equal(t, 0, exit, "stdout=%s stderr=%s", stdout.String(), stderr.String())

	nodeList, err := exec.Command(kgPath, "--db", dbPath, "node", "list", "--domain", "demoapp").Output()
	require.NoError(t, err)
	require.Contains(t, string(nodeList), "demoapp:demo")
	require.Contains(t, string(nodeList), "demoapp:demo-first")
	require.Contains(t, string(nodeList), "demoapp:demo-second")

	edges, err := exec.Command(kgPath, "--db", dbPath, "edge", "list-from", "demoapp:demo-first").Output()
	require.NoError(t, err)
	require.Contains(t, string(edges), "demoapp:demo-second")
	require.Contains(t, string(edges), "references")
}
```

- [ ] **Step 2: Run, verify pass**

```bash
go test ./cmd/kg-extractor/ -run TestBashDemoEndToEnd -v
```

Expected: PASS (assuming bash + jq + go toolchain are present; otherwise SKIP).

- [ ] **Step 3: Commit**

```bash
git add cmd/kg-extractor/integration_test.go
git commit -m "test(extractor): add bash-demo end-to-end pipeline test"
```

---

## Phase 5 — Multi-module workspace for the tree-sitter plugin

Sets up `plugins/tree-sitter/` as a separate Go module with its own `go.mod`/`go.sum` (CGO deps live there only). Adds a `go.work` at the repo root so local dev resolves `github.com/ggfarmco/kg` to the checkout. CI exports `GOWORK=off` to test plugins as external consumers.

### Task 19: Create the plugin module skeleton + go.work

**Files:**
- Create: `plugins/tree-sitter/go.mod`
- Create: `plugins/tree-sitter/main.go`
- Create: `go.work` (repo root)

- [ ] **Step 1: Create the plugin directory and module**

```bash
mkdir -p plugins/tree-sitter
```

Create `plugins/tree-sitter/go.mod`:

```
module github.com/ggfarmco/kg/plugins/tree-sitter

go 1.26

replace github.com/ggfarmco/kg => ../..

require (
	github.com/ggfarmco/kg v0.0.0-00010101000000-000000000000
	github.com/smacker/go-tree-sitter v0.0.0-20240827094217-dd81d9e9be82
	github.com/spf13/cobra v1.9.1
	github.com/stretchr/testify v1.9.0
)
```

(Pin a known-good `smacker/go-tree-sitter` revision; the exact hash above is a placeholder — when running `Step 3` below, `go mod tidy` will resolve and lock to the current latest. Commit the resolved versions.)

- [ ] **Step 2: Create a minimal `main.go` so the module builds**

Create `plugins/tree-sitter/main.go`:

```go
package main

import "fmt"

func main() {
	fmt.Println("kg-extractor-tree-sitter: not yet implemented")
}
```

- [ ] **Step 3: Initialize go.work at the repo root and tidy**

Create `go.work`:

```
go 1.26

use (
	.
	./plugins/tree-sitter
)
```

Then:

```bash
go -C ./plugins/tree-sitter mod tidy
```

This populates `plugins/tree-sitter/go.sum` with the resolved versions.

- [ ] **Step 4: Verify both modules build**

```bash
go build ./...
go -C ./plugins/tree-sitter build ./...
```

Expected: both succeed.

- [ ] **Step 5: Verify GOWORK=off still works for the plugin (external-consumer simulation)**

```bash
GOWORK=off go -C ./plugins/tree-sitter build ./...
```

Expected: succeeds (because the `replace` directive in `plugins/tree-sitter/go.mod` still points at `../..`).

- [ ] **Step 6: Commit**

```bash
git add go.work plugins/tree-sitter/
git commit -m "chore(plugin-tree-sitter): scaffold separate Go module with CGO deps and go.work"
```

---

### Task 20: Tree-sitter cobra root with `--language` flag and registry

**Files:**
- Create: `plugins/tree-sitter/root.go`
- Create: `plugins/tree-sitter/registry.go`
- Create: `plugins/tree-sitter/registry_test.go`
- Modify: `plugins/tree-sitter/main.go`

- [ ] **Step 1: Write failing test for the registry**

Create `plugins/tree-sitter/registry_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryLookupAndRegister(t *testing.T) {
	r := newRegistry()
	require.Nil(t, r.lookup("nosuch"))
	r.register(&fakeLang{id: "fakelang"})
	got := r.lookup("fakelang")
	require.NotNil(t, got)
	require.Equal(t, "fakelang", got.ID())
}

type fakeLang struct{ id string }

func (f *fakeLang) ID() string                                          { return f.id }
func (f *fakeLang) FileExtensions() []string                            { return []string{".fake"} }
func (f *fakeLang) Extract(ctx *extractCtx, file fileInfo) error        { return nil }
func (f *fakeLang) ResolveCalls(ctx *extractCtx, pkg *packageInfo) error { return nil }
```

- [ ] **Step 2: Run, verify failure**

```bash
go -C ./plugins/tree-sitter test -run TestRegistry -v ./...
```

Expected: FAIL.

- [ ] **Step 3: Implement `registry.go` and supporting types**

Create `plugins/tree-sitter/registry.go`:

```go
package main

import "sync"

type fileInfo struct {
	AbsPath     string
	RelPath     string
	PackagePath string
	Source      []byte
}

type packageInfo struct {
	Path     string
	Slug     string
	Files    []fileInfo
	DeclByID map[string]struct{}
}

type extractCtx struct {
	Domain   string
	Packages map[string]*packageInfo
}

type Language interface {
	ID() string
	FileExtensions() []string
	Extract(ctx *extractCtx, file fileInfo) error
	ResolveCalls(ctx *extractCtx, pkg *packageInfo) error
}

type registry struct {
	mu    sync.Mutex
	langs map[string]Language
}

func newRegistry() *registry { return &registry{langs: map[string]Language{}} }

func (r *registry) register(l Language) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.langs[l.ID()] = l
}

func (r *registry) lookup(id string) Language {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.langs[id]
}

func (r *registry) ids() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.langs))
	for id := range r.langs {
		out = append(out, id)
	}
	return out
}

var defaultRegistry = newRegistry()
```

Create `plugins/tree-sitter/root.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

type cliCtx struct {
	stdout io.Writer
	stderr io.Writer
}

type stdinConfig struct {
	Input           string         `json:"input"`
	Domain          string         `json:"domain"`
	ProtocolVersion int            `json:"protocol_version"`
	Config          map[string]any `json:"config"`
}

func newRootCmd(c *cliCtx) *cobra.Command {
	var language string
	cmd := &cobra.Command{
		Use:           "kg-extractor-tree-sitter",
		Short:         "kg-extractor-tree-sitter — extract structure from source code via tree-sitter",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := readStdinConfig(os.Stdin)
			if err != nil {
				return err
			}
			if cfg.ProtocolVersion != 1 {
				return fmt.Errorf("unsupported protocol_version %d", cfg.ProtocolVersion)
			}
			if language == "" {
				if v, ok := cfg.Config["language"].(string); ok {
					language = v
				}
			}
			if language == "" {
				return errors.New("--language not set and config.language missing")
			}
			lang := defaultRegistry.lookup(language)
			if lang == nil {
				return fmt.Errorf("LANGUAGE_NOT_SUPPORTED: %q (registered: %v)", language, defaultRegistry.ids())
			}
			return runExtraction(cmd.Context(), c.stdout, c.stderr, lang, cfg)
		},
	}
	cmd.Flags().StringVar(&language, "language", "", "language id (e.g. go); falls back to config.language")
	return cmd
}

func readStdinConfig(r io.Reader) (stdinConfig, error) {
	var cfg stdinConfig
	body, err := io.ReadAll(r)
	if err != nil {
		return cfg, fmt.Errorf("read stdin: %w", err)
	}
	if len(body) == 0 {
		return cfg, errors.New("empty stdin: kg-extractor must send the JSON config")
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return cfg, fmt.Errorf("parse stdin config: %w", err)
	}
	return cfg, nil
}

func runExtraction(ctx context.Context, stdout io.Writer, stderr io.Writer, lang Language, cfg stdinConfig) error {
	return errors.New("runExtraction wired up in Phase 6")
}

func run(args []string, stdout, stderr io.Writer) int {
	c := &cliCtx{stdout: stdout, stderr: stderr}
	cmd := newRootCmd(c)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
```

Overwrite `plugins/tree-sitter/main.go`:

```go
package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go -C ./plugins/tree-sitter test ./...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add plugins/tree-sitter/
git commit -m "feat(plugin-tree-sitter): scaffold cobra root with --language dispatch and registry"
```

---

## Phase 6 — Tree-sitter shared layer (language-agnostic)

Slug sanitization, directory walker, and op emission helpers. None of these reference a specific grammar — they orchestrate whichever `Language` the registry returns.

### Task 21: Slug sanitization

Per spec: lowercase, replace `/_.` and any non-`[a-z0-9-]` with `-`, collapse repeats, trim leading/trailing `-`. Returns "" if the result is empty (caller emits a stderr warning and skips).

**Files:**
- Create: `plugins/tree-sitter/slug.go`
- Create: `plugins/tree-sitter/slug_test.go`

- [ ] **Step 1: Write failing tests**

Create `plugins/tree-sitter/slug_test.go`:

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"internal/graph", "internal-graph"},
		{"Node.go", "node-go"},
		{"tree-sitter", "tree-sitter"},
		{"__init__", "init"},
		{"My Class!", "my-class"},
		{"   ", ""},
		{"123abc", "123abc"},
		{"a---b", "a-b"},
		{"camelCase", "camelcase"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, sanitizeSlug(tc.in), "input=%q", tc.in)
	}
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go -C ./plugins/tree-sitter test -run TestSanitize -v ./...
```

Expected: FAIL.

- [ ] **Step 3: Implement `slug.go`**

Create `plugins/tree-sitter/slug.go`:

```go
package main

import "strings"

func sanitizeSlug(in string) string {
	var b strings.Builder
	b.Grow(len(in))
	prevDash := false
	for _, r := range strings.ToLower(in) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || r == '/' || r == '.':
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return out
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go -C ./plugins/tree-sitter test -run TestSanitize -v ./...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add plugins/tree-sitter/
git commit -m "feat(plugin-tree-sitter): add slug sanitization helper"
```

---

### Task 22: Directory walker

Walks `cfg.Input` recursively. Skips `vendor/`, `.git/`, `node_modules/`. For each file matching the language's `FileExtensions()`, reads source and bundles into the matching `packageInfo` (a package is one directory containing `.go` files, identified by relative path).

**Files:**
- Create: `plugins/tree-sitter/walker.go`
- Create: `plugins/tree-sitter/walker_test.go`

- [ ] **Step 1: Write failing test**

Create `plugins/tree-sitter/walker_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWalkPackages(t *testing.T) {
	root := t.TempDir()
	mkFile(t, root, "a/x.go", "package a\n")
	mkFile(t, root, "a/y.go", "package a\n")
	mkFile(t, root, "a/sub/z.go", "package sub\n")
	mkFile(t, root, "vendor/skip/v.go", "package skip\n")
	mkFile(t, root, ".git/skip/g.go", "package skip\n")
	mkFile(t, root, "a/x_test.go", "package a\n")

	pkgs, err := walkPackages(root, []string{".go"}, true)
	require.NoError(t, err)
	paths := pathsOf(pkgs)
	require.ElementsMatch(t, []string{"a", "a/sub"}, paths)

	pkgA := findPkg(pkgs, "a")
	require.Len(t, pkgA.Files, 2, "test file excluded")
}

func TestWalkPackagesIncludesTestFiles(t *testing.T) {
	root := t.TempDir()
	mkFile(t, root, "a/x.go", "package a\n")
	mkFile(t, root, "a/x_test.go", "package a\n")

	pkgs, err := walkPackages(root, []string{".go"}, false)
	require.NoError(t, err)
	require.Len(t, findPkg(pkgs, "a").Files, 2)
}

func mkFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func pathsOf(pkgs []*packageInfo) []string {
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, p.Path)
	}
	return out
}

func findPkg(pkgs []*packageInfo, path string) *packageInfo {
	for _, p := range pkgs {
		if p.Path == path {
			return p
		}
	}
	return nil
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go -C ./plugins/tree-sitter test -run TestWalk -v ./...
```

Expected: FAIL.

- [ ] **Step 3: Implement `walker.go`**

Create `plugins/tree-sitter/walker.go`:

```go
package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var skipDirs = map[string]bool{
	"vendor":       true,
	".git":         true,
	"node_modules": true,
}

func walkPackages(root string, extensions []string, skipTests bool) ([]*packageInfo, error) {
	byPath := map[string]*packageInfo{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !hasAnyExt(d.Name(), extensions) {
			return nil
		}
		if skipTests && strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		pkgPath := filepath.ToSlash(filepath.Dir(rel))
		if pkgPath == "." {
			pkgPath = filepath.Base(root)
		}
		pkg, ok := byPath[pkgPath]
		if !ok {
			pkg = &packageInfo{
				Path:     pkgPath,
				Slug:     sanitizeSlug(pkgPath),
				DeclByID: map[string]struct{}{},
			}
			byPath[pkgPath] = pkg
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		pkg.Files = append(pkg.Files, fileInfo{
			AbsPath:     path,
			RelPath:     filepath.ToSlash(rel),
			PackagePath: pkgPath,
			Source:      src,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]*packageInfo, 0, len(byPath))
	for _, p := range byPath {
		sort.Slice(p.Files, func(i, j int) bool { return p.Files[i].RelPath < p.Files[j].RelPath })
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func hasAnyExt(name string, exts []string) bool {
	for _, e := range exts {
		if strings.HasSuffix(name, e) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run, verify pass**

```bash
go -C ./plugins/tree-sitter test -run TestWalk -v ./...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add plugins/tree-sitter/
git commit -m "feat(plugin-tree-sitter): add directory walker with skip-dirs and test-file filter"
```

---

### Task 23: Op emission via `batch/` Encoder + `runExtraction` wiring

Top-level extraction loop: build `extractCtx`, walk packages, for each file call `lang.Extract`, emit `domain.add` + per-package/file/decl `node.add` ops, then for each package call `lang.ResolveCalls` and emit `edge.add` ops. Buffers all ops into memory (per v1 spec — streaming is a v1.x optimization), then writes them via `batch.Encoder` to stdout, prefixed by a `meta` op.

Languages communicate emitted decls back to the orchestrator via `packageInfo.Files[].Decls` and per-package `Imports`/`Calls` lists. Extend `packageInfo` and `fileInfo` accordingly.

**Files:**
- Modify: `plugins/tree-sitter/registry.go` (extend types)
- Create: `plugins/tree-sitter/emit.go`
- Create: `plugins/tree-sitter/emit_test.go`

- [ ] **Step 1: Extend the registry types**

In `plugins/tree-sitter/registry.go`, replace the `fileInfo`, `packageInfo`, and `extractCtx` types with:

```go
type Decl struct {
	NameSlug   string
	Properties map[string]any
}

type Import struct {
	From string
	To   string
}

type Call struct {
	FromDecl string
	ToDecl   string
}

type fileInfo struct {
	AbsPath     string
	RelPath     string
	BasenameSlug string
	PackagePath string
	Source      []byte
	Decls       []Decl
}

type packageInfo struct {
	Path     string
	Slug     string
	Files    []fileInfo
	DeclByID map[string]struct{}
	Imports  []Import
	Calls    []Call
	Properties map[string]any
}

type extractCtx struct {
	Domain   string
	Packages map[string]*packageInfo
}
```

Update the walker to populate `BasenameSlug`. In `walker.go`, replace the `pkg.Files = append(pkg.Files, fileInfo{...})` block with:

```go
		base := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		pkg.Files = append(pkg.Files, fileInfo{
			AbsPath:      path,
			RelPath:      filepath.ToSlash(rel),
			BasenameSlug: sanitizeSlug(base),
			PackagePath:  pkgPath,
			Source:       src,
		})
```

Update `fakeLang` in `registry_test.go` to match the extended interface (no change needed if the signatures still match — the test fake is shape-only). Verify by running the existing tests:

```bash
go -C ./plugins/tree-sitter test ./...
```

If `registry_test.go`'s `fakeLang` no longer compiles, sync its method signatures.

- [ ] **Step 2: Write failing test for emission**

Create `plugins/tree-sitter/emit_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmitProducesOrderedOps(t *testing.T) {
	pkg := &packageInfo{
		Path:     "a",
		Slug:     "a",
		DeclByID: map[string]struct{}{},
		Properties: map[string]any{"import_path": "x/a"},
		Files: []fileInfo{
			{
				BasenameSlug: "x-go",
				RelPath:      "a/x.go",
				Decls: []Decl{
					{NameSlug: "foo", Properties: map[string]any{"kind": "function"}},
				},
			},
		},
		Imports: []Import{{From: "a", To: "b"}},
		Calls:   []Call{},
	}
	pkgB := &packageInfo{Path: "b", Slug: "b", DeclByID: map[string]struct{}{}}

	var buf bytes.Buffer
	require.NoError(t, emitOps(&buf, "lang", "demo", []*packageInfo{pkg, pkgB}))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 6)
	require.Contains(t, lines[0], `"meta"`)
	require.Contains(t, lines[1], `"domain.add"`)
	// First non-meta/domain op should be a node.add for a package.
	require.Contains(t, lines[2], `"node.add"`)
	require.Contains(t, lines[2], `"layer":"package"`)
	// File and decl ops should follow.
	require.Contains(t, buf.String(), `"layer":"file"`)
	require.Contains(t, buf.String(), `"layer":"decl"`)
	// imports edge appears after all node ops.
	require.Contains(t, buf.String(), `"imports"`)
}
```

- [ ] **Step 3: Run, verify failure**

```bash
go -C ./plugins/tree-sitter test -run TestEmit -v ./...
```

Expected: FAIL.

- [ ] **Step 4: Implement `emit.go`**

Create `plugins/tree-sitter/emit.go`:

```go
package main

import (
	"fmt"
	"io"
	"sort"

	"github.com/ggfarmco/kg/batch"
)

func emitOps(w io.Writer, language, domain string, pkgs []*packageInfo) error {
	enc := batch.NewEncoder(w)

	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Path < pkgs[j].Path })

	var total int64
	total += 1 + 1 // meta + domain
	for _, p := range pkgs {
		total++
		for _, f := range p.Files {
			total++
			total += int64(len(f.Decls))
		}
		total += int64(len(p.Imports))
		total += int64(len(p.Calls))
	}

	if err := enc.Meta(batch.MetaArgs{Plugin: "tree-sitter", Language: language, TotalOps: total}); err != nil {
		return err
	}
	if err := enc.DomainAdd(batch.DomainAddArgs{
		ID:          domain,
		Layers:      []string{"package", "file", "decl"},
		Description: "Extracted by kg-extractor-tree-sitter (" + language + ")",
		IfNotExists: true,
	}); err != nil {
		return err
	}

	for _, p := range pkgs {
		pkgID := domain + ":" + p.Slug
		if err := enc.NodeAdd(batch.NodeAddArgs{
			Domain: domain, Layer: "package", Name: p.Path, ID: p.Slug,
			Properties: nonNilMap(p.Properties), IfNotExists: true,
		}); err != nil {
			return err
		}
		for _, f := range p.Files {
			fileSlug := p.Slug + "/" + f.BasenameSlug
			if err := enc.NodeAdd(batch.NodeAddArgs{
				Domain: domain, Layer: "file", Name: f.RelPath, ID: fileSlug,
				Parent: pkgID,
				IfNotExists: true,
			}); err != nil {
				return err
			}
			for _, d := range f.Decls {
				declSlug := fileSlug + "::" + d.NameSlug
				if err := enc.NodeAdd(batch.NodeAddArgs{
					Domain: domain, Layer: "decl", Name: d.NameSlug, ID: declSlug,
					Parent:     domain + ":" + fileSlug,
					Properties: nonNilMap(d.Properties),
					IfNotExists: true,
				}); err != nil {
					return err
				}
			}
		}
	}

	for _, p := range pkgs {
		for _, imp := range p.Imports {
			if err := enc.EdgeAdd(batch.EdgeAddArgs{
				Source: domain + ":" + sanitizeSlug(imp.From),
				Target: domain + ":" + sanitizeSlug(imp.To),
				Type:   "imports",
				IfNotExists: true,
			}); err != nil {
				return err
			}
		}
	}
	for _, p := range pkgs {
		for _, call := range p.Calls {
			if err := enc.EdgeAdd(batch.EdgeAddArgs{
				Source: domain + ":" + call.FromDecl,
				Target: domain + ":" + call.ToDecl,
				Type:   "calls",
				IfNotExists: true,
			}); err != nil {
				return err
			}
		}
	}

	if total < 0 {
		return fmt.Errorf("negative total: %d", total)
	}
	return nil
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	return m
}
```

Now wire `runExtraction` in `plugins/tree-sitter/root.go`. Replace the stub with:

```go
func runExtraction(ctx context.Context, stdout io.Writer, stderr io.Writer, lang Language, cfg stdinConfig) error {
	skipTests := true
	if v, ok := cfg.Config["skip_test_files"].(bool); ok {
		skipTests = v
	}
	pkgs, err := walkPackages(cfg.Input, lang.FileExtensions(), skipTests)
	if err != nil {
		return fmt.Errorf("walk: %w", err)
	}
	ec := &extractCtx{Domain: cfg.Domain, Packages: map[string]*packageInfo{}}
	for _, p := range pkgs {
		ec.Packages[p.Path] = p
	}
	for _, p := range pkgs {
		for _, f := range p.Files {
			if err := lang.Extract(ec, f); err != nil {
				fmt.Fprintf(stderr, "extract %s: %v\n", f.RelPath, err)
			}
		}
	}
	for _, p := range pkgs {
		if err := lang.ResolveCalls(ec, p); err != nil {
			fmt.Fprintf(stderr, "resolve calls %s: %v\n", p.Path, err)
		}
	}
	for _, p := range pkgs {
		updated := ec.Packages[p.Path]
		if updated != nil {
			*p = *updated
		}
	}
	return emitOps(stdout, lang.ID(), cfg.Domain, pkgs)
}
```

(Move `"context"`, `"io"` into the existing stdlib import block; the file already imports them.)

- [ ] **Step 5: Run, verify pass**

```bash
go -C ./plugins/tree-sitter test ./...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add plugins/tree-sitter/
git commit -m "feat(plugin-tree-sitter): emit batch ops and orchestrate language extraction"
```

---

## Phase 7 — Go language extractor

Registers the Go grammar with the registry and implements `Extract` (per file: walk top-level decls, populate `fileInfo.Decls` and `packageInfo.Imports`) and `ResolveCalls` (per package: resolve intra-package, unqualified calls to other decls in the same package). Modeled on Understand-Anything's `go-extractor.ts`.

### Task 24: Register Go grammar and helper

**Files:**
- Create: `plugins/tree-sitter/languages/golang/lang.go`
- Create: `plugins/tree-sitter/languages/golang/exported.go`
- Create: `plugins/tree-sitter/languages/golang/exported_test.go`
- Modify: `plugins/tree-sitter/main.go` (blank-import the language package)

- [ ] **Step 1: Write failing test for the `IsExported` helper**

Create `plugins/tree-sitter/languages/golang/exported_test.go`:

```go
package golang

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsExported(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"Foo", true},
		{"foo", false},
		{"", false},
		{"_Foo", false},
		{"ID", true},
		{"αlpha", false}, // tree-sitter Go identifiers are ASCII; non-ASCII = not exported in our model
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, IsExported(tc.in), "in=%q", tc.in)
	}
}
```

- [ ] **Step 2: Implement helpers and language registration**

Create `plugins/tree-sitter/languages/golang/exported.go`:

```go
package golang

func IsExported(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}
```

Create `plugins/tree-sitter/languages/golang/lang.go`:

```go
package golang

import (
	"context"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	tsgolang "github.com/smacker/go-tree-sitter/golang"
)

type Plugin interface {
	Register(register func(Language))
}

type Language interface {
	ID() string
	FileExtensions() []string
}

type GoLang struct {
	once   sync.Once
	parser *sitter.Parser
}

func (g *GoLang) ID() string               { return "go" }
func (g *GoLang) FileExtensions() []string { return []string{".go"} }

func (g *GoLang) parse(ctx context.Context, src []byte) (*sitter.Tree, error) {
	g.once.Do(func() {
		g.parser = sitter.NewParser()
		g.parser.SetLanguage(tsgolang.GetLanguage())
	})
	return g.parser.ParseCtx(ctx, nil, src)
}

func New() *GoLang { return &GoLang{} }
```

(`Plugin` and `Language` are private contracts inside this package; the orchestrator's `Language` interface in the parent `main` package is structurally compatible — we'll add the missing `Extract` and `ResolveCalls` methods in Task 25–27 and bind to it from a tiny adapter file.)

To wire registration, create `plugins/tree-sitter/languages/golang/register.go`:

```go
package golang

import "context"

type Registrar interface {
	Register(id string, ext []string,
		extract func(ctx context.Context, ec any, file any) error,
		resolveCalls func(ctx context.Context, ec any, pkg any) error,
	)
}
```

We will use a thin adapter in the parent package instead of having `golang` import the parent — keeping the dependency direction clean (orchestrator depends on language, not the reverse).

Adapter file `plugins/tree-sitter/lang_go_adapter.go`:

```go
package main

import (
	"context"

	"github.com/ggfarmco/kg/plugins/tree-sitter/languages/golang"
)

type goAdapter struct {
	g *golang.GoLang
}

func (a *goAdapter) ID() string               { return a.g.ID() }
func (a *goAdapter) FileExtensions() []string { return a.g.FileExtensions() }

func (a *goAdapter) Extract(ec *extractCtx, f fileInfo) error {
	return golang.Extract(context.Background(), a.g, asGoCtx(ec), asGoFile(f))
}

func (a *goAdapter) ResolveCalls(ec *extractCtx, p *packageInfo) error {
	return golang.ResolveCalls(context.Background(), a.g, asGoCtx(ec), asGoPkg(p))
}

func init() {
	defaultRegistry.register(&goAdapter{g: golang.New()})
}

// Bridge types — keep the language package decoupled from the orchestrator's
// internal types by passing thin shapes it can mutate via pointer.
type goCtx struct{ pkgs map[string]*goPkg }
type goPkg struct {
	source  *packageInfo
	files   []*goFile
	decls   map[string]struct{}
	imports []golang.Import
	calls   []golang.Call
}
type goFile struct {
	source *fileInfo
	decls  []golang.Decl
}

func asGoCtx(ec *extractCtx) *goCtx {
	out := &goCtx{pkgs: make(map[string]*goPkg, len(ec.Packages))}
	for path, p := range ec.Packages {
		gp := &goPkg{source: p, decls: map[string]struct{}{}}
		for i := range p.Files {
			gp.files = append(gp.files, &goFile{source: &p.Files[i]})
		}
		out.pkgs[path] = gp
	}
	return out
}

func asGoFile(f fileInfo) *goFile { return &goFile{source: &f} }
func asGoPkg(p *packageInfo) *goPkg {
	return &goPkg{source: p, decls: map[string]struct{}{}}
}
```

(The adapter is intentionally a bit verbose: it keeps the `languages/golang/` subpackage from ever importing the orchestrator's private types. If during implementation this proves too painful, collapse the language package back into the orchestrator package and skip the adapter — but losing the directory boundary loses the per-language file layout the spec calls for.)

Now `plugins/tree-sitter/languages/golang/lang.go` needs companion top-level functions `Extract` and `ResolveCalls`:

```go
func Extract(ctx context.Context, g *GoLang, gc *interface{}, f *interface{}) error {
	// implemented in Task 25–27
	return nil
}

func ResolveCalls(ctx context.Context, g *GoLang, gc *interface{}, p *interface{}) error {
	// implemented in Task 27
	return nil
}
```

(These will be replaced in subsequent tasks with real bodies; for now they're stubs so the registration compiles.)

In `plugins/tree-sitter/main.go`, blank-import the language package so its `init()` runs:

```go
package main

import (
	"os"

	_ "github.com/ggfarmco/kg/plugins/tree-sitter/languages/golang"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
```

Wait — the `_` import binds to the *adapter* file (which lives in package `main`), not the language package directly. The blank import isn't needed since the adapter file IS in `package main` and is compiled directly. Drop the `_` import; keep only `os`. The `init()` in `lang_go_adapter.go` runs automatically when the binary starts.

- [ ] **Step 3: Make the adapter's call signatures consistent**

Replace the placeholder signatures of `Extract` / `ResolveCalls` in `lang.go` with the real ones the adapter calls:

```go
func Extract(ctx context.Context, g *GoLang, gc *goCtxRef, f *goFileRef) error {
	return nil
}

func ResolveCalls(ctx context.Context, g *GoLang, gc *goCtxRef, p *goPkgRef) error {
	return nil
}
```

And introduce dummy types in `lang.go`:

```go
type (
	goCtxRef  = struct{}
	goFileRef = struct{}
	goPkgRef  = struct{}
)
```

(The adapter passes `*goCtx` / `*goFile` / `*goPkg` from package `main`. To avoid cross-package type plumbing, define a tiny mutual interface in the language package and let the adapter satisfy it.)

Better — drop the bridge types entirely. Use generics on `*GoLang`'s methods so the language package can pull data out of structures it doesn't import. In `lang.go`, replace the package-level `Extract` / `ResolveCalls` with methods on `*GoLang` that take a small interface:

```go
type FileSource interface {
	Bytes() []byte
	RelPath() string
}

type DeclSink interface {
	AddDecl(slug string, props map[string]any)
}

type ImportSink interface {
	AddImport(from, to string)
}

type CallSink interface {
	AddCall(fromDecl, toDecl string)
	HasDecl(slug string) bool
}

func (g *GoLang) ExtractFile(ctx context.Context, fs FileSource, ds DeclSink, is ImportSink) error {
	return nil
}

func (g *GoLang) ResolvePackage(ctx context.Context, fs []FileSource, cs CallSink) error {
	return nil
}
```

Then `plugins/tree-sitter/lang_go_adapter.go` becomes much smaller:

```go
package main

import (
	"context"

	"github.com/ggfarmco/kg/plugins/tree-sitter/languages/golang"
)

type goAdapter struct{ g *golang.GoLang }

func (a *goAdapter) ID() string               { return a.g.ID() }
func (a *goAdapter) FileExtensions() []string { return a.g.FileExtensions() }

func (a *goAdapter) Extract(ec *extractCtx, f fileInfo) error {
	pkg := ec.Packages[f.PackagePath]
	if pkg == nil {
		return nil
	}
	fa := &fileAccess{f: &f}
	da := &declAccess{pkg: pkg, file: &f}
	ia := &importAccess{pkg: pkg}
	if err := a.g.ExtractFile(context.Background(), fa, da, ia); err != nil {
		return err
	}
	// Persist the file (with collected decls) back into pkg.
	for i := range pkg.Files {
		if pkg.Files[i].RelPath == f.RelPath {
			pkg.Files[i].Decls = da.decls
			break
		}
	}
	return nil
}

func (a *goAdapter) ResolveCalls(ec *extractCtx, p *packageInfo) error {
	srcs := make([]golang.FileSource, 0, len(p.Files))
	for i := range p.Files {
		srcs = append(srcs, &fileAccess{f: &p.Files[i]})
	}
	ca := &callAccess{pkg: p}
	return a.g.ResolvePackage(context.Background(), srcs, ca)
}

func init() {
	defaultRegistry.register(&goAdapter{g: golang.New()})
}

type fileAccess struct{ f *fileInfo }

func (a *fileAccess) Bytes() []byte   { return a.f.Source }
func (a *fileAccess) RelPath() string { return a.f.RelPath }

type declAccess struct {
	pkg  *packageInfo
	file *fileInfo
	decls []golang.Decl
}

func (a *declAccess) AddDecl(slug string, props map[string]any) {
	a.decls = append(a.decls, Decl{NameSlug: slug, Properties: props})
	a.pkg.DeclByID[slug] = struct{}{}
}

type importAccess struct{ pkg *packageInfo }

func (a *importAccess) AddImport(from, to string) {
	a.pkg.Imports = append(a.pkg.Imports, Import{From: from, To: to})
}

type callAccess struct{ pkg *packageInfo }

func (a *callAccess) AddCall(fromDecl, toDecl string) {
	a.pkg.Calls = append(a.pkg.Calls, Call{FromDecl: fromDecl, ToDecl: toDecl})
}

func (a *callAccess) HasDecl(slug string) bool {
	_, ok := a.pkg.DeclByID[slug]
	return ok
}
```

The `Decl` type in `declAccess.decls` needs to be the language package's `golang.Decl`. To bridge to the orchestrator's `Decl`, convert in place:

```go
func (a *declAccess) Persist() []Decl {
	out := make([]Decl, 0, len(a.decls))
	for _, d := range a.decls {
		out = append(out, Decl{NameSlug: d.NameSlug, Properties: d.Properties})
	}
	return out
}
```

(And in the adapter's `Extract`, replace `pkg.Files[i].Decls = da.decls` with `pkg.Files[i].Decls = da.Persist()`.)

Add the public type to the language package — append to `plugins/tree-sitter/languages/golang/lang.go`:

```go
type Decl struct {
	NameSlug   string
	Properties map[string]any
}

type Import struct {
	From, To string
}

type Call struct {
	FromDecl, ToDecl string
}
```

(`Import` and `Call` are exported for symmetry; the call-resolution implementation will use them internally.)

- [ ] **Step 4: Run the IsExported test and verify the module builds**

```bash
go -C ./plugins/tree-sitter test -run TestIsExported -v ./...
go -C ./plugins/tree-sitter build ./...
```

Expected: PASS and clean build.

- [ ] **Step 5: Commit**

```bash
git add plugins/tree-sitter/
git commit -m "feat(plugin-tree-sitter): register Go grammar with adapter bridging language package"
```

---

### Task 25: Decl extraction — functions, methods, structs, interfaces, var/const

**Files:**
- Modify: `plugins/tree-sitter/languages/golang/lang.go` (implement `ExtractFile`)
- Create: `plugins/tree-sitter/languages/golang/decl.go`
- Create: `plugins/tree-sitter/languages/golang/decl_test.go`

- [ ] **Step 1: Write failing test using a synthetic source file**

Create `plugins/tree-sitter/languages/golang/decl_test.go`:

```go
package golang

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type capturingSinks struct {
	decls   []recordedDecl
	imports [][2]string
}

type recordedDecl struct {
	slug  string
	props map[string]any
}

func (c *capturingSinks) Bytes() []byte                                  { return c.src }
func (c *capturingSinks) RelPath() string                                { return "x.go" }
func (c *capturingSinks) AddDecl(slug string, props map[string]any)      { c.decls = append(c.decls, recordedDecl{slug, props}) }
func (c *capturingSinks) AddImport(from, to string)                      { c.imports = append(c.imports, [2]string{from, to}) }

type capturingSrc struct{ src []byte }

func (c *capturingSrc) Bytes() []byte   { return c.src }
func (c *capturingSrc) RelPath() string { return "x.go" }

func TestExtractFunctionDecls(t *testing.T) {
	src := []byte(`package x

func Foo(a string) (int, error) { return 0, nil }
func bar() {}
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	require.Len(t, sinks.decls, 2)
	require.Equal(t, "foo", sinks.decls[0].slug)
	require.Equal(t, "function", sinks.decls[0].props["kind"])
	require.Equal(t, true, sinks.decls[0].props["exported"])
}

func TestExtractMethodDecls(t *testing.T) {
	src := []byte(`package x

type S struct{}

func (s *S) Hello() {}
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	kinds := kindsOf(sinks.decls)
	require.Contains(t, kinds, "struct")
	require.Contains(t, kinds, "method")
	for _, d := range sinks.decls {
		if d.props["kind"] == "method" {
			require.Equal(t, "S", d.props["receiver"])
		}
	}
}

func TestExtractStructAndInterface(t *testing.T) {
	src := []byte(`package x

type Repo struct{ Name string }
type Reader interface { Read() error }
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	require.Len(t, sinks.decls, 2)
	kinds := kindsOf(sinks.decls)
	require.ElementsMatch(t, []string{"struct", "interface"}, kinds)
}

func TestExtractVarAndConst(t *testing.T) {
	src := []byte(`package x

var pi = 3.14
const Greeting = "hi"
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	require.Len(t, sinks.decls, 2)
}

type captureAll struct {
	src     []byte
	decls   []recordedDecl
	imports [][2]string
}

func (c *captureAll) Bytes() []byte                              { return c.src }
func (c *captureAll) RelPath() string                            { return "x.go" }
func (c *captureAll) AddDecl(slug string, props map[string]any)  { c.decls = append(c.decls, recordedDecl{slug, props}) }
func (c *captureAll) AddImport(from, to string)                  { c.imports = append(c.imports, [2]string{from, to}) }

func kindsOf(ds []recordedDecl) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.props["kind"].(string))
	}
	return out
}
```

(Remove the earlier `capturingSinks` boilerplate — `captureAll` consolidates the three sinks into one. The `c.src` field is the source bytes the test wants the language to see.)

- [ ] **Step 2: Run, verify failure**

```bash
go -C ./plugins/tree-sitter/languages/golang test -v ./...
```

Expected: FAIL (`ExtractFile` returns nil; no decls collected).

- [ ] **Step 3: Implement `decl.go` and update `ExtractFile`**

Create `plugins/tree-sitter/languages/golang/decl.go`:

```go
package golang

import (
	sitter "github.com/smacker/go-tree-sitter"
)

func walkDecls(root *sitter.Node, src []byte, ds DeclSink) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		switch child.Type() {
		case "function_declaration":
			emitFunc(child, src, ds, false)
		case "method_declaration":
			emitFunc(child, src, ds, true)
		case "type_declaration":
			emitType(child, src, ds)
		case "var_declaration":
			emitVarConst(child, src, ds, "var")
		case "const_declaration":
			emitVarConst(child, src, ds, "const")
		}
	}
}

func emitFunc(n *sitter.Node, src []byte, ds DeclSink, isMethod bool) {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(src)
	props := map[string]any{
		"kind":       "function",
		"name":       name,
		"exported":   IsExported(name),
		"line_start": int(n.StartPoint().Row) + 1,
		"line_end":   int(n.EndPoint().Row) + 1,
		"params":     extractParams(n.ChildByFieldName("parameters"), src),
		"returns":    nodeText(n.ChildByFieldName("result"), src),
	}
	if isMethod {
		props["kind"] = "method"
		props["receiver"] = extractReceiver(n.ChildByFieldName("receiver"), src)
	}
	ds.AddDecl(sanitizeIdent(name), props)
}

func emitType(n *sitter.Node, src []byte, ds DeclSink) {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		spec := n.NamedChild(i)
		if spec.Type() != "type_spec" {
			continue
		}
		nameNode := spec.ChildByFieldName("name")
		typeNode := spec.ChildByFieldName("type")
		if nameNode == nil || typeNode == nil {
			continue
		}
		name := nameNode.Content(src)
		kind := "type"
		switch typeNode.Type() {
		case "struct_type":
			kind = "struct"
		case "interface_type":
			kind = "interface"
		}
		ds.AddDecl(sanitizeIdent(name), map[string]any{
			"kind":       kind,
			"name":       name,
			"exported":   IsExported(name),
			"line_start": int(n.StartPoint().Row) + 1,
			"line_end":   int(n.EndPoint().Row) + 1,
		})
	}
}

func emitVarConst(n *sitter.Node, src []byte, ds DeclSink, kind string) {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		spec := n.NamedChild(i)
		nameNode := spec.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nameNode.Content(src)
		ds.AddDecl(sanitizeIdent(name), map[string]any{
			"kind":       kind,
			"name":       name,
			"exported":   IsExported(name),
			"line_start": int(spec.StartPoint().Row) + 1,
			"line_end":   int(spec.EndPoint().Row) + 1,
		})
	}
}

func extractParams(params *sitter.Node, src []byte) []string {
	if params == nil {
		return nil
	}
	var out []string
	for i := 0; i < int(params.NamedChildCount()); i++ {
		decl := params.NamedChild(i)
		if decl.Type() != "parameter_declaration" {
			continue
		}
		for j := 0; j < int(decl.NamedChildCount()); j++ {
			child := decl.NamedChild(j)
			if child.Type() == "identifier" {
				out = append(out, child.Content(src))
			}
		}
	}
	return out
}

func extractReceiver(recv *sitter.Node, src []byte) string {
	if recv == nil {
		return ""
	}
	for i := 0; i < int(recv.NamedChildCount()); i++ {
		decl := recv.NamedChild(i)
		if decl.Type() != "parameter_declaration" {
			continue
		}
		for j := 0; j < int(decl.NamedChildCount()); j++ {
			child := decl.NamedChild(j)
			if child.Type() == "type_identifier" {
				return child.Content(src)
			}
			if child.Type() == "pointer_type" {
				for k := 0; k < int(child.NamedChildCount()); k++ {
					inner := child.NamedChild(k)
					if inner.Type() == "type_identifier" {
						return inner.Content(src)
					}
				}
			}
		}
	}
	return ""
}

func nodeText(n *sitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	return n.Content(src)
}

func sanitizeIdent(name string) string {
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z':
			out = append(out, c+32)
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			out = append(out, c)
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}
```

In `lang.go`, replace the stub `ExtractFile` with:

```go
func (g *GoLang) ExtractFile(ctx context.Context, fs FileSource, ds DeclSink, is ImportSink) error {
	tree, err := g.parse(ctx, fs.Bytes())
	if err != nil {
		return err
	}
	defer tree.Close()
	root := tree.RootNode()
	walkDecls(root, fs.Bytes(), ds)
	walkImports(root, fs.Bytes(), is) // implemented in Task 26
	return nil
}
```

Add a temporary stub for `walkImports` so the build passes — it will be implemented in Task 26:

```go
func walkImports(root *sitter.Node, src []byte, is ImportSink) {}
```

- [ ] **Step 4: Run, verify pass**

```bash
go -C ./plugins/tree-sitter/languages/golang test -v ./...
```

Expected: PASS for the four decl tests.

- [ ] **Step 5: Commit**

```bash
git add plugins/tree-sitter/languages/golang/
git commit -m "feat(plugin-tree-sitter): extract Go functions/methods/types/var/const decls"
```

---

### Task 26: Imports extraction

Handles both `import "foo"` and grouped `import ( "a"; "b" )` forms. The import target is the import path; the import source for the edge is the *current package's* import path. The current package's import path is unknown to the language extractor; it just passes raw target strings. The orchestrator decides whether to skip externals (via `config.include_external_imports`) — but to keep the language extractor simple, we record every import here and let `emit.go` filter.

Per spec, the edge `imports` is package-to-package. The `from` is the current package's path; `to` is the imported package's path. We populate `Import{From: <currentPkgPath>, To: <importPath>}` — the orchestrator knows current pkg path via the `fileInfo.PackagePath`.

To carry the "current pkg" into the language, add a method on `Import` Sink:

**Files:**
- Modify: `plugins/tree-sitter/languages/golang/lang.go`
- Create: `plugins/tree-sitter/languages/golang/imports.go`
- Create: `plugins/tree-sitter/languages/golang/imports_test.go`
- Modify: `plugins/tree-sitter/lang_go_adapter.go` (forward current pkg path)

- [ ] **Step 1: Update the `ImportSink` interface to carry the source package**

In `plugins/tree-sitter/languages/golang/lang.go`, change the `ImportSink` interface to:

```go
type ImportSink interface {
	AddImport(to string)
}
```

Now the *adapter* knows the source (it has access to `fileInfo.PackagePath`); the language extractor only emits the target.

In `plugins/tree-sitter/lang_go_adapter.go`, update `importAccess` to capture the source:

```go
type importAccess struct {
	pkg    *packageInfo
	source string
}

func (a *importAccess) AddImport(to string) {
	a.pkg.Imports = append(a.pkg.Imports, Import{From: a.source, To: to})
}
```

And in the adapter's `Extract` method, populate `source` from `f.PackagePath`:

```go
	ia := &importAccess{pkg: pkg, source: f.PackagePath}
```

- [ ] **Step 2: Write failing tests**

Create `plugins/tree-sitter/languages/golang/imports_test.go`:

```go
package golang

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractSingleImport(t *testing.T) {
	src := []byte(`package x

import "fmt"
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	require.Equal(t, []string{"fmt"}, importsOf(sinks))
}

func TestExtractGroupedImports(t *testing.T) {
	src := []byte(`package x

import (
	"fmt"
	"io"
)
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	require.ElementsMatch(t, []string{"fmt", "io"}, importsOf(sinks))
}

func TestExtractAliasedImport(t *testing.T) {
	src := []byte(`package x

import alt "io"
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	require.Equal(t, []string{"io"}, importsOf(sinks), "alias does not change the import path")
}

func importsOf(c *captureAll) []string {
	out := make([]string, 0, len(c.imports))
	for _, i := range c.imports {
		out = append(out, i[1])
	}
	return out
}
```

Also update `captureAll.AddImport` in `decl_test.go` to match the new single-arg signature:

```go
func (c *captureAll) AddImport(to string) { c.imports = append(c.imports, [2]string{"", to}) }
```

- [ ] **Step 3: Run, verify failure**

```bash
go -C ./plugins/tree-sitter/languages/golang test -run TestExtract -v ./...
```

Expected: FAIL on import tests (walker is still a stub).

- [ ] **Step 4: Implement `imports.go`**

Create `plugins/tree-sitter/languages/golang/imports.go`:

```go
package golang

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

func walkImports(root *sitter.Node, src []byte, is ImportSink) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child.Type() != "import_declaration" {
			continue
		}
		for j := 0; j < int(child.NamedChildCount()); j++ {
			inner := child.NamedChild(j)
			switch inner.Type() {
			case "import_spec":
				addImportSpec(inner, src, is)
			case "import_spec_list":
				for k := 0; k < int(inner.NamedChildCount()); k++ {
					spec := inner.NamedChild(k)
					if spec.Type() == "import_spec" {
						addImportSpec(spec, src, is)
					}
				}
			}
		}
	}
}

func addImportSpec(spec *sitter.Node, src []byte, is ImportSink) {
	path := spec.ChildByFieldName("path")
	if path == nil {
		return
	}
	raw := path.Content(src)
	raw = strings.Trim(raw, "`\"")
	is.AddImport(raw)
}
```

Remove the placeholder stub in `lang.go`.

- [ ] **Step 5: Run, verify pass**

```bash
go -C ./plugins/tree-sitter test ./...
```

Expected: PASS (decl tests + import tests + adapter / module build).

- [ ] **Step 6: Commit**

```bash
git add plugins/tree-sitter/
git commit -m "feat(plugin-tree-sitter): extract Go import declarations (single, grouped, aliased)"
```

---

### Task 27: Intra-package call graph

Walks every function/method body in a package, captures `call_expression` nodes whose callee is an unqualified identifier matching a decl in the same package. Cross-package and selector-qualified calls (`pkg.Foo`, `s.Method()`) are dropped — without type info the false-positive rate is too high.

The slug used in `Call.FromDecl` / `Call.ToDecl` follows the orchestrator's `<pkg-slug>/<file-slug>::<decl-slug>` form. The language extractor only knows decl slugs (it can't compute file slugs). To bridge, expose the package slug + file slug to it via `ResolvePackage`'s `FileSource` interface.

**Files:**
- Modify: `plugins/tree-sitter/languages/golang/lang.go` (extend `FileSource`)
- Create: `plugins/tree-sitter/languages/golang/calls.go`
- Create: `plugins/tree-sitter/languages/golang/calls_test.go`
- Modify: `plugins/tree-sitter/lang_go_adapter.go` (provide pkg/file slugs)

- [ ] **Step 1: Extend `FileSource`**

In `plugins/tree-sitter/languages/golang/lang.go`, change `FileSource` to:

```go
type FileSource interface {
	Bytes() []byte
	RelPath() string
	PackageSlug() string
	FileSlug() string
}
```

Update `captureAll` in test files to satisfy the new methods:

```go
func (c *captureAll) PackageSlug() string { return "x" }
func (c *captureAll) FileSlug() string    { return "x-go" }
```

In `plugins/tree-sitter/lang_go_adapter.go`, update `fileAccess`:

```go
type fileAccess struct {
	f       *fileInfo
	pkgSlug string
}

func (a *fileAccess) Bytes() []byte       { return a.f.Source }
func (a *fileAccess) RelPath() string     { return a.f.RelPath }
func (a *fileAccess) PackageSlug() string { return a.pkgSlug }
func (a *fileAccess) FileSlug() string    { return a.f.BasenameSlug }
```

Update both adapter call sites to populate `pkgSlug`:

```go
// in (a *goAdapter) Extract:
	fa := &fileAccess{f: &f, pkgSlug: pkg.Slug}

// in (a *goAdapter) ResolveCalls:
	srcs := make([]golang.FileSource, 0, len(p.Files))
	for i := range p.Files {
		srcs = append(srcs, &fileAccess{f: &p.Files[i], pkgSlug: p.Slug})
	}
```

- [ ] **Step 2: Write failing test**

Create `plugins/tree-sitter/languages/golang/calls_test.go`:

```go
package golang

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type callCapture struct{ calls [][2]string }

func (c *callCapture) AddCall(from, to string)  { c.calls = append(c.calls, [2]string{from, to}) }
func (c *callCapture) HasDecl(slug string) bool { return slug == "foo" || slug == "bar" }

func TestResolveIntraPackageCalls(t *testing.T) {
	src := []byte(`package x

func foo() { bar() }
func bar() {}
func baz() { fmt.Println("hi") } // qualified — should NOT appear
`)
	g := New()
	fs := &captureAll{src: src}
	cs := &callCapture{}
	require.NoError(t, g.ResolvePackage(context.Background(), []FileSource{fs}, cs))
	require.Len(t, cs.calls, 1)
	require.Equal(t, [2]string{"x/x-go::foo", "x/x-go::bar"}, cs.calls[0])
}
```

- [ ] **Step 3: Run, verify failure**

```bash
go -C ./plugins/tree-sitter/languages/golang test -run TestResolveIntra -v ./...
```

Expected: FAIL (`ResolvePackage` is a stub).

- [ ] **Step 4: Implement `calls.go` and `ResolvePackage`**

Create `plugins/tree-sitter/languages/golang/calls.go`:

```go
package golang

import (
	sitter "github.com/smacker/go-tree-sitter"
)

func walkCalls(g *GoLang, fs FileSource, cs CallSink) error {
	tree, err := g.parse(nil, fs.Bytes())
	if err != nil {
		return err
	}
	defer tree.Close()
	root := tree.RootNode()
	src := fs.Bytes()
	pkgSlug := fs.PackageSlug()
	fileSlug := fs.FileSlug()

	for i := 0; i < int(root.NamedChildCount()); i++ {
		fn := root.NamedChild(i)
		var fnName string
		switch fn.Type() {
		case "function_declaration", "method_declaration":
			name := fn.ChildByFieldName("name")
			if name == nil {
				continue
			}
			fnName = sanitizeIdent(name.Content(src))
		default:
			continue
		}
		body := fn.ChildByFieldName("body")
		if body == nil {
			continue
		}
		walkCallExprs(body, src, func(callee string) {
			if !cs.HasDecl(callee) {
				return
			}
			from := pkgSlug + "/" + fileSlug + "::" + fnName
			to := pkgSlug + "/" + fileSlug + "::" + callee
			cs.AddCall(from, to)
		})
	}
	return nil
}

func walkCallExprs(n *sitter.Node, src []byte, onCall func(string)) {
	if n.Type() == "call_expression" {
		fn := n.ChildByFieldName("function")
		if fn != nil && fn.Type() == "identifier" {
			onCall(sanitizeIdent(fn.Content(src)))
		}
	}
	for i := 0; i < int(n.NamedChildCount()); i++ {
		walkCallExprs(n.NamedChild(i), src, onCall)
	}
}
```

In `lang.go`, replace the `ResolvePackage` stub with:

```go
func (g *GoLang) ResolvePackage(ctx context.Context, files []FileSource, cs CallSink) error {
	for _, f := range files {
		if err := walkCalls(g, f, cs); err != nil {
			return err
		}
	}
	return nil
}
```

Note: the test's `to` target uses `x/x-go::bar` even though `bar` may live in a different file in the same package. For v1, since intra-package calls might cross files, we use the *caller's* file slug as part of the target ID — which is wrong if the callee is in a different file. Fix: the callsite emits `from = pkgSlug+"/"+callerFileSlug+"::"+fromName`, and `to = pkgSlug + "/<callee-file-slug>::" + toName` — but the language extractor doesn't know which file the callee lives in. The orchestrator does (via `DeclByID` which is keyed by `<file-slug>::<decl-slug>`).

For simplicity in v1, we sidestep this: callee slug is just the package slug + decl slug, with a fake file slug `_` indicating "any file in package". Update the spec note for v1: the `calls` edge ID format becomes `<pkg-slug>::<decl-slug>` on the target side. Or — simpler — change the **node ID for decls** to drop the file slug entirely.

Re-read the design spec section "Layers" — it specifies:
> `decl` — one node per top-level declaration. ID: `<domain>:<pkg-slug>/<basename-slug>::<name-slug>`. Parent: file.

So the file slug IS in the decl ID. The cleanest fix for v1: keep emitting `calls` edges only for **same-file** intra-package calls. Update `calls.go`:

```go
		walkCallExprs(body, src, func(callee string) {
			if !cs.HasDecl(callee) {
				return
			}
			// V1 conservative: same-file intra-package only.
			// (Future v2 will resolve cross-file via package-wide symbol table.)
			from := pkgSlug + "/" + fileSlug + "::" + fnName
			to := pkgSlug + "/" + fileSlug + "::" + callee
			cs.AddCall(from, to)
		})
```

And update `HasDecl` semantics: it must check whether the callee is defined in the *same file*, not just the same package. Extend the `CallSink` interface in `lang.go`:

```go
type CallSink interface {
	AddCall(fromDecl, toDecl string)
	HasDeclInFile(fileSlug, slug string) bool
}
```

Then `walkCalls` calls `cs.HasDeclInFile(fileSlug, callee)` instead of `cs.HasDecl(callee)`.

In `lang_go_adapter.go`, update `callAccess.HasDeclInFile`:

```go
func (a *callAccess) HasDeclInFile(fileSlug, slug string) bool {
	for _, f := range a.pkg.Files {
		if f.BasenameSlug != fileSlug {
			continue
		}
		for _, d := range f.Decls {
			if d.NameSlug == slug {
				return true
			}
		}
	}
	return false
}
```

(Drop the old `HasDecl` method.)

Update the test's `callCapture`:

```go
func (c *callCapture) HasDeclInFile(fileSlug, slug string) bool {
	return slug == "foo" || slug == "bar"
}
```

- [ ] **Step 5: Run, verify pass**

```bash
go -C ./plugins/tree-sitter test ./...
```

Expected: PASS (decl, import, call tests; module build clean).

- [ ] **Step 6: Commit**

```bash
git add plugins/tree-sitter/
git commit -m "feat(plugin-tree-sitter): resolve same-file intra-package call edges"
```

---

## Phase 8 — Golden tests for the Go extractor

End-to-end of the *plugin* (not the full pipeline): run `kg-extractor-tree-sitter` against a tiny source tree, diff its stdout against a checked-in `expected.jsonl`. Three fixtures cover single-file, multi-package, and structs+methods.

### Task 28: Golden test runner + three fixtures

**Files:**
- Create: `plugins/tree-sitter/languages/golang/golden_test.go`
- Create: `plugins/tree-sitter/languages/golang/testdata/golden/01-single-file/input/main.go`
- Create: `plugins/tree-sitter/languages/golang/testdata/golden/01-single-file/expected.jsonl`
- Create: `plugins/tree-sitter/languages/golang/testdata/golden/02-multi-package/input/a/a.go`
- Create: `plugins/tree-sitter/languages/golang/testdata/golden/02-multi-package/input/b/b.go`
- Create: `plugins/tree-sitter/languages/golang/testdata/golden/02-multi-package/expected.jsonl`
- Create: `plugins/tree-sitter/languages/golang/testdata/golden/03-with-methods/input/types.go`
- Create: `plugins/tree-sitter/languages/golang/testdata/golden/03-with-methods/expected.jsonl`

- [ ] **Step 1: Create the fixture sources**

```bash
mkdir -p plugins/tree-sitter/languages/golang/testdata/golden/01-single-file/input
mkdir -p plugins/tree-sitter/languages/golang/testdata/golden/02-multi-package/input/a
mkdir -p plugins/tree-sitter/languages/golang/testdata/golden/02-multi-package/input/b
mkdir -p plugins/tree-sitter/languages/golang/testdata/golden/03-with-methods/input
```

Create `01-single-file/input/main.go`:

```go
package main

import "fmt"

func Hello() string {
	return "hi"
}

func main() {
	fmt.Println(Hello())
}
```

Create `02-multi-package/input/a/a.go`:

```go
package a

import "io"

var _ = io.Discard

func A() {}
```

Create `02-multi-package/input/b/b.go`:

```go
package b

func B() {}
```

Create `03-with-methods/input/types.go`:

```go
package types

type Repo struct {
	Name string
}

func (r *Repo) Save() error { return nil }

type Reader interface {
	Read() error
}
```

- [ ] **Step 2: Write the golden test runner**

Create `plugins/tree-sitter/languages/golang/golden_test.go`:

```go
package golang_test

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "rewrite expected.jsonl fixtures from current output")

func TestGolden(t *testing.T) {
	binary := buildPluginBinary(t)
	cases := []string{"01-single-file", "02-multi-package", "03-with-methods"}
	for _, name := range cases {
		name := name
		t.Run(name, func(t *testing.T) {
			input := filepath.Join("testdata", "golden", name, "input")
			expected := filepath.Join("testdata", "golden", name, "expected.jsonl")
			abs, err := filepath.Abs(input)
			require.NoError(t, err)

			cmd := exec.Command(binary, "--language", "go")
			cmd.Stdin = bytes.NewReader([]byte(`{"input":"` + abs + `","domain":"g","protocol_version":1,"config":{}}`))
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			require.NoError(t, cmd.Run(), "stderr=%s", stderr.String())

			got := normalizeAbs(stdout.Bytes(), abs)
			if *updateGolden {
				require.NoError(t, os.WriteFile(expected, got, 0o644))
				return
			}
			want, err := os.ReadFile(expected)
			require.NoError(t, err)
			require.Equal(t, string(want), string(got))
		})
	}
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

- [ ] **Step 3: Generate expected.jsonl files**

Run the test with `-update` once to write the expected outputs:

```bash
go -C ./plugins/tree-sitter/languages/golang test -run TestGolden -update -v ./...
```

Review the generated `expected.jsonl` files manually. They should look like:

- `01-single-file/expected.jsonl` ends up with: meta + domain.add + 1 package node + 1 file node + 2 decl nodes (Hello, main) + 1 imports edge + 1 calls edge (main → Hello).
- `02-multi-package/expected.jsonl` ends up with: meta + domain.add + 2 package nodes (a, b) + 2 file nodes + 2 decl nodes (A, B) + 1 var decl + 1 imports edge (a → io).
- `03-with-methods/expected.jsonl` ends up with: meta + domain.add + 1 package + 1 file + 3 decls (Repo, Save, Reader).

If any output looks wrong (missing decls, swapped slugs, etc.), the bug is in Tasks 25–27 — fix and re-run with `-update`.

- [ ] **Step 4: Run without `-update` to confirm stability**

```bash
go -C ./plugins/tree-sitter/languages/golang test -run TestGolden -v ./...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add plugins/tree-sitter/languages/golang/
git commit -m "test(plugin-tree-sitter): add golden fixtures for Go extraction"
```

---

## Phase 9 — End-to-end pipeline + polish

The capstone: a build-tag-gated test that runs the full `plugin → kg-extractor → kg batch` pipeline against kg's own `internal/graph` package. Plus Makefile additions and a README extractor section.

### Task 29: Makefile additions

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Append targets**

Append to `Makefile`:

```makefile
.PHONY: build-extractor build-plugin-treesitter test-all e2e

build-extractor: build
	go build -o ./bin/kg-extractor ./cmd/kg-extractor

build-plugin-treesitter:
	CGO_ENABLED=1 go -C ./plugins/tree-sitter build -o ../../bin/kg-extractor-tree-sitter .

test-all: test
	go -C ./plugins/tree-sitter test ./...

e2e: build build-extractor build-plugin-treesitter
	go test -tags=e2e -v ./e2e/...
```

- [ ] **Step 2: Verify each target works**

```bash
make build
make build-extractor
make build-plugin-treesitter
make test-all
```

Expected: each succeeds. (`make e2e` will fail until Task 30 creates the test file.)

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile targets for kg-extractor, tree-sitter plugin, test-all, e2e"
```

---

### Task 30: `e2e/extract_self_test.go`

Builds all three binaries, lays out a tmp plugins dir with the tree-sitter manifest pointing at the built binary, runs the pipeline against `internal/graph`, asserts DB shape.

**Files:**
- Create: `e2e/extract_self_test.go`
- Create: `e2e/testutil.go`

- [ ] **Step 1: Write the test**

Create `e2e/testutil.go`:

```go
//go:build e2e

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustBuild(t *testing.T, outBin, pkg string) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, outBin)
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Stderr = bytes.NewBuffer(nil)
	require.NoError(t, cmd.Run(), "build %s failed: %s", pkg, cmd.Stderr)
	return out
}

func mustBuildPlugin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "kg-extractor-tree-sitter")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = filepath.Join("..", "plugins", "tree-sitter")
	cmd.Stderr = bytes.NewBuffer(nil)
	require.NoError(t, cmd.Run(), "build plugin: %s", cmd.Stderr)
	return out
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
```

Create `e2e/extract_self_test.go`:

```go
//go:build e2e

package e2e

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractSelf(t *testing.T) {
	kgBin := mustBuild(t, "kg", "../cmd/kg")
	extractorBin := mustBuild(t, "kg-extractor", "../cmd/kg-extractor")
	pluginBin := mustBuildPlugin(t)

	pluginsDir := t.TempDir()
	pluginHome := filepath.Join(pluginsDir, "tree-sitter")
	writeFile(t, filepath.Join(pluginHome, "manifest.json"), `{
		"name": "tree-sitter",
		"version": "0.1.0",
		"description": "tree-sitter (Go)",
		"runtime": "native",
		"executable": "kg-extractor-tree-sitter"
	}`)
	require.NoError(t, exec.Command("cp", pluginBin, filepath.Join(pluginHome, "kg-extractor-tree-sitter")).Run())

	dbPath := filepath.Join(t.TempDir(), "selfg.db")
	require.NoError(t, exec.Command(kgBin, "--db", dbPath, "init").Run())

	abs, err := filepath.Abs("../internal/graph")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(extractorBin,
		"--plugins-path", pluginsDir,
		"extract",
		"--plugin", "tree-sitter",
		"--language", "go",
		"--input", abs,
		"--domain", "selfg",
		"--db", dbPath,
		"--kg-binary", kgBin,
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), "stderr=%s", stderr.String())

	// Domain shape.
	dom, err := exec.Command(kgBin, "--db", dbPath, "domain", "get", "selfg").Output()
	require.NoError(t, err)
	require.Contains(t, string(dom), `"package"`)
	require.Contains(t, string(dom), `"file"`)
	require.Contains(t, string(dom), `"decl"`)

	// Package node.
	pkgs, err := exec.Command(kgBin, "--db", dbPath, "node", "list", "--domain", "selfg", "--layer", "package").Output()
	require.NoError(t, err)
	require.Contains(t, string(pkgs), "selfg:graph")

	// At least one decl node.
	decls, err := exec.Command(kgBin, "--db", dbPath, "node", "list", "--domain", "selfg", "--layer", "decl").Output()
	require.NoError(t, err)
	require.Contains(t, string(decls), "::parseslug")

	// At least one imports edge from the graph package.
	edges, err := exec.Command(kgBin, "--db", dbPath, "edge", "list-from", "selfg:graph").Output()
	require.NoError(t, err)
	require.Contains(t, string(edges), `"imports"`)
}
```

- [ ] **Step 2: Run the e2e target**

```bash
make e2e
```

Expected: PASS. If the assertion on `selfg:graph` fails, inspect with:

```bash
./bin/kg --db /tmp/selfg.db node list --domain selfg --layer package
```

The package slug for `internal/graph` should sanitize to `graph` (the walker uses `filepath.Dir(rel)` which for the immediate input dir produces `.`, falling back to `filepath.Base(root)` = "graph").

- [ ] **Step 3: Commit**

```bash
git add e2e/
git commit -m "test(e2e): add full-pipeline self-extraction test under e2e build tag"
```

---

### Task 31: README extractor section

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Append an extractor section to README.md**

Append the following section to `README.md` (above existing license/contributing sections if any; otherwise at the end):

```markdown
## Extractors (v1)

kg is a generic graph engine. To populate it from real-world inputs, use
`kg-extractor`, a separate binary that discovers and dispatches plugins.

### Pipeline

```
┌─────────────────┐       ┌─────────────────┐       ┌─────────────┐
│   plugin        │ JSONL │  kg-extractor   │ JSONL │     kg      │
│  (any runtime)  ├──────►│  (validator)    ├──────►│    batch    │
└─────────────────┘       └─────────────────┘       └─────────────┘
```

Plugins live in `~/.config/kg-extractor/plugins/<name>/` (override via
`KG_EXTRACTOR_PLUGINS_PATH`). Each has a `manifest.json` and an executable
(native binary) or command (`["bash", "extract.sh"]`).

### Try the bash demo

```sh
make build-extractor
ln -s "$(pwd)/examples/kg-extractor-plugins/bash-demo" ~/.config/kg-extractor/plugins/bash-demo
./bin/kg --db ./kg.db init
./bin/kg-extractor extract --plugin bash-demo --domain demoapp --db ./kg.db --kg-binary ./bin/kg
./bin/kg node list --domain demoapp
```

### Extract Go code via the tree-sitter plugin

The tree-sitter plugin needs CGO; build it separately:

```sh
make build-plugin-treesitter
mkdir -p ~/.config/kg-extractor/plugins/tree-sitter
cp examples/kg-extractor-plugins/tree-sitter/manifest.json ~/.config/kg-extractor/plugins/tree-sitter/  # not yet shipped — write your own
cp ./bin/kg-extractor-tree-sitter ~/.config/kg-extractor/plugins/tree-sitter/

./bin/kg-extractor extract \
    --plugin tree-sitter --language go \
    --input ./internal/graph --domain mykg \
    --db ./kg.db --kg-binary ./bin/kg
```

This produces a `package → file → decl` graph plus `imports` and intra-package
`calls` edges. See `docs/superpowers/specs/2026-05-23-kg-v1-extractor-design.md`
for the full contract.

### Custom plugins

Any executable that reads a JSON config on stdin and emits JSONL ops on stdout
satisfies the contract. See `examples/kg-extractor-plugins/bash-demo/extract.sh`
for a 10-line template.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add extractor section to README"
```

---

### Task 32: Final integration check + branch close-out

- [ ] **Step 1: Run the full test matrix**

```bash
make test
make test-all
make e2e
```

Expected: all green. If any test fails, fix before merging.

- [ ] **Step 2: Verify GOWORK=off builds (external-consumer simulation for CI)**

```bash
GOWORK=off go build ./...
GOWORK=off go -C ./plugins/tree-sitter build ./...
```

Expected: both succeed (the `replace` directive in `plugins/tree-sitter/go.mod` keeps the plugin building without the workspace file).

- [ ] **Step 3: Review the diff summary**

```bash
git log --oneline main..feat/kg-v1
git diff --stat main..feat/kg-v1
```

Sanity-check: the only files changed under `internal/` should be `internal/graph/service.go` and the two related tests (Properties extension). No modifications to `internal/store/` or `internal/graph/{domain,node,edge,store,errors}.go`.

- [ ] **Step 4: Open the merge**

Push the branch and open a PR (or merge locally if no remote is configured):

```bash
git push -u origin feat/kg-v1
gh pr create --title "feat: v1 — extractor system" --body "$(cat <<'EOF'
## Summary
- Public `batch/` contract package (Op + JSONL codec)
- `kg batch` subcommand (atomic / chunked / continue-on-error / dry-run / progress)
- `cmd/kg-extractor/` dispatcher with manifest-based plugin discovery
- `examples/kg-extractor-plugins/bash-demo/` proving the contract works without compiled code
- `plugins/tree-sitter/` as a separate Go module (CGO isolated) with the Go grammar registered
- `e2e/extract_self_test.go` runs the full pipeline against `internal/graph`

Spec: `docs/superpowers/specs/2026-05-23-kg-v1-extractor-design.md`.

## Test plan
- [x] `make test` (root module)
- [x] `make test-all` (root + plugins/tree-sitter)
- [x] `make e2e` (full pipeline self-extraction)
- [x] `GOWORK=off make test` (CI external-consumer simulation)
EOF
)"
```

Merge as a single unit (squash or merge commit per project preference). v1 ships when this lands on `main`.

---

## Self-Review Notes

After writing this plan, the spec was re-read top-to-bottom. Coverage matrix:

| Spec section | Implementing task(s) |
|---|---|
| `batch/` public contract | Task 1, Task 2 |
| `kg batch` subcommand (atomic) | Task 4 |
| `--continue-on-error` | Task 5 |
| `--chunk-size N` | Task 6 |
| `--dry-run` | Task 7 |
| `--progress` | Task 8 |
| Stream parse errors before tx | Task 4 (initial), Task 8 (final) |
| Manifest format + parsing | Task 10 |
| Discovery + `KG_EXTRACTOR_PLUGINS_PATH` | Task 11 |
| `kg-extractor list` / `info` | Task 12 |
| Subprocess invocation + WASM error | Task 13 |
| JSONL validation pipeline | Task 14 |
| `extract` pass-through | Task 15 |
| `extract --db` → `kg batch` | Task 16 |
| bash-demo plugin | Task 17 |
| Tier-2 integration via bash-demo | Task 18 |
| Multi-module workspace + `go.work` | Task 19 |
| Tree-sitter cobra root + registry | Task 20 |
| Slug sanitization | Task 21 |
| Directory walker + skip rules | Task 22 |
| Op emission + orchestration | Task 23 |
| Go grammar registration | Task 24 |
| Decl extraction (5 kinds) | Task 25 |
| Imports (single + grouped + aliased) | Task 26 |
| Intra-package call graph | Task 27 (same-file conservative v1 cut) |
| Golden tests (3 fixtures) | Task 28 |
| Makefile additions | Task 29 |
| Tier-3 e2e self-extraction | Task 30 |
| README extractor section | Task 31 |
| Branch close-out / merge | Task 32 |

**Deliberate v1 simplifications (called out in the relevant tasks):**

- `--continue-on-error` and `--chunk-size` are mutually exclusive (Task 5). The spec doesn't pin a behavior for both-set; this rule keeps the runner predictable.
- The Go call graph resolves only **same-file intra-package** calls (Task 27). Cross-file intra-package resolution is straightforward in v2 (build a per-package symbol-table-with-file-affinity before resolution) but not necessary for v1's value prop.
- `properties` is settable via batch ops but not via CLI flags (per spec). Service input structs grow `Properties` fields in Task 3.
- The language registration uses an adapter pattern (`lang_go_adapter.go`) instead of importing the orchestrator's private types from `languages/golang/`. This keeps language packages drop-in replaceable; the cost is ~30 lines of boilerplate per language.
- Test for the `kg batch` router uses `os.Stdin` swap rather than refactoring `run` to accept an `io.Reader`. If a later test needs stricter isolation, the swap pattern composes (sequential tests cleanup at end via `t.Cleanup`).

**Risks acknowledged:**

- `github.com/smacker/go-tree-sitter` is unmaintained-ish (last release ~2023). The pinned version in `plugins/tree-sitter/go.mod` should be a known-good revision. The golden tests will catch grammar changes via exact output comparison if we ever bump.
- The `lang_go_adapter.go` bridge does a slightly awkward dance to keep `languages/golang/` from importing the orchestrator's types. If the abstraction becomes painful when a second language is added, collapsing the language packages back into the orchestrator package (one file per language, no subdir) is a small refactor.
- `make e2e` requires a working Go toolchain and ~30s of build time. CI should run it in a separate, optional job.
