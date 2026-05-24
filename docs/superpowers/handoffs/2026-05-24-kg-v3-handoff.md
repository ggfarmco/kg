# kg v3 handoff — Skill-Driven LLM Enrichment

**Date:** 2026-05-24
**Status:** Brainstorm + spec done. Plan not yet written. Implementation not started.
**Next action:** invoke `superpowers:writing-plans` to author the implementation plan.

## TL;DR for resuming

Open a new Claude Code session in `/Users/cali/develop/norimori/ggfarmco/kg/` and say:

> Загружаю контекст v3 продолжения работы: @docs/superpowers/handoffs/2026-05-24-kg-v3-handoff.md и @docs/superpowers/specs/2026-05-24-kg-v3-skill-enrichment-design.md. Запускай `superpowers:writing-plans` чтобы написать implementation plan для v3.

Then let the skill drive — it will probably ask 1-2 clarifying questions, then generate the plan at `docs/superpowers/plans/2026-05-24-kg-v3-implementation.md`.

After plan is written and self-reviewed, the skill will ask you to pick execution mode. Pick **subagent-driven-development** (the same one we used for v2).

## Project state

- Working dir: `/Users/cali/develop/norimori/ggfarmco/kg/`
- Branch: `main` (only branch left)
- Latest commits:
  - `1fe1b2d docs(v3): skill-driven LLM enrichment design spec` ← the design
  - `ae23382 refactor(v2): remove trust — _property_sources + changes log give enough attribution`
  - `873cfbb feat: v2 — provenance + declarative apply` (merge commit, 35 implementation commits underneath)
  - `27498e3 feat: v1 — extractor system`
- Tests: all green (`make test`, `make test-all`, `make e2e`)
- No uncommitted changes, no other branches

## What v3 is

A Claude Code plugin layered over kg that uses LLM agents (via Claude Code's subagent dispatch) to enrich the structural graph that tree-sitter produces:
- Per-decl summaries, tags, complexity ratings, semantic edges (writer: `kg-summary:0.1.0`)
- Architectural layer inference (writer: `kg-arch:0.1.0`, creates new `<orig>-arch` domain)
- Pedagogical tour generation (writer: `kg-tours:0.1.0`, creates new `<orig>-tours` domain)
- Onboarding doc generation (read-only)

Pattern is taken from Understand-Anything (see [Understand-Anything investigation](#understand-anything-reference) section).

## Scope decisions already made (don't relitigate)

These were brainstormed in the previous session — locked in for v3 MVP:

1. **Skills (4):** `/kg-enrich`, `/kg-explain`, `/kg-tour`, `/kg-onboard`
2. **Agents (3):** `file-summarizer`, `architecture-analyzer`, `tour-builder`
3. **Source IDs per agent (not shared):** `kg-summary:0.1.0`, `kg-arch:0.1.0`, `kg-tours:0.1.0`
4. **Architecture output:** new `<orig>-arch` domain + cross-domain `contains` edges (NOT properties on file nodes)
5. **Tour storage:** new `<orig>-tours` domain + cross-domain `teaches` edges (consistent with arch)
6. **Batching:** 5 parallel file-summarizer agents (matches UA)
7. **No heuristic fallbacks** (if LLM unavailable, agent fails clean)
8. **Per-batch apply, NOT merge-then-apply** (kg's namespace model makes parallel applies safe; no merger script needed)
9. **Engine tweak (Phase 1 prerequisite):** `Service.applyNodeSpec` in additive scope must write properties on foreign-owned nodes. Currently it silently skips — breaks the v3 use case.
10. **New CLI verb (Phase 1 prerequisite):** `kg export --domain X --source Y --format snapshot` for agents that want baseline state.

## Scope deferred (don't include in v3 MVP)

- `/kg-domain` (business-process mapping) → v3.1
- `/kg-knowledge` (Karpathy LLM wiki mode) → v3.1
- `/kg-diff` (diff-aware re-enrichment) → v3.1
- `/kg-chat` (conversational graph exploration) → v3.1
- Multi-environment plugin packaging (Codex, Cursor, Gemini, Copilot) → v3.1 or later
- More tree-sitter languages (Python, TS, Rust) → v5+
- Dashboard (React/Vite UI) → **v4** (separate effort; will likely fork UA's dashboard + add `kg export --format ua-dashboard` converter)
- MCP server / HTTP API → v5+
- Embeddings / semantic search → v5+ or v7
- Fingerprint-based incremental update → not needed; `kg apply` is already idempotent

## Files to read in the new session (priority order)

1. **`docs/superpowers/specs/2026-05-24-kg-v3-skill-enrichment-design.md`** — the v3 design spec (732 lines). The plan derives from this.
2. **`docs/superpowers/plans/2026-05-24-kg-v2-provenance-implementation.md`** — most recent plan, the stylistic reference for what v3's plan should look like.
3. **`docs/superpowers/specs/2026-05-24-kg-v2-provenance-design.md`** — v2 design, for context on the engine v3 builds on.
4. **`docs/superpowers/plans/2026-05-23-kg-v1-implementation.md`** — older plan, also stylistic reference (more mechanical phases like the script-bundling ones in v3).
5. **`docs/superpowers/specs/2026-05-23-kg-v1-extractor-design.md`** and **`2026-05-23-kg-mvp-design.md`** — only if you need to understand v0/v1 details.

## Expected plan shape

From the spec's "Implementation plan" section sketch (will be expanded by writing-plans skill):

**Phase 1 — Engine prep (3 tasks)**
1. Engine tweak: additive scope writes properties on foreign-owned nodes (+ relaxed NodeSpec validation: layer/parent/name optional in additive scope when id exists).
2. New CLI verb: `kg export --domain X --source Y --format snapshot`.
3. README + CHANGELOG update for v3 engine changes.

**Phase 2 — Plugin scaffold (2 tasks)**
4. Create `.claude-plugin/` directory structure with `plugin.json` and `marketplace.json`.
5. Implement bundled scripts: `dump-files.sh`, `dump-batch-context.sh`, `dump-graph-shape.sh`, `dump-topology.sh`, `apply-snapshot.sh`. Tests for each.

**Phase 3 — Agents (3 tasks)**
6. Write `agents/file-summarizer.md` — the largest agent (3k+ words). Two-phase contract, output spec, retry logic, semantic edge vocabulary.
7. Write `agents/architecture-analyzer.md`.
8. Write `agents/tour-builder.md`.

**Phase 4 — Skills (4 tasks)**
9. Write `skills/kg-enrich/SKILL.md` — orchestrator with 5 dispatch waves.
10. Write `skills/kg-explain/SKILL.md`.
11. Write `skills/kg-tour/SKILL.md`.
12. Write `skills/kg-onboard/SKILL.md`.

**Phase 5 — Tests + polish (4 tasks)**
13. Manual smoke test against `testdata/v3-fixture/` (small Go project), capture observed behavior in `docs/v3-fixture-trace.md`.
14. E2E test `e2e/enrich_self_test.go` (build tag, manual-trigger CI job).
15. README v3 section.
16. Branch close-out + PR.

Total: ~15-18 tasks. Substantially smaller than v2 (30 tasks).

## Caveats / things to watch out for

- **Engine tweak MUST be Phase 1 task 1.** Without it, file-summarizer's `kg apply` silently no-ops. The whole plugin breaks if Phase 1 is skipped.
- **Skill prompts are hard to TDD.** Markdown-driven orchestration is fragile to LLM behavior. Plan should include manual smoke-test step, and skill bodies should embed defensive guard rails (input validation, retry-with-correction patterns) copied from UA's understand SKILL.md.
- **e2e for v3 costs real LLM tokens.** Spec recommends running on main post-merge only, not every PR. Plan should include a `make e2e-enrich` target gated behind `LLM_ENABLED=1` env var or build tag.
- **No remote git configured.** When PR creation comes up at the end, kg has no remote (we saw this in the v2 merge). Either set up a remote first, or merge locally with `--no-ff` like we did for v2.
- **Don't add a Go binary `cmd/kg-enricher`.** The user explicitly chose the skill+agent path over a Go binary. The Go binary option remains for v3.1+ if there's demand for headless/CI enrichment.
- **`/kg-enrich` will run on a project's own code.** When developing, smoke-test against `internal/graph` (the kg engine's own code, which already has tree-sitter graph from v1/v2 e2e tests).

## Understand-Anything reference

UA lives at `/Users/cali/develop/norimori/Understand-Anything/`. v3's architecture mirrors UA's patterns closely. If you need to dig deeper into how UA does things:

- `understand-anything-plugin/skills/understand/SKILL.md` — the main orchestrator (~790 lines, ~6.5k words). The single best reference for how a complex skill is structured.
- `understand-anything-plugin/agents/file-analyzer.md` — UA's equivalent of our `file-summarizer`. Excellent reference for the two-phase contract pattern.
- `understand-anything-plugin/agents/architecture-analyzer.md`, `tour-builder.md`, `project-scanner.md` — other agents worth reading.
- `understand-anything-plugin/packages/core/src/analyzer/` — TS-side prompt construction patterns (`llm-analyzer.ts`, `layer-detector.ts`, `tour-generator.ts`).
- `understand-anything-plugin/packages/core/src/schema.ts` — three-tier validation (sanitize → alias-fix → schema-parse) with alias maps for common LLM-emitted variants.

In the previous session a full UA investigation report was generated by an Explore agent — comprehensive coverage of the architecture. The report wasn't saved to a file. If you want it in the new session, re-run the investigation prompt.

## Decision log (chronological summary of brainstorm)

In case you want to revisit any choice:

| Question | Decision | Rationale |
|---|---|---|
| Skill+agent vs Go binary? | Skill+agent | Mirrors UA pattern. Claude Code handles LLM calls, key mgmt, batching. No new Go deps. |
| Which skills in MVP? | 4: enrich/explain/tour/onboard (Scope C) | Full onboarding loop. /domain and /knowledge punted to v3.1. |
| One source id for all agents or separate? | Separate per agent | Enables targeted re-runs (just re-enrich, just re-architect, just re-tour). Cost: more rows in `sources` table. |
| Architecture output: domain or properties? | Domain | Cross-domain edges are v0 design intent. Layer descriptions get first-class nodes. Multi-arch-view composable. |
| Tour storage: domain or edge-chain or sidecar? | Domain (consistent with arch) | Same reasoning. Step metadata fits naturally. Multi-tour ready. |
| Batching: parallel vs sequential? | 5 parallel (matches UA) | LLM is slow part, batching wins; kg apply is fast, contention low. |
| Heuristic fallbacks? | No | YAGNI; if LLM unavailable, fail clean. |
| Per-batch apply or merge-first? | Per-batch | kg namespaces make parallel applies safe; no merger script overhead. |

## Status of related work

- v2 branch `feat/kg-v2-provenance`: deleted (merged into main on 2026-05-24 as merge commit `873cfbb`)
- v1 branch `feat/kg-v1`: deleted (was already in main)
- No open branches
- No uncommitted changes
- Latest test run before handoff: all green (`make test`, `make test-all`, `make e2e`)
