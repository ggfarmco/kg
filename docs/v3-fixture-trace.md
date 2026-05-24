# v3-fixture smoke trace

**Status:** filled — captured on 2026-05-24 via manual orchestration of the v3 SKILL.md flow (no headless `claude code run` exists at this time, so the pipeline was driven step-by-step using subagent dispatch + direct `kg apply` invocations). Re-run this end-to-end after the next v3 SKILL.md edit and re-fill if observations change.

## Setup recipe

```bash
make build build-extractor build-plugin-treesitter

mkdir -p ~/.config/kg-extractor/plugins/tree-sitter
cat > ~/.config/kg-extractor/plugins/tree-sitter/manifest.json <<'EOF'
{
  "name": "tree-sitter",
  "version": "0.2.0",
  "description": "tree-sitter (Go) declarative",
  "runtime": "declarative-native",
  "executable": "kg-extractor-tree-sitter",
  "source_id": "tree-sitter:0.2.0",
  "trust": 100
}
EOF
cp ./bin/kg-extractor-tree-sitter ~/.config/kg-extractor/plugins/tree-sitter/

rm -f /tmp/v3-smoke.db /tmp/v3-smoke.db-wal /tmp/v3-smoke.db-shm
./bin/kg --db /tmp/v3-smoke.db init
./bin/kg-extractor extract \
  --plugin tree-sitter --language go \
  --input "$(pwd)/testdata/v3-fixture" --domain fixture \
  --db /tmp/v3-smoke.db --kg-binary ./bin/kg
```

## Pre-enrichment state

```
$ kg node list --domain fixture --source tree-sitter:0.2.0 --limit 0 | jq '.data | length'
16
```

Breakdown (via `node list | jq 'group_by(.layer)'`):
- `package`: 1 (`fixture:v3-fixture`)
- `file`: 4 (handler.go, main.go, service.go, store.go)
- `decl`: 11

Edges from tree-sitter: 0 (single-package fixture; no inter-file imports get extracted as edges).

## After /kg-enrich

### file-summarizer

- Batches dispatched: **1** (4 files < 25 per batch)
- Batches succeeded: **1**
- `kg apply` result: `nodes_updated: 15, edges_added: 10, claims_added: 10` (retries: 0)
- Summary samples (kg-summary:0.1.0 namespace):
  - `fixture:v3-fixture/handler` → "Defines the HTTP handler layer that bridges incoming requests to the service tier. Decodes query parameters, delegates to Service.GetUser, and encodes JSON responses." (tags: `http handler json request-routing`, complexity: `simple`)
  - `fixture:v3-fixture/handler::servehttp` → "Handles an HTTP request by extracting the id query param, calling Service.GetUser, and writing a JSON-encoded User or an error response. Implements http.Handler." (tags: `http json error-handling request-handling`, complexity: `simple`)
  - `fixture:v3-fixture/store::find` → "Looks up a User by ID in the in-memory map, returning nil error on miss. Context parameter is accepted but unused." (tags: `query lookup user`, complexity: `trivial`)
- Semantic edges added: **10** (all vocabulary-compliant — `depends_on` × 4, `uses` × 6; no hallucinated types)
- Example edges:
  - `handler::handler --depends_on--> service::service`
  - `handler::servehttp --uses--> service::getuser`
  - `service::service --depends_on--> store::store`
  - `main::main --uses--> handler::newhandler` (+ same to newservice + newstore)

### architecture-analyzer

- Layers (observed):
  1. **Entry Point** — `Application entry point that wires all layers together.`
  2. **HTTP API Layer** — `HTTP handlers that parse requests and write responses.`
  3. **Service Layer** — `Business logic orchestrating domain operations.`
  4. **Storage Layer** — `Data persistence and retrieval abstractions.`
- `contains` edges (cross-domain `fixture-arch:* → fixture:*`): **4** (one per file)
- README hint was honored — agent used the README's explicit layer names verbatim, then added a fourth "Entry Point" layer for `main.go` rather than forcing it into HTTP.

### tour-builder

- Steps: **5**
  1. `01-data-model` (~4 min) — teaches `store::user` + `store::store`
  2. `02-store-lifecycle` (~5 min) — teaches `store::newstore` + `store::find`
  3. `03-service-layer` (~5 min) — teaches `service::service` + `service::newservice` + `service::getuser`
  4. `04-http-handler` (~6 min) — teaches `handler::handler` + `handler::newhandler` + `handler::servehttp`
  5. `05-entry-point-wiring` (~4 min) — teaches `main::main`
- `teaches` edges total: **11**
- Ordering: bottom-up dependency chain (data types → storage → service → HTTP → composition root). Different from `architecture-analyzer`'s top-down `order` field — both views coexist in the graph.

### Phase 6 summary report

```
/kg-enrich complete for domain fixture:
  ✓ file-summarizer: 1/1 batches
  ✓ architecture-analyzer: ok
  ✓ tour-builder: ok

Graph deltas:
  nodes enriched (kg-summary:0.1.0): 15
  semantic edges added: 10
  arch layers (fixture-arch): 4
  tour steps (fixture-tours): 5

Failures: none
```

`/kg-onboard` produced `testdata/v3-fixture/ONBOARDING.md` (77 lines, fully populated from the enriched graph).

## Multi-source coexistence proof

`kg node get fixture:v3-fixture/handler::servehttp --merged` returns a single object with `_property_sources` attribution showing both writers — tree-sitter's `kind`/`line_start`/`line_end`/`params`/`receiver`/`returns`/`exported`/`name` AND kg-summary's `summary`/`tags`/`complexity` — proving v2's namespaced property model holds end-to-end.

## Notes / surprises (these inform future prompt tuning)

1. **`file_path` is not populated by tree-sitter on file-layer nodes.** `dump-files.sh` resolves it to empty string, so the SKILL pipeline as written cannot hand the agent a real path for the `Read` tool. Workaround in this run: hand-built the batch JSON with explicit absolute paths. **Fix candidates for v3.0.1:** (a) tree-sitter plugin should write `path` into file-layer `properties`, or (b) SKILL Phase 2 should reconstruct the path by joining `--input` root + the parent-package hierarchy.

2. **`manifest.json` for the tree-sitter plugin is referenced in README/SKILL but doesn't exist as a checked-in file** (only inlined in `e2e/extract_self_test.go`). v3 README's install recipe needs `cp plugins/tree-sitter/manifest.json ~/.config/kg-extractor/plugins/tree-sitter/` to succeed — either ship the manifest as a real file under `plugins/tree-sitter/`, or update README to show the inline `cat <<EOF` form.

3. **Single-package fixture has zero imports edges,** so topology has empty `edges[]`. tour-builder degraded gracefully — fell back to filename semantics + README signals to order steps. For a real codebase with multi-package imports, ordering will be sharper.

4. **`depends_on` vs `uses` boundary is fuzzy** for struct-field references (constructor injection felt like `depends_on`; runtime method calls felt like `uses`). Vocabulary text could clarify with one example each.

5. **No retries needed.** All three agent dispatches produced spec-valid output on the first attempt. `kg apply` accepted all three snapshots cleanly.

6. **The 4 layers chosen by architecture-analyzer (Entry Point / HTTP / Service / Storage) is one more than the README's 3** (HTTP / Service / Storage). Adding a separate "Entry Point" layer for `main.go` is a reasonable interpretation — the agent recognized composition-root as architecturally distinct from request-handling.

7. **Total LLM cost for this fixture:** 3 dispatches (1 file-summarizer + 1 architecture-analyzer + 1 tour-builder). On a 100-file project, expect ~6-8 dispatches (5 parallel file-summarizer batches + 1 arch + 1 tour).
