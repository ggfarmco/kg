package testutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/graph/testutil"
)

func TestFakeStoreRoundtrip(t *testing.T) {
	s := testutil.NewFakeStore()
	ctx := t.Context()

	d := graph.Domain{
		ID:        "cars",
		Layers:    []string{"system", "subsystem", "part"},
		CreatedAt: time.UnixMilli(1),
	}
	require.NoError(t, s.CreateDomain(ctx, d))

	got, err := s.GetDomain(ctx, "cars")
	require.NoError(t, err)
	require.Equal(t, d.ID, got.ID)
	require.Equal(t, d.Layers, got.Layers)

	require.ErrorIs(t, s.CreateDomain(ctx, d), graph.ErrDomainAlreadyExists)

	_, err = s.GetDomain(ctx, "missing")
	require.ErrorIs(t, err, graph.ErrDomainNotFound)
}

func TestFakeStoreInTxRollback(t *testing.T) {
	s := testutil.NewFakeStore()
	ctx := t.Context()
	require.NoError(t, s.CreateDomain(ctx, graph.Domain{ID: "cars", Layers: []string{"system"}, CreatedAt: time.UnixMilli(1)}))

	wantErr := graph.ErrDomainAlreadyExists
	err := s.InTx(ctx, func(ctx context.Context) error {
		_ = s.DeleteDomain(ctx, "cars")
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)

	_, err = s.GetDomain(ctx, "cars")
	require.NoError(t, err, "rollback should restore the domain")
}
