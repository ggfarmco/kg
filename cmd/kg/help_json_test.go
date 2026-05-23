package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHelpJSONListsCommandTree(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--help", "--json"}, &out, &errOut)
	require.Equal(t, 0, code)

	var env envelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.True(t, env.OK)

	root := env.Data.(map[string]any)
	require.Equal(t, "kg", root["name"])
	cmds := root["commands"].([]any)
	names := []string{}
	for _, c := range cmds {
		names = append(names, c.(map[string]any)["name"].(string))
	}
	require.Contains(t, names, "domain")
	require.Contains(t, names, "node")
	require.Contains(t, names, "edge")
	require.Contains(t, names, "init")

	rootFlags := root["flags"].([]any)
	hasDB := false
	for _, f := range rootFlags {
		if f.(map[string]any)["name"] == "db" {
			hasDB = true
			break
		}
	}
	require.True(t, hasDB, "--db persistent flag should appear in root flags")
}
