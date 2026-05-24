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
write; `kg sources register --id ...` lets you refine the description.
Nodes are single-owner (the source that created them); edges are
reference-counted via `edge_claims`. An edge survives as long as ≥1 source
claims it.

Properties are stored namespaced by source id:

```json
{
  "tree-sitter:0.2.0": {"kind": "function", "line_start": 40},
  "llm-enricher:1.0": {"summary": "Validates a slug..."}
}
```

`kg node get <id>` shows the raw namespaced form by default;
`--source <id>` flattens one namespace; `--merged` returns a union of all
namespaces with a sibling `_property_sources` attribution map.

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

### v3 additions

- `kg apply` with `scope: additive` writes properties on foreign-owned nodes (in the writer's own namespace) — previously such writes were silently dropped. This makes the engine usable for LLM-based annotators that don't own the underlying structural nodes.
- `kg export --domain <id> --source <id>` emits the current `(domain, source)` slice as a snapshot JSON document. Round-trips with `kg apply` for diffing or re-importing. The v3 LLM enrichment plugin uses this to give agents a baseline view of what their source has already written.
- v3's annotation pipeline (Claude Code plugin under `.claude-plugin/`) is a separate concern documented at the end of this README; the engine changes above are usable standalone.

## v3 enrichment plugin (Claude Code)

The `.claude-plugin/` directory at the repo root is a Claude Code plugin that
layers LLM-driven semantic enrichment on top of kg's structural graph. It runs
inside any Claude Code session (CLI, IDE, web). The kg engine (the binaries
above) is a prerequisite.

### Install (v0.3.1+)

```sh
# In Claude Code:
/plugin marketplace add github:ggfarmco/kg
/plugin install kg@kg-graph

# Then in any project directory:
/kg:kg-enrich
# On first run, the plugin offers to download the kg CLI from the matching
# GitHub release (~10MB, verified by SHA-256, installed to ~/.config/kg/).
# Accept once, then it stays cached. The plugin offers an upgrade whenever a
# newer plugin version is installed.
```

The plugin works on `darwin/arm64` (Apple Silicon), `linux/amd64`, and `linux/arm64`. Intel macOS, Windows, and Alpine/musl are not supported; build from source (Developer setup below) on those platforms.

Override the install location via `KG_HOME` (defaults to `$HOME/.config/kg/`). `jq` is a prerequisite for the bootstrap script — install via `brew install jq` or `apt install jq` if absent.

### Developer setup (build from source)

Required only if you're hacking on kg itself or running on an unsupported platform.

```sh
# Build the engine.
make install                       # or: go install ./cmd/kg

# Build the tree-sitter extractor + install its manifest.
make build-extractor build-plugin-treesitter
mkdir -p ~/.config/kg-extractor/plugins/tree-sitter
cp plugins/tree-sitter/manifest.json ~/.config/kg-extractor/plugins/tree-sitter/
cp ./bin/kg-extractor-tree-sitter ~/.config/kg-extractor/plugins/tree-sitter/

# Then in Claude Code, point the marketplace at your local checkout:
/plugin marketplace add /absolute/path/to/this/kg/checkout
/plugin install kg@kg-graph
```

The SKILL pre-check finds the locally-built `./bin/kg` and uses it directly — no auto-install offer is triggered for dogfooders.

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

```sh
# In Claude Code, with kg.db in your cwd:
/kg-enrich
# ... wait for batches to complete ...
/kg-onboard
# review docs/ONBOARDING.md
```

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
