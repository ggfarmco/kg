package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func seedTwoNodes(t *testing.T, svc *graph.Service) (graph.NodeID, graph.NodeID) {
	t.Helper()
	seedCarsDomain(t, svc)
	a, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"})
	require.NoError(t, err)
	b, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "chassis"})
	require.NoError(t, err)
	return a.ID, b.ID
}

func TestAddEdgeHappyPath(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc)
	e, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "depends_on"})
	require.NoError(t, err)
	require.NotZero(t, e.ID)
}

func TestAddEdgeRejectsSelfLoop(t *testing.T) {
	svc, _ := newService(t)
	a, _ := seedTwoNodes(t, svc)
	_, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(a), Type: "x"})
	require.ErrorIs(t, err, graph.ErrEdgeSelfLoop)
}

func TestAddEdgeMissingEndpoint(t *testing.T) {
	svc, _ := newService(t)
	a, _ := seedTwoNodes(t, svc)
	_, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: "cars:missing", Type: "x"})
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}

func TestAddEdgeRejectsEmptyType(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc)
	_, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: ""})
	require.Error(t, err)
}

func TestAddEdgeDuplicate(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc)
	in := graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "depends_on"}
	_, err := svc.AddEdge(t.Context(), in)
	require.NoError(t, err)
	_, err = svc.AddEdge(t.Context(), in)
	require.ErrorIs(t, err, graph.ErrEdgeAlreadyExists)
}

func TestEdgesFromAndTo(t *testing.T) {
	svc, _ := newService(t)
	a, b := seedTwoNodes(t, svc)
	_, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "depends_on"})
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
	a, b := seedTwoNodes(t, svc)
	e, err := svc.AddEdge(t.Context(), graph.AddEdgeInput{Source: string(a), Target: string(b), Type: "depends_on"})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteEdge(t.Context(), e.ID))
	require.ErrorIs(t, svc.DeleteEdge(t.Context(), e.ID), graph.ErrEdgeNotFound)
}
