package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootHelpExitsZero(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--help"}, &out, &errOut)
	require.Equal(t, 0, code)
	require.Contains(t, out.String()+errOut.String(), "kg")
}

func TestUnknownCommandReturnsEnvelope(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"frob"}, &out, &errOut)
	require.NotEqual(t, 0, code)

	var env envelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.False(t, env.OK)
	require.NotNil(t, env.Error)
}
