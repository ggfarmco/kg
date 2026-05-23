package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEdgeDeleteInvalidIDExits1(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kg.db")
	code, _ := runCLI(dbPath, "init")
	require.Equal(t, 0, code)
	code, body := runCLI(dbPath, "edge", "delete", "not-a-number")
	require.Equal(t, 1, code, body)
	require.Contains(t, body, "INVALID_INPUT")
}

func TestEdgeWalkthrough(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kg.db")
	for _, args := range [][]string{
		{"init"},
		{"domain", "add", "cars", "--layers", "system"},
		{"node", "add", "--domain", "cars", "--layer", "system", "--name", "A"},
		{"node", "add", "--domain", "cars", "--layer", "system", "--name", "B"},
	} {
		code, body := runCLI(dbPath, args...)
		require.Equal(t, 0, code, body)
	}

	code, body := runCLI(dbPath, "edge", "add", "cars:a", "cars:b", "--type", "depends_on")
	require.Equal(t, 0, code, body)

	var env envelope
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	data := env.Data.(map[string]any)
	id := int64(data["id"].(float64))
	require.NotZero(t, id)

	code, body = runCLI(dbPath, "edge", "list-from", "cars:a")
	require.Equal(t, 0, code, body)

	code, body = runCLI(dbPath, "edge", "list-to", "cars:b", "--type", "depends_on")
	require.Equal(t, 0, code, body)

	code, body = runCLI(dbPath, "edge", "delete", fmt.Sprint(id))
	require.Equal(t, 0, code, body)
}
