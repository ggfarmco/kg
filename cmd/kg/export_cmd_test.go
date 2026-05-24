package main

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func runOKBytes(t *testing.T, dbPath string, args ...string) []byte {
	t.Helper()
	var out, errb bytes.Buffer
	exit := run(append([]string{"--db", dbPath}, args...), &out, &errb)
	require.Equal(t, 0, exit, "stderr=%s stdout=%s", errb.String(), out.String())
	return out.Bytes()
}

func runOKWithStdin(t *testing.T, dbPath string, stdin io.Reader, args ...string) []byte {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old; _ = r.Close() })
	go func() { _, _ = io.Copy(w, stdin); _ = w.Close() }()
	var out, errb bytes.Buffer
	exit := run(append([]string{"--db", dbPath}, args...), &out, &errb)
	require.Equal(t, 0, exit, "stderr=%s stdout=%s", errb.String(), out.String())
	return out.Bytes()
}

func TestExportRoundTrip(t *testing.T) {
	dbPath := freshDB(t)
	runOKBytes(t, dbPath, "domain", "add", "d", "--layers", "pkg,file", "--description", "demo")
	runOKBytes(t, dbPath, "node", "add", "--domain", "d", "--layer", "pkg", "--name", "a", "--id", "a")
	runOKBytes(t, dbPath, "node", "add", "--domain", "d", "--layer", "file", "--name", "x", "--id", "a/x", "--parent", "d:a")
	runOKBytes(t, dbPath, "edge", "add", "d:a", "d:a/x", "--type", "contains")

	exported := runOKBytes(t, dbPath, "export", "--domain", "d", "--source", "cli")
	require.Contains(t, string(exported), `"protocol_version": 2`)
	require.Contains(t, string(exported), `"d:a/x"`)

	res := runOKWithStdin(t, dbPath, bytes.NewReader(exported), "apply", "--source", "cli", "--domain", "d")
	require.Contains(t, string(res), `"nodes_added": 0`, "round-trip is a no-op")
	require.Contains(t, string(res), `"nodes_updated": 0`)
	require.Contains(t, string(res), `"nodes_removed": 0`)
}

func TestExportEmptySource(t *testing.T) {
	dbPath := freshDB(t)
	runOKBytes(t, dbPath, "domain", "add", "d", "--layers", "pkg", "--description", "")

	out := runOKBytes(t, dbPath, "export", "--domain", "d", "--source", "kg-summary:0.1.0")
	s := string(out)
	require.Contains(t, s, `"source": "kg-summary:0.1.0"`)
	require.Contains(t, s, `"nodes": []`)
	require.Contains(t, s, `"edges": []`)
}

func TestExportFiltersCrossDomainEdges(t *testing.T) {
	dbPath := freshDB(t)
	runOKBytes(t, dbPath, "domain", "add", "a", "--layers", "pkg,file", "--description", "")
	runOKBytes(t, dbPath, "domain", "add", "b", "--layers", "pkg,file", "--description", "")
	runOKBytes(t, dbPath, "node", "add", "--domain", "a", "--layer", "pkg", "--id", "p", "--name", "p")
	runOKBytes(t, dbPath, "node", "add", "--domain", "a", "--layer", "file", "--id", "p/x", "--parent", "a:p", "--name", "x")
	runOKBytes(t, dbPath, "node", "add", "--domain", "b", "--layer", "pkg", "--id", "p", "--name", "p")
	runOKBytes(t, dbPath, "node", "add", "--domain", "b", "--layer", "file", "--id", "p/x", "--parent", "b:p", "--name", "x")
	runOKBytes(t, dbPath, "edge", "add", "a:p/x", "a:p", "--type", "contains")
	runOKBytes(t, dbPath, "edge", "add", "b:p/x", "b:p", "--type", "contains")

	out := runOKBytes(t, dbPath, "export", "--domain", "a", "--source", "cli")
	s := string(out)
	require.Contains(t, s, `"a:p/x"`, "domain a's node included")
	require.NotContains(t, s, `"b:p/x"`, "domain b's edge endpoint must be filtered out")
}
