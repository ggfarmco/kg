# Changelog

## v0.3.3 — 2026-05-31

### Engine / extractor

- Fix: the tree-sitter Go extractor lowercases identifiers when deriving decl slugs, so case-distinct declarations in one file (an exported method plus the unexported sqlc query const of the same name, or a package-level func plus a same-named method) collapsed to a single decl id. The duplicate nodes broke `kg apply` with `NODE_EXISTS`, which in turn broke `/kg-enrich` on any project using sqlc-generated code. Colliding decls now get a `-<kind>` suffix (then a numeric tiebreaker) so every decl id is unique.
- Hardening: `snapshot.Validate` now rejects duplicate node ids, so any extractor emitting a collision fails fast with a clear error instead of deep inside `kg apply`.

## v0.3.2 — 2026-05-24

### Plugin (Claude Code)

- New: all four SKILL commands now auto-locate kg.db across `$KG_DB` env → repo-local `./kg.db` → global `${KG_HOME:-$HOME/.config/kg}/kg.db`. If no DB is found, the SKILL prompts via `AskUserQuestion` whether to create it locally or globally.
- New: `/kg-enrich` detects an empty graph (`domain list` returns 0) and offers to auto-run `kg-extractor` against the current directory (default domain: `$(basename "$PWD")`, plugin: `tree-sitter`, language: `go` with override option). This makes `/kg-enrich` work end-to-end from a fresh clone with no manual `kg init` / `kg-extractor extract` steps.
- No CLI / binary changes — release `v0.3.2` ships the same 3 binaries as `v0.3.1` (lock-step version bump for plugin manifest sync).

## v0.3.1 — 2026-05-24

### Plugin (Claude Code)

- New: `kg-plugin/scripts/bootstrap.sh` — pure-bash installer that downloads the matching kg CLI release from `github.com/ggfarmco/kg/releases`, verifies SHA-256, and extracts to `${KG_HOME:-$HOME/.config/kg}/`.
- All four SKILL commands (`/kg-enrich`, `/kg-explain`, `/kg-tour`, `/kg-onboard`) now locate `kg` via a 4-step cascade (`PATH` → `$KG_HOME/bin/` → repo `bin/` → `$(pwd)/bin/`). On first run with no install, the SKILL offers via `AskUserQuestion` to download the matching release.
- Plugin version + release tag are lock-step: `plugin.json` `version` and the release tag (without `v` prefix) always match.

### Release pipeline

- New: `.github/workflows/release.yml` — pure-bash matrix release workflow building 3 supported platforms (`darwin/arm64`, `linux/amd64`, `linux/arm64`) natively on GitHub-hosted runners. Each release uploads 3 `.tar.gz` archives (containing `kg`, `kg-extractor`, `kg-extractor-tree-sitter`, plus `manifest.json`, README, LICENSE) and a single `checksums.txt`. (GoReleaser was evaluated but skipped — the `plugins/tree-sitter/` separate Go module made a pure-bash approach simpler. Intel macOS was dropped from the matrix — macos-13 runners have long queue times on free tier; build from source on Intel Macs.)

## v0.3.0 — 2026-05-24

### Engine
- `Service.Apply` in additive scope now writes properties on foreign-owned nodes (under the writer's source namespace). Previously silently skipped.
- Snapshot validator no longer requires `layer` and `name` on `NodeSpec` entries when `scope: additive`. Required fields tighten only for `domain-source` and `domain` scopes.
- New CLI verb: `kg export --domain X --source Y --format snapshot`. Round-trips with `kg apply`.

### Plugin (Claude Code)
- New `.claude-plugin/` directory adds four skills (`/kg-enrich`, `/kg-explain`, `/kg-tour`, `/kg-onboard`) and three subagents (`file-summarizer`, `architecture-analyzer`, `tour-builder`). See README's "v3 enrichment plugin" section.

## v0.2.0 — 2026-05-24

- See spec `docs/superpowers/specs/2026-05-24-kg-v2-provenance-design.md`.
