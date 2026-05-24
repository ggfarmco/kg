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
	seedDomain(t, s)
	now := time.UnixMilli(1)
	require.NoError(t, s.CreateNode(t.Context(), graph.Node{
		ID: "cars:a", Domain: "cars", Layer: "system", Name: "A", Source: "manual",
		Properties: map[graph.SourceID]map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, s.CreateNode(t.Context(), graph.Node{
		ID: "cars:b", Domain: "cars", Layer: "system", Name: "B", Source: "manual",
		Properties: map[graph.SourceID]map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}))
	return "cars:a", "cars:b"
}

func TestEdgeCRUD(t *testing.T) {
	s := openTestDB(t)
	a, b := seedTwoNodes(t, s)
	ctx := t.Context()

	id, err := s.UpsertEdge(ctx, graph.Edge{
		SourceID: a, TargetID: b, Type: "depends_on",
		Properties: map[graph.SourceID]map[string]any{}, CreatedAt: time.UnixMilli(1),
	})
	require.NoError(t, err)
	require.NotZero(t, id)

	got, err := s.GetEdge(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "depends_on", got.Type)

	from, err := s.EdgesFrom(ctx, a, nil)
	require.NoError(t, err)
	require.Len(t, from, 1)

	to, err := s.EdgesTo(ctx, b, []string{"depends_on"})
	require.NoError(t, err)
	require.Len(t, to, 1)

	require.NoError(t, s.DeleteEdge(ctx, id))
	_, err = s.GetEdge(ctx, id)
	require.ErrorIs(t, err, graph.ErrEdgeNotFound)
}

func TestUpsertEdgeIsIdempotent(t *testing.T) {
	s := openTestDB(t)
	a, b := seedTwoNodes(t, s)
	ctx := t.Context()
	id1, err := s.UpsertEdge(ctx, graph.Edge{
		SourceID: a, TargetID: b, Type: "x",
		Properties: map[graph.SourceID]map[string]any{}, CreatedAt: time.UnixMilli(1),
	})
	require.NoError(t, err)
	id2, err := s.UpsertEdge(ctx, graph.Edge{
		SourceID: a, TargetID: b, Type: "x",
		Properties: map[graph.SourceID]map[string]any{}, CreatedAt: time.UnixMilli(2),
	})
	require.NoError(t, err)
	require.Equal(t, id1, id2, "upsert must return same id for duplicate key")
}

func TestEdgeCascadeOnNodeDelete(t *testing.T) {
	s := openTestDB(t)
	a, b := seedTwoNodes(t, s)
	ctx := t.Context()
	id, err := s.UpsertEdge(ctx, graph.Edge{
		SourceID: a, TargetID: b, Type: "x",
		Properties: map[graph.SourceID]map[string]any{}, CreatedAt: time.UnixMilli(1),
	})
	require.NoError(t, err)

	require.NoError(t, s.DeleteNode(ctx, b))
	_, err = s.GetEdge(ctx, id)
	require.ErrorIs(t, err, graph.ErrEdgeNotFound)
}
