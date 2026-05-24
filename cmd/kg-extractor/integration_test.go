package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractDeclarativePipesToKgApply(t *testing.T) {
	mustBash(t)

	pluginsDir := t.TempDir()
	pluginHome := filepath.Join(pluginsDir, "snap-demo")
	require.NoError(t, os.MkdirAll(pluginHome, 0o755))
	require.NoError(t, writeFile(filepath.Join(pluginHome, "manifest.json"), `{
  "name":"snap-demo","version":"0.1.0","description":"declarative bash demo",
  "runtime":"declarative-command","command":["bash","extract.sh"],
  "source_id":"snap-demo:0.1.0"
}`))
	require.NoError(t, writeFile(filepath.Join(pluginHome, "extract.sh"), `#!/usr/bin/env bash
set -euo pipefail
cat <<'EOF'
{
  "protocol_version": 2,
  "source": "snap-demo:0.1.0",
  "domain": "snap",
  "scope": "domain-source",
  "domain_spec": {"id":"snap","layers":["l1"]},
  "nodes": [{"id":"snap:a","layer":"l1","name":"a"}],
  "edges": []
}
EOF
`))
	require.NoError(t, os.Chmod(filepath.Join(pluginHome, "extract.sh"), 0o755))

	kgBin := buildKg(t)
	dbPath := filepath.Join(t.TempDir(), "snap.db")
	require.NoError(t, exec.Command(kgBin, "--db", dbPath, "init").Run())

	var out, errOut bytes.Buffer
	exit := run([]string{
		"--plugins-path", pluginsDir, "extract",
		"--plugin", "snap-demo", "--domain", "snap",
		"--db", dbPath, "--kg-binary", kgBin,
	}, &out, &errOut)
	require.Equal(t, 0, exit, errOut.String())

	listOut, err := exec.Command(kgBin, "--db", dbPath, "node", "list", "--domain", "snap").Output()
	require.NoError(t, err)
	require.Contains(t, string(listOut), `"snap:a"`)
}

func TestBashDemoEndToEnd(t *testing.T) {
	t.Skip("bash-demo emits v1 edge.add wire format; migration pending Phase 7 (Task 27)")
	mustBash(t)
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}

	kgPath := buildKg(t)
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "kg.db")
	require.NoError(t, exec.Command(kgPath, "--db", dbPath, "init").Run())

	pluginsRoot := t.TempDir()
	require.NoError(t, exec.Command("cp", "-R", "../../examples/kg-extractor-plugins/bash-demo", filepath.Join(pluginsRoot, "bash-demo")).Run())

	var stdout, stderr bytes.Buffer
	exit := run([]string{
		"--plugins-path", pluginsRoot, "extract",
		"--plugin", "bash-demo", "--input", "/x", "--domain", "demoapp",
		"--db", dbPath, "--kg-binary", kgPath,
	}, &stdout, &stderr)
	require.Equal(t, 0, exit, "stdout=%s stderr=%s", stdout.String(), stderr.String())

	nodeList, err := exec.Command(kgPath, "--db", dbPath, "node", "list", "--domain", "demoapp").Output()
	require.NoError(t, err)
	require.Contains(t, string(nodeList), "demoapp:demo")
	require.Contains(t, string(nodeList), "demoapp:demo-first")
	require.Contains(t, string(nodeList), "demoapp:demo-second")

	edges, err := exec.Command(kgPath, "--db", dbPath, "edge", "list-from", "demoapp:demo-first").Output()
	require.NoError(t, err)
	require.Contains(t, string(edges), "demoapp:demo-second")
	require.Contains(t, string(edges), "references")
}
