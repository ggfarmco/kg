package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAddAndCountEdgeClaims(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	edgeID := seedTwoNodesAndEdge(t, st, "x")
	require.NoError(t, st.AddEdgeClaim(ctx, edgeID, "x", time.UnixMilli(1)))
	n, err := st.CountEdgeClaims(ctx, edgeID)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	require.NoError(t, st.AddEdgeClaim(ctx, edgeID, "x", time.UnixMilli(2)))
	n, err = st.CountEdgeClaims(ctx, edgeID)
	require.NoError(t, err)
	require.Equal(t, 1, n, "INSERT OR IGNORE — duplicate (edge_id, source) must not double-count")
}

func TestRemoveEdgeClaim(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	edgeID := seedTwoNodesAndEdge(t, st, "x")
	require.NoError(t, st.AddEdgeClaim(ctx, edgeID, "x", time.UnixMilli(1)))
	require.NoError(t, st.RemoveEdgeClaim(ctx, edgeID, "x"))
	n, err := st.CountEdgeClaims(ctx, edgeID)
	require.NoError(t, err)
	require.Equal(t, 0, n)
}
