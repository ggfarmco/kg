# bash-demo

A ~25-line bash plugin that emits a fixed mini-graph snapshot (declarative-command runtime).
Demonstrates the v2 plugin contract works without compiled code.

Requires `bash` and `jq`.

## Try it

```sh
ln -s "$(pwd)" ~/.config/kg-extractor/plugins/bash-demo
kg-extractor extract --plugin bash-demo --domain example | jq .
```

## What it produces

- a domain with layers `[root, item]`
- one root node `Demo`
- two item nodes `First`, `Second` parented at `Demo`
- one `references` edge from `First` to `Second`

Re-running is idempotent — kg apply diffs against the previous snapshot for
the `(domain, source)` pair.
