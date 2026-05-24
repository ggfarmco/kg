package snapshot_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/snapshot"
)

func TestSnapshotMarshalRoundTrip(t *testing.T) {
	in := snapshot.Snapshot{
		ProtocolVersion: 2, Source: "tree-sitter:0.1.0",
		Domain: "myapp", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{
			ID: "myapp", Layers: []string{"package", "file", "decl"},
			Description: "x",
		},
		Nodes: []snapshot.NodeSpec{
			{ID: "myapp:graph", Layer: "package", Name: "graph",
				Properties: map[string]any{"import_path": "x"}},
			{ID: "myapp:graph/node-go", Layer: "file",
				Parent: "myapp:graph", Name: "node.go"},
		},
		Edges: []snapshot.EdgeSpec{
			{Src: "myapp:graph", Target: "myapp:store", Type: "imports"},
		},
	}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	var out snapshot.Snapshot
	require.NoError(t, json.Unmarshal(b, &out))
	require.Equal(t, in, out)
}

func TestEdgeSpecAcceptsBothSourceAndSrc(t *testing.T) {
	body := []byte(`{"source":"a:n","target":"a:m","type":"x"}`)
	var got snapshot.EdgeSpec
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "a:n", got.Src, "wire field `source` aliases to Go field Src")
}
