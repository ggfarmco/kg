package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func runCLI(dbPath string, args ...string) (int, string) {
	var out, errOut bytes.Buffer
	full := append([]string{"--db", dbPath}, args...)
	code := run(full, &out, &errOut)
	return code, out.String()
}

func TestDomainAddListGetDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kg.db")

	code, body := runCLI(dbPath, "init")
	require.Equal(t, 0, code, body)

	code, body = runCLI(dbPath, "domain", "add", "cars", "--layers", "system,subsystem,part")
	require.Equal(t, 0, code, body)
	var env envelope
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	require.True(t, env.OK)

	code, body = runCLI(dbPath, "domain", "add", "cars", "--layers", "system")
	require.Equal(t, 2, code, body)

	code, body = runCLI(dbPath, "domain", "add", "cars", "--layers", "system", "--if-not-exists")
	require.Equal(t, 0, code, body)
	require.Contains(t, body, `"skipped": true`)

	code, body = runCLI(dbPath, "domain", "get", "cars")
	require.Equal(t, 0, code, body)
	require.Contains(t, body, `"cars"`)

	code, body = runCLI(dbPath, "domain", "list")
	require.Equal(t, 0, code, body)

	code, body = runCLI(dbPath, "domain", "delete", "cars")
	require.Equal(t, 0, code, body)
}

func TestDomainAddDryRun(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kg.db")
	code, body := runCLI(dbPath, "init")
	require.Equal(t, 0, code, body)
	code, body = runCLI(dbPath, "domain", "add", "cars", "--layers", "system", "--dry-run")
	require.Equal(t, 0, code, body)
	require.Contains(t, body, `"dry_run": true`)

	code, body = runCLI(dbPath, "domain", "list")
	require.Equal(t, 0, code, body)
	require.NotContains(t, body, `"cars"`)
}
