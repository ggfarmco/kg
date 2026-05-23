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
cat > ~/.config/kg-extractor/plugins/tree-sitter/manifest.json <<'EOF'
{
  "name": "tree-sitter",
  "version": "0.1.0",
  "description": "tree-sitter (Go)",
  "runtime": "native",
  "executable": "kg-extractor-tree-sitter"
}
EOF
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
