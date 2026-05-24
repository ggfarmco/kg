package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseManifestNative(t *testing.T) {
	path := writeManifest(t, `{
		"name": "tree-sitter",
		"version": "0.1.0",
		"description": "...",
		"runtime": "native",
		"executable": "kg-extractor-tree-sitter"
	}`)
	m, err := parseManifest(path)
	require.NoError(t, err)
	require.Equal(t, "tree-sitter", m.Name)
	require.Equal(t, runtimeNative, m.Runtime)
	require.Equal(t, "kg-extractor-tree-sitter", m.Executable)
}

func TestParseManifestCommand(t *testing.T) {
	path := writeManifest(t, `{
		"name": "bash-demo",
		"version": "0.1.0",
		"description": "...",
		"runtime": "command",
		"command": ["bash", "extract.sh"]
	}`)
	m, err := parseManifest(path)
	require.NoError(t, err)
	require.Equal(t, []string{"bash", "extract.sh"}, m.Command)
}

func TestParseManifestRejectsBadName(t *testing.T) {
	path := writeManifest(t, `{"name":"Bad Name","version":"0.1.0","runtime":"native","executable":"x","description":"x"}`)
	_, err := parseManifest(path)
	require.ErrorContains(t, err, "name")
}

func TestParseManifestRejectsUnknownRuntime(t *testing.T) {
	path := writeManifest(t, `{"name":"x","version":"0.1.0","runtime":"docker","description":"x"}`)
	_, err := parseManifest(path)
	require.ErrorContains(t, err, "runtime")
}

func TestParseManifestWASMReserved(t *testing.T) {
	path := writeManifest(t, `{"name":"x","version":"0.1.0","runtime":"wasm","module":"x.wasm","description":"x"}`)
	m, err := parseManifest(path)
	require.NoError(t, err)
	require.Equal(t, runtimeWASM, m.Runtime)
}

func TestManifestParsesV2Fields(t *testing.T) {
	path := writeManifest(t, `{
	  "name":"foo","version":"1.0","description":"x",
	  "runtime":"declarative-native","executable":"foo",
	  "source_id":"acme/foo:1.0","trust":80,
	  "supported_scopes":["domain-source","domain"]
	}`)
	m, err := parseManifest(path)
	require.NoError(t, err)
	require.Equal(t, runtimeDeclarativeNative, m.Runtime)
	require.Equal(t, "acme/foo:1.0", m.SourceID)
	require.Equal(t, 80, m.Trust)
	require.Equal(t, []string{"domain-source", "domain"}, m.SupportedScopes)
}

func TestManifestSourceIDDefaultsToNameVersion(t *testing.T) {
	path := writeManifest(t, `{"name":"foo","version":"1.0","description":"x","runtime":"native","executable":"foo"}`)
	m, err := parseManifest(path)
	require.NoError(t, err)
	require.Equal(t, "foo:1.0", m.SourceID)
	require.Equal(t, 100, m.Trust)
}

func TestManifestRejectsUnknownRuntime(t *testing.T) {
	path := writeManifest(t, `{"name":"foo","version":"1.0","description":"x","runtime":"quantum"}`)
	_, err := parseManifest(path)
	require.Error(t, err)
}
