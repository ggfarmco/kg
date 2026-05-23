//go:build e2e

package e2e

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractSelf(t *testing.T) {
	kgBin := mustBuild(t, "kg", "../cmd/kg")
	extractorBin := mustBuild(t, "kg-extractor", "../cmd/kg-extractor")
	pluginBin := mustBuildPlugin(t)

	pluginsDir := t.TempDir()
	pluginHome := filepath.Join(pluginsDir, "tree-sitter")
	writeFile(t, filepath.Join(pluginHome, "manifest.json"), `{
		"name": "tree-sitter",
		"version": "0.1.0",
		"description": "tree-sitter (Go)",
		"runtime": "native",
		"executable": "kg-extractor-tree-sitter"
	}`)
	require.NoError(t, exec.Command("cp", pluginBin, filepath.Join(pluginHome, "kg-extractor-tree-sitter")).Run())

	dbPath := filepath.Join(t.TempDir(), "selfg.db")
	require.NoError(t, exec.Command(kgBin, "--db", dbPath, "init").Run())

	abs, err := filepath.Abs("../internal/graph")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(extractorBin,
		"--plugins-path", pluginsDir,
		"extract",
		"--plugin", "tree-sitter",
		"--language", "go",
		"--input", abs,
		"--domain", "selfg",
		"--db", dbPath,
		"--kg-binary", kgBin,
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), "stderr=%s", stderr.String())

	dom, err := exec.Command(kgBin, "--db", dbPath, "domain", "get", "selfg").Output()
	require.NoError(t, err)
	require.Contains(t, string(dom), `"package"`)
	require.Contains(t, string(dom), `"file"`)
	require.Contains(t, string(dom), `"decl"`)

	pkgs, err := exec.Command(kgBin, "--db", dbPath, "node", "list", "--domain", "selfg", "--layer", "package").Output()
	require.NoError(t, err)
	require.Contains(t, string(pkgs), "selfg:graph")

	decls, err := exec.Command(kgBin, "--db", dbPath, "node", "list", "--domain", "selfg", "--layer", "decl").Output()
	require.NoError(t, err)
	require.Contains(t, string(decls), "::parseslug")

	edges, err := exec.Command(kgBin, "--db", dbPath, "edge", "list-from", "selfg:testutil").Output()
	require.NoError(t, err)
	require.Contains(t, string(edges), `"imports"`)
	require.Contains(t, string(edges), `"selfg:graph"`)
}
