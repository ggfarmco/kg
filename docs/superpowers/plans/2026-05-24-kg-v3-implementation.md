# kg v3 Implementation Plan — Skill-Driven LLM Enrichment

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Layer a Claude Code plugin over the v2 kg engine that uses LLM subagents to enrich tree-sitter's structural graph with per-decl summaries, semantic edges, architectural layer nodes, and pedagogical tours. The plugin ships from the kg repo itself under `.claude-plugin/`; the engine grows two small additions (additive-foreign-property writes, `kg export` verb) so foreign sources can annotate without redefining structure.

**Architecture:** Three LLM agents (`file-summarizer`, `architecture-analyzer`, `tour-builder`) each own a distinct source id (`kg-summary:0.1.0`, `kg-arch:0.1.0`, `kg-tours:0.1.0`) and follow a deterministic-then-LLM contract: a bundled bash script dumps structured input from kg → agent reads source files + LLM judges → agent emits a snapshot → snapshot piped to `kg apply`. Four skills (`/kg-enrich`, `/kg-explain`, `/kg-tour`, `/kg-onboard`) orchestrate. v2's namespaced properties + edge-claim refcounts make parallel batches safe with no merger step; architecture and tour outputs land in their own derived domains (`<orig>-arch`, `<orig>-tours`) with cross-domain `contains` / `teaches` edges.

**Tech Stack (delta from v2):** None on the engine side (same Go 1.26, `modernc.org/sqlite`, cobra, testify, snapshot package). New surface: markdown (skills + agents), bash + `jq` (bundled scripts). No new Go third-party deps. The plugin itself is consumed by Claude Code's runtime; kg the engine remains an independently buildable Go CLI.

**Spec:** `docs/superpowers/specs/2026-05-24-kg-v3-skill-enrichment-design.md`. Re-read it before each new phase — design intent (why three source IDs, why per-batch apply, why derived domains instead of properties on file nodes, the engine tweak rationale) is not redundantly restated in each task. v2 context: `docs/superpowers/specs/2026-05-24-kg-v2-provenance-design.md`. v1 context: `docs/superpowers/specs/2026-05-23-kg-v1-extractor-design.md`. Understand-Anything (UA) at `../../../../../Understand-Anything/` is the pattern reference for skill+agent shape — UA's `understand-anything-plugin/skills/understand/SKILL.md` and `agents/file-analyzer.md` are the closest analogues to our `kg-enrich` and `file-summarizer`.

**Prereq:** This plan builds on v0 + v1 + v2 (shipped on `main`, latest pre-v3 commit `1fe1b2d`, "docs(v3): skill-driven LLM enrichment design spec"). All v1 + v2 tests are green. v3 work lives on a new branch `feat/kg-v3-enrichment`; the spec is already committed. Branch merges back to `main` as one unit when Phase 5 is green.

**Before starting:** cut the branch off main:

```bash
git rev-parse --abbrev-ref HEAD  # should print main
git switch -c feat/kg-v3-enrichment
```

**Conventions:**
- Import grouping (Go, 4 blocks separated by blank lines): stdlib, third-party, Kufar non-current module (N/A for kg), current module (`github.com/ggfarmco/kg/...`).
- No comments in code unless they explain a non-obvious *why*. Generated sqlc files are exempt. Skill / agent markdown can have descriptive prose — that IS the artifact, not commentary on code.
- Tests are minimal and non-redundant — each test covers one distinct behavior.
- Every task ends with a commit. Commit messages follow `<type>(scope): <imperative summary>` (types: feat, test, chore, docs, refactor, fix). v3 scopes: `engine` (apply/validate changes), `cli` (kg export), `plugin` (.claude-plugin manifest), `script` (bundled bash scripts), `agent` (agents/*.md), `skill` (skills/*/SKILL.md), `e2e`, `docs`.
- Skill and agent files MUST contain working examples and concrete invariants — they're prompts for an LLM at runtime, not documentation. Vague phrasing leads to hallucinated outputs. UA's files are the bar.
- Bundled scripts have companion `<name>.test.sh` files driven by a fixture-based testing pattern introduced in Task 5. `make test-scripts` runs them.
- Plugin files use kebab-case for directory names (`kg-enrich`, not `kgenrich`). Source IDs use the format `<purpose>:<semver>` (e.g., `kg-summary:0.1.0`).

**State during the rewrite.** Phase 1 is a small engine change (one function in `service_apply.go`, one validator relax, one new CLI verb). All existing tests must stay green throughout Phase 1. Phases 2–4 add new files only — no existing-code changes — so they cannot break the engine. Phase 5 adds tests + docs.

---

## Phase 1 — Engine prep

Two engine changes, both prerequisites for the plugin to function. Without Task 1, `file-summarizer`'s `kg apply` calls silently no-op on foreign-owned nodes (the whole pipeline becomes dead code). Without Task 2, agents that want a baseline of "what's currently in kg for source X" have to reinvent the joining via raw `kg node list` + `jq`, which is fragile.

### Task 1: `Service.applyNodeSpec` annotates foreign-owned nodes in additive scope

The current code at `internal/graph/service_apply.go:153` silently returns `nil` when an additive-scope snapshot references a node owned by a different source. This is correct for "additive snapshot tries to redefine a node" but wrong for "additive snapshot wants to annotate a foreign node with properties in its own namespace." v3's enrichers always do the latter. Also, the snapshot validator currently rejects `NodeSpec` entries with empty `layer` or `name`; that's a structural-data requirement that LLM agents (which only annotate) don't have. Relax it for additive scope.

**Files:**
- Modify: `internal/graph/service_apply.go` (function `applyNodeSpec` around line 144)
- Modify: `snapshot/validate.go` (function `Validate` around line 34)
- Test: `internal/graph/service_apply_test.go` (append 3 tests)
- Test: `snapshot/validate_test.go` (append 1 test)

- [ ] **Step 1: Write the failing test for foreign-node annotation**

Append to `internal/graph/service_apply_test.go`:

```go
func TestApplyAdditiveAnnotatesForeignNode(t *testing.T) {
	svc, fs := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "a", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{{
			ID: "d:x", Layer: "l1", Name: "x",
			Properties: map[string]any{"a-key": "a-val"},
		}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "b", Domain: "d", Scope: snapshot.ScopeAdditive,
		Nodes: []snapshot.NodeSpec{{
			ID: "d:x",
			Properties: map[string]any{"b-key": "b-val"},
		}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.NodesUpdated, "additive annotation counts as an update")

	n, err := fs.GetNode(t.Context(), "d:x")
	require.NoError(t, err)
	require.Equal(t, graph.SourceID("a"), n.Source, "ownership unchanged")
	require.Equal(t, "x", n.Name, "name untouched")
	require.Equal(t, "a-val", n.Properties["a"]["a-key"])
	require.Equal(t, "b-val", n.Properties["b"]["b-key"], "B's namespace populated")
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/graph/... -run TestApplyAdditiveAnnotatesForeignNode -v
```

Expected: FAIL — `Properties["b"]` will be nil (current code skips foreign nodes in additive scope without writing properties).

- [ ] **Step 3: Modify `applyNodeSpec` to write properties on foreign nodes**

In `internal/graph/service_apply.go`, replace the block at lines ~149–157:

```go
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
```

with:

```go
	existing, ok := byID[id]
	if !ok {
		other, gerr := s.store.GetNode(ctx, id)
		if gerr == nil && other != nil {
			if scope == snapshot.ScopeAdditive {
				if len(spec.Properties) == 0 {
					return nil
				}
				if err := s.SetNodeProperties(ctx, id, source, spec.Properties); err != nil {
					return err
				}
				res.NodesUpdated++
				return nil
			}
			return fmt.Errorf("%w: id=%s owner=%s", ErrNodeOwnedByDifferentSource, id, other.Source)
		}
		if gerr != nil && !errors.Is(gerr, ErrNodeNotFound) {
			return gerr
		}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/graph/... -run TestApplyAdditiveAnnotatesForeignNode -v
```

Expected: PASS.

- [ ] **Step 5: Write the backward-compat test (no properties → still a no-op)**

Append to `internal/graph/service_apply_test.go`:

```go
func TestApplyAdditiveSkipsForeignNodeWithoutProperties(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "a", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes:      []snapshot.NodeSpec{{ID: "d:x", Layer: "l1", Name: "x"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "b", Domain: "d", Scope: snapshot.ScopeAdditive,
		Nodes:          []snapshot.NodeSpec{{ID: "d:x"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, res.NodesUpdated)
}
```

Run:

```bash
go test ./internal/graph/... -run TestApplyAdditiveSkipsForeignNodeWithoutProperties -v
```

Expected: PASS (the `len(spec.Properties) == 0` guard preserves the old no-op behavior).

- [ ] **Step 6: Write the validator test for relaxed required fields in additive scope**

Append to `snapshot/validate_test.go`:

```go
func TestValidateAdditiveAllowsBareNodeID(t *testing.T) {
	err := Validate(&Snapshot{
		ProtocolVersion: 2, Source: "b", Domain: "d", Scope: ScopeAdditive,
		Nodes:           []NodeSpec{{ID: "d:x", Properties: map[string]any{"k": "v"}}},
	})
	require.NoError(t, err)
}

func TestValidateDomainSourceStillRequiresLayerAndName(t *testing.T) {
	err := Validate(&Snapshot{
		ProtocolVersion: 2, Source: "b", Domain: "d", Scope: ScopeDomainSource,
		Nodes:           []NodeSpec{{ID: "d:x"}},
	})
	require.Error(t, err)
}
```

- [ ] **Step 7: Run the validator tests to verify the first fails**

```bash
go test ./snapshot/... -run TestValidateAdditiveAllowsBareNodeID -v
```

Expected: FAIL — current validator unconditionally requires layer + name.

- [ ] **Step 8: Relax the validator**

In `snapshot/validate.go`, replace the loop body around lines 34–40:

```go
	for i, n := range s.Nodes {
		if !nodeIDRE.MatchString(n.ID) {
			return fmt.Errorf("%w: nodes[%d].id=%q", ErrInvalidNodeID, i, n.ID)
		}
		if n.Layer == "" || n.Name == "" {
			return fmt.Errorf("nodes[%d]: layer and name are required", i)
		}
	}
```

with:

```go
	for i, n := range s.Nodes {
		if !nodeIDRE.MatchString(n.ID) {
			return fmt.Errorf("%w: nodes[%d].id=%q", ErrInvalidNodeID, i, n.ID)
		}
		if s.Scope != ScopeAdditive {
			if n.Layer == "" || n.Name == "" {
				return fmt.Errorf("nodes[%d]: layer and name are required", i)
			}
		}
	}
```

- [ ] **Step 9: Run both validator tests to verify they pass**

```bash
go test ./snapshot/... -run 'TestValidateAdditiveAllowsBareNodeID|TestValidateDomainSourceStillRequiresLayerAndName' -v
```

Expected: PASS for both.

- [ ] **Step 10: Run the full engine test suite to confirm no regressions**

```bash
make test
```

Expected: all green. Pay attention to `TestApplyAdditiveScopeSkipsCleanup` and `TestApplyOverrideScopeAdditivePreservesUnclaimedEdges` — those exercise the additive-scope code paths and should still pass.

- [ ] **Step 11: Commit**

```bash
git add internal/graph/service_apply.go internal/graph/service_apply_test.go snapshot/validate.go snapshot/validate_test.go
git commit -m "feat(engine): additive scope writes properties on foreign-owned nodes"
```

---

### Task 2: New CLI verb `kg export --domain X --source Y`

Outputs the current `(domain, source)` slice as a valid snapshot JSON document. Pipe-friendly: `kg export ... | kg apply ...` is a round-trip identity. Used by agents that want a baseline of "what does my source currently have in the graph" (e.g., re-runs that want to diff before writing). The output's scope defaults to `domain-source` since exporting a single source's view is exactly that.

**Files:**
- Create: `cmd/kg/export_cmd.go`
- Create: `cmd/kg/export_cmd_test.go`
- Modify: `cmd/kg/root.go` (register the verb)

- [ ] **Step 1: Write the failing round-trip test**

Create `cmd/kg/export_cmd_test.go`:

```go
package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExportRoundTrip(t *testing.T) {
	dbPath := newTestDB(t)
	runOK(t, dbPath, "init")
	runOK(t, dbPath, "domain", "add", "--id", "d", "--layers", "pkg,file", "--description", "demo")
	runOK(t, dbPath, "node", "add", "--domain", "d", "--layer", "pkg", "--name", "a", "--id", "a")
	runOK(t, dbPath, "node", "add", "--domain", "d", "--layer", "file", "--name", "x", "--id", "a/x", "--parent", "d:a")
	runOK(t, dbPath, "edge", "add", "--from", "d:a", "--to", "d:a/x", "--type", "contains")

	exported := runOKBytes(t, dbPath, "export", "--domain", "d", "--source", "cli")
	require.Contains(t, string(exported), `"protocol_version": 2`)
	require.Contains(t, string(exported), `"d:a/x"`)

	res := runOKWithStdin(t, dbPath, bytes.NewReader(exported), "apply", "--source", "cli", "--domain", "d")
	require.Contains(t, string(res), `"nodes_added": 0`, "round-trip is a no-op")
	require.Contains(t, string(res), `"nodes_updated": 0`)
	require.Contains(t, string(res), `"nodes_removed": 0`)
}

func TestExportEmptySource(t *testing.T) {
	dbPath := newTestDB(t)
	runOK(t, dbPath, "init")
	runOK(t, dbPath, "domain", "add", "--id", "d", "--layers", "pkg", "--description", "")

	out := runOKBytes(t, dbPath, "export", "--domain", "d", "--source", "kg-summary:0.1.0")
	s := string(out)
	require.Contains(t, s, `"source": "kg-summary:0.1.0"`)
	require.Contains(t, s, `"nodes": []`)
	require.Contains(t, s, `"edges": []`)
}

func runOKBytes(t *testing.T, dbPath string, args ...string) []byte {
	t.Helper()
	out := bytes.Buffer{}
	errb := bytes.Buffer{}
	full := append([]string{"--db", dbPath}, args...)
	exit := run(full, &out, &errb)
	require.Equal(t, 0, exit, "stderr=%s stdout=%s", errb.String(), out.String())
	require.True(t, strings.HasPrefix(out.String(), `{`), "expected JSON output, got: %s", out.String())
	return out.Bytes()
}

func runOKWithStdin(t *testing.T, dbPath string, stdin io.Reader, args ...string) []byte {
	t.Helper()
	t.Skip("stdin support needs runOKWithStdin helper from apply_cmd_test.go pattern — see existing helper if present")
	return nil
}
```

(If `runOKWithStdin` already exists in `apply_cmd_test.go`, reuse it and drop the `t.Skip` stub. If not, copy the pattern from there — `run` takes `stdout, stderr`, but `apply_cmd.go` uses `os.Stdin` directly, so the test harness needs to dup-and-restore `os.Stdin`. Check `apply_cmd_test.go` for the exact helper before writing this. If it doesn't exist there either, add one to that file in this task as part of the same commit.)

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./cmd/kg/... -run 'TestExportRoundTrip|TestExportEmptySource' -v
```

Expected: FAIL — `kg export` doesn't exist yet.

- [ ] **Step 3: Implement `cmd/kg/export_cmd.go`**

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/snapshot"
)

func newExportCmd(c *cliCtx) *cobra.Command {
	var domain, source, format string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export current (domain, source) state as a snapshot JSON document",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if format != "snapshot" {
				return fmt.Errorf("--format must be 'snapshot' (got %q)", format)
			}
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()

			dID := graph.DomainID(domain)
			srcID := graph.SourceID(source)

			d, err := svc.GetDomain(cmd.Context(), dID)
			if err != nil {
				return err
			}

			nodes, err := svc.ListNodes(cmd.Context(), graph.NodeFilter{Domain: dID, Source: srcID})
			if err != nil {
				return err
			}

			snap := snapshot.Snapshot{
				ProtocolVersion: snapshot.ProtocolVersion,
				Source:          source,
				Domain:          domain,
				Scope:           snapshot.ScopeDomainSource,
				DomainSpec: &snapshot.DomainSpec{
					ID: string(d.ID), Layers: d.Layers, Description: d.Description,
				},
				Nodes: make([]snapshot.NodeSpec, 0, len(nodes)),
				Edges: []snapshot.EdgeSpec{},
			}
			for _, n := range nodes {
				spec := snapshot.NodeSpec{
					ID: string(n.ID), Layer: n.Layer, Name: n.Name,
					Properties: n.Properties[srcID],
				}
				if n.ParentID != nil {
					spec.Parent = string(*n.ParentID)
				}
				snap.Nodes = append(snap.Nodes, spec)
			}

			claimedIDs, err := svc.EdgeIDsClaimedBy(cmd.Context(), srcID)
			if err != nil {
				return err
			}
			for _, eid := range claimedIDs {
				e, err := svc.GetEdge(cmd.Context(), eid)
				if err != nil {
					return err
				}
				snap.Edges = append(snap.Edges, snapshot.EdgeSpec{
					Src: string(e.SourceID), Target: string(e.TargetID),
					Type: e.Type, Properties: e.Properties[srcID],
				})
			}

			return snapshot.Encode(c.stdout, snap)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "domain id (required)")
	cmd.Flags().StringVar(&source, "source", "", "writer source id (required)")
	cmd.Flags().StringVar(&format, "format", "snapshot", "output format (only 'snapshot' supported in v3)")
	_ = cmd.MarkFlagRequired("domain")
	_ = cmd.MarkFlagRequired("source")
	return cmd
}
```

`Service.EdgeIDsClaimedBy` and `Service.GetEdge` may not yet be exposed on the public Service surface — Service forwards most store calls (see `service.go:392-398` for `EdgesFrom/EdgesTo`). If `EdgeIDsClaimedBy` is store-only, add a thin Service wrapper:

```go
// service.go (append near EdgesFrom)
func (s *Service) EdgeIDsClaimedBy(ctx context.Context, source SourceID) ([]EdgeID, error) {
	return s.store.EdgeIDsClaimedBy(ctx, source)
}

func (s *Service) GetEdge(ctx context.Context, id EdgeID) (*Edge, error) {
	return s.store.GetEdge(ctx, id)
}
```

Verify by grepping first:

```bash
grep -n "func (s \*Service) EdgeIDsClaimedBy\|func (s \*Service) GetEdge" internal/graph/service.go
```

Add the wrappers only if grep returns nothing.

- [ ] **Step 4: Register the verb in `cmd/kg/root.go`**

In `cmd/kg/root.go:31`, extend the `AddCommand` call:

```go
	root.AddCommand(newInitCmd(c), newDomainCmd(c), newNodeCmd(c), newEdgeCmd(c), newBatchCmd(c), newSourcesCmd(c), newApplyCmd(c), newExportCmd(c))
```

- [ ] **Step 5: Run the export tests to verify they pass**

```bash
go test ./cmd/kg/... -run 'TestExportRoundTrip|TestExportEmptySource' -v
```

Expected: PASS.

- [ ] **Step 6: Run the full suite**

```bash
make test
```

Expected: all green.

- [ ] **Step 7: Manual smoke test**

```bash
make build
./bin/kg --db /tmp/v3-export-smoke.db init
./bin/kg --db /tmp/v3-export-smoke.db domain add --id myapp --layers package,file --description "demo"
./bin/kg --db /tmp/v3-export-smoke.db node add --domain myapp --layer package --name graph --id graph
./bin/kg --db /tmp/v3-export-smoke.db export --domain myapp --source cli
```

Expected output: a single JSON document like:

```json
{
  "protocol_version": 2,
  "source": "cli",
  "domain": "myapp",
  "scope": "domain-source",
  "domain_spec": { "id": "myapp", "layers": ["package", "file"], "description": "demo" },
  "nodes": [ { "id": "myapp:graph", "layer": "package", "name": "graph" } ],
  "edges": []
}
```

Clean up:

```bash
rm -f /tmp/v3-export-smoke.db /tmp/v3-export-smoke.db-wal /tmp/v3-export-smoke.db-shm
```

- [ ] **Step 8: Commit**

```bash
git add cmd/kg/export_cmd.go cmd/kg/export_cmd_test.go cmd/kg/root.go internal/graph/service.go
git commit -m "feat(cli): kg export --domain X --source Y emits snapshot JSON"
```

---

### Task 3: README + CHANGELOG note for v3 engine changes

Document the engine deltas (additive-foreign-properties, `kg export`) for users who consume the engine standalone. v3's plugin-side docs come later (Task 16).

**Files:**
- Modify: `README.md` (append a small v3-engine subsection to the existing v2 "Extractors" section)
- Create: `CHANGELOG.md` if absent; else append a v0.3.0 entry

- [ ] **Step 1: Inspect current README and CHANGELOG state**

```bash
grep -n "## Extractors" README.md | head -5
ls CHANGELOG.md 2>/dev/null && head -20 CHANGELOG.md
```

If `CHANGELOG.md` doesn't exist, create it with the format below in step 3. If it exists, prepend the new entry.

- [ ] **Step 2: Append a "v3 engine additions" paragraph to README**

After the v2 extractors section (find the heading `## Extractors (v2`), append a sibling subsection:

```markdown
### v3 additions

- `kg apply` with `scope: additive` writes properties on foreign-owned nodes (in the writer's own namespace) — previously such writes were silently dropped. This makes the engine usable for LLM-based annotators that don't own the underlying structural nodes.
- `kg export --domain <id> --source <id>` emits the current `(domain, source)` slice as a snapshot JSON document. Round-trips with `kg apply` for diffing or re-importing. The v3 LLM enrichment plugin uses this to give agents a baseline view of what their source has already written.
- v3's annotation pipeline (Claude Code plugin under `.claude-plugin/`) is a separate concern documented at the end of this README; the engine changes above are usable standalone.
```

- [ ] **Step 3: Create or append `CHANGELOG.md`**

Either create:

```markdown
# Changelog

## v0.3.0 — 2026-05-24

### Engine
- `Service.Apply` in additive scope now writes properties on foreign-owned nodes (under the writer's source namespace). Previously silently skipped.
- Snapshot validator no longer requires `layer` and `name` on `NodeSpec` entries when `scope: additive`. Required fields tighten only for `domain-source` and `domain` scopes.
- New CLI verb: `kg export --domain X --source Y --format snapshot`. Round-trips with `kg apply`.

### Plugin (Claude Code)
- New `.claude-plugin/` directory adds four skills (`/kg-enrich`, `/kg-explain`, `/kg-tour`, `/kg-onboard`) and three subagents (`file-summarizer`, `architecture-analyzer`, `tour-builder`). See README's "v3 enrichment plugin" section.

## v0.2.0 — 2026-05-24

- See spec `docs/superpowers/specs/2026-05-24-kg-v2-provenance-design.md`.
```

…or, if `CHANGELOG.md` already has entries, insert the v0.3.0 block above them (keep reverse-chronological order).

- [ ] **Step 4: Commit**

```bash
git add README.md CHANGELOG.md
git commit -m "docs(engine): document v3 engine additions (additive-foreign-properties, kg export)"
```

---

## Phase 2 — Plugin scaffold + bundled scripts

Build the plumbing layer the agents and skills will sit on top of. Three tasks: the manifest files, simple bash wrappers, and the larger graph-shape / topology dumpers. Each script is tested with a tiny fixture-based harness so they don't drift silently when kg's CLI surface evolves.

### Task 4: Create `.claude-plugin/` manifest and directory layout

The plugin's filesystem layout follows Claude Code's convention. `plugin.json` is the local manifest; `marketplace.json` lets users `/plugin marketplace add github:ggfarmco/kg`.

**Files:**
- Create: `.claude-plugin/plugin.json`
- Create: `.claude-plugin/marketplace.json`
- Create: directory stubs (touch files to anchor git): `.claude-plugin/skills/.gitkeep`, `.claude-plugin/agents/.gitkeep`

- [ ] **Step 1: Verify directory creation is safe**

```bash
ls .claude-plugin 2>/dev/null && echo EXISTS || echo NEW
```

Expected: `NEW`. If `EXISTS`, stop and review — something is off.

- [ ] **Step 2: Create `.claude-plugin/plugin.json`**

```json
{
  "name": "kg",
  "displayName": "kg Knowledge Graph",
  "version": "0.3.0",
  "description": "LLM-driven enrichment over kg structural extraction. Generates per-decl summaries, semantic edges, architectural layers, and pedagogical tours for codebases ingested via the kg engine. Requires `kg` CLI on PATH.",
  "skills": "./skills",
  "agents": "./agents"
}
```

- [ ] **Step 3: Create `.claude-plugin/marketplace.json`**

```json
{
  "schema": "marketplace-v1",
  "name": "kg",
  "repository": "github.com/ggfarmco/kg",
  "plugin": "./.claude-plugin"
}
```

- [ ] **Step 4: Create the directory anchors**

```bash
mkdir -p .claude-plugin/skills .claude-plugin/agents
touch .claude-plugin/skills/.gitkeep .claude-plugin/agents/.gitkeep
```

- [ ] **Step 5: Commit**

```bash
git add .claude-plugin/plugin.json .claude-plugin/marketplace.json .claude-plugin/skills/.gitkeep .claude-plugin/agents/.gitkeep
git commit -m "feat(plugin): .claude-plugin scaffold (manifest + marketplace + skill/agent dirs)"
```

---

### Task 5: Simple bundled scripts (`dump-files`, `dump-batch-context`, `apply-snapshot`)

Three wrappers around `kg` + `jq`. `dump-files.sh` lists file-layer nodes for a `(domain, source)`; `dump-batch-context.sh` enriches each file with its decl list; `apply-snapshot.sh` is sugar for `kg apply --source X --domain Y --scope Z`. Tests use a fake `kg` shell function that returns canned JSON; assertions are `diff` against expected output.

**Files:**
- Create: `.claude-plugin/skills/kg-enrich/scripts/dump-files.sh`
- Create: `.claude-plugin/skills/kg-enrich/scripts/dump-batch-context.sh`
- Create: `.claude-plugin/skills/kg-enrich/scripts/apply-snapshot.sh`
- Create: `.claude-plugin/skills/kg-enrich/scripts/tests/dump-files.test.sh`
- Create: `.claude-plugin/skills/kg-enrich/scripts/tests/dump-batch-context.test.sh`
- Create: `.claude-plugin/skills/kg-enrich/scripts/tests/apply-snapshot.test.sh`
- Create: `.claude-plugin/skills/kg-enrich/scripts/tests/fixtures/` (small JSON fixtures)
- Modify: `Makefile` — add `test-scripts` target

- [ ] **Step 1: Create the scripts directory**

```bash
mkdir -p .claude-plugin/skills/kg-enrich/scripts/tests/fixtures
```

- [ ] **Step 2: Write `dump-files.sh`**

`.claude-plugin/skills/kg-enrich/scripts/dump-files.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: dump-files.sh <domain> <source-id>" >&2
  exit 2
fi

domain="$1"
source_id="$2"

kg node list --domain "$domain" --layer file --source "$source_id" \
  | jq --arg src "$source_id" '
    .data | map({
      node_id: .id,
      file_path: (.properties[$src].path // ""),
      package_node_id: .parent_id,
      name: .name
    })
  '
```

Make executable: `chmod +x .claude-plugin/skills/kg-enrich/scripts/dump-files.sh`

- [ ] **Step 3: Write `dump-batch-context.sh`**

`.claude-plugin/skills/kg-enrich/scripts/dump-batch-context.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: dump-batch-context.sh <files-json-path> <source-id>" >&2
  exit 2
fi

input="$1"
source_id="$2"

if [ ! -f "$input" ]; then
  echo "input file not found: $input" >&2
  exit 2
fi

jq -c '.[]' "$input" | while read -r file; do
  fileId=$(echo "$file" | jq -r '.node_id')
  decls=$(kg node children "$fileId" \
    | jq --arg src "$source_id" '
      [.data[] | select(.layer == "decl") | {
        node_id: .id,
        name: .name,
        kind: (.properties[$src].kind // ""),
        line_range: [
          (.properties[$src].line_start // 0),
          (.properties[$src].line_end   // 0)
        ]
      }]
    ')
  echo "$file" | jq --argjson decls "$decls" '. + {decls: $decls}'
done | jq -s '.'
```

Make executable.

- [ ] **Step 4: Write `apply-snapshot.sh`**

`.claude-plugin/skills/kg-enrich/scripts/apply-snapshot.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 3 ]; then
  echo "usage: apply-snapshot.sh <source-id> <domain-id> <scope>" >&2
  echo "snapshot JSON is read from stdin." >&2
  exit 2
fi

exec kg apply --source "$1" --domain "$2" --scope "$3"
```

Make executable.

- [ ] **Step 5: Write fixtures for the tests**

`.claude-plugin/skills/kg-enrich/scripts/tests/fixtures/kg-node-list-files.json` (canned output that the fake `kg` returns for `node list --layer file`):

```json
{
  "data": [
    {
      "id": "myapp:graph/handler-go",
      "domain": "myapp",
      "layer": "file",
      "name": "handler.go",
      "parent_id": "myapp:graph",
      "source": "tree-sitter:0.2.0",
      "properties": {
        "tree-sitter:0.2.0": { "path": "/abs/internal/graph/handler.go" }
      }
    },
    {
      "id": "myapp:graph/service-go",
      "domain": "myapp",
      "layer": "file",
      "name": "service.go",
      "parent_id": "myapp:graph",
      "source": "tree-sitter:0.2.0",
      "properties": {
        "tree-sitter:0.2.0": { "path": "/abs/internal/graph/service.go" }
      }
    }
  ]
}
```

`.claude-plugin/skills/kg-enrich/scripts/tests/fixtures/expected-dump-files.json`:

```json
[
  {
    "node_id": "myapp:graph/handler-go",
    "file_path": "/abs/internal/graph/handler.go",
    "package_node_id": "myapp:graph",
    "name": "handler.go"
  },
  {
    "node_id": "myapp:graph/service-go",
    "file_path": "/abs/internal/graph/service.go",
    "package_node_id": "myapp:graph",
    "name": "service.go"
  }
]
```

`.claude-plugin/skills/kg-enrich/scripts/tests/fixtures/kg-node-children-handler.json`:

```json
{
  "data": [
    {
      "id": "myapp:graph/handler-go::serve",
      "layer": "decl",
      "name": "Serve",
      "properties": { "tree-sitter:0.2.0": { "kind": "function", "line_start": 42, "line_end": 87 } }
    },
    {
      "id": "myapp:graph/handler-go::route",
      "layer": "decl",
      "name": "Route",
      "properties": { "tree-sitter:0.2.0": { "kind": "function", "line_start": 90, "line_end": 105 } }
    }
  ]
}
```

`.claude-plugin/skills/kg-enrich/scripts/tests/fixtures/kg-node-children-service.json`:

```json
{
  "data": [
    {
      "id": "myapp:graph/service-go::do",
      "layer": "decl",
      "name": "Do",
      "properties": { "tree-sitter:0.2.0": { "kind": "function", "line_start": 10, "line_end": 30 } }
    }
  ]
}
```

`.claude-plugin/skills/kg-enrich/scripts/tests/fixtures/dump-batch-input.json` (the input the test passes to `dump-batch-context.sh` — same shape as `expected-dump-files.json`, copy it):

```json
[
  {
    "node_id": "myapp:graph/handler-go",
    "file_path": "/abs/internal/graph/handler.go",
    "package_node_id": "myapp:graph",
    "name": "handler.go"
  },
  {
    "node_id": "myapp:graph/service-go",
    "file_path": "/abs/internal/graph/service.go",
    "package_node_id": "myapp:graph",
    "name": "service.go"
  }
]
```

`.claude-plugin/skills/kg-enrich/scripts/tests/fixtures/expected-dump-batch.json`:

```json
[
  {
    "node_id": "myapp:graph/handler-go",
    "file_path": "/abs/internal/graph/handler.go",
    "package_node_id": "myapp:graph",
    "name": "handler.go",
    "decls": [
      { "node_id": "myapp:graph/handler-go::serve", "name": "Serve", "kind": "function", "line_range": [42, 87] },
      { "node_id": "myapp:graph/handler-go::route", "name": "Route", "kind": "function", "line_range": [90, 105] }
    ]
  },
  {
    "node_id": "myapp:graph/service-go",
    "file_path": "/abs/internal/graph/service.go",
    "package_node_id": "myapp:graph",
    "name": "service.go",
    "decls": [
      { "node_id": "myapp:graph/service-go::do", "name": "Do", "kind": "function", "line_range": [10, 30] }
    ]
  }
]
```

- [ ] **Step 6: Write `dump-files.test.sh`**

`.claude-plugin/skills/kg-enrich/scripts/tests/dump-files.test.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# Fake kg: when called as `kg node list --layer file ...`, emit the canned fixture.
kg() {
  case "$*" in
    *"node list --domain myapp --layer file --source tree-sitter:0.2.0"*)
      cat fixtures/kg-node-list-files.json ;;
    *) echo "unexpected kg call: $*" >&2; exit 1 ;;
  esac
}
export -f kg

actual=$(../dump-files.sh myapp tree-sitter:0.2.0)
expected=$(cat fixtures/expected-dump-files.json)
diff <(echo "$actual" | jq -S .) <(echo "$expected" | jq -S .) \
  || { echo "FAIL dump-files.sh"; exit 1; }
echo "OK dump-files.sh"
```

Make executable.

- [ ] **Step 7: Write `dump-batch-context.test.sh`**

`.claude-plugin/skills/kg-enrich/scripts/tests/dump-batch-context.test.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

kg() {
  case "$*" in
    *"node children myapp:graph/handler-go"*)
      cat fixtures/kg-node-children-handler.json ;;
    *"node children myapp:graph/service-go"*)
      cat fixtures/kg-node-children-service.json ;;
    *) echo "unexpected kg call: $*" >&2; exit 1 ;;
  esac
}
export -f kg

actual=$(../dump-batch-context.sh fixtures/dump-batch-input.json tree-sitter:0.2.0)
expected=$(cat fixtures/expected-dump-batch.json)
diff <(echo "$actual" | jq -S .) <(echo "$expected" | jq -S .) \
  || { echo "FAIL dump-batch-context.sh"; exit 1; }
echo "OK dump-batch-context.sh"
```

Make executable.

- [ ] **Step 8: Write `apply-snapshot.test.sh`**

`.claude-plugin/skills/kg-enrich/scripts/tests/apply-snapshot.test.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# Fake kg captures args via tmpfile so we can assert order.
trace=$(mktemp)
kg() {
  echo "$*" > "$trace"
  echo '{"ok":true,"data":{"nodes_added":0}}'
}
export -f kg

echo '{"protocol_version":2,"source":"kg-summary:0.1.0","domain":"myapp","scope":"additive","nodes":[],"edges":[]}' \
  | ../apply-snapshot.sh kg-summary:0.1.0 myapp additive >/dev/null

got=$(cat "$trace")
expected="apply --source kg-summary:0.1.0 --domain myapp --scope additive"
[ "$got" = "$expected" ] || { echo "FAIL apply-snapshot.sh: got '$got' expected '$expected'"; exit 1; }
echo "OK apply-snapshot.sh"
rm -f "$trace"
```

Make executable.

- [ ] **Step 9: Add `test-scripts` target to `Makefile`**

In `Makefile`, append (and add `test-scripts` to `.PHONY`):

```makefile
test-scripts:
	@find .claude-plugin -name '*.test.sh' -print -exec bash {} \;
```

Update the `.PHONY:` line at the top to include `test-scripts`.

- [ ] **Step 10: Run the script tests to verify they pass**

```bash
make test-scripts
```

Expected: 3 `OK` lines, exit 0.

- [ ] **Step 11: Commit**

```bash
git add .claude-plugin/skills/kg-enrich/scripts Makefile
git commit -m "feat(script): simple bundled scripts (dump-files, dump-batch-context, apply-snapshot) with tests"
```

---

### Task 6: Complex bundled scripts (`dump-graph-shape`, `dump-topology`)

`dump-graph-shape.sh` builds per-package import adjacency + directory grouping for `architecture-analyzer`. `dump-topology.sh` computes fan-in/fan-out, ranks entry points (heuristic scoring), and traces BFS chains for `tour-builder`. Both are pure bash + `jq` + `kg` CLI; tests use the same fixture pattern.

**Files:**
- Create: `.claude-plugin/skills/kg-enrich/scripts/dump-graph-shape.sh`
- Create: `.claude-plugin/skills/kg-enrich/scripts/dump-topology.sh`
- Create: `.claude-plugin/skills/kg-enrich/scripts/tests/dump-graph-shape.test.sh`
- Create: `.claude-plugin/skills/kg-enrich/scripts/tests/dump-topology.test.sh`
- Create: fixtures for both

- [ ] **Step 1: Write `dump-graph-shape.sh`**

`.claude-plugin/skills/kg-enrich/scripts/dump-graph-shape.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: dump-graph-shape.sh <domain> <source-id>" >&2
  exit 2
fi

domain="$1"
source_id="$2"

packages=$(kg node list --domain "$domain" --layer package --source "$source_id" \
  | jq --arg src "$source_id" '
    .data | map({
      slug: .id,
      name: .name,
      path: (.properties[$src].path // ""),
      files: []
    })
  ')

files=$(kg node list --domain "$domain" --layer file --source "$source_id" \
  | jq --arg src "$source_id" '
    .data | map({
      node_id: .id,
      package_node_id: .parent_id,
      path: (.properties[$src].path // ""),
      name: .name
    })
  ')

packages=$(jq -n --argjson pkgs "$packages" --argjson files "$files" '
  $pkgs | map(. as $p | $p + {
    files: ($files | map(select(.package_node_id == $p.slug)) | map({node_id, path, name}))
  })
')

imports=$(kg edge list --domain "$domain" --type imports 2>/dev/null \
  | jq '
    if .data then .data | map({from: .source_id, to: .target_id}) else [] end
  ' \
  || echo '[]')

jq -n \
  --argjson packages "$packages" \
  --argjson imports "$imports" \
  '{packages: $packages, imports: $imports}'
```

Make executable. Note: `kg edge list --domain X --type imports` may not exist as a CLI verb — verify with `./bin/kg edge list --help` after build. If it doesn't, substitute the closest equivalent (`kg edge list-from` per node, looped via jq). Adapt the script to whatever CLI exists at the time; the test fixture mocks the call so the test still passes regardless.

- [ ] **Step 2: Write `dump-topology.sh`**

`.claude-plugin/skills/kg-enrich/scripts/dump-topology.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: dump-topology.sh <domain> <source-id>" >&2
  exit 2
fi

domain="$1"
source_id="$2"

# Pull files + decls; we score entry points and compute fan in/out from imports/calls edges.
files=$(kg node list --domain "$domain" --layer file --source "$source_id" \
  | jq --arg src "$source_id" '
    .data | map({
      node_id: .id,
      name: .name,
      path: (.properties[$src].path // "")
    })
  ')

# Heuristic entry scoring: main.go +5, cmd/*/main.go +3.
entries=$(echo "$files" | jq '
  map(. + {
    score: (
      ([
        (if (.path | test("/main\\.go$")) then 5 else 0 end),
        (if (.path | test("/cmd/[^/]+/main\\.go$")) then 3 else 0 end)
      ] | add)
    )
  })
  | map(select(.score > 0))
  | sort_by(-.score)
')

imports=$(kg edge list --domain "$domain" --type imports 2>/dev/null \
  | jq 'if .data then .data | map({from: .source_id, to: .target_id}) else [] end' \
  || echo '[]')

# Fan-in / fan-out per file.
fanned=$(jq -n --argjson files "$files" --argjson imports "$imports" '
  $files | map(. as $f |
    $f + {
      fan_in:  ([$imports[] | select(.to == $f.node_id)]  | length),
      fan_out: ([$imports[] | select(.from == $f.node_id)] | length)
    }
  )
')

hotspots=$(echo "$fanned" | jq 'sort_by(-.fan_in) | .[0:10]')

jq -n \
  --argjson entries  "$entries" \
  --argjson hotspots "$hotspots" \
  --argjson edges    "$imports" \
  '{entries: $entries, hotspots: $hotspots, edges: $edges}'
```

Make executable. Same caveat about `kg edge list --type` — adapt if the CLI shape differs.

- [ ] **Step 3: Write fixtures**

`.claude-plugin/skills/kg-enrich/scripts/tests/fixtures/kg-node-list-packages.json`:

```json
{
  "data": [
    {
      "id": "myapp:cmd",
      "name": "cmd",
      "layer": "package",
      "source": "tree-sitter:0.2.0",
      "properties": { "tree-sitter:0.2.0": { "path": "/abs/cmd" } }
    },
    {
      "id": "myapp:internal-handler",
      "name": "handler",
      "layer": "package",
      "source": "tree-sitter:0.2.0",
      "properties": { "tree-sitter:0.2.0": { "path": "/abs/internal/handler" } }
    }
  ]
}
```

`.claude-plugin/skills/kg-enrich/scripts/tests/fixtures/kg-node-list-files-shape.json`:

```json
{
  "data": [
    {
      "id": "myapp:cmd/main-go",
      "name": "main.go",
      "layer": "file",
      "parent_id": "myapp:cmd",
      "source": "tree-sitter:0.2.0",
      "properties": { "tree-sitter:0.2.0": { "path": "/abs/cmd/main.go" } }
    },
    {
      "id": "myapp:internal-handler/serve-go",
      "name": "serve.go",
      "layer": "file",
      "parent_id": "myapp:internal-handler",
      "source": "tree-sitter:0.2.0",
      "properties": { "tree-sitter:0.2.0": { "path": "/abs/internal/handler/serve.go" } }
    }
  ]
}
```

`.claude-plugin/skills/kg-enrich/scripts/tests/fixtures/kg-edge-imports.json`:

```json
{
  "data": [
    { "source_id": "myapp:cmd/main-go", "target_id": "myapp:internal-handler/serve-go", "type": "imports" }
  ]
}
```

`.claude-plugin/skills/kg-enrich/scripts/tests/fixtures/expected-graph-shape.json`:

```json
{
  "packages": [
    {
      "slug": "myapp:cmd",
      "name": "cmd",
      "path": "/abs/cmd",
      "files": [
        { "node_id": "myapp:cmd/main-go", "path": "/abs/cmd/main.go", "name": "main.go" }
      ]
    },
    {
      "slug": "myapp:internal-handler",
      "name": "handler",
      "path": "/abs/internal/handler",
      "files": [
        { "node_id": "myapp:internal-handler/serve-go", "path": "/abs/internal/handler/serve.go", "name": "serve.go" }
      ]
    }
  ],
  "imports": [
    { "from": "myapp:cmd/main-go", "to": "myapp:internal-handler/serve-go" }
  ]
}
```

`.claude-plugin/skills/kg-enrich/scripts/tests/fixtures/expected-topology.json`:

```json
{
  "entries": [
    {
      "node_id": "myapp:cmd/main-go",
      "name": "main.go",
      "path": "/abs/cmd/main.go",
      "score": 8
    }
  ],
  "hotspots": [
    {
      "node_id": "myapp:internal-handler/serve-go",
      "name": "serve.go",
      "path": "/abs/internal/handler/serve.go",
      "fan_in": 1,
      "fan_out": 0
    },
    {
      "node_id": "myapp:cmd/main-go",
      "name": "main.go",
      "path": "/abs/cmd/main.go",
      "fan_in": 0,
      "fan_out": 1
    }
  ],
  "edges": [
    { "from": "myapp:cmd/main-go", "to": "myapp:internal-handler/serve-go" }
  ]
}
```

(Note: the topology fixture's hotspots ordering depends on `sort_by(-.fan_in)` — files with `fan_in: 1` come first, then `fan_in: 0`. Verify by running the test and adjust if jq's sort handles ties differently.)

- [ ] **Step 4: Write `dump-graph-shape.test.sh`**

`.claude-plugin/skills/kg-enrich/scripts/tests/dump-graph-shape.test.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

kg() {
  case "$*" in
    *"node list --domain myapp --layer package --source tree-sitter:0.2.0"*)
      cat fixtures/kg-node-list-packages.json ;;
    *"node list --domain myapp --layer file --source tree-sitter:0.2.0"*)
      cat fixtures/kg-node-list-files-shape.json ;;
    *"edge list --domain myapp --type imports"*)
      cat fixtures/kg-edge-imports.json ;;
    *) echo "unexpected kg call: $*" >&2; exit 1 ;;
  esac
}
export -f kg

actual=$(../dump-graph-shape.sh myapp tree-sitter:0.2.0)
expected=$(cat fixtures/expected-graph-shape.json)
diff <(echo "$actual" | jq -S .) <(echo "$expected" | jq -S .) \
  || { echo "FAIL dump-graph-shape.sh"; exit 1; }
echo "OK dump-graph-shape.sh"
```

Make executable.

- [ ] **Step 5: Write `dump-topology.test.sh`**

`.claude-plugin/skills/kg-enrich/scripts/tests/dump-topology.test.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

kg() {
  case "$*" in
    *"node list --domain myapp --layer file --source tree-sitter:0.2.0"*)
      cat fixtures/kg-node-list-files-shape.json ;;
    *"edge list --domain myapp --type imports"*)
      cat fixtures/kg-edge-imports.json ;;
    *) echo "unexpected kg call: $*" >&2; exit 1 ;;
  esac
}
export -f kg

actual=$(../dump-topology.sh myapp tree-sitter:0.2.0)
expected=$(cat fixtures/expected-topology.json)
diff <(echo "$actual" | jq -S .) <(echo "$expected" | jq -S .) \
  || { echo "FAIL dump-topology.sh"; exit 1; }
echo "OK dump-topology.sh"
```

Make executable.

- [ ] **Step 6: Run the new tests**

```bash
make test-scripts
```

Expected: 5 `OK` lines (the 3 from Task 5 + the 2 new ones).

If the topology test fails due to jq sort ordering of ties, capture the actual output, hand-verify it's correct, then update the fixture (this is jq behavior, not a script bug):

```bash
bash .claude-plugin/skills/kg-enrich/scripts/tests/dump-topology.test.sh
```

- [ ] **Step 7: Commit**

```bash
git add .claude-plugin/skills/kg-enrich/scripts
git commit -m "feat(script): graph-shape + topology dumpers with fixture tests"
```

---

## Phase 3 — Agents

Three subagent definitions. Each agent is a markdown file with frontmatter declaring `name`, `description`, `model: inherit`, `allowed-tools`. The body is the system prompt the subagent runs under — it must be explicit, contain output examples, list invariants, and prescribe the deterministic-then-LLM two-phase contract. UA's `understand-anything-plugin/agents/file-analyzer.md` is the canonical reference for shape.

### Task 7: `agents/file-summarizer.md` — per-decl summaries + semantic edges

The biggest agent. Annotates a batch of file/decl nodes with summaries, tags, complexity, and emits semantic edges between decls. Dispatched 5-in-parallel by `/kg-enrich` Phase 3.

**Files:**
- Create: `.claude-plugin/agents/file-summarizer.md`

- [ ] **Step 1: Write the frontmatter + intro section**

`.claude-plugin/agents/file-summarizer.md` starts with:

```markdown
---
name: file-summarizer
description: Reads a batch of source files referenced by kg file/decl nodes, generates per-decl summaries, tags, complexity ratings, and semantic edges, then emits a kg snapshot piped through `kg apply --source kg-summary:0.1.0 --scope additive`. One agent instance handles one batch of ~25 files. Use when /kg-enrich dispatches Phase 3 file-summarization.
model: inherit
allowed-tools: Read, Bash
---

# file-summarizer

You annotate a batch of file/decl nodes in a kg knowledge graph with summaries, tags, complexity, and semantic edges. You **never create new structural nodes** — you only write properties on existing nodes (in your own source namespace) and add semantic edges between existing nodes.

## Input contract

Your dispatcher passes a JSON object via prompt with this shape:

\`\`\`json
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
\`\`\`
```

- [ ] **Step 2: Write the two-phase workflow section**

Append:

```markdown
## Workflow — two phases

### Phase 1 — Deterministic (read)

For each file in `files`:

1. Use the `Read` tool on `file_path`. If the file is >2000 lines, read only the line ranges spanning all `decls` (extend by ±10 lines for context).
2. For each decl, extract the exact `line_range` excerpt. Keep a per-decl buffer.

### Phase 2 — LLM judgment (synthesize + emit)

For each file:

1. Write one **file-level** summary in 1-3 sentences. State what this file does in the project's context. No filler words ("This file contains…"); start with a verb.
2. For each decl, write:
   - `summary`: 1-2 sentence behavior description. Start with a verb. Reference parameters and return values where they're load-bearing.
   - `tags`: 1-4 short strings from a permissive vocabulary (examples: `api`, `handler`, `validator`, `entrypoint`, `helper`, `generated`, `crypto`, `db`, `auth`). Lowercase, kebab-case.
   - `complexity`: one of `trivial`, `simple`, `moderate`, `complex`. Use cyclomatic complexity intuitively (≤3 branches: simple, 4-7: moderate, 8+: complex; getters/setters: trivial).
3. Identify **semantic edges** between decls. Use only the curated vocabulary in the next section. Each edge: `{src, target, type}`. Endpoints MUST be `node_id`s present in the input batch — never invent IDs or reference nodes from other batches.
```

- [ ] **Step 3: Write the edge vocabulary section**

Append:

```markdown
## Semantic edge vocabulary

Emit ONLY these types. Unknown types are accepted by the engine but pollute the graph — discipline yourself.

| Type | Direction | When to use |
|---|---|---|
| `depends_on` | A → B | A's correctness depends on B's specific behavior (not just A imports B). |
| `implements` | A → B | A is a concrete implementation of interface/contract B. |
| `exposes` | A → B | A makes B accessible to callers (e.g., handler exposes service endpoint). |
| `documented_in` | A → B | A is described in B (typically a `.md` file node). |
| `configured_by` | A → B | A reads runtime config from B (config file, env var loader). |
| `uses` | A → B | Weaker than `depends_on`; A invokes B but isn't blocked by B. |
| `extends` | A → B | A extends or inherits B's structure (composition or embedding). |
| `tested_by` | A → B | A's correctness is verified by test B. |

Do NOT emit:
- Structural edges (`contains`, `child_of`, `imports`, `calls`) — tree-sitter already owns those. Re-claiming them under your source clutters the edge_claims table.
- `teaches` or `is_layer` — those belong to `tour-builder` and `architecture-analyzer`.
```

- [ ] **Step 4: Write the output snapshot section**

Append:

```markdown
## Output snapshot

After processing every file in the batch, emit ONE snapshot JSON and pipe it to `kg apply`. Shape:

\`\`\`json
{
  "protocol_version": 2,
  "source": "kg-summary:0.1.0",
  "domain": "<input.domain>",
  "scope": "additive",
  "nodes": [
    {
      "id": "myapp:graph/handler-go",
      "properties": {
        "summary": "HTTP handler for /api/users.",
        "tags": ["api", "http", "users"],
        "complexity": "moderate",
        "language_notes": "Uses gorilla/mux router pattern."
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
    {"src": "myapp:graph/handler-go::serve", "target": "myapp:service/user-go::get", "type": "depends_on"}
  ]
}
\`\`\`

### Output invariants — verify before piping

- `nodes[].id` MUST be from your input batch. If you find yourself wanting an ID not in `files[].node_id` or `files[].decls[].node_id`, stop and drop it.
- `nodes[]` entries omit `layer`, `parent`, `name`. The engine in additive scope writes properties only; structural fields belong to `tree-sitter:0.2.0`.
- `edges[].src` and `edges[].target` MUST be existing node IDs in the kg graph. The engine rejects unknown IDs.
- `edges[].type` MUST come from the vocabulary table above.
- `protocol_version` MUST be `2`. `source` MUST be exactly `kg-summary:0.1.0`. `scope` MUST be `additive`.

## Piping to kg apply

\`\`\`bash
echo "$snapshot_json" \
  | bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/apply-snapshot.sh" \
      kg-summary:0.1.0 "<input.domain>" additive
\`\`\`
```

- [ ] **Step 5: Write the retry + edge-case section**

Append:

```markdown
## Retry on apply failure

If `kg apply` exits non-zero, read the error envelope from stderr (look for an `ok: false` JSON with `code` and `message`). Common codes:

- `NODE_NOT_FOUND`: an edge endpoint isn't in the graph. Drop the offending edge and re-emit.
- `INVALID_NODE_ID`: an `id` field doesn't match the slug grammar. Fix the offending entry.
- `SOURCE_MISMATCH` / `DOMAIN_MISMATCH`: your `--source` or `--domain` flag disagreed with the snapshot JSON. Make them match.

Retry **once**. If the second apply also fails, exit with a non-zero status and emit a structured error summary to stdout:

\`\`\`json
{ "batch_id": 3, "status": "failed", "reason": "<error code>", "details": "<message>" }
\`\`\`

The orchestrator (/kg-enrich SKILL) reads this and continues other batches.

## Edge cases

- **File with zero decls:** emit only the file-level node entry (with `summary`, `tags`, `complexity`). Skip the `decls` array entirely.
- **All-generated file (`_gen.go`, `_pb.go`, `*-mock.go`):** tag as `["generated"]` and set `summary: "Generated code."`. Skip per-decl analysis.
- **File >2000 lines:** in Phase 1, read only the union of decl `line_range`s (extended ±10 lines). Avoid loading the whole file.
- **Empty function bodies:** still emit a summary based on the signature ("Returns the stored user; called from handler.Serve.").
- **Test files:** rate `complexity: trivial` unless the test itself contains nontrivial setup. Emit `tested_by` edges from the tested decl to the test decl, not the reverse.

## What success looks like

After your batch's `kg apply` succeeds, the graph contains:
- A property block under `kg-summary:0.1.0` on each batch node (file-level and decl-level) with `summary`, `tags`, `complexity`.
- Semantic edges between decls claimed by `kg-summary:0.1.0`.
- Tree-sitter's structural data is untouched.

Verify by spot-checking one node:

\`\`\`bash
kg node get myapp:graph/handler-go --source kg-summary:0.1.0
\`\`\`

Expected: a flat object with the summary, tags, complexity fields you wrote.
```

- [ ] **Step 6: Sanity-check the file (length, frontmatter parse)**

```bash
wc -l .claude-plugin/agents/file-summarizer.md
head -10 .claude-plugin/agents/file-summarizer.md
```

Expected: ~150-250 lines (matches UA's `file-analyzer.md` ballpark). Frontmatter parses as YAML (no stray tabs, terminating `---`).

- [ ] **Step 7: Commit**

```bash
git add .claude-plugin/agents/file-summarizer.md
git commit -m "feat(agent): file-summarizer.md — per-decl summaries + semantic edges"
```

---

### Task 8: `agents/architecture-analyzer.md`

Single-instance agent. Reads a `graph-shape.json` (per-package import adjacency + directory grouping) plus the project README. Infers 3-10 architectural layer names, assigns every file to exactly one layer. Output: new domain `<orig>-arch` with `layer` nodes and cross-domain `contains` edges pointing to original-domain file nodes.

**Files:**
- Create: `.claude-plugin/agents/architecture-analyzer.md`

- [ ] **Step 1: Write the frontmatter + intro**

```markdown
---
name: architecture-analyzer
description: Reads a kg-derived graph shape (per-package import adjacency, directory tree) plus the project README, then synthesizes 3-10 architectural layer names and assigns every file node to exactly one layer. Emits a snapshot creating an `<orig>-arch` domain with `layer` nodes and cross-domain `contains` edges to original-domain file nodes. Single instance per /kg-enrich invocation, runs after file-summarizer batches complete.
model: inherit
allowed-tools: Read, Bash
---

# architecture-analyzer

You infer the architectural layers of a codebase from its package structure and import topology, then attribute each file to its layer.

## Input contract

\`\`\`json
{
  "domain": "myapp",
  "structural_source": "tree-sitter:0.2.0",
  "graph_shape_path": "/abs/path/.kg-enrich-tmp/graph-shape.json"
}
\`\`\`
```

- [ ] **Step 2: Write the workflow section**

Append:

```markdown
## Workflow

### Phase 1 — Deterministic (read)

1. Use `Read` on `graph_shape_path`. The file's shape:

\`\`\`json
{
  "packages": [
    { "slug": "myapp:cmd", "name": "cmd", "path": "/abs/cmd", "files": [ { "node_id": "myapp:cmd/main-go", "path": "/abs/cmd/main.go", "name": "main.go" } ] }
  ],
  "imports": [ { "from": "myapp:cmd/main-go", "to": "myapp:internal-handler/serve-go" } ]
}
\`\`\`

2. Use `Read` on `<repo>/README.md` if present (project root inferred from any package path). Look for explicit architectural language: "API layer", "service layer", "repository", "domain model", "infrastructure". These hints take priority over your inference.

### Phase 2 — LLM judgment

1. Name 3-10 layers. Prefer concrete names ("HTTP API Layer", "Domain Logic", "Persistence") over abstract ones ("Module A"). Lowercase-kebab-case the slugs (`http-api-layer`, `domain-logic`, `persistence`).
2. Assign every file in `packages[].files[]` to exactly one layer. Files MUST NOT belong to two layers. If a file genuinely spans concerns, pick the dominant one — don't invent a "Mixed" layer.
3. Order layers top-to-bottom by dependency flow (entry points first, lowest-level utilities last). Encode the order as `properties.order: 1..N`.
```

- [ ] **Step 3: Write the output + invariants section**

Append:

````markdown
## Output snapshot

\`\`\`json
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
\`\`\`

### Output invariants

- `domain` is exactly `<orig>-arch` (input domain + `-arch` suffix).
- `domain_spec.layers` is exactly `["layer"]` — one layer name only ("layer"), with N nodes inside it.
- Every layer node ID matches `<orig>-arch:<kebab-case-slug>`. The slug MUST be unique within the snapshot.
- Every `contains` edge has `src` in the `-arch` domain and `target` in the original domain. Cross-domain edges are intentional.
- `properties.order` is 1..N with no gaps and no duplicates.
- `scope: domain-source` ensures re-running cleanly replaces the previous architecture (last apply wins).

## Piping

\`\`\`bash
echo "$snapshot_json" \
  | bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/apply-snapshot.sh" \
      kg-arch:0.1.0 "<orig>-arch" domain-source
\`\`\`

## Retry on apply failure

Same retry policy as file-summarizer: read error envelope, fix, re-apply once. Second failure → exit non-zero with `{ "status": "failed", "reason": ..., "details": ... }`.

## Edge cases

- **<3 packages total:** still emit at least 2 layers (typically "Entry" + "Library"). Don't degenerate to 1.
- **Project with no README:** infer purely from structure. Don't make up project context that isn't there.
- **Files matching multiple plausible layers:** prefer the layer the file's `package` most strongly evokes (e.g., `internal/storage/` → Persistence even if a file does some validation).
- **Layer assignment is incomplete:** the snapshot MUST contain a `contains` edge for every file in `packages[].files[]`. Verify before emitting.
````

- [ ] **Step 4: Commit**

```bash
git add .claude-plugin/agents/architecture-analyzer.md
git commit -m "feat(agent): architecture-analyzer.md — infers layers + emits <orig>-arch domain"
```

---

### Task 9: `agents/tour-builder.md`

Single-instance agent. Reads topology (fan-in/fan-out, entry-point scores), optionally arch layers, and produces 5-15 ordered tour steps that teach the codebase. Output: new domain `<orig>-tours` with `step` nodes and cross-domain `teaches` edges.

**Files:**
- Create: `.claude-plugin/agents/tour-builder.md`

- [ ] **Step 1: Write the frontmatter + intro**

```markdown
---
name: tour-builder
description: Reads kg topology (entry-point ranking, fan-in/fan-out, import chains) and architectural layers, then designs 5-15 ordered tour steps that teach a new contributor the codebase in dependency order. Emits a snapshot creating an `<orig>-tours` domain with `step` nodes and cross-domain `teaches` edges to file/decl nodes. Single instance per /kg-enrich invocation; runs after architecture-analyzer.
model: inherit
allowed-tools: Read, Bash
---

# tour-builder

You design a step-by-step learning path through a codebase. Output: a `step` node per stop in the tour, ordered, with `teaches` edges pointing at the file/decl nodes that step covers.

## Input contract

\`\`\`json
{
  "domain": "myapp",
  "structural_source": "tree-sitter:0.2.0",
  "arch_domain": "myapp-arch",
  "topology_path": "/abs/path/.kg-enrich-tmp/topology.json"
}
\`\`\`
```

- [ ] **Step 2: Write the workflow section**

Append:

````markdown
## Workflow

### Phase 1 — Deterministic (read)

1. Use `Read` on `topology_path`. Shape:

\`\`\`json
{
  "entries":  [ { "node_id": "myapp:cmd/main-go", "name": "main.go", "path": "/abs/cmd/main.go", "score": 8 } ],
  "hotspots": [ { "node_id": "myapp:internal-handler/serve-go", "fan_in": 5, "fan_out": 0 } ],
  "edges":    [ { "from": "myapp:cmd/main-go", "to": "myapp:internal-handler/serve-go" } ]
}
\`\`\`

2. If `arch_domain` is provided, run `kg node list --domain <arch_domain>` to get the architectural layer names + their `contains` edges. Use them to scaffold tour order: usually `Entry → API → Service → Persistence → Utilities`.

3. Pick top 1-3 entries (highest score). They open the tour.

### Phase 2 — Design tour steps

1. Plan 5-15 steps. Each step covers 1 conceptual chunk (~5-10 minutes of reading for a new contributor).
2. Order steps by **dependency-aware narrative**: start at entry points, follow imports outward to leaves. Don't jump architectural layers without a transition step.
3. Each step references 1-3 nodes via `teaches` edges. Usually 1 file + 1-2 of its key decls. A node may appear in multiple steps (e.g., main.go appears in "Project Overview" AND "Entry Point Wiring") — that's fine.
4. Write `properties.description` as one paragraph telling the reader what to look for and why this step matters.
5. Estimate `properties.estimated_minutes` honestly (3-15 per step).
````

- [ ] **Step 3: Write the output + invariants section**

Append:

````markdown
## Output snapshot

\`\`\`json
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
        "description": "Start with the README and main.go to understand the high-level shape. Pay attention to the package layout — `cmd/` is the entry, `internal/` holds the implementation.",
        "estimated_minutes": 5
      }
    },
    {
      "id": "myapp-tours:02-entry-point",
      "layer": "step",
      "name": "Entry Point",
      "properties": {
        "order": 2,
        "description": "Trace startup from main() through dependency injection. Note how the HTTP server is configured and which handlers it mounts.",
        "estimated_minutes": 10
      }
    }
  ],
  "edges": [
    { "src": "myapp-tours:01-overview", "target": "myapp:docs/readme-md", "type": "teaches" },
    { "src": "myapp-tours:02-entry-point", "target": "myapp:cmd/cmd-main-go::main", "type": "teaches" }
  ]
}
\`\`\`

### Output invariants

- `domain` is exactly `<orig>-tours`.
- `domain_spec.layers` is exactly `["step"]`.
- Step node IDs: `<orig>-tours:NN-kebab-slug` where `NN` is zero-padded order (`01`, `02`, …). Slug describes the step in 1-3 words.
- `properties.order` is 1..N with no gaps. Matches the `NN` in the ID.
- `properties.description` is one paragraph (3-6 sentences).
- `properties.estimated_minutes` is an integer 3-15.
- Every `teaches` edge has `src` in the `-tours` domain and `target` in the original domain.
- Every step has at least one `teaches` edge.
- 5 ≤ total step count ≤ 15.

## Piping

\`\`\`bash
echo "$snapshot_json" \
  | bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/apply-snapshot.sh" \
      kg-tours:0.1.0 "<orig>-tours" domain-source
\`\`\`

## Retry on apply failure

Same as the other agents: read error envelope, fix, retry once, then fail clean.

## Edge cases

- **Tiny project (<5 files):** still emit at least 3 steps. Combine related files into one step rather than dropping below 3.
- **No clear entry point** (no main.go, no cmd/):  start with the README, then the highest-fan-in file. Document the heuristic in the step description so the reader knows.
- **Very large project (>500 files):** target 10-15 steps. Each step covers a higher-level concept; don't try to mention every file.
- **Architecture domain missing:** still produce a tour, just based purely on topology. Don't fail.
````

- [ ] **Step 4: Commit**

```bash
git add .claude-plugin/agents/tour-builder.md
git commit -m "feat(agent): tour-builder.md — designs ordered tour steps + emits <orig>-tours domain"
```

---

## Phase 4 — Skills

Four skills. `/kg-enrich` is the headline orchestrator (long, ~600 lines of prompt). The other three are smaller (200-400 lines each). UA's `skills/understand/SKILL.md` is the pattern for the orchestrator.

### Task 10: `skills/kg-enrich/SKILL.md` — orchestrator

The orchestrator. Six phases: pre-check → list files → batch → dispatch 5×file-summarizer waves → dispatch architecture-analyzer → dispatch tour-builder → summary report.

**Files:**
- Create: `.claude-plugin/skills/kg-enrich/SKILL.md`

- [ ] **Step 1: Frontmatter + intro**

```markdown
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
```

- [ ] **Step 2: Pre-check section**

Append:

````markdown
## Pre-check

Run these in sequence; abort with a clear error if any fail.

1. **`kg` on PATH:**
   \`\`\`bash
   kg --version
   \`\`\`
   On failure: tell user "kg CLI not found. Install: cd into the kg repo and run `make install`."

2. **`kg.db` exists:**
   \`\`\`bash
   test -f "${KG_DB:-./kg.db}"
   \`\`\`
   On failure: "No kg.db in cwd. Run `kg init` first, then extract structural data with `kg-extractor extract ...`."

3. **Detect domain (if --domain omitted):**
   \`\`\`bash
   kg domain list
   \`\`\`
   If exactly one domain: use it. If multiple: ask the user. If zero: tell the user to extract first.

4. **Source has nodes in that domain:**
   \`\`\`bash
   kg node list --domain "<domain>" --layer file --source "<source>" --limit 1
   \`\`\`
   On empty: "Source '<source>' has no file nodes in domain '<domain>'. Did you run kg-extractor? Or did you mean a different --source?"

5. **Create scratch dir:**
   \`\`\`bash
   mkdir -p .kg-enrich-tmp
   \`\`\`
````

- [ ] **Step 3: Phase 1-3 — dump + batch + dispatch summarizers**

Append:

````markdown
## Phase 1 — Dump file list

\`\`\`bash
bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/dump-files.sh" \
  "<domain>" "<source>" > .kg-enrich-tmp/files.json
\`\`\`

Inspect `.kg-enrich-tmp/files.json`. Count entries. If `--max-files N` was passed, truncate the list to N before batching.

## Phase 2 — Batch

Split `files.json` into batches of ~25 (configurable; adjust upward to 30 for tiny files, downward to 15 for files with many decls).

For each batch N:

1. Write `.kg-enrich-tmp/batch-N-files.json` (the batch's slice of `files.json`).
2. Run `dump-batch-context.sh` to enrich with per-file decl info:
   \`\`\`bash
   bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/dump-batch-context.sh" \
     ".kg-enrich-tmp/batch-N-files.json" "<source>" \
     > ".kg-enrich-tmp/batch-N-input.json"
   \`\`\`

## Phase 3 — Dispatch file-summarizer (5 parallel)

For each wave of up to 5 batches, dispatch concurrently. Use a **single message** with multiple Task tool invocations (this is required to get parallel execution — sequential messages run serially).

Each dispatch:

\`\`\`
Task(
  subagent_type="file-summarizer",
  description="Enrich batch N",
  prompt=<<contents of .kg-enrich-tmp/batch-N-input.json plus a one-line preamble: "You are batch N of M. Process every file in this batch. Pipe your snapshot to: bash ${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/apply-snapshot.sh kg-summary:0.1.0 <domain> additive">>
)
\`\`\`

Collect results. Track `succeeded[]` and `failed[]` lists of batch IDs.

**Failure handling:** if an agent returns `{"status": "failed", "reason": ...}`, log it and continue. Do not retry within this phase — the user gets a chance to retry from the summary report.
````

- [ ] **Step 4: Phase 4-5 — architecture + tour**

Append:

````markdown
## Phase 4 — Dispatch architecture-analyzer

Generate graph shape input:

\`\`\`bash
bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/dump-graph-shape.sh" \
  "<domain>" "<source>" > .kg-enrich-tmp/graph-shape.json
\`\`\`

Dispatch single agent:

\`\`\`
Task(
  subagent_type="architecture-analyzer",
  description="Infer architectural layers for <domain>",
  prompt='{"domain": "<domain>", "structural_source": "<source>", "graph_shape_path": "<abs-path-to>/.kg-enrich-tmp/graph-shape.json"}'
)
\`\`\`

If it fails: record the failure but DO NOT abort. Tour-builder can run without arch.

## Phase 5 — Dispatch tour-builder

Generate topology:

\`\`\`bash
bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/dump-topology.sh" \
  "<domain>" "<source>" > .kg-enrich-tmp/topology.json
\`\`\`

Dispatch:

\`\`\`
Task(
  subagent_type="tour-builder",
  description="Build onboarding tour for <domain>",
  prompt='{"domain": "<domain>", "structural_source": "<source>", "arch_domain": "<domain>-arch", "topology_path": "<abs-path-to>/.kg-enrich-tmp/topology.json"}'
)
\`\`\`

(If architecture-analyzer failed, omit `arch_domain` from the prompt — tour-builder degrades gracefully.)
````

- [ ] **Step 5: Phase 6 — summary + cleanup**

Append:

````markdown
## Phase 6 — Summary report

Compute and print:

\`\`\`bash
nodes_enriched=$(kg node list --domain "<domain>" --source kg-summary:0.1.0 --limit 0 | jq '.data | length')
arch_layers=$(kg node list --domain "<domain>-arch" --source kg-arch:0.1.0 --limit 0 2>/dev/null | jq '.data | length // 0')
tour_steps=$(kg node list --domain "<domain>-tours" --source kg-tours:0.1.0 --limit 0 2>/dev/null | jq '.data | length // 0')
\`\`\`

Print to user:

\`\`\`
/kg-enrich complete for domain <domain>:
  ✓ file-summarizer: <succeeded.length>/<batch_count> batches
  ✓ architecture-analyzer: <ok|failed>
  ✓ tour-builder: <ok|failed>

Graph deltas:
  nodes enriched (kg-summary:0.1.0): <nodes_enriched>
  semantic edges added: <count from kg edge list ... > (heuristic: use `kg edge list --source kg-summary:0.1.0 | jq '.data | length'`)
  arch layers (<domain>-arch): <arch_layers>
  tour steps (<domain>-tours): <tour_steps>

Failures: <list of failed batch IDs with reasons, or "none">

Next steps:
- /kg-onboard --domain <domain> — generate docs/ONBOARDING.md
- /kg-explain <node-id> — ask Claude about a specific node
- /kg-tour --domain <domain> — re-run tour-builder only
\`\`\`

If there were failures: prompt the user via AskUserQuestion: "Retry N failed batches?" If yes, re-dispatch only those.

## Cleanup

Leave `.kg-enrich-tmp/` in place — it's useful for debugging. Document it in the user-facing summary: "Intermediate files in .kg-enrich-tmp/ (safe to delete)."

## Idempotency

Re-running `/kg-enrich` overwrites all property/edge contributions in this source's namespace. Tree-sitter's data is untouched (different source ID, different namespace).
````

- [ ] **Step 6: Sanity-check length**

```bash
wc -l .claude-plugin/skills/kg-enrich/SKILL.md
```

Expected: ~250-400 lines.

- [ ] **Step 7: Commit**

```bash
git add .claude-plugin/skills/kg-enrich/SKILL.md
git commit -m "feat(skill): kg-enrich SKILL.md — 6-phase orchestrator"
```

---

### Task 11: `skills/kg-explain/SKILL.md` — read-only single-node Q&A

Read-only. Pulls a node + its 1-hop neighbors with merged properties, then Claude answers in-prompt (no subagent).

**Files:**
- Create: `.claude-plugin/skills/kg-explain/SKILL.md`

- [ ] **Step 1: Write the file**

```markdown
---
name: kg-explain
description: Read-only. Answers questions about a specific kg node using its enriched properties + 1-hop neighborhood. No graph mutation. Use when the user wants to understand what a specific function, file, or package does in context.
---

# /kg-explain

Explain a kg node using all available enrichment (tree-sitter structure + LLM summaries) plus its immediate graph neighborhood.

## Arguments

`$ARGUMENTS` is the node ID, e.g., `myapp:graph/handler-go::serve`.

If empty or malformed: ask the user to provide a node ID. Suggest: `kg node list --domain <some-domain> --limit 20` to discover candidates.

## Workflow

1. **Fetch the node with merged properties:**
   \`\`\`bash
   kg node get "<node-id>" --merged
   \`\`\`
   On `NODE_NOT_FOUND`: tell the user and suggest `kg node list` to find similar IDs.

2. **Fetch outgoing edges (and their targets' merged properties):**
   \`\`\`bash
   kg edge list-from "<node-id>"
   for target in $(kg edge list-from "<node-id>" | jq -r '.data[].target_id'); do
     kg node get "$target" --merged
   done
   \`\`\`

3. **Fetch incoming edges (and their sources' merged properties):**
   Same as above with `kg edge list-to`.

4. **Synthesize the answer** in 3-6 paragraphs:
   - **What it does:** one paragraph based on the node's own `summary` + signature.
   - **How it fits in:** one paragraph describing the 1-hop neighborhood (who calls it, who it calls, what it implements/extends).
   - **What to read next:** 2-4 bullet links to neighbor node IDs, ordered by relevance (highest = direct dependencies for understanding).
   - **Tour position (optional):** if any `myapp-tours:` step has a `teaches` edge to this node, mention which step covers it.

## Output format

Print as markdown to the user. Use code blocks for IDs. Don't pipe to a file — the user is asking a question, not generating documentation.

## Edge cases

- **Node has no enrichment yet** (only tree-sitter data): say so. Suggest `/kg-enrich --domain <domain>` first.
- **Node is in an unexpected domain** (e.g., `<orig>-arch:api-layer`): explain it's an architectural layer node, list the files it contains.
- **Node has no neighbors:** still explain based on properties alone. Don't fabricate connections.

## Non-goals

- Don't mutate the graph.
- Don't dispatch agents.
- Don't read source files unless the user explicitly asks (the enriched summaries are the answer).
```

- [ ] **Step 2: Commit**

```bash
git add .claude-plugin/skills/kg-explain/SKILL.md
git commit -m "feat(skill): kg-explain SKILL.md — read-only single-node Q&A"
```

---

### Task 12: `skills/kg-tour/SKILL.md` — standalone tour-builder trigger

Re-runs tour-builder without re-enriching. Useful when the user manually edited some summaries and wants a fresh tour.

**Files:**
- Create: `.claude-plugin/skills/kg-tour/SKILL.md`

- [ ] **Step 1: Write the file**

```markdown
---
name: kg-tour
description: Re-runs only the tour-builder agent against an already-enriched kg graph. Use when the user wants to regenerate /kg-onboard's source material without re-running file-summarizer or architecture-analyzer. Faster + cheaper than /kg-enrich.
---

# /kg-tour

Standalone re-trigger of tour-builder.

## Arguments

- `--domain <id>` (default: auto-detect single domain, else prompt)
- `--source <id>` (structural source; default `tree-sitter:0.2.0`)
- `--arch-domain <id>` (default: `<domain>-arch`; pass empty to skip)

## Pre-check

Same first 4 checks as /kg-enrich. Plus:
- Verify the structural source has nodes in `<domain>`.
- If `<arch-domain>` is non-empty, verify it has at least one `layer` node. If not, warn and continue without arch.

## Workflow

1. **Generate topology:**
   \`\`\`bash
   mkdir -p .kg-enrich-tmp
   bash "${CLAUDE_PLUGIN_ROOT}/skills/kg-enrich/scripts/dump-topology.sh" \
     "<domain>" "<source>" > .kg-enrich-tmp/topology.json
   \`\`\`

2. **Dispatch tour-builder:**
   Same Task dispatch as /kg-enrich Phase 5.

3. **Report:**
   \`\`\`bash
   kg node list --domain "<domain>-tours" --source kg-tours:0.1.0
   \`\`\`
   Print step count and the first 3 step names + descriptions as a preview.

## Idempotency

`scope: domain-source` ensures the previous tour is cleanly replaced. The previous step IDs disappear; the new ones may not match.

## Non-goals

- Don't touch summaries (file-summarizer's output).
- Don't touch architecture (architecture-analyzer's output).
- Don't generate ONBOARDING.md — that's /kg-onboard.
```

- [ ] **Step 2: Commit**

```bash
git add .claude-plugin/skills/kg-tour/SKILL.md
git commit -m "feat(skill): kg-tour SKILL.md — standalone tour-builder trigger"
```

---

### Task 13: `skills/kg-onboard/SKILL.md` — generate `docs/ONBOARDING.md`

Read-only. Queries the enriched graph and writes a markdown onboarding document.

**Files:**
- Create: `.claude-plugin/skills/kg-onboard/SKILL.md`

- [ ] **Step 1: Write the file**

````markdown
---
name: kg-onboard
description: Generates a markdown onboarding document (default path `docs/ONBOARDING.md`) from an enriched kg graph. Combines the project description, architectural overview, and tour steps with cross-references to file paths and decl summaries. Use after /kg-enrich.
---

# /kg-onboard

Generates `docs/ONBOARDING.md` (or a user-specified path) from the kg graph.

## Arguments

- `--domain <id>` (default: auto-detect or prompt)
- `--output <path>` (default: `docs/ONBOARDING.md`)
- `--arch-domain <id>` (default: `<domain>-arch`)
- `--tours-domain <id>` (default: `<domain>-tours`)

## Pre-check

- `kg --version` works
- `<domain>` exists
- `<tours-domain>` has at least one step (otherwise the doc would be skeletal). If empty, tell user to run /kg-enrich or /kg-tour first.

## Workflow

1. **Project header.** Read the top-layer node (usually `package`):
   \`\`\`bash
   kg node list --domain "<domain>" --layer package --limit 1
   \`\`\`
   Use its `name` as the H1 title. Use its `kg-summary:0.1.0.summary` (if any) as the intro paragraph.

2. **Architecture section.** If `<arch-domain>` exists:
   \`\`\`bash
   kg node list --domain "<arch-domain>" --source kg-arch:0.1.0
   \`\`\`
   Sort by `properties.order`. For each layer, emit a subsection with its `description` and a bullet list of the file paths it `contains`:
   \`\`\`bash
   kg edge list-from "<layer-node-id>" --type contains
   \`\`\`
   For each `target`, fetch its merged properties (file path comes from `tree-sitter:0.2.0`).

3. **Tour section.** Pull steps sorted by `order`:
   \`\`\`bash
   kg node list --domain "<tours-domain>" --source kg-tours:0.1.0
   \`\`\`
   For each step:
   - H3 heading: `Step N — <name> (~M minutes)`
   - The `description` paragraph
   - Bullet list of `teaches` targets with their summaries:
     \`\`\`bash
     kg edge list-from "<step-node-id>" --type teaches
     \`\`\`
     For each target, fetch `kg-summary:0.1.0.summary` and the file path.

4. **Write the file.** Confirm the path with the user before writing if it would overwrite an existing file. Use the `Write` tool.

## Output template

\`\`\`markdown
# <Project Name>

<intro paragraph from package summary>

## Architecture

<for each layer in order:>
### <Layer name>

<layer description>

Files:
- `<file path>` — <file summary>

## Tour

<for each step:>
### Step N — <step name> (~M minutes)

<step description>

Covers:
- `<file path>` (`<node id>`) — <file summary>
\`\`\`

## Edge cases

- **Project has no package-layer summary:** use the directory name and a one-line synthesized intro ("`<project>` is a Go codebase with `<arch_layers_count>` architectural layers and `<file_count>` source files.").
- **No architecture domain:** skip the Architecture section entirely.
- **Existing ONBOARDING.md:** ask the user before overwriting via AskUserQuestion.

## Non-goals

- Don't fetch source files. The summaries are the authoritative content.
- Don't dispatch agents.
- Don't mutate the graph.
````

- [ ] **Step 2: Commit**

```bash
git add .claude-plugin/skills/kg-onboard/SKILL.md
git commit -m "feat(skill): kg-onboard SKILL.md — generates docs/ONBOARDING.md from enriched graph"
```

---

## Phase 5 — Tests, smoke, polish, branch close-out

The hardest parts of v3 to test mechanically: skills and agents are LLM-driven. We get partial coverage via the script tests (Phase 2) and engine tests (Phase 1); the rest is a tiny fixture + a manual smoke trace + an opt-in e2e behind a build tag.

### Task 14: `testdata/v3-fixture/` + manual smoke trace

A 5-file Go project that exercises the full pipeline end-to-end. After running `/kg-enrich` against it manually, capture the observed behavior in `docs/v3-fixture-trace.md` for future regression checks.

**Files:**
- Create: `testdata/v3-fixture/README.md`
- Create: `testdata/v3-fixture/main.go`
- Create: `testdata/v3-fixture/handler.go`
- Create: `testdata/v3-fixture/service.go`
- Create: `testdata/v3-fixture/store.go`
- Create: `docs/v3-fixture-trace.md`

- [ ] **Step 1: Create the fixture project**

`testdata/v3-fixture/README.md`:

```markdown
# v3-fixture

Tiny Go project used as a smoke-test target for `/kg-enrich`. Three architectural layers:
- HTTP (handler.go)
- Service (service.go)
- Storage (store.go)

main.go wires them. Five files total including this README.
```

`testdata/v3-fixture/main.go`:

```go
package main

import (
	"log"
	"net/http"
)

func main() {
	store := NewStore()
	service := NewService(store)
	handler := NewHandler(service)
	log.Fatal(http.ListenAndServe(":8080", handler))
}
```

`testdata/v3-fixture/handler.go`:

```go
package main

import (
	"encoding/json"
	"net/http"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	user, err := h.svc.GetUser(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(user)
}
```

`testdata/v3-fixture/service.go`:

```go
package main

import "context"

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) GetUser(ctx context.Context, id string) (*User, error) {
	return s.store.Find(ctx, id)
}
```

`testdata/v3-fixture/store.go`:

```go
package main

import "context"

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Store struct {
	rows map[string]*User
}

func NewStore() *Store {
	return &Store{rows: map[string]*User{
		"1": {ID: "1", Name: "Alice"},
	}}
}

func (s *Store) Find(_ context.Context, id string) (*User, error) {
	return s.rows[id], nil
}
```

- [ ] **Step 2: Manually run the pipeline**

```bash
make build build-extractor build-plugin-treesitter

mkdir -p ~/.config/kg-extractor/plugins/tree-sitter
cp plugins/tree-sitter/manifest.json ~/.config/kg-extractor/plugins/tree-sitter/
cp ./bin/kg-extractor-tree-sitter ~/.config/kg-extractor/plugins/tree-sitter/

rm -f /tmp/v3-smoke.db
./bin/kg --db /tmp/v3-smoke.db init
./bin/kg-extractor extract \
  --plugin tree-sitter --language go \
  --input ./testdata/v3-fixture --domain fixture \
  --db /tmp/v3-smoke.db --kg-binary ./bin/kg

./bin/kg --db /tmp/v3-smoke.db node list --domain fixture --layer file
```

Expected: 4 file nodes (one per `.go` file). README.md is not extracted by tree-sitter's Go plugin (it's a docs file).

Open a fresh Claude Code session in the kg repo, run `/kg-enrich --domain fixture` against `/tmp/v3-smoke.db` (set `KG_DB=/tmp/v3-smoke.db`).

Watch carefully:
- file-summarizer dispatches once (4 files < batch size 25, so one batch).
- architecture-analyzer produces 2-3 layers.
- tour-builder produces 3-5 steps.
- Final summary shows nonzero counts.

- [ ] **Step 3: Capture the trace**

Write `docs/v3-fixture-trace.md`:

```markdown
# v3-fixture smoke trace

Captured on YYYY-MM-DD by running `/kg-enrich --domain fixture` against `testdata/v3-fixture/`.

## Pre-enrichment state

\`\`\`
$ kg node list --domain fixture --source tree-sitter:0.2.0 --limit 0 | jq '.data | length'
<N file nodes + N decl nodes + N package nodes>
\`\`\`

## After /kg-enrich

### file-summarizer
- 1 batch (4 files)
- Succeeded
- Summary samples:
  - `fixture:fixture/handler-go`: <observed summary>
  - `fixture:fixture/handler-go::servehttp`: <observed summary>
- Semantic edges added: <observed count> (typical: handler::ServeHTTP `depends_on` service::GetUser)

### architecture-analyzer
- Layers (observed): <e.g., "HTTP Layer", "Service Layer", "Storage Layer">
- Layer-to-file `contains` edges: <count>

### tour-builder
- Steps (observed): <e.g., 3-5 steps starting with "Project Overview" → "Entry Point" → "HTTP Handler" → "Service Logic" → "Storage">

### Summary report
\`\`\`
<paste the literal report output from /kg-enrich>
\`\`\`

## Notes / surprises

<anything unexpected: hallucinated edges, wrong layer assignments, missing decls, etc. These guide future prompt tuning.>
```

Fill in actual observed values as you run the smoke test. Don't pre-fill with predictions — the trace's value is in what *actually* happened.

- [ ] **Step 4: Commit**

```bash
git add testdata/v3-fixture docs/v3-fixture-trace.md
git commit -m "test(e2e): v3-fixture project + manual smoke trace"
```

---

### Task 15: E2E test `e2e/enrich_self_test.go` (build tag, manual-trigger)

An automated end-to-end that runs `/kg-enrich` against kg's own `internal/graph` directory. Costs real LLM tokens; gated behind a build tag so it doesn't run on every PR.

**Files:**
- Create: `e2e/enrich_self_test.go`
- Modify: `Makefile` — add `e2e-enrich` target

- [ ] **Step 1: Write the test**

`e2e/enrich_self_test.go`:

```go
//go:build e2e_enrich

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnrichSelf(t *testing.T) {
	if os.Getenv("LLM_ENABLED") != "1" {
		t.Skip("LLM_ENABLED=1 not set; skipping enrich e2e (would cost real tokens)")
	}

	dbPath := filepath.Join(t.TempDir(), "selfg.db")

	mustRun(t, "./bin/kg", "--db", dbPath, "init")
	mustRun(t, "./bin/kg-extractor", "extract",
		"--plugin", "tree-sitter", "--language", "go",
		"--input", "./internal/graph", "--domain", "selfg",
		"--db", dbPath, "--kg-binary", "./bin/kg")

	mustRun(t, "claude", "code", "run", "/kg-enrich", "--domain", "selfg",
		"--env", "KG_DB="+dbPath)

	files := jsonField(t, dbPath, "node", "list", "--domain", "selfg", "--layer", "file", "--source", "kg-summary:0.1.0", "--limit", "0")
	require.NotEmpty(t, files, "no files were enriched")

	arch := jsonField(t, dbPath, "node", "list", "--domain", "selfg-arch", "--source", "kg-arch:0.1.0", "--limit", "0")
	require.GreaterOrEqual(t, len(arch), 3, "expected at least 3 arch layers")

	tours := jsonField(t, dbPath, "node", "list", "--domain", "selfg-tours", "--source", "kg-tours:0.1.0", "--limit", "0")
	require.GreaterOrEqual(t, len(tours), 5, "expected at least 5 tour steps")
}

func mustRun(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "command failed: %s %v", name, args)
}

func jsonField(t *testing.T, dbPath string, args ...string) []any {
	t.Helper()
	cmd := exec.Command("./bin/kg", append([]string{"--db", dbPath}, args...)...)
	out, err := cmd.Output()
	require.NoError(t, err)
	type resp struct {
		OK   bool        `json:"ok"`
		Data interface{} `json:"data"`
	}
	var r resp
	require.NoError(t, jsonUnmarshal(out, &r))
	arr, _ := r.Data.([]any)
	return arr
}
```

Note: `claude code run /kg-enrich` is a placeholder for whatever the actual Claude Code CLI invocation is for headless skill execution. If no such mode exists at the time, this test should print a manual-instruction block to stdout and skip, so it can be promoted to fully-automated later. Verify the right invocation form before merging.

If `jsonUnmarshal` helper doesn't exist in `e2e/testutil.go`, add it there:

```go
import "encoding/json"

func jsonUnmarshal(b []byte, v any) error {
	return json.Unmarshal(b, v)
}
```

(Or just inline `json.Unmarshal` directly in the test.)

- [ ] **Step 2: Add `e2e-enrich` target to `Makefile`**

```makefile
e2e-enrich: build build-extractor build-plugin-treesitter
	LLM_ENABLED=1 go test -tags=e2e_enrich -v -timeout=15m ./e2e/...
```

Update `.PHONY` to include `e2e-enrich`.

- [ ] **Step 3: Verify the build tag works (test should skip without LLM_ENABLED)**

```bash
go test -tags=e2e_enrich -v ./e2e/... -run TestEnrichSelf
```

Expected: SKIP (LLM_ENABLED=1 not set). Exit 0.

```bash
go test -v ./e2e/... -run TestEnrichSelf
```

Expected: no such test (build tag excludes it). Exit 0.

- [ ] **Step 4: Commit (do NOT run the full e2e with real tokens yet — that's a separate decision)**

```bash
git add e2e/enrich_self_test.go Makefile
git commit -m "test(e2e): enrich-self test gated behind e2e_enrich build tag + LLM_ENABLED"
```

---

### Task 16: README v3 plugin section (user-facing install + usage)

Document how end users install and use the plugin. Covers prerequisites, `/plugin marketplace add`, the four skills, cost expectations.

**Files:**
- Modify: `README.md` (append "v3 enrichment plugin" section)

- [ ] **Step 1: Append the section**

After the v2 extractor section in `README.md`, append:

````markdown
## v3 enrichment plugin (Claude Code)

The `.claude-plugin/` directory at the repo root is a Claude Code plugin that
layers LLM-driven semantic enrichment on top of kg's structural graph. It runs
inside any Claude Code session (CLI, IDE, web). The kg engine (the binaries
above) is a prerequisite.

### Install

\`\`\`sh
# Make sure the kg CLI is on PATH (this plugin shells out to it)
make install                       # or: go install ./cmd/kg

# In Claude Code, add this repo as a plugin marketplace:
/plugin marketplace add github:ggfarmco/kg
/plugin install kg@kg
\`\`\`

The plugin contributes four skills and three subagents.

### Skills

- **`/kg-enrich`** — orchestrates the full pipeline: per-decl summaries
  (5 parallel `file-summarizer` agents) → architectural layer inference
  (`architecture-analyzer`) → onboarding tour (`tour-builder`). One run
  populates three derived sources: `kg-summary:0.1.0`, `kg-arch:0.1.0`,
  `kg-tours:0.1.0`.
- **`/kg-explain <node-id>`** — read-only Q&A about a single node using its
  enriched properties + 1-hop neighbors.
- **`/kg-tour [--domain X]`** — re-runs only `tour-builder` (cheaper than a
  full `/kg-enrich`).
- **`/kg-onboard [--output path]`** — generates `docs/ONBOARDING.md` from
  the enriched graph.

### Quick start (assumes you already have a kg.db with tree-sitter data)

\`\`\`sh
# In Claude Code, with kg.db in your cwd:
/kg-enrich
# ... wait for batches to complete ...
/kg-onboard
# review docs/ONBOARDING.md
\`\`\`

### Cost expectations

`/kg-enrich` makes one Claude call per file-summarizer batch (~25 files per
batch, 5 batches in parallel) plus one call each for architecture-analyzer
and tour-builder. For a 100-file project, expect ~6 inference calls total.
Use `--max-files N` to cap; intermediate files in `.kg-enrich-tmp/` show
exactly what was sent.

### Idempotency

Re-running any skill is safe. `file-summarizer` writes under `scope: additive`
so its properties overwrite cleanly in its own namespace. `architecture-analyzer`
and `tour-builder` use `scope: domain-source` so re-running cleanly replaces
the previous arch / tours.

See `docs/superpowers/specs/2026-05-24-kg-v3-skill-enrichment-design.md` for
the full design.
````

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: README v3 enrichment plugin section (install + skills + costs)"
```

---

### Task 17: Branch close-out

Final integration checks, PR description, merge.

- [ ] **Step 1: Run every check**

```bash
make test
make test-all
make test-scripts
make e2e
GOWORK=off go build ./...
```

Expected: all green. (Do NOT run `make e2e-enrich` here — it costs real tokens. It's a post-merge / on-demand action.)

- [ ] **Step 2: Sanity-check the diff**

```bash
git log --oneline main..feat/kg-v3-enrichment
git diff --stat main..feat/kg-v3-enrichment
```

Expected changes:
- `internal/graph/service_apply.go`, `snapshot/validate.go` — small targeted edits
- `internal/graph/service_apply_test.go`, `snapshot/validate_test.go` — appended tests
- `cmd/kg/export_cmd.go`, `cmd/kg/export_cmd_test.go`, `cmd/kg/root.go`, `internal/graph/service.go` (if `EdgeIDsClaimedBy` / `GetEdge` wrappers were added) — new CLI verb
- `.claude-plugin/**` — all new (manifest, 3 agents, 4 skills, 5 scripts + tests + fixtures)
- `testdata/v3-fixture/**` — new
- `docs/v3-fixture-trace.md`, `README.md`, `CHANGELOG.md` — new + appended
- `e2e/enrich_self_test.go`, `Makefile` — new test + targets

Nothing outside that surface should change. No `go.mod` bumps, no Makefile rewrites beyond appended targets, no migration changes.

- [ ] **Step 3: Open the merge**

Since this repo has no remote configured (per handoff), merge locally with `--no-ff`:

```bash
git switch main
git merge --no-ff feat/kg-v3-enrichment -m "$(cat <<'EOF'
feat: v3 — skill-driven LLM enrichment

Engine:
- Service.Apply in additive scope writes properties on foreign-owned nodes
- Snapshot validator allows bare NodeSpec (no layer/name) in additive scope
- New CLI verb: kg export --domain X --source Y (snapshot round-trip)

Plugin (.claude-plugin/):
- 3 agents: file-summarizer, architecture-analyzer, tour-builder
- 4 skills: /kg-enrich, /kg-explain, /kg-tour, /kg-onboard
- 5 bundled bash scripts with fixture-based tests

Tests:
- 4 new engine tests (additive-foreign-properties + validator relax)
- 2 new CLI tests (export round-trip + empty source)
- 5 bundled script tests (make test-scripts)
- testdata/v3-fixture/ + manual smoke trace
- e2e/enrich_self_test.go gated behind e2e_enrich tag + LLM_ENABLED=1

Spec: docs/superpowers/specs/2026-05-24-kg-v3-skill-enrichment-design.md
EOF
)"

git branch -d feat/kg-v3-enrichment
```

If a remote is later added: `git push origin main` and optionally open a retroactive PR for documentation purposes.

- [ ] **Step 4: Verify the merge**

```bash
git log --oneline -5
make test && make test-all && make test-scripts && make e2e
```

Expected: merge commit visible, all checks green.

---

## Self-Review Notes

After writing this plan, the spec was re-read top-to-bottom. Coverage matrix:

| Spec section | Implementing task(s) |
|---|---|
| Engine tweak — additive scope writes properties on foreign nodes | Task 1 |
| Engine tweak — NodeSpec validation relaxation in additive scope | Task 1 |
| New CLI verb: `kg export --domain --source --format snapshot` | Task 2 |
| Engine README / CHANGELOG docs | Task 3 |
| Plugin packaging: `plugin.json` + `marketplace.json` | Task 4 |
| Bundled scripts — `dump-files.sh` | Task 5 |
| Bundled scripts — `dump-batch-context.sh` | Task 5 |
| Bundled scripts — `apply-snapshot.sh` | Task 5 |
| Bundled scripts — `dump-graph-shape.sh` | Task 6 |
| Bundled scripts — `dump-topology.sh` | Task 6 |
| Script test harness pattern + `make test-scripts` | Task 5 |
| Agent: `file-summarizer` (two-phase, output spec, retry, edge vocab) | Task 7 |
| Agent: `architecture-analyzer` (layer inference, cross-domain contains) | Task 8 |
| Agent: `tour-builder` (ordered steps, cross-domain teaches) | Task 9 |
| Skill: `/kg-enrich` (6 phases, dispatch waves) | Task 10 |
| Skill: `/kg-explain` (read-only Q&A, 1-hop neighborhood) | Task 11 |
| Skill: `/kg-tour` (standalone tour re-trigger) | Task 12 |
| Skill: `/kg-onboard` (`docs/ONBOARDING.md` generation) | Task 13 |
| Smoke fixture + trace | Task 14 |
| E2E enrich-self test (build-tag gated) | Task 15 |
| User-facing README v3 plugin section | Task 16 |
| Branch close-out + merge | Task 17 |
| Error handling per-batch (retry, partial success) | Tasks 7, 10 (agent retry + orchestrator failure list) |
| Idempotency claims (additive overwrite + domain-source replace) | Tasks 7, 8, 9, 10 |
| Source IDs (`kg-summary:0.1.0`, `kg-arch:0.1.0`, `kg-tours:0.1.0`) | Tasks 7-9 (each agent owns its source ID) |
| Domains created (`<orig>-arch`, `<orig>-tours`) | Tasks 8, 9 |
| Semantic edge vocabulary table | Task 7 |
| Concurrency (5 parallel file-summarizer dispatches) | Task 10 (Phase 3) |
| Cross-phase partial-state tolerance | Task 10 (continue on arch failure) |

**Deliberate v3 simplifications (called out where they matter):**

- **No `merge-batch-graphs` script.** UA needs one because its storage is a single JSON file. kg's namespaced + multi-claim model makes per-batch `kg apply` safe and faster (Task 10 Phase 3). Skipped entirely.
- **No fingerprint-based incremental update.** `kg apply` is already idempotent, so re-running on unchanged code is a no-op on the engine side; the LLM cost we manage via `--max-files`. v3.1 may revisit if hot codebases drive demand.
- **No heuristic fallback when LLM unavailable.** UA has heuristic layer-detector and tour-generator backstops. v3 takes the cleaner "fail visibly" path: agents exit non-zero with a structured error envelope (Tasks 7-9), the orchestrator reports them, the user knows exactly what to retry.
- **No Go binary `cmd/kg-enricher`.** Explicitly punted to v3.1+ if headless/CI enrichment demand emerges. The plugin path is enough for the workflows v3 targets.
- **`/kg-explain` does NOT dispatch a subagent.** The orchestrator skill itself constructs the answer in-prompt because the context is small (one node + 1-hop). Faster, cheaper, simpler to debug (Task 11).
- **`/kg-onboard` does NOT call any agent.** Pure read + format. Markdown generation is deterministic; no LLM needed (Task 13).
- **`apply-snapshot.sh` is sugar.** Saves remembering flag order in three agent prompts. Could be inlined; the wrapper improves agent prompt readability (Tasks 5, 7-9).
- **`tour-builder` degrades to topology-only if arch fails.** Catalogued in Task 10 (don't abort on arch failure) and Task 9 (handle missing `arch_domain`).
- **Script tests use bash function shadowing.** No subprocess sandboxing, no mock framework. The `export -f kg` trick is the simplest reliable way to intercept `kg` calls from inside the script under test (Tasks 5-6).

**Risks acknowledged:**

- **CLI shape assumption: `kg edge list --domain X --type Y`.** Used in `dump-graph-shape.sh` and `dump-topology.sh` (Task 6). If this verb doesn't exist or has a different shape, the scripts won't work in production but the *tests* will still pass (the mock kg function returns canned JSON). Verify against `./bin/kg edge --help` before the smoke test in Task 14. If absent, either add it as a small CLI task before Task 6 or fall back to per-node `edge list-from` loops in the scripts.
- **`runOKWithStdin` test helper.** Task 2 assumes it exists in `apply_cmd_test.go` (since `apply` also reads stdin). If it doesn't, add it inline; do not skip the round-trip assertion silently. The stdin-injection pattern needs `os.Stdin = ...` + restore-in-cleanup.
- **`Service.EdgeIDsClaimedBy` / `Service.GetEdge` may already be on Service.** Task 2 prescribes a grep before adding wrappers. If they exist with a different signature, use the existing one; do not add a duplicate.
- **Topology test's hotspot sort with tied `fan_in`.** jq's sort_by is stable per input order, which may or may not match expectations. If the test fails due to ordering, hand-verify correctness then update the expected fixture — this is a property of jq, not a bug to fix.
- **Claude Code CLI invocation for e2e (`claude code run /kg-enrich`).** Task 15 uses a placeholder form. If headless skill execution isn't available, the test should degrade to a manual-instruction skip rather than fail. Verify the actual invocation before treating the e2e as authoritative.
- **Markdown skills/agents are LLM prompts, not code.** They cannot be unit-tested in any meaningful way. The smoke trace in Task 14 is the only line of defense against silent prompt regressions. Re-run it after any agent prompt edit.
- **`docs/v3-fixture-trace.md` is observational, not assertional.** Don't pre-fill it with predictions. If observed behavior diverges next month, that's signal — investigate before updating the trace.
- **Plugin install path** (`/plugin marketplace add github:ggfarmco/kg`) assumes Claude Code can fetch the `.claude-plugin/` from the repo root. If marketplace-fetching the bundle requires a remote, the merge in Task 17 leaves no remote configured — set one up before encouraging users to install. The plan does not add a remote.
- **Engine wrapper additions in Task 2.** If `Service.EdgeIDsClaimedBy` / `Service.GetEdge` get added as Service-level wrappers, they become part of the v3 public Service surface. Document in CHANGELOG (Task 3 can be extended).
- **CLI: `--source` flag default on `node list`.** `dump-files.sh` filters by `--source "$source_id"`. If a future structural source coexists with `tree-sitter:0.2.0` and the user passes a different source to `/kg-enrich`, the scripts will silently work on that subset. Document in the skill: "Enriches the subset of nodes owned by the chosen --source."
