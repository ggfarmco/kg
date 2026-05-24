# kg Plugin Bootstrap — Zero-Toolchain Install Design Spec

**Date:** 2026-05-24
**Status:** Approved for implementation planning
**Builds on:** `2026-05-24-kg-v3-skill-enrichment-design.md` (v3 plugin) — this spec layers a bootstrap pipeline over the existing plugin so end users don't need Go, CGO, or manual file-copying to install.

## Background

v3 shipped a Claude Code plugin (`kg-plugin/`) that wraps the kg engine's structural extractor + LLM enrichment. The plugin is installable via `/plugin marketplace add` + `/plugin install kg@kg-graph` (post-`56043`-workaround) — but `/kg:kg-enrich` immediately fails on a fresh machine because:

1. The `kg` CLI binary must be on PATH (or in `${CLAUDE_PLUGIN_ROOT}/../bin/` for repo dogfooding).
2. The tree-sitter extractor plugin manifest + binary must be copied to `~/.config/kg-extractor/plugins/tree-sitter/`.
3. Both require a Go 1.26 + CGO toolchain to build from source.

The first real Claude Code session that ran `/kg:kg-enrich` against the kg repo itself surfaced these exact failures (`docs/v3-fixture-trace.md`, post-install trace). Removing this friction is required before the plugin is usable outside the development context.

This spec describes a two-part fix shipped together as v0.3.1:

- **GitHub releases pipeline** that produces prebuilt platform tarballs on every semver tag (`v*`).
- **`bootstrap.sh` + smart pre-check** in the plugin that auto-discovers `kg`, offers to install from the matching release, and verifies via SHA-256 checksum before extracting to `${KG_HOME:-$HOME/.config/kg}`.

## Goals

1. **Zero-toolchain install for end users.** A new user runs `/plugin marketplace add github:ggfarmco/kg`, `/plugin install kg@kg-graph`, then `/kg:kg-enrich`. The plugin auto-detects missing CLI, offers to install, downloads + verifies + extracts. No Go, no CGO, no `make install`.
2. **Dogfood preservation.** Developers running from a kg checkout (CLI in `./bin/`) still work without bootstrapping — the smart pre-check finds the local binary first.
3. **Reproducible releases.** Every `v*` tag pushed to `github.com/ggfarmco/kg` triggers a GitHub Actions workflow that builds `kg`, `kg-extractor`, `kg-extractor-tree-sitter` natively on each of the 4 supported platforms, packages them as `.tar.gz`, and uploads them to a GitHub release with auto-generated `checksums.txt`.
4. **Lock-step versioning.** Plugin manifest version and CLI release tag move together — the plugin always fetches the release matching its declared version, eliminating skew.
5. **Single source of truth for the install location.** Everything kg-related (binaries, extractor plugins, version marker) lives under `${KG_HOME:-$HOME/.config/kg}`. The plugin's SKILL always knows where to look.

## Non-Goals (deferred)

- **Windows support.** No Windows runner in the GHA matrix. Defer until someone asks.
- **musl/glibc separate Linux builds.** Single `linux/amd64` and `linux/arm64` tarballs built on `ubuntu-24.04` runners (glibc). Alpine users build from source. Re-evaluate if demand emerges.
- **GPG signing.** HTTPS + SHA-256 checksums are sufficient for v0.3.1. Add later if supply-chain concerns surface.
- **Silent auto-update.** Background sneaky downloads. Every install/upgrade requires explicit user consent via `AskUserQuestion`.
- **`SessionStart` hook bootstrap.** Plugin hook system can run bootstrap automatically on session start; rejected to avoid hidden network traffic and confused user state ("why is something downloading?").
- **Independent plugin/CLI versioning + compatibility matrix.** Lock-step versioning eliminates the entire class of problems. Independent versioning is a v0.4+ concern if release cadence diverges.
- **`kg-bootstrap` Go binary.** Chicken-and-egg: needs Go to compile. Pure-bash `bootstrap.sh` is portable and self-contained.
- **Removal / `kg uninstall`.** `rm -rf ~/.config/kg/` is a one-liner. Not worth a custom verb.
- **Background upgrade nagging.** When a newer plugin version is available, the *next* `/kg:kg-enrich` will offer to upgrade — not a passive notification.

## Architecture

Three new components + two modifications to existing files:

```
kg/                                          # repo (with new GitHub remote)
├── .goreleaser.yml                          # NEW: GoReleaser config
├── .github/workflows/release.yml            # NEW: 4-platform matrix → GH release
├── kg-plugin/scripts/bootstrap.sh           # NEW: platform detect → download → install
├── kg-plugin/scripts/tests/bootstrap.test.sh # NEW: fixture-based bootstrap test
├── kg-plugin/commands/kg-enrich.md          # MODIFIED: smart pre-check + auto-install
└── README.md                                # MODIFIED: install section rewrite

~/.config/kg/                                # NEW: stable install location (overridable via KG_HOME)
├── bin/
│   ├── kg
│   ├── kg-extractor
│   └── kg-extractor-tree-sitter
├── extractor-plugins/
│   └── tree-sitter/
│       ├── manifest.json
│       └── kg-extractor-tree-sitter         # symlink → ../../bin/
└── VERSION                                  # plain text "v0.3.1"
```

### Data flow — happy path install

```
User: /plugin install kg@kg-graph                          # already works after v3 + #56043 workaround
User: /kg:kg-enrich
SKILL pre-check:
  1. command -v kg                            → not found
  2. ${KG_HOME:-$HOME/.config/kg}/bin/kg      → not found
  3. ${CLAUDE_PLUGIN_ROOT}/../bin/kg          → not found (no source checkout)
  4. ./bin/kg                                  → not found
SKILL → AskUserQuestion:
  "kg CLI not installed. Download v0.3.1 from github.com/ggfarmco/kg/releases? [y/N]"
User: yes
SKILL: bash ${CLAUDE_PLUGIN_ROOT}/scripts/bootstrap.sh
bootstrap.sh:
  a. Read version from ${CLAUDE_PLUGIN_ROOT}/.claude-plugin/plugin.json → "0.3.1"
  b. Detect: darwin-arm64 / darwin-amd64 / linux-amd64 / linux-arm64 (via uname -sm)
  c. curl -fLo /tmp/kg-bootstrap.tar.gz \
       https://github.com/ggfarmco/kg/releases/download/v0.3.1/kg_v0.3.1_<platform>.tar.gz
  d. curl -fLo /tmp/kg-checksums.txt \
       https://github.com/ggfarmco/kg/releases/download/v0.3.1/checksums.txt
  e. Verify sha256sum of tarball matches checksums.txt entry
  f. mkdir -p ${KG_HOME:-$HOME/.config/kg}/{bin,extractor-plugins/tree-sitter}
  g. tar -xzf /tmp/kg-bootstrap.tar.gz -C ${KG_HOME}/bin
  h. mv ${KG_HOME}/bin/manifest.json ${KG_HOME}/extractor-plugins/tree-sitter/
  i. ln -sf ${KG_HOME}/bin/kg-extractor-tree-sitter \
       ${KG_HOME}/extractor-plugins/tree-sitter/kg-extractor-tree-sitter
  j. echo v0.3.1 > ${KG_HOME}/VERSION
  k. Print: "kg v0.3.1 installed to ${KG_HOME}/bin/. Add to PATH? export PATH=${KG_HOME}/bin:\$PATH"
SKILL re-checks → kg found at ${KG_HOME}/bin/kg → continues enrichment workflow.
```

### Data flow — upgrade path

```
User has ${KG_HOME}/VERSION = "v0.3.0", plugin updated to "v0.3.1".
User: /kg:kg-enrich
SKILL: read VERSION → "v0.3.0"; read plugin.json version → "0.3.1"; mismatch.
SKILL → AskUserQuestion:
  "Installed kg is v0.3.0, plugin needs v0.3.1. Upgrade? [y/N]"
User: yes
bootstrap.sh same flow, overwrites bin/, updates VERSION.
```

## Components

### 1. `.goreleaser.yml`

Standard GoReleaser config with three builds and four platform combinations.

```yaml
version: 2
project_name: kg

builds:
  - id: kg
    main: ./cmd/kg
    binary: kg
    goos: [darwin, linux]
    goarch: [amd64, arm64]
    env: [CGO_ENABLED=0]
    ldflags: ['-s', '-w', '-X main.version={{.Version}}']

  - id: kg-extractor
    main: ./cmd/kg-extractor
    binary: kg-extractor
    goos: [darwin, linux]
    goarch: [amd64, arm64]
    env: [CGO_ENABLED=0]

  - id: kg-extractor-tree-sitter
    main: ./plugins/tree-sitter
    binary: kg-extractor-tree-sitter
    goos: [darwin, linux]
    goarch: [amd64, arm64]
    env: [CGO_ENABLED=1]   # tree-sitter is C
    no_unique_dist_dir: false

archives:
  - id: kg
    name_template: 'kg_{{ .Tag }}_{{ .Os }}_{{ .Arch }}'
    format: tar.gz
    files:
      - src: plugins/tree-sitter/manifest.json
        dst: manifest.json
      - src: README.md
      - src: LICENSE*

checksum:
  name_template: 'checksums.txt'
  algorithm: sha256

release:
  github:
    owner: ggfarmco
    name: kg
  draft: false
  prerelease: auto
```

Each tarball contains:
- `kg`
- `kg-extractor`
- `kg-extractor-tree-sitter`
- `manifest.json` (tree-sitter plugin manifest)
- `README.md`
- `LICENSE` (to be added if absent)

### 2. `.github/workflows/release.yml`

Matrix workflow: 4 parallel native builds, then a release job that aggregates artifacts.

```yaml
name: release
on:
  push:
    tags: ['v*']

permissions:
  contents: write

jobs:
  build:
    strategy:
      fail-fast: false
      matrix:
        include:
          - { os: macos-14,         goos: darwin, goarch: arm64 }
          - { os: macos-13,         goos: darwin, goarch: amd64 }
          - { os: ubuntu-24.04,     goos: linux,  goarch: amd64 }
          - { os: ubuntu-24.04-arm, goos: linux,  goarch: arm64 }
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.26' }
      - uses: goreleaser/goreleaser-action@v6
        with:
          version: '~> v2'
          args: build --single-target --clean
        env:
          GOOS:   ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
      - uses: actions/upload-artifact@v4
        with:
          name: kg-${{ matrix.goos }}-${{ matrix.goarch }}
          path: dist/**

  release:
    needs: build
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
      - uses: actions/download-artifact@v4
        with: { path: dist/ }
      - uses: goreleaser/goreleaser-action@v6
        with:
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

The release job uses `goreleaser release` to bundle artifacts, generate `checksums.txt`, and publish to GitHub releases under the matching tag.

### 3. `kg-plugin/scripts/bootstrap.sh`

Pure-bash installer, ~120 lines. Dependencies: `bash`, `curl`, `tar`, `sha256sum` (or `shasum -a 256` on macOS), `uname`, `mktemp`, `jq` (for reading plugin.json version).

```bash
#!/usr/bin/env bash
set -euo pipefail

# --- config
PLUGIN_ROOT="${CLAUDE_PLUGIN_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
KG_HOME="${KG_HOME:-$HOME/.config/kg}"
REPO_OWNER="ggfarmco"
REPO_NAME="kg"

# --- detect platform
case "$(uname -s)" in Darwin) OS=darwin ;; Linux) OS=linux ;; *) echo "unsupported OS: $(uname -s)" >&2; exit 2 ;; esac
case "$(uname -m)" in arm64|aarch64) ARCH=arm64 ;; x86_64) ARCH=amd64 ;; *) echo "unsupported arch: $(uname -m)" >&2; exit 2 ;; esac
PLATFORM="${OS}_${ARCH}"

# --- read version
VERSION=$(jq -r '.version' "$PLUGIN_ROOT/.claude-plugin/plugin.json" 2>/dev/null || echo "")
if [ -z "$VERSION" ]; then echo "cannot read plugin.json version" >&2; exit 2; fi
TAG="v${VERSION#v}"

# --- already installed?
if [ -f "$KG_HOME/VERSION" ] && [ "$(cat "$KG_HOME/VERSION")" = "$TAG" ] && [ -x "$KG_HOME/bin/kg" ]; then
  echo "kg $TAG already installed at $KG_HOME"; exit 0
fi

# --- download
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
TARBALL="kg_${TAG}_${PLATFORM}.tar.gz"
BASE_URL="https://github.com/$REPO_OWNER/$REPO_NAME/releases/download/$TAG"
echo "Downloading $BASE_URL/$TARBALL ..."
curl -fL --proto '=https' -o "$TMP/$TARBALL" "$BASE_URL/$TARBALL"
curl -fL --proto '=https' -o "$TMP/checksums.txt" "$BASE_URL/checksums.txt"

# --- verify
EXPECTED=$(grep " $TARBALL\$" "$TMP/checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then echo "checksum entry missing for $TARBALL" >&2; exit 1; fi
if command -v sha256sum >/dev/null; then
  ACTUAL=$(sha256sum "$TMP/$TARBALL" | awk '{print $1}')
else
  ACTUAL=$(shasum -a 256 "$TMP/$TARBALL" | awk '{print $1}')
fi
if [ "$ACTUAL" != "$EXPECTED" ]; then
  echo "checksum mismatch: expected $EXPECTED, got $ACTUAL" >&2; exit 1
fi

# --- install
mkdir -p "$KG_HOME/bin" "$KG_HOME/extractor-plugins/tree-sitter"
tar -xzf "$TMP/$TARBALL" -C "$KG_HOME/bin" --strip-components=0
chmod +x "$KG_HOME/bin/"*
mv -f "$KG_HOME/bin/manifest.json" "$KG_HOME/extractor-plugins/tree-sitter/manifest.json"
ln -sf "$KG_HOME/bin/kg-extractor-tree-sitter" "$KG_HOME/extractor-plugins/tree-sitter/kg-extractor-tree-sitter"
echo "$TAG" > "$KG_HOME/VERSION"

echo "kg $TAG installed to $KG_HOME"
echo "Add to PATH (optional): export PATH=\"$KG_HOME/bin:\$PATH\""
```

### 4. `kg-plugin/scripts/tests/bootstrap.test.sh`

Fixture-based test that mocks `curl` and `sha256sum` via PATH-shadowing, runs `bootstrap.sh`, asserts the install layout is correct.

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

# Set up fake plugin checkout
PLUGIN_ROOT=$(mktemp -d)
trap 'rm -rf "$PLUGIN_ROOT" "$FAKE_HOME"' EXIT
mkdir -p "$PLUGIN_ROOT/.claude-plugin" "$PLUGIN_ROOT/scripts"
echo '{"name":"kg","version":"0.3.1"}' > "$PLUGIN_ROOT/.claude-plugin/plugin.json"
cp ../bootstrap.sh "$PLUGIN_ROOT/scripts/"

# Set up fake home
FAKE_HOME=$(mktemp -d)
export HOME="$FAKE_HOME"
export KG_HOME="$FAKE_HOME/.config/kg"
export CLAUDE_PLUGIN_ROOT="$PLUGIN_ROOT"

# Create a real tarball with stub binaries + manifest
STUB=$(mktemp -d)
echo '#!/bin/sh' > "$STUB/kg"; echo 'echo kg' >> "$STUB/kg"; chmod +x "$STUB/kg"
echo '#!/bin/sh' > "$STUB/kg-extractor"; chmod +x "$STUB/kg-extractor"
echo '#!/bin/sh' > "$STUB/kg-extractor-tree-sitter"; chmod +x "$STUB/kg-extractor-tree-sitter"
echo '{"name":"tree-sitter"}' > "$STUB/manifest.json"
tar -C "$STUB" -czf "$STUB/tarball.tar.gz" kg kg-extractor kg-extractor-tree-sitter manifest.json
EXPECTED_HASH=$(shasum -a 256 "$STUB/tarball.tar.gz" | awk '{print $1}')
TARBALL_NAME="kg_v0.3.1_$(uname -s | tr A-Z a-z)_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
echo "$EXPECTED_HASH  ${TARBALL_NAME}.tar.gz" > "$STUB/checksums.txt"

# Shadow curl: route github.com URLs to local files
SHIM=$(mktemp -d)
cat > "$SHIM/curl" <<EOF
#!/usr/bin/env bash
out=""
for arg; do
  case "\$arg" in -o) shift; out="\$1" ;; esac
  shift || true
done
case "\$*" in
  *"${TARBALL_NAME}.tar.gz"*) cp "$STUB/tarball.tar.gz" "\$out" ;;
  *"checksums.txt"*)         cp "$STUB/checksums.txt" "\$out" ;;
  *) echo "unexpected curl: \$*" >&2; exit 1 ;;
esac
EOF
chmod +x "$SHIM/curl"
export PATH="$SHIM:$PATH"

# Run bootstrap
bash "$PLUGIN_ROOT/scripts/bootstrap.sh"

# Assertions
[ -x "$KG_HOME/bin/kg" ]                                         || { echo FAIL kg not installed; exit 1; }
[ -x "$KG_HOME/bin/kg-extractor" ]                               || { echo FAIL kg-extractor; exit 1; }
[ -x "$KG_HOME/bin/kg-extractor-tree-sitter" ]                   || { echo FAIL plugin binary; exit 1; }
[ -f "$KG_HOME/extractor-plugins/tree-sitter/manifest.json" ]    || { echo FAIL manifest; exit 1; }
[ -L "$KG_HOME/extractor-plugins/tree-sitter/kg-extractor-tree-sitter" ] || { echo FAIL symlink; exit 1; }
[ "$(cat "$KG_HOME/VERSION")" = "v0.3.1" ]                       || { echo FAIL VERSION; exit 1; }

echo "OK bootstrap.sh"
```

A second test asserts the idempotency path (running bootstrap twice with the same VERSION is a no-op exit 0).

### 5. SKILL pre-check modification

`kg-plugin/commands/kg-enrich.md` Pre-check section gains the multi-location lookup + auto-install offer. Replacing the current minimal pre-check:

```bash
# Try each candidate in priority order; first executable wins.
KG_BIN=""
for c in \
  "$(command -v kg 2>/dev/null || true)" \
  "${KG_HOME:-$HOME/.config/kg}/bin/kg" \
  "${CLAUDE_PLUGIN_ROOT}/../bin/kg" \
  "$(pwd)/bin/kg"; do
  if [ -n "$c" ] && [ -x "$c" ]; then KG_BIN="$c"; break; fi
done

if [ -z "$KG_BIN" ]; then
  # Offer install via AskUserQuestion.
  # If yes: bash "${CLAUDE_PLUGIN_ROOT}/scripts/bootstrap.sh"; then re-run loop.
  # If no: abort with pointer to manual install.
fi
```

All subsequent `kg ...` invocations in the SKILL body become `"$KG_BIN" ...`. Same pattern applied to `kg-extractor` lookup where the SKILL needs it (none in current /kg-enrich body, but other commands like /kg-onboard call `kg node get` — they reuse `$KG_BIN`).

Existing version check: if `$KG_BIN` is found but `${KG_HOME}/VERSION` != plugin's expected `version`, offer upgrade via the same AskUserQuestion flow.

### 6. README.md install section rewrite

Replace the current 5-step recipe with:

```sh
# In Claude Code:
/plugin marketplace add github:ggfarmco/kg
/plugin install kg@kg-graph

# Then in any project directory:
/kg:kg-enrich
# (on first run, plugin offers to install the kg CLI; accept once, done forever)
```

The old "build from source" recipe stays in a `### Developer setup` subsection for contributors.

## Versioning

- **Tag → release:** `git tag v0.3.1 && git push origin v0.3.1` triggers the workflow.
- **Plugin manifest:** `kg-plugin/.claude-plugin/plugin.json` `version` field is the source of truth for "what CLI version this plugin expects." Always exactly matches the release tag (without leading `v`, per semver convention — bootstrap.sh adds `v` prefix when constructing URL).
- **Plugin update mechanism:** Claude Code's `/plugin update kg@kg-graph` pulls the latest marketplace.json + plugin.json from the git remote. Next `/kg:kg-enrich` reads new version, detects mismatch, offers upgrade.
- **Marketplace.json:** does NOT track versions per plugin entry. Marketplace lists plugins; versions live in each plugin's own manifest.

## Error handling

| Failure | Behavior |
|---|---|
| `curl` fails (network, 404) | bootstrap exits non-zero with the curl error; SKILL prints `manual install: see README` pointer |
| Checksum mismatch | bootstrap exits 1 with `expected X, got Y` + a `report this at https://github.com/ggfarmco/kg/issues` hint |
| Unsupported platform | bootstrap exits 2 with `kg release not available for <platform>; build from source: https://...` |
| `jq` not installed (reading plugin.json version) | bootstrap fallback: read version via `grep '"version"' plugin.json | sed ...` (less robust but no extra dep) |
| `sha256sum` and `shasum` both absent | bootstrap exits with clear error; documented as a one-off install hint |
| Existing different version | AskUserQuestion → user chooses upgrade or stay on old version (skill aborts cleanly if they decline) |
| Existing same version (idempotency) | bootstrap exits 0 immediately with "already installed" |
| `~/.config/kg/` exists but is not a directory | bootstrap errors out; user fixes manually |
| `tar -xzf` fails (corrupt download) | bootstrap exits 1 (relies on `set -e`); user re-runs |

The SKILL itself never silently swallows bootstrap failures — if bootstrap exits non-zero, the SKILL surfaces the error message and aborts the enrichment workflow.

## Testing strategy

### Bash test (`make test-scripts`)

`bootstrap.test.sh` (covered in component 4 above) mocks `curl` via PATH shimming and verifies the install layout. Runs in <1s. Added to `make test-scripts` target output (6 OK lines after this lands).

A companion `bootstrap-idempotent.test.sh` runs bootstrap twice and asserts the second run exits 0 immediately with "already installed."

### GHA workflow test

Tested by the first real `v0.3.1` tag — if the workflow succeeds, all four tarballs upload, and `checksums.txt` is generated. No dry-run; small enough that fixing CI on the fly is acceptable.

### End-to-end smoke

After v0.3.1 release lands on GitHub:
1. Spin up a `docker run --rm -it ubuntu:24.04 bash` (or use a clean homedir on macOS).
2. Install Claude Code + run through `/plugin marketplace add` + `/plugin install`.
3. `/kg:kg-enrich` triggers auto-install offer; accept it.
4. Run against `testdata/v3-fixture/` (already populates an extracted graph if user first runs `kg init` + `kg-extractor extract`).
5. Verify the same observations from `docs/v3-fixture-trace.md` hold (15 enriched nodes, 4 arch layers, 5 tour steps, no manual `make` invocation).

Capture this trace in a new `docs/v3-bootstrap-trace.md` placeholder for the first install reproducer to fill in.

## Open risks

| Risk | Mitigation |
|---|---|
| CGO build fails on `ubuntu-24.04-arm` runner (newer runner with arm64) | Use `ubuntu-22.04-arm` if 24.04-arm has gcc/CGO toolchain issues; rate-of-change suggests these stabilize quickly |
| GoReleaser v2 changes archive layout, breaks bootstrap.sh's tar extraction | bootstrap.sh tests assert layout; will catch on the next bootstrap test run after a GoReleaser bump |
| GHA matrix runs out of build minutes on the free tier | 4 jobs × ~3 min each = 12 min per release; well under monthly free tier limits even at weekly cadence |
| `jq` not present on user's machine breaks version parsing | Document as a prereq; consider fallback `grep/sed` if a real user trips it |
| GitHub release ratelimits anonymous `curl` for the binary download | `curl` to releases is rate-limit-friendly compared to API calls; documented `GH_TOKEN` env var fallback for paranoid users |
| `${CLAUDE_PLUGIN_ROOT}` path semantics differ across Claude Code versions | Tested on 2.1.150; SKILL code uses both `${CLAUDE_PLUGIN_ROOT}` and `command -v` cascade so a single missing-var case doesn't break the lookup |
| User's `~/.config/kg/` is read-only or has unexpected permissions | bootstrap errors clearly; documented as edge case |
| First real `v0.3.1` release fails CI partway and produces a half-complete GitHub release | GoReleaser is idempotent; re-running the workflow against the same tag (force-push tag if needed) should complete; document recovery in README |
| Lock-step versioning forces a new tag for every plugin-only change (e.g., README edit) | Document: plugin-only changes can ship without bumping CLI binary by skipping the matching release (plugin reads `version` and finds existing v0.3.x release — bootstrap is idempotent). Re-evaluate if this gets confusing. |

## Implementation plan (to be expanded by writing-plans)

Approximately 5 tasks across 2 phases:

**Phase 1 — Local plumbing (no GitHub required yet)**
1. Write `bootstrap.sh` + `bootstrap.test.sh` + `bootstrap-idempotent.test.sh`; update `Makefile`'s `test-scripts` target if needed.
2. Modify `kg-plugin/commands/kg-enrich.md` pre-check to use the multi-location lookup + auto-install offer; bump `kg-plugin/.claude-plugin/plugin.json` version to `0.3.1`.
3. Update `README.md` install section.

**Phase 2 — Release pipeline**
4. Write `.goreleaser.yml` + `.github/workflows/release.yml`. Add `LICENSE` file if absent. Set up the `github.com/ggfarmco/kg` remote (`git remote add origin ...` + initial push).
5. Tag `v0.3.1`, push, observe CI, fix issues, verify release artifacts. Run end-to-end smoke test (in fresh Claude Code session or docker container). Capture trace in `docs/v3-bootstrap-trace.md`.

Estimated scope: ~300 LOC bash + ~100 LOC YAML + minor markdown edits. One full day of focused work.
