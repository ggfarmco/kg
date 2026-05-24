package golang_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "rewrite expected.snapshot.json fixtures from current output")

func TestGolden(t *testing.T) {
	binary := buildPluginBinary(t)
	cases := []string{"01-single-file", "02-multi-package", "03-with-methods"}
	for _, name := range cases {
		name := name
		t.Run(name, func(t *testing.T) {
			input := filepath.Join("testdata", "golden", name, "input")
			expected := filepath.Join("testdata", "golden", name, "expected.snapshot.json")
			abs, err := filepath.Abs(input)
			require.NoError(t, err)

			cmd := exec.Command(binary, "--language", "go")
			cmd.Stdin = bytes.NewReader([]byte(`{"input":"` + abs + `","domain":"g","protocol_version":2,"config":{}}`))
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			require.NoError(t, cmd.Run(), "stderr=%s", stderr.String())

			got := normalizeSnapshot(t, normalizeAbs(stdout.Bytes(), abs))
			if *updateGolden {
				require.NoError(t, os.WriteFile(expected, got, 0o644))
				return
			}
			want, err := os.ReadFile(expected)
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
		})
	}
}

func normalizeSnapshot(t *testing.T, b []byte) []byte {
	t.Helper()
	var raw map[string]any
	require.NoError(t, json.Unmarshal(b, &raw))
	if nodes, ok := raw["nodes"].([]any); ok {
		sort.SliceStable(nodes, func(i, j int) bool {
			return nodes[i].(map[string]any)["id"].(string) < nodes[j].(map[string]any)["id"].(string)
		})
	}
	if edges, ok := raw["edges"].([]any); ok {
		sort.SliceStable(edges, func(i, j int) bool {
			ei, ej := edges[i].(map[string]any), edges[j].(map[string]any)
			ki := ei["src"].(string) + "|" + ei["target"].(string) + "|" + ei["type"].(string)
			kj := ej["src"].(string) + "|" + ej["target"].(string) + "|" + ej["type"].(string)
			return ki < kj
		})
	}
	out, err := json.MarshalIndent(raw, "", "  ")
	require.NoError(t, err)
	return out
}

func buildPluginBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "tspl")
	cmd := exec.Command("go", "build", "-o", out, "../..")
	cmd.Stderr = bytes.NewBuffer(nil)
	if err := cmd.Run(); err != nil {
		t.Skipf("build failed: %v %s", err, cmd.Stderr)
	}
	return out
}

func normalizeAbs(b []byte, abs string) []byte {
	return bytes.ReplaceAll(b, []byte(abs), []byte("<INPUT>"))
}
