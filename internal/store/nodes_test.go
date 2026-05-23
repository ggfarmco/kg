package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/store"
)

func seedDomain(t *testing.T, s *store.Store) {
	t.Helper()
	require.NoError(t, s.CreateDomain(t.Context(), graph.Domain{
		ID: "cars", Layers: []string{"system", "subsystem", "part"}, CreatedAt: time.UnixMilli(1),
	}))
}

func TestNodeCRUDAndRevision(t *testing.T) {
	s := openTestDB(t)
	seedDomain(t, s)
	ctx := t.Context()

	require.NoError(t, s.CreateNode(ctx, graph.Node{
		ID: "cars:pt", Domain: "cars", Layer: "system", Name: "Powertrain",
		Properties: map[string]any{}, CreatedAt: time.UnixMilli(1), UpdatedAt: time.UnixMilli(1),
	}))

	got, err := s.GetNode(ctx, "cars:pt")
	require.NoError(t, err)
	require.Equal(t, "Powertrain", got.Name)
	require.Equal(t, int64(1), got.Revision)

	got.Name = "Drive"
	got.UpdatedAt = time.UnixMilli(2)
	require.NoError(t, s.UpdateNode(ctx, *got))

	after, err := s.GetNode(ctx, "cars:pt")
	require.NoError(t, err)
	require.Equal(t, "Drive", after.Name)
	require.Equal(t, int64(2), after.Revision)

	list, err := s.ListNodes(ctx, graph.NodeFilter{Domain: "cars"})
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, s.DeleteNode(ctx, "cars:pt"))
	_, err = s.GetNode(ctx, "cars:pt")
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}

func TestNodeChangesLog(t *testing.T) {
	s := openTestDB(t)
	seedDomain(t, s)
	ctx := t.Context()

	n := graph.Node{ID: "cars:pt", Domain: "cars", Layer: "system", Name: "PT", Properties: map[string]any{}, CreatedAt: time.UnixMilli(1), UpdatedAt: time.UnixMilli(1)}
	require.NoError(t, s.CreateNode(ctx, n))
	require.NoError(t, s.UpdateNode(ctx, n))
	require.NoError(t, s.DeleteNode(ctx, "cars:pt"))

	rows, err := s.DB().Query(`SELECT op, revision FROM changes WHERE entity='node' ORDER BY seq`)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rows.Close() })

	var ops []string
	var revs []*int64
	for rows.Next() {
		var op string
		var rev *int64
		require.NoError(t, rows.Scan(&op, &rev))
		ops = append(ops, op)
		revs = append(revs, rev)
	}
	require.Equal(t, []string{"create", "update", "delete"}, ops)
	require.NotNil(t, revs[0])
	require.Equal(t, int64(1), *revs[0])
	require.NotNil(t, revs[1])
	require.Equal(t, int64(2), *revs[1])
	require.Nil(t, revs[2])
}

func TestChildrenOf(t *testing.T) {
	s := openTestDB(t)
	seedDomain(t, s)
	ctx := t.Context()

	require.NoError(t, s.CreateNode(ctx, graph.Node{ID: "cars:pt", Domain: "cars", Layer: "system", Name: "PT", Properties: map[string]any{}, CreatedAt: time.UnixMilli(1), UpdatedAt: time.UnixMilli(1)}))
	pt := graph.NodeID("cars:pt")
	require.NoError(t, s.CreateNode(ctx, graph.Node{ID: "cars:engine", Domain: "cars", Layer: "subsystem", Name: "Engine", ParentID: &pt, Properties: map[string]any{}, CreatedAt: time.UnixMilli(2), UpdatedAt: time.UnixMilli(2)}))

	kids, err := s.ChildrenOf(ctx, pt)
	require.NoError(t, err)
	require.Len(t, kids, 1)
	require.Equal(t, graph.NodeID("cars:engine"), kids[0].ID)
}
