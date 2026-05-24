package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourcesRegisterListShow(t *testing.T) {
	db := freshDB(t)

	{
		var out, errOut bytes.Buffer
		exit := run([]string{"--db", db, "sources", "register",
			"--id", "tree-sitter:0.1.0", "--description", "ts"}, &out, &errOut)
		require.Equal(t, 0, exit, errOut.String())
	}

	{
		var out, errOut bytes.Buffer
		exit := run([]string{"--db", db, "sources", "list"}, &out, &errOut)
		require.Equal(t, 0, exit, errOut.String())
		var env struct {
			Data []struct{ ID, Description string } `json:"data"`
		}
		require.NoError(t, json.Unmarshal(out.Bytes(), &env))
		ids := make([]string, 0, len(env.Data))
		for _, s := range env.Data {
			ids = append(ids, s.ID)
		}
		require.Contains(t, ids, "cli")
		require.Contains(t, ids, "manual")
		require.Contains(t, ids, "tree-sitter:0.1.0")
	}

	{
		var out, errOut bytes.Buffer
		exit := run([]string{"--db", db, "sources", "show", "tree-sitter:0.1.0"}, &out, &errOut)
		require.Equal(t, 0, exit, errOut.String())
		require.Contains(t, out.String(), `"description": "ts"`)
	}
}

func TestSourcesRegisterIfNotExistsSkipsDuplicate(t *testing.T) {
	db := freshDB(t)
	args := []string{"--db", db, "sources", "register", "--id", "x", "--if-not-exists"}
	var out, errOut bytes.Buffer
	require.Equal(t, 0, run(args, &out, &errOut), errOut.String())
	out.Reset()
	require.Equal(t, 0, run(args, &out, &errOut), errOut.String())
	require.Contains(t, out.String(), `"skipped": true`)
}

func TestSourcesUpdateChangesDescription(t *testing.T) {
	db := freshDB(t)
	require.Equal(t, 0, run([]string{"--db", db, "sources", "register", "--id", "x"}, new(bytes.Buffer), new(bytes.Buffer)))
	require.Equal(t, 0, run([]string{"--db", db, "sources", "update", "x", "--description", "Updated"}, new(bytes.Buffer), new(bytes.Buffer)))

	var out bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "sources", "show", "x"}, &out, new(bytes.Buffer)))
	require.Contains(t, out.String(), `"description": "Updated"`)
}

func TestSourcesDeleteFailsWithDependents(t *testing.T) {
	db := freshDB(t)
	require.Equal(t, 0, run([]string{"--db", db, "domain", "add", "d", "--layers", "l1", "--source", "owner"}, new(bytes.Buffer), new(bytes.Buffer)))
	require.Equal(t, 0, run([]string{"--db", db, "node", "add", "--domain", "d", "--layer", "l1", "--name", "n", "--source", "owner"}, new(bytes.Buffer), new(bytes.Buffer)))

	var out bytes.Buffer
	exit := run([]string{"--db", db, "sources", "delete", "owner"}, &out, new(bytes.Buffer))
	require.NotEqual(t, 0, exit)
	require.True(t, strings.Contains(out.String(), "SOURCE_HAS_DEPENDENTS"), "got: %s", out.String())
}
