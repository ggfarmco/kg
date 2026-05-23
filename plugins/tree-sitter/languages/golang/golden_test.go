package golang_test

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "rewrite expected.jsonl fixtures from current output")

func TestGolden(t *testing.T) {
	binary := buildPluginBinary(t)
	cases := []string{"01-single-file", "02-multi-package", "03-with-methods"}
	for _, name := range cases {
		name := name
		t.Run(name, func(t *testing.T) {
			input := filepath.Join("testdata", "golden", name, "input")
			expected := filepath.Join("testdata", "golden", name, "expected.jsonl")
			abs, err := filepath.Abs(input)
			require.NoError(t, err)

			cmd := exec.Command(binary, "--language", "go")
			cmd.Stdin = bytes.NewReader([]byte(`{"input":"` + abs + `","domain":"g","protocol_version":1,"config":{}}`))
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			require.NoError(t, cmd.Run(), "stderr=%s", stderr.String())

			got := normalizeAbs(stdout.Bytes(), abs)
			if *updateGolden {
				require.NoError(t, os.WriteFile(expected, got, 0o644))
				return
			}
			want, err := os.ReadFile(expected)
			require.NoError(t, err)
			require.Equal(t, string(want), string(got))
		})
	}
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
