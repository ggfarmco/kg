//go:build e2e

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustBuild(t *testing.T, outBin, pkg string) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, outBin)
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Stderr = bytes.NewBuffer(nil)
	require.NoError(t, cmd.Run(), "build %s failed: %s", pkg, cmd.Stderr)
	return out
}

func mustBuildPlugin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "kg-extractor-tree-sitter")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = filepath.Join("..", "plugins", "tree-sitter")
	cmd.Stderr = bytes.NewBuffer(nil)
	require.NoError(t, cmd.Run(), "build plugin: %s", cmd.Stderr)
	return out
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
