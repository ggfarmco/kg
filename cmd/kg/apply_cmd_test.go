package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func execApplyCmd(t *testing.T, dbPath, stdin string, extra ...string) (string, string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, _ = w.WriteString(stdin)
	require.NoError(t, w.Close())
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })

	var stdout, stderr bytes.Buffer
	args := append([]string{"--db", dbPath, "apply"}, extra...)
	exit := run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exit
}

func TestApplyHappyPath(t *testing.T) {
	db := freshDB(t)
	snap := `{
	  "protocol_version": 2, "source": "tree-sitter:0.1.0", "domain": "d", "scope": "domain-source",
	  "domain_spec": {"id": "d", "layers": ["package","file"]},
	  "nodes": [
	    {"id":"d:pkg","layer":"package","name":"pkg"},
	    {"id":"d:pkg/foo","layer":"file","parent":"d:pkg","name":"foo.go"}
	  ],
	  "edges": []
	}`
	out, errOut, exit := execApplyCmd(t, db, snap,
		"--source", "tree-sitter:0.1.0", "--domain", "d")
	require.Equal(t, 0, exit, errOut)
	var env struct {
		OK   bool `json:"ok"`
		Data struct{ NodesAdded int `json:"nodes_added"` } `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &env))
	require.True(t, env.OK)
	require.Equal(t, 2, env.Data.NodesAdded)
}

func TestApplyRejectsSnapshotSourceMismatch(t *testing.T) {
	db := freshDB(t)
	snap := `{"protocol_version":2,"source":"a","domain":"d","scope":"domain-source","nodes":[],"edges":[]}`
	out, _, exit := execApplyCmd(t, db, snap, "--source", "b", "--domain", "d")
	require.NotEqual(t, 0, exit)
	require.Contains(t, out, "SOURCE_MISMATCH")
}

func TestApplyDryRunRollsBack(t *testing.T) {
	db := freshDB(t)
	snap := `{
	  "protocol_version": 2, "source": "x", "domain": "d", "scope": "domain-source",
	  "domain_spec": {"id":"d","layers":["l1"]},
	  "nodes": [{"id":"d:a","layer":"l1","name":"a"}],
	  "edges": []
	}`
	out, _, exit := execApplyCmd(t, db, snap, "--source", "x", "--domain", "d", "--dry-run")
	require.Equal(t, 0, exit)
	require.Contains(t, out, `"dry_run": true`)

	var listOut bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "node", "list", "--domain", "d"}, &listOut, new(bytes.Buffer)))
	require.Contains(t, listOut.String(), `"data": []`)
}

func TestApplyForeignClaimsErrorsWithoutForce(t *testing.T) {
	db := freshDB(t)
	require.Equal(t, 0, run([]string{"--db", db, "sources", "register", "--id", "y", "--if-not-exists"}, new(bytes.Buffer), new(bytes.Buffer)))

	first := `{
	  "protocol_version":2,"source":"x","domain":"d","scope":"domain-source",
	  "domain_spec":{"id":"d","layers":["l1"]},
	  "nodes":[{"id":"d:a","layer":"l1","name":"a"},{"id":"d:b","layer":"l1","name":"b"}],
	  "edges":[{"src":"d:a","target":"d:b","type":"imports"}]
	}`
	_, _, exit := execApplyCmd(t, db, first, "--source", "x", "--domain", "d")
	require.Equal(t, 0, exit)
	require.Equal(t, 0, run([]string{"--db", db, "edge", "add", "d:a", "d:b", "--type", "imports", "--source", "y"}, new(bytes.Buffer), new(bytes.Buffer)))

	rm := strings.Replace(first, `,{"id":"d:b","layer":"l1","name":"b"}`, "", 1)
	rm = strings.Replace(rm, `[{"src":"d:a","target":"d:b","type":"imports"}]`, "[]", 1)
	out, _, exit := execApplyCmd(t, db, rm, "--source", "x", "--domain", "d")
	require.NotEqual(t, 0, exit)
	require.Contains(t, out, "NODE_HAS_FOREIGN_CLAIMS")
}
