package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/store"
)

func TestInTxCommit(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	err := s.InTx(ctx, func(ctx context.Context) error {
		_, err := store.ExecForTest(ctx, s, `INSERT INTO domains(id, layers, created_at) VALUES ('x','["a"]',1)`)
		return err
	})
	require.NoError(t, err)

	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM domains`).Scan(&n))
	require.Equal(t, 1, n)
}

func TestInTxRollback(t *testing.T) {
	s := openTestDB(t)
	wantErr := errors.New("boom")
	err := s.InTx(t.Context(), func(ctx context.Context) error {
		_, _ = store.ExecForTest(ctx, s, `INSERT INTO domains(id, layers, created_at) VALUES ('x','["a"]',1)`)
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)

	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM domains`).Scan(&n))
	require.Equal(t, 0, n)
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
			_, _ = store.ExecForTest(ctx, s, `INSERT INTO domains(id, layers, created_at) VALUES ('p','["a"]',1)`)
			panic("nope")
		})
	})

	var n int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM domains`).Scan(&n))
	require.Equal(t, 0, n)
}
