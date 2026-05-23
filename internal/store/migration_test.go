package store

import (
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/ggfarmco/kg/migrations"
)

func TestMigrationsUp(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	goose.SetBaseFS(migrations.FS)
	t.Cleanup(func() { goose.SetBaseFS(nil) })
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpContext(t.Context(), db, "."))

	want := []string{"domains", "nodes", "edges", "changes"}
	for _, name := range want {
		var got string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&got)
		require.NoError(t, err, "table %s should exist", name)
		require.Equal(t, name, got)
	}
}
