# kg v1 — Extractor System Design Spec

**Date:** 2026-05-23
**Status:** Approved for implementation planning
**Builds on:** `docs/superpowers/specs/2026-05-23-kg-mvp-design.md` (v0)

## Background

v0 shipped a generic graph engine (`kg` binary) with hexagonal architecture, JSON-envelope CLI, and a `changes` log foundation — but no extractors. The roadmap calls for v1 to be "first extractor (no LLM), proves the extractor interface."

This spec defines that extractor system. The headline architectural choice: **extractors are NOT part of `kg` itself.** They live as separate binaries alongside it, communicating via a documented JSONL operation stream. This keeps `kg`'s core domain-agnostic (per v0 spec) while making the extractor surface genuinely pluggable — including for non-Go runtimes (WASM modules, bash scripts).

The first concrete extractor is Go-via-tree-sitter, modeled on the deterministic-extraction patterns in [Understand-Anything](../../../../../Understand-Anything/) (an existing TypeScript Claude Code plugin that does the same job for code analysis). Understand-Anything's `go-extractor.ts` is the structural reference; we adapt its tree-sitter walking patterns to a Go CGO plugin, and project its file/function model onto kg's stricter ordered-layer schema.

### Why "extractor is not part of kg"

The user-stated goal: "extractor может быть не один, под определенные задачи, не всегда это будет парсинг GO или других языков, так что это даже по идее не часть kg прямая, а что-то рядом, отдельным бинарем". Some future extractor might be a bash script wrapping a CLI tool; some might be WASM modules. Building this pluggability into kg core would couple it to a plugin runtime. Instead, kg gains exactly one capability — `kg batch` for atomic bulk-ingest of a JSONL operation stream — and everything else is a separate process tree that ultimately pipes into that stream.

## Goals

1. **Decouple structure extraction from the graph engine.** kg stays a generic store/CLI; extractors are independent processes that feed it via a documented contract.
2. **Plugin model with three runtimes:** native binaries, external commands (bash, python, etc.), WASM modules. v1 ships native + command; WASM is a manifest-level decision that v1.x can implement without breaking plugins.
3. **One unified tree-sitter plugin (`plugins/tree-sitter/`)** that dispatches by `--language`. v1 ships with the Go grammar wired in; future languages (Python, Rust, etc.) are added by registering their grammar + extraction rules in the same plugin. Proves the contract handles a realistic structural extractor with non-trivial output (~1000 ops on a moderate codebase).
4. **One demo plugin (bash + jq, ~10 lines):** proving the contract works without compiled code.
5. **Atomic bulk-ingest into kg:** a new `kg batch` subcommand reads JSONL ops from stdin and applies them in one transaction (with chunking and continue-on-error opt-ins).
6. **End-to-end pipeline tested by extracting kg's own `internal/graph` package** and asserting the resulting graph shape.

## Non-Goals (deferred to v2+)

- WASM runtime implementation (manifest accepts `"runtime": "wasm"` but the v1 dispatcher errors with `WASM_NOT_SUPPORTED`).
- Incremental extraction (git-fingerprint, file-hash diff). v1 is full re-extract with `if_not_exists`.
- Cross-package call resolution (requires `go/packages` + type checker — outside tree-sitter's reach).
- `implements` edges (interface satisfaction — requires type checker).
- Snapshot replacement semantics (extractor declares full domain state, kg diffs and removes stale nodes).
- LLM-driven semantic enrichment (a future extractor reads the existing graph and adds summaries/layers).
- `domain.update` / `domain.delete` ops in the WAL vocabulary (extractors rarely modify domain metadata; add if a real need emerges).
- `kg-extractor validate <plugin>` tooling; plugin packaging / install commands. v1 users place files in the plugins directory by hand.
- Additional tree-sitter language grammars beyond Go (Python, Rust, TypeScript, etc.) — the plugin architecture supports them, but v1 only registers the Go grammar. Adding a language post-v1 is a small PR (one new `languages/<lang>/` subdir + grammar import).
- Markdown-with-wikilinks plugin. The bash-demo is sufficient proof that the contract handles non-Go extractors.

## Architecture

Three new moving parts wired with subprocess pipes:

```
┌─────────────────┐       ┌─────────────────┐       ┌─────────────┐
│   Plugin        │       │  kg-extractor   │       │     kg      │
│  (executable    │ JSONL │  (dispatcher    │ JSONL │   (engine,  │
│   in plugins    ├──────►│   binary,       ├──────►│   uses new  │
│   dir, runs as  │  ops  │   validates,    │  ops  │   `batch`   │
│   subprocess)   │       │   pipes through)│       │   command)  │
└─────────────────┘       └─────────────────┘       └─────────────┘
       stdout                   stdout                  stdin
       (JSONL)                  (JSONL)                 (JSONL)
```

- **Plugin** — any executable conforming to the plugin contract (Section "Plugin Contract"). Reads JSON config on stdin, emits JSONL graph operations on stdout, logs to stderr, exits 0 on success.
- **kg-extractor** — a new dispatcher binary (`cmd/kg-extractor/`, pure Go, no CGO). Discovers plugins, invokes the requested one with the right args, validates the JSONL stream syntactically, forwards to `kg batch` (when `--db` is set) or passes through to its own stdout.
- **`kg batch`** — a new subcommand on the existing `kg` binary. Consumes JSONL ops from stdin, applies them through `graph.Service` in a single transaction (or chunked, on opt-in). Returns a summary envelope.

Two typical invocations:

```sh
# Internal pipe (kg-extractor handles forwarding):
kg-extractor extract --plugin tree-sitter --language go --input ./src --domain my-app --db ./kg.db

# External pipe (debugging or custom processing in between):
kg-extractor extract --plugin tree-sitter --language go --input ./src --domain my-app \
    | jq -c 'select(.op != "edge.add")' \
    | kg --db ./kg.db batch
```

The external-pipe form is what makes this composable. Any tool that can produce or transform JSONL ops can sit in the pipeline.

### Module / CGO isolation

Plugins are **physically separate Go modules** under `plugins/<name>/`, each with its own `go.mod`. The kg main module never imports plugin code; plugins import kg's public contract package (`batch/`) for op types and the JSONL codec.

This achieves the user-stated goal "extractor не часть kg прямая" at the strongest level: a plugin's dependencies (including CGO ones like tree-sitter) don't even appear in kg's `go.sum`. v0 users who don't need extractors are completely untouched by the CGO toolchain requirement.

For local development, plugins use a `replace` directive pointing at the kg checkout:

```go
// plugins/tree-sitter/go.mod
module github.com/ggfarmco/kg/plugins/tree-sitter

go 1.26

replace github.com/ggfarmco/kg => ../..

require (
    github.com/ggfarmco/kg v0.0.0-00010101000000-000000000000  // local via replace
    github.com/smacker/go-tree-sitter v0.0.0-...               // CGO + Go grammar in v1
)
```

For external plugin authors, the `replace` is omitted and the plugin pins to a published kg version.

Build targets are split:

- `make build` — produces `bin/kg` (pure Go, unchanged from v0). Only touches the root module.
- `make build-extractor` — additionally produces `bin/kg-extractor` (pure Go, still in the root module).
- `make build-plugin-treesitter` — produces `bin/kg-extractor-tree-sitter` from the `plugins/tree-sitter/` module (CGO required). In v1 the binary embeds only the Go grammar; future grammars are added as Go imports in the plugin source.

## Plugin Contract

### Discovery

Plugins live in `~/.config/kg-extractor/plugins/<name>/`. Each plugin directory contains a `manifest.json` plus executable / module / script files referenced by the manifest. The discovery path can be overridden via `KG_EXTRACTOR_PLUGINS_PATH` (colon-separated, like `$PATH`) — primarily for tests and dev environments.

`kg-extractor list` walks every entry in the discovery path, parses each `manifest.json`, and prints a JSON envelope listing valid plugins (with their `name`, `version`, `runtime`, `description`). Invalid manifests are reported in `errors[]` but don't abort discovery.

### Manifest format

```json
{
  "name": "tree-sitter",
  "version": "0.1.0",
  "description": "Extract code structure via tree-sitter (multi-language; v1 ships Go)",
  "runtime": "native",
  "executable": "kg-extractor-tree-sitter",
  "supported_layers": ["package", "file", "decl"],
  "supported_languages": ["go"]
}
```

Fields:
- `name` (required) — slug matching `^[a-z0-9-]+$`. Must equal the directory name.
- `version` (required) — semver string. Informational; v1 doesn't enforce compatibility.
- `description` (required) — one-line human-readable.
- `runtime` (required) — one of `"native"`, `"command"`, `"wasm"`. v1 supports `native` and `command`; `wasm` is reserved.
- For `runtime: native` — `executable: <path>` (relative to plugin dir or absolute).
- For `runtime: command` — `command: [<arg0>, <arg1>, ...]` (e.g., `["bash", "extract.sh"]`).
- For `runtime: wasm` (v1 reserved) — `module: <path-to-.wasm>`.
- `supported_layers` (optional) — informational hint for `kg-extractor list`, not enforced.

### Invocation

kg-extractor invokes the plugin process and writes the following JSON document to its stdin (then closes stdin):

```json
{
  "input": "/abs/path/to/extract",
  "domain": "my-app",
  "protocol_version": 1,
  "config": { "include_external_imports": false, "skip_test_files": true }
}
```

Fields:
- `input` — opaque to kg-extractor; plugin-specific meaning (for tree-sitter with `--language go`, an absolute path to a Go module/directory).
- `domain` — kg domain id (slug). Plugin emits all nodes into this domain.
- `protocol_version` — set to `1` in v1; plugins MUST refuse on unknown values to allow forward-compat evolution.
- `config` — plugin-specific JSON. Forwarded from `kg-extractor extract --config-file foo.json` or `--config-json '{...}'`. May be absent (treated as `{}`).

Plugin output contract:
- **stdout:** JSONL graph operations, one valid JSON object per line (see "Op Vocabulary" below). Plugin MAY emit an optional `{"op":"meta", ...}` as the first line.
- **stderr:** human-readable logs. kg-extractor forwards to its own stderr (or suppresses with `--quiet`).
- **exit code:** 0 = success (kg-extractor proceeds with the collected stream). Non-zero = fatal error; kg-extractor aborts and forwards NOTHING to kg.

### What kg-extractor does with the stream

1. Reads the plugin's stdout line-by-line.
2. Validates each line: must be valid JSON, must have a known `op`, must have required `args` fields.
3. Buffers everything in memory in v1 (matches `kg batch`'s buffered default). Streaming-mode for very large extractions is a documented v1.x optimization; the protocol is already streaming-friendly (line-oriented JSONL).
4. If any line fails validation → abort: print error envelope, exit non-zero, send nothing to kg.
5. If plugin exits non-zero → abort similarly.
6. Otherwise, depending on `--db`:
   - With `--db`: spawn `kg --db <path> batch`, pipe the validated stream to its stdin, propagate its exit code and stdout envelope.
   - Without `--db`: write the validated stream to kg-extractor's own stdout (for piping to an external `kg batch` or another tool).

## Op Vocabulary

JSONL stream consumed by both kg-extractor (validation) and `kg batch` (execution). Operations mirror existing `kg` CLI commands one-to-one — args use `snake_case` versions of CLI flag names.

```json
{"op":"meta",        "args":{"plugin":"tree-sitter","language":"go","version":"0.1.0","total_ops":1543}}
{"op":"domain.add",  "args":{"id":"my-app","layers":["package","file","decl"],"description":"...","if_not_exists":true}}
{"op":"node.add",    "args":{"domain":"my-app","layer":"package","name":"fmt","id":"fmt","parent":"","summary":"...","if_not_exists":true}}
{"op":"node.update", "args":{"id":"my-app:fmt","name":"...","summary":"..."}}
{"op":"node.delete", "args":{"id":"my-app:fmt"}}
{"op":"edge.add",    "args":{"source":"my-app:fmt/print-go","target":"my-app:io","type":"imports","if_not_exists":true}}
{"op":"edge.delete", "args":{"id":42}}
```

Rules:
- `op` MUST be one of: `meta`, `domain.add`, `node.add`, `node.update`, `node.delete`, `edge.add`, `edge.delete`.
- `args` fields exactly match the corresponding CLI flag names with `snake_case` (`if_not_exists`, not `if-not-exists`).
- `meta` is informational. Both kg-extractor and `kg batch` log it to stderr (for human visibility) and consume `total_ops` if present to drive the `--progress` denominator. It is NOT applied as a graph mutation — the counter passes through unchanged.
- All other ops map to a Service method via a table in `kg batch`'s router.
- `if_not_exists: true` matches the existing CLI semantic — already-exists errors are silently swallowed, the op counts as `skipped`, batch continues.
- Plugins emit ops with `if_not_exists: true` by default to support re-extraction without errors.
- A plugin MAY emit ops without `if_not_exists` for strict-error semantics (mainly for tests).
- The `meta` op MAY appear only as the first line of the stream.

Ordering constraints (plugin's responsibility):
- `domain.add` before any node/edge ops referencing that domain.
- A node's parent MUST appear before the node itself (kg validates parent existence).
- An edge's endpoints MUST appear before the edge.

Excluded from v1:
- `domain.update`, `domain.delete` — kg v0 doesn't expose `domain update`; extractors don't currently need to delete entire domains.
- Transaction markers (`batch.begin` / `batch.commit`) — the entire stdin is one transaction by default (Section "kg batch").
- Snapshot-mode operations — incremental re-extract in v1 is full re-extract + `if_not_exists`.

## `kg batch` Subcommand

```
kg --db ./kg.db batch [--chunk-size N] [--continue-on-error] [--dry-run] [--progress] < ops.jsonl
```

### Atomicity (default)

The entire stdin is consumed and applied in one `Store.InTx`. Any op error (validation failure, parent-not-found, etc.) triggers full rollback and a non-zero exit with the JSON-envelope error of the failing op.

Predictability is the value: a plugin emits N ops and gets either all-or-nothing.

### `--chunk-size N`

When set, `kg batch` commits a transaction every N successfully-applied ops. A failure inside a chunk rolls back only that chunk; earlier chunks remain committed. Use for very large streams (100k+ ops) where holding a single transaction strains SQLite's WAL.

Default: 0 (whole-stream-as-one-tx).

### `--continue-on-error`

When set, `kg batch` keeps applying ops even after individual failures. Returns the final envelope:

```json
{
  "ok": false,
  "data": {"applied": 1500, "skipped": 5, "failed": 38},
  "error": {"code": "BATCH_PARTIAL", "message": "38 ops failed; see failures[]"},
  "failures": [
    {"line": 42, "op": "node.add", "code": "PARENT_LAYER_MISMATCH", "message": "..."},
    ...
  ]
}
```

Exit code = the highest sentinel exit code among `failures` (so a single validation error → exit 1, etc.). If `failed == 0`, returns `ok: true` with the same `data` minus `failures`.

### `--dry-run`

Runs the full stream inside an `InTx` and returns a sentinel error to force rollback (same pattern as v0 `--dry-run` flags). Emits:

```json
{"ok": true, "data": {"dry_run": true, "would_apply": 1542, "would_skip": 5}}
```

Useful for LLM agents validating a stream before committing.

### `--progress`

Emits `applied N/total` lines to stderr roughly every 100ms (uses `meta.total_ops` if present, otherwise just counts). Default off — keeps stderr clean for piping. Stdout always contains only the final envelope (so JSON consumers stay clean).

### Stream parse errors (before transaction)

Invalid JSON, unknown `op`, missing required `args` field — `kg batch` errors out BEFORE touching the database:

```json
{"ok": false, "error": {"code": "INVALID_OP", "message": "line 42: unknown op 'foo.bar'"}}
```

Exit 1 (validation class).

### Final envelope (success)

```json
{
  "ok": true,
  "data": {"applied": 1542, "skipped": 5, "took_ms": 234}
}
```

## tree-sitter Plugin

Single native CGO binary `kg-extractor-tree-sitter`. Built with `CGO_ENABLED=1`. Links against `github.com/smacker/go-tree-sitter` (the engine) plus one grammar package per supported language (in v1: `github.com/smacker/go-tree-sitter/golang`).

### Language dispatch

The plugin internally dispatches by a `--language` flag (or `config.language` from the JSON config on stdin). Each language is implemented as a sub-package under `languages/<lang>/` that registers itself with a central registry exposing:

- An identifier (`"go"`, `"python"`, ...).
- The tree-sitter `*sitter.Language` for that grammar.
- An `Extractor` interface with hooks: `WalkFile(*sitter.Node) []Decl`, `ResolveCalls(pkg *Package) []Edge`, file-extension filter, etc.

The shared walker code (directory traversal, slug sanitization, op emission via `batch/`) lives in `plugins/tree-sitter/` root files and is language-agnostic. Only the per-language extraction rules (which tree-sitter node types map to which kg-side concept) live in `languages/<lang>/`.

**Invocation in v1:**

```sh
kg-extractor extract --plugin tree-sitter --input ./src --domain my-app \
    --config-json '{"language":"go"}'
```

Or via dedicated flag (forwarded into `config`):

```sh
kg-extractor extract --plugin tree-sitter --language go --input ./src --domain my-app
```

The plugin errors with `LANGUAGE_NOT_SUPPORTED` if `config.language` isn't in its registry. v1 has only `"go"` registered.

### Languages registered in v1

| Language | Status | Notes |
|----------|--------|-------|
| `go`     | ✓ shipped | Full structural extraction, intra-package call graph. |
| `python` | deferred  | Plugin architecture supports it; awaits a real need. |
| `rust`   | deferred  | Same. |
| ...      | deferred  | Adding one is `languages/<lang>/{lang.go, decl.go, ...}` + grammar import. |

### Layers

Three layers per Understand-Anything's adapted hierarchy:

```
package → file → decl
```

- `package` — one node per unique Go package found under `input`. ID: `<domain>:<pkg-slug>`.
- `file` — one node per `.go` file. ID: `<domain>:<pkg-slug>/<basename-slug>`. Parent: package.
- `decl` — one node per top-level declaration (function, method, struct, interface, top-level `var`/`const`). ID: `<domain>:<pkg-slug>/<basename-slug>::<name-slug>`. Parent: file.

Sub-function content (locals, expressions) is NOT extracted. That's the `body` layer from Understand-Anything's richer model — overkill for v1, explodes graph size.

### Slug sanitization

kg requires `^[a-z0-9-]+$` per slug component. Plugin transforms identifiers:
1. Lowercase.
2. Replace `/`, `_`, `.`, and any non-`[a-z0-9-]` with `-`.
3. Collapse repeated `-` to single `-`.
4. Trim leading/trailing `-`.

Examples:
- `internal/graph` → `internal-graph`
- `Node.go` → `node-go`
- `tree-sitter` → `tree-sitter` (unchanged)
- `__init__` → `init`

If the result is empty after trimming, the plugin emits an error line to stderr and skips that entity (no op emitted).

### Properties (opaque blob per spec, populated for downstream consumers)

Plugin populates `properties` (already a TEXT column on every entity in v0 schema). kg's CLI doesn't surface these in v1, but future consumers can read them.

**Package node properties:**
```json
{"import_path": "github.com/ggfarmco/kg/internal/graph", "files_count": 8, "go_files_total_lines": 1247}
```

**File node properties:**
```json
{"path": "internal/graph/node.go", "lines": 62, "imports": ["fmt", "errors"]}
```

**Decl node properties:**
```json
{
  "kind": "function" | "method" | "struct" | "interface" | "var" | "const",
  "name": "ParseSlug",
  "exported": true,
  "line_start": 12,
  "line_end": 18,
  "params": ["s string"],
  "returns": "(SlugID, error)",
  "receiver": null
}
```

`receiver` is non-null only for `kind: "method"`; value is the receiver's base type name (without `*`).

### Edges

Two edge types in v1:

1. **`imports`** — package → package.
   ```json
   {"op":"edge.add","args":{"source":"myapp:internal-graph","target":"myapp:internal-store","type":"imports","if_not_exists":true}}
   ```
   - Emitted at package level (not file level — dedup via `UNIQUE(source, target, type)` would collapse them anyway).
   - Target may be an intra-domain package or an external one.
   - **External packages** (stdlib, third-party): controlled by `config.include_external_imports` (default `false`). When `true`, plugin creates placeholder package nodes for externals (id prefix `<domain>:ext-`, e.g., `myapp:ext-fmt`). When `false`, edges to externals are skipped.

2. **`calls`** — decl → decl (intra-domain only).
   - Tree-sitter call-graph traversal gives the callee as raw text (e.g., `fmt.Println`, `s.Name()`, `foo()`).
   - Plugin resolves only **intra-package, unqualified calls** by name lookup in a symbol table built per-package during a first pass.
   - Cross-package calls and method-via-receiver calls are dropped in v1 (no type info → false-positive rate too high). v2 may add this via `go/packages`.

### Emission order

1. `domain.add` (with `if_not_exists: true`) — domain with layers `[package, file, decl]`.
2. For each package (depth-first by path):
   1. `node.add` for the package.
   2. For each `.go` file in the package:
      1. `node.add` for the file (parent: package).
      2. `node.add` for each top-level decl in the file (parent: file).
3. After all node ops:
   1. `edge.add` for every `imports` edge.
   2. `edge.add` for every `calls` edge.

Edges go last so endpoints definitely exist before any edge tries to reference them.

### Excluded from v1

- Build-tag-conditional code (e.g., `//go:build cgo` files) — extracted normally, with all decls, regardless of build constraints.
- Generic type parameters — params recorded as text; generic info doesn't surface in `properties`.
- Anonymous functions / closures as separate decls.
- `implements` (interface satisfaction) — needs type checker.
- `references` (non-call usage) — without type info, too noisy.
- Test files when `config.skip_test_files: true` (default `true`). Plugin skips `*_test.go`.
- `vendor/`, `.git/`, `node_modules/` directories — always skipped.

### Incrementality (deferred)

v1 = full re-extract. Re-running the plugin on the same code emits the same ops with `if_not_exists: true`; existing nodes/edges are skipped via kg's batch handling. Stale nodes (from deleted files) remain in the graph. v2 adds git-fingerprint + snapshot semantics.

## Demo Plugin (`bash-demo`)

Lives in `examples/kg-extractor-plugins/bash-demo/`. Used by e2e tests and as a copy-paste template for users writing their own non-Go plugins.

`manifest.json`:
```json
{
  "name": "bash-demo",
  "version": "0.1.0",
  "description": "Trivial bash plugin emitting a fixed mini-graph for contract testing",
  "runtime": "command",
  "command": ["bash", "extract.sh"]
}
```

`extract.sh` (~10 lines):
```bash
#!/usr/bin/env bash
set -euo pipefail
config=$(cat)
domain=$(echo "$config" | jq -r '.domain')
cat <<EOF
{"op":"domain.add","args":{"id":"$domain","layers":["root","item"],"if_not_exists":true}}
{"op":"node.add","args":{"domain":"$domain","layer":"root","name":"Demo","if_not_exists":true}}
{"op":"node.add","args":{"domain":"$domain","layer":"item","name":"First","parent":"$domain:demo","if_not_exists":true}}
{"op":"node.add","args":{"domain":"$domain","layer":"item","name":"Second","parent":"$domain:demo","if_not_exists":true}}
{"op":"edge.add","args":{"source":"$domain:demo-first","target":"$domain:demo-second","type":"references","if_not_exists":true}}
EOF
```

This proves the contract works without compiled code or CGO. It's also a regression test target: changes to the plugin contract that break bash-demo break a known-good baseline.

## Project Layout

Final tree after v1 lands (changes from v0 marked NEW). The repo becomes a **multi-module workspace**: the kg root module plus one module per plugin under `plugins/`.

```
ggfarmco/kg/
├── batch/                                 # NEW public package (the contract)
│   ├── op.go                              # Op types shared by kg batch + kg-extractor + plugins
│   ├── op_test.go
│   ├── codec.go                           # JSONL encoder/decoder
│   └── codec_test.go
├── cmd/
│   ├── kg/                                # existing, +1 file
│   │   ├── ... (existing v0 files)
│   │   ├── batch_cmd.go                   # NEW: kg batch subcommand (uses batch/)
│   │   └── batch_cmd_test.go              # NEW
│   └── kg-extractor/                      # NEW (pure Go dispatcher; uses batch/)
│       ├── main.go
│       ├── root.go
│       ├── list_cmd.go
│       ├── info_cmd.go
│       ├── extract_cmd.go
│       ├── manifest.go
│       ├── discovery.go
│       ├── invoke.go
│       ├── validator.go
│       ├── pipe.go
│       └── *_test.go
├── plugins/                               # NEW (separate-module plugins)
│   └── tree-sitter/                       # NEW separate Go module (unified, dispatches by --language)
│       ├── go.mod                         # module github.com/ggfarmco/kg/plugins/tree-sitter
│       ├── go.sum                         # CGO deps live here, not in root go.sum
│       ├── main.go                        # imports github.com/ggfarmco/kg/batch
│       ├── root.go                        # cobra root + --language flag
│       ├── walker.go                      # language-agnostic directory walk + dispatch
│       ├── slug.go                        # language-agnostic slug sanitization
│       ├── emit.go                        # language-agnostic op emission via batch/
│       ├── registry.go                    # language registry + Extractor interface
│       ├── languages/
│       │   └── golang/                    # v1: only language registered
│       │       ├── lang.go                # registers "go" with the grammar + extractor
│       │       ├── decl.go                # function/method/struct/interface extraction
│       │       ├── imports.go             # import_declaration handling
│       │       ├── calls.go               # intra-package call graph
│       │       ├── exported.go            # capitalization rule helper
│       │       ├── testdata/golden/
│       │       │   ├── 01-single-file/
│       │       │   ├── 02-multi-package/
│       │       │   └── 03-with-methods/
│       │       └── *_test.go
│       └── *_test.go
├── internal/
│   ├── graph/                             # existing
│   ├── graph/testutil/                    # existing
│   └── store/                             # existing
├── examples/
│   └── kg-extractor-plugins/
│       └── bash-demo/                     # not a Go module; raw files
│           ├── manifest.json
│           ├── extract.sh
│           └── README.md
├── e2e/                                   # NEW (build-tag gated, in root module)
│   ├── extract_self_test.go
│   └── testutil.go
├── docs/superpowers/
│   ├── specs/
│   │   ├── 2026-05-23-kg-mvp-design.md             # existing v0
│   │   └── 2026-05-23-kg-v1-extractor-design.md    # this file
│   └── plans/
│       ├── 2026-05-23-kg-mvp-implementation.md     # existing v0
│       └── 2026-05-23-kg-v1-implementation.md      # NEW (to be written)
├── migrations/                            # existing
├── go.work                                # NEW (optional but recommended)
├── Makefile                               # +3 targets (build-extractor, build-plugin-treesitter, e2e)
├── go.mod                                 # NO new deps from plugins (CGO stays isolated)
└── README.md                              # +section on extractors
```

### `batch/` — public contract package

Defines `Op` types and a JSONL codec. Lives at the repo root (not under `internal/`) because plugins in separate modules need to import it. Used by:
- `cmd/kg/batch_cmd.go` (the consumer of the stream).
- `cmd/kg-extractor/validator.go` (the gatekeeper that validates before forwarding).
- `plugins/tree-sitter/` (the producer; uses it to emit well-typed ops with `batch.Encoder`).

Without this shared package, every producer and consumer of the contract would redefine the op shape, and drift on field names and edge cases would be inevitable.

### Multi-module setup

Each plugin under `plugins/` is its own Go module. Local development uses a `replace` directive so plugins resolve `github.com/ggfarmco/kg` to the local checkout:

```go
// plugins/tree-sitter/go.mod
module github.com/ggfarmco/kg/plugins/tree-sitter

go 1.26

replace github.com/ggfarmco/kg => ../..
```

To avoid maintaining `replace` directives manually across many plugins, the root ships a `go.work` file:

```go
// go.work
go 1.26

use (
    .
    ./plugins/tree-sitter
)
```

`go.work` makes `go build`, `go test`, and `gopls` automatically resolve the local kg module from all plugin modules during dev. CI exports `GOWORK=off` to test plugins as if they were external consumers (catching missing `replace` directives in plugin `go.mod`s before publishing).

### Makefile additions

```makefile
.PHONY: build-extractor build-plugin-treesitter e2e test-all

build-extractor: build
	go build -o ./bin/kg-extractor ./cmd/kg-extractor

build-plugin-treesitter:
	CGO_ENABLED=1 go -C ./plugins/tree-sitter build -o ../../bin/kg-extractor-tree-sitter .

test-all: test
	go -C ./plugins/tree-sitter test ./...

e2e: build build-extractor build-plugin-treesitter
	go test -tags=e2e -v ./e2e/...
```

`make test` and `make lint` cover only the root module (kg + kg-extractor). `make test-all` adds the plugin module's tests. `make e2e` runs the full pipeline end-to-end.

## Testing Strategy

Three tiers, each gated by build effort.

### Tier 1 — unit (fast, no I/O)

- **`batch/`**: Op type round-trips, JSONL codec edge cases (empty lines, trailing whitespace, embedded newlines in strings).
- **`cmd/kg/batch_cmd.go`**: Op router (every `op` maps to the right Service call), counters (applied/skipped/failed), error envelope shape. Uses the existing `testutil.FakeStore`.
- **`cmd/kg-extractor/`**: Manifest parser (valid/invalid manifests, missing fields, unknown runtime), discovery (tmp plugins dir with several manifests), JSONL validator (re-uses `batch/` codec).
- **`plugins/tree-sitter/`**: Slug sanitization edge cases; param/return/receiver helpers on synthetic tree-sitter ASTs (where feasible — most decl extraction is integration-tested via golden). Runs as part of `make test-all`, not the default `make test`.

### Tier 2 — integration (real SQLite via `:memory:`)

- **kg batch:**
  - Happy path (mixed ops applied successfully).
  - Atomicity (failure on op N rolls back ops 1..N-1).
  - `--continue-on-error` (mixed success/failure; final envelope correct).
  - `--chunk-size N` (failure at op X commits chunks before X-1's chunk, rolls back X's chunk).
  - `--dry-run` (validates without committing).
  - `--if-not-exists` per-op (duplicates skipped without halting batch).
  - Stream parse errors (invalid JSON / unknown op halt before transaction).

- **kg-extractor → kg batch pipeline:**
  - Run `kg-extractor` with bash-demo plugin, `--db` set; verify expected nodes/edges in DB.
  - Plugin exits non-zero → kg-extractor sends nothing to kg (DB empty).
  - Plugin emits malformed JSONL → kg-extractor halts before kg (DB empty).

### Tier 3 — e2e (real CGO plugin, real disk DB)

One large test in `e2e/extract_self_test.go`:

```go
//go:build e2e

func TestExtractSelf(t *testing.T) {
    // 1. Build all three binaries (kg, kg-extractor, kg-extractor-tree-sitter).
    // 2. Lay out a tmp plugins dir with the tree-sitter manifest.
    // 3. Run: kg-extractor extract --plugin tree-sitter --language go \
    //         --input ../internal/graph --domain selfg --db /tmp/selfg.db
    // 4. Assert: kg domain get selfg → layers [package, file, decl].
    // 5. Assert: kg node list --domain selfg --layer package → contains "selfg:internal-graph".
    // 6. Assert: kg node list --domain selfg --layer decl → contains "selfg:internal-graph/node-go::parseslug".
    // 7. Assert: kg edge list-from selfg:internal-graph → "imports" edges to errors/regexp/strings/time.
    // 8. Cleanup tmp DB and plugins dir.
}
```

Not run in `make test` (slow, needs CGO, downloads tree-sitter grammar). Runs in CI as a separate job via `make e2e`.

### Golden tests for the tree-sitter plugin (per language)

Per-language golden tests live under `languages/<lang>/testdata/golden/`. In v1 only Go is registered:

```
plugins/tree-sitter/languages/golang/testdata/golden/
├── 01-single-file/{input/, expected.jsonl}
├── 02-multi-package/{input/, expected.jsonl}
└── 03-with-methods/{input/, expected.jsonl}
```

Each subtest runs the plugin against `input/` with `--language go` and diffs stdout against `expected.jsonl`. Update via `go test -update`. Coverage:
- Single file with mixed decls.
- Multi-package directory tree.
- Struct with attached methods, interface with method elements, imports as both single and grouped form.

Future languages each get their own `languages/<lang>/testdata/golden/` directory with the same shape — the test runner is generic.

### What we don't test

- tree-sitter library internals.
- cobra framework internals.
- Exact `properties` JSON content at the e2e tier (covered by golden tests).

## Tooling additions

Added to the **`plugins/tree-sitter/` module only** (not kg's root module):

- `github.com/smacker/go-tree-sitter` — CGO tree-sitter bindings.
- `github.com/smacker/go-tree-sitter/golang` — Go grammar. (Future languages each add their own grammar import.)
- (Optional, for tests) `github.com/google/go-cmp` — better diff output for golden tests; or stick with `require.Equal` for simpler diffs.

No changes to existing tooling (cobra, modernc.org/sqlite, sqlc, goose, testify, golangci-lint).

## Open Risks

| Risk | Mitigation |
|------|------------|
| go-tree-sitter version drift | Pin to a specific version; golden tests catch grammar changes via exact output comparison. |
| CGO build complexity for users | `make build` is unchanged (no CGO); CGO is opt-in via `make build-plugin-treesitter`. Plugin lives in its own module so its `go.sum` doesn't leak into kg's. README explains. |
| Plugin auth / sandboxing — a malicious plugin runs with user privileges | Out of scope for v1. Plugins are user-installed local binaries; same trust model as kubectl plugins. |
| Tree-sitter call graph false-positives without type resolution | Intra-package-only resolution drastically narrows scope; cross-package calls are dropped. v2 adds proper resolution. |
| Slug collision (two packages with the same slug after sanitization) | Plugin emits a stderr error and skips the second; e2e test on a known clean repo (kg's own `internal/graph`) won't trigger. |
| Stale nodes accumulate over re-extracts | Documented as v1 limitation; v2 adds snapshot semantics. |
| `kg batch` single-tx grows too large in SQLite for huge codebases | `--chunk-size` opt-in for large streams; we document the tradeoff. |
| Plugin protocol_version evolution breaks old plugins | Plugins are required to reject unknown `protocol_version`; v1 = 1. v2 can ship as protocol_version=2 with kg-extractor detecting and adapting. |

## Implementation plan

To be authored next via the `writing-plans` skill. The plan will sequence:

1. `batch/` public package — Op types, JSONL codec, unit tests.
2. `kg batch` subcommand — parser, router, atomic execution, chunking, continue-on-error, dry-run, progress.
3. `cmd/kg-extractor/` skeleton — cobra root, list, info, extract subcommands; manifest parser; discovery.
4. Plugin invocation: subprocess management, stdin config writing, stdout JSONL collection, validator pipeline.
5. Pipeline to `kg batch` (with-`--db` mode) and pass-through (without-`--db`).
6. `examples/kg-extractor-plugins/bash-demo/` and its integration test.
7. Workspace setup: `plugins/tree-sitter/` as a separate Go module with its own `go.mod`/`go.sum`, `go.work` at repo root, `replace` directive for local dev.
8. `plugins/tree-sitter/` shared layer (no language-specific code): cobra root with `--language` dispatch, registry, directory walker, slug sanitization, op emission via `batch/`.
9. `plugins/tree-sitter/languages/golang/`: tree-sitter Go grammar import, language registration, decl extraction (function/method/struct/interface), import statements, intra-package call graph.
10. Golden tests for the Go grammar under `languages/golang/testdata/golden/`.
11. `e2e/extract_self_test.go` — wire everything together (`kg-extractor extract --plugin tree-sitter --language go ...`).
12. Makefile additions, README section on extractors.
