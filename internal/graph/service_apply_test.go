package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/snapshot"
)

func TestApplyHappyPathAddsNodes(t *testing.T) {
	svc, _ := newService(t)
	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"package", "file"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "package", Name: "a"},
			{ID: "d:a/b", Layer: "file", Parent: "d:a", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, res.NodesAdded)
	require.Equal(t, 0, res.NodesUpdated)
	require.Equal(t, 0, res.NodesRemoved)
}

func TestApplyReApplyIsNoOp(t *testing.T) {
	svc, _ := newService(t)
	snap := snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"package"}},
		Nodes:      []snapshot.NodeSpec{{ID: "d:a", Layer: "package", Name: "a"}},
		Edges:      []snapshot.EdgeSpec{},
	}
	_, err := svc.Apply(t.Context(), snap, graph.ApplyOptions{})
	require.NoError(t, err)
	res, err := svc.Apply(t.Context(), snap, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, res.NodesAdded)
	require.Equal(t, 0, res.NodesUpdated)
	require.Equal(t, 0, res.NodesRemoved)
}

func TestApplyUpdatesChangedName(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"package"}},
		Nodes:      []snapshot.NodeSpec{{ID: "d:a", Layer: "package", Name: "old"}},
		Edges:      []snapshot.EdgeSpec{},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		Nodes: []snapshot.NodeSpec{{ID: "d:a", Layer: "package", Name: "new"}},
		Edges: []snapshot.EdgeSpec{},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.NodesUpdated)

	n, err := svc.GetNode(t.Context(), "d:a")
	require.NoError(t, err)
	require.Equal(t, "new", n.Name)
}

func TestApplyRemovesNodesNotInSnapshot(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"package"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "package", Name: "a"},
			{ID: "d:b", Layer: "package", Name: "b"},
		},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		Nodes: []snapshot.NodeSpec{{ID: "d:a", Layer: "package", Name: "a"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.NodesRemoved)
}
