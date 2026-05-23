package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestGetNodeNotFound(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.GetNode(t.Context(), "cars:missing")
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}

func TestListNodesFilterAndLimit(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	for _, name := range []string{"pt", "chassis", "body"} {
		_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: name})
		require.NoError(t, err)
	}
	all, err := svc.ListNodes(t.Context(), graph.NodeFilter{Domain: "cars"})
	require.NoError(t, err)
	require.Len(t, all, 3)

	limited, err := svc.ListNodes(t.Context(), graph.NodeFilter{Domain: "cars", Layer: "system", Limit: 2})
	require.NoError(t, err)
	require.Len(t, limited, 2)
}

func TestChildrenOf(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine", Parent: "cars:pt"})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "transmission", Parent: "cars:pt"})
	require.NoError(t, err)

	kids, err := svc.ChildrenOf(t.Context(), "cars:pt")
	require.NoError(t, err)
	require.Len(t, kids, 2)
}

func TestUpdateNodeBumpsRevision(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"})
	require.NoError(t, err)

	newSummary := "powertrain summary"
	updated, err := svc.UpdateNode(t.Context(), "cars:pt", graph.UpdateNodeInput{Summary: &newSummary})
	require.NoError(t, err)
	require.Equal(t, "powertrain summary", updated.Summary)
	require.Equal(t, int64(2), updated.Revision)
}

func TestUpdateNodeNotFound(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.UpdateNode(t.Context(), "cars:missing", graph.UpdateNodeInput{})
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}

func TestDeleteNode(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteNode(t.Context(), "cars:pt"))
	_, err = svc.GetNode(t.Context(), "cars:pt")
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}
