package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/store"
)

func seedTwoNodes(t *testing.T, s *store.Store) (graph.NodeID, graph.NodeID) {
	t.Helper()
	now := time.UnixMilli(1)
	require.NoError(t, s.CreateNode(t.Context(), graph.Node{ID: "cars:a", Domain: "cars", Layer: "system", Name: "A", Properties: map[string]any{}, CreatedAt: now, UpdatedAt: now}))
	require.NoError(t, s.CreateNode(t.Context(), graph.Node{ID: "cars:b", Domain: "cars", Layer: "system", Name: "B", Properties: map[string]any{}, CreatedAt: now, UpdatedAt: now}))
	return "cars:a", "cars:b"
}

func TestEdgeCRUD(t *testing.T) {
	s := openTestDB(t)
	seedDomain(t, s)
	a, b := seedTwoNodes(t, s)
	ctx := t.Context()

	e := &graph.Edge{SourceID: a, TargetID: b, Type: "depends_on", Properties: map[string]any{}, CreatedAt: time.UnixMilli(1)}
	require.NoError(t, s.CreateEdge(ctx, e))
	require.NotZero(t, e.ID)

	got, err := s.GetEdge(ctx, e.ID)
	require.NoError(t, err)
	require.Equal(t, "depends_on", got.Type)

	from, err := s.EdgesFrom(ctx, a, nil)
	require.NoError(t, err)
	require.Len(t, from, 1)

	to, err := s.EdgesTo(ctx, b, []string{"depends_on"})
	require.NoError(t, err)
	require.Len(t, to, 1)

	require.NoError(t, s.DeleteEdge(ctx, e.ID))
	_, err = s.GetEdge(ctx, e.ID)
	require.ErrorIs(t, err, graph.ErrEdgeNotFound)
}

func TestEdgeUniqueViolation(t *testing.T) {
	s := openTestDB(t)
	seedDomain(t, s)
	a, b := seedTwoNodes(t, s)
	ctx := t.Context()
	e := &graph.Edge{SourceID: a, TargetID: b, Type: "x", Properties: map[string]any{}, CreatedAt: time.UnixMilli(1)}
	require.NoError(t, s.CreateEdge(ctx, e))
	dup := &graph.Edge{SourceID: a, TargetID: b, Type: "x", Properties: map[string]any{}, CreatedAt: time.UnixMilli(2)}
	require.ErrorIs(t, s.CreateEdge(ctx, dup), graph.ErrEdgeAlreadyExists)
}

func TestEdgeCascadeOnNodeDelete(t *testing.T) {
	s := openTestDB(t)
	seedDomain(t, s)
	a, b := seedTwoNodes(t, s)
	ctx := t.Context()
	e := &graph.Edge{SourceID: a, TargetID: b, Type: "x", Properties: map[string]any{}, CreatedAt: time.UnixMilli(1)}
	require.NoError(t, s.CreateEdge(ctx, e))

	require.NoError(t, s.DeleteNode(ctx, b))
	_, err := s.GetEdge(ctx, e.ID)
	require.ErrorIs(t, err, graph.ErrEdgeNotFound)
}
