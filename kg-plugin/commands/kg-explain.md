---
description: Read-only. Answers questions about a specific kg node using its enriched properties + 1-hop neighborhood. No graph mutation. Use when the user wants to understand what a specific function, file, or package does in context.
argument-hint: <node-id>
allowed-tools: Read, Bash
---

# /kg-explain

Explain a kg node using all available enrichment (tree-sitter structure + LLM summaries) plus its immediate graph neighborhood.

## Arguments

`$ARGUMENTS` is the node ID, e.g., `myapp:graph/handler-go::serve`.

If empty or malformed: ask the user to provide a node ID. Suggest: `kg node list --domain <some-domain> --limit 20` to discover candidates.

## Workflow

1. **Fetch the node with merged properties:**
   ```bash
   kg node get "<node-id>" --merged
   ```
   On `NODE_NOT_FOUND`: tell the user and suggest `kg node list` to find similar IDs.

2. **Fetch outgoing edges (and their targets' merged properties):**
   ```bash
   kg edge list-from "<node-id>"
   for target in $(kg edge list-from "<node-id>" | jq -r '.data[].target_id'); do
     kg node get "$target" --merged
   done
   ```

3. **Fetch incoming edges (and their sources' merged properties):**
   Same as above with `kg edge list-to`.

4. **Synthesize the answer** in 3-6 paragraphs:
   - **What it does:** one paragraph based on the node's own `summary` + signature.
   - **How it fits in:** one paragraph describing the 1-hop neighborhood (who calls it, who it calls, what it implements/extends).
   - **What to read next:** 2-4 bullet links to neighbor node IDs, ordered by relevance (highest = direct dependencies for understanding).
   - **Tour position (optional):** if any `myapp-tours:` step has a `teaches` edge to this node, mention which step covers it.

## Output format

Print as markdown to the user. Use code blocks for IDs. Don't pipe to a file — the user is asking a question, not generating documentation.

## Edge cases

- **Node has no enrichment yet** (only tree-sitter data): say so. Suggest `/kg-enrich --domain <domain>` first.
- **Node is in an unexpected domain** (e.g., `<orig>-arch:api-layer`): explain it's an architectural layer node, list the files it contains.
- **Node has no neighbors:** still explain based on properties alone. Don't fabricate connections.

## Non-goals

- Don't mutate the graph.
- Don't dispatch agents.
- Don't read source files unless the user explicitly asks (the enriched summaries are the answer).
