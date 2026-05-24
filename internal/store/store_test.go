package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/store"
)

func openTestDB(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	return openTestDB(t)
}

func TestOpenAppliesMigrations(t *testing.T) {
	s := openTestDB(t)
	require.NotNil(t, s.DB())
	var got string
	require.NoError(t, s.DB().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='domains'`).Scan(&got))
	require.Equal(t, "domains", got)
}

func seedDomainAndSource(t *testing.T, st *store.Store, domain, source string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, st.UpsertSource(ctx, graph.Source{
		ID: graph.SourceID(source), Trust: 100,
		FirstSeen: time.UnixMilli(1), LastSeen: time.UnixMilli(1),
	}))
	require.NoError(t, st.CreateDomain(ctx, graph.Domain{
		ID: graph.DomainID(domain), Layers: []string{"l1", "l2"}, Revision: 1, CreatedAt: time.UnixMilli(1),
	}))
}

func seedTwoNodesAndEdge(t *testing.T, st *store.Store, source string) graph.EdgeID {
	t.Helper()
	ctx := context.Background()
	seedDomainAndSource(t, st, "d", source)
	require.NoError(t, st.CreateNode(ctx, graph.Node{
		ID: "d:a", Domain: "d", Layer: "l1", Name: "a", Source: graph.SourceID(source),
		Properties: map[graph.SourceID]map[string]any{},
		CreatedAt:  time.UnixMilli(1), UpdatedAt: time.UnixMilli(1),
	}))
	require.NoError(t, st.CreateNode(ctx, graph.Node{
		ID: "d:b", Domain: "d", Layer: "l1", Name: "b", Source: graph.SourceID(source),
		Properties: map[graph.SourceID]map[string]any{},
		CreatedAt:  time.UnixMilli(1), UpdatedAt: time.UnixMilli(1),
	}))
	id, err := st.UpsertEdge(ctx, graph.Edge{
		SourceID: "d:a", TargetID: "d:b", Type: "imports",
		Properties: map[graph.SourceID]map[string]any{},
		CreatedAt:  time.UnixMilli(1),
	})
	require.NoError(t, err)
	return id
}
