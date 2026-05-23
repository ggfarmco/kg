package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiscoverFindsValidPluginsAndReportsBad(t *testing.T) {
	root := t.TempDir()
	mkPlugin(t, root, "good", `{"name":"good","version":"0.1.0","description":"x","runtime":"command","command":["echo"]}`)
	mkPlugin(t, root, "bad", `{"name":"BAD","version":"0.1.0","description":"x","runtime":"native","executable":"x"}`)
	mkPlugin(t, root, "another", `{"name":"another","version":"0.1.0","description":"x","runtime":"native","executable":"a"}`)

	plugins, errs := discoverPlugins(root)
	require.ElementsMatch(t, []string{"good", "another"}, pluginNames(plugins))
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Error(), "bad")
}

func TestDiscoverMultipleDirs(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	mkPlugin(t, a, "x", `{"name":"x","version":"0.1.0","description":"x","runtime":"native","executable":"x"}`)
	mkPlugin(t, b, "y", `{"name":"y","version":"0.1.0","description":"x","runtime":"native","executable":"y"}`)

	plugins, _ := discoverPlugins(a + string(os.PathListSeparator) + b)
	require.ElementsMatch(t, []string{"x", "y"}, pluginNames(plugins))
}

func TestDiscoverIgnoresMissingDirs(t *testing.T) {
	plugins, errs := discoverPlugins(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Empty(t, plugins)
	require.Empty(t, errs)
}

func pluginNames(ps []discoveredPlugin) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Manifest.Name)
	}
	return out
}
