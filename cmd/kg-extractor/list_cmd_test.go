package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListSubcommand(t *testing.T) {
	root := t.TempDir()
	mkPlugin(t, root, "alpha", `{"name":"alpha","version":"0.1.0","description":"first","runtime":"native","executable":"a"}`)
	mkPlugin(t, root, "beta", `{"name":"beta","version":"0.2.0","description":"second","runtime":"command","command":["bash","x.sh"]}`)

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--plugins-path", root, "list"}, &stdout, &stderr)
	require.Equal(t, 0, exit, "stderr=%s", stderr.String())

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Plugins []struct {
				Name        string `json:"name"`
				Version     string `json:"version"`
				Runtime     string `json:"runtime"`
				Description string `json:"description"`
			} `json:"plugins"`
			Errors []string `json:"errors"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &env))
	require.True(t, env.OK)
	require.Len(t, env.Data.Plugins, 2)
}

func TestInfoSubcommandFindsPlugin(t *testing.T) {
	root := t.TempDir()
	mkPlugin(t, root, "alpha", `{"name":"alpha","version":"0.1.0","description":"first","runtime":"native","executable":"a"}`)

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--plugins-path", root, "info", "alpha"}, &stdout, &stderr)
	require.Equal(t, 0, exit, "stderr=%s", stderr.String())
	require.Contains(t, stdout.String(), `"name": "alpha"`)
}

func TestInfoSubcommandMissingPluginErrors(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	exit := run([]string{"--plugins-path", root, "info", "ghost"}, &stdout, &stderr)
	require.NotEqual(t, 0, exit)
	require.Contains(t, stdout.String(), "PLUGIN_NOT_FOUND")
}
