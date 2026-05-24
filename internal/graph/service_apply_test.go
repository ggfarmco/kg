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

func TestApplyAddsEdgesAndClaims(t *testing.T) {
	svc, fs := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{{Src: "d:a", Target: "d:b", Type: "imports"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	es, err := fs.EdgesFrom(t.Context(), "d:a", nil)
	require.NoError(t, err)
	require.Len(t, es, 1)
}

func TestApplyRemovesUnclaimedEdgesAndGCs(t *testing.T) {
	svc, fs := newService(t)
	base := snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{{Src: "d:a", Target: "d:b", Type: "imports"}},
	}
	_, err := svc.Apply(t.Context(), base, graph.ApplyOptions{})
	require.NoError(t, err)

	base.Edges = nil
	res, err := svc.Apply(t.Context(), base, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.ClaimsRemoved)
	require.Equal(t, 1, res.EdgesGC)
	es, _ := fs.EdgesFrom(t.Context(), "d:a", nil)
	require.Empty(t, es)
}

func TestApplyForeignClaimSurvivesUnclaim(t *testing.T) {
	svc, fs := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{{Src: "d:a", Target: "d:b", Type: "imports"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	_, err = svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: "d:a", Target: "d:b", Type: "imports", WriterSource: "y"})
	require.NoError(t, err)

	_, err = svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	es, _ := fs.EdgesFrom(t.Context(), "d:a", nil)
	require.Len(t, es, 1, "y's claim keeps the edge alive")
}

func TestApplyDomainScopeFailsWithForeignWriters(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes:      []snapshot.NodeSpec{{ID: "d:a", Layer: "l1", Name: "a"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	_, err = svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "y", Domain: "d", Scope: snapshot.ScopeDomain,
		Nodes: []snapshot.NodeSpec{{ID: "d:b", Layer: "l1", Name: "b"}},
	}, graph.ApplyOptions{})
	require.ErrorIs(t, err, graph.ErrDomainHasForeignWriters)
}

func TestApplyAdditiveScopeSkipsCleanup(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeAdditive,
		Nodes: []snapshot.NodeSpec{{ID: "d:a", Layer: "l1", Name: "a"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, res.NodesRemoved, "additive scope leaves d:b alone")
}

func TestApplyForceCascadeRemovesForeignClaims(t *testing.T) {
	svc, fs := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{{Src: "d:a", Target: "d:b", Type: "imports"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	_, err = svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: "d:a", Target: "d:b", Type: "imports", WriterSource: "y"})
	require.NoError(t, err)

	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		Nodes: []snapshot.NodeSpec{{ID: "d:b", Layer: "l1", Name: "b"}},
		Edges: []snapshot.EdgeSpec{},
	}, graph.ApplyOptions{ForceCascade: true})
	require.NoError(t, err)
	require.Equal(t, 1, res.NodesRemoved)
	es, _ := fs.EdgesFrom(t.Context(), "d:a", nil)
	require.Empty(t, es, "edge cascade-removed along with node, foreign claim dropped")
}

func TestApplyForceDomainTakeoverBypassesForeignCheck(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes:      []snapshot.NodeSpec{{ID: "d:a", Layer: "l1", Name: "a"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	_, err = svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "y", Domain: "d", Scope: snapshot.ScopeDomain,
		Nodes:          []snapshot.NodeSpec{{ID: "d:b", Layer: "l1", Name: "b"}},
	}, graph.ApplyOptions{ForceDomainTakeover: true})
	require.NoError(t, err)
}

func TestApplyOverrideScopeAdditivePreservesUnclaimedEdges(t *testing.T) {
	svc, fs := newService(t)
	base := snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{{Src: "d:a", Target: "d:b", Type: "imports"}},
	}
	_, err := svc.Apply(t.Context(), base, graph.ApplyOptions{})
	require.NoError(t, err)

	base.Edges = nil
	_, err = svc.Apply(t.Context(), base, graph.ApplyOptions{OverrideScope: snapshot.ScopeAdditive})
	require.NoError(t, err)
	es, _ := fs.EdgesFrom(t.Context(), "d:a", nil)
	require.Len(t, es, 1, "additive override should preserve edges")
}

func TestApplyBlocksRemovalOfNodeWithForeignEdgeClaim(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{
			{ID: "d:a", Layer: "l1", Name: "a"}, {ID: "d:b", Layer: "l1", Name: "b"},
		},
		Edges: []snapshot.EdgeSpec{{Src: "d:a", Target: "d:b", Type: "imports"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	_, err = svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: "d:a", Target: "d:b", Type: "imports", WriterSource: "y"})
	require.NoError(t, err)

	_, err = svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: snapshot.ScopeDomainSource,
		Nodes: []snapshot.NodeSpec{{ID: "d:b", Layer: "l1", Name: "b"}},
		Edges: []snapshot.EdgeSpec{},
	}, graph.ApplyOptions{})
	require.ErrorIs(t, err, graph.ErrNodeHasForeignClaims, "should block removal without ForceCascade")
}

func TestApplyAdditiveAnnotatesForeignNode(t *testing.T) {
	svc, fs := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "a", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes: []snapshot.NodeSpec{{
			ID: "d:x", Layer: "l1", Name: "x",
			Properties: map[string]any{"a-key": "a-val"},
		}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "b", Domain: "d", Scope: snapshot.ScopeAdditive,
		Nodes: []snapshot.NodeSpec{{
			ID: "d:x",
			Properties: map[string]any{"b-key": "b-val"},
		}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.NodesUpdated, "additive annotation counts as an update")

	n, err := fs.GetNode(t.Context(), "d:x")
	require.NoError(t, err)
	require.Equal(t, graph.SourceID("a"), n.Source, "ownership unchanged")
	require.Equal(t, "x", n.Name, "name untouched")
	require.Equal(t, "a-val", n.Properties["a"]["a-key"])
	require.Equal(t, "b-val", n.Properties["b"]["b-key"], "B's namespace populated")
}

func TestApplyAdditiveSkipsForeignNodeWithoutProperties(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "a", Domain: "d", Scope: snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{ID: "d", Layers: []string{"l1"}},
		Nodes:      []snapshot.NodeSpec{{ID: "d:x", Layer: "l1", Name: "x"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)

	res, err := svc.Apply(t.Context(), snapshot.Snapshot{
		ProtocolVersion: 2, Source: "b", Domain: "d", Scope: snapshot.ScopeAdditive,
		Nodes:          []snapshot.NodeSpec{{ID: "d:x"}},
	}, graph.ApplyOptions{})
	require.NoError(t, err)
	require.Equal(t, 0, res.NodesUpdated)
}
