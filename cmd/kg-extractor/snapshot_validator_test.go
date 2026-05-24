package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSnapshotHappy(t *testing.T) {
	in := strings.NewReader(`{
	  "protocol_version":2,"source":"foo:1.0","domain":"d","scope":"domain-source",
	  "nodes":[{"id":"d:n","layer":"l","name":"n"}],"edges":[]
	}`)
	var out bytes.Buffer
	require.NoError(t, validateSnapshot(in, &out, "foo:1.0"))
	require.Contains(t, out.String(), `"source":"foo:1.0"`)
}

func TestValidateSnapshotRejectsSourceMismatch(t *testing.T) {
	in := strings.NewReader(`{"protocol_version":2,"source":"foo:1.0","domain":"d","scope":"domain-source","nodes":[],"edges":[]}`)
	var out bytes.Buffer
	err := validateSnapshot(in, &out, "bar:2.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "SOURCE_MISMATCH")
}

func TestValidateSnapshotRejectsJSONL(t *testing.T) {
	in := strings.NewReader(`{"protocol_version":2}` + "\n" + `{"protocol_version":2}` + "\n")
	var out bytes.Buffer
	err := validateSnapshot(in, &out, "x:1")
	require.Error(t, err)
}

func TestValidateSnapshotRejectsBadShape(t *testing.T) {
	in := strings.NewReader(`{"protocol_version":1,"source":"x","domain":"d","scope":"domain-source","nodes":[],"edges":[]}`)
	var out bytes.Buffer
	err := validateSnapshot(in, &out, "x")
	require.Error(t, err)
}
