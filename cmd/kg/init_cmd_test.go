package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitSeedsBuiltinSources(t *testing.T) {
	db := freshDB(t)
	var out, errOut bytes.Buffer
	require.Equal(t, 0, run([]string{"--db", db, "sources", "list"}, &out, &errOut), errOut.String())
	var env struct {
		Data []struct{ ID string } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	ids := map[string]bool{}
	for _, s := range env.Data {
		ids[s.ID] = true
	}
	require.True(t, ids["cli"], "cli source must be seeded by init")
	require.True(t, ids["manual"], "manual source must be seeded by init")
}
