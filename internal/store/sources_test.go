package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestSourcesUpsertAndGet(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	require.NoError(t, st.UpsertSource(ctx, graph.Source{
		ID: "tree-sitter:0.1.0", Description: "ts", Trust: 100,
		FirstSeen: time.UnixMilli(1000), LastSeen: time.UnixMilli(1000),
	}))
	got, err := st.GetSource(ctx, "tree-sitter:0.1.0")
	require.NoError(t, err)
	require.Equal(t, "ts", got.Description)
	require.Equal(t, 100, got.Trust)
}

func TestSourcesUpsertBumpsLastSeen(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	require.NoError(t, st.UpsertSource(ctx, graph.Source{
		ID: "x", Trust: 100,
		FirstSeen: time.UnixMilli(1000), LastSeen: time.UnixMilli(1000),
	}))
	require.NoError(t, st.UpsertSource(ctx, graph.Source{
		ID: "x", Trust: 100,
		FirstSeen: time.UnixMilli(1000), LastSeen: time.UnixMilli(2000),
	}))
	got, err := st.GetSource(ctx, "x")
	require.NoError(t, err)
	require.Equal(t, int64(2000), got.LastSeen.UnixMilli())
	require.Equal(t, int64(1000), got.FirstSeen.UnixMilli())
}

func TestDeleteSourceWithOwnedNodeFails(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	seedDomainAndSource(t, st, "d", "x")
	require.NoError(t, st.CreateNode(ctx, graph.Node{
		ID: "d:n", Domain: "d", Layer: "l1", Name: "n", Source: "x",
		Properties: map[graph.SourceID]map[string]any{},
		CreatedAt:  time.UnixMilli(1), UpdatedAt: time.UnixMilli(1),
	}))
	err := st.DeleteSource(ctx, "x")
	require.ErrorIs(t, err, graph.ErrSourceHasDependents)
}
