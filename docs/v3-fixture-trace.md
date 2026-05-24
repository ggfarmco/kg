# v3-fixture smoke trace

**Status:** unfilled — this is a structural placeholder. Run `/kg-enrich --domain fixture` against `testdata/v3-fixture/` in a Claude Code session, then replace `<TO BE FILLED>` markers below with the observed values. Commit the filled version under a follow-up `docs(test):` commit, leaving the structure intact so future regression checks have a baseline to compare against.

Captured on: `<DATE>` by `<reproducer-name>`.

## Pre-enrichment state

Setup:

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

Expected pre-state:

```
$ kg node list --domain fixture --source tree-sitter:0.2.0 --limit 0 | jq '.data | length'
<TO BE FILLED: total node count = N file + N decl + N package>
```

## After /kg-enrich

### file-summarizer
- Batches dispatched: `<TO BE FILLED: 1 batch (4 files < 25 per batch)>`
- Batches succeeded: `<TO BE FILLED>`
- Summary samples:
  - `fixture:fixture/handler-go`: `<TO BE FILLED: observed summary>`
  - `fixture:fixture/handler-go::servehttp`: `<TO BE FILLED: observed summary>`
- Semantic edges added: `<TO BE FILLED: observed count>` (typical: `handler::ServeHTTP --depends_on--> service::GetUser`)

### architecture-analyzer
- Layers (observed): `<TO BE FILLED: e.g., "HTTP Layer", "Service Layer", "Storage Layer">`
- Layer-to-file `contains` edges: `<TO BE FILLED>`

### tour-builder
- Steps (observed): `<TO BE FILLED: e.g., 3-5 steps starting with "Project Overview" → "Entry Point" → "HTTP Handler" → "Service Logic" → "Storage">`

### Summary report

```
<TO BE FILLED: paste literal report output from /kg-enrich>
```

## Notes / surprises

`<TO BE FILLED: anything unexpected — hallucinated edges, wrong layer assignments, missing decls, etc. These observations guide future prompt tuning.>`
