package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestInTxCommit(t *testing.T) {
	s := openTestDB(t)
	err := s.InTx(t.Context(), func(ctx context.Context) error {
		return s.CreateDomain(ctx, graph.Domain{ID: "x", Layers: []string{"a"}, CreatedAt: time.UnixMilli(1)})
	})
	require.NoError(t, err)
	_, err = s.GetDomain(t.Context(), "x")
	require.NoError(t, err)
}

func TestInTxRollback(t *testing.T) {
	s := openTestDB(t)
	wantErr := errors.New("boom")
	err := s.InTx(t.Context(), func(ctx context.Context) error {
		_ = s.CreateDomain(ctx, graph.Domain{ID: "x", Layers: []string{"a"}, CreatedAt: time.UnixMilli(1)})
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)
	_, err = s.GetDomain(t.Context(), "x")
	require.ErrorIs(t, err, graph.ErrDomainNotFound)
}

func TestInTxNested(t *testing.T) {
	s := openTestDB(t)
	err := s.InTx(t.Context(), func(ctx context.Context) error {
		return s.InTx(ctx, func(ctx context.Context) error { return nil })
	})
	require.ErrorIs(t, err, graph.ErrNestedTransaction)
}

func TestInTxPanicRollsBack(t *testing.T) {
	s := openTestDB(t)
	require.Panics(t, func() {
		_ = s.InTx(t.Context(), func(ctx context.Context) error {
			_ = s.CreateDomain(ctx, graph.Domain{ID: "p", Layers: []string{"a"}, CreatedAt: time.UnixMilli(1)})
			panic("nope")
		})
	})
	_, err := s.GetDomain(t.Context(), "p")
	require.ErrorIs(t, err, graph.ErrDomainNotFound)
}
