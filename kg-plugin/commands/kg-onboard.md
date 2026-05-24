---
description: Generates a markdown onboarding document (default path `docs/ONBOARDING.md`) from an enriched kg graph. Combines the project description, architectural overview, and tour steps with cross-references to file paths and decl summaries. Use after /kg-enrich.
argument-hint: [--domain <id>] [--output <path>] [--arch-domain <id>] [--tours-domain <id>]
allowed-tools: Read, Bash, Write, AskUserQuestion
---

# /kg-onboard

Generates `docs/ONBOARDING.md` (or a user-specified path) from the kg graph.

## Arguments

- `--domain <id>` (default: auto-detect or prompt)
- `--output <path>` (default: `docs/ONBOARDING.md`)
- `--arch-domain <id>` (default: `<domain>-arch`)
- `--tours-domain <id>` (default: `<domain>-tours`)

## Pre-check

- `kg --version` works
- `<domain>` exists
- `<tours-domain>` has at least one step (otherwise the doc would be skeletal). If empty, tell user to run /kg-enrich or /kg-tour first.

## Workflow

1. **Project header.** Read the top-layer node (usually `package`):
   ```bash
   kg node list --domain "<domain>" --layer package --limit 1
   ```
   Use its `name` as the H1 title. Use its `kg-summary:0.1.0.summary` (if any) as the intro paragraph.

2. **Architecture section.** If `<arch-domain>` exists:
   ```bash
   kg node list --domain "<arch-domain>" --source kg-arch:0.1.0
   ```
   Sort by `properties.order`. For each layer, emit a subsection with its `description` and a bullet list of the file paths it `contains`:
   ```bash
   kg edge list-from "<layer-node-id>" --type contains
   ```
   For each `target`, fetch its merged properties (file path comes from `tree-sitter:0.2.0`).

3. **Tour section.** Pull steps sorted by `order`:
   ```bash
   kg node list --domain "<tours-domain>" --source kg-tours:0.1.0
   ```
   For each step:
   - H3 heading: `Step N — <name> (~M minutes)`
   - The `description` paragraph
   - Bullet list of `teaches` targets with their summaries:
     ```bash
     kg edge list-from "<step-node-id>" --type teaches
     ```
     For each target, fetch `kg-summary:0.1.0.summary` and the file path.

4. **Write the file.** Confirm the path with the user before writing if it would overwrite an existing file. Use the `Write` tool.

## Output template

```markdown
# <Project Name>

<intro paragraph from package summary>

## Architecture

<for each layer in order:>
### <Layer name>

<layer description>

Files:
- `<file path>` — <file summary>

## Tour

<for each step:>
### Step N — <step name> (~M minutes)

<step description>

Covers:
- `<file path>` (`<node id>`) — <file summary>
```

## Edge cases

- **Project has no package-layer summary:** use the directory name and a one-line synthesized intro ("`<project>` is a Go codebase with `<arch_layers_count>` architectural layers and `<file_count>` source files.").
- **No architecture domain:** skip the Architecture section entirely.
- **Existing ONBOARDING.md:** ask the user before overwriting via AskUserQuestion.

## Non-goals

- Don't fetch source files. The summaries are the authoritative content.
- Don't dispatch agents.
- Don't mutate the graph.
