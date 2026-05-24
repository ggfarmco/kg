package snapshot_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/snapshot"
)

func TestDecodeRejectsJSONL(t *testing.T) {
	jsonl := `{"protocol_version":2,"source":"a"}` + "\n" + `{"protocol_version":2,"source":"b"}` + "\n"
	_, err := snapshot.Decode(strings.NewReader(jsonl))
	require.Error(t, err)
	require.Contains(t, err.Error(), "trailing data")
}

func TestDecodeAcceptsWhitespacePadded(t *testing.T) {
	body := "\n  " + `{"protocol_version":2,"source":"a","domain":"d","scope":"domain-source","nodes":[],"edges":[]}` + "\n\n"
	s, err := snapshot.Decode(strings.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, 2, s.ProtocolVersion)
}

func TestEncodeRoundTrip(t *testing.T) {
	in := snapshot.Snapshot{ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeAdditive, Nodes: []snapshot.NodeSpec{}, Edges: []snapshot.EdgeSpec{}}
	var buf bytes.Buffer
	require.NoError(t, snapshot.Encode(&buf, in))
	out, err := snapshot.Decode(&buf)
	require.NoError(t, err)
	require.Equal(t, in, *out)
}
