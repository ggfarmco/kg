# bash-demo

A 10-line bash plugin that emits a fixed mini-graph. Demonstrates that the
kg-extractor plugin contract works without compiled code or CGO.

Requires `bash` and `jq`.

## Try it

```sh
ln -s "$(pwd)" ~/.config/kg-extractor/plugins/bash-demo
kg-extractor extract --plugin bash-demo --domain example | head
```

## What it produces

- a domain with layers `[root, item]`
- one root node `Demo`
- two item nodes `First`, `Second` parented at `Demo`
- one `references` edge from `First` to `Second`
