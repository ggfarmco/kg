package store_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/store"
)

func openTestDB(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenAppliesMigrations(t *testing.T) {
	s := openTestDB(t)
	require.NotNil(t, s.DB())
	var got string
	require.NoError(t, s.DB().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='domains'`).Scan(&got))
	require.Equal(t, "domains", got)
}
