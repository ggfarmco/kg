//go:build e2e_enrich

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnrichSelf(t *testing.T) {
	if os.Getenv("LLM_ENABLED") != "1" {
		t.Skip("LLM_ENABLED=1 not set; skipping enrich e2e (would cost real LLM tokens)")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not found on PATH; install Claude Code to enable")
	}

	dbPath := filepath.Join(t.TempDir(), "selfg.db")

	mustRun(t, "./bin/kg", "--db", dbPath, "init")
	mustRun(t, "./bin/kg-extractor", "extract",
		"--plugin", "tree-sitter", "--language", "go",
		"--input", "./internal/graph", "--domain", "selfg",
		"--db", dbPath, "--kg-binary", "./bin/kg")

	enrich := exec.Command("claude", "code", "run", "/kg-enrich", "--domain", "selfg")
	enrich.Env = append(os.Environ(), "KG_DB="+dbPath)
	enrich.Stdout = os.Stdout
	enrich.Stderr = os.Stderr
	require.NoError(t, enrich.Run(), "headless /kg-enrich invocation failed (verify the claude CLI form)")

	files := listLen(t, dbPath, "node", "list", "--domain", "selfg", "--layer", "file", "--source", "kg-summary:0.1.0", "--limit", "0")
	require.NotZero(t, files, "no files were enriched")

	arch := listLen(t, dbPath, "node", "list", "--domain", "selfg-arch", "--source", "kg-arch:0.1.0", "--limit", "0")
	require.GreaterOrEqual(t, arch, 3, "expected at least 3 arch layers")

	tours := listLen(t, dbPath, "node", "list", "--domain", "selfg-tours", "--source", "kg-tours:0.1.0", "--limit", "0")
	require.GreaterOrEqual(t, tours, 5, "expected at least 5 tour steps")
}

func mustRun(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "command failed: %s %v", name, args)
}

func listLen(t *testing.T, dbPath string, args ...string) int {
	t.Helper()
	cmd := exec.Command("./bin/kg", append([]string{"--db", dbPath}, args...)...)
	out, err := cmd.Output()
	require.NoError(t, err)
	var r struct {
		OK   bool          `json:"ok"`
		Data []interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out, &r))
	return len(r.Data)
}
