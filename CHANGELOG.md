# Changelog

## v0.3.1 — 2026-05-24

### Plugin (Claude Code)

- New: `kg-plugin/scripts/bootstrap.sh` — pure-bash installer that downloads the matching kg CLI release from `github.com/ggfarmco/kg/releases`, verifies SHA-256, and extracts to `${KG_HOME:-$HOME/.config/kg}/`.
- All four SKILL commands (`/kg-enrich`, `/kg-explain`, `/kg-tour`, `/kg-onboard`) now locate `kg` via a 4-step cascade (`PATH` → `$KG_HOME/bin/` → repo `bin/` → `$(pwd)/bin/`). On first run with no install, the SKILL offers via `AskUserQuestion` to download the matching release.
- Plugin version + release tag are lock-step: `plugin.json` `version` and the release tag (without `v` prefix) always match.

### Release pipeline

- New: `.goreleaser.yml` + `.github/workflows/release.yml` build all 4 supported platforms (`darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`) natively on GitHub-hosted runners. Each release uploads 4 `.tar.gz` archives (containing `kg`, `kg-extractor`, `kg-extractor-tree-sitter`, plus `manifest.json`, README, LICENSE) and a single `checksums.txt`.

## v0.3.0 — 2026-05-24

### Engine
- `Service.Apply` in additive scope now writes properties on foreign-owned nodes (under the writer's source namespace). Previously silently skipped.
- Snapshot validator no longer requires `layer` and `name` on `NodeSpec` entries when `scope: additive`. Required fields tighten only for `domain-source` and `domain` scopes.
- New CLI verb: `kg export --domain X --source Y --format snapshot`. Round-trips with `kg apply`.

### Plugin (Claude Code)
- New `.claude-plugin/` directory adds four skills (`/kg-enrich`, `/kg-explain`, `/kg-tour`, `/kg-onboard`) and three subagents (`file-summarizer`, `architecture-analyzer`, `tour-builder`). See README's "v3 enrichment plugin" section.

## v0.2.0 — 2026-05-24

- See spec `docs/superpowers/specs/2026-05-24-kg-v2-provenance-design.md`.
