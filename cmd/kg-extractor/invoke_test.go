package main

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustBash(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
}

func TestInvokeCapturesStdoutOps(t *testing.T) {
	mustBash(t)
	dir := t.TempDir()
	script := `#!/usr/bin/env bash
set -e
cat > /dev/null
echo '{"op":"meta","args":{"plugin":"t"}}'
echo '{"op":"domain.add","args":{"id":"a","layers":["l1"]}}'
`
	scriptPath := filepath.Join(dir, "extract.sh")
	require.NoError(t, writeFile(scriptPath, script))
	require.NoError(t, exec.Command("chmod", "+x", scriptPath).Run())

	m := &manifest{Name: "t", Runtime: runtimeCommand, Command: []string{"bash", scriptPath}}
	cfg := pluginConfig{Input: "/x", Domain: "a", ProtocolVersion: 1}

	var stderr bytes.Buffer
	stream, err := invokePlugin(context.Background(), discoveredPlugin{Dir: dir, Manifest: m}, cfg, &stderr)
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(stream.String(), "\n"))
	require.Contains(t, stream.String(), "domain.add")
}

func TestInvokePropagatesNonZeroExit(t *testing.T) {
	mustBash(t)
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "extract.sh")
	require.NoError(t, writeFile(scriptPath, `#!/usr/bin/env bash
cat > /dev/null
echo "bad" >&2
exit 7
`))
	require.NoError(t, exec.Command("chmod", "+x", scriptPath).Run())

	m := &manifest{Name: "t", Runtime: runtimeCommand, Command: []string{"bash", scriptPath}}
	var stderr bytes.Buffer
	_, err := invokePlugin(context.Background(), discoveredPlugin{Dir: dir, Manifest: m}, pluginConfig{ProtocolVersion: 1}, &stderr)
	require.Error(t, err)
	require.Contains(t, stderr.String(), "bad")
}

func TestInvokeRejectsWASM(t *testing.T) {
	m := &manifest{Name: "w", Runtime: runtimeWASM, Module: "x.wasm"}
	_, err := invokePlugin(context.Background(), discoveredPlugin{Manifest: m}, pluginConfig{ProtocolVersion: 1}, &bytes.Buffer{})
	require.ErrorContains(t, err, "WASM_NOT_SUPPORTED")
}
