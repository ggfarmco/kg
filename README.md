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
