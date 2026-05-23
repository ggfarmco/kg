package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNodeWalkthrough(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kg.db")

	code, body := runCLI(dbPath, "init")
	require.Equal(t, 0, code, body)
	code, body = runCLI(dbPath, "domain", "add", "cars", "--layers", "system,subsystem,part")
	require.Equal(t, 0, code, body)

	code, body = runCLI(dbPath, "node", "add", "--domain", "cars", "--layer", "system", "--name", "Powertrain")
	require.Equal(t, 0, code, body)
	require.Contains(t, body, `"cars:powertrain"`)

	code, body = runCLI(dbPath, "node", "add", "--domain", "cars", "--layer", "subsystem", "--name", "Engine", "--parent", "cars:powertrain")
	require.Equal(t, 0, code, body)

	code, body = runCLI(dbPath, "node", "children", "cars:powertrain")
	require.Equal(t, 0, code, body)
	var env envelope
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	require.True(t, env.OK)
	kids := env.Data.([]any)
	require.Len(t, kids, 1)

	code, body = runCLI(dbPath, "node", "update", "cars:powertrain", "--summary", "the drive train")
	require.Equal(t, 0, code, body)
	require.Contains(t, body, "the drive train")

	code, body = runCLI(dbPath, "node", "delete", "cars:powertrain")
	require.Equal(t, 1, code, body)
	require.Contains(t, body, "HAS_DEPENDENTS")
}

func TestNodeAddIfNotExistsSkips(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kg.db")
	c1, _ := runCLI(dbPath, "init")
	c2, _ := runCLI(dbPath, "domain", "add", "cars", "--layers", "system")
	c3, _ := runCLI(dbPath, "node", "add", "--domain", "cars", "--layer", "system", "--name", "PT")
	require.Equal(t, 0, c1)
	require.Equal(t, 0, c2)
	require.Equal(t, 0, c3)

	code, body := runCLI(dbPath, "node", "add", "--domain", "cars", "--layer", "system", "--name", "PT", "--if-not-exists")
	require.Equal(t, 0, code, body)
	require.Contains(t, body, `"skipped": true`)
}
