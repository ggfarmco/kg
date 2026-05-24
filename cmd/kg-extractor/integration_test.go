package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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
