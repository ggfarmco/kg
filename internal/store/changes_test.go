package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestChangesSeqMonotonicAcrossMutationsAndDeletes(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	require.NoError(t, s.CreateDomain(ctx, graph.Domain{ID: "cars", Layers: []string{"system"}, CreatedAt: time.UnixMilli(1)}))
	require.NoError(t, s.UpsertSource(ctx, graph.Source{
		ID:        "manual",
		FirstSeen: time.UnixMilli(1), LastSeen: time.UnixMilli(1),
	}))
	require.NoError(t, s.CreateNode(ctx, graph.Node{
		ID: "cars:a", Domain: "cars", Layer: "system", Name: "A", Source: "manual",
		Properties: map[graph.SourceID]map[string]any{}, CreatedAt: time.UnixMilli(2), UpdatedAt: time.UnixMilli(2),
	}))
	require.NoError(t, s.DeleteNode(ctx, "cars:a"))
	require.NoError(t, s.CreateNode(ctx, graph.Node{
		ID: "cars:b", Domain: "cars", Layer: "system", Name: "B", Source: "manual",
		Properties: map[graph.SourceID]map[string]any{}, CreatedAt: time.UnixMilli(3), UpdatedAt: time.UnixMilli(3),
	}))

	rows, err := s.DB().Query(`SELECT seq FROM changes ORDER BY seq`)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rows.Close() })

	var seqs []int64
	for rows.Next() {
		var seq int64
		require.NoError(t, rows.Scan(&seq))
		seqs = append(seqs, seq)
	}
	require.Len(t, seqs, 4)
	for i := 1; i < len(seqs); i++ {
		require.Greater(t, seqs[i], seqs[i-1], "seq must be strictly increasing")
	}
	require.Equal(t, int64(1), seqs[0])
	require.Equal(t, int64(4), seqs[3])
}

func TestChangesRolledBackWhenTxFails(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	require.NoError(t, s.CreateDomain(ctx, graph.Domain{ID: "cars", Layers: []string{"system"}, CreatedAt: time.UnixMilli(1)}))
	require.NoError(t, s.UpsertSource(ctx, graph.Source{
		ID:        "manual",
		FirstSeen: time.UnixMilli(1), LastSeen: time.UnixMilli(1),
	}))

	wantErr := errors.New("boom")
	_ = s.InTx(ctx, func(ctx context.Context) error {
		_ = s.CreateNode(ctx, graph.Node{
			ID: "cars:a", Domain: "cars", Layer: "system", Name: "A", Source: "manual",
			Properties: map[graph.SourceID]map[string]any{}, CreatedAt: time.UnixMilli(2), UpdatedAt: time.UnixMilli(2),
		})
		return wantErr
	})

	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM changes WHERE entity='node'`).Scan(&n))
	require.Equal(t, 0, n)
}
