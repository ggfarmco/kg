package main

import (
	"bytes"
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
	code, body := runCLI(dbPath, "edge", "delete", "not-a-number", "--force")
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

	code, body = runCLI(dbPath, "edge", "delete", fmt.Sprint(id), "--force")
	require.Equal(t, 0, code, body)
}

func TestEdgeUnclaimGCs(t *testing.T) {
	db := freshDB(t)
	require.Equal(t, 0, run([]string{"--db", db, "domain", "add", "d", "--layers", "l1", "--source", "cli"}, new(bytes.Buffer), new(bytes.Buffer)))
	require.Equal(t, 0, run([]string{"--db", db, "node", "add", "--domain", "d", "--layer", "l1", "--name", "a", "--source", "cli"}, new(bytes.Buffer), new(bytes.Buffer)))
	require.Equal(t, 0, run([]string{"--db", db, "node", "add", "--domain", "d", "--layer", "l1", "--name", "b", "--source", "cli"}, new(bytes.Buffer), new(bytes.Buffer)))
	var addOut bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "edge", "add", "d:a", "d:b", "--type", "imports", "--source", "cli"}, &addOut, new(bytes.Buffer)))
	var added struct {
		Data struct{ ID int64 `json:"id"` } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(addOut.Bytes(), &added))
	require.NotZero(t, added.Data.ID)

	require.Equal(t, 0, run([]string{"--db", db, "edge", "unclaim", fmt.Sprint(added.Data.ID), "--source", "cli"}, new(bytes.Buffer), new(bytes.Buffer)))

	var listOut bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "edge", "list-from", "d:a"}, &listOut, new(bytes.Buffer)))
	require.Contains(t, listOut.String(), `"data": []`)
}
