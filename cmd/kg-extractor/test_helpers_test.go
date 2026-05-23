package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "manifest.json")
	require.NoError(t, writeFile(p, body))
	return p
}

func mkPlugin(t *testing.T, root, name, manifest string) {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644))
}
