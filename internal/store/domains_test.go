package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestDomainCRUD(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	d := graph.Domain{
		ID: "cars", Description: "vehicles",
		Layers:    []string{"system", "subsystem"},
		CreatedAt: time.UnixMilli(1700000000000),
	}
	require.NoError(t, s.CreateDomain(ctx, d))

	got, err := s.GetDomain(ctx, "cars")
	require.NoError(t, err)
	require.Equal(t, d.ID, got.ID)
	require.Equal(t, d.Layers, got.Layers)
	require.Equal(t, int64(1), got.Revision)

	list, err := s.ListDomains(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, s.DeleteDomain(ctx, "cars"))
	_, err = s.GetDomain(ctx, "cars")
	require.ErrorIs(t, err, graph.ErrDomainNotFound)
}

func TestCreateDomainDuplicate(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	d := graph.Domain{ID: "cars", Layers: []string{"x"}, CreatedAt: time.UnixMilli(1)}
	require.NoError(t, s.CreateDomain(ctx, d))
	require.ErrorIs(t, s.CreateDomain(ctx, d), graph.ErrDomainAlreadyExists)
}

func TestDeleteDomainWithNodesIsRestricted(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	require.NoError(t, s.CreateDomain(ctx, graph.Domain{ID: "cars", Layers: []string{"system"}, CreatedAt: time.UnixMilli(1)}))
	require.NoError(t, s.CreateNode(ctx, graph.Node{
		ID: "cars:pt", Domain: "cars", Layer: "system", Name: "PT",
		Properties: map[string]any{}, CreatedAt: time.UnixMilli(2), UpdatedAt: time.UnixMilli(2),
	}))

	require.ErrorIs(t, s.DeleteDomain(ctx, "cars"), graph.ErrHasDependents)

	got, err := s.GetDomain(ctx, "cars")
	require.NoError(t, err, "domain must survive a blocked delete")
	require.Equal(t, graph.DomainID("cars"), got.ID)
}

func TestDomainChangesLogged(t *testing.T) {
	s := openTestDB(t)
	ctx := t.Context()
	d := graph.Domain{ID: "cars", Layers: []string{"x"}, CreatedAt: time.UnixMilli(1)}
	require.NoError(t, s.CreateDomain(ctx, d))
	require.NoError(t, s.DeleteDomain(ctx, "cars"))

	rows, err := s.DB().Query(`SELECT entity, op, revision FROM changes ORDER BY seq`)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rows.Close() })

	type ch struct {
		entity, op string
		rev        *int64
	}
	var out []ch
	for rows.Next() {
		var c ch
		require.NoError(t, rows.Scan(&c.entity, &c.op, &c.rev))
		out = append(out, c)
	}
	require.Len(t, out, 2)
	require.Equal(t, "domain", out[0].entity)
	require.Equal(t, "create", out[0].op)
	require.Equal(t, "delete", out[1].op)
	require.Nil(t, out[1].rev)
}
