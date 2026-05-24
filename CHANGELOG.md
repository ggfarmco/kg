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
