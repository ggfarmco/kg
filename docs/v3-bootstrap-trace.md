# v3.1 bootstrap end-to-end trace

**Date:** 2026-05-24
**Tester:** norimori (pisaruk.vadzim@gmail.com)
**Platform:** darwin/arm64 (Apple Silicon)
**Release:** v0.3.1 — https://github.com/ggfarmco/kg/releases/tag/v0.3.1

## Setup

- Clean install root: `$(mktemp -d)/.config/kg/` (no prior install)
- Plugin checkout at `/Users/cali/develop/norimori/ggfarmco/kg/` (just-merged main, commit `e29f5ca`)
- Real GitHub release artifacts (no shimmed `curl`)

## Bootstrap invocation

```bash
TMPHOME=$(mktemp -d)
HOME="$TMPHOME" \
KG_HOME="$TMPHOME/.config/kg" \
CLAUDE_PLUGIN_ROOT="$(pwd)/kg-plugin" \
  bash kg-plugin/scripts/bootstrap.sh
```

## Observations

1. **Platform detect:** `darwin` + `arm64` → `PLATFORM=darwin_arm64`. The Intel-Mac guard added in commit `e29f5ca` did not fire (correctly — this is Apple Silicon).
2. **Version read:** `jq -r '.version' "${CLAUDE_PLUGIN_ROOT}/.claude-plugin/plugin.json"` → `0.3.1`. `TAG=v0.3.1`.
3. **Idempotency short-circuit:** Did not trigger (no pre-existing VERSION file).
4. **Download:** `kg_v0.3.1_darwin_arm64.tar.gz` (~11.2MB) downloaded from `https://github.com/ggfarmco/kg/releases/download/v0.3.1/` in ~1 second on a fast connection. `checksums.txt` (286 bytes) downloaded similarly.
5. **SHA-256 verification:** matched expected `754ace8fae5439cef12a025502ea8e6853804bea528ed26607df038a315aaf8e`. No checksum mismatch.
6. **Extraction + placement:**
   - `$KG_HOME/bin/kg` (11424866 bytes, 0755)
   - `$KG_HOME/bin/kg-extractor` (4499154 bytes, 0755)
   - `$KG_HOME/bin/kg-extractor-tree-sitter` (5118578 bytes, 0755)
   - `$KG_HOME/extractor-plugins/tree-sitter/manifest.json` (230 bytes)
   - `$KG_HOME/extractor-plugins/tree-sitter/kg-extractor-tree-sitter` (symlink → `../../bin/kg-extractor-tree-sitter`)
   - `$KG_HOME/VERSION` = `v0.3.1`
7. **No leaks:** `bin/` contains only the 3 binaries. `extractor-plugins/tree-sitter/` contains only the manifest + symlink. README.md and LICENSE from the tarball were correctly dropped.
8. **Binary runnability:** `$KG_HOME/bin/kg --help` prints `kg — domain-agnostic knowledge graph engine` — binary executes correctly out of the box.

## Compared to pre-v3.1 install

| Step | Pre-v3.1 | v3.1 |
|---|---|---|
| Install kg CLI | `make install` (requires Go 1.26 + CGO toolchain) | `/kg:kg-enrich` triggers `bootstrap.sh` via AskUserQuestion (no toolchain) |
| Install tree-sitter plugin | `make build-plugin-treesitter` + `mkdir -p ~/.config/kg-extractor/plugins/tree-sitter` + 2 `cp` commands | bundled in bootstrap.sh — single tarball |
| Verify install location | manual `ls ~/.config/kg-extractor/...` | `kg --help` works; `$KG_HOME/VERSION` records the tag |
| Time to first `/kg-enrich` ready | ~5 minutes if Go is installed (slower if not) | ~10 seconds on a fast connection |

## CI run reference

- **Final successful run:** https://github.com/ggfarmco/kg/actions/runs/26368705764 (~2 min, 3 matrix builds + release aggregator)
- **First run that hung:** https://github.com/ggfarmco/kg/actions/runs/26366742295 (queued forever on macos-13 / darwin-amd64 — the queue wait was what motivated the `e29f5ca` drop-darwin-amd64 commit)

## Issues found

None blocking. Two notes for future work:

1. **Default branch on remote:** `ggfarmco/kg`'s GitHub-side default branch is still `feat/kg-v3.1-bootstrap` (auto-assigned because that was the first branch pushed to the empty repo). `gh repo edit ggfarmco/kg --default-branch main` requires `admin:repo` scope, which the tester's token lacks. **Manual action needed:** in the GitHub UI under Settings → General → Default branch, change to `main`. Then `feat/kg-v3.1-bootstrap` can be deleted on the remote.
2. **Zombie CI run 26366742295:** unable to cancel from this token (no admin scope). It will auto-time-out after GitHub's default 6-hour limit and is harmless in the meantime (the macos-13 build will never start since the runner was dropped).

## Validation script (reproducible)

```bash
TMPHOME=$(mktemp -d)
HOME="$TMPHOME" \
KG_HOME="$TMPHOME/.config/kg" \
CLAUDE_PLUGIN_ROOT="$(pwd)/kg-plugin" \
  bash kg-plugin/scripts/bootstrap.sh
ls -la "$TMPHOME/.config/kg/bin/"
"$TMPHOME/.config/kg/bin/kg" --help | head -3
rm -rf "$TMPHOME"
```
