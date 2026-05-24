package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func seedTwoNodes(t *testing.T, svc *graph.Service, source string) (graph.NodeID, graph.NodeID) {
	t.Helper()
	seedCarsDomain(t, svc)
	a, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt", Source: source})
	require.NoError(t, err)
	b, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "chassis", Source: source})
	require.NoError(t, err)
	return a.ID, b.ID
}

func TestAddEdgeUpsertsAndClaims(t *testing.T) {
	svc, fs := newService(t)
	a, b := seedTwoNodes(t, svc, "x")
	e, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{
		Source: string(a), Target: string(b), Type: "imports", WriterSource: "x",
	})
	require.NoError(t, err)
	claims, err := fs.ListEdgeClaims(t.Context(), e.ID)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, graph.SourceID("x"), claims[0].Source)
}

func TestAddSameEdgeFromTwoSourcesProducesTwoClaims(t *testing.T) {
	svc, fs := newService(t)
	a, b := seedTwoNodes(t, svc, "x")
	e1, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "imports", WriterSource: "x"})
	require.NoError(t, err)
	e2, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "imports", WriterSource: "y"})
	require.NoError(t, err)
	require.Equal(t, e1.ID, e2.ID, "same physical edge")
	claims, err := fs.ListEdgeClaims(t.Context(), e1.ID)
	require.NoError(t, err)
	require.Len(t, claims, 2)
}

func TestRemoveEdgeClaimGCsWhenLast(t *testing.T) {
	svc, fs := newService(t)
	a, b := seedTwoNodes(t, svc, "x")
	e, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "imports", WriterSource: "x"})
	require.NoError(t, err)
	require.NoError(t, svc.RemoveEdgeClaim(t.Context(), e.ID, "x"))
	_, gerr := fs.GetEdge(t.Context(), e.ID)
	require.ErrorIs(t, gerr, graph.ErrEdgeNotFound)
}

func TestRemoveOneOfTwoClaimsKeepsEdgeAlive(t *testing.T) {
	svc, fs := newService(t)
	a, b := seedTwoNodes(t, svc, "x")
	e, _ := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "imports", WriterSource: "x"})
	_, _ = svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "imports", WriterSource: "y"})
	require.NoError(t, svc.RemoveEdgeClaim(t.Context(), e.ID, "x"))
	_, gerr := fs.GetEdge(t.Context(), e.ID)
	require.NoError(t, gerr)
}

func TestAddEdgeRejectsSelfLoop(t *testing.T) {
	svc, _ := newService(t)
	a, _ := seedTwoNodes(t, svc, "manual")
	_, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(a), Type: "x", WriterSource: "manual"})
	require.ErrorIs(t, err, graph.ErrEdgeSelfLoop)
}

func TestAddEdgeMissingEndpoint(t *testing.T) {
	svc, _ := newService(t)
	a, _ := seedTwoNodes(t, svc, "manual")
	_, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: "cars:missing", Type: "x", WriterSource: "manual"})
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}

func TestAddEdgeRejectsEmptyType(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc, "manual")
	_, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "", WriterSource: "manual"})
	require.Error(t, err)
}

func TestAddEdgeRejectsEmptyWriterSource(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc, "manual")
	_, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "x", WriterSource: ""})
	require.ErrorIs(t, err, graph.ErrSourceRequired)
}

func TestEdgesFromAndTo(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc, "manual")
	_, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "depends_on", WriterSource: "manual"})
	require.NoError(t, err)

	out, err := svc.EdgesFrom(t.Context(), a, nil)
	require.NoError(t, err)
	require.Len(t, out, 1)

	in, err := svc.EdgesTo(t.Context(), b, nil)
	require.NoError(t, err)
	require.Len(t, in, 1)
}

func TestDeleteEdge(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc, "manual")
	e, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "depends_on", WriterSource: "manual"})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteEdge(t.Context(), e.ID))
	require.ErrorIs(t, svc.DeleteEdge(t.Context(), e.ID), graph.ErrEdgeNotFound)
}

func TestAddEdgeHasEmptyNamespacedProperties(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc, "manual")
	e, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{
		Source: string(a), Target: string(b), Type: "x", WriterSource: "manual",
	})
	require.NoError(t, err)
	require.NotNil(t, e.Properties)
}

func TestAddEdgeStoresProperties(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc, "manual")
	e, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{
		Source: string(a), Target: string(b), Type: "x",
		Properties:   map[string]any{"weight": 42},
		WriterSource: "manual",
	})
	require.NoError(t, err)
	require.Equal(t, 42, e.Properties["manual"]["weight"])
}
